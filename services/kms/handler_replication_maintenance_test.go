package kms_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kms"

	"strings"
	"time"
)

func TestHandler_ReplicateKey_ViaHTTP(t *testing.T) {
	t.Parallel()
	h := b2newHandler(t)
	b := h.Backend.(*kms.InMemoryBackend)

	createOut, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{MultiRegion: true})
	keyID := createOut.KeyMetadata.KeyID

	body := fmt.Sprintf(`{"KeyId":"%s","ReplicaRegion":"eu-west-1"}`, keyID)
	rec := b2postKMSOp(t, h, "ReplicateKey", body)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_GetParametersForImport_ViaHTTP(t *testing.T) {
	t.Parallel()
	h := b2newHandler(t)
	b := h.Backend.(*kms.InMemoryBackend)

	keyID := b2mustCreateExternalKey(t, b)

	body := fmt.Sprintf(`{"KeyId":"%s","WrappingAlgorithm":"RSAES_OAEP_SHA_256","WrappingKeySpec":"RSA_2048"}`, keyID)
	rec := b2postKMSOp(t, h, "GetParametersForImport", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		KeyID             string  `json:"KeyId"`
		ParametersValidTo float64 `json:"ParametersValidTo"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, keyID, resp.KeyID)
	assert.Greater(t, resp.ParametersValidTo, float64(0))
}

func TestHandler_ImportKeyMaterial_ExpirationValidation_ViaHTTP(t *testing.T) {
	t.Parallel()
	h := b2newHandler(t)
	b := h.Backend.(*kms.InMemoryBackend)

	keyID := b2mustCreateExternalKey(t, b)

	mat := make([]byte, 32)
	for i := range mat {
		mat[i] = byte(i + 1)
	}
	matB64 := encodeBase64(mat)

	// KEY_MATERIAL_EXPIRES without ValidTo should fail
	body := fmt.Sprintf(
		`{"KeyId":"%s","EncryptedKeyMaterial":"%s","ExpirationModel":"KEY_MATERIAL_EXPIRES"}`, keyID, matB64,
	)
	rec := b2postKMSOp(t, h, "ImportKeyMaterial", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_ImportKeyMaterial_WireFieldName_ViaHTTP sends the real AWS wire
// field name (EncryptedKeyMaterial, not KeyMaterial) and asserts the import
// actually takes effect.
func TestHandler_ImportKeyMaterial_WireFieldName_ViaHTTP(t *testing.T) {
	t.Parallel()
	h := b2newHandler(t)
	b := h.Backend.(*kms.InMemoryBackend)

	keyID := b2mustCreateExternalKey(t, b)

	mat := make([]byte, 32)
	for i := range mat {
		mat[i] = byte(i + 1)
	}
	matB64 := encodeBase64(mat)

	body := fmt.Sprintf(`{"KeyId":"%s","EncryptedKeyMaterial":"%s"}`, keyID, matB64)
	rec := b2postKMSOp(t, h, "ImportKeyMaterial", body)
	require.Equal(t, http.StatusOK, rec.Code)

	descBody := fmt.Sprintf(`{"KeyId":"%s"}`, keyID)
	descRec := b2postKMSOp(t, h, "DescribeKey", descBody)
	require.Equal(t, http.StatusOK, descRec.Code)

	var descOut kms.DescribeKeyOutput
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))
	assert.Equal(t, kms.KeyStateEnabled, descOut.KeyMetadata.KeyState)
}

// TestKMSHandlerImportKeyMaterial verifies ImportKeyMaterial and DeleteImportedKeyMaterial
// via the HTTP handler.
func TestKMSHandlerImportKeyMaterial(t *testing.T) {
	t.Parallel()

	h := kms.NewHandler(kms.NewInMemoryBackend())

	// Create EXTERNAL-origin key.
	createBody, err := json.Marshal(kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(createBody))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "TrentService.CreateKey")
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var createOut kms.CreateKeyOutput
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createOut))
	assert.Equal(t, kms.KeyStatePendingImport, createOut.KeyMetadata.KeyState)

	// Import key material.
	mat := make([]byte, 32)
	for i := range mat {
		mat[i] = byte(i + 7)
	}

	importBody, err := json.Marshal(kms.ImportKeyMaterialInput{
		KeyID:       createOut.KeyMetadata.KeyID,
		KeyMaterial: mat,
	})
	require.NoError(t, err)
	importReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(importBody))
	importReq.Header.Set("Content-Type", "application/x-amz-json-1.1")
	importReq.Header.Set("X-Amz-Target", "TrentService.ImportKeyMaterial")
	importRec := httptest.NewRecorder()

	c2 := e.NewContext(importReq, importRec)
	require.NoError(t, h.Handler()(c2))
	assert.Equal(t, http.StatusOK, importRec.Code)

	// Describe should show Enabled.
	descBody, err := json.Marshal(kms.DescribeKeyInput{KeyID: createOut.KeyMetadata.KeyID})
	require.NoError(t, err)
	descReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(descBody))
	descReq.Header.Set("Content-Type", "application/x-amz-json-1.1")
	descReq.Header.Set("X-Amz-Target", "TrentService.DescribeKey")
	descRec := httptest.NewRecorder()

	c3 := e.NewContext(descReq, descRec)
	require.NoError(t, h.Handler()(c3))
	require.Equal(t, http.StatusOK, descRec.Code)

	var descOut kms.DescribeKeyOutput
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))
	assert.Equal(t, kms.KeyStateEnabled, descOut.KeyMetadata.KeyState)

	// DeleteImportedKeyMaterial.
	delBody, err := json.Marshal(kms.DeleteImportedKeyMaterialInput{KeyID: createOut.KeyMetadata.KeyID})
	require.NoError(t, err)
	delReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(delBody))
	delReq.Header.Set("Content-Type", "application/x-amz-json-1.1")
	delReq.Header.Set("X-Amz-Target", "TrentService.DeleteImportedKeyMaterial")
	delRec := httptest.NewRecorder()

	c4 := e.NewContext(delReq, delRec)
	require.NoError(t, h.Handler()(c4))
	assert.Equal(t, http.StatusOK, delRec.Code)

	// Key should be back to PendingImport.
	descBody2, err := json.Marshal(kms.DescribeKeyInput{KeyID: createOut.KeyMetadata.KeyID})
	require.NoError(t, err)
	descReq2 := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(descBody2))
	descReq2.Header.Set("Content-Type", "application/x-amz-json-1.1")
	descReq2.Header.Set("X-Amz-Target", "TrentService.DescribeKey")
	descRec2 := httptest.NewRecorder()

	c5 := e.NewContext(descReq2, descRec2)
	require.NoError(t, h.Handler()(c5))
	require.Equal(t, http.StatusOK, descRec2.Code)

	var descOut2 kms.DescribeKeyOutput
	require.NoError(t, json.Unmarshal(descRec2.Body.Bytes(), &descOut2))
	assert.Equal(t, kms.KeyStatePendingImport, descOut2.KeyMetadata.KeyState)
}

func TestKMSHandlerNewMaintenanceOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, b *kms.InMemoryBackend) string
		name       string
		action     string
		wantField  string
		wantStatus int
	}{
		{
			name:   "GetParametersForImport",
			action: "GetParametersForImport",
			setup: func(t *testing.T, b *kms.InMemoryBackend) string {
				t.Helper()
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
				require.NoError(t, err)

				return `{"KeyId":"` + key.KeyMetadata.KeyID + `"}`
			},
			wantStatus: http.StatusOK,
			wantField:  "ImportToken",
		},
		{
			name:   "ListKeyPolicies",
			action: "ListKeyPolicies",
			setup: func(t *testing.T, b *kms.InMemoryBackend) string {
				t.Helper()
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)

				return `{"KeyId":"` + key.KeyMetadata.KeyID + `"}`
			},
			wantStatus: http.StatusOK,
			wantField:  "PolicyNames",
		},
		{
			name:   "RotateKeyOnDemand",
			action: "RotateKeyOnDemand",
			setup: func(t *testing.T, b *kms.InMemoryBackend) string {
				t.Helper()
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)

				return `{"KeyId":"` + key.KeyMetadata.KeyID + `"}`
			},
			wantStatus: http.StatusOK,
			wantField:  "KeyId",
		},
		{
			name:   "UpdateKeyDescription",
			action: "UpdateKeyDescription",
			setup: func(t *testing.T, b *kms.InMemoryBackend) string {
				t.Helper()
				key, err := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
				require.NoError(t, err)

				return `{"KeyId":"` + key.KeyMetadata.KeyID + `","Description":"handler update"}`
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kms.NewInMemoryBackend()
			h := kms.NewHandler(b)
			body := tt.setup(t, b)

			rec := postKMSOp(t, h, tt.action, body)
			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantField == "" {
				return
			}

			var payload map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			_, ok := payload[tt.wantField]
			assert.True(t, ok)
		})
	}
}

// Test_KMS_ErrorClassification_MissingTableEntries drives the full HTTP handler path
// to confirm two sentinels that were absent from kmsErrorTable now classify correctly
// instead of falling through to the generic 500 branch:
//   - ErrKeyInvalidState (raised by checkKeyMaterialExpiry when an imported key's
//     material has passed its ValidTo) must surface as the real AWS
//     KMSInvalidStateException (400, client fault), not a 500. Encrypt/Decrypt's own
//     deserializeOpError (kms@v1.55.4 deserializers.go) declares KMSInvalidStateException,
//     not ExpiredImportTokenException -- gopherstack-8u3f.
//   - ErrKeyMaterialUnavailable (raised when key material is absent, e.g. after
//     restoring an old-format snapshot that never persisted key material) must surface
//     as the real AWS KeyUnavailableException (500, server fault — matches the real
//     SDK's ErrorFault: Server for this exception), not the made-up "InternalServiceError"
//     type string that classifyKMSError's default branch used to emit (not a real KMS
//     exception name at all).
func TestKMS_ErrorClassification_MissingTableEntries(t *testing.T) {
	t.Parallel()

	t.Run("expired_import_material", func(t *testing.T) {
		t.Parallel()

		e := echo.New()
		backend := kms.NewInMemoryBackend()
		h := kms.NewHandler(backend)

		keyOut, err := backend.CreateKey(context.Background(), &kms.CreateKeyInput{Origin: kms.KeyOriginExternal})
		require.NoError(t, err)
		keyID := keyOut.KeyMetadata.KeyID

		mat := make([]byte, 32)
		for i := range mat {
			mat[i] = byte(i + 1)
		}
		pastTS := float64(time.Now().Add(-time.Hour).UnixNano()) / 1e9
		require.NoError(t, backend.ImportKeyMaterial(context.Background(), &kms.ImportKeyMaterialInput{
			KeyID:       keyID,
			KeyMaterial: mat,
			ValidTo:     pastTS,
		}))

		body, err := json.Marshal(kms.EncryptInput{KeyID: keyID, Plaintext: []byte("test")})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
		req.Header.Set("X-Amz-Target", "TrentService.Encrypt")
		rec := httptest.NewRecorder()
		require.NoError(t, h.Handler()(e.NewContext(req, rec)))

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp kms.ErrorResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		assert.Equal(t, "KMSInvalidStateException", errResp.Type)
	})

	t.Run("key_material_unavailable_after_restore", func(t *testing.T) {
		t.Parallel()

		orig := kms.NewInMemoryBackend()
		keyOut, err := orig.CreateKey(context.Background(), &kms.CreateKeyInput{})
		require.NoError(t, err)
		keyID := keyOut.KeyMetadata.KeyID

		snap := orig.Snapshot(t.Context())
		require.NotEmpty(t, snap)
		stripped := strings.ReplaceAll(string(snap), `"key_materials":`, `"_key_materials":`)

		restored := kms.NewInMemoryBackend()
		require.NoError(t, restored.Restore(t.Context(), []byte(stripped)))

		e := echo.New()
		h := kms.NewHandler(restored)

		body, err := json.Marshal(kms.EncryptInput{KeyID: keyID, Plaintext: []byte("test")})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
		req.Header.Set("X-Amz-Target", "TrentService.Encrypt")
		rec := httptest.NewRecorder()
		require.NoError(t, h.Handler()(e.NewContext(req, rec)))

		assert.Equal(t, http.StatusInternalServerError, rec.Code)

		var errResp kms.ErrorResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		assert.Equal(t, "KeyUnavailableException", errResp.Type)
	})
}

func TestHandlerCreateKeyHMACMultiRegionRejected(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	h := kms.NewHandler(b)

	rec := sendKMSOp(t, h, "CreateKey", `{"KeySpec":"HMAC_256","KeyUsage":"GENERATE_VERIFY_MAC","MultiRegion":true}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandlerReplicateKeyRequiresMultiRegion(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	h := kms.NewHandler(b)

	// Create non-MultiRegion key.
	keyRec := sendKMSOp(t, h, "CreateKey", `{}`)
	require.Equal(t, http.StatusOK, keyRec.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(keyRec.Body.Bytes(), &createOut))
	keyID := createOut["KeyMetadata"].(map[string]any)["KeyId"].(string)

	rec := sendKMSOp(t, h, "ReplicateKey", `{"KeyId":"`+keyID+`","ReplicaRegion":"us-west-2"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandlerReplicateKeyCopiesTags(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	h := kms.NewHandler(b)

	keyRec := sendKMSOp(t, h, "CreateKey", `{"MultiRegion":true}`)
	require.Equal(t, http.StatusOK, keyRec.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(keyRec.Body.Bytes(), &createOut))
	keyID := createOut["KeyMetadata"].(map[string]any)["KeyId"].(string)

	tagRec := sendKMSOp(t, h, "TagResource", `{"KeyId":"`+keyID+`","Tags":[{"TagKey":"env","TagValue":"prod"}]}`)
	require.Equal(t, http.StatusOK, tagRec.Code)

	repRec := sendKMSOp(t, h, "ReplicateKey", `{"KeyId":"`+keyID+`","ReplicaRegion":"eu-west-1"}`)
	require.Equal(t, http.StatusOK, repRec.Code)

	var repOut map[string]any
	require.NoError(t, json.Unmarshal(repRec.Body.Bytes(), &repOut))
	replicaID := repOut["ReplicaKeyMetadata"].(map[string]any)["KeyId"].(string)

	tagListRec := sendKMSOp(t, h, "ListResourceTags", `{"KeyId":"`+replicaID+`"}`)
	require.Equal(t, http.StatusOK, tagListRec.Code)

	var tagListOut map[string]any
	require.NoError(t, json.Unmarshal(tagListRec.Body.Bytes(), &tagListOut))
	tags := tagListOut["Tags"].([]any)
	require.NotEmpty(t, tags)

	found := false
	for _, tag := range tags {
		tagMap := tag.(map[string]any)
		if tagMap["TagKey"] == "env" && tagMap["TagValue"] == "prod" {
			found = true
		}
	}

	assert.True(t, found, "expected env=prod tag to be copied to replica")
}

func TestHandlerUpdatePrimaryRegionViaHTTP(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	h := kms.NewHandler(b)

	keyRec := sendKMSOp(t, h, "CreateKey", `{"MultiRegion":true}`)
	require.Equal(t, http.StatusOK, keyRec.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(keyRec.Body.Bytes(), &createOut))
	keyID := createOut["KeyMetadata"].(map[string]any)["KeyId"].(string)

	// Replicate to eu-central-1 first — UpdatePrimaryRegion requires an actual replica.
	repRec := sendKMSOp(t, h, "ReplicateKey", `{"KeyId":"`+keyID+`","ReplicaRegion":"eu-central-1"}`)
	require.Equal(t, http.StatusOK, repRec.Code)

	rec := sendKMSOp(t, h, "UpdatePrimaryRegion", `{"KeyId":"`+keyID+`","PrimaryRegion":"eu-central-1"}`)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerReplicateKeyWithInputTags(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	h := kms.NewHandler(b)

	keyRec := sendKMSOp(t, h, "CreateKey", `{"MultiRegion":true,"Tags":[{"TagKey":"src","TagValue":"yes"}]}`)
	require.Equal(t, http.StatusOK, keyRec.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(keyRec.Body.Bytes(), &createOut))
	keyID := createOut["KeyMetadata"].(map[string]any)["KeyId"].(string)

	// Replicate with explicit tags that should overlay the source tags.
	repRec := sendKMSOp(t, h, "ReplicateKey",
		`{"KeyId":"`+keyID+`","ReplicaRegion":"ap-northeast-1","Tags":[{"TagKey":"extra","TagValue":"1"}]}`)
	require.Equal(t, http.StatusOK, repRec.Code)

	var repOut map[string]any
	require.NoError(t, json.Unmarshal(repRec.Body.Bytes(), &repOut))
	replicaID := repOut["ReplicaKeyMetadata"].(map[string]any)["KeyId"].(string)

	tagListRec := sendKMSOp(t, h, "ListResourceTags", `{"KeyId":"`+replicaID+`"}`)
	require.Equal(t, http.StatusOK, tagListRec.Code)

	var tagListOut map[string]any
	require.NoError(t, json.Unmarshal(tagListRec.Body.Bytes(), &tagListOut))
	tags := tagListOut["Tags"].([]any)

	keys := make(map[string]string)
	for _, tag := range tags {
		tm := tag.(map[string]any)
		keys[tm["TagKey"].(string)] = tm["TagValue"].(string)
	}

	assert.Equal(t, "yes", keys["src"], "expected source tag copied")
	assert.Equal(t, "1", keys["extra"], "expected input tag applied")
}
