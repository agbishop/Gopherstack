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
)

func TestHandler_InferenceRecommendationsJobLifecycle(t *testing.T) {
	t.Parallel()

	// CreateInferenceRecommendationsJob schedules its own
	// IN_PROGRESS->COMPLETED transition immediately, so the whole body
	// stays in one bubble.
	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		// Create
		rec := doSageMakerRequest(t, h, "CreateInferenceRecommendationsJob", map[string]any{
			"JobName":        "my-rec-job",
			"JobType":        "Default",
			"JobDescription": "Test recommendation job",
			"RoleArn":        "arn:aws:iam::000000000000:role/TestRole",
			"InputConfig":    map[string]any{"ModelName": "my-model"},
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var createResp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
		assert.Contains(t, createResp["JobArn"], "my-rec-job")

		// Describe
		rec = doSageMakerRequest(t, h, "DescribeInferenceRecommendationsJob", map[string]any{
			"JobName": "my-rec-job",
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var descResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
		assert.Equal(t, "my-rec-job", descResp["JobName"])
		assert.Equal(t, "IN_PROGRESS", descResp["Status"])
		assert.Equal(t, "Default", descResp["JobType"])
		recs := descResp["InferenceRecommendations"].([]any)
		assert.Empty(t, recs)

		// List
		rec = doSageMakerRequest(t, h, "ListInferenceRecommendationsJobs", map[string]any{})
		assert.Equal(t, http.StatusOK, rec.Code)

		var listResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
		summaries := listResp["InferenceRecommendationsJobs"].([]any)
		assert.Len(t, summaries, 1)

		// List steps (always empty)
		rec = doSageMakerRequest(t, h, "ListInferenceRecommendationsJobSteps", map[string]any{
			"JobName": "my-rec-job",
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var stepsResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stepsResp))
		steps := stepsResp["Steps"].([]any)
		assert.Empty(t, steps)

		// Stop
		rec = doSageMakerRequest(t, h, "StopInferenceRecommendationsJob", map[string]any{
			"JobName": "my-rec-job",
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		rec = doSageMakerRequest(t, h, "DescribeInferenceRecommendationsJob", map[string]any{
			"JobName": "my-rec-job",
		})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
		assert.Equal(t, "STOPPING", descResp["Status"])

		// Correct immediately after Stop, but an assertion that only checks this
		// moment cannot catch a machine that never advances -- confirm it
		// actually reaches STOPPED.
		time.Sleep(time.Second)
		synctest.Wait()

		rec = doSageMakerRequest(t, h, "DescribeInferenceRecommendationsJob", map[string]any{
			"JobName": "my-rec-job",
		})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
		assert.Equal(t, "STOPPED", descResp["Status"])
	})
}

func TestHandler_InferenceRecommendationsJob_ReachesCompleted(t *testing.T) {
	t.Parallel()

	// CreateInferenceRecommendationsJob schedules its own
	// IN_PROGRESS->COMPLETED transition immediately, so the whole body
	// stays in one bubble.
	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		rec := doSageMakerRequest(t, h, "CreateInferenceRecommendationsJob", map[string]any{
			"JobName":     "rec-job-completes",
			"JobType":     "Default",
			"RoleArn":     "arn:aws:iam::000000000000:role/TestRole",
			"InputConfig": map[string]any{"ModelName": "my-model"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		time.Sleep(time.Second)
		synctest.Wait()

		descRec := doSageMakerRequest(t, h, "DescribeInferenceRecommendationsJob", map[string]any{
			"JobName": "rec-job-completes",
		})
		var out map[string]any
		require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &out))
		assert.Equal(t, "COMPLETED", out["Status"])
	})
}

func TestHandler_CreateInferenceRecommendationsJob_InputConfigRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreateInferenceRecommendationsJob", map[string]any{
		"JobName": "rec-job-input-config",
		"JobType": "Default",
		"RoleArn": "arn:aws:iam::000000000000:role/TestRole",
		"InputConfig": map[string]any{
			"ModelName": "my-model",
			"Endpoints": []any{
				map[string]any{"EndpointName": "my-endpoint"},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "DescribeInferenceRecommendationsJob", map[string]any{
		"JobName": "rec-job-input-config",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))

	inputConfig, ok := descResp["InputConfig"].(map[string]any)
	require.True(t, ok, "DescribeInferenceRecommendationsJob must return the accepted InputConfig")
	assert.Equal(t, "my-model", inputConfig["ModelName"])
}

// TestHandler_CreateInferenceRecommendationsJob_RequiredFieldsEnforced
// asserts RoleArn/InputConfig are validated -- previously neither was
// decoded at all, so a request missing both succeeded.
func TestHandler_CreateInferenceRecommendationsJob_RequiredFieldsEnforced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{name: "missing role arn", body: map[string]any{
			"JobName":     "opt-missing-role",
			"InputConfig": map[string]any{"ModelName": "m"},
		}},
		{name: "missing input config", body: map[string]any{
			"JobName": "opt-missing-input",
			"RoleArn": "arn:aws:iam::000000000000:role/TestRole",
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doSageMakerRequest(t, h, "CreateInferenceRecommendationsJob", tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestHandler_CreateInferenceRecommendationsJob_JobTypeDefault asserts
// JobType defaults to "Default" when omitted, per the op's own doc ("If left
// unspecified, ... will run ... (DEFAULT)"), rather than being rejected as a
// missing required field despite the generated struct comment saying
// required.
func TestHandler_CreateInferenceRecommendationsJob_JobTypeDefault(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreateInferenceRecommendationsJob", map[string]any{
		"JobName":     "rec-job-default-type",
		"RoleArn":     "arn:aws:iam::000000000000:role/TestRole",
		"InputConfig": map[string]any{"ModelName": "my-model"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "DescribeInferenceRecommendationsJob", map[string]any{
		"JobName": "rec-job-default-type",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "Default", resp["JobType"])
}

// TestHandler_ListInferenceRecommendationsJobs_Filters_RealClient asserts
// ListInferenceRecommendationsJobsInput's ModelNameEquals and SortBy/SortOrder
// -- previously the handler decoded only NextToken.
func TestHandler_ListInferenceRecommendationsJobs_Filters_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	names := []string{"beta-rec", "alpha-rec"}
	for _, n := range names {
		_, err := client.CreateInferenceRecommendationsJob(
			t.Context(),
			&sagemakersdk.CreateInferenceRecommendationsJobInput{
				JobName: aws.String(n),
				JobType: smtypes.RecommendationJobTypeDefault,
				RoleArn: aws.String("arn:aws:iam::000000000000:role/TestRole"),
				InputConfig: &smtypes.RecommendationJobInputConfig{
					ModelName: aws.String("shared-model"),
				},
			},
		)
		require.NoError(t, err)
	}

	t.Run("ascending sort by name", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListInferenceRecommendationsJobs(
			t.Context(),
			&sagemakersdk.ListInferenceRecommendationsJobsInput{
				SortBy: smtypes.ListInferenceRecommendationsJobsSortByName, SortOrder: smtypes.SortOrderAscending,
			},
		)
		require.NoError(t, err)
		require.Len(t, out.InferenceRecommendationsJobs, 2)
		assert.Equal(t, "alpha-rec", aws.ToString(out.InferenceRecommendationsJobs[0].JobName))
		assert.Equal(t, "beta-rec", aws.ToString(out.InferenceRecommendationsJobs[1].JobName))
	})

	t.Run("model name equals matches both", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListInferenceRecommendationsJobs(
			t.Context(),
			&sagemakersdk.ListInferenceRecommendationsJobsInput{
				ModelNameEquals: aws.String("shared-model"),
			},
		)
		require.NoError(t, err)
		assert.Len(t, out.InferenceRecommendationsJobs, 2)
	})

	t.Run("model name equals no match", func(t *testing.T) {
		t.Parallel()

		out, err := client.ListInferenceRecommendationsJobs(
			t.Context(),
			&sagemakersdk.ListInferenceRecommendationsJobsInput{
				ModelNameEquals: aws.String("other-model"),
			},
		)
		require.NoError(t, err)
		assert.Empty(t, out.InferenceRecommendationsJobs)
	})
}

func TestHandler_InferenceRecommendationsJob_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "DescribeInferenceRecommendationsJob", map[string]any{
		"JobName": "nonexistent",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_ListInferenceRecommendationsJobSteps_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "ListInferenceRecommendationsJobSteps", map[string]any{
		"JobName": "nonexistent",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// ListMlflowTrackingServers tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// AIRecommendationJob
// ---------------------------------------------------------------------------

func TestHandler_AIRecommendationJobLifecycle(t *testing.T) {
	t.Parallel()

	createBody := func(name, workloadConfig string) map[string]any {
		return map[string]any{
			"AIRecommendationJobName":    name,
			"AIWorkloadConfigIdentifier": workloadConfig,
			"RoleArn":                    "arn:aws:iam::000000000000:role/TestRole",
			"ModelSource":                map[string]any{"S3": map[string]any{"S3Uri": "s3://bucket/model/"}},
			"OutputConfig":               map[string]any{"S3OutputLocation": "s3://bucket/out/"},
			"PerformanceTarget":          map[string]any{"MetricName": "ttft-ms", "Threshold": 100},
		}
	}

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{
			name: "create rejects an unknown AIWorkloadConfigIdentifier",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)

				rec := doSageMakerRequest(t, h, "CreateAIRecommendationJob", createBody("rec-1", "nonexistent-config"))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create then describe starts InProgress with empty Recommendations",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				doSageMakerRequest(t, h, "CreateAIWorkloadConfig", map[string]any{"AIWorkloadConfigName": "wc-rec"})

				rec := doSageMakerRequest(t, h, "CreateAIRecommendationJob", createBody("rec-2", "wc-rec"))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

				var createResp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
				assert.Contains(t, createResp["AIRecommendationJobArn"], "rec-2")

				rec = doSageMakerRequest(t, h, "DescribeAIRecommendationJob", map[string]any{
					"AIRecommendationJobName": "rec-2",
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				var descResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
				assert.Equal(t, "InProgress", descResp["AIRecommendationJobStatus"])
				assert.Empty(t, descResp["Recommendations"])
			},
		},
		{
			name: "stop transitions the job to Stopping",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				doSageMakerRequest(t, h, "CreateAIWorkloadConfig", map[string]any{"AIWorkloadConfigName": "wc-stop"})
				doSageMakerRequest(t, h, "CreateAIRecommendationJob", createBody("rec-stop", "wc-stop"))

				rec := doSageMakerRequest(t, h, "StopAIRecommendationJob", map[string]any{
					"AIRecommendationJobName": "rec-stop",
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doSageMakerRequest(t, h, "DescribeAIRecommendationJob", map[string]any{
					"AIRecommendationJobName": "rec-stop",
				})
				var descResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
				assert.Equal(t, "Stopping", descResp["AIRecommendationJobStatus"])
			},
		},
		{
			name: "delete removes the job",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				doSageMakerRequest(t, h, "CreateAIWorkloadConfig", map[string]any{"AIWorkloadConfigName": "wc-del"})
				doSageMakerRequest(t, h, "CreateAIRecommendationJob", createBody("rec-del", "wc-del"))

				rec := doSageMakerRequest(t, h, "DeleteAIRecommendationJob", map[string]any{
					"AIRecommendationJobName": "rec-del",
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doSageMakerRequest(t, h, "DescribeAIRecommendationJob", map[string]any{
					"AIRecommendationJobName": "rec-del",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "list returns created jobs",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				doSageMakerRequest(t, h, "CreateAIWorkloadConfig", map[string]any{"AIWorkloadConfigName": "wc-list"})
				doSageMakerRequest(t, h, "CreateAIRecommendationJob", createBody("rec-list", "wc-list"))

				rec := doSageMakerRequest(t, h, "ListAIRecommendationJobs", map[string]any{})
				assert.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				items := resp["AIRecommendationJobs"].([]any)
				require.Len(t, items, 1)
				assert.Equal(t, "rec-list", items[0].(map[string]any)["AIRecommendationJobName"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.run(t)
		})
	}
}

// TestHandler_ListAIRecommendationJobs_DefaultSortOrder_RealClient asserts
// the op's own doc default (api_op_ListAIRecommendationJobs.go:51,55: SortBy
// CreationTime, SortOrder Descending) -- previously an unset SortBy/
// SortOrder fell through to Name/Ascending instead, the reverse of the real
// default.
func TestHandler_ListAIRecommendationJobs_DefaultSortOrder_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	doSageMakerRequest(t, h, "CreateAIWorkloadConfig", map[string]any{"AIWorkloadConfigName": "wc-rec-sort"})

	body := func(name string) map[string]any {
		return map[string]any{
			"AIRecommendationJobName":    name,
			"AIWorkloadConfigIdentifier": "wc-rec-sort",
			"RoleArn":                    "arn:aws:iam::000000000000:role/TestRole",
			"ModelSource":                map[string]any{"S3": map[string]any{"S3Uri": "s3://bucket/model/"}},
			"OutputConfig":               map[string]any{"S3OutputLocation": "s3://bucket/out/"},
			"PerformanceTarget":          map[string]any{"MetricName": "ttft-ms", "Threshold": 100},
		}
	}

	for _, name := range []string{"first-rec", "second-rec"} {
		rec := doSageMakerRequest(t, h, "CreateAIRecommendationJob", body(name))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	out, err := client.ListAIRecommendationJobs(t.Context(), &sagemakersdk.ListAIRecommendationJobsInput{})
	require.NoError(t, err)
	require.Len(t, out.AIRecommendationJobs, 2)
	assert.Equal(t, "second-rec", aws.ToString(out.AIRecommendationJobs[0].AIRecommendationJobName))
	assert.Equal(t, "first-rec", aws.ToString(out.AIRecommendationJobs[1].AIRecommendationJobName))
}

// TestHandler_CreateAIRecommendationJob_AdapterSourceRoundTrip_RealClient
// asserts AdapterSource -- previously entirely absent from decode -- now
// round-trips through DescribeAIRecommendationJob.
func TestHandler_CreateAIRecommendationJob_AdapterSourceRoundTrip_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	doSageMakerRequest(t, h, "CreateAIWorkloadConfig", map[string]any{"AIWorkloadConfigName": "wc-rec-adapter"})

	_, err := client.CreateAIRecommendationJob(t.Context(), &sagemakersdk.CreateAIRecommendationJobInput{
		AIRecommendationJobName:    aws.String("rec-adapter"),
		AIWorkloadConfigIdentifier: aws.String("wc-rec-adapter"),
		RoleArn:                    aws.String("arn:aws:iam::000000000000:role/TestRole"),
		ModelSource: &smtypes.AIModelSourceMemberS3{
			Value: smtypes.AIModelSourceS3{S3Uri: aws.String("s3://bucket/model/")},
		},
		OutputConfig: &smtypes.AIRecommendationOutputConfig{
			S3OutputLocation: aws.String("s3://bucket/out/"),
		},
		PerformanceTarget: &smtypes.AIRecommendationPerformanceTarget{
			Constraints: []smtypes.AIRecommendationConstraint{{Metric: smtypes.AIRecommendationMetricThroughput}},
		},
		AdapterSource: &smtypes.AIAdapterSourceMemberS3Uris{
			Value: []smtypes.AIAdapterS3Entry{
				{AdapterId: aws.String("adapter-1"), S3Uri: aws.String("s3://bucket/adapter/")},
			},
		},
	})
	require.NoError(t, err)

	out, err := client.DescribeAIRecommendationJob(t.Context(), &sagemakersdk.DescribeAIRecommendationJobInput{
		AIRecommendationJobName: aws.String("rec-adapter"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.AdapterSource)
	member, ok := out.AdapterSource.(*smtypes.AIAdapterSourceMemberS3Uris)
	require.True(t, ok)
	require.Len(t, member.Value, 1)
	assert.Equal(t, "s3://bucket/adapter/", aws.ToString(member.Value[0].S3Uri))
}
