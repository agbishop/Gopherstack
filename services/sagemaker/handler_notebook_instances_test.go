package sagemaker_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_NotebookInstanceLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create notebook instance.
	recCreate := doSageMakerRequest(t, h, "CreateNotebookInstance", map[string]any{
		"NotebookInstanceName": "my-notebook",
		"InstanceType":         "ml.t2.medium",
		"RoleArn":              "arn:aws:iam::000000000000:role/notebook-role",
	})
	assert.Equal(t, http.StatusOK, recCreate.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &createOut))
	assert.NotEmpty(t, createOut["NotebookInstanceArn"])

	// Describe.
	recDesc := doSageMakerRequest(t, h, "DescribeNotebookInstance", map[string]any{
		"NotebookInstanceName": "my-notebook",
	})
	assert.Equal(t, http.StatusOK, recDesc.Code)

	// List.
	recList := doSageMakerRequest(t, h, "ListNotebookInstances", map[string]any{})
	assert.Equal(t, http.StatusOK, recList.Code)

	var listOut map[string]any
	require.NoError(t, json.Unmarshal(recList.Body.Bytes(), &listOut))
	assert.Len(t, listOut["NotebookInstances"].([]any), 1)

	// Start.
	recStart := doSageMakerRequest(t, h, "StartNotebookInstance", map[string]any{
		"NotebookInstanceName": "my-notebook",
	})
	assert.Equal(t, http.StatusOK, recStart.Code)

	// Stop before update: real AWS requires Stopped state to update a notebook instance.
	recStop := doSageMakerRequest(t, h, "StopNotebookInstance", map[string]any{
		"NotebookInstanceName": "my-notebook",
	})
	assert.Equal(t, http.StatusOK, recStop.Code)

	// Update (notebook is now Stopped).
	recUpdate := doSageMakerRequest(t, h, "UpdateNotebookInstance", map[string]any{
		"NotebookInstanceName": "my-notebook",
		"InstanceType":         "ml.t3.medium",
	})
	assert.Equal(t, http.StatusOK, recUpdate.Code)

	// CreatePresignedNotebookInstanceUrl.
	recURL := doSageMakerRequest(t, h, "CreatePresignedNotebookInstanceUrl", map[string]any{
		"NotebookInstanceName": "my-notebook",
	})
	assert.Equal(t, http.StatusOK, recURL.Code)

	// Delete.
	recDelete := doSageMakerRequest(t, h, "DeleteNotebookInstance", map[string]any{
		"NotebookInstanceName": "my-notebook",
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

// TestHandler_DeleteNotebookInstance_NotStopped asserts DeleteNotebookInstance
// rejects a notebook instance that has not been stopped, matching
// UpdateNotebookInstanceFull's sibling guard (api_op_DeleteNotebookInstance.go:
// "Before you can delete a notebook instance, you must call the
// StopNotebookInstance API.").
func TestHandler_DeleteNotebookInstance_NotStopped(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateNotebookInstance", map[string]any{
		"NotebookInstanceName": "del-notebook",
		"InstanceType":         "ml.t2.medium",
		"RoleArn":              "arn:aws:iam::000000000000:role/notebook-role",
	})

	// Cannot delete while still Pending (not Stopped).
	recEarly := doSageMakerRequest(t, h, "DeleteNotebookInstance", map[string]any{
		"NotebookInstanceName": "del-notebook",
	})
	assert.Equal(t, http.StatusBadRequest, recEarly.Code)

	recStop := doSageMakerRequest(t, h, "StopNotebookInstance", map[string]any{
		"NotebookInstanceName": "del-notebook",
	})
	require.Equal(t, http.StatusOK, recStop.Code)

	recDelete := doSageMakerRequest(t, h, "DeleteNotebookInstance", map[string]any{
		"NotebookInstanceName": "del-notebook",
	})
	assert.Equal(t, http.StatusOK, recDelete.Code)
}

func TestHandler_NotebookInstance_EventuallyInService(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateNotebookInstance", map[string]any{
		"NotebookInstanceName": "async-notebook",
		"InstanceType":         "ml.t2.medium",
		"RoleArn":              "arn:aws:iam::000000000000:role/notebook-role",
	})

	// Wait for async status transition.
	time.Sleep(300 * time.Millisecond)

	recDesc := doSageMakerRequest(t, h, "DescribeNotebookInstance", map[string]any{
		"NotebookInstanceName": "async-notebook",
	})
	assert.Equal(t, http.StatusOK, recDesc.Code)

	var descOut map[string]any
	require.NoError(t, json.Unmarshal(recDesc.Body.Bytes(), &descOut))
	assert.NotEmpty(t, descOut["NotebookInstanceStatus"])
}

// TestUpdateNotebookInstance_RequiresStoppedState verifies that updating a notebook
// instance that is not in Stopped status returns 400. Real AWS returns ValidationException
// for updates on InService, Pending, Stopping, or other non-Stopped notebooks.
func TestUpdateNotebookInstance_RequiresStoppedState(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create a notebook instance.
	rec := doSageMakerRequest(t, h, "CreateNotebookInstance", map[string]any{
		"NotebookInstanceName": "update-state-nb",
		"InstanceType":         "ml.t2.medium",
		"RoleArn":              "arn:aws:iam::123456789012:role/SageMakerRole",
	})
	require.Equal(t, http.StatusOK, rec.Code, "CreateNotebookInstance failed: %s", rec.Body.String())

	// While still in Pending/InService state (freshly created), update must be rejected.
	rec = doSageMakerRequest(t, h, "UpdateNotebookInstance", map[string]any{
		"NotebookInstanceName": "update-state-nb",
		"InstanceType":         "ml.t3.medium",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"UpdateNotebookInstance on non-Stopped notebook must return 400; body: %s",
		rec.Body.String())
}
