package ecs

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	defaultECSJanitorInterval = 30 * time.Second
	defaultStoppedTaskTTL     = time.Hour
	ecsWorkerService          = "ecs"
	stoppedTaskSweeperName    = "StoppedTaskSweeper"
)

// Janitor is the ECS background worker that evicts stopped tasks older than
// a configurable TTL to prevent unbounded growth of the tasks map.
type Janitor struct {
	Backend     *InMemoryBackend
	Interval    time.Duration
	TaskTimeout time.Duration
	taskTTL     time.Duration
}

// NewJanitor creates a new ECS Janitor for the given backend.
// If interval is zero it falls back to defaultECSJanitorInterval.
func NewJanitor(backend *InMemoryBackend, interval time.Duration) *Janitor {
	if interval == 0 {
		interval = defaultECSJanitorInterval
	}

	return &Janitor{
		Backend:  backend,
		Interval: interval,
		taskTTL:  defaultStoppedTaskTTL,
	}
}

// SetTaskTTL overrides the default stopped-task TTL. Intended for testing.
func (j *Janitor) SetTaskTTL(d time.Duration) { j.taskTTL = d }

// Run runs the janitor loop until ctx is cancelled.
func (j *Janitor) Run(ctx context.Context) {
	g := worker.NewGroup(ctx, ecsWorkerService)
	g.Ticker(stoppedTaskSweeperName, j.Interval, j.TaskTimeout, j.sweepStoppedTasks)

	<-ctx.Done()
	g.Stop()
}

// SweepOnce runs a single sweep pass. Exposed for testing.
func (j *Janitor) SweepOnce(ctx context.Context) {
	j.sweepStoppedTasks(ctx)
}

// sweepStoppedTasks removes tasks from every cluster where LastStatus is
// "STOPPED" and StoppedAt is older than the configured TTL.
func (j *Janitor) sweepStoppedTasks(ctx context.Context) {
	now := time.Now()
	cutoff := now.Add(-j.taskTTL)

	b := j.Backend

	evicted := 0
	cancelled := false

	func() {
		b.mu.Lock("JanitorSweepStoppedTasks")
		defer b.mu.Unlock()

		for _, task := range b.tasks.All() {
			select {
			case <-ctx.Done():
				cancelled = true

				return
			default:
			}

			if task.LastStatus != statusStopped {
				continue
			}

			if task.StoppedAt == nil || task.StoppedAt.IsZero() || task.StoppedAt.After(cutoff) {
				continue
			}

			b.tasks.Delete(task.TaskArn)
			b.deleteResourceTagsLocked(task.TaskArn)
			evicted++
		}
	}()

	if cancelled {
		return
	}

	telemetry.RecordWorkerItems(ecsWorkerService, stoppedTaskSweeperName, evicted)
	telemetry.RecordWorkerTask(ecsWorkerService, stoppedTaskSweeperName, "success")

	if evicted > 0 {
		logger.Load(ctx).InfoContext(ctx,
			"ECS janitor: evicted stopped tasks past TTL",
			"evicted", evicted)
	}
}
