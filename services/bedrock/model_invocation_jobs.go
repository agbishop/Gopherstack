package bedrock

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) newModelInvocationJobID() string {
	b.modelInvocationJobCounter++

	return "mij-" + strconv.Itoa(b.modelInvocationJobCounter)
}

// CreateModelInvocationJob creates a new batch model invocation job.
// AWS initial status is "Submitted" (not InProgress).
func (b *InMemoryBackend) CreateModelInvocationJob(
	name string,
	tags []Tag,
	opts ...*CreateModelInvocationJobInput,
) (*ModelInvocationJob, error) {
	b.mu.Lock("CreateModelInvocationJob")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: jobName is required", ErrValidation)
	}

	for _, j := range b.modelInvocationJobs.All() {
		if j.JobName == name {
			return nil, fmt.Errorf("%w: model invocation job %s already exists", ErrAlreadyExists, name)
		}
	}

	id := b.newModelInvocationJobID()
	jobARN := arn.Build("bedrock", b.region, b.accountID, "model-invocation-job/"+id)
	now := time.Now().UTC()

	job := &ModelInvocationJob{
		JobArn:           jobARN,
		JobName:          name,
		Status:           "Submitted",
		CreationTime:     now,
		LastModifiedTime: now,
		Tags:             copyTags(tags),
	}

	if len(opts) > 0 && opts[0] != nil {
		opt := opts[0]
		job.RoleArn = opt.RoleArn
		job.ModelID = opt.ModelID
		job.InputDataConfig = opt.InputDataConfig
		job.OutputDataConfig = opt.OutputDataConfig
		job.ClientToken = opt.ClientToken
	}

	b.modelInvocationJobs.Put(job)
	cp := *job
	cp.Tags = copyTags(job.Tags)

	return &cp, nil
}

// GetModelInvocationJob returns a single invocation job by ARN.
func (b *InMemoryBackend) GetModelInvocationJob(jobARN string) (*ModelInvocationJob, error) {
	b.mu.RLock("GetModelInvocationJob")
	defer b.mu.RUnlock()

	job, ok := b.modelInvocationJobs.Get(jobARN)
	if !ok {
		return nil, fmt.Errorf("%w: model invocation job %s not found", ErrNotFound, jobARN)
	}

	cp := *job
	cp.Tags = copyTags(job.Tags)

	return &cp, nil
}

// ListModelInvocationJobs returns invocation jobs with optional filters and
// pagination. Structurally similar to ListEvaluationJobs (same
// filter/sort/paginate shape) but over a distinct resource type and filter
// set; see matchesInvocationJobFilter.
//
//nolint:dupl // see doc comment above.
func (b *InMemoryBackend) ListModelInvocationJobs(
	in *ListModelInvocationJobsInput,
) ([]*ModelInvocationJob, string) {
	b.mu.RLock("ListModelInvocationJobs")
	defer b.mu.RUnlock()

	jobs := make([]*ModelInvocationJob, 0, b.modelInvocationJobs.Len())
	for _, j := range b.modelInvocationJobs.All() {
		if !matchesInvocationJobFilter(j, in) {
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

// matchesInvocationJobFilter reports whether a job satisfies the list filters.
func matchesInvocationJobFilter(j *ModelInvocationJob, in *ListModelInvocationJobsInput) bool {
	if in == nil {
		return true
	}
	if in.StatusEquals != "" && j.Status != in.StatusEquals {
		return false
	}
	if in.NameContains != "" && !containsIgnoreCase(j.JobName, in.NameContains) {
		return false
	}
	if in.SubmitTimeAfter != nil && !j.CreationTime.After(*in.SubmitTimeAfter) {
		return false
	}
	if in.SubmitTimeBefore != nil && !j.CreationTime.Before(*in.SubmitTimeBefore) {
		return false
	}

	return true
}

// containsIgnoreCase is a case-insensitive substring check.
func containsIgnoreCase(s, sub string) bool {
	if sub == "" {
		return true
	}
	sLower := toLower(s)
	subLower := toLower(sub)

	return contains(sLower, subLower)
}

// toLower lowercases ASCII characters only (avoids unicode import).
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}

	return string(b)
}

// contains reports whether sub is a substring of s.
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}

// StopModelInvocationJob marks an invocation job as stopped.
func (b *InMemoryBackend) StopModelInvocationJob(jobARN string) error {
	b.mu.Lock("StopModelInvocationJob")
	defer b.mu.Unlock()

	job, ok := b.modelInvocationJobs.Get(jobARN)
	if !ok {
		return fmt.Errorf("%w: model invocation job %s not found", ErrNotFound, jobARN)
	}

	if job.Status != statusInProgress && job.Status != "Submitted" {
		return fmt.Errorf(
			"%w: model invocation job %s cannot be stopped in status %s",
			ErrValidation,
			jobARN,
			job.Status,
		)
	}

	job.Status = statusStopped
	job.LastModifiedTime = time.Now().UTC()

	return nil
}
