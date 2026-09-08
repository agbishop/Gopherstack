package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	guardrailsPrefix            = "/guardrails"
	foundationModelsPrefix      = "/foundation-models"
	provisionedModelThroughput  = "/provisioned-model-throughput"
	provisionedModelThroughputs = "/provisioned-model-throughputs"
	listTagsForResourcePath     = "/listTagsForResource"
	tagResourcePath             = "/tagResource"
	untagResourcePath           = "/untagResource"
	evaluationJobsPrefix        = "/evaluation-jobs"
	evaluationJobsBatchDelete   = "/evaluation-jobs/batch-delete"
	// evaluationJobSingularPrefix is the real StopEvaluationJob path family: AWS uses
	// the SINGULAR "evaluation-job" (no "s") for POST .../stop, distinct from the
	// plural "evaluation-jobs" used by every other evaluation job op.
	evaluationJobSingularPrefix  = "/evaluation-job"
	automatedReasoningPrefix     = "/automated-reasoning-policies"
	customModelsCreate           = "/custom-models/create-custom-model"
	customModelDeploymentsPath   = "/model-customization/custom-model-deployments"
	foundationModelAgreement     = "/create-foundation-model-agreement"
	customModelsPrefix           = "/custom-models"
	modelCustomizationJobsPrefix = "/model-customization-jobs"
	inferenceProfilesPrefix      = "/inference-profiles"
	marketplaceEndpointsPrefix   = "/marketplace-model/endpoints"
	loggingConfigPath            = "/logging/modelinvocations"
	// advancedPromptOptimizationJobsPrefix is the real (plural) path for
	// Create/Get/List/Stop AdvancedPromptOptimizationJob.
	advancedPromptOptimizationJobsPrefix = "/advanced-prompt-optimization-jobs"
	// advancedPromptOptimizationJobSingularPrefix is the real SINGULAR path
	// family for BatchDeleteAdvancedPromptOptimizationJob (POST
	// .../advanced-prompt-optimization-job/batch-delete) -- same
	// singular/plural split as evaluationJobSingularPrefix and
	// modelInvocationJobSingularPrefix above. It is also a strict prefix of
	// advancedPromptOptimizationJobsPrefix, so a single HasPrefix check
	// against it alone covers both the singular batch-delete path and every
	// plural path.
	advancedPromptOptimizationJobSingularPrefix  = "/advanced-prompt-optimization-job"
	advancedPromptOptimizationJobBatchDeletePath = "/advanced-prompt-optimization-job/batch-delete"
	// dataRetentionPath is the real GetAccountDataRetention/
	// PutAccountDataRetention path.
	dataRetentionPath = "/data-retention"
	// resourcePolicyPath is the real core-bedrock PutResourcePolicy/
	// GetResourcePolicy/DeleteResourcePolicy path prefix (POST
	// /resource-policy, GET/DELETE /resource-policy/{resourceArn}). Distinct
	// from bedrock-agent's "/resourcepolicy/{resourceArn}" (no hyphen,
	// singular) -- see resource_policy.go's package doc comment.
	resourcePolicyPath = "/resource-policy"

	// Response key constants.
	keyJobArn                   = "jobArn"
	keyStatus                   = "status"
	keyDeploymentArn            = "deploymentArn"
	keyJobName                  = "jobName"
	keyCreationTime             = "creationTime"
	keyLastModifiedTime         = "lastModifiedTime"
	keyPolicyArn                = "policyArn"
	keyBuildWorkflowID          = "buildWorkflowId"
	keyCreatedAt                = "createdAt"
	jobStatusCompleted          = "Completed"
	keyTestCaseID               = "testCaseId"
	keyName                     = "name"
	keyUpdatedAt                = "updatedAt"
	keyModelArn                 = "modelArn"
	keyModelID                  = "modelId"
	keyPromptRouterArn          = "promptRouterArn"
	keyCustomModelDeploymentArn = "customModelDeploymentArn"
	keyRoleArn                  = "roleArn"

	// Stub operation paths.
	modelCopyJobsPrefix       = "/model-copy-jobs"
	modelImportJobsPrefix     = "/model-import-jobs"
	modelInvocationJobsPrefix = "/model-invocation-jobs"
	// modelInvocationJobSingularPrefix is the real path family for CreateModelInvocationJob,
	// GetModelInvocationJob, and StopModelInvocationJob: AWS uses the SINGULAR
	// "model-invocation-job" (no "s") for these, while ListModelInvocationJobs alone
	// uses the plural modelInvocationJobsPrefix above.
	modelInvocationJobSingularPrefix = "/model-invocation-job"
	promptRoutersPrefix              = "/prompt-routers"
	importedModelsPrefix             = "/imported-models"
	foundationModelAvailPath         = "/foundation-model-availability"
	// foundationModelAgreementOffersPath is the real ListFoundationModelAgreementOffers
	// path family: GET "/list-foundation-model-agreement-offers/{modelId}"; gopherstack
	// previously used the invented "/foundation-model-agreement-offers" (no modelId
	// path param) and modeled the wrong resource entirely -- see
	// foundation_model_agreements.go's ListFoundationModelAgreementOffers doc comment.
	foundationModelAgreementOffersPath = "/list-foundation-model-agreement-offers"
	// deleteFoundationModelAgreementPath is the real DeleteFoundationModelAgreement
	// path: POST "/delete-foundation-model-agreement" with modelId in the JSON body;
	// gopherstack previously used DELETE with modelId as a path suffix.
	deleteFoundationModelAgreementPath = "/delete-foundation-model-agreement"
	// useCaseForModelAccessPath is the real GetUseCaseForModelAccess /
	// PutUseCaseForModelAccess path; gopherstack previously used the typo'd
	// "/usecase-for-model-access" (no hyphen between "use" and "case").
	useCaseForModelAccessPath = "/use-case-for-model-access"
	// enforcedGuardrailsPath is the real PutEnforcedGuardrailConfiguration /
	// ListEnforcedGuardrailsConfiguration / DeleteEnforcedGuardrailConfiguration
	// path; gopherstack previously used the invented, kebab-case
	// "/enforced-guardrail-configuration" instead of AWS's camelCase
	// "/enforcedGuardrailsConfiguration". Delete additionally appends
	// "/{configId}" as a path parameter (real AWS has no query-param form).
	enforcedGuardrailsPath = "/enforcedGuardrailsConfiguration"
)

// isoTime is a [time.Time] that marshals as RFC3339.
type isoTime struct {
	time.Time
}

func (t isoTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Time.Format(time.RFC3339))
}

// Handler is the Echo HTTP handler for Amazon Bedrock operations.
type Handler struct {
	Backend       *InMemoryBackend
	janitorCancel context.CancelFunc
	janitorDone   chan struct{}
}

// NewHandler creates a new Bedrock handler backed by backend.
// backend must not be nil.
func NewHandler(backend *InMemoryBackend) *Handler {
	return &Handler{Backend: backend}
}

// StartWorker starts the background janitor for status advancement.
func (h *Handler) StartWorker(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	h.janitorCancel = cancel
	h.janitorDone = done

	go func() {
		defer close(done)
		h.Backend.RunJanitor(runCtx, defaultJanitorInterval)
	}()

	return nil
}

// Shutdown stops the background janitor.
func (h *Handler) Shutdown(ctx context.Context) {
	if h.janitorCancel != nil {
		h.janitorCancel()
	}

	if h.janitorDone != nil {
		select {
		case <-h.janitorDone:
		case <-ctx.Done():
		}
	}
}

var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
)

// Name returns the service name.
func (h *Handler) Name() string { return "Bedrock" }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return append(baseSupportedOperations(), parity4SupportedOperations()...)
}

// baseSupportedOperations returns every operation supported before the
// parity-4 SDK bump. Split out of GetSupportedOperations to keep both
// functions comfortably under the project's function-length gate.
func baseSupportedOperations() []string {
	return []string{
		"BatchDeleteEvaluationJob",
		"CancelAutomatedReasoningPolicyBuildWorkflow",
		"CreateAutomatedReasoningPolicy",
		"CreateAutomatedReasoningPolicyTestCase",
		"CreateAutomatedReasoningPolicyVersion",
		"CreateCustomModel",
		"CreateCustomModelDeployment",
		"CreateEvaluationJob",
		"CreateFoundationModelAgreement",
		"CreateGuardrail",
		"CreateGuardrailVersion",
		"CreateInferenceProfile",
		"CreateMarketplaceModelEndpoint",
		"CreateModelCustomizationJob",
		"CreateProvisionedModelThroughput",
		"DeleteCustomModel",
		"DeleteGuardrail",
		"DeleteInferenceProfile",
		"DeleteMarketplaceModelEndpoint",
		"DeleteModelInvocationLoggingConfiguration",
		"DeleteProvisionedModelThroughput",
		"DeregisterMarketplaceModelEndpoint",
		"GetCustomModel",
		"GetFoundationModel",
		"GetGuardrail",
		"GetInferenceProfile",
		"GetMarketplaceModelEndpoint",
		"GetModelCustomizationJob",
		"GetModelInvocationLoggingConfiguration",
		"GetProvisionedModelThroughput",
		"ListCustomModels",
		"ListFoundationModels",
		"ListGuardrails",
		"ListInferenceProfiles",
		"ListMarketplaceModelEndpoints",
		"ListModelCustomizationJobs",
		"ListProvisionedModelThroughputs",
		opListTagsForResource,
		"PutModelInvocationLoggingConfiguration",
		"RegisterMarketplaceModelEndpoint",
		"StopModelCustomizationJob",
		opTagResource,
		opUntagResource,
		"UpdateGuardrail",
		"UpdateMarketplaceModelEndpoint",
		"UpdateProvisionedModelThroughput",
		// Batch 2: real stateful ops implemented in this release.
		"CreateModelCopyJob",
		"CreateModelImportJob",
		"CreateModelInvocationJob",
		"CreatePromptRouter",
		"DeleteAutomatedReasoningPolicy",
		"DeleteAutomatedReasoningPolicyBuildWorkflow",
		"DeleteAutomatedReasoningPolicyTestCase",
		"DeleteCustomModelDeployment",
		"DeleteEnforcedGuardrailConfiguration",
		"DeleteFoundationModelAgreement",
		"DeleteImportedModel",
		"DeletePromptRouter",
		"ExportAutomatedReasoningPolicyVersion",
		"GetAutomatedReasoningPolicy",
		"GetAutomatedReasoningPolicyAnnotations",
		"GetAutomatedReasoningPolicyBuildWorkflow",
		"GetAutomatedReasoningPolicyBuildWorkflowResultAssets",
		"GetAutomatedReasoningPolicyNextScenario",
		"GetAutomatedReasoningPolicyTestCase",
		"GetAutomatedReasoningPolicyTestResult",
		"GetCustomModelDeployment",
		"GetEvaluationJob",
		"GetFoundationModelAvailability",
		"GetImportedModel",
		"GetModelCopyJob",
		"GetModelImportJob",
		"GetModelInvocationJob",
		"GetPromptRouter",
		"GetUseCaseForModelAccess",
		"ListAutomatedReasoningPolicies",
		"ListAutomatedReasoningPolicyBuildWorkflows",
		"ListAutomatedReasoningPolicyTestCases",
		"ListAutomatedReasoningPolicyTestResults",
		"ListCustomModelDeployments",
		"ListEnforcedGuardrailsConfiguration",
		"ListEvaluationJobs",
		"ListFoundationModelAgreementOffers",
		"ListImportedModels",
		"ListModelCopyJobs",
		"ListModelImportJobs",
		"ListModelInvocationJobs",
		"ListPromptRouters",
		"PutEnforcedGuardrailConfiguration",
		"PutUseCaseForModelAccess",
		"StartAutomatedReasoningPolicyBuildWorkflow",
		"StartAutomatedReasoningPolicyTestWorkflow",
		"StopEvaluationJob",
		"StopModelInvocationJob",
		"UpdateAutomatedReasoningPolicy",
		"UpdateAutomatedReasoningPolicyAnnotations",
		"UpdateAutomatedReasoningPolicyTestCase",
		"UpdateCustomModelDeployment",
	}
}

// parity4SupportedOperations returns the 10 ops added by the
// aws-sdk-go-v2/service/bedrock bump to v1.66.4 (see PARITY.md).
func parity4SupportedOperations() []string {
	return []string{
		"CreateAdvancedPromptOptimizationJob",
		"GetAdvancedPromptOptimizationJob",
		"ListAdvancedPromptOptimizationJobs",
		"StopAdvancedPromptOptimizationJob",
		"BatchDeleteAdvancedPromptOptimizationJob",
		"GetAccountDataRetention",
		"PutAccountDataRetention",
		opGetResourcePolicy,
		opPutResourcePolicy,
		opDeleteResourcePolicy,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "bedrock" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches Bedrock requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return matchBedrockPath(c.Request().URL.Path)
	}
}

// matchBedrockPath returns true if the path matches a known Bedrock API path.
func matchBedrockPath(path string) bool {
	return matchBedrockPrefixPaths(path) || matchBedrockExactPaths(path)
}

// matchBedrockPrefixPaths returns true if path has a known Bedrock prefix.
func matchBedrockPrefixPaths(path string) bool {
	return matchBedrockCorePrefixes(path) || matchBedrockExtPrefixes(path) || matchBedrockNewFamilyPrefixes(path)
}

// matchBedrockNewFamilyPrefixes checks the AdvancedPromptOptimizationJob and
// core-bedrock ResourcePolicy prefixes.
func matchBedrockNewFamilyPrefixes(path string) bool {
	return strings.HasPrefix(path, advancedPromptOptimizationJobSingularPrefix) ||
		strings.HasPrefix(path, resourcePolicyPath)
}

// matchBedrockCorePrefixes checks the core Bedrock resource prefixes.
func matchBedrockCorePrefixes(path string) bool {
	return strings.HasPrefix(path, guardrailsPrefix) ||
		strings.HasPrefix(path, foundationModelsPrefix) ||
		strings.HasPrefix(path, provisionedModelThroughput) ||
		strings.HasPrefix(path, evaluationJobsPrefix) ||
		strings.HasPrefix(path, evaluationJobSingularPrefix) ||
		strings.HasPrefix(path, automatedReasoningPrefix) ||
		strings.HasPrefix(path, modelCustomizationJobsPrefix) ||
		strings.HasPrefix(path, inferenceProfilesPrefix) ||
		strings.HasPrefix(path, marketplaceEndpointsPrefix) ||
		strings.HasPrefix(path, customModelsPrefix)
}

// matchBedrockExtPrefixes checks the extended Bedrock resource prefixes.
func matchBedrockExtPrefixes(path string) bool {
	return strings.HasPrefix(path, modelCopyJobsPrefix) ||
		strings.HasPrefix(path, modelImportJobsPrefix) ||
		// modelInvocationJobSingularPrefix ("/model-invocation-job") is a strict prefix of
		// the plural modelInvocationJobsPrefix ("/model-invocation-jobs"), so this one
		// check covers both the singular (Create/Get/Stop) and plural (List) paths.
		strings.HasPrefix(path, modelInvocationJobSingularPrefix) ||
		strings.HasPrefix(path, promptRoutersPrefix) ||
		strings.HasPrefix(path, importedModelsPrefix) ||
		strings.HasPrefix(path, foundationModelAvailPath) ||
		strings.HasPrefix(path, foundationModelAgreementOffersPath) ||
		// CustomModelDeployment List/Get/Update/Delete share the same base path as
		// Create ("/model-customization/custom-model-deployments"), with
		// Get/Update/Delete appending "/{id}" — a prefix match covers all of them.
		strings.HasPrefix(path, customModelDeploymentsPath)
}

// matchBedrockExactPaths returns true if path exactly matches a known Bedrock path.
func matchBedrockExactPaths(path string) bool {
	return path == useCaseForModelAccessPath ||
		path == enforcedGuardrailsPath ||
		// DeleteEnforcedGuardrailConfiguration appends "/{configId}".
		strings.HasPrefix(path, enforcedGuardrailsPath+"/") ||
		path == loggingConfigPath ||
		path == foundationModelAgreement ||
		path == deleteFoundationModelAgreementPath ||
		path == listTagsForResourcePath ||
		path == tagResourcePath ||
		path == untagResourcePath ||
		path == dataRetentionPath
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityPathVersioned }

// ExtractOperation returns the operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	path := c.Request().URL.Path
	method := c.Request().Method

	for _, fn := range []func(string, string) (string, bool){
		extractGuardrailOperation,
		extractFoundationModelOperation,
		extractPMTOperation,
		extractTagOperation,
		extractEvaluationJobOperation,
		extractARPOperation,
		extractCustomModelOperation,
		extractCustomModelListOperation,
		extractCustomizationJobOperation,
		extractInferenceProfileOperation,
		extractMarketplaceEndpointOperation,
		extractLoggingConfigOperation,
		extractModelInvocationJobOperation,
		extractAdvancedPromptOptimizationJobOperation,
		extractAccountDataRetentionOperation,
		extractResourcePolicyOperation,
		extractModelCopyImportOperation,
		extractPromptRouterOperation,
		extractEnforcedGuardrailConfigOperation,
		extractUseCaseForModelAccessOperation,
		extractFoundationModelStubOperation,
	} {
		if op, ok := fn(path, method); ok {
			return op
		}
	}

	return "Unknown"
}

// ExtractResource extracts a resource identifier from the request path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	path := c.Request().URL.Path

	if id, ok := strings.CutPrefix(path, guardrailsPrefix+"/"); ok {
		return id
	}

	if id, ok := strings.CutPrefix(path, foundationModelsPrefix+"/"); ok {
		return id
	}

	if id, ok := strings.CutPrefix(path, provisionedModelThroughput+"/"); ok {
		return id
	}

	if id, ok := strings.CutPrefix(path, automatedReasoningPrefix+"/"); ok {
		return id
	}

	if id, ok := strings.CutPrefix(path, customModelsPrefix+"/"); ok {
		return id
	}

	if id, ok := strings.CutPrefix(path, modelCustomizationJobsPrefix+"/"); ok {
		return id
	}

	if id, ok := strings.CutPrefix(path, inferenceProfilesPrefix+"/"); ok {
		return id
	}

	if id, ok := strings.CutPrefix(path, marketplaceEndpointsPrefix+"/"); ok {
		return id
	}

	return ""
}

// Handler returns the Echo handler function for Bedrock requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		path := r.URL.Path
		method := r.Method
		log := logger.Load(r.Context())

		var body []byte
		if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
			var err error
			body, err = httputils.ReadBody(r)
			if err != nil {
				log.ErrorContext(r.Context(), "bedrock: failed to read request body", "error", err)

				return c.JSON(
					http.StatusInternalServerError,
					errorResponse("InternalServerException", "internal server error"),
				)
			}
		}

		return h.dispatch(c, path, method, body)
	}
}

// dispatch routes a Bedrock request to the appropriate handler.
func (h *Handler) dispatch(c *echo.Context, path, method string, body []byte) error {
	if ok, err := h.routeGuardrail(c, path, method, body); ok {
		return err
	}
	if ok, err := h.routeFoundationModel(c, path, method); ok {
		return err
	}
	if ok, err := h.routePMT(c, path, method, body); ok {
		return err
	}
	if ok, err := h.routeTag(c, path, method, body); ok {
		return err
	}
	if ok, err := h.routeEvaluationJob(c, path, method, body); ok {
		return err
	}
	if ok, err := h.routeARP(c, path, method, body); ok {
		return err
	}
	if ok, err := h.routeCustomModel(c, path, method, body); ok {
		return err
	}
	if ok, err := h.routeCustomModelList(c, path, method); ok {
		return err
	}

	return h.dispatchExtended(c, path, method, body)
}

// dispatchExtended handles additional Bedrock route groups.
func (h *Handler) dispatchExtended(c *echo.Context, path, method string, body []byte) error {
	if ok, err := h.routeCustomizationJob(c, path, method, body); ok {
		return err
	}
	if ok, err := h.routeInferenceProfile(c, path, method, body); ok {
		return err
	}
	if ok, err := h.routeMarketplaceEndpoint(c, path, method, body); ok {
		return err
	}
	if ok, err := h.routeLoggingConfig(c, path, method, body); ok {
		return err
	}
	if ok, err := h.routeNewFamilies(c, path, method, body); ok {
		return err
	}

	if ok, err := h.routeStubOps(c, path, method, body); ok {
		return err
	}

	return c.JSON(
		http.StatusNotFound,
		errorResponse("UnknownOperationException", "unknown operation: "+path),
	)
}

// routeNewFamilies handles the AdvancedPromptOptimizationJob,
// AccountDataRetention, and core-bedrock ResourcePolicy route groups.
func (h *Handler) routeNewFamilies(c *echo.Context, path, method string, body []byte) (bool, error) {
	if ok, err := h.routeAdvancedPromptOptimizationJob(c, path, method, body); ok {
		return true, err
	}
	if ok, err := h.routeAccountDataRetention(c, path, method, body); ok {
		return true, err
	}

	return h.routeResourcePolicy(c, path, method, body)
}

// routeStubOps handles stub operations that return minimal valid responses.
func (h *Handler) routeStubOps(c *echo.Context, path, method string, body []byte) (bool, error) {
	if ok, err := h.routeStubJobOps(c, path, method); ok {
		return true, err
	}

	if ok, err := h.routeStubModelOps(c, path, method, body); ok {
		return true, err
	}

	return h.routeStubMiscOps(c, path, method)
}

// routeStubJobOps handles model copy, import, and invocation job stubs.
func (h *Handler) routeStubJobOps(c *echo.Context, path, method string) (bool, error) {
	if ok, err := h.routeStubCopyImportOps(c, path, method); ok {
		return true, err
	}

	return h.routeStubInvocationOps(c, path, method)
}

// routeStubModelOps handles prompt router, imported model, and foundation model stubs.
func (h *Handler) routeStubModelOps(c *echo.Context, path, method string, body []byte) (bool, error) {
	if ok, err := h.routeStubPromptRouterOps(c, path, method); ok {
		return true, err
	}

	return h.routeStubFoundationModelOps(c, path, method, body)
}

// routeStubMiscOps handles custom model deployment, use case, and enforced guardrail stubs.
func (h *Handler) routeStubMiscOps(c *echo.Context, path, method string) (bool, error) {
	if ok, err := h.routeStubDeploymentOps(c, path, method); ok {
		return true, err
	}

	if ok, err := h.routeUseCaseForModelAccess(c, path, method); ok {
		return true, err
	}

	return h.routeEnforcedGuardrailConfig(c, path, method)
}

func (h *Handler) writeError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, errorResponse("ResourceNotFoundException", err.Error()))
	case errors.Is(err, ErrAlreadyExists):
		return c.JSON(http.StatusConflict, errorResponse("ConflictException", err.Error()))
	case errors.Is(err, ErrValidation):
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", err.Error()))
	case errors.Is(err, ErrResourceInUse):
		return c.JSON(http.StatusConflict, errorResponse("ResourceInUseException", err.Error()))
	default:
		return c.JSON(http.StatusInternalServerError, errorResponse("InternalServerException", err.Error()))
	}
}

func errorResponse(code, msg string) map[string]string {
	return map[string]string{"message": msg, "__type": code}
}

// parseBody parses JSON bytes into a value of type T.
func parseBody[T any](body []byte) (*T, error) {
	var v T
	if len(body) == 0 {
		return &v, nil
	}

	if err := json.Unmarshal(body, &v); err != nil {
		return nil, err
	}

	return &v, nil
}

// decodePath URL-decodes a path segment (e.g., ARNs encoded with %3A).
func decodePath(s string) string {
	decoded, err := url.PathUnescape(s)
	if err != nil {
		return s
	}

	return decoded
}
