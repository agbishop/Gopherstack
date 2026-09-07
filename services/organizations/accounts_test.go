package organizations_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// TestBackend_AccountLifecycle tests account creation and listing.
func TestBackend_AccountLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		accountName string
		email       string
		wantErr     bool
		noOrg       bool
	}{
		{
			name:        "create_and_list",
			accountName: "dev-account",
			email:       "dev@example.com",
		},
		{
			name:        "no_org_returns_error",
			accountName: "dev-account",
			email:       "dev@example.com",
			noOrg:       true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if !tt.noOrg {
				_, _, err := b.CreateOrganization("ALL")
				require.NoError(t, err)
			}

			status, err := b.CreateAccount(tt.accountName, tt.email, "", "", nil)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, status)
			assert.NotEmpty(t, status.AccountID)
			assert.Equal(t, "SUCCEEDED", status.State)

			// DescribeAccount.
			acct, err := b.DescribeAccount(status.AccountID)
			require.NoError(t, err)
			assert.Equal(t, tt.accountName, acct.Name)
			assert.Equal(t, tt.email, acct.Email)

			// ListAccounts.
			accts, err := b.ListAccounts()
			require.NoError(t, err)
			assert.NotEmpty(t, accts)

			found := false

			for _, a := range accts {
				if a.ID == status.AccountID {
					found = true

					break
				}
			}

			assert.True(t, found, "created account should appear in ListAccounts")
		})
	}
}

// TestBackend_DescribeCreateAccountStatus tests the DescribeCreateAccountStatus operation.
func TestBackend_DescribeCreateAccountStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requestID string
		wantErr   bool
	}{
		{
			name:    "found",
			wantErr: false,
		},
		{
			name:      "not_found",
			requestID: "car-doesnotexist",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			requestID := tt.requestID

			if requestID == "" {
				status, err := b.CreateAccount("test-account", "test@example.com", "", "", nil)
				require.NoError(t, err)
				requestID = status.ID
			}

			desc, err := b.DescribeCreateAccountStatus(requestID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, desc.AccountID)
		})
	}
}

// TestBackend_RemoveAccountFromOrganization tests removing an account.
func TestBackend_RemoveAccountFromOrganization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		accountID string
		wantErr   bool
	}{
		{
			name:    "removes_account",
			wantErr: false,
		},
		{
			name:      "not_found",
			accountID: "999999999999",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			accountID := tt.accountID

			if accountID == "" {
				status, err := b.CreateAccount("test-account", "test@example.com", "", "", nil)
				require.NoError(t, err)
				accountID = status.AccountID
			}

			err := b.RemoveAccountFromOrganization(accountID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			// Account should not be in list any more.
			_, descErr := b.DescribeAccount(accountID)
			require.Error(t, descErr, "removed account should no longer be describable")
		})
	}
}

// TestBackend_RemoveAccountFromOrganization_MasterAccount verifies AWS
// behaviour: RemoveAccountFromOrganization's own per-op error switch
// (deserializers.go) models MasterCannotLeaveOrganizationException for the
// management account, not the generic InvalidInputException.
func TestBackend_RemoveAccountFromOrganization_MasterAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "management_account_cannot_be_removed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			org, err := b.DescribeOrganization()
			require.NoError(t, err)

			err = b.RemoveAccountFromOrganization(org.MasterAccountID)
			require.ErrorIs(t, err, organizations.ErrMasterCannotLeaveOrganization)
			require.NotErrorIs(t, err, organizations.ErrInvalidInput)
		})
	}
}

// TestBackend_RemoveAccountFromOrganization_FreesEmailForReuse verifies that
// removing an account clears its email from the duplicate-email index, so a
// new account can be created with the same email afterward. Previously the
// stale index entry caused CreateAccount to wrongly reject the same email
// with ErrInvalidInput even though no account was using it any more.
func TestBackend_RemoveAccountFromOrganization_FreesEmailForReuse(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	status, err := b.CreateAccount("reused-account", "reused@example.com", "", "", nil)
	require.NoError(t, err)

	require.NoError(t, b.RemoveAccountFromOrganization(status.AccountID))

	_, err = b.CreateAccount("reused-account-2", "reused@example.com", "", "", nil)
	require.NoError(t, err, "email should be free for reuse after account removal")
}

// TestBackend_MoveAccount tests moving an account between OUs.
func TestBackend_MoveAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name: "moves_account",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			// Create target OU.
			ou, err := b.CreateOrganizationalUnit(rootID, "target-ou", nil)
			require.NoError(t, err)

			// Create account (placed in root by default).
			status, err := b.CreateAccount("move-account", "move@example.com", "", "", nil)
			require.NoError(t, err)

			// Move account from root to OU.
			err = b.MoveAccount(status.AccountID, rootID, ou.ID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			// Verify account is in the target OU.
			parents, parentsErr := b.ListParents(status.AccountID)
			require.NoError(t, parentsErr)
			require.Len(t, parents, 1)
			assert.Equal(t, ou.ID, parents[0].ID)
		})
	}
}

// TestListAccountsWithInvalidEffectivePolicy_AllTypes tests
// all six policy types are accepted.
func TestListAccountsWithInvalidEffectivePolicy_AllTypes(t *testing.T) {
	t.Parallel()

	policyTypes := []string{
		"SERVICE_CONTROL_POLICY",
		"TAG_POLICY",
		"BACKUP_POLICY",
		"AISERVICES_OPT_OUT_POLICY",
		"CHATBOT_POLICY",
		"DECLARATIVE_POLICY_EC2",
	}

	for _, pt := range policyTypes {
		t.Run(pt, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			accounts, err := b.ListAccountsWithInvalidEffectivePolicy(pt)
			require.NoError(t, err)
			assert.Empty(t, accounts)
		})
	}
}

// TestCreateAccount_WithTags verifies tags are stored on created accounts.
func TestCreateAccount_WithTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tags     []organizations.Tag
		wantTags int
	}{
		{name: "no_tags", tags: nil, wantTags: 0},
		{name: "single_tag", tags: []organizations.Tag{{Key: "env", Value: "prod"}}, wantTags: 1},
		{
			name: "multi_tags",
			tags: []organizations.Tag{
				{Key: "env", Value: "prod"},
				{Key: "owner", Value: "team-a"},
				{Key: "cost-center", Value: "cc-123"},
			},
			wantTags: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			status, err := b.CreateAccount("tagged-account", "tagged@example.com", "", "", tt.tags)
			require.NoError(t, err)

			tags, err := b.ListTagsForResource(status.AccountID)
			require.NoError(t, err)
			assert.Len(t, tags, tt.wantTags)
		})
	}
}

// TestCreateAccount_GovCloudIDFormat verifies GovCloud account ID format.
func TestCreateAccount_GovCloudIDFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		accountName string
		email       string
	}{
		{
			name:        "govcloud_id_twelve_digits",
			accountName: "gov-account",
			email:       "gov@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			status, err := b.CreateGovCloudAccount(tt.accountName, tt.email, "", "", nil)
			require.NoError(t, err)
			assert.NotEmpty(t, status.GovCloudAccountID)
			assert.Len(t, status.GovCloudAccountID, 12, "GovCloud ID must be 12 digits")
			// GovCloud IDs are offset by 1_000_000_000 from the commercial ID counter.
			// Both commercial and GovCloud IDs are returned in the same status.
			assert.NotEqual(t, status.AccountID, status.GovCloudAccountID,
				"GovCloud ID must differ from commercial ID")
		})
	}
}

// TestMoveAccount_Scenarios tests moving accounts between OUs and roots.
func TestMoveAccount_Scenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		moveTo      string // "ou", "root", "nested_ou"
		wantErr     bool
		wrongSource bool
	}{
		{name: "root_to_ou", moveTo: "ou"},
		{name: "ou_to_root", moveTo: "root"},
		{name: "root_to_nested_ou", moveTo: "nested_ou"},
		{name: "wrong_source_id_fails", moveTo: "ou", wrongSource: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			ouTop, err := b.CreateOrganizationalUnit(rootID, "top-ou", nil)
			require.NoError(t, err)

			ouNested, err := b.CreateOrganizationalUnit(ouTop.ID, "nested-ou", nil)
			require.NoError(t, err)

			status, err := b.CreateAccount("move-me", "move@example.com", "", "", nil)
			require.NoError(t, err)
			accountID := status.AccountID

			var destParentID string

			switch tt.moveTo {
			case "ou":
				destParentID = ouTop.ID
			case "root":
				// First move to OU then back to root.
				require.NoError(t, b.MoveAccount(accountID, rootID, ouTop.ID))
				destParentID = rootID
				rootID = ouTop.ID // source is now ouTop
			case "nested_ou":
				destParentID = ouNested.ID
			}

			sourceID := rootID
			if tt.wrongSource {
				sourceID = "r-wrong"
			}

			err = b.MoveAccount(accountID, sourceID, destParentID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			parents, err := b.ListParents(accountID)
			require.NoError(t, err)
			require.Len(t, parents, 1)
			assert.Equal(t, destParentID, parents[0].ID)
		})
	}
}

// TestMoveAccount_ErrorCodes verifies AWS behaviour: MoveAccount's own
// per-op error switch (deserializers.go) models SourceParentNotFoundException
// and DestinationParentNotFoundException, not the generic
// InvalidInputException.
func TestMoveAccount_ErrorCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		name      string
		badSource bool
		badDest   bool
	}{
		{name: "bad_source", badSource: true, wantErr: organizations.ErrSourceParentNotFound},
		{name: "bad_dest", badDest: true, wantErr: organizations.ErrDestinationParentNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, rootID := newOrgBackend(t)

			status, err := b.CreateAccount("move-me", "move-err@example.com", "", "", nil)
			require.NoError(t, err)
			accountID := status.AccountID

			sourceID := rootID
			if tt.badSource {
				sourceID = "r-doesnotexist"
			}

			destID := rootID
			if tt.badDest {
				destID = "ou-doesnotexist-00000000"
			}

			err = b.MoveAccount(accountID, sourceID, destID)
			require.ErrorIs(t, err, tt.wantErr)
			require.NotErrorIs(t, err, organizations.ErrInvalidInput)
		})
	}
}

// TestCloseAccount_Scenarios verifies close account behavior.
func TestCloseAccount_Scenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		isMgmt   bool
		notFound bool
		wantErr  bool
		wantSusp bool
	}{
		{name: "closes_member_account", wantSusp: true},
		{name: "management_account_fails", isMgmt: true, wantErr: true},
		{name: "not_found_fails", notFound: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			org, _, err := b.CreateOrganization("ALL")
			require.NoError(t, err)

			var targetID string
			switch {
			case tt.isMgmt:
				targetID = org.MasterAccountID
			case tt.notFound:
				targetID = "000000000000"
			default:
				createStatus, createErr := b.CreateAccount("close-me", "close@example.com", "", "", nil)
				require.NoError(t, createErr)
				targetID = createStatus.AccountID
			}

			err = b.CloseAccount(targetID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.wantSusp {
				acct, descErr := b.DescribeAccount(targetID)
				require.NoError(t, descErr)
				assert.Equal(t, "PENDING_CLOSURE", acct.Status)
			}
		})
	}
}

// TestRemoveAccount_RejectsWhileDelegatedAdmin verifies AWS behaviour:
// RemoveAccountFromOrganization's doc comment requires the target account to
// not be a delegated administrator for any service ("you must first change
// the delegated administrator account to another account"), backed by
// ConstraintViolationExceptionReasonCannotRemoveDelegatedAdministratorFromOrg
// (types/enums.go). Once deregistered, removal succeeds and leaves no ghost
// delegated-admin rows.
func TestRemoveAccount_RejectsWhileDelegatedAdmin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		services []string
	}{
		{name: "single_service", services: []string{"ssm.amazonaws.com"}},
		{
			name: "multi_service",
			services: []string{
				"ssm.amazonaws.com",
				"cloudtrail.amazonaws.com",
				"guardduty.amazonaws.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			status, err := b.CreateAccount("delegate", "delegate@example.com", "", "", nil)
			require.NoError(t, err)
			accountID := status.AccountID

			for _, svc := range tt.services {
				require.NoError(t, b.EnableAWSServiceAccess(svc))
				require.NoError(t, b.RegisterDelegatedAdministrator(accountID, svc))
			}

			err = b.RemoveAccountFromOrganization(accountID)
			require.ErrorIs(t, err, organizations.ErrCannotRemoveDelegatedAdministratorFromOrg)

			for _, svc := range tt.services {
				require.NoError(t, b.DeregisterDelegatedAdministrator(accountID, svc))
			}

			require.NoError(t, b.RemoveAccountFromOrganization(accountID))

			for _, svc := range tt.services {
				admins, listErr := b.ListDelegatedAdministrators(svc)
				require.NoError(t, listErr)
				for _, a := range admins {
					assert.NotEqual(t, accountID, a.AccountID,
						"removed account must not appear as delegated admin for %s", svc)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// OrganizationalUnit hierarchy
// ---------------------------------------------------------------------------

// TestListCreateAccountStatus_Filter tests filtering by state.
func TestListCreateAccountStatus_Filter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		states    []string
		wantCount int
	}{
		{name: "no_filter_all", states: nil, wantCount: 3},
		{name: "filter_succeeded", states: []string{"SUCCEEDED"}, wantCount: 3},
		{name: "filter_in_progress", states: []string{"IN_PROGRESS"}, wantCount: 0},
		{name: "filter_failed", states: []string{"FAILED"}, wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			for i := range 3 {
				_, err := b.CreateAccount(
					"acct-"+string(rune('a'+i)),
					"acct"+string(rune('a'+i))+"@example.com",
					"", "",
					nil,
				)
				require.NoError(t, err)
			}

			statuses, err := b.ListCreateAccountStatus(tt.states)
			require.NoError(t, err)
			assert.Len(t, statuses, tt.wantCount)
		})
	}
}

// TestCloseAccount_PendingClosure verifies CloseAccount sets status to PENDING_CLOSURE.
func TestCloseAccount_PendingClosure(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	status, err := b.CreateAccount("close-me", "close@example.com", "", "", nil)
	require.NoError(t, err)

	err = b.CloseAccount(status.AccountID)
	require.NoError(t, err)

	acct, err := b.DescribeAccount(status.AccountID)
	require.NoError(t, err)
	assert.Equal(t, "PENDING_CLOSURE", acct.Status)
}

// TestCloseAccount_DoubleCloseRejected verifies that closing an already-closed account fails.
func TestCloseAccount_DoubleCloseRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupStatus string // "PENDING_CLOSURE" or "SUSPENDED" (via direct internal method)
		wantErr     bool
	}{
		{name: "double_pending_closure", setupStatus: "PENDING_CLOSURE", wantErr: true},
		{name: "suspended_also_rejected", setupStatus: "SUSPENDED", wantErr: true},
		{name: "active_succeeds", setupStatus: "", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			status, err := b.CreateAccount("close-me", "close@example.com", "", "", nil)
			require.NoError(t, err)
			accountID := status.AccountID

			switch tt.setupStatus {
			case "PENDING_CLOSURE":
				// Close once to get to PENDING_CLOSURE.
				err = b.CloseAccount(accountID)
				require.NoError(t, err)
			case "SUSPENDED":
				// Seed a SUSPENDED account directly.
				b.AddAccountInternal(&organizations.Account{
					ID:     "999999999999",
					ARN:    "arn:aws:organizations::123456789012:account/o-test/999999999999",
					Name:   "suspended",
					Email:  "suspended@example.com",
					Status: "SUSPENDED",
				})
				accountID = "999999999999"
			}

			err = b.CloseAccount(accountID)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateAccount_EmailUniqueness verifies that duplicate email addresses are rejected.
func TestCreateAccount_EmailUniqueness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		email1  string
		email2  string
		wantErr bool
	}{
		{
			name:    "duplicate_email_rejected",
			email1:  "same@example.com",
			email2:  "same@example.com",
			wantErr: true,
		},
		{
			name:    "unique_emails_ok",
			email1:  "first@example.com",
			email2:  "second@example.com",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			_, err := b.CreateAccount("Account1", tt.email1, "", "", nil)
			require.NoError(t, err, "first account creation should succeed")

			_, err = b.CreateAccount("Account2", tt.email2, "", "", nil)
			if tt.wantErr {
				require.Error(t, err, "duplicate email should fail")
			} else {
				require.NoError(t, err, "unique email should succeed")
			}
		})
	}
}

// TestCreateAccount_EmailUniqueness_AfterReset verifies that Reset clears email index.
func TestCreateAccount_EmailUniqueness_AfterReset(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	_, _, err := b.CreateOrganization("ALL")
	require.NoError(t, err)

	_, err = b.CreateAccount("Account1", "reuse@example.com", "", "", nil)
	require.NoError(t, err)

	b.Reset()

	_, _, err = b.CreateOrganization("ALL")
	require.NoError(t, err)

	// After reset, the same email should be usable again.
	_, err = b.CreateAccount("Account2", "reuse@example.com", "", "", nil)
	require.NoError(t, err, "email should be available after Reset()")
}

// ---------------------------------------------------------------------------
// Item 9: Handshake ARN contains lowercase action
// ---------------------------------------------------------------------------

// TestRemoveAccount_CleansPolicyTargets verifies that removing an account
// also removes it from every attached policy's target list (no dangling references).
func TestRemoveAccount_CleansPolicyTargets(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	status, err := b.CreateAccount("to-remove", "remove@example.com", "", "", nil)
	require.NoError(t, err)
	accountID := status.AccountID

	// Create a policy and attach it to the account.
	p, err := b.CreatePolicy("test-scp", "", `{"Version":"2012-10-17"}`, "SERVICE_CONTROL_POLICY", nil)
	require.NoError(t, err)
	require.NoError(t, b.AttachPolicy(p.PolicySummary.ID, accountID))

	// Verify the account appears in ListTargetsForPolicy before removal.
	targets, err := b.ListTargetsForPolicy(p.PolicySummary.ID)
	require.NoError(t, err)

	found := false
	for _, tgt := range targets {
		if tgt.TargetID == accountID {
			found = true

			break
		}
	}
	assert.True(t, found, "account should appear in ListTargetsForPolicy before removal")

	// Remove the account.
	require.NoError(t, b.RemoveAccountFromOrganization(accountID))

	// Verify the account no longer appears in ListTargetsForPolicy.
	targets, err = b.ListTargetsForPolicy(p.PolicySummary.ID)
	require.NoError(t, err)

	for _, tgt := range targets {
		assert.NotEqual(t, accountID, tgt.TargetID,
			"removed account must not appear in ListTargetsForPolicy")
	}
}

// TestRemoveAccount_InvitedCreatesLeaveHandshake verifies that removing
// an INVITED account generates a LEAVE_ORGANIZATION handshake record.
func TestRemoveAccount_InvitedCreatesLeaveHandshake(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	// Invite an account (this creates an INVITED account after accepting).
	h, err := b.InviteAccountToOrganization(organizations.HandshakeParty{
		ID:   "999999999999",
		Type: "ACCOUNT",
	}, "", nil)
	require.NoError(t, err)

	_, err = b.AcceptHandshake(h.ID)
	require.NoError(t, err)

	// Verify the account is now in the org.
	accts, err := b.ListAccounts()
	require.NoError(t, err)

	var invitedID string
	for _, a := range accts {
		if a.ID == "999999999999" {
			invitedID = a.ID

			break
		}
	}
	require.NotEmpty(t, invitedID, "invited account should be present after AcceptHandshake")

	// Count handshakes before removal.
	countBefore := organizations.HandshakeCount(b)

	// Remove the invited account.
	require.NoError(t, b.RemoveAccountFromOrganization(invitedID))

	// A LEAVE_ORGANIZATION handshake should have been created.
	countAfter := organizations.HandshakeCount(b)
	assert.Greater(t, countAfter, countBefore,
		"removing an INVITED account should create a LEAVE_ORGANIZATION handshake")

	// Verify we can list it.
	handshakes, err := b.ListHandshakesForOrganization("")
	require.NoError(t, err)

	found := false
	for _, hs := range handshakes {
		if hs.Action == "LEAVE_ORGANIZATION" {
			found = true

			break
		}
	}
	assert.True(t, found, "LEAVE_ORGANIZATION handshake should appear in ListHandshakesForOrganization")
}

// TestRemoveAccount_CreatedNoLeaveHandshake verifies that removing
// a CREATED account does NOT generate a LEAVE_ORGANIZATION handshake.
func TestRemoveAccount_CreatedNoLeaveHandshake(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	// CreateAccount produces a CREATED account.
	status, err := b.CreateAccount("created-acct", "created@example.com", "", "", nil)
	require.NoError(t, err)
	accountID := status.AccountID

	countBefore := organizations.HandshakeCount(b)

	require.NoError(t, b.RemoveAccountFromOrganization(accountID))

	countAfter := organizations.HandshakeCount(b)
	assert.Equal(t, countBefore, countAfter,
		"removing a CREATED account must not create any handshake")
}

// ---------------------------------------------------------------------------
// Item 19: DescribeEffectivePolicy walks full hierarchy chain
// ---------------------------------------------------------------------------

// TestCreateAccount_NoGovCloudID verifies that CreateAccount followed by
// DescribeCreateAccountStatus returns a payload without GovCloudAccountId.
func TestCreateAccount_NoGovCloudID(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	status, err := b.CreateAccount("commercial-acct", "commercial@example.com", "", "", nil)
	require.NoError(t, err)

	described, err := b.DescribeCreateAccountStatus(status.ID)
	require.NoError(t, err)
	assert.Empty(t, described.GovCloudAccountID,
		"CreateAccount DescribeCreateAccountStatus must not include GovCloudAccountId")
}

func TestBackend_CloseAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		accountID string
		wantErr   bool
	}{
		{
			name:    "closes_account",
			wantErr: false,
		},
		{
			name:      "not_found",
			accountID: "999999999999",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			accountID := tt.accountID

			if accountID == "" {
				status, err := b.CreateAccount("close-test", "close@example.com", "", "", nil)
				require.NoError(t, err)
				accountID = status.AccountID
			}

			err := b.CloseAccount(accountID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			// Account should now be PENDING_CLOSURE.
			acct, descErr := b.DescribeAccount(accountID)
			require.NoError(t, descErr)
			assert.Equal(t, "PENDING_CLOSURE", acct.Status)
		})
	}
}

// TestBackend_CreateGovCloudAccount tests the CreateGovCloudAccount backend method.
func TestBackend_CreateGovCloudAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		acctName string
		email    string
		noOrg    bool
		wantErr  bool
	}{
		{
			name:     "creates_govcloud_account",
			acctName: "govtest",
			email:    "gov@example.com",
		},
		{
			name:     "no_org_returns_error",
			acctName: "govtest",
			email:    "gov@example.com",
			noOrg:    true,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if !tt.noOrg {
				_, _, err := b.CreateOrganization("ALL")
				require.NoError(t, err)
			}

			status, err := b.CreateGovCloudAccount(tt.acctName, tt.email, "", "", nil)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, status)
			assert.NotEmpty(t, status.AccountID)
			assert.NotEmpty(t, status.GovCloudAccountID)
			assert.Equal(t, "SUCCEEDED", status.State)
			assert.Equal(t, tt.acctName, status.AccountName)
		})
	}
}

// TestListAccounts_Sorted verifies deterministic sorted order by account ID.
func TestListAccounts_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		extraAcct int
		wantCount int
	}{
		{name: "one_account", extraAcct: 0, wantCount: 1},
		{name: "three_accounts", extraAcct: 2, wantCount: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			for i := range tt.extraAcct {
				_, createErr := b.CreateAccount("acct", fmt.Sprintf("a%d@example.com", i), "", "", nil)
				require.NoError(t, createErr)
			}

			accts, err := b.ListAccounts()
			require.NoError(t, err)
			require.Len(t, accts, tt.wantCount)

			for i := 1; i < len(accts); i++ {
				assert.LessOrEqual(t, accts[i-1].ID, accts[i].ID, "accounts should be sorted by ID")
			}
		})
	}
}

// TestBackend_AddAccountInternal verifies seed helper.
func TestBackend_AddAccountInternal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		accountID string
		wantCount int
	}{
		{name: "adds_account", accountID: "111111111111", wantCount: 2}, // management + seed
		{name: "replaces_if_same_id", accountID: "111111111111", wantCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			b.AddAccountInternal(&organizations.Account{
				ID:     tt.accountID,
				Name:   "seed-account",
				Status: "ACTIVE",
			})

			assert.Equal(t, tt.wantCount, organizations.AccountCount(b))
		})
	}
}
