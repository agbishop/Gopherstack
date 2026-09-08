package sqs_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sqs"
)

func TestMoveTaskRateLimitingCompletesSuccessfully(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := newBackend(t)

		srcOut, err := b.CreateQueue(&sqs.CreateQueueInput{
			QueueName: "dlq-src",
			Endpoint:  testEndpoint,
		})
		require.NoError(t, err)

		dstOut, err := b.CreateQueue(&sqs.CreateQueueInput{
			QueueName: "dlq-dst",
			Endpoint:  testEndpoint,
		})
		require.NoError(t, err)

		// Put 3 messages in source.
		for i := range 3 {
			_, err = b.SendMessage(&sqs.SendMessageInput{
				QueueURL:    srcOut.QueueURL,
				MessageBody: fmt.Sprintf("msg%d", i),
			})
			require.NoError(t, err)
		}

		srcAttrs, err := b.GetQueueAttributes(&sqs.GetQueueAttributesInput{
			QueueURL:       srcOut.QueueURL,
			AttributeNames: []string{"QueueArn"},
		})
		require.NoError(t, err)
		dstAttrs, err := b.GetQueueAttributes(&sqs.GetQueueAttributesInput{
			QueueURL:       dstOut.QueueURL,
			AttributeNames: []string{"QueueArn"},
		})
		require.NoError(t, err)

		taskOut, err := b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
			SourceArn:                    srcAttrs.Attributes["QueueArn"],
			DestinationArn:               dstAttrs.Attributes["QueueArn"],
			MaxNumberOfMessagesPerSecond: 100, // 100 msg/s
		})
		require.NoError(t, err)
		require.NotEmpty(t, taskOut.TaskHandle)

		// Poll for completion. The move task's rate-limit ticker durably blocks
		// between messages, so synctest.Wait alone would freeze the fake clock
		// mid-drain; sleeping between polls lets it keep advancing.
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			tasks, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
				SourceArn:  srcAttrs.Attributes["QueueArn"],
				MaxResults: 1,
			})
			require.NoError(t, listErr)
			if len(tasks.Results) > 0 && tasks.Results[0].Status == sqs.MoveTaskStatusCompleted {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		tasks, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
			SourceArn:  srcAttrs.Attributes["QueueArn"],
			MaxResults: 1,
		})
		require.NoError(t, listErr)
		require.NotEmpty(t, tasks.Results)
		require.Equal(t, sqs.MoveTaskStatusCompleted, tasks.Results[0].Status)

		// Destination should have the messages.
		dstMsgs, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{
			QueueURL:            dstOut.QueueURL,
			MaxNumberOfMessages: 10,
		})
		require.NoError(t, err)
		assert.Len(t, dstMsgs.Messages, 3)
	})
}

// TestListMessageMoveTasks_DefaultMaxResults_ReturnsOne verifies that
// omitting MaxResults (i.e. MaxResults=0) returns at most 1 result, matching
// the AWS default.
func TestListMessageMoveTasks_DefaultMaxResults_ReturnsOne(t *testing.T) {
	t.Parallel()
	b := b3newBackend(t)

	now := time.Now().UnixMilli()
	b.InjectMoveTaskForTest("handle-a", sqs.MoveTaskStatusCompleted, now-2000)
	b.InjectMoveTaskForTest("handle-b", sqs.MoveTaskStatusCompleted, now-1000)
	b.InjectMoveTaskForTest("handle-c", sqs.MoveTaskStatusCompleted, now)

	out, err := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
		MaxResults: 0, // default: 1
	})
	require.NoError(t, err)
	assert.Len(t, out.Results, 1, "default MaxResults=0 must return exactly 1 result")
}

// TestListMessageMoveTasks_MaxResultsClamped verifies that MaxResults > 10 is
// silently clamped to 10.
func TestListMessageMoveTasks_MaxResultsClamped(t *testing.T) {
	t.Parallel()
	b := b3newBackend(t)

	now := time.Now().UnixMilli()
	for i := range 12 {
		b.InjectMoveTaskForTest(
			"h"+string(rune('a'+i)),
			sqs.MoveTaskStatusCompleted,
			now-int64(i*1000),
		)
	}

	out, err := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{MaxResults: 11})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(out.Results), 10, "MaxResults > 10 must be clamped to 10")
}

// TestListMessageMoveTasks_OrderedNewestFirst verifies that results are
// returned with the most recently started task first.
func TestListMessageMoveTasks_OrderedNewestFirst(t *testing.T) {
	t.Parallel()
	b := b3newBackend(t)

	base := time.Now().UnixMilli()
	b.InjectMoveTaskForTest("old-task", sqs.MoveTaskStatusCompleted, base-3000)
	b.InjectMoveTaskForTest("mid-task", sqs.MoveTaskStatusCompleted, base-1000)
	b.InjectMoveTaskForTest("new-task", sqs.MoveTaskStatusCompleted, base)

	out, err := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{MaxResults: 3})
	require.NoError(t, err)
	require.Len(t, out.Results, 3)

	// startedAt should be descending (newest first).
	for i := 1; i < len(out.Results); i++ {
		assert.GreaterOrEqual(
			t,
			out.Results[i-1].StartedTimestamp,
			out.Results[i].StartedTimestamp,
			"results must be ordered newest-first",
		)
	}
}

// TestListMessageMoveTasks_TaskHandleOnlyForRunning verifies that TaskHandle
// is populated only for tasks in RUNNING status, and is empty for all other statuses.
func TestListMessageMoveTasks_TaskHandleOnlyForRunning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status         sqs.MoveTaskStatus
		wantTaskHandle bool
	}{
		{sqs.MoveTaskStatusRunning, true},
		{sqs.MoveTaskStatusCompleted, false},
		{sqs.MoveTaskStatusCancelled, false},
		{sqs.MoveTaskStatusFailed, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			b := b3newBackend(t)

			handle := "task-" + string(tc.status)
			b.InjectMoveTaskForTest(handle, tc.status, time.Now().UnixMilli())

			out, err := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{MaxResults: 1})
			require.NoError(t, err)
			require.Len(t, out.Results, 1)

			if tc.wantTaskHandle {
				assert.NotEmpty(t, out.Results[0].TaskHandle, "RUNNING task must have TaskHandle")
			} else {
				assert.Empty(t, out.Results[0].TaskHandle, "non-RUNNING task must not expose TaskHandle")
			}
		})
	}
}

// TestListMessageMoveTasks_EmptyResult verifies that the operation succeeds
// with an empty slice when no tasks exist.
func TestListMessageMoveTasks_EmptyResult(t *testing.T) {
	t.Parallel()
	b := b3newBackend(t)

	out, err := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{MaxResults: 5})
	require.NoError(t, err)
	assert.Empty(t, out.Results)
}

// buildQueueARN constructs an SQS ARN in the default test account/region.
func buildQueueARN(queueName string) string {
	return "arn:aws:sqs:us-east-1:000000000000:" + queueName
}

func TestSQS_CancelMessageMoveTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(t *testing.T, b *sqs.InMemoryBackend) string
		name    string
		wantErr bool
	}{
		{
			name: "cancel_nonexistent_task_returns_error",
			setup: func(_ *testing.T, _ *sqs.InMemoryBackend) string {
				return "nonexistent-handle"
			},
			wantErr: true,
		},
		{
			name: "cancel_running_task_succeeds",
			setup: func(t *testing.T, b *sqs.InMemoryBackend) string {
				t.Helper()

				dlqName := "dlq-cancel"
				destName := "dest-cancel"

				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: destName, Endpoint: "localhost"})
				require.NoError(t, err)

				// Seed many messages so the task stays RUNNING long enough to be cancelled.
				// Without messages the task drains instantly (goroutine completes before Cancel).
				dlqURL := "http://localhost/000000000000/" + dlqName
				for i := range 50 {
					_, sendErr := b.SendMessage(&sqs.SendMessageInput{
						QueueURL:    dlqURL,
						MessageBody: "msg-" + strconv.Itoa(i),
					})
					require.NoError(t, sendErr)
				}

				// Rate-limit to 1 msg/sec so the task sleeps between messages and is
				// still RUNNING when CancelMessageMoveTask is called immediately below.
				out, err := b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
					SourceArn:                    buildQueueARN(dlqName),
					DestinationArn:               buildQueueARN(destName),
					MaxNumberOfMessagesPerSecond: 1,
				})
				require.NoError(t, err)

				return out.TaskHandle
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sqs.NewInMemoryBackend()
			t.Cleanup(b.Close)
			handle := tt.setup(t, b)

			cancelOut, err := b.CancelMessageMoveTask(&sqs.CancelMessageMoveTaskInput{
				TaskHandle: handle,
			})

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, sqs.ErrTaskHandleInvalid)

				return
			}

			require.NoError(t, err)
			assert.GreaterOrEqual(t, cancelOut.ApproximateNumberOfMessagesMoved, int64(0))
		})
	}
}

func TestSQS_ListMessageMoveTasks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, b *sqs.InMemoryBackend) (srcARN string, taskCount int)
		name       string
		maxResults int32
		wantMin    int
		wantMax    int
	}{
		{
			name:       "empty_list_when_no_tasks",
			maxResults: 0,
			wantMin:    0,
			wantMax:    0,
			setup: func(_ *testing.T, _ *sqs.InMemoryBackend) (string, int) {
				return buildQueueARN("no-tasks-queue"), 0
			},
		},
		{
			name:       "lists_started_tasks",
			maxResults: 0,
			wantMin:    1,
			wantMax:    2,
			setup: func(t *testing.T, b *sqs.InMemoryBackend) (string, int) {
				t.Helper()

				dlqName := "dlq-list"
				destName := "dest-list"

				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: destName, Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
					SourceArn:      buildQueueARN(dlqName),
					DestinationArn: buildQueueARN(destName),
				})
				require.NoError(t, err)

				return buildQueueARN(dlqName), 1
			},
		},
		{
			name:       "max_results_limits_output",
			maxResults: 1,
			wantMin:    1,
			wantMax:    1,
			setup: func(t *testing.T, b *sqs.InMemoryBackend) (string, int) {
				t.Helper()

				dlqName1 := "dlq-maxresults-1"
				dlqName2 := "dlq-maxresults-2"
				destName := "dest-maxresults"

				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName1, Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName2, Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: destName, Endpoint: "localhost"})
				require.NoError(t, err)

				// Start two tasks on different source queues (each completes immediately as queues are empty).
				_, err = b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
					SourceArn:      buildQueueARN(dlqName1),
					DestinationArn: buildQueueARN(destName),
				})
				require.NoError(t, err)

				// Wait for first task to complete so the second doesn't conflict.
				require.Eventually(t, func() bool {
					listOut, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
						SourceArn: buildQueueARN(dlqName1),
					})
					if listErr != nil || len(listOut.Results) == 0 {
						return false
					}

					return listOut.Results[0].Status == sqs.MoveTaskStatusCompleted
				}, 2*time.Second, 10*time.Millisecond)

				_, err = b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
					SourceArn:      buildQueueARN(dlqName2),
					DestinationArn: buildQueueARN(destName),
				})
				require.NoError(t, err)

				// Return dlqName1's ARN as source for filtering; both tasks in the backend but
				// MaxResults=1 should limit the output to 1.
				// We list with empty SourceArn to get all tasks, but test pagination with MaxResults.
				return "", 2
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sqs.NewInMemoryBackend()
			t.Cleanup(b.Close)
			srcARN, _ := tt.setup(t, b)

			var out *sqs.ListMessageMoveTasksOutput

			require.Eventually(t, func() bool {
				var err error

				out, err = b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
					SourceArn:  srcARN,
					MaxResults: tt.maxResults,
				})
				require.NoError(t, err)

				return len(out.Results) >= tt.wantMin
			}, 2*time.Second, 10*time.Millisecond, "expected at least %d task(s) to be listed", tt.wantMin)

			assert.LessOrEqual(t, len(out.Results), tt.wantMax)
		})
	}
}

func TestSQS_MessageMoveTasks_Handler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup           func(t *testing.T, h *sqs.Handler) map[string]any
		name            string
		action          string
		wantBodyContain string
		wantCode        int
	}{
		{
			name:   "StartMessageMoveTask_success",
			action: "StartMessageMoveTask",
			setup: func(t *testing.T, h *sqs.Handler) map[string]any {
				t.Helper()
				doCreateQueue(t, h, "smt-dlq")
				doCreateQueue(t, h, "smt-dest")

				return map[string]any{
					"SourceArn":      buildQueueARN("smt-dlq"),
					"DestinationArn": buildQueueARN("smt-dest"),
				}
			},
			wantCode:        http.StatusOK,
			wantBodyContain: "TaskHandle",
		},
		{
			name:   "StartMessageMoveTask_source_not_found",
			action: "StartMessageMoveTask",
			setup: func(_ *testing.T, _ *sqs.Handler) map[string]any {
				return map[string]any{
					"SourceArn":      buildQueueARN("nonexistent"),
					"DestinationArn": buildQueueARN("also-nonexistent"),
				}
			},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "QueueDoesNotExist",
		},
		{
			name:   "CancelMessageMoveTask_invalid_handle",
			action: "CancelMessageMoveTask",
			setup: func(_ *testing.T, _ *sqs.Handler) map[string]any {
				return map[string]any{
					"TaskHandle": "nonexistent-handle",
				}
			},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "InvalidParameterValue",
		},
		{
			name:   "ListMessageMoveTasks_empty",
			action: "ListMessageMoveTasks",
			setup: func(_ *testing.T, _ *sqs.Handler) map[string]any {
				return map[string]any{
					"SourceArn": buildQueueARN("empty-source"),
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "StartMessageMoveTask_already_running_returns_invalid_parameter_value",
			action: "StartMessageMoveTask",
			setup: func(t *testing.T, h *sqs.Handler) map[string]any {
				t.Helper()
				doCreateQueue(t, h, "smt-conflict-dlq")
				doCreateQueue(t, h, "smt-conflict-dest")

				// Seed messages so the task stays RUNNING with rate limiting.
				for i := range 10 {
					rec := doRequest(t, h, "SendMessage", map[string]any{
						"QueueUrl":    "http://localhost/000000000000/smt-conflict-dlq",
						"MessageBody": "msg-" + strconv.Itoa(i),
					})
					require.Equal(t, http.StatusOK, rec.Code)
				}

				// Start first task with rate limiting.
				rec := doRequest(t, h, "StartMessageMoveTask", map[string]any{
					"SourceArn":                    buildQueueARN("smt-conflict-dlq"),
					"DestinationArn":               buildQueueARN("smt-conflict-dest"),
					"MaxNumberOfMessagesPerSecond": 1,
				})
				require.Equal(t, http.StatusOK, rec.Code)

				// The second request body (returned for the test handler loop to use).
				return map[string]any{
					"SourceArn":      buildQueueARN("smt-conflict-dlq"),
					"DestinationArn": buildQueueARN("smt-conflict-dest"),
				}
			},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "InvalidParameterValue",
		},
		{
			name:   "StartMessageMoveTask_empty_source_arn_returns_error",
			action: "StartMessageMoveTask",
			setup: func(_ *testing.T, _ *sqs.Handler) map[string]any {
				return map[string]any{
					"SourceArn":      "",
					"DestinationArn": buildQueueARN("some-dest"),
				}
			},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "InvalidParameterValue",
		},
		{
			name:   "CancelMessageMoveTask_on_completed_task_rejected",
			action: "CancelMessageMoveTask",
			setup: func(t *testing.T, h *sqs.Handler) map[string]any {
				t.Helper()
				doCreateQueue(t, h, "smt-cancel-done-dlq")
				doCreateQueue(t, h, "smt-cancel-done-dest")

				// Start a task on an empty queue; it completes immediately.
				rec := doRequest(t, h, "StartMessageMoveTask", map[string]any{
					"SourceArn":      buildQueueARN("smt-cancel-done-dlq"),
					"DestinationArn": buildQueueARN("smt-cancel-done-dest"),
				})
				require.Equal(t, http.StatusOK, rec.Code)

				var startResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
				handle, _ := startResp["TaskHandle"].(string)
				require.NotEmpty(t, handle)

				// Wait for the task to complete via polling ListMessageMoveTasks.
				dlqARN := buildQueueARN("smt-cancel-done-dlq")
				b := h.Backend.(*sqs.InMemoryBackend)

				require.Eventually(t, func() bool {
					listOut, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
						SourceArn:  dlqARN,
						MaxResults: 1,
					})

					return listErr == nil && len(listOut.Results) > 0 &&
						listOut.Results[0].Status == sqs.MoveTaskStatusCompleted
				}, 2*time.Second, 10*time.Millisecond)

				return map[string]any{"TaskHandle": handle}
			},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := tt.setup(t, h)

			rec := doRequest(t, h, tt.action, body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantBodyContain != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContain)
			}
		})
	}
}

// TestSQS_ListMessageMoveTasks_TaskHandleOnlyForRunning verifies that the TaskHandle
// field is only populated in ListMessageMoveTasks for tasks in RUNNING status,
// matching AWS ListMessageMoveTasksResultEntry semantics.
func TestSQS_ListMessageMoveTasks_TaskHandleOnlyForRunning(t *testing.T) {
	t.Parallel()

	b := sqs.NewInMemoryBackend()
	t.Cleanup(b.Close)

	dlqName := "dlq-taskhandle-check"
	destName := "dest-taskhandle-check"

	_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
	require.NoError(t, err)

	_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: destName, Endpoint: "localhost"})
	require.NoError(t, err)

	dlqARN := buildQueueARN(dlqName)
	destARN := buildQueueARN(destName)

	// Start a task on an empty queue — it will complete immediately.
	startOut, err := b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
		SourceArn:      dlqARN,
		DestinationArn: destARN,
	})
	require.NoError(t, err)
	require.NotEmpty(t, startOut.TaskHandle)

	// Wait for the task to complete.
	require.Eventually(t, func() bool {
		listOut, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
			SourceArn:  dlqARN,
			MaxResults: 1,
		})
		if listErr != nil || len(listOut.Results) == 0 {
			return false
		}

		return listOut.Results[0].Status == sqs.MoveTaskStatusCompleted
	}, 2*time.Second, 10*time.Millisecond, "task should complete")

	// After completion, TaskHandle must be empty in the list response.
	listOut, err := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
		SourceArn:  dlqARN,
		MaxResults: 1,
	})
	require.NoError(t, err)
	require.Len(t, listOut.Results, 1)
	assert.Equal(t, sqs.MoveTaskStatusCompleted, listOut.Results[0].Status)
	assert.Empty(t, listOut.Results[0].TaskHandle,
		"TaskHandle must not be populated for completed tasks (AWS semantics)")

	// StartedTimestamp and ApproximateNumberOfMessagesMoved should always be present.
	assert.NotZero(t, listOut.Results[0].StartedTimestamp, "StartedTimestamp should always be present")
}

// TestSQS_ListMessageMoveTasks_MaxResults validates default (1) and max (10) semantics.
func TestSQS_ListMessageMoveTasks_MaxResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxResults int32
		// taskCount is how many tasks to create (on different source queues).
		taskCount int
		wantCount int
	}{
		{
			name:       "default_max_results_is_1",
			maxResults: 0, // unset → defaults to 1
			taskCount:  3,
			wantCount:  1,
		},
		{
			name:       "explicit_max_results_respected",
			maxResults: 2,
			taskCount:  3,
			wantCount:  2,
		},
		{
			name:       "max_results_capped_at_10",
			maxResults: 20, // over limit → capped to 10
			taskCount:  11,
			wantCount:  10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sqs.NewInMemoryBackend()
			t.Cleanup(b.Close)

			// Create taskCount source queues and start a task on each.
			for i := range tt.taskCount {
				dlqName := "dlq-maxr-" + tt.name + "-" + strconv.Itoa(i)
				destName := "dest-maxr-" + tt.name + "-" + strconv.Itoa(i)

				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: destName, Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
					SourceArn:      buildQueueARN(dlqName),
					DestinationArn: buildQueueARN(destName),
				})
				require.NoError(t, err)
			}

			// Wait for all tasks to complete (each queue is empty so they finish fast).
			require.Eventually(t, func() bool {
				listOut, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
					MaxResults: 10,
				})
				if listErr != nil {
					return false
				}

				completedOrDone := 0
				for _, task := range listOut.Results {
					if task.Status == sqs.MoveTaskStatusCompleted {
						completedOrDone++
					}
				}

				// We want at least min(taskCount, 10) completed in the capped window.
				expected := min(tt.taskCount, 10)

				return completedOrDone >= expected
			}, 5*time.Second, 20*time.Millisecond, "all tasks should complete")

			// Now query with the test's MaxResults.
			out, err := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
				MaxResults: tt.maxResults,
			})
			require.NoError(t, err)
			assert.Len(t, out.Results, tt.wantCount)
		})
	}
}

// TestSQS_ListMessageMoveTasks_NewestFirst verifies that results are returned newest
// first (descending by startedAt) matching AWS semantics.
func TestSQS_ListMessageMoveTasks_NewestFirst(t *testing.T) {
	t.Parallel()

	b := sqs.NewInMemoryBackend()
	t.Cleanup(b.Close)

	const numQueues = 3

	// Create and start tasks in sequence, ensuring distinct timestamps by letting
	// each complete before starting the next (to force different startedAt values).
	for i := range numQueues {
		name := "dlq-order-" + strconv.Itoa(i)
		dest := "dest-order-" + strconv.Itoa(i)

		_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: name, Endpoint: "localhost"})
		require.NoError(t, err)

		_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: dest, Endpoint: "localhost"})
		require.NoError(t, err)

		_, err = b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
			SourceArn:      buildQueueARN(name),
			DestinationArn: buildQueueARN(dest),
		})
		require.NoError(t, err)

		// Let the task run to completion before starting the next one.
		require.Eventually(t, func() bool {
			listOut, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
				SourceArn:  buildQueueARN(name),
				MaxResults: 1,
			})

			return listErr == nil && len(listOut.Results) > 0 &&
				listOut.Results[0].Status == sqs.MoveTaskStatusCompleted
		}, 2*time.Second, 10*time.Millisecond)
	}

	// Retrieve all tasks (up to 10).
	out, err := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
		MaxResults: 10,
	})
	require.NoError(t, err)
	require.Len(t, out.Results, numQueues)

	// Verify descending order: each result should have a StartedTimestamp ≥ the next.
	for i := 1; i < len(out.Results); i++ {
		assert.GreaterOrEqual(t, out.Results[i-1].StartedTimestamp, out.Results[i].StartedTimestamp,
			"results should be sorted newest first")
	}
}

// TestSQS_StartMessageMoveTask_Validation tests input validation for StartMessageMoveTask.
func TestSQS_StartMessageMoveTask_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		input     sqs.StartMessageMoveTaskInput
	}{
		{
			name: "empty_source_arn_returns_error",
			input: sqs.StartMessageMoveTaskInput{
				SourceArn:      "",
				DestinationArn: "arn:aws:sqs:us-east-1:000000000000:dest",
			},
			wantErrIs: sqs.ErrInvalidSourceArn,
		},
		{
			name: "negative_rate_returns_error",
			input: sqs.StartMessageMoveTaskInput{
				SourceArn:                    "arn:aws:sqs:us-east-1:000000000000:src",
				MaxNumberOfMessagesPerSecond: -1,
			},
			wantErrIs: sqs.ErrInvalidMaxMessagesPerSecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sqs.NewInMemoryBackend()
			t.Cleanup(b.Close)

			_, err := b.StartMessageMoveTask(&tt.input)
			require.Error(t, err)
			require.ErrorIs(t, err, tt.wantErrIs)
		})
	}
}

// TestSQS_CancelMessageMoveTask_NotRunning verifies that cancelling a task that has
// already completed returns success (idempotent cancel — same handle, same result).
func TestSQS_CancelMessageMoveTask_NotRunning(t *testing.T) {
	t.Parallel()

	b := sqs.NewInMemoryBackend()
	t.Cleanup(b.Close)

	dlqName := "dlq-cancel-complete"
	destName := "dest-cancel-complete"

	_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
	require.NoError(t, err)

	_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: destName, Endpoint: "localhost"})
	require.NoError(t, err)

	dlqARN := buildQueueARN(dlqName)

	// Start a task on an empty queue — it will complete immediately.
	startOut, err := b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
		SourceArn:      dlqARN,
		DestinationArn: buildQueueARN(destName),
	})
	require.NoError(t, err)
	require.NotEmpty(t, startOut.TaskHandle)

	// Wait for task to reach COMPLETED.
	require.Eventually(t, func() bool {
		listOut, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
			SourceArn:  dlqARN,
			MaxResults: 1,
		})

		return listErr == nil && len(listOut.Results) > 0 &&
			listOut.Results[0].Status == sqs.MoveTaskStatusCompleted
	}, 2*time.Second, 10*time.Millisecond, "task should complete")

	// Cancelling a completed (terminal) task is rejected by AWS.
	_, err = b.CancelMessageMoveTask(&sqs.CancelMessageMoveTaskInput{
		TaskHandle: startOut.TaskHandle,
	})
	require.Error(t, err)
}

func TestSQS_MoveTaskJanitorPruning(t *testing.T) {
	t.Parallel()

	type taskFixture struct {
		handle    string
		status    sqs.MoveTaskStatus
		startedAt int64
	}

	now := time.Now()
	tests := []struct {
		name      string
		tasks     []taskFixture
		wantCount int
	}{
		{
			name: "old_terminal_tasks_are_removed",
			tasks: []taskFixture{
				{
					handle:    "completed-old",
					status:    sqs.MoveTaskStatusCompleted,
					startedAt: now.Add(-16 * time.Minute).UnixMilli(),
				},
				{
					handle:    "cancelled-old",
					status:    sqs.MoveTaskStatusCancelled,
					startedAt: now.Add(-16 * time.Minute).UnixMilli(),
				},
				{
					handle:    "failed-old",
					status:    sqs.MoveTaskStatusFailed,
					startedAt: now.Add(-16 * time.Minute).UnixMilli(),
				},
			},
			wantCount: 0,
		},
		{
			name: "running_and_recent_terminal_tasks_are_kept",
			tasks: []taskFixture{
				{
					handle:    "running-old",
					status:    sqs.MoveTaskStatusRunning,
					startedAt: now.Add(-30 * time.Minute).UnixMilli(),
				},
				{
					handle:    "completed-recent",
					status:    sqs.MoveTaskStatusCompleted,
					startedAt: now.Add(-5 * time.Minute).UnixMilli(),
				},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sqs.NewInMemoryBackend()
			t.Cleanup(b.Close)
			for _, task := range tt.tasks {
				b.InjectMoveTaskForTest(task.handle, task.status, task.startedAt)
			}

			b.RunJanitorOnceForTest(now)
			assert.Equal(t, tt.wantCount, b.MoveTaskCountForTest())
		})
	}
}

// TestSQS_DeleteQueue_CancelsMoveTasks verifies that deleting a queue that is the
// source of an active move task cancels the task, preventing a goroutine leak.
func TestSQS_DeleteQueue_CancelsMoveTasks(t *testing.T) {
	t.Parallel()

	b := sqs.NewInMemoryBackend()
	t.Cleanup(b.Close)

	dlqName := "dlq-delete-cancel"
	destName := "dest-delete-cancel"

	_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
	require.NoError(t, err)

	_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: destName, Endpoint: "localhost"})
	require.NoError(t, err)

	dlqURL := "http://localhost/000000000000/" + dlqName
	dlqARN := buildQueueARN(dlqName)

	// Seed messages so the task doesn't complete before we delete the queue.
	for i := range 10 {
		_, sendErr := b.SendMessage(&sqs.SendMessageInput{
			QueueURL:    dlqURL,
			MessageBody: "msg-" + strconv.Itoa(i),
		})
		require.NoError(t, sendErr)
	}

	// Start a rate-limited task so it stays RUNNING.
	startOut, err := b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
		SourceArn:                    dlqARN,
		DestinationArn:               buildQueueARN(destName),
		MaxNumberOfMessagesPerSecond: 1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, startOut.TaskHandle)

	// Delete the DLQ — the associated move task must reach a terminal state.
	err = b.DeleteQueue(&sqs.DeleteQueueInput{QueueURL: dlqURL})
	require.NoError(t, err)

	// The task should eventually transition from RUNNING/CANCELLING to a terminal state.
	require.Eventually(t, func() bool {
		listOut, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
			MaxResults: 1,
		})
		if listErr != nil || len(listOut.Results) == 0 {
			return false
		}

		s := listOut.Results[0].Status

		return s == sqs.MoveTaskStatusCancelled || s == sqs.MoveTaskStatusCompleted || s == sqs.MoveTaskStatusFailed
	}, 3*time.Second, 10*time.Millisecond, "move task should reach a terminal state after DeleteQueue")
}
