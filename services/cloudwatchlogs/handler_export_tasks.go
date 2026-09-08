package cloudwatchlogs

import (
	"context"
	"encoding/json"
	"fmt"
)

type cancelExportTaskInput struct {
	TaskID string `json:"taskId"`
}

type cancelExportTaskOutput struct{}

type cancelImportTaskInput struct {
	ImportID string `json:"importId"`
}

type cancelImportTaskOutput struct {
	CreationTime    *int64 `json:"creationTime,omitempty"`
	LastUpdatedTime *int64 `json:"lastUpdatedTime,omitempty"`
	ImportID        string `json:"importId,omitempty"`
	ImportStatus    string `json:"importStatus,omitempty"`
}

type createExportTaskInput struct {
	TaskName            string `json:"taskName"`
	LogGroupName        string `json:"logGroupName"`
	LogStreamNamePrefix string `json:"logStreamNamePrefix"`
	Destination         string `json:"destination"`
	DestinationPrefix   string `json:"destinationPrefix"`
	From                int64  `json:"from"`
	To                  int64  `json:"to"`
}

type createExportTaskOutput struct {
	TaskID string `json:"taskId,omitempty"`
}

type createImportTaskInput struct {
	ImportRoleArn   string `json:"importRoleArn"`
	ImportSourceArn string `json:"importSourceArn"`
}

type createImportTaskOutput struct {
	CreationTime         *int64 `json:"creationTime,omitempty"`
	ImportID             string `json:"importId,omitempty"`
	ImportDestinationArn string `json:"importDestinationArn,omitempty"`
}

// --- DescribeExportTasks ---.
type describeExportTasksInput struct {
	TaskID     string `json:"taskId"`
	StatusCode string `json:"statusCode"`
	NextToken  string `json:"nextToken"`
	Limit      int    `json:"limit"`
}

type describeExportTasksOutput struct {
	NextToken   string           `json:"nextToken,omitempty"`
	ExportTasks []wireExportTask `json:"exportTasks"`
}

// wireExportTaskStatus is the nested object aws-sdk-go-v2 types.ExportTaskStatus
// serializes to on the wire: {"code": ..., "message": ...}, not a bare string.
type wireExportTaskStatus struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// wireExportTaskExecutionInfo is the nested object aws-sdk-go-v2
// types.ExportTaskExecutionInfo serializes to on the wire:
// {"creationTime": ..., "completionTime": ...}, not two flat top-level fields.
type wireExportTaskExecutionInfo struct {
	CreationTime   int64 `json:"creationTime,omitempty"`
	CompletionTime int64 `json:"completionTime,omitempty"`
}

// wireExportTask is the AWS wire shape for one entry in DescribeExportTasks's
// exportTasks array (aws-sdk-go-v2 types.ExportTask). This package's internal
// ExportTask (models.go) keeps Status/StatusMessage/CreationTime/CompletionTime
// as flat fields for simplicity of backend state mutation; toWireExportTask
// nests them correctly for the wire. A real SDK client unmarshalling the flat
// shape would see Status and ExecutionInfo as nil, since it expects nested
// objects under those keys, not scalars.
type wireExportTask struct {
	Status              *wireExportTaskStatus        `json:"status,omitempty"`
	ExecutionInfo       *wireExportTaskExecutionInfo `json:"executionInfo,omitempty"`
	TaskID              string                       `json:"taskId,omitempty"`
	TaskName            string                       `json:"taskName,omitempty"`
	LogGroupName        string                       `json:"logGroupName,omitempty"`
	LogStreamNamePrefix string                       `json:"logStreamNamePrefix,omitempty"`
	Destination         string                       `json:"destination,omitempty"`
	DestinationPrefix   string                       `json:"destinationPrefix,omitempty"`
	From                int64                        `json:"from,omitempty"`
	To                  int64                        `json:"to,omitempty"`
}

// toWireExportTask maps the internal flat ExportTask model to the nested AWS
// wire shape (see wireExportTask doc comment).
func toWireExportTask(t ExportTask) wireExportTask {
	w := wireExportTask{
		TaskID:              t.TaskID,
		TaskName:            t.TaskName,
		LogGroupName:        t.LogGroupName,
		LogStreamNamePrefix: t.LogStreamNamePrefix,
		Destination:         t.Destination,
		DestinationPrefix:   t.DestinationPrefix,
		From:                t.From,
		To:                  t.To,
	}

	if t.Status != "" {
		w.Status = &wireExportTaskStatus{Code: t.Status, Message: t.StatusMessage}
	}

	if t.CreationTime != 0 || t.CompletionTime != 0 {
		w.ExecutionInfo = &wireExportTaskExecutionInfo{
			CreationTime:   t.CreationTime,
			CompletionTime: t.CompletionTime,
		}
	}

	return w
}

// --- DescribeImportTasks ---.
// ImportId/"imports" are the real DescribeImportTasksInput/Output wire keys
// (deserializers.go's awsAwsjson11_deserializeOpDocumentDescribeImportTasksOutput
// case "imports":, serializers.go's ...DescribeImportTasksInput case "importId":).
// A previous revision used "taskId"/"importTasks" -- ExportTask's own
// convention, copied onto Import by mistake -- so a real client's ImportId
// filter was silently ignored and its typed Imports field was always empty.
type describeImportTasksInput struct {
	ImportID  string `json:"importId"`
	NextToken string `json:"nextToken"`
	Limit     int    `json:"limit"`
}

type describeImportTasksOutput struct {
	NextToken string       `json:"nextToken,omitempty"`
	Imports   []ImportTask `json:"imports"`
}

func (h *Handler) handleCancelExportTask(
	ctx context.Context, //nolint:revive // existing issue.
	b []byte,
) (any, error) {
	var input cancelExportTaskInput
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}
	if err := h.Backend.CancelExportTask(input.TaskID); err != nil {
		return nil, err
	}

	return &cancelExportTaskOutput{}, nil
}

func (h *Handler) handleCancelImportTask(
	ctx context.Context, //nolint:revive // existing issue.
	b []byte,
) (any, error) {
	var input cancelImportTaskInput
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}

	task, err := h.Backend.CancelImportTask(input.ImportID)
	if err != nil {
		return nil, err
	}

	return &cancelImportTaskOutput{
		ImportID:        task.ImportID,
		ImportStatus:    task.Status,
		CreationTime:    &task.CreationTime,
		LastUpdatedTime: &task.LastUpdatedTime,
	}, nil
}

func (h *Handler) handleCreateExportTask(
	ctx context.Context,
	b []byte,
) (any, error) {
	var input createExportTaskInput
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}

	taskID, err := h.Backend.CreateExportTask(
		ctx,
		input.TaskName, input.LogGroupName, input.LogStreamNamePrefix,
		input.Destination, input.DestinationPrefix, input.From, input.To,
	)
	if err != nil {
		return nil, err
	}

	return &createExportTaskOutput{TaskID: taskID}, nil
}

func (h *Handler) handleCreateImportTask(
	ctx context.Context,
	b []byte,
) (any, error) {
	var input createImportTaskInput
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}

	task, err := h.Backend.CreateImportTask(ctx, input.ImportRoleArn, input.ImportSourceArn)
	if err != nil {
		return nil, err
	}

	return &createImportTaskOutput{
		ImportID:             task.ImportID,
		ImportDestinationArn: task.ImportDestinationArn,
		CreationTime:         &task.CreationTime,
	}, nil
}

func (h *Handler) handleDescribeExportTasks(
	ctx context.Context, //nolint:revive // existing issue.
	b []byte,
) (any, error) {
	var input describeExportTasksInput
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}
	tasks, next, err := h.Backend.DescribeExportTasks(input.TaskID, input.StatusCode, input.Limit, input.NextToken)
	if err != nil {
		return nil, err
	}

	wireTasks := make([]wireExportTask, len(tasks))
	for i, t := range tasks {
		wireTasks[i] = toWireExportTask(t)
	}

	return &describeExportTasksOutput{ExportTasks: wireTasks, NextToken: next}, nil
}

func (h *Handler) handleDescribeImportTasks(
	ctx context.Context, //nolint:revive // existing issue.
	b []byte,
) (any, error) {
	var input describeImportTasksInput
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}
	tasks, next, err := h.Backend.DescribeImportTasks(input.ImportID, input.Limit, input.NextToken)
	if err != nil {
		return nil, err
	}

	return &describeImportTasksOutput{Imports: tasks, NextToken: next}, nil
}

// handleDescribeImportTaskBatches validates the request and returns an
// empty-but-valid response. The backend tracks import tasks (DescribeImportTasks)
// but does not model per-task import batches, so the list itself is
// validation-only: the import identifier is required and, when supplied, must
// reference a known import task; otherwise an empty importBatches list is
// returned. importId/importBatches are the real DescribeImportTaskBatchesInput/
// Output wire keys (serializers.go/deserializers.go's
// ...DescribeImportTaskBatches{Input,Output} case "importId":/"importBatches":)
// -- a previous revision used "taskId"/"importTaskBatches", so every real SDK
// client request failed this handler's own required-field check regardless of
// what it sent, and a structurally-populated response would still have gone
// unread.
func (h *Handler) handleDescribeImportTaskBatches(
	ctx context.Context, //nolint:revive // existing issue.
	body []byte,
) (any, error) {
	var input struct {
		ImportID string `json:"importId"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &input); err != nil {
			return nil, err
		}
	}

	if input.ImportID == "" {
		return nil, fmt.Errorf("%w: importId is required", ErrValidation)
	}

	resp := map[string]any{"importBatches": []any{}, "importId": input.ImportID}

	if b := cwlBackend(h); b != nil {
		tasks, _, err := b.DescribeImportTasks(input.ImportID, 1, "")
		if err != nil {
			return nil, err
		}
		if len(tasks) == 0 {
			return nil, fmt.Errorf("%w: import task %s not found", ErrImportTaskNotFound, input.ImportID)
		}
		if tasks[0].ImportSourceArn != "" {
			resp["importSourceArn"] = tasks[0].ImportSourceArn
		}
	}

	return resp, nil
}
