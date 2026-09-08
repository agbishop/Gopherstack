package apigatewayv2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubLambdaInvoker records whether it was called, for proving MOCK
// integrations never invoke a backend.
type stubLambdaInvoker struct {
	called bool
}

func (s *stubLambdaInvoker) InvokeFunction(
	_ context.Context, _, _ string, _ []byte,
) ([]byte, int, error) {
	s.called = true

	return []byte(`{"statusCode":200}`), 200, nil
}

// TestInvokeWSRoute_MockIntegrationIsLoopback proves a WebSocket route backed
// by a MOCK integration succeeds without invoking any backend, per
// api_op_CreateIntegration.go's IntegrationType doc comment: "MOCK: for
// integrating the route or method request with API Gateway as a 'loopback'
// endpoint without invoking any backend." Before this fix, invokeWSRoute only
// recognized AWS_PROXY and rejected every other type -- including MOCK, a
// genuinely valid WebSocket integration type -- with ErrUnsupportedType,
// which surfaces as a $connect route failing with 403 Forbidden.
func TestInvokeWSRoute_MockIntegrationIsLoopback(t *testing.T) {
	t.Parallel()

	h := NewHandler(NewInMemoryBackend())
	lambda := &stubLambdaInvoker{}
	h.SetLambdaInvoker(lambda)

	api, err := h.Backend.CreateAPI(context.Background(), CreateAPIInput{
		Name: "ws-mock-api", ProtocolType: protocolTypeWebSocket,
	})
	require.NoError(t, err)

	integ, err := h.Backend.CreateIntegration(api.APIID, CreateIntegrationInput{
		IntegrationType: integrationTypeMock,
	})
	require.NoError(t, err)

	_, err = h.Backend.CreateRoute(api.APIID, CreateRouteInput{
		RouteKey: "$connect",
		Target:   "integrations/" + integ.IntegrationID,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rr)

	err = h.invokeWSRoute(c, api.APIID, "$connect", "conn-1", []byte{})

	require.NoError(t, err)
	assert.False(t, lambda.called, "MOCK integration must not invoke a backend")
}
