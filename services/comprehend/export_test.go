package comprehend

// PolicyTimestampCountsForTest exposes the sizes of policyCreatedAt and
// policyModifiedAt for tests, since neither is otherwise observable once its
// backing policy entry is gone (GetResourcePolicy errors before reaching
// them, and PutResourcePolicy always overwrites them on next use of the same
// ARN -- see gopherstack-xvm1).
func (b *InMemoryBackend) PolicyTimestampCountsForTest() (int, int) {
	b.mu.RLock("PolicyTimestampCountsForTest")
	defer b.mu.RUnlock()

	return len(b.policyCreatedAt), len(b.policyModifiedAt)
}
