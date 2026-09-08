package mwaa

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

func (h *Handler) handlePublishMetrics(c *echo.Context, name string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "ValidationException", "failed to read request body")
	}

	var req publishMetricsRequest

	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(c, http.StatusBadRequest, "ValidationException", "invalid request body")
	}

	if pubErr := h.Backend.PublishMetrics(h.contextWithRegion(c), name, &req); pubErr != nil {
		// PublishMetrics's deserializer switch recognizes only
		// InternalServerException and ValidationException -- alone among the
		// not-found-capable ops here, it has no ResourceNotFoundException.
		if errors.Is(pubErr, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusBadRequest, "ValidationException", pubErr.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerException", pubErr.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, map[string]any{})

	return nil
}
