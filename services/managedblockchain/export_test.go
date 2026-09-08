package managedblockchain

import "time"

// SetProposalExpiration overwrites a proposal's ExpirationDate (for test use only).
func SetProposalExpiration(b *InMemoryBackend, networkID, proposalID string, expiration time.Time) {
	b.mu.Lock("export_test")
	defer b.mu.Unlock()

	if p, exists := b.proposals.Get(proposalKey(networkID, proposalID)); exists {
		p.ExpirationDate = &expiration
	}
}

// NetworkCount returns the number of networks stored in b (for test use only).
func NetworkCount(b *InMemoryBackend) int {
	b.mu.RLock("export_test")
	defer b.mu.RUnlock()

	return b.networks.Len()
}

// MemberCount returns the total number of members across all networks (for test use only).
func MemberCount(b *InMemoryBackend) int {
	b.mu.RLock("export_test")
	defer b.mu.RUnlock()

	return b.members.Len()
}

// NodeCount returns the total number of nodes across all networks and members (for test use only).
func NodeCount(b *InMemoryBackend) int {
	b.mu.RLock("export_test")
	defer b.mu.RUnlock()

	return b.nodes.Len()
}

// AccessorCount returns the number of accessors stored in b (for test use only).
func AccessorCount(b *InMemoryBackend) int {
	b.mu.RLock("export_test")
	defer b.mu.RUnlock()

	return b.accessors.Len()
}

// ProposalCount returns the total number of proposals across all networks (for test use only).
func ProposalCount(b *InMemoryBackend) int {
	b.mu.RLock("export_test")
	defer b.mu.RUnlock()

	return b.proposals.Len()
}

// InvitationCount returns the number of invitations stored in b (for test use only).
func InvitationCount(b *InMemoryBackend) int {
	b.mu.RLock("export_test")
	defer b.mu.RUnlock()

	return b.invitations.Len()
}

// HandlerOpsLen returns the number of supported operations for h (for test use only).
func HandlerOpsLen(h *Handler) int {
	return len(h.GetSupportedOperations())
}

// ARNIndexSize returns the number of entries in the ARN index (for test use only).
func ARNIndexSize(b *InMemoryBackend) int {
	b.mu.RLock("export_test")
	defer b.mu.RUnlock()

	return len(b.arnToResource)
}
