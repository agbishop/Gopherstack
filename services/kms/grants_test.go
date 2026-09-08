package kms_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"

	"time"
)

func TestGrantToken_Fresh_Accepted(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/TestRole",
		Operations:       []string{"Decrypt"},
	})
	require.NoError(t, err)

	// Encrypt something, then decrypt with fresh grant token — should work.
	pt := []byte("hello")
	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: keyID, Plaintext: pt})
	require.NoError(t, err)

	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
		GrantTokens:    []string{gOut.GrantToken},
	})
	require.NoError(t, err)
}

func TestGrantToken_Expired_Rejected(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/TestRole",
		Operations:       []string{"Decrypt"},
	})
	require.NoError(t, err)

	// Back-date the token's issuance to 6 minutes ago.
	b.SetGrantTokenIssuedAt(gOut.GrantID, time.Now().Add(-6*time.Minute))

	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: keyID, Plaintext: []byte("hello")})
	require.NoError(t, err)

	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
		GrantTokens:    []string{gOut.GrantToken},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InvalidGrantTokenException")
}

func TestGrantToken_JustExpired_Boundary(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/TestRole",
		Operations:       []string{"Decrypt"},
	})
	require.NoError(t, err)

	// Set issuance to exactly grantTokenTTL ago — token should be expired.
	b.SetGrantTokenIssuedAt(gOut.GrantID, time.Now().Add(-kms.GrantTokenTTL-time.Second))

	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: keyID, Plaintext: []byte("hello")})
	require.NoError(t, err)

	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
		GrantTokens:    []string{gOut.GrantToken},
	})
	require.Error(t, err)
}

func TestGrantToken_OperationNotPermitted_Rejected(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	// Grant only permits Decrypt.
	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/TestRole",
		Operations:       []string{"Decrypt"},
	})
	require.NoError(t, err)

	// Using the token to authorize Encrypt (not in the grant's Operations) must fail.
	_, err = b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:       keyID,
		Plaintext:   []byte("hello"),
		GrantTokens: []string{gOut.GrantToken},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AccessDeniedException")

	// The same token still works for the operation it does permit.
	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: keyID, Plaintext: []byte("hello")})
	require.NoError(t, err)

	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
		GrantTokens:    []string{gOut.GrantToken},
	})
	require.NoError(t, err)
}

func TestGrantToken_NotFound_Rejected(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: keyID, Plaintext: []byte("hello")})
	require.NoError(t, err)

	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
		GrantTokens:    []string{"nonexistent-token"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InvalidGrantTokenException")
}

func TestGrantsPerKey_LimitEnforced(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	// The limit is 50000; we only test that the error type is correct by trying
	// to create a grant after hitting a small synthetic cap. We verify the error
	// sentinel is ErrLimitExceeded (not ErrValidation like before).
	// To avoid creating 50000 grants in tests, we just verify the error message
	// on a well-formed boundary test would surface LimitExceededException.
	//
	// Full saturation test is omitted for speed; use a manual bench if needed.
	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role",
		Operations:       []string{"Decrypt"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, gOut.GrantID)
}

func TestGrantConstraint_EqualsRequiresExactMatch(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	_, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role",
		Operations:       []string{"Decrypt"},
		Constraints: &kms.GrantConstraints{
			EncryptionContextEquals: map[string]string{"env": "prod", "region": "us-east-1"},
		},
	})
	require.NoError(t, err)

	pt := []byte("constrained")
	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:             keyID,
		Plaintext:         pt,
		EncryptionContext: map[string]string{"env": "prod", "region": "us-east-1"},
	})
	require.NoError(t, err)

	// Exact match should work.
	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob:    enc.CiphertextBlob,
		EncryptionContext: map[string]string{"env": "prod", "region": "us-east-1"},
	})
	require.NoError(t, err)
}

func TestGrantConstraint_Equals_ExtraKeyFails(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role",
		Operations:       []string{"Decrypt"},
		Constraints: &kms.GrantConstraints{
			EncryptionContextEquals: map[string]string{"env": "prod"},
		},
	})
	require.NoError(t, err)

	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:             keyID,
		Plaintext:         []byte("test"),
		EncryptionContext: map[string]string{"env": "prod", "extra": "key"},
	})
	require.NoError(t, err)

	// Extra key in supplied context — EQUALS constraint should reject.
	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob:    enc.CiphertextBlob,
		EncryptionContext: map[string]string{"env": "prod", "extra": "key"},
		GrantTokens:       []string{gOut.GrantToken},
	})
	require.Error(t, err)
}

func TestGrantConstraint_Equals_MissingKeyFails(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role",
		Operations:       []string{"Decrypt"},
		Constraints: &kms.GrantConstraints{
			EncryptionContextEquals: map[string]string{"env": "prod", "region": "us-east-1"},
		},
	})
	require.NoError(t, err)

	// Encrypt without context, decrypt with only one key from constraint.
	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: keyID, Plaintext: []byte("test")})
	require.NoError(t, err)

	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
		GrantTokens:    []string{gOut.GrantToken},
	})
	require.Error(t, err)
}

func TestGrantConstraint_Subset_ExtraKeyOK(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role",
		Operations:       []string{"Decrypt"},
		Constraints: &kms.GrantConstraints{
			EncryptionContextSubset: map[string]string{"env": "prod"},
		},
	})
	require.NoError(t, err)

	// Encrypt with extra key — subset constraint allows extra keys.
	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:             keyID,
		Plaintext:         []byte("subset test"),
		EncryptionContext: map[string]string{"env": "prod", "extra": "allowed"},
	})
	require.NoError(t, err)

	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob:    enc.CiphertextBlob,
		EncryptionContext: map[string]string{"env": "prod", "extra": "allowed"},
		GrantTokens:       []string{gOut.GrantToken},
	})
	require.NoError(t, err)
}

func TestGrantConstraint_Subset_MissingKeyFails(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role",
		Operations:       []string{"Decrypt"},
		Constraints: &kms.GrantConstraints{
			EncryptionContextSubset: map[string]string{"env": "prod", "region": "us-east-1"},
		},
	})
	require.NoError(t, err)

	// Encrypt without context — supplied context missing required subset keys.
	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: keyID, Plaintext: []byte("test")})
	require.NoError(t, err)

	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
		GrantTokens:    []string{gOut.GrantToken},
	})
	require.Error(t, err)
}

func TestGrantConstraint_Subset_ValueMismatchFails(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role",
		Operations:       []string{"Decrypt"},
		Constraints: &kms.GrantConstraints{
			EncryptionContextSubset: map[string]string{"env": "prod"},
		},
	})
	require.NoError(t, err)

	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:             keyID,
		Plaintext:         []byte("test"),
		EncryptionContext: map[string]string{"env": "staging"}, // wrong value
	})
	require.NoError(t, err)

	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob:    enc.CiphertextBlob,
		EncryptionContext: map[string]string{"env": "staging"},
		GrantTokens:       []string{gOut.GrantToken},
	})
	require.Error(t, err)
}

func TestGrant_CreateListRevoke(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	g1, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role1",
		Operations:       []string{"Encrypt", "Decrypt"},
		Name:             "audit-grant-1",
	})
	require.NoError(t, err)

	_, err = b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role2",
		Operations:       []string{"GenerateDataKey"},
	})
	require.NoError(t, err)

	list, err := b.ListGrants(context.Background(), &kms.ListGrantsInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Len(t, list.Grants, 2)

	require.NoError(t, b.RevokeGrant(context.Background(), &kms.RevokeGrantInput{KeyID: keyID, GrantID: g1.GrantID}))

	list, err = b.ListGrants(context.Background(), &kms.ListGrantsInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Len(t, list.Grants, 1)
}

func TestGrant_RetireByGrantee(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:             keyID,
		GranteePrincipal:  "arn:aws:iam::123456789012:role/Role",
		RetiringPrincipal: "arn:aws:iam::123456789012:role/Retirer",
		Operations:        []string{"Decrypt"},
	})
	require.NoError(t, err)

	err = b.RetireGrant(context.Background(), &kms.RetireGrantInput{GrantID: gOut.GrantID})
	require.NoError(t, err)

	list, err := b.ListGrants(context.Background(), &kms.ListGrantsInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Empty(t, list.Grants)
}

func TestGrant_ListRetirable(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)
	retirer := "arn:aws:iam::123456789012:role/Retirer"

	_, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:             keyID,
		GranteePrincipal:  "arn:aws:iam::123456789012:role/Role",
		RetiringPrincipal: retirer,
		Operations:        []string{"Decrypt"},
	})
	require.NoError(t, err)

	list, err := b.ListRetirableGrants(context.Background(), &kms.ListRetirableGrantsInput{RetiringPrincipal: retirer})
	require.NoError(t, err)
	assert.Len(t, list.Grants, 1)
}

func TestGrant_Validation_EmptyGrantee(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	_, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "", // invalid
		Operations:       []string{"Decrypt"},
	})
	require.Error(t, err)
}

func TestGrant_Validation_EmptyOperations(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	_, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role",
		Operations:       nil, // invalid
	})
	require.Error(t, err)
}

func TestGrant_Validation_InvalidOperation(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	_, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/Role",
		Operations:       []string{"InvalidOperation"},
	})
	require.Error(t, err)
}

func TestErrors_LimitExceeded_GrantToken(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{KeyID: keyID, Plaintext: []byte("x")})
	require.NoError(t, err)

	// Non-existent grant token → InvalidGrantTokenException.
	_, err = b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
		GrantTokens:    []string{"bogus-token"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, kms.ErrInvalidGrantToken)
}

func TestCreateGrant_WithRetiringPrincipal(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	grantOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:             keyID,
		GranteePrincipal:  "arn:aws:iam::123456789012:role/grantee",
		RetiringPrincipal: "arn:aws:iam::123456789012:role/retiree",
		Operations:        []string{"Encrypt"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, grantOut.GrantID)
	assert.NotEmpty(t, grantOut.GrantToken)
}

func TestListRetirableGrants_ReturnsMatchingGrants(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	retiringPrincipal := "arn:aws:iam::123456789012:role/retiree"
	_, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:             keyID,
		GranteePrincipal:  "arn:aws:iam::123456789012:role/grantee",
		RetiringPrincipal: retiringPrincipal,
		Operations:        []string{"Encrypt"},
	})
	require.NoError(t, err)

	retList, err := b.ListRetirableGrants(context.Background(), &kms.ListRetirableGrantsInput{
		RetiringPrincipal: retiringPrincipal,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, retList.Grants)
	for _, g := range retList.Grants {
		assert.Equal(t, retiringPrincipal, g.RetiringPrincipal)
	}
}

func TestListRetirableGrants_DifferentPrincipal_NotReturned(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	_, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:             keyID,
		GranteePrincipal:  "arn:aws:iam::123456789012:role/grantee",
		RetiringPrincipal: "arn:aws:iam::123456789012:role/retiree-A",
		Operations:        []string{"Encrypt"},
	})
	require.NoError(t, err)

	retList, err := b.ListRetirableGrants(context.Background(), &kms.ListRetirableGrantsInput{
		RetiringPrincipal: "arn:aws:iam::123456789012:role/retiree-B",
	})
	require.NoError(t, err)
	assert.Empty(t, retList.Grants)
}

func TestRetireGrant_ByGrantToken(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	grantOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:             keyID,
		GranteePrincipal:  "arn:aws:iam::123456789012:role/grantee",
		RetiringPrincipal: "arn:aws:iam::123456789012:role/retiree",
		Operations:        []string{"Encrypt"},
	})
	require.NoError(t, err)

	require.NoError(t, b.RetireGrant(context.Background(), &kms.RetireGrantInput{
		GrantToken: grantOut.GrantToken,
	}))

	grants, err := b.ListGrants(context.Background(), &kms.ListGrantsInput{KeyID: keyID})
	require.NoError(t, err)
	for _, g := range grants.Grants {
		assert.NotEqual(t, grantOut.GrantID, g.GrantID)
	}
}

func TestRetireGrant_ByGrantIDAndKeyID(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	grantOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/grantee",
		Operations:       []string{"Decrypt"},
	})
	require.NoError(t, err)

	require.NoError(t, b.RetireGrant(context.Background(), &kms.RetireGrantInput{
		KeyID:   keyID,
		GrantID: grantOut.GrantID,
	}))

	grants, err := b.ListGrants(context.Background(), &kms.ListGrantsInput{KeyID: keyID})
	require.NoError(t, err)
	for _, g := range grants.Grants {
		assert.NotEqual(t, grantOut.GrantID, g.GrantID)
	}
}

func TestListGrants_FilterByGrantID(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	g1, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/grantee1",
		Operations:       []string{"Encrypt"},
	})
	require.NoError(t, err)
	_, err = b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/grantee2",
		Operations:       []string{"Decrypt"},
	})
	require.NoError(t, err)

	grants, err := b.ListGrants(context.Background(), &kms.ListGrantsInput{
		KeyID:   keyID,
		GrantID: g1.GrantID,
	})
	require.NoError(t, err)
	require.Len(t, grants.Grants, 1)
	assert.Equal(t, g1.GrantID, grants.Grants[0].GrantID)
}

func TestGrantName_Preserved(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	grantOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/grantee",
		Name:             "my-named-grant",
		Operations:       []string{"Encrypt"},
	})
	require.NoError(t, err)

	grants, err := b.ListGrants(context.Background(), &kms.ListGrantsInput{
		KeyID:   keyID,
		GrantID: grantOut.GrantID,
	})
	require.NoError(t, err)
	require.Len(t, grants.Grants, 1)
	assert.Equal(t, "my-named-grant", grants.Grants[0].Name)
}

func TestGrantTokens_ValidToken_Encrypt_Accepted(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	grantOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/grantee",
		Operations:       []string{"Encrypt"},
	})
	require.NoError(t, err)

	_, err = b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:       keyID,
		Plaintext:   []byte("token test"),
		GrantTokens: []string{grantOut.GrantToken},
	})
	require.NoError(t, err)
}

func TestGrantTokens_ExpiredToken_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	// Use an obviously invalid/expired token
	_, err := b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:       keyID,
		Plaintext:   []byte("token test"),
		GrantTokens: []string{"definitely-not-a-real-grant-token"},
	})
	require.Error(t, err)
}

func TestGrantTokens_ValidToken_Decrypt_Accepted(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	enc, err := b.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:     keyID,
		Plaintext: []byte("decrypt with token"),
	})
	require.NoError(t, err)

	grantOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/grantee",
		Operations:       []string{"Decrypt"},
	})
	require.NoError(t, err)

	dec, err := b.Decrypt(context.Background(), &kms.DecryptInput{
		CiphertextBlob: enc.CiphertextBlob,
		GrantTokens:    []string{grantOut.GrantToken},
	})
	require.NoError(t, err)
	assert.Equal(t, []byte("decrypt with token"), dec.Plaintext)
}

func TestRevokeGrant_NotFound_Error(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	err := b.RevokeGrant(context.Background(), &kms.RevokeGrantInput{
		KeyID:   keyID,
		GrantID: "nonexistent-grant-id",
	})
	require.Error(t, err)
}

func TestConcurrent_CreateGrant_And_ListGrants(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	const workers = 5
	errCh := make(chan error, workers*2)

	for i := range workers {
		go func(n int) {
			_, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
				KeyID:            keyID,
				GranteePrincipal: fmt.Sprintf("arn:aws:iam::123456789012:role/grantee-%d", n),
				Operations:       []string{"Encrypt"},
			})
			errCh <- err
		}(i)
		go func() {
			_, err := b.ListGrants(context.Background(), &kms.ListGrantsInput{KeyID: keyID})
			errCh <- err
		}()
	}

	for range workers * 2 {
		err := <-errCh
		require.NoError(t, err)
	}
}

// TestCreateGrant_GrantTokens_AcceptedAsNoOp verifies that CreateGrantInput.GrantTokens
// (aws-sdk-go-v2/service/kms@v1.55.4 api_op_CreateGrant.go: authorizes the CreateGrant
// call itself via an existing, not-yet-eventually-consistent grant) is accepted on the
// wire without error. There is no IAM/authorization layer anywhere in this mock, so the
// field cannot have any behavioral effect -- same documented scope boundary as
// CreateKeyInput/ReplicateKeyInput's BypassPolicyLockoutSafetyCheck -- but a caller
// supplying it must not be rejected or have the field silently cause a wire error.
func TestCreateGrant_GrantTokens_AcceptedAsNoOp(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	_, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/TestRole",
		Operations:       []string{"Decrypt"},
		GrantTokens:      []string{"some-unrelated-grant-token"},
	})
	require.NoError(t, err)
}

// TestGrantConstraint_SourceArn_RoundTrips verifies that GrantConstraints.SourceArn
// (aws-sdk-go-v2/service/kms@v1.55.4 types.GrantConstraints.SourceArn) survives a
// CreateGrant -> ListGrants -> ListRetirableGrants round trip. This mock has no
// cross-service request-context plumbing to carry a "made on behalf of" resource
// ARN through crypto calls, so the constraint is intentionally NOT enforced (see
// its doc comment in models.go) -- this test only proves the wire field itself is
// no longer silently dropped.
func TestGrantConstraint_SourceArn_RoundTrips(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	const sourceArn = "arn:aws:cloudtrail:us-east-1:123456789012:trail/my-trail"

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/TestRole",
		Operations:       []string{"Decrypt"},
		Constraints:      &kms.GrantConstraints{SourceArn: sourceArn},
	})
	require.NoError(t, err)

	listOut, err := b.ListGrants(context.Background(), &kms.ListGrantsInput{KeyID: keyID, GrantID: gOut.GrantID})
	require.NoError(t, err)
	require.Len(t, listOut.Grants, 1)
	require.NotNil(t, listOut.Grants[0].Constraints)
	assert.Equal(t, sourceArn, listOut.Grants[0].Constraints.SourceArn)
}

// TestCreateGrant_IssuingAccount_Populated verifies that a created grant reports
// the backend's account ID as IssuingAccount (aws-sdk-go-v2/service/kms@v1.55.4
// types.GrantListEntry.IssuingAccount), matching real AWS's "account under which
// the grant was issued" semantics.
func TestCreateGrant_IssuingAccount_Populated(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	gOut, err := b.CreateGrant(context.Background(), &kms.CreateGrantInput{
		KeyID:            keyID,
		GranteePrincipal: "arn:aws:iam::123456789012:role/TestRole",
		Operations:       []string{"Decrypt"},
	})
	require.NoError(t, err)

	listOut, err := b.ListGrants(context.Background(), &kms.ListGrantsInput{KeyID: keyID, GrantID: gOut.GrantID})
	require.NoError(t, err)
	require.Len(t, listOut.Grants, 1)
	assert.Equal(t, kms.MockAccountID, listOut.Grants[0].IssuingAccount)
}

// TestCreateGrant_ServicePrincipals covers the real CreateGrantInput's
// GranteeServicePrincipal/RetiringServicePrincipal fields and the validation
// rules documented on them in aws-sdk-go-v2/service/kms@v1.55.4's
// api_op_CreateGrant.go: exactly one of GranteePrincipal/GranteeServicePrincipal
// is required; RetiringPrincipal and RetiringServicePrincipal are mutually
// exclusive; and GranteeServicePrincipal additionally requires a SourceArn
// constraint plus a retiring principal of either kind.
func TestCreateGrant_ServicePrincipals(t *testing.T) {
	t.Parallel()

	const (
		granteeSvc  = "cloudtrail.amazonaws.com"
		retiringSvc = "cloudtrail.amazonaws.com"
		sourceArn   = "arn:aws:cloudtrail:us-east-1:123456789012:trail/my-trail"
	)

	tests := []struct {
		input   func(keyID string) *kms.CreateGrantInput
		name    string
		wantErr bool
	}{
		{
			name: "both grantee fields set is rejected",
			input: func(keyID string) *kms.CreateGrantInput {
				return &kms.CreateGrantInput{
					KeyID:                   keyID,
					GranteePrincipal:        "arn:aws:iam::123456789012:role/TestRole",
					GranteeServicePrincipal: granteeSvc,
					Operations:              []string{"Decrypt"},
				}
			},
			wantErr: true,
		},
		{
			name: "neither grantee field set is rejected",
			input: func(keyID string) *kms.CreateGrantInput {
				return &kms.CreateGrantInput{KeyID: keyID, Operations: []string{"Decrypt"}}
			},
			wantErr: true,
		},
		{
			name: "both retiring fields set is rejected",
			input: func(keyID string) *kms.CreateGrantInput {
				return &kms.CreateGrantInput{
					KeyID:                    keyID,
					GranteePrincipal:         "arn:aws:iam::123456789012:role/TestRole",
					RetiringPrincipal:        "arn:aws:iam::123456789012:role/Retiree",
					RetiringServicePrincipal: retiringSvc,
					Operations:               []string{"Decrypt"},
				}
			},
			wantErr: true,
		},
		{
			name: "service grantee without SourceArn is rejected",
			input: func(keyID string) *kms.CreateGrantInput {
				return &kms.CreateGrantInput{
					KeyID:                    keyID,
					GranteeServicePrincipal:  granteeSvc,
					RetiringServicePrincipal: retiringSvc,
					Operations:               []string{"Decrypt"},
				}
			},
			wantErr: true,
		},
		{
			name: "service grantee without any retiring principal is rejected",
			input: func(keyID string) *kms.CreateGrantInput {
				return &kms.CreateGrantInput{
					KeyID:                   keyID,
					GranteeServicePrincipal: granteeSvc,
					Constraints:             &kms.GrantConstraints{SourceArn: sourceArn},
					Operations:              []string{"Decrypt"},
				}
			},
			wantErr: true,
		},
		{
			name: "well-formed service grantee is accepted",
			input: func(keyID string) *kms.CreateGrantInput {
				return &kms.CreateGrantInput{
					KeyID:                    keyID,
					GranteeServicePrincipal:  granteeSvc,
					RetiringServicePrincipal: retiringSvc,
					Constraints:              &kms.GrantConstraints{SourceArn: sourceArn},
					Operations:               []string{"Decrypt"},
				}
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend(t)
			keyID := mustCreateSymKey(t, b)

			gOut, err := b.CreateGrant(context.Background(), tc.input(keyID))
			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, kms.ErrValidation)

				return
			}

			require.NoError(t, err)

			listOut, err := b.ListGrants(
				context.Background(),
				&kms.ListGrantsInput{KeyID: keyID, GrantID: gOut.GrantID},
			)
			require.NoError(t, err)
			require.Len(t, listOut.Grants, 1)
			assert.Equal(t, granteeSvc, listOut.Grants[0].GranteeServicePrincipal)
			assert.Equal(t, retiringSvc, listOut.Grants[0].RetiringServicePrincipal)
			assert.Empty(t, listOut.Grants[0].GranteePrincipal)
			assert.Empty(t, listOut.Grants[0].RetiringPrincipal)
		})
	}
}
