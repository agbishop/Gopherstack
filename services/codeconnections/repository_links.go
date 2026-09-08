package codeconnections

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateRepositoryLink creates a new repository link.
func (b *InMemoryBackend) CreateRepositoryLink(
	ctx context.Context,
	connectionArn, ownerID, repoName, encryptionKeyArn string,
	tags map[string]string,
) (*RepositoryLink, error) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("CreateRepositoryLink")
	defer b.mu.Unlock()

	// Derive provider type from the connection if present.
	providerType := ""
	if conn, ok := b.connections.Get(connectionArn); ok {
		providerType = conn.ProviderType
	}

	// Check for duplicate: same connection + owner + repo, within this region.
	for _, existing := range b.repositoryLinksByRegion.Get(region) {
		if existing.ConnectionArn == connectionArn &&
			existing.OwnerID == ownerID &&
			existing.RepositoryName == repoName {
			return nil, fmt.Errorf(
				"%w: repository link for %s/%s already exists", ErrAlreadyExists, ownerID, repoName,
			)
		}
	}

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	id := uuid.NewString()
	linkArn := arn.Build("codeconnections", region, b.accountID, "repository-link/"+id)

	link := &RepositoryLink{
		ConnectionArn:     connectionArn,
		OwnerID:           ownerID,
		RepositoryName:    repoName,
		RepositoryLinkID:  id,
		RepositoryLinkArn: linkArn,
		ProviderType:      providerType,
		EncryptionKeyArn:  encryptionKeyArn,
		Tags:              tagsCopy,
		CreatedAt:         time.Now().UTC(),
		region:            region,
	}

	b.repositoryLinks.Put(link)

	cp := *link
	cp.Tags = make(map[string]string, len(link.Tags))
	maps.Copy(cp.Tags, link.Tags)

	return &cp, nil
}

// GetRepositoryLink retrieves a repository link by ID.
func (b *InMemoryBackend) GetRepositoryLink(
	ctx context.Context,
	repositoryLinkID string,
) (*RepositoryLink, error) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.RLock("GetRepositoryLink")
	defer b.mu.RUnlock()

	link, ok := b.repositoryLinks.Get(regionKey(region, repositoryLinkID))
	if !ok {
		return nil, ErrNotFound
	}

	cp := *link
	cp.Tags = make(map[string]string, len(link.Tags))
	maps.Copy(cp.Tags, link.Tags)

	return &cp, nil
}

// syncConfigHasReferenceToLinkLocked returns true if any sync configuration in
// region references repositoryLinkID. Must be called with at least an RLock held.
func (b *InMemoryBackend) syncConfigHasReferenceToLinkLocked(region, repositoryLinkID string) bool {
	for _, cfg := range b.syncConfigurationsByRegion.Get(region) {
		if cfg.RepositoryLinkID == repositoryLinkID {
			return true
		}
	}

	return false
}

// DeleteRepositoryLink removes a repository link by ID.
func (b *InMemoryBackend) DeleteRepositoryLink(ctx context.Context, repositoryLinkID string) error {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("DeleteRepositoryLink")
	defer b.mu.Unlock()

	key := regionKey(region, repositoryLinkID)
	if !b.repositoryLinks.Has(key) {
		return ErrNotFound
	}

	if b.syncConfigHasReferenceToLinkLocked(region, repositoryLinkID) {
		return fmt.Errorf("%w: repository link %q has active sync configurations; delete them first",
			ErrSyncConfigStillExists, repositoryLinkID)
	}

	b.repositoryLinks.Delete(key)

	return nil
}

// AddRepositoryLinkInternal seeds a repository link directly for testing.
func (b *InMemoryBackend) AddRepositoryLinkInternal(ctx context.Context, link *RepositoryLink) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("AddRepositoryLinkInternal")
	defer b.mu.Unlock()

	link.region = region
	b.repositoryLinks.Put(link)
}

// ListRepositoryLinks returns all repository links sorted by ID.
func (b *InMemoryBackend) ListRepositoryLinks(ctx context.Context) []*RepositoryLink {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.RLock("ListRepositoryLinks")
	defer b.mu.RUnlock()

	links := b.repositoryLinksByRegion.Get(region)
	result := make([]*RepositoryLink, 0, len(links))

	for _, link := range links {
		cp := *link
		cp.Tags = make(map[string]string, len(link.Tags))
		maps.Copy(cp.Tags, link.Tags)
		result = append(result, &cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].RepositoryLinkID < result[j].RepositoryLinkID
	})

	return result
}

// UpdateRepositoryLink updates the connection ARN or encryption key for a repository link.
func (b *InMemoryBackend) UpdateRepositoryLink(
	ctx context.Context,
	repositoryLinkID, connectionArn, encryptionKeyArn string,
) (*RepositoryLink, error) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("UpdateRepositoryLink")
	defer b.mu.Unlock()

	link, ok := b.repositoryLinks.Get(regionKey(region, repositoryLinkID))
	if !ok {
		return nil, ErrNotFound
	}

	// ConnectionArn/EncryptionKeyArn are not part of the repositoryLinks key
	// (region|RepositoryLinkID) or the byRegion index, so mutating the stored
	// *RepositoryLink in place is safe -- no Delete+Put needed.
	if connectionArn != "" {
		// UpdateRepositoryLinkInput.ConnectionArn's own doc comment
		// (api_op_UpdateRepositoryLink.go) states: "The updated connection
		// ARN must have the same providerType (such as GitHub) as the
		// original connection ARN for the repo link." UpdateRepositoryLink's
		// own error switch (deserializers.go
		// awsAwsjson10_deserializeOpErrorUpdateRepositoryLink) has both
		// ResourceNotFoundException and InvalidInputException, matching
		// ErrNotFound/ErrValidation below.
		conn, connOK := b.connections.Get(connectionArn)
		if !connOK || regionFromARN(connectionArn) != region {
			// A missing target connection isn't literally documented for
			// this op, but ResourceNotFoundException is the same type this
			// service already uses for "referenced resource doesn't exist"
			// on a sibling field (CreateConnection's HostArn, connections.go)
			// when the op's own switch offers it -- it does here.
			return nil, fmt.Errorf("%w: connection %q does not exist", ErrNotFound, connectionArn)
		}

		if conn.ProviderType != link.ProviderType {
			return nil, fmt.Errorf(
				"%w: updated connection provider type %q must match the repository link's provider type %q",
				ErrValidation, conn.ProviderType, link.ProviderType,
			)
		}

		link.ConnectionArn = connectionArn
	}

	if encryptionKeyArn != "" {
		link.EncryptionKeyArn = encryptionKeyArn
	}

	cp := *link
	cp.Tags = make(map[string]string, len(link.Tags))
	maps.Copy(cp.Tags, link.Tags)

	return &cp, nil
}
