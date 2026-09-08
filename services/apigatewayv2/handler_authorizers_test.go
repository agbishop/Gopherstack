package apigatewayv2_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigatewayv2"
)

func TestHandler_Authorizers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		authorizerName string
		wantCreateCode int
	}{
		{
			name:           "success",
			authorizerName: "my-auth",
			wantCreateCode: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID := createAPI(t, h, "test-api")

			rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/authorizers", apiID), map[string]any{
				"name":           tt.authorizerName,
				"authorizerType": "JWT",
				"jwtConfiguration": map[string]any{
					"issuer":   "https://issuer.example.com",
					"audience": []string{"client-id"},
				},
			})
			require.Equal(t, tt.wantCreateCode, rr.Code)

			var auth apigatewayv2.Authorizer
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &auth))
			assert.Equal(t, tt.authorizerName, auth.Name)

			// GetAuthorizers
			rr = doRequest(t, h, http.MethodGet, fmt.Sprintf("/v2/apis/%s/authorizers", apiID), nil)
			require.Equal(t, http.StatusOK, rr.Code)

			// GetAuthorizer
			rr = doRequest(
				t,
				h,
				http.MethodGet,
				fmt.Sprintf("/v2/apis/%s/authorizers/%s", apiID, auth.AuthorizerID),
				nil,
			)
			require.Equal(t, http.StatusOK, rr.Code)

			// UpdateAuthorizer
			rr = doRequest(
				t,
				h,
				http.MethodPatch,
				fmt.Sprintf("/v2/apis/%s/authorizers/%s", apiID, auth.AuthorizerID),
				map[string]any{
					"name": "updated-auth",
				},
			)
			require.Equal(t, http.StatusOK, rr.Code)

			// DeleteAuthorizer
			rr = doRequest(
				t,
				h,
				http.MethodDelete,
				fmt.Sprintf("/v2/apis/%s/authorizers/%s", apiID, auth.AuthorizerID),
				nil,
			)
			require.Equal(t, http.StatusNoContent, rr.Code)

			// Get after delete = 404
			rr = doRequest(
				t,
				h,
				http.MethodGet,
				fmt.Sprintf("/v2/apis/%s/authorizers/%s", apiID, auth.AuthorizerID),
				nil,
			)
			require.Equal(t, http.StatusNotFound, rr.Code)
		})
	}
}

func TestHandler_ResetAuthorizersCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *apigatewayv2.Handler) (apiID, stageName string)
		name       string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *apigatewayv2.Handler) (string, string) {
				apiID := createAPI(t, h, "test-api")
				createStage(t, h, apiID, "prod")

				return apiID, "prod"
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name: "api_not_found",
			setup: func(_ *apigatewayv2.Handler) (string, string) {
				return "nonexistent", "prod"
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "stage_not_found",
			setup: func(h *apigatewayv2.Handler) (string, string) {
				return createAPI(t, h, "test-api"), "nonexistent"
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID, stageName := tt.setup(h)

			rr := doRequest(t, h, http.MethodDelete,
				fmt.Sprintf("/v2/apis/%s/stages/%s/cache/authorizers", apiID, stageName), nil)

			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestHandler_CreateAuthorizer_InvalidType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "valid_jwt",
			body: map[string]any{
				"name":           "my-auth",
				"authorizerType": "JWT",
				"jwtConfiguration": map[string]any{
					"issuer":   "https://issuer.example.com",
					"audience": []string{"client-id"},
				},
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid_request",
			body:       map[string]any{"name": "my-auth", "authorizerType": "REQUEST"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid_type",
			body:       map[string]any{"name": "my-auth", "authorizerType": "INVALID"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID := createAPI(t, h, "test-api")

			rr := doRequest(t, h, http.MethodPost, fmt.Sprintf("/v2/apis/%s/authorizers", apiID), tt.body)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestGetAuthorizers_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	apiID := createAPI(t, h, "auth-api")

	for i := range 4 {
		rr := doRequest(t, h, http.MethodPost,
			fmt.Sprintf("/v2/apis/%s/authorizers", apiID),
			map[string]any{
				"name":           fmt.Sprintf("auth-%02d", i),
				"authorizerType": "JWT",
				"jwtConfiguration": map[string]any{
					"issuer":   "https://issuer.example.com",
					"audience": []string{"aud"},
				},
			},
		)
		require.Equal(t, http.StatusCreated, rr.Code)
	}

	tests := []struct {
		name       string
		maxResults int
		wantPages  int
	}{
		{"two_per_page", 2, 2},
		{"all_at_once", 10, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			seen := map[string]int{}
			nextToken := ""
			pages := 0

			for {
				path := fmt.Sprintf("/v2/apis/%s/authorizers?maxResults=%d", apiID, tc.maxResults)
				if nextToken != "" {
					path += "&nextToken=" + nextToken
				}

				rr := doRequest(t, h, http.MethodGet, path, nil)
				require.Equal(t, http.StatusOK, rr.Code)

				var resp struct {
					NextToken string           `json:"nextToken"`
					Items     []map[string]any `json:"items"`
				}
				require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
				require.LessOrEqual(t, len(resp.Items), tc.maxResults)

				for _, a := range resp.Items {
					id, _ := a["authorizerId"].(string)
					seen[id]++
				}

				pages++
				nextToken = resp.NextToken

				if nextToken == "" {
					break
				}

				require.Less(t, pages, 20)
			}

			assert.Equal(t, tc.wantPages, pages)
			assert.Len(t, seen, 4)

			for id, count := range seen {
				assert.Equalf(t, 1, count, "authorizer %s duplicated", id)
			}
		})
	}
}

const (
	authFnName   = "arn:aws:lambda:us-east-1:123456789012:function:auth-fn"
	backendFnURI = "arn:aws:lambda:us-east-1:123456789012:function:backend-fn/invocations"
	// errTypeHeaderKey is the canonical form of the x-amzn-ErrorType header
	// (Go's http.Header canonicalises header keys to this casing).
	errTypeHeaderKey = "X-Amzn-Errortype"
	sigV4AuthHeader  = "AWS4-HMAC-SHA256 Credential=AKID/20230101/us-east-1/execute-api/aws4_request"
)

// authScenario describes a Lambda authorizer configuration and its response.
type authScenario struct {
	authResponse      map[string]any
	payloadVersion    string
	identitySource    []string
	ttlSeconds        int
	enableSimple      bool
	authorizerMissing bool
}

// setupRequestAuthAPI creates an HTTP API with an AWS_PROXY integration, a
// REQUEST authorizer, and a CUSTOM-authorized route. It returns the api id and
// two atomic counters recording how many times the authorizer Lambda and the
// route's integration Lambda are each invoked -- integrationCalls exists so
// tests can prove a denied request never reaches the integration
// (gopherstack-wsvb: enforceRequestAuthorizer wrote its 401/403 and returned
// nil, so a denial fell through to invoke the integration anyway).
func setupRequestAuthAPI(
	t *testing.T,
	h *apigatewayv2.Handler,
	sc authScenario,
) (string, *atomic.Int64, *atomic.Int64) {
	t.Helper()

	const routeKey = "GET /secure"

	authCalls := &atomic.Int64{}
	integrationCalls := &atomic.Int64{}

	respBytes, _ := json.Marshal(sc.authResponse)

	h.SetLambdaInvoker(&mockLambdaInvoker{
		fn: func(_ context.Context, name, _ string, _ []byte) ([]byte, int, error) {
			if strings.Contains(name, "auth-fn") {
				authCalls.Add(1)

				return respBytes, 200, nil
			}

			// Integration backend function.
			integrationCalls.Add(1)

			b, _ := json.Marshal(map[string]any{"statusCode": 200, "body": "ok"})

			return b, 200, nil
		},
	})

	apiID := createAPI(t, h, "authz-api")

	// Integration.
	rr := doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/integrations", map[string]any{
		"integrationType":      "AWS_PROXY",
		"integrationUri":       backendFnURI,
		"payloadFormatVersion": "2.0",
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	var integ apigatewayv2.Integration
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integ))

	// Authorizer.
	identity := sc.identitySource
	if identity == nil {
		identity = []string{"$request.header.Authorization"}
	}

	authBody := map[string]any{
		"name":                         "req-authorizer",
		"authorizerType":               "REQUEST",
		"authorizerUri":                authFnName,
		"identitySource":               identity,
		"authorizerResultTtlInSeconds": sc.ttlSeconds,
		"enableSimpleResponses":        sc.enableSimple,
	}
	if sc.payloadVersion != "" {
		authBody["authorizerPayloadFormatVersion"] = sc.payloadVersion
	}

	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/authorizers", authBody)
	require.Equal(t, http.StatusCreated, rr.Code)

	var auth apigatewayv2.Authorizer
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &auth))

	authorizerID := auth.AuthorizerID
	if sc.authorizerMissing {
		authorizerID = "does-not-exist"
	}

	// Route with CUSTOM authorization.
	rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/routes", map[string]any{
		"routeKey":          routeKey,
		"target":            "integrations/" + integ.IntegrationID,
		"authorizationType": "CUSTOM",
		"authorizerId":      authorizerID,
	})
	require.Equal(t, http.StatusCreated, rr.Code)

	return apiID, authCalls, integrationCalls
}

func TestRequestAuthorizer_SimpleResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		isAuth               bool
		wantStatus           int
		wantIntegrationCalls int64
	}{
		{name: "simple_allow", isAuth: true, wantStatus: http.StatusOK, wantIntegrationCalls: 1},
		{name: "simple_deny", isAuth: false, wantStatus: http.StatusForbidden, wantIntegrationCalls: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID, authCalls, integrationCalls := setupRequestAuthAPI(t, h, authScenario{
				payloadVersion: "2.0",
				enableSimple:   true,
				ttlSeconds:     0,
				authResponse:   map[string]any{"isAuthorized": tt.isAuth},
			})

			rr := doProxyRequest(t, h, http.MethodGet, apiID, "/secure", map[string]string{
				"Authorization": "Bearer token-123",
			})

			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Equal(t, int64(1), authCalls.Load())
			// A denied REQUEST-authorizer decision must not reach the route's
			// integration (gopherstack-wsvb): a status-only assertion here would
			// still pass even if the denial fell through to invoke it, since
			// the first WriteHeader call wins and the second write only
			// corrupts the body.
			assert.Equal(t, tt.wantIntegrationCalls, integrationCalls.Load())

			if tt.wantStatus == http.StatusForbidden {
				assert.Equal(t, "AccessDeniedException", rr.Header().Get(errTypeHeaderKey))
			}
		})
	}
}

func TestRequestAuthorizer_IAMPolicyResponse(t *testing.T) {
	t.Parallel()

	allowStmt := func(effect, resource string) map[string]any {
		return map[string]any{
			"principalId": "user-1",
			"policyDocument": map[string]any{
				"Version": "2012-10-17",
				"Statement": []any{
					map[string]any{
						"Effect":   effect,
						"Action":   "execute-api:Invoke",
						"Resource": resource,
					},
				},
			},
		}
	}

	tests := []struct {
		response             map[string]any
		name                 string
		wantStatus           int
		wantIntegrationCalls int64
	}{
		{
			name:                 "policy_allow_wildcard",
			response:             allowStmt("Allow", "*"),
			wantStatus:           http.StatusOK,
			wantIntegrationCalls: 1,
		},
		{
			name:                 "policy_explicit_deny",
			response:             allowStmt("Deny", "*"),
			wantStatus:           http.StatusForbidden,
			wantIntegrationCalls: 0,
		},
		{
			name:                 "policy_no_matching_resource_implicit_deny",
			response:             allowStmt("Allow", "arn:aws:execute-api:us-east-1:123456789012:other/*/GET/nope"),
			wantStatus:           http.StatusForbidden,
			wantIntegrationCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID, authCalls, integrationCalls := setupRequestAuthAPI(t, h, authScenario{
				payloadVersion: "1.0",
				enableSimple:   false,
				authResponse:   tt.response,
			})

			rr := doProxyRequest(t, h, http.MethodGet, apiID, "/secure", map[string]string{
				"Authorization": "Bearer token-123",
			})

			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Equal(t, int64(1), authCalls.Load())
			assert.Equal(t, tt.wantIntegrationCalls, integrationCalls.Load(),
				"an explicit or implicit IAM-policy deny must not reach the integration (gopherstack-wsvb)")
		})
	}
}

func TestRequestAuthorizer_MissingIdentitySource(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	apiID, authCalls, integrationCalls := setupRequestAuthAPI(t, h, authScenario{
		payloadVersion: "2.0",
		enableSimple:   true,
		identitySource: []string{"$request.header.Authorization"},
		authResponse:   map[string]any{"isAuthorized": true},
	})

	// No Authorization header → required identity source missing → 401, and the
	// authorizer Lambda must not be invoked.
	rr := doProxyRequest(t, h, http.MethodGet, apiID, "/secure", nil)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Equal(t, "UnauthorizedException", rr.Header().Get(errTypeHeaderKey))
	assert.Equal(t, int64(0), authCalls.Load())
	assert.Equal(t, int64(0), integrationCalls.Load(),
		"a missing identity source must not reach the integration (gopherstack-wsvb)")
}

func TestRequestAuthorizer_CachingTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		ttl        int
		wantCalls  int64
		numRequest int
	}{
		{name: "cache_hit_ttl_positive", ttl: 300, numRequest: 3, wantCalls: 1},
		{name: "no_cache_ttl_zero", ttl: 0, numRequest: 3, wantCalls: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID, authCalls, integrationCalls := setupRequestAuthAPI(t, h, authScenario{
				payloadVersion: "2.0",
				enableSimple:   true,
				ttlSeconds:     tt.ttl,
				authResponse:   map[string]any{"isAuthorized": true},
			})

			for range tt.numRequest {
				rr := doProxyRequest(t, h, http.MethodGet, apiID, "/secure", map[string]string{
					"Authorization": "Bearer token-abc",
				})
				require.Equal(t, http.StatusOK, rr.Code)
			}

			assert.Equal(t, tt.wantCalls, authCalls.Load())
			assert.Equal(t, int64(tt.numRequest), integrationCalls.Load(),
				"every allowed request must reach the integration regardless of auth-decision caching")
		})
	}
}

func TestRequestAuthorizer_MissingAuthorizerRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	apiID, _, integrationCalls := setupRequestAuthAPI(t, h, authScenario{
		payloadVersion:    "2.0",
		enableSimple:      true,
		authorizerMissing: true,
		authResponse:      map[string]any{"isAuthorized": true},
	})

	rr := doProxyRequest(t, h, http.MethodGet, apiID, "/secure", map[string]string{
		"Authorization": "Bearer token-123",
	})

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Equal(t, int64(0), integrationCalls.Load(),
		"a CUSTOM route whose authorizerId does not resolve must not reach the integration (gopherstack-wsvb)")
}

func TestRouteAuth_AWSIAM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		headers              map[string]string
		query                string
		name                 string
		wantStatus           int
		wantIntegrationCalls int64
	}{
		{
			name:                 "unsigned_request_rejected",
			headers:              nil,
			wantStatus:           http.StatusForbidden,
			wantIntegrationCalls: 0,
		},
		{
			name:                 "sigv4_signed_allowed",
			headers:              map[string]string{"Authorization": sigV4AuthHeader},
			wantStatus:           http.StatusOK,
			wantIntegrationCalls: 1,
		},
		{
			name:                 "presigned_query_allowed",
			query:                "?X-Amz-Algorithm=AWS4-HMAC-SHA256",
			wantStatus:           http.StatusOK,
			wantIntegrationCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			apiID := createAPI(t, h, "iam-api")

			rr := doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/integrations", map[string]any{
				"integrationType":      "AWS_PROXY",
				"integrationUri":       backendFnURI,
				"payloadFormatVersion": "2.0",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			var integ apigatewayv2.Integration
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &integ))

			rr = doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/routes", map[string]any{
				"routeKey":          "GET /iam",
				"target":            "integrations/" + integ.IntegrationID,
				"authorizationType": "AWS_IAM",
			})
			require.Equal(t, http.StatusCreated, rr.Code)

			rr = doProxyRequest(t, h, http.MethodGet, apiID, "/iam"+tt.query, tt.headers)
			assert.Equal(t, tt.wantStatus, rr.Code)
			// A rejected AWS_IAM request must not reach the integration
			// (gopherstack-wsvb): the status alone can pass even when it does,
			// since c.JSON's first WriteHeader call wins and the integration's
			// later write only corrupts the body underneath the 403.
			assert.Equal(t, tt.wantIntegrationCalls, integrationCalls.Load())

			if tt.wantStatus == http.StatusForbidden {
				assert.Equal(t, "AccessDeniedException", rr.Header().Get(errTypeHeaderKey))
			}
		})
	}
}

func TestCreateRoute_AuthorizationTypeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		authType   string
		wantStatus int
	}{
		{name: "none_ok", authType: "NONE", wantStatus: http.StatusCreated},
		{name: "aws_iam_ok", authType: "AWS_IAM", wantStatus: http.StatusCreated},
		{name: "invalid_rejected", authType: "BOGUS", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			apiID := createAPI(t, h, "route-auth-api")

			rr := doRequest(t, h, http.MethodPost, "/v2/apis/"+apiID+"/routes", map[string]any{
				"routeKey":          "GET /r",
				"authorizationType": tt.authType,
			})

			assert.Equal(t, tt.wantStatus, rr.Code)

			if tt.wantStatus == http.StatusBadRequest {
				assert.Equal(t, "BadRequestException", rr.Header().Get(errTypeHeaderKey))
			}
		})
	}
}
