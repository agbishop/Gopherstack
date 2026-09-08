package cognitoidp_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

func TestExplicitAuthFlows_Enforcement(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("authflow-pool")
	require.NoError(t, err)

	client, err := b.CreateUserPoolClientWithOpts(pool.ID, "restricted-client", cognitoidp.UserPoolClientOptions{
		ExplicitAuthFlows: []string{"ALLOW_USER_PASSWORD_AUTH"},
	})
	require.NoError(t, err)

	user, err := b.SignUp(client.ClientID, "dave", "Pass1234!", nil)
	require.NoError(t, err)
	err = b.ConfirmSignUp(client.ClientID, "dave", user.ConfirmCode)
	require.NoError(t, err)

	_, err = b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "dave", "Pass1234!")
	require.NoError(t, err)

	_, err = b.InitiateAuth(client.ClientID, "USER_SRP_AUTH", "dave", "Pass1234!")
	require.ErrorIs(t, err, cognitoidp.ErrInvalidUserPoolConfig)
}

func TestSignUpWithValidation_AutoVerify(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("auto-verify-pool", cognitoidp.UserPoolOptions{
		AutoVerifiedAttributes: []string{"email"},
	})
	require.NoError(t, err)

	client, err := b.CreateUserPoolClient(pool.ID, "av-client")
	require.NoError(t, err)

	// AutoVerifiedAttributes selects which contact channel Cognito sends a
	// confirmation code to; it does not skip confirmation itself (AWS docs,
	// "Signing up and confirming user accounts": self-signed-up users always
	// start Unconfirmed, and only a PreSignUp Lambda's autoConfirmUser
	// response -- not AutoVerifiedAttributes -- can bypass that).
	user, err := b.SignUpWithValidation(client.ClientID, "frank", "Pass1234!",
		map[string]string{"email": "frank@example.com"})
	require.NoError(t, err)
	assert.Equal(t, cognitoidp.UserStatusUnconfirmed, user.Status)
	assert.NotEmpty(t, user.ConfirmCode)
	assert.Equal(t, "true", user.Attributes["email_verified"])

	err = b.ConfirmSignUp(client.ClientID, "frank", user.ConfirmCode)
	require.NoError(t, err)
}

func TestSignUpWithValidation_RequiresCode_WhenEmailMissing(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("auto-verify-pool2", cognitoidp.UserPoolOptions{
		AutoVerifiedAttributes: []string{"email"},
	})
	require.NoError(t, err)

	client, err := b.CreateUserPoolClient(pool.ID, "av-client2")
	require.NoError(t, err)

	user, err := b.SignUpWithValidation(client.ClientID, "greta", "Pass1234!", nil)
	require.NoError(t, err)
	assert.Equal(t, cognitoidp.UserStatusUnconfirmed, user.Status)
	assert.NotEmpty(t, user.ConfirmCode)
}

func TestAdminCreateUser_ForceChangePassword(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("fcp-pool")
	require.NoError(t, err)

	client, err := b.CreateUserPoolClient(pool.ID, "fcp-client")
	require.NoError(t, err)

	_, err = b.AdminCreateUser(pool.ID, "hank", "Temp1234!", nil)
	require.NoError(t, err)

	result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "hank", "Temp1234!")
	require.NoError(t, err)
	assert.Equal(t, "NEW_PASSWORD_REQUIRED", result.ChallengeName)
	assert.NotEmpty(t, result.MFASession)
}

func TestForceChangePassword_TemporaryPasswordExpires(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := newTestBackend()
		pool, err := b.CreateUserPoolWithOpts("temp-pwd-pool", cognitoidp.UserPoolOptions{
			PasswordPolicy: &cognitoidp.PasswordPolicy{
				TemporaryPasswordValidityDays: 1,
			},
		})
		require.NoError(t, err)

		client, err := b.CreateUserPoolClient(pool.ID, "temp-pwd-client")
		require.NoError(t, err)

		_, err = b.AdminCreateUser(pool.ID, "ivy", "Temp1234!", nil)
		require.NoError(t, err)

		result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "ivy", "Temp1234!")
		require.NoError(t, err)
		assert.Equal(t, "NEW_PASSWORD_REQUIRED", result.ChallengeName,
			"temp password within the validity window must still work")

		time.Sleep(25 * time.Hour)
		synctest.Wait()

		_, err = b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "ivy", "Temp1234!")
		require.Error(t, err)
		require.ErrorIs(t, err, cognitoidp.ErrNotAuthorized,
			"an expired temporary password must be rejected, not challenged")
		assert.Contains(t, err.Error(), "expired")
	})
}

func TestSignUpWithValidation_PasswordPolicyEnforced(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("pp-enforce-pool", cognitoidp.UserPoolOptions{
		PasswordPolicy: &cognitoidp.PasswordPolicy{
			MinimumLength:    10,
			RequireUppercase: true,
			RequireNumbers:   true,
		},
	})
	require.NoError(t, err)

	client, err := b.CreateUserPoolClient(pool.ID, "pp-client")
	require.NoError(t, err)

	_, err = b.SignUpWithValidation(client.ClientID, "pat", "short", nil)
	require.ErrorIs(t, err, cognitoidp.ErrInvalidPassword)

	_, err = b.SignUpWithValidation(client.ClientID, "pat2", "alllowercase1", nil)
	require.ErrorIs(t, err, cognitoidp.ErrInvalidPassword)

	_, err = b.SignUpWithValidation(client.ClientID, "pat3", "LongEnough1234", nil)
	require.NoError(t, err)
}

func TestChangePassword_PolicyEnforced(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("cp-pool", cognitoidp.UserPoolOptions{
		PasswordPolicy: &cognitoidp.PasswordPolicy{
			MinimumLength:    8,
			RequireUppercase: true,
		},
	})
	require.NoError(t, err)

	client, err := b.CreateUserPoolClient(pool.ID, "cp-client")
	require.NoError(t, err)

	user, err := b.SignUpWithValidation(client.ClientID, "quinn", "Pass1234",
		map[string]string{"email": "q@x.com"})
	require.NoError(t, err)
	err = b.AdminConfirmSignUp(pool.ID, user.Username)
	require.NoError(t, err)

	result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "quinn", "Pass1234")
	require.NoError(t, err)

	err = b.ChangePassword(result.Tokens.AccessToken, "Pass1234", "toolow")
	require.ErrorIs(t, err, cognitoidp.ErrInvalidPassword)

	err = b.ChangePassword(result.Tokens.AccessToken, "Pass1234", "NewPass1234")
	require.NoError(t, err)
}

func TestConfirmForgotPassword_PolicyEnforced(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("cfp-pool", cognitoidp.UserPoolOptions{
		PasswordPolicy: &cognitoidp.PasswordPolicy{
			MinimumLength:    8,
			RequireUppercase: true,
		},
	})
	require.NoError(t, err)

	client, err := b.CreateUserPoolClient(pool.ID, "cfp-client")
	require.NoError(t, err)

	user, err := b.SignUpWithValidation(client.ClientID, "rachel", "Pass1234",
		map[string]string{"email": "r@x.com"})
	require.NoError(t, err)
	err = b.AdminConfirmSignUp(pool.ID, user.Username)
	require.NoError(t, err)

	code, err := b.ForgotPassword(client.ClientID, "rachel")
	require.NoError(t, err)

	err = b.ConfirmForgotPassword(client.ClientID, "rachel", code, "toolow")
	require.ErrorIs(t, err, cognitoidp.ErrInvalidPassword)

	err = b.ConfirmForgotPassword(client.ClientID, "rachel", code, "NewPass1234")
	require.NoError(t, err)
}

func TestSecretHash_InitiateAuth_Valid(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("sh-pool", cognitoidp.UserPoolOptions{})
	require.NoError(t, err)

	client, err := b.CreateUserPoolClientWithOpts(pool.ID, "sh-client", cognitoidp.UserPoolClientOptions{
		GenerateSecret: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, client.ClientSecret)

	user, err := b.SignUp(client.ClientID, "alice", "Pass1234!", nil)
	require.NoError(t, err)
	err = b.ConfirmSignUp(client.ClientID, "alice", user.ConfirmCode)
	require.NoError(t, err)

	validHash := computeSecretHash(client.ClientID, "alice", client.ClientSecret)
	err = b.ValidateSecretHash(client.ClientID, "alice", validHash)
	require.NoError(t, err)
}

// TestSecretHash_Validation table-drives the ValidateSecretHash matrix
// previously spread across four near-identical single-scenario tests
// (invalid hash, secret required, secret forbidden, empty accepted).
func TestSecretHash_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(b *cognitoidp.InMemoryBackend) *cognitoidp.UserPoolClient
		errTarget error
		name      string
		hash      string
		wantErr   bool
	}{
		{
			name: "wrong_hash_rejected",
			setup: func(b *cognitoidp.InMemoryBackend) *cognitoidp.UserPoolClient {
				pool, err := b.CreateUserPoolWithOpts("sh-fail-pool", cognitoidp.UserPoolOptions{})
				require.NoError(t, err)

				client, err := b.CreateUserPoolClientWithOpts(
					pool.ID,
					"sh-fail-client",
					cognitoidp.UserPoolClientOptions{
						GenerateSecret: true,
					},
				)
				require.NoError(t, err)

				return client
			},
			hash:      "wronghash",
			wantErr:   true,
			errTarget: cognitoidp.ErrNotAuthorized,
		},
		{
			name: "required_when_client_has_secret",
			setup: func(b *cognitoidp.InMemoryBackend) *cognitoidp.UserPoolClient {
				pool, err := b.CreateUserPoolWithOpts("sh-req-pool", cognitoidp.UserPoolOptions{})
				require.NoError(t, err)

				client, err := b.CreateUserPoolClientWithOpts(
					pool.ID,
					"sh-req-client",
					cognitoidp.UserPoolClientOptions{
						GenerateSecret: true,
					},
				)
				require.NoError(t, err)

				return client
			},
			hash:      "",
			wantErr:   true,
			errTarget: cognitoidp.ErrInvalidParameter,
		},
		{
			name: "forbidden_when_client_has_no_secret",
			setup: func(b *cognitoidp.InMemoryBackend) *cognitoidp.UserPoolClient {
				pool, err := b.CreateUserPoolWithOpts("sh-forbid-pool", cognitoidp.UserPoolOptions{})
				require.NoError(t, err)

				client, err := b.CreateUserPoolClientWithOpts(
					pool.ID,
					"sh-forbid-client",
					cognitoidp.UserPoolClientOptions{},
				)
				require.NoError(t, err)

				return client
			},
			hash:      "somehash",
			wantErr:   true,
			errTarget: cognitoidp.ErrInvalidParameter,
		},
		{
			name: "accepts_empty_when_no_secret",
			setup: func(b *cognitoidp.InMemoryBackend) *cognitoidp.UserPoolClient {
				pool, err := b.CreateUserPoolWithOpts("sh-ok-pool", cognitoidp.UserPoolOptions{})
				require.NoError(t, err)

				client, err := b.CreateUserPoolClientWithOpts(
					pool.ID,
					"sh-ok-client",
					cognitoidp.UserPoolClientOptions{},
				)
				require.NoError(t, err)

				return client
			},
			hash: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			client := tt.setup(b)

			err := b.ValidateSecretHash(client.ClientID, "alice", tt.hash)
			if tt.wantErr {
				require.ErrorIs(t, err, tt.errTarget)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestInMemoryBackend_SignUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget error
		setup     func(b *cognitoidp.InMemoryBackend) string
		name      string
		username  string
		password  string
		wantErr   bool
	}{
		{
			name: "success",
			setup: func(b *cognitoidp.InMemoryBackend) string {
				pool, _ := b.CreateUserPool("p")
				client, _ := b.CreateUserPoolClient(pool.ID, "c")

				return client.ClientID
			},
			username: "alice",
			password: "Password123!",
		},
		{
			name: "duplicate_user",
			setup: func(b *cognitoidp.InMemoryBackend) string {
				pool, _ := b.CreateUserPool("p")
				client, _ := b.CreateUserPoolClient(pool.ID, "c")
				_, _ = b.SignUp(client.ClientID, "alice", "Password123!", nil)

				return client.ClientID
			},
			username:  "alice",
			password:  "Password123!",
			wantErr:   true,
			errTarget: cognitoidp.ErrUsernameExists,
		},
		{
			name: "client_not_found",
			setup: func(_ *cognitoidp.InMemoryBackend) string {
				return "nonexistent-client"
			},
			username:  "alice",
			password:  "Password123!",
			wantErr:   true,
			errTarget: cognitoidp.ErrClientNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			clientID := tt.setup(b)

			user, err := b.SignUp(clientID, tt.username, tt.password, map[string]string{"email": "alice@example.com"})

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.username, user.Username)
			assert.Equal(t, cognitoidp.UserStatusUnconfirmed, user.Status)
			assert.NotEmpty(t, user.Sub)
		})
	}
}

func TestInMemoryBackend_ConfirmSignUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget        error
		setup            func(b *cognitoidp.InMemoryBackend) string
		name             string
		username         string
		confirmationCode string
		wantErr          bool
	}{
		{
			name: "success",
			setup: func(b *cognitoidp.InMemoryBackend) string {
				pool, _ := b.CreateUserPool("p")
				client, _ := b.CreateUserPoolClient(pool.ID, "c")
				u, _ := b.SignUp(client.ClientID, "bob", "Password123!", nil)

				_ = b.ConfirmSignUp(client.ClientID, "bob", u.ConfirmCode)

				return client.ClientID
			},
			username:         "bob",
			confirmationCode: "irrelevant-the-setup-already-confirmed",
		},
		{
			name: "user_not_found",
			setup: func(b *cognitoidp.InMemoryBackend) string {
				pool, _ := b.CreateUserPool("p")
				client, _ := b.CreateUserPoolClient(pool.ID, "c")

				return client.ClientID
			},
			username:         "nobody",
			confirmationCode: "123456",
			wantErr:          true,
			errTarget:        cognitoidp.ErrUserNotFound,
		},
		{
			name: "empty_code",
			setup: func(b *cognitoidp.InMemoryBackend) string {
				pool, _ := b.CreateUserPool("p")
				client, _ := b.CreateUserPoolClient(pool.ID, "c")
				_, _ = b.SignUp(client.ClientID, "carol", "Password123!", nil)

				return client.ClientID
			},
			username:         "carol",
			confirmationCode: "",
			wantErr:          true,
			errTarget:        cognitoidp.ErrCodeMismatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			clientID := tt.setup(b)

			err := b.ConfirmSignUp(clientID, tt.username, tt.confirmationCode)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestBackend_SignUpWithValidation_PasswordPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "valid", password: "ValidPass123!"},
		{name: "too_short", password: "Abc1!", wantErr: true},
		{name: "no_uppercase", password: "lowercase1!", wantErr: true},
		{name: "no_lowercase", password: "UPPERCASE1!", wantErr: true},
		{name: "no_number", password: "NoNumber!", wantErr: true},
		{name: "no_symbol", password: "NoSymbol123", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			pool, err := b.CreateUserPool("policy-pool")
			require.NoError(t, err)

			require.NoError(t, b.UpdateUserPoolWithOpts(pool.ID, "", cognitoidp.UserPoolOptions{
				PasswordPolicy: &cognitoidp.PasswordPolicy{
					MinimumLength:    8,
					RequireUppercase: true,
					RequireLowercase: true,
					RequireNumbers:   true,
					RequireSymbols:   true,
				},
			}))

			client, err := b.CreateUserPoolClientWithOpts(pool.ID, "pc", cognitoidp.UserPoolClientOptions{})
			require.NoError(t, err)

			_, signUpErr := b.SignUpWithValidation(client.ClientID, "test-user", tt.password, map[string]string{})

			if tt.wantErr {
				require.Error(t, signUpErr)
			} else {
				require.NoError(t, signUpErr)
			}
		})
	}
}
