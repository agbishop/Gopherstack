package organizations

import (
	"cmp"
	"slices"
)

// maxOUDepth is the maximum depth for organizational units (root=0, OUs=1-5).
const maxOUDepth = 5

// ouDepthLocked computes the depth of a parent node (root = 0, direct child = 1, etc.)
// Must be called with lock held.
func (b *InMemoryBackend) ouDepthLocked(parentID string) int {
	depth := 0
	current := parentID

	for {
		if b.root != nil && current == b.root.ID {
			return depth
		}

		if parentOfCurrent, ok := b.ouParent[current]; ok {
			depth++
			current = parentOfCurrent
		} else {
			return depth
		}
	}
}

// CreateOrganizationalUnit creates a new OU under the given parent.
func (b *InMemoryBackend) CreateOrganizationalUnit(
	parentID, name string,
	tags []Tag,
) (*OrganizationalUnit, error) {
	b.mu.Lock("CreateOrganizationalUnit")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if !b.parentExists(parentID) {
		return nil, ErrInvalidInput
	}

	// Depth limit: root is depth 0, OUs are depth 1-5; creating at depth 6 is rejected.
	// parentID is the parent; the new OU's depth = ouDepthLocked(parentID) + 1.
	// If the parent's depth is already maxOUDepth, the new OU would be at depth 6, which is invalid.
	if b.ouDepthLocked(parentID) >= maxOUDepth {
		return nil, ErrOUDepthLimitExceeded
	}

	// O(1) sibling name uniqueness check via ousByParent index.
	if siblings := b.ousByParent[parentID]; siblings != nil {
		if _, exists := siblings[name]; exists {
			return nil, ErrDuplicateOrganizationalUnit
		}
	}

	if err := validateNewTags(nil, tags); err != nil {
		return nil, err
	}

	ouID := newOUID(b.root.ID)
	ou := &OrganizationalUnit{
		ID:       ouID,
		ARN:      b.ouARN(b.org.ID, ouID),
		Name:     name,
		ParentID: parentID,
	}

	b.ous.Put(ou)
	b.ouParent[ouID] = parentID
	if b.ousByParent[parentID] == nil {
		b.ousByParent[parentID] = make(map[string]string)
	}
	b.ousByParent[parentID][name] = ouID
	b.setTagsLocked(ouID, tags)
	ou.Path = b.ouPathLocked(ou)

	return ou, nil
}

// DescribeOrganizationalUnit returns an OU by ID.
func (b *InMemoryBackend) DescribeOrganizationalUnit(ouID string) (*OrganizationalUnit, error) {
	b.mu.RLock("DescribeOrganizationalUnit")
	defer b.mu.RUnlock()

	ou, ok := b.ous.Get(ouID)
	if !ok {
		return nil, ErrOUNotFound
	}

	cp := copyOU(ou)
	cp.Path = b.ouPathLocked(cp)

	return cp, nil
}

// DeleteOrganizationalUnit removes an OU.
func (b *InMemoryBackend) DeleteOrganizationalUnit(ouID string) error {
	b.mu.Lock("DeleteOrganizationalUnit")
	defer b.mu.Unlock()

	ou, ok := b.ous.Get(ouID)
	if !ok {
		return ErrOUNotFound
	}

	// AWS rejects deletion of OUs that still contain accounts or child OUs
	// with OrganizationalUnitNotEmptyException, not InvalidInputException.
	if len(b.accountChildrenByParent[ouID]) > 0 {
		return ErrOrganizationalUnitNotEmpty
	}

	if len(b.ousByParent[ouID]) > 0 {
		return ErrOrganizationalUnitNotEmpty
	}

	b.ous.Delete(ouID)
	parentID := b.ouParent[ouID]
	delete(b.ouParent, ouID)
	if siblings := b.ousByParent[parentID]; siblings != nil {
		delete(siblings, ou.Name)
	}
	delete(b.tags, ouID)

	// Clean the reverse index too: otherwise a deleted OU lingers as a
	// "target" in ListTargetsForPolicy for any policy that was attached to it.
	for _, policyID := range b.targetPolicies[ouID] {
		b.policyTargets[policyID] = removeString(b.policyTargets[policyID], ouID)
	}

	delete(b.targetPolicies, ouID)

	return nil
}

// UpdateOrganizationalUnit renames an OU.
func (b *InMemoryBackend) UpdateOrganizationalUnit(ouID, name string) (*OrganizationalUnit, error) {
	b.mu.Lock("UpdateOrganizationalUnit")
	defer b.mu.Unlock()

	ou, ok := b.ous.Get(ouID)
	if !ok {
		return nil, ErrOUNotFound
	}

	// O(1) sibling name uniqueness check via ousByParent index (excluding self).
	if name != "" && name != ou.Name {
		parentID := b.ouParent[ouID]
		if siblings := b.ousByParent[parentID]; siblings != nil {
			if existingID, exists := siblings[name]; exists && existingID != ouID {
				return nil, ErrDuplicateOrganizationalUnit
			}
		}
		// Update the index: remove old name, add new name.
		if b.ousByParent[parentID] == nil {
			b.ousByParent[parentID] = make(map[string]string)
		}
		delete(b.ousByParent[parentID], ou.Name)
		b.ousByParent[parentID][name] = ouID
	}

	ou.Name = name

	cp := copyOU(ou)
	cp.Path = b.ouPathLocked(cp)

	return cp, nil
}

// ListOrganizationalUnitsForParent returns all OUs under a parent.
func (b *InMemoryBackend) ListOrganizationalUnitsForParent(
	parentID string,
) ([]*OrganizationalUnit, error) {
	b.mu.RLock("ListOrganizationalUnitsForParent")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if !b.parentExists(parentID) {
		return nil, ErrInvalidInput
	}

	var out []*OrganizationalUnit

	for _, ou := range b.ousByParentIdx.Get(parentID) {
		cp := copyOU(ou)
		cp.Path = b.ouPathLocked(cp)
		out = append(out, cp)
	}

	slices.SortFunc(out, func(a, b *OrganizationalUnit) int { return cmp.Compare(a.Name, b.Name) })

	return out, nil
}

// ListAccountsForParent returns all accounts directly under a parent.
func (b *InMemoryBackend) ListAccountsForParent(parentID string) ([]*Account, error) {
	b.mu.RLock("ListAccountsForParent")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if !b.parentExists(parentID) {
		return nil, ErrInvalidInput
	}

	var out []*Account

	for acctID, pid := range b.accountParent {
		if pid == parentID {
			if a, ok := b.accounts.Get(acctID); ok {
				cp := copyAccount(a)
				cp.Paths = b.accountPathsLocked(acctID)
				out = append(out, cp)
			}
		}
	}

	slices.SortFunc(out, func(a, b *Account) int { return cmp.Compare(a.ID, b.ID) })

	return out, nil
}

// ListParents returns the parents of an account or OU.
func (b *InMemoryBackend) ListParents(childID string) ([]ParentSummary, error) {
	b.mu.RLock("ListParents")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	// Check if childID is an account.
	if parentID, ok := b.accountParent[childID]; ok {
		parentType := b.resolveParentType(parentID)

		return []ParentSummary{{ID: parentID, Type: parentType}}, nil
	}

	// Check if childID is an OU.
	if parentID, ok := b.ouParent[childID]; ok {
		parentType := b.resolveParentType(parentID)

		return []ParentSummary{{ID: parentID, Type: parentType}}, nil
	}

	return nil, ErrChildNotFound
}

// resolveParentType returns "ROOT" or targetTypeOU for a given parent ID.
func (b *InMemoryBackend) resolveParentType(parentID string) string {
	if b.root != nil && b.root.ID == parentID {
		return "ROOT"
	}

	return targetTypeOU
}

// ListChildren returns children of a given type under a parent.
func (b *InMemoryBackend) ListChildren(parentID, childType string) ([]ChildSummary, error) {
	b.mu.RLock("ListChildren")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if !b.parentExists(parentID) {
		return nil, ErrInvalidInput
	}

	var out []ChildSummary

	switch childType {
	case targetTypeAccount:
		for acctID := range b.accountChildrenByParent[parentID] {
			out = append(out, ChildSummary{ID: acctID, Type: targetTypeAccount})
		}
	case targetTypeOU:
		for _, ouID := range b.ousByParent[parentID] {
			out = append(out, ChildSummary{ID: ouID, Type: targetTypeOU})
		}
	default:
		return nil, ErrInvalidInput
	}

	slices.SortFunc(out, func(a, b ChildSummary) int { return cmp.Compare(a.ID, b.ID) })

	return out, nil
}

// ResolveAccountIDsUnderParent returns every account ID under parentID (an OU
// or root ID), recursing into child OUs. Used by CloudFormation to expand
// SERVICE_MANAGED StackSet deployment targets against the real OU tree.
func (b *InMemoryBackend) ResolveAccountIDsUnderParent(parentID string) ([]string, error) {
	b.mu.RLock("ResolveAccountIDsUnderParent")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if !b.parentExists(parentID) {
		return nil, ErrInvalidInput
	}

	var ids []string
	b.collectAccountIDsLocked(parentID, &ids)
	slices.Sort(ids)

	return ids, nil
}

// collectAccountIDsLocked must be called with b.mu held (read or write).
func (b *InMemoryBackend) collectAccountIDsLocked(parentID string, out *[]string) {
	for acctID, pid := range b.accountParent {
		if pid == parentID {
			*out = append(*out, acctID)
		}
	}
	for _, ouID := range b.ousByParent[parentID] {
		b.collectAccountIDsLocked(ouID, out)
	}
}

// AddOUInternal seeds an OU directly for testing.
// Requires an organization to have been created first.
func (b *InMemoryBackend) AddOUInternal(ou *OrganizationalUnit) {
	b.mu.Lock("AddOUInternal")
	defer b.mu.Unlock()

	// ou.ParentID must be finalized before Put: the ousByParentIdx secondary
	// index is populated from the value's ParentID at Put time, so mutating
	// it afterward would leave the index pointing at the wrong (empty) key.
	if ou.ParentID != "" {
		b.ouParent[ou.ID] = ou.ParentID
	} else if b.root != nil {
		b.ouParent[ou.ID] = b.root.ID
		ou.ParentID = b.root.ID
	}

	b.ous.Put(ou)
}

// copyOU returns a value copy of an OrganizationalUnit (all fields are scalars).
func copyOU(ou *OrganizationalUnit) *OrganizationalUnit {
	cp := *ou

	return &cp
}
