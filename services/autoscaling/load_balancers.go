package autoscaling

import "fmt"

// AttachLoadBalancerTargetGroups adds target group ARNs to the specified Auto Scaling group.
func (b *InMemoryBackend) AttachLoadBalancerTargetGroups(groupName string, targetGroupARNs []string) error {
	b.mu.Lock("AttachLoadBalancerTargetGroups")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	existing := make(map[string]bool, len(g.TargetGroupARNs))
	for _, arn := range g.TargetGroupARNs {
		existing[arn] = true
	}

	added := make([]string, 0, len(targetGroupARNs))

	for _, arn := range targetGroupARNs {
		if !existing[arn] {
			g.TargetGroupARNs = append(g.TargetGroupARNs, arn)
			added = append(added, arn)
		}
	}

	// Newly attached target groups pick up every instance currently in the
	// group, mirroring real AWS registering existing members immediately on attach.
	b.registerELBTargets(instanceIDsOf(g.Instances), added)

	return nil
}

// AttachLoadBalancers adds load balancer names to the specified Auto Scaling group.
func (b *InMemoryBackend) AttachLoadBalancers(groupName string, loadBalancerNames []string) error {
	b.mu.Lock("AttachLoadBalancers")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	existing := make(map[string]bool, len(g.LoadBalancerNames))
	for _, lb := range g.LoadBalancerNames {
		existing[lb] = true
	}

	added := make([]string, 0, len(loadBalancerNames))

	for _, lb := range loadBalancerNames {
		if !existing[lb] {
			g.LoadBalancerNames = append(g.LoadBalancerNames, lb)
			added = append(added, lb)
		}
	}

	// Newly attached load balancers pick up every instance currently in the
	// group, mirroring real AWS registering existing members immediately on attach.
	b.registerELBInstances(instanceIDsOf(g.Instances), added)

	return nil
}

// DescribeLoadBalancers returns the load balancers attached to the group.
func (b *InMemoryBackend) DescribeLoadBalancers(groupName string) ([]LoadBalancerState, error) {
	b.mu.RLock("DescribeLoadBalancers")
	defer b.mu.RUnlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	result := make([]LoadBalancerState, 0, len(g.LoadBalancerNames))
	for _, lb := range g.LoadBalancerNames {
		result = append(result, LoadBalancerState{LoadBalancerName: lb, State: lbStateAdded})
	}

	return result, nil
}

// DescribeLoadBalancerTargetGroups returns the target groups attached to the group.
func (b *InMemoryBackend) DescribeLoadBalancerTargetGroups(groupName string) ([]LoadBalancerTargetGroupState, error) {
	b.mu.RLock("DescribeLoadBalancerTargetGroups")
	defer b.mu.RUnlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	result := make([]LoadBalancerTargetGroupState, 0, len(g.TargetGroupARNs))
	for _, arn := range g.TargetGroupARNs {
		result = append(result, LoadBalancerTargetGroupState{LoadBalancerTargetGroupARN: arn, State: lbStateAdded})
	}

	return result, nil
}

// DetachLoadBalancerTargetGroups removes target group ARNs from the ASG.
func (b *InMemoryBackend) DetachLoadBalancerTargetGroups(groupName string, targetGroupARNs []string) error {
	b.mu.Lock("DetachLoadBalancerTargetGroups")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	removeSet := make(map[string]bool, len(targetGroupARNs))
	for _, arn := range targetGroupARNs {
		removeSet[arn] = true
	}

	newARNs := make([]string, 0, len(g.TargetGroupARNs))
	removedARNs := make([]string, 0, len(targetGroupARNs))

	for _, arn := range g.TargetGroupARNs {
		if removeSet[arn] {
			removedARNs = append(removedARNs, arn)

			continue
		}

		newARNs = append(newARNs, arn)
	}

	g.TargetGroupARNs = newARNs

	b.deregisterELBTargets(instanceIDsOf(g.Instances), removedARNs)

	return nil
}

// DetachLoadBalancers removes load balancer names from the ASG.
func (b *InMemoryBackend) DetachLoadBalancers(groupName string, lbNames []string) error {
	b.mu.Lock("DetachLoadBalancers")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(groupName)
	if !ok {
		return fmt.Errorf("%w: %q", ErrGroupNotFound, groupName)
	}

	removeSet := make(map[string]bool, len(lbNames))
	for _, lb := range lbNames {
		removeSet[lb] = true
	}

	newLBs := make([]string, 0, len(g.LoadBalancerNames))
	removedLBs := make([]string, 0, len(lbNames))

	for _, lb := range g.LoadBalancerNames {
		if removeSet[lb] {
			removedLBs = append(removedLBs, lb)

			continue
		}

		newLBs = append(newLBs, lb)
	}

	g.LoadBalancerNames = newLBs

	b.deregisterELBInstances(instanceIDsOf(g.Instances), removedLBs)

	return nil
}
