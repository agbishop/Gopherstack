package pinpoint

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// handleRemoveAttributes handles PUT /v1/apps/{appId}/attributes/{attributeType}.
func (h *Handler) handleRemoveAttributes(c *echo.Context, appID, attributeType string) error {
	body, _ := httputils.ReadBody(c.Request())

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req removeAttributesRequest
	if len(body) > 0 {
		if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
			return writeErrorResponse(c, http.StatusBadRequest, "BadRequestException", "invalid request body")
		}
	}

	resp, err := h.Backend.RemoveAttributes(appID, attributeType, req.Blacklist)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(c, http.StatusInternalServerError, "InternalServerErrorException", err.Error())
	}

	httputils.WriteJSON(c.Request().Context(), c.Response(), http.StatusOK, resp)

	return nil
}
