package sagemaker_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sagemakersdk "github.com/aws/aws-sdk-go-v2/service/sagemaker"
	smtypes "github.com/aws/aws-sdk-go-v2/service/sagemaker/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sagemaker"
)

func TestHandler_ListTrainingJobsForHyperParameterTuningJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupJobs     func(t *testing.T, h *sagemaker.Handler)
		name          string
		jobName       string
		wantCode      int
		wantSummaries int
	}{
		{
			name:          "empty_training_jobs",
			jobName:       "my-hp-job",
			setupJobs:     func(_ *testing.T, _ *sagemaker.Handler) {},
			wantCode:      http.StatusOK,
			wantSummaries: 0,
		},
		{
			name:    "populated_training_jobs",
			jobName: "my-hp-job-2",
			setupJobs: func(t *testing.T, h *sagemaker.Handler) {
				t.Helper()
				doSageMakerRequest(t, h, "CreateTrainingJob", map[string]any{
					"TrainingJobName": "my-hp-job-2-001",
					"AlgorithmSpecification": map[string]any{
						"TrainingInputMode": "File",
					},
					"RoleArn": "arn:aws:iam::123456789012:role/SageMakerRole",
					"OutputDataConfig": map[string]any{
						"S3OutputPath": "s3://bucket/output",
					},
					"ResourceConfig": map[string]any{
						"InstanceCount":  1,
						"InstanceType":   "ml.m5.large",
						"VolumeSizeInGB": 10,
					},
					"StoppingCondition": map[string]any{
						"MaxRuntimeInSeconds": 3600,
					},
				})
			},
			wantCode:      http.StatusOK,
			wantSummaries: 1,
		},
		{
			name:          "not_found",
			jobName:       "nonexistent",
			setupJobs:     func(_ *testing.T, _ *sagemaker.Handler) {},
			wantCode:      http.StatusBadRequest,
			wantSummaries: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.wantCode == http.StatusOK {
				doSageMakerRequest(t, h, "CreateHyperParameterTuningJob", map[string]any{
					"HyperParameterTuningJobName": tt.jobName,
					"HyperParameterTuningJobConfig": map[string]any{
						"Strategy": "Bayesian",
					},
				})
			}

			tt.setupJobs(t, h)

			rec := doSageMakerRequest(t, h, "ListTrainingJobsForHyperParameterTuningJob", map[string]any{
				"HyperParameterTuningJobName": tt.jobName,
			})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				summaries, ok := resp["TrainingJobSummaries"].([]any)
				require.True(t, ok)
				assert.Len(t, summaries, tt.wantSummaries)
			}
		})
	}
}

func TestHandler_DescribeHyperParameterTuningJob_WireShape(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateHyperParameterTuningJob", map[string]any{
		"HyperParameterTuningJobName": "wire-shape-job",
		"HyperParameterTuningJobConfig": map[string]any{
			"Strategy": "Bayesian",
			"ResourceLimits": map[string]any{
				"MaxNumberOfTrainingJobs": 10,
				"MaxParallelTrainingJobs": 2,
				"MaxRuntimeInSeconds":     3600,
			},
		},
	})

	rec := doSageMakerRequest(t, h, "DescribeHyperParameterTuningJob", map[string]any{
		"HyperParameterTuningJobName": "wire-shape-job",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Real AWS nests Strategy/ResourceLimits inside HyperParameterTuningJobConfig
	// rather than emitting a flat top-level Strategy field — a client reading
	// output.HyperParameterTuningJobConfig.Strategy would get nothing from the
	// old flat shape.
	cfg, ok := resp["HyperParameterTuningJobConfig"].(map[string]any)
	require.True(t, ok, "HyperParameterTuningJobConfig must be a nested object")
	assert.Equal(t, "Bayesian", cfg["Strategy"])

	limits, ok := cfg["ResourceLimits"].(map[string]any)
	require.True(t, ok, "ResourceLimits must be nested under HyperParameterTuningJobConfig")
	assert.InDelta(t, float64(10), limits["MaxNumberOfTrainingJobs"], 0)
	assert.InDelta(t, float64(2), limits["MaxParallelTrainingJobs"], 0)
	assert.InDelta(t, float64(3600), limits["MaxRuntimeInSeconds"], 0)

	// ObjectiveStatusCounters and TrainingJobStatusCounters are both
	// "This member is required" in the real DescribeHyperParameterTuningJobOutput —
	// real AWS SDK client code dereferences them unconditionally, so the emulator
	// must always emit both objects even though this backend never launches child
	// training jobs (hence the zero counts).
	require.Contains(t, resp, "ObjectiveStatusCounters")
	objCounters, ok := resp["ObjectiveStatusCounters"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, float64(0), objCounters["Succeeded"], 0)
	assert.InDelta(t, float64(0), objCounters["Pending"], 0)
	assert.InDelta(t, float64(0), objCounters["Failed"], 0)

	require.Contains(t, resp, "TrainingJobStatusCounters")
	tjCounters, ok := resp["TrainingJobStatusCounters"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, float64(0), tjCounters["Completed"], 0)
	assert.InDelta(t, float64(0), tjCounters["InProgress"], 0)
}

func TestHandler_ListHyperParameterTuningJobs_WireShape(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateHyperParameterTuningJob", map[string]any{
		"HyperParameterTuningJobName": "wire-shape-list-job",
		"HyperParameterTuningJobConfig": map[string]any{
			"Strategy": "Random",
			"ResourceLimits": map[string]any{
				"MaxParallelTrainingJobs": 4,
			},
		},
	})

	rec := doSageMakerRequest(t, h, "ListHyperParameterTuningJobs", map[string]any{})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	summaries, ok := resp["HyperParameterTuningJobSummaries"].([]any)
	require.True(t, ok)
	require.Len(t, summaries, 1)

	summary, ok := summaries[0].(map[string]any)
	require.True(t, ok)

	// Unlike Describe, ListHyperParameterTuningJobsSummary keeps Strategy flat
	// (it is not nested under a config object in the real HyperParameterTuningJobSummary
	// shape) but ObjectiveStatusCounters/TrainingJobStatusCounters are still
	// required members.
	assert.Equal(t, "Random", summary["Strategy"])
	require.Contains(t, summary, "ObjectiveStatusCounters")
	require.Contains(t, summary, "TrainingJobStatusCounters")
	require.Contains(t, summary, "ResourceLimits")
}

func TestHandler_HyperParameterTuningJobLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create HPT job.
	recCreate := doSageMakerRequest(t, h, "CreateHyperParameterTuningJob", map[string]any{
		"HyperParameterTuningJobName": "my-hpt-job",
		"HyperParameterTuningJobConfig": map[string]any{
			"Strategy": "Bayesian",
			"ResourceLimits": map[string]any{
				"MaxNumberOfTrainingJobs": 10,
				"MaxParallelTrainingJobs": 2,
			},
		},
	})
	assert.Equal(t, http.StatusOK, recCreate.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &createOut))
	assert.NotEmpty(t, createOut["HyperParameterTuningJobArn"])

	// Describe.
	recDesc := doSageMakerRequest(t, h, "DescribeHyperParameterTuningJob", map[string]any{
		"HyperParameterTuningJobName": "my-hpt-job",
	})
	assert.Equal(t, http.StatusOK, recDesc.Code)

	// List.
	recList := doSageMakerRequest(t, h, "ListHyperParameterTuningJobs", map[string]any{})
	assert.Equal(t, http.StatusOK, recList.Code)

	var listOut map[string]any
	require.NoError(t, json.Unmarshal(recList.Body.Bytes(), &listOut))
	assert.Len(t, listOut["HyperParameterTuningJobSummaries"].([]any), 1)

	// Stop.
	recStop := doSageMakerRequest(t, h, "StopHyperParameterTuningJob", map[string]any{
		"HyperParameterTuningJobName": "my-hpt-job",
	})
	assert.Equal(t, http.StatusOK, recStop.Code)

	// Delete.
	recDelete := doSageMakerRequest(t, h, "DeleteHyperParameterTuningJob", map[string]any{
		"HyperParameterTuningJobName": "my-hpt-job",
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

// TestHandler_StopHyperParameterTuningJob_ReachesStopped asserts
// StopHyperParameterTuningJob transitions Stopping -> Stopped -- previously
// nothing advanced a Stopping job (no ticker, no later call), so
// DescribeHyperParameterTuningJob showed Stopping for the entire remaining
// lifetime of every stopped job. Correct immediately after Stop, but an
// assertion that only checks this moment cannot catch a machine that never
// advances -- confirm it actually reaches Stopped.
func TestHandler_StopHyperParameterTuningJob_ReachesStopped(t *testing.T) {
	t.Parallel()

	// CreateHyperParameterTuningJob schedules its own InProgress->Completed
	// transition immediately, so the whole body stays in one bubble.
	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		doSageMakerRequest(t, h, "CreateHyperParameterTuningJob", map[string]any{
			"HyperParameterTuningJobName": "hpt-stop-reaches",
			"HyperParameterTuningJobConfig": map[string]any{
				"Strategy":       "Bayesian",
				"ResourceLimits": map[string]any{"MaxParallelTrainingJobs": 1},
			},
		})

		rec := doSageMakerRequest(t, h, "StopHyperParameterTuningJob", map[string]any{
			"HyperParameterTuningJobName": "hpt-stop-reaches",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = doSageMakerRequest(t, h, "DescribeHyperParameterTuningJob", map[string]any{
			"HyperParameterTuningJobName": "hpt-stop-reaches",
		})
		var descResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
		assert.Equal(t, "Stopping", descResp["HyperParameterTuningJobStatus"])

		time.Sleep(time.Second)
		synctest.Wait()

		rec = doSageMakerRequest(t, h, "DescribeHyperParameterTuningJob", map[string]any{
			"HyperParameterTuningJobName": "hpt-stop-reaches",
		})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
		assert.Equal(t, "Stopped", descResp["HyperParameterTuningJobStatus"])
	})
}

// TestHandler_CreateHyperParameterTuningJob_ReachesCompleted asserts
// HyperParameterTuningJobStatus advances InProgress -> Completed, matching
// every sibling job family's own FSM (TrainingJob, ProcessingJob,
// TransformJob, InferenceRecommendationsJob, CompilationJob, ...). Previously
// nothing ever advanced it off InProgress -- only Stop's Stopping -> Stopped
// leg had an FSM -- so a job left running showed InProgress forever.
func TestHandler_CreateHyperParameterTuningJob_ReachesCompleted(t *testing.T) {
	t.Parallel()

	// CreateHyperParameterTuningJob schedules its own InProgress->Completed
	// transition immediately, so the whole body stays in one bubble.
	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		doSageMakerRequest(t, h, "CreateHyperParameterTuningJob", map[string]any{
			"HyperParameterTuningJobName": "hpt-create-reaches-completed",
			"HyperParameterTuningJobConfig": map[string]any{
				"Strategy":       "Bayesian",
				"ResourceLimits": map[string]any{"MaxParallelTrainingJobs": 1},
			},
		})

		rec := doSageMakerRequest(t, h, "DescribeHyperParameterTuningJob", map[string]any{
			"HyperParameterTuningJobName": "hpt-create-reaches-completed",
		})
		var descResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
		assert.Equal(t, "InProgress", descResp["HyperParameterTuningJobStatus"])

		time.Sleep(time.Second)
		synctest.Wait()

		rec = doSageMakerRequest(t, h, "DescribeHyperParameterTuningJob", map[string]any{
			"HyperParameterTuningJobName": "hpt-create-reaches-completed",
		})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
		assert.Equal(t, "Completed", descResp["HyperParameterTuningJobStatus"])
	})
}

// TestHandler_CreateHyperParameterTuningJob_ExtrasRoundTrip_RealClient
// asserts Autotune/WarmStartConfig/TrainingJobDefinition/
// HyperParameterTuningJobConfig's ParameterRanges/TrainingJobEarlyStoppingType
// -- previously all entirely absent from both the request decode and the
// Describe response -- now round-trip through DescribeHyperParameterTuningJob.
func TestHandler_CreateHyperParameterTuningJob_ExtrasRoundTrip_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreateHyperParameterTuningJob(t.Context(), &sagemakersdk.CreateHyperParameterTuningJobInput{
		HyperParameterTuningJobName: aws.String("hpt-extras"),
		HyperParameterTuningJobConfig: &smtypes.HyperParameterTuningJobConfig{
			Strategy:       smtypes.HyperParameterTuningJobStrategyTypeBayesian,
			ResourceLimits: &smtypes.ResourceLimits{MaxParallelTrainingJobs: aws.Int32(2)},
			ParameterRanges: &smtypes.ParameterRanges{
				IntegerParameterRanges: []smtypes.IntegerParameterRange{
					{Name: aws.String("epochs"), MinValue: aws.String("1"), MaxValue: aws.String("10")},
				},
			},
			TrainingJobEarlyStoppingType: smtypes.TrainingJobEarlyStoppingTypeAuto,
		},
		Autotune: &smtypes.Autotune{Mode: smtypes.AutotuneModeEnabled},
		TrainingJobDefinition: &smtypes.HyperParameterTrainingJobDefinition{
			AlgorithmSpecification: &smtypes.HyperParameterAlgorithmSpecification{
				TrainingInputMode: smtypes.TrainingInputModeFile,
			},
			RoleArn:          aws.String("arn:aws:iam::000000000000:role/TestRole"),
			OutputDataConfig: &smtypes.OutputDataConfig{S3OutputPath: aws.String("s3://bucket/out")},
			ResourceConfig: &smtypes.ResourceConfig{
				InstanceType:   smtypes.TrainingInstanceTypeMlM5Large,
				InstanceCount:  aws.Int32(1),
				VolumeSizeInGB: aws.Int32(20),
			},
			StoppingCondition: &smtypes.StoppingCondition{MaxRuntimeInSeconds: aws.Int32(3600)},
		},
	})
	require.NoError(t, err)

	out, err := client.DescribeHyperParameterTuningJob(t.Context(), &sagemakersdk.DescribeHyperParameterTuningJobInput{
		HyperParameterTuningJobName: aws.String("hpt-extras"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.Autotune)
	assert.Equal(t, smtypes.AutotuneModeEnabled, out.Autotune.Mode)
	require.NotNil(t, out.TrainingJobDefinition)
	assert.Equal(t, "arn:aws:iam::000000000000:role/TestRole", aws.ToString(out.TrainingJobDefinition.RoleArn))
	require.NotNil(t, out.HyperParameterTuningJobConfig.ParameterRanges)
	require.Len(t, out.HyperParameterTuningJobConfig.ParameterRanges.IntegerParameterRanges, 1)
	assert.Equal(
		t,
		"epochs",
		aws.ToString(out.HyperParameterTuningJobConfig.ParameterRanges.IntegerParameterRanges[0].Name),
	)
	assert.Equal(
		t,
		smtypes.TrainingJobEarlyStoppingTypeAuto,
		out.HyperParameterTuningJobConfig.TrainingJobEarlyStoppingType,
	)
}

// TestHandler_ListHyperParameterTuningJobs_FilterSortPage_RealClient asserts
// ListHyperParameterTuningJobsInput's NameContains, SortBy/SortOrder, and
// LastModifiedTimeAfter -- previously the handler decoded only NextToken.
func TestHandler_ListHyperParameterTuningJobs_FilterSortPage_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	names := []string{"beta-hpt", "alpha-hpt"}
	for _, n := range names {
		_, err := client.CreateHyperParameterTuningJob(t.Context(), &sagemakersdk.CreateHyperParameterTuningJobInput{
			HyperParameterTuningJobName: aws.String(n),
			HyperParameterTuningJobConfig: &smtypes.HyperParameterTuningJobConfig{
				Strategy:       smtypes.HyperParameterTuningJobStrategyTypeRandom,
				ResourceLimits: &smtypes.ResourceLimits{MaxParallelTrainingJobs: aws.Int32(1)},
			},
		})
		require.NoError(t, err)
	}

	t.Run("ascending sort by name", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListHyperParameterTuningJobs(t.Context(), &sagemakersdk.ListHyperParameterTuningJobsInput{
			SortBy: smtypes.HyperParameterTuningJobSortByOptionsName, SortOrder: smtypes.SortOrderAscending,
		})
		require.NoError(t, err)
		require.Len(t, out.HyperParameterTuningJobSummaries, 2)
		assert.Equal(t, "alpha-hpt", aws.ToString(out.HyperParameterTuningJobSummaries[0].HyperParameterTuningJobName))
		assert.Equal(t, "beta-hpt", aws.ToString(out.HyperParameterTuningJobSummaries[1].HyperParameterTuningJobName))
	})

	t.Run("name contains", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListHyperParameterTuningJobs(t.Context(), &sagemakersdk.ListHyperParameterTuningJobsInput{
			NameContains: aws.String("alpha"),
		})
		require.NoError(t, err)
		require.Len(t, out.HyperParameterTuningJobSummaries, 1)
		assert.Equal(t, "alpha-hpt", aws.ToString(out.HyperParameterTuningJobSummaries[0].HyperParameterTuningJobName))
	})

	t.Run("last modified time after future excludes", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListHyperParameterTuningJobs(t.Context(), &sagemakersdk.ListHyperParameterTuningJobsInput{
			LastModifiedTimeAfter: aws.Time(time.Now().Add(time.Hour)),
		})
		require.NoError(t, err)
		assert.Empty(t, out.HyperParameterTuningJobSummaries)
	})
}
