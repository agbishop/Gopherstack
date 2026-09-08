package organizations_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/organizations"
)

// TestEnableAllFeatures_Backend tests the backend directly.
func TestEnableAllFeatures_Backend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantState string
		hasOrg    bool
		wantErr   bool
	}{
		{name: "creates_handshake", hasOrg: true, wantState: "OPEN"},
		{name: "no_org", hasOrg: false, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.hasOrg {
				_, _, err := b.CreateOrganization("ALL")
				require.NoError(t, err)
			}

			hs, err := b.EnableAllFeatures()

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, hs)
			assert.Equal(t, "ENABLE_ALL_FEATURES", hs.Action)
			assert.Equal(t, tt.wantState, hs.State)
			assert.NotEmpty(t, hs.ID)
			assert.NotEmpty(t, hs.ARN)
			assert.False(t, hs.ExpirationTimestamp.IsZero())
		})
	}
}

// TestAcceptHandshake_InviteAddsAccount tests that accepting an INVITE handshake
// causes the invited account to appear in the organization.
func TestAcceptHandshake_InviteAddsAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		targetID         string
		wantAccountInOrg bool
		preExisting      bool
	}{
		{
			name:             "new_account_added",
			targetID:         "555555555555",
			preExisting:      false,
			wantAccountInOrg: true,
		},
		{
			name:             "existing_account_not_duplicated",
			targetID:         "555555555556",
			preExisting:      true,
			wantAccountInOrg: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			if tt.preExisting {
				b.AddAccountInternal(&organizations.Account{
					ID:     tt.targetID,
					ARN:    "arn:aws:organizations::123456789012:account/o-test/" + tt.targetID,
					Name:   "pre-existing",
					Email:  tt.targetID + "@example.com",
					Status: "ACTIVE",
				})
			}

			hs, err := b.InviteAccountToOrganization(
				organizations.HandshakeParty{ID: tt.targetID, Type: "ACCOUNT"},
				"",
				nil,
			)
			require.NoError(t, err)
			assert.Equal(t, "OPEN", hs.State)

			accepted, err := b.AcceptHandshake(hs.ID)
			require.NoError(t, err)
			assert.Equal(t, "ACCEPTED", accepted.State)

			if tt.wantAccountInOrg {
				acct, descErr := b.DescribeAccount(tt.targetID)
				require.NoError(t, descErr)
				assert.Equal(t, tt.targetID, acct.ID)
			}
		})
	}
}

// TestAcceptHandshake_StateTransitions tests accept/cancel/decline state machine.
func TestAcceptHandshake_StateTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		transition string // "accept", "cancel", "decline"
		wantState  string
		setupState string // pre-set state (empty = OPEN)
		wantErr    bool
	}{
		{name: "accept_open", transition: "accept", wantState: "ACCEPTED"},
		{name: "cancel_open", transition: "cancel", wantState: "CANCELED"},
		{name: "decline_open", transition: "decline", wantState: "DECLINED"},
		{
			name:       "accept_already_accepted",
			transition: "accept",
			setupState: "ACCEPTED",
			wantErr:    true,
		},
		{
			name:       "cancel_already_canceled",
			transition: "cancel",
			setupState: "CANCELED",
			wantErr:    true,
		},
		{
			name:       "decline_already_declined",
			transition: "decline",
			setupState: "DECLINED",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			state := "OPEN"
			if tt.setupState != "" {
				state = tt.setupState
			}

			b.AddHandshakeInternal(&organizations.Handshake{
				Action: "INVITE",
				State:  state,
				Parties: []organizations.HandshakeParty{
					{ID: "123456789012", Type: "ACCOUNT"},
				},
			})

			handshakes, err := b.ListHandshakesForOrganization("")
			require.NoError(t, err)
			require.Len(t, handshakes, 1)
			hsID := handshakes[0].ID

			var transErr error
			var result *organizations.Handshake

			switch tt.transition {
			case "accept":
				result, transErr = b.AcceptHandshake(hsID)
			case "cancel":
				result, transErr = b.CancelHandshake(hsID)
			case "decline":
				result, transErr = b.DeclineHandshake(hsID)
			}

			if tt.wantErr {
				require.Error(t, transErr)

				return
			}

			require.NoError(t, transErr)
			assert.Equal(t, tt.wantState, result.State)
		})
	}
}

// TestHandshakeFilter_ForAccount tests ListHandshakesForAccount with ActionType filter.
func TestHandshakeFilter_ForAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filter    string
		wantCount int
	}{
		{name: "no_filter_returns_all", filter: "", wantCount: 2},
		{name: "filter_invite", filter: "INVITE", wantCount: 1},
		{name: "filter_enable_all_features", filter: "ENABLE_ALL_FEATURES", wantCount: 1},
		{name: "filter_nonexistent", filter: "APPROVE_ALL_FEATURES", wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			b.AddHandshakeInternal(&organizations.Handshake{Action: "INVITE", State: "OPEN"})
			b.AddHandshakeInternal(&organizations.Handshake{Action: "ENABLE_ALL_FEATURES", State: "OPEN"})

			out, err := b.ListHandshakesForAccount(tt.filter)
			require.NoError(t, err)
			assert.Len(t, out, tt.wantCount)
		})
	}
}

// TestHandshakeFilter_ForOrganization tests ListHandshakesForOrganization with ActionType filter.
func TestHandshakeFilter_ForOrganization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filter    string
		wantCount int
	}{
		{name: "no_filter_returns_all", filter: "", wantCount: 3},
		{name: "filter_invite", filter: "INVITE", wantCount: 2},
		{name: "filter_enable", filter: "ENABLE_ALL_FEATURES", wantCount: 1},
		{name: "filter_no_match", filter: "CANCELLED_THING", wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			b.AddHandshakeInternal(&organizations.Handshake{Action: "INVITE", State: "OPEN"})
			b.AddHandshakeInternal(&organizations.Handshake{Action: "INVITE", State: "ACCEPTED"})
			b.AddHandshakeInternal(&organizations.Handshake{Action: "ENABLE_ALL_FEATURES", State: "OPEN"})

			out, err := b.ListHandshakesForOrganization(tt.filter)
			require.NoError(t, err)
			assert.Len(t, out, tt.wantCount)
		})
	}
}

// TestEnableAllFeatures_HandshakeInList tests that the ENABLE_ALL_FEATURES
// handshake created by EnableAllFeatures appears in list operations.
func TestEnableAllFeatures_HandshakeInList(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	hs, err := b.EnableAllFeatures()
	require.NoError(t, err)

	allHs, err := b.ListHandshakesForOrganization("")
	require.NoError(t, err)

	found := false
	for _, h := range allHs {
		if h.ID == hs.ID {
			found = true
			assert.Equal(t, "ENABLE_ALL_FEATURES", h.Action)
			assert.Equal(t, "OPEN", h.State)
		}
	}

	assert.True(t, found, "ENABLE_ALL_FEATURES handshake must appear in ListHandshakesForOrganization")
}

// TestInviteHandshake_StateMachine tests the full INVITE handshake state machine
// including accept (adds account), decline (no account), cancel.
func TestInviteHandshake_StateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		targetID         string
		action           string // accept, decline, cancel
		wantFinalState   string
		wantAccountInOrg bool
	}{
		{
			name:             "accept_adds_account",
			targetID:         "444444444444",
			action:           "accept",
			wantFinalState:   "ACCEPTED",
			wantAccountInOrg: true,
		},
		{
			name:             "decline_no_account",
			targetID:         "444444444445",
			action:           "decline",
			wantFinalState:   "DECLINED",
			wantAccountInOrg: false,
		},
		{
			name:             "cancel_no_account",
			targetID:         "444444444446",
			action:           "cancel",
			wantFinalState:   "CANCELED",
			wantAccountInOrg: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			hs, err := b.InviteAccountToOrganization(
				organizations.HandshakeParty{ID: tt.targetID, Type: "ACCOUNT"},
				"test invite notes",
				nil,
			)
			require.NoError(t, err)
			assert.Equal(t, "OPEN", hs.State)
			assert.Equal(t, "INVITE", hs.Action)
			assert.NotEmpty(t, hs.ID)
			assert.NotEmpty(t, hs.ARN)

			var result *organizations.Handshake
			var transErr error

			switch tt.action {
			case "accept":
				result, transErr = b.AcceptHandshake(hs.ID)
			case "decline":
				result, transErr = b.DeclineHandshake(hs.ID)
			case "cancel":
				result, transErr = b.CancelHandshake(hs.ID)
			}

			require.NoError(t, transErr)
			assert.Equal(t, tt.wantFinalState, result.State)

			_, descErr := b.DescribeAccount(tt.targetID)
			if tt.wantAccountInOrg {
				require.NoError(t, descErr, "accepted invite should add account")
			} else {
				require.Error(t, descErr, "declined/canceled invite should not add account")
			}
		})
	}
}

// TestInviteHandshake_Resources tests handshake resources include
// the target account, organization ID, and master email.
func TestInviteHandshake_Resources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		targetID        string
		notes           string
		wantNotesInRes  bool
		wantResMinCount int
	}{
		{
			name:            "without_notes",
			targetID:        "333333333333",
			notes:           "",
			wantNotesInRes:  false,
			wantResMinCount: 3,
		},
		{
			name:            "with_notes",
			targetID:        "333333333334",
			notes:           "please join our org",
			wantNotesInRes:  true,
			wantResMinCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			hs, err := b.InviteAccountToOrganization(
				organizations.HandshakeParty{ID: tt.targetID, Type: "ACCOUNT"},
				tt.notes,
				nil,
			)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(hs.Resources), tt.wantResMinCount)

			hasNotes := false
			hasAccount := false
			hasOrg := false

			for _, r := range hs.Resources {
				switch r.Type {
				case "NOTES":
					hasNotes = true
					assert.Equal(t, tt.notes, r.Value)
				case "ACCOUNT":
					hasAccount = true
					assert.Equal(t, tt.targetID, r.Value)
				case "ORGANIZATION":
					hasOrg = true
				}
			}

			assert.True(t, hasAccount, "ACCOUNT resource must be present")
			assert.True(t, hasOrg, "ORGANIZATION resource must be present")
			assert.Equal(t, tt.wantNotesInRes, hasNotes)
		})
	}
}

// TestInviteHandshake_DuplicateAcceptFails tests that a non-OPEN handshake
// cannot be re-acted upon.
func TestInviteHandshake_DuplicateAcceptFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		firstOp  string
		secondOp string
	}{
		{name: "accept_then_accept", firstOp: "accept", secondOp: "accept"},
		{name: "accept_then_cancel", firstOp: "accept", secondOp: "cancel"},
		{name: "accept_then_decline", firstOp: "accept", secondOp: "decline"},
		{name: "cancel_then_accept", firstOp: "cancel", secondOp: "accept"},
		{name: "decline_then_cancel", firstOp: "decline", secondOp: "cancel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			hs, err := b.InviteAccountToOrganization(
				organizations.HandshakeParty{ID: "777777777777", Type: "ACCOUNT"},
				"",
				nil,
			)
			require.NoError(t, err)

			applyOp := func(op, hsID string) error {
				switch op {
				case "accept":
					_, e := b.AcceptHandshake(hsID)

					return e
				case "decline":
					_, e := b.DeclineHandshake(hsID)

					return e
				case "cancel":
					_, e := b.CancelHandshake(hsID)

					return e
				}

				return nil
			}

			require.NoError(t, applyOp(tt.firstOp, hs.ID))
			require.Error(t, applyOp(tt.secondOp, hs.ID),
				"second operation on non-OPEN handshake must fail")
		})
	}
}

// TestHandshakeARN_LowercaseAction verifies that handshake ARNs use lowercase action.
func TestHandshakeARN_LowercaseAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setupFn      func(b *organizations.InMemoryBackend) (*organizations.Handshake, error)
		wantFragment string
	}{
		{
			name: "invite_action_lowercase",
			setupFn: func(b *organizations.InMemoryBackend) (*organizations.Handshake, error) {
				return b.InviteAccountToOrganization(organizations.HandshakeParty{
					ID:   "123456789012",
					Type: "ACCOUNT",
				}, "", nil)
			},
			wantFragment: "/invite/",
		},
		{
			name: "enable_all_features_lowercase",
			setupFn: func(b *organizations.InMemoryBackend) (*organizations.Handshake, error) {
				return b.EnableAllFeatures()
			},
			wantFragment: "/enable_all_features/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			hs, err := tt.setupFn(b)
			require.NoError(t, err)

			assert.Contains(t, hs.ARN, tt.wantFragment,
				"handshake ARN %q should contain lowercase action %q", hs.ARN, tt.wantFragment)

			// Verify no uppercase in the action portion.
			assert.True(t, strings.ToLower(hs.ARN) == hs.ARN || !strings.Contains(hs.ARN, "/INVITE/"),
				"ARN should not contain uppercase action")
		})
	}
}

// ---------------------------------------------------------------------------
// Item 10: Lazy handshake expiration
// ---------------------------------------------------------------------------

// TestHandshakeExpiration verifies that expired handshakes are transitioned to EXPIRED.
func TestHandshakeExpiration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantState   string
		checkMethod string
		expired     bool
	}{
		{
			name:        "describe_transitions_expired",
			expired:     true,
			wantState:   "EXPIRED",
			checkMethod: "describe",
		},
		{
			name:        "list_for_org_transitions_expired",
			expired:     true,
			wantState:   "EXPIRED",
			checkMethod: "list_org",
		},
		{
			name:        "list_for_account_transitions_expired",
			expired:     true,
			wantState:   "EXPIRED",
			checkMethod: "list_acct",
		},
		{
			name:        "not_yet_expired_stays_open",
			expired:     false,
			wantState:   "OPEN",
			checkMethod: "describe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			// Seed a handshake with expiration in the past or future.
			expTime := time.Now().Add(24 * time.Hour)
			if tt.expired {
				expTime = time.Now().Add(-1 * time.Hour)
			}

			hs := &organizations.Handshake{
				ID:                  "h-test001",
				Action:              "INVITE",
				State:               "OPEN",
				ExpirationTimestamp: expTime,
				RequestedTimestamp:  time.Now().Add(-2 * time.Hour),
			}
			b.AddHandshakeInternal(hs)

			var gotState string

			switch tt.checkMethod {
			case "describe":
				result, err := b.DescribeHandshake("h-test001")
				require.NoError(t, err)
				gotState = result.State
			case "list_org":
				results, err := b.ListHandshakesForOrganization("")
				require.NoError(t, err)
				for _, h := range results {
					if h.ID == "h-test001" {
						gotState = h.State
					}
				}
			case "list_acct":
				results, err := b.ListHandshakesForAccount("")
				require.NoError(t, err)
				for _, h := range results {
					if h.ID == "h-test001" {
						gotState = h.State
					}
				}
			}

			assert.Equal(t, tt.wantState, gotState)
		})
	}
}

// ---------------------------------------------------------------------------
// Item 11: AttachPolicy target validation
// ---------------------------------------------------------------------------

// TestInviteAccount_PartyValidation verifies handshake party validation.
func TestInviteAccount_PartyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  organizations.HandshakeParty
		wantErr bool
	}{
		{
			name:    "account_type_12_digits_ok",
			target:  organizations.HandshakeParty{ID: "123456789012", Type: "ACCOUNT"},
			wantErr: false,
		},
		{
			name:    "account_type_11_digits_rejected",
			target:  organizations.HandshakeParty{ID: "12345678901", Type: "ACCOUNT"},
			wantErr: true,
		},
		{
			name:    "account_type_13_digits_rejected",
			target:  organizations.HandshakeParty{ID: "1234567890123", Type: "ACCOUNT"},
			wantErr: true,
		},
		{
			name:    "email_type_with_at_ok",
			target:  organizations.HandshakeParty{ID: "user@example.com", Type: "EMAIL"},
			wantErr: false,
		},
		{
			name:    "email_type_without_at_rejected",
			target:  organizations.HandshakeParty{ID: "userexample.com", Type: "EMAIL"},
			wantErr: true,
		},
		{
			name:    "empty_id_rejected",
			target:  organizations.HandshakeParty{ID: "", Type: "ACCOUNT"},
			wantErr: true,
		},
		{
			name:    "invalid_type_rejected",
			target:  organizations.HandshakeParty{ID: "123456789012", Type: "ORGANIZATION"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, _ := newOrgBackend(t)

			_, err := b.InviteAccountToOrganization(tt.target, "", nil)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Combined / integration tests
// ---------------------------------------------------------------------------

// TestInviteAccount_DuplicateOpen_Rejected verifies that inviting an
// account that already has an open invitation returns DuplicateHandshakeException.
func TestInviteAccount_DuplicateOpen_Rejected(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	target := organizations.HandshakeParty{ID: "111111111111", Type: "ACCOUNT"}

	_, err := b.InviteAccountToOrganization(target, "", nil)
	require.NoError(t, err)

	_, err = b.InviteAccountToOrganization(target, "second attempt", nil)
	require.Error(t, err, "duplicate open invite must be rejected")
}

// TestInviteAccount_AfterCancel_OK verifies that after canceling the
// first invitation, a new invitation to the same account succeeds.
func TestInviteAccount_AfterCancel_OK(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	target := organizations.HandshakeParty{ID: "333333333333", Type: "ACCOUNT"}

	h1, err := b.InviteAccountToOrganization(target, "", nil)
	require.NoError(t, err)

	_, err = b.CancelHandshake(h1.ID)
	require.NoError(t, err)

	_, err = b.InviteAccountToOrganization(target, "re-invite after cancel", nil)
	require.NoError(t, err, "invite after cancellation must succeed")
}

// TestInviteAccount_DifferentTargets_OK verifies that multiple open
// invitations to different accounts are permitted.
func TestInviteAccount_DifferentTargets_OK(t *testing.T) {
	t.Parallel()

	b, _ := newOrgBackend(t)

	_, err := b.InviteAccountToOrganization(organizations.HandshakeParty{ID: "444444444444", Type: "ACCOUNT"}, "", nil)
	require.NoError(t, err)

	_, err = b.InviteAccountToOrganization(organizations.HandshakeParty{ID: "555555555555", Type: "ACCOUNT"}, "", nil)
	require.NoError(t, err, "inviting different accounts simultaneously must succeed")
}

// TestBackend_HandshakeOperations tests handshake CRUD methods.
func TestBackend_HandshakeOperations(t *testing.T) {
	t.Parallel()

	makeOpenHandshake := func(id string) *organizations.Handshake {
		now := time.Now()

		return &organizations.Handshake{
			ID:                  id,
			ARN:                 "arn:aws:organizations::123456789012:handshake/o-test/invite/" + id,
			Action:              "INVITE",
			State:               "OPEN",
			RequestedTimestamp:  now,
			ExpirationTimestamp: now.Add(7 * 24 * time.Hour),
		}
	}

	tests := []struct {
		name      string
		op        string
		id        string
		seedID    string
		seedState string
		wantState string
		wantErr   bool
	}{
		{
			name:      "describe_found",
			op:        "DescribeHandshake",
			id:        "h-desc001",
			seedID:    "h-desc001",
			wantState: "OPEN",
		},
		{
			name:    "describe_not_found",
			op:      "DescribeHandshake",
			id:      "h-notfound",
			wantErr: true,
		},
		{
			name:      "accept_open",
			op:        "AcceptHandshake",
			id:        "h-acc0001",
			seedID:    "h-acc0001",
			wantState: "ACCEPTED",
		},
		{
			name:    "accept_not_found",
			op:      "AcceptHandshake",
			id:      "h-missing",
			wantErr: true,
		},
		{
			name:      "cancel_open",
			op:        "CancelHandshake",
			id:        "h-can0001",
			seedID:    "h-can0001",
			wantState: "CANCELED",
		},
		{
			name:      "decline_open",
			op:        "DeclineHandshake",
			id:        "h-dec0001",
			seedID:    "h-dec0001",
			wantState: "DECLINED",
		},
		{
			name:      "accept_already_accepted",
			op:        "AcceptHandshake",
			id:        "h-aa00001",
			seedID:    "h-aa00001",
			seedState: "ACCEPTED",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.seedID != "" {
				state := "OPEN"
				if tt.seedState != "" {
					state = tt.seedState
				}

				hs := makeOpenHandshake(tt.seedID)
				hs.State = state
				b.AddHandshakeInternal(hs)
			}

			var (
				result *organizations.Handshake
				err    error
			)

			switch tt.op {
			case "DescribeHandshake":
				result, err = b.DescribeHandshake(tt.id)
			case "AcceptHandshake":
				result, err = b.AcceptHandshake(tt.id)
			case "CancelHandshake":
				result, err = b.CancelHandshake(tt.id)
			case "DeclineHandshake":
				result, err = b.DeclineHandshake(tt.id)
			}

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.id, result.ID)
			assert.Equal(t, tt.wantState, result.State)
		})
	}
}

// TestBackend_DescribeResponsibilityTransfer tests the backend method.
func TestBackend_DescribeResponsibilityTransfer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		transferID string
		seed       bool
		wantErr    bool
	}{
		{
			name:       "found",
			transferID: "rt-00000001",
			seed:       true,
		},
		{
			name:       "not_found",
			transferID: "rt-notfound",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.seed {
				b.AddResponsibilityTransferInternal(&organizations.ResponsibilityTransfer{
					ID:                tt.transferID,
					ARN:               "arn:aws:organizations::123456789012:transfer/o-test/billing/outbound/" + tt.transferID,
					ActiveHandshakeID: "h-rt00001",
					Name:              "billing-transfer",
					Status:            "REQUESTED",
					Type:              "BILLING",
				})
			}

			result, err := b.DescribeResponsibilityTransfer(tt.transferID)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.transferID, result.ID)
			assert.Equal(t, "billing-transfer", result.Name)
			assert.Equal(t, "REQUESTED", result.Status)
		})
	}
}

// TestBackend_ResponsibilityTransfer_Lifecycle tests
// Update/TerminateResponsibilityTransfer and the Handshake-lifecycle status sync.
func TestBackend_ResponsibilityTransfer_Lifecycle(t *testing.T) {
	t.Parallel()

	t.Run("update_renames", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		b.AddResponsibilityTransferInternal(&organizations.ResponsibilityTransfer{
			ID: "rt-update01", Status: "REQUESTED", Type: "BILLING",
		})

		result, err := b.UpdateResponsibilityTransfer("rt-update01", "new-name")
		require.NoError(t, err)
		assert.Equal(t, "new-name", result.Name)
	})

	t.Run("update_not_found", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()

		_, err := b.UpdateResponsibilityTransfer("rt-missing", "new-name")
		require.Error(t, err)
	})

	t.Run("terminate_requires_accepted", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		b.AddResponsibilityTransferInternal(&organizations.ResponsibilityTransfer{
			ID: "rt-term01", Status: "REQUESTED", Type: "BILLING",
		})

		_, err := b.TerminateResponsibilityTransfer("rt-term01", nil)
		require.Error(t, err)
	})

	t.Run("terminate_accepted_sets_end_timestamp", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		b.AddResponsibilityTransferInternal(&organizations.ResponsibilityTransfer{
			ID: "rt-term02", Status: "ACCEPTED", Type: "BILLING",
		})

		result, err := b.TerminateResponsibilityTransfer("rt-term02", nil)
		require.NoError(t, err)
		assert.False(t, result.EndTimestamp.IsZero())
	})

	t.Run("terminate_already_ended", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		b.AddResponsibilityTransferInternal(&organizations.ResponsibilityTransfer{
			ID: "rt-term03", Status: "ACCEPTED", Type: "BILLING",
		})

		_, err := b.TerminateResponsibilityTransfer("rt-term03", nil)
		require.NoError(t, err)

		_, err = b.TerminateResponsibilityTransfer("rt-term03", nil)
		require.Error(t, err)
	})

	t.Run("accept_handshake_syncs_transfer_status", func(t *testing.T) {
		t.Parallel()

		b, _ := newOrgBackend(t)

		hs, err := b.InviteOrganizationToTransferResponsibility(
			organizations.HandshakeParty{ID: "888888888888", Type: "ACCOUNT"},
			organizations.TransferResponsibilityParams{
				SourceName:     "billing-transfer",
				StartTimestamp: time.Now().Add(time.Hour),
				Type:           "BILLING",
			},
		)
		require.NoError(t, err)

		outbound, err := b.ListOutboundResponsibilityTransfers("BILLING")
		require.NoError(t, err)
		require.Len(t, outbound, 1)
		assert.Equal(t, "REQUESTED", outbound[0].Status)

		_, err = b.AcceptHandshake(hs.ID)
		require.NoError(t, err)

		result, err := b.DescribeResponsibilityTransfer(outbound[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "ACCEPTED", result.Status)
	})
}

// TestAddHandshakeInternal_SetsExpiry verifies expiry is set automatically.
func TestAddHandshakeInternal_SetsExpiry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		withExpiry bool
		withID     bool
		withARN    bool
	}{
		{name: "auto_expiry_id_arn", withExpiry: false, withID: false, withARN: false},
		{name: "explicit_expiry_preserved", withExpiry: true, withID: true, withARN: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			h := &organizations.Handshake{
				State: "OPEN",
			}

			if tt.withExpiry {
				h.ExpirationTimestamp = time.Now().Add(24 * time.Hour)
			}

			if tt.withID {
				h.ID = "h-testid00"
			}

			b.AddHandshakeInternal(h)

			assert.False(t, h.ExpirationTimestamp.IsZero(), "expiry should be set")

			if tt.withID {
				assert.Equal(t, "h-testid00", h.ID)
			} else {
				assert.NotEmpty(t, h.ID)
			}

			assert.Equal(t, 1, organizations.HandshakeCount(b))
		})
	}
}

// TestLeaveOrganization_AlwaysFailsForManagementAccount verifies AWS
// behaviour: LeaveOrganization's doc comment says "You can only call from
// operation from a member account." This backend's caller identity is
// always the management account (organization.go's CreateOrganization sets
// b.accountID as MasterAccountID), so the call must always fail with
// MasterCannotLeaveOrganizationException rather than succeeding as a no-op.
func TestLeaveOrganization_AlwaysFailsForManagementAccount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "management_account_cannot_leave"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			createOrgOn(t, b)

			err := b.LeaveOrganization()
			require.ErrorIs(t, err, organizations.ErrMasterCannotLeaveOrganization)

			org, describeErr := b.DescribeOrganization()
			require.NoError(t, describeErr)
			require.NotNil(t, org, "organization must still exist after a failed LeaveOrganization")
		})
	}
}
