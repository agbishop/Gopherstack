package sagemaker_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_DescribeEndpoint_FullResponse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create endpoint config with a production variant
	doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
		"EndpointConfigName": "ec",
		"ProductionVariants": []any{
			map[string]any{
				"VariantName":          "main",
				"ModelName":            "my-model",
				"InitialInstanceCount": 1,
				"InstanceType":         "ml.m5.large",
				"InitialVariantWeight": 1.0,
			},
		},
	})

	// Create endpoint
	rec := doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
		"EndpointName":       "ep1",
		"EndpointConfigName": "ec",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Describe
	rec = doSageMakerRequest(t, h, "DescribeEndpoint", map[string]any{
		"EndpointName": "ep1",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	assert.Equal(t, "ep1", descResp["EndpointName"])
	assert.NotEmpty(t, descResp["EndpointArn"])
	// Status is Creating initially (FSM transitions async)
	assert.Contains(t, []any{"Creating", "InService"}, descResp["EndpointStatus"])

	// ProductionVariants must be populated from the endpoint config, using
	// the AWS DescribeEndpoint wire shape (Desired*/Current*, not Initial*).
	variants, ok := descResp["ProductionVariants"].([]any)
	require.True(t, ok, "ProductionVariants must be present in DescribeEndpoint response")
	require.Len(t, variants, 1)
	variant, ok := variants[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "main", variant["VariantName"])
	assert.InDelta(t, 1.0, variant["DesiredWeight"], 0.0001)
	assert.InDelta(t, float64(1), variant["DesiredInstanceCount"], 0.0001)
	assert.Nil(t, variant["CurrentWeight"])
	assert.Nil(t, variant["CurrentInstanceCount"])
}

func TestHandler_DescribeEndpoint_DataCaptureAndAsyncInferenceConfig(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
		"EndpointConfigName": "ec-capture",
		"ProductionVariants": []any{
			map[string]any{
				"VariantName": "main", "ModelName": "m", "InitialInstanceCount": 1, "InstanceType": "ml.m5.large",
			},
		},
		"DataCaptureConfig": map[string]any{
			"EnableCapture":             true,
			"InitialSamplingPercentage": 50,
			"DestinationS3Uri":          "s3://bucket/capture/",
		},
		"AsyncInferenceConfig": map[string]any{
			"OutputConfig": map[string]any{"S3OutputPath": "s3://bucket/async-out/"},
		},
	})

	doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
		"EndpointName": "ep-capture", "EndpointConfigName": "ec-capture",
	})

	rec := doSageMakerRequest(t, h, "DescribeEndpoint", map[string]any{"EndpointName": "ep-capture"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	dcc, ok := resp["DataCaptureConfig"].(map[string]any)
	require.True(t, ok, "DataCaptureConfig must be surfaced from the active EndpointConfig")
	assert.Equal(t, "Started", dcc["CaptureStatus"])
	assert.InDelta(t, 50, dcc["CurrentSamplingPercentage"], 0)
	assert.Equal(t, "s3://bucket/capture/", dcc["DestinationS3Uri"])

	aic, ok := resp["AsyncInferenceConfig"].(map[string]any)
	require.True(t, ok, "AsyncInferenceConfig must be surfaced from the active EndpointConfig")
	outputConfig, ok := aic["OutputConfig"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "s3://bucket/async-out/", outputConfig["S3OutputPath"])
}

func TestHandler_CreateEndpoint_DeploymentConfigEchoedOnDescribe(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
		"EndpointConfigName": "ec-deploy",
		"ProductionVariants": []any{
			map[string]any{
				"VariantName": "main", "ModelName": "m", "InitialInstanceCount": 1, "InstanceType": "ml.m5.large",
			},
		},
	})

	doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
		"EndpointName":       "ep-deploy",
		"EndpointConfigName": "ec-deploy",
		"DeploymentConfig": map[string]any{
			"RollingUpdatePolicy": map[string]any{
				"MaximumBatchSize": map[string]any{"Type": "INSTANCE_COUNT", "Value": 1},
			},
		},
	})

	rec := doSageMakerRequest(t, h, "DescribeEndpoint", map[string]any{"EndpointName": "ep-deploy"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	lastDeploymentConfig, ok := resp["LastDeploymentConfig"].(map[string]any)
	require.True(t, ok, "LastDeploymentConfig must echo the DeploymentConfig sent to Create/UpdateEndpoint")
	rollingUpdatePolicy, ok := lastDeploymentConfig["RollingUpdatePolicy"].(map[string]any)
	require.True(t, ok)
	maxBatchSize, ok := rollingUpdatePolicy["MaximumBatchSize"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "INSTANCE_COUNT", maxBatchSize["Type"])
}

func TestHandler_UpdateEndpoint_RetainAllVariantProperties(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
		"EndpointConfigName": "ec-retain-1",
		"ProductionVariants": []any{
			map[string]any{
				"VariantName": "main", "ModelName": "m", "InitialInstanceCount": 1, "InitialVariantWeight": 1.0,
				"InstanceType": "ml.m5.large",
			},
		},
	})
	doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
		"EndpointConfigName": "ec-retain-2",
		"ProductionVariants": []any{
			map[string]any{
				"VariantName": "main", "ModelName": "m", "InitialInstanceCount": 5, "InitialVariantWeight": 9.0,
				"InstanceType": "ml.m5.large",
			},
		},
	})

	doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
		"EndpointName": "ep-retain", "EndpointConfigName": "ec-retain-1",
	})

	t.Run("default does not retain, takes new EndpointConfig's Desired values", func(t *testing.T) {
		t.Parallel()

		rec := doSageMakerRequest(t, h, "UpdateEndpoint", map[string]any{
			"EndpointName": "ep-retain", "EndpointConfigName": "ec-retain-2",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = doSageMakerRequest(t, h, "DescribeEndpoint", map[string]any{"EndpointName": "ep-retain"})
		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		variant := resp["ProductionVariants"].([]any)[0].(map[string]any)
		assert.InDelta(t, float64(5), variant["DesiredInstanceCount"], 0)
		assert.InDelta(t, 9.0, variant["DesiredWeight"], 0.0001)
	})

	t.Run("RetainAllVariantProperties keeps the old Desired values", func(t *testing.T) {
		t.Parallel()

		doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
			"EndpointName": "ep-retain-true", "EndpointConfigName": "ec-retain-1",
		})

		rec := doSageMakerRequest(t, h, "UpdateEndpoint", map[string]any{
			"EndpointName": "ep-retain-true", "EndpointConfigName": "ec-retain-2",
			"RetainAllVariantProperties": true,
		})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = doSageMakerRequest(t, h, "DescribeEndpoint", map[string]any{"EndpointName": "ep-retain-true"})
		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		variant := resp["ProductionVariants"].([]any)[0].(map[string]any)
		assert.InDelta(t, float64(1), variant["DesiredInstanceCount"], 0)
		assert.InDelta(t, 1.0, variant["DesiredWeight"], 0.0001)
	})

	t.Run("ExcludeRetainedVariantProperties overrides one retained property", func(t *testing.T) {
		t.Parallel()

		doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
			"EndpointName": "ep-retain-exclude", "EndpointConfigName": "ec-retain-1",
		})

		rec := doSageMakerRequest(t, h, "UpdateEndpoint", map[string]any{
			"EndpointName": "ep-retain-exclude", "EndpointConfigName": "ec-retain-2",
			"RetainAllVariantProperties":       true,
			"ExcludeRetainedVariantProperties": []string{"DesiredInstanceCount"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = doSageMakerRequest(t, h, "DescribeEndpoint", map[string]any{"EndpointName": "ep-retain-exclude"})
		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		variant := resp["ProductionVariants"].([]any)[0].(map[string]any)
		// Excluded, so takes the new config's value instead of retaining.
		assert.InDelta(t, float64(5), variant["DesiredInstanceCount"], 0)
		// Not excluded, so still retained from the old config.
		assert.InDelta(t, 1.0, variant["DesiredWeight"], 0.0001)
	})
}

func TestHandler_ListEndpoints_Filters(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
		"EndpointConfigName": "ec-list-filters",
		"ProductionVariants": []any{
			map[string]any{
				"VariantName": "main", "ModelName": "m", "InitialInstanceCount": 1, "InstanceType": "ml.m5.large",
			},
		},
	})
	// Both endpoints' Creating -> InService transitions fire inside this
	// bubble so StatusEquals below is not racing the async FSM transition.
	// future/past are computed here too, against the bubble's fake clock:
	// CreationTime/LastModifiedTime were stamped with that same fake clock,
	// which does not track the real wall clock read outside the bubble.
	var future, past float64

	synctest.Test(t, func(t *testing.T) {
		doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
			"EndpointName": "list-filter-a", "EndpointConfigName": "ec-list-filters",
		})
		doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
			"EndpointName": "list-filter-b", "EndpointConfigName": "ec-list-filters",
		})

		time.Sleep(time.Second)
		synctest.Wait()

		future = float64(time.Now().Add(time.Hour).Unix())
		past = float64(time.Now().Add(-time.Hour).Unix())
	})

	tests := []struct {
		body      map[string]any
		name      string
		wantCount int
	}{
		{name: "name contains", body: map[string]any{"NameContains": "list-filter-a"}, wantCount: 1},
		{name: "status equals match", body: map[string]any{"StatusEquals": "InService"}, wantCount: 2},
		{name: "status equals no match", body: map[string]any{"StatusEquals": "Failed"}, wantCount: 0},
		{name: "creation time after future excludes", body: map[string]any{"CreationTimeAfter": future}, wantCount: 0},
		{name: "creation time after past includes", body: map[string]any{"CreationTimeAfter": past}, wantCount: 2},
		{
			name:      "last modified before past excludes",
			body:      map[string]any{"LastModifiedTimeBefore": past},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doSageMakerRequest(t, h, "ListEndpoints", tc.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			endpoints, _ := resp["Endpoints"].([]any)
			assert.Len(t, endpoints, tc.wantCount)
		})
	}
}

func TestHandler_ListEndpoints_SortByName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
		"EndpointConfigName": "ec-sort",
		"ProductionVariants": []any{
			map[string]any{
				"VariantName": "main", "ModelName": "m", "InitialInstanceCount": 1, "InstanceType": "ml.m5.large",
			},
		},
	})
	doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
		"EndpointName": "sort-b", "EndpointConfigName": "ec-sort",
	})
	doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
		"EndpointName": "sort-a", "EndpointConfigName": "ec-sort",
	})

	rec := doSageMakerRequest(t, h, "ListEndpoints", map[string]any{"SortBy": "Name", "SortOrder": "Ascending"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	endpoints, ok := resp["Endpoints"].([]any)
	require.True(t, ok)
	require.Len(t, endpoints, 2)
	assert.Equal(t, "sort-a", endpoints[0].(map[string]any)["EndpointName"])
}

func TestHandler_CreateEndpoint_UnknownEndpointConfig(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
		"EndpointName":       "ep-missing-config",
		"EndpointConfigName": "does-not-exist",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ValidationException", errResp["__type"])

	rec = doSageMakerRequest(t, h, "DescribeEndpoint", map[string]any{
		"EndpointName": "ep-missing-config",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_DescribeEndpoint_EventuallyInService(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
		"EndpointConfigName": "ec2",
		"ProductionVariants": []any{
			map[string]any{
				"VariantName":          "v1",
				"ModelName":            "m1",
				"InitialInstanceCount": 1,
				"InstanceType":         "ml.m5.large",
				"InitialVariantWeight": 1.0,
			},
		},
	})

	var descResp map[string]any

	synctest.Test(t, func(t *testing.T) {
		doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
			"EndpointName":       "ep2",
			"EndpointConfigName": "ec2",
		})

		time.Sleep(time.Second)
		synctest.Wait()

		rec := doSageMakerRequest(t, h, "DescribeEndpoint", map[string]any{
			"EndpointName": "ep2",
		})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	})

	require.Equal(t, "InService", descResp["EndpointStatus"])

	variants, ok := descResp["ProductionVariants"].([]any)
	require.True(t, ok)
	require.Len(t, variants, 1)
	variant, ok := variants[0].(map[string]any)
	require.True(t, ok)
	// Once InService, Current* mirrors Desired*.
	assert.InDelta(t, 1.0, variant["CurrentWeight"], 0.0001)
	assert.InDelta(t, float64(1), variant["CurrentInstanceCount"], 0.0001)
}

// ---------------------------------------------------------------------------
// Snapshot/Restore preserves TransformJob, featureRecords, featureMetadata (gap #28)
// ---------------------------------------------------------------------------

func TestHandler_EndpointLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create endpoint config first.
	doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
		"EndpointConfigName": "my-ep-config",
		"ProductionVariants": []map[string]any{
			{
				"VariantName":          "main",
				"ModelName":            "m",
				"InstanceType":         "ml.m5.large",
				"InitialInstanceCount": 1,
			},
		},
	})

	// Create endpoint.
	recCreate := doSageMakerRequest(t, h, "CreateEndpoint", map[string]any{
		"EndpointName":       "my-endpoint",
		"EndpointConfigName": "my-ep-config",
	})
	assert.Equal(t, http.StatusOK, recCreate.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &createOut))
	assert.NotEmpty(t, createOut["EndpointArn"])

	// Describe endpoint.
	recDesc := doSageMakerRequest(
		t,
		h,
		"DescribeEndpoint",
		map[string]any{"EndpointName": "my-endpoint"},
	)
	assert.Equal(t, http.StatusOK, recDesc.Code)

	// List endpoints.
	recList := doSageMakerRequest(t, h, "ListEndpoints", map[string]any{})
	assert.Equal(t, http.StatusOK, recList.Code)

	var listOut map[string]any
	require.NoError(t, json.Unmarshal(recList.Body.Bytes(), &listOut))
	assert.Len(t, listOut["Endpoints"].([]any), 1)

	// Update endpoint.
	recUpdate := doSageMakerRequest(t, h, "UpdateEndpoint", map[string]any{
		"EndpointName":       "my-endpoint",
		"EndpointConfigName": "my-ep-config",
	})
	assert.Equal(t, http.StatusOK, recUpdate.Code)

	// UpdateEndpointWeightsAndCapacities.
	recUpdateWeights := doSageMakerRequest(
		t,
		h,
		"UpdateEndpointWeightsAndCapacities",
		map[string]any{
			"EndpointName":                "my-endpoint",
			"DesiredWeightsAndCapacities": []map[string]any{},
		},
	)
	assert.Equal(t, http.StatusOK, recUpdateWeights.Code)

	// Delete endpoint.
	recDelete := doSageMakerRequest(
		t,
		h,
		"DeleteEndpoint",
		map[string]any{"EndpointName": "my-endpoint"},
	)
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

func TestHandler_Endpoint_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, op := range []string{"DescribeEndpoint", "UpdateEndpoint", "DeleteEndpoint"} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()

			rec := doSageMakerRequest(t, h, op, map[string]any{"EndpointName": "nonexistent"})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// ---------------------------------------------------------------------------
// Training Job lifecycle
// ---------------------------------------------------------------------------
