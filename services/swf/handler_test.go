package swf_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/swf"
)

func newTestSWFHandler(t *testing.T) *swf.Handler {
	t.Helper()

	return swf.NewHandler(swf.NewInMemoryBackend())
}

func doSWFRequest(t *testing.T, h *swf.Handler, action string, body any) *httptest.ResponseRecorder {
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
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "SimpleWorkflowService."+action)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

// parseSWFResp decodes a handler response body into a generic map.
func parseSWFResp(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &m))

	return m
}

type setupAction struct {
	body   any
	action string
}

// createSWFDomain registers a domain via the handler for test setup.
func createSWFDomain(t *testing.T, h *swf.Handler, name string) {
	t.Helper()

	rec := doSWFRequest(t, h, "RegisterDomain", map[string]any{
		"name":                                   name,
		"workflowExecutionRetentionPeriodInDays": "10",
	})
	require.Equal(t, http.StatusOK, rec.Code)
}

// createSWFWorkflowType registers a workflow type via the handler for test setup.
func createSWFWorkflowType(t *testing.T, h *swf.Handler, domain, name string) {
	t.Helper()

	rec := doSWFRequest(t, h, "RegisterWorkflowType", map[string]any{
		"domain":  domain,
		"name":    name,
		"version": "1.0",
	})
	require.Equal(t, http.StatusOK, rec.Code)
}

// startSWFExecution starts a workflow execution via the handler for test
// setup and returns its runId.
func startSWFExecution(t *testing.T, h *swf.Handler, domain, workflowType, execID string) string {
	t.Helper()

	rec := doSWFRequest(t, h, "StartWorkflowExecution", map[string]any{
		"domain":     domain,
		"workflowId": execID,
		"workflowType": map[string]any{
			"name":    workflowType,
			"version": "1.0",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseSWFResp(t, rec)
	runID, _ := resp["runId"].(string)
	require.NotEmpty(t, runID)

	return runID
}

func TestSWFHandler_Actions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body              any
		name              string
		action            string
		wantRespContains  string
		wantNotEmptyField string
		setup             []setupAction
		wantCode          int
	}{
		{
			name:     "RegisterDomain",
			action:   "RegisterDomain",
			body:     map[string]any{"name": "my-domain", "description": "test"},
			wantCode: http.StatusOK,
		},
		{
			name: "ListDomains",
			setup: []setupAction{
				{action: "RegisterDomain", body: map[string]any{"name": "d1"}},
				{action: "RegisterDomain", body: map[string]any{"name": "d2"}},
			},
			action:           "ListDomains",
			body:             map[string]any{"registrationStatus": "REGISTERED"},
			wantCode:         http.StatusOK,
			wantRespContains: "domainInfos",
		},
		{
			name:     "DeprecateDomain",
			setup:    []setupAction{{action: "RegisterDomain", body: map[string]any{"name": "my-domain"}}},
			action:   "DeprecateDomain",
			body:     map[string]any{"name": "my-domain"},
			wantCode: http.StatusOK,
		},
		{
			name:     "DeprecateDomain_NotFound",
			action:   "DeprecateDomain",
			body:     map[string]any{"name": "nonexistent"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "RegisterWorkflowType",
			setup:    []setupAction{{action: "RegisterDomain", body: map[string]any{"name": "my-domain"}}},
			action:   "RegisterWorkflowType",
			body:     map[string]any{"domain": "my-domain", "name": "my-workflow", "version": "1.0"},
			wantCode: http.StatusOK,
		},
		{
			name: "ListWorkflowTypes",
			setup: []setupAction{
				{action: "RegisterDomain", body: map[string]any{"name": "d1"}},
				{action: "RegisterWorkflowType", body: map[string]any{"domain": "d1", "name": "wf1", "version": "1.0"}},
			},
			action:           "ListWorkflowTypes",
			body:             map[string]any{"domain": "d1"},
			wantCode:         http.StatusOK,
			wantRespContains: "typeInfos",
		},
		{
			name: "StartWorkflowExecution",
			setup: []setupAction{
				{action: "RegisterDomain", body: map[string]any{"name": "my-domain"}},
			},
			action:            "StartWorkflowExecution",
			body:              map[string]any{"domain": "my-domain", "workflowId": "wf-001"},
			wantCode:          http.StatusOK,
			wantNotEmptyField: "runId",
		},
		{
			name: "DescribeWorkflowExecution",
			setup: []setupAction{
				{action: "RegisterDomain", body: map[string]any{"name": "d1"}},
				{action: "StartWorkflowExecution", body: map[string]any{"domain": "d1", "workflowId": "wf-001"}},
			},
			action:           "DescribeWorkflowExecution",
			body:             map[string]any{"domain": "d1", "execution": map[string]any{"workflowId": "wf-001"}},
			wantCode:         http.StatusOK,
			wantRespContains: "executionInfo",
		},
		{
			name:     "UnknownAction",
			action:   "UnknownAction",
			body:     nil,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "RegisterDomain_AlreadyExists",
			setup:    []setupAction{{action: "RegisterDomain", body: map[string]any{"name": "my-domain"}}},
			action:   "RegisterDomain",
			body:     map[string]any{"name": "my-domain"},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "RegisterDomain_Deprecated",
			setup: []setupAction{
				{action: "RegisterDomain", body: map[string]any{"name": "my-domain"}},
				{action: "DeprecateDomain", body: map[string]any{"name": "my-domain"}},
			},
			action:   "RegisterDomain",
			body:     map[string]any{"name": "my-domain"},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "RegisterWorkflowType_AlreadyExists",
			setup: []setupAction{
				{action: "RegisterDomain", body: map[string]any{"name": "my-domain"}},
				{action: "RegisterWorkflowType", body: map[string]any{
					"domain": "my-domain", "name": "my-wf", "version": "1.0",
				}},
			},
			action:   "RegisterWorkflowType",
			body:     map[string]any{"domain": "my-domain", "name": "my-wf", "version": "1.0"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "DescribeWorkflowExecution_NotFound",
			action:   "DescribeWorkflowExecution",
			body:     map[string]any{"domain": "d1", "execution": map[string]any{"workflowId": "nonexistent"}},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSWFHandler(t)

			for _, s := range tt.setup {
				doSWFRequest(t, h, s.action, s.body)
			}

			rec := doSWFRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantRespContains != "" {
				resp := parseSWFResp(t, rec)
				assert.Contains(t, resp, tt.wantRespContains)
			}

			if tt.wantNotEmptyField != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp[tt.wantNotEmptyField])
			}
		})
	}
}

// TestHandler_ValidationError_Returns400 verifies validation failures surface
// as HTTP 400 with a ValidationException __type.
func TestHandler_ValidationError_Returns400(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)

	// Register domain with empty name should return 400.
	rec := doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": ""})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	resp := parseSWFResp(t, rec)
	assert.Equal(t, "ValidationException", resp["__type"])
}

func TestSWFHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		wantMatch bool
	}{
		{
			name:      "Match",
			target:    "SimpleWorkflowService.RegisterDomain",
			wantMatch: true,
		},
		{
			name:      "NoMatch",
			target:    "Firehose_20150804.CreateDeliveryStream",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSWFHandler(t)
			matcher := h.RouteMatcher()

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			c := e.NewContext(req, httptest.NewRecorder())

			assert.Equal(t, tt.wantMatch, matcher(c))
		})
	}
}

// TestSWFHandler_ResponseContentType verifies the response Content-Type
// matches real SWF's wire protocol. SWF uses awsjson1.0
// (application/x-amz-json-1.0), unlike the more common awsjson1.1 protocol
// most other AWS JSON services use -- see the Content-Type/X-Amz-Target
// headers the real aws-sdk-go-v2 swf serializer sets on every operation.
func TestSWFHandler_ResponseContentType(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)
	rec := doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "my-domain"})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/x-amz-json-1.0", rec.Header().Get("Content-Type"))
}

func TestSWFHandler_Name(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)
	assert.Equal(t, "SWF", h.Name())
}

func TestSWFHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)
	ops := h.GetSupportedOperations()
	assert.Contains(t, ops, "RegisterDomain")
	assert.Contains(t, ops, "ListDomains")
	assert.Contains(t, ops, "DeprecateDomain")
	assert.Contains(t, ops, "RegisterWorkflowType")
	assert.Contains(t, ops, "ListWorkflowTypes")
	assert.Contains(t, ops, "StartWorkflowExecution")
	assert.Contains(t, ops, "DescribeWorkflowExecution")
}

// TestHandler_OpsLen verifies GetSupportedOperations count.
func TestHandler_OpsLen(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	h := swf.NewHandler(b)
	assert.Len(t, h.GetSupportedOperations(), 39)
}

// TestHandler_GetSupportedOperationsSorted verifies GetSupportedOperations is sorted.
func TestHandler_GetSupportedOperationsSorted(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	h := swf.NewHandler(b)
	ops := h.GetSupportedOperations()

	require.NotEmpty(t, ops)

	for i := 1; i < len(ops); i++ {
		assert.LessOrEqual(t, ops[i-1], ops[i],
			"ops not sorted at index %d: %s > %s", i, ops[i-1], ops[i])
	}
}

// TestHandler_Reset verifies Handler.Reset() delegates to the backend.
func TestHandler_Reset(t *testing.T) {
	t.Parallel()

	b := swf.NewInMemoryBackend()
	require.NoError(t, b.RegisterDomain("d1", "", "NONE"))
	h := swf.NewHandler(b)

	h.Reset()

	assert.Zero(t, swf.DomainCount(b))
}

func TestSWFHandler_MatchPriority(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)
	assert.Equal(t, 100, h.MatchPriority())
}

func TestSWFHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		wantOp string
	}{
		{
			name:   "WithTarget",
			target: "SimpleWorkflowService.RegisterDomain",
			wantOp: "RegisterDomain",
		},
		{
			name:   "NoTarget",
			target: "",
			wantOp: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSWFHandler(t)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}
			c := e.NewContext(req, httptest.NewRecorder())

			assert.Equal(t, tt.wantOp, h.ExtractOperation(c))
		})
	}
}

func TestSWFHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantResource string
	}{
		{
			name:         "NameField",
			body:         `{"name":"my-domain"}`,
			wantResource: "my-domain",
		},
		{
			name:         "DomainFallback",
			body:         `{"domain":"test-domain"}`,
			wantResource: "test-domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSWFHandler(t)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			c := e.NewContext(req, httptest.NewRecorder())

			assert.Equal(t, tt.wantResource, h.ExtractResource(c))
		})
	}
}

func TestSWFProvider(t *testing.T) {
	t.Parallel()

	t.Run("Name", func(t *testing.T) {
		t.Parallel()

		p := &swf.Provider{}
		assert.Equal(t, "SWF", p.Name())
	})

	t.Run("Init", func(t *testing.T) {
		t.Parallel()

		p := &swf.Provider{}
		ctx := &service.AppContext{Logger: slog.Default()}
		svc, err := p.Init(ctx)
		require.NoError(t, err)
		assert.NotNil(t, svc)
		assert.Equal(t, "SWF", svc.Name())
	})

	t.Run("InitWithJanitorCtx", func(t *testing.T) {
		t.Parallel()

		p := &swf.Provider{}
		reg, err := p.Init(&service.AppContext{JanitorCtx: t.Context()})
		require.NoError(t, err)
		assert.NotNil(t, reg)
	})

	t.Run("NilAppContext", func(t *testing.T) {
		t.Parallel()

		p := &swf.Provider{}
		_, err := p.Init(nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, swf.ErrNilAppContext)
	})
}

// TestSWFHandler_ErrorTypes verifies that typed SWF faults include __type in the JSON response
// so that the AWS SDK v2 can deserialize them to the correct error types.
func TestSWFHandler_ErrorTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		action   string
		wantType string
		setup    []setupAction
		wantCode int
	}{
		{
			name:   "DomainAlreadyExistsFault",
			action: "RegisterDomain",
			setup: []setupAction{
				{action: "RegisterDomain", body: map[string]any{"name": "dup-domain"}},
			},
			body:     map[string]any{"name": "dup-domain"},
			wantCode: http.StatusBadRequest,
			wantType: "DomainAlreadyExistsFault",
		},
		{
			// Per aws-sdk-go-v2's DomainAlreadyExistsFault doc comment, "You may
			// get this fault if you are registering a domain that is either
			// already registered or deprecated" -- RegisterDomain's modelled
			// error set has no DomainDeprecatedFault at all, so re-registering a
			// deprecated domain surfaces DomainAlreadyExistsFault, same as a
			// non-deprecated duplicate.
			name:   "RegisterDomain_OnDeprecatedDomain_ReturnsAlreadyExistsFault",
			action: "RegisterDomain",
			setup: []setupAction{
				{action: "RegisterDomain", body: map[string]any{"name": "dep-domain"}},
				{action: "DeprecateDomain", body: map[string]any{"name": "dep-domain"}},
			},
			body:     map[string]any{"name": "dep-domain"},
			wantCode: http.StatusBadRequest,
			wantType: "DomainAlreadyExistsFault",
		},
		{
			name:   "TypeAlreadyExistsFault",
			action: "RegisterWorkflowType",
			setup: []setupAction{
				{action: "RegisterDomain", body: map[string]any{"name": "d1"}},
				{action: "RegisterWorkflowType", body: map[string]any{"domain": "d1", "name": "wf1", "version": "1.0"}},
			},
			body:     map[string]any{"domain": "d1", "name": "wf1", "version": "1.0"},
			wantCode: http.StatusBadRequest,
			wantType: "TypeAlreadyExistsFault",
		},
		{
			name:     "UnknownResourceFault",
			action:   "DeprecateDomain",
			body:     map[string]any{"name": "nonexistent"},
			wantCode: http.StatusNotFound,
			wantType: "UnknownResourceFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSWFHandler(t)

			for _, s := range tt.setup {
				doSWFRequest(t, h, s.action, s.body)
			}

			rec := doSWFRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantType, resp["__type"])
		})
	}
}
