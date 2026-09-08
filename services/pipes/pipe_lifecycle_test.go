package pipes_test

// Covers the pipe resource lifecycle: CREATING/UPDATING/DELETING/STARTING/
// STOPPING state transitions, CRUD error paths, and cross-cutting pipe-level
// attributes (KmsKeyIdentifier, LogConfiguration, ARN format, timestamps).
// DeadLetterConfig round-tripping is covered in sources_test.go, nested under
// the source parameters (its only real-API location).

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/pipes"
)

// --- lifecycle tests ---

// TestLifecycle_Creating verifies that CreatePipe returns CREATING state initially.
func TestLifecycle_Creating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		desiredState string
		wantCreating string
	}{
		{
			name:         "desired_running_starts_creating",
			desiredState: "RUNNING",
			wantCreating: "CREATING",
		},
		{
			name:         "desired_stopped_starts_creating",
			desiredState: "STOPPED",
			wantCreating: "CREATING",
		},
		{
			name:         "empty_desired_defaults_to_running_via_creating",
			desiredState: "",
			wantCreating: "CREATING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := auditNewBackend()
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":  "arn:aws:lambda:us-west-2:123456789012:function:fn",
			}
			if tt.desiredState != "" {
				body["DesiredState"] = tt.desiredState
			}
			h := pipes.NewHandler(b)
			resp := auditCreate(t, h, tt.name+"-pipe", body)

			assert.Equal(t, tt.wantCreating, resp["CurrentState"], "pipe should start in CREATING state")
		})
	}
}

// TestLifecycle_CreatingToRunning verifies CREATING → RUNNING transition.
func TestLifecycle_CreatingToRunning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		desiredState      string
		wantEventualState string
	}{
		{
			name:              "running_desired_transitions_to_running",
			desiredState:      "RUNNING",
			wantEventualState: "RUNNING",
		},
		{
			name:              "stopped_desired_transitions_to_stopped",
			desiredState:      "STOPPED",
			wantEventualState: "STOPPED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := auditNewBackend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN:      "arn:aws:iam::123456789012:role/r",
				Name:         tt.name,
				Source:       "arn:aws:sqs:us-west-2:123456789012:q",
				Target:       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				DesiredState: tt.desiredState,
			})
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				p, getErr := b.GetPipe(context.Background(), tt.name)

				return getErr == nil && p.CurrentState == tt.wantEventualState
			}, 500*time.Millisecond, 5*time.Millisecond)
		})
	}
}

// TestLifecycle_Updating verifies that UpdatePipe passes through UPDATING state.
func TestLifecycle_Updating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		description       string
		wantEventualState string
	}{
		{
			name:              "update_description_transitions_updating_running",
			description:       "updated desc",
			wantEventualState: "RUNNING",
		},
		{
			name:              "update_desired_stopped_transitions_updating_stopped",
			description:       "stop it",
			wantEventualState: "STOPPED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := auditNewBackend()
			pipeName := tt.name + "-pipe"
			desiredState := "RUNNING"
			if tt.wantEventualState == "STOPPED" {
				desiredState = "STOPPED"
			}
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN:      "arn:aws:iam::123456789012:role/r",
				Name:         pipeName,
				Source:       "arn:aws:sqs:us-west-2:123456789012:q",
				Target:       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				DesiredState: "RUNNING",
			})
			require.NoError(t, err)
			pipes.WaitPipeRunning(t, b, pipeName)

			desc := tt.description
			updated, err := b.UpdatePipe(context.Background(), pipeName, pipes.UpdatePipeInput{
				RoleARN:      "arn:aws:iam::123456789012:role/r",
				Description:  &desc,
				DesiredState: desiredState,
			})
			require.NoError(t, err)
			assert.Equal(t, "UPDATING", updated.CurrentState, "UpdatePipe should return UPDATING state")

			require.Eventually(t, func() bool {
				p, e := b.GetPipe(context.Background(), pipeName)

				return e == nil && p.CurrentState == tt.wantEventualState
			}, 500*time.Millisecond, 5*time.Millisecond)
		})
	}
}

// TestLifecycle_Deleting verifies that DeletePipe returns DELETING state and then removes the pipe.
func TestLifecycle_Deleting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "delete_returns_deleting_state"},
		{name: "pipe_removed_after_deleting"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := auditNewBackend()
			pipeName := tt.name + "-pipe"
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN:      "arn:aws:iam::123456789012:role/r",
				Name:         pipeName,
				Source:       "arn:aws:sqs:us-west-2:123456789012:q",
				Target:       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				DesiredState: "RUNNING",
			})
			require.NoError(t, err)

			deleted, err := b.DeletePipe(context.Background(), pipeName)
			require.NoError(t, err)
			assert.Equal(t, "DELETING", deleted.CurrentState, "DeletePipe should return DELETING state")

			require.Eventually(t, func() bool {
				_, e := b.GetPipe(context.Background(), pipeName)

				return e != nil
			}, 500*time.Millisecond, 5*time.Millisecond, "pipe should be removed after DELETING transition")
		})
	}
}

// TestLifecycle_DeleteReturnsBody verifies HTTP delete returns pipe body in DELETING state.
func TestLifecycle_DeleteReturnsBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pipeName string
	}{
		{name: "delete_http_returns_deleting_body", pipeName: "http-del-pipe"},
		{name: "delete_http_includes_arn", pipeName: "http-del-arn-pipe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			auditCreate(t, h, tt.pipeName, map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":  "arn:aws:lambda:us-west-2:123456789012:function:fn",
			})

			rec := auditDo(t, h, http.MethodDelete, "/v1/pipes/"+tt.pipeName, nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "DELETING", resp["CurrentState"])
			assert.Equal(t, tt.pipeName, resp["Name"])
			assert.NotEmpty(t, resp["Arn"])
		})
	}
}

// TestLifecycle_StartStop verifies start/stop transitions with ECS params.
func TestLifecycle_StartStop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "start-stop-ecs-a"},
		{name: "start-stop-ecs-b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN:      "arn:aws:iam::123456789012:role/r",
				Name:         tt.name,
				Source:       b2SQSSource,
				Target:       b2ECSTarget,
				DesiredState: "RUNNING",
				TargetParameters: &pipes.TargetParameters{
					EcsTaskParameters: &pipes.ECSTaskTargetParameters{
						TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
						LaunchType:        "FARGATE",
						NetworkConfiguration: &pipes.NetworkConfiguration{
							AwsvpcConfiguration: &pipes.AwsVpcConfiguration{
								Subnets: []string{"subnet-aaa"},
							},
						},
					},
				},
			})
			require.NoError(t, err)
			pipes.WaitPipeRunning(t, b, tt.name)

			stopped, err := b.StopPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t, "STOPPING", stopped.CurrentState)

			require.Eventually(t, func() bool {
				p, e := b.GetPipe(context.Background(), tt.name)

				return e == nil && p.CurrentState == "STOPPED"
			}, 500*time.Millisecond, 5*time.Millisecond)

			started, err := b.StartPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t, "STARTING", started.CurrentState)

			require.Eventually(t, func() bool {
				p, e := b.GetPipe(context.Background(), tt.name)

				return e == nil && p.CurrentState == "RUNNING"
			}, 500*time.Millisecond, 5*time.Millisecond)

			p, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			ecs := p.TargetParameters.EcsTaskParameters
			require.NotNil(t, ecs.NetworkConfiguration)
			assert.Equal(t, "subnet-aaa",
				ecs.NetworkConfiguration.AwsvpcConfiguration.Subnets[0])
		})
	}
}

// TestLifecycle_Delete verifies new params survive until deletion.
func TestLifecycle_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "delete-with-batch-params-a"},
		{name: "delete-with-batch-params-b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  b2SQSSource,
				Target:  "arn:aws:batch:us-east-1:123456789012:job-queue/q",
				TargetParameters: &pipes.TargetParameters{
					BatchJobParameters: &pipes.BatchJobTargetParameters{
						JobDefinition: "jd",
						JobName:       "job",
						DependsOn: []pipes.BatchJobDependency{
							{JobID: "parent-job", Type: "SEQUENTIAL"},
						},
					},
				},
			})
			require.NoError(t, err)

			deleted, err := b.DeletePipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t, "DELETING", deleted.CurrentState)
			assert.Equal(t, "parent-job",
				deleted.TargetParameters.BatchJobParameters.DependsOn[0].JobID)

			require.Eventually(t, func() bool {
				_, e := b.GetPipe(context.Background(), tt.name)

				return e != nil
			}, 500*time.Millisecond, 5*time.Millisecond)
		})
	}
}

// TestPipeStateTransitions verifies STARTING and STOPPING intermediate states.
func TestPipeStateTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		initialState      string
		action            string // "start" or "stop"
		wantImmediate     string // state immediately after the call
		wantEventualFinal string // state after async transition completes
	}{
		{
			name:              "stop_pipe_goes_through_stopping",
			initialState:      "RUNNING",
			action:            "stop",
			wantImmediate:     "STOPPING",
			wantEventualFinal: "STOPPED",
		},
		{
			name:              "start_stopped_pipe_goes_through_starting",
			initialState:      "STOPPED",
			action:            "start",
			wantImmediate:     "STARTING",
			wantEventualFinal: "RUNNING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newPipeBackend()
			pipeName := "transition-" + tt.name

			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN:      "arn:aws:iam::123456789012:role/r",
				Name:         pipeName,
				Source:       "arn:aws:sqs:us-east-1:000000000000:queue",
				Target:       "arn:aws:lambda:us-east-1:000000000000:function:fn",
				DesiredState: tt.initialState,
			})
			require.NoError(t, err)

			// Perform the action.
			var result *pipes.Pipe
			switch tt.action {
			case "stop":
				result, err = b.StopPipe(context.Background(), pipeName)
			case "start":
				result, err = b.StartPipe(context.Background(), pipeName)
			}
			require.NoError(t, err)

			// Verify intermediate state in the synchronous return value.
			assert.Equal(t, tt.wantImmediate, result.CurrentState,
				"expected intermediate state %q", tt.wantImmediate)

			// Wait for the async transition to complete.
			require.Eventually(t, func() bool {
				p, e := b.GetPipe(context.Background(), pipeName)

				return e == nil && p.CurrentState == tt.wantEventualFinal
			}, 2*time.Second, 10*time.Millisecond,
				"timed out waiting for pipe to reach %q", tt.wantEventualFinal)
		})
	}
}

// TestPipeStateTransitions_DoubleStart verifies that starting an already-RUNNING pipe errors.
func TestPipeStateTransitions_DoubleStart(t *testing.T) {
	t.Parallel()

	b := newPipeBackend()
	_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
		RoleARN:      "arn:aws:iam::123456789012:role/r",
		Name:         "double-start",
		Source:       "arn:aws:sqs:us-east-1:000000000000:queue",
		Target:       "arn:aws:lambda:us-east-1:000000000000:function:fn",
		DesiredState: "RUNNING",
	})
	require.NoError(t, err)

	// Should fail since pipe is already RUNNING — real AWS returns ConflictException.
	_, err = b.StartPipe(context.Background(), "double-start")
	require.Error(t, err)
	require.ErrorIs(t, err, pipes.ErrConflict)
}

// --- error path tests ---

// TestErrors verifies error conditions return correct HTTP status codes.
func TestErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		setup      func(t *testing.T, h *pipes.Handler)
		name       string
		method     string
		path       string
		wantType   string
		wantStatus int
	}{
		{
			name:   "create_duplicate_pipe_returns_409",
			method: http.MethodPost,
			path:   "/v1/pipes/dup-pipe",
			body: map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":  "arn:aws:lambda:us-west-2:123456789012:function:fn",
			},
			wantStatus: http.StatusConflict,
			wantType:   "ConflictException",
			setup: func(t *testing.T, h *pipes.Handler) {
				t.Helper()
				auditCreate(t, h, "dup-pipe", map[string]any{
					"RoleArn": "arn:aws:iam::123456789012:role/r",
					"Source":  "arn:aws:sqs:us-west-2:123456789012:q",
					"Target":  "arn:aws:lambda:us-west-2:123456789012:function:fn",
				})
			},
		},
		{
			name:       "describe_nonexistent_pipe_returns_404",
			method:     http.MethodGet,
			path:       "/v1/pipes/no-such-pipe",
			wantStatus: http.StatusNotFound,
			wantType:   "NotFoundException",
		},
		{
			name:       "delete_nonexistent_pipe_returns_404",
			method:     http.MethodDelete,
			path:       "/v1/pipes/gone-pipe",
			wantStatus: http.StatusNotFound,
			wantType:   "NotFoundException",
		},
		{
			name:   "update_nonexistent_pipe_returns_404",
			method: http.MethodPut,
			path:   "/v1/pipes/missing-pipe",
			body: map[string]any{
				"RoleArn":     "arn:aws:iam::123456789012:role/r",
				"Description": "x",
			},
			wantStatus: http.StatusNotFound,
			wantType:   "NotFoundException",
		},
		{
			name:   "create_with_invalid_desired_state_returns_400",
			method: http.MethodPost,
			path:   "/v1/pipes/bad-state-pipe",
			body: map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "INVALID",
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "ValidationException",
		},
		{
			// Real AWS returns ConflictException (409), not ValidationException (400).
			name:       "start_already_desired_running_returns_400",
			method:     http.MethodPost,
			path:       "/v1/pipes/already-running-pipe/start",
			wantStatus: http.StatusConflict,
			wantType:   "ConflictException",
			setup: func(t *testing.T, h *pipes.Handler) {
				t.Helper()
				auditCreate(t, h, "already-running-pipe", map[string]any{
					"RoleArn":      "arn:aws:iam::123456789012:role/r",
					"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
					"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
					"DesiredState": "RUNNING",
				})
			},
		},
		{
			// Real AWS returns ConflictException (409), not ValidationException (400).
			name:       "stop_already_desired_stopped_returns_400",
			method:     http.MethodPost,
			path:       "/v1/pipes/already-stopped-pipe/stop",
			wantStatus: http.StatusConflict,
			wantType:   "ConflictException",
			setup: func(t *testing.T, h *pipes.Handler) {
				t.Helper()
				auditCreate(t, h, "already-stopped-pipe", map[string]any{
					"RoleArn":      "arn:aws:iam::123456789012:role/r",
					"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
					"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
					"DesiredState": "STOPPED",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := auditDo(t, h, tt.method, tt.path, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantType != "" {
				assert.Contains(t, rec.Body.String(), tt.wantType)
			}
		})
	}
}

// TestError_DuplicatePipe verifies duplicate create returns conflict.
func TestError_DuplicatePipe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "dup-pipe-a"},
		{name: "dup-pipe-b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  b2SQSSource,
				"Target":  b2LambdaTarget,
			}
			b2Create(t, h, tt.name, body)
			rec := b2Do(t, h, http.MethodPost, "/v1/pipes/"+tt.name, body)
			assert.Equal(t, http.StatusConflict, rec.Code)
		})
	}
}

// TestError_NotFound verifies describe returns 404 for unknown pipe.
func TestError_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "does-not-exist-a"},
		{name: "does-not-exist-b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			rec := b2Do(t, h, http.MethodGet, "/v1/pipes/"+tt.name, nil)
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

// TestError_InvalidDesiredState verifies validation error for invalid state.
func TestError_InvalidDesiredState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		desiredState string
	}{
		{name: "invalid_state_a", desiredState: "INVALID"},
		{name: "invalid_state_b", desiredState: "STARTING"},
		{name: "invalid_state_c", desiredState: "CREATING"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       b2SQSSource,
				"Target":       b2LambdaTarget,
				"DesiredState": tt.desiredState,
			}
			rec := b2Do(t, h, http.MethodPost, "/v1/pipes/"+tt.name, body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestMarkPipeFailed verifies MarkPipeFailed sets state and reason.
func TestMarkPipeFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		failState  string
		failReason string
	}{
		{
			name:       "create_failed",
			failState:  "CREATE_FAILED",
			failReason: "IAM role not authorized",
		},
		{
			name:       "update_failed",
			failState:  "UPDATE_FAILED",
			failReason: "Target ARN is invalid",
		},
		{
			name:       "start_failed",
			failState:  "START_FAILED",
			failReason: "Failed to connect to source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := auditNewBackend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN:      "arn:aws:iam::123456789012:role/r",
				Name:         tt.name + "-pipe",
				Source:       "arn:aws:sqs:us-west-2:123456789012:q",
				Target:       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				DesiredState: "RUNNING",
			})
			require.NoError(t, err)

			b.MarkPipeFailed(tt.name+"-pipe", tt.failState, tt.failReason)

			p, err := b.GetPipe(context.Background(), tt.name+"-pipe")
			require.NoError(t, err)
			assert.Equal(t, tt.failState, p.CurrentState)
			assert.Equal(t, tt.failReason, p.StateReason)
		})
	}
}

// TestARN_Format verifies that created pipe ARNs have the expected format.
func TestARN_Format(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pipeName  string
		wantInARN string
	}{
		{name: "arn_contains_pipe_name", pipeName: "my-pipe", wantInARN: "pipe/my-pipe"},
		{name: "arn_contains_region", pipeName: "region-pipe", wantInARN: "us-west-2"},
		{name: "arn_contains_account", pipeName: "acct-pipe", wantInARN: "123456789012"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			resp := auditCreate(t, h, tt.pipeName, map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
			})

			arn := resp["Arn"].(string)
			assert.Contains(t, arn, tt.wantInARN)
		})
	}
}

// TestTimestamps_SetOnCreate verifies CreationTime and LastModifiedTime are set.
func TestTimestamps_SetOnCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "timestamps_present"},
		{name: "timestamps_nonzero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			resp := auditCreate(t, h, tt.name+"-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
			})

			ct, _ := resp["CreationTime"].(float64)
			lmt, _ := resp["LastModifiedTime"].(float64)
			assert.Greater(t, ct, float64(0), "CreationTime should be nonzero")
			assert.Greater(t, lmt, float64(0), "LastModifiedTime should be nonzero")
			assert.InDelta(t, ct, lmt, 1.0, "CreationTime and LastModifiedTime should be close on creation")
		})
	}
}

// TestUpdatePipe_UpdatesLastModifiedTime verifies LastModifiedTime increases on update.
func TestUpdatePipe_UpdatesLastModifiedTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "update_bumps_modified_time"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := auditNewBackend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN:      "arn:aws:iam::123456789012:role/r",
				Name:         tt.name + "-pipe",
				Source:       "arn:aws:sqs:us-west-2:123456789012:q",
				Target:       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				DesiredState: "RUNNING",
			})
			require.NoError(t, err)
			pipes.WaitPipeRunning(t, b, tt.name+"-pipe")

			before, _ := b.GetPipe(context.Background(), tt.name+"-pipe")
			time.Sleep(2 * time.Millisecond)

			updatedDesc := "updated"
			_, err = b.UpdatePipe(context.Background(), tt.name+"-pipe", pipes.UpdatePipeInput{
				RoleARN:     "arn:aws:iam::123456789012:role/r",
				Description: &updatedDesc,
			})
			require.NoError(t, err)

			after, _ := b.GetPipe(context.Background(), tt.name+"-pipe")
			assert.True(t, after.LastModifiedTime.After(before.LastModifiedTime),
				"LastModifiedTime should increase after update")
		})
	}
}

// --- UpdatePipe Description pointer semantics ---

// TestUpdatePipe_Description_AbsentMeansUnchanged verifies that not sending
// Description in an update leaves the existing value intact.
func TestUpdatePipe_Description_AbsentMeansUnchanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		initialDesc string
		updateDesc  string
		wantDesc    string
	}{
		{
			name:        "empty_description_clears",
			initialDesc: "original description",
			updateDesc:  "",
			wantDesc:    "",
		},
		{
			name:        "non_empty_description_updates",
			initialDesc: "old",
			updateDesc:  "new description",
			wantDesc:    "new description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b3Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				Name:         tt.name,
				RoleARN:      "arn:aws:iam::111122223333:role/r",
				Source:       b3SQSSource,
				Target:       b3LambdaTarget,
				Description:  tt.initialDesc,
				DesiredState: "RUNNING",
			})
			require.NoError(t, err)
			pipes.WaitPipeRunning(t, b, tt.name)

			desc := tt.updateDesc
			_, err = b.UpdatePipe(context.Background(), tt.name, pipes.UpdatePipeInput{
				RoleARN:     "arn:aws:iam::123456789012:role/r",
				Description: &desc,
			})
			require.NoError(t, err)

			p, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDesc, p.Description)
		})
	}
}

// TestUpdatePipe_Description_HTTPAbsentPreserves verifies that omitting
// Description from the HTTP body leaves it unchanged, matching AWS semantics.
func TestUpdatePipe_Description_HTTPAbsentPreserves(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     map[string]any
		wantDesc string
	}{
		{
			name: "no_description_field_preserves",
			body: map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"DesiredState": "RUNNING",
			},
			wantDesc: "keep me",
		},
		{
			// JSON "" → *string pointing to "" → backend clears; response omits field (omitempty).
			name: "empty_string_clears_description",
			body: map[string]any{
				"RoleArn":     "arn:aws:iam::123456789012:role/r",
				"Description": "",
			},
			wantDesc: "",
		},
		{
			name: "new_value_updates_description",
			body: map[string]any{
				"RoleArn":     "arn:aws:iam::123456789012:role/r",
				"Description": "new desc",
			},
			wantDesc: "new desc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b3Handler(t)
			b3Create(t, h, tt.name+"-pipe", map[string]any{
				"RoleArn":     "arn:aws:iam::123456789012:role/r",
				"Source":      b3SQSSource,
				"Target":      b3LambdaTarget,
				"Description": "keep me",
			})

			rec := b3Do(t, h, http.MethodPut, "/v1/pipes/"+tt.name+"-pipe", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := b3Describe(t, h, tt.name+"-pipe")
			// Response uses omitempty so empty string → absent key (nil).
			gotDesc, _ := resp["Description"].(string)
			assert.Equal(t, tt.wantDesc, gotDesc,
				"Description should match expected value after update")
		})
	}
}

// --- KmsKeyIdentifier tests ---

// TestKmsKeyIdentifier verifies that KmsKeyIdentifier is stored and returned.
func TestKmsKeyIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kmsKey  string
		wantKey string
	}{
		{
			name:    "no_kms_key",
			kmsKey:  "",
			wantKey: "",
		},
		{
			name:    "arn_kms_key",
			kmsKey:  "arn:aws:kms:us-west-2:123456789012:key/key-id",
			wantKey: "arn:aws:kms:us-west-2:123456789012:key/key-id",
		},
		{
			name:    "alias_kms_key",
			kmsKey:  "alias/my-pipe-key",
			wantKey: "alias/my-pipe-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			body := map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
			}
			if tt.kmsKey != "" {
				body["KmsKeyIdentifier"] = tt.kmsKey
			}

			resp := auditCreate(t, h, tt.name+"-pipe", body)

			if tt.wantKey != "" {
				assert.Equal(t, tt.wantKey, resp["KmsKeyIdentifier"])
			} else {
				_, hasKey := resp["KmsKeyIdentifier"]
				assert.False(t, hasKey, "KmsKeyIdentifier should not be present when not set")
			}
		})
	}
}

// TestKmsKeyIdentifier_Update verifies that KmsKeyIdentifier can be updated.
func TestKmsKeyIdentifier_Update(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initialKey string
		updatedKey string
	}{
		{
			name:       "add_key_on_update",
			initialKey: "",
			updatedKey: "arn:aws:kms:us-west-2:123456789012:key/new-key",
		},
		{
			name:       "update_existing_key",
			initialKey: "alias/old-key",
			updatedKey: "alias/new-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := auditNewBackend()
			inp := pipes.CreatePipeInput{
				RoleARN:      "arn:aws:iam::123456789012:role/r",
				Name:         tt.name + "-pipe",
				Source:       "arn:aws:sqs:us-west-2:123456789012:q",
				Target:       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				DesiredState: "RUNNING",
			}
			if tt.initialKey != "" {
				inp.KmsKeyIdentifier = tt.initialKey
			}
			_, err := b.CreatePipe(context.Background(), inp)
			require.NoError(t, err)
			pipes.WaitPipeRunning(t, b, tt.name+"-pipe")

			updated, err := b.UpdatePipe(context.Background(), tt.name+"-pipe", pipes.UpdatePipeInput{
				RoleARN:          "arn:aws:iam::123456789012:role/r",
				KmsKeyIdentifier: aws.String(tt.updatedKey),
			})
			require.NoError(t, err)
			assert.Equal(t, tt.updatedKey, updated.KmsKeyIdentifier)
		})
	}
}

// --- log configuration tests ---

// TestLogConfiguration verifies log configuration round-trip for all destination types.
func TestLogConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		logLevel              string
		cloudwatchLogGroupArn string
		firehoseStreamArn     string
		s3BucketName          string
		s3Prefix              string
		includeExecutionData  []string
	}{
		{
			name:                  "cloudwatch_logs",
			logLevel:              "ERROR",
			cloudwatchLogGroupArn: "arn:aws:logs:us-west-2:123456789012:log-group:/pipes/logs",
			includeExecutionData:  []string{"ALL"},
		},
		{
			name:              "firehose_destination",
			logLevel:          "INFO",
			firehoseStreamArn: "arn:aws:firehose:us-west-2:123456789012:deliverystream/pipe-logs",
		},
		{
			name:         "s3_destination",
			logLevel:     "TRACE",
			s3BucketName: "my-pipe-logs",
			s3Prefix:     "pipes/",
		},
		{
			name:                  "all_destinations",
			logLevel:              "ERROR",
			cloudwatchLogGroupArn: "arn:aws:logs:us-west-2:123456789012:log-group:/pipes/all",
			firehoseStreamArn:     "arn:aws:firehose:us-west-2:123456789012:deliverystream/pipe-all",
			s3BucketName:          "pipe-log-bucket",
			includeExecutionData:  []string{"ALL"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			logConfig := map[string]any{"Level": tt.logLevel}

			if tt.cloudwatchLogGroupArn != "" {
				logConfig["CloudwatchLogsLogDestination"] = map[string]any{
					"LogGroupArn": tt.cloudwatchLogGroupArn,
				}
			}
			if tt.firehoseStreamArn != "" {
				logConfig["FirehoseLogDestination"] = map[string]any{
					"DeliveryStreamArn": tt.firehoseStreamArn,
				}
			}
			if tt.s3BucketName != "" {
				dest := map[string]any{"BucketName": tt.s3BucketName}
				if tt.s3Prefix != "" {
					dest["Prefix"] = tt.s3Prefix
				}
				logConfig["S3LogDestination"] = dest
			}
			if len(tt.includeExecutionData) > 0 {
				logConfig["IncludeExecutionData"] = tt.includeExecutionData
			}

			resp := auditCreate(t, h, tt.name+"-pipe", map[string]any{
				"RoleArn":          "arn:aws:iam::123456789012:role/r",
				"Source":           "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":           "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState":     "RUNNING",
				"LogConfiguration": logConfig,
			})

			lc, _ := resp["LogConfiguration"].(map[string]any)
			require.NotNil(t, lc, "LogConfiguration missing")
			assert.Equal(t, tt.logLevel, lc["Level"])

			if tt.cloudwatchLogGroupArn != "" {
				cwl, ok := lc["CloudwatchLogsLogDestination"].(map[string]any)
				require.True(t, ok, "CloudWatch log destination not found")
				assert.Equal(t, tt.cloudwatchLogGroupArn, cwl["LogGroupArn"])
			}

			if tt.firehoseStreamArn != "" {
				fh, ok := lc["FirehoseLogDestination"].(map[string]any)
				require.True(t, ok, "Firehose log destination not found")
				assert.Equal(t, tt.firehoseStreamArn, fh["DeliveryStreamArn"])
			}

			if tt.s3BucketName != "" {
				s3, ok := lc["S3LogDestination"].(map[string]any)
				require.True(t, ok, "S3 log destination not found")
				assert.Equal(t, tt.s3BucketName, s3["BucketName"])
			}
		})
	}
}

// TestLogConfigurationLevelRequired verifies CreatePipe and UpdatePipe reject
// a LogConfiguration with no Level, matching aws-sdk-go-v2 pipes
// validators.go's validatePipeLogConfigurationParameters. Reached from both
// validateOpCreatePipeInput and validateOpUpdatePipeInput.
func TestLogConfigurationLevelRequired(t *testing.T) {
	t.Parallel()

	t.Run("create_missing_rejected", func(t *testing.T) {
		t.Parallel()
		b := auditNewBackend()
		_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
			Name:             "log-create-missing",
			RoleARN:          "arn:aws:iam::123456789012:role/r",
			Source:           "arn:aws:sqs:us-west-2:123456789012:q",
			Target:           "arn:aws:lambda:us-west-2:123456789012:function:fn",
			DesiredState:     "RUNNING",
			LogConfiguration: &pipes.LogConfiguration{},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, pipes.ErrValidation)
	})

	t.Run("create_present_accepted", func(t *testing.T) {
		t.Parallel()
		b := auditNewBackend()
		_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
			Name:             "log-create-present",
			RoleARN:          "arn:aws:iam::123456789012:role/r",
			Source:           "arn:aws:sqs:us-west-2:123456789012:q",
			Target:           "arn:aws:lambda:us-west-2:123456789012:function:fn",
			DesiredState:     "RUNNING",
			LogConfiguration: &pipes.LogConfiguration{Level: "ERROR"},
		})
		assert.NoError(t, err)
	})

	t.Run("create_absent_accepted", func(t *testing.T) {
		t.Parallel()
		b := auditNewBackend()
		_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
			Name:         "log-create-absent",
			RoleARN:      "arn:aws:iam::123456789012:role/r",
			Source:       "arn:aws:sqs:us-west-2:123456789012:q",
			Target:       "arn:aws:lambda:us-west-2:123456789012:function:fn",
			DesiredState: "RUNNING",
		})
		assert.NoError(t, err)
	})

	t.Run("update_missing_rejected", func(t *testing.T) {
		t.Parallel()
		b := auditNewBackend()
		_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
			Name:         "log-update-missing",
			RoleARN:      "arn:aws:iam::123456789012:role/r",
			Source:       "arn:aws:sqs:us-west-2:123456789012:q",
			Target:       "arn:aws:lambda:us-west-2:123456789012:function:fn",
			DesiredState: "RUNNING",
		})
		require.NoError(t, err)

		_, err = b.UpdatePipe(context.Background(), "log-update-missing", pipes.UpdatePipeInput{
			RoleARN:          "arn:aws:iam::123456789012:role/r",
			LogConfiguration: &pipes.LogConfiguration{},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, pipes.ErrValidation)
	})
}
