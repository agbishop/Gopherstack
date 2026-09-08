package bedrock_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/bedrock"
)

// --- Handler metadata tests --- //nolint:godot // existing issue.
func TestHandler_Name(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	assert.Equal(t, "Bedrock", h.Name())
}

func TestHandler_ChaosServiceName(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	assert.Equal(t, "bedrock", h.ChaosServiceName())
}

func TestHandler_MatchPriority(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	assert.Equal(t, service.PriorityPathVersioned, h.MatchPriority())
}

func TestHandler_GetSupportedOperations(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	ops := h.GetSupportedOperations()

	for _, op := range []string{
		"CreateGuardrail",
		"GetGuardrail",
		"ListGuardrails",
		"UpdateGuardrail",
		"DeleteGuardrail",
		"ListFoundationModels",
		"GetFoundationModel",
		"CreateProvisionedModelThroughput",
		"GetProvisionedModelThroughput",
		"ListProvisionedModelThroughputs",
		"UpdateProvisionedModelThroughput",
		"DeleteProvisionedModelThroughput",
		"ListTagsForResource",
		"TagResource",
		"UntagResource",
	} {
		assert.Contains(t, ops, op)
	}
}

func TestHandler_ChaosOperations(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	assert.Equal(t, h.GetSupportedOperations(), h.ChaosOperations())
}

func TestHandler_ChaosRegions(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	assert.Equal(t, []string{"us-east-1"}, h.ChaosRegions())
}

func TestHandler_RouteMatcher(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"guardrails path", "/guardrails", true},
		{"guardrail with id", "/guardrails/abc123", true},
		{"foundation models", "/foundation-models", true},
		{"foundation model with id", "/foundation-models/amazon.titan-text-express-v1", true},
		{"provisioned throughput", "/provisioned-model-throughput", true},
		{"provisioned throughputs list", "/provisioned-model-throughputs", true},
		{"list tags", "/listTagsForResource", true},
		{"tag resource", "/tagResource", true},
		{"untag resource", "/untagResource", true},
		{"unmatched path", "/something-else", false},
		{"s3 path", "/s3/bucket", false},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			matcher := h.RouteMatcher()
			assert.Equal(t, tt.want, matcher(c))
		})
	}
}

func TestHandler_ExtractOperation(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{"CreateGuardrail", http.MethodPost, "/guardrails", "CreateGuardrail"},
		{"ListGuardrails", http.MethodGet, "/guardrails", "ListGuardrails"},
		{"GetGuardrail", http.MethodGet, "/guardrails/abc123", "GetGuardrail"},
		{"UpdateGuardrail", http.MethodPut, "/guardrails/abc123", "UpdateGuardrail"},
		{"DeleteGuardrail", http.MethodDelete, "/guardrails/abc123", "DeleteGuardrail"},
		{"ListFoundationModels", http.MethodGet, "/foundation-models", "ListFoundationModels"},
		{
			"GetFoundationModel",
			http.MethodGet,
			"/foundation-models/amazon.titan-text-express-v1",
			"GetFoundationModel",
		},
		{
			"CreateProvisionedModelThroughput",
			http.MethodPost,
			"/provisioned-model-throughput",
			"CreateProvisionedModelThroughput",
		},
		{
			"ListProvisionedModelThroughputs",
			http.MethodGet,
			"/provisioned-model-throughputs",
			"ListProvisionedModelThroughputs",
		},
		{
			"GetProvisionedModelThroughput",
			http.MethodGet,
			"/provisioned-model-throughput/pmt-123",
			"GetProvisionedModelThroughput",
		},
		{
			"UpdateProvisionedModelThroughput",
			http.MethodPatch,
			"/provisioned-model-throughput/pmt-123",
			"UpdateProvisionedModelThroughput",
		},
		{
			"DeleteProvisionedModelThroughput",
			http.MethodDelete,
			"/provisioned-model-throughput/pmt-123",
			"DeleteProvisionedModelThroughput",
		},
		{"ListTagsForResource", http.MethodPost, "/listTagsForResource", "ListTagsForResource"},
		{"TagResource", http.MethodPost, "/tagResource", "TagResource"},
		{"UntagResource", http.MethodPost, "/untagResource", "UntagResource"},
		{"Unknown", http.MethodGet, "/unknown", "Unknown"},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

func TestHandler_ExtractResource(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name string
		path string
		want string
	}{
		{"guardrail id", "/guardrails/abc123", "abc123"},
		{"foundation model id", "/foundation-models/amazon.titan-text-express-v1", "amazon.titan-text-express-v1"},
		{"provisioned throughput id", "/provisioned-model-throughput/pmt-001", "pmt-001"},
		{"no resource", "/guardrails", ""},
		{"tags path", "/listTagsForResource", ""},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			assert.Equal(t, tt.want, h.ExtractResource(c))
		})
	}
}

// TestHandler_Tags tests tag operations in sequence using a single shared handler state.
// //nolint:goimports,godot // existing issue.
func TestHandler_Tags(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	g, err := h.Backend.CreateGuardrail("tagged-guardrail", "", "", "", nil)
	require.NoError(t, err)

	arn := g.GuardrailArn

	t.Run("list empty tags", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest(t, h, http.MethodPost, "/listTagsForResource", map[string]any{
			"resourceARN": arn,
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		mustUnmarshal(t, rec, &out)
		tags := out["tags"].([]any)
		assert.Empty(t, tags)
	})

	t.Run("tag resource", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest(t, h, http.MethodPost, "/tagResource", map[string]any{
			"resourceARN": arn,
			"tags": []map[string]string{
				{"key": "env", "value": "test"},
			},
		})
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("list tags after tagging", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest(t, h, http.MethodPost, "/listTagsForResource", map[string]any{
			"resourceARN": arn,
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		mustUnmarshal(t, rec, &out)
		tags := out["tags"].([]any)
		assert.Len(t, tags, 1)
	})

	t.Run("untag resource", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest(t, h, http.MethodPost, "/untagResource", map[string]any{
			"resourceARN": arn,
			"tagKeys":     []string{"env"},
		})
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("list tags after untagging", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest(t, h, http.MethodPost, "/listTagsForResource", map[string]any{
			"resourceARN": arn,
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		mustUnmarshal(t, rec, &out)
		tags := out["tags"].([]any)
		assert.Empty(t, tags)
	})
}

func TestHandler_UnknownOperation(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/unknown-path", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_InvalidJSON(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"untagResource bad json", http.MethodPost, "/untagResource"},
		{"tagResource bad json", http.MethodPost, "/tagResource"},
		{"listTagsForResource bad json", http.MethodPost, "/listTagsForResource"},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader([]byte("bad json")))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_NewOperationsRouteMatcher(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"evaluation jobs", "/evaluation-jobs", true},
		{"evaluation jobs batch-delete", "/evaluation-jobs/batch-delete", true},
		{"automated reasoning policies", "/automated-reasoning-policies", true},
		{"arp sub-path", "/automated-reasoning-policies/arn:aws:bedrock::/test-cases", true},
		{"custom models create", "/custom-models/create-custom-model", true},
		{"custom model deployments", "/model-customization/custom-model-deployments", true},
		{"foundation model agreement", "/create-foundation-model-agreement", true},
		{"unmatched", "/other-path", false},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			matcher := h.RouteMatcher()
			assert.Equal(t, tt.want, matcher(c))
		})
	}
}

func TestHandler_NewOperationsExtractOperation(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{"CreateEvaluationJob", http.MethodPost, "/evaluation-jobs", "CreateEvaluationJob"},
		{"BatchDeleteEvaluationJob", http.MethodPost, "/evaluation-jobs/batch-delete", "BatchDeleteEvaluationJob"},
		{
			"CreateAutomatedReasoningPolicy",
			http.MethodPost,
			"/automated-reasoning-policies",
			"CreateAutomatedReasoningPolicy",
		},
		{
			"CreateAutomatedReasoningPolicyTestCase",
			http.MethodPost,
			"/automated-reasoning-policies/arn/test-cases",
			"CreateAutomatedReasoningPolicyTestCase",
		},
		{
			"CreateAutomatedReasoningPolicyVersion",
			http.MethodPost,
			"/automated-reasoning-policies/arn/versions",
			"CreateAutomatedReasoningPolicyVersion",
		},
		{
			"CancelAutomatedReasoningPolicyBuildWorkflow",
			http.MethodPost,
			"/automated-reasoning-policies/arn/build-workflows/wf-001/cancel",
			"CancelAutomatedReasoningPolicyBuildWorkflow",
		},
		{"CreateCustomModel", http.MethodPost, "/custom-models/create-custom-model", "CreateCustomModel"},
		{
			"CreateCustomModelDeployment",
			http.MethodPost,
			"/model-customization/custom-model-deployments",
			"CreateCustomModelDeployment",
		},
		{
			"CreateFoundationModelAgreement",
			http.MethodPost,
			"/create-foundation-model-agreement",
			"CreateFoundationModelAgreement",
		},
		{"CreateGuardrailVersion", http.MethodPost, "/guardrails/my-guardrail-id", "CreateGuardrailVersion"},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

func TestHandler_NewOperationsBadJSON(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	e := echo.New()

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/evaluation-jobs"},
		{http.MethodPost, "/evaluation-jobs/batch-delete"},
		{http.MethodPost, "/automated-reasoning-policies"},
		{http.MethodPost, "/custom-models/create-custom-model"},
		{http.MethodPost, "/model-customization/custom-model-deployments"},
		{http.MethodPost, "/create-foundation-model-agreement"},
		{http.MethodPost, "/guardrails/some-id"},
	}

	for _, p := range paths { //nolint:paralleltest // existing issue.
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			req := httptest.NewRequest(p.method, p.path, bytes.NewReader([]byte("bad json")))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_GetSupportedOperationsIncludes(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	ops := h.GetSupportedOperations()

	for _, op := range []string{
		"BatchDeleteEvaluationJob",
		"CancelAutomatedReasoningPolicyBuildWorkflow",
		"CreateAutomatedReasoningPolicy",
		"CreateAutomatedReasoningPolicyTestCase",
		"CreateAutomatedReasoningPolicyVersion",
		"CreateCustomModel",
		"CreateCustomModelDeployment",
		"CreateEvaluationJob",
		"CreateFoundationModelAgreement",
		"CreateGuardrailVersion",
		// parity-4 additions.
		"CreateAdvancedPromptOptimizationJob",
		"GetAdvancedPromptOptimizationJob",
		"ListAdvancedPromptOptimizationJobs",
		"StopAdvancedPromptOptimizationJob",
		"BatchDeleteAdvancedPromptOptimizationJob",
		"GetAccountDataRetention",
		"PutAccountDataRetention",
		"GetResourcePolicy",
		"PutResourcePolicy",
		"DeleteResourcePolicy",
	} {
		assert.Contains(t, ops, op)
	}
}

func TestHandler_BackendReset(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	// Create resources.
	_, err := h.Backend.CreateGuardrail("reset-guardrail", "", "", "", nil)
	require.NoError(t, err)

	_, err = h.Backend.CreateEvaluationJob("reset-job", nil)
	require.NoError(t, err)

	_, err = h.Backend.CreateAutomatedReasoningPolicy("reset-policy", "", nil)
	require.NoError(t, err)

	// Verify resources exist.
	rec := doRequest(t, h, http.MethodGet, "/guardrails", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var listOut map[string]any
	mustUnmarshal(t, rec, &listOut)
	assert.Len(t, listOut["guardrails"].([]any), 1)

	// Reset.
	h.Backend.Reset()

	// Verify resources are gone.
	rec2 := doRequest(t, h, http.MethodGet, "/guardrails", nil)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var listOut2 map[string]any
	mustUnmarshal(t, rec2, &listOut2)
	assert.Empty(t, listOut2["guardrails"].([]any))

	// Verify foundation models still seeded after reset.
	rec3 := doRequest(t, h, http.MethodGet, "/foundation-models", nil)
	assert.Equal(t, http.StatusOK, rec3.Code)

	var fmOut map[string]any
	mustUnmarshal(t, rec3, &fmOut)
	assert.NotEmpty(t, fmOut["modelSummaries"].([]any))
}

func TestHandler_Provider(t *testing.T) { //nolint:paralleltest // existing issue.
	p := &bedrock.Provider{}
	assert.Equal(t, "Bedrock", p.Name())

	h, err := p.Init(&service.AppContext{})
	require.NoError(t, err)
	require.NotNil(t, h)
}

// TestHandler_RealSDKWireShapeRouteMatcher guards against a whole class of latent bugs:
// unit tests calling h.Handler()(c) directly skip RouteMatcher, so an op can look "done"
// while a real aws-sdk-go-v2 client would never reach it (wrong HTTP method — PUT vs the
// real PATCH — or wrong singular/plural path segment). Every path/method pair below is
// exactly what the real SDK serializer emits (verified against
// aws-sdk-go-v2/service/bedrock@v1.56.0's serializers.go), so RouteMatcher must accept
// all of them and Handler() must not 404/return "Unknown".
func TestHandler_RealSDKWireShapeRouteMatcher(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name   string
		method string
		path   string
	}{
		// StopEvaluationJob: singular "evaluation-job", not the plural
		// "evaluation-jobs" used by every other evaluation job op.
		{"StopEvaluationJob", http.MethodPost, "/evaluation-job/some-arn/stop"},
		// CreateModelInvocationJob/GetModelInvocationJob/StopModelInvocationJob: singular
		// "model-invocation-job"; only ListModelInvocationJobs uses the plural.
		{"CreateModelInvocationJob", http.MethodPost, "/model-invocation-job"},
		{"GetModelInvocationJob", http.MethodGet, "/model-invocation-job/some-arn"},
		{"StopModelInvocationJob", http.MethodPost, "/model-invocation-job/some-arn/stop"},
		// UpdateProvisionedModelThroughput: PATCH, not PUT.
		{"UpdateProvisionedModelThroughput", http.MethodPatch, "/provisioned-model-throughput/pmt-1"},
		// UpdateMarketplaceModelEndpoint: PATCH, not PUT.
		{"UpdateMarketplaceModelEndpoint", http.MethodPatch, "/marketplace-model/endpoints/ep-1"},
		// DeregisterMarketplaceModelEndpoint: DELETE on the SAME "/registration" path
		// Register uses — there is no separate "/deregistration" path.
		{"DeregisterMarketplaceModelEndpoint", http.MethodDelete, "/marketplace-model/endpoints/ep-1/registration"},
		// CustomModelDeployment List/Get/Update/Delete share Create's base path
		// ("/model-customization/custom-model-deployments"), not "/custom-model-deployments".
		{"ListCustomModelDeployments", http.MethodGet, "/model-customization/custom-model-deployments"},
		{"GetCustomModelDeployment", http.MethodGet, "/model-customization/custom-model-deployments/dep-1"},
		{
			"UpdateCustomModelDeployment", http.MethodPatch,
			"/model-customization/custom-model-deployments/dep-1",
		},
		{
			"DeleteCustomModelDeployment", http.MethodDelete,
			"/model-customization/custom-model-deployments/dep-1",
		},
		// BatchDeleteAdvancedPromptOptimizationJob: singular
		// "advanced-prompt-optimization-job", not the plural
		// "advanced-prompt-optimization-jobs" every other op in this family uses
		// (verified against aws-sdk-go-v2/service/bedrock@v1.66.4's serializers.go).
		{
			"BatchDeleteAdvancedPromptOptimizationJob", http.MethodPost,
			"/advanced-prompt-optimization-job/batch-delete",
		},
	}

	for _, tt := range tests { //nolint:paralleltest // existing issue.
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			matcher := h.RouteMatcher()
			assert.True(t, matcher(c), "RouteMatcher must accept the real SDK path/method")

			op := h.ExtractOperation(c)
			assert.Equal(t, tt.name, op, "ExtractOperation must resolve the real op name")

			// A resource-not-found 404 (ResourceNotFoundException, since these IDs don't
			// exist) is expected AWS behavior. A route-not-found 404
			// (UnknownOperationException, from dispatch's final fallback) means the real
			// SDK request would never have reached a handler at all — that's the bug this
			// test guards against.
			req2 := httptest.NewRequest(tt.method, tt.path, nil)
			rec2 := httptest.NewRecorder()
			c2 := e.NewContext(req2, rec2)
			require.NoError(t, h.Handler()(c2))

			var out map[string]string
			_ = json.Unmarshal(rec2.Body.Bytes(), &out)
			assert.NotEqual(
				t, "UnknownOperationException", out["__type"],
				"path/method must be dispatched to a real handler, not fall through as unrouted",
			)
		})
	}
}

func TestAccuracy_Bedrock_Handler_StartWorker(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Verify that StartWorker / Shutdown can be called without panic.
	err := h.StartWorker(t.Context())
	require.NoError(t, err)

	// Allow the janitor a short window to run.
	time.Sleep(20 * time.Millisecond)

	shutdownCtx := t.Context()
	h.Shutdown(shutdownCtx)
}

func TestAccuracy_StubRoutes_Return200(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		body   map[string]any
		name   string
		method string
		path   string
	}{
		{
			name:   "get logging config",
			method: http.MethodGet,
			path:   "/logging/modelinvocations",
		},
		{
			name:   "list inference profiles empty",
			method: http.MethodGet,
			path:   "/inference-profiles",
		},
		{
			name:   "list marketplace endpoints empty",
			method: http.MethodGet,
			path:   "/marketplace-model/endpoints",
		},
		{
			name:   "list model invocation jobs empty",
			method: http.MethodGet,
			path:   "/model-invocation-jobs",
		},
		{
			name:   "list prompt routers empty",
			method: http.MethodGet,
			path:   "/prompt-routers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, tt.method, tt.path, tt.body)
			assert.Equal(t, http.StatusOK, rec.Code, "path %s returned %d: %s", tt.path, rec.Code, rec.Body.String())
		})
	}
}

func TestHandler_ListAutomatedReasoningPolicies(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodGet, "/automated-reasoning-policies", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	mustUnmarshal(t, rec, &out)
	assert.Empty(t, out["automatedReasoningPolicySummaries"])

	doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "pol-a"})
	doRequest(t, h, http.MethodPost, "/automated-reasoning-policies", map[string]any{"name": "pol-b"})

	rec2 := doRequest(t, h, http.MethodGet, "/automated-reasoning-policies", nil)
	var out2 map[string]any
	mustUnmarshal(t, rec2, &out2)
	assert.Len(t, out2["automatedReasoningPolicySummaries"], 2)
}

// TestParity_ValidationException_Returns400 verifies that invalid request
// bodies to Bedrock endpoints return HTTP 400 with a ValidationException error
// code rather than HTTP 500 InternalFailure.
func TestParity_ValidationException_Returns400(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   any
		name   string
		method string
		path   string
	}{
		{
			name:   "CreateModelCustomizationJob_null_body",
			method: http.MethodPost,
			path:   "/model-customization-jobs",
			body:   nil,
		},
		{
			name:   "CreateModelCustomizationJob_empty_object",
			method: http.MethodPost,
			path:   "/model-customization-jobs",
			body:   map[string]any{},
		},
		{
			name:   "CreateModelCopyJob_null_body",
			method: http.MethodPost,
			path:   "/model-copy-jobs",
			body:   nil,
		},
		{
			name:   "CreateModelCopyJob_empty_object",
			method: http.MethodPost,
			path:   "/model-copy-jobs",
			body:   map[string]any{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tc.method, tc.path, tc.body)

			assert.Equal(t, http.StatusBadRequest, rec.Code,
				"ValidationException must return 400, not 500")

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			errType, _ := resp["__type"].(string)
			assert.Contains(t, errType, "ValidationException",
				"error type must be ValidationException")
		})
	}
}

// TestParity_ValidationException_NotInternalError verifies that
// ValidationException is never confused with InternalFailure (HTTP 500).
func TestParity_ValidationException_NotInternalError(t *testing.T) {
	t.Parallel()

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/model-customization-jobs"},
		{http.MethodPost, "/model-copy-jobs"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+"_"+ep.path, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, ep.method, ep.path, nil)

			assert.NotEqual(t, http.StatusInternalServerError, rec.Code,
				"validation failure must not return 500")
		})
	}
}

// TestParity_ValidationException_ErrorShape verifies the response body shape
// for ValidationException matches the Bedrock error envelope.
func TestParity_ValidationException_ErrorShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{"model-customization-jobs", "/model-customization-jobs"},
		{"model-copy-jobs", "/model-copy-jobs"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, tc.path, nil)

			require.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			_, hasType := resp["__type"]
			assert.True(t, hasType, "error response must have '__type' field")

			_, hasMsg := resp["message"]
			assert.True(t, hasMsg, "error response must have 'message' field")
		})
	}
}

// advancedPromptOptimizationJobCreateBody builds a valid
// CreateAdvancedPromptOptimizationJob request body.
func advancedPromptOptimizationJobCreateBody(jobName string, modelConfigs ...map[string]any) map[string]any {
	if len(modelConfigs) == 0 {
		modelConfigs = []map[string]any{{"modelId": "amazon.titan-text-express-v1"}}
	}

	return map[string]any{
		"jobName":             jobName,
		"inputConfig":         map[string]any{"s3Uri": "s3://bucket/in"},
		"outputConfig":        map[string]any{"s3Uri": "s3://bucket/out"},
		"modelConfigurations": modelConfigs,
	}
}

// TestHandler_AdvancedPromptOptimizationJobLifecycle covers
// CreateAdvancedPromptOptimizationJob, GetAdvancedPromptOptimizationJob,
// ListAdvancedPromptOptimizationJobs, StopAdvancedPromptOptimizationJob, and
// BatchDeleteAdvancedPromptOptimizationJob. Each case builds its own isolated
// handler so subtests share no state and can run fully in parallel.
func TestHandler_AdvancedPromptOptimizationJobLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{
			name: "create returns a job arn",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				rec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-jobs",
					advancedPromptOptimizationJobCreateBody("job-a"))
				require.Equal(t, http.StatusOK, rec.Code)

				var out map[string]any
				mustUnmarshal(t, rec, &out)
				assert.NotEmpty(t, out["jobArn"])
			},
		},
		{
			name: "create missing jobName is a validation error",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				body := advancedPromptOptimizationJobCreateBody("")
				delete(body, "jobName")
				rec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-jobs", body)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create rejects more than 5 model configurations",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				configs := make([]map[string]any, 6)
				for i := range configs {
					configs[i] = map[string]any{"modelId": "amazon.titan-text-express-v1"}
				}
				rec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-jobs",
					advancedPromptOptimizationJobCreateBody("too-many-models", configs...))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "get after create returns InProgress and never fabricates a result",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				createRec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-jobs",
					advancedPromptOptimizationJobCreateBody("job-get"))
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut map[string]any
				mustUnmarshal(t, createRec, &createOut)
				jobARN, ok := createOut["jobArn"].(string)
				require.True(t, ok)

				getRec := doRequest(t, h, http.MethodGet,
					"/advanced-prompt-optimization-jobs/"+url.PathEscape(jobARN), nil)
				require.Equal(t, http.StatusOK, getRec.Code)

				var getOut map[string]any
				mustUnmarshal(t, getRec, &getOut)
				assert.Equal(t, "InProgress", getOut["jobStatus"])
				// Real AWS's GetAdvancedPromptOptimizationJobOutput has no field for the
				// optimized prompt text at all -- results are written to the caller's S3
				// OutputConfig location, outside the API response. This backend must not
				// fabricate one.
				assert.NotContains(t, getOut, "optimizedPrompt")
				assert.NotContains(t, getOut, "result")
			},
		},
		{
			name: "get nonexistent job is not found",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				rec := doRequest(t, h, http.MethodGet, "/advanced-prompt-optimization-jobs/does-not-exist", nil)
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "list returns created jobs",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				for _, name := range []string{"list-a", "list-b"} {
					rec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-jobs",
						advancedPromptOptimizationJobCreateBody(name))
					require.Equal(t, http.StatusOK, rec.Code)
				}

				listRec := doRequest(t, h, http.MethodGet, "/advanced-prompt-optimization-jobs", nil)
				require.Equal(t, http.StatusOK, listRec.Code)

				var listOut map[string]any
				mustUnmarshal(t, listRec, &listOut)
				assert.Len(t, listOut["jobSummaries"], 2)
			},
		},
		{
			// maxResults is a real ListAdvancedPromptOptimizationJobsInput field
			// (bedrock@v1.66.4 api_op_ListAdvancedPromptOptimizationJobs.go:30-31,
			// serialized as the "maxResults" query param at serializers.go:5661-5663)
			// that the handler must page by, not silently ignore.
			name: "list respects maxResults",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				for _, name := range []string{"page-a", "page-b", "page-c"} {
					rec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-jobs",
						advancedPromptOptimizationJobCreateBody(name))
					require.Equal(t, http.StatusOK, rec.Code)
				}

				listRec := doRequest(t, h, http.MethodGet, "/advanced-prompt-optimization-jobs?maxResults=1", nil)
				require.Equal(t, http.StatusOK, listRec.Code)

				var listOut map[string]any
				mustUnmarshal(t, listRec, &listOut)
				assert.Len(t, listOut["jobSummaries"], 1)
				assert.NotEmpty(t, listOut["nextToken"])
			},
		},
		{
			name: "stop transitions status to Stopped",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				createRec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-jobs",
					advancedPromptOptimizationJobCreateBody("job-stop"))
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut map[string]any
				mustUnmarshal(t, createRec, &createOut)
				jobARN, ok := createOut["jobArn"].(string)
				require.True(t, ok)

				stopRec := doRequest(t, h, http.MethodPost,
					"/advanced-prompt-optimization-jobs/"+url.PathEscape(jobARN)+"/stop", nil)
				assert.Equal(t, http.StatusOK, stopRec.Code)

				getRec := doRequest(t, h, http.MethodGet,
					"/advanced-prompt-optimization-jobs/"+url.PathEscape(jobARN), nil)
				var getOut map[string]any
				mustUnmarshal(t, getRec, &getOut)
				assert.Equal(t, "Stopped", getOut["jobStatus"])
			},
		},
		{
			name: "stopping an already-stopped job is a validation error",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				createRec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-jobs",
					advancedPromptOptimizationJobCreateBody("job-double-stop"))
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut map[string]any
				mustUnmarshal(t, createRec, &createOut)
				jobARN, ok := createOut["jobArn"].(string)
				require.True(t, ok)

				stopPath := "/advanced-prompt-optimization-jobs/" + url.PathEscape(jobARN) + "/stop"
				firstStop := doRequest(t, h, http.MethodPost, stopPath, nil)
				require.Equal(t, http.StatusOK, firstStop.Code)

				secondStop := doRequest(t, h, http.MethodPost, stopPath, nil)
				assert.Equal(t, http.StatusBadRequest, secondStop.Code)
			},
		},
		{
			name: "batch delete returns per-item success and error results",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				createRec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-jobs",
					advancedPromptOptimizationJobCreateBody("job-batch-delete"))
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut map[string]any
				mustUnmarshal(t, createRec, &createOut)
				jobARN, ok := createOut["jobArn"].(string)
				require.True(t, ok)

				rec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-job/batch-delete",
					map[string]any{"jobIdentifiers": []string{jobARN, "missing-job"}})
				require.Equal(t, http.StatusAccepted, rec.Code)

				var out map[string]any
				mustUnmarshal(t, rec, &out)
				assert.Len(t, out["advancedPromptOptimizationJobs"], 1)
				assert.Len(t, out["errors"], 1)

				getAfterDelete := doRequest(t, h, http.MethodGet,
					"/advanced-prompt-optimization-jobs/"+url.PathEscape(jobARN), nil)
				assert.Equal(t, http.StatusNotFound, getAfterDelete.Code)
			},
		},
		{
			name: "batch delete with no identifiers is a validation error",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				rec := doRequest(t, h, http.MethodPost, "/advanced-prompt-optimization-job/batch-delete",
					map[string]any{"jobIdentifiers": []string{}})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

// TestHandler_AccountDataRetention covers GetAccountDataRetention and
// PutAccountDataRetention. Each case builds its own isolated handler.
func TestHandler_AccountDataRetention(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mode       string
		wantStatus int
	}{
		{name: "default mode", mode: "default", wantStatus: http.StatusOK},
		{name: "none mode", mode: "none", wantStatus: http.StatusOK},
		{name: "provider_data_share mode", mode: "provider_data_share", wantStatus: http.StatusOK},
		{name: "inherit mode", mode: "inherit", wantStatus: http.StatusOK},
		{name: "invalid mode is a validation error", mode: "bogus", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// An account that has never called Put still gets a value back,
			// defaulting to "default" -- GetAccountDataRetentionOutput.Mode is a
			// required field on the real wire shape.
			getBefore := doRequest(t, h, http.MethodGet, "/data-retention", nil)
			require.Equal(t, http.StatusOK, getBefore.Code)

			var beforeOut map[string]any
			mustUnmarshal(t, getBefore, &beforeOut)
			assert.Equal(t, "default", beforeOut["mode"])

			putRec := doRequest(t, h, http.MethodPut, "/data-retention", map[string]any{"mode": tt.mode})
			require.Equal(t, tt.wantStatus, putRec.Code)

			if tt.wantStatus != http.StatusOK {
				return
			}

			var putOut map[string]any
			mustUnmarshal(t, putRec, &putOut)
			assert.Equal(t, tt.mode, putOut["mode"])
			assert.NotEmpty(t, putOut["updatedAt"])

			getAfter := doRequest(t, h, http.MethodGet, "/data-retention", nil)
			require.Equal(t, http.StatusOK, getAfter.Code)

			var afterOut map[string]any
			mustUnmarshal(t, getAfter, &afterOut)
			assert.Equal(t, tt.mode, afterOut["mode"])
		})
	}
}

// TestHandler_ResourcePolicy covers core bedrock's PutResourcePolicy,
// GetResourcePolicy, and DeleteResourcePolicy. Each case builds its own
// isolated handler so subtests share no state and can run fully in parallel.
func TestHandler_ResourcePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{
			name: "put get delete lifecycle on a real guardrail resource",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				g, err := h.Backend.CreateGuardrail("policy-guardrail", "", "", "", nil)
				require.NoError(t, err)

				putRec := doRequest(t, h, http.MethodPost, "/resource-policy", map[string]any{
					"resourceArn":    g.GuardrailArn,
					"resourcePolicy": `{"Version":"2012-10-17","Statement":[]}`,
				})
				require.Equal(t, http.StatusCreated, putRec.Code)

				var putOut map[string]any
				mustUnmarshal(t, putRec, &putOut)
				assert.Equal(t, g.GuardrailArn, putOut["resourceArn"])

				getRec := doRequest(t, h, http.MethodGet,
					"/resource-policy/"+url.PathEscape(g.GuardrailArn), nil)
				require.Equal(t, http.StatusOK, getRec.Code)

				var getOut map[string]any
				mustUnmarshal(t, getRec, &getOut)
				policy, ok := getOut["resourcePolicy"].(string)
				require.True(t, ok)
				assert.Contains(t, policy, "Statement")

				delRec := doRequest(t, h, http.MethodDelete,
					"/resource-policy/"+url.PathEscape(g.GuardrailArn), nil)
				assert.Equal(t, http.StatusOK, delRec.Code)

				getAfterDelete := doRequest(t, h, http.MethodGet,
					"/resource-policy/"+url.PathEscape(g.GuardrailArn), nil)
				assert.Equal(t, http.StatusNotFound, getAfterDelete.Code)
			},
		},
		{
			// PutResourcePolicy's deserializer declares no
			// ResourceNotFoundException (bedrock@v1.66.4 deserializers.go),
			// so a nonexistent target reports ValidationException/400, not
			// ResourceNotFoundException/404.
			name: "put on a nonexistent resource is a validation error",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				rec := doRequest(t, h, http.MethodPost, "/resource-policy", map[string]any{
					"resourceArn":    "arn:aws:bedrock:us-east-1:000000000000:guardrail/does-not-exist",
					"resourcePolicy": `{}`,
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "put with a non-bedrock ARN is a validation error",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				rec := doRequest(t, h, http.MethodPost, "/resource-policy", map[string]any{
					"resourceArn":    "not-an-arn",
					"resourcePolicy": `{}`,
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "put with empty policy document is a validation error",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				g, err := h.Backend.CreateGuardrail("policy-guardrail-2", "", "", "", nil)
				require.NoError(t, err)

				rec := doRequest(t, h, http.MethodPost, "/resource-policy", map[string]any{
					"resourceArn": g.GuardrailArn,
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "get on a resource with no policy attached is not found",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				g, err := h.Backend.CreateGuardrail("policy-guardrail-3", "", "", "", nil)
				require.NoError(t, err)

				rec := doRequest(t, h, http.MethodGet,
					"/resource-policy/"+url.PathEscape(g.GuardrailArn), nil)
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}
