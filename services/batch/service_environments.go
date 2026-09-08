package batch

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateServiceEnvironment creates a new service environment.
func (b *InMemoryBackend) CreateServiceEnvironment(
	ctx context.Context,
	name, envType, state string,
	capacityLimits []CapacityLimit,
	tags map[string]string,
) (*ServiceEnvironment, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateServiceEnvironment")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: serviceEnvironmentName is required", ErrValidation)
	}

	if envType == "" {
		return nil, fmt.Errorf("%w: serviceEnvironmentType is required", ErrValidation)
	}

	if len(capacityLimits) == 0 {
		return nil, fmt.Errorf("%w: capacityLimits is required", ErrValidation)
	}

	if b.serviceEnvironments.Has(regionKey(region, name)) {
		return nil, fmt.Errorf("%w: service environment %s already exists", ErrAlreadyExists, name)
	}

	if err := validateTags(tags); err != nil {
		return nil, err
	}

	seARN := arn.Build("batch", region, b.accountID, "service-environment/"+name)

	if state == "" {
		state = stateEnabled
	}

	se := &ServiceEnvironment{
		region:                 region,
		ServiceEnvironmentName: name,
		ServiceEnvironmentArn:  seARN,
		ServiceEnvironmentType: envType,
		State:                  state,
		Status:                 statusValid,
		CapacityLimits:         cloneCapacityLimits(capacityLimits),
		Tags:                   tagsCloneOrEmpty(tags),
	}
	b.serviceEnvironments.Put(se)
	cp := *se

	return &cp, nil
}

// cloneCapacityLimits deep-copies a CapacityLimit slice, returning nil for empty input.
func cloneCapacityLimits(limits []CapacityLimit) []CapacityLimit {
	if len(limits) == 0 {
		return nil
	}

	out := make([]CapacityLimit, len(limits))
	copy(out, limits)

	return out
}

// DeleteServiceEnvironment removes a service environment by name or ARN.
// Requires DISABLED state and no referencing job queue first, via
// UpdateServiceEnvironment/UpdateJobQueue (api_op_DeleteServiceEnvironment.go).
func (b *InMemoryBackend) DeleteServiceEnvironment(ctx context.Context, nameOrARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteServiceEnvironment")
	defer b.mu.Unlock()

	se, ok := b.lookupServiceEnvironmentByNameOrARN(region, nameOrARN)
	if !ok {
		return fmt.Errorf("%w: service environment %s not found", ErrNotFound, nameOrARN)
	}

	if se.State != stateDisabled {
		return fmt.Errorf(
			"%w: service environment %s must be DISABLED before it can be deleted",
			ErrValidation, nameOrARN,
		)
	}

	for _, jq := range b.jobQueuesByRegion.Get(region) {
		for _, seOrder := range jq.ServiceEnvironmentOrder {
			refersToSE := seOrder.ServiceEnvironment == se.ServiceEnvironmentName ||
				seOrder.ServiceEnvironment == se.ServiceEnvironmentArn
			if refersToSE {
				return fmt.Errorf(
					"%w: service environment %s is referenced by job queue %s",
					ErrValidation, nameOrARN, jq.JobQueueName,
				)
			}
		}
	}

	b.serviceEnvironments.Delete(regionKey(region, se.ServiceEnvironmentName))

	return nil
}

// lookupServiceEnvironmentByNameOrARN returns a service environment by name or ARN within region.
// Caller must hold at least a read lock.
func (b *InMemoryBackend) lookupServiceEnvironmentByNameOrARN(region, nameOrARN string) (*ServiceEnvironment, bool) {
	if se, ok := b.serviceEnvironments.Get(regionKey(region, nameOrARN)); ok {
		return se, true
	}

	for _, se := range b.serviceEnvironmentsByRegion.Get(region) {
		if se.ServiceEnvironmentArn == nameOrARN {
			return se, true
		}
	}

	return nil, false
}

// cloneServiceEnvironmentWithTags returns a tag-cloned copy of se.
func cloneServiceEnvironmentWithTags(se *ServiceEnvironment) *ServiceEnvironment {
	cp := *se
	cp.Tags = tagsCloneOrEmpty(se.Tags)

	return &cp
}

// DescribeServiceEnvironments returns service environments, optionally
// filtered by names/ARNs. When names is empty, results are paginated via
// maxResults/nextToken, matching aws-sdk-go-v2/service/batch's
// DescribeServiceEnvironmentsInput.
func (b *InMemoryBackend) DescribeServiceEnvironments(
	ctx context.Context,
	names []string,
	maxResults int32,
	nextToken string,
) ([]*ServiceEnvironment, string) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeServiceEnvironments")
	defer b.mu.RUnlock()

	return describeResourcesPaginated(
		names, maxResults, nextToken,
		func(nameOrARN string) (*ServiceEnvironment, bool) {
			return b.lookupServiceEnvironmentByNameOrARN(region, nameOrARN)
		},
		func() []string {
			return sortedNames(b.serviceEnvironmentsByRegion.Get(region), func(se *ServiceEnvironment) string {
				return se.ServiceEnvironmentName
			})
		},
		func(key string) (*ServiceEnvironment, bool) { return b.serviceEnvironments.Get(regionKey(region, key)) },
		cloneServiceEnvironmentWithTags,
	)
}

// UpdateServiceEnvironment updates the state and/or capacity limits of a service environment.
func (b *InMemoryBackend) UpdateServiceEnvironment(
	ctx context.Context,
	nameOrARN, state string,
	capacityLimits []CapacityLimit,
) (*ServiceEnvironment, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateServiceEnvironment")
	defer b.mu.Unlock()

	se, ok := b.lookupServiceEnvironmentByNameOrARN(region, nameOrARN)
	if !ok {
		return nil, fmt.Errorf("%w: service environment %s not found", ErrNotFound, nameOrARN)
	}

	if state != "" {
		se.State = state
	}

	if capacityLimits != nil {
		se.CapacityLimits = cloneCapacityLimits(capacityLimits)
	}

	cp := *se
	cp.Tags = tagsCloneOrEmpty(se.Tags)

	return &cp, nil
}
