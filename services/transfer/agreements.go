package transfer

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"
)

// CreateAgreement creates an AS2 agreement on an existing server.
func (b *InMemoryBackend) CreateAgreement(
	serverID, description, localProfileID, partnerProfileID, baseDirectory, accessRole string,
	tags map[string]string,
) (*Agreement, error) {
	return b.CreateAgreementFull(
		serverID,
		description,
		localProfileID,
		partnerProfileID,
		baseDirectory,
		accessRole,
		agreementStatusActive,
		tags,
	)
}

// CreateAgreementFull creates an AS2 agreement with an explicit initial status.
func (b *InMemoryBackend) CreateAgreementFull(
	serverID, description, localProfileID, partnerProfileID, baseDirectory, accessRole, status string,
	tags map[string]string,
) (*Agreement, error) {
	b.mu.Lock("CreateAgreement")
	defer b.mu.Unlock()

	if !b.servers.Has(serverID) {
		return nil, fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	// Validate status.
	if status == "" {
		status = agreementStatusActive
	}

	switch status {
	case agreementStatusActive, agreementStatusInactive:
		// valid
	default:
		return nil, fmt.Errorf(
			"%w: Status must be ACTIVE or INACTIVE, got %q",
			ErrValidation,
			status,
		)
	}

	agreementID := "a-" + uuid.NewString()[:20]

	merged := make(map[string]string, len(tags))
	maps.Copy(merged, tags)

	ag := &Agreement{
		AgreementID:      agreementID,
		ServerID:         serverID,
		Description:      description,
		LocalProfileID:   localProfileID,
		PartnerProfileID: partnerProfileID,
		BaseDirectory:    baseDirectory,
		AccessRole:       accessRole,
		Status:           status,
		CreatedAt:        time.Now(),
		Tags:             merged,
		AccountID:        b.accountID,
		Region:           b.region,
	}
	b.agreements.Put(ag)
	b.initTagsStore(agreementARN(b.accountID, b.region, serverID, agreementID), merged)

	return cloneAgreement(ag), nil
}

// DeleteAgreement removes an AS2 agreement from a server.
func (b *InMemoryBackend) DeleteAgreement(serverID, agreementID string) error {
	b.mu.Lock("DeleteAgreement")
	defer b.mu.Unlock()

	if !b.servers.Has(serverID) {
		return fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	key := agreementKey(serverID, agreementID)
	if !b.agreements.Has(key) {
		return fmt.Errorf(
			"%w: agreement %s not found on server %s",
			ErrAgreementNotFound,
			agreementID,
			serverID,
		)
	}

	b.agreements.Delete(key)
	delete(b.tagsStore, agreementARN(b.accountID, b.region, serverID, agreementID))

	return nil
}

// DescribeAgreement returns an agreement from a server.
func (b *InMemoryBackend) DescribeAgreement(serverID, agreementID string) (*Agreement, error) {
	b.mu.RLock("DescribeAgreement")
	defer b.mu.RUnlock()

	ag, ok := b.agreements.Get(agreementKey(serverID, agreementID))
	if !ok {
		return nil, fmt.Errorf(
			"%w: agreement %s not found on server %s",
			ErrAgreementNotFound,
			agreementID,
			serverID,
		)
	}

	return cloneAgreement(ag), nil
}

// ListAgreements returns all agreements on a server sorted by agreementID.
func (b *InMemoryBackend) ListAgreements(serverID string) ([]*Agreement, error) {
	b.mu.RLock("ListAgreements")
	defer b.mu.RUnlock()

	if !b.servers.Has(serverID) {
		return nil, fmt.Errorf("%w: server %s not found", ErrServerNotFound, serverID)
	}

	agreements := b.agreementsByServer.Get(serverID)
	out := make([]*Agreement, 0, len(agreements))

	for _, ag := range agreements {
		out = append(out, cloneAgreement(ag))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].AgreementID < out[j].AgreementID
	})

	return out, nil
}

// UpdateAgreement updates mutable fields on an agreement.
func (b *InMemoryBackend) UpdateAgreement(
	serverID, agreementID, description, status string,
) (*Agreement, error) {
	b.mu.Lock("UpdateAgreement")
	defer b.mu.Unlock()

	ag, ok := b.agreements.Get(agreementKey(serverID, agreementID))
	if !ok {
		return nil, fmt.Errorf(
			"%w: agreement %s not found on server %s",
			ErrAgreementNotFound,
			agreementID,
			serverID,
		)
	}

	if description != "" {
		ag.Description = description
	}

	if status != "" {
		switch status {
		case agreementStatusActive, agreementStatusInactive:
			// valid
		default:
			return nil, fmt.Errorf(
				"%w: Status must be ACTIVE or INACTIVE, got %q",
				ErrValidation,
				status,
			)
		}

		ag.Status = status
	}

	return cloneAgreement(ag), nil
}
