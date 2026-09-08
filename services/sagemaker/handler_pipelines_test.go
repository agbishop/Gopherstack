package sagemaker_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	sagemakersdk "github.com/aws/aws-sdk-go-v2/service/sagemaker"
	smtypes "github.com/aws/aws-sdk-go-v2/service/sagemaker/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sagemaker"
)

// mockPipelineS3 is a minimal sagemaker.S3Accessor over an in-memory
// bucket/key map, mirroring services/mgn's identical test-only mockS3 (used
// there for StartImport's S3 source object).
type mockPipelineS3 struct {
	objects map[string][]byte
}

func newMockPipelineS3() *mockPipelineS3 {
	return &mockPipelineS3{objects: make(map[string][]byte)}
}

func (m *mockPipelineS3) put(bucket, key, body string) {
	m.objects[bucket+"/"+key] = []byte(body)
}

func (m *mockPipelineS3) GetObject(_ context.Context, in *s3sdk.GetObjectInput) (*s3sdk.GetObjectOutput, error) {
	data, ok := m.objects[aws.ToString(in.Bucket)+"/"+aws.ToString(in.Key)]
	if !ok {
		return nil, &s3types.NoSuchKey{}
	}

	return &s3sdk.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func TestHandler_CreatePipeline_FullFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreatePipeline", map[string]any{
		"PipelineName":        "my-pipeline",
		"PipelineDisplayName": "My Pipeline",
		"PipelineDescription": "A test pipeline",
		"RoleArn":             "arn:aws:iam::000000000000:role/SageMakerRole",
		"ParallelismConfiguration": map[string]any{
			"MaxParallelExecutionSteps": 5,
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Describe returns new fields
	rec = doSageMakerRequest(t, h, "DescribePipeline", map[string]any{
		"PipelineName": "my-pipeline",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "My Pipeline", descResp["PipelineDisplayName"])
	assert.Equal(t, "A test pipeline", descResp["PipelineDescription"])
	assert.NotNil(t, descResp["ParallelismConfiguration"])
}

func TestHandler_UpdatePipeline_FullFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreatePipeline", map[string]any{
		"PipelineName": "my-pipeline",
		"RoleArn":      "arn:aws:iam::000000000000:role/Role",
	})

	rec := doSageMakerRequest(t, h, "UpdatePipeline", map[string]any{
		"PipelineName":        "my-pipeline",
		"PipelineDisplayName": "Updated Display",
		"PipelineDescription": "Updated desc",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "DescribePipeline", map[string]any{
		"PipelineName": "my-pipeline",
	})
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "Updated Display", descResp["PipelineDisplayName"])
	assert.Equal(t, "Updated desc", descResp["PipelineDescription"])
}

// ---------------------------------------------------------------------------
// CreatePipeline/UpdatePipeline: PipelineDefinitionS3Location (gopherstack-i359)
// — CreatePipeline/UpdatePipeline fetch the real object through the backend's
// wired S3Accessor (s3pipeline.go). With no S3 backend wired (as
// newTestHandler leaves it) or the object missing, the fetch fails honestly
// with a ValidationException rather than fabricating a definition or
// silently dropping the field. Real fetch-and-use is covered by
// TestHandler_CreatePipeline_S3Location_Fetched below; the cli.go
// composition-root wiring itself is covered by
// cli_sagemaker_s3_pipeline_wiring_test.go.
// ---------------------------------------------------------------------------

func TestHandler_CreatePipeline_S3Location_UnreadableRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreatePipeline", map[string]any{
		"PipelineName": "s3-def-pipeline",
		"RoleArn":      "arn:aws:iam::000000000000:role/Role",
		"PipelineDefinitionS3Location": map[string]any{
			"Bucket":    "my-bucket",
			"ObjectKey": "defs/pipeline.json",
		},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ValidationException", body["__type"])

	rec = doSageMakerRequest(t, h, "DescribePipeline", map[string]any{"PipelineName": "s3-def-pipeline"})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "the pipeline must not have been created")
}

func TestHandler_UpdatePipeline_S3Location_UnreadableRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreatePipeline", map[string]any{
		"PipelineName": "s3-def-pipeline-2",
		"RoleArn":      "arn:aws:iam::000000000000:role/Role",
	})

	rec := doSageMakerRequest(t, h, "UpdatePipeline", map[string]any{
		"PipelineName": "s3-def-pipeline-2",
		"PipelineDefinitionS3Location": map[string]any{
			"Bucket":    "my-bucket",
			"ObjectKey": "defs/pipeline.json",
		},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ValidationException", body["__type"])
}

// TestHandler_CreatePipeline_S3Location_UnreadableRejected_RealClient confirms
// the fetch-and-fail-honestly path fires against the real SDK's wire encoding
// of PipelineDefinitionS3Location (types/types.go:17313, sagemaker@v1.263.2),
// not just this test file's own hand-built JSON.
func TestHandler_CreatePipeline_S3Location_UnreadableRejected_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("s3-def-real"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
		PipelineDefinitionS3Location: &smtypes.PipelineDefinitionS3Location{
			Bucket:    aws.String("my-bucket"),
			ObjectKey: aws.String("defs/pipeline.json"),
		},
	})
	require.Error(t, err)
}

// TestHandler_CreatePipeline_S3Location_Fetched drives CreatePipeline with a
// PipelineDefinitionS3Location against a wired mock S3 backend and confirms
// the pipeline is created with the fetched object's body as its
// PipelineDefinition, real end to end through the SDK client.
func TestHandler_CreatePipeline_S3Location_Fetched(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	s3 := newMockPipelineS3()
	s3.put("my-bucket", "defs/pipeline.json", `{"Version":"2020-12-01","Steps":[]}`)
	h.Backend.SetS3Backend(s3)

	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("s3-def-fetched"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
		PipelineDefinitionS3Location: &smtypes.PipelineDefinitionS3Location{
			Bucket:    aws.String("my-bucket"),
			ObjectKey: aws.String("defs/pipeline.json"),
		},
	})
	require.NoError(t, err)

	out, err := client.DescribePipeline(t.Context(), &sagemakersdk.DescribePipelineInput{
		PipelineName: aws.String("s3-def-fetched"),
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"Version":"2020-12-01","Steps":[]}`, aws.ToString(out.PipelineDefinition))
}

// TestHandler_UpdatePipeline_S3Location_Fetched mirrors the create case for
// UpdatePipeline: the wired S3 object's body replaces the pipeline's stored
// definition.
func TestHandler_UpdatePipeline_S3Location_Fetched(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	s3 := newMockPipelineS3()
	s3.put("my-bucket", "defs/v2.json", `{"Version":"2020-12-01","Steps":[{"Name":"Step1"}]}`)
	h.Backend.SetS3Backend(s3)

	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName:       aws.String("s3-def-update"),
		RoleArn:            aws.String("arn:aws:iam::000000000000:role/Role"),
		PipelineDefinition: aws.String(`{"Version":"2020-12-01","Steps":[]}`),
	})
	require.NoError(t, err)

	_, err = client.UpdatePipeline(t.Context(), &sagemakersdk.UpdatePipelineInput{
		PipelineName: aws.String("s3-def-update"),
		PipelineDefinitionS3Location: &smtypes.PipelineDefinitionS3Location{
			Bucket:    aws.String("my-bucket"),
			ObjectKey: aws.String("defs/v2.json"),
		},
	})
	require.NoError(t, err)

	out, err := client.DescribePipeline(t.Context(), &sagemakersdk.DescribePipelineInput{
		PipelineName: aws.String("s3-def-update"),
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"Version":"2020-12-01","Steps":[{"Name":"Step1"}]}`, aws.ToString(out.PipelineDefinition))
}

func TestHandler_StartPipelineExecution_WithParams(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreatePipeline", map[string]any{
		"PipelineName": "param-pipeline",
	})

	rec := doSageMakerRequest(t, h, "StartPipelineExecution", map[string]any{
		"PipelineName":                 "param-pipeline",
		"PipelineExecutionDisplayName": "Run 1",
		"PipelineExecutionDescription": "First run",
		"PipelineParameters": []any{
			map[string]any{"Name": "learning_rate", "Value": "0.001"},
			map[string]any{"Name": "epochs", "Value": "10"},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var startResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	execArn := startResp["PipelineExecutionArn"]
	assert.NotEmpty(t, execArn)

	// Describe returns parameters
	rec = doSageMakerRequest(t, h, "DescribePipelineExecution", map[string]any{
		"PipelineExecutionArn": execArn,
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "Run 1", descResp["PipelineExecutionDisplayName"])
	assert.Equal(t, "First run", descResp["PipelineExecutionDescription"])
	params := descResp["PipelineParameters"].([]any)
	assert.Len(t, params, 2)
}

func TestHandler_StartPipelineExecution_ParallelismAndSelectiveExecution(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreatePipeline", map[string]any{
		"PipelineName": "sel-pipeline",
		"RoleArn":      "arn:aws:iam::000000000000:role/Role",
	})

	rec := doSageMakerRequest(t, h, "StartPipelineExecution", map[string]any{
		"PipelineName": "sel-pipeline",
		"ParallelismConfiguration": map[string]any{
			"MaxParallelExecutionSteps": 3,
		},
		"SelectiveExecutionConfig": map[string]any{
			"SourcePipelineExecutionArn": "arn:aws:sagemaker:us-east-1:000000000000:pipeline/sel-pipeline/execution/prior",
			"SelectedSteps": []any{
				map[string]any{"StepName": "TrainStep"},
			},
		},
		"PipelineVersionId": 2,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var startResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	execArn := startResp["PipelineExecutionArn"]
	require.NotEmpty(t, execArn)

	rec = doSageMakerRequest(t, h, "DescribePipelineExecution", map[string]any{
		"PipelineExecutionArn": execArn,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))

	parallelism, ok := descResp["ParallelismConfiguration"].(map[string]any)
	require.True(t, ok, "ParallelismConfiguration must be present in DescribePipelineExecution")
	assert.InEpsilon(t, float64(3), parallelism["MaxParallelExecutionSteps"], 0)

	sec, ok := descResp["SelectiveExecutionConfig"].(map[string]any)
	require.True(t, ok, "SelectiveExecutionConfig must be present in DescribePipelineExecution")
	assert.Equal(t,
		"arn:aws:sagemaker:us-east-1:000000000000:pipeline/sel-pipeline/execution/prior",
		sec["SourcePipelineExecutionArn"],
	)

	assert.InEpsilon(t, float64(2), descResp["PipelineVersionId"], 0)
}

// ---------------------------------------------------------------------------
// DescribeEndpoint — ProductionVariants + FailureReason (gap #9)
// ---------------------------------------------------------------------------

func TestHandler_ListPipelineParametersForExecution(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create pipeline and start execution with parameters.
	doSageMakerRequest(t, h, "CreatePipeline", map[string]any{
		"PipelineName":       "param-pipe",
		"PipelineDefinition": `{"Version":"2020-12-01","Steps":[]}`,
		"RoleArn":            "arn:aws:iam::000000000000:role/Role",
	})

	rec := doSageMakerRequest(t, h, "StartPipelineExecution", map[string]any{
		"PipelineName":                 "param-pipe",
		"PipelineExecutionDisplayName": "run-1",
		"PipelineParameters": []map[string]string{
			{"Name": "LearningRate", "Value": "0.01"},
			{"Name": "BatchSize", "Value": "32"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var startResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	execArn := startResp["PipelineExecutionArn"]
	require.NotEmpty(t, execArn)

	// ListPipelineParametersForExecution returns the stored parameters.
	rec2 := doSageMakerRequest(t, h, "ListPipelineParametersForExecution", map[string]any{
		"PipelineExecutionArn": execArn,
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var listResp struct {
		PipelineParameters []struct {
			Name  string `json:"Name"`
			Value string `json:"Value"`
		} `json:"PipelineParameters"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &listResp))
	assert.Len(t, listResp.PipelineParameters, 2)

	names := make(map[string]string)
	for _, p := range listResp.PipelineParameters {
		names[p.Name] = p.Value
	}
	assert.Equal(t, "0.01", names["LearningRate"])
	assert.Equal(t, "32", names["BatchSize"])
}

func TestHandler_ListPipelineParametersForExecution_Empty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreatePipeline", map[string]any{
		"PipelineName":       "empty-param-pipe",
		"PipelineDefinition": `{"Version":"2020-12-01","Steps":[]}`,
		"RoleArn":            "arn:aws:iam::000000000000:role/Role",
	})

	rec := doSageMakerRequest(t, h, "StartPipelineExecution", map[string]any{
		"PipelineName": "empty-param-pipe",
	})
	var startResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))

	rec2 := doSageMakerRequest(t, h, "ListPipelineParametersForExecution", map[string]any{
		"PipelineExecutionArn": startResp["PipelineExecutionArn"],
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var listResp struct {
		PipelineParameters []any `json:"PipelineParameters"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &listResp))
	assert.NotNil(t, listResp.PipelineParameters)
	assert.Empty(t, listResp.PipelineParameters)
}

func TestHandler_ListPipelineParametersForExecution_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "ListPipelineParametersForExecution", map[string]any{
		"PipelineExecutionArn": "arn:aws:sagemaker:us-east-1:000000000000:pipeline/nonexistent/execution/abc",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_PipelineLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	synctest.Test(t, func(t *testing.T) {
		// Create pipeline.
		recCreate := doSageMakerRequest(t, h, "CreatePipeline", map[string]any{
			"PipelineName":       "my-pipeline",
			"PipelineDefinition": `{"Version":"2020-12-01","Steps":[]}`,
		})
		assert.Equal(t, http.StatusOK, recCreate.Code)

		var createOut map[string]any
		require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &createOut))
		assert.NotEmpty(t, createOut["PipelineArn"])

		// Describe pipeline.
		recDesc := doSageMakerRequest(
			t,
			h,
			"DescribePipeline",
			map[string]any{"PipelineName": "my-pipeline"},
		)
		assert.Equal(t, http.StatusOK, recDesc.Code)

		// List pipelines.
		recList := doSageMakerRequest(t, h, "ListPipelines", map[string]any{})
		assert.Equal(t, http.StatusOK, recList.Code)

		var listOut map[string]any
		require.NoError(t, json.Unmarshal(recList.Body.Bytes(), &listOut))
		assert.Len(t, listOut["PipelineSummaries"].([]any), 1)

		// Update pipeline.
		recUpdate := doSageMakerRequest(t, h, "UpdatePipeline", map[string]any{
			"PipelineName":       "my-pipeline",
			"PipelineDefinition": `{"Version":"2020-12-01","Steps":[{"Name":"step1"}]}`,
		})
		assert.Equal(t, http.StatusOK, recUpdate.Code)

		// Start pipeline execution.
		recExec := doSageMakerRequest(t, h, "StartPipelineExecution", map[string]any{
			"PipelineName": "my-pipeline",
		})
		assert.Equal(t, http.StatusOK, recExec.Code)

		var execOut map[string]any
		require.NoError(t, json.Unmarshal(recExec.Body.Bytes(), &execOut))
		execArn := execOut["PipelineExecutionArn"].(string)
		assert.NotEmpty(t, execArn)

		// Describe pipeline execution.
		recDescExec := doSageMakerRequest(t, h, "DescribePipelineExecution", map[string]any{
			"PipelineExecutionArn": execArn,
		})
		assert.Equal(t, http.StatusOK, recDescExec.Code)

		// List pipeline executions.
		recListExec := doSageMakerRequest(t, h, "ListPipelineExecutions", map[string]any{
			"PipelineName": "my-pipeline",
		})
		assert.Equal(t, http.StatusOK, recListExec.Code)

		// The execution is still Executing here (StartPipelineExecution's
		// Executing -> Succeeded transition is delayed, gopherstack-z5hj), so
		// DeletePipeline must refuse per its own doc comment ("no running
		// instances") -- see TestHandler_DeletePipeline_RunningExecution for
		// the dedicated regression test.
		recDeleteWhileRunning := doSageMakerRequest(
			t,
			h,
			"DeletePipeline",
			map[string]any{"PipelineName": "my-pipeline"},
		)
		assert.Equal(t, http.StatusBadRequest, recDeleteWhileRunning.Code)

		// Wait for the execution to reach Succeeded, then delete should succeed.
		time.Sleep(time.Second)
		synctest.Wait()

		recDelete := doSageMakerRequest(
			t,
			h,
			"DeletePipeline",
			map[string]any{"PipelineName": "my-pipeline"},
		)
		assert.Equal(t, http.StatusOK, recDelete.Code)
	})
}

// TestHandler_DeletePipeline_RunningExecution verifies DeletePipeline
// refuses while an execution is Executing, per its own doc comment
// (api_op_DeletePipeline.go:12-14, sagemaker@v1.263.2): "Deletes a pipeline
// if there are no running instances of the pipeline." (gopherstack-yp2t).
func TestHandler_DeletePipeline_RunningExecution(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	synctest.Test(t, func(t *testing.T) {
		recCreate := doSageMakerRequest(t, h, "CreatePipeline", map[string]any{
			"PipelineName":       "running-pipeline",
			"PipelineDefinition": `{"Version":"2020-12-01","Steps":[]}`,
		})
		require.Equal(t, http.StatusOK, recCreate.Code)

		recExec := doSageMakerRequest(t, h, "StartPipelineExecution", map[string]any{
			"PipelineName": "running-pipeline",
		})
		require.Equal(t, http.StatusOK, recExec.Code)

		recDelete := doSageMakerRequest(t, h, "DeletePipeline", map[string]any{
			"PipelineName": "running-pipeline",
		})
		require.Equal(t, http.StatusBadRequest, recDelete.Code)

		var errResp map[string]any
		require.NoError(t, json.Unmarshal(recDelete.Body.Bytes(), &errResp))
		assert.Equal(t, "ConflictException", errResp["__type"])

		// Once the execution reaches Succeeded, the delete is allowed.
		time.Sleep(time.Second)
		synctest.Wait()

		recDeleteAfter := doSageMakerRequest(t, h, "DeletePipeline", map[string]any{
			"PipelineName": "running-pipeline",
		})
		assert.Equal(t, http.StatusOK, recDeleteAfter.Code)
	})
}

func TestHandler_Pipeline_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, op := range []string{"DescribePipeline", "UpdatePipeline", "DeletePipeline"} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()

			rec := doSageMakerRequest(t, h, op, map[string]any{"PipelineName": "nonexistent"})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_Pipeline_Duplicate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{"PipelineName": "dup-pipeline"}
	rec := doSageMakerRequest(t, h, "CreatePipeline", body)
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSageMakerRequest(t, h, "CreatePipeline", body)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

// TestHandler_Pipeline_Duplicate_RealClient asserts the wire error type is
// ConflictException -- CreatePipeline's documented error (botocore
// sagemaker/2017-07-24@1.43.56 service-2.json), not the generic
// ResourceInUse gopherstack-kbxx found this mapped to.
func TestHandler_Pipeline_Duplicate_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	in := &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("dup-pipeline-real"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/SageMakerRole"),
	}

	_, err := client.CreatePipeline(t.Context(), in)
	require.NoError(t, err)

	_, err = client.CreatePipeline(t.Context(), in)
	require.Error(t, err)

	var conflict *smtypes.ConflictException
	require.ErrorAs(t, err, &conflict)
}

// ---------------------------------------------------------------------------
// Pipeline execution step operations
// ---------------------------------------------------------------------------

func TestHandler_PipelineExecutionSteps(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create and start pipeline.
	doSageMakerRequest(t, h, "CreatePipeline", map[string]any{"PipelineName": "step-pipeline"})
	recExec := doSageMakerRequest(t, h, "StartPipelineExecution", map[string]any{
		"PipelineName": "step-pipeline",
	})
	require.Equal(t, http.StatusOK, recExec.Code)

	var execOut map[string]any
	require.NoError(t, json.Unmarshal(recExec.Body.Bytes(), &execOut))
	execArn := execOut["PipelineExecutionArn"].(string)

	// ListPipelineExecutionSteps.
	recList := doSageMakerRequest(t, h, "ListPipelineExecutionSteps", map[string]any{
		"PipelineExecutionArn": execArn,
	})
	assert.Equal(t, http.StatusOK, recList.Code)

	// SendPipelineExecutionStepSuccess.
	recSuccess := doSageMakerRequest(t, h, "SendPipelineExecutionStepSuccess", map[string]any{
		"CallbackToken": execArn,
	})
	assert.Equal(t, http.StatusOK, recSuccess.Code)

	// SendPipelineExecutionStepFailure.
	recFail := doSageMakerRequest(t, h, "SendPipelineExecutionStepFailure", map[string]any{
		"CallbackToken": execArn,
		"FailureReason": "test failure",
	})
	assert.Equal(t, http.StatusOK, recFail.Code)

	// RetryPipelineExecution.
	recRetry := doSageMakerRequest(t, h, "RetryPipelineExecution", map[string]any{
		"PipelineExecutionArn": execArn,
	})
	assert.Equal(t, http.StatusOK, recRetry.Code)

	// StopPipelineExecution.
	recStop := doSageMakerRequest(t, h, "StopPipelineExecution", map[string]any{
		"PipelineExecutionArn": execArn,
	})
	assert.Equal(t, http.StatusOK, recStop.Code)
}

func TestBackend_PipelineOps_Direct(t *testing.T) {
	t.Parallel()

	b := sagemaker.NewInMemoryBackend("000000000000", "us-east-1")

	// Create and start a pipeline.
	_, err := b.CreatePipeline(context.Background(), "direct-pipeline", `{"Version":"2020-12-01"}`, "", nil)
	require.NoError(t, err)

	exec, err := b.StartPipelineExecution(context.Background(), "direct-pipeline")
	require.NoError(t, err)
	execArn := exec.PipelineExecutionArn

	// ListPipelineExecutionSteps.
	steps, _ := b.ListPipelineExecutionSteps(
		context.Background(),
		sagemaker.ListPipelineExecutionStepsParams{ExecutionArn: execArn},
	)
	assert.NotNil(t, steps)

	// SendPipelineExecutionStepSuccess.
	err = b.SendPipelineExecutionStepSuccess(context.Background(), execArn, nil)
	require.NoError(t, err)

	// SendPipelineExecutionStepFailure.
	err = b.SendPipelineExecutionStepFailure(context.Background(), execArn, "out of memory")
	require.NoError(t, err)

	// RetryPipelineExecution.
	retried, err := b.RetryPipelineExecution(context.Background(), execArn, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, retried.PipelineExecutionArn)

	// StopPipelineExecution.
	stopped, err := b.StopPipelineExecution(context.Background(), execArn)
	require.NoError(t, err)
	assert.NotEmpty(t, stopped.PipelineExecutionArn)
}

func TestBackend_PipelineOps_NotFound(t *testing.T) {
	t.Parallel()

	b := sagemaker.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.RetryPipelineExecution(context.Background(), "nonexistent-exec-arn", nil)
	require.Error(t, err)

	_, err = b.StopPipelineExecution(context.Background(), "nonexistent-exec-arn")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// ListPipelines — CreatedAfter/CreatedBefore/PipelineNamePrefix/MaxResults/
// SortBy/SortOrder (previously absent: only NextToken existed) — and
// PipelineSummary response completeness (PipelineDescription/
// PipelineDisplayName/RoleArn/LastExecutionTime, previously dropped).
// ---------------------------------------------------------------------------

// TestHandler_ListPipelines_CreationWindowAndSort_RealClient proves
// CreatedAfter/CreatedBefore/SortBy/SortOrder narrow and order the real
// result set through the real SDK client.
func TestHandler_ListPipelines_CreationWindowAndSort_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("older-pipeline"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
	})
	require.NoError(t, err)

	_, err = client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("newer-pipeline"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
	})
	require.NoError(t, err)

	future := time.Now().Add(365 * 24 * time.Hour)
	past := time.Now().Add(-365 * 24 * time.Hour)

	out, err := client.ListPipelines(t.Context(), &sagemakersdk.ListPipelinesInput{
		CreatedAfter:  &past,
		CreatedBefore: &future,
		SortBy:        smtypes.SortPipelinesByCreationTime,
		SortOrder:     smtypes.SortOrderAscending,
	})
	require.NoError(t, err)
	require.Len(t, out.PipelineSummaries, 2)
	assert.Equal(t, "older-pipeline", aws.ToString(out.PipelineSummaries[0].PipelineName))
	assert.Equal(t, "newer-pipeline", aws.ToString(out.PipelineSummaries[1].PipelineName))

	excluded, err := client.ListPipelines(t.Context(), &sagemakersdk.ListPipelinesInput{CreatedAfter: &future})
	require.NoError(t, err)
	assert.Empty(t, excluded.PipelineSummaries)
}

// TestHandler_ListPipelines_NamePrefixAndMaxResults_RealClient proves
// PipelineNamePrefix and MaxResults/NextToken — both previously absent —
// filter and page the real result set.
func TestHandler_ListPipelines_NamePrefixAndMaxResults_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	for _, name := range []string{"team-a-pipeline-1", "team-a-pipeline-2", "team-b-pipeline"} {
		_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
			PipelineName: aws.String(name),
			RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
		})
		require.NoError(t, err)
	}

	prefixed, err := client.ListPipelines(t.Context(), &sagemakersdk.ListPipelinesInput{
		PipelineNamePrefix: aws.String("team-a"),
	})
	require.NoError(t, err)
	assert.Len(t, prefixed.PipelineSummaries, 2)

	page1, err := client.ListPipelines(t.Context(), &sagemakersdk.ListPipelinesInput{
		PipelineNamePrefix: aws.String("team-a"), MaxResults: aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, page1.PipelineSummaries, 1)
	require.NotEmpty(t, aws.ToString(page1.NextToken))
}

// TestHandler_ListPipelines_SummaryFields_RealClient proves PipelineSummary's
// PipelineDescription/PipelineDisplayName/RoleArn/LastExecutionTime —
// previously dropped even though CreatePipeline/StartPipelineExecution
// already stored them — round-trip through the real SDK client.
func TestHandler_ListPipelines_SummaryFields_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName:        aws.String("summary-pipeline"),
		PipelineDescription: aws.String("a description"),
		PipelineDisplayName: aws.String("A Display Name"),
		RoleArn:             aws.String("arn:aws:iam::000000000000:role/SummaryRole"),
	})
	require.NoError(t, err)

	before, err := client.ListPipelines(t.Context(), &sagemakersdk.ListPipelinesInput{
		PipelineNamePrefix: aws.String("summary-pipeline"),
	})
	require.NoError(t, err)
	require.Len(t, before.PipelineSummaries, 1)
	sum := before.PipelineSummaries[0]
	assert.Equal(t, "a description", aws.ToString(sum.PipelineDescription))
	assert.Equal(t, "A Display Name", aws.ToString(sum.PipelineDisplayName))
	assert.Equal(t, "arn:aws:iam::000000000000:role/SummaryRole", aws.ToString(sum.RoleArn))
	assert.Nil(t, sum.LastExecutionTime, "never run: LastExecutionTime must be absent")

	_, err = client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
		PipelineName: aws.String("summary-pipeline"),
	})
	require.NoError(t, err)

	after, err := client.ListPipelines(t.Context(), &sagemakersdk.ListPipelinesInput{
		PipelineNamePrefix: aws.String("summary-pipeline"),
	})
	require.NoError(t, err)
	require.Len(t, after.PipelineSummaries, 1)
	require.NotNil(t, after.PipelineSummaries[0].LastExecutionTime)
}

// ---------------------------------------------------------------------------
// ListPipelineExecutions — CreatedAfter/CreatedBefore/MaxResults/SortBy/
// SortOrder (previously absent) and PipelineExecutionSummary response
// completeness (DisplayName/Description, previously dropped).
// ---------------------------------------------------------------------------

// TestHandler_ListPipelineExecutions_CreationWindowAndSort_RealClient proves
// CreatedAfter/CreatedBefore/SortBy/SortOrder narrow and order the real
// result set.
func TestHandler_ListPipelineExecutions_CreationWindowAndSort_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("exec-window-pipeline"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
	})
	require.NoError(t, err)

	arns := make([]string, 0, 2)

	for range 2 {
		started, startErr := client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
			PipelineName: aws.String("exec-window-pipeline"),
		})
		require.NoError(t, startErr)
		arns = append(arns, aws.ToString(started.PipelineExecutionArn))
	}

	future := time.Now().Add(365 * 24 * time.Hour)
	past := time.Now().Add(-365 * 24 * time.Hour)

	out, err := client.ListPipelineExecutions(t.Context(), &sagemakersdk.ListPipelineExecutionsInput{
		PipelineName:  aws.String("exec-window-pipeline"),
		CreatedAfter:  &past,
		CreatedBefore: &future,
		SortBy:        smtypes.SortPipelineExecutionsByPipelineExecutionArn,
		SortOrder:     smtypes.SortOrderAscending,
	})
	require.NoError(t, err)
	require.Len(t, out.PipelineExecutionSummaries, 2)

	got := []string{
		aws.ToString(out.PipelineExecutionSummaries[0].PipelineExecutionArn),
		aws.ToString(out.PipelineExecutionSummaries[1].PipelineExecutionArn),
	}
	assert.ElementsMatch(t, arns, got)
	assert.Less(t, got[0], got[1], "SortBy=PipelineExecutionArn, SortOrder=Ascending must order by ARN")

	excluded, err := client.ListPipelineExecutions(t.Context(), &sagemakersdk.ListPipelineExecutionsInput{
		PipelineName: aws.String("exec-window-pipeline"),
		CreatedAfter: &future,
	})
	require.NoError(t, err)
	assert.Empty(t, excluded.PipelineExecutionSummaries)
}

// TestHandler_ListPipelineExecutions_MaxResults_RealClient proves MaxResults/
// NextToken — previously absent — cap and page the real result set.
func TestHandler_ListPipelineExecutions_MaxResults_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("exec-page-pipeline"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
	})
	require.NoError(t, err)

	for range 2 {
		_, startErr := client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
			PipelineName: aws.String("exec-page-pipeline"),
		})
		require.NoError(t, startErr)
	}

	page1, err := client.ListPipelineExecutions(t.Context(), &sagemakersdk.ListPipelineExecutionsInput{
		PipelineName: aws.String("exec-page-pipeline"), MaxResults: aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, page1.PipelineExecutionSummaries, 1)
	require.NotEmpty(t, aws.ToString(page1.NextToken))
}

// TestHandler_ListPipelineExecutions_SummaryFields_RealClient proves
// PipelineExecutionSummary's PipelineExecutionDisplayName/
// PipelineExecutionDescription — previously dropped even though
// StartPipelineExecution already stored them — round-trip.
func TestHandler_ListPipelineExecutions_SummaryFields_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("exec-summary-pipeline"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
	})
	require.NoError(t, err)

	_, err = client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
		PipelineName:                 aws.String("exec-summary-pipeline"),
		PipelineExecutionDisplayName: aws.String("Nightly Run"),
		PipelineExecutionDescription: aws.String("nightly batch"),
	})
	require.NoError(t, err)

	out, err := client.ListPipelineExecutions(t.Context(), &sagemakersdk.ListPipelineExecutionsInput{
		PipelineName: aws.String("exec-summary-pipeline"),
	})
	require.NoError(t, err)
	require.Len(t, out.PipelineExecutionSummaries, 1)
	assert.Equal(t, "Nightly Run", aws.ToString(out.PipelineExecutionSummaries[0].PipelineExecutionDisplayName))
	assert.Equal(t, "nightly batch", aws.ToString(out.PipelineExecutionSummaries[0].PipelineExecutionDescription))
}

// ---------------------------------------------------------------------------
// RetryPipelineExecution — ParallelismConfiguration (previously absent: the
// field parsed nowhere, so a retried execution never carried any parallelism
// configuration at all, not even the parent pipeline's).
// ---------------------------------------------------------------------------

func TestHandler_RetryPipelineExecution_ParallelismConfiguration_RealClient(t *testing.T) {
	t.Parallel()

	t.Run("default inherits the parent pipeline's configuration", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		client := newTestSageMakerClient(t, h)

		_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
			PipelineName:             aws.String("retry-default-pipeline"),
			RoleArn:                  aws.String("arn:aws:iam::000000000000:role/Role"),
			ParallelismConfiguration: &smtypes.ParallelismConfiguration{MaxParallelExecutionSteps: aws.Int32(5)},
		})
		require.NoError(t, err)

		started, err := client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
			PipelineName: aws.String("retry-default-pipeline"),
		})
		require.NoError(t, err)

		retried, err := client.RetryPipelineExecution(t.Context(), &sagemakersdk.RetryPipelineExecutionInput{
			PipelineExecutionArn: started.PipelineExecutionArn,
		})
		require.NoError(t, err)

		desc, err := client.DescribePipelineExecution(t.Context(), &sagemakersdk.DescribePipelineExecutionInput{
			PipelineExecutionArn: retried.PipelineExecutionArn,
		})
		require.NoError(t, err)
		require.NotNil(t, desc.ParallelismConfiguration)
		assert.EqualValues(t, 5, aws.ToInt32(desc.ParallelismConfiguration.MaxParallelExecutionSteps))
	})

	t.Run("explicit override replaces the parent pipeline's configuration", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		client := newTestSageMakerClient(t, h)

		_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
			PipelineName:             aws.String("retry-override-pipeline"),
			RoleArn:                  aws.String("arn:aws:iam::000000000000:role/Role"),
			ParallelismConfiguration: &smtypes.ParallelismConfiguration{MaxParallelExecutionSteps: aws.Int32(5)},
		})
		require.NoError(t, err)

		started, err := client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
			PipelineName: aws.String("retry-override-pipeline"),
		})
		require.NoError(t, err)

		retried, err := client.RetryPipelineExecution(t.Context(), &sagemakersdk.RetryPipelineExecutionInput{
			PipelineExecutionArn:     started.PipelineExecutionArn,
			ParallelismConfiguration: &smtypes.ParallelismConfiguration{MaxParallelExecutionSteps: aws.Int32(2)},
		})
		require.NoError(t, err)

		desc, err := client.DescribePipelineExecution(t.Context(), &sagemakersdk.DescribePipelineExecutionInput{
			PipelineExecutionArn: retried.PipelineExecutionArn,
		})
		require.NoError(t, err)
		require.NotNil(t, desc.ParallelismConfiguration)
		assert.EqualValues(t, 2, aws.ToInt32(desc.ParallelismConfiguration.MaxParallelExecutionSteps))
	})
}

// ---------------------------------------------------------------------------
// SendPipelineExecutionStepSuccess/Failure — real wire shape correction.
//
// The previous version of these handlers read PipelineExecutionArn and
// StepName from the request body. Neither field exists on the real
// SendPipelineExecutionStepSuccessInput/SendPipelineExecutionStepFailureInput
// (api_op_SendPipelineExecutionStepSuccess.go:29-43, api_op_
// SendPipelineExecutionStepFailure.go:29-42, sagemaker@v1.263.2) — AWS
// resolves the step from CallbackToken alone. No real client (including the
// real aws-sdk-go-v2 client used below) can ever populate those two fields,
// so this proves the fix through the actual wire shape rather than this
// repo's own hand-built JSON.
//
// OutputParameters — entirely absent before this pass — now round-trips
// through ListPipelineExecutionSteps' Metadata.Callback.
// ---------------------------------------------------------------------------

func TestHandler_SendPipelineExecutionStepSuccess_OutputParameters_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("callback-pipeline"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
	})
	require.NoError(t, err)

	started, err := client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
		PipelineName: aws.String("callback-pipeline"),
	})
	require.NoError(t, err)

	out, err := client.SendPipelineExecutionStepSuccess(
		t.Context(),
		&sagemakersdk.SendPipelineExecutionStepSuccessInput{
			CallbackToken: started.PipelineExecutionArn,
			OutputParameters: []smtypes.OutputParameter{
				{Name: aws.String("accuracy"), Value: aws.String("0.97")},
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, aws.ToString(started.PipelineExecutionArn), aws.ToString(out.PipelineExecutionArn))

	steps, err := client.ListPipelineExecutionSteps(t.Context(), &sagemakersdk.ListPipelineExecutionStepsInput{
		PipelineExecutionArn: started.PipelineExecutionArn,
	})
	require.NoError(t, err)
	require.Len(t, steps.PipelineExecutionSteps, 1)

	step := steps.PipelineExecutionSteps[0]
	assert.Equal(t, smtypes.StepStatusSucceeded, step.StepStatus)
	require.NotNil(t, step.Metadata)
	require.NotNil(t, step.Metadata.Callback)
	assert.Equal(t, aws.ToString(started.PipelineExecutionArn), aws.ToString(step.Metadata.Callback.CallbackToken))
	require.Len(t, step.Metadata.Callback.OutputParameters, 1)
	assert.Equal(t, "accuracy", aws.ToString(step.Metadata.Callback.OutputParameters[0].Name))
	assert.Equal(t, "0.97", aws.ToString(step.Metadata.Callback.OutputParameters[0].Value))
}

func TestHandler_SendPipelineExecutionStepFailure_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("callback-failure-pipeline"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
	})
	require.NoError(t, err)

	started, err := client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
		PipelineName: aws.String("callback-failure-pipeline"),
	})
	require.NoError(t, err)

	_, err = client.SendPipelineExecutionStepFailure(t.Context(), &sagemakersdk.SendPipelineExecutionStepFailureInput{
		CallbackToken: started.PipelineExecutionArn,
		FailureReason: aws.String("timed out"),
	})
	require.NoError(t, err)

	steps, err := client.ListPipelineExecutionSteps(t.Context(), &sagemakersdk.ListPipelineExecutionStepsInput{
		PipelineExecutionArn: started.PipelineExecutionArn,
	})
	require.NoError(t, err)
	require.Len(t, steps.PipelineExecutionSteps, 1)
	assert.Equal(t, smtypes.StepStatusFailed, steps.PipelineExecutionSteps[0].StepStatus)
	assert.Equal(t, "timed out", aws.ToString(steps.PipelineExecutionSteps[0].FailureReason))
}

// TestHandler_SendPipelineExecutionStepSuccess_RequiresCallbackToken proves
// CallbackToken is enforced as required, matching the real op (api_op_
// SendPipelineExecutionStepSuccess.go:33-36: "This member is required").
func TestHandler_SendPipelineExecutionStepSuccess_RequiresCallbackToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "SendPipelineExecutionStepSuccess", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// ListPipelineExecutionSteps — MaxResults/SortOrder (previously absent).
//
// This backend can record at most one callback step per execution (see
// pipelineCallbackStepName's doc comment in pipeline_executions.go): Success/
// Failure both write under the same fixed step name, so the second call
// overwrites rather than adds a step. That means MaxResults' truncation
// branch is real, working code that cannot currently be exercised through
// any public request shape on this backend — disclosed here rather than
// fabricating a multi-step scenario no real pipeline execution on this
// backend can produce, matching the standard set by parity-9's
// CreateHubContentPresignedUrls disclosure.
// ---------------------------------------------------------------------------

func TestHandler_ListPipelineExecutionSteps_MaxResults_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("steps-page-pipeline"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
	})
	require.NoError(t, err)

	started, err := client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
		PipelineName: aws.String("steps-page-pipeline"),
	})
	require.NoError(t, err)

	_, err = client.SendPipelineExecutionStepSuccess(t.Context(), &sagemakersdk.SendPipelineExecutionStepSuccessInput{
		CallbackToken: started.PipelineExecutionArn,
	})
	require.NoError(t, err)

	out, err := client.ListPipelineExecutionSteps(t.Context(), &sagemakersdk.ListPipelineExecutionStepsInput{
		PipelineExecutionArn: started.PipelineExecutionArn,
		MaxResults:           aws.Int32(1),
	})
	require.NoError(t, err)
	assert.Len(t, out.PipelineExecutionSteps, 1)
	assert.Empty(t, aws.ToString(out.NextToken), "only one step can ever exist for this execution")
}

// ---------------------------------------------------------------------------
// ListPipelineParametersForExecution — MaxResults (previously absent).
// ---------------------------------------------------------------------------

func TestHandler_ListPipelineParametersForExecution_MaxResults_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("params-page-pipeline"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
	})
	require.NoError(t, err)

	started, err := client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
		PipelineName: aws.String("params-page-pipeline"),
		PipelineParameters: []smtypes.Parameter{
			{Name: aws.String("a"), Value: aws.String("1")},
			{Name: aws.String("b"), Value: aws.String("2")},
		},
	})
	require.NoError(t, err)

	page1, err := client.ListPipelineParametersForExecution(
		t.Context(),
		&sagemakersdk.ListPipelineParametersForExecutionInput{
			PipelineExecutionArn: started.PipelineExecutionArn,
			MaxResults:           aws.Int32(1),
		},
	)
	require.NoError(t, err)
	require.Len(t, page1.PipelineParameters, 1)
	require.NotEmpty(t, aws.ToString(page1.NextToken))

	page2, err := client.ListPipelineParametersForExecution(
		t.Context(),
		&sagemakersdk.ListPipelineParametersForExecutionInput{
			PipelineExecutionArn: started.PipelineExecutionArn,
			MaxResults:           aws.Int32(1),
			NextToken:            page1.NextToken,
		},
	)
	require.NoError(t, err)
	require.Len(t, page2.PipelineParameters, 1)
	assert.NotEqual(t,
		aws.ToString(page1.PipelineParameters[0].Name),
		aws.ToString(page2.PipelineParameters[0].Name),
	)
}

// ---------------------------------------------------------------------------
// StartPipelineExecution — MlflowExperimentName (previously absent).
// ---------------------------------------------------------------------------

func TestHandler_StartPipelineExecution_MlflowExperimentName_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreatePipeline(t.Context(), &sagemakersdk.CreatePipelineInput{
		PipelineName: aws.String("mlflow-pipeline"),
		RoleArn:      aws.String("arn:aws:iam::000000000000:role/Role"),
	})
	require.NoError(t, err)

	started, err := client.StartPipelineExecution(t.Context(), &sagemakersdk.StartPipelineExecutionInput{
		PipelineName:         aws.String("mlflow-pipeline"),
		MlflowExperimentName: aws.String("exp-42"),
	})
	require.NoError(t, err)

	desc, err := client.DescribePipelineExecution(t.Context(), &sagemakersdk.DescribePipelineExecutionInput{
		PipelineExecutionArn: started.PipelineExecutionArn,
	})
	require.NoError(t, err)
	require.NotNil(t, desc.MLflowConfig)
	assert.Equal(t, "exp-42", aws.ToString(desc.MLflowConfig.MlflowExperimentName))
	assert.Nil(t, desc.MLflowConfig.MlflowResourceArn,
		"MlflowResourceArn is not modeled: no tracking-server association exists on this backend")
}
