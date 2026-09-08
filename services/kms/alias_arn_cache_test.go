package kms_test

// alias_arn_cache_test.go — regression test for a stale keyIDResolutionCache entry.
//
// resolveKeyID caches ANY "arn:"-prefixed KeyId (store.go), including an alias's own
// ARN form (e.g. "arn:aws:kms:<region>:<account>:alias/foo" -- a documented, legitimate
// KeyId shape per aws-sdk-go-v2/service/kms's KeyId doc comments: "Alias ARN: ...").
// UpdateAlias and DeleteAlias only invalidate the cache entry keyed by the bare alias
// name (input.AliasName); neither evicts the entry keyed by the alias's ARN. So once a
// client resolves an alias via its ARN form even once, redirecting or deleting+
// recreating that alias leaves every future ARN-form lookup returning the stale,
// pre-update target key -- a real KeyId-resolution divergence, not just a cache-size
// leak (unlike the already-fixed purgeKey ARN-cache entries, which are safe because key
// UUIDs are never reused; alias NAMES are reused across UpdateAlias/Delete+CreateAlias).

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

func TestAliasARNCache_StaleAfterUpdateAlias(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := kms.NewInMemoryBackendWithConfig("000000000000", "us-east-1")

	keyA, err := b.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)

	keyB, err := b.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)

	require.NoError(t, b.CreateAlias(ctx, &kms.CreateAliasInput{
		AliasName:   "alias/rotating",
		TargetKeyID: keyA.KeyMetadata.KeyID,
	}))

	listed, err := b.ListAliases(ctx, &kms.ListAliasesInput{})
	require.NoError(t, err)
	require.Len(t, listed.Aliases, 1)
	aliasARN := listed.Aliases[0].AliasArn
	require.Contains(t, aliasARN, "alias/rotating")

	// Resolve via the alias's ARN form once, populating keyIDResolutionCache under
	// the ARN string.
	before, err := b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: aliasARN})
	require.NoError(t, err)
	require.Equal(t, keyA.KeyMetadata.KeyID, before.KeyMetadata.KeyID)

	// Redirect the alias to key B.
	require.NoError(t, b.UpdateAlias(ctx, &kms.UpdateAliasInput{
		AliasName:   "alias/rotating",
		TargetKeyID: keyB.KeyMetadata.KeyID,
	}))

	// Resolving via the bare alias name must see the new target immediately.
	byName, err := b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: "alias/rotating"})
	require.NoError(t, err)
	require.Equal(t, keyB.KeyMetadata.KeyID, byName.KeyMetadata.KeyID,
		"bare alias name must resolve to the new target after UpdateAlias")

	// Resolving via the alias's ARN form must ALSO see the new target -- this is the
	// exact call shape that previously hit the stale cache entry.
	byARN, err := b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: aliasARN})
	require.NoError(t, err)
	require.Equal(t, keyB.KeyMetadata.KeyID, byARN.KeyMetadata.KeyID,
		"alias ARN must resolve to the new target after UpdateAlias, not the stale cached one")
}

func TestAliasARNCache_StaleAfterDeleteRecreate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := kms.NewInMemoryBackendWithConfig("000000000000", "us-east-1")

	keyA, err := b.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)

	keyC, err := b.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)

	require.NoError(t, b.CreateAlias(ctx, &kms.CreateAliasInput{
		AliasName:   "alias/recycled",
		TargetKeyID: keyA.KeyMetadata.KeyID,
	}))

	listed, err := b.ListAliases(ctx, &kms.ListAliasesInput{})
	require.NoError(t, err)
	require.Len(t, listed.Aliases, 1)
	aliasARN := listed.Aliases[0].AliasArn

	_, err = b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: aliasARN})
	require.NoError(t, err)

	require.NoError(t, b.DeleteAlias(ctx, &kms.DeleteAliasInput{AliasName: "alias/recycled"}))
	require.NoError(t, b.CreateAlias(ctx, &kms.CreateAliasInput{
		AliasName:   "alias/recycled",
		TargetKeyID: keyC.KeyMetadata.KeyID,
	}))

	byARN, err := b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: aliasARN})
	require.NoError(t, err)
	require.Equal(t, keyC.KeyMetadata.KeyID, byARN.KeyMetadata.KeyID,
		"alias ARN must resolve to the recreated alias's new target, not the stale cached one")
}

// TestAliasARNCache_EvictedOnKeyPurge covers the same stale-cache class reached
// through the janitor's cascade alias delete (purgeKey) rather than DeleteAlias
// directly: an alias resolved via its ARN form, whose target key is later
// permanently purged, must not leave a stale ARN cache entry that a
// same-named alias recreated afterward could be shadowed by.
func TestAliasARNCache_EvictedOnKeyPurge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := kms.NewInMemoryBackendWithConfig("000000000000", "us-east-1")

	keyA, err := b.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)

	require.NoError(t, b.CreateAlias(ctx, &kms.CreateAliasInput{
		AliasName:   "alias/purged",
		TargetKeyID: keyA.KeyMetadata.KeyID,
	}))

	listed, err := b.ListAliases(ctx, &kms.ListAliasesInput{})
	require.NoError(t, err)
	require.Len(t, listed.Aliases, 1)
	aliasARN := listed.Aliases[0].AliasArn

	// Warm the cache under the alias's ARN form.
	_, err = b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: aliasARN})
	require.NoError(t, err)
	require.True(t, kms.ResolutionCacheHas(b, aliasARN))

	_, err = b.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
		KeyID:               keyA.KeyMetadata.KeyID,
		PendingWindowInDays: 7,
	})
	require.NoError(t, err)
	b.SetDeletionDateForTest(keyA.KeyMetadata.KeyID, time.Now().Add(-time.Second))

	kms.NewJanitor(b, time.Hour).SweepOnce(ctx)

	assert.False(t, kms.ResolutionCacheHas(b, aliasARN),
		"alias-ARN cache entry must be evicted when the janitor purges the target key")

	// Recreating the alias name against a fresh key must resolve correctly via
	// its (new) ARN, not be shadowed by anything left over from key A.
	keyB, err := b.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	require.NoError(t, b.CreateAlias(ctx, &kms.CreateAliasInput{
		AliasName:   "alias/purged",
		TargetKeyID: keyB.KeyMetadata.KeyID,
	}))

	byARN, err := b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: aliasARN})
	require.NoError(t, err)
	require.Equal(t, keyB.KeyMetadata.KeyID, byARN.KeyMetadata.KeyID)
}

// TestResolutionCache_KeyOwnARNEvictedOnPurge verifies a key's own ARN-keyed
// resolution-cache entry (populated by resolving KeyId="arn:...:key/<id>"
// directly, not via an alias) is evicted when the janitor permanently purges
// it -- otherwise it would sit in keyIDResolutionCache forever (key UUIDs are
// never reused, so this is a pure unbounded-growth leak, not a staleness bug).
func TestResolutionCache_KeyOwnARNEvictedOnPurge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := kms.NewInMemoryBackendWithConfig("000000000000", "us-east-1")

	key, err := b.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyARN := key.KeyMetadata.Arn

	_, err = b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: keyARN})
	require.NoError(t, err)
	require.True(t, kms.ResolutionCacheHas(b, keyARN))

	_, err = b.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
		KeyID:               key.KeyMetadata.KeyID,
		PendingWindowInDays: 7,
	})
	require.NoError(t, err)
	b.SetDeletionDateForTest(key.KeyMetadata.KeyID, time.Now().Add(-time.Second))

	kms.NewJanitor(b, time.Hour).SweepOnce(ctx)

	assert.False(t, kms.ResolutionCacheHas(b, keyARN),
		"key's own ARN cache entry must be evicted when the janitor purges it")
}
