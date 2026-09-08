package ssm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

var ErrUnknownOperation = errors.New("UnknownOperationException")

const errCodeDoesNotExist = "DoesNotExistException"

type regionContextKey struct{}

// Handler is the Echo HTTP service handler for SSM operations.
type Handler struct {
	Backend StorageBackend
	janitor *Janitor
	ops     map[string]ssmActionFn
}

// NewHandler creates a new SSM handler with the given storage backend.
func NewHandler(backend StorageBackend) *Handler {
	h := &Handler{Backend: backend}
	h.ops = h.ssmDispatchTable()

	return h
}

// WithJanitor attaches a background janitor to the handler.
// The janitor periodically evicts expired commands. interval=0 uses the default.
func (h *Handler) WithJanitor(interval time.Duration, taskTimeout ...time.Duration) *Handler {
	if memBackend, ok := h.Backend.(*InMemoryBackend); ok {
		j := NewJanitor(memBackend, interval)
		if len(taskTimeout) > 0 {
			j.TaskTimeout = taskTimeout[0]
		}
		h.janitor = j
	}

	return h
}

// StartWorker starts the background janitor if it is configured.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.janitor != nil {
		go h.janitor.Run(ctx)
	}

	return nil
}

// Name returns the service name.
func (h *Handler) Name() string {
	return "SSM"
}

// GetSupportedOperations returns the sorted list of mocked SSM operations.
// The set is derived from the dispatch table itself (built once per Handler
// in NewHandler from the family ssm*Ops() maps) so it can never drift from
// what is actually routable.
func (h *Handler) GetSupportedOperations() []string {
	ops := slices.Collect(maps.Keys(h.ops))
	slices.Sort(ops)

	return ops
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "ssm" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this SSM instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a function that matches incoming requests for SSM.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, "AmazonSSM")
	}
}

// MatchPriority returns the routing priority for the SSM handler.
func (h *Handler) MatchPriority() int {
	return service.PriorityHeaderExact // Same header-based priority as DynamoDB
}

// ExtractOperation attempts to extract the specific SSM operation from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	const targetParts = 2
	if len(parts) == targetParts {
		return parts[1]
	}

	return "Unknown"
}

// ExtractResource attempts to extract the specific SSM resource from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	if name, exists := data["Name"]; exists {
		if nameStr, ok := name.(string); ok {
			return nameStr
		}
	}

	return ""
}

// Handler is the Echo HTTP handler for SSM operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		region := httputils.ExtractRegionFromRequest(c.Request(), config.DefaultRegion)
		ctx = context.WithValue(ctx, regionContextKey{}, region)

		return service.HandleTarget(
			c, logger.Load(ctx),
			"SSM", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			func(_ context.Context, action string, body []byte) ([]byte, error) {
				return h.dispatch(ctx, action, body)
			},
			h.handleError,
		)
	}
}

type ssmActionFn func(context.Context, []byte) (any, error)

// jsonOp adapts a backend method with the common
// func(context.Context, *Input) (*Output, error) shape into an ssmActionFn
// that JSON-decodes the request body into a fresh Input. Nearly every SSM
// operation follows this shape, so every ssm*Ops() dispatch table is built
// from calls to this one helper instead of repeating the decode-and-call
// boilerplate per operation.
func jsonOp[I, O any](fn func(context.Context, *I) (O, error)) ssmActionFn {
	return func(ctx context.Context, b []byte) (any, error) {
		var input I
		if err := json.Unmarshal(b, &input); err != nil {
			return nil, err
		}

		return fn(ctx, &input)
	}
}

func (h *Handler) ssmDispatchTable() map[string]ssmActionFn {
	ops := h.ssmParameterOps()
	maps.Copy(ops, h.ssmTagOps())
	maps.Copy(ops, h.ssmDocumentOps())
	maps.Copy(ops, h.ssmCommandOps())
	maps.Copy(ops, h.ssmCloudConnectorOps())
	maps.Copy(ops, h.ssmAssociationOps())
	maps.Copy(ops, h.ssmMaintenanceWindowOps())
	maps.Copy(ops, h.ssmPatchBaselineOps())
	maps.Copy(ops, h.ssmOpsItemOps())
	maps.Copy(ops, h.ssmInventoryOps())
	maps.Copy(ops, h.ssmSessionOps())
	maps.Copy(ops, h.ssmActivationOps())
	maps.Copy(ops, h.ssmInstanceOps())
	maps.Copy(ops, h.ssmAutomationOps())
	maps.Copy(ops, h.ssmServiceSettingOps())
	maps.Copy(ops, h.ssmResourcePolicyOps())

	return ops
}

func (h *Handler) ssmParameterOps() map[string]ssmActionFn {
	return map[string]ssmActionFn{
		"PutParameter":            jsonOp(h.Backend.PutParameter),
		"GetParameter":            jsonOp(h.Backend.GetParameter),
		"GetParameters":           jsonOp(h.Backend.GetParameters),
		"GetParameterHistory":     jsonOp(h.Backend.GetParameterHistory),
		"DeleteParameter":         jsonOp(h.Backend.DeleteParameter),
		"DeleteParameters":        jsonOp(h.Backend.DeleteParameters),
		"GetParametersByPath":     jsonOp(h.Backend.GetParametersByPath),
		"DescribeParameters":      jsonOp(h.Backend.DescribeParameters),
		"LabelParameterVersion":   jsonOp(h.Backend.LabelParameterVersion),
		"UnlabelParameterVersion": jsonOp(h.Backend.UnlabelParameterVersion),
	}
}

func (h *Handler) ssmTagOps() map[string]ssmActionFn {
	return map[string]ssmActionFn{
		"AddTagsToResource": func(ctx context.Context, b []byte) (any, error) {
			var input AddTagsToResourceInput
			if err := json.Unmarshal(b, &input); err != nil {
				return nil, err
			}

			return struct{}{}, h.Backend.AddTagsToResource(ctx, &input)
		},
		"RemoveTagsFromResource": func(ctx context.Context, b []byte) (any, error) {
			var input RemoveTagsFromResourceInput
			if err := json.Unmarshal(b, &input); err != nil {
				return nil, err
			}

			return struct{}{}, h.Backend.RemoveTagsFromResource(ctx, &input)
		},
		"ListTagsForResource": jsonOp(h.Backend.ListTagsForResource),
	}
}

func (h *Handler) ssmDocumentOps() map[string]ssmActionFn {
	return map[string]ssmActionFn{
		"CreateDocument":               jsonOp(h.Backend.CreateDocument),
		"GetDocument":                  jsonOp(h.Backend.GetDocument),
		"DescribeDocument":             jsonOp(h.Backend.DescribeDocument),
		"ListDocuments":                jsonOp(h.Backend.ListDocuments),
		"UpdateDocument":               jsonOp(h.Backend.UpdateDocument),
		"DeleteDocument":               jsonOp(h.Backend.DeleteDocument),
		"DescribeDocumentPermission":   jsonOp(h.Backend.DescribeDocumentPermission),
		"ModifyDocumentPermission":     jsonOp(h.Backend.ModifyDocumentPermission),
		"ListDocumentVersions":         jsonOp(h.Backend.ListDocumentVersions),
		"ListDocumentMetadataHistory":  jsonOp(h.Backend.ListDocumentMetadataHistory),
		"UpdateDocumentDefaultVersion": jsonOp(h.Backend.UpdateDocumentDefaultVersion),
		"UpdateDocumentMetadata":       jsonOp(h.Backend.UpdateDocumentMetadata),
	}
}

func (h *Handler) ssmCommandOps() map[string]ssmActionFn {
	return map[string]ssmActionFn{
		"CancelCommand":          jsonOp(h.Backend.CancelCommand),
		"SendCommand":            jsonOp(h.Backend.SendCommand),
		"ListCommands":           jsonOp(h.Backend.ListCommands),
		"GetCommandInvocation":   jsonOp(h.Backend.GetCommandInvocation),
		"ListCommandInvocations": jsonOp(h.Backend.ListCommandInvocations),
	}
}

func (h *Handler) ssmCloudConnectorOps() map[string]ssmActionFn {
	return map[string]ssmActionFn{
		"CreateCloudConnector":   jsonOp(h.Backend.CreateCloudConnector),
		"DeleteCloudConnector":   jsonOp(h.Backend.DeleteCloudConnector),
		"GetCloudConnector":      jsonOp(h.Backend.GetCloudConnector),
		"ListCloudConnectors":    jsonOp(h.Backend.ListCloudConnectors),
		"UpdateCloudConnector":   jsonOp(h.Backend.UpdateCloudConnector),
		"ValidateCloudConnector": jsonOp(h.Backend.ValidateCloudConnector),
	}
}

// dispatch routes the operation to the appropriate handler.
func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w:%s", ErrUnknownOperation, action)
	}

	response, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(response)
}

// classifySSMError maps a backend error to an HTTP status code and error type string.
func classifySSMError(reqErr error) (string, int) {
	statusCode := http.StatusBadRequest

	switch {
	case errors.Is(reqErr, ErrParameterVersionNotFound):
		return "ParameterVersionNotFound", statusCode
	case errors.Is(reqErr, ErrParameterNotFound):
		return "ParameterNotFound", statusCode
	case errors.Is(reqErr, ErrParameterAlreadyExists):
		return "ParameterAlreadyExists", statusCode
	case errors.Is(reqErr, ErrDocumentAlreadyExists):
		return "DocumentAlreadyExists", statusCode
	case errors.Is(reqErr, ErrDocumentNotFound):
		return "InvalidDocument", statusCode
	case errors.Is(reqErr, ErrInvalidDocumentVersion):
		return "InvalidDocumentVersion", statusCode
	case errors.Is(reqErr, ErrCommandNotFound):
		return "InvalidCommandId", statusCode
	case errors.Is(reqErr, ErrValidationException):
		return "ValidationException", statusCode
	case errors.Is(reqErr, ErrHierarchyLevelLimitExceeded):
		return "HierarchyLevelLimitExceededException", statusCode
	case errors.Is(reqErr, ErrParameterMaxVersionLimitExceeded):
		return "ParameterMaxVersionLimitExceeded", statusCode
	}

	return classifySSMErrorExtended(reqErr)
}

// classifySSMResourceDataSyncError handles the two ResourceDataSync-specific
// errors, split out of classifySSMErrorExtended purely to keep that
// function's cyclomatic complexity under this repo's cyclop limit.
func classifySSMResourceDataSyncError(reqErr error) (string, int, bool) {
	statusCode := http.StatusBadRequest

	switch {
	case errors.Is(reqErr, ErrResourceDataSyncNotFound):
		return "ResourceDataSyncNotFoundException", statusCode, true
	case errors.Is(reqErr, ErrResourceDataSyncExists):
		return "ResourceDataSyncAlreadyExistsException", statusCode, true
	default:
		return "", 0, false
	}
}

// classifySSMResourcePolicyError handles the two ResourcePolicy-specific
// errors, split out for the same cyclop-budget reason as
// classifySSMResourceDataSyncError.
func classifySSMResourcePolicyError(reqErr error) (string, int, bool) {
	statusCode := http.StatusBadRequest

	switch {
	case errors.Is(reqErr, ErrResourcePolicyNotFound):
		return "ResourcePolicyNotFoundException", statusCode, true
	case errors.Is(reqErr, ErrResourcePolicyConflict):
		return "ResourcePolicyConflictException", statusCode, true
	default:
		return "", 0, false
	}
}

// classifySSMDocumentError handles the DeleteDocument-still-shared error,
// split out for the same cyclop-budget reason as classifySSMResourceDataSyncError.
func classifySSMDocumentError(reqErr error) (string, int, bool) {
	if errors.Is(reqErr, ErrDocumentStillShared) {
		return "InvalidDocumentOperation", http.StatusBadRequest, true
	}

	return "", 0, false
}

// classifySSMOpsError handles the OpsItem/OpsMetadata-specific errors, split
// out for the same cyclop-budget reason as classifySSMResourceDataSyncError.
func classifySSMOpsError(reqErr error) (string, int, bool) {
	statusCode := http.StatusBadRequest

	switch {
	case errors.Is(reqErr, ErrOpsItemNotFound):
		return "OpsItemNotFoundException", statusCode, true
	case errors.Is(reqErr, ErrOpsMetadataNotFound):
		return "OpsMetadataNotFoundException", statusCode, true
	case errors.Is(reqErr, ErrOpsMetadataAlreadyExists):
		return "OpsMetadataAlreadyExistsException", statusCode, true
	default:
		return "", 0, false
	}
}

// classifySSMMiscNotFoundError handles three unrelated not-found errors that
// all map to errCodeDoesNotExist, split out for the same cyclop-budget
// reason as classifySSMResourceDataSyncError.
func classifySSMMiscNotFoundError(reqErr error) (string, int, bool) {
	statusCode := http.StatusBadRequest

	switch {
	case errors.Is(reqErr, ErrMaintenanceWindowExecutionNotFound):
		return errCodeDoesNotExist, statusCode, true
	case errors.Is(reqErr, ErrMaintenanceWindowNotFound):
		return errCodeDoesNotExist, statusCode, true
	case errors.Is(reqErr, ErrPatchBaselineNotFound):
		return errCodeDoesNotExist, statusCode, true
	default:
		return "", 0, false
	}
}

// classifySSMResourceIdentityError handles the three malformed/unknown
// resource-identifier errors, split out for the same cyclop-budget reason as
// classifySSMResourceDataSyncError.
func classifySSMResourceIdentityError(reqErr error) (string, int, bool) {
	statusCode := http.StatusBadRequest

	switch {
	case errors.Is(reqErr, ErrInvalidKeyID):
		return "InvalidKeyId", statusCode, true
	case errors.Is(reqErr, ErrInvalidActivationID):
		return "InvalidActivationId", statusCode, true
	case errors.Is(reqErr, ErrInvalidResourceID):
		return "InvalidResourceId", statusCode, true
	default:
		return "", 0, false
	}
}

// classifySSMParameterValidationError handles PutParameter's own declared
// input-validation exceptions, split out for the same cyclop-budget reason as
// classifySSMResourceDataSyncError.
func classifySSMParameterValidationError(reqErr error) (string, int, bool) {
	statusCode := http.StatusBadRequest

	switch {
	case errors.Is(reqErr, ErrParameterNamePattern):
		return "ParameterPatternMismatchException", statusCode, true
	case errors.Is(reqErr, ErrUnsupportedParameterType):
		return "UnsupportedParameterType", statusCode, true
	case errors.Is(reqErr, ErrInvalidAllowedPattern):
		return "InvalidAllowedPatternException", statusCode, true
	default:
		return "", 0, false
	}
}

// ssmErrorClassifier reports (code, status, true) when reqErr matches the
// group of sentinels it handles, or ("", 0, false) otherwise.
type ssmErrorClassifier func(error) (string, int, bool)

func classifySSMErrorExtended(reqErr error) (string, int) {
	statusCode := http.StatusBadRequest

	// A slice walked in a loop, rather than one `if` per classifier, keeps
	// this function's cyclomatic complexity under this repo's cyclop limit
	// as new sentinel groups are added.
	classifiers := []ssmErrorClassifier{
		classifySSMResourceDataSyncError,
		classifySSMParameterValidationError,
		classifySSMResourcePolicyError,
		classifySSMDocumentError,
		classifySSMOpsError,
		classifySSMMiscNotFoundError,
		classifySSMResourceIdentityError,
	}

	for _, classify := range classifiers {
		if code, status, ok := classify(reqErr); ok {
			return code, status
		}
	}

	switch {
	case errors.Is(reqErr, ErrInvalidAggregator):
		return "InvalidAggregatorException", statusCode
	case errors.Is(reqErr, ErrPatchBaselineInUse):
		return "ResourceInUseException", statusCode
	case errors.Is(reqErr, ErrCloudConnectorNotFound):
		return "ResourceNotFoundException", statusCode
	case errors.Is(reqErr, ErrAccessRequestNotFound):
		return "ResourceNotFoundException", statusCode
	case errors.Is(reqErr, ErrAssociationNotFound):
		return "AssociationDoesNotExist", statusCode
	case errors.Is(reqErr, ErrAutomationExecutionNotFound):
		return "AutomationExecutionNotFoundException", statusCode
	case errors.Is(reqErr, ErrUnknownOperation):
		return "UnknownOperationException", statusCode
	default:
		return "InternalServerError", http.StatusInternalServerError
	}
}

// handleError writes a standardized error response back to the client.
func (h *Handler) handleError(ctx context.Context, c *echo.Context, action string, reqErr error) error {
	log := logger.Load(ctx)
	c.Response().Header().Set("Content-Type", "application/x-amz-json-1.1")

	errorType, statusCode := classifySSMError(reqErr)

	if errorType == "InternalServerError" {
		log.ErrorContext(ctx, "SSM internal error", "error", reqErr, "action", action)
	} else {
		log.WarnContext(ctx, "SSM request error", "error", reqErr, "action", action)
	}

	errResp := service.JSONErrorResponse{
		Type:    errorType,
		Message: reqErr.Error(),
	}

	payload, _ := json.Marshal(errResp)

	return c.JSONBlob(statusCode, payload)
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Reset()
	}
}
