package codeartifact

import (
	"context"
	"fmt"

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

// checkPolicyRevision enforces the optimistic-locking contract documented on
// PolicyRevision across Put/DeleteDomainPermissionsPolicy and Put/
// DeleteRepositoryPermissionsPolicy: "This revision is used for optimistic
// locking, which prevents others from overwriting your changes to the ...
// resource policy." want is empty when the caller omitted PolicyRevision,
// which real AWS accepts unconditionally.
func checkPolicyRevision(want, current string) error {
	if want != "" && want != current {
		return fmt.Errorf("%w: policy revision %s does not match current revision %s", ErrAlreadyExists, want, current)
	}

	return nil
}

// InMemoryBackend is the in-memory store for CodeArtifact resources.
//
// domains and repositories already carry a real, wire-visible Region field,
// so each registers directly on b.registry as a flat *store.Table keyed by
// the composite "region|id" string (see regionKey), with a companion
// *store.Index grouping entries by region for the per-region scans the old
// region-nested maps used to answer directly -- the region-qualified-table
// pattern services/emr uses.
//
// packageGroups, packages, and packageVersions have no wire-visible region
// field, so each gained an unexported region field purely for this composite
// key; they are "dirty" tables (store.New only, NOT store.Register-ed onto
// b.registry -- see store_setup.go) round-tripped through a DTO wrapper in
// persistence.go. repositoryPolicies and domainPolicies are also "dirty" for
// the same reason (region, plus domainName/repoName -- neither type carries
// any parent-identity field at all).
//
// externalConnections is deliberately NOT converted: its value,
// []ExternalConnection, is a slice of plain structs with no identity of its
// own, so there is nothing for store.Table to key on. It remains a plain
// region-nested map, unchanged by this refactor.
type InMemoryBackend struct {
	registry                *store.Registry
	domains                 *store.Table[Domain]
	domainsByRegion         *store.Index[Domain]
	repositories            *store.Table[Repository]
	repositoriesByRegion    *store.Index[Repository]
	packageGroups           *store.Table[PackageGroup]
	packageGroupsByRegion   *store.Index[PackageGroup]
	packages                *store.Table[Package]
	packagesByRegion        *store.Index[Package]
	packageVersions         *store.Table[PackageVersion]
	packageVersionsByRegion *store.Index[PackageVersion]
	repositoryPolicies      *store.Table[RepositoryPermissionsPolicy]
	domainPolicies          *store.Table[DomainPermissionsPolicy]
	externalConnections     map[string]map[string][]ExternalConnection // region → domainName/repoName
	mu                      *lockmetrics.RWMutex
	accountID               string
	region                  string
}

// NewInMemoryBackend creates a new in-memory CodeArtifact backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:            store.NewRegistry(),
		externalConnections: make(map[string]map[string][]ExternalConnection),
		accountID:           accountID,
		region:              region,
		mu:                  lockmetrics.New("codeartifact"),
	}

	registerAllTables(b)

	return b
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string { return b.region }

// regionKey builds the composite store.Table primary key ("region|id") shared
// by every region-nested resource table (see store_setup.go's
// registerAllTables).
func regionKey(region, id string) string { return region + "|" + id }

// externalConnectionsStore returns the per-region inner map, lazily creating
// it. Callers must hold b.mu. Unlike the store.Table-backed collections
// above, externalConnections remains a plain region-nested map (see the
// InMemoryBackend doc comment for why), so this helper is unchanged by the
// Phase 3.3 conversion.
func (b *InMemoryBackend) externalConnectionsStore(region string) map[string][]ExternalConnection {
	if b.externalConnections[region] == nil {
		b.externalConnections[region] = make(map[string][]ExternalConnection)
	}

	return b.externalConnections[region]
}

// externalConnectionsStoreRO returns the per-region external connections map
// for region without mutating the outer map. Safe to call while holding only
// b.mu.RLock(): if the region has not been observed yet, it returns a fresh,
// unregistered, empty map instead of lazily creating (and persisting) an
// entry.
func (b *InMemoryBackend) externalConnectionsStoreRO(region string) map[string][]ExternalConnection {
	if v := b.externalConnections[region]; v != nil {
		return v
	}

	return map[string][]ExternalConnection{}
}

// Reset clears all stored resources, closing Tags on each domain, repository, and package group.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, d := range b.domains.All() {
		d.Tags.Close()
	}
	for _, r := range b.repositories.All() {
		r.Tags.Close()
	}
	for _, pg := range b.packageGroups.All() {
		pg.Tags.Close()
	}

	b.registry.ResetAll()
	// "Dirty" tables (hidden region/domainName/repoName fields) are
	// deliberately NOT on b.registry -- see store_setup.go's
	// registerAllTables doc -- so each needs its own Reset() call here.
	b.packageGroups.Reset()
	b.packages.Reset()
	b.packageVersions.Reset()
	b.repositoryPolicies.Reset()
	b.domainPolicies.Reset()
	b.externalConnections = make(map[string]map[string][]ExternalConnection)
}
