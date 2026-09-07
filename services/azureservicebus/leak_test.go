package azureservicebus_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

// TestJanitor_SweepOnce_ReleasesExpiredLocksAndDeadLettersTTLExpired verifies
// the two things the Janitor sweep is responsible for: releasing a message
// whose peek-lock has expired without being Completed or Abandoned (so it
// doesn't leak as permanently unavailable), and moving a message whose TTL
// has elapsed into the dead-letter sub-queue (so it doesn't leak as live
// forever). Mirrors services/ssm's leak_test.go naming/shape for a janitor
// sweep test, adapted to azureservicebus's own SweepOnce/nowFunc seam.
func TestJanitor_SweepOnce_ReleasesExpiredLocksAndDeadLettersTTLExpired(t *testing.T) {
	t.Parallel()

	t.Run("expired lock is released for redelivery", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.CreateQueue("q")
		require.NoError(t, err)

		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		azureservicebus.SetNowFunc(b, func() time.Time { return now })

		_, err = b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{
			Body: []byte("m1"), TimeToLive: time.Hour,
		})
		require.NoError(t, err)

		info, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Second)
		require.NoError(t, err)

		// Advance time past the lock's expiry but well within the message TTL.
		now = now.Add(2 * time.Second)

		stats := azureservicebus.SweepOnce(b, now)
		assert.Equal(t, 1, stats.Unlocked)
		assert.Equal(t, 0, stats.DeadLettered)
		assert.Equal(t, 0, stats.Expired)

		redelivered, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.NoError(t, err, "message should be available again after its lock expired")
		assert.Equal(t, info.MessageID, redelivered.MessageID)
	})

	t.Run("lock expiry past MaxDeliveryCount dead-letters instead of releasing", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.CreateQueue("q")
		require.NoError(t, err)

		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		azureservicebus.SetNowFunc(b, func() time.Time { return now })

		_, err = b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
		require.NoError(t, err)

		var messageID string

		for i := range azureservicebus.MaxDeliveryCount {
			info, peekErr := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Second)
			require.NoError(t, peekErr, "delivery attempt %d", i+1)
			messageID = info.MessageID

			now = now.Add(2 * time.Second)
			azureservicebus.SweepOnce(b, now)
		}

		_, err = b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.ErrorIs(t, err, azureservicebus.ErrMessageNotFound,
			"message should have been dead-lettered, not released")

		dl, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, true, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, messageID, dl.MessageID)
	})

	t.Run("TTL-expired live message is moved to dead-letter", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.CreateQueue("q")
		require.NoError(t, err)

		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		azureservicebus.SetNowFunc(b, func() time.Time { return now })

		_, err = b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{
			Body: []byte("m1"), TimeToLive: time.Second,
		})
		require.NoError(t, err)

		now = now.Add(2 * time.Second)

		stats := azureservicebus.SweepOnce(b, now)
		assert.Equal(t, 0, stats.Unlocked)
		assert.Equal(t, 0, stats.DeadLettered)
		assert.Equal(t, 1, stats.Expired)

		_, err = b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, false, time.Minute)
		require.ErrorIs(t, err, azureservicebus.ErrMessageNotFound)

		dl, err := b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, true, time.Minute)
		require.NoError(t, err)
		assert.Equal(t, []byte("m1"), dl.Body)
	})

	t.Run("dead-lettered message that itself expires is dropped, not re-dead-lettered", func(t *testing.T) {
		t.Parallel()

		b := azureservicebus.NewInMemoryBackend()
		_, err := b.CreateQueue("q")
		require.NoError(t, err)

		now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		azureservicebus.SetNowFunc(b, func() time.Time { return now })

		_, err = b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{
			Body: []byte("m1"), TimeToLive: time.Second,
		})
		require.NoError(t, err)

		now = now.Add(2 * time.Second)
		firstSweep := azureservicebus.SweepOnce(b, now)
		require.Equal(t, 1, firstSweep.Expired)

		// A far-future sweep (past the fresh TTL window dead-lettering grants
		// a message, see janitor.go's sweepMessageQueueLocked) should find
		// nothing left to clean up: the dead-lettered message is simply
		// dropped, not endlessly re-processed.
		now = now.Add(azureservicebus.DefaultMessageTTL + time.Hour)
		secondSweep := azureservicebus.SweepOnce(b, now)
		assert.Equal(t, 0, secondSweep.Unlocked)
		assert.Equal(t, 0, secondSweep.DeadLettered)
		assert.Equal(t, 0, secondSweep.Expired)

		_, err = b.PeekLock(azureservicebus.EntityRef{Queue: "q"}, true, time.Minute)
		require.ErrorIs(t, err, azureservicebus.ErrMessageNotFound)
	})
}

// TestJanitor_Run_StartsAndStopsWithoutLeaking verifies the background
// goroutine started by Janitor.Run exits promptly when its context is
// cancelled, mirroring services/azurequeue's identical StartWorker/Janitor
// lifecycle test shape.
func TestJanitor_Run_StartsAndStopsWithoutLeaking(t *testing.T) {
	t.Parallel()

	b := azureservicebus.NewInMemoryBackend()
	j := azureservicebus.NewJanitor(b, time.Millisecond)

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})
	go func() {
		j.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Janitor.Run did not stop after context cancellation")
	}
}
