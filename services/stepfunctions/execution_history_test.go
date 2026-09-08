package stepfunctions_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions"
)

func TestGetExecutionHistory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr      error
		name         string
		executionArn string
		wantFirst    string
		wantSecond   string
		wantLen      int
		createExec   bool
		reverse      bool
	}{
		{
			name:       "forward",
			createExec: true,
			wantLen:    4,
			wantFirst:  "ExecutionStarted",
			wantSecond: "PassStateEntered",
		},
		{
			name:       "ReverseOrder",
			createExec: true,
			reverse:    true,
			wantLen:    4,
			wantFirst:  "ExecutionSucceeded",
			wantSecond: "PassStateExited",
		},
		{
			name:         "NotFound",
			executionArn: "arn:nonexistent",
			wantErr:      stepfunctions.ErrExecutionDoesNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

			arn := tt.executionArn
			if tt.createExec {
				sm, err := b.CreateStateMachine(context.Background(), "hist-sm", passDefinition, "arn:role", "STANDARD")
				require.NoError(t, err)
				exec, err := b.StartExecution(sm.StateMachineArn, "exec-h", "")
				require.NoError(t, err)
				arn = exec.ExecutionArn
				// Wait for async execution to complete.
				require.Eventually(t, func() bool {
					desc, descErr := b.DescribeExecution(arn)

					return descErr == nil && desc.Status != "RUNNING"
				}, 5*time.Second, 50*time.Millisecond)
			}

			events, next, err := b.GetExecutionHistory(arn, "", 0, tt.reverse)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Empty(t, next)
			assert.Len(t, events, tt.wantLen)
			assert.Equal(t, tt.wantFirst, events[0].Type)
			assert.Equal(t, tt.wantSecond, events[1].Type)
		})
	}
}

const taskLambdaDefinition = `{
"StartAt": "T",
"States": {
"T": {"Type": "Task", "Resource": "arn:aws:lambda:us-east-1:000000000000:function:fn", "End": true}
}
}`

// TestGetExecutionHistory_TaskEventDetails verifies that Task lifecycle
// history events (TaskScheduled/TaskSucceeded/TaskFailed) and state
// entered/exited events carry their AWS-documented detail payloads
// (resource, input, output, error, cause) rather than an empty details
// object.
func TestGetExecutionHistory_TaskEventDetails(t *testing.T) {
	t.Parallel()

	t.Run("succeeded_task_populates_resource_and_output", func(t *testing.T) {
		t.Parallel()

		b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
		b.SetLambdaInvoker(&mockLambdaForBackend{})

		sm, err := b.CreateStateMachine(
			context.Background(),
			"hist-task-sm",
			taskLambdaDefinition,
			"arn:role",
			"STANDARD",
		)
		require.NoError(t, err)

		exec, err := b.StartExecution(sm.StateMachineArn, "exec-task-ok", `{"in": 1}`)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			desc, descErr := b.DescribeExecution(exec.ExecutionArn)

			return descErr == nil && desc.Status != "RUNNING"
		}, 5*time.Second, 50*time.Millisecond)

		events, _, err := b.GetExecutionHistory(exec.ExecutionArn, "", 0, false)
		require.NoError(t, err)

		var sawScheduled, sawSucceeded, sawStateEntered bool

		for _, ev := range events {
			switch ev.Type {
			case "TaskScheduled":
				require.NotNil(t, ev.TaskScheduledEventDetails)
				assert.Equal(
					t,
					"arn:aws:lambda:us-east-1:000000000000:function:fn",
					ev.TaskScheduledEventDetails.Resource,
				)
				assert.Equal(t, "lambda", ev.TaskScheduledEventDetails.ResourceType)
				sawScheduled = true
			case "TaskSucceeded":
				require.NotNil(t, ev.TaskSucceededEventDetails)
				assert.Contains(t, ev.TaskSucceededEventDetails.Output, "ok")
				require.NotNil(t, ev.TaskSucceededEventDetails.OutputDetails)
				assert.False(t, ev.TaskSucceededEventDetails.OutputDetails.Truncated)
				sawSucceeded = true
			case "TaskStateEntered":
				require.NotNil(t, ev.StateEnteredEventDetails)
				assert.Contains(t, ev.StateEnteredEventDetails.Input, `"in":1`)
				sawStateEntered = true
			}
		}

		assert.True(t, sawScheduled, "expected a TaskScheduled event")
		assert.True(t, sawSucceeded, "expected a TaskSucceeded event")
		assert.True(t, sawStateEntered, "expected a TaskStateEntered event with populated input")
	})

	t.Run("failed_task_populates_error_and_cause", func(t *testing.T) {
		t.Parallel()

		b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
		b.SetLambdaInvoker(&mockLambdaForBackend{returnErr: assert.AnError})

		sm, err := b.CreateStateMachine(
			context.Background(),
			"hist-task-fail-sm",
			taskLambdaDefinition,
			"arn:role",
			"STANDARD",
		)
		require.NoError(t, err)

		exec, err := b.StartExecution(sm.StateMachineArn, "exec-task-fail", `{}`)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			desc, descErr := b.DescribeExecution(exec.ExecutionArn)

			return descErr == nil && desc.Status != "RUNNING"
		}, 5*time.Second, 50*time.Millisecond)

		events, _, err := b.GetExecutionHistory(exec.ExecutionArn, "", 0, false)
		require.NoError(t, err)

		var sawFailed bool

		for _, ev := range events {
			if ev.Type == "TaskFailed" {
				require.NotNil(t, ev.TaskFailedEventDetails)
				assert.NotEmpty(t, ev.TaskFailedEventDetails.Error)
				assert.NotEmpty(t, ev.TaskFailedEventDetails.Cause)
				sawFailed = true
			}
		}

		assert.True(t, sawFailed, "expected a TaskFailed event")
	})
}

func TestHandler_GetExecutionHistory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, ctx context.Context, h *stepfunctions.Handler, e *echo.Echo) string
		bodyFn     func(setupResult string) string
		name       string
		body       string
		wantCode   int
		wantEvents int
	}{
		{
			name: "returns history events for execution",
			setup: func(t *testing.T, ctx context.Context, h *stepfunctions.Handler, e *echo.Echo) string {
				t.Helper()

				smArn := createSM(ctx, t, h, e, "hist-sm")
				execArn := startExec(ctx, t, h, e, smArn, "hist-exec")

				// Wait for the async execution to complete before checking history.
				require.Eventually(t, func() bool {
					rec := sfnPost(ctx, t, h, e, "DescribeExecution",
						`{"executionArn":"`+execArn+`"}`)
					if rec.Code != http.StatusOK {
						return false
					}
					var resp map[string]any
					if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
						return false
					}

					return resp["status"] != "RUNNING"
				}, 5*time.Second, 50*time.Millisecond)

				return execArn
			},
			bodyFn:     func(execArn string) string { return `{"executionArn":"` + execArn + `"}` },
			wantCode:   http.StatusOK,
			wantEvents: 4,
		},
		{
			name:     "not found returns 404",
			body:     `{"executionArn":"arn:nonexistent"}`,
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)

			var setupResult string
			if tt.setup != nil {
				setupResult = tt.setup(t, ctx, h, e)
			}

			body := tt.body
			if tt.bodyFn != nil {
				body = tt.bodyFn(setupResult)
			}

			rec := sfnPost(ctx, t, h, e, "GetExecutionHistory", body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantEvents > 0 {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Len(t, resp["events"].([]any), tt.wantEvents)
			}
		})
	}
}

func TestGetExecutionHistory_HasExecutionStarted(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"hist-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)
	defer b.Destroy()

	exec, err := b.StartExecution(sm.StateMachineArn, "hist-exec", "{}")
	require.NoError(t, err)

	// Allow time for history to populate.
	require.Eventually(t, func() bool {
		events, _, err2 := b.GetExecutionHistory(exec.ExecutionArn, "", 100, false)

		return err2 == nil && len(events) >= 1
	}, 5*time.Second, 20*time.Millisecond)

	events, _, err := b.GetExecutionHistory(exec.ExecutionArn, "", 100, false)
	require.NoError(t, err)
	assert.Equal(t, "ExecutionStarted", events[0].Type)
}

func TestGetExecutionHistory_ReverseOrder(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"rev-hist-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)
	defer b.Destroy()

	exec, err := b.StartExecution(sm.StateMachineArn, "rev-hist-exec", "{}")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		d, e := b.DescribeExecution(exec.ExecutionArn)

		return e == nil && d.Status == "SUCCEEDED"
	}, 5*time.Second, 20*time.Millisecond)

	events, _, err := b.GetExecutionHistory(exec.ExecutionArn, "", 100, true)
	require.NoError(t, err)
	require.NotEmpty(t, events)
	// Last event in forward order should be first in reverse.
	assert.Equal(t, "ExecutionSucceeded", events[0].Type)
}

func TestGetExecutionHistory_Pagination(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"page-hist-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)
	defer b.Destroy()

	exec, err := b.StartExecution(sm.StateMachineArn, "page-hist-exec", "{}")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		d, e := b.DescribeExecution(exec.ExecutionArn)

		return e == nil && d.Status == "SUCCEEDED"
	}, 5*time.Second, 20*time.Millisecond)

	// Get all events first.
	all, _, err := b.GetExecutionHistory(exec.ExecutionArn, "", 100, false)
	require.NoError(t, err)
	require.NotEmpty(t, all)

	// Paginate with maxResults=1.
	var collected []stepfunctions.HistoryEvent
	tok := ""

	for {
		page, next, err2 := b.GetExecutionHistory(exec.ExecutionArn, tok, 1, false)
		require.NoError(t, err2)
		collected = append(collected, page...)

		if next == "" {
			break
		}

		tok = next
	}

	assert.Len(t, collected, len(all))
}

func TestGetExecutionHistory_EventIDsMonotonicallyIncreasing(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"mono-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)
	defer b.Destroy()

	exec, err := b.StartExecution(sm.StateMachineArn, "mono-exec", "{}")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		d, e := b.DescribeExecution(exec.ExecutionArn)

		return e == nil && d.Status == "SUCCEEDED"
	}, 5*time.Second, 20*time.Millisecond)

	events, _, err := b.GetExecutionHistory(exec.ExecutionArn, "", 100, false)
	require.NoError(t, err)

	for i := 1; i < len(events); i++ {
		assert.Greater(
			t,
			events[i].ID,
			events[i-1].ID,
			"event IDs must be monotonically increasing",
		)
	}
}

// ─── ListExecutions ───────────────────────────────────────────────────────────

// TestHistoryEventCap verifies that history recording is capped at maxHistoryEvents.
func TestHistoryEventCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		preFill       int
		addMoreEvents int
		wantLen       int
	}{
		{
			name:          "cap_enforced_when_at_limit",
			preFill:       stepfunctions.MaxHistoryEventsForTest,
			addMoreEvents: 5,
			wantLen:       stepfunctions.MaxHistoryEventsForTest,
		},
		{
			name:          "events_recorded_below_cap",
			preFill:       10,
			addMoreEvents: 5,
			wantLen:       15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newSFBackend()
			sm, err := b.CreateStateMachine(context.Background(), "cap-sm", exprPassDef, "arn:role", "STANDARD")
			require.NoError(t, err)

			exec, err := b.StartExecution(sm.StateMachineArn, "cap-exec", "{}")
			require.NoError(t, err)

			execARN := exec.ExecutionArn

			// Wait for execution to reach terminal state so goroutine is done.
			require.Eventually(t, func() bool {
				d, e := b.DescribeExecution(execARN)

				return e == nil && d.Status != "RUNNING"
			}, 5*time.Second, 50*time.Millisecond)

			// Pre-fill history to the desired count using the exported test helper.
			b.FillHistoryForTest(execARN, tt.preFill)

			// Try to add more events via the exported recorder helper.
			for range tt.addMoreEvents {
				b.RecordStateEnteredForTest(execARN, "ExtraState", "Pass")
			}

			histLen := b.HistoryLenForTest(execARN)
			assert.Equal(t, tt.wantLen, histLen)
		})
	}
}

func TestBackend_GetExecutionHistory_ReverseOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wantFirst    string
		reverseOrder bool
	}{
		{
			name:         "normal_order",
			reverseOrder: false,
			wantFirst:    "ExecutionStarted",
		},
		{
			name:         "reverse_order_last_event_first",
			reverseOrder: true,
			wantFirst:    "ExecutionSucceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := stepfunctions.NewInMemoryBackend()
			sm, err := b.CreateStateMachine(context.Background(), "hist-sm", sfnPassDefinition, "arn:role", "STANDARD")
			require.NoError(t, err)

			exec, err := b.StartExecution(sm.StateMachineArn, "hist-exec", `{}`)
			require.NoError(t, err)

			// Wait for execution to complete
			require.Eventually(t, func() bool {
				desc, descErr := b.DescribeExecution(exec.ExecutionArn)

				return descErr == nil && desc.Status == "SUCCEEDED"
			}, 5*time.Second, 50*time.Millisecond)

			events, _, err := b.GetExecutionHistory(exec.ExecutionArn, "", 0, tt.reverseOrder)
			require.NoError(t, err)
			require.NotEmpty(t, events)
			assert.Equal(t, tt.wantFirst, events[0].Type)
		})
	}
}

// ---- Handler: executionActions via HTTP ----

func TestExecutionHistory_Events(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		definition     string
		wantEventTypes []string
	}{
		{
			name:       "pass_state_records_entered_exited",
			definition: exprPassDef,
			wantEventTypes: []string{
				"ExecutionStarted",
				"PassStateEntered",
				"PassStateExited",
				"ExecutionSucceeded",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newSFBackend()
			sm, err := b.CreateStateMachine(
				context.Background(),
				"hist-"+tt.name,
				tt.definition,
				"arn:role",
				"STANDARD",
			)
			require.NoError(t, err)

			exec, err := b.StartExecution(sm.StateMachineArn, "hist-exec", "{}")
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				d, e := b.DescribeExecution(exec.ExecutionArn)

				return e == nil && d.Status != "RUNNING"
			}, 10*time.Second, 25*time.Millisecond)

			events, _, err := b.GetExecutionHistory(exec.ExecutionArn, "", 100, false)
			require.NoError(t, err)
			require.NotEmpty(t, events)

			// Collect event types.
			types := make([]string, len(events))
			for i, e := range events {
				types[i] = e.Type
			}

			for _, wantType := range tt.wantEventTypes {
				assert.Contains(t, types, wantType, "expected event type %q in history", wantType)
			}

			// Verify IDs are monotonically increasing.
			for i := 1; i < len(events); i++ {
				assert.Greater(t, events[i].ID, events[i-1].ID, "event IDs should increase")
			}
		})
	}
}

func TestExecutionHistory_ReverseOrder(t *testing.T) {
	t.Parallel()

	b := newSFBackend()
	sm, err := b.CreateStateMachine(context.Background(), "hist-rev", exprPassDef, "arn:role", "STANDARD")
	require.NoError(t, err)

	exec, err := b.StartExecution(sm.StateMachineArn, "rev-exec", "{}")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		d, e := b.DescribeExecution(exec.ExecutionArn)

		return e == nil && d.Status != "RUNNING"
	}, 10*time.Second, 25*time.Millisecond)

	forward, _, err := b.GetExecutionHistory(exec.ExecutionArn, "", 100, false)
	require.NoError(t, err)

	reverse, _, err := b.GetExecutionHistory(exec.ExecutionArn, "", 100, true)
	require.NoError(t, err)

	require.Len(t, reverse, len(forward))

	// Reverse order means IDs should decrease.
	for i := 1; i < len(reverse); i++ {
		assert.Less(t, reverse[i].ID, reverse[i-1].ID)
	}
}

// TestResourceTypeFromResource verifies TaskScheduled/TaskSucceeded/TaskFailed's
// ResourceType is derived from the actual service-integration resource, not
// hardcoded to "lambda" for every service this emulator doesn't special-case.
func TestResourceTypeFromResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resource string
		want     string
	}{
		{"lambda direct arn", "arn:aws:lambda:us-east-1:000000000000:function:fn", "lambda"},
		{"activity", "arn:aws:states:us-east-1:000000000000:activity:act", "activity"},
		{"sqs", "arn:aws:states:::sqs:sendMessage", "sqs"},
		{"sns", "arn:aws:states:::sns:publish", "sns"},
		{"dynamodb", "arn:aws:states:::dynamodb:putItem", "dynamodb"},
		{"ecs sync", "arn:aws:states:::ecs:runTask.sync", "ecs"},
		{"glue sync", "arn:aws:states:::glue:startJobRun.sync", "glue"},
		{"eventbridge", "arn:aws:states:::events:putEvents", "events"},
		{"apigateway", "arn:aws:states:::apigateway:invoke", "apigateway"},
		{"aws-sdk prefixed", "arn:aws:states:::aws-sdk:sqs:sendMessage", "sqs"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, stepfunctions.ResourceTypeFromResourceForTest(tt.resource))
		})
	}
}
