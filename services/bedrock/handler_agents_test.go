package bedrock_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentsHandler_Name(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)
	assert.Equal(t, "BedrockAgents", h.Name())
}

func TestAgentsHandler_ChaosServiceName(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)
	assert.Equal(t, "bedrock-agent", h.ChaosServiceName())
}

func TestAgentsHandler_ChaosOperations(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)
	assert.Equal(t, h.GetSupportedOperations(), h.ChaosOperations())
}

func TestAgentsHandler_ChaosRegions(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)
	assert.Equal(t, []string{"us-east-1"}, h.ChaosRegions())
}

func TestAgentsHandler_MatchPriority(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)
	assert.Positive(t, h.MatchPriority())
}

func TestAgentsHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/agents/agent-001", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	// ExtractResource always returns empty for AgentsHandler.
	assert.Empty(t, h.ExtractResource(c))
}

func TestAgentsHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)
	ops := h.GetSupportedOperations()

	for _, op := range []string{
		"CreateAgent", "GetAgent", "ListAgents", "UpdateAgent", "DeleteAgent", "PrepareAgent",
		"CreateAgentActionGroup", "GetAgentActionGroup", "ListAgentActionGroups",
		"UpdateAgentActionGroup", "DeleteAgentActionGroup",
		"CreateAgentAlias", "GetAgentAlias", "ListAgentAliases", "UpdateAgentAlias", "DeleteAgentAlias",
		"AssociateAgentKnowledgeBase", "DisassociateAgentKnowledgeBase",
		"GetAgentKnowledgeBase", "ListAgentKnowledgeBases",
		"CreateKnowledgeBase", "GetKnowledgeBase", "ListKnowledgeBases",
		"UpdateKnowledgeBase", "DeleteKnowledgeBase",
		"CreateDataSource", "GetDataSource", "ListDataSources", "UpdateDataSource", "DeleteDataSource",
		"StartIngestionJob", "GetIngestionJob", "ListIngestionJobs",
	} {
		assert.Contains(t, ops, op)
	}
}

func TestAgentsHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)
	e := echo.New()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"agents root", "/agents", true},
		{"agent with id", "/agents/agent-001", true},
		{"knowledgebases root", "/knowledgebases", true},
		{"knowledgebase with id", "/knowledgebases/kb-001", true},
		{"unmatched", "/other-path", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			assert.Equal(t, tt.want, h.RouteMatcher()(c))
		})
	}
}

func TestAgentsHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)
	e := echo.New()

	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		// PUT=Create, POST=List (real wire method for List*, per
		// bedrockagent@v1.58.4 serializers.go) share the same bare
		// collection path for all these families; GET remains accepted
		// too as harmless extra leniency for this package's own tests.
		{"CreateAgent", http.MethodPut, "/agents", "CreateAgent"},
		{"ListAgents (POST)", http.MethodPost, "/agents", "ListAgents"},
		{"ListAgents (GET)", http.MethodGet, "/agents", "ListAgents"},
		{"PrepareAgent", http.MethodPost, "/agents/agent-001/prepare", "PrepareAgent"},
		// The hyphenated "/action-groups" path is the OTHER non-canonical
		// internal-test-only route (dispatchActionGroupRoutes, distinct
		// from the real "/agentversions/{v}/actiongroups/" shape above) --
		// no real bedrock-agent client sends this shape, so it keeps the
		// original POST=Create/GET=List convention rather than the
		// PUT/POST convention the real wire shape uses.
		{"CreateAgentActionGroup", http.MethodPost, "/agents/agent-001/action-groups", "CreateAgentActionGroup"},
		{"ListAgentActionGroups", http.MethodGet, "/agents/agent-001/action-groups", "ListAgentActionGroups"},
		{"CreateAgentAlias", http.MethodPut, "/agents/agent-001/aliases", "CreateAgentAlias"},
		{"ListAgentAliases (POST)", http.MethodPost, "/agents/agent-001/aliases", "ListAgentAliases"},
		{"ListAgentAliases (GET)", http.MethodGet, "/agents/agent-001/aliases", "ListAgentAliases"},
		{"CreateKnowledgeBase", http.MethodPut, "/knowledgebases", "CreateKnowledgeBase"},
		{"ListKnowledgeBases (POST)", http.MethodPost, "/knowledgebases", "ListKnowledgeBases"},
		{"ListKnowledgeBases (GET)", http.MethodGet, "/knowledgebases", "ListKnowledgeBases"},
		{
			"StartIngestionJob",
			http.MethodPut,
			"/knowledgebases/kb-001/datasources/ds-001/ingestionjobs",
			"StartIngestionJob",
		},
		{
			"ListIngestionJobs (POST)",
			http.MethodPost,
			"/knowledgebases/kb-001/datasources/ds-001/ingestionjobs",
			"ListIngestionJobs",
		},
		{
			"ListIngestionJobs (GET)",
			http.MethodGet,
			"/knowledgebases/kb-001/datasources/ds-001/ingestionjobs",
			"ListIngestionJobs",
		},
		{"Unknown", http.MethodGet, "/unknown-path", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

func TestAgentsHandler_CreateAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      map[string]any
		name       string
		wantStatus int
		wantAgent  bool
	}{
		{
			name: "valid agent",
			input: map[string]any{
				"agentName":       "my-agent",
				"foundationModel": "anthropic.claude-v2",
				"instruction":     "You are a helpful assistant",
			},
			wantStatus: http.StatusAccepted,
			wantAgent:  true,
		},
		{
			name: "duplicate name",
			input: map[string]any{
				"agentName": "dup-agent",
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "invalid json",
			input:      nil,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestAgentsHandler(t)

			if tt.name == "duplicate name" {
				_, err := b.CreateAgent("dup-agent", "", "", "", nil)
				require.NoError(t, err)
			}

			if tt.input == nil {
				e := echo.New()
				req := httptest.NewRequest(http.MethodPut, "/agents", bytes.NewReader([]byte("bad json")))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				err := h.Handler()(c)
				require.NoError(t, err)
				assert.Equal(t, tt.wantStatus, rec.Code)

				return
			}

			rec := doAgentRequest(t, h, http.MethodPut, "/agents", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantAgent {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				agent := out["agent"].(map[string]any)
				assert.NotEmpty(t, agent["agentId"])
				assert.NotEmpty(t, agent["agentArn"])
			}
		})
	}
}

func TestAgentsHandler_GetAgent(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	ag, err := b.CreateAgent("get-agent", "anthropic.claude-v2", "", "", nil)
	require.NoError(t, err)

	t.Run("existing agent", func(t *testing.T) {
		t.Parallel()

		rec := doAgentRequest(t, h, http.MethodGet, "/agents/"+ag.AgentID, nil)
		assert.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		agent := out["agent"].(map[string]any)
		assert.Equal(t, ag.AgentID, agent["agentId"])
	})

	t.Run("nonexistent agent", func(t *testing.T) {
		t.Parallel()

		rec := doAgentRequest(t, h, http.MethodGet, "/agents/nonexistent", nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestAgentsHandler_ListAgents(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)

	rec := doAgentRequest(t, h, http.MethodGet, "/agents", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out["agentSummaries"])

	// Add agents.
	_, err := b.CreateAgent("agent-a", "", "", "", nil)
	require.NoError(t, err)
	_, err = b.CreateAgent("agent-b", "", "", "", nil)
	require.NoError(t, err)

	rec2 := doAgentRequest(t, h, http.MethodGet, "/agents", nil)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))
	assert.Len(t, out2["agentSummaries"].([]any), 2)
}

func TestAgentsHandler_UpdateAgent(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	ag, err := b.CreateAgent("update-agent", "", "", "", nil)
	require.NoError(t, err)

	t.Run("update existing", func(t *testing.T) {
		t.Parallel()

		rec := doAgentRequest(t, h, http.MethodPut, "/agents/"+ag.AgentID, map[string]any{
			"foundationModel": "anthropic.claude-v2",
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		agent := out["agent"].(map[string]any)
		assert.Equal(t, "anthropic.claude-v2", agent["foundationModel"])
	})

	t.Run("update nonexistent", func(t *testing.T) {
		t.Parallel()

		rec := doAgentRequest(t, h, http.MethodPut, "/agents/nonexistent", map[string]any{
			"foundationModel": "anthropic.claude-v2",
		})
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		e := echo.New()
		req := httptest.NewRequest(http.MethodPut, "/agents/"+ag.AgentID, bytes.NewReader([]byte("bad json")))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		handlerErr := h.Handler()(c)
		require.NoError(t, handlerErr)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestAgentsHandler_DeleteAgent(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	ag, err := b.CreateAgent("delete-agent", "", "", "", nil)
	require.NoError(t, err)

	rec := doAgentRequest(t, h, http.MethodDelete, "/agents/"+ag.AgentID, nil)
	assert.Equal(t, http.StatusAccepted, rec.Code)

	// Now it should be gone.
	rec2 := doAgentRequest(t, h, http.MethodGet, "/agents/"+ag.AgentID, nil)
	assert.Equal(t, http.StatusNotFound, rec2.Code)

	// Delete nonexistent.
	rec3 := doAgentRequest(t, h, http.MethodDelete, "/agents/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, rec3.Code)
}

// TestAgentsHandler_DeleteAgent_ClearsTags verifies DeleteAgent clears
// agentTags for the deleted agent's ARN. TagAgentResource/ListAgentResourceTags
// key on ARN with no existence check against the agent itself, so a leaked
// entry is directly observable: querying tags for a deleted agent's ARN would
// otherwise still return them, and agentTags is persisted verbatim in
// Snapshot(), so the leak also grows the snapshot without bound.
func TestAgentsHandler_DeleteAgent_ClearsTags(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	ag, err := b.CreateAgent("delete-agent-tags", "", "", "", nil)
	require.NoError(t, err)
	otherAg, err := b.CreateAgent("delete-agent-tags-sibling", "", "", "", nil)
	require.NoError(t, err)

	require.NoError(t, b.TagAgentResource(ag.AgentArn, map[string]string{"k": "v"}))
	require.NoError(t, b.TagAgentResource(otherAg.AgentArn, map[string]string{"k": "v"}))
	require.NotEmpty(t, b.ListAgentResourceTags(ag.AgentArn))

	rec := doAgentRequest(t, h, http.MethodDelete, "/agents/"+ag.AgentID, nil)
	assert.Equal(t, http.StatusAccepted, rec.Code)

	assert.Empty(t, b.ListAgentResourceTags(ag.AgentArn))
	assert.NotEmpty(t, b.ListAgentResourceTags(otherAg.AgentArn),
		"deleting one agent must not disturb another agent's tags")
}

func TestAgentsHandler_PrepareAgent(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	ag, err := b.CreateAgent("prepare-agent", "amazon.titan-text-express-v1", "", "", nil)
	require.NoError(t, err)

	rec := doAgentRequest(t, h, http.MethodPost, "/agents/"+ag.AgentID+"/prepare", nil)
	assert.Equal(t, http.StatusAccepted, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "PREPARING", out["agentStatus"])

	// Prepare nonexistent.
	rec2 := doAgentRequest(t, h, http.MethodPost, "/agents/nonexistent/prepare", nil)
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

func TestAgentsHandler_AgentPreparationTerminalStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		foundationModel string
		wantTerminal    string
	}{
		{name: "prepared", foundationModel: "amazon.titan-text-express-v1", wantTerminal: "PREPARED"},
		{name: "failed without model", wantTerminal: "FAILED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, b := newTestAgentsHandler(t)
			ag, err := b.CreateAgent(tt.name, tt.foundationModel, "", "", nil)
			require.NoError(t, err)
			assert.Equal(t, "NOT_PREPARED", ag.AgentStatus)

			preparing, err := b.PrepareAgent(ag.AgentID)
			require.NoError(t, err)
			assert.Equal(t, "PREPARING", preparing.AgentStatus)

			time.Sleep(150 * time.Millisecond)
			got, err := b.GetAgent(ag.AgentID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantTerminal, got.AgentStatus)
			if tt.wantTerminal == "FAILED" {
				assert.NotEmpty(t, got.FailureReasons)
			}
		})
	}
}

func TestAgentVersionCRUD(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)

	// Create agent
	rec := doAgentRequest(t, h, http.MethodPut, "/agents", map[string]any{
		"agentName":            "av-agent",
		"foundationModel":      "amazon.titan-text-express-v1",
		"agentResourceRoleArn": "arn:aws:iam::000000000000:role/role",
	})
	require.Equal(t, http.StatusAccepted, rec.Code)

	var ab map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ab))
	agentID := ab["agent"].(map[string]any)["agentId"].(string)

	// Create version
	rec = doAgentRequest(t, h, http.MethodPost,
		fmt.Sprintf("/agents/%s/versions", agentID), nil)
	assert.Equal(t, http.StatusAccepted, rec.Code)

	var vb map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &vb))
	version := vb["agentVersion"].(map[string]any)["agentVersion"].(string)
	assert.Equal(t, "1", version)

	// Get version
	rec = doAgentRequest(t, h, http.MethodGet,
		fmt.Sprintf("/agents/%s/versions/%s", agentID, version), nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// List versions
	rec = doAgentRequest(t, h, http.MethodGet,
		fmt.Sprintf("/agents/%s/versions", agentID), nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var lb map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lb))
	assert.Len(t, lb["agentVersionSummaries"], 1)

	// Delete version
	rec = doAgentRequest(t, h, http.MethodDelete,
		fmt.Sprintf("/agents/%s/versions/%s", agentID, version), nil)
	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestAgentResourceTags(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)

	resourceArn := "arn:aws:bedrock:us-east-1:000000000000:flow/flow-00000001"

	// Tag
	rec := doAgentRequest(t, h, http.MethodPost, "/tags/"+resourceArn, map[string]any{
		"tags": map[string]string{"env": "prod", "team": "core"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// List tags
	rec = doAgentRequest(t, h, http.MethodGet, "/tags/"+resourceArn, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var lb map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lb))
	tags := lb["tags"].(map[string]any)
	assert.Equal(t, "prod", tags["env"])
	assert.Equal(t, "core", tags["team"])

	// Untag
	rec = doAgentRequest(t, h, http.MethodDelete, "/tags/"+resourceArn, map[string]any{
		"tagKeys": []string{"team"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// List tags again — team should be gone
	rec = doAgentRequest(t, h, http.MethodGet, "/tags/"+resourceArn, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var lb2 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lb2))
	tags2 := lb2["tags"].(map[string]any)
	assert.Equal(t, "prod", tags2["env"])
	_, hasTeam := tags2["team"]
	assert.False(t, hasTeam)
}

func TestAgentMemory(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)

	// Create agent
	rec := doAgentRequest(t, h, http.MethodPut, "/agents", map[string]any{
		"agentName":            "mem-agent",
		"foundationModel":      "amazon.titan-text-express-v1",
		"agentResourceRoleArn": "arn:aws:iam::000000000000:role/role",
	})
	require.Equal(t, http.StatusAccepted, rec.Code)

	var ab map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &ab))
	agentID := ab["agent"].(map[string]any)["agentId"].(string)

	memPath := fmt.Sprintf("/agents/%s/agentversions/DRAFT/memories", agentID)

	// Get memory (empty)
	rec = doAgentRequest(t, h, http.MethodGet, memPath+"?sessionId=s1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Delete memory
	rec = doAgentRequest(t, h, http.MethodDelete, memPath+"?sessionId=s1", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestAgentsHandler_AgentExtendedConfiguration(t *testing.T) {
	t.Parallel()

	h, _ := newTestAgentsHandler(t)
	rec := doAgentRequest(t, h, http.MethodPut, "/agents", map[string]any{
		"agentName":          "configured-agent",
		"foundationModel":    "amazon.titan-text-express-v1",
		"agentCollaboration": "SUPERVISOR",
		"description":        "coordinates specialists",
		"guardrailConfiguration": map[string]any{
			"guardrailIdentifier": "guardrail-1",
			"guardrailVersion":    "1",
		},
		"memoryConfiguration": map[string]any{
			"enabledMemoryTypes": []string{"SESSION_SUMMARY"},
			"storageDays":        float64(7),
		},
	})
	require.Equal(t, http.StatusAccepted, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	agent := body["agent"].(map[string]any)
	assert.Equal(t, "SUPERVISOR", agent["agentCollaboration"])
	assert.Equal(t, "guardrail-1", agent["guardrailConfiguration"].(map[string]any)["guardrailIdentifier"])
	assert.InDelta(t, 7, agent["memoryConfiguration"].(map[string]any)["storageDays"], 0)
}

func TestBatch2AgentOps_DeleteAgent_WithActiveAlias_Rejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "one_alias"},
		{name: "named_alias"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestAgentsHandler(t)
			agent, err := b.CreateAgent("delete-agent-"+tc.name, "model", "", "", nil)
			require.NoError(t, err)

			// Create an alias for the agent.
			aliasRec := doAgentRequest(t, h, http.MethodPut,
				fmt.Sprintf("/agents/%s/aliases", agent.AgentID),
				map[string]any{"agentAliasName": "live"})
			require.Equal(t, http.StatusAccepted, aliasRec.Code)

			// Delete agent with active alias — must fail with ConflictException.
			delRec := doAgentRequest(t, h, http.MethodDelete,
				fmt.Sprintf("/agents/%s", agent.AgentID), nil)
			assert.Equal(t, http.StatusConflict, delRec.Code,
				"deleting agent with active aliases should return 409 ConflictException")

			var errBody map[string]any
			require.NoError(t, json.Unmarshal(delRec.Body.Bytes(), &errBody))
			assert.Equal(t, "ConflictException", errBody["__type"])

			// Agent must still exist.
			getRec := doAgentRequest(t, h, http.MethodGet,
				fmt.Sprintf("/agents/%s", agent.AgentID), nil)
			assert.Equal(t, http.StatusOK, getRec.Code, "agent must still exist after rejected delete")
		})
	}
}

func TestBatch2AgentOps_DeleteAgent_NoAliases_Succeeds(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	agent, err := b.CreateAgent("delete-no-alias", "model", "", "", nil)
	require.NoError(t, err)

	// Delete agent with no aliases — must succeed.
	delRec := doAgentRequest(t, h, http.MethodDelete,
		fmt.Sprintf("/agents/%s", agent.AgentID), nil)
	assert.Equal(t, http.StatusAccepted, delRec.Code)

	// Agent must be gone.
	getRec := doAgentRequest(t, h, http.MethodGet,
		fmt.Sprintf("/agents/%s", agent.AgentID), nil)
	assert.Equal(t, http.StatusNotFound, getRec.Code)
}

func TestBatch2AgentOps_DeleteAgent_AfterDeleteAlias_Succeeds(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	agent, err := b.CreateAgent("delete-after-alias-removed", "model", "", "", nil)
	require.NoError(t, err)

	// Create then delete the alias.
	aliasRec := doAgentRequest(t, h, http.MethodPut,
		fmt.Sprintf("/agents/%s/aliases", agent.AgentID),
		map[string]any{"agentAliasName": "temp"})
	require.Equal(t, http.StatusAccepted, aliasRec.Code)

	var aliasBody map[string]any
	require.NoError(t, json.Unmarshal(aliasRec.Body.Bytes(), &aliasBody))
	aliasID := aliasBody["agentAlias"].(map[string]any)["agentAliasId"].(string)

	delAliasRec := doAgentRequest(t, h, http.MethodDelete,
		fmt.Sprintf("/agents/%s/aliases/%s", agent.AgentID, aliasID), nil)
	require.Equal(t, http.StatusAccepted, delAliasRec.Code)

	// Now delete the agent — must succeed.
	delRec := doAgentRequest(t, h, http.MethodDelete,
		fmt.Sprintf("/agents/%s", agent.AgentID), nil)
	assert.Equal(t, http.StatusAccepted, delRec.Code)
}
