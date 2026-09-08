package shield_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/shield"
)

// TestAudit_Gap3_CreateSubscriptionDefaultsProactiveDisabled verifies DISABLED default.
func TestInMemoryBackend_CreateSubscriptionDefaultsProactiveDisabled(t *testing.T) {
	t.Parallel()

	b := shield.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b.CreateSubscription())
	assert.Equal(t, shield.ProactiveEngagementDisabled, shield.GetProactiveEngagementStatus(b))
}

// --- Gap 4: Protection response includes ApplicationLayerAutomaticResponseConfiguration ---

// TestBackend_DeleteSubscription tests backend subscription deletion.
func TestBackend_DeleteSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(*shield.InMemoryBackend)
		name    string
		wantErr bool
	}{
		{
			name: "success",
			setup: func(b *shield.InMemoryBackend) {
				require.NoError(t, b.CreateSubscription())
			},
		},
		{
			name:    "no subscription",
			setup:   func(_ *shield.InMemoryBackend) {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := shield.NewInMemoryBackend("000000000000", "us-east-1")
			tt.setup(b)

			err := b.DeleteSubscription()

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "INACTIVE", b.GetSubscriptionState())
		})
	}
}

// TestRefinement1_CreateSubscription tests subscription creation.
func TestInMemoryBackend_CreateSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(*shield.InMemoryBackend)
		name    string
		wantErr bool
	}{
		{
			name:    "create_new",
			setup:   func(*shield.InMemoryBackend) {},
			wantErr: false,
		},
		{
			name: "duplicate_silently_accepted",
			setup: func(b *shield.InMemoryBackend) {
				require.NoError(t, b.CreateSubscription())
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := shield.NewInMemoryBackend("000000000000", "us-east-1")
			tt.setup(b)

			err := b.CreateSubscription()

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestRefinement1_GetSubscriptionState tests GetSubscriptionState.
func TestInMemoryBackend_GetSubscriptionState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(*shield.InMemoryBackend)
		wantState string
	}{
		{
			name:      "inactive_no_subscription",
			setup:     func(*shield.InMemoryBackend) {},
			wantState: "INACTIVE",
		},
		{
			name: "active_with_subscription",
			setup: func(b *shield.InMemoryBackend) {
				require.NoError(t, b.CreateSubscription())
			},
			wantState: "ACTIVE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := shield.NewInMemoryBackend("000000000000", "us-east-1")
			tt.setup(b)

			assert.Equal(t, tt.wantState, b.GetSubscriptionState())
		})
	}
}

// TestRefinement1_AddSubscriptionInternal tests the AddSubscriptionInternal seed helper.
func TestInMemoryBackend_AddSubscriptionInternal(t *testing.T) {
	t.Parallel()

	b := shield.NewInMemoryBackend("000000000000", "us-east-1")
	b.AddSubscriptionInternal()

	assert.Equal(t, "ACTIVE", b.GetSubscriptionState())

	sub, err := b.DescribeSubscription()
	require.NoError(t, err)
	assert.Equal(t, shield.AutoRenewEnabled, sub.AutoRenew)
}

// TestRefinement1_UpdateSubscription tests updating subscription auto-renew.
func TestInMemoryBackend_UpdateSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup         func(*shield.InMemoryBackend)
		name          string
		autoRenew     string
		wantAutoRenew string
		wantErr       bool
	}{
		{
			name: "set_disabled",
			setup: func(b *shield.InMemoryBackend) {
				require.NoError(t, b.CreateSubscription())
			},
			autoRenew: shield.AutoRenewDisabled,
		},
		{
			name: "set_enabled",
			setup: func(b *shield.InMemoryBackend) {
				require.NoError(t, b.CreateSubscription())
			},
			autoRenew: shield.AutoRenewEnabled,
		},
		{
			name:      "no_subscription",
			setup:     func(*shield.InMemoryBackend) {},
			autoRenew: shield.AutoRenewDisabled,
			wantErr:   true,
		},
		{
			// api_op_UpdateSubscription.go: an omitted AutoRenew leaves the existing value
			// unchanged.
			name: "omit_preserves_existing_value",
			setup: func(b *shield.InMemoryBackend) {
				require.NoError(t, b.CreateSubscription())
				require.NoError(t, b.UpdateSubscription(shield.AutoRenewDisabled))
			},
			autoRenew:     "",
			wantAutoRenew: shield.AutoRenewDisabled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := shield.NewInMemoryBackend("000000000000", "us-east-1")
			tt.setup(b)

			err := b.UpdateSubscription(tt.autoRenew)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			want := tt.wantAutoRenew
			if want == "" {
				want = tt.autoRenew
			}

			sub, err := b.DescribeSubscription()
			require.NoError(t, err)
			assert.Equal(t, want, sub.AutoRenew)
		})
	}
}
