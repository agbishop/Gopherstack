package sts_test

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/services/sts"
)

// TestDecodeAuthorizationMessage verifies the DecodeAuthorizationMessage action.
func TestDecodeAuthorizationMessage(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()
	h := sts.NewHandler(backend)
	e := echo.New()

	original := "this is a test message"
	// Only STS-issued encoded messages are accepted. Use the backend to issue one.
	encoded := backend.IssueEncodedAuthorizationMessage(original)

	form := url.Values{
		"Action":         {"DecodeAuthorizationMessage"},
		"Version":        {"2011-06-15"},
		"EncodedMessage": {encoded},
	}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	ctxWithLogger := logger.Save(req.Context(), nil)
	req = req.WithContext(ctxWithLogger)

	err := h.Handler()(e.NewContext(req, rec))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		XMLName xml.Name `xml:"DecodeAuthorizationMessageResponse"`
		Result  struct {
			DecodedMessage string `xml:"DecodedMessage"`
		} `xml:"DecodeAuthorizationMessageResult"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, original, resp.Result.DecodedMessage)
}

// TestDecodeAuthorizationMessageEmpty verifies a missing EncodedMessage gives
// 400 MissingParameter. Previously asserted the sibling code "InvalidParameter"
// -- gopherstack-yatn orphan-code triage found "InvalidParameter" does not
// exist anywhere in the pinned sts@v1.45.4 module, and AWS's own STS Common
// Errors page (docs.aws.amazon.com/STS/latest/APIReference/CommonErrors.html)
// documents "MissingParameter" (not "InvalidParameter") for exactly this
// condition ("A required parameter for the specified action isn't included
// in the request"), matching every other missing-required-parameter sentinel
// in this package (ErrMissingRoleArn et al., handler.go's
// mapValidationErrorToCode).
func TestDecodeAuthorizationMessageEmpty(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	h := sts.NewHandler(b)

	rec := r1PostForm(t, h, url.Values{
		"Action":  {"DecodeAuthorizationMessage"},
		"Version": {"2011-06-15"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp sts.ErrorResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "MissingParameter", errResp.Error.Code)
}

// TestDecodeAuthorizationMessageInvalidBase64Returns400 verifies malformed
// base64 blobs are rejected with InvalidAuthorizationMessageException.
func TestDecodeAuthorizationMessageInvalidBase64Returns400(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		encoded string
	}{
		{
			name:    "garbage_string",
			encoded: "this-is-not-base64!!!",
		},
		{
			name:    "truncated_base64",
			encoded: "SGVsbG8=truncated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _, e := accuracyHandler(t)
			form := url.Values{
				"Action":         {"DecodeAuthorizationMessage"},
				"Version":        {"2011-06-15"},
				"EncodedMessage": {tt.encoded},
			}
			rec := accuracyPost(t, h, e, form)
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			errResp := decodeError(t, rec.Body.Bytes())
			assert.Equal(t, "InvalidAuthorizationMessageException", errResp.Error.Code)
		})
	}
}

// TestDecodeAuthorizationMessageVerifiesIssuer verifies that only messages
// issued by IssueEncodedAuthorizationMessage on the same backend are accepted.
func TestDecodeAuthorizationMessageVerifiesIssuer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupMsg    func(b *sts.InMemoryBackend) string
		wantErr     error
		wantDecoded string
	}{
		{
			name: "sts_issued_message_accepted_and_decoded",
			setupMsg: func(b *sts.InMemoryBackend) string {
				return b.IssueEncodedAuthorizationMessage(
					"Access denied to s3:GetObject on arn:aws:s3:::my-bucket/secret",
				)
			},
			wantDecoded: "Access denied to s3:GetObject on arn:aws:s3:::my-bucket/secret",
		},
		{
			name: "arbitrary_base64_rejected",
			setupMsg: func(_ *sts.InMemoryBackend) string {
				return "SGVsbG8gV29ybGQ=" // base64("Hello World") — not STS-issued
			},
			wantErr: sts.ErrInvalidAuthorizationMessage,
		},
		{
			name: "empty_message_issued_and_decoded",
			setupMsg: func(b *sts.InMemoryBackend) string {
				return b.IssueEncodedAuthorizationMessage("")
			},
			wantDecoded: "",
		},
		{
			name: "message_from_different_backend_rejected",
			setupMsg: func(_ *sts.InMemoryBackend) string {
				// Issue from a different backend instance — different signing key.
				other := sts.NewInMemoryBackend()

				return other.IssueEncodedAuthorizationMessage("cross-backend message")
			},
			wantErr: sts.ErrInvalidAuthorizationMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			encoded := tt.setupMsg(b)

			decoded, err := b.VerifyEncodedAuthorizationMessage(encoded)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantDecoded, decoded)
			}
		})
	}
}

// TestDecodeAuthorizationMessageViaHandler exercises DecodeAuthorizationMessage through the HTTP handler.
func TestDecodeAuthorizationMessageViaHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupMsg  func(b *sts.InMemoryBackend) string
		name      string
		wantError string
		wantCode  int
	}{
		{
			name: "sts_issued_message_returns_200",
			setupMsg: func(b *sts.InMemoryBackend) string {
				return b.IssueEncodedAuthorizationMessage("denied: s3:PutObject")
			},
			wantCode: http.StatusOK,
		},
		{
			name: "arbitrary_base64_returns_200",
			setupMsg: func(_ *sts.InMemoryBackend) string {
				return "SGVsbG8=" // base64("Hello")
			},
			wantCode: http.StatusOK,
		},
		{
			name: "garbage_non_base64_returns_400",
			setupMsg: func(_ *sts.InMemoryBackend) string {
				return "not-base64!!!"
			},
			wantCode:  http.StatusBadRequest,
			wantError: "InvalidAuthorizationMessageException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b, e := accuracyHandler(t)
			encoded := tt.setupMsg(b)

			form := url.Values{
				"Action":         {"DecodeAuthorizationMessage"},
				"Version":        {"2011-06-15"},
				"EncodedMessage": {encoded},
			}
			rec := accuracyPost(t, h, e, form)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantError != "" {
				errResp := decodeError(t, rec.Body.Bytes())
				assert.Equal(t, tt.wantError, errResp.Error.Code)
			}
		})
	}
}
