package swf_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/swf"
)

func TestWorkflowExecution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr    error
		name       string
		domain     string
		workflowID string
		runID      string
		wantStatus string
		wantRunID  string
		startFirst bool
	}{
		{
			name:       "StartAndDescribe",
			startFirst: true,
			domain:     "my-domain",
			workflowID: "wf-001",
			runID:      "run-001",
			wantStatus: "RUNNING",
			wantRunID:  "run-001",
		},
		{
			name:       "DescribeNotFound",
			domain:     "my-domain",
			workflowID: "nonexistent",
			wantErr:    swf.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := swf.NewInMemoryBackend()
			require.NoError(t, b.RegisterDomain(tt.domain, "", "NONE"))

			if tt.startFirst {
				exec, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
					Domain:     tt.domain,
					WorkflowID: tt.workflowID,
					RunID:      tt.runID,
				})
				require.NoError(t, err)
				assert.Equal(t, tt.wantStatus, exec.Status)
			}

			got, err := b.DescribeWorkflowExecution(tt.domain, tt.workflowID, "")
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRunID, got.RunID)
		})
	}
}

func TestStartWorkflowExecution_WithWorkflowType(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterWorkflowType("dom", "wf", "1.0", "", swf.WorkflowTypeDefaults{
		DefaultTaskList:    "default",
		DefaultChildPolicy: "TERMINATE",
	}))

	exec, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain:              "dom",
		WorkflowID:          "wf-1",
		WorkflowTypeName:    "wf",
		WorkflowTypeVersion: "1.0",
		Input:               `{"key":"value"}`,
		TagList:             []string{"env:test"},
	})
	require.NoError(t, err)
	assert.Equal(t, "RUNNING", exec.Status)
	assert.Equal(t, "wf", exec.WorkflowTypeName)
	assert.Equal(t, "default", exec.TaskList)
	assert.Equal(t, "TERMINATE", exec.ChildPolicy)
	assert.Equal(t, []string{"env:test"}, exec.TagList)
}

func TestStartWorkflowExecution_DeprecatedTypeRejected(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.RegisterWorkflowType("dom", "wf", "1.0", "", swf.WorkflowTypeDefaults{}))
	require.NoError(t, b.DeprecateWorkflowType("dom", "wf", "1.0"))

	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain:              "dom",
		WorkflowID:          "wf-1",
		WorkflowTypeName:    "wf",
		WorkflowTypeVersion: "1.0",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrTypeDeprecated)
}

// TestStartWorkflowExecution_DeprecatedDomainRejected verifies ErrNotFound
// (UnknownResourceFault) -- per DeprecateDomain's doc ("it cannot be used to
// create new workflow executions or register new types"). StartWorkflowExecution
// models UnknownResourceFault but no DomainDeprecatedFault.
func TestStartWorkflowExecution_DeprecatedDomainRejected(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	require.NoError(t, b.DeprecateDomain("dom"))

	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain:     "dom",
		WorkflowID: "wf-1",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrNotFound)
}

// TestErrValidation_StartWorkflowExecution validates required fields.
func TestErrValidation_StartWorkflowExecution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		domain     string
		workflowID string
	}{
		{name: "empty_domain", domain: "", workflowID: "wf-1"},
		{name: "empty_workflowID", domain: "d1", workflowID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := swf.NewInMemoryBackend()
			_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
				Domain:     tt.domain,
				WorkflowID: tt.workflowID,
				RunID:      "run-1",
			})

			require.Error(t, err)
			assert.ErrorIs(t, err, swf.ErrValidation)
		})
	}
}

func TestValidateChildPolicy(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))

	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain:      "dom",
		WorkflowID:  "wf-1",
		ChildPolicy: "INVALID_POLICY",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrValidation)
}

// TestStartWorkflowExecution_EnqueuesInitialDecisionTask verifies that starting
// a workflow execution schedules its first decision task, matching real AWS
// SWF (which schedules the initial decision task immediately after
// WorkflowExecutionStarted). Without this, a freshly started workflow with no
// other stimulus (signal, cancel, activity completion) never gets a decision
// task and stays OPEN forever -- no decider could ever poll for it.
func TestStartWorkflowExecution_EnqueuesInitialDecisionTask(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))

	assert.Equal(t, 0, b.CountPendingDecisionTasks("dom", "default"))

	exec, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain:     "dom",
		WorkflowID: "wf-1",
		TaskList:   "default",
	})
	require.NoError(t, err)

	assert.Equal(t, 1, b.CountPendingDecisionTasks("dom", "default"))

	task := b.PollForDecisionTask("dom", "default", 0, "")
	require.NotNil(t, task, "expected an initial decision task without any other stimulus")
	assert.Equal(t, "wf-1", task.WorkflowID)
	assert.Equal(t, exec.RunID, task.RunID)
	assert.NotEmpty(t, task.Events, "decision task should include the WorkflowExecutionStarted event")
}

// TestStartWorkflowExecution_NoTaskListNoDecisionTask verifies that starting a
// workflow without a task list (and no workflow-type default) does not panic
// or enqueue a decision task nobody can ever poll for.
func TestStartWorkflowExecution_NoTaskListNoDecisionTask(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))

	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain:     "dom",
		WorkflowID: "wf-1",
	})
	require.NoError(t, err)

	assert.Equal(t, 0, b.CountPendingDecisionTasks("dom", ""))
}

// TestStartWorkflowExecution_SetsTimestamp verifies StartTimestamp is non-zero.
func TestStartWorkflowExecution_SetsTimestamp(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	exec, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain: "dom", WorkflowID: "wf-1", RunID: "run-1",
	})

	require.NoError(t, err)
	assert.NotZero(t, exec.StartTimestamp)
}

// TestParity_StartWorkflowExecution_AlreadyStarted verifies WorkflowExecutionAlreadyStartedFault.
func TestStartWorkflowExecution_AlreadyStarted(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain: "dom", WorkflowID: "wf-1",
	})
	require.NoError(t, err)

	_, err = b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain: "dom", WorkflowID: "wf-1",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrWorkflowAlreadyStarted)
}

func TestTerminateWorkflowExecution_ReasonInHistory(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain:     "dom",
		WorkflowID: "wf-1",
	})
	require.NoError(t, err)

	require.NoError(t, b.TerminateWorkflowExecution("dom", "wf-1", "", "out of budget", "details here", ""))

	events, _ := b.GetWorkflowExecutionHistory("dom", "wf-1", "", 0, "", false)
	require.NotEmpty(t, events)
	last := events[len(events)-1]
	assert.Equal(t, "WorkflowExecutionTerminated", last.EventType)
	attrKey := "workflowExecutionTerminatedEventAttributes"
	attrs, ok := last.Attributes[attrKey].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "out of budget", attrs["reason"])
}

// TestTerminateWorkflowExecution verifies status change.
func TestTerminateWorkflowExecution(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain: "dom", WorkflowID: "wf-1", RunID: "run-1",
	})
	require.NoError(t, err)

	require.NoError(t, b.TerminateWorkflowExecution("dom", "wf-1", "", "", "", ""))

	exec, err := b.DescribeWorkflowExecution("dom", "wf-1", "")
	require.NoError(t, err)
	assert.Equal(t, "TERMINATED", exec.Status)
	assert.Equal(t, "TERMINATED", exec.CloseStatus)
	assert.NotZero(t, exec.CloseTimestamp)
}

// TestTerminateWorkflowExecution_NotFound returns ErrNotFound.
func TestTerminateWorkflowExecution_NotFound(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	err := b.TerminateWorkflowExecution("dom", "missing", "", "", "", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrNotFound)
}

// TestTerminateWorkflowExecution_AlreadyTerminated returns ErrNotFound.
func TestTerminateWorkflowExecution_AlreadyTerminated(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain: "dom", WorkflowID: "wf-1", RunID: "run-1",
	})
	require.NoError(t, err)

	require.NoError(t, b.TerminateWorkflowExecution("dom", "wf-1", "", "", "", ""))

	err = b.TerminateWorkflowExecution("dom", "wf-1", "", "", "", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrNotFound)
}

// TestTerminateWorkflowExecution_AlreadyClosed verifies UnknownResourceFault on a closed execution.
func TestTerminateWorkflowExecution_AlreadyClosed(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain: "dom", WorkflowID: "wf-1", RunID: "run-1",
	})
	require.NoError(t, err)
	require.NoError(t, b.TerminateWorkflowExecution("dom", "wf-1", "", "", "", ""))

	err = b.TerminateWorkflowExecution("dom", "wf-1", "", "", "", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, swf.ErrNotFound)
}

func TestRequestCancelWorkflowExecution_SetsFlag(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain:     "dom",
		WorkflowID: "wf-1",
		TaskList:   "default",
	})
	require.NoError(t, err)

	require.NoError(t, b.RequestCancelWorkflowExecution("dom", "wf-1", ""))

	exec, err := b.DescribeWorkflowExecution("dom", "wf-1", "")
	require.NoError(t, err)
	assert.True(t, exec.CancelRequested)
}

// TestRequestCancelWorkflowExecution_NotOpen_UnknownResourceFault verifies
// RequestCancelWorkflowExecution on a closed execution fails with
// UnknownResourceFault, per the real SWF API doc: "If the specified workflow
// execution isn't open, this method fails with UnknownResource." --
// ValidationException isn't in this operation's fault model at all.
func TestRequestCancelWorkflowExecution_NotOpen_UnknownResourceFault(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain:     "dom",
		WorkflowID: "wf-1",
	})
	require.NoError(t, err)
	require.NoError(t, b.TerminateWorkflowExecution("dom", "wf-1", "", "", "", ""))

	err = b.RequestCancelWorkflowExecution("dom", "wf-1", "")
	require.ErrorIs(t, err, swf.ErrNotFound)
	assert.NotErrorIs(t, err, swf.ErrValidation)
}

// TestCountTerminatedExecution_CountsClosed verifies terminated goes to closed.
func TestCountTerminatedExecution_CountsClosed(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
	_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
		Domain: "dom", WorkflowID: "wf-1", RunID: "run-1",
	})
	require.NoError(t, err)

	assert.Equal(t, 1, b.CountOpenWorkflowExecutions("dom", swf.ExecutionFilter{}))
	assert.Equal(t, 0, b.CountClosedWorkflowExecutions("dom", swf.ExecutionFilter{}))

	require.NoError(t, b.TerminateWorkflowExecution("dom", "wf-1", "", "", "", ""))

	assert.Equal(t, 0, b.CountOpenWorkflowExecutions("dom", swf.ExecutionFilter{}))
	assert.Equal(t, 1, b.CountClosedWorkflowExecutions("dom", swf.ExecutionFilter{}))
}

// TestExecutionStatus_OpenClosed verifies executionStatus is OPEN/CLOSED not internal values.
func TestExecutionStatus_OpenClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		wantExecStatus string
		terminate      bool
		wantHasClose   bool
	}{
		{name: "running_is_open", terminate: false, wantExecStatus: "OPEN", wantHasClose: false},
		{name: "terminated_is_closed", terminate: true, wantExecStatus: "CLOSED", wantHasClose: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := swf.NewInMemoryBackend()
			require.NoError(t, b.RegisterDomain("dom", "", "NONE"))
			_, err := b.StartWorkflowExecution(swf.StartWorkflowExecutionInput{
				Domain: "dom", WorkflowID: "wf-1",
			})
			require.NoError(t, err)

			if tt.terminate {
				require.NoError(t, b.TerminateWorkflowExecution("dom", "wf-1", "", "", "", ""))
			}

			h := swf.NewHandler(b)
			rec := doSWFRequest(t, h, "DescribeWorkflowExecution", map[string]any{
				"domain":    "dom",
				"execution": map[string]any{"workflowId": "wf-1", "runId": ""},
			})
			require.Equal(t, 200, rec.Code)

			resp := parseSWFResp(t, rec)
			info := resp["executionInfo"].(map[string]any)
			assert.Equal(t, tt.wantExecStatus, info["executionStatus"])

			_, hasClose := info["closeStatus"]
			assert.Equal(t, tt.wantHasClose, hasClose)
		})
	}
}
