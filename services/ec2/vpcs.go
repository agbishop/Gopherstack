package ec2

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// vpcTenancyDefault is the documented CreateVpc InstanceTenancy default
// (ec2@v1.319.1 api_op_CreateVpc.go: "Default: default").
const vpcTenancyDefault = "default"

// CreateDefaultVpc creates a new default VPC in the account.
// Returns error if a default VPC already exists.
func (b *InMemoryBackend) CreateDefaultVpc() (*VPC, error) {
	b.mu.Lock("CreateDefaultVpc")
	defer b.mu.Unlock()

	for _, v := range b.vpcs.All() {
		if v.IsDefault {
			return nil, fmt.Errorf("%w: a default VPC already exists", ErrInvalidParameter)
		}
	}

	vpc := &VPC{
		ID:        newVPCID(),
		CIDRBlock: defaultVPCCIDR,
		IsDefault: true,
	}
	b.vpcs.Put(vpc)

	return vpc, nil
}

// ---- CreateDefaultSubnet ----

// ModifyVpcTenancy sets the instance tenancy for a VPC ("default" or "dedicated").
func (b *InMemoryBackend) ModifyVpcTenancy(vpcID, tenancy string) error {
	if vpcID == "" {
		return fmt.Errorf("%w: VpcId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyVpcTenancy")
	defer b.mu.Unlock()

	if _, ok := b.vpcs.Get(vpcID); !ok {
		return fmt.Errorf("%w: %s", ErrVPCNotFound, vpcID)
	}
	b.vpcTenancy[vpcID] = tenancy

	return nil
}

// VpcTenancy returns the stored instance tenancy for a VPC, defaulting to
// "default" (the documented CreateVpc default) when nothing was recorded --
// e.g. a VPC created by CreateDefaultVpc, which never accepts a tenancy.
func (b *InMemoryBackend) VpcTenancy(vpcID string) string {
	b.mu.RLock("VpcTenancy")
	defer b.mu.RUnlock()

	if t, ok := b.vpcTenancy[vpcID]; ok && t != "" {
		return t
	}

	return vpcTenancyDefault
}

// ---- ModifyVpcPeeringConnectionOptions ----

// ModifyVpcPeeringConnectionOptions updates DNS options for a VPC peering connection.
func (b *InMemoryBackend) ModifyVpcPeeringConnectionOptions(
	peeringID string,
	opts PeeringConnectionOptions,
) error {
	if peeringID == "" {
		return fmt.Errorf("%w: VpcPeeringConnectionId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyVpcPeeringConnectionOptions")
	defer b.mu.Unlock()

	if _, ok := b.vpcPeeringConnections.Get(peeringID); !ok {
		return fmt.Errorf("%w: %s", ErrVpcPeeringConnectionNotFound, peeringID)
	}
	o := opts
	b.vpcPeeringOptions[peeringID] = &o

	return nil
}

// GetVpcPeeringConnectionOptions returns stored options for a peering connection.
func (b *InMemoryBackend) GetVpcPeeringConnectionOptions(
	peeringID string,
) *PeeringConnectionOptions {
	b.mu.RLock("GetVpcPeeringConnectionOptions")
	defer b.mu.RUnlock()

	return b.vpcPeeringOptions[peeringID]
}

// ---- EIP attributes ----

// DisassociateVpcCidrBlock removes a secondary CIDR block association from a
// VPC, returning the owning VPC ID and the association as it stood at
// removal (State forced to "disassociated": this backend drops the
// association synchronously rather than modeling the real API's transient
// "disassociating" state).
func (b *InMemoryBackend) DisassociateVpcCidrBlock(associationID string) (string, *VpcCidrBlockAssociation, error) {
	if associationID == "" {
		return "", nil, fmt.Errorf("%w: AssociationId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DisassociateVpcCidrBlock")
	defer b.mu.Unlock()

	// Keys are stored as "vpcID:assocID"
	for key, assoc := range b.vpcCidrAssociations {
		if assoc.AssociationID == associationID {
			delete(b.vpcCidrAssociations, key)

			cp := *assoc
			cp.State = vpcCidrBlockStateDisassociated
			vpcID := strings.TrimSuffix(key, ":"+associationID)

			return vpcID, &cp, nil
		}
	}

	return "", nil, fmt.Errorf("%w: %s", ErrInvalidParameter, associationID)
}

// vpcCidrBlockStateDisassociated matches types.VpcCidrBlockStateCodeDisassociated
// (ec2@v1.319.1 types/enums.go).
const vpcCidrBlockStateDisassociated = "disassociated"

// ---- NAT Gateway address ops ----

// ModifyVpcAttribute enables or disables DNS support or DNS hostnames for a VPC.
func (b *InMemoryBackend) ModifyVpcAttribute(vpcID, attribute string, value bool) error {
	if vpcID == "" {
		return fmt.Errorf("%w: VpcId is required", ErrInvalidParameter)
	}

	b.mu.Lock("ModifyVpcAttribute")
	defer b.mu.Unlock()

	vpc, ok := b.vpcs.Get(vpcID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrVPCNotFound, vpcID)
	}

	switch attribute {
	case attrEnableDNSSupport, attrEnableDNSHostnames:
		if vpc.Attributes == nil {
			vpc.Attributes = make(map[string]bool)
		}
		vpc.Attributes[attribute] = value

		return nil
	default:
		return fmt.Errorf("%w: unknown VPC attribute %q", ErrInvalidParameter, attribute)
	}
}

// CreateVpcPeeringConnection creates a new pending VPC peering connection.
func (b *InMemoryBackend) CreateVpcPeeringConnection(
	requesterVPCID, accepterVPCID string,
) (*VpcPeeringConnection, error) {
	if requesterVPCID == "" || accepterVPCID == "" {
		return nil, fmt.Errorf("%w: VpcId and PeerVpcId are required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateVpcPeeringConnection")
	defer b.mu.Unlock()

	if _, ok := b.vpcs.Get(requesterVPCID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrVPCNotFound, requesterVPCID)
	}

	pc := &VpcPeeringConnection{
		VpcPeeringConnectionID: "pcx-" + uuid.New().String()[:8],
		RequesterVpcID:         requesterVPCID,
		AccepterVpcID:          accepterVPCID,
		State:                  "pending-acceptance",
	}
	b.vpcPeeringConnections.Put(pc)

	cp := *pc

	return &cp, nil
}

// DeleteVpcPeeringConnection removes a VPC peering connection.
func (b *InMemoryBackend) DeleteVpcPeeringConnection(id string) error {
	if id == "" {
		return fmt.Errorf("%w: VpcPeeringConnectionId is required", ErrInvalidParameter)
	}

	b.mu.Lock("DeleteVpcPeeringConnection")
	defer b.mu.Unlock()

	if _, ok := b.vpcPeeringConnections.Get(id); !ok {
		return fmt.Errorf("%w: peering connection %s not found", ErrInvalidParameter, id)
	}
	b.vpcPeeringConnections.Delete(id)
	delete(b.tags, id)

	return nil
}

// ---- Transit gateways ----

// Real AWS default values for TransitGatewayRequestOptions fields not
// explicitly supplied on CreateTransitGateway, per the aws-sdk-go-v2 docs on
// types.TransitGatewayRequestOptions.
const (
	tgwDefaultAmazonSideAsn                   = 64512
	tgwAutoAcceptSharedAttachmentsDisable     = "disable"
	tgwDefaultRouteTableAssociationEnable     = "enable"
	tgwDefaultRouteTablePropagationEnable     = "enable"
	tgwDNSSupportEnable                       = "enable"
	tgwMulticastSupportDisable                = "disable"
	tgwSecurityGroupReferencingSupportDisable = "disable"
	tgwVpnEcmpSupportEnable                   = "enable"
)

// TransitGatewayOptions holds the configurable options of a transit gateway,
// mirroring the real AWS TransitGatewayOptions/TransitGatewayRequestOptions
// shapes. AssociationDefaultRouteTableId / PropagationDefaultRouteTableId are
// intentionally left unset: this backend does not auto-create a default
// transit gateway route table on CreateTransitGateway, so there is no real ID
// to report (see PARITY.md).
type TransitGatewayOptions struct {
	AutoAcceptSharedAttachments     string   `json:"autoAcceptSharedAttachments,omitempty"`
	DefaultRouteTableAssociation    string   `json:"defaultRouteTableAssociation,omitempty"`
	DefaultRouteTablePropagation    string   `json:"defaultRouteTablePropagation,omitempty"`
	DNSSupport                      string   `json:"dnsSupport,omitempty"`
	MulticastSupport                string   `json:"multicastSupport,omitempty"`
	SecurityGroupReferencingSupport string   `json:"securityGroupReferencingSupport,omitempty"`
	VpnEcmpSupport                  string   `json:"vpnEcmpSupport,omitempty"`
	TransitGatewayCidrBlocks        []string `json:"transitGatewayCidrBlocks,omitempty"`
	AmazonSideAsn                   int64    `json:"amazonSideAsn,omitempty"`
}

// TransitGateway represents an AWS Transit Gateway resource.
type TransitGateway struct {
	CreationTime time.Time             `json:"creationTime"`
	ID           string                `json:"transitGatewayId,omitempty"`
	Arn          string                `json:"transitGatewayArn,omitempty"`
	Description  string                `json:"description,omitempty"`
	State        string                `json:"state,omitempty"`
	OwnerID      string                `json:"ownerId,omitempty"`
	Options      TransitGatewayOptions `json:"options"`
}

// DescribeVpcs returns VPCs, optionally filtered by IDs.
// When ids are provided, lookups are O(len(ids)) via the VPC map rather than
// scanning every VPC in the backend.
func (b *InMemoryBackend) DescribeVpcs(ids []string) []*VPC {
	b.mu.RLock("DescribeVpcs")
	defer b.mu.RUnlock()

	if len(ids) > 0 {
		out := make([]*VPC, 0, len(ids))

		for _, id := range ids {
			v, ok := b.vpcs.Get(id)
			if !ok {
				continue
			}

			cp := *v
			out = append(out, &cp)
		}

		return out
	}

	out := make([]*VPC, 0, b.vpcs.Len())

	for _, v := range b.vpcs.All() {
		cp := *v
		out = append(out, &cp)
	}

	return out
}

// CreateVpc creates a new VPC with the given CIDR block and instance
// tenancy ("default" or "dedicated"; ec2@v1.319.1 api_op_CreateVpc.go
// documents InstanceTenancy's default as "default"). Matching real AWS,
// a default security group named "default" and a main route table (with a
// local route for cidr) are created and stored for the new VPC. The default
// network ACL is not stored here -- DescribeNetworkAcls derives it per-VPC
// instead (do not add a stored duplicate).
func (b *InMemoryBackend) CreateVpc(cidr, tenancy string) (*VPC, error) {
	if cidr == "" {
		return nil, fmt.Errorf("%w: CidrBlock is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateVpc")
	defer b.mu.Unlock()

	for _, existing := range b.vpcs.All() {
		if cidrsOverlap(cidr, existing.CIDRBlock) {
			return nil, fmt.Errorf("%w: CIDR %s overlaps with existing VPC %s (%s)",
				ErrCIDRConflict, cidr, existing.ID, existing.CIDRBlock)
		}
	}

	id := newVPCID()
	v := &VPC{
		ID:        id,
		CIDRBlock: cidr,
	}
	b.vpcs.Put(v)
	b.vpcTenancy[id] = tenancy

	sgID := newSecurityGroupID()
	b.securityGroups.Put(&SecurityGroup{
		ID:          sgID,
		Name:        defaultSecurityGroupName,
		Description: defaultSecurityGroupDescription,
		VPCID:       id,
		EgressRules: []SecurityGroupRule{
			{Protocol: "-1", IPRange: cidrAllIPv4},
		},
	})
	b.indexSGLocked(sgID, id)

	rtID := newRouteTableID()
	b.routeTables.Put(&RouteTable{
		ID:    rtID,
		VPCID: id,
		Main:  true,
		Routes: []Route{
			{DestinationCIDR: cidr, GatewayID: routeGatewayLocal, State: stateActive},
		},
		Associations: []RouteAssociation{
			{ID: newRouteTableAssociationID(), RouteTableID: rtID, Main: true},
		},
	})
	b.indexRouteTableLocked(rtID, id)

	return v, nil
}

// vpcDependencyViolationLocked returns a DependencyViolation error naming the
// first dependent resource found for vpcID, or nil if the VPC has no
// dependents blocking deletion. Mirrors real AWS: the default security group
// and the main route table are auto-deleted with the VPC and do not block
// deletion; every other VPC-scoped resource (subnets, non-default security
// groups, non-main route tables, attached internet/egress-only gateways, NAT
// gateways, non-default network ACLs, VPC endpoints) must be removed first.
// Must be called with b.mu held.
func (b *InMemoryBackend) vpcDependencyViolationLocked(vpcID string) error {
	if err := b.vpcIndexedDependencyViolationLocked(vpcID); err != nil {
		return err
	}

	return b.vpcScannedDependencyViolationLocked(vpcID)
}

// vpcIndexedDependencyViolationLocked checks the dependents that are tracked
// via the per-VPC secondary indexes (subnets, non-default security groups,
// route tables, NAT gateways) — an O(1) map-length check per resource type.
// Must be called with b.mu held.
func (b *InMemoryBackend) vpcIndexedDependencyViolationLocked(vpcID string) error {
	if len(b.subnetIDsByVPC[vpcID]) > 0 {
		return fmt.Errorf("%w: the vpc %s has dependencies (subnets) and cannot be deleted",
			ErrDependencyViolation, vpcID)
	}

	for sgID := range b.sgIDsByVPC[vpcID] {
		sg, ok := b.securityGroups.Get(sgID)
		if ok && sg.Name != defaultSecurityGroupName {
			return fmt.Errorf(
				"%w: the vpc %s has dependencies (security group %s) and cannot be deleted",
				ErrDependencyViolation, vpcID, sgID,
			)
		}
	}

	for rtID := range b.routeTableIDsByVPC[vpcID] {
		rt, ok := b.routeTables.Get(rtID)
		if ok && !rt.Main {
			return fmt.Errorf(
				"%w: the vpc %s has dependencies (route table %s) and cannot be deleted",
				ErrDependencyViolation, vpcID, rtID,
			)
		}
	}

	if len(b.natGatewayIDsByVPC[vpcID]) > 0 {
		return fmt.Errorf("%w: the vpc %s has dependencies (NAT gateways) and cannot be deleted",
			ErrDependencyViolation, vpcID)
	}

	return nil
}

// vpcScannedDependencyViolationLocked checks the dependents that have no
// per-VPC secondary index and so require a full-table scan (internet
// gateways, egress-only internet gateways, network ACLs, VPC endpoints).
// Must be called with b.mu held.
func (b *InMemoryBackend) vpcScannedDependencyViolationLocked(vpcID string) error {
	for _, igw := range b.internetGateways.All() {
		for _, att := range igw.Attachments {
			if att.VPCID == vpcID {
				return fmt.Errorf(
					"%w: the vpc %s has dependencies (internet gateway %s) and cannot be deleted",
					ErrDependencyViolation, vpcID, internetGatewaysKeyFn(igw),
				)
			}
		}
	}

	for _, eigw := range b.egressOnlyIGWs.All() {
		if eigw.VPCID == vpcID {
			return fmt.Errorf(
				"%w: the vpc %s has dependencies (egress-only internet gateway %s) "+
					"and cannot be deleted",
				ErrDependencyViolation, vpcID, eigw.ID,
			)
		}
	}

	for _, acl := range b.networkACLs.All() {
		if acl.VPCID == vpcID {
			return fmt.Errorf(
				"%w: the vpc %s has dependencies (network ACL %s) and cannot be deleted",
				ErrDependencyViolation, vpcID, acl.ID,
			)
		}
	}

	for _, ep := range b.vpcEndpoints.All() {
		if ep.VPCID == vpcID {
			return fmt.Errorf(
				"%w: the vpc %s has dependencies (VPC endpoint %s) and cannot be deleted",
				ErrDependencyViolation, vpcID, ep.ID,
			)
		}
	}

	return nil
}

// DeleteVpc removes a VPC by ID. Matching real AWS (ec2@v1.319.1
// api_op_DeleteVpc.go:16: "it deletes the default security group, network
// ACL, and route table for the VPC"), this fails with DependencyViolation if
// the VPC still has other dependent resources — it does NOT cascade-delete
// them. The default security group and main route table are the sole
// exceptions: like AWS, both are deleted automatically along with the VPC.
func (b *InMemoryBackend) DeleteVpc(id string) error {
	b.mu.Lock("DeleteVpc")
	defer b.mu.Unlock()

	if _, ok := b.vpcs.Get(id); !ok {
		return fmt.Errorf("%w: %s", ErrVPCNotFound, id)
	}

	if err := b.vpcDependencyViolationLocked(id); err != nil {
		return err
	}

	// The default security group is auto-deleted with the VPC (it never blocks
	// deletion — see vpcDependencyViolationLocked).
	for sgID := range b.sgIDsByVPC[id] {
		b.securityGroups.Delete(sgID)
		delete(b.tags, sgID)
		delete(b.sgVpcAssociations, sgID)
	}
	delete(b.sgIDsByVPC, id)

	// The main route table is auto-deleted with the VPC too (also never
	// blocks deletion — see vpcDependencyViolationLocked).
	for rtID := range b.routeTableIDsByVPC[id] {
		if rt, ok := b.routeTables.Get(rtID); ok && rt.Main {
			b.routeTables.Delete(rtID)
			delete(b.tags, rtID)
		}
	}
	delete(b.routeTableIDsByVPC, id)

	b.vpcs.Delete(id)
	delete(b.tags, id)

	return nil
}
