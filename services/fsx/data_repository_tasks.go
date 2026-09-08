package fsx

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

type storedDataRepositoryTask struct {
	CreationTime time.Time         `json:"creationTime"`
	Report       *CompletionReport `json:"report,omitempty"`
	Tags         map[string]string `json:"tags"`
	TaskID       string            `json:"taskId"`
	FileSystemID string            `json:"fileSystemId"`
	Type         string            `json:"type"`
	Lifecycle    string            `json:"lifecycle"`
	ResourceARN  string            `json:"resourceArn"`
	Paths        []string          `json:"paths,omitempty"`
}

func (t *storedDataRepositoryTask) toPublic() *DataRepositoryTask {
	return &DataRepositoryTask{
		CreationTime: epochTime(t.CreationTime),
		Report:       t.Report,
		TaskID:       t.TaskID,
		FileSystemID: t.FileSystemID,
		Type:         t.Type,
		Lifecycle:    t.Lifecycle,
		ResourceARN:  t.ResourceARN,
		Paths:        t.Paths,
		Tags:         tagsMapToSlice(t.Tags),
	}
}

type createDataRepositoryTaskInput struct {
	Report       *CompletionReport `json:"Report"`
	FileSystemID string            `json:"FileSystemId"`
	Type         string            `json:"Type"`
	Paths        []string          `json:"Paths,omitempty"`
	Tags         []Tag             `json:"Tags,omitempty"`
}

// CreateDataRepositoryTask creates a data repository task. Report is a
// required CreateDataRepositoryTaskInput member (verified against
// validateOpCreateDataRepositoryTaskInput, validators.go), and its own
// Enabled member is required whenever Report is present (validateCompletionReport)
// -- the pre-fix request never read Report at all.
func (b *InMemoryBackend) CreateDataRepositoryTask(input *createDataRepositoryTaskInput) (*DataRepositoryTask, error) {
	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	if input.Report == nil {
		return nil, fmt.Errorf("%w: Report is required", ErrValidation)
	}

	if input.Report.Enabled == nil {
		return nil, fmt.Errorf("%w: Report.Enabled is required", ErrValidation)
	}

	b.mu.Lock("CreateDataRepositoryTask")
	defer b.mu.Unlock()

	if !b.fileSystems.Has(input.FileSystemID) {
		return nil, ErrFileSystemNotFound
	}

	if b.hasExecutingTaskLocked(input.FileSystemID) {
		return nil, ErrDataRepositoryTaskExecuting
	}

	id := newDataRepositoryTaskID()
	arn := b.drtARN(id)
	now := time.Now().UTC()
	tags := tagsSliceToMap(input.Tags)

	t := &storedDataRepositoryTask{
		CreationTime: now,
		Report:       input.Report,
		Tags:         tags,
		Paths:        input.Paths,
		TaskID:       id,
		FileSystemID: input.FileSystemID,
		Type:         input.Type,
		Lifecycle:    "EXECUTING",
		ResourceARN:  arn,
	}

	b.dataRepositoryTasks.Put(t)
	b.tags[arn] = tags

	return t.toPublic(), nil
}

// hasExecutingTaskLocked reports whether fileSystemID already has a task
// with Lifecycle EXECUTING. Caller must already hold b.mu.
func (b *InMemoryBackend) hasExecutingTaskLocked(fileSystemID string) bool {
	found := false

	b.dataRepositoryTasks.Range(func(t *storedDataRepositoryTask) bool {
		if t.FileSystemID == fileSystemID && t.Lifecycle == "EXECUTING" {
			found = true

			return false
		}

		return true
	})

	return found
}

// CancelDataRepositoryTask marks a task as cancelled.
func (b *InMemoryBackend) CancelDataRepositoryTask(taskID string) error {
	b.mu.Lock("CancelDataRepositoryTask")
	defer b.mu.Unlock()

	t, ok := b.dataRepositoryTasks.Get(taskID)
	if !ok {
		return ErrDataRepositoryTaskNotFound
	}

	t.Lifecycle = "CANCELING"

	return nil
}

// DescribeDataRepositoryTasks returns tasks, optionally filtered by ID or
// Filters. Real DataRepositoryTaskFilterName (aws-sdk-go-v2/service/fsx@v1.68.4
// types/enums.go) has 4 values: file-system-id, task-lifecycle,
// data-repository-association-id, file-cache-id. Only the first two are
// recognized here -- CreateDataRepositoryTask never accepts an association or
// file-cache reference to track, so those two have no honest value; matches
// everything for them, same as an unset filter.
func (b *InMemoryBackend) DescribeDataRepositoryTasks( //nolint:dupl // existing issue.
	ids []string,
	filters []wireFilter,
	maxResults int32,
	nextToken string,
) ([]*DataRepositoryTask, string, error) {
	b.mu.RLock("DescribeDataRepositoryTasks")
	defer b.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = maxResultsDefault
	}

	var all []*storedDataRepositoryTask

	if len(ids) > 0 {
		for _, id := range ids {
			t, ok := b.dataRepositoryTasks.Get(id)
			if !ok {
				return nil, "", ErrDataRepositoryTaskNotFound
			}

			all = append(all, t)
		}
	} else {
		for _, t := range b.dataRepositoryTasks.All() {
			if matchesFilters(filters, func(name string) (string, bool) {
				switch name {
				case filterNameFileSystemID:
					return t.FileSystemID, true
				case "task-lifecycle":
					return t.Lifecycle, true
				default:
					return "", false
				}
			}) {
				all = append(all, t)
			}
		}

		sort.Slice(all, func(i, j int) bool { return all[i].TaskID < all[j].TaskID })
	}

	start, end, next := paginate(len(all), int(maxResults), nextToken, func(i int) string {
		return all[i].TaskID
	})

	result := make([]*DataRepositoryTask, end-start)
	for i, t := range all[start:end] {
		result[i] = t.toPublic()
	}

	return result, next, nil
}

func (b *InMemoryBackend) drtARN(id string) string {
	return arn.Build("fsx", b.region, b.accountID, fmt.Sprintf("task/%s", id))
}
