package bedrockagent

import (
	"context"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
)

// ---------------------------------------------------------------------------
// Agent version handlers
// ---------------------------------------------------------------------------

func (h *Handler) handleGetAgentVersion(
	ctx context.Context, c *echo.Context, agentID, version string,
) error {
	av, err := h.Backend.GetAgentVersion(ctx, agentID, version)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyAgentVersion: av})
}

func (h *Handler) handleDeleteAgentVersion(
	ctx context.Context, c *echo.Context, agentID, version string,
) error {
	skip, _ := strconv.ParseBool(c.QueryParam("skipResourceInUseCheck"))

	if err := h.Backend.DeleteAgentVersion(ctx, agentID, version, skip); err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyAgentID:      agentID,
		keyAgentVersion: version,
		keyAgentStatus:  statusDeleting,
	})
}

func (h *Handler) handleListAgentVersions(
	ctx context.Context, c *echo.Context, agentID string, body []byte,
) error {
	maxResults, nextToken := bodyPageParams(body)

	summaries, outToken, err := h.Backend.ListAgentVersions(ctx, agentID, maxResults, nextToken)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"agentVersionSummaries": summaries,
		keyNextToken:            outToken,
	})
}

func classifyAgentVersionPath(method string, segs []string) string {
	idx := indexOf(segs, "agentversions")
	hasVersionID := len(segs) > idx+1 && segs[idx+1] != ""

	// No CreateAgentVersion case: it is not a real bedrockagent SDK
	// operation. ListAgentVersions is a POST to this collection path.
	if !hasVersionID {
		switch method {
		case http.MethodPost, http.MethodGet:
			return opListAgentVersions
		}
	}

	switch method {
	case http.MethodGet:
		return opGetAgentVersion
	case http.MethodDelete:
		return opDeleteAgentVersion
	}

	return opUnknown
}
