package networkmanager

import (
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// This file implements PARITY.md families G-J: four Global-Networks
// association kinds binding a Device/Link to something else --
// CustomerGatewayAssociation (an EC2 CustomerGateway ARN), TransitGateway
// Registration (an EC2 TransitGateway ARN, scoped to the GlobalNetwork
// rather than a Device/Link), TransitGatewayConnectPeerAssociation (an EC2
// TransitGatewayConnectPeer ARN), and ConnectPeerAssociation (a Cloud WAN
// ConnectPeer -- the one real structural bridge between this service's two
// product halves, PARITY.md family J).
//
// Cross-service validation: CustomerGatewayArn/TransitGatewayArn/
// TransitGatewayConnectPeerArn are validated against services/ec2's real
// state via EC2Resolver (crossservice.go) when cli.go's
// wireNetworkManagerEC2 has wired one in; a nil resolver (isolated unit
// tests) accepts any non-empty ARN, matching this package's original scope
// decision. ConnectPeerId, by contrast, names a resource this OWN package
// creates (family K) and is always validated below.

// requireLinkedDevice reports a ResourceNotFoundException unless deviceID
// names a real device belonging to globalNetworkID. Callers must hold b.mu.
func (b *InMemoryBackend) requireLinkedDevice(globalNetworkID, deviceID string) error {
	if d, ok := b.devices.Get(deviceID); !ok || d.GlobalNetworkID != globalNetworkID {
		return notFoundError(resourceDevice, deviceID)
	}

	return nil
}

// requireCrossServiceARN validates arnValue is non-empty and, when resolve
// is non-nil (an EC2Resolver/DirectConnectResolver method wired in),
// resolves against real cross-service state -- the shared shape behind
// AssociateCustomerGateway/AssociateTransitGatewayConnectPeer's identical
// validate-then-store flow.
func requireCrossServiceARN(arnValue, fieldName string, resolve func(string) bool, notFoundResourceType string) error {
	if arnValue == "" {
		return validationError(fieldName + " is required")
	}

	if resolve != nil && !resolve(arnValue) {
		return notFoundError(notFoundResourceType, arnValue)
	}

	return nil
}

// associateDeviceLinkedResource is the shared shape behind
// AssociateCustomerGateway/AssociateTransitGatewayConnectPeer: validate
// deviceID belongs to globalNetworkID, validate/resolve the cross-service
// ARN, require an already-associated link when the caller specified one,
// store the caller-built PENDING association, and schedule its real
// PENDING->AVAILABLE advance.
func associateDeviceLinkedResource[T any, PT interface {
	*T
	clone() *T
}](
	b *InMemoryBackend,
	label, globalNetworkID, deviceID, arnValue, fieldName, linkID string,
	resolve func(string) bool, notFoundResourceType string,
	table *store.Table[T], key string,
	build func() PT,
	statePtr func(*T) *string,
) (*T, error) {
	if err := b.requireLinkedDevice(globalNetworkID, deviceID); err != nil {
		return nil, err
	}

	if err := requireCrossServiceARN(arnValue, fieldName, resolve, notFoundResourceType); err != nil {
		return nil, err
	}

	// Both AssociateCustomerGateway and AssociateTransitGatewayConnectPeer
	// document: "If you specify a link, it must be associated with the
	// specified device."
	if linkID != "" {
		if _, ok := b.linkAssociations.Get(linkAssociationKey(globalNetworkID, deviceID, linkID)); !ok {
			return nil, conflictError(resourceLink, linkID, "link is not associated with device "+deviceID)
		}
	}

	v := build()
	table.Put(v)
	scheduleAdvance(b, label, table, key, statePtr, assocStatePending, assocStateAvailable)

	return v.clone(), nil
}

// ---- Customer Gateway Association ----

func (b *InMemoryBackend) AssociateCustomerGateway(
	globalNetworkID, customerGatewayArn, deviceID, linkID string,
) (*CustomerGatewayAssociation, error) {
	b.mu.Lock("AssociateCustomerGateway")
	defer b.mu.Unlock()

	var resolve func(string) bool
	if b.ec2Resolver != nil {
		resolve = b.ec2Resolver.ResolveCustomerGateway
	}

	return associateDeviceLinkedResource(
		b, "CustomerGatewayAssociationAvailable", globalNetworkID, deviceID, customerGatewayArn,
		"CustomerGatewayArn", linkID, resolve, resourceEC2CustomerGateway,
		b.customerGatewayAssociations, customerGatewayAssociationKey(globalNetworkID, customerGatewayArn),
		func() *CustomerGatewayAssociation {
			return &CustomerGatewayAssociation{
				CustomerGatewayArn: customerGatewayArn, DeviceID: deviceID, GlobalNetworkID: globalNetworkID,
				LinkID: linkID, State: assocStatePending,
			}
		},
		func(v *CustomerGatewayAssociation) *string { return &v.State },
	)
}

func (b *InMemoryBackend) DisassociateCustomerGateway(
	globalNetworkID, customerGatewayArn string,
) (*CustomerGatewayAssociation, error) {
	b.mu.Lock("DisassociateCustomerGateway")
	defer b.mu.Unlock()

	key := customerGatewayAssociationKey(globalNetworkID, customerGatewayArn)

	a, ok := b.customerGatewayAssociations.Get(key)
	if !ok {
		return nil, notFoundError(resourceCustomerGwAssn, key)
	}

	a.State = stateDeleting
	scheduleRemoval(b, "CustomerGatewayAssociationDeleted", b.customerGatewayAssociations, key)

	return a.clone(), nil
}

func (b *InMemoryBackend) GetCustomerGatewayAssociations(
	globalNetworkID string, arns []string, token string, limit int,
) (page.Page[*CustomerGatewayAssociation], error) {
	b.mu.RLock("GetCustomerGatewayAssociations")
	defer b.mu.RUnlock()

	all := filterByGlobalNetwork(
		b.customerGatewayAssociations.Snapshot(), globalNetworkID,
		func(a *CustomerGatewayAssociation) string { return a.GlobalNetworkID },
	)
	if len(arns) > 0 {
		all = filterByIDs(all, arns, func(a *CustomerGatewayAssociation) string { return a.CustomerGatewayArn })
	}

	p := sortAndPage(
		all, func(a *CustomerGatewayAssociation) string { return a.CustomerGatewayArn },
		(*CustomerGatewayAssociation).clone, token, limit,
	)

	return p, nil
}

// ---- Transit Gateway Registration ----

func (b *InMemoryBackend) RegisterTransitGateway(
	globalNetworkID, transitGatewayArn string,
) (*TransitGatewayRegistration, error) {
	b.mu.Lock("RegisterTransitGateway")
	defer b.mu.Unlock()

	if !b.globalNetworkExists(globalNetworkID) {
		return nil, notFoundError(resourceGlobalNetwork, globalNetworkID)
	}

	if transitGatewayArn == "" {
		return nil, validationError("TransitGatewayArn is required")
	}

	if b.ec2Resolver != nil && !b.ec2Resolver.ResolveTransitGateway(transitGatewayArn) {
		return nil, notFoundError(resourceEC2TransitGateway, transitGatewayArn)
	}

	r := &TransitGatewayRegistration{
		GlobalNetworkID: globalNetworkID, TransitGatewayArn: transitGatewayArn,
		State: &TransitGatewayRegistrationStateReason{Code: tgwRegStatePending},
	}
	b.transitGatewayRegistrations.Put(r)

	key := transitGatewayRegistrationKey(globalNetworkID, transitGatewayArn)
	b.work.After("TransitGatewayRegistrationAvailable", asyncTransitionDelay, func() {
		b.mu.Lock("TransitGatewayRegistrationAvailable-async")
		defer b.mu.Unlock()

		if v, ok := b.transitGatewayRegistrations.Get(key); ok && v.State != nil && v.State.Code == tgwRegStatePending {
			v.State.Code = tgwRegStateAvailable
		}
	})

	return r.clone(), nil
}

// DeregisterTransitGateway cascades to CustomerGatewayAssociations, matching the real op's doc
// (api_op_DeregisterTransitGateway.go): "This action removes any customer gateway associations."
// (gopherstack-3fkj).
func (b *InMemoryBackend) DeregisterTransitGateway(
	globalNetworkID, transitGatewayArn string,
) (*TransitGatewayRegistration, error) {
	b.mu.Lock("DeregisterTransitGateway")
	defer b.mu.Unlock()

	key := transitGatewayRegistrationKey(globalNetworkID, transitGatewayArn)

	r, ok := b.transitGatewayRegistrations.Get(key)
	if !ok {
		return nil, notFoundError(resourceTgwReg, key)
	}

	if r.State == nil {
		r.State = &TransitGatewayRegistrationStateReason{}
	}

	r.State.Code = tgwRegStateDeleting
	scheduleRemoval(b, "TransitGatewayRegistrationDeleted", b.transitGatewayRegistrations, key)

	b.cascadeDeregisterCustomerGatewayAssociations(globalNetworkID, transitGatewayArn)

	return r.clone(), nil
}

// cascadeDeregisterCustomerGatewayAssociations transitions to DELETING (matching
// DisassociateCustomerGateway's own PENDING->DELETING->gone pattern, not a hard delete) every
// CustomerGatewayAssociation whose customer gateway's real EC2 VpnConnection points at
// transitGatewayArn -- the link AssociateCustomerGateway's doc says AWS uses (see
// CustomerGatewayArnsForTransitGateway's doc, crossservice.go). A nil ec2Resolver -- the default
// in isolated unit tests and any backend cli.go hasn't wired yet -- leaves every association
// untouched, matching this package's existing nil-resolver no-op convention throughout
// associations.go/globalnetworks.go/peerings.go. Callers must hold b.mu.
func (b *InMemoryBackend) cascadeDeregisterCustomerGatewayAssociations(globalNetworkID, transitGatewayArn string) {
	if b.ec2Resolver == nil {
		return
	}

	for _, cgwArn := range b.ec2Resolver.CustomerGatewayArnsForTransitGateway(transitGatewayArn) {
		key := customerGatewayAssociationKey(globalNetworkID, cgwArn)

		a, ok := b.customerGatewayAssociations.Get(key)
		if !ok || a.State == stateDeleting {
			continue
		}

		a.State = stateDeleting
		scheduleRemoval(b, "CustomerGatewayAssociationDeleted", b.customerGatewayAssociations, key)
	}
}

func (b *InMemoryBackend) GetTransitGatewayRegistrations(
	globalNetworkID string, arns []string, token string, limit int,
) (page.Page[*TransitGatewayRegistration], error) {
	b.mu.RLock("GetTransitGatewayRegistrations")
	defer b.mu.RUnlock()

	all := filterByGlobalNetwork(
		b.transitGatewayRegistrations.Snapshot(), globalNetworkID,
		func(r *TransitGatewayRegistration) string { return r.GlobalNetworkID },
	)
	if len(arns) > 0 {
		all = filterByIDs(all, arns, func(r *TransitGatewayRegistration) string { return r.TransitGatewayArn })
	}

	p := sortAndPage(
		all, func(r *TransitGatewayRegistration) string { return r.TransitGatewayArn },
		(*TransitGatewayRegistration).clone, token, limit,
	)

	return p, nil
}

// ---- Transit Gateway Connect Peer Association ----

func (b *InMemoryBackend) AssociateTransitGatewayConnectPeer(
	globalNetworkID, deviceID, transitGatewayConnectPeerArn, linkID string,
) (*TransitGatewayConnectPeerAssociation, error) {
	b.mu.Lock("AssociateTransitGatewayConnectPeer")
	defer b.mu.Unlock()

	var resolve func(string) bool
	if b.ec2Resolver != nil {
		resolve = b.ec2Resolver.ResolveTransitGatewayConnectPeer
	}

	return associateDeviceLinkedResource(
		b, "TransitGatewayConnectPeerAssociationAvailable", globalNetworkID, deviceID, transitGatewayConnectPeerArn,
		"TransitGatewayConnectPeerArn", linkID, resolve, resourceEC2TransitGatewayConnectPeer,
		b.transitGatewayConnectPeerAssociations,
		transitGatewayConnectPeerAssociationKey(globalNetworkID, transitGatewayConnectPeerArn),
		func() *TransitGatewayConnectPeerAssociation {
			return &TransitGatewayConnectPeerAssociation{
				DeviceID: deviceID, GlobalNetworkID: globalNetworkID, LinkID: linkID,
				TransitGatewayConnectPeerArn: transitGatewayConnectPeerArn, State: assocStatePending,
			}
		},
		func(v *TransitGatewayConnectPeerAssociation) *string { return &v.State },
	)
}

func (b *InMemoryBackend) DisassociateTransitGatewayConnectPeer(
	globalNetworkID, transitGatewayConnectPeerArn string,
) (*TransitGatewayConnectPeerAssociation, error) {
	b.mu.Lock("DisassociateTransitGatewayConnectPeer")
	defer b.mu.Unlock()

	key := transitGatewayConnectPeerAssociationKey(globalNetworkID, transitGatewayConnectPeerArn)

	a, ok := b.transitGatewayConnectPeerAssociations.Get(key)
	if !ok {
		return nil, notFoundError(resourceTgwCpAssn, key)
	}

	a.State = stateDeleting
	scheduleRemoval(b, "TransitGatewayConnectPeerAssociationDeleted", b.transitGatewayConnectPeerAssociations, key)

	return a.clone(), nil
}

func (b *InMemoryBackend) GetTransitGatewayConnectPeerAssociations(
	globalNetworkID string, arns []string, token string, limit int,
) (page.Page[*TransitGatewayConnectPeerAssociation], error) {
	b.mu.RLock("GetTransitGatewayConnectPeerAssociations")
	defer b.mu.RUnlock()

	all := filterByGlobalNetwork(
		b.transitGatewayConnectPeerAssociations.Snapshot(), globalNetworkID,
		func(a *TransitGatewayConnectPeerAssociation) string { return a.GlobalNetworkID },
	)
	if len(arns) > 0 {
		all = filterByIDs(
			all, arns, func(a *TransitGatewayConnectPeerAssociation) string { return a.TransitGatewayConnectPeerArn },
		)
	}

	p := sortAndPage(
		all, func(a *TransitGatewayConnectPeerAssociation) string { return a.TransitGatewayConnectPeerArn },
		(*TransitGatewayConnectPeerAssociation).clone, token, limit,
	)

	return p, nil
}

// ---- Connect Peer <-> Global Network association (the concrete bridge) ----

func (b *InMemoryBackend) AssociateConnectPeer(
	globalNetworkID, connectPeerID, deviceID, linkID string,
) (*ConnectPeerAssociation, error) {
	b.mu.Lock("AssociateConnectPeer")
	defer b.mu.Unlock()

	if d, ok := b.devices.Get(deviceID); !ok || d.GlobalNetworkID != globalNetworkID {
		return nil, notFoundError(resourceDevice, deviceID)
	}

	if _, ok := b.connectPeers.Get(connectPeerID); !ok {
		return nil, notFoundError(resourceConnectPeer, connectPeerID)
	}

	// AssociateConnectPeer doc (api_op_AssociateConnectPeer.go): "If you
	// specify a link, it must be associated with the specified device."
	if linkID != "" {
		if _, ok := b.linkAssociations.Get(linkAssociationKey(globalNetworkID, deviceID, linkID)); !ok {
			return nil, conflictError(resourceLink, linkID, "link is not associated with device "+deviceID)
		}
	}

	a := &ConnectPeerAssociation{
		ConnectPeerID: connectPeerID, DeviceID: deviceID, GlobalNetworkID: globalNetworkID, LinkID: linkID,
		State: assocStatePending,
	}
	b.connectPeerAssociations.Put(a)

	key := connectPeerAssociationKey(globalNetworkID, connectPeerID)
	scheduleAdvance(b, "ConnectPeerAssociationAvailable", b.connectPeerAssociations, key,
		func(v *ConnectPeerAssociation) *string { return &v.State }, assocStatePending, assocStateAvailable)

	return a.clone(), nil
}

func (b *InMemoryBackend) DisassociateConnectPeer(
	globalNetworkID, connectPeerID string,
) (*ConnectPeerAssociation, error) {
	b.mu.Lock("DisassociateConnectPeer")
	defer b.mu.Unlock()

	key := connectPeerAssociationKey(globalNetworkID, connectPeerID)

	a, ok := b.connectPeerAssociations.Get(key)
	if !ok {
		return nil, notFoundError(resourceConnectPeerAsn, key)
	}

	a.State = stateDeleting
	scheduleRemoval(b, "ConnectPeerAssociationDeleted", b.connectPeerAssociations, key)

	return a.clone(), nil
}

func (b *InMemoryBackend) GetConnectPeerAssociations(
	globalNetworkID string, ids []string, token string, limit int,
) (page.Page[*ConnectPeerAssociation], error) {
	b.mu.RLock("GetConnectPeerAssociations")
	defer b.mu.RUnlock()

	all := filterByGlobalNetwork(
		b.connectPeerAssociations.Snapshot(), globalNetworkID,
		func(a *ConnectPeerAssociation) string { return a.GlobalNetworkID },
	)
	if len(ids) > 0 {
		all = filterByIDs(all, ids, func(a *ConnectPeerAssociation) string { return a.ConnectPeerID })
	}

	p := sortAndPage(
		all, func(a *ConnectPeerAssociation) string { return a.ConnectPeerID }, (*ConnectPeerAssociation).clone,
		token, limit,
	)

	return p, nil
}
