package verifiedpermissions

import (
	"fmt"
	"sort"
	"strings"
	"time"

	cedar "github.com/cedar-policy/cedar-go"
	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// policyARN builds the ARN for a policy.
func policyARN(accountID, policyStoreID, policyID string) string {
	return arn.Build("verifiedpermissions", "", accountID, fmt.Sprintf("policy/%s/%s", policyStoreID, policyID))
}

// clonePolicy returns a deep copy of a Policy.
func clonePolicy(p *Policy) *Policy {
	cp := *p

	return &cp
}

// policyKey, policyTemplateKey, and identitySourceKey build the composite
// store.Table primary key ("policyStoreID/id") shared by the resource tables
// that were previously nested by policy store (see store_setup.go).
func policyKey(policyStoreID, policyID string) string { return policyStoreID + "/" + policyID }

// parseCedarStatement validates a Cedar policy statement using the cedar-go parser.
func parseCedarStatement(statement string) error {
	if _, err := cedar.NewPolicyListFromBytes("policy.cedar", []byte(statement)); err != nil {
		return fmt.Errorf("%w: Cedar syntax error: %w", ErrValidation, err)
	}

	return nil
}

// CreatePolicy creates a new policy in the given policy store.
func (b *InMemoryBackend) CreatePolicy(policyStoreID string, params CreatePolicyParams) (*Policy, error) {
	b.mu.Lock("CreatePolicy")
	defer b.mu.Unlock()

	if !b.policyStores.Has(policyStoreID) {
		return nil, fmt.Errorf("%w: policy store %s not found", ErrPolicyStoreNotFound, policyStoreID)
	}

	fingerprint := createPolicyFingerprint(policyStoreID, params)

	existingID, err := b.checkClientToken("CreatePolicy", params.ClientToken, fingerprint)
	if err != nil {
		return nil, err
	}

	if existingID != "" {
		if existing, ok := b.policies.Get(policyKey(policyStoreID, existingID)); ok {
			return clonePolicy(existing), nil
		}
	}

	if params.PolicyType == policyTypeStatic {
		if statementErr := parseCedarStatement(params.Statement); statementErr != nil {
			return nil, statementErr
		}
	}

	if params.PolicyType == policyTypeTemplateLinked {
		// Validate the referenced template exists.
		if !b.policyTemplates.Has(policyTemplateKey(policyStoreID, params.PolicyTemplateID)) {
			return nil, fmt.Errorf(
				"%w: policy template %s not found in policy store %s",
				ErrPolicyTemplateNotFound, params.PolicyTemplateID, policyStoreID,
			)
		}
	}

	id := uuid.NewString()
	now := time.Now()
	p := &Policy{
		PolicyID:            id,
		PolicyStoreID:       policyStoreID,
		PolicyType:          params.PolicyType,
		Statement:           params.Statement,
		Description:         params.Description,
		PolicyTemplateID:    params.PolicyTemplateID,
		PrincipalEntityType: params.PrincipalEntityType,
		PrincipalEntityID:   params.PrincipalEntityID,
		ResourceEntityType:  params.ResourceEntityType,
		ResourceEntityID:    params.ResourceEntityID,
		CreatedDate:         now,
		LastUpdated:         now,
	}
	b.policies.Put(p)
	b.arnIndex[policyARN(b.accountID, policyStoreID, id)] = arnKindPolicy + ":" + policyStoreID + ":" + id
	b.invalidatePolicySetCache(policyStoreID)
	b.recordClientToken("CreatePolicy", params.ClientToken, fingerprint, id)

	return clonePolicy(p), nil
}

// createPolicyFingerprint deterministically encodes the parameters of a
// CreatePolicy call for ClientToken idempotency (see
// InMemoryBackend.checkClientToken): a retry with the same ClientToken but a
// different fingerprint is a real AWS ConflictException.
func createPolicyFingerprint(policyStoreID string, params CreatePolicyParams) string {
	return strings.Join([]string{
		policyStoreID, params.PolicyType, params.Statement, params.Description,
		params.PolicyTemplateID, params.PrincipalEntityType, params.PrincipalEntityID,
		params.ResourceEntityType, params.ResourceEntityID,
	}, "\x00")
}

// GetPolicy returns the policy with the given ID.
func (b *InMemoryBackend) GetPolicy(policyStoreID, policyID string) (*Policy, error) {
	b.mu.RLock("GetPolicy")
	defer b.mu.RUnlock()

	if !b.policyStores.Has(policyStoreID) {
		return nil, fmt.Errorf("%w: policy store %s not found", ErrPolicyStoreNotFound, policyStoreID)
	}

	p, ok := b.policies.Get(policyKey(policyStoreID, policyID))
	if !ok {
		return nil, fmt.Errorf("%w: policy %s not found", ErrPolicyNotFound, policyID)
	}

	return clonePolicy(p), nil
}

// ListPolicies returns policies in a policy store, with optional filter and pagination.
func (b *InMemoryBackend) ListPolicies(
	policyStoreID string,
	filter ListPoliciesFilter,
	nextToken string,
	maxResults int,
) ([]Policy, string, error) {
	b.mu.RLock("ListPolicies")
	defer b.mu.RUnlock()

	if !b.policyStores.Has(policyStoreID) {
		return nil, "", fmt.Errorf("%w: policy store %s not found", ErrPolicyStoreNotFound, policyStoreID)
	}

	policies := b.policiesByStore.Get(policyStoreID)
	out := make([]Policy, 0, len(policies))

	for _, p := range policies {
		if !matchesPolicyFilter(p, filter) {
			continue
		}

		out = append(out, *clonePolicy(p))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedDate.Before(out[j].CreatedDate)
	})

	page, tok := paginate(out, nextToken, maxResults, func(p Policy) string { return p.PolicyID })

	return page, tok, nil
}

// matchesPolicyFilter returns true if the policy matches all non-empty filter fields.
func matchesPolicyFilter(p *Policy, f ListPoliciesFilter) bool {
	if f.PolicyType != "" && p.PolicyType != f.PolicyType {
		return false
	}

	if f.PolicyTemplateID != "" && p.PolicyTemplateID != f.PolicyTemplateID {
		return false
	}

	return matchesPrincipalFilter(p, f) && matchesResourceFilter(p, f)
}

// matchesPrincipalFilter checks the principal-scope portion of f.
func matchesPrincipalFilter(p *Policy, f ListPoliciesFilter) bool {
	if f.PrincipalUnspecified && (p.PrincipalEntityType != "" || p.PrincipalEntityID != "") {
		return false
	}

	if f.PrincipalEntityType != "" && p.PrincipalEntityType != f.PrincipalEntityType {
		return false
	}

	if f.PrincipalEntityID != "" && p.PrincipalEntityID != f.PrincipalEntityID {
		return false
	}

	return true
}

// matchesResourceFilter checks the resource-scope portion of f.
func matchesResourceFilter(p *Policy, f ListPoliciesFilter) bool {
	if f.ResourceUnspecified && (p.ResourceEntityType != "" || p.ResourceEntityID != "") {
		return false
	}

	if f.ResourceEntityType != "" && p.ResourceEntityType != f.ResourceEntityType {
		return false
	}

	if f.ResourceEntityID != "" && p.ResourceEntityID != f.ResourceEntityID {
		return false
	}

	return true
}

// UpdatePolicy updates an existing static policy. Real AWS's
// UpdatePolicyDefinition union has only a "static" member -- there is no
// templateLinked variant -- and the operation doc is explicit: "You can
// directly update only static policies. To change a template-linked policy,
// you must update the template instead, using UpdatePolicyTemplate." A
// template-linked target is therefore always rejected here.
func (b *InMemoryBackend) UpdatePolicy(policyStoreID, policyID string, params UpdatePolicyParams) (*Policy, error) {
	b.mu.Lock("UpdatePolicy")
	defer b.mu.Unlock()

	if !b.policyStores.Has(policyStoreID) {
		return nil, fmt.Errorf("%w: policy store %s not found", ErrPolicyStoreNotFound, policyStoreID)
	}

	p, ok := b.policies.Get(policyKey(policyStoreID, policyID))
	if !ok {
		return nil, fmt.Errorf("%w: policy %s not found", ErrPolicyNotFound, policyID)
	}

	if p.PolicyType != policyTypeStatic {
		return nil, fmt.Errorf(
			"%w: policy %s is template-linked; update the policy template instead", ErrValidation, policyID,
		)
	}

	if params.Statement != "" {
		if err := parseCedarStatement(params.Statement); err != nil {
			return nil, err
		}

		p.Statement = params.Statement
	}

	if params.Description != "" {
		p.Description = params.Description
	}

	p.LastUpdated = time.Now()
	b.invalidatePolicySetCache(policyStoreID)

	return clonePolicy(p), nil
}

// DeletePolicy removes a policy from the given policy store. Idempotent for
// a nonexistent policyID, matching the real SDK's documented idempotency
// ("This operation is idempotent; if you specify a policy that doesn't
// exist, the request response returns a successful HTTP 200 status code").
// A nonexistent policyStoreID still errors: ResourceNotFoundException
// remains in DeletePolicy's modelled error set (unlike DeletePolicyStore's),
// so the idempotency only covers the policy, not its store.
func (b *InMemoryBackend) DeletePolicy(policyStoreID, policyID string) error {
	b.mu.Lock("DeletePolicy")
	defer b.mu.Unlock()

	if !b.policyStores.Has(policyStoreID) {
		return fmt.Errorf("%w: policy store %s not found", ErrPolicyStoreNotFound, policyStoreID)
	}

	if !b.policies.Has(policyKey(policyStoreID, policyID)) {
		return nil
	}

	resourceARN := policyARN(b.accountID, policyStoreID, policyID)
	delete(b.arnIndex, resourceARN)
	delete(b.resourceTags, resourceARN)
	b.policies.Delete(policyKey(policyStoreID, policyID))
	b.invalidatePolicySetCache(policyStoreID)

	return nil
}

// BatchGetPolicy retrieves multiple policies in a single request.
func (b *InMemoryBackend) BatchGetPolicy(items []BatchGetPolicyItem) BatchGetPolicyResult {
	// Snapshot needed entries under the lock, then format outside.
	b.mu.RLock("BatchGetPolicy")

	type entry struct {
		policy *Policy
		err    *batchGetPolicyErrorItem
	}

	entries := make([]entry, 0, len(items))

	for _, item := range items {
		if !b.policyStores.Has(item.PolicyStoreID) {
			entries = append(entries, entry{err: &batchGetPolicyErrorItem{
				PolicyStoreID: item.PolicyStoreID,
				PolicyID:      item.PolicyID,
				Code:          "POLICY_STORE_NOT_FOUND",
				Message:       fmt.Sprintf("policy store %s not found", item.PolicyStoreID),
			}})

			continue
		}

		p, ok := b.policies.Get(policyKey(item.PolicyStoreID, item.PolicyID))
		if !ok {
			entries = append(entries, entry{err: &batchGetPolicyErrorItem{
				PolicyStoreID: item.PolicyStoreID,
				PolicyID:      item.PolicyID,
				Code:          "POLICY_NOT_FOUND",
				Message:       fmt.Sprintf("policy %s not found", item.PolicyID),
			}})

			continue
		}

		copied := *clonePolicy(p)
		entries = append(entries, entry{policy: &copied})
	}

	b.mu.RUnlock()

	result := BatchGetPolicyResult{
		Results: make([]Policy, 0, len(items)),
		Errors:  make([]batchGetPolicyErrorItem, 0, len(items)),
	}

	for _, e := range entries {
		if e.err != nil {
			result.Errors = append(result.Errors, *e.err)
		} else {
			result.Results = append(result.Results, *e.policy)
		}
	}

	return result
}
