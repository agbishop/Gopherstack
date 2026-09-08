package codepipeline

import (
	"context"
	"fmt"
	"sort"
)

// AcknowledgeJob acknowledges that a job worker has received a job.
// Returns InProgress if Nonce matches; otherwise returns current status unchanged.
func (b *InMemoryBackend) AcknowledgeJob(ctx context.Context, jobID, nonce string) (string, error) {
	b.mu.Lock("AcknowledgeJob")
	defer b.mu.Unlock()

	job, ok := b.jobs.Get(regionKey(getRegion(ctx, b.region), jobID))
	if !ok {
		return "", fmt.Errorf("%w: job %q", ErrJobNotFound, jobID)
	}

	if job.Nonce == nonce {
		job.Status = statusInProgress
	}

	return job.Status, nil
}

// GetJobDetails returns details for a job by ID.
func (b *InMemoryBackend) GetJobDetails(ctx context.Context, jobID string) (*Job, error) {
	b.mu.RLock("GetJobDetails")
	defer b.mu.RUnlock()

	job, ok := b.jobs.Get(regionKey(getRegion(ctx, b.region), jobID))
	if !ok {
		return nil, fmt.Errorf("%w: job %q", ErrJobNotFound, jobID)
	}

	cp := *job

	return &cp, nil
}

// AddJobInternal seeds a job into the backend's default region (for testing).
func (b *InMemoryBackend) AddJobInternal(job *Job) {
	b.mu.Lock("AddJobInternal")
	defer b.mu.Unlock()

	cp := *job
	cp.region = b.region
	b.jobs.Put(&cp)
}

// getJobLocked looks up a job by ID. Callers must already hold b.mu.
func (b *InMemoryBackend) getJobLocked(ctx context.Context, jobID string) (*Job, error) {
	job, ok := b.jobs.Get(regionKey(getRegion(ctx, b.region), jobID))
	if !ok {
		return nil, fmt.Errorf("%w: job %q", ErrJobNotFound, jobID)
	}

	return job, nil
}

// pollForJobsLocked returns the live (unsorted-copy) queued jobs matching the
// given ActionTypeID, in ID order. Callers must already hold b.mu (either
// side): the returned pointers alias the store, so a caller wanting to
// mutate them (e.g. to lazily issue a ClientID) must hold the write lock.
func (b *InMemoryBackend) pollForJobsLocked(ctx context.Context, category, owner, provider, version string) []*Job {
	entries := b.jobsByRegion.Get(getRegion(ctx, b.region))

	result := make([]*Job, 0, len(entries))

	for _, job := range entries {
		if job.Status != "Queued" {
			continue
		}

		at := job.ActionTypeID
		if at.Category != category || at.Provider != provider || at.Version != version {
			continue
		}

		if owner != "" && at.Owner != "" && at.Owner != owner {
			continue
		}

		result = append(result, job)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// PollForJobs returns available queued jobs matching the given ActionTypeID.
func (b *InMemoryBackend) PollForJobs(ctx context.Context, category, owner, provider, version string) ([]*Job, error) {
	b.mu.RLock("PollForJobs")
	defer b.mu.RUnlock()

	jobs := b.pollForJobsLocked(ctx, category, owner, provider, version)

	result := make([]*Job, len(jobs))
	for i, j := range jobs {
		cp := *j
		result[i] = &cp
	}

	return result, nil
}

// PutJobSuccessResult acknowledges job success.
func (b *InMemoryBackend) PutJobSuccessResult(ctx context.Context, jobID string) error {
	b.mu.Lock("PutJobSuccessResult")
	defer b.mu.Unlock()

	job, err := b.getJobLocked(ctx, jobID)
	if err != nil {
		return err
	}

	job.Status = "Succeeded"

	return nil
}

// PutJobFailureResult acknowledges job failure.
func (b *InMemoryBackend) PutJobFailureResult(ctx context.Context, jobID, message, failureType string) error {
	b.mu.Lock("PutJobFailureResult")
	defer b.mu.Unlock()

	job, err := b.getJobLocked(ctx, jobID)
	if err != nil {
		return err
	}

	job.Status = "Failed"
	job.FailureMessage = message
	job.FailureType = failureType

	return nil
}
