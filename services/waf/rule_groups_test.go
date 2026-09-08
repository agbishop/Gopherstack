package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func wafCreateRuleGroup(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateRuleGroup", map[string]any{
		"ChangeToken": token,
		"Name":        name,
		"MetricName":  name + "Metric",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	rgMap, ok := resp["RuleGroup"].(map[string]any)
	require.True(t, ok)
	id, ok := rgMap["RuleGroupId"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func TestRuleGroupLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{"single rule group CRUD"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)

			// Prerequisite: create a regular rule to activate
			ruleID := wafCreateRule(t, h, "TestRule")

			// Create rule group
			rgID := wafCreateRuleGroup(t, h, "MyGroup")
			assert.Equal(t, 1, waf.RuleGroupCount(h.Backend.(*waf.InMemoryBackend)))

			// Get
			rec := wafDo(t, h, "GetRuleGroup", map[string]any{"RuleGroupId": rgID})
			require.Equal(t, http.StatusOK, rec.Code)
			var getResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
			rgMap := getResp["RuleGroup"].(map[string]any)
			assert.Equal(t, "MyGroup", rgMap["Name"])

			// Update: insert activated rule
			token := wafGetToken(t, h)
			rec = wafDo(t, h, "UpdateRuleGroup", map[string]any{
				"ChangeToken": token,
				"RuleGroupId": rgID,
				"Updates": []map[string]any{
					{
						"Action": "INSERT",
						"ActivatedRule": map[string]any{
							"RuleId":   ruleID,
							"Priority": 1,
							"Type":     "REGULAR",
							"Action":   map[string]any{"Type": "BLOCK"},
						},
					},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			// ListActivatedRulesInRuleGroup
			rec = wafDo(t, h, "ListActivatedRulesInRuleGroup", map[string]any{
				"RuleGroupId": rgID,
			})
			require.Equal(t, http.StatusOK, rec.Code)
			var listARResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listARResp))
			activated := listARResp["ActivatedRules"].([]any)
			assert.Len(t, activated, 1)

			// Delete activated rule
			token = wafGetToken(t, h)
			rec = wafDo(t, h, "UpdateRuleGroup", map[string]any{
				"ChangeToken": token,
				"RuleGroupId": rgID,
				"Updates": []map[string]any{
					{
						"Action": "DELETE",
						"ActivatedRule": map[string]any{
							"RuleId":   ruleID,
							"Priority": 1,
							"Type":     "REGULAR",
							"Action":   map[string]any{"Type": "BLOCK"},
						},
					},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			// List rule groups
			rec = wafDo(t, h, "ListRuleGroups", nil)
			require.Equal(t, http.StatusOK, rec.Code)
			var listRGResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listRGResp))
			groups := listRGResp["RuleGroups"].([]any)
			assert.Len(t, groups, 1)

			// Delete rule group
			token = wafGetToken(t, h)
			rec = wafDo(t, h, "DeleteRuleGroup", map[string]any{
				"ChangeToken": token,
				"RuleGroupId": rgID,
			})
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, 0, waf.RuleGroupCount(h.Backend.(*waf.InMemoryBackend)))
		})
	}
}

func TestRuleGroupNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{name: "GetRuleGroup not-found", action: "GetRuleGroup", body: map[string]any{"RuleGroupId": "no-such-id"}},
		{
			name:   "DeleteRuleGroup not-found",
			action: "DeleteRuleGroup",
			body:   map[string]any{"ChangeToken": "t", "RuleGroupId": "no-such-id"},
		},
		{
			name:   "ListActivatedRulesInRuleGroup not-found",
			action: "ListActivatedRulesInRuleGroup",
			body:   map[string]any{"RuleGroupId": "no-such-id"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)
			rec := wafDo(t, h, tc.action, tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestUpdateRuleGroup_RejectsDuplicateRuleId(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	ruleID := wafCreateRule(t, h, "DupRule")
	rgID := wafCreateRuleGroup(t, h, "DupGroup")

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "UpdateRuleGroup", map[string]any{
		"ChangeToken": token,
		"RuleGroupId": rgID,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"ActivatedRule": map[string]any{
					"RuleId":   ruleID,
					"Priority": 1,
					"Type":     "REGULAR",
					"Action":   map[string]any{"Type": "BLOCK"},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateRuleGroup", map[string]any{
		"ChangeToken": token,
		"RuleGroupId": rgID,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"ActivatedRule": map[string]any{
					"RuleId":   ruleID,
					"Priority": 2,
					"Type":     "REGULAR",
					"Action":   map[string]any{"Type": "BLOCK"},
				},
			},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"inserting the same RuleId twice into a RuleGroup must be rejected: "+
			"a duplicate RuleId in the group breaks ListActivatedRulesInRuleGroup's "+
			"RuleId-marker pagination")
	assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()),
		"real AWS WAF models a duplicate-insert as \"nothing to do\" "+
			"(types/errors.go WAFInvalidOperationException), not WAFInvalidParameterException")

	rec = wafDo(t, h, "ListActivatedRulesInRuleGroup", map[string]any{"RuleGroupId": rgID})
	require.Equal(t, http.StatusOK, rec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	activated, _ := listResp["ActivatedRules"].([]any)
	assert.Len(t, activated, 1, "group must still contain only the one successfully-inserted rule")
}

func wafActivateRuleUpdate(action, ruleID string, priority int) map[string]any {
	return map[string]any{
		"Action": action,
		"ActivatedRule": map[string]any{
			"RuleId":   ruleID,
			"Priority": priority,
			"Type":     "REGULAR",
			"Action":   map[string]any{"Type": "BLOCK"},
		},
	}
}

func TestUpdateRuleGroup_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		ruleID := wafCreateRule(t, h, "NoOpRuleInsert")
		rgID := wafCreateRuleGroup(t, h, "NoOpGroupInsert")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateRuleGroup", map[string]any{
			"ChangeToken": token,
			"RuleGroupId": rgID,
			"Updates":     []map[string]any{wafActivateRuleUpdate("INSERT", ruleID, 1)},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateRuleGroup", map[string]any{
			"ChangeToken": token,
			"RuleGroupId": rgID,
			"Updates":     []map[string]any{wafActivateRuleUpdate("INSERT", ruleID, 2)},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		ruleID := wafCreateRule(t, h, "NoOpRuleDelete")
		rgID := wafCreateRuleGroup(t, h, "NoOpGroupDelete")

		// The group never contained ruleID, so this DELETE is itself a no-op.
		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateRuleGroup", map[string]any{
			"ChangeToken": token,
			"RuleGroupId": rgID,
			"Updates":     []map[string]any{wafActivateRuleUpdate("DELETE", ruleID, 1)},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestListSubscribedRuleGroups(t *testing.T) {
	t.Parallel()

	t.Run("returns empty list", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		rec := wafDo(t, h, "ListSubscribedRuleGroups", nil)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		groups := resp["RuleGroups"].([]any)
		assert.Empty(t, groups)
	})
}
