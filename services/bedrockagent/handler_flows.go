package bedrockagent

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
)

// ---------------------------------------------------------------------------
// Flow handlers
// ---------------------------------------------------------------------------

func (h *Handler) handleCreateFlow(ctx context.Context, c *echo.Context, body []byte) error {
	var req struct {
		Tags        map[string]string `json:"tags"`
		Definition  map[string]any    `json:"definition"`
		Name        string            `json:"name"`
		Description string            `json:"description"`
		RoleARN     string            `json:"executionRoleArn"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return handleErr(c, err)
	}

	f, err := h.Backend.CreateFlow(ctx, FlowConfig{
		Name:        req.Name,
		Description: req.Description,
		RoleARN:     req.RoleARN,
		Definition:  req.Definition,
		Tags:        req.Tags,
	})
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusCreated, f)
}

func (h *Handler) handleGetFlow(ctx context.Context, c *echo.Context, flowID string) error {
	f, err := h.Backend.GetFlow(ctx, flowID)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, f)
}

func (h *Handler) handleUpdateFlow(
	ctx context.Context, c *echo.Context, flowID string, body []byte,
) error {
	var req struct {
		Tags        map[string]string `json:"tags"`
		Definition  map[string]any    `json:"definition"`
		Name        string            `json:"name"`
		Description string            `json:"description"`
		RoleARN     string            `json:"executionRoleArn"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return handleErr(c, err)
	}

	f, err := h.Backend.UpdateFlow(ctx, flowID, FlowConfig{
		Name:        req.Name,
		Description: req.Description,
		RoleARN:     req.RoleARN,
		Definition:  req.Definition,
		Tags:        req.Tags,
	})
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, f)
}

func (h *Handler) handleDeleteFlow(ctx context.Context, c *echo.Context, flowID string) error {
	if err := h.Backend.DeleteFlow(ctx, flowID); err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"id": flowID})
}

func (h *Handler) handleListFlows(ctx context.Context, c *echo.Context) error {
	maxResults, nextToken := pageParams(c.Request().URL.Query())

	summaries, outToken, err := h.Backend.ListFlows(ctx, maxResults, nextToken)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"flowSummaries": summaries, keyNextToken: outToken})
}

func (h *Handler) handlePrepareFlow(ctx context.Context, c *echo.Context, flowID string) error {
	f, err := h.Backend.PrepareFlow(ctx, flowID)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusAccepted, f)
}

func (h *Handler) handleValidateFlowDef(ctx context.Context, c *echo.Context, body []byte) error {
	var req struct {
		Definition map[string]any `json:"definition"`
	}

	_ = json.Unmarshal(body, &req)

	errs, err := h.Backend.ValidateFlowDefinition(ctx, req.Definition)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"validations": errs})
}

// ---------------------------------------------------------------------------
// Flow version handlers
// ---------------------------------------------------------------------------

func (h *Handler) handleCreateFlowVersion(
	ctx context.Context, c *echo.Context, flowID string, body []byte,
) error {
	var req struct {
		Description string `json:"description"`
	}

	_ = json.Unmarshal(body, &req)

	fv, err := h.Backend.CreateFlowVersion(ctx, flowID, req.Description)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusCreated, fv)
}

func (h *Handler) handleGetFlowVersion(
	ctx context.Context, c *echo.Context, flowID, flowVersion string,
) error {
	fv, err := h.Backend.GetFlowVersion(ctx, flowID, flowVersion)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, fv)
}

func (h *Handler) handleDeleteFlowVersion(
	ctx context.Context, c *echo.Context, flowID, flowVersion string,
) error {
	skip, _ := strconv.ParseBool(c.QueryParam("skipResourceInUseCheck"))

	if err := h.Backend.DeleteFlowVersion(ctx, flowID, flowVersion, skip); err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"id": flowID, "version": flowVersion})
}

func (h *Handler) handleListFlowVersions(
	ctx context.Context, c *echo.Context, flowID string,
) error {
	maxResults, nextToken := pageParams(c.Request().URL.Query())

	summaries, outToken, err := h.Backend.ListFlowVersions(ctx, flowID, maxResults, nextToken)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"flowVersionSummaries": summaries, keyNextToken: outToken})
}

// ---------------------------------------------------------------------------
// Flow alias handlers
// ---------------------------------------------------------------------------

func (h *Handler) handleCreateFlowAlias(
	ctx context.Context, c *echo.Context, flowID string, body []byte,
) error {
	var req struct {
		Tags                 map[string]string  `json:"tags"`
		Name                 string             `json:"name"`
		Description          string             `json:"description"`
		RoutingConfiguration []FlowAliasRouting `json:"routingConfiguration"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return handleErr(c, err)
	}

	al, err := h.Backend.CreateFlowAlias(ctx, flowID, FlowAliasConfig{
		Name:                 req.Name,
		Description:          req.Description,
		RoutingConfiguration: req.RoutingConfiguration,
		Tags:                 req.Tags,
	})
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusCreated, al)
}

func (h *Handler) handleGetFlowAlias(
	ctx context.Context, c *echo.Context, flowID, aliasID string,
) error {
	al, err := h.Backend.GetFlowAlias(ctx, flowID, aliasID)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, al)
}

func (h *Handler) handleUpdateFlowAlias(
	ctx context.Context, c *echo.Context, flowID, aliasID string, body []byte,
) error {
	var req struct {
		Tags                 map[string]string  `json:"tags"`
		Name                 string             `json:"name"`
		Description          string             `json:"description"`
		RoutingConfiguration []FlowAliasRouting `json:"routingConfiguration"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return handleErr(c, err)
	}

	al, err := h.Backend.UpdateFlowAlias(ctx, flowID, aliasID, FlowAliasConfig{
		Name:                 req.Name,
		Description:          req.Description,
		RoutingConfiguration: req.RoutingConfiguration,
		Tags:                 req.Tags,
	})
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, al)
}

func (h *Handler) handleDeleteFlowAlias(
	ctx context.Context, c *echo.Context, flowID, aliasID string,
) error {
	if err := h.Backend.DeleteFlowAlias(ctx, flowID, aliasID); err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"id": aliasID, "flowId": flowID})
}

func (h *Handler) handleListFlowAliases(
	ctx context.Context, c *echo.Context, flowID string,
) error {
	maxResults, nextToken := pageParams(c.Request().URL.Query())

	summaries, outToken, err := h.Backend.ListFlowAliases(ctx, flowID, maxResults, nextToken)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{"flowAliasSummaries": summaries, keyNextToken: outToken})
}

func classifyFlowPath(method, path string) string {
	rest, _ := strings.CutPrefix(path, flowsBase+"/")
	segs := strings.Split(rest, "/")

	// ValidateFlowDefinition POSTs to the literal "/flows/validate-definition"
	// path, not a "/flows/{flowIdentifier}/" -- same single-segment,
	// POST shape as PrepareFlow's real wire request, so it must be checked
	// first or it misclassifies as PrepareFlow. dispatchFlows (handler.go)
	// already special-cases this literal path ahead of its own
	// flowID/suffix parsing; this mirrors that ordering.
	if rest == "validate-definition" && method == http.MethodPost {
		return opValidateFlowDefinition
	}

	switch {
	case len(segs) == 1 && method == http.MethodGet:
		return opGetFlow
	case len(segs) == 1 && method == http.MethodPut:
		return opUpdateFlow
	case len(segs) == 1 && method == http.MethodDelete:
		return opDeleteFlow
	// Real PrepareFlow POSTs to "/flows/{flowIdentifier}/" -- no "/prepare"
	// suffix (botocore bedrock-agent 2023-06-05) -- so it's a single
	// segment, same as Get/Update/Delete, disambiguated by method alone.
	case len(segs) == 1 && method == http.MethodPost:
		return opPrepareFlow
	case containsSeg(segs, "versions"):
		return classifyFlowVersionPath(method, segs)
	case containsSeg(segs, "aliases"):
		return classifyFlowAliasPath(method, segs)
	}

	return opUnknown
}

func classifyFlowVersionPath(method string, segs []string) string {
	idx := indexOf(segs, "versions")
	hasID := len(segs) > idx+1 && segs[idx+1] != ""

	if !hasID {
		switch method {
		case http.MethodPost:
			return opCreateFlowVersion
		case http.MethodGet:
			return opListFlowVersions
		}
	}

	switch method {
	case http.MethodGet:
		return opGetFlowVersion
	case http.MethodDelete:
		return opDeleteFlowVersion
	}

	return opUnknown
}

func classifyFlowAliasPath(method string, segs []string) string {
	idx := indexOf(segs, "aliases")
	hasID := len(segs) > idx+1 && segs[idx+1] != ""

	if !hasID {
		switch method {
		case http.MethodPost:
			return opCreateFlowAlias
		case http.MethodGet:
			return opListFlowAliases
		}
	}

	switch method {
	case http.MethodGet:
		return opGetFlowAlias
	case http.MethodPut:
		return opUpdateFlowAlias
	case http.MethodDelete:
		return opDeleteFlowAlias
	}

	return opUnknown
}
