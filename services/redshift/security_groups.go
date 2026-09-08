package redshift

import (
	"fmt"
	"maps"
	"slices"
)

// CreateClusterSecurityGroup creates a new cluster security group.
func (b *InMemoryBackend) CreateClusterSecurityGroup(
	name, description string, tagsIn map[string]string,
) (*ClusterSecurityGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: ClusterSecurityGroupName is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateClusterSecurityGroup")
	defer b.mu.Unlock()

	if _, exists := b.securityGroups.Get(name); exists {
		return nil, fmt.Errorf("%w: security group %s already exists", ErrSecurityGroupAlreadyExists, name)
	}

	sg := &ClusterSecurityGroup{
		ClusterSecurityGroupName: name,
		Description:              description,
		IPRanges:                 []IPRange{},
		EC2SecurityGroups:        []EC2SecurityGroup{},
		Tags:                     tagsIn,
	}
	b.securityGroups.Put(sg)

	return cloneSecurityGroup(sg), nil
}

// defaultClusterSecurityGroupName is the name of the security group every
// account has provisioned by default. Real AWS: DeleteClusterSecurityGroup's
// own doc comment, "You cannot delete the default security group".
const defaultClusterSecurityGroupName = "default"

// DeleteClusterSecurityGroup removes a cluster security group. Real AWS:
// "You cannot delete a security group that is associated with any
// clusters. You cannot delete the default security group".
func (b *InMemoryBackend) DeleteClusterSecurityGroup(name string) error {
	if name == "" {
		return fmt.Errorf("%w: ClusterSecurityGroupName is required", ErrInvalidParameter)
	}

	if name == defaultClusterSecurityGroupName {
		return fmt.Errorf("%w: cannot delete the default security group", ErrSecurityGroupInvalidState)
	}

	b.mu.Lock("DeleteClusterSecurityGroup")
	defer b.mu.Unlock()

	if _, exists := b.securityGroups.Get(name); !exists {
		return fmt.Errorf("%w: security group %s not found", ErrSecurityGroupNotFound, name)
	}

	for _, c := range b.clusters.All() {
		if slices.Contains(c.ClusterSecurityGroups, name) {
			return fmt.Errorf(
				"%w: security group %s is associated with cluster %s",
				ErrSecurityGroupInvalidState, name, c.ClusterIdentifier,
			)
		}
	}

	b.securityGroups.Delete(name)

	return nil
}

// DescribeClusterSecurityGroups returns security groups. If name is
// non-empty, returns only that group (ignoring marker/maxRecords/tag
// filters, matching DescribeClusters' id-lookup shortcut). Otherwise
// tagKeys/tagValues are applied to the full set before Marker/MaxRecords
// pagination, following the same convention as DescribeClusters (see
// store.go).
func (b *InMemoryBackend) DescribeClusterSecurityGroups(
	name, marker string, maxRecords int, tagKeys, tagValues []string,
) ([]ClusterSecurityGroup, string, error) {
	b.mu.RLock("DescribeClusterSecurityGroups")
	defer b.mu.RUnlock()

	return describeTaggedGroup(
		b.securityGroups, name, marker, maxRecords, tagKeys, tagValues,
		func(name string) error {
			return fmt.Errorf("%w: security group %s not found", ErrSecurityGroupNotFound, name)
		},
		func(sg *ClusterSecurityGroup) ClusterSecurityGroup { return *cloneSecurityGroup(sg) },
		func(sg *ClusterSecurityGroup) map[string]string { return sg.Tags },
		func(sg *ClusterSecurityGroup) string { return sg.ClusterSecurityGroupName },
	)
}

// RevokeClusterSecurityGroupIngress removes an ingress rule from a cluster security group.
func (b *InMemoryBackend) RevokeClusterSecurityGroupIngress(
	groupName, cidrIP, ec2GroupName, ec2GroupOwnerID string,
) (*ClusterSecurityGroup, error) {
	if groupName == "" {
		return nil, fmt.Errorf("%w: ClusterSecurityGroupName is required", ErrInvalidParameter)
	}
	if cidrIP == "" && ec2GroupName == "" {
		return nil, fmt.Errorf("%w: CIDRIP or EC2SecurityGroupName is required", ErrInvalidParameter)
	}

	b.mu.Lock("RevokeClusterSecurityGroupIngress")
	defer b.mu.Unlock()

	sg, exists := b.securityGroups.Get(groupName)
	if !exists {
		return nil, fmt.Errorf("%w: security group %s not found", ErrSecurityGroupNotFound, groupName)
	}

	foundCIDR := revokeCIDRIngress(sg, cidrIP)
	foundEC2Group := revokeEC2GroupIngress(sg, ec2GroupName, ec2GroupOwnerID)

	if !foundCIDR && !foundEC2Group {
		return nil, fmt.Errorf(
			"%w: no matching ingress rule in security group %s",
			ErrSecurityGroupIngressNotFound,
			groupName,
		)
	}

	return cloneSecurityGroup(sg), nil
}

// revokeCIDRIngress removes cidrIP from sg.IPRanges, reporting whether it was present.
// A blank cidrIP is a no-op (the caller didn't ask to revoke a CIDR rule).
func revokeCIDRIngress(sg *ClusterSecurityGroup, cidrIP string) bool {
	if cidrIP == "" {
		return false
	}

	before := len(sg.IPRanges)
	filtered := sg.IPRanges[:0]

	for _, r := range sg.IPRanges {
		if r.CIDRIP != cidrIP {
			filtered = append(filtered, r)
		}
	}

	sg.IPRanges = filtered

	return len(filtered) < before
}

// revokeEC2GroupIngress removes the matching EC2 security group from sg.EC2SecurityGroups,
// reporting whether it was present. A blank ec2GroupName is a no-op.
func revokeEC2GroupIngress(sg *ClusterSecurityGroup, ec2GroupName, ec2GroupOwnerID string) bool {
	if ec2GroupName == "" {
		return false
	}

	before := len(sg.EC2SecurityGroups)
	filtered := sg.EC2SecurityGroups[:0]

	for _, g := range sg.EC2SecurityGroups {
		if g.EC2SecurityGroupName != ec2GroupName ||
			(ec2GroupOwnerID != "" && g.EC2SecurityGroupOwnerID != ec2GroupOwnerID) {
			filtered = append(filtered, g)
		}
	}

	sg.EC2SecurityGroups = filtered

	return len(filtered) < before
}

// AuthorizeClusterSecurityGroupIngress adds an ingress rule to a cluster security group.
func (b *InMemoryBackend) AuthorizeClusterSecurityGroupIngress(
	groupName, cidrIP, ec2GroupName, ec2GroupOwnerID string,
) (*ClusterSecurityGroup, error) {
	if groupName == "" {
		return nil, fmt.Errorf("%w: ClusterSecurityGroupName is required", ErrInvalidParameter)
	}
	if cidrIP == "" && ec2GroupName == "" {
		return nil, fmt.Errorf("%w: CIDRIP or EC2SecurityGroupName is required", ErrInvalidParameter)
	}

	b.mu.Lock("AuthorizeClusterSecurityGroupIngress")
	defer b.mu.Unlock()

	sg, exists := b.securityGroups.Get(groupName)
	if !exists {
		return nil, fmt.Errorf("%w: security group %s not found", ErrSecurityGroupNotFound, groupName)
	}

	if cidrIP != "" {
		for _, r := range sg.IPRanges {
			if r.CIDRIP == cidrIP {
				return nil, fmt.Errorf(
					"%w: CIDRIP %s already authorized on security group %s",
					ErrAuthorizationAlreadyExists, cidrIP, groupName,
				)
			}
		}

		sg.IPRanges = append(sg.IPRanges, IPRange{CIDRIP: cidrIP, Status: ingressStatusAuthorized})
	}
	if ec2GroupName != "" {
		for _, g := range sg.EC2SecurityGroups {
			if g.EC2SecurityGroupName == ec2GroupName && g.EC2SecurityGroupOwnerID == ec2GroupOwnerID {
				return nil, fmt.Errorf(
					"%w: EC2SecurityGroupName %s already authorized on security group %s",
					ErrAuthorizationAlreadyExists, ec2GroupName, groupName,
				)
			}
		}

		sg.EC2SecurityGroups = append(sg.EC2SecurityGroups, EC2SecurityGroup{
			EC2SecurityGroupName:    ec2GroupName,
			EC2SecurityGroupOwnerID: ec2GroupOwnerID,
			Status:                  ingressStatusAuthorized,
		})
	}

	return cloneSecurityGroup(sg), nil
}

// AddSecurityGroupInternal seeds a cluster security group directly into the backend.
func (b *InMemoryBackend) AddSecurityGroupInternal(sg *ClusterSecurityGroup) {
	b.mu.Lock("AddSecurityGroupInternal")
	defer b.mu.Unlock()
	b.securityGroups.Put(sg)
}

// cloneSecurityGroup returns a deep copy of a ClusterSecurityGroup.
func cloneSecurityGroup(sg *ClusterSecurityGroup) *ClusterSecurityGroup {
	cp := *sg
	cp.IPRanges = make([]IPRange, len(sg.IPRanges))
	copy(cp.IPRanges, sg.IPRanges)
	cp.EC2SecurityGroups = make([]EC2SecurityGroup, len(sg.EC2SecurityGroups))
	copy(cp.EC2SecurityGroups, sg.EC2SecurityGroups)
	cp.Tags = maps.Clone(sg.Tags)

	return &cp
}
