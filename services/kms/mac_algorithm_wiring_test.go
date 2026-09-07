package kms_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"
)

// TestGenerateMac_WrongAlgorithm_WireType_IsInvalidKeyUsage covers gopherstack-yatn:
// validateMacAlgorithm raised ErrInvalidAlgorithm ("InvalidAlgorithmException"), a code
// absent from the whole pinned kms@v1.55.4 module -- not declared by any operation.
// GenerateMac's own deserializeOpError declares InvalidKeyUsageException, whose doc
// (types/errors.go) covers exactly this: "the encryption algorithm or signing algorithm
// specified for the operation is incompatible with the type of key material in the KMS
// key (KeySpec)".
func TestGenerateMac_WrongAlgorithm_WireType_IsInvalidKeyUsage(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)
	out, err := h.Backend.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeySpec:  "HMAC_256",
		KeyUsage: kms.KeyUsageGenerateMac,
	})
	require.NoError(t, err)

	rec := doKMSRequest(t, h, "GenerateMac", mustJSON(t, kms.GenerateMacInput{
		KeyID:        out.KeyMetadata.KeyID,
		Message:      []byte("test"),
		MacAlgorithm: "HMAC_SHA_512",
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "InvalidKeyUsageException", errResp.Type)
}

// TestVerifyMac_WrongAlgorithm_WireType_IsInvalidKeyUsage is VerifyMac's control for the
// same fix: VerifyMac's own deserializeOpError also declares InvalidKeyUsageException.
func TestVerifyMac_WrongAlgorithm_WireType_IsInvalidKeyUsage(t *testing.T) {
	t.Parallel()

	h := ab2NewHandler(t)
	out, err := h.Backend.CreateKey(context.Background(), &kms.CreateKeyInput{
		KeySpec:  "HMAC_384",
		KeyUsage: kms.KeyUsageGenerateMac,
	})
	require.NoError(t, err)

	rec := doKMSRequest(t, h, "VerifyMac", mustJSON(t, kms.VerifyMacInput{
		KeyID:        out.KeyMetadata.KeyID,
		Message:      []byte("test"),
		Mac:          []byte("fakemac"),
		MacAlgorithm: "HMAC_SHA_256",
	}))

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "InvalidKeyUsageException", errResp.Type)
}
