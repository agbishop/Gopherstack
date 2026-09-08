package glue_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glue"
)

// testJobName is the job used by the reconciler test helpers.
const testJobName = "j"

// startTestJobRun creates a job and starts one run, returning its run ID.
func startTestJobRun(t *testing.T, b *glue.InMemoryBackend) string {
	t.Helper()

	_, err := b.CreateJob(glue.Job{
		Name:    testJobName,
		Role:    "arn:aws:iam::000000000000:role/glue",
		Command: glue.JobCommand{Name: "glueetl"},
	})
	require.NoError(t, err)

	run, err := b.StartJobRun(testJobName, nil)
	require.NoError(t, err)

	return run.ID
}

// TestReconciler_NoLeakOnConstruct verifies the constructor does not start a
// background goroutine, so a backend that is never explicitly closed cannot leak the
// reconciler (the package-wide goleak TestMain enforces this across every test).
func TestReconciler_NoLeakOnConstruct(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")

	// No StartReconciler / StartWorker: nothing to stop. Close must be a safe no-op.
	b.Close()
	b.Close() // idempotent
}

// TestReconciler_StartStopClean verifies the managed reconciler starts and stops
// cleanly, that Stop blocks until the goroutine exits, and that both operations are
// idempotent.
func TestReconciler_StartStopClean(t *testing.T) {
	t.Parallel()

	tests := []struct {
		stop func(*glue.InMemoryBackend, context.CancelFunc)
		name string
	}{
		{
			name: "stop via StopReconciler",
			stop: func(b *glue.InMemoryBackend, _ context.CancelFunc) {
				b.StopReconciler()
				b.StopReconciler() // idempotent
			},
		},
		{
			name: "stop via Close",
			stop: func(b *glue.InMemoryBackend, _ context.CancelFunc) {
				b.Close()
			},
		},
		{
			name: "stop via context cancel then StopReconciler",
			stop: func(b *glue.InMemoryBackend, cancel context.CancelFunc) {
				cancel()
				// Give the goroutine a moment to observe cancellation, then ensure
				// StopReconciler still returns cleanly.
				b.StopReconciler()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := glue.NewInMemoryBackend("000000000000", "us-east-1")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			b.StartReconciler(ctx)
			b.StartReconciler(ctx) // second start is a no-op

			tc.stop(b, cancel)
		})
	}
}

// TestReconciler_LazyAdvanceJobRun verifies job-run transitions are observed on read
// even when the background reconciler is not running (lazy advancement), and that
// advancement is deterministic against an explicit clock.
func TestReconciler_LazyAdvanceJobRun(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	runID := startTestJobRun(t, b)

	// Immediately after start the run is STARTING.
	run, err := b.GetJobRun(testJobName, runID)
	require.NoError(t, err)
	assert.Equal(t, "STARTING", run.JobRunState)

	// Force advancement far into the future: STARTING→RUNNING→SUCCEEDED.
	glue.AdvanceStatesForTest(b, time.Now().Add(time.Hour))

	run, err = b.GetJobRun(testJobName, runID)
	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", run.JobRunState)
	assert.Positive(t, run.CompletedOn)
}

// TestReconciler_DeleteJob_PrunesOrphanedTimers verifies that deleting a job
// eventually prunes its job-run timer entries (jobRunReadyAt/DoneAt/TimeoutAt/
// StopAt), not just the job-run records themselves. DeleteJob clears
// b.jobRuns[name] but not these timer maps directly; pruneOrphanJobRunTimersLocked
// (called at the end of every reconcileLocked) is what actually drops the
// now-orphaned entries, keyed off b.jobRuns no longer containing the run. This
// asserts the observable effect through the existing PendingDueForTest/
// AdvanceStatesForTest test seams rather than adding a new exported accessor
// for the timer maps themselves.
func TestReconciler_DeleteJob_PrunesOrphanedTimers(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	startTestJobRun(t, b)

	farFuture := time.Now().Add(time.Hour)
	require.True(t, glue.PendingDueForTest(b, farFuture), "the started run's STARTING->RUNNING timer should be due")

	require.NoError(t, b.DeleteJob(testJobName))

	glue.AdvanceStatesForTest(b, farFuture)

	assert.False(t, glue.PendingDueForTest(b, farFuture),
		"deleting the job should prune its orphaned job-run timers, not just its job-run records")
}

// TestReconciler_LazyAdvanceCrawler verifies crawler RUNNING→READY is observed on
// read via lazy advancement.
func TestReconciler_LazyAdvanceCrawler(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateCrawler("c", "arn:aws:iam::000000000000:role/glue", "", glue.CrawlerTarget{}, nil)
	require.NoError(t, err)
	require.NoError(t, b.StartCrawler("c"))

	c, err := b.GetCrawler("c")
	require.NoError(t, err)
	assert.Equal(t, "RUNNING", c.State)

	glue.AdvanceStatesForTest(b, time.Now().Add(time.Hour))

	c, err = b.GetCrawler("c")
	require.NoError(t, err)
	assert.Equal(t, "READY", c.State)
}

// TestReconciler_BackgroundLoopAdvances verifies the running reconciler goroutine
// advances transitions without any read on the resource.
func TestReconciler_BackgroundLoopAdvances(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	b.StartReconciler(ctx)
	t.Cleanup(b.StopReconciler)

	runID := startTestJobRun(t, b)

	require.Eventually(t, func() bool {
		run, err := b.GetJobRun(testJobName, runID)
		require.NoError(t, err)

		return run.JobRunState == "SUCCEEDED"
	}, 3*time.Second, 10*time.Millisecond, "job run never reached SUCCEEDED")
}

// TestReconciler_PendingDueGuard verifies the performance guard: an idle backend
// reports no due work (so the reconciler never takes the global write lock), a
// freshly scheduled transition is not yet due, and it becomes due only once its time
// has passed.
func TestReconciler_PendingDueGuard(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")

	now := time.Now()
	assert.False(t, glue.PendingDueForTest(b, now), "empty backend must report no due work")

	startTestJobRun(t, b)

	// Just-scheduled transition is in the future: not due yet.
	assert.False(t, glue.PendingDueForTest(b, now), "freshly scheduled run must not be due")

	// Far enough in the future, the transition is due.
	assert.True(t, glue.PendingDueForTest(b, now.Add(time.Hour)), "past-due run must be reported due")
}

// TestReconciler_ResetClearsPendingWork verifies Reset clears pending lifecycle
// timers so a reset never resurrects transitions for deleted resources.
func TestReconciler_ResetClearsPendingWork(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	startTestJobRun(t, b)

	require.True(t, glue.PendingDueForTest(b, time.Now().Add(time.Hour)))

	b.Reset()

	assert.False(t, glue.PendingDueForTest(b, time.Now().Add(time.Hour)),
		"Reset must clear pending transition timers")
}
