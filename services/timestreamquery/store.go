package timestreamquery

import (
	"context"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// regionContextKey is the context key under which the per-request AWS region is stored.
type regionContextKey struct{}

// getRegion extracts the region from ctx, falling back to defaultRegion when unset.
// Every backend operation resolves the caller's region from the request context and
// operates only on that region's nested store.
func getRegion(ctx context.Context, defaultRegion string) string {
	if r, ok := ctx.Value(regionContextKey{}).(string); ok && r != "" {
		return r
	}

	return defaultRegion
}

// regionFromARN extracts the region component (index 3) from an AWS ARN
// (arn:partition:service:region:account:resource), falling back to defaultRegion.
func regionFromARN(resourceARN, defaultRegion string) string {
	parts := strings.Split(resourceARN, ":")
	const regionIndex = 3
	if len(parts) > regionIndex && parts[regionIndex] != "" {
		return parts[regionIndex]
	}

	return defaultRegion
}

// InMemoryBackend is the in-memory backend for the Timestream Query service.
type InMemoryBackend struct {
	mu                       *lockmetrics.RWMutex
	registry                 *store.Registry
	scheduledQueries         *store.Table[ScheduledQuery] // keyed by Arn (globally unique; embeds region)
	scheduledQueriesByRegion *store.Index[ScheduledQuery] // region → *ScheduledQuery
	queries                  *store.Table[QueryResult]    // keyed by QueryID; not region-isolated
	accountSettings          map[string]AccountSettings   // region → settings
	clientTokens             *clientTokenCache            // Query ClientToken -> QueryID
	scheduledQueryTokens     *clientTokenCache            // CreateScheduledQuery ClientToken -> Arn
	pageStore                *nextTokenStore
	tagWriteBackend          TagWriteBackend
	accountID                string
	defaultRegion            string
}

// TagWriteBackend is the seam into the Timestream Write backend, which
// serves TagResource/UntagResource/ListTagsForResource for scheduled-query
// ARNs too (handler.go's writeServiceTagOps: real Timestream exposes one
// shared tag store across both API surfaces). Without this wired,
// CreateScheduledQuery's Tags never reach the store those reads consult.
type TagWriteBackend interface {
	TagResource(arn string, tags map[string]string) error
	UntagResource(arn string, tagKeys []string) error
}

// SetTagWriteBackend wires the Timestream Write backend that owns the shared
// tag store. Left unwired, CreateScheduledQuery still records tags on the
// ScheduledQuery itself (this package's own TagResource/ListTagsForResource
// still serve them) -- only cross-service reads via TimestreamWrite are
// affected, matching how other cross-service seams in this repo degrade.
func (b *InMemoryBackend) SetTagWriteBackend(wb TagWriteBackend) {
	b.mu.Lock("SetTagWriteBackend")
	defer b.mu.Unlock()
	b.tagWriteBackend = wb
}

// NewInMemoryBackend creates a new in-memory Timestream Query backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		accountID:            accountID,
		defaultRegion:        region,
		mu:                   lockmetrics.New("timestreamquery"),
		registry:             store.NewRegistry(),
		accountSettings:      make(map[string]AccountSettings),
		clientTokens:         newClientTokenCache(queryClientTokenTTL),
		scheduledQueryTokens: newClientTokenCache(createScheduledQueryClientTokenTTL),
		pageStore:            newNextTokenStore(),
	}
	registerAllTables(b)

	return b
}

// Reset clears all backend state, returning it to a freshly initialised condition.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.accountSettings = make(map[string]AccountSettings)
	b.clientTokens = newClientTokenCache(queryClientTokenTTL)
	b.scheduledQueryTokens = newClientTokenCache(createScheduledQueryClientTokenTTL)
	b.pageStore = newNextTokenStore()
}

// AccountID returns the account ID for the backend.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Region returns the default region for the backend.
func (b *InMemoryBackend) Region() string { return b.defaultRegion }
