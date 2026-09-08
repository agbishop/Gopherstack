package bedrock_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/bedrock"
)

func TestDeleteAgent_SkipResourceInUseCheck_Absent_Refused(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	agent, err := b.CreateAgent("kr6t-absent", "model", "", "", nil)
	require.NoError(t, err)

	aliasRec := doAgentRequest(t, h, http.MethodPut,
		fmt.Sprintf("/agents/%s/aliases", agent.AgentID),
		map[string]any{"agentAliasName": "live"})
	require.Equal(t, http.StatusAccepted, aliasRec.Code)

	delRec := doAgentRequest(t, h, http.MethodDelete, "/agents/"+agent.AgentID, nil)
	assert.Equal(t, http.StatusConflict, delRec.Code,
		"omitting skipResourceInUseCheck must preserve the existing refusal")

	getRec := doAgentRequest(t, h, http.MethodGet, "/agents/"+agent.AgentID, nil)
	assert.Equal(t, http.StatusOK, getRec.Code, "agent must still exist")
}

func TestDeleteAgent_SkipResourceInUseCheck_False_Refused(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	agent, err := b.CreateAgent("kr6t-false", "model", "", "", nil)
	require.NoError(t, err)

	aliasRec := doAgentRequest(t, h, http.MethodPut,
		fmt.Sprintf("/agents/%s/aliases", agent.AgentID),
		map[string]any{"agentAliasName": "live"})
	require.Equal(t, http.StatusAccepted, aliasRec.Code)

	delRec := doAgentRequest(t, h, http.MethodDelete,
		"/agents/"+agent.AgentID+"?skipResourceInUseCheck=false", nil)
	assert.Equal(t, http.StatusConflict, delRec.Code,
		"an explicit false must refuse just like the absent case")

	getRec := doAgentRequest(t, h, http.MethodGet, "/agents/"+agent.AgentID, nil)
	assert.Equal(t, http.StatusOK, getRec.Code, "agent must still exist")
}

func TestDeleteAgent_SkipResourceInUseCheck_True_DeletesDespiteAlias(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	agent, err := b.CreateAgent("kr6t-true", "model", "", "", nil)
	require.NoError(t, err)

	aliasRec := doAgentRequest(t, h, http.MethodPut,
		fmt.Sprintf("/agents/%s/aliases", agent.AgentID),
		map[string]any{"agentAliasName": "live"})
	require.Equal(t, http.StatusAccepted, aliasRec.Code)

	delRec := doAgentRequest(t, h, http.MethodDelete,
		"/agents/"+agent.AgentID+"?skipResourceInUseCheck=true", nil)
	assert.Equal(t, http.StatusAccepted, delRec.Code,
		"an explicit true must bypass the in-use refusal")

	getRec := doAgentRequest(t, h, http.MethodGet, "/agents/"+agent.AgentID, nil)
	assert.Equal(t, http.StatusNotFound, getRec.Code, "agent must be gone")
}

// TestDeleteAgent_SkipResourceInUseCheck_True_CascadesAliases proves the
// alias fate decision: skipping the in-use check does not leave the
// bypassed aliases addressable as ghost rows. Two aliases are created so the
// assertion is positive (the survivor's identity is checked, not just an
// empty-list check that would also pass if the cascade over-deleted).
func TestDeleteAgent_SkipResourceInUseCheck_True_CascadesAliases(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	roleArn := "arn:aws:iam::000000000000:role/test"

	agent, err := b.CreateAgent("kr6t-cascade", "amazon.titan-text-express-v1", "", roleArn, nil)
	require.NoError(t, err)
	sibling, err := b.CreateAgent("kr6t-cascade-sibling", "amazon.titan-text-express-v1", "", roleArn, nil)
	require.NoError(t, err)

	alias1, err := b.CreateAgentAlias(agent.AgentID, "alias-one", "DRAFT")
	require.NoError(t, err)
	alias2, err := b.CreateAgentAlias(agent.AgentID, "alias-two", "DRAFT")
	require.NoError(t, err)
	siblingAlias, err := b.CreateAgentAlias(sibling.AgentID, "alias-sibling", "DRAFT")
	require.NoError(t, err)

	require.NoError(t, b.TagAgentResource(alias1.AgentAliasArn, map[string]string{"k": "v"}))

	require.NoError(t, b.DeleteAgent(agent.AgentID, true))

	_, err = b.GetAgentAlias(agent.AgentID, alias1.AgentAliasID)
	require.Error(t, err, "alias1 must not survive its parent agent's skipped-check deletion")
	_, err = b.GetAgentAlias(agent.AgentID, alias2.AgentAliasID)
	require.Error(t, err, "alias2 must not survive its parent agent's skipped-check deletion")

	aliases, _ := b.ListAgentAliases(agent.AgentID, 0, "")
	assert.Empty(t, aliases, "ListAgentAliases must not return ghost rows for the deleted agent")

	assert.Empty(t, b.ListAgentResourceTags(alias1.AgentAliasArn),
		"deleted alias's tags must not leak past the cascade")

	survivor, err := b.GetAgentAlias(sibling.AgentID, siblingAlias.AgentAliasID)
	require.NoError(t, err, "a sibling agent's alias must survive an unrelated agent's cascade")
	assert.Equal(t, siblingAlias.AgentAliasID, survivor.AgentAliasID)
}

func TestDeleteAgent_NoAliases_SkipResourceInUseCheckIrrelevant(t *testing.T) {
	t.Parallel()

	h, b := newTestAgentsHandler(t)
	agent, err := b.CreateAgent("kr6t-no-alias", "model", "", "", nil)
	require.NoError(t, err)

	delRec := doAgentRequest(t, h, http.MethodDelete,
		"/agents/"+agent.AgentID+"?skipResourceInUseCheck=true", nil)
	assert.Equal(t, http.StatusAccepted, delRec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(delRec.Body.Bytes(), &body))
	assert.Equal(t, agent.AgentID, body["agentId"])
}
