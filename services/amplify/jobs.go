package amplify

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// StartJob creates and starts a new deployment job for a branch. jobID is
// required when jobType is RETRY (real Amplify's StartJobInput.JobId is
// required in that case) and identifies the job being retried: if that job
// exists, its commit metadata is inherited by the new job unless the caller
// supplies its own. commitTime may be the zero time, in which case the
// response's commitTime field is omitted (matches an app with no Git commit
// metadata, e.g. a manually deployed app).
func (b *InMemoryBackend) StartJob(
	appID, branchName, jobType, jobID, commitID, commitMsg string,
	commitTime time.Time,
) (*Job, error) {
	if !isValidJobType(jobType) && jobType != "" {
		return nil, fmt.Errorf("%w: invalid jobType %q", ErrValidation, jobType)
	}

	jt := JobType(jobType)
	if jt == "" {
		jt = JobTypeRelease
	}

	if jt == JobTypeRetry && jobID == "" {
		return nil, fmt.Errorf("%w: jobId is required when jobType is RETRY", ErrValidation)
	}

	b.mu.Lock("StartJob")
	defer b.mu.Unlock()

	if !b.apps.Has(appID) {
		return nil, fmt.Errorf("%w: app %s not found", ErrNotFound, appID)
	}

	if !b.branches.Has(branchKey(appID, branchName)) {
		return nil, fmt.Errorf("%w: branch %s not found for app %s", ErrNotFound, branchName, appID)
	}

	if jt == JobTypeRetry {
		commitID, commitMsg, commitTime = b.inheritRetryCommitInfo(
			appID, branchName, jobID, commitID, commitMsg, commitTime,
		)
	}

	newJobID := randomID()
	now := time.Now().UTC()

	job := &Job{
		JobID:      newJobID,
		JobARN:     b.jobARN(appID, branchName, newJobID),
		CommitID:   commitID,
		CommitMsg:  commitMsg,
		CommitTime: commitTime,
		Status:     JobStatusRunning,
		Type:       jt,
		StartTime:  now,
		AppID:      appID,
		BranchName: branchName,
	}

	b.jobs.Put(job)

	cp := *job

	return &cp, nil
}

// inheritRetryCommitInfo fills in any commit fields the caller left empty
// for a RETRY job from the job being retried, when that job still exists.
// Must be called while holding the write lock.
func (b *InMemoryBackend) inheritRetryCommitInfo(
	appID, branchName, priorJobID, commitID, commitMsg string,
	commitTime time.Time,
) (string, string, time.Time) {
	prior, ok := b.jobs.Get(jobKey(appID, branchName, priorJobID))
	if !ok {
		return commitID, commitMsg, commitTime
	}

	if commitID == "" {
		commitID = prior.CommitID
	}

	if commitMsg == "" {
		commitMsg = prior.CommitMsg
	}

	if commitTime.IsZero() {
		commitTime = prior.CommitTime
	}

	return commitID, commitMsg, commitTime
}

// jobARN builds the ARN for a deployment job. Real Amplify's JobSummary.JobArn
// is a required response field; every job created by this backend must carry
// one so the aws-sdk-go-v2 deserializer doesn't leave it nil.
func (b *InMemoryBackend) jobARN(appID, branchName, jobID string) string {
	return arn.Build(
		"amplify",
		b.region,
		b.accountID,
		fmt.Sprintf("apps/%s/branches/%s/jobs/%s", appID, branchName, jobID),
	)
}

// StopJob cancels a running job. Real Amplify's StopJob "[s]tops a job that
// is in progress" (aws-sdk-go-v2/service/amplify api_op_StopJob.go); a job
// already in a terminal state (SUCCEED/FAILED/CANCELLED) is not in progress,
// so stopping it is rejected rather than silently overwriting its outcome.
func (b *InMemoryBackend) StopJob(appID, branchName, jobID string) (*Job, error) {
	b.mu.Lock("StopJob")
	defer b.mu.Unlock()

	job, err := b.findJob(appID, branchName, jobID)
	if err != nil {
		return nil, err
	}

	if isTerminalJobStatus(job.Status) {
		return nil, fmt.Errorf(
			"%w: job %s is not in progress (status %s)", ErrValidation, jobID, job.Status,
		)
	}

	job.Status = JobStatusCancelled
	job.EndTime = time.Now().UTC()

	cp := *job

	return &cp, nil
}

// GetJob returns a job by ID.
func (b *InMemoryBackend) GetJob(appID, branchName, jobID string) (*Job, error) {
	b.mu.RLock("GetJob")
	defer b.mu.RUnlock()

	job, err := b.findJob(appID, branchName, jobID)
	if err != nil {
		return nil, err
	}

	cp := *job

	return &cp, nil
}

// ListJobs lists all jobs for a branch.
func (b *InMemoryBackend) ListJobs(
	appID, branchName, nextToken string,
	maxResults int,
) ([]*Job, string, error) {
	b.mu.RLock("ListJobs")
	defer b.mu.RUnlock()

	if !b.apps.Has(appID) {
		return nil, "", fmt.Errorf("%w: app %s not found", ErrNotFound, appID)
	}

	var all []*Job

	for _, job := range b.jobsByBranch.Get(branchKey(appID, branchName)) {
		cp := *job
		all = append(all, &cp)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].JobID < all[j].JobID })

	page, token := amplifyPaginate(all, nextToken, maxResults)

	return page, token, nil
}

// DeleteJob deletes a job record, cascading the deletion to every artifact
// that job produced (see janitor.go's advanceJobs) so ListArtifacts can
// never return a row for a job that no longer exists.
func (b *InMemoryBackend) DeleteJob(appID, branchName, jobID string) (*Job, error) {
	b.mu.Lock("DeleteJob")
	defer b.mu.Unlock()

	job, err := b.findJob(appID, branchName, jobID)
	if err != nil {
		return nil, err
	}

	cp := *job
	key := jobKey(appID, branchName, jobID)

	for _, art := range slices.Clone(b.artifactsByJob.Get(key)) {
		b.artifacts.Delete(art.ArtifactID)
	}

	b.jobs.Delete(key)

	return &cp, nil
}

// findJob locates a job in the jobs table. Must be called while holding a lock.
func (b *InMemoryBackend) findJob(appID, branchName, jobID string) (*Job, error) {
	job, ok := b.jobs.Get(jobKey(appID, branchName, jobID))
	if !ok {
		return nil, fmt.Errorf(
			"%w: job %s not found for branch %s app %s",
			ErrNotFound,
			jobID,
			branchName,
			appID,
		)
	}

	return job, nil
}
