package bedrock_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/blackbirdworks/gopherstack/services/bedrock"
	"github.com/stretchr/testify/require"
)

// walkAttempts is how many times each paginated walk is repeated against the
// same, unchanged backend state. Go randomises map iteration order per
// range, not per map instance, so a non-total sort over store.Table.All()
// can (and, per the glue precedent, reliably does) disagree with itself
// across separate calls with nothing changed in between. One walk can pass
// by luck; the bug is about instability *across* calls.
const walkAttempts = 30

// walkAndVerify repeats a small-page paginated walk walkAttempts times,
// failing if any attempt drops or duplicates an item relative to want, or
// returns the same id on two different pages within one walk.
func walkAndVerify(t *testing.T, want map[string]bool, listPage func(token string) (ids []string, next string)) {
	t.Helper()

	for attempt := range walkAttempts {
		got := make(map[string]bool, len(want))
		token := ""

		for {
			ids, next := listPage(token)
			for _, id := range ids {
				require.Falsef(t, got[id], "attempt %d: id %q returned on more than one page", attempt, id)
				got[id] = true
			}

			if next == "" {
				break
			}

			token = next
		}

		require.Equalf(t, want, got, "attempt %d: paginated walk did not reproduce the created set exactly", attempt)
	}
}

func TestListAgentActionGroupsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	agent, err := b.CreateAgent("agent1", "anthropic.claude-v2", "instr", "arn:aws:iam::111111111111:role/x", nil)
	require.NoError(t, err)

	want := make(map[string]bool, 3)
	for i := range 3 {
		ag, createErr := b.CreateAgentActionGroup(agent.AgentID, "dup-name", fmt.Sprintf("desc-%d", i), nil)
		require.NoError(t, createErr)
		want[ag.ActionGroupID] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListAgentActionGroups(agent.AgentID, 1, token)
		ids := make([]string, len(page))
		for i, ag := range page {
			ids[i] = ag.ActionGroupID
		}

		return ids, next
	})
}

func TestListDataSourcesSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	kb, err := b.CreateKnowledgeBase("kb1", "", "arn:aws:iam::111111111111:role/x", nil, nil, nil)
	require.NoError(t, err)

	want := make(map[string]bool, 3)
	for i := range 3 {
		ds, createErr := b.CreateDataSource(kb.KnowledgeBaseID, "dup-name", fmt.Sprintf("desc-%d", i), nil)
		require.NoError(t, createErr)
		want[ds.DataSourceID] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListDataSources(kb.KnowledgeBaseID, 1, token)
		ids := make([]string, len(page))
		for i, ds := range page {
			ids[i] = ds.DataSourceID
		}

		return ids, next
	})
}

func TestListFlowAliasesSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	flow, err := b.CreateFlow("flow1", "", nil)
	require.NoError(t, err)

	want := make(map[string]bool, 3)
	for i := range 3 {
		fa, createErr := b.CreateFlowAlias(flow.FlowID, "dup-name", fmt.Sprintf("desc-%d", i))
		require.NoError(t, createErr)
		want[fa.FlowAliasID] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListFlowAliases(flow.FlowID, 1, token)
		ids := make([]string, len(page))
		for i, fa := range page {
			ids[i] = fa.FlowAliasID
		}

		return ids, next
	})
}

func TestListAgentAliasesSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	agent, err := b.CreateAgent("agent1", "anthropic.claude-v2", "instr", "arn:aws:iam::111111111111:role/x", nil)
	require.NoError(t, err)

	want := make(map[string]bool, 3)
	for range 3 {
		alias, createErr := b.CreateAgentAlias(agent.AgentID, "dup-name", "DRAFT")
		require.NoError(t, createErr)
		want[alias.AgentAliasID] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListAgentAliases(agent.AgentID, 1, token)
		ids := make([]string, len(page))
		for i, alias := range page {
			ids[i] = alias.AgentAliasID
		}

		return ids, next
	})
}

func TestListCustomModelsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	tie := time.Now().UTC()

	// paginateBedrockSlice has a fixed page size (bedrockDefaultPageSize ==
	// 100), so the tie group must exceed one page to exercise a real page
	// boundary.
	const n = 105
	want := make(map[string]bool, n)
	for i := range n {
		arn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:custom-model/m-%03d", i)
		b.SeedCustomModelForTest(&bedrock.CustomModel{
			ModelArn:     arn,
			ModelName:    fmt.Sprintf("model-%03d", i),
			ModelStatus:  "Active",
			CreationTime: tie,
		})
		want[arn] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListCustomModels(&bedrock.ListCustomModelsInput{NextToken: token})
		ids := make([]string, len(page))
		for i, m := range page {
			ids[i] = m.ModelArn
		}

		return ids, next
	})
}

func TestListEvaluationJobsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	tie := time.Now().UTC()

	const n = 105
	want := make(map[string]bool, n)
	for i := range n {
		arn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:evaluation-job/j-%03d", i)
		b.SeedEvaluationJobForTest(&bedrock.EvaluationJob{
			JobArn:       arn,
			JobName:      fmt.Sprintf("job-%03d", i),
			Status:       "InProgress",
			CreationTime: tie,
		})
		want[arn] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListEvaluationJobs(&bedrock.ListEvaluationJobsInput{NextToken: token})
		ids := make([]string, len(page))
		for i, j := range page {
			ids[i] = j.JobArn
		}

		return ids, next
	})
}

func TestListCustomModelDeploymentsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	tie := time.Now().UTC()

	want := make(map[string]bool, 3)
	for i := range 3 {
		arn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:custom-model-deployment/d-%03d", i)
		b.SeedCustomModelDeploymentForTest(&bedrock.CustomModelDeployment{
			CustomModelDeploymentArn: arn,
			ModelDeploymentName:      fmt.Sprintf("deploy-%03d", i),
			Status:                   "Active",
			CreationTime:             tie,
		})
		want[arn] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListCustomModelDeployments(&bedrock.ListCustomModelDeploymentsInput{
			MaxResults: 1,
			NextToken:  token,
		})
		ids := make([]string, len(page))
		for i, d := range page {
			ids[i] = d.CustomModelDeploymentArn
		}

		return ids, next
	})
}

func TestListModelCopyJobsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	tie := time.Now().UTC()

	want := make(map[string]bool, 3)
	for i := range 3 {
		arn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:model-copy-job/c-%03d", i)
		b.SeedModelCopyJobForTest(&bedrock.ModelCopyJob{
			JobArn:         arn,
			SourceModelArn: "arn:aws:bedrock:us-west-2::foundation-model/anthropic.claude-v2",
			TargetModelArn: arn,
			Status:         "Completed",
			CreationTime:   tie,
		})
		want[arn] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListModelCopyJobs(&bedrock.ListModelCopyJobsInput{MaxResults: 1, NextToken: token})
		ids := make([]string, len(page))
		for i, j := range page {
			ids[i] = j.JobArn
		}

		return ids, next
	})
}

func TestListModelInvocationJobsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	tie := time.Now().UTC()

	const n = 105
	want := make(map[string]bool, n)
	for i := range n {
		arn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:model-invocation-job/i-%03d", i)
		b.SeedModelInvocationJobForTest(&bedrock.ModelInvocationJob{
			JobArn:       arn,
			JobName:      fmt.Sprintf("job-%03d", i),
			Status:       "InProgress",
			CreationTime: tie,
		})
		want[arn] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListModelInvocationJobs(&bedrock.ListModelInvocationJobsInput{NextToken: token})
		ids := make([]string, len(page))
		for i, j := range page {
			ids[i] = j.JobArn
		}

		return ids, next
	})
}

func TestListModelImportJobsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	tie := time.Now().UTC()

	want := make(map[string]bool, 3)
	for i := range 3 {
		arn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:model-import-job/m-%03d", i)
		b.SeedModelImportJobForTest(&bedrock.ModelImportJob{
			JobArn:       arn,
			JobName:      fmt.Sprintf("job-%03d", i),
			RoleArn:      "arn:aws:iam::111111111111:role/x",
			Status:       "InProgress",
			CreationTime: tie,
		})
		want[arn] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListModelImportJobs(&bedrock.ListModelImportJobsInput{MaxResults: 1, NextToken: token})
		ids := make([]string, len(page))
		for i, j := range page {
			ids[i] = j.JobArn
		}

		return ids, next
	})
}

func TestListImportedModelsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	tie := time.Now().UTC()

	const n = 105
	want := make(map[string]bool, n)
	for i := range n {
		jobArn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:model-import-job/m-%03d", i)
		modelArn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:imported-model/im-%03d", i)
		b.SeedModelImportJobForTest(&bedrock.ModelImportJob{
			JobArn:            jobArn,
			JobName:           fmt.Sprintf("job-%03d", i),
			RoleArn:           "arn:aws:iam::111111111111:role/x",
			Status:            "Completed",
			ImportedModelArn:  modelArn,
			ImportedModelName: fmt.Sprintf("imported-%03d", i),
			CreationTime:      tie,
		})
		want[modelArn] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListImportedModels(&bedrock.ListImportedModelsInput{NextToken: token})
		ids := make([]string, len(page))
		for i, m := range page {
			ids[i] = m.ImportedModelArn
		}

		return ids, next
	})
}

func TestListModelCustomizationJobsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	tie := time.Now().UTC()

	const n = 105
	want := make(map[string]bool, n)
	for i := range n {
		arn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:model-customization-job/j-%03d", i)
		b.SeedModelCustomizationJobForTest(&bedrock.ModelCustomizationJob{
			JobArn:       arn,
			JobName:      fmt.Sprintf("job-%03d", i),
			Status:       "InProgress",
			CreationTime: tie,
		})
		want[arn] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListModelCustomizationJobs(&bedrock.ListModelCustomizationJobsInput{NextToken: token})
		ids := make([]string, len(page))
		for i, j := range page {
			ids[i] = j.JobArn
		}

		return ids, next
	})
}

func TestListProvisionedModelThroughputsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	tie := time.Now().UTC()

	want := make(map[string]bool, 3)
	for i := range 3 {
		arn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:provisioned-model/p-%03d", i)
		b.SeedProvisionedModelThroughputForTest(&bedrock.ProvisionedModelThroughput{
			ProvisionedModelArn:  arn,
			ProvisionedModelName: fmt.Sprintf("pmt-%03d", i),
			Status:               "InService",
			CreationTime:         tie,
		})
		want[arn] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListProvisionedModelThroughputs(&bedrock.ListProvisionedModelThroughputsInput{
			MaxResults: 1,
			NextToken:  token,
		})
		ids := make([]string, len(page))
		for i, p := range page {
			ids[i] = p.ProvisionedModelArn
		}

		return ids, next
	})
}

func TestListAdvancedPromptOptimizationJobsSortIsTotal(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	tie := time.Now().UTC()

	want := make(map[string]bool, 3)
	for i := range 3 {
		arn := fmt.Sprintf("arn:aws:bedrock:us-east-1:111111111111:advanced-prompt-optimization-job/j-%03d", i)
		b.SeedAdvancedPromptOptimizationJobForTest(&bedrock.AdvancedPromptOptimizationJob{
			JobArn:       arn,
			JobName:      fmt.Sprintf("job-%03d", i),
			JobStatus:    "InProgress",
			CreationTime: tie,
		})
		want[arn] = true
	}

	walkAndVerify(t, want, func(token string) ([]string, string) {
		page, next := b.ListAdvancedPromptOptimizationJobs(&bedrock.ListAdvancedPromptOptimizationJobsInput{
			MaxResults: 1,
			NextToken:  token,
		})
		ids := make([]string, len(page))
		for i, j := range page {
			ids[i] = j.JobArn
		}

		return ids, next
	})
}

// TestPaginateRejectsNegativeToken proves the shared bedrock paginate()
// helper (used by ~20 List operations, including ListAgentActionGroups,
// ListDataSources, ListFlowAliases, and ListAgentAliases above) no longer
// panics on a forged/stale negative-offset NextToken. Before the fix,
// strconv.Atoi("-1") parsed cleanly and paginate never clamped it, so
// list[startIdx:end] paniced with a negative low index.
func TestPaginateRejectsNegativeToken(t *testing.T) {
	t.Parallel()

	b := bedrock.NewInMemoryBackend("111111111111", "us-east-1")
	agent, err := b.CreateAgent("agent1", "anthropic.claude-v2", "instr", "arn:aws:iam::111111111111:role/x", nil)
	require.NoError(t, err)

	_, err = b.CreateAgentActionGroup(agent.AgentID, "ag1", "", nil)
	require.NoError(t, err)

	require.NotPanics(t, func() {
		b.ListAgentActionGroups(agent.AgentID, 1, "-1")
	})
}
