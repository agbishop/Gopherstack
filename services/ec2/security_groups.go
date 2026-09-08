package ec2

import (
	"fmt"
	"slices"
	"sort"
)

// AuthorizeSecurityGroupIngress adds ingress rules to a security group.
func (b *InMemoryBackend) AuthorizeSecurityGroupIngress(
	groupID string,
	rules []SecurityGroupRule,
) error {
	b.mu.Lock("AuthorizeSecurityGroupIngress")
	defer b.mu.Unlock()

	sg, ok := b.securityGroups.Get(groupID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSecurityGroupNotFound, groupID)
	}

	if err := validateSecurityGroupRules(sg.IngressRules, rules); err != nil {
		return err
	}

	sg.IngressRules = append(sg.IngressRules, rules...)

	return nil
}

// AuthorizeSecurityGroupEgress adds egress rules to a security group.
func (b *InMemoryBackend) AuthorizeSecurityGroupEgress(
	groupID string,
	rules []SecurityGroupRule,
) error {
	b.mu.Lock("AuthorizeSecurityGroupEgress")
	defer b.mu.Unlock()

	sg, ok := b.securityGroups.Get(groupID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSecurityGroupNotFound, groupID)
	}

	if err := validateSecurityGroupRules(sg.EgressRules, rules); err != nil {
		return err
	}

	sg.EgressRules = append(sg.EgressRules, rules...)

	return nil
}

// RevokeSecurityGroupIngress removes matching ingress rules from a security group.
// Rules that don't match anything are reported back as unknown rather than
// silently ignored, matching the real RevokeSecurityGroupIngressOutput shape
// (Return, SecurityGroupRules, UnknownIpPermissions).
func (b *InMemoryBackend) RevokeSecurityGroupIngress(
	groupID string,
	rules []SecurityGroupRule,
) ([]*SecurityGroupRuleDetail, []SecurityGroupRule, error) {
	b.mu.Lock("RevokeSecurityGroupIngress")
	defer b.mu.Unlock()

	sg, ok := b.securityGroups.Get(groupID)
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s", ErrSecurityGroupNotFound, groupID)
	}

	var revoked []*SecurityGroupRuleDetail
	var unknown []SecurityGroupRule

	for _, rule := range rules {
		idx := -1
		key := ruleKey(rule)

		for i, r := range sg.IngressRules {
			if ruleKey(r) == key {
				idx = i

				break
			}
		}

		if idx < 0 {
			unknown = append(unknown, rule)

			continue
		}

		matched := sg.IngressRules[idx]
		revoked = append(revoked, &SecurityGroupRuleDetail{
			SecurityGroupRuleID: fmt.Sprintf("sgr-%s-in-%d", groupID, idx),
			GroupID:             groupID,
			Protocol:            matched.Protocol,
			CIDRIPv4:            matched.IPRange,
			Description:         matched.Description,
			FromPort:            matched.FromPort,
			ToPort:              matched.ToPort,
			IsEgress:            false,
		})
		sg.IngressRules = removeRule(sg.IngressRules, rule)
	}

	return revoked, unknown, nil
}

// RevokeSecurityGroupEgress removes matching egress rules from a security group,
// returning the details of the rules actually revoked. Returns
// InvalidPermission.NotFound if any requested rule does not exist.
func (b *InMemoryBackend) RevokeSecurityGroupEgress(
	groupID string,
	rules []SecurityGroupRule,
) ([]*SecurityGroupRuleDetail, error) {
	b.mu.Lock("RevokeSecurityGroupEgress")
	defer b.mu.Unlock()

	sg, ok := b.securityGroups.Get(groupID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSecurityGroupNotFound, groupID)
	}

	for _, rule := range rules {
		if !ruleExists(sg.EgressRules, rule) {
			return nil, fmt.Errorf(
				"%w: rule not found in group %s",
				ErrNetworkInterfacePermissionNotFound,
				groupID,
			)
		}
	}

	revoked := make([]*SecurityGroupRuleDetail, 0, len(rules))

	for _, rule := range rules {
		key := ruleKey(rule)
		idx := -1

		for i, r := range sg.EgressRules {
			if ruleKey(r) == key {
				idx = i

				break
			}
		}

		matched := sg.EgressRules[idx]
		revoked = append(revoked, &SecurityGroupRuleDetail{
			SecurityGroupRuleID: fmt.Sprintf("sgr-%s-out-%d", groupID, idx),
			GroupID:             groupID,
			Protocol:            matched.Protocol,
			CIDRIPv4:            matched.IPRange,
			Description:         matched.Description,
			FromPort:            matched.FromPort,
			ToPort:              matched.ToPort,
			IsEgress:            true,
		})
		sg.EgressRules = removeRule(sg.EgressRules, rule)
	}

	return revoked, nil
}

// removeRule removes matching SecurityGroupRule entries from a slice. Matching
// ignores Description (see ruleKey): revoking a rule doesn't require quoting
// back whatever description it may have been given.
func removeRule(rules []SecurityGroupRule, target SecurityGroupRule) []SecurityGroupRule {
	key := ruleKey(target)
	out := rules[:0]

	for _, r := range rules {
		if ruleKey(r) != key {
			out = append(out, r)
		}
	}

	return out
}

// AssociateSecurityGroupVpc extends a security group to an additional VPC.
func (b *InMemoryBackend) AssociateSecurityGroupVpc(
	sgID, vpcID string,
) (*SGVpcAssociationState, error) {
	if sgID == "" {
		return nil, fmt.Errorf("%w: GroupId is required", ErrInvalidParameter)
	}
	if vpcID == "" {
		return nil, fmt.Errorf("%w: VpcId is required", ErrInvalidParameter)
	}

	b.mu.Lock("AssociateSecurityGroupVpc")
	defer b.mu.Unlock()

	if _, ok := b.securityGroups.Get(sgID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrSecurityGroupNotFound, sgID)
	}
	if _, ok := b.vpcs.Get(vpcID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrVPCNotFound, vpcID)
	}

	if b.sgVpcAssociations[sgID] == nil {
		b.sgVpcAssociations[sgID] = make(map[string]string)
	}
	b.sgVpcAssociations[sgID][vpcID] = stateAssociated

	return &SGVpcAssociationState{SGID: sgID, VPCID: vpcID, State: stateAssociated}, nil
}

// ---- DisassociateSecurityGroupVpc ----

// DisassociateSecurityGroupVpc removes a security group from a VPC.
func (b *InMemoryBackend) DisassociateSecurityGroupVpc(sgID, vpcID string) error {
	if sgID == "" {
		return fmt.Errorf("%w: GroupId is required", ErrInvalidParameter)
	}
	if vpcID == "" {
		return fmt.Errorf("%w: VpcId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DisassociateSecurityGroupVpc")
	defer b.mu.Unlock()

	if m, ok := b.sgVpcAssociations[sgID]; ok {
		delete(m, vpcID)

		return nil
	}

	return fmt.Errorf("%w: %s is not associated with %s", ErrInvalidParameter, sgID, vpcID)
}

// ---- DescribeSecurityGroupReferences ----

// SGReference represents a reference to a security group from another VPC.
type SGReference struct {
	GroupID                string `json:"groupID,omitempty"`
	ReferencingVPCID       string `json:"referencingVPCID,omitempty"`
	VpcPeeringConnectionID string `json:"vpcPeeringConnectionID,omitempty"`
}

// DescribeSecurityGroupReferences returns cross-VPC references for the given SG IDs.
// Returns empty for SGs not referenced externally (stub-level accuracy).
func (b *InMemoryBackend) DescribeSecurityGroupReferences(sgIDs []string) []SGReference {
	b.mu.RLock("DescribeSecurityGroupReferences")
	defer b.mu.RUnlock()

	// Return references based on sg-vpc associations as proxy for cross-VPC rules.
	var out []SGReference
	filter := make(map[string]bool, len(sgIDs))
	for _, id := range sgIDs {
		filter[id] = true
	}
	for sgID, vpcMap := range b.sgVpcAssociations {
		if len(filter) > 0 && !filter[sgID] {
			continue
		}
		for vpcID := range vpcMap {
			if sg, ok := b.securityGroups.Get(sgID); ok && sg.VPCID != vpcID {
				out = append(out, SGReference{
					GroupID:          sgID,
					ReferencingVPCID: vpcID,
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GroupID < out[j].GroupID })

	return out
}

// ---- DescribeStaleSecurityGroups ----

// StaleSGItem is a stale security group entry.
type StaleSGItem struct {
	GroupID     string `json:"groupID,omitempty"`
	GroupName   string `json:"groupName,omitempty"`
	Description string `json:"description,omitempty"`
	VPCID       string `json:"vpcid,omitempty"`
}

// findDeletedPeerVPCsLocked returns VPC IDs with terminated peering connections to vpcID.
func (b *InMemoryBackend) findDeletedPeerVPCsLocked(vpcID string) map[string]bool {
	result := make(map[string]bool)
	for _, pc := range b.vpcPeeringConnections.All() {
		if pc.State != tgwRouteStateDeleted && pc.State != "rejected" && pc.State != "failed" {
			continue
		}
		if pc.RequesterVpcID == vpcID {
			result[pc.AccepterVpcID] = true
		} else if pc.AccepterVpcID == vpcID {
			result[pc.RequesterVpcID] = true
		}
	}

	return result
}

// hasStaleRuleLocked returns true if sg has any rule referencing a group in a deleted-peer VPC.
func (b *InMemoryBackend) hasStaleRuleLocked(
	sg *SecurityGroup,
	deletedPeerVPCs map[string]bool,
) bool {
	for _, rule := range append(sg.IngressRules, sg.EgressRules...) {
		if rule.SourceGroupID == "" {
			continue
		}
		srcSG, ok := b.securityGroups.Get(rule.SourceGroupID)
		if ok && deletedPeerVPCs[srcSG.VPCID] {
			return true
		}
	}

	return false
}

// DescribeStaleSecurityGroups returns security groups in stale peering state for the VPC.
// In this implementation, stale SGs are those with dangling VPC peering references.
func (b *InMemoryBackend) DescribeStaleSecurityGroups(vpcID string) []StaleSGItem {
	b.mu.RLock("DescribeStaleSecurityGroups")
	defer b.mu.RUnlock()

	deletedPeerVPCs := b.findDeletedPeerVPCsLocked(vpcID)

	var out []StaleSGItem
	for _, sg := range b.securityGroups.All() {
		if sg.VPCID != vpcID || !b.hasStaleRuleLocked(sg, deletedPeerVPCs) {
			continue
		}
		out = append(out, StaleSGItem{
			GroupID:     sg.ID,
			GroupName:   sg.Name,
			Description: sg.Description,
			VPCID:       sg.VPCID,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GroupID < out[j].GroupID })

	return out
}

// ---- DescribeSecurityGroupVpcAssociations ----

// SGVpcAssocItem is an entry returned by DescribeSecurityGroupVpcAssociations.
type SGVpcAssocItem struct {
	SGID         string `json:"sgid,omitempty"`
	VPCID        string `json:"vpcid,omitempty"`
	State        string `json:"state,omitempty"`
	GroupOwnerID string `json:"groupOwnerID,omitempty"`
	VPCOwnerID   string `json:"vpcOwnerID,omitempty"`
}

// DescribeSecurityGroupVpcAssociations returns SG-VPC associations for the given SG IDs.
func (b *InMemoryBackend) DescribeSecurityGroupVpcAssociations(sgIDs []string) []SGVpcAssocItem {
	b.mu.RLock("DescribeSecurityGroupVpcAssociations")
	defer b.mu.RUnlock()

	filter := make(map[string]bool, len(sgIDs))
	for _, id := range sgIDs {
		filter[id] = true
	}

	var out []SGVpcAssocItem
	for sgID, vpcMap := range b.sgVpcAssociations {
		if len(filter) > 0 && !filter[sgID] {
			continue
		}
		for vpcID, state := range vpcMap {
			out = append(out, SGVpcAssocItem{
				SGID: sgID, VPCID: vpcID, State: state,
				GroupOwnerID: b.AccountID, VPCOwnerID: b.AccountID,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SGID != out[j].SGID {
			return out[i].SGID < out[j].SGID
		}

		return out[i].VPCID < out[j].VPCID
	})

	return out
}

// ---- ModifyVpcTenancy ----

// GetSecurityGroupsForVpc returns security groups associated with a VPC.
func (b *InMemoryBackend) GetSecurityGroupsForVpc(vpcID string) ([]SecurityGroupForVpcItem, error) {
	if vpcID == "" {
		return nil, fmt.Errorf("%w: VpcId is required", ErrInvalidParameter)
	}

	b.mu.RLock("GetSecurityGroupsForVpc")
	defer b.mu.RUnlock()

	var out []SecurityGroupForVpcItem
	for _, sg := range b.securityGroups.All() {
		if sg.VPCID == vpcID {
			out = append(out, SecurityGroupForVpcItem{
				GroupID:     sg.ID,
				GroupName:   sg.Name,
				Description: sg.Description,
				VPCID:       sg.VPCID,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GroupID < out[j].GroupID })

	return out, nil
}

// ---- ReplaceRoute ----

// UpdateSecurityGroupRuleDescriptionsIngress updates descriptions of ingress rules.
func (b *InMemoryBackend) UpdateSecurityGroupRuleDescriptionsIngress(
	groupID string,
	updates []SecurityGroupRule,
) error {
	if groupID == "" {
		return fmt.Errorf("%w: GroupId is required", ErrInvalidParameter)
	}

	b.mu.Lock("UpdateSecurityGroupRuleDescriptionsIngress")
	defer b.mu.Unlock()

	sg, ok := b.securityGroups.Get(groupID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSecurityGroupNotFound, groupID)
	}

	applyRuleDescriptions(sg.IngressRules, updates)

	return nil
}

// UpdateSecurityGroupRuleDescriptionsEgress updates descriptions of egress rules.
func (b *InMemoryBackend) UpdateSecurityGroupRuleDescriptionsEgress(
	groupID string,
	updates []SecurityGroupRule,
) error {
	if groupID == "" {
		return fmt.Errorf("%w: GroupId is required", ErrInvalidParameter)
	}

	b.mu.Lock("UpdateSecurityGroupRuleDescriptionsEgress")
	defer b.mu.Unlock()

	sg, ok := b.securityGroups.Get(groupID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSecurityGroupNotFound, groupID)
	}

	applyRuleDescriptions(sg.EgressRules, updates)

	return nil
}

// applyRuleDescriptions sets Description on each stored rule whose identity
// (protocol/ports/CIDR/source-group — see ruleKey) matches an incoming
// update, ignoring the incoming rule's own Description for matching purposes.
func applyRuleDescriptions(stored []SecurityGroupRule, updates []SecurityGroupRule) {
	for i := range stored {
		key := ruleKey(stored[i])

		for _, u := range updates {
			if ruleKey(u) == key {
				stored[i].Description = u.Description
			}
		}
	}
}

// ---- Volume recycle bin ----

// DescribeSecurityGroupRules returns all ingress and egress rules for the given group.
func (b *InMemoryBackend) DescribeSecurityGroupRules(
	groupID string,
) ([]*SecurityGroupRuleDetail, error) {
	if groupID == "" {
		return nil, fmt.Errorf("%w: GroupId is required", ErrInvalidParameter)
	}

	b.mu.RLock("DescribeSecurityGroupRules")
	defer b.mu.RUnlock()

	sg, ok := b.securityGroups.Get(groupID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSecurityGroupNotFound, groupID)
	}

	var out []*SecurityGroupRuleDetail

	for i, r := range sg.IngressRules {
		out = append(out, &SecurityGroupRuleDetail{
			SecurityGroupRuleID: fmt.Sprintf("sgr-%s-in-%d", groupID, i),
			GroupID:             groupID,
			Protocol:            r.Protocol,
			CIDRIPv4:            r.IPRange,
			Description:         r.Description,
			FromPort:            r.FromPort,
			ToPort:              r.ToPort,
			IsEgress:            false,
		})
	}

	for i, r := range sg.EgressRules {
		out = append(out, &SecurityGroupRuleDetail{
			SecurityGroupRuleID: fmt.Sprintf("sgr-%s-out-%d", groupID, i),
			GroupID:             groupID,
			Protocol:            r.Protocol,
			CIDRIPv4:            r.IPRange,
			Description:         r.Description,
			FromPort:            r.FromPort,
			ToPort:              r.ToPort,
			IsEgress:            true,
		})
	}

	return out, nil
}

// ModifySecurityGroupRules updates one or more rules (by position index) within a security group.
// Only protocol, IPRange, and port range can be mutated; egress/ingress direction is immutable.
func (b *InMemoryBackend) ModifySecurityGroupRules(
	groupID string,
	updates []SecurityGroupRule,
	egress bool,
) error {
	if groupID == "" {
		return fmt.Errorf("%w: GroupId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifySecurityGroupRules")
	defer b.mu.Unlock()

	sg, ok := b.securityGroups.Get(groupID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSecurityGroupNotFound, groupID)
	}

	if egress {
		sg.EgressRules = updates
	} else {
		sg.IngressRules = updates
	}

	return nil
}

// ---- Launch template delete + versions ----

// DescribeSecurityGroups returns security groups, optionally filtered by IDs.
// When ids are provided, lookups are O(len(ids)) via the security-group map
// rather than scanning every group in the backend.
func (b *InMemoryBackend) DescribeSecurityGroups(ids []string) []*SecurityGroup {
	b.mu.RLock("DescribeSecurityGroups")
	defer b.mu.RUnlock()

	if len(ids) > 0 {
		out := make([]*SecurityGroup, 0, len(ids))

		for _, id := range ids {
			sg, ok := b.securityGroups.Get(id)
			if !ok {
				continue
			}

			cp := *sg
			out = append(out, &cp)
		}

		return out
	}

	out := make([]*SecurityGroup, 0, b.securityGroups.Len())

	for _, sg := range b.securityGroups.All() {
		cp := *sg
		out = append(out, &cp)
	}

	return out
}

// CreateSecurityGroup creates a new security group and returns its ID.
func (b *InMemoryBackend) CreateSecurityGroup(
	name, description, vpcID string,
) (*SecurityGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: GroupName is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateSecurityGroup")
	defer b.mu.Unlock()

	if vpcID != "" {
		if _, ok := b.vpcs.Get(vpcID); !ok {
			return nil, fmt.Errorf("%w: %s", ErrVPCNotFound, vpcID)
		}
	}

	for _, sg := range b.securityGroups.All() {
		if sg.Name == name && sg.VPCID == vpcID {
			return nil, fmt.Errorf(
				"%w: group named %s already exists in VPC %s",
				ErrDuplicateSGName,
				name,
				vpcID,
			)
		}
	}

	id := newSecurityGroupID()
	sg := &SecurityGroup{
		ID:          id,
		Name:        name,
		Description: description,
		VPCID:       vpcID,
		ARN:         "arn:aws:ec2:" + b.Region + ":" + b.AccountID + ":security-group/" + id,
		// Real AWS creates new security groups with a default allow-all egress rule.
		EgressRules: []SecurityGroupRule{
			{Protocol: "-1", IPRange: cidrAllIPv4},
		},
	}
	b.securityGroups.Put(sg)
	b.indexSGLocked(id, vpcID)

	return sg, nil
}

// securityGroupDependencyViolationLocked returns a DependencyViolation error
// if sg is still attached to an instance or referenced by another security
// group's rules in the same VPC. Must be called with b.mu held.
func (b *InMemoryBackend) securityGroupDependencyViolationLocked(sg *SecurityGroup) error {
	for _, inst := range b.instances.All() {
		if inst.State == StateTerminated {
			continue
		}

		if slices.Contains(inst.SecurityGroups, sg.ID) {
			return fmt.Errorf(
				"%w: the security group %s has dependencies (instance %s) and cannot be deleted",
				ErrDependencyViolation, sg.ID, inst.ID,
			)
		}
	}

	for _, other := range b.securityGroups.All() {
		if other.ID == sg.ID || other.VPCID != sg.VPCID {
			continue
		}

		if securityGroupRulesReference(other.IngressRules, sg.ID) ||
			securityGroupRulesReference(other.EgressRules, sg.ID) {
			return fmt.Errorf(
				"%w: the security group %s has dependencies (security group %s) and cannot be deleted",
				ErrDependencyViolation, sg.ID, other.ID,
			)
		}
	}

	return nil
}

func securityGroupRulesReference(rules []SecurityGroupRule, sgID string) bool {
	for _, rule := range rules {
		if rule.SourceGroupID == sgID {
			return true
		}
	}

	return false
}

// DeleteSecurityGroup removes a security group by ID.
func (b *InMemoryBackend) DeleteSecurityGroup(id string) error {
	b.mu.Lock("DeleteSecurityGroup")
	defer b.mu.Unlock()

	sg, ok := b.securityGroups.Get(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSecurityGroupNotFound, id)
	}

	if err := b.securityGroupDependencyViolationLocked(sg); err != nil {
		return err
	}

	b.deindexSGLocked(id, sg.VPCID)
	b.securityGroups.Delete(id)
	delete(b.tags, id)
	delete(b.sgVpcAssociations, id)

	return nil
}
