package apigatewayv2_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigatewayv2"
	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

var errShouldNotBeCalled = errors.New("should not be called")

// mockLambdaInvoker is a stub LambdaInvoker for tests.
type mockLambdaInvoker struct {
	fn func(ctx context.Context, name, invocationType string, payload []byte) ([]byte, int, error)
}

func (m *mockLambdaInvoker) InvokeFunction(
	ctx context.Context,
	name, invocationType string,
	payload []byte,
) ([]byte, int, error) {
	if m.fn != nil {
		return m.fn(ctx, name, invocationType, payload)
	}

	resp := map[string]any{"statusCode": 200, "body": "ok"}
	b, _ := json.Marshal(resp)

	return b, 200, nil
}

// ensureDefaultStage creates the API's $default stage if it doesn't already
// exist. The data plane requires a deployed Stage before it will route a
// request (see the stage-gating fix in proxy.go's handleProxy), so every test
// that hits the proxy endpoint needs one; a 409 from a stage created earlier
// in the same test (e.g. via quick create) is expected, not a failure.
func ensureDefaultStage(t *testing.T, h *apigatewayv2.Handler, apiID string) {
	t.Helper()

	rr := doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/stages", map[string]any{
		"stageName":  "$default",
		"autoDeploy": true,
	})
	if rr.Code != http.StatusCreated && rr.Code != http.StatusConflict {
		t.Fatalf("ensureDefaultStage: unexpected status %d: %s", rr.Code, rr.Body.String())
	}
}

// doProxyRequest sends an HTTP request to the v2proxy data plane.
func doProxyRequest(
	t *testing.T,
	h *apigatewayv2.Handler,
	method, apiID, path string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	ensureDefaultStage(t, h, apiID)

	return doProxyRequestToStage(t, h, method, apiID, "$default", path, headers)
}

// doProxyRequestToStage sends an HTTP request to the v2proxy data plane
// against an explicit, already-created stage (unlike doProxyRequest, it does
// not provision "$default" itself, since callers need a specific stage with
// its own autoDeploy setting).
func doProxyRequestToStage(
	t *testing.T,
	h *apigatewayv2.Handler,
	method, apiID, stageName, path string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	proxyPath := fmt.Sprintf("/v2proxy/%s/%s%s", apiID, stageName, path)
	req := httptest.NewRequest(method, proxyPath, strings.NewReader(""))

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rr := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rr)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rr
}

// buildHTTPAPI creates an HTTP API with one route pointing to an AWS_PROXY Lambda integration.
// Returns apiID.
func buildHTTPAPIWithLambda(
	t *testing.T,
	h *apigatewayv2.Handler,
	routeKey string,
	lambdaURI string,
) string {
	t.Helper()

	// Create API.
	rr := doRequest(t, h, http.MethodPost, "/v2/apis", map[string]any{
		"name":         "test-api",
		"protocolType": "HTTP",
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var api apigatewayv2.API
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &api))

	theAPIID := api.APIID

	// Create integration.
	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+theAPIID+"/integrations", map[string]any{
		"integrationType":      "AWS_PROXY",
		"integrationUri":       lambdaURI,
		"payloadFormatVersion": "2.0",
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var integ apigatewayv2.Integration
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integ))

	theIntegrationID := integ.IntegrationID

	// Create route.
	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+theAPIID+"/routes", map[string]any{
		"routeKey": routeKey,
		"target":   "integrations/" + theIntegrationID,
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	return theAPIID
}

// TestHTTPAPIProxy_RouteMatching verifies that the HTTP API data plane correctly
// matches routes, including literal paths, path params, $default fallback, and ANY method.
func TestHTTPAPIProxy_RouteMatching(t *testing.T) {
	t.Parallel()

	const lambdaURI = "arn:aws:lambda:us-east-1:123456789012:function:my-fn/invocations"

	tests := []struct {
		name          string
		requestMethod string
		requestPath   string
		wantRouteKey  string
		setupRoutes   []string
		wantStatus    int
	}{
		{
			name:          "literal_route_exact_match",
			setupRoutes:   []string{"GET /hello"},
			requestMethod: http.MethodGet,
			requestPath:   "/hello",
			wantStatus:    http.StatusOK,
			wantRouteKey:  "GET /hello",
		},
		{
			name:          "path_param_route",
			setupRoutes:   []string{"GET /users/{id}"},
			requestMethod: http.MethodGet,
			requestPath:   "/users/42",
			wantStatus:    http.StatusOK,
			wantRouteKey:  "GET /users/{id}",
		},
		{
			name:          "default_fallback",
			setupRoutes:   []string{"$default"},
			requestMethod: http.MethodPost,
			requestPath:   "/anything/goes",
			wantStatus:    http.StatusOK,
			wantRouteKey:  "$default",
		},
		{
			name:          "any_method_wildcard",
			setupRoutes:   []string{"ANY /items"},
			requestMethod: http.MethodDelete,
			requestPath:   "/items",
			wantStatus:    http.StatusOK,
			wantRouteKey:  "ANY /items",
		},
		{
			name:          "literal_beats_param",
			setupRoutes:   []string{"GET /users/{id}", "GET /users/me"},
			requestMethod: http.MethodGet,
			requestPath:   "/users/me",
			wantStatus:    http.StatusOK,
			wantRouteKey:  "GET /users/me",
		},
		{
			name:          "no_match_no_default",
			setupRoutes:   []string{"GET /specific"},
			requestMethod: http.MethodGet,
			requestPath:   "/other",
			wantStatus:    http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			// Capture which routeKey Lambda receives.
			var capturedRouteKey string

			lambda := &mockLambdaInvoker{
				fn: func(_ context.Context, _, _ string, payload []byte) ([]byte, int, error) {
					var ev map[string]any
					if err := json.Unmarshal(payload, &ev); err == nil {
						capturedRouteKey, _ = ev["routeKey"].(string)
					}

					resp := map[string]any{"statusCode": 200, "body": "routed"}
					b, _ := json.Marshal(resp)

					return b, 200, nil
				},
			}

			h.SetLambdaInvoker(lambda)

			apiID := buildHTTPAPIWithLambda(t, h, tt.setupRoutes[0], lambdaURI)

			// Create extra routes if provided.
			for _, rk := range tt.setupRoutes[1:] {
				rr := doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/integrations", map[string]any{
					"integrationType":      "AWS_PROXY",
					"integrationUri":       lambdaURI,
					"payloadFormatVersion": "2.0",
				})
				require.Equal(t, http.StatusCreated, rr.Code)

				var integ apigatewayv2.Integration
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integ))

				rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/routes", map[string]any{
					"routeKey": rk,
					"target":   "integrations/" + integ.IntegrationID,
				})
				require.Equal(t, http.StatusCreated, rr.Code)
			}

			rr := doProxyRequest(t, h, tt.requestMethod, apiID, tt.requestPath, nil)

			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantRouteKey != "" {
				assert.Equal(t, tt.wantRouteKey, capturedRouteKey)
			}
		})
	}
}

// TestHTTPAPIProxy_NonexistentStage_NotFound proves the data plane requires a
// deployed Stage before it will route a request. Before this fix,
// handleProxy never checked stageName against the backend at all -- any
// string in the URL's stage-name slot, including one that was never created
// via CreateStage, still routed through to the live route/integration.
func TestHTTPAPIProxy_NonexistentStage_NotFound(t *testing.T) {
	t.Parallel()

	const lambdaURI = "arn:aws:lambda:us-east-1:123456789012:function:undeployed-fn/invocations"

	h := newTestHandler()

	lambdaCalled := false
	h.SetLambdaInvoker(&mockLambdaInvoker{
		fn: func(_ context.Context, _, _ string, _ []byte) ([]byte, int, error) {
			lambdaCalled = true

			return nil, 0, errShouldNotBeCalled
		},
	})

	// Deliberately skip doProxyRequest/ensureDefaultStage: the API has a
	// route and integration but no $default stage was ever created.
	apiID := buildHTTPAPIWithLambda(t, h, "GET /hello", lambdaURI)

	req := httptest.NewRequest(http.MethodGet, "/v2proxy/"+apiID+"/$default/hello", strings.NewReader(""))
	rr := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rr)

	require.NoError(t, h.Handler()(c))

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.False(t, lambdaCalled, "an undeployed stage must not reach the integration")
}

// TestHTTPAPIProxy_PayloadFormat verifies both format 1.0 and 2.0 payloads are built correctly.
func TestHTTPAPIProxy_PayloadFormat(t *testing.T) {
	t.Parallel()

	const lambdaURI = "arn:aws:lambda:us-east-1:123456789012:function:pf-fn/invocations"

	tests := []struct {
		name         string
		pfv          string
		wantVersion  string
		wantEventKey string
	}{
		{
			name:         "format_2_0",
			pfv:          "2.0",
			wantVersion:  "2.0",
			wantEventKey: "routeKey",
		},
		{
			name:         "format_1_0",
			pfv:          "1.0",
			wantEventKey: "httpMethod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			var capturedPayload map[string]any
			lambda := &mockLambdaInvoker{
				fn: func(_ context.Context, _, _ string, payload []byte) ([]byte, int, error) {
					_ = json.Unmarshal(payload, &capturedPayload)

					resp := map[string]any{"statusCode": 200, "body": "ok"}
					b, _ := json.Marshal(resp)

					return b, 200, nil
				},
			}
			h.SetLambdaInvoker(lambda)

			// Create API + integration with specific PFV.
			rr := doRequest(t, h, http.MethodPost, "/v2/apis", map[string]any{
				"name": "pf-api", "protocolType": "HTTP",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			var api apigatewayv2.API
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &api))

			rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/integrations", map[string]any{
				"integrationType":      "AWS_PROXY",
				"integrationUri":       lambdaURI,
				"payloadFormatVersion": tt.pfv,
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			var integ apigatewayv2.Integration
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integ))

			rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/routes", map[string]any{
				"routeKey": "GET /test",
				"target":   "integrations/" + integ.IntegrationID,
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			doProxyRequest(t, h, http.MethodGet, api.APIID, "/test", nil)

			require.NotNil(t, capturedPayload)
			_, hasKey := capturedPayload[tt.wantEventKey]
			assert.True(t, hasKey, "expected event key %q in payload", tt.wantEventKey)

			if tt.wantVersion != "" {
				assert.Equal(t, tt.wantVersion, capturedPayload["version"])
			}
		})
	}
}

// TestHTTPAPIProxy_CORSPreflight verifies that OPTIONS requests return CORS headers
// without hitting the Lambda integration.
func TestHTTPAPIProxy_CORSPreflight(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	lambdaCalled := false
	h.SetLambdaInvoker(&mockLambdaInvoker{
		fn: func(_ context.Context, _, _ string, _ []byte) ([]byte, int, error) {
			lambdaCalled = true

			return nil, 0, errShouldNotBeCalled
		},
	})

	// Create API with CORS.
	rr := doRequest(t, h, http.MethodPost, "/v2/apis", map[string]any{
		"name":         "cors-api",
		"protocolType": "HTTP",
		"corsConfiguration": map[string]any{
			"allowOrigins": []string{"https://example.com"},
			"allowMethods": []string{"GET", "POST"},
			"allowHeaders": []string{"Content-Type"},
		},
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var api apigatewayv2.API
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &api))

	// Create a route (even though OPTIONS won't use it).
	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/integrations", map[string]any{
		"integrationType": "AWS_PROXY",
		"integrationUri":  "arn:aws:lambda:us-east-1:123456789012:function:fn/invocations",
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var integ apigatewayv2.Integration
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integ))

	doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/routes", map[string]any{
		"routeKey": "GET /items",
		"target":   "integrations/" + integ.IntegrationID,
	})

	// OPTIONS preflight.
	rr = doProxyRequest(t, h, http.MethodOptions, api.APIID, "/items",
		map[string]string{"Origin": "https://example.com"})

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Contains(t, rr.Header().Get("Access-Control-Allow-Origin"), "example.com")
	assert.False(t, lambdaCalled, "Lambda must not be called for CORS preflight")
}

// TestHTTPAPIProxy_JWTAuthorizer verifies that JWT authorization blocks requests
// without a valid token, and that a blocked request never reaches the
// integration (gopherstack-wsvb: enforceRouteAuth wrote the 401 for a failed
// enforceJWTAuthorizer check and returned nil, so applyRouteControls'
// `if throttleErr/ctrlErr != nil` never fired and the request was forwarded
// anyway). A status-only assertion would still pass under that bug, since
// c.JSON's first WriteHeader call wins the response and the integration's
// later write only corrupts the body underneath the 401 -- hence the
// integrationCalls assertions below.
func TestHTTPAPIProxy_JWTAuthorizer(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	var integrationCalls atomic.Int64
	h.SetLambdaInvoker(&mockLambdaInvoker{
		fn: func(_ context.Context, _, _ string, _ []byte) ([]byte, int, error) {
			integrationCalls.Add(1)

			b, _ := json.Marshal(map[string]any{"statusCode": 200, "body": "ok"})

			return b, 200, nil
		},
	})

	// Create API.
	rr := doRequest(t, h, http.MethodPost, "/v2/apis", map[string]any{
		"name": "jwt-api", "protocolType": "HTTP",
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var api apigatewayv2.API
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &api))

	// Create JWT authorizer.
	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/authorizers", map[string]any{
		"name":           "jwt-auth",
		"authorizerType": "JWT",
		"identitySource": []string{"$request.header.Authorization"},
		"jwtConfiguration": map[string]any{
			"issuer":   "https://example.com/",
			"audience": []string{"my-audience"},
		},
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var authorizer apigatewayv2.Authorizer
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &authorizer))

	// Create integration + route with JWT auth.
	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/integrations", map[string]any{
		"integrationType": "AWS_PROXY",
		"integrationUri":  "arn:aws:lambda:us-east-1:123456789012:function:fn/invocations",
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var integ apigatewayv2.Integration
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integ))

	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/routes", map[string]any{
		"routeKey":          "GET /secure",
		"authorizationType": "JWT",
		"authorizerId":      authorizer.AuthorizerID,
		"target":            "integrations/" + integ.IntegrationID,
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	// Request without Authorization header → 401, integration never invoked.
	rr = doProxyRequest(t, h, http.MethodGet, api.APIID, "/secure", nil)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Equal(t, int64(0), integrationCalls.Load(), "a missing JWT must not reach the integration")

	// Request with a garbage token → 401, integration never invoked.
	rr = doProxyRequest(t, h, http.MethodGet, api.APIID, "/secure",
		map[string]string{"Authorization": "Bearer not-a-jwt"})
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Equal(t, int64(0), integrationCalls.Load(), "an invalid JWT must not reach the integration")
}

// TestHTTPAPIProxy_JWTAuthorizerMissing_DoesNotInvokeIntegration proves a JWT
// route whose authorizerId does not resolve to a stored authorizer is
// rejected with 401 and never reaches the integration (gopherstack-wsvb: the
// same enforceRouteAuth branch that handles a failed JWT check also writes a
// 401 for h.Backend.GetAuthorizer failing and returned nil, defeating the
// applyRouteControls/handleHTTPAPIProxy checks the same way).
func TestHTTPAPIProxy_JWTAuthorizerMissing_DoesNotInvokeIntegration(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	var integrationCalls atomic.Int64
	h.SetLambdaInvoker(&mockLambdaInvoker{
		fn: func(_ context.Context, _, _ string, _ []byte) ([]byte, int, error) {
			integrationCalls.Add(1)

			b, _ := json.Marshal(map[string]any{"statusCode": 200, "body": "ok"})

			return b, 200, nil
		},
	})

	rr := doRequest(t, h, http.MethodPost, "/v2/apis", map[string]any{
		"name": "jwt-missing-authorizer-api", "protocolType": "HTTP",
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var api apigatewayv2.API
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &api))

	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/integrations", map[string]any{
		"integrationType": "AWS_PROXY",
		"integrationUri":  "arn:aws:lambda:us-east-1:123456789012:function:fn/invocations",
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var integ apigatewayv2.Integration
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integ))

	// authorizerId names an authorizer that was never created.
	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/routes", map[string]any{
		"routeKey":          "GET /secure",
		"authorizationType": "JWT",
		"authorizerId":      "does-not-exist",
		"target":            "integrations/" + integ.IntegrationID,
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	rr = doProxyRequest(t, h, http.MethodGet, api.APIID, "/secure",
		map[string]string{"Authorization": "Bearer whatever"})

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Equal(t, int64(0), integrationCalls.Load(),
		"a JWT route with an unresolvable authorizerId must not reach the integration (gopherstack-wsvb)")
}

// TestHTTPAPIProxy_JWTAuthorizer_ValidCognitoToken verifies that a token issued and
// signed by the emulator's Cognito backend passes the JWT authorizer signature check.
func TestHTTPAPIProxy_JWTAuthorizer_ValidCognitoToken(t *testing.T) {
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

	// Build the API Gateway handler with the Cognito backend wired as JWKS provider.
	h := newTestHandler()
	h.SetLambdaInvoker(&mockLambdaInvoker{})
	h.SetJWKSProvider(cognitoBk)

	issuerURL := endpoint + "/" + pool.ID

	rr := doRequest(t, h, http.MethodPost, "/v2/apis", map[string]any{
		"name": "jwt-cognito-api", "protocolType": "HTTP",
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var api apigatewayv2.API
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &api))

	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/authorizers", map[string]any{
		"name":           "cognito-jwt",
		"authorizerType": "JWT",
		"identitySource": []string{"$request.header.Authorization"},
		"jwtConfiguration": map[string]any{
			"issuer":   issuerURL,
			"audience": []string{client.ClientID},
		},
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var authorizer apigatewayv2.Authorizer
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &authorizer))

	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/integrations", map[string]any{
		"integrationType": "AWS_PROXY",
		"integrationUri":  "arn:aws:lambda:us-east-1:123456789012:function:fn/invocations",
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var integ apigatewayv2.Integration
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integ))

	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+api.APIID+"/routes", map[string]any{
		"routeKey":          "GET /secure",
		"authorizationType": "JWT",
		"authorizerId":      authorizer.AuthorizerID,
		"target":            "integrations/" + integ.IntegrationID,
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	// Valid Cognito ID token → 200.
	rr = doProxyRequest(t, h, http.MethodGet, api.APIID, "/secure",
		map[string]string{"Authorization": "Bearer " + idToken})
	assert.Equal(t, http.StatusOK, rr.Code, "valid Cognito-signed token must be accepted")

	// Tampered token (replace signature) → 401.
	parts := strings.Split(idToken, ".")
	require.Len(t, parts, 3)
	tamperedToken := parts[0] + "." + parts[1] + ".invalidsignature"
	rr = doProxyRequest(t, h, http.MethodGet, api.APIID, "/secure",
		map[string]string{"Authorization": "Bearer " + tamperedToken})
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "tampered token must be rejected")
}

// TestHTTPAPIProxy_LambdaResponse verifies that Lambda response fields (status code,
// headers, body) are correctly forwarded to the HTTP client.
func TestHTTPAPIProxy_LambdaResponse(t *testing.T) {
	t.Parallel()

	const lambdaURI = "arn:aws:lambda:us-east-1:123456789012:function:resp-fn/invocations"

	tests := []struct {
		name       string
		lambdaBody map[string]any
		wantBody   string
		wantHeader string
		wantHdrVal string
		wantStatus int
	}{
		{
			name:       "200_with_body",
			lambdaBody: map[string]any{"statusCode": 200, "body": `{"ok":true}`},
			wantStatus: http.StatusOK,
			wantBody:   `{"ok":true}`,
		},
		{
			name:       "404_response",
			lambdaBody: map[string]any{"statusCode": 404, "body": "not found"},
			wantStatus: http.StatusNotFound,
			wantBody:   "not found",
		},
		{
			name: "custom_header",
			lambdaBody: map[string]any{
				"statusCode": 200,
				"body":       "ok",
				"headers":    map[string]any{"X-Custom": "hello"},
			},
			wantStatus: http.StatusOK,
			wantHeader: "X-Custom",
			wantHdrVal: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			lambda := &mockLambdaInvoker{
				fn: func(_ context.Context, _, _ string, _ []byte) ([]byte, int, error) {
					b, _ := json.Marshal(tt.lambdaBody)

					return b, 200, nil
				},
			}
			h.SetLambdaInvoker(lambda)

			apiID := buildHTTPAPIWithLambda(t, h, "GET /check", lambdaURI)
			rr := doProxyRequest(t, h, http.MethodGet, apiID, "/check", nil)

			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantBody != "" {
				assert.Equal(t, tt.wantBody, rr.Body.String())
			}

			if tt.wantHeader != "" {
				assert.Equal(t, tt.wantHdrVal, rr.Header().Get(tt.wantHeader))
			}
		})
	}
}

// TestHTTPAPIProxy_PathParameters verifies that path parameters extracted during
// route matching are forwarded in the Lambda event.
func TestHTTPAPIProxy_PathParameters(t *testing.T) {
	t.Parallel()

	const lambdaURI = "arn:aws:lambda:us-east-1:123456789012:function:pp-fn/invocations"

	h := newTestHandler()

	var capturedPathParams map[string]any

	h.SetLambdaInvoker(&mockLambdaInvoker{
		fn: func(_ context.Context, _, _ string, payload []byte) ([]byte, int, error) {
			var ev map[string]any
			if err := json.Unmarshal(payload, &ev); err == nil {
				if pp, ok := ev["pathParameters"].(map[string]any); ok {
					capturedPathParams = pp
				}
			}

			b, _ := json.Marshal(map[string]any{"statusCode": 200, "body": "ok"})

			return b, 200, nil
		},
	})

	apiID := buildHTTPAPIWithLambda(t, h, "GET /orgs/{orgId}/users/{userId}", lambdaURI)
	rr := doProxyRequest(t, h, http.MethodGet, apiID, "/orgs/acme/users/99", nil)

	require.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, capturedPathParams)
	assert.Equal(t, "acme", capturedPathParams["orgId"])
	assert.Equal(t, "99", capturedPathParams["userId"])
}

// TestHTTPAPIProxy_DeploymentSnapshot proves the data plane routes through a
// stage's pinned deployment snapshot instead of the API's live route and
// integration state (gopherstack-cfr1). Before this fix, handleHTTPAPIProxy
// called h.Backend.GetRoutes/GetIntegration on every request regardless of
// stage, so an autoDeploy=false stage saw an edited integration immediately,
// with no new deployment required -- indistinguishable from autoDeploy=true.
func TestHTTPAPIProxy_DeploymentSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		wantAfterEdit string
		autoDeploy    bool
	}{
		{name: "auto_deploy_false_serves_frozen_snapshot", autoDeploy: false, wantAfterEdit: "from-a"},
		{name: "auto_deploy_true_serves_live_state", autoDeploy: true, wantAfterEdit: "from-b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			h.SetLambdaInvoker(&mockLambdaInvoker{
				fn: func(_ context.Context, name, _ string, _ []byte) ([]byte, int, error) {
					body := "from-a"
					if strings.Contains(name, "fn-b") {
						body = "from-b"
					}

					b, _ := json.Marshal(map[string]any{"statusCode": 200, "body": body})

					return b, 200, nil
				},
			})

			apiID := createAPI(t, h, "snapshot-api")
			createStageAutoDeploy(t, h, apiID, "prod", tt.autoDeploy)

			rr := doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/integrations", map[string]any{
				"integrationType":      "AWS_PROXY",
				"integrationUri":       "arn:aws:lambda:us-east-1:123456789012:function:fn-a/invocations",
				"payloadFormatVersion": "2.0",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			var integ apigatewayv2.Integration
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integ))

			rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/routes", map[string]any{
				"routeKey": "GET /a",
				"target":   "integrations/" + integ.IntegrationID,
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			// Pin "prod" to a deployment covering integration A. Needed for
			// the autoDeploy=false case; a harmless re-pin to an equivalent
			// snapshot for autoDeploy=true, whose CreateRoute above already
			// auto-deployed it.
			rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/deployments", map[string]any{
				"stageName": "prod",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			rr = doProxyRequestToStage(t, h, http.MethodGet, apiID, "prod", "/a", nil)
			require.Equal(t, http.StatusOK, rr.Code)
			require.Equal(t, "from-a", rr.Body.String())

			// Mutate: repoint integration A at a different backend. For the
			// autoDeploy=true stage this fires autoDeployLocked and repoints
			// the stage at a fresh deployment; for autoDeploy=false nothing
			// deploys.
			updatePath := "/v2/apis/" + apiID + "/integrations/" + integ.IntegrationID
			rr = doRequest(t, h, http.MethodPatch, updatePath, map[string]any{
				"integrationUri": "arn:aws:lambda:us-east-1:123456789012:function:fn-b/invocations",
			})
			require.Equal(t, http.StatusOK, rr.Code)

			rr = doProxyRequestToStage(t, h, http.MethodGet, apiID, "prod", "/a", nil)
			require.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, tt.wantAfterEdit, rr.Body.String(),
				"proxy response right after editing the integration, no new deployment yet")

			if tt.autoDeploy {
				return
			}

			// A fresh CreateDeployment must pick up the edit for the
			// autoDeploy=false stage.
			rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/deployments", map[string]any{
				"stageName": "prod",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			rr = doProxyRequestToStage(t, h, http.MethodGet, apiID, "prod", "/a", nil)
			require.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "from-b", rr.Body.String(), "proxy response after a fresh deployment")
		})
	}
}

// TestHTTPAPIProxy_NoDeploymentYet_ServesLiveState proves a stage that exists
// but has never been deployed (stage.DeploymentID == "") still serves
// traffic against the API's current live routes, rather than 500ing because
// it has no pinned snapshot to fall back to (gopherstack-cfr1 negative case).
func TestHTTPAPIProxy_NoDeploymentYet_ServesLiveState(t *testing.T) {
	t.Parallel()

	const lambdaURI = "arn:aws:lambda:us-east-1:123456789012:function:fn/invocations"

	h := newTestHandler()
	h.SetLambdaInvoker(&mockLambdaInvoker{})

	apiID := buildHTTPAPIWithLambda(t, h, "GET /a", lambdaURI)
	createStageAutoDeploy(t, h, apiID, "prod", false)

	stage := getStage(t, h, apiID, "prod")
	require.Empty(t, stage.DeploymentID, "stage must not have a deployment yet for this test")

	rr := doProxyRequestToStage(t, h, http.MethodGet, apiID, "prod", "/a", nil)
	require.Equal(t, http.StatusOK, rr.Code, "an undeployed stage must fall back to live state, not 500")
	assert.Equal(t, "ok", rr.Body.String())
}
