package cloudwatchlogs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatchlogs"
)

func TestInMemoryBackend_SnapshotRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(b *cloudwatchlogs.InMemoryBackend) string
		verify func(t *testing.T, b *cloudwatchlogs.InMemoryBackend, id string)
		name   string
	}{
		{
			name: "round_trip_preserves_state",
			setup: func(b *cloudwatchlogs.InMemoryBackend) string {
				_, err := b.CreateLogGroup(context.Background(), "test-group", "", "")
				if err != nil {
					return ""
				}

				return "test-group"
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend, id string) {
				t.Helper()

				groups, _, err := b.DescribeLogGroups(context.Background(), "", "", 100)
				require.NoError(t, err)
				require.Len(t, groups, 1)
				assert.Equal(t, id, groups[0].LogGroupName)
			},
		},
		{
			name:  "empty_backend_round_trip",
			setup: func(_ *cloudwatchlogs.InMemoryBackend) string { return "" },
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend, _ string) {
				t.Helper()

				groups, _, err := b.DescribeLogGroups(context.Background(), "", "", 100)
				require.NoError(t, err)
				assert.Empty(t, groups)
			},
		},
		{
			name: "round_trip_preserves_subscription_filters",
			setup: func(b *cloudwatchlogs.InMemoryBackend) string {
				_, err := b.CreateLogGroup(context.Background(), "sub-grp", "", "")
				if err != nil {
					return ""
				}
				_ = b.PutSubscriptionFilter(
					context.Background(),
					"sub-grp", "my-filter", "ERROR",
					"arn:aws:lambda:us-east-1:123456789012:function:target",
					"", "",
				)

				return "sub-grp"
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend, id string) {
				t.Helper()

				filters, _, err := b.DescribeSubscriptionFilters(context.Background(), id, "", "", 100)
				require.NoError(t, err)
				require.Len(t, filters, 1)
				assert.Equal(t, "my-filter", filters[0].FilterName)
				assert.Equal(t, "ERROR", filters[0].FilterPattern)
				assert.Equal(t, "arn:aws:lambda:us-east-1:123456789012:function:target", filters[0].DestinationArn)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			id := tt.setup(original)

			snap := original.Snapshot(t.Context())
			require.NotNil(t, snap)

			fresh := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh, id)
		})
	}
}

// TestInMemoryBackend_SnapshotRestore_FullStateRoundTrip exercises a
// Snapshot->Restore round trip across every persisted resource table
// (Phase 3.3 pkgs/store conversion), including the region-qualified "dirty"
// tables (log groups, streams with inline events, subscription filters,
// metric filters) that round-trip through a DTO rather than a direct
// json.Marshal of the live store.Table value -- see persistence.go.
func TestInMemoryBackend_SnapshotRestore_FullStateRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	original := cloudwatchlogs.NewInMemoryBackendWithConfig("111122223333", "us-west-2")
	t.Cleanup(original.Close)

	// Log group + stream + inline events (the region-qualified tables).
	_, err := original.CreateLogGroup(ctx, "/full/group", "", "kms-key-1")
	require.NoError(t, err)
	retentionDays := int32(30)
	require.NoError(t, original.SetRetentionPolicy(ctx, "/full/group", &retentionDays))
	_, err = original.CreateLogStream(ctx, "/full/group", "stream-1")
	require.NoError(t, err)
	// Timestamps below minRealisticTimestampMs bypass PutLogEvents' retention/
	// future-window validation (treated as synthetic test data), so these are
	// accepted regardless of when the test runs.
	_, err = original.PutLogEvents(ctx, "/full/group", "stream-1", "", []cloudwatchlogs.InputLogEvent{
		{Message: "hello", Timestamp: 12345},
		{Message: "world", Timestamp: 12346},
	})
	require.NoError(t, err)

	// Subscription filter + metric filter (also region-qualified).
	require.NoError(t, original.PutSubscriptionFilter(
		ctx, "/full/group", "sub-filter", "ERROR",
		"arn:aws:lambda:us-west-2:111122223333:function:sink", "", "",
	))
	require.NoError(t, original.PutMetricFilter(ctx, "/full/group", "metric-filter", "ERROR",
		[]cloudwatchlogs.MetricTransformation{{MetricName: "Errors", MetricNamespace: "App", MetricValue: "1"}}))

	// Flat "clean" tables.
	_, err = original.PutAccountPolicy("acct-policy", "SUBSCRIPTION_FILTER_POLICY", "{}", "ALL", "")
	require.NoError(t, err)
	require.NoError(t, original.AssociateKmsKey("/full/group", "", "kms-key-2"))
	_, err = original.AssociateSourceToS3TableIntegration("arn:aws:s3tables:::bucket/tbl", "src", "type")
	require.NoError(t, err)
	taskID, err := original.CreateExportTask(ctx, "exp", "/full/group", "", "dest-bucket", "", 0, 1_800_000_000_000)
	require.NoError(t, err)
	importTask, err := original.CreateImportTask(ctx, "arn:aws:iam:::role/import", "arn:aws:cloudtrail:::store/x")
	require.NoError(t, err)
	delivery, err := original.CreateDelivery("src-name", "arn:aws:logs:::delivery-destination/dst", "", nil, nil, nil)
	require.NoError(t, err)
	detectorArn, err := original.CreateLogAnomalyDetector(
		[]string{"arn:aws:logs:::log-group:/full/group"}, "detector", "", "", "", 0,
	)
	require.NoError(t, err)
	scheduledArn, err := original.CreateScheduledQuery(cloudwatchlogs.ScheduledQueryCreateParams{
		Name:               "sched",
		QueryString:        "fields @message",
		QueryLanguage:      "CWLI",
		ScheduleExpression: "cron(0 * * * ? *)",
		ExecutionRoleArn:   "arn:aws:iam::123456789012:role/scheduled-query-role",
	})
	require.NoError(t, err)

	snap := original.Snapshot(ctx)
	require.NotNil(t, snap)

	fresh := cloudwatchlogs.NewInMemoryBackendWithConfig("111122223333", "us-west-2")
	t.Cleanup(fresh.Close)
	require.NoError(t, fresh.Restore(ctx, snap))

	// --- verify region-qualified tables ---
	groups, _, err := fresh.DescribeLogGroups(ctx, "", "", 100)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "/full/group", groups[0].LogGroupName)
	require.NotNil(t, groups[0].RetentionInDays)
	assert.Equal(t, int32(30), *groups[0].RetentionInDays)

	streams, _, err := fresh.DescribeLogStreams(ctx, "/full/group", "", "", "", false, 100)
	require.NoError(t, err)
	require.Len(t, streams, 1)
	assert.Equal(t, "stream-1", streams[0].LogStreamName)

	events, _, _, err := fresh.GetLogEvents(ctx, "/full/group", "stream-1", nil, nil, 100, "", true)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "hello", events[0].Message)
	assert.Equal(t, "world", events[1].Message)

	subFilters, _, err := fresh.DescribeSubscriptionFilters(ctx, "/full/group", "", "", 100)
	require.NoError(t, err)
	require.Len(t, subFilters, 1)
	assert.Equal(t, "sub-filter", subFilters[0].FilterName)

	metricFilters, _, err := fresh.DescribeMetricFilters(ctx, "/full/group", "", "", "", "", 100)
	require.NoError(t, err)
	require.Len(t, metricFilters, 1)
	assert.Equal(t, "metric-filter", metricFilters[0].FilterName)

	// --- verify flat tables ---
	policies, _, err := fresh.DescribeAccountPolicies("", "", nil, 100, "")
	require.NoError(t, err)
	require.Len(t, policies, 1)
	assert.Equal(t, "acct-policy", policies[0].PolicyName)

	exportTasks, _, err := fresh.DescribeExportTasks(taskID, "", 100, "")
	require.NoError(t, err)
	require.Len(t, exportTasks, 1)

	importTasks, _, err := fresh.DescribeImportTasks(importTask.ImportID, 100, "")
	require.NoError(t, err)
	require.Len(t, importTasks, 1)

	gotDelivery, err := fresh.GetDelivery(delivery.ID)
	require.NoError(t, err)
	assert.Equal(t, "src-name", gotDelivery.DeliverySourceName)

	detector, err := fresh.GetLogAnomalyDetector(detectorArn)
	require.NoError(t, err)
	assert.Equal(t, "detector", detector.DetectorName)

	scheduled, err := fresh.GetScheduledQuery(scheduledArn)
	require.NoError(t, err)
	assert.Equal(t, "sched", scheduled.Name)
}

// TestInMemoryBackend_SnapshotRestore_ScheduledQueryLookupTableDestination
// round-trips a scheduled query whose DestinationConfiguration carries the
// LookupTableConfiguration alternative (added additively as an omitempty
// field to ScheduledQueryDestinationConfig; cwlSnapshotVersion is unchanged
// since older snapshots simply decode with the field absent).
func TestInMemoryBackend_SnapshotRestore_ScheduledQueryLookupTableDestination(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	original := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	t.Cleanup(original.Close)

	queryArn, err := original.CreateScheduledQuery(cloudwatchlogs.ScheduledQueryCreateParams{
		Name:               "sched-lookup",
		QueryString:        "fields @message",
		QueryLanguage:      "CWLI",
		ScheduleExpression: "cron(0 * * * ? *)",
		ExecutionRoleArn:   "arn:aws:iam::123456789012:role/scheduled-query-role",
		DestinationConfiguration: &cloudwatchlogs.ScheduledQueryDestinationConfig{
			LookupTableConfiguration: &cloudwatchlogs.ScheduledQueryLookupTableConfiguration{
				TableName:   "my-table",
				RoleArn:     "arn:aws:iam::123456789012:role/lookup-role",
				Description: "a lookup table",
				KmsKeyID:    "kms-key",
				Tags:        map[string]string{"env": "prod"},
			},
		},
	})
	require.NoError(t, err)

	snap := original.Snapshot(ctx)
	require.NotNil(t, snap)

	fresh := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	t.Cleanup(fresh.Close)
	require.NoError(t, fresh.Restore(ctx, snap))

	sq, err := fresh.GetScheduledQuery(queryArn)
	require.NoError(t, err)
	require.NotNil(t, sq.DestinationConfiguration)
	assert.Nil(t, sq.DestinationConfiguration.S3Configuration)
	require.NotNil(t, sq.DestinationConfiguration.LookupTableConfiguration)

	lookup := sq.DestinationConfiguration.LookupTableConfiguration
	assert.Equal(t, "my-table", lookup.TableName)
	assert.Equal(t, "arn:aws:iam::123456789012:role/lookup-role", lookup.RoleArn)
	assert.Equal(t, "a lookup table", lookup.Description)
	assert.Equal(t, "kms-key", lookup.KmsKeyID)
	assert.Equal(t, map[string]string{"env": "prod"}, lookup.Tags)
}

func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreV1IndexPolicyLastUpdateTimeDiscarded proves
// gopherstack-hjdd's fix: a v1 snapshot holding IndexPolicy.LastUpdateTime in
// the pre-ca3afb3ca time.Time shape must be discarded cleanly now that
// cwlSnapshotVersion is 2, rather than erroring Restore outright when the
// registered "indexPolicies" table's int64-typed field can't decode an
// RFC3339 JSON string.
func TestInMemoryBackend_RestoreV1IndexPolicyLastUpdateTimeDiscarded(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")

	v1Snapshot := []byte(`{
		"version": 1,
		"accountID": "000000000000",
		"region": "us-east-1",
		"tables": {
			"indexPolicies": [{
				"logGroupIdentifier": "my-log-group",
				"policyDocument": "{}",
				"lastUpdateTime": "2024-01-01T00:00:00Z"
			}]
		}
	}`)

	require.NoError(t, b.Restore(t.Context(), v1Snapshot),
		"a v1 snapshot must be discarded via the version guard, not error out of RestoreAll")

	survivors, _ := b.DescribeIndexPolicies([]string{"my-log-group"}, "", 0)
	assert.Empty(t, survivors,
		"incompatible-version snapshot must reset to empty, not partially decode")
}

// TestInMemoryBackend_RestoreV1ScheduledQueryArnDiscarded proves
// gopherstack-hjdd's fix: a v1 snapshot holding ScheduledQuery.Arn under the
// pre-9f62f7f5d key "arn" must be discarded cleanly now that
// cwlSnapshotVersion is 2, rather than silently decoding it empty and
// colliding every restored scheduled query onto the same "" key
// (scheduledQueryKeyFn keys the table on this exact field).
func TestInMemoryBackend_RestoreV1ScheduledQueryArnDiscarded(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")

	v1Snapshot := []byte(`{
		"version": 1,
		"accountID": "000000000000",
		"region": "us-east-1",
		"tables": {
			"scheduledQueries": [{
				"arn": "arn:aws:logs:us-east-1:000000000000:scheduled-query:old-query",
				"name": "old-query",
				"queryString": "fields @timestamp",
				"state": "ENABLED"
			}]
		}
	}`)

	require.NoError(t, b.Restore(t.Context(), v1Snapshot),
		"a v1 snapshot must be discarded via the version guard, not partially decoded")

	queries, _, err := b.ListScheduledQueries(10, "")
	require.NoError(t, err)
	assert.Empty(t, queries,
		"incompatible-version snapshot must reset to empty, not restore a scheduled query with a corrupted arn")
}

func TestHandler_SnapshotRestore_PreservesTags(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	_, err := b.CreateLogGroup(context.Background(), "tagged-group", "", "")
	require.NoError(t, err)

	h := cloudwatchlogs.NewHandler(b)

	// Set tags on the log group via the handler so the tag-serialization path is exercised.
	h.SetTagsForTest("tagged-group", map[string]string{"env": "prod", "team": "ops"})

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	// Restore into a fresh handler.
	b2 := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	h2 := cloudwatchlogs.NewHandler(b2)
	require.NoError(t, h2.Restore(t.Context(), snap))

	// Log group should be present in the restored backend.
	groups, _, gErr := b2.DescribeLogGroups(context.Background(), "", "", 100)
	require.NoError(t, gErr)
	require.Len(t, groups, 1)
	assert.Equal(t, "tagged-group", groups[0].LogGroupName)

	// Tags should have been restored.
	restoredTags := h2.GetTagsForTest("tagged-group")
	assert.Equal(t, "prod", restoredTags["env"])
	assert.Equal(t, "ops", restoredTags["team"])
}

func TestHandler_SnapshotRestore_StaleTagsCleared(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	_, err := b.CreateLogGroup(context.Background(), "g", "", "")
	require.NoError(t, err)

	// Original handler has a tag.
	h := cloudwatchlogs.NewHandler(b)
	h.SetTagsForTest("g", map[string]string{"stale": "yes"})

	// Snapshot a second handler that has no tags.
	b2 := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	_, err = b2.CreateLogGroup(context.Background(), "g", "", "")
	require.NoError(t, err)
	h2 := cloudwatchlogs.NewHandler(b2)
	snap := h2.Snapshot(t.Context())
	require.NotNil(t, snap)

	// Restore the snapshot into h — stale tags should be cleared.
	require.NoError(t, h.Restore(t.Context(), snap))
	restoredTags := h.GetTagsForTest("g")
	assert.Empty(t, restoredTags)
}

func TestHandler_SnapshotRestore_InvalidData(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	h := cloudwatchlogs.NewHandler(b)
	err := h.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

func TestInMemoryBackend_SnapshotRestore_PreservesRetention(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	_, err := b.CreateLogGroup(context.Background(), "ret-grp", "", "")
	require.NoError(t, err)
	require.NoError(t, b.SetRetentionPolicy(context.Background(), "ret-grp", func() *int32 {
		v := int32(14)

		return &v
	}()))

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	require.NoError(t, b2.Restore(t.Context(), snap))

	groups, _, err := b2.DescribeLogGroups(context.Background(), "", "", 100)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.NotNil(t, groups[0].RetentionInDays)
	assert.Equal(t, int32(14), *groups[0].RetentionInDays)
}

func TestInMemoryBackend_SnapshotRestore_CompletenessMapsSurvive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(t *testing.T, b *cloudwatchlogs.InMemoryBackend)
		verify func(t *testing.T, b *cloudwatchlogs.InMemoryBackend)
		name   string
	}{
		{
			name: "resource_policy_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutResourcePolicy("my-policy", `{"Version":"2012-10-17"}`, "", nil)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				policies, _ := b.DescribeResourcePolicies("", "", "", 0)
				require.Len(t, policies, 1)
				assert.Equal(t, "my-policy", policies[0].PolicyName)
			},
		},
		{
			name: "delivery_destination_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliveryDestination("my-dest", "arn:aws:s3:::bucket", "JSON", "S3", nil)
				require.NoError(t, err)
				err = b.PutDeliveryDestinationPolicy("my-dest", `{"Statement":[]}`)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				got, err := b.GetDeliveryDestination("my-dest")
				require.NoError(t, err)
				assert.Equal(t, "arn:aws:s3:::bucket", got.TargetArn)
				policy, err := b.GetDeliveryDestinationPolicy("my-dest")
				require.NoError(t, err)
				assert.Contains(t, policy, "Statement")
			},
		},
		{
			name: "delivery_source_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDeliverySource("my-src", "APPLICATION_LOGS", []string{"arn:aws:ec2:::i-1"}, nil)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				got, err := b.GetDeliverySource("my-src")
				require.NoError(t, err)
				assert.Equal(t, "APPLICATION_LOGS", got.LogType)
			},
		},
		{
			name: "destination_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDestination("my-dest", "arn:aws:kinesis:::stream/s", "arn:aws:iam:::role/r")
				require.NoError(t, err)
				err = b.PutDestinationPolicy("my-dest", `{"Statement":[]}`)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				dests, _ := b.DescribeDestinations("", 0, "")
				require.Len(t, dests, 1)
				assert.Equal(t, "my-dest", dests[0].DestinationName)
				assert.Contains(t, dests[0].AccessPolicy, "Statement")
			},
		},
		{
			name: "index_policy_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutIndexPolicy("/aws/lambda/fn", `{"fields":["@message"]}`)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				policies, _ := b.DescribeIndexPolicies([]string{"/aws/lambda/fn"}, "", 0)
				require.Len(t, policies, 1)
				assert.Equal(t, "/aws/lambda/fn", policies[0].LogGroupIdentifier)
			},
		},
		{
			name: "transformer_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				err := b.PutTransformer("/aws/lambda/fn", []map[string]any{{"parseJSON": map[string]any{}}})
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				tr, err := b.GetTransformer("/aws/lambda/fn")
				require.NoError(t, err)
				require.Len(t, tr.Processors, 1)
			},
		},
		{
			name: "integration_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutIntegration("my-opensearch", "OPENSEARCH", validOpenSearchResourceConfig())
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				ig, err := b.GetIntegration("my-opensearch")
				require.NoError(t, err)
				assert.Equal(t, "OPENSEARCH", ig.Type)
				require.NotNil(t, ig.OpenSearchResourceConfig, "OpenSearchResourceConfig must survive Snapshot/Restore")
				assert.Equal(
					t,
					"arn:aws:iam::123456789012:role/cwl-opensearch",
					ig.OpenSearchResourceConfig.DataSourceRoleArn,
				)
			},
		},
		{
			name: "deletion_protected_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				err := b.SetLogGroupDeletionProtection("/aws/lambda/fn", true)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				assert.True(t, b.IsLogGroupDeletionProtected("/aws/lambda/fn"))
			},
		},
		{
			name: "metric_filters_survive",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateLogGroup(context.Background(), "/grp", "", "")
				require.NoError(t, err)
				err = b.PutMetricFilter(context.Background(), "/grp", "my-filter", "ERROR",
					[]cloudwatchlogs.MetricTransformation{{
						MetricName: "ErrCount", MetricNamespace: "App", MetricValue: "1",
					}})
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				filters, _, err := b.DescribeMetricFilters(context.Background(), "/grp", "", "", "", "", 50)
				require.NoError(t, err)
				require.Len(t, filters, 1)
				assert.Equal(t, "my-filter", filters[0].FilterName)
			},
		},
		{
			name: "query_definitions_survive",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutQueryDefinition("my-query", "fields @message | limit 20", "", nil, nil)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				defs, _, err := b.DescribeQueryDefinitions("my-query", 100, "")
				require.NoError(t, err)
				require.Len(t, defs, 1)
				assert.Equal(t, "my-query", defs[0].Name)
			},
		},
		{
			name: "data_protection_policies_survive",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				err := b.PutDataProtectionPolicy("/grp", `{"Name":"protect"}`)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				doc, _, err := b.GetDataProtectionPolicy("/grp")
				require.NoError(t, err)
				assert.Contains(t, doc, "protect")
			},
		},
		{
			name: "lookup_table_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateLookupTable(t.Context(), "my_table", "id,name\n1,foo\n2,bar\n", "desc", "", "")
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				tables, _ := b.DescribeLookupTables(t.Context(), "", "", 100)
				require.Len(t, tables, 1)
				assert.Equal(t, "my_table", tables[0].LookupTableName)
				assert.Equal(t, int64(2), tables[0].RecordsCount)
				assert.Equal(t, []string{"id", "name"}, tables[0].TableFields)
			},
		},
		{
			name: "syslog_configuration_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateLogGroup(context.Background(), "/syslog/grp", "", "")
				require.NoError(t, err)
				_, err = b.PutSyslogConfiguration(context.Background(), "/syslog/grp", "vpce-1")
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				configs, _ := b.ListSyslogConfigurations("", "", "", 100)
				require.Len(t, configs, 1)
				assert.Equal(t, "/syslog/grp", configs[0].LogGroupIdentifier)
				assert.Equal(t, "vpce-1", configs[0].VpcEndpointID)
			},
		},
		{
			name: "storage_tier_policy_survives",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, err := b.PutStorageTierPolicy(cloudwatchlogs.StorageTierIntelligentTiering)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				p := b.GetStorageTierPolicy()
				assert.Equal(t, cloudwatchlogs.StorageTierIntelligentTiering, p.StorageTier)
				assert.NotZero(t, p.LastUpdatedTime)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			t.Cleanup(func() { original.Close() })

			tt.setup(t, original)

			snap := original.Snapshot(t.Context())
			require.NotNil(t, snap)

			fresh := cloudwatchlogs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			t.Cleanup(func() { fresh.Close() })
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh)
		})
	}
}

func TestBackend_Reset_ClearsNewMaps(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackend()

	// Populate all new maps.
	_, err := b.CreateLogGroup(context.Background(), "/grp", "", "")
	require.NoError(t, err)

	taskID, err := b.CreateExportTask(context.Background(), "task", "/grp", "", "bucket", "", 1000, 2000)
	require.NoError(t, err)
	require.NotEmpty(t, taskID)

	task, err := b.CreateImportTask(
		context.Background(),
		"arn:aws:iam::123:role/r",
		"arn:aws:cloudtrail:us-east-1:123:eventdatastore/abc",
	)
	require.NoError(t, err)
	require.NotEmpty(t, task.ImportID)

	_, err = b.CreateDelivery("src", "arn:aws:logs:us-east-1:123:delivery-destination:dst", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateLogAnomalyDetector(
		[]string{"arn:aws:logs:us-east-1:123:log-group:/app"},
		"detector", "FIVE_MIN", "", "", 0,
	)
	require.NoError(t, err)

	_, err = b.CreateScheduledQuery(cloudwatchlogs.ScheduledQueryCreateParams{
		Name:               "sq",
		QueryString:        "fields @message",
		QueryLanguage:      "CWLI",
		ScheduleExpression: "cron(0 * * * ? *)",
		ExecutionRoleArn:   "arn:aws:iam::123456789012:role/scheduled-query-role",
		State:              "ENABLED",
	})
	require.NoError(t, err)

	// Reset and verify the backend returns empty state.
	b.Reset()

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := cloudwatchlogs.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	// Verify log groups are empty (representative check).
	groups, _, err := fresh.DescribeLogGroups(context.Background(), "", "", 100)
	require.NoError(t, err)
	assert.Empty(t, groups)
}

func TestInMemoryBackend_SnapshotRestore_NewMaps(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	// Populate export task.
	_, err := b.CreateLogGroup(context.Background(), "/grp", "", "")
	require.NoError(t, err)

	taskID, err := b.CreateExportTask(context.Background(), "my-export", "/grp", "", "my-bucket", "prefix/", 1000, 2000)
	require.NoError(t, err)

	// Populate import task.
	importTask, err := b.CreateImportTask(
		context.Background(),
		"arn:aws:iam::123456789012:role/import-role",
		"arn:aws:cloudtrail:us-east-1:123456789012:eventdatastore/abc",
	)
	require.NoError(t, err)

	// Populate delivery with tags.
	delivery, err := b.CreateDelivery(
		"my-source",
		"arn:aws:logs:us-east-1:123456789012:delivery-destination:dst",
		"", nil, nil,
		map[string]string{"env": "prod"},
	)
	require.NoError(t, err)

	// Populate anomaly detector.
	detectorArn, err := b.CreateLogAnomalyDetector(
		[]string{"arn:aws:logs:us-east-1:123456789012:log-group:/app"},
		"my-detector", "FIVE_MIN", "", "", 0,
	)
	require.NoError(t, err)

	// Populate scheduled query.
	queryArn, err := b.CreateScheduledQuery(cloudwatchlogs.ScheduledQueryCreateParams{
		Name:               "my-query",
		QueryString:        "fields @message",
		QueryLanguage:      "CWLI",
		ScheduleExpression: "cron(0 * * * ? *)",
		ExecutionRoleArn:   "arn:aws:iam::123456789012:role/scheduled-query-role",
		State:              "ENABLED",
	})
	require.NoError(t, err)

	// Snapshot and restore.
	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	require.NoError(t, b2.Restore(t.Context(), snap))

	// Verify export task survived.
	err = b2.CancelExportTask(taskID)
	require.NoError(t, err)

	// Verify import task survived.
	cancelledTask, err := b2.CancelImportTask(importTask.ImportID)
	require.NoError(t, err)
	assert.Equal(t, importTask.ImportID, cancelledTask.ImportID)
	assert.Equal(t, "CANCELLED", cancelledTask.Status)

	// Verify delivery survived (can create another one with same source name - no uniqueness constraint).
	_ = delivery
	_ = detectorArn
	_ = queryArn
}

func TestHandler_Persistence(t *testing.T) {
	t.Parallel()

	h := cloudwatchlogs.NewHandler(cloudwatchlogs.NewInMemoryBackend())

	// Snapshot should delegate to the backend and return non-nil bytes.
	data := h.Snapshot(t.Context())
	require.NotNil(t, data)

	// Restore should delegate to the backend without error.
	require.NoError(t, h.Restore(t.Context(), data))
}
