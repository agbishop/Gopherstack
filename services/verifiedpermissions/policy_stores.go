package verifiedpermissions

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
)

// policyStoreARN builds the ARN for a policy store.
func policyStoreARN(accountID, _, policyStoreID string) string {
	return arnNoRegion(accountID, "policy-store", policyStoreID)
}

// clonePolicyStore returns a deep copy of a PolicyStore.
func clonePolicyStore(ps *PolicyStore) *PolicyStore {
	cp := *ps
	cp.Tags = make(map[string]string, len(ps.Tags))
	maps.Copy(cp.Tags, ps.Tags)

	return &cp
}

// CreatePolicyStore creates a new policy store. A non-empty clientToken
// makes the call idempotent for eight hours: a retry with the same token and
// the same parameters replays the original policy store instead of creating
// a duplicate, and a retry with the same token but different parameters
// fails with ErrConflict, matching real AWS's documented ClientToken
// semantics.
func (b *InMemoryBackend) CreatePolicyStore(
	description string,
	tags map[string]string,
	validationMode, deletionProtection, clientToken string,
) (*PolicyStore, error) {
	b.mu.Lock("CreatePolicyStore")
	defer b.mu.Unlock()

	fingerprint := description + "\x00" + validationMode + "\x00" + deletionProtection + "\x00" + tagsFingerprint(tags)

	existingID, err := b.checkClientToken("CreatePolicyStore", clientToken, fingerprint)
	if err != nil {
		return nil, err
	}

	if existingID != "" {
		if existing, ok := b.policyStores.Get(existingID); ok {
			return clonePolicyStore(existing), nil
		}
	}

	merged := make(map[string]string, len(tags))
	maps.Copy(merged, tags)

	if tagErr := validateTagInput(nil, merged, ErrValidation); tagErr != nil {
		return nil, tagErr
	}

	id := uuid.NewString()
	now := time.Now()

	if deletionProtection == "" {
		deletionProtection = DeletionProtectionDisabled
	}

	ps := &PolicyStore{
		PolicyStoreID:      id,
		Arn:                policyStoreARN(b.accountID, b.region, id),
		Description:        description,
		CreatedDate:        now,
		LastUpdated:        now,
		Tags:               merged,
		AccountID:          b.accountID,
		Region:             b.region,
		ValidationMode:     validationMode,
		DeletionProtection: deletionProtection,
	}
	b.policyStores.Put(ps)
	b.arnIndex[ps.Arn] = arnKindPolicyStore + ":" + id
	if len(merged) > 0 {
		b.resourceTags[ps.Arn] = maps.Clone(merged)
	}

	b.recordClientToken("CreatePolicyStore", clientToken, fingerprint, id)

	return clonePolicyStore(ps), nil
}

// GetPolicyStore returns the policy store with the given ID.
func (b *InMemoryBackend) GetPolicyStore(policyStoreID string) (*PolicyStore, error) {
	b.mu.RLock("GetPolicyStore")
	defer b.mu.RUnlock()

	ps, ok := b.policyStores.Get(policyStoreID)
	if !ok {
		return nil, fmt.Errorf("%w: policy store %s not found", ErrPolicyStoreNotFound, policyStoreID)
	}

	out := clonePolicyStore(ps)
	out.Tags = maps.Clone(b.resourceTags[ps.Arn])

	return out, nil
}

// ListPolicyStores returns all policy stores sorted by creation date (newest first).
func (b *InMemoryBackend) ListPolicyStores(nextToken string, maxResults int) ([]PolicyStore, string) {
	b.mu.RLock("ListPolicyStores")
	defer b.mu.RUnlock()

	all := b.policyStores.All()
	out := make([]PolicyStore, 0, len(all))
	for _, ps := range all {
		out = append(out, *clonePolicyStore(ps))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedDate.After(out[j].CreatedDate)
	})

	return paginate(out, nextToken, maxResults, func(ps PolicyStore) string { return ps.PolicyStoreID })
}

// UpdatePolicyStore updates a policy store.
func (b *InMemoryBackend) UpdatePolicyStore(
	policyStoreID, description, validationMode, deletionProtection string,
) (*PolicyStore, error) {
	b.mu.Lock("UpdatePolicyStore")
	defer b.mu.Unlock()

	ps, ok := b.policyStores.Get(policyStoreID)
	if !ok {
		return nil, fmt.Errorf("%w: policy store %s not found", ErrPolicyStoreNotFound, policyStoreID)
	}

	ps.Description = description
	if validationMode != "" {
		ps.ValidationMode = validationMode
	}

	if deletionProtection != "" {
		ps.DeletionProtection = deletionProtection
	}

	ps.LastUpdated = time.Now()

	return clonePolicyStore(ps), nil
}

// DeletePolicyStore removes a policy store and all its policies and
// templates. Idempotent: a nonexistent policyStoreID is a no-op success,
// matching the real SDK's documented idempotency ("This operation is
// idempotent. If you specify a policy store that does not exist, the
// request response will still return a successful HTTP 200 status code") --
// ResourceNotFoundException isn't even in DeletePolicyStore's modelled
// error set.
func (b *InMemoryBackend) DeletePolicyStore(policyStoreID string) error {
	b.mu.Lock("DeletePolicyStore")
	defer b.mu.Unlock()

	ps, ok := b.policyStores.Get(policyStoreID)
	if !ok {
		return nil
	}

	if ps.DeletionProtection == DeletionProtectionEnabled {
		return fmt.Errorf(
			"%w: policy store %s has deletion protection enabled", ErrPolicyStoreDeletionProtected, policyStoreID,
		)
	}

	// Remove ARN index and tag entries for all child resources, then delete
	// the child resources themselves. Index result slices mutate under
	// Delete, so clone before the delete loop.
	for _, p := range slices.Clone(b.policiesByStore.Get(policyStoreID)) {
		resourceARN := policyARN(b.accountID, policyStoreID, p.PolicyID)
		delete(b.arnIndex, resourceARN)
		delete(b.resourceTags, resourceARN)
		b.policies.Delete(policyKey(policyStoreID, p.PolicyID))
	}

	for _, pt := range slices.Clone(b.policyTemplatesByStore.Get(policyStoreID)) {
		resourceARN := policyTemplateARN(b.accountID, policyStoreID, pt.PolicyTemplateID)
		delete(b.arnIndex, resourceARN)
		delete(b.resourceTags, resourceARN)
		b.policyTemplates.Delete(policyTemplateKey(policyStoreID, pt.PolicyTemplateID))
	}

	for _, is := range slices.Clone(b.identitySourcesByStore.Get(policyStoreID)) {
		resourceARN := identitySourceARN(b.accountID, policyStoreID, is.IdentitySourceID)
		delete(b.arnIndex, resourceARN)
		delete(b.resourceTags, resourceARN)
		b.identitySources.Delete(identitySourceKey(policyStoreID, is.IdentitySourceID))
	}

	// Cascade-delete any aliases pointing at this store. The real API's docs
	// are silent on this (DeletePolicyStore predates policy store aliases
	// entirely, and its own doc page never mentions them) -- gopherstack
	// picks cascade-delete rather than leaving a dangling alias that would
	// resolve (via ResolvePolicyStoreAlias) to a policy store ID that no
	// longer exists, the same ghost-row bug class fixed elsewhere in this
	// campaign (e.g. emr sessions surviving cluster termination). Aliases
	// carry no arnIndex/resourceTags entries (see PolicyStoreAlias's doc
	// comment), so nothing else needs cleaning up for them here.
	for _, a := range slices.Clone(b.policyStoreAliasesByStore.Get(policyStoreID)) {
		b.policyStoreAliases.Delete(a.AliasName)
	}

	delete(b.arnIndex, ps.Arn)
	delete(b.resourceTags, ps.Arn)
	b.policyStores.Delete(policyStoreID)
	b.schemas.Delete(policyStoreID)
	delete(b.policySetCache, policyStoreID)
	delete(b.policySetDirty, policyStoreID)

	return nil
}
