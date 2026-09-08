package codepipeline_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codepipeline"
)

func TestHandler_DisableEnableStageTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codepipeline.Handler)
		input      any
		name       string
		action     string
		httpStatus int
		wantErr    bool
	}{
		{
			name:   "disable_success",
			action: "DisableStageTransition",
			setup: func(h *codepipeline.Handler) {
				_, err := h.Backend.CreatePipeline(context.Background(), samplePipeline("trans-pipeline"), nil)
				require.NoError(t, err)
			},
			input: map[string]any{
				"pipelineName":   "trans-pipeline",
				"stageName":      "Source",
				"transitionType": "Inbound",
				"reason":         "waiting for approval",
			},
			httpStatus: http.StatusOK,
		},
		{
			name:   "disable_pipeline_not_found",
			action: "DisableStageTransition",
			setup:  nil,
			input: map[string]any{
				"pipelineName":   "ghost-pipeline",
				"stageName":      "Source",
				"transitionType": "Inbound",
				"reason":         "test",
			},
			httpStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:   "disable_missing_reason",
			action: "DisableStageTransition",
			setup:  nil,
			input: map[string]any{
				"pipelineName":   "any",
				"stageName":      "Source",
				"transitionType": "Inbound",
			},
			httpStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:   "enable_success",
			action: "EnableStageTransition",
			setup: func(h *codepipeline.Handler) {
				_, err := h.Backend.CreatePipeline(context.Background(), samplePipeline("enable-pipeline"), nil)
				require.NoError(t, err)
			},
			input: map[string]any{
				"pipelineName":   "enable-pipeline",
				"stageName":      "Source",
				"transitionType": "Outbound",
			},
			httpStatus: http.StatusOK,
		},
		{
			name:   "enable_pipeline_not_found",
			action: "EnableStageTransition",
			setup:  nil,
			input: map[string]any{
				"pipelineName":   "ghost",
				"stageName":      "Source",
				"transitionType": "Outbound",
			},
			httpStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:   "enable_missing_transitionType",
			action: "EnableStageTransition",
			setup:  nil,
			input: map[string]any{
				"pipelineName": "any",
				"stageName":    "Source",
			},
			httpStatus: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, tt.action, tt.input)
			assert.Equal(t, tt.httpStatus, rec.Code)
		})
	}
}

func TestHandler_DisableEnableStageTransition_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreatePipeline(context.Background(), samplePipeline("rt-pipeline"), nil)
	require.NoError(t, err)

	// Disable the transition.
	rec := doRequest(t, h, "DisableStageTransition", map[string]any{
		"pipelineName":   "rt-pipeline",
		"stageName":      "Source",
		"transitionType": "Inbound",
		"reason":         "blocked",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Re-enable the transition.
	rec = doRequest(t, h, "EnableStageTransition", map[string]any{
		"pipelineName":   "rt-pipeline",
		"stageName":      "Source",
		"transitionType": "Inbound",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestHandler_DisableStageTransition_BlocksInboundExecution proves
// DisableStageTransition actually gates a pipeline run rather than being
// pure bookkeeping. Before this fix, StartPipelineExecution ignored
// stageTransitions entirely (action_engine.go never consulted it), so a
// disabled inbound transition on Deploy never stopped the execution from
// running straight through Deploy's actions to Succeeded -- the real SDK's
// own doc comment for DisableStageTransition ("Prevents artifacts in a
// pipeline from transitioning to the next stage in the pipeline") was not
// honored at all.
func TestHandler_DisableStageTransition_BlocksInboundExecution(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreatePipeline", map[string]any{"pipeline": twoStagePipeline("inbound-gate")})

	rec := doRequest(t, h, "DisableStageTransition", map[string]any{
		"pipelineName":   "inbound-gate",
		"stageName":      "Deploy",
		"transitionType": "Inbound",
		"reason":         "change freeze",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	startRec := doRequest(t, h, "StartPipelineExecution", map[string]any{"name": "inbound-gate"})
	require.Equal(t, http.StatusOK, startRec.Code)
	execID, _ := decodeBody(t, startRec.Body.Bytes())["pipelineExecutionId"].(string)
	require.NotEmpty(t, execID)

	getRec := doRequest(t, h, "GetPipelineExecution", map[string]any{
		"pipelineName": "inbound-gate", "pipelineExecutionId": execID,
	})
	require.Equal(t, http.StatusOK, getRec.Code)
	execBody := decodeBody(t, getRec.Body.Bytes())
	pipelineExecution, _ := execBody["pipelineExecution"].(map[string]any)
	require.Equal(t, "InProgress", pipelineExecution["status"],
		"execution must stall at the disabled inbound gate, not run through to Succeeded")

	stateRec := doRequest(t, h, "GetPipelineState", map[string]any{"name": "inbound-gate"})
	stageStates, _ := decodeBody(t, stateRec.Body.Bytes())["stageStates"].([]any)
	require.Len(t, stageStates, 2)
	deployStage, _ := stageStates[1].(map[string]any)
	deployActions, _ := deployStage["actionStates"].([]any)
	require.Len(t, deployActions, 1)
	deployAction, _ := deployActions[0].(map[string]any)
	assert.Nil(t, deployAction["latestExecution"],
		"Deploy must not have been entered while its inbound transition is disabled")

	// EnableStageTransition must resume the parked execution -- real AWS
	// requires no further client call once the transition is re-enabled.
	enableRec := doRequest(t, h, "EnableStageTransition", map[string]any{
		"pipelineName":   "inbound-gate",
		"stageName":      "Deploy",
		"transitionType": "Inbound",
	})
	require.Equal(t, http.StatusOK, enableRec.Code)

	getRec = doRequest(t, h, "GetPipelineExecution", map[string]any{
		"pipelineName": "inbound-gate", "pipelineExecutionId": execID,
	})
	execBody = decodeBody(t, getRec.Body.Bytes())
	pipelineExecution, _ = execBody["pipelineExecution"].(map[string]any)
	assert.Equal(t, "Succeeded", pipelineExecution["status"], "re-enabling must resume the execution to completion")
}

// TestHandler_DisableStageTransition_BlocksOutboundExecution proves the
// outbound half of the same gate: Source's actions run to completion, but
// the execution does not proceed into Deploy while Source's outbound
// transition is disabled.
func TestHandler_DisableStageTransition_BlocksOutboundExecution(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreatePipeline", map[string]any{"pipeline": twoStagePipeline("outbound-gate")})

	rec := doRequest(t, h, "DisableStageTransition", map[string]any{
		"pipelineName":   "outbound-gate",
		"stageName":      "Source",
		"transitionType": "Outbound",
		"reason":         "change freeze",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	startRec := doRequest(t, h, "StartPipelineExecution", map[string]any{"name": "outbound-gate"})
	execID, _ := decodeBody(t, startRec.Body.Bytes())["pipelineExecutionId"].(string)
	require.NotEmpty(t, execID)

	getRec := doRequest(t, h, "GetPipelineExecution", map[string]any{
		"pipelineName": "outbound-gate", "pipelineExecutionId": execID,
	})
	execBody := decodeBody(t, getRec.Body.Bytes())
	pipelineExecution, _ := execBody["pipelineExecution"].(map[string]any)
	require.Equal(t, "InProgress", pipelineExecution["status"])

	stateRec := doRequest(t, h, "GetPipelineState", map[string]any{"name": "outbound-gate"})
	stageStates, _ := decodeBody(t, stateRec.Body.Bytes())["stageStates"].([]any)
	sourceStage, _ := stageStates[0].(map[string]any)
	sourceActions, _ := sourceStage["actionStates"].([]any)
	sourceAction, _ := sourceActions[0].(map[string]any)
	sourceLatest, _ := sourceAction["latestExecution"].(map[string]any)
	require.NotNil(t, sourceLatest, "Source's own actions must still run")
	assert.Equal(t, "Succeeded", sourceLatest["status"])

	deployStage, _ := stageStates[1].(map[string]any)
	deployActions, _ := deployStage["actionStates"].([]any)
	deployAction, _ := deployActions[0].(map[string]any)
	assert.Nil(t, deployAction["latestExecution"],
		"Deploy must not have been entered while Source's outbound transition is disabled")
}

func TestHandler_DisableStageTransition_StageValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      map[string]any
		setup      func(h *codepipeline.Handler)
		name       string
		wantType   string
		wantStatus int
	}{
		{
			name: "existing stage disabled ok",
			setup: func(h *codepipeline.Handler) {
				_, err := h.Backend.CreatePipeline(context.Background(), samplePipeline("stage-exists"), nil)
				require.NoError(t, err)
			},
			input: map[string]any{
				"pipelineName":   "stage-exists",
				"stageName":      "Source",
				"transitionType": "Inbound",
				"reason":         "testing",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "non-existent stage rejected",
			setup: func(h *codepipeline.Handler) {
				_, err := h.Backend.CreatePipeline(context.Background(), samplePipeline("stage-missing"), nil)
				require.NoError(t, err)
			},
			input: map[string]any{
				"pipelineName":   "stage-missing",
				"stageName":      "NonExistentStage",
				"transitionType": "Inbound",
				"reason":         "testing",
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "StageNotFoundException",
		},
		{
			name:  "non-existent pipeline returns PipelineNotFoundException",
			setup: nil,
			input: map[string]any{
				"pipelineName":   "ghost-pipeline",
				"stageName":      "Source",
				"transitionType": "Inbound",
				"reason":         "testing",
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "PipelineNotFoundException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "DisableStageTransition", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantType != "" {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Equal(t, tt.wantType, out["__type"])
			}
		})
	}
}

// --------------------------------------------------------------------------
// #27 & #28 Persistence: Restore calls Reset + defensive copy
// --------------------------------------------------------------------------

func TestTransitionTypeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		action         string
		transitionType string
		wantErr        bool
	}{
		{
			name:           "disable_inbound_ok",
			action:         "DisableStageTransition",
			transitionType: "Inbound",
			wantErr:        false,
		},
		{
			name:           "disable_outbound_ok",
			action:         "DisableStageTransition",
			transitionType: "Outbound",
			wantErr:        false,
		},
		{
			name:           "disable_invalid",
			action:         "DisableStageTransition",
			transitionType: "Invalid",
			wantErr:        true,
		},
		{
			name:           "enable_inbound_ok",
			action:         "EnableStageTransition",
			transitionType: "Inbound",
			wantErr:        false,
		},
		{
			name:           "enable_invalid",
			action:         "EnableStageTransition",
			transitionType: "BadValue",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreatePipeline(context.Background(), samplePipeline("enum-pl"), nil)
			require.NoError(t, err)

			var input map[string]any
			if tt.action == "DisableStageTransition" {
				input = map[string]any{
					"pipelineName":   "enum-pl",
					"stageName":      "Source",
					"transitionType": tt.transitionType,
					"reason":         "test",
				}
			} else {
				input = map[string]any{
					"pipelineName":   "enum-pl",
					"stageName":      "Source",
					"transitionType": tt.transitionType,
				}
			}

			rec := doRequest(t, h, tt.action, input)

			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			} else {
				assert.Equal(t, http.StatusOK, rec.Code)
			}
		})
	}
}

func TestGetStageTransitionState(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreatePipeline(context.Background(), samplePipeline("state-pl"), nil)
	require.NoError(t, err)

	// Initially enabled (nil).
	state := h.Backend.GetStageTransitionState(context.Background(), "state-pl", "Source", "Inbound")
	assert.Nil(t, state)

	// Disable it.
	rec := doRequest(t, h, "DisableStageTransition", map[string]any{
		"pipelineName":   "state-pl",
		"stageName":      "Source",
		"transitionType": "Inbound",
		"reason":         "blocked",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	state = h.Backend.GetStageTransitionState(context.Background(), "state-pl", "Source", "Inbound")
	require.NotNil(t, state)
	assert.Equal(t, "blocked", state.Reason)
	assert.True(t, state.Disabled)

	// Re-enable it.
	rec = doRequest(t, h, "EnableStageTransition", map[string]any{
		"pipelineName":   "state-pl",
		"stageName":      "Source",
		"transitionType": "Inbound",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	state = h.Backend.GetStageTransitionState(context.Background(), "state-pl", "Source", "Inbound")
	assert.Nil(t, state)
}

func TestPipelineNameRequiredInDisable(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "DisableStageTransition", map[string]any{
		"stageName":      "Source",
		"transitionType": "Inbound",
		"reason":         "test",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_GetPipelineState(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreatePipeline", map[string]any{
		"pipeline": samplePipeline("state-pipeline"),
	})

	rec := doRequest(t, h, "GetPipelineState", map[string]any{
		"name": "state-pipeline",
	})
	require.Equal(t, 200, rec.Code)

	// Missing name
	rec = doRequest(t, h, "GetPipelineState", map[string]any{})
	assert.Equal(t, 400, rec.Code)
}

// ---- Retry / Rollback / Override tests ----

func TestHandler_GetPipelineState_ActionStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codepipeline.Handler)
		checkFn    func(t *testing.T, out map[string]any)
		name       string
		wantStatus int
	}{
		{
			name: "actionStates included per stage",
			setup: func(h *codepipeline.Handler) {
				_, err := h.Backend.CreatePipeline(context.Background(), samplePipeline("state-pl"), nil)
				require.NoError(t, err)
			},
			wantStatus: http.StatusOK,
			checkFn: func(t *testing.T, out map[string]any) {
				t.Helper()

				stages, _ := out["stageStates"].([]any)
				require.Len(t, stages, 1)

				stage0, _ := stages[0].(map[string]any)
				actionStates, ok := stage0["actionStates"]
				assert.True(t, ok, "actionStates key must be present")
				assert.NotNil(t, actionStates)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "GetPipelineState", map[string]any{"name": "state-pl"})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.checkFn != nil {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				tt.checkFn(t, out)
			}
		})
	}
}

// --------------------------------------------------------------------------
// ListPipelineExecutions returns stored executions
// --------------------------------------------------------------------------

func TestHandler_GetPipelineState_LatestExecution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		checkFn      func(t *testing.T, out map[string]any)
		name         string
		pipelineName string
		wantStatus   int
		runExec      bool
	}{
		{
			name:         "latestExecution absent before any execution",
			pipelineName: "le-no-exec",
			runExec:      false,
			wantStatus:   http.StatusOK,
			checkFn: func(t *testing.T, out map[string]any) {
				t.Helper()

				stages, _ := out["stageStates"].([]any)
				require.Len(t, stages, 1)

				stage0, _ := stages[0].(map[string]any)
				actionStates, _ := stage0["actionStates"].([]any)
				require.Len(t, actionStates, 1)

				action0, _ := actionStates[0].(map[string]any)
				_, hasLatest := action0["latestExecution"]
				assert.False(t, hasLatest, "latestExecution must be absent before any execution")
			},
		},
		{
			name:         "latestExecution populated after StartPipelineExecution",
			pipelineName: "le-with-exec",
			runExec:      true,
			wantStatus:   http.StatusOK,
			checkFn: func(t *testing.T, out map[string]any) {
				t.Helper()

				stages, _ := out["stageStates"].([]any)
				require.Len(t, stages, 1)

				stage0, _ := stages[0].(map[string]any)
				actionStates, _ := stage0["actionStates"].([]any)
				require.Len(t, actionStates, 1)

				action0, _ := actionStates[0].(map[string]any)
				latest, ok := action0["latestExecution"].(map[string]any)
				require.True(t, ok, "latestExecution must be present after execution")

				assert.NotEmpty(t, latest["actionExecutionId"], "actionExecutionId must be set")
				assert.NotEmpty(t, latest["status"], "status must be set")
				assert.NotZero(t, latest["startTime"], "startTime must be set")
				assert.NotZero(t, latest["lastUpdateTime"], "lastUpdateTime must be set")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := codepipeline.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreatePipeline(context.Background(), samplePipeline(tt.pipelineName), nil)
			require.NoError(t, err)

			if tt.runExec {
				_, err = b.StartPipelineExecution(context.Background(), tt.pipelineName)
				require.NoError(t, err)
			}

			h := codepipeline.NewHandler(b)
			rec := doRequest(t, h, "GetPipelineState", map[string]any{"name": tt.pipelineName})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.checkFn != nil {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				tt.checkFn(t, out)
			}
		})
	}
}

// --------------------------------------------------------------------------
// PollForJobs respects maxBatchSize
// --------------------------------------------------------------------------

func TestGetPipelineState_PipelineVersion(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		name        string
		wantVersion float64
		updates     int
	}{
		{name: "version_1_on_create", updates: 0, wantVersion: 1},
		{name: "version_2_after_update", updates: 1, wantVersion: 2},
		{name: "version_3_after_two_updates", updates: 2, wantVersion: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h2 := newTestHandler(t)
			pl := samplePipeline("state-version-pipe")

			rec := doRequest(t, h2, "CreatePipeline", map[string]any{"pipeline": pl})
			require.Equal(t, http.StatusOK, rec.Code)

			for range tt.updates {
				rec = doRequest(t, h2, "UpdatePipeline", map[string]any{"pipeline": pl})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec = doRequest(t, h2, "GetPipelineState", map[string]any{"name": pl.Name})
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.InDelta(t, tt.wantVersion, out["pipelineVersion"], 0, "pipelineVersion mismatch")
		})
	}

	_ = h
}

// TestParity_GetJobDetails_DataPopulated verifies data.actionTypeId is populated.

// TestHandler_RetryStageExecution_RequiresFailedAction locks in real AWS's
// precondition for RetryStageExecution: a stage is only retryable once one
// of its actions has actually failed. Before this fix, RetryStageExecution
// fabricated a 200/InProgress response for ANY existing pipeline+execution
// regardless of stage state, so this pass could never distinguish "stage
// legitimately failed" from "stage never ran" -- StageNotRetryableException
// is the real AWS error for the latter (verified against
// aws-sdk-go-v2/service/codepipeline/types/errors.go).
func TestHandler_RetryStageExecution_RequiresFailedAction(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreatePipeline", map[string]any{"pipeline": samplePipeline("retry-pipeline")})
	startRec := doRequest(t, h, "StartPipelineExecution", map[string]any{"name": "retry-pipeline"})
	execID, _ := decodeBody(t, startRec.Body.Bytes())["pipelineExecutionId"].(string)
	require.NotEmpty(t, execID)

	tests := []struct {
		input      map[string]any
		name       string
		wantType   string
		httpStatus int
	}{
		{
			name: "no failed action is not retryable",
			input: map[string]any{
				"pipelineName": "retry-pipeline", "stageName": "Source",
				"pipelineExecutionId": execID, "retryMode": "FAILED_ACTIONS",
			},
			httpStatus: http.StatusBadRequest,
			wantType:   "StageNotRetryableException",
		},
		{
			name: "unknown stage",
			input: map[string]any{
				"pipelineName": "retry-pipeline", "stageName": "NoSuchStage",
				"pipelineExecutionId": execID, "retryMode": "FAILED_ACTIONS",
			},
			httpStatus: http.StatusBadRequest,
			wantType:   "StageNotFoundException",
		},
		{
			name: "unknown execution",
			input: map[string]any{
				"pipelineName": "retry-pipeline", "stageName": "Source",
				"pipelineExecutionId": "no-such-execution", "retryMode": "FAILED_ACTIONS",
			},
			httpStatus: http.StatusBadRequest,
			wantType:   "PipelineExecutionNotFoundException",
		},
		{
			name: "invalid retryMode",
			input: map[string]any{
				"pipelineName": "retry-pipeline", "stageName": "Source", "pipelineExecutionId": execID,
			},
			httpStatus: http.StatusBadRequest,
			wantType:   "ValidationException",
		},
		{
			name:       "missing pipelineName",
			input:      map[string]any{},
			httpStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, "RetryStageExecution", tt.input)
			assert.Equal(t, tt.httpStatus, rec.Code)

			if tt.wantType != "" {
				assert.Equal(t, tt.wantType, decodeBody(t, rec.Body.Bytes())["__type"])
			}
		})
	}
}

// TestHandler_RetryStageExecution_ResumesAfterRejectedApproval exercises the
// full real path to a genuinely retryable stage: an Approval action gates
// StartPipelineExecution, PutApprovalResult Rejects it (failing the stage),
// and RetryStageExecution then succeeds, resuming the SAME execution through
// to a terminal Succeeded status.
func TestHandler_RetryStageExecution_ResumesAfterRejectedApproval(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreatePipeline", map[string]any{"pipeline": approvalPipeline("retry-approval-pipeline")})
	startRec := doRequest(t, h, "StartPipelineExecution", map[string]any{"name": "retry-approval-pipeline"})
	execID, _ := decodeBody(t, startRec.Body.Bytes())["pipelineExecutionId"].(string)
	require.NotEmpty(t, execID)

	stateRec := doRequest(t, h, "GetPipelineState", map[string]any{"name": "retry-approval-pipeline"})
	token := approvalToken(t, decodeBody(t, stateRec.Body.Bytes()), "Approve", "ApprovalAction")
	require.NotEmpty(t, token)

	rejectRec := doRequest(t, h, "PutApprovalResult", map[string]any{
		"pipelineName": "retry-approval-pipeline", "stageName": "Approve", "actionName": "ApprovalAction",
		"token":  token,
		"result": map[string]any{"status": "Rejected", "summary": "not ready"},
	})
	require.Equal(t, http.StatusOK, rejectRec.Code)

	failedRec := doRequest(t, h, "GetPipelineExecution", map[string]any{
		"pipelineName": "retry-approval-pipeline", "pipelineExecutionId": execID,
	})
	failedOut := decodeBody(t, failedRec.Body.Bytes())
	pe, _ := failedOut["pipelineExecution"].(map[string]any)
	require.Equal(t, "Failed", pe["status"], "rejected approval must fail the pipeline execution")

	retryRec := doRequest(t, h, "RetryStageExecution", map[string]any{
		"pipelineName": "retry-approval-pipeline", "stageName": "Approve",
		"pipelineExecutionId": execID, "retryMode": "FAILED_ACTIONS",
	})
	require.Equal(t, http.StatusOK, retryRec.Code)

	retryOut := decodeBody(t, retryRec.Body.Bytes())
	assert.Equal(t, execID, retryOut["pipelineExecutionId"], "retry resumes the SAME execution, not a new one")

	succeededRec := doRequest(t, h, "GetPipelineExecution", map[string]any{
		"pipelineName": "retry-approval-pipeline", "pipelineExecutionId": execID,
	})
	succeededOut := decodeBody(t, succeededRec.Body.Bytes())
	pe, _ = succeededOut["pipelineExecution"].(map[string]any)
	assert.Equal(t, "Succeeded", pe["status"], "retry must resume through Deploy to a terminal status")
}

// TestHandler_RollbackStage covers RollbackStage's real precondition (the
// target execution must have actually succeeded through the given stage --
// UnableToRollbackStageException otherwise) and its successful path, which
// creates and persists a new ROLLBACK-type PipelineExecution rather than
// fabricating an unpersisted response.
func TestHandler_RollbackStage(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreatePipeline", map[string]any{"pipeline": samplePipeline("rollback-pipeline")})
	startRec := doRequest(t, h, "StartPipelineExecution", map[string]any{"name": "rollback-pipeline"})
	execID, _ := decodeBody(t, startRec.Body.Bytes())["pipelineExecutionId"].(string)
	require.NotEmpty(t, execID)

	t.Run("missing pipelineName", func(t *testing.T) {
		t.Parallel()
		rec := doRequest(t, h, "RollbackStage", map[string]any{})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("unknown target execution is not rollback-able", func(t *testing.T) {
		t.Parallel()
		rec := doRequest(t, h, "RollbackStage", map[string]any{
			"pipelineName":              "rollback-pipeline",
			"stageName":                 "Source",
			"targetPipelineExecutionId": "no-such-execution",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "UnableToRollbackStageException", decodeBody(t, rec.Body.Bytes())["__type"])
	})

	t.Run("succeeded target rolls back and persists a new execution", func(t *testing.T) {
		t.Parallel()
		rec := doRequest(t, h, "RollbackStage", map[string]any{
			"pipelineName": "rollback-pipeline", "stageName": "Source", "targetPipelineExecutionId": execID,
		})
		require.Equal(t, http.StatusOK, rec.Code)

		out := decodeBody(t, rec.Body.Bytes())
		newExecID, _ := out["pipelineExecutionId"].(string)
		require.NotEmpty(t, newExecID)
		require.NotEqual(t, execID, newExecID, "rollback creates a NEW execution, distinct from the target")

		getRec := doRequest(t, h, "GetPipelineExecution", map[string]any{
			"pipelineName": "rollback-pipeline", "pipelineExecutionId": newExecID,
		})
		require.Equal(t, http.StatusOK, getRec.Code, "the rollback execution must actually be persisted")

		getOut := decodeBody(t, getRec.Body.Bytes())
		pe, _ := getOut["pipelineExecution"].(map[string]any)
		assert.Equal(t, "Succeeded", pe["status"])
		assert.Equal(t, "ROLLBACK", pe["executionType"])

		meta, _ := pe["rollbackMetadata"].(map[string]any)
		assert.Equal(t, execID, meta["rollbackTargetPipelineExecutionId"])
	})
}

// TestHandler_OverrideStageCondition covers the validated-but-not-mutating
// path documented in pipeline_state.go's OverrideStageCondition: this
// backend has no condition-rule engine, so success just means the pipeline,
// stage, and execution referenced by the request were all confirmed real.
func TestHandler_OverrideStageCondition(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreatePipeline", map[string]any{"pipeline": samplePipeline("override-pipeline")})
	startRec := doRequest(t, h, "StartPipelineExecution", map[string]any{"name": "override-pipeline"})
	execID, _ := decodeBody(t, startRec.Body.Bytes())["pipelineExecutionId"].(string)
	require.NotEmpty(t, execID)

	tests := []struct {
		input      map[string]any
		name       string
		wantType   string
		httpStatus int
	}{
		{
			name: "success",
			input: map[string]any{
				"pipelineName": "override-pipeline", "stageName": "Source",
				"pipelineExecutionId": execID, "conditionType": "BEFORE_ENTRY",
			},
			httpStatus: http.StatusOK,
		},
		{
			// ConditionType's real SDK enum (types.ConditionType) has exactly
			// two values, BEFORE_ENTRY and ON_SUCCESS -- ON_SUCCESS must be
			// accepted too, not just BEFORE_ENTRY.
			name: "success with ON_SUCCESS conditionType",
			input: map[string]any{
				"pipelineName": "override-pipeline", "stageName": "Source",
				"pipelineExecutionId": execID, "conditionType": "ON_SUCCESS",
			},
			httpStatus: http.StatusOK,
		},
		{
			name: "unknown execution",
			input: map[string]any{
				"pipelineName": "override-pipeline", "stageName": "Source",
				"pipelineExecutionId": "no-such-execution", "conditionType": "BEFORE_ENTRY",
			},
			httpStatus: http.StatusBadRequest,
			wantType:   "PipelineExecutionNotFoundException",
		},
		{
			name: "invalid conditionType",
			input: map[string]any{
				"pipelineName": "override-pipeline", "stageName": "Source",
				"pipelineExecutionId": execID, "conditionType": "NOT_A_TYPE",
			},
			httpStatus: http.StatusBadRequest,
			wantType:   "ValidationException",
		},
		{
			name:       "missing pipelineName",
			input:      map[string]any{},
			httpStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, "OverrideStageCondition", tt.input)
			assert.Equal(t, tt.httpStatus, rec.Code)

			if tt.wantType != "" {
				assert.Equal(t, tt.wantType, decodeBody(t, rec.Body.Bytes())["__type"])
			}
		})
	}
}

// ---- Webhook tests ----
