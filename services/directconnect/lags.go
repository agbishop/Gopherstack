package directconnect

import (
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	lagMaxConnsSmallPort = 4 // 1Gbps or 10Gbps ports: up to 4 connections
	lagMaxConnsBigPort   = 2 // 100Gbps or 400Gbps ports: up to 2 connections
)

// lagMaxConnections returns the maximum number of physical connections a
// LAG at the given ConnectionsBandwidth may hold, per Lag.NumberOfConnections'
// own doc comment ("a maximum of four connections when the port speed is 1
// Gbps or 10 Gbps, or two when...100 Gbps or 400 Gbps"). Falls back to the
// smaller-port cap for any other/unrecognized bandwidth string, a
// conservative default rather than an unbounded one.
func lagMaxConnections(bandwidth string) int32 {
	switch bandwidth {
	case bandwidth100Gbps, bandwidth400Gbps:
		return lagMaxConnsBigPort
	default:
		return lagMaxConnsSmallPort
	}
}

// CreateLag creates a new link aggregation group, provisioning
// NumberOfConnections fresh child connections (or converting an existing
// standalone ConnectionId into the LAG's first member, per PARITY.md).
func (b *InMemoryBackend) CreateLag(req *createLagRequest) (*Lag, error) {
	if req.ConnectionsBandwidth == "" || req.LagName == "" || req.Location == "" {
		return nil, clientError("connectionsBandwidth, lagName, and location are required")
	}

	if req.NumberOfConnections < 1 {
		return nil, clientError("numberOfConnections must be at least 1")
	}

	maxConns := lagMaxConnections(req.ConnectionsBandwidth)
	if req.NumberOfConnections > maxConns {
		return nil, clientError("numberOfConnections exceeds the maximum for this bandwidth")
	}

	if err := validateNewTags(tagWireKeys(req.Tags)); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateLag")
	defer b.mu.Unlock()

	if req.ConnectionID != "" {
		if _, ok := b.connections.Get(req.ConnectionID); !ok {
			return nil, notFoundError(resourceConnection, req.ConnectionID)
		}
	}

	id := newLagID()
	t := tags.New("directconnect.lag." + id + ".tags")
	t.Merge(tagWireToMap(req.Tags))

	lag := &Lag{
		LagID:                   id,
		LagName:                 req.LagName,
		LagState:                LagStateRequested,
		ConnectionsBandwidth:    req.ConnectionsBandwidth,
		Location:                req.Location,
		Region:                  b.region,
		OwnerAccount:            b.accountID,
		ProviderName:            req.ProviderName,
		MinimumLinks:            0,
		NumberOfConnections:     req.NumberOfConnections,
		AllowsHostedConnections: true,
		Tags:                    t,
	}
	b.lags.Put(lag)

	if req.ConnectionID != "" {
		c, _ := b.connections.Get(req.ConnectionID)
		c.LagID = id
	} else {
		for range int(req.NumberOfConnections) {
			cid := newConnectionID()
			ct := tags.New("directconnect.connection." + cid + ".tags")
			ct.Merge(tagWireToMap(req.ChildConnectionTags))

			b.connections.Put(&Connection{
				ConnectionID:    cid,
				ConnectionName:  req.LagName + "-" + cid,
				ConnectionState: ConnectionStateRequested,
				Bandwidth:       req.ConnectionsBandwidth,
				Location:        req.Location,
				Region:          b.region,
				LagID:           id,
				OwnerAccount:    b.accountID,
				ProviderName:    req.ProviderName,
				Tags:            ct,
			})
		}
	}

	b.scheduleTransition("lag:"+id, []string{LagStatePending, LagStateAvailable}, &lag.LagState)

	return lag.clone(), nil
}

// DescribeLags returns LAGs, optionally filtered by a single LagId.
func (b *InMemoryBackend) DescribeLags(lagID string) []*Lag {
	b.mu.RLock("DescribeLags")
	defer b.mu.RUnlock()

	if lagID != "" {
		if l, ok := b.lags.Get(lagID); ok {
			return []*Lag{l.clone()}
		}

		return nil
	}

	all := b.lags.Snapshot()
	out := make([]*Lag, 0, len(all))

	for _, l := range all {
		out = append(out, l.clone())
	}

	return out
}

// DeleteLag transitions a LAG to "deleting" then "deleted". AWS rejects
// deletion "if it has active virtual interfaces or hosted connections"
// (api_op_DeleteLag.go:12-13); in this backend's model both are only ever
// reachable through a connection still associated with the LAG (VIFs
// attach to a connectionId, never directly to a LagID), so checking for
// any non-terminal member/hosted connection covers both clauses.
func (b *InMemoryBackend) DeleteLag(lagID string) (*Lag, error) {
	b.mu.Lock("DeleteLag")
	defer b.mu.Unlock()

	l, ok := b.lags.Get(lagID)
	if !ok {
		return nil, notFoundError(resourceLag, lagID)
	}

	for _, c := range b.connectionsByLagLocked(lagID) {
		if c.ConnectionState != ConnectionStateDeleted && c.ConnectionState != ConnectionStateRejected {
			return nil, clientError("LAG " + lagID + " still has an active connection: " + c.ConnectionID)
		}
	}

	l.LagState = LagStateDeleting
	b.scheduleTransition("lag:"+lagID, []string{LagStateDeleted}, &l.LagState)

	return l.clone(), nil
}

// UpdateLag applies a partial update, including raising MinimumLinks (a
// real, checkable operational lever -- see PARITY.md).
func (b *InMemoryBackend) UpdateLag(req *updateLagRequest) (*Lag, error) {
	b.mu.Lock("UpdateLag")
	defer b.mu.Unlock()

	l, ok := b.lags.Get(req.LagID)
	if !ok {
		return nil, notFoundError(resourceLag, req.LagID)
	}

	if req.MinimumLinks != 0 {
		if req.MinimumLinks > l.NumberOfConnections {
			return nil, clientError("minimumLinks cannot exceed the LAG's numberOfConnections")
		}

		l.MinimumLinks = req.MinimumLinks
	}

	if req.LagName != "" {
		l.LagName = req.LagName
	}

	if req.EncryptionMode != "" {
		l.EncryptionMode = req.EncryptionMode
	}

	return l.clone(), nil
}
