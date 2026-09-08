package outposts

// RenewalIdempotencyLenForTest returns the number of cached CreateRenewal
// idempotency entries, for tests verifying DeleteOutpost's pruning.
func (b *InMemoryBackend) RenewalIdempotencyLenForTest() int {
	b.mu.RLock("RenewalIdempotencyLenForTest")
	defer b.mu.RUnlock()

	return len(b.renewalIdempotency)
}
