package iot_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iot"
)

func TestBackend_CreateAndDescribeThing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   *iot.CreateThingInput
		name    string
		wantErr bool
	}{
		{
			name: "create_basic_thing",
			input: &iot.CreateThingInput{
				ThingName:     "sensor-1",
				ThingTypeName: "TemperatureSensor",
				AttributePayload: &iot.AttributePayload{
					Attributes: map[string]string{"location": "lab"},
				},
			},
		},
		{
			name: "create_thing_no_attributes",
			input: &iot.CreateThingInput{
				ThingName: "sensor-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()

			out, err := b.CreateThing(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.input.ThingName, out.ThingName)
			assert.NotEmpty(t, out.ThingARN)
			assert.NotEmpty(t, out.ThingID)

			described, dErr := b.DescribeThing(tt.input.ThingName)
			require.NoError(t, dErr)
			assert.Equal(t, tt.input.ThingName, described.ThingName)
		})
	}
}

func TestBackend_DescribeThing_NotFound(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	_, err := b.DescribeThing("nonexistent")
	require.ErrorIs(t, err, iot.ErrThingNotFound)
}

func TestBackend_DeleteThing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(*iot.InMemoryBackend)
		name      string
		thingName string
		wantErr   bool
	}{
		{
			name:      "delete_existing",
			thingName: "my-thing",
			setup: func(b *iot.InMemoryBackend) {
				_, _ = b.CreateThing(&iot.CreateThingInput{ThingName: "my-thing"})
			},
		},
		{
			name:      "delete_nonexistent",
			thingName: "missing",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()

			if tt.setup != nil {
				tt.setup(b)
			}

			err := b.DeleteThing(tt.thingName, 0)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			_, descErr := b.DescribeThing(tt.thingName)
			require.ErrorIs(t, descErr, iot.ErrThingNotFound)
		})
	}
}

func TestDeleteThing_ClearsGhostStateOnRecreate(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()

	created, err := b.CreateThing(&iot.CreateThingInput{ThingName: "reused-thing"})
	require.NoError(t, err)
	require.NoError(t, b.TagResourceGeneric(created.ThingARN, map[string]string{"env": "prod"}))
	require.NoError(t, b.AddThingToBillingGroup(&iot.AddThingToBillingGroupInput{
		ThingName:        "reused-thing",
		BillingGroupName: "old-group",
	}))

	described, err := b.DescribeThing("reused-thing")
	require.NoError(t, err)
	require.Equal(t, "old-group", described.BillingGroupName)

	require.NoError(t, b.DeleteThing("reused-thing", 0))

	recreated, err := b.CreateThing(&iot.CreateThingInput{ThingName: "reused-thing"})
	require.NoError(t, err)
	require.Equal(t, created.ThingARN, recreated.ThingARN)
	assert.Empty(t, b.ListTagsForResource(recreated.ThingARN))

	redescribed, err := b.DescribeThing("reused-thing")
	require.NoError(t, err)
	assert.Empty(t, redescribed.BillingGroupName)
}

func TestDeleteThing_LeavesOtherThingStateIntact(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()

	gone, err := b.CreateThing(&iot.CreateThingInput{ThingName: "gone-thing"})
	require.NoError(t, err)
	kept, err := b.CreateThing(&iot.CreateThingInput{ThingName: "kept-thing"})
	require.NoError(t, err)

	require.NoError(t, b.TagResourceGeneric(gone.ThingARN, map[string]string{"env": "prod"}))
	require.NoError(t, b.TagResourceGeneric(kept.ThingARN, map[string]string{"env": "dev"}))
	require.NoError(t, b.AddThingToBillingGroup(&iot.AddThingToBillingGroupInput{
		ThingName:        "kept-thing",
		BillingGroupName: "kept-group",
	}))

	require.NoError(t, b.DeleteThing("gone-thing", 0))

	assert.Empty(t, b.ListTagsForResource(gone.ThingARN))
	assert.Equal(t, map[string]string{"env": "dev"}, b.ListTagsForResource(kept.ThingARN))

	kdesc, err := b.DescribeThing("kept-thing")
	require.NoError(t, err)
	assert.Equal(t, "kept-group", kdesc.BillingGroupName)
}

// TestSortedListThings verifies ListThings returns items sorted by name.
func TestSortedListThings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		seedNames []string
		wantOrder []string
	}{
		{
			name:      "three_things_sorted",
			seedNames: []string{"zebra", "alpha", "mango"},
			wantOrder: []string{"alpha", "mango", "zebra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()
			for _, n := range tt.seedNames {
				b.AddThingInternal(iot.Thing{ThingName: n})
			}

			things := b.ListThings()
			require.Len(t, things, len(tt.wantOrder))

			for i, want := range tt.wantOrder {
				assert.Equal(t, want, things[i].ThingName)
			}
		})
	}
}

// TestDeepCopy_DescribeThing verifies mutations do not affect backend state.
func TestDeepCopy_DescribeThing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "mutate_copy_does_not_affect_backend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()
			b.AddThingInternal(iot.Thing{
				ThingName:  "sensor",
				Attributes: map[string]string{"env": "prod"},
			})

			t1, err := b.DescribeThing("sensor")
			require.NoError(t, err)

			// Mutate the returned copy.
			t1.Attributes["env"] = "mutated"

			// Fetch again – original should be unchanged.
			t2, err := b.DescribeThing("sensor")
			require.NoError(t, err)
			assert.Equal(t, "prod", t2.Attributes["env"])
		})
	}
}

// TestThingID_StoredAndReturned verifies ThingID is stored and returned.
func TestThingID_StoredAndReturned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "thingid_roundtrip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()

			out, err := b.CreateThing(&iot.CreateThingInput{ThingName: "id-test"})
			require.NoError(t, err)
			assert.NotEmpty(t, out.ThingID)

			thing, err := b.DescribeThing("id-test")
			require.NoError(t, err)
			assert.Equal(t, out.ThingID, thing.ThingID)
		})
	}
}

// TestNonNilAttributes verifies Thing.Attributes is never nil.
func TestNonNilAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "no_attributes_payload_gives_empty_map"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()

			_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "no-attrs"})
			require.NoError(t, err)

			thing, err := b.DescribeThing("no-attrs")
			require.NoError(t, err)
			assert.NotNil(t, thing.Attributes)
		})
	}
}

// TestCreateThing_Validation verifies empty ThingName is rejected at backend.
func TestCreateThing_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		name      string
		thingName string
	}{
		{name: "empty_name", thingName: "", wantErr: iot.ErrValidation},
		{name: "non_empty_name", thingName: "valid", wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()
			_, err := b.CreateThing(&iot.CreateThingInput{ThingName: tt.thingName})

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestThingKey_ARNFallback verifies thingKey uses ARN when name is empty.
func TestThingKey_ARNFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "billing_group_with_arn_only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()
			// Pass ARN only, no thing name.
			err := b.AddThingToBillingGroup(&iot.AddThingToBillingGroupInput{
				BillingGroupName: "bg1",
				ThingArn:         "arn:aws:iot:us-east-1:123456789012:thing/my-thing",
			})
			require.NoError(t, err)
		})
	}
}

func TestThing_VersionStartsAt1(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	_, err := backend.CreateThing(&iot.CreateThingInput{ThingName: "v1-thing"})
	require.NoError(t, err)

	th, err := backend.DescribeThing("v1-thing")
	require.NoError(t, err)
	assert.Equal(t, int64(1), th.Version)
}

func TestUpdateThing_IncrementsVersion(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	_, err := backend.CreateThing(&iot.CreateThingInput{ThingName: "update-thing"})
	require.NoError(t, err)

	err = backend.UpdateThing(&iot.UpdateThingInput{
		ThingName: "update-thing",
		AttributePayload: &iot.AttributePayload{
			Attributes: map[string]string{"env": "prod"},
		},
	})
	require.NoError(t, err)

	th, err := backend.DescribeThing("update-thing")
	require.NoError(t, err)
	assert.Equal(t, int64(2), th.Version)
	assert.Equal(t, "prod", th.Attributes["env"])
}

func TestUpdateThing_NotFound(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	err := backend.UpdateThing(&iot.UpdateThingInput{ThingName: "nonexistent"})
	require.Error(t, err)
	assert.ErrorIs(t, err, iot.ErrThingNotFound)
}

func TestListThingPrincipals(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	backend.AddThingInternal(iot.Thing{ThingName: "principal-thing"})

	require.NoError(t, backend.AttachThingPrincipal(&iot.AttachThingPrincipalInput{
		ThingName: "principal-thing",
		Principal: "arn:aws:iot:us-east-1:000000000000:cert/abc123",
	}))

	principals, err := backend.ListThingPrincipals("principal-thing")
	require.NoError(t, err)
	require.Len(t, principals, 1)
	assert.Equal(t, "arn:aws:iot:us-east-1:000000000000:cert/abc123", principals[0])
}

func TestListThingPrincipals_EmptyOnNoAttachment(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	backend.AddThingInternal(iot.Thing{ThingName: "empty-thing"})

	principals, err := backend.ListThingPrincipals("empty-thing")
	require.NoError(t, err)
	assert.NotNil(t, principals)
	assert.Empty(t, principals)
}

func TestListThingPrincipals_NotFound(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	_, err := backend.ListThingPrincipals("ghost-thing")
	require.Error(t, err)
	assert.ErrorIs(t, err, iot.ErrThingNotFound)
}

func TestThingPrincipalCount_Helper(t *testing.T) {
	t.Parallel()

	backend := iot.NewInMemoryBackend()
	backend.AddThingInternal(iot.Thing{ThingName: "count-thing"})
	assert.Equal(t, 0, backend.ThingPrincipalCount("count-thing"))

	require.NoError(t, backend.AttachThingPrincipal(&iot.AttachThingPrincipalInput{
		ThingName: "count-thing",
		Principal: "cert-1",
	}))
	assert.Equal(t, 1, backend.ThingPrincipalCount("count-thing"))
}

func TestUpdateThing_ExpectedVersionMatch(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "ver-thing"})
	require.NoError(t, err)

	err = b.UpdateThing(&iot.UpdateThingInput{
		ThingName:       "ver-thing",
		ExpectedVersion: 1,
		AttributePayload: &iot.AttributePayload{
			Attributes: map[string]string{"env": "test"},
		},
	})
	require.NoError(t, err)

	t2, err := b.DescribeThing("ver-thing")
	require.NoError(t, err)
	assert.Equal(t, int64(2), t2.Version)
}

func TestUpdateThing_ExpectedVersionMismatch_ReturnsVersionConflict(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "conflict-thing"})
	require.NoError(t, err)

	err = b.UpdateThing(&iot.UpdateThingInput{
		ThingName:       "conflict-thing",
		ExpectedVersion: 99,
	})
	require.ErrorIs(t, err, iot.ErrVersionConflict)
}

func TestUpdateThing_ZeroExpectedVersion_Ignored(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "nocheck-thing"})
	require.NoError(t, err)

	err = b.UpdateThing(&iot.UpdateThingInput{
		ThingName:       "nocheck-thing",
		ExpectedVersion: 0,
	})
	require.NoError(t, err)
}

func TestUpdateThing_VersionConflict_AfterSuccessfulUpdate(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "seq-ver-thing"})
	require.NoError(t, err)

	err = b.UpdateThing(&iot.UpdateThingInput{
		ThingName:       "seq-ver-thing",
		ExpectedVersion: 1,
	})
	require.NoError(t, err)

	err = b.UpdateThing(&iot.UpdateThingInput{
		ThingName:       "seq-ver-thing",
		ExpectedVersion: 1,
	})
	require.ErrorIs(t, err, iot.ErrVersionConflict)
}

func TestUpdateThing_EmptyPayload_IncrementsVersion(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "empty-update-thing"})
	require.NoError(t, err)

	err = b.UpdateThing(&iot.UpdateThingInput{ThingName: "empty-update-thing"})
	require.NoError(t, err)

	th, err := b.DescribeThing("empty-update-thing")
	require.NoError(t, err)
	assert.Equal(t, int64(2), th.Version)
}

func TestCreateThing_DuplicateName_Conflict(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "dup-thing"})
	require.NoError(t, err)

	_, err = b.CreateThing(&iot.CreateThingInput{ThingName: "dup-thing"})
	require.ErrorIs(t, err, iot.ErrAlreadyExists)
}

func TestDeleteThing_NotFound_Error(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	err := b.DeleteThing("ghost-thing", 0)
	require.ErrorIs(t, err, iot.ErrThingNotFound)
}

// DeleteThingInput.ExpectedVersion (iot@v1.77.4/api_op_DeleteThing.go:40-43):
// if the version of the record in the registry does not match the expected
// version specified in the request, the DeleteThing request is rejected
// with a VersionConflictException.
func TestDeleteThing_VersionConflict_Rejected(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "del-ver-thing"})
	require.NoError(t, err)

	err = b.DeleteThing("del-ver-thing", 99)
	require.ErrorIs(t, err, iot.ErrVersionConflict)

	_, err = b.DescribeThing("del-ver-thing")
	require.NoError(t, err, "thing must survive a rejected version-mismatched delete")
}

func TestDeleteThing_VersionMatch_Succeeds(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "del-ver-match-thing"})
	require.NoError(t, err)

	th, err := b.DescribeThing("del-ver-match-thing")
	require.NoError(t, err)

	require.NoError(t, b.DeleteThing("del-ver-match-thing", th.Version))

	_, err = b.DescribeThing("del-ver-match-thing")
	require.ErrorIs(t, err, iot.ErrThingNotFound)
}

func TestUpdateThing_NotFound_Error(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	err := b.UpdateThing(&iot.UpdateThingInput{ThingName: "ghost-thing"})
	require.ErrorIs(t, err, iot.ErrThingNotFound)
}

func TestListThings_SortedByName(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	names := []string{"zebra", "alpha", "mango"}
	for _, n := range names {
		_, err := b.CreateThing(&iot.CreateThingInput{ThingName: n})
		require.NoError(t, err)
	}

	things := b.ListThings()
	require.Len(t, things, 3)
	assert.Equal(t, "alpha", things[0].ThingName)
	assert.Equal(t, "mango", things[1].ThingName)
	assert.Equal(t, "zebra", things[2].ThingName)
}

func TestDescribeThing_ReturnsThingID(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	out, err := b.CreateThing(&iot.CreateThingInput{ThingName: "id-thing"})
	require.NoError(t, err)

	th, err := b.DescribeThing("id-thing")
	require.NoError(t, err)
	assert.Equal(t, out.ThingID, th.ThingID)
	assert.NotEmpty(t, th.ThingID)
}

func TestThing_ARNFormat(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackendWithConfig("999988887777", "ap-southeast-1")
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "arn-test-thing"})
	require.NoError(t, err)

	th, err := b.DescribeThing("arn-test-thing")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(th.ARN, "arn:aws:iot:ap-southeast-1:999988887777:thing/"),
		"thing ARN should contain region+account, got: %s", th.ARN)
}

func TestAttachThingPrincipal_Stored(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "principal-thing"})
	require.NoError(t, err)

	err = b.AttachThingPrincipal(&iot.AttachThingPrincipalInput{
		ThingName: "principal-thing",
		Principal: "arn:aws:iot:us-east-1:123456789012:cert/" + strings.Repeat("a", 64),
	})
	require.NoError(t, err)

	principals, err := b.ListThingPrincipals("principal-thing")
	require.NoError(t, err)
	require.Len(t, principals, 1)
}

func TestListThingPrincipals_ThingNotFound_Error(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.ListThingPrincipals("ghost-thing")
	require.ErrorIs(t, err, iot.ErrThingNotFound)
}

// TestUpdateThing_MergeFalse_ReplacesAttributes verifies merge:false replaces attributes.
func TestUpdateThing_MergeFalse_ReplacesAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		initialAttr map[string]string
		updateBody  map[string]any
		wantAttrs   map[string]any
		name        string
	}{
		{
			name:        "merge_true_merges",
			initialAttr: map[string]string{"a": "1", "b": "2"},
			updateBody: map[string]any{
				"attributePayload": map[string]any{
					"attributes": map[string]string{"c": "3"},
					"merge":      true,
				},
			},
			wantAttrs: map[string]any{"a": "1", "b": "2", "c": "3"},
		},
		{
			name:        "merge_false_replaces",
			initialAttr: map[string]string{"a": "1", "b": "2"},
			updateBody: map[string]any{
				"attributePayload": map[string]any{
					"attributes": map[string]string{"c": "3"},
					"merge":      false,
				},
			},
			wantAttrs: map[string]any{"c": "3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()
			_, err := b.CreateThing(&iot.CreateThingInput{
				ThingName: "merge-test",
				AttributePayload: &iot.AttributePayload{
					Attributes: tt.initialAttr,
				},
			})
			require.NoError(t, err)

			var bodyAttr iot.AttributePayload
			raw, _ := json.Marshal(tt.updateBody["attributePayload"])
			require.NoError(t, json.Unmarshal(raw, &bodyAttr))

			err = b.UpdateThing(&iot.UpdateThingInput{
				ThingName:        "merge-test",
				AttributePayload: &bodyAttr,
			})
			require.NoError(t, err)

			thing, err := b.DescribeThing("merge-test")
			require.NoError(t, err)

			got := make(map[string]any, len(thing.Attributes))
			for k, v := range thing.Attributes {
				got[k] = v
			}

			assert.Equal(t, tt.wantAttrs, got, "attributes mismatch after update")
		})
	}
}

// TestDeleteThing_WithPrincipals_Blocked verifies things with attached principals
// cannot be deleted.
func TestDeleteThing_WithPrincipals_Blocked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantDeleteErr error
		name          string
		attachPrinc   bool
	}{
		{
			name:          "with_principal_blocked",
			attachPrinc:   true,
			wantDeleteErr: iot.ErrDeleteConflict,
		},
		{
			name:          "no_principal_allowed",
			attachPrinc:   false,
			wantDeleteErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()
			_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "del-thing-" + tt.name})
			require.NoError(t, err)

			if tt.attachPrinc {
				err = b.AttachThingPrincipal(&iot.AttachThingPrincipalInput{
					ThingName: "del-thing-" + tt.name,
					Principal: "arn:aws:iot:us-east-1:000000000000:cert/abc123",
				})
				require.NoError(t, err)
			}

			err = b.DeleteThing("del-thing-"+tt.name, 0)
			if tt.wantDeleteErr != nil {
				require.ErrorIs(t, err, tt.wantDeleteErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
