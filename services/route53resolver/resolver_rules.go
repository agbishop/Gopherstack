package route53resolver

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) resolverRulePoliciesStore(region string) map[string]string {
	if b.resolverRulePolicies[region] == nil {
		b.resolverRulePolicies[region] = make(map[string]string)
	}

	return b.resolverRulePolicies[region]
}

// resolverRulePoliciesStoreRO returns the region-scoped resolverRulePolicies
// map for region without mutating the outer map. Safe to call while holding
// only b.mu.RLock(): if the region has not been observed yet, it returns a
// fresh, unregistered, empty map instead of lazily creating (and
// persisting) an entry.
func (b *InMemoryBackend) resolverRulePoliciesStoreRO(region string) map[string]string {
	if v := b.resolverRulePolicies[region]; v != nil {
		return v
	}

	return map[string]string{}
}

func (b *InMemoryBackend) CreateResolverRule(
	ctx context.Context,
	name, domainName, ruleType, endpointID, creatorRequestID, delegationRecord string,
	targetIps []TargetIP,
) (*ResolverRule, error) {
	b.mu.Lock("CreateResolverRule")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidation)
	}

	if domainName == "" {
		return nil, fmt.Errorf("%w: DomainName is required", ErrValidation)
	}

	switch ruleType {
	case ruleTypeForward, ruleTypeSystem, ruleTypeRecursive:
		// valid
	default:
		return nil, fmt.Errorf(
			"%w: RuleType must be %s, %s, or %s",
			ErrValidation,
			ruleTypeForward,
			ruleTypeSystem,
			ruleTypeRecursive,
		)
	}

	// SYSTEM and RECURSIVE rules must not have TargetIps or ResolverEndpointId.
	if ruleType == ruleTypeSystem || ruleType == ruleTypeRecursive {
		if endpointID != "" {
			return nil, fmt.Errorf(
				"%w: SYSTEM/RECURSIVE rules must not have a ResolverEndpointId",
				ErrValidation,
			)
		}
		if len(targetIps) > 0 {
			return nil, fmt.Errorf(
				"%w: SYSTEM/RECURSIVE rules must not have TargetIps",
				ErrValidation,
			)
		}
	}

	if endpointID != "" {
		if !b.endpoints.Has(regionalKey(region, endpointID)) {
			return nil, fmt.Errorf("%w: resolver endpoint %s not found", ErrNotFound, endpointID)
		}
	}

	var tipsCopy []TargetIP
	if len(targetIps) > 0 {
		tipsCopy = make([]TargetIP, len(targetIps))
		copy(tipsCopy, targetIps)
	}

	if r, handled, err := b.matchExistingRuleByCreatorRequestID(
		region, creatorRequestID, name, domainName, ruleType, endpointID, delegationRecord, tipsCopy,
	); handled {
		return r, err
	}

	now := currentTime()
	id := "rslvr-rr-" + uuid.New().String()[:8]
	ruleARN := arn.Build("route53resolver", region, b.accountID, "resolver-rule/"+id)
	r := &ResolverRule{
		ID:                 id,
		ARN:                ruleARN,
		Name:               name,
		DomainName:         domainName,
		RuleType:           ruleType,
		Status:             statusComplete,
		ShareStatus:        shareStatusNotShared,
		ResolverEndpointID: endpointID,
		AccountID:          b.accountID,
		Region:             region,
		TargetIps:          tipsCopy,
		CreatorRequestID:   creatorRequestID,
		DelegationRecord:   delegationRecord,
		OwnerID:            b.accountID,
		CreationTime:       now,
		ModificationTime:   now,
	}
	b.rules.Put(r)
	cp := cloneRule(r)

	return cp, nil
}

// matchExistingRuleByCreatorRequestID looks up an existing resolver rule
// created under creatorRequestID. handled is false when creatorRequestID is
// empty or no match is found (caller proceeds to create); when handled is
// true, the caller must return (r, err) as-is -- either the existing
// resource (matching retry) or a ResourceExistsException (conflicting
// retry).
func (b *InMemoryBackend) matchExistingRuleByCreatorRequestID(
	region, creatorRequestID, name, domainName, ruleType, endpointID, delegationRecord string,
	targetIps []TargetIP,
) (*ResolverRule, bool, error) {
	if creatorRequestID == "" {
		return nil, false, nil
	}

	for _, existing := range b.rulesByRegion.Get(region) {
		if existing.CreatorRequestID != creatorRequestID {
			continue
		}
		if existing.Name == name &&
			existing.DomainName == domainName &&
			existing.RuleType == ruleType &&
			existing.ResolverEndpointID == endpointID &&
			existing.DelegationRecord == delegationRecord &&
			slices.Equal(existing.TargetIps, targetIps) {
			return cloneRule(existing), true, nil
		}

		return nil, true, fmt.Errorf(
			"%w: a resolver rule already exists for CreatorRequestId %s with different parameters",
			ErrAlreadyExists, creatorRequestID,
		)
	}

	return nil, false, nil
}

func (b *InMemoryBackend) GetResolverRule(ctx context.Context, id string) (*ResolverRule, error) {
	b.mu.RLock("GetResolverRule")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	r, ok := b.rules.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf("%w: resolver rule %s not found", ErrNotFound, id)
	}

	return cloneRule(r), nil
}

func (b *InMemoryBackend) ListResolverRules(ctx context.Context) []*ResolverRule {
	b.mu.RLock("ListResolverRules")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	regionRules := b.rulesByRegion.Get(region)
	list := make([]*ResolverRule, 0, len(regionRules))
	for _, r := range regionRules {
		list = append(list, cloneRule(r))
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return list
}

func (b *InMemoryBackend) DeleteResolverRule(ctx context.Context, id string) error {
	b.mu.Lock("DeleteResolverRule")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	r, ok := b.rules.Get(regionalKey(region, id))
	if !ok {
		return fmt.Errorf("%w: resolver rule %s not found", ErrNotFound, id)
	}

	tags := b.tagsStore(region)

	// Clean up tags.
	delete(tags, r.ARN)
	// Clean up the resource policy. Direct map access (not the lazy-creating
	// Store helper) so deleting a rule whose region never had a policy set
	// doesn't leave behind an empty region entry.
	delete(b.resolverRulePolicies[region], r.ARN)

	// Cascade: delete all associations referencing this rule. slices.Clone
	// before deleting in the loop -- see DeleteResolverEndpoint's comment.
	regionAssocs := slices.Clone(b.ruleAssociationsByRegion.Get(region))
	for _, assoc := range regionAssocs {
		if assoc.ResolverRuleID == id {
			b.ruleAssociations.Delete(regionalKey(region, assoc.ID))
		}
	}

	b.rules.Delete(regionalKey(region, id))

	return nil
}

// cloneRule returns a deep copy of a ResolverRule.
func cloneRule(r *ResolverRule) *ResolverRule {
	cp := *r
	if r.TargetIps != nil {
		cp.TargetIps = make([]TargetIP, len(r.TargetIps))
		copy(cp.TargetIps, r.TargetIps)
	}

	return &cp
}

// AddRuleInternal adds a resolver rule directly to the backend (test seed helper).
func (b *InMemoryBackend) AddRuleInternal(name, domainName, ruleType string) *ResolverRule {
	b.mu.Lock("AddRuleInternal")
	defer b.mu.Unlock()

	id := "rslvr-rr-" + uuid.New().String()[:8]
	ruleARN := arn.Build("route53resolver", b.region, b.accountID, "resolver-rule/"+id)
	r := &ResolverRule{
		ID:          id,
		ARN:         ruleARN,
		Name:        name,
		DomainName:  domainName,
		RuleType:    ruleType,
		Status:      statusComplete,
		ShareStatus: shareStatusNotShared,
		AccountID:   b.accountID,
		Region:      b.region,
	}
	b.rules.Put(r)

	return cloneRule(r)
}

// AddRuleInternalWithEndpoint adds a resolver rule with an endpoint ID directly to the backend (demo seed helper).
func (b *InMemoryBackend) AddRuleInternalWithEndpoint(
	name, domainName, ruleType, endpointID string,
) *ResolverRule {
	b.mu.Lock("AddRuleInternalWithEndpoint")
	defer b.mu.Unlock()

	id := "rslvr-rr-" + uuid.New().String()[:8]
	ruleARN := arn.Build("route53resolver", b.region, b.accountID, "resolver-rule/"+id)
	r := &ResolverRule{
		ID:                 id,
		ARN:                ruleARN,
		Name:               name,
		DomainName:         domainName,
		RuleType:           ruleType,
		Status:             statusComplete,
		ShareStatus:        shareStatusNotShared,
		ResolverEndpointID: endpointID,
		AccountID:          b.accountID,
		Region:             b.region,
	}
	b.rules.Put(r)

	return cloneRule(r)
}

// GetResolverRulePolicy retrieves a resource policy for a resolver rule ARN.
func (b *InMemoryBackend) GetResolverRulePolicy(ctx context.Context, arnStr string) string {
	b.mu.RLock("GetResolverRulePolicy")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	return b.resolverRulePoliciesStoreRO(region)[arnStr]
}

// PutResolverRulePolicy stores a resource policy for a resolver rule ARN.
func (b *InMemoryBackend) PutResolverRulePolicy(ctx context.Context, arnStr, policy string) error {
	b.mu.Lock("PutResolverRulePolicy")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	b.resolverRulePoliciesStore(region)[arnStr] = policy

	return nil
}

// --- Resolver Endpoint Update ---

// UpdateResolverRule updates fields of a resolver rule.
func (b *InMemoryBackend) UpdateResolverRule(
	ctx context.Context,
	id, name, resolverEndpointID string,
	targetIps []TargetIP,
) (*ResolverRule, error) {
	b.mu.Lock("UpdateResolverRule")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	r, ok := b.rules.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf("%w: resolver rule %s not found", ErrNotFound, id)
	}
	if name != "" {
		r.Name = name
	}
	if resolverEndpointID != "" {
		r.ResolverEndpointID = resolverEndpointID
	}
	if targetIps != nil {
		tipsCopy := make([]TargetIP, len(targetIps))
		copy(tipsCopy, targetIps)
		r.TargetIps = tipsCopy
	}

	return cloneRule(r), nil
}
