package sts_test

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sts"
)

func TestGetDelegatedAccessToken_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input *sts.GetDelegatedAccessTokenInput
		name  string
	}{
		{
			name:  "valid_trade_in_token",
			input: &sts.GetDelegatedAccessTokenInput{TradeInToken: "some-trade-in-token"},
		},
		{
			name:  "another_valid_token",
			input: &sts.GetDelegatedAccessTokenInput{TradeInToken: "token-abc-123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.GetDelegatedAccessToken(tt.input)
			require.NoError(t, err)
			require.NotNil(t, resp)

			res := resp.GetDelegatedAccessTokenResult
			assert.True(t, strings.HasPrefix(res.Credentials.AccessKeyID, "ASIA"))
			assert.NotEmpty(t, res.Credentials.SecretAccessKey)
			assert.NotEmpty(t, res.Credentials.SessionToken)
			assert.NotEmpty(t, res.Credentials.Expiration)
			assert.NotEmpty(t, res.AssumedPrincipal)
			assert.NotEmpty(t, resp.ResponseMetadata.RequestID)
			assert.Equal(t, sts.STSNamespace, resp.Xmlns)
		})
	}
}

func TestGetDelegatedAccessToken_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		input   *sts.GetDelegatedAccessTokenInput
		name    string
	}{
		{
			name:    "missing_trade_in_token",
			input:   &sts.GetDelegatedAccessTokenInput{},
			wantErr: sts.ErrMissingTradeInToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			_, err := b.GetDelegatedAccessToken(tt.input)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestGetDelegatedAccessToken_TradeInTokenExpiry verifies that a JWT-shaped
// TradeInToken is checked for expiry (AWS ExpiredTradeInTokenException),
// mirroring the real GetDelegatedAccessToken behaviour documented by
// aws-sdk-go-v2/service/sts/types.ExpiredTradeInTokenException: "The trade-in
// token provided in the request has expired". Opaque (non-JWT) tokens remain
// accepted unchanged, matching the emulator's existing permissive handling of
// simple test fixtures that have no verifiable structure.
func TestGetDelegatedAccessToken_TradeInTokenExpiry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr    error
		buildToken func(t *testing.T) string
		name       string
	}{
		{
			name: "expired_jwt_shaped_token_rejected",
			buildToken: func(t *testing.T) string {
				t.Helper()

				return buildJWT(t, map[string]any{
					"sub": "delegate",
					"exp": time.Now().Add(-time.Hour).Unix(),
				})
			},
			wantErr: sts.ErrExpiredTradeInToken,
		},
		{
			name: "unexpired_jwt_shaped_token_accepted",
			buildToken: func(t *testing.T) string {
				t.Helper()

				return buildJWT(t, map[string]any{
					"sub": "delegate",
					"exp": time.Now().Add(time.Hour).Unix(),
				})
			},
		},
		{
			name: "opaque_non_jwt_token_accepted",
			buildToken: func(*testing.T) string {
				return "opaque-trade-in-token-without-dots"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.GetDelegatedAccessToken(&sts.GetDelegatedAccessTokenInput{
				TradeInToken: tt.buildToken(t),
			})

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, resp.GetDelegatedAccessTokenResult.Credentials.AccessKeyID)
		})
	}
}

func TestGetDelegatedAccessToken_SessionTrackedForCallerIdentity(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.GetDelegatedAccessToken(&sts.GetDelegatedAccessTokenInput{TradeInToken: "my-token"})
	require.NoError(t, err)

	creds := resp.GetDelegatedAccessTokenResult.Credentials

	ciResp, err := b.GetCallerIdentity(creds.AccessKeyID, creds.SessionToken)
	require.NoError(t, err)
	assert.NotEmpty(t, ciResp.GetCallerIdentityResult.Arn)
	assert.Equal(t, 1, b.SessionCount())
}

func TestHandler_GetDelegatedAccessToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		formValues url.Values
		name       string
		wantCode   string
		wantStatus int
	}{
		{
			name: "success",
			formValues: url.Values{
				"Action":       {"GetDelegatedAccessToken"},
				"Version":      {"2011-06-15"},
				"TradeInToken": {"some-trade-in-token"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_trade_in_token_returns_400",
			formValues: url.Values{
				"Action":  {"GetDelegatedAccessToken"},
				"Version": {"2011-06-15"},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MissingParameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			h := sts.NewHandler(b)
			e := echo.New()

			rec := postForm(t, e, h, tt.formValues)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantCode != "" {
				var errResp sts.ErrorResponse
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Equal(t, tt.wantCode, errResp.Error.Code)
			} else {
				var resp sts.GetDelegatedAccessTokenResponse
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp.GetDelegatedAccessTokenResult.Credentials.AccessKeyID)
				assert.NotEmpty(t, resp.GetDelegatedAccessTokenResult.AssumedPrincipal)
			}
		})
	}
}

// TestDelegatedTokenDuration verifies GetDelegatedAccessToken issues credentials
// with a fixed lifetime, since GetDelegatedAccessTokenInput (unlike AssumeRole,
// GetSessionToken, etc.) carries no DurationSeconds member in the real AWS API —
// the caller cannot influence the credential lifetime for this operation.
func TestDelegatedTokenDuration(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()

	before := time.Now().UTC()

	resp, err := b.GetDelegatedAccessToken(&sts.GetDelegatedAccessTokenInput{
		TradeInToken: "test-token",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	expiration, err := time.Parse(time.RFC3339, resp.GetDelegatedAccessTokenResult.Credentials.Expiration)
	require.NoError(t, err)

	gotDuration := expiration.Sub(before)
	assert.InDelta(t, sts.DefaultDurationSeconds, gotDuration.Seconds(), 5)
}

// TestGetDelegatedAccessTokenResultBeforeMetadata verifies the XML result element precedes ResponseMetadata.
func TestGetDelegatedAccessTokenResultBeforeMetadata(t *testing.T) {
	t.Parallel()

	h, _, e := accuracyHandler(t)
	form := url.Values{
		"Action":       {"GetDelegatedAccessToken"},
		"Version":      {"2011-06-15"},
		"TradeInToken": {"my-token"},
	}
	rec := accuracyPost(t, h, e, form)
	require.Equal(t, http.StatusOK, rec.Code)

	order := xmlElementOrder(t, rec.Body.Bytes())
	require.Len(t, order, 2)
	assert.Equal(t, "GetDelegatedAccessTokenResult", order[0])
	assert.Equal(t, "ResponseMetadata", order[1])
}
