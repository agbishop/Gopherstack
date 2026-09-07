package sagemaker

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrPipelineNotFound is returned when a pipeline does not exist.
	ErrPipelineNotFound = awserr.New("ResourceNotFound", awserr.ErrNotFound)
	// ErrPipelineAlreadyExists is returned when a pipeline already exists.
	// CreatePipeline's error list is ConflictException, not ResourceInUse
	// (botocore sagemaker/2017-07-24@1.43.56 service-2.json's
	// CreatePipeline.errors) — wrap ErrConflictException so handleError's
	// special case (see errors.go) picks the accurate wire type.
	ErrPipelineAlreadyExists = awserr.New("ConflictException", ErrConflictException)
	// ErrPipelineExecutionNotFound is returned when a pipeline execution does not exist.
	ErrPipelineExecutionNotFound = awserr.New("ResourceNotFound", awserr.ErrNotFound)
)

// ParallelismConfiguration limits concurrent steps in a pipeline execution.
type ParallelismConfiguration struct {
	MaxParallelExecutionSteps int32 `json:"MaxParallelExecutionSteps,omitempty"`
}

// PipelineParameter is a name/value pair passed to StartPipelineExecution.
type PipelineParameter struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// Pipeline represents a SageMaker Pipeline.
type Pipeline struct {
	CreationTime             time.Time                 `json:"CreationTime"`
	LastModifiedTime         time.Time                 `json:"LastModifiedTime"`
	Tags                     map[string]string         `json:"Tags,omitempty"`
	ParallelismConfiguration *ParallelismConfiguration `json:"ParallelismConfiguration,omitempty"`
	PipelineName             string                    `json:"PipelineName"`
	PipelineArn              string                    `json:"PipelineArn"`
	PipelineStatus           string                    `json:"PipelineStatus"`
	PipelineDefinition       string                    `json:"PipelineDefinition,omitempty"`
	PipelineDisplayName      string                    `json:"PipelineDisplayName,omitempty"`
	PipelineDescription      string                    `json:"PipelineDescription,omitempty"`
	RoleArn                  string                    `json:"RoleArn,omitempty"`
}

func clonePipeline(p *Pipeline) *Pipeline {
	cp := *p
	cp.Tags = maps.Clone(p.Tags)

	return &cp
}

// SelectedStep names one step to run in a SelectiveExecutionConfig.
type SelectedStep struct {
	StepName string `json:"StepName"`
}

// SelectiveExecutionConfig restricts a pipeline execution to a subset of steps.
type SelectiveExecutionConfig struct {
	SourcePipelineExecutionArn string         `json:"SourcePipelineExecutionArn,omitempty"`
	SelectedSteps              []SelectedStep `json:"SelectedSteps"`
}

// PipelineExecution represents a single execution of a SageMaker Pipeline.
type PipelineExecution struct {
	StartTime                    time.Time                 `json:"StartTime"`
	ParallelismConfiguration     *ParallelismConfiguration `json:"ParallelismConfiguration,omitempty"`
	SelectiveExecutionConfig     *SelectiveExecutionConfig `json:"SelectiveExecutionConfig,omitempty"`
	PipelineExecutionDisplayName string                    `json:"PipelineExecutionDisplayName,omitempty"`
	PipelineExecutionArn         string                    `json:"PipelineExecutionArn"`
	PipelineExecutionStatus      string                    `json:"PipelineExecutionStatus"`
	PipelineArn                  string                    `json:"PipelineArn"`
	PipelineExecutionDescription string                    `json:"PipelineExecutionDescription,omitempty"`
	FailureReason                string                    `json:"FailureReason,omitempty"`
	PipelineDefinition           string                    `json:"PipelineDefinition,omitempty"`
	MlflowExperimentName         string                    `json:"MlflowExperimentName,omitempty"`
	PipelineParameters           []PipelineParameter       `json:"PipelineParameters,omitempty"`
	PipelineVersionID            int64                     `json:"PipelineVersionId,omitempty"`
}

func clonePipelineExecution(pe *PipelineExecution) *PipelineExecution {
	cp := *pe
	cp.PipelineParameters = make([]PipelineParameter, len(pe.PipelineParameters))
	copy(cp.PipelineParameters, pe.PipelineParameters)

	if pe.ParallelismConfiguration != nil {
		pc := *pe.ParallelismConfiguration
		cp.ParallelismConfiguration = &pc
	}

	if pe.SelectiveExecutionConfig != nil {
		sec := *pe.SelectiveExecutionConfig
		sec.SelectedSteps = append([]SelectedStep(nil), pe.SelectiveExecutionConfig.SelectedSteps...)
		cp.SelectiveExecutionConfig = &sec
	}

	return &cp
}

// CreatePipeline creates a new pipeline.
func (b *InMemoryBackend) CreatePipeline(
	ctx context.Context,
	name, definition, roleArn string,
	tags map[string]string,
) (*Pipeline, error) {
	b.mu.Lock("CreatePipeline")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.pipelinesStore(region).Get(name); ok {
		return nil, fmt.Errorf("%w: pipeline %s already exists", ErrPipelineAlreadyExists, name)
	}

	pArn := arn.Build("sagemaker", region, b.accountID, "pipeline/"+name)
	now := time.Now()

	p := &Pipeline{
		PipelineName:       name,
		PipelineArn:        pArn,
		PipelineStatus:     statusActive,
		PipelineDefinition: definition,
		RoleArn:            roleArn,
		CreationTime:       now,
		LastModifiedTime:   now,
		Tags:               mergeTags(nil, tags),
	}
	b.pipelinesStore(region).Put(p)

	return clonePipeline(p), nil
}

// DescribePipeline returns a pipeline by name. If versionID is non-zero, the
// returned Pipeline's PipelineDefinition reflects that specific historical
// version instead of the current one (DescribePipelineInput.PipelineVersionId,
// api_op_DescribePipeline.go). lastRunTime is the StartTime of the most
// recent PipelineExecution for this pipeline (DescribePipelineOutput.LastRunTime),
// or the zero time if the pipeline has never been run.
func (b *InMemoryBackend) DescribePipeline(
	ctx context.Context, name string, versionID int64,
) (*Pipeline, time.Time, error) {
	b.mu.RLock("DescribePipeline")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelinesStoreRO(region).Get(name)
	if !ok {
		return nil, time.Time{}, fmt.Errorf("%w: pipeline %q not found", ErrPipelineNotFound, name)
	}

	result := clonePipeline(p)

	if versionID != 0 {
		v, found := findPipelineVersion(b.pipelineVersionsStoreRO(region)[name], versionID)
		if !found {
			return nil, time.Time{}, fmt.Errorf(
				"%w: pipeline %q version %d not found", ErrPipelineNotFound, name, versionID,
			)
		}

		result.PipelineDefinition = v.PipelineDefinition
	}

	lastRunTime := latestExecutionStartTime(b.pipelineExecutionsStoreRO(region).All(), p.PipelineArn)

	return result, lastRunTime, nil
}

// ListPipelinesParams bundles ListPipelines' filter/sort/pagination input
// (api_op_ListPipelines.go:29-58, sagemaker@v1.263.2).
type ListPipelinesParams struct {
	CreatedAfter       *time.Time
	CreatedBefore      *time.Time
	PipelineNamePrefix string
	NextToken          string
	SortBy             string
	SortOrder          string
	MaxResults         int32
}

// ListPipelines returns pipelines matching params, sorted per params.SortBy
// (documented default CreationTime, api_op_ListPipelines.go:51) / SortOrder
// (no default documented — docs.aws.amazon.com/sagemaker/latest/APIReference/
// API_ListPipelines.html states none, so ascending is kept as the disclosed
// fallback, matching ListHubs' precedent), capped at params.MaxResults.
func (b *InMemoryBackend) ListPipelines(ctx context.Context, params ListPipelinesParams) ([]*Pipeline, string) {
	b.mu.RLock("ListPipelines")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	tbl := b.pipelinesStoreRO(region)
	list := make([]*Pipeline, 0, tbl.Len())

	for _, p := range tbl.All() {
		if params.PipelineNamePrefix != "" && !strings.HasPrefix(p.PipelineName, params.PipelineNamePrefix) {
			continue
		}

		if params.CreatedAfter != nil && !p.CreationTime.After(*params.CreatedAfter) {
			continue
		}

		if params.CreatedBefore != nil && !p.CreationTime.Before(*params.CreatedBefore) {
			continue
		}

		list = append(list, clonePipeline(p))
	}

	desc := strings.EqualFold(params.SortOrder, sortOrderDescending)
	sort.Slice(list, func(i, j int) bool {
		var less bool
		if params.SortBy == keyGenericName {
			less = list[i].PipelineName < list[j].PipelineName
		} else {
			less = list[i].CreationTime.Before(list[j].CreationTime)
		}

		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, params.NextToken, params.MaxResults)
}

// PipelineLastExecutionTime returns the StartTime of the most recent
// PipelineExecution belonging to pipelineArn, or the zero time if it has
// never been run. Exported so handleListPipelines can enrich
// PipelineSummary.LastExecutionTime (api_op_ListPipelines.go response) without
// reaching into the backend's internal store tables directly.
func (b *InMemoryBackend) PipelineLastExecutionTime(ctx context.Context, pipelineArn string) time.Time {
	b.mu.RLock("PipelineLastExecutionTime")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	return latestExecutionStartTime(b.pipelineExecutionsStoreRO(region).All(), pipelineArn)
}

// latestExecutionStartTime returns the StartTime of the most recent
// PipelineExecution belonging to pipelineArn, or the zero time if it has
// never been run. Shared by DescribePipeline (LastRunTime) and
// PipelineLastExecutionTime (LastExecutionTime) — both surface the same
// underlying value under different response field names (api_op_
// DescribePipeline.go:52, api_op_ListPipelines.go response PipelineSummary.
// LastExecutionTime).
func latestExecutionStartTime(execs []*PipelineExecution, pipelineArn string) time.Time {
	var latest time.Time

	for _, pe := range execs {
		if pe.PipelineArn == pipelineArn && pe.StartTime.After(latest) {
			latest = pe.StartTime
		}
	}

	return latest
}

// UpdatePipeline updates a pipeline definition.
func (b *InMemoryBackend) UpdatePipeline(ctx context.Context, name, definition string) (*Pipeline, error) {
	b.mu.Lock("UpdatePipeline")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelinesStore(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: pipeline %q not found", ErrPipelineNotFound, name)
	}

	if definition != "" {
		p.PipelineDefinition = definition
	}

	p.LastModifiedTime = time.Now()

	return clonePipeline(p), nil
}

// DeletePipeline deletes a pipeline.
func (b *InMemoryBackend) DeletePipeline(ctx context.Context, name string) (*Pipeline, error) {
	b.mu.Lock("DeletePipeline")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	store := b.pipelinesStore(region)

	p, ok := store.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: pipeline %q not found", ErrPipelineNotFound, name)
	}

	cp := clonePipeline(p)
	store.Delete(name)
	delete(b.pipelineVersionsStore(region), name)

	return cp, nil
}

// StartPipelineExecution creates a pipeline execution.
func (b *InMemoryBackend) StartPipelineExecution(ctx context.Context, pipelineName string) (*PipelineExecution, error) {
	b.mu.Lock("StartPipelineExecution")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelinesStore(region).Get(pipelineName)
	if !ok {
		return nil, fmt.Errorf("%w: pipeline %q not found", ErrPipelineNotFound, pipelineName)
	}

	execID := generateID()
	execArn := p.PipelineArn + "/execution/" + execID

	pe := &PipelineExecution{
		PipelineArn:             p.PipelineArn,
		PipelineExecutionArn:    execArn,
		PipelineExecutionStatus: pipelineStatusExecuting,
		StartTime:               time.Now(),
	}
	b.pipelineExecutionsStore(region).Put(pe)

	b.runDelayed(b.lifecycleCtx, startTransitionDelay, func() {
		b.mu.Lock("StartPipelineExecution.goroutine")
		defer b.mu.Unlock()

		if exec, exists := b.pipelineExecutionsStore(region).Get(execArn); exists &&
			exec.PipelineExecutionStatus == pipelineStatusExecuting {
			exec.PipelineExecutionStatus = pipelineStatusSucceeded
		}
	})

	return clonePipelineExecution(pe), nil
}

// DescribePipelineExecution returns a pipeline execution.
func (b *InMemoryBackend) DescribePipelineExecution(ctx context.Context, execArn string) (*PipelineExecution, error) {
	b.mu.RLock("DescribePipelineExecution")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	pe, ok := b.pipelineExecutionsStoreRO(region).Get(execArn)
	if !ok {
		return nil, fmt.Errorf(
			"%w: pipeline execution %q not found",
			ErrPipelineExecutionNotFound,
			execArn,
		)
	}

	return clonePipelineExecution(pe), nil
}

// ListPipelineExecutionsParams bundles ListPipelineExecutions' filter/sort/
// pagination input (api_op_ListPipelineExecutions.go:29-62, sagemaker@v1.263.2).
type ListPipelineExecutionsParams struct {
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	PipelineName  string
	NextToken     string
	SortBy        string
	SortOrder     string
	MaxResults    int32
}

// ListPipelineExecutions returns executions for a pipeline matching params,
// sorted per params.SortBy (documented default CreationTime, api_op_
// ListPipelineExecutions.go:51) / SortOrder (no default documented, ascending
// kept as the disclosed fallback), capped at params.MaxResults.
func (b *InMemoryBackend) ListPipelineExecutions(
	ctx context.Context,
	params ListPipelineExecutionsParams,
) ([]*PipelineExecution, string) {
	b.mu.RLock("ListPipelineExecutions")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelinesStoreRO(region).Get(params.PipelineName)
	execStore := b.pipelineExecutionsStoreRO(region)
	list := make([]*PipelineExecution, 0, execStore.Len())

	if ok {
		for _, pe := range execStore.All() {
			if pe.PipelineArn != p.PipelineArn {
				continue
			}

			if params.CreatedAfter != nil && !pe.StartTime.After(*params.CreatedAfter) {
				continue
			}

			if params.CreatedBefore != nil && !pe.StartTime.Before(*params.CreatedBefore) {
				continue
			}

			list = append(list, clonePipelineExecution(pe))
		}
	}

	desc := strings.EqualFold(params.SortOrder, sortOrderDescending)
	sort.Slice(list, func(i, j int) bool {
		var less bool
		if params.SortBy == "PipelineExecutionArn" {
			less = list[i].PipelineExecutionArn < list[j].PipelineExecutionArn
		} else {
			less = list[i].StartTime.Before(list[j].StartTime)
		}

		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, params.NextToken, params.MaxResults)
}

// CreatePipelineOptions holds full input for CreatePipeline.
type CreatePipelineOptions struct {
	Tags                     map[string]string
	ParallelismConfiguration *ParallelismConfiguration
	PipelineName             string
	PipelineDefinition       string
	PipelineDisplayName      string
	PipelineDescription      string
	RoleArn                  string
}

// CreatePipelineFull creates a pipeline with full AWS input fields.
func (b *InMemoryBackend) CreatePipelineFull(ctx context.Context, opts CreatePipelineOptions) (*Pipeline, error) {
	b.mu.Lock("CreatePipelineFull")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.pipelinesStore(region).Get(opts.PipelineName); ok {
		return nil, fmt.Errorf(
			"%w: pipeline %s already exists",
			ErrPipelineAlreadyExists,
			opts.PipelineName,
		)
	}

	pArn := arn.Build("sagemaker", region, b.accountID, "pipeline/"+opts.PipelineName)
	now := time.Now()

	p := &Pipeline{
		PipelineName:             opts.PipelineName,
		PipelineArn:              pArn,
		PipelineStatus:           "Active",
		PipelineDefinition:       opts.PipelineDefinition,
		PipelineDisplayName:      opts.PipelineDisplayName,
		PipelineDescription:      opts.PipelineDescription,
		RoleArn:                  opts.RoleArn,
		ParallelismConfiguration: opts.ParallelismConfiguration,
		CreationTime:             now,
		LastModifiedTime:         now,
		Tags:                     mergeTags(nil, opts.Tags),
	}
	b.pipelinesStore(region).Put(p)
	b.recordPipelineVersionLocked(region, p)

	return clonePipeline(p), nil
}

// UpdatePipelineFull updates a pipeline with full AWS input fields.
func (b *InMemoryBackend) UpdatePipelineFull(
	ctx context.Context,
	name, definition, displayName, description, roleArn string,
	parallelismConfig *ParallelismConfiguration,
) (*Pipeline, error) {
	b.mu.Lock("UpdatePipelineFull")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelinesStore(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: pipeline %q not found", ErrPipelineNotFound, name)
	}

	if definition != "" {
		p.PipelineDefinition = definition
	}
	if displayName != "" {
		p.PipelineDisplayName = displayName
	}
	if description != "" {
		p.PipelineDescription = description
	}
	if roleArn != "" {
		p.RoleArn = roleArn
	}
	if parallelismConfig != nil {
		p.ParallelismConfiguration = parallelismConfig
	}
	p.LastModifiedTime = time.Now()
	b.recordPipelineVersionLocked(region, p)

	return clonePipeline(p), nil
}

// StartPipelineExecutionOptions holds full input for StartPipelineExecution.
type StartPipelineExecutionOptions struct {
	ParallelismConfiguration     *ParallelismConfiguration
	SelectiveExecutionConfig     *SelectiveExecutionConfig
	PipelineName                 string
	PipelineExecutionDisplayName string
	PipelineExecutionDescription string
	MlflowExperimentName         string
	PipelineParameters           []PipelineParameter
	PipelineVersionID            int64
}

// StartPipelineExecutionFull creates an execution with full AWS input fields.
func (b *InMemoryBackend) StartPipelineExecutionFull(
	ctx context.Context,
	opts StartPipelineExecutionOptions,
) (*PipelineExecution, error) {
	b.mu.Lock("StartPipelineExecutionFull")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	p, ok := b.pipelinesStore(region).Get(opts.PipelineName)
	if !ok {
		return nil, fmt.Errorf("%w: pipeline %q not found", ErrPipelineNotFound, opts.PipelineName)
	}

	execID := generateID()
	execArn := p.PipelineArn + "/execution/" + execID

	params := make([]PipelineParameter, len(opts.PipelineParameters))
	copy(params, opts.PipelineParameters)

	pe := &PipelineExecution{
		PipelineArn:                  p.PipelineArn,
		PipelineExecutionArn:         execArn,
		PipelineExecutionStatus:      pipelineStatusExecuting,
		PipelineExecutionDisplayName: opts.PipelineExecutionDisplayName,
		PipelineExecutionDescription: opts.PipelineExecutionDescription,
		PipelineParameters:           params,
		PipelineDefinition:           p.PipelineDefinition,
		ParallelismConfiguration:     opts.ParallelismConfiguration,
		SelectiveExecutionConfig:     opts.SelectiveExecutionConfig,
		PipelineVersionID:            opts.PipelineVersionID,
		MlflowExperimentName:         opts.MlflowExperimentName,
		StartTime:                    time.Now(),
	}
	b.pipelineExecutionsStore(region).Put(pe)
	b.recordPipelineExecutionOnLatestVersionLocked(region, opts.PipelineName, execArn)

	b.runDelayed(b.lifecycleCtx, startTransitionDelay, func() {
		b.mu.Lock("StartPipelineExecutionFull.goroutine")
		defer b.mu.Unlock()

		if exec, exists := b.pipelineExecutionsStore(region).Get(execArn); exists &&
			exec.PipelineExecutionStatus == pipelineStatusExecuting {
			exec.PipelineExecutionStatus = pipelineStatusSucceeded
		}
	})

	return clonePipelineExecution(pe), nil
}
