package dynamodbstreams

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	ddbbackend "github.com/blackbirdworks/gopherstack/services/dynamodb"
)

const (
	targetPrefix = "DynamoDBStreams_20120810."

	keyErrType = "__type"
	keyMessage = "message"
)

var errUnknownOperation = errors.New("UnknownOperationException")

// Handler handles HTTP requests for DynamoDB Streams operations.
//
// Handler intentionally owns no independent state and therefore does not
// implement the Snapshot/Restore persistable shape that cli.go's
// setupPersistence auto-registers. Streams is wired (in cli.go's
// wireDynamoDBStreams) directly to the DynamoDB service's *InMemoryDB
// backend, which already persists everything durable about streams --
// StreamARN, StreamsEnabled, StreamViewType, StreamCreatedAt, and the
// StreamRecords ring buffer -- as part of each Table in its own
// snapshot/restore (see services/dynamodb/persistence.go). Implementing
// Snapshot/Restore here would register a second "DynamoDBStreams" entry
// that duplicates and re-restores that same shared backend object. Shard
// iterators (GetShardIterator) are genuinely ephemeral request-scoped
// tokens with a short TTL in real AWS too, so not persisting them matches
// AWS behavior rather than being a gap. See persistence_test.go for the
// guard test on this invariant.
type Handler struct {
	Streams       ddbbackend.StreamsBackend
	DefaultRegion string
}

// NewHandler creates a new DynamoDB Streams handler with the given backend.
func NewHandler(backend ddbbackend.StreamsBackend) *Handler {
	return &Handler{Streams: backend}
}

// Name returns the service identifier.
func (h *Handler) Name() string { return "DynamoDBStreams" }

// GetSupportedOperations returns the list of supported DynamoDB Streams operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"DescribeStream",
		"GetRecords",
		"GetShardIterator",
		"ListStreams",
	}
}

// RouteMatcher returns a matcher for DynamoDB Streams requests (by X-Amz-Target header).
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, targetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the DynamoDB Streams operation from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")

	return strings.TrimPrefix(target, targetPrefix)
}

// ExtractResource extracts the stream ARN from the DynamoDB Streams request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	if v, ok := data["StreamArn"]; ok {
		if s, strOk := v.(string); strOk {
			return s
		}
	}

	return ""
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "dynamodbstreams" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this DynamoDB Streams handler handles.
func (h *Handler) ChaosRegions() []string {
	if h.DefaultRegion != "" {
		return []string{h.DefaultRegion}
	}

	return []string{}
}

// Handler returns the Echo handler function for DynamoDB Streams requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		// Inject the per-request AWS region (from SigV4 credential scope) so that
		// backend operations are correctly scoped to the request's region.
		region := httputils.ExtractRegionFromRequest(c.Request(), h.DefaultRegion)
		ctx = ddbbackend.WithRegion(ctx, region)

		operation := h.ExtractOperation(c)

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "failed to read request body", "error", err)

			return writeDynamoDBStreamsDispatchError(c, "internal server error")
		}

		log.DebugContext(ctx, "DynamoDB Streams request", "operation", operation)

		response, reqErr := h.dispatch(ctx, operation, body)
		if reqErr != nil {
			return h.handleError(ctx, c, operation, reqErr)
		}

		payload, err := json.Marshal(response)
		if err != nil {
			log.ErrorContext(ctx, "failed to marshal JSON response", "error", err)

			return writeDynamoDBStreamsDispatchError(c, "internal server error")
		}

		checksum := crc32.ChecksumIEEE(payload)
		c.Response().Header().Set("X-Amz-Crc32", strconv.FormatUint(uint64(checksum), 10))
		c.Response().Header().Set("Content-Type", "application/x-amz-json-1.0")

		return c.JSONBlob(http.StatusOK, payload)
	}
}

func (h *Handler) dispatch(ctx context.Context, operation string, body []byte) (any, error) {
	if h.Streams == nil {
		return nil, fmt.Errorf("%w:%s", errUnknownOperation, operation)
	}

	switch operation {
	case "DescribeStream":
		return dispatchDescribeStream(ctx, body, h.Streams.DescribeStream)
	case "GetShardIterator":
		return dispatchStreamsOp(ctx, body, h.Streams.GetShardIterator)
	case "GetRecords":
		return dispatchGetRecords(ctx, body, h.Streams.GetRecords)
	case "ListStreams":
		return dispatchStreamsOp(ctx, body, h.Streams.ListStreams)
	default:
		return nil, fmt.Errorf("%w:%s", errUnknownOperation, operation)
	}
}

func dispatchStreamsOp[In any, Out any](
	ctx context.Context,
	body []byte,
	op func(context.Context, *In) (*Out, error),
) (any, error) {
	var input In
	if len(body) > 0 {
		if err := json.Unmarshal(body, &input); err != nil {
			return nil, err
		}
	}

	return op(ctx, &input)
}

func dispatchGetRecords(
	ctx context.Context,
	body []byte,
	op func(context.Context, *dynamodbstreams.GetRecordsInput) (*dynamodbstreams.GetRecordsOutput, error),
) (any, error) {
	var input dynamodbstreams.GetRecordsInput
	if len(body) > 0 {
		if err := json.Unmarshal(body, &input); err != nil {
			return nil, err
		}
	}

	out, err := op(ctx, &input)
	if err != nil {
		return nil, err
	}

	return ddbbackend.ToWireGetRecordsOutput(out)
}

// writeDynamoDBStreamsDispatchError writes a typed awsjson1.0 error for a
// failure in Handler() itself (body-read or response-marshal failure) --
// framework-level errors that never reach the service's own handleError. A
// bare text/plain body here cannot be parsed by the real SDK's
// __type/message JSON error decoder, so the client would see
// smithy.GenericAPIError{Code:"UnknownError"} instead of InternalServerError
// (gopherstack-o7gx).
func writeDynamoDBStreamsDispatchError(c *echo.Context, message string) error {
	body, _ := json.Marshal(map[string]string{
		keyErrType: "com.amazonaws.dynamodbstreams.v20120810#InternalServerError",
		keyMessage: message,
	})
	c.Response().Header().Set("Content-Type", "application/x-amz-json-1.0")

	return c.JSONBlob(http.StatusInternalServerError, body)
}

func (h *Handler) handleError(_ context.Context, c *echo.Context, operation string, reqErr error) error {
	if strings.HasPrefix(reqErr.Error(), "UnknownOperationException:") {
		body := []byte(
			`{"__type":"com.amazon.coral.service#UnknownOperationException","message":"Action not supported"}`,
		)
		c.Response().Header().Set("Content-Type", "application/x-amz-json-1.0")

		return c.JSONBlob(http.StatusBadRequest, body)
	}

	// If the backend returned a structured error (e.g. ResourceNotFoundException,
	// ExpiredIteratorException), propagate its __type and message directly so the
	// AWS SDK client can unmarshal the correct error type.
	if backendErr, ok := errors.AsType[*ddbbackend.Error](reqErr); ok {
		httpStatus := http.StatusBadRequest
		// InternalServerError maps to 500; all other DynamoDB Streams errors map to 400.
		if strings.HasSuffix(backendErr.Type, "#InternalServerError") {
			httpStatus = http.StatusInternalServerError
		}

		// Rewrite the DynamoDB service namespace to the DynamoDB Streams namespace so
		// the AWS SDK client resolves the correct error type. Real AWS returns error
		// types prefixed with "com.amazonaws.dynamodbstreams.v20120810#".
		errType := strings.ReplaceAll(
			backendErr.Type,
			"com.amazonaws.dynamodb.v20120810#",
			"com.amazonaws.dynamodbstreams.v20120810#",
		)

		body, _ := json.Marshal(map[string]string{
			keyErrType: errType,
			keyMessage: backendErr.Message,
		})
		c.Response().Header().Set("Content-Type", "application/x-amz-json-1.0")

		return c.JSONBlob(httpStatus, body)
	}

	// Generic fallback for errors without structured type information.
	msg := reqErr.Error()
	body, _ := json.Marshal(map[string]string{
		keyErrType: "com.amazonaws.dynamodbstreams.v20120810#" + operation + "Exception",
		keyMessage: msg,
	})
	c.Response().Header().Set("Content-Type", "application/x-amz-json-1.0")

	return c.JSONBlob(http.StatusBadRequest, body)
}

func dispatchDescribeStream(
	ctx context.Context,
	body []byte,
	op func(context.Context, *dynamodbstreams.DescribeStreamInput) (*dynamodbstreams.DescribeStreamOutput, error),
) (any, error) {
	var input dynamodbstreams.DescribeStreamInput
	if len(body) > 0 {
		if err := json.Unmarshal(body, &input); err != nil {
			return nil, err
		}
	}

	out, err := op(ctx, &input)
	if err != nil {
		return nil, err
	}

	return ddbbackend.ToWireDescribeStreamOutput(out), nil
}
