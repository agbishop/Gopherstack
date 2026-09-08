package kms_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"

	"strings"
)

func TestAliasUpdate_CacheInvalidated(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	k1 := mustCreateSymKey(t, b)
	k2 := mustCreateSymKey(t, b)
	alias := "alias/test-cache"

	require.NoError(t, b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: alias, TargetKeyID: k1}))

	// Encrypt with k1 via alias.
	plain := []byte("cache test")
	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: alias, Plaintext: plain})
	require.NoError(t, err)

	// Update alias to k2.
	require.NoError(t, b.UpdateAlias(context.Background(), &kms.UpdateAliasInput{AliasName: alias, TargetKeyID: k2}))

	// Describe via alias should now resolve to k2.
	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: alias})
	require.NoError(t, err)
	assert.Equal(t, k2, desc.KeyMetadata.KeyID)

	// Old ciphertext (encrypted under k1) should still decrypt directly by k1 keyID.
	dec, err := b.Decrypt(context.Background(), &kms.DecryptInput{CiphertextBlob: enc.CiphertextBlob})
	require.NoError(t, err)
	assert.Equal(t, plain, dec.Plaintext)
}

func TestAliasDelete_CacheInvalidated(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)
	alias := "alias/test-delete-cache"

	require.NoError(t, b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: alias, TargetKeyID: keyID}))

	// Confirm alias resolves.
	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: alias})
	require.NoError(t, err)
	assert.Equal(t, keyID, desc.KeyMetadata.KeyID)

	// Delete alias.
	require.NoError(t, b.DeleteAlias(context.Background(), &kms.DeleteAliasInput{AliasName: alias}))

	// Now alias should not resolve.
	_, err = b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: alias})
	require.Error(t, err)
}

func TestAlias_CreateUpdateDeleteList(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	k1 := mustCreateSymKey(t, b)
	k2 := mustCreateSymKey(t, b)
	alias := "alias/audit-lifecycle"

	// Create.
	require.NoError(t, b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: alias, TargetKeyID: k1}))

	// List — alias must appear.
	list, err := b.ListAliases(context.Background(), &kms.ListAliasesInput{})
	require.NoError(t, err)
	found := false
	for _, a := range list.Aliases {
		if a.AliasName == alias {
			assert.Equal(t, k1, a.TargetKeyID)
			found = true
		}
	}
	assert.True(t, found, "alias should appear in ListAliases")

	// Update.
	require.NoError(t, b.UpdateAlias(context.Background(), &kms.UpdateAliasInput{AliasName: alias, TargetKeyID: k2}))
	desc, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: alias})
	require.NoError(t, err)
	assert.Equal(t, k2, desc.KeyMetadata.KeyID)

	// Delete.
	require.NoError(t, b.DeleteAlias(context.Background(), &kms.DeleteAliasInput{AliasName: alias}))
	_, err = b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: alias})
	require.Error(t, err)
}

func TestAlias_DuplicateCreateFails(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)
	alias := "alias/dup-test"

	require.NoError(t, b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: alias, TargetKeyID: keyID}))
	err := b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: alias, TargetKeyID: keyID})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrAliasAlreadyExists)
}

func TestAlias_DeleteNonexistentFails(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	err := b.DeleteAlias(context.Background(), &kms.DeleteAliasInput{AliasName: "alias/nonexistent"})
	require.Error(t, err)
}

func TestErrors_AliasNotFound_Sentinel(t *testing.T) {
	t.Parallel()
	b := newBackend(t)

	err := b.DeleteAlias(context.Background(), &kms.DeleteAliasInput{AliasName: "alias/ghost"})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrAliasNotFound)
}

func TestListAliases_FilterByKeyID_ReturnsOnlyMatching(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out1, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	out2, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	key1 := out1.KeyMetadata.KeyID
	key2 := out2.KeyMetadata.KeyID

	require.NoError(
		t,
		b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: "alias/key1-a", TargetKeyID: key1}),
	)
	require.NoError(
		t,
		b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: "alias/key1-b", TargetKeyID: key1}),
	)
	require.NoError(
		t,
		b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: "alias/key2-a", TargetKeyID: key2}),
	)

	aliases, err := b.ListAliases(context.Background(), &kms.ListAliasesInput{KeyID: key1})
	require.NoError(t, err)
	assert.Len(t, aliases.Aliases, 2)
	for _, a := range aliases.Aliases {
		assert.Equal(t, key1, a.TargetKeyID)
	}
}

func TestListAliases_NoFilter_ReturnsAll(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out1, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	out2, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})

	require.NoError(
		t,
		b.CreateAlias(
			context.Background(),
			&kms.CreateAliasInput{AliasName: "alias/one", TargetKeyID: out1.KeyMetadata.KeyID},
		),
	)
	require.NoError(
		t,
		b.CreateAlias(
			context.Background(),
			&kms.CreateAliasInput{AliasName: "alias/two", TargetKeyID: out2.KeyMetadata.KeyID},
		),
	)

	aliases, err := b.ListAliases(context.Background(), &kms.ListAliasesInput{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(aliases.Aliases), 2)
}

func TestListAliases_NonMatchingKeyID_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	key1 := out.KeyMetadata.KeyID
	require.NoError(
		t,
		b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: "alias/unrelated", TargetKeyID: key1}),
	)

	out2, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	key2 := out2.KeyMetadata.KeyID // no aliases

	aliases, err := b.ListAliases(context.Background(), &kms.ListAliasesInput{KeyID: key2})
	require.NoError(t, err)
	assert.Empty(t, aliases.Aliases)
}

func TestListAliases_Pagination(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	for i := range 5 {
		name := fmt.Sprintf("alias/pagtest-%d", i)
		require.NoError(
			t,
			b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: name, TargetKeyID: keyID}),
		)
	}

	limit := int32(2)
	first, err := b.ListAliases(context.Background(), &kms.ListAliasesInput{Limit: &limit})
	require.NoError(t, err)
	assert.Len(t, first.Aliases, 2)
	assert.True(t, first.Truncated)
	assert.NotEmpty(t, first.NextMarker)

	second, err := b.ListAliases(context.Background(), &kms.ListAliasesInput{Limit: &limit, Marker: first.NextMarker})
	require.NoError(t, err)
	assert.NotEmpty(t, second.Aliases)
}

// TestListAliases_DefaultLimit_Is50 verifies the documented default page
// size when Limit is omitted: aws-sdk-go-v2/service/kms's
// ListAliasesInput.Limit doc comment says "If you do not include a value, it
// defaults to 50" (max 100) -- distinct from ListKeys/ListKeyPolicies/
// ListKeyRotations/DescribeCustomKeyStores, whose documented default is 100.
func TestListAliases_DefaultLimit_Is50(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	for i := range 51 {
		name := fmt.Sprintf("alias/deflimit-%d", i)
		require.NoError(
			t,
			b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: name, TargetKeyID: keyID}),
		)
	}

	page, err := b.ListAliases(context.Background(), &kms.ListAliasesInput{})
	require.NoError(t, err)
	assert.Len(t, page.Aliases, 50)
	assert.True(t, page.Truncated)
	assert.Equal(t, "50", page.NextMarker)
}

func TestListAliases_ReturnsCreationAndUpdateDates(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID
	require.NoError(
		t,
		b.CreateAlias(context.Background(), &kms.CreateAliasInput{AliasName: "alias/dated", TargetKeyID: keyID}),
	)

	aliases, err := b.ListAliases(context.Background(), &kms.ListAliasesInput{})
	require.NoError(t, err)
	require.NotEmpty(t, aliases.Aliases)
	for _, a := range aliases.Aliases {
		if a.AliasName == "alias/dated" {
			assert.Greater(t, a.CreationDate, float64(0))
		}
	}
}

// TestKMSBackendAliases verifies alias create, list, and delete.
func TestKMSBackendAliases(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()
	key, _ := backend.CreateKey(context.Background(), &kms.CreateKeyInput{})

	err := backend.CreateAlias(context.Background(), &kms.CreateAliasInput{
		AliasName:   "alias/myalias",
		TargetKeyID: key.KeyMetadata.KeyID,
	})
	require.NoError(t, err)

	// Duplicate should fail
	err2 := backend.CreateAlias(context.Background(), &kms.CreateAliasInput{
		AliasName:   "alias/myalias",
		TargetKeyID: key.KeyMetadata.KeyID,
	})
	require.ErrorIs(t, err2, kms.ErrAliasAlreadyExists)

	// ListAliases by key
	listOut, err := backend.ListAliases(context.Background(), &kms.ListAliasesInput{KeyID: key.KeyMetadata.KeyID})
	require.NoError(t, err)
	assert.Len(t, listOut.Aliases, 1)
	assert.Equal(t, "alias/myalias", listOut.Aliases[0].AliasName)

	// Delete alias
	err = backend.DeleteAlias(context.Background(), &kms.DeleteAliasInput{AliasName: "alias/myalias"})
	require.NoError(t, err)

	// Delete again should fail
	err2 = backend.DeleteAlias(context.Background(), &kms.DeleteAliasInput{AliasName: "alias/myalias"})
	require.ErrorIs(t, err2, kms.ErrAliasNotFound)
}

// TestKMSCreateAliasKeyNotFound tests CreateAlias with nonexistent target key.
func TestKMSCreateAliasKeyNotFound(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()

	err := backend.CreateAlias(context.Background(), &kms.CreateAliasInput{
		AliasName:   "alias/no-key",
		TargetKeyID: "nonexistent-key-id",
	})
	require.ErrorIs(t, err, kms.ErrKeyNotFound)
}

// TestKMSListAliasesWithAliasFilter tests ListAliases with an alias as the key ID.
func TestKMSListAliasesWithAliasFilter(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()
	key, _ := backend.CreateKey(context.Background(), &kms.CreateKeyInput{})
	_ = backend.CreateAlias(context.Background(), &kms.CreateAliasInput{
		AliasName:   "alias/lookup-test",
		TargetKeyID: key.KeyMetadata.KeyID,
	})

	// Filter using the alias name itself
	out, err := backend.ListAliases(context.Background(), &kms.ListAliasesInput{
		KeyID: "alias/lookup-test",
	})
	require.NoError(t, err)
	assert.Len(t, out.Aliases, 1)
}

// TestKMSUpdateAlias verifies that UpdateAlias redirects an alias to a new key.
func TestKMSUpdateAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		setupFn func(b *kms.InMemoryBackend) (aliasName, firstKeyID, targetKeyID string)
		name    string
	}{
		{
			name: "redirect_alias_to_new_key",
			setupFn: func(b *kms.InMemoryBackend) (string, string, string) {
				k1, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)
				k2, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)
				require.NoError(t, b.CreateAlias(context.Background(), &kms.CreateAliasInput{
					AliasName:   "alias/redirect-test",
					TargetKeyID: k1.KeyMetadata.KeyID,
				}))

				return "alias/redirect-test", k1.KeyMetadata.KeyID, k2.KeyMetadata.KeyID
			},
		},
		{
			name: "update_nonexistent_alias_fails",
			setupFn: func(b *kms.InMemoryBackend) (string, string, string) {
				k, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)

				return "alias/does-not-exist", "", k.KeyMetadata.KeyID
			},
			wantErr: kms.ErrAliasNotFound,
		},
		{
			name: "update_to_nonexistent_key_fails",
			setupFn: func(b *kms.InMemoryBackend) (string, string, string) {
				k, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)
				require.NoError(t, b.CreateAlias(context.Background(), &kms.CreateAliasInput{
					AliasName:   "alias/update-bad-target",
					TargetKeyID: k.KeyMetadata.KeyID,
				}))

				return "alias/update-bad-target", k.KeyMetadata.KeyID, "nonexistent-key-id"
			},
			wantErr: kms.ErrKeyNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			aliasName, _, targetKeyID := tt.setupFn(b)

			err := b.UpdateAlias(context.Background(), &kms.UpdateAliasInput{
				AliasName:   aliasName,
				TargetKeyID: targetKeyID,
			})

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			// Verify the alias now points to the new target.
			aliases, listErr := b.ListAliases(context.Background(), &kms.ListAliasesInput{KeyID: targetKeyID})
			require.NoError(t, listErr)
			require.Len(t, aliases.Aliases, 1)
			assert.Equal(t, aliasName, aliases.Aliases[0].AliasName)
		})
	}
}

func TestCreateAlias_InvalidCharacters_Rejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		aliasName string
	}{
		{"exclamation", "alias/my!key"},
		{"at-sign", "alias/my@key"},
		{"hash", "alias/my#key"},
		{"dollar", "alias/my$key"},
		{"space", "alias/my key"},
		{"tab", "alias/my\tkey"},
		{"newline", "alias/my\nkey"},
		{"dot", "alias/my.key"},
		{"asterisk", "alias/my*key"},
		{"open-paren", "alias/my(key"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := ops2NewBackend(t)
			keyID := ops2MustCreateSymKey(t, b)

			err := b.CreateAlias(context.Background(), &kms.CreateAliasInput{
				AliasName:   tc.aliasName,
				TargetKeyID: keyID,
			})
			require.Error(t, err, "alias name %q should be rejected", tc.aliasName)
			assert.ErrorIs(t, err, kms.ErrInvalidAliasName,
				"expected InvalidAliasNameException for %q", tc.aliasName)
		})
	}
}

func TestCreateAlias_ValidCharacters_Accepted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		aliasName string
	}{
		{"letters", "alias/myKey"},
		{"digits", "alias/key123"},
		{"hyphen", "alias/my-key"},
		{"underscore", "alias/my_key"},
		{"colon", "alias/my:key"},
		{"nested-slash", "alias/team/mykey"},
		{"mixed", "alias/prod_us-east-1:kms/key-01"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := ops2NewBackend(t)
			keyID := ops2MustCreateSymKey(t, b)

			err := b.CreateAlias(context.Background(), &kms.CreateAliasInput{
				AliasName:   tc.aliasName,
				TargetKeyID: keyID,
			})
			assert.NoError(t, err, "alias name %q should be accepted", tc.aliasName)
		})
	}
}

// TestListAliases_LimitBound verifies ListAliases enforces the same 1–1000 bound.
func TestListAliases_LimitBound(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	over := int32(1001)
	_, err := b.ListAliases(context.Background(), &kms.ListAliasesInput{Limit: &over})
	require.ErrorIs(t, err, kms.ErrValidation)

	ok := int32(50)
	_, err = b.ListAliases(context.Background(), &kms.ListAliasesInput{Limit: &ok})
	require.NoError(t, err)
}

func TestCreateAliasValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		aliasName string
		wantErr   bool
	}{
		{name: "valid", aliasName: "alias/my-key", wantErr: false},
		{name: "no_prefix", aliasName: "my-key", wantErr: true},
		{name: "aws_reserved", aliasName: "alias/aws/s3", wantErr: true},
		{
			name:      "too_long",
			aliasName: "alias/" + strings.Repeat("a", 251),
			wantErr:   true,
		},
		{
			name:      "at_max_length",
			aliasName: "alias/" + strings.Repeat("a", 250),
			wantErr:   false,
		},
		{name: "with_space", aliasName: "alias/my key", wantErr: true},
		{name: "with_tab", aliasName: "alias/my\tkey", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
			require.NoError(t, err)

			createErr := b.CreateAlias(context.Background(), &kms.CreateAliasInput{
				AliasName:   tt.aliasName,
				TargetKeyID: key.KeyMetadata.KeyID,
			})

			if tt.wantErr {
				require.Error(t, createErr)
			} else {
				require.NoError(t, createErr)
			}
		})
	}
}

func TestEncryptViaAliasARN(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)

	err = b.CreateAlias(context.Background(), &kms.CreateAliasInput{
		AliasName:   "alias/my-arn-test",
		TargetKeyID: key.KeyMetadata.KeyID,
	})
	require.NoError(t, err)

	// Build an ARN for the alias.
	aliasARN := "arn:aws:kms:us-east-1:000000000000:alias/my-arn-test"

	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:     aliasARN,
		Plaintext: []byte("alias-arn"),
	})
	require.NoError(t, err)
	require.NotNil(t, enc)
}

func TestHandlerResetClearsAliasCache(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	h := kms.NewHandler(b)

	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)

	err = b.CreateAlias(context.Background(), &kms.CreateAliasInput{
		AliasName:   "alias/to-be-cleared",
		TargetKeyID: key.KeyMetadata.KeyID,
	})
	require.NoError(t, err)

	// Resolve alias to populate the cache.
	_, err = b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:     "alias/to-be-cleared",
		Plaintext: []byte("pre-reset"),
	})
	require.NoError(t, err)

	// Reset clears everything including the resolution cache.
	h.Reset()

	// After reset the alias should no longer exist.
	_, err = b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:     "alias/to-be-cleared",
		Plaintext: []byte("post-reset"),
	})
	require.Error(t, err)
}

// TestCreateAlias_InvalidName_WireErrorType verifies the wire __type for a
// malformed alias name is InvalidAliasNameException, not ValidationException.
//
// CreateAlias's own deserializeOpError (aws-sdk-go-v2/service/kms@v1.55.4
// deserializers.go) recognizes AlreadyExistsException, DependencyTimeoutException,
// InvalidAliasNameException, KMSInternalException, KMSInvalidStateException,
// LimitExceededException and NotFoundException -- no ValidationException case,
// and "ValidationException" names no type anywhere in this SDK version.
func TestCreateAlias_InvalidName_WireErrorType(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)

	rec := doKMSRequest(t, h, "CreateAlias",
		`{"AliasName":"not-prefixed","TargetKeyId":"whatever"}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var body struct {
		Type string `json:"__type"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "InvalidAliasNameException", body.Type)
	assert.NotEqual(t, "ValidationException", body.Type)
}
