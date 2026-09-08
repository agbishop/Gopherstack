package codeartifact

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// repoKey returns the map key for a repository.
func repoKey(domainName, repoName string) string {
	return domainName + "/" + repoName
}

// CreateRepository creates a new CodeArtifact repository.
func (b *InMemoryBackend) CreateRepository(
	ctx context.Context,
	domainName, repoName, description string,
	kv map[string]string,
	upstreams []string,
) (*Repository, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateRepository")
	defer b.mu.Unlock()

	if !b.domains.Has(regionKey(region, domainName)) {
		return nil, fmt.Errorf("%w: domain %s not found", ErrNotFound, domainName)
	}

	key := repoKey(domainName, repoName)
	if b.repositories.Has(regionKey(region, key)) {
		return nil, fmt.Errorf("%w: repository %s already exists in domain %s", ErrAlreadyExists, repoName, domainName)
	}

	repoARN := arn.Build("codeartifact", region, b.accountID, "repository/"+domainName+"/"+repoName)
	t := tags.New("codeartifact.repository." + key + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}
	r := &Repository{
		Name:                 repoName,
		ARN:                  repoARN,
		DomainName:           domainName,
		DomainOwner:          b.accountID,
		Description:          description,
		AdministratorAccount: b.accountID,
		Region:               region,
		CreatedTime:          time.Now().UTC(),
		Tags:                 t,
		UpstreamRepositories: upstreams,
	}
	b.repositories.Put(r)
	cp := *r

	return &cp, nil
}

// DescribeRepository returns a repository by domain and name.
func (b *InMemoryBackend) DescribeRepository(ctx context.Context, domainName, repoName string) (*Repository, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeRepository")
	defer b.mu.RUnlock()

	r, ok := b.repositories.Get(regionKey(region, repoKey(domainName, repoName)))
	if !ok {
		return nil, fmt.Errorf("%w: repository %s not found in domain %s", ErrNotFound, repoName, domainName)
	}
	cp := *r

	return &cp, nil
}

// ListRepositoriesInDomain returns all repositories in a domain, sorted by name.
// Returns ErrNotFound if the domain does not exist.
func (b *InMemoryBackend) ListRepositoriesInDomain(
	ctx context.Context, domainName, repositoryPrefix, administratorAccount string,
) ([]*Repository, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListRepositoriesInDomain")
	defer b.mu.RUnlock()

	if !b.domains.Has(regionKey(region, domainName)) {
		return nil, fmt.Errorf("%w: domain %s not found", ErrNotFound, domainName)
	}

	entries := b.repositoriesByRegion.Get(region)
	list := make([]*Repository, 0, len(entries))
	for _, r := range entries {
		if r.DomainName != domainName {
			continue
		}
		if repositoryPrefix != "" && !strings.HasPrefix(r.Name, repositoryPrefix) {
			continue
		}
		if administratorAccount != "" && r.AdministratorAccount != administratorAccount {
			continue
		}
		cp := *r
		list = append(list, &cp)
	}
	slices.SortFunc(list, func(a, b *Repository) int {
		return strings.Compare(a.Name, b.Name)
	})

	return list, nil
}

// ListRepositories returns all repositories across all domains, sorted by name.
func (b *InMemoryBackend) ListRepositories(ctx context.Context, repositoryPrefix string) []*Repository {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListRepositories")
	defer b.mu.RUnlock()

	entries := b.repositoriesByRegion.Get(region)
	list := make([]*Repository, 0, len(entries))
	for _, r := range entries {
		if repositoryPrefix != "" && !strings.HasPrefix(r.Name, repositoryPrefix) {
			continue
		}
		cp := *r
		list = append(list, &cp)
	}
	slices.SortFunc(list, func(a, b *Repository) int {
		return strings.Compare(a.Name, b.Name)
	})

	return list
}

// DeleteRepository deletes a repository by domain and name, cascade-deleting all
// its packages, package versions, external connections, permissions policy, and Tags.
func (b *InMemoryBackend) DeleteRepository(ctx context.Context, domainName, repoName string) (*Repository, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteRepository")
	defer b.mu.Unlock()

	key := repoKey(domainName, repoName)
	r, ok := b.repositories.Get(regionKey(region, key))
	if !ok {
		return nil, fmt.Errorf("%w: repository %s not found in domain %s", ErrNotFound, repoName, domainName)
	}
	cp := *r

	for _, p := range slices.Clone(b.packagesByRegion.Get(region)) {
		if p.DomainName == domainName && p.Repository == repoName {
			b.packages.Delete(regionKey(region, packageKey(p.DomainName, p.Repository, p.Format, p.Namespace, p.Name)))
		}
	}
	for _, pv := range slices.Clone(b.packageVersionsByRegion.Get(region)) {
		if pv.DomainName == domainName && pv.Repository == repoName {
			b.packageVersions.Delete(regionKey(region, packageVersionKey(
				pv.DomainName, pv.Repository, pv.Format, pv.Namespace, pv.PackageName, pv.Version,
			)))
		}
	}
	delete(b.externalConnectionsStore(region), key)
	b.repositoryPolicies.Delete(regionKey(region, key))
	b.repositories.Delete(regionKey(region, key))
	r.Tags.Close()

	return &cp, nil
}

// --- External connection methods ---

// Package format strings recognized by externalConnectionFormat, mirroring
// aws-sdk-go-v2 types.PackageFormat's relevant enum values.
const (
	packageFormatNPM   = "npm"
	packageFormatMaven = "maven"
)

// externalConnectionFormat derives the package format from a connection name.
func externalConnectionFormat(connectionName string) string {
	switch connectionName {
	case "public:npmjs":
		return packageFormatNPM
	case "public:pypi":
		return "pypi"
	case "public:maven-central", "public:maven-commonsware", "public:maven-googleandroid",
		"public:maven-gradleplugins", "public:maven-apacheorg":
		return packageFormatMaven
	case "public:nuget-org":
		return "nuget"
	case "public:crates-io":
		return "cargo"
	default:
		return "generic"
	}
}

// AssociateExternalConnection associates an external connection with a repository.
func (b *InMemoryBackend) AssociateExternalConnection(
	ctx context.Context,
	domainName, repoName, connectionName string,
) (*Repository, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("AssociateExternalConnection")
	defer b.mu.Unlock()

	r, ok := b.repositories.Get(regionKey(region, repoKey(domainName, repoName)))
	if !ok {
		return nil, fmt.Errorf("%w: repository %s not found in domain %s", ErrNotFound, repoName, domainName)
	}

	key := repoKey(domainName, repoName)
	externalConnections := b.externalConnectionsStore(region)
	for _, ec := range externalConnections[key] {
		if ec.ExternalConnectionName == connectionName {
			return nil, fmt.Errorf("%w: external connection %s already associated", ErrAlreadyExists, connectionName)
		}
	}

	externalConnections[key] = append(externalConnections[key], ExternalConnection{
		ExternalConnectionName: connectionName,
		PackageFormat:          externalConnectionFormat(connectionName),
		Status:                 "AVAILABLE",
	})
	cp := *r

	return &cp, nil
}

// --- Repository permissions policy methods ---

// DeleteRepositoryPermissionsPolicy removes the permissions policy from a repository.
// policyRevision, when non-empty, must match the existing policy's revision
// (see checkPolicyRevision).
func (b *InMemoryBackend) DeleteRepositoryPermissionsPolicy(
	ctx context.Context,
	domainName, repoName, policyRevision string,
) (*RepositoryPermissionsPolicy, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteRepositoryPermissionsPolicy")
	defer b.mu.Unlock()

	if !b.repositories.Has(regionKey(region, repoKey(domainName, repoName))) {
		return nil, fmt.Errorf("%w: repository %s not found in domain %s", ErrNotFound, repoName, domainName)
	}

	key := repoKey(domainName, repoName)
	pol, ok := b.repositoryPolicies.Get(regionKey(region, key))
	if !ok {
		return nil, fmt.Errorf("%w: no permissions policy found for repository %s", ErrNotFound, repoName)
	}
	if err := checkPolicyRevision(policyRevision, pol.Revision); err != nil {
		return nil, err
	}
	cp := *pol
	b.repositoryPolicies.Delete(regionKey(region, key))

	return &cp, nil
}

// PutRepositoryPermissionsPolicy stores a permissions policy for a repository.
// policyRevision, when non-empty, must match the existing policy's revision
// (see checkPolicyRevision); a repository with no existing policy accepts any
// policyRevision, matching PutRepositoryPermissionsPolicyInput's own
// semantics of locking against a policy that must already exist to have a
// revision.
func (b *InMemoryBackend) PutRepositoryPermissionsPolicy(
	ctx context.Context,
	domainName, repoName, document, policyRevision string,
) (*RepositoryPermissionsPolicy, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("PutRepositoryPermissionsPolicy")
	defer b.mu.Unlock()

	r, ok := b.repositories.Get(regionKey(region, repoKey(domainName, repoName)))
	if !ok {
		return nil, fmt.Errorf("%w: repository %s not found in domain %s", ErrNotFound, repoName, domainName)
	}

	if existing, hasPolicy := b.repositoryPolicies.Get(regionKey(region, repoKey(domainName, repoName))); hasPolicy {
		if err := checkPolicyRevision(policyRevision, existing.Revision); err != nil {
			return nil, err
		}
	}

	pol := &RepositoryPermissionsPolicy{
		Document:    document,
		Revision:    uuid.NewString()[:8],
		ResourceARN: r.ARN,
		region:      region,
		domainName:  domainName,
		repoName:    repoName,
	}
	b.repositoryPolicies.Put(pol)
	cp := *pol

	return &cp, nil
}

// GetRepositoryPermissionsPolicy retrieves the permissions policy for a repository.
func (b *InMemoryBackend) GetRepositoryPermissionsPolicy(
	ctx context.Context,
	domainName, repoName string,
) (*RepositoryPermissionsPolicy, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetRepositoryPermissionsPolicy")
	defer b.mu.RUnlock()

	if !b.repositories.Has(regionKey(region, repoKey(domainName, repoName))) {
		return nil, fmt.Errorf("%w: repository %s not found in domain %s", ErrNotFound, repoName, domainName)
	}

	key := repoKey(domainName, repoName)
	pol, ok := b.repositoryPolicies.Get(regionKey(region, key))
	if !ok {
		return nil, fmt.Errorf("%w: no permissions policy found for repository %s", ErrNotFound, repoName)
	}
	cp := *pol

	return &cp, nil
}

// DisassociateExternalConnection removes an external connection from a repository.
func (b *InMemoryBackend) DisassociateExternalConnection(
	ctx context.Context,
	domainName, repoName, connectionName string,
) (*Repository, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DisassociateExternalConnection")
	defer b.mu.Unlock()

	key := repoKey(domainName, repoName)
	r, ok := b.repositories.Get(regionKey(region, key))
	if !ok {
		return nil, fmt.Errorf("%w: repository %s/%s not found", ErrNotFound, domainName, repoName)
	}

	externalConnections := b.externalConnectionsStore(region)
	conns := externalConnections[key]
	filtered := conns[:0]

	for _, c := range conns {
		if c.ExternalConnectionName != connectionName {
			filtered = append(filtered, c)
		}
	}

	externalConnections[key] = filtered
	cp := *r

	return &cp, nil
}

// UpdateRepository updates repository description or upstreams.
func (b *InMemoryBackend) UpdateRepository(
	ctx context.Context, domainName, repoName, description string, upstreams []string,
) (*Repository, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateRepository")
	defer b.mu.Unlock()

	repo, ok := b.repositories.Get(regionKey(region, repoKey(domainName, repoName)))

	if !ok {
		return nil, fmt.Errorf("%w: repository %s/%s not found", ErrNotFound, domainName, repoName)
	}

	if description != "" {
		repo.Description = description
	}

	if upstreams != nil {
		repo.UpstreamRepositories = upstreams
	}

	cp := *repo

	return &cp, nil
}

// --- Additional query methods ---

// CountRepositoriesInDomain returns the number of repositories in a domain.
func (b *InMemoryBackend) CountRepositoriesInDomain(ctx context.Context, domainName string) int {
	region := getRegion(ctx, b.region)

	b.mu.RLock("CountRepositoriesInDomain")
	defer b.mu.RUnlock()

	count := 0
	for _, r := range b.repositoriesByRegion.Get(region) {
		if r.DomainName == domainName {
			count++
		}
	}

	return count
}

// GetExternalConnections returns a copy of the external connections for a repository.
func (b *InMemoryBackend) GetExternalConnections(
	ctx context.Context, domainName, repoName string,
) []ExternalConnection {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetExternalConnections")
	defer b.mu.RUnlock()

	key := repoKey(domainName, repoName)
	conns := b.externalConnectionsStoreRO(region)[key]
	result := make([]ExternalConnection, len(conns))
	copy(result, conns)

	return result
}
