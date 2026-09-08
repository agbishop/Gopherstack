package cognitoidp_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

func TestCognitoIDPJanitor_SweepOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		expiredCount   int
		activeCount    int
		wantAfterSweep int
	}{
		{
			name:           "expired_tokens_removed",
			expiredCount:   2,
			activeCount:    1,
			wantAfterSweep: 1,
		},
		{
			name:           "no_expired_tokens",
			expiredCount:   0,
			activeCount:    2,
			wantAfterSweep: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			pool, err := b.CreateUserPool("janitor-pool")
			require.NoError(t, err)

			client, err := b.CreateUserPoolClient(pool.ID, "janitor-client")
			require.NoError(t, err)

			user, err := b.SignUp(client.ClientID, "janitor-user", "Password123!", nil)
			require.NoError(t, err)
			require.NoError(t, b.ConfirmSignUp(client.ClientID, "janitor-user", user.ConfirmCode))

			totalTokens := tt.expiredCount + tt.activeCount
			refreshTokens := make([]string, 0, totalTokens)

			for range totalTokens {
				tokens, authErr := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "janitor-user", "Password123!")
				require.NoError(t, authErr)
				require.NotNil(t, tokens.Tokens)
				refreshTokens = append(refreshTokens, tokens.Tokens.RefreshToken)
			}

			for i := range tt.expiredCount {
				require.True(t, b.SetRefreshTokenExpiry(refreshTokens[i], time.Now().UTC().Add(-time.Second)))
			}

			j := cognitoidp.NewJanitor(b, time.Minute)
			j.SweepOnce(t.Context())

			assert.Equal(t, tt.wantAfterSweep, b.RefreshTokenCount())
		})
	}
}

func TestCognitoIDPJanitor_SweepOnce_EvictsExpiredAttrVerificationCode(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("attr-janitor-pool")
	require.NoError(t, err)

	client, err := b.CreateUserPoolClient(pool.ID, "attr-janitor-client")
	require.NoError(t, err)

	user, err := b.SignUp(client.ClientID, "attr-janitor-user", "Password123!", map[string]string{
		"email": "attr-janitor@example.com",
	})
	require.NoError(t, err)
	require.NoError(t, b.ConfirmSignUp(client.ClientID, "attr-janitor-user", user.ConfirmCode))

	result, err := b.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "attr-janitor-user", "Password123!")
	require.NoError(t, err)

	code, _, _, err := b.GetUserAttributeVerificationCode(result.Tokens.AccessToken, "email")
	require.NoError(t, err)
	require.NotEmpty(t, code)

	b.ExpireAttrVerificationCodeForTest(pool.ID, "attr-janitor-user", "email")

	j := cognitoidp.NewJanitor(b, time.Minute)
	j.SweepOnce(t.Context())

	assert.Empty(t, b.GetAttrVerificationCodeForTest(pool.ID, "attr-janitor-user", "email"))
}

func TestCognitoIDPHandler_StartWorker_WithJanitor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		withJanitor bool
	}{
		{
			name:        "starts_with_janitor",
			withJanitor: true,
		},
		{
			name:        "starts_without_janitor",
			withJanitor: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			h := cognitoidp.NewHandler(b, "us-east-1")
			if tt.withJanitor {
				h.WithJanitor(10*time.Millisecond, 30*time.Second)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()

			require.NoError(t, h.StartWorker(ctx))
		})
	}
}
