package sagemaker

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// ---------------------------------------------------------------------------
// Lifecycle simulator delays
// ---------------------------------------------------------------------------

const (
	notebookPendingToInServiceDelay       = 250 * time.Millisecond
	notebookStoppingToStoppedDelay        = 150 * time.Millisecond
	notebookUpdatingToInService           = 200 * time.Millisecond
	trainingInProgressToCompleted         = 300 * time.Millisecond
	trainingStoppingToStopped             = 150 * time.Millisecond
	endpointCreatingToInService           = 300 * time.Millisecond
	endpointUpdatingToInService           = 250 * time.Millisecond
	inferenceComponentCreatingToInService = 300 * time.Millisecond
	inferenceComponentUpdatingToInService = 250 * time.Millisecond
	processingJobCompletionDelay          = 300 * time.Millisecond
	processingJobStopDelay                = 150 * time.Millisecond
	// aiJobInProgressToCompleted/aiJobStoppingToStopped drive the InProgress
	// -> Completed and Stopping -> Stopped transitions shared by the
	// AIBenchmarkJob, AIRecommendationJob, and generic Job families (added
	// in the sagemaker SDK bump that introduced CreateJob et al.). Kept as
	// their own constants (rather than reusing trainingInProgressToCompleted)
	// so these three new families can be retimed independently of
	// TrainingJob's FSM.
	aiJobInProgressToCompleted = 300 * time.Millisecond
	aiJobStoppingToStopped     = 150 * time.Millisecond
	// edgePackagingJobStartingToCompleted/edgePackagingJobStoppingToStopped
	// drive EdgePackagingJob's STARTING -> COMPLETED and STOPPING -> STOPPED
	// transitions. Kept as their own constants rather than reusing the
	// aiJob* pair above, matching this file's own established precedent for
	// independently-retimeable families.
	edgePackagingJobStartingToCompleted = 300 * time.Millisecond
	edgePackagingJobStoppingToStopped   = 150 * time.Millisecond
	// inferenceRecommendationsJobInProgressToCompleted/-StoppingToStopped
	// drive InferenceRecommendationsJob's own IN_PROGRESS -> COMPLETED and
	// STOPPING -> STOPPED transitions, kept independent of the aiJob*/
	// edgePackagingJob* constants above for the same retiming reason.
	inferenceRecommendationsJobInProgressToCompleted = 300 * time.Millisecond
	inferenceRecommendationsJobStoppingToStopped     = 150 * time.Millisecond
	// hpTuningJobStoppingToStopped drives HyperParameterTuningJob's own
	// Stopping -> Stopped transition. Nothing previously advanced a
	// Stopping job -- no ticker, no later call -- so
	// DescribeHyperParameterTuningJob showed Stopping for the entire
	// remaining lifetime of every stopped job.
	hpTuningJobStoppingToStopped = 150 * time.Millisecond
	// compilationJobInProgressToCompleted/compilationJobStoppingToStopped
	// drive CompilationJob's own INPROGRESS -> COMPLETED and
	// STOPPING -> STOPPED transitions. Previously nothing ever advanced
	// INPROGRESS at all, and Stop set STOPPED directly with no STOPPING
	// step, contradicting api_op_StopCompilationJob.go's own doc.
	compilationJobInProgressToCompleted = 300 * time.Millisecond
	compilationJobStoppingToStopped     = 150 * time.Millisecond
	// hpTuningJobInProgressToCompleted drives HyperParameterTuningJob's own
	// InProgress -> Completed transition. Nothing previously advanced a
	// running job off InProgress at all -- only Stop's Stopping -> Stopped
	// leg had an FSM (hpTuningJobStoppingToStopped above) -- so
	// DescribeHyperParameterTuningJob showed InProgress for the entire
	// remaining lifetime of every job that was never explicitly stopped.
	hpTuningJobInProgressToCompleted = 300 * time.Millisecond
)

// ---------------------------------------------------------------------------
// Status constants
// ---------------------------------------------------------------------------

const (
	notebookStatusStopped       = "Stopped"
	notebookStatusPending       = "Pending"
	notebookStatusStopping      = "Stopping"
	trainingJobStatusInProgress = "InProgress"
	keyNotebookInstanceArn      = "NotebookInstanceArn"
	// secondaryStatusStarting is the initial SecondaryStatus/AutoMLJobSecondaryStatus
	// value used by both TrainingJob and AutoMLJob's synthetic secondary-status FSMs.
	secondaryStatusStarting = "Starting"
)

// ---------------------------------------------------------------------------
// Lifecycle context management — per-backend context cancelled by Reset()
// ---------------------------------------------------------------------------

// resetLifecycleContext replaces the backend's lifecycle context with a fresh one,
// cancelling all pending goroutines from the previous context.
// Must be called with no lock held.
func (b *InMemoryBackend) resetLifecycleContext() {
	if b.lifecycleCancel != nil {
		b.lifecycleCancel()
	}

	parent := b.lifecycleParent
	if parent == nil {
		parent = context.Background()
	}

	ctx, cancel := context.WithCancel(parent)
	b.lifecycleCtx = ctx
	b.lifecycleCancel = cancel
}

// runDelayed runs fn after delay unless ctx is cancelled first (via Reset,
// Restore, or Shutdown, all of which cancel the lifecycle context). The goroutine
// is tracked by b.wg so Shutdown can wait for in-flight transitions to drain. fn is
// responsible for taking any locks it needs and re-checking resource existence.
//
// ctx must be the backend's lifecycle context (b.lifecycleCtx) captured by the
// caller — typically while holding b.mu — so a concurrent Reset that swaps
// b.lifecycleCtx cannot race this goroutine's select.
func (b *InMemoryBackend) runDelayed(ctx context.Context, delay time.Duration, fn func()) {
	b.wg.Go(func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		fn()
	})
}

// stopSimpleJobFSM implements the InProgress -> Stopping -> Stopped state
// machine shared by StopAIBenchmarkJob and StopAIRecommendationJob (the two
// new job families whose Stop op has no secondary-status transitions to
// track, unlike TrainingJob's/the generic Job's richer FSMs). storeOf must
// be one of the backend's own xxxStore(region) helpers (called under b.mu,
// per that helper family's convention) so this generic function stays
// consistent with every other resource family's locking discipline.
// opLabel is the lockmetrics label used for both the synchronous lock and
// the delayed goroutine's lock.
func stopSimpleJobFSM[T any](
	b *InMemoryBackend,
	opLabel, region, name string,
	storeOf func(region string) *store.Table[T],
	notFound error,
	getStatus func(*T) string,
	setStatus func(*T, string),
	setEndTime func(*T, time.Time),
	getArn func(*T) string,
) (string, error) {
	b.mu.Lock(opLabel)
	defer b.mu.Unlock()

	j, ok := storeOf(region).Get(name)
	if !ok {
		return "", notFound
	}

	if getStatus(j) == trainingJobStatusInProgress {
		setStatus(j, pipelineStatusStopping)

		b.runDelayed(b.lifecycleCtx, aiJobStoppingToStopped, func() {
			b.mu.Lock(opLabel + ".goroutine")
			defer b.mu.Unlock()

			if j2, ok2 := storeOf(region).Get(name); ok2 && getStatus(j2) == pipelineStatusStopping {
				now := time.Now()
				setStatus(j2, pipelineStatusStopped)
				setEndTime(j2, now)
			}
		})
	}

	return getArn(j), nil
}

// Shutdown cancels all pending lifecycle-transition goroutines and waits for the
// in-flight ones to finish, bounded by ctx. It implements the shutdown half of the
// service.Shutdowner contract (wired through the Handler).
func (b *InMemoryBackend) Shutdown(ctx context.Context) {
	func() {
		b.mu.Lock("Shutdown")
		defer b.mu.Unlock()

		if b.lifecycleCancel != nil {
			b.lifecycleCancel()
		}
	}()

	done := make(chan struct{})

	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}
