package scheduler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/scheduler"
)

func TestInMemoryBackend_SnapshotRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(b *scheduler.InMemoryBackend) string
		verify func(t *testing.T, b *scheduler.InMemoryBackend, id string)
		name   string
	}{
		{
			name: "round_trip_preserves_state",
			setup: func(b *scheduler.InMemoryBackend) string {
				sched, err := b.CreateSchedule(
					context.Background(),
					"test-schedule",
					"",
					"rate(1 minute)",
					"",
					"",
					scheduler.Target{
						ARN:     "arn:aws:lambda:us-east-1:000000000000:function:test",
						RoleARN: "arn:aws:iam::000000000000:role/test",
					},
					"ENABLED",
					scheduler.FlexibleTimeWindow{Mode: "OFF"},
				)
				if err != nil {
					return ""
				}

				return sched.Name
			},
			verify: func(t *testing.T, b *scheduler.InMemoryBackend, id string) {
				t.Helper()

				sched, err := b.GetSchedule(context.Background(), id, "")
				require.NoError(t, err)
				assert.Equal(t, id, sched.Name)
				assert.Equal(t, "rate(1 minute)", sched.ScheduleExpression)
			},
		},
		{
			name: "restore_rebuilds_arn_index",
			setup: func(b *scheduler.InMemoryBackend) string {
				sched, err := b.CreateSchedule(
					context.Background(),
					"idx-schedule",
					"",
					"rate(5 minutes)",
					"",
					"",
					scheduler.Target{
						ARN:     "arn:aws:lambda:us-east-1:000000000000:function:idx",
						RoleARN: "arn:aws:iam::000000000000:role/idx",
					},
					"ENABLED",
					scheduler.FlexibleTimeWindow{Mode: "OFF"},
				)
				if err != nil {
					return ""
				}

				return sched.ARN
			},
			verify: func(t *testing.T, b *scheduler.InMemoryBackend, resourceARN string) {
				t.Helper()

				// TagResource uses the scheduleARNIndex; must succeed after restore.
				err := b.TagResource(context.Background(), resourceARN, map[string]string{"env": "test"})
				require.NoError(t, err)

				kv, err := b.ListTagsForResource(context.Background(), resourceARN)
				require.NoError(t, err)
				assert.Equal(t, "test", kv["env"])
			},
		},
		{
			name:  "empty_backend_round_trip",
			setup: func(_ *scheduler.InMemoryBackend) string { return "" },
			verify: func(t *testing.T, b *scheduler.InMemoryBackend, _ string) {
				t.Helper()

				schedules, _ := b.ListSchedules(context.Background(), "", "", "", "", 0)
				assert.Empty(t, schedules)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
			id := tt.setup(original)

			snap := original.Snapshot(t.Context())
			require.NotNil(t, snap)

			fresh := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh, id)
		})
	}
}

func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

func TestSchedulerHandler_Persistence(t *testing.T) {
	t.Parallel()

	backend := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	h := scheduler.NewHandler(backend)

	_, err := backend.CreateSchedule(
		context.Background(),
		"snap-schedule",
		"",
		"rate(5 minutes)",
		"",
		"",
		scheduler.Target{
			ARN:     "arn:aws:lambda:us-east-1:000000000000:function:test",
			RoleARN: "arn:aws:iam::000000000000:role/test",
		},
		"ENABLED",
		scheduler.FlexibleTimeWindow{Mode: "OFF"},
	)
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	freshH := scheduler.NewHandler(fresh)
	require.NoError(t, freshH.Restore(t.Context(), snap))

	schedules, _ := fresh.ListSchedules(context.Background(), "", "", "", "", 0)
	assert.Len(t, schedules, 1)
}

// TestPersistence_RoundTripWithGroupName verifies a non-default group (with
// tags) and a schedule within it both survive Snapshot/Restore.
func TestPersistence_RoundTripWithGroupName(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateScheduleGroup(context.Background(), "mygrp", map[string]string{"env": "test"})
	require.NoError(t, err)

	_, err = b.CreateSchedule(
		context.Background(),
		"grp-sched", "mygrp", "rate(5 minutes)", "desc", "UTC",
		scheduler.Target{ARN: "arn:a", RoleARN: "arn:r"},
		"ENABLED",
		scheduler.FlexibleTimeWindow{Mode: "OFF"},
	)
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	s, err := fresh.GetSchedule(context.Background(), "grp-sched", "mygrp")
	require.NoError(t, err)
	assert.Equal(t, "mygrp", s.GroupName)
	assert.Equal(t, "UTC", s.ScheduleExpressionTimezone)
	assert.Equal(t, "desc", s.Description)

	g, err := fresh.GetScheduleGroup(context.Background(), "mygrp")
	require.NoError(t, err)
	assert.Equal(t, "mygrp", g.Name)

	// Verify tags were persisted for the group.
	kv, err := fresh.ListTagsForResource(context.Background(), g.ARN)
	require.NoError(t, err)
	assert.Equal(t, "test", kv["env"])
}

// TestPersistence_NewScheduleFields verifies ActionAfterCompletion and KmsKeyArn
// survive Snapshot/Restore.
func TestPersistence_NewScheduleFields(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	kmsARN := "arn:aws:kms:us-east-1:000000000000:key/abc"

	_, err := b.CreateSchedule(
		context.Background(),
		"persist-sched", "", "rate(1 minute)", "desc", "",
		scheduler.Target{ARN: "arn:aws:sqs:us-east-1:0:q", RoleARN: "arn:aws:iam::0:role/r"},
		"ENABLED",
		scheduler.FlexibleTimeWindow{Mode: "OFF"},
		scheduler.WithActionAfterCompletion("DELETE"),
		scheduler.WithKmsKeyArn(kmsARN),
	)
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b2.Restore(t.Context(), snap))

	s, err := b2.GetSchedule(context.Background(), "persist-sched", "default")
	require.NoError(t, err)
	assert.Equal(t, "DELETE", s.ActionAfterCompletion)
	assert.Equal(t, kmsARN, s.KmsKeyArn)
}
