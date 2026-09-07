package codedeploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const codedeployTargetPrefix = "CodeDeploy_20141006."

// stopStatusSucceeded is the StopDeploymentOutput.status value for a
// synchronously-completed stop request. This is distinct from the
// Deployment's own status (which becomes "Stopped", see statusStopped in
// store.go): the real StopStatus enum only ever holds "Pending" or
// "Succeeded" -- it describes the outcome of the stop *operation* itself,
// never the deployment's resulting lifecycle status.
const stopStatusSucceeded = "Succeeded"

// stopStatusSucceededMessage is StopDeploymentOutput.statusMessage for a
// synchronously-completed stop request, taken verbatim from the real SDK's
// own doc comment for the Succeeded StopStatus value (api_op_StopDeployment.go).
const stopStatusSucceededMessage = "The stop operation was successful."

var errUnknownAction = errors.New("unknown action")

// Handler is the Echo HTTP handler for AWS CodeDeploy operations.
type Handler struct {
	Backend *InMemoryBackend
	// ops is a pre-built dispatch table to avoid allocating a new map on every request.
	ops map[string]service.JSONOpFunc
}

// NewHandler creates a new CodeDeploy handler with a pre-built dispatch table.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{Backend: backend}
	h.ops = h.dispatchTable()

	return h
}

// Reset clears the handler state by delegating to the backend.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Name returns the service name.
func (h *Handler) Name() string { return "CodeDeploy" }

// GetSupportedOperations returns the list of supported CodeDeploy operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CreateApplication",
		"GetApplication",
		"ListApplications",
		"DeleteApplication",
		"UpdateApplication",
		"CreateDeploymentGroup",
		"GetDeploymentGroup",
		"ListDeploymentGroups",
		"DeleteDeploymentGroup",
		"UpdateDeploymentGroup",
		"CreateDeployment",
		"GetDeployment",
		"ListDeployments",
		"StopDeployment",
		"ContinueDeployment",
		"SkipWaitTimeForInstanceTermination",
		"CreateDeploymentConfig",
		"GetDeploymentConfig",
		"ListDeploymentConfigs",
		"DeleteDeploymentConfig",
		"TagResource",
		"UntagResource",
		"ListTagsForResource",
		"AddTagsToOnPremisesInstances",
		"RemoveTagsFromOnPremisesInstances",
		"RegisterOnPremisesInstance",
		"DeregisterOnPremisesInstance",
		"GetOnPremisesInstance",
		"ListOnPremisesInstances",
		"BatchGetApplicationRevisions",
		"BatchGetApplications",
		"BatchGetDeploymentGroups",
		"BatchGetDeploymentInstances",
		"BatchGetDeploymentTargets",
		"BatchGetDeployments",
		"BatchGetOnPremisesInstances",
		"RegisterApplicationRevision",
		"GetApplicationRevision",
		"ListApplicationRevisions",
		"GetDeploymentInstance",
		"GetDeploymentTarget",
		"ListDeploymentInstances",
		"ListDeploymentTargets",
		"PutLifecycleEventHookExecutionStatus",
		"DeleteGitHubAccountToken",
		"ListGitHubAccountTokenNames",
		"DeleteResourcesByExternalId",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "codedeploy" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this CodeDeploy instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches AWS CodeDeploy requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), codedeployTargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the CodeDeploy operation from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	action := strings.TrimPrefix(target, codedeployTargetPrefix)

	if action == "" || action == target {
		return "Unknown"
	}

	return action
}

// ExtractResource extracts the application name from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, readErr := httputils.ReadBody(c.Request())
	if readErr != nil {
		return ""
	}

	var input struct {
		ApplicationName string `json:"applicationName"`
	}
	if jsonErr := json.Unmarshal(body, &input); jsonErr != nil {
		return ""
	}

	return input.ApplicationName
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"CodeDeploy", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

func (h *Handler) dispatchTable() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"CreateApplication":                    service.WrapOp(h.handleCreateApplication),
		"GetApplication":                       service.WrapOp(h.handleGetApplication),
		"ListApplications":                     service.WrapOp(h.handleListApplications),
		"DeleteApplication":                    service.WrapOp(h.handleDeleteApplication),
		"UpdateApplication":                    service.WrapOp(h.handleUpdateApplication),
		"CreateDeploymentGroup":                service.WrapOp(h.handleCreateDeploymentGroup),
		"GetDeploymentGroup":                   service.WrapOp(h.handleGetDeploymentGroup),
		"ListDeploymentGroups":                 service.WrapOp(h.handleListDeploymentGroups),
		"DeleteDeploymentGroup":                service.WrapOp(h.handleDeleteDeploymentGroup),
		"UpdateDeploymentGroup":                service.WrapOp(h.handleUpdateDeploymentGroup),
		"CreateDeployment":                     service.WrapOp(h.handleCreateDeployment),
		"GetDeployment":                        service.WrapOp(h.handleGetDeployment),
		"ListDeployments":                      service.WrapOp(h.handleListDeployments),
		"StopDeployment":                       service.WrapOp(h.handleStopDeployment),
		"ContinueDeployment":                   service.WrapOp(h.handleContinueDeployment),
		"SkipWaitTimeForInstanceTermination":   service.WrapOp(h.handleSkipWaitTimeForInstanceTermination),
		"CreateDeploymentConfig":               service.WrapOp(h.handleCreateDeploymentConfig),
		"GetDeploymentConfig":                  service.WrapOp(h.handleGetDeploymentConfig),
		"ListDeploymentConfigs":                service.WrapOp(h.handleListDeploymentConfigs),
		"DeleteDeploymentConfig":               service.WrapOp(h.handleDeleteDeploymentConfig),
		"TagResource":                          service.WrapOp(h.handleTagResource),
		"UntagResource":                        service.WrapOp(h.handleUntagResource),
		"ListTagsForResource":                  service.WrapOp(h.handleListTagsForResource),
		"AddTagsToOnPremisesInstances":         service.WrapOp(h.handleAddTagsToOnPremisesInstances),
		"RemoveTagsFromOnPremisesInstances":    service.WrapOp(h.handleRemoveTagsFromOnPremisesInstances),
		"RegisterOnPremisesInstance":           service.WrapOp(h.handleRegisterOnPremisesInstance),
		"DeregisterOnPremisesInstance":         service.WrapOp(h.handleDeregisterOnPremisesInstance),
		"GetOnPremisesInstance":                service.WrapOp(h.handleGetOnPremisesInstance),
		"ListOnPremisesInstances":              service.WrapOp(h.handleListOnPremisesInstances),
		"BatchGetApplicationRevisions":         service.WrapOp(h.handleBatchGetApplicationRevisions),
		"BatchGetApplications":                 service.WrapOp(h.handleBatchGetApplications),
		"BatchGetDeploymentGroups":             service.WrapOp(h.handleBatchGetDeploymentGroups),
		"BatchGetDeploymentInstances":          service.WrapOp(h.handleBatchGetDeploymentInstances),
		"BatchGetDeploymentTargets":            service.WrapOp(h.handleBatchGetDeploymentTargets),
		"BatchGetDeployments":                  service.WrapOp(h.handleBatchGetDeployments),
		"BatchGetOnPremisesInstances":          service.WrapOp(h.handleBatchGetOnPremisesInstances),
		"RegisterApplicationRevision":          service.WrapOp(h.handleRegisterApplicationRevision),
		"GetApplicationRevision":               service.WrapOp(h.handleGetApplicationRevision),
		"ListApplicationRevisions":             service.WrapOp(h.handleListApplicationRevisions),
		"GetDeploymentInstance":                service.WrapOp(h.handleGetDeploymentInstance),
		"GetDeploymentTarget":                  service.WrapOp(h.handleGetDeploymentTarget),
		"ListDeploymentInstances":              service.WrapOp(h.handleListDeploymentInstances),
		"ListDeploymentTargets":                service.WrapOp(h.handleListDeploymentTargets),
		"PutLifecycleEventHookExecutionStatus": service.WrapOp(h.handlePutLifecycleEventHookExecutionStatus),
		"DeleteGitHubAccountToken":             service.WrapOp(h.handleDeleteGitHubAccountToken),
		"ListGitHubAccountTokenNames":          service.WrapOp(h.handleListGitHubAccountTokenNames),
		"DeleteResourcesByExternalId":          service.WrapOp(h.handleDeleteResourcesByExternalID),
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

// errorMapping maps sentinel errors to HTTP status and exception type.
type errorMapping struct {
	sentinel error
	code     string
	status   int
}

// errorMappings is the ordered lookup table for handleError.
//
//nolint:gochecknoglobals // package-level lookup table for error mapping
var errorMappings = []errorMapping{
	{ErrNotFound, "ApplicationDoesNotExistException", http.StatusNotFound},
	{ErrDeploymentGroupNotFound, "DeploymentGroupDoesNotExistException", http.StatusNotFound},
	{ErrDeploymentNotFound, "DeploymentDoesNotExistException", http.StatusNotFound},
	{ErrDeploymentConfigNotFound, "DeploymentConfigDoesNotExistException", http.StatusNotFound},
	{ErrGitHubAccountTokenNotFound, "GitHubAccountTokenDoesNotExistException", http.StatusNotFound},
	{ErrOnPremisesInstanceNotFound, "InstanceDoesNotExistException", http.StatusNotFound},
	{ErrOnPremisesInstanceNotRegistered, "InstanceNotRegisteredException", http.StatusNotFound},
	{ErrRevisionNotFound, "RevisionDoesNotExistException", http.StatusNotFound},
	{ErrDeploymentTargetNotFound, "DeploymentTargetDoesNotExistException", http.StatusNotFound},
	{ErrAlreadyExists, "ApplicationAlreadyExistsException", http.StatusConflict},
	{ErrDeploymentGroupAlreadyExists, "DeploymentGroupAlreadyExistsException", http.StatusConflict},
	{ErrDeploymentConfigAlreadyExists, "DeploymentConfigAlreadyExistsException", http.StatusConflict},
	{ErrDeploymentAlreadyCompleted, "DeploymentAlreadyCompletedException", http.StatusConflict},
	{ErrDeploymentNotInReadyState, "DeploymentIsNotInReadyStateException", http.StatusConflict},
	{ErrDeploymentConfigIsDefault, "InvalidOperationException", http.StatusBadRequest},
	{ErrDeploymentConfigInUse, "DeploymentConfigInUseException", http.StatusConflict},
	{ErrInvalidDeploymentWaitType, "InvalidDeploymentWaitTypeException", http.StatusBadRequest},
	{ErrInvalidFileExistsBehavior, "InvalidFileExistsBehaviorException", http.StatusBadRequest},
	{ErrInvalidComputePlatform, "InvalidComputePlatformException", http.StatusBadRequest},
	{ErrInvalidEC2TagCombination, "InvalidEC2TagCombinationException", http.StatusBadRequest},
	{ErrInvalidOnPremisesTagCombination, "InvalidOnPremisesTagCombinationException", http.StatusBadRequest},
	{ErrInvalidEC2Tag, "InvalidEC2TagException", http.StatusBadRequest},
	{ErrIamArnRequired, "IamArnRequiredException", http.StatusBadRequest},
	{ErrMultipleIamArns, "MultipleIamArnsProvidedException", http.StatusBadRequest},
	{ErrInvalidTagsToAdd, "InvalidTagsToAddException", http.StatusBadRequest},
	{ErrBatchLimitExceeded, "BatchLimitExceededException", http.StatusBadRequest},
	{ErrInvalidInstanceName, "InvalidInstanceNameException", http.StatusBadRequest},
	{ErrApplicationNameRequired, "ApplicationNameRequiredException", http.StatusBadRequest},
	{ErrDeploymentGroupNameRequired, "DeploymentGroupNameRequiredException", http.StatusBadRequest},
	{ErrDeploymentIDRequired, "DeploymentIdRequiredException", http.StatusBadRequest},
	{ErrInstanceIDRequired, "InstanceIdRequiredException", http.StatusBadRequest},
	{ErrDeploymentTargetIDRequired, "DeploymentTargetIdRequiredException", http.StatusBadRequest},
	{ErrDeploymentConfigNameRequired, "DeploymentConfigNameRequiredException", http.StatusBadRequest},
	{ErrInstanceNameRequired, "InstanceNameRequiredException", http.StatusBadRequest},
	{ErrResourceArnRequired, "ResourceArnRequiredException", http.StatusBadRequest},
	{ErrGitHubTokenNameRequired, "GitHubAccountTokenNameRequiredException", http.StatusBadRequest},
	// errUnknownAction fires when the routed Action string matches no known
	// CodeDeploy operation -- a router-level condition no operation's own
	// deserializer models (there is no operation to consult), so this
	// deliberately keeps the pre-existing fallback code rather than inventing
	// one.
	{errUnknownAction, "InvalidRequestException", http.StatusBadRequest},
}

func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	makePayload := func(code, msg string) []byte {
		b, _ := json.Marshal(service.JSONErrorResponse{Type: code, Message: msg})

		return b
	}

	for _, m := range errorMappings {
		if errors.Is(err, m.sentinel) {
			return c.JSONBlob(m.status, makePayload(m.code, err.Error()))
		}
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return c.JSONBlob(http.StatusBadRequest,
			makePayload("InvalidRequestException", err.Error()))
	}

	return c.JSONBlob(http.StatusInternalServerError,
		makePayload("ServiceException", err.Error()))
}
