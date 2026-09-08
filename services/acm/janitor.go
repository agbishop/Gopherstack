package acm

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	// defaultIdempotencyRetention is how long idempotency tokens are retained.
	defaultIdempotencyRetention = 24 * time.Hour
)

// RunJanitor periodically cleans up expired idempotency tokens.
func (b *InMemoryBackend) RunJanitor(ctx context.Context, interval time.Duration) {
	g := worker.NewGroup(ctx, "acm")
	g.Ticker("AcmJanitor", interval, 0, b.sweepIdempotencyMaps)

	<-ctx.Done()
	g.Stop()
}

func (b *InMemoryBackend) sweepIdempotencyMaps(ctx context.Context) {
	b.mu.Lock("sweepIdempotencyMaps")

	now := time.Now().UTC()
	cutoffIdempotency := now.Add(-b.getIdempotencyRetentionLocked())
	removedCount := 0

	certCreatedAt := func(e certIdempotencyEntry) time.Time { return e.CreatedAt }
	accountCreatedAt := func(e accountIdempotencyEntry) time.Time { return e.CreatedAt }
	removedCount += sweepExpiredTokens(b.idempotencyMap, cutoffIdempotency, certCreatedAt)
	removedCount += sweepExpiredTokens(b.accountIdempotency, cutoffIdempotency, accountCreatedAt)
	// The ACME resource family's Create* idempotency tokens (endpoints/EABs/
	// domain-validations) previously had no TTL sweep at all -- unlike
	// idempotencyMap/accountIdempotency above, they grew unbounded for every
	// token-bearing Create call, and DeleteAcmeEndpoint's cascade delete
	// (acme_endpoints.go) only cleans its own endpointIdempotency entry, not
	// the eabIdempotency/domainValidationIdempotency entries of the children
	// it cascade-deletes -- so those were also orphaned with no cleanup path.
	acmeFingerprint := func(e acmeIdempotencyEntry) time.Time { return e.CreatedAt }
	removedCount += sweepExpiredTokens(b.endpointIdempotency, cutoffIdempotency, acmeFingerprint)
	removedCount += sweepExpiredTokens(b.eabIdempotency, cutoffIdempotency, acmeFingerprint)
	removedCount += sweepExpiredTokens(b.domainValidationIdempotency, cutoffIdempotency, acmeFingerprint)

	staleTimeouts, staleExpirations := b.collectStaleCerts(now)

	b.mu.Unlock()

	// Drive the real TimeoutPendingValidation/ExpireCertificate transitions
	// through their locking, timer-aware public methods rather than mutating
	// cert.Status inline here, so this sweep is not a second, drift-prone
	// copy of that logic. Counted separately below, not folded into
	// removedCount -- a status transition is not a purge.
	transitionedCount := b.applyStaleCertTransitions(ctx, staleTimeouts, staleExpirations)

	b.mu.Lock("sweepIdempotencyMaps")
	removedCount += b.sweepTimers()
	b.mu.Unlock()

	if removedCount > 0 {
		telemetry.RecordWorkerItems("acm", "AcmJanitor", removedCount)
		logger.Load(ctx).DebugContext(ctx, "ACM janitor: resources purged",
			"count", removedCount)
	}

	if transitionedCount > 0 {
		telemetry.RecordWorkerItems("acm", "AcmJanitorCertTransitions", transitionedCount)
		logger.Load(ctx).DebugContext(ctx, "ACM janitor: certificates transitioned",
			"count", transitionedCount)
	}

	telemetry.RecordWorkerTask("acm", "AcmJanitor", "success")
}

// sweepExpiredTokens removes idempotency-token entries older than cutoff
// across all regions of a region-scoped token map, and returns the number
// removed. Shared by every idempotency-token family in this package
// (RequestCertificate, PutAccountConfiguration, and the ACME endpoint/EAB/
// domain-validation families).
func sweepExpiredTokens[T any](m map[string]map[string]T, cutoff time.Time, createdAt func(T) time.Time) int {
	removed := 0
	for _, regionTokens := range m {
		for token, entry := range regionTokens {
			if createdAt(entry).Before(cutoff) {
				delete(regionTokens, token)
				removed++
			}
		}
	}

	return removed
}

// staleCertRef identifies a certificate eligible for a janitor-driven status
// transition.
type staleCertRef struct {
	region string
	arn    string
}

// collectStaleCerts finds certs eligible for TimeoutPendingValidation
// (abandoned pending validations -- aws-sdk-go-v2/service/acm@v1.43.4
// types/types.go CertificateDetail.Status doc: "ACM makes repeated attempts
// to validate a certificate for 72 hours and then times out") and
// ExpireCertificate (NotAfter has passed), across all regions. Callers must
// hold b.mu.
func (b *InMemoryBackend) collectStaleCerts(now time.Time) ([]staleCertRef, []staleCertRef) {
	var timeouts, expirations []staleCertRef

	cutoffPending := now.Add(-72 * time.Hour)

	for _, cert := range b.certs.All() {
		ref := staleCertRef{region: cert.region, arn: cert.ARN}

		switch {
		case cert.Status == statusPendingValidation && cert.CreatedAt.Before(cutoffPending):
			timeouts = append(timeouts, ref)
		case cert.Status == statusIssued && !cert.NotAfter.IsZero() && cert.NotAfter.Before(now):
			expirations = append(expirations, ref)
		}
	}

	return timeouts, expirations
}

// applyStaleCertTransitions drives the janitor-collected transitions through
// the real locking TimeoutPendingValidation/ExpireCertificate methods.
// Callers must NOT hold b.mu -- each call takes it itself. Errors (e.g. a
// concurrent request already moved the cert on) are not fatal to the sweep.
func (b *InMemoryBackend) applyStaleCertTransitions(ctx context.Context, timeouts, expirations []staleCertRef) int {
	applied := 0

	for _, ref := range timeouts {
		regionCtx := context.WithValue(ctx, regionContextKey{}, ref.region)
		if err := b.TimeoutPendingValidation(regionCtx, ref.arn); err == nil {
			applied++
		}
	}

	for _, ref := range expirations {
		regionCtx := context.WithValue(ctx, regionContextKey{}, ref.region)
		if err := b.ExpireCertificate(regionCtx, ref.arn); err == nil {
			applied++
		}
	}

	return applied
}

func (b *InMemoryBackend) sweepTimers() int {
	removedCount := 0
	for region, regionTimers := range b.timers {
		for arn, timer := range regionTimers {
			cert, ok := b.certs.Get(regionKey(region, arn))
			if !ok {
				timer.Stop()
				delete(regionTimers, arn)
				removedCount++

				continue
			}

			isPending := cert.Status == statusPendingValidation
			hasRenewal := cert.RenewalSummary != nil &&
				cert.RenewalSummary.RenewalStatus == renewalStatusPendingValidation

			if !isPending && !hasRenewal {
				timer.Stop()
				delete(regionTimers, arn)
				removedCount++
			}
		}
	}

	return removedCount
}
