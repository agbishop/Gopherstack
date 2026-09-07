package dax

import (
	"fmt"
	"regexp"
	"strings"
)

// vpcSuffixMaxLen is the maximum length of the VPC ID suffix derived from a subnet ID.
const vpcSuffixMaxLen = 8

// subnetRegexp validates subnet IDs.
var subnetRegexp = regexp.MustCompile(`^subnet-[0-9a-f]{8}([0-9a-f]{9})?$`)

// CreateSubnetGroup creates a DAX subnet group.
func (b *InMemoryBackend) CreateSubnetGroup(
	name, description string,
	subnetIDs []string,
) (*SubnetGroup, error) {
	// Unlike ClusterName/ParameterGroupName, SubnetGroupName carries no
	// format constraint in the real API (validateOpCreateSubnetGroupInput
	// only checks presence) and CreateSubnetGroup's own
	// deserializeOpErrorCreateSubnetGroup switch has no
	// InvalidParameterValueException case -- so it cannot legitimately
	// reject a name on format grounds.
	if err := validateSubnetIDs(subnetIDs); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateSubnetGroup")
	defer b.mu.Unlock()

	if b.subnetGroups.Has(name) {
		return nil, fmt.Errorf("%w: %s", ErrSubnetGroupAlreadyExists, name)
	}

	subnets := subnetEntriesFromIDs(subnetIDs, b.Region)
	vpcID := vpcIDFromSubnets(subnetIDs)

	sg := &SubnetGroup{
		SubnetGroupName:       name,
		Description:           description,
		VpcID:                 vpcID,
		Subnets:               subnets,
		SupportedNetworkTypes: []string{NetworkTypeIPv4},
	}

	b.subnetGroups.Put(sg)

	b.emitEventLocked(name, EventSourceTypeSubnetGroup,
		fmt.Sprintf("Subnet group %s created.", name))

	return subnetGroupCopy(sg), nil
}

// DescribeSubnetGroups returns DAX subnet groups with pagination.
func (b *InMemoryBackend) DescribeSubnetGroups(
	names []string,
	maxResults int,
	nextToken string,
) ([]*SubnetGroup, string, error) {
	b.mu.RLock("DescribeSubnetGroups")
	defer b.mu.RUnlock()

	return describeNamedGroups(
		b.subnetGroups, names, maxResults, nextToken, ErrSubnetGroupNotFound,
		func(sg *SubnetGroup) string { return sg.SubnetGroupName }, subnetGroupCopy,
	)
}

// UpdateSubnetGroup updates a subnet group's description and/or subnet list.
func (b *InMemoryBackend) UpdateSubnetGroup(input UpdateSubnetGroupInput) (*SubnetGroup, error) {
	// UpdateSubnetGroup's own deserializeOpErrorUpdateSubnetGroup switch has no
	// InvalidParameterValueException case; SubnetGroupNotFoundFault is the code it
	// actually types, matching DeleteSubnetGroup's treatment of the same condition.
	if input.SubnetGroupName == "" {
		return nil, fmt.Errorf("%w: SubnetGroupName is required", ErrSubnetGroupNotFound)
	}

	if len(input.SubnetIDs) > 0 {
		if err := validateSubnetIDs(input.SubnetIDs); err != nil {
			return nil, err
		}
	}

	b.mu.Lock("UpdateSubnetGroup")
	defer b.mu.Unlock()

	sg, ok := b.subnetGroups.Get(input.SubnetGroupName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSubnetGroupNotFound, input.SubnetGroupName)
	}

	if input.Description != "" {
		sg.Description = input.Description
	}

	if len(input.SubnetIDs) > 0 {
		sg.Subnets = subnetEntriesFromIDs(input.SubnetIDs, b.Region)
		sg.VpcID = vpcIDFromSubnets(input.SubnetIDs)
	}

	b.emitEventLocked(input.SubnetGroupName, EventSourceTypeSubnetGroup,
		fmt.Sprintf("Subnet group %s updated.", input.SubnetGroupName))

	return subnetGroupCopy(sg), nil
}

// DeleteSubnetGroup deletes a DAX subnet group.
func (b *InMemoryBackend) DeleteSubnetGroup(name string) error {
	if name == "" {
		return fmt.Errorf("%w: SubnetGroupName is required", ErrSubnetGroupNotFound)
	}

	b.mu.Lock("DeleteSubnetGroup")
	defer b.mu.Unlock()

	if !b.subnetGroups.Has(name) {
		return fmt.Errorf("%w: %s", ErrSubnetGroupNotFound, name)
	}

	for _, cluster := range b.clusters.All() {
		if cluster.SubnetGroupName == name {
			return fmt.Errorf("%w: subnet group %s is in use by cluster %s",
				ErrSubnetGroupInUse, name, cluster.ClusterName)
		}
	}

	b.subnetGroups.Delete(name)

	return nil
}

// subnetGroupCopy returns a deep copy of a SubnetGroup.
func subnetGroupCopy(sg *SubnetGroup) *SubnetGroup {
	cp := *sg
	cp.Subnets = append([]SubnetEntry(nil), sg.Subnets...)
	cp.SupportedNetworkTypes = append([]string(nil), sg.SupportedNetworkTypes...)

	return &cp
}

// validateSubnetIDs rejects a missing or malformed SubnetIds list.
// CreateSubnetGroup/UpdateSubnetGroup's own deserializeOpError switches
// (aws-sdk-go-v2/service/dax@v1.32.4 deserializers.go) type InvalidSubnet
// for this, not the generic InvalidParameterValueException every other
// bad-value check in this package uses -- neither op's switch has an
// InvalidParameterValueException case at all. The SDK's client-side
// validateOpCreateSubnetGroupInput only rejects a nil SubnetIds, not an
// empty-but-non-nil one, so a real client can reach both branches here.
func validateSubnetIDs(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("%w: at least one SubnetId is required", ErrInvalidSubnet)
	}
	for _, id := range ids {
		if !subnetRegexp.MatchString(id) {
			return fmt.Errorf("%w: invalid subnet ID %q", ErrInvalidSubnet, id)
		}
	}

	return nil
}

// subnetEntriesFromIDs converts string subnet IDs to SubnetEntry slices using default AZ.
func subnetEntriesFromIDs(ids []string, region string) []SubnetEntry {
	entries := make([]SubnetEntry, 0, len(ids))

	for _, id := range ids {
		entries = append(entries, SubnetEntry{
			SubnetID:              id,
			AvailabilityZone:      region + "a",
			SupportedNetworkTypes: []string{NetworkTypeIPv4},
		})
	}

	return entries
}

// vpcIDFromSubnets returns a deterministic placeholder VPC ID derived from the first subnet ID.
// Real AWS would look up the actual VPC; in emulation we derive a plausible ID from the subnet.
func vpcIDFromSubnets(subnetIDs []string) string {
	if len(subnetIDs) == 0 {
		return "vpc-00000000"
	}

	first := subnetIDs[0]
	if idx := strings.LastIndexByte(first, '-'); idx >= 0 && idx < len(first)-1 {
		suffix := first[idx+1:]
		if len(suffix) > vpcSuffixMaxLen {
			suffix = suffix[:vpcSuffixMaxLen]
		}

		return "vpc-" + suffix
	}

	return "vpc-00000000"
}
