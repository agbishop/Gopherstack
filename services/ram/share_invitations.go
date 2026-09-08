package ram

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// invitationARN builds an ARN for a resource share invitation.
func (b *InMemoryBackend) invitationARN(id string) string {
	return arn.Build("ram", b.region, b.accountID, "resource-share-invitation/"+id)
}

// AddInvitationInternal adds a pre-built invitation for testing or seeding.
func (b *InMemoryBackend) AddInvitationInternal(inv *ResourceShareInvitation) {
	b.mu.Lock("AddInvitationInternal")
	defer b.mu.Unlock()

	b.invitations.Put(inv)
}

// AcceptResourceShareInvitation accepts a pending resource share invitation.
func (b *InMemoryBackend) AcceptResourceShareInvitation(
	invitationARN string,
) (*ResourceShareInvitation, error) {
	b.mu.Lock("AcceptResourceShareInvitation")
	defer b.mu.Unlock()

	inv, ok := b.invitations.Get(invitationARN)
	if !ok {
		return nil, fmt.Errorf("%w: invitation %s not found", ErrInvitationNotFound, invitationARN)
	}

	now := time.Now()
	if b.expireInvitationLocked(inv, now) {
		return nil, fmt.Errorf("%w: invitation %s has expired", ErrInvitationExpired, invitationARN)
	}

	switch inv.Status {
	case invitationStatusAccepted:
		return nil, fmt.Errorf("%w: invitation %s already accepted", ErrInvitationAlreadyAccepted, invitationARN)
	case invitationStatusRejected:
		return nil, fmt.Errorf("%w: invitation %s already rejected", ErrInvitationAlreadyRejected, invitationARN)
	case invitationStatusExpired:
		return nil, fmt.Errorf("%w: invitation %s has expired", ErrInvitationExpired, invitationARN)
	}

	inv.Status = invitationStatusAccepted
	inv.LastUpdatedTime = now

	return cloneInvitation(inv), nil
}

// expireInvitationLocked lazily transitions a still-PENDING invitation past its
// expiry window to EXPIRED, disassociating its receiver's principal association
// (see invitationExpiryWindow). Caller must hold the write lock. Returns true if inv
// was transitioned by this call.
func (b *InMemoryBackend) expireInvitationLocked(inv *ResourceShareInvitation, now time.Time) bool {
	if inv.Status != invitationStatusPending || now.Sub(inv.CreationTime) < invitationExpiryWindow {
		return false
	}

	inv.Status = invitationStatusExpired
	inv.LastUpdatedTime = now
	b.disassociateReceiverPrincipalsLocked(inv.ResourceShareARN, inv.ReceiverAccountID, now)

	return true
}

// disassociateReceiverPrincipalsLocked marks any ASSOCIATED principal association on
// shareARN belonging to receiverAcctID as DISASSOCIATED. Mirrors AWS: a principal who
// never accepts an invitation (rejects it, or lets it expire) is disassociated. Caller
// must hold the write lock.
func (b *InMemoryBackend) disassociateReceiverPrincipalsLocked(shareARN, receiverAcctID string, now time.Time) {
	for _, a := range b.associations {
		if a.ResourceShareARN != shareARN ||
			a.AssociationType != associationTypePrincipal ||
			a.Status != associationStatusAssociated {
			continue
		}

		if principalReceiverAccountID(a.AssociatedEntity) != receiverAcctID {
			continue
		}

		a.Status = associationStatusDisassociated
		a.LastUpdatedTime = now
	}
}

// createInvitationLocked creates a pending invitation without acquiring a lock.
// Caller must hold the write lock.
func (b *InMemoryBackend) createInvitationLocked(shareARN, shareNm, receiverAcctID string) {
	invID := uuid.NewString()
	invARN := b.invitationARN(invID)
	now := time.Now()

	b.invitations.Put(&ResourceShareInvitation{
		InvitationARN:     invARN,
		ResourceShareARN:  shareARN,
		ResourceShareName: shareNm,
		SenderAccountID:   b.accountID,
		ReceiverAccountID: receiverAcctID,
		Status:            invitationStatusPending,
		CreationTime:      now,
		LastUpdatedTime:   now,
	})
}

// principalReceiverAccountID extracts the effective AWS account ID from a principal string.
// For 12-digit account IDs the string itself is returned; for ARNs the account field is extracted.
func principalReceiverAccountID(principal string) string {
	if len(principal) == accountIDLen {
		return principal
	}

	// ARN format: arn:partition:service:region:account-id:...
	parts := strings.SplitN(principal, ":", arnPartCountPrincipal)
	if len(parts) >= arnAccountIdx+1 && parts[0] == arnPrefix {
		return parts[arnAccountIdx]
	}

	return principal
}

// CreateInvitation creates a pending invitation for a resource share.
// This is a helper for the mock that mirrors what AWS does when AssociateResourceShare is called with principals.
func (b *InMemoryBackend) CreateInvitation(
	shareARN, shareNm, senderAcctID, receiverAcctID string,
) *ResourceShareInvitation {
	b.mu.Lock("CreateInvitation")
	defer b.mu.Unlock()

	invID := uuid.NewString()
	invARN := b.invitationARN(invID)
	now := time.Now()

	inv := &ResourceShareInvitation{
		InvitationARN:     invARN,
		ResourceShareARN:  shareARN,
		ResourceShareName: shareNm,
		SenderAccountID:   senderAcctID,
		ReceiverAccountID: receiverAcctID,
		Status:            invitationStatusPending,
		CreationTime:      now,
		LastUpdatedTime:   now,
	}
	b.invitations.Put(inv)

	return cloneInvitation(inv)
}

// GetResourceShareInvitations returns invitations filtered by ARN or resource share ARN,
// sorted by creation time (oldest first) for deterministic output. Lazily expires any
// still-PENDING invitation past invitationExpiryWindow before reading.
func (b *InMemoryBackend) GetResourceShareInvitations(
	invitationARNs, shareARNs []string,
) []*ResourceShareInvitation {
	b.mu.Lock("GetResourceShareInvitations")
	defer b.mu.Unlock()

	now := time.Now()
	for _, inv := range b.invitations.All() {
		b.expireInvitationLocked(inv, now)
	}

	arnSet := make(map[string]struct{}, len(invitationARNs))

	for _, a := range invitationARNs {
		arnSet[a] = struct{}{}
	}

	shareSet := make(map[string]struct{}, len(shareARNs))

	for _, s := range shareARNs {
		shareSet[s] = struct{}{}
	}

	result := make([]*ResourceShareInvitation, 0, b.invitations.Len())

	for _, inv := range b.invitations.All() {
		if len(arnSet) > 0 {
			if _, ok := arnSet[inv.InvitationARN]; !ok {
				continue
			}
		}

		if len(shareSet) > 0 {
			if _, ok := shareSet[inv.ResourceShareARN]; !ok {
				continue
			}
		}

		result = append(result, cloneInvitation(inv))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreationTime.Before(result[j].CreationTime)
	})

	return result
}

// RejectResourceShareInvitation rejects a pending resource share invitation.
func (b *InMemoryBackend) RejectResourceShareInvitation(
	invitationARN string,
) (*ResourceShareInvitation, error) {
	b.mu.Lock("RejectResourceShareInvitation")
	defer b.mu.Unlock()

	inv, ok := b.invitations.Get(invitationARN)
	if !ok {
		return nil, fmt.Errorf("%w: invitation %s not found", ErrInvitationNotFound, invitationARN)
	}

	now := time.Now()
	if b.expireInvitationLocked(inv, now) {
		return nil, fmt.Errorf("%w: invitation %s has expired", ErrInvitationExpired, invitationARN)
	}

	switch inv.Status {
	case invitationStatusAccepted:
		return nil, fmt.Errorf("%w: invitation %s already accepted", ErrInvitationAlreadyAccepted, invitationARN)
	case invitationStatusRejected:
		return nil, fmt.Errorf("%w: invitation %s already rejected", ErrInvitationAlreadyRejected, invitationARN)
	case invitationStatusExpired:
		return nil, fmt.Errorf("%w: invitation %s has expired", ErrInvitationExpired, invitationARN)
	}

	inv.Status = invitationStatusRejected
	inv.LastUpdatedTime = now
	b.disassociateReceiverPrincipalsLocked(inv.ResourceShareARN, inv.ReceiverAccountID, now)

	return cloneInvitation(inv), nil
}
