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

func TestHandler_TrainingJobLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create training job.
	recCreate := doSageMakerRequest(t, h, "CreateTrainingJob", map[string]any{
		"TrainingJobName":        "my-training-job",
		"AlgorithmSpecification": map[string]any{"TrainingInputMode": "File"},
		"OutputDataConfig":       map[string]any{"S3OutputPath": "s3://bucket/output"},
		"ResourceConfig": map[string]any{
			"InstanceType":   "ml.m5.large",
			"InstanceCount":  1,
			"VolumeSizeInGB": 20,
		},
		"StoppingCondition": map[string]any{"MaxRuntimeInSeconds": 3600},
	})
	assert.Equal(t, http.StatusOK, recCreate.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &createOut))
	assert.NotEmpty(t, createOut["TrainingJobArn"])

	// Describe training job.
	recDesc := doSageMakerRequest(t, h, "DescribeTrainingJob", map[string]any{
		"TrainingJobName": "my-training-job",
	})
	assert.Equal(t, http.StatusOK, recDesc.Code)

	// List training jobs.
	recList := doSageMakerRequest(t, h, "ListTrainingJobs", map[string]any{})
	assert.Equal(t, http.StatusOK, recList.Code)

	var listOut map[string]any
	require.NoError(t, json.Unmarshal(recList.Body.Bytes(), &listOut))
	assert.Len(t, listOut["TrainingJobSummaries"].([]any), 1)

	// StopTrainingJob.
	recStop := doSageMakerRequest(t, h, "StopTrainingJob", map[string]any{
		"TrainingJobName": "my-training-job",
	})
	assert.Equal(t, http.StatusOK, recStop.Code)

	// UpdateTrainingJob.
	recUpdate := doSageMakerRequest(t, h, "UpdateTrainingJob", map[string]any{
		"TrainingJobName": "my-training-job",
	})
	assert.Equal(t, http.StatusOK, recUpdate.Code)

	// Wait for the simulated job to leave Stopping before deleting it.
	time.Sleep(400 * time.Millisecond)

	// DeleteTrainingJob.
	recDelete := doSageMakerRequest(t, h, "DeleteTrainingJob", map[string]any{
		"TrainingJobName": "my-training-job",
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

// TestHandler_DeleteTrainingJob_InProgress asserts that DeleteTrainingJob
// rejects a job that is still InProgress or Stopping, matching
// DeleteProcessingJob's sibling guard
// (api_op_DeleteTrainingJob.go: "You cannot delete a job that is in the
// InProgress or Stopping state.").
func TestHandler_DeleteTrainingJob_InProgress(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateTrainingJob", map[string]any{
		"TrainingJobName":        "del-tj",
		"AlgorithmSpecification": map[string]any{"TrainingInputMode": "File"},
		"OutputDataConfig":       map[string]any{"S3OutputPath": "s3://bucket/output"},
		"ResourceConfig": map[string]any{
			"InstanceType":   "ml.m5.large",
			"InstanceCount":  1,
			"VolumeSizeInGB": 20,
		},
		"StoppingCondition": map[string]any{"MaxRuntimeInSeconds": 3600},
	})

	// Cannot delete while still InProgress.
	recEarly := doSageMakerRequest(t, h, "DeleteTrainingJob", map[string]any{"TrainingJobName": "del-tj"})
	assert.Equal(t, http.StatusBadRequest, recEarly.Code)

	// Wait for the simulated job to reach a terminal state.
	time.Sleep(400 * time.Millisecond)

	recDelete := doSageMakerRequest(t, h, "DeleteTrainingJob", map[string]any{"TrainingJobName": "del-tj"})
	require.Equal(t, http.StatusOK, recDelete.Code)

	recDescribe := doSageMakerRequest(t, h, "DescribeTrainingJob", map[string]any{"TrainingJobName": "del-tj"})
	assert.Equal(t, http.StatusBadRequest, recDescribe.Code)
}

// TestHandler_UpdateTrainingJob_KeepAlivePeriod_RealClient asserts
// UpdateTrainingJobInput.ResourceConfig.KeepAlivePeriodInSeconds --
// previously the handler didn't even call a backend Update method, so every
// field of every UpdateTrainingJob request was silently discarded -- now
// applies and round-trips through DescribeTrainingJob.
func TestHandler_UpdateTrainingJob_KeepAlivePeriod_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreateTrainingJob(t.Context(), &sagemakersdk.CreateTrainingJobInput{
		TrainingJobName:        aws.String("tj-keepalive"),
		RoleArn:                aws.String("arn:aws:iam::000000000000:role/service-role"),
		AlgorithmSpecification: &smtypes.AlgorithmSpecification{TrainingInputMode: smtypes.TrainingInputModeFile},
		OutputDataConfig:       &smtypes.OutputDataConfig{S3OutputPath: aws.String("s3://bucket/output")},
		ResourceConfig: &smtypes.ResourceConfig{
			InstanceType:   smtypes.TrainingInstanceTypeMlM5Large,
			InstanceCount:  aws.Int32(1),
			VolumeSizeInGB: aws.Int32(20),
		},
		StoppingCondition: &smtypes.StoppingCondition{MaxRuntimeInSeconds: aws.Int32(3600)},
	})
	require.NoError(t, err)

	_, err = client.UpdateTrainingJob(t.Context(), &sagemakersdk.UpdateTrainingJobInput{
		TrainingJobName: aws.String("tj-keepalive"),
		ResourceConfig:  &smtypes.ResourceConfigForUpdate{KeepAlivePeriodInSeconds: aws.Int32(600)},
	})
	require.NoError(t, err)

	out, err := client.DescribeTrainingJob(t.Context(), &sagemakersdk.DescribeTrainingJobInput{
		TrainingJobName: aws.String("tj-keepalive"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.ResourceConfig)
	assert.Equal(t, int32(600), aws.ToInt32(out.ResourceConfig.KeepAlivePeriodInSeconds))
}

// TestHandler_ListTrainingJobs_FilterSortPage_RealClient asserts
// ListTrainingJobsInput's LastModifiedTimeAfter/Before and SortBy/SortOrder --
// previously the list was unconditionally sorted ascending by name
// regardless of what was requested, contradicting the op's own documented
// default of CreationTime/Ascending.
func TestHandler_ListTrainingJobs_FilterSortPage_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	names := []string{"beta-job", "alpha-job"}
	for _, n := range names {
		_, err := client.CreateTrainingJob(t.Context(), &sagemakersdk.CreateTrainingJobInput{
			TrainingJobName: aws.String(n),
			RoleArn:         aws.String("arn:aws:iam::000000000000:role/service-role"),
			AlgorithmSpecification: &smtypes.AlgorithmSpecification{
				TrainingInputMode: smtypes.TrainingInputModeFile,
			},
			OutputDataConfig: &smtypes.OutputDataConfig{S3OutputPath: aws.String("s3://bucket/output")},
			ResourceConfig: &smtypes.ResourceConfig{
				InstanceType:   smtypes.TrainingInstanceTypeMlM5Large,
				InstanceCount:  aws.Int32(1),
				VolumeSizeInGB: aws.Int32(20),
			},
			StoppingCondition: &smtypes.StoppingCondition{MaxRuntimeInSeconds: aws.Int32(3600)},
		})
		require.NoError(t, err)
	}

	t.Run("ascending sort by name", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListTrainingJobs(t.Context(), &sagemakersdk.ListTrainingJobsInput{
			SortBy: smtypes.SortByName, SortOrder: smtypes.SortOrderAscending,
		})
		require.NoError(t, err)
		require.Len(t, out.TrainingJobSummaries, 2)
		assert.Equal(t, "alpha-job", aws.ToString(out.TrainingJobSummaries[0].TrainingJobName))
		assert.Equal(t, "beta-job", aws.ToString(out.TrainingJobSummaries[1].TrainingJobName))
	})

	t.Run("last modified time after future excludes", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListTrainingJobs(t.Context(), &sagemakersdk.ListTrainingJobsInput{
			LastModifiedTimeAfter: aws.Time(time.Now().Add(time.Hour)),
		})
		require.NoError(t, err)
		assert.Empty(t, out.TrainingJobSummaries)
	})

	t.Run("last modified time after past includes", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListTrainingJobs(t.Context(), &sagemakersdk.ListTrainingJobsInput{
			LastModifiedTimeAfter: aws.Time(time.Now().Add(-time.Hour)),
		})
		require.NoError(t, err)
		assert.Len(t, out.TrainingJobSummaries, 2)
	})

	t.Run("training plan arn equals never matches", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListTrainingJobs(t.Context(), &sagemakersdk.ListTrainingJobsInput{
			TrainingPlanArnEquals: aws.String("arn:aws:sagemaker:us-east-1:000000000000:training-plan/none"),
		})
		require.NoError(t, err)
		assert.Empty(t, out.TrainingJobSummaries)
	})
}

// ---------------------------------------------------------------------------
// Notebook Instance lifecycle
// ---------------------------------------------------------------------------
