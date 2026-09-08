package cognitoidentity

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
)

// CreateIdentityPool creates a new identity pool.
func (b *InMemoryBackend) CreateIdentityPool(
	ctx context.Context,
	name string,
	allowUnauthenticated bool,
	allowClassicFlow bool,
	developerProviderName string,
	providers []IdentityProvider,
	supportedLoginProviders map[string]string,
	tags map[string]string,
	opts ...*PoolExtendedConfig,
) (*IdentityPool, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateIdentityPool")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: IdentityPoolName is required", ErrInvalidParameter)
	}

	if b.poolNameTaken(region, name) {
		return nil, fmt.Errorf(
			"%w: identity pool %q already exists",
			ErrIdentityPoolAlreadyExists,
			name,
		)
	}

	poolID := region + ":" + uuid.New().String()
	arn := fmt.Sprintf(
		"arn:aws:cognito-identity:%s:%s:identitypool/%s",
		region,
		b.accountID,
		poolID,
	)

	var cfg PoolExtendedConfig
	if len(opts) > 0 && opts[0] != nil {
		cfg = *opts[0]
	}

	pool := &IdentityPool{
		IdentityPoolID:                 poolID,
		IdentityPoolName:               name,
		ARN:                            arn,
		DeveloperProviderName:          developerProviderName,
		AllowUnauthenticatedIdentities: allowUnauthenticated,
		AllowClassicFlow:               allowClassicFlow,
		IdentityProviders:              cloneProviders(providers),
		SupportedLoginProviders:        cloneStringMap(supportedLoginProviders),
		Tags:                           cloneStringMap(tags),
		OpenIDConnectProviderARNs:      cloneStringSlice(cfg.OpenIDConnectProviderARNs),
		SamlProviderARNs:               cloneStringSlice(cfg.SamlProviderARNs),
		CreatedAt:                      time.Now(),
		region:                         region,
	}

	b.poolPut(pool)

	return clonePool(pool), nil
}

// DeleteIdentityPool removes an identity pool and all associated identities, roles,
// and principal-tag configurations.
func (b *InMemoryBackend) DeleteIdentityPool(ctx context.Context, poolID string) error {
	if poolID == "" {
		return fmt.Errorf("%w: IdentityPoolId is required", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteIdentityPool")
	defer b.mu.Unlock()

	if _, ok := b.poolGet(region, poolID); !ok {
		return fmt.Errorf("%w: identity pool %q not found", ErrIdentityPoolNotFound, poolID)
	}

	b.poolDelete(region, poolID)
	b.rolesDelete(region, poolID)

	// Delete every identity in the pool. The index slice is owned by
	// identitiesByPool and mutates as entries are deleted from it, so clone it first.
	for _, identity := range slices.Clone(b.identitiesInPool(region, poolID)) {
		b.identityDelete(region, identity.IdentityID)
	}

	// Purge all principal-tag mappings that belong to this pool. Same cloning
	// requirement as above.
	for _, tag := range slices.Clone(b.principalTagsInPool(region, poolID)) {
		b.principalTagDelete(region, tag.poolID, tag.providerName)
	}

	return nil
}

// DescribeIdentityPool returns the identity pool with the given ID.
func (b *InMemoryBackend) DescribeIdentityPool(
	ctx context.Context,
	poolID string,
) (*IdentityPool, error) {
	if poolID == "" {
		return nil, fmt.Errorf("%w: IdentityPoolId is required", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeIdentityPool")
	defer b.mu.RUnlock()

	pool, ok := b.poolGet(region, poolID)
	if !ok {
		return nil, fmt.Errorf("%w: identity pool %q not found", ErrIdentityPoolNotFound, poolID)
	}

	return clonePool(pool), nil
}

// ListIdentityPools returns all identity pools sorted by name, up to maxResults (0 = all).
// nextToken is an opaque cursor that encodes the last-returned pool name for pagination.
func (b *InMemoryBackend) ListIdentityPools(
	ctx context.Context,
	maxResults int,
	nextToken string,
) ([]*IdentityPool, string) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListIdentityPools")
	defer b.mu.RUnlock()

	// poolsByRegion's slice is owned by the index; clone it before sorting in place.
	pools := slices.Clone(b.poolsInRegion(region))

	sort.Slice(pools, func(i, j int) bool {
		return pools[i].IdentityPoolName < pools[j].IdentityPoolName
	})

	// Apply cursor: skip all pools up to and including the one named by nextToken.
	// Default to len(pools) so an unrecognised / past-end token returns an empty page.
	startIdx := 0
	if nextToken != "" {
		startIdx = len(pools)

		for i, p := range pools {
			if p.IdentityPoolName == nextToken {
				startIdx = i + 1

				break
			}
		}
	}

	pools = pools[startIdx:]

	limit := len(pools)
	if maxResults > 0 && maxResults < limit {
		limit = maxResults
	}

	out := make([]*IdentityPool, 0, limit)

	for i, p := range pools {
		out = append(out, clonePool(p))

		if maxResults > 0 && len(out) >= maxResults {
			// Return the name of the last item as the cursor for the next page.
			if i+1 < len(pools) {
				return out, p.IdentityPoolName
			}

			break
		}
	}

	return out, ""
}

// UpdateIdentityPool updates the settings of an existing identity pool.
func (b *InMemoryBackend) UpdateIdentityPool(
	ctx context.Context,
	poolID string,
	name string,
	allowUnauthenticated bool,
	allowClassicFlow bool,
	developerProviderName string,
	providers []IdentityProvider,
	supportedLoginProviders map[string]string,
	tags map[string]string,
	opts ...*PoolExtendedConfig,
) (*IdentityPool, error) {
	if poolID == "" {
		return nil, fmt.Errorf("%w: IdentityPoolId is required", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateIdentityPool")
	defer b.mu.Unlock()

	pool, ok := b.poolGet(region, poolID)
	if !ok {
		return nil, fmt.Errorf("%w: identity pool %q not found", ErrIdentityPoolNotFound, poolID)
	}

	if err := b.renamePoolIfNeeded(region, pool, name); err != nil {
		return nil, err
	}

	pool.AllowUnauthenticatedIdentities = allowUnauthenticated
	pool.AllowClassicFlow = allowClassicFlow
	// api_op_CreateIdentityPool.go: "Once you have set a developer provider name, you
	// cannot change it." A pool with one already set silently keeps it; only a pool that
	// has never had one can adopt a new value via Update.
	if developerProviderName != "" && pool.DeveloperProviderName == "" {
		pool.DeveloperProviderName = developerProviderName
	}
	pool.IdentityProviders = cloneProviders(providers)
	pool.SupportedLoginProviders = cloneStringMap(supportedLoginProviders)
	if tags != nil {
		pool.Tags = cloneStringMap(tags)
	}

	if len(opts) > 0 && opts[0] != nil {
		pool.OpenIDConnectProviderARNs = cloneStringSlice(opts[0].OpenIDConnectProviderARNs)
		pool.SamlProviderARNs = cloneStringSlice(opts[0].SamlProviderARNs)
	}

	return clonePool(pool), nil
}

// renamePoolIfNeeded renames pool to name when name is non-empty and different from
// the pool's current name, after checking the new name is not already taken in region.
// Renaming an indexed field (IdentityPoolName, which the byName index keys on) in place
// would leave that index stale (see store.Table.AddIndex's doc comment), so the pool is
// first deleted -- which evicts it from every index using its pre-rename state -- then
// mutated, then re-inserted so every index picks up the new state. Must be called with
// b.mu held.
func (b *InMemoryBackend) renamePoolIfNeeded(region string, pool *IdentityPool, name string) error {
	if name == "" || name == pool.IdentityPoolName {
		return nil
	}

	if b.poolNameTaken(region, name) {
		return fmt.Errorf(
			"%w: identity pool %q already exists",
			ErrIdentityPoolAlreadyExists,
			name,
		)
	}

	b.poolDelete(region, pool.IdentityPoolID)
	pool.IdentityPoolName = name
	b.poolPut(pool)

	return nil
}
