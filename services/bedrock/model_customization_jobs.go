package bedrock

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// newCustomizationJobID generates a unique customization job ID.
func (b *InMemoryBackend) newCustomizationJobID() string {
	b.customizationJobCounter++

	return fmt.Sprintf("cj-%07d", b.customizationJobCounter)
}

// CreateModelCustomizationJob creates a new model customization job.
// customModelName is the required name of the resulting output model
// (bedrock@v1.66.4 CreateModelCustomizationJobRequest: "customModelName" is
// required and distinct from jobName). It is validated and reserved now so
// AdvanceCustomizationJobStatuses can materialize the output CustomModel
// without discovering a name conflict after the job has already committed to
// running.
//
// roleArn, outputDataConfig and trainingDataConfig are also required members
// (api_op_CreateModelCustomizationJob.go:66,75,80). outputDataConfig.S3Uri is
// itself required within OutputDataConfig; TrainingDataConfig's own leaves
// (S3Uri, InvocationLogsConfig) are not required by the SDK, only the
// TrainingDataConfig object itself, so no leaf-level check is made there.
func (b *InMemoryBackend) CreateModelCustomizationJob(
	jobName, customModelName, baseModelID, customizationType, roleArn string,
	outputDataConfig OutputDataConfig, trainingDataConfig TrainingDataConfig,
	validatorS3Uris []string,
	tags []Tag,
) (*ModelCustomizationJob, error) {
	b.mu.Lock("CreateModelCustomizationJob")
	defer b.mu.Unlock()

	if jobName == "" {
		return nil, fmt.Errorf("%w: jobName is required", ErrValidation)
	}

	if customModelName == "" {
		return nil, fmt.Errorf("%w: customModelName is required", ErrValidation)
	}

	if roleArn == "" {
		return nil, fmt.Errorf("%w: roleArn is required", ErrValidation)
	}

	if outputDataConfig.S3Uri == "" {
		return nil, fmt.Errorf("%w: outputDataConfig.s3Uri is required", ErrValidation)
	}

	if _, exists := b.customizationJobsByName[jobName]; exists {
		return nil, fmt.Errorf("%w: customization job %s already exists", ErrAlreadyExists, jobName)
	}

	if _, exists := b.customModelsByName[customModelName]; exists {
		return nil, fmt.Errorf("%w: custom model %s already exists", ErrAlreadyExists, customModelName)
	}

	for _, j := range b.modelCustomizationJobs.All() {
		if j.CustomModelName == customModelName {
			return nil, fmt.Errorf("%w: custom model %s already exists", ErrAlreadyExists, customModelName)
		}
	}

	id := b.newCustomizationJobID()
	jobARN := arn.Build("bedrock", b.region, b.accountID, "model-customization-job/"+id)
	outputModelARN := arn.Build("bedrock", b.region, b.accountID, "custom-model/output-"+id)
	baseModelARN := foundationModelARN(b.region, baseModelID)
	now := time.Now().UTC()

	var baseModelName string
	if fm := b.findFoundationModelByID(baseModelID); fm != nil {
		baseModelName = fm.ModelName
	}

	job := &ModelCustomizationJob{
		JobArn:             jobARN,
		JobName:            jobName,
		BaseModelArn:       baseModelARN,
		BaseModelName:      baseModelName,
		OutputModelArn:     outputModelARN,
		CustomModelName:    customModelName,
		Status:             statusInProgress,
		CustomizationType:  customizationType,
		RoleArn:            roleArn,
		OutputDataConfig:   outputDataConfig,
		TrainingDataConfig: trainingDataConfig,
		ValidatorS3Uris:    validatorS3Uris,
		CreationTime:       now,
		LastModifiedTime:   now,
		Tags:               copyTags(tags),
	}
	b.modelCustomizationJobs.Put(job)
	b.customizationJobsByName[jobName] = jobARN
	cp := *job
	cp.Tags = copyTags(job.Tags)

	return &cp, nil
}

// findCustomizationJobARN resolves a job ID or name to its ARN.
// Caller must hold at least a read lock.
func (b *InMemoryBackend) findCustomizationJobARN(idOrARN string) (string, bool) {
	if _, ok := b.modelCustomizationJobs.Get(idOrARN); ok {
		return idOrARN, true
	}

	if a := b.customizationJobsByName[idOrARN]; a != "" {
		return a, true
	}

	return "", false
}

// GetModelCustomizationJob returns a model customization job by ARN or name.
func (b *InMemoryBackend) GetModelCustomizationJob(idOrARN string) (*ModelCustomizationJob, error) {
	b.mu.RLock("GetModelCustomizationJob")
	defer b.mu.RUnlock()

	jobARN, ok := b.findCustomizationJobARN(idOrARN)
	if !ok {
		return nil, fmt.Errorf("%w: model customization job %s not found", ErrNotFound, idOrARN)
	}

	j, _ := b.modelCustomizationJobs.Get(jobARN)
	cp := *j
	cp.Tags = copyTags(j.Tags)

	return &cp, nil
}

// ListModelCustomizationJobs returns customization jobs matching in's filters,
// sorted and paginated. in may be nil, matching an unfiltered call.
// Structurally similar to ListEvaluationJobs/ListModelInvocationJobs (same
// filter/sort/paginate shape) but over a distinct resource type and filter
// set; see matchesCustomizationJobFilter.
//
//nolint:dupl // see doc comment above.
func (b *InMemoryBackend) ListModelCustomizationJobs(
	in *ListModelCustomizationJobsInput,
) ([]*ModelCustomizationJob, string) {
	b.mu.RLock("ListModelCustomizationJobs")
	defer b.mu.RUnlock()

	list := make([]*ModelCustomizationJob, 0, b.modelCustomizationJobs.Len())

	for _, j := range b.modelCustomizationJobs.All() {
		if !matchesCustomizationJobFilter(j, in) {
			continue
		}

		cp := *j
		cp.Tags = copyTags(j.Tags)
		list = append(list, &cp)
	}

	descending := in != nil && in.SortOrder == sortOrderDescending
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreationTime.Equal(list[j].CreationTime) {
			if descending {
				return list[i].CreationTime.After(list[j].CreationTime)
			}

			return list[i].CreationTime.Before(list[j].CreationTime)
		}

		return list[i].JobArn < list[j].JobArn
	})

	maxResults, nextToken := 0, ""
	if in != nil {
		maxResults, nextToken = int(in.MaxResults), in.NextToken
	}

	return paginate(list, maxResults, nextToken)
}

// matchesCustomizationJobFilter reports whether a job satisfies the list
// filters (statusEquals, nameContains, creationTimeAfter/Before).
func matchesCustomizationJobFilter(j *ModelCustomizationJob, in *ListModelCustomizationJobsInput) bool {
	if in == nil {
		return true
	}
	if in.StatusEquals != "" && j.Status != in.StatusEquals {
		return false
	}
	if in.NameContains != "" && !containsIgnoreCase(j.JobName, in.NameContains) {
		return false
	}
	if in.CreationTimeAfter != nil && !j.CreationTime.After(*in.CreationTimeAfter) {
		return false
	}
	if in.CreationTimeBefore != nil && !j.CreationTime.Before(*in.CreationTimeBefore) {
		return false
	}

	return true
}

// StopModelCustomizationJob stops a running customization job.
func (b *InMemoryBackend) StopModelCustomizationJob(idOrARN string) error {
	b.mu.Lock("StopModelCustomizationJob")
	defer b.mu.Unlock()

	jobARN, ok := b.findCustomizationJobARN(idOrARN)
	if !ok {
		return fmt.Errorf("%w: model customization job %s not found", ErrNotFound, idOrARN)
	}

	j, _ := b.modelCustomizationJobs.Get(jobARN)
	j.Status = statusStopped

	return nil
}

// AdvanceCustomizationJobStatuses moves InProgress customization jobs to Completed.
// Called by the janitor after the simulated training delay has elapsed.
func (b *InMemoryBackend) AdvanceCustomizationJobStatuses(minAge time.Duration) int {
	b.mu.Lock("AdvanceCustomizationJobStatuses")
	defer b.mu.Unlock()

	now := time.Now().UTC()
	advanced := 0

	for _, job := range b.modelCustomizationJobs.All() {
		if job.Status != statusInProgress {
			continue
		}

		if now.Sub(job.CreationTime) >= minAge {
			job.Status = statusCompleted
			job.LastModifiedTime = now
			job.EndTime = now
			b.materializeCustomizationOutputModel(job, now)
			advanced++
		}
	}

	return advanced
}

// materializeCustomizationOutputModel creates the CustomModel record for a
// job's output once it completes -- until now nothing ever populated
// b.customModels for a job's OutputModelArn, so it was never listed or
// gettable. Caller must hold the write lock.
func (b *InMemoryBackend) materializeCustomizationOutputModel(job *ModelCustomizationJob, now time.Time) {
	model := &CustomModel{
		ModelArn:          job.OutputModelArn,
		ModelName:         job.CustomModelName,
		ModelStatus:       statusActive,
		BaseModelArn:      job.BaseModelArn,
		BaseModelName:     job.BaseModelName,
		CustomizationType: job.CustomizationType,
		JobArn:            job.JobArn,
		JobName:           job.JobName,
		CreationTime:      now,
	}
	b.customModels.Put(model)
	b.customModelsByName[job.CustomModelName] = job.OutputModelArn
}
