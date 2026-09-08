package managedblockchain_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/managedblockchain"
)

func TestInMemoryBackend_MemberLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		memberName string
	}{
		{
			name:       "full member lifecycle",
			memberName: "test-member",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()

			network, _, err := b.CreateNetwork(
				testRegion,
				testAccountID,
				"lifecycle-net",
				"",
				"",
				"",
				"initial",
				"",
				nil, nil,
				nil,
				"",
				"admin",
				"")
			require.NoError(t, err)

			inv := b.AddInvitationInternal(testRegion, testAccountID, network.ID, "lifecycle-net")

			// CreateMember
			member, err := b.CreateMember(
				testRegion, testAccountID, network.ID, inv.InvitationID, tt.memberName, "", "admin", "", nil)
			require.NoError(t, err)
			assert.NotEmpty(t, member.ID)
			assert.Equal(t, tt.memberName, member.Name)
			assert.Equal(t, "AVAILABLE", member.Status)

			// GetMember
			got, err := b.GetMember(network.ID, member.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.memberName, got.Name)

			// ListMembers - should have initial + new member
			members, err := b.ListMembers(network.ID, managedblockchain.ListMembersFilter{})
			require.NoError(t, err)
			assert.Len(t, members, 2)

			// DeleteMember
			err = b.DeleteMember(network.ID, member.ID)
			require.NoError(t, err)

			// Verify deletion
			_, err = b.GetMember(network.ID, member.ID)
			require.Error(t, err)
			assert.ErrorIs(t, err, awserr.ErrNotFound)
		})
	}
}

func TestInMemoryBackend_CreateMember_NetworkNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "member in nonexistent network"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()

			_, err := b.CreateMember(testRegion, testAccountID, "nonexistent-id", "inv1", "m1", "", "admin", "", nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, awserr.ErrNotFound)
		})
	}
}

func TestHandler_MemberLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "create get list delete member"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandlerWithBackend(t)

			// Create network
			rec := doRequest(t, h, http.MethodPost, "/networks",
				map[string]any{
					"Name":                "net1",
					"ClientRequestToken":  "tok-net1",
					"MemberConfiguration": testMemberConfiguration("initial"),
				})
			require.Equal(t, http.StatusOK, rec.Code)

			var createNetResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createNetResp))
			networkID := createNetResp["NetworkId"].(string)

			// Create member
			invitationID := createTestInvitation(t, b, networkID, "net1")
			rec = doRequest(t, h, http.MethodPost, "/networks/"+networkID+"/members",
				map[string]any{
					"InvitationId":        invitationID,
					"ClientRequestToken":  "tok-newmember",
					"MemberConfiguration": testMemberConfiguration("new-member"),
				})
			require.Equal(t, http.StatusOK, rec.Code)

			var createMemResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createMemResp))
			memberID := createMemResp["MemberId"].(string)
			assert.NotEmpty(t, memberID)

			// Get member
			rec = doRequest(t, h, http.MethodGet, "/networks/"+networkID+"/members/"+memberID, nil)
			assert.Equal(t, http.StatusOK, rec.Code)

			// List members - initial + new
			rec = doRequest(t, h, http.MethodGet, "/networks/"+networkID+"/members", nil)
			assert.Equal(t, http.StatusOK, rec.Code)

			var listResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
			members, ok := listResp["Members"].([]any)
			require.True(t, ok)
			assert.Len(t, members, 2)

			// Delete member
			rec = doRequest(t, h, http.MethodDelete, "/networks/"+networkID+"/members/"+memberID, nil)
			assert.Equal(t, http.StatusNoContent, rec.Code)

			// Verify not found
			rec = doRequest(t, h, http.MethodGet, "/networks/"+networkID+"/members/"+memberID, nil)
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

func TestHandler_MemberErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		op         string
		wantStatus int
	}{
		{
			name:       "create member missing member name",
			wantStatus: http.StatusBadRequest,
			op:         "create_missing_name",
		},
		{
			name:       "create member in nonexistent network",
			wantStatus: http.StatusNotFound,
			op:         "create_bad_network",
		},
		{
			name:       "list members bad network",
			wantStatus: http.StatusNotFound,
			op:         "list_bad_network",
		},
		{
			name:       "delete member bad network",
			wantStatus: http.StatusNotFound,
			op:         "delete_bad_network",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			var rec *httptest.ResponseRecorder

			switch tt.op {
			case "create_missing_name":
				rec = doRequest(t, h, http.MethodPost, "/networks/net1/members",
					map[string]any{"MemberConfiguration": map[string]any{}})
			case "create_bad_network":
				rec = doRequest(t, h, http.MethodPost, "/networks/nonexistent/members",
					map[string]any{
						"InvitationId":        "some-invitation-id",
						"ClientRequestToken":  "tok-badnet",
						"MemberConfiguration": testMemberConfiguration("m1"),
					})
			case "list_bad_network":
				rec = doRequest(t, h, http.MethodGet, "/networks/nonexistent/members", nil)
			case "delete_bad_network":
				rec = doRequest(t, h, http.MethodDelete, "/networks/nonexistent/members/mid", nil)
			}

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_CreateMember_InvitationId verifies CreateMember validates
// InvitationId the way the real API requires it: the real aws-sdk-go-v2
// client-side validator (validateOpCreateMemberInput, validators.go:805-806,
// v1.34.4) marks InvitationId required, and Invitation.Status's doc comment
// (types/enums.go:110-114) documents PENDING as the only status an
// invitation can be consumed from -- ACCEPTED/REJECTED/EXPIRED cannot be
// reused. gopherstack-u84u: CreateMember previously parsed InvitationId off
// the request body and then never read it again.
func TestHandler_CreateMember_InvitationId(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, b *managedblockchain.InMemoryBackend, networkID string) string
		name       string
		wantStatus int
	}{
		{
			name: "missing invitation id is rejected",
			setup: func(*testing.T, *managedblockchain.InMemoryBackend, string) string {
				return ""
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unknown invitation id is rejected",
			setup: func(*testing.T, *managedblockchain.InMemoryBackend, string) string {
				return "no-such-invitation"
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "invitation for a different network is rejected",
			setup: func(t *testing.T, b *managedblockchain.InMemoryBackend, _ string) string {
				t.Helper()

				other := b.AddNetworkInternal(testRegion, testAccountID, "other-net")

				return createTestInvitation(t, b, other.ID, "other-net")
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "already-rejected invitation is rejected",
			setup: func(t *testing.T, b *managedblockchain.InMemoryBackend, networkID string) string {
				t.Helper()

				invitationID := createTestInvitation(t, b, networkID, "invite-net")
				require.NoError(t, b.RejectInvitation(invitationID))

				return invitationID
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "already-accepted invitation cannot be reused",
			setup: func(t *testing.T, b *managedblockchain.InMemoryBackend, networkID string) string {
				t.Helper()

				invitationID := createTestInvitation(t, b, networkID, "invite-net")
				_, err := b.CreateMember(
					testRegion, testAccountID, networkID, invitationID, "first", "", "admin", "", nil)
				require.NoError(t, err)

				return invitationID
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "pending invitation for this network is accepted",
			setup: func(t *testing.T, b *managedblockchain.InMemoryBackend, networkID string) string {
				t.Helper()

				return createTestInvitation(t, b, networkID, "invite-net")
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := managedblockchain.NewInMemoryBackend()
			n := b.AddNetworkInternal(testRegion, testAccountID, "invite-net")
			h := managedblockchain.NewHandler(b)
			h.AccountID = testAccountID
			h.DefaultRegion = testRegion

			invitationID := tt.setup(t, b, n.ID)

			rec := doRequest(t, h, http.MethodPost, "/networks/"+n.ID+"/members",
				map[string]any{
					"InvitationId":        invitationID,
					"ClientRequestToken":  "tok-invite",
					"MemberConfiguration": testMemberConfiguration("m1"),
				})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_CreateMember_ConsumesInvitation verifies a successful
// CreateMember transitions its invitation from PENDING to ACCEPTED (real
// AWS's Invitation.Status doc: "ACCEPTED - The invitee created a member and
// joined the network using the InvitationId", types/enums.go:111,
// aws-sdk-go-v2 managedblockchain@v1.34.4) so it cannot be replayed.
func TestHandler_CreateMember_ConsumesInvitation(t *testing.T) {
	t.Parallel()

	b := managedblockchain.NewInMemoryBackend()
	n := b.AddNetworkInternal(testRegion, testAccountID, "invite-net")
	h := managedblockchain.NewHandler(b)
	h.AccountID = testAccountID
	h.DefaultRegion = testRegion

	invitationID := createTestInvitation(t, b, n.ID, "invite-net")

	rec := doRequest(t, h, http.MethodPost, "/networks/"+n.ID+"/members",
		map[string]any{
			"InvitationId":        invitationID,
			"ClientRequestToken":  "tok-consume-1",
			"MemberConfiguration": testMemberConfiguration("m1"),
		})
	require.Equal(t, http.StatusOK, rec.Code)

	invitations, err := b.ListInvitations()
	require.NoError(t, err)
	require.Len(t, invitations, 1)
	assert.Equal(t, "ACCEPTED", invitations[0].Status)

	// Replaying the same invitation must now fail: it is no longer PENDING.
	rec = doRequest(t, h, http.MethodPost, "/networks/"+n.ID+"/members",
		map[string]any{
			"InvitationId":        invitationID,
			"ClientRequestToken":  "tok-consume-2",
			"MemberConfiguration": testMemberConfiguration("m2"),
		})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_ListMembersFilters verifies query param filtering for ListMembers.
func TestHandler_ListMembersFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query     map[string]string
		name      string
		wantCount int
	}{
		{
			name:      "no filter returns all",
			query:     map[string]string{},
			wantCount: 3,
		},
		{
			name:      "filter by name",
			query:     map[string]string{"name": "alice"},
			wantCount: 1,
		},
		{
			name:      "filter by status",
			query:     map[string]string{"status": "AVAILABLE"},
			wantCount: 3,
		},
		{
			name:      "filter isOwned=true returns all (all seeded members are owned)",
			query:     map[string]string{"isOwned": "true"},
			wantCount: 3,
		},
		{
			name:      "filter isOwned=false returns none (all seeded members are owned)",
			query:     map[string]string{"isOwned": "false"},
			wantCount: 0,
		},
		{
			name:      "filter by nonexistent name",
			query:     map[string]string{"name": "nobody"},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := managedblockchain.NewInMemoryBackend()
			n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
			b.AddMemberInternal(testRegion, testAccountID, n.ID, "alice")
			b.AddMemberInternal(testRegion, testAccountID, n.ID, "bob")
			b.AddMemberInternal(testRegion, testAccountID, n.ID, "carol")

			h := managedblockchain.NewHandler(b)
			h.AccountID = testAccountID
			h.DefaultRegion = testRegion

			rec := doRequestWithQuery(t, h, "/networks/"+n.ID+"/members", tt.query)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			members := resp["Members"].([]any)
			assert.Len(t, members, tt.wantCount)
		})
	}
}

// TestInMemoryBackend_ListMembersFilter verifies backend-level filtering for ListMembers.
func TestInMemoryBackend_ListMembersFilter(t *testing.T) {
	t.Parallel()

	trueVal := true
	falseVal := false

	tests := []struct {
		filter    managedblockchain.ListMembersFilter
		name      string
		wantCount int
	}{
		{
			name:      "empty filter returns all",
			filter:    managedblockchain.ListMembersFilter{},
			wantCount: 2,
		},
		{
			name:      "filter by Name",
			filter:    managedblockchain.ListMembersFilter{Name: "alice"},
			wantCount: 1,
		},
		{
			name:      "filter by Status",
			filter:    managedblockchain.ListMembersFilter{Status: "AVAILABLE"},
			wantCount: 2,
		},
		{
			name:      "filter isOwned=true",
			filter:    managedblockchain.ListMembersFilter{IsOwned: &trueVal},
			wantCount: 2,
		},
		{
			name:      "filter isOwned=false",
			filter:    managedblockchain.ListMembersFilter{IsOwned: &falseVal},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := managedblockchain.NewInMemoryBackend()
			n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
			b.AddMemberInternal(testRegion, testAccountID, n.ID, "alice")
			b.AddMemberInternal(testRegion, testAccountID, n.ID, "bob")

			members, err := b.ListMembers(n.ID, tt.filter)
			require.NoError(t, err)
			assert.Len(t, members, tt.wantCount)
		})
	}
}

// TestHandler_MemberSummaryIsOwned verifies IsOwned field appears in ListMembers response.
func TestHandler_MemberSummaryIsOwned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantOwned bool
	}{
		{
			name:      "seeded member is owned",
			wantOwned: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := managedblockchain.NewInMemoryBackend()
			n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
			b.AddMemberInternal(testRegion, testAccountID, n.ID, "owned-member")

			h := managedblockchain.NewHandler(b)
			h.AccountID = testAccountID
			h.DefaultRegion = testRegion

			rec := doRequest(t, h, http.MethodGet, "/networks/"+n.ID+"/members", nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			members := resp["Members"].([]any)
			require.Len(t, members, 1)

			member := members[0].(map[string]any)
			isOwned, ok := member["IsOwned"]
			assert.True(t, ok, "IsOwned field should be present")
			assert.Equal(t, tt.wantOwned, isOwned)
		})
	}
}

// TestHandler_MemberObjectIsOwned verifies IsOwned field in GetMember response.
func TestHandler_MemberObjectIsOwned(t *testing.T) {
	t.Parallel()

	b := managedblockchain.NewInMemoryBackend()
	n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
	m := b.AddMemberInternal(testRegion, testAccountID, n.ID, "owned-member")

	h := managedblockchain.NewHandler(b)
	h.AccountID = testAccountID
	h.DefaultRegion = testRegion

	rec := doRequest(t, h, http.MethodGet, "/networks/"+n.ID+"/members/"+m.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	member := resp["Member"].(map[string]any)
	isOwned, ok := member["IsOwned"]
	assert.True(t, ok, "IsOwned field should be present in GetMember response")
	assert.Equal(t, true, isOwned)
}

// TestHandler_UpdateMemberLogPublishingConfig verifies log config is stored and returned.
func TestHandler_UpdateMemberLogPublishingConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		logEnabled bool
	}{
		{
			name:       "enable CA log → stored and returned in GetMember",
			logEnabled: true,
		},
		{
			name:       "disable CA log → stored and returned",
			logEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := managedblockchain.NewInMemoryBackend()
			n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
			m := b.AddMemberInternal(testRegion, testAccountID, n.ID, "m1")

			h := managedblockchain.NewHandler(b)
			h.AccountID = testAccountID
			h.DefaultRegion = testRegion

			// UpdateMember with log config
			patchPath := fmt.Sprintf("/networks/%s/members/%s", n.ID, m.ID)
			rec := doRequest(t, h, http.MethodPatch, patchPath, map[string]any{
				"LogPublishingConfiguration": map[string]any{
					"Fabric": map[string]any{
						"CaLogs": map[string]any{
							"Cloudwatch": map[string]any{
								"Enabled": tt.logEnabled,
							},
						},
					},
				},
			})
			require.Equal(t, http.StatusNoContent, rec.Code)

			// GetMember and verify log config
			rec2 := doRequest(t, h, http.MethodGet, patchPath, nil)
			require.Equal(t, http.StatusOK, rec2.Code)

			var getResp map[string]any
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &getResp))

			member := getResp["Member"].(map[string]any)
			logConfig, ok := member["LogPublishingConfiguration"]
			assert.True(t, ok, "LogPublishingConfiguration should be present after update")

			logConfigMap := logConfig.(map[string]any)
			fabric := logConfigMap["Fabric"].(map[string]any)
			caLogs := fabric["CaLogs"].(map[string]any)
			cw := caLogs["Cloudwatch"].(map[string]any)
			assert.Equal(t, tt.logEnabled, cw["Enabled"])
		})
	}
}

func TestHandler_UpdateMember(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		networkID  string
		memberID   string
		wantStatus int
	}{
		{
			name:       "happy path returns 204",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "network not found returns 404",
			networkID:  "nonexistent-network",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "member not found returns 404",
			memberID:   "nonexistent-member",
			wantStatus: http.StatusNotFound,
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

			networkID := net.ID
			memberID := mem.ID

			if tt.networkID != "" {
				networkID = tt.networkID
			}

			if tt.memberID != "" {
				memberID = tt.memberID
			}

			path := fmt.Sprintf("/networks/%s/members/%s", networkID, memberID)
			rec := doRequest(t, h, http.MethodPatch, path, map[string]any{})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_CreateMemberWithTags verifies tags supplied via
// MemberConfiguration.Tags -- the real CreateMemberInput's only Tags field
// (confirmed against aws-sdk-go-v2 managedblockchain@v1.34.4
// api_op_CreateMember.go, which has no top-level Tags at all) -- are
// persisted on CreateMember (gopherstack-2mwl).
func TestHandler_CreateMemberWithTags(t *testing.T) {
	t.Parallel()

	b := managedblockchain.NewInMemoryBackend()
	n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
	h := managedblockchain.NewHandler(b)
	h.AccountID = testAccountID
	h.DefaultRegion = testRegion

	memberConfig := testMemberConfiguration("tagged-member")
	memberConfig["Tags"] = map[string]string{"role": "validator"}

	invitationID := createTestInvitation(t, b, n.ID, "net1")

	rec := doRequest(t, h, http.MethodPost, "/networks/"+n.ID+"/members", map[string]any{
		"InvitationId":        invitationID,
		"ClientRequestToken":  "tok-tagged-member",
		"MemberConfiguration": memberConfig,
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	memberID, _ := resp["MemberId"].(string)
	require.NotEmpty(t, memberID)

	// GetMember and verify tags
	rec2 := doRequest(t, h, http.MethodGet, "/networks/"+n.ID+"/members/"+memberID, nil)
	require.Equal(t, http.StatusOK, rec2.Code)

	var memResp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &memResp))

	member := memResp["Member"].(map[string]any)
	tags := member["Tags"].(map[string]any)
	assert.Equal(t, "validator", tags["role"])

	// ListTagsForResource must see the same tags (gopherstack-2mwl: creation
	// tags reaching the resource's own struct is not sufficient -- they must
	// also reach the shared tag store).
	memberArn, _ := member["Arn"].(string)
	require.NotEmpty(t, memberArn)

	listedTags, err := b.ListTagsForResource(memberArn)
	require.NoError(t, err)
	assert.Equal(t, "validator", listedTags["role"])
}

// TestHandler_DeleteMemberCascadeNodes verifies nodes are deleted with the member.
func TestHandler_DeleteMemberCascadeNodes(t *testing.T) {
	t.Parallel()

	b := managedblockchain.NewInMemoryBackend()
	n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
	m := b.AddMemberInternal(testRegion, testAccountID, n.ID, "member1")
	b.AddNodeInternal(testRegion, testAccountID, n.ID, m.ID, "bc.t3.small.ethereum")
	b.AddNodeInternal(testRegion, testAccountID, n.ID, m.ID, "bc.t3.large.ethereum")

	require.Equal(t, 2, managedblockchain.NodeCount(b))

	h := managedblockchain.NewHandler(b)
	h.AccountID = testAccountID
	h.DefaultRegion = testRegion

	rec := doRequest(t, h, http.MethodDelete, "/networks/"+n.ID+"/members/"+m.ID, nil)
	require.Equal(t, http.StatusNoContent, rec.Code)

	// Nodes should be gone too
	assert.Equal(t, 0, managedblockchain.NodeCount(b))
}

// TestInMemoryBackend_DeleteMemberCascadeARNIndex verifies node ARNs are removed from the index.
func TestInMemoryBackend_DeleteMemberCascadeARNIndex(t *testing.T) {
	t.Parallel()

	b := managedblockchain.NewInMemoryBackend()
	n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
	m := b.AddMemberInternal(testRegion, testAccountID, n.ID, "member1")
	node := b.AddNodeInternal(testRegion, testAccountID, n.ID, m.ID, "bc.t3.small.ethereum")

	// ARN index should have: network + member + node = 3
	require.Equal(t, 3, managedblockchain.ARNIndexSize(b))

	err := b.DeleteMember(n.ID, m.ID)
	require.NoError(t, err)

	// member1 was the network's last (and only) member, so real AWS deletes the
	// network too ("If MemberId is the last member in a network specified by the
	// last Amazon Web Services account, the network is deleted also." --
	// aws-sdk-go-v2 managedblockchain api_op_DeleteMember.go doc comment,
	// v1.34.4): network + member + node are all gone.
	assert.Equal(t, 0, managedblockchain.ARNIndexSize(b))

	// Tagging the deleted node's ARN should fail
	err = b.TagResource(node.Arn, map[string]string{"k": "v"})
	require.Error(t, err)
}

// TestInMemoryBackend_DeleteMemberNetworkCascade verifies real AWS's documented
// DeleteMember side effect: "If MemberId is the last member in a network
// specified by the last Amazon Web Services account, the network is deleted
// also." (aws-sdk-go-v2 managedblockchain api_op_DeleteMember.go doc comment,
// v1.34.4).
func TestInMemoryBackend_DeleteMemberNetworkCascade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		lastMember    bool
		wantNetworkOK bool
	}{
		{
			name:          "deleting the network's last member deletes the network",
			lastMember:    true,
			wantNetworkOK: false,
		},
		{
			name:          "deleting one of several members leaves the network intact",
			lastMember:    false,
			wantNetworkOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := managedblockchain.NewInMemoryBackend()
			n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
			m := b.AddMemberInternal(testRegion, testAccountID, n.ID, "member1")

			if !tt.lastMember {
				b.AddMemberInternal(testRegion, testAccountID, n.ID, "member2")
			}

			require.NoError(t, b.DeleteMember(n.ID, m.ID))

			_, err := b.GetNetwork(n.ID)

			if tt.wantNetworkOK {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, managedblockchain.ErrNetworkNotFound)
			}
		})
	}
}
