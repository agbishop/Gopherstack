package azureservicebus_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

// newQueueBackend returns a fresh backend with one pre-created queue named
// "q" -- every caller in this file uses that fixed name.
func newQueueBackend(t *testing.T) *azureservicebus.InMemoryBackend {
	t.Helper()

	b := azureservicebus.NewInMemoryBackend()
	_, err := b.CreateQueue("q")
	require.NoError(t, err)

	return b
}

func TestInMemoryBackend_Send(t *testing.T) {
	t.Parallel()

	t.Run("queue", func(t *testing.T) {
		t.Parallel()

		b := newQueueBackend(t)
		info, err := b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("hello")})
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), info.Body)
		assert.NotZero(t, info.SequenceNumber)
	})

	t.Run("missing queue errors", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.Send(azureservicebus.EntityRef{Queue: "missing"}, azureservicebus.NewMessage{})
		require.ErrorIs(t, err, azureservicebus.ErrQueueNotFound)
	})

	t.Run("topic fans out to every subscription", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.CreateTopic("t")
		require.NoError(t, err)
		_, err = b.CreateSubscription("t", "s1")
		require.NoError(t, err)
		_, err = b.CreateSubscription("t", "s2")
		require.NoError(t, err)

		_, err = b.Send(azureservicebus.EntityRef{Topic: "t"}, azureservicebus.NewMessage{Body: []byte("fanout")})
		require.NoError(t, err)

		for _, sub := range []string{"s1", "s2"} {
			info, peekErr := b.PeekLock(
				azureservicebus.EntityRef{Topic: "t", Subscription: sub}, false, time.Minute,
			)
			require.NoError(t, peekErr, "subscription %s should have received a copy", sub)
			assert.Equal(t, []byte("fanout"), info.Body)
		}
	})

	t.Run("topic with no subscriptions is not an error", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.CreateTopic("t")
		require.NoError(t, err)

		_, err = b.Send(azureservicebus.EntityRef{Topic: "t"}, azureservicebus.NewMessage{Body: []byte("x")})
		require.NoError(t, err)
	})

	t.Run("missing topic errors", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.Send(azureservicebus.EntityRef{Topic: "missing"}, azureservicebus.NewMessage{})
		require.ErrorIs(t, err, azureservicebus.ErrTopicNotFound)
	})
}

func TestInMemoryBackend_PeekLock(t *testing.T) {
	t.Parallel()

	t.Run("locks the oldest available message", func(t *testing.T) {
		t.Parallel()

		b := newQueueBackend(t)
		_, err := b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
		require.NoError(t, err)

		info, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, []byte("m1"), info.Body)
		assert.NotEmpty(t, info.LockToken)
		assert.EqualValues(t, 1, info.DeliveryCount)
	})

	t.Run("locked message is not returned again", func(t *testing.T) {
		t.Parallel()

		b := newQueueBackend(t)
		_, err := b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
		require.NoError(t, err)

		_, err = b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.NoError(t, err)

		_, err = b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.ErrorIs(t, err, azureservicebus.ErrMessageNotFound)
	})

	t.Run("empty queue returns ErrMessageNotFound", func(t *testing.T) {
		t.Parallel()

		b := newQueueBackend(t)
		_, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.ErrorIs(t, err, azureservicebus.ErrMessageNotFound)
	})

	t.Run("missing queue errors", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.PeekLock(azureservicebus.EntityRef{Queue: "missing"}, false, time.Minute)
		require.ErrorIs(t, err, azureservicebus.ErrQueueNotFound)
	})
}

func TestInMemoryBackend_Complete(t *testing.T) {
	t.Parallel()

	t.Run("removes a correctly-locked message", func(t *testing.T) {
		t.Parallel()

		b := newQueueBackend(t)
		_, err := b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
		require.NoError(t, err)

		info, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.NoError(t, err)

		require.NoError(t, b.Complete(azureservicebus.EntityRef{Queue: "q"}, false, info.MessageID, info.LockToken))

		_, err = b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.ErrorIs(t, err, azureservicebus.ErrMessageNotFound, "completed message should be gone")
	})

	t.Run("wrong lock token is rejected", func(t *testing.T) {
		t.Parallel()

		b := newQueueBackend(t)
		_, err := b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
		require.NoError(t, err)

		info, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.NoError(t, err)

		err = b.Complete(azureservicebus.EntityRef{Queue: "q"}, false, info.MessageID, "wrong-token")
		require.ErrorIs(t, err, azureservicebus.ErrLockTokenMismatch)
	})

	t.Run("unlocked message cannot be completed", func(t *testing.T) {
		t.Parallel()

		b := newQueueBackend(t)
		_, err := b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
		require.NoError(t, err)

		err = b.Complete(azureservicebus.EntityRef{Queue: "q"}, false, "does-not-matter", "does-not-matter")
		require.ErrorIs(t, err, azureservicebus.ErrMessageNotFound)
	})

	t.Run("missing queue errors", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		err := b.Complete(azureservicebus.EntityRef{Queue: "missing"}, false, "id", "token")
		require.ErrorIs(t, err, azureservicebus.ErrQueueNotFound)
	})
}

func TestInMemoryBackend_Abandon(t *testing.T) {
	t.Parallel()

	t.Run("releases the lock for redelivery", func(t *testing.T) {
		t.Parallel()

		b := newQueueBackend(t)
		_, err := b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
		require.NoError(t, err)

		info, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.NoError(t, err)

		require.NoError(t, b.Abandon(azureservicebus.EntityRef{Queue: "q"}, false, info.MessageID, info.LockToken))

		redelivered, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.NoError(t, err, "abandoned message should be immediately available again")
		assert.Equal(t, info.MessageID, redelivered.MessageID)
		assert.EqualValues(t, 2, redelivered.DeliveryCount)
	})

	t.Run("wrong lock token is rejected", func(t *testing.T) {
		t.Parallel()

		b := newQueueBackend(t)
		_, err := b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
		require.NoError(t, err)

		info, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.NoError(t, err)

		err = b.Abandon(azureservicebus.EntityRef{Queue: "q"}, false, info.MessageID, "wrong-token")
		require.ErrorIs(t, err, azureservicebus.ErrLockTokenMismatch)
	})

	t.Run("dead-letters once MaxDeliveryCount is exhausted", func(t *testing.T) {
		t.Parallel()

		b := newQueueBackend(t)
		_, err := b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
		require.NoError(t, err)

		var messageID string

		for i := range azureservicebus.MaxDeliveryCount {
			info, peekErr := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
			require.NoError(t, peekErr, "delivery attempt %d", i+1)
			messageID = info.MessageID

			require.NoError(t, b.Abandon(azureservicebus.EntityRef{Queue: "q"}, false, info.MessageID, info.LockToken))
		}

		_, err = b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.ErrorIs(t, err, azureservicebus.ErrMessageNotFound, "message should have been dead-lettered")

		dl, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, true, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, messageID, dl.MessageID)
	})

	t.Run("missing queue errors", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		err := b.Abandon(azureservicebus.EntityRef{Queue: "missing"}, false, "id", "token")
		require.ErrorIs(t, err, azureservicebus.ErrQueueNotFound)
	})
}
