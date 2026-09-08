package datasync

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// CreateTask creates a new DataSync task.
func (b *InMemoryBackend) CreateTask(
	sourceLocationArn, destinationLocationArn, name, cloudWatchLogGroupArn string,
	settings TaskSettings,
	tags map[string]string,
) (*Task, error) {
	b.mu.Lock("CreateTask")
	defer b.mu.Unlock()

	if !b.locations.Has(sourceLocationArn) {
		return nil, fmt.Errorf("source location %s not found: %w", sourceLocationArn, ErrInvalidParameter)
	}

	if !b.locations.Has(destinationLocationArn) {
		return nil, fmt.Errorf("destination location %s not found: %w", destinationLocationArn, ErrInvalidParameter)
	}

	id := newID()
	taskArn := b.taskARN(id)
	now := time.Now().UTC()

	taskTags := make(map[string]string)
	maps.Copy(taskTags, tags)

	taskMode := settings.TaskMode
	if taskMode == "" {
		taskMode = taskModeBasic
	}

	t := &storedTask{
		TaskArn:                taskArn,
		Name:                   name,
		Status:                 taskStatusAvailable,
		SourceLocationArn:      sourceLocationArn,
		DestinationLocationArn: destinationLocationArn,
		CloudWatchLogGroupArn:  cloudWatchLogGroupArn,
		CreationTime:           now,
		Tags:                   taskTags,
		Options:                settings.Options,
		ManifestConfig:         settings.ManifestConfig,
		TaskReportConfig:       settings.TaskReportConfig,
		Excludes:               toStoredFilterRules(settings.Excludes),
		Includes:               toStoredFilterRules(settings.Includes),
		TaskMode:               taskMode,
	}
	if settings.Schedule != nil {
		t.Schedule = &storedTaskSchedule{
			ScheduleExpression: settings.Schedule.ScheduleExpression,
			Status:             settings.Schedule.Status,
		}
	}

	b.tasks.Put(t)

	if len(taskTags) > 0 {
		b.tags[taskArn] = make(map[string]string)
		maps.Copy(b.tags[taskArn], taskTags)
	}

	cp := t.toTask()

	return &cp, nil
}

// DescribeTask returns task details.
func (b *InMemoryBackend) DescribeTask(taskArn string) (*Task, error) {
	b.mu.RLock("DescribeTask")
	defer b.mu.RUnlock()

	t, ok := b.tasks.Get(taskArn)
	if !ok {
		return nil, ErrNotFound
	}

	cp := t.toTask()

	return &cp, nil
}

// UpdateTask updates a task's mutable settings. AWS's UpdateTaskInput has no
// TaskArn-only "clear the log group" convention like some other fields, so
// CloudWatchLogGroupArn is always assigned (empty clears it, matching prior
// behavior). Options/Excludes/Includes/Schedule/ManifestConfig/
// TaskReportConfig follow AWS's "only supplied fields change" semantics: a
// nil field in settings means "not supplied, leave unchanged"; a non-nil
// (possibly empty) field means "set to this value" -- which also covers the
// documented "specify this parameter as empty to remove" behavior for
// ManifestConfig/TaskReportConfig, since an explicit empty map is non-nil.
func (b *InMemoryBackend) UpdateTask(taskArn, name, cloudWatchLogGroupArn string, settings TaskSettings) error {
	b.mu.Lock("UpdateTask")
	defer b.mu.Unlock()

	t, ok := b.tasks.Get(taskArn)
	if !ok {
		return ErrNotFound
	}

	if name != "" {
		t.Name = name
	}

	t.CloudWatchLogGroupArn = cloudWatchLogGroupArn

	applyTaskSettings(t, settings)

	return nil
}

// applyTaskSettings merges TaskSettings onto a stored task, following AWS's
// "only supplied fields change" semantics for UpdateTask (see UpdateTask doc
// comment).
func applyTaskSettings(t *storedTask, settings TaskSettings) {
	if settings.Options != nil {
		t.Options = settings.Options
	}

	if settings.Excludes != nil {
		t.Excludes = toStoredFilterRules(settings.Excludes)
	}

	if settings.Includes != nil {
		t.Includes = toStoredFilterRules(settings.Includes)
	}

	if settings.ManifestConfig != nil {
		t.ManifestConfig = settings.ManifestConfig
	}

	if settings.TaskReportConfig != nil {
		t.TaskReportConfig = settings.TaskReportConfig
	}

	if settings.Schedule != nil {
		t.Schedule = &storedTaskSchedule{
			ScheduleExpression: settings.Schedule.ScheduleExpression,
			Status:             settings.Schedule.Status,
		}
	}

	if settings.TaskMode != "" {
		t.TaskMode = settings.TaskMode
	}
}

// DeleteTask deletes a task.
func (b *InMemoryBackend) DeleteTask(taskArn string) error {
	b.mu.Lock("DeleteTask")
	defer b.mu.Unlock()

	if !b.tasks.Has(taskArn) {
		return ErrNotFound
	}

	b.tasks.Delete(taskArn)

	execs := slices.Clone(b.executionsByTask.Get(taskArn))
	for _, e := range execs {
		b.executions.Delete(e.TaskExecutionArn)
	}

	delete(b.tags, taskArn)

	return nil
}

// ListTasks returns tasks, sorted by ARN.
func (b *InMemoryBackend) ListTasks(
	filters []TaskFilter,
	maxResults int32,
	nextToken string,
) ([]*TaskListEntry, string, error) {
	b.mu.RLock("ListTasks")
	defer b.mu.RUnlock()

	sorted := b.tasks.Snapshot()

	all := make([]*TaskListEntry, 0, len(sorted))
	for _, t := range sorted {
		matched, err := matchTaskFilters(t, filters)
		if err != nil {
			return nil, "", err
		}

		if !matched {
			continue
		}

		all = append(all, &TaskListEntry{
			TaskArn:  t.TaskArn,
			Name:     t.Name,
			Status:   t.Status,
			TaskMode: t.TaskMode,
		})
	}

	limit := int(maxResults)
	pg := page.New(all, nextToken, limit, defaultMaxResults)

	return pg.Data, pg.Next, nil
}

// matchTaskFilters reports whether t satisfies every filter (AND across
// filters, per the shared AWS list-filter convention). LocationId matches
// against either the task's source or destination location ARN -- AWS's own
// doc example ("retrieve all tasks on a specific source location") names
// only source, but a task's location membership is naturally either side.
func matchTaskFilters(t *storedTask, filters []TaskFilter) (bool, error) {
	for _, f := range filters {
		var matched bool

		var err error

		switch f.Name {
		case "LocationId":
			var srcMatch, dstMatch bool

			srcMatch, err = matchFilterOperator(f.Operator, t.SourceLocationArn, f.Values)
			if err == nil {
				dstMatch, err = matchFilterOperator(f.Operator, t.DestinationLocationArn, f.Values)
			}

			matched = srcMatch || dstMatch
		case "CreationTime":
			matched, err = matchFilterOperator(f.Operator, t.CreationTime.UTC().Format(time.RFC3339), f.Values)
		default:
			return false, fmt.Errorf("%w: unrecognized filter Name %q", ErrInvalidParameter, f.Name)
		}

		if err != nil {
			return false, err
		}

		if !matched {
			return false, nil
		}
	}

	return true, nil
}

// isTerminalExecutionStatus reports whether a task execution status is a
// terminal (finished) state. AWS only allows one task execution in progress
// per task at a time, so StartTaskExecution consults this to decide whether
// the task's current execution has actually finished.
func isTerminalExecutionStatus(status string) bool {
	return status == executionStatusSuccess || status == executionStatusError
}

// StartTaskExecution starts a new task execution.
//
// AWS allows only one task execution in progress per task at a time
// ("For each task, you can only run one task execution at a time.", per the
// StartTaskExecution API docs); attempting to start another while one is
// still running is rejected as an invalid request.
func (b *InMemoryBackend) StartTaskExecution(
	taskArn string,
	overrides TaskExecutionOverrides,
	tags map[string]string,
) (*TaskExecution, error) {
	b.mu.Lock("StartTaskExecution")
	defer b.mu.Unlock()

	t, ok := b.tasks.Get(taskArn)
	if !ok {
		return nil, ErrNotFound
	}

	if t.CurrentTaskExecutionArn != "" {
		if cur, found := b.executions.Get(t.CurrentTaskExecutionArn); found && !isTerminalExecutionStatus(cur.Status) {
			return nil, fmt.Errorf(
				"%w: task %s already has an execution in progress (%s)",
				ErrInvalidParameter, taskArn, t.CurrentTaskExecutionArn,
			)
		}
	}

	id := newID()
	execArn := b.executionARN(taskArn, id)
	now := time.Now().UTC()

	e := &storedTaskExecution{
		TaskExecutionArn: execArn,
		Status:           executionStatusLaunching,
		TaskMode:         t.TaskMode,
		StartTime:        now,
		Options:          overrides.Options,
		ManifestConfig:   overrides.ManifestConfig,
		TaskReportConfig: overrides.TaskReportConfig,
		Excludes:         toStoredFilterRules(overrides.Excludes),
		Includes:         toStoredFilterRules(overrides.Includes),
	}

	b.executions.Put(e)
	t.CurrentTaskExecutionArn = execArn
	t.Status = taskStatusRunning

	if len(tags) > 0 {
		b.tags[execArn] = make(map[string]string, len(tags))
		maps.Copy(b.tags[execArn], tags)
	}

	cp := e.toTaskExecution()

	return &cp, nil
}

// CancelTaskExecution cancels a running task execution.
//
// AWS has no CANCELLED task-execution status (see types.TaskExecutionStatus);
// a cancelled execution settles into ERROR, matching the documented
// "some transient states... interrupt a task execution" behavior. Unlike a
// truly "current" (actively running) marker, CurrentTaskExecutionArn on the
// parent task is documented as "the ARN of the most recent task execution",
// so it is left pointing at the cancelled execution rather than cleared.
func (b *InMemoryBackend) CancelTaskExecution(taskExecutionArn string) error {
	b.mu.Lock("CancelTaskExecution")
	defer b.mu.Unlock()

	taskArn := extractTaskArnFromExecution(taskExecutionArn)
	if taskArn == "" {
		return ErrNotFound
	}

	e, ok := b.executions.Get(taskExecutionArn)
	if !ok {
		return ErrNotFound
	}

	// Same terminal-state guard UpdateTaskExecution already applies (see
	// below): once an execution has settled into SUCCESS or ERROR -- whether
	// via the lazy advance in DescribeTaskExecution or a prior Cancel --
	// cancelling it again must not silently overwrite that outcome.
	if isTerminalExecutionStatus(e.Status) {
		return fmt.Errorf(
			"%w: task execution %s is in terminal state %s and cannot be cancelled",
			ErrInvalidParameter, taskExecutionArn, e.Status,
		)
	}

	e.Status = executionStatusError

	if t, found := b.tasks.Get(taskArn); found && t.CurrentTaskExecutionArn == taskExecutionArn {
		t.Status = taskStatusAvailable
	}

	return nil
}

// DescribeTaskExecution returns task execution details.
// Executions in LAUNCHING state are lazily advanced to SUCCESS on first
// describe. When the execution that finishes is still the task's current
// one, the parent task's Status reverts from RUNNING to AVAILABLE, matching
// AWS (task Status is RUNNING only while a task execution is in progress).
func (b *InMemoryBackend) DescribeTaskExecution(taskExecutionArn string) (*TaskExecution, error) {
	b.mu.Lock("DescribeTaskExecution")
	defer b.mu.Unlock()

	taskArn := extractTaskArnFromExecution(taskExecutionArn)
	if taskArn == "" {
		return nil, ErrNotFound
	}

	e, ok := b.executions.Get(taskExecutionArn)
	if !ok {
		return nil, ErrNotFound
	}

	if e.Status == executionStatusLaunching {
		e.Status = executionStatusSuccess

		if t, found := b.tasks.Get(taskArn); found && t.CurrentTaskExecutionArn == taskExecutionArn {
			t.Status = taskStatusAvailable
		}
	}

	cp := e.toTaskExecution()

	return &cp, nil
}

// ListTaskExecutions returns executions for a task (or, when taskArn is
// empty, every execution across all tasks -- TaskArn is an optional filter
// per the real ListTaskExecutions API), sorted by ARN.
func (b *InMemoryBackend) ListTaskExecutions(
	taskArn string,
	maxResults int32,
	nextToken string,
) ([]*TaskExecutionListEntry, string, error) {
	b.mu.RLock("ListTaskExecutions")
	defer b.mu.RUnlock()

	var execs []*storedTaskExecution
	if taskArn != "" {
		if !b.tasks.Has(taskArn) {
			return nil, "", ErrNotFound
		}

		execs = slices.Clone(b.executionsByTask.Get(taskArn))
	} else {
		execs = b.executions.Snapshot()
	}

	slices.SortFunc(execs, func(a, b *storedTaskExecution) int {
		return strings.Compare(a.TaskExecutionArn, b.TaskExecutionArn)
	})

	all := make([]*TaskExecutionListEntry, 0, len(execs))
	for _, e := range execs {
		all = append(all, &TaskExecutionListEntry{
			TaskExecutionArn: e.TaskExecutionArn,
			Status:           e.Status,
			TaskMode:         e.TaskMode,
		})
	}

	limit := int(maxResults)
	pg := page.New(all, nextToken, limit, defaultMaxResults)

	return pg.Data, pg.Next, nil
}

// UpdateTaskExecution updates a task execution (no-op: options are advisory only).
func (b *InMemoryBackend) UpdateTaskExecution(taskExecutionArn string, options map[string]any) error {
	b.mu.Lock("UpdateTaskExecution")
	defer b.mu.Unlock()

	taskArn := extractTaskArnFromExecution(taskExecutionArn)
	if taskArn == "" {
		return ErrNotFound
	}

	exec, ok := b.executions.Get(taskExecutionArn)
	if !ok {
		return ErrNotFound
	}

	// AWS only allows UpdateTaskExecution while the execution is still in a
	// pre-transfer/transfer phase; terminal (SUCCESS/ERROR) executions cannot
	// be updated.
	if exec.Status == executionStatusSuccess || exec.Status == executionStatusError {
		return fmt.Errorf(
			"%w: task execution %s is in terminal state %s and cannot be updated",
			ErrInvalidParameter, taskExecutionArn, exec.Status,
		)
	}

	// Merge the supplied Options onto the execution (AWS updates only the
	// fields present in the request). BytesPerSecond is the most common knob.
	if len(options) > 0 {
		if exec.Options == nil {
			exec.Options = make(map[string]any, len(options))
		}

		maps.Copy(exec.Options, options)
	}

	return nil
}

// extractTaskArnFromExecution extracts the task ARN from a task execution ARN.
// Execution ARN format: arn:aws:datasync:region:account:task/<task-id>/execution/<exec-id>.
func extractTaskArnFromExecution(execArn string) string {
	// Find /execution/ suffix and strip it.
	idx := strings.LastIndex(execArn, "/execution/")
	if idx < 0 {
		return ""
	}

	return execArn[:idx]
}
