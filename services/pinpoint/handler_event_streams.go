package pinpoint

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// extractEventStreamOp returns the event stream operation name.
func extractEventStreamOp(method string) string {
	switch method {
	case http.MethodGet:
		return "GetEventStream"
	case http.MethodPost:
		return "PutEventStream"
	case http.MethodDelete:
		return "DeleteEventStream"
	}

	return unknownOperation
}

func (h *Handler) dispatchEventStream(c *echo.Context, appID string) error {
	switch c.Request().Method {
	case http.MethodGet:
		return h.handleGetEventStream(c, appID)
	case http.MethodPost:
		return h.handlePutEventStream(c, appID)
	case http.MethodDelete:
		return h.handleDeleteEventStream(c, appID)
	}

	return writeErrorResponse(c, http.StatusMethodNotAllowed, "MethodNotAllowedException", "method not allowed")
}

// handleGetEventStream handles GET /v1/apps/{appId}/eventstream.
func (h *Handler) handleGetEventStream(c *echo.Context, appID string) error {
	es, err := h.Backend.GetEventStream(appID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, eventStreamResponse{
		ApplicationID:        es.ApplicationID,
		DestinationStreamArn: es.DestinationStreamArn,
		RoleArn:              es.RoleArn,
		LastModifiedDate:     es.LastModifiedDate,
	})

	return nil
}

// handlePutEventStream handles POST /v1/apps/{appId}/eventstream.
func (h *Handler) handlePutEventStream(c *echo.Context, appID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "failed to read request body")
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req putEventStreamRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
	}

	es, backendErr := h.Backend.PutEventStream(appID, req)
	if backendErr != nil {
		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", backendErr.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, eventStreamResponse{
		ApplicationID:        es.ApplicationID,
		DestinationStreamArn: es.DestinationStreamArn,
		RoleArn:              es.RoleArn,
		LastModifiedDate:     es.LastModifiedDate,
	})

	return nil
}

// handleDeleteEventStream handles DELETE /v1/apps/{appId}/eventstream.
func (h *Handler) handleDeleteEventStream(c *echo.Context, appID string) error {
	es, err := h.Backend.DeleteEventStream(appID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, eventStreamResponse{
		ApplicationID:        es.ApplicationID,
		DestinationStreamArn: es.DestinationStreamArn,
		RoleArn:              es.RoleArn,
	})

	return nil
}

// ──────────────────────────────────────────────────
// Messaging / analytics handlers
// ──────────────────────────────────────────────────
