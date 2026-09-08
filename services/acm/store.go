package acm

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// regionContextKey is the context key under which the per-request AWS region is stored.
type regionContextKey struct{}

// getRegion extracts the region from ctx, falling back to defaultRegion when unset.
func getRegion(ctx context.Context, defaultRegion string) string {
	if r, ok := ctx.Value(regionContextKey{}).(string); ok && r != "" {
		return r
	}

	return defaultRegion
}

// InMemoryBackend is the in-memory store for ACM certificates.
// InMemoryBackend stores ACM state. certs is a single flat store.Table keyed
// by the composite "region|arn" string (see regionKey) with a companion
// byRegion Index, replacing the previous map[region]map[arn]*Certificate
// nesting -- see store_setup.go. The remaining maps are non-*T value maps
// (their values are plain structs, not pointers) and stay nested by region
// as plain maps.
type InMemoryBackend struct {
	timers map[string]map[string]*time.Timer
	certs  *store.Table[Certificate]
	// certsByRegion groups certs by region, replacing the per-region scans
	// the old outer map nesting answered directly.
	certsByRegion *store.Index[Certificate]
	// idempotencyMap maps RequestCertificate idempotency tokens to cert info (per region).
	idempotencyMap map[string]map[string]certIdempotencyEntry
	// accountIdempotency maps PutAccountConfiguration tokens to their applied settings (per region).
	accountIdempotency map[string]map[string]accountIdempotencyEntry
	// accountConfig holds the account-level configuration per region.
	accountConfig map[string]AccountConfig

	// The ACME resource family (acme_endpoints.go, acme_eab.go,
	// acme_accounts.go, acme_domain_validations.go). endpoints is the root;
	// eabs/domainValidations/acmeAccounts each carry an AcmeEndpointArn FK
	// and are indexed both by region (List*) and by owning endpoint
	// (ownership scans + DeleteAcmeEndpoint's cascade delete).
	endpoints                   *store.Table[AcmeEndpoint]
	endpointsByRegion           *store.Index[AcmeEndpoint]
	eabs                        *store.Table[AcmeExternalAccountBinding]
	eabsByEndpoint              *store.Index[AcmeExternalAccountBinding]
	domainValidations           *store.Table[AcmeDomainValidation]
	domainValidationsByEndpoint *store.Index[AcmeDomainValidation]
	acmeAccounts                *store.Table[AcmeAccount]
	acmeAccountsByEndpoint      *store.Index[AcmeAccount]
	// endpointIdempotency/eabIdempotency/domainValidationIdempotency map
	// each family's Create* IdempotencyToken to the resource it produced
	// (per region), mirroring idempotencyMap/accountIdempotency above.
	endpointIdempotency         map[string]map[string]acmeIdempotencyEntry
	eabIdempotency              map[string]map[string]acmeIdempotencyEntry
	domainValidationIdempotency map[string]map[string]acmeIdempotencyEntry

	registry             *store.Registry
	mu                   *lockmetrics.RWMutex
	accountID            string
	region               string
	autoValidateDelay    time.Duration
	idempotencyRetention time.Duration
}

// getAutoValidateDelayLocked returns the configured auto-validation delay, or the default.
func (b *InMemoryBackend) getAutoValidateDelayLocked() time.Duration {
	if b.autoValidateDelay > 0 {
		return b.autoValidateDelay
	}

	return autoValidateDelayMS * time.Millisecond
}

// getIdempotencyRetentionLocked returns the configured idempotency-token
// retention window, or the default.
func (b *InMemoryBackend) getIdempotencyRetentionLocked() time.Duration {
	if b.idempotencyRetention > 0 {
		return b.idempotencyRetention
	}

	return defaultIdempotencyRetention
}

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		timers:                      make(map[string]map[string]*time.Timer),
		idempotencyMap:              make(map[string]map[string]certIdempotencyEntry),
		accountIdempotency:          make(map[string]map[string]accountIdempotencyEntry),
		accountConfig:               make(map[string]AccountConfig),
		endpointIdempotency:         make(map[string]map[string]acmeIdempotencyEntry),
		eabIdempotency:              make(map[string]map[string]acmeIdempotencyEntry),
		domainValidationIdempotency: make(map[string]map[string]acmeIdempotencyEntry),
		accountID:                   accountID,
		region:                      region,
		mu:                          lockmetrics.New("acm"),
		registry:                    store.NewRegistry(),
	}

	registerAllTables(b)

	return b
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string { return b.region }

// regionKey builds the composite store.Table primary key ("region|id") used
// by certs -- see store_setup.go.
func regionKey(region, id string) string { return region + "|" + id }

// The *Store helpers return the per-region inner map, lazily creating it.
// Callers must hold b.mu.

func (b *InMemoryBackend) timersStore(region string) map[string]*time.Timer {
	if b.timers[region] == nil {
		b.timers[region] = make(map[string]*time.Timer)
	}

	return b.timers[region]
}

func (b *InMemoryBackend) idempotencyStore(region string) map[string]certIdempotencyEntry {
	if b.idempotencyMap[region] == nil {
		b.idempotencyMap[region] = make(map[string]certIdempotencyEntry)
	}

	return b.idempotencyMap[region]
}

func (b *InMemoryBackend) accountIdempotencyStore(region string) map[string]accountIdempotencyEntry {
	if b.accountIdempotency[region] == nil {
		b.accountIdempotency[region] = make(map[string]accountIdempotencyEntry)
	}

	return b.accountIdempotency[region]
}

func (b *InMemoryBackend) endpointIdempotencyStore(region string) map[string]acmeIdempotencyEntry {
	if b.endpointIdempotency[region] == nil {
		b.endpointIdempotency[region] = make(map[string]acmeIdempotencyEntry)
	}

	return b.endpointIdempotency[region]
}

func (b *InMemoryBackend) eabIdempotencyStore(region string) map[string]acmeIdempotencyEntry {
	if b.eabIdempotency[region] == nil {
		b.eabIdempotency[region] = make(map[string]acmeIdempotencyEntry)
	}

	return b.eabIdempotency[region]
}

func (b *InMemoryBackend) domainValidationIdempotencyStore(region string) map[string]acmeIdempotencyEntry {
	if b.domainValidationIdempotency[region] == nil {
		b.domainValidationIdempotency[region] = make(map[string]acmeIdempotencyEntry)
	}

	return b.domainValidationIdempotency[region]
}

// Reset clears all certificate state and stops any pending auto-validate timers.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, regionTimers := range b.timers {
		for _, t := range regionTimers {
			t.Stop()
		}
	}

	b.registry.ResetAll()
	// certs is a "dirty" table (hidden region field) deliberately NOT on
	// b.registry -- see store_setup.go's registerAllTables doc -- so it
	// needs its own Reset() call here.
	b.certs.Reset()
	b.timers = make(map[string]map[string]*time.Timer)
	b.idempotencyMap = make(map[string]map[string]certIdempotencyEntry)
	b.accountIdempotency = make(map[string]map[string]accountIdempotencyEntry)
	b.accountConfig = make(map[string]AccountConfig)
	b.endpointIdempotency = make(map[string]map[string]acmeIdempotencyEntry)
	b.eabIdempotency = make(map[string]map[string]acmeIdempotencyEntry)
	b.domainValidationIdempotency = make(map[string]map[string]acmeIdempotencyEntry)
}

// Close stops all in-flight certificate auto-validation timers so their
// goroutines do not outlive the backend, without otherwise clearing state. It is
// safe to call multiple times.
func (b *InMemoryBackend) Close() {
	b.mu.Lock("Close")
	defer b.mu.Unlock()

	for _, regionTimers := range b.timers {
		for _, t := range regionTimers {
			t.Stop()
		}
	}
	b.timers = make(map[string]map[string]*time.Timer)
}
