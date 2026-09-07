package azureservicebus_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

func TestInMemoryBackend_CreateSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr     error
		name        string
		createTopic bool
		preexisting bool
		wantCreated bool
	}{
		{name: "new subscription is created", createTopic: true, wantCreated: true},
		{name: "pre-existing subscription is idempotent", createTopic: true, preexisting: true, wantCreated: false},
		{name: "missing topic errors", createTopic: false, wantErr: azureservicebus.ErrTopicNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := azureservicebus.NewInMemoryBackend()

			if tt.createTopic {
				_, err := b.CreateTopic("t")
				require.NoError(t, err)
			}

			if tt.preexisting {
				_, err := b.CreateSubscription("t", "s")
				require.NoError(t, err)
			}

			created, err := b.CreateSubscription("t", "s")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCreated, created)
			assert.True(t, b.SubscriptionExists("t", "s"))
		})
	}
}

func TestInMemoryBackend_DeleteSubscription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr     error
		name        string
		createTopic bool
		preexisting bool
	}{
		{name: "existing subscription is deleted", createTopic: true, preexisting: true},
		{name: "missing subscription errors", createTopic: true, wantErr: azureservicebus.ErrSubscriptionNotFound},
		{name: "missing topic errors", createTopic: false, wantErr: azureservicebus.ErrTopicNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := azureservicebus.NewInMemoryBackend()

			if tt.createTopic {
				_, err := b.CreateTopic("t")
				require.NoError(t, err)
			}

			if tt.preexisting {
				_, err := b.CreateSubscription("t", "s")
				require.NoError(t, err)
			}

			err := b.DeleteSubscription("t", "s")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.False(t, b.SubscriptionExists("t", "s"))
		})
	}
}

func TestInMemoryBackend_SubscriptionExists(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	assert.False(t, b.SubscriptionExists("t", "s"), "nonexistent topic")

	_, err := b.CreateTopic("t")
	require.NoError(t, err)
	assert.False(t, b.SubscriptionExists("t", "s"), "topic exists, subscription doesn't")

	_, err = b.CreateSubscription("t", "s")
	require.NoError(t, err)
	assert.True(t, b.SubscriptionExists("t", "s"))
}
