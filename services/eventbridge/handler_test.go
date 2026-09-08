package eventbridge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/eventbridge"
)

// makeRequest is a helper to send a POST request to the EventBridge handler.
func makeRequest(t *testing.T, action, body string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	backend := eventbridge.NewInMemoryBackend()
	handler := eventbridge.NewHandler(backend)

	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	} else {
		req = httptest.NewRequest(http.MethodPost, "/", nil)
	}

	if action != "" {
		req.Header.Set("X-Amz-Target", "AmazonEventBridge."+action)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Handler()(c)
	require.NoError(t, err)

	return rec
}

// makeRequestWithHandler sends a POST to a specific handler instance.
func makeRequestWithHandler(
	t *testing.T,
	handler *eventbridge.Handler,
	e *echo.Echo,
	action, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	} else {
		req = httptest.NewRequest(http.MethodPost, "/", nil)
	}

	if action != "" {
		req.Header.Set("X-Amz-Target", "AmazonEventBridge."+action)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantOps []string
	}{
		{
			name:    "GET returns list of supported operations",
			wantOps: []string{"CreateEventBus", "PutEvents"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			backend := eventbridge.NewInMemoryBackend()
			handler := eventbridge.NewHandler(backend)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)

			var ops []string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ops))
			for _, op := range tt.wantOps {
				assert.Contains(t, ops, op)
			}
		})
	}
}

func TestHandler_DispatchErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		method        string
		target        string
		body          string
		wantBody      string
		wantErrorType string
		wantCode      int
	}{
		{
			name:     "missing X-Amz-Target header returns bad request",
			method:   http.MethodPost,
			body:     "{}",
			wantCode: http.StatusBadRequest,
			wantBody: "Missing X-Amz-Target",
		},
		{
			name:     "PUT method returns method not allowed",
			method:   http.MethodPut,
			wantCode: http.StatusMethodNotAllowed,
		},
		{
			name:          "unknown action returns bad request with UnknownOperationException",
			method:        http.MethodPost,
			target:        "AmazonEventBridge.NonExistentAction",
			body:          "{}",
			wantCode:      http.StatusBadRequest,
			wantErrorType: "UnknownOperationException",
		},
		{
			name:          "delete default event bus returns IllegalStatusException",
			method:        http.MethodPost,
			target:        "AmazonEventBridge.DeleteEventBus",
			body:          `{"Name":"default"}`,
			wantCode:      http.StatusBadRequest,
			wantErrorType: "IllegalStatusException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			handler := eventbridge.NewHandler(eventbridge.NewInMemoryBackend())

			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, "/", strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, "/", nil)
			}
			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantBody != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBody)
			}
			if tt.wantErrorType != "" {
				var errResp service.JSONErrorResponse
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Equal(t, tt.wantErrorType, errResp.Type)
			}
		})
	}
}

func TestHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		wantMatch bool
	}{
		{
			name:      "matches AmazonEventBridge target",
			target:    "AmazonEventBridge.PutEvents",
			wantMatch: true,
		},
		{
			name:      "does not match non-EventBridge target",
			target:    "AmazonSSM.GetParameter",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			handler := eventbridge.NewHandler(eventbridge.NewInMemoryBackend())
			matcher := handler.RouteMatcher()

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.wantMatch, matcher(c))
		})
	}
}

func TestHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		wantOp string
	}{
		{
			name:   "extracts PutRule from AmazonEventBridge target header",
			target: "AmazonEventBridge.PutRule",
			wantOp: "PutRule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			handler := eventbridge.NewHandler(eventbridge.NewInMemoryBackend())

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.wantOp, handler.ExtractOperation(c))
		})
	}
}

func TestHandler_Shutdown_ImplementsShutdowner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		putEvents bool
	}{
		{
			name:      "shutdown on idle backend completes promptly",
			putEvents: false,
		},
		{
			name:      "shutdown after PutEvents drains in-flight deliveries",
			putEvents: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := eventbridge.NewInMemoryBackend()
			handler := eventbridge.NewHandler(backend)

			// Verify the handler satisfies the Shutdowner interface.
			var _ service.Shutdowner = handler

			if tt.putEvents {
				// Put an event so an async delivery goroutine may be in-flight.
				backend.PutEvents(context.Background(), []eventbridge.EventEntry{
					{Source: "test", DetailType: "test", Detail: `{}`},
				})
			}

			// Shutdown should complete without blocking or panicking.
			handler.Shutdown(t.Context())

			// After Shutdown, Close has been called and the backend's context is
			// cancelled — any subsequent PutEvents must not panic or deadlock.
			assert.NotPanics(t, func() {
				_, _ = backend.PutEvents(context.Background(), []eventbridge.EventEntry{
					{Source: "after-shutdown", DetailType: "test", Detail: `{}`},
				})
			})
		})
	}
}

func auditMakeRequest(
	t *testing.T,
	handler *eventbridge.Handler,
	e *echo.Echo,
	action string,
	body map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(b)))
	req.Header.Set("X-Amz-Target", "AmazonEventBridge."+action)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, handler.Handler()(c))

	return rec
}

func TestHandler_ResourceLimitExceededMapsTo400(t *testing.T) {
	t.Parallel()

	e := echo.New()
	b := newBackend()
	h := eventbridge.NewHandler(b)

	// Create 200 buses directly.
	for i := range 200 {
		_, err := b.CreateEventBus(
			context.Background(),
			eventbridge.CreateEventBusParams{Name: fmt.Sprintf("bus-%d", i)},
		)
		require.NoError(t, err)
	}

	// 201st via handler should return 400.
	rec := auditMakeRequest(t, h, e, "CreateEventBus", map[string]any{"Name": "bus-overflow"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	// CHANGED (orphan-code sweep, gopherstack-yatn): this assertion previously
	// checked for "ResourceLimitExceededException", which is not a real
	// eventbridge SDK error code -- CreateEventBus's own deserializeOpError
	// switch (eventbridge@v1.48.4 deserializers.go) declares
	// LimitExceededException, not this. The old assertion just confirmed the
	// bug, not correct behavior; see TestCreateEventBus_AccountLimit_RealClient
	// for the real-client proof.
	assert.Contains(t, rec.Body.String(), "LimitExceededException")
}

// TestHandler_GetSupportedOperationsExcludesPolicyOps verifies GetEventBusPolicy
// and PutEventBusPolicy are NOT advertised: neither is a real EventBridge SDK
// operation (see the doc comment beside their omission in
// Handler.GetSupportedOperations()), so a real AWS SDK client can never send
// them and the sdkcheck reverse-completeness check must not see them as
// "supported". They remain reachable internal-only via policyActions().
func TestHandler_GetSupportedOperationsExcludesPolicyOps(t *testing.T) {
	t.Parallel()
	h := eventbridge.NewHandler(newBackend())
	ops := h.GetSupportedOperations()

	assert.NotContains(t, ops, "GetEventBusPolicy")
	assert.NotContains(t, ops, "PutEventBusPolicy")
}

func TestHandler_GetSupportedOperationsIncludesDeliveryTargetTypes(t *testing.T) {
	t.Parallel()
	h := eventbridge.NewHandler(newBackend())
	ops := h.GetSupportedOperations()

	// Verify a broad sample of expected operations.
	expected := []string{
		"CreateEventBus", "DeleteEventBus", "ListEventBuses", "DescribeEventBus", "UpdateEventBus",
		"PutRule", "DeleteRule", "ListRules", "DescribeRule", "EnableRule", "DisableRule",
		"PutTargets", "RemoveTargets", "ListTargetsByRule",
		"PutEvents", "PutPartnerEvents",
		"CreateArchive", "DeleteArchive", "DescribeArchive", "ListArchives", "UpdateArchive",
		"StartReplay", "DescribeReplay", "ListReplays", "CancelReplay",
		"CreateConnection", "DeleteConnection", "DescribeConnection", "ListConnections", "UpdateConnection",
		"DeauthorizeConnection",
		"CreateApiDestination", "DeleteApiDestination", "DescribeApiDestination",
		"ListApiDestinations", "UpdateApiDestination",
		"CreateEndpoint", "DeleteEndpoint", "DescribeEndpoint", "ListEndpoints", "UpdateEndpoint",
		"PutPermission", "RemovePermission",
		"TagResource", "UntagResource", "ListTagsForResource",
		"TestEventPattern", "ListRuleNamesByTarget",
	}

	for _, op := range expected {
		assert.Contains(t, ops, op, "missing operation: %s", op)
	}
}

func TestHandler_SchemaOperationsIncluded(t *testing.T) {
	t.Parallel()
	h := eventbridge.NewHandler(newBackend())
	ops := h.GetSupportedOperations()

	schemaOps := []string{
		"CreateRegistry", "DeleteRegistry", "DescribeRegistry", "ListRegistries", "UpdateRegistry",
		"CreateSchema", "DeleteSchema", "DescribeSchema", "ListSchemas", "SearchSchemas", "UpdateSchema",
		"ListSchemaVersions", "DeleteSchemaVersion",
		"GetDiscoveredSchema",
		"PutCodeBinding", "DescribeCodeBinding", "GetCodeBindingSource",
	}
	for _, op := range schemaOps {
		assert.Contains(t, ops, op, "missing schema operation: %s", op)
	}
}

func TestHandler_Metadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		want  any
		check func(h *eventbridge.Handler) any
		name  string
	}{
		{
			name:  "name is EventBridge",
			check: func(h *eventbridge.Handler) any { return h.Name() },
			want:  "EventBridge",
		},
		{
			name:  "match priority is 100",
			check: func(h *eventbridge.Handler) any { return h.MatchPriority() },
			want:  100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := eventbridge.NewHandler(eventbridge.NewInMemoryBackend())
			assert.Equal(t, tt.want, tt.check(h))
		})
	}
}

func TestHandler_RouteMatcherCoverage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		wantMatch bool
	}{
		{
			name:      "matches EventBridge target",
			target:    "AmazonEventBridge.CreateEventBus",
			wantMatch: true,
		},
		{
			name:      "does not match non-EventBridge target",
			target:    "AmazonSQS.CreateQueue",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := eventbridge.NewHandler(eventbridge.NewInMemoryBackend())
			matcher := h.RouteMatcher()
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			assert.Equal(t, tt.wantMatch, matcher(e.NewContext(req, httptest.NewRecorder())))
		})
	}
}

func TestHandler_ExtractOperationCoverage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		wantOp string
	}{
		{
			name:   "extracts PutEvents operation",
			target: "AmazonEventBridge.PutEvents",
			wantOp: "PutEvents",
		},
		{
			name:   "returns Unknown for missing target",
			target: "",
			wantOp: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := eventbridge.NewHandler(eventbridge.NewInMemoryBackend())
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}
			assert.Equal(t, tt.wantOp, h.ExtractOperation(e.NewContext(req, httptest.NewRecorder())))
		})
	}
}

func TestHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantRes   string
		wantEmpty bool
	}{
		{
			name:    "extracts Name field",
			body:    `{"Name":"my-bus"}`,
			wantRes: "my-bus",
		},
		{
			name:    "extracts Rule field",
			body:    `{"Rule":"my-rule"}`,
			wantRes: "my-rule",
		},
		{
			name:      "returns empty when no Name or Rule",
			body:      `{}`,
			wantEmpty: true,
		},
		{
			name:      "returns empty for invalid JSON",
			body:      `not-json`,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := eventbridge.NewHandler(eventbridge.NewInMemoryBackend())
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			got := h.ExtractResource(e.NewContext(req, httptest.NewRecorder()))
			if tt.wantEmpty {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tt.wantRes, got)
			}
		})
	}
}

func TestHandler_InvalidTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		target   string
		body     string
		wantCode int
	}{
		{
			name:     "invalid target returns bad request",
			target:   "InvalidTarget",
			body:     "{}",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := echo.New()

			h := eventbridge.NewHandler(eventbridge.NewInMemoryBackend())
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			req.Header.Set("X-Amz-Target", tt.target)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_NotFoundErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
		body   string
	}{
		{name: "delete nonexistent event bus", action: "DeleteEventBus", body: `{"Name":"nonexistent"}`},
		{name: "describe nonexistent event bus", action: "DescribeEventBus", body: `{"Name":"nonexistent"}`},
		{name: "delete nonexistent rule", action: "DeleteRule", body: `{"Name":"r","EventBusName":"default"}`},
		{name: "describe nonexistent rule", action: "DescribeRule", body: `{"Name":"r","EventBusName":"default"}`},
		{name: "enable nonexistent rule", action: "EnableRule", body: `{"Name":"r","EventBusName":"default"}`},
		{name: "disable nonexistent rule", action: "DisableRule", body: `{"Name":"r","EventBusName":"default"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := makeRequest(t, tt.action, tt.body)
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		action   string
		body     string
		wantCode int
	}{
		{
			// JSON parse errors are mapped to 500 InternalServerError
			name:     "invalid JSON returns internal server error",
			action:   "CreateEventBus",
			body:     `not-json`,
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := makeRequest(t, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestHandlerOpsPreBuilt verifies ops are cached at construction time.
func TestHandlerOpsPreBuilt(t *testing.T) {
	t.Parallel()

	backend := eventbridge.NewInMemoryBackend()
	handler := eventbridge.NewHandler(backend)

	assert.Positive(t, handler.HandlerOpsLen())
}

// TestGetSupportedOperations_AllOps checks all 27 ops are present.
func TestGetSupportedOperations_AllOps(t *testing.T) {
	t.Parallel()

	backend := eventbridge.NewInMemoryBackend()
	handler := eventbridge.NewHandler(backend)
	ops := handler.GetSupportedOperations()

	required := []string{
		"ActivateEventSource", "CancelReplay", "CreateApiDestination",
		"CreateArchive", "CreateConnection", "CreateEndpoint",
		"CreatePartnerEventSource", "DeactivateEventSource",
		"DeauthorizeConnection", "DeleteApiDestination",
		"CreateEventBus", "DeleteEventBus", "ListEventBuses",
		"DescribeEventBus", "PutRule", "DeleteRule", "ListRules",
		"DescribeRule", "EnableRule", "DisableRule", "PutTargets",
		"RemoveTargets", "ListTargetsByRule", "PutEvents",
		"ListTagsForResource", "TagResource", "UntagResource",
	}

	opSet := make(map[string]bool, len(ops))
	for _, op := range ops {
		opSet[op] = true
	}

	for _, op := range required {
		assert.True(t, opSet[op], "expected op %s in GetSupportedOperations", op)
	}
}

// TestErrValidationMapping verifies ErrInvalidParameter maps to HTTP 400.
func TestErrValidationMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
		body   string
	}{
		{
			name:   "activate_event_source_empty_name",
			action: "ActivateEventSource",
			body:   `{"Name":""}`,
		},
		{
			name:   "cancel_replay_empty_name",
			action: "CancelReplay",
			body:   `{"ReplayName":""}`,
		},
		{
			name:   "create_api_destination_no_name",
			action: "CreateApiDestination",
			body:   `{"ConnectionArn":"arn:x","InvocationEndpoint":"https://x","HttpMethod":"GET"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := makeRequest(t, tt.action, tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestErrNotFoundMapping verifies ErrNotFound maps to HTTP 404.
func TestErrNotFoundMapping(t *testing.T) {
	t.Parallel()

	rec := makeRequest(t, "CancelReplay", `{"ReplayName":"no-such-replay"}`)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestErrAlreadyExistsMapping verifies ErrAlreadyExists maps to HTTP 409.
func TestErrAlreadyExistsMapping(t *testing.T) {
	t.Parallel()

	b := eventbridge.NewInMemoryBackend()
	handler := eventbridge.NewHandler(b)
	e := echo.New()

	body := `{"Name":"my-conn","AuthorizationType":"BASIC"}`
	rec1 := makeRequestWithHandler(t, handler, e, "CreateConnection", body)
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := makeRequestWithHandler(t, handler, e, "CreateConnection", body)
	assert.Equal(t, http.StatusConflict, rec2.Code)
}

// TestCancelReplay_TerminalStateMapping verifies a terminal-state CancelReplay
// maps to HTTP 400 IllegalStatusException, not InvalidStateException --
// CancelReplay's own deserializeOpError switch (eventbridge deserializers.go)
// declares IllegalStatusException; InvalidStateException is correct only for
// ActivateEventSource/CreateEventBus/DeactivateEventSource (gopherstack-uox6
// sweep). Asserting only the status here would not have caught the
// wrong-code bug this test used to pin (both codes map to 400).
func TestCancelReplay_TerminalStateMapping(t *testing.T) {
	t.Parallel()

	b := eventbridge.NewInMemoryBackend()
	b.AddReplayInternal(&eventbridge.Replay{ReplayName: "r1", State: "COMPLETED"})
	handler := eventbridge.NewHandler(b)
	e := echo.New()

	rec := makeRequestWithHandler(t, handler, e, "CancelReplay", `{"ReplayName":"r1"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp service.JSONErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "IllegalStatusException", errResp.Type)
}
