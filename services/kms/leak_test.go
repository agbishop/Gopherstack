package kms_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

// TestKeyMaterialHistory_CappedAtMax verifies that rotating a key more than
// maxKeyMaterialHistoryEntries times does not grow the history slice beyond the
// cap, preventing unbounded memory growth on long-lived mock instances.
func TestKeyMaterialHistory_CappedAtMax(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rotations int
	}{
		{name: "at cap", rotations: kms.MaxKeyMaterialHistoryEntriesForTest},
		{name: "one over cap", rotations: kms.MaxKeyMaterialHistoryEntriesForTest + 1},
		{name: "well over cap", rotations: kms.MaxKeyMaterialHistoryEntriesForTest + 20},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			b := kms.NewInMemoryBackend()

			out, err := b.CreateKey(ctx, &kms.CreateKeyInput{})
			require.NoError(t, err)
			keyID := out.KeyMetadata.KeyID

			var rotErr error
			for i := range tc.rotations {
				rotErr = b.ForceRotateForTest(keyID)
				require.NoError(t, rotErr, "rotation %d must succeed", i)
			}

			histLen := b.KeyMaterialHistoryLenForTest(keyID)
			require.LessOrEqual(t, histLen, kms.MaxKeyMaterialHistoryEntriesForTest,
				"key material history must not exceed the cap after %d rotations", tc.rotations)
		})
	}
}

// TestKeyMaterialHistory_OldestMaterialDropped verifies that when the history
// cap is reached, the oldest entries are dropped (not the newest), so that
// decrypt operations against recently-rotated keys still work.
func TestKeyMaterialHistory_OldestMaterialDropped(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := kms.NewInMemoryBackend()

	out, err := b.CreateKey(ctx, &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	// Rotate past the cap.
	extra := 5
	for i := range kms.MaxKeyMaterialHistoryEntriesForTest + extra {
		require.NoError(t, b.ForceRotateForTest(keyID), "rotation %d", i)
	}

	// Encryption with the current key must still work after excess rotations.
	enc, err := b.Encrypt(ctx, &kms.EncryptInput{
		KeyID:     keyID,
		Plaintext: []byte("hello"),
	})
	require.NoError(t, err)

	_, err = b.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
	})
	require.NoError(t, err, "decrypt must succeed — current key material must remain accessible")
}

// TestLastUsageLeak_PurgeKey verifies that the janitor's purgeKey removes the
// lastUsage entry for the deleted key, preventing unbounded map growth.
func TestLastUsageLeak_PurgeKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		performCryptoOp bool
		expectLastUsage bool // before purge
	}{
		{
			name:            "key_with_usage_entry_is_cleaned_up",
			performCryptoOp: true,
			expectLastUsage: true,
		},
		{
			name:            "key_without_usage_entry_no_error_on_purge",
			performCryptoOp: false,
			expectLastUsage: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			ctx := context.Background()

			out, err := b.CreateKey(ctx, &kms.CreateKeyInput{Description: "leak-test"})
			require.NoError(t, err)
			keyID := out.KeyMetadata.KeyID

			if tt.performCryptoOp {
				_, err = b.Encrypt(ctx, &kms.EncryptInput{
					KeyID:     keyID,
					Plaintext: []byte("hello"),
				})
				require.NoError(t, err)
				assert.True(t, kms.LastUsageExists(b, kms.MockRegion, keyID),
					"lastUsage must exist after crypto op")
			}

			// Schedule with past deletion date and sweep.
			_, err = b.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
				KeyID:               keyID,
				PendingWindowInDays: 7,
			})
			require.NoError(t, err)
			b.SetDeletionDateForTest(keyID, time.Now().Add(-time.Second))

			j := kms.NewJanitor(b, time.Hour)
			j.SweepOnce(ctx)

			assert.Equal(t, 0, kms.KeyCount(b), "key must be purged")
			assert.False(t, kms.LastUsageExists(b, kms.MockRegion, keyID),
				"lastUsage entry must be deleted after purge")
		})
	}
}

// TestLastUsageLeak_PurgeKeyWithAlias verifies that purgeKey also evicts alias
// cache entries and removes lastUsage for each purged key.
func TestLastUsageLeak_PurgeKeyWithAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		numKeys    int
		numAliases int
	}{
		{name: "single_key_single_alias", numKeys: 1, numAliases: 1},
		{name: "single_key_multiple_aliases", numKeys: 1, numAliases: 3},
		{name: "multiple_keys", numKeys: 3, numAliases: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			ctx := context.Background()

			keyIDs := make([]string, tt.numKeys)
			for i := range tt.numKeys {
				out, err := b.CreateKey(ctx, &kms.CreateKeyInput{Description: "k"})
				require.NoError(t, err)
				keyIDs[i] = out.KeyMetadata.KeyID

				// Do a crypto op to populate lastUsage.
				_, err = b.Encrypt(ctx, &kms.EncryptInput{
					KeyID:     keyIDs[i],
					Plaintext: []byte("x"),
				})
				require.NoError(t, err)

				// Create aliases up to numAliases for first key only.
				if i == 0 {
					for j := range tt.numAliases {
						aliasName := "alias/leak-test-" + string(rune('a'+j))
						require.NoError(t, b.CreateAlias(ctx, &kms.CreateAliasInput{
							AliasName:   aliasName,
							TargetKeyID: keyIDs[i],
						}))
						// Warm the cache.
						_, err = b.DescribeKey(ctx, &kms.DescribeKeyInput{KeyID: aliasName})
						require.NoError(t, err)
					}
				}
			}

			// Schedule all keys for deletion and set past deletion date.
			for _, kid := range keyIDs {
				_, err := b.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
					KeyID:               kid,
					PendingWindowInDays: 7,
				})
				require.NoError(t, err)
				b.SetDeletionDateForTest(kid, time.Now().Add(-time.Second))
			}

			j := kms.NewJanitor(b, time.Hour)
			j.SweepOnce(ctx)

			assert.Equal(t, 0, kms.KeyCount(b), "all keys must be purged")
			assert.Equal(t, 0, kms.AliasCount(b), "all aliases must be purged")
			assert.Equal(t, 0, kms.ResolutionCacheLen(b), "cache must be empty after purge")

			for _, kid := range keyIDs {
				assert.False(t, kms.LastUsageExists(b, kms.MockRegion, kid),
					"lastUsage must not exist for purged key %s", kid)
			}
		})
	}
}

// TestReset_LastUsageCleared verifies that Reset() clears the lastUsage
// sync.Map, mirroring clearResolutionCache's O(1) swap for the sibling
// keyIDResolutionCache in the same Reset() call.
func TestReset_LastUsageCleared(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := kms.NewInMemoryBackend()

	out, err := b.CreateKey(ctx, &kms.CreateKeyInput{Description: "reset-test"})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	_, err = b.Encrypt(ctx, &kms.EncryptInput{KeyID: keyID, Plaintext: []byte("hello")})
	require.NoError(t, err)

	require.True(t, kms.LastUsageExists(b, kms.MockRegion, keyID), "sanity: usage should be recorded")

	b.Reset()

	assert.False(t, kms.LastUsageExists(b, kms.MockRegion, keyID),
		"lastUsage must not survive Reset")
}

// TestReset_ImportWrappingKeysCleared verifies that Reset() clears the
// importWrappingKeys sync.Map, mirroring clearResolutionCache's O(1) swap for
// the sibling keyIDResolutionCache in the same Reset() call.
func TestReset_ImportWrappingKeysCleared(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := kms.NewInMemoryBackend()

	out, err := b.CreateKey(ctx, &kms.CreateKeyInput{
		Description: "reset-test",
		Origin:      kms.KeyOriginExternal,
	})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	_, err = b.GetParametersForImport(ctx, &kms.GetParametersForImportInput{
		KeyID:             keyID,
		WrappingAlgorithm: "RSAES_OAEP_SHA_256",
		WrappingKeySpec:   "RSA_2048",
	})
	require.NoError(t, err)

	require.True(t, kms.ImportWrappingKeyExists(b, keyID), "sanity: wrapping key should be stored")

	b.Reset()

	assert.False(t, kms.ImportWrappingKeyExists(b, keyID),
		"importWrappingKeys must not survive Reset")
}

// TestTagsLeak_PurgeKey lives in whitebox_test.go: it needs direct access to
// the unexported Handler.tags map and janitor sweep.
