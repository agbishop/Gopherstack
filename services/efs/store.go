package efs

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

const (
	statusAvailable = "available"
	statusCreating  = "creating"
	statusDeleting  = "deleting"
	statusDeleted   = "deleted"
	statusUpdating  = "updating"
)

const (
	protectionEnabled     = "ENABLED"
	protectionDisabled    = "DISABLED"
	protectionReplicating = "REPLICATING"
)

const (
	backupStatusEnabled  = "ENABLED"
	backupStatusEnabling = "ENABLING"
	backupStatusDisabled = "DISABLED"
)

const (
	managedKMSKeyARN = "arn:aws:kms:us-east-1:000000000000:key/mrk-00000000000000000000000000000000"

	maxTagsPerResource = 50
	maxTagKeyLen       = 128
	maxTagValueLen     = 256

	maxSecurityGroups = 5

	maxFileSystemPolicyBytes = 20 * 1024

	throughputCooldown = 24 * time.Hour

	maxCreationTokenLen        = 64
	maxReplicationDestinations = 1
)

const (
	throughputModeBursting    = "bursting"
	throughputModeProvisioned = "provisioned"
	throughputModeElastic     = "elastic"
	performanceModeGeneral    = "generalPurpose"
	performanceModeMaxIO      = "maxIO"
)

const (
	ipAddressTypeIPv4Only  = "IPV4_ONLY"
	ipAddressTypeIPv6Only  = "IPV6_ONLY"
	ipAddressTypeDualStack = "DUAL_STACK"
)

const (
	resourceIDTypeLong  = "LONG_ID"
	resourceIDTypeShort = "SHORT_ID"
)

// InMemoryBackend is the in-memory store for EFS resources.
//
// The four resource collections below (fileSystems, mountTargets,
// accessPoints, replicationConfigs) were previously nested by region (outer
// key = region) so that same-named resources in different regions were fully
// isolated. Phase 3.3 flattens each into a single *store.Table[T] keyed by
// the composite "region|id" string (see regionKey in store_setup.go), with a
// companion *store.Index grouping entries by region for the old per-region
// scans, and -- for fileSystemsByARN/mountTargetsByARN/accessPointsByARN/
// accessPointsByClientToken -- an unregistered derived-cache *store.Table
// (same pattern as services/ecs's taskDefByArn) providing O(1) region-scoped
// lookup by the resource's own ARN/ClientToken. accountPreferences is
// account-level state in AWS and so is not region-nested.
type InMemoryBackend struct {
	// registry is the Phase 3.3 datalayer lifecycle registry: every
	// *store.Table below except the four ARN/client-token derived caches
	// (which are rebuilt from the registered tables, not independently
	// persisted -- see store_setup.go) is registered on it exactly once at
	// construction, so Reset/Snapshot/Restore collapse to one registry call
	// each instead of one hand-written block per map.
	registry                   *store.Registry
	fileSystems                *store.Table[FileSystem]
	fileSystemsByRegion        *store.Index[FileSystem]
	fileSystemsByARN           *store.Table[FileSystem]
	mountTargets               *store.Table[MountTarget]
	mountTargetsByRegion       *store.Index[MountTarget]
	mountTargetsByARN          *store.Table[MountTarget]
	accessPoints               *store.Table[AccessPoint]
	accessPointsByRegion       *store.Index[AccessPoint]
	accessPointsByARN          *store.Table[AccessPoint]
	accessPointsByClientToken  *store.Table[AccessPoint]
	replicationConfigs         *store.Table[ReplicationConfiguration]
	replicationConfigsByRegion *store.Index[ReplicationConfiguration]

	// lifecyclePolicies, backupPolicies, and fileSystemPolicies are
	// deliberately NOT store.Table-converted: their value types
	// ([]LifecyclePolicy / string) carry no identity of their own, so
	// store.Table (which wraps map[string]*V) is out of scope for them. They
	// remain plain region-nested maps, persisted directly (see
	// persistence.go).
	lifecyclePolicies  map[string]map[string][]LifecyclePolicy
	backupPolicies     map[string]map[string]string
	fileSystemPolicies map[string]map[string]string

	// creationTokenIdx, mtSubnetIdx, and apByFS are performance indexes
	// (avoid O(n) scans on hot paths) whose values are plain strings/
	// struct{}, not *T, so they too are out of store.Table's scope. They are
	// never persisted; Restore rebuilds them from the restored resource
	// tables (see rebuildDerivedIndexes in persistence.go).
	creationTokenIdx map[string]map[string]string              // region → creationToken → fsID
	mtSubnetIdx      map[string]map[string]map[string]string   // region → fsID → subnetID → mtID
	apByFS           map[string]map[string]map[string]struct{} // region → fsID → apID → {}

	accountPreferences AccountPreferences
	mu                 *lockmetrics.RWMutex
	// ec2Resolver validates CreateMountTarget subnets against the real
	// services/ec2 backend -- see EC2Resolver's doc comment (crossservice.go).
	// nil by default (unwired), which leaves subnet placement unvalidated.
	ec2Resolver EC2Resolver
	accountID   string
	region      string
	// fsActivationDelay controls how long CreateFileSystem waits before transitioning
	// a file system from "creating" to "available". Zero (default) means the transition
	// is synchronous and immediate, matching legacy behaviour. A non-zero value enables
	// the AWS-accurate lifecycle simulation and is only set in parity tests.
	fsActivationDelay time.Duration
}

// NewInMemoryBackend creates a new in-memory EFS backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		accountPreferences: AccountPreferences{ResourceIDType: resourceIDTypeLong},
		accountID:          accountID,
		region:             region,
		mu:                 lockmetrics.New("efs"),
		registry:           store.NewRegistry(),
	}

	registerAllTables(b)
	b.initRegionMaps()

	return b
}

// initRegionMaps (re)allocates the (empty) auxiliary maps that were not
// converted to store.Table -- see the doc comments on InMemoryBackend's
// lifecyclePolicies/backupPolicies/fileSystemPolicies/creationTokenIdx/
// mtSubnetIdx/apByFS fields.
func (b *InMemoryBackend) initRegionMaps() {
	b.lifecyclePolicies = make(map[string]map[string][]LifecyclePolicy)
	b.backupPolicies = make(map[string]map[string]string)
	b.fileSystemPolicies = make(map[string]map[string]string)
	b.creationTokenIdx = make(map[string]map[string]string)
	b.mtSubnetIdx = make(map[string]map[string]map[string]string)
	b.apByFS = make(map[string]map[string]map[string]struct{})
}

// The following per-region store helpers return the inner map for region,
// lazily creating it on first access. Callers must hold b.mu.

func (b *InMemoryBackend) lifecycleStore(region string) map[string][]LifecyclePolicy {
	if b.lifecyclePolicies[region] == nil {
		b.lifecyclePolicies[region] = make(map[string][]LifecyclePolicy)
	}

	return b.lifecyclePolicies[region]
}

// lifecycleStoreRO returns the region-scoped lifecyclePolicies map for region
// without mutating the outer map. Safe to call while holding only
// b.mu.RLock(): if the region has not been observed yet, it returns a fresh,
// unregistered, empty map instead of lazily creating (and persisting) an
// entry.
func (b *InMemoryBackend) lifecycleStoreRO(region string) map[string][]LifecyclePolicy {
	if v := b.lifecyclePolicies[region]; v != nil {
		return v
	}

	return map[string][]LifecyclePolicy{}
}

func (b *InMemoryBackend) backupStore(region string) map[string]string {
	if b.backupPolicies[region] == nil {
		b.backupPolicies[region] = make(map[string]string)
	}

	return b.backupPolicies[region]
}

// backupStoreRO returns the region-scoped backupPolicies map for region
// without mutating the outer map. Safe to call while holding only
// b.mu.RLock(): if the region has not been observed yet, it returns a fresh,
// unregistered, empty map instead of lazily creating (and persisting) an
// entry.
func (b *InMemoryBackend) backupStoreRO(region string) map[string]string {
	if v := b.backupPolicies[region]; v != nil {
		return v
	}

	return map[string]string{}
}

func (b *InMemoryBackend) fsPolicyStore(region string) map[string]string {
	if b.fileSystemPolicies[region] == nil {
		b.fileSystemPolicies[region] = make(map[string]string)
	}

	return b.fileSystemPolicies[region]
}

// fsPolicyStoreRO returns the region-scoped fileSystemPolicies map for
// region without mutating the outer map. Safe to call while holding only
// b.mu.RLock(): if the region has not been observed yet, it returns a fresh,
// unregistered, empty map instead of lazily creating (and persisting) an
// entry.
func (b *InMemoryBackend) fsPolicyStoreRO(region string) map[string]string {
	if v := b.fileSystemPolicies[region]; v != nil {
		return v
	}

	return map[string]string{}
}

func (b *InMemoryBackend) tokenIdxStore(region string) map[string]string {
	if b.creationTokenIdx[region] == nil {
		b.creationTokenIdx[region] = make(map[string]string)
	}

	return b.creationTokenIdx[region]
}

func (b *InMemoryBackend) mtSubnetStore(region, fsID string) map[string]string {
	if b.mtSubnetIdx[region] == nil {
		b.mtSubnetIdx[region] = make(map[string]map[string]string)
	}

	if b.mtSubnetIdx[region][fsID] == nil {
		b.mtSubnetIdx[region][fsID] = make(map[string]string)
	}

	return b.mtSubnetIdx[region][fsID]
}

func (b *InMemoryBackend) apFSStore(region, fsID string) map[string]struct{} {
	if b.apByFS[region] == nil {
		b.apByFS[region] = make(map[string]map[string]struct{})
	}

	if b.apByFS[region][fsID] == nil {
		b.apByFS[region][fsID] = make(map[string]struct{})
	}

	return b.apByFS[region][fsID]
}

// Reset clears all stored resources, returning the backend to its empty initial state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, fs := range b.fileSystems.All() {
		fs.Tags.Close()
	}
	for _, ap := range b.accessPoints.All() {
		ap.Tags.Close()
	}

	b.registry.ResetAll()
	b.fileSystemsByARN.Reset()
	b.mountTargetsByARN.Reset()
	b.accessPointsByARN.Reset()
	b.accessPointsByClientToken.Reset()

	b.initRegionMaps()
	b.accountPreferences = AccountPreferences{ResourceIDType: resourceIDTypeLong}
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string { return b.region }

// SetEC2Resolver wires the backend to validate CreateMountTarget subnets
// against the real services/ec2 backend -- see EC2Resolver's doc comment.
// Called from cli.go's wireComputeAndObservabilityIntegrations.
func (b *InMemoryBackend) SetEC2Resolver(r EC2Resolver) {
	b.mu.Lock("SetEC2Resolver")
	defer b.mu.Unlock()

	b.ec2Resolver = r
}

// requireFileSystem returns ErrNotFound if fileSystemID doesn't exist in region.
// Callers use it to guard the FileSystemId-filter path of a Describe* op: real
// AWS raises FileSystemNotFound for an unknown filter, distinct from "this file
// system exists but has no matching items".
func (b *InMemoryBackend) requireFileSystem(region, fileSystemID string) error {
	if _, ok := b.fileSystems.Get(regionKey(region, fileSystemID)); !ok {
		return fmt.Errorf("%w: file system %s not found", ErrNotFound, fileSystemID)
	}

	return nil
}

// describeByIDOrFilter is a generic helper for Describe* methods that look up
// a single item by ID via getByID, or filter allInRegion by file-system ID,
// then paginate. checkFileSystem, if non-nil, validates a non-empty
// fileSystemID filter exists before falling through to an (indistinguishable)
// empty result -- real AWS raises FileSystemNotFound for an unknown filter.
func describeByIDOrFilter[T any](
	getByID func(id string) (*T, bool),
	allInRegion []*T,
	singleID string,
	notFoundErr error,
	fileSystemID string,
	checkFileSystem func(string) error,
	fsIDOf func(*T) string,
	copyFn func(*T) *T,
	idOf func(*T) string,
	marker string,
	maxItems int,
) ([]*T, string, error) {
	if singleID != "" {
		item, ok := getByID(singleID)
		if !ok {
			return nil, "", fmt.Errorf("%w: %s not found", notFoundErr, singleID)
		}

		return []*T{copyFn(item)}, "", nil
	}

	if fileSystemID != "" && checkFileSystem != nil {
		if err := checkFileSystem(fileSystemID); err != nil {
			return nil, "", err
		}
	}

	all := make([]*T, 0, len(allInRegion))
	for _, item := range allInRegion {
		if fileSystemID != "" && fsIDOf(item) != fileSystemID {
			continue
		}
		all = append(all, copyFn(item))
	}
	sort.Slice(all, func(i, j int) bool { return idOf(all[i]) < idOf(all[j]) })

	return paginate(all, marker, maxItems, idOf)
}

// paginate applies cursor-based pagination to a sorted slice.
// The marker returned as nextToken is the key of the first item NOT included in the
// current page (see "next := keyFn(items[maxItems])" below); resuming with that marker
// must therefore start AT (inclusive of) the matched index, not after it. Items from
// marker onward are returned up to maxItems. nextToken is non-empty when more items
// remain. Marker lookup uses binary search (O(log n)) since the slice is already
// sorted by keyFn.
//
// Bug history: an earlier version resumed at items[idx+1:] (skip the matched item),
// which silently dropped exactly one item -- the one at the page boundary -- every
// time a caller paginated across more than one page. A client listing N resources
// page by page would observe strictly fewer than N in total. Caught by a
// DescribeReplicationConfigurations pagination regression test that checked the
// *union* of all pages against the total created, something the pre-existing
// FileSystems/MountTargets/AccessPoints pagination tests never did (they only
// checked each page's length and NextMarker presence in isolation).
func paginate[T any](
	items []T,
	marker string,
	maxItems int,
	keyFn func(T) string,
) ([]T, string, error) {
	if marker != "" {
		// Binary search: find the leftmost index where keyFn(items[i]) >= marker.
		idx := sort.Search(len(items), func(i int) bool { return keyFn(items[i]) >= marker })
		if idx >= len(items) || keyFn(items[idx]) != marker {
			return nil, "", fmt.Errorf("%w: invalid pagination marker", ErrValidation)
		}
		items = items[idx:]
	}

	if maxItems <= 0 || maxItems >= len(items) {
		return items, "", nil
	}

	page := items[:maxItems]
	next := keyFn(items[maxItems])

	return page, next, nil
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
