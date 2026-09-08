package applicationautoscaling_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/applicationautoscaling"
)

func Test_PersistenceSnapshotRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(t *testing.T, b *applicationautoscaling.InMemoryBackend)
		verify func(t *testing.T, b *applicationautoscaling.InMemoryBackend)
		name   string
	}{
		{
			name:  "empty",
			setup: func(*testing.T, *applicationautoscaling.InMemoryBackend) {},
			verify: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				targets, _, _ := b.DescribeScalableTargets(applicationautoscaling.DescribeScalableTargetsFilter{})
				assert.Empty(t, targets)
			},
		},
		{
			name: "scalable_target_preserved_with_arn_index",
			setup: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				_, err := b.RegisterScalableTarget(
					"ecs",
					"service/my-cluster/my-svc",
					"ecs:service:DesiredCount",
					int32p(1),
					int32p(10),
					nil,
					"",
					nil,
				)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				targets, _, _ := b.DescribeScalableTargets(
					applicationautoscaling.DescribeScalableTargetsFilter{ServiceNamespace: "ecs"},
				)
				require.Len(t, targets, 1)

				// Verify the targetsByARN index is rebuilt -- tag ops should work.
				err := b.TagResource(targets[0].ARN, map[string]string{"team": "platform"})
				require.NoError(t, err)
				gotTags, err := b.ListTagsForResource(targets[0].ARN)
				require.NoError(t, err)
				assert.Equal(t, "platform", gotTags["team"])
			},
		},
		{
			name: "scaling_policy_preserved_with_name_index",
			setup: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				_, err := b.RegisterScalableTarget(
					"ecs", "service/default/svc", "ecs:service:DesiredCount", int32p(1), int32p(10), nil, "", nil,
				)
				require.NoError(t, err)

				_, err = b.PutScalingPolicy(
					"ecs", "service/default/svc", "ecs:service:DesiredCount",
					"my-policy", "TargetTrackingScaling",
					map[string]any{"TargetValue": 50.0}, nil, nil,
				)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				policies, _, _ := b.DescribeScalingPolicies(applicationautoscaling.DescribeScalingPoliciesFilter{
					ServiceNamespace: "ecs",
				})
				require.Len(t, policies, 1)
				assert.Equal(t, "my-policy", policies[0].PolicyName)

				// Verify the policiesByName index is rebuilt -- an update-by-name
				// PutScalingPolicy call must hit the existing policy, not create a
				// second one.
				_, err := b.PutScalingPolicy(
					"ecs", "service/default/svc", "ecs:service:DesiredCount",
					"my-policy", "",
					map[string]any{"TargetValue": 75.0}, nil, nil,
				)
				require.NoError(t, err)

				policies, _, _ = b.DescribeScalingPolicies(applicationautoscaling.DescribeScalingPoliciesFilter{
					ServiceNamespace: "ecs",
				})
				require.Len(t, policies, 1)
				assert.InEpsilon(t, 75.0, policies[0].TargetTrackingConfig["TargetValue"], 0)

				// Verify the policiesByName index also supports delete-by-name.
				err = b.DeleteScalingPolicy("ecs", "service/default/svc", "ecs:service:DesiredCount", "my-policy")
				require.NoError(t, err)

				policies, _, _ = b.DescribeScalingPolicies(applicationautoscaling.DescribeScalingPoliciesFilter{
					ServiceNamespace: "ecs",
				})
				assert.Empty(t, policies)
			},
		},
		{
			name: "scheduled_action_preserved_with_name_index",
			setup: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				_, err := b.RegisterScalableTarget(
					"ecs", "service/default/svc", "ecs:service:DesiredCount", int32p(1), int32p(10), nil, "", nil,
				)
				require.NoError(t, err)

				_, err = b.PutScheduledAction(
					"ecs", "service/default/svc", "ecs:service:DesiredCount",
					"my-action", "at(2030-01-01T00:00:00)", "",
					nil, nil, nil,
				)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				actions, _, _ := b.DescribeScheduledActions(applicationautoscaling.DescribeScheduledActionsFilter{
					ServiceNamespace: "ecs",
				})
				require.Len(t, actions, 1)
				assert.Equal(t, "my-action", actions[0].ScheduledActionName)

				// Verify the actionsByName index is rebuilt -- an update-by-name
				// PutScheduledAction call must hit the existing action, not create a
				// second one.
				_, err := b.PutScheduledAction(
					"ecs", "service/default/svc", "ecs:service:DesiredCount",
					"my-action", "at(2031-01-01T00:00:00)", "",
					nil, nil, nil,
				)
				require.NoError(t, err)

				actions, _, _ = b.DescribeScheduledActions(applicationautoscaling.DescribeScheduledActionsFilter{
					ServiceNamespace: "ecs",
				})
				require.Len(t, actions, 1)
				assert.Equal(t, "at(2031-01-01T00:00:00)", actions[0].Schedule)

				// Verify the actionsByName index also supports delete-by-name.
				err = b.DeleteScheduledAction("ecs", "service/default/svc", "ecs:service:DesiredCount", "my-action")
				require.NoError(t, err)

				actions, _, _ = b.DescribeScheduledActions(applicationautoscaling.DescribeScheduledActionsFilter{
					ServiceNamespace: "ecs",
				})
				assert.Empty(t, actions)
			},
		},
		{
			name: "full_state_cascade_delete_after_restore",
			setup: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				_, err := b.RegisterScalableTarget(
					"ecs", "service/default/svc", "ecs:service:DesiredCount", int32p(1), int32p(10), nil, "", nil,
				)
				require.NoError(t, err)

				_, err = b.PutScalingPolicy(
					"ecs", "service/default/svc", "ecs:service:DesiredCount",
					"cascade-policy", "TargetTrackingScaling",
					map[string]any{"TargetValue": 50.0}, nil, nil,
				)
				require.NoError(t, err)

				_, err = b.PutScheduledAction(
					"ecs", "service/default/svc", "ecs:service:DesiredCount",
					"cascade-action", "at(2030-01-01T00:00:00)", "",
					nil, nil, nil,
				)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				// Restored state must still support the deregister cascade -- i.e.
				// the scaling policy and scheduled action tables were restored
				// consistently with the scalable target table.
				err := b.DeregisterScalableTarget("ecs", "service/default/svc", "ecs:service:DesiredCount")
				require.NoError(t, err)

				targets, _, _ := b.DescribeScalableTargets(applicationautoscaling.DescribeScalableTargetsFilter{
					ServiceNamespace: "ecs",
				})
				assert.Empty(t, targets)

				policies, _, _ := b.DescribeScalingPolicies(applicationautoscaling.DescribeScalingPoliciesFilter{
					ServiceNamespace: "ecs",
				})
				assert.Empty(t, policies)

				actions, _, _ := b.DescribeScheduledActions(applicationautoscaling.DescribeScheduledActionsFilter{
					ServiceNamespace: "ecs",
				})
				assert.Empty(t, actions)
			},
		},
		{
			name: "scaling_activities_are_ephemeral_not_restored",
			setup: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				_, err := b.RegisterScalableTarget(
					"ecs", "service/default/svc", "ecs:service:DesiredCount", int32p(1), int32p(10), nil, "", nil,
				)
				require.NoError(t, err)
			},
			verify: func(t *testing.T, b *applicationautoscaling.InMemoryBackend) {
				t.Helper()

				// scalingActivities was never part of the pre-Phase-3.3 snapshot
				// either, so it must remain empty across a Restore.
				activities, _, _ := b.DescribeScalingActivities(
					applicationautoscaling.DescribeScalingActivitiesFilter{ServiceNamespace: "ecs"},
				)
				assert.Empty(t, activities)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
			tt.setup(t, b)

			snap := b.Snapshot(t.Context())
			require.NotNil(t, snap)

			b2 := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
			err := b2.Restore(t.Context(), snap)
			require.NoError(t, err)

			tt.verify(t, b2)
		})
	}
}

// Test_PersistenceRestoreInvalidData verifies that malformed snapshot JSON is
// rejected with an error via persistence.UnmarshalSnapshot, rather than
// silently discarded like a version mismatch.
func Test_PersistenceRestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// Test_PersistenceRestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend (including the pre-Phase-3.3
// format, which has no version field at all and so decodes with Version ==
// 0) is discarded cleanly rather than partially decoded: the backend resets
// to empty state and Restore returns no error.
func Test_PersistenceRestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.RegisterScalableTarget(
		"ecs", "service/default/svc", "ecs:service:DesiredCount", int32p(1), int32p(10), nil, "", nil,
	)
	require.NoError(t, err)

	// A syntactically valid but version-less/mismatched snapshot.
	err = b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	targets, _, _ := b.DescribeScalableTargets(applicationautoscaling.DescribeScalableTargetsFilter{})
	assert.Empty(t, targets)

	activities, _, _ := b.DescribeScalingActivities(applicationautoscaling.DescribeScalingActivitiesFilter{})
	assert.Empty(t, activities)
}

// Test_PersistenceOldPreVersionSnapshotDiscarded verifies that a snapshot
// produced by the pre-Phase-3.3 shape (flat top-level scalableTargets/
// scalingPolicies/scheduledActions maps, no version, no tables envelope) is
// discarded rather than misinterpreted.
func Test_PersistenceOldPreVersionSnapshotDiscarded(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")

	oldFormat := `{
		"scalableTargets": {"ecs/service/default/svc/ecs:service:DesiredCount": {"resourceId":"service/default/svc"}},
		"scalingPolicies": {},
		"scheduledActions": {},
		"accountID": "123456789012",
		"region": "us-east-1"
	}`

	err := b.Restore(t.Context(), []byte(oldFormat))
	require.NoError(t, err)

	targets, _, _ := b.DescribeScalableTargets(applicationautoscaling.DescribeScalableTargetsFilter{})
	assert.Empty(t, targets)
}

// Test_PersistenceHandlerDelegates verifies Handler.Snapshot/Handler.Restore
// delegate to the backend end to end.
func Test_PersistenceHandlerDelegates(t *testing.T) {
	t.Parallel()

	h := applicationautoscaling.NewHandler(
		applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1"),
	)

	_, err := h.Backend.RegisterScalableTarget(
		"dynamodb", "table/my-table", "dynamodb:table:ReadCapacityUnits", int32p(1), int32p(10), nil, "", nil,
	)
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := applicationautoscaling.NewHandler(
		applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1"),
	)
	require.NoError(t, h2.Restore(t.Context(), snap))

	targets, _, _ := h2.Backend.DescribeScalableTargets(applicationautoscaling.DescribeScalableTargetsFilter{
		ServiceNamespace: "dynamodb",
	})
	require.Len(t, targets, 1)
	assert.Equal(t, "table/my-table", targets[0].ResourceID)
}

func TestPersistence_SnapshotRestore(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(10),
	})

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := newTestHandler(t)
	require.NoError(t, h2.Restore(t.Context(), snap))

	rec := doRequest(t, h2, "DescribeScalableTargets", map[string]any{"ServiceNamespace": "ecs"})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	targets := resp["ScalableTargets"].([]any)
	assert.Len(t, targets, 1)
}

func TestHandler_PersistenceRebuildsSecondaryIndexes(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(10),
	})
	doRequest(t, h, "PutScalingPolicy", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"PolicyName":        "my-policy",
		"PolicyType":        "TargetTrackingScaling",
	})
	doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "my-action",
		"Schedule":            "rate(1 hour)",
	})

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := newTestHandler(t)
	require.NoError(t, h2.Restore(t.Context(), snap))

	// Delete should work via secondary index (O(1) lookup)
	rec := doRequest(t, h2, "DeleteScalingPolicy", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"PolicyName":        "my-policy",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec2 := doRequest(t, h2, "DeleteScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/my-svc",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "my-action",
	})
	assert.Equal(t, http.StatusOK, rec2.Code)
}
