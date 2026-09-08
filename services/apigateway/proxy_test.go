package apigateway_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigateway"
)

var (
	errFunctionError = errors.New("function error")
	errLambdaError   = errors.New("lambda error")
)

// proxyMockInvoker records the payload sent to InvokeFunction.
type proxyMockInvoker struct {
	returnError error
	capturedFn  string
	response    []byte
	statusCode  int
}

func (m *proxyMockInvoker) InvokeFunction(_ context.Context, fn, _ string, _ []byte) ([]byte, int, error) {
	m.capturedFn = fn
	if m.returnError != nil {
		return nil, http.StatusInternalServerError, m.returnError
	}

	if m.response != nil {
		return m.response, m.statusCode, nil
	}

	return []byte(`{"statusCode":200,"body":"ok","headers":{}}`), http.StatusOK, nil
}

// captureInvoker records the last payload sent to InvokeFunction.
type captureInvoker struct {
	capture *[]byte
}

func (c *captureInvoker) InvokeFunction(_ context.Context, _, _ string, payload []byte) ([]byte, int, error) {
	*c.capture = payload

	return payload, http.StatusOK, nil
}

// setupProxyAPIViaHandler creates a full API setup using HTTP handler calls.
// Returns (handler, echoEngine, apiID).
func setupProxyAPIViaHandler(
	t *testing.T,
	integrationType, uri string,
) (*apigateway.Handler, *echo.Echo, string) {
	t.Helper()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	// Create REST API.
	createRec := postWithHandler(t, h, e, "CreateRestApi", `{"name":"proxy-api","description":"test"}`)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	apiID := createResp["id"].(string)

	// Get root resource.
	listRec := postWithHandler(t, h, e, "GetResources", `{"restApiId":"`+apiID+`"}`)
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	rootID := listResp["item"].([]any)[0].(map[string]any)["id"].(string)

	// Create child resource.
	childRec := postWithHandler(t, h, e, "CreateResource",
		`{"restApiId":"`+apiID+`","parentId":"`+rootID+`","pathPart":"items"}`)
	require.Equal(t, http.StatusCreated, childRec.Code)

	var childResp map[string]any
	require.NoError(t, json.Unmarshal(childRec.Body.Bytes(), &childResp))
	childID := childResp["id"].(string)

	// PutMethod.
	methodRec := postWithHandler(t, h, e, "PutMethod",
		`{"restApiId":"`+apiID+`","resourceId":"`+childID+`","httpMethod":"POST","authorizationType":"NONE"}`)
	require.Equal(t, http.StatusCreated, methodRec.Code)

	// PutIntegration.
	integBody := `{"restApiId":"` + apiID + `","resourceId":"` + childID + `","httpMethod":"POST","type":"` +
		integrationType + `","uri":"` + uri + `"}`
	integRec := postWithHandler(t, h, e, "PutIntegration", integBody)
	require.Equal(t, http.StatusCreated, integRec.Code)

	// CreateDeployment.
	deplRec := postWithHandler(t, h, e, "CreateDeployment",
		`{"restApiId":"`+apiID+`","stageName":"prod","description":"v1"}`)
	require.Equal(t, http.StatusCreated, deplRec.Code)

	return h, e, apiID
}

const testStageName = "prod"

// proxyReq makes a POST request via the /proxy/{apiId}/prod/{path} endpoint.
func proxyReq(
	t *testing.T,
	h *apigateway.Handler,
	e *echo.Echo,
	apiID, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	url := "/proxy/" + apiID + "/" + testStageName + path
	var req *http.Request

	if body != "" {
		req = httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(http.MethodPost, url, nil)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestHandleAWSProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupInvoker func(*apigateway.Handler)
		name         string
		path         string
		body         string
		wantBody     string
		wantStatus   int
	}{
		{
			name: "success",
			path: "/items",
			body: `{"key":"val"}`,
			setupInvoker: func(h *apigateway.Handler) {
				h.SetLambdaInvoker(&proxyMockInvoker{})
			},
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name: "lambda_error",
			path: "/items",
			body: `{}`,
			setupInvoker: func(h *apigateway.Handler) {
				h.SetLambdaInvoker(&proxyMockInvoker{returnError: errFunctionError})
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:         "no_lambda_invoker",
			path:         "/items",
			body:         `{}`,
			setupInvoker: nil,
			wantStatus:   http.StatusServiceUnavailable,
		},
		{
			// Unmatched resource path on a deployed stage: AWS returns 403
			// "Missing Authentication Token", not 404.
			name: "not_found",
			path: "/unknown/path",
			body: `{}`,
			setupInvoker: func(h *apigateway.Handler) {
				h.SetLambdaInvoker(&proxyMockInvoker{})
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "base64_response",
			path: "/items",
			body: `{}`,
			setupInvoker: func(h *apigateway.Handler) {
				h.SetLambdaInvoker(&proxyMockInvoker{
					response: []byte(`{"statusCode":200,"body":"aGVsbG8=","isBase64Encoded":true}`),
				})
			},
			wantStatus: http.StatusOK,
			wantBody:   "hello",
		},
		{
			name: "non_proxy_response",
			path: "/items",
			body: `{}`,
			setupInvoker: func(h *apigateway.Handler) {
				h.SetLambdaInvoker(&proxyMockInvoker{response: []byte(`not json`)})
			},
			wantStatus: http.StatusOK,
			wantBody:   "not json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e, apiID := setupProxyAPIViaHandler(t, "AWS_PROXY", "arn:aws:lambda:us-east-1:123:function:myFn")
			if tt.setupInvoker != nil {
				tt.setupInvoker(h)
			}

			rec := proxyReq(t, h, e, apiID, tt.path, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, rec.Body.String())
			}
		})
	}
}

// setupVTLAPI creates an API with a /transform resource that has a requestTemplates integration.
func setupVTLAPI(t *testing.T) (*apigateway.Handler, *echo.Echo, string) {
	t.Helper()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	createRec := postWithHandler(t, h, e, "CreateRestApi", `{"name":"vtl-api","description":"test"}`)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	apiID := createResp["id"].(string)

	listRec := postWithHandler(t, h, e, "GetResources", `{"restApiId":"`+apiID+`"}`)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	rootID := listResp["item"].([]any)[0].(map[string]any)["id"].(string)

	childRec := postWithHandler(t, h, e, "CreateResource",
		`{"restApiId":"`+apiID+`","parentId":"`+rootID+`","pathPart":"transform"}`)
	var childResp map[string]any
	require.NoError(t, json.Unmarshal(childRec.Body.Bytes(), &childResp))
	childID := childResp["id"].(string)

	postWithHandler(t, h, e, "PutMethod",
		`{"restApiId":"`+apiID+`","resourceId":"`+childID+`","httpMethod":"POST","authorizationType":"NONE"}`)

	integBody := `{
		"restApiId":"` + apiID + `",
		"resourceId":"` + childID + `",
		"httpMethod":"POST",
		"type":"AWS",
		"uri":"arn:aws:lambda:us-east-1:123:function:fn",
		"requestTemplates":{"application/json":"{\"name\":$input.json('$.user')}"}
	}`
	integRec := postWithHandler(t, h, e, "PutIntegration", integBody)
	require.Equal(t, http.StatusCreated, integRec.Code)

	postWithHandler(t, h, e, "CreateDeployment",
		`{"restApiId":"`+apiID+`","stageName":"prod","description":"v1"}`)

	return h, e, apiID
}

func TestHandleAWSIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupFn      func(t *testing.T) (*apigateway.Handler, *echo.Echo, string)
		invokerFn    func(capture *[]byte) apigateway.LambdaInvoker
		checkCapture func(t *testing.T, captured []byte)
		name         string
		path         string
		body         string
		wantStatus   int
	}{
		{
			name: "with_request_template",
			setupFn: func(t *testing.T) (*apigateway.Handler, *echo.Echo, string) {
				t.Helper()

				return setupVTLAPI(t)
			},
			invokerFn: func(capture *[]byte) apigateway.LambdaInvoker {
				return &captureInvoker{capture: capture}
			},
			path:       "/transform",
			body:       `{"user":"alice"}`,
			wantStatus: http.StatusOK,
			checkCapture: func(t *testing.T, captured []byte) {
				t.Helper()
				var got map[string]any
				require.NoError(t, json.Unmarshal(captured, &got))
				assert.Equal(t, "alice", got["name"])
			},
		},
		{
			name: "lambda_error",
			setupFn: func(t *testing.T) (*apigateway.Handler, *echo.Echo, string) {
				t.Helper()

				return setupProxyAPIViaHandler(t, "AWS", "arn:aws:lambda:us-east-1:123:function:fn")
			},
			invokerFn: func(_ *[]byte) apigateway.LambdaInvoker {
				return &proxyMockInvoker{returnError: errLambdaError}
			},
			path:       "/items",
			body:       `{}`,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "no_request_template",
			setupFn: func(t *testing.T) (*apigateway.Handler, *echo.Echo, string) {
				t.Helper()

				return setupProxyAPIViaHandler(t, "AWS", "arn:aws:lambda:us-east-1:123:function:fn")
			},
			invokerFn: func(capture *[]byte) apigateway.LambdaInvoker {
				return &captureInvoker{capture: capture}
			},
			path:       "/items",
			body:       `{"data":"test"}`,
			wantStatus: http.StatusOK,
			checkCapture: func(t *testing.T, captured []byte) {
				t.Helper()
				assert.JSONEq(t, `{"data":"test"}`, string(captured))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e, apiID := tt.setupFn(t)

			var capturedPayload []byte
			h.SetLambdaInvoker(tt.invokerFn(&capturedPayload))

			rec := proxyReq(t, h, e, apiID, tt.path, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.checkCapture != nil {
				tt.checkCapture(t, capturedPayload)
			}
		})
	}
}

func TestHandleProxy_UnsupportedIntegrationType(t *testing.T) {
	t.Parallel()

	t.Run("unknown_type_not_implemented", func(t *testing.T) {
		t.Parallel()

		// "UNKNOWN_CUSTOM" is rejected by the handler (real AWS also rejects it), so we
		// set up the integration directly via the backend to test proxy runtime behavior.
		backend := apigateway.NewInMemoryBackend()
		h := apigateway.NewHandler(backend)
		e := echo.New()

		api, err := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "proxy-api"})
		require.NoError(t, err)
		apiID := api.ID

		resources, _, err := backend.GetResources(apiID, "", 0)
		require.NoError(t, err)
		rootID := resources[0].ID

		childRes, err := backend.CreateResource(apiID, rootID, "items")
		require.NoError(t, err)

		_, err = backend.PutMethod(apigateway.PutMethodInput{
			RestAPIID:         apiID,
			ResourceID:        childRes.ID,
			HTTPMethod:        "POST",
			AuthorizationType: "NONE",
		})
		require.NoError(t, err)

		_, err = backend.PutIntegration(apiID, childRes.ID, "POST", apigateway.PutIntegrationInput{
			Type: "UNKNOWN_CUSTOM",
		})
		require.NoError(t, err)

		_, err = backend.CreateDeployment(apiID, "prod", "v1")
		require.NoError(t, err)

		h.SetLambdaInvoker(&proxyMockInvoker{})

		rec := proxyReq(t, h, e, apiID, "/items", `{}`)
		assert.Equal(t, http.StatusNotImplemented, rec.Code)
	})
}

func TestHandleStageProxy_InvalidPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		url        string
		wantStatus int
	}{
		{
			name:       "single_path_segment",
			url:        "/proxy/onlyonepart",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := apigateway.NewInMemoryBackend()
			h := apigateway.NewHandler(backend)
			e := echo.New()

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// setupProxyAPIWithResource creates a handler, Echo instance, and API with a specific resource path.
// Each segment of resourcePath is created as a nested resource under root.
func setupProxyAPIWithResource(
	t *testing.T,
	resourcePath, integrationType, uri string,
) (*apigateway.Handler, *echo.Echo, string) {
	t.Helper()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	// Create REST API.
	createRec := postWithHandler(t, h, e, "CreateRestApi", `{"name":"path-api","description":"test"}`)
	require.Equal(t, http.StatusCreated, createRec.Code)
	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	apiID := createResp["id"].(string)

	// Get root resource ID by finding the resource with path "/".
	listRec := postWithHandler(t, h, e, "GetResources", `{"restApiId":"`+apiID+`"}`)
	require.Equal(t, http.StatusOK, listRec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))

	items, _ := listResp["item"].([]any)
	var parentID string

	for _, item := range items {
		res, ok := item.(map[string]any)
		if !ok {
			continue
		}

		if path, _ := res["path"].(string); path == "/" {
			parentID, _ = res["id"].(string)

			break
		}
	}

	require.NotEmpty(t, parentID, "root resource with path '/' not found")

	// Create each path segment as a nested resource.
	if resourcePath != "" {
		parts := strings.SplitSeq(strings.Trim(resourcePath, "/"), "/")
		for part := range parts {
			r := postWithHandler(t, h, e, "CreateResource",
				`{"restApiId":"`+apiID+`","parentId":"`+parentID+`","pathPart":"`+part+`"}`)
			require.Equal(t, http.StatusCreated, r.Code)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(r.Body.Bytes(), &resp))
			parentID = resp["id"].(string)
		}
	}

	childID := parentID

	// PutMethod.
	methodRec := postWithHandler(t, h, e, "PutMethod",
		`{"restApiId":"`+apiID+`","resourceId":"`+childID+`","httpMethod":"GET","authorizationType":"NONE"}`)
	require.Equal(t, http.StatusCreated, methodRec.Code)

	// PutIntegration.
	integBody := `{"restApiId":"` + apiID + `","resourceId":"` + childID + `","httpMethod":"GET","type":"` +
		integrationType + `","uri":"` + uri + `"}`
	integRec := postWithHandler(t, h, e, "PutIntegration", integBody)
	require.Equal(t, http.StatusCreated, integRec.Code)

	// CreateDeployment.
	deployRec := postWithHandler(t, h, e, "CreateDeployment",
		`{"restApiId":"`+apiID+`","stageName":"prod","description":"v1"}`)
	require.Equal(t, http.StatusCreated, deployRec.Code)

	return h, e, apiID
}

// userReq sends a GET request via the /restapis/{apiId}/prod/_user_request_/{path} endpoint.
func userReq(
	t *testing.T,
	h *apigateway.Handler,
	e *echo.Echo,
	apiID, path string,
) *httptest.ResponseRecorder {
	t.Helper()

	url := "/restapis/" + apiID + "/prod/_user_request_" + path
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestUserRequestEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupInvoker func(*apigateway.Handler)
		name         string
		resourcePath string
		requestPath  string
		wantStatus   int
	}{
		{
			name:         "exact_match",
			resourcePath: "items",
			requestPath:  "/items",
			setupInvoker: func(h *apigateway.Handler) {
				h.SetLambdaInvoker(&proxyMockInvoker{})
			},
			wantStatus: http.StatusOK,
		},
		{
			// Unmatched resource path on a deployed stage: AWS returns 403
			// "Missing Authentication Token", not 404.
			name:         "not_found",
			resourcePath: "items",
			requestPath:  "/unknown",
			setupInvoker: func(h *apigateway.Handler) {
				h.SetLambdaInvoker(&proxyMockInvoker{})
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name:         "no_lambda_invoker",
			resourcePath: "items",
			requestPath:  "/items",
			wantStatus:   http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e, apiID := setupProxyAPIWithResource(t, tt.resourcePath, "AWS_PROXY", "fn")
			if tt.setupInvoker != nil {
				tt.setupInvoker(h)
			}

			rec := userReq(t, h, e, apiID, tt.requestPath)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestPathVariableMatching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		checkEvent   func(t *testing.T, payload []byte)
		name         string
		resourcePath string
		requestPath  string
		wantStatus   int
	}{
		{
			name:         "single_param",
			resourcePath: "items/{id}",
			requestPath:  "/items/42",
			wantStatus:   http.StatusOK,
			checkEvent: func(t *testing.T, payload []byte) {
				t.Helper()
				var event map[string]any
				require.NoError(t, json.Unmarshal(payload, &event))
				params, _ := event["pathParameters"].(map[string]any)
				assert.Equal(t, "42", params["id"])
			},
		},
		{
			name:         "greedy_param",
			resourcePath: "{proxy+}",
			requestPath:  "/a/b/c",
			wantStatus:   http.StatusOK,
			checkEvent: func(t *testing.T, payload []byte) {
				t.Helper()
				var event map[string]any
				require.NoError(t, json.Unmarshal(payload, &event))
				params, _ := event["pathParameters"].(map[string]any)
				assert.Equal(t, "/a/b/c", params["proxy"])
			},
		},
		{
			// Unmatched resource path on a deployed stage: AWS returns 403
			// "Missing Authentication Token", not 404.
			name:         "param_no_match_wrong_depth",
			resourcePath: "items/{id}/details",
			requestPath:  "/items/42",
			wantStatus:   http.StatusForbidden,
		},
		{
			name:         "exact_match_resource",
			resourcePath: "items/special",
			requestPath:  "/items/special",
			wantStatus:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var captured []byte
			h, e, apiID := setupProxyAPIWithResource(t, tt.resourcePath, "AWS_PROXY", "fn")
			h.SetLambdaInvoker(&captureInvoker{capture: &captured})

			rec := userReq(t, h, e, apiID, tt.requestPath)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.checkEvent != nil {
				tt.checkEvent(t, captured)
			}
		})
	}
}

func TestMockIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "default_200",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e, apiID := setupProxyAPIWithResource(t, "items", "MOCK", "")
			rec := userReq(t, h, e, apiID, "/items")
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHTTPProxyIntegration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		intType    string
		serverResp string
		wantBody   string
		serverCode int
		wantStatus int
	}{
		{
			name:       "http_proxy_success",
			intType:    "HTTP_PROXY",
			serverResp: `{"result":"upstream"}`,
			serverCode: http.StatusOK,
			wantStatus: http.StatusOK,
			wantBody:   `{"result":"upstream"}`,
		},
		{
			name:       "http_proxy_upstream_not_found",
			intType:    "HTTP_PROXY",
			serverResp: `not found`,
			serverCode: http.StatusNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "http_integration_success",
			intType:    "HTTP",
			serverResp: `{"ok":true}`,
			serverCode: http.StatusOK,
			wantStatus: http.StatusOK,
			wantBody:   `{"ok":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Start a local upstream server.
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.serverCode)
				_, _ = w.Write([]byte(tt.serverResp))
			}))
			defer upstream.Close()

			h, e, apiID := setupProxyAPIWithResource(t, "items", tt.intType, upstream.URL+"/items")
			h.SetHTTPClient(upstream.Client())

			rec := userReq(t, h, e, apiID, "/items")
			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, rec.Body.String())
			}
		})
	}
}

func TestHTTPProxyIntegration_BadURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		uri        string
		wantStatus int
	}{
		{
			name:       "invalid_uri",
			uri:        "://invalid-uri",
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e, apiID := setupProxyAPIWithResource(t, "items", "HTTP_PROXY", tt.uri)
			rec := userReq(t, h, e, apiID, "/items")
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestExtractLambdaFunctionName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		uri    string
		wantFn string
	}{
		{
			name:   "plain_name",
			uri:    "my-function",
			wantFn: "my-function",
		},
		{
			name:   "lambda_arn",
			uri:    "arn:aws:lambda:us-east-1:123456789012:function:my-function",
			wantFn: "my-function",
		},
		{
			name:   "lambda_arn_with_qualifier",
			uri:    "arn:aws:lambda:us-east-1:123456789012:function:my-function:prod",
			wantFn: "my-function:prod",
		},
		{
			name: "apigateway_invoke_uri",
			uri: "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/" +
				"arn:aws:lambda:us-east-1:123456789012:function:my-function/invocations",
			wantFn: "my-function",
		},
		{
			name: "apigateway_invoke_uri_with_qualifier",
			uri: "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/" +
				"arn:aws:lambda:us-east-1:123456789012:function:my-function:prod/invocations",
			wantFn: "my-function:prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := apigateway.ExtractLambdaFunctionName(tt.uri)
			assert.Equal(t, tt.wantFn, got)
		})
	}
}

// TestHandleProxyRequest_RequiresDeployedStage verifies that AWS's real contract --
// a request only routes to an integration when its URL names a stage that a real
// CreateDeployment actually produced -- is enforced. AWS returns 403 "Missing
// Authentication Token" (not 404, and not a silent 200) for an invalid/undeployed
// stage; before this fix gopherstack routed to the RestApi's live, current resource
// tree regardless of whether any deployment had ever been created or whether the
// stage name in the URL matched a real stage.
func TestHandleProxyRequest_RequiresDeployedStage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stageName  string
		wantStatus int
		deploy     bool
	}{
		{
			name:       "never_deployed",
			deploy:     false,
			stageName:  "prod",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "deployed_but_wrong_stage_in_url",
			deploy:     true,
			stageName:  "totally-made-up-stage",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "deployed_correct_stage",
			deploy:     true,
			stageName:  testStageName,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := apigateway.NewInMemoryBackend()
			h := apigateway.NewHandler(backend)
			e := echo.New()
			h.SetLambdaInvoker(&proxyMockInvoker{})

			createRec := postWithHandler(t, h, e, "CreateRestApi", `{"name":"gate-api"}`)
			require.Equal(t, http.StatusCreated, createRec.Code)
			var createResp map[string]any
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
			apiID := createResp["id"].(string)

			listRec := postWithHandler(t, h, e, "GetResources", `{"restApiId":"`+apiID+`"}`)
			require.Equal(t, http.StatusOK, listRec.Code)
			var listResp map[string]any
			require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
			rootID := listResp["item"].([]any)[0].(map[string]any)["id"].(string)

			childRec := postWithHandler(t, h, e, "CreateResource",
				`{"restApiId":"`+apiID+`","parentId":"`+rootID+`","pathPart":"items"}`)
			require.Equal(t, http.StatusCreated, childRec.Code)
			var childResp map[string]any
			require.NoError(t, json.Unmarshal(childRec.Body.Bytes(), &childResp))
			childID := childResp["id"].(string)

			methodRec := postWithHandler(t, h, e, "PutMethod",
				`{"restApiId":"`+apiID+`","resourceId":"`+childID+`","httpMethod":"GET","authorizationType":"NONE"}`)
			require.Equal(t, http.StatusCreated, methodRec.Code)

			integRec := postWithHandler(t, h, e, "PutIntegration",
				`{"restApiId":"`+apiID+`","resourceId":"`+childID+`","httpMethod":"GET","type":"MOCK"}`)
			require.Equal(t, http.StatusCreated, integRec.Code)

			integRespRec := postWithHandler(t, h, e, "PutIntegrationResponse",
				`{"restApiId":"`+apiID+`","resourceId":"`+childID+`","httpMethod":"GET","statusCode":"200"}`)
			require.Equal(t, http.StatusCreated, integRespRec.Code)

			if tt.deploy {
				deplRec := postWithHandler(t, h, e, "CreateDeployment",
					`{"restApiId":"`+apiID+`","stageName":"`+testStageName+`"}`)
				require.Equal(t, http.StatusCreated, deplRec.Code)
			}

			req := httptest.NewRequest(http.MethodGet, "/proxy/"+apiID+"/"+tt.stageName+"/items", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			require.NoError(t, h.Handler()(c))

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusForbidden {
				assert.Equal(t, "MissingAuthenticationTokenException", rec.Header().Get("X-Amzn-Errortype"))
				assert.Contains(t, rec.Body.String(), "Missing Authentication Token")
			}
		})
	}
}
