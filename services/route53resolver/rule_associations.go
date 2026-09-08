package route53resolver

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// AssociateResolverRule associates a resolver rule with a VPC.
func (b *InMemoryBackend) AssociateResolverRule(
	ctx context.Context,
	resolverRuleID, vpcID, name string,
) (*ResolverRuleAssociation, error) {
	b.mu.Lock("AssociateResolverRule")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if !b.rules.Has(regionalKey(region, resolverRuleID)) {
		return nil, fmt.Errorf("%w: resolver rule %s not found", ErrNotFound, resolverRuleID)
	}

	// AssociateResolverRule models ResourceExistsException ("The resource that
	// you tried to create already exists" -- API_route53resolver_
	// AssociateResolverRule.html); DisassociateResolverRule already treats
	// (ResolverRuleId, VPCId) as this association's identity, so re-associating
	// the same pair is the duplicate this error covers.
	for _, assoc := range b.ruleAssociationsByRegion.Get(region) {
		if assoc.ResolverRuleID == resolverRuleID && assoc.VPCID == vpcID {
			return nil, fmt.Errorf(
				"%w: resolver rule %s is already associated with VPC %s",
				ErrAlreadyExists,
				resolverRuleID,
				vpcID,
			)
		}
	}

	id := "rslvr-rrassoc-" + uuid.New().String()[:8]
	assoc := &ResolverRuleAssociation{
		ID:             id,
		Name:           name,
		ResolverRuleID: resolverRuleID,
		VPCID:          vpcID,
		Status:         statusComplete,
		Region:         region,
	}
	b.ruleAssociations.Put(assoc)
	cp := *assoc

	return &cp, nil
}

// GetResolverRuleAssociation retrieves a rule association by ID.
func (b *InMemoryBackend) GetResolverRuleAssociation(ctx context.Context, id string) (*ResolverRuleAssociation, error) {
	b.mu.RLock("GetResolverRuleAssociation")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	assoc, ok := b.ruleAssociations.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf("%w: resolver rule association %s not found", ErrNotFound, id)
	}
	cp := *assoc

	return &cp, nil
}

// DisassociateResolverRule removes a resolver rule association, looked up by
// the (resolverRuleID, vpcID) pair -- this matches the real
// DisassociateResolverRule API, which takes ResolverRuleId + VPCId (not an
// opaque association ID; that only appears in Get/List responses).
func (b *InMemoryBackend) DisassociateResolverRule(
	ctx context.Context,
	resolverRuleID, vpcID string,
) (*ResolverRuleAssociation, error) {
	b.mu.Lock("DisassociateResolverRule")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	for _, assoc := range b.ruleAssociationsByRegion.Get(region) {
		if assoc.ResolverRuleID != resolverRuleID || assoc.VPCID != vpcID {
			continue
		}

		cp := *assoc
		b.ruleAssociations.Delete(regionalKey(region, assoc.ID))

		return &cp, nil
	}

	return nil, fmt.Errorf(
		"%w: resolver rule association for rule %s and VPC %s not found",
		ErrNotFound,
		resolverRuleID,
		vpcID,
	)
}

// ListResolverRuleAssociations lists all resolver rule associations.
func (b *InMemoryBackend) ListResolverRuleAssociations(ctx context.Context) []*ResolverRuleAssociation {
	b.mu.RLock("ListResolverRuleAssociations")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	regionAssocs := b.ruleAssociationsByRegion.Get(region)
	list := make([]*ResolverRuleAssociation, 0, len(regionAssocs))
	for _, a := range regionAssocs {
		cp := *a
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	return list
}
