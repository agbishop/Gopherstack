package neptune

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) subnetGroupGet(region, name string) (*DBSubnetGroup, bool) {
	return b.subnetGroups.Get(regionKey(region, name))
}

func (b *InMemoryBackend) subnetGroupHas(region, name string) bool {
	return b.subnetGroups.Has(regionKey(region, name))
}

func (b *InMemoryBackend) subnetGroupPut(v *DBSubnetGroup) { b.subnetGroups.Put(v) }

func (b *InMemoryBackend) subnetGroupDelete(region, name string) {
	b.subnetGroups.Delete(regionKey(region, name))
}

func (b *InMemoryBackend) subnetGroupsInRegion(region string) []*DBSubnetGroup {
	return b.subnetGroupsByRegion.Get(region)
}

// cloneSubnetGroup returns a deep copy of a subnet group (with its SubnetIDs slice copied).
func cloneSubnetGroup(sg *DBSubnetGroup) DBSubnetGroup {
	cp := *sg
	cp.SubnetIDs = make([]string, len(sg.SubnetIDs))
	copy(cp.SubnetIDs, sg.SubnetIDs)

	return cp
}

// subnetGroupARN returns the region-scoped ARN for a Neptune DB subnet group.
func (b *InMemoryBackend) subnetGroupARN(region, name string) string {
	return arn.Build("rds", region, b.accountID, "subgrp:"+name)
}

// CreateDBSubnetGroup creates a new Neptune DB subnet group.
func (b *InMemoryBackend) CreateDBSubnetGroup(
	ctx context.Context,
	name, description, vpcID string,
	subnetIDs []string,
) (*DBSubnetGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: DBSubnetGroupName is required", ErrInvalidParameter)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("CreateDBSubnetGroup")
	defer b.mu.Unlock()
	if b.subnetGroupHas(region, name) {
		return nil, fmt.Errorf(
			"%w: subnet group %s already exists",
			ErrSubnetGroupAlreadyExists,
			name,
		)
	}
	ids := make([]string, len(subnetIDs))
	copy(ids, subnetIDs)
	sg := &DBSubnetGroup{
		region:                   region,
		DBSubnetGroupName:        name,
		DBSubnetGroupArn:         b.subnetGroupARN(region, name),
		DBSubnetGroupDescription: description,
		VpcID:                    vpcID,
		Status:                   "Complete",
		SubnetIDs:                ids,
	}
	b.subnetGroupPut(sg)
	cp := *sg
	cp.SubnetIDs = make([]string, len(ids))
	copy(cp.SubnetIDs, ids)

	return &cp, nil
}

// DescribeDBSubnetGroups returns all Neptune DB subnet groups or a specific one.
func (b *InMemoryBackend) DescribeDBSubnetGroups(
	ctx context.Context,
	name string,
) ([]DBSubnetGroup, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeDBSubnetGroups")
	defer b.mu.RUnlock()
	if name != "" {
		sg, exists := b.subnetGroupGet(region, name)
		if !exists {
			return nil, fmt.Errorf("%w: subnet group %s not found", ErrSubnetGroupNotFound, name)
		}

		return []DBSubnetGroup{cloneSubnetGroup(sg)}, nil
	}
	subnetGroups := b.subnetGroupsInRegion(region)
	result := make([]DBSubnetGroup, 0, len(subnetGroups))
	for _, sg := range subnetGroups {
		result = append(result, cloneSubnetGroup(sg))
	}
	slices.SortFunc(result, func(a, b DBSubnetGroup) int {
		return strings.Compare(a.DBSubnetGroupName, b.DBSubnetGroupName)
	})

	return result, nil
}

// DeleteDBSubnetGroup deletes a Neptune DB subnet group.
func (b *InMemoryBackend) DeleteDBSubnetGroup(ctx context.Context, name string) error {
	region := getRegion(ctx, b.region)
	b.mu.Lock("DeleteDBSubnetGroup")
	defer b.mu.Unlock()
	if !b.subnetGroupHas(region, name) {
		return fmt.Errorf("%w: subnet group %s not found", ErrSubnetGroupNotFound, name)
	}
	for _, c := range b.clustersInRegion(region) {
		if c.DBSubnetGroupName == name {
			return fmt.Errorf(
				"%w: subnet group %s is used by cluster %s",
				ErrSubnetGroupInUse,
				name,
				c.DBClusterIdentifier,
			)
		}
	}
	b.subnetGroupDelete(region, name)
	delete(b.tagsStore(region), b.subnetGroupARN(region, name))

	return nil
}

// ModifyDBSubnetGroup modifies a Neptune DB subnet group.
func (b *InMemoryBackend) ModifyDBSubnetGroup(
	ctx context.Context,
	name, description string,
	subnetIDs []string,
) (*DBSubnetGroup, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("ModifyDBSubnetGroup")
	defer b.mu.Unlock()
	sg, exists := b.subnetGroupGet(region, name)
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
	cp := cloneSubnetGroup(sg)

	return &cp, nil
}
