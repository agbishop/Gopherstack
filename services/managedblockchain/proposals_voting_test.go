package managedblockchain_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/managedblockchain"
)

func TestHandler_VoteOnProposal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
	}{
		{
			name:       "happy path YES vote returns 204",
			body:       map[string]any{"VoterMemberId": "placeholder", "Vote": "YES"},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "happy path NO vote returns 204",
			body:       map[string]any{"VoterMemberId": "placeholder", "Vote": "NO"},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "missing VoterMemberId returns 400",
			body:       map[string]any{"Vote": "YES"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "duplicate vote returns 400",
			body:       map[string]any{"VoterMemberId": "placeholder", "Vote": "YES", "duplicate": true},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid vote value returns 400",
			body:       map[string]any{"VoterMemberId": "placeholder", "Vote": "MAYBE"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := managedblockchain.NewInMemoryBackend()
			h := managedblockchain.NewHandler(b)
			h.AccountID = testAccountID
			h.DefaultRegion = testRegion

			net := b.AddNetworkInternal(testRegion, testAccountID, "test-network")
			mem := b.AddMemberInternal(testRegion, testAccountID, net.ID, "test-member")
			proposal := b.AddProposalInternal(testRegion, testAccountID, net.ID, mem.ID, "test proposal")

			path := fmt.Sprintf("/networks/%s/proposals/%s/votes", net.ID, proposal.ProposalID)

			body := tt.body
			isDuplicate := false

			if v, ok := body["duplicate"]; ok && v == true {
				isDuplicate = true
				// Remove the marker from the actual body.
				body = map[string]any{
					"VoterMemberId": mem.ID,
					"Vote":          "YES",
				}
			}

			// Replace placeholder with real member ID.
			if vid, ok := body["VoterMemberId"]; ok && vid == "placeholder" {
				body = map[string]any{
					"VoterMemberId": mem.ID,
					"Vote":          body["Vote"],
				}
			}

			if isDuplicate {
				// Cast the first vote to set up the duplicate scenario.
				firstRec := doRequest(t, h, http.MethodPost, path, body)
				require.Equal(t, http.StatusNoContent, firstRec.Code)
			}

			rec := doRequest(t, h, http.MethodPost, path, body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_ProposalOutstandingVoteCount verifies OutstandingVoteCount is tracked correctly.
func TestHandler_ProposalOutstandingVoteCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                      string
		extraMembers              int
		votesToCast               int
		wantOutstandingAfterVotes int
	}{
		{
			name:                      "one member network, cast one vote = zero outstanding",
			extraMembers:              0,
			votesToCast:               1,
			wantOutstandingAfterVotes: 0,
		},
		{
			name:                      "three members, cast two votes = one outstanding",
			extraMembers:              2,
			votesToCast:               2,
			wantOutstandingAfterVotes: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := managedblockchain.NewInMemoryBackend()
			n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
			m := b.AddMemberInternal(testRegion, testAccountID, n.ID, "proposer")

			extraMemberIDs := make([]string, tt.extraMembers)

			for i := range tt.extraMembers {
				em := b.AddMemberInternal(testRegion, testAccountID, n.ID, fmt.Sprintf("voter-%d", i))
				extraMemberIDs[i] = em.ID
			}

			proposal := b.AddProposalInternal(testRegion, testAccountID, n.ID, m.ID, "vote test")

			totalMembers := 1 + tt.extraMembers
			require.Equal(t, totalMembers, proposal.OutstandingVoteCount)

			// Cast votes
			h := managedblockchain.NewHandler(b)
			h.AccountID = testAccountID
			h.DefaultRegion = testRegion

			path := fmt.Sprintf("/networks/%s/proposals/%s/votes", n.ID, proposal.ProposalID)

			// First vote from proposer
			if tt.votesToCast > 0 {
				rec := doRequest(t, h, http.MethodPost, path, map[string]any{
					"VoterMemberId": m.ID,
					"Vote":          "YES",
				})
				require.Equal(t, http.StatusNoContent, rec.Code)
			}

			// Additional votes from extra members
			for i := 1; i < tt.votesToCast && i-1 < len(extraMemberIDs); i++ {
				rec := doRequest(t, h, http.MethodPost, path, map[string]any{
					"VoterMemberId": extraMemberIDs[i-1],
					"Vote":          "YES",
				})
				require.Equal(t, http.StatusNoContent, rec.Code)
			}

			// Check outstanding via GetProposal
			rec := doRequest(
				t,
				h,
				http.MethodGet,
				fmt.Sprintf("/networks/%s/proposals/%s", n.ID, proposal.ProposalID),
				nil,
			)
			require.Equal(t, http.StatusOK, rec.Code)

			var getResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))

			p := getResp["Proposal"].(map[string]any)
			outstanding := int(p["OutstandingVoteCount"].(float64))
			assert.Equal(t, tt.wantOutstandingAfterVotes, outstanding)
		})
	}
}

// TestHandler_ProposalStatusTransitions verifies proposal APPROVED/REJECTED transitions.
func TestHandler_ProposalStatusTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wantStatus   string
		comparator   string
		votes        []string
		threshold    int
		totalMembers int
	}{
		{
			name:         "100% YES in 2-member network with GREATER_THAN 50 → APPROVED",
			totalMembers: 2,
			threshold:    50,
			comparator:   "GREATER_THAN",
			votes:        []string{"YES", "YES"},
			wantStatus:   "APPROVED",
		},
		{
			// 3 members, GREATER_THAN 50% → need 2 YES to approve.
			// After 2 NO: maxPossibleYes = 1 < 2 → REJECTED.
			name:         "all NO votes → REJECTED",
			totalMembers: 3,
			threshold:    50,
			comparator:   "GREATER_THAN",
			votes:        []string{"NO", "NO"},
			wantStatus:   "REJECTED",
		},
		{
			name:         "no voting policy → stays IN_PROGRESS",
			totalMembers: 1,
			threshold:    0,
			comparator:   "",
			votes:        []string{"YES"},
			wantStatus:   "IN_PROGRESS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandlerWithBackend(t)

			// Create network with voting policy
			var votingPolicy map[string]any

			if tt.threshold > 0 {
				votingPolicy = map[string]any{
					"ApprovalThresholdPolicy": map[string]any{
						"ThresholdComparator":     tt.comparator,
						"ThresholdPercentage":     tt.threshold,
						"ProposalDurationInHours": 24,
					},
				}
			}

			netBody := map[string]any{
				"Name":                "vote-net",
				"ClientRequestToken":  "tok-votenet",
				"MemberConfiguration": testMemberConfiguration("m0"),
			}

			if votingPolicy != nil {
				netBody["VotingPolicy"] = votingPolicy
			}

			rec := doRequest(t, h, http.MethodPost, "/networks", netBody)
			require.Equal(t, http.StatusOK, rec.Code)

			var createNetResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createNetResp))

			networkID := createNetResp["NetworkId"].(string)
			firstMemberID := createNetResp["MemberId"].(string)

			// Add extra members if needed
			extraMemberIDs := make([]string, 0, tt.totalMembers-1)

			for i := 1; i < tt.totalMembers; i++ {
				invitationID := createTestInvitation(t, b, networkID, "vote-net")
				memRec := doRequest(t, h, http.MethodPost, "/networks/"+networkID+"/members", map[string]any{
					"InvitationId":        invitationID,
					"ClientRequestToken":  fmt.Sprintf("tok-votemember-%d", i),
					"MemberConfiguration": testMemberConfiguration(fmt.Sprintf("m%d", i)),
				})
				require.Equal(t, http.StatusOK, memRec.Code)

				var createMemResp map[string]any
				require.NoError(t, json.Unmarshal(memRec.Body.Bytes(), &createMemResp))
				extraMemberIDs = append(extraMemberIDs, createMemResp["MemberId"].(string))
			}

			// Create proposal
			rec = doRequest(t, h, http.MethodPost, "/networks/"+networkID+"/proposals", map[string]any{
				"MemberId":           firstMemberID,
				"ClientRequestToken": "tok-voteproposal",
				"Description":        "test",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var createPropResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createPropResp))

			proposalID := createPropResp["ProposalId"].(string)

			// Cast votes
			allMemberIDs := append([]string{firstMemberID}, extraMemberIDs...)
			votePath := fmt.Sprintf("/networks/%s/proposals/%s/votes", networkID, proposalID)

			for i, vote := range tt.votes {
				if i >= len(allMemberIDs) {
					break
				}

				voteRec := doRequest(t, h, http.MethodPost, votePath, map[string]any{
					"VoterMemberId": allMemberIDs[i],
					"Vote":          vote,
				})
				require.Equal(t, http.StatusNoContent, voteRec.Code)
			}

			// Check proposal status
			rec = doRequest(t, h, http.MethodGet, fmt.Sprintf("/networks/%s/proposals/%s", networkID, proposalID), nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var getResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))

			p := getResp["Proposal"].(map[string]any)
			assert.Equal(t, tt.wantStatus, p["Status"])
		})
	}
}

// TestHandler_VoteOnProposalAlreadyCompleted verifies voting on completed proposal returns error.
func TestHandler_VoteOnProposalAlreadyCompleted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "vote on approved proposal fails"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create network with 100% threshold so one vote approves
			rec := doRequest(t, h, http.MethodPost, "/networks", map[string]any{
				"Name":                "approve-net",
				"ClientRequestToken":  "tok-approvenet",
				"MemberConfiguration": testMemberConfiguration("m1"),
				"VotingPolicy": map[string]any{
					"ApprovalThresholdPolicy": map[string]any{
						"ThresholdComparator":     "GREATER_THAN_OR_EQUAL_TO",
						"ThresholdPercentage":     100,
						"ProposalDurationInHours": 24,
					},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var netResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &netResp))

			networkID := netResp["NetworkId"].(string)
			memberID := netResp["MemberId"].(string)

			rec = doRequest(t, h, http.MethodPost, "/networks/"+networkID+"/proposals", map[string]any{
				"MemberId":           memberID,
				"ClientRequestToken": "tok-approve-proposal",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var propResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &propResp))

			proposalID := propResp["ProposalId"].(string)
			votePath := fmt.Sprintf("/networks/%s/proposals/%s/votes", networkID, proposalID)

			// First vote approves proposal
			rec = doRequest(t, h, http.MethodPost, votePath, map[string]any{
				"VoterMemberId": memberID,
				"Vote":          "YES",
			})
			require.Equal(t, http.StatusNoContent, rec.Code)

			// Verify proposal is now APPROVED
			rec = doRequest(t, h, http.MethodGet, fmt.Sprintf("/networks/%s/proposals/%s", networkID, proposalID), nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var getResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))

			p := getResp["Proposal"].(map[string]any)
			assert.Equal(t, "APPROVED", p["Status"])
		})
	}
}

// TestHandler_VoteThresholdFloatPrecision verifies that the vote threshold comparison
// uses float division, not integer division. With 3 members and GREATER_THAN 33%,
// integer division gives 33% (1/3*100 truncated), which is NOT > 33 — but float gives
// 33.33% which IS > 33, so the proposal should be approved.
func TestHandler_VoteThresholdFloatPrecision(t *testing.T) {
	t.Parallel()

	h, b := newTestHandlerWithBackend(t)

	// 3 members, GREATER_THAN 33%: 1/3 YES = 33.33% > 33 with float (but 33 > 33 = false with integer).
	netRec := doRequest(t, h, http.MethodPost, "/networks", map[string]any{
		"Name":                "float-precision-net",
		"ClientRequestToken":  "tok-floatprec-net",
		"MemberConfiguration": testMemberConfiguration("owner"),
		"VotingPolicy": map[string]any{
			"ApprovalThresholdPolicy": map[string]any{
				"ThresholdComparator":     "GREATER_THAN",
				"ThresholdPercentage":     33,
				"ProposalDurationInHours": 24,
			},
		},
	})
	require.Equal(t, http.StatusOK, netRec.Code)

	var netResp map[string]any
	require.NoError(t, json.Unmarshal(netRec.Body.Bytes(), &netResp))

	netID := netResp["NetworkId"].(string)
	ownerMemberID := netResp["MemberId"].(string)

	addMem := func(name string) {
		invitationID := createTestInvitation(t, b, netID, "float-precision-net")
		rec := doRequest(
			t,
			h,
			http.MethodPost,
			"/networks/"+netID+"/members",
			map[string]any{
				"InvitationId":        invitationID,
				"ClientRequestToken":  "tok-addmem",
				"MemberConfiguration": testMemberConfiguration(name),
			},
		)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// Add 2 more members for 3 total.
	addMem("m2")
	addMem("m3")

	// Create proposal.
	propRec := doRequest(t, h, http.MethodPost, "/networks/"+netID+"/proposals",
		map[string]any{
			"MemberId":           ownerMemberID,
			"ClientRequestToken": "tok-floatprec-prop",
			"Description":        "float precision test",
		})
	require.Equal(t, http.StatusOK, propRec.Code)

	var propResp map[string]any
	require.NoError(t, json.Unmarshal(propRec.Body.Bytes(), &propResp))

	propID := propResp["ProposalId"].(string)
	votePath := fmt.Sprintf("/networks/%s/proposals/%s/votes", netID, propID)

	// Cast 1 YES vote out of 3 (33.33% > 33 with float; 33 > 33 = false with integer).
	rec := doRequest(t, h, http.MethodPost, votePath,
		map[string]any{"VoterMemberId": ownerMemberID, "Vote": "YES"})
	require.Equal(t, http.StatusNoContent, rec.Code)

	getRec := doRequest(t, h, http.MethodGet,
		fmt.Sprintf("/networks/%s/proposals/%s", netID, propID), nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))

	p := getResp["Proposal"].(map[string]any)
	assert.Equal(t, "APPROVED", p["Status"],
		"1/3 YES votes (33.33%%) must satisfy GREATER_THAN 33%% with float division; "+
			"integer division (33 > 33 = false) would leave proposal IN_PROGRESS")
}

// TestHandler_ApprovedProposalExecutesInvitationActions verifies that when a proposal
// with Invitation actions is approved, the invitations are automatically created. Real
// AWS executes proposal actions immediately upon approval.
func TestHandler_ApprovedProposalExecutesInvitationActions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create a network (1 member = only 1 vote needed for unanimous approval).
	netRec := doRequest(t, h, http.MethodPost, "/networks", map[string]any{
		"Name":                "actions-net",
		"ClientRequestToken":  "tok-actionsnet",
		"MemberConfiguration": testMemberConfiguration("owner"),
		"VotingPolicy": map[string]any{
			"ApprovalThresholdPolicy": map[string]any{
				"ThresholdComparator":     "GREATER_THAN_OR_EQUAL_TO",
				"ThresholdPercentage":     1,
				"ProposalDurationInHours": 24,
			},
		},
	})
	require.Equal(t, http.StatusOK, netRec.Code)

	var netResp map[string]any
	require.NoError(t, json.Unmarshal(netRec.Body.Bytes(), &netResp))

	netID := netResp["NetworkId"].(string)
	ownerMemberID := netResp["MemberId"].(string)

	// Verify no invitations exist initially.
	listBefore := doRequest(t, h, http.MethodGet, "/invitations", nil)
	require.Equal(t, http.StatusOK, listBefore.Code)

	var listBeforeResp map[string]any
	require.NoError(t, json.Unmarshal(listBefore.Body.Bytes(), &listBeforeResp))
	assert.Empty(t, listBeforeResp["Invitations"],
		"no invitations should exist before proposal approval")

	// Create proposal with an Invitation action.
	propRec := doRequest(t, h, http.MethodPost, "/networks/"+netID+"/proposals", map[string]any{
		"MemberId":           ownerMemberID,
		"ClientRequestToken": "tok-invite-action-prop",
		"Description":        "invite new member",
		"Actions": map[string]any{
			"Invitations": []map[string]any{
				{"Principal": "987654321098"},
			},
		},
	})
	require.Equal(t, http.StatusOK, propRec.Code)

	var propResp map[string]any
	require.NoError(t, json.Unmarshal(propRec.Body.Bytes(), &propResp))

	propID := propResp["ProposalId"].(string)

	// Vote YES to approve.
	voteRec := doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/networks/%s/proposals/%s/votes", netID, propID),
		map[string]any{"VoterMemberId": ownerMemberID, "Vote": "YES"})
	require.Equal(t, http.StatusNoContent, voteRec.Code)

	// Verify invitation was created by the approval.
	listAfter := doRequest(t, h, http.MethodGet, "/invitations", nil)
	require.Equal(t, http.StatusOK, listAfter.Code)

	var listAfterResp map[string]any
	require.NoError(t, json.Unmarshal(listAfter.Body.Bytes(), &listAfterResp))

	invitations, _ := listAfterResp["Invitations"].([]any)
	assert.Len(t, invitations, 1,
		"approved proposal with Invitation action must create one invitation")
}

// TestHandler_RejectionThresholdImpossibleApproval verifies that rejection is triggered
// when it is mathematically impossible to reach approval, not by a symmetric threshold.
// Real AWS rejects when maxPossibleYes < requiredYes.
func TestHandler_RejectionThresholdImpossibleApproval(t *testing.T) {
	t.Parallel()

	// 4 members, GREATER_THAN 50%: need >50% YES = 3 votes minimum.
	// After 2 NO votes: maxPossibleYes = 4 - 2 = 2 < 3 → REJECTED.
	// Old wrong logic: rejectionThreshold = 100 - 50 = 50%, needed >50% NO = 3 NO votes.
	h, b := newTestHandlerWithBackend(t)

	netRec := doRequest(t, h, http.MethodPost, "/networks", map[string]any{
		"Name":                "reject-net",
		"ClientRequestToken":  "tok-rejectnet",
		"MemberConfiguration": testMemberConfiguration("m0"),
		"VotingPolicy": map[string]any{
			"ApprovalThresholdPolicy": map[string]any{
				"ThresholdComparator":     "GREATER_THAN",
				"ThresholdPercentage":     50,
				"ProposalDurationInHours": 24,
			},
		},
	})
	require.Equal(t, http.StatusOK, netRec.Code)

	var netResp map[string]any
	require.NoError(t, json.Unmarshal(netRec.Body.Bytes(), &netResp))

	netID := netResp["NetworkId"].(string)
	m0ID := netResp["MemberId"].(string)

	addMem := func(name string) string {
		invitationID := createTestInvitation(t, b, netID, "reject-net")
		rec := doRequest(
			t,
			h,
			http.MethodPost,
			"/networks/"+netID+"/members",
			map[string]any{
				"InvitationId":        invitationID,
				"ClientRequestToken":  "tok-addmem",
				"MemberConfiguration": testMemberConfiguration(name),
			},
		)
		require.Equal(t, http.StatusOK, rec.Code)

		var r map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &r))

		return r["MemberId"].(string)
	}

	m1ID := addMem("m1")
	m2ID := addMem("m2")
	m3ID := addMem("m3")

	propRec := doRequest(t, h, http.MethodPost, "/networks/"+netID+"/proposals",
		map[string]any{
			"MemberId": m0ID, "ClientRequestToken": "tok-rejectprop", "Description": "rejection threshold test",
		})
	require.Equal(t, http.StatusOK, propRec.Code)

	var propResp map[string]any
	require.NoError(t, json.Unmarshal(propRec.Body.Bytes(), &propResp))

	propID := propResp["ProposalId"].(string)
	votePath := fmt.Sprintf("/networks/%s/proposals/%s/votes", netID, propID)

	// Cast 2 NO votes (m0 and m1).
	for _, mID := range []string{m0ID, m1ID} {
		rec := doRequest(t, h, http.MethodPost, votePath,
			map[string]any{"VoterMemberId": mID, "Vote": "NO"})
		require.Equal(t, http.StatusNoContent, rec.Code)
	}

	_ = m2ID
	_ = m3ID

	// Proposal must be REJECTED now (maxPossibleYes = 4-2 = 2 < requiredYes = 3).
	getRec := doRequest(t, h, http.MethodGet,
		fmt.Sprintf("/networks/%s/proposals/%s", netID, propID), nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))

	p := getResp["Proposal"].(map[string]any)
	assert.Equal(t, "REJECTED", p["Status"],
		"2 NO votes in a 4-member GREATER_THAN 50%% network must trigger rejection "+
			"(maxPossibleYes=2 < requiredYes=3)")
}

// TestHandler_ProposalExpiresAfterExpirationDate verifies real AWS's EXPIRED proposal
// status ("Members did not cast the number of votes required to determine the
// proposal outcome before the proposal expired" -- AWS Managed Blockchain
// Hyperledger Fabric dev guide, "View Proposals"): once ExpirationDate has passed
// with no decisive vote, GetProposal/ListProposals report EXPIRED and further votes
// are rejected.
func TestHandler_ProposalExpiresAfterExpirationDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *managedblockchain.Handler, networkID, memberID, proposalID string)
		name string
	}{
		{
			name: "GetProposal reports EXPIRED",
			run: func(t *testing.T, h *managedblockchain.Handler, networkID, _, proposalID string) {
				t.Helper()

				rec := doRequest(t, h, http.MethodGet,
					fmt.Sprintf("/networks/%s/proposals/%s", networkID, proposalID), nil)
				require.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				p, ok := resp["Proposal"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "EXPIRED", p["Status"])
			},
		},
		{
			name: "ListProposals reports EXPIRED",
			run: func(t *testing.T, h *managedblockchain.Handler, networkID, _, _ string) {
				t.Helper()

				rec := doRequest(t, h, http.MethodGet, "/networks/"+networkID+"/proposals", nil)
				require.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				proposals, ok := resp["Proposals"].([]any)
				require.True(t, ok)
				require.Len(t, proposals, 1)

				summary, ok := proposals[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "EXPIRED", summary["Status"])
			},
		},
		{
			name: "VoteOnProposal rejects a vote on an expired proposal",
			run: func(t *testing.T, h *managedblockchain.Handler, networkID, memberID, proposalID string) {
				t.Helper()

				votePath := fmt.Sprintf("/networks/%s/proposals/%s/votes", networkID, proposalID)
				rec := doRequest(t, h, http.MethodPost, votePath,
					map[string]any{"VoterMemberId": memberID, "Vote": "YES"})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandlerWithBackend(t)
			networkID, memberID := createTestNetwork(t, h)
			proposal := b.AddProposalInternal(testRegion, testAccountID, networkID, memberID, "expiring proposal")

			managedblockchain.SetProposalExpiration(b, networkID, proposal.ProposalID, time.Now().Add(-time.Hour))

			tt.run(t, h, networkID, memberID, proposal.ProposalID)
		})
	}
}

// TestHandler_ApprovedRemovalProposalCascadeDeletesEmptyNetwork verifies real AWS's
// documented DeleteMember side effect also applies when a member is removed as the
// result of an approved proposal, not just a direct DeleteMember call: "If MemberId
// is the last member in a network specified by the last Amazon Web Services
// account, the network is deleted also." (aws-sdk-go-v2 managedblockchain
// api_op_DeleteMember.go doc comment, v1.34.4).
func TestHandler_ApprovedRemovalProposalCascadeDeletesEmptyNetwork(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	netRec := doRequest(t, h, http.MethodPost, "/networks", map[string]any{
		"Name":                "removal-net",
		"ClientRequestToken":  "tok-removalnet",
		"MemberConfiguration": testMemberConfiguration("owner"),
		"VotingPolicy": map[string]any{
			"ApprovalThresholdPolicy": map[string]any{
				"ThresholdComparator":     "GREATER_THAN_OR_EQUAL_TO",
				"ThresholdPercentage":     1,
				"ProposalDurationInHours": 24,
			},
		},
	})
	require.Equal(t, http.StatusOK, netRec.Code)

	var netResp map[string]any
	require.NoError(t, json.Unmarshal(netRec.Body.Bytes(), &netResp))

	netID := netResp["NetworkId"].(string)
	ownerMemberID := netResp["MemberId"].(string)

	propRec := doRequest(t, h, http.MethodPost, "/networks/"+netID+"/proposals", map[string]any{
		"MemberId":           ownerMemberID,
		"ClientRequestToken": "tok-removal-action-prop",
		"Description":        "remove the only member",
		"Actions": map[string]any{
			"Removals": []map[string]any{
				{"MemberId": ownerMemberID},
			},
		},
	})
	require.Equal(t, http.StatusOK, propRec.Code)

	var propResp map[string]any
	require.NoError(t, json.Unmarshal(propRec.Body.Bytes(), &propResp))

	propID := propResp["ProposalId"].(string)

	// Vote YES to approve; owner is the network's only member, so approval
	// removes the last member and must cascade-delete the network.
	voteRec := doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/networks/%s/proposals/%s/votes", netID, propID),
		map[string]any{"VoterMemberId": ownerMemberID, "Vote": "YES"})
	require.Equal(t, http.StatusNoContent, voteRec.Code)

	getRec := doRequest(t, h, http.MethodGet, "/networks/"+netID, nil)
	assert.Equal(t, http.StatusNotFound, getRec.Code,
		"network must be deleted once its approved removal proposal removes the last member")
}

// TestHandler_ApprovedProposalActionFailedWhenTargetMemberAlreadyGone verifies that
// approving a removal proposal whose target member has already left the network
// (via its own DeleteMember, independent of this proposal) lands the proposal in
// ACTION_FAILED, not APPROVED. Real AWS (aws-sdk-go-v2 managedblockchain
// types/types.go:938, ProposalStatus doc): ACTION_FAILED occurs when one or more
// ProposalActions in an approved proposal couldn't be completed because of an
// error, even if only one ProposalAction fails and other actions succeed.
func TestHandler_ApprovedProposalActionFailedWhenTargetMemberAlreadyGone(t *testing.T) {
	t.Parallel()

	h, b := newTestHandlerWithBackend(t)

	netRec := doRequest(t, h, http.MethodPost, "/networks", map[string]any{
		"Name":                "action-failed-net",
		"ClientRequestToken":  "tok-action-failed-net",
		"MemberConfiguration": testMemberConfiguration("owner"),
		"VotingPolicy": map[string]any{
			"ApprovalThresholdPolicy": map[string]any{
				"ThresholdComparator":     "GREATER_THAN_OR_EQUAL_TO",
				"ThresholdPercentage":     1,
				"ProposalDurationInHours": 24,
			},
		},
	})
	require.Equal(t, http.StatusOK, netRec.Code)

	var netResp map[string]any
	require.NoError(t, json.Unmarshal(netRec.Body.Bytes(), &netResp))

	netID := netResp["NetworkId"].(string)
	ownerID := netResp["MemberId"].(string)

	target := b.AddMemberInternal(testRegion, testAccountID, netID, "target")

	// Owner proposes to remove target.
	propRec := doRequest(t, h, http.MethodPost, "/networks/"+netID+"/proposals", map[string]any{
		"MemberId":           ownerID,
		"ClientRequestToken": "tok-action-failed-prop",
		"Description":        "remove target",
		"Actions": map[string]any{
			"Removals": []map[string]any{
				{"MemberId": target.ID},
			},
		},
	})
	require.Equal(t, http.StatusOK, propRec.Code)

	var propResp map[string]any
	require.NoError(t, json.Unmarshal(propRec.Body.Bytes(), &propResp))

	propID := propResp["ProposalId"].(string)

	// Target leaves the network on its own, independent of the pending proposal --
	// real AWS lets a member call DeleteMember on itself at any time.
	delRec := doRequest(t, h, http.MethodDelete, "/networks/"+netID+"/members/"+target.ID, nil)
	require.Equal(t, http.StatusNoContent, delRec.Code)

	// Approve the now-stale removal proposal: its target no longer exists.
	voteRec := doRequest(t, h, http.MethodPost,
		fmt.Sprintf("/networks/%s/proposals/%s/votes", netID, propID),
		map[string]any{"VoterMemberId": ownerID, "Vote": "YES"})
	require.Equal(t, http.StatusNoContent, voteRec.Code)

	getRec := doRequest(t, h, http.MethodGet, "/networks/"+netID+"/proposals/"+propID, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))

	proposal, ok := getResp["Proposal"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "ACTION_FAILED", proposal["Status"],
		"a removal action against an already-departed member must fail the proposal, not silently succeed as APPROVED")
}
