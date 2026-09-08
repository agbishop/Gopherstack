package sts_test

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sts"
)

func TestGetFederationToken_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    *sts.GetFederationTokenInput
		wantName string
	}{
		{
			name:     "default_duration",
			input:    &sts.GetFederationTokenInput{Name: "alice"},
			wantName: "alice",
		},
		{
			name:     "custom_duration",
			input:    &sts.GetFederationTokenInput{Name: "bob", DurationSeconds: 3600},
			wantName: "bob",
		},
		{
			name: "max_duration",
			input: &sts.GetFederationTokenInput{
				Name:            "charlie",
				DurationSeconds: sts.MaxFederationTokenDurationSeconds,
			},
			wantName: "charlie",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.GetFederationToken(tt.input)
			require.NoError(t, err)
			require.NotNil(t, resp)

			creds := resp.GetFederationTokenResult.Credentials
			assert.True(t, strings.HasPrefix(creds.AccessKeyID, "ASIA"))
			assert.NotEmpty(t, creds.SecretAccessKey)
			assert.NotEmpty(t, creds.SessionToken)
			assert.NotEmpty(t, creds.Expiration)

			fu := resp.GetFederationTokenResult.FederatedUser
			assert.Contains(t, fu.Arn, "federated-user/"+tt.wantName)
			assert.Contains(t, fu.FederatedUserID, tt.wantName)
			assert.NotEmpty(t, resp.ResponseMetadata.RequestID)
		})
	}
}

func TestGetFederationToken_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		input   *sts.GetFederationTokenInput
		name    string
	}{
		{
			name:    "missing_name",
			input:   &sts.GetFederationTokenInput{},
			wantErr: sts.ErrMissingFederationTokenName,
		},
		{
			name:    "duration_too_short",
			input:   &sts.GetFederationTokenInput{Name: "user", DurationSeconds: 100},
			wantErr: sts.ErrInvalidDuration,
		},
		{
			name: "duration_too_long",
			input: &sts.GetFederationTokenInput{
				Name:            "user",
				DurationSeconds: sts.MaxFederationTokenDurationSeconds + 1,
			},
			wantErr: sts.ErrInvalidDuration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			_, err := b.GetFederationToken(tt.input)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestGetFederationToken_SessionTrackedForCallerIdentity(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.GetFederationToken(&sts.GetFederationTokenInput{Name: "feduser"})
	require.NoError(t, err)

	creds := resp.GetFederationTokenResult.Credentials

	ciResp, err := b.GetCallerIdentity(creds.AccessKeyID, creds.SessionToken)
	require.NoError(t, err)
	assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "federated-user/feduser")
	assert.Contains(t, ciResp.GetCallerIdentityResult.UserID, "feduser")
	assert.Equal(t, 1, b.SessionCount())
}

func TestHandler_GetFederationToken(t *testing.T) {
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
				"Action":  {"GetFederationToken"},
				"Version": {"2011-06-15"},
				"Name":    {"testuser"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_name_returns_400",
			formValues: url.Values{
				"Action":  {"GetFederationToken"},
				"Version": {"2011-06-15"},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MissingParameter",
		},
		{
			name: "invalid_duration_returns_400",
			formValues: url.Values{
				"Action":          {"GetFederationToken"},
				"Version":         {"2011-06-15"},
				"Name":            {"testuser"},
				"DurationSeconds": {"100"},
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
				var resp sts.GetFederationTokenResponse
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp.GetFederationTokenResult.Credentials.AccessKeyID)
				assert.Contains(t, resp.GetFederationTokenResult.FederatedUser.Arn, "federated-user/testuser")
			}
		})
	}
}

// TestFederationTokenNameTooShort verifies a 1-char name gives error.
func TestFederationTokenNameTooShort(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
		Name: "x",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sts.ErrInvalidFederationName)
}

// TestFederationTokenNameTooLong verifies a 33-char name gives error.
func TestFederationTokenNameTooLong(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
		Name: strings.Repeat("a", 33),
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sts.ErrInvalidFederationName)
}

// TestFederationTokenNameValid verifies a 2-char name succeeds.
func TestFederationTokenNameValid(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.GetFederationToken(&sts.GetFederationTokenInput{
		Name: "ab",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetFederationTokenResult.Credentials.AccessKeyID)
}

// TestGetFederationToken_NameCharset verifies federation-token name charset validation.
func TestGetFederationToken_NameCharset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fedName string
		wantErr bool
	}{
		{name: "valid alphanumeric", fedName: "alice", wantErr: false},
		{name: "valid with allowed chars", fedName: "alice+=,.@-", wantErr: false},
		{name: "invalid colon", fedName: "alice:bob", wantErr: true},
		{name: "invalid space", fedName: "alice bob", wantErr: true},
		{name: "invalid slash", fedName: "alice/bob", wantErr: true},
		{name: "too short", fedName: "a", wantErr: true},
		{name: "too long 33 chars", fedName: strings.Repeat("x", 33), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
				Name: tc.fedName,
			})

			if tc.wantErr {
				require.ErrorIs(t, err, sts.ErrInvalidFederationName)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestGetFederationToken_TagConstraints verifies session-tag constraints for GetFederationToken.
func TestGetFederationToken_TagConstraints(t *testing.T) {
	t.Parallel()

	t.Run("aws_prefix_tag_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
			Name: "alice",
			Tags: []sts.Tag{{Key: "aws:reserved", Value: "val"}},
		})
		require.ErrorIs(t, err, sts.ErrInvalidTagKey)
	})

	t.Run("duplicate_tag_key_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
			Name: "alice",
			Tags: []sts.Tag{{Key: "MyKey", Value: "v1"}, {Key: "mykey", Value: "v2"}},
		})
		require.ErrorIs(t, err, sts.ErrInvalidTagKey)
	})

	t.Run("value_over_256_chars_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
			Name: "alice",
			Tags: []sts.Tag{{Key: "k", Value: strings.Repeat("v", 257)}},
		})
		require.ErrorIs(t, err, sts.ErrInvalidTagValue)
	})
}

// TestGetFederationToken_PolicyArnsValidation verifies managed-policy-ARN validation for GetFederationToken.
func TestGetFederationToken_PolicyArnsValidation(t *testing.T) {
	t.Parallel()

	t.Run("too_many_policy_arns_rejected", func(t *testing.T) {
		t.Parallel()

		arns := make([]string, 11)
		for i := range arns {
			arns[i] = "arn:aws:iam::aws:policy/P"
		}

		b := sts.NewInMemoryBackend()
		_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
			Name:       "alice",
			PolicyArns: arns,
		})
		require.ErrorIs(t, err, sts.ErrTooManyPolicyArns)
	})

	t.Run("malformed_policy_arn_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
			Name:       "alice",
			PolicyArns: []string{"not-an-arn"},
		})
		require.ErrorIs(t, err, sts.ErrInvalidPolicyArn)
	})
}

// TestGetFederationToken_PolicyValidation verifies inline-policy validation for GetFederationToken.
func TestGetFederationToken_PolicyValidation(t *testing.T) {
	t.Parallel()

	t.Run("malformed_json_policy_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
			Name:   "alice",
			Policy: "not valid json",
		})
		require.ErrorIs(t, err, sts.ErrMalformedPolicyDocument)
	})

	t.Run("policy_too_large_rejected", func(t *testing.T) {
		t.Parallel()

		bigPolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"` +
			strings.Repeat("arn:aws:s3:::bucket/prefix/", 90) + `*"}]}`
		b := sts.NewInMemoryBackend()
		_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
			Name:   "alice",
			Policy: bigPolicy,
		})
		require.ErrorIs(t, err, sts.ErrPackedPolicyTooLarge)
	})

	t.Run("valid_policy_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.GetFederationToken(&sts.GetFederationTokenInput{
			Name:   "alice",
			Policy: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`,
		})
		require.NoError(t, err)
		assert.Positive(t, resp.GetFederationTokenResult.PackedPolicySize)
	})
}

// TestGetFederationToken_PolicyMissingStatement verifies a policy without a
// Statement field is rejected.
func TestGetFederationToken_PolicyMissingStatement(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	_, err := b.GetFederationToken(&sts.GetFederationTokenInput{
		Name:   "alice",
		Policy: `{"Version":"2012-10-17"}`,
	})
	require.ErrorIs(t, err, sts.ErrMalformedPolicyDocument)
}

// TestGetFederationToken_PackedPolicySizeWithArns verifies PackedPolicySize
// includes PolicyArns for GetFederationToken.
func TestGetFederationToken_PackedPolicySizeWithArns(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.GetFederationToken(&sts.GetFederationTokenInput{
		Name:       "alice",
		PolicyArns: []string{"arn:aws:iam::aws:policy/ReadOnlyAccess"},
	})
	require.NoError(t, err)
	assert.Positive(t, resp.GetFederationTokenResult.PackedPolicySize)
}
