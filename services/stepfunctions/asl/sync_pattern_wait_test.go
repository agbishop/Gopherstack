package asl_test

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions/asl"
)

// --- mock ECSSyncWaiter / GlueSyncWaiter (gopherstack-tdp6) ---

// mockECSSyncWaiter reports Done=false for the first pollsBeforeDone calls,
// then reports the configured terminal outcome, so tests can prove the
// executor actually polls more than once before advancing.
type mockECSSyncWaiter struct {
	result          any
	failureReason   string
	pollsBeforeDone int
	polls           int
	mu              sync.Mutex
	failed          bool
}

func (m *mockECSSyncWaiter) SFNPollSyncTask(_ context.Context, _ any) (asl.ECSSyncPoll, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.polls++
	if m.polls <= m.pollsBeforeDone {
		return asl.ECSSyncPoll{}, nil
	}

	return asl.ECSSyncPoll{
		Done:          true,
		Failed:        m.failed,
		FailureReason: m.failureReason,
		Result:        m.result,
	}, nil
}

func (m *mockECSSyncWaiter) pollCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.polls
}

type mockGlueSyncWaiter struct {
	result          any
	failureReason   string
	pollsBeforeDone int
	polls           int
	mu              sync.Mutex
	failed          bool
}

func (m *mockGlueSyncWaiter) SFNPollSyncJobRun(_ context.Context, _, _ string) (asl.GlueSyncPoll, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.polls++
	if m.polls <= m.pollsBeforeDone {
		return asl.GlueSyncPoll{}, nil
	}

	return asl.GlueSyncPoll{
		Done:          true,
		Failed:        m.failed,
		FailureReason: m.failureReason,
		Result:        m.result,
	}, nil
}

func (m *mockGlueSyncWaiter) pollCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.polls
}

const ecsSyncTaskDef = `{
	"StartAt": "Run",
	"States": {
		"Run": {
			"Type": "Task",
			"Resource": "arn:aws:states:::ecs:runTask.sync",
			"End": true
		}
	}
}`

const glueSyncJobDef = `{
	"StartAt": "Run",
	"States": {
		"Run": {
			"Type": "Task",
			"Resource": "arn:aws:states:::glue:startJobRun.sync",
			"End": true
		}
	}
}`

// TestSFN_ECSSyncWaitsForTaskCompletion is a regression test for
// gopherstack-tdp6: before the fix, a ".sync" RunTask dispatched
// fire-and-forget and the state advanced with RunTask's own start-call
// response as soon as it returned, never consulting task completion at all.
func TestSFN_ECSSyncWaitsForTaskCompletion(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		sm, err := asl.Parse(ecsSyncTaskDef)
		require.NoError(t, err)

		startResponse := map[string]any{"Tasks": []any{"start-response"}, "Failures": []any{}}
		finalResponse := map[string]any{"Tasks": []any{"stopped-task"}, "Failures": []any{}}

		waiter := &mockECSSyncWaiter{pollsBeforeDone: 3, result: finalResponse}

		exec := asl.NewExecutor(sm, nil, nil)
		exec.SetECSIntegration(&mockECS{returnOutput: startResponse})
		exec.SetECSSyncWaiter(waiter)

		result, execErr := exec.Execute(t.Context(), "exec-ecs-sync-wait", `{}`)
		require.NoError(t, execErr)
		require.Empty(t, result.Error)

		// The state's output is ECS's own description of the *completed*
		// task, not RunTask's start-call response.
		assert.Equal(t, finalResponse, result.Output)
		// The executor actually polled repeatedly rather than accepting the
		// first (not-yet-terminal) observation.
		assert.GreaterOrEqual(t, waiter.pollCount(), 4)
	})
}

// TestSFN_ECSSyncTaskFailureFailsState is a regression test for
// gopherstack-tdp6: before the fix, nothing ever observed task completion,
// so a failing ECS task could never fail the state.
func TestSFN_ECSSyncTaskFailureFailsState(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		sm, err := asl.Parse(ecsSyncTaskDef)
		require.NoError(t, err)

		waiter := &mockECSSyncWaiter{
			pollsBeforeDone: 1,
			failed:          true,
			failureReason:   "ECS task stopped: Essential container in task exited (container \"c\" exited with code 1)",
		}

		exec := asl.NewExecutor(sm, nil, nil)
		exec.SetECSIntegration(&mockECS{returnOutput: map[string]any{"Tasks": []any{}, "Failures": []any{}}})
		exec.SetECSSyncWaiter(waiter)

		result, execErr := exec.Execute(t.Context(), "exec-ecs-sync-fail", `{}`)
		require.NoError(t, execErr)

		assert.True(t, result.Failed)
		assert.Equal(t, "TaskFailed", result.Error)
		assert.Contains(t, result.Cause, "exited with code 1")
	})
}

// TestSFN_ECSSyncUnwiredWaiterStaysFireAndForget proves that wiring
// ECSIntegration alone (no ECSSyncWaiter) leaves ".sync" ECS tasks exactly
// as fire-and-forget as they were before gopherstack-tdp6 -- an unwired
// hook must be a silent no-op, never a hang.
func TestSFN_ECSSyncUnwiredWaiterStaysFireAndForget(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		sm, err := asl.Parse(ecsSyncTaskDef)
		require.NoError(t, err)

		startResponse := map[string]any{"Tasks": []any{"start-response"}, "Failures": []any{}}

		exec := asl.NewExecutor(sm, nil, nil)
		exec.SetECSIntegration(&mockECS{returnOutput: startResponse})
		// Deliberately not calling SetECSSyncWaiter.

		result, execErr := exec.Execute(t.Context(), "exec-ecs-sync-unwired", `{}`)
		require.NoError(t, execErr)
		require.Empty(t, result.Error)
		assert.Equal(t, startResponse, result.Output)
	})
}

// TestSFN_GlueSyncWaitsForJobRunCompletion mirrors
// TestSFN_ECSSyncWaitsForTaskCompletion for Glue.
func TestSFN_GlueSyncWaitsForJobRunCompletion(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		sm, err := asl.Parse(glueSyncJobDef)
		require.NoError(t, err)

		finalResponse := map[string]any{"JobRun": map[string]any{"JobRunState": "SUCCEEDED"}}
		waiter := &mockGlueSyncWaiter{pollsBeforeDone: 3, result: finalResponse}

		exec := asl.NewExecutor(sm, nil, nil)
		exec.SetGlueIntegration(&mockGlue{returnJobRunID: "jr-1"})
		exec.SetGlueSyncWaiter(waiter)

		result, execErr := exec.Execute(t.Context(), "exec-glue-sync-wait", `{"JobName": "job"}`)
		require.NoError(t, execErr)
		require.Empty(t, result.Error)

		assert.Equal(t, finalResponse, result.Output)
		assert.GreaterOrEqual(t, waiter.pollCount(), 4)
	})
}

// TestSFN_GlueSyncJobRunFailureFailsState mirrors
// TestSFN_ECSSyncTaskFailureFailsState for Glue.
func TestSFN_GlueSyncJobRunFailureFailsState(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		sm, err := asl.Parse(glueSyncJobDef)
		require.NoError(t, err)

		waiter := &mockGlueSyncWaiter{
			pollsBeforeDone: 1,
			failed:          true,
			failureReason:   "Glue job run job/jr-1 ended in state TIMEOUT",
		}

		exec := asl.NewExecutor(sm, nil, nil)
		exec.SetGlueIntegration(&mockGlue{returnJobRunID: "jr-1"})
		exec.SetGlueSyncWaiter(waiter)

		result, execErr := exec.Execute(t.Context(), "exec-glue-sync-fail", `{"JobName": "job"}`)
		require.NoError(t, execErr)

		assert.True(t, result.Failed)
		assert.Equal(t, "TaskFailed", result.Error)
		assert.Contains(t, result.Cause, "TIMEOUT")
	})
}

// TestSFN_GlueSyncUnwiredWaiterStaysFireAndForget mirrors
// TestSFN_ECSSyncUnwiredWaiterStaysFireAndForget for Glue.
func TestSFN_GlueSyncUnwiredWaiterStaysFireAndForget(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		sm, err := asl.Parse(glueSyncJobDef)
		require.NoError(t, err)

		exec := asl.NewExecutor(sm, nil, nil)
		exec.SetGlueIntegration(&mockGlue{returnJobRunID: "jr-1"})
		// Deliberately not calling SetGlueSyncWaiter.

		result, execErr := exec.Execute(t.Context(), "exec-glue-sync-unwired", `{"JobName": "job"}`)
		require.NoError(t, execErr)
		require.Empty(t, result.Error)
		assert.Equal(t, map[string]any{"JobRunId": "jr-1"}, result.Output)
	})
}

// TestSFN_ECSSyncBoundedByTimeoutSeconds proves a ".sync" ECS task that
// never reaches a terminal state is bounded by the Task state's own
// TimeoutSeconds -- the same deadline every other Task type in this
// executor already honors -- rather than blocking the execution forever.
func TestSFN_ECSSyncBoundedByTimeoutSeconds(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		def := `{
			"StartAt": "Run",
			"States": {
				"Run": {
					"Type": "Task",
					"Resource": "arn:aws:states:::ecs:runTask.sync",
					"TimeoutSeconds": 5,
					"Catch": [{"ErrorEquals": ["States.Timeout"], "Next": "TimedOut"}],
					"End": true
				},
				"TimedOut": {"Type": "Pass", "End": true, "Result": "timeout"}
			}
		}`

		sm, err := asl.Parse(def)
		require.NoError(t, err)

		// Never reports Done: this task never reaches STOPPED.
		waiter := &mockECSSyncWaiter{pollsBeforeDone: 1 << 30}

		exec := asl.NewExecutor(sm, nil, nil)
		exec.SetECSIntegration(&mockECS{returnOutput: map[string]any{"Tasks": []any{}, "Failures": []any{}}})
		exec.SetECSSyncWaiter(waiter)

		result, execErr := exec.Execute(t.Context(), "exec-ecs-sync-timeout", `{}`)
		require.NoError(t, execErr)
		require.Empty(t, result.Error)
		assert.Equal(t, "timeout", result.Output)
	})
}
