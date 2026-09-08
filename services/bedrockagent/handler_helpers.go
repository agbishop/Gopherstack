package bedrockagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

func handleErr(c *echo.Context, err error) error {
	var syntaxErr *json.SyntaxError

	var code string
	var status int

	switch {
	case errors.Is(err, awserr.ErrNotFound):
		status = http.StatusNotFound
		code = "ResourceNotFoundException"
	case errors.Is(err, awserr.ErrAlreadyExists), errors.Is(err, awserr.ErrConflict):
		status = http.StatusConflict
		code = "ConflictException"
	case errors.Is(err, awserr.ErrInvalidParameter):
		status = http.StatusBadRequest
		code = "ValidationException"
	case errors.As(err, &syntaxErr):
		status = http.StatusBadRequest
		code = "ValidationException"
	default:
		status = http.StatusInternalServerError
		code = "InternalServerException"
	}

	c.Response().Header().Set("X-Amzn-Errortype", code)

	return c.JSON(status, map[string]any{"message": err.Error()})
}

func errResp(code, msg string) map[string]any {
	return map[string]any{"__type": code, "message": msg}
}

func pageParams(query url.Values) (int, string) {
	maxResults := maxPageDefault
	nextToken := query.Get(keyNextToken)

	if mr := query.Get("maxResults"); mr != "" {
		_, _ = fmt.Sscanf(mr, "%d", &maxResults)
	}

	return maxResults, nextToken
}

// bodyPageParams reads maxResults/nextToken from a List op's JSON request
// body. Most List operations here bind them to the body, not the query
// string (confirmed per-op against aws-sdk-go-v2/service/bedrockagent's
// serializers.go httpBindings functions) -- unlike ListFlows/ListFlowVersions/
// ListFlowAliases/ListPrompts, which really do bind them as query params
// (those keep using pageParams).
func bodyPageParams(body []byte) (int, string) {
	var req struct {
		NextToken  string `json:"nextToken"`
		MaxResults int    `json:"maxResults"`
	}

	if len(body) > 0 {
		_ = json.Unmarshal(body, &req)
	}

	maxResults := maxPageDefault
	if req.MaxResults > 0 {
		maxResults = req.MaxResults
	}

	return maxResults, req.NextToken
}

// classifyPath returns the operation name from method+path (used by ExtractOperation).

func classifyPath(method, path string) string {
	path = strings.TrimSuffix(path, "/")

	switch {
	case path == agentsBase && method == http.MethodPut:
		return opCreateAgent
	case path == agentsBase:
		return opListAgents
	case path == kbBase && method == http.MethodPut:
		return opCreateKnowledgeBase
	case path == kbBase:
		return opListKnowledgeBases
	case path == flowsBase && method == http.MethodPost:
		return opCreateFlow
	case path == flowsBase:
		return opListFlows
	case path == promptsBase && method == http.MethodPost:
		return opCreatePrompt
	case path == promptsBase:
		return opListPrompts
	}

	return classifySubPath(method, path)
}

func classifySubPath(method, path string) string {
	switch {
	case strings.HasPrefix(path, agentsBase+"/"):
		return classifyAgentPath(method, path)
	case strings.HasPrefix(path, kbBase+"/"):
		return classifyKBPath(method, path)
	case strings.HasPrefix(path, flowsBase+"/"):
		return classifyFlowPath(method, path)
	case strings.HasPrefix(path, promptsBase+"/"):
		return classifyPromptPath(method, path)
	case strings.HasPrefix(path, tagsBase):
		return classifyTagPath(method)
	case strings.HasPrefix(path, resourcePolicyBase):
		return classifyResourcePolicyPath(method, path)
	}

	return opUnknown
}

// classifyAgentVersionedSubPath handles the collaborator, agentKB, alias, and actiongroup cases.
func classifyAgentVersionedSubPath(method string, segs []string) string {
	switch {
	case containsSeg(segs, "actiongroups"):
		return classifyActionGroupPath(method, segs)
	case containsSeg(segs, "agentcollaborators"):
		return classifyCollabPath(method, segs)
	case containsSeg(segs, "knowledgebases"):
		return classifyAgentKBPath(method, segs)
	default:
		return classifyAgentVersionPath(method, segs)
	}
}

func containsSeg(segs []string, seg string) bool {
	return slices.Contains(segs, seg)
}

func indexOf(segs []string, seg string) int {
	for i, s := range segs {
		if s == seg {
			return i
		}
	}

	return -1
}
