package ec2

import (
	"errors"
	"fmt"
)

// Errors for route table operations.
var (
	ErrRouteTableNotFound = errors.New("InvalidRouteTableID.NotFound")
	ErrRouteNotFound      = errors.New("InvalidRoute.NotFound")
)

// routeGatewayLocal is the fixed GatewayId AWS assigns to a route table's
// local route (ec2@v1.319.1 api_op_ReplaceRoute.go:77 documents resetting
// "the local route to its default target ( local )").
const routeGatewayLocal = "local"

// Route represents a route table entry.
type Route struct {
	DestinationCIDR string `json:"destinationCIDR,omitempty"`
	GatewayID       string `json:"gatewayID,omitempty"`
	NatGatewayID    string `json:"natGatewayID,omitempty"`
	State           string `json:"state,omitempty"`
}

// RouteAssociation represents an association between a route table and a
// subnet, or -- when SubnetID is empty and Main is true -- the implicit
// VPC-wide association every main route table carries (ec2@v1.319.1
// types.RouteTableAssociation: "A subnet ID is not returned for an implicit
// association").
type RouteAssociation struct {
	ID           string `json:"id,omitempty"`
	RouteTableID string `json:"routeTableID,omitempty"`
	SubnetID     string `json:"subnetID,omitempty"`
	Main         bool   `json:"main,omitempty"`
}

// RouteTable represents an EC2 Route Table. Main marks the VPC's main route
// table -- the discriminator vpcDependencyViolationLocked and DeleteVpc use
// to carve it out of the normal route-table dependency check (it is deleted
// automatically with the VPC, like the default security group).
type RouteTable struct {
	ID           string             `json:"id,omitempty"`
	VPCID        string             `json:"vpcID,omitempty"`
	Routes       []Route            `json:"routes,omitempty"`
	Associations []RouteAssociation `json:"associations,omitempty"`
	Main         bool               `json:"main,omitempty"`
}

// CreateRouteTable creates a new route table in a VPC.
func (b *InMemoryBackend) CreateRouteTable(vpcID string) (*RouteTable, error) {
	b.mu.Lock("CreateRouteTable")
	defer b.mu.Unlock()

	if _, ok := b.vpcs.Get(vpcID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrVPCNotFound, vpcID)
	}

	id := newRouteTableID()
	rt := &RouteTable{
		ID:           id,
		VPCID:        vpcID,
		Routes:       []Route{},
		Associations: []RouteAssociation{},
	}
	b.routeTables.Put(rt)
	b.indexRouteTableLocked(id, vpcID)

	return rt, nil
}

// DeleteRouteTable removes a route table. Matching real AWS, this fails with
// DependencyViolation while the route table still has subnet associations —
// callers must DisassociateRouteTable first.
func (b *InMemoryBackend) DeleteRouteTable(id string) error {
	b.mu.Lock("DeleteRouteTable")
	defer b.mu.Unlock()

	rt, ok := b.routeTables.Get(id)
	if !ok {
		return fmt.Errorf("%w: %s", ErrRouteTableNotFound, id)
	}

	if rt.Main {
		return fmt.Errorf(
			"%w: %s is the main route table for %s and cannot be deleted",
			ErrDependencyViolation, id, rt.VPCID,
		)
	}

	if len(rt.Associations) > 0 {
		return fmt.Errorf(
			"%w: the routeTable %s has dependencies (subnet associations) and cannot be deleted",
			ErrDependencyViolation, id,
		)
	}

	b.deindexRouteTableLocked(id, rt.VPCID)
	b.routeTables.Delete(id)
	delete(b.tags, id)

	return nil
}

// DescribeRouteTables returns route tables, optionally filtered by IDs.
// When ids are provided, lookups are O(len(ids)) via the route-table map
// rather than scanning every route table in the backend.
func (b *InMemoryBackend) DescribeRouteTables(ids []string) []*RouteTable {
	b.mu.RLock("DescribeRouteTables")
	defer b.mu.RUnlock()

	if len(ids) > 0 {
		out := make([]*RouteTable, 0, len(ids))

		for _, id := range ids {
			rt, ok := b.routeTables.Get(id)
			if !ok {
				continue
			}

			cp := *rt
			out = append(out, &cp)
		}

		return out
	}

	out := make([]*RouteTable, 0, b.routeTables.Len())

	for _, rt := range b.routeTables.All() {
		cp := *rt
		out = append(out, &cp)
	}

	return out
}

// CreateRoute adds a route to a route table.
func (b *InMemoryBackend) CreateRoute(rtID, destCIDR, gatewayID, natGatewayID string) error {
	b.mu.Lock("CreateRoute")
	defer b.mu.Unlock()

	rt, ok := b.routeTables.Get(rtID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrRouteTableNotFound, rtID)
	}

	rt.Routes = append(rt.Routes, Route{
		DestinationCIDR: destCIDR,
		GatewayID:       gatewayID,
		NatGatewayID:    natGatewayID,
		State:           stateActive,
	})

	return nil
}

// DeleteRoute removes a route from a route table by destination CIDR.
func (b *InMemoryBackend) DeleteRoute(rtID, destCIDR string) error {
	b.mu.Lock("DeleteRoute")
	defer b.mu.Unlock()

	rt, ok := b.routeTables.Get(rtID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrRouteTableNotFound, rtID)
	}

	for i, r := range rt.Routes {
		if r.DestinationCIDR == destCIDR {
			rt.Routes = append(rt.Routes[:i], rt.Routes[i+1:]...)

			return nil
		}
	}

	return fmt.Errorf("%w: no route with destination %s in %s", ErrRouteNotFound, destCIDR, rtID)
}

// AssociateRouteTable associates a route table with a subnet.
func (b *InMemoryBackend) AssociateRouteTable(rtID, subnetID string) (string, error) {
	b.mu.Lock("AssociateRouteTable")
	defer b.mu.Unlock()

	rt, ok := b.routeTables.Get(rtID)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrRouteTableNotFound, rtID)
	}

	if _, subnetOK := b.subnets.Get(subnetID); !subnetOK {
		return "", fmt.Errorf("%w: %s", ErrSubnetNotFound, subnetID)
	}

	assocID := newRouteTableAssociationID()
	rt.Associations = append(rt.Associations, RouteAssociation{
		ID:           assocID,
		RouteTableID: rtID,
		SubnetID:     subnetID,
	})

	return assocID, nil
}

// DisassociateRouteTable removes a route table association.
func (b *InMemoryBackend) DisassociateRouteTable(assocID string) error {
	b.mu.Lock("DisassociateRouteTable")
	defer b.mu.Unlock()

	for _, rt := range b.routeTables.All() {
		for i, assoc := range rt.Associations {
			if assoc.ID == assocID {
				if assoc.Main {
					return fmt.Errorf(
						"%w: %s is the implicit main-route-table association for %s and cannot be disassociated",
						ErrInvalidParameter, assocID, rt.VPCID,
					)
				}

				rt.Associations = append(rt.Associations[:i], rt.Associations[i+1:]...)

				return nil
			}
		}
	}

	return fmt.Errorf("%w: %s", ErrAssociationNotFound, assocID)
}

// ReplaceRoute replaces an existing route in a route table.
func (b *InMemoryBackend) ReplaceRoute(rtID, destCIDR, gatewayID, natGatewayID string) error {
	if rtID == "" || destCIDR == "" {
		return fmt.Errorf(
			"%w: RouteTableId and DestinationCidrBlock are required",
			ErrInvalidParameter,
		)
	}

	b.mu.Lock("ReplaceRoute")
	defer b.mu.Unlock()

	rt, ok := b.routeTables.Get(rtID)
	if !ok {
		return fmt.Errorf("%w: route table %s not found", ErrInvalidParameter, rtID)
	}

	for i, route := range rt.Routes {
		if route.DestinationCIDR == destCIDR {
			rt.Routes[i] = Route{
				DestinationCIDR: destCIDR,
				GatewayID:       gatewayID,
				NatGatewayID:    natGatewayID,
				State:           stateActive,
			}

			return nil
		}
	}

	return fmt.Errorf("%w: route %s not found in %s", ErrInvalidParameter, destCIDR, rtID)
}

// ---- RegisterInstanceEventNotificationAttributes ----
