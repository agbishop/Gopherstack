package bedrock

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// newEvaluationJobID generates a unique evaluation job ID.
func (b *InMemoryBackend) newEvaluationJobID() string {
	b.evaluationJobCounter++

	return fmt.Sprintf("eval-job-%07d", b.evaluationJobCounter)
}

// CreateEvaluationJob creates a new evaluation job.
func (b *InMemoryBackend) CreateEvaluationJob(
	name string,
	tags []Tag,
	opts ...*CreateEvaluationJobInput,
) (*EvaluationJob, error) {
	b.mu.Lock("CreateEvaluationJob")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: jobName is required", ErrValidation)
	}

	if _, exists := b.evaluationJobsByName[name]; exists {
		return nil, fmt.Errorf("%w: evaluation job %s already exists", ErrAlreadyExists, name)
	}

	id := b.newEvaluationJobID()
	jobARN := arn.Build("bedrock", b.region, b.accountID, "evaluation-job/"+id)
	now := time.Now().UTC()

	job := &EvaluationJob{
		JobArn:           jobARN,
		JobName:          name,
		Status:           statusInProgress,
		CreationTime:     now,
		LastModifiedTime: now,
		Tags:             copyTags(tags),
	}

	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		job.JobDescription = opt.JobDescription
		job.RoleArn = opt.RoleArn
		job.ApplicationType = opt.ApplicationType
		job.InferenceConfig = opt.InferenceConfig
		job.EvaluationConfig = opt.EvaluationConfig
		job.OutputDataConfig = opt.OutputDataConfig
	}

	b.evaluationJobs.Put(job)
	b.evaluationJobsByName[name] = jobARN
	cp := *job
	cp.Tags = copyTags(job.Tags)

	return &cp, nil
}

// BatchDeleteEvaluationJob deletes multiple evaluation jobs.
func (b *InMemoryBackend) BatchDeleteEvaluationJob(jobARNs []string) (
	[]BatchDeleteEvaluationJobError, []BatchDeleteEvaluationJobItem, error,
) {
	b.mu.Lock("BatchDeleteEvaluationJob")
	defer b.mu.Unlock()

	if len(jobARNs) == 0 {
		return nil, nil, fmt.Errorf("%w: jobIdentifiers must not be empty", ErrValidation)
	}

	var errs []BatchDeleteEvaluationJobError

	deleted := make([]BatchDeleteEvaluationJobItem, 0, len(jobARNs))

	for _, jobARN := range jobARNs {
		job, ok := b.evaluationJobs.Get(jobARN)
		if !ok {
			errs = append(errs, BatchDeleteEvaluationJobError{
				JobARN:  jobARN,
				Code:    "ResourceNotFoundException",
				Message: fmt.Sprintf("evaluation job %s not found", jobARN),
			})

			continue
		}

		if job.Status == statusInProgress {
			errs = append(errs, BatchDeleteEvaluationJobError{
				JobARN:  jobARN,
				Code:    "ValidationException",
				Message: fmt.Sprintf("evaluation job %s cannot be deleted while in status %s", jobARN, job.Status),
			})

			continue
		}

		delete(b.evaluationJobsByName, job.JobName)
		b.evaluationJobs.Delete(jobARN)
		deleted = append(deleted, BatchDeleteEvaluationJobItem{
			JobARN: jobARN,
			Status: "Deleting",
		})
	}

	if errs == nil {
		errs = []BatchDeleteEvaluationJobError{}
	}

	if deleted == nil {
		deleted = []BatchDeleteEvaluationJobItem{}
	}

	return errs, deleted, nil
}

// GetEvaluationJob returns a single evaluation job by ARN.
func (b *InMemoryBackend) GetEvaluationJob(jobARN string) (*EvaluationJob, error) {
	b.mu.RLock("GetEvaluationJob")
	defer b.mu.RUnlock()

	job, ok := b.evaluationJobs.Get(jobARN)
	if !ok {
		return nil, fmt.Errorf("%w: evaluation job %s not found", ErrNotFound, jobARN)
	}

	cp := *job
	cp.Tags = copyTags(job.Tags)

	return &cp, nil
}

// ListEvaluationJobs returns evaluation jobs matching the given filters, sorted
// and paginated. in may be nil, matching an unfiltered ListEvaluationJobs call.
// Structurally similar to ListModelInvocationJobs (same filter/sort/paginate
// shape) but over a distinct resource type and filter set; see
// matchesEvaluationJobFilter.
//
//nolint:dupl // see doc comment above.
func (b *InMemoryBackend) ListEvaluationJobs(in *ListEvaluationJobsInput) ([]*EvaluationJob, string) {
	b.mu.RLock("ListEvaluationJobs")
	defer b.mu.RUnlock()

	jobs := make([]*EvaluationJob, 0, b.evaluationJobs.Len())
	for _, j := range b.evaluationJobs.All() {
		if !matchesEvaluationJobFilter(j, in) {
			continue
		}

		cp := *j
		cp.Tags = copyTags(j.Tags)
		jobs = append(jobs, &cp)
	}

	descending := in != nil && in.SortOrder == sortOrderDescending
	sort.Slice(jobs, func(i, k int) bool {
		if !jobs[i].CreationTime.Equal(jobs[k].CreationTime) {
			if descending {
				return jobs[i].CreationTime.After(jobs[k].CreationTime)
			}

			return jobs[i].CreationTime.Before(jobs[k].CreationTime)
		}

		return jobs[i].JobArn < jobs[k].JobArn
	})

	maxResults, nextToken := 0, ""
	if in != nil {
		maxResults, nextToken = int(in.MaxResults), in.NextToken
	}

	return paginate(jobs, maxResults, nextToken)
}

// matchesEvaluationJobFilter reports whether an evaluation job satisfies the
// list filters (statusEquals, applicationTypeEquals, nameContains,
// creationTimeAfter/Before).
func matchesEvaluationJobFilter(j *EvaluationJob, in *ListEvaluationJobsInput) bool {
	if in == nil {
		return true
	}
	if in.StatusEquals != "" && j.Status != in.StatusEquals {
		return false
	}
	if in.ApplicationTypeEquals != "" && j.ApplicationType != in.ApplicationTypeEquals {
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

// StopEvaluationJob marks an evaluation job as stopped.
func (b *InMemoryBackend) StopEvaluationJob(jobARN string) error {
	b.mu.Lock("StopEvaluationJob")
	defer b.mu.Unlock()

	job, ok := b.evaluationJobs.Get(jobARN)
	if !ok {
		return fmt.Errorf("%w: evaluation job %s not found", ErrNotFound, jobARN)
	}

	if job.Status != statusInProgress {
		return fmt.Errorf(
			"%w: evaluation job %s cannot be stopped in status %s",
			ErrValidation,
			jobARN,
			job.Status,
		)
	}

	job.Status = statusStopped
	job.LastModifiedTime = time.Now().UTC()

	return nil
}
