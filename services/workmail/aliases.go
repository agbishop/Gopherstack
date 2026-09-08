package workmail

import (
	"fmt"
	"sort"
)

// maxAliasesPerUser is AWS's documented, non-adjustable quota ("Maximum
// number of aliases per user | 100. This is a hard quota and can't be
// changed.", docs.aws.amazon.com/workmail/latest/adminguide/
// workmail_limits.html). Documentation-sourced, not verified against the
// pinned SDK (no wire-visible quota field exists to check it against).
// Applied per entity (user/group/resource), matching the per-entity alias
// bookkeeping this backend already keeps -- not per organization.
const maxAliasesPerUser = 100

// --- Aliases ---

// CreateAlias creates an email alias for an entity.
func (b *InMemoryBackend) CreateAlias(orgID, entityID, alias string) error {
	b.mu.Lock("CreateAlias")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}

	if ta, exists := b.globalAliases.Get(alias); exists && ta.OrgID == orgID {
		return fmt.Errorf("%w: alias %q already in use", ErrEmailInUse, alias)
	}

	// Verify entity exists.
	var actualID string
	if u := b.findUser(orgID, entityID); u != nil {
		actualID = u.UserID
	} else if g := b.findGroup(orgID, entityID); g != nil {
		actualID = g.GroupID
	} else if r := b.findResource(orgID, entityID); r != nil {
		actualID = r.ResourceID
	} else {
		return fmt.Errorf("%w: entity %q not found", ErrNotFound, entityID)
	}

	if len(b.aliases[orgID][actualID]) >= maxAliasesPerUser {
		return fmt.Errorf("%w: entity %q already has %d aliases", ErrLimitExceeded, entityID, maxAliasesPerUser)
	}

	b.aliases[orgID][actualID] = append(b.aliases[orgID][actualID], alias)
	b.globalAliases.Put(&trackedAlias{Alias: alias, OrgID: orgID, EntityID: actualID})

	return nil
}

// DeleteAlias removes an email alias.
func (b *InMemoryBackend) DeleteAlias(orgID, entityID, alias string) error {
	b.mu.Lock("DeleteAlias")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}

	var actualID string
	if u := b.findUser(orgID, entityID); u != nil {
		actualID = u.UserID
	} else if g := b.findGroup(orgID, entityID); g != nil {
		actualID = g.GroupID
	} else if r := b.findResource(orgID, entityID); r != nil {
		actualID = r.ResourceID
	} else {
		return fmt.Errorf("%w: entity %q not found", ErrNotFound, entityID)
	}

	aliases := b.aliases[orgID][actualID]
	found := false
	newAliases := make([]string, 0, len(aliases))
	for _, a := range aliases {
		if a == alias {
			found = true

			continue
		}
		newAliases = append(newAliases, a)
	}
	if !found {
		return fmt.Errorf("%w: alias %q not found", ErrNotFound, alias)
	}
	b.aliases[orgID][actualID] = newAliases
	b.globalAliases.Delete(alias)

	return nil
}

// ListAliases returns aliases for an entity.
func (b *InMemoryBackend) ListAliases(
	orgID, entityID string,
	maxResults int32,
	nextToken string,
) ([]string, string, error) {
	b.mu.RLock("ListAliases")
	defer b.mu.RUnlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return nil, "", fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}

	var actualID string
	var primaryEmail string
	if u := b.findUser(orgID, entityID); u != nil {
		actualID = u.UserID
		primaryEmail = u.Email
	} else if g := b.findGroup(orgID, entityID); g != nil {
		actualID = g.GroupID
		primaryEmail = g.Email
	} else if r := b.findResource(orgID, entityID); r != nil {
		actualID = r.ResourceID
		primaryEmail = r.Email
	} else {
		return nil, "", fmt.Errorf("%w: entity %q not found", ErrNotFound, entityID)
	}

	all := make([]string, 0)
	if primaryEmail != "" {
		all = append(all, primaryEmail)
	}
	all = append(all, b.aliases[orgID][actualID]...)
	sort.Strings(all)

	items, next := paginate(all, maxResults, nextToken)

	return items, next, nil
}
