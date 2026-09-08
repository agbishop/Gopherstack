package secretsmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

// ErrUnknownOperation is returned when an unsupported operation is requested.
var ErrUnknownOperation = errors.New("UnknownOperationException")

// LambdaInvoker can invoke a Lambda function synchronously.
type LambdaInvoker interface {
	InvokeFunction(ctx context.Context, name, invocationType string, payload []byte) ([]byte, int, error)
}

// Handler is the Echo HTTP handler for Secrets Manager operations.
type Handler struct {
	Backend       StorageBackend
	lambdaInvoker LambdaInvoker
	ops           map[string]smActionFn
	janitor       *Janitor
	DefaultRegion string
	janitorRun    worker.SingleRun
}

// NewHandler creates a new Secrets Manager handler.
func NewHandler(backend StorageBackend) *Handler {
	h := &Handler{
		Backend: backend,
	}
	h.buildOps()

	return h
}

// WithJanitor attaches a background janitor to the handler.
func (h *Handler) WithJanitor(interval ...time.Duration) *Handler {
	if memBackend, ok := h.Backend.(*InMemoryBackend); ok {
		var d time.Duration
		if len(interval) > 0 {
			d = interval[0]
		}
		h.janitor = NewJanitor(memBackend, d)
	}

	return h
}

// StartWorker starts the background janitor if it is configured.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.janitor == nil {
		return nil
	}

	h.janitorRun.Start(ctx, h.janitor)

	return nil
}

// Shutdown stops the janitor worker and the rotation scheduler, waiting for the
// janitor to exit.
func (h *Handler) Shutdown(ctx context.Context) {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.StopRotationScheduler()
	}

	h.janitorRun.Stop(ctx)
}

var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
)

// buildOps builds and caches the operation dispatch table.
func (h *Handler) buildOps() {
	table := make(map[string]smActionFn)
	maps.Copy(table, h.smSecretsActions())
	maps.Copy(table, h.smSecretVersionsActions())
	maps.Copy(table, h.smRotationActions())
	maps.Copy(table, h.smResourcePolicyActions())
	maps.Copy(table, h.smReplicationActions())
	maps.Copy(table, h.smTagActions())
	maps.Copy(table, h.smRandomPasswordActions())
	h.ops = table
}

// SetLambdaInvoker sets the Lambda invoker used for RotateSecret. It is stored
// on both the handler (for HTTP-triggered rotations) and the backend (for
// scheduled rotations triggered by the rotation scheduler goroutine).
func (h *Handler) SetLambdaInvoker(invoker LambdaInvoker) {
	h.lambdaInvoker = invoker

	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.SetLambdaInvoker(invoker)
	}
}

// Name returns the service name.
func (h *Handler) Name() string {
	return "SecretsManager"
}

// GetSupportedOperations returns the list of supported Secrets Manager operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"BatchGetSecretValue",
		"CancelRotateSecret",
		"CreateSecret",
		"DeleteResourcePolicy",
		"DeleteSecret",
		opDescribeSecret,
		"GetRandomPassword",
		opGetResourcePolicy,
		"GetSecretValue",
		"ListSecretVersionIds",
		opListSecrets,
		"PutResourcePolicy",
		"PutSecretValue",
		"RemoveRegionsFromReplication",
		"ReplicateSecretToRegions",
		"RestoreSecret",
		"RotateSecret",
		"StopReplicationToReplica",
		"TagResource",
		"UntagResource",
		"UpdateSecret",
		"UpdateSecretVersionStage",
		opValidateResourcePolicy,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "secretsmanager" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this Secrets Manager instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.DefaultRegion} }

// RouteMatcher returns a function that matches Secrets Manager requests by X-Amz-Target header.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, "secretsmanager")
	}
}

// MatchPriority returns the routing priority for the Secrets Manager handler.
func (h *Handler) MatchPriority() int {
	return service.PriorityHeaderPartial
}

// ExtractOperation extracts the Secrets Manager operation name from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")

	const targetParts = 2
	if len(parts) == targetParts {
		return parts[1]
	}

	return "Unknown"
}

// ExtractResource returns the secret ID from the request body when present.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	if secretID, ok := data["SecretId"].(string); ok {
		return secretID
	}

	if name, ok := data["Name"].(string); ok {
		return name
	}

	return ""
}

// Handler returns the Echo handler function for Secrets Manager operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"SecretsManager", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			func(ctx context.Context, action string, body []byte) ([]byte, error) {
				return h.dispatch(ctx, c.Request(), action, body)
			},
			h.handleError,
		)
	}
}

type smActionFn func(ctx context.Context, region string, body []byte) (any, error)

// decodeAction builds an smActionFn that unmarshals the request body into a
// fresh *I and forwards it to fn. It centralizes the JSON-decode boilerplate
// shared by every region-agnostic operation.
func decodeAction[I any](fn func(ctx context.Context, input *I) (any, error)) smActionFn {
	return func(ctx context.Context, _ string, body []byte) (any, error) {
		var input I
		if err := json.Unmarshal(body, &input); err != nil {
			return nil, err
		}

		return fn(ctx, &input)
	}
}

// dispatch routes the operation to the appropriate backend method.
func (h *Handler) dispatch(ctx context.Context, r *http.Request, action string, body []byte) ([]byte, error) {
	region := httputils.ExtractRegionFromRequest(r, h.DefaultRegion)
	// Attach the resolved region to the context so backend operations are region-scoped.
	ctx = context.WithValue(ctx, regionContextKey{}, region)

	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownOperation, action)
	}

	response, err := fn(ctx, region, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(response)
}

// handleError writes a structured error response for a Secrets Manager operation failure.
func (h *Handler) handleError(ctx context.Context, c *echo.Context, action string, reqErr error) error {
	log := logger.Load(ctx)
	c.Response().Header().Set("Content-Type", "application/x-amz-json-1.1")

	var errorType string

	statusCode := http.StatusBadRequest

	switch {
	case errors.Is(reqErr, ErrSecretNotFound), errors.Is(reqErr, ErrVersionNotFound):
		errorType = errResourceNotFoundException
	case errors.Is(reqErr, ErrMalformedPolicyDocument):
		errorType = "MalformedPolicyDocumentException"
	case errors.Is(reqErr, ErrPublicPolicyException):
		errorType = "PublicPolicyException"
	case errors.Is(reqErr, ErrSecretAlreadyExists):
		errorType = "ResourceExistsException"
	case errors.Is(reqErr, ErrSecretDeleted), errors.Is(reqErr, ErrRotationStrategyRequired),
		errors.Is(reqErr, ErrReplicaAlreadyExists), errors.Is(reqErr, ErrReplicaNotWritable):
		errorType = "InvalidRequestException"
	case errors.Is(reqErr, ErrSecretValueTooLarge),
		errors.Is(reqErr, ErrInvalidPasswordParameters),
		errors.Is(reqErr, ErrInvalidParameter),
		errors.Is(reqErr, ErrInvalidSecretName):
		errorType = "InvalidParameterException"
	case errors.Is(reqErr, ErrUnknownOperation):
		errorType = "UnknownOperationException"
	default:
		errorType = "InternalServiceError"
		statusCode = http.StatusInternalServerError
	}

	if statusCode == http.StatusInternalServerError {
		log.ErrorContext(ctx, "SecretsManager internal error", "error", reqErr, "action", action)
	} else {
		log.WarnContext(ctx, "SecretsManager request error", "error", reqErr, "action", action)
	}

	// Real AWS Secrets Manager (awsJson1_1 protocol) returns the error shape in
	// the body AND echoes the error code in the X-Amzn-Errortype response header.
	// AWS SDKs and the CLI read this header to construct the typed exception, so
	// emitting it is required for faithful client-side error handling.
	c.Response().Header().Set("X-Amzn-Errortype", errorType)

	payload, _ := json.Marshal(ErrorResponse{
		Type:    errorType,
		Message: reqErr.Error(),
	})

	return c.JSONBlob(statusCode, payload)
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Reset()
	}
}
