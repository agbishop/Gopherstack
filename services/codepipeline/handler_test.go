package codepipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/codepipeline"
)

func newTestHandler(t *testing.T) *codepipeline.Handler {
	t.Helper()

	return codepipeline.NewHandler(codepipeline.NewInMemoryBackend("000000000000", "us-east-1"))
}

func doRequest(t *testing.T, h *codepipeline.Handler, action string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	} else {
		bodyBytes = []byte("{}")
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "CodePipeline_20150709."+action)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func samplePipeline(name string) codepipeline.PipelineDeclaration {
	return codepipeline.PipelineDeclaration{
		Name:    name,
		RoleArn: "arn:aws:iam::000000000000:role/pipeline-role",
		ArtifactStore: codepipeline.ArtifactStore{
			Type:     "S3",
			Location: "my-artifact-bucket",
		},
		Stages: []codepipeline.Stage{
			{
				Name: "Source",
				Actions: []codepipeline.Action{
					{
						Name: "SourceAction",
						ActionTypeID: codepipeline.ActionTypeID{
							Category: "Source",
							Owner:    "ThirdParty",
							Provider: "GitHub",
							Version:  "1",
						},
					},
				},
			},
		},
	}
}

// approvalPipeline returns a 3-stage pipeline (Source -> Approve -> Deploy)
// whose middle stage is a single manual-approval action, used by tests that
// exercise the approval-gate machine in action_engine.go/approvals.go:
// StartPipelineExecution runs Source, then gates on Approve until
// PutApprovalResult resolves it, and only then runs Deploy.
func approvalPipeline(name string) codepipeline.PipelineDeclaration {
	p := samplePipeline(name)
	p.Stages = append(p.Stages,
		codepipeline.Stage{
			Name: "Approve",
			Actions: []codepipeline.Action{
				{
					Name: "ApprovalAction",
					ActionTypeID: codepipeline.ActionTypeID{
						Category: "Approval",
						Owner:    "AWS",
						Provider: "Manual",
						Version:  "1",
					},
				},
			},
		},
		codepipeline.Stage{
			Name: "Deploy",
			Actions: []codepipeline.Action{
				{
					Name: "DeployAction",
					ActionTypeID: codepipeline.ActionTypeID{
						Category: "Deploy",
						Owner:    "AWS",
						Provider: "S3",
						Version:  "1",
					},
				},
			},
		},
	)

	return p
}

// twoStagePipeline returns a 2-stage pipeline (Source -> Deploy) with no
// approval gate, used by tests exercising DisableStageTransition/
// EnableStageTransition (pipeline_state.go).
func twoStagePipeline(name string) codepipeline.PipelineDeclaration {
	p := samplePipeline(name)
	p.Stages = append(p.Stages, codepipeline.Stage{
		Name: "Deploy",
		Actions: []codepipeline.Action{
			{
				Name: "DeployAction",
				ActionTypeID: codepipeline.ActionTypeID{
					Category: "Deploy",
					Owner:    "AWS",
					Provider: "S3",
					Version:  "1",
				},
			},
		},
	})

	return p
}

// approvalToken extracts the pending approval token for stageName/actionName
// from a decoded GetPipelineState response body.
//
//nolint:unparam // stageName is always "Approve" today; the helper stays general.
func approvalToken(t *testing.T, body map[string]any, stageName, actionName string) string {
	t.Helper()

	stageStates, _ := body["stageStates"].([]any)
	for _, s := range stageStates {
		stage, _ := s.(map[string]any)
		if stage["stageName"] != stageName {
			continue
		}

		actionStates, _ := stage["actionStates"].([]any)
		for _, a := range actionStates {
			action, _ := a.(map[string]any)
			if action["actionName"] != actionName {
				continue
			}

			latest, _ := action["latestExecution"].(map[string]any)
			token, _ := latest["token"].(string)

			return token
		}
	}

	return ""
}

func decodeBody(t *testing.T, data []byte) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	return m
}

// ---- Pipeline Execution tests ----

func TestHandler_Name(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, "CodePipeline", h.Name())
}

func TestHandler_ChaosServiceName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, "codepipeline", h.ChaosServiceName())
}

func TestHandler_MatchPriority(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, service.PriorityHeaderExact, h.MatchPriority())
}

func TestHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.GetSupportedOperations()
	assert.Contains(t, ops, "CreatePipeline")
	assert.Contains(t, ops, "GetPipeline")
	assert.Contains(t, ops, "UpdatePipeline")
	assert.Contains(t, ops, "DeletePipeline")
	assert.Contains(t, ops, "ListPipelines")
	assert.Contains(t, ops, "ListTagsForResource")
	assert.Contains(t, ops, "TagResource")
	assert.Contains(t, ops, "UntagResource")
}

func TestHandler_GetSupportedOperations_JobAndWebhookOps(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.GetSupportedOperations()

	for _, want := range []string{
		"AcknowledgeJob",
		"AcknowledgeThirdPartyJob",
		"CreateCustomActionType",
		"DeleteCustomActionType",
		"DeleteWebhook",
		"DeregisterWebhookWithThirdParty",
		"DisableStageTransition",
		"EnableStageTransition",
		"GetActionType",
		"GetJobDetails",
	} {
		assert.Contains(t, ops, want)
	}
}

// ============================================================
// Refinement check 1 tests
// ============================================================

func TestHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	matcher := h.RouteMatcher()

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{
			name:   "codepipeline prefix",
			target: "CodePipeline_20150709.CreatePipeline",
			want:   true,
		},
		{
			name:   "other service",
			target: "CodeBuild_20161006.CreateProject",
			want:   false,
		},
		{
			name:   "empty",
			target: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.want, matcher(c))
		})
	}
}

func TestHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "CreatePipeline",
			target: "CodePipeline_20150709.CreatePipeline",
			want:   "CreatePipeline",
		},
		{
			name:   "GetPipeline",
			target: "CodePipeline_20150709.GetPipeline",
			want:   "GetPipeline",
		},
		{
			name:   "no prefix",
			target: "SomeOtherTarget",
			want:   "SomeOtherTarget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

func TestHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	assert.Empty(t, h.ExtractResource(c))
}

func TestHandler_UnknownAction(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "UnknownAction", map[string]any{})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "InvalidActionException", out["__type"])
}

func TestHandler_ChaosOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.ChaosOperations()
	assert.Equal(t, h.GetSupportedOperations(), ops)
}

func TestHandler_ChaosRegions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	regions := h.ChaosRegions()
	require.Len(t, regions, 1)
	assert.Equal(t, "us-east-1", regions[0])
}

func TestHandler_ErrorEnvelopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      any
		setup      func(h *codepipeline.Handler)
		name       string
		action     string
		wantType   string
		wantStatus int
	}{
		{
			name:       "not found returns PipelineNotFoundException",
			action:     "GetPipeline",
			input:      map[string]any{"name": "nonexistent"},
			wantStatus: http.StatusBadRequest,
			wantType:   "PipelineNotFoundException",
		},
		{
			name: "duplicate create returns PipelineNameInUseException",
			setup: func(h *codepipeline.Handler) {
				_, err := h.Backend.CreatePipeline(context.Background(), samplePipeline("duplicate-pipeline"), nil)
				require.NoError(t, err)
			},
			action:     "CreatePipeline",
			input:      map[string]any{"pipeline": samplePipeline("duplicate-pipeline")},
			wantStatus: http.StatusBadRequest,
			wantType:   "PipelineNameInUseException",
		},
		{
			name:       "unknown action returns InvalidActionException",
			action:     "NoSuchAction",
			input:      map[string]any{},
			wantStatus: http.StatusBadRequest,
			wantType:   "InvalidActionException",
		},
		{
			name:       "missing required field returns ValidationException",
			action:     "CreatePipeline",
			input:      map[string]any{},
			wantStatus: http.StatusBadRequest,
			wantType:   "ValidationException",
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
			require.Equal(t, tt.wantStatus, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, tt.wantType, out["__type"])
		})
	}
}

func TestErrValidationMapping(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Trigger ErrValidation via invalid category.
	rec := doRequest(t, h, "CreateCustomActionType", map[string]any{
		"category":              "NotAValidCategory",
		"provider":              "P",
		"version":               "1",
		"inputArtifactDetails":  map[string]any{"minimumCount": 0, "maximumCount": 5},
		"outputArtifactDetails": map[string]any{"minimumCount": 0, "maximumCount": 5},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "ValidationException", out["__type"])
}

func TestHandlerOpsPreBuilt(t *testing.T) {
	t.Parallel()

	// Verify that handler dispatches correctly without rebuilding the table
	// (ops cached in NewHandler). Run requests to exercise the cached dispatch path.
	const numRequests = 5

	h := newTestHandler(t)

	for range numRequests {
		rec := doRequest(t, h, "ListPipelines", map[string]any{})
		assert.Equal(t, http.StatusOK, rec.Code)
	}
}

func TestExportCountHelpers(t *testing.T) {
	t.Parallel()

	b := codepipeline.NewInMemoryBackend("000000000000", "us-east-1")

	assert.Equal(t, 0, b.PipelineCount())
	assert.Equal(t, 0, b.CustomActionTypeCount())
	assert.Equal(t, 0, b.JobCount())
	assert.Equal(t, 0, b.WebhookCount())
	assert.Equal(t, 0, b.StageTransitionCount())

	b.AddPipelineInternal(samplePipeline("cnt-pl"), nil)
	assert.Equal(t, 1, b.PipelineCount())

	b.AddCustomActionTypeInternal(&codepipeline.CustomActionType{
		Category: "Build", Provider: "Cnt", Version: "1",
		InputArtifactDetails:  codepipeline.ArtifactDetails{MinimumCount: 0, MaximumCount: 5},
		OutputArtifactDetails: codepipeline.ArtifactDetails{MinimumCount: 0, MaximumCount: 5},
	})
	assert.Equal(t, 1, b.CustomActionTypeCount())

	b.AddJobInternal(&codepipeline.Job{ID: "cnt-j", Nonce: "n", Status: "Created"})
	assert.Equal(t, 1, b.JobCount())

	b.AddWebhookInternal(&codepipeline.Webhook{Name: "cnt-wh", TargetPipeline: "cnt-pl"})
	assert.Equal(t, 1, b.WebhookCount())
}

func TestSeedHelpers(t *testing.T) {
	t.Parallel()

	b := codepipeline.NewInMemoryBackend("000000000000", "us-east-1")

	// AddPipelineInternal
	b.AddPipelineInternal(samplePipeline("seed-pl"), map[string]string{"x": "y"})
	assert.Equal(t, 1, b.PipelineCount())

	// AddCustomActionTypeInternal
	b.AddCustomActionTypeInternal(&codepipeline.CustomActionType{
		Category: "Build", Provider: "Seed", Version: "1",
		InputArtifactDetails:  codepipeline.ArtifactDetails{MinimumCount: 0, MaximumCount: 5},
		OutputArtifactDetails: codepipeline.ArtifactDetails{MinimumCount: 0, MaximumCount: 5},
	})
	assert.Equal(t, 1, b.CustomActionTypeCount())

	// AddJobInternal
	b.AddJobInternal(&codepipeline.Job{ID: "j-1", Nonce: "n", Status: "Created"})
	assert.Equal(t, 1, b.JobCount())

	// AddWebhookInternal
	b.AddWebhookInternal(&codepipeline.Webhook{Name: "wh-seed", TargetPipeline: "pl"})
	assert.Equal(t, 1, b.WebhookCount())
}

func TestReset(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create a pipeline so there is state to reset.
	_, err := h.Backend.CreatePipeline(context.Background(), samplePipeline("reset-pl"), nil)
	require.NoError(t, err)

	assert.Equal(t, 1, h.Backend.PipelineCount())

	h.Reset()

	assert.Equal(t, 0, h.Backend.PipelineCount())
	assert.Equal(t, 0, h.Backend.CustomActionTypeCount())
	assert.Equal(t, 0, h.Backend.JobCount())
	assert.Equal(t, 0, h.Backend.WebhookCount())
	assert.Equal(t, 0, h.Backend.StageTransitionCount())
}
