package timestreamwrite

import (
	"fmt"
	"sort"
	"time"
)

const (
	// BatchLoadStatusCreated indicates a task has been created and is pending execution.
	BatchLoadStatusCreated = "CREATED"
	// BatchLoadStatusInProgress indicates a task is currently loading data.
	BatchLoadStatusInProgress = "IN_PROGRESS"
	// BatchLoadStatusFailed indicates a task has failed.
	BatchLoadStatusFailed = "FAILED"
	// BatchLoadStatusSucceeded indicates a task completed successfully.
	BatchLoadStatusSucceeded = "SUCCEEDED"
	// BatchLoadStatusProgressStopped indicates a task was stopped before completion.
	BatchLoadStatusProgressStopped = "PROGRESS_STOPPED"
	// BatchLoadStatusPendingResume indicates a task is pending a resume operation.
	BatchLoadStatusPendingResume = "PENDING_RESUME"
)

// CreateBatchLoadTask creates a new batch load task targeting the specified database and table.
func (b *InMemoryBackend) CreateBatchLoadTask(
	targetDatabase, targetTable string,
	dataSourceCfg *DataSourceConfiguration,
	reportCfg *ReportConfiguration,
	dataModelCfg *DataModelConfiguration,
	recordVersion int64,
) (*BatchLoadTask, error) {
	b.mu.Lock("CreateBatchLoadTask")
	defer b.mu.Unlock()

	if !b.databases.Has(targetDatabase) {
		return nil, fmt.Errorf("%w: database %s not found", ErrDatabaseNotFound, targetDatabase)
	}

	if !b.tables.Has(tableKey(targetDatabase, targetTable)) {
		return nil, fmt.Errorf("%w: table %s not found", ErrTableNotFound, targetTable)
	}

	b.nextTaskID++
	taskID := fmt.Sprintf("batch-load-task-%d", b.nextTaskID)

	now := time.Now()
	task := &BatchLoadTask{
		TaskID:                  taskID,
		TargetDatabaseName:      targetDatabase,
		TargetTableName:         targetTable,
		TaskStatus:              BatchLoadStatusCreated,
		CreationTime:            now,
		LastUpdatedTime:         now,
		DataSourceConfiguration: dataSourceCfg,
		ReportConfiguration:     reportCfg,
		DataModelConfiguration:  dataModelCfg,
		RecordVersion:           recordVersion,
	}
	b.batchLoadTasks.Put(task)

	cp := *task

	return &cp, nil
}

// DescribeBatchLoadTask returns information about a batch load task.
func (b *InMemoryBackend) DescribeBatchLoadTask(taskID string) (*BatchLoadTask, error) {
	b.mu.RLock("DescribeBatchLoadTask")
	defer b.mu.RUnlock()

	task, ok := b.batchLoadTasks.Get(taskID)
	if !ok {
		return nil, fmt.Errorf("%w: batch load task %s not found", ErrBatchLoadTaskNotFound, taskID)
	}

	cp := *task

	return &cp, nil
}

// ListBatchLoadTasks returns all batch load tasks, optionally filtered by status.
// Results are sorted by creation time (oldest first).
func (b *InMemoryBackend) ListBatchLoadTasks(statusFilter string) []BatchLoadTask {
	b.mu.RLock("ListBatchLoadTasks")
	defer b.mu.RUnlock()

	out := make([]BatchLoadTask, 0, b.batchLoadTasks.Len())

	for _, task := range b.batchLoadTasks.All() {
		if statusFilter != "" && task.TaskStatus != statusFilter {
			continue
		}

		cp := *task
		out = append(out, cp)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CreationTime.Before(out[j].CreationTime)
	})

	return out
}

// ResumeBatchLoadTask resumes a batch load task that is in PROGRESS_STOPPED or FAILED status.
// Pinned SDK documents no precondition (gopherstack-3geb); matches the guard below.
func (b *InMemoryBackend) ResumeBatchLoadTask(taskID string) error {
	b.mu.Lock("ResumeBatchLoadTask")
	defer b.mu.Unlock()

	task, ok := b.batchLoadTasks.Get(taskID)
	if !ok {
		return fmt.Errorf("%w: batch load task %s not found", ErrBatchLoadTaskNotFound, taskID)
	}

	if task.TaskStatus != BatchLoadStatusProgressStopped && task.TaskStatus != BatchLoadStatusFailed {
		return fmt.Errorf(
			"%w: task %s cannot be resumed from status %s",
			ErrInvalidBatchLoadStatus,
			taskID,
			task.TaskStatus,
		)
	}

	task.TaskStatus = BatchLoadStatusCreated
	task.LastUpdatedTime = time.Now()

	return nil
}

// SetBatchLoadTaskStatus sets the status of a batch load task.
// This is a test seed helper to set specific task states.
func (b *InMemoryBackend) SetBatchLoadTaskStatus(taskID, status string) error {
	b.mu.Lock("SetBatchLoadTaskStatus")
	defer b.mu.Unlock()

	task, ok := b.batchLoadTasks.Get(taskID)
	if !ok {
		return fmt.Errorf("%w: batch load task %s not found", ErrBatchLoadTaskNotFound, taskID)
	}

	task.TaskStatus = status
	task.LastUpdatedTime = time.Now()

	return nil
}

// AddBatchLoadTaskInternal directly inserts a batch load task, bypassing
// validation. Intended only for test setup.
func (b *InMemoryBackend) AddBatchLoadTaskInternal(task *BatchLoadTask) {
	b.mu.Lock("AddBatchLoadTaskInternal")
	defer b.mu.Unlock()

	cp := *task
	b.batchLoadTasks.Put(&cp)
}
