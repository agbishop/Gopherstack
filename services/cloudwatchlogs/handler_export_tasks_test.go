package cloudwatchlogs_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwlsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatchlogs"
)

func TestHandler_ExportTask_CancelRoundTrip(t *testing.T) {
	t.Parallel()

	e := echo.New()
	backend := cloudwatchlogs.NewInMemoryBackend()
	h := cloudwatchlogs.NewHandler(backend)

	rec := doLogsRequest(t, h, e, "CreateLogGroup", `{"logGroupName":"/my/group"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	// Create an export task.
	rec = doLogsRequest(t, h, e, "CreateExportTask",
		`{"logGroupName":"/my/group","destination":"my-bucket","from":1000,"to":2000}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createOut))
	taskID, ok := createOut["taskId"].(string)
	require.True(t, ok)
	require.NotEmpty(t, taskID)

	// Cancel the task.
	bodyBytes, err := json.Marshal(map[string]any{"taskId": taskID})
	require.NoError(t, err)
	rec = doLogsRequest(t, h, e, "CancelExportTask", string(bodyBytes))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestHandler_DescribeExportTasks_WireShape locks the AWS wire shape for
// ExportTask (aws-sdk-go-v2 types.ExportTask): Status is a nested
// {code, message} object (types.ExportTaskStatus) and the creation/completion
// timestamps live under a nested executionInfo object
// (types.ExportTaskExecutionInfo), not as flat top-level fields. A real SDK
// client's generated deserializer only reads status.code/status.message and
// executionInfo.creationTime/executionInfo.completionTime -- it would see nil
// for all four if this handler emitted the flat internal-model shape.
func TestHandler_DescribeExportTasks_WireShape(t *testing.T) {
	t.Parallel()

	e := echo.New()
	backend := cloudwatchlogs.NewInMemoryBackend()
	h := cloudwatchlogs.NewHandler(backend)

	cloudwatchlogs.AddExportTaskInternal(backend, cloudwatchlogs.ExportTask{
		TaskID:         "t1",
		TaskName:       "my-task",
		LogGroupName:   "/grp",
		Destination:    "my-bucket",
		Status:         "COMPLETED",
		StatusMessage:  "Completed successfully. Exported 3 log events.",
		From:           1000,
		To:             2000,
		CreationTime:   1700000000000,
		CompletionTime: 1700000005000,
	})

	rec := doLogsRequest(t, h, e, "DescribeExportTasks", `{}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		ExportTasks []struct {
			Status *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"status"`
			ExecutionInfo *struct {
				CreationTime   int64 `json:"creationTime"`
				CompletionTime int64 `json:"completionTime"`
			} `json:"executionInfo"`
			TaskID string `json:"taskId"`
		} `json:"exportTasks"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.ExportTasks, 1)

	task := out.ExportTasks[0]
	assert.Equal(t, "t1", task.TaskID)
	require.NotNil(t, task.Status, "status must be a nested object, not a flat string")
	assert.Equal(t, "COMPLETED", task.Status.Code)
	assert.Equal(t, "Completed successfully. Exported 3 log events.", task.Status.Message)
	require.NotNil(t, task.ExecutionInfo, "executionInfo must be a nested object")
	assert.Equal(t, int64(1700000000000), task.ExecutionInfo.CreationTime)
	assert.Equal(t, int64(1700000005000), task.ExecutionInfo.CompletionTime)

	// The raw JSON must not carry the old flat keys at the top level of an
	// export task entry.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	rawTasks, ok := raw["exportTasks"].([]any)
	require.True(t, ok)
	require.Len(t, rawTasks, 1)
	rawTask, ok := rawTasks[0].(map[string]any)
	require.True(t, ok)
	_, hasFlatStatusMessage := rawTask["statusMessage"]
	assert.False(t, hasFlatStatusMessage, "statusMessage must not appear flat on the export task")
	_, hasFlatCreationTime := rawTask["creationTime"]
	assert.False(t, hasFlatCreationTime, "creationTime must not appear flat on the export task")
	_, hasFlatCompletionTime := rawTask["completionTime"]
	assert.False(t, hasFlatCompletionTime, "completionTime must not appear flat on the export task")
}

func TestHandler_ImportTask_CancelRoundTrip(t *testing.T) {
	t.Parallel()

	e := echo.New()
	backend := cloudwatchlogs.NewInMemoryBackend()
	h := cloudwatchlogs.NewHandler(backend)

	// Create an import task.
	createBody := `{"importRoleArn":"arn:aws:iam::123:role/my-role",` +
		`"importSourceArn":"arn:aws:cloudtrail:us-east-1:123:eventdatastore/abc"}`
	rec := doLogsRequest(t, h, e, "CreateImportTask", createBody)
	require.Equal(t, http.StatusOK, rec.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createOut))
	importID, ok := createOut["importId"].(string)
	require.True(t, ok)
	require.NotEmpty(t, importID)

	importDestARN, ok := createOut["importDestinationArn"].(string)
	require.True(t, ok)
	require.NotEmpty(t, importDestARN)

	// Cancel the task.
	bodyBytes, err := json.Marshal(map[string]any{"importId": importID})
	require.NoError(t, err)
	rec = doLogsRequest(t, h, e, "CancelImportTask", string(bodyBytes))
	require.Equal(t, http.StatusOK, rec.Code)

	var cancelOut map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cancelOut))
	assert.Equal(t, importID, cancelOut["importId"])
	assert.Equal(t, "CANCELLED", cancelOut["importStatus"])
}

// TestHandler_DescribeImportTasks_WireShape locks the AWS wire shape for
// DescribeImportTasks end to end, via the real aws-sdk-go-v2 client rather
// than a raw map: the response wrapper key is "imports", not "importTasks"
// (deserializers.go's ...DescribeImportTasksOutput case "imports":), and the
// request filter key is "importId", not "taskId" (serializers.go's
// ...DescribeImportTasksInput case "importId":). A previous revision used
// ExportTask's "taskId"/"importTasks" convention on Import by mistake, so a
// real client's ImportId filter never reached the backend and its typed
// Imports field was always empty regardless of what the backend tracked.
// Within an import task, aws-sdk-go-v2 types.Import.ImportStatus serializes
// to "importStatus", not "status", and ImportRoleArn is not a field on the
// real Import type at all.
func TestHandler_DescribeImportTasks_WireShape(t *testing.T) {
	t.Parallel()

	backend := cloudwatchlogs.NewInMemoryBackend()
	h := cloudwatchlogs.NewHandler(backend)
	client := newTestCloudWatchLogsClient(t, h)
	ctx := t.Context()

	cloudwatchlogs.AddImportTaskInternal(backend, cloudwatchlogs.ImportTask{
		ImportID:             "i1",
		ImportSourceArn:      "arn:aws:cloudtrail:us-east-1:123:eventdatastore/abc",
		ImportRoleArn:        "arn:aws:iam::123:role/my-role",
		ImportDestinationArn: "arn:aws:logs:us-east-1:123:log-group:/aws/import",
		Status:               "IN_PROGRESS",
		CreationTime:         1700000000000,
		LastUpdatedTime:      1700000001000,
	})
	cloudwatchlogs.AddImportTaskInternal(backend, cloudwatchlogs.ImportTask{
		ImportID:             "i2",
		ImportSourceArn:      "arn:aws:cloudtrail:us-east-1:123:eventdatastore/def",
		ImportDestinationArn: "arn:aws:logs:us-east-1:123:log-group:/aws/import2",
		Status:               "COMPLETED",
		CreationTime:         1700000002000,
		LastUpdatedTime:      1700000003000,
	})

	out, err := client.DescribeImportTasks(ctx, &cwlsdk.DescribeImportTasksInput{
		ImportId: aws.String("i1"),
	})
	require.NoError(t, err)
	require.Len(t, out.Imports, 1, "ImportId filter must reach the backend via the real wire key")

	got := out.Imports[0]
	assert.Equal(t, "i1", aws.ToString(got.ImportId))
	assert.Equal(t, cwltypes.ImportStatusInProgress, got.ImportStatus)
	assert.Equal(t, "arn:aws:cloudtrail:us-east-1:123:eventdatastore/abc", aws.ToString(got.ImportSourceArn))
}

func TestHandler_CancelExportTask_StateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		seedStatus string
		wantCode   int
	}{
		{
			name:       "cancel_pending_succeeds",
			seedStatus: "PENDING",
			wantCode:   http.StatusOK,
		},
		{
			name:       "cancel_running_succeeds",
			seedStatus: "RUNNING",
			wantCode:   http.StatusOK,
		},
		{
			name:       "cancel_completed_fails",
			seedStatus: "COMPLETED",
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "cancel_failed_fails",
			seedStatus: "FAILED",
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "cancel_already_cancelled_fails",
			seedStatus: "CANCELLED",
			wantCode:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := cloudwatchlogs.NewInMemoryBackend()
			taskID := "task-" + tt.name
			cloudwatchlogs.AddExportTaskInternal(backend, cloudwatchlogs.ExportTask{
				TaskID:       taskID,
				LogGroupName: "/grp",
				Destination:  "my-bucket",
				Status:       tt.seedStatus,
				From:         1000,
				To:           2000,
			})

			e := echo.New()
			h := cloudwatchlogs.NewHandler(backend)

			bodyBytes, _ := json.Marshal(map[string]any{"taskId": taskID})
			rec := doLogsRequest(t, h, e, "CancelExportTask", string(bodyBytes))
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_CancelImportTask_StateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		seedStatus string
		wantCode   int
	}{
		{
			// aws-sdk-go-v2 types.ImportStatus's in-progress member is
			// IN_PROGRESS, not ACTIVE (that value belongs to a different
			// enum, IntegrationStatus).
			name:       "cancel_in_progress_succeeds",
			seedStatus: "IN_PROGRESS",
			wantCode:   http.StatusOK,
		},
		{
			name:       "cancel_failed_task_fails",
			seedStatus: "FAILED",
			wantCode:   http.StatusBadRequest,
		},
		{
			// aws-sdk-go-v2 types.ImportStatus's terminal-success member is
			// COMPLETED, not SUCCEEDED.
			name:       "cancel_completed_task_fails",
			seedStatus: "COMPLETED",
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "cancel_already_cancelled_fails",
			seedStatus: "CANCELLED",
			wantCode:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := cloudwatchlogs.NewInMemoryBackend()
			importID := "import-" + tt.name
			cloudwatchlogs.AddImportTaskInternal(backend, cloudwatchlogs.ImportTask{
				ImportID:             importID,
				ImportSourceArn:      "arn:aws:cloudtrail:us-east-1:123:eventdatastore/abc",
				ImportRoleArn:        "arn:aws:iam::123:role/role",
				ImportDestinationArn: "arn:aws:logs:us-east-1:123:log-group:/aws/import",
				Status:               tt.seedStatus,
			})

			e := echo.New()
			h := cloudwatchlogs.NewHandler(backend)

			bodyBytes, _ := json.Marshal(map[string]any{"importId": importID})
			rec := doLogsRequest(t, h, e, "CancelImportTask", string(bodyBytes))
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_CreateExportTask_FromToValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantCode   int
		needsGroup bool
	}{
		{
			name:     "from_equal_to_fails",
			body:     `{"logGroupName":"/grp","destination":"bucket","from":1000,"to":1000}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "from_greater_than_to_fails",
			body:     `{"logGroupName":"/grp","destination":"bucket","from":2000,"to":1000}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:       "from_less_than_to_succeeds",
			body:       `{"logGroupName":"/grp","destination":"bucket","from":1000,"to":2000}`,
			wantCode:   http.StatusOK,
			needsGroup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			backend := cloudwatchlogs.NewInMemoryBackend()
			h := cloudwatchlogs.NewHandler(backend)

			if tt.needsGroup {
				setupRec := doLogsRequest(t, h, e, "CreateLogGroup", `{"logGroupName":"/grp"}`)
				require.Equal(t, http.StatusOK, setupRec.Code)
			}

			rec := doLogsRequest(t, h, e, "CreateExportTask", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_ExportImportTaskOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, h *cloudwatchlogs.Handler, e *echo.Echo)
		body     map[string]any
		name     string
		action   string
		wantKey  string
		wantVal  string
		wantCode int
	}{
		// CancelExportTask
		{
			name:     "CancelExportTask/EmptyTaskId",
			action:   "CancelExportTask",
			body:     map[string]any{"taskId": ""},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "CancelExportTask/NotFound",
			action:   "CancelExportTask",
			body:     map[string]any{"taskId": "nonexistent-task"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "CancelExportTask/MissingTaskId",
			action:   "CancelExportTask",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		// CancelImportTask
		{
			name:     "CancelImportTask/NotFound",
			action:   "CancelImportTask",
			body:     map[string]any{"importId": "nonexistent"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "CancelImportTask/MissingImportId",
			action:   "CancelImportTask",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		// CreateExportTask
		{
			name: "CreateExportTask/OK",
			setup: func(t *testing.T, h *cloudwatchlogs.Handler, e *echo.Echo) {
				t.Helper()
				rec := doLogsRequest(t, h, e, "CreateLogGroup", `{"logGroupName":"/my/group"}`)
				require.Equal(t, http.StatusOK, rec.Code)
			},
			action: "CreateExportTask",
			body: map[string]any{
				"logGroupName": "/my/group",
				"destination":  "my-bucket",
				"from":         1000,
				"to":           2000,
			},
			wantCode: http.StatusOK,
			wantKey:  "taskId",
		},
		{
			name:     "CreateExportTask/MissingLogGroup",
			action:   "CreateExportTask",
			body:     map[string]any{"destination": "my-bucket", "from": 1000, "to": 2000},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "CreateExportTask/LogGroupNotFound",
			action: "CreateExportTask",
			body: map[string]any{
				"logGroupName": "/does/not/exist",
				"destination":  "my-bucket",
				"from":         1000,
				"to":           2000,
			},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "CreateExportTask/MissingDestination",
			action:   "CreateExportTask",
			body:     map[string]any{"logGroupName": "/my/group", "from": 1000, "to": 2000},
			wantCode: http.StatusBadRequest,
		},
		// CreateImportTask
		{
			name:   "CreateImportTask/OK",
			action: "CreateImportTask",
			body: map[string]any{
				"importRoleArn":   "arn:aws:iam::123:role/my-role",
				"importSourceArn": "arn:aws:cloudtrail:us-east-1:123:eventdatastore/abc",
			},
			wantCode: http.StatusOK,
			wantKey:  "importId",
		},
		{
			name:   "CreateImportTask/MissingRoleArn",
			action: "CreateImportTask",
			body: map[string]any{
				"importSourceArn": "arn:aws:cloudtrail:us-east-1:123:eventdatastore/abc",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "CreateImportTask/MissingSourceArn",
			action:   "CreateImportTask",
			body:     map[string]any{"importRoleArn": "arn:aws:iam::123:role/my-role"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			backend := cloudwatchlogs.NewInMemoryBackend()
			h := cloudwatchlogs.NewHandler(backend)

			if tt.setup != nil {
				tt.setup(t, h, e)
			}

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			rec := doLogsRequest(t, h, e, tt.action, string(bodyBytes))
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK && tt.wantKey != "" {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				if tt.wantVal != "" {
					assert.Equal(t, tt.wantVal, out[tt.wantKey])
				} else {
					assert.NotEmpty(t, out[tt.wantKey], "expected non-empty %s", tt.wantKey)
				}
			}
		})
	}
}

func TestHandler_ImportTaskBatchesValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body          map[string]any
		name          string
		action        string
		wantListField string
		wantCode      int
	}{
		{
			// DescribeImportTaskBatches is validation-only: importId is
			// required.
			name:     "DescribeImportTaskBatches/RequiresImportID",
			action:   "DescribeImportTaskBatches",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			rec := makeLogsRequest(t, tt.action, string(bodyBytes))
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantListField != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				list, ok := resp[tt.wantListField].([]any)
				require.True(t, ok, "expected list field %q in response", tt.wantListField)
				assert.Empty(t, list)
			}
		})
	}
}

// TestHandler_DescribeImportTaskBatches_RealClient proves DescribeImportTaskBatches
// is actually reachable by a real aws-sdk-go-v2 client. Before the fix, the
// handler required a "taskId" body field that no real client ever sends (the
// real DescribeImportTaskBatchesInput serializes its filter as "importId",
// serializers.go's ...DescribeImportTaskBatchesInput case "importId":), so
// every real client call failed the handler's own required-field check
// regardless of what it sent. The response wrapper is "importBatches", not
// "importTaskBatches" (deserializers.go's
// ...DescribeImportTaskBatchesOutput case "importBatches":), and ImportId/
// ImportSourceArn are echoed alongside it.
func TestHandler_DescribeImportTaskBatches_RealClient(t *testing.T) {
	t.Parallel()

	backend := cloudwatchlogs.NewInMemoryBackend()
	h := cloudwatchlogs.NewHandler(backend)
	client := newTestCloudWatchLogsClient(t, h)
	ctx := t.Context()

	cloudwatchlogs.AddImportTaskInternal(backend, cloudwatchlogs.ImportTask{
		ImportID:        "batch-i1",
		ImportSourceArn: "arn:aws:cloudtrail:us-east-1:123:eventdatastore/abc",
		Status:          "IN_PROGRESS",
	})

	out, err := client.DescribeImportTaskBatches(ctx, &cwlsdk.DescribeImportTaskBatchesInput{
		ImportId: aws.String("batch-i1"),
	})
	require.NoError(t, err, "a real client's importId filter must reach the handler")
	assert.Equal(t, "batch-i1", aws.ToString(out.ImportId))
	assert.Equal(t,
		"arn:aws:cloudtrail:us-east-1:123:eventdatastore/abc",
		aws.ToString(out.ImportSourceArn),
	)
	assert.Empty(t, out.ImportBatches)
}
