package emrserverless

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) jobRunARN(applicationID, jobRunID string) string {
	return arn.Build("emr-serverless", b.region, b.accountID,
		fmt.Sprintf("/applications/%s/jobruns/%s", applicationID, jobRunID))
}

// StartJobRun creates and starts a new job run. If opts carries a non-empty
// ClientToken that was already used successfully for this application, the
// previously created job run is returned instead of creating a duplicate --
// matching AWS's client-idempotency-token contract. JobDriver and
// ConfigurationOverrides are stored and echoed back verbatim by
// GetJobRun/ListJobRuns rather than discarded: JobDriver is a required field
// on the real JobRun response shape, so dropping it there would silently
// erase the job specification the caller submitted.
func (b *InMemoryBackend) StartJobRun(
	applicationID, executionRoleArn, name, mode string,
	tags map[string]string,
	opts ...StartJobRunOptions,
) (*JobRun, error) {
	b.mu.Lock("StartJobRun")
	defer b.mu.Unlock()

	var opt StartJobRunOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	if executionRoleArn == "" {
		return nil, fmt.Errorf("%w: executionRoleArn is required", ErrValidation)
	}

	app, ok := b.applications.Get(applicationID)
	if !ok {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, applicationID)
	}

	if opt.ClientToken != "" {
		if jr := b.jobRunForToken(applicationID, opt.ClientToken); jr != nil {
			return cloneJobRun(jr), nil
		}
	}

	if app.State != ApplicationStateStarted {
		if !applicationAutoStartEnabled(app) {
			return nil, fmt.Errorf(
				"%w: application %s is not started and autoStartConfiguration disables implicit start",
				ErrConflict, applicationID,
			)
		}

		app.State = ApplicationStateStarted
		app.UpdatedAt = time.Now().UTC()
	}

	if mode == "" {
		mode = "BATCH"
	}

	executionTimeoutMinutes := opt.ExecutionTimeoutMinutes
	if executionTimeoutMinutes <= 0 {
		executionTimeoutMinutes = DefaultJobRunExecutionTimeoutMinutes
	}

	jobRunID := newID()
	now := time.Now().UTC()

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	jr := &JobRun{
		ApplicationID:    applicationID,
		JobRunID:         jobRunID,
		Arn:              b.jobRunARN(applicationID, jobRunID),
		Name:             name,
		State:            JobRunStateSubmitted,
		ExecutionRoleArn: executionRoleArn,
		// CreatedBy is not tracked by this backend's IAM model; the
		// execution role ARN is used as a best-effort substitute for the
		// required response field (see JobRun.CreatedBy).
		CreatedBy:               executionRoleArn,
		Mode:                    mode,
		ReleaseLabel:            app.ReleaseLabel,
		JobDriver:               cloneJSONValue(opt.JobDriver),
		ConfigurationOverrides:  cloneJSONValue(opt.ConfigurationOverrides),
		ExecutionIamPolicy:      cloneJSONValue(opt.ExecutionIamPolicy),
		RetryPolicy:             cloneJSONValue(opt.RetryPolicy),
		ExecutionTimeoutMinutes: executionTimeoutMinutes,
		CreatedAt:               now,
		UpdatedAt:               now,
		Tags:                    tagsCopy,
	}

	b.jobRuns.Put(jr)

	if opt.ClientToken != "" {
		if b.jobRunTokens[applicationID] == nil {
			b.jobRunTokens[applicationID] = make(map[string]string)
		}

		b.jobRunTokens[applicationID][opt.ClientToken] = jobRunID
	}

	return cloneJobRun(jr), nil
}

// jobRunForToken looks up a previously started job run by its client
// idempotency token, scoped to applicationID. Returns nil if the token is
// unknown or the job run it pointed to no longer exists (a stale token is
// treated as a miss, not an error, so StartJobRun falls through to creating
// a fresh job run). Caller must hold the write lock.
func (b *InMemoryBackend) jobRunForToken(applicationID, clientToken string) *JobRun {
	jobRunID := b.jobRunTokens[applicationID][clientToken]
	if jobRunID == "" {
		return nil
	}

	jr, ok := b.jobRuns.Get(jobRunID)
	if !ok || jr.ApplicationID != applicationID {
		return nil
	}

	return jr
}

// GetJobRun retrieves a job run by application ID and job run ID.
func (b *InMemoryBackend) GetJobRun(applicationID, jobRunID string) (*JobRun, error) {
	b.mu.RLock("GetJobRun")
	defer b.mu.RUnlock()

	if !b.applications.Has(applicationID) {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, applicationID)
	}

	jr, ok := b.jobRuns.Get(jobRunID)
	if !ok || jr.ApplicationID != applicationID {
		return nil, fmt.Errorf("%w: job run %s not found", ErrNotFound, jobRunID)
	}

	return cloneJobRun(jr), nil
}

// ListJobRuns returns paginated job runs for an application, optionally filtered by state.
func (b *InMemoryBackend) ListJobRuns(
	applicationID, nextToken string, maxResults int, states ...string,
) ([]*JobRun, string, error) {
	b.mu.RLock("ListJobRuns")
	defer b.mu.RUnlock()

	if !b.applications.Has(applicationID) {
		return nil, "", fmt.Errorf("%w: application %s not found", ErrNotFound, applicationID)
	}

	runs := b.jobRunsByApplication.Get(applicationID)
	list := make([]*JobRun, 0, len(runs))

	for _, jr := range runs {
		list = append(list, cloneJobRun(jr))
	}

	if len(states) > 0 {
		stateSet := make(map[string]struct{}, len(states))
		for _, s := range states {
			stateSet[s] = struct{}{}
		}
		filtered := list[:0]
		for _, jr := range list {
			if _, ok := stateSet[jr.State]; ok {
				filtered = append(filtered, jr)
			}
		}
		list = filtered
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].JobRunID < list[j].JobRunID
		}

		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	page, token := emrPaginate(list, nextToken, maxResults)

	return page, token, nil
}

// isTerminalJobRunState returns true if the given state prevents cancellation.
func isTerminalJobRunState(state string) bool {
	return state == JobRunStateSuccess || state == JobRunStateFailed || state == JobRunStateCancelled
}

// CancelJobRun cancels a job run.
func (b *InMemoryBackend) CancelJobRun(applicationID, jobRunID string) (*JobRun, error) {
	b.mu.Lock("CancelJobRun")
	defer b.mu.Unlock()

	if !b.applications.Has(applicationID) {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, applicationID)
	}

	jr, ok := b.jobRuns.Get(jobRunID)
	if !ok || jr.ApplicationID != applicationID {
		return nil, fmt.Errorf("%w: job run %s not found", ErrNotFound, jobRunID)
	}

	if isTerminalJobRunState(jr.State) {
		return nil, fmt.Errorf(
			"%w: job run %s cannot be cancelled from state %s",
			ErrInvalidState, jobRunID, jr.State,
		)
	}

	jr.State = JobRunStateCancelled
	jr.StateDetails = "Job run cancelled by user request"
	jr.UpdatedAt = time.Now().UTC()

	return cloneJobRun(jr), nil
}

// GetDashboardForJobRun returns a dashboard URL for a job run.
func (b *InMemoryBackend) GetDashboardForJobRun(applicationID, jobRunID string) (string, error) {
	b.mu.RLock("GetDashboardForJobRun")
	defer b.mu.RUnlock()

	if !b.applications.Has(applicationID) {
		return "", fmt.Errorf("%w: application %s not found", ErrNotFound, applicationID)
	}

	jr, ok := b.jobRuns.Get(jobRunID)
	if !ok || jr.ApplicationID != applicationID {
		return "", fmt.Errorf("%w: job run %s not found", ErrNotFound, jobRunID)
	}

	url := fmt.Sprintf("https://console.aws.amazon.com/emr-serverless/home?region=%s#/applications/%s/jobruns/%s",
		b.region, applicationID, jobRunID)

	return url, nil
}

// cloneJobRun returns a deep copy of a JobRun with its Tags map cloned.
// The returned copy always has a non-nil Tags map.
func cloneJobRun(jr *JobRun) *JobRun {
	cp := *jr
	cp.Tags = make(map[string]string, len(jr.Tags))
	maps.Copy(cp.Tags, jr.Tags)
	cp.JobDriver = cloneJSONValue(jr.JobDriver)
	cp.ConfigurationOverrides = cloneJSONValue(jr.ConfigurationOverrides)
	cp.ExecutionIamPolicy = cloneJSONValue(jr.ExecutionIamPolicy)
	cp.RetryPolicy = cloneJSONValue(jr.RetryPolicy)

	return &cp
}

// AddJobRunInternal directly inserts a JobRun into the backend without going through
// the HTTP layer.  The application must already exist.  Intended for test seeding only.
func (b *InMemoryBackend) AddJobRunInternal(jr *JobRun) {
	b.mu.Lock("AddJobRunInternal")
	defer b.mu.Unlock()

	if jr.Tags == nil {
		jr.Tags = make(map[string]string)
	}

	b.jobRuns.Put(jr)
}
