package route53resolver

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// AssociateResolverQueryLogConfig associates a VPC with a query log config.
func (b *InMemoryBackend) AssociateResolverQueryLogConfig(
	ctx context.Context,
	queryLogConfigID, resourceID string,
) (*ResolverQueryLogConfigAssociation, error) {
	b.mu.Lock("AssociateResolverQueryLogConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	cfg, ok := b.queryLogConfigs.Get(regionalKey(region, queryLogConfigID))
	if !ok {
		return nil, fmt.Errorf(
			"%w: resolver query log config %s not found",
			ErrNotFound,
			queryLogConfigID,
		)
	}

	// AssociateResolverQueryLogConfig models ResourceExistsException ("The
	// resource that you tried to create already exists" -- API_route53resolver_
	// AssociateResolverQueryLogConfig.html); DisassociateResolverQueryLogConfig
	// already treats (ResolverQueryLogConfigId, ResourceId) as this
	// association's identity, so re-associating the same pair is the
	// duplicate this error covers.
	for _, assoc := range b.queryLogConfigAssociationsByRegion.Get(region) {
		if assoc.ResolverQueryLogConfigID == queryLogConfigID && assoc.ResourceID == resourceID {
			return nil, fmt.Errorf(
				"%w: resource %s is already associated with query log config %s",
				ErrAlreadyExists,
				resourceID,
				queryLogConfigID,
			)
		}
	}

	now := currentTime()
	id := "rqlca-" + uuid.New().String()[:8]
	assoc := &ResolverQueryLogConfigAssociation{
		ID:                       id,
		ResolverQueryLogConfigID: queryLogConfigID,
		ResourceID:               resourceID,
		Status:                   statusActive,
		CreationTime:             now,
		Region:                   region,
	}
	b.queryLogConfigAssociations.Put(assoc)

	// Increment AssociationCount on the config.
	cfg.AssociationCount++

	cp := *assoc

	return &cp, nil
}

// GetResolverQueryLogConfigAssociation retrieves an association by ID.
func (b *InMemoryBackend) GetResolverQueryLogConfigAssociation(
	ctx context.Context,
	id string,
) (*ResolverQueryLogConfigAssociation, error) {
	b.mu.RLock("GetResolverQueryLogConfigAssociation")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	assoc, ok := b.queryLogConfigAssociations.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf(
			"%w: resolver query log config association %s not found",
			ErrNotFound,
			id,
		)
	}
	cp := *assoc

	return &cp, nil
}

// DisassociateResolverQueryLogConfig removes a query log config association,
// looked up by the (queryLogConfigID, resourceID) pair -- this matches the
// real DisassociateResolverQueryLogConfig API, which takes
// ResolverQueryLogConfigId + ResourceId (not an opaque association ID; that
// only appears in Get/List responses).
func (b *InMemoryBackend) DisassociateResolverQueryLogConfig(
	ctx context.Context,
	queryLogConfigID, resourceID string,
) (*ResolverQueryLogConfigAssociation, error) {
	b.mu.Lock("DisassociateResolverQueryLogConfig")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	for _, assoc := range b.queryLogConfigAssociationsByRegion.Get(region) {
		if assoc.ResolverQueryLogConfigID != queryLogConfigID || assoc.ResourceID != resourceID {
			continue
		}

		cp := *assoc
		b.queryLogConfigAssociations.Delete(regionalKey(region, assoc.ID))

		// Decrement AssociationCount on the config.
		if cfg, ok := b.queryLogConfigs.Get(regionalKey(region, queryLogConfigID)); ok && cfg.AssociationCount > 0 {
			cfg.AssociationCount--
		}

		return &cp, nil
	}

	return nil, fmt.Errorf(
		"%w: resolver query log config association for config %s and resource %s not found",
		ErrNotFound,
		queryLogConfigID,
		resourceID,
	)
}

// ListResolverQueryLogConfigAssociations lists all query log config associations.
func (b *InMemoryBackend) ListResolverQueryLogConfigAssociations(
	ctx context.Context,
) []*ResolverQueryLogConfigAssociation {
	b.mu.RLock("ListResolverQueryLogConfigAssociations")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	regionAssocs := b.queryLogConfigAssociationsByRegion.Get(region)
	list := make([]*ResolverQueryLogConfigAssociation, 0, len(regionAssocs))
	for _, a := range regionAssocs {
		cp := *a
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	return list
}
