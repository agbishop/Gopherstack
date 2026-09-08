package kms_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"

	"strings"
)

func TestVerifyMac_Handler(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	h := kms.NewHandler(b)

	key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{KeySpec: "HMAC_256"})
	require.NoError(t, err)

	msg := []byte("hello")
	macOut, err := b.GenerateMac(context.Background(), &kms.GenerateMacInput{
		KeyID:        key.KeyMetadata.KeyID,
		MacAlgorithm: "HMAC_SHA_256",
		Message:      msg,
	})
	require.NoError(t, err)

	bodyBytes, _ := json.Marshal(map[string]any{
		"KeyId":        key.KeyMetadata.KeyID,
		"MacAlgorithm": "HMAC_SHA_256",
		"Message":      msg,
		"Mac":          macOut.Mac,
	})
	rec := sendKMSOp(t, h, "VerifyMac", string(bodyBytes))
	require.Equal(t, http.StatusOK, rec.Code)

	var verOut kms.VerifyMacOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &verOut))
	assert.True(t, verOut.MacValid)
}

// TestKMSHandlerGenerateDataKey tests data key generation via HTTP.
func TestKMSHandlerGenerateDataKey(t *testing.T) {
	t.Parallel()

	e := echo.New()

	backend := kms.NewInMemoryBackend()
	h := kms.NewHandler(backend)

	// Create key
	createReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	createReq.Header.Set("X-Amz-Target", "TrentService.CreateKey")
	createRec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(createReq, createRec)))

	var createOut kms.CreateKeyOutput
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
	keyID := createOut.KeyMetadata.KeyID

	body, _ := json.Marshal(map[string]string{"KeyID": keyID, "KeySpec": "AES_256"})
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Amz-Target", "TrentService.GenerateDataKey")
	rec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusOK, rec.Code)

	var out kms.GenerateDataKeyOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Len(t, out.Plaintext, 32)
	assert.NotEmpty(t, out.CiphertextBlob)
}

// TestKMSGenerateDataKeyWithoutPlaintext verifies GenerateDataKeyWithoutPlaintext.
func TestKMSGenerateDataKeyWithoutPlaintext(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()
	h := kms.NewHandler(backend)

	keyOut, err := backend.CreateKey(context.Background(), &kms.CreateKeyInput{})
	require.NoError(t, err)
	keyID := keyOut.KeyMetadata.KeyID

	body, _ := json.Marshal(map[string]string{"KeyId": keyID, "KeySpec": "AES_256"})
	rec := doKMSHTTPRequest(t, h, "GenerateDataKeyWithoutPlaintext", string(body))
	assert.Equal(t, http.StatusOK, rec.Code)
	var out kms.GenerateDataKeyWithoutPlaintextOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.NotEmpty(t, out.CiphertextBlob)
	assert.Equal(t, keyOut.KeyMetadata.Arn, out.KeyID)
}

// Test_KMS_InvalidKeySpec_Returns400ValidationException drives the full HTTP handler
// path to confirm an unrecognized KeySpec/KeyPairSpec surfaces as a 400
// ValidationException, not a 500 InternalServiceError. Before the fix,
// generateKeyMaterial's "unsupported key spec" sentinel was never wrapped with
// ErrValidation, so classifyKMSError fell through to its default 500 branch.
func TestKMS_InvalidKeySpec_Returns400UnsupportedOperationException(t *testing.T) {
	t.Parallel()

	tests := []struct {
		buildBody    func(t *testing.T, wrapKeyID string) (target, body string)
		name         string
		needsWrapKey bool
	}{
		{
			name: "CreateKey_bogus_KeySpec",
			buildBody: func(t *testing.T, _ string) (string, string) {
				t.Helper()

				return "TrentService.CreateKey", `{"KeySpec":"BOGUS_SPEC"}`
			},
		},
		{
			name:         "GenerateDataKeyPair_bogus_KeyPairSpec",
			needsWrapKey: true,
			buildBody: func(t *testing.T, wrapKeyID string) (string, string) {
				t.Helper()

				body, err := json.Marshal(map[string]string{
					"KeyId":       wrapKeyID,
					"KeyPairSpec": "BOGUS_SPEC",
				})
				require.NoError(t, err)

				return "TrentService.GenerateDataKeyPair", string(body)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			backend := kms.NewInMemoryBackend()
			h := kms.NewHandler(backend)

			var wrapKeyID string

			if tt.needsWrapKey {
				out, err := backend.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)
				wrapKeyID = out.KeyMetadata.KeyID
			}

			target, body := tt.buildBody(t, wrapKeyID)

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			req.Header.Set("X-Amz-Target", target)

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			require.NoError(t, h.Handler()(c))
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp kms.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, "UnsupportedOperationException", errResp.Type)
		})
	}
}
