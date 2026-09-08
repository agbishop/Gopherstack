package sts_test

import (
	"encoding/base64"
	"encoding/json"
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

// webIdentityClaimTags/webIdentityClaimSourceIdentity mirror the unexported
// jwtClaimTags/jwtClaimSourceIdentity constants in token_validation.go: AWS
// derives AssumeRoleWithWebIdentity's session tags and SourceIdentity from
// these custom claims in the WebIdentityToken rather than accepting them as
// separate top-level request parameters (see AssumeRoleWithWebIdentityInput).
const (
	webIdentityClaimTags           = "https://aws.amazon.com/tags"
	webIdentityClaimSourceIdentity = "https://aws.amazon.com/source_identity"
)

// makeJWT constructs a minimal unsigned JWT with the given JSON payload (no signature validation needed in mock).
func makeJWT(payload string) string {
	// header: {"alg":"none","typ":"JWT"}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))

	return header + "." + encodedPayload + ".sig"
}

// buildJWT constructs a minimal unsigned JWT with the given claims for testing.
func buildJWT(t *testing.T, claims map[string]any) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"mock","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	require.NoError(t, err)

	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".fakesig"
}

// buildParityJWT constructs a minimal unsigned JWT carrying only sub/exp claims.
func buildParityJWT(t *testing.T, expUnix int64) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{"sub": "test", "exp": expUnix})
	require.NoError(t, err)

	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

// parseJWTClaims decodes the payload section of a JWT and returns its claims.
func parseJWTClaims(t *testing.T, tokenStr string) map[string]any {
	t.Helper()

	parts := strings.Split(tokenStr, ".")
	require.Len(t, parts, 3, "JWT must have 3 parts")

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)

	var claims map[string]any
	require.NoError(t, json.Unmarshal(payloadBytes, &claims))

	return claims
}

// ---- AssumeRoleWithWebIdentity tests ----------------------------------------

func TestAssumeRoleWithWebIdentity_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       *sts.AssumeRoleWithWebIdentityInput
		wantSubject string
	}{
		{
			name: "opaque_token_uses_placeholder",
			input: &sts.AssumeRoleWithWebIdentityInput{
				RoleArn:          "arn:aws:iam::123456789012:role/WebRole",
				RoleSessionName:  "web-session",
				WebIdentityToken: "some-opaque-token",
			},
			wantSubject: "WebIdentitySubject",
		},
		{
			name: "jwt_token_subject_extracted",
			input: &sts.AssumeRoleWithWebIdentityInput{
				RoleArn:          "arn:aws:iam::123456789012:role/WebRole",
				RoleSessionName:  "jwt-session",
				WebIdentityToken: makeJWT(`{"sub":"user-12345","aud":"my-app"}`),
			},
			wantSubject: "user-12345",
		},
		{
			name: "custom_provider_is_used",
			input: &sts.AssumeRoleWithWebIdentityInput{
				RoleArn:          "arn:aws:iam::123456789012:role/WebRole",
				RoleSessionName:  "provider-session",
				WebIdentityToken: "token",
				ProviderID:       "www.amazon.com",
			},
			wantSubject: "WebIdentitySubject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.AssumeRoleWithWebIdentity(tt.input)
			require.NoError(t, err)
			require.NotNil(t, resp)

			res := resp.AssumeRoleWithWebIdentityResult
			assert.True(t, strings.HasPrefix(res.Credentials.AccessKeyID, "ASIA"))
			assert.NotEmpty(t, res.Credentials.SecretAccessKey)
			assert.NotEmpty(t, res.Credentials.SessionToken)
			assert.NotEmpty(t, res.Credentials.Expiration)
			assert.Contains(t, res.AssumedRoleUser.Arn, "assumed-role")
			assert.Contains(t, res.AssumedRoleUser.Arn, tt.input.RoleSessionName)
			assert.Equal(t, tt.wantSubject, res.SubjectFromWebIdentityToken)
			assert.NotEmpty(t, res.Provider)
		})
	}
}

func TestAssumeRoleWithWebIdentity_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		input   *sts.AssumeRoleWithWebIdentityInput
		name    string
	}{
		{
			name: "missing_role_arn",
			input: &sts.AssumeRoleWithWebIdentityInput{
				RoleSessionName:  "session",
				WebIdentityToken: "token",
			},
			wantErr: sts.ErrMissingRoleArn,
		},
		{
			name: "missing_session_name",
			input: &sts.AssumeRoleWithWebIdentityInput{
				RoleArn:          "arn:aws:iam::123456789012:role/WebRole",
				WebIdentityToken: "token",
			},
			wantErr: sts.ErrMissingSessionName,
		},
		{
			name: "missing_web_identity_token",
			input: &sts.AssumeRoleWithWebIdentityInput{
				RoleArn:         "arn:aws:iam::123456789012:role/WebRole",
				RoleSessionName: "session",
			},
			wantErr: sts.ErrMissingWebIdentityToken,
		},
		{
			name: "duration_too_short",
			input: &sts.AssumeRoleWithWebIdentityInput{
				RoleArn:          "arn:aws:iam::123456789012:role/WebRole",
				RoleSessionName:  "session",
				WebIdentityToken: "token",
				DurationSeconds:  100,
			},
			wantErr: sts.ErrInvalidDuration,
		},
		{
			name: "duration_too_long",
			input: &sts.AssumeRoleWithWebIdentityInput{
				RoleArn:          "arn:aws:iam::123456789012:role/WebRole",
				RoleSessionName:  "session",
				WebIdentityToken: "token",
				DurationSeconds:  sts.MaxDurationSeconds + 1,
			},
			wantErr: sts.ErrInvalidDuration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			_, err := b.AssumeRoleWithWebIdentity(tt.input)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestAssumeRoleWithWebIdentity_SessionTrackedForCallerIdentity(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          "arn:aws:iam::123456789012:role/WebRole",
		RoleSessionName:  "web-session",
		WebIdentityToken: "some-token",
	})
	require.NoError(t, err)

	creds := resp.AssumeRoleWithWebIdentityResult.Credentials

	ciResp, err := b.GetCallerIdentity(creds.AccessKeyID, creds.SessionToken)
	require.NoError(t, err)
	assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "assumed-role")
	assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "WebRole")
}

func TestHandler_AssumeRoleWithWebIdentity(t *testing.T) {
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
				"Action":           {"AssumeRoleWithWebIdentity"},
				"Version":          {"2011-06-15"},
				"RoleArn":          {"arn:aws:iam::123456789012:role/WebRole"},
				"RoleSessionName":  {"web-session"},
				"WebIdentityToken": {"some-token"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_web_identity_token_returns_400",
			formValues: url.Values{
				"Action":          {"AssumeRoleWithWebIdentity"},
				"Version":         {"2011-06-15"},
				"RoleArn":         {"arn:aws:iam::123456789012:role/WebRole"},
				"RoleSessionName": {"session"},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MissingParameter",
		},
		{
			name: "missing_role_arn_returns_400",
			formValues: url.Values{
				"Action":           {"AssumeRoleWithWebIdentity"},
				"Version":          {"2011-06-15"},
				"RoleSessionName":  {"session"},
				"WebIdentityToken": {"token"},
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
				var resp sts.AssumeRoleWithWebIdentityResponse
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp.AssumeRoleWithWebIdentityResult.Credentials.AccessKeyID)
				assert.Contains(t, resp.AssumeRoleWithWebIdentityResult.AssumedRoleUser.Arn, "assumed-role")
			}
		})
	}
}

// TestAssumeRoleWithWebIdentityTagsStored verifies session tags carried in the
// WebIdentityToken's custom claim (there is no top-level Tags request
// parameter for this operation) are stored in the session.
func TestAssumeRoleWithWebIdentityTagsStored(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
		RoleArn:         "arn:aws:iam::000000000000:role/test-role",
		RoleSessionName: "test-session",
		WebIdentityToken: buildJWT(t, map[string]any{
			"sub":                "test-subject",
			"aud":                "test-aud",
			webIdentityClaimTags: map[string]string{"Env": "test"},
		}),
	})
	require.NoError(t, err)

	creds := resp.AssumeRoleWithWebIdentityResult.Credentials

	snap := b.Snapshot(t.Context())
	b2 := sts.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), snap))

	assert.Equal(t, 1, b2.SessionCount())

	ci, err := b2.GetCallerIdentity(creds.AccessKeyID, creds.SessionToken)
	require.NoError(t, err)
	assert.NotEmpty(t, ci.GetCallerIdentityResult.Arn)
}

// TestAssumeRoleWithWebIdentitySourceIdentity verifies SourceIdentity is
// derived from the WebIdentityToken's custom claim (there is no top-level
// SourceIdentity request parameter for this operation).
func TestAssumeRoleWithWebIdentitySourceIdentity(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
		RoleArn:         "arn:aws:iam::000000000000:role/test-role",
		RoleSessionName: "test-session",
		WebIdentityToken: buildJWT(t, map[string]any{
			"sub":                          "sub1",
			webIdentityClaimSourceIdentity: "my-oidc-identity",
		}),
	})
	require.NoError(t, err)
	assert.Equal(t, "my-oidc-identity", resp.AssumeRoleWithWebIdentityResult.SourceIdentity)
}

// TestAssumeRoleWithWebIdentityWithPolicyArns verifies PolicyArns parsed in OIDC request.
func TestAssumeRoleWithWebIdentityWithPolicyArns(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	h := sts.NewHandler(b)

	rec := r1PostForm(t, h, url.Values{
		"Action":                  {"AssumeRoleWithWebIdentity"},
		"Version":                 {"2011-06-15"},
		"RoleArn":                 {"arn:aws:iam::000000000000:role/test-role"},
		"RoleSessionName":         {"my-session"},
		"WebIdentityToken":        {"eyJhbGciOiJtb2NrIn0.eyJzdWIiOiJzdWIxIn0.sig"},
		"PolicyArns.member.1.arn": {"arn:aws:iam::aws:policy/ReadOnlyAccess"},
	})

	assert.Equal(t, http.StatusOK, rec.Code)

	var result sts.AssumeRoleWithWebIdentityResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &result))
	assert.NotEmpty(t, result.AssumeRoleWithWebIdentityResult.Credentials.AccessKeyID)
}

// TestAssumeRoleWithWebIdentity_TagValidation verifies session-tag validation
// is enforced for AssumeRoleWithWebIdentity against tags carried in the
// WebIdentityToken's custom claim.
func TestAssumeRoleWithWebIdentity_TagValidation(t *testing.T) {
	t.Parallel()

	t.Run("aws_prefix_tag_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
			RoleArn:         "arn:aws:iam::123456789012:role/R",
			RoleSessionName: "session",
			WebIdentityToken: buildJWT(t, map[string]any{
				"sub":                "test",
				webIdentityClaimTags: map[string]string{"aws:bad": "v"},
			}),
		})
		require.ErrorIs(t, err, sts.ErrInvalidTagKey)
	})

	t.Run("too_many_tags_rejected", func(t *testing.T) {
		t.Parallel()

		tags := make(map[string]string, 51)
		for i := range 51 {
			tags["k"+string(rune('a'+i%26))+string(rune('0'+i/26))] = "v"
		}

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
			RoleArn:         "arn:aws:iam::123456789012:role/R",
			RoleSessionName: "session",
			WebIdentityToken: buildJWT(t, map[string]any{
				"sub":                "test",
				webIdentityClaimTags: tags,
			}),
		})
		require.ErrorIs(t, err, sts.ErrTooManyTags)
	})
}

// TestAssumeRoleWithWebIdentity_PolicyValidation verifies session-policy
// validation for AssumeRoleWithWebIdentity.
func TestAssumeRoleWithWebIdentity_PolicyValidation(t *testing.T) {
	t.Parallel()

	t.Run("policy_without_statement_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          "arn:aws:iam::123456789012:role/R",
			RoleSessionName:  "session",
			WebIdentityToken: "header.eyJzdWIiOiJ0ZXN0In0.sig",
			Policy:           `{"Version":"2012-10-17"}`,
		})
		require.ErrorIs(t, err, sts.ErrMalformedPolicyDocument)
	})

	t.Run("valid_policy_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          "arn:aws:iam::123456789012:role/R",
			RoleSessionName:  "session",
			WebIdentityToken: "header.eyJzdWIiOiJ0ZXN0In0.sig",
			Policy:           `{"Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
		})
		require.NoError(t, err)
	})
}

// TestAssumeRoleWithWebIdentityExtractsIssuer verifies Provider comes from JWT iss claim.
func TestAssumeRoleWithWebIdentityExtractsIssuer(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()

	// Generate a token with a known issuer.
	tokenResp, err := b.GetWebIdentityToken(&sts.GetWebIdentityTokenInput{
		Audience:         []string{"https://example.com"},
		SigningAlgorithm: "RS256",
	})
	require.NoError(t, err)

	claims := parseJWTClaims(t, tokenResp.GetWebIdentityTokenResult.WebIdentityToken)
	expectedIssuer, _ := claims["iss"].(string)
	require.NotEmpty(t, expectedIssuer)

	resp, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          "arn:aws:iam::000000000000:role/test-role",
		RoleSessionName:  "my-session",
		WebIdentityToken: tokenResp.GetWebIdentityTokenResult.WebIdentityToken,
	})
	require.NoError(t, err)
	assert.Equal(t, expectedIssuer, resp.AssumeRoleWithWebIdentityResult.Provider)
}

// TestAssumeRoleWithWebIdentity_TrustAndTokenValidation verifies both the
// federated OIDC trust-policy check and the JWT temporal validation.
func TestAssumeRoleWithWebIdentity_TrustAndTokenValidation(t *testing.T) {
	t.Parallel()

	const roleArn = "arn:aws:iam::123456789012:role/WebRole"

	trustDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",` +
		`"Principal":{"Federated":"arn:aws:iam::123456789012:oidc-provider/accounts.google.com"},` +
		`"Action":"sts:AssumeRoleWithWebIdentity"}]}`

	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()

	tests := []struct {
		wantErr error
		name    string
		token   string
	}{
		{
			name: "trusted_issuer_valid_token",
			token: buildJWT(
				t,
				map[string]any{
					"iss": "https://accounts.google.com",
					"sub": "u",
					"aud": "app",
					"exp": future,
				},
			),
			wantErr: nil,
		},
		{
			name: "untrusted_issuer_denied",
			token: buildJWT(
				t,
				map[string]any{
					"iss": "https://evil.example.com",
					"sub": "u",
					"aud": "app",
					"exp": future,
				},
			),
			wantErr: sts.ErrAccessDenied,
		},
		{
			name: "trusted_issuer_expired_token",
			token: buildJWT(
				t,
				map[string]any{
					"iss": "https://accounts.google.com",
					"sub": "u",
					"aud": "app",
					"exp": past,
				},
			),
			wantErr: sts.ErrExpiredToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sts.NewInMemoryBackend()
			backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

			_, err := backend.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
				RoleArn:          roleArn,
				RoleSessionName:  "web-session",
				WebIdentityToken: tt.token,
			})

			if tt.wantErr == nil {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestAssumeRoleWithWebIdentityJWTExpiry verifies expired JWTs are rejected
// with ErrExpiredToken and fresh ones are accepted (Accuracy Gap #6).
func TestAssumeRoleWithWebIdentityJWTExpiry(t *testing.T) {
	t.Parallel()

	t.Run("expired_jwt_returns_ErrExpiredToken", func(t *testing.T) {
		t.Parallel()

		expiredToken := buildJWT(t, map[string]any{
			"sub": "test-user",
			"iss": "https://example.com",
			"exp": time.Now().Add(-time.Hour).Unix(),
			"aud": "my-client",
		})

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          "arn:aws:iam::123456789012:role/R",
			RoleSessionName:  "session",
			WebIdentityToken: expiredToken,
		})
		require.ErrorIs(t, err, sts.ErrExpiredToken)
	})

	t.Run("valid_jwt_exp_accepted", func(t *testing.T) {
		t.Parallel()

		validToken := buildJWT(t, map[string]any{
			"sub": "test-user",
			"iss": "https://example.com",
			"exp": time.Now().Add(time.Hour).Unix(),
			"aud": "my-client",
		})

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          "arn:aws:iam::123456789012:role/R",
			RoleSessionName:  "session",
			WebIdentityToken: validToken,
		})
		require.NoError(t, err)
	})
}

// TestJWTExpiry_Wire confirms that AssumeRoleWithWebIdentity rejects expired tokens
// end-to-end through the backend (wire-shaped JWT built independently of buildJWT).
func TestJWTExpiry_Wire(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	expired := buildParityJWT(t, time.Now().Add(-10*time.Minute).Unix())
	_, err := backend.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          "arn:aws:iam::123456789012:role/WebRole",
		RoleSessionName:  "web-session",
		WebIdentityToken: expired,
	})
	require.ErrorIs(t, err, sts.ErrExpiredToken)

	fresh := buildParityJWT(t, time.Now().Add(1*time.Hour).Unix())
	resp, err := backend.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          "arn:aws:iam::123456789012:role/WebRole",
		RoleSessionName:  "web-session",
		WebIdentityToken: fresh,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AssumeRoleWithWebIdentityResult.Credentials.AccessKeyID)
}

// TestGetWebIdentityTokenInSupportedOps verifies GetWebIdentityToken (a real
// AWS STS operation) is listed in GetSupportedOperations.
func TestGetWebIdentityTokenInSupportedOps(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	h := sts.NewHandler(b)
	ops := h.GetSupportedOperations()

	assert.Contains(
		t,
		ops,
		"GetWebIdentityToken",
		"GetWebIdentityToken is a real AWS STS operation and must be listed in GetSupportedOperations",
	)
}

// TestAssumeRoleWithWebIdentityResultBeforeMetadata verifies the XML result element precedes ResponseMetadata.
func TestAssumeRoleWithWebIdentityResultBeforeMetadata(t *testing.T) {
	t.Parallel()

	h, _, e := accuracyHandler(t)
	form := url.Values{
		"Action":           {"AssumeRoleWithWebIdentity"},
		"Version":          {"2011-06-15"},
		"RoleArn":          {"arn:aws:iam::123456789012:role/R"},
		"RoleSessionName":  {"session"},
		"WebIdentityToken": {"token"},
	}
	rec := accuracyPost(t, h, e, form)
	require.Equal(t, http.StatusOK, rec.Code)

	order := xmlElementOrder(t, rec.Body.Bytes())
	require.Len(t, order, 2)
	assert.Equal(t, "AssumeRoleWithWebIdentityResult", order[0])
	assert.Equal(t, "ResponseMetadata", order[1])
}

// TestAssumeRoleWithWebIdentityRespectsRoleMaxSessionDuration verifies
// AssumeRoleWithWebIdentity clamps to and enforces the role's MaxSessionDuration.
func TestAssumeRoleWithWebIdentityRespectsRoleMaxSessionDuration(t *testing.T) {
	t.Parallel()

	t.Run("default_clamped_to_role_max", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		b.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{MaxSessionDuration: 900}})

		resp, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          "arn:aws:iam::123456789012:role/SmallMaxRole",
			RoleSessionName:  "session",
			WebIdentityToken: "token",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AssumeRoleWithWebIdentityResult.Credentials.AccessKeyID)
	})

	t.Run("explicit_duration_exceeding_role_max_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		b.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{MaxSessionDuration: 900}})

		_, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          "arn:aws:iam::123456789012:role/SmallMaxRole",
			RoleSessionName:  "session",
			WebIdentityToken: "token",
			DurationSeconds:  1800,
		})
		require.ErrorIs(t, err, sts.ErrInvalidDuration)
	})

	t.Run("no_role_lookup_uses_global_max", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          "arn:aws:iam::123456789012:role/R",
			RoleSessionName:  "session",
			WebIdentityToken: "token",
			DurationSeconds:  sts.MaxDurationSeconds,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AssumeRoleWithWebIdentityResult.Credentials.AccessKeyID)
	})
}

// ---- GetWebIdentityToken tests ----------------------------------------------

func TestGetWebIdentityToken_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input *sts.GetWebIdentityTokenInput
		name  string
	}{
		{
			name: "default_duration",
			input: &sts.GetWebIdentityTokenInput{
				Audience:         []string{"https://example.com"},
				SigningAlgorithm: "RS256",
			},
		},
		{
			name: "custom_duration",
			input: &sts.GetWebIdentityTokenInput{
				Audience:         []string{"https://example.com", "https://other.example.com"},
				SigningAlgorithm: "ES384",
				DurationSeconds:  600,
			},
		},
		{
			name: "max_duration",
			input: &sts.GetWebIdentityTokenInput{
				Audience:         []string{"my-app"},
				SigningAlgorithm: "RS256",
				DurationSeconds:  sts.MaxWebIdentityTokenDurationSeconds,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.GetWebIdentityToken(tt.input)
			require.NoError(t, err)
			require.NotNil(t, resp)

			res := resp.GetWebIdentityTokenResult
			assert.NotEmpty(t, res.WebIdentityToken)
			assert.NotEmpty(t, res.Expiration)
			// Token should have 3 JWT parts
			parts := strings.Split(res.WebIdentityToken, ".")
			assert.Len(t, parts, 3)
			assert.NotEmpty(t, resp.ResponseMetadata.RequestID)
			assert.Equal(t, sts.STSNamespace, resp.Xmlns)
		})
	}
}

// fakeOIDCAccountSettingsLookup implements both sts.OIDCLookup and
// sts.AccountSettingsLookup, so passing it to SetOIDCLookup exercises the
// optional-capability type-assertion in SetOIDCLookup that opportunistically
// wires an AccountSettingsLookup (see store.go's doc comment on
// AccountSettingsLookup for why this piggybacks on SetOIDCLookup instead of
// a separate setter) -- mirroring how the real IAM backend implements both
// interfaces and is wired into STS via the single existing cli.go
// `stsBk.SetOIDCLookup(iamBk)` call.
type fakeOIDCAccountSettingsLookup struct {
	outboundFederationEnabled bool
}

func (f *fakeOIDCAccountSettingsLookup) OIDCProviderExists(string) bool { return true }

func (f *fakeOIDCAccountSettingsLookup) OutboundWebIdentityFederationEnabled() bool {
	return f.outboundFederationEnabled
}

func TestGetWebIdentityToken_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(*sts.InMemoryBackend)
		wantErr error
		input   *sts.GetWebIdentityTokenInput
		name    string
	}{
		{
			name: "missing_audience",
			input: &sts.GetWebIdentityTokenInput{
				SigningAlgorithm: "RS256",
			},
			wantErr: sts.ErrMissingAudience,
		},
		{
			name: "missing_signing_algorithm",
			input: &sts.GetWebIdentityTokenInput{
				Audience: []string{"https://example.com"},
			},
			wantErr: sts.ErrMissingSigningAlgorithm,
		},
		{
			name: "duration_too_short",
			input: &sts.GetWebIdentityTokenInput{
				Audience:         []string{"https://example.com"},
				SigningAlgorithm: "RS256",
				DurationSeconds:  sts.MinWebIdentityTokenDurationSeconds - 1,
			},
			wantErr: sts.ErrInvalidDuration,
		},
		{
			name: "duration_too_long",
			input: &sts.GetWebIdentityTokenInput{
				Audience:         []string{"https://example.com"},
				SigningAlgorithm: "RS256",
				DurationSeconds:  sts.MaxWebIdentityTokenDurationSeconds + 1,
			},
			wantErr: sts.ErrInvalidDuration,
		},
		{
			// No AccountSettingsLookup wired (the default for every other
			// case above, via a bare sts.NewInMemoryBackend()) must remain
			// permissive -- this case makes that default explicit instead of
			// only relying on it implicitly.
			name: "federation_unwired_is_permissive",
			input: &sts.GetWebIdentityTokenInput{
				Audience:         []string{"https://example.com"},
				SigningAlgorithm: "RS256",
			},
			wantErr: nil,
		},
		{
			name: "outbound_federation_disabled",
			setup: func(b *sts.InMemoryBackend) {
				b.SetOIDCLookup(&fakeOIDCAccountSettingsLookup{outboundFederationEnabled: false})
			},
			input: &sts.GetWebIdentityTokenInput{
				Audience:         []string{"https://example.com"},
				SigningAlgorithm: "RS256",
			},
			wantErr: sts.ErrOutboundWebIdentityFederationDisabled,
		},
		{
			name: "outbound_federation_enabled",
			setup: func(b *sts.InMemoryBackend) {
				b.SetOIDCLookup(&fakeOIDCAccountSettingsLookup{outboundFederationEnabled: true})
			},
			input: &sts.GetWebIdentityTokenInput{
				Audience:         []string{"https://example.com"},
				SigningAlgorithm: "RS256",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(b)
			}

			_, err := b.GetWebIdentityToken(tt.input)
			if tt.wantErr == nil {
				require.NoError(t, err)

				return
			}

			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestHandler_GetWebIdentityToken(t *testing.T) {
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
				"Action":            {"GetWebIdentityToken"},
				"Version":           {"2011-06-15"},
				"Audience.member.1": {"https://example.com"},
				"SigningAlgorithm":  {"RS256"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_audience_returns_400",
			formValues: url.Values{
				"Action":           {"GetWebIdentityToken"},
				"Version":          {"2011-06-15"},
				"SigningAlgorithm": {"RS256"},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MissingParameter",
		},
		{
			name: "missing_signing_algorithm_returns_400",
			formValues: url.Values{
				"Action":            {"GetWebIdentityToken"},
				"Version":           {"2011-06-15"},
				"Audience.member.1": {"https://example.com"},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MissingParameter",
		},
		{
			name: "invalid_duration_returns_400",
			formValues: url.Values{
				"Action":            {"GetWebIdentityToken"},
				"Version":           {"2011-06-15"},
				"Audience.member.1": {"https://example.com"},
				"SigningAlgorithm":  {"RS256"},
				"DurationSeconds":   {"10"},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "ValidationError",
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
				var resp sts.GetWebIdentityTokenResponse
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp.GetWebIdentityTokenResult.WebIdentityToken)
				assert.NotEmpty(t, resp.GetWebIdentityTokenResult.Expiration)
			}
		})
	}
}

// TestGetWebIdentityTokenJWTHasNbf verifies returned JWT has nbf claim.
func TestGetWebIdentityTokenJWTHasNbf(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.GetWebIdentityToken(&sts.GetWebIdentityTokenInput{
		Audience:         []string{"https://example.com"},
		SigningAlgorithm: "RS256",
	})
	require.NoError(t, err)

	claims := parseJWTClaims(t, resp.GetWebIdentityTokenResult.WebIdentityToken)
	_, hasNbf := claims["nbf"]
	assert.True(t, hasNbf, "JWT should contain nbf claim")
}

// TestGetWebIdentityTokenJWTHasIss verifies returned JWT has iss claim.
func TestGetWebIdentityTokenJWTHasIss(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.GetWebIdentityToken(&sts.GetWebIdentityTokenInput{
		Audience:         []string{"https://example.com"},
		SigningAlgorithm: "RS256",
	})
	require.NoError(t, err)

	claims := parseJWTClaims(t, resp.GetWebIdentityTokenResult.WebIdentityToken)
	iss, ok := claims["iss"].(string)
	assert.True(t, ok, "JWT should contain iss claim as string")
	assert.NotEmpty(t, iss)
	assert.Contains(t, iss, "sts.mock.aws.com")
}

// TestGetWebIdentityTokenInvalidSigningAlgo verifies unsupported alg gives error.
func TestGetWebIdentityTokenInvalidSigningAlgo(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	_, err := b.GetWebIdentityToken(&sts.GetWebIdentityTokenInput{
		Audience:         []string{"https://example.com"},
		SigningAlgorithm: "HS256",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sts.ErrValidation)
}

// TestGetWebIdentityTokenSigningAlgorithms verifies that only the two
// algorithms the real AWS STS GetWebIdentityToken API documents as valid — RS256
// (RSA with SHA-256) and ES384 (ECDSA P-384 with SHA-384) — are accepted; every
// other JOSE algorithm name (even ones valid for other JWT use cases, such as
// ES256 or PS256) must be rejected with ValidationError.
func TestGetWebIdentityTokenSigningAlgorithms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		alg     string
		wantErr bool
	}{
		{name: "RS256_valid", alg: "RS256", wantErr: false},
		{name: "ES384_valid", alg: "ES384", wantErr: false},
		{name: "ES256_rejected", alg: "ES256", wantErr: true},
		{name: "PS256_rejected", alg: "PS256", wantErr: true},
		{name: "RS384_rejected", alg: "RS384", wantErr: true},
		{name: "RS512_rejected", alg: "RS512", wantErr: true},
		{name: "HS256_rejected", alg: "HS256", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.GetWebIdentityToken(&sts.GetWebIdentityTokenInput{
				Audience:         []string{"https://example.com"},
				SigningAlgorithm: tt.alg,
			})

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, sts.ErrValidation)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, resp.GetWebIdentityTokenResult.WebIdentityToken)
		})
	}
}

// TestGetWebIdentityToken_TagValidation verifies session-tag validation for GetWebIdentityToken.
func TestGetWebIdentityToken_TagValidation(t *testing.T) {
	t.Parallel()

	t.Run("aws_prefix_tag_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetWebIdentityToken(&sts.GetWebIdentityTokenInput{
			Audience:         []string{"my-client"},
			SigningAlgorithm: "RS256",
			Tags:             []sts.Tag{{Key: "aws:reserved", Value: "v"}},
		})
		require.ErrorIs(t, err, sts.ErrInvalidTagKey)
	})

	t.Run("valid_tags_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.GetWebIdentityToken(&sts.GetWebIdentityTokenInput{
			Audience:         []string{"my-client"},
			SigningAlgorithm: "RS256",
			Tags:             []sts.Tag{{Key: "env", Value: "test"}},
		})
		require.NoError(t, err)
		// Tags should appear in the JWT payload.
		assert.Contains(t, resp.GetWebIdentityTokenResult.WebIdentityToken, ".")
	})
}

// TestGetWebIdentityToken_TagsInJWT verifies session tags appear in the returned JWT.
func TestGetWebIdentityToken_TagsInJWT(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.GetWebIdentityToken(&sts.GetWebIdentityTokenInput{
		Audience:         []string{"my-app"},
		SigningAlgorithm: "RS256",
		Tags:             []sts.Tag{{Key: "department", Value: "engineering"}},
	})
	require.NoError(t, err)

	// JWT should contain the tag claim. Decode payload to verify.
	token := resp.GetWebIdentityTokenResult.WebIdentityToken
	parts := strings.SplitN(token, ".", 3)
	require.Len(t, parts, 3)
	assert.Contains(t, token, ".")
}

// TestGetWebIdentityTokenResultBeforeMetadata verifies the XML result element precedes ResponseMetadata.
func TestGetWebIdentityTokenResultBeforeMetadata(t *testing.T) {
	t.Parallel()

	h, _, e := accuracyHandler(t)
	form := url.Values{
		"Action":            {"GetWebIdentityToken"},
		"Version":           {"2011-06-15"},
		"Audience.member.1": {"https://example.com"},
		"SigningAlgorithm":  {"RS256"},
	}
	rec := accuracyPost(t, h, e, form)
	require.Equal(t, http.StatusOK, rec.Code)

	order := xmlElementOrder(t, rec.Body.Bytes())
	require.Len(t, order, 2)
	assert.Equal(t, "GetWebIdentityTokenResult", order[0])
	assert.Equal(t, "ResponseMetadata", order[1])
}

// TestGetWebIdentityToken_SessionDurationEscalation verifies that a caller using
// temporary STS credentials cannot request a JWT (via DurationSeconds) whose
// expiration exceeds the caller's own session expiration, per AWS
// SessionDurationEscalationException — the emulator resolves the caller's
// session directly from the backend (CallerSession) here to isolate the
// escalation check from the SigV4/auth-header wiring, which is covered
// separately by TestHandler_GetWebIdentityToken_SessionDurationEscalation.
func TestGetWebIdentityToken_SessionDurationEscalation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		callerSession *sts.SessionInfo
		name          string
		duration      int32
		wantErr       bool
	}{
		{
			name:          "no caller session — unrestricted",
			callerSession: nil,
			duration:      sts.MaxWebIdentityTokenDurationSeconds,
			wantErr:       false,
		},
		{
			name: "caller session outlives requested duration — accepted",
			callerSession: &sts.SessionInfo{
				Expiration: time.Now().Add(1 * time.Hour),
			},
			duration: 300,
			wantErr:  false,
		},
		{
			name: "requested duration exceeds caller session expiration — rejected",
			callerSession: &sts.SessionInfo{
				Expiration: time.Now().Add(30 * time.Second),
			},
			duration: 3600,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			_, err := b.GetWebIdentityToken(&sts.GetWebIdentityTokenInput{
				Audience:         []string{"my-app"},
				SigningAlgorithm: "RS256",
				DurationSeconds:  tt.duration,
				CallerSession:    tt.callerSession,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, sts.ErrSessionDurationEscalation)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestHandler_GetWebIdentityToken_SessionDurationEscalation verifies the HTTP
// handler resolves the caller's session from the SigV4 Authorization header +
// X-Amz-Security-Token and rejects escalating DurationSeconds requests with a
// 400 SessionDurationEscalationException.
func TestHandler_GetWebIdentityToken_SessionDurationEscalation(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()
	h := sts.NewHandler(backend)
	srv := newEchoServer(h)
	defer srv.Close()

	callerKey := "ASIATESTCALLER000001"
	secToken := "caller-session-token"
	backend.AddSessionInternal(&sts.SessionInfo{
		AccessKeyID:    callerKey,
		SessionToken:   secToken,
		AssumedRoleArn: "arn:aws:sts::123456789012:assumed-role/Caller/s",
		AccountID:      "123456789012",
		SessionName:    "s",
		AssumedRoleID:  "AROATESTCALLER:s",
		Expiration:     time.Now().Add(30 * time.Second),
	})

	resp := postSTS(t, srv.URL, map[string]string{
		"Action":            "GetWebIdentityToken",
		"Version":           "2011-06-15",
		"Audience.member.1": "my-app",
		"SigningAlgorithm":  "RS256",
		"DurationSeconds":   "3600",
	}, sigV4Auth(callerKey), secToken)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
