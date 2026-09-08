package mediaconvert

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"
)

// --- Resource share handlers ---

type createResourceShareInput struct {
	JobID         string `json:"jobId"`
	SupportCaseID string `json:"supportCaseId"`
}

func (h *Handler) handleCreateResourceShare(c *echo.Context, body []byte) error {
	var in createResourceShareInput
	if err := json.Unmarshal(body, &in); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", "invalid request body"))
	}

	if in.JobID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", "jobId is required"))
	}

	if in.SupportCaseID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", "supportCaseId is required"))
	}

	if _, err := h.Backend.CreateResourceShare(in.JobID, in.SupportCaseID); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}
