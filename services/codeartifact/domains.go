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

// CreateDomain creates a new CodeArtifact domain.
func (b *InMemoryBackend) CreateDomain(
	ctx context.Context, name, encryptionKey string, kv map[string]string,
) (*Domain, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateDomain")
	defer b.mu.Unlock()

	if b.domains.Has(regionKey(region, name)) {
		return nil, fmt.Errorf("%w: domain %s already exists", ErrAlreadyExists, name)
	}

	domainARN := arn.Build("codeartifact", region, b.accountID, "domain/"+name)
	t := tags.New("codeartifact.domain." + name + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}
	d := &Domain{
		Name:          name,
		ARN:           domainARN,
		EncryptionKey: encryptionKey,
		Owner:         b.accountID,
		Region:        region,
		Status:        "Active",
		S3BucketARN:   "arn:aws:s3:::assets-" + uuid.NewString()[:8],
		CreatedTime:   time.Now().UTC(),
		Tags:          t,
	}
	b.domains.Put(d)
	cp := *d

	return &cp, nil
}

// DescribeDomain returns a domain by name.
func (b *InMemoryBackend) DescribeDomain(ctx context.Context, name string) (*Domain, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeDomain")
	defer b.mu.RUnlock()

	d, ok := b.domains.Get(regionKey(region, name))
	if !ok {
		return nil, fmt.Errorf("%w: domain %s not found", ErrNotFound, name)
	}
	cp := *d

	return &cp, nil
}

// ListDomains returns all domains sorted by name.
func (b *InMemoryBackend) ListDomains(ctx context.Context) []*Domain {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListDomains")
	defer b.mu.RUnlock()

	entries := b.domainsByRegion.Get(region)
	list := make([]*Domain, 0, len(entries))
	for _, d := range entries {
		cp := *d
		list = append(list, &cp)
	}
	slices.SortFunc(list, func(a, b *Domain) int {
		return strings.Compare(a.Name, b.Name)
	})

	return list
}

// DeleteDomain deletes a domain by name, cascade-deleting its package groups,
// domain policy, and Tags. A nonexistent domain returns (nil, nil) rather than
// ErrNotFound: unlike every sibling Delete op in this package (DeleteRepository,
// DeletePackage, DeletePackageGroup), codeartifact@v1.41.4's
// awsRestjson1_deserializeOpErrorDeleteDomain switch does not type
// ResourceNotFoundException at all, so a real client would see an untyped
// smithy.GenericAPIError instead of *types.ResourceNotFoundException. Inference: a
// delete this op's own SDK cannot report not-found for must be idempotent instead
// (DeleteDomainOutput.Domain is a nilable pointer on the wire, so omitting it is not
// a fabrication).
//
// Per api_op_DeleteDomain.go: "You cannot delete a domain that contains
// repositories. If you want to delete a domain with repositories, first delete its
// repositories." -- DeleteDomain models ConflictException for exactly this, so a
// domain with any repository is rejected instead of cascade-deleted.
func (b *InMemoryBackend) DeleteDomain(ctx context.Context, name string) (*Domain, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteDomain")
	defer b.mu.Unlock()

	d, ok := b.domains.Get(regionKey(region, name))
	if !ok {
		return nil, nil //nolint:nilnil // idempotent delete: no domain to describe, not an error
	}
	cp := *d

	for _, r := range b.repositoriesByRegion.Get(region) {
		if r.DomainName == name {
			return nil, fmt.Errorf("%w: domain %s contains repositories", ErrAlreadyExists, name)
		}
	}

	groups := slices.Clone(b.packageGroupsByRegion.Get(region))

	// Cascade: delete every package group owned by this domain (ghost rows +
	// a Tags leak otherwise -- package groups are keyed by domainName+pattern,
	// independent of repositories, so they survive the repository-emptiness
	// check above).
	for _, pg := range groups {
		if pg.DomainName != name {
			continue
		}

		b.packageGroups.Delete(regionKey(region, packageGroupKey(pg.DomainName, pg.Pattern)))
		pg.Tags.Close()
	}

	b.domainPolicies.Delete(regionKey(region, name))
	b.domains.Delete(regionKey(region, name))
	d.Tags.Close()

	return &cp, nil
}

// --- Domain permissions policy methods ---

// GetDomainPermissionsPolicy retrieves the permissions policy for a domain.
// Returns ErrNotFound if the domain does not exist or if no policy has been set.
func (b *InMemoryBackend) GetDomainPermissionsPolicy(
	ctx context.Context, domainName string,
) (*DomainPermissionsPolicy, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetDomainPermissionsPolicy")
	defer b.mu.RUnlock()

	if !b.domains.Has(regionKey(region, domainName)) {
		return nil, fmt.Errorf("%w: domain %s not found", ErrNotFound, domainName)
	}

	pol, ok := b.domainPolicies.Get(regionKey(region, domainName))
	if !ok {
		return nil, fmt.Errorf("%w: no permissions policy found for domain %s", ErrNotFound, domainName)
	}
	cp := *pol

	return &cp, nil
}

// PutDomainPermissionsPolicy stores a permissions policy for a domain.
// policyRevision, when non-empty, must match the existing policy's revision
// (see checkPolicyRevision); a domain with no existing policy accepts any
// policyRevision, matching PutDomainPermissionsPolicyInput's own semantics of
// locking against a policy that must already exist to have a revision.
func (b *InMemoryBackend) PutDomainPermissionsPolicy(
	ctx context.Context, domainName, document, policyRevision string,
) (*DomainPermissionsPolicy, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("PutDomainPermissionsPolicy")
	defer b.mu.Unlock()

	d, ok := b.domains.Get(regionKey(region, domainName))
	if !ok {
		return nil, fmt.Errorf("%w: domain %s not found", ErrNotFound, domainName)
	}

	if existing, hasPolicy := b.domainPolicies.Get(regionKey(region, domainName)); hasPolicy {
		if err := checkPolicyRevision(policyRevision, existing.Revision); err != nil {
			return nil, err
		}
	}

	pol := &DomainPermissionsPolicy{
		Document:    document,
		Revision:    uuid.NewString()[:8],
		ResourceARN: d.ARN,
		region:      region,
		domainName:  domainName,
	}
	b.domainPolicies.Put(pol)
	cp := *pol

	return &cp, nil
}

// DeleteDomainPermissionsPolicy removes the permissions policy from a domain.
// Returns ErrNotFound if the domain does not exist or if no policy has been set.
// policyRevision, when non-empty, must match the existing policy's revision
// (see checkPolicyRevision).
func (b *InMemoryBackend) DeleteDomainPermissionsPolicy(
	ctx context.Context, domainName, policyRevision string,
) (*DomainPermissionsPolicy, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteDomainPermissionsPolicy")
	defer b.mu.Unlock()

	if !b.domains.Has(regionKey(region, domainName)) {
		return nil, fmt.Errorf("%w: domain %s not found", ErrNotFound, domainName)
	}

	pol, ok := b.domainPolicies.Get(regionKey(region, domainName))
	if !ok {
		return nil, fmt.Errorf("%w: no permissions policy found for domain %s", ErrNotFound, domainName)
	}
	if err := checkPolicyRevision(policyRevision, pol.Revision); err != nil {
		return nil, err
	}
	cp := *pol
	b.domainPolicies.Delete(regionKey(region, domainName))

	return &cp, nil
}
