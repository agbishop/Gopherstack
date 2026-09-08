package apigatewayv2

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

var (
	ErrUnsupportedType  = errors.New("unsupported integration type")
	ErrInvalidLambdaArn = errors.New("invalid lambda arn")
)

const (
	protocolTypeWebSocket = "WEBSOCKET"
	routeKeyDefault       = "$default"
)

// handleStageProxyEcho routes /v2proxy/{apiId}/{stageName}/{path} requests to the proxy handler.
func (h *Handler) handleStageProxyEcho(c *echo.Context) error {
	rest := strings.TrimPrefix(c.Request().URL.Path, "/v2proxy/")
	const minProxyPathParts = 2
	const splitCount = 3

	parts := strings.SplitN(rest, "/", splitCount)
	if len(parts) < minProxyPathParts {
		return c.String(http.StatusNotFound, "invalid proxy path")
	}

	apiID := parts[0]
	stageName := parts[1]

	resourcePath := "/"
	if len(parts) == splitCount && parts[2] != "" {
		resourcePath = "/" + parts[2]
	}

	return h.handleProxy(c, apiID, stageName, resourcePath)
}

// handleUserRequestEcho handles data-plane invocations at the standard AWS endpoint format:
// /restapis/{apiId}/{stageName}/_user_request_/{resourcePath...}.
func (h *Handler) handleUserRequestEcho(c *echo.Context) error {
	segs := strings.Split(strings.TrimPrefix(c.Request().URL.Path, "/"), "/")
	const (
		idxAPIID     = 1
		idxStageName = 2
		idxPathStart = 4
	)

	apiID := segs[idxAPIID]
	stageName := segs[idxStageName]

	resourcePath := "/"
	if len(segs) > idxPathStart && segs[idxPathStart] != "" {
		resourcePath = "/" + strings.Join(segs[idxPathStart:], "/")
	}

	return h.handleProxy(c, apiID, stageName, resourcePath)
}

// handleProxy performs the actual WebSocket or HTTP API routing.
func (h *Handler) handleProxy(c *echo.Context, apiID, stageName, resourcePath string) error {
	// 1. Get the API
	api, err := h.Backend.GetAPI(apiID)
	if err != nil {
		return c.String(http.StatusNotFound, "API not found")
	}

	// A request naming a stage that was never created has no deployed
	// snapshot to route against. Previously this handler never checked
	// stageName against the backend at all here, so any string in the URL's
	// stage-name slot -- including one that was never created via
	// CreateStage -- still routed through to the live integration.
	if _, stageErr := h.Backend.GetStage(apiID, stageName); stageErr != nil {
		return writeErr(c, http.StatusNotFound, msgNotFound)
	}

	// 2. Determine protocol
	protocol := api.ProtocolType
	switch protocol {
	case protocolTypeWebSocket:
		return h.handleWebSocketProxy(c, apiID)
	case protocolTypeHTTP:
		return h.handleHTTPAPIProxy(c, apiID, stageName, resourcePath)
	}

	return c.String(http.StatusNotImplemented, "unsupported protocol type: "+protocol)
}

func (h *Handler) handleWebSocketProxy(c *echo.Context, apiID string) error {
	log := logger.Load(c.Request().Context())

	// A WebSocket proxy requires a lambda invoker to handle $connect, $disconnect, etc.
	if h.lambdaInvoker == nil {
		return c.String(http.StatusInternalServerError, "Lambda invoker not configured")
	}

	// For WebSockets, the initial request must be an HTTP Upgrade.
	if !websocket.IsWebSocketUpgrade(c.Request()) {
		return c.String(http.StatusBadRequest, "request is not a WebSocket upgrade")
	}

	connectionID := uuid.New().String()

	// Route the $connect event
	err := h.invokeWSRoute(c, apiID, "$connect", connectionID, []byte{})
	if err != nil {
		log.Error("apigatewayv2: $connect route failed", "error", err)

		return c.String(http.StatusForbidden, "Forbidden")
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool {
			return true // Allow all origins for the local emulator
		},
	}

	// Upgrade the connection
	conn, upgradeErr := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if upgradeErr != nil {
		log.Error("apigatewayv2: websocket upgrade failed", "error", upgradeErr)

		return upgradeErr
	}

	const downstreamChanSize = 10
	downstream := make(chan []byte, downstreamChanSize)

	if h.managementAPI != nil {
		_, regErr := h.managementAPI.CreateConnection(connectionID, c.RealIP(), c.Request().UserAgent(), downstream)
		if regErr != nil {
			log.Error("apigatewayv2: failed to register connection", "error", regErr)
			_ = conn.Close()

			return regErr
		}
	}

	go func() {
		for msg := range downstream {
			writeErr := conn.WriteMessage(websocket.TextMessage, msg)
			if writeErr != nil {
				log.Info("apigatewayv2: websocket write error", "error", writeErr)

				break
			}
		}
		_ = conn.Close()
	}()

	h.wsReadLoop(c, conn, apiID, connectionID)

	if h.managementAPI != nil {
		_ = h.managementAPI.DeleteConnection(connectionID)
	}

	// Route the $disconnect event
	_ = h.invokeWSRoute(c, apiID, "$disconnect", connectionID, []byte{})

	return nil
}

// invokeWSRoute invokes the backend integration for a specific route.
func (h *Handler) wsReadLoop(c *echo.Context, conn *websocket.Conn, apiID, connectionID string) {
	log := logger.Load(c.Request().Context())

	for {
		msgType, msgBody, readErr := conn.ReadMessage()
		if readErr != nil {
			log.Info("apigatewayv2: websocket closed", "error", readErr)

			break
		}

		if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
			routeKey := routeKeyDefault

			var payload map[string]any
			if json.Unmarshal(msgBody, &payload) == nil {
				if action, ok := payload["action"].(string); ok && action != "" {
					routeKey = action
				}
			}

			_ = h.invokeWSRoute(c, apiID, routeKey, connectionID, msgBody)
		}
	}
}

func (h *Handler) invokeWSRoute(c *echo.Context, apiID, routeKey, connectionID string, body []byte) error {
	_, err := h.Backend.GetAPI(apiID)
	if err != nil {
		return err
	}

	routes, err := h.Backend.GetRoutes(apiID)
	if err != nil {
		return err
	}

	var matchedRoute *Route
	for _, r := range routes {
		if r.RouteKey == routeKey {
			matchedRoute = &r

			break
		}
	}

	if matchedRoute == nil {
		if routeKey == routeKeyDefault || routeKey == "$connect" || routeKey == "$disconnect" {
			return nil
		}

		return fmt.Errorf("%w: %s", ErrRouteNotFound, routeKey)
	}

	integrationID := strings.TrimPrefix(matchedRoute.Target, "integrations/")
	integration, err := h.Backend.GetIntegration(apiID, integrationID)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrIntegrationNotFound, integrationID)
	}

	// MOCK: "integrating the route or method request with API Gateway as a
	// 'loopback' endpoint without invoking any backend" (api_op_CreateIntegration.go).
	if integration.IntegrationType == integrationTypeMock {
		return nil
	}

	if integration.IntegrationType != IntegrationTypeAWSProxy {
		return fmt.Errorf("%w: %s", ErrUnsupportedType, integration.IntegrationType)
	}

	const expectedArnParts = 2
	parts := strings.Split(integration.IntegrationURI, "function:")
	if len(parts) < expectedArnParts {
		return fmt.Errorf("%w: %s", ErrInvalidLambdaArn, integration.IntegrationURI)
	}
	lambdaArn := strings.TrimSuffix(parts[1], "/invocations")

	// AWS API Gateway WebSocket AWS_PROXY payload format.
	wsEvent := map[string]any{
		"requestContext": map[string]any{
			"routeKey":     routeKey,
			"eventType":    "MESSAGE",
			"connectionId": connectionID,
			logKeyAPIID:    apiID,
		},
		"body": string(body),
	}

	payload, marshalErr := json.Marshal(wsEvent)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal WebSocket event: %w", marshalErr)
	}

	// 5. Invoke Lambda
	_, _, err = h.lambdaInvoker.InvokeFunction(c.Request().Context(), lambdaArn, "RequestResponse", payload)
	if err != nil {
		return fmt.Errorf("lambda invocation failed: %w", err)
	}

	return nil
}
