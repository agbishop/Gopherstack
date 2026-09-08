package glue

import (
	"context"
	"fmt"
	"time"
)

// advanceJobRunState applies STARTING→RUNNING, RUNNING→SUCCEEDED or TIMEOUT,
// and STOPPING→STOPPED transitions for a single run, consulting the
// readyMap, doneMap, timeoutMap, and stopMap timing tables. Must be called
// with b.mu held.
func advanceJobRunState(run *JobRun, readyMap, doneMap, timeoutMap, stopMap map[string]time.Time, now time.Time) {
	advanceJobRunTimer(run, readyMap, now, stateStarting, func() {
		run.JobRunState = stateRunning
	})

	advanceRunningJobRun(run, doneMap, timeoutMap, now)

	advanceJobRunTimer(run, stopMap, now, stateStopping, func() {
		run.JobRunState = stateStopped
		run.CompletedOn = float64(now.Unix())
	})
}

// advanceRunningJobRun resolves a RUNNING run's eventual SUCCEEDED or TIMEOUT
// transition. A run only reaches TIMEOUT if its JobRun.Timeout deadline
// (timeoutMap) falls before its natural completion (doneMap) and has passed
// -- matching real Glue, where TIMEOUT only fires for a run still executing
// once its configured timeout elapses; a run that finishes first succeeds
// normally regardless of Timeout. Clears whichever timer entries applied so a
// terminal run is never re-evaluated. Must be called with b.mu held.
func advanceRunningJobRun(run *JobRun, doneMap, timeoutMap map[string]time.Time, now time.Time) {
	if run.JobRunState != stateRunning {
		delete(doneMap, run.ID)
		delete(timeoutMap, run.ID)

		return
	}

	doneAt, hasDone := doneMap[run.ID]
	timeoutAt, hasTimeout := timeoutMap[run.ID]

	timesOut := hasTimeout && (!hasDone || timeoutAt.Before(doneAt)) && now.After(timeoutAt)
	succeeds := !timesOut && hasDone && now.After(doneAt)

	switch {
	case timesOut:
		run.JobRunState = stateTimeout
		run.CompletedOn = float64(now.Unix())
		run.ExecutionTime = run.Timeout * secondsPerMinute
		run.ErrorMessage = fmt.Sprintf(
			"Job run timed out after exceeding the configured Timeout of %d minutes", run.Timeout,
		)
	case succeeds:
		run.JobRunState = stateSucceeded
		run.CompletedOn = float64(now.Unix())
		run.ExecutionTime = int(jobSucceededDelay.Seconds())
	default:
		return
	}

	delete(doneMap, run.ID)
	delete(timeoutMap, run.ID)
}

// advanceJobRunTimer applies apply to run if its timer in timers is due and
// run is still in fromState, then clears the timer either way. Must be
// called with b.mu held.
func advanceJobRunTimer(
	run *JobRun,
	timers map[string]time.Time,
	now time.Time,
	fromState string,
	apply func(),
) {
	if timers == nil {
		return
	}

	t, ok := timers[run.ID]
	if !ok || !now.After(t) {
		return
	}

	if run.JobRunState == fromState {
		apply()
	}

	delete(timers, run.ID)
}

// reconcileLocked applies pending lifecycle transitions as of now. Must be called
// with b.mu held. Taking now as a parameter (rather than reading the clock again)
// keeps the due-scan and the application consistent and makes advancement
// deterministically testable.
func (b *InMemoryBackend) reconcileLocked(now time.Time) {
	// Job run transitions: STARTING→RUNNING, RUNNING→SUCCEEDED/TIMEOUT, STOPPING→STOPPED.
	for jobName, runs := range b.jobRuns {
		readyMap := b.jobRunReadyAt[jobName]
		doneMap := b.jobRunDoneAt[jobName]
		timeoutMap := b.jobRunTimeoutAt[jobName]
		stopMap := b.jobRunStopAt[jobName]

		for _, run := range runs {
			advanceJobRunState(run, readyMap, doneMap, timeoutMap, stopMap, now)
		}
	}

	// Crawler transitions:
	//   RUNNING→READY  — crawl completes; create catalog tables from S3 targets.
	//   STOPPING→READY — StopCrawler was issued; the crawler winds down to READY
	//                    without creating tables (the crawl was interrupted).
	for name, readyAt := range b.crawlerReadyAt {
		if now.After(readyAt) {
			c, ok := b.crawlers.Get(name)
			if ok && c.State == stateRunning {
				c.State = stateReady
				c.LastUpdated = float64(now.Unix())
				created := b.createCrawlerTablesLocked(c)
				b.finishCrawlHistoryLocked(name, "COMPLETED", created, now)
			} else if ok && c.State == stateStopping {
				c.State = stateReady
				c.LastUpdated = float64(now.Unix())
				b.finishCrawlHistoryLocked(name, "STOPPED", 0, now)
			}

			delete(b.crawlerReadyAt, name)
		}
	}

	// Integration transitions: CREATING→ACTIVE. Real Glue Zero-ETL integrations
	// activate on their own with no client call required, the same shape as a
	// crawler's RUNNING→READY.
	for name, readyAt := range b.integrationReadyAt {
		if now.After(readyAt) {
			if ig, ok := b.integrations.Get(name); ok && ig.Status == "CREATING" {
				ig.Status = stateActive
			}

			delete(b.integrationReadyAt, name)
		}
	}

	b.pruneOrphanJobRunTimersLocked()
}

// pruneOrphanJobRunTimersLocked removes job-run timing entries whose job or run no
// longer exists. Without this, a stale due timer would make pendingDueLocked report
// work forever, so the reconciler would take the global write lock every tick — the
// exact hot-loop the performance fix is meant to avoid. Must be called with b.mu held.
func (b *InMemoryBackend) pruneOrphanJobRunTimersLocked() {
	for jobName, timers := range b.jobRunReadyAt {
		b.pruneJobTimerMapLocked(jobName, timers, b.jobRunReadyAt)
	}

	for jobName, timers := range b.jobRunDoneAt {
		b.pruneJobTimerMapLocked(jobName, timers, b.jobRunDoneAt)
	}

	for jobName, timers := range b.jobRunTimeoutAt {
		b.pruneJobTimerMapLocked(jobName, timers, b.jobRunTimeoutAt)
	}

	for jobName, timers := range b.jobRunStopAt {
		b.pruneJobTimerMapLocked(jobName, timers, b.jobRunStopAt)
	}
}

// pruneJobTimerMapLocked drops entries in a single job's timer sub-map for runs that
// no longer exist, and drops the whole sub-map when the job or all its timers are
// gone. Must be called with b.mu held.
func (b *InMemoryBackend) pruneJobTimerMapLocked(
	jobName string,
	timers map[string]time.Time,
	parent map[string]map[string]time.Time,
) {
	live := make(map[string]struct{}, len(b.jobRuns[jobName]))
	for _, run := range b.jobRuns[jobName] {
		live[run.ID] = struct{}{}
	}

	for runID := range timers {
		if _, ok := live[runID]; !ok {
			delete(timers, runID)
		}
	}

	if len(timers) == 0 {
		delete(parent, jobName)
	}
}

// defaultReconcileInterval is how often the managed reconciler wakes to advance
// due lifecycle transitions (job-run STARTING→RUNNING→SUCCEEDED and crawler
// RUNNING/STOPPING→READY). It is derived from the shortest transition delay so
// transitions surface promptly without busy-spinning.
const defaultReconcileInterval = jobTransitionDelay / reconcilerTickDivisor

// pendingDueLocked reports whether any scheduled lifecycle transition is due at
// now. Callers MUST hold at least b.mu.RLock. It is the cheap guard that lets the
// reconciler (and lazy read-path advancement) skip taking the global write lock
// on the overwhelmingly common tick where nothing has come due yet.
func (b *InMemoryBackend) pendingDueLocked(now time.Time) bool {
	return nestedTimerDue(b.jobRunReadyAt, now) ||
		nestedTimerDue(b.jobRunDoneAt, now) ||
		nestedTimerDue(b.jobRunTimeoutAt, now) ||
		nestedTimerDue(b.jobRunStopAt, now) ||
		flatTimerDue(b.crawlerReadyAt, now) ||
		flatTimerDue(b.integrationReadyAt, now)
}

// nestedTimerDue reports whether any timer in a jobName→runID→time map is due at now.
func nestedTimerDue(timers map[string]map[string]time.Time, now time.Time) bool {
	for _, sub := range timers {
		if flatTimerDue(sub, now) {
			return true
		}
	}

	return false
}

// flatTimerDue reports whether any timer in a name→time map is due at now.
func flatTimerDue(timers map[string]time.Time, now time.Time) bool {
	for _, t := range timers {
		if now.After(t) {
			return true
		}
	}

	return false
}

// advanceStates applies every lifecycle transition whose scheduled time has passed.
// It first scans under a read lock and only upgrades to the global write lock when
// there is work to do, keeping the common (nothing-due) case cheap — this is the
// performance fix for the previous reconciler, which took the write lock every tick
// regardless of pending work.
//
// It is safe for concurrent use and is called both lazily on reads (so SDK waiters
// polling GetJobRun/GetCrawler always observe the true state, even when the
// background reconciler is not running) and periodically by the reconciler loop.
func (b *InMemoryBackend) advanceStates(now time.Time) {
	b.mu.RLock("advanceStates.scan")
	due := b.pendingDueLocked(now)
	b.mu.RUnlock()

	if !due {
		return
	}

	b.mu.Lock("advanceStates.apply")
	defer b.mu.Unlock()

	// reconcileLocked re-checks each timer against now, so it is idempotent when a
	// lazy read and the reconciler race to advance the same resource.
	b.reconcileLocked(now)
}

// StartReconciler starts the managed background reconciler that advances Glue job-run
// and crawler lifecycle transitions. It replaces the previous unmanaged
// `go b.runReconciler()` (which leaked because nothing called Close) with a single
// goroutine owning a stop channel and WaitGroup, so it can be cancelled
// deterministically via ctx or StopReconciler. Calling it while already running is a
// no-op.
//
// ctx originates from the service framework's background-worker lifecycle, so no
// context.Background() is introduced here.
func (b *InMemoryBackend) StartReconciler(ctx context.Context) {
	b.reconcileMu.Lock()
	defer b.reconcileMu.Unlock()

	if b.reconcileOn {
		return
	}

	if b.reconcileInterval <= 0 {
		b.reconcileInterval = defaultReconcileInterval
	}

	stop := make(chan struct{})
	b.reconcileStop = stop
	b.reconcileOn = true

	b.reconcileWG.Add(1)

	go b.reconcileLoop(ctx, stop, b.reconcileInterval)
}

// reconcileLoop is the reconciler goroutine body. It exits cleanly when either the
// context is cancelled or the stop channel is closed, then signals the WaitGroup.
func (b *InMemoryBackend) reconcileLoop(ctx context.Context, stop <-chan struct{}, interval time.Duration) {
	defer b.reconcileWG.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			b.advanceStates(time.Now())
		}
	}
}

// StopReconciler signals the reconciler to stop and blocks until the goroutine has
// exited, guaranteeing no leaked goroutine survives shutdown. It is idempotent and
// safe to call even when the reconciler was never started.
func (b *InMemoryBackend) StopReconciler() {
	wasOn := func() bool {
		b.reconcileMu.Lock()
		defer b.reconcileMu.Unlock()

		if !b.reconcileOn {
			return false
		}

		b.reconcileOn = false
		close(b.reconcileStop)

		return true
	}()

	if !wasOn {
		return
	}

	b.reconcileWG.Wait()
}

// Close stops the background reconciler and waits for its goroutine to exit. It is
// retained for callers (and tests) that manage a backend's lifecycle directly; it is
// an idempotent alias for StopReconciler.
func (b *InMemoryBackend) Close() { b.StopReconciler() }
