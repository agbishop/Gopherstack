package sagemaker_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sagemakersdk "github.com/aws/aws-sdk-go-v2/service/sagemaker"
	smtypes "github.com/aws/aws-sdk-go-v2/service/sagemaker/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ListAndDisassociateTrialComponents(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateExperiment", map[string]any{"ExperimentName": "exp-a"})
	doSageMakerRequest(t, h, "CreateTrial", map[string]any{"TrialName": "trial-a", "ExperimentName": "exp-a"})
	doSageMakerRequest(t, h, "CreateTrialComponent", map[string]any{"TrialComponentName": "tc-a"})
	doSageMakerRequest(t, h, "CreateTrialComponent", map[string]any{"TrialComponentName": "tc-b"})
	doSageMakerRequest(t, h, "AssociateTrialComponent", map[string]any{
		"TrialName": "trial-a", "TrialComponentName": "tc-a",
	})

	// Filtering by TrialName returns only the associated component.
	recByTrial := doSageMakerRequest(t, h, "ListTrialComponents", map[string]any{"TrialName": "trial-a"})
	require.Equal(t, http.StatusOK, recByTrial.Code)

	var byTrial map[string]any
	require.NoError(t, json.Unmarshal(recByTrial.Body.Bytes(), &byTrial))
	summaries, _ := byTrial["TrialComponentSummaries"].([]any)
	require.Len(t, summaries, 1)
	assert.Equal(t, "tc-a", summaries[0].(map[string]any)["TrialComponentName"])

	// Filtering by ExperimentName joins through the trial.
	recByExp := doSageMakerRequest(t, h, "ListTrialComponents", map[string]any{"ExperimentName": "exp-a"})
	require.Equal(t, http.StatusOK, recByExp.Code)

	var byExp map[string]any
	require.NoError(t, json.Unmarshal(recByExp.Body.Bytes(), &byExp))
	expSummaries, _ := byExp["TrialComponentSummaries"].([]any)
	require.Len(t, expSummaries, 1)

	// No filter returns every trial component regardless of association.
	recAll := doSageMakerRequest(t, h, "ListTrialComponents", map[string]any{})
	require.Equal(t, http.StatusOK, recAll.Code)

	var all map[string]any
	require.NoError(t, json.Unmarshal(recAll.Body.Bytes(), &all))
	allSummaries, _ := all["TrialComponentSummaries"].([]any)
	assert.Len(t, allSummaries, 2)

	// Disassociate, then the trial-scoped list is empty.
	recDisassoc := doSageMakerRequest(t, h, "DisassociateTrialComponent", map[string]any{
		"TrialName": "trial-a", "TrialComponentName": "tc-a",
	})
	require.Equal(t, http.StatusOK, recDisassoc.Code)

	var disassocOut map[string]any
	require.NoError(t, json.Unmarshal(recDisassoc.Body.Bytes(), &disassocOut))
	assert.NotEmpty(t, disassocOut["TrialArn"])
	assert.NotEmpty(t, disassocOut["TrialComponentArn"])

	recByTrial2 := doSageMakerRequest(t, h, "ListTrialComponents", map[string]any{"TrialName": "trial-a"})
	require.Equal(t, http.StatusOK, recByTrial2.Code)

	var byTrial2 map[string]any
	require.NoError(t, json.Unmarshal(recByTrial2.Body.Bytes(), &byTrial2))
	assert.Empty(t, byTrial2["TrialComponentSummaries"])
}

func TestHandler_DisassociateTrialComponent_Idempotent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Disassociating components that were never associated still succeeds
	// and returns the resources' ARNs (mirrors AssociateTrialComponent's
	// non-strict existence checks).
	rec := doSageMakerRequest(t, h, "DisassociateTrialComponent", map[string]any{
		"TrialName": "never-existed", "TrialComponentName": "never-existed-either",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Contains(t, out["TrialArn"], "never-existed")
}

func TestHandler_TrialComponent_Lifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create trial component.
	recCreate := doSageMakerRequest(t, h, "CreateTrialComponent", map[string]any{
		"TrialComponentName": "my-component",
	})
	assert.Equal(t, http.StatusOK, recCreate.Code)

	// Describe trial component.
	recDesc := doSageMakerRequest(t, h, "DescribeTrialComponent", map[string]any{
		"TrialComponentName": "my-component",
	})
	assert.Equal(t, http.StatusOK, recDesc.Code)

	// Delete trial component.
	recDelete := doSageMakerRequest(t, h, "DeleteTrialComponent", map[string]any{
		"TrialComponentName": "my-component",
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

// TestHandler_DeleteTrialComponent_Associated asserts DeleteTrialComponent
// rejects a trial component that is still associated with a trial
// (api_op_DeleteTrialComponent.go: "A trial component must be disassociated
// from all trials before the trial component can be deleted.").
func TestHandler_DeleteTrialComponent_Associated(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateExperiment", map[string]any{"ExperimentName": "exp-tc"})
	doSageMakerRequest(t, h, "CreateTrial", map[string]any{"TrialName": "trial-tc", "ExperimentName": "exp-tc"})
	doSageMakerRequest(t, h, "CreateTrialComponent", map[string]any{"TrialComponentName": "tc-assoc"})
	doSageMakerRequest(t, h, "AssociateTrialComponent", map[string]any{
		"TrialName": "trial-tc", "TrialComponentName": "tc-assoc",
	})

	recEarly := doSageMakerRequest(t, h, "DeleteTrialComponent", map[string]any{
		"TrialComponentName": "tc-assoc",
	})
	assert.Equal(t, http.StatusBadRequest, recEarly.Code)

	recDisassociate := doSageMakerRequest(t, h, "DisassociateTrialComponent", map[string]any{
		"TrialName": "trial-tc", "TrialComponentName": "tc-assoc",
	})
	require.Equal(t, http.StatusOK, recDisassociate.Code)

	recDelete := doSageMakerRequest(t, h, "DeleteTrialComponent", map[string]any{
		"TrialComponentName": "tc-assoc",
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

func TestHandler_UpdateTrialComponent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateTrialComponent", map[string]any{
		"TrialComponentName": "my-tc",
	})

	rec := doSageMakerRequest(t, h, "UpdateTrialComponent", map[string]any{
		"TrialComponentName": "my-tc",
		"DisplayName":        "TC Display",
		"Status":             map[string]any{"PrimaryStatus": "InProgress"},
		"Parameters": map[string]any{
			"lr": map[string]any{"NumberValue": 0.001},
		},
		"InputArtifacts": map[string]any{
			"train": map[string]any{"Value": "s3://bucket/train", "MediaType": "text/csv"},
		},
		"OutputArtifacts": map[string]any{
			"model": map[string]any{"Value": "s3://bucket/model.tar.gz"},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var updateResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
	assert.NotEmpty(t, updateResp["TrialComponentArn"])

	// Describe returns updated fields
	rec = doSageMakerRequest(t, h, "DescribeTrialComponent", map[string]any{
		"TrialComponentName": "my-tc",
	})
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "TC Display", descResp["DisplayName"])
	assert.Equal(t, "InProgress", descResp["Status"].(map[string]any)["PrimaryStatus"])
	assert.NotNil(t, descResp["Parameters"])
	assert.NotNil(t, descResp["InputArtifacts"])
	assert.NotNil(t, descResp["OutputArtifacts"])
}

func TestHandler_CreateTrialComponent_FullFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreateTrialComponent", map[string]any{
		"TrialComponentName": "tc-full",
		"DisplayName":        "Training Run 1",
		"StartTime":          1700000000,
		"EndTime":            1700003600,
		"Status":             map[string]any{"PrimaryStatus": "Completed"},
		"Parameters": map[string]any{
			"lr": map[string]any{"NumberValue": 0.01},
		},
		"InputArtifacts": map[string]any{
			"train": map[string]any{"Value": "s3://bucket/train", "MediaType": "text/csv"},
		},
		"OutputArtifacts": map[string]any{
			"model": map[string]any{"Value": "s3://bucket/model.tar.gz"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "DescribeTrialComponent", map[string]any{
		"TrialComponentName": "tc-full",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "Training Run 1", descResp["DisplayName"])
	assert.InEpsilon(t, float64(1700000000), descResp["StartTime"], 0)
	assert.InEpsilon(t, float64(1700003600), descResp["EndTime"], 0)

	status, ok := descResp["Status"].(map[string]any)
	require.True(t, ok, "Status must be a {PrimaryStatus,Message} object, not a bare string")
	assert.Equal(t, "Completed", status["PrimaryStatus"])

	params, ok := descResp["Parameters"].(map[string]any)
	require.True(t, ok)
	lr, ok := params["lr"].(map[string]any)
	require.True(t, ok)
	assert.InEpsilon(t, 0.01, lr["NumberValue"], 0)
}

// TestHandler_CreateTrialComponent_MetadataProperties_RealClient asserts
// CreateTrialComponentInput.MetadataProperties -- previously entirely absent
// -- is now stored and echoed back on Describe, and that
// DescribeTrialComponentOutput.LineageGroupArn -- also previously absent -- is
// now populated with the account's single auto-provisioned default lineage
// group ARN (there is no CreateLineageGroup op; every trial component
// belongs to it).
func TestHandler_CreateTrialComponent_MetadataProperties_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreateTrialComponent(t.Context(), &sagemakersdk.CreateTrialComponentInput{
		TrialComponentName: aws.String("tc-metadata"),
		MetadataProperties: &smtypes.MetadataProperties{
			CommitId:    aws.String("abc123"),
			GeneratedBy: aws.String("pipeline-x"),
			ProjectId:   aws.String("proj-1"),
			Repository:  aws.String("git@example.com/repo"),
		},
	})
	require.NoError(t, err)

	out, err := client.DescribeTrialComponent(t.Context(), &sagemakersdk.DescribeTrialComponentInput{
		TrialComponentName: aws.String("tc-metadata"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.MetadataProperties)
	assert.Equal(t, "abc123", aws.ToString(out.MetadataProperties.CommitId))
	assert.Equal(t, "pipeline-x", aws.ToString(out.MetadataProperties.GeneratedBy))
	assert.Equal(t, "proj-1", aws.ToString(out.MetadataProperties.ProjectId))
	assert.Equal(t, "git@example.com/repo", aws.ToString(out.MetadataProperties.Repository))
	assert.Contains(t, aws.ToString(out.LineageGroupArn), "lineage-group/")
}

// TestHandler_ListTrialComponents_FilterSortPage_RealClient asserts
// ListTrialComponentsInput's SortBy/SortOrder/MaxResults/CreatedAfter/
// CreatedBefore -- previously all absent, only ExperimentName/TrialName/
// NextToken were ever read -- now work.
func TestHandler_ListTrialComponents_FilterSortPage_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	names := []string{"alpha-tc", "beta-tc", "gamma-tc"}
	for _, n := range names {
		_, err := client.CreateTrialComponent(t.Context(), &sagemakersdk.CreateTrialComponentInput{
			TrialComponentName: aws.String(n),
		})
		require.NoError(t, err)
	}

	t.Run("ascending sort by name", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListTrialComponents(t.Context(), &sagemakersdk.ListTrialComponentsInput{
			SortBy:    smtypes.SortTrialComponentsByName,
			SortOrder: smtypes.SortOrderAscending,
		})
		require.NoError(t, err)
		require.Len(t, out.TrialComponentSummaries, 3)
		assert.Equal(t, "alpha-tc", aws.ToString(out.TrialComponentSummaries[0].TrialComponentName))
		assert.Equal(t, "gamma-tc", aws.ToString(out.TrialComponentSummaries[2].TrialComponentName))
	})

	t.Run("max results caps the page and returns a token", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListTrialComponents(t.Context(), &sagemakersdk.ListTrialComponentsInput{
			MaxResults: aws.Int32(1),
			SortBy:     smtypes.SortTrialComponentsByName,
			SortOrder:  smtypes.SortOrderAscending,
		})
		require.NoError(t, err)
		require.Len(t, out.TrialComponentSummaries, 1)
		assert.Equal(t, "alpha-tc", aws.ToString(out.TrialComponentSummaries[0].TrialComponentName))
		assert.NotEmpty(t, aws.ToString(out.NextToken))
	})

	t.Run("creation time filter does not error", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListTrialComponents(t.Context(), &sagemakersdk.ListTrialComponentsInput{
			CreatedAfter: aws.Time(time.Now().Add(-time.Hour)),
		})
		require.NoError(t, err)
		assert.Len(t, out.TrialComponentSummaries, 3)
	})
}

// TestHandler_UpdateTrialComponent_RemoveLists_RealClient asserts
// UpdateTrialComponentInput's ParametersToRemove/InputArtifactsToRemove/
// OutputArtifactsToRemove -- previously entirely absent, so a real client
// could add entries but never remove one -- now delete the named keys.
func TestHandler_UpdateTrialComponent_RemoveLists_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreateTrialComponent(t.Context(), &sagemakersdk.CreateTrialComponentInput{
		TrialComponentName: aws.String("tc-remove"),
		Parameters: map[string]smtypes.TrialComponentParameterValue{
			"lr":    &smtypes.TrialComponentParameterValueMemberNumberValue{Value: 0.01},
			"epoch": &smtypes.TrialComponentParameterValueMemberNumberValue{Value: 10},
		},
		InputArtifacts: map[string]smtypes.TrialComponentArtifact{
			"train": {Value: aws.String("s3://bucket/train")},
			"test":  {Value: aws.String("s3://bucket/test")},
		},
		OutputArtifacts: map[string]smtypes.TrialComponentArtifact{
			"model": {Value: aws.String("s3://bucket/model")},
			"logs":  {Value: aws.String("s3://bucket/logs")},
		},
	})
	require.NoError(t, err)

	_, err = client.UpdateTrialComponent(t.Context(), &sagemakersdk.UpdateTrialComponentInput{
		TrialComponentName:      aws.String("tc-remove"),
		ParametersToRemove:      []string{"epoch"},
		InputArtifactsToRemove:  []string{"test"},
		OutputArtifactsToRemove: []string{"logs"},
	})
	require.NoError(t, err)

	out, err := client.DescribeTrialComponent(t.Context(), &sagemakersdk.DescribeTrialComponentInput{
		TrialComponentName: aws.String("tc-remove"),
	})
	require.NoError(t, err)
	assert.Contains(t, out.Parameters, "lr")
	assert.NotContains(t, out.Parameters, "epoch")
	assert.Contains(t, out.InputArtifacts, "train")
	assert.NotContains(t, out.InputArtifacts, "test")
	assert.Contains(t, out.OutputArtifacts, "model")
	assert.NotContains(t, out.OutputArtifacts, "logs")
}
