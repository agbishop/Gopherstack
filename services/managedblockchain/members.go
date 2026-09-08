package managedblockchain

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// memberStatusAvailable is the status for a ready member.
const memberStatusAvailable = "AVAILABLE"

// awsOwnedKMSKey is the literal string real AWS returns for KmsKeyArn when
// the caller doesn't supply a customer managed key, meaning the member (or
// node, which inherits its owning member's key) is encrypted with an
// Amazon Web Services owned KMS key.
const awsOwnedKMSKey = "AWS Owned KMS Key"

// memberARN builds the ARN for a Managed Blockchain member.
func memberARN(region, accountID, memberID string) string {
	return arn.Build("managedblockchain", region, accountID, "members/"+memberID)
}

// resolveMemberKmsKeyArn returns kmsKeyArn verbatim if the caller supplied
// one, or the real API's documented default sentinel otherwise.
func resolveMemberKmsKeyArn(kmsKeyArn string) string {
	if kmsKeyArn == "" {
		return awsOwnedKMSKey
	}

	return kmsKeyArn
}

// memberCaEndpoint synthesizes the endpoint exposed on
// Member.FrameworkAttributes.Fabric.CaEndpoint. gopherstack has no real
// Fabric CA to connect to; this deterministically derives a plausible
// endpoint from the member's own identity, matching real AWS's
// "ca.<member-id>...:30002" shape.
func memberCaEndpoint(memberID, networkID, region string) string {
	return fmt.Sprintf("ca.%s.%s.managedblockchain.%s.amazonaws.com:30002", memberID, networkID, region)
}

// buildMemberFrameworkAttributes builds Member.FrameworkAttributes from a
// CreateMember/CreateNetwork caller's requested Fabric AdminUsername. If
// adminUsername is empty, no Fabric attributes are synthesized.
func buildMemberFrameworkAttributes(memberID, networkID, region, adminUsername string) *MemberFrameworkAttributesState {
	if adminUsername == "" {
		return nil
	}

	return &MemberFrameworkAttributesState{
		Fabric: &MemberFabricAttributesState{
			AdminUsername: adminUsername,
			CaEndpoint:    memberCaEndpoint(memberID, networkID, region),
		},
	}
}

// cloneMember returns a deep copy of m with the Tags map cloned.
func cloneMember(m *Member) *Member {
	cp := *m
	cp.Tags = maps.Clone(m.Tags)
	cp.FrameworkAttributes = cloneMemberFrameworkAttributes(m.FrameworkAttributes)

	return &cp
}

// cloneMemberFrameworkAttributes returns a deep copy of a MemberFrameworkAttributesState.
func cloneMemberFrameworkAttributes(fa *MemberFrameworkAttributesState) *MemberFrameworkAttributesState {
	if fa == nil {
		return nil
	}

	cp := &MemberFrameworkAttributesState{}

	if fa.Fabric != nil {
		fabric := *fa.Fabric
		cp.Fabric = &fabric
	}

	return cp
}

// CreateMember creates a new member in an existing network. invitationID must name a
// PENDING invitation issued for networkID -- real AWS requires InvitationId on every
// CreateMember call (aws-sdk-go-v2 managedblockchain validators.go:805-806, v1.34.4) and
// documents ACCEPTED as the status an invitation moves to once used (types/enums.go:111).
// On success the invitation is consumed (marked ACCEPTED) so it cannot be reused.
func (b *InMemoryBackend) CreateMember(
	region, accountID, networkID, invitationID, name, description, adminUsername, kmsKeyArn string,
	tags map[string]string,
) (*Member, error) {
	b.mu.Lock("CreateMember")
	defer b.mu.Unlock()

	if err := checkTagLimit(nil, tags); err != nil {
		return nil, err
	}

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	inv, exists := b.invitations.Get(invitationID)
	if !exists {
		return nil, ErrInvitationNotFound
	}

	if inv.NetworkID != networkID {
		return nil, ErrInvitationNetworkMismatch
	}

	if inv.Status != invitationStatusPending {
		return nil, ErrInvitationNotPending
	}

	now := time.Now().UTC()
	memberID := uuid.NewString()

	t := make(map[string]string)
	maps.Copy(t, tags)

	member := &Member{
		ID:                  memberID,
		Arn:                 memberARN(region, accountID, memberID),
		Name:                name,
		Description:         description,
		NetworkID:           networkID,
		Status:              memberStatusAvailable,
		CreationDate:        &now,
		Tags:                t,
		IsOwned:             true,
		KmsKeyArn:           resolveMemberKmsKeyArn(kmsKeyArn),
		FrameworkAttributes: buildMemberFrameworkAttributes(memberID, networkID, region, adminUsername),
	}

	b.members.Put(member)
	b.arnToResource[member.Arn] = member

	inv.Status = invitationStatusAccepted

	return cloneMember(member), nil
}

// GetMember returns a member by network ID and member ID.
func (b *InMemoryBackend) GetMember(networkID, memberID string) (*Member, error) {
	b.mu.RLock("GetMember")
	defer b.mu.RUnlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	member, exists := b.members.Get(memberKey(networkID, memberID))
	if !exists {
		return nil, ErrMemberNotFound
	}

	return cloneMember(member), nil
}

// ListMembers returns all members in a network, optionally filtered.
func (b *InMemoryBackend) ListMembers(networkID string, filter ListMembersFilter) ([]*Member, error) {
	b.mu.RLock("ListMembers")
	defer b.mu.RUnlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	members := b.membersByNetwork.Get(networkID)
	all := make([]*Member, 0, len(members))

	for _, m := range members {
		if filter.Name != "" && m.Name != filter.Name {
			continue
		}

		if filter.Status != "" && m.Status != filter.Status {
			continue
		}

		if filter.IsOwned != nil && m.IsOwned != *filter.IsOwned {
			continue
		}

		all = append(all, cloneMember(m))
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Name < all[j].Name
	})

	return all, nil
}

// DeleteMember removes a member from a network, cascading the delete to all of its
// nodes. If memberID is the network's last member, the network is deleted too:
// "If MemberId is the last member in a network specified by the last Amazon Web
// Services account, the network is deleted also." (aws-sdk-go-v2 managedblockchain
// api_op_DeleteMember.go doc comment, v1.34.4). gopherstack has no multi-account
// model, so every network's only account is trivially "the last" one.
func (b *InMemoryBackend) DeleteMember(networkID, memberID string) error {
	b.mu.Lock("DeleteMember")
	defer b.mu.Unlock()

	network, exists := b.networks.Get(networkID)
	if !exists {
		return ErrNetworkNotFound
	}

	m, exists := b.members.Get(memberKey(networkID, memberID))
	if !exists {
		return ErrMemberNotFound
	}

	delete(b.arnToResource, m.Arn)
	b.members.Delete(memberKey(networkID, memberID))

	b.deleteNodesForMemberLocked(networkID, memberID)

	deleteNetworkIfEmptyLocked(b, network)

	return nil
}

// AddMemberInternal adds a member directly to the backend (for testing and seeding).
// The network must already exist.
func (b *InMemoryBackend) AddMemberInternal(region, accountID, networkID, name string) *Member {
	b.mu.Lock("AddMemberInternal")
	defer b.mu.Unlock()

	now := time.Now().UTC()
	memberID := uuid.NewString()

	member := &Member{
		ID:           memberID,
		Arn:          memberARN(region, accountID, memberID),
		Name:         name,
		NetworkID:    networkID,
		Status:       memberStatusAvailable,
		CreationDate: &now,
		Tags:         make(map[string]string),
		IsOwned:      true,
		KmsKeyArn:    awsOwnedKMSKey,
	}

	b.members.Put(member)
	b.arnToResource[member.Arn] = member

	return cloneMember(member)
}

// UpdateMember updates a member's log publishing configuration.
func (b *InMemoryBackend) UpdateMember(
	networkID, memberID string,
	logConfig *MemberLogPublishingConfigState,
) (*Member, error) {
	b.mu.Lock("UpdateMember")
	defer b.mu.Unlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	m, exists := b.members.Get(memberKey(networkID, memberID))
	if !exists {
		return nil, ErrMemberNotFound
	}

	if logConfig != nil {
		m.LogPublishingConfiguration = cloneMemberLogConfig(logConfig)
	}

	return cloneMember(m), nil
}

// cloneMemberLogConfig returns a deep copy of MemberLogPublishingConfigState.
func cloneMemberLogConfig(c *MemberLogPublishingConfigState) *MemberLogPublishingConfigState {
	if c == nil {
		return nil
	}

	cp := &MemberLogPublishingConfigState{}

	if c.Fabric != nil {
		fabric := &MemberFabricLogState{}

		if c.Fabric.CALogs != nil {
			caLogs := cloneLogConfig(c.Fabric.CALogs)
			fabric.CALogs = caLogs
		}

		cp.Fabric = fabric
	}

	return cp
}

// cloneLogConfig returns a deep copy of LogConfigState. Shared by both
// cloneMemberLogConfig above and cloneNodeLogConfig in nodes.go.
func cloneLogConfig(c *LogConfigState) *LogConfigState {
	if c == nil {
		return nil
	}

	cp := &LogConfigState{}

	if c.CloudWatch != nil {
		cw := *c.CloudWatch
		cp.CloudWatch = &cw
	}

	return cp
}
