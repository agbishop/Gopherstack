package kms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

func TestKeyPolicy_PutGet(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	policy := `{"Version":"2012-10-17","Statement":[]}`
	err := b.PutKeyPolicy(context.Background(), &kms.PutKeyPolicyInput{
		KeyID:      keyID,
		PolicyName: "default",
		Policy:     policy,
	})
	require.NoError(t, err)

	got, err := b.GetKeyPolicy(context.Background(), &kms.GetKeyPolicyInput{KeyID: keyID, PolicyName: "default"})
	require.NoError(t, err)
	assert.Equal(t, policy, got.Policy)
}

func TestKeyPolicy_ListKeyPolicies(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	list, err := b.ListKeyPolicies(context.Background(), &kms.ListKeyPoliciesInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Contains(t, list.PolicyNames, "default")
}

// TestKeyPolicy_InvalidPolicyName is a regression test for gopherstack-i4q8:
// PutKeyPolicy's own PolicyName check (defense in depth; the handler already
// rejects this before reaching the backend, see
// TestHandler_PutKeyPolicy_InvalidPolicyName_ViaHTTP) must return
// UnsupportedOperationException, not the fabricated ValidationException.
func TestKeyPolicy_InvalidPolicyName(t *testing.T) {
	t.Parallel()
	b := newBackend(t)
	keyID := mustCreateSymKey(t, b)

	err := b.PutKeyPolicy(context.Background(), &kms.PutKeyPolicyInput{
		KeyID:      keyID,
		PolicyName: "custom",
		Policy:     `{}`,
	})
	require.ErrorIs(t, err, kms.ErrUnsupportedParameter)
	assert.NotErrorIs(t, err, kms.ErrValidation)
}

func TestPutKeyPolicy_AndGetKeyPolicy_Roundtrip(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"kms:*","Resource":"*"}]}`
	require.NoError(t, b.PutKeyPolicy(context.Background(), &kms.PutKeyPolicyInput{
		KeyID:      keyID,
		PolicyName: "default",
		Policy:     policy,
	}))

	policyOut, err := b.GetKeyPolicy(context.Background(), &kms.GetKeyPolicyInput{
		KeyID:      keyID,
		PolicyName: "default",
	})
	require.NoError(t, err)
	assert.Equal(t, policy, policyOut.Policy)
}

// TestKMSBackendGetKeyPolicyDefaultAndCustom verifies GetKeyPolicy returns default policy.
func TestKMSBackendGetKeyPolicyDefaultAndCustom(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := keyOut.KeyMetadata.KeyID

	// Default policy
	out, err := b.GetKeyPolicy(context.Background(), &kms.GetKeyPolicyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.NotEmpty(t, out.Policy)
	assert.Equal(t, "default", out.PolicyName)

	// Custom policy
	customPolicy := `{"Version":"2012-10-17","Statement":[]}`
	require.NoError(t, b.PutKeyPolicy(context.Background(), &kms.PutKeyPolicyInput{
		KeyID:  keyID,
		Policy: customPolicy,
	}))
	out2, err := b.GetKeyPolicy(context.Background(), &kms.GetKeyPolicyInput{
		KeyID:      keyID,
		PolicyName: "custom",
	})
	require.NoError(t, err)
	assert.Equal(t, customPolicy, out2.Policy)
	assert.Equal(t, "custom", out2.PolicyName)
}

// TestKMSBackendPutKeyPolicyNotFound verifies PutKeyPolicy fails for missing keys.
func TestKMSBackendPutKeyPolicyNotFound(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	err := b.PutKeyPolicy(context.Background(), &kms.PutKeyPolicyInput{
		KeyID:  "non-existent",
		Policy: `{"Version":"2012-10-17", "Statement":[]}`,
	})
	require.ErrorIs(t, err, kms.ErrKeyNotFound)
}

// TestKMSBackendPutKeyPolicyValidation verifies PutKeyPolicy validates the policy JSON.
func TestKMSBackendPutKeyPolicyValidation(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)

	tests := []struct {
		name   string
		policy string
	}{
		{"invalid json", `{"Version": "2012-10-17", "State`},
		{"missing version", `{"Statement": []}`},
		{"missing statement", `{"Version": "2012-10-17"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotErr := b.PutKeyPolicy(context.Background(), &kms.PutKeyPolicyInput{
				KeyID:  key.KeyMetadata.KeyID,
				Policy: tc.policy,
			})
			require.ErrorIs(t, gotErr, kms.ErrMalformedPolicyDocument)
		})
	}
}
