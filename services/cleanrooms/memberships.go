package cleanrooms

import (
	"maps"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) membershipARN(id string) string {
	return arn.Build("cleanrooms", b.region, b.accountID, "membership/"+id)
}

// defaultPaymentConfig returns explicit if the caller supplied one, otherwise
// computes the real AWS default documented on
// QueryComputePaymentConfig.IsResponsible: "If the collaboration creator
// hasn't specified anyone as the member paying for query compute costs, then
// the member who can query is the default payer." paymentConfiguration is a
// required field on both Membership and MemberSummary, so this ensures it is
// never emitted empty.
func defaultPaymentConfig(abilities []string, explicit map[string]any) map[string]any {
	if explicit != nil {
		return explicit
	}

	return map[string]any{
		"queryCompute": map[string]any{"isResponsible": contains(abilities, "CAN_QUERY")},
	}
}

// createMembershipLocked creates a membership under collab. Callers must
// hold b.mu (write lock) and have already validated collab is non-nil. Used
// both by the public CreateMembership entry point and by
// CreateCollaboration, which -- matching real AWS behavior, where the
// Collaboration response carries membershipArn/membershipId for the caller's
// own membership -- automatically creates a membership for the
// collaboration creator.
func (b *InMemoryBackend) createMembershipLocked(
	collab *Collaboration,
	queryLogStatus string,
	memberAbilities []string,
	defaultResultConfiguration map[string]any,
	paymentConfiguration map[string]any,
	tags map[string]string,
) *Membership {
	id := uuid.NewString()
	ts := b.now()
	if memberAbilities == nil {
		// MemberAbilities is required on the wire (Membership/MembershipSummary);
		// a nil Go slice marshals as JSON null, which a real client's deserializer
		// treats identically to the key being absent -- must be non-nil so it
		// marshals as [] (gopherstack-r80d).
		memberAbilities = []string{}
	}
	m := &Membership{
		MembershipIdentifier:            id,
		Arn:                             b.membershipARN(id),
		CollaborationIdentifier:         collab.ID,
		CollaborationArn:                collab.Arn,
		CollaborationCreatorAccountID:   collab.CreatorAccountID,
		CollaborationCreatorDisplayName: collab.CreatorDisplayName,
		CollaborationName:               collab.Name,
		Status:                          statusActive,
		QueryLogStatus:                  queryLogStatus,
		MemberAbilities:                 memberAbilities,
		DefaultResultConfiguration:      defaultResultConfiguration,
		PaymentConfiguration:            defaultPaymentConfig(memberAbilities, paymentConfiguration),
		CreateTime:                      ts,
		UpdateTime:                      ts,
		ID:                              id,
		CollaborationID:                 collab.ID,
	}
	b.memberships.Put(m)
	if len(tags) > 0 {
		b.tagsByArn[m.Arn] = maps.Clone(tags)
	}

	return m
}

func (b *InMemoryBackend) CreateMembership(
	collaborationID, queryLogStatus string,
	memberAbilities []string,
	defaultResultConfiguration map[string]any,
	paymentConfiguration map[string]any,
	tags map[string]string,
) (*Membership, error) {
	b.mu.Lock("CreateMembership")
	defer b.mu.Unlock()
	if collaborationID == "" {
		return nil, ErrValidation
	}
	collab, ok := b.collaborations.Get(collaborationID)
	if !ok {
		return nil, ErrNotFound
	}

	return b.createMembershipLocked(
		collab, queryLogStatus, memberAbilities, defaultResultConfiguration, paymentConfiguration, tags,
	), nil
}

func (b *InMemoryBackend) GetMembership(id string) (*Membership, error) {
	b.mu.RLock("GetMembership")
	defer b.mu.RUnlock()
	m, ok := b.memberships.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return m, nil
}

func (b *InMemoryBackend) ListMemberships(
	status, maxResults, nextToken string,
) ([]*MembershipSummary, string) {
	b.mu.RLock("ListMemberships")
	defer b.mu.RUnlock()
	var items []*MembershipSummary
	for _, m := range b.memberships.All() {
		if status != "" && m.Status != status {
			continue
		}
		items = append(items, &MembershipSummary{
			MembershipIdentifier:            m.MembershipIdentifier,
			Arn:                             m.Arn,
			CollaborationIdentifier:         m.CollaborationIdentifier,
			CollaborationArn:                m.CollaborationArn,
			CollaborationCreatorAccountID:   m.CollaborationCreatorAccountID,
			CollaborationCreatorDisplayName: m.CollaborationCreatorDisplayName,
			CollaborationName:               m.CollaborationName,
			Status:                          m.Status,
			MemberAbilities:                 m.MemberAbilities,
			PaymentConfiguration:            m.PaymentConfiguration,
			CreateTime:                      m.CreateTime,
			UpdateTime:                      m.UpdateTime,
			ID:                              m.ID,
			CollaborationID:                 m.CollaborationID,
		})
	}
	sort.Slice(
		items,
		func(i, j int) bool { return items[i].ID < items[j].ID },
	)
	page, next := paginate(items, maxResults, nextToken)

	return page, next
}

func (b *InMemoryBackend) UpdateMembership(
	id, queryLogStatus string,
	defaultResultConfiguration map[string]any,
) (*Membership, error) {
	b.mu.Lock("UpdateMembership")
	defer b.mu.Unlock()
	m, ok := b.memberships.Get(id)
	if !ok {
		return nil, ErrNotFound
	}
	if queryLogStatus != "" {
		m.QueryLogStatus = queryLogStatus
	}
	if defaultResultConfiguration != nil {
		m.DefaultResultConfiguration = defaultResultConfiguration
	}
	m.UpdateTime = b.now()

	return m, nil
}

// DeleteMembership deletes the membership identified by id.
//
// api_op_DeleteMembership.go: "Deletes a specified membership. All resources
// under a membership must be deleted." -- unlike DeleteCollaboration (no such
// statement), a membership with any deletable child resource still attached
// must be rejected rather than silently orphaning it.
func (b *InMemoryBackend) DeleteMembership(id string) error {
	b.mu.Lock("DeleteMembership")
	defer b.mu.Unlock()
	m, ok := b.memberships.Get(id)
	if !ok {
		return ErrNotFound
	}
	if b.membershipHasResources(id) {
		return ErrConflict
	}
	delete(b.tagsByArn, m.Arn)
	b.memberships.Delete(id)

	return nil
}

func (b *InMemoryBackend) membershipHasResources(membershipID string) bool {
	return len(b.ctAssociationsByMembership.Get(membershipID)) > 0 ||
		len(b.analysisTemplatesByMembership.Get(membershipID)) > 0 ||
		len(b.privacyBudgetTemplatesByMembership.Get(membershipID)) > 0 ||
		len(b.idMappingTablesByMembership.Get(membershipID)) > 0 ||
		len(b.idNamespaceAssociationsByMembership.Get(membershipID)) > 0 ||
		len(b.camaAssociationsByMembership.Get(membershipID)) > 0 ||
		len(b.intermediateTablesByMembership.Get(membershipID)) > 0
}
