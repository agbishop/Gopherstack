package rolesanywhere_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rolesanywhere"
)

// ---- Tags ----

// newTaggableTrustAnchor creates a trust anchor and returns its ARN, for
// tests that need a real resourceArn -- TagResource/ListTagsForResource
// validate that the ARN corresponds to an existing trust anchor, profile, or
// CRL (real AWS models ResourceNotFoundException for both), so synthetic
// ARNs are no longer accepted the way they were before that fix.
func newTaggableTrustAnchor(t *testing.T, b *rolesanywhere.InMemoryBackend) string {
	t.Helper()

	src := rolesanywhere.TrustAnchorSource{SourceType: "CERTIFICATE_BUNDLE"}
	ta, err := b.CreateTrustAnchor(context.Background(), t.Name(), src, nil, nil, nil)
	require.NoError(t, err)

	return ta.TrustAnchorArn
}

func TestTagResource_Roundtrip(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	resARN := newTaggableTrustAnchor(t, b)

	tags := []rolesanywhere.TagEntry{
		{Key: "env", Value: "prod"},
		{Key: "team", Value: "security"},
	}

	require.NoError(t, b.TagResource(context.Background(), resARN, tags))

	got, err := b.ListTagsForResource(context.Background(), resARN)
	require.NoError(t, err)
	assert.Len(t, got, 2)

	found := make(map[string]string)
	for _, t := range got {
		found[t.Key] = t.Value
	}

	assert.Equal(t, "prod", found["env"])
	assert.Equal(t, "security", found["team"])
}

func TestUntagResource_RemovesTags(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	resARN := newTaggableTrustAnchor(t, b)

	_ = b.TagResource(
		context.Background(),
		resARN,
		[]rolesanywhere.TagEntry{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
	)
	_ = b.UntagResource(context.Background(), resARN, []string{"a"})

	got, _ := b.ListTagsForResource(context.Background(), resARN)
	assert.Len(t, got, 1)
	assert.Equal(t, "b", got[0].Key)
}

func TestTagResource_Upsert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantValues map[string]string
		name       string
		initial    []rolesanywhere.TagEntry
		updates    []rolesanywhere.TagEntry
	}{
		{
			name: "update existing key preserves other keys",
			initial: []rolesanywhere.TagEntry{
				{Key: "env", Value: "dev"},
				{Key: "team", Value: "ops"},
			},
			updates: []rolesanywhere.TagEntry{{Key: "env", Value: "prod"}},
			wantValues: map[string]string{
				"env":  "prod",
				"team": "ops",
			},
		},
		{
			name:    "add new key",
			initial: []rolesanywhere.TagEntry{{Key: "a", Value: "1"}},
			updates: []rolesanywhere.TagEntry{{Key: "b", Value: "2"}},
			wantValues: map[string]string{
				"a": "1",
				"b": "2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			arn := newTaggableTrustAnchor(t, b)
			require.NoError(t, b.TagResource(context.Background(), arn, tt.initial))
			require.NoError(t, b.TagResource(context.Background(), arn, tt.updates))

			got, err := b.ListTagsForResource(context.Background(), arn)
			require.NoError(t, err)

			found := make(map[string]string)
			for _, tg := range got {
				found[tg.Key] = tg.Value
			}
			assert.Equal(t, tt.wantValues, found)
		})
	}
}

// TestTagResource_UnknownResourceNotFound proves that TagResource,
// ListTagsForResource, and UntagResource all reject an ARN that matches no
// trust anchor, profile, or CRL with ResourceNotFoundException, matching
// real AWS (all three operations model that exception -- see
// aws-sdk-go-v2/service/rolesanywhere/deserializers.go's
// awsRestjson1_deserializeOpErrorUntagResource, which is not a no-op-only
// error set as a prior version of this test incorrectly assumed).
func TestTagResource_UnknownResourceNotFound(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	unknownARN := "arn:aws:rolesanywhere:us-east-1:000000000000:trust-anchor/does-not-exist"

	err := b.TagResource(context.Background(), unknownARN, []rolesanywhere.TagEntry{{Key: "a", Value: "1"}})
	require.Error(t, err)

	_, err = b.ListTagsForResource(context.Background(), unknownARN)
	require.Error(t, err)

	err = b.UntagResource(context.Background(), unknownARN, []string{"a"})
	require.Error(t, err)
}

// TestTagResource_DeleteCascadesTags proves that deleting a trust anchor
// removes its tags from the tag store too, so a subsequent TagResource
// against the same (now-gone) ARN correctly reports ResourceNotFoundException
// rather than resurrecting a ghost row.
func TestTagResource_DeleteCascadesTags(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	src := rolesanywhere.TrustAnchorSource{SourceType: "CERTIFICATE_BUNDLE"}
	ta, err := b.CreateTrustAnchor(context.Background(), "cascade-anchor", src, nil, nil, nil)
	require.NoError(t, err)

	require.NoError(t, b.TagResource(context.Background(), ta.TrustAnchorArn,
		[]rolesanywhere.TagEntry{{Key: "a", Value: "1"}}))

	_, err = b.DeleteTrustAnchor(context.Background(), ta.TrustAnchorID)
	require.NoError(t, err)

	_, err = b.ListTagsForResource(context.Background(), ta.TrustAnchorArn)
	require.Error(t, err)
}

// TestTagResource_TooManyTags proves that pushing a resource's total tag
// count past the 200-tag limit (the same limit the real SDK's shared
// TagList shape enforces, max: 200) returns TooManyTagsException without
// applying any of the offending batch, rather than partially tagging past
// the limit.
func TestTagResource_TooManyTags(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	resARN := newTaggableTrustAnchor(t, b)

	const overLimit = 201

	tags := make([]rolesanywhere.TagEntry, overLimit)
	for i := range tags {
		tags[i] = rolesanywhere.TagEntry{Key: "k" + strconv.Itoa(i), Value: "v"}
	}

	err := b.TagResource(context.Background(), resARN, tags)
	require.Error(t, err)

	got, listErr := b.ListTagsForResource(context.Background(), resARN)
	require.NoError(t, listErr)
	assert.Empty(t, got, "the over-limit batch must not be partially applied")
}

// TestTagResource_TooManyTags_LeavesExistingValuesUntouched proves that a
// rejected over-limit batch does not even partially apply -- not just to the
// new keys it would have added, but to existing keys it would have
// overwritten. TagResource merges the batch into the same backing array
// backing the tag store before checking the resulting count, so an update to
// an already-present key must not survive a rejected batch.
func TestTagResource_TooManyTags_LeavesExistingValuesUntouched(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	resARN := newTaggableTrustAnchor(t, b)

	const atLimit = 200

	initial := make([]rolesanywhere.TagEntry, atLimit)
	for i := range initial {
		initial[i] = rolesanywhere.TagEntry{Key: "k" + strconv.Itoa(i), Value: "v"}
	}

	initial[0] = rolesanywhere.TagEntry{Key: "env", Value: "old"}
	require.NoError(t, b.TagResource(context.Background(), resARN, initial))

	err := b.TagResource(context.Background(), resARN, []rolesanywhere.TagEntry{
		{Key: "env", Value: "new"},
		{Key: "extra", Value: "v"},
	})
	require.Error(t, err)

	got, listErr := b.ListTagsForResource(context.Background(), resARN)
	require.NoError(t, listErr)

	found := make(map[string]string, len(got))
	for _, tg := range got {
		found[tg.Key] = tg.Value
	}

	assert.Equal(t, "old", found["env"], "a rejected batch must not overwrite an existing tag's value")
	assert.NotContains(t, found, "extra", "a rejected batch must not add any of its new keys")
}
