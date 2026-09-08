package apigateway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigateway"
	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

// captureAuthInvoker records the payload and returns a configurable response.
type captureAuthInvoker struct {
	returnError error
	capturedFn  string
	response    []byte
}

func (m *captureAuthInvoker) InvokeFunction(_ context.Context, fn, _ string, _ []byte) ([]byte, int, error) {
	m.capturedFn = fn
	if m.returnError != nil {
		return nil, http.StatusInternalServerError, m.returnError
	}

	if m.response != nil {
		return m.response, http.StatusOK, nil
	}

	return []byte(`{}`), http.StatusOK, nil
}

// setupAuthorizerAPI creates an API with a /secure resource associated with an authorizer.
// Returns (handler, echoEngine, apiID).
func setupAuthorizerAPI(
	t *testing.T,
	authType string,
) (*apigateway.Handler, *echo.Echo, string) {
	t.Helper()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	// Create REST API.
	createRec := postWithHandler(t, h, e, "CreateRestApi", `{"name":"auth-api","description":"test"}`)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	apiID := createResp["id"].(string)

	// Get root resource.
	listRec := postWithHandler(t, h, e, "GetResources", `{"restApiId":"`+apiID+`"}`)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	rootID := listResp["item"].([]any)[0].(map[string]any)["id"].(string)

	// Create /secure resource.
	childRec := postWithHandler(t, h, e, "CreateResource",
		`{"restApiId":"`+apiID+`","parentId":"`+rootID+`","pathPart":"secure"}`)
	require.Equal(t, http.StatusCreated, childRec.Code)

	var childResp map[string]any
	require.NoError(t, json.Unmarshal(childRec.Body.Bytes(), &childResp))
	childID := childResp["id"].(string)

	// Create authorizer.
	authBody := `{
"restApiId":"` + apiID + `",
"name":"test-auth",
"type":"` + authType + `",
"identitySource":"method.request.header.Authorization",
"authorizerUri":"arn:aws:lambda:us-east-1:123:function:authFn"
}`
	authRec := postWithHandler(t, h, e, "CreateAuthorizer", authBody)
	require.Equal(t, http.StatusCreated, authRec.Code)

	var authResp map[string]any
	require.NoError(t, json.Unmarshal(authRec.Body.Bytes(), &authResp))
	authID := authResp["id"].(string)

	// PutMethod with authorizerId.
	methodBody := `{
"restApiId":"` + apiID + `",
"resourceId":"` + childID + `",
"httpMethod":"GET",
"authorizationType":"CUSTOM",
"authorizerId":"` + authID + `"
}`
	methodRec := postWithHandler(t, h, e, "PutMethod", methodBody)
	require.Equal(t, http.StatusCreated, methodRec.Code)

	// PutIntegration (MOCK so we can test the authorizer in isolation).
	integBody := `{
"restApiId":"` + apiID + `",
"resourceId":"` + childID + `",
"httpMethod":"GET",
"type":"MOCK"
}`
	integRec := postWithHandler(t, h, e, "PutIntegration", integBody)
	require.Equal(t, http.StatusCreated, integRec.Code)

	// CreateDeployment.
	deplRec := postWithHandler(t, h, e, "CreateDeployment",
		`{"restApiId":"`+apiID+`","stageName":"prod","description":"v1"}`)
	require.Equal(t, http.StatusCreated, deplRec.Code)

	return h, e, apiID
}

// allowPolicy returns a standard "Allow" IAM policy response from a Lambda authorizer.
func allowPolicy() []byte {
	return []byte(`{"principalId":"u","policyDocument":{"Statement":[` +
		`{"Effect":"Allow","Action":"execute-api:Invoke","Resource":"arn:*"}]}}`)
}

// denyPolicy returns a standard "Deny" IAM policy response from a Lambda authorizer.
func denyPolicy() []byte {
	return []byte(`{"principalId":"u","policyDocument":{"Statement":[` +
		`{"Effect":"Deny","Action":"execute-api:Invoke","Resource":"arn:*"}]}}`)
}

// authReq sends a GET request with an optional Authorization header to /restapis/.../prod/_user_request_/secure.
func authReq(
	t *testing.T,
	h *apigateway.Handler,
	e *echo.Echo,
	apiID, token string,
) *httptest.ResponseRecorder {
	t.Helper()

	url := "/restapis/" + apiID + "/prod/_user_request_/secure"
	req := httptest.NewRequest(http.MethodGet, url, nil)

	if token != "" {
		req.Header.Set("Authorization", token)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestRunAuthorizer_TOKEN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		invokerErr  error
		name        string
		invokerResp []byte
		wantStatus  int
		setInvoker  bool
	}{
		{
			name:        "allow_returns_200",
			invokerResp: allowPolicy(),
			setInvoker:  true,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "deny_returns_403",
			invokerResp: denyPolicy(),
			setInvoker:  true,
			wantStatus:  http.StatusForbidden,
		},
		{
			name:       "no_lambda_invoker_returns_503",
			setInvoker: false,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "invocation_failure_returns_401",
			invokerErr: errLambdaError,
			setInvoker: true,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:        "malformed_response_returns_401",
			invokerResp: []byte("not json"),
			setInvoker:  true,
			wantStatus:  http.StatusUnauthorized,
		},
		{
			name:        "empty_policy_document_returns_403",
			invokerResp: []byte(`{"principalId":"u","policyDocument":{"Statement":[]}}`),
			setInvoker:  true,
			wantStatus:  http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e, apiID := setupAuthorizerAPI(t, "TOKEN")

			if tt.setInvoker {
				h.SetLambdaInvoker(&captureAuthInvoker{
					response:    tt.invokerResp,
					returnError: tt.invokerErr,
				})
			}

			rec := authReq(t, h, e, apiID, "Bearer test-token")
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestRunAuthorizer_REQUEST(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		invokerResp []byte
		wantStatus  int
	}{
		{
			name:        "allow_returns_200",
			invokerResp: allowPolicy(),
			wantStatus:  http.StatusOK,
		},
		{
			name:        "deny_returns_403",
			invokerResp: denyPolicy(),
			wantStatus:  http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e, apiID := setupAuthorizerAPI(t, "REQUEST")
			h.SetLambdaInvoker(&captureAuthInvoker{response: tt.invokerResp})

			rec := authReq(t, h, e, apiID, "Bearer test-token")
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestRunAuthorizer_MethodArn_ResourcePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "method_arn_uses_stripped_resource_path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var capturedPayload []byte

			// Use a captureInvoker to record the authorizer event payload, but return allow.
			capture := &captureAuthInvokerWithCapture{
				capturedPayload: &capturedPayload,
				response:        allowPolicy(),
			}

			h, e, apiID := setupAuthorizerAPI(t, "TOKEN")
			h.SetLambdaInvoker(capture)

			rec := authReq(t, h, e, apiID, "Bearer token")
			assert.Equal(t, http.StatusOK, rec.Code)

			// Verify the methodArn in the captured event uses the stripped resource path ("/secure"),
			// not the full proxy path ("/restapis/.../prod/_user_request_/secure").
			var event map[string]any
			require.NoError(t, json.Unmarshal(capturedPayload, &event))
			methodArn, _ := event["methodArn"].(string)
			assert.Contains(t, methodArn, "/secure", "methodArn should contain the stripped path")
			assert.NotContains(t, methodArn, "_user_request_", "methodArn must not contain internal proxy prefix")
		})
	}
}

func TestRunAuthorizer_Cache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "second_request_uses_cache"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			invoker := &captureAuthInvoker{response: allowPolicy()}
			h, e, apiID := setupAuthorizerAPI(t, "TOKEN")

			calls := 0
			h.SetLambdaInvoker(&trackingInvoker{inner: invoker, calls: &calls})

			// First request - should call Lambda.
			rec1 := authReq(t, h, e, apiID, "Bearer cached-token")
			assert.Equal(t, http.StatusOK, rec1.Code)
			assert.Equal(t, 1, calls, "first request should call Lambda once")

			// Second request with same token - should hit cache.
			rec2 := authReq(t, h, e, apiID, "Bearer cached-token")
			assert.Equal(t, http.StatusOK, rec2.Code)
			assert.Equal(t, 1, calls, "second request should use cache, not call Lambda again")
		})
	}
}

// trackingInvoker wraps an invoker and counts calls.
type trackingInvoker struct {
	inner apigateway.LambdaInvoker
	calls *int
}

func (t *trackingInvoker) InvokeFunction(ctx context.Context, fn, invType string, payload []byte) ([]byte, int, error) {
	*t.calls++

	return t.inner.InvokeFunction(ctx, fn, invType, payload)
}

// captureAuthInvokerWithCapture records the event payload and returns a configurable response.
type captureAuthInvokerWithCapture struct {
	capturedPayload *[]byte
	response        []byte
}

func (m *captureAuthInvokerWithCapture) InvokeFunction(
	_ context.Context,
	_, _ string,
	payload []byte,
) ([]byte, int, error) {
	if m.capturedPayload != nil {
		*m.capturedPayload = make([]byte, len(payload))
		copy(*m.capturedPayload, payload)
	}

	return m.response, http.StatusOK, nil
}

// --- Integration request parameter mapping tests ---

// httpCapture is a test HTTP server that records the last received request headers and query.
type httpCapture struct {
	lastHeader http.Header
	server     *httptest.Server
	lastQuery  string
}

func newHTTPCapture() *httpCapture {
	c := &httpCapture{}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.lastHeader = r.Header.Clone()
		c.lastQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))

	return c
}

func TestProxy_IntegrationRequestParams_HeaderMapping(t *testing.T) {
	t.Parallel()

	capture := newHTTPCapture()
	defer capture.server.Close()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "req-param-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})

	// Map incoming Authorization header to X-Api-Caller header in integration.
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{
		Type:       "HTTP",
		HTTPMethod: "GET",
		URI:        capture.server.URL,
		RequestParameters: map[string]string{
			"integration.request.header.X-Api-Caller": "method.request.header.Authorization",
		},
	})
	_, _ = backend.CreateDeployment(api.ID, "prod", "v1")

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer tok123")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Bearer tok123", capture.lastHeader.Get("X-Api-Caller"))
}

func TestProxy_IntegrationRequestParams_QuerystringMapping(t *testing.T) {
	t.Parallel()

	capture := newHTTPCapture()
	defer capture.server.Close()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "qs-param-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})

	// Map incoming query param "userId" to integration query param "user_id".
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{
		Type:       "HTTP",
		HTTPMethod: "GET",
		URI:        capture.server.URL,
		RequestParameters: map[string]string{
			"integration.request.querystring.user_id": "method.request.querystring.userId",
		},
	})
	_, _ = backend.CreateDeployment(api.ID, "prod", "v1")

	url := "/restapis/" + api.ID + "/prod/_user_request_/?userId=42"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	// The integration should have received user_id=42.
	assert.Contains(t, capture.lastQuery, "user_id=42")
}

func TestProxy_IntegrationRequestParams_StaticValue(t *testing.T) {
	t.Parallel()

	capture := newHTTPCapture()
	defer capture.server.Close()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "static-param-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})

	// Static value for a header.
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{
		Type:       "HTTP",
		HTTPMethod: "GET",
		URI:        capture.server.URL,
		RequestParameters: map[string]string{
			"integration.request.header.X-Service-Name": "myservice",
		},
	})
	_, _ = backend.CreateDeployment(api.ID, "prod", "v1")

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "myservice", capture.lastHeader.Get("X-Service-Name"))
}

// --- Integration response parameter mapping tests ---

func TestProxy_IntegrationResponseParams_StaticHeader(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "resp-param-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{Type: "MOCK"})
	_, _ = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{
		ResponseParameters: map[string]string{
			"method.response.header.X-Custom-Header": "static-value",
		},
	})
	_, _ = backend.CreateDeployment(api.ID, "prod", "v1")

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "static-value", rec.Header().Get("X-Custom-Header"))
}

func TestProxy_IntegrationResponseParams_IntegrationHeaderEcho(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "echo-header-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{Type: "MOCK"})
	_, _ = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{
		ResponseParameters: map[string]string{
			"method.response.header.X-Request-Id": "integration.response.header.X-Amzn-Requestid",
		},
	})
	_, _ = backend.CreateDeployment(api.ID, "prod", "v1")

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	// The header should be set (resolved from integration.response.header.X-Amzn-Requestid → name).
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

// --- API Key enforcement tests ---

// setupAPIKeyRequired creates a minimal proxy setup where the method requires an API key.
func setupAPIKeyRequired(t *testing.T) (*apigateway.Handler, *echo.Echo, string) {
	t.Helper()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, err := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "key-api"})
	require.NoError(t, err)

	resources, _, err := backend.GetResources(api.ID, "", 0)
	require.NoError(t, err)
	rootID := resources[0].ID

	_, err = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
		APIKeyRequired:    true,
	})
	require.NoError(t, err)

	_, err = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{
		Type: "MOCK",
	})
	require.NoError(t, err)

	_, err = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{})
	require.NoError(t, err)

	_, err = backend.CreateDeployment(api.ID, "prod", "v1")
	require.NoError(t, err)

	return h, e, api.ID
}

func TestProxy_APIKeyRequired_MissingKey_Returns403(t *testing.T) {
	t.Parallel()

	h, e, apiID := setupAPIKeyRequired(t)

	url := "/restapis/" + apiID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestProxy_APIKeyRequired_InvalidKey_Returns403(t *testing.T) {
	t.Parallel()

	h, e, apiID := setupAPIKeyRequired(t)

	url := "/restapis/" + apiID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Api-Key", "definitely-not-a-real-key-value")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestProxy_APIKeyRequired_ValidKey_Passes(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "keypass-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
		APIKeyRequired:    true,
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{Type: "MOCK"})
	_, _ = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{})

	// Create an enabled API key.
	apiKey, err := backend.CreateAPIKey(apigateway.CreateAPIKeyInput{
		Name:    "test-key",
		Enabled: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, apiKey.Value)

	depl, _ := backend.CreateDeployment(api.ID, "prod", "v1")
	_, _ = backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Api-Key", apiKey.Value)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestProxy_APIKeyRequired_DisabledKey_Returns403(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "disabled-key-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
		APIKeyRequired:    true,
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{Type: "MOCK"})
	_, _ = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{})

	// Create a disabled API key.
	apiKey, err := backend.CreateAPIKey(apigateway.CreateAPIKeyInput{
		Name:    "disabled-key",
		Enabled: false,
	})
	require.NoError(t, err)

	depl, _ := backend.CreateDeployment(api.ID, "prod", "v1")
	_, _ = backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("X-Api-Key", apiKey.Value)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestProxy_APIKeyNotRequired_NoKey_Passes(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "no-key-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
		APIKeyRequired:    false,
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{Type: "MOCK"})
	_, _ = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{})

	depl, _ := backend.CreateDeployment(api.ID, "prod", "v1")
	_, _ = backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	// No key required → request passes.
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestProxy_APIKeyRequired_EnabledThenDisabled(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "toggle-key-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
		APIKeyRequired:    true,
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{Type: "MOCK"})
	_, _ = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{})

	apiKey, _ := backend.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "toggle-key", Enabled: true})
	depl, _ := backend.CreateDeployment(api.ID, "prod", "v1")
	_, _ = backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})

	makeReq := func() int {
		url := "/restapis/" + api.ID + "/prod/_user_request_/"
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("X-Api-Key", apiKey.Value)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		require.NoError(t, h.Handler()(c))

		return rec.Code
	}

	// First: key is enabled → should pass.
	assert.Equal(t, http.StatusOK, makeReq())

	// Disable the key.
	disabled := false
	_, err := backend.UpdateAPIKey(apiKey.ID, apigateway.UpdateAPIKeyInput{Enabled: &disabled})
	require.NoError(t, err)

	// Second: key is now disabled → should be forbidden.
	assert.Equal(t, http.StatusForbidden, makeReq())
}

// TestRunCognitoAuthorizer_ValidToken verifies that a JWT signed by the emulator's
// Cognito backend is accepted, while a tampered token is rejected.
func TestRunCognitoAuthorizer_ValidToken(t *testing.T) {
	t.Parallel()

	const endpoint = "http://localhost:8000"

	cognitoBk := cognitoidp.NewInMemoryBackend("000000000000", "us-east-1", endpoint)

	pool, err := cognitoBk.CreateUserPool("test-pool")
	require.NoError(t, err)

	client, err := cognitoBk.CreateUserPoolClient(pool.ID, "test-client")
	require.NoError(t, err)

	_, err = cognitoBk.SignUp(client.ClientID, "testuser", "Password1!", nil)
	require.NoError(t, err)

	require.NoError(t, cognitoBk.AdminConfirmSignUp(pool.ID, "testuser"))

	result, err := cognitoBk.InitiateAuth(client.ClientID, "USER_PASSWORD_AUTH", "testuser", "Password1!")
	require.NoError(t, err)
	idToken := result.Tokens.IDToken

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	h.SetJWKSProvider(cognitoBk)

	e := echo.New()

	createRec := postWithHandler(t, h, e, "CreateRestApi", `{"name":"cognito-api"}`)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	apiID := createResp["id"].(string)

	listRec := postWithHandler(t, h, e, "GetResources", `{"restApiId":"`+apiID+`"}`)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	rootID := listResp["item"].([]any)[0].(map[string]any)["id"].(string)

	childRec := postWithHandler(t, h, e, "CreateResource",
		`{"restApiId":"`+apiID+`","parentId":"`+rootID+`","pathPart":"secure"}`)
	require.Equal(t, http.StatusCreated, childRec.Code)

	var childResp map[string]any
	require.NoError(t, json.Unmarshal(childRec.Body.Bytes(), &childResp))
	childID := childResp["id"].(string)

	authBody := `{
"restApiId":"` + apiID + `",
"name":"cognito-auth",
"type":"COGNITO_USER_POOLS",
"identitySource":"method.request.header.Authorization"
}`
	authRec := postWithHandler(t, h, e, "CreateAuthorizer", authBody)
	require.Equal(t, http.StatusCreated, authRec.Code)

	var authResp map[string]any
	require.NoError(t, json.Unmarshal(authRec.Body.Bytes(), &authResp))
	authID := authResp["id"].(string)

	putMethodBody := `{"restApiId":"` + apiID + `","resourceId":"` + childID +
		`","httpMethod":"GET","authorizationType":"COGNITO_USER_POOLS","authorizerId":"` + authID + `"}`
	methodRec := postWithHandler(t, h, e, "PutMethod", putMethodBody)
	require.Equal(t, http.StatusCreated, methodRec.Code)

	integRec := postWithHandler(t, h, e, "PutIntegration",
		`{"restApiId":"`+apiID+`","resourceId":"`+childID+`","httpMethod":"GET","type":"MOCK"}`)
	require.Equal(t, http.StatusCreated, integRec.Code)

	deplRec := postWithHandler(t, h, e, "CreateDeployment",
		`{"restApiId":"`+apiID+`","stageName":"prod","description":"v1"}`)
	require.Equal(t, http.StatusCreated, deplRec.Code)

	// Valid Cognito ID token → 200 (MOCK integration returns 200 by default).
	rr := authReq(t, h, e, apiID, idToken)
	assert.Equal(t, http.StatusOK, rr.Code, "valid Cognito-signed token must be accepted")

	// Tampered token → 401.
	parts := strings.Split(idToken, ".")
	require.Len(t, parts, 3)
	tampered := parts[0] + "." + parts[1] + ".invalidsignature"
	rr = authReq(t, h, e, apiID, tampered)
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "tampered token must be rejected")

	// No token → 401.
	rr = authReq(t, h, e, apiID, "")
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "missing token must be rejected")
}
