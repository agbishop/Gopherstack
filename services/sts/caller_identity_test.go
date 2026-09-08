package sts_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/services/sts"
)

func TestGetCallerIdentity(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	resp, err := backend.GetCallerIdentity("", "")
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, sts.MockAccountID, resp.GetCallerIdentityResult.Account)
	assert.Equal(t, sts.MockUserArn, resp.GetCallerIdentityResult.Arn)
	assert.Equal(t, sts.MockUserID, resp.GetCallerIdentityResult.UserID)
	assert.NotEmpty(t, resp.ResponseMetadata.RequestID)
	assert.Equal(t, sts.STSNamespace, resp.Xmlns)
}

func TestHandler_GetCallerIdentity(t *testing.T) {
	t.Parallel()

	h, e := newTestHandler(t)
	rec := postForm(t, e, h, url.Values{
		"Action":  {"GetCallerIdentity"},
		"Version": {"2011-06-15"},
	})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/xml")

	var resp sts.GetCallerIdentityResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, sts.MockAccountID, resp.GetCallerIdentityResult.Account)
	assert.Equal(t, sts.MockUserArn, resp.GetCallerIdentityResult.Arn)
}

// TestGetCallerIdentity_AssumedRole_ReturnsAssumedRoleArn verifies that an
// assumed-role access key resolves to the assumed-role ARN and AROA user ID.
func TestGetCallerIdentity_AssumedRole_ReturnsAssumedRoleArn(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	assumeResp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
		RoleSessionName: "my-session",
		SourceIdentity:  "caller",
	})
	require.NoError(t, err)

	creds := assumeResp.AssumeRoleResult.Credentials

	ciResp, err := backend.GetCallerIdentity(creds.AccessKeyID, creds.SessionToken)
	require.NoError(t, err)

	assert.Equal(t, "123456789012", ciResp.GetCallerIdentityResult.Account)
	assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "assumed-role")
	assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "TestRole")
	assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "my-session")
	assert.Truef(t, strings.HasPrefix(ciResp.GetCallerIdentityResult.UserID, "AROA"),
		"expected UserID to start with AROA, got %s", ciResp.GetCallerIdentityResult.UserID)
	assert.Contains(t, ciResp.GetCallerIdentityResult.UserID, "my-session")
}

// TestGetCallerIdentity_UnknownAsiaKey_ReturnsInvalidClientTokenId verifies an
// unknown ASIA-prefixed key is rejected rather than falling back to the root identity.
func TestGetCallerIdentity_UnknownAsiaKey_ReturnsInvalidClientTokenId(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	// ASIA-prefixed keys are temporary session credentials. AWS returns
	// InvalidClientTokenId when such a key is not found in the session store.
	_, err := backend.GetCallerIdentity("ASIANOTISSUED1234567", "")
	require.ErrorIs(t, err, sts.ErrUnknownAccessKeyID)
}

func TestGetCallerIdentity_EmptyAccessKey_ReturnsDefaultIdentity(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	resp, err := backend.GetCallerIdentity("", "")
	require.NoError(t, err)

	assert.Equal(t, sts.MockAccountID, resp.GetCallerIdentityResult.Account)
	assert.Equal(t, sts.MockUserArn, resp.GetCallerIdentityResult.Arn)
	assert.Equal(t, sts.MockUserID, resp.GetCallerIdentityResult.UserID)
}

func TestHandler_GetCallerIdentity_WithAssumedRoleCredentials(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()
	h := sts.NewHandler(backend)
	e := echo.New()

	// First: AssumeRole to get a credential.
	assumeBody := url.Values{
		"Action":          {"AssumeRole"},
		"Version":         {"2011-06-15"},
		"RoleArn":         {"arn:aws:iam::123456789012:role/MyRole"},
		"RoleSessionName": {"my-session"},
	}.Encode()
	assumeReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(assumeBody))
	assumeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	assumeReq = assumeReq.WithContext(logger.Save(assumeReq.Context(), logger.NewTestLogger()))
	assumeRec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(assumeReq, assumeRec)))
	require.Equal(t, http.StatusOK, assumeRec.Code)

	var assumeResp sts.AssumeRoleResponse
	require.NoError(t, xml.Unmarshal(assumeRec.Body.Bytes(), &assumeResp))
	creds := assumeResp.AssumeRoleResult.Credentials

	// Second: GetCallerIdentity with the assumed-role access key and its session
	// token in Authorization/X-Amz-Security-Token, as a real SigV4-signed request
	// for temporary credentials always carries both.
	ciBody := "Action=GetCallerIdentity&Version=2011-06-15"
	ciReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(ciBody))
	ciReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ciReq.Header.Set(
		"Authorization",
		fmt.Sprintf(
			"AWS4-HMAC-SHA256 Credential=%s/20230101/us-east-1/sts/aws4_request, SignedHeaders=host, Signature=abc",
			creds.AccessKeyID,
		),
	)
	ciReq.Header.Set("X-Amz-Security-Token", creds.SessionToken)
	ciReq = ciReq.WithContext(logger.Save(ciReq.Context(), logger.NewTestLogger()))
	ciRec := httptest.NewRecorder()
	require.NoError(t, h.Handler()(e.NewContext(ciReq, ciRec)))
	require.Equal(t, http.StatusOK, ciRec.Code)

	var ciResp sts.GetCallerIdentityResponse
	require.NoError(t, xml.Unmarshal(ciRec.Body.Bytes(), &ciResp))
	assert.Equal(t, "123456789012", ciResp.GetCallerIdentityResult.Account)
	assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "assumed-role")
	assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "MyRole")
	assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "my-session")
}

// TestGetCallerIdentity_SessionTokenMismatch verifies that a mismatched session token
// is rejected as InvalidClientTokenId (HTTP 400 via ErrUnknownAccessKeyID), matching AWS,
// rather than AccessDenied (403).
func TestGetCallerIdentity_SessionTokenMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrKind error
		name        string
		wrongToken  string
		useRealTok  bool
		wantErr     bool
	}{
		{name: "matching_token", useRealTok: true, wantErr: false},
		{name: "mismatched_token", wrongToken: "wrong-token", wantErr: true, wantErrKind: sts.ErrUnknownAccessKeyID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.GetSessionToken(&sts.GetSessionTokenInput{})
			require.NoError(t, err)

			accessKeyID := resp.GetSessionTokenResult.Credentials.AccessKeyID
			token := tt.wrongToken
			if tt.useRealTok {
				token = resp.GetSessionTokenResult.Credentials.SessionToken
			}

			_, err = b.GetCallerIdentity(accessKeyID, token)
			if tt.wantErr {
				require.ErrorIs(t, err, tt.wantErrKind)

				return
			}
			require.NoError(t, err)
		})
	}
}

// TestGetCallerIdentitySessionToken exercises X-Amz-Security-Token validation
// against a stored session (Accuracy Gap #10).
func TestGetCallerIdentitySessionToken(t *testing.T) {
	t.Parallel()

	t.Run("correct_session_token_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		// Issue a session.
		assumeResp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
			RoleSessionName: "my-session",
		})
		require.NoError(t, err)

		accessKeyID := assumeResp.AssumeRoleResult.Credentials.AccessKeyID
		sessionToken := assumeResp.AssumeRoleResult.Credentials.SessionToken

		// GetCallerIdentity with matching session token.
		resp, err := b.GetCallerIdentity(accessKeyID, sessionToken)
		require.NoError(t, err)
		assert.Contains(t, resp.GetCallerIdentityResult.Arn, "assumed-role")
	})

	t.Run("wrong_session_token_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		assumeResp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
			RoleSessionName: "my-session",
		})
		require.NoError(t, err)

		accessKeyID := assumeResp.AssumeRoleResult.Credentials.AccessKeyID

		// AWS rejects a mismatched session token with InvalidClientTokenId (HTTP 400),
		// surfaced here as ErrUnknownAccessKeyID, not AccessDenied.
		_, err = b.GetCallerIdentity(accessKeyID, "wrong-token")
		require.ErrorIs(t, err, sts.ErrUnknownAccessKeyID)
	})

	// TestGetCallerIdentitySessionToken/missing_session_token_rejected guards
	// against gopherstack-g58j: an absent X-Amz-Security-Token must be rejected
	// the same way a wrong one is, not treated as an automatic match that lets
	// the bare ASIA access key ID impersonate the session.
	t.Run("missing_session_token_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		assumeResp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
			RoleSessionName: "my-session",
		})
		require.NoError(t, err)

		accessKeyID := assumeResp.AssumeRoleResult.Credentials.AccessKeyID

		_, err = b.GetCallerIdentity(accessKeyID, "")
		require.ErrorIs(t, err, sts.ErrUnknownAccessKeyID)
	})

	t.Run("http_request_uses_x_amz_security_token_header", func(t *testing.T) {
		t.Parallel()

		h, b, e := accuracyHandler(t)
		assumeResp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
			RoleSessionName: "session",
		})
		require.NoError(t, err)

		accessKeyID := assumeResp.AssumeRoleResult.Credentials.AccessKeyID
		sessionToken := assumeResp.AssumeRoleResult.Credentials.SessionToken

		form := url.Values{
			"Action":  {"GetCallerIdentity"},
			"Version": {"2011-06-15"},
		}
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set(
			"Authorization",
			"AWS4-HMAC-SHA256 Credential="+accessKeyID+"/20240101/us-east-1/sts/aws4_request",
		)
		req.Header.Set("X-Amz-Security-Token", sessionToken)
		rec := httptest.NewRecorder()
		req = req.WithContext(logger.Save(req.Context(), nil))

		require.NoError(t, h.Handler()(e.NewContext(req, rec)))
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp sts.GetCallerIdentityResponse
		require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, resp.GetCallerIdentityResult.Arn, "assumed-role")
		assert.Contains(t, resp.GetCallerIdentityResult.Arn, "TestRole")
	})

	t.Run("http_request_wrong_x_amz_security_token_returns_400", func(t *testing.T) {
		t.Parallel()

		h, b, e := accuracyHandler(t)
		assumeResp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
			RoleSessionName: "session",
		})
		require.NoError(t, err)

		accessKeyID := assumeResp.AssumeRoleResult.Credentials.AccessKeyID

		form := url.Values{
			"Action":  {"GetCallerIdentity"},
			"Version": {"2011-06-15"},
		}
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set(
			"Authorization",
			"AWS4-HMAC-SHA256 Credential="+accessKeyID+"/20240101/us-east-1/sts/aws4_request",
		)
		req.Header.Set("X-Amz-Security-Token", "bad-token")
		rec := httptest.NewRecorder()
		req = req.WithContext(logger.Save(req.Context(), nil))

		require.NoError(t, h.Handler()(e.NewContext(req, rec)))
		// AWS returns InvalidClientTokenId (HTTP 400) for a mismatched session token.
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "InvalidClientTokenId")
	})
}

// TestValidateSessionCredential verifies (accessKeyID, sessionToken) lookup
// behaviour (Accuracy Gap #11).
func TestValidateSessionCredential(t *testing.T) {
	t.Parallel()

	t.Run("valid_credential_returns_session_info", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		assumeResp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
			RoleSessionName: "my-session",
		})
		require.NoError(t, err)

		creds := assumeResp.AssumeRoleResult.Credentials
		info, err := b.ValidateSessionCredential(creds.AccessKeyID, creds.SessionToken)
		require.NoError(t, err)
		assert.Contains(t, info.AssumedRoleArn, "assumed-role")
		assert.Equal(t, "123456789012", info.AccountID)
	})

	t.Run("unknown_key_returns_session_not_found", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.ValidateSessionCredential("ASIANOTEXIST123456789", "any-token")
		require.ErrorIs(t, err, sts.ErrSessionNotFound)
	})

	t.Run("wrong_token_returns_access_denied", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		assumeResp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
			RoleSessionName: "my-session",
		})
		require.NoError(t, err)

		creds := assumeResp.AssumeRoleResult.Credentials
		_, err = b.ValidateSessionCredential(creds.AccessKeyID, "wrong-token")
		require.ErrorIs(t, err, sts.ErrAccessDenied)
	})

	t.Run("expired_session_returns_not_found", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		assumeResp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
			RoleSessionName: "my-session",
		})
		require.NoError(t, err)

		creds := assumeResp.AssumeRoleResult.Credentials
		b.SetSessionExpiration(creds.AccessKeyID, time.Now().Add(-time.Minute))

		_, err = b.ValidateSessionCredential(creds.AccessKeyID, creds.SessionToken)
		require.ErrorIs(t, err, sts.ErrSessionNotFound)
	})

	t.Run("session_stores_secret_access_key", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		assumeResp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
			RoleSessionName: "my-session",
		})
		require.NoError(t, err)

		creds := assumeResp.AssumeRoleResult.Credentials
		info, err := b.ValidateSessionCredential(creds.AccessKeyID, creds.SessionToken)
		require.NoError(t, err)
		assert.Equal(t, creds.SecretAccessKey, info.SecretAccessKey)
		assert.Equal(t, creds.SessionToken, info.SessionToken)
	})
}

// TestGetCallerIdentityUnknownAccessKeyMatrix covers the broader matrix of
// unknown/empty/long-term access keys passed to GetCallerIdentity.
func TestGetCallerIdentityUnknownAccessKeyMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		name      string
		accessKey string
	}{
		{
			name:      "asia_key_never_issued_returns_InvalidClientTokenId",
			accessKey: "ASIANEVERISSUED1234",
			wantErr:   sts.ErrUnknownAccessKeyID,
		},
		{
			name:      "empty_key_returns_root_identity",
			accessKey: "",
			wantErr:   nil,
		},
		{
			name:      "akia_key_unknown_returns_root_identity",
			accessKey: "AKIAIOSFODNN7EXAMPLE",
			wantErr:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.GetCallerIdentity(tt.accessKey, "")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, sts.MockAccountID, resp.GetCallerIdentityResult.Account)
			}
		})
	}
}
