package ec2

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// awsRegions is the standard "aws" partition commercial region list, sourced
// from the partition entries in the pinned aws-sdk-go-v2/service/ec2 v1.319.1
// module (internal/endpoints/endpoints.go). It excludes cn-*, us-gov-*, and
// us-iso-* regions, which belong to separate partitions.
//
//nolint:gochecknoglobals // package-level lookup data for DescribeRegions
var awsRegions = []string{
	regionUSEast1, "us-east-2", "us-west-1", "us-west-2",
	"af-south-1",
	"ap-east-1", "ap-east-2",
	"ap-northeast-1", "ap-northeast-2", "ap-northeast-3",
	"ap-south-1", "ap-south-2",
	"ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-southeast-4", "ap-southeast-5",
	"ap-southeast-6", "ap-southeast-7",
	"ca-central-1", "ca-west-1",
	"eu-central-1", "eu-central-2",
	"eu-north-1",
	"eu-south-1", "eu-south-2",
	"eu-west-1", "eu-west-2", "eu-west-3",
	"il-central-1",
	"me-central-1", "me-south-1",
	"mx-central-1",
	"sa-east-1",
}

// DescribeRegions returns the standard AWS commercial region names.
func (b *InMemoryBackend) DescribeRegions() []string {
	return awsRegions
}

// DescribeAvailabilityZones returns AZs for a region.
func (b *InMemoryBackend) DescribeAvailabilityZones(region string) []string {
	if region == "" {
		region = b.Region
	}

	return []string{region + "a", region + "b", region + "c"}
}

// ---- errors ----

var (
	// ErrEgressOnlyIGWNotFound is returned when an egress-only internet gateway is not found.
	ErrEgressOnlyIGWNotFound = errors.New("InvalidEgressOnlyInternetGatewayID.NotFound")
	// ErrIAMAssociationNotFound is returned when an IAM instance profile association is not found.
	ErrIAMAssociationNotFound = errors.New("InvalidAssociationID.NotFound")
	// ErrIAMInstanceProfileAlreadyAssociated is returned by AssociateIamInstanceProfile
	// when the target instance already has an active association: real AWS
	// "You cannot associate more than one IAM instance profile with an
	// instance" (ec2@v1.319.1 api_op_AssociateIamInstanceProfile.go).
	ErrIAMInstanceProfileAlreadyAssociated = errors.New("IncorrectState")
	// ErrTGWRouteTableNotFound is returned when a transit gateway route table is not found.
	ErrTGWRouteTableNotFound = errors.New("InvalidTransitGatewayRouteTableId.NotFound")
)

// ---- constants ----

const (
	tgwRouteTypeStatic     = "static"
	tgwRouteStateDeleted   = "deleted"
	tgwRouteStateBlackhole = "blackhole"
)

// ---- models ----

// EgressOnlyInternetGateway represents an EC2 egress-only internet gateway.
type EgressOnlyInternetGateway struct {
	CreateTime time.Time `json:"createTime"`
	ID         string    `json:"id,omitempty"`
	VPCID      string    `json:"vpcID,omitempty"`
	State      string    `json:"state,omitempty"`
}

// IamInstanceProfileAssociation represents an IAM instance profile association.
type IamInstanceProfileAssociation struct {
	Timestamp          time.Time `json:"timestamp"`
	AssociationID      string    `json:"associationID,omitempty"`
	InstanceID         string    `json:"instanceID,omitempty"`
	IamInstanceProfile string    `json:"iamInstanceProfile,omitempty"` // ARN or name
	State              string    `json:"state,omitempty"`
}

// TransitGatewayRouteTable represents a TGW route table.
type TransitGatewayRouteTable struct {
	CreateTime         time.Time `json:"createTime"`
	TransitGatewayID   string    `json:"transitGatewayID,omitempty"`
	RouteTableID       string    `json:"routeTableID,omitempty"`
	State              string    `json:"state,omitempty"`
	DefaultAssociation bool      `json:"defaultAssociation,omitempty"`
	DefaultPropagation bool      `json:"defaultPropagation,omitempty"`
}

// TransitGatewayRoute represents a route in a TGW route table.
type TransitGatewayRoute struct {
	DestinationCidrBlock       string `json:"destinationCidrBlock,omitempty"`
	TransitGatewayAttachmentID string `json:"transitGatewayAttachmentID,omitempty"`
	TransitGatewayRouteTableID string `json:"transitGatewayRouteTableID,omitempty"`
	State                      string `json:"state,omitempty"`
	Type                       string `json:"type,omitempty"`
	// ResourceID/ResourceType describe the attachment's real underlying
	// resource (e.g. a VPC ID / "vpc", a peer transit gateway ID / "peering"),
	// derived via tgwAttachmentResourceLocked at creation time. Empty for a
	// blackhole route, which has no attachment.
	ResourceID   string `json:"resourceID,omitempty"`
	ResourceType string `json:"resourceType,omitempty"`
}

// TransitGatewayRouteTableAssociation represents an association between a TGW route table and attachment.
type TransitGatewayRouteTableAssociation struct {
	TransitGatewayRouteTableID string `json:"transitGatewayRouteTableID,omitempty"`
	TransitGatewayAttachmentID string `json:"transitGatewayAttachmentID,omitempty"`
	ResourceID                 string `json:"resourceID,omitempty"`
	ResourceType               string `json:"resourceType,omitempty"`
	State                      string `json:"state,omitempty"`
}

// VpcCidrBlockAssociation represents a secondary CIDR block associated with a VPC.
type VpcCidrBlockAssociation struct {
	AssociationID string `json:"associationID,omitempty"`
	CidrBlock     string `json:"cidrBlock,omitempty"`
	State         string `json:"state,omitempty"`
}

// ---- EgressOnly Internet Gateway ----

// CreateEgressOnlyInternetGateway creates a new egress-only internet gateway.
func (b *InMemoryBackend) CreateEgressOnlyInternetGateway(
	vpcID string,
) (*EgressOnlyInternetGateway, error) {
	if vpcID == "" {
		return nil, fmt.Errorf("%w: VpcId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateEgressOnlyInternetGateway")
	defer b.mu.Unlock()

	if _, ok := b.vpcs.Get(vpcID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrVPCNotFound, vpcID)
	}

	igw := &EgressOnlyInternetGateway{
		ID:         newEgressOnlyInternetGatewayID(),
		VPCID:      vpcID,
		State:      stateAvailable,
		CreateTime: time.Now(),
	}
	b.egressOnlyIGWs.Put(igw)

	cp := *igw

	return &cp, nil
}

// DescribeEgressOnlyInternetGateways returns egress-only internet gateways, optionally filtered by IDs.
func (b *InMemoryBackend) DescribeEgressOnlyInternetGateways(
	ids []string,
) []*EgressOnlyInternetGateway {
	b.mu.RLock("DescribeEgressOnlyInternetGateways")
	defer b.mu.RUnlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	out := make([]*EgressOnlyInternetGateway, 0, b.egressOnlyIGWs.Len())

	for _, igw := range b.egressOnlyIGWs.All() {
		if len(idSet) > 0 && !idSet[igw.ID] {
			continue
		}

		cp := *igw
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

// DeleteEgressOnlyInternetGateway removes an egress-only internet gateway.
func (b *InMemoryBackend) DeleteEgressOnlyInternetGateway(id string) error {
	if id == "" {
		return fmt.Errorf("%w: EgressOnlyInternetGatewayId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteEgressOnlyInternetGateway")
	defer b.mu.Unlock()

	if _, ok := b.egressOnlyIGWs.Get(id); !ok {
		return fmt.Errorf("%w: %s", ErrEgressOnlyIGWNotFound, id)
	}
	b.egressOnlyIGWs.Delete(id)
	delete(b.tags, id)

	return nil
}

// ---- IAM Instance Profile Associations ----

// AssociateIamInstanceProfile associates an IAM instance profile with an instance.
func (b *InMemoryBackend) AssociateIamInstanceProfile(
	instanceID, profileARN string,
) (*IamInstanceProfileAssociation, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("%w: InstanceId is required", ErrInvalidParameter)
	}

	b.mu.Lock("AssociateIamInstanceProfile")
	defer b.mu.Unlock()

	if _, ok := b.instances.Get(instanceID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, instanceID)
	}

	for _, existing := range b.iamAssociations.All() {
		if existing.InstanceID == instanceID && existing.State == stateAssociated {
			return nil, fmt.Errorf(
				"%w: There is an existing association for instance %s",
				ErrIAMInstanceProfileAlreadyAssociated, instanceID,
			)
		}
	}

	assoc := &IamInstanceProfileAssociation{
		AssociationID:      newIAMInstanceProfileAssociationID(),
		InstanceID:         instanceID,
		IamInstanceProfile: profileARN,
		State:              stateAssociated,
		Timestamp:          time.Now(),
	}
	b.iamAssociations.Put(assoc)

	cp := *assoc

	return &cp, nil
}

// DisassociateIamInstanceProfile removes an IAM instance profile association.
func (b *InMemoryBackend) DisassociateIamInstanceProfile(
	associationID string,
) (*IamInstanceProfileAssociation, error) {
	if associationID == "" {
		return nil, fmt.Errorf("%w: AssociationId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DisassociateIamInstanceProfile")
	defer b.mu.Unlock()

	assoc, ok := b.iamAssociations.Get(associationID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrIAMAssociationNotFound, associationID)
	}
	b.iamAssociations.Delete(associationID)

	cp := *assoc

	return &cp, nil
}

// disassociateIamInstanceProfilesLocked removes every IAM instance profile
// association for instanceID. Called from TerminateInstances so a terminated
// instance never leaves a ghost "associated" row behind (gopherstack-hmfm).
// Must be called with b.mu held.
func (b *InMemoryBackend) disassociateIamInstanceProfilesLocked(instanceID string) {
	for _, assoc := range b.iamAssociations.All() {
		if assoc.InstanceID == instanceID {
			b.iamAssociations.Delete(assoc.AssociationID)
		}
	}
}

// DescribeIamInstanceProfileAssociations returns IAM instance profile associations,
// optionally filtered by IDs or instance ID.
func (b *InMemoryBackend) DescribeIamInstanceProfileAssociations(
	associationIDs []string,
	instanceID string,
) []*IamInstanceProfileAssociation {
	b.mu.RLock("DescribeIamInstanceProfileAssociations")
	defer b.mu.RUnlock()

	idSet := make(map[string]bool, len(associationIDs))
	for _, id := range associationIDs {
		idSet[id] = true
	}

	out := make([]*IamInstanceProfileAssociation, 0, b.iamAssociations.Len())

	for _, assoc := range b.iamAssociations.All() {
		if len(idSet) > 0 && !idSet[assoc.AssociationID] {
			continue
		}

		if instanceID != "" && assoc.InstanceID != instanceID {
			continue
		}

		cp := *assoc
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].AssociationID < out[j].AssociationID
	})

	return out
}

// ReplaceIamInstanceProfileAssociation replaces an IAM instance profile on an existing association.
func (b *InMemoryBackend) ReplaceIamInstanceProfileAssociation(
	associationID, profileARN string,
) (*IamInstanceProfileAssociation, error) {
	if associationID == "" {
		return nil, fmt.Errorf("%w: AssociationId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ReplaceIamInstanceProfileAssociation")
	defer b.mu.Unlock()

	assoc, ok := b.iamAssociations.Get(associationID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrIAMAssociationNotFound, associationID)
	}

	assoc.IamInstanceProfile = profileARN
	assoc.Timestamp = time.Now()

	cp := *assoc

	return &cp, nil
}

// ---- ReplaceRouteTableAssociation ----

// ReplaceRouteTableAssociation replaces an existing route table association with a new route table.
// Returns the new association ID. Reassigning a VPC's main route table by
// passing its implicit association ID (ec2@v1.319.1
// api_op_ReplaceRouteTableAssociation.go:17: "You can also use this
// operation to change which table is the main route table in the VPC") is
// not supported -- rejected explicitly rather than silently corrupting the
// main-table invariant.
func (b *InMemoryBackend) ReplaceRouteTableAssociation(
	associationID, newRouteTableID string,
) (string, error) {
	if associationID == "" {
		return "", fmt.Errorf("%w: AssociationId is required", ErrInvalidParameter)
	}

	if newRouteTableID == "" {
		return "", fmt.Errorf("%w: RouteTableId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ReplaceRouteTableAssociation")
	defer b.mu.Unlock()

	newRT, ok := b.routeTables.Get(newRouteTableID)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrRouteTableNotFound, newRouteTableID)
	}

	// Find the old association without mutating anything yet -- a mismatched
	// found/subnetID sentinel here previously spliced out an association
	// before checking whether it was safe to move.
	var (
		oldRT    *RouteTable
		oldIndex int
		subnetID string
		found    bool
	)

	for _, rt := range b.routeTables.All() {
		for i, assoc := range rt.Associations {
			if assoc.ID == associationID {
				oldRT, oldIndex, subnetID, found = rt, i, assoc.SubnetID, true

				break
			}
		}

		if found {
			break
		}
	}

	if !found {
		return "", fmt.Errorf("%w: %s", ErrAssociationNotFound, associationID)
	}

	if subnetID == "" {
		return "", fmt.Errorf(
			"%w: %s is the implicit main-route-table association for %s; "+
				"reassigning a VPC's main route table is not supported",
			ErrInvalidParameter, associationID, oldRT.VPCID,
		)
	}

	oldRT.Associations = append(oldRT.Associations[:oldIndex], oldRT.Associations[oldIndex+1:]...)

	newAssocID := newRouteTableAssociationID()
	newRT.Associations = append(newRT.Associations, RouteAssociation{
		ID:           newAssocID,
		RouteTableID: newRouteTableID,
		SubnetID:     subnetID,
	})

	return newAssocID, nil
}

// ---- AssociateVpcCidrBlock ----

// AssociateVpcCidrBlock associates a secondary CIDR block with a VPC.
func (b *InMemoryBackend) AssociateVpcCidrBlock(
	vpcID, cidrBlock string,
) (*VpcCidrBlockAssociation, error) {
	if vpcID == "" {
		return nil, fmt.Errorf("%w: VpcId is required", ErrInvalidParameter)
	}

	b.mu.Lock("AssociateVpcCidrBlock")
	defer b.mu.Unlock()

	if _, ok := b.vpcs.Get(vpcID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrVPCNotFound, vpcID)
	}

	assoc := &VpcCidrBlockAssociation{
		AssociationID: newVPCCIDRAssociationID(),
		CidrBlock:     cidrBlock,
		State:         stateAvailable,
	}
	b.vpcCidrAssociations[vpcID+":"+assoc.AssociationID] = assoc

	cp := *assoc

	return &cp, nil
}

// ---- Transit Gateway Route Tables ----

// CreateTransitGatewayRouteTable creates a new TGW route table.
func (b *InMemoryBackend) CreateTransitGatewayRouteTable(
	tgwID string, tags map[string]string,
) (*TransitGatewayRouteTable, error) {
	if tgwID == "" {
		return nil, fmt.Errorf("%w: TransitGatewayId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateTransitGatewayRouteTable")
	defer b.mu.Unlock()

	if _, ok := b.transitGateways.Get(tgwID); !ok {
		return nil, fmt.Errorf("%w: transit gateway %s not found", ErrInvalidParameter, tgwID)
	}

	rt := &TransitGatewayRouteTable{
		RouteTableID:     newTransitGatewayRouteTableID(),
		TransitGatewayID: tgwID,
		State:            stateAvailable,
		CreateTime:       time.Now(),
	}
	b.tgwRouteTables.Put(rt)
	b.setTagsLocked(rt.RouteTableID, tags)

	cp := *rt

	return &cp, nil
}

// DescribeTransitGatewayRouteTables returns TGW route tables, optionally filtered by IDs.
func (b *InMemoryBackend) DescribeTransitGatewayRouteTables(
	ids []string,
) []*TransitGatewayRouteTable {
	b.mu.RLock("DescribeTransitGatewayRouteTables")
	defer b.mu.RUnlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	out := make([]*TransitGatewayRouteTable, 0, b.tgwRouteTables.Len())

	for _, rt := range b.tgwRouteTables.All() {
		if len(idSet) > 0 && !idSet[rt.RouteTableID] {
			continue
		}

		cp := *rt
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].RouteTableID < out[j].RouteTableID
	})

	return out
}

// DeleteTransitGatewayRouteTable removes a TGW route table.
func (b *InMemoryBackend) DeleteTransitGatewayRouteTable(id string) error {
	if id == "" {
		return fmt.Errorf("%w: TransitGatewayRouteTableId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteTransitGatewayRouteTable")
	defer b.mu.Unlock()

	if _, ok := b.tgwRouteTables.Get(id); !ok {
		return fmt.Errorf("%w: %s", ErrTGWRouteTableNotFound, id)
	}
	b.tgwRouteTables.Delete(id)
	delete(b.tags, id)

	return nil
}

// ---- Transit Gateway Routes ----

// CreateTransitGatewayRoute adds a static route to a TGW route table.
// blackhole mirrors the real CreateTransitGatewayRouteInput.Blackhole flag: a
// blackhole route has no attachment and drops matching traffic, so
// attachmentID is ignored and the route's ResourceID/ResourceType stay empty.
func (b *InMemoryBackend) CreateTransitGatewayRoute(
	routeTableID, destinationCIDR, attachmentID string,
	blackhole bool,
) (*TransitGatewayRoute, error) {
	if routeTableID == "" {
		return nil, fmt.Errorf("%w: TransitGatewayRouteTableId is required", ErrInvalidParameter)
	}

	if destinationCIDR == "" {
		return nil, fmt.Errorf("%w: DestinationCidrBlock is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateTransitGatewayRoute")
	defer b.mu.Unlock()

	if _, ok := b.tgwRouteTables.Get(routeTableID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrTGWRouteTableNotFound, routeTableID)
	}

	route := &TransitGatewayRoute{
		DestinationCidrBlock:       destinationCIDR,
		TransitGatewayRouteTableID: routeTableID,
		Type:                       tgwRouteTypeStatic,
	}

	if blackhole {
		route.State = tgwRouteStateBlackhole
	} else {
		if attachmentID == "" {
			return nil, fmt.Errorf("%w: TransitGatewayAttachmentId is required", ErrInvalidParameter)
		}

		if !b.transitGatewayAttachmentExistsLocked(attachmentID) {
			return nil, fmt.Errorf("%w: %s", ErrTGWAttachmentNotFound, attachmentID)
		}

		resourceID, resourceType := b.tgwAttachmentResourceLocked(attachmentID)
		route.TransitGatewayAttachmentID = attachmentID
		route.ResourceID = resourceID
		route.ResourceType = resourceType
		route.State = stateActive
	}

	b.tgwRoutes.Put(route)

	cp := *route

	return &cp, nil
}

// DeleteTransitGatewayRoute removes a static route from a TGW route table.
func (b *InMemoryBackend) DeleteTransitGatewayRoute(routeTableID, destinationCIDR string) error {
	if routeTableID == "" {
		return fmt.Errorf("%w: TransitGatewayRouteTableId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteTransitGatewayRoute")
	defer b.mu.Unlock()

	key := routeTableID + ":" + destinationCIDR
	if _, ok := b.tgwRoutes.Get(key); !ok {
		return fmt.Errorf(
			"%w: route %s in %s not found",
			ErrRouteNotFound,
			destinationCIDR,
			routeTableID,
		)
	}
	b.tgwRoutes.Delete(key)

	return nil
}

// ReplaceTransitGatewayRoute replaces an existing route in a TGW route table.
// Real AWS requires a route for destinationCIDR to already exist
// (InvalidRoute.NotFound otherwise) - it does not create one.
func (b *InMemoryBackend) ReplaceTransitGatewayRoute(
	routeTableID, destinationCIDR, attachmentID string,
	blackhole bool,
) (*TransitGatewayRoute, error) {
	if routeTableID == "" {
		return nil, fmt.Errorf("%w: TransitGatewayRouteTableId is required", ErrInvalidParameter)
	}

	if destinationCIDR == "" {
		return nil, fmt.Errorf("%w: DestinationCidrBlock is required", ErrInvalidParameter)
	}

	b.mu.Lock("ReplaceTransitGatewayRoute")
	defer b.mu.Unlock()

	if _, ok := b.tgwRouteTables.Get(routeTableID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrTGWRouteTableNotFound, routeTableID)
	}

	// Real AWS's Replace only ever replaces a route that already exists in
	// the target route table; it does not silently create one for an unknown
	// destination CIDR (that is CreateTransitGatewayRoute's job).
	key := routeTableID + ":" + destinationCIDR
	if _, ok := b.tgwRoutes.Get(key); !ok {
		return nil, fmt.Errorf("%w: route %s in %s not found", ErrRouteNotFound, destinationCIDR, routeTableID)
	}

	route := &TransitGatewayRoute{
		DestinationCidrBlock:       destinationCIDR,
		TransitGatewayRouteTableID: routeTableID,
		Type:                       tgwRouteTypeStatic,
	}

	if blackhole {
		route.State = tgwRouteStateBlackhole
	} else {
		if attachmentID == "" {
			return nil, fmt.Errorf("%w: TransitGatewayAttachmentId is required", ErrInvalidParameter)
		}

		if !b.transitGatewayAttachmentExistsLocked(attachmentID) {
			return nil, fmt.Errorf("%w: %s", ErrTGWAttachmentNotFound, attachmentID)
		}

		resourceID, resourceType := b.tgwAttachmentResourceLocked(attachmentID)
		route.TransitGatewayAttachmentID = attachmentID
		route.ResourceID = resourceID
		route.ResourceType = resourceType
		route.State = stateActive
	}

	b.tgwRoutes.Put(route)

	cp := *route

	return &cp, nil
}

// ---- Transit Gateway Route Table Associations ----

// AssociateTransitGatewayRouteTable associates a TGW attachment with a route table.
func (b *InMemoryBackend) AssociateTransitGatewayRouteTable(
	routeTableID, attachmentID string,
) (*TransitGatewayRouteTableAssociation, error) {
	if routeTableID == "" {
		return nil, fmt.Errorf("%w: TransitGatewayRouteTableId is required", ErrInvalidParameter)
	}

	if attachmentID == "" {
		return nil, fmt.Errorf("%w: TransitGatewayAttachmentId is required", ErrInvalidParameter)
	}

	b.mu.Lock("AssociateTransitGatewayRouteTable")
	defer b.mu.Unlock()

	if _, ok := b.tgwRouteTables.Get(routeTableID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrTGWRouteTableNotFound, routeTableID)
	}

	if !b.transitGatewayAttachmentExistsLocked(attachmentID) {
		return nil, fmt.Errorf("%w: %s", ErrTGWAttachmentNotFound, attachmentID)
	}

	// ResourceType must reflect the attachment's real kind (vpc/peering/
	// connect/client-vpn), not be hardcoded -- an association response for a
	// non-VPC attachment previously always misreported "vpc" (found during
	// the gopherstack-8pce TGW route-table field-diff, matching the same
	// real-resource-type derivation EnableTransitGatewayRouteTablePropagation
	// already uses).
	_, resourceType := b.tgwAttachmentResourceLocked(attachmentID)

	assoc := &TransitGatewayRouteTableAssociation{
		TransitGatewayRouteTableID: routeTableID,
		TransitGatewayAttachmentID: attachmentID,
		ResourceType:               resourceType,
		State:                      stateAvailable,
	}
	b.tgwRTAssociations.Put(assoc)

	cp := *assoc

	return &cp, nil
}

// DisassociateTransitGatewayRouteTable removes an association between a TGW attachment and route table.
func (b *InMemoryBackend) DisassociateTransitGatewayRouteTable(
	routeTableID, attachmentID string,
) error {
	if routeTableID == "" {
		return fmt.Errorf("%w: TransitGatewayRouteTableId is required", ErrInvalidParameter)
	}

	if attachmentID == "" {
		return fmt.Errorf("%w: TransitGatewayAttachmentId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DisassociateTransitGatewayRouteTable")
	defer b.mu.Unlock()

	key := routeTableID + ":" + attachmentID
	if _, ok := b.tgwRTAssociations.Get(key); !ok {
		return fmt.Errorf(
			"%w: association between %s and %s not found",
			ErrInvalidParameter,
			routeTableID,
			attachmentID,
		)
	}
	b.tgwRTAssociations.Delete(key)

	return nil
}
