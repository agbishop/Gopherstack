package bedrock_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/bedrock"
)

func TestDeleteKnowledgeBase_RemovesDataSources(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	kb, err := b.CreateKnowledgeBase("kb1", "", "arn:aws:iam::000000000000:role/test", nil, nil, nil)
	require.NoError(t, err)

	ds, err := b.CreateDataSource(kb.KnowledgeBaseID, "ds1", "", nil)
	require.NoError(t, err)

	require.NoError(t, b.DeleteKnowledgeBase(kb.KnowledgeBaseID))

	_, err = b.GetDataSource(kb.KnowledgeBaseID, ds.DataSourceID)
	require.Error(t, err, "data source must not survive its parent knowledge base's deletion")

	list, _ := b.ListDataSources(kb.KnowledgeBaseID, 0, "")
	assert.Empty(t, list, "ListDataSources must not return ghost rows for a deleted knowledge base")
}

func TestDeleteAgent_ClearsVersionsAndCollaborators(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	ag, err := b.CreateAgent("agent1", "amazon.titan-text-express-v1", "", "arn:aws:iam::000000000000:role/test", nil)
	require.NoError(t, err)

	ver, err := b.CreateAgentVersion(ag.AgentID)
	require.NoError(t, err)

	collab, err := b.AssociateAgentCollaborator(
		ag.AgentID, ver.AgentVersion, "arn:aws:bedrock:us-east-1:000000000000:agent/other", "DISABLED",
	)
	require.NoError(t, err)

	require.NoError(t, b.DeleteAgent(ag.AgentID))

	_, err = b.GetAgentVersion(ag.AgentID, ver.AgentVersion)
	require.Error(t, err, "agent version must not survive the agent's deletion")

	versions, _ := b.ListAgentVersions(ag.AgentID, 0, "")
	assert.Empty(t, versions, "ListAgentVersions must not return ghost rows for a deleted agent")

	_, err = b.GetAgentCollaborator(ag.AgentID, collab.CollaboratorID)
	require.Error(t, err, "agent collaborator must not survive the agent's deletion")

	collabs, _ := b.ListAgentCollaborators(ag.AgentID, 0, "")
	assert.Empty(t, collabs, "ListAgentCollaborators must not return ghost rows for a deleted agent")
}
