package pinpoint

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

func (h *Handler) extractEndpointSubOp(method, rest string) string {
	parts := strings.SplitN(rest, "/", dispatchSplitTwo)
	subPath := ""

	if len(parts) == dispatchSplitTwo {
		subPath = parts[1]
	}

	if subPath == "inappmessages" {
		return "GetInAppMessages"
	}

	switch method {
	case http.MethodGet:
		return "GetEndpoint"
	case http.MethodPut:
		return "UpdateEndpoint"
	case http.MethodDelete:
		return "DeleteEndpoint"
	}

	return unknownOperation
}

// extractUserEndpointsOp returns the user endpoints operation name.
func extractUserEndpointsOp(method string) string {
	switch method {
	case http.MethodGet:
		return "GetUserEndpoints"
	case http.MethodDelete:
		return "DeleteUserEndpoints"
	}

	return unknownOperation
}

func (h *Handler) dispatchEndpointByID(c *echo.Context, appID, rest string) error {
	// rest: {endpointId} or {endpointId}/inappmessages
	parts := strings.SplitN(rest, "/", dispatchSplitTwo)
	endpointID := parts[0]
	subPath := ""

	if len(parts) == dispatchSplitTwo {
		subPath = parts[1]
	}

	if subPath == "inappmessages" {
		return h.handleGetInAppMessages(c, appID, endpointID)
	}

	switch c.Request().Method {
	case http.MethodGet:
		return h.handleGetEndpoint(c, appID, endpointID)
	case http.MethodPut:
		return h.handleUpdateEndpoint(c, appID, endpointID)
	case http.MethodDelete:
		return h.handleDeleteEndpoint(c, appID, endpointID)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

func (h *Handler) dispatchUserByID(c *echo.Context, appID, userID string) error {
	switch c.Request().Method {
	case http.MethodGet:
		return h.handleGetUserEndpoints(c, appID, userID)
	case http.MethodDelete:
		return h.handleDeleteUserEndpoints(c, appID, userID)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

// toEndpointResponse converts an Endpoint to its wire format.
func toEndpointResponse(e *Endpoint) endpointResponse {
	return endpointResponse{
		ApplicationID:  e.ApplicationID,
		ID:             e.ID,
		ChannelType:    e.ChannelType,
		Address:        e.Address,
		EffectiveDate:  e.EffectiveDate,
		CreationDate:   e.CreationDate,
		EndpointStatus: e.EndpointStatus,
		OptOut:         e.OptOut,
		RequestID:      e.RequestID,
		Attributes:     e.Attributes,
		Metrics:        e.Metrics,
		Demographic:    e.Demographic,
		Location:       e.Location,
		User: endpointUserResponse{
			UserID:         e.UserID,
			UserAttributes: e.UserAttributes,
		},
	}
}

// handleGetEndpoint handles GET /v1/apps/{appId}/endpoints/{endpointId}.
func (h *Handler) handleGetEndpoint(c *echo.Context, appID, endpointID string) error {
	e, err := h.Backend.GetEndpoint(appID, endpointID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toEndpointResponse(e))

	return nil
}

// handleUpdateEndpoint handles PUT /v1/apps/{appId}/endpoints/{endpointId}.
func (h *Handler) handleUpdateEndpoint(c *echo.Context, appID, endpointID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxEndpointRequestBytes) {
		return nil
	}

	var req updateEndpointRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
	}

	_, backendErr := h.Backend.UpdateEndpoint(appID, endpointID, req)
	if backendErr != nil {
		if errors.Is(backendErr, awserr.ErrInvalidParameter) {
			return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", backendErr.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", backendErr.Error())
	}

	httputils.WriteJSON(
		c.Request().Context(), c.Response(),
		http.StatusAccepted, messageBodyResponse{Message: acceptedMessage},
	)

	return nil
}

// handleDeleteEndpoint handles DELETE /v1/apps/{appId}/endpoints/{endpointId}.
func (h *Handler) handleDeleteEndpoint(c *echo.Context, appID, endpointID string) error {
	e, err := h.Backend.DeleteEndpoint(appID, endpointID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, toEndpointResponse(e))

	return nil
}

// handleGetUserEndpoints handles GET /v1/apps/{appId}/users/{userId}.
func (h *Handler) handleGetUserEndpoints(c *echo.Context, appID, userID string) error {
	endpoints, err := h.Backend.GetUserEndpoints(appID, userID)
	if err != nil {
		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	items := make([]endpointResponse, 0, len(endpoints))

	for _, e := range endpoints {
		items = append(items, toEndpointResponse(e))
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, endpointsResponse{Item: items})

	return nil
}

// handleDeleteUserEndpoints handles DELETE /v1/apps/{appId}/users/{userId}.
func (h *Handler) handleDeleteUserEndpoints(c *echo.Context, appID, userID string) error {
	deleted, err := h.Backend.DeleteUserEndpoints(appID, userID)
	if err != nil {
		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	items := make([]endpointResponse, 0, len(deleted))

	for _, e := range deleted {
		items = append(items, toEndpointResponse(e))
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, endpointsResponse{Item: items})

	return nil
}

// handleUpdateEndpointsBatch handles PUT /v1/apps/{appId}/endpoints.
func (h *Handler) handleUpdateEndpointsBatch(c *echo.Context, appID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req updateEndpointsBatchRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
	}

	if backendErr := h.Backend.UpdateEndpointsBatch(appID, req.Item); backendErr != nil {
		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", backendErr.Error())
	}

	httputils.WriteJSON(
		c.Request().Context(),
		c.Response(),
		http.StatusAccepted,
		messageBodyResponse{Message: acceptedMessage},
	)

	return nil
}

// ──────────────────────────────────────────────────
// EventStream handlers
// ──────────────────────────────────────────────────
