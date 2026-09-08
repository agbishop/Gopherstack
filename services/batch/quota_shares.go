package batch

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// quotaShareNameRegex validates AWS Batch quota share names: up to 128
// characters, uppercase/lowercase letters, numbers, hyphens, and underscores
// (see CreateQuotaShareInput.QuotaShareName's documented constraint, same
// character class as job definition names -- see jobDefNameRegex).
var quotaShareNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// cloneQuotaShareCapacityLimits deep-copies a QuotaShareCapacityLimit slice,
// returning nil for empty input.
func cloneQuotaShareCapacityLimits(limits []QuotaShareCapacityLimit) []QuotaShareCapacityLimit {
	if len(limits) == 0 {
		return nil
	}

	out := make([]QuotaShareCapacityLimit, len(limits))
	copy(out, limits)

	return out
}

// cloneQuotaSharePreemptionConfiguration deep-copies a
// QuotaSharePreemptionConfiguration, returning nil for nil input.
func cloneQuotaSharePreemptionConfiguration(
	c *QuotaSharePreemptionConfiguration,
) *QuotaSharePreemptionConfiguration {
	if c == nil {
		return nil
	}

	cp := *c

	return &cp
}

// cloneQuotaShareResourceSharingConfiguration deep-copies a
// QuotaShareResourceSharingConfiguration, returning nil for nil input.
func cloneQuotaShareResourceSharingConfiguration(
	c *QuotaShareResourceSharingConfiguration,
) *QuotaShareResourceSharingConfiguration {
	if c == nil {
		return nil
	}

	cp := *c

	return &cp
}

// validateQuotaSharePreemptionConfiguration checks that c is present and its
// inSharePreemption is one of the two enum values real AWS Batch defines
// (types.QuotaShareInSharePreemptionState: ENABLED, DISABLED).
func validateQuotaSharePreemptionConfiguration(c *QuotaSharePreemptionConfiguration) error {
	if c == nil || c.InSharePreemption == "" {
		return fmt.Errorf("%w: preemptionConfiguration.inSharePreemption is required", ErrValidation)
	}

	if c.InSharePreemption != stateEnabled && c.InSharePreemption != stateDisabled {
		return fmt.Errorf(
			"%w: invalid preemptionConfiguration.inSharePreemption %q", ErrValidation, c.InSharePreemption,
		)
	}

	return nil
}

// validateQuotaShareResourceSharingConfiguration checks that c is present
// and its strategy is one of the three enum values real AWS Batch defines
// (types.QuotaShareResourceSharingStrategy: RESERVE, LEND, LEND_AND_BORROW).
func validateQuotaShareResourceSharingConfiguration(c *QuotaShareResourceSharingConfiguration) error {
	if c == nil || c.Strategy == "" {
		return fmt.Errorf("%w: resourceSharingConfiguration.strategy is required", ErrValidation)
	}

	switch c.Strategy {
	case "RESERVE", "LEND", "LEND_AND_BORROW":
	default:
		return fmt.Errorf("%w: invalid resourceSharingConfiguration.strategy %q", ErrValidation, c.Strategy)
	}

	return nil
}

// CreateQuotaShare creates a new quota share associated with an existing job
// queue. Real AWS Batch requires the referenced job queue to already exist
// and be in the VALID state before it can be associated with a quota share
// (see CreateQuotaShareInput's jobQueue documentation); this emulator's job
// queues are always created VALID (see statusValid in store.go) and never
// transition away from it, so the state check below, while real, is not
// currently reachable through this backend's own state machine.
func (b *InMemoryBackend) CreateQuotaShare(
	ctx context.Context,
	name, jobQueue string,
	capacityLimits []QuotaShareCapacityLimit,
	preemption *QuotaSharePreemptionConfiguration,
	resourceSharing *QuotaShareResourceSharingConfiguration,
	state string,
	tags map[string]string,
) (*QuotaShare, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateQuotaShare")
	defer b.mu.Unlock()

	if !quotaShareNameRegex.MatchString(name) {
		return nil, fmt.Errorf("%w: quotaShareName must match [a-zA-Z0-9_-]{1,128}", ErrValidation)
	}

	if jobQueue == "" {
		return nil, fmt.Errorf("%w: jobQueue is required", ErrValidation)
	}

	jq, ok := b.lookupJQByNameOrARN(region, jobQueue)
	if !ok {
		return nil, fmt.Errorf("%w: job queue %s not found", ErrNotFound, jobQueue)
	}

	if jq.Status != statusValid {
		return nil, fmt.Errorf("%w: job queue %s must be in VALID state", ErrValidation, jobQueue)
	}

	if len(capacityLimits) == 0 {
		return nil, fmt.Errorf("%w: capacityLimits is required", ErrValidation)
	}

	for _, cl := range capacityLimits {
		if cl.CapacityUnit == "" {
			return nil, fmt.Errorf("%w: capacityLimits.capacityUnit is required", ErrValidation)
		}
	}

	if err := validateQuotaSharePreemptionConfiguration(preemption); err != nil {
		return nil, err
	}

	if err := validateQuotaShareResourceSharingConfiguration(resourceSharing); err != nil {
		return nil, err
	}

	shareState := state
	if shareState == "" {
		shareState = stateEnabled
	}

	if shareState != stateEnabled && shareState != stateDisabled {
		return nil, fmt.Errorf("%w: invalid state %q", ErrValidation, state)
	}

	if err := validateTags(tags); err != nil {
		return nil, err
	}

	quotaShareARN := arn.Build("batch", region, b.accountID, "job-queue/"+jq.JobQueueName+"/quota-share/"+name)

	if b.quotaShares.Has(regionKey(region, quotaShareARN)) {
		return nil, fmt.Errorf("%w: quota share %s already exists on job queue %s", ErrAlreadyExists, name, jobQueue)
	}

	qs := &QuotaShare{
		region:                       region,
		QuotaShareArn:                quotaShareARN,
		QuotaShareName:               name,
		JobQueueArn:                  jq.JobQueueArn,
		CapacityLimits:               cloneQuotaShareCapacityLimits(capacityLimits),
		PreemptionConfiguration:      cloneQuotaSharePreemptionConfiguration(preemption),
		ResourceSharingConfiguration: cloneQuotaShareResourceSharingConfiguration(resourceSharing),
		State:                        shareState,
		Status:                       statusValid,
		Tags:                         tagsCloneOrEmpty(tags),
	}
	b.quotaShares.Put(qs)
	cp := *qs
	cp.Tags = tagsCloneOrEmpty(qs.Tags)
	cp.CapacityLimits = cloneQuotaShareCapacityLimits(qs.CapacityLimits)

	return &cp, nil
}

// DescribeQuotaShare returns a single quota share by ARN.
func (b *InMemoryBackend) DescribeQuotaShare(ctx context.Context, quotaShareARN string) (*QuotaShare, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeQuotaShare")
	defer b.mu.RUnlock()

	qs, ok := b.quotaShares.Get(regionKey(region, quotaShareARN))
	if !ok {
		return nil, fmt.Errorf("%w: quota share %s not found", ErrNotFound, quotaShareARN)
	}

	cp := *qs
	cp.Tags = tagsCloneOrEmpty(qs.Tags)
	cp.CapacityLimits = cloneQuotaShareCapacityLimits(qs.CapacityLimits)

	return &cp, nil
}

// DeleteQuotaShare removes a quota share by ARN. Requires DISABLED state
// first, via UpdateQuotaShare (api_op_DeleteQuotaShare.go).
func (b *InMemoryBackend) DeleteQuotaShare(ctx context.Context, quotaShareARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteQuotaShare")
	defer b.mu.Unlock()

	qs, ok := b.quotaShares.Get(regionKey(region, quotaShareARN))
	if !ok {
		return fmt.Errorf("%w: quota share %s not found", ErrNotFound, quotaShareARN)
	}

	if qs.State != stateDisabled {
		return fmt.Errorf(
			"%w: quota share %s must be DISABLED before it can be deleted",
			ErrValidation, quotaShareARN,
		)
	}

	b.quotaShares.Delete(regionKey(region, quotaShareARN))

	return nil
}

// UpdateQuotaShare updates a quota share's capacity limits, preemption
// configuration, resource sharing configuration, and/or state. Only
// non-nil/non-empty fields are applied, matching UpdateQuotaShareInput where
// everything except quotaShareArn is optional.
func (b *InMemoryBackend) UpdateQuotaShare(
	ctx context.Context,
	quotaShareARN string,
	capacityLimits []QuotaShareCapacityLimit,
	preemption *QuotaSharePreemptionConfiguration,
	resourceSharing *QuotaShareResourceSharingConfiguration,
	state string,
) (*QuotaShare, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateQuotaShare")
	defer b.mu.Unlock()

	qs, ok := b.quotaShares.Get(regionKey(region, quotaShareARN))
	if !ok {
		return nil, fmt.Errorf("%w: quota share %s not found", ErrNotFound, quotaShareARN)
	}

	if len(capacityLimits) > 0 {
		for _, cl := range capacityLimits {
			if cl.CapacityUnit == "" {
				return nil, fmt.Errorf("%w: capacityLimits.capacityUnit is required", ErrValidation)
			}
		}

		qs.CapacityLimits = cloneQuotaShareCapacityLimits(capacityLimits)
	}

	if preemption != nil {
		if err := validateQuotaSharePreemptionConfiguration(preemption); err != nil {
			return nil, err
		}

		qs.PreemptionConfiguration = cloneQuotaSharePreemptionConfiguration(preemption)
	}

	if resourceSharing != nil {
		if err := validateQuotaShareResourceSharingConfiguration(resourceSharing); err != nil {
			return nil, err
		}

		qs.ResourceSharingConfiguration = cloneQuotaShareResourceSharingConfiguration(resourceSharing)
	}

	if state != "" {
		if state != stateEnabled && state != stateDisabled {
			return nil, fmt.Errorf("%w: invalid state %q", ErrValidation, state)
		}

		qs.State = state
	}

	cp := *qs
	cp.Tags = tagsCloneOrEmpty(qs.Tags)
	cp.CapacityLimits = cloneQuotaShareCapacityLimits(qs.CapacityLimits)

	return &cp, nil
}

// ListQuotaShares returns every quota share associated with jobQueue, sorted
// by quota share name. jobQueue is required and must reference an existing
// job queue (matching ListQuotaSharesInput, where jobQueue is a required
// field). Pagination (maxResults/nextToken) is applied by the caller (see
// handleListQuotaShares), matching the shared convention used by
// ListSchedulingPolicies/ListConsumableResources.
func (b *InMemoryBackend) ListQuotaShares(ctx context.Context, jobQueue string) ([]*QuotaShare, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListQuotaShares")
	defer b.mu.RUnlock()

	jq, ok := b.lookupJQByNameOrARN(region, jobQueue)
	if !ok {
		return nil, fmt.Errorf("%w: job queue %s not found", ErrNotFound, jobQueue)
	}

	group := b.quotaSharesByRegion.Get(region)
	list := make([]*QuotaShare, 0, len(group))

	for _, qs := range group {
		if qs.JobQueueArn != jq.JobQueueArn {
			continue
		}

		cp := *qs
		cp.Tags = tagsCloneOrEmpty(qs.Tags)
		cp.CapacityLimits = cloneQuotaShareCapacityLimits(qs.CapacityLimits)
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].QuotaShareName < list[j].QuotaShareName })

	return list, nil
}
