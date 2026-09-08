package workmail_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/workmail"
)

// --- Access Control Rules ---

func TestWorkMail_AccessControlRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *workmail.Handler)
		name string
	}{
		{
			name: "put_and_list_rules",
			run: func(t *testing.T, h *workmail.Handler) {
				t.Helper()
				orgID := createTestOrg(t, h, "acrorg")
				rec := doOp(t, h, "PutAccessControlRule", fmt.Sprintf(
					`{"OrganizationId":%q,"Name":"AllowAll","Effect":"ALLOW","Description":"Allow everything"}`,
					orgID,
				))
				require.Equal(t, http.StatusOK, rec.Code)
				rec2 := doOp(t, h, "ListAccessControlRules", fmt.Sprintf(
					`{"OrganizationId":%q}`, orgID,
				))
				require.Equal(t, http.StatusOK, rec2.Code)
				m := decodeJSON(t, rec2)
				rules, ok := m["Rules"].([]any)
				require.True(t, ok)
				require.Len(t, rules, 1)
				r := rules[0].(map[string]any)
				assert.Equal(t, "AllowAll", r["Name"])
				assert.Equal(t, "ALLOW", r["Effect"])
			},
		},
		{
			name: "delete_rule",
			run: func(t *testing.T, h *workmail.Handler) {
				t.Helper()
				orgID := createTestOrg(t, h, "acrorgrm")
				doOp(t, h, "PutAccessControlRule", fmt.Sprintf(
					`{"OrganizationId":%q,"Name":"TempRule","Effect":"DENY"}`, orgID,
				))
				rec := doOp(t, h, "DeleteAccessControlRule", fmt.Sprintf(
					`{"OrganizationId":%q,"Name":"TempRule"}`, orgID,
				))
				require.Equal(t, http.StatusOK, rec.Code)
				rec2 := doOp(t, h, "ListAccessControlRules", fmt.Sprintf(`{"OrganizationId":%q}`, orgID))
				m := decodeJSON(t, rec2)
				rules := m["Rules"].([]any)
				assert.Empty(t, rules)
			},
		},
		{
			name: "get_access_control_effect",
			run: func(t *testing.T, h *workmail.Handler) {
				t.Helper()
				orgID := createTestOrg(t, h, "aceorg")
				rec := doOp(t, h, "GetAccessControlEffect", fmt.Sprintf(
					`{"OrganizationId":%q,"IpAddress":"10.0.0.1","Action":"LOGIN","UserId":"u-123"}`, orgID,
				))
				require.Equal(t, http.StatusOK, rec.Code)
				m := decodeJSON(t, rec)
				assert.NotEmpty(t, m["Effect"])
				_, hasMatched := m["MatchedRules"]
				assert.True(t, hasMatched)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.run(t, h)
		})
	}
}

// TestDeleteAccessControlRule_NonExistent locks the real WorkMail wire
// behavior (aws-sdk-go-v2 api_op_DeleteAccessControlRule.go doc: "Deleting
// already deleted and non-existing rules does not produce an error. In those
// cases, the service sends back an HTTP 200 response with an empty HTTP
// body."). DeleteAccessControlRule's own error model
// (awsAwsjson11_deserializeOpErrorDeleteAccessControlRule) also declares no
// EntityNotFoundException at all, so a not-found error here can never be a
// real wire response either way.
func TestDeleteAccessControlRule_NonExistent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	orgID := createTestOrg(t, h, "acr-missing-org")

	rec := doOp(t, h, "DeleteAccessControlRule", fmt.Sprintf(
		`{"OrganizationId":%q,"Name":"never-existed"}`, orgID,
	))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	m := decodeJSON(t, rec)
	_, hasType := m["__type"]
	assert.False(t, hasType, "expected empty success body, got error type %v", m["__type"])
}

func TestGetAccessControlEffect_RuleEvaluation(t *testing.T) {
	t.Parallel()

	// putACRule puts a single ACR into the handler's org.
	putACRule := func(t *testing.T, h *workmail.Handler, orgID, name, effect string, body map[string]any) {
		t.Helper()

		body["OrganizationId"] = orgID
		body["Name"] = name
		body["Effect"] = effect

		jsonBody, err := json.Marshal(body)
		require.NoError(t, err)

		rec := doOp(t, h, "PutAccessControlRule", string(jsonBody))
		require.Equal(t, http.StatusOK, rec.Code)
	}

	aceRequest := func(t *testing.T, h *workmail.Handler, orgID, ipAddr, action, userID string) map[string]any {
		t.Helper()

		body := fmt.Sprintf(`{"OrganizationId":%q,"IpAddress":%q,"Action":%q,"UserId":%q}`,
			orgID, ipAddr, action, userID)
		rec := doOp(t, h, "GetAccessControlEffect", body)
		require.Equal(t, http.StatusOK, rec.Code)

		return decodeJSON(t, rec)
	}

	tests := []struct {
		setupRules  func(t *testing.T, h *workmail.Handler, orgID string)
		name        string
		ipAddr      string
		action      string
		userID      string
		wantEffect  string
		wantMatched bool
	}{
		{
			name: "no_rules_returns_allow",
			setupRules: func(_ *testing.T, _ *workmail.Handler, _ string) {
				// no rules
			},
			ipAddr:      "1.2.3.4",
			action:      "AutoDiscover",
			userID:      "some-user",
			wantEffect:  "ALLOW",
			wantMatched: false,
		},
		{
			name: "action_match_deny",
			setupRules: func(t *testing.T, h *workmail.Handler, orgID string) {
				t.Helper()
				putACRule(t, h, orgID, "r1", "DENY",
					map[string]any{"Actions": []string{"ActiveSync"}})
			},
			ipAddr:      "1.2.3.4",
			action:      "ActiveSync",
			userID:      "some-user",
			wantEffect:  "DENY",
			wantMatched: true,
		},
		{
			name: "action_no_match_allow",
			setupRules: func(t *testing.T, h *workmail.Handler, orgID string) {
				t.Helper()
				putACRule(t, h, orgID, "r1", "DENY",
					map[string]any{"Actions": []string{"ActiveSync"}})
			},
			ipAddr:      "1.2.3.4",
			action:      "AutoDiscover",
			userID:      "some-user",
			wantEffect:  "ALLOW",
			wantMatched: false,
		},
		{
			name: "ip_in_range_deny",
			setupRules: func(t *testing.T, h *workmail.Handler, orgID string) {
				t.Helper()
				putACRule(t, h, orgID, "r1", "DENY",
					map[string]any{"IpRanges": []string{"10.0.0.0/8"}})
			},
			ipAddr:      "10.0.0.1",
			action:      "AutoDiscover",
			userID:      "some-user",
			wantEffect:  "DENY",
			wantMatched: true,
		},
		{
			name: "ip_outside_range_allow",
			setupRules: func(t *testing.T, h *workmail.Handler, orgID string) {
				t.Helper()
				putACRule(t, h, orgID, "r1", "DENY",
					map[string]any{"IpRanges": []string{"10.0.0.0/8"}})
			},
			ipAddr:      "192.168.1.1",
			action:      "AutoDiscover",
			userID:      "some-user",
			wantEffect:  "ALLOW",
			wantMatched: false,
		},
		{
			// NotActions=["ActiveSync"] means rule matches when action is NOT "ActiveSync".
			name: "not_actions_blocks_other_actions",
			setupRules: func(t *testing.T, h *workmail.Handler, orgID string) {
				t.Helper()
				putACRule(t, h, orgID, "r1", "DENY",
					map[string]any{"NotActions": []string{"ActiveSync"}})
			},
			ipAddr:      "1.2.3.4",
			action:      "AutoDiscover", // not "ActiveSync", so matches
			userID:      "some-user",
			wantEffect:  "DENY",
			wantMatched: true,
		},
		{
			// NotActions=["ActiveSync"] means rule does NOT match when action IS "ActiveSync".
			name: "not_actions_skips_listed_action",
			setupRules: func(t *testing.T, h *workmail.Handler, orgID string) {
				t.Helper()
				putACRule(t, h, orgID, "r1", "DENY",
					map[string]any{"NotActions": []string{"ActiveSync"}})
			},
			ipAddr:      "1.2.3.4",
			action:      "ActiveSync", // is in NotActions, so rule does not match
			userID:      "some-user",
			wantEffect:  "ALLOW",
			wantMatched: false,
		},
		{
			name: "user_id_match_deny",
			setupRules: func(t *testing.T, h *workmail.Handler, orgID string) {
				t.Helper()
				putACRule(t, h, orgID, "r1", "DENY",
					map[string]any{"UserIds": []string{"target-user-id"}})
			},
			ipAddr:      "1.2.3.4",
			action:      "AutoDiscover",
			userID:      "target-user-id",
			wantEffect:  "DENY",
			wantMatched: true,
		},
		{
			name: "user_id_no_match_allow",
			setupRules: func(t *testing.T, h *workmail.Handler, orgID string) {
				t.Helper()
				putACRule(t, h, orgID, "r1", "DENY",
					map[string]any{"UserIds": []string{"target-user-id"}})
			},
			ipAddr:      "1.2.3.4",
			action:      "AutoDiscover",
			userID:      "other-user-id",
			wantEffect:  "ALLOW",
			wantMatched: false,
		},
		{
			// First rule DENYs; second rule ALLOWs the same action but should never be reached.
			name: "first_matching_rule_wins",
			setupRules: func(t *testing.T, h *workmail.Handler, orgID string) {
				t.Helper()
				putACRule(t, h, orgID, "r1", "DENY",
					map[string]any{"Actions": []string{"ActiveSync"}})
				putACRule(t, h, orgID, "r2", "ALLOW",
					map[string]any{"Actions": []string{"ActiveSync"}})
			},
			ipAddr:      "1.2.3.4",
			action:      "ActiveSync",
			userID:      "some-user",
			wantEffect:  "DENY",
			wantMatched: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			orgID := createTestOrg(t, h, "ace-org-"+tc.name)
			tc.setupRules(t, h, orgID)

			m := aceRequest(t, h, orgID, tc.ipAddr, tc.action, tc.userID)
			assert.Equal(t, tc.wantEffect, m["Effect"], "effect mismatch")

			matched, _ := m["MatchedRules"].([]any)
			if tc.wantMatched {
				assert.NotEmpty(t, matched, "expected MatchedRules to be non-empty")
			} else {
				assert.Empty(t, matched, "expected MatchedRules to be empty")
			}
		})
	}
}

func TestListAccessControlRules_Timestamps(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	orgID := createTestOrg(t, h, "acr-ts-org")

	rec := doOp(t, h, "PutAccessControlRule", fmt.Sprintf(
		`{"OrganizationId":%q,"Name":"ts-rule","Effect":"ALLOW","Description":"ts test"}`,
		orgID,
	))
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doOp(t, h, "ListAccessControlRules", fmt.Sprintf(`{"OrganizationId":%q}`, orgID))
	require.Equal(t, http.StatusOK, rec.Code)

	m := decodeJSON(t, rec)
	rules, _ := m["Rules"].([]any)
	require.Len(t, rules, 1)

	rule := rules[0].(map[string]any)
	dateCreated, _ := rule["DateCreated"].(float64)
	dateModified, _ := rule["DateModified"].(float64)

	assert.NotZero(t, dateCreated, "DateCreated should be non-zero")
	assert.NotZero(t, dateModified, "DateModified should be non-zero")
}

// TestAccessControlRule_ImpersonationRoleIDs locks
// ImpersonationRoleIds/NotImpersonationRoleIds on AccessControlRule
// (PutAccessControlRule's request, ListAccessControlRules' response, and
// GetAccessControlEffect's ImpersonationRoleId condition matching) --
// previously accepted on PutAccessControlRule's wire shape but neither
// stored, echoed back by ListAccessControlRules, nor evaluated by
// GetAccessControlEffect.
func TestAccessControlRule_ImpersonationRoleIDs(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	orgID := createTestOrg(t, h, "acr-impersonation-org")

	rec := doOp(t, h, "PutAccessControlRule", fmt.Sprintf(`{
		"OrganizationId":%q,"Name":"deny-role-x","Effect":"DENY",
		"ImpersonationRoleIds":["role-x"],"NotImpersonationRoleIds":["role-y"]
	}`, orgID))
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doOp(t, h, "ListAccessControlRules", fmt.Sprintf(`{"OrganizationId":%q}`, orgID))
	require.Equal(t, http.StatusOK, rec.Code)
	m := decodeJSON(t, rec)
	rules, _ := m["Rules"].([]any)
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]any)
	ids, _ := rule["ImpersonationRoleIds"].([]any)
	notIDs, _ := rule["NotImpersonationRoleIds"].([]any)
	assert.Equal(t, []any{"role-x"}, ids)
	assert.Equal(t, []any{"role-y"}, notIDs)

	tests := []struct {
		name                string
		impersonationRoleID string
		wantEffect          string
		wantMatched         bool
	}{
		{name: "matching_role_denied", impersonationRoleID: "role-x", wantEffect: "DENY", wantMatched: true},
		{name: "other_role_allowed", impersonationRoleID: "role-z", wantEffect: "ALLOW", wantMatched: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			aceRec := doOp(t, h, "GetAccessControlEffect", fmt.Sprintf(
				`{"OrganizationId":%q,"IpAddress":"1.2.3.4","Action":"EWS","ImpersonationRoleId":%q}`,
				orgID, tc.impersonationRoleID,
			))
			require.Equal(t, http.StatusOK, aceRec.Code)

			aceBody := decodeJSON(t, aceRec)
			assert.Equal(t, tc.wantEffect, aceBody["Effect"])
			matched, _ := aceBody["MatchedRules"].([]any)
			if tc.wantMatched {
				assert.NotEmpty(t, matched)
			} else {
				assert.Empty(t, matched)
			}
		})
	}
}
