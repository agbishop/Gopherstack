package elasticache

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// maxCacheSubnetGroupsPerRegion and maxSubnetsPerCacheSubnetGroup are AWS's
// documented default quotas ("Subnet groups per Region" / "Subnets per
// subnet group", docs.aws.amazon.com/AmazonElastiCache/latest/dg/
// quota-limits.html).
const (
	maxCacheSubnetGroupsPerRegion = 300
	maxSubnetsPerCacheSubnetGroup = 20
)

func (b *InMemoryBackend) subnetGroupARN(region, name string) string {
	return arn.Build("elasticache", region, b.accountID, "subnetgroup:"+name)
}

// CreateSubnetGroup creates a new cache subnet group.
func (b *InMemoryBackend) CreateSubnetGroup(
	ctx context.Context,
	name, description string,
	subnetIDs []string,
) (*CacheSubnetGroup, error) {
	b.mu.Lock("CreateSubnetGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	tbl := b.subnetGroupsStore(region)
	if _, exists := tbl.Get(name); exists {
		return nil, ErrSubnetGroupAlreadyExists
	}

	if len(subnetIDs) > maxSubnetsPerCacheSubnetGroup {
		return nil, ErrCacheSubnetQuotaExceeded
	}

	if tbl.Len() >= maxCacheSubnetGroupsPerRegion {
		return nil, ErrCacheSubnetGroupQuotaExceeded
	}

	sg := &CacheSubnetGroup{
		Name:        name,
		Description: description,
		SubnetIDs:   subnetIDs,
		ARN:         b.subnetGroupARN(region, name),
		Tags:        tags.New("elasticache.sg." + name + ".tags"),
	}
	tbl.Put(sg)

	return sg, nil
}

// DeleteSubnetGroup removes a cache subnet group.
func (b *InMemoryBackend) DeleteSubnetGroup(ctx context.Context, name string) error {
	b.mu.Lock("DeleteSubnetGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	tbl := b.subnetGroupsStore(region)
	sg, exists := tbl.Get(name)
	if !exists {
		return ErrSubnetGroupNotFound
	}

	for _, c := range b.clustersStore(region).All() {
		if c.SubnetGroupName == name {
			return ErrSubnetGroupInUse
		}
	}

	sg.Tags.Close()
	tbl.Delete(name)

	return nil
}

// DescribeSubnetGroups returns one subnet group by name, or a paginated list of all.
func (b *InMemoryBackend) DescribeSubnetGroups(
	ctx context.Context,
	name, marker string,
	maxRecords int,
) (page.Page[CacheSubnetGroup], error) {
	b.mu.RLock("DescribeSubnetGroups")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	return describePaged(b.subnetGroupsStoreRO(region), name, ErrSubnetGroupNotFound, nil,
		func(sg CacheSubnetGroup) string { return sg.Name }, marker, maxRecords)
}

// ModifySubnetGroup updates a cache subnet group.
func (b *InMemoryBackend) ModifySubnetGroup(
	ctx context.Context,
	name, description string,
	subnetIDs []string,
) (*CacheSubnetGroup, error) {
	b.mu.Lock("ModifySubnetGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	sg, exists := b.subnetGroupsStore(region).Get(name)
	if !exists {
		return nil, ErrSubnetGroupNotFound
	}

	if description != "" {
		sg.Description = description
	}

	if len(subnetIDs) > 0 {
		if len(subnetIDs) > maxSubnetsPerCacheSubnetGroup {
			return nil, ErrCacheSubnetQuotaExceeded
		}

		sg.SubnetIDs = subnetIDs
	}

	cp := *sg

	return &cp, nil
}

// CreateSubnetGroupFull creates a cache subnet group with an explicit VPC ID.
func (b *InMemoryBackend) CreateSubnetGroupFull(
	ctx context.Context,
	name, description, vpcID string,
	subnetIDs []string,
) (*CacheSubnetGroup, error) {
	b.mu.Lock("CreateSubnetGroupFull")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	tbl := b.subnetGroupsStore(region)
	if _, exists := tbl.Get(name); exists {
		return nil, ErrSubnetGroupAlreadyExists
	}

	if len(subnetIDs) > maxSubnetsPerCacheSubnetGroup {
		return nil, ErrCacheSubnetQuotaExceeded
	}

	if tbl.Len() >= maxCacheSubnetGroupsPerRegion {
		return nil, ErrCacheSubnetGroupQuotaExceeded
	}

	sg := &CacheSubnetGroup{
		Name:        name,
		Description: description,
		VpcID:       vpcID,
		SubnetIDs:   subnetIDs,
		ARN:         b.subnetGroupARN(region, name),
		Tags:        tags.New("elasticache.sg." + name + ".tags"),
	}
	tbl.Put(sg)

	cp := *sg

	return &cp, nil
}

// ----------------------------------------
// CopySnapshotFull — with KmsKeyId
// ----------------------------------------
