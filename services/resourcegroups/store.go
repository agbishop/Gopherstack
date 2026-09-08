package resourcegroups

import (
	"context"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// regionContextKey is the context key under which the per-request AWS region is stored.
type regionContextKey struct{}

// getRegion extracts the region from ctx, falling back to defaultRegion when unset.
// Resource Groups resources are isolated per region: every backend operation resolves
// the caller's region from the request context and operates only on that region's
// nested store.
func getRegion(ctx context.Context, defaultRegion string) string {
	if r, ok := ctx.Value(regionContextKey{}).(string); ok && r != "" {
		return r
	}

	return defaultRegion
}

// InMemoryBackend is the in-memory store for Resource Groups.
//
// Phase 3.3 of the datalayer refactor replaces the region-nested
// map[string]map[string]*Group and its companion map[string]map[string]string
// ARN reverse index with a single flat *store.Table[Group], keyed by the
// composite "region|name" string (see regionKey below), with companion
// *store.Index values grouping entries by region (replacing the old outer map
// nesting used by ListGroups) and by "region|ARN" (replacing the old
// per-region arnIndex reverse map used by findByARN). tagSyncTasks receives
// the identical treatment. See store_setup.go.
//
// Group and TagSyncTask carry no exported Region field (not part of either's
// AWS wire shape); both always embed their owning region in their ARN (see
// CreateGroup/StartTagSyncTask), so groupRegionOf/taskRegionOf derive it from
// there instead of adding a new field.
//
// groups is a "dirty" table in the store_setup.go/persistence.go sense: each
// Group carries a live *tags.Tags field (a Prometheus-instrumented map) that
// plain json.Marshal cannot round-trip while preserving the per-resource
// metric name -- so it is NOT registered on the shared b.registry; it is
// built with store.New only, and persistence.go handles its Snapshot/Restore
// via a plain-map DTO. tagSyncTasks has no such field and is registered
// directly.
//
// groupConfigurations, groupResources, and groupingStatuses remain plain
// region-nested maps: their values (a slice of items, a slice of ARN
// strings, a slice of status records) have no identity of their own for
// store.Table to key on, so there is nothing to convert -- each is persisted
// here directly instead (see persistence.go).
type InMemoryBackend struct {
	groups               *store.Table[Group]
	groupsByRegion       *store.Index[Group]
	groupsByARN          *store.Index[Group]
	tagSyncTasks         *store.Table[TagSyncTask]
	tagSyncTasksByRegion *store.Index[TagSyncTask]
	registry             *store.Registry
	groupConfigurations  map[string]map[string][]GroupConfigurationItem
	groupResources       map[string]map[string][]string // region → group name → []resourceARN
	groupingStatuses     map[string]map[string][]GroupingStatusItem
	mu                   *lockmetrics.RWMutex
	accountSettings      AccountSettings
	accountID            string
	region               string
	taskIDCounter        int64 // monotonically incremented for unique task ARNs
}

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		groupConfigurations: make(map[string]map[string][]GroupConfigurationItem),
		groupResources:      make(map[string]map[string][]string),
		groupingStatuses:    make(map[string]map[string][]GroupingStatusItem),
		accountID:           accountID,
		region:              region,
		mu:                  lockmetrics.New("resourcegroups"),
		registry:            store.NewRegistry(),
	}
	registerAllTables(b)

	return b
}

// regionKey builds the composite store.Table primary key ("region|id") shared
// by both region-qualified tables registered in store_setup.go.
func regionKey(region, id string) string { return region + "|" + id }

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

// groupRegionOf returns the region a Group belongs to, derived from its ARN
// (always built with the owning region -- see CreateGroup), falling back to
// the backend's own default region for a malformed/test-injected ARN.
func (b *InMemoryBackend) groupRegionOf(g *Group) string {
	return regionFromARN(g.ARN, b.region)
}

// groupTableKeyFn is the store.Table primary key function for groups.
func (b *InMemoryBackend) groupTableKeyFn(g *Group) string {
	return regionKey(b.groupRegionOf(g), g.Name)
}

// groupRegionIndexKeyFn is the groupsByRegion index key function.
func (b *InMemoryBackend) groupRegionIndexKeyFn(g *Group) string {
	return b.groupRegionOf(g)
}

// groupARNIndexKeyFn is the groupsByARN index key function.
func (b *InMemoryBackend) groupARNIndexKeyFn(g *Group) string {
	return regionKey(b.groupRegionOf(g), g.ARN)
}

// taskRegionOf returns the region a TagSyncTask belongs to, derived from its
// TaskArn (always built with the owning region -- see StartTagSyncTask),
// falling back to the backend's own default region for a malformed/
// test-injected ARN.
func (b *InMemoryBackend) taskRegionOf(t *TagSyncTask) string {
	return regionFromARN(t.TaskArn, b.region)
}

// tagSyncTaskTableKeyFn is the store.Table primary key function for tagSyncTasks.
func (b *InMemoryBackend) tagSyncTaskTableKeyFn(t *TagSyncTask) string {
	return regionKey(b.taskRegionOf(t), t.TaskArn)
}

// tagSyncTaskRegionIndexKeyFn is the tagSyncTasksByRegion index key function.
func (b *InMemoryBackend) tagSyncTaskRegionIndexKeyFn(t *TagSyncTask) string {
	return b.taskRegionOf(t)
}

// The *Store helpers return the per-region inner map, lazily creating it.
// Callers must hold b.mu (write lock).

func (b *InMemoryBackend) groupConfigurationsStore(region string) map[string][]GroupConfigurationItem {
	if b.groupConfigurations[region] == nil {
		b.groupConfigurations[region] = make(map[string][]GroupConfigurationItem)
	}

	return b.groupConfigurations[region]
}

func (b *InMemoryBackend) groupResourcesStore(region string) map[string][]string {
	if b.groupResources[region] == nil {
		b.groupResources[region] = make(map[string][]string)
	}

	return b.groupResources[region]
}

func (b *InMemoryBackend) groupingStatusesStore(region string) map[string][]GroupingStatusItem {
	if b.groupingStatuses[region] == nil {
		b.groupingStatuses[region] = make(map[string][]GroupingStatusItem)
	}

	return b.groupingStatuses[region]
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string { return b.region }

// AccountID returns the AWS account ID this backend is configured for.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Reset clears all in-memory state. It closes all group Tags to release
// Prometheus metrics before discarding the groups table.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, g := range b.groups.All() {
		if g.Tags != nil {
			g.Tags.Close()
		}
	}

	b.groups.Reset()
	b.tagSyncTasks.Reset()
	b.groupConfigurations = make(map[string]map[string][]GroupConfigurationItem)
	b.groupResources = make(map[string]map[string][]string)
	b.groupingStatuses = make(map[string]map[string][]GroupingStatusItem)
	b.accountSettings = AccountSettings{}
	b.taskIDCounter = 0
}

// resolveGroupName extracts the group name from a name-or-ARN value.
func resolveGroupName(nameOrARN string) string {
	if idx := strings.LastIndex(nameOrARN, "group/"); idx >= 0 {
		return nameOrARN[idx+len("group/"):]
	}

	return nameOrARN
}

// paginate returns a page of items starting after nextToken, limited to maxResults.
// keyFn extracts a unique, stable sort key from each item (used as the continuation token).
// If maxResults is 0, all items are returned. If nextToken is empty, results start from the beginning.
func paginate[T any](list []T, keyFn func(T) string, nextToken string, maxResults int) ([]T, string) {
	if nextToken != "" {
		start := 0

		for i, item := range list {
			if keyFn(item) == nextToken {
				start = i + 1

				break
			}
		}

		list = list[start:]
	}

	if maxResults <= 0 || maxResults >= len(list) {
		return list, ""
	}

	page := list[:maxResults]

	return page, keyFn(page[len(page)-1])
}
