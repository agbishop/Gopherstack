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
	_, err := b.CreateQueue("q", azureservicebus.EntityConfig{
		LockDuration: 2 * time.Minute, MaxDeliveryCount: 4, DefaultMessageTTL: time.Hour,
	})
	require.NoError(t, err)
	_, err = b.CreateTopic("t", azureservicebus.EntityConfig{DefaultMessageTTL: 30 * time.Minute})
	require.NoError(t, err)
	_, err = b.CreateSubscription("t", "s", azureservicebus.EntityConfig{
		LockDuration: 90 * time.Second, MaxDeliveryCount: 7,
	})
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

	// The per-entity EntityConfig properties (gap #1) must round-trip too.
	queueInfo, err := restored.GetQueueInfo("q")
	require.NoError(t, err)
	assert.Equal(t, 2*time.Minute, queueInfo.LockDuration)
	assert.Equal(t, 4, queueInfo.MaxDeliveryCount)
	assert.Equal(t, time.Hour, queueInfo.DefaultMessageTTL)

	topicInfo, err := restored.GetTopicInfo("t")
	require.NoError(t, err)
	assert.Equal(t, 30*time.Minute, topicInfo.DefaultMessageTTL)

	subInfo, err := restored.GetSubscriptionInfo("t", "s")
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, subInfo.LockDuration)
	assert.Equal(t, 7, subInfo.MaxDeliveryCount)

	// A restored entity's notify channel must be usable (not nil) --
	// PeekLockWait would otherwise wait on a nil channel forever. Send is
	// enough to exercise it: it calls broadcastLocked, which panics on a nil
	// channel only via close(nil), which broadcastLocked guards against, but
	// a genuinely nil channel would also make PeekLockWait's select block
	// forever; a short-timeout PeekLockWait against the now-empty queue
	// proves the channel is live and selectable rather than a permanently
	// blocking nil.
	_, err = restored.PeekLockWait(t.Context(), azureservicebus.EntityRef{Queue: "q"}, false, time.Minute, 0)
	require.ErrorIs(t, err, azureservicebus.ErrMessageNotFound)
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
