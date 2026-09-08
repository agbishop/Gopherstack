package iot_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iot"
)

// searchThingGroupNames returns the ThingGroupNames SearchIndex reports for
// thingName -- the observable surface backed by the thingThingGroups reverse
// index (indexing.go's searchThingsIndex).
func searchThingGroupNames(t *testing.T, b *iot.InMemoryBackend, thingName string) []string {
	t.Helper()

	out, err := b.SearchIndex(&iot.SearchIndexInput{QueryString: "thingName:" + thingName})
	require.NoError(t, err)
	require.Len(t, out.Things, 1)

	return out.Things[0].ThingGroupNames
}

func setupThingGroupWithMember(t *testing.T, b *iot.InMemoryBackend, groupName string) {
	t.Helper()

	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: groupName})
	require.NoError(t, err)
	require.NoError(t, b.AddThingToThingGroup(&iot.AddThingToThingGroupInput{
		ThingGroupName: groupName,
		ThingName:      "t1",
	}))
}

// TestRemoveThingFromThingGroup_UpdatesReverseIndex covers gopherstack-6pt8:
// RemoveThingFromThingGroup must clear the removed thing's entry from
// thingThingGroups, not just the group's own member list.
func TestRemoveThingFromThingGroup_UpdatesReverseIndex(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()

	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "t1"})
	require.NoError(t, err)
	setupThingGroupWithMember(t, b, "g1")

	require.Equal(t, []string{"g1"}, searchThingGroupNames(t, b, "t1"))

	require.NoError(t, b.RemoveThingFromThingGroup(&iot.RemoveThingFromThingGroupInput{
		ThingGroupName: "g1",
		ThingName:      "t1",
	}))

	assert.Empty(t, searchThingGroupNames(t, b, "t1"))

	members, err := b.ListThingsInThingGroup(&iot.ListThingsInThingGroupInput{ThingGroupName: "g1"})
	require.NoError(t, err)
	assert.Empty(t, members)
}

// TestRemoveThingFromThingGroup_LeavesOtherGroupsIntact is the negative case:
// removing a thing from one group must not disturb its membership in others.
func TestRemoveThingFromThingGroup_LeavesOtherGroupsIntact(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()

	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "t1"})
	require.NoError(t, err)
	setupThingGroupWithMember(t, b, "g1")
	setupThingGroupWithMember(t, b, "g2")

	require.NoError(t, b.RemoveThingFromThingGroup(&iot.RemoveThingFromThingGroupInput{
		ThingGroupName: "g1",
		ThingName:      "t1",
	}))

	assert.Equal(t, []string{"g2"}, searchThingGroupNames(t, b, "t1"))
}

// TestDeleteThingGroup_ClearsReverseIndexForSurvivingMembers covers
// gopherstack-6pt8: deleting a thing group must remove it from
// thingThingGroups for every former member, not just drop the group's own
// member list. The group itself is gone either way; the assertion is on the
// surviving member's index.
func TestDeleteThingGroup_ClearsReverseIndexForSurvivingMembers(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()

	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "t1"})
	require.NoError(t, err)
	_, err = b.CreateThing(&iot.CreateThingInput{ThingName: "t2"})
	require.NoError(t, err)

	setupThingGroupWithMember(t, b, "g1")
	require.NoError(t, b.AddThingToThingGroup(&iot.AddThingToThingGroupInput{
		ThingGroupName: "g1",
		ThingName:      "t2",
	}))
	setupThingGroupWithMember(t, b, "g2")

	require.Equal(t, []string{"g1", "g2"}, searchThingGroupNames(t, b, "t1"))

	require.NoError(t, b.DeleteThingGroup("g1", 0))

	assert.Equal(t, []string{"g2"}, searchThingGroupNames(t, b, "t1"), "t1 must keep g2 but lose deleted g1")
	assert.Empty(t, searchThingGroupNames(t, b, "t2"), "t2 had only g1, which is now deleted")
}

// TestDeleteDynamicThingGroup_ClearsReverseIndexForSurvivingMembers is the
// same fix applied to CreateDynamicThingGroup/DeleteDynamicThingGroup, which
// share thingGroups/thingGroupMembers with the static path.
func TestDeleteDynamicThingGroup_ClearsReverseIndexForSurvivingMembers(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()

	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "t1"})
	require.NoError(t, err)

	_, err = b.CreateDynamicThingGroup(&iot.CreateThingGroupInput{
		ThingGroupName: "dg1",
		QueryString:    "connectivity.connected = true",
	})
	require.NoError(t, err)
	require.NoError(t, b.AddThingToThingGroup(&iot.AddThingToThingGroupInput{
		ThingGroupName: "dg1",
		ThingName:      "t1",
	}))

	require.Equal(t, []string{"dg1"}, searchThingGroupNames(t, b, "t1"))

	require.NoError(t, b.DeleteDynamicThingGroup("dg1", 0))

	assert.Empty(t, searchThingGroupNames(t, b, "t1"))
}

// TestDeleteBillingGroup_ClearsThingBillingGroupsForSurvivingMembers covers
// the billing-group half of gopherstack-6pt8: deleting a billing group must
// clear thingBillingGroups for every thing still pointing at it, so
// DescribeThing.BillingGroupName stops naming a group that no longer exists.
func TestDeleteBillingGroup_ClearsThingBillingGroupsForSurvivingMembers(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()

	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "t1"})
	require.NoError(t, err)
	_, err = b.CreateThing(&iot.CreateThingInput{ThingName: "t2"})
	require.NoError(t, err)

	_, err = b.CreateBillingGroup(&iot.CreateBillingGroupInput{BillingGroupName: "bg1"})
	require.NoError(t, err)
	_, err = b.CreateBillingGroup(&iot.CreateBillingGroupInput{BillingGroupName: "bg2"})
	require.NoError(t, err)

	require.NoError(t, b.AddThingToBillingGroup(&iot.AddThingToBillingGroupInput{
		ThingName:        "t1",
		BillingGroupName: "bg1",
	}))
	require.NoError(t, b.AddThingToBillingGroup(&iot.AddThingToBillingGroupInput{
		ThingName:        "t2",
		BillingGroupName: "bg2",
	}))

	require.NoError(t, b.DeleteBillingGroup("bg1", 0))

	d1, err := b.DescribeThing("t1")
	require.NoError(t, err)
	assert.Empty(t, d1.BillingGroupName, "t1's billing group was deleted and must not be reported as a ghost")

	d2, err := b.DescribeThing("t2")
	require.NoError(t, err)
	assert.Equal(t, "bg2", d2.BillingGroupName, "t2's own billing group must be untouched")
}
