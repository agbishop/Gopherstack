package kms_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

// TestCancelKeyDeletion_DisabledKey_NotDisabledException covers gopherstack-8u3f:
// CancelKeyDeletion's own deserializeOpError (kms@v1.55.4 deserializers.go) does not
// recognize DisabledException, only KMSInvalidStateException -- but the old code routed
// through keyStateError, which returns DisabledException whenever KeyState==Disabled,
// regardless of whether Disabled is even the state being rejected here.
func TestCancelKeyDeletion_DisabledKey_NotDisabledException(t *testing.T) {
	t.Parallel()

	h := b2newHandler(t)

	out, err := h.Backend.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	require.NoError(t, h.Backend.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: keyID}))

	rec := doKMSRequest(t, h, "CancelKeyDeletion", mustJSON(t, kms.CancelKeyDeletionInput{KeyID: keyID}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "KMSInvalidStateException", errResp.Type)

	desc, err := h.Backend.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(
		t, kms.KeyStateDisabled, desc.KeyMetadata.KeyState,
		"rejected CancelKeyDeletion must not mutate KeyState",
	)
}

// TestDecrypt_DisabledKey_StillDisabledException proves the CancelKeyDeletion/
// ImportKeyMaterial narrowing above did not touch keyStateError itself: a crypto op that
// legitimately declares DisabledException (Decrypt) must still emit it for a disabled key.
func TestDecrypt_DisabledKey_StillDisabledException(t *testing.T) {
	t.Parallel()

	h := b2newHandler(t)

	out, err := h.Backend.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	encOut, err := h.Backend.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:     keyID,
		Plaintext: []byte("payload"),
	})
	require.NoError(t, err)

	require.NoError(t, h.Backend.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: keyID}))

	rec := doKMSRequest(t, h, "Decrypt", mustJSON(t, kms.DecryptInput{
		KeyID:          keyID,
		CiphertextBlob: encOut.CiphertextBlob,
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "DisabledException", errResp.Type)
}

// TestImportKeyMaterial_DisabledKey_NotDisabledException covers gopherstack-8u3f:
// ImportKeyMaterial's own deserializeOpError does not recognize DisabledException either
// (only KMSInvalidStateException), but the old "wrong state" branch routed through the
// same keyStateError helper as CancelKeyDeletion above.
func TestImportKeyMaterial_DisabledKey_NotDisabledException(t *testing.T) {
	t.Parallel()

	h := b2newHandler(t)

	extOut, err := h.Backend.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
	require.NoError(t, err)
	keyID := extOut.KeyMetadata.KeyID

	original := make([]byte, 32)
	for i := range original {
		original[i] = byte(i + 1)
	}
	require.NoError(t, h.Backend.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: original,
	}))

	encOut, err := h.Backend.Encrypt(context.Background(), &kms.EncryptInput{
		KeyID:     keyID,
		Plaintext: []byte("marker"),
	})
	require.NoError(t, err)

	require.NoError(t, h.Backend.DisableKey(context.Background(), &kms.DisableKeyInput{KeyID: keyID}))

	replacement := make([]byte, 32)
	for i := range replacement {
		replacement[i] = byte(255 - i)
	}
	rec := doKMSRequest(t, h, "ImportKeyMaterial", mustJSON(t, kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: replacement,
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "KMSInvalidStateException", errResp.Type)

	require.NoError(t, h.Backend.EnableKey(context.Background(), &kms.EnableKeyInput{KeyID: keyID}))

	decOut, err := h.Backend.Decrypt(context.Background(), &kms.DecryptInput{
		KeyID:          keyID,
		CiphertextBlob: encOut.CiphertextBlob,
	})
	require.NoError(t, err, "original key material must be intact: rejected reimport must not mutate it")
	assert.Equal(t, []byte("marker"), decOut.Plaintext)
}

// TestVerifyMac_Mismatch_IsInvalidMac_NotInvalidSignature covers gopherstack-8u3f:
// VerifyMac's own deserializeOpError recognizes KMSInvalidMacException, not
// KMSInvalidSignatureException -- the old code reused ErrInvalidSignature (the sentinel
// Verify's asymmetric-signature mismatch legitimately uses) for VerifyMac's HMAC mismatch.
func TestVerifyMac_Mismatch_IsInvalidMac_NotInvalidSignature(t *testing.T) {
	t.Parallel()

	h := b2newHandler(t)

	out, err := h.Backend.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeyUsage: kms.KeyUsageGenerateMac,
		KeySpec:  "HMAC_256",
	})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	macOut, err := h.Backend.GenerateMac(context.Background(), &kms.GenerateMacInput{
		KeyID:        keyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      []byte("test message"),
	})
	require.NoError(t, err)

	badMac := make([]byte, len(macOut.Mac))
	copy(badMac, macOut.Mac)
	badMac[0] ^= 0xFF

	rec := doKMSRequest(t, h, "VerifyMac", mustJSON(t, kms.VerifyMacInput{
		KeyID:        keyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      []byte("test message"),
		Mac:          badMac,
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "KMSInvalidMacException", errResp.Type)
}

// TestDecrypt_ExpiredImportedMaterial_KMSInvalidStateException covers gopherstack-8u3f:
// Decrypt's own deserializeOpError declares KMSInvalidStateException, not
// ExpiredImportTokenException, for the same reason Encrypt's does (see
// TestKMS_ErrorClassification_MissingTableEntries/expired_import_material in
// handler_replication_maintenance_test.go, corrected by this same issue).
func TestDecrypt_ExpiredImportedMaterial_KMSInvalidStateException(t *testing.T) {
	t.Parallel()

	h := b2newHandler(t)

	extOut, err := h.Backend.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
	require.NoError(t, err)
	keyID := extOut.KeyMetadata.KeyID

	mat := make([]byte, 32)
	for i := range mat {
		mat[i] = byte(i + 1)
	}
	pastTS := float64(time.Now().Add(-time.Hour).UnixNano()) / 1e9
	require.NoError(t, h.Backend.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: mat,
		ValidTo:     pastTS,
	}))

	// Decrypt resolves the key from the blob's 36-byte KeyId prefix before ever getting
	// to checkKeyMaterialExpiry, so the blob must carry a real (well-formed) KeyId even
	// though the payload bytes after it are never reached.
	blob := append([]byte(keyID), []byte("padding-past-the-key-id-prefix")...)

	rec := doKMSRequest(t, h, "Decrypt", mustJSON(t, kms.DecryptInput{
		KeyID:          keyID,
		CiphertextBlob: blob,
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "KMSInvalidStateException", errResp.Type)
}
