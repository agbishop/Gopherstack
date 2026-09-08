package dlm

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// maxPoliciesPerRegion is AWS's documented default "Policies per Region"
// quota (adjustable; quota code L-5407D8DA,
// docs.aws.amazon.com/general/latest/gr/dlm.html).
const maxPoliciesPerRegion = 100

// CreateLifecyclePolicy creates a new lifecycle policy and returns it.
func (b *InMemoryBackend) CreateLifecyclePolicy(
	description, executionRoleARN, state string,
	tags map[string]string,
	policyDetails map[string]any,
) (*Policy, error) {
	b.mu.Lock("CreateLifecyclePolicy")
	defer b.mu.Unlock()

	// ExecutionRoleArn and Description are "This member is required" fields
	// on CreateLifecyclePolicyInput (State is too, but callers may rely on
	// the ENABLED default below, matching this backend's documented
	// leniency for State).
	if description == "" || executionRoleARN == "" {
		return nil, ErrInvalidRequest
	}

	if b.policies.Len() >= maxPoliciesPerRegion {
		return nil, ErrLimitExceeded
	}

	b.counter++
	policyID := fmt.Sprintf("%s%016x", policyIDPrefix, b.counter)
	policyARN := arn.Build("dlm", b.region, b.accountID, fmt.Sprintf("policy/%s", policyID))

	now := time.Now().UTC()
	resolvedState := state
	if resolvedState == "" {
		resolvedState = stateEnabled
	}

	storedTags := make(map[string]string)
	maps.Copy(storedTags, tags)

	p := &storedPolicy{
		DateCreated:      now,
		DateModified:     now,
		Description:      description,
		ExecutionRoleARN: executionRoleARN,
		PolicyArn:        policyARN,
		PolicyID:         policyID,
		State:            resolvedState,
		Tags:             storedTags,
		PolicyDetails:    policyDetails,
	}
	b.policies.Put(p)

	return p.toPolicy(), nil
}

// DeleteLifecyclePolicy removes a lifecycle policy.
func (b *InMemoryBackend) DeleteLifecyclePolicy(policyID string) error {
	b.mu.Lock("DeleteLifecyclePolicy")
	defer b.mu.Unlock()

	if !b.policies.Has(policyID) {
		return ErrPolicyNotFound
	}

	b.policies.Delete(policyID)

	return nil
}

// GetLifecyclePolicies returns summary info for lifecycle policies, optionally
// narrowed by filter (see PolicyFilter for the matching semantics).
func (b *InMemoryBackend) GetLifecyclePolicies(filter PolicyFilter) ([]*PolicySummary, error) {
	b.mu.RLock("GetLifecyclePolicies")
	defer b.mu.RUnlock()

	idFilter := make(map[string]struct{}, len(filter.PolicyIDs))
	for _, id := range filter.PolicyIDs {
		idFilter[id] = struct{}{}
	}

	wantTargetTags := parseTagPairs(filter.TargetTags)
	wantTagsToAdd := parseTagPairs(filter.TagsToAdd)

	var result []*PolicySummary

	for _, p := range b.policies.All() {
		if len(idFilter) > 0 {
			if _, ok := idFilter[p.PolicyID]; !ok {
				continue
			}
		}

		if filter.State != "" && !strings.EqualFold(p.State, filter.State) {
			continue
		}

		if !matchesResourceTypes(p.PolicyDetails, filter.ResourceTypes) {
			continue
		}

		if !matchesDefaultPolicyType(p.PolicyDetails, filter.DefaultPolicyType) {
			continue
		}

		if !matchesAnyTagPair(policyDetailsTagPairs(p.PolicyDetails, "TargetTags"), wantTargetTags) {
			continue
		}

		if !matchesAnyTagPair(policyDetailsScheduleTagsToAdd(p.PolicyDetails), wantTagsToAdd) {
			continue
		}

		result = append(result, p.toSummary())
	}

	sort.Slice(result, func(i, j int) bool { return result[i].PolicyID < result[j].PolicyID })

	return result, nil
}

// GetLifecyclePolicy returns full details for a lifecycle policy.
func (b *InMemoryBackend) GetLifecyclePolicy(policyID string) (*Policy, error) {
	b.mu.RLock("GetLifecyclePolicy")
	defer b.mu.RUnlock()

	p, ok := b.policies.Get(policyID)
	if !ok {
		return nil, ErrPolicyNotFound
	}

	return p.toPolicy(), nil
}

// UpdateLifecyclePolicy updates mutable fields of an existing policy.
//
// description and executionRoleARN follow presence semantics, not
// truthiness: nil means the field was omitted from the request (leave
// unchanged), a non-nil pointer to "" means the caller explicitly cleared
// it. See the StorageBackend.UpdateLifecyclePolicy doc comment for why.
func (b *InMemoryBackend) UpdateLifecyclePolicy(
	policyID string, description, executionRoleARN *string, state string,
	policyDetails map[string]any, defaultPolicyOverrides map[string]any,
) error {
	b.mu.Lock("UpdateLifecyclePolicy")
	defer b.mu.Unlock()

	p, ok := b.policies.Get(policyID)
	if !ok {
		return ErrPolicyNotFound
	}

	if description != nil {
		p.Description = *description
	}

	if executionRoleARN != nil {
		p.ExecutionRoleARN = *executionRoleARN
	}

	if state != "" {
		p.State = state
	}

	if policyDetails != nil || len(defaultPolicyOverrides) > 0 {
		base := policyDetails
		if base == nil {
			base = p.PolicyDetails
		}

		merged := make(map[string]any, len(base)+len(defaultPolicyOverrides))
		maps.Copy(merged, base)
		maps.Copy(merged, defaultPolicyOverrides)
		p.PolicyDetails = merged
	}

	p.DateModified = time.Now().UTC()

	return nil
}
