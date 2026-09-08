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

// ---------------------------------------------------------------------------
// TransformJob — gap #12 (partial)
// ---------------------------------------------------------------------------

var (
	// ErrTransformJobNotFound is returned when a transform job does not exist.
	ErrTransformJobNotFound = awserr.New("ResourceNotFound", ErrResourceNotFound)
	// ErrTransformJobAlreadyExists is returned when a transform job already exists.
	ErrTransformJobAlreadyExists = awserr.New("ResourceInUse", awserr.ErrConflict)
	// ErrTransformJobModelNotFound is returned when CreateTransformJob's ModelName
	// does not name an existing model. api_op_CreateTransformJob.go: "ModelName
	// must be the name of an existing Amazon SageMaker model"; ResourceNotFound is
	// modeled for CreateTransformJob (unlike CreateEndpointConfig, which has no
	// such error and is not checked here -- gopherstack-tauw).
	ErrTransformJobModelNotFound = awserr.New("ResourceNotFound", ErrResourceNotFound)
)

const (
	transformJobCompletionDelay = 300 * time.Millisecond
	transformJobStoppingDelay   = 150 * time.Millisecond
)

// TransformDataSource specifies the S3 input location for a transform job.
type TransformDataSource struct {
	S3DataSource TransformS3DataSource `json:"S3DataSource"`
}

// TransformS3DataSource is the S3-specific input for a batch transform job.
type TransformS3DataSource struct {
	S3Uri             string `json:"S3Uri"`
	S3DataType        string `json:"S3DataType,omitempty"`
	S3CompressionType string `json:"S3CompressionType,omitempty"`
}

// TransformInput specifies the input data location and format for a transform job.
type TransformInput struct {
	DataSource      TransformDataSource `json:"DataSource"`
	ContentType     string              `json:"ContentType,omitempty"`
	CompressionType string              `json:"CompressionType,omitempty"`
	SplitType       string              `json:"SplitType,omitempty"`
}

// TransformOutput specifies where to store transform results.
type TransformOutput struct {
	S3OutputPath string `json:"S3OutputPath"`
	Accept       string `json:"Accept,omitempty"`
	KmsKeyID     string `json:"KmsKeyId,omitempty"`
	AssembleWith string `json:"AssembleWith,omitempty"`
}

// TransformResources specifies compute resources for a transform job.
type TransformResources struct {
	InstanceType   string `json:"InstanceType"`
	VolumeKmsKeyID string `json:"VolumeKmsKeyId,omitempty"`
	InstanceCount  int32  `json:"InstanceCount"`
}

// TransformJob represents a SageMaker batch transform job.
type TransformJob struct {
	CreationTime            time.Time          `json:"CreationTime"`
	LastModifiedTime        time.Time          `json:"LastModifiedTime"`
	TransformStartTime      *time.Time         `json:"TransformStartTime,omitempty"`
	TransformEndTime        *time.Time         `json:"TransformEndTime,omitempty"`
	Tags                    map[string]string  `json:"Tags,omitempty"`
	Environment             map[string]string  `json:"Environment,omitempty"`
	TransformInput          TransformInput     `json:"TransformInput"`
	TransformOutput         TransformOutput    `json:"TransformOutput"`
	ModelName               string             `json:"ModelName,omitempty"`
	TransformJobName        string             `json:"TransformJobName"`
	TransformJobArn         string             `json:"TransformJobArn"`
	TransformJobStatus      string             `json:"TransformJobStatus"`
	BatchStrategy           string             `json:"BatchStrategy,omitempty"`
	FailureReason           string             `json:"FailureReason,omitempty"`
	TransformResources      TransformResources `json:"TransformResources"`
	MaxConcurrentTransforms int32              `json:"MaxConcurrentTransforms,omitempty"`
	MaxPayloadInMB          int32              `json:"MaxPayloadInMB,omitempty"`
}

// cloneTransformJob returns a deep copy of tj.
func cloneTransformJob(tj *TransformJob) *TransformJob {
	cp := *tj
	cp.Tags = maps.Clone(tj.Tags)
	cp.Environment = maps.Clone(tj.Environment)

	return &cp
}

// TransformJobOptions holds all input fields for CreateTransformJob.
type TransformJobOptions struct {
	Tags                    map[string]string
	Environment             map[string]string
	TransformInput          TransformInput
	TransformOutput         TransformOutput
	TransformJobName        string
	ModelName               string
	BatchStrategy           string
	TransformResources      TransformResources
	MaxConcurrentTransforms int32
	MaxPayloadInMB          int32
}

// CreateTransformJob creates a new batch transform job.
func (b *InMemoryBackend) CreateTransformJob(ctx context.Context, opts TransformJobOptions) (*TransformJob, error) {
	if opts.TransformJobName == "" {
		return nil, fmt.Errorf("%w: TransformJobName is required", ErrValidation)
	}
	if opts.ModelName == "" {
		return nil, fmt.Errorf("%w: ModelName is required", ErrValidation)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateTransformJob")
	defer b.mu.Unlock()

	if _, ok := b.modelsStore(region).Get(opts.ModelName); !ok {
		return nil, fmt.Errorf("%w: model %q does not exist", ErrTransformJobModelNotFound, opts.ModelName)
	}

	if _, ok := b.transformJobsStore(region).Get(opts.TransformJobName); ok {
		return nil, fmt.Errorf(
			"%w: transform job %s already exists",
			ErrTransformJobAlreadyExists,
			opts.TransformJobName,
		)
	}

	jobARN := arn.Build(
		"sagemaker",
		region,
		b.accountID,
		"transform-job/"+opts.TransformJobName,
	)
	now := time.Now()

	tj := &TransformJob{
		TransformJobName:        opts.TransformJobName,
		TransformJobArn:         jobARN,
		TransformJobStatus:      trainingJobStatusInProgress,
		ModelName:               opts.ModelName,
		BatchStrategy:           opts.BatchStrategy,
		MaxConcurrentTransforms: opts.MaxConcurrentTransforms,
		MaxPayloadInMB:          opts.MaxPayloadInMB,
		TransformInput:          opts.TransformInput,
		TransformOutput:         opts.TransformOutput,
		TransformResources:      opts.TransformResources,
		Tags:                    mergeTags(nil, opts.Tags),
		Environment:             maps.Clone(opts.Environment),
		CreationTime:            now,
		LastModifiedTime:        now,
	}
	b.transformJobsStore(region).Put(tj)
	b.transformJobARNIndexStore(region)[jobARN] = opts.TransformJobName

	b.runDelayed(b.lifecycleCtx, transformJobCompletionDelay, func() {
		b.applyTransformJobCompletion(ctx, opts.TransformJobName)
	})

	return cloneTransformJob(tj), nil
}

func (b *InMemoryBackend) applyTransformJobCompletion(ctx context.Context, name string) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("applyTransformJobCompletion")
	defer b.mu.Unlock()

	tj, ok := b.transformJobsStore(region).Get(name)
	if !ok || tj.TransformJobStatus != trainingJobStatusInProgress {
		return
	}

	now := time.Now()
	tj.TransformJobStatus = "Completed"
	tj.TransformEndTime = &now
	tj.LastModifiedTime = now
}

// DescribeTransformJob returns a transform job by name.
func (b *InMemoryBackend) DescribeTransformJob(ctx context.Context, name string) (*TransformJob, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeTransformJob")
	defer b.mu.RUnlock()

	tj, ok := b.transformJobsStoreRO(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: transform job %q not found", ErrTransformJobNotFound, name)
	}

	return cloneTransformJob(tj), nil
}

// StopTransformJob transitions a transform job to Stopping then Stopped.
func (b *InMemoryBackend) StopTransformJob(ctx context.Context, name string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("StopTransformJob")
	defer b.mu.Unlock()

	tj, ok := b.transformJobsStore(region).Get(name)
	if !ok {
		return fmt.Errorf("%w: transform job %q not found", ErrTransformJobNotFound, name)
	}
	if tj.TransformJobStatus != trainingJobStatusInProgress {
		return fmt.Errorf(
			"%w: transform job %q is not in InProgress state (status: %s)",
			ErrValidation,
			name,
			tj.TransformJobStatus,
		)
	}

	tj.TransformJobStatus = pipelineStatusStopping
	tj.LastModifiedTime = time.Now()

	b.runDelayed(b.lifecycleCtx, transformJobStoppingDelay, func() {
		b.mu.Lock("StopTransformJob.goroutine")
		defer b.mu.Unlock()
		if tj2, found := b.transformJobsStore(region).
			Get(name); found &&
			tj2.TransformJobStatus == pipelineStatusStopping {
			tj2.TransformJobStatus = pipelineStatusStopped
			tj2.LastModifiedTime = time.Now()
		}
	})

	return nil
}

// ListTransformJobsFilter narrows ListTransformJobs results, mirroring
// ListTransformJobsInput (api_op_ListTransformJobs.go). SortBy defaults to
// CreationTime, SortOrder defaults to Descending per that op's own doc.
type ListTransformJobsFilter struct {
	CreationTimeAfter      *time.Time
	CreationTimeBefore     *time.Time
	LastModifiedTimeAfter  *time.Time
	LastModifiedTimeBefore *time.Time
	StatusEquals           string
	NameContains           string
	SortBy                 string
	SortOrder              string
	MaxResults             int32
}

// ListTransformJobs returns transform jobs matching filter.
func (b *InMemoryBackend) ListTransformJobs(
	ctx context.Context,
	nextToken string,
	filter ListTransformJobsFilter,
) ([]*TransformJob, string) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListTransformJobs")
	defer b.mu.RUnlock()

	list := make([]*TransformJob, 0, b.transformJobsStoreRO(region).Len())

	for _, tj := range b.transformJobsStoreRO(region).All() {
		if filter.StatusEquals != "" && tj.TransformJobStatus != filter.StatusEquals {
			continue
		}

		if filter.NameContains != "" && !strings.Contains(tj.TransformJobName, filter.NameContains) {
			continue
		}

		if !timeWindowOK(tj.CreationTime, filter.CreationTimeAfter, filter.CreationTimeBefore) {
			continue
		}

		if !timeWindowOK(tj.LastModifiedTime, filter.LastModifiedTimeAfter, filter.LastModifiedTimeBefore) {
			continue
		}

		list = append(list, cloneTransformJob(tj))
	}

	desc := filter.SortOrder != sortOrderAscending
	sort.Slice(list, func(i, k int) bool {
		var less bool

		switch filter.SortBy {
		case keyGenericName:
			less = list[i].TransformJobName < list[k].TransformJobName
		case sortByStatus:
			less = list[i].TransformJobStatus < list[k].TransformJobStatus
		default:
			less = list[i].CreationTime.Before(list[k].CreationTime)
		}

		if desc {
			return !less
		}

		return less
	})

	return paginateSlice(list, nextToken, filter.MaxResults)
}
