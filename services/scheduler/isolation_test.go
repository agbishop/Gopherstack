package scheduler //nolint:testpackage // needs access to the unexported region context key.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ctxRegion(region string) context.Context {
	return context.WithValue(context.Background(), regionContextKey{}, region)
}

func newSchedTarget() Target {
	return Target{
		ARN:     "arn:aws:lambda:us-east-1:000000000000:function:fn",
		RoleARN: "arn:aws:iam::000000000000:role/r",
	}
}

func newSchedFTW() FlexibleTimeWindow {
	return FlexibleTimeWindow{Mode: "OFF"}
}

func TestSchedulerRegionIsolation(t *testing.T) {
	t.Parallel()

	backend := NewInMemoryBackend("000000000000", "us-east-1")

	ctxEast := ctxRegion("us-east-1")
	ctxWest := ctxRegion("us-west-2")

	// 1. Create a schedule named "sched1" in us-east-1.
	eastSched, err := backend.CreateSchedule(
		ctxEast, "sched1", "", "rate(1 minute)", "", "",
		newSchedTarget(), "ENABLED", newSchedFTW(),
	)
	require.NoError(t, err)
	assert.Contains(t, eastSched.ARN, "us-east-1")
	assert.Equal(t, "us-east-1", eastSched.Region)

	// 2. Create a schedule with the SAME NAME in us-west-2.
	westSched, err := backend.CreateSchedule(
		ctxWest, "sched1", "", "rate(5 minutes)", "", "",
		newSchedTarget(), "ENABLED", newSchedFTW(),
	)
	require.NoError(t, err)
	assert.Contains(t, westSched.ARN, "us-west-2")
	assert.Equal(t, "us-west-2", westSched.Region)

	// 3. us-east-1 sees only its own schedule with its own expression.
	eastList, _ := backend.ListSchedules(ctxEast, "", "", "", "", 0)
	require.Len(t, eastList, 1)
	assert.Equal(t, "sched1", eastList[0].Name)
	assert.Equal(t, "rate(1 minute)", eastList[0].ScheduleExpression)
	assert.Contains(t, eastList[0].ARN, "us-east-1")

	// 4. us-west-2 sees only its own schedule with its own expression.
	westList, _ := backend.ListSchedules(ctxWest, "", "", "", "", 0)
	require.Len(t, westList, 1)
	assert.Equal(t, "sched1", westList[0].Name)
	assert.Equal(t, "rate(5 minutes)", westList[0].ScheduleExpression)
	assert.Contains(t, westList[0].ARN, "us-west-2")

	// 5. Delete in us-east-1; us-west-2 still has its schedule.
	require.NoError(t, backend.DeleteSchedule(ctxEast, "sched1", ""))

	eastAfter, _ := backend.ListSchedules(ctxEast, "", "", "", "", 0)
	assert.Empty(t, eastAfter)

	westAfter, _ := backend.ListSchedules(ctxWest, "", "", "", "", 0)
	assert.Len(t, westAfter, 1)
}

func TestSchedulerScheduleGroupRegionIsolation(t *testing.T) {
	t.Parallel()

	backend := NewInMemoryBackend("000000000000", "us-east-1")

	ctxEast := ctxRegion("us-east-1")
	ctxWest := ctxRegion("us-west-2")

	_, err := backend.CreateScheduleGroup(ctxEast, "grp1", nil)
	require.NoError(t, err)

	_, err = backend.CreateScheduleGroup(ctxWest, "grp1", nil)
	require.NoError(t, err)

	// Each region sees its own "default" group plus "grp1".
	eastGroups, _ := backend.ListScheduleGroups(ctxEast, "", "", 0)
	assert.Len(t, eastGroups, 2)

	westGroups, _ := backend.ListScheduleGroups(ctxWest, "", "", 0)
	assert.Len(t, westGroups, 2)

	// Deleting grp1 in us-east-1 leaves us-west-2's grp1 intact.
	require.NoError(t, backend.DeleteScheduleGroup(ctxEast, "grp1"))

	_, err = backend.GetScheduleGroup(ctxEast, "grp1")
	require.Error(t, err)

	g, err := backend.GetScheduleGroup(ctxWest, "grp1")
	require.NoError(t, err)
	assert.Equal(t, "grp1", g.Name)
	assert.Contains(t, g.ARN, "us-west-2")
}
