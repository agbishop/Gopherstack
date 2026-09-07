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

func TestHandler_InferenceComponentLifecycle(t *testing.T) {
	t.Parallel()

	// The whole lifecycle runs inside one bubble: runDelayed's goroutine (and
	// the WaitGroup it uses) must not be touched from both inside and outside
	// a synctest bubble on the same backend, so every request against h below
	// -- including UpdateInferenceComponent/UpdateInferenceComponentRuntimeConfig,
	// which each schedule their own delayed transition -- has to stay in this
	// bubble too, not just the initial Create -> InService wait.
	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		rec := doSageMakerRequest(t, h, "CreateInferenceComponent", map[string]any{
			"InferenceComponentName": "my-component",
			"EndpointName":           "my-endpoint",
			"VariantName":            "variant-1",
			"RuntimeConfig":          map[string]any{"CopyCount": 2},
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var createResp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
		assert.Contains(t, createResp["InferenceComponentArn"], "my-component")

		// Describe
		rec = doSageMakerRequest(t, h, "DescribeInferenceComponent", map[string]any{
			"InferenceComponentName": "my-component",
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var descResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
		assert.Equal(t, "my-component", descResp["InferenceComponentName"])
		assert.Equal(t, "my-endpoint", descResp["EndpointName"])
		assert.Contains(t, descResp["EndpointArn"], "endpoint/my-endpoint")
		assert.Equal(t, "variant-1", descResp["VariantName"])
		assert.Equal(t, "Creating", descResp["InferenceComponentStatus"])
		assert.Nil(t, descResp["Tags"], "DescribeInferenceComponentOutput has no Tags member")

		runtimeConfig, ok := descResp["RuntimeConfig"].(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, 2, runtimeConfig["DesiredCopyCount"], 0)
		assert.InDelta(t, 0, runtimeConfig["CurrentCopyCount"], 0)

		time.Sleep(time.Second)
		synctest.Wait()

		// Creating -> InService, and CurrentCopyCount catches up to DesiredCopyCount.
		rec = doSageMakerRequest(t, h, "DescribeInferenceComponent", map[string]any{
			"InferenceComponentName": "my-component",
		})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
		assert.Equal(t, "InService", descResp["InferenceComponentStatus"])

		runtimeConfig, ok = descResp["RuntimeConfig"].(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, 2, runtimeConfig["CurrentCopyCount"], 0)

		// List
		rec = doSageMakerRequest(t, h, "ListInferenceComponents", map[string]any{})
		assert.Equal(t, http.StatusOK, rec.Code)

		var listResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
		components := listResp["InferenceComponents"].([]any)
		require.Len(t, components, 1)

		summary, ok := components[0].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, summary["EndpointArn"], "endpoint/my-endpoint")
		assert.Equal(t, "variant-1", summary["VariantName"])

		// Update runtime config
		rec = doSageMakerRequest(t, h, "UpdateInferenceComponentRuntimeConfig", map[string]any{
			"InferenceComponentName": "my-component",
			"DesiredRuntimeConfig":   map[string]any{"CopyCount": 4},
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var updateResp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
		assert.Contains(t, updateResp["InferenceComponentArn"], "my-component")

		// Update component: DeploymentConfig is opaque and echoed back verbatim
		// under LastDeploymentConfig (the real UpdateInferenceComponentInput has
		// no VariantName member, so a variant move is never attempted here).
		rec = doSageMakerRequest(t, h, "UpdateInferenceComponent", map[string]any{
			"InferenceComponentName": "my-component",
			"DeploymentConfig": map[string]any{
				"RollingUpdatePolicy": map[string]any{
					"MaximumBatchSize": map[string]any{"Type": "COPY_COUNT", "Value": 1},
				},
			},
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		rec = doSageMakerRequest(t, h, "DescribeInferenceComponent", map[string]any{
			"InferenceComponentName": "my-component",
		})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
		assert.Equal(t, "Updating", descResp["InferenceComponentStatus"])

		lastDeploymentConfig, ok := descResp["LastDeploymentConfig"].(map[string]any)
		require.True(t, ok)
		rollingUpdatePolicy, ok := lastDeploymentConfig["RollingUpdatePolicy"].(map[string]any)
		require.True(t, ok)
		maxBatchSize, ok := rollingUpdatePolicy["MaximumBatchSize"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "COPY_COUNT", maxBatchSize["Type"])

		// Delete
		rec = doSageMakerRequest(t, h, "DeleteInferenceComponent", map[string]any{
			"InferenceComponentName": "my-component",
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify deleted
		rec = doSageMakerRequest(t, h, "DescribeInferenceComponent", map[string]any{
			"InferenceComponentName": "my-component",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		// Drain the still-pending Updating -> InService transitions scheduled
		// by UpdateInferenceComponentRuntimeConfig/UpdateInferenceComponent
		// above (both no-op once fired, since the record is gone) so none are
		// left blocked on their timer when the bubble exits.
		time.Sleep(time.Second)
		synctest.Wait()
	})
}

func TestHandler_InferenceComponent_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "DescribeInferenceComponent", map[string]any{
		"InferenceComponentName": "nonexistent",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_CreateInferenceComponent_MissingRequiredFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		body map[string]any
		name string
	}{
		{name: "missing name", body: map[string]any{"EndpointName": "ep"}},
		{name: "missing endpoint name", body: map[string]any{"InferenceComponentName": "x"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doSageMakerRequest(t, h, "CreateInferenceComponent", tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_CreateInferenceComponent_SpecificationContainerImage(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreateInferenceComponent", map[string]any{
		"InferenceComponentName": "spec-component",
		"EndpointName":           "spec-endpoint",
		"Specification": map[string]any{
			"ModelName":    "my-model",
			"InstanceType": "ml.g5.2xlarge",
			"Container": map[string]any{
				"Image":       "000000000000.dkr.ecr.us-east-1.amazonaws.com/my-image:latest",
				"ArtifactUrl": "s3://bucket/model.tar.gz",
				"Environment": map[string]any{"FOO": "bar"},
			},
			"ComputeResourceRequirements": map[string]any{"MinMemoryRequiredInMb": 1024},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "DescribeInferenceComponent", map[string]any{
		"InferenceComponentName": "spec-component",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	spec, ok := resp["Specification"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "my-model", spec["ModelName"])
	assert.Equal(t, "ml.g5.2xlarge", spec["InstanceType"])

	computeResourceRequirements, ok := spec["ComputeResourceRequirements"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 1024, computeResourceRequirements["MinMemoryRequiredInMb"], 0)

	container, ok := spec["Container"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "s3://bucket/model.tar.gz", container["ArtifactUrl"])
	assert.Nil(t, container["Image"], "the response type has no Image member, only DeployedImage")

	deployedImage, ok := container["DeployedImage"].(map[string]any)
	require.True(t, ok, "Container.Image must be echoed back as DeployedImage.SpecifiedImage")
	assert.Equal(
		t, "000000000000.dkr.ecr.us-east-1.amazonaws.com/my-image:latest", deployedImage["SpecifiedImage"],
	)
}

func TestHandler_ListInferenceComponents_Filters(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Both components' Creating -> InService transitions fire inside this
	// bubble so StatusEquals below is not racing the async FSM transition.
	// future/past are computed here too, against the bubble's fake clock:
	// CreationTime/LastModifiedTime were stamped with that same fake clock,
	// which does not track the real wall clock read outside the bubble.
	var future, past float64

	synctest.Test(t, func(t *testing.T) {
		doSageMakerRequest(t, h, "CreateInferenceComponent", map[string]any{
			"InferenceComponentName": "filter-a",
			"EndpointName":           "filter-ep",
			"VariantName":            "variant-a",
		})
		doSageMakerRequest(t, h, "CreateInferenceComponent", map[string]any{
			"InferenceComponentName": "filter-b",
			"EndpointName":           "filter-ep",
			"VariantName":            "variant-b",
		})

		time.Sleep(time.Second)
		synctest.Wait()

		rec := doSageMakerRequest(t, h, "ListInferenceComponents", map[string]any{"StatusEquals": "InService"})
		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		components, _ := resp["InferenceComponents"].([]any)
		require.Len(t, components, 2)

		future = float64(time.Now().Add(time.Hour).Unix())
		past = float64(time.Now().Add(-time.Hour).Unix())
	})

	tests := []struct {
		body      map[string]any
		name      string
		wantCount int
	}{
		{name: "name contains", body: map[string]any{"NameContains": "filter-a"}, wantCount: 1},
		{name: "variant name equals", body: map[string]any{"VariantNameEquals": "variant-b"}, wantCount: 1},
		{name: "status equals no match", body: map[string]any{"StatusEquals": "Failed"}, wantCount: 0},
		{name: "status equals match", body: map[string]any{"StatusEquals": "InService"}, wantCount: 2},
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

			rec := doSageMakerRequest(t, h, "ListInferenceComponents", tc.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			components, _ := resp["InferenceComponents"].([]any)
			assert.Len(t, components, tc.wantCount)
		})
	}
}

func TestHandler_ListInferenceComponents_SortByName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateInferenceComponent", map[string]any{
		"InferenceComponentName": "sort-b", "EndpointName": "sort-ep",
	})
	doSageMakerRequest(t, h, "CreateInferenceComponent", map[string]any{
		"InferenceComponentName": "sort-a", "EndpointName": "sort-ep",
	})

	rec := doSageMakerRequest(t, h, "ListInferenceComponents", map[string]any{
		"SortBy": "Name", "SortOrder": "Ascending",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	components, ok := resp["InferenceComponents"].([]any)
	require.True(t, ok)
	require.Len(t, components, 2)
	assert.Equal(t, "sort-a", components[0].(map[string]any)["InferenceComponentName"])
}

func TestHandler_InferenceComponent_Duplicate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"InferenceComponentName": "dup-component",
		"EndpointName":           "ep",
	}
	doSageMakerRequest(t, h, "CreateInferenceComponent", body)

	rec := doSageMakerRequest(t, h, "CreateInferenceComponent", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_ListInferenceComponents_EndpointFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateInferenceComponent", map[string]any{
		"InferenceComponentName": "comp-a",
		"EndpointName":           "ep-1",
	})
	doSageMakerRequest(t, h, "CreateInferenceComponent", map[string]any{
		"InferenceComponentName": "comp-b",
		"EndpointName":           "ep-2",
	})

	rec := doSageMakerRequest(t, h, "ListInferenceComponents", map[string]any{
		"EndpointNameEquals": "ep-1",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	components := resp["InferenceComponents"].([]any)
	assert.Len(t, components, 1)
}

// ---------------------------------------------------------------------------
// ClusterSchedulerConfig tests
// ---------------------------------------------------------------------------
