package bedrock_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/bedrock"
)

func TestDeleteAgentAlias_PrunesAgentTags(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	ag, err := b.CreateAgent(
		"agent1",
		"amazon.titan-text-express-v1",
		"",
		"arn:aws:iam::000000000000:role/test",
		nil,
	)
	require.NoError(t, err)

	alias, err := b.CreateAgentAlias(ag.AgentID, "alias1", "DRAFT")
	require.NoError(t, err)

	require.NoError(t, b.TagAgentResource(alias.AgentAliasArn, map[string]string{"k": "v"}))

	require.NoError(t, b.DeleteAgentAlias(ag.AgentID, alias.AgentAliasID))

	tags := b.ListAgentResourceTags(alias.AgentAliasArn)
	assert.Empty(
		t,
		tags,
		"ListAgentResourceTags must not return ghost tags for a deleted agent alias",
	)
}

func TestDeleteKnowledgeBase_PrunesAgentTagsAndIngestionArtifacts(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	kb, err := b.CreateKnowledgeBase(
		"kb1",
		"",
		"arn:aws:iam::000000000000:role/test",
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)

	require.NoError(t, b.TagAgentResource(kb.KnowledgeBaseArn, map[string]string{"k": "v"}))

	ds, err := b.CreateDataSource(kb.KnowledgeBaseID, "ds1", "", nil)
	require.NoError(t, err)

	job, err := b.StartIngestionJob(kb.KnowledgeBaseID, ds.DataSourceID, "")
	require.NoError(t, err)

	_, err = b.IngestKnowledgeBaseDocuments(kb.KnowledgeBaseID, ds.DataSourceID, []string{"doc1"})
	require.NoError(t, err)

	require.NoError(t, b.DeleteKnowledgeBase(kb.KnowledgeBaseID))

	tags := b.ListAgentResourceTags(kb.KnowledgeBaseArn)
	assert.Empty(
		t,
		tags,
		"ListAgentResourceTags must not return ghost tags for a deleted knowledge base",
	)

	_, err = b.GetIngestionJob(kb.KnowledgeBaseID, ds.DataSourceID, job.IngestionJobID)
	require.Error(t, err, "ingestion job must not survive its parent knowledge base's deletion")

	jobs, _ := b.ListIngestionJobs(kb.KnowledgeBaseID, ds.DataSourceID, 0, "")
	assert.Empty(
		t,
		jobs,
		"ListIngestionJobs must not return ghost rows for a deleted knowledge base",
	)

	docs, _ := b.ListKnowledgeBaseDocuments(kb.KnowledgeBaseID, ds.DataSourceID, 0, "")
	assert.Empty(
		t,
		docs,
		"ListKnowledgeBaseDocuments must not return ghost rows for a deleted knowledge base",
	)
}

func TestDeleteFlow_PrunesTagsVersionsAndAliases(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	f, err := b.CreateFlow("flow1", "", nil)
	require.NoError(t, err)

	require.NoError(t, b.TagAgentResource(f.FlowArn, map[string]string{"k": "v"}))

	ver, err := b.CreateFlowVersion(f.FlowID)
	require.NoError(t, err)

	alias, err := b.CreateFlowAlias(f.FlowID, "alias1", "")
	require.NoError(t, err)

	require.NoError(t, b.DeleteFlow(f.FlowID))

	tags := b.ListAgentResourceTags(f.FlowArn)
	assert.Empty(t, tags, "ListAgentResourceTags must not return ghost tags for a deleted flow")

	_, err = b.GetFlowVersion(f.FlowID, ver.Version)
	require.Error(t, err, "flow version must not survive the flow's deletion")

	versions, _ := b.ListFlowVersions(f.FlowID, 0, "")
	assert.Empty(t, versions, "ListFlowVersions must not return ghost rows for a deleted flow")

	_, err = b.GetFlowAlias(f.FlowID, alias.FlowAliasID)
	require.Error(t, err, "flow alias must not survive the flow's deletion")

	aliases, _ := b.ListFlowAliases(f.FlowID, 0, "")
	assert.Empty(t, aliases, "ListFlowAliases must not return ghost rows for a deleted flow")

	assert.Zero(
		t,
		b.FlowVersionCounterForTest(f.FlowID),
		"flowVersionCounters must be pruned on flow delete",
	)
}

func TestDeleteFlowAlias_PrunesAgentTags(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	f, err := b.CreateFlow("flow1", "", nil)
	require.NoError(t, err)

	alias, err := b.CreateFlowAlias(f.FlowID, "alias1", "")
	require.NoError(t, err)

	require.NoError(t, b.TagAgentResource(alias.FlowAliasArn, map[string]string{"k": "v"}))

	require.NoError(t, b.DeleteFlowAlias(f.FlowID, alias.FlowAliasID))

	tags := b.ListAgentResourceTags(alias.FlowAliasArn)
	assert.Empty(
		t,
		tags,
		"ListAgentResourceTags must not return ghost tags for a deleted flow alias",
	)
}

func TestDeletePrompt_PrunesTagsAndVersions(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	p, err := b.CreatePrompt("prompt1", "", nil)
	require.NoError(t, err)

	require.NoError(t, b.TagAgentResource(p.PromptArn, map[string]string{"k": "v"}))

	ver, err := b.CreatePromptVersion(p.PromptID)
	require.NoError(t, err)

	require.NoError(t, b.DeletePrompt(p.PromptID))

	tags := b.ListAgentResourceTags(p.PromptArn)
	assert.Empty(t, tags, "ListAgentResourceTags must not return ghost tags for a deleted prompt")

	_, err = b.GetPromptVersion(p.PromptID, ver.Version)
	require.Error(t, err, "prompt version must not survive the prompt's deletion")

	versions, _ := b.ListPromptVersions(p.PromptID, 0, "")
	assert.Empty(t, versions, "ListPromptVersions must not return ghost rows for a deleted prompt")

	assert.Zero(
		t,
		b.PromptVersionCounterForTest(p.PromptID),
		"promptVersionCounters must be pruned on prompt delete",
	)
}

func TestDeleteAutomatedReasoningPolicy_PrunesVersionCounterAndAnnotations(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	policy, err := b.CreateAutomatedReasoningPolicy("policy1", "", nil)
	require.NoError(t, err)

	_, err = b.CreateAutomatedReasoningPolicyVersion(policy.PolicyArn, "hash1", nil)
	require.NoError(t, err)

	wf, err := b.StartAutomatedReasoningPolicyBuildWorkflow(
		policy.PolicyArn,
		"INGEST_CONTENT",
		json.RawMessage(`{}`),
	)
	require.NoError(t, err)

	_, err = b.UpdateAutomatedReasoningPolicyAnnotations(
		policy.PolicyArn,
		wf.BuildWorkflowID,
		[]any{},
		"hash",
	)
	require.NoError(t, err)

	require.True(t, b.ARPAnnotationStateExistsForTest(policy.PolicyArn, wf.BuildWorkflowID),
		"test setup must actually mint annotation state before deletion")
	assert.Positive(
		t,
		b.ARPVersionCountForTest(
			policy.PolicyArn,
		),
		"test setup must actually bump the version counter",
	)

	require.NoError(t, b.DeleteAutomatedReasoningPolicy(policy.PolicyArn, true))

	assert.Zero(
		t,
		b.ARPVersionCountForTest(policy.PolicyArn),
		"arpVersionCountByPolicy must be pruned on policy delete",
	)
	assert.False(t, b.ARPAnnotationStateExistsForTest(policy.PolicyArn, wf.BuildWorkflowID),
		"annotation state must be pruned when the owning policy is deleted")
}

func TestDeleteAutomatedReasoningPolicyBuildWorkflow_PrunesAnnotations(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend(testAccountID, testRegion)

	policy, err := b.CreateAutomatedReasoningPolicy("policy1", "", nil)
	require.NoError(t, err)

	wf, err := b.StartAutomatedReasoningPolicyBuildWorkflow(
		policy.PolicyArn,
		"INGEST_CONTENT",
		json.RawMessage(`{}`),
	)
	require.NoError(t, err)

	_, err = b.UpdateAutomatedReasoningPolicyAnnotations(
		policy.PolicyArn,
		wf.BuildWorkflowID,
		[]any{},
		"hash",
	)
	require.NoError(t, err)

	require.True(t, b.ARPAnnotationStateExistsForTest(policy.PolicyArn, wf.BuildWorkflowID),
		"test setup must actually mint annotation state before deletion")

	require.NoError(
		t,
		b.DeleteAutomatedReasoningPolicyBuildWorkflow(policy.PolicyArn, wf.BuildWorkflowID),
	)

	assert.False(t, b.ARPAnnotationStateExistsForTest(policy.PolicyArn, wf.BuildWorkflowID),
		"annotation state must be pruned when its owning build workflow is deleted")
}
