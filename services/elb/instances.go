package elb

import (
	"context"
	"fmt"
	"regexp"
	"sort"
)

const (
	notApplicable = "N/A"
)

var (
	// instanceIDRe matches valid EC2 instance IDs: i- followed by 8-17 lowercase hex chars.
	instanceIDRe = regexp.MustCompile(`^i-[a-f0-9]{8,17}$`)
)

// RegisterInstancesWithLoadBalancer registers EC2 instances with a load balancer.
func (b *InMemoryBackend) RegisterInstancesWithLoadBalancer(
	ctx context.Context, name string, instances []Instance,
) ([]Instance, error) {
	b.mu.Lock("RegisterInstancesWithLoadBalancer")
	defer b.mu.Unlock()

	lb, ok := b.lbs.Get(lbTableKey(getRegion(ctx, b.region), name))
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
	}

	for _, inst := range instances {
		if !instanceIDRe.MatchString(inst.InstanceID) {
			return nil, fmt.Errorf(
				"%w: invalid instance ID format %q; must match i-[a-f0-9]{8,17}",
				ErrInvalidParameter,
				inst.InstanceID,
			)
		}
	}

	if b.ec2Resolver != nil {
		for _, inst := range instances {
			if !b.ec2Resolver.InstanceExists(inst.InstanceID) {
				return nil, fmt.Errorf("%w: %s", ErrInvalidInstance, inst.InstanceID)
			}
		}
	}

	existing := make(map[string]bool, len(lb.Instances))
	for _, inst := range lb.Instances {
		existing[inst.InstanceID] = true
	}

	// Real AWS's RegisterInstancesWithLoadBalancer typed-error switch only
	// recognizes InvalidInstance and LoadBalancerNotFound (see
	// deserializers.go's awsAwsquery_deserializeOpErrorRegisterInstancesWithLoadBalancer);
	// there is no typed exception for exceeding the "classic-registered-instances"
	// account limit shown by DescribeAccountLimits, so it is not enforced here.
	for _, inst := range instances {
		if !existing[inst.InstanceID] {
			lb.Instances = append(lb.Instances, inst)
			existing[inst.InstanceID] = true
		}
	}

	result := make([]Instance, len(lb.Instances))
	copy(result, lb.Instances)

	return result, nil
}

// DeregisterInstancesFromLoadBalancer removes EC2 instances from a load balancer.
func (b *InMemoryBackend) DeregisterInstancesFromLoadBalancer(
	ctx context.Context, name string, instances []Instance,
) ([]Instance, error) {
	b.mu.Lock("DeregisterInstancesFromLoadBalancer")
	defer b.mu.Unlock()

	lb, ok := b.lbs.Get(lbTableKey(getRegion(ctx, b.region), name))
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
	}

	remove := make(map[string]bool, len(instances))
	for _, inst := range instances {
		remove[inst.InstanceID] = true
	}

	kept := lb.Instances[:0]
	for _, inst := range lb.Instances {
		if !remove[inst.InstanceID] {
			kept = append(kept, inst)
		}
	}

	lb.Instances = kept

	result := make([]Instance, len(lb.Instances))
	copy(result, lb.Instances)

	return result, nil
}

// DescribeInstanceHealth returns the health state of registered instances.
func (b *InMemoryBackend) DescribeInstanceHealth(
	ctx context.Context, name string, instances []Instance,
) ([]InstanceState, error) {
	b.mu.RLock("DescribeInstanceHealth")
	defer b.mu.RUnlock()

	lb, ok := b.lbs.Get(lbTableKey(getRegion(ctx, b.region), name))
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
	}

	// If specific instances requested, validate them and return their health.
	if len(instances) > 0 {
		for _, inst := range instances {
			if !instanceIDRe.MatchString(inst.InstanceID) {
				return nil, fmt.Errorf(
					"%w: invalid instance ID format %q; must match i-[a-f0-9]{8,17}",
					ErrInvalidInstance,
					inst.InstanceID,
				)
			}
		}

		registered := make(map[string]bool, len(lb.Instances))
		for _, inst := range lb.Instances {
			registered[inst.InstanceID] = true
		}

		result := make([]InstanceState, 0, len(instances))
		for _, inst := range instances {
			if !registered[inst.InstanceID] {
				return nil, fmt.Errorf(
					"%w: instance %q is not registered with load balancer %q",
					ErrInvalidInstance,
					inst.InstanceID,
					name,
				)
			}

			result = append(result, InstanceState{
				InstanceID:  inst.InstanceID,
				State:       "InService",
				ReasonCode:  notApplicable,
				Description: notApplicable,
			})
		}

		return result, nil
	}

	// Return all registered instances as InService.
	result := make([]InstanceState, 0, len(lb.Instances))
	for _, inst := range lb.Instances {
		result = append(result, InstanceState{
			InstanceID:  inst.InstanceID,
			State:       "InService",
			ReasonCode:  notApplicable,
			Description: notApplicable,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].InstanceID < result[j].InstanceID
	})

	return result, nil
}
