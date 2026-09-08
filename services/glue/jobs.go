package glue

import (
	"fmt"
	"maps"
	mrand "math/rand/v2"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const jobTransitionDelay = 150 * time.Millisecond // STARTING→RUNNING

const jobSucceededDelay = 300 * time.Millisecond // RUNNING→SUCCEEDED

const jobStopDelay = 150 * time.Millisecond // STOPPING→STOPPED

// notionalRunMinutes is the real-Glue run duration that this mock's compressed
// jobTransitionDelay+jobSucceededDelay timeline stands in for. JobRun.Timeout
// is scaled against it, so a Timeout below this bites and anything at or above
// it -- including Glue's 2880-minute default and ordinary values like 60 --
// leaves a normally-completing run alone.
const notionalRunMinutes = 10

// jobRunTimeoutUnit is the compressed duration of one notional Glue minute.
const jobRunTimeoutUnit = jobSucceededDelay / notionalRunMinutes

// secondsPerMinute converts JobRun.Timeout (minutes) to JobRun.ExecutionTime
// (seconds) for a timed-out run.
const secondsPerMinute = 60

// maxJobRetries is the maximum value for MaxRetries on a Glue job.
const maxJobRetries = 10

// cloneJob returns a deep copy of a Job.
func cloneJob(j *Job) *Job {
	cp := *j
	cp.Tags = maps.Clone(j.Tags)
	cp.DefaultArguments = maps.Clone(j.DefaultArguments)
	if len(j.Connections.Connections) > 0 {
		cp.Connections.Connections = make([]string, len(j.Connections.Connections))
		copy(cp.Connections.Connections, j.Connections.Connections)
	}
	if j.SourceControlDetails != nil {
		scd := *j.SourceControlDetails
		cp.SourceControlDetails = &scd
	}

	return &cp
}

// jobARN returns the ARN for a Glue job.
func (b *InMemoryBackend) jobARN(name string) string {
	return arn.Build("glue", b.region, b.accountID, "job/"+name)
}

// CreateJob creates a new Glue job.
func (b *InMemoryBackend) CreateJob(input Job) (*Job, error) {
	b.mu.Lock("CreateJob")
	defer b.mu.Unlock()

	if input.Name == "" || len(input.Name) > maxNameLen || input.Role == "" {
		return nil, ErrValidation
	}

	if input.Command.Name == "" {
		return nil, fmt.Errorf("%w: Command.Name is required", ErrValidation)
	}

	if input.MaxRetries < 0 || input.MaxRetries > maxJobRetries {
		return nil, fmt.Errorf(
			"%w: MaxRetries must be between 0 and %d",
			ErrValidation,
			maxJobRetries,
		)
	}

	if err := validateJobCapacity(input); err != nil {
		return nil, err
	}

	if err := validateTags(input.Tags); err != nil {
		return nil, err
	}

	if b.jobs.Has(input.Name) {
		return nil, ErrAlreadyExists
	}

	now := float64(time.Now().Unix())
	j := &Job{
		Name:                 input.Name,
		Description:          input.Description,
		Role:                 input.Role,
		Command:              input.Command,
		DefaultArguments:     input.DefaultArguments,
		GlueVersion:          input.GlueVersion,
		WorkerType:           input.WorkerType,
		NumberOfWorkers:      input.NumberOfWorkers,
		MaxCapacity:          input.MaxCapacity,
		MaxRetries:           input.MaxRetries,
		Timeout:              input.Timeout,
		ARN:                  b.jobARN(input.Name),
		Tags:                 maps.Clone(input.Tags),
		ExecutionProperty:    input.ExecutionProperty,
		Connections:          input.Connections,
		NotificationProperty: input.NotificationProperty,
		CreatedOn:            now,
		LastModifiedOn:       now,
	}
	b.jobs.Put(j)

	return j, nil
}

// validateJobCapacity enforces AWS Glue's mutual-exclusion rule between the
// legacy MaxCapacity (DPU) knob and WorkerType+NumberOfWorkers: a job may
// specify one or the other but not both.
func validateJobCapacity(input Job) error {
	if input.MaxCapacity > 0 && (input.WorkerType != "" || input.NumberOfWorkers != 0) {
		return fmt.Errorf(
			"%w: cannot specify MaxCapacity and WorkerType/NumberOfWorkers together",
			ErrValidation,
		)
	}

	return nil
}

// GetJob retrieves a Glue job by name.
func (b *InMemoryBackend) GetJob(name string) (*Job, error) {
	b.mu.RLock("GetJob")
	defer b.mu.RUnlock()

	j, ok := b.jobs.Get(name)
	if !ok {
		return nil, ErrNotFound
	}

	return cloneJob(j), nil
}

// GetJobs returns all Glue jobs sorted by name.
func (b *InMemoryBackend) GetJobs() []*Job {
	b.mu.RLock("GetJobs")
	defer b.mu.RUnlock()

	src := b.jobs.Snapshot()
	out := make([]*Job, 0, len(src))
	for _, j := range src {
		out = append(out, cloneJob(j))
	}

	return out
}

// UpdateJob updates an existing Glue job.
func (b *InMemoryBackend) UpdateJob(name string, input Job) error {
	b.mu.Lock("UpdateJob")
	defer b.mu.Unlock()

	j, ok := b.jobs.Get(name)
	if !ok {
		return ErrNotFound
	}

	if input.MaxRetries < 0 || input.MaxRetries > maxJobRetries {
		return fmt.Errorf("%w: MaxRetries must be between 0 and %d", ErrValidation, maxJobRetries)
	}

	if err := validateJobCapacity(input); err != nil {
		return err
	}

	j.Description = input.Description
	j.Role = input.Role
	j.Command = input.Command
	j.DefaultArguments = input.DefaultArguments
	j.GlueVersion = input.GlueVersion
	j.WorkerType = input.WorkerType
	j.NumberOfWorkers = input.NumberOfWorkers
	j.MaxCapacity = input.MaxCapacity
	j.MaxRetries = input.MaxRetries
	j.Timeout = input.Timeout
	j.ExecutionProperty = input.ExecutionProperty
	j.Connections = input.Connections
	j.NotificationProperty = input.NotificationProperty
	j.LastModifiedOn = float64(time.Now().Unix())

	return nil
}

// UpdateJobFromSourceControl synchronizes a job definition from its linked
// remote repository. The emulator has no real repository to pull from, so it
// records the sync linkage against the job as real, queryable state.
func (b *InMemoryBackend) UpdateJobFromSourceControl(jobName string, details SourceControlDetails) error {
	if jobName == "" {
		return fmt.Errorf("%w: JobName is required", ErrValidation)
	}

	b.mu.Lock("UpdateJobFromSourceControl")
	defer b.mu.Unlock()

	j, ok := b.jobs.Get(jobName)
	if !ok {
		return ErrNotFound
	}

	j.SourceControlDetails = &details
	j.LastModifiedOn = float64(time.Now().Unix())

	return nil
}

// UpdateSourceControlFromJob pushes a job's current definition to its linked
// remote repository. As with UpdateJobFromSourceControl, the emulator has no
// real repository, so it records the same sync linkage against the job.
func (b *InMemoryBackend) UpdateSourceControlFromJob(jobName string, details SourceControlDetails) error {
	if jobName == "" {
		return fmt.Errorf("%w: JobName is required", ErrValidation)
	}

	b.mu.Lock("UpdateSourceControlFromJob")
	defer b.mu.Unlock()

	j, ok := b.jobs.Get(jobName)
	if !ok {
		return ErrNotFound
	}

	j.SourceControlDetails = &details
	j.LastModifiedOn = float64(time.Now().Unix())

	return nil
}

// DeleteJob deletes a Glue job by name, also removing all job runs and
// bookmarks. Per AWS's documented behavior (api_op_DeleteJob.go: "If the job
// definition is not found, no exception is thrown"), deleting an unknown
// name is a no-op, not an error.
func (b *InMemoryBackend) DeleteJob(name string) error {
	b.mu.Lock("DeleteJob")
	defer b.mu.Unlock()

	if !b.jobs.Has(name) {
		return nil
	}

	b.jobs.Delete(name)
	delete(b.jobRuns, name)
	b.jobBookmarks.Delete(name)

	return nil
}

// StartJobRun creates a new job run record for the named job.
func (b *InMemoryBackend) StartJobRun(
	jobName string,
	arguments map[string]string,
) (*JobRun, error) {
	return b.StartJobRunWithOptions(jobName, arguments, StartJobRunOptions{})
}

// StartJobRunWithOptions is StartJobRun plus the optional per-run overrides
// AWS's StartJobRunRequest supports (WorkerType/NumberOfWorkers/MaxCapacity/
// Timeout/NotificationProperty/SecurityConfiguration). Per AWS's documented
// StartJobRunRequest semantics, any override left unset (zero-value) falls
// back to the value inherited from the job definition; overrides set here
// apply to this run only and do not mutate the job definition itself.
// checkJobConcurrencyLocked returns ErrConcurrentRunsExceeded if job j already
// has ExecutionProperty.MaxConcurrentRuns active (RUNNING/STARTING) runs.
// Must be called with b.mu held.
func (b *InMemoryBackend) checkJobConcurrencyLocked(jobName string, j *Job) error {
	maxConcurrent := j.ExecutionProperty.MaxConcurrentRuns
	if maxConcurrent <= 0 {
		return nil
	}

	active := 0
	for _, r := range b.jobRuns[jobName] {
		if r.JobRunState == stateRunning || r.JobRunState == stateStarting {
			active++
		}
	}

	if active >= maxConcurrent {
		return ErrConcurrentRunsExceeded
	}

	return nil
}

// jobRunOverrides holds the per-run capacity/timeout/notification settings
// resolved by resolveJobRunOverrides.
type jobRunOverrides struct {
	workerType      string
	notification    NotificationProperty
	numberOfWorkers int
	maxCapacity     float64
	timeout         int
}

// resolveJobRunOverrides applies StartJobRunOptions on top of job j's
// defaults, matching AWS's StartJobRunRequest semantics: unset (zero-value)
// overrides fall back to the job definition; a per-run WorkerType/
// NumberOfWorkers override supersedes a job-level MaxCapacity (and vice
// versa), since the two are mutually exclusive per run just as they are per
// job.
func resolveJobRunOverrides(j *Job, opts StartJobRunOptions) jobRunOverrides {
	out := jobRunOverrides{
		workerType:      j.WorkerType,
		numberOfWorkers: j.NumberOfWorkers,
		maxCapacity:     j.MaxCapacity,
		timeout:         j.Timeout,
		notification:    j.NotificationProperty,
	}

	if opts.WorkerType != "" {
		out.workerType = opts.WorkerType
	}

	if opts.NumberOfWorkers != 0 {
		out.numberOfWorkers = opts.NumberOfWorkers
	}

	if opts.MaxCapacity != 0 {
		out.maxCapacity = opts.MaxCapacity
	}

	if opts.WorkerType != "" || opts.NumberOfWorkers != 0 {
		out.maxCapacity = opts.MaxCapacity
	}

	if opts.MaxCapacity != 0 {
		out.workerType = ""
		out.numberOfWorkers = 0
	}

	if opts.Timeout != 0 {
		out.timeout = opts.Timeout
	}

	if opts.NotificationProperty != nil {
		out.notification = *opts.NotificationProperty
	}

	return out
}

func (b *InMemoryBackend) StartJobRunWithOptions(
	jobName string,
	arguments map[string]string,
	opts StartJobRunOptions,
) (*JobRun, error) {
	if opts.MaxCapacity > 0 && (opts.WorkerType != "" || opts.NumberOfWorkers != 0) {
		return nil, fmt.Errorf(
			"%w: cannot specify MaxCapacity and WorkerType/NumberOfWorkers together",
			ErrValidation,
		)
	}

	b.advanceStates(time.Now())

	b.mu.Lock("StartJobRun")
	defer b.mu.Unlock()

	j, ok := b.jobs.Get(jobName)
	if !ok {
		return nil, ErrNotFound
	}

	if err := b.checkJobConcurrencyLocked(jobName, j); err != nil {
		return nil, err
	}

	ov := resolveJobRunOverrides(j, opts)

	now := time.Now()
	run := &JobRun{
		ID: fmt.Sprintf(
			"jr_%d_%04d",
			now.UnixNano(),
			mrand.IntN(10000), //nolint:gosec,mnd // non-security mock run ID
		),
		JobName:               jobName,
		JobRunState:           stateStarting,
		StartedOn:             float64(now.Unix()),
		Arguments:             maps.Clone(arguments),
		WorkerType:            ov.workerType,
		NumberOfWorkers:       ov.numberOfWorkers,
		MaxCapacity:           ov.maxCapacity,
		GlueVersion:           j.GlueVersion,
		Timeout:               ov.timeout,
		NotificationProperty:  ov.notification,
		SecurityConfiguration: opts.SecurityConfiguration,
	}
	b.jobRuns[jobName] = append(b.jobRuns[jobName], run)

	// Schedule STARTING→RUNNING→SUCCEEDED transitions.
	if b.jobRunReadyAt[jobName] == nil {
		b.jobRunReadyAt[jobName] = make(map[string]time.Time)
	}

	if b.jobRunDoneAt[jobName] == nil {
		b.jobRunDoneAt[jobName] = make(map[string]time.Time)
	}

	b.jobRunReadyAt[jobName][run.ID] = now.Add(jobTransitionDelay)
	b.jobRunDoneAt[jobName][run.ID] = now.Add(jobTransitionDelay + jobSucceededDelay)

	if run.Timeout > 0 {
		if b.jobRunTimeoutAt[jobName] == nil {
			b.jobRunTimeoutAt[jobName] = make(map[string]time.Time)
		}

		b.jobRunTimeoutAt[jobName][run.ID] = now.Add(jobTransitionDelay + time.Duration(run.Timeout)*jobRunTimeoutUnit)
	}

	bm, ok := b.jobBookmarks.Get(jobName)
	if !ok {
		bm = &JobBookmark{JobName: jobName}
		b.jobBookmarks.Put(bm)
	}
	bm.ActiveRun = run.ID
	bm.Attempt++

	return run, nil
}

// GetJobRun retrieves a specific job run by job name and run ID.
func (b *InMemoryBackend) GetJobRun(jobName, runID string) (*JobRun, error) {
	b.advanceStates(time.Now())

	b.mu.RLock("GetJobRun")
	defer b.mu.RUnlock()

	for _, run := range b.jobRuns[jobName] {
		if run.ID == runID {
			cp := *run
			cp.Arguments = maps.Clone(run.Arguments)
			cp.WorkflowRunID = "" // internal-only; real JobRun has no such field

			return &cp, nil
		}
	}

	return nil, ErrNotFound
}

// GetJobRuns returns all runs for a job.
func (b *InMemoryBackend) GetJobRuns(jobName string) ([]*JobRun, error) {
	b.advanceStates(time.Now())

	b.mu.RLock("GetJobRuns")
	defer b.mu.RUnlock()

	if !b.jobs.Has(jobName) {
		return nil, ErrNotFound
	}

	src := b.jobRuns[jobName]
	out := make([]*JobRun, 0, len(src))
	for _, run := range src {
		cp := *run
		cp.Arguments = maps.Clone(run.Arguments)
		cp.WorkflowRunID = "" // internal-only; real JobRun has no such field
		out = append(out, &cp)
	}

	return out, nil
}

// BatchStopJobRun stops multiple job runs by setting their state to STOPPING.
// Only RUNNING or STARTING runs can be stopped.
func (b *InMemoryBackend) BatchStopJobRun(
	jobName string,
	runIDs []string,
) ([]BatchStopJobRunSuccessfulSubmission, []BatchStopJobRunError) {
	now := time.Now()
	b.advanceStates(now)

	b.mu.Lock("BatchStopJobRun")
	defer b.mu.Unlock()

	successes := make([]BatchStopJobRunSuccessfulSubmission, 0, len(runIDs))
	errs := make([]BatchStopJobRunError, 0, len(runIDs))

	for _, id := range runIDs {
		found := false
		for _, run := range b.jobRuns[jobName] {
			if run.ID != id {
				continue
			}
			found = true
			if run.JobRunState != stateRunning && run.JobRunState != stateStarting {
				errs = append(errs, BatchStopJobRunError{
					JobName:  jobName,
					JobRunID: id,
					ErrorDetail: ErrorDetail{
						ErrorCode:    "IllegalStateException",
						ErrorMessage: "job run " + id + " is not in a stoppable state: " + run.JobRunState,
					},
				})
			} else {
				run.JobRunState = stateStopping

				if b.jobRunStopAt[jobName] == nil {
					b.jobRunStopAt[jobName] = make(map[string]time.Time)
				}

				b.jobRunStopAt[jobName][run.ID] = now.Add(jobStopDelay)

				successes = append(successes, BatchStopJobRunSuccessfulSubmission{
					JobName:  jobName,
					JobRunID: id,
				})
			}

			break
		}
		if !found {
			errs = append(errs, BatchStopJobRunError{
				JobName:  jobName,
				JobRunID: id,
				ErrorDetail: ErrorDetail{
					ErrorCode:    "EntityNotFoundException",
					ErrorMessage: "job run not found: " + id,
				},
			})
		}
	}

	return successes, errs
}

// GetJobBookmark returns the bookmark for a job.
func (b *InMemoryBackend) GetJobBookmark(jobName string) (*JobBookmark, error) {
	b.mu.RLock("GetJobBookmark")
	defer b.mu.RUnlock()

	if !b.jobs.Has(jobName) {
		return nil, ErrNotFound
	}

	bm, ok := b.jobBookmarks.Get(jobName)
	if !ok {
		return &JobBookmark{JobName: jobName}, nil
	}

	cp := *bm

	return &cp, nil
}

// ResetJobBookmark clears the bookmark for a job and returns the post-reset bookmark.
func (b *InMemoryBackend) ResetJobBookmark(jobName string) error {
	b.mu.Lock("ResetJobBookmark")
	defer b.mu.Unlock()

	if !b.jobs.Has(jobName) {
		return ErrNotFound
	}

	b.jobBookmarks.Delete(jobName)

	return nil
}

// ResetJobBookmarkWithResult atomically clears the bookmark for a job and returns the post-reset bookmark.
func (b *InMemoryBackend) ResetJobBookmarkWithResult(jobName string) (*JobBookmark, error) {
	b.mu.Lock("ResetJobBookmarkWithResult")
	defer b.mu.Unlock()

	if !b.jobs.Has(jobName) {
		return nil, ErrNotFound
	}

	b.jobBookmarks.Delete(jobName)

	return &JobBookmark{JobName: jobName}, nil
}

// AddJobRunInternal adds a job run directly to the backend without validation.
func (b *InMemoryBackend) AddJobRunInternal(run *JobRun) {
	b.mu.Lock("AddJobRunInternal")
	defer b.mu.Unlock()

	cp := *run
	cp.Arguments = maps.Clone(run.Arguments)
	b.jobRuns[run.JobName] = append(b.jobRuns[run.JobName], &cp)
}
