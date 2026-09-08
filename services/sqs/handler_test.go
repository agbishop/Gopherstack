package sqs_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/sqs"
)

func TestSQSHandler_ExtractResource_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "invalid_json_returns_empty",
			body: "not-json",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			req.Header.Set("X-Amz-Target", "AmazonSQS.GetQueueAttributes")
			c := e.NewContext(req, httptest.NewRecorder())

			assert.Equal(t, tt.want, h.ExtractResource(c))
		})
	}
}

func TestSQSHandler_Reset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "handler_reset_clears_queues"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doCreateQueue(t, h, "before-reset-queue")

			h.Reset()

			rec := doRequest(t, h, "ListQueues", map[string]any{})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				QueueURLs []string `json:"QueueUrls"`
			}

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Empty(t, resp.QueueURLs, tt.name)
		})
	}
}

func TestSQSChaosServiceName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "returns_sqs", want: "sqs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			assert.Equal(t, tt.want, h.ChaosServiceName())
		})
	}
}

// TestErrNilAppContext verifies the provider nil guard.
func TestErrNilAppContext(t *testing.T) {
	t.Parallel()

	p := &sqs.Provider{}
	_, err := p.Init(nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, sqs.ErrNilAppContext)
}

// TestProviderInit verifies normal provider init.
func TestProviderInit(t *testing.T) {
	t.Parallel()

	p := &sqs.Provider{}
	reg, err := p.Init(&service.AppContext{})
	require.NoError(t, err)
	assert.NotNil(t, reg)
}

// TestHandlerOpsLen verifies 24 operations are supported.
func TestHandlerOpsLen(t *testing.T) {
	t.Parallel()

	bk := sqs.NewInMemoryBackend()
	t.Cleanup(bk.Close)
	h := sqs.NewHandler(bk)
	assert.Len(t, h.GetSupportedOperations(), 23)
}

// errInternalTest is a sentinel used to exercise the default InternalError branch in errorDetails.
var errInternalTest = errors.New("unexpected internal error")

// jsonErr is a convenience struct for parsing JSON error responses.
type jsonErr struct {
	Type    string `json:"__type"`
	Message string `json:"message"`
}

func newTestHandler(t *testing.T) *sqs.Handler {
	t.Helper()

	backend := sqs.NewInMemoryBackend()
	t.Cleanup(backend.Close)

	return sqs.NewHandler(backend)
}

// doRequest sends a JSON request to the handler with the given X-Amz-Target action.
// Pass action="" to omit the X-Amz-Target header (tests missing action handling).
func doRequest(t *testing.T, h *sqs.Handler, action string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	} else {
		bodyBytes = []byte("{}")
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")

	if action != "" {
		req.Header.Set("X-Amz-Target", "AmazonSQS."+action)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func doCreateQueue(t *testing.T, h *sqs.Handler, name string) string {
	t.Helper()

	rec := doRequest(t, h, "CreateQueue", map[string]any{"QueueName": name})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		QueueURL string `json:"QueueUrl"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	return resp.QueueURL
}

// doRawRequest sends raw bytes to the handler with the given action header.
func doRawRequest(t *testing.T, h *sqs.Handler, action string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonSQS."+action)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

// errorBackend is a StorageBackend that always returns an error for all operations.
type errorBackend struct {
	err error
}

func (e *errorBackend) CreateQueue(_ *sqs.CreateQueueInput) (*sqs.CreateQueueOutput, error) {
	return nil, e.err
}

func (e *errorBackend) DeleteQueue(_ *sqs.DeleteQueueInput) error { return e.err }

func (e *errorBackend) ListQueues(_ *sqs.ListQueuesInput) (*sqs.ListQueuesOutput, error) {
	return nil, e.err
}

func (e *errorBackend) GetQueueURL(_ *sqs.GetQueueURLInput) (*sqs.GetQueueURLOutput, error) {
	return nil, e.err
}

func (e *errorBackend) GetQueueAttributes(
	_ *sqs.GetQueueAttributesInput,
) (*sqs.GetQueueAttributesOutput, error) {
	return nil, e.err
}

func (e *errorBackend) SetQueueAttributes(_ *sqs.SetQueueAttributesInput) error { return e.err }

func (e *errorBackend) SendMessage(_ *sqs.SendMessageInput) (*sqs.SendMessageOutput, error) {
	return nil, e.err
}

func (e *errorBackend) ReceiveMessage(
	_ *sqs.ReceiveMessageInput,
) (*sqs.ReceiveMessageOutput, error) {
	return nil, e.err
}

func (e *errorBackend) DeleteMessage(_ *sqs.DeleteMessageInput) error { return e.err }

func (e *errorBackend) ChangeMessageVisibility(
	_ *sqs.ChangeMessageVisibilityInput,
) error {
	return e.err
}

func (e *errorBackend) SendMessageBatch(
	_ *sqs.SendMessageBatchInput,
) (*sqs.SendMessageBatchOutput, error) {
	return nil, e.err
}

func (e *errorBackend) DeleteMessageBatch(
	_ *sqs.DeleteMessageBatchInput,
) (*sqs.DeleteMessageBatchOutput, error) {
	return nil, e.err
}

func (e *errorBackend) PurgeQueue(_ *sqs.PurgeQueueInput) error { return e.err }

func (e *errorBackend) TagQueue(_ *sqs.TagQueueInput) error { return e.err }

func (e *errorBackend) UntagQueue(_ *sqs.UntagQueueInput) error { return e.err }

func (e *errorBackend) ListQueueTags(_ *sqs.ListQueueTagsInput) (*sqs.ListQueueTagsOutput, error) {
	return nil, e.err
}

func (e *errorBackend) ChangeMessageVisibilityBatch(
	_ *sqs.ChangeMessageVisibilityBatchInput,
) (*sqs.ChangeMessageVisibilityBatchOutput, error) {
	return nil, e.err
}

func (e *errorBackend) ListDeadLetterSourceQueues(
	_ *sqs.ListDeadLetterSourceQueuesInput,
) (*sqs.ListDeadLetterSourceQueuesOutput, error) {
	return nil, e.err
}

func (e *errorBackend) AddPermission(_ *sqs.AddPermissionInput) error { return e.err }

func (e *errorBackend) RemovePermission(_ *sqs.RemovePermissionInput) error { return e.err }

func (e *errorBackend) StartMessageMoveTask(
	_ *sqs.StartMessageMoveTaskInput,
) (*sqs.StartMessageMoveTaskOutput, error) {
	return nil, e.err
}

func (e *errorBackend) CancelMessageMoveTask(
	_ *sqs.CancelMessageMoveTaskInput,
) (*sqs.CancelMessageMoveTaskOutput, error) {
	return nil, e.err
}

func (e *errorBackend) ListMessageMoveTasks(
	_ *sqs.ListMessageMoveTasksInput,
) (*sqs.ListMessageMoveTasksOutput, error) {
	return nil, e.err
}

func (e *errorBackend) ListAll() []sqs.QueueInfo { return nil }

func newErrorHandler(t *testing.T, err error) *sqs.Handler {
	t.Helper()

	return sqs.NewHandler(&errorBackend{err: err})
}

func TestHandlerActions_Routing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body            map[string]any
		name            string
		action          string
		wantBodyContain string
		wantCode        int
	}{
		{
			name:            "missing action",
			action:          "",
			body:            map[string]any{"QueueName": "test"},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "Missing X-Amz-Target",
		},
		{
			name:            "unknown action",
			action:          "NonExistentAction",
			body:            map[string]any{},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "InvalidAction",
		},
		{
			name:   "queue not found",
			action: "SendMessage",
			body: map[string]any{
				"QueueUrl":    "http://localhost/000000000000/nonexistent",
				"MessageBody": "hello",
			},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "QueueDoesNotExist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantBodyContain != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContain)
			}
		})
	}
}

func TestHandlerActions_NotFoundErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "DeleteQueue",
			action: "DeleteQueue",
			body:   map[string]any{"QueueUrl": "http://localhost/000000000000/noqueue"},
		},
		{
			name:   "PurgeQueue",
			action: "PurgeQueue",
			body:   map[string]any{"QueueUrl": "http://localhost/000000000000/noqueue"},
		},
		{
			name:   "GetQueueAttributes",
			action: "GetQueueAttributes",
			body:   map[string]any{"QueueUrl": "http://localhost/000000000000/noqueue"},
		},
		{
			name:   "SetQueueAttributes",
			action: "SetQueueAttributes",
			body: map[string]any{
				"QueueUrl":   "http://localhost/000000000000/noqueue",
				"Attributes": map[string]string{"VisibilityTimeout": "60"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.action, tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandlerActions_InvalidBody(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRawRequest(t, h, "CreateQueue", []byte("{invalid"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandlerActions_ErrorBackend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		backendErr      error
		body            map[string]any
		name            string
		action          string
		wantBodyContain string
		wantCode        int
	}{
		{
			name:       "queue name from URL edge cases",
			backendErr: sqs.ErrQueueNotFound,
			action:     "DeleteQueue",
			body:       map[string]any{"QueueUrl": ""},
			wantCode:   http.StatusBadRequest,
		},
		{
			name:            "ListQueues",
			backendErr:      sqs.ErrQueueNotFound,
			action:          "ListQueues",
			body:            map[string]any{},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "QueueDoesNotExist",
		},
		{
			name:            "invalid attribute",
			backendErr:      sqs.ErrInvalidAttribute,
			action:          "SetQueueAttributes",
			body:            map[string]any{"QueueUrl": "http://localhost/000000000000/q"},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "InvalidAttributeValue",
		},
		{
			name:       "too many entries in batch",
			backendErr: sqs.ErrTooManyEntriesInBatch,
			action:     "SendMessageBatch",
			body: map[string]any{
				"QueueUrl": "http://localhost/000000000000/q",
				"Entries":  []map[string]any{{"Id": "1", "MessageBody": "body"}},
			},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "TooManyEntriesInBatchRequest",
		},
		{
			name:            "internal error",
			backendErr:      errInternalTest,
			action:          "PurgeQueue",
			body:            map[string]any{"QueueUrl": "http://localhost/000000000000/q"},
			wantCode:        http.StatusInternalServerError,
			wantBodyContain: "InternalError",
		},
		{
			// Regression test: ErrInvalidDelaySeconds was missing from both
			// invalidParameterValueMessage and every errorDetails lookup table,
			// so it fell through to the default InternalError/500 branch
			// instead of the AWS-accurate 400 InvalidParameterValue.
			name:       "invalid delay seconds",
			backendErr: sqs.ErrInvalidDelaySeconds,
			action:     "SendMessage",
			body: map[string]any{
				"QueueUrl":    "http://localhost/000000000000/q",
				"MessageBody": "body",
			},
			wantCode:        http.StatusBadRequest,
			wantBodyContain: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newErrorHandler(t, tt.backendErr)
			rec := doRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantBodyContain != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContain)
			}
		})
	}
}

func TestHandlerRouteMatcher(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	matcher := h.RouteMatcher()
	e := echo.New()

	tests := []struct {
		name   string
		path   string
		target string // X-Amz-Target header; "" means omit
		want   bool
	}{
		{"match root path with SQS target", "/", "AmazonSQS.CreateQueue", true},
		{"match queue path with SQS target", "/000000000000/my-queue", "AmazonSQS.SendMessage", true},
		{"no match missing X-Amz-Target", "/", "", false},
		{"no match wrong path with SQS target", "/dashboard/sqs/create", "AmazonSQS.CreateQueue", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}

			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.want, matcher(c))
		})
	}
}

func TestHandlerIntrospection(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	assert.Equal(t, "SQS", h.Name())
	assert.Equal(t, 75, h.MatchPriority())
	assert.NotEmpty(t, h.GetSupportedOperations())

	t.Run("ExtractOperation", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name   string
			target string
			body   string
			want   string
		}{
			{
				name:   "valid",
				target: "AmazonSQS.SendMessage",
				body:   `{"QueueUrl":"http://x/000000000000/q"}`,
				want:   "SendMessage",
			},
			{
				name: "invalid body",
				body: "{invalid}",
				want: "Unknown",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				e := echo.New()
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/x-amz-json-1.0")

				if tt.target != "" {
					req.Header.Set("X-Amz-Target", tt.target)
				}

				c := e.NewContext(req, httptest.NewRecorder())
				assert.Equal(t, tt.want, h.ExtractOperation(c))
			})
		}
	})

	t.Run("ExtractResource", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name   string
			target string
			body   string
			want   string
		}{
			{
				name:   "valid",
				target: "AmazonSQS.SendMessage",
				body:   `{"QueueUrl":"http://x/000000000000/q"}`,
				want:   "q",
			},
			{
				name: "no QueueUrl",
				body: `{"Action":"SendMessage"}`,
				want: "",
			},
			{
				name: "invalid body",
				body: "{invalid}",
				want: "",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				e := echo.New()
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/x-amz-json-1.0")

				if tt.target != "" {
					req.Header.Set("X-Amz-Target", tt.target)
				}

				c := e.NewContext(req, httptest.NewRecorder())
				assert.Equal(t, tt.want, h.ExtractResource(c))
			})
		}
	})
}

// TestSQSHandler_JSONResponseContentType verifies the JSON-protocol
// response carries the AWS-accurate media type. The pinned SDK
// (aws-sdk-go-v2/service/sqs@v1.46.4 serializers.go) sets
// "application/x-amz-json-1.0" on every request it sends — SQS uses the
// awsJson1_0 protocol, not 1.1 — so a spec-conformant server response uses
// the same media type for both success and error bodies.
func TestSQSHandler_JSONResponseContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body    map[string]any
		name    string
		action  string
		wantErr bool
	}{
		{
			name:   "success_response",
			action: "ListQueues",
			body:   map[string]any{},
		},
		{
			name:    "error_response",
			action:  "GetQueueAttributes",
			body:    map[string]any{"QueueUrl": "http://localhost/000000000000/does-not-exist"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.action, tt.body)

			if tt.wantErr {
				require.NotEqual(t, http.StatusOK, rec.Code)
			} else {
				require.Equal(t, http.StatusOK, rec.Code)
			}

			assert.Equal(t, "application/x-amz-json-1.0", rec.Header().Get("Content-Type"))
		})
	}
}

func TestProviderNameAndInit(t *testing.T) {
	t.Parallel()

	p := &sqs.Provider{}
	assert.Equal(t, "SQS", p.Name())

	appCtx := &service.AppContext{Logger: slog.Default()}

	svc, err := p.Init(appCtx)
	require.NoError(t, err)
	assert.NotNil(t, svc)
}
