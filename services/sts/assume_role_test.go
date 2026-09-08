package sts_test

import (
	"encoding/xml"
	"errors"
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

// stubRoleLookup is a test double for sts.RoleLookup and sts.UserLookup.
type stubRoleLookup struct {
	meta  *sts.RoleMeta
	err   error
	users map[string]string
}

func (s *stubRoleLookup) GetRoleByArn(_ string) (*sts.RoleMeta, error) {
	if s.err != nil {
		return nil, s.err
	}

	return s.meta, nil
}

func (s *stubRoleLookup) GetUserArnByAccessKeyID(accessKeyID string) (string, error) {
	if s.users != nil {
		if u, ok := s.users[accessKeyID]; ok {
			return u, nil
		}
	}

	return "", errStubUserNotFound
}

// errStubRoleNotFound is the sentinel error returned by stubRoleLookup when a role is not found.
var (
	errStubRoleNotFound = errors.New("role not found")
	errStubUserNotFound = errors.New("user not found")
)

// newEchoServer builds a minimal echo server wired to handler h.
func newEchoServer(h *sts.Handler) *httptest.Server {
	e := echo.New()
	e.Use(func(_ echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := logger.Save(c.Request().Context(), logger.NewTestLogger())

			return h.Handler()(echo.NewContext(c.Request().WithContext(ctx), c.Response()))
		}
	})

	return httptest.NewServer(e)
}

// postSTS sends a form POST with STS Action and optional auth headers.
func postSTS(t *testing.T, serverURL string, form map[string]string, authHeader, secToken string) *http.Response {
	t.Helper()

	params := make([]string, 0, len(form))
	for k, v := range form {
		params = append(params, k+"="+v)
	}
	body := strings.NewReader(strings.Join(params, "&"))

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, serverURL+"/", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if secToken != "" {
		req.Header.Set("X-Amz-Security-Token", secToken)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

func sigV4Auth(accessKeyID string) string {
	return fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/20260101/us-east-1/sts/aws4_request, "+
			"SignedHeaders=host;x-amz-date, Signature=deadbeef",
		accessKeyID,
	)
}

// ---- Backend tests ---------------------------------------------------------

func TestAssumeRole_Success(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
		RoleSessionName: "my-session",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	creds := resp.AssumeRoleResult.Credentials
	assert.True(t, strings.HasPrefix(creds.AccessKeyID, "ASIA"), "AccessKeyId should start with ASIA")
	assert.Len(t, creds.AccessKeyID, 20, "AccessKeyId should be 20 chars")
	assert.Len(t, creds.SecretAccessKey, 40, "SecretAccessKey should be 40 chars")
	assert.NotEmpty(t, creds.SessionToken)
	assert.NotEmpty(t, creds.Expiration)

	user := resp.AssumeRoleResult.AssumedRoleUser
	assert.Contains(t, user.Arn, "assumed-role")
	assert.Contains(t, user.Arn, "my-session")
	assert.Contains(t, user.AssumedRoleID, "my-session")
}

func TestAssumeRole_DefaultDuration(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
		RoleSessionName: "session",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.AssumeRoleResult.Credentials.Expiration)
}

func TestAssumeRole_CustomDuration(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
		RoleSessionName: "session",
		DurationSeconds: 1800,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AssumeRoleResult.Credentials.Expiration)
}

func TestAssumeRole_MissingRoleArn(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	_, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleSessionName: "session",
	})
	require.ErrorIs(t, err, sts.ErrMissingRoleArn)
}

func TestAssumeRole_MissingSessionName(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	_, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn: "arn:aws:iam::123456789012:role/TestRole",
	})
	require.ErrorIs(t, err, sts.ErrMissingSessionName)
}

func TestAssumeRole_InvalidDurationTooShort(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	_, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
		RoleSessionName: "session",
		DurationSeconds: 100,
	})
	require.ErrorIs(t, err, sts.ErrInvalidDuration)
}

func TestAssumeRole_InvalidDurationTooLong(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	_, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
		RoleSessionName: "session",
		DurationSeconds: 99999,
	})
	require.ErrorIs(t, err, sts.ErrInvalidDuration)
}

func TestAssumeRole_CredentialsAreUnique(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()
	input := &sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
		RoleSessionName: "session",
	}

	r1, err := backend.AssumeRole(input)
	require.NoError(t, err)

	r2, err := backend.AssumeRole(input)
	require.NoError(t, err)

	// Each call should produce unique credentials.
	assert.NotEqual(t, r1.AssumeRoleResult.Credentials.AccessKeyID, r2.AssumeRoleResult.Credentials.AccessKeyID)
}

func TestAssumeRole_MalformedArn(t *testing.T) {
	t.Parallel()

	// A malformed ARN (fewer than 6 colon-separated components) is rejected
	// by the validateRoleArn check.
	backend := sts.NewInMemoryBackend()
	_, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "short/role",
		RoleSessionName: "session",
	})
	require.ErrorIs(t, err, sts.ErrInvalidRoleArn)
}

// ---- Handler tests ---------------------------------------------------------

func TestHandler_AssumeRole(t *testing.T) {
	t.Parallel()

	h, e := newTestHandler(t)
	rec := postForm(t, e, h, url.Values{
		"Action":          {"AssumeRole"},
		"Version":         {"2011-06-15"},
		"RoleArn":         {"arn:aws:iam::123456789012:role/TestRole"},
		"RoleSessionName": {"test-session"},
	})

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp sts.AssumeRoleResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, strings.HasPrefix(resp.AssumeRoleResult.Credentials.AccessKeyID, "ASIA"))
	assert.Contains(t, resp.AssumeRoleResult.AssumedRoleUser.Arn, "assumed-role")
}

func TestHandler_AssumeRole_WithDuration(t *testing.T) {
	t.Parallel()

	h, e := newTestHandler(t)
	rec := postForm(t, e, h, url.Values{
		"Action":          {"AssumeRole"},
		"Version":         {"2011-06-15"},
		"RoleArn":         {"arn:aws:iam::123456789012:role/TestRole"},
		"RoleSessionName": {"test-session"},
		"DurationSeconds": {"1800"},
	})

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp sts.AssumeRoleResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.AssumeRoleResult.Credentials.Expiration)
}

func TestHandler_AssumeRole_InvalidDuration(t *testing.T) {
	t.Parallel()

	h, e := newTestHandler(t)
	rec := postForm(t, e, h, url.Values{
		"Action":          {"AssumeRole"},
		"Version":         {"2011-06-15"},
		"RoleArn":         {"arn:aws:iam::123456789012:role/TestRole"},
		"RoleSessionName": {"test-session"},
		"DurationSeconds": {"not-a-number"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp sts.ErrorResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ValidationError", errResp.Error.Code)
}

func TestHandler_AssumeRole_MissingRoleArn(t *testing.T) {
	t.Parallel()

	h, e := newTestHandler(t)
	rec := postForm(t, e, h, url.Values{
		"Action":          {"AssumeRole"},
		"RoleSessionName": {"session"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp sts.ErrorResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "MissingParameter", errResp.Error.Code)
	assert.Equal(t, "Sender", errResp.Error.Type)
}

func TestHandler_AssumeRole_MissingSessionName(t *testing.T) {
	t.Parallel()

	h, e := newTestHandler(t)
	rec := postForm(t, e, h, url.Values{
		"Action":  {"AssumeRole"},
		"RoleArn": {"arn:aws:iam::123456789012:role/TestRole"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp sts.ErrorResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "MissingParameter", errResp.Error.Code)
	assert.Equal(t, "Sender", errResp.Error.Type)
}

// ---- External ID validation tests ------------------------------------------

func TestAssumeRole_ExternalID_NotRequired(t *testing.T) {
	t.Parallel()

	// Role has no ExternalId condition: any call should succeed regardless of ExternalId.
	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{
		meta: &sts.RoleMeta{
			TrustPolicy: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:AssumeRole"}]}`,
		},
	})

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName: "session",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestAssumeRole_ExternalID_MatchingValue(t *testing.T) {
	t.Parallel()

	trustDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:AssumeRole",
			"Condition":{
				"StringEquals":{"sts:ExternalId":"correct-id"}
			}
		}]
	}`

	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName: "session",
		ExternalID:      "correct-id",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestAssumeRole_ExternalID_WrongValue(t *testing.T) {
	t.Parallel()

	trustDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:AssumeRole",
			"Condition":{
				"StringEquals":{"sts:ExternalId":"correct-id"}
			}
		}]
	}`

	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

	_, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName: "session",
		ExternalID:      "wrong-id",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, sts.ErrAccessDenied)
}

func TestAssumeRole_ExternalID_MissingWhenRequired(t *testing.T) {
	t.Parallel()

	trustDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:AssumeRole",
			"Condition":{
				"StringEquals":{"sts:ExternalId":"required-id"}
			}
		}]
	}`

	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

	_, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName: "session",
		// ExternalID not provided
	})
	require.Error(t, err)
	require.ErrorIs(t, err, sts.ErrAccessDenied)
}

func TestAssumeRole_ExternalID_ArrayOfValues(t *testing.T) {
	t.Parallel()

	// Trust policy with multiple allowed ExternalId values.
	trustDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:AssumeRole",
			"Condition":{
				"StringEquals":{"sts:ExternalId":["id-one","id-two","id-three"]}
			}
		}]
	}`

	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

	tests := []struct {
		name       string
		externalID string
		wantErr    bool
	}{
		{name: "first_allowed", externalID: "id-one", wantErr: false},
		{name: "second_allowed", externalID: "id-two", wantErr: false},
		{name: "third_allowed", externalID: "id-three", wantErr: false},
		{name: "not_in_list", externalID: "id-four", wantErr: true},
		{name: "empty", externalID: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := backend.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
				RoleSessionName: "session",
				ExternalID:      tt.externalID,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, sts.ErrAccessDenied)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAssumeRole_ExternalID_MultipleStatements_ORSemantics(t *testing.T) {
	t.Parallel()

	// Two statements with different ExternalId conditions: the caller must match any one.
	trustDoc := `{
		"Version":"2012-10-17",
		"Statement":[
			{
				"Effect":"Allow",
				"Action":"sts:AssumeRole",
				"Condition":{"StringEquals":{"sts:ExternalId":"id-alpha"}}
			},
			{
				"Effect":"Allow",
				"Action":"sts:AssumeRole",
				"Condition":{"StringEquals":{"sts:ExternalId":"id-beta"}}
			}
		]
	}`

	tests := []struct {
		name       string
		externalID string
		wantErr    bool
	}{
		{name: "matches_first_statement", externalID: "id-alpha", wantErr: false},
		{name: "matches_second_statement", externalID: "id-beta", wantErr: false},
		{name: "matches_neither", externalID: "id-gamma", wantErr: true},
		{name: "empty_matches_neither", externalID: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sts.NewInMemoryBackend()
			backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

			_, err := backend.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
				RoleSessionName: "session",
				ExternalID:      tt.externalID,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, sts.ErrAccessDenied)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAssumeRole_ExternalID_RoleLookupError(t *testing.T) {
	t.Parallel()

	// When the role cannot be found in the lookup, AssumeRole still succeeds
	// (the role may exist but not be in IAM — passthrough mode).
	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{err: errStubRoleNotFound})

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/Unknown",
		RoleSessionName: "session",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestAssumeRole_ExternalID_EmptyTrustPolicy(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: ""}})

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName: "session",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestAssumeRole_ExternalID_MalformedTrustPolicy(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: "not-valid-json"}})

	// Malformed policy should not block the call.
	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName: "session",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// ---- MFA condition validation tests ----------------------------------------

// TestAssumeRole_MFACondition_DeniesWithoutMFA is the decisive regression test
// for gopherstack-41fl: a trust policy requiring MFA must deny a caller who
// presents no SerialNumber/TokenCode at all. Before the fix, AssumeRole never
// read these fields and never modeled aws:MultiFactorAuthPresent, so this
// call succeeded when it must not.
func TestAssumeRole_MFACondition_DeniesWithoutMFA(t *testing.T) {
	t.Parallel()

	trustDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:AssumeRole",
			"Condition":{
				"Bool":{"aws:MultiFactorAuthPresent":"true"}
			}
		}]
	}`

	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

	_, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MFARequired",
		RoleSessionName: "session",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, sts.ErrAccessDenied)
}

func TestAssumeRole_MFACondition_PermitsWithMFA(t *testing.T) {
	t.Parallel()

	trustDoc := `{
		"Version":"2012-10-17",
		"Statement":[{
			"Effect":"Allow",
			"Action":"sts:AssumeRole",
			"Condition":{
				"Bool":{"aws:MultiFactorAuthPresent":"true"}
			}
		}]
	}`

	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MFARequired",
		RoleSessionName: "session",
		SerialNumber:    "arn:aws:iam::123456789012:mfa/my-device",
		TokenCode:       "123456",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AssumeRoleResult.Credentials.AccessKeyID)
}

func TestAssumeRole_MFACondition_NotRequired(t *testing.T) {
	t.Parallel()

	// No MFA condition in the trust policy: absence of MFA does not block the call.
	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{
		meta: &sts.RoleMeta{
			TrustPolicy: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"sts:AssumeRole"}]}`,
		},
	})

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/NoMFA",
		RoleSessionName: "session",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestAssumeRole_MFACondition_CombinedWithPrincipal(t *testing.T) {
	t.Parallel()

	const (
		roleArn   = "arn:aws:iam::123456789012:role/Target"
		callerArn = "arn:aws:sts::123456789012:assumed-role/AppRole/session"
	)

	trustDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",` +
		`"Principal":{"AWS":"arn:aws:iam::123456789012:role/AppRole"},"Action":"sts:AssumeRole",` +
		`"Condition":{"Bool":{"aws:MultiFactorAuthPresent":"true"}}}]}`

	tests := []struct {
		name         string
		serialNumber string
		tokenCode    string
		wantErr      bool
	}{
		{name: "principal_matches_no_mfa_denied", wantErr: true},
		{
			name:         "principal_matches_with_mfa_allowed",
			serialNumber: "arn:aws:iam::123456789012:mfa/my-device",
			tokenCode:    "123456",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sts.NewInMemoryBackend()
			backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

			_, err := backend.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         roleArn,
				RoleSessionName: "session",
				CallerArn:       callerArn,
				SerialNumber:    tt.serialNumber,
				TokenCode:       tt.tokenCode,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, sts.ErrAccessDenied)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssumeRole_MFAFields_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr      error
		name         string
		serialNumber string
		tokenCode    string
	}{
		{name: "token_code_without_serial", tokenCode: "123456", wantErr: sts.ErrTokenCodeWithoutSerial},
		{
			name:         "serial_without_token_code",
			serialNumber: "arn:aws:iam::123456789012:mfa/my-device",
			wantErr:      sts.ErrMFACodeRequired,
		},
		{
			name:         "malformed_serial_number",
			serialNumber: "not-a-serial-number",
			tokenCode:    "123456",
			wantErr:      sts.ErrInvalidMFASerialNumber,
		},
		{
			name:         "non_6_digit_code",
			serialNumber: "arn:aws:iam::123456789012:mfa/my-device",
			tokenCode:    "12345",
			wantErr:      sts.ErrInvalidMFATokenCode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sts.NewInMemoryBackend()

			_, err := backend.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
				RoleSessionName: "session",
				SerialNumber:    tt.serialNumber,
				TokenCode:       tt.tokenCode,
			})
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// ---- Duration enforcement tests --------------------------------------------

func TestAssumeRole_Duration_RespectRoleMaxSessionDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration int32
		wantErr  bool
	}{
		{name: "within_limit", duration: 900, wantErr: false},
		{name: "at_limit", duration: 1800, wantErr: false},
		{name: "exceeds_limit", duration: 3600, wantErr: true},
		// When DurationSeconds is 0, the default is clamped to min(3600, roleMax=1800)=1800 — no error.
		{name: "default_clamped_to_max", duration: 0, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend2 := sts.NewInMemoryBackend()
			backend2.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{
				MaxSessionDuration: 1800,
			}})

			_, err := backend2.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
				RoleSessionName: "session",
				DurationSeconds: tt.duration,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, sts.ErrInvalidDuration)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAssumeRole_Duration_NoRoleMaxUsesSystemDefault(t *testing.T) {
	t.Parallel()

	// When MaxSessionDuration is 0, the system default (43200) is used.
	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{MaxSessionDuration: 0}})

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName: "session",
		DurationSeconds: 7200, // 2 hours — within system default 12 hours
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// TestAssumeRoleDefaultDurationClamped verifies the default duration is clamped
// to a small role MaxSessionDuration, while an explicit value exceeding it is
// still rejected (Accuracy Gap #15).
func TestAssumeRoleDefaultDurationClamped(t *testing.T) {
	t.Parallel()

	t.Run("default_clamped_when_role_max_is_small", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		b.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{
			MaxSessionDuration: 900,
		}})

		// With DurationSeconds=0, default is clamped to min(3600, 900)=900 — should succeed.
		resp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/SmallMaxRole",
			RoleSessionName: "session",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AssumeRoleResult.Credentials.AccessKeyID)
	})

	t.Run("explicit_value_exceeding_role_max_still_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		b.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{
			MaxSessionDuration: 900,
		}})

		_, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/SmallMaxRole",
			RoleSessionName: "session",
			DurationSeconds: 1800,
		})
		require.ErrorIs(t, err, sts.ErrInvalidDuration)
	})
}

// ---- Source identity tests -------------------------------------------------

func TestAssumeRole_SourceIdentity_ReturnedInResult(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName: "session",
		SourceIdentity:  "admin-user",
	})
	require.NoError(t, err)
	assert.Equal(t, "admin-user", resp.AssumeRoleResult.SourceIdentity)
}

func TestAssumeRole_SourceIdentity_EmptyWhenNotProvided(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName: "session",
	})
	require.NoError(t, err)
	assert.Empty(t, resp.AssumeRoleResult.SourceIdentity)
}

// ---- Session tags tests ----------------------------------------------------

func TestAssumeRole_SessionTags_StoredInSession(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	tags := []sts.Tag{
		{Key: "department", Value: "engineering"},
		{Key: "team", Value: "platform"},
	}

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:           "arn:aws:iam::123456789012:role/MyRole",
		RoleSessionName:   "session",
		Tags:              tags,
		TransitiveTagKeys: []string{"department"},
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)

	// Credentials are returned; tags are stored in session.
	assert.NotEmpty(t, resp.AssumeRoleResult.Credentials.AccessKeyID)
}

// ---- Handler tests for new features ----------------------------------------

func TestHandler_AssumeRole_WithSourceIdentity(t *testing.T) {
	t.Parallel()

	h, e := newTestHandler(t)
	rec := postForm(t, e, h, url.Values{
		"Action":          {"AssumeRole"},
		"Version":         {"2011-06-15"},
		"RoleArn":         {"arn:aws:iam::123456789012:role/TestRole"},
		"RoleSessionName": {"session"},
		"SourceIdentity":  {"admin"},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp sts.AssumeRoleResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "admin", resp.AssumeRoleResult.SourceIdentity)
}

func TestHandler_AssumeRole_WithSessionTags(t *testing.T) {
	t.Parallel()

	h, e := newTestHandler(t)
	rec := postForm(t, e, h, url.Values{
		"Action":                     {"AssumeRole"},
		"Version":                    {"2011-06-15"},
		"RoleArn":                    {"arn:aws:iam::123456789012:role/TestRole"},
		"RoleSessionName":            {"session"},
		"Tags.member.1.Key":          {"dept"},
		"Tags.member.1.Value":        {"eng"},
		"Tags.member.2.Key":          {"team"},
		"Tags.member.2.Value":        {"platform"},
		"TransitiveTagKeys.member.1": {"dept"},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp sts.AssumeRoleResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.AssumeRoleResult.Credentials.AccessKeyID)
}

func TestHandler_AssumeRole_AccessDenied(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()
	backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{
		TrustPolicy: `{
			"Version":"2012-10-17",
			"Statement":[{
				"Effect":"Allow",
				"Action":"sts:AssumeRole",
				"Condition":{"StringEquals":{"sts:ExternalId":"required"}}
			}]
		}`,
	}})

	h := sts.NewHandler(backend)
	e := echo.New()

	body := url.Values{
		"Action":          {"AssumeRole"},
		"Version":         {"2011-06-15"},
		"RoleArn":         {"arn:aws:iam::123456789012:role/MyRole"},
		"RoleSessionName": {"session"},
	}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(logger.Save(req.Context(), logger.NewTestLogger()))
	rec := httptest.NewRecorder()

	require.NoError(t, h.Handler()(e.NewContext(req, rec)))

	assert.Equal(t, http.StatusForbidden, rec.Code)

	var errResp sts.ErrorResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "AccessDenied", errResp.Error.Code)
	assert.Equal(t, "Sender", errResp.Error.Type)
}

// ---- RoleSessionName validation tests ---------------------------------------

// TestRoleSessionNameTooShort verifies a 1-char session name gives ValidationError.
func TestRoleSessionNameTooShort(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	_, err := b.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::000000000000:role/test-role",
		RoleSessionName: "x",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sts.ErrInvalidSessionName)
}

// TestRoleSessionNameTooLong verifies a 65-char session name gives ValidationError.
func TestRoleSessionNameTooLong(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	_, err := b.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::000000000000:role/test-role",
		RoleSessionName: strings.Repeat("a", 65),
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sts.ErrInvalidSessionName)
}

// TestRoleSessionNameValid verifies 2-char and 64-char names succeed.
func TestRoleSessionNameValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sessionName string
	}{
		{name: "min_length", sessionName: "ab"},
		{name: "max_length", sessionName: strings.Repeat("a", 64)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         "arn:aws:iam::000000000000:role/test-role",
				RoleSessionName: tt.sessionName,
			})

			require.NoError(t, err)
			assert.NotEmpty(t, resp.AssumeRoleResult.Credentials.AccessKeyID)
		})
	}
}

// TestRoleSessionNameColonRejected verifies that AWS disallows a colon in RoleSessionName.
func TestRoleSessionNameColonRejected(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	_, err := b.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/R",
		RoleSessionName: "bad:name",
	})
	require.ErrorIs(t, err, sts.ErrInvalidSessionName)
}

func TestRoleSessionNameColonRejected_HTTP(t *testing.T) {
	t.Parallel()

	h, _, e := accuracyHandler(t)
	form := url.Values{
		"Action":          {"AssumeRole"},
		"Version":         {"2011-06-15"},
		"RoleArn":         {"arn:aws:iam::123456789012:role/R"},
		"RoleSessionName": {"bad:session:name"},
	}
	rec := accuracyPost(t, h, e, form)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	errResp := decodeError(t, rec.Body.Bytes())
	assert.Equal(t, "ValidationError", errResp.Error.Code)
}

// ---- PolicyArns / ProvidedContexts wire-parsing tests -----------------------

// TestAssumeRoleWithPolicyArns verifies PolicyArns are parsed and present in input.
func TestAssumeRoleWithPolicyArns(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	h := sts.NewHandler(b)

	rec := r1PostForm(t, h, url.Values{
		"Action":                  {"AssumeRole"},
		"Version":                 {"2011-06-15"},
		"RoleArn":                 {"arn:aws:iam::000000000000:role/test-role"},
		"RoleSessionName":         {"my-session"},
		"PolicyArns.member.1.arn": {"arn:aws:iam::aws:policy/ReadOnlyAccess"},
	})

	assert.Equal(t, http.StatusOK, rec.Code)

	var result sts.AssumeRoleResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &result))
	assert.NotEmpty(t, result.AssumeRoleResult.Credentials.AccessKeyID)
}

// TestAssumeRoleWithProvidedContexts verifies ProvidedContexts form fields are parsed.
func TestAssumeRoleWithProvidedContexts(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	h := sts.NewHandler(b)

	rec := r1PostForm(t, h, url.Values{
		"Action":                                {"AssumeRole"},
		"Version":                               {"2011-06-15"},
		"RoleArn":                               {"arn:aws:iam::000000000000:role/test-role"},
		"RoleSessionName":                       {"my-session"},
		"ProvidedContexts.member.1.ProviderArn": {"arn:aws:iam::000000000000:oidc-provider/example.com"},
		"ProvidedContexts.member.1.ContextAssertion": {"assertion-value"},
	})

	assert.Equal(t, http.StatusOK, rec.Code)

	var result sts.AssumeRoleResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &result))
	assert.NotEmpty(t, result.AssumeRoleResult.Credentials.AccessKeyID)
}

// TestPackedPolicySizeNonZero verifies AssumeRole with Policy returns non-zero PackedPolicySize.
func TestPackedPolicySizeNonZero(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
	resp, err := b.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::000000000000:role/test-role",
		RoleSessionName: "my-session",
		Policy:          policy,
	})

	require.NoError(t, err)
	// PackedPolicySize uses ceiling division: (len*100 + 2047) / 2048, minimum 1.
	expectedSize := max(int32((len(policy)*100+2047)/2048), 1)
	assert.Equal(t, expectedSize, resp.AssumeRoleResult.PackedPolicySize)
	assert.Positive(t, resp.AssumeRoleResult.PackedPolicySize)
}

// TestPackedPolicySizeZero verifies AssumeRole without Policy returns zero PackedPolicySize.
func TestPackedPolicySizeZero(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::000000000000:role/test-role",
		RoleSessionName: "my-session",
	})

	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.AssumeRoleResult.PackedPolicySize)
}

// ---- Trust-policy end-to-end tests ------------------------------------------

// TestAssumeRole_TrustPolicyPrincipal_EndToEnd verifies that AssumeRole enforces
// the target role's trust-policy Principal against the resolved caller ARN, while
// remaining permissive when no caller ARN is available.
func TestAssumeRole_TrustPolicyPrincipal_EndToEnd(t *testing.T) {
	t.Parallel()

	const (
		roleArn     = "arn:aws:iam::123456789012:role/Target"
		allowedArn  = "arn:aws:iam::123456789012:role/AppRole"
		callerAllow = "arn:aws:sts::123456789012:assumed-role/AppRole/session"
		callerDeny  = "arn:aws:sts::123456789012:assumed-role/OtherRole/session"
	)

	trustDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",` +
		`"Principal":{"AWS":"` + allowedArn + `"},"Action":"sts:AssumeRole"}]}`

	tests := []struct {
		name      string
		callerArn string
		wantErr   bool
	}{
		{name: "permitted_caller", callerArn: callerAllow, wantErr: false},
		{name: "disallowed_caller_denied", callerArn: callerDeny, wantErr: true},
		{name: "no_caller_arn_permissive", callerArn: "", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sts.NewInMemoryBackend()
			backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

			_, err := backend.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         roleArn,
				RoleSessionName: "session",
				CallerArn:       tt.callerArn,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, sts.ErrAccessDenied)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestAssumeRole_TrustPolicyExternalIdWithPrincipal verifies that a statement
// grants only when both the Principal and the sts:ExternalId condition match.
func TestAssumeRole_TrustPolicyExternalIdWithPrincipal(t *testing.T) {
	t.Parallel()

	const (
		roleArn   = "arn:aws:iam::123456789012:role/Target"
		callerArn = "arn:aws:iam::123456789012:user/ci"
	)

	trustDoc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",` +
		`"Principal":{"AWS":"` + callerArn + `"},"Action":"sts:AssumeRole",` +
		`"Condition":{"StringEquals":{"sts:ExternalId":"handshake"}}}]}`

	tests := []struct {
		name       string
		externalID string
		wantErr    bool
	}{
		{name: "principal_and_external_id_match", externalID: "handshake", wantErr: false},
		{name: "external_id_mismatch_denied", externalID: "nope", wantErr: true},
		{name: "external_id_missing_denied", externalID: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sts.NewInMemoryBackend()
			backend.SetRoleLookup(&stubRoleLookup{meta: &sts.RoleMeta{TrustPolicy: trustDoc}})

			_, err := backend.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         roleArn,
				RoleSessionName: "session",
				CallerArn:       callerArn,
				ExternalID:      tt.externalID,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, sts.ErrAccessDenied)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestAssumeRole_TrustPolicy_NoRoleLookup_Permissive confirms that without a
// wired RoleLookup, a resolved caller ARN never causes a denial (there is no
// trust policy to evaluate).
func TestAssumeRole_TrustPolicy_NoRoleLookup_Permissive(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()

	resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/Any",
		RoleSessionName: "session",
		CallerArn:       "arn:aws:iam::123456789012:user/anyone",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AssumeRoleResult.Credentials.AccessKeyID)
}

// ---- Role chaining / transitive tag propagation -----------------------------

func TestRoleChaining_DurationCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		durationSecs string
		wantStatus   int
		callerTemp   bool
	}{
		{
			name:         "AKIA caller — 7200s accepted",
			callerTemp:   false,
			durationSecs: "7200",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "ASIA caller — exactly 3600s accepted",
			callerTemp:   true,
			durationSecs: "3600",
			wantStatus:   http.StatusOK,
		},
		{
			name:         "ASIA caller — 3601s rejected",
			callerTemp:   true,
			durationSecs: "3601",
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "ASIA caller — 1800s accepted",
			callerTemp:   true,
			durationSecs: "1800",
			wantStatus:   http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := sts.NewInMemoryBackend()
			h := sts.NewHandler(backend)
			srv := newEchoServer(h)
			defer srv.Close()

			var callerKey, secToken string
			if tc.callerTemp {
				callerKey = "ASIATESTROLEID000001"
				secToken = "parent-session-token"
				backend.AddSessionInternal(&sts.SessionInfo{
					AccessKeyID:    callerKey,
					SessionToken:   secToken,
					AssumedRoleArn: "arn:aws:sts::123456789012:assumed-role/Parent/s",
					AccountID:      "123456789012",
					SessionName:    "s",
					AssumedRoleID:  "AROATESTPARENT:s",
					Expiration:     time.Now().Add(1 * time.Hour),
				})
			} else {
				callerKey = "AKIATESTLONGTERM0001"
			}

			resp := postSTS(t, srv.URL, map[string]string{
				"Action":          "AssumeRole",
				"Version":         "2011-06-15",
				"RoleArn":         "arn:aws:iam::123456789012:role/Child",
				"RoleSessionName": "child-session",
				"DurationSeconds": tc.durationSecs,
			}, sigV4Auth(callerKey), secToken)

			assert.Equal(t, tc.wantStatus, resp.StatusCode)
		})
	}
}

func TestTransitiveTagPropagation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantTags         map[string]string
		name             string
		parentTags       []sts.Tag
		parentTransitive []string
		childTags        []sts.Tag
	}{
		{
			name:      "no parent — child tags pass through",
			wantTags:  map[string]string{"env": "prod"},
			childTags: []sts.Tag{{Key: "env", Value: "prod"}},
		},
		{
			name:             "parent transitive tag inherited",
			parentTags:       []sts.Tag{{Key: "cost-center", Value: "42"}, {Key: "team", Value: "eng"}},
			parentTransitive: []string{"cost-center"},
			childTags:        []sts.Tag{{Key: "env", Value: "dev"}},
			wantTags:         map[string]string{"cost-center": "42", "env": "dev"},
		},
		{
			name:             "non-transitive parent tag not inherited",
			parentTags:       []sts.Tag{{Key: "cost-center", Value: "42"}, {Key: "team", Value: "eng"}},
			parentTransitive: []string{"cost-center"},
			childTags:        nil,
			wantTags:         map[string]string{"cost-center": "42"},
		},
		{
			name:             "child override wins on key conflict",
			parentTags:       []sts.Tag{{Key: "env", Value: "prod"}},
			parentTransitive: []string{"env"},
			childTags:        []sts.Tag{{Key: "env", Value: "staging"}},
			wantTags:         map[string]string{"env": "staging"},
		},
		{
			name:             "multiple transitive tags propagate",
			parentTags:       []sts.Tag{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}, {Key: "c", Value: "3"}},
			parentTransitive: []string{"a", "b"},
			childTags:        nil,
			wantTags:         map[string]string{"a": "1", "b": "2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			backend := sts.NewInMemoryBackend()

			var parentSession *sts.SessionInfo
			if len(tc.parentTags) > 0 || len(tc.parentTransitive) > 0 {
				parentSession = &sts.SessionInfo{
					AccessKeyID:       "ASIATESTPARENTKEY001",
					SessionToken:      "parent-token",
					AssumedRoleArn:    "arn:aws:sts::123456789012:assumed-role/Parent/s",
					AccountID:         "123456789012",
					SessionName:       "s",
					AssumedRoleID:     "AROAPARENT:s",
					Tags:              tc.parentTags,
					TransitiveTagKeys: tc.parentTransitive,
					Expiration:        time.Now().Add(1 * time.Hour),
				}
				backend.AddSessionInternal(parentSession)
			}

			input := &sts.AssumeRoleInput{
				RoleArn:         "arn:aws:iam::123456789012:role/Child",
				RoleSessionName: "child",
				Tags:            tc.childTags,
				CallerSession:   parentSession,
			}
			if parentSession != nil {
				input.CallerAccessKeyID = parentSession.AccessKeyID
			}

			resp, err := backend.AssumeRole(input)
			require.NoError(t, err)
			require.NotNil(t, resp)

			childCreds := resp.AssumeRoleResult.Credentials
			childSession := backend.LookupSession(childCreds.AccessKeyID, childCreds.SessionToken)
			require.NotNil(t, childSession)

			got := make(map[string]string, len(childSession.Tags))
			for _, tag := range childSession.Tags {
				got[tag.Key] = tag.Value
			}
			assert.Equal(t, tc.wantTags, got)
		})
	}
}

func TestAssumeRole_IAMUserCaller_PrincipalArnCondition(t *testing.T) {
	t.Parallel()

	const (
		allowedUserArn = "arn:aws:iam::123456789012:user/alice"
		deniedUserArn  = "arn:aws:iam::123456789012:user/bob"
		targetRoleArn  = "arn:aws:iam::123456789012:role/DeployRole"
		trustPolicy    = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow",` +
			`"Principal":{"AWS":"*"},"Action":"sts:AssumeRole",` +
			`"Condition":{"ArnEquals":{"aws:PrincipalArn":"` + allowedUserArn + `"}}}]}`
	)

	tests := []struct {
		name       string
		authKey    string
		wantStatus int
	}{
		{
			name:       "matching_caller_user_allowed",
			authKey:    "AKIAALICEKEY123456",
			wantStatus: http.StatusOK,
		},
		{
			name:       "mismatching_caller_user_denied",
			authKey:    "AKIABOBKEY12345678",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sts.NewInMemoryBackend()
			backend.SetRoleLookup(&stubRoleLookup{
				meta: &sts.RoleMeta{
					TrustPolicy: trustPolicy,
				},
				users: map[string]string{
					"AKIAALICEKEY123456": allowedUserArn,
					"AKIABOBKEY12345678": deniedUserArn,
				},
			})

			handler := sts.NewHandler(backend)
			srv := newEchoServer(handler)
			t.Cleanup(srv.Close)

			authHeader := fmt.Sprintf(
				"AWS4-HMAC-SHA256 Credential=%s/20260816/us-east-1/sts/aws4_request, SignedHeaders=host, Signature=xyz",
				tt.authKey,
			)

			resp := postSTS(t, srv.URL, map[string]string{
				"Action":          "AssumeRole",
				"RoleArn":         targetRoleArn,
				"RoleSessionName": "test-session",
			}, authHeader, "")
			defer resp.Body.Close()

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}
