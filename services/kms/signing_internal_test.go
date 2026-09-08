package kms_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"

	"strings"
)

// TestKMSSnapshotRestoreRSA3072 verifies snapshot/restore preserves RSA-3072 key material.
func TestKMSSnapshotRestoreRSA3072(t *testing.T) {
	t.Parallel()

	original := kms.NewInMemoryBackend()
	keyOut, err := original.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "RSA_3072",
	})
	require.NoError(t, err)

	msg := []byte("rsa-3072-persistence-test")
	signOut, err := original.Sign(context.Background(), &kms.SignInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          msg,
		MessageType:      "RAW",
		SigningAlgorithm: "RSASSA_PSS_SHA_384",
	})
	require.NoError(t, err)

	snap := original.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	restored := kms.NewInMemoryBackend()
	require.NoError(t, restored.Restore(t.Context(), snap))

	verifyOut, err := restored.Verify(context.Background(), &kms.VerifyInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          msg,
		MessageType:      "RAW",
		Signature:        signOut.Signature,
		SigningAlgorithm: "RSASSA_PSS_SHA_384",
	})
	require.NoError(t, err)
	assert.True(t, verifyOut.SignatureValid)
}

// TestKMSBackendGetPublicKeySymmetricFails verifies GetPublicKey on symmetric keys returns an error.
func TestKMSBackendGetPublicKeySymmetricFails(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{KeyUsage: kms.KeyUsageEncryptDecrypt})
	require.NoError(t, err)

	_, err = b.GetPublicKey(context.Background(), &kms.GetPublicKeyInput{KeyID: keyOut.KeyMetadata.KeyID})
	require.ErrorIs(t, err, kms.ErrInvalidKeyUsage)
}

// TestKMSBackendVerifyDisabledKey verifies that Verify fails on a disabled key.
func TestKMSBackendVerifyDisabledKey(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "ECC_NIST_P256",
	})
	require.NoError(t, err)
	keyID := keyOut.KeyMetadata.KeyID

	signOut, err := b.Sign(context.Background(), &kms.SignInput{
		KeyID:            keyID,
		Message:          []byte("test"),
		MessageType:      "RAW",
		SigningAlgorithm: "ECDSA_SHA_256",
	})
	require.NoError(t, err)

	require.NoError(t, b.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: keyID}))

	_, err = b.Verify(context.Background(), &kms.VerifyInput{
		KeyID:            keyID,
		Message:          []byte("test"),
		MessageType:      "RAW",
		Signature:        signOut.Signature,
		SigningAlgorithm: "ECDSA_SHA_256",
	})
	require.ErrorIs(t, err, kms.ErrKeyDisabled)
}

// TestKMSBackendSignDisabledKey verifies that Sign fails on a disabled key.
func TestKMSBackendSignDisabledKey(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "RSA_2048",
	})
	require.NoError(t, err)

	require.NoError(t, b.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: keyOut.KeyMetadata.KeyID}))

	_, err = b.Sign(context.Background(), &kms.SignInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          []byte("test"),
		MessageType:      "RAW",
		SigningAlgorithm: "RSASSA_PSS_SHA_256",
	})
	require.ErrorIs(t, err, kms.ErrKeyDisabled)
}

// TestKMSKeyMetadataSigningAlgorithms verifies that DescribeKey returns signing algorithms
// for asymmetric keys.
func TestKMSKeyMetadataSigningAlgorithms(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "RSA_2048",
	})
	require.NoError(t, err)

	descOut, err := b.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyOut.KeyMetadata.KeyID})
	require.NoError(t, err)
	assert.NotEmpty(t, descOut.KeyMetadata.SigningAlgorithms)
	assert.Contains(t, descOut.KeyMetadata.SigningAlgorithms, "RSASSA_PSS_SHA_256")
}

// TestKMSBackendSignUnsupportedAlgorithm verifies that unsupported signing algorithms return an error.
func TestKMSBackendSignUnsupportedAlgorithm(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "RSA_2048",
	})
	require.NoError(t, err)

	_, err = b.Sign(context.Background(), &kms.SignInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          []byte("test"),
		MessageType:      "RAW",
		SigningAlgorithm: "UNSUPPORTED_ALGORITHM",
	})
	require.Error(t, err)
}

// TestKMSBackendVerifyUnsupportedAlgorithm verifies that unsupported algorithms return an error.
func TestKMSBackendVerifyUnsupportedAlgorithm(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "RSA_2048",
	})
	require.NoError(t, err)

	_, err = b.Verify(context.Background(), &kms.VerifyInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          []byte("test"),
		MessageType:      "RAW",
		Signature:        []byte("sig"),
		SigningAlgorithm: "UNSUPPORTED_ALGORITHM",
	})
	require.Error(t, err)
}

// TestKMSBackendVerifyECDSAInvalidASN1 verifies that a non-ASN.1 ECDSA signature is rejected.
func TestKMSBackendVerifyECDSAInvalidASN1(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "ECC_NIST_P256",
	})
	require.NoError(t, err)

	// Provide a signature that is not valid ASN.1
	invalidSig := []byte("not-asn1-signature-data-at-all-!!!!")
	_, err = b.Verify(context.Background(), &kms.VerifyInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          []byte("test message"),
		MessageType:      "RAW",
		Signature:        invalidSig,
		SigningAlgorithm: "ECDSA_SHA_256",
	})
	require.ErrorIs(t, err, kms.ErrInvalidSignature)
}

// TestKMSBackendVerifyECDSAWrongSignature verifies that a well-formed but wrong ECDSA signature is rejected.
func TestKMSBackendVerifyECDSAWrongSignature(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "ECC_NIST_P256",
	})
	require.NoError(t, err)
	keyID := keyOut.KeyMetadata.KeyID

	// Sign one message
	signOut, err := b.Sign(context.Background(), &kms.SignInput{
		KeyID:            keyID,
		Message:          []byte("message-a"),
		MessageType:      "RAW",
		SigningAlgorithm: "ECDSA_SHA_256",
	})
	require.NoError(t, err)

	// Verify against a different message — should fail
	_, err = b.Verify(context.Background(), &kms.VerifyInput{
		KeyID:            keyID,
		Message:          []byte("message-b"),
		MessageType:      "RAW",
		Signature:        signOut.Signature,
		SigningAlgorithm: "ECDSA_SHA_256",
	})
	require.ErrorIs(t, err, kms.ErrInvalidSignature)
}

// TestKMSBackendSnapshotRestoreECDSA verifies snapshot/restore preserves ECDSA key material.
func TestKMSBackendSnapshotRestoreECDSA(t *testing.T) {
	t.Parallel()

	orig := kms.NewInMemoryBackend()
	keyOut, err := orig.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "ECC_NIST_P384",
	})
	require.NoError(t, err)

	msg := []byte("ecdsa-snapshot-test")
	signOut, err := orig.Sign(context.Background(), &kms.SignInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          msg,
		MessageType:      "RAW",
		SigningAlgorithm: "ECDSA_SHA_384",
	})
	require.NoError(t, err)

	snap := orig.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	restored := kms.NewInMemoryBackend()
	require.NoError(t, restored.Restore(t.Context(), snap))

	verifyOut, err := restored.Verify(context.Background(), &kms.VerifyInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          msg,
		MessageType:      "RAW",
		Signature:        signOut.Signature,
		SigningAlgorithm: "ECDSA_SHA_384",
	})
	require.NoError(t, err)
	assert.True(t, verifyOut.SignatureValid)
}

// TestKMSBackendGetPublicKeyDisabledKey verifies GetPublicKey fails on a disabled key.
func TestKMSBackendGetPublicKeyDisabledKey(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "ECC_NIST_P256",
	})
	require.NoError(t, err)

	require.NoError(t, b.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: keyOut.KeyMetadata.KeyID}))

	_, err = b.GetPublicKey(context.Background(), &kms.GetPublicKeyInput{KeyID: keyOut.KeyMetadata.KeyID})
	require.ErrorIs(t, err, kms.ErrKeyDisabled)
}

// TestKMSBackendGetPublicKeyNotFound verifies GetPublicKey returns ErrKeyNotFound.
func TestKMSBackendGetPublicKeyNotFound(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	_, err := b.GetPublicKey(context.Background(), &kms.GetPublicKeyInput{KeyID: "non-existent-key"})
	require.ErrorIs(t, err, kms.ErrKeyNotFound)
}

// TestKMSCreateKeyIncompatibleSpecUsage verifies that CreateKey fails when KeySpec
// and KeyUsage are incompatible (e.g. RSA key with ENCRYPT_DECRYPT usage).
func TestKMSCreateKeyIncompatibleSpecUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		keySpec  string
		keyUsage string
	}{
		{
			name:     "ECC_NIST_P256_with_ENCRYPT_DECRYPT",
			keySpec:  "ECC_NIST_P256",
			keyUsage: kms.KeyUsageEncryptDecrypt,
		},
		{
			name:     "SYMMETRIC_DEFAULT_with_SIGN_VERIFY",
			keySpec:  "SYMMETRIC_DEFAULT",
			keyUsage: kms.KeyUsageSignVerify,
		},
		{
			// The HMAC branch of validateKeySpecUsage had no coverage, so its
			// sentinel was unverified by any test (gopherstack-5rjn).
			name:     "HMAC_256_with_SIGN_VERIFY",
			keySpec:  "HMAC_256",
			keyUsage: kms.KeyUsageSignVerify,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			_, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
				KeySpec:  tt.keySpec,
				KeyUsage: tt.keyUsage,
			})
			// gopherstack-5rjn: CreateKey's declared error set has no
			// key-usage-shaped code (no InvalidKeyUsageException), so this
			// asserted the wrong sentinel; UnsupportedOperationException is
			// the code CreateKey actually declares and real AWS uses for a
			// KeySpec-related CreateKey rejection.
			require.ErrorIs(t, err, kms.ErrUnsupportedParameter)
		})
	}
}

// TestKMSSignVerifyAlgorithmKeySpecMismatch verifies that Sign/Verify reject algorithms
// that are incompatible with the key spec (e.g. ECDSA_SHA_256 on ECC_NIST_P384).
func TestKMSSignVerifyAlgorithmKeySpecMismatch(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "ECC_NIST_P384",
	})
	require.NoError(t, err)

	msg := []byte("test")

	// ECDSA_SHA_256 is only valid for ECC_NIST_P256, not ECC_NIST_P384
	_, signErr := b.Sign(context.Background(), &kms.SignInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          msg,
		MessageType:      "RAW",
		SigningAlgorithm: "ECDSA_SHA_256",
	})
	require.Error(t, signErr)

	_, verifyErr := b.Verify(context.Background(), &kms.VerifyInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          msg,
		MessageType:      "RAW",
		Signature:        []byte("sig"),
		SigningAlgorithm: "ECDSA_SHA_256",
	})
	require.Error(t, verifyErr)
}

// TestKMSHashAndAlgorithmInvalidMessageType verifies that an invalid message type is rejected.
func TestKMSHashAndAlgorithmInvalidMessageType(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	keyOut, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "RSA_2048",
	})
	require.NoError(t, err)

	_, err = b.Sign(context.Background(), &kms.SignInput{
		KeyID:            keyOut.KeyMetadata.KeyID,
		Message:          []byte("test"),
		MessageType:      "INVALID_TYPE",
		SigningAlgorithm: "RSASSA_PSS_SHA_256",
	})
	require.Error(t, err)
}

// TestKMSKeyMaterialUnavailableAfterManualRestore verifies that Encrypt/Sign/Verify
// return ErrKeyMaterialUnavailable when key material is missing after a snapshot restore
// from an old format that did not persist key materials.
func TestKMSKeyMaterialUnavailableAfterManualRestore(t *testing.T) {
	t.Parallel()

	orig := kms.NewInMemoryBackend()
	symKey, err := orig.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	asymKey, err := orig.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "ECC_NIST_P256",
	})
	require.NoError(t, err)

	// Build a snapshot without key_materials to simulate an old-format snapshot.
	snap := orig.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	// Strip key_materials from the JSON.
	stripped := strings.ReplaceAll(string(snap), `"key_materials":`, `"_key_materials":`)

	restored := kms.NewInMemoryBackend()
	require.NoError(t, restored.Restore(t.Context(), []byte(stripped)))

	// Encrypt should fail with ErrKeyMaterialUnavailable.
	_, encErr := restored.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:     symKey.KeyMetadata.KeyID,
		Plaintext: []byte("test"),
	})
	require.ErrorIs(t, encErr, kms.ErrKeyMaterialUnavailable)

	// Sign should fail with ErrKeyMaterialUnavailable.
	_, signErr := restored.Sign(context.Background(), &kms.SignInput{
		KeyID:            asymKey.KeyMetadata.KeyID,
		Message:          []byte("test"),
		MessageType:      "RAW",
		SigningAlgorithm: "ECDSA_SHA_256",
	})
	require.ErrorIs(t, signErr, kms.ErrKeyMaterialUnavailable)
}

// TestKMSBackendGetPublicKeyMaterialUnavailable verifies GetPublicKey returns ErrKeyMaterialUnavailable
// when key material is missing.
func TestKMSBackendGetPublicKeyMaterialUnavailable(t *testing.T) {
	t.Parallel()

	orig := kms.NewInMemoryBackend()
	key, err := orig.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageSignVerify,
		KeySpec:  "ECC_NIST_P256",
	})
	require.NoError(t, err)

	snap := orig.Snapshot(t.Context())
	require.NotEmpty(t, snap)
	stripped := strings.ReplaceAll(string(snap), `"key_materials":`, `"_key_materials":`)

	restored := kms.NewInMemoryBackend()
	require.NoError(t, restored.Restore(t.Context(), []byte(stripped)))

	_, err = restored.GetPublicKey(context.Background(), &kms.GetPublicKeyInput{KeyID: key.KeyMetadata.KeyID})
	require.ErrorIs(t, err, kms.ErrKeyMaterialUnavailable)
}

func TestECDSASignVerifyAllAlgorithms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		keySpec   string
		algorithm string
	}{
		{name: "p384_sha384", keySpec: "ECC_NIST_P384", algorithm: "ECDSA_SHA_384"},
		{name: "p521_sha512", keySpec: "ECC_NIST_P521", algorithm: "ECDSA_SHA_512"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			key, err := b.CreateKey(
				context.Background(),
				&kms.CreateKeyInput{KeySpec: tt.keySpec, KeyUsage: "SIGN_VERIFY"},
			)
			require.NoError(t, err)

			signOut, err := b.Sign(context.Background(), &kms.SignInput{
				KeyID:            key.KeyMetadata.KeyID,
				Message:          []byte("message"),
				SigningAlgorithm: tt.algorithm,
			})
			require.NoError(t, err)

			verOut, err := b.Verify(context.Background(), &kms.VerifyInput{
				KeyID:            key.KeyMetadata.KeyID,
				Message:          []byte("message"),
				Signature:        signOut.Signature,
				SigningAlgorithm: tt.algorithm,
			})
			require.NoError(t, err)
			assert.True(t, verOut.SignatureValid)
		})
	}
}

func TestRSAPKCS1SignVerify(t *testing.T) {
	t.Parallel()

	b := kms.NewInMemoryBackend()
	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeySpec:  "RSA_2048",
		KeyUsage: "SIGN_VERIFY",
	})
	require.NoError(t, err)

	signOut, err := b.Sign(context.Background(), &kms.SignInput{
		KeyID:            key.KeyMetadata.KeyID,
		Message:          []byte("data"),
		SigningAlgorithm: "RSASSA_PKCS1_V1_5_SHA_256",
	})
	require.NoError(t, err)

	verOut, err := b.Verify(context.Background(), &kms.VerifyInput{
		KeyID:            key.KeyMetadata.KeyID,
		Message:          []byte("data"),
		Signature:        signOut.Signature,
		SigningAlgorithm: "RSASSA_PKCS1_V1_5_SHA_256",
	})
	require.NoError(t, err)
	assert.True(t, verOut.SignatureValid)
}

func TestRSAPSSSignVerifyAllSizes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keySpec string
		algo    string
	}{
		{name: "rsa3072_pss_sha384", keySpec: "RSA_3072", algo: "RSASSA_PSS_SHA_384"},
		{name: "rsa4096_pss_sha512", keySpec: "RSA_4096", algo: "RSASSA_PSS_SHA_512"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			key, err := b.CreateKey(
				context.Background(),
				&kms.CreateKeyInput{KeySpec: tt.keySpec, KeyUsage: "SIGN_VERIFY"},
			)
			require.NoError(t, err)

			msg := []byte("test message for " + tt.name)
			signOut, err := b.Sign(context.Background(), &kms.SignInput{
				KeyID:            key.KeyMetadata.KeyID,
				Message:          msg,
				SigningAlgorithm: tt.algo,
			})
			require.NoError(t, err)

			verOut, err := b.Verify(context.Background(), &kms.VerifyInput{
				KeyID:            key.KeyMetadata.KeyID,
				Message:          msg,
				Signature:        signOut.Signature,
				SigningAlgorithm: tt.algo,
			})
			require.NoError(t, err)
			assert.True(t, verOut.SignatureValid)
		})
	}
}
