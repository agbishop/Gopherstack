package sagemaker_test

import (
	"context"
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

func TestHandler_CreateCompilationJob(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreateCompilationJob", map[string]any{
		"CompilationJobName": "my-compile",
		"RoleArn":            "arn:test",
		"OutputConfig":       map[string]any{"S3OutputLocation": "s3://bucket/out/"},
		"StoppingCondition":  map[string]any{"MaxRuntimeInSeconds": 3600},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["CompilationJobArn"], "my-compile")
}

// TestHandler_CreateCompilationJob_RequiredFieldsEnforced asserts
// RoleArn/OutputConfig/StoppingCondition (all "This member is required" per
// api_op_CreateCompilationJob.go:60,98,102) are each independently
// rejected when absent -- previously none of the three were validated, so
// a request supplying none of them still succeeded (see
// TestHandler_CreateCompilationJob above, before this pass).
func TestHandler_CreateCompilationJob_RequiredFieldsEnforced(t *testing.T) {
	t.Parallel()

	validBody := map[string]any{
		"CompilationJobName": "cj-required",
		"RoleArn":            "arn:test",
		"OutputConfig":       map[string]any{"S3OutputLocation": "s3://bucket/out/"},
		"StoppingCondition":  map[string]any{"MaxRuntimeInSeconds": 3600},
	}

	tests := []struct {
		name   string
		remove string
	}{
		{name: "missing role arn", remove: "RoleArn"},
		{name: "missing output config", remove: "OutputConfig"},
		{name: "missing stopping condition", remove: "StoppingCondition"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			body := make(map[string]any, len(validBody))
			for k, v := range validBody {
				if k != tt.remove {
					body[k] = v
				}
			}

			rec := doSageMakerRequest(t, h, "CreateCompilationJob", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_DescribeCompilationJob(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(
		t,
		h,
		"CreateCompilationJob",
		map[string]any{
			"CompilationJobName": "cj-1",
			"RoleArn":            "arn:test",
			"OutputConfig":       map[string]any{"S3OutputLocation": "s3://bucket/out/"},
			"StoppingCondition":  map[string]any{"MaxRuntimeInSeconds": 3600},
		},
	)

	rec := doSageMakerRequest(t, h, "DescribeCompilationJob", map[string]any{"CompilationJobName": "cj-1"})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "cj-1", resp["CompilationJobName"])
	assert.Equal(t, "INPROGRESS", resp["CompilationJobStatus"])
}

// TestHandler_StopCompilationJob asserts the real INPROGRESS -> STOPPING ->
// STOPPED sequence (api_op_StopCompilationJob.go:16-19) -- previously Stop
// set STOPPED directly, so a client would never observe STOPPING at all.
func TestHandler_StopCompilationJob(t *testing.T) {
	t.Parallel()

	// CreateCompilationJob itself schedules a delayed InProgress->Completed
	// transition, so the whole body has to stay in one bubble from the
	// start: runDelayed's goroutine touches the backend's WaitGroup, and
	// that must not happen both inside and outside a bubble.
	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		doSageMakerRequest(
			t,
			h,
			"CreateCompilationJob",
			map[string]any{
				"CompilationJobName": "cj-stop",
				"RoleArn":            "arn:test",
				"OutputConfig":       map[string]any{"S3OutputLocation": "s3://bucket/out/"},
				"StoppingCondition":  map[string]any{"MaxRuntimeInSeconds": 3600},
			},
		)
		rec := doSageMakerRequest(t, h, "StopCompilationJob", map[string]any{"CompilationJobName": "cj-stop"})
		assert.Equal(t, http.StatusOK, rec.Code)

		rec = doSageMakerRequest(t, h, "DescribeCompilationJob", map[string]any{"CompilationJobName": "cj-stop"})
		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "STOPPING", resp["CompilationJobStatus"])

		time.Sleep(time.Second)
		synctest.Wait()

		rec = doSageMakerRequest(t, h, "DescribeCompilationJob", map[string]any{"CompilationJobName": "cj-stop"})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "STOPPED", resp["CompilationJobStatus"])
	})
}

func TestHandler_ListCompilationJobs(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, name := range []string{"cj-a", "cj-b"} {
		doSageMakerRequest(
			t,
			h,
			"CreateCompilationJob",
			map[string]any{
				"CompilationJobName": name,
				"RoleArn":            "arn:test",
				"OutputConfig":       map[string]any{"S3OutputLocation": "s3://bucket/out/"},
				"StoppingCondition":  map[string]any{"MaxRuntimeInSeconds": 3600},
			},
		)
	}

	rec := doSageMakerRequest(t, h, "ListCompilationJobs", map[string]any{})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items := resp["CompilationJobSummaries"].([]any)
	assert.Len(t, items, 2)
}

// ---------------------------------------------------------------------------
// MonitoringSchedule
// ---------------------------------------------------------------------------

func TestCompilationJob_InputOutputConfigRoundtrip(t *testing.T) {
	t.Parallel()

	b := sagemaker.NewInMemoryBackend("000000000000", "us-east-1")
	ctx := context.Background()

	_, err := b.CreateCompilationJob(ctx, "roundtrip-job", "arn:aws:iam::123456789012:role/Neo", nil)
	require.NoError(t, err)

	inputCfg := &sagemaker.CompilationInputConfig{
		S3Uri:     "s3://my-bucket/model.tar.gz",
		Framework: "TENSORFLOW",
	}
	outputCfg := &sagemaker.CompilationOutputConfig{
		S3OutputLocation: "s3://my-bucket/output/",
		TargetDevice:     "ml_c5",
	}
	sc := &sagemaker.StoppingCondition{MaxRuntimeInSeconds: 300}

	err = b.SetCompilationJobExtras(ctx, "roundtrip-job", inputCfg, outputCfg, sc, "", nil)
	require.NoError(t, err)

	got, err := b.DescribeCompilationJob(ctx, "roundtrip-job")
	require.NoError(t, err)

	require.NotNil(t, got.InputConfig, "InputConfig must be persisted")
	assert.Equal(t, "s3://my-bucket/model.tar.gz", got.InputConfig.S3Uri)
	assert.Equal(t, "TENSORFLOW", got.InputConfig.Framework)

	require.NotNil(t, got.OutputConfig, "OutputConfig must be persisted")
	assert.Equal(t, "s3://my-bucket/output/", got.OutputConfig.S3OutputLocation)
	assert.Equal(t, "ml_c5", got.OutputConfig.TargetDevice)

	require.NotNil(t, got.StoppingCondition, "StoppingCondition must be persisted")
	assert.Equal(t, int32(300), got.StoppingCondition.MaxRuntimeInSeconds)
}

// TestCompilationJob_HandlerCapturesInputOutputConfig verifies that the HTTP handler
// passes InputConfig and OutputConfig through to the backend on creation.

func TestCompilationJob_HandlerCapturesInputOutputConfig(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doSageMakerRequest(t, h, "CreateCompilationJob", map[string]any{
		"CompilationJobName": "handler-roundtrip-job",
		"RoleArn":            "arn:aws:iam::123456789012:role/Neo",
		"InputConfig": map[string]any{
			"S3Uri":     "s3://bucket/model.tar.gz",
			"Framework": "PYTORCH",
		},
		"OutputConfig": map[string]any{
			"S3OutputLocation": "s3://bucket/out/",
			"TargetDevice":     "jetson_nano",
		},
		"StoppingCondition": map[string]any{
			"MaxRuntimeInSeconds": 600,
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code,
		"CreateCompilationJob failed: %s", createRec.Body.String())

	descRec := doSageMakerRequest(t, h, "DescribeCompilationJob", map[string]any{
		"CompilationJobName": "handler-roundtrip-job",
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var out struct {
		InputConfig *struct {
			S3Uri     string `json:"S3Uri"`
			Framework string `json:"Framework"`
		} `json:"InputConfig"`
		OutputConfig *struct {
			S3OutputLocation string `json:"S3OutputLocation"`
			TargetDevice     string `json:"TargetDevice"`
		} `json:"OutputConfig"`
		StoppingCondition *struct {
			MaxRuntimeInSeconds int32 `json:"MaxRuntimeInSeconds"`
		} `json:"StoppingCondition"`
	}
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &out))

	require.NotNil(t, out.InputConfig, "InputConfig must be returned by DescribeCompilationJob")
	assert.Equal(t, "s3://bucket/model.tar.gz", out.InputConfig.S3Uri)
	assert.Equal(t, "PYTORCH", out.InputConfig.Framework)

	require.NotNil(t, out.OutputConfig, "OutputConfig must be returned by DescribeCompilationJob")
	assert.Equal(t, "s3://bucket/out/", out.OutputConfig.S3OutputLocation)
	assert.Equal(t, "jetson_nano", out.OutputConfig.TargetDevice)

	require.NotNil(t, out.StoppingCondition, "StoppingCondition must be returned by DescribeCompilationJob")
	assert.Equal(t, int32(600), out.StoppingCondition.MaxRuntimeInSeconds)
}

// TestAutoMLJob_OutputDataConfigRoundtrip verifies that OutputDataConfig and
// AutoMLJobObjective provided at CreateAutoMLJob are persisted and returned by
// DescribeAutoMLJob. Real AWS stores and returns these fields.

func TestCompilationJob_InitialStatus_InProgress(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateCompilationJob", map[string]any{
		"CompilationJobName": "compile-status",
		"RoleArn":            "arn:test",
		"OutputConfig":       map[string]any{"S3OutputLocation": "s3://bucket/out/"},
		"StoppingCondition":  map[string]any{"MaxRuntimeInSeconds": 3600},
	})

	rec := doSageMakerRequest(t, h, "DescribeCompilationJob", map[string]any{
		"CompilationJobName": "compile-status",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "INPROGRESS", resp["CompilationJobStatus"])
}

func TestStopCompilationJob_Terminal_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateCompilationJob", map[string]any{
		"CompilationJobName": "compile-terminal",
		"RoleArn":            "arn:test",
		"OutputConfig":       map[string]any{"S3OutputLocation": "s3://bucket/out/"},
		"StoppingCondition":  map[string]any{"MaxRuntimeInSeconds": 3600},
	})
	doSageMakerRequest(t, h, "StopCompilationJob", map[string]any{
		"CompilationJobName": "compile-terminal",
	})

	rec := doSageMakerRequest(t, h, "StopCompilationJob", map[string]any{
		"CompilationJobName": "compile-terminal",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// Image: version guard on delete
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// AIBenchmarkJob
// ---------------------------------------------------------------------------

func TestHandler_AIBenchmarkJobLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{
			name: "create rejects an unknown AIWorkloadConfigIdentifier",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)

				rec := doSageMakerRequest(t, h, "CreateAIBenchmarkJob", map[string]any{
					"AIBenchmarkJobName":         "bench-1",
					"AIWorkloadConfigIdentifier": "nonexistent-config",
					"RoleArn":                    "arn:aws:iam::000000000000:role/TestRole",
					"BenchmarkTarget": map[string]any{
						"Endpoint": map[string]any{"Identifier": "my-endpoint"},
					},
					"OutputConfig": map[string]any{"S3OutputLocation": "s3://bucket/out/"},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create then describe starts InProgress, stop transitions to Stopping",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				doSageMakerRequest(t, h, "CreateAIWorkloadConfig", map[string]any{"AIWorkloadConfigName": "wc-1"})

				rec := doSageMakerRequest(t, h, "CreateAIBenchmarkJob", map[string]any{
					"AIBenchmarkJobName":         "bench-2",
					"AIWorkloadConfigIdentifier": "wc-1",
					"RoleArn":                    "arn:aws:iam::000000000000:role/TestRole",
					"BenchmarkTarget": map[string]any{
						"Endpoint": map[string]any{"Identifier": "my-endpoint"},
					},
					"OutputConfig": map[string]any{"S3OutputLocation": "s3://bucket/out/"},
				})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

				var createResp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
				assert.Contains(t, createResp["AIBenchmarkJobArn"], "bench-2")

				rec = doSageMakerRequest(
					t,
					h,
					"DescribeAIBenchmarkJob",
					map[string]any{"AIBenchmarkJobName": "bench-2"},
				)
				assert.Equal(t, http.StatusOK, rec.Code)

				var descResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
				assert.Equal(t, "InProgress", descResp["AIBenchmarkJobStatus"])
				assert.Equal(t, "wc-1", descResp["AIWorkloadConfigIdentifier"])
				assert.NotEmpty(t, descResp["BenchmarkTarget"])
				assert.NotEmpty(t, descResp["OutputConfig"])

				rec = doSageMakerRequest(t, h, "StopAIBenchmarkJob", map[string]any{"AIBenchmarkJobName": "bench-2"})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doSageMakerRequest(
					t,
					h,
					"DescribeAIBenchmarkJob",
					map[string]any{"AIBenchmarkJobName": "bench-2"},
				)
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
				assert.Equal(t, "Stopping", descResp["AIBenchmarkJobStatus"])
			},
		},
		{
			name: "delete removes the job",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				doSageMakerRequest(t, h, "CreateAIWorkloadConfig", map[string]any{"AIWorkloadConfigName": "wc-del"})
				doSageMakerRequest(t, h, "CreateAIBenchmarkJob", map[string]any{
					"AIBenchmarkJobName":         "bench-del",
					"AIWorkloadConfigIdentifier": "wc-del",
					"RoleArn":                    "arn:aws:iam::000000000000:role/TestRole",
					"BenchmarkTarget": map[string]any{
						"Endpoint": map[string]any{"Identifier": "my-endpoint"},
					},
					"OutputConfig": map[string]any{"S3OutputLocation": "s3://bucket/out/"},
				})

				rec := doSageMakerRequest(
					t,
					h,
					"DeleteAIBenchmarkJob",
					map[string]any{"AIBenchmarkJobName": "bench-del"},
				)
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doSageMakerRequest(
					t,
					h,
					"DescribeAIBenchmarkJob",
					map[string]any{"AIBenchmarkJobName": "bench-del"},
				)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "list returns created jobs with a derived AIWorkloadConfigName",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				doSageMakerRequest(t, h, "CreateAIWorkloadConfig", map[string]any{"AIWorkloadConfigName": "wc-list"})
				doSageMakerRequest(t, h, "CreateAIBenchmarkJob", map[string]any{
					"AIBenchmarkJobName":         "bench-list",
					"AIWorkloadConfigIdentifier": "wc-list",
					"RoleArn":                    "arn:aws:iam::000000000000:role/TestRole",
					"BenchmarkTarget": map[string]any{
						"Endpoint": map[string]any{"Identifier": "my-endpoint"},
					},
					"OutputConfig": map[string]any{"S3OutputLocation": "s3://bucket/out/"},
				})

				rec := doSageMakerRequest(t, h, "ListAIBenchmarkJobs", map[string]any{})
				assert.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				items := resp["AIBenchmarkJobs"].([]any)
				require.Len(t, items, 1)
				item := items[0].(map[string]any)
				assert.Equal(t, "bench-list", item["AIBenchmarkJobName"])
				assert.Equal(t, "wc-list", item["AIWorkloadConfigName"])
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

// TestHandler_ListAIBenchmarkJobs_DefaultSortOrder_RealClient asserts the
// op's own doc default (api_op_ListAIBenchmarkJobs.go:44,49: SortBy
// CreationTime, SortOrder Descending) -- previously an unset SortBy/
// SortOrder fell through to Name/Ascending instead, the reverse of the
// real default.
func TestHandler_ListAIBenchmarkJobs_DefaultSortOrder_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	doSageMakerRequest(t, h, "CreateAIWorkloadConfig", map[string]any{"AIWorkloadConfigName": "wc-sort"})

	for _, name := range []string{"first-bench", "second-bench"} {
		rec := doSageMakerRequest(t, h, "CreateAIBenchmarkJob", map[string]any{
			"AIBenchmarkJobName":         name,
			"AIWorkloadConfigIdentifier": "wc-sort",
			"RoleArn":                    "arn:aws:iam::000000000000:role/TestRole",
			"BenchmarkTarget": map[string]any{
				"Endpoint": map[string]any{"Identifier": "my-endpoint"},
			},
			"OutputConfig": map[string]any{"S3OutputLocation": "s3://bucket/out/"},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	out, err := client.ListAIBenchmarkJobs(t.Context(), &sagemakersdk.ListAIBenchmarkJobsInput{})
	require.NoError(t, err)
	require.Len(t, out.AIBenchmarkJobs, 2)
	assert.Equal(t, "second-bench", aws.ToString(out.AIBenchmarkJobs[0].AIBenchmarkJobName))
	assert.Equal(t, "first-bench", aws.ToString(out.AIBenchmarkJobs[1].AIBenchmarkJobName))
}

// TestHandler_CompilationJob_ExtrasRoundTrip_RealClient asserts
// ModelPackageVersionArn/VpcConfig -- previously entirely absent from
// decode -- now round-trip through DescribeCompilationJob.
func TestHandler_CompilationJob_ExtrasRoundTrip_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreateCompilationJob(t.Context(), &sagemakersdk.CreateCompilationJobInput{
		CompilationJobName:     aws.String("cj-extras"),
		RoleArn:                aws.String("arn:aws:iam::000000000000:role/TestRole"),
		OutputConfig:           &smtypes.OutputConfig{S3OutputLocation: aws.String("s3://bucket/out")},
		StoppingCondition:      &smtypes.StoppingCondition{MaxRuntimeInSeconds: aws.Int32(3600)},
		ModelPackageVersionArn: aws.String("arn:aws:sagemaker:us-east-1:000000000000:model-package/pkg/1"),
		VpcConfig: &smtypes.NeoVpcConfig{
			SecurityGroupIds: []string{"sg-1"},
			Subnets:          []string{"subnet-1"},
		},
	})
	require.NoError(t, err)

	out, err := client.DescribeCompilationJob(t.Context(), &sagemakersdk.DescribeCompilationJobInput{
		CompilationJobName: aws.String("cj-extras"),
	})
	require.NoError(t, err)
	assert.Equal(
		t,
		"arn:aws:sagemaker:us-east-1:000000000000:model-package/pkg/1",
		aws.ToString(out.ModelPackageVersionArn),
	)
	require.NotNil(t, out.VpcConfig)
	assert.Equal(t, []string{"sg-1"}, out.VpcConfig.SecurityGroupIds)
	assert.Equal(t, []string{"subnet-1"}, out.VpcConfig.Subnets)
}

// TestHandler_CompilationJob_ReachesCompleted_RealClient asserts
// CompilationJobStatus advances INPROGRESS -> COMPLETED on its own --
// previously nothing ever advanced it past INPROGRESS unless a client
// called StopCompilationJob -- and that ModelArtifacts (a required
// DescribeCompilationJobOutput member, types.go/api_op_DescribeCompilationJob.go:88)
// is populated once it does, derived from OutputConfig.S3OutputLocation.
func TestHandler_CompilationJob_ReachesCompleted_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreateCompilationJob(t.Context(), &sagemakersdk.CreateCompilationJobInput{
		CompilationJobName: aws.String("cj-completes"),
		RoleArn:            aws.String("arn:aws:iam::000000000000:role/TestRole"),
		OutputConfig:       &smtypes.OutputConfig{S3OutputLocation: aws.String("s3://bucket/out")},
		StoppingCondition:  &smtypes.StoppingCondition{MaxRuntimeInSeconds: aws.Int32(3600)},
	})
	require.NoError(t, err)

	out, err := client.DescribeCompilationJob(t.Context(), &sagemakersdk.DescribeCompilationJobInput{
		CompilationJobName: aws.String("cj-completes"),
	})
	require.NoError(t, err)
	assert.Equal(t, smtypes.CompilationJobStatusInprogress, out.CompilationJobStatus)

	// require.Eventually stays here deliberately (gopherstack-k3ae): wrapping
	// this test in synctest.Test, including newTestSageMakerClient's real
	// httptest.NewServer, deadlocks. The accept/read/write goroutines behind
	// the real socket join the bubble (they're spawned from inside it) but
	// block on real network I/O, which synctest does not count as "durably
	// blocked" -- so the bubble never goes quiescent, the fake clock never
	// advances, and runDelayed's timer for the InProgress->Completed
	// transition never fires. Confirmed: 35s of zero output, goroutine dump
	// showing accept/read/write parked in real IO wait, not "(durable)".
	// The callback this polls already re-checks CompilationJobStatus before
	// writing (compilation_jobs.go's scheduleCompilationJobCompletion), so
	// this doesn't have the missed-intermediate-state shape gopherstack-7lrq
	// hid behind Eventually.
	require.Eventually(t, func() bool {
		polled, pollErr := client.DescribeCompilationJob(t.Context(), &sagemakersdk.DescribeCompilationJobInput{
			CompilationJobName: aws.String("cj-completes"),
		})
		require.NoError(t, pollErr)

		return polled.CompilationJobStatus == smtypes.CompilationJobStatusCompleted
	}, 2*time.Second, 10*time.Millisecond)

	out, err = client.DescribeCompilationJob(t.Context(), &sagemakersdk.DescribeCompilationJobInput{
		CompilationJobName: aws.String("cj-completes"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.ModelArtifacts)
	assert.Equal(t, "s3://bucket/out/model.tar.gz", aws.ToString(out.ModelArtifacts.S3ModelArtifacts))
	assert.NotNil(t, out.CompilationEndTime)
}
