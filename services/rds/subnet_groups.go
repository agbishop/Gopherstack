package rds

import (
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/strs"
)

// CreateDBSubnetGroup creates a DB subnet group.
func (b *InMemoryBackend) CreateDBSubnetGroup(
	name, description, vpcID string,
	subnetIDs []string,
) (*DBSubnetGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: DBSubnetGroupName must not be empty", ErrInvalidParameter)
	}

	b.mu.Lock("CreateDBSubnetGroup")
	defer b.mu.Unlock()

	if _, exists := b.subnetGroups.Get(name); exists {
		return nil, fmt.Errorf("%w: subnet group %s already exists", ErrSubnetGroupAlreadyExists, name)
	}

	ids := make([]string, len(subnetIDs))
	copy(ids, subnetIDs)

	sg := &DBSubnetGroup{
		DBSubnetGroupName:        name,
		DBSubnetGroupDescription: description,
		DBSubnetGroupArn:         b.rdsARN("subgrp", name),
		VpcID:                    vpcID,
		SubnetIDs:                ids,
		Status:                   "Complete",
	}
	b.subnetGroups.Put(sg)

	cp := *sg
	cp.SubnetIDs = make([]string, len(ids))
	copy(cp.SubnetIDs, ids)

	return &cp, nil
}

// DescribeDBSubnetGroups returns subnet groups. If name is non-empty, returns only that group.
func (b *InMemoryBackend) DescribeDBSubnetGroups(name string) ([]DBSubnetGroup, error) {
	b.mu.RLock("DescribeDBSubnetGroups")
	defer b.mu.RUnlock()

	if name != "" {
		sg, exists := b.subnetGroups.Get(name)
		if !exists {
			return nil, fmt.Errorf("%w: subnet group %s not found", ErrSubnetGroupNotFound, name)
		}

		cp := *sg
		cp.SubnetIDs = make([]string, len(sg.SubnetIDs))
		copy(cp.SubnetIDs, sg.SubnetIDs)

		return []DBSubnetGroup{cp}, nil
	}

	sgs := make([]DBSubnetGroup, 0, b.subnetGroups.Len())

	for _, sg := range b.subnetGroups.All() {
		cp := *sg
		cp.SubnetIDs = make([]string, len(sg.SubnetIDs))
		copy(cp.SubnetIDs, sg.SubnetIDs)
		sgs = append(sgs, cp)
	}

	return sgs, nil
}

// DeleteDBSubnetGroup removes the given subnet group.
func (b *InMemoryBackend) DeleteDBSubnetGroup(name string) error {
	b.mu.Lock("DeleteDBSubnetGroup")
	defer b.mu.Unlock()

	if _, exists := b.subnetGroups.Get(name); !exists {
		return fmt.Errorf("%w: subnet group %s not found", ErrSubnetGroupNotFound, name)
	}

	for _, inst := range b.instances.All() {
		if strs.Equal(inst.DBSubnetGroupName, name) {
			return fmt.Errorf("%w: subnet group %s is in use by DB instance %s",
				ErrSubnetGroupInUse, name, inst.DBInstanceIdentifier)
		}
	}

	b.subnetGroups.Delete(name)
	delete(b.tags, b.rdsARN("subgrp", name))

	return nil
}

// ModifyDBSubnetGroup modifies an existing DB subnet group.
func (b *InMemoryBackend) ModifyDBSubnetGroup(name, description string, subnetIDs []string) (*DBSubnetGroup, error) {
	b.mu.Lock("ModifyDBSubnetGroup")
	defer b.mu.Unlock()
	sg, exists := b.subnetGroups.Get(name)
	if !exists {
		return nil, fmt.Errorf("%w: subnet group %s not found", ErrSubnetGroupNotFound, name)
	}
	if description != "" {
		sg.DBSubnetGroupDescription = description
	}
	if len(subnetIDs) > 0 {
		ids := make([]string, len(subnetIDs))
		copy(ids, subnetIDs)
		sg.SubnetIDs = ids
	}
	cp := *sg

	return &cp, nil
}
