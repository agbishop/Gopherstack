package bedrockagent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/bedrockagent"
)

// TestNewAgentVersion_SnapshotsSubResources locks in that creating a numbered
// agent version (via CreateAgentAlias's auto-version-creation, the only real
// path to one) snapshots the agent's DRAFT action groups, collaborators, and
// knowledge-base associations into the new version -- matching real AWS,
// confirmed via GetAgentActionGroup's API reference: its {agentVersion} path
// pattern accepts non-DRAFT versions too, so a real client can Get/List these
// against a numbered version and expects the DRAFT-at-creation-time content,
// not an empty result.
func TestNewAgentVersion_SnapshotsSubResources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := bedrockagent.NewTestBackend("us-east-1", "123456789012")

	agent, err := b.CreateAgent(ctx, bedrockagent.AgentConfig{
		AgentName:       "snapshot-agent",
		FoundationModel: "anthropic.claude-v2",
		RoleARN:         "arn:aws:iam::123456789012:role/BedrockRole",
	})
	require.NoError(t, err)

	ag, err := b.CreateAgentActionGroup(ctx, agent.AgentID, "DRAFT", bedrockagent.ActionGroupConfig{
		ActionGroupName: "snap-ag",
	})
	require.NoError(t, err)

	collab, err := b.AssociateAgentCollaborator(ctx, agent.AgentID, "DRAFT", bedrockagent.CollaboratorConfig{
		CollaboratorName:         "snap-collab",
		CollaborationInstruction: "help",
		AgentDescriptor: map[string]any{
			"aliasArn": "arn:aws:bedrock:us-east-1:123456789012:agent-alias/OTHER12345/ALIASOTHER",
		},
	})
	require.NoError(t, err)

	kb, err := b.CreateKnowledgeBase(ctx, bedrockagent.KnowledgeBaseConfig{
		Name:    "snap-kb",
		RoleARN: "arn:aws:iam::123456789012:role/KBRole",
	})
	require.NoError(t, err)

	_, err = b.AssociateAgentKnowledgeBase(ctx, agent.AgentID, "DRAFT", kb.KnowledgeBaseID, "test", "ENABLED")
	require.NoError(t, err)

	// Empty RoutingConfiguration triggers real AWS's auto-create-a-version
	// behavior (see agent_versions.go's newAgentVersionLocked doc comment).
	alias, err := b.CreateAgentAlias(ctx, agent.AgentID, bedrockagent.AliasConfig{
		AliasName: "snap-alias",
	})
	require.NoError(t, err)

	version := alias.RoutingConfiguration[0].AgentVersion
	require.NotEqual(t, "DRAFT", version)

	gotAG, err := b.GetAgentActionGroup(ctx, agent.AgentID, version, ag.ActionGroupID)
	require.NoError(t, err)
	require.Equal(t, ag.ActionGroupName, gotAG.ActionGroupName)
	require.Equal(t, version, gotAG.AgentVersion)

	gotCollab, err := b.GetAgentCollaborator(ctx, agent.AgentID, version, collab.CollaboratorID)
	require.NoError(t, err)
	require.Equal(t, collab.CollaboratorName, gotCollab.CollaboratorName)

	gotKB, err := b.GetAgentKnowledgeBase(ctx, agent.AgentID, version, kb.KnowledgeBaseID)
	require.NoError(t, err)
	require.Equal(t, "ENABLED", gotKB.KBState)

	agList, _, err := b.ListAgentActionGroups(ctx, agent.AgentID, version, 10, "")
	require.NoError(t, err)
	require.Len(t, agList, 1)

	collabList, _, err := b.ListAgentCollaborators(ctx, agent.AgentID, version, 10, "")
	require.NoError(t, err)
	require.Len(t, collabList, 1)

	kbList, _, err := b.ListAgentKnowledgeBases(ctx, agent.AgentID, version, 10, "")
	require.NoError(t, err)
	require.Len(t, kbList, 1)

	// DRAFT keeps its own live copies untouched.
	draftAG, err := b.GetAgentActionGroup(ctx, agent.AgentID, "DRAFT", ag.ActionGroupID)
	require.NoError(t, err)
	require.Equal(t, "DRAFT", draftAG.AgentVersion)
}

// TestDeleteAgentVersion_CascadesSubResources locks in that DeleteAgentVersion
// removes the action groups/collaborators/KB associations snapshotted into
// that version, without touching DRAFT's own copies -- the ghost-row class
// of bug this package's DeleteAgent cascade fix already covers, now also
// reachable via numbered-version snapshot rows introduced by
// snapshotSubResourcesLocked.
func TestDeleteAgentVersion_CascadesSubResources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := bedrockagent.NewTestBackend("us-east-1", "123456789012")

	agent, err := b.CreateAgent(ctx, bedrockagent.AgentConfig{
		AgentName:       "del-version-agent",
		FoundationModel: "anthropic.claude-v2",
		RoleARN:         "arn:aws:iam::123456789012:role/BedrockRole",
	})
	require.NoError(t, err)

	_, err = b.CreateAgentActionGroup(ctx, agent.AgentID, "DRAFT", bedrockagent.ActionGroupConfig{
		ActionGroupName: "del-ag",
	})
	require.NoError(t, err)

	alias, err := b.CreateAgentAlias(ctx, agent.AgentID, bedrockagent.AliasConfig{
		AliasName: "del-alias",
	})
	require.NoError(t, err)

	version := alias.RoutingConfiguration[0].AgentVersion

	agBefore, _, err := b.ListAgentActionGroups(ctx, agent.AgentID, version, 10, "")
	require.NoError(t, err)
	require.Len(t, agBefore, 1, "precondition: snapshot must exist before delete")

	// skipResourceInUseCheck=true: alias still routes to version, and this
	// test's concern is sub-resource cascade cleanup, not the in-use guard
	// (see TestDeleteAgentVersion_BlockedWhileAliasReferencesIt for that).
	require.NoError(t, b.DeleteAgentVersion(ctx, agent.AgentID, version, true))

	agAfter, _, err := b.ListAgentActionGroups(ctx, agent.AgentID, version, 10, "")
	require.NoError(t, err)
	require.Empty(t, agAfter, "action group snapshot not cascade-deleted with the version")

	draftAfter, _, err := b.ListAgentActionGroups(ctx, agent.AgentID, "DRAFT", 10, "")
	require.NoError(t, err)
	require.Len(t, draftAfter, 1, "DRAFT's own action group must survive deleting the numbered version")
}
