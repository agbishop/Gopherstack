package kms_test

import (
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

	"log/slog"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// TestHandler_CreateKey_DescriptionTooLong_ViaHTTP is a regression test for
// gopherstack-i4q8: CreateKey declares LimitExceededException for exactly
// this condition ("a length constraint or quota was exceeded"), not the
// fabricated ValidationException.
func TestHandler_CreateKey_DescriptionTooLong_ViaHTTP(t *testing.T) {
	t.Parallel()
	h := b2newHandler(t)
	b := h.Backend.(*kms.InMemoryBackend)

	longDescription := strings.Repeat("x", 8193)
	body, err := json.Marshal(map[string]string{"Description": longDescription})
	require.NoError(t, err)

	rec := b2postKMSOp(t, h, "CreateKey", string(body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp kms.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "LimitExceededException", errResp.Type)
	assert.NotEqual(t, "ValidationException", errResp.Type)

	listOut, err := b.ListKeys(context.Background(), &kms.ListKeysInput{})
	require.NoError(t, err)
	assert.Empty(t, listOut.Keys, "rejected CreateKey must not create a key")
}

func TestHandler_ScheduleKeyDeletion_ViaHTTP(t *testing.T) {
	t.Parallel()
	h := b2newHandler(t)
	b := h.Backend.(*kms.InMemoryBackend)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	body := fmt.Sprintf(`{"KeyId":"%s","PendingWindowInDays":7}`, keyID)
	rec := b2postKMSOp(t, h, "ScheduleKeyDeletion", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		KeyState string `json:"KeyState"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, kms.KeyStatePendingDeletion, resp.KeyState)
}

func TestHandler_CancelKeyDeletion_ViaHTTP(t *testing.T) {
	t.Parallel()
	h := b2newHandler(t)
	b := h.Backend.(*kms.InMemoryBackend)

	out, _ := b.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	_, err := b.ScheduleKeyDeletion(context.Background(), &kms.ScheduleKeyDeletionInput{
		KeyID:               keyID,
		PendingWindowInDays: 7,
	})
	require.NoError(t, err)

	body := fmt.Sprintf(`{"KeyId":"%s"}`, keyID)
	rec := b2postKMSOp(t, h, "CancelKeyDeletion", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		KeyState string `json:"KeyState"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, kms.KeyStateDisabled, resp.KeyState)
}

func TestHandlerReset(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	h := kms.NewHandler(b)

	// Create a key via the handler.
	rec := sendKMSOp(t, h, "CreateKey", `{}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, kms.KeyCount(b))

	h.Reset()
	assert.Equal(t, 0, kms.KeyCount(b))
}

// TestKMSHandler verifies the HTTP handler dispatches operations correctly.
func TestKMSHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupFn        func(*testing.T, kms.StorageBackend) string
		checkFn        func(*testing.T, *httptest.ResponseRecorder)
		target         string
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "CreateKey",
			target:         "TrentService.CreateKey",
			body:           `{"Description":"my key"}`,
			expectedStatus: http.StatusOK,
			checkFn: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var out kms.CreateKeyOutput
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.NotEmpty(t, out.KeyMetadata.KeyID)
			},
		},
		{
			name:           "UnknownAction",
			target:         "TrentService.FakeOp",
			body:           `{}`,
			expectedStatus: http.StatusBadRequest,
			checkFn: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var errResp kms.ErrorResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Equal(t, "UnknownOperationException", errResp.Type)
			},
		},
		{
			name:           "MissingTarget",
			target:         "",
			body:           `{}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "GetSupportedOps",
			target:         "",
			body:           "",
			expectedStatus: http.StatusOK,
			checkFn: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var ops []string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ops))
				assert.Contains(t, ops, "CreateKey")
			},
		},
		{
			name:           "DescribeKeyNotFound",
			target:         "TrentService.DescribeKey",
			body:           `{"KeyId":"missing"}`,
			expectedStatus: http.StatusBadRequest,
			checkFn: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var errResp kms.ErrorResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Equal(t, "NotFoundException", errResp.Type)
			},
		},
		{
			name:   "ListKeys",
			target: "TrentService.ListKeys",
			body:   `{}`,
			setupFn: func(t *testing.T, backend kms.StorageBackend) string {
				t.Helper()
				_, _ = backend.CreateKey(context.Background(), &kms.CreateKeyInput{})

				return ""
			},
			expectedStatus: http.StatusOK,
			checkFn: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				var out kms.ListKeysOutput
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Len(t, out.Keys, 1)
			},
		},
		{
			name:           "InvalidTarget",
			target:         "TrentServiceNoSep",
			body:           `{}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()

			backend := kms.NewInMemoryBackend()

			if tt.setupFn != nil {
				tt.setupFn(t, backend)
			}

			h := kms.NewHandler(backend)

			var req *http.Request

			switch {
			case tt.target != "":
				req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
				req.Header.Set("X-Amz-Target", tt.target)
			case tt.body != "":
				req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			default:
				req = httptest.NewRequest(http.MethodGet, "/", nil)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, rec.Code)

			if tt.checkFn != nil {
				tt.checkFn(t, rec)
			}
		})
	}
}

// TestKMSHandlerRouteMatcher verifies the route matcher for KMS.
func TestKMSHandlerRouteMatcher(t *testing.T) {
	t.Parallel()

	e := echo.New()
	backend := kms.NewInMemoryBackend()
	h := kms.NewHandler(backend)
	matcher := h.RouteMatcher()

	t.Run("MatchesTrentService", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("X-Amz-Target", "TrentService.CreateKey")
		c := e.NewContext(req, httptest.NewRecorder())
		assert.True(t, matcher(c))
	})

	t.Run("DoesNotMatchOther", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("X-Amz-Target", "AmazonSSM.GetParameter")
		c := e.NewContext(req, httptest.NewRecorder())
		assert.False(t, matcher(c))
	})
}

// TestKMSHandlerInterface verifies the handler interface methods.
func TestKMSHandlerInterface(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()
	h := kms.NewHandler(backend)

	assert.Equal(t, "KMS", h.Name())
	assert.Equal(t, 95, h.MatchPriority())

	e := echo.New()

	// ExtractOperation
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Amz-Target", "TrentService.CreateKey")
	c := e.NewContext(req, httptest.NewRecorder())
	assert.Equal(t, "CreateKey", h.ExtractOperation(c))

	// ExtractOperation with no separator
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set("X-Amz-Target", "TrentServiceNoSep")
	c2 := e.NewContext(req2, httptest.NewRecorder())
	assert.Equal(t, "Unknown", h.ExtractOperation(c2))

	// ExtractResource with body
	body := `{"KeyId":"test-key"}`
	req3 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req3.Header.Set("Content-Type", "application/json")
	c3 := e.NewContext(req3, httptest.NewRecorder())
	resource := h.ExtractResource(c3)
	assert.Equal(t, "test-key", resource)

	// ExtractResource with no KeyId
	req4 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	c4 := e.NewContext(req4, httptest.NewRecorder())
	assert.Empty(t, h.ExtractResource(c4))
}

// TestKMSHandlerDisableEnableKey verifies DisableKey and EnableKey via HTTP handler.
func TestKMSHandlerDisableEnableKey(t *testing.T) {
	t.Parallel()

	backend := kms.NewInMemoryBackend()
	h := kms.NewHandler(backend)

	out, _ := backend.CreateKey(context.Background(), &kms.CreateKeyInput{})
	keyID := out.KeyMetadata.KeyID

	doKMSReqLocal := func(t *testing.T, h *kms.Handler, action, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("X-Amz-Target", "TrentService."+action)
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		ctx := logger.Save(req.Context(), slog.Default())
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		e := echo.New()
		c := e.NewContext(req, rec)
		err := h.Handler()(c)
		require.NoError(t, err)

		return rec
	}

	body := `{"KeyId":"` + keyID + `"}`
	rec := doKMSReqLocal(t, h, "DisableKey", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doKMSReqLocal(t, h, "EnableKey", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doKMSReqLocal(t, h, "ScheduleKeyDeletion", `{"KeyId":"`+keyID+`","PendingWindowInDays":7}`)
	assert.Equal(t, http.StatusOK, rec.Code)
	var schedResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &schedResp))
	assert.Equal(t, kms.KeyStatePendingDeletion, schedResp["KeyState"])

	rec = doKMSReqLocal(t, h, "CancelKeyDeletion", body)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerCancelKeyDeletionReturnsBody(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	h := kms.NewHandler(b)

	keyRec := sendKMSOp(t, h, "CreateKey", `{}`)
	require.Equal(t, http.StatusOK, keyRec.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(keyRec.Body.Bytes(), &createOut))
	keyID := createOut["KeyMetadata"].(map[string]any)["KeyId"].(string)

	sendKMSOp(t, h, "ScheduleKeyDeletion", `{"KeyId":"`+keyID+`","PendingWindowInDays":7}`)

	rec := sendKMSOp(t, h, "CancelKeyDeletion", `{"KeyId":"`+keyID+`"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, keyID, out["KeyId"])
	assert.Equal(t, kms.KeyStateDisabled, out["KeyState"])
}

func TestHandlerUpdateKeyDescriptionMaxLengthRejected(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	h := kms.NewHandler(b)

	keyRec := sendKMSOp(t, h, "CreateKey", `{}`)
	require.Equal(t, http.StatusOK, keyRec.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(keyRec.Body.Bytes(), &createOut))
	keyID := createOut["KeyMetadata"].(map[string]any)["KeyId"].(string)

	longDesc := strings.Repeat("x", 8193)
	rec := sendKMSOp(t, h, "UpdateKeyDescription", `{"KeyId":"`+keyID+`","Description":"`+longDesc+`"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
