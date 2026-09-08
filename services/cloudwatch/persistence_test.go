package cloudwatch_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

func TestInMemoryBackend_SnapshotRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(b *cloudwatch.InMemoryBackend) string
		verify func(t *testing.T, b *cloudwatch.InMemoryBackend, id string)
		name   string
	}{
		{
			name: "round_trip_preserves_state",
			setup: func(b *cloudwatch.InMemoryBackend) string {
				err := b.PutMetricAlarm(&cloudwatch.MetricAlarm{
					AlarmName:          "test-alarm",
					ComparisonOperator: "GreaterThanThreshold",
					MetricName:         "CPUUtilization",
					Namespace:          "AWS/EC2",
					Statistic:          "Average",
				})
				if err != nil {
					return ""
				}

				return "test-alarm"
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend, id string) {
				t.Helper()

				alarms, _, _, err := b.DescribeAlarms([]string{id}, nil, "", "", "", 0, "", "", "")
				require.NoError(t, err)
				require.Len(t, alarms.Data, 1)
				assert.Equal(t, id, alarms.Data[0].AlarmName)
				assert.Equal(t, "CPUUtilization", alarms.Data[0].MetricName)
			},
		},
		{
			name:  "empty_backend_round_trip",
			setup: func(_ *cloudwatch.InMemoryBackend) string { return "" },
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend, _ string) {
				t.Helper()

				alarms, _, _, _ := b.DescribeAlarms(nil, nil, "", "", "", 0, "", "", "")
				assert.Empty(t, alarms.Data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			id := tt.setup(original)

			snap := original.Snapshot(t.Context())
			require.NotNil(t, snap)

			fresh := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh, id)
		})
	}
}

func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

func TestInMemoryBackend_SnapshotRestore_CompositeAndHistory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(t *testing.T, b *cloudwatch.InMemoryBackend)
		verify func(t *testing.T, b *cloudwatch.InMemoryBackend)
		name   string
	}{
		{
			name: "composite_alarm_round_trip",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "child-persist", StateValue: "OK"}),
				)
				require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
					AlarmName: "parent-persist",
					AlarmRule: `ALARM("child-persist")`,
				}))
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()

				_, composites, _, err := b.DescribeAlarms(nil, []string{"CompositeAlarm"}, "", "", "", 0, "", "", "")
				require.NoError(t, err)
				require.Len(t, composites.Data, 1)
				assert.Equal(t, "parent-persist", composites.Data[0].AlarmName)
				assert.Equal(t, `ALARM("child-persist")`, composites.Data[0].AlarmRule)
			},
		},
		{
			name: "alarm_history_round_trip",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "hist-persist", StateValue: "OK"}),
				)
				require.NoError(t, b.SetAlarmState(t.Context(), "hist-persist", "ALARM", "test reason", ""))
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()

				p, err := b.DescribeAlarmHistory("hist-persist", nil, "", "", "", time.Time{}, time.Time{}, 0)
				require.NoError(t, err)
				assert.NotEmpty(t, p.Data)
				assert.Equal(t, "hist-persist", p.Data[0].AlarmName)
			},
		},
		{
			name: "log_alarm_round_trip",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutLogAlarm(&cloudwatch.LogAlarm{
					AlarmName:              "log-persist",
					ComparisonOperator:     "GreaterThanThreshold",
					Threshold:              1,
					QueryResultsToAlarm:    1,
					QueryResultsToEvaluate: 1,
					ScheduledQueryConfiguration: cloudwatch.ScheduledQueryConfiguration{
						AggregationExpression: "count(*)",
						QueryString:           "fields @message",
						ScheduledQueryRoleARN: "arn:aws:iam::000000000000:role/cw-log-alarm",
						ScheduleConfiguration: cloudwatch.ScheduleConfiguration{
							ScheduleExpression: "rate(5 minutes)",
						},
					},
				}))
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()

				_, _, logAlarms, err := b.DescribeAlarms(nil, []string{"LogAlarm"}, "", "", "", 0, "", "", "")
				require.NoError(t, err)
				require.Len(t, logAlarms.Data, 1)
				assert.Equal(t, "log-persist", logAlarms.Data[0].AlarmName)
				assert.Equal(
					t, "fields @message", logAlarms.Data[0].ScheduledQueryConfiguration.QueryString,
				)
			},
		},
		{
			name: "dataset_kms_round_trip",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.AssociateDatasetKmsKey(
					"default",
					"arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
				))
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()

				ds, err := b.GetDataset("default")
				require.NoError(t, err)
				assert.Equal(
					t,
					"arn:aws:kms:us-east-1:000000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					ds.KmsKeyArn,
				)
			},
		},
		{
			name: "otel_enrichment_round_trip",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.StartOTelEnrichment())
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()

				status, err := b.GetOTelEnrichment()
				require.NoError(t, err)
				assert.Equal(t, "Running", status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			tt.setup(t, original)

			snap := original.Snapshot(t.Context())
			require.NotNil(t, snap)

			fresh := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh)
		})
	}
}

func TestHandler_SnapshotRestore(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	h := cloudwatch.NewHandler(b)

	// Put an alarm so there is some state to snapshot.
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:  "snap-alarm",
		MetricName: "CPU",
		Namespace:  "AWS/EC2",
	}))

	// Handler.Snapshot delegates to the backend.
	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	// Create a fresh handler and restore into it.
	b2 := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	h2 := cloudwatch.NewHandler(b2)
	require.NoError(t, h2.Restore(t.Context(), snap))

	alarms, _, _, err := b2.DescribeAlarms([]string{"snap-alarm"}, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, alarms.Data, 1)
	assert.Equal(t, "snap-alarm", alarms.Data[0].AlarmName)
}

func TestInMemoryBackend_SnapshotRestore_NewResourceTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(t *testing.T, b *cloudwatch.InMemoryBackend)
		verify func(t *testing.T, b *cloudwatch.InMemoryBackend)
		name   string
	}{
		{
			name: "anomaly_detector_round_trip",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutAnomalyDetectorInternal(&cloudwatch.AnomalyDetector{
					Namespace:  "AWS/EC2",
					MetricName: "CPUUtilization",
					Stat:       "Average",
				})
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				p, err := b.DescribeAnomalyDetectors("AWS/EC2", "", "", 0)
				require.NoError(t, err)
				require.Len(t, p.Data, 1)
				assert.Equal(t, "CPUUtilization", p.Data[0].MetricName)
				assert.Equal(t, "TRAINED_INSUFFICIENT_DATA", p.Data[0].StateValue)
			},
		},
		{
			name: "insight_rule_round_trip",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "persist-rule", State: "ENABLED"})
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				p, err := b.DescribeInsightRules("", 0)
				require.NoError(t, err)
				require.Len(t, p.Data, 1)
				assert.Equal(t, "persist-rule", p.Data[0].Name)
				assert.Equal(t, "ENABLED", p.Data[0].State)
				assert.NotEmpty(t, p.Data[0].Arn)
				assert.False(t, p.Data[0].CreatedAt.IsZero())
			},
		},
		{
			name: "metric_stream_round_trip",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutMetricStreamInternal(&cloudwatch.MetricStream{
					Name:         "persist-stream",
					FirehoseArn:  "arn:aws:firehose:us-east-1:000000000000:deliverystream/my-stream",
					OutputFormat: "json",
				})
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				// Delete succeeds -> the stream was persisted and restored.
				err := b.DeleteMetricStream("persist-stream")
				require.NoError(t, err)
			},
		},
		{
			name: "alarm_mute_rule_round_trip",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutAlarmMuteRuleInternal(&cloudwatch.AlarmMuteRule{
					Name:        "persist-mute",
					Description: "test mute",
					AlarmNames:  []string{"alarm-a", "alarm-b"},
				})
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				rule, err := b.GetAlarmMuteRule("persist-mute")
				require.NoError(t, err)
				assert.Equal(t, "test mute", rule.Description)
				assert.Equal(t, []string{"alarm-a", "alarm-b"}, rule.AlarmNames)
				assert.False(t, rule.CreationTime.IsZero())
			},
		},
		{
			name: "dashboard_round_trip",
			setup: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				_, err := b.PutDashboard("persist-dash", `{"widgets":[]}`)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				entry, body, err := b.GetDashboard("persist-dash")
				require.NoError(t, err)
				assert.Equal(t, "persist-dash", entry.DashboardName)
				assert.JSONEq(t, `{"widgets":[]}`, body)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			tt.setup(t, original)

			snap := original.Snapshot(t.Context())
			require.NotNil(t, snap)

			fresh := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh)
		})
	}
}

func TestInMemoryBackend_Reset_ClearsNewMaps(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")

	b.PutAnomalyDetectorInternal(&cloudwatch.AnomalyDetector{Namespace: "NS", MetricName: "M", Stat: "Sum"})
	b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "rule-to-clear"})
	b.PutMetricStreamInternal(&cloudwatch.MetricStream{Name: "stream-to-clear"})
	b.PutAlarmMuteRuleInternal(&cloudwatch.AlarmMuteRule{Name: "mute-to-clear"})

	b.Reset()

	p1, err := b.DescribeAnomalyDetectors("", "", "", 0)
	require.NoError(t, err)
	assert.Empty(t, p1.Data, "anomaly detectors should be empty after reset")

	p2, err := b.DescribeInsightRules("", 0)
	require.NoError(t, err)
	assert.Empty(t, p2.Data, "insight rules should be empty after reset")

	err = b.DeleteMetricStream("stream-to-clear")
	require.Error(t, err, "metric stream should not exist after reset")

	_, err = b.GetAlarmMuteRule("mute-to-clear")
	require.Error(t, err, "mute rule should not exist after reset")
}

func TestHandler_Reset_ClearsTags(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	h := cloudwatch.NewHandler(b)

	// Tag a resource.
	const alarmARN = "arn:aws:cloudwatch:us-east-1:000000000000:alarm:tagged-alarm-reset"
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:  "tagged-alarm-reset",
		AlarmArn:   alarmARN,
		MetricName: "CPU",
		Namespace:  "NS",
	}))

	rec := postForm(t, h,
		"Action=TagResource&ResourceARN="+alarmARN+
			"&Tags.member.1.Key=env&Tags.member.1.Value=prod")
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify tag exists.
	rec = postForm(t, h, "Action=ListTagsForResource&ResourceARN="+alarmARN)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "env")

	// Reset.
	h.Reset()

	// Tag should be gone after reset.
	rec = postForm(t, h, "Action=ListTagsForResource&ResourceARN="+alarmARN)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "env")
}

func TestHandler_SnapshotRestore_IncludesTags(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	h := cloudwatch.NewHandler(b)

	const alarmARN = "arn:aws:cloudwatch:us-east-1:000000000000:alarm:tagged-alarm-snap2"
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:  "tagged-alarm-snap2",
		AlarmArn:   alarmARN,
		MetricName: "CPU",
		Namespace:  "NS",
	}))

	rec := postForm(t, h, "Action=TagResource&ResourceARN="+alarmARN+
		"&Tags.member.1.Key=env&Tags.member.1.Value=staging")
	require.Equal(t, http.StatusOK, rec.Code)

	// Snapshot + restore into a fresh handler.
	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	h2 := cloudwatch.NewHandler(b2)
	require.NoError(t, h2.Restore(t.Context(), snap))

	// Tags should have been restored.
	rec = postForm(t, h2, "Action=ListTagsForResource&ResourceARN="+alarmARN)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "staging")
}

// TestInMemoryBackend_SnapshotRestore_FullState is a Phase 3.3 (pkgs/store
// conversion) regression test: it populates every top-level resource
// collection on the backend -- both the store.Table-backed ones (alarms,
// compositeAlarms, dashboards, anomalyDetectors, insightRules, metricStreams,
// alarmMuteRules) and the deliberately-raw ones (metrics via
// PutMetricData, alarmHistory via SetAlarmState) -- in a single backend, then
// verifies a Snapshot->Restore round trip into a fresh backend reproduces
// every one of them.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")

	// metrics (raw, hot-path map).
	require.NoError(t, original.PutMetricData("AWS/EC2", []cloudwatch.MetricDatum{
		{
			MetricName: "CPUUtilization",
			Value:      42,
			Count:      1,
			Sum:        42,
			Min:        42,
			Max:        42,
			Timestamp:  time.Now().UTC(),
		},
	}))

	// alarms + alarmHistory (alarmHistory raw, populated as a side effect of PutMetricAlarm).
	require.NoError(t, original.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName:  "full-state-alarm",
		MetricName: "CPUUtilization",
		Namespace:  "AWS/EC2",
		Statistic:  "Average",
	}))

	// compositeAlarms.
	require.NoError(t, original.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "full-state-composite",
		AlarmRule: `ALARM("full-state-alarm")`,
	}))

	// dashboards.
	_, dashErr := original.PutDashboard("full-state-dash", `{"widgets":[]}`)
	require.NoError(t, dashErr)

	// anomalyDetectors.
	original.PutAnomalyDetectorInternal(&cloudwatch.AnomalyDetector{
		Namespace:  "AWS/EC2",
		MetricName: "CPUUtilization",
		Stat:       "Average",
	})

	// insightRules.
	original.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "full-state-rule", State: "ENABLED"})

	// metricStreams.
	original.PutMetricStreamInternal(&cloudwatch.MetricStream{
		Name:        "full-state-stream",
		FirehoseArn: "arn:aws:firehose:us-east-1:000000000000:deliverystream/full-state",
	})

	// alarmMuteRules.
	original.PutAlarmMuteRuleInternal(&cloudwatch.AlarmMuteRule{Name: "full-state-mute"})

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	// metrics.
	stats, err := fresh.GetMetricStatistics(
		"AWS/EC2", "CPUUtilization", nil,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour),
		60, []string{"Average"}, nil,
	)
	require.NoError(t, err)
	require.NotEmpty(t, stats)

	// alarms + alarmHistory. AlarmTypes must list both explicitly: DescribeAlarms
	// defaults to metric alarms only when AlarmTypes is omitted (bd
	// gopherstack-yvb7), and this check wants both metric and composite alarms
	// back to verify both survived the snapshot/restore round trip.
	alarms, composites, _, err := fresh.DescribeAlarms(
		nil, []string{"MetricAlarm", "CompositeAlarm"}, "", "", "", 0,
		"", "", "")
	require.NoError(t, err)
	require.Len(t, alarms.Data, 1)
	assert.Equal(t, "full-state-alarm", alarms.Data[0].AlarmName)

	hist, err := fresh.DescribeAlarmHistory("full-state-alarm", nil, "", "", "", time.Time{}, time.Time{}, 0)
	require.NoError(t, err)
	assert.NotEmpty(t, hist.Data)

	// compositeAlarms.
	require.Len(t, composites.Data, 1)
	assert.Equal(t, "full-state-composite", composites.Data[0].AlarmName)

	// dashboards.
	entry, body, err := fresh.GetDashboard("full-state-dash")
	require.NoError(t, err)
	assert.Equal(t, "full-state-dash", entry.DashboardName)
	assert.JSONEq(t, `{"widgets":[]}`, body)

	// anomalyDetectors.
	detectors, err := fresh.DescribeAnomalyDetectors("AWS/EC2", "CPUUtilization", "", 0)
	require.NoError(t, err)
	require.Len(t, detectors.Data, 1)

	// insightRules.
	rules, err := fresh.DescribeInsightRules("", 0)
	require.NoError(t, err)
	require.Len(t, rules.Data, 1)
	assert.Equal(t, "full-state-rule", rules.Data[0].Name)

	// metricStreams.
	stream, err := fresh.GetMetricStream("full-state-stream")
	require.NoError(t, err)
	assert.Equal(t, "full-state-stream", stream.Name)

	// alarmMuteRules.
	_, err = fresh.GetAlarmMuteRule("full-state-mute")
	require.NoError(t, err)
}

// TestInMemoryBackend_Restore_VersionGuard exercises the Phase 3.3 snapshot
// version guard: a snapshot whose "version" field does not match the
// backend's current snapshot version must be discarded cleanly (reset to
// empty, no error) rather than partially decoded, since the Tables blob shape
// is not guaranteed compatible across versions.
func TestInMemoryBackend_Restore_VersionGuard(t *testing.T) {
	t.Parallel()

	original := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	require.NoError(t, original.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "version-guard-alarm"}))

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	// Corrupt the version field to simulate an incompatible (older/newer) snapshot.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(snap, &raw))
	raw["version"] = 999999
	corrupted, err := json.Marshal(raw)
	require.NoError(t, err)

	// Restore into a backend that already has unrelated state, to confirm the
	// version mismatch path resets it rather than merging/erroring.
	target := cloudwatch.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	require.NoError(t, target.PutMetricAlarm(&cloudwatch.MetricAlarm{AlarmName: "pre-existing-alarm"}))

	require.NoError(t, target.Restore(t.Context(), corrupted))

	alarms, _, _, err := target.DescribeAlarms(nil, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	assert.Empty(t, alarms.Data, "version-mismatched snapshot should reset the backend to empty, not merge or error")
}
