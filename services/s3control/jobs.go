package s3control

import (
	"fmt"
	"maps"
	"sort"
)

// CreateJob creates an S3 Batch Operations job.
// Returns ErrValidation if roleArn is empty.
func (b *InMemoryBackend) CreateJob(accountID, roleArn string, priority int32) (*BatchJob, error) {
	if roleArn == "" {
		return nil, fmt.Errorf("roleArn is required: %w", ErrValidation)
	}

	// AWS S3 Control bounds Priority to a non-negative integer
	// (@range(min:0, max:2147483647)). int32 already caps the upper bound;
	// reject negative values here.
	if priority < 0 {
		return nil, fmt.Errorf("priority must be non-negative: %w", ErrValidation)
	}

	b.mu.Lock("CreateJob")
	defer b.mu.Unlock()

	id := b.newID("job")
	arn := fmt.Sprintf(arnFmtJob, b.region, accountID, id)

	job := &BatchJob{
		AccountID:    accountID,
		JobID:        id,
		JobArn:       arn,
		RoleArn:      roleArn,
		Priority:     priority,
		Status:       jobStatusNew,
		CreationTime: nowRFC3339(),
	}
	b.batchJobs.Put(job)

	cp := *job

	return &cp, nil
}

// UpdateJobDetails persists the extended fields from a CreateJob request to an existing job.
func (b *InMemoryBackend) UpdateJobDetails(
	accountID, jobID, description, manifest, operation, report string,
	confirmationRequired bool,
) error {
	b.mu.Lock("UpdateJobDetails")
	defer b.mu.Unlock()

	job, ok := b.batchJobs.Get(accountID + ":" + jobID)
	if !ok {
		return fmt.Errorf("%w: %s", errJobNotFound, jobID)
	}

	job.Description = description
	job.Manifest = manifest
	job.Operation = operation
	job.Report = report
	job.ConfirmationRequired = confirmationRequired

	return nil
}

// GetJob retrieves a batch job by ID.
func (b *InMemoryBackend) GetJob(accountID, jobID string) (*BatchJob, error) {
	b.mu.RLock("GetJob")
	defer b.mu.RUnlock()

	job, ok := b.batchJobs.Get(accountID + ":" + jobID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errJobNotFound, jobID)
	}

	cp := *job

	return &cp, nil
}

// ListJobs returns all batch jobs for an account, sorted by JobID so the
// handler's index-based nextToken pagination (s3cPaginate) stays stable
// across calls -- store.Table.All()'s iteration order is unspecified.
func (b *InMemoryBackend) ListJobs(accountID string) []*BatchJob {
	b.mu.RLock("ListJobs")
	defer b.mu.RUnlock()

	var out []*BatchJob

	for _, v := range b.batchJobs.All() {
		if v.AccountID == accountID {
			cp := *v
			out = append(out, &cp)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].JobID < out[j].JobID })

	return out
}

// UpdateJobPriority changes the priority of a batch job. The non-negative
// bound is inferred from CreateJob's own check, not stated by this op's doc:
// both fields are the same JobPriority quantity.
func (b *InMemoryBackend) UpdateJobPriority(accountID, jobID string, priority int32) (*BatchJob, error) {
	if priority < 0 {
		return nil, fmt.Errorf("priority must be non-negative: %w", ErrValidation)
	}

	b.mu.Lock("UpdateJobPriority")
	defer b.mu.Unlock()

	job, ok := b.batchJobs.Get(accountID + ":" + jobID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errJobNotFound, jobID)
	}

	job.Priority = priority
	cp := *job

	return &cp, nil
}

// UpdateJobStatus changes the status of a batch job.
func (b *InMemoryBackend) UpdateJobStatus(accountID, jobID, status string) (*BatchJob, error) {
	b.mu.Lock("UpdateJobStatus")
	defer b.mu.Unlock()

	job, ok := b.batchJobs.Get(accountID + ":" + jobID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errJobNotFound, jobID)
	}

	job.Status = status
	cp := *job

	return &cp, nil
}

// ---- Job Tagging ----

// PutJobTagging sets tags on a batch job.
func (b *InMemoryBackend) PutJobTagging(accountID, jobID string, tags TagSet) error {
	b.mu.Lock("PutJobTagging")
	defer b.mu.Unlock()

	key := accountID + ":" + jobID
	if !b.batchJobs.Has(key) {
		return fmt.Errorf("%w: %s", errJobNotFound, jobID)
	}
	b.jobTags[key] = tags

	return nil
}

// GetJobTagging returns tags for a batch job.
func (b *InMemoryBackend) GetJobTagging(accountID, jobID string) (TagSet, error) {
	b.mu.RLock("GetJobTagging")
	defer b.mu.RUnlock()

	key := accountID + ":" + jobID
	if !b.batchJobs.Has(key) {
		return nil, fmt.Errorf("%w: %s", errJobNotFound, jobID)
	}

	tags := b.jobTags[key]
	if tags == nil {
		return TagSet{}, nil
	}
	cp := make(TagSet, len(tags))
	maps.Copy(cp, tags)

	return cp, nil
}

// DeleteJobTagging removes all tags from a batch job.
func (b *InMemoryBackend) DeleteJobTagging(accountID, jobID string) error {
	b.mu.Lock("DeleteJobTagging")
	defer b.mu.Unlock()

	key := accountID + ":" + jobID
	if !b.batchJobs.Has(key) {
		return fmt.Errorf("%w: %s", errJobNotFound, jobID)
	}
	delete(b.jobTags, key)

	return nil
}

// ---- Job status validation ----

// validJobRequestedStatuses returns the set of allowed RequestedJobStatus values.
// AWS only allows transitioning to "Cancelled" or "Ready" via UpdateJobStatus.
func validJobRequestedStatuses() map[string]bool {
	return map[string]bool{
		"Cancelled": true,
		"Ready":     true,
	}
}

// UpdateJobStatusValidated changes the status of a batch job after validating the requested transition.
func (b *InMemoryBackend) UpdateJobStatusValidated(
	accountID, jobID, requestedStatus, statusUpdateReason string,
) (*BatchJob, error) {
	if !validJobRequestedStatuses()[requestedStatus] {
		return nil, fmt.Errorf(
			"requestedJobStatus must be Cancelled or Ready, got %q: %w",
			requestedStatus,
			ErrValidation,
		)
	}

	b.mu.Lock("UpdateJobStatusValidated")
	defer b.mu.Unlock()

	job, ok := b.batchJobs.Get(accountID + ":" + jobID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errJobNotFound, jobID)
	}

	job.Status = requestedStatus
	if statusUpdateReason != "" {
		job.StatusUpdateReason = statusUpdateReason
	}

	cp := *job

	return &cp, nil
}

// ---- Seed helpers for testing ----

// AddBatchJobInternal creates a batch job directly, for seeding test data.
func (b *InMemoryBackend) AddBatchJobInternal(accountID, roleArn string, priority int32) *BatchJob {
	job, _ := b.CreateJob(accountID, roleArn, priority)

	return job
}
