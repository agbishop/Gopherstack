package ec2

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// newSubnetID returns a subnet ID in real AWS's shape: "subnet-" followed by
// lowercase hex digits only. uuid.New().String() is hyphenated
// (8-4-4-4-12), so a naive [:N] slice embeds literal "-" characters into the
// ID once N crosses a hyphen boundary; strip them first.
func newSubnetID() string {
	return "subnet-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:17]
}

// CreateDefaultSubnet creates a new default subnet in the given availability zone.
func (b *InMemoryBackend) CreateDefaultSubnet(az string) (*Subnet, error) {
	if az == "" {
		az = b.Region + "a"
	}

	b.mu.Lock("CreateDefaultSubnet")
	defer b.mu.Unlock()

	var defaultVPCID string
	for _, v := range b.vpcs.All() {
		if v.IsDefault {
			defaultVPCID = v.ID

			break
		}
	}
	if defaultVPCID == "" {
		return nil, fmt.Errorf("%w: no default VPC found", ErrVPCNotFound)
	}

	subnet := &Subnet{
		ID:                  newSubnetID(),
		VPCID:               defaultVPCID,
		CIDRBlock:           defaultSubnetCIDR,
		AvailabilityZone:    az,
		IsDefault:           true,
		MapPublicIPOnLaunch: true,
	}
	b.subnets.Put(subnet)

	return subnet, nil
}

// ---- AssociateSubnetCidrBlock ----

// AssociateSubnetCidrBlock adds an IPv6 CIDR block to a subnet.
func (b *InMemoryBackend) AssociateSubnetCidrBlock(
	subnetID, ipv6CIDRBlock string,
) (*SubnetCIDRAssociation, error) {
	if subnetID == "" {
		return nil, fmt.Errorf("%w: SubnetId is required", ErrInvalidParameter)
	}
	if ipv6CIDRBlock == "" {
		return nil, fmt.Errorf("%w: Ipv6CidrBlock is required", ErrInvalidParameter)
	}

	b.mu.Lock("AssociateSubnetCidrBlock")
	defer b.mu.Unlock()

	if _, ok := b.subnets.Get(subnetID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
	}

	assoc := &SubnetCIDRAssociation{
		AssociationID: "subnet-cidr-assoc-" + uuid.New().String()[:8],
		IPv6CIDRBlock: ipv6CIDRBlock,
		State:         stateAssociated,
	}
	b.subnetCIDRAssociations[subnetID] = append(b.subnetCIDRAssociations[subnetID], assoc)

	return assoc, nil
}

// ---- DisassociateSubnetCidrBlock ----

// DisassociateSubnetCidrBlock removes an IPv6 CIDR block association from a subnet.
func (b *InMemoryBackend) DisassociateSubnetCidrBlock(associationID string) (string, error) {
	if associationID == "" {
		return "", fmt.Errorf("%w: AssociationId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DisassociateSubnetCidrBlock")
	defer b.mu.Unlock()

	for subnetID, assocs := range b.subnetCIDRAssociations {
		for i, assoc := range assocs {
			if assoc.AssociationID == associationID {
				b.subnetCIDRAssociations[subnetID] = append(assocs[:i], assocs[i+1:]...)

				return subnetID, nil
			}
		}
	}

	return "", fmt.Errorf("%w: %s", ErrSubnetCIDRNotFound, associationID)
}

// ---- AssociateSecurityGroupVpc ----

// SGVpcAssociationState holds the state of a security group VPC association.
type SGVpcAssociationState struct {
	SGID  string `json:"sgid,omitempty"`
	VPCID string `json:"vpcid,omitempty"`
	State string `json:"state,omitempty"`
}

// CreateSubnetCidrReservation creates a CIDR reservation within a subnet.
func (b *InMemoryBackend) CreateSubnetCidrReservation(
	subnetID, cidr, reservationType, description string,
) (*SubnetCIDRReservation, error) {
	if subnetID == "" || cidr == "" {
		return nil, fmt.Errorf("%w: SubnetId and Cidr are required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateSubnetCidrReservation")
	defer b.mu.Unlock()

	if _, ok := b.subnets.Get(subnetID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
	}

	reservation := &SubnetCIDRReservation{
		SubnetCIDRReservationID: "scr-" + uuid.New().String()[:8],
		SubnetID:                subnetID,
		CIDR:                    cidr,
		ReservationType:         reservationType,
		Description:             description,
		OwnerID:                 b.AccountID,
		State:                   "assigned",
	}
	b.subnetCIDRReservations[subnetID] = append(b.subnetCIDRReservations[subnetID], reservation)

	return reservation, nil
}

// DeleteSubnetCidrReservation removes a subnet CIDR reservation.
func (b *InMemoryBackend) DeleteSubnetCidrReservation(reservationID string) (*SubnetCIDRReservation, error) {
	if reservationID == "" {
		return nil, fmt.Errorf("%w: SubnetCidrReservationId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteSubnetCidrReservation")
	defer b.mu.Unlock()

	for subnetID, reservations := range b.subnetCIDRReservations {
		for i, r := range reservations {
			if r.SubnetCIDRReservationID == reservationID {
				cp := *r
				b.subnetCIDRReservations[subnetID] = append(reservations[:i], reservations[i+1:]...)

				return &cp, nil
			}
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrInvalidParameter, reservationID)
}

// GetSubnetCidrReservations returns CIDR reservations for a subnet.
func (b *InMemoryBackend) GetSubnetCidrReservations(
	subnetID string,
) ([]*SubnetCIDRReservation, error) {
	if subnetID == "" {
		return nil, fmt.Errorf("%w: SubnetId is required", ErrInvalidParameter)
	}

	b.mu.RLock("GetSubnetCidrReservations")
	defer b.mu.RUnlock()

	reservations := b.subnetCIDRReservations[subnetID]
	out := make([]*SubnetCIDRReservation, len(reservations))
	for i, r := range reservations {
		cp := *r
		out[i] = &cp
	}

	return out, nil
}

// ---- GetSecurityGroupsForVpc ----

// SecurityGroupForVpcItem is a SG returned by GetSecurityGroupsForVpc.
type SecurityGroupForVpcItem struct {
	GroupID     string `json:"groupID,omitempty"`
	GroupName   string `json:"groupName,omitempty"`
	Description string `json:"description,omitempty"`
	VPCID       string `json:"vpcid,omitempty"`
}

// ModifySubnetAttribute enables or disables auto-assign public IP for a subnet.
func (b *InMemoryBackend) ModifySubnetAttribute(subnetID, attribute string, value bool) error {
	if subnetID == "" {
		return fmt.Errorf("%w: SubnetId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifySubnetAttribute")
	defer b.mu.Unlock()

	subnet, ok := b.subnets.Get(subnetID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
	}

	switch attribute {
	case attrMapPublicIPOnLaunch:
		subnet.MapPublicIPOnLaunch = value

		return nil
	case attrEnableResourceNameDNSARec:
		return nil
	default:
		return fmt.Errorf("%w: unknown subnet attribute %q", ErrInvalidParameter, attribute)
	}
}

// ---- Network ACL CRUD ----

// DescribeSubnetsByVPC returns subnets filtered by VPC ID using the secondary index.
func (b *InMemoryBackend) DescribeSubnetsByVPC(vpcID string) []*Subnet {
	b.mu.RLock("DescribeSubnetsByVPC")
	defer b.mu.RUnlock()

	var out []*Subnet

	for _, s := range b.subnets.All() {
		if s.VPCID != vpcID {
			continue
		}

		cp := *s
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

// DescribeSubnets returns subnets, optionally filtered by IDs.
// When ids are provided, lookups are O(len(ids)) via the subnet map rather
// than scanning every subnet in the backend.
func (b *InMemoryBackend) DescribeSubnets(ids []string) []*Subnet {
	b.mu.RLock("DescribeSubnets")
	defer b.mu.RUnlock()

	if len(ids) > 0 {
		out := make([]*Subnet, 0, len(ids))

		for _, id := range ids {
			s, ok := b.subnets.Get(id)
			if !ok {
				continue
			}

			cp := *s
			out = append(out, &cp)
		}

		return out
	}

	out := make([]*Subnet, 0, b.subnets.Len())

	for _, s := range b.subnets.All() {
		cp := *s
		out = append(out, &cp)
	}

	return out
}

// CreateSubnet creates a new subnet in the given VPC.
func (b *InMemoryBackend) CreateSubnet(vpcID, cidr, az string) (*Subnet, error) {
	return b.CreateSubnetWithOutpost(vpcID, cidr, az, "")
}

// CreateSubnetWithOutpost is CreateSubnet plus an optional OutpostArn,
// cross-validated against the real Outposts backend when wired (see
// cross_service.go's validateOutpostArn) -- matches real AWS CreateSubnet
// rejecting an OutpostArn that doesn't resolve to a real Outpost.
func (b *InMemoryBackend) CreateSubnetWithOutpost(vpcID, cidr, az, outpostArn string) (*Subnet, error) {
	if vpcID == "" {
		return nil, fmt.Errorf("%w: VpcId is required", ErrInvalidParameter)
	}

	if cidr == "" {
		return nil, fmt.Errorf("%w: CidrBlock is required", ErrInvalidParameter)
	}

	if err := b.validateOutpostArn(outpostArn); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateSubnet")
	defer b.mu.Unlock()

	if _, ok := b.vpcs.Get(vpcID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrVPCNotFound, vpcID)
	}

	if az == "" {
		az = b.Region + "a"
	}

	vpc, _ := b.vpcs.Get(vpcID)
	if !cidrContains(vpc.CIDRBlock, cidr) {
		return nil, fmt.Errorf("%w: subnet CIDR %s is not within VPC CIDR %s",
			ErrInvalidParameter, cidr, vpc.CIDRBlock)
	}

	for _, existing := range b.subnets.All() {
		if existing.VPCID == vpcID && cidrsOverlap(cidr, existing.CIDRBlock) {
			return nil, fmt.Errorf("%w: CIDR %s overlaps with existing subnet %s (%s)",
				ErrCIDRConflict, cidr, existing.ID, existing.CIDRBlock)
		}
	}

	id := newSubnetID()
	s := &Subnet{
		ID:               id,
		VPCID:            vpcID,
		CIDRBlock:        cidr,
		AvailabilityZone: az,
		OutpostArn:       outpostArn,
	}
	b.subnets.Put(s)
	b.indexSubnetLocked(id, vpcID)

	return s, nil
}

// subnetDependencyViolationLocked returns a DependencyViolation error naming
// the first dependent resource found for subnetID, or nil if the subnet has
// no dependents blocking deletion. Mirrors real AWS: network interfaces
// (including the primary ENI of every non-terminated instance — instances
// must be terminated first), NAT gateways, and VPC endpoints using the subnet
// all block deletion. Must be called with b.mu held.
func (b *InMemoryBackend) subnetDependencyViolationLocked(subnetID string) error {
	for _, eni := range b.networkInterfaces.All() {
		if eni.SubnetID == subnetID {
			return fmt.Errorf(
				"%w: the subnet %s has dependencies (network interface %s) and cannot be deleted",
				ErrDependencyViolation, subnetID, networkInterfacesKeyFn(eni),
			)
		}
	}

	for _, ngw := range b.natGateways.All() {
		if ngw.SubnetID == subnetID {
			return fmt.Errorf(
				"%w: the subnet %s has dependencies (NAT gateway %s) and cannot be deleted",
				ErrDependencyViolation, subnetID, natGatewaysKeyFn(ngw),
			)
		}
	}

	for _, ep := range b.vpcEndpoints.All() {
		if slices.Contains(ep.SubnetIDs, subnetID) {
			return fmt.Errorf(
				"%w: the subnet %s has dependencies (VPC endpoint %s) and cannot be deleted",
				ErrDependencyViolation, subnetID, ep.ID,
			)
		}
	}

	return nil
}

// DeleteSubnet removes a subnet by ID. Matching real AWS, this fails with
// DependencyViolation if the subnet still has dependent resources (network
// interfaces, NAT gateways, VPC endpoints) — it does NOT cascade-delete them.
func (b *InMemoryBackend) DeleteSubnet(id string) error {
	b.mu.Lock("DeleteSubnet")
	defer b.mu.Unlock()

	subnet, ok := b.subnets.Get(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrSubnetNotFound, id)
	}

	if err := b.subnetDependencyViolationLocked(id); err != nil {
		return err
	}

	b.deindexSubnetLocked(id, subnet.VPCID)
	b.subnets.Delete(id)
	delete(b.tags, id)
	delete(b.subnetCIDRReservations, id)
	delete(b.subnetCIDRAssociations, id)

	return nil
}
