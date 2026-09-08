package ram

import (
	"fmt"
	"time"
)

// AssociateResourceShare associates principals or resource ARNs with a resource share.
// Entities already ASSOCIATED are silently skipped (idempotent), matching AWS behavior.
// An entity with an existing DISASSOCIATED row (produced by a prior DisassociateResourceShare)
// is reactivated in place -- a real re-association, since AWS keeps a single row per
// (share, entity) pair and transitions its status rather than accumulating duplicates.
// Returns deep copies of the associations that changed state so callers cannot mutate
// backend state.
func (b *InMemoryBackend) AssociateResourceShare(
	shareARN string,
	principals, resourceARNs []string,
) ([]*ResourceShareAssociation, error) {
	b.mu.Lock("AssociateResourceShare")
	defer b.mu.Unlock()

	rs, ok := b.resourceShares.Get(shareARN)
	if !ok || rs.Status == statusDeleted {
		return nil, fmt.Errorf("%w: resource share %s not found", ErrNotFound, shareARN)
	}

	// Index existing rows for this share by entity. Only an ASSOCIATED row means
	// "already associated" (skip); a DISASSOCIATED row is a reactivation candidate.
	active, inactive := b.indexAssociationsByEntityLocked(shareARN)

	// Validate every non-duplicate principal against AllowExternalPrincipals
	// before mutating any state. Checking this inside the mutation loop below
	// would leave associations (and invitations) already appended for
	// earlier principals committed to the backend even though the overall
	// call returns an error to the caller.
	if err := b.validateExternalPrincipalsLocked(rs, principals, active); err != nil {
		return nil, err
	}

	if err := validateResourceARNs(resourceARNs); err != nil {
		return nil, err
	}

	now := time.Now()
	added := make([]*ResourceShareAssociation, 0, len(principals)+len(resourceARNs))
	added = append(added, b.associatePrincipalsLocked(rs, principals, active, inactive, now)...)
	added = append(added, b.associateResourcesLocked(rs, resourceARNs, active, inactive, now)...)

	return added, nil
}

// validateExternalPrincipalsLocked returns an error if any principal not already
// ASSOCIATED (per active) is external and rs disallows external principals. Caller must
// hold the write lock.
func (b *InMemoryBackend) validateExternalPrincipalsLocked(
	rs *ResourceShare, principals []string, active map[string]struct{},
) error {
	for _, p := range principals {
		if _, dup := active[p]; dup {
			continue
		}

		if b.isExternalPrincipal(p) && !rs.AllowExternalPrincipals {
			return fmt.Errorf(
				"%w: external principals not allowed for resource share %s",
				ErrValidation,
				rs.ARN,
			)
		}
	}

	return nil
}

// associatePrincipalsLocked associates every non-duplicate principal, creating an
// invitation for external ones, and returns clones of the associations that changed
// state. Caller must hold the write lock and must have already validated principals via
// validateExternalPrincipalsLocked.
func (b *InMemoryBackend) associatePrincipalsLocked(
	rs *ResourceShare, principals []string,
	active map[string]struct{}, inactive map[string]*ResourceShareAssociation,
	now time.Time,
) []*ResourceShareAssociation {
	added := make([]*ResourceShareAssociation, 0, len(principals))

	for _, p := range principals {
		if _, dup := active[p]; dup {
			continue
		}

		external := b.isExternalPrincipal(p)
		assoc := b.reactivateOrCreateLocked(inactive, rs.ARN, rs.Name, p, associationTypePrincipal, external, now)
		added = append(added, cloneAssociation(assoc))

		if external {
			receiverID := principalReceiverAccountID(p)
			b.createInvitationLocked(rs.ARN, rs.Name, receiverID)
		}
	}

	return added
}

// associateResourcesLocked associates every non-duplicate resource ARN and returns
// clones of the associations that changed state. Caller must hold the write lock.
func (b *InMemoryBackend) associateResourcesLocked(
	rs *ResourceShare, resourceARNs []string,
	active map[string]struct{}, inactive map[string]*ResourceShareAssociation,
	now time.Time,
) []*ResourceShareAssociation {
	added := make([]*ResourceShareAssociation, 0, len(resourceARNs))

	for _, r := range resourceARNs {
		if _, dup := active[r]; dup {
			continue
		}

		assoc := b.reactivateOrCreateLocked(inactive, rs.ARN, rs.Name, r, associationTypeResource, false, now)
		added = append(added, cloneAssociation(assoc))
	}

	return added
}

// indexAssociationsByEntityLocked splits shareARN's existing association rows into two
// indexes keyed by AssociatedEntity: active (ASSOCIATED, presence-only) and inactive
// (any other status, e.g. DISASSOCIATED, keyed to the row itself so it can be
// reactivated in place). Caller must hold at least a read lock.
func (b *InMemoryBackend) indexAssociationsByEntityLocked(
	shareARN string,
) (map[string]struct{}, map[string]*ResourceShareAssociation) {
	active := make(map[string]struct{})
	inactive := make(map[string]*ResourceShareAssociation)

	for _, a := range b.associations {
		if a.ResourceShareARN != shareARN {
			continue
		}

		if a.Status == associationStatusAssociated {
			active[a.AssociatedEntity] = struct{}{}
		} else {
			inactive[a.AssociatedEntity] = a
		}
	}

	return active, inactive
}

// reactivateOrCreateLocked reactivates entity's existing DISASSOCIATED row (if inactive
// holds one) by flipping it back to ASSOCIATED in place, or appends a brand-new
// ASSOCIATED row when no prior row exists. Caller must hold the write lock.
func (b *InMemoryBackend) reactivateOrCreateLocked(
	inactive map[string]*ResourceShareAssociation,
	shareARN, shareName, entity, assocType string,
	external bool,
	now time.Time,
) *ResourceShareAssociation {
	if prior, ok := inactive[entity]; ok {
		prior.Status = associationStatusAssociated
		prior.StatusMessage = ""
		prior.External = external
		prior.LastUpdatedTime = now

		return prior
	}

	assoc := &ResourceShareAssociation{
		ResourceShareARN:  shareARN,
		ResourceShareName: shareName,
		AssociatedEntity:  entity,
		AssociationType:   assocType,
		Status:            associationStatusAssociated,
		External:          external,
		CreationTime:      now,
		LastUpdatedTime:   now,
	}
	b.associations = append(b.associations, assoc)

	return assoc
}

// DisassociateResourceShare marks principals or resource ARNs on a resource share as
// DISASSOCIATED. Rows are kept in place (soft-deleted), matching the same pattern
// DeleteResourceShare uses for every association on a deleted share -- this lets
// GetResourceShareAssociations(associationStatus=DISASSOCIATED) see the history, and
// lets a later AssociateResourceShare reactivate the row instead of accumulating
// duplicates (see reactivateOrCreateLocked). Only currently-ASSOCIATED rows are
// affected; disassociating an entity that is not currently associated is a no-op for
// that entity.
func (b *InMemoryBackend) DisassociateResourceShare(
	shareARN string,
	principals, resourceARNs []string,
) ([]*ResourceShareAssociation, error) {
	b.mu.Lock("DisassociateResourceShare")
	defer b.mu.Unlock()

	rs, ok := b.resourceShares.Get(shareARN)
	if !ok || rs.Status == statusDeleted {
		return nil, fmt.Errorf("%w: resource share %s not found", ErrNotFound, shareARN)
	}

	toRemove := make(map[string]struct{}, len(principals)+len(resourceARNs))

	for _, p := range principals {
		toRemove[p] = struct{}{}
	}

	for _, r := range resourceARNs {
		toRemove[r] = struct{}{}
	}

	now := time.Now()

	var updated []*ResourceShareAssociation

	for _, a := range b.associations {
		if a.ResourceShareARN != shareARN || a.Status != associationStatusAssociated {
			continue
		}

		if _, found := toRemove[a.AssociatedEntity]; !found {
			continue
		}

		a.Status = associationStatusDisassociated
		a.LastUpdatedTime = now
		updated = append(updated, cloneAssociation(a))
	}

	return updated, nil
}

// GetResourceShareAssociations returns associations for the given resource share ARNs and type.
func (b *InMemoryBackend) GetResourceShareAssociations(
	associationType string,
	shareARNs []string,
) []*ResourceShareAssociation {
	b.mu.RLock("GetResourceShareAssociations")
	defer b.mu.RUnlock()

	arnSet := make(map[string]struct{}, len(shareARNs))

	for _, a := range shareARNs {
		arnSet[a] = struct{}{}
	}

	result := make([]*ResourceShareAssociation, 0, len(b.associations))

	for _, a := range b.associations {
		if associationType != "" && a.AssociationType != associationType {
			continue
		}

		if len(arnSet) > 0 {
			if _, ok := arnSet[a.ResourceShareARN]; !ok {
				continue
			}
		}

		result = append(result, cloneAssociation(a))
	}

	return result
}
