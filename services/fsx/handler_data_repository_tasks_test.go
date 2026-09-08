package fsx_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFSx_DataRepositoryTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		taskType string
		wantCode int
	}{
		{
			name:     "create EXPORT_TO_REPOSITORY task",
			taskType: "EXPORT_TO_REPOSITORY",
			wantCode: http.StatusOK,
		},
		{
			name:     "create IMPORT_METADATA_FROM_REPOSITORY task",
			taskType: "IMPORT_METADATA_FROM_REPOSITORY",
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			fsID := createFS(t, h, "LUSTRE")

			rec := doFSxRequest(t, h, "CreateDataRepositoryTask", map[string]any{
				"FileSystemId": fsID,
				"Type":         tc.taskType,
				"Paths":        []string{"/data"},
				"Report":       map[string]any{"Enabled": false},
			})
			require.Equal(t, tc.wantCode, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			task := out["DataRepositoryTask"].(map[string]any)
			assert.Contains(t, task["TaskId"].(string), "task-")
			assert.Equal(t, "EXECUTING", task["Lifecycle"])
		})
	}
}

// TestFSx_DataRepositoryTask_MissingReport verifies gopherstack-4ggy's fix:
// Report is a required CreateDataRepositoryTaskInput member
// (api_op_CreateDataRepositoryTask.go:49-129) that the pre-fix request never
// read at all, so a request that omitted it (or omitted Report.Enabled)
// still succeeded.
func TestFSx_DataRepositoryTask_MissingReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{
			name: "missing report",
			body: map[string]any{"Type": "EXPORT_TO_REPOSITORY"},
		},
		{
			name: "missing report enabled",
			body: map[string]any{"Type": "EXPORT_TO_REPOSITORY", "Report": map[string]any{}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			fsID := createFS(t, h, "LUSTRE")

			body := map[string]any{"FileSystemId": fsID}
			maps.Copy(body, tc.body)

			rec := doFSxRequest(t, h, "CreateDataRepositoryTask", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestFSx_DataRepositoryTaskLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("describe and cancel cycle", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		fsID := createFS(t, h, "LUSTRE")

		rec := doFSxRequest(t, h, "CreateDataRepositoryTask", map[string]any{
			"FileSystemId": fsID,
			"Type":         "EXPORT_TO_REPOSITORY",
			"Report":       map[string]any{"Enabled": false},
		})
		require.Equal(t, http.StatusOK, rec.Code)
		var cr map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cr))
		taskID := cr["DataRepositoryTask"].(map[string]any)["TaskId"].(string)

		// describe
		rec2 := doFSxRequest(t, h, "DescribeDataRepositoryTasks", map[string]any{
			"TaskIds": []string{taskID},
		})
		require.Equal(t, http.StatusOK, rec2.Code)
		var dr map[string]any
		require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &dr))
		assert.Len(t, dr["DataRepositoryTasks"].([]any), 1)

		// cancel
		rec3 := doFSxRequest(t, h, "CancelDataRepositoryTask", map[string]any{"TaskId": taskID})
		require.Equal(t, http.StatusOK, rec3.Code)
		var car map[string]any
		require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &car))
		assert.Equal(t, "CANCELING", car["Lifecycle"])
	})
}

// TestFSx_CreateDataRepositoryTask_RejectsConcurrentExecuting verifies the
// real DataRepositoryTaskExecuting exception (fsx@v1.68.4 types/errors.go:
// "An existing data repository task is currently executing on the file
// system. Wait until the existing task has completed, then create the new
// task."): a second CreateDataRepositoryTask on a file system that already
// has an EXECUTING task must be rejected; a different file system's own
// EXECUTING task must not block it (the guard is per-file-system, not
// global); and cancelling the blocking task must free the file system up
// for a new one.
func TestFSx_CreateDataRepositoryTask_RejectsConcurrentExecuting(t *testing.T) {
	t.Parallel()

	newTaskBody := func(fsID string) map[string]any {
		return map[string]any{
			"FileSystemId": fsID,
			"Type":         "EXPORT_TO_REPOSITORY",
			"Report":       map[string]any{"Enabled": false},
		}
	}

	t.Run("second task on same file system rejected", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		fsID := createFS(t, h, "LUSTRE")

		rec1 := doFSxRequest(t, h, "CreateDataRepositoryTask", newTaskBody(fsID))
		require.Equal(t, http.StatusOK, rec1.Code)

		rec2 := doFSxRequest(t, h, "CreateDataRepositoryTask", newTaskBody(fsID))
		require.Equal(t, http.StatusBadRequest, rec2.Code)

		var out map[string]any
		require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))
		assert.Equal(t, "DataRepositoryTaskExecuting", out["__type"])
	})

	t.Run("other file system unaffected", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		fs1 := createFS(t, h, "LUSTRE")
		fs2 := createFS(t, h, "LUSTRE")

		rec1 := doFSxRequest(t, h, "CreateDataRepositoryTask", newTaskBody(fs1))
		require.Equal(t, http.StatusOK, rec1.Code)

		rec2 := doFSxRequest(t, h, "CreateDataRepositoryTask", newTaskBody(fs2))
		assert.Equal(t, http.StatusOK, rec2.Code, "an EXECUTING task on a different file system must not block")
	})

	t.Run("freed up after cancel", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		fsID := createFS(t, h, "LUSTRE")

		rec1 := doFSxRequest(t, h, "CreateDataRepositoryTask", newTaskBody(fsID))
		require.Equal(t, http.StatusOK, rec1.Code)
		var cr map[string]any
		require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &cr))
		taskID := cr["DataRepositoryTask"].(map[string]any)["TaskId"].(string)

		cancelRec := doFSxRequest(t, h, "CancelDataRepositoryTask", map[string]any{"TaskId": taskID})
		require.Equal(t, http.StatusOK, cancelRec.Code)

		rec2 := doFSxRequest(t, h, "CreateDataRepositoryTask", newTaskBody(fsID))
		assert.Equal(t, http.StatusOK, rec2.Code, "a CANCELING task must not block a new one")
	})
}

// TestDataRepositoryTask_TagsStoredAtCreation verifies that tags passed
// to CreateDataRepositoryTask are persisted and retrievable via ListTagsForResource.
// Previously CreateDataRepositoryTask did not populate b.tags[arn], so creation-time
// tags were silently dropped.
func TestDataRepositoryTask_TagsStoredAtCreation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []map[string]string
	}{
		{
			name: "single_tag",
			tags: []map[string]string{{"Key": "env", "Value": "test"}},
		},
		{
			name: "multiple_tags",
			tags: []map[string]string{
				{"Key": "env", "Value": "prod"},
				{"Key": "team", "Value": "data"},
			},
		},
		{
			name: "no_tags",
			tags: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			fsID := createFS(t, h, "LUSTRE")

			body := map[string]any{
				"FileSystemId": fsID,
				"Type":         "EXPORT_TO_REPOSITORY",
				"Report":       map[string]any{"Enabled": false},
			}
			if tc.tags != nil {
				body["Tags"] = tc.tags
			}

			rec := doFSxRequest(t, h, "CreateDataRepositoryTask", body)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			task := out["DataRepositoryTask"].(map[string]any)
			taskARN := task["ResourceARN"].(string)
			require.NotEmpty(t, taskARN)

			// Tags must be retrievable via ListTagsForResource.
			rec2 := doFSxRequest(t, h, "ListTagsForResource", map[string]any{"ResourceARN": taskARN})
			require.Equal(t, http.StatusOK, rec2.Code, "ListTagsForResource on DRT must succeed")

			var tagOut map[string]any
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &tagOut))

			tags, ok := tagOut["Tags"].([]any)
			require.True(t, ok, "Tags must be a JSON array")
			assert.Len(t, tags, len(tc.tags))
		})
	}
}

// TestDataRepositoryTask_TagResource verifies that TagResource works on
// DataRepositoryTask ARNs. Previously arnExists() did not check DRT ARNs,
// causing TagResource to return FileSystemNotFound for task ARNs.
func TestDataRepositoryTask_TagResource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	fsID := createFS(t, h, "LUSTRE")

	rec := doFSxRequest(t, h, "CreateDataRepositoryTask", map[string]any{
		"FileSystemId": fsID,
		"Type":         "EXPORT_TO_REPOSITORY",
		"Report":       map[string]any{"Enabled": false},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	taskARN := out["DataRepositoryTask"].(map[string]any)["ResourceARN"].(string)

	// TagResource on a DRT ARN must succeed.
	rec2 := doFSxRequest(t, h, "TagResource", map[string]any{
		"ResourceARN": taskARN,
		"Tags":        []map[string]string{{"Key": "added", "Value": "after"}},
	})
	assert.Equal(t, http.StatusOK, rec2.Code, "TagResource on DRT must succeed")

	// Verify the tag is now visible.
	rec3 := doFSxRequest(t, h, "ListTagsForResource", map[string]any{"ResourceARN": taskARN})
	require.Equal(t, http.StatusOK, rec3.Code)

	var tagOut map[string]any
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &tagOut))
	tags := tagOut["Tags"].([]any)
	require.Len(t, tags, 1)
	assert.Equal(t, "added", tags[0].(map[string]any)["Key"])
}
