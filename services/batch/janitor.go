package batch

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	defaultBatchJanitorInterval   = time.Minute
	defaultBatchInactiveJobDefTTL = 24 * time.Hour
	defaultBatchCompletedJobTTL   = 24 * time.Hour

	batchWorkerServiceName         = "batch"
	inactiveJobDefSweeperComponent = "InactiveJobDefinitionSweeper"
	completedJobSweeperComponent   = "CompletedJobSweeper"

	// jobAdvanceAttemptTimedOut is an internal advanceKey.newStatus marker (not
	// a real Batch job status) meaning the running attempt exceeded its
	// JobTimeout.AttemptDurationSeconds; applyAdvanceRegularJobs resolves it to
	// either a retry (RUNNABLE) or a terminal FAILED, per RetryStrategy.Attempts.
	jobAdvanceAttemptTimedOut = "internal:AttemptTimedOut"
)

// Janitor is the Batch background worker that evicts INACTIVE job definitions
// after a configurable TTL to prevent unbounded growth of in-memory state.
// This matches AWS behavior where deregistered definitions eventually disappear.
// It also evicts completed and failed jobs after a configurable TTL, matching
// the AWS Batch job history retention behavior.
type Janitor struct {
	Backend           *InMemoryBackend
	Interval          time.Duration
	InactiveJobDefTTL time.Duration
	CompletedJobTTL   time.Duration
	TaskTimeout       time.Duration
}

// NewJanitor creates a new Batch Janitor for the given backend.
// Zero values for interval, inactiveJobDefTTL, or completedJobTTL fall back to defaults.
func NewJanitor(backend *InMemoryBackend, interval, inactiveJobDefTTL, completedJobTTL time.Duration) *Janitor {
	if interval == 0 {
		interval = defaultBatchJanitorInterval
	}

	if inactiveJobDefTTL == 0 {
		inactiveJobDefTTL = defaultBatchInactiveJobDefTTL
	}

	if completedJobTTL == 0 {
		completedJobTTL = defaultBatchCompletedJobTTL
	}

	return &Janitor{
		Backend:           backend,
		Interval:          interval,
		InactiveJobDefTTL: inactiveJobDefTTL,
		CompletedJobTTL:   completedJobTTL,
	}
}

// Run runs the janitor loop until ctx is cancelled.
func (j *Janitor) Run(ctx context.Context) {
	g := worker.NewGroup(ctx, batchWorkerServiceName)
	g.Ticker(
		inactiveJobDefSweeperComponent,
		j.Interval,
		j.TaskTimeout,
		j.SweepOnce,
	)

	<-ctx.Done()
	g.Stop()
}

// SweepOnce runs a single sweep pass. Exposed for testing.
func (j *Janitor) SweepOnce(ctx context.Context) {
	j.sweepInactiveJobDefinitions(ctx)
	j.sweepCompletedJobs(ctx)
	j.advanceJobs(ctx)
}

// jobDefEvictKey identifies a job definition to evict: region plus the
// composite store.Table key derived from it (see regionKey in backend.go).
type jobDefEvictKey struct {
	region, tableKey string
}

// sweepInactiveJobDefinitions removes job definitions that have been in INACTIVE
// status for longer than InactiveJobDefTTL. Orphaned revision counters (names
// with no remaining definitions) are also removed to prevent unbounded growth.
func (j *Janitor) sweepInactiveJobDefinitions(ctx context.Context) {
	cutoff := time.Now().Add(-j.InactiveJobDefTTL)

	var toEvict []jobDefEvictKey

	j.Backend.mu.RLock("BatchJanitorInactiveDefs")
	for _, jd := range j.Backend.jobDefinitions.All() {
		if jd.Status == jobDefStatusInactive && jd.DeregisteredAt != nil && jd.DeregisteredAt.Before(cutoff) {
			toEvict = append(toEvict, jobDefEvictKey{jd.region, regionKey(jd.region, jd.JobDefinitionArn)})
		}
	}
	j.Backend.mu.RUnlock()

	if len(toEvict) == 0 {
		return
	}

	j.Backend.mu.Lock("BatchJanitorInactiveDefsDel")
	for _, k := range toEvict {
		j.Backend.jobDefinitions.Delete(k.tableKey)
	}

	regionsSet := make(map[string]struct{})
	for _, k := range toEvict {
		regionsSet[k.region] = struct{}{}
	}

	for region := range regionsSet {
		surviving := make(map[string]struct{})
		for _, jd := range j.Backend.jobDefinitionsByRegion.Get(region) {
			surviving[jd.JobDefinitionName] = struct{}{}
		}

		revisions := j.Backend.jobDefRevisions[region]
		for name := range revisions {
			if _, ok := surviving[name]; !ok {
				delete(revisions, name)
			}
		}
	}
	j.Backend.mu.Unlock()

	count := len(toEvict)

	telemetry.RecordWorkerTask(batchWorkerServiceName, inactiveJobDefSweeperComponent, "success")
	telemetry.RecordWorkerItems(batchWorkerServiceName, inactiveJobDefSweeperComponent, count)
	logger.Load(ctx).InfoContext(ctx, "Batch janitor: INACTIVE job definitions evicted", "count", count)
}

// jobEvictKey identifies a job to evict: region plus job ID.
type jobEvictKey struct {
	region, id string
}

// sweepCompletedJobs removes completed or failed Batch jobs whose StoppedAt
// timestamp is older than CompletedJobTTL. This mirrors AWS Batch behavior where
// job history is retained for a limited period before automatic removal.
func (j *Janitor) sweepCompletedJobs(ctx context.Context) {
	cutoffMs := time.Now().Add(-j.CompletedJobTTL).UnixMilli()

	var toEvict []jobEvictKey

	j.Backend.mu.RLock("BatchJanitorCompletedJobs")
	for _, job := range j.Backend.jobs.All() {
		if !isTerminalJobStatus(job.Status) {
			continue
		}

		if job.StoppedAt == nil {
			continue
		}

		if *job.StoppedAt < cutoffMs {
			toEvict = append(toEvict, jobEvictKey{job.region, job.JobID})
		}
	}
	j.Backend.mu.RUnlock()

	if len(toEvict) == 0 {
		return
	}

	j.Backend.mu.Lock("BatchJanitorCompletedJobsDel")
	for _, k := range toEvict {
		// Table.Delete also removes the job from the byRegion/byARN/byQueue
		// indexes, replacing the old manual jobsByARN cleanup.
		j.Backend.jobs.Delete(regionKey(k.region, k.id))
	}
	j.Backend.mu.Unlock()

	count := len(toEvict)

	telemetry.RecordWorkerTask(batchWorkerServiceName, completedJobSweeperComponent, "success")
	telemetry.RecordWorkerItems(batchWorkerServiceName, completedJobSweeperComponent, count)
	logger.Load(ctx).InfoContext(ctx, "Batch janitor: completed jobs evicted", "count", count)
}

type advanceKey struct {
	region, id, newStatus string
}

func (j *Janitor) getJobsToAdvance() ([]advanceKey, []advanceKey) {
	var toAdvance []advanceKey
	var toAdvanceSvc []advanceKey

	j.Backend.mu.RLock("BatchJanitorAdvanceJobsLock")
	defer j.Backend.mu.RUnlock()

	for _, job := range j.Backend.jobs.All() {
		switch job.Status {
		case jobStatusSubmitted, jobStatusPending, jobStatusRunnable, jobStatusStarting:
			switch depStatus := j.dependencyStatus(job); depStatus {
			case dependencyPending:
			case dependencyFailed:
				toAdvance = append(toAdvance, advanceKey{job.region, job.JobID, jobStatusFailed})
			case dependencySatisfied:
				toAdvance = append(toAdvance, advanceKey{job.region, job.JobID, jobStatusRunning})
			}
		case jobStatusRunning:
			if job.StoppedAt != nil {
				continue
			}

			if j.Backend.jobAttemptTimedOutLocked(job) {
				toAdvance = append(toAdvance, advanceKey{job.region, job.JobID, jobAdvanceAttemptTimedOut})
			} else {
				toAdvance = append(toAdvance, advanceKey{job.region, job.JobID, jobStatusSucceeded})
			}
		}
	}

	for _, job := range j.Backend.serviceJobs.All() {
		switch job.Status {
		case jobStatusSubmitted, jobStatusPending, jobStatusRunnable, jobStatusStarting:
			toAdvanceSvc = append(toAdvanceSvc, advanceKey{job.region, job.JobID, jobStatusRunning})
		case jobStatusRunning:
			if job.StoppedAt == nil {
				toAdvanceSvc = append(toAdvanceSvc, advanceKey{job.region, job.JobID, jobStatusSucceeded})
			}
		}
	}

	return toAdvance, toAdvanceSvc
}

type dependencyState int

const (
	dependencySatisfied dependencyState = iota
	dependencyPending
	dependencyFailed
)

// dependencyStatus evaluates a job's DependsOn list (batch@v1.68.4
// api_op_SubmitJob.go: "A list of dependencies for the job... each index
// child of this job must wait for the corresponding index child of each
// dependency to complete before it can begin"). A job with any dependency
// not yet in a terminal state stays dependencyPending (blocks the
// SUBMITTED/PENDING/RUNNABLE/STARTING -> RUNNING advance below); a FAILED
// dependency propagates as dependencyFailed. Caller must hold at least a
// read lock.
//
// Only the plain JobId form is evaluated. SEQUENTIAL/N_TO_N dependency
// types reference array-job children this backend never spawns (SubmitJob
// stores ArrayProperties.Size without creating child Job records -- a
// pre-existing, disclosed gap; see ListJobs's arrayJobId note in
// PARITY.md), so an entry with no JobId can never be resolved and is
// skipped rather than blocking a job forever.
func (j *Janitor) dependencyStatus(job *Job) dependencyState {
	for _, dep := range job.DependsOn {
		if dep.JobID == "" {
			continue
		}

		depJob, ok := j.Backend.jobs.Get(regionKey(job.region, dep.JobID))
		if !ok {
			return dependencyPending
		}

		switch depJob.Status {
		case jobStatusFailed:
			return dependencyFailed
		case jobStatusSucceeded:
		default:
			return dependencyPending
		}
	}

	return dependencySatisfied
}

func (j *Janitor) advanceJobs(_ context.Context) {
	now := time.Now().UnixMilli()

	toAdvance, toAdvanceSvc := j.getJobsToAdvance()
	if len(toAdvance) == 0 && len(toAdvanceSvc) == 0 {
		return
	}

	j.Backend.mu.Lock("BatchJanitorAdvanceJobs")
	j.applyAdvanceRegularJobs(toAdvance, now)
	j.applyAdvanceServiceJobs(toAdvanceSvc, now)
	j.Backend.mu.Unlock()
}

func (j *Janitor) applyAdvanceRegularJobs(toAdvance []advanceKey, now int64) {
	for _, k := range toAdvance {
		job, ok := j.Backend.jobs.Get(regionKey(k.region, k.id))
		if !ok {
			continue
		}

		if k.newStatus == jobAdvanceAttemptTimedOut {
			j.Backend.applyAttemptTimeoutLocked(job, now)

			continue
		}

		job.Status = k.newStatus
		switch k.newStatus {
		case jobStatusRunning:
			job.StartedAt = &now
			job.StatusReason = ""
		case jobStatusSucceeded:
			job.StoppedAt = &now
		case jobStatusFailed:
			job.StoppedAt = &now
			job.StatusReason = "dependency failed"
		}
	}
}

func (j *Janitor) applyAdvanceServiceJobs(toAdvanceSvc []advanceKey, now int64) {
	for _, k := range toAdvanceSvc {
		if job, ok := j.Backend.serviceJobs.Get(regionKey(k.region, k.id)); ok {
			job.Status = k.newStatus
			switch k.newStatus {
			case jobStatusRunning:
				job.StartedAt = &now
			case jobStatusSucceeded:
				job.StoppedAt = &now
			}
		}
	}
}

// isTerminalJobStatus reports whether the given job status is terminal.
func isTerminalJobStatus(status string) bool {
	return status == jobStatusSucceeded || status == jobStatusFailed
}
