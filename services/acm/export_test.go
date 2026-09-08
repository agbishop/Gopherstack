package acm

import (
	"context"
	"time"
)

// TimerCountForTest returns the number of pending auto-validation timers
// currently stored in the backend.
func (b *InMemoryBackend) TimerCountForTest() int {
	b.mu.RLock("TimerCountForTest")
	defer b.mu.RUnlock()

	total := 0
	for _, regionTimers := range b.timers {
		total += len(regionTimers)
	}

	return total
}

// SetAutoValidateDelayForTest overrides the auto-validation timer duration for tests.
func (b *InMemoryBackend) SetAutoValidateDelayForTest(d time.Duration) {
	b.mu.Lock("SetAutoValidateDelayForTest")
	defer b.mu.Unlock()

	b.autoValidateDelay = d
}

// SetIdempotencyRetentionForTest overrides the janitor's idempotency-token
// retention window for tests.
func (b *InMemoryBackend) SetIdempotencyRetentionForTest(d time.Duration) {
	b.mu.Lock("SetIdempotencyRetentionForTest")
	defer b.mu.Unlock()

	b.idempotencyRetention = d
}

// SweepJanitorOnceForTest runs one synchronous pass of the janitor's
// idempotency-token/stale-cert/timer sweep.
func (b *InMemoryBackend) SweepJanitorOnceForTest() {
	b.sweepIdempotencyMaps(context.Background())
}

// BackdateCertForTest overrides a certificate's CreatedAt and NotAfter
// timestamps so tests can trigger the janitor's stale-cert sweep without
// waiting for real time to pass.
func (b *InMemoryBackend) BackdateCertForTest(region, certARN string, createdAt, notAfter time.Time) {
	b.mu.Lock("BackdateCertForTest")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return
	}

	cert.CreatedAt = createdAt
	cert.NotAfter = notAfter
}

// AcmeIdempotencyCountsForTest returns the total number of entries, across
// all regions, in the endpoint/EAB/domain-validation idempotency-token maps.
func (b *InMemoryBackend) AcmeIdempotencyCountsForTest() (int, int, int) {
	b.mu.RLock("AcmeIdempotencyCountsForTest")
	defer b.mu.RUnlock()

	var endpoints, eabs, domainValidations int

	for _, m := range b.endpointIdempotency {
		endpoints += len(m)
	}

	for _, m := range b.eabIdempotency {
		eabs += len(m)
	}

	for _, m := range b.domainValidationIdempotency {
		domainValidations += len(m)
	}

	return endpoints, eabs, domainValidations
}
