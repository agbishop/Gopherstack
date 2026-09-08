package ram

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// isValidResourceARN reports whether s is syntactically ARN-shaped: it starts
// with "arn:" and has at least five colon separators (six colon-delimited
// segments), matching the resourcegroups package's own resource-ARN check.
func isValidResourceARN(s string) bool {
	return strings.HasPrefix(s, "arn:") && strings.Count(s, ":") >= 5
}

// validateResourceARNs returns ErrMalformedArn if any resourceARNs entry isn't
// ARN-shaped. CreateResourceShare and AssociateResourceShare both model
// MalformedArnException for a malformed resourceArns entry (see ErrMalformedArn).
func validateResourceARNs(resourceARNs []string) error {
	for _, r := range resourceARNs {
		if !isValidResourceARN(r) {
			return fmt.Errorf("%w: %q is not a valid ARN", ErrMalformedArn, r)
		}
	}

	return nil
}

// CreateResourceShare creates a new resource share.
func (b *InMemoryBackend) CreateResourceShare(
	name string,
	allowExternalPrincipals bool,
	tags map[string]string,
	principals, resourceARNs []string,
) (*ResourceShare, error) {
	b.mu.Lock("CreateResourceShare")
	defer b.mu.Unlock()

	// Check for name collision.
	for _, rs := range b.resourceShares.All() {
		if rs.Name == name && rs.Status != statusDeleted {
			return nil, fmt.Errorf("%w: resource share %s already exists", ErrAlreadyExists, name)
		}
	}

	// Validate every principal against AllowExternalPrincipals before
	// mutating any state. Checking this inside the mutation loop below would
	// leave an orphaned resource share (and any associations/invitations
	// already appended for earlier principals) committed to the backend even
	// though the overall call returns an error to the caller.
	for _, p := range principals {
		if b.isExternalPrincipal(p) && !allowExternalPrincipals {
			return nil, fmt.Errorf(
				"%w: external principals not allowed for this resource share",
				ErrValidation,
			)
		}
	}

	if err := validateResourceARNs(resourceARNs); err != nil {
		return nil, err
	}

	now := time.Now()
	// Use a stable UUID-based resource share ID so the ARN remains valid
	// even if the share is later renamed via UpdateResourceShare.
	shareID := uuid.NewString()
	shareARN := arn.Build("ram", b.region, b.accountID, "resource-share/"+shareID)

	rs := &ResourceShare{
		Name:                    name,
		ARN:                     shareARN,
		OwningAccountID:         b.accountID,
		Status:                  statusActive,
		AllowExternalPrincipals: allowExternalPrincipals,
		CreationTime:            now,
		LastUpdatedTime:         now,
		Tags:                    mergeTags(nil, tags),
	}
	b.resourceShares.Put(rs)

	// Add principal associations. AllowExternalPrincipals was already validated above.
	for _, p := range principals {
		external := b.isExternalPrincipal(p)

		b.associations = append(b.associations, &ResourceShareAssociation{
			ResourceShareARN:  shareARN,
			ResourceShareName: name,
			AssociatedEntity:  p,
			AssociationType:   associationTypePrincipal,
			Status:            associationStatusAssociated,
			External:          external,
			CreationTime:      now,
			LastUpdatedTime:   now,
		})

		if external {
			receiverID := principalReceiverAccountID(p)
			b.createInvitationLocked(shareARN, name, receiverID)
		}
	}

	// Add resource associations.
	for _, r := range resourceARNs {
		b.associations = append(b.associations, &ResourceShareAssociation{
			ResourceShareARN:  shareARN,
			ResourceShareName: name,
			AssociatedEntity:  r,
			AssociationType:   associationTypeResource,
			Status:            associationStatusAssociated,
			CreationTime:      now,
			LastUpdatedTime:   now,
		})
	}

	return cloneResourceShare(rs), nil
}

// GetResourceShare returns a resource share by ARN.
func (b *InMemoryBackend) GetResourceShare(shareARN string) (*ResourceShare, error) {
	b.mu.RLock("GetResourceShare")
	defer b.mu.RUnlock()

	rs, ok := b.resourceShares.Get(shareARN)
	if !ok || rs.Status == statusDeleted {
		return nil, fmt.Errorf("%w: resource share %s not found", ErrNotFound, shareARN)
	}

	return cloneResourceShare(rs), nil
}

// ListResourceShares returns resource shares matching the given owner and optional status filter.
// resourceOwner must be "SELF" or "OTHER-ACCOUNTS".
//   - "SELF": shares owned by this account (not deleted, optionally filtered by status).
//   - "OTHER-ACCOUNTS": shares owned by another account where this account is a PRINCIPAL.
//
// Pass status="" to return all matching shares, or e.g. "ACTIVE" to filter by status.
func (b *InMemoryBackend) ListResourceShares(resourceOwner, status string) []*ResourceShare {
	b.mu.RLock("ListResourceShares")
	defer b.mu.RUnlock()

	var list []*ResourceShare

	switch resourceOwner {
	case resourceOwnerSelf, "":
		list = b.listOwnedShares(status)
	case resourceOwnerOtherAccounts:
		list = b.listSharedWithMe(status)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return list
}

// listOwnedShares returns non-deleted shares owned by this account, filtered by status.
// Caller must hold at least a read lock.
func (b *InMemoryBackend) listOwnedShares(status string) []*ResourceShare {
	list := make([]*ResourceShare, 0, b.resourceShares.Len())

	for _, rs := range b.resourceShares.All() {
		if rs.Status == statusDeleted || rs.OwningAccountID != b.accountID {
			continue
		}

		if status != "" && rs.Status != status {
			continue
		}

		list = append(list, cloneResourceShare(rs))
	}

	return list
}

// listSharedWithMe returns non-deleted shares owned by other accounts where this
// account appears as an active PRINCIPAL association. Caller must hold at least a read lock.
func (b *InMemoryBackend) listSharedWithMe(status string) []*ResourceShare {
	// Build a set of share ARNs where this account is an active PRINCIPAL.
	principalShares := make(map[string]struct{})

	for _, a := range b.associations {
		if a.AssociationType == associationTypePrincipal &&
			a.Status == associationStatusAssociated &&
			a.AssociatedEntity == b.accountID {
			principalShares[a.ResourceShareARN] = struct{}{}
		}
	}

	list := make([]*ResourceShare, 0)

	for _, rs := range b.resourceShares.All() {
		if rs.Status == statusDeleted || rs.OwningAccountID == b.accountID {
			continue
		}

		if _, ok := principalShares[rs.ARN]; !ok {
			continue
		}

		if status != "" && rs.Status != status {
			continue
		}

		list = append(list, cloneResourceShare(rs))
	}

	return list
}

// UpdateResourceShare updates an existing resource share.
// If name is changed, all matching associations are updated to reflect the new name.
func (b *InMemoryBackend) UpdateResourceShare(
	shareARN, name string,
	allowExternalPrincipals *bool,
) (*ResourceShare, error) {
	b.mu.Lock("UpdateResourceShare")
	defer b.mu.Unlock()

	rs, ok := b.resourceShares.Get(shareARN)
	if !ok || rs.Status == statusDeleted {
		return nil, fmt.Errorf("%w: resource share %s not found", ErrNotFound, shareARN)
	}

	if name != "" {
		rs.Name = name
		// Keep association ResourceShareName in sync.
		for _, a := range b.associations {
			if a.ResourceShareARN == shareARN {
				a.ResourceShareName = name
			}
		}
	}

	if allowExternalPrincipals != nil {
		rs.AllowExternalPrincipals = *allowExternalPrincipals
	}

	rs.LastUpdatedTime = time.Now()

	return cloneResourceShare(rs), nil
}

// DeleteResourceShare deletes a resource share and removes it from the store.
// Associations for the share are disassociated before the share is removed so
// that ListResources / ListPrincipals no longer return them.
func (b *InMemoryBackend) DeleteResourceShare(shareARN string) error {
	b.mu.Lock("DeleteResourceShare")
	defer b.mu.Unlock()

	rs, ok := b.resourceShares.Get(shareARN)
	if !ok || rs.Status == statusDeleted {
		return fmt.Errorf("%w: resource share %s not found", ErrNotFound, shareARN)
	}

	now := time.Now()

	// Disassociate all associations for this share.
	for _, a := range b.associations {
		if a.ResourceShareARN == shareARN {
			a.Status = associationStatusDisassociated
			a.LastUpdatedTime = now
		}
	}

	// Soft-delete: mark as DELETED but keep in the map so callers can still
	// retrieve it with a DELETED status filter (matches real AWS behaviour).
	rs.Status = statusDeleted
	rs.LastUpdatedTime = now

	return nil
}

// isExternalPrincipal returns true if principal is not owned by the backend account.
// AllowExternalPrincipals ("Specifies whether principals outside your organization...
// can be associated", api_op_CreateResourceShare.go) gates OTHER ACCOUNTS, not IAM
// identities within this account -- an IAM role/user ARN is only ever associable
// within the resource share's own account (AssociateResourceShare's Principals doc:
// "An ARN of an IAM role"/"An ARN of an IAM user"), so a same-account one is never
// external regardless of AllowExternalPrincipals. Organization/OU ARNs are always
// external even when their account segment (the org's management account) matches
// b.accountID: unlike an IAM ARN, an org/OU principal can admit arbitrary other member
// accounts, so it must still be gated.
func (b *InMemoryBackend) isExternalPrincipal(principal string) bool {
	// An account-ID principal is exactly accountIDLen digits. Anything else is external
	// unless it is a same-account IAM role/user ARN (handled below).
	if len(principal) == accountIDLen {
		for _, c := range principal {
			if c < '0' || c > '9' {
				return true
			}
		}

		return principal != b.accountID
	}

	parts := strings.SplitN(principal, ":", arnPartCountPrincipal)
	if len(parts) >= arnAccountIdx+1 && parts[0] == arnPrefix && parts[arnServiceIdx] == arnServiceIAM {
		return parts[arnAccountIdx] != b.accountID
	}

	return true
}

// AddResourceShareInternal inserts a resource share directly, bypassing validation.
// Useful for seeding test state.
func (b *InMemoryBackend) AddResourceShareInternal(rs *ResourceShare) {
	b.mu.Lock("AddResourceShareInternal")
	defer b.mu.Unlock()

	if rs.Tags == nil {
		rs.Tags = make(map[string]string)
	}

	b.resourceShares.Put(rs)
}

// ownerMatchesFilter reports whether the resource share identified by shareARN satisfies
// the given resourceOwner filter ("SELF" or "OTHER-ACCOUNTS").
// Returns false when the share is not found or the owner does not match.
func (b *InMemoryBackend) ownerMatchesFilter(shareARN, resourceOwner string) bool {
	rs, exists := b.resourceShares.Get(shareARN)
	if !exists {
		return false
	}

	switch resourceOwner {
	case resourceOwnerSelf:
		return rs.OwningAccountID == b.accountID
	case resourceOwnerOtherAccounts:
		return rs.OwningAccountID != b.accountID
	default:
		return true
	}
}

// PromoteResourceShareCreatedFromPolicy promotes a resource share to standard feature set.
// In this mock, it simply returns the existing share unchanged.
func (b *InMemoryBackend) PromoteResourceShareCreatedFromPolicy(
	shareARN string,
) (*ResourceShare, error) {
	b.mu.RLock("PromoteResourceShareCreatedFromPolicy")
	defer b.mu.RUnlock()

	rs, ok := b.resourceShares.Get(shareARN)
	if !ok || rs.Status == statusDeleted {
		return nil, fmt.Errorf("%w: resource share %s not found", ErrNotFound, shareARN)
	}

	return cloneResourceShare(rs), nil
}
