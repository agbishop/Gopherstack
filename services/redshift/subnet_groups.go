package redshift

import (
	"fmt"
	"maps"
)

// Subnet represents a single subnet within a cluster subnet group.
type Subnet struct {
	SubnetIdentifier string `json:"subnetIdentifier"`
	SubnetStatus     string `json:"subnetStatus"`
}

// ClusterSubnetGroup represents a Redshift cluster subnet group.
type ClusterSubnetGroup struct {
	Tags                   map[string]string `json:"tags,omitempty"`
	ClusterSubnetGroupName string            `json:"clusterSubnetGroupName"`
	Description            string            `json:"description"`
	VpcID                  string            `json:"vpcId"`
	SubnetGroupStatus      string            `json:"subnetGroupStatus"`
	Subnets                []Subnet          `json:"subnets"`
}

// CreateClusterSubnetGroup creates a new cluster subnet group.
func (b *InMemoryBackend) CreateClusterSubnetGroup(
	name, description, vpcID string,
	subnetIDs []string,
	tagsIn map[string]string,
) (*ClusterSubnetGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: ClusterSubnetGroupName is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateClusterSubnetGroup")
	defer b.mu.Unlock()

	if _, exists := b.subnetGroups.Get(name); exists {
		return nil, fmt.Errorf("%w: subnet group %s already exists", ErrSubnetGroupAlreadyExists, name)
	}

	subnets := make([]Subnet, 0, len(subnetIDs))
	for _, id := range subnetIDs {
		subnets = append(subnets, Subnet{SubnetIdentifier: id, SubnetStatus: partnerStatusActive})
	}

	sg := &ClusterSubnetGroup{
		ClusterSubnetGroupName: name,
		Description:            description,
		VpcID:                  vpcID,
		SubnetGroupStatus:      "Complete",
		Subnets:                subnets,
		Tags:                   tagsIn,
	}
	b.subnetGroups.Put(sg)

	return cloneSubnetGroup(sg), nil
}

// DeleteClusterSubnetGroup removes a cluster subnet group.
func (b *InMemoryBackend) DeleteClusterSubnetGroup(name string) error {
	if name == "" {
		return fmt.Errorf("%w: ClusterSubnetGroupName is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteClusterSubnetGroup")
	defer b.mu.Unlock()

	if _, exists := b.subnetGroups.Get(name); !exists {
		return fmt.Errorf("%w: subnet group %s not found", ErrSubnetGroupNotFound, name)
	}

	b.subnetGroups.Delete(name)

	return nil
}

// DescribeClusterSubnetGroups returns subnet groups. If name is non-empty,
// returns only that group (ignoring marker/maxRecords/tag filters, matching
// DescribeClusters' id-lookup shortcut). Otherwise tagKeys/tagValues are
// applied to the full set before Marker/MaxRecords pagination, following the
// same convention as DescribeClusters (see store.go).
func (b *InMemoryBackend) DescribeClusterSubnetGroups(
	name, marker string, maxRecords int, tagKeys, tagValues []string,
) ([]ClusterSubnetGroup, string, error) {
	b.mu.RLock("DescribeClusterSubnetGroups")
	defer b.mu.RUnlock()

	return describeTaggedGroup(
		b.subnetGroups, name, marker, maxRecords, tagKeys, tagValues,
		func(name string) error {
			return fmt.Errorf("%w: subnet group %s not found", ErrSubnetGroupNotFound, name)
		},
		func(sg *ClusterSubnetGroup) ClusterSubnetGroup { return *cloneSubnetGroup(sg) },
		func(sg *ClusterSubnetGroup) map[string]string { return sg.Tags },
		func(sg *ClusterSubnetGroup) string { return sg.ClusterSubnetGroupName },
	)
}

// ModifyClusterSubnetGroup updates the description or subnets of a cluster subnet group.
func (b *InMemoryBackend) ModifyClusterSubnetGroup(
	name, description string,
	subnetIDs []string,
) (*ClusterSubnetGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: ClusterSubnetGroupName is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyClusterSubnetGroup")
	defer b.mu.Unlock()

	sg, exists := b.subnetGroups.Get(name)
	if !exists {
		return nil, fmt.Errorf("%w: subnet group %s not found", ErrSubnetGroupNotFound, name)
	}

	if description != "" {
		sg.Description = description
	}

	if len(subnetIDs) > 0 {
		subnets := make([]Subnet, 0, len(subnetIDs))
		for _, id := range subnetIDs {
			subnets = append(subnets, Subnet{SubnetIdentifier: id, SubnetStatus: partnerStatusActive})
		}

		sg.Subnets = subnets
	}

	return cloneSubnetGroup(sg), nil
}

// cloneSubnetGroup returns a deep copy of a ClusterSubnetGroup.
func cloneSubnetGroup(sg *ClusterSubnetGroup) *ClusterSubnetGroup {
	cp := *sg
	cp.Subnets = make([]Subnet, len(sg.Subnets))
	copy(cp.Subnets, sg.Subnets)
	cp.Tags = maps.Clone(sg.Tags)

	return &cp
}

// AddSubnetGroupInternal seeds a subnet group directly into the backend.
func (b *InMemoryBackend) AddSubnetGroupInternal(sg *ClusterSubnetGroup) {
	b.mu.Lock("AddSubnetGroupInternal")
	defer b.mu.Unlock()
	b.subnetGroups.Put(sg)
}
