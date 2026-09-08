package workmail_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Mobile Device Access Rules ----

func TestMobileDeviceAccessRuleLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	orgID := createTestOrg(t, h, "mdar-org")

	type createCase struct {
		name         string
		effect       string
		description  string
		deviceModels []string
		deviceTypes  []string
	}
	cases := []createCase{
		{name: "rule-allow", effect: "ALLOW", description: "Allow iPhones", deviceModels: []string{"iPhone"}},
		{name: "rule-deny", effect: "DENY", description: "Deny Androids", deviceTypes: []string{"Android"}},
	}

	ruleIDs := map[string]string{}
	for _, tc := range cases {
		body := fmt.Sprintf(`{"OrganizationId":%q,"Name":%q,"Effect":%q,"Description":%q`,
			orgID, tc.name, tc.effect, tc.description)
		if len(tc.deviceModels) > 0 {
			modelsJSON, _ := json.Marshal(tc.deviceModels)
			body += fmt.Sprintf(`,"DeviceModels":%s`, string(modelsJSON))
		}
		if len(tc.deviceTypes) > 0 {
			typesJSON, _ := json.Marshal(tc.deviceTypes)
			body += fmt.Sprintf(`,"DeviceTypes":%s`, string(typesJSON))
		}
		body += "}"
		rec := doOp(t, h, "CreateMobileDeviceAccessRule", body)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		m := decodeJSON(t, rec)
		ruleID, ok := m["MobileDeviceAccessRuleId"].(string)
		require.True(t, ok)
		require.NotEmpty(t, ruleID)
		ruleIDs[tc.name] = ruleID
	}

	// List
	rec := doOp(t, h, "ListMobileDeviceAccessRules", fmt.Sprintf(`{"OrganizationId":%q}`, orgID))
	require.Equal(t, http.StatusOK, rec.Code)
	m := decodeJSON(t, rec)
	rules, ok := m["Rules"].([]any)
	require.True(t, ok)
	assert.Len(t, rules, 2)

	// Update
	ruleID := ruleIDs["rule-allow"]
	rec = doOp(t, h, "UpdateMobileDeviceAccessRule", fmt.Sprintf(
		`{"OrganizationId":%q,"MobileDeviceAccessRuleId":%q,"Name":"updated-rule","Effect":"DENY","Description":"Updated"}`,
		orgID,
		ruleID,
	))
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify update in list
	rec = doOp(t, h, "ListMobileDeviceAccessRules", fmt.Sprintf(`{"OrganizationId":%q}`, orgID))
	m = decodeJSON(t, rec)
	rules = m["Rules"].([]any)
	found := false
	for _, r := range rules {
		rule := r.(map[string]any)
		if rule["MobileDeviceAccessRuleId"] == ruleID {
			assert.Equal(t, "updated-rule", rule["Name"])
			assert.Equal(t, "DENY", rule["Effect"])
			found = true
		}
	}
	assert.True(t, found)

	// Delete
	rec = doOp(t, h, "DeleteMobileDeviceAccessRule", fmt.Sprintf(
		`{"OrganizationId":%q,"MobileDeviceAccessRuleId":%q}`, orgID, ruleID,
	))
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify gone
	rec = doOp(t, h, "ListMobileDeviceAccessRules", fmt.Sprintf(`{"OrganizationId":%q}`, orgID))
	m = decodeJSON(t, rec)
	rules = m["Rules"].([]any)
	assert.Len(t, rules, 1)
}

// TestDeleteMobileDeviceAccessRule_NonExistent locks the real WorkMail wire
// behavior (aws-sdk-go-v2 api_op_DeleteMobileDeviceAccessRule.go doc:
// "Deleting already deleted and non-existing rules does not produce an
// error. In those cases, the service sends back an HTTP 200 response with an
// empty HTTP body."). DeleteMobileDeviceAccessRule's own error model
// (awsAwsjson11_deserializeOpErrorDeleteMobileDeviceAccessRule) also declares
// no EntityNotFoundException at all.
func TestDeleteMobileDeviceAccessRule_NonExistent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	orgID := createTestOrg(t, h, "mdar-missing-org")

	rec := doOp(t, h, "DeleteMobileDeviceAccessRule", fmt.Sprintf(
		`{"OrganizationId":%q,"MobileDeviceAccessRuleId":"never-existed"}`, orgID,
	))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	m := decodeJSON(t, rec)
	_, hasType := m["__type"]
	assert.False(t, hasType, "expected empty success body, got error type %v", m["__type"])
}

func TestGetMobileDeviceAccessEffect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		ruleEffect   string
		queryModel   string
		wantEffect   string
		deviceModels []string
		wantMatchLen int
	}{
		{
			name:         "matches ALLOW rule",
			ruleEffect:   "ALLOW",
			deviceModels: []string{"iPhone"},
			queryModel:   "iPhone",
			wantEffect:   "ALLOW",
			wantMatchLen: 1,
		},
		{
			name:         "no match returns default ALLOW",
			ruleEffect:   "DENY",
			deviceModels: []string{"Android"},
			queryModel:   "iPhone",
			wantEffect:   "ALLOW",
			wantMatchLen: 0,
		},
		{
			name:         "matches DENY rule",
			ruleEffect:   "DENY",
			deviceModels: []string{"BlackBerry"},
			queryModel:   "BlackBerry",
			wantEffect:   "DENY",
			wantMatchLen: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			orgID := createTestOrg(t, h, "effect-org-"+tc.name)

			modelsJSON, _ := json.Marshal(tc.deviceModels)
			doOp(t, h, "CreateMobileDeviceAccessRule", fmt.Sprintf(
				`{"OrganizationId":%q,"Name":"test-rule","Effect":%q,"DeviceModels":%s}`,
				orgID, tc.ruleEffect, string(modelsJSON),
			))

			rec := doOp(t, h, "GetMobileDeviceAccessEffect", fmt.Sprintf(
				`{"OrganizationId":%q,"DeviceModel":%q}`, orgID, tc.queryModel,
			))
			require.Equal(t, http.StatusOK, rec.Code)
			m := decodeJSON(t, rec)
			assert.Equal(t, tc.wantEffect, m["Effect"])
			matched := m["MatchedRules"].([]any)
			assert.Len(t, matched, tc.wantMatchLen)
		})
	}
}

// ---- Mobile Device Access Overrides ----

func TestMobileDeviceAccessOverrideLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	orgID := createTestOrg(t, h, "override-org")

	tests := []struct {
		name        string
		userID      string
		deviceID    string
		effect      string
		description string
	}{
		{
			name:        "allow override",
			userID:      "user-abc",
			deviceID:    "device-001",
			effect:      "ALLOW",
			description: "Allow specific device",
		},
		{
			name:        "deny override",
			userID:      "user-xyz",
			deviceID:    "device-002",
			effect:      "DENY",
			description: "Deny specific device",
		},
	}

	for _, tc := range tests {
		// Put
		rec := doOp(t, h, "PutMobileDeviceAccessOverride", fmt.Sprintf(
			`{"OrganizationId":%q,"UserId":%q,"DeviceId":%q,"Effect":%q,"Description":%q}`,
			orgID, tc.userID, tc.deviceID, tc.effect, tc.description,
		))
		require.Equal(t, http.StatusOK, rec.Code, tc.name)

		// Get
		rec = doOp(t, h, "GetMobileDeviceAccessOverride", fmt.Sprintf(
			`{"OrganizationId":%q,"UserId":%q,"DeviceId":%q}`, orgID, tc.userID, tc.deviceID,
		))
		require.Equal(t, http.StatusOK, rec.Code)
		m := decodeJSON(t, rec)
		assert.Equal(t, tc.userID, m["UserId"])
		assert.Equal(t, tc.effect, m["Effect"])
		assert.Equal(t, tc.description, m["Description"])
	}

	// List all
	rec := doOp(t, h, "ListMobileDeviceAccessOverrides", fmt.Sprintf(
		`{"OrganizationId":%q}`, orgID,
	))
	require.Equal(t, http.StatusOK, rec.Code)
	m := decodeJSON(t, rec)
	overrides := m["Overrides"].([]any)
	assert.Len(t, overrides, 2)

	// List filtered by user
	rec = doOp(t, h, "ListMobileDeviceAccessOverrides", fmt.Sprintf(
		`{"OrganizationId":%q,"UserId":"user-abc"}`, orgID,
	))
	m = decodeJSON(t, rec)
	overrides = m["Overrides"].([]any)
	assert.Len(t, overrides, 1)
	assert.Equal(t, "user-abc", overrides[0].(map[string]any)["UserId"])

	// Delete
	rec = doOp(t, h, "DeleteMobileDeviceAccessOverride", fmt.Sprintf(
		`{"OrganizationId":%q,"UserId":"user-abc","DeviceId":"device-001"}`, orgID,
	))
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify gone
	rec = doOp(t, h, "ListMobileDeviceAccessOverrides", fmt.Sprintf(`{"OrganizationId":%q}`, orgID))
	m = decodeJSON(t, rec)
	overrides = m["Overrides"].([]any)
	assert.Len(t, overrides, 1)
}

func TestMobileDeviceAccessOverrideErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		action    string
		body      string
		wantError string
		wantCode  int
	}{
		{
			name:      "get nonexistent override",
			action:    "GetMobileDeviceAccessOverride",
			body:      `{"OrganizationId":"org-123456789012","UserId":"nope","DeviceId":"dev-nope"}`,
			wantCode:  http.StatusBadRequest,
			wantError: "EntityNotFoundException",
		},
		{
			name:      "delete nonexistent override",
			action:    "DeleteMobileDeviceAccessOverride",
			body:      `{"OrganizationId":"org-123456789012","UserId":"nope","DeviceId":"dev-nope"}`,
			wantCode:  http.StatusBadRequest,
			wantError: "EntityNotFoundException",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rec := doOp(t, h, tc.action, tc.body)
			require.Equal(t, tc.wantCode, rec.Code)
			m := decodeJSON(t, rec)
			assert.Contains(t, m["__type"], tc.wantError)
		})
	}
}
