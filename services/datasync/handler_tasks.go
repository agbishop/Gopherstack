package datasync

import (
	"context"
	"fmt"
)

// --- Task operations ---

// filterRuleInput/filterRuleOutput mirror the real FilterRule shape
// (FilterType, Value) used by CreateTask/UpdateTask/DescribeTaskOutput's
// Excludes and Includes members.
type filterRuleInput struct {
	FilterType string `json:"FilterType,omitempty"`
	Value      string `json:"Value,omitempty"`
}

type filterRuleOutput struct {
	FilterType string `json:"FilterType,omitempty"`
	Value      string `json:"Value,omitempty"`
}

// taskScheduleInput/taskScheduleOutput mirror the real TaskSchedule shape.
type taskScheduleInput struct {
	ScheduleExpression string `json:"ScheduleExpression"`
	Status             string `json:"Status,omitempty"`
}

type taskScheduleOutput struct {
	ScheduleExpression string `json:"ScheduleExpression"`
	Status             string `json:"Status,omitempty"`
}

func filterRulesFromInput(rules []filterRuleInput) []FilterRule {
	if rules == nil {
		return nil
	}

	out := make([]FilterRule, len(rules))
	for i, r := range rules {
		out[i] = FilterRule(r)
	}

	return out
}

func filterRulesToOutput(rules []FilterRule) []filterRuleOutput {
	if rules == nil {
		return nil
	}

	out := make([]filterRuleOutput, len(rules))
	for i, r := range rules {
		out[i] = filterRuleOutput(r)
	}

	return out
}

func taskScheduleFromInput(s *taskScheduleInput) *TaskSchedule {
	if s == nil {
		return nil
	}

	return &TaskSchedule{ScheduleExpression: s.ScheduleExpression, Status: s.Status}
}

func taskScheduleToOutput(s *TaskSchedule) *taskScheduleOutput {
	if s == nil {
		return nil
	}

	return &taskScheduleOutput{ScheduleExpression: s.ScheduleExpression, Status: s.Status}
}

type createTaskInput struct {
	Options                map[string]any     `json:"Options"`
	Schedule               *taskScheduleInput `json:"Schedule"`
	ManifestConfig         map[string]any     `json:"ManifestConfig"`
	TaskReportConfig       map[string]any     `json:"TaskReportConfig"`
	SourceLocationArn      string             `json:"SourceLocationArn"`
	DestinationLocationArn string             `json:"DestinationLocationArn"`
	Name                   string             `json:"Name"`
	CloudWatchLogGroupArn  string             `json:"CloudWatchLogGroupArn,omitempty"`
	TaskMode               string             `json:"TaskMode,omitempty"`
	Tags                   []tagInput         `json:"Tags"`
	Excludes               []filterRuleInput  `json:"Excludes"`
	Includes               []filterRuleInput  `json:"Includes"`
}

type createTaskOutput struct {
	TaskArn string `json:"TaskArn"`
}

func (h *Handler) handleCreateTask(_ context.Context, in *createTaskInput) (*createTaskOutput, error) {
	if in.SourceLocationArn == "" {
		return nil, fmt.Errorf("%w: SourceLocationArn is required", errInvalidRequest)
	}

	if in.DestinationLocationArn == "" {
		return nil, fmt.Errorf("%w: DestinationLocationArn is required", errInvalidRequest)
	}

	tags := tagsFromInput(in.Tags)

	settings := TaskSettings{
		Options:          in.Options,
		Schedule:         taskScheduleFromInput(in.Schedule),
		ManifestConfig:   in.ManifestConfig,
		TaskReportConfig: in.TaskReportConfig,
		Excludes:         filterRulesFromInput(in.Excludes),
		Includes:         filterRulesFromInput(in.Includes),
		TaskMode:         in.TaskMode,
	}

	t, err := h.Backend.CreateTask(
		in.SourceLocationArn,
		in.DestinationLocationArn,
		in.Name,
		in.CloudWatchLogGroupArn,
		settings,
		tags,
	)
	if err != nil {
		return nil, err
	}

	return &createTaskOutput{TaskArn: t.TaskArn}, nil
}

type describeTaskInput struct {
	TaskArn string `json:"TaskArn"`
}

// describeTaskOutput covers the real DescribeTaskOutput's FK/status/settings
// members. DestinationNetworkInterfaceArns, SourceNetworkInterfaceArns,
// ErrorCode, ErrorDetail, and ScheduleDetails also exist on the real output
// but are omitted here: gopherstack doesn't model ENIs or task-execution
// failures, so they would always be empty/absent -- an honest omission
// rather than a fabricated value.
type describeTaskOutput struct {
	Options                 map[string]any      `json:"Options,omitempty"`
	Schedule                *taskScheduleOutput `json:"Schedule,omitempty"`
	ManifestConfig          map[string]any      `json:"ManifestConfig,omitempty"`
	TaskReportConfig        map[string]any      `json:"TaskReportConfig,omitempty"`
	TaskArn                 string              `json:"TaskArn"`
	Name                    string              `json:"Name"`
	Status                  string              `json:"Status"`
	SourceLocationArn       string              `json:"SourceLocationArn"`
	DestinationLocationArn  string              `json:"DestinationLocationArn"`
	CloudWatchLogGroupArn   string              `json:"CloudWatchLogGroupArn,omitempty"`
	CurrentTaskExecutionArn string              `json:"CurrentTaskExecutionArn,omitempty"`
	TaskMode                string              `json:"TaskMode,omitempty"`
	Excludes                []filterRuleOutput  `json:"Excludes,omitempty"`
	Includes                []filterRuleOutput  `json:"Includes,omitempty"`
	CreationTime            int64               `json:"CreationTime"`
}

func (h *Handler) handleDescribeTask(_ context.Context, in *describeTaskInput) (*describeTaskOutput, error) {
	if in.TaskArn == "" {
		return nil, fmt.Errorf("%w: TaskArn is required", errInvalidRequest)
	}

	t, err := h.Backend.DescribeTask(in.TaskArn)
	if err != nil {
		return nil, err
	}

	return &describeTaskOutput{
		TaskArn:                 t.TaskArn,
		Name:                    t.Name,
		Status:                  t.Status,
		SourceLocationArn:       t.SourceLocationArn,
		DestinationLocationArn:  t.DestinationLocationArn,
		CloudWatchLogGroupArn:   t.CloudWatchLogGroupArn,
		CurrentTaskExecutionArn: t.CurrentTaskExecutionArn,
		CreationTime:            t.CreationTime.Unix(),
		Options:                 t.Options,
		Schedule:                taskScheduleToOutput(t.Schedule),
		ManifestConfig:          t.ManifestConfig,
		TaskReportConfig:        t.TaskReportConfig,
		Excludes:                filterRulesToOutput(t.Excludes),
		Includes:                filterRulesToOutput(t.Includes),
		TaskMode:                t.TaskMode,
	}, nil
}

type updateTaskInput struct {
	Options               map[string]any     `json:"Options"`
	Schedule              *taskScheduleInput `json:"Schedule"`
	ManifestConfig        map[string]any     `json:"ManifestConfig"`
	TaskReportConfig      map[string]any     `json:"TaskReportConfig"`
	TaskArn               string             `json:"TaskArn"`
	Name                  string             `json:"Name,omitempty"`
	CloudWatchLogGroupArn string             `json:"CloudWatchLogGroupArn,omitempty"`
	Excludes              []filterRuleInput  `json:"Excludes"`
	Includes              []filterRuleInput  `json:"Includes"`
}

type updateTaskOutput struct{}

func (h *Handler) handleUpdateTask(_ context.Context, in *updateTaskInput) (*updateTaskOutput, error) {
	if in.TaskArn == "" {
		return nil, fmt.Errorf("%w: TaskArn is required", errInvalidRequest)
	}

	settings := TaskSettings{
		Options:          in.Options,
		Schedule:         taskScheduleFromInput(in.Schedule),
		ManifestConfig:   in.ManifestConfig,
		TaskReportConfig: in.TaskReportConfig,
		Excludes:         filterRulesFromInput(in.Excludes),
		Includes:         filterRulesFromInput(in.Includes),
	}

	if err := h.Backend.UpdateTask(in.TaskArn, in.Name, in.CloudWatchLogGroupArn, settings); err != nil {
		return nil, err
	}

	return &updateTaskOutput{}, nil
}

type deleteTaskInput struct {
	TaskArn string `json:"TaskArn"`
}

type deleteTaskOutput struct{}

func (h *Handler) handleDeleteTask(_ context.Context, in *deleteTaskInput) (*deleteTaskOutput, error) {
	if in.TaskArn == "" {
		return nil, fmt.Errorf("%w: TaskArn is required", errInvalidRequest)
	}

	if err := h.Backend.DeleteTask(in.TaskArn); err != nil {
		return nil, err
	}

	return &deleteTaskOutput{}, nil
}

type taskFilterInput struct {
	Name     string   `json:"Name"`
	Operator string   `json:"Operator"`
	Values   []string `json:"Values"`
}

type listTasksInput struct {
	NextToken  string            `json:"NextToken"`
	Filters    []taskFilterInput `json:"Filters"`
	MaxResults int32             `json:"MaxResults"`
}

type taskListEntryOutput struct {
	TaskArn  string `json:"TaskArn"`
	Name     string `json:"Name"`
	Status   string `json:"Status"`
	TaskMode string `json:"TaskMode,omitempty"`
}

type listTasksOutput struct {
	NextToken string                `json:"NextToken,omitempty"`
	Tasks     []taskListEntryOutput `json:"Tasks"`
}

func (h *Handler) handleListTasks(_ context.Context, in *listTasksInput) (*listTasksOutput, error) {
	filters := make([]TaskFilter, 0, len(in.Filters))
	for _, f := range in.Filters {
		filters = append(filters, TaskFilter(f))
	}

	tasks, nextToken, err := h.Backend.ListTasks(filters, in.MaxResults, in.NextToken)
	if err != nil {
		return nil, err
	}

	out := make([]taskListEntryOutput, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskListEntryOutput{
			TaskArn:  t.TaskArn,
			Name:     t.Name,
			Status:   t.Status,
			TaskMode: t.TaskMode,
		})
	}

	return &listTasksOutput{Tasks: out, NextToken: nextToken}, nil
}

// --- Task execution operations ---

type startTaskExecutionInput struct {
	OverrideOptions  map[string]any    `json:"OverrideOptions"`
	ManifestConfig   map[string]any    `json:"ManifestConfig"`
	TaskReportConfig map[string]any    `json:"TaskReportConfig"`
	TaskArn          string            `json:"TaskArn"`
	Tags             []tagInput        `json:"Tags"`
	Excludes         []filterRuleInput `json:"Excludes"`
	Includes         []filterRuleInput `json:"Includes"`
}

type startTaskExecutionOutput struct {
	TaskExecutionArn string `json:"TaskExecutionArn"`
}

func (h *Handler) handleStartTaskExecution(
	_ context.Context,
	in *startTaskExecutionInput,
) (*startTaskExecutionOutput, error) {
	if in.TaskArn == "" {
		return nil, fmt.Errorf("%w: TaskArn is required", errInvalidRequest)
	}

	overrides := TaskExecutionOverrides{
		Options:          in.OverrideOptions,
		ManifestConfig:   in.ManifestConfig,
		TaskReportConfig: in.TaskReportConfig,
		Excludes:         filterRulesFromInput(in.Excludes),
		Includes:         filterRulesFromInput(in.Includes),
	}

	e, err := h.Backend.StartTaskExecution(in.TaskArn, overrides, tagsFromInput(in.Tags))
	if err != nil {
		return nil, err
	}

	return &startTaskExecutionOutput{TaskExecutionArn: e.TaskExecutionArn}, nil
}

type cancelTaskExecutionInput struct {
	TaskExecutionArn string `json:"TaskExecutionArn"`
}

type cancelTaskExecutionOutput struct{}

func (h *Handler) handleCancelTaskExecution(
	_ context.Context,
	in *cancelTaskExecutionInput,
) (*cancelTaskExecutionOutput, error) {
	if in.TaskExecutionArn == "" {
		return nil, fmt.Errorf("%w: TaskExecutionArn is required", errInvalidRequest)
	}

	if err := h.Backend.CancelTaskExecution(in.TaskExecutionArn); err != nil {
		return nil, err
	}

	return &cancelTaskExecutionOutput{}, nil
}

type describeTaskExecutionInput struct {
	TaskExecutionArn string `json:"TaskExecutionArn"`
}

type describeTaskExecutionOutput struct {
	Options                  map[string]any     `json:"Options,omitempty"`
	ManifestConfig           map[string]any     `json:"ManifestConfig,omitempty"`
	TaskReportConfig         map[string]any     `json:"TaskReportConfig,omitempty"`
	TaskExecutionArn         string             `json:"TaskExecutionArn"`
	Status                   string             `json:"Status"`
	TaskMode                 string             `json:"TaskMode,omitempty"`
	Excludes                 []filterRuleOutput `json:"Excludes,omitempty"`
	Includes                 []filterRuleOutput `json:"Includes,omitempty"`
	StartTime                int64              `json:"StartTime"`
	EstimatedFilesToTransfer int64              `json:"EstimatedFilesToTransfer"`
	EstimatedBytesToTransfer int64              `json:"EstimatedBytesToTransfer"`
	FilesTransferred         int64              `json:"FilesTransferred"`
	BytesTransferred         int64              `json:"BytesTransferred"`
}

func (h *Handler) handleDescribeTaskExecution(
	_ context.Context,
	in *describeTaskExecutionInput,
) (*describeTaskExecutionOutput, error) {
	if in.TaskExecutionArn == "" {
		return nil, fmt.Errorf("%w: TaskExecutionArn is required", errInvalidRequest)
	}

	e, err := h.Backend.DescribeTaskExecution(in.TaskExecutionArn)
	if err != nil {
		return nil, err
	}

	return &describeTaskExecutionOutput{
		TaskExecutionArn:         e.TaskExecutionArn,
		Status:                   e.Status,
		TaskMode:                 e.TaskMode,
		StartTime:                e.StartTime.Unix(),
		Options:                  e.Options,
		ManifestConfig:           e.ManifestConfig,
		TaskReportConfig:         e.TaskReportConfig,
		Excludes:                 filterRulesToOutput(e.Excludes),
		Includes:                 filterRulesToOutput(e.Includes),
		EstimatedFilesToTransfer: e.EstimatedFilesToTransfer,
		EstimatedBytesToTransfer: e.EstimatedBytesToTransfer,
		FilesTransferred:         e.FilesTransferred,
		BytesTransferred:         e.BytesTransferred,
	}, nil
}

type listTaskExecutionsInput struct {
	TaskArn    string `json:"TaskArn"`
	NextToken  string `json:"NextToken"`
	MaxResults int32  `json:"MaxResults"`
}

type taskExecutionListEntryOutput struct {
	TaskExecutionArn string `json:"TaskExecutionArn"`
	Status           string `json:"Status"`
	TaskMode         string `json:"TaskMode,omitempty"`
}

type listTaskExecutionsOutput struct {
	NextToken      string                         `json:"NextToken,omitempty"`
	TaskExecutions []taskExecutionListEntryOutput `json:"TaskExecutions"`
}

func (h *Handler) handleListTaskExecutions(
	_ context.Context,
	in *listTaskExecutionsInput,
) (*listTaskExecutionsOutput, error) {
	executions, nextToken, err := h.Backend.ListTaskExecutions(in.TaskArn, in.MaxResults, in.NextToken)
	if err != nil {
		return nil, err
	}

	out := make([]taskExecutionListEntryOutput, 0, len(executions))
	for _, e := range executions {
		out = append(out, taskExecutionListEntryOutput{
			TaskExecutionArn: e.TaskExecutionArn,
			Status:           e.Status,
			TaskMode:         e.TaskMode,
		})
	}

	return &listTaskExecutionsOutput{TaskExecutions: out, NextToken: nextToken}, nil
}

// --- UpdateTaskExecution ---

type updateTaskExecutionInput struct {
	Options          map[string]any `json:"Options"`
	TaskExecutionArn string         `json:"TaskExecutionArn"`
}

type updateTaskExecutionOutput struct{}

func (h *Handler) handleUpdateTaskExecution(
	_ context.Context,
	in *updateTaskExecutionInput,
) (*updateTaskExecutionOutput, error) {
	if in.TaskExecutionArn == "" {
		return nil, fmt.Errorf("%w: TaskExecutionArn is required", errInvalidRequest)
	}

	// AWS requires the Options member on UpdateTaskExecution.
	if len(in.Options) == 0 {
		return nil, fmt.Errorf("%w: Options is required", errInvalidRequest)
	}

	if err := h.Backend.UpdateTaskExecution(in.TaskExecutionArn, in.Options); err != nil {
		return nil, err
	}

	return &updateTaskExecutionOutput{}, nil
}
