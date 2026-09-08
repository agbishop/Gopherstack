package bedrock

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
)

func (h *AgentsHandler) handleCreateAgent(c *echo.Context, body []byte) error {
	var req struct {
		Tags                   map[string]string `json:"tags"`
		GuardrailConfiguration map[string]any    `json:"guardrailConfiguration"`
		MemoryConfiguration    map[string]any    `json:"memoryConfiguration"`
		AgentName              string            `json:"agentName"`
		AgentCollaboration     string            `json:"agentCollaboration"`
		Description            string            `json:"description"`
		FoundationModel        string            `json:"foundationModel"`
		Instruction            string            `json:"instruction"`
		AgentResourceRole      string            `json:"agentResourceRoleArn"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return c.JSON(
			http.StatusBadRequest,
			agentErrResp("ValidationException", "invalid request body"),
		)
	}

	ag, err := h.Backend.CreateAgentWithConfiguration(AgentConfiguration{
		Tags:                   req.Tags,
		GuardrailConfiguration: req.GuardrailConfiguration,
		MemoryConfiguration:    req.MemoryConfiguration,
		AgentName:              req.AgentName,
		AgentCollaboration:     req.AgentCollaboration,
		Description:            req.Description,
		FoundationModel:        req.FoundationModel,
		Instruction:            req.Instruction,
		RoleArn:                req.AgentResourceRole,
	})
	if err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			return c.JSON(http.StatusConflict, agentErrResp("ConflictException", err.Error()))
		}

		return c.JSON(http.StatusBadRequest, agentErrResp("ValidationException", err.Error()))
	}

	return c.JSON(http.StatusAccepted, map[string]any{respAgent: ag})
}

func (h *AgentsHandler) handleGetAgent(c *echo.Context, agentID string) error {
	ag, err := h.Backend.GetAgent(agentID)
	if err != nil {
		return c.JSON(http.StatusNotFound, agentErrResp("ResourceNotFoundException", err.Error()))
	}

	return c.JSON(http.StatusOK, map[string]any{respAgent: ag})
}

func (h *AgentsHandler) handleListAgents(c *echo.Context) error {
	list, outToken := h.Backend.ListAgents(0, c.QueryParam("nextToken"))
	resp := map[string]any{"agentSummaries": list}

	if outToken != "" {
		resp["nextToken"] = outToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *AgentsHandler) handleUpdateAgent(c *echo.Context, agentID string, body []byte) error {
	var req struct {
		GuardrailConfiguration map[string]any `json:"guardrailConfiguration"`
		MemoryConfiguration    map[string]any `json:"memoryConfiguration"`
		AgentName              string         `json:"agentName"`
		AgentCollaboration     string         `json:"agentCollaboration"`
		Description            string         `json:"description"`
		FoundationModel        string         `json:"foundationModel"`
		Instruction            string         `json:"instruction"`
		AgentResourceRole      string         `json:"agentResourceRoleArn"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return c.JSON(
			http.StatusBadRequest,
			agentErrResp("ValidationException", "invalid request body"),
		)
	}

	ag, err := h.Backend.UpdateAgentWithConfiguration(agentID, AgentConfiguration{
		GuardrailConfiguration: req.GuardrailConfiguration,
		MemoryConfiguration:    req.MemoryConfiguration,
		AgentName:              req.AgentName,
		AgentCollaboration:     req.AgentCollaboration,
		Description:            req.Description,
		FoundationModel:        req.FoundationModel,
		Instruction:            req.Instruction,
		RoleArn:                req.AgentResourceRole,
	})
	if err != nil {
		return c.JSON(http.StatusNotFound, agentErrResp("ResourceNotFoundException", err.Error()))
	}

	return c.JSON(http.StatusOK, map[string]any{respAgent: ag})
}

func (h *AgentsHandler) handleDeleteAgent(c *echo.Context, agentID string) error {
	skipResourceInUseCheck, _ := strconv.ParseBool(c.QueryParam("skipResourceInUseCheck"))

	if err := h.Backend.DeleteAgent(agentID, skipResourceInUseCheck); err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			return c.JSON(http.StatusConflict, agentErrResp("ConflictException", err.Error()))
		}

		return c.JSON(http.StatusNotFound, agentErrResp("ResourceNotFoundException", err.Error()))
	}

	return c.JSON(
		http.StatusAccepted,
		map[string]any{keyAgentID: agentID, opAgentStatusKey: statusDeleting},
	)
}

func (h *AgentsHandler) handlePrepareAgent(c *echo.Context, agentID string) error {
	ag, err := h.Backend.PrepareAgent(agentID)
	if err != nil {
		return c.JSON(http.StatusNotFound, agentErrResp("ResourceNotFoundException", err.Error()))
	}

	return c.JSON(
		http.StatusAccepted,
		map[string]any{
			keyAgentID:       ag.AgentID,
			opAgentStatusKey: ag.AgentStatus,
			"agentVersion":   ag.AgentVersion,
		},
	)
}

// dispatchAgentVersionRoutes handles /agents/{agentId}/versions/...
func (h *AgentsHandler) dispatchAgentVersionRoutes(
	c *echo.Context, agentID, suffix, method string,
) error {
	if suffix == suffixVersions && method == http.MethodGet {
		return h.handleListAgentVersions(c, agentID)
	}

	if suffix == suffixVersions && method == http.MethodPost {
		return h.handleCreateAgentVersion(c, agentID)
	}

	if ver, ok := strings.CutPrefix(suffix, suffixVersions+"/"); ok {
		switch method {
		case http.MethodGet:
			return h.handleGetAgentVersion(c, agentID, ver)
		case http.MethodDelete:
			return h.handleDeleteAgentVersion(c, agentID, ver)
		}
	}

	return c.JSON(
		http.StatusNotFound,
		agentErrResp("UnknownOperationException", "unknown agent version operation"),
	)
}

func (h *AgentsHandler) dispatchCanonicalAgentVersionRoutes(
	c *echo.Context, agentID, suffix, method string,
) error {
	// ListAgentVersions is real bedrock-agent@v1.58.4 serializers.go:4535:
	// POST /agents/{id}/agentversions/. GET is accepted too as harmless
	// extra leniency for this package's own tests.
	if suffix == "/agentversions" && (method == http.MethodPost || method == http.MethodGet) {
		return h.handleListAgentVersions(c, agentID)
	}

	version, ok := strings.CutPrefix(suffix, "/agentversions/")
	if !ok {
		return c.JSON(http.StatusNotFound, agentErrResp("UnknownOperationException", "unknown agent version operation"))
	}

	if version == agentStatusDraft && method == http.MethodPost {
		return h.handlePrepareAgent(c, agentID)
	}
	if method == http.MethodGet {
		return h.handleGetAgentVersion(c, agentID, version)
	}
	if method == http.MethodDelete {
		return h.handleDeleteAgentVersion(c, agentID, version)
	}

	return c.JSON(http.StatusNotFound, agentErrResp("UnknownOperationException", "unknown agent version operation"))
}

// dispatchTagRoutes handles /tags/{resourceArn} REST paths.
func (h *AgentsHandler) dispatchTagRoutes(
	c *echo.Context, path, method string, body []byte,
) error {
	resourceArn, ok := strings.CutPrefix(path, "/tags/")
	if !ok || resourceArn == "" {
		return c.JSON(
			http.StatusBadRequest,
			agentErrResp("ValidationException", "missing resourceArn in path"),
		)
	}

	switch method {
	case http.MethodGet:
		return h.handleListTagsForAgentResource(c, resourceArn)
	case http.MethodPost:
		return h.handleTagAgentResource(c, resourceArn, body)
	case http.MethodDelete:
		return h.handleUntagAgentResource(c, resourceArn, body)
	}

	return c.JSON(
		http.StatusNotFound,
		agentErrResp("UnknownOperationException", "unknown tag operation"),
	)
}

// dispatchMemoryRoutes handles /agents/{agentId}/agentversions/{v}/memories/...
func (h *AgentsHandler) dispatchMemoryRoutes(
	c *echo.Context, agentID, suffix, method string,
) error {
	// Extract session from query param or simple path
	sessionID := c.QueryParam("sessionId")

	if sessionID == "" {
		// Try parsing /memories/{sessionId}
		if rest, ok := strings.CutPrefix(suffix, "/memories/"); ok {
			sessionID = rest
		}
	}

	switch method {
	case http.MethodGet:
		return h.handleGetAgentMemory(c, agentID, sessionID)
	case http.MethodDelete:
		return h.handleDeleteAgentMemory(c, agentID, sessionID)
	}

	return c.JSON(
		http.StatusNotFound,
		agentErrResp("UnknownOperationException", "unknown memory operation"),
	)
}

func (h *AgentsHandler) handleCreateAgentVersion(c *echo.Context, agentID string) error {
	av, err := h.Backend.CreateAgentVersion(agentID)
	if err != nil {
		return c.JSON(http.StatusNotFound, agentErrResp("ResourceNotFoundException", err.Error()))
	}

	return c.JSON(http.StatusAccepted, map[string]any{respAgentVersion: av})
}

func (h *AgentsHandler) handleGetAgentVersion(
	c *echo.Context, agentID, version string,
) error {
	av, err := h.Backend.GetAgentVersion(agentID, version)
	if err != nil {
		return c.JSON(http.StatusNotFound, agentErrResp("ResourceNotFoundException", err.Error()))
	}

	return c.JSON(http.StatusOK, map[string]any{respAgentVersion: av})
}

func (h *AgentsHandler) handleListAgentVersions(c *echo.Context, agentID string) error {
	list, outToken := h.Backend.ListAgentVersions(agentID, 0, c.QueryParam("nextToken"))
	resp := map[string]any{"agentVersionSummaries": list}

	if outToken != "" {
		resp["nextToken"] = outToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *AgentsHandler) handleDeleteAgentVersion(
	c *echo.Context, agentID, version string,
) error {
	if err := h.Backend.DeleteAgentVersion(agentID, version); err != nil {
		return c.JSON(http.StatusNotFound, agentErrResp("ResourceNotFoundException", err.Error()))
	}

	return c.JSON(
		http.StatusAccepted,
		map[string]any{keyAgentID: agentID, keyVersion: version, opAgentStatusKey: statusDeleting},
	)
}

func (h *AgentsHandler) handleListTagsForAgentResource(
	c *echo.Context, resourceArn string,
) error {
	tags := h.Backend.ListAgentResourceTags(resourceArn)

	return c.JSON(http.StatusOK, map[string]any{"tags": tags})
}

func (h *AgentsHandler) handleTagAgentResource(
	c *echo.Context, resourceArn string, body []byte,
) error {
	var req struct {
		Tags map[string]string `json:"tags"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return c.JSON(
			http.StatusBadRequest,
			agentErrResp("ValidationException", "invalid request body"),
		)
	}

	if err := h.Backend.TagAgentResource(resourceArn, req.Tags); err != nil {
		return c.JSON(http.StatusBadRequest, agentErrResp("ValidationException", err.Error()))
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *AgentsHandler) handleUntagAgentResource(
	c *echo.Context, resourceArn string, body []byte,
) error {
	var req struct {
		TagKeys []string `json:"tagKeys"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return c.JSON(
			http.StatusBadRequest,
			agentErrResp("ValidationException", "invalid request body"),
		)
	}

	if err := h.Backend.UntagAgentResource(resourceArn, req.TagKeys); err != nil {
		return c.JSON(http.StatusBadRequest, agentErrResp("ValidationException", err.Error()))
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *AgentsHandler) handleGetAgentMemory(
	c *echo.Context, agentID, sessionID string,
) error {
	entries := h.Backend.GetAgentMemory(agentID, sessionID)

	return c.JSON(http.StatusOK, map[string]any{"memoryContents": entries})
}

func (h *AgentsHandler) handleDeleteAgentMemory(
	c *echo.Context, agentID, sessionID string,
) error {
	if err := h.Backend.DeleteAgentMemory(agentID, sessionID); err != nil {
		return c.JSON(http.StatusNotFound, agentErrResp("ResourceNotFoundException", err.Error()))
	}

	return c.NoContent(http.StatusNoContent)
}
