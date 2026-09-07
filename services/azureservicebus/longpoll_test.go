package azureservicebus_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureservicebus"
)

// TestInMemoryBackend_PeekLockWait covers PeekLock's long-poll variant:
// returning promptly on message arrival, on timeout elapsing, and on context
// cancellation, all under testing/synctest's virtual clock so the test is
// both deterministic and fast. Each subtest's goroutine is required to have
// exited by the time synctest.Test's function returns (synctest.Test itself
// enforces this, failing the test on a durable leak), which is this test's
// proof that PeekLockWait does not leak goroutines.
func TestInMemoryBackend_PeekLockWait(t *testing.T) {
	t.Parallel()

	t.Run("returns immediately when a message is already available", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := azureservicebus.NewInMemoryBackend()
			_, err := b.CreateQueue("q")
			require.NoError(t, err)

			_, err = b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
			require.NoError(t, err)

			info, err := b.PeekLockWait(
				t.Context(),
				azureservicebus.EntityRef{Queue: "q"},
				false,
				time.Minute,
				5*time.Second,
			)
			require.NoError(t, err)
			assert.Equal(t, []byte("m1"), info.Body)
		})
	})

	t.Run("waits and returns once a message is sent", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := azureservicebus.NewInMemoryBackend()
			_, err := b.CreateQueue("q")
			require.NoError(t, err)

			type result struct {
				err  error
				info azureservicebus.MessageInfo
			}

			resultCh := make(chan result, 1)

			go func() {
				gotInfo, gotErr := b.PeekLockWait(
					t.Context(), azureservicebus.EntityRef{Queue: "q"}, false, time.Minute, 10*time.Second,
				)
				resultCh <- result{gotErr, gotInfo}
			}()

			// Let the goroutine block on its first empty read.
			synctest.Wait()

			_, err = b.Send(azureservicebus.EntityRef{Queue: "q"}, azureservicebus.NewMessage{Body: []byte("m1")})
			require.NoError(t, err)

			synctest.Wait()

			select {
			case res := <-resultCh:
				require.NoError(t, res.err)
				assert.Equal(t, []byte("m1"), res.info.Body)
			default:
				t.Fatal("PeekLockWait did not return promptly after Send")
			}
		})
	})

	t.Run("returns ErrMessageNotFound once the timeout elapses", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := azureservicebus.NewInMemoryBackend()
			_, err := b.CreateQueue("q")
			require.NoError(t, err)

			start := time.Now()

			_, err = b.PeekLockWait(
				t.Context(),
				azureservicebus.EntityRef{Queue: "q"},
				false,
				time.Minute,
				3*time.Second,
			)
			require.ErrorIs(t, err, azureservicebus.ErrMessageNotFound)
			assert.GreaterOrEqual(t, time.Since(start), 3*time.Second)
		})
	})

	t.Run("returns promptly when the context is cancelled", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := azureservicebus.NewInMemoryBackend()
			_, err := b.CreateQueue("q")
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(t.Context())

			type result struct{ err error }

			resultCh := make(chan result, 1)

			go func() {
				_, gotErr := b.PeekLockWait(
					ctx,
					azureservicebus.EntityRef{Queue: "q"},
					false,
					time.Minute,
					30*time.Second,
				)
				resultCh <- result{gotErr}
			}()

			synctest.Wait()

			cancel()

			synctest.Wait()

			select {
			case res := <-resultCh:
				require.ErrorIs(t, res.err, azureservicebus.ErrMessageNotFound)
			default:
				t.Fatal("PeekLockWait did not return promptly after context cancellation")
			}
		})
	})

	t.Run("missing queue errors without waiting", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := azureservicebus.NewInMemoryBackend()

			_, err := b.PeekLockWait(
				t.Context(), azureservicebus.EntityRef{Queue: "missing"}, false, time.Minute, 5*time.Second,
			)
			require.ErrorIs(t, err, azureservicebus.ErrQueueNotFound)
		})
	})
}
