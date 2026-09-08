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

func TestHandler_ExperimentLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create experiment.
	recCreate := doSageMakerRequest(t, h, "CreateExperiment", map[string]any{
		"ExperimentName": "my-experiment",
	})
	assert.Equal(t, http.StatusOK, recCreate.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &createOut))
	assert.NotEmpty(t, createOut["ExperimentArn"])

	// Describe experiment.
	recDesc := doSageMakerRequest(t, h, "DescribeExperiment", map[string]any{
		"ExperimentName": "my-experiment",
	})
	assert.Equal(t, http.StatusOK, recDesc.Code)

	// List experiments.
	recList := doSageMakerRequest(t, h, "ListExperiments", map[string]any{})
	assert.Equal(t, http.StatusOK, recList.Code)

	var listOut map[string]any
	require.NoError(t, json.Unmarshal(recList.Body.Bytes(), &listOut))
	assert.Len(t, listOut["ExperimentSummaries"].([]any), 1)

	// Delete experiment.
	recDelete := doSageMakerRequest(t, h, "DeleteExperiment", map[string]any{
		"ExperimentName": "my-experiment",
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

// TestHandler_DeleteExperiment_HasTrials asserts DeleteExperiment rejects an
// experiment that still has an associated trial
// (api_op_DeleteExperiment.go: "All trials associated with the experiment
// must be deleted first.").
func TestHandler_DeleteExperiment_HasTrials(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateExperiment", map[string]any{
		"ExperimentName": "exp-with-trial",
	})
	doSageMakerRequest(t, h, "CreateTrial", map[string]any{
		"TrialName":      "trial-in-exp",
		"ExperimentName": "exp-with-trial",
	})

	recEarly := doSageMakerRequest(t, h, "DeleteExperiment", map[string]any{
		"ExperimentName": "exp-with-trial",
	})
	assert.Equal(t, http.StatusBadRequest, recEarly.Code)

	recDeleteTrial := doSageMakerRequest(t, h, "DeleteTrial", map[string]any{
		"TrialName": "trial-in-exp",
	})
	require.Equal(t, http.StatusOK, recDeleteTrial.Code)

	recDelete := doSageMakerRequest(t, h, "DeleteExperiment", map[string]any{
		"ExperimentName": "exp-with-trial",
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

func TestHandler_Experiment_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, op := range []string{"DescribeExperiment", "DeleteExperiment"} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()

			rec := doSageMakerRequest(t, h, op, map[string]any{"ExperimentName": "nonexistent"})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_UpdateExperiment(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateExperiment", map[string]any{
		"ExperimentName": "my-exp",
	})

	rec := doSageMakerRequest(t, h, "UpdateExperiment", map[string]any{
		"ExperimentName": "my-exp",
		"DisplayName":    "My Experiment",
		"Description":    "A test experiment",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var updateResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
	assert.NotEmpty(t, updateResp["ExperimentArn"])

	// Describe returns updated fields
	rec = doSageMakerRequest(t, h, "DescribeExperiment", map[string]any{
		"ExperimentName": "my-exp",
	})
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "My Experiment", descResp["DisplayName"])
	assert.Equal(t, "A test experiment", descResp["Description"])
}

func TestHandler_UpdateExperiment_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "UpdateExperiment", map[string]any{
		"ExperimentName": "nonexistent",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_Tags_Experiment(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreateExperiment", map[string]any{
		"ExperimentName": "tagged-exp",
		"Tags": []any{
			map[string]any{"Key": "project", "Value": "ml"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	expARN := createResp["ExperimentArn"]
	require.NotEmpty(t, expARN)

	rec = doSageMakerRequest(t, h, "ListTags", map[string]any{
		"ResourceArn": expARN,
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var tagsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tagsResp))
	tags := tagsResp["Tags"].([]any)
	require.Len(t, tags, 1)
	assert.Equal(t, "project", tags[0].(map[string]any)["Key"])
}

func TestHandler_CreateExperiment_DisplayNameAndDescription(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreateExperiment", map[string]any{
		"ExperimentName": "exp-with-display",
		"DisplayName":    "Friendly Name",
		"Description":    "An experiment created with a display name",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "DescribeExperiment", map[string]any{
		"ExperimentName": "exp-with-display",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "Friendly Name", descResp["DisplayName"])
	assert.Equal(t, "An experiment created with a display name", descResp["Description"])

	rec = doSageMakerRequest(t, h, "ListExperiments", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	summaries := listResp["ExperimentSummaries"].([]any)
	require.Len(t, summaries, 1)
	assert.Equal(t, "Friendly Name", summaries[0].(map[string]any)["DisplayName"])
}

// TestHandler_ListExperiments_DefaultSortOrder_RealClient asserts the op's
// own doc default (api_op_ListExperiments.go:48,51: SortBy CreationTime,
// SortOrder Descending) -- previously this decoded only NextToken and
// dropped every sort/filter control, always returning Name/Ascending order.
func TestHandler_ListExperiments_DefaultSortOrder_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	for _, name := range []string{"first-exp", "second-exp"} {
		_, err := client.CreateExperiment(t.Context(), &sagemakersdk.CreateExperimentInput{
			ExperimentName: aws.String(name),
		})
		require.NoError(t, err)
	}

	out, err := client.ListExperiments(t.Context(), &sagemakersdk.ListExperimentsInput{})
	require.NoError(t, err)
	require.Len(t, out.ExperimentSummaries, 2)
	assert.Equal(t, "second-exp", aws.ToString(out.ExperimentSummaries[0].ExperimentName))
	assert.Equal(t, "first-exp", aws.ToString(out.ExperimentSummaries[1].ExperimentName))
}

// TestHandler_UpdateExperiment_ClearsDescription_RealClient asserts an
// explicit empty Description clears it -- previously UpdateExperiment
// decoded DisplayName/Description as plain (non-pointer) strings, so an
// omitted key and an explicit "" were indistinguishable and the op's own
// doc ("adds, updates, or removes the description") could never remove one.
func TestHandler_UpdateExperiment_ClearsDescription_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreateExperiment(t.Context(), &sagemakersdk.CreateExperimentInput{
		ExperimentName: aws.String("exp-clear-desc"),
		Description:    aws.String("will be cleared"),
	})
	require.NoError(t, err)

	_, err = client.UpdateExperiment(t.Context(), &sagemakersdk.UpdateExperimentInput{
		ExperimentName: aws.String("exp-clear-desc"),
		Description:    aws.String(""),
	})
	require.NoError(t, err)

	out, err := client.DescribeExperiment(t.Context(), &sagemakersdk.DescribeExperimentInput{
		ExperimentName: aws.String("exp-clear-desc"),
	})
	require.NoError(t, err)
	assert.Empty(t, aws.ToString(out.Description))
}

// TestHandler_UpdateExperiment_OmittedFieldsLeaveUnchanged_RealClient
// asserts that omitting DisplayName/Description on Update leaves the
// existing values untouched, the complement of the clear-semantics test
// above.
func TestHandler_UpdateExperiment_OmittedFieldsLeaveUnchanged_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreateExperiment(t.Context(), &sagemakersdk.CreateExperimentInput{
		ExperimentName: aws.String("exp-keep-desc"),
		DisplayName:    aws.String("Original Display"),
		Description:    aws.String("Original description"),
	})
	require.NoError(t, err)

	_, err = client.UpdateExperiment(t.Context(), &sagemakersdk.UpdateExperimentInput{
		ExperimentName: aws.String("exp-keep-desc"),
	})
	require.NoError(t, err)

	out, err := client.DescribeExperiment(t.Context(), &sagemakersdk.DescribeExperimentInput{
		ExperimentName: aws.String("exp-keep-desc"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Original Display", aws.ToString(out.DisplayName))
	assert.Equal(t, "Original description", aws.ToString(out.Description))
}

// TestHandler_ListExperiments_CreatedAfterFilter_RealClient asserts
// CreatedAfter -- previously absent from decode entirely, along with every
// other filter/sort control ListExperimentsInput declares.
func TestHandler_ListExperiments_CreatedAfterFilter_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreateExperiment(t.Context(), &sagemakersdk.CreateExperimentInput{
		ExperimentName: aws.String("exp-past"),
	})
	require.NoError(t, err)

	out, err := client.ListExperiments(t.Context(), &sagemakersdk.ListExperimentsInput{
		CreatedAfter: aws.Time(time.Now().Add(time.Hour)),
	})
	require.NoError(t, err)
	assert.Empty(t, out.ExperimentSummaries)

	out, err = client.ListExperiments(t.Context(), &sagemakersdk.ListExperimentsInput{
		CreatedAfter: aws.Time(time.Now().Add(-time.Hour)),
		SortBy:       smtypes.SortExperimentsByName,
		SortOrder:    smtypes.SortOrderAscending,
	})
	require.NoError(t, err)
	require.Len(t, out.ExperimentSummaries, 1)
	assert.Equal(t, "exp-past", aws.ToString(out.ExperimentSummaries[0].ExperimentName))
}
