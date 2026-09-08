package bedrockruntime

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	modelPathPrefix           = "/model/"
	guardrailPathPrefix       = "/guardrail/"
	asyncInvokePathPrefix     = "/async-invoke"
	asyncInvokeItemPathPrefix = asyncInvokePathPrefix + "/"
)

// guardrailChecksPath is declared in handler_guardrail_checks.go
// ("/guardrail-checks/invoke", confirmed from serializers.go). It is a
// distinct fixed endpoint, NOT under guardrailPathPrefix ("/guardrail/") --
// InvokeGuardrailChecks takes its check configuration inline in the request
// body rather than referencing a stored guardrailIdentifier.

const (
	opConverse                      = "Converse"
	opConverseStream                = "ConverseStream"
	opCountTokens                   = "CountTokens"
	opInvokeGuardrailChecks         = "InvokeGuardrailChecks"
	opInvokeModel                   = "InvokeModel"
	opInvokeModelWithBidiStream     = "InvokeModelWithBidirectionalStream"
	opInvokeModelWithResponseStream = "InvokeModelWithResponseStream"

	hdrMessageType = ":message-type"
	hdrEventType   = ":event-type"
	hdrContentType = ":content-type"

	keyInputTokens   = "inputTokens"
	keyUsage         = "usage"
	keyInvocationArn = "invocationArn"
	keyText          = "text"
	keyMessage       = "message"

	mockResponseText  = "This is a mock response from Gopherstack."
	stopReasonEndTurn = "end_turn"

	hdrMessageTypeEvent = "event"
	keyRole             = "role"
	keyStopReason       = "stop_reason"

	roleAssistant     = "assistant"
	convStopReasonKey = "stopReason"
	convOutputTokens  = "outputTokens"
	convTotalTokens   = "totalTokens"
	convContentIdx    = "contentBlockIndex"

	keyContent = "content"
	keyModel   = "model"
)

// Mock response token counts used in model responses.
const (
	mockInputTokenCount  = 10
	mockOutputTokenCount = 10
	mockTotalTokenCount  = 20
	mockLatencyMS        = 1

	//nolint:gosec // header names are not credentials
	hdrBedrockInputTokenCount = "X-Amzn-Bedrock-Input-Token-Count"
	//nolint:gosec // header names are not credentials
	hdrBedrockOutputTokenCount = "X-Amzn-Bedrock-Output-Token-Count"
)

// maxInvocationStringBytes caps the stored request/response string length to prevent unbounded growth.
const maxInvocationStringBytes = 10_000

// Handler is the Echo HTTP handler for AWS Bedrock Runtime operations.
type Handler struct {
	Backend       *InMemoryBackend
	janitorCancel context.CancelFunc
	janitorDone   chan struct{}
}

// NewHandler creates a new Bedrock Runtime handler backed by backend.
// backend must not be nil.
func NewHandler(backend *InMemoryBackend) *Handler {
	return &Handler{Backend: backend}
}

// StartWorker starts the background janitor for async invocations.
func (h *Handler) StartWorker(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	h.janitorCancel = cancel
	h.janitorDone = done

	go func() {
		defer close(done)
		// Interval matches defaultAsyncInvokeCompletionDelay (janitor.go) so
		// InProgress invocations advance to Completed close to that delay,
		// not up to a full tick late -- the sibling services/bedrock janitor
		// follows the same interval-matches-completion-delay pattern.
		h.Backend.RunJanitor(runCtx, defaultAsyncInvokeCompletionDelay)
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
func (h *Handler) Name() string { return "BedrockRuntime" }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"ApplyGuardrail",
		opConverse,
		opConverseStream,
		opCountTokens,
		"GetAsyncInvoke",
		opInvokeGuardrailChecks,
		opInvokeModel,
		opInvokeModelWithBidiStream,
		opInvokeModelWithResponseStream,
		"ListAsyncInvokes",
		"StartAsyncInvoke",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule
// matching. This must be "bedrock", not "bedrockruntime": real Bedrock
// Runtime signs every request with SigV4 service name "bedrock" (verified in
// aws-sdk-go-v2/service/bedrockruntime@v1.57.1's auth.go, serviceAuthOptions
// -- unconditional for every operation, no per-operation override), the same
// signing name the sibling services/bedrock (control plane) handler already
// declares. pkgs/chaos's Middleware extracts the fault-matching "service"
// string straight from the real Authorization header's SigV4 credential
// scope, so a ChaosServiceName that doesn't match the real signing name can
// never match real client traffic -- getTargets already merges entries that
// share one signing name across multiple handlers (its own doc comment cites
// S3/S3 Control as the existing precedent for this exact situation).
func (h *Handler) ChaosServiceName() string { return "bedrock" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches Bedrock Runtime requests.
// It matches paths for /model/, /guardrail/, and /async-invoke.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		if strings.HasPrefix(c.Request().Host, "bedrock-runtime.") {
			return true
		}

		return strings.HasPrefix(path, modelPathPrefix) ||
			strings.HasPrefix(path, guardrailPathPrefix) ||
			path == guardrailChecksPath ||
			path == asyncInvokePathPrefix ||
			strings.HasPrefix(path, asyncInvokeItemPathPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityPathVersioned }

// ExtractOperation returns the operation name from the request path.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return pathToOperation(c.Request().URL.Path, c.Request().Method)
}

// ExtractResource extracts the primary resource identifier from the request path.
// For metrics/logging purposes, returns stable low-cardinality values:
// model paths return the modelId, guardrail paths return the guardrailIdentifier,
// and /async-invoke item paths return "async-invoke" (stable, not the ARN).
func (h *Handler) ExtractResource(c *echo.Context) string {
	path := c.Request().URL.Path

	switch {
	case strings.HasPrefix(path, modelPathPrefix):
		op := pathToOperation(path, c.Request().Method)

		return extractModelID(path, op)
	case strings.HasPrefix(path, guardrailPathPrefix):
		rest, _ := strings.CutPrefix(path, guardrailPathPrefix)
		guardrailID, _, _ := strings.Cut(rest, "/")

		return guardrailID
	case strings.HasPrefix(path, asyncInvokeItemPathPrefix):
		// Return stable label; the full ARN would be unique per invocation.
		return "async-invoke"
	default:
		return ""
	}
}

// Reset clears all backend state. Implements service.Resettable.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Handler returns the Echo handler function for Bedrock Runtime requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		path := r.URL.Path
		method := r.Method
		log := logger.Load(r.Context())

		body, err := httputils.ReadBody(r)
		if err != nil {
			log.ErrorContext(r.Context(), "bedrockruntime: failed to read request body", "error", err)

			return c.JSON(
				http.StatusInternalServerError,
				errorResponse("InternalServerException", "internal server error"),
			)
		}

		switch {
		case strings.HasPrefix(path, modelPathPrefix):
			return h.handleModelPath(c, method, path, body)
		case strings.HasPrefix(path, guardrailPathPrefix):
			return h.handleGuardrailPath(c, method, path, body)
		case path == guardrailChecksPath:
			if method != http.MethodPost {
				return c.JSON(http.StatusMethodNotAllowed, errorResponse("ValidationException", "method not allowed"))
			}

			return h.handleInvokeGuardrailChecks(c, body)
		case path == asyncInvokePathPrefix || strings.HasPrefix(path, asyncInvokeItemPathPrefix):
			return h.handleAsyncInvokePath(c, method, path, body)
		default:
			return c.JSON(http.StatusNotFound, errorResponse("UnknownOperationException", "unknown operation: "+path))
		}
	}
}

// handleModelPath dispatches requests for /model/{modelId}/... paths.
func (h *Handler) handleModelPath(c *echo.Context, method, path string, body []byte) error {
	if method != http.MethodPost {
		return c.JSON(http.StatusMethodNotAllowed, errorResponse("ValidationException", "method not allowed"))
	}

	op := pathToOperation(path, method)

	modelID := extractModelID(path, op)
	if modelID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "missing modelId in path"))
	}

	switch op {
	case opInvokeModel:
		return h.handleInvokeModel(c, modelID, body)
	case opInvokeModelWithResponseStream:
		return h.handleInvokeModelWithResponseStream(c, modelID, body)
	case opConverse:
		return h.handleConverse(c, modelID, body)
	case opConverseStream:
		return h.handleConverseStream(c, modelID, body)
	case opCountTokens:
		return h.handleCountTokens(c, modelID, body)
	case opInvokeModelWithBidiStream:
		return h.handleInvokeModelWithBidirectionalStream(c, modelID, body)
	default:
		return c.JSON(http.StatusNotFound, errorResponse("UnknownOperationException", "unknown operation: "+path))
	}
}

// handleGuardrailPath dispatches requests for /guardrail/... paths.
func (h *Handler) handleGuardrailPath(c *echo.Context, method, path string, body []byte) error {
	if method != http.MethodPost {
		return c.JSON(http.StatusMethodNotAllowed, errorResponse("ValidationException", "method not allowed"))
	}

	if strings.HasSuffix(path, "/apply") {
		return h.handleApplyGuardrail(c, path, body)
	}

	return c.JSON(http.StatusNotFound, errorResponse("UnknownOperationException", "unknown operation: "+path))
}

// handleAsyncInvokePath dispatches requests for /async-invoke[/{arn}] paths.
func (h *Handler) handleAsyncInvokePath(c *echo.Context, method, path string, body []byte) error {
	switch {
	case path == asyncInvokePathPrefix && method == http.MethodGet:
		return h.handleListAsyncInvokes(c)
	case path == asyncInvokePathPrefix && method == http.MethodPost:
		return h.handleStartAsyncInvoke(c, body)
	case strings.HasPrefix(path, asyncInvokeItemPathPrefix) && method == http.MethodGet:
		return h.handleGetAsyncInvoke(c, path)
	default:
		return c.JSON(http.StatusMethodNotAllowed, errorResponse("ValidationException", "method not allowed"))
	}
}

// truncateString limits a string to maxInvocationStringBytes bytes to cap memory usage.
func truncateString(s string) string {
	if len(s) <= maxInvocationStringBytes {
		return s
	}

	return s[:maxInvocationStringBytes]
}

// --- Helpers ---

// modelPathSuffixForOp returns the fixed literal path suffix for a model-path
// operation (and whether op is a known one). Used by extractModelID to
// correctly bound the modelId segment when the modelId itself is an ARN
// containing an embedded '/' (e.g. an inference-profile or custom-model ARN
// such as "arn:aws:bedrock:us-east-1:111122223333:inference-profile/us.anthropic.claude-3-sonnet-20240229-v1:0").
// The AWS SDK percent-encodes that embedded '/' on the wire (modelId is a
// non-greedy `{modelId}` label), but net/http decodes it back to a literal
// '/' in URL.Path -- so naively cutting at the FIRST '/' after "/model/"
// truncates the modelId and silently drops the model-family suffix,
// producing the wrong mock response envelope (see mockInvokeModelResponse's
// family matching).
func modelPathSuffixForOp(op string) (string, bool) {
	switch op {
	case opInvokeModel:
		return "/invoke", true
	case opInvokeModelWithResponseStream:
		return "/invoke-with-response-stream", true
	case opInvokeModelWithBidiStream:
		return "/invoke-with-bidirectional-stream", true
	case opConverse:
		return "/converse", true
	case opConverseStream:
		return "/converse-stream", true
	case opCountTokens:
		return "/count-tokens", true
	default:
		return "", false
	}
}

// extractModelID extracts the modelId path parameter from a decoded
// /model/{modelId}/{suffix} path. op is the already-resolved operation (see
// pathToOperation), used to look up the fixed literal suffix to trim from
// the tail -- this correctly bounds ARN-style modelIds that themselves
// contain a '/'. If op has no known suffix, falls back to cutting at the
// first '/' (best effort for unrecognized/unsupported operations).
func extractModelID(path, op string) string {
	rest, ok := strings.CutPrefix(path, modelPathPrefix)
	if !ok {
		return ""
	}

	if suffix, known := modelPathSuffixForOp(op); known {
		return strings.TrimSuffix(rest, suffix)
	}

	modelID, _, _ := strings.Cut(rest, "/")

	return modelID
}

func pathToOperation(path, method string) string {
	if op := modelPathOperation(path); op != "" {
		return op
	}

	if op := asyncOrGuardrailOperation(path, method); op != "" {
		return op
	}

	return "Unknown"
}

// modelPathOperation maps /model/{modelId}/... paths to operation names.
// Gated on modelPathPrefix before the suffix switch below: without it,
// InvokeGuardrailChecks's real wire path "/guardrail-checks/invoke" (POST)
// also ends in "/invoke" and was misclassified as InvokeModel by
// ExtractOperation, even though Handler() itself dispatched it correctly
// (Handler()'s own switch checks path == guardrailChecksPath, not a bare
// suffix). Runtime dispatch was never wrong; only the observability label
// was.
func modelPathOperation(path string) string {
	if !strings.HasPrefix(path, modelPathPrefix) {
		return ""
	}

	switch {
	case strings.HasSuffix(path, "/invoke-with-response-stream"):
		return opInvokeModelWithResponseStream
	case strings.HasSuffix(path, "/invoke-with-bidirectional-stream"):
		return opInvokeModelWithBidiStream
	case strings.HasSuffix(path, "/invoke"):
		return opInvokeModel
	case strings.HasSuffix(path, "/converse-stream"):
		return opConverseStream
	case strings.HasSuffix(path, "/converse"):
		return opConverse
	case strings.HasSuffix(path, "/count-tokens"):
		return opCountTokens
	default:
		return ""
	}
}

// asyncOrGuardrailOperation maps /guardrail/... and /async-invoke paths to operation names.
func asyncOrGuardrailOperation(path, method string) string {
	switch {
	case strings.HasPrefix(path, guardrailPathPrefix) && strings.HasSuffix(path, "/apply"):
		return "ApplyGuardrail"
	case path == guardrailChecksPath && method == http.MethodPost:
		return opInvokeGuardrailChecks
	case path == asyncInvokePathPrefix && method == http.MethodGet:
		return "ListAsyncInvokes"
	case path == asyncInvokePathPrefix && method == http.MethodPost:
		return "StartAsyncInvoke"
	case strings.HasPrefix(path, asyncInvokeItemPathPrefix) && method == http.MethodGet:
		return "GetAsyncInvoke"
	default:
		return ""
	}
}

// extractGuardrailIDAndVersion extracts the guardrailIdentifier and guardrailVersion
// from a path of the form /guardrail/{id}/version/{ver}/apply.
func extractGuardrailIDAndVersion(path string) (string, string) {
	rest, ok := strings.CutPrefix(path, guardrailPathPrefix)
	if !ok {
		return "", ""
	}

	// rest = "{id}/version/{ver}/apply"
	guardrailID, rest, _ := strings.Cut(rest, "/version/")
	guardrailVersion, _, _ := strings.Cut(rest, "/")

	return guardrailID, guardrailVersion
}

func errorResponse(code, msg string) map[string]string {
	return map[string]string{"__type": code, keyMessage: msg}
}

// handleError writes a standardized error response, mapping sentinel errors to HTTP status codes.
func handleError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrValidation):
		return c.JSON(http.StatusBadRequest, errorResponse("ValidationException", err.Error()))
	case errors.Is(err, awserr.ErrNotFound):
		return c.JSON(http.StatusNotFound, errorResponse("ResourceNotFoundException", err.Error()))
	default:
		return c.JSON(http.StatusInternalServerError, errorResponse("InternalServerException", "internal server error"))
	}
}

// Purge implements service.Purgeable by removing all Bedrock Runtime invocation records older than cutoff.
func (h *Handler) Purge(ctx context.Context, cutoff time.Time) {
	h.Backend.Purge(ctx, cutoff)
}
