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

func TestAssumeRoot_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input *sts.AssumeRootInput
		name  string
	}{
		{
			name: "default_duration",
			input: &sts.AssumeRootInput{
				TargetPrincipal: "123456789012",
				TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
		},
		{
			name: "custom_duration",
			input: &sts.AssumeRootInput{
				TargetPrincipal: "123456789012",
				TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				DurationSeconds: sts.MaxRootDurationSeconds,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.AssumeRoot(tt.input)
			require.NoError(t, err)
			require.NotNil(t, resp)

			creds := resp.AssumeRootResult.Credentials
			assert.True(t, strings.HasPrefix(creds.AccessKeyID, "ASIA"))
			assert.NotEmpty(t, creds.SecretAccessKey)
			assert.NotEmpty(t, creds.SessionToken)
			assert.NotEmpty(t, creds.Expiration)
			assert.NotEmpty(t, resp.ResponseMetadata.RequestID)
			assert.Equal(t, sts.STSNamespace, resp.Xmlns)
		})
	}
}

func TestAssumeRoot_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		input   *sts.AssumeRootInput
		name    string
	}{
		{
			name: "missing_target_principal",
			input: &sts.AssumeRootInput{
				TaskPolicyArn: "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			},
			wantErr: sts.ErrMissingTargetPrincipal,
		},
		{
			name: "missing_task_policy_arn",
			input: &sts.AssumeRootInput{
				TargetPrincipal: "123456789012",
			},
			wantErr: sts.ErrMissingTaskPolicyArn,
		},
		{
			name: "duration_too_long",
			input: &sts.AssumeRootInput{
				TargetPrincipal: "123456789012",
				TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				DurationSeconds: sts.MaxRootDurationSeconds + 1,
			},
			wantErr: sts.ErrInvalidDuration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			_, err := b.AssumeRoot(tt.input)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestAssumeRoot_SessionTrackedForCallerIdentity(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.AssumeRoot(&sts.AssumeRootInput{
		TargetPrincipal: "123456789012",
		TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
	})
	require.NoError(t, err)

	creds := resp.AssumeRootResult.Credentials

	ciResp, err := b.GetCallerIdentity(creds.AccessKeyID, creds.SessionToken)
	require.NoError(t, err)
	assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "assumed-root")
	assert.Equal(t, 1, b.SessionCount())
}

func TestHandler_AssumeRoot(t *testing.T) {
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
				"Action":          {"AssumeRoot"},
				"Version":         {"2011-06-15"},
				"TargetPrincipal": {"123456789012"},
				"TaskPolicyArn":   {"arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_target_principal_returns_400",
			formValues: url.Values{
				"Action":        {"AssumeRoot"},
				"Version":       {"2011-06-15"},
				"TaskPolicyArn": {"arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials"},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MissingParameter",
		},
		{
			name: "missing_task_policy_arn_returns_400",
			formValues: url.Values{
				"Action":          {"AssumeRoot"},
				"Version":         {"2011-06-15"},
				"TargetPrincipal": {"123456789012"},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "MissingParameter",
		},
		{
			name: "duration_too_long_returns_400",
			formValues: url.Values{
				"Action":          {"AssumeRoot"},
				"Version":         {"2011-06-15"},
				"TargetPrincipal": {"123456789012"},
				"TaskPolicyArn":   {"arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials"},
				"DurationSeconds": {"901"},
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
				var resp sts.AssumeRootResponse
				require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp.AssumeRootResult.Credentials.AccessKeyID)
				assert.NotEmpty(t, resp.AssumeRootResult.Credentials.Expiration)
			}
		})
	}
}

// TestAssumeRootWithPolicyDescriptorType verifies TaskPolicyArn.arn form field is parsed.
func TestAssumeRootWithPolicyDescriptorType(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	h := sts.NewHandler(b)

	rec := r1PostForm(t, h, url.Values{
		"Action":            {"AssumeRoot"},
		"Version":           {"2011-06-15"},
		"TargetPrincipal":   {"000000000000"},
		"TaskPolicyArn.arn": {"arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials"},
	})

	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestAssumeRootMissingTargetPrincipal verifies empty TargetPrincipal gives 400.
func TestAssumeRootMissingTargetPrincipal(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	h := sts.NewHandler(b)

	rec := r1PostForm(t, h, url.Values{
		"Action":            {"AssumeRoot"},
		"Version":           {"2011-06-15"},
		"TaskPolicyArn.arn": {"arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp sts.ErrorResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "MissingParameter", errResp.Error.Code)
}

// TestAssumeRootMissingTaskPolicyArn verifies missing TaskPolicyArn.arn gives 400.
func TestAssumeRootMissingTaskPolicyArn(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	h := sts.NewHandler(b)

	rec := r1PostForm(t, h, url.Values{
		"Action":          {"AssumeRoot"},
		"Version":         {"2011-06-15"},
		"TargetPrincipal": {"000000000000"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp sts.ErrorResponse
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "MissingParameter", errResp.Error.Code)
}

// TestAssumeRootDerivedAccount verifies AssumeRoot uses the TargetPrincipal account.
func TestAssumeRootDerivedAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		targetPrincipal string
		wantAccount     string
	}{
		{
			name:            "account_id",
			targetPrincipal: "123456789012",
			wantAccount:     "123456789012",
		},
		{
			name:            "arn",
			targetPrincipal: "arn:aws:iam::987654321098:root",
			wantAccount:     "987654321098",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			resp, err := b.AssumeRoot(&sts.AssumeRootInput{
				TargetPrincipal: tt.targetPrincipal,
				TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			})
			require.NoError(t, err)
			require.NotEmpty(t, resp.AssumeRootResult.Credentials.AccessKeyID)

			// The account in the session should be derived from TargetPrincipal.
			creds := resp.AssumeRootResult.Credentials
			snap := b.Snapshot(t.Context())
			b2 := sts.NewInMemoryBackend()
			require.NoError(t, b2.Restore(t.Context(), snap))

			ci, err := b2.GetCallerIdentity(creds.AccessKeyID, creds.SessionToken)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAccount, ci.GetCallerIdentityResult.Account)
		})
	}
}

// TestAssumeRoot_TargetPrincipalValidation verifies TargetPrincipal shape validation.
func TestAssumeRoot_TargetPrincipalValidation(t *testing.T) {
	t.Parallel()

	t.Run("valid_12_digit_account_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "123456789012",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		})
		require.NoError(t, err)
	})

	t.Run("valid_arn_target_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "arn:aws:iam::987654321098:root",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		})
		require.NoError(t, err)
	})

	t.Run("invalid_account_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "not-an-account",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		})
		require.ErrorIs(t, err, sts.ErrInvalidTargetPrincipal)
	})

	t.Run("short_account_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "12345",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		})
		require.ErrorIs(t, err, sts.ErrInvalidTargetPrincipal)
	})
}

// TestAssumeRootExactDuration verifies AssumeRoot only accepts exactly 900s (Accuracy Gap #23).
func TestAssumeRootExactDuration(t *testing.T) {
	t.Parallel()

	t.Run("duration_900_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "123456789012",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			DurationSeconds: 900,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AssumeRootResult.Credentials.AccessKeyID)
	})

	t.Run("duration_0_defaults_to_900_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "123456789012",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.AssumeRootResult.Credentials.AccessKeyID)
	})

	t.Run("duration_1800_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "123456789012",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			DurationSeconds: 1800,
		})
		require.ErrorIs(t, err, sts.ErrInvalidDuration)
	})

	t.Run("duration_899_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "123456789012",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			DurationSeconds: 899,
		})
		require.ErrorIs(t, err, sts.ErrInvalidDuration)
	})
}

// TestAssumeRootApprovedPolicyArns verifies TaskPolicyArn must be in the AWS-approved set (Accuracy Gap #7).
func TestAssumeRootApprovedPolicyArns(t *testing.T) {
	t.Parallel()

	approved := []string{
		"arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		"arn:aws:iam::aws:policy/root-task/IAMCreateRootUserPassword",
		"arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials",
		"arn:aws:iam::aws:policy/root-task/S3UnlockBucketPolicy",
		"arn:aws:iam::aws:policy/root-task/SQSUnlockQueuePolicy",
	}

	for _, policyArn := range approved {
		t.Run("approved_"+policyArn[len(policyArn)-15:], func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			_, err := b.AssumeRoot(&sts.AssumeRootInput{
				TargetPrincipal: "123456789012",
				TaskPolicyArn:   policyArn,
			})
			require.NoError(t, err)
		})
	}

	t.Run("non_approved_arn_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "123456789012",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/AdministratorAccess",
		})
		require.ErrorIs(t, err, sts.ErrValidation)
	})
}

// TestAssumeRoot_SourceIdentityPersistsFromCallerSession verifies that
// AssumeRoot's SourceIdentity — which has no corresponding request parameter —
// is inherited from the caller's own STS session, per AWS's documented
// "persists across chained role sessions" behavior for source identity.
func TestAssumeRoot_SourceIdentityPersistsFromCallerSession(t *testing.T) {
	t.Parallel()

	t.Run("no_caller_session_empty_source_identity", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "123456789012",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		})
		require.NoError(t, err)
		assert.Empty(t, resp.AssumeRootResult.SourceIdentity)
	})

	t.Run("caller_session_source_identity_inherited", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.AssumeRoot(&sts.AssumeRootInput{
			TargetPrincipal: "123456789012",
			TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			CallerSession:   &sts.SessionInfo{SourceIdentity: "alice"},
		})
		require.NoError(t, err)
		assert.Equal(t, "alice", resp.AssumeRootResult.SourceIdentity)

		rootCreds := resp.AssumeRootResult.Credentials
		rootSession := b.LookupSession(rootCreds.AccessKeyID, rootCreds.SessionToken)
		require.NotNil(t, rootSession)
		assert.Equal(t, "alice", rootSession.SourceIdentity)
	})
}

// TestAssumeRootResultBeforeMetadata verifies the XML result element precedes ResponseMetadata.
func TestAssumeRootResultBeforeMetadata(t *testing.T) {
	t.Parallel()

	h, _, e := accuracyHandler(t)
	form := url.Values{
		"Action":          {"AssumeRoot"},
		"Version":         {"2011-06-15"},
		"TargetPrincipal": {"123456789012"},
		"TaskPolicyArn":   {"arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials"},
	}
	rec := accuracyPost(t, h, e, form)
	require.Equal(t, http.StatusOK, rec.Code)

	order := xmlElementOrder(t, rec.Body.Bytes())
	require.Len(t, order, 2)
	assert.Equal(t, "AssumeRootResult", order[0])
	assert.Equal(t, "ResponseMetadata", order[1])
}
