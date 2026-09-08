package elb

import (
	"context"
	"fmt"
)

// ModifyLoadBalancerAttributes updates the tunable attributes for a load
// balancer. Only the attribute groups marked present in mask are changed;
// groups the caller omitted keep their current stored value (AWS's
// LoadBalancerAttributes sub-structs are each optional and independently
// settable — see types.LoadBalancerAttributes in the AWS SDK).
func (b *InMemoryBackend) ModifyLoadBalancerAttributes(
	ctx context.Context,
	name string,
	attrs LoadBalancerAttributes,
	mask LoadBalancerAttributesMask,
) (*LoadBalancerAttributes, error) {
	b.mu.Lock("ModifyLoadBalancerAttributes")
	defer b.mu.Unlock()

	lb, ok := b.lbs.Get(lbTableKey(getRegion(ctx, b.region), name))
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
	}

	if mask.CrossZoneLoadBalancing {
		lb.Attributes.CrossZoneLoadBalancing = attrs.CrossZoneLoadBalancing
	}

	if mask.ConnectionDraining {
		lb.Attributes.ConnectionDraining = attrs.ConnectionDraining
		lb.Attributes.ConnectionDrainingTimeout = attrs.ConnectionDrainingTimeout
	}

	if mask.ConnectionSettings {
		lb.Attributes.IdleTimeout = attrs.IdleTimeout
	}

	if mask.AccessLog {
		lb.Attributes.AccessLog = attrs.AccessLog
	}

	if mask.DesyncMitigationMode {
		lb.Attributes.DesyncMitigationMode = attrs.DesyncMitigationMode
	}

	cp := lb.Attributes

	return &cp, nil
}

// DescribeLoadBalancerAttributes returns the tunable attributes for a load balancer.
func (b *InMemoryBackend) DescribeLoadBalancerAttributes(
	ctx context.Context, name string,
) (*LoadBalancerAttributes, error) {
	b.mu.RLock("DescribeLoadBalancerAttributes")
	defer b.mu.RUnlock()

	lb, ok := b.lbs.Get(lbTableKey(getRegion(ctx, b.region), name))
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
	}

	cp := lb.Attributes

	return &cp, nil
}
