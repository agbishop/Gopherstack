package medialive

import "time"

// SetNow overrides the backend's time source, for tests exercising
// PurchaseOffering's Start window relative to a controlled clock.
func SetNow(b *InMemoryBackend, now func() time.Time) {
	b.mu.Lock("SetNow")
	defer b.mu.Unlock()
	b.nowFunc = now
}

// ChannelCount returns the number of stored channels.
func ChannelCount(b *InMemoryBackend) int {
	b.mu.RLock("ChannelCount")
	defer b.mu.RUnlock()

	return b.channels.Len()
}

// InputCount returns the number of stored inputs.
func InputCount(b *InMemoryBackend) int {
	b.mu.RLock("InputCount")
	defer b.mu.RUnlock()

	return b.inputs.Len()
}

// InputSecurityGroupCount returns the number of stored input security groups.
func InputSecurityGroupCount(b *InMemoryBackend) int {
	b.mu.RLock("InputSecurityGroupCount")
	defer b.mu.RUnlock()

	return b.inputSecurityGroups.Len()
}

// InputDeviceCount returns the number of stored input devices.
func InputDeviceCount(b *InMemoryBackend) int {
	b.mu.RLock("InputDeviceCount")
	defer b.mu.RUnlock()

	return b.inputDevices.Len()
}

// MultiplexCount returns the number of stored multiplexes.
func MultiplexCount(b *InMemoryBackend) int {
	b.mu.RLock("MultiplexCount")
	defer b.mu.RUnlock()

	return b.multiplexes.Len()
}

// MultiplexProgramCount returns the number of programs in a multiplex.
func MultiplexProgramCount(b *InMemoryBackend, multiplexID string) int {
	b.mu.RLock("MultiplexProgramCount")
	defer b.mu.RUnlock()

	m, ok := b.multiplexes.Get(multiplexID)
	if !ok {
		return 0
	}

	return len(m.Programs)
}

// ClusterCount returns the number of stored clusters.
func ClusterCount(b *InMemoryBackend) int {
	b.mu.RLock("ClusterCount")
	defer b.mu.RUnlock()

	return b.clusters.Len()
}

// NodeCount returns the number of nodes in a cluster.
func NodeCount(b *InMemoryBackend, clusterID string) int {
	b.mu.RLock("NodeCount")
	defer b.mu.RUnlock()

	c, ok := b.clusters.Get(clusterID)
	if !ok {
		return 0
	}

	return len(c.Nodes)
}

// ChannelPlacementGroupCount returns the number of placement groups in a cluster.
func ChannelPlacementGroupCount(b *InMemoryBackend, clusterID string) int {
	b.mu.RLock("ChannelPlacementGroupCount")
	defer b.mu.RUnlock()

	count := 0

	for _, g := range b.channelPlacementGroups.All() {
		if g.ClusterID == clusterID {
			count++
		}
	}

	return count
}

// NetworkCount returns the number of stored networks.
func NetworkCount(b *InMemoryBackend) int {
	b.mu.RLock("NetworkCount")
	defer b.mu.RUnlock()

	return b.networks.Len()
}

// SdiSourceCount returns the number of stored SDI sources.
func SdiSourceCount(b *InMemoryBackend) int {
	b.mu.RLock("SdiSourceCount")
	defer b.mu.RUnlock()

	return b.sdiSources.Len()
}

// ForceClusterState sets the state of a cluster directly, for testing purposes.
func ForceClusterState(b *InMemoryBackend, clusterID, state string) {
	b.mu.Lock("ForceClusterState")
	defer b.mu.Unlock()

	if c, ok := b.clusters.Get(clusterID); ok {
		c.State = state
	}
}

// ForceReservationEnd sets a reservation's term end, so a test can make it
// still-active. State is derived from the term (see effectiveState).
func ForceReservationEnd(b *InMemoryBackend, reservationID, end string) {
	b.mu.Lock("ForceReservationEnd")
	defer b.mu.Unlock()

	if r, ok := b.reservations.Get(reservationID); ok {
		r.End = end
	}
}

// ForceSdiSourceInputs sets the Inputs attachment list of an SdiSource
// directly, for testing purposes -- CreateInput/UpdateInput don't yet wire
// the real SdiSources request field, so this is otherwise unreachable.
func ForceSdiSourceInputs(b *InMemoryBackend, sdiSourceID string, inputIDs []string) {
	b.mu.Lock("ForceSdiSourceInputs")
	defer b.mu.Unlock()

	if s, ok := b.sdiSources.Get(sdiSourceID); ok {
		s.Inputs = inputIDs
	}
}
