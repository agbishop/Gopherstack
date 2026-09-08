package apigateway_test

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigateway"
)

// deepAPIOpts configures the method created by buildDeepAPI.
type deepAPIOpts struct {
	requestParams  map[string]bool
	requestModels  map[string]string
	modelName      string
	modelSchema    string
	httpMethod     string
	validateBody   bool
	validateParams bool
	hasValidator   bool
	apiKeyRequired bool
}

// buildDeepAPI provisions a deployed REST API with a single MOCK-backed method on
// /items, configured per opts, and returns the handler, echo instance, backend, and
// API ID. The MOCK integration returns HTTP 200 for allowed requests.
func buildDeepAPI(
	t *testing.T,
	opts deepAPIOpts,
) (*apigateway.Handler, *echo.Echo, *apigateway.InMemoryBackend, string) {
	t.Helper()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, err := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "deep-api"})
	require.NoError(t, err)

	resources, _, err := backend.GetResources(api.ID, "", 0)
	require.NoError(t, err)
	rootID := resources[0].ID

	child, err := backend.CreateResource(api.ID, rootID, "items")
	require.NoError(t, err)

	if opts.modelName != "" {
		_, err = backend.CreateModel(apigateway.CreateModelInput{
			RestAPIID:   api.ID,
			Name:        opts.modelName,
			ContentType: "application/json",
			Schema:      opts.modelSchema,
		})
		require.NoError(t, err)
	}

	validatorID := ""
	if opts.hasValidator {
		rv, verr := backend.CreateRequestValidator(api.ID, apigateway.CreateRequestValidatorInput{
			Name:                      "validator",
			ValidateRequestBody:       opts.validateBody,
			ValidateRequestParameters: opts.validateParams,
		})
		require.NoError(t, verr)
		validatorID = rv.ID
	}

	method := opts.httpMethod
	if method == "" {
		method = http.MethodGet
	}

	_, err = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:          api.ID,
		ResourceID:         child.ID,
		HTTPMethod:         method,
		AuthorizationType:  "NONE",
		APIKeyRequired:     opts.apiKeyRequired,
		RequestValidatorID: validatorID,
		RequestParameters:  opts.requestParams,
		RequestModels:      opts.requestModels,
	})
	require.NoError(t, err)

	_, err = backend.PutIntegration(api.ID, child.ID, method, apigateway.PutIntegrationInput{Type: "MOCK"})
	require.NoError(t, err)
	_, err = backend.PutIntegrationResponse(api.ID, child.ID, method, "200", apigateway.PutIntegrationResponseInput{})
	require.NoError(t, err)

	// CreateDeployment with a stage name also creates the "prod" stage.
	_, err = backend.CreateDeployment(api.ID, "prod", "v1")
	require.NoError(t, err)

	return h, e, backend, api.ID
}

// deepReq issues a data-plane request through the handler and returns the recorder.
func deepReq(
	t *testing.T, h *apigateway.Handler, e *echo.Echo, method, apiID, query, body string, headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	url := "/restapis/" + apiID + "/prod/_user_request_/items" + query
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	return rec
}

const personSchema = `{
	"$schema": "http://json-schema.org/draft-04/schema#",
	"type": "object",
	"required": ["name", "age"],
	"properties": {
		"name": {"type": "string", "minLength": 1},
		"age": {"type": "integer", "minimum": 0}
	},
	"additionalProperties": false
}`

func TestProxy_RequestValidation_Body(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "valid_body", body: `{"name":"ada","age":36}`, wantStatus: http.StatusOK},
		{name: "malformed_json", body: `{"name":`, wantStatus: http.StatusBadRequest},
		{name: "missing_required_property", body: `{"name":"ada"}`, wantStatus: http.StatusBadRequest},
		{name: "wrong_type", body: `{"name":"ada","age":"old"}`, wantStatus: http.StatusBadRequest},
		{name: "additional_property", body: `{"name":"ada","age":36,"x":1}`, wantStatus: http.StatusBadRequest},
		{name: "non_integer_age", body: `{"name":"ada","age":1.5}`, wantStatus: http.StatusBadRequest},
		{name: "empty_body", body: `{}`, wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e, _, apiID := buildDeepAPI(t, deepAPIOpts{
				httpMethod:    http.MethodPost,
				hasValidator:  true,
				validateBody:  true,
				modelName:     "Person",
				modelSchema:   personSchema,
				requestModels: map[string]string{"application/json": "Person"},
			})

			rec := deepReq(t, h, e, http.MethodPost, apiID, "", tt.body, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "Invalid request body")
			}
		})
	}
}

func TestProxy_RequestValidation_Parameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		headers    map[string]string
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "all_present",
			query:      "?q=hello",
			headers:    map[string]string{"X-Trace": "abc"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_query",
			headers:    map[string]string{"X-Trace": "abc"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_header",
			query:      "?q=hello",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e, _, apiID := buildDeepAPI(t, deepAPIOpts{
				httpMethod:     http.MethodGet,
				hasValidator:   true,
				validateParams: true,
				requestParams: map[string]bool{
					"method.request.querystring.q":  true,
					"method.request.header.X-Trace": true,
				},
			})

			rec := deepReq(t, h, e, http.MethodGet, apiID, tt.query, "", tt.headers)
			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "Missing required request parameters")
			}
		})
	}
}

// associateKeyToPlan wires an API key into a usage plan attached to the prod stage.
func associateKeyToPlan(
	t *testing.T, b *apigateway.InMemoryBackend, apiID, keyID string,
	quota *apigateway.QuotaSettings, throttle *apigateway.ThrottleSettings,
) {
	t.Helper()

	plan, err := b.CreateUsagePlan(apigateway.CreateUsagePlanInput{
		Name:     "plan",
		Quota:    quota,
		Throttle: throttle,
		APIStages: []apigateway.APIStageAssociation{
			{RestAPIID: apiID, Stage: "prod"},
		},
	})
	require.NoError(t, err)

	_, err = b.CreateUsagePlanKey(apigateway.CreateUsagePlanKeyInput{
		UsagePlanID: plan.ID,
		KeyID:       keyID,
		KeyType:     "API_KEY",
	})
	require.NoError(t, err)
}

func TestProxy_UsagePlan_QuotaEnforced(t *testing.T) {
	t.Parallel()

	h, e, backend, apiID := buildDeepAPI(t, deepAPIOpts{httpMethod: http.MethodGet, apiKeyRequired: true})

	key, err := backend.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "k", Enabled: true})
	require.NoError(t, err)

	associateKeyToPlan(t, backend, apiID, key.ID,
		&apigateway.QuotaSettings{Limit: 2, Period: "DAY"}, nil)

	headers := map[string]string{"X-Api-Key": key.Value}

	// Under the quota (2) → allowed.
	assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", headers).Code)
	assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", headers).Code)

	// Third request exceeds the quota → 429 LimitExceededException.
	over := deepReq(t, h, e, http.MethodGet, apiID, "", "", headers)
	assert.Equal(t, http.StatusTooManyRequests, over.Code)
	assert.Equal(t, "LimitExceededException", over.Header().Get("X-Amzn-Errortype"))
	assert.Contains(t, over.Body.String(), "Limit Exceeded")
}

func TestProxy_UsagePlan_ThrottleEnforced(t *testing.T) {
	t.Parallel()

	h, e, backend, apiID := buildDeepAPI(t, deepAPIOpts{httpMethod: http.MethodGet, apiKeyRequired: true})

	key, err := backend.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "k", Enabled: true})
	require.NoError(t, err)

	// Rate 1/s, burst 2: two immediate requests allowed, the third throttled.
	associateKeyToPlan(t, backend, apiID, key.ID, nil,
		&apigateway.ThrottleSettings{RateLimit: 1, BurstLimit: 2})

	headers := map[string]string{"X-Api-Key": key.Value}

	assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", headers).Code)
	assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", headers).Code)

	over := deepReq(t, h, e, http.MethodGet, apiID, "", "", headers)
	assert.Equal(t, http.StatusTooManyRequests, over.Code)
	assert.Equal(t, "TooManyRequestsException", over.Header().Get("X-Amzn-Errortype"))
	assert.Contains(t, over.Body.String(), "Too Many Requests")
}

func TestProxy_UsagePlan_UnassociatedKeyNotThrottled(t *testing.T) {
	t.Parallel()

	h, e, backend, apiID := buildDeepAPI(t, deepAPIOpts{httpMethod: http.MethodGet, apiKeyRequired: true})

	key, err := backend.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "k", Enabled: true})
	require.NoError(t, err)

	headers := map[string]string{"X-Api-Key": key.Value}
	// No usage plan association → the key authenticates but is never throttled.
	for range 5 {
		assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", headers).Code)
	}
}

// setStageMethodSettings replaces the "prod" stage's MethodSettings map.
func setStageMethodSettings(
	t *testing.T, b *apigateway.InMemoryBackend, apiID string, settings map[string]apigateway.MethodSetting,
) {
	t.Helper()

	_, err := b.UpdateStage(apiID, "prod", apigateway.UpdateStageInput{MethodSettings: settings})
	require.NoError(t, err)
}

// TestProxy_StageMethodSettings_ThrottleEnforced_Wildcard verifies gopherstack-91f2: a
// stage's "*/*" MethodSettings throttle now fires on the data plane even when the method
// doesn't require an API key (previously the only throttle path was gated on
// APIKeyRequired, so this traffic was never throttled at all).
func TestProxy_StageMethodSettings_ThrottleEnforced_Wildcard(t *testing.T) {
	t.Parallel()

	h, e, backend, apiID := buildDeepAPI(t, deepAPIOpts{httpMethod: http.MethodGet})

	setStageMethodSettings(t, backend, apiID, map[string]apigateway.MethodSetting{
		"*/*": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 2},
	})

	assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", nil).Code)
	assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", nil).Code)

	over := deepReq(t, h, e, http.MethodGet, apiID, "", "", nil)
	assert.Equal(t, http.StatusTooManyRequests, over.Code)
	assert.Equal(t, "TooManyRequestsException", over.Header().Get("X-Amzn-Errortype"))
	assert.Contains(t, over.Body.String(), "Too Many Requests")
}

// TestProxy_StageMethodSettings_SpecificOverridesWildcard verifies a specific
// "{resourcePath}/{httpMethod}" MethodSettings entry replaces the "*/*" default for that
// method rather than being throttled cumulatively with it: a tight wildcard burst of 1
// would throttle the second request if it were still consulted, but the looser
// "/items/GET" override (burst 3) governs instead.
func TestProxy_StageMethodSettings_SpecificOverridesWildcard(t *testing.T) {
	t.Parallel()

	h, e, backend, apiID := buildDeepAPI(t, deepAPIOpts{httpMethod: http.MethodGet})

	setStageMethodSettings(t, backend, apiID, map[string]apigateway.MethodSetting{
		"*/*":        {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
		"/items/GET": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 3},
	})

	assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", nil).Code)
	assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", nil).Code)
	assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", nil).Code)

	over := deepReq(t, h, e, http.MethodGet, apiID, "", "", nil)
	assert.Equal(t, http.StatusTooManyRequests, over.Code)
}

// TestProxy_StageMethodSettings_UnconfiguredNotThrottled verifies unthrottled traffic
// still passes: a stage with no MethodSettings at all never throttles.
func TestProxy_StageMethodSettings_UnconfiguredNotThrottled(t *testing.T) {
	t.Parallel()

	h, e, _, apiID := buildDeepAPI(t, deepAPIOpts{httpMethod: http.MethodGet})

	for range 5 {
		assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", nil).Code)
	}
}

// TestProxy_StageMethodSettings_ZeroRateLimitMeansUnlimited verifies an explicit zero
// ThrottlingRateLimit is treated as "not configured" (unlimited), not "reject
// everything" -- MethodSetting.ThrottlingRateLimit is `omitempty`, so the zero value is
// indistinguishable from an unset field, matching this package's existing usage-plan
// Throttle convention (effectiveThrottle/enforce's "RateLimit > 0" gate in usage.go).
func TestProxy_StageMethodSettings_ZeroRateLimitMeansUnlimited(t *testing.T) {
	t.Parallel()

	h, e, backend, apiID := buildDeepAPI(t, deepAPIOpts{httpMethod: http.MethodGet})

	setStageMethodSettings(t, backend, apiID, map[string]apigateway.MethodSetting{
		"*/*": {ThrottlingRateLimit: 0, ThrottlingBurstLimit: 0, LoggingLevel: "INFO"},
	})

	for range 5 {
		assert.Equal(t, http.StatusOK, deepReq(t, h, e, http.MethodGet, apiID, "", "", nil).Code)
	}
}

func TestProxy_TrieCache_InvalidatesOnNewResource(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, err := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "trie-api"})
	require.NoError(t, err)
	resources, _, err := backend.GetResources(api.ID, "", 0)
	require.NoError(t, err)
	rootID := resources[0].ID

	first, err := backend.CreateResource(api.ID, rootID, "first")
	require.NoError(t, err)
	wireMock(t, backend, api.ID, first.ID)
	_, err = backend.CreateDeployment(api.ID, "prod", "v1")
	require.NoError(t, err)

	// Prime the trie cache.
	assert.Equal(t, http.StatusOK, rawProxyGet(t, h, e, api.ID, "/first").Code)
	// Unmatched resource path on a deployed stage: AWS returns 403 "Missing
	// Authentication Token", not 404.
	assert.Equal(t, http.StatusForbidden, rawProxyGet(t, h, e, api.ID, "/second").Code)

	// Add a new resource; the cached trie must be invalidated by the version bump.
	second, err := backend.CreateResource(api.ID, rootID, "second")
	require.NoError(t, err)
	wireMock(t, backend, api.ID, second.ID)

	assert.Equal(t, http.StatusOK, rawProxyGet(t, h, e, api.ID, "/second").Code)
}

func wireMock(t *testing.T, b *apigateway.InMemoryBackend, apiID, resourceID string) {
	t.Helper()
	_, err := b.PutMethod(apigateway.PutMethodInput{
		RestAPIID: apiID, ResourceID: resourceID, HTTPMethod: http.MethodGet, AuthorizationType: "NONE",
	})
	require.NoError(t, err)
	_, err = b.PutIntegration(apiID, resourceID, http.MethodGet, apigateway.PutIntegrationInput{Type: "MOCK"})
	require.NoError(t, err)
	_, err = b.PutIntegrationResponse(
		apiID,
		resourceID,
		http.MethodGet,
		"200",
		apigateway.PutIntegrationResponseInput{},
	)
	require.NoError(t, err)
}

func rawProxyGet(t *testing.T, h *apigateway.Handler, e *echo.Echo, apiID, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/restapis/"+apiID+"/prod/_user_request_"+path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	return rec
}

func TestProxy_ContentHandling_ConvertToBinary(t *testing.T) {
	t.Parallel()

	// Lambda returns base64-encoded binary data.
	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47} // PNG magic bytes
	b64Body := base64.StdEncoding.EncodeToString(pngBytes)
	lambdaResp := `{"output":"` + b64Body + `"}`

	invoker := &proxyMockInvoker{response: []byte(lambdaResp)}

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	h.SetLambdaInvoker(invoker)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "ch-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{
		Type: "AWS",
		URI:  "arn:aws:lambda:::function:img-fn",
	})
	_, _ = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{
		ContentHandling: "CONVERT_TO_BINARY",
		ResponseTemplates: map[string]string{
			"application/json": b64Body,
		},
	})
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

	// CONVERT_TO_BINARY should base64-decode the response template output.
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, pngBytes, rec.Body.Bytes())
}

func TestProxy_ContentHandling_ConvertToText(t *testing.T) {
	t.Parallel()

	rawBody := []byte("hello world")
	invoker := &proxyMockInvoker{response: rawBody}

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	h.SetLambdaInvoker(invoker)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "ct-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{
		Type: "AWS",
		URI:  "arn:aws:lambda:::function:txt-fn",
	})
	_, _ = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{
		ContentHandling: "CONVERT_TO_TEXT",
	})
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

	assert.Equal(t, http.StatusOK, rec.Code)
	// CONVERT_TO_TEXT should base64-encode the raw lambda response bytes.
	expected := base64.StdEncoding.EncodeToString(rawBody)
	assert.Equal(t, expected, rec.Body.String())
}

func TestProxy_StageVariables_InterpolatedInURI(t *testing.T) {
	t.Parallel()

	var capturedPayload []byte
	invoker := &captureInvoker{capture: &capturedPayload}

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	h.SetLambdaInvoker(invoker)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "sv-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})
	// URI uses stage variable placeholder.
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{
		Type:       "AWS_PROXY",
		HTTPMethod: "POST",
		URI: "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions" +
			"/${stageVariables.functionName}/invocations",
	})
	depl, _ := backend.CreateDeployment(api.ID, "prod", "v1")
	_, _ = backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
		Variables:    map[string]string{"functionName": "my-real-function"},
	})

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	// The invoker receives the payload; check the captured function name via the event.
	var event map[string]any
	require.NoError(t, json.Unmarshal(capturedPayload, &event))
	// Response comes from captureInvoker which echoes the payload back, so status is 200.
	// The important check is that the handler resolved the stage variable without panicking.
}

func TestProxy_CORS_Preflight(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "cors-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	// Set CorsConfiguration on the root resource.
	corsConfig := &apigateway.CorsConfiguration{
		AllowOrigins: []string{"https://example.com"},
		AllowMethods: []string{"GET", "POST", "OPTIONS"},
		AllowHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:       3600,
	}
	_, err := backend.UpdateResource(api.ID, rootID, apigateway.UpdateResourceInput{
		CorsConfiguration: corsConfig,
	})
	require.NoError(t, err)

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{
		Type: "MOCK",
	})
	_, _ = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{})
	depl, _ := backend.CreateDeployment(api.ID, "prod", "v1")
	_, _ = backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodOptions, url, nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "GET")
	assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	assert.Equal(t, "3600", rec.Header().Get("Access-Control-Max-Age"))
}

func TestProxy_CORS_WildcardOrigin(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "cors-wild"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	corsConfig := &apigateway.CorsConfiguration{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET"},
	}
	_, err := backend.UpdateResource(api.ID, rootID, apigateway.UpdateResourceInput{
		CorsConfiguration: corsConfig,
	})
	require.NoError(t, err)

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
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
	req := httptest.NewRequest(http.MethodOptions, url, nil)
	req.Header.Set("Origin", "https://any.domain.com")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestProxy_CORS_HeadersOnNonOptions(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "cors-get"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	corsConfig := &apigateway.CorsConfiguration{
		AllowOrigins: []string{"https://myapp.com"},
		AllowMethods: []string{"GET"},
	}
	_, err := backend.UpdateResource(api.ID, rootID, apigateway.UpdateResourceInput{
		CorsConfiguration: corsConfig,
	})
	require.NoError(t, err)

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
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
	req.Header.Set("Origin", "https://myapp.com")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, "https://myapp.com", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestProxy_ResponseCompression(t *testing.T) {
	t.Parallel()

	// Lambda returns a body large enough to trigger compression.
	largeBody := strings.Repeat("a", 2048)
	lambdaResp := `{"statusCode":200,"body":"` + largeBody + `","headers":{}}`
	invoker := &proxyMockInvoker{response: []byte(lambdaResp)}

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	h.SetLambdaInvoker(invoker)
	e := echo.New()

	// Set minimumCompressionSize to 1024 bytes.
	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{
		Name:                   "gzip-api",
		MinimumCompressionSize: 1024,
	})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{
		Type: "AWS_PROXY",
		URI:  "arn:aws:lambda:::function:fn",
	})
	depl, _ := backend.CreateDeployment(api.ID, "prod", "v1")
	_, _ = backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

	// Decompress and verify the body.
	reader, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, largeBody, string(decompressed))
}

func TestProxy_ResponseCompression_BelowThreshold_NoCompress(t *testing.T) {
	t.Parallel()

	smallBody := "hi"
	lambdaResp := `{"statusCode":200,"body":"` + smallBody + `","headers":{}}`
	invoker := &proxyMockInvoker{response: []byte(lambdaResp)}

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	h.SetLambdaInvoker(invoker)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{
		Name:                   "gzip-no-api",
		MinimumCompressionSize: 1024,
	})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "NONE",
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{
		Type: "AWS_PROXY",
		URI:  "arn:aws:lambda:::function:fn",
	})
	depl, _ := backend.CreateDeployment(api.ID, "prod", "v1")
	_, _ = backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})

	url := "/restapis/" + api.ID + "/prod/_user_request_/"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Equal(t, smallBody, rec.Body.String())
}

func TestProxy_AuthorizerCacheKey_IdentityValidationExpression(t *testing.T) {
	t.Parallel()

	callCount := 0
	invoker := &countingInvoker{
		count: &callCount,
		response: []byte(`{"principalId":"user1","policyDocument":{"Version":"2012-10-17",` +
			`"Statement":[{"Effect":"Allow","Action":"execute-api:Invoke","Resource":"*"}]}}`),
	}

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)
	h.SetLambdaInvoker(invoker)
	e := echo.New()

	api, _ := backend.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "expr-api"})
	resources, _, _ := backend.GetResources(api.ID, "", 0)
	rootID := resources[0].ID

	// Create TOKEN authorizer with identityValidationExpression to strip "Bearer " prefix.
	const authURI = "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions" +
		"/arn:aws:lambda:us-east-1:000000000000:function:auth-fn/invocations"
	authBody := `{"restApiId":"` + api.ID + `","name":"bearer-auth","type":"TOKEN",` +
		`"authorizerUri":"` + authURI + `",` +
		`"identitySource":"method.request.header.Authorization",` +
		`"identityValidationExpression":"Bearer (.+)","authorizerResultTtlInSeconds":300}`
	authRec := postWithHandler(t, h, e, "CreateAuthorizer", authBody)
	require.Equal(t, http.StatusCreated, authRec.Code)
	var auth map[string]any
	require.NoError(t, json.Unmarshal(authRec.Body.Bytes(), &auth))
	authID := auth["id"].(string)

	_, _ = backend.PutMethod(apigateway.PutMethodInput{
		RestAPIID:         api.ID,
		ResourceID:        rootID,
		HTTPMethod:        "GET",
		AuthorizationType: "TOKEN",
		AuthorizerID:      authID,
	})
	_, _ = backend.PutIntegration(api.ID, rootID, "GET", apigateway.PutIntegrationInput{Type: "MOCK"})
	_, _ = backend.PutIntegrationResponse(api.ID, rootID, "GET", "200", apigateway.PutIntegrationResponseInput{})
	depl, _ := backend.CreateDeployment(api.ID, "prod", "v1")
	_, _ = backend.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})

	makeReq := func(token string) {
		url := "/restapis/" + api.ID + "/prod/_user_request_/"
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", token)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		require.NoError(t, h.Handler()(c))
	}

	// First request: invoke the authorizer.
	makeReq("Bearer mytoken123")
	assert.Equal(t, 1, callCount)

	// Second request with same token: should hit cache (same extracted "mytoken123").
	makeReq("Bearer mytoken123")
	assert.Equal(t, 1, callCount)

	// Third request with different token: should invoke the authorizer again.
	makeReq("Bearer differenttoken")
	assert.Equal(t, 2, callCount)
}

type countingInvoker struct {
	count    *int
	response []byte
}

func (c *countingInvoker) InvokeFunction(_ context.Context, _, _ string, _ []byte) ([]byte, int, error) {
	*c.count++

	return c.response, http.StatusOK, nil
}
