package route53resolver

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) firewallRuleGroupPoliciesStore(region string) map[string]string {
	if b.firewallRuleGroupPolicies[region] == nil {
		b.firewallRuleGroupPolicies[region] = make(map[string]string)
	}

	return b.firewallRuleGroupPolicies[region]
}

// firewallRuleGroupPoliciesStoreRO returns the region-scoped
// firewallRuleGroupPolicies map for region without mutating the outer map.
// Safe to call while holding only b.mu.RLock(): if the region has not been
// observed yet, it returns a fresh, unregistered, empty map instead of
// lazily creating (and persisting) an entry.
func (b *InMemoryBackend) firewallRuleGroupPoliciesStoreRO(region string) map[string]string {
	if v := b.firewallRuleGroupPolicies[region]; v != nil {
		return v
	}

	return map[string]string{}
}

// CreateFirewallRuleGroup creates a new DNS Firewall rule group.
func (b *InMemoryBackend) CreateFirewallRuleGroup(
	ctx context.Context,
	name, creatorRequestID string,
) (*FirewallRuleGroup, error) {
	b.mu.Lock("CreateFirewallRuleGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	now := currentTime()
	id := "rslvr-frg-" + uuid.New().String()[:8]
	groupARN := arn.Build("route53resolver", region, b.accountID, "firewall-rule-group/"+id)
	g := &FirewallRuleGroup{
		ID:               id,
		ARN:              groupARN,
		Name:             name,
		CreatorRequestID: creatorRequestID,
		Status:           statusComplete,
		OwnerID:          b.accountID,
		ShareStatus:      shareStatusNotShared,
		CreationTime:     now,
		ModificationTime: now,
		Region:           region,
	}
	b.firewallRuleGroups.Put(g)
	cp := *g

	return &cp, nil
}

// AssociateFirewallRuleGroup associates a FirewallRuleGroup with a VPC.
func (b *InMemoryBackend) AssociateFirewallRuleGroup(
	ctx context.Context,
	firewallRuleGroupID, vpcID, name, creatorRequestID, mutationProtection string,
	priority int32,
) (*FirewallRuleGroupAssociation, error) {
	b.mu.Lock("AssociateFirewallRuleGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if !b.firewallRuleGroups.Has(regionalKey(region, firewallRuleGroupID)) {
		return nil, fmt.Errorf(
			"%w: firewall rule group %s not found",
			ErrNotFound,
			firewallRuleGroupID,
		)
	}

	if mutationProtection == "" {
		mutationProtection = mutationProtectionDisabled
	}

	now := currentTime()
	id := "rslvr-frgassoc-" + uuid.New().String()[:8]
	assocARN := arn.Build(
		"route53resolver",
		region,
		b.accountID,
		"firewall-rule-group-association/"+id,
	)
	assoc := &FirewallRuleGroupAssociation{
		ID:                  id,
		ARN:                 assocARN,
		Name:                name,
		FirewallRuleGroupID: firewallRuleGroupID,
		VpcID:               vpcID,
		Priority:            priority,
		Status:              statusComplete,
		MutationProtection:  mutationProtection,
		CreatorRequestID:    creatorRequestID,
		CreationTime:        now,
		ModificationTime:    now,
		Region:              region,
	}
	b.firewallRuleGroupAssociations.Put(assoc)
	cp := *assoc

	return &cp, nil
}

// AddFirewallRuleGroupInternal adds a firewall rule group directly to the backend (test seed helper).
func (b *InMemoryBackend) AddFirewallRuleGroupInternal(name string) *FirewallRuleGroup {
	b.mu.Lock("AddFirewallRuleGroupInternal")
	defer b.mu.Unlock()

	id := "rslvr-frg-" + uuid.New().String()[:8]
	groupARN := arn.Build("route53resolver", b.region, b.accountID, "firewall-rule-group/"+id)
	g := &FirewallRuleGroup{
		ID:      id,
		ARN:     groupARN,
		Name:    name,
		Status:  statusComplete,
		OwnerID: b.accountID,
		Region:  b.region,
	}
	b.firewallRuleGroups.Put(g)
	cp := *g

	return &cp
}

// DeleteFirewallRuleGroup deletes a firewall rule group and cascades to its rules and associations.
func (b *InMemoryBackend) DeleteFirewallRuleGroup(ctx context.Context, id string) (*FirewallRuleGroup, error) {
	b.mu.Lock("DeleteFirewallRuleGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	grp, ok := b.firewallRuleGroups.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf("%w: firewall rule group %s not found", ErrNotFound, id)
	}
	cp := *grp

	// Clean up tags.
	delete(b.tagsStore(region), grp.ARN)
	// Clean up the resource policy. Direct map access (not the lazy-creating
	// Store helper) so deleting a group whose region never had a policy set
	// doesn't leave behind an empty region entry.
	delete(b.firewallRuleGroupPolicies[region], grp.ARN)

	// Cascade: delete rules belonging to this group. slices.Clone before
	// deleting in the loop -- see DeleteResolverEndpoint's comment.
	for _, rule := range slices.Clone(b.firewallRulesByRegion.Get(region)) {
		if rule.FirewallRuleGroupID == id {
			b.firewallRules.Delete(regionalKey(region, rule.ID))
		}
	}
	// Cascade: delete associations for this group.
	for _, assoc := range slices.Clone(b.firewallRuleGroupAssociationsByRegion.Get(region)) {
		if assoc.FirewallRuleGroupID == id {
			b.firewallRuleGroupAssociations.Delete(regionalKey(region, assoc.ID))
		}
	}
	b.firewallRuleGroups.Delete(regionalKey(region, id))

	return &cp, nil
}

// GetFirewallRuleGroup retrieves a firewall rule group by ID.
func (b *InMemoryBackend) GetFirewallRuleGroup(ctx context.Context, id string) (*FirewallRuleGroup, error) {
	b.mu.RLock("GetFirewallRuleGroup")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	grp, ok := b.firewallRuleGroups.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf("%w: firewall rule group %s not found", ErrNotFound, id)
	}
	cp := *grp

	return &cp, nil
}

// ListFirewallRuleGroups lists all firewall rule groups.
func (b *InMemoryBackend) ListFirewallRuleGroups(ctx context.Context) []*FirewallRuleGroup {
	b.mu.RLock("ListFirewallRuleGroups")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	regionGroups := b.firewallRuleGroupsByRegion.Get(region)
	list := make([]*FirewallRuleGroup, 0, len(regionGroups))
	for _, g := range regionGroups {
		cp := *g
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return list
}

// GetFirewallRuleGroupPolicy retrieves the resource policy for a firewall rule group ARN.
func (b *InMemoryBackend) GetFirewallRuleGroupPolicy(ctx context.Context, arnStr string) string {
	b.mu.RLock("GetFirewallRuleGroupPolicy")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	return b.firewallRuleGroupPoliciesStoreRO(region)[arnStr]
}

// PutFirewallRuleGroupPolicy stores a resource policy for a firewall rule group ARN.
func (b *InMemoryBackend) PutFirewallRuleGroupPolicy(ctx context.Context, arnStr, policy string) error {
	b.mu.Lock("PutFirewallRuleGroupPolicy")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	b.firewallRuleGroupPoliciesStore(region)[arnStr] = policy

	return nil
}

// --- Firewall Rule Group Association operations ---

// GetFirewallRuleGroupAssociation retrieves an association by ID.
func (b *InMemoryBackend) GetFirewallRuleGroupAssociation(
	ctx context.Context,
	id string,
) (*FirewallRuleGroupAssociation, error) {
	b.mu.RLock("GetFirewallRuleGroupAssociation")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	assoc, ok := b.firewallRuleGroupAssociations.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf("%w: firewall rule group association %s not found", ErrNotFound, id)
	}
	cp := *assoc

	return &cp, nil
}

// ListFirewallRuleGroupAssociations lists associations, optionally filtered by VPC or group.
func (b *InMemoryBackend) ListFirewallRuleGroupAssociations(
	ctx context.Context,
	vpcID, firewallRuleGroupID string,
) []*FirewallRuleGroupAssociation {
	b.mu.RLock("ListFirewallRuleGroupAssociations")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	regionAssocs := b.firewallRuleGroupAssociationsByRegion.Get(region)
	list := make([]*FirewallRuleGroupAssociation, 0, len(regionAssocs))
	for _, a := range regionAssocs {
		if vpcID != "" && a.VpcID != vpcID {
			continue
		}
		if firewallRuleGroupID != "" && a.FirewallRuleGroupID != firewallRuleGroupID {
			continue
		}
		cp := *a
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Priority < list[j].Priority })

	return list
}

// DisassociateFirewallRuleGroup removes a firewall rule group association.
func (b *InMemoryBackend) DisassociateFirewallRuleGroup(
	ctx context.Context,
	id string,
) (*FirewallRuleGroupAssociation, error) {
	b.mu.Lock("DisassociateFirewallRuleGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	assoc, ok := b.firewallRuleGroupAssociations.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf("%w: firewall rule group association %s not found", ErrNotFound, id)
	}

	if assoc.MutationProtection == mutationProtectionEnabled {
		return nil, fmt.Errorf(
			"%w: cannot disassociate: MutationProtection is ENABLED",
			ErrBatchValidation,
		)
	}

	cp := *assoc
	b.firewallRuleGroupAssociations.Delete(regionalKey(region, id))

	return &cp, nil
}

// UpdateFirewallRuleGroupAssociation updates name, priority, or mutation protection of an association.
func (b *InMemoryBackend) UpdateFirewallRuleGroupAssociation(
	ctx context.Context,
	id, name, mutationProtection string,
	priority int32,
) (*FirewallRuleGroupAssociation, error) {
	b.mu.Lock("UpdateFirewallRuleGroupAssociation")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	assoc, ok := b.firewallRuleGroupAssociations.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf("%w: firewall rule group association %s not found", ErrNotFound, id)
	}
	if mutationProtection != "" &&
		mutationProtection != mutationProtectionEnabled &&
		mutationProtection != mutationProtectionDisabled {
		return nil, fmt.Errorf(
			"%w: MutationProtection must be ENABLED or DISABLED",
			ErrBatchValidation,
		)
	}
	if name != "" {
		assoc.Name = name
	}
	if priority != 0 {
		assoc.Priority = priority
	}
	if mutationProtection != "" {
		assoc.MutationProtection = mutationProtection
	}
	assoc.ModificationTime = currentTime()
	cp := *assoc

	return &cp, nil
}

// --- Firewall Domain List operations ---
