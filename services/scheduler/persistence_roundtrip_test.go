package scheduler //nolint:testpackage // needs the unexported region context key and snapshot internals.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScheduler_SnapshotRestore_FullState exercises a full-state Snapshot -> Restore
// round trip over both store.Table collections (schedules + scheduleGroups),
// covering multiple regions, tags, and the optional StartDate/EndDate pointer
// fields, then asserts every field and per-region isolation survives intact.
func TestScheduler_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	ctxEast := ctxRegion("us-east-1")
	ctxWest := ctxRegion("us-west-2")

	orig := NewInMemoryBackend("000000000000", "us-east-1")

	// A schedule group with tags in the default region.
	_, err := orig.CreateScheduleGroup(ctxEast, "team-a", map[string]string{"owner": "a"})
	require.NoError(t, err)

	// A schedule in the custom group with start/end dates and tags.
	start := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	end := time.Date(2031, 6, 7, 8, 9, 10, 0, time.UTC)
	eastSched, err := orig.CreateSchedule(
		ctxEast, "sched-east", "team-a", "cron(0 12 * * ? *)", "desc", "UTC",
		newSchedTarget(), scheduleStateEnabled, newSchedFTW(),
		WithStartDate(start), WithEndDate(end),
	)
	require.NoError(t, err)
	require.NoError(t, orig.TagResource(ctxEast, eastSched.ARN, map[string]string{"env": "prod"}))

	// A same-named schedule in a different region (isolation check).
	_, err = orig.CreateSchedule(
		ctxWest, "sched-east", "", "rate(5 minutes)", "", "",
		newSchedTarget(), scheduleStateDisabled, newSchedFTW(),
	)
	require.NoError(t, err)

	snap := orig.Snapshot(context.Background())
	require.NotNil(t, snap)

	fresh := NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(context.Background(), snap))

	// Counts across all regions: 2 schedules; groups = default(east)+team-a+default(west) = 3.
	assert.Equal(t, 2, fresh.schedules.Len())
	assert.Equal(t, 3, fresh.scheduleGroups.Len())

	// East schedule fully preserved, including group, dates, expression, and tags.
	got, err := fresh.GetSchedule(ctxEast, "sched-east", "team-a")
	require.NoError(t, err)
	assert.Equal(t, "cron(0 12 * * ? *)", got.ScheduleExpression)
	assert.Equal(t, "team-a", got.GroupName)
	assert.Equal(t, scheduleStateEnabled, got.State)
	require.NotNil(t, got.StartDate)
	require.NotNil(t, got.EndDate)
	assert.True(t, got.StartDate.Equal(start))
	assert.True(t, got.EndDate.Equal(end))

	kv, err := fresh.ListTagsForResource(ctxEast, eastSched.ARN)
	require.NoError(t, err)
	assert.Equal(t, "prod", kv["env"])

	// Group state preserved.
	groupTags, err := fresh.GetScheduleGroup(ctxEast, "team-a")
	require.NoError(t, err)
	assert.Equal(t, scheduleGroupStateActive, groupTags.State)

	// West region isolated: its own same-named schedule with a different expression.
	west, err := fresh.GetSchedule(ctxWest, "sched-east", "")
	require.NoError(t, err)
	assert.Equal(t, "rate(5 minutes)", west.ScheduleExpression)
	assert.Equal(t, scheduleStateDisabled, west.State)

	// East default group untouched and never deleted.
	_, err = fresh.GetScheduleGroup(ctxEast, defaultGroupName)
	require.NoError(t, err)
}

// TestScheduler_Restore_VersionMismatch verifies that a snapshot whose version
// does not match schedulerSnapshotVersion is discarded (registry-style reset +
// re-seed of the default group) rather than partially decoded.
func TestScheduler_Restore_VersionMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{
			name: "future_version",
			data: `{"version":9999,"tables":{},"accountID":"000000000000","region":"us-east-1"}`,
		},
		{
			// No version field decodes as 0, which is also a mismatch.
			name: "absent_version",
			data: `{"tables":{},"accountID":"000000000000","region":"us-east-1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewInMemoryBackend("000000000000", "us-east-1")

			// Seed some state that must be wiped on a version-mismatch restore.
			_, err := b.CreateSchedule(
				ctxRegion("us-east-1"), "doomed", "", "rate(1 minute)", "", "",
				newSchedTarget(), scheduleStateEnabled, newSchedFTW(),
			)
			require.NoError(t, err)

			require.NoError(t, b.Restore(context.Background(), []byte(tt.data)))

			// Schedules wiped; only the re-seeded default group remains.
			assert.Equal(t, 0, b.schedules.Len())
			assert.Equal(t, 1, b.scheduleGroups.Len())

			_, err = b.GetScheduleGroup(ctxRegion("us-east-1"), defaultGroupName)
			require.NoError(t, err)
		})
	}
}
