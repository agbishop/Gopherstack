package rds

import (
	"fmt"
	"slices"
)

// AuthorizeDBSecurityGroupIngress authorizes CIDR IP access to a DB security group.
// If the group does not exist it is created automatically (matching AWS behaviour for legacy VPC-less accounts).
func (b *InMemoryBackend) AuthorizeDBSecurityGroupIngress(
	groupName, cidrIP string,
) (*DBSecurityGroup, error) {
	if groupName == "" {
		return nil, fmt.Errorf("%w: DBSecurityGroupName must not be empty", ErrInvalidParameter)
	}
	if cidrIP == "" {
		return nil, fmt.Errorf("%w: CIDRIP must not be empty", ErrInvalidParameter)
	}

	b.mu.Lock("AuthorizeDBSecurityGroupIngress")
	defer b.mu.Unlock()

	sg, exists := b.dbSecurityGroups.Get(groupName)
	if !exists {
		sg = &DBSecurityGroup{
			DBSecurityGroupName: groupName,
			IPRanges:            []IPRange{},
		}
		b.dbSecurityGroups.Put(sg)
	}

	for _, r := range sg.IPRanges {
		if r.CIDRIP == cidrIP {
			cp := *sg
			cp.IPRanges = make([]IPRange, len(sg.IPRanges))
			copy(cp.IPRanges, sg.IPRanges)

			return &cp, nil
		}
	}

	sg.IPRanges = append(sg.IPRanges, IPRange{CIDRIP: cidrIP, Status: ipRangeStatusAuthorized})

	cp := *sg
	cp.IPRanges = make([]IPRange, len(sg.IPRanges))
	copy(cp.IPRanges, sg.IPRanges)

	return &cp, nil
}

// DescribeDBSecurityGroups returns security groups, optionally by name.
func (b *InMemoryBackend) DescribeDBSecurityGroups(name string) ([]DBSecurityGroup, error) {
	b.mu.RLock("DescribeDBSecurityGroups")
	defer b.mu.RUnlock()
	if name != "" {
		sg, exists := b.dbSecurityGroups.Get(name)
		if !exists {
			return nil, fmt.Errorf("%w: security group %s not found", ErrDBSecurityGroupNotFound, name)
		}
		cp := *sg

		return []DBSecurityGroup{cp}, nil
	}
	result := make([]DBSecurityGroup, 0, b.dbSecurityGroups.Len())
	for _, sg := range b.dbSecurityGroups.All() {
		result = append(result, *sg)
	}

	return result, nil
}

// defaultDBSecurityGroupName is the reserved name of the DB security group
// every account has by default. Real AWS: DeleteDBSecurityGroupInput.
// DBSecurityGroupName's own doc comment, "You can't delete the default DB
// security group".
const defaultDBSecurityGroupName = "Default"

// DeleteDBSecurityGroup removes the named security group.
func (b *InMemoryBackend) DeleteDBSecurityGroup(name string) error {
	if name == "" {
		return fmt.Errorf("%w: DBSecurityGroupName is required", ErrInvalidParameter)
	}

	if name == defaultDBSecurityGroupName {
		return fmt.Errorf("%w: cannot delete the default DB security group", ErrDBSecurityGroupInvalidState)
	}

	b.mu.Lock("DeleteDBSecurityGroup")
	defer b.mu.Unlock()
	if _, exists := b.dbSecurityGroups.Get(name); !exists {
		return fmt.Errorf("%w: security group %s not found", ErrDBSecurityGroupNotFound, name)
	}
	for _, inst := range b.instances.All() {
		for _, sg := range inst.DBSecurityGroups {
			if sg.DBSecurityGroupName == name {
				return fmt.Errorf(
					"%w: security group %s is associated with DB instance %s",
					ErrDBSecurityGroupInvalidState, name, inst.DBInstanceIdentifier,
				)
			}
		}
	}
	b.dbSecurityGroups.Delete(name)
	delete(b.tags, b.rdsARN("secgrp", name))

	return nil
}

// RevokeDBSecurityGroupIngress revokes a CIDR IP from a security group.
func (b *InMemoryBackend) RevokeDBSecurityGroupIngress(groupName, cidrIP string) (*DBSecurityGroup, error) {
	if groupName == "" || cidrIP == "" {
		return nil, fmt.Errorf("%w: DBSecurityGroupName and CIDRIP are required", ErrInvalidParameter)
	}
	b.mu.Lock("RevokeDBSecurityGroupIngress")
	defer b.mu.Unlock()
	sg, exists := b.dbSecurityGroups.Get(groupName)
	if !exists {
		return nil, fmt.Errorf("%w: security group %s not found", ErrDBSecurityGroupNotFound, groupName)
	}
	idx := -1
	for i, r := range sg.IPRanges {
		if r.CIDRIP == cidrIP {
			idx = i

			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: CIDR %s not found in security group %s", ErrInvalidParameter, cidrIP, groupName)
	}
	sg.IPRanges = slices.Delete(sg.IPRanges, idx, idx+1)
	cp := *sg

	return &cp, nil
}

// CreateDBSecurityGroup creates a new DB security group.
func (b *InMemoryBackend) CreateDBSecurityGroup(name, description string) (*DBSecurityGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidParameter)
	}
	b.mu.Lock("CreateDBSecurityGroup")
	defer b.mu.Unlock()
	if _, exists := b.dbSecurityGroups.Get(name); exists {
		return nil, fmt.Errorf("%w: %s", ErrDBSecurityGroupAlreadyExists, name)
	}
	sg := &DBSecurityGroup{
		DBSecurityGroupName:        name,
		DBSecurityGroupDescription: description,
		DBSecurityGroupArn:         b.rdsARN("secgrp", name),
	}
	b.dbSecurityGroups.Put(sg)
	cp := *sg

	return &cp, nil
}
