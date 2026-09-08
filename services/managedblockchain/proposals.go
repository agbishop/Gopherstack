package managedblockchain

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	// proposalStatusInProgress is the status for an in-progress proposal.
	proposalStatusInProgress = "IN_PROGRESS"
	// proposalStatusApproved is the status for an approved proposal.
	proposalStatusApproved = "APPROVED"
	// proposalStatusRejected is the status for a rejected proposal.
	proposalStatusRejected = "REJECTED"
	// proposalStatusExpired is the status for a proposal whose voting period
	// ended without enough votes to approve or reject it.
	proposalStatusExpired = "EXPIRED"
	// proposalStatusActionFailed is the status for an approved proposal where at
	// least one ProposalAction couldn't be carried out.
	proposalStatusActionFailed = "ACTION_FAILED"
	// proposalExpirationHours is the number of hours before a proposal expires.
	proposalExpirationHours = 24
	// percentBase is the base used to convert a vote fraction to a percentage.
	percentBase = 100.0
)

// proposalARN builds the ARN for a Managed Blockchain proposal.
func proposalARN(region, accountID, networkID, proposalID string) string {
	return arn.Build("managedblockchain", region, accountID,
		fmt.Sprintf("networks/%s/proposals/%s", networkID, proposalID))
}

// cloneProposalActions returns a deep copy of ProposalActions.
func cloneProposalActions(a *ProposalActions) *ProposalActions {
	if a == nil {
		return nil
	}

	cp := &ProposalActions{}

	if len(a.Invitations) > 0 {
		cp.Invitations = make([]InviteAction, len(a.Invitations))
		copy(cp.Invitations, a.Invitations)
	}

	if len(a.Removals) > 0 {
		cp.Removals = make([]RemoveAction, len(a.Removals))
		copy(cp.Removals, a.Removals)
	}

	return cp
}

// cloneProposal returns a deep copy of p with the Tags map cloned.
func cloneProposal(p *Proposal) *Proposal {
	cp := *p
	cp.Tags = maps.Clone(p.Tags)
	cp.Actions = cloneProposalActions(p.Actions)

	return &cp
}

// CreateProposal creates a new governance proposal on a network.
func (b *InMemoryBackend) CreateProposal(
	region, accountID, networkID, memberID, description string,
	actions *ProposalActions,
	tags map[string]string,
) (*Proposal, error) {
	b.mu.Lock("CreateProposal")
	defer b.mu.Unlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	member, exists := b.members.Get(memberKey(networkID, memberID))
	if !exists {
		return nil, ErrMemberNotFound
	}

	now := time.Now().UTC()
	expiry := now.Add(proposalExpirationHours * time.Hour)
	proposalID := uuid.NewString()

	t := make(map[string]string)
	maps.Copy(t, tags)

	memberCount := len(b.membersByNetwork.Get(networkID))
	outstandingVotes := memberCount

	proposal := &Proposal{
		ProposalID:           proposalID,
		Arn:                  proposalARN(region, accountID, networkID, proposalID),
		NetworkID:            networkID,
		ProposedByMemberID:   memberID,
		ProposedByMemberName: member.Name,
		Description:          description,
		Status:               proposalStatusInProgress,
		CreationDate:         &now,
		ExpirationDate:       &expiry,
		Tags:                 t,
		Actions:              cloneProposalActions(actions),
		OutstandingVoteCount: outstandingVotes,
	}

	b.proposals.Put(proposal)
	b.arnToResource[proposal.Arn] = proposal

	return cloneProposal(proposal), nil
}

// GetProposal returns a proposal by network ID and proposal ID.
func (b *InMemoryBackend) GetProposal(networkID, proposalID string) (*Proposal, error) {
	b.mu.Lock("GetProposal")
	defer b.mu.Unlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	proposal, exists := b.proposals.Get(proposalKey(networkID, proposalID))
	if !exists {
		return nil, ErrProposalNotFound
	}

	expireProposalLocked(proposal)

	return cloneProposal(proposal), nil
}

// ListProposals returns all proposals for a network sorted by proposal ID.
// statusFilter, when non-empty, limits results to proposals with that status.
func (b *InMemoryBackend) ListProposals(networkID, statusFilter string) ([]*Proposal, error) {
	b.mu.Lock("ListProposals")
	defer b.mu.Unlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	proposals := b.proposalsByNetwork.Get(networkID)
	all := make([]*Proposal, 0, len(proposals))

	for _, p := range proposals {
		expireProposalLocked(p)

		if statusFilter != "" && p.Status != statusFilter {
			continue
		}

		all = append(all, cloneProposal(p))
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].ProposalID < all[j].ProposalID
	})

	return all, nil
}

// ListProposalVotes returns all votes for a proposal.
func (b *InMemoryBackend) ListProposalVotes(networkID, proposalID string) ([]*ProposalVote, error) {
	b.mu.RLock("ListProposalVotes")
	defer b.mu.RUnlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return nil, ErrNetworkNotFound
	}

	if _, exists := b.proposals.Get(proposalKey(networkID, proposalID)); !exists {
		return nil, ErrProposalNotFound
	}

	votes := b.proposalVotesByProposal.Get(proposalID)
	result := make([]*ProposalVote, len(votes))

	for i, v := range votes {
		cp := *v
		result[i] = &cp
	}

	return result, nil
}

// AddProposalInternal adds a proposal directly to the backend (for testing and seeding).
// The network and member must already exist.
func (b *InMemoryBackend) AddProposalInternal(region, accountID, networkID, memberID, description string) *Proposal {
	b.mu.Lock("AddProposalInternal")
	defer b.mu.Unlock()

	now := time.Now().UTC()
	expiry := now.Add(proposalExpirationHours * time.Hour)
	proposalID := uuid.NewString()

	var memberName string

	memberCount := len(b.membersByNetwork.Get(networkID))

	if m, exists := b.members.Get(memberKey(networkID, memberID)); exists {
		memberName = m.Name
	}

	proposal := &Proposal{
		ProposalID:           proposalID,
		Arn:                  proposalARN(region, accountID, networkID, proposalID),
		NetworkID:            networkID,
		ProposedByMemberID:   memberID,
		ProposedByMemberName: memberName,
		Description:          description,
		Status:               proposalStatusInProgress,
		CreationDate:         &now,
		ExpirationDate:       &expiry,
		Tags:                 make(map[string]string),
		OutstandingVoteCount: memberCount,
	}

	b.proposals.Put(proposal)
	b.arnToResource[proposal.Arn] = proposal

	return cloneProposal(proposal)
}

// VoteOnProposal records a YES or NO vote on a proposal and transitions its status when threshold met.
func (b *InMemoryBackend) VoteOnProposal(networkID, proposalID, memberID, vote string) error {
	b.mu.Lock("VoteOnProposal")
	defer b.mu.Unlock()

	if _, exists := b.networks.Get(networkID); !exists {
		return ErrNetworkNotFound
	}

	proposal, exists := b.proposals.Get(proposalKey(networkID, proposalID))
	if !exists {
		return ErrProposalNotFound
	}

	expireProposalLocked(proposal)

	if proposal.Status != proposalStatusInProgress {
		return ErrValidation
	}

	member, exists := b.members.Get(memberKey(networkID, memberID))
	if !exists {
		return ErrMemberNotFound
	}

	if vote != "YES" && vote != "NO" {
		return ErrValidation
	}

	// Check for duplicate vote.
	for _, v := range b.proposalVotesByProposal.Get(proposalID) {
		if v.MemberID == memberID {
			return ErrValidation
		}
	}

	memberName := member.Name
	b.proposalVotes.Put(&ProposalVote{
		MemberID:   memberID,
		MemberName: memberName,
		Vote:       vote,
		proposalID: proposalID,
	})

	if vote == "YES" {
		proposal.YesVoteCount++
	} else {
		proposal.NoVoteCount++
	}

	// Recalculate outstanding votes.
	totalMembers := len(b.membersByNetwork.Get(networkID))
	proposal.OutstandingVoteCount = totalMembers - proposal.YesVoteCount - proposal.NoVoteCount

	// Apply threshold policy if network has one.
	network, _ := b.networks.Get(networkID)
	b.applyVoteThresholdLocked(network, proposal, totalMembers)

	return nil
}

// expireProposalLocked transitions an IN_PROGRESS proposal to EXPIRED once its
// ExpirationDate has passed with no decisive vote. Must be called with mu held.
//
// Real AWS: "EXPIRED - Members did not cast the number of votes required to
// determine the proposal outcome before the proposal expired." (AWS Managed
// Blockchain Hyperledger Fabric dev guide, "View Proposals" proposal statuses.)
func expireProposalLocked(proposal *Proposal) {
	if proposal.Status != proposalStatusInProgress {
		return
	}

	if proposal.ExpirationDate == nil || time.Now().UTC().Before(*proposal.ExpirationDate) {
		return
	}

	proposal.Status = proposalStatusExpired
}

// applyVoteThresholdLocked checks vote counts against the network's voting policy
// and transitions the proposal status when thresholds are met. Must be called with mu held.
func (b *InMemoryBackend) applyVoteThresholdLocked(network *Network, proposal *Proposal, totalMembers int) {
	if network.VotingPolicy == nil || network.VotingPolicy.ApprovalThresholdPolicy == nil {
		return
	}

	atp := network.VotingPolicy.ApprovalThresholdPolicy
	threshold := float64(atp.ThresholdPercentage)
	comparator := atp.ThresholdComparator

	if totalMembers == 0 || threshold == 0 {
		return
	}

	yesPercent := float64(proposal.YesVoteCount) * percentBase / float64(totalMembers)

	var yesApproved bool

	switch comparator {
	case "GREATER_THAN":
		yesApproved = yesPercent > threshold
	case "GREATER_THAN_OR_EQUAL_TO":
		yesApproved = yesPercent >= threshold
	default:
		yesApproved = yesPercent > threshold
	}

	// Rejection: it is mathematically impossible for approval to be reached, i.e.
	// even if all outstanding votes were YES, the proposal cannot be approved.
	// requiredYes = minimum YES votes needed for approval.
	var requiredYes float64

	switch comparator {
	case "GREATER_THAN_OR_EQUAL_TO":
		requiredYes = threshold / percentBase * float64(totalMembers)
		// ceil for fractional required votes
		if requiredYes != float64(int(requiredYes)) {
			requiredYes = float64(int(requiredYes)) + 1
		}
	default: // GREATER_THAN
		requiredYes = float64(int(threshold/percentBase*float64(totalMembers))) + 1
	}

	maxPossibleYes := float64(totalMembers - proposal.NoVoteCount)
	noRejected := maxPossibleYes < requiredYes

	if yesApproved {
		proposal.Status = proposalStatusApproved
		b.executeProposalActionsLocked(network, proposal)
	} else if noRejected {
		proposal.Status = proposalStatusRejected
	}
}

// arnRegionAccount extracts the region and account ID from an ARN string.
// ARN format: arn:{partition}:{service}:{region}:{accountID}:{resource}.
func arnRegionAccount(arnStr string) (string, string) {
	const arnParts = 6
	parts := strings.SplitN(arnStr, ":", arnParts)

	if len(parts) < arnParts {
		return "", ""
	}

	return parts[3], parts[4]
}

// executeProposalActionsLocked runs the actions from an approved proposal.
// Must be called with mu held.
func (b *InMemoryBackend) executeProposalActionsLocked(network *Network, proposal *Proposal) {
	if proposal.Actions == nil {
		return
	}

	now := time.Now().UTC()
	region, accountID := arnRegionAccount(network.Arn)

	for _, inv := range proposal.Actions.Invitations {
		invitationID := uuid.NewString()

		netSummary := &InvitationNetworkSummary{
			ID:               network.ID,
			Arn:              network.Arn,
			Name:             network.Name,
			Description:      network.Description,
			Framework:        network.Framework,
			FrameworkVersion: network.FrameworkVersion,
			Status:           network.Status,
			CreationDate:     network.CreationDate,
		}

		invitation := &Invitation{
			InvitationID:   invitationID,
			Arn:            invitationARN(region, inv.Principal, invitationID),
			NetworkID:      network.ID,
			NetworkName:    network.Name,
			Status:         invitationStatusPending,
			CreationDate:   &now,
			NetworkSummary: netSummary,
		}

		_ = accountID
		b.invitations.Put(invitation)
	}

	actionFailed := false

	for _, rem := range proposal.Actions.Removals {
		memberID := rem.MemberID

		m, exists := b.members.Get(memberKey(network.ID, memberID))
		if !exists {
			// Real AWS: "ACTION_FAILED ... One or more of the specified
			// ProposalActions in a proposal that was approved couldn't be
			// completed because of an error." A member can leave a network on
			// its own (DeleteMember) between proposal creation and approval,
			// making a pending RemoveAction stale by the time it executes.
			actionFailed = true

			continue
		}

		delete(b.arnToResource, m.Arn)
		b.members.Delete(memberKey(network.ID, memberID))

		b.deleteNodesForMemberLocked(network.ID, memberID)
	}

	if actionFailed {
		proposal.Status = proposalStatusActionFailed
	}

	deleteNetworkIfEmptyLocked(b, network)
}
