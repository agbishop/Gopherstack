package autoscaling

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// ELBInstanceRegistrar lets the Auto Scaling backend register/deregister real
// classic ELB instances as group membership and LoadBalancerNames change, so
// ELB DescribeInstanceHealth reflects ASG-managed instances instead of
// LoadBalancerNames being stored and echoed with no effect. When no
// registrar is configured (the default), behavior is unchanged from before
// this feature existed.
type ELBInstanceRegistrar interface {
	// RegisterInstances registers instanceIDs with the named load balancer.
	RegisterInstances(ctx context.Context, loadBalancerName string, instanceIDs []string) error
	// DeregisterInstances deregisters instanceIDs from the named load balancer.
	DeregisterInstances(ctx context.Context, loadBalancerName string, instanceIDs []string) error
}

// SetELBRegistrar wires an ELBInstanceRegistrar so subsequent instance
// launches/terminations and LoadBalancerNames attach/detach calls register or
// deregister real classic ELB instances. Passing nil restores the
// historical, registration-less behavior. Intended to be called once during
// service wiring, before the backend serves traffic.
func (b *InMemoryBackend) SetELBRegistrar(r ELBInstanceRegistrar) {
	b.mu.Lock("SetELBRegistrar")
	defer b.mu.Unlock()
	b.elbRegistrar = r
}

// registerELBInstances registers each of instanceIDs with every load
// balancer name in lbNames. No-op when no registrar is wired or either slice
// is empty. Best-effort: errors are logged, not propagated, mirroring
// registerELBTargets's failure handling. Must be called with b.mu held
// (write lock).
func (b *InMemoryBackend) registerELBInstances(instanceIDs, lbNames []string) {
	if b.elbRegistrar == nil || len(instanceIDs) == 0 || len(lbNames) == 0 {
		return
	}

	for _, lb := range lbNames {
		if err := b.elbRegistrar.RegisterInstances(context.Background(), lb, instanceIDs); err != nil {
			logger.Load(context.Background()).Error(
				"autoscaling: ELB RegisterInstances failed",
				"error", err, "loadBalancerName", lb, "instanceIDs", instanceIDs)
		}
	}
}

// deregisterELBInstances deregisters each of instanceIDs from every load
// balancer name in lbNames. No-op when no registrar is wired or either slice
// is empty. Best-effort: errors are logged, not propagated. Must be called
// with b.mu held (write lock).
func (b *InMemoryBackend) deregisterELBInstances(instanceIDs, lbNames []string) {
	if b.elbRegistrar == nil || len(instanceIDs) == 0 || len(lbNames) == 0 {
		return
	}

	for _, lb := range lbNames {
		if err := b.elbRegistrar.DeregisterInstances(context.Background(), lb, instanceIDs); err != nil {
			logger.Load(context.Background()).Error(
				"autoscaling: ELB DeregisterInstances failed",
				"error", err, "loadBalancerName", lb, "instanceIDs", instanceIDs)
		}
	}
}
