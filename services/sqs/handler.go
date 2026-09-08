package sqs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// sqsJSONContentType is the media type for SQS's JSON protocol responses.
// SQS uses the awsJson1_0 protocol (aws-sdk-go-v2/service/sqs@v1.46.4
// serializers.go sets this same value on every request), not 1.1.
const sqsJSONContentType = "application/x-amz-json-1.0"

// Handler is the Echo HTTP handler for SQS operations.
type Handler struct {
	Backend       StorageBackend
	janitor       *Janitor
	janitorCancel context.CancelFunc
	janitorDone   chan struct{}
	Endpoint      string
	DefaultRegion string
	janitorMu     sync.Mutex
}

// NewHandler creates a new SQS Handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// WithJanitor attaches a background janitor to the handler. It stops the
// backend's auto-started internal janitor so the two do not run concurrently
// and race on the shared lock.
func (h *Handler) WithJanitor(interval ...time.Duration) *Handler {
	if memBackend, ok := h.Backend.(*InMemoryBackend); ok {
		memBackend.stopInternalJanitor()

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

	runCtx, done := h.startJanitorLocked(ctx)
	if done == nil {
		return nil
	}

	go func() {
		defer close(done)
		h.janitor.Run(runCtx)
	}()

	return nil
}

// startJanitorLocked registers a new janitor run context under h.janitorMu,
// unless one is already running (in which case it returns a nil done channel
// to signal that StartWorker should not start a second run). Extracted from
// StartWorker so the locked region is a plain method body rather than a
// function literal, and so h.janitorCancel = cancel is a direct field store
// gosec can trace.
func (h *Handler) startJanitorLocked(ctx context.Context) (context.Context, chan struct{}) {
	h.janitorMu.Lock()
	defer h.janitorMu.Unlock()

	if h.janitorDone != nil {
		return nil, nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	h.janitorCancel = cancel
	h.janitorDone = done

	return runCtx, done
}

// Shutdown stops the janitor worker and waits for it to exit.
func (h *Handler) Shutdown(ctx context.Context) {
	done := h.stopJanitorLocked()
	if done == nil {
		return
	}

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// stopJanitorLocked cancels any active janitor run and clears the run
// bookkeeping under h.janitorMu, returning the run's done channel so Shutdown
// can wait for it to exit outside the lock (or nil if no run is active).
// Extracted from Shutdown so the locked region is a plain method body rather
// than a function literal, and so gosec sees h.janitorCancel loaded and called
// as a direct field access in this method (not through a nested closure).
func (h *Handler) stopJanitorLocked() chan struct{} {
	h.janitorMu.Lock()
	defer h.janitorMu.Unlock()

	cancel := h.janitorCancel
	done := h.janitorDone
	h.janitorCancel = nil
	h.janitorDone = nil

	if cancel == nil || done == nil {
		return nil
	}

	cancel()

	return done
}

var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
)

// Name returns the service name.
func (h *Handler) Name() string {
	return "SQS"
}

// Purge implements service.Purgeable by delegating to the backend structure if supported.
func (h *Handler) Purge(ctx context.Context, cutoff time.Time) {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Purge(ctx, cutoff)
	}
}

// GetSupportedOperations returns the list of supported SQS operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opAddPermission,
		opCancelMessageMoveTask,
		opChangeMessageVisibility,
		opChangeMessageVisibilityBatch,
		opCreateQueue,
		opDeleteMessage,
		opDeleteMessageBatch,
		opDeleteQueue,
		opGetQueueAttributes,
		opGetQueueURL,
		opListDeadLetterSourceQueues,
		opListMessageMoveTasks,
		opListQueueTags,
		opListQueues,
		opPurgeQueue,
		opReceiveMessage,
		opRemovePermission,
		opSendMessage,
		opSendMessageBatch,
		opSetQueueAttributes,
		opStartMessageMoveTask,
		opTagQueue,
		opUntagQueue,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "sqs" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this SQS instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.DefaultRegion} }

// RouteMatcher returns a function that matches incoming SQS requests.
// It accepts two request styles:
//  1. JSON protocol: POST with X-Amz-Target: AmazonSQS.<Action>
//  2. Query protocol: POST with Content-Type: application/x-www-form-urlencoded
//     and a recognised Action= body parameter
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		r := c.Request()

		// JSON protocol: X-Amz-Target header approach
		if strings.HasPrefix(r.Header.Get("X-Amz-Target"), "AmazonSQS.") {
			path := r.URL.Path

			return path == "/" || strings.HasPrefix(path, "/000000000000/")
		}

		// Query protocol: form-encoded POST with a known Action
		if !isQueryProtocol(r) {
			return false
		}

		body, err := httputils.ReadBody(r)
		if err != nil {
			// Body unreadable (e.g. oversized): fall back to the User-Agent
			// marker every aws-sdk-go-v2 sqs client sets (api_client.go's
			// AddSDKAgentKeyValue -- "api/sqs"). That still identifies this
			// as ours, so claim it and let Handler() produce the typed
			// error instead of masking the read failure as a 404.
			return service.MatchesUserAgentMarker(r.Header, "api/sqs")
		}

		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return false
		}

		return isKnownSQSAction(vals.Get("Action"))
	}
}

// sqsMatchPriority is lower than header-based matchers (e.g. SSM at 100) but higher
// than path-based matchers (e.g. Dashboard at 50).
const sqsMatchPriority = 75

// unknownOperation is the default operation name returned when the action cannot be determined.
const unknownOperation = "Unknown"

// SQS operation name constants shared between JSON and Query protocol dispatch.
const (
	opAddPermission                = "AddPermission"
	opCancelMessageMoveTask        = "CancelMessageMoveTask"
	opChangeMessageVisibility      = "ChangeMessageVisibility"
	opChangeMessageVisibilityBatch = "ChangeMessageVisibilityBatch"
	opCreateQueue                  = "CreateQueue"
	opDeleteMessage                = "DeleteMessage"
	opDeleteMessageBatch           = "DeleteMessageBatch"
	opDeleteQueue                  = "DeleteQueue"
	opGetQueueAttributes           = "GetQueueAttributes"
	opGetQueueURL                  = "GetQueueUrl"
	opListDeadLetterSourceQueues   = "ListDeadLetterSourceQueues"
	opListMessageMoveTasks         = "ListMessageMoveTasks"
	opListQueueTags                = "ListQueueTags"
	opListQueues                   = "ListQueues"
	opPurgeQueue                   = "PurgeQueue"
	opReceiveMessage               = "ReceiveMessage"
	opRemovePermission             = "RemovePermission"
	opSendMessage                  = "SendMessage"
	opSendMessageBatch             = "SendMessageBatch"
	opSetQueueAttributes           = "SetQueueAttributes"
	opStartMessageMoveTask         = "StartMessageMoveTask"
	opTagQueue                     = "TagQueue"
	opUntagQueue                   = "UntagQueue"
)

// MatchPriority returns the routing priority for the SQS handler.
func (h *Handler) MatchPriority() int {
	return sqsMatchPriority
}

// ExtractOperation extracts the SQS action from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	action := strings.TrimPrefix(target, "AmazonSQS.")

	if action == "" || action == target {
		return unknownOperation
	}

	return action
}

type extractQueueURLInput struct {
	QueueURL string `json:"QueueUrl"`
}

// ExtractResource extracts the queue name from the JSON request body's QueueUrl field.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var req extractQueueURLInput

	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		return ""
	}

	return queueNameFromURL(req.QueueURL)
}

// Handler returns the Echo handler function for SQS operations.
// It supports both the JSON protocol (X-Amz-Target) and the legacy Query
// (form-encoded) protocol.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		if isQueryProtocol(c.Request()) {
			return h.handleQueryProtocol(c)
		}

		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"SQS", sqsJSONContentType,
			h.GetSupportedOperations(),
			func(ctx context.Context, action string, body []byte) ([]byte, error) {
				return h.sqsRoute(ctx, c.Request(), action, body)
			},
			h.handleError,
		)
	}
}

type sqsDispatchFn func(ctx context.Context, r *http.Request, body []byte) (any, error)

func (h *Handler) sqsDispatchTable() map[string]sqsDispatchFn {
	return map[string]sqsDispatchFn{
		opCreateQueue:                  h.handleCreateQueue,
		opDeleteQueue:                  h.handleDeleteQueue,
		opListQueues:                   h.handleListQueues,
		opGetQueueURL:                  h.handleGetQueueURL,
		opGetQueueAttributes:           h.handleGetQueueAttributes,
		opSetQueueAttributes:           h.handleSetQueueAttributes,
		opSendMessage:                  h.handleSendMessage,
		opReceiveMessage:               h.handleReceiveMessage,
		opDeleteMessage:                h.handleDeleteMessage,
		opChangeMessageVisibility:      h.handleChangeMessageVisibility,
		opSendMessageBatch:             h.handleSendMessageBatch,
		opDeleteMessageBatch:           h.handleDeleteMessageBatch,
		opChangeMessageVisibilityBatch: h.handleChangeMessageVisibilityBatch,
		opPurgeQueue:                   h.handlePurgeQueue,
		opTagQueue:                     h.handleTagQueue,
		opUntagQueue:                   h.handleUntagQueue,
		opListQueueTags:                h.handleListQueueTags,
		opListDeadLetterSourceQueues:   h.handleListDeadLetterSourceQueues,
		opAddPermission:                h.handleAddPermission,
		opRemovePermission:             h.handleRemovePermission,
		opStartMessageMoveTask:         h.handleStartMessageMoveTask,
		opCancelMessageMoveTask:        h.handleCancelMessageMoveTask,
		opListMessageMoveTasks:         h.handleListMessageMoveTasks,
	}
}

// sqsRoute dispatches an SQS action to the appropriate handler method.
func (h *Handler) sqsRoute(
	ctx context.Context,
	r *http.Request,
	action string,
	body []byte,
) ([]byte, error) {
	fn, ok := h.sqsDispatchTable()[action]
	if !ok {
		return nil, ErrUnknownAction
	}

	result, err := fn(ctx, r, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

// handleError writes an SQS error response using the standard error details mapping.
func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	errType, message, status := errorDetails(err)

	payload, marshalErr := json.Marshal(jsonSQSError{Type: errType, Message: message})
	if marshalErr != nil {
		return marshalErr
	}

	c.Response().Header().Set("Content-Type", sqsJSONContentType)

	return c.JSONBlob(status, payload)
}

// --- JSON request types ---

type jsonSQSError struct {
	Type    string `json:"__type"`
	Message string `json:"message"`
}

// --- handler methods ---

const errTypeInvalidParameterValue = "com.amazonaws.sqs#InvalidParameterValue"

// invalidParameterValueMessage returns the AWS error message for parameter-validation
// sentinel errors, or ("", false) if the error is not a parameter error.
func invalidParameterValueMessage(err error) (string, bool) {
	if ipe, ok := errors.AsType[*InvalidParameterError](err); ok {
		return ipe.Message, true
	}

	switch {
	case errors.Is(err, ErrInvalidWaitTime):
		return "Value for parameter WaitTimeSeconds is invalid. Reason: Must be between 0 and 20, if provided.", true
	case errors.Is(err, ErrInvalidVisibilityTimeout):
		return "Value for parameter VisibilityTimeout is invalid. Reason: Must be between 0 and 43200, if provided.", true
	case errors.Is(err, ErrInvalidDelaySeconds):
		return "Value for parameter DelaySeconds is invalid. Reason: Must be between 0 and 900, if provided.", true
	case errors.Is(err, ErrMissingMessageGroupID):
		return "The request must contain the parameter MessageGroupId.", true
	case errors.Is(err, ErrMissingDeduplicationID):
		const dedupMsg = "The queue should either have ContentBasedDeduplication enabled" +
			" or MessageDeduplicationId provided explicitly."

		return dedupMsg, true
	default:
		return "", false
	}
}

// errorEntry maps a sentinel error to its SQS error type, message, and HTTP status code.
type errorEntry struct {
	errType string
	message string
	status  int
}

// errorDetails maps an error to its SQS JSON error type, message, and HTTP status.
func errorDetails(err error) (string, string, int) {
	if msg, ok := invalidParameterValueMessage(err); ok {
		return errTypeInvalidParameterValue, msg, http.StatusBadRequest
	}

	if e, ok := sqsErrorDetails(err); ok {
		return e.errType, e.message, e.status
	}

	return "com.amazonaws.sqs#InternalError",
		"An internal error occurred.",
		http.StatusInternalServerError
}

// sqsErrorDetails looks up an error in the well-known SQS error table.
// Extracted to keep errorDetails itself under the funlen limit.
// The table is split across two helpers to stay within funlen.
func sqsErrorDetails(err error) (errorEntry, bool) {
	if e, ok := sqsCoreErrorDetails(err); ok {
		return e, true
	}

	return sqsPermMoveErrorDetails(err)
}

// sqsCoreErrorDetails handles the core queue/message sentinel errors.
func sqsCoreErrorDetails(err error) (errorEntry, bool) {
	type errRow struct {
		sentinel error
		entry    errorEntry
	}

	const badReq = http.StatusBadRequest

	rows := [...]errRow{
		{
			ErrQueueNotFound,
			errorEntry{
				"com.amazonaws.sqs#QueueDoesNotExist",
				"The specified queue does not exist.",
				badReq,
			},
		},
		{
			ErrQueueAlreadyExists,
			errorEntry{
				"com.amazonaws.sqs#QueueNameExists",
				"A queue with this name already exists.",
				badReq,
			},
		},
		{
			ErrReceiptHandleInvalid,
			errorEntry{
				"com.amazonaws.sqs#ReceiptHandleIsInvalid",
				"The receipt handle is not valid.",
				badReq,
			},
		},
		{ErrMessageNotInflight, errorEntry{
			"com.amazonaws.sqs#MessageNotInflight",
			"The message referred to by the receipt handle is not in-flight.",
			badReq,
		}},
		{
			ErrTooManyEntriesInBatch,
			errorEntry{
				"com.amazonaws.sqs#TooManyEntriesInBatchRequest",
				"Too many entries in batch request.",
				badReq,
			},
		},
		{
			ErrBatchEntryIDsNotDistinct,
			errorEntry{
				"com.amazonaws.sqs#BatchEntryIdsNotDistinct",
				"Two or more batch entries in the request have the same Id.",
				badReq,
			},
		},
		{
			ErrInvalidBatchEntry,
			errorEntry{
				"com.amazonaws.sqs#EmptyBatchRequest",
				"The batch request is empty.",
				badReq,
			},
		},
		{
			ErrInvalidAttribute,
			errorEntry{
				"com.amazonaws.sqs#InvalidAttributeValue",
				"Invalid attribute value.",
				badReq,
			},
		},
		{
			ErrMessageTooLarge,
			errorEntry{
				"com.amazonaws.sqs#InvalidMessageContents",
				"The message exceeds the maximum message size.",
				badReq,
			},
		},
		{
			ErrUnknownAction,
			errorEntry{
				"com.amazonaws.sqs#InvalidAction",
				"The action or operation requested is invalid.",
				badReq,
			},
		},
	}

	for _, row := range rows {
		if errors.Is(err, row.sentinel) {
			return row.entry, true
		}
	}

	return errorEntry{}, false
}

// sqsPermMoveErrorDetails handles permission, move-task, and validation sentinel errors.
func sqsPermMoveErrorDetails(err error) (errorEntry, bool) {
	if e, ok := sqsPermErrorDetails(err); ok {
		return e, true
	}

	return sqsValidationErrorDetails(err)
}

// sqsPermErrorDetails covers permission and move-task sentinel errors.
func sqsPermErrorDetails(err error) (errorEntry, bool) {
	type errRow struct {
		sentinel error
		entry    errorEntry
	}

	const badReq = http.StatusBadRequest
	const ipv = errTypeInvalidParameterValue

	rows := [...]errRow{
		{ErrTaskHandleInvalid, errorEntry{ipv, "The task handle provided is not valid.", badReq}},
		{ErrInvalidPermissionLabel, errorEntry{
			ipv,
			"The value for the required parameter 'Label' is not valid. Reason: label must not be empty.",
			badReq,
		}},
		{ErrInvalidPermissionActions, errorEntry{
			ipv,
			"The value for 'Actions' is not valid. Reason: Actions must not be empty.",
			badReq,
		}},
		{ErrInvalidPermissionAccountIDs, errorEntry{
			ipv,
			"The value for 'AWSAccountIds' is not valid. Reason: AWSAccountIds must not be empty.",
			badReq,
		}},
		{ErrInvalidSourceArn, errorEntry{
			ipv,
			"The value for 'SourceArn' is not valid. Reason: SourceArn must not be empty.",
			badReq,
		}},
		{ErrInvalidMaxMessagesPerSecond, errorEntry{
			ipv,
			"The value for 'MaxNumberOfMessagesPerSecond' is not valid. Reason: must be >= 0.",
			badReq,
		}},
		// "com.amazonaws.sqs#ResourceInConflict" doesn't name any real type in
		// this SDK (sqs@v1.46.4 types/errors.go has no Conflict-named
		// exception at all), so neither StartMessageMoveTask's nor
		// CancelMessageMoveTask's own deserializeOpError could ever recognize
		// it. ipv is the same fallback this table already uses for every
		// other move-task condition none of these ops model as a typed
		// exception (see ErrTaskHandleInvalid above).
		{ErrMoveTaskAlreadyRunning, errorEntry{
			ipv,
			"A message move task already exists for the specified source queue.",
			badReq,
		}},
		{ErrMoveTaskNotRunning, errorEntry{
			ipv,
			"A message move task with the specified task handle is not running.",
			badReq,
		}},
	}

	for _, row := range rows {
		if errors.Is(err, row.sentinel) {
			return row.entry, true
		}
	}

	return errorEntry{}, false
}

// sqsValidationErrorDetails covers queue name, message, and limit sentinel errors.
func sqsValidationErrorDetails(err error) (errorEntry, bool) {
	type errRow struct {
		sentinel error
		entry    errorEntry
	}

	const badReq = http.StatusBadRequest
	const ipv = errTypeInvalidParameterValue

	rows := [...]errRow{
		{ErrInvalidQueueName, errorEntry{
			ipv,
			"The name of a queue can only include alphanumeric characters, hyphens, or underscores. " +
				"Queue name must be between 1 and 80 characters.",
			badReq,
		}},
		{ErrInvalidMessageBody, errorEntry{
			ipv,
			"The request includes a parameter that is not valid for this queue type.",
			badReq,
		}},
		{ErrInvalidMaxMessages, errorEntry{
			ipv,
			"Value for parameter MaxNumberOfMessages is invalid. Reason: must be between 1 and 10, if provided.",
			badReq,
		}},
		{ErrPurgeQueueInProgress, errorEntry{
			"com.amazonaws.sqs#PurgeQueueInProgress",
			"Only one PurgeQueue operation on SomeQueue is allowed every 60 seconds.",
			badReq,
		}},
		{ErrQueueDeletedRecently, errorEntry{
			"com.amazonaws.sqs#QueueDeletedRecently",
			"You must wait 60 seconds after deleting a queue before you can create another with the same name.",
			badReq,
		}},
		{ErrOverLimit, errorEntry{
			"OverLimit",
			"The specified action violates a service quota.",
			http.StatusForbidden,
		}},
		{ErrInvalidMessageAttributeValue, errorEntry{
			ipv,
			"Message attribute value is invalid. Check that the DataType and the associated value are correct.",
			badReq,
		}},
		{ErrInvalidAttributeName, errorEntry{
			"com.amazonaws.sqs#InvalidAttributeName",
			"Unknown Attribute FifoQueue.",
			badReq,
		}},
		{ErrFIFODelayNotSupported, errorEntry{
			ipv,
			"The request include parameter that is not valid for this queue type." +
				" DelaySeconds is not supported for FIFO queues.",
			badReq,
		}},
		{ErrBatchRequestTooLong, errorEntry{
			"com.amazonaws.sqs#BatchRequestTooLong",
			"The length of all the messages put together is more than the limit.",
			badReq,
		}},
	}

	for _, row := range rows {
		if errors.Is(err, row.sentinel) {
			return row.entry, true
		}
	}

	return errorEntry{}, false
}

// queueNameFromURL extracts the queue name from a full queue URL.
func queueNameFromURL(queueURL string) string {
	parts := strings.Split(queueURL, "/")
	if len(parts) == 0 {
		return ""
	}

	return parts[len(parts)-1]
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Reset()
	}
}

// --- JSON request/response types for new operations ---
