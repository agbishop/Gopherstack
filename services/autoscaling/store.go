package autoscaling

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// completedProgress is the progress value for a successfully completed scaling activity.
const completedProgress = int32(100)

// maxDesiredCapacity is the upper bound on DesiredCapacity for any ASG, used to
// cap user-supplied values and prevent excessive slice allocations
// (go/slice-memory-allocation-excessive-size).
const maxDesiredCapacity = 100

const (
	lifecycleActionContinue = "CONTINUE"
	lifecycleActionAbandon  = "ABANDON"
	// healthStatusHealthy is the health status for a healthy instance.
	healthStatusHealthy = "Healthy"
	// lifecycleStateInService is the lifecycle state for a running, healthy instance.
	lifecycleStateInService = "InService"
	// lifecycleStatePendingWait is the lifecycle state for an instance launching but
	// paused for an EC2_INSTANCE_LAUNCHING lifecycle hook to complete.
	lifecycleStatePendingWait = "Pending:Wait"
	// lifecycleStateTerminatingWait is the lifecycle state for an instance being
	// terminated but paused for an EC2_INSTANCE_TERMINATING lifecycle hook to complete.
	lifecycleStateTerminatingWait = "Terminating:Wait"
	// transitionLaunching and transitionTerminating are the two AWS-defined lifecycle
	// hook transition points.
	transitionLaunching   = "autoscaling:EC2_INSTANCE_LAUNCHING"
	transitionTerminating = "autoscaling:EC2_INSTANCE_TERMINATING"
	// statusCodeSuccessful is the status code for a successfully completed scaling activity.
	statusCodeSuccessful = "Successful"
	// statusInProgress is the status for an in-progress instance refresh or scaling activity.
	statusInProgress = "InProgress"
	// statusPending is the status for an instance refresh that has not yet started.
	statusPending = "Pending"
	// Instance refresh terminal/transitional statuses (autoscaling@v1.70.4
	// types/enums.go:289-303, InstanceRefreshStatus).
	statusSuccessful         = "Successful"
	statusCancelling         = "Cancelling"
	statusCancelled          = "Cancelled"
	statusRollbackInProgress = "RollbackInProgress"
	statusRollbackSuccessful = "RollbackSuccessful"
	// instanceRefreshTransitionDelay is the simulated async delay before an
	// instance refresh (or its cancel/rollback) reaches a terminal status,
	// matching the time.AfterFunc pattern used by lifecycle hooks and eks's
	// clusterTransitionDelay.
	instanceRefreshTransitionDelay = 100 * time.Millisecond
	// granularity1Minute is the only supported CloudWatch metric granularity.
	granularity1Minute = "1Minute"
	// lbStateAdded is the state for a load balancer that has been attached to the ASG.
	lbStateAdded = "InService"
	// maxAccountASGs is the simulated account limit for Auto Scaling groups.
	maxAccountASGs = int32(200)
	// maxAccountLaunchConfigs is the simulated account limit for launch configurations.
	maxAccountLaunchConfigs = int32(200)
	// percentDivisor is used when computing PercentChangeInCapacity adjustments.
	percentDivisor = 100.0
)

// defaultAvailabilityZone is the fallback AZ used when none is specified.
const defaultAvailabilityZone = "us-east-1a"

// InMemoryBackend implements StorageBackend using in-memory maps.
type InMemoryBackend struct {
	// ec2Launcher, when set (see SetEC2Launcher), routes scale-out/scale-in
	// through the EC2 backend so ASG membership stays consistent with EC2
	// DescribeInstances. Nil preserves the historical fabricated-instance-ID
	// behavior.
	ec2Launcher EC2Launcher
	// elbv2Registrar, when set (see SetELBv2Registrar), registers/deregisters
	// real ELBv2 targets as group membership and TargetGroupARNs change. Nil
	// preserves the historical behavior of TargetGroupARNs/LoadBalancerNames
	// being stored and echoed with no effect on ELBv2.
	elbv2Registrar ELBv2TargetRegistrar
	// elbRegistrar, when set (see SetELBRegistrar), registers/deregisters real
	// classic ELB instances as group membership and LoadBalancerNames change.
	// Nil preserves the historical behavior of LoadBalancerNames being stored
	// and echoed with no effect on classic ELB.
	elbRegistrar            ELBInstanceRegistrar
	groups                  *store.Table[AutoScalingGroup]
	launchConfigurations    *store.Table[LaunchConfiguration]
	activities              map[string][]ScalingActivity
	scheduledActions        *store.Table[ScheduledAction]
	scheduledActionsByGroup *store.Index[ScheduledAction]
	instanceRefreshes       map[string][]*InstanceRefresh
	lifecycleHooks          *store.Table[LifecycleHook]
	lifecycleHooksByGroup   *store.Index[LifecycleHook]
	scalingPolicies         *store.Table[ScalingPolicy]
	scalingPoliciesByGroup  *store.Index[ScalingPolicy]
	notificationConfigs     map[string][]*NotificationConfiguration
	warmPools               *store.Table[WarmPool]
	// pendingHookTokens is a *store.Table for Get/Put/Delete/Range convenience
	// but is deliberately NOT registered on registry — see registerAllTables.
	pendingHookTokens *store.Table[pendingHookAction]
	// pendingRefreshActions tracks in-flight instance-refresh transition
	// timers, keyed by InstanceRefreshID. Deliberately NOT registered on
	// registry, for the same reason as pendingHookTokens.
	pendingRefreshActions *store.Table[pendingRefreshAction]
	registry              *store.Registry
	// instanceIndex maps instanceID → groupName for O(1) lookup.
	instanceIndex map[string]string
	mu            *lockmetrics.RWMutex
	// nextHookSeq assigns LifecycleHook.Sequence on first registration (see
	// putLifecycleHookLocked); recomputed from restored data by Restore.
	nextHookSeq int64
}

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend() *InMemoryBackend {
	b := &InMemoryBackend{
		activities:          make(map[string][]ScalingActivity),
		instanceRefreshes:   make(map[string][]*InstanceRefresh),
		notificationConfigs: make(map[string][]*NotificationConfiguration),
		instanceIndex:       make(map[string]string),
		registry:            store.NewRegistry(),
		mu:                  lockmetrics.New("autoscaling"),
	}
	registerAllTables(b)

	return b
}

// Close stops any in-flight lifecycle-hook and instance-refresh timers so their goroutines do
// not outlive the backend. It is safe to call multiple times.
func (b *InMemoryBackend) Close() {
	b.mu.Lock("Close")
	defer b.mu.Unlock()

	b.pendingHookTokens.Range(func(action *pendingHookAction) bool {
		action.timer.Stop()

		return true
	})
	b.pendingHookTokens.Reset()

	b.pendingRefreshActions.Range(func(action *pendingRefreshAction) bool {
		action.timer.Stop()

		return true
	})
	b.pendingRefreshActions.Reset()
}

// Purge removes all AutoScaling groups and launch configurations created before the cutoff time.
func (b *InMemoryBackend) Purge(ctx context.Context, cutoff time.Time) {
	if ctx.Err() != nil {
		return
	}

	b.mu.Lock("Purge")
	defer b.mu.Unlock()

	// 1. Purge groups
	for _, g := range b.groups.All() {
		if ctx.Err() != nil {
			return
		}
		if g.CreatedTime.Before(cutoff) {
			name := g.AutoScalingGroupName
			b.cleanupHookTimers(name, "")
			b.cleanupRefreshTimers(name)
			b.groups.Delete(name)
			delete(b.activities, name)
			b.deleteScheduledActionsForGroupLocked(name)
			delete(b.instanceRefreshes, name)
			b.deleteLifecycleHooksForGroupLocked(name)
			b.deleteScalingPoliciesForGroupLocked(name)
			delete(b.notificationConfigs, name)
			b.warmPools.Delete(name)
		}
	}

	// 2. Purge launch configurations
	for _, lc := range b.launchConfigurations.All() {
		if ctx.Err() != nil {
			return
		}
		if lc.CreatedTime.Before(cutoff) {
			b.launchConfigurations.Delete(lc.LaunchConfigurationName)
		}
	}
}
