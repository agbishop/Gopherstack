package apigatewayv2_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigatewayv2"
)

const throttleLambdaURI = "arn:aws:lambda:us-east-1:123456789012:function:my-fn/invocations"

// TestHTTPAPIProxy_RouteThrottle_TooManyRequests proves the HTTP API data plane
// enforces a route's RouteSettings burst limit and returns AWS's real 429 shape
// (gopherstack-dv44: RouteSettings were stored and echoed but nothing enforced
// them).
//
// It also proves the throttled request never reaches the integration
// (gopherstack-wsvb: enforceRouteThrottle wrote the 429 and returned nil, so
// applyRouteControls' `if throttleErr != nil` never fired and the request was
// forwarded anyway). A status-only assertion here would still pass under that
// bug, since c.JSON's first WriteHeader call wins the response and the
// integration's later write only corrupts the body underneath the 429 --
// hence the integrationCalls assertion below, not just the body-contains check.
func TestHTTPAPIProxy_RouteThrottle_TooManyRequests(t *testing.T) {
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

	apiID := buildHTTPAPIWithLambda(t, h, "GET /items", throttleLambdaURI)
	ensureDefaultStage(t, h, apiID)

	_, err := h.Backend.UpdateStage(apiID, "$default", apigatewayv2.UpdateStageInput{
		RouteSettings: map[string]apigatewayv2.RouteSettings{
			"GET /items": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
		},
	})
	require.NoError(t, err)

	first := doProxyRequest(t, h, http.MethodGet, apiID, "/items", nil)
	require.Equal(t, http.StatusOK, first.Code, "first request must consume the single burst token")
	require.Equal(t, int64(1), integrationCalls.Load(), "the allowed first request must reach the integration")

	second := doProxyRequest(t, h, http.MethodGet, apiID, "/items", nil)
	require.Equal(t, http.StatusTooManyRequests, second.Code, "the burst-1 bucket must be exhausted after one request")
	assert.Contains(t, second.Body.String(), "Too Many Requests")
	assert.Equal(t, "TooManyRequestsException", second.Header().Get("X-Amzn-Errortype"))
	assert.Equal(t, int64(1), integrationCalls.Load(),
		"a throttled request must not reach the integration (gopherstack-wsvb)")
}

// TestHTTPAPIProxy_RouteThrottle_Unconfigured proves an unconfigured route never
// throttles: no RouteSettings entry and no DefaultRouteSettings means AWS applies
// no route-level limit.
func TestHTTPAPIProxy_RouteThrottle_Unconfigured(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	h.SetLambdaInvoker(&mockLambdaInvoker{})

	apiID := buildHTTPAPIWithLambda(t, h, "GET /items", throttleLambdaURI)

	for range 5 {
		rr := doProxyRequest(t, h, http.MethodGet, apiID, "/items", nil)
		require.Equal(t, http.StatusOK, rr.Code, "an unconfigured route must never throttle")
	}
}

// TestHTTPAPIProxy_RouteThrottle_ZeroLimitUnlimited proves an explicit zero
// ThrottlingRateLimit behaves as unconfigured (unlimited), not as "throttle
// everything" -- RouteSettings.ThrottlingRateLimit is `omitempty`, so a zero
// value is indistinguishable on the wire from an absent one.
func TestHTTPAPIProxy_RouteThrottle_ZeroLimitUnlimited(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	h.SetLambdaInvoker(&mockLambdaInvoker{})

	apiID := buildHTTPAPIWithLambda(t, h, "GET /items", throttleLambdaURI)
	ensureDefaultStage(t, h, apiID)

	_, err := h.Backend.UpdateStage(apiID, "$default", apigatewayv2.UpdateStageInput{
		RouteSettings: map[string]apigatewayv2.RouteSettings{
			"GET /items": {ThrottlingRateLimit: 0, ThrottlingBurstLimit: 1},
		},
	})
	require.NoError(t, err)

	for range 5 {
		rr := doProxyRequest(t, h, http.MethodGet, apiID, "/items", nil)
		require.Equal(t, http.StatusOK, rr.Code, "a zero ThrottlingRateLimit must mean unlimited")
	}
}

// TestHTTPAPIProxy_RouteThrottle_DefaultRouteSettingsApplies proves a stage's
// DefaultRouteSettings throttles a route that has no per-route RouteSettings
// override (aws-sdk-go-v2 apigatewayv2 types.Stage.DefaultRouteSettings: "Default
// route settings for the stage").
func TestHTTPAPIProxy_RouteThrottle_DefaultRouteSettingsApplies(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	h.SetLambdaInvoker(&mockLambdaInvoker{})

	apiID := buildHTTPAPIWithLambda(t, h, "GET /items", throttleLambdaURI)
	ensureDefaultStage(t, h, apiID)

	_, err := h.Backend.UpdateStage(apiID, "$default", apigatewayv2.UpdateStageInput{
		DefaultRouteSettings: &apigatewayv2.RouteSettings{ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
	})
	require.NoError(t, err)

	first := doProxyRequest(t, h, http.MethodGet, apiID, "/items", nil)
	require.Equal(t, http.StatusOK, first.Code)

	second := doProxyRequest(t, h, http.MethodGet, apiID, "/items", nil)
	require.Equal(t, http.StatusTooManyRequests, second.Code,
		"DefaultRouteSettings must throttle a route without its own RouteSettings override")
}

// TestHTTPAPIProxy_RouteThrottle_RouteOverrideWinsOverDefault proves a route's
// RouteSettings entry replaces DefaultRouteSettings entirely rather than
// merging with it: a generous default and a burst-1 override must throttle at
// the override's limit.
func TestHTTPAPIProxy_RouteThrottle_RouteOverrideWinsOverDefault(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	h.SetLambdaInvoker(&mockLambdaInvoker{})

	apiID := buildHTTPAPIWithLambda(t, h, "GET /items", throttleLambdaURI)
	ensureDefaultStage(t, h, apiID)

	_, err := h.Backend.UpdateStage(apiID, "$default", apigatewayv2.UpdateStageInput{
		DefaultRouteSettings: &apigatewayv2.RouteSettings{ThrottlingRateLimit: 1000, ThrottlingBurstLimit: 1000},
		RouteSettings: map[string]apigatewayv2.RouteSettings{
			"GET /items": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
		},
	})
	require.NoError(t, err)

	first := doProxyRequest(t, h, http.MethodGet, apiID, "/items", nil)
	require.Equal(t, http.StatusOK, first.Code)

	second := doProxyRequest(t, h, http.MethodGet, apiID, "/items", nil)
	require.Equal(t, http.StatusTooManyRequests, second.Code,
		"a per-route RouteSettings override must apply instead of the generous DefaultRouteSettings")
}

// TestHTTPAPIProxy_RouteThrottle_PerRouteIsolation proves each route's throttle
// bucket is independent: exhausting one route's burst must not affect another
// route on the same stage.
func TestHTTPAPIProxy_RouteThrottle_PerRouteIsolation(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	h.SetLambdaInvoker(&mockLambdaInvoker{})

	apiID := buildHTTPAPIWithLambda(t, h, "GET /items", throttleLambdaURI)

	integrations, err := h.Backend.GetIntegrations(apiID)
	require.NoError(t, err)
	require.Len(t, integrations, 1)

	_, err = h.Backend.CreateRoute(apiID, apigatewayv2.CreateRouteInput{
		RouteKey: "GET /other",
		Target:   "integrations/" + integrations[0].IntegrationID,
	})
	require.NoError(t, err)

	ensureDefaultStage(t, h, apiID)

	_, err = h.Backend.UpdateStage(apiID, "$default", apigatewayv2.UpdateStageInput{
		RouteSettings: map[string]apigatewayv2.RouteSettings{
			"GET /items": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
		},
	})
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, doProxyRequest(t, h, http.MethodGet, apiID, "/items", nil).Code)
	require.Equal(t, http.StatusTooManyRequests, doProxyRequest(t, h, http.MethodGet, apiID, "/items", nil).Code)

	other := doProxyRequest(t, h, http.MethodGet, apiID, "/other", nil)
	assert.Equal(t, http.StatusOK, other.Code, "an unconfigured route must not share a bucket with a throttled one")
}

// TestBackend_DeleteStage_ClearsRouteThrottleBuckets proves DeleteStage evicts
// the stage's route throttle buckets, so a stage name reused after deletion
// starts with a fresh bucket instead of inheriting an already-exhausted one.
func TestBackend_DeleteStage_ClearsRouteThrottleBuckets(t *testing.T) {
	t.Parallel()

	b := apigatewayv2.NewInMemoryBackend()
	api, err := b.CreateAPI(context.Background(), apigatewayv2.CreateAPIInput{Name: "a", ProtocolType: "HTTP"})
	require.NoError(t, err)

	_, err = b.CreateStage(api.APIID, apigatewayv2.CreateStageInput{
		StageName: "prod",
		RouteSettings: map[string]apigatewayv2.RouteSettings{
			"GET /items": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.EnforceRouteThrottle(api.APIID, "prod", "GET /items"))
	require.ErrorIs(t, b.EnforceRouteThrottle(api.APIID, "prod", "GET /items"), apigatewayv2.ErrThrottled,
		"the burst-1 bucket must be exhausted after one request")

	require.NoError(t, b.DeleteStage(api.APIID, "prod"))

	_, err = b.CreateStage(api.APIID, apigatewayv2.CreateStageInput{
		StageName: "prod",
		RouteSettings: map[string]apigatewayv2.RouteSettings{
			"GET /items": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.EnforceRouteThrottle(api.APIID, "prod", "GET /items"),
		"a recreated stage must not inherit a deleted stage's exhausted throttle bucket")
}

// TestBackend_DeleteRoute_ClearsRouteThrottleBucket proves DeleteRoute evicts
// the deleted route's throttle bucket across every stage of the API, so a new
// route reusing the same routeKey starts fresh.
func TestBackend_DeleteRoute_ClearsRouteThrottleBucket(t *testing.T) {
	t.Parallel()

	b := apigatewayv2.NewInMemoryBackend()
	api, err := b.CreateAPI(context.Background(), apigatewayv2.CreateAPIInput{Name: "a", ProtocolType: "HTTP"})
	require.NoError(t, err)

	_, err = b.CreateStage(api.APIID, apigatewayv2.CreateStageInput{
		StageName: "prod",
		RouteSettings: map[string]apigatewayv2.RouteSettings{
			"GET /items": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
		},
	})
	require.NoError(t, err)

	route, err := b.CreateRoute(api.APIID, apigatewayv2.CreateRouteInput{RouteKey: "GET /items"})
	require.NoError(t, err)

	require.NoError(t, b.EnforceRouteThrottle(api.APIID, "prod", "GET /items"))
	require.ErrorIs(t, b.EnforceRouteThrottle(api.APIID, "prod", "GET /items"), apigatewayv2.ErrThrottled)

	require.NoError(t, b.DeleteRoute(api.APIID, route.RouteID))

	_, err = b.CreateRoute(api.APIID, apigatewayv2.CreateRouteInput{RouteKey: "GET /items"})
	require.NoError(t, err)

	require.NoError(t, b.EnforceRouteThrottle(api.APIID, "prod", "GET /items"),
		"a re-created route must not inherit the deleted route's exhausted throttle bucket")
}

// TestBackend_UpdateRoute_RenameClearsOldRouteThrottleBucket proves renaming a
// route's routeKey evicts the throttle bucket keyed by the old routeKey, so a
// later route created under that freed key doesn't inherit stale state.
func TestBackend_UpdateRoute_RenameClearsOldRouteThrottleBucket(t *testing.T) {
	t.Parallel()

	b := apigatewayv2.NewInMemoryBackend()
	api, err := b.CreateAPI(context.Background(), apigatewayv2.CreateAPIInput{Name: "a", ProtocolType: "HTTP"})
	require.NoError(t, err)

	_, err = b.CreateStage(api.APIID, apigatewayv2.CreateStageInput{
		StageName: "prod",
		RouteSettings: map[string]apigatewayv2.RouteSettings{
			"GET /items": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
		},
	})
	require.NoError(t, err)

	route, err := b.CreateRoute(api.APIID, apigatewayv2.CreateRouteInput{RouteKey: "GET /items"})
	require.NoError(t, err)

	require.NoError(t, b.EnforceRouteThrottle(api.APIID, "prod", "GET /items"))
	require.ErrorIs(t, b.EnforceRouteThrottle(api.APIID, "prod", "GET /items"), apigatewayv2.ErrThrottled)

	_, err = b.UpdateRoute(api.APIID, route.RouteID, apigatewayv2.UpdateRouteInput{RouteKey: "GET /renamed"})
	require.NoError(t, err)

	_, err = b.CreateRoute(api.APIID, apigatewayv2.CreateRouteInput{RouteKey: "GET /items"})
	require.NoError(t, err)

	require.NoError(t, b.EnforceRouteThrottle(api.APIID, "prod", "GET /items"),
		"a route created under a freed routeKey must not inherit the renamed route's exhausted bucket")
}
