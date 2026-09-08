package directconnect

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/ptrconv"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// connectionsByLagLocked returns every Connection whose LagID is lagID, in
// unspecified order. Callers must hold b.mu. See models.go's Lag doc
// comment for why member connections are looked up dynamically rather than
// stored as a nested slice.
func (b *InMemoryBackend) connectionsByLagLocked(lagID string) []*Connection {
	if lagID == "" {
		return nil
	}

	var out []*Connection

	for _, c := range b.connections.All() {
		if c.LagID == lagID {
			out = append(out, c)
		}
	}

	return out
}

// CreateConnection creates a new standard (non-hosted) Connection, starting
// in the "requested" state and auto-progressing to "pending" then
// "available" -- see PARITY.md's ConnectionState notes.
func (b *InMemoryBackend) CreateConnection(req *createConnectionRequest) (*Connection, error) {
	if req.Bandwidth == "" || req.ConnectionName == "" || req.Location == "" {
		return nil, clientError("bandwidth, connectionName, and location are required")
	}

	if err := validateNewTags(tagWireKeys(req.Tags)); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateConnection")
	defer b.mu.Unlock()

	if req.LagID != "" {
		if _, ok := b.lags.Get(req.LagID); !ok {
			return nil, notFoundError(resourceLag, req.LagID)
		}
	}

	id := newConnectionID()
	t := tags.New("directconnect.connection." + id + ".tags")
	t.Merge(tagWireToMap(req.Tags))

	c := &Connection{
		ConnectionID:    id,
		ConnectionName:  req.ConnectionName,
		ConnectionState: ConnectionStateRequested,
		Bandwidth:       req.Bandwidth,
		Location:        req.Location,
		Region:          b.region,
		LagID:           req.LagID,
		ProviderName:    req.ProviderName,
		OwnerAccount:    b.accountID,
		MacSecCapable:   ptrconv.Bool(req.RequestMACSec),
		Tags:            t,
	}
	b.connections.Put(c)

	b.scheduleTransition(
		"connection:"+id,
		[]string{ConnectionStatePending, ConnectionStateAvailable},
		&c.ConnectionState,
	)

	return c.clone(), nil
}

// resolveHostLocked resolves a "hosting connection or LAG" id (per
// AllocateHostedConnection/AssociateHostedConnection's shared doc comment)
// to whichever kind it names, reporting which via isLag. Callers must hold
// b.mu.
func (b *InMemoryBackend) resolveHostLocked(id string) (bool, bool) {
	if _, found := b.lags.Get(id); found {
		return true, true
	}

	if _, found := b.connections.Get(id); found {
		return false, true
	}

	return false, false
}

// AllocateConnectionOnInterconnect creates a Connection hosted on an
// existing Interconnect for a named end-customer OwnerAccount -- the
// partner/reseller flow, real bookkeeping only (see PARITY.md).
func (b *InMemoryBackend) AllocateConnectionOnInterconnect(
	req *allocateConnectionOnInterconnectRequest,
) (*Connection, error) {
	if req.Bandwidth == "" || req.ConnectionName == "" || req.InterconnectID == "" || req.OwnerAccount == "" {
		return nil, clientError("bandwidth, connectionName, interconnectId, and ownerAccount are required")
	}

	b.mu.Lock("AllocateConnectionOnInterconnect")
	defer b.mu.Unlock()

	if _, ok := b.interconnects.Get(req.InterconnectID); !ok {
		return nil, notFoundError(resourceInterconnect, req.InterconnectID)
	}

	id := newConnectionID()
	c := &Connection{
		ConnectionID:    id,
		ConnectionName:  req.ConnectionName,
		ConnectionState: ConnectionStateOrdering,
		Bandwidth:       req.Bandwidth,
		Location:        b.locationOfInterconnectLocked(req.InterconnectID),
		Region:          b.region,
		InterconnectID:  req.InterconnectID,
		OwnerAccount:    req.OwnerAccount,
		Vlan:            req.Vlan,
		Tags:            tags.New("directconnect.connection." + id + ".tags"),
	}
	b.connections.Put(c)

	return c.clone(), nil
}

func (b *InMemoryBackend) locationOfInterconnectLocked(id string) string {
	if ic, ok := b.interconnects.Get(id); ok {
		return ic.Location
	}

	return ""
}

// AllocateHostedConnection creates a Connection hosted on an existing
// Connection or LAG for a named end-customer OwnerAccount.
func (b *InMemoryBackend) AllocateHostedConnection(req *allocateHostedConnectionRequest) (*Connection, error) {
	if req.Bandwidth == "" || req.ConnectionID == "" || req.ConnectionName == "" || req.OwnerAccount == "" {
		return nil, clientError("bandwidth, connectionId, connectionName, and ownerAccount are required")
	}

	if err := validateNewTags(tagWireKeys(req.Tags)); err != nil {
		return nil, err
	}

	b.mu.Lock("AllocateHostedConnection")
	defer b.mu.Unlock()

	isLag, ok := b.resolveHostLocked(req.ConnectionID)
	if !ok {
		return nil, notFoundError(resourceConnection, req.ConnectionID)
	}

	id := newConnectionID()
	t := tags.New("directconnect.connection." + id + ".tags")
	t.Merge(tagWireToMap(req.Tags))

	c := &Connection{
		ConnectionID:    id,
		ConnectionName:  req.ConnectionName,
		ConnectionState: ConnectionStateOrdering,
		Bandwidth:       req.Bandwidth,
		Region:          b.region,
		OwnerAccount:    req.OwnerAccount,
		Vlan:            req.Vlan,
		Tags:            t,
	}

	if isLag {
		c.LagID = req.ConnectionID
	} else {
		c.ParentConnectionID = req.ConnectionID
	}

	b.connections.Put(c)

	return c.clone(), nil
}

// AssociateHostedConnection reassigns an already-hosted connection to a
// different parent connection or LAG.
func (b *InMemoryBackend) AssociateHostedConnection(req *associateHostedConnectionRequest) (*Connection, error) {
	if req.ConnectionID == "" || req.ParentConnectionID == "" {
		return nil, clientError("connectionId and parentConnectionId are required")
	}

	b.mu.Lock("AssociateHostedConnection")
	defer b.mu.Unlock()

	c, ok := b.connections.Get(req.ConnectionID)
	if !ok {
		return nil, notFoundError(resourceConnection, req.ConnectionID)
	}

	isLag, ok := b.resolveHostLocked(req.ParentConnectionID)
	if !ok {
		return nil, notFoundError(resourceConnection, req.ParentConnectionID)
	}

	if isLag {
		c.LagID = req.ParentConnectionID
		c.ParentConnectionID = ""
	} else {
		c.ParentConnectionID = req.ParentConnectionID
		c.LagID = ""
	}

	return c.clone(), nil
}

// ConfirmConnection confirms a hosted connection (in "ordering") into
// "pending"/"available".
func (b *InMemoryBackend) ConfirmConnection(connectionID string) (string, error) {
	b.mu.Lock("ConfirmConnection")
	defer b.mu.Unlock()

	c, ok := b.connections.Get(connectionID)
	if !ok {
		return "", notFoundError(resourceConnection, connectionID)
	}

	if c.ConnectionState != ConnectionStateOrdering {
		return "", clientError("connection " + connectionID + " is not awaiting confirmation")
	}

	c.ConnectionState = ConnectionStatePending
	b.scheduleTransition("connection:"+connectionID, []string{ConnectionStateAvailable}, &c.ConnectionState)

	return c.ConnectionState, nil
}

// DescribeConnections returns standard (non-hosted) connections, optionally
// filtered by a single ConnectionId.
func (b *InMemoryBackend) DescribeConnections(connectionID string) []*Connection {
	b.mu.RLock("DescribeConnections")
	defer b.mu.RUnlock()

	if connectionID != "" {
		if c, ok := b.connections.Get(connectionID); ok {
			return []*Connection{c.clone()}
		}

		return nil
	}

	all := b.connections.Snapshot()
	out := make([]*Connection, 0, len(all))

	for _, c := range all {
		out = append(out, c.clone())
	}

	return out
}

// DescribeHostedConnections returns the hosted (child) connections riding
// on the given hosting connection or LAG.
func (b *InMemoryBackend) DescribeHostedConnections(hostID string) []*Connection {
	b.mu.RLock("DescribeHostedConnections")
	defer b.mu.RUnlock()

	var out []*Connection

	for _, c := range b.connections.Snapshot() {
		if c.ParentConnectionID == hostID || (hostID != "" && c.LagID == hostID) {
			out = append(out, c.clone())
		}
	}

	return out
}

// DescribeConnectionsOnInterconnect returns the connections allocated on
// the given interconnect.
func (b *InMemoryBackend) DescribeConnectionsOnInterconnect(interconnectID string) []*Connection {
	b.mu.RLock("DescribeConnectionsOnInterconnect")
	defer b.mu.RUnlock()

	var out []*Connection

	for _, c := range b.connections.Snapshot() {
		if c.InterconnectID == interconnectID {
			out = append(out, c.clone())
		}
	}

	return out
}

// DeleteConnection transitions a Connection to "deleting" then "deleted"
// (or "rejected" for a hosted connection still in "ordering", per
// PARITY.md's ConnectionState notes -- customer-initiated deletion of an
// unconfirmed hosted connection).
func (b *InMemoryBackend) DeleteConnection(connectionID string) (*Connection, error) {
	b.mu.Lock("DeleteConnection")
	defer b.mu.Unlock()

	c, ok := b.connections.Get(connectionID)
	if !ok {
		return nil, notFoundError(resourceConnection, connectionID)
	}

	if c.ConnectionState == ConnectionStateOrdering {
		c.ConnectionState = ConnectionStateRejected

		return c.clone(), nil
	}

	c.ConnectionState = ConnectionStateDeleting
	b.scheduleTransition("connection:"+connectionID, []string{ConnectionStateDeleted}, &c.ConnectionState)

	return c.clone(), nil
}

// UpdateConnection applies a partial rename/encryption-mode update.
func (b *InMemoryBackend) UpdateConnection(req *updateConnectionRequest) (*Connection, error) {
	b.mu.Lock("UpdateConnection")
	defer b.mu.Unlock()

	c, ok := b.connections.Get(req.ConnectionID)
	if !ok {
		return nil, notFoundError(resourceConnection, req.ConnectionID)
	}

	if req.ConnectionName != "" {
		c.ConnectionName = req.ConnectionName
	}

	if req.EncryptionMode != "" {
		c.EncryptionMode = req.EncryptionMode
	}

	return c.clone(), nil
}

// AssociateConnectionWithLag moves connectionID into lagID, enforcing the
// target LAG's own bandwidth-derived capacity (see lagMaxConnections) -- the
// one op in this service where LimitExceededException is reachable without
// any Tags input (PARITY.md wire-trap #8) -- and, on re-association away
// from a different LAG, that LAG's minimum-links floor (gopherstack-55so).
func (b *InMemoryBackend) AssociateConnectionWithLag(connectionID, lagID string) (*Connection, error) {
	b.mu.Lock("AssociateConnectionWithLag")
	defer b.mu.Unlock()

	c, ok := b.connections.Get(connectionID)
	if !ok {
		return nil, notFoundError(resourceConnection, connectionID)
	}

	lag, ok := b.lags.Get(lagID)
	if !ok {
		return nil, notFoundError(resourceLag, lagID)
	}

	// "its bandwidth must match the bandwidth for the LAG"
	// (api_op_AssociateConnectionWithLag.go:15-16).
	if c.Bandwidth != lag.ConnectionsBandwidth {
		return nil, clientError(
			"connection " + connectionID + " bandwidth does not match LAG " + lagID + "'s bandwidth",
		)
	}

	if c.LagID != lagID {
		maxConns := int(lagMaxConnections(lag.ConnectionsBandwidth))
		if len(b.connectionsByLagLocked(lagID)) >= maxConns {
			return nil, limitExceededError("LAG " + lagID + " already has the maximum number of connections")
		}

		if err := b.reassociationMinimumLinksErrorLocked(connectionID, c.LagID); err != nil {
			return nil, err
		}
	}

	c.LagID = lagID

	return c.clone(), nil
}

// reassociationMinimumLinksErrorLocked rejects moving connectionID out of
// oldLagID if that would drop oldLagID below its minimum links.
// "if removing the connection would cause the original LAG to fall below its
// setting for minimum number of operational connections, the request fails"
// (api_op_AssociateConnectionWithLag.go:17-20) -- unlike
// DisassociateConnectionFromLag, this carries no last-member exception
// (gopherstack-55so). Callers must hold b.mu.
func (b *InMemoryBackend) reassociationMinimumLinksErrorLocked(connectionID, oldLagID string) error {
	if oldLagID == "" {
		return nil
	}

	origLag, ok := b.lags.Get(oldLagID)
	if !ok {
		return nil
	}

	remaining := len(b.connectionsByLagLocked(oldLagID)) - 1
	if remaining < int(origLag.MinimumLinks) {
		return clientError(
			"re-associating " + connectionID + " would drop LAG " + oldLagID + " below its minimum links",
		)
	}

	return nil
}

// DisassociateConnectionFromLag removes connectionID from lagID.
func (b *InMemoryBackend) DisassociateConnectionFromLag(connectionID, lagID string) (*Connection, error) {
	b.mu.Lock("DisassociateConnectionFromLag")
	defer b.mu.Unlock()

	c, ok := b.connections.Get(connectionID)
	if !ok {
		return nil, notFoundError(resourceConnection, connectionID)
	}

	if c.LagID != lagID {
		return nil, clientError("connection " + connectionID + " is not associated with LAG " + lagID)
	}

	// "If disassociating the connection would cause the LAG to fall below
	// its setting for minimum number of operational connections, the
	// request fails, except when it's the last member of the LAG"
	// (api_op_DisassociateConnectionFromLag.go:20-22).
	if lag, lagFound := b.lags.Get(lagID); lagFound {
		remaining := len(b.connectionsByLagLocked(lagID)) - 1
		if remaining > 0 && remaining < int(lag.MinimumLinks) {
			return nil, clientError(
				"disassociating " + connectionID + " would drop LAG " + lagID + " below its minimum links",
			)
		}
	}

	c.LagID = ""

	return c.clone(), nil
}

// synthesizeMacSecSecretARN builds a plausible, but NOT actually backed by
// services/secretsmanager, ARN for a raw Cak/Ckn-provided MACsec key --
// DisassociateMacSecKey needs a resolvable SecretARN to remove even a
// caller-supplied raw key later (PARITY.md). JUDGMENT CALL: a real
// implementation could create an actual services/secretsmanager secret
// under the hood (this repo has that backend); this is the simpler,
// ARN-only stand-in, explicitly NOT wired to secretsmanager -- see
// PARITY.md's MACsec section.
func (b *InMemoryBackend) synthesizeMacSecSecretARN(id string) string {
	return "arn:aws:secretsmanager:" + b.region + ":" + b.accountID + ":secret:directconnect!" + id
}

// AssociateMacSecKey associates a MACsec key (either a raw Cak/Ckn pair or
// a pre-existing SecretARN) with a Connection.
func (b *InMemoryBackend) AssociateMacSecKey(req *associateMacSecKeyRequest) (string, []*MacSecKey, error) {
	if req.SecretARN == "" && (req.Cak == "" || req.Ckn == "") {
		return "", nil, clientError("either secretARN or both cak and ckn must be provided")
	}

	b.mu.Lock("AssociateMacSecKey")
	defer b.mu.Unlock()

	c, ok := b.connections.Get(req.ConnectionID)
	if !ok {
		return "", nil, notFoundError(resourceConnection, req.ConnectionID)
	}

	secretARN := req.SecretARN
	if secretARN == "" {
		secretARN = b.synthesizeMacSecSecretARN(newBgpPeerID())
	}

	key := &MacSecKey{
		Ckn:       req.Ckn,
		SecretARN: secretARN,
		StartOn:   time.Now().UTC().Format(time.RFC3339),
		State:     MacSecStateAssociating,
	}
	c.MacSecKeys = append(c.MacSecKeys, key)
	c.MacSecCapable = true

	b.scheduleTransition("macsec:"+secretARN, []string{MacSecStateAssociated}, &key.State)

	return c.ConnectionID, cloneMacSecKeys(c.MacSecKeys), nil
}

// DisassociateMacSecKey removes the MACsec key identified by SecretARN.
func (b *InMemoryBackend) DisassociateMacSecKey(connectionID, secretARN string) (string, []*MacSecKey, error) {
	b.mu.Lock("DisassociateMacSecKey")
	defer b.mu.Unlock()

	c, ok := b.connections.Get(connectionID)
	if !ok {
		return "", nil, notFoundError(resourceConnection, connectionID)
	}

	found := false

	for _, k := range c.MacSecKeys {
		if k.SecretARN == secretARN {
			k.State = MacSecStateDisassociating
			found = true

			b.scheduleTransition("macsec:"+secretARN, []string{MacSecStateDisassociated}, &k.State)
		}
	}

	if !found {
		return "", nil, notFoundError(resourceMacSecKey, secretARN)
	}

	return c.ConnectionID, cloneMacSecKeys(c.MacSecKeys), nil
}
