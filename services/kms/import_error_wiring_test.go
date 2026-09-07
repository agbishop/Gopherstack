package kms_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

// TestImportKeyMaterial_RSAUnwrapFailure_IsInvalidCiphertext covers gopherstack-h88p:
// resolveKeyMaterial's RSA-OAEP unwrap failure raised ErrInvalidKeyUsage, which
// ImportKeyMaterial's own deserializeOpError does not recognize. InvalidCiphertextException's
// doc (kms@v1.55.4 types/errors.go) is explicit for this exact case: "From the
// ImportKeyMaterial operation, the request was rejected because KMS could not decrypt
// the encrypted (wrapped) key material".
func TestImportKeyMaterial_RSAUnwrapFailure_IsInvalidCiphertext(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)
	keyID := ab2MustCreateKeyExternal(t, h, true)

	_, err := h.Backend.GetParametersForImport(context.Background(), &kms.GetParametersForImportInput{
		KeyID:             keyID,
		WrappingAlgorithm: "RSAES_OAEP_SHA_256",
		WrappingKeySpec:   "RSA_2048",
	})
	require.NoError(t, err)

	// >= minRSAWrappedMaterialBytes (256) but not real RSA-OAEP ciphertext for the stored
	// wrapping key, so rsa.DecryptOAEP fails.
	garbage := make([]byte, 256)
	_, randErr := rand.Read(garbage)
	require.NoError(t, randErr)

	rec := doKMSRequest(t, h, "ImportKeyMaterial", mustJSON(t, kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: garbage,
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "InvalidCiphertextException", errResp.Type)
	assert.NotEqual(t, "InvalidKeyUsageException", errResp.Type)

	desc, err := h.Backend.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingImport, desc.KeyMetadata.KeyState, "rejected import must not mutate KeyState")
}

// TestImportKeyMaterial_AsymmetricKeySpec_IsUnsupportedOperation covers gopherstack-h88p:
// the "only SYMMETRIC_DEFAULT keys" guard raised ErrInvalidKeyUsage, not declared by
// ImportKeyMaterial. UnsupportedOperationException's doc covers a resource (the key's own
// KeySpec) not valid for this operation, and ErrUnsupportedParameter already carries that
// exact doc for KeySpec-shaped mismatches elsewhere in this file.
func TestImportKeyMaterial_AsymmetricKeySpec_IsUnsupportedOperation(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)

	out, err := h.Backend.CreateKey(context.Background(), &kms.CreateKeyInput{
		Origin:   kms.KeyOriginExternal,
		KeySpec:  "RSA_2048",
		KeyUsage: kms.KeyUsageSignVerify,
	})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	rec := doKMSRequest(t, h, "ImportKeyMaterial", mustJSON(t, kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: make([]byte, 32),
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "UnsupportedOperationException", errResp.Type)
	assert.NotEqual(t, "InvalidKeyUsageException", errResp.Type)

	desc, err := h.Backend.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingImport, desc.KeyMetadata.KeyState, "rejected import must not mutate KeyState")
}

// TestImportKeyMaterial_EmptyMaterial_IsIncorrectKeyMaterial covers gopherstack-h88p: the
// empty-KeyMaterial guard raised ErrInvalidKeyUsage, not declared by ImportKeyMaterial.
// ValidationException is not a fix either -- it names no type anywhere in this SDK module,
// same as the CreateKey landmine. IncorrectKeyMaterialException's own doc disjunction
// ("is, expired, invalid, or does not meet expectations") covers empty material under
// "invalid", the same declared code as the wrong-length site below under "does not meet
// expectations".
func TestImportKeyMaterial_EmptyMaterial_IsIncorrectKeyMaterial(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)
	keyID := ab2MustCreateKeyExternal(t, h, true)

	rec := doKMSRequest(t, h, "ImportKeyMaterial", mustJSON(t, kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: []byte{},
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "IncorrectKeyMaterialException", errResp.Type)
	assert.NotEqual(t, "InvalidKeyUsageException", errResp.Type)
	assert.NotEqual(t, "ValidationException", errResp.Type)

	desc, err := h.Backend.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingImport, desc.KeyMetadata.KeyState, "rejected import must not mutate KeyState")
}

// TestImportKeyMaterial_WrongLength_IsIncorrectKeyMaterial covers gopherstack-h88p: the
// wrong-length guard raised ErrInvalidKeyUsage, not declared by ImportKeyMaterial.
// IncorrectKeyMaterialException's doc (kms@v1.55.4 types/errors.go) fits: "the key
// material in the request is, expired, invalid, or does not meet expectations".
func TestImportKeyMaterial_WrongLength_IsIncorrectKeyMaterial(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)
	keyID := ab2MustCreateKeyExternal(t, h, true)

	rec := doKMSRequest(t, h, "ImportKeyMaterial", mustJSON(t, kms.ImportKeyMaterialInput{
		KeyID:       keyID,
		KeyMaterial: make([]byte, 16), // too short; must be exactly 32 (aes256Bytes)
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "IncorrectKeyMaterialException", errResp.Type)
	assert.NotEqual(t, "InvalidKeyUsageException", errResp.Type)

	desc, err := h.Backend.DescribeKey(context.Background(), &kms.DescribeKeyInput{KeyID: keyID})
	require.NoError(t, err)
	assert.Equal(t, kms.KeyStatePendingImport, desc.KeyMetadata.KeyState, "rejected import must not mutate KeyState")
}

// TestDecrypt_WrongKeyUsage_StillInvalidKeyUsageException and
// TestVerifyMac_WrongKeyUsage_StillInvalidKeyUsageException guard against the
// ImportKeyMaterial narrowing above being too broad: Decrypt and VerifyMac both
// legitimately declare InvalidKeyUsageException in their own deserializeOpError and must
// keep emitting it.
func TestDecrypt_WrongKeyUsage_StillInvalidKeyUsageException(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)

	out, err := h.Backend.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeySpec:  "RSA_2048",
		KeyUsage: kms.KeyUsageSignVerify,
	})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	blob := append([]byte(keyID), []byte("padding-past-the-key-id-prefix-marker")...)

	rec := doKMSRequest(t, h, "Decrypt", mustJSON(t, kms.DecryptInput{
		KeyID:          keyID,
		CiphertextBlob: blob,
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "InvalidKeyUsageException", errResp.Type)
}

func TestVerifyMac_WrongKeyUsage_StillInvalidKeyUsageException(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)

	out, err := h.Backend.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := out.KeyMetadata.KeyID

	rec := doKMSRequest(t, h, "VerifyMac", mustJSON(t, kms.VerifyMacInput{
		KeyID:        keyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      []byte("test message"),
		Mac:          make([]byte, 32),
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "InvalidKeyUsageException", errResp.Type)
}
