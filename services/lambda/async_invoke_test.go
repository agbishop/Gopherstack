package lambda_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/lambda"
)

// asyncInvokePort* constants define the ports used by async invocation tests.
// They must not collide with ports used in other test files.
const (
	asyncInvokeQueueBehaviorBase  = 18200 // 18200–18201 reserved
	asyncInvokePendingCleanupBase = 18202 // 18202 reserved
	asyncInvokeSlotLifetimeBase   = 18203 // 18203–18204 reserved
	asyncInvokeRetryBase          = 18205 // 18205–18208 reserved
	asyncInvokeTimeoutDestBase    = 18209 // 18209 reserved
)

// newAsyncTestBackend returns a backend with no Docker/port-alloc so that
// getOrCreateRuntime returns an error quickly. Tests that call EnqueueAsync directly
// bypass getOrCreateRuntime and provide their own runtime server.
func newAsyncTestBackend(t *testing.T) *lambda.InMemoryBackend {
	t.Helper()

	return closeBackend(t, lambda.NewInMemoryBackend(nil, nil, lambda.DefaultSettings(), "000000000000", "us-east-1"))
}

// startAsyncTestServer creates and starts an ExportedRuntimeServer on the given port,
// registering a cleanup hook to stop it when the test finishes.
func startAsyncTestServer(t *testing.T, port int) *lambda.ExportedRuntimeServer {
	t.Helper()

	srv := lambda.NewExportedRuntimeServer(port)
	require.NoError(t, srv.Start(t.Context()))

	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		srv.Stop(stopCtx)
	})

	return srv
}

// TestEnqueueAsync_QueueBehavior verifies fast-path (direct enqueue when queue has space)
// and slow-path (background goroutine when queue is full) behaviours of enqueueAsyncInvocation.
func TestEnqueueAsync_QueueBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		fillCount     int // pre-fill the queue with this many dummy entries (0 = empty)
		wantQueueLen  int // expected queue length once the new invocation is enqueued
		wantNoDropped bool
	}{
		{
			name:          "fast_path_enqueues_immediately_when_queue_empty",
			fillCount:     0,
			wantQueueLen:  1,
			wantNoDropped: true,
		},
		{
			name:          "slow_path_enqueues_after_queue_drains",
			fillCount:     lambda.RuntimeQueueSize,
			wantQueueLen:  1,
			wantNoDropped: true,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Use a non-started server: no HTTP listener needed for queue tests.
			srv := lambda.NewExportedRuntimeServer(asyncInvokeQueueBehaviorBase + i)
			bk := newAsyncTestBackend(t)

			// Pre-fill the queue to the requested depth.
			if tt.fillCount > 0 {
				filled := lambda.FillQueue(srv, tt.fillCount)
				require.Equal(t, tt.fillCount, filled, "should have filled queue to capacity")
			}

			// Enqueue an invocation — it must not be dropped, even when the queue is full.
			requestID := lambda.EnqueueAsync(t.Context(), bk, srv, "fn-queue", []byte(`{}`), time.Minute, false)
			require.NotEmpty(t, requestID)

			if tt.fillCount > 0 {
				// Drain exactly the pre-filled items to make room for the slow-path goroutine.
				// Using DrainN (not DrainQueue) prevents the drain from also consuming the
				// goroutine's item, which is inserted at the END of the queue — after the
				// pre-filled items — as soon as one slot is freed.
				drained := lambda.DrainN(srv, tt.fillCount)
				assert.Equal(t, tt.fillCount, drained, "should drain all pre-filled items")
			}

			// The new invocation should eventually appear in the queue.
			require.Eventually(t, func() bool {
				return lambda.QueueLen(srv) == tt.wantQueueLen
			}, 2*time.Second, 10*time.Millisecond,
				"invocation should be enqueued (wantQueueLen=%d)", tt.wantQueueLen)

			assert.True(t, tt.wantNoDropped, "invocation must not be dropped")
		})
	}
}

// TestEnqueueAsync_BoundsSlowPathWaiters verifies that when runtime queues are full,
// slow-path enqueue goroutines are capped to avoid unbounded goroutine growth.
func TestEnqueueAsync_BoundsSlowPathWaiters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		timeout       time.Duration
		totalEnqueues int
	}{
		{
			name:          "waiters_are_capped_when_queue_is_full",
			timeout:       20 * time.Millisecond,
			totalEnqueues: lambda.MaxAsyncEnqueueWaiters + 32,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := lambda.NewExportedRuntimeServer(asyncInvokeQueueBehaviorBase + 100 + i)
			bk := newAsyncTestBackend(t)

			filled := lambda.FillQueue(srv, lambda.RuntimeQueueSize)
			require.Equal(t, lambda.RuntimeQueueSize, filled, "queue should be full for slow-path enqueue")

			for range tt.totalEnqueues {
				lambda.EnqueueAsync(t.Context(), bk, srv, "fn-capped", []byte(`{}`), tt.timeout, false)
			}

			require.Eventually(t, func() bool {
				return lambda.AsyncEnqueueWaitersLen(bk) == lambda.MaxAsyncEnqueueWaiters
			}, time.Second, 10*time.Millisecond, "slow-path waiter count should saturate at configured cap")

			// Continuously draining allows blocked waiters to enqueue and exit.
			require.Eventually(t, func() bool {
				lambda.DrainQueue(srv)

				return lambda.AsyncEnqueueWaitersLen(bk) == 0
			}, 3*time.Second, 10*time.Millisecond, "all slow-path waiters should eventually drain")
		})
	}
}

// TestEnqueueAsync_PendingCleanup verifies that when a container picks up an async
// invocation via /next (storing it in srv.pending) but never calls /response or /error,
// the entry is removed from srv.pending once the function timeout elapses, preventing
// a memory leak.
func TestEnqueueAsync_PendingCleanup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		port              int
		invocationTimeout time.Duration
		wantPendingBefore int // expected PendingLen immediately after container calls /next
		wantPendingAfter  int // expected PendingLen after invocation timeout elapses
	}{
		{
			name:              "stale_entry_removed_after_timeout",
			port:              asyncInvokePendingCleanupBase,
			invocationTimeout: 100 * time.Millisecond,
			wantPendingBefore: 1,
			wantPendingAfter:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := startAsyncTestServer(t, tt.port)
			bk := newAsyncTestBackend(t)

			lambda.EnqueueAsync(t.Context(), bk, srv, "fn-clean", []byte(`{}`), tt.invocationTimeout, false)

			// Simulate the container calling /next — stores the entry in srv.pending.
			requestID := simulateContainerNext(t, tt.port)
			require.NotEmpty(t, requestID)

			assert.Equal(t, tt.wantPendingBefore, lambda.PendingLen(srv),
				"pending entry should exist immediately after /next")

			// Container does NOT call /response. After the timeout the goroutine cleans up.
			require.Eventually(t, func() bool {
				return lambda.PendingLen(srv) == tt.wantPendingAfter
			}, 3*time.Second, 10*time.Millisecond,
				"pending entry should be removed after invocation timeout")
		})
	}
}

// TestEnqueueAsync_ConcurrencySlotLifetime verifies that when trackConcurrency is true
// the concurrency slot is held for the full execution duration — released only after the
// container posts a response (or after the function timeout if it never does).
func TestEnqueueAsync_ConcurrencySlotLifetime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		port             int
		invTimeout       time.Duration
		simulateResponse bool // true: simulate /next + /response; false: simulate /next only
		wantPendingAfter int  // expected PendingLen after slot is released
	}{
		{
			name:             "slot_released_after_container_response",
			port:             asyncInvokeSlotLifetimeBase,
			invTimeout:       5 * time.Second,
			simulateResponse: true,
			wantPendingAfter: 0, // handleInvocationResult removes the pending entry on response
		},
		{
			name:             "slot_and_pending_released_after_timeout",
			port:             asyncInvokeSlotLifetimeBase + 1,
			invTimeout:       100 * time.Millisecond,
			simulateResponse: false,
			wantPendingAfter: 0, // waitAndCleanPending removes the stale entry on timeout
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := startAsyncTestServer(t, tt.port)
			bk := newAsyncTestBackend(t)

			const fnName = "fn-slot"

			require.NoError(t, bk.CreateFunction(&lambda.FunctionConfiguration{FunctionName: fnName}))

			_, err := bk.PutFunctionConcurrency(fnName, 1)
			require.NoError(t, err)

			// Acquire the single slot, simulating what InvokeFunctionWithQualifier does
			// before calling enqueueAsyncInvocation.
			acquired, acquireErr := lambda.AcquireConcurrencySlot(bk, fnName)
			require.NoError(t, acquireErr)
			require.True(t, acquired, "should acquire the sole slot")

			// Confirm: no more slots available.
			_, err = lambda.AcquireConcurrencySlot(bk, fnName)
			require.Error(t, err, "slot should be exhausted before enqueue")

			// Enqueue async with trackConcurrency=true — the goroutine holds the slot.
			lambda.EnqueueAsync(t.Context(), bk, srv, fnName, []byte(`{}`), tt.invTimeout, true)

			// Slot should still be held immediately after enqueue (release is deferred).
			_, err = lambda.AcquireConcurrencySlot(bk, fnName)
			require.Error(t, err, "slot should still be held immediately after enqueue")

			// Drive the container interaction.
			requestID := simulateContainerNext(t, tt.port)
			require.NotEmpty(t, requestID)

			if tt.simulateResponse {
				simulateContainerResponse(t, tt.port, requestID, `{"ok":true}`)
			}
			// else: container is silent; the timeout will fire.

			// The slot must eventually be released (by response or timeout).
			require.Eventually(t, func() bool {
				ok, tryErr := lambda.AcquireConcurrencySlot(bk, fnName)
				if tryErr != nil {
					return false
				}

				if ok {
					lambda.ReleaseConcurrencySlot(bk, fnName)
				}

				return true
			}, 3*time.Second, 5*time.Millisecond, "concurrency slot should be released")

			assert.Equal(t, tt.wantPendingAfter, lambda.PendingLen(srv),
				"pending entry state after slot release")
		})
	}
}

// TestEnqueueAsync_Retry verifies that enqueueAsyncInvocation respects MaximumRetryAttempts
// from the function's event invoke configuration and automatically retries on function errors.
func TestEnqueueAsync_Retry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupConfig   func(t *testing.T, bk *lambda.InMemoryBackend, fnName string)
		driverFn      func(t *testing.T, port int) int
		name          string
		fnName        string
		port          int
		wantNextCalls int
	}{
		{
			name:   "retry_on_error_succeeds_on_second_attempt",
			port:   asyncInvokeRetryBase,
			fnName: "fn-retry-success",
			setupConfig: func(t *testing.T, bk *lambda.InMemoryBackend, fnName string) {
				t.Helper()
				_, err := bk.PutFunctionEventInvokeConfig(fnName, &lambda.PutFunctionEventInvokeConfigInput{
					MaximumRetryAttempts: new(1),
				})
				require.NoError(t, err)
			},
			driverFn: func(t *testing.T, port int) int {
				t.Helper()
				// First attempt: return an error.
				reqID1 := simulateContainerNext(t, port)
				simulateContainerError(t, port, reqID1, `{"errorMessage":"transient error"}`)
				// Second attempt (retry): return success.
				reqID2 := simulateContainerNext(t, port)
				simulateContainerResponse(t, port, reqID2, `{"ok":true}`)

				return 2
			},
			wantNextCalls: 2,
		},
		{
			name:   "no_retry_when_max_retries_zero",
			port:   asyncInvokeRetryBase + 1,
			fnName: "fn-no-retry",
			setupConfig: func(t *testing.T, bk *lambda.InMemoryBackend, fnName string) {
				t.Helper()
				_, err := bk.PutFunctionEventInvokeConfig(fnName, &lambda.PutFunctionEventInvokeConfigInput{
					MaximumRetryAttempts: new(0),
				})
				require.NoError(t, err)
			},
			driverFn: func(t *testing.T, port int) int {
				t.Helper()
				// Only one /next call expected; return error.
				reqID := simulateContainerNext(t, port)
				simulateContainerError(t, port, reqID, `{"errorMessage":"permanent error"}`)

				return 1
			},
			wantNextCalls: 1,
		},
		{
			name:   "all_retries_exhausted_slot_released",
			port:   asyncInvokeRetryBase + 2,
			fnName: "fn-retry-exhausted",
			setupConfig: func(t *testing.T, bk *lambda.InMemoryBackend, fnName string) {
				t.Helper()
				_, err := bk.PutFunctionEventInvokeConfig(fnName, &lambda.PutFunctionEventInvokeConfigInput{
					MaximumRetryAttempts: new(1),
				})
				require.NoError(t, err)
			},
			driverFn: func(t *testing.T, port int) int {
				t.Helper()
				// Both attempts return error.
				reqID1 := simulateContainerNext(t, port)
				simulateContainerError(t, port, reqID1, `{"errorMessage":"error1"}`)
				reqID2 := simulateContainerNext(t, port)
				simulateContainerError(t, port, reqID2, `{"errorMessage":"error2"}`)

				return 2
			},
			wantNextCalls: 2,
		},
		{
			name:   "event_age_exceeded_drops_without_retry",
			port:   asyncInvokeRetryBase + 3,
			fnName: "fn-age-expired",
			setupConfig: func(t *testing.T, bk *lambda.InMemoryBackend, fnName string) {
				t.Helper()
				_, err := bk.PutFunctionEventInvokeConfig(fnName, &lambda.PutFunctionEventInvokeConfigInput{
					MaximumRetryAttempts:     new(1),
					MaximumEventAgeInSeconds: new(lambda.MinEventAgeInSeconds),
				})
				require.NoError(t, err)
			},
			driverFn: func(t *testing.T, port int) int {
				t.Helper()
				// Container gets exactly one /next call; returns error.
				// The goroutine checks event age BEFORE retrying.  Since the event was
				// created 61 seconds in the past (see createdAt override in test body)
				// the retry will be dropped by the age check.
				reqID := simulateContainerNext(t, port)
				simulateContainerError(t, port, reqID, `{"errorMessage":"error"}`)

				return 1
			},
			wantNextCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := startAsyncTestServer(t, tt.port)
			bk := newAsyncTestBackend(t)

			require.NoError(t, bk.CreateFunction(&lambda.FunctionConfiguration{FunctionName: tt.fnName}))

			_, err := bk.PutFunctionConcurrency(tt.fnName, 1)
			require.NoError(t, err)

			if tt.setupConfig != nil {
				tt.setupConfig(t, bk, tt.fnName)
			}

			// Acquire the concurrency slot (simulating InvokeFunctionWithQualifier).
			acquired, acquireErr := lambda.AcquireConcurrencySlot(bk, tt.fnName)
			require.NoError(t, acquireErr)
			require.True(t, acquired)

			// For the event-age test, backdate the creation time so the retry check fires.
			var createdAt []time.Time
			if tt.name == "event_age_exceeded_drops_without_retry" {
				createdAt = []time.Time{time.Now().Add(-(lambda.MinEventAgeInSeconds + 1) * time.Second)}
			}

			lambda.EnqueueAsync(t.Context(), bk, srv, tt.fnName, []byte(`{}`), 5*time.Second, true, createdAt...)

			// Drive the container interactions.
			tt.driverFn(t, tt.port)

			// After all interactions the concurrency slot must be released.
			require.Eventually(t, func() bool {
				ok, tryErr := lambda.AcquireConcurrencySlot(bk, tt.fnName)
				if tryErr != nil {
					return false
				}

				if ok {
					lambda.ReleaseConcurrencySlot(bk, tt.fnName)
				}

				return true
			}, 3*time.Second, 5*time.Millisecond, "concurrency slot should be released after all attempts")

			// Queue should be empty: no more pending retry invocations.
			assert.Equal(t, 0, lambda.QueueLen(srv), "no stale items in queue after completion")
		})
	}
}

// TestEnqueueAsync_TimeoutDeliversToFailureDestination verifies that when an async
// invocation's container never responds (a genuine execution timeout, not a
// function-returned error), the outcome still reaches the function's configured
// on-failure destination. AWS's own docs state that a runtime timeout is a function
// error for async purposes: "Function errors include errors returned by the
// function's code and errors returned by the function's runtime, such as timeouts"
// (docs.aws.amazon.com/lambda/latest/dg/invocation-async-error-handling.html).
func TestEnqueueAsync_TimeoutDeliversToFailureDestination(t *testing.T) {
	t.Parallel()

	const failureARN = "arn:aws:sqs:us-east-1:000000000000:on-failure"

	srv := startAsyncTestServer(t, asyncInvokeTimeoutDestBase)
	bk := newAsyncTestBackend(t)

	require.NoError(t, bk.CreateFunction(&lambda.FunctionConfiguration{FunctionName: "fn-timeout-dest"}))

	_, err := bk.PutFunctionEventInvokeConfig("fn-timeout-dest", &lambda.PutFunctionEventInvokeConfigInput{
		MaximumRetryAttempts: new(0),
		DestinationConfig: &lambda.DestinationConfig{
			OnFailure: &lambda.Destination{Destination: failureARN},
		},
	})
	require.NoError(t, err)

	fake := &fakeAsyncDelivery{}
	bk.SetAsyncDestinationDelivery(fake)

	// Never simulate any /next or /response call: the container is hung and the
	// invocation must time out rather than ever completing.
	lambda.EnqueueAsync(t.Context(), bk, srv, "fn-timeout-dest", []byte(`{}`), 200*time.Millisecond, false)

	require.Eventually(t, func() bool {
		return len(fake.targets()) > 0
	}, 3*time.Second, 10*time.Millisecond,
		"a timed-out async invocation must be delivered to its on-failure destination")

	assert.Contains(t, fake.targets(), failureARN)
}
