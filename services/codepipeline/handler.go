package codepipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	codepipelineTargetPrefix = "CodePipeline_20150709."

	// transitionTypeInbound and transitionTypeOutbound are the valid values for StageTransitionType.
	transitionTypeInbound = "Inbound"

	// keyOwnerCustom is the owner value for custom action types.
	keyOwnerCustom = "Custom"

	transitionTypeOutbound = "Outbound"
)

var (
	errUnknownAction  = errors.New("unknown action")
	errInvalidRequest = errors.New("invalid request")
)

// cpParseNextToken converts an opaque NextToken string to a slice start index.
func cpParseNextToken(token string) int {
	if token == "" {
		return 0
	}

	idx, err := strconv.Atoi(token)
	if err != nil || idx < 0 {
		return 0
	}

	return idx
}

// cpPaginate applies MaxResults/NextToken pagination to a slice.
// maxResultsCap is the per-operation maximum. A zero maxResults means "use cap".
func cpPaginate[T any](
	items []T,
	nextToken string,
	maxResults int32,
	maxResultsCap int32,
) ([]T, string, error) {
	limit := maxResultsCap

	if maxResults > 0 {
		if maxResults > maxResultsCap {
			return nil, "", fmt.Errorf(
				"%w: maxResults must be between 1 and %d",
				errInvalidRequest,
				maxResultsCap,
			)
		}

		limit = maxResults
	}

	start := cpParseNextToken(nextToken)

	if start >= len(items) {
		return items[:0], "", nil
	}

	end := start + int(limit)

	var outToken string

	if end < len(items) {
		outToken = strconv.Itoa(end)
	} else {
		end = len(items)
	}

	return items[start:end], outToken, nil
}

// emptyOut is the shared empty-response body for handlers whose operation
// returns no output fields.
type emptyOut struct{}

// tagsToMap converts a wire []Tag to the plain map[string]string the backend stores.
func tagsToMap(tags []Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[t.Key] = t.Value
	}

	return m
}

// Handler is the Echo HTTP handler for CodePipeline operations.
type Handler struct {
	Backend *InMemoryBackend
	ops     map[string]service.JSONOpFunc
}

// NewHandler creates a new CodePipeline handler backed by backend.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{Backend: backend}
	h.ops = h.dispatchTable()

	return h
}

// Reset clears all handler and backend state.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Name returns the service name.
func (h *Handler) Name() string { return "CodePipeline" }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"AcknowledgeJob",
		"AcknowledgeThirdPartyJob",
		"CreateCustomActionType",
		"CreatePipeline",
		"DeleteCustomActionType",
		"DeletePipeline",
		"DeleteWebhook",
		"DeregisterWebhookWithThirdParty",
		"DisableStageTransition",
		"EnableStageTransition",
		"GetActionType",
		"GetJobDetails",
		"GetPipeline",
		"ListPipelines",
		"ListTagsForResource",
		"TagResource",
		"UntagResource",
		"UpdatePipeline",
		"GetPipelineExecution",
		"GetPipelineState",
		"GetThirdPartyJobDetails",
		"ListActionExecutions",
		"ListActionTypes",
		"ListDeployActionExecutionTargets",
		"ListPipelineExecutions",
		"ListRuleExecutions",
		"ListRuleTypes",
		"ListWebhooks",
		"OverrideStageCondition",
		"PollForJobs",
		"PollForThirdPartyJobs",
		"PutActionRevision",
		"PutApprovalResult",
		"PutJobFailureResult",
		"PutJobSuccessResult",
		"PutThirdPartyJobFailureResult",
		"PutThirdPartyJobSuccessResult",
		"PutWebhook",
		"RegisterWebhookWithThirdParty",
		"RetryStageExecution",
		"RollbackStage",
		"StartPipelineExecution",
		"StopPipelineExecution",
		"UpdateActionType",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "codepipeline" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches CodePipeline requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), codepipelineTargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the CodePipeline action from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")

	return strings.TrimPrefix(target, codepipelineTargetPrefix)
}

// ExtractResource extracts the resource identifier from the request (not used for CodePipeline).
func (h *Handler) ExtractResource(_ *echo.Context) string {
	return ""
}

// Handler returns the Echo handler function for CodePipeline requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		// Resolve the per-request region (from SigV4 / X-Amz-Region) and attach
		// it to the context so backend operations are region-scoped.
		region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())

		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"CodePipeline", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			func(ctx context.Context, action string, body []byte) ([]byte, error) {
				return h.dispatch(context.WithValue(ctx, regionContextKey{}, region), action, body)
			},
			h.handleError,
		)
	}
}

func (h *Handler) dispatchTable() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"AcknowledgeJob":                  service.WrapOp(h.handleAcknowledgeJob),
		"AcknowledgeThirdPartyJob":        service.WrapOp(h.handleAcknowledgeThirdPartyJob),
		"CreateCustomActionType":          service.WrapOp(h.handleCreateCustomActionType),
		"CreatePipeline":                  service.WrapOp(h.handleCreatePipeline),
		"DeleteCustomActionType":          service.WrapOp(h.handleDeleteCustomActionType),
		"DeletePipeline":                  service.WrapOp(h.handleDeletePipeline),
		"DeleteWebhook":                   service.WrapOp(h.handleDeleteWebhook),
		"DeregisterWebhookWithThirdParty": service.WrapOp(h.handleDeregisterWebhookWithThirdParty),
		"DisableStageTransition":          service.WrapOp(h.handleDisableStageTransition),
		"EnableStageTransition":           service.WrapOp(h.handleEnableStageTransition),
		"GetActionType":                   service.WrapOp(h.handleGetActionType),
		"GetJobDetails":                   service.WrapOp(h.handleGetJobDetails),
		"GetPipeline":                     service.WrapOp(h.handleGetPipeline),
		"ListPipelines":                   service.WrapOp(h.handleListPipelines),
		"ListTagsForResource":             service.WrapOp(h.handleListTagsForResource),
		"TagResource":                     service.WrapOp(h.handleTagResource),
		"UntagResource":                   service.WrapOp(h.handleUntagResource),
		"UpdatePipeline":                  service.WrapOp(h.handleUpdatePipeline),
		"GetPipelineExecution":            service.WrapOp(h.handleGetPipelineExecution),
		"GetPipelineState":                service.WrapOp(h.handleGetPipelineState),
		"GetThirdPartyJobDetails":         service.WrapOp(h.handleGetThirdPartyJobDetails),
		"ListActionExecutions":            service.WrapOp(h.handleListActionExecutions),
		"ListActionTypes":                 service.WrapOp(h.handleListActionTypes),
		"ListDeployActionExecutionTargets": service.WrapOp(
			h.handleListDeployActionExecutionTargets,
		),
		"ListPipelineExecutions":        service.WrapOp(h.handleListPipelineExecutions),
		"ListRuleExecutions":            service.WrapOp(h.handleListRuleExecutions),
		"ListRuleTypes":                 service.WrapOp(h.handleListRuleTypes),
		"ListWebhooks":                  service.WrapOp(h.handleListWebhooks),
		"OverrideStageCondition":        service.WrapOp(h.handleOverrideStageCondition),
		"PollForJobs":                   service.WrapOp(h.handlePollForJobs),
		"PollForThirdPartyJobs":         service.WrapOp(h.handlePollForThirdPartyJobs),
		"PutActionRevision":             service.WrapOp(h.handlePutActionRevision),
		"PutApprovalResult":             service.WrapOp(h.handlePutApprovalResult),
		"PutJobFailureResult":           service.WrapOp(h.handlePutJobFailureResult),
		"PutJobSuccessResult":           service.WrapOp(h.handlePutJobSuccessResult),
		"PutThirdPartyJobFailureResult": service.WrapOp(h.handlePutThirdPartyJobFailureResult),
		"PutThirdPartyJobSuccessResult": service.WrapOp(h.handlePutThirdPartyJobSuccessResult),
		"PutWebhook":                    service.WrapOp(h.handlePutWebhook),
		"RegisterWebhookWithThirdParty": service.WrapOp(h.handleRegisterWebhookWithThirdParty),
		"RetryStageExecution":           service.WrapOp(h.handleRetryStageExecution),
		"RollbackStage":                 service.WrapOp(h.handleRollbackStage),
		"StartPipelineExecution":        service.WrapOp(h.handleStartPipelineExecution),
		"StopPipelineExecution":         service.WrapOp(h.handleStopPipelineExecution),
		"UpdateActionType":              service.WrapOp(h.handleUpdateActionType),
	}
}

func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnknownAction, action)
	}

	result, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	type errMapping struct {
		sentinel error
		errType  string
	}

	sentinels := []errMapping{
		{ErrPipelineNameInUse, "PipelineNameInUseException"},
		{ErrNotFound, "PipelineNotFoundException"},
		{ErrActionTypeNotFound, "ActionTypeNotFoundException"},
		{ErrJobNotFound, "JobNotFoundException"},
		{ErrWebhookNotFound, "WebhookNotFoundException"},
		{ErrAlreadyExists, "InvalidStructureException"},
		{ErrValidation, "ValidationException"},
		{ErrConflict, "ConflictException"},
		// ErrResourceInUse fires only from DeleteCustomActionType (the sole
		// call site, custom_action_types.go). "ResourceInUseException" names
		// no type CodePipeline defines anywhere (aws-sdk-go-v2/service/
		// codepipeline@v1.49.4/types/errors.go has no such type), and
		// DeleteCustomActionType's own deserializeOpErrorDeleteCustomActionType
		// (deserializers.go:560) models only ConcurrentModificationException
		// and ValidationException -- neither fits "referenced by a pipeline".
		// Left unfixed: no operation here models a code for this failure.
		{ErrResourceInUse, "ResourceInUseException"},
		{ErrResourceNotFound, "ResourceNotFoundException"},
		{ErrStageNotFound, "StageNotFoundException"},
		{ErrInvalidStructure, "InvalidStructureException"},
		{ErrExecutionNotFound, "PipelineExecutionNotFoundException"},
		{ErrVersionNotFound, "PipelineVersionNotFoundException"},
		{ErrActionNotFound, "ActionNotFoundException"},
		{ErrInvalidApprovalToken, "InvalidApprovalTokenException"},
		{ErrApprovalAlreadyCompleted, "ApprovalAlreadyCompletedException"},
		{ErrStageNotRetryable, "StageNotRetryableException"},
		{ErrUnableToRollbackStage, "UnableToRollbackStageException"},
		{ErrActionExecutionNotFound, "ActionExecutionNotFoundException"},
		{ErrInvalidClientToken, "InvalidClientTokenException"},
		// errUnknownAction fires when the routed Action string matches no
		// known CodePipeline operation -- a dispatch-level condition no
		// operation's own deserializer models (there is no operation to
		// consult), so this deliberately keeps the pre-existing fallback
		// code rather than inventing one (same reasoning as codedeploy's
		// own errUnknownAction row, 5e0b4978a).
		{errUnknownAction, "InvalidActionException"},
		{errInvalidRequest, "ValidationException"},
	}

	for _, m := range sentinels {
		if errors.Is(err, m.sentinel) {
			return errorBlob(c, http.StatusBadRequest, m.errType, err)
		}
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return errorBlob(c, http.StatusBadRequest, "ValidationException", err)
	}

	return errorBlob(c, http.StatusInternalServerError, "InternalFailure", err)
}

// errorBlob marshals a JSON error response and writes it to the echo context.
func errorBlob(c *echo.Context, status int, errType string, err error) error {
	payload, _ := json.Marshal(service.JSONErrorResponse{
		Type:    errType,
		Message: err.Error(),
	})

	return c.JSONBlob(status, payload)
}
