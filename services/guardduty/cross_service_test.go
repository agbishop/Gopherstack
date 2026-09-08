package guardduty_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/guardduty"
	organizationsbackend "github.com/blackbirdworks/gopherstack/services/organizations"
)

// fakeSiblingServices structurally satisfies guardduty's unexported
// siblingServices interface (matched by SetAppConfig's type assertion),
// mirroring how the real *CLI wires GetOrganizationsHandler.
type fakeSiblingServices struct {
	orgHandler service.Registerable
}

func (f *fakeSiblingServices) GetOrganizationsHandler() service.Registerable { return f.orgHandler }

// TestMemberOps_AutoEnableOrganizationMembersAll_AllowedAfterAccountLeavesOrg
// proves DisassociateMembers/DeleteMembers/StopMonitoringMembers reject an
// account only while it is still in the AWS Organization: once
// organizations.RemoveAccountFromOrganization removes it, the same call
// with the same detector-level autoEnableOrganizationMembers=ALL config
// must succeed (gopherstack-uu0n). Real DisassociateMembers' doc text:
// "you'll receive an error if you attempt to disassociate a member account
// before removing them from your organization" -- the error is conditioned
// on org membership, not merely on the ALL setting.
func TestMemberOps_AutoEnableOrganizationMembersAll_AllowedAfterAccountLeavesOrg(t *testing.T) {
	t.Parallel()

	const accountID = "111111111111"

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
				accountID, accountID+"@example.com", "OrganizationAccountAccessRole", "ALLOW", nil,
			)
			require.NoError(t, err)

			// CreateAccount mints a synthetic account ID rather than
			// accepting the caller's -- adopt whatever it actually
			// created so DescribeAccount/RemoveAccountFromOrganization
			// address the same account this test's GuardDuty member uses.
			realAccountID := status.AccountID

			backend := guardduty.NewInMemoryBackend("123456789012", "us-east-1")
			backend.SetAppConfig(&fakeSiblingServices{orgHandler: orgHandler})
			h := guardduty.NewHandler(backend)

			id := createTestDetector(t, h)

			doRequest(t, h, http.MethodPost, "/detector/"+id+"/member", map[string]any{
				"accountDetails": []map[string]any{
					{"accountId": realAccountID, "email": "a@example.com"},
				},
			})

			rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/admin", map[string]any{
				"autoEnableOrganizationMembers": "ALL",
			})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			// Still in the org: must reject.
			rec = doRequest(t, h, http.MethodPost, "/detector/"+id+tt.path, map[string]any{
				"accountIds": []string{realAccountID},
			})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

			require.NoError(t, orgBk.RemoveAccountFromOrganization(realAccountID))

			// Left the org: must now succeed.
			rec = doRequest(t, h, http.MethodPost, "/detector/"+id+tt.path, map[string]any{
				"accountIds": []string{realAccountID},
			})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			require.Empty(t, out["unprocessedAccounts"], "account should have been processed, not left unprocessed")
		})
	}
}
