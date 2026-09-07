package stepfunctions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

var errUnknownOperation = errors.New("UnknownOperationException")

// Handler is the Echo HTTP service handler for Step Functions operations.
type Handler struct {
	Backend       StorageBackend
	svcCtx        context.Context
	tags          map[string]*tags.Tags
	tagsMu        *lockmetrics.RWMutex
	DefaultRegion string
}

// NewHandler creates a new Step Functions handler.
func NewHandler(backend StorageBackend) *Handler {
	svcCtx := context.Background()
	if bk, ok := backend.(*InMemoryBackend); ok {
		svcCtx = bk.svcCtx
	}

	return &Handler{
		Backend:       backend,
		DefaultRegion: config.DefaultRegion,
		tags:          make(map[string]*tags.Tags),
		tagsMu:        lockmetrics.New("sfn.tags"),
		svcCtx:        svcCtx,
	}
}

func (h *Handler) setTags(resourceID string, kv map[string]string) {
	h.tagsMu.Lock("setTags")
	defer h.tagsMu.Unlock()
	if h.tags[resourceID] == nil {
		h.tags[resourceID] = tags.New("sfn." + resourceID + ".tags")
	}
	h.tags[resourceID].Merge(kv)
}

func (h *Handler) removeTags(resourceID string, keys []string) {
	h.tagsMu.RLock("removeTags")
	defer h.tagsMu.RUnlock()

	t := h.tags[resourceID]
	if t != nil {
		t.DeleteKeys(keys)
	}
}

func (h *Handler) getTags(resourceID string) map[string]string {
	h.tagsMu.RLock("getTags")
	t := h.tags[resourceID]
	h.tagsMu.RUnlock()
	if t == nil {
		return map[string]string{}
	}

	return t.Clone()
}

// Name returns the service name.
func (h *Handler) Name() string { return "StepFunctions" }

// StartWorker starts the background janitor for execution pruning.
// It implements service.BackgroundWorker.
func (h *Handler) StartWorker(ctx context.Context) error {
	if sfnBk, ok := h.Backend.(*InMemoryBackend); ok {
		janitor := NewJanitor(sfnBk, sfnBk.settings)
		go janitor.Run(ctx)
	}

	return nil
}

// GetSupportedOperations returns all mocked Step Functions operations.
//
// NOTE: "DescribeStateMachineVersion" is deliberately absent -- it does not
// exist as an operation in real AWS Step Functions (verified against
// aws-sdk-go-v2/service/sfn, which has no api_op_DescribeStateMachineVersion.go).
// A prior gopherstack pass fabricated it; AWS's real mechanism for
// retrieving version details is calling DescribeStateMachine with a
// version-qualified ARN (stateMachineArn:N), which DescribeStateMachine now
// implements directly (see state_machines.go).
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CreateActivity",
		"CreateStateMachine",
		"CreateStateMachineAlias",
		"DeleteActivity",
		"DeleteStateMachine",
		"DeleteStateMachineAlias",
		"DeleteStateMachineVersion",
		"DescribeActivity",
		"DescribeExecution",
		"DescribeMapRun",
		"DescribeStateMachine",
		"DescribeStateMachineAlias",
		"DescribeStateMachineForExecution",
		"GetActivityTask",
		"GetExecutionHistory",
		"ListActivities",
		"ListExecutions",
		"ListMapRuns",
		"ListStateMachineAliases",
		"ListStateMachineVersions",
		"ListStateMachines",
		"ListTagsForResource",
		"PublishStateMachineVersion",
		"RedriveExecution",
		"SendTaskFailure",
		"SendTaskHeartbeat",
		"SendTaskSuccess",
		"StartExecution",
		"StartSyncExecution",
		"StopExecution",
		"TagResource",
		"TestState",
		"UntagResource",
		"UpdateMapRun",
		"UpdateStateMachine",
		"UpdateStateMachineAlias",
		"ValidateStateMachineDefinition",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "states" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this Step Functions instance handles.
func (h *Handler) ChaosRegions() []string {
	if h.DefaultRegion != "" {
		return []string{h.DefaultRegion}
	}

	return []string{config.DefaultRegion}
}

// RouteMatcher returns a matcher for Step Functions requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, "AmazonStates.") ||
			strings.HasPrefix(target, "AWSStepFunctions.")
	}
}

const stepFunctionsMatchPriority = 100

// MatchPriority returns the routing priority for the Step Functions handler.
func (h *Handler) MatchPriority() int { return stepFunctionsMatchPriority }

// ExtractOperation extracts the operation name from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	const targetParts = 2
	if len(parts) == targetParts {
		return parts[1]
	}

	return "Unknown"
}

// ExtractResource extracts the resource name from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	for _, key := range []string{"name", "stateMachineArn", "executionArn"} {
		if v, ok := data[key].(string); ok && v != "" {
			return v
		}
	}

	return ""
}

// Handler returns the Echo handler function for Step Functions requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		region := httputils.ExtractRegionFromRequest(c.Request(), h.DefaultRegion)
		ctx := context.WithValue(c.Request().Context(), regionContextKey{}, region)
		c.SetRequest(c.Request().WithContext(ctx))

		return service.HandleTarget(
			c, logger.Load(ctx),
			"StepFunctions", "application/x-amz-json-1.0",
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

type actionFn func([]byte) (any, error)

func (h *Handler) dispatchTable() map[string]actionFn {
	table := make(map[string]actionFn)
	maps.Copy(table, h.stateMachineActions())
	maps.Copy(table, h.executionActions())
	maps.Copy(table, h.activityActions())
	maps.Copy(table, h.utilActions())
	maps.Copy(table, h.mapRunActions())

	return table
}

// dispatch routes the action to the correct handler function.
func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	// Context-aware actions are handled directly before the dispatch table.
	switch action {
	case "GetActivityTask":
		return h.handleGetActivityTask(ctx, body)
	case "CreateStateMachine":
		resp, err := h.createStateMachineAction(ctx, body)
		if err != nil {
			return nil, err
		}

		return json.Marshal(resp)
	case "ListStateMachines":
		var input listStateMachinesInput
		if err := json.Unmarshal(body, &input); err != nil {
			return nil, err
		}

		sms, next, err := h.Backend.ListStateMachines(ctx, input.NextToken, input.MaxResults)
		if err != nil {
			return nil, err
		}

		items := make([]stateMachineListItem, len(sms))
		for i := range sms {
			items[i] = newStateMachineListItem(&sms[i])
		}

		return json.Marshal(&listStateMachinesOutput{StateMachines: items, NextToken: next})
	case "CreateActivity":
		resp, err := h.handleCreateActivity(ctx, body)
		if err != nil {
			return nil, err
		}

		return json.Marshal(resp)
	case "ListActivities":
		resp, err := h.handleListActivities(ctx, body)
		if err != nil {
			return nil, err
		}

		return json.Marshal(resp)
	}

	fn, ok := h.dispatchTable()[action]
	if !ok {
		return nil, fmt.Errorf("%w:%s", errUnknownOperation, action)
	}

	response, err := fn(body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(response)
}

// handleError writes a standardized JSON error response.
func (h *Handler) handleError(
	ctx context.Context,
	c *echo.Context,
	action string,
	reqErr error,
) error {
	log := logger.Load(ctx)
	c.Response().Header().Set("Content-Type", "application/x-amz-json-1.0")

	errType, statusCode := classifyError(reqErr)

	if statusCode == http.StatusInternalServerError {
		log.ErrorContext(ctx, "StepFunctions internal error", "error", reqErr, "action", action)
	} else {
		log.WarnContext(ctx, "StepFunctions request error", "error", reqErr, "action", action)
	}

	errResp := service.JSONErrorResponse{
		Type:    errType,
		Message: reqErr.Error(),
	}

	payload, _ := json.Marshal(errResp)

	return c.JSONBlob(statusCode, payload)
}

func classifyError(reqErr error) (string, int) {
	type mapping struct {
		err    error
		kind   string
		status int
	}

	mappings := []mapping{
		{ErrStateMachineDoesNotExist, "StateMachineDoesNotExist", http.StatusNotFound},
		{
			ErrStateMachineVersionDoesNotExist,
			"StateMachineVersionDoesNotExist",
			http.StatusNotFound,
		},
		// AWS: Describe/Update/DeleteStateMachineAlias each model
		// ResourceNotFound for a missing alias -- "StateMachineAliasDoesNotExist"
		// names no type anywhere in this SDK.
		{ErrStateMachineAliasDoesNotExist, "ResourceNotFound", http.StatusNotFound},
		{ErrExecutionDoesNotExist, "ExecutionDoesNotExist", http.StatusNotFound},
		{ErrActivityDoesNotExist, "ActivityDoesNotExist", http.StatusNotFound},
		// AWS: Describe/UpdateMapRun each model ResourceNotFound for a missing
		// map run -- "MapRunDoesNotExist" names no type anywhere in this SDK.
		{ErrMapRunDoesNotExist, "ResourceNotFound", http.StatusNotFound},
		{ErrTaskTokenNotFound, "TaskDoesNotExist", http.StatusNotFound},
		{ErrStateMachineAlreadyExists, "StateMachineAlreadyExists", http.StatusConflict},
		// AWS: CreateStateMachineAlias models ConflictException for a duplicate
		// alias name -- "StateMachineAliasAlreadyExists" names no type anywhere
		// in this SDK.
		{ErrStateMachineAliasAlreadyExists, "ConflictException", http.StatusConflict},
		// AWS: DeleteStateMachineVersion's own error switch models
		// ConflictException for a version still referenced by an alias.
		{ErrStateMachineVersionReferencedByAlias, "ConflictException", http.StatusConflict},
		{ErrExecutionAlreadyExists, "ExecutionAlreadyExists", http.StatusConflict},
		{ErrActivityAlreadyExists, "ActivityAlreadyExists", http.StatusConflict},
		{ErrExecutionNotRedrivable, "ExecutionNotRedrivable", http.StatusBadRequest},
		{ErrInvalidDefinition, "InvalidDefinition", http.StatusBadRequest},
		{ErrInvalidExecutionType, "InvalidExecutionType", http.StatusBadRequest},
		{ErrStateMachineTypeNotSupported, "StateMachineTypeNotSupported", http.StatusBadRequest},
		{ErrInvalidExecutionInput, "InvalidExecutionInput", http.StatusBadRequest},
		{ErrInvalidName, "InvalidName", http.StatusBadRequest},
		{ErrInvalidRoleArn, "InvalidArn", http.StatusBadRequest},
		// AWS: Create/UpdateStateMachineAlias both model ValidationException,
		// not "InvalidRoutingConfiguration" (names no type anywhere in this
		// SDK) -- AWS represents this exact condition as
		// ValidationExceptionReasonInvalidRoutingConfiguration
		// ("INVALID_ROUTING_CONFIGURATION", sfn@v1.45.4 types/enums.go:491).
		{ErrInvalidRoutingConfiguration, "ValidationException", http.StatusBadRequest},
		{ErrTagPolicyViolation, "TagPolicyViolation", http.StatusBadRequest},
		// AWS: TagResource models TooManyTags for exceeding the per-resource tag
		// limit.
		{ErrTooManyTags, "TooManyTags", http.StatusBadRequest},
		{ErrTaskTokenAlreadyExists, "TaskTokenAlreadyExists", http.StatusBadRequest},
		{ErrValidation, "ValidationException", http.StatusBadRequest},
		{errUnknownOperation, "UnknownOperationException", http.StatusBadRequest},
	}

	for _, m := range mappings {
		if errors.Is(reqErr, m.err) {
			return m.kind, m.status
		}
	}

	return "InternalServerError", http.StatusInternalServerError
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	// Close and clear all Tags objects to avoid lockmetrics leaks.
	h.tagsMu.Lock("Reset")
	for _, t := range h.tags {
		t.Close()
	}
	h.tags = make(map[string]*tags.Tags)
	h.tagsMu.Unlock()

	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Reset()
	}
}

// Shutdown implements service.Shutdowner.
// It cancels all running execution goroutines and releases associated resources.
// Destroy() calls each cancel func synchronously and returns immediately; the
// ASL goroutines exit asynchronously. If ctx expires before Destroy returns,
// Shutdown returns early so the process shutdown is not blocked.
func (h *Handler) Shutdown(ctx context.Context) {
	type destroyer interface{ Destroy() }

	b, ok := h.Backend.(destroyer)
	if !ok {
		return
	}

	done := make(chan struct{})

	go func() {
		b.Destroy()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Ensure Handler implements service.Shutdowner at compile time.
var _ service.Shutdowner = (*Handler)(nil)
