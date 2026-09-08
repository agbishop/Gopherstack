package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func wafCreateWebACL(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateWebACL", map[string]any{
		"ChangeToken":   token,
		"Name":          name,
		"MetricName":    name + "Metric",
		"DefaultAction": map[string]any{"Type": "ALLOW"},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	aclMap, ok := resp["WebACL"].(map[string]any)
	require.True(t, ok)
	id, ok := aclMap["WebACLId"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func TestWAF_WebACL_CreateGetDeleteList(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)

	// Create
	id := wafCreateWebACL(t, h, "my-acl")
	assert.Equal(t, 1, waf.WebACLCount(h.Backend.(*waf.InMemoryBackend)))

	// Get
	rec := wafDo(t, h, "GetWebACL", map[string]any{"WebACLId": id})
	require.Equal(t, http.StatusOK, rec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
	acl := getResp["WebACL"].(map[string]any)
	assert.Equal(t, "my-acl", acl["Name"])
	assert.Equal(t, id, acl["WebACLId"])
	assert.NotEmpty(t, acl["WebACLArn"])

	// DefaultAction round-trip
	da := acl["DefaultAction"].(map[string]any)
	assert.Equal(t, "ALLOW", da["Type"])

	// List
	rec = wafDo(t, h, "ListWebACLs", map[string]any{"Limit": 100})
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	acls := listResp["WebACLs"].([]any)
	assert.Len(t, acls, 1)

	// Delete
	token := wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteWebACL", map[string]any{
		"ChangeToken": token,
		"WebACLId":    id,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, waf.WebACLCount(h.Backend.(*waf.InMemoryBackend)))
}

func TestWAF_WebACL_DefaultAction_BLOCK(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateWebACL", map[string]any{
		"ChangeToken":   token,
		"Name":          "block-acl",
		"MetricName":    "blockAclMetric",
		"DefaultAction": map[string]any{"Type": "BLOCK"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	acl := resp["WebACL"].(map[string]any)
	da := acl["DefaultAction"].(map[string]any)
	assert.Equal(t, "BLOCK", da["Type"])
}

func TestWAF_WebACL_UpdateDefaultAction(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	id := wafCreateWebACL(t, h, "update-acl")
	token := wafGetToken(t, h)

	rec := wafDo(t, h, "UpdateWebACL", map[string]any{
		"ChangeToken":   token,
		"WebACLId":      id,
		"DefaultAction": map[string]any{"Type": "BLOCK"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetWebACL", map[string]any{"WebACLId": id})
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	acl := resp["WebACL"].(map[string]any)
	da := acl["DefaultAction"].(map[string]any)
	assert.Equal(t, "BLOCK", da["Type"])
}

func TestWAF_WebACL_UpdateRules_InsertAndDelete(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	aclID := wafCreateWebACL(t, h, "rules-acl")
	ruleID := wafCreateRule(t, h, "rule-1")

	// Insert rule into WebACL
	token := wafGetToken(t, h)
	rec := wafDo(t, h, "UpdateWebACL", map[string]any{
		"ChangeToken": token,
		"WebACLId":    aclID,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"ActivatedRule": map[string]any{
					"Priority": 1,
					"RuleId":   ruleID,
					"Action":   map[string]any{"Type": "BLOCK"},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify rule present
	rec = wafDo(t, h, "GetWebACL", map[string]any{"WebACLId": aclID})
	var getResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
	acl := getResp["WebACL"].(map[string]any)
	rules := acl["Rules"].([]any)
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]any)
	assert.Equal(t, ruleID, rule["RuleId"])
	assert.EqualValues(t, 1, rule["Priority"])

	// Delete rule from WebACL
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateWebACL", map[string]any{
		"ChangeToken": token,
		"WebACLId":    aclID,
		"Updates": []map[string]any{
			{
				"Action": "DELETE",
				"ActivatedRule": map[string]any{
					"Priority": 1,
					"RuleId":   ruleID,
					"Action":   map[string]any{"Type": "BLOCK"},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetWebACL", map[string]any{"WebACLId": aclID})
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
	acl = getResp["WebACL"].(map[string]any)
	rules = acl["Rules"].([]any)
	assert.Empty(t, rules)
}

func TestWAF_WebACL_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		aclID := wafCreateWebACL(t, h, "noop-insert-acl")
		ruleID := wafCreateRule(t, h, "noop-insert-rule")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateWebACL", map[string]any{
			"ChangeToken": token,
			"WebACLId":    aclID,
			"Updates":     []map[string]any{wafActivateRuleUpdate("INSERT", ruleID, 1)},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateWebACL", map[string]any{
			"ChangeToken": token,
			"WebACLId":    aclID,
			"Updates":     []map[string]any{wafActivateRuleUpdate("INSERT", ruleID, 2)},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		aclID := wafCreateWebACL(t, h, "noop-delete-acl")
		ruleID := wafCreateRule(t, h, "noop-delete-rule")

		// The WebACL never contained ruleID, so this DELETE is itself a no-op.
		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateWebACL", map[string]any{
			"ChangeToken": token,
			"WebACLId":    aclID,
			"Updates":     []map[string]any{wafActivateRuleUpdate("DELETE", ruleID, 1)},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestWAF_WebACL_RulesPrioritySorted(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	aclID := wafCreateWebACL(t, h, "sorted-acl")
	rule1 := wafCreateRule(t, h, "high-pri")
	rule2 := wafCreateRule(t, h, "low-pri")

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "UpdateWebACL", map[string]any{
		"ChangeToken": token,
		"WebACLId":    aclID,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"ActivatedRule": map[string]any{
					"Priority": 10,
					"RuleId":   rule2,
					"Action":   map[string]any{"Type": "ALLOW"},
				},
			},
			{
				"Action": "INSERT",
				"ActivatedRule": map[string]any{
					"Priority": 1,
					"RuleId":   rule1,
					"Action":   map[string]any{"Type": "BLOCK"},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetWebACL", map[string]any{"WebACLId": aclID})
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	rules := resp["WebACL"].(map[string]any)["Rules"].([]any)
	require.Len(t, rules, 2)
	assert.EqualValues(t, 1, rules[0].(map[string]any)["Priority"])
	assert.EqualValues(t, 10, rules[1].(map[string]any)["Priority"])
}

func TestWAF_WebACL_ChangeTokenRoundTrip(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateWebACL", map[string]any{
		"ChangeToken":   token,
		"Name":          "token-acl",
		"MetricName":    "tokenAclMetric",
		"DefaultAction": map[string]any{"Type": "ALLOW"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, token, resp["ChangeToken"])
}

func TestWAF_MultipleWebACLs_List(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	wafCreateWebACL(t, h, "acl-1")
	wafCreateWebACL(t, h, "acl-2")
	wafCreateWebACL(t, h, "acl-3")

	rec := wafDo(t, h, "ListWebACLs", map[string]any{"Limit": 100})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	acls := resp["WebACLs"].([]any)
	assert.Len(t, acls, 3)
}

func TestWebACL_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     func(token string) map[string]any
		name     string
		action   string
		wantType string
	}{
		{
			name:     "GetWebACL",
			action:   "GetWebACL",
			wantType: "WAFNonexistentItemException",
			body:     func(string) map[string]any { return map[string]any{"WebACLId": "nonexistent"} },
		},
		{
			name:   "UpdateWebACL",
			action: "UpdateWebACL",
			body: func(token string) map[string]any {
				return map[string]any{"ChangeToken": token, "WebACLId": "nonexistent"}
			},
		},
		{
			name:     "DeleteWebACL",
			action:   "DeleteWebACL",
			wantType: "WAFNonexistentItemException",
			body: func(token string) map[string]any {
				return map[string]any{"ChangeToken": token, "WebACLId": "nonexistent"}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)
			token := wafGetToken(t, h)
			rec := wafDo(t, h, tc.action, tc.body(token))
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			if tc.wantType != "" {
				assert.Equal(t, tc.wantType, errType(t, rec.Body.Bytes()))
			}
		})
	}
}

func TestCreateWebACLMigrationStack(t *testing.T) {
	t.Parallel()

	t.Run("returns S3 URL", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		aclID := wafCreateWebACL(t, h, "MigrateACL")

		rec := wafDo(t, h, "CreateWebACLMigrationStack", map[string]any{
			"WebACLId":              aclID,
			"S3BucketName":          "my-migration-bucket",
			"IgnoreUnsupportedType": true,
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		s3URL, ok := resp["S3ObjectUrl"].(string)
		require.True(t, ok)
		assert.Contains(t, s3URL, "my-migration-bucket")
		assert.Contains(t, s3URL, aclID)
	})
}
