package ec2

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// CreateNetworkACL creates a non-default network ACL in a VPC.
func (b *InMemoryBackend) CreateNetworkACL(vpcID string) (*StoredNetworkACL, error) {
	if vpcID == "" {
		return nil, fmt.Errorf("%w: VpcId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateNetworkACL")
	defer b.mu.Unlock()

	if _, ok := b.vpcs.Get(vpcID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrVPCNotFound, vpcID)
	}

	acl := &StoredNetworkACL{
		ID:             newNetworkACLID(),
		VPCID:          vpcID,
		IsDefault:      false,
		AssociationIDs: nil,
		Entries: []NACLEntry{
			// AWS default: deny all inbound and outbound
			{
				RuleNumber: naclDefaultDenyRuleNumber, Protocol: "-1",
				CIDRBlock: cidrAllIPv4, RuleAction: "deny", Egress: false,
			},
			{
				RuleNumber: naclDefaultDenyRuleNumber, Protocol: "-1",
				CIDRBlock: cidrAllIPv4, RuleAction: "deny", Egress: true,
			},
		},
	}
	b.networkACLs.Put(acl)

	cp := *acl
	cp.Entries = append([]NACLEntry(nil), acl.Entries...)

	return &cp, nil
}

// DeleteNetworkACL removes a non-default network ACL.
func (b *InMemoryBackend) DeleteNetworkACL(id string) error {
	if id == "" {
		return fmt.Errorf("%w: NetworkAclId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteNetworkACL")
	defer b.mu.Unlock()

	acl, ok := b.networkACLs.Get(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNetworkACLNotFound, id)
	}

	if acl.IsDefault {
		return fmt.Errorf("%w: cannot delete default network ACL", ErrInvalidParameter)
	}

	if len(acl.AssociationIDs) > 0 {
		return fmt.Errorf(
			"%w: the network acl %s has dependencies (subnet %s) and cannot be deleted",
			ErrDependencyViolation, id, acl.AssociationIDs[0],
		)
	}

	b.networkACLs.Delete(id)
	delete(b.tags, id)

	return nil
}

// CreateNetworkACLEntry adds a rule to an existing network ACL.
func (b *InMemoryBackend) CreateNetworkACLEntry(
	aclID string, ruleNumber int, protocol, action, cidr string,
	egress bool, fromPort, toPort int,
) error {
	if aclID == "" {
		return fmt.Errorf("%w: NetworkAclId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateNetworkACLEntry")
	defer b.mu.Unlock()

	acl, ok := b.networkACLs.Get(aclID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNetworkACLNotFound, aclID)
	}

	// check duplicate rule number + direction
	for _, e := range acl.Entries {
		if e.RuleNumber == ruleNumber && e.Egress == egress {
			return fmt.Errorf(
				"%w: rule number %d already exists for egress=%v",
				ErrInvalidParameter, ruleNumber, egress,
			)
		}
	}

	acl.Entries = append(acl.Entries, NACLEntry{
		RuleNumber: ruleNumber,
		Protocol:   protocol,
		RuleAction: action,
		CIDRBlock:  cidr,
		Egress:     egress,
		FromPort:   fromPort,
		ToPort:     toPort,
	})

	return nil
}

// DeleteNetworkACLEntry removes a rule from a network ACL.
func (b *InMemoryBackend) DeleteNetworkACLEntry(aclID string, ruleNumber int, egress bool) error {
	if aclID == "" {
		return fmt.Errorf("%w: NetworkAclId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteNetworkACLEntry")
	defer b.mu.Unlock()

	acl, ok := b.networkACLs.Get(aclID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNetworkACLNotFound, aclID)
	}

	filtered := acl.Entries[:0]
	found := false

	for _, e := range acl.Entries {
		if e.RuleNumber == ruleNumber && e.Egress == egress {
			found = true

			continue
		}

		filtered = append(filtered, e)
	}

	if !found {
		return fmt.Errorf(
			"%w: rule %d (egress=%v) not found",
			ErrInvalidParameter,
			ruleNumber,
			egress,
		)
	}

	acl.Entries = filtered

	return nil
}

// DescribeStoredNetworkAcls returns explicitly created network ACLs, filtered by VPC IDs.
func (b *InMemoryBackend) DescribeStoredNetworkAcls(ids []string) []*StoredNetworkACL {
	b.mu.RLock("DescribeStoredNetworkAcls")
	defer b.mu.RUnlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	out := make([]*StoredNetworkACL, 0, b.networkACLs.Len())
	for _, acl := range b.networkACLs.All() {
		if len(idSet) > 0 && !idSet[acl.ID] {
			continue
		}

		cp := *acl
		cp.Entries = append([]NACLEntry(nil), acl.Entries...)
		cp.AssociationIDs = append([]string(nil), acl.AssociationIDs...)
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

// ---- Security group rules ----

// DescribeNetworkAclsFiltered returns ACLs from both default (auto-generated) and
// stored (CreateNetworkAcl) collections, with optional VPC-ID filter.
func (b *InMemoryBackend) DescribeNetworkAclsFiltered(vpcIDs []string) []*NetworkACL {
	// default ACLs
	defaultACLs := b.DescribeNetworkAcls(vpcIDs)

	// stored ACLs
	b.mu.RLock("DescribeNetworkAclsFiltered")
	defer b.mu.RUnlock()

	allowed := make(map[string]bool, len(vpcIDs))
	for _, id := range vpcIDs {
		allowed[id] = true
	}

	for _, acl := range b.networkACLs.All() {
		if len(allowed) > 0 && !allowed[acl.VPCID] {
			continue
		}

		assocIDs := append([]string(nil), acl.AssociationIDs...)
		entries := append([]NACLEntry(nil), acl.Entries...)
		defaultACLs = append(defaultACLs, &NetworkACL{
			ID:             acl.ID,
			VPCID:          acl.VPCID,
			IsDefault:      acl.IsDefault,
			AssociationIDs: assocIDs,
			Entries:        entries,
		})
	}

	sort.Slice(defaultACLs, func(i, j int) bool {
		return defaultACLs[i].ID < defaultACLs[j].ID
	})

	return defaultACLs
}

// ReplaceNetworkACLEntry replaces an existing NACL rule identified by
// (aclID, ruleNumber, egress) with new parameters.
func (b *InMemoryBackend) ReplaceNetworkACLEntry(
	aclID string, ruleNumber int, protocol, action, cidr string,
	egress bool, fromPort, toPort int,
) error {
	if aclID == "" {
		return fmt.Errorf("%w: NetworkAclId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ReplaceNetworkACLEntry")
	defer b.mu.Unlock()

	acl, ok := b.networkACLs.Get(aclID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNetworkACLNotFound, aclID)
	}

	for i, e := range acl.Entries {
		if e.RuleNumber == ruleNumber && e.Egress == egress {
			acl.Entries[i] = NACLEntry{
				RuleNumber: ruleNumber,
				Protocol:   protocol,
				RuleAction: action,
				CIDRBlock:  cidr,
				Egress:     egress,
				FromPort:   fromPort,
				ToPort:     toPort,
			}

			return nil
		}
	}

	return fmt.Errorf(
		"%w: rule %d (egress=%v) not found in ACL %s",
		ErrInvalidParameter, ruleNumber, egress, aclID,
	)
}

// ---- Network ACL: Association management ----

// ReplaceNetworkACLAssociation moves a subnet from its current NACL to
// the specified one and returns a new association ID.
func (b *InMemoryBackend) ReplaceNetworkACLAssociation(aclID, subnetID string) (string, error) {
	if aclID == "" {
		return "", fmt.Errorf("%w: NetworkAclId is required", ErrInvalidParameter)
	}

	if subnetID == "" {
		return "", fmt.Errorf("%w: SubnetId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ReplaceNetworkACLAssociation")
	defer b.mu.Unlock()

	if _, ok := b.networkACLs.Get(aclID); !ok {
		return "", fmt.Errorf("%w: %s", ErrNetworkACLNotFound, aclID)
	}

	if _, ok := b.subnets.Get(subnetID); !ok {
		return "", fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
	}

	// Remove subnetID from any existing ACL.
	for _, existing := range b.networkACLs.All() {
		for i, assoc := range existing.AssociationIDs {
			if assoc == subnetID {
				existing.AssociationIDs = append(
					existing.AssociationIDs[:i],
					existing.AssociationIDs[i+1:]...,
				)

				break
			}
		}
	}

	target, _ := b.networkACLs.Get(aclID)
	target.AssociationIDs = append(target.AssociationIDs, subnetID)
	newAssocID := "aclassoc-" + uuid.New().String()[:8]

	return newAssocID, nil
}

// ---- VPC Endpoint Services ----
