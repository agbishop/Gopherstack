package organizations

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	handshakeActionInvite                   = "INVITE"
	handshakeActionLeave                    = "LEAVE_ORGANIZATION"
	handshakeActionEnableFeatures           = "ENABLE_ALL_FEATURES"
	handshakeActionApproveAll               = "APPROVE_ALL_FEATURES"
	handshakeActionTransferResponsibility   = "TRANSFER_RESPONSIBILITY"
	handshakeResourceOrg                    = "ORGANIZATION"
	handshakeResourceMasterEmail            = "MASTER_EMAIL"
	handshakeResourceNotes                  = "NOTES"
	handshakeResourceResponsibilityTransfer = "RESPONSIBILITY_TRANSFER"
	handshakeResourceTransferStartTimestamp = "TRANSFER_START_TIMESTAMP"
	handshakeResourceTransferType           = "TRANSFER_TYPE"

	handshakeStateOpen     = "OPEN"
	handshakeStateCanceled = "CANCELED"
	handshakeStateAccepted = "ACCEPTED"
	handshakeStateDeclined = "DECLINED"
	handshakeStateExpired  = "EXPIRED"

	// handshakeExpirationDuration is the default lifetime of a handshake (AWS default: 15 days).
	handshakeExpirationDuration = 15 * 24 * time.Hour
)

// ResponsibilityTransferStatus values (types/enums.go:
// ResponsibilityTransferStatus). These mirror the underlying Handshake's
// State 1:1 except OPEN, which maps to REQUESTED -- see
// responsibilityTransferStatusForHandshakeState. WITHDRAWN has no backend
// path that produces it: it would require the source side to withdraw an
// invitation before the target acts, which this single-account backend
// doesn't model separately from Cancel (see
// ListInboundResponsibilityTransfers).
const (
	responsibilityTransferStatusRequested = "REQUESTED"
	responsibilityTransferStatusAccepted  = "ACCEPTED"
	responsibilityTransferStatusDeclined  = "DECLINED"
	responsibilityTransferStatusCanceled  = "CANCELED"
	responsibilityTransferStatusExpired   = "EXPIRED"

	// responsibilityTransferTypeBilling is the only supported
	// ResponsibilityTransferType value (types/enums.go).
	responsibilityTransferTypeBilling = "BILLING"

	// responsibilityTransferDirectionOutbound is the only direction this
	// single-account backend ever produces: InviteOrganizationToTransferResponsibility
	// can only be called from the sending (self) account, so every transfer
	// this backend creates has self as ResponsibilityTransfer.Source and the
	// invited party as Target -- see ResponsibilityTransfer.Source/Target's
	// doc comments and ListInboundResponsibilityTransfers' rationale for why
	// this backend never produces an "inbound" transfer.
	responsibilityTransferDirectionOutbound = "outbound"
)

// AcceptHandshake accepts an OPEN handshake.
// For INVITE handshakes, the invited account is added to the organization.
func (b *InMemoryBackend) AcceptHandshake(handshakeID string) (*Handshake, error) {
	b.mu.Lock("AcceptHandshake")
	defer b.mu.Unlock()

	h, ok := b.handshakes.Get(handshakeID)
	if !ok {
		return nil, ErrHandshakeNotFound
	}

	if h.State != handshakeStateOpen {
		return nil, ErrHandshakeConstraintViolation
	}

	h.State = handshakeStateAccepted
	b.syncResponsibilityTransferStatusLocked(h)

	if h.Action == handshakeActionInvite && b.org != nil {
		for _, r := range h.Resources {
			if r.Type == targetTypeAccount {
				acctID := r.Value
				if !b.accounts.Has(acctID) {
					now := time.Now()
					acct := &Account{
						ID:           acctID,
						ARN:          b.accountARN(b.org.ID, acctID),
						Name:         acctID,
						Email:        acctID + "@invited.example.com",
						Status:       accountStatusActive,
						JoinedMethod: joinedMethodInvited,
						JoinedAt:     now,
					}
					b.accounts.Put(acct)
					b.accountParent[acctID] = b.root.ID
					b.addAccountChild(b.root.ID, acctID)
					b.setTagsLocked(acctID, h.PendingTags)
				}

				break
			}
		}
	}

	return copyHandshake(h), nil
}

// CancelHandshake cancels an OPEN handshake.
func (b *InMemoryBackend) CancelHandshake(handshakeID string) (*Handshake, error) {
	b.mu.Lock("CancelHandshake")
	defer b.mu.Unlock()

	h, ok := b.handshakes.Get(handshakeID)
	if !ok {
		return nil, ErrHandshakeNotFound
	}

	if h.State != handshakeStateOpen {
		return nil, ErrHandshakeConstraintViolation
	}

	h.State = handshakeStateCanceled
	b.syncResponsibilityTransferStatusLocked(h)

	return copyHandshake(h), nil
}

// DeclineHandshake declines an OPEN handshake.
func (b *InMemoryBackend) DeclineHandshake(handshakeID string) (*Handshake, error) {
	b.mu.Lock("DeclineHandshake")
	defer b.mu.Unlock()

	h, ok := b.handshakes.Get(handshakeID)
	if !ok {
		return nil, ErrHandshakeNotFound
	}

	if h.State != handshakeStateOpen {
		return nil, ErrHandshakeConstraintViolation
	}

	h.State = handshakeStateDeclined
	b.syncResponsibilityTransferStatusLocked(h)

	return copyHandshake(h), nil
}

// DescribeHandshake returns a handshake by ID.
func (b *InMemoryBackend) DescribeHandshake(handshakeID string) (*Handshake, error) {
	b.mu.Lock("DescribeHandshake")
	defer b.mu.Unlock()

	b.expireStaleHandshakesLocked()

	h, ok := b.handshakes.Get(handshakeID)
	if !ok {
		return nil, ErrHandshakeNotFound
	}

	return copyHandshake(h), nil
}

// DescribeResponsibilityTransfer returns a responsibility transfer by its own
// Id (rt-..., not the Id of its ActiveHandshakeId) --
// api_op_DescribeResponsibilityTransfer.go's Input.Id and Output.ResponsibilityTransfer
// (awsAwsjson11_deserializeOpDocumentDescribeResponsibilityTransferOutput,
// deserializers.go).
func (b *InMemoryBackend) DescribeResponsibilityTransfer(transferID string) (*ResponsibilityTransfer, error) {
	b.mu.RLock("DescribeResponsibilityTransfer")
	defer b.mu.RUnlock()

	rt, ok := b.responsibilityTransfers.Get(transferID)
	if !ok {
		return nil, ErrResponsibilityTransferNotFound
	}

	return copyResponsibilityTransfer(rt), nil
}

// AddHandshakeInternal seeds a handshake directly for testing.
// If h.ID is empty, a new ID is generated. If h.ExpirationTimestamp is zero,
// it is set to now + handshakeExpirationDuration.
func (b *InMemoryBackend) AddHandshakeInternal(h *Handshake) {
	b.mu.Lock("AddHandshakeInternal")
	defer b.mu.Unlock()

	if h.ID == "" {
		h.ID = newHandshakeID()
	}

	if h.ExpirationTimestamp.IsZero() {
		h.ExpirationTimestamp = time.Now().Add(handshakeExpirationDuration)
	}

	if h.ARN == "" && b.org != nil {
		action := h.Action
		if action == "" {
			action = handshakeActionInvite
		}

		h.ARN = b.handshakeARN(b.org.ID, action, h.ID)
	}

	b.handshakes.Put(h)
}

// AddResponsibilityTransferInternal seeds a responsibility transfer directly for testing.
// If rt.ID is empty, a new ID is generated. If rt.ARN is empty and an org exists, it is
// derived assuming the outbound direction (see InviteOrganizationToTransferResponsibility;
// there is no inbound case for this backend to seed -- ListInboundResponsibilityTransfers).
func (b *InMemoryBackend) AddResponsibilityTransferInternal(rt *ResponsibilityTransfer) {
	b.mu.Lock("AddResponsibilityTransferInternal")
	defer b.mu.Unlock()

	if rt.ID == "" {
		rt.ID = newResponsibilityTransferID()
	}

	if rt.ARN == "" && b.org != nil {
		transferType := rt.Type
		if transferType == "" {
			transferType = responsibilityTransferTypeBilling
		}

		rt.ARN = b.responsibilityTransferARN(b.org.ID, transferType, responsibilityTransferDirectionOutbound, rt.ID)
	}

	b.responsibilityTransfers.Put(rt)
}

// expireStaleHandshakesLocked transitions OPEN handshakes past their ExpirationTimestamp to EXPIRED.
// Must be called with a write lock held.
func (b *InMemoryBackend) expireStaleHandshakesLocked() {
	now := time.Now()
	for _, h := range b.handshakes.All() {
		if h.State == handshakeStateOpen && !h.ExpirationTimestamp.IsZero() && now.After(h.ExpirationTimestamp) {
			h.State = handshakeStateExpired
			b.syncResponsibilityTransferStatusLocked(h)
		}
	}
}

// responsibilityTransferStatusForHandshakeState maps a Handshake's State to
// the corresponding ResponsibilityTransferStatus. The two enums share every
// literal (types/enums.go: HandshakeState, ResponsibilityTransferStatus)
// except HandshakeState's OPEN, which corresponds to
// ResponsibilityTransferStatus's REQUESTED.
func responsibilityTransferStatusForHandshakeState(state string) string {
	if state == handshakeStateOpen {
		return responsibilityTransferStatusRequested
	}

	return state
}

// syncResponsibilityTransferStatusLocked updates the ResponsibilityTransfer
// record (if any) whose ActiveHandshakeID is h.ID to mirror h's current
// State. Must be called with the write lock held, after h.State is set.
func (b *InMemoryBackend) syncResponsibilityTransferStatusLocked(h *Handshake) {
	if h.Action != handshakeActionTransferResponsibility {
		return
	}

	rts := b.responsibilityTransfersByHandshake.Get(h.ID)
	if len(rts) == 0 {
		return
	}

	rts[0].Status = responsibilityTransferStatusForHandshakeState(h.State)
}

// EnableAllFeatures creates an ENABLE_ALL_FEATURES handshake and returns it.
// Real AWS sends this handshake to all member accounts to approve the feature upgrade.
func (b *InMemoryBackend) EnableAllFeatures() (*Handshake, error) {
	b.mu.Lock("EnableAllFeatures")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	now := time.Now()
	id := newHandshakeID()
	h := &Handshake{
		ID:                  id,
		ARN:                 b.handshakeARN(b.org.ID, handshakeActionEnableFeatures, id),
		Action:              handshakeActionEnableFeatures,
		State:               handshakeStateOpen,
		RequestedTimestamp:  now,
		ExpirationTimestamp: now.Add(handshakeExpirationDuration),
		Parties: []HandshakeParty{
			{ID: b.org.MasterAccountID, Type: targetTypeAccount},
		},
		Resources: []HandshakeResource{
			{Type: handshakeResourceOrg, Value: b.org.ID},
			{Type: handshakeResourceMasterEmail, Value: b.org.MasterAccountEmail},
		},
	}

	b.handshakes.Put(h)

	return copyHandshake(h), nil
}

// InviteAccountToOrganization creates an OPEN invitation handshake targeting an account.
func validateHandshakeTarget(target HandshakeParty) error {
	if target.ID == "" {
		return ErrInvalidInput
	}
	switch target.Type {
	case targetTypeAccount:
		if len(target.ID) != accountIDLength {
			return ErrInvalidInput
		}
	case "EMAIL":
		if !strings.Contains(target.ID, "@") {
			return ErrInvalidInput
		}
	default:
		if target.Type != "" {
			return ErrInvalidInput
		}
	}

	return nil
}

func (b *InMemoryBackend) InviteAccountToOrganization(
	target HandshakeParty,
	notes string,
	tags []Tag,
) (*Handshake, error) {
	b.mu.Lock("InviteAccountToOrganization")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if err := validateHandshakeTarget(target); err != nil {
		return nil, err
	}

	// AWS: "If any one of the tags is not valid ... the entire request fails
	// and invitations are not sent", matching CreateAccount's Tags handling.
	if err := validateNewTags(nil, tags); err != nil {
		return nil, err
	}

	// AWS rejects duplicate open invitations to the same target.
	for _, existing := range b.handshakes.All() {
		if existing.State != handshakeStateOpen || existing.Action != handshakeActionInvite {
			continue
		}
		for _, r := range existing.Resources {
			if r.Type == targetTypeAccount && r.Value == target.ID {
				return nil, ErrDuplicateHandshake
			}
		}
	}

	now := time.Now()
	id := newHandshakeID()
	h := &Handshake{
		ID:                  id,
		ARN:                 b.handshakeARN(b.org.ID, handshakeActionInvite, id),
		Action:              handshakeActionInvite,
		State:               handshakeStateOpen,
		RequestedTimestamp:  now,
		ExpirationTimestamp: now.Add(handshakeExpirationDuration),
		Parties: []HandshakeParty{
			{ID: b.org.MasterAccountID, Type: targetTypeAccount},
			{ID: target.ID, Type: target.Type},
		},
		Resources: []HandshakeResource{
			{Type: targetTypeAccount, Value: target.ID},
			{Type: handshakeResourceOrg, Value: b.org.ID},
			{Type: handshakeResourceMasterEmail, Value: b.org.MasterAccountEmail},
		},
		PendingTags: tags,
	}

	if notes != "" {
		h.Resources = append(h.Resources, HandshakeResource{Type: handshakeResourceNotes, Value: notes})
	}

	b.handshakes.Put(h)

	return copyHandshake(h), nil
}

// LeaveOrganization is callable only from a member account (SDK doc comment
// on the operation), but this backend's caller identity is always the
// management account (organization.go's CreateOrganization: b.accountID
// becomes org.MasterAccountID), so it always fails.
func (b *InMemoryBackend) LeaveOrganization() error {
	b.mu.RLock("LeaveOrganization")
	defer b.mu.RUnlock()

	if b.org == nil {
		return ErrOrgNotFound
	}

	return ErrMasterCannotLeaveOrganization
}

// ListHandshakesForAccount returns all handshakes visible to the calling account.
// actionTypeFilter optionally restricts results to handshakes with the given Action value.
func (b *InMemoryBackend) ListHandshakesForAccount(actionTypeFilter string) ([]*Handshake, error) {
	b.mu.Lock("ListHandshakesForAccount")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	b.expireStaleHandshakesLocked()

	out := make([]*Handshake, 0, b.handshakes.Len())

	for _, h := range b.handshakes.All() {
		if actionTypeFilter == "" || h.Action == actionTypeFilter {
			out = append(out, copyHandshake(h))
		}
	}

	slices.SortFunc(out, func(a, b *Handshake) int { return cmp.Compare(a.ID, b.ID) })

	return out, nil
}

// ListHandshakesForOrganization returns all handshakes for the organization.
// actionTypeFilter optionally restricts results to handshakes with the given Action value.
func (b *InMemoryBackend) ListHandshakesForOrganization(actionTypeFilter string) ([]*Handshake, error) {
	b.mu.Lock("ListHandshakesForOrganization")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	b.expireStaleHandshakesLocked()

	out := make([]*Handshake, 0, b.handshakes.Len())

	for _, h := range b.handshakes.All() {
		if actionTypeFilter == "" || h.Action == actionTypeFilter {
			out = append(out, copyHandshake(h))
		}
	}

	slices.SortFunc(out, func(a, b *Handshake) int { return cmp.Compare(a.ID, b.ID) })

	return out, nil
}

// ListInboundResponsibilityTransfers returns responsibility transfers where this account manages
// responsibilities for another organization -- transfers sent TO this account by another
// organization's management account. InviteOrganizationToTransferResponsibility can only be called
// from the sending account (api_op_InviteOrganizationToTransferResponsibility.go doc comment), and this
// single-account backend has no way to simulate a transfer initiated by a foreign account, so every
// TRANSFER_RESPONSIBILITY handshake this backend ever creates is outbound -- see
// ListOutboundResponsibilityTransfers. Returning empty here is honest given that structural limit, not a
// stub: there is no fabricated data to return. transferType/id are accepted (matching
// ListInboundResponsibilityTransfersInput's Type/Id members) but unused, since the result is always empty.
func (b *InMemoryBackend) ListInboundResponsibilityTransfers(_, _ string) ([]*ResponsibilityTransfer, error) {
	b.mu.RLock("ListInboundResponsibilityTransfers")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	return nil, nil
}

// ListOutboundResponsibilityTransfers returns responsibility transfers initiated by this org, optionally
// filtered by transferType (ListOutboundResponsibilityTransfersInput.Type).
func (b *InMemoryBackend) ListOutboundResponsibilityTransfers(transferType string) ([]*ResponsibilityTransfer, error) {
	b.mu.RLock("ListOutboundResponsibilityTransfers")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	var out []*ResponsibilityTransfer

	for _, rt := range b.responsibilityTransfers.All() {
		if transferType == "" || rt.Type == transferType {
			out = append(out, copyResponsibilityTransfer(rt))
		}
	}

	slices.SortFunc(out, func(a, b *ResponsibilityTransfer) int { return cmp.Compare(a.ID, b.ID) })

	return out, nil
}

// TerminateResponsibilityTransfer ends an ACCEPTED responsibility transfer, setting EndTimestamp
// (defaulting to now when the caller doesn't supply one -- TerminateResponsibilityTransferInput.EndTimestamp
// is optional). ErrInvalidResponsibilityTransferTransition/ErrResponsibilityTransferAlreadyInStatus are
// both declared on this op's own deserializeOpErrorTerminateResponsibilityTransfer switch
// (deserializers.go), distinguishing "never accepted" from "already terminated".
func (b *InMemoryBackend) TerminateResponsibilityTransfer(
	transferID string, endTimestamp *time.Time,
) (*ResponsibilityTransfer, error) {
	b.mu.Lock("TerminateResponsibilityTransfer")
	defer b.mu.Unlock()

	rt, ok := b.responsibilityTransfers.Get(transferID)
	if !ok {
		return nil, ErrResponsibilityTransferNotFound
	}

	if rt.Status != responsibilityTransferStatusAccepted {
		return nil, ErrInvalidResponsibilityTransferTransition
	}

	if !rt.EndTimestamp.IsZero() {
		return nil, ErrResponsibilityTransferAlreadyInStatus
	}

	if endTimestamp != nil {
		rt.EndTimestamp = *endTimestamp
	} else {
		rt.EndTimestamp = time.Now()
	}

	return copyResponsibilityTransfer(rt), nil
}

// UpdateResponsibilityTransfer renames a transfer -- the only field
// UpdateResponsibilityTransferInput carries besides Id (api_op_UpdateResponsibilityTransfer.go's doc
// comment: "You can update the name assigned to a transfer."). Unlike TerminateResponsibilityTransfer,
// its error switch declares no transition-related exception, so renaming is allowed at any Status.
func (b *InMemoryBackend) UpdateResponsibilityTransfer(transferID, name string) (*ResponsibilityTransfer, error) {
	b.mu.Lock("UpdateResponsibilityTransfer")
	defer b.mu.Unlock()

	rt, ok := b.responsibilityTransfers.Get(transferID)
	if !ok {
		return nil, ErrResponsibilityTransferNotFound
	}

	rt.Name = name

	return copyResponsibilityTransfer(rt), nil
}

// InviteOrganizationToTransferResponsibility creates an OPEN invitation for org-to-org responsibility transfer.
// SourceName/StartTimestamp/Type are required InviteOrganizationToTransferResponsibilityInput members with no
// first-class Handshake field to carry them, so they are embedded as HandshakeResource entries the same way
// InviteAccountToOrganization/EnableAllFeatures embed their own extra fields (NOTES, MASTER_EMAIL) --
// RESPONSIBILITY_TRANSFER/TRANSFER_START_TIMESTAMP/TRANSFER_TYPE are the matching HandshakeResourceType enum
// values (types/enums.go).
func (b *InMemoryBackend) InviteOrganizationToTransferResponsibility(
	target HandshakeParty,
	params TransferResponsibilityParams,
) (*Handshake, error) {
	b.mu.Lock("InviteOrganizationToTransferResponsibility")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if target.ID == "" || params.SourceName == "" || params.Type == "" || params.StartTimestamp.IsZero() {
		return nil, ErrInvalidInput
	}

	now := time.Now()
	id := newHandshakeID()
	h := &Handshake{
		ID:                  id,
		ARN:                 b.handshakeARN(b.org.ID, handshakeActionTransferResponsibility, id),
		Action:              handshakeActionTransferResponsibility,
		State:               handshakeStateOpen,
		RequestedTimestamp:  now,
		ExpirationTimestamp: now.Add(handshakeExpirationDuration),
		Parties: []HandshakeParty{
			{ID: b.org.MasterAccountID, Type: targetTypeAccount},
			{ID: target.ID, Type: target.Type},
		},
		Resources: []HandshakeResource{
			{Type: handshakeResourceOrg, Value: b.org.ID},
			{Type: handshakeResourceResponsibilityTransfer, Value: params.SourceName},
			{
				Type:  handshakeResourceTransferStartTimestamp,
				Value: strconv.FormatFloat(epochSeconds(params.StartTimestamp), 'f', -1, 64),
			},
			{Type: handshakeResourceTransferType, Value: params.Type},
		},
	}

	if params.Notes != "" {
		h.Resources = append(h.Resources, HandshakeResource{Type: handshakeResourceNotes, Value: params.Notes})
	}

	b.handshakes.Put(h)

	transferID := newResponsibilityTransferID()
	rt := &ResponsibilityTransfer{
		StartTimestamp: params.StartTimestamp,
		ID:             transferID,
		ARN: b.responsibilityTransferARN(
			b.org.ID,
			params.Type,
			responsibilityTransferDirectionOutbound,
			transferID,
		),
		ActiveHandshakeID: id,
		Name:              params.SourceName,
		Status:            responsibilityTransferStatusRequested,
		Type:              params.Type,
		Source: TransferParticipant{
			ManagementAccountID:    b.org.MasterAccountID,
			ManagementAccountEmail: b.org.MasterAccountEmail,
		},
		Target: transferParticipantFromHandshakeParty(target),
	}
	b.responsibilityTransfers.Put(rt)

	return copyHandshake(h), nil
}

// transferParticipantFromHandshakeParty converts an invite's HandshakeParty target into a
// TransferParticipant, populating only the field the party's Type actually tells us -- an
// EMAIL-type party's account ID isn't known until they join, and an ACCOUNT-type party's email
// isn't known at all in this backend (see TransferParticipant's doc comment).
func transferParticipantFromHandshakeParty(p HandshakeParty) TransferParticipant {
	switch p.Type {
	case targetTypeAccount:
		return TransferParticipant{ManagementAccountID: p.ID}
	case "EMAIL":
		return TransferParticipant{ManagementAccountEmail: p.ID}
	default:
		return TransferParticipant{}
	}
}

// copyResponsibilityTransfer returns a copy of a ResponsibilityTransfer. Source/Target are
// plain value structs (no nested slices/pointers), so a shallow struct copy is a full copy.
func copyResponsibilityTransfer(rt *ResponsibilityTransfer) *ResponsibilityTransfer {
	cp := *rt

	return &cp
}

// copyHandshake returns a deep copy of a Handshake.
func copyHandshake(h *Handshake) *Handshake {
	cp := *h

	if h.Parties != nil {
		cp.Parties = make([]HandshakeParty, len(h.Parties))
		copy(cp.Parties, h.Parties)
	}

	cp.Resources = copyHandshakeResources(h.Resources)

	return &cp
}

// copyHandshakeResources returns a deep copy of a HandshakeResource slice.
func copyHandshakeResources(rs []HandshakeResource) []HandshakeResource {
	if rs == nil {
		return nil
	}

	out := make([]HandshakeResource, len(rs))

	for i, r := range rs {
		out[i] = HandshakeResource{
			Type:      r.Type,
			Value:     r.Value,
			Resources: copyHandshakeResources(r.Resources),
		}
	}

	return out
}
