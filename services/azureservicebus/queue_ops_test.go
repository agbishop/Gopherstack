package azureservicebus_test

import (
	"testing"

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
