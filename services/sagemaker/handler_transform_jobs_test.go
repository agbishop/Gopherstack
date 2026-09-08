package sagemaker_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sagemaker"
)

func TestHandler_TransformJobLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateModel", map[string]any{"ModelName": "my-model"})

	// Create
	rec := doSageMakerRequest(t, h, "CreateTransformJob", map[string]any{
		"TransformJobName": "my-transform",
		"ModelName":        "my-model",
		"TransformInput": map[string]any{
			"DataSource": map[string]any{
				"S3DataSource": map[string]any{
					"S3Uri":      "s3://bucket/input",
					"S3DataType": "S3Prefix",
				},
			},
			"ContentType": "text/csv",
		},
		"TransformOutput": map[string]any{
			"S3OutputPath": "s3://bucket/output",
		},
		"TransformResources": map[string]any{
			"InstanceType":  "ml.m5.large",
			"InstanceCount": 1,
		},
		"BatchStrategy": "MultiRecord",
		"Environment":   map[string]string{"KEY": "VALUE"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	assert.NotEmpty(t, createResp["TransformJobArn"])

	// Describe — InProgress initially
	rec = doSageMakerRequest(t, h, "DescribeTransformJob", map[string]any{
		"TransformJobName": "my-transform",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "my-transform", descResp["TransformJobName"])
	assert.Equal(t, "my-model", descResp["ModelName"])
	assert.Equal(t, "InProgress", descResp["TransformJobStatus"])
	assert.Equal(t, "MultiRecord", descResp["BatchStrategy"])

	// List
	rec = doSageMakerRequest(t, h, "ListTransformJobs", map[string]any{})
	assert.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	summaries := listResp["TransformJobSummaries"].([]any)
	assert.Len(t, summaries, 1)

	// Wait for completion
	time.Sleep(400 * time.Millisecond)
	rec = doSageMakerRequest(t, h, "DescribeTransformJob", map[string]any{
		"TransformJobName": "my-transform",
	})
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "Completed", descResp["TransformJobStatus"])
}

func TestHandler_TransformJob_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "DescribeTransformJob", map[string]any{
		"TransformJobName": "nonexistent",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_TransformJob_Duplicate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateModel", map[string]any{"ModelName": "my-model"})

	createBody := map[string]any{
		"TransformJobName": "dup-transform",
		"ModelName":        "my-model",
		"TransformInput": map[string]any{
			"DataSource": map[string]any{
				"S3DataSource": map[string]any{"S3Uri": "s3://bucket/input"},
			},
		},
		"TransformOutput":    map[string]any{"S3OutputPath": "s3://bucket/output"},
		"TransformResources": map[string]any{"InstanceType": "ml.m5.large", "InstanceCount": 1},
	}

	rec := doSageMakerRequest(t, h, "CreateTransformJob", createBody)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "CreateTransformJob", createBody)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_CreateTransformJob_ModelNotFound proves CreateTransformJob
// rejects a ModelName that does not name an existing model with the
// ResourceNotFound wire error api_op_CreateTransformJob.go's deserializer
// models for this op (gopherstack-tauw). Real AWS: "ModelName must be the
// name of an existing Amazon SageMaker model" (api_op_CreateTransformJob.go).
func TestHandler_CreateTransformJob_ModelNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreateTransformJob", map[string]any{
		"TransformJobName": "orphan-transform",
		"ModelName":        "does-not-exist",
		"TransformInput": map[string]any{
			"DataSource": map[string]any{
				"S3DataSource": map[string]any{"S3Uri": "s3://bucket/input"},
			},
		},
		"TransformOutput":    map[string]any{"S3OutputPath": "s3://bucket/output"},
		"TransformResources": map[string]any{"InstanceType": "ml.m5.large", "InstanceCount": 1},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ResourceNotFound", errResp["__type"])

	rec = doSageMakerRequest(t, h, "DescribeTransformJob", map[string]any{
		"TransformJobName": "orphan-transform",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "job must not have been created")
}

func TestHandler_StopTransformJob(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateModel", map[string]any{"ModelName": "my-model"})

	doSageMakerRequest(t, h, "CreateTransformJob", map[string]any{
		"TransformJobName": "stop-me",
		"ModelName":        "my-model",
		"TransformInput": map[string]any{
			"DataSource": map[string]any{
				"S3DataSource": map[string]any{"S3Uri": "s3://bucket/input"},
			},
		},
		"TransformOutput":    map[string]any{"S3OutputPath": "s3://bucket/output"},
		"TransformResources": map[string]any{"InstanceType": "ml.m5.large", "InstanceCount": 1},
	})

	rec := doSageMakerRequest(t, h, "StopTransformJob", map[string]any{
		"TransformJobName": "stop-me",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Should now be Stopping
	rec = doSageMakerRequest(t, h, "DescribeTransformJob", map[string]any{
		"TransformJobName": "stop-me",
	})
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "Stopping", descResp["TransformJobStatus"])
}

func TestHandler_ListTransformJobs_StatusFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateModel", map[string]any{"ModelName": "model"})

	for i, name := range []string{"tj-1", "tj-2"} {
		doSageMakerRequest(t, h, "CreateTransformJob", map[string]any{
			"TransformJobName": name,
			"ModelName":        "model",
			"TransformInput": map[string]any{
				"DataSource": map[string]any{
					"S3DataSource": map[string]any{"S3Uri": "s3://b/in"},
				},
			},
			"TransformOutput":    map[string]any{"S3OutputPath": "s3://b/out"},
			"TransformResources": map[string]any{"InstanceType": "ml.m5.large", "InstanceCount": 1},
		})
		if i == 0 {
			doSageMakerRequest(t, h, "StopTransformJob", map[string]any{"TransformJobName": name})
		}
	}

	rec := doSageMakerRequest(t, h, "ListTransformJobs", map[string]any{
		"StatusEquals": "InProgress",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	summaries := listResp["TransformJobSummaries"].([]any)
	assert.Len(t, summaries, 1)
	assert.Equal(t, "tj-2", summaries[0].(map[string]any)["TransformJobName"])
}

func TestHandler_ListTransformJobs_SortByName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateModel", map[string]any{"ModelName": "model"})

	for _, name := range []string{"tj-alpha", "tj-beta"} {
		doSageMakerRequest(t, h, "CreateTransformJob", map[string]any{
			"TransformJobName": name,
			"ModelName":        "model",
			"TransformInput": map[string]any{
				"DataSource": map[string]any{
					"S3DataSource": map[string]any{"S3Uri": "s3://b/in"},
				},
			},
			"TransformOutput":    map[string]any{"S3OutputPath": "s3://b/out"},
			"TransformResources": map[string]any{"InstanceType": "ml.m5.large", "InstanceCount": 1},
		})
	}

	rec := doSageMakerRequest(t, h, "ListTransformJobs", map[string]any{
		"SortBy":    "Name",
		"SortOrder": "Ascending",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	summaries := resp["TransformJobSummaries"].([]any)
	require.Len(t, summaries, 2)
	assert.Equal(t, "tj-alpha", summaries[0].(map[string]any)["TransformJobName"])
	assert.Equal(t, "tj-beta", summaries[1].(map[string]any)["TransformJobName"])
}

func TestHandler_CreateTransformJob_RequiredFields(t *testing.T) {
	t.Parallel()

	base := func() map[string]any {
		return map[string]any{
			"TransformJobName": "req-fields",
			"ModelName":        "model",
			"TransformInput": map[string]any{
				"DataSource": map[string]any{
					"S3DataSource": map[string]any{"S3Uri": "s3://b/in"},
				},
			},
			"TransformOutput":    map[string]any{"S3OutputPath": "s3://b/out"},
			"TransformResources": map[string]any{"InstanceType": "ml.m5.large", "InstanceCount": 1},
		}
	}

	tests := []struct {
		mutate func(map[string]any)
		name   string
	}{
		{name: "missing model name", mutate: func(b map[string]any) { delete(b, "ModelName") }},
		{name: "missing s3 uri", mutate: func(b map[string]any) {
			b["TransformInput"] = map[string]any{"DataSource": map[string]any{"S3DataSource": map[string]any{}}}
		}},
		{name: "missing output path", mutate: func(b map[string]any) { b["TransformOutput"] = map[string]any{} }},
		{name: "missing instance type", mutate: func(b map[string]any) {
			b["TransformResources"] = map[string]any{"InstanceCount": 1}
		}},
		{name: "missing instance count", mutate: func(b map[string]any) {
			b["TransformResources"] = map[string]any{"InstanceType": "ml.m5.large"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doSageMakerRequest(t, h, "CreateModel", map[string]any{"ModelName": "model"})
			body := base()
			tt.mutate(body)

			rec := doSageMakerRequest(t, h, "CreateTransformJob", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestHandler_CreateTransformJob_RoleArnNotPartOfWireShape proves RoleArn is
// not a field CreateTransformJobInput declares at all
// (api_op_CreateTransformJob.go:55-166) -- a fabricated field this handler
// previously accepted, stored, and echoed back on Describe even though no
// real client ever sends it.
func TestHandler_CreateTransformJob_RoleArnNotPartOfWireShape(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateModel", map[string]any{"ModelName": "model"})

	rec := doSageMakerRequest(t, h, "CreateTransformJob", map[string]any{
		"TransformJobName": "no-role-arn",
		"ModelName":        "model",
		"RoleArn":          "arn:aws:iam::000000000000:role/ignored",
		"TransformInput": map[string]any{
			"DataSource": map[string]any{
				"S3DataSource": map[string]any{"S3Uri": "s3://b/in"},
			},
		},
		"TransformOutput":    map[string]any{"S3OutputPath": "s3://b/out"},
		"TransformResources": map[string]any{"InstanceType": "ml.m5.large", "InstanceCount": 1},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "DescribeTransformJob", map[string]any{
		"TransformJobName": "no-role-arn",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	_, present := descResp["RoleArn"]
	assert.False(t, present, "RoleArn is not a real CreateTransformJobInput/DescribeTransformJobOutput field")
}

// ---------------------------------------------------------------------------
// UpdateFeatureGroup tests (gap #19)
// ---------------------------------------------------------------------------

func TestHandler_Persistence_TransformJob(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateModel", map[string]any{"ModelName": "snap-model"})

	doSageMakerRequest(t, h, "CreateTransformJob", map[string]any{
		"TransformJobName": "snap-transform",
		"ModelName":        "snap-model",
		"TransformInput": map[string]any{
			"DataSource": map[string]any{
				"S3DataSource": map[string]any{"S3Uri": "s3://bucket/input"},
			},
		},
		"TransformOutput":    map[string]any{"S3OutputPath": "s3://bucket/output"},
		"TransformResources": map[string]any{"InstanceType": "ml.m5.large", "InstanceCount": 1},
	})

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := sagemaker.NewHandler(sagemaker.NewInMemoryBackend("000000000000", "us-east-1"))
	require.NoError(t, h2.Restore(t.Context(), snap))

	rec := doSageMakerRequest(t, h2, "DescribeTransformJob", map[string]any{
		"TransformJobName": "snap-transform",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---------------------------------------------------------------------------
// Tag coverage extended to featureGroups, experiments, trials (gap #27)
// ---------------------------------------------------------------------------

func TestHandler_Tags_TransformJob(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateModel", map[string]any{"ModelName": "m1"})

	rec := doSageMakerRequest(t, h, "CreateTransformJob", map[string]any{
		"TransformJobName": "tagged-transform",
		"ModelName":        "m1",
		"TransformInput": map[string]any{
			"DataSource": map[string]any{
				"S3DataSource": map[string]any{"S3Uri": "s3://b/i"},
			},
		},
		"TransformOutput":    map[string]any{"S3OutputPath": "s3://b/o"},
		"TransformResources": map[string]any{"InstanceType": "ml.m5.large", "InstanceCount": 1},
		"Tags": []any{
			map[string]any{"Key": "owner", "Value": "alice"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	jobARN := createResp["TransformJobArn"]
	require.NotEmpty(t, jobARN)

	rec = doSageMakerRequest(t, h, "ListTags", map[string]any{
		"ResourceArn": jobARN,
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var tagsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tagsResp))
	tags := tagsResp["Tags"].([]any)
	require.Len(t, tags, 1)
	assert.Equal(t, "owner", tags[0].(map[string]any)["Key"])
}

// ---------------------------------------------------------------------------
// GetSupportedOperations covers new ops
// ---------------------------------------------------------------------------
