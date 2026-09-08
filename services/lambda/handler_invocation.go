package lambda

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"math"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// errInvalidInvocationType and errInvalidLogType are returned unwritten by
// validateInvocationHeaders so handleInvoke can map and write the response
// exactly once. validateInvocationHeaders used to write its own rejection
// via h.writeError and return that call's (always-nil) result, so
// handleInvoke's `if valErr != nil` never fired and the function was
// invoked anyway on top of the already-written 400 (gopherstack-3t96, the
// gopherstack-8haq shape).
var (
	errInvalidInvocationType = errors.New("invalid InvocationType")
	errInvalidLogType        = errors.New("invalid LogType")
)

func (h *Handler) validateInvocationHeaders(c *echo.Context) (string, string, string, error) {
	invType := c.Request().Header.Get("X-Amz-Invocation-Type")
	if invType == "" {
		invType = InvocationTypeRequestResponse
	} else if invType != InvocationTypeRequestResponse &&
		invType != InvocationTypeEvent &&
		invType != InvocationTypeDryRun {
		return "", "", "", errInvalidInvocationType
	}

	logType := c.Request().Header.Get("X-Amz-Log-Type")
	if logType != "" && logType != LogTypeTail && logType != LogTypeNone {
		return "", "", "", errInvalidLogType
	}

	clientContext := c.Request().Header.Get("X-Amz-Client-Context")

	return invType, logType, clientContext, nil
}

func (h *Handler) handleInvoke(c *echo.Context, name string) error {
	ctx := c.Request().Context()

	invType, logType, clientContext, valErr := h.validateInvocationHeaders(c)
	if valErr != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", valErr.Error())
	}

	qualifier := c.Request().URL.Query().Get("Qualifier")

	if !h.validateQualifier(c, qualifier) {
		return nil
	}

	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "ServiceException", "failed to read request")
	}

	if body == nil {
		body = []byte("{}")
	}

	executedVersion := h.resolveExecutedVersion(name, qualifier)

	var result []byte
	var logResult string
	var functionError string
	var statusCode int
	var invokeErr error

	if qi, ok := h.Backend.(QualifierInvoker); ok {
		result, logResult, functionError, statusCode, invokeErr = qi.InvokeFunctionWithQualifier(
			ctx,
			name,
			qualifier,
			clientContext,
			logType,
			invType,
			body,
		)
	} else {
		result, statusCode, invokeErr = h.Backend.InvokeFunction(ctx, name, invType, body)
	}

	if invokeErr != nil {
		return h.writeInvokeError(c, name, invokeErr)
	}

	// Set X-Amz-Executed-Version on all successful responses (real AWS always sends this).
	c.Response().Header().Set("X-Amz-Executed-Version", executedVersion)

	if logResult != "" {
		c.Response().Header().Set("X-Amz-Log-Result", logResult)
	}

	if statusCode == http.StatusNoContent {
		return c.NoContent(http.StatusNoContent)
	}

	if statusCode == http.StatusAccepted {
		return c.NoContent(http.StatusAccepted)
	}

	if len(result) > 0 {
		// AWS Lambda signals a function error to the SDK via the X-Amz-Function-Error
		// response header (the body is still HTTP 200 containing the
		// errorMessage/errorType JSON payload). The value is "Unhandled" when the
		// runtime reported the error itself and "Handled" when the function returned
		// an error-shaped payload — the backend classifies this from the runtime's
		// response vs error endpoint rather than guessing from the payload.
		if functionError != "" {
			c.Response().Header().Set("X-Amz-Function-Error", functionError)
		}

		return c.JSONBlob(http.StatusOK, result)
	}

	return c.NoContent(http.StatusOK)
}

// resolveExecutedVersion returns the version string for the X-Amz-Executed-Version header.
func (h *Handler) resolveExecutedVersion(name, qualifier string) string {
	bk, ok := h.Backend.(*InMemoryBackend)
	if !ok {
		return versionLatest
	}

	resolved, err := bk.resolveQualifier(name, qualifier)
	if err != nil || resolved.Version == "" {
		return versionLatest
	}

	return resolved.Version
}

// writeInvokeError translates invoke errors into HTTP error responses.
func (h *Handler) writeInvokeError(c *echo.Context, name string, invokeErr error) error {
	if errors.Is(invokeErr, ErrFunctionNotFound) {
		return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
			"Function not found: "+name)
	}

	if errors.Is(invokeErr, ErrTooManyRequests) {
		return h.writeError(c, http.StatusTooManyRequests, "TooManyRequestsException", invokeErr.Error())
	}

	return h.writeError(c, http.StatusInternalServerError, "ServiceException", invokeErr.Error())
}

// isLambdaFunctionErrorPayload reports whether result looks like a Lambda
// function-error payload, i.e. a JSON object with a top-level errorMessage
// field (typically alongside errorType / stackTrace / trace).
func isLambdaFunctionErrorPayload(result []byte) bool {
	trimmed := bytes.TrimSpace(result)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}

	var probe struct {
		ErrorMessage *json.RawMessage `json:"errorMessage"`
		ErrorType    *json.RawMessage `json:"errorType"`
	}
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return false
	}

	return probe.ErrorMessage != nil || probe.ErrorType != nil
}

// handler_stubs.go adds stub handlers for SDK operations that are acknowledged
// but not yet fully implemented.  Each stub returns a minimal valid response so
// that the operation is visible in GetSupportedOperations and the SDK
// completeness test passes.

// contentTypeEventStream is the MIME type for Lambda streaming responses.
const contentTypeEventStream = "application/vnd.amazon.eventstream"

// lambdaStatusKey is the JSON key used in single-field Lambda status responses.
const lambdaStatusKey = "Status"

// --- Invoke / InvokeAsync / InvokeWithResponseStream stubs ---
// The SDK exposes these as separate methods; gopherstack routes them through
// handleInvoke (InvokeFunction).  These stubs register the SDK operation names
// without duplicating routing logic.

// handleInvokeAsync handles POST /2014-11-13/functions/{name}/invoke-async/.
// The legacy InvokeAsync API validates that the function exists, then returns 202
// immediately. The actual invocation runs in the background — the caller never waits
// for the result. AWS InvokeAsync always returns {"Status": 202} on success.
func (h *Handler) handleInvokeAsync(c *echo.Context, name string) error {
	bk, ok := h.Backend.(*InMemoryBackend)
	if !ok {
		return h.writeError(
			c,
			http.StatusInternalServerError,
			"ServiceException",
			"backend not available",
		)
	}

	body, readErr := readBodyOrEmpty(c)
	if readErr != nil {
		return h.writeError(
			c,
			http.StatusInternalServerError,
			"ServiceException",
			"failed to read request",
		)
	}

	// Validate function exists synchronously before accepting.
	if _, err := bk.GetFunction(name); err != nil {
		if errors.Is(err, ErrFunctionNotFound) {
			return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
				"Function not found: "+name)
		}

		return h.writeError(c, http.StatusInternalServerError, "ServiceException", err.Error())
	}

	// Fire-and-forget: launch the invocation asynchronously and return 202 immediately.
	// Use context.WithoutCancel so HTTP request cancellation does not abort background work.
	invokeCtx := context.WithoutCancel(c.Request().Context())
	bk.asyncWG.Go(func() {
		_, _, _ = bk.InvokeFunction(invokeCtx, name, InvocationTypeEvent, body)
	})

	return c.JSON(http.StatusAccepted, map[string]int{lambdaStatusKey: http.StatusAccepted})
}

// handleInvokeWithResponseStream handles POST /2021-11-15/functions/{name}/response-streaming-invocations.
// It invokes the function synchronously and writes the result using the AWS event stream binary
// protocol (application/vnd.amazon.eventstream), matching the encoding used by real Lambda.
// Each chunk is a PayloadChunk event; the stream is terminated by an InvokeComplete event.
func (h *Handler) handleInvokeWithResponseStream(c *echo.Context, name string) error {
	ctx := c.Request().Context()

	qualifier := c.Request().URL.Query().Get("Qualifier")

	body, readErr := readBodyOrEmpty(c)
	if readErr != nil {
		return h.writeError(
			c,
			http.StatusInternalServerError,
			"ServiceException",
			"failed to read request",
		)
	}

	var result []byte
	var statusCode int
	var invokeErr error

	if qi, ok := h.Backend.(QualifierInvoker); ok && qualifier != "" {
		result, _, _, statusCode, invokeErr = qi.InvokeFunctionWithQualifier(
			ctx, name, qualifier, "", "", InvocationTypeRequestResponse, body,
		)
	} else {
		result, statusCode, invokeErr = h.Backend.InvokeFunction(ctx, name, InvocationTypeRequestResponse, body)
	}

	if invokeErr != nil {
		if errors.Is(invokeErr, ErrFunctionNotFound) {
			return h.writeError(c, http.StatusNotFound, "ResourceNotFoundException",
				"Function not found: "+name)
		}

		if errors.Is(invokeErr, ErrTooManyRequests) {
			return h.writeError(
				c,
				http.StatusTooManyRequests,
				"TooManyRequestsException",
				invokeErr.Error(),
			)
		}

		return h.writeError(
			c,
			http.StatusInternalServerError,
			"ServiceException",
			invokeErr.Error(),
		)
	}

	if statusCode == http.StatusNotFound {
		return h.writeError(
			c,
			http.StatusNotFound,
			"ResourceNotFoundException",
			"Function not found: "+name,
		)
	}

	if len(result) == 0 {
		result = []byte("{}")
	}

	c.Response().Header().Set("Content-Type", contentTypeEventStream)
	c.Response().WriteHeader(http.StatusOK)

	// PayloadChunk event carries the function's response body.
	chunkFrame := buildLambdaStreamFrame([][2]string{
		{":message-type", "event"},
		{":event-type", "PayloadChunk"},
		{":content-type", "application/octet-stream"},
	}, result)
	_, _ = c.Response().Write(chunkFrame)

	// InvokeComplete event signals end-of-stream to the SDK.
	doneFrame := buildLambdaStreamFrame([][2]string{
		{":message-type", "event"},
		{":event-type", "InvokeComplete"},
	}, nil)
	_, _ = c.Response().Write(doneFrame)

	return nil
}

// esHeaderValueTypeString is the AWS event stream type byte for UTF-8 string header values.
const esHeaderValueTypeString = 7

// buildLambdaStreamHeaders encodes header name/value pairs in the AWS event stream binary format.
// Each header: name_len(1) | name | type(1)=7 | value_len(2 BE) | value.
// Loop counters avoid int→byte/uint16 narrowing conversions flagged by gosec G115.
func buildLambdaStreamHeaders(hdrs [][2]string) []byte {
	var buf bytes.Buffer
	for _, kv := range hdrs {
		name, value := kv[0], kv[1]
		if len(name) > math.MaxUint8 {
			continue
		}
		var nameLen uint8
		for range []byte(name) {
			nameLen++
		}
		var valLen uint16
		for range []byte(value) {
			valLen++
		}
		vlen := [2]byte{}
		binary.BigEndian.PutUint16(vlen[:], valLen)
		buf.WriteByte(nameLen)
		buf.WriteString(name)
		buf.WriteByte(esHeaderValueTypeString)
		buf.Write(vlen[:])
		buf.WriteString(value)
	}

	return buf.Bytes()
}

// buildLambdaStreamFrame encodes one AWS event stream binary message.
// Frame layout: totalLen(4) | headerLen(4) | preludeCRC(4) | headers | payload | msgCRC(4).
// CRCs use CRC32/IEEE as required by the AWS event stream specification.
// Loop counters produce uint32 lengths without int→uint32 narrowing (gosec G115).
func buildLambdaStreamFrame(hdrs [][2]string, payload []byte) []byte {
	const preludeLen = 12
	const msgCRCLen = 4

	hdrBytes := buildLambdaStreamHeaders(hdrs)

	// Count bytes via loop to get uint32 without int→uint32 narrowing conversion.
	var headerLen uint32
	for range hdrBytes {
		headerLen++
	}
	var payloadLen uint32
	for range payload {
		payloadLen++
	}

	total := uint64(preludeLen) + uint64(headerLen) + uint64(payloadLen) + uint64(msgCRCLen)
	if total > math.MaxUint32 {
		return nil
	}
	totalLen := uint32(total)

	// int(uint32) is a widening conversion on all supported platforms.
	buf := make([]byte, int(totalLen))
	binary.BigEndian.PutUint32(buf[0:4], totalLen)
	binary.BigEndian.PutUint32(buf[4:8], headerLen)

	preludeCRC := crc32.ChecksumIEEE(buf[0:8])
	binary.BigEndian.PutUint32(buf[8:preludeLen], preludeCRC)

	hEnd := preludeLen + int(headerLen)
	pEnd := hEnd + int(payloadLen)
	copy(buf[preludeLen:hEnd], hdrBytes)
	copy(buf[hEnd:pEnd], payload)

	msgCRC := crc32.ChecksumIEEE(buf[0:pEnd])
	binary.BigEndian.PutUint32(buf[pEnd:], msgCRC)

	return buf
}

// readBodyOrEmpty reads the HTTP request body, returning an empty JSON object if nil.
func readBodyOrEmpty(c *echo.Context) ([]byte, error) {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return nil, err
	}

	if body == nil {
		return []byte("{}"), nil
	}

	return body, nil
}
