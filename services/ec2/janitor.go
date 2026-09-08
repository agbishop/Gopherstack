package ec2

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	defaultJanitorInterval  = time.Minute
	defaultTerminatedTTL    = time.Hour
	defaultCancelledSpotTTL = 6 * time.Hour // AWS shows cancelled spot requests for ~6 hours

	instanceSweeperComponent = "TerminatedInstanceCleaner"
	spotSweeperComponent     = "CancelledSpotRequestCleaner"
	janitorWorkerServiceName = "ec2"
)

// Janitor is the EC2 background worker that sweeps terminated instances after
// a configurable TTL, matching the AWS behavior where terminated instances
// remain visible for approximately one hour. It also removes cancelled/closed
// spot instance requests after a separate TTL.
type Janitor struct {
	Backend          *InMemoryBackend `json:"backend,omitempty"`
	Interval         time.Duration    `json:"interval"`
	TerminatedTTL    time.Duration    `json:"terminatedTTL"`
	CancelledSpotTTL time.Duration    `json:"cancelledSpotTTL"`
	TaskTimeout      time.Duration    `json:"taskTimeout"`
}

// NewJanitor creates a new EC2 Janitor for the given backend.
// If interval, terminatedTTL, or cancelledSpotTTL are zero, defaults are used.
func NewJanitor(
	backend *InMemoryBackend,
	interval, terminatedTTL, cancelledSpotTTL time.Duration,
) *Janitor {
	if interval == 0 {
		interval = defaultJanitorInterval
	}

	if terminatedTTL == 0 {
		terminatedTTL = defaultTerminatedTTL
	}

	if cancelledSpotTTL == 0 {
		cancelledSpotTTL = defaultCancelledSpotTTL
	}

	return &Janitor{
		Backend:          backend,
		Interval:         interval,
		TerminatedTTL:    terminatedTTL,
		CancelledSpotTTL: cancelledSpotTTL,
	}
}

// Run runs the janitor loop until ctx is cancelled.
func (j *Janitor) Run(ctx context.Context) {
	g := worker.NewGroup(ctx, janitorWorkerServiceName)
	g.Ticker(instanceSweeperComponent, j.Interval, j.TaskTimeout, j.SweepOnce)

	<-ctx.Done()
	g.Stop()
}

// SweepOnce runs a single sweep pass. Exposed for testing.
func (j *Janitor) SweepOnce(ctx context.Context) {
	j.sweepTerminatedInstances(ctx)
	j.sweepCancelledSpotRequests(ctx)
}

// sweepTerminatedInstances removes instances that have been in the terminated
// state longer than TerminatedTTL and cleans up their tags. As a defensive
// measure it also removes any network interfaces still attached to swept
// instances (e.g. state restored from a snapshot predating the ENI cleanup).
func (j *Janitor) sweepTerminatedInstances(ctx context.Context) {
	cutoff := time.Now().Add(-j.TerminatedTTL)

	j.Backend.mu.Lock("sweepTerminatedInstances")

	var swept []string

	for _, inst := range j.Backend.instances.All() {
		id := instancesKeyFn(inst)
		if inst.State == StateTerminated && !inst.TerminatedAt.IsZero() &&
			inst.TerminatedAt.Before(cutoff) {
			swept = append(swept, id)
			j.Backend.instances.Delete(id)
			delete(j.Backend.tags, id)

			// Defensive: remove any ENIs still referencing this instance
			// (can happen when state is restored from a pre-cleanup snapshot).
			for _, eni := range j.Backend.networkInterfaces.All() {
				eniID := networkInterfacesKeyFn(eni)
				if eni.InstanceID == id {
					j.Backend.recycleENIIPsLocked(eni)
					j.Backend.networkInterfaces.Delete(eniID)
					delete(j.Backend.tags, eniID)
					delete(j.Backend.niIPv6Addresses, eniID)
				}
			}
		}
	}

	j.Backend.mu.Unlock()

	count := len(swept)

	telemetry.RecordWorkerTask(janitorWorkerServiceName, instanceSweeperComponent, "success")

	if count == 0 {
		return
	}

	telemetry.RecordWorkerItems(janitorWorkerServiceName, instanceSweeperComponent, count)

	for _, id := range swept {
		logger.Load(ctx).
			InfoContext(ctx, "EC2 janitor: terminated instance swept", "instanceID", id)
	}
}

// sweepCancelledSpotRequests removes spot instance requests that have been in
// the stateCancelled or "closed" state longer than CancelledSpotTTL.
// In AWS, cancelled/closed spot requests remain visible for approximately
// 6 hours before they are permanently removed.
func (j *Janitor) sweepCancelledSpotRequests(ctx context.Context) {
	cutoff := time.Now().Add(-j.CancelledSpotTTL)

	j.Backend.mu.Lock("sweepCancelledSpotRequests")

	var swept []string

	for _, req := range j.Backend.spotRequests.All() {
		id := spotRequestsKeyFn(req)
		terminal := req.State == stateCancelled || req.State == "closed"
		if terminal && !req.CancelledAt.IsZero() && req.CancelledAt.Before(cutoff) {
			swept = append(swept, id)
			j.Backend.spotRequests.Delete(id)
			delete(j.Backend.tags, id)
		}
	}

	j.Backend.mu.Unlock()

	count := len(swept)

	telemetry.RecordWorkerTask(janitorWorkerServiceName, spotSweeperComponent, "success")

	if count == 0 {
		return
	}

	telemetry.RecordWorkerItems(janitorWorkerServiceName, spotSweeperComponent, count)

	for _, id := range swept {
		logger.Load(ctx).
			InfoContext(ctx, "EC2 janitor: cancelled spot request swept", "spotRequestID", id)
	}
}
