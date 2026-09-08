package sts_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sts"
)

// TestGetSessionTokenSessionStored verifies GetSessionToken stores a session
// that GetCallerIdentity can subsequently resolve.
func TestGetSessionTokenSessionStored(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.GetSessionToken(&sts.GetSessionTokenInput{})
	require.NoError(t, err)

	creds := resp.GetSessionTokenResult.Credentials
	ci, err := b.GetCallerIdentity(creds.AccessKeyID, creds.SessionToken)
	require.NoError(t, err)
	assert.NotEmpty(t, ci.GetCallerIdentityResult.Arn)
}

// TestGetSessionToken_TokenCodeWithoutSerial verifies the MFA
// SerialNumber/TokenCode pairing requirement for GetSessionToken.
func TestGetSessionToken_TokenCodeWithoutSerial(t *testing.T) {
	t.Parallel()

	t.Run("token_code_without_serial_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			TokenCode: "123456",
		})
		require.ErrorIs(t, err, sts.ErrTokenCodeWithoutSerial)
	})

	t.Run("serial_without_token_code_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			SerialNumber: "arn:aws:iam::123456789012:mfa/my-device",
		})
		require.ErrorIs(t, err, sts.ErrMFACodeRequired)
	})

	t.Run("both_provided_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			SerialNumber: "arn:aws:iam::123456789012:mfa/my-device",
			TokenCode:    "654321",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.GetSessionTokenResult.Credentials.AccessKeyID)
	})

	t.Run("neither_provided_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.GetSessionToken(&sts.GetSessionTokenInput{})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.GetSessionTokenResult.Credentials.AccessKeyID)
	})
}

// TestGetSessionTokenMaxDuration verifies the GetSessionToken duration bounds
// (Accuracy Gap #8: max 129600s / 36h).
func TestGetSessionTokenMaxDuration(t *testing.T) {
	t.Parallel()

	t.Run("accepts_129600", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			DurationSeconds: 129600,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.GetSessionTokenResult.Credentials.AccessKeyID)
	})

	t.Run("rejects_above_129600", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			DurationSeconds: 129601,
		})
		require.ErrorIs(t, err, sts.ErrInvalidDuration)
	})

	t.Run("rejects_old_max_43200_was_valid_now_still_valid", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			DurationSeconds: 43200,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.GetSessionTokenResult.Credentials.AccessKeyID)
	})
}

// TestGetSessionTokenMFAValidation verifies SerialNumber/TokenCode charset
// and format validation (Accuracy Gap #9).
func TestGetSessionTokenMFAValidation(t *testing.T) {
	t.Parallel()

	t.Run("valid_token_code_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			SerialNumber: "arn:aws:iam::123456789012:mfa/my-device",
			TokenCode:    "123456",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.GetSessionTokenResult.Credentials.AccessKeyID)
	})

	t.Run("non_6_digit_code_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			SerialNumber: "arn:aws:iam::123456789012:mfa/my-device",
			TokenCode:    "12345",
		})
		require.ErrorIs(t, err, sts.ErrInvalidMFATokenCode)
	})

	t.Run("alpha_token_code_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			SerialNumber: "arn:aws:iam::123456789012:mfa/my-device",
			TokenCode:    "abc123",
		})
		require.ErrorIs(t, err, sts.ErrInvalidMFATokenCode)
	})

	t.Run("seven_digits_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			SerialNumber: "arn:aws:iam::123456789012:mfa/my-device",
			TokenCode:    "1234567",
		})
		require.ErrorIs(t, err, sts.ErrInvalidMFATokenCode)
	})

	t.Run("valid_mfa_arn_accepted", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			SerialNumber: "arn:aws:iam::000000000000:mfa/virtual-device",
			TokenCode:    "999999",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.GetSessionTokenResult.Credentials.AccessKeyID)
	})

	t.Run("malformed_serial_number_rejected", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		_, err := b.GetSessionToken(&sts.GetSessionTokenInput{
			SerialNumber: "not-a-serial-number",
			TokenCode:    "123456",
		})
		require.ErrorIs(t, err, sts.ErrInvalidMFASerialNumber)
	})
}

func TestGetSessionToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		wantErrContains string
		wantDuration    time.Duration
		tolerance       time.Duration
		duration        int32
		wantErr         bool
	}{
		{
			name:         "DefaultDuration",
			duration:     0,
			wantErr:      false,
			wantDuration: 12 * time.Hour,
			tolerance:    time.Hour,
		},
		{
			name:         "CustomDuration",
			duration:     3600,
			wantErr:      false,
			wantDuration: time.Hour,
			tolerance:    time.Minute,
		},
		{
			name:            "InvalidDuration",
			duration:        100,
			wantErr:         true,
			wantErrContains: "DurationSeconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			backend := sts.NewInMemoryBackend()

			input := &sts.GetSessionTokenInput{}
			if tt.duration != 0 {
				input.DurationSeconds = tt.duration
			}

			resp, err := backend.GetSessionToken(input)

			if tt.wantErr {
				require.Error(t, err, "expected error")
				assert.Contains(t, err.Error(), tt.wantErrContains)

				return
			}

			require.NoError(t, err)

			creds := resp.GetSessionTokenResult.Credentials
			assert.True(
				t,
				strings.HasPrefix(creds.AccessKeyID, "ASIA"),
				"expected ASIA prefix, got %q",
				creds.AccessKeyID,
			)
			assert.NotEmpty(t, creds.SecretAccessKey, "expected non-empty SecretAccessKey")
			assert.NotEmpty(t, creds.SessionToken, "expected non-empty SessionToken")

			exp, err := time.Parse(time.RFC3339, creds.Expiration)
			require.NoError(t, err, "parse expiration")

			diff := time.Until(exp)
			assert.InDelta(t, tt.wantDuration, diff, float64(tt.tolerance), "expected ~%v expiration", tt.wantDuration)
		})
	}
}
