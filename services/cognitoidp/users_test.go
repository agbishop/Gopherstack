package cognitoidp_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

func TestUserSRPAuth_TwoStepFlow(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("srp-pool", cognitoidp.UserPoolOptions{})
	require.NoError(t, err)

	client, err := b.CreateUserPoolClientWithOpts(pool.ID, "srp-client", cognitoidp.UserPoolClientOptions{
		ExplicitAuthFlows: []string{"ALLOW_USER_SRP_AUTH"},
	})
	require.NoError(t, err)

	user, err := b.SignUp(client.ClientID, "tom", "Pass1234!", nil)
	require.NoError(t, err)
	err = b.ConfirmSignUp(client.ClientID, "tom", user.ConfirmCode)
	require.NoError(t, err)

	srpClient := newSRPTestClient(t)

	// Step 1: returns PASSWORD_VERIFIER challenge.
	result, err := b.InitiateAuthSRP(client.ClientID, "USER_SRP_AUTH", "tom", srpClient.srpA())
	require.NoError(t, err)
	assert.Equal(t, "PASSWORD_VERIFIER", result.ChallengeName)
	assert.NotEmpty(t, result.MFASession)

	// Step 2: complete SRP, receive tokens.
	responses := srpClient.challengeResponses(t, pool.ID, "Pass1234!", result.ChallengeParameters)
	authResult, err := b.RespondToSRPChallenge(client.ClientID, result.MFASession, responses)
	require.NoError(t, err)
	require.NotNil(t, authResult.Tokens)
	assert.NotEmpty(t, authResult.Tokens.IDToken)
	assert.NotEmpty(t, authResult.Tokens.AccessToken)
}

func TestUserSRPAuth_WrongPassword_Rejected(t *testing.T) {
	t.Parallel()

	b, pool, client := setupTestPoolAndClient(t)

	user, err := b.SignUp(client.ClientID, "uma", "Pass1234!", nil)
	require.NoError(t, err)
	err = b.ConfirmSignUp(client.ClientID, "uma", user.ConfirmCode)
	require.NoError(t, err)

	srpClient := newSRPTestClient(t)

	result, err := b.InitiateAuthSRP(client.ClientID, "USER_SRP_AUTH", "uma", srpClient.srpA())
	require.NoError(t, err)

	responses := srpClient.challengeResponses(t, pool.ID, "WrongPassword!", result.ChallengeParameters)
	_, err = b.RespondToSRPChallenge(client.ClientID, result.MFASession, responses)
	require.ErrorIs(t, err, cognitoidp.ErrNotAuthorized)
}

func TestUserSRPAuth_SessionExpiry(t *testing.T) {
	t.Parallel()

	b, _, client := setupTestPoolAndClient(t)

	user, err := b.SignUp(client.ClientID, "ursula", "Pass1234!", nil)
	require.NoError(t, err)
	err = b.ConfirmSignUp(client.ClientID, "ursula", user.ConfirmCode)
	require.NoError(t, err)

	srpClient := newSRPTestClient(t)

	result, err := b.InitiateAuthSRP(client.ClientID, "USER_SRP_AUTH", "ursula", srpClient.srpA())
	require.NoError(t, err)

	b.ExpireMFASessionForTest(result.MFASession)

	_, err = b.RespondToSRPChallenge(client.ClientID, result.MFASession, map[string]string{})
	require.Error(t, err, "expired SRP session must be rejected")
}

func TestAdminCreateUserWithPolicy_Enforced(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("admin-policy-pool", cognitoidp.UserPoolOptions{
		PasswordPolicy: &cognitoidp.PasswordPolicy{
			MinimumLength:    8,
			RequireUppercase: true,
		},
	})
	require.NoError(t, err)

	_, err = b.AdminCreateUserWithPolicy(pool.ID, "badpass", "short", nil)
	require.ErrorIs(t, err, cognitoidp.ErrInvalidPassword)

	_, err = b.AdminCreateUserWithPolicy(pool.ID, "goodpass", "ValidTemp1", nil)
	require.NoError(t, err)
}

func TestInMemoryBackend_AdminCreateUser(t *testing.T) {
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

				return pool.ID
			},
			username: "iris",
			password: "Temp123!",
		},
		{
			name: "duplicate_user",
			setup: func(b *cognitoidp.InMemoryBackend) string {
				pool, _ := b.CreateUserPool("p")
				_, _ = b.AdminCreateUser(pool.ID, "iris", "Temp123!", nil)

				return pool.ID
			},
			username:  "iris",
			password:  "Temp123!",
			wantErr:   true,
			errTarget: cognitoidp.ErrUsernameExists,
		},
		{
			name: "pool_not_found",
			setup: func(_ *cognitoidp.InMemoryBackend) string {
				return "us-east-1_nonexistent"
			},
			username:  "iris",
			password:  "Temp123!",
			wantErr:   true,
			errTarget: cognitoidp.ErrUserPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			poolID := tt.setup(b)

			user, err := b.AdminCreateUser(
				poolID,
				tt.username,
				tt.password,
				map[string]string{"email": "test@example.com"},
			)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.username, user.Username)
			assert.Equal(t, cognitoidp.UserStatusForceChangePassword, user.Status)
		})
	}
}

func TestInMemoryBackend_AdminSetUserPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget error
		setup     func(b *cognitoidp.InMemoryBackend) (poolID, username string)
		name      string
		password  string
		permanent bool
		wantErr   bool
	}{
		{
			name: "permanent_password",
			setup: func(b *cognitoidp.InMemoryBackend) (string, string) {
				pool, _ := b.CreateUserPool("p")
				_, _ = b.AdminCreateUser(pool.ID, "jack", "Temp!", nil)

				return pool.ID, "jack"
			},
			password:  "NewPass123!",
			permanent: true,
		},
		{
			name: "temporary_password",
			setup: func(b *cognitoidp.InMemoryBackend) (string, string) {
				pool, _ := b.CreateUserPool("p")
				_, _ = b.AdminCreateUser(pool.ID, "kate", "Temp!", nil)

				return pool.ID, "kate"
			},
			password:  "NewTemp123!",
			permanent: false,
		},
		{
			name: "user_not_found",
			setup: func(b *cognitoidp.InMemoryBackend) (string, string) {
				pool, _ := b.CreateUserPool("p")

				return pool.ID, "nobody"
			},
			password:  "Pass123!",
			permanent: true,
			wantErr:   true,
			errTarget: cognitoidp.ErrUserNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			poolID, username := tt.setup(b)

			err := b.AdminSetUserPassword(poolID, username, tt.password, tt.permanent)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)

			if tt.permanent {
				user, getUserErr := b.AdminGetUser(poolID, username)
				require.NoError(t, getUserErr)
				assert.Equal(t, cognitoidp.UserStatusConfirmed, user.Status)
			}
		})
	}
}

func TestInMemoryBackend_AdminGetUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget error
		setup     func(b *cognitoidp.InMemoryBackend) (poolID, username string)
		name      string
		wantErr   bool
	}{
		{
			name: "success",
			setup: func(b *cognitoidp.InMemoryBackend) (string, string) {
				pool, _ := b.CreateUserPool("p")
				_, _ = b.AdminCreateUser(pool.ID, "lena", "Temp!", nil)

				return pool.ID, "lena"
			},
		},
		{
			name: "not_found",
			setup: func(b *cognitoidp.InMemoryBackend) (string, string) {
				pool, _ := b.CreateUserPool("p")

				return pool.ID, "nobody"
			},
			wantErr:   true,
			errTarget: cognitoidp.ErrUserNotFound,
		},
		{
			name: "pool_not_found",
			setup: func(_ *cognitoidp.InMemoryBackend) (string, string) {
				return "us-east-1_nonexistent", "user"
			},
			wantErr:   true,
			errTarget: cognitoidp.ErrUserPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			poolID, username := tt.setup(b)

			user, err := b.AdminGetUser(poolID, username)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, username, user.Username)
		})
	}
}

// TestInMemoryBackend_AdminGetUserAuthFactors covers the AdminGetUserAuthFactors
// backend method, verifying every returned factor is derived from real,
// independently-set user state (password hash, legacy SMS MFAOptions,
// verified TOTP secret, registered WebAuthn credential) rather than fabricated.
func TestInMemoryBackend_AdminGetUserAuthFactors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget   error
		setup       func(t *testing.T, b *cognitoidp.InMemoryBackend) (poolID, username string)
		name        string
		wantFactors []string
		wantErr     bool
	}{
		{
			name: "password_only",
			setup: func(t *testing.T, b *cognitoidp.InMemoryBackend) (string, string) {
				t.Helper()

				pool, err := b.CreateUserPool("factors-pool-1")
				require.NoError(t, err)
				client, err := b.CreateUserPoolClient(pool.ID, "factors-client-1")
				require.NoError(t, err)

				_ = signUpConfirmAndLogin(t, b, client.ClientID, "factors-user-1")

				return pool.ID, "factors-user-1"
			},
			wantFactors: []string{"PASSWORD"},
		},
		{
			name: "sms_totp_and_webauthn_all_derived_from_real_state",
			setup: func(t *testing.T, b *cognitoidp.InMemoryBackend) (string, string) {
				t.Helper()

				pool, err := b.CreateUserPool("factors-pool-2")
				require.NoError(t, err)
				client, err := b.CreateUserPoolClient(pool.ID, "factors-client-2")
				require.NoError(t, err)

				tokens := signUpConfirmAndLogin(t, b, client.ClientID, "factors-user-2")

				require.NoError(t, b.AdminSetUserSettings(pool.ID, "factors-user-2", []cognitoidp.MFAOptionType{
					{DeliveryMedium: "SMS", AttributeName: "phone_number"},
				}))

				secret, _, err := b.AssociateSoftwareToken(tokens.AccessToken, "")
				require.NoError(t, err)
				code, err := cognitoidp.GenerateTOTPCode(secret, time.Now())
				require.NoError(t, err)
				_, err = b.VerifySoftwareToken(tokens.AccessToken, "", code)
				require.NoError(t, err)

				_, err = b.CompleteWebAuthnRegistration(tokens.AccessToken, "admin-factor-cred", "", nil)
				require.NoError(t, err)

				return pool.ID, "factors-user-2"
			},
			wantFactors: []string{"PASSWORD", "SMS_OTP", "SOFTWARE_TOKEN", "WEB_AUTHN"},
		},
		{
			name: "user_not_found",
			setup: func(t *testing.T, b *cognitoidp.InMemoryBackend) (string, string) {
				t.Helper()

				pool, err := b.CreateUserPool("factors-pool-3")
				require.NoError(t, err)

				return pool.ID, "ghost"
			},
			wantErr:   true,
			errTarget: cognitoidp.ErrUserNotFound,
		},
		{
			name: "pool_not_found",
			setup: func(t *testing.T, _ *cognitoidp.InMemoryBackend) (string, string) {
				t.Helper()

				return "us-east-1_nonexistent", "someone"
			},
			wantErr:   true,
			errTarget: cognitoidp.ErrUserPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			poolID, username := tt.setup(t, b)

			user, factors, err := b.AdminGetUserAuthFactors(poolID, username)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, username, user.Username)
			assert.ElementsMatch(t, tt.wantFactors, factors)
		})
	}
}

// TestInMemoryBackend_GetUserAuthFactors_SoftwareToken is a regression guard:
// GetUserAuthFactors and AdminGetUserAuthFactors share commonAuthFactorSetLocked,
// which must derive SOFTWARE_TOKEN for both -- a real SDK client's
// ConfiguredUserAuthFactors uses the same types.AuthFactorType enum
// (PASSWORD/EMAIL_OTP/SMS_OTP/WEB_AUTHN/SOFTWARE_TOKEN) on both
// GetUserAuthFactorsOutput and AdminGetUserAuthFactorsOutput.
func TestInMemoryBackend_GetUserAuthFactors_SoftwareToken(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("factors-pool-self")
	require.NoError(t, err)
	client, err := b.CreateUserPoolClient(pool.ID, "factors-client-self")
	require.NoError(t, err)

	tokens := signUpConfirmAndLogin(t, b, client.ClientID, "factors-user-self")

	secret, _, err := b.AssociateSoftwareToken(tokens.AccessToken, "")
	require.NoError(t, err)
	code, err := cognitoidp.GenerateTOTPCode(secret, time.Now())
	require.NoError(t, err)
	_, err = b.VerifySoftwareToken(tokens.AccessToken, "", code)
	require.NoError(t, err)

	_, factors, err := b.GetUserAuthFactors(tokens.AccessToken)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"PASSWORD", "SOFTWARE_TOKEN"}, factors)
}

func TestInMemoryBackend_ListUsersFiltered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget error
		name      string
		filter    string
		wantNames []string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "no_filter_returns_all",
			filter:    "",
			wantCount: 3,
		},
		{
			name:      "username_prefix_filter",
			filter:    `username ^= "ali"`,
			wantCount: 1,
			wantNames: []string{"alice"},
		},
		{
			name:      "username_wildcard_filter",
			filter:    `username = "bob*"`,
			wantCount: 1,
			wantNames: []string{"bob"},
		},
		{
			name:      "no_match",
			filter:    `username ^= "zzz"`,
			wantCount: 0,
		},
		{
			name:      "cognito_user_status_filter",
			filter:    `cognito:user_status = "FORCE_CHANGE_PASSWORD"`,
			wantCount: 3,
		},
		{
			name:      "status_enabled_filter",
			filter:    `status = "Enabled"`,
			wantCount: 3,
		},
		{
			name:      "pool_not_found",
			wantErr:   true,
			errTarget: cognitoidp.ErrUserPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if !tt.wantErr {
				p, _ := b.CreateUserPool("pool")
				_, _ = b.AdminCreateUser(p.ID, "alice", "Pass1!", nil)
				_, _ = b.AdminCreateUser(p.ID, "bob", "Pass1!", nil)
				_, _ = b.AdminCreateUser(p.ID, "charlie", "Pass1!", nil)

				users, err := b.ListUsersFiltered(p.ID, tt.filter)
				require.NoError(t, err)
				assert.Len(t, users, tt.wantCount)

				if len(tt.wantNames) > 0 {
					names := make([]string, 0, len(users))
					for _, u := range users {
						names = append(names, u.Username)
					}
					assert.Equal(t, tt.wantNames, names)
				}

				return
			}

			_, err := b.ListUsersFiltered("us-east-1_missing", tt.filter)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.errTarget)
		})
	}
}

func TestInMemoryBackend_ListUsersFiltered_BySub(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	p, err := b.CreateUserPool("pool")
	require.NoError(t, err)

	alice, err := b.AdminCreateUser(p.ID, "alice", "Pass1!", nil)
	require.NoError(t, err)
	_, err = b.AdminCreateUser(p.ID, "bob", "Pass1!", nil)
	require.NoError(t, err)

	users, err := b.ListUsersFiltered(p.ID, `sub = "`+alice.Sub+`"`)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "alice", users[0].Username)
}

func TestAdminCreateUser_Backend_Full(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("admin-create-backend-pool")
	require.NoError(t, err)

	// Create with SUPPRESS.
	user, err := b.AdminCreateUserFull(
		pool.ID, "backend-user", "Temp1234!",
		map[string]string{"email": "backend@example.com"},
		"SUPPRESS", nil, false,
	)
	require.NoError(t, err)
	assert.Equal(t, "FORCE_CHANGE_PASSWORD", user.Status)
	// SUPPRESS: custom:temporaryPassword should NOT be set.
	assert.Empty(t, user.Attributes["custom:temporaryPassword"])

	// Duplicate should fail (not RESEND).
	_, err = b.AdminCreateUserFull(pool.ID, "backend-user", "New1234!", nil, "", nil, false)
	require.Error(t, err)
}

func TestAdminSetUserPasswordFull_PolicyEnforced(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("admin-pwd-policy-pool", cognitoidp.UserPoolOptions{
		PasswordPolicy: &cognitoidp.PasswordPolicy{
			MinimumLength:    10,
			RequireUppercase: true,
			RequireNumbers:   true,
			RequireSymbols:   true,
		},
	})
	require.NoError(t, err)

	_, err = b.AdminCreateUser(pool.ID, "policy-user", "Temp1234!@#", nil)
	require.NoError(t, err)

	// Short password — policy violation.
	err = b.AdminSetUserPasswordFull(pool.ID, "policy-user", "short", true)
	require.Error(t, err)

	// Valid password.
	err = b.AdminSetUserPasswordFull(pool.ID, "policy-user", "LongPass1234!", true)
	require.NoError(t, err)
}

func TestAdminSetUserPassword_Backend_UserNotFound(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("admin-pwd-notfound-pool")
	require.NoError(t, err)

	err = b.AdminSetUserPasswordFull(pool.ID, "nonexistent", "Pass1234!", true)
	require.Error(t, err)
}

func TestBackend_ListUsers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{name: "success_empty"},
		{name: "with_users"},
		{name: "pool_not_found", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.name == "pool_not_found" {
				_, err := b.ListUsers("bad-pool")
				require.Error(t, err)

				return
			}

			pool, err := b.CreateUserPool("list-users-pool")
			require.NoError(t, err)

			client, err := b.CreateUserPoolClient(pool.ID, "lc")
			require.NoError(t, err)

			if tt.name == "with_users" {
				for _, name := range []string{"alice", "bob"} {
					user, err2 := b.SignUp(client.ClientID, name, "Pass1234!", map[string]string{})
					require.NoError(t, err2)
					require.NoError(t, b.ConfirmSignUp(client.ClientID, name, user.ConfirmCode))
				}
			}

			users, err := b.ListUsers(pool.ID)
			require.NoError(t, err)

			if tt.name == "with_users" {
				assert.Len(t, users, 2)
			} else {
				assert.Empty(t, users)
			}
		})
	}
}

// TestBackend_DeleteUser covers the backend DeleteUser function.
func TestBackend_DeleteUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget error
		name      string
		wantErr   bool
	}{
		{name: "success"},
		{name: "bad_token", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.name == "bad_token" {
				err := b.DeleteUser("bad-token")
				require.Error(t, err)

				return
			}

			pool, err := b.CreateUserPool("del-pool")
			require.NoError(t, err)

			client, err := b.CreateUserPoolClient(pool.ID, "dc")
			require.NoError(t, err)

			user, err := b.SignUp(client.ClientID, "del-me", "Pass1234!", map[string]string{})
			require.NoError(t, err)
			require.NoError(t, b.ConfirmSignUp(client.ClientID, "del-me", user.ConfirmCode))

			result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "del-me", "Pass1234!")
			require.NoError(t, err)
			require.NotNil(t, result.Tokens)

			err = b.DeleteUser(result.Tokens.AccessToken)
			require.NoError(t, err)

			assert.Equal(t, 0, b.UserCount())
		})
	}
}

func TestDeleteUser_ClearsDeviceStateOnRecreate(t *testing.T) {
	t.Parallel()

	t.Run("admin_delete", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		pool, err := b.CreateUserPool("admin-del-pool")
		require.NoError(t, err)

		_, err = b.AdminCreateUser(pool.ID, "reused-user", "Pass1234!", nil)
		require.NoError(t, err)

		b.SeedDeviceForTest(pool.ID, "reused-user", &cognitoidp.Device{DeviceKey: "dev1", Status: "valid"})
		b.SeedAuthEventForTest(pool.ID, "reused-user", &cognitoidp.AuthEvent{EventID: "ev1", EventType: "SignIn"})
		require.True(t, b.HasDeviceStateForTest(pool.ID, "reused-user"))

		require.NoError(t, b.AdminDeleteUser(pool.ID, "reused-user"))

		_, err = b.AdminCreateUser(pool.ID, "reused-user", "Pass1234!", nil)
		require.NoError(t, err)

		assert.False(t, b.HasDeviceStateForTest(pool.ID, "reused-user"))
	})

	t.Run("self_delete", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		pool, err := b.CreateUserPool("self-del-pool")
		require.NoError(t, err)

		client, err := b.CreateUserPoolClient(pool.ID, "self-del-client")
		require.NoError(t, err)

		user, err := b.SignUp(client.ClientID, "reused-user", "Pass1234!", map[string]string{})
		require.NoError(t, err)
		require.NoError(t, b.ConfirmSignUp(client.ClientID, "reused-user", user.ConfirmCode))

		result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "reused-user", "Pass1234!")
		require.NoError(t, err)
		require.NotNil(t, result.Tokens)

		b.SeedDeviceForTest(pool.ID, "reused-user", &cognitoidp.Device{DeviceKey: "dev1", Status: "valid"})
		b.SeedAuthEventForTest(pool.ID, "reused-user", &cognitoidp.AuthEvent{EventID: "ev1", EventType: "SignIn"})
		require.True(t, b.HasDeviceStateForTest(pool.ID, "reused-user"))

		require.NoError(t, b.DeleteUser(result.Tokens.AccessToken))

		user2, err := b.SignUp(client.ClientID, "reused-user", "Pass1234!", map[string]string{})
		require.NoError(t, err)
		require.NoError(t, b.ConfirmSignUp(client.ClientID, "reused-user", user2.ConfirmCode))

		assert.False(t, b.HasDeviceStateForTest(pool.ID, "reused-user"))
	})
}

// TestDeleteUser_ClearsGroupAndWebAuthnStateOnRecreate covers gopherstack-ljak:
// AdminDeleteUser (and self-service DeleteUser) left groupMembers and
// webauthnCredentials behind. Usernames are caller-chosen and the pool
// persists, so recreating a deleted username used to make it inherit group
// membership -- observable through ListUsersInGroup and the
// cognito:groups-feeding userGroupsLocked -- and WebAuthn credentials it was
// never granted, observable through the WEB_AUTHN entry in
// AdminGetUserAuthFactors.
func TestDeleteUser_ClearsGroupAndWebAuthnStateOnRecreate(t *testing.T) {
	t.Parallel()

	t.Run("admin_delete", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		pool, err := b.CreateUserPool("admin-del-group-pool")
		require.NoError(t, err)

		_, err = b.AdminCreateUser(pool.ID, "reused-user", "Pass1234!", nil)
		require.NoError(t, err)
		_, err = b.AdminCreateUser(pool.ID, "kept-user", "Pass1234!", nil)
		require.NoError(t, err)

		_, err = b.CreateGroup(pool.ID, "admins", "", 0)
		require.NoError(t, err)
		require.NoError(t, b.AdminAddUserToGroup(pool.ID, "reused-user", "admins"))
		require.NoError(t, b.AdminAddUserToGroup(pool.ID, "kept-user", "admins"))

		b.SeedWebAuthnCredentialForTest(pool.ID, "reused-user", &cognitoidp.WebAuthnCredential{CredentialID: "cred1"})

		require.NoError(t, b.AdminDeleteUser(pool.ID, "reused-user"))

		_, err = b.AdminCreateUser(pool.ID, "reused-user", "Pass1234!", nil)
		require.NoError(t, err)

		members, err := b.ListUsersInGroup(pool.ID, "admins")
		require.NoError(t, err)

		names := make([]string, len(members))
		for i, m := range members {
			names[i] = m.Username
		}

		assert.NotContains(t, names, "reused-user",
			"recreated username must not inherit the deleted user's group membership")
		assert.Contains(t, names, "kept-user",
			"deleting one user must not disturb another user's group membership")

		_, factors, err := b.AdminGetUserAuthFactors(pool.ID, "reused-user")
		require.NoError(t, err)
		assert.NotContains(t, factors, "WEB_AUTHN",
			"recreated username must not inherit the deleted user's WebAuthn credentials")
	})

	t.Run("self_delete", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		pool, err := b.CreateUserPool("self-del-group-pool")
		require.NoError(t, err)

		client, err := b.CreateUserPoolClient(pool.ID, "self-del-group-client")
		require.NoError(t, err)

		user, err := b.SignUp(client.ClientID, "reused-user", "Pass1234!", map[string]string{})
		require.NoError(t, err)
		require.NoError(t, b.ConfirmSignUp(client.ClientID, "reused-user", user.ConfirmCode))

		result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "reused-user", "Pass1234!")
		require.NoError(t, err)
		require.NotNil(t, result.Tokens)

		_, err = b.CreateGroup(pool.ID, "admins", "", 0)
		require.NoError(t, err)
		require.NoError(t, b.AdminAddUserToGroup(pool.ID, "reused-user", "admins"))

		b.SeedWebAuthnCredentialForTest(pool.ID, "reused-user", &cognitoidp.WebAuthnCredential{CredentialID: "cred1"})

		require.NoError(t, b.DeleteUser(result.Tokens.AccessToken))

		user2, err := b.SignUp(client.ClientID, "reused-user", "Pass1234!", map[string]string{})
		require.NoError(t, err)
		require.NoError(t, b.ConfirmSignUp(client.ClientID, "reused-user", user2.ConfirmCode))

		members, err := b.ListUsersInGroup(pool.ID, "admins")
		require.NoError(t, err)
		assert.Empty(t, members, "recreated username must not inherit the deleted user's group membership")

		_, factors, err := b.AdminGetUserAuthFactors(pool.ID, "reused-user")
		require.NoError(t, err)
		assert.NotContains(t, factors, "WEB_AUTHN",
			"recreated username must not inherit the deleted user's WebAuthn credentials")
	})
}

func unmarshalBody(t *testing.T, rec *httptest.ResponseRecorder, v any) error {
	t.Helper()

	return json.Unmarshal(rec.Body.Bytes(), v)
}

func TestAdminSetUserPassword_PolicyEnforced(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("admin-set-pwd-policy", cognitoidp.UserPoolOptions{
		PasswordPolicy: &cognitoidp.PasswordPolicy{
			MinimumLength:    10,
			RequireUppercase: true,
			RequireNumbers:   true,
			RequireSymbols:   true,
		},
	})
	require.NoError(t, err)

	_, err = b.AdminCreateUser(pool.ID, "policy-user", "Temp1234!@#", nil)
	require.NoError(t, err)

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "too short", password: "short", wantErr: true},
		{name: "missing uppercase/number/symbol", password: "alllowercase", wantErr: true},
		{name: "valid", password: "LongPass1234!", wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			setErr := b.AdminSetUserPassword(pool.ID, "policy-user", tc.password, true)
			if tc.wantErr {
				require.Error(t, setErr)
			} else {
				require.NoError(t, setErr)
			}
		})
	}
}

// TestListUsers_Pagination verifies ListUsers honors Limit and PaginationToken.
