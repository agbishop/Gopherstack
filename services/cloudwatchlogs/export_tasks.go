package cloudwatchlogs

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	exportStatusPending   = "PENDING"
	exportStatusRunning   = "RUNNING"
	exportStatusCompleted = "COMPLETED"
	exportStatusCancelled = "CANCELLED"
	exportStatusFailed    = "FAILED"
)

// importStatus* mirror the real aws-sdk-go-v2 types.ImportStatus enum exactly
// (IN_PROGRESS/CANCELLED/COMPLETED/FAILED). Earlier code used the shared
// "ACTIVE" completenessStatusActive constant (correct for Integration, whose
// status enum really does include ACTIVE) as an import task's initial
// status, which is not a member of ImportStatus at all -- a real SDK client
// would see importStatus: "ACTIVE" and fail to parse it into any known enum
// value.
const (
	importStatusInProgress = "IN_PROGRESS"
	importStatusCancelled  = "CANCELLED"
	importStatusCompleted  = "COMPLETED" // not yet emitted: no simulated import execution transitions to this state.
	importStatusFailed     = "FAILED"    // not yet emitted: no simulated import execution transitions to this state.
)

// CancelExportTask cancels a pending or running export task.
// Returns an error if the task is already in a terminal state.
func (b *InMemoryBackend) CancelExportTask(taskID string) error {
	if taskID == "" {
		return fmt.Errorf("%w: taskId is required", ErrValidation)
	}

	b.mu.Lock("CancelExportTask")
	defer b.mu.Unlock()

	task, ok := b.exportTasks.Get(taskID)
	if !ok {
		return fmt.Errorf("%w: export task %s not found", ErrExportTaskNotFound, taskID)
	}

	// AWS only allows cancellation of tasks in PENDING or RUNNING state.
	if task.Status != exportStatusPending && task.Status != exportStatusRunning {
		return fmt.Errorf("%w: export task %s is in terminal state %s and cannot be cancelled",
			ErrValidation, taskID, task.Status)
	}

	task.Status = exportStatusCancelled

	return nil
}

// CancelImportTask cancels a running import task.
// Returns an error if the task is not in the ACTIVE state.
func (b *InMemoryBackend) CancelImportTask(importID string) (*ImportTask, error) {
	if importID == "" {
		return nil, fmt.Errorf("%w: importId is required", ErrValidation)
	}

	b.mu.Lock("CancelImportTask")
	defer b.mu.Unlock()

	task, ok := b.importTasks.Get(importID)
	if !ok {
		return nil, fmt.Errorf("%w: import task %s not found", ErrImportTaskNotFound, importID)
	}

	// AWS only allows cancellation of IN_PROGRESS tasks.
	if task.Status != importStatusInProgress {
		return nil, fmt.Errorf("%w: import task %s is in state %s and cannot be cancelled",
			ErrValidation, importID, task.Status)
	}

	task.Status = importStatusCancelled
	task.LastUpdatedTime = time.Now().UnixMilli()

	cp := *task

	return &cp, nil
}

// CreateExportTask creates an export task to export log data to S3.
// Returns the task ID. When an S3 export sink is configured (see SetExportSink),
// the matching log events are written to the destination bucket as gzipped
// objects using the AWS key layout and the task completes synchronously;
// otherwise the task starts PENDING and advances by janitor age.
func (b *InMemoryBackend) CreateExportTask(
	ctx context.Context,
	taskName, logGroupName, logStreamNamePrefix, destination, destinationPrefix string,
	from, to int64,
) (string, error) {
	if logGroupName == "" {
		return "", fmt.Errorf("%w: logGroupName is required", ErrValidation)
	}

	if destination == "" {
		return "", fmt.Errorf("%w: destination is required", ErrValidation)
	}

	if from >= to {
		return "", fmt.Errorf("%w: from (%d) must be less than to (%d)", ErrValidation, from, to)
	}

	region := getRegion(ctx, b.region)

	task := &ExportTask{
		TaskID:              uuid.New().String(),
		TaskName:            taskName,
		LogGroupName:        logGroupName,
		LogStreamNamePrefix: logStreamNamePrefix,
		Destination:         destination,
		DestinationPrefix:   destinationPrefix,
		StatusMessage:       "",
		From:                from,
		To:                  to,
		Status:              exportStatusPending,
		CreationTime:        time.Now().UnixMilli(),
		CompletionTime:      0,
		Region:              region,
	}

	var limitErr error
	var sink ExportSink
	func() {
		b.mu.Lock("CreateExportTask")
		defer b.mu.Unlock()

		if !b.groupHas(region, logGroupName) {
			limitErr = fmt.Errorf("%w: Log group %s not found", ErrLogGroupNotFound, logGroupName)

			return
		}

		if b.exportTasks.Len() >= maxExportTasks {
			limitErr = fmt.Errorf("%w: export task limit exceeded", ErrValidation)

			return
		}

		b.exportTasks.Put(task)
		sink = b.exportSink
	}()
	if limitErr != nil {
		return "", limitErr
	}

	if sink != nil {
		b.finishExport(task)
	}

	return task.TaskID, nil
}

// finishExport materialises the export to S3 and records the terminal status.
func (b *InMemoryBackend) finishExport(task *ExportTask) {
	count, err := b.runExport(b.ctx, task)

	b.mu.Lock("finishExport")
	defer b.mu.Unlock()

	stored, ok := b.exportTasks.Get(task.TaskID)
	if !ok {
		return
	}

	stored.CompletionTime = time.Now().UnixMilli()
	if err != nil {
		stored.Status = exportStatusFailed
		stored.StatusMessage = err.Error()

		return
	}

	stored.Status = exportStatusCompleted
	stored.StatusMessage = fmt.Sprintf("Completed successfully. Exported %d log events.", count)
}

// CreateImportTask creates an import task from a CloudTrail Lake event data store.
func (b *InMemoryBackend) CreateImportTask(
	ctx context.Context,
	importRoleArn, importSourceArn string,
) (*ImportTask, error) {
	if importRoleArn == "" {
		return nil, fmt.Errorf("%w: importRoleArn is required", ErrValidation)
	}

	if importSourceArn == "" {
		return nil, fmt.Errorf("%w: importSourceArn is required", ErrValidation)
	}

	region := getRegion(ctx, b.region)
	importID := uuid.New().String()
	now := time.Now().UnixMilli()
	destARN := arn.Build("logs", region, b.accountID, "log-group:/aws/cloudtrail/"+importID)

	task := &ImportTask{
		ImportID:             importID,
		ImportSourceArn:      importSourceArn,
		ImportRoleArn:        importRoleArn,
		ImportDestinationArn: destARN,
		Status:               importStatusInProgress,
		CreationTime:         now,
		LastUpdatedTime:      now,
	}

	b.mu.Lock("CreateImportTask")
	defer b.mu.Unlock()

	if b.importTasks.Len() >= maxImportTasks {
		return nil, fmt.Errorf("%w: import task limit exceeded", ErrValidation)
	}

	b.importTasks.Put(task)

	cp := *task

	return &cp, nil
}

// advanceExportTaskStatesLocked lazily advances every export task's state
// from PENDING→RUNNING→COMPLETED based on elapsed time. Caller must hold b.mu.
func (b *InMemoryBackend) advanceExportTaskStatesLocked() {
	now := time.Now().UnixMilli()
	for _, t := range b.exportTasks.All() {
		age := now - t.CreationTime
		if age > maxExportTaskAgeMs {
			continue
		}
		if t.Status == exportStatusPending && age > exportTaskAgeRunningMs {
			t.Status = exportStatusRunning
		}
		if t.Status == exportStatusRunning && age > exportTaskAgeCompletedMs {
			t.Status = exportStatusCompleted
		}
	}
}

// DescribeExportTasks lists export tasks optionally filtered by task ID or status.
// It also lazily advances task state from PENDING→RUNNING→COMPLETED based on elapsed time.
func (b *InMemoryBackend) DescribeExportTasks(
	taskID, statusCode string,
	limit int,
	nextToken string,
) ([]ExportTask, string, error) {
	b.mu.Lock("DescribeExportTasks")
	defer b.mu.Unlock()

	b.advanceExportTaskStatesLocked()

	all := make([]ExportTask, 0, b.exportTasks.Len())
	for _, t := range b.exportTasks.All() {
		if taskID != "" && t.TaskID != taskID {
			continue
		}
		if statusCode != "" && t.Status != statusCode {
			continue
		}
		all = append(all, *t)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreationTime != all[j].CreationTime {
			return all[i].CreationTime < all[j].CreationTime
		}

		return all[i].TaskID < all[j].TaskID
	})

	startIdx := parseNextToken(nextToken)
	if startIdx >= len(all) {
		return []ExportTask{}, "", nil
	}
	if limit <= 0 {
		limit = defaultDescribeLimit
	}
	end := startIdx + limit
	var outToken string
	if end < len(all) {
		outToken = encodeNextToken(end)
	} else {
		end = len(all)
	}

	return all[startIdx:end], outToken, nil
}

// DescribeImportTasks lists import tasks optionally filtered by task ID.
func (b *InMemoryBackend) DescribeImportTasks(
	taskID string,
	limit int,
	nextToken string,
) ([]ImportTask, string, error) {
	b.mu.RLock("DescribeImportTasks")
	defer b.mu.RUnlock()

	all := make([]ImportTask, 0, b.importTasks.Len())
	for _, t := range b.importTasks.All() {
		if taskID != "" && t.ImportID != taskID {
			continue
		}
		all = append(all, *t)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreationTime != all[j].CreationTime {
			return all[i].CreationTime < all[j].CreationTime
		}

		return all[i].ImportID < all[j].ImportID
	})

	startIdx := parseNextToken(nextToken)
	if startIdx >= len(all) {
		return []ImportTask{}, "", nil
	}
	if limit <= 0 {
		limit = defaultDescribeLimit
	}
	end := startIdx + limit
	var outToken string
	if end < len(all) {
		outToken = encodeNextToken(end)
	} else {
		end = len(all)
	}

	return all[startIdx:end], outToken, nil
}

// AddExportTaskInternal seeds an ExportTask directly into the store for testing.
// It overwrites any existing task with the same ID.
func (b *InMemoryBackend) AddExportTaskInternal(task ExportTask) {
	b.mu.Lock("AddExportTaskInternal")
	defer b.mu.Unlock()

	t := task
	b.exportTasks.Put(&t)
}

// AddImportTaskInternal seeds an ImportTask directly into the store for testing.
// It overwrites any existing task with the same ID.
func (b *InMemoryBackend) AddImportTaskInternal(task ImportTask) {
	b.mu.Lock("AddImportTaskInternal")
	defer b.mu.Unlock()

	t := task
	b.importTasks.Put(&t)
}
