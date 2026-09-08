package batch

import (
	"context"
	"fmt"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateSchedulingPolicy creates a new scheduling policy.
func (b *InMemoryBackend) CreateSchedulingPolicy(
	ctx context.Context,
	name string,
	tags map[string]string,
	fairsharePolicy *FairsharePolicy,
	quotaSharePolicy *QuotaSharePolicy,
) (*SchedulingPolicy, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateSchedulingPolicy")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}

	if err := validateTags(tags); err != nil {
		return nil, err
	}

	if len(b.schedulingPoliciesByName.Get(regionKey(region, name))) > 0 {
		return nil, fmt.Errorf("%w: scheduling policy %s already exists", ErrAlreadyExists, name)
	}

	policyARN := arn.Build("batch", region, b.accountID, "scheduling-policy/"+name)

	sp := &SchedulingPolicy{
		region:           region,
		Arn:              policyARN,
		Name:             name,
		Tags:             tagsCloneOrEmpty(tags),
		FairsharePolicy:  cloneFairsharePolicy(fairsharePolicy),
		QuotaSharePolicy: cloneQuotaSharePolicy(quotaSharePolicy),
	}
	b.schedulingPolicies.Put(sp)
	cp := *sp

	return &cp, nil
}

// cloneFairsharePolicy deep-copies a FairsharePolicy.
func cloneFairsharePolicy(fp *FairsharePolicy) *FairsharePolicy {
	if fp == nil {
		return nil
	}

	clone := *fp
	if len(fp.ShareDistribution) > 0 {
		sd := make([]ShareDistribution, len(fp.ShareDistribution))
		copy(sd, fp.ShareDistribution)
		clone.ShareDistribution = sd
	}

	return &clone
}

// cloneQuotaSharePolicy deep-copies a QuotaSharePolicy.
func cloneQuotaSharePolicy(qp *QuotaSharePolicy) *QuotaSharePolicy {
	if qp == nil {
		return nil
	}

	clone := *qp

	return &clone
}

// DeleteSchedulingPolicy removes a scheduling policy by ARN. Rejects a
// policy still referenced by any job queue (api_op_DeleteSchedulingPolicy.go).
func (b *InMemoryBackend) DeleteSchedulingPolicy(ctx context.Context, policyARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteSchedulingPolicy")
	defer b.mu.Unlock()

	if !b.schedulingPolicies.Has(regionKey(region, policyARN)) {
		return fmt.Errorf("%w: scheduling policy %s not found", ErrNotFound, policyARN)
	}

	for _, jq := range b.jobQueuesByRegion.Get(region) {
		if jq.SchedulingPolicyArn == policyARN {
			return fmt.Errorf(
				"%w: scheduling policy %s is used by job queue %s",
				ErrValidation, policyARN, jq.JobQueueName,
			)
		}
	}

	b.schedulingPolicies.Delete(regionKey(region, policyARN))

	return nil
}

// ListSchedulingPolicies returns all scheduling policies sorted by ARN.
func (b *InMemoryBackend) ListSchedulingPolicies(ctx context.Context) []*SchedulingPolicy {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListSchedulingPolicies")
	defer b.mu.RUnlock()

	group := b.schedulingPoliciesByRegion.Get(region)
	list := make([]*SchedulingPolicy, 0, len(group))

	for _, sp := range group {
		cp := *sp
		cp.Tags = tagsCloneOrEmpty(sp.Tags)
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Arn < list[j].Arn })

	return list
}

// DescribeSchedulingPolicies returns scheduling policies, optionally filtered by ARNs.
func (b *InMemoryBackend) DescribeSchedulingPolicies(ctx context.Context, arns []string) []*SchedulingPolicy {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeSchedulingPolicies")
	defer b.mu.RUnlock()

	if len(arns) == 0 {
		group := b.schedulingPoliciesByRegion.Get(region)
		list := make([]*SchedulingPolicy, 0, len(group))

		for _, sp := range group {
			cp := *sp
			cp.Tags = tagsCloneOrEmpty(sp.Tags)
			list = append(list, &cp)
		}

		sort.Slice(list, func(i, j int) bool { return list[i].Arn < list[j].Arn })

		return list
	}

	list := make([]*SchedulingPolicy, 0, len(arns))

	for _, a := range arns {
		if sp, ok := b.schedulingPolicies.Get(regionKey(region, a)); ok {
			cp := *sp
			cp.Tags = tagsCloneOrEmpty(sp.Tags)
			list = append(list, &cp)
		}
	}

	return list
}

// UpdateSchedulingPolicy updates a scheduling policy's fairshare/quota-share configuration.
func (b *InMemoryBackend) UpdateSchedulingPolicy(
	ctx context.Context,
	policyARN string,
	fairsharePolicy *FairsharePolicy,
	quotaSharePolicy *QuotaSharePolicy,
) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateSchedulingPolicy")
	defer b.mu.Unlock()

	sp, ok := b.schedulingPolicies.Get(regionKey(region, policyARN))
	if !ok {
		return fmt.Errorf("%w: scheduling policy %s not found", ErrNotFound, policyARN)
	}

	if fairsharePolicy != nil {
		sp.FairsharePolicy = cloneFairsharePolicy(fairsharePolicy)
	}

	if quotaSharePolicy != nil {
		sp.QuotaSharePolicy = cloneQuotaSharePolicy(quotaSharePolicy)
	}

	return nil
}
