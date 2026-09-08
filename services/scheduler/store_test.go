package scheduler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/scheduler"
)

func TestStorageBackendInterface(t *testing.T) {
	t.Parallel()

	// Verify compile-time assertion: *InMemoryBackend must satisfy StorageBackend.
	var _ scheduler.StorageBackend = scheduler.NewInMemoryBackend("000000000000", "us-east-1")
}

func TestInMemoryBackend_AccountID(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("123456789012", "eu-west-1")

	assert.Equal(t, "123456789012", b.AccountID())
}

func TestInMemoryBackend_Region(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-west-2")
	assert.Equal(t, "us-west-2", b.Region())
}

func TestScheduleCount(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	h := scheduler.NewHandler(b)

	assert.Equal(t, 0, scheduler.ScheduleCount(b))

	createScheduleViaHandler(t, h, "s1", "", "rate(1 minute)")
	assert.Equal(t, 1, scheduler.ScheduleCount(b))
}

func TestScheduleGroupCount(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	// Seeded with "default" group.
	assert.Equal(t, 1, scheduler.ScheduleGroupCount(b))

	_, err := b.CreateScheduleGroup(context.Background(), "prod", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, scheduler.ScheduleGroupCount(b))
}

func TestAddScheduleInternal(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	b.AddScheduleInternal(&scheduler.Schedule{
		Name:               "injected",
		ARN:                "arn:aws:scheduler:us-east-1:000000000000:schedule/default/injected",
		GroupName:          "default",
		ScheduleExpression: "rate(1 hour)",
		State:              "ENABLED",
	})

	assert.Equal(t, 1, scheduler.ScheduleCount(b))

	s, err := b.GetSchedule(context.Background(), "injected", "default")
	require.NoError(t, err)
	assert.Equal(t, "injected", s.Name)
}

func TestAddScheduleGroupInternal(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	b.AddScheduleGroupInternal(&scheduler.ScheduleGroup{
		Name:  "injected-group",
		ARN:   "arn:aws:scheduler:us-east-1:000000000000:schedule-group/injected-group",
		State: "ACTIVE",
	})

	assert.Equal(t, 2, scheduler.ScheduleGroupCount(b))

	g, err := b.GetScheduleGroup(context.Background(), "injected-group")
	require.NoError(t, err)
	assert.Equal(t, "injected-group", g.Name)
}

// TestSchedulerBackend_Reset verifies Reset clears schedules and non-default
// groups, leaving only the built-in "default" group behind.
func TestSchedulerBackend_Reset(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateSchedule(context.Background(), "s1", "",
		"rate(1 minute)",
		"",
		"",
		scheduler.Target{ARN: "arn:a", RoleARN: "arn:r"},
		"ENABLED", scheduler.FlexibleTimeWindow{Mode: "OFF"})
	require.NoError(t, err)

	_, err = b.CreateScheduleGroup(context.Background(), "g1", nil)
	require.NoError(t, err)

	b.Reset()

	schedules, _ := b.ListSchedules(context.Background(), "", "", "", "", 0)
	assert.Empty(t, schedules)
	groups, _ := b.ListScheduleGroups(context.Background(), "", "", 0)
	require.Len(t, groups, 1)
	assert.Equal(t, "default", groups[0].Name)
}

// TestInMemoryBackend_Reset_ViaCountHelpers duplicates the Reset assertions above
// using the ScheduleCount/ScheduleGroupCount test helpers instead of List calls.
func TestInMemoryBackend_Reset_ViaCountHelpers(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateScheduleGroup(context.Background(), "grp", nil)
	require.NoError(t, err)
	_, err = b.CreateSchedule(context.Background(), "s1", "grp", "rate(1 minute)", "", "",
		scheduler.Target{ARN: "arn:a", RoleARN: "arn:r"}, "ENABLED", scheduler.FlexibleTimeWindow{Mode: "OFF"})
	require.NoError(t, err)

	b.Reset()

	assert.Equal(t, 0, scheduler.ScheduleCount(b))
	// Only the default group should remain.
	assert.Equal(t, 1, scheduler.ScheduleGroupCount(b))
}

func TestHandlerReset(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	h := scheduler.NewHandler(b)

	createScheduleViaHandler(t, h, "r1", "", "rate(1 minute)")
	assert.Equal(t, 1, scheduler.ScheduleCount(b))

	h.Reset()
	assert.Equal(t, 0, scheduler.ScheduleCount(b))
}
