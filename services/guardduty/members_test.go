package guardduty_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/guardduty"
	organizationsbackend "github.com/blackbirdworks/gopherstack/services/organizations"
)

func TestMembers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, h *guardduty.Handler)
		name string
	}{
		{
			name: "create_list_delete",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()

				id := createTestDetector(t, h)

				// CreateMembers
				rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/member", map[string]any{
					"accountDetails": []map[string]any{
						{"accountId": "111111111111", "email": "a@example.com"},
						{"accountId": "222222222222", "email": "b@example.com"},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				var createResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
				assert.Empty(t, createResp["unprocessedAccounts"])

				// ListMembers
				rec = doRequest(t, h, http.MethodGet, "/detector/"+id+"/member", nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var listResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
				members, _ := listResp["members"].([]any)
				assert.Len(t, members, 2)

				// GetMembers
				rec = doRequest(t, h, http.MethodPost, "/detector/"+id+"/member/get", map[string]any{
					"accountIds": []string{"111111111111"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				var getResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
				got, _ := getResp["members"].([]any)
				assert.Len(t, got, 1)

				// DeleteMembers
				rec = doRequest(t, h, http.MethodPost, "/detector/"+id+"/member/delete", map[string]any{
					"accountIds": []string{"111111111111", "222222222222"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				// ListMembers after delete
				rec = doRequest(t, h, http.MethodGet, "/detector/"+id+"/member", nil)
				var listAfter map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listAfter))
				membersAfter, _ := listAfter["members"].([]any)
				assert.Empty(t, membersAfter)
			},
		},
		{
			// ListMembersInput.OnlyAssociated is a real query parameter (see
			// aws-sdk-go-v2/service/guardduty serializers.go:
			// encoder.SetQuery("onlyAssociated")) that must filter out
			// not-yet-invited members, not be silently ignored.
			name: "list_only_associated_filters_unassociated_members",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()

				id := createTestDetector(t, h)

				doRequest(t, h, http.MethodPost, "/detector/"+id+"/member", map[string]any{
					"accountDetails": []map[string]any{
						{"accountId": "444444444444", "email": "d@example.com"},
						{"accountId": "555555555555", "email": "e@example.com"},
					},
				})

				doRequest(t, h, http.MethodPost, "/detector/"+id+"/member/start", map[string]any{
					"accountIds": []string{"444444444444"},
				})

				rec := doRequest(t, h, http.MethodGet, "/detector/"+id+"/member?onlyAssociated=true", nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				members, _ := resp["members"].([]any)
				require.Len(t, members, 1)

				first, _ := members[0].(map[string]any)
				assert.Equal(t, "444444444444", first["accountId"])
			},
		},
		{
			name: "invite_and_monitoring",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()

				id := createTestDetector(t, h)

				doRequest(t, h, http.MethodPost, "/detector/"+id+"/member", map[string]any{
					"accountDetails": []map[string]any{
						{"accountId": "333333333333", "email": "c@example.com"},
					},
				})

				// InviteMembers
				rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/member/invite", map[string]any{
					"accountIds": []string{"333333333333"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				// StartMonitoringMembers
				rec = doRequest(t, h, http.MethodPost, "/detector/"+id+"/member/start", map[string]any{
					"accountIds": []string{"333333333333"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				// StopMonitoringMembers
				rec = doRequest(t, h, http.MethodPost, "/detector/"+id+"/member/stop", map[string]any{
					"accountIds": []string{"333333333333"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "disassociate_members",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()

				id := createTestDetector(t, h)

				doRequest(t, h, http.MethodPost, "/detector/"+id+"/member", map[string]any{
					"accountDetails": []map[string]any{
						{"accountId": "444444444444", "email": "d@example.com"},
					},
				})

				rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/member/disassociate", map[string]any{
					"accountIds": []string{"444444444444"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "member_detectors",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()

				id := createTestDetector(t, h)

				doRequest(t, h, http.MethodPost, "/detector/"+id+"/member", map[string]any{
					"accountDetails": []map[string]any{
						{"accountId": "555555555555", "email": "e@example.com"},
					},
				})

				// GetMemberDetectors
				rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/member/detector/get", map[string]any{
					"accountIds": []string{"555555555555"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				// UpdateMemberDetectors
				rec = doRequest(t, h, http.MethodPost, "/detector/"+id+"/member/detector/update", map[string]any{
					"accountIds": []string{"555555555555"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			tt.fn(t, h)
		})
	}
}

// TestMemberBatchOps_UnknownDetector_NotFound locks that every
// detector-scoped member batch op rejects an unknown DetectorId with 404,
// matching ListMembers/CreateMembers (which already validated this) instead
// of silently returning 200 with every requested account marked
// unprocessed -- an unknown detector is a different failure than an unknown
// member, and conflating the two let a bogus detector ID report success.
func TestMemberBatchOps_UnknownDetector_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		method string
		path   string
	}{
		{name: "get_members", method: http.MethodPost, path: "/detector/no-such-detector/member/get",
			body: map[string]any{"accountIds": []string{"111111111111"}}},
		{name: "delete_members", method: http.MethodPost, path: "/detector/no-such-detector/member/delete",
			body: map[string]any{"accountIds": []string{"111111111111"}}},
		{name: "invite_members", method: http.MethodPost, path: "/detector/no-such-detector/member/invite",
			body: map[string]any{"accountIds": []string{"111111111111"}}},
		{name: "start_monitoring_members", method: http.MethodPost, path: "/detector/no-such-detector/member/start",
			body: map[string]any{"accountIds": []string{"111111111111"}}},
		{name: "stop_monitoring_members", method: http.MethodPost, path: "/detector/no-such-detector/member/stop",
			body: map[string]any{"accountIds": []string{"111111111111"}}},
		{name: "disassociate_members", method: http.MethodPost, path: "/detector/no-such-detector/member/disassociate",
			body: map[string]any{"accountIds": []string{"111111111111"}}},
		{name: "get_member_detectors", method: http.MethodPost, path: "/detector/no-such-detector/member/detector/get",
			body: map[string]any{"accountIds": []string{"111111111111"}}},
		{
			name: "update_member_detectors", method: http.MethodPost,
			path: "/detector/no-such-detector/member/detector/update",
			body: map[string]any{"accountIds": []string{"111111111111"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.method, tt.path, tt.body)
			assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		})
	}
}

// TestMemberOps_AutoEnableOrganizationMembersAll_Rejected proves
// DeleteMembers/DisassociateMembers/StopMonitoringMembers each reject with
// BadRequestException when the detector's autoEnableOrganizationMembers is
// ALL and the requested account is still in the AWS Organization, matching
// the AWS doc text for all three operations ("With
// autoEnableOrganizationMembers configuration for your organization set to
// ALL, you'll receive an error..."). Previously all three ignored this org
// setting entirely and always returned 200 (gopherstack-krb1).
//
// Updated for gopherstack-uu0n: the ALL guard used to reject unconditionally
// regardless of the account's actual org membership, which over-rejected an
// account that had already left the organization (see
// TestMemberOps_AutoEnableOrganizationMembersAll_AllowedAfterAccountLeavesOrg
// in cross_service_test.go for that case). This test now wires a real
// Organizations backend and keeps the account in it, so it continues to
// prove the case the AWS doc text actually describes -- rejection while
// still a member -- instead of a case the fix now correctly allows.
func TestMemberOps_AutoEnableOrganizationMembersAll_Rejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "delete_members", path: "/member/delete"},
		{name: "disassociate_members", path: "/member/disassociate"},
		{name: "stop_monitoring_members", path: "/member/stop"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			orgBk := organizationsbackend.NewInMemoryBackend("123456789012", "us-east-1")
			orgHandler := organizationsbackend.NewHandler(orgBk)

			_, _, err := orgBk.CreateOrganization("ALL")
			require.NoError(t, err)

			status, err := orgBk.CreateAccount(
				"member", "member@example.com", "OrganizationAccountAccessRole", "ALLOW", nil,
			)
			require.NoError(t, err)
			accountID := status.AccountID

			backend := guardduty.NewInMemoryBackend("123456789012", "us-east-1")
			backend.SetAppConfig(&fakeSiblingServices{orgHandler: orgHandler})
			h := guardduty.NewHandler(backend)

			id := createTestDetector(t, h)

			doRequest(t, h, http.MethodPost, "/detector/"+id+"/member", map[string]any{
				"accountDetails": []map[string]any{
					{"accountId": accountID, "email": "a@example.com"},
				},
			})

			rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/admin", map[string]any{
				"autoEnableOrganizationMembers": "ALL",
			})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			rec = doRequest(t, h, http.MethodPost, "/detector/"+id+tt.path, map[string]any{
				"accountIds": []string{accountID},
			})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

			var errOut map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errOut))
			assert.Equal(t, "BadRequestException", errOut["__type"])
		})
	}
}

// TestMemberOps_AutoEnableOrganizationMembers_NonALL_Allowed proves the
// autoEnableOrganizationMembers guard only fires for ALL: unset, NEW, and
// NONE must all still let DeleteMembers/DisassociateMembers/
// StopMonitoringMembers succeed, so the fix for gopherstack-krb1 does not
// reject more than the real API does.
func TestMemberOps_AutoEnableOrganizationMembers_NonALL_Allowed(t *testing.T) {
	t.Parallel()

	opPaths := []string{"/member/delete", "/member/disassociate", "/member/stop"}

	tests := []struct {
		name  string
		value string
	}{
		{name: "unset", value: ""},
		{name: "new", value: "NEW"},
		{name: "none", value: "NONE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, path := range opPaths {
				h := newTestHandler(t)
				id := createTestDetector(t, h)

				doRequest(t, h, http.MethodPost, "/detector/"+id+"/member", map[string]any{
					"accountDetails": []map[string]any{
						{"accountId": "111111111111", "email": "a@example.com"},
					},
				})

				if tt.value != "" {
					rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/admin", map[string]any{
						"autoEnableOrganizationMembers": tt.value,
					})
					require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				}

				rec := doRequest(t, h, http.MethodPost, "/detector/"+id+path, map[string]any{
					"accountIds": []string{"111111111111"},
				})
				assert.Equal(t, http.StatusOK, rec.Code, "path=%s value=%q body=%s", path, tt.value, rec.Body.String())
			}
		})
	}
}

func TestInvitations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, h *guardduty.Handler)
		name string
	}{
		{
			name: "count_and_list",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()

				// GetInvitationsCount
				rec := doRequest(t, h, http.MethodGet, "/invitation/count", nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var countResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &countResp))
				assert.NotNil(t, countResp["invitationsCount"])

				// ListInvitations
				rec = doRequest(t, h, http.MethodGet, "/invitation", nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var listResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
				assert.NotNil(t, listResp["invitations"])
			},
		},
		{
			name: "decline_and_delete",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()

				// DeclineInvitations
				rec := doRequest(t, h, http.MethodPost, "/invitation/decline", map[string]any{
					"accountIds": []string{"999999999999"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				// DeleteInvitations
				rec = doRequest(t, h, http.MethodPost, "/invitation/delete", map[string]any{
					"accountIds": []string{"999999999999"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "administrator_account",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()

				id := createTestDetector(t, h)

				// AcceptAdministratorInvitation
				rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/administrator", map[string]any{
					"administratorId": "777777777777",
					"invitationId":    "inv-001",
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				// GetAdministratorAccount
				rec = doRequest(t, h, http.MethodGet, "/detector/"+id+"/administrator", nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var adminResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &adminResp))
				assert.NotNil(t, adminResp["administrator"])

				// DisassociateFromAdministratorAccount
				rec = doRequest(t, h, http.MethodPost, "/detector/"+id+"/administrator/disassociate", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "master_account_legacy",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()

				id := createTestDetector(t, h)

				// AcceptInvitation (legacy master)
				rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/master", map[string]any{
					"masterId":     "888888888888",
					"invitationId": "inv-002",
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				// GetMasterAccount
				rec = doRequest(t, h, http.MethodGet, "/detector/"+id+"/master", nil)
				assert.Equal(t, http.StatusOK, rec.Code)

				var masterResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &masterResp))
				assert.NotNil(t, masterResp["master"])

				// DisassociateFromMasterAccount
				rec = doRequest(t, h, http.MethodPost, "/detector/"+id+"/master/disassociate", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			tt.fn(t, h)
		})
	}
}
