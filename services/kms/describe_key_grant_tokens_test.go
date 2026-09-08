package kms_test

// describe_key_grant_tokens_test.go — wire-shape regression for DescribeKeyInput.GrantTokens.
//
// The real aws-sdk-go-v2 DescribeKeyInput (kms@v1.54.0 AND v1.55.4, both checked directly)
// carries GrantTokens []string, so the field itself is real and must round-trip. But
// DescribeKey's deserializeOpError declares only DependencyTimeoutException,
// InvalidArnException, KMSInternalException, NotFoundException in both versions -- no
// InvalidGrantTokenException, unlike its sibling grant ops (Sign/Verify/GetPublicKey/
// GenerateMac/VerifyMac/DeriveSharedSecret, which do declare it). gopherstack-k3ww:
// the 2026-07-12 entry that added validateGrantTokenPresence here cited v1.54.0 as
// declaring the code; that citation was wrong, not SDK drift (v1.54.0 was re-checked
// directly and matches v1.55.4 exactly). Reverted the validation call; the field stays
// wire-accurate but unvalidated, since AWS's own declared error set gives it nothing to
// reject with.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

func TestDescribeKey_GrantTokens_WireField(t *testing.T) {
	t.Parallel()

	// The field must deserialize from the AWS wire name "GrantTokens".
	var input kms.DescribeKeyInput
	require.NoError(t, json.Unmarshal(
		[]byte(`{"KeyId":"k-123","GrantTokens":["tok-a","tok-b"]}`), &input))
	assert.Equal(t, "k-123", input.KeyID)
	assert.Equal(t, []string{"tok-a", "tok-b"}, input.GrantTokens)
}

// TestDescribeKey_GrantTokens_NotValidated pins gopherstack-k3ww: DescribeKey does not
// declare InvalidGrantTokenException, so a bogus GrantTokens entry must NOT be rejected --
// unlike Sign/Verify/GetPublicKey/GenerateMac/VerifyMac/DeriveSharedSecret, which do
// declare it and legitimately reject a bogus token via the same validateGrantTokenPresence
// helper.
func TestDescribeKey_GrantTokens_NotValidated(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := kms.NewInMemoryBackend()

	created, err := b.CreateKey(ctx, &kms.CreateKeyInput{Description: "grant-token-desc"})
	require.NoError(t, err)
	keyID := created.KeyMetadata.KeyID

	grant, err := b.CreateGrant(ctx, &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::000000000000:role/app",
		Operations:       []string{"DescribeKey"},
	})
	require.NoError(t, err)

	tests := []struct {
		name   string
		tokens []string
	}{
		{name: "no tokens", tokens: nil},
		{name: "real grant token", tokens: []string{grant.GrantToken}},
		{name: "bogus grant token", tokens: []string{"not-a-real-token"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, describeErr := b.DescribeKey(ctx, &kms.DescribeKeyInput{
				KeyID:       keyID,
				GrantTokens: tc.tokens,
			})

			require.NoError(t, describeErr)
			assert.Equal(t, keyID, out.KeyMetadata.KeyID)
		})
	}
}
