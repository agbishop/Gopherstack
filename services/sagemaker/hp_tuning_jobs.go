package sagemaker

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrHPTuningJobNotFound is returned when an HP tuning job does not exist.
	ErrHPTuningJobNotFound = awserr.New("ResourceNotFound", ErrResourceNotFound)
	// ErrHPTuningJobAlreadyExists is returned when an HP tuning job already exists.
	ErrHPTuningJobAlreadyExists = awserr.New("ResourceInUse", awserr.ErrConflict)
)

// HPResourceLimits mirrors AWS's ResourceLimits: the maximum number of
// training jobs (total and concurrent) a tuning job may launch.
type HPResourceLimits struct {
	MaxParallelTrainingJobs int32 `json:"MaxParallelTrainingJobs"`
	MaxNumberOfTrainingJobs int32 `json:"MaxNumberOfTrainingJobs,omitempty"`
	MaxRuntimeInSeconds     int32 `json:"MaxRuntimeInSeconds,omitempty"`
}

// HPObjectiveStatusCounters mirrors AWS's ObjectiveStatusCounters: counts of
// child training jobs by objective-metric-evaluation status. This emulator
// does not launch child training jobs, so these always read zero.
type HPObjectiveStatusCounters struct {
	Succeeded int32 `json:"Succeeded"`
	Pending   int32 `json:"Pending"`
	Failed    int32 `json:"Failed"`
}

// HPTrainingJobStatusCounters mirrors AWS's TrainingJobStatusCounters: counts
// of child training jobs by status. This emulator does not launch child
// training jobs, so these always read zero.
type HPTrainingJobStatusCounters struct {
	Completed         int32 `json:"Completed"`
	InProgress        int32 `json:"InProgress"`
	NonRetryableError int32 `json:"NonRetryableError"`
	RetryableError    int32 `json:"RetryableError"`
	Stopped           int32 `json:"Stopped"`
}

// HyperParameterTuningJob represents a SageMaker hyperparameter tuning job.
// HyperParameterTuningJobConfig/Autotune/WarmStartConfig/TrainingJobDefinition/
// TrainingJobDefinitions are stored as opaque json.RawMessage passthrough
// (same convention as ai_workload_configs.go): this backend never launches
// child training jobs or actually searches hyperparameters, so only the
// client-supplied config round-trips — every field a real client sent is
// echoed back verbatim, since DescribeHyperParameterTuningJobOutput never
// mutates any of these after Create. Strategy/ResourceLimits are additionally
// kept as their own typed fields because ListHyperParameterTuningJobs'
// summary and this file's own filter/sort logic need them independent of the
// raw config blob.
type HyperParameterTuningJob struct {
	LastModifiedTime              time.Time                   `json:"LastModifiedTime"`
	CreationTime                  time.Time                   `json:"CreationTime"`
	Tags                          map[string]string           `json:"Tags,omitempty"`
	HyperParameterTuningJobName   string                      `json:"HyperParameterTuningJobName"`
	Strategy                      string                      `json:"Strategy,omitempty"`
	HyperParameterTuningJobStatus string                      `json:"HyperParameterTuningJobStatus"`
	HyperParameterTuningJobArn    string                      `json:"HyperParameterTuningJobArn"`
	HyperParameterTuningJobConfig json.RawMessage             `json:"HyperParameterTuningJobConfig,omitempty"`
	TrainingJobDefinitions        json.RawMessage             `json:"TrainingJobDefinitions,omitempty"`
	TrainingJobDefinition         json.RawMessage             `json:"TrainingJobDefinition,omitempty"`
	WarmStartConfig               json.RawMessage             `json:"WarmStartConfig,omitempty"`
	Autotune                      json.RawMessage             `json:"Autotune,omitempty"`
	TrainingJobStatusCounters     HPTrainingJobStatusCounters `json:"TrainingJobStatusCounters"`
	ResourceLimits                HPResourceLimits            `json:"ResourceLimits"`
	ObjectiveStatusCounters       HPObjectiveStatusCounters   `json:"ObjectiveStatusCounters"`
}

// cloneHPTuningJob returns a deep copy of j.
func cloneHPTuningJob(j *HyperParameterTuningJob) *HyperParameterTuningJob {
	cp := *j
	cp.Tags = maps.Clone(j.Tags)
	cp.HyperParameterTuningJobConfig = append(json.RawMessage(nil), j.HyperParameterTuningJobConfig...)
	cp.Autotune = append(json.RawMessage(nil), j.Autotune...)
	cp.WarmStartConfig = append(json.RawMessage(nil), j.WarmStartConfig...)
	cp.TrainingJobDefinition = append(json.RawMessage(nil), j.TrainingJobDefinition...)
	cp.TrainingJobDefinitions = append(json.RawMessage(nil), j.TrainingJobDefinitions...)

	return &cp
}

// ---------------------------------------------------------------------------
// HyperParameterTuningJob
// ---------------------------------------------------------------------------

// CreateHyperParameterTuningJobOptions holds the parameters for creating an
// HP tuning job (api_op_CreateHyperParameterTuningJob.go:44-77).
type CreateHyperParameterTuningJobOptions struct {
	Tags                          map[string]string
	Name                          string
	Strategy                      string
	HyperParameterTuningJobConfig json.RawMessage
	Autotune                      json.RawMessage
	WarmStartConfig               json.RawMessage
	TrainingJobDefinition         json.RawMessage
	TrainingJobDefinitions        json.RawMessage
	Limits                        HPResourceLimits
}

// CreateHyperParameterTuningJob creates a new HPO job.
func (b *InMemoryBackend) CreateHyperParameterTuningJob(
	ctx context.Context,
	opts CreateHyperParameterTuningJobOptions,
) (*HyperParameterTuningJob, error) {
	b.mu.Lock("CreateHyperParameterTuningJob")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.hpTuningJobsStore(region).Get(opts.Name); ok {
		return nil, fmt.Errorf(
			"%w: HP tuning job %s already exists",
			ErrHPTuningJobAlreadyExists,
			opts.Name,
		)
	}

	jobARN := arn.Build("sagemaker", region, b.accountID, "hyper-parameter-tuning-job/"+opts.Name)
	now := time.Now()
	j := &HyperParameterTuningJob{
		HyperParameterTuningJobName:   opts.Name,
		HyperParameterTuningJobArn:    jobARN,
		HyperParameterTuningJobStatus: trainingJobStatusInProgress,
		Strategy:                      opts.Strategy,
		ResourceLimits:                opts.Limits,
		HyperParameterTuningJobConfig: opts.HyperParameterTuningJobConfig,
		Autotune:                      opts.Autotune,
		WarmStartConfig:               opts.WarmStartConfig,
		TrainingJobDefinition:         opts.TrainingJobDefinition,
		TrainingJobDefinitions:        opts.TrainingJobDefinitions,
		CreationTime:                  now,
		LastModifiedTime:              now,
		Tags:                          mergeTags(nil, opts.Tags),
	}
	b.hpTuningJobsStore(region).Put(j)
	b.hpTuningJobARNIndexStore(region)[jobARN] = opts.Name

	b.scheduleHPTuningJobCompletion(b.lifecycleCtx, region, opts.Name)

	return cloneHPTuningJob(j), nil
}

// scheduleHPTuningJobCompletion drives InProgress -> Completed after delay,
// mirroring scheduleTrainingCompletion. ctx must be b.lifecycleCtx captured
// by the caller while holding b.mu.
func (b *InMemoryBackend) scheduleHPTuningJobCompletion(ctx context.Context, region, name string) {
	b.runDelayed(ctx, hpTuningJobInProgressToCompleted, func() {
		b.mu.Lock("scheduleHPTuningJobCompletion.goroutine")
		defer b.mu.Unlock()

		j, ok := b.hpTuningJobsStore(region).Get(name)
		if !ok || j.HyperParameterTuningJobStatus != trainingJobStatusInProgress {
			return
		}

		j.HyperParameterTuningJobStatus = algorithmStatusCompleted
		j.LastModifiedTime = time.Now()
	})
}

// DescribeHyperParameterTuningJob returns an HP tuning job by name.
func (b *InMemoryBackend) DescribeHyperParameterTuningJob(
	ctx context.Context,
	name string,
) (*HyperParameterTuningJob, error) {
	b.mu.RLock("DescribeHyperParameterTuningJob")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	j, ok := b.hpTuningJobsStoreRO(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: HP tuning job %q not found", ErrHPTuningJobNotFound, name)
	}

	return cloneHPTuningJob(j), nil
}

// ListHyperParameterTuningJobsFilter narrows and orders the results of
// ListHyperParameterTuningJobs (api_op_ListHyperParameterTuningJobs.go:30-64).
// The op's own doc states both real defaults explicitly: SortBy is Name,
// SortOrder is Ascending — unlike most sibling List ops in this service,
// which default to CreationTime.
type ListHyperParameterTuningJobsFilter struct {
	CreationTimeAfter      *time.Time
	CreationTimeBefore     *time.Time
	LastModifiedTimeAfter  *time.Time
	LastModifiedTimeBefore *time.Time
	NameContains           string
	StatusEquals           string
	SortBy                 string
	SortOrder              string
	MaxResults             int32
}

func hpTuningJobMatchesFilter(j *HyperParameterTuningJob, filter ListHyperParameterTuningJobsFilter) bool {
	if filter.StatusEquals != "" && j.HyperParameterTuningJobStatus != filter.StatusEquals {
		return false
	}

	if filter.NameContains != "" &&
		!strings.Contains(strings.ToLower(j.HyperParameterTuningJobName), strings.ToLower(filter.NameContains)) {
		return false
	}

	if filter.CreationTimeAfter != nil && !j.CreationTime.After(*filter.CreationTimeAfter) {
		return false
	}

	if filter.CreationTimeBefore != nil && !j.CreationTime.Before(*filter.CreationTimeBefore) {
		return false
	}

	if filter.LastModifiedTimeAfter != nil && !j.LastModifiedTime.After(*filter.LastModifiedTimeAfter) {
		return false
	}

	if filter.LastModifiedTimeBefore != nil && !j.LastModifiedTime.Before(*filter.LastModifiedTimeBefore) {
		return false
	}

	return true
}

// lessHPTuningJob orders a before b by sortBy (Status/CreationTime/default
// Name, tie-broken by name).
func lessHPTuningJob(a, b *HyperParameterTuningJob, sortBy string) bool {
	switch sortBy {
	case keyStatus:
		return a.HyperParameterTuningJobStatus < b.HyperParameterTuningJobStatus
	case keyCreationTime:
		if a.CreationTime.Equal(b.CreationTime) {
			return a.HyperParameterTuningJobName < b.HyperParameterTuningJobName
		}

		return a.CreationTime.Before(b.CreationTime)
	default:
		return a.HyperParameterTuningJobName < b.HyperParameterTuningJobName
	}
}

// ListHyperParameterTuningJobs returns jobs matching filter, sorted by
// filter.SortBy (default Name) / filter.SortOrder (default Ascending).
func (b *InMemoryBackend) ListHyperParameterTuningJobs(
	ctx context.Context,
	nextToken string,
	filter ListHyperParameterTuningJobsFilter,
) ([]*HyperParameterTuningJob, string) {
	b.mu.RLock("ListHyperParameterTuningJobs")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	list := make([]*HyperParameterTuningJob, 0, b.hpTuningJobsStoreRO(region).Len())

	for _, j := range b.hpTuningJobsStoreRO(region).All() {
		if hpTuningJobMatchesFilter(j, filter) {
			list = append(list, cloneHPTuningJob(j))
		}
	}

	desc := strings.EqualFold(filter.SortOrder, "Descending")
	sort.Slice(list, func(i, k int) bool {
		less := lessHPTuningJob(list[i], list[k], filter.SortBy)
		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, nextToken, filter.MaxResults)
}

// StopHyperParameterTuningJob transitions Stopping -> Stopped.
func (b *InMemoryBackend) StopHyperParameterTuningJob(ctx context.Context, name string) error {
	b.mu.Lock("StopHyperParameterTuningJob")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	j, ok := b.hpTuningJobsStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: HP tuning job %q not found", ErrHPTuningJobNotFound, name)
	}

	j.HyperParameterTuningJobStatus = pipelineStatusStopping
	j.LastModifiedTime = time.Now()

	b.runDelayed(b.lifecycleCtx, hpTuningJobStoppingToStopped, func() {
		b.mu.Lock("StopHyperParameterTuningJob.goroutine")
		defer b.mu.Unlock()

		j2, ok2 := b.hpTuningJobsStore(region).Get(name)
		if !ok2 || j2.HyperParameterTuningJobStatus != pipelineStatusStopping {
			return
		}

		j2.HyperParameterTuningJobStatus = pipelineStatusStopped
		j2.LastModifiedTime = time.Now()
	})

	return nil
}

// DeleteHyperParameterTuningJob removes an HP tuning job from the backend.
func (b *InMemoryBackend) DeleteHyperParameterTuningJob(ctx context.Context, name string) error {
	b.mu.Lock("DeleteHyperParameterTuningJob")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	j, ok := b.hpTuningJobsStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: HP tuning job %q not found", ErrHPTuningJobNotFound, name)
	}

	arnIdx := b.hpTuningJobARNIndexStore(region)
	delete(arnIdx, j.HyperParameterTuningJobArn)
	store := b.hpTuningJobsStore(region)
	store.Delete(name)

	return nil
}

// ListTrainingJobsForHyperParameterTuningJobFilter narrows and orders the
// results of ListTrainingJobsForHyperParameterTuningJob
// (api_op_ListTrainingJobsForHyperParameterTuningJob.go:27-56). The op's own
// doc states the real default explicitly: SortBy is Name, SortOrder is
// Ascending. SortBy == FinalObjectiveMetricValue is a real, disclosed no-op:
// the doc's own text says a training job with no objective metric is
// excluded entirely when sorting by it, and this backend never computes an
// objective metric for any child training job, so the correct (not merely
// convenient) result is always empty.
type ListTrainingJobsForHyperParameterTuningJobFilter struct {
	StatusEquals string
	SortBy       string
	SortOrder    string
	MaxResults   int32
}

// lessHPTrainingJob orders a before b by sortBy (Status/CreationTime/default
// Name, tie-broken by name) — the op's own doc states the real default is
// Name, unlike most sibling List ops in this service which default to
// CreationTime.
func lessHPTrainingJob(a, b *TrainingJob, sortBy string) bool {
	switch sortBy {
	case keyStatus:
		return a.TrainingJobStatus < b.TrainingJobStatus
	case keyCreationTime:
		if a.CreationTime.Equal(b.CreationTime) {
			return a.TrainingJobName < b.TrainingJobName
		}

		return a.CreationTime.Before(b.CreationTime)
	default:
		return a.TrainingJobName < b.TrainingJobName
	}
}

// ListTrainingJobsForHyperParameterTuningJob returns training jobs for an HP
// tuning job matching filter, sorted by filter.SortBy (default Name) /
// filter.SortOrder (default Ascending).
func (b *InMemoryBackend) ListTrainingJobsForHyperParameterTuningJob(
	ctx context.Context,
	jobName, nextToken string,
	filter ListTrainingJobsForHyperParameterTuningJobFilter,
) ([]*TrainingJob, string, error) {
	b.mu.RLock("ListTrainingJobsForHyperParameterTuningJob")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	tuningJob, ok := b.hpTuningJobsStoreRO(region).Get(jobName)
	if !ok {
		return nil, "", fmt.Errorf("%w: HP tuning job %q not found", ErrHPTuningJobNotFound, jobName)
	}

	if filter.SortBy == "FinalObjectiveMetricValue" {
		return []*TrainingJob{}, "", nil
	}

	prefix := jobName + "-"
	out := make([]*TrainingJob, 0)

	for _, tj := range b.trainingJobsStoreRO(region).All() {
		if tj.TuningJobArn != tuningJob.HyperParameterTuningJobArn && !strings.HasPrefix(tj.TrainingJobName, prefix) {
			continue
		}

		if filter.StatusEquals != "" && tj.TrainingJobStatus != filter.StatusEquals {
			continue
		}

		out = append(out, cloneTrainingJob(tj))
	}

	desc := strings.EqualFold(filter.SortOrder, "Descending")
	sort.Slice(out, func(i, k int) bool {
		less := lessHPTrainingJob(out[i], out[k], filter.SortBy)
		if desc {
			return !less
		}

		return less
	})

	page, next := paginateSlice(out, nextToken, filter.MaxResults)

	return page, next, nil
}
