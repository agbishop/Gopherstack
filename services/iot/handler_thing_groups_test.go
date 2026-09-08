package iot_test

import (
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	iotsdk "github.com/aws/aws-sdk-go-v2/service/iot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iot"
)

func TestThingGroups(t *testing.T) {
	t.Parallel()

	h := newIoTHandler(t)

	// CreateThingGroup
	rec := doIoTRequest(t, h, http.MethodPost, "/thing-groups/my-group", map[string]any{
		"thingGroupProperties": map[string]any{
			"thingGroupDescription": "test group",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// ListThingGroups
	rec = doIoTRequest(t, h, http.MethodGet, "/thing-groups", nil)
	assert.True(t, rec.Code >= 200 && rec.Code < 300)

	// CreateThing and add to group
	doIoTRequest(t, h, http.MethodPost, "/things/my-thing", nil)
	rec = doIoTRequest(t, h, http.MethodPut, "/thing-groups/addThingToThingGroup", map[string]any{
		"thingName":      "my-thing",
		"thingGroupName": "my-group",
	})
	assert.True(t, rec.Code >= 200 && rec.Code < 300)

	// ListThingsInThingGroup
	rec = doIoTRequest(t, h, http.MethodGet, "/thing-groups/my-group/things", nil)
	assert.True(t, rec.Code >= 200 && rec.Code < 300)

	// UpdateThingGroup
	rec = doIoTRequest(t, h, http.MethodPatch, "/thing-groups/my-group", map[string]any{
		"thingGroupProperties": map[string]any{
			"thingGroupDescription": "updated",
		},
	})
	assert.True(t, rec.Code >= 200 && rec.Code < 300)

	// RemoveThingFromThingGroup
	rec = doIoTRequest(t, h, http.MethodPut, "/thing-groups/removeThingFromThingGroup", map[string]any{
		"thingName":      "my-thing",
		"thingGroupName": "my-group",
	})
	assert.True(t, rec.Code >= 200 && rec.Code < 300)

	// DeleteThingGroup
	rec = doIoTRequest(t, h, http.MethodDelete, "/thing-groups/my-group", nil)
	assert.True(t, rec.Code >= 200 && rec.Code < 300)
}

// TestBatch2_ThingGroupAndBillingOps tests thing-group/billing helpers.
func TestThingGroupAndBillingOps(t *testing.T) {
	t.Parallel()
	b := iot.NewInMemoryBackend()
	h := iot.NewHandler(b, nil)

	// Seed: thing, group, billing group
	_, _ = b.CreateThing(&iot.CreateThingInput{ThingName: "my-thing"})
	_, _ = b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "my-group"})
	_ = b.AddThingToThingGroup(&iot.AddThingToThingGroupInput{
		ThingName:      "my-thing",
		ThingGroupName: "my-group",
	})
	_ = b.AddThingToBillingGroup(&iot.AddThingToBillingGroupInput{
		ThingName:        "my-thing",
		BillingGroupName: "my-billing-group",
	})

	// ListThingGroupsForThing
	out := iotOK(t, h, http.MethodGet, "/things/my-thing/thing-groups", nil)
	groups, _ := out["thingGroups"].([]any)
	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}

	// ListThingsInBillingGroup
	out2 := iotOK(t, h, http.MethodGet, "/billing-groups/my-billing-group/things", nil)
	things, _ := out2["things"].([]any)
	if len(things) != 1 {
		t.Errorf("expected 1 thing in billing group, got %d", len(things))
	}

	// RemoveThingFromBillingGroup
	iotOK(t, h, http.MethodPut, "/billing-groups/removeThingFromBillingGroup", map[string]any{
		"thingName":        "my-thing",
		"billingGroupName": "my-billing-group",
	})

	out3 := iotOK(t, h, http.MethodGet, "/billing-groups/my-billing-group/things", nil)
	things3, _ := out3["things"].([]any)
	if len(things3) != 0 {
		t.Errorf("expected 0 things after removal, got %d", len(things3))
	}
}

// TestBatch3_DynamicThingGroup tests CreateDynamicThingGroup, UpdateDynamicThingGroup, DeleteDynamicThingGroup.
func TestDynamicThingGroup(t *testing.T) {
	t.Parallel()
	h, _ := newHandlerForBatch3Test(t)

	// Create
	out := iotOK(t, h, http.MethodPost, "/dynamic-thing-groups/my-dynamic-group", map[string]any{
		"queryString": "thingName:*",
		"thingGroupProperties": map[string]any{
			"thingGroupDescription": "test dynamic group",
		},
	})
	if out["thingGroupName"] != "my-dynamic-group" {
		t.Errorf("expected thingGroupName=my-dynamic-group, got %v", out)
	}

	// Update
	out2 := iotOK(t, h, http.MethodPatch, "/dynamic-thing-groups/my-dynamic-group", map[string]any{
		"queryString": "thingName:device*",
	})
	if out2["version"] == nil {
		t.Errorf("expected version in response, got %v", out2)
	}

	// Delete
	iotOK(t, h, http.MethodDelete, "/dynamic-thing-groups/my-dynamic-group", nil)
}

// DeleteDynamicThingGroupInput.ExpectedVersion (iot@v1.77.4
// api_op_DeleteDynamicThingGroup.go:38-39), expectedVersion is a QUERY
// parameter (awsRestjson1_serializeOpHttpBindingsDeleteDynamicThingGroupInput),
// and its deserializeOpError switch declares a VersionConflictException case.
func TestDeleteDynamicThingGroup_VersionConflict_Rejected(t *testing.T) {
	t.Parallel()
	h, _ := newHandlerForBatch3Test(t)

	iotOK(t, h, http.MethodPost, "/dynamic-thing-groups/del-ver-dyn-group", map[string]any{
		"queryString": "thingName:*",
	})

	rec := iotRequest(t, h, http.MethodDelete, "/dynamic-thing-groups/del-ver-dyn-group?expectedVersion=99", nil)
	require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())

	iotOK(t, h, http.MethodGet, "/thing-groups/del-ver-dyn-group", nil)
}

// TestDynamicThingGroup_RealWireShape guards CreateDynamicThingGroupInput/
// UpdateDynamicThingGroupInput's real body shape (iot@v1.77.4
// api_op_CreateDynamicThingGroup.go/api_op_UpdateDynamicThingGroup.go):
// description lives nested under thingGroupProperties.thingGroupDescription
// (a top-level "description" field, as this handler previously read, does
// not exist on the wire and is silently ignored by a real client's
// deserializer) -- description was always dropped. Also guards indexName/
// queryVersion (previously entirely unmodeled) and UpdateDynamicThingGroup's
// real expectedVersion optimistic-lock semantics, previously entirely
// unimplemented despite the sibling static UpdateThingGroup already
// enforcing it via the same UpdateThingGroupInput struct.
func TestDynamicThingGroup_RealWireShape(t *testing.T) {
	t.Parallel()
	h, _ := newHandlerForBatch3Test(t)

	out := iotOK(t, h, http.MethodPost, "/dynamic-thing-groups/wire-shape-group", map[string]any{
		"queryString":  "thingName:*",
		"indexName":    "AWS_Things",
		"queryVersion": "2017-09-30",
		"thingGroupProperties": map[string]any{
			"thingGroupDescription": "nested description",
		},
	})
	assert.Equal(t, "AWS_Things", out["indexName"])
	assert.Equal(t, "2017-09-30", out["queryVersion"])

	descOut := iotOK(t, h, http.MethodGet, "/thing-groups/wire-shape-group", nil)
	props, _ := descOut["thingGroupProperties"].(map[string]any)
	assert.Equal(t, "nested description", props["thingGroupDescription"])

	t.Run("expected_version_conflict", func(t *testing.T) {
		t.Parallel()

		rec := iotRequest(t, h, http.MethodPatch, "/dynamic-thing-groups/wire-shape-group", map[string]any{
			"queryString":     "thingName:device*",
			"expectedVersion": 99,
		})
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}

func TestBackend_UpdateThingGroupsForThing(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "thing-1"})
	require.NoError(t, err)
	_, err = b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "group-a"})
	require.NoError(t, err)
	_, err = b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "group-b"})
	require.NoError(t, err)

	err = b.UpdateThingGroupsForThing(&iot.UpdateThingGroupsForThingInput{
		ThingName:        "thing-1",
		ThingGroupsToAdd: []string{"group-a", "group-b"},
	})
	require.NoError(t, err)

	members, err := b.ListThingsInThingGroup(&iot.ListThingsInThingGroupInput{ThingGroupName: "group-a"})
	require.NoError(t, err)
	assert.Contains(t, members, "thing-1")

	err = b.UpdateThingGroupsForThing(&iot.UpdateThingGroupsForThingInput{
		ThingName:           "thing-1",
		ThingGroupsToRemove: []string{"group-a"},
	})
	require.NoError(t, err)

	members2, err := b.ListThingsInThingGroup(&iot.ListThingsInThingGroupInput{ThingGroupName: "group-a"})
	require.NoError(t, err)
	assert.NotContains(t, members2, "thing-1")

	members3, err := b.ListThingsInThingGroup(&iot.ListThingsInThingGroupInput{ThingGroupName: "group-b"})
	require.NoError(t, err)
	assert.Contains(t, members3, "thing-1")
}

func TestBackend_UpdateThingGroupsForThing_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		setup   func(*iot.InMemoryBackend)
		input   *iot.UpdateThingGroupsForThingInput
		name    string
	}{
		{
			name:    "missing_thing_name",
			input:   &iot.UpdateThingGroupsForThingInput{},
			wantErr: iot.ErrValidation,
		},
		{
			name:    "thing_not_found",
			input:   &iot.UpdateThingGroupsForThingInput{ThingName: "nope"},
			wantErr: iot.ErrThingNotFound,
		},
		{
			name: "group_not_found",
			setup: func(b *iot.InMemoryBackend) {
				_, _ = b.CreateThing(&iot.CreateThingInput{ThingName: "thing-1"})
			},
			input: &iot.UpdateThingGroupsForThingInput{
				ThingName:        "thing-1",
				ThingGroupsToAdd: []string{"missing-group"},
			},
			wantErr: iot.ErrThingGroupNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(b)
			}

			err := b.UpdateThingGroupsForThing(tt.input)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestHandler_UpdateThingGroupsForThing(t *testing.T) {
	t.Parallel()
	h := newIoTHandler(t)

	iotOK(t, h, http.MethodPost, "/things/thing-1", nil)
	iotOK(t, h, http.MethodPost, "/thing-groups/group-a", nil)

	iotOK(t, h, http.MethodPut, "/thing-groups/updateThingGroupsForThing", map[string]any{
		"thingName":        "thing-1",
		"thingGroupsToAdd": []string{"group-a"},
	})

	out := iotOK(t, h, http.MethodGet, "/thing-groups/group-a/things", nil)
	things, _ := out["things"].([]any)
	assert.Contains(t, things, "thing-1")
}

func TestUpdateThingGroup_ReturnsVersion(t *testing.T) {
	t.Parallel()

	h, _ := newR3Handler()
	r3Req(t, h, http.MethodPost, "/thing-groups/ver-group", map[string]any{
		"thingGroupProperties": map[string]any{
			"thingGroupDescription": "original",
		},
	})

	var out map[string]any
	code := r3JSON(t, h, http.MethodPatch, "/thing-groups/ver-group", map[string]any{
		"thingGroupProperties": map[string]any{
			"thingGroupDescription": "updated",
		},
	}, &out)
	require.Equal(t, http.StatusOK, code)
	v, ok := out["version"]
	assert.True(t, ok, "response should contain 'version' key")
	assert.InEpsilon(t, float64(2), v, 0.001)
}

func TestUpdateThingGroup_VersionIncrements(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "inc-group"})
	require.NoError(t, err)

	ver1, err := b.UpdateThingGroup(&iot.UpdateThingGroupInput{
		ThingGroupName: "inc-group",
		Description:    "first update",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), ver1)

	ver2, err := b.UpdateThingGroup(&iot.UpdateThingGroupInput{
		ThingGroupName: "inc-group",
		Description:    "second update",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), ver2)
}

func TestUpdateThingGroup_ExpectedVersionMismatch(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "ev-group"})
	require.NoError(t, err)

	_, err = b.UpdateThingGroup(&iot.UpdateThingGroupInput{
		ThingGroupName:  "ev-group",
		Description:     "upd",
		ExpectedVersion: 99,
	})
	require.ErrorIs(t, err, iot.ErrVersionConflict)
}

func TestUpdateThingGroup_ZeroExpectedVersion_Ignored(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "nocheck-group"})
	require.NoError(t, err)

	_, err = b.UpdateThingGroup(&iot.UpdateThingGroupInput{
		ThingGroupName:  "nocheck-group",
		Description:     "updated",
		ExpectedVersion: 0,
	})
	require.NoError(t, err)
}

func TestUpdateThingGroup_MultipleUpdates_VersionTrackingCorrect(t *testing.T) {
	t.Parallel()

	h, _ := newR3Handler()
	r3Req(t, h, http.MethodPost, "/thing-groups/multi-ver-group", nil)

	for expectedVersion := float64(2); expectedVersion <= 5; expectedVersion++ {
		var out map[string]any
		code := r3JSON(t, h, http.MethodPatch, "/thing-groups/multi-ver-group", map[string]any{
			"thingGroupProperties": map[string]any{
				"thingGroupDescription": "update",
			},
		}, &out)
		require.Equal(t, http.StatusOK, code)
		assert.InEpsilon(t, expectedVersion, out["version"], 0.001)
	}
}

func TestDescribeThingGroup_IncludesParentGroupName(t *testing.T) {
	t.Parallel()

	h, _ := newR3Handler()
	r3Req(t, h, http.MethodPost, "/thing-groups/parent-group", nil)
	r3Req(t, h, http.MethodPost, "/thing-groups/child-group", map[string]any{
		"parentGroupName": "parent-group",
	})

	var out map[string]any
	code := r3JSON(t, h, http.MethodGet, "/thing-groups/child-group", nil, &out)
	require.Equal(t, http.StatusOK, code)
	meta, ok := out["thingGroupMetadata"].(map[string]any)
	require.True(t, ok, "response should contain thingGroupMetadata")
	assert.Equal(t, "parent-group", meta["parentGroupName"])
}

// TestDescribeThingGroup_RootToParentThingGroups covers ThingGroupMetadata's
// "rootToParentThingGroups" field (root-first list of {groupName, groupArn}
// ancestors; awsRestjson1_deserializeDocumentThingGroupMetadata, v1.77.4). A
// root-level group has no ancestors, so real AWS omits the field entirely
// rather than sending an empty list.
func TestDescribeThingGroup_RootToParentThingGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		groupPath     string
		wantAncestors []string // root-first
	}{
		{name: "root_group_has_no_rootToParentThingGroups_field", groupPath: "root-rtp", wantAncestors: nil},
		{name: "child_group_has_one_ancestor", groupPath: "child-rtp", wantAncestors: []string{"root-rtp"}},
		{
			name:          "grandchild_group_has_root_first_ancestor_chain",
			groupPath:     "grandchild-rtp",
			wantAncestors: []string{"root-rtp", "child-rtp"},
		},
	}

	h, _ := newR3Handler()
	r3Req(t, h, http.MethodPost, "/thing-groups/root-rtp", nil)
	r3Req(t, h, http.MethodPost, "/thing-groups/child-rtp", map[string]any{"parentGroupName": "root-rtp"})
	r3Req(t, h, http.MethodPost, "/thing-groups/grandchild-rtp", map[string]any{"parentGroupName": "child-rtp"})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out map[string]any
			code := r3JSON(t, h, http.MethodGet, "/thing-groups/"+tt.groupPath, nil, &out)
			require.Equal(t, http.StatusOK, code)
			meta, ok := out["thingGroupMetadata"].(map[string]any)
			require.True(t, ok, "response should contain thingGroupMetadata")

			roots, hasRoots := meta["rootToParentThingGroups"]
			if tt.wantAncestors == nil {
				assert.False(t, hasRoots, "rootToParentThingGroups should be absent for a root-level group")

				return
			}

			require.True(t, hasRoots, "rootToParentThingGroups should be present")

			entries, ok := roots.([]any)
			require.True(t, ok)
			require.Len(t, entries, len(tt.wantAncestors))

			for i, wantName := range tt.wantAncestors {
				entry, entryOK := entries[i].(map[string]any)
				require.True(t, entryOK)
				assert.Equal(t, wantName, entry["groupName"])
				assert.NotEmpty(t, entry["groupArn"])
			}
		})
	}
}

func TestDescribeThingGroup_MetadataHasCreationDate(t *testing.T) {
	t.Parallel()

	h, _ := newR3Handler()
	r3Req(t, h, http.MethodPost, "/thing-groups/cd-group", nil)

	var out map[string]any
	r3JSON(t, h, http.MethodGet, "/thing-groups/cd-group", nil, &out)
	meta := out["thingGroupMetadata"].(map[string]any)
	assert.NotEmpty(t, meta["creationDate"])
}

func TestCreateThingGroup_ParentGroupNameStored(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "root"})
	require.NoError(t, err)

	child, err := b.CreateThingGroup(&iot.CreateThingGroupInput{
		ThingGroupName:  "child",
		ParentGroupName: "root",
	})
	require.NoError(t, err)
	assert.Equal(t, "root", child.ParentGroupName)
}

func TestErrorFormat_ThingGroupNotFound_UsesAWSFormat(t *testing.T) {
	t.Parallel()

	h, _ := newR3Handler()
	var out map[string]string
	code := r3JSON(t, h, http.MethodGet, "/thing-groups/missing-group", nil, &out)
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, "ResourceNotFoundException", out["__type"])
}

func TestDescribeThingGroup_StoresAndReturnsParent(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "parent"})
	require.NoError(t, err)

	_, err = b.CreateThingGroup(&iot.CreateThingGroupInput{
		ThingGroupName:  "child",
		ParentGroupName: "parent",
	})
	require.NoError(t, err)

	child, err := b.DescribeThingGroup("child")
	require.NoError(t, err)
	assert.Equal(t, "parent", child.ParentGroupName)
}

func TestCreateThingGroup_DuplicateName_Conflict(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "dup-group"})
	require.NoError(t, err)

	_, err = b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "dup-group"})
	require.ErrorIs(t, err, iot.ErrAlreadyExists)
}

func TestDeleteThingGroup_NotFound_Error(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	err := b.DeleteThingGroup("nonexistent-group", 0)
	require.ErrorIs(t, err, iot.ErrThingGroupNotFound)
}

// DeleteThingGroupInput.ExpectedVersion (iot@v1.77.4/api_op_DeleteThingGroup.go:38-39)
// and its deserializeOpError switch declares a VersionConflictException case.
func TestDeleteThingGroup_VersionConflict_Rejected(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "del-ver-group"})
	require.NoError(t, err)

	err = b.DeleteThingGroup("del-ver-group", 99)
	require.ErrorIs(t, err, iot.ErrVersionConflict)

	_, err = b.DescribeThingGroup("del-ver-group")
	require.NoError(t, err, "thing group must survive a rejected version-mismatched delete")
}

func TestCreateThingGroup_VersionStartsAt1(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	tg, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "ver1-group"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), tg.Version)
}

func TestUpdateThingGroup_NotFound_Error(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.UpdateThingGroup(&iot.UpdateThingGroupInput{ThingGroupName: "ghost-group"})
	require.ErrorIs(t, err, iot.ErrThingGroupNotFound)
}

func TestListThingsInThingGroup_EmptyGroup(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "empty-group"})
	require.NoError(t, err)

	members, err := b.ListThingsInThingGroup(&iot.ListThingsInThingGroupInput{ThingGroupName: "empty-group"})
	require.NoError(t, err)
	assert.Empty(t, members)
}

func TestAddThingToGroup_ListsCorrectly(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "member-group"})
	require.NoError(t, err)

	_, err = b.CreateThing(&iot.CreateThingInput{ThingName: "group-member-thing"})
	require.NoError(t, err)

	err = b.AddThingToThingGroup(&iot.AddThingToThingGroupInput{
		ThingGroupName: "member-group",
		ThingName:      "group-member-thing",
	})
	require.NoError(t, err)

	members, err := b.ListThingsInThingGroup(&iot.ListThingsInThingGroupInput{ThingGroupName: "member-group"})
	require.NoError(t, err)
	assert.Contains(t, members, "group-member-thing")
}

func TestRemoveThingFromGroup_UpdatesMembership(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThingGroup(&iot.CreateThingGroupInput{ThingGroupName: "remove-test-group"})
	require.NoError(t, err)

	_, err = b.CreateThing(&iot.CreateThingInput{ThingName: "remove-member-thing"})
	require.NoError(t, err)

	err = b.AddThingToThingGroup(&iot.AddThingToThingGroupInput{
		ThingGroupName: "remove-test-group",
		ThingName:      "remove-member-thing",
	})
	require.NoError(t, err)

	err = b.RemoveThingFromThingGroup(&iot.RemoveThingFromThingGroupInput{
		ThingGroupName: "remove-test-group",
		ThingName:      "remove-member-thing",
	})
	require.NoError(t, err)

	members, err := b.ListThingsInThingGroup(&iot.ListThingsInThingGroupInput{ThingGroupName: "remove-test-group"})
	require.NoError(t, err)
	assert.NotContains(t, members, "remove-member-thing")
}

func TestDescribeThingGroup_NotFound_Error(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.DescribeThingGroup("ghost-group")
	require.ErrorIs(t, err, iot.ErrThingGroupNotFound)
}

// TestListThingGroups_RealSDKClient_GroupNameAndArnKeys proves ListThingGroups'
// items key their name/ARN groupName/groupArn (types.GroupNameAndArn,
// iot@v1.77.4 deserializers.go's awsRestjson1_deserializeDocumentGroupNameAndArn),
// not thingGroupName/thingGroupArn -- the correct keys for the DIFFERENT
// CreateThingGroupOutput/DescribeThingGroupOutput shape. Before this fix
// every real client's ListThingGroups().ThingGroups[].GroupName/GroupArn
// decoded empty.
func TestListThingGroups_RealSDKClient_GroupNameAndArnKeys(t *testing.T) {
	t.Parallel()

	h := iot.NewHandler(iot.NewInMemoryBackend(), nil)
	client := newTestIoTClient(t, h)
	ctx := t.Context()

	_, err := client.CreateThingGroup(ctx, &iotsdk.CreateThingGroupInput{
		ThingGroupName: aws.String("group-real-client"),
	})
	require.NoError(t, err)

	listOut, err := client.ListThingGroups(ctx, &iotsdk.ListThingGroupsInput{})
	require.NoError(t, err)
	require.Len(t, listOut.ThingGroups, 1)
	assert.Equal(t, "group-real-client", aws.ToString(listOut.ThingGroups[0].GroupName))
	assert.NotEmpty(t, aws.ToString(listOut.ThingGroups[0].GroupArn))
}
