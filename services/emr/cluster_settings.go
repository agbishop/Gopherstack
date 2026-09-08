package emr

import (
	"context"
	"fmt"
)

// ModifyCluster updates StepConcurrencyLevel on a cluster.
func (b *InMemoryBackend) ModifyCluster(
	ctx context.Context, clusterID string, stepConcurrencyLevel int,
) (int, error) {
	if stepConcurrencyLevel < minStepConcurrency || stepConcurrencyLevel > maxStepConcurrency {
		return 0, fmt.Errorf(
			"%w: StepConcurrencyLevel must be between %d and %d",
			ErrValidation,
			minStepConcurrency,
			maxStepConcurrency,
		)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("ModifyCluster")
	defer b.mu.Unlock()

	cluster, ok := b.clusterGet(region, clusterID)
	if !ok {
		return 0, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterID)
	}

	cluster.StepConcurrencyLevel = stepConcurrencyLevel

	return stepConcurrencyLevel, nil
}

// SetTerminationProtection sets the TerminationProtected flag on clusters.
func (b *InMemoryBackend) SetTerminationProtection(
	ctx context.Context, jobFlowIDs []string, protect bool,
) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("SetTerminationProtection")
	defer b.mu.Unlock()

	for _, id := range jobFlowIDs {
		cluster, ok := b.clusterGet(region, id)
		if !ok {
			return fmt.Errorf("%w: cluster %s not found", ErrNotFound, id)
		}

		cluster.TerminationProtected = protect
	}

	return nil
}

// SetKeepJobFlowAliveWhenNoSteps sets the KeepJobFlowAliveWhenNoSteps flag.
func (b *InMemoryBackend) SetKeepJobFlowAliveWhenNoSteps(
	ctx context.Context, jobFlowIDs []string, keep bool,
) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("SetKeepJobFlowAliveWhenNoSteps")
	defer b.mu.Unlock()

	for _, id := range jobFlowIDs {
		cluster, ok := b.clusterGet(region, id)
		if !ok {
			return fmt.Errorf("%w: cluster %s not found", ErrNotFound, id)
		}

		cluster.KeepJobFlowAliveWhenNoSteps = keep
		// AutoTerminate is the real API's inverse field (types.Cluster,
		// emr@v1.64.4 types/types.go:315) -- keep it in sync or the
		// janitor's auto-termination sweep (janitor.go) would act on a
		// stale value after this call.
		cluster.AutoTerminate = !keep
	}

	return nil
}

// SetVisibleToAllUsers sets the VisibleToAllUsers flag.
func (b *InMemoryBackend) SetVisibleToAllUsers(
	ctx context.Context, jobFlowIDs []string, visible bool,
) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("SetVisibleToAllUsers")
	defer b.mu.Unlock()

	for _, id := range jobFlowIDs {
		cluster, ok := b.clusterGet(region, id)
		if !ok {
			return fmt.Errorf("%w: cluster %s not found", ErrNotFound, id)
		}

		cluster.VisibleToAllUsers = visible
	}

	return nil
}

// SetUnhealthyNodeReplacement sets the UnhealthyNodeReplacement flag.
func (b *InMemoryBackend) SetUnhealthyNodeReplacement(
	ctx context.Context, jobFlowIDs []string, replace bool,
) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("SetUnhealthyNodeReplacement")
	defer b.mu.Unlock()

	for _, id := range jobFlowIDs {
		cluster, ok := b.clusterGet(region, id)
		if !ok {
			return fmt.Errorf("%w: cluster %s not found", ErrNotFound, id)
		}

		cluster.UnhealthyNodeReplacement = replace
	}

	return nil
}
