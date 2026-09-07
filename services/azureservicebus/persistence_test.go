package azureservicebus_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

func TestInMemoryBackend_SnapshotRestore(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	_, err := b.CreateQueue("q")
	require.NoError(t, err)
	_, err = b.CreateTopic("t")
	require.NoError(t, err)
	_, err = b.CreateSubscription("t", "s")
	require.NoError(t, err)

	_, err = b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
	require.NoError(t, err)
	_, err = b.Send(
		azureservicebus.EntityRef{Topic: "t", Subscription: "s"}, azureservicebus.NewMessage{Body: []byte("m2")},
	)
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	restored := azureservicebus.NewInMemoryBackend()
	require.NoError(t, restored.Restore(t.Context(), snap))

	assert.True(t, restored.QueueExists("q"))
	assert.True(t, restored.TopicExists("t"))
	assert.True(t, restored.SubscriptionExists("t", "s"))

	info, err := restored.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, []byte("m1"), info.Body)

	info, err = restored.PeekLock(azureservicebus.EntityRef{Topic: "t", Subscription: "s"}, false, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, []byte("m2"), info.Body)
}

func TestInMemoryBackend_Restore_IncompatibleVersionStartsEmpty(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	_, err := b.CreateQueue("q")
	require.NoError(t, err)

	// A snapshot from a hypothetical future/incompatible version: valid JSON,
	// but a version number that will never match azureServiceBusSnapshotVersion.
	err = b.Restore(t.Context(), []byte(`{"version":999999,"queues":{},"topics":{}}`))
	require.NoError(t, err, "an incompatible version must be discarded, not treated as a fatal error")
	assert.False(t, b.QueueExists("q"), "restore should have reset to empty state")
}

func TestInMemoryBackend_Restore_RejectsNullQueue(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	err := b.Restore(t.Context(), []byte(`{"version":1,"queues":{"q":null},"topics":{}}`))
	require.Error(t, err)
}

func TestHandler_SnapshotRestore(t *testing.T) {
	t.Parallel()

	backend := azureservicebus.NewInMemoryBackend()
	h := azureservicebus.NewHandler(backend)

	_, err := backend.CreateQueue("q")
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	other := azureservicebus.NewHandler(azureservicebus.NewInMemoryBackend())
	require.NoError(t, other.Restore(t.Context(), snap))
	assert.True(t, other.Backend.QueueExists("q"))
}
