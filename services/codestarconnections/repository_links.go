package codestarconnections

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// syncConfigHasReferenceToLinkLocked returns true if any sync config references the given repositoryLinkID.
// Must be called with at least an RLock held.
func (b *InMemoryBackend) syncConfigHasReferenceToLinkLocked(region, repositoryLinkID string) bool {
	for _, cfg := range b.syncConfigurationsByRegion.Get(region) {
		if cfg.RepositoryLinkID == repositoryLinkID {
			return true
		}
	}

	return false
}

// CreateRepositoryLink creates a new repository link.
func (b *InMemoryBackend) CreateRepositoryLink(
	ctx context.Context,
	connectionArn, ownerID, repoName, encryptionKeyArn string,
	tags map[string]string,
) (*RepositoryLink, error) {
	if err := validateTags(tags); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("CreateRepositoryLink")
	defer b.mu.Unlock()

	// Derive provider type from the connection if it exists (Connection is
	// keyed directly by its own ARN, which already embeds its region).
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
				"%w: repository link for %s/%s already exists", ErrResourceAlreadyExists, ownerID, repoName,
			)
		}
	}

	id := uuid.NewString()
	linkArn := arn.Build("codestar-connections", region, b.accountID, "repository-link/"+id)

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	link := &RepositoryLink{
		ConnectionArn:     connectionArn,
		OwnerID:           ownerID,
		RepositoryName:    repoName,
		RepositoryLinkID:  id,
		RepositoryLinkArn: linkArn,
		ProviderType:      providerType,
		EncryptionKeyArn:  encryptionKeyArn,
		CreatedAt:         time.Now().UTC(),
		Tags:              tagsCopy,
		region:            region,
	}

	b.repositoryLinks.Put(link)

	cp := *link
	cp.Tags = make(map[string]string, len(link.Tags))
	maps.Copy(cp.Tags, link.Tags)

	return &cp, nil
}

// GetRepositoryLink retrieves a repository link by ID.
func (b *InMemoryBackend) GetRepositoryLink(ctx context.Context, repositoryLinkID string) (*RepositoryLink, error) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.RLock("GetRepositoryLink")
	defer b.mu.RUnlock()

	link, ok := b.repositoryLinks.Get(regionKey(region, repositoryLinkID))
	if !ok {
		return nil, fmt.Errorf("%w: repository link not found: %s", ErrNotFound, repositoryLinkID)
	}

	cp := *link
	cp.Tags = make(map[string]string, len(link.Tags))
	maps.Copy(cp.Tags, link.Tags)

	return &cp, nil
}

// DeleteRepositoryLink removes a repository link by ID. Returns ErrResourceInUse if sync configs reference it.
func (b *InMemoryBackend) DeleteRepositoryLink(ctx context.Context, repositoryLinkID string) error {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("DeleteRepositoryLink")
	defer b.mu.Unlock()

	key := regionKey(region, repositoryLinkID)
	if !b.repositoryLinks.Has(key) {
		return fmt.Errorf("%w: repository link not found: %s", ErrNotFound, repositoryLinkID)
	}

	if b.syncConfigHasReferenceToLinkLocked(region, repositoryLinkID) {
		return fmt.Errorf("%w: repository link %q has active sync configurations; delete them first",
			ErrSyncConfigStillExists, repositoryLinkID)
	}

	b.repositoryLinks.Delete(key)

	return nil
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
		return nil, fmt.Errorf("%w: repository link not found: %s", ErrNotFound, repositoryLinkID)
	}

	// ConnectionArn/EncryptionKeyArn are not part of the repositoryLinks key
	// (region|RepositoryLinkID) or the byRegion index, so mutating the
	// stored *RepositoryLink in place is safe -- no Delete+Put needed.
	if connectionArn != "" {
		// UpdateRepositoryLinkInput.ConnectionArn's own doc comment
		// (api_op_UpdateRepositoryLink.go): "The updated connection ARN must
		// have the same providerType (such as GitHub) as the original
		// connection ARN for the repo link." UpdateRepositoryLink's own
		// error switch (deserializers.go
		// awsAwsjson10_deserializeOpErrorUpdateRepositoryLink) carries both
		// ResourceNotFoundException and InvalidInputException.
		conn, connOK := b.connections.Get(connectionArn)
		if !connOK || regionFromARN(connectionArn, b.defaultRegion) != region {
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
