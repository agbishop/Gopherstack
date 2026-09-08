package ecs

import (
	"fmt"
	"slices"
	"time"
)

// builtinCapacityProviders returns a synthesized CapacityProvider for FARGATE or
// FARGATE_SPOT, which are managed by AWS and do not require explicit creation.
func builtinCapacityProvider(name string) *CapacityProvider {
	switch name {
	case launchTypeFargate, "FARGATE_SPOT":
		return &CapacityProvider{
			Name:                name,
			Status:              statusActive,
			CapacityProviderArn: "arn:aws:ecs:::capacity-provider/" + name,
		}
	default:
		return nil
	}
}

// findCapacityProviderLocked returns the map key and pointer for a capacity provider by name or ARN.
// Must be called with at least an RLock held.
func (b *InMemoryBackend) findCapacityProviderLocked(nameOrArn string) (string, *CapacityProvider) {
	if cp, ok := b.capacityProviders.Get(nameOrArn); ok {
		return nameOrArn, cp
	}

	for _, cp := range b.capacityProviders.All() {
		if cp.CapacityProviderArn == nameOrArn {
			return cp.Name, cp
		}
	}

	return "", nil
}

// CreateCapacityProvider creates a new capacity provider.
func (b *InMemoryBackend) CreateCapacityProvider(
	input CreateCapacityProviderInput,
) (*CapacityProvider, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateCapacityProvider")
	defer b.mu.Unlock()

	if b.capacityProviders.Has(input.Name) {
		return nil, fmt.Errorf("%w: capacity provider %s already exists", ErrInvalidParameter, input.Name)
	}

	cp := &CapacityProvider{
		CreatedAt: time.Now(),
		CapacityProviderArn: fmt.Sprintf(
			"arn:aws:ecs:%s:%s:capacity-provider/%s", b.region, b.accountID, input.Name,
		),
		Name:                     input.Name,
		Status:                   statusActive,
		AutoScalingGroupProvider: input.AutoScalingGroupProvider,
		Tags:                     copyTags(input.Tags),
	}

	b.capacityProviders.Put(cp)

	if len(input.Tags) > 0 {
		b.setResourceTagsLocked(cp.CapacityProviderArn, input.Tags)
	}

	out := b.capacityProviderWithLiveTagsLocked(cp)

	return &out, nil
}

// DeleteCapacityProvider deletes a capacity provider by name or ARN.
func (b *InMemoryBackend) DeleteCapacityProvider(nameOrArn string) (*CapacityProvider, error) {
	b.mu.Lock("DeleteCapacityProvider")
	defer b.mu.Unlock()

	key, cp := b.findCapacityProviderLocked(nameOrArn)
	if cp == nil {
		return nil, fmt.Errorf("%w: capacity provider %s not found", ErrInvalidParameter, nameOrArn)
	}

	for _, cluster := range b.clusters.All() {
		if slices.Contains(cluster.CapacityProviders, cp.Name) {
			return nil, fmt.Errorf(
				"%w: capacity provider %s is associated with cluster %s",
				ErrCapacityProviderInUse, cp.Name, cluster.ClusterName,
			)
		}
	}

	b.capacityProviders.Delete(key)

	out := *cp

	return &out, nil
}

// filterRefsByClusterAssociationLocked narrows refs down to the ones present in
// the given cluster's associated capacity-provider list, or -- when refs is
// empty -- returns the cluster's full associated list. Must be called with at
// least a read lock held.
func filterRefsByClusterAssociation(refs, clusterCapacityProviders []string) []string {
	if len(refs) == 0 {
		return clusterCapacityProviders
	}

	assoc := make(map[string]struct{}, len(clusterCapacityProviders))
	for _, name := range clusterCapacityProviders {
		assoc[name] = struct{}{}
	}

	filtered := make([]string, 0, len(refs))

	for _, ref := range refs {
		if _, associated := assoc[ref]; associated {
			filtered = append(filtered, ref)
		}
	}

	return filtered
}

// resolveCapacityProviderRefsLocked resolves the effective set of capacity
// provider name/ARN references to describe, applying the optional Cluster
// filter. The second return value reports whether the cluster filter caused an
// early "cluster not found -> empty result" outcome, matching AWS's
// filter-parameter semantics (not a hard 404). Must be called with at least a
// read lock held.
func (b *InMemoryBackend) resolveCapacityProviderRefsLocked(
	nameOrArns []string,
	cluster string,
) ([]string, bool) {
	if cluster == "" {
		return nameOrArns, false
	}

	c, ok := b.clusters.Get(clusterKey(cluster))
	if !ok {
		return nil, true
	}

	return filterRefsByClusterAssociation(nameOrArns, c.CapacityProviders), false
}

// capacityProviderWithLiveTagsLocked returns a copy of cp with Tags sourced
// from the resourceTags side map instead of cp's own creation-time snapshot,
// so tags applied via TagResource/UntagResource after creation are reflected
// -- same fix as taskWithLiveTagsLocked and the existing
// ExpressGatewayService pattern. Must be called with at least a read lock held.
func (b *InMemoryBackend) capacityProviderWithLiveTagsLocked(cp *CapacityProvider) CapacityProvider {
	c := *cp
	c.Tags = copyTags(b.resourceTags[resourceTagKey(cp.CapacityProviderArn)])

	return c
}

// allCapacityProvidersLocked returns every known capacity provider (used when
// DescribeCapacityProviders is called with no name/ARN filter and no cluster
// filter). Must be called with at least a read lock held.
func (b *InMemoryBackend) allCapacityProvidersLocked() []CapacityProvider {
	all := b.capacityProviders.All()
	out := make([]CapacityProvider, 0, len(all))

	for _, cp := range all {
		out = append(out, b.capacityProviderWithLiveTagsLocked(cp))
	}

	return out
}

// resolveCapacityProviderRefLocked resolves a single name/ARN reference to a
// CapacityProvider (checking created providers, then the FARGATE/FARGATE_SPOT
// builtins), returning ok=false if it resolves to neither. Must be called
// with at least a read lock held.
func (b *InMemoryBackend) resolveCapacityProviderRefLocked(ref string) (CapacityProvider, bool) {
	if _, cp := b.findCapacityProviderLocked(ref); cp != nil {
		return b.capacityProviderWithLiveTagsLocked(cp), true
	}

	if builtin := builtinCapacityProvider(ref); builtin != nil {
		return *builtin, true
	}

	return CapacityProvider{}, false
}

// DescribeCapacityProviders returns capacity providers, optionally filtered by name/ARN
// and/or by cluster association. Names/ARNs that don't resolve to a known or built-in
// capacity provider are reported in the returned Failures slice (reason MISSING),
// matching AWS's DescribeCapacityProviders behavior of partial success rather than
// failing the whole call.
//
// When cluster is non-empty, only capacity providers associated with that cluster (via
// CreateCluster/UpdateCluster/PutClusterCapacityProviders) are considered, matching AWS's
// documented Cluster filter parameter. An unknown cluster name yields an empty result
// (not an error), consistent with AWS's filter semantics for this parameter.
func (b *InMemoryBackend) DescribeCapacityProviders(
	nameOrArns []string,
	cluster string,
) ([]CapacityProvider, []Failure, error) {
	b.mu.RLock("DescribeCapacityProviders")
	defer b.mu.RUnlock()

	refs, clusterNotFound := b.resolveCapacityProviderRefsLocked(nameOrArns, cluster)
	if clusterNotFound {
		return nil, nil, nil
	}

	if len(refs) == 0 && cluster == "" {
		return b.allCapacityProvidersLocked(), nil, nil
	}

	out := make([]CapacityProvider, 0, len(refs))
	failures := make([]Failure, 0, len(refs))

	for _, ref := range refs {
		cp, ok := b.resolveCapacityProviderRefLocked(ref)
		if !ok {
			failures = append(failures, Failure{
				Arn:    ref,
				Reason: statusMissing,
				Detail: fmt.Sprintf("capacity provider %s not found", ref),
			})

			continue
		}

		out = append(out, cp)
	}

	return out, failures, nil
}

// validateCapacityProviderStrategyLocked returns ErrClient if any strategy item
// references a capacity provider name that does not exist -- neither explicitly
// created via CreateCapacityProvider nor a FARGATE/FARGATE_SPOT builtin. Matches
// real AWS validation: RunTask, CreateTaskSet, CreateService, UpdateService,
// CreateCluster, UpdateCluster, and PutClusterCapacityProviders all reject a
// capacityProviderStrategy that references an unknown capacity provider with a
// 400 ClientException rather than silently accepting any string. Must be called
// with at least a read lock held (all call sites already hold the write lock).
func (b *InMemoryBackend) validateCapacityProviderStrategyLocked(
	strategy []CapacityProviderStrategyItem,
) error {
	for _, item := range strategy {
		if item.CapacityProvider == "" {
			continue
		}

		if _, cp := b.findCapacityProviderLocked(item.CapacityProvider); cp != nil {
			continue
		}

		if builtinCapacityProvider(item.CapacityProvider) != nil {
			continue
		}

		return fmt.Errorf(
			"%w: capacity provider %s does not exist", ErrClient, item.CapacityProvider,
		)
	}

	return nil
}

// UpdateCapacityProvider updates the managed-scaling settings of an
// ASG-backed capacity provider. Unlike CreateCapacityProvider, the ASG the
// provider wraps cannot be changed here (there is no ARN field on the real
// UpdateCapacityProviderRequest's AutoScalingGroupProviderUpdate) -- only
// ManagedScaling/ManagedTerminationProtection/ManagedDraining are merged
// onto the existing AutoScalingGroupProvider, so its ARN is preserved.
func (b *InMemoryBackend) UpdateCapacityProvider(
	input UpdateCapacityProviderInput,
) (*CapacityProvider, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidParameter)
	}

	b.mu.Lock("UpdateCapacityProvider")
	defer b.mu.Unlock()

	_, cp := b.findCapacityProviderLocked(input.Name)
	if cp == nil {
		return nil, fmt.Errorf("%w: capacity provider %s not found", ErrInvalidParameter, input.Name)
	}

	if input.AutoScalingGroupProvider != nil {
		if cp.AutoScalingGroupProvider == nil {
			cp.AutoScalingGroupProvider = &AutoScalingGroupProvider{}
		}

		cp.AutoScalingGroupProvider.ManagedScaling = input.AutoScalingGroupProvider.ManagedScaling
		cp.AutoScalingGroupProvider.ManagedTerminationProtection = input.AutoScalingGroupProvider.ManagedTerminationProtection
		cp.AutoScalingGroupProvider.ManagedDraining = input.AutoScalingGroupProvider.ManagedDraining
	}

	out := b.capacityProviderWithLiveTagsLocked(cp)

	return &out, nil
}

// AddCapacityProviderInternal adds a capacity provider directly (seed helper for tests).
func (b *InMemoryBackend) AddCapacityProviderInternal(cp *CapacityProvider) {
	b.mu.Lock("AddCapacityProviderInternal")
	defer b.mu.Unlock()

	c := *cp
	c.Tags = copyTags(cp.Tags)
	b.capacityProviders.Put(&c)
}
