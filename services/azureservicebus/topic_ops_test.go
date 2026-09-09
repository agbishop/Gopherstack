package azureservicebus_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

func TestInMemoryBackend_CreateTopic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		preexisting bool
		wantCreated bool
	}{
		{name: "new topic is created", preexisting: false, wantCreated: true},
		{name: "pre-existing topic is idempotent", preexisting: true, wantCreated: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := azureservicebus.NewInMemoryBackend()

			if tt.preexisting {
				_, err := b.CreateTopic("t")
				require.NoError(t, err)
			}

			created, err := b.CreateTopic("t")
			require.NoError(t, err)
			assert.Equal(t, tt.wantCreated, created)
			assert.True(t, b.TopicExists("t"))
		})
	}
}

func TestInMemoryBackend_DeleteTopic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr     error
		name        string
		preexisting bool
	}{
		{name: "existing topic is deleted", preexisting: true, wantErr: nil},
		{name: "missing topic errors", preexisting: false, wantErr: azureservicebus.ErrTopicNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := azureservicebus.NewInMemoryBackend()

			if tt.preexisting {
				_, err := b.CreateTopic("t")
				require.NoError(t, err)
			}

			err := b.DeleteTopic("t")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.False(t, b.TopicExists("t"))
			}
		})
	}
}

func TestInMemoryBackend_TopicExists(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	assert.False(t, b.TopicExists("t"))

	_, err := b.CreateTopic("t")
	require.NoError(t, err)
	assert.True(t, b.TopicExists("t"))
}

// TestInMemoryBackend_DeleteTopic_RemovesSubscriptions covers a scenario that
// doesn't fit the table above: it asserts a structural side effect (a
// subscription disappearing) rather than a single error/state comparison.
func TestInMemoryBackend_DeleteTopic_RemovesSubscriptions(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	_, err := b.CreateTopic("t")
	require.NoError(t, err)
	_, err = b.CreateSubscription("t", "s")
	require.NoError(t, err)

	require.NoError(t, b.DeleteTopic("t"))
	assert.False(t, b.SubscriptionExists("t", "s"))
}

func TestInMemoryBackend_GetTopicInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     azureservicebus.EntityConfig
		wantTTL time.Duration
	}{
		{name: "unconfigured topic reports the package default TTL", wantTTL: azureservicebus.DefaultMessageTTL},
		{
			name: "configured topic reports its own TTL",
			cfg:  azureservicebus.EntityConfig{DefaultMessageTTL: time.Hour}, wantTTL: time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := azureservicebus.NewInMemoryBackend()
			_, err := b.CreateTopic("t", tt.cfg)
			require.NoError(t, err)

			info, err := b.GetTopicInfo("t")
			require.NoError(t, err)
			assert.Equal(t, "t", info.Name)
			assert.Equal(t, tt.wantTTL, info.DefaultMessageTTL)
		})
	}

	t.Run("missing topic errors", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.GetTopicInfo("missing")
		require.ErrorIs(t, err, azureservicebus.ErrTopicNotFound)
	})
}

func TestInMemoryBackend_ListTopics(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	assert.Empty(t, b.ListTopics())

	_, err := b.CreateTopic("b")
	require.NoError(t, err)
	_, err = b.CreateTopic("a")
	require.NoError(t, err)

	infos := b.ListTopics()
	require.Len(t, infos, 2)
	assert.Equal(t, "a", infos[0].Name, "results should be sorted by name")
	assert.Equal(t, "b", infos[1].Name)
}

func TestInMemoryBackend_ListSubscriptions(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	_, err := b.CreateTopic("t")
	require.NoError(t, err)

	_, err = b.CreateSubscription("t", "b")
	require.NoError(t, err)
	_, err = b.CreateSubscription("t", "a")
	require.NoError(t, err)

	infos, err := b.ListSubscriptions("t")
	require.NoError(t, err)
	require.Len(t, infos, 2)
	assert.Equal(t, "a", infos[0].Name, "results should be sorted by name")
	assert.Equal(t, "b", infos[1].Name)

	_, err = b.ListSubscriptions("missing")
	require.ErrorIs(t, err, azureservicebus.ErrTopicNotFound)
}
