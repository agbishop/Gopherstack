package stepfunctions

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	sfnWorkerService   = "stepfunctions"
	prunerName         = "ExecutionPruner"
	deletionReaperName = "StateMachineDeletionReaper"
)

// Janitor is the Step Functions background worker that manages resource cleanup.
type Janitor struct {
	Backend     *InMemoryBackend
	Interval    time.Duration
	TaskTimeout time.Duration
}

// NewJanitor creates a new Janitor for the given backend.
func NewJanitor(backend *InMemoryBackend, settings Settings) *Janitor {
	interval := settings.JanitorInterval
	if interval == 0 {
		interval = defaultJanitorInterval
	}

	return &Janitor{
		Backend:  backend,
		Interval: interval,
	}
}

// Run runs the janitor loop until ctx is cancelled.
func (j *Janitor) Run(ctx context.Context) {
	g := worker.NewGroup(ctx, sfnWorkerService)
	g.Ticker(prunerName, j.Interval, j.TaskTimeout, j.sweepExecutions)
	g.Ticker(deletionReaperName, j.Interval, j.TaskTimeout, j.sweepDeletingStateMachines)

	<-ctx.Done()
	g.Stop()
}

// sweepDeletingStateMachines completes DeleteStateMachine for state machines
// that were left marked DELETING because executions were still running.
// AWS: a state machine is deleted only once all its executions complete.
func (j *Janitor) sweepDeletingStateMachines(ctx context.Context) {
	swept := j.Backend.SweepDeletingStateMachines(ctx)

	telemetry.RecordWorkerTask(sfnWorkerService, deletionReaperName, "success")
	if swept > 0 {
		telemetry.RecordWorkerItems(sfnWorkerService, deletionReaperName, swept)
		logger.Load(ctx).InfoContext(
			ctx, "Step Functions janitor: completed deferred state machine deletions", "count", swept,
		)
	}
}

func (j *Janitor) sweepExecutions(ctx context.Context) {
	pruned := j.Backend.PruneExecutions(ctx)
	evicted := j.Backend.SweepTaskTokens()

	telemetry.RecordWorkerTask(sfnWorkerService, prunerName, "success")
	if pruned > 0 {
		telemetry.RecordWorkerItems(sfnWorkerService, prunerName, pruned)
		logger.Load(ctx).InfoContext(ctx, "Step Functions janitor: pruned old executions", "count", pruned)
	}
	if evicted > 0 {
		telemetry.RecordWorkerItems(sfnWorkerService, prunerName, evicted)
		logger.Load(ctx).InfoContext(ctx, "Step Functions janitor: evicted stale task tokens", "count", evicted)
	}
}
