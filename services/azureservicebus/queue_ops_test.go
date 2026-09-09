package azureservicebus_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

func TestInMemoryBackend_CreateQueue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		preexisting bool
		wantCreated bool
	}{
		{name: "new queue is created", preexisting: false, wantCreated: true},
		{name: "pre-existing queue is idempotent", preexisting: true, wantCreated: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := azureservicebus.NewInMemoryBackend()

			if tt.preexisting {
				_, err := b.CreateQueue("q")
				require.NoError(t, err)
			}

			created, err := b.CreateQueue("q")
			require.NoError(t, err)
			assert.Equal(t, tt.wantCreated, created)
			assert.True(t, b.QueueExists("q"))
		})
	}
}

func TestInMemoryBackend_DeleteQueue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr     error
		name        string
		preexisting bool
	}{
		{name: "existing queue is deleted", preexisting: true, wantErr: nil},
		{name: "missing queue errors", preexisting: false, wantErr: azureservicebus.ErrQueueNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := azureservicebus.NewInMemoryBackend()

			if tt.preexisting {
				_, err := b.CreateQueue("q")
				require.NoError(t, err)
			}

			err := b.DeleteQueue("q")

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.False(t, b.QueueExists("q"))
			}
		})
	}
}

func TestInMemoryBackend_QueueExists(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	assert.False(t, b.QueueExists("q"))

	_, err := b.CreateQueue("q")
	require.NoError(t, err)
	assert.True(t, b.QueueExists("q"))
}

func TestInMemoryBackend_GetQueueInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        azureservicebus.EntityConfig
		wantConfig azureservicebus.EntityConfig
	}{
		{
			name: "unconfigured queue reports package defaults",
			cfg:  azureservicebus.EntityConfig{},
			wantConfig: azureservicebus.EntityConfig{
				LockDuration:      azureservicebus.DefaultLockDuration,
				MaxDeliveryCount:  azureservicebus.MaxDeliveryCount,
				DefaultMessageTTL: azureservicebus.DefaultMessageTTL,
			},
		},
		{
			name: "configured queue reports its own values",
			cfg: azureservicebus.EntityConfig{
				LockDuration: time.Minute, MaxDeliveryCount: 3, DefaultMessageTTL: time.Hour,
			},
			wantConfig: azureservicebus.EntityConfig{
				LockDuration: time.Minute, MaxDeliveryCount: 3, DefaultMessageTTL: time.Hour,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := azureservicebus.NewInMemoryBackend()
			_, err := b.CreateQueue("q", tt.cfg)
			require.NoError(t, err)

			info, err := b.GetQueueInfo("q")
			require.NoError(t, err)
			assert.Equal(t, "q", info.Name)
			assert.Equal(t, tt.wantConfig.LockDuration, info.LockDuration)
			assert.Equal(t, tt.wantConfig.MaxDeliveryCount, info.MaxDeliveryCount)
			assert.Equal(t, tt.wantConfig.DefaultMessageTTL, info.DefaultMessageTTL)
		})
	}

	t.Run("missing queue errors", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.GetQueueInfo("missing")
		require.ErrorIs(t, err, azureservicebus.ErrQueueNotFound)
	})
}

func TestInMemoryBackend_ListQueues(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	assert.Empty(t, b.ListQueues())

	_, err := b.CreateQueue("b")
	require.NoError(t, err)
	_, err = b.CreateQueue("a")
	require.NoError(t, err)

	infos := b.ListQueues()
	require.Len(t, infos, 2)
	assert.Equal(t, "a", infos[0].Name, "results should be sorted by name")
	assert.Equal(t, "b", infos[1].Name)
}
