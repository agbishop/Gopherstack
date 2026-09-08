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
	// ErrTrainingJobNotFound is returned when a training job does not exist.
	ErrTrainingJobNotFound = awserr.New("ResourceNotFound", ErrResourceNotFound)
	// ErrTrainingJobAlreadyExists is returned when a training job already exists.
	ErrTrainingJobAlreadyExists = awserr.New("ResourceInUse", awserr.ErrConflict)
)

type TrainingJob struct {
	LastModifiedTime                      time.Time                   `json:"LastModifiedTime"`
	CreationTime                          time.Time                   `json:"CreationTime"`
	VpcConfig                             *VpcConfig                  `json:"VpcConfig,omitempty"`
	TrainingStartTime                     *time.Time                  `json:"TrainingStartTime,omitempty"`
	TrainingEndTime                       *time.Time                  `json:"TrainingEndTime,omitempty"`
	Tags                                  map[string]string           `json:"Tags,omitempty"`
	HyperParameters                       map[string]string           `json:"HyperParameters,omitempty"`
	Environment                           map[string]string           `json:"Environment,omitempty"`
	ModelArtifacts                        *ModelArtifacts             `json:"ModelArtifacts,omitempty"`
	CheckpointConfig                      *CheckpointConfig           `json:"CheckpointConfig,omitempty"`
	OutputDataConfig                      OutputDataConfig            `json:"OutputDataConfig"`
	SecondaryStatus                       string                      `json:"SecondaryStatus,omitempty"`
	FailureReason                         string                      `json:"FailureReason,omitempty"`
	TrainingJobName                       string                      `json:"TrainingJobName"`
	TrainingJobArn                        string                      `json:"TrainingJobArn"`
	TuningJobArn                          string                      `json:"TuningJobArn,omitempty"`
	RoleArn                               string                      `json:"RoleArn,omitempty"`
	TrainingJobStatus                     string                      `json:"TrainingJobStatus"`
	InputDataConfig                       []Channel                   `json:"InputDataConfig,omitempty"`
	SecondaryStatusTransitions            []SecondaryStatusTransition `json:"SecondaryStatusTransitions,omitempty"`
	AlgorithmSpecification                AlgorithmSpecification      `json:"AlgorithmSpecification"`
	ResourceConfig                        ResourceConfig              `json:"ResourceConfig"`
	StoppingCondition                     StoppingCondition           `json:"StoppingCondition"`
	BillableTimeInSeconds                 int32                       `json:"BillableTimeInSeconds,omitempty"`
	TrainingTimeInSeconds                 int32                       `json:"TrainingTimeInSeconds,omitempty"`
	EnableNetworkIsolation                bool                        `json:"EnableNetworkIsolation,omitempty"`
	EnableManagedSpotTraining             bool                        `json:"EnableManagedSpotTraining,omitempty"`
	EnableInterContainerTrafficEncryption bool                        `json:"EnableInterContainerTrafficEncryption,omitempty"` //nolint:lll // AWS API field name exceeds 120 chars; cannot be shortened
}

// cloneTrainingJob returns a deep copy of tj.
func cloneTrainingJob(tj *TrainingJob) *TrainingJob {
	cp := *tj
	cp.Tags = maps.Clone(tj.Tags)
	cp.HyperParameters = maps.Clone(tj.HyperParameters)
	cp.Environment = maps.Clone(tj.Environment)
	cp.InputDataConfig = make([]Channel, len(tj.InputDataConfig))
	copy(cp.InputDataConfig, tj.InputDataConfig)
	cp.SecondaryStatusTransitions = make(
		[]SecondaryStatusTransition,
		len(tj.SecondaryStatusTransitions),
	)
	copy(cp.SecondaryStatusTransitions, tj.SecondaryStatusTransitions)
	if tj.VpcConfig != nil {
		vpc := *tj.VpcConfig
		vpc.SecurityGroupIDs = append([]string(nil), tj.VpcConfig.SecurityGroupIDs...)
		vpc.Subnets = append([]string(nil), tj.VpcConfig.Subnets...)
		cp.VpcConfig = &vpc
	}
	if tj.CheckpointConfig != nil {
		cc := *tj.CheckpointConfig
		cp.CheckpointConfig = &cc
	}
	if tj.ModelArtifacts != nil {
		ma := *tj.ModelArtifacts
		cp.ModelArtifacts = &ma
	}

	return &cp
}

// NotebookInstance represents a SageMaker notebook instance.

// ---------------------------------------------------------------------------
// TrainingJob
// ---------------------------------------------------------------------------

// CreateTrainingJob creates a new training job (legacy signature, kept for compatibility).
func (b *InMemoryBackend) CreateTrainingJob(
	ctx context.Context,
	name, roleArn string,
	algorithmSpec map[string]string,
	tags map[string]string,
) (*TrainingJob, error) {
	spec := AlgorithmSpecification{
		TrainingImage:     algorithmSpec["TrainingImage"],
		AlgorithmName:     algorithmSpec["AlgorithmName"],
		TrainingInputMode: algorithmSpec["TrainingInputMode"],
	}

	return b.CreateTrainingJobFull(ctx, TrainingJobOptions{
		TrainingJobName:        name,
		RoleArn:                roleArn,
		AlgorithmSpecification: spec,
		Tags:                   tags,
	})
}

// DescribeTrainingJob returns a training job by name.
func (b *InMemoryBackend) DescribeTrainingJob(ctx context.Context, name string) (*TrainingJob, error) {
	b.mu.RLock("DescribeTrainingJob")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	tj, ok := b.trainingJobsStoreRO(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: training job %q not found", ErrTrainingJobNotFound, name)
	}

	return cloneTrainingJob(tj), nil
}

// ListTrainingJobs returns training jobs sorted by name with optional pagination.
func (b *InMemoryBackend) ListTrainingJobs(ctx context.Context, nextToken string) ([]*TrainingJob, string) {
	b.mu.RLock("ListTrainingJobs")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	return sagemakerListPaged(b.trainingJobsStoreRO(region), nextToken, cloneTrainingJob,
		func(a, b *TrainingJob) bool { return a.TrainingJobName < b.TrainingJobName })
}

// StopTrainingJob marks a training job as Stopping.
func (b *InMemoryBackend) StopTrainingJob(ctx context.Context, name string) error {
	b.mu.Lock("StopTrainingJob")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	tj, ok := b.trainingJobsStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: training job %q not found", ErrTrainingJobNotFound, name)
	}

	tj.TrainingJobStatus = pipelineStatusStopping
	tj.LastModifiedTime = time.Now()

	return nil
}

// DeleteTrainingJob removes a training job from the backend.
func (b *InMemoryBackend) DeleteTrainingJob(ctx context.Context, name string) error {
	b.mu.Lock("DeleteTrainingJob")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	tj, ok := b.trainingJobsStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: training job %q not found", ErrTrainingJobNotFound, name)
	}

	if tj.TrainingJobStatus == trainingJobStatusInProgress || tj.TrainingJobStatus == notebookStatusStopping {
		return fmt.Errorf(
			"%w: training job %q cannot be deleted while status is %q",
			ErrValidation, name, tj.TrainingJobStatus,
		)
	}

	arnIdx := b.trainingJobARNIndexStore(region)
	delete(arnIdx, tj.TrainingJobArn)
	store := b.trainingJobsStore(region)
	store.Delete(name)

	return nil
}

// ---------------------------------------------------------------------------
// Training job lifecycle FSM + expanded struct (#4, #5, #6)
// ---------------------------------------------------------------------------

// AlgorithmSpecification is the typed algorithm spec for a training job.
type AlgorithmSpecification struct {
	TrainingImage                    string             `json:"TrainingImage,omitempty"`
	AlgorithmName                    string             `json:"AlgorithmName,omitempty"`
	TrainingInputMode                string             `json:"TrainingInputMode,omitempty"`
	MetricDefinitions                []MetricDefinition `json:"MetricDefinitions,omitempty"`
	ContainerEntrypoint              []string           `json:"ContainerEntrypoint,omitempty"`
	ContainerArguments               []string           `json:"ContainerArguments,omitempty"`
	EnableSageMakerMetricsTimeSeries bool               `json:"EnableSageMakerMetricsTimeSeries,omitempty"`
}

// MetricDefinition maps a metric name to a regex.
type MetricDefinition struct {
	Name  string `json:"Name"`
	Regex string `json:"Regex,omitempty"`
}

// ChannelDataSource holds either an S3 or file system data source.
type ChannelDataSource struct {
	S3DataSource *S3DataSource `json:"S3DataSource,omitempty"`
}

// S3DataSource references an S3 location for training data.
type S3DataSource struct {
	S3Uri                  string `json:"S3Uri"`
	S3DataType             string `json:"S3DataType,omitempty"`
	S3DataDistributionType string `json:"S3DataDistributionType,omitempty"`
}

// Channel is one input data channel for a training job.
type Channel struct {
	DataSource        ChannelDataSource `json:"DataSource"`
	ChannelName       string            `json:"ChannelName"`
	ContentType       string            `json:"ContentType,omitempty"`
	CompressionType   string            `json:"CompressionType,omitempty"`
	RecordWrapperType string            `json:"RecordWrapperType,omitempty"`
	InputMode         string            `json:"InputMode,omitempty"`
}

// OutputDataConfig specifies where training output is stored.
type OutputDataConfig struct {
	S3OutputPath    string `json:"S3OutputPath"`
	KmsKeyID        string `json:"KmsKeyId,omitempty"`
	CompressionType string `json:"CompressionType,omitempty"`
}

// ResourceConfig specifies compute resources for a training job.
type ResourceConfig struct {
	InstanceType             string          `json:"InstanceType"`
	VolumeKmsKeyID           string          `json:"VolumeKmsKeyId,omitempty"`
	InstanceGroups           []InstanceGroup `json:"InstanceGroups,omitempty"`
	InstanceCount            int32           `json:"InstanceCount"`
	VolumeSizeInGB           int32           `json:"VolumeSizeInGB"`
	KeepAlivePeriodInSeconds int32           `json:"KeepAlivePeriodInSeconds,omitempty"`
}

// InstanceGroup is a heterogeneous instance group in a training job.
type InstanceGroup struct {
	InstanceGroupName string `json:"InstanceGroupName"`
	InstanceType      string `json:"InstanceType"`
	InstanceCount     int32  `json:"InstanceCount"`
}

// StoppingCondition defines the maximum run time for a training job.
type StoppingCondition struct {
	MaxRuntimeInSeconds     int32 `json:"MaxRuntimeInSeconds,omitempty"`
	MaxWaitTimeInSeconds    int32 `json:"MaxWaitTimeInSeconds,omitempty"`
	MaxPendingTimeInSeconds int32 `json:"MaxPendingTimeInSeconds,omitempty"`
}

// VpcConfig specifies the VPC subnets and security groups.
type VpcConfig struct {
	SecurityGroupIDs []string `json:"SecurityGroupIds,omitempty"`
	Subnets          []string `json:"Subnets,omitempty"`
}

// CheckpointConfig stores checkpoint location for managed spot.
type CheckpointConfig struct {
	S3Uri     string `json:"S3Uri"`
	LocalPath string `json:"LocalPath,omitempty"`
}

// ModelArtifacts references the S3 model output of a training job.
type ModelArtifacts struct {
	S3ModelArtifacts string `json:"S3ModelArtifacts"`
}

// SecondaryStatusTransition records a FSM step in a training job.
type SecondaryStatusTransition struct {
	StartTime     time.Time  `json:"StartTime"`
	EndTime       *time.Time `json:"EndTime,omitempty"`
	Status        string     `json:"Status"`
	StatusMessage string     `json:"StatusMessage,omitempty"`
}

// TrainingJobOptions holds all fields for CreateTrainingJob.
type TrainingJobOptions struct {
	Tags                                  map[string]string      `json:"Tags,omitempty"`
	Environment                           map[string]string      `json:"Environment,omitempty"`
	HyperParameters                       map[string]string      `json:"HyperParameters,omitempty"`
	CheckpointConfig                      *CheckpointConfig      `json:"CheckpointConfig,omitempty"`
	VpcConfig                             *VpcConfig             `json:"VpcConfig,omitempty"`
	OutputDataConfig                      OutputDataConfig       `json:"OutputDataConfig"`
	TrainingJobName                       string                 `json:"TrainingJobName"`
	RoleArn                               string                 `json:"RoleArn"`
	InputDataConfig                       []Channel              `json:"InputDataConfig,omitempty"`
	AlgorithmSpecification                AlgorithmSpecification `json:"AlgorithmSpecification"`
	ResourceConfig                        ResourceConfig         `json:"ResourceConfig"`
	StoppingCondition                     StoppingCondition      `json:"StoppingCondition"`
	EnableNetworkIsolation                bool                   `json:"EnableNetworkIsolation,omitempty"`
	EnableManagedSpotTraining             bool                   `json:"EnableManagedSpotTraining,omitempty"`
	EnableInterContainerTrafficEncryption bool                   `json:"EnableInterContainerTrafficEncryption,omitempty"`
}

// CreateTrainingJobFull creates a training job from a full options struct
// and schedules InProgress → Completed after a short delay.
func (b *InMemoryBackend) CreateTrainingJobFull(ctx context.Context, opts TrainingJobOptions) (*TrainingJob, error) {
	b.mu.Lock("CreateTrainingJobFull")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.trainingJobsStore(region).Get(opts.TrainingJobName); ok {
		return nil, fmt.Errorf(
			"%w: training job %s already exists",
			ErrTrainingJobAlreadyExists,
			opts.TrainingJobName,
		)
	}

	jobARN := arn.Build("sagemaker", region, b.accountID, "training-job/"+opts.TrainingJobName)
	now := time.Now()

	tj := &TrainingJob{
		TrainingJobName:                       opts.TrainingJobName,
		TrainingJobArn:                        jobARN,
		TrainingJobStatus:                     trainingJobStatusInProgress,
		SecondaryStatus:                       secondaryStatusStarting,
		RoleArn:                               opts.RoleArn,
		AlgorithmSpecification:                opts.AlgorithmSpecification,
		InputDataConfig:                       opts.InputDataConfig,
		OutputDataConfig:                      opts.OutputDataConfig,
		ResourceConfig:                        opts.ResourceConfig,
		StoppingCondition:                     opts.StoppingCondition,
		VpcConfig:                             opts.VpcConfig,
		CheckpointConfig:                      opts.CheckpointConfig,
		HyperParameters:                       maps.Clone(opts.HyperParameters),
		Environment:                           maps.Clone(opts.Environment),
		CreationTime:                          now,
		LastModifiedTime:                      now,
		TrainingStartTime:                     &now,
		Tags:                                  mergeTags(nil, opts.Tags),
		EnableNetworkIsolation:                opts.EnableNetworkIsolation,
		EnableManagedSpotTraining:             opts.EnableManagedSpotTraining,
		EnableInterContainerTrafficEncryption: opts.EnableInterContainerTrafficEncryption,
		SecondaryStatusTransitions: []SecondaryStatusTransition{
			{StartTime: now, Status: secondaryStatusStarting, StatusMessage: "Launching requested ML instances"},
		},
	}
	b.trainingJobsStore(region).Put(tj)
	b.trainingJobARNIndexStore(region)[jobARN] = opts.TrainingJobName

	b.scheduleTrainingCompletion(b.lifecycleCtx, region, opts.TrainingJobName)

	return cloneTrainingJob(tj), nil
}

// scheduleTrainingCompletion drives InProgress → Completed after delay.
// ctx must be b.lifecycleCtx captured by the caller while holding b.mu.
// region must be captured by the caller before the lock is released.
func (b *InMemoryBackend) scheduleTrainingCompletion(ctx context.Context, region, name string) {
	b.runDelayed(ctx, trainingInProgressToCompleted, func() {
		b.mu.Lock("scheduleTrainingCompletion.goroutine")
		defer b.mu.Unlock()

		tj, ok := b.trainingJobsStore(region).Get(name)
		if !ok {
			return
		}

		if tj.TrainingJobStatus != trainingJobStatusInProgress {
			return
		}

		now := time.Now()
		tj.TrainingJobStatus = algorithmStatusCompleted
		tj.SecondaryStatus = algorithmStatusCompleted
		tj.TrainingEndTime = &now
		tj.LastModifiedTime = now
		billable := max(int32(trainingInProgressToCompleted.Seconds()), 1)
		tj.BillableTimeInSeconds = billable
		tj.TrainingTimeInSeconds = billable
		tj.ModelArtifacts = &ModelArtifacts{
			S3ModelArtifacts: "s3://" + name + "-output/output/model.tar.gz",
		}
		if tj.OutputDataConfig.S3OutputPath != "" {
			tj.ModelArtifacts.S3ModelArtifacts = tj.OutputDataConfig.S3OutputPath + "/output/model.tar.gz"
		}

		tj.SecondaryStatusTransitions = append(
			tj.SecondaryStatusTransitions,
			SecondaryStatusTransition{StartTime: now, EndTime: &now, Status: algorithmStatusCompleted},
		)
	})
}

// StopTrainingJobFSM transitions InProgress → Stopping → Stopped.
func (b *InMemoryBackend) StopTrainingJobFSM(ctx context.Context, name string) error {
	b.mu.Lock("StopTrainingJobFSM")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	tj, ok := b.trainingJobsStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: training job %q not found", ErrTrainingJobNotFound, name)
	}

	tj.TrainingJobStatus = pipelineStatusStopping
	tj.LastModifiedTime = time.Now()

	b.runDelayed(b.lifecycleCtx, trainingStoppingToStopped, func() {
		b.mu.Lock("StopTrainingJobFSM.goroutine")
		defer b.mu.Unlock()

		if tj2, ok2 := b.trainingJobsStore(region).Get(name); ok2 &&
			tj2.TrainingJobStatus == pipelineStatusStopping {
			tj2.TrainingJobStatus = pipelineStatusStopped
			tj2.LastModifiedTime = time.Now()
		}
	})

	return nil
}

// ListTrainingJobsFilter narrows and orders the results of ListTrainingJobs
// (api_op_ListTrainingJobs.go:33-84). The op's own doc states both real
// defaults explicitly: SortBy is CreationTime, SortOrder is Ascending.
// TrainingPlanArnEquals/WarmPoolStatusEquals are decoded for wire-shape
// fidelity but are disclosed no-ops: this backend never associates a
// TrainingJob with a training plan ARN or a warm-pool status, so no job can
// ever match a non-empty value of either — the correct answer given neither
// concept is modeled, not a silently-ignored filter.
type ListTrainingJobsFilter struct {
	CreationTimeAfter      *time.Time
	CreationTimeBefore     *time.Time
	LastModifiedTimeAfter  *time.Time
	LastModifiedTimeBefore *time.Time
	StatusEquals           string
	NameContains           string
	SortBy                 string
	SortOrder              string
	TrainingPlanArnEquals  string
	WarmPoolStatusEquals   string
	MaxResults             int32
}

// ListTrainingJobsFiltered returns training jobs matching filter, sorted by
// filter.SortBy (default CreationTime) / filter.SortOrder (default Ascending).
// trainingJobMatchesListFilter reports whether tj satisfies every set field
// of f (api_op_ListTrainingJobs.go:33-84's StatusEquals/NameContains/
// CreationTime*/LastModifiedTime* filters).
func trainingJobMatchesListFilter(tj *TrainingJob, f ListTrainingJobsFilter) bool {
	if f.StatusEquals != "" && !strings.EqualFold(tj.TrainingJobStatus, f.StatusEquals) {
		return false
	}

	if f.NameContains != "" && !strings.Contains(strings.ToLower(tj.TrainingJobName), strings.ToLower(f.NameContains)) {
		return false
	}

	if !timeWindowOK(tj.CreationTime, f.CreationTimeAfter, f.CreationTimeBefore) {
		return false
	}

	return timeWindowOK(tj.LastModifiedTime, f.LastModifiedTimeAfter, f.LastModifiedTimeBefore)
}

// lessTrainingJobBySortBy orders a before b by sortBy (Name/Status/default
// CreationTime, tie-broken by name).
func lessTrainingJobBySortBy(a, b *TrainingJob, sortBy string) bool {
	switch sortBy {
	case keyGenericName:
		return a.TrainingJobName < b.TrainingJobName
	case keyStatus:
		return a.TrainingJobStatus < b.TrainingJobStatus
	default:
		if a.CreationTime.Equal(b.CreationTime) {
			return a.TrainingJobName < b.TrainingJobName
		}

		return a.CreationTime.Before(b.CreationTime)
	}
}

func (b *InMemoryBackend) ListTrainingJobsFiltered(
	ctx context.Context,
	nextToken string,
	f ListTrainingJobsFilter,
) ([]*TrainingJob, string) {
	b.mu.RLock("ListTrainingJobsFiltered")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	if f.TrainingPlanArnEquals != "" || f.WarmPoolStatusEquals != "" {
		return []*TrainingJob{}, ""
	}

	list := make([]*TrainingJob, 0, b.trainingJobsStoreRO(region).Len())

	for _, tj := range b.trainingJobsStoreRO(region).All() {
		if trainingJobMatchesListFilter(tj, f) {
			list = append(list, cloneTrainingJob(tj))
		}
	}

	desc := strings.EqualFold(f.SortOrder, "Descending")
	sort.Slice(list, func(i, k int) bool {
		less := lessTrainingJobBySortBy(list[i], list[k], f.SortBy)
		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, nextToken, f.MaxResults)
}

// UpdateTrainingJobOptions holds the parameters for updating a training job
// (api_op_UpdateTrainingJob.go:29-56). ProfilerConfig/ProfilerRuleConfigurations/
// RemoteDebugConfig are disclosed not modeled: this backend's TrainingJob has
// no profiler or remote-debug concept at all (Create never captures them
// either), so there is nothing on the resource for an Update to mutate.
type UpdateTrainingJobOptions struct {
	KeepAlivePeriodInSeconds *int32
}

// UpdateTrainingJob applies a partial update to a training job's
// ResourceConfig.KeepAlivePeriodInSeconds, the only field of
// UpdateTrainingJobInput this backend's data model can honor.
func (b *InMemoryBackend) UpdateTrainingJob(
	ctx context.Context,
	name string,
	opts UpdateTrainingJobOptions,
) (*TrainingJob, error) {
	b.mu.Lock("UpdateTrainingJob")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	tj, ok := b.trainingJobsStore(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: training job %q not found", ErrTrainingJobNotFound, name)
	}

	if opts.KeepAlivePeriodInSeconds != nil {
		tj.ResourceConfig.KeepAlivePeriodInSeconds = *opts.KeepAlivePeriodInSeconds
	}

	tj.LastModifiedTime = time.Now()

	return cloneTrainingJob(tj), nil
}
