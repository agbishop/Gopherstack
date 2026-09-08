package guardduty

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

// CreateMembers creates member accounts for a detector.
func (b *InMemoryBackend) CreateMembers(
	detectorID string,
	accountDetails []map[string]any,
) ([]*Member, []map[string]any) {
	b.mu.Lock("CreateMembers")
	defer b.mu.Unlock()

	now := time.Now().UTC()

	var created []*Member
	var unprocessed []map[string]any

	if !b.detectors.Has(detectorID) {
		for _, acc := range accountDetails {
			unprocessed = append(unprocessed, map[string]any{
				"accountId": acc["accountId"],   //nolint:goconst // existing issue.
				"result":    "DetectorNotFound", //nolint:goconst // existing issue.
			})
		}

		return nil, unprocessed
	}

	for _, acc := range accountDetails {
		accountID, _ := acc["accountId"].(string)
		email, _ := acc["email"].(string)

		if accountID == "" {
			unprocessed = append(unprocessed, map[string]any{
				"accountId": accountID,
				"result":    "InvalidInput",
			})

			continue
		}

		if b.members.Has(detectorKey(detectorID, accountID)) {
			unprocessed = append(unprocessed, map[string]any{
				"accountId": accountID,
				"result":    "ResourceConflictException",
			})

			continue
		}

		m := &Member{
			AccountID:          accountID,
			AdministratorID:    b.accountID,
			MasterID:           b.accountID,
			DetectorID:         detectorID,
			Email:              email,
			RelationshipStatus: "Created",
			UpdatedAt:          now,
		}
		b.members.Put(m)
		created = append(created, m)
	}

	return created, unprocessed
}

// DeleteMembers removes member accounts from a detector. Real GuardDuty
// rejects every detector-scoped member operation with an unknown DetectorId
// (see ListMembers/CreateMembers, which already check this) -- this and its
// six siblings below did not, silently returning 200 with every account
// listed unprocessed instead.
func (b *InMemoryBackend) DeleteMembers(detectorID string, accountIDs []string) ([]map[string]any, error) {
	b.mu.Lock("DeleteMembers")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	if b.rejectOrgMembersStillInOrg(detectorID, accountIDs) {
		return nil, ErrValidation
	}

	var unprocessed []map[string]any

	for _, id := range accountIDs {
		if !b.members.Delete(detectorKey(detectorID, id)) {
			unprocessed = append(unprocessed, map[string]any{
				"accountId": id,
				"result":    "ResourceNotFoundException", //nolint:goconst // existing issue.
			})
		}
	}

	return unprocessed, nil
}

// autoEnableOrgMembersAll reports whether the detector's org config has
// autoEnableOrganizationMembers set to ALL.
func (b *InMemoryBackend) autoEnableOrgMembersAll(detectorID string) bool {
	cfg, ok := b.orgConfigs.Get(detectorID)

	return ok && cfg.AutoEnableOrganizationMembers == "ALL"
}

// rejectOrgMembersStillInOrg reports whether DeleteMembers/
// DisassociateMembers/StopMonitoringMembers must reject this call: the
// detector's autoEnableOrganizationMembers is ALL, and at least one
// requested account is still in the AWS Organization per the wired
// Organizations backend (see cross_service.go's stillInOrganization).
// Matches the real ops' doc text, e.g. DisassociateMembers: "you'll receive
// an error if you attempt to disassociate a member account before removing
// them from your organization" -- the error is conditioned on org
// membership, not merely on the ALL setting (gopherstack-uu0n).
func (b *InMemoryBackend) rejectOrgMembersStillInOrg(detectorID string, accountIDs []string) bool {
	if !b.autoEnableOrgMembersAll(detectorID) {
		return false
	}

	return slices.ContainsFunc(accountIDs, b.stillInOrganization)
}

// GetMembers retrieves member account details.
func (b *InMemoryBackend) GetMembers(detectorID string, accountIDs []string) ([]*Member, []map[string]any, error) {
	b.mu.RLock("GetMembers")
	defer b.mu.RUnlock()

	if !b.detectors.Has(detectorID) {
		return nil, nil, ErrDetectorNotFound
	}

	var found []*Member
	var unprocessed []map[string]any

	for _, id := range accountIDs {
		if m, ok := b.members.Get(detectorKey(detectorID, id)); ok {
			cp := *m
			found = append(found, &cp)

			continue
		}

		unprocessed = append(unprocessed, map[string]any{
			"accountId": id,
			"result":    "ResourceNotFoundException",
		})
	}

	return found, unprocessed, nil
}

// InviteMembers sends invitations to member accounts.
func (b *InMemoryBackend) InviteMembers(detectorID string, accountIDs []string) ([]map[string]any, error) {
	b.mu.Lock("InviteMembers")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)

	var unprocessed []map[string]any

	for _, id := range accountIDs {
		b.memberSeq++
		invitationID := fmt.Sprintf("%s-invite-%d", b.accountID, b.memberSeq)

		b.invitations.Put(&Invitation{
			AccountID:          id,
			InvitationID:       invitationID,
			InvitedAt:          now,
			RelationshipStatus: "Invited",
		})

		if m, ok := b.members.Get(detectorKey(detectorID, id)); ok {
			m.RelationshipStatus = "Invited"
			m.InvitedAt = now
		}
	}

	return unprocessed, nil
}

// ListMembers returns member accounts for a detector.
func (b *InMemoryBackend) ListMembers(
	detectorID string, onlyAssociated bool, maxResults int32, nextToken string,
) ([]*Member, string, error) {
	b.mu.RLock("ListMembers")
	defer b.mu.RUnlock()

	if !b.detectors.Has(detectorID) {
		return nil, "", ErrDetectorNotFound
	}

	var all []*Member

	for _, m := range b.membersByDetector.Get(detectorID) {
		if onlyAssociated && m.RelationshipStatus != "Enabled" { //nolint:goconst // existing issue.
			continue
		}

		cp := *m
		all = append(all, &cp)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].AccountID < all[j].AccountID })

	offset, err := decodeToken(nextToken)
	if err != nil {
		return nil, "", ErrValidation
	}

	size := resolvePageSize(int(maxResults))
	page, next := paginate(all, offset, size)

	return page, next, nil
}

// StartMonitoringMembers starts monitoring member accounts.
func (b *InMemoryBackend) StartMonitoringMembers(detectorID string, accountIDs []string) ([]map[string]any, error) {
	b.mu.Lock("StartMonitoringMembers")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	var unprocessed []map[string]any

	for _, id := range accountIDs {
		if m, ok := b.members.Get(detectorKey(detectorID, id)); ok {
			m.RelationshipStatus = "Enabled"

			continue
		}

		unprocessed = append(unprocessed, map[string]any{
			"accountId": id,
			"result":    "ResourceNotFoundException",
		})
	}

	return unprocessed, nil
}

// StopMonitoringMembers stops monitoring member accounts.
func (b *InMemoryBackend) StopMonitoringMembers(detectorID string, accountIDs []string) ([]map[string]any, error) {
	b.mu.Lock("StopMonitoringMembers")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	if b.rejectOrgMembersStillInOrg(detectorID, accountIDs) {
		return nil, ErrValidation
	}

	var unprocessed []map[string]any

	for _, id := range accountIDs {
		if m, ok := b.members.Get(detectorKey(detectorID, id)); ok {
			m.RelationshipStatus = "Disabled"

			continue
		}

		unprocessed = append(unprocessed, map[string]any{
			"accountId": id,
			"result":    "ResourceNotFoundException",
		})
	}

	return unprocessed, nil
}

// DisassociateMembers disassociates member accounts from a detector.
func (b *InMemoryBackend) DisassociateMembers(detectorID string, accountIDs []string) ([]map[string]any, error) {
	b.mu.Lock("DisassociateMembers")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	if b.rejectOrgMembersStillInOrg(detectorID, accountIDs) {
		return nil, ErrValidation
	}

	var unprocessed []map[string]any

	for _, id := range accountIDs {
		if m, ok := b.members.Get(detectorKey(detectorID, id)); ok {
			m.RelationshipStatus = "Removed"

			continue
		}

		unprocessed = append(unprocessed, map[string]any{
			"accountId": id,
			"result":    "ResourceNotFoundException",
		})
	}

	return unprocessed, nil
}

// GetMemberDetectors returns detector configurations for member accounts.
func (b *InMemoryBackend) GetMemberDetectors(
	detectorID string,
	accountIDs []string,
) ([]map[string]any, []map[string]any, error) {
	b.mu.RLock("GetMemberDetectors")
	defer b.mu.RUnlock()

	if !b.detectors.Has(detectorID) {
		return nil, nil, ErrDetectorNotFound
	}

	var memberDetails []map[string]any
	var unprocessed []map[string]any

	for _, id := range accountIDs {
		if m, ok := b.members.Get(detectorKey(detectorID, id)); ok {
			memberDetails = append(memberDetails, map[string]any{
				"accountId":  m.AccountID,
				"detectorId": m.DetectorID, //nolint:goconst // existing issue.
				"features":   []any{},      //nolint:goconst // existing issue.
			})

			continue
		}

		unprocessed = append(unprocessed, map[string]any{
			"accountId": id,
			"result":    "ResourceNotFoundException",
		})
	}

	return memberDetails, unprocessed, nil
}

// UpdateMemberDetectors updates detector configurations for member accounts.
func (b *InMemoryBackend) UpdateMemberDetectors(
	detectorID string,
	accountIDs []string,
) ([]map[string]any, error) {
	b.mu.Lock("UpdateMemberDetectors")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	var unprocessed []map[string]any

	for _, id := range accountIDs {
		if !b.members.Has(detectorKey(detectorID, id)) {
			unprocessed = append(unprocessed, map[string]any{
				"accountId": id,
				"result":    "ResourceNotFoundException",
			})
		}
	}

	return unprocessed, nil
}

// AcceptAdministratorInvitation records acceptance of an administrator invitation.
func (b *InMemoryBackend) AcceptAdministratorInvitation(detectorID, administratorID, invitationID string) error {
	b.mu.Lock("AcceptAdministratorInvitation")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return ErrDetectorNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)
	acc := &AdminAccount{
		AccountID:          administratorID,
		InvitationID:       invitationID,
		InvitedAt:          now,
		RelationshipStatus: "Enabled",
	}
	acc.detectorID = detectorID
	b.adminAccounts.Put(acc)

	return nil
}

// AcceptInvitation records acceptance of a legacy master invitation.
func (b *InMemoryBackend) AcceptInvitation(detectorID, masterID, invitationID string) error {
	b.mu.Lock("AcceptInvitation")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return ErrDetectorNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)
	acc := &AdminAccount{
		AccountID:          masterID,
		InvitationID:       invitationID,
		InvitedAt:          now,
		RelationshipStatus: "Enabled",
	}
	acc.detectorID = detectorID
	b.adminAccounts.Put(acc)

	return nil
}

// DeclineInvitations declines invitations from specified accounts.
func (b *InMemoryBackend) DeclineInvitations(accountIDs []string) []map[string]any {
	b.mu.Lock("DeclineInvitations")
	defer b.mu.Unlock()

	var unprocessed []map[string]any

	for _, inv := range b.invitations.All() {
		for _, id := range accountIDs {
			if inv.AccountID == id {
				inv.RelationshipStatus = "Declined"
				b.invitations.Delete(inv.InvitationID)
			}
		}
	}

	return unprocessed
}

// DeleteInvitations deletes invitations from specified accounts.
func (b *InMemoryBackend) DeleteInvitations(accountIDs []string) []map[string]any {
	b.mu.Lock("DeleteInvitations")
	defer b.mu.Unlock()

	var unprocessed []map[string]any

	for _, inv := range b.invitations.All() {
		for _, id := range accountIDs {
			if inv.AccountID == id {
				b.invitations.Delete(inv.InvitationID)
			}
		}
	}

	return unprocessed
}

// GetInvitationsCount returns the count of pending invitations.
func (b *InMemoryBackend) GetInvitationsCount() int {
	b.mu.RLock("GetInvitationsCount")
	defer b.mu.RUnlock()

	return b.invitations.Len()
}

// ListInvitations returns all pending invitations.
func (b *InMemoryBackend) ListInvitations(maxResults int32, nextToken string) ([]*Invitation, string) {
	b.mu.RLock("ListInvitations")
	defer b.mu.RUnlock()

	items := b.invitations.Snapshot()
	all := make([]*Invitation, 0, len(items))

	for _, inv := range items {
		cp := *inv
		all = append(all, &cp)
	}

	sort.Slice(all, func(i, j int) bool { return all[i].AccountID < all[j].AccountID })

	offset, err := decodeToken(nextToken)
	if err != nil {
		return nil, ""
	}

	size := resolvePageSize(int(maxResults))
	page, next := paginate(all, offset, size)

	return page, next
}
