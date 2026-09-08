package autoscaling

import (
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

// scalingActivityCause builds Activity.Cause (required, types.go:298) in AWS's
// documented "At <time> a user request <verb> instance <id>." narrative form.
func scalingActivityCause(verb, instanceID string) string {
	return fmt.Sprintf("At %s a user request %s instance %s.", time.Now().UTC().Format(time.RFC3339), verb, instanceID)
}

// AttachInstances adds the given instance IDs to the specified Auto Scaling group.
func (b *InMemoryBackend) AttachInstances(groupName string, instanceIDs []string) error {
	b.mu.Lock("AttachInstances")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	existing := make(map[string]bool, len(g.Instances))
	for _, inst := range g.Instances {
		existing[inst.InstanceID] = true
	}

	az := defaultAvailabilityZone
	if len(g.AvailabilityZones) > 0 {
		az = g.AvailabilityZones[0]
	}

	added := make([]string, 0, len(instanceIDs))

	for _, id := range instanceIDs {
		if existing[id] {
			continue
		}

		g.Instances = append(g.Instances, Instance{
			InstanceID:              id,
			AvailabilityZone:        az,
			LifecycleState:          lifecycleStateInService,
			HealthStatus:            healthStatusHealthy,
			LaunchConfigurationName: g.LaunchConfigurationName,
			InstanceType:            "t2.micro",
		})
		added = append(added, id)
	}

	b.registerELBTargets(added, g.TargetGroupARNs)
	b.registerELBInstances(added, g.LoadBalancerNames)

	return nil
}

// TerminateInstanceInAutoScalingGroup terminates a specific instance in an ASG.
// When shouldDecrement is true, MinSize is decremented (capped at 0) and DesiredCapacity is
// decreased by 1 without a replacement. Otherwise a replacement is launched.
func (b *InMemoryBackend) TerminateInstanceInAutoScalingGroup(
	instanceID string,
	shouldDecrement bool,
) (*ScalingActivity, error) {
	b.mu.Lock("TerminateInstanceInAutoScalingGroup")
	defer b.mu.Unlock()

	groupName, ok := b.instanceIndex[instanceID]
	if !ok {
		return nil, fmt.Errorf("%w: instance %q not found in any auto scaling group", ErrInstanceNotFound, instanceID)
	}

	targetGroup, _ := b.groups.Get(groupName)
	if targetGroup == nil {
		return nil, fmt.Errorf("%w: instance %q not found in any auto scaling group", ErrInstanceNotFound, instanceID)
	}

	// When a terminating lifecycle hook is registered, the instance is paused in
	// Terminating:Wait until CompleteLifecycleAction is called or the hook's
	// HeartbeatTimeout elapses; the actual removal/replacement is deferred to
	// finishTermination. This mirrors real AWS behavior instead of terminating
	// instantly regardless of configured hooks.
	if hook := firstHookInChain(b.lifecycleHooksByGroup.Get(groupName), transitionTerminating); hook != nil {
		disposition := terminationReplace
		if shouldDecrement {
			disposition = terminationDecrement
		}

		for i := range targetGroup.Instances {
			if targetGroup.Instances[i].InstanceID == instanceID {
				b.armLifecycleWait(targetGroup, hook, &targetGroup.Instances[i], transitionTerminating, disposition)

				break
			}
		}

		activity := ScalingActivity{
			ActivityID:           uuid.NewString(),
			AutoScalingGroupName: targetGroup.AutoScalingGroupName,
			Description: "Terminating EC2 instance: " + instanceID +
				" (waiting for lifecycle hook '" + hook.LifecycleHookName + "')",
			Cause:      scalingActivityCause("terminated", instanceID),
			StatusCode: statusInProgress,
			Progress:   50, //nolint:mnd // AWS reports partial progress mid-wait; no finer granularity to model
			StartTime:  time.Now(),
		}
		b.activities[groupName] = append(b.activities[groupName], activity)

		return &activity, nil
	}

	// Remove the instance from the group.
	newInstances := make([]Instance, 0, len(targetGroup.Instances)-1)

	for _, inst := range targetGroup.Instances {
		if inst.InstanceID != instanceID {
			newInstances = append(newInstances, inst)
		}
	}

	targetGroup.Instances = newInstances
	delete(b.instanceIndex, instanceID)
	b.terminateInEC2([]string{instanceID})
	b.deregisterELBTargets([]string{instanceID}, targetGroup.TargetGroupARNs)
	b.deregisterELBInstances([]string{instanceID}, targetGroup.LoadBalancerNames)

	if shouldDecrement {
		if targetGroup.DesiredCapacity > 0 {
			targetGroup.DesiredCapacity--
		}

		if targetGroup.MinSize > 0 {
			targetGroup.MinSize--
		}
	} else {
		// Launch a replacement to maintain DesiredCapacity.
		oldLen := len(targetGroup.Instances)
		targetGroup.Instances = b.adjustInstances(targetGroup, targetGroup.Instances, targetGroup.DesiredCapacity)

		for _, inst := range targetGroup.Instances[oldLen:] {
			b.instanceIndex[inst.InstanceID] = targetGroup.AutoScalingGroupName
		}

		b.gateNewLaunchInstances(targetGroup, oldLen)
	}

	activity := ScalingActivity{
		ActivityID:           uuid.NewString(),
		AutoScalingGroupName: targetGroup.AutoScalingGroupName,
		Description:          "Terminating EC2 instance: " + instanceID,
		Cause:                scalingActivityCause("terminated", instanceID),
		StatusCode:           statusCodeSuccessful,
		Progress:             completedProgress,
		StartTime:            time.Now(),
		EndTime:              time.Now(),
	}

	b.activities[targetGroup.AutoScalingGroupName] = append(
		b.activities[targetGroup.AutoScalingGroupName],
		activity,
	)

	return &activity, nil
}

// DescribeAutoScalingInstances returns instance details across all ASGs, optionally filtered by instance ID.
func (b *InMemoryBackend) DescribeAutoScalingInstances(instanceIDs []string) ([]InstanceDetails, error) {
	b.mu.RLock("DescribeAutoScalingInstances")
	defer b.mu.RUnlock()

	idFilter := make(map[string]bool, len(instanceIDs))

	for _, id := range instanceIDs {
		idFilter[id] = true
	}

	var result []InstanceDetails

	for _, g := range b.groups.All() {
		for _, inst := range g.Instances {
			if len(idFilter) > 0 && !idFilter[inst.InstanceID] {
				continue
			}

			result = append(result, InstanceDetails{
				AutoScalingGroupName:    g.AutoScalingGroupName,
				InstanceID:              inst.InstanceID,
				AvailabilityZone:        inst.AvailabilityZone,
				LifecycleState:          inst.LifecycleState,
				HealthStatus:            inst.HealthStatus,
				LaunchConfigurationName: inst.LaunchConfigurationName,
				InstanceType:            inst.InstanceType,
				ProtectedFromScaleIn:    inst.ProtectedFromScaleIn,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].InstanceID < result[j].InstanceID
	})

	return result, nil
}

// gateNewLaunchInstances transitions newly-appended instances (g.Instances[startIdx:])
// to Pending:Wait and arms a heartbeat timer when the group has an active
// EC2_INSTANCE_LAUNCHING lifecycle hook. It is a no-op when no such hook exists, so
// callers can invoke it unconditionally after adding instances. Must be called with
// b.mu held (write lock).
func (b *InMemoryBackend) gateNewLaunchInstances(g *AutoScalingGroup, startIdx int) {
	hook := firstHookInChain(b.lifecycleHooksByGroup.Get(g.AutoScalingGroupName), transitionLaunching)
	if hook == nil {
		return
	}

	for i := startIdx; i < len(g.Instances); i++ {
		// Disposition is ignored for transitionLaunching waits (see
		// applyLifecycleResult); terminationReplace is passed as the neutral zero
		// value.
		b.armLifecycleWait(g, hook, &g.Instances[i], transitionLaunching, terminationReplace)
	}
}

// removeInstanceByID removes the named instance from the group and its index,
// if present, and terminates it in EC2 (when an EC2Launcher is wired — see
// terminateInEC2). Must be called with b.mu held (write lock).
func (b *InMemoryBackend) removeInstanceByID(g *AutoScalingGroup, instanceID string) {
	for i, inst := range g.Instances {
		if inst.InstanceID == instanceID {
			g.Instances = append(g.Instances[:i], g.Instances[i+1:]...)

			break
		}
	}

	delete(b.instanceIndex, instanceID)
	b.terminateInEC2([]string{instanceID})
	b.deregisterELBTargets([]string{instanceID}, g.TargetGroupARNs)
	b.deregisterELBInstances([]string{instanceID}, g.LoadBalancerNames)
}

// finishTermination completes a deferred termination once its Terminating:Wait hook
// resolves: removes the instance, applies the originally-requested capacity
// adjustment per action.Disposition (decrement / replacement launch / already
// applied by the caller), and records the completion activity. Must be called with
// b.mu held (write lock).
func (b *InMemoryBackend) finishTermination(g *AutoScalingGroup, action *pendingHookAction) {
	found := false

	for _, inst := range g.Instances {
		if inst.InstanceID == action.InstanceID {
			found = true

			break
		}
	}

	if !found {
		return
	}

	b.removeInstanceByID(g, action.InstanceID)

	switch action.Disposition {
	case terminationDecrement:
		if g.DesiredCapacity > 0 {
			g.DesiredCapacity--
		}

		if g.MinSize > 0 {
			g.MinSize--
		}
	case terminationCapacityPreset:
		// DesiredCapacity/MinSize were already adjusted by applyScaleIn before this
		// wait was armed; the removal above is all that's needed.
	case terminationReplace:
		fallthrough
	default:
		oldLen := len(g.Instances)
		g.Instances = b.adjustInstances(g, g.Instances, g.DesiredCapacity)

		for _, inst := range g.Instances[oldLen:] {
			b.instanceIndex[inst.InstanceID] = g.AutoScalingGroupName
		}

		b.gateNewLaunchInstances(g, oldLen)
	}

	b.activities[g.AutoScalingGroupName] = append(b.activities[g.AutoScalingGroupName], ScalingActivity{
		ActivityID:           uuid.NewString(),
		AutoScalingGroupName: g.AutoScalingGroupName,
		Description:          "Terminating EC2 instance: " + action.InstanceID,
		Cause:                scalingActivityCause("terminated", action.InstanceID),
		StatusCode:           statusCodeSuccessful,
		Progress:             completedProgress,
		StartTime:            time.Now(),
		EndTime:              time.Now(),
	})
}

// DetachInstances removes instances from the ASG, optionally decrementing desired capacity.
func (b *InMemoryBackend) DetachInstances(
	groupName string, instanceIDs []string, shouldDecrement bool,
) ([]ScalingActivity, error) {
	b.mu.Lock("DetachInstances")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	idSet := make(map[string]bool, len(instanceIDs))
	for _, id := range instanceIDs {
		idSet[id] = true
	}

	newInstances := make([]Instance, 0, len(g.Instances))
	detachedIDs := make([]string, 0, len(instanceIDs))

	for _, inst := range g.Instances {
		if idSet[inst.InstanceID] {
			detachedIDs = append(detachedIDs, inst.InstanceID)

			continue
		}

		newInstances = append(newInstances, inst)
	}

	detached := len(g.Instances) - len(newInstances)
	g.Instances = newInstances

	// Detached instances stop being ASG-managed, including their target group
	// membership, mirroring real AWS's default deregister-on-detach behavior.
	b.deregisterELBTargets(detachedIDs, g.TargetGroupARNs)
	b.deregisterELBInstances(detachedIDs, g.LoadBalancerNames)

	if shouldDecrement && detached > 0 {
		delta := int32(detached) //nolint:gosec // detached <= len(g.Instances), bounded by maxDesiredCapacity
		if g.DesiredCapacity >= delta {
			g.DesiredCapacity -= delta
		} else {
			g.DesiredCapacity = 0
		}
	}

	activities := make([]ScalingActivity, 0, len(instanceIDs))
	for _, id := range instanceIDs {
		act := ScalingActivity{
			ActivityID:           uuid.NewString(),
			AutoScalingGroupName: groupName,
			Description:          "Detaching EC2 instance: " + id,
			Cause:                scalingActivityCause("detached", id),
			StatusCode:           statusCodeSuccessful,
			Progress:             completedProgress,
			StartTime:            time.Now(),
			EndTime:              time.Now(),
		}
		b.activities[groupName] = append(b.activities[groupName], act)
		activities = append(activities, act)
	}

	return activities, nil
}

// EnterStandby puts instances into standby mode.
func (b *InMemoryBackend) EnterStandby(
	groupName string,
	instanceIDs []string,
	decrementCapacity bool,
) ([]ScalingActivity, error) {
	b.mu.Lock("EnterStandby")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	idSet := make(map[string]bool, len(instanceIDs))
	for _, id := range instanceIDs {
		idSet[id] = true
	}

	count := 0
	for i := range g.Instances {
		if idSet[g.Instances[i].InstanceID] {
			g.Instances[i].LifecycleState = "Standby"
			count++
		}
	}

	if decrementCapacity && count > 0 {
		delta := int32(count)
		if g.DesiredCapacity >= delta {
			g.DesiredCapacity -= delta
		} else {
			g.DesiredCapacity = 0
		}
	}

	activities := make([]ScalingActivity, 0, len(instanceIDs))
	for _, id := range instanceIDs {
		act := ScalingActivity{
			ActivityID:           uuid.NewString(),
			AutoScalingGroupName: groupName,
			Description:          "Moving EC2 instance to Standby: " + id,
			Cause:                scalingActivityCause("moved to standby", id),
			StatusCode:           statusCodeSuccessful,
			Progress:             completedProgress,
			StartTime:            time.Now(),
			EndTime:              time.Now(),
		}
		b.activities[groupName] = append(b.activities[groupName], act)
		activities = append(activities, act)
	}

	return activities, nil
}

// ExitStandby moves instances from standby back to InService.
func (b *InMemoryBackend) ExitStandby(groupName string, instanceIDs []string) ([]ScalingActivity, error) {
	b.mu.Lock("ExitStandby")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	idSet := make(map[string]bool, len(instanceIDs))
	for _, id := range instanceIDs {
		idSet[id] = true
	}

	for i := range g.Instances {
		if idSet[g.Instances[i].InstanceID] {
			g.Instances[i].LifecycleState = lifecycleStateInService
		}
	}

	activities := make([]ScalingActivity, 0, len(instanceIDs))
	for _, id := range instanceIDs {
		act := ScalingActivity{
			ActivityID:           uuid.NewString(),
			AutoScalingGroupName: groupName,
			Description:          "Moving EC2 instance out of Standby: " + id,
			Cause:                scalingActivityCause("moved out of standby", id),
			StatusCode:           statusCodeSuccessful,
			Progress:             completedProgress,
			StartTime:            time.Now(),
			EndTime:              time.Now(),
		}
		b.activities[groupName] = append(b.activities[groupName], act)
		activities = append(activities, act)
	}

	return activities, nil
}

// withinHealthCheckGracePeriod reports whether inst is still inside g's
// HealthCheckGracePeriod as of now, i.e. an Unhealthy mark on it should be
// silently ignored when the caller asked to respect the grace period.
func withinHealthCheckGracePeriod(g *AutoScalingGroup, inst *Instance, now time.Time) bool {
	if g.HealthCheckGracePeriod <= 0 || inst.LaunchTime.IsZero() {
		return false
	}

	grace := time.Duration(g.HealthCheckGracePeriod) * time.Second

	return now.Sub(inst.LaunchTime) < grace
}

// findInstanceLocked returns the group and in-place instance pointer for
// instanceID across every group, or nil, nil if not found. Must be called
// with b.mu held.
func (b *InMemoryBackend) findInstanceLocked(instanceID string) (*AutoScalingGroup, *Instance) {
	for _, g := range b.groups.All() {
		for i := range g.Instances {
			if g.Instances[i].InstanceID == instanceID {
				return g, &g.Instances[i]
			}
		}
	}

	return nil, nil
}

// SetInstanceHealth sets the health status of an instance across all ASGs.
// When shouldRespectGracePeriod is true and the instance launched within the group's
// HealthCheckGracePeriod, the unhealthy mark is silently ignored (AWS behavior).
func (b *InMemoryBackend) SetInstanceHealth(
	instanceID string,
	healthStatus string,
	shouldRespectGracePeriod bool,
) error {
	b.mu.Lock("SetInstanceHealth")
	defer b.mu.Unlock()

	if healthStatus != healthStatusHealthy && healthStatus != "Unhealthy" {
		return fmt.Errorf("%w: HealthStatus must be Healthy or Unhealthy, got %q",
			ErrInvalidParameter, healthStatus)
	}

	g, inst := b.findInstanceLocked(instanceID)
	if inst == nil {
		return fmt.Errorf("%w: instance %q not found in any auto scaling group", ErrInstanceNotFound, instanceID)
	}

	if shouldRespectGracePeriod && healthStatus == "Unhealthy" && withinHealthCheckGracePeriod(g, inst, time.Now()) {
		return nil
	}

	inst.HealthStatus = healthStatus

	return nil
}

// SetInstanceProtection sets the protected-from-scale-in flag on instances.
func (b *InMemoryBackend) SetInstanceProtection(
	groupName string,
	instanceIDs []string,
	protectedFromScaleIn bool,
) error {
	b.mu.Lock("SetInstanceProtection")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	idSet := make(map[string]bool, len(instanceIDs))
	for _, id := range instanceIDs {
		idSet[id] = true
	}

	for i := range g.Instances {
		if idSet[g.Instances[i].InstanceID] {
			g.Instances[i].ProtectedFromScaleIn = protectedFromScaleIn
		}
	}

	return nil
}

// LaunchInstances adds new instances to the ASG.
func (b *InMemoryBackend) LaunchInstances(groupName string, count int32) ([]Instance, error) {
	b.mu.Lock("LaunchInstances")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	oldLen := len(g.Instances)
	newInstances := b.makeInstances(g, count)
	g.Instances = append(g.Instances, newInstances...)
	g.DesiredCapacity = int32(len(g.Instances)) //nolint:gosec // bounded by maxDesiredCapacity

	for _, inst := range g.Instances[oldLen:] {
		b.instanceIndex[inst.InstanceID] = groupName
	}

	b.gateNewLaunchInstances(g, oldLen)

	// Return a copy reflecting any lifecycle-hook gating just applied (e.g.
	// Pending:Wait), not the pre-gating snapshot.
	result := make([]Instance, len(g.Instances)-oldLen)
	copy(result, g.Instances[oldLen:])

	return result, nil
}
