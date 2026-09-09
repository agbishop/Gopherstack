package azureservicebus

import "time"

// Exported wrappers/seams for internal state used in blackbox tests. Mirrors
// services/azurequeue's export_test.go.

// SetNowFunc replaces the backend's time provider with fn for deterministic
// testing of lock-expiry, TTL, and dead-letter logic without real sleeps.
func SetNowFunc(b *InMemoryBackend, fn func() time.Time) {
	b.nowFunc = fn
}

// SetIDFunc replaces the backend's message-ID/lock-token generator with fn
// for deterministic testing.
func SetIDFunc(b *InMemoryBackend, fn func() string) {
	b.idFunc = fn
}

// SweepStats mirrors the unexported sweepStats shape for external tests.
type SweepStats struct {
	Unlocked     int
	DeadLettered int
	Expired      int
}

// SweepOnce exposes InMemoryBackend.sweepOnce for external tests.
func SweepOnce(b *InMemoryBackend, now time.Time) SweepStats {
	s := b.sweepOnce(now)

	return SweepStats(s)
}
