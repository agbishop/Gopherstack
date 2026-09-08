package stepfunctions_test

import (
	"context"
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sfn "github.com/blackbirdworks/gopherstack/services/stepfunctions"
)

const passDefinitionForLeak = `{
"StartAt": "P",
"States": {
"P": {"Type": "Pass", "End": true}
}
}`

// TestDeleteActivity_ClosesChannelAndEvictsTokens verifies that deleting an
// activity closes its pending task queue channel and removes all associated
// task tokens from tasksByToken, preventing goroutine leaks when the janitor
// is disabled.
func TestDeleteActivity_ClosesChannelAndEvictsTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		activities int
	}{
		{name: "single activity", activities: 1},
		{name: "multiple activities", activities: 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			b := sfn.NewInMemoryBackendWithContext(ctx, "123456789012", "us-east-1")

			arns := make([]string, 0, tc.activities)
			for i := range tc.activities {
				act, err := b.CreateActivity(ctx, fmt.Sprintf("activity-%d", i))
				require.NoError(t, err)
				arns = append(arns, act.ActivityArn)
			}

			// Verify queues exist before delete.
			for _, arn := range arns {
				require.GreaterOrEqual(t, b.PendingTaskQueueLenForTest(arn), 0,
					"task queue must exist before delete")
			}

			for _, arn := range arns {
				require.NoError(t, b.DeleteActivity(arn))
			}

			// After delete, queues must not exist.
			for _, arn := range arns {
				require.Equal(t, -1, b.PendingTaskQueueLenForTest(arn),
					"task queue must be removed after DeleteActivity")
			}
		})
	}
}

// TestStartExecution_InlinePrunesOldExecutions verifies that StartExecution
// opportunistically removes finished executions that have aged past the
// retention period when the execution count exceeds the inline-sweep threshold,
// keeping the executions and history maps bounded even without the janitor.
func TestStartExecution_InlinePrunesOldExecutions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := sfn.NewInMemoryBackendWithContext(ctx, "123456789012", "us-east-1")

	const roleARN = "arn:aws:iam::123456789012:role/r"
	sm, err := b.CreateStateMachine(ctx, "pruner-sm", passDefinitionForLeak, roleARN, "STANDARD")
	require.NoError(t, err)

	threshold := sfn.ExecutionPruneSweepThresholdForTest

	// Create enough executions to reach the threshold and mark them all as
	// SUCCEEDED with a StopDate well in the past so they qualify for pruning.
	pastStop := float64(time.Now().Add(-48 * time.Hour).Unix())

	var startErr error
	var startExec *sfn.Execution
	for i := range threshold {
		startExec, startErr = b.StartExecution(sm.StateMachineArn, fmt.Sprintf("exec-%d", i), `{}`)
		require.NoError(t, startErr)
		b.SetExecutionStopDateForTest(startExec.ExecutionArn, pastStop)
	}

	require.Equal(t, threshold, b.ExecutionCount(),
		"execution count must equal threshold before the sweep-triggering start")

	// One more StartExecution must trigger the inline prune because
	// len(b.executions) >= executionPruneSweepThreshold.
	_, err = b.StartExecution(sm.StateMachineArn, "trigger-exec", `{}`)
	require.NoError(t, err)

	// All threshold executions were expired; only "trigger-exec" survives.
	require.Equal(t, 1, b.ExecutionCount(),
		"expired executions must be pruned inline on StartExecution")
}

// TestStartExecution_BelowThresholdNoEagerPrune confirms that inline pruning
// is a no-op below the threshold so steady-state starts stay cheap.
func TestStartExecution_BelowThresholdNoEagerPrune(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := sfn.NewInMemoryBackendWithContext(ctx, "123456789012", "us-east-1")

	sm, err := b.CreateStateMachine(
		ctx, "no-prune-sm", passDefinitionForLeak, "arn:aws:iam::123456789012:role/r", "STANDARD",
	)
	require.NoError(t, err)

	pastStop := float64(time.Now().Add(-48 * time.Hour).Unix())
	belowThreshold := sfn.ExecutionPruneSweepThresholdForTest - 1

	var btErr error
	var btExec *sfn.Execution
	for i := range belowThreshold {
		btExec, btErr = b.StartExecution(sm.StateMachineArn, fmt.Sprintf("old-exec-%d", i), `{}`)
		require.NoError(t, btErr)
		b.SetExecutionStopDateForTest(btExec.ExecutionArn, pastStop)
	}

	// Below threshold: no eager prune, so expired entries remain.
	require.Equal(t, belowThreshold, b.ExecutionCount(),
		"below-threshold start must not eagerly prune expired executions")
}

// TestDeleteActivity_UnblocksInFlightInvokeActivity verifies that deleting an
// activity while InvokeActivity is blocked on a worker response signals the
// resultCh, preventing the goroutine from leaking indefinitely.
func TestDeleteActivity_UnblocksInFlightInvokeActivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		tasks int
	}{
		{name: "single in-flight task", tasks: 1},
		{name: "multiple in-flight tasks", tasks: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			b := sfn.NewInMemoryBackendWithContext(ctx, "123456789012", "us-east-1")

			act, err := b.CreateActivity(ctx, "test-activity")
			require.NoError(t, err)

			done := make(chan error, tc.tasks)

			// Spin up InvokeActivity goroutines; each blocks waiting for a worker.
			for range tc.tasks {
				go func() {
					_, invokeErr := b.InvokeActivity(ctx, act.ActivityArn, `{}`, 0)
					done <- invokeErr
				}()
			}

			// Drain the queue so every goroutine is past the send and blocking on resultCh.
			for range tc.tasks {
				task, getErr := b.GetActivityTask(ctx, act.ActivityArn, "worker")
				require.NoError(t, getErr)
				require.NotEmpty(t, task.TaskToken, "GetActivityTask must return a token")
			}

			// Verify all tokens are registered.
			require.Equal(t, tc.tasks, b.TaskTokenCount())

			// Delete the activity: must signal all blocked goroutines.
			require.NoError(t, b.DeleteActivity(act.ActivityArn))

			// All goroutines must unblock within a short window.
			deadline := time.After(2 * time.Second)
			for range tc.tasks {
				select {
				case invokeErr := <-done:
					require.Error(t, invokeErr, "InvokeActivity must return an error after activity deletion")
				case <-deadline:
					t.Fatal("InvokeActivity goroutine did not unblock after DeleteActivity")
				}
			}

			// Tokens must be cleaned up.
			require.Equal(t, 0, b.TaskTokenCount(),
				"task tokens must be evicted after DeleteActivity")
		})
	}
}

// TestSweepTaskTokens_EvictsStaleTokens verifies that SweepTaskTokens removes
// task tokens older than the TTL and unblocks the waiting InvokeActivity goroutines,
// bounding tasksByToken growth when workers never respond.
func TestSweepTaskTokens_EvictsStaleTokens(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := sfn.NewInMemoryBackendWithContext(ctx, "123456789012", "us-east-1")

	act, err := b.CreateActivity(ctx, "sweep-activity")
	require.NoError(t, err)

	const taskCount = 4

	done := make(chan error, taskCount)
	for range taskCount {
		go func() {
			_, invokeErr := b.InvokeActivity(ctx, act.ActivityArn, `{}`, 0)
			done <- invokeErr
		}()
	}

	// Drain queue so goroutines block on resultCh.
	for range taskCount {
		task, getErr := b.GetActivityTask(ctx, act.ActivityArn, "worker")
		require.NoError(t, getErr)
		require.NotEmpty(t, task.TaskToken)
	}

	require.Equal(t, taskCount, b.TaskTokenCount(), "all tokens must be registered")

	// Sweep with default TTL — tokens are fresh, nothing evicted.
	evicted := b.SweepTaskTokens()
	require.Equal(t, 0, evicted, "fresh tokens must not be evicted")
	require.Equal(t, taskCount, b.TaskTokenCount())

	// Age the tokens past the TTL, then sweep again.
	b.AgeTaskTokensForTest(sfn.DefaultTaskTokenTTLForTest + time.Second)
	evicted = b.SweepTaskTokens()
	require.Equal(t, taskCount, evicted, "all stale tokens must be evicted")
	require.Equal(t, 0, b.TaskTokenCount(), "tasksByToken must be empty after sweep")

	// All blocked goroutines must have been unblocked.
	deadline := time.After(2 * time.Second)
	for range taskCount {
		select {
		case invokeErr := <-done:
			require.Error(t, invokeErr, "InvokeActivity must return an error after token eviction")
		case <-deadline:
			t.Fatal("InvokeActivity goroutine did not unblock after SweepTaskTokens")
		}
	}
}

// TestSweepTaskTokens_ReapsWaitForTaskTokenEntries verifies that a
// .waitForTaskToken callback registered via WaitForTaskToken (not
// InvokeActivity) is also bounded by the TaskTokenTTL janitor sweep, the
// same as an activity task token, rather than leaking forever when no
// SendTaskSuccess/SendTaskFailure ever arrives. Uses synctest so the TTL
// wait is real elapsed (fake) time, not a manually backdated createdAt --
// AgeTaskTokensForTest's unconditional Add(-d) would mask a bug where
// createdAt was never set to a real time in the first place.
func TestSweepTaskTokens_ReapsWaitForTaskTokenEntries(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		b := sfn.NewInMemoryBackendWithContext(ctx, "123456789012", "us-east-1")
		b.SetSettings(sfn.Settings{TaskTokenTTL: 10 * time.Millisecond})

		done := make(chan error, 1)
		go func() {
			_, waitErr := b.WaitForTaskToken(ctx, "orphan-token", 0)
			done <- waitErr
		}()

		synctest.Wait()
		require.Equal(t, 1, b.TaskTokenCount(), "WaitForTaskToken must register its token")

		evicted := b.SweepTaskTokens()
		require.Equal(t, 0, evicted, "fresh tokens must not be evicted")

		time.Sleep(11 * time.Millisecond)
		evicted = b.SweepTaskTokens()
		require.Equal(t, 1, evicted, "a stale .waitForTaskToken entry must be evicted like an activity token")
		require.Equal(t, 0, b.TaskTokenCount(), "tasksByToken must be empty after sweep")

		synctest.Wait()

		select {
		case waitErr := <-done:
			require.Error(t, waitErr, "WaitForTaskToken must return an error after token eviction")
		default:
			t.Fatal("WaitForTaskToken goroutine did not unblock after SweepTaskTokens")
		}
	})
}

// TestSendTaskHeartbeat_RenewsTaskTokenTTL verifies that a callback task
// (registered via WaitForTaskToken) which keeps calling SendTaskHeartbeat
// survives janitor sweeps even though the total elapsed time since
// registration exceeds TaskTokenTTL, as long as no single gap between
// heartbeats exceeds the TTL. Uses synctest so elapsed time is real (fake)
// time, matching TestSweepTaskTokens_ReapsWaitForTaskTokenEntries's rationale.
func TestSendTaskHeartbeat_RenewsTaskTokenTTL(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		b := sfn.NewInMemoryBackendWithContext(ctx, "123456789012", "us-east-1")
		b.SetSettings(sfn.Settings{TaskTokenTTL: 10 * time.Millisecond})

		done := make(chan error, 1)
		go func() {
			_, waitErr := b.WaitForTaskToken(ctx, "hb-token", 0)
			done <- waitErr
		}()

		synctest.Wait()
		require.Equal(t, 1, b.TaskTokenCount(), "WaitForTaskToken must register its token")

		// Heartbeat every 7ms for 21ms total -- longer than the 10ms TTL
		// measured from registration, but each individual gap stays under it.
		for range 3 {
			time.Sleep(7 * time.Millisecond)
			require.NoError(t, b.SendTaskHeartbeat("hb-token"))
		}

		evicted := b.SweepTaskTokens()
		require.Equal(t, 0, evicted, "a heartbeating callback task must not be evicted at TTL")
		require.Equal(t, 1, b.TaskTokenCount())

		require.NoError(t, b.SendTaskSuccess("hb-token", `{}`))
		synctest.Wait()

		select {
		case waitErr := <-done:
			require.NoError(t, waitErr, "WaitForTaskToken must complete normally, not via TTL eviction")
		default:
			t.Fatal("WaitForTaskToken did not complete after SendTaskSuccess")
		}
	})
}

// TestSendTaskHeartbeat_DoesNotExtendContextTimeout verifies that renewing
// the TaskTokenTTL backstop via SendTaskHeartbeat does not push out a Task
// state's own TimeoutSeconds, which the ASL executor enforces as a ctx
// deadline around WaitForTaskToken/InvokeActivity (asl/executor.go
// runTaskAttempt), independent of tasksByToken's createdAt.
func TestSendTaskHeartbeat_DoesNotExtendContextTimeout(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := sfn.NewInMemoryBackendWithContext(context.Background(), "123456789012", "us-east-1")

		taskCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			_, waitErr := b.WaitForTaskToken(taskCtx, "timeout-token", 0)
			done <- waitErr
		}()

		time.Sleep(5 * time.Millisecond)
		require.NoError(t, b.SendTaskHeartbeat("timeout-token"))

		// Advance past the 20ms deadline (15ms remaining from t=5ms); synctest
		// auto-advances the fake clock to the ctx timer once both goroutines
		// are durably blocked, then Wait lets the unblocked goroutine run to
		// its done-channel send before we check it.
		time.Sleep(16 * time.Millisecond)
		synctest.Wait()

		select {
		case waitErr := <-done:
			require.ErrorIs(t, waitErr, context.DeadlineExceeded,
				"a heartbeat must not extend the overall task's context deadline")
		default:
			t.Fatal("WaitForTaskToken did not observe context deadline expiry")
		}
	})
}

// TestLeak_DeletedExecsTombstoneCleanup verifies that pruneExecutionsLocked
// cleans up orphaned tombstones (goroutines that exited without clearing them).
func TestDeletedExecsTombstoneCleanup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupFn func(t *testing.T, bk *sfn.InMemoryBackend) float64 // returns prune cutoff
		name    string
	}{
		{
			name: "no tombstones after prune clears finished execs",
			setupFn: func(t *testing.T, bk *sfn.InMemoryBackend) float64 {
				t.Helper()

				ctx := context.Background()
				sm, err := bk.CreateStateMachine(
					ctx, "tomb-sm",
					`{"StartAt":"P","States":{"P":{"Type":"Pass","End":true}}}`,
					"arn:aws:iam::123456789012:role/r", "STANDARD",
				)
				require.NoError(t, err)

				exec, err := bk.StartExecution(sm.StateMachineArn, "tomb-exec", `{}`)
				require.NoError(t, err)

				require.Eventually(t, func() bool {
					e, descErr := bk.DescribeExecution(exec.ExecutionArn)

					return descErr == nil && e.Status != "RUNNING"
				}, 3*time.Second, 10*time.Millisecond)

				// Return a cutoff far in the future to prune everything.
				return float64(time.Now().Add(10 * time.Second).Unix())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bk := sfn.NewInMemoryBackend()
			cutoff := tt.setupFn(t, bk)

			beforeTombstones := bk.DeletedExecsCountForTest()
			bk.PruneExecutionsForTest(cutoff)
			afterTombstones := bk.DeletedExecsCountForTest()

			// Tombstone count should not increase after pruning.
			assert.LessOrEqual(t, afterTombstones, beforeTombstones,
				"tombstone count should not increase after prune")
		})
	}
}
