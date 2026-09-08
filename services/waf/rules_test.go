package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func wafCreateRule(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateRule", map[string]any{
		"ChangeToken": token,
		"Name":        name,
		"MetricName":  name + "Metric",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ruleMap := resp["Rule"].(map[string]any)
	id := ruleMap["RuleId"].(string)
	require.NotEmpty(t, id)

	return id
}

func TestWAF_Rule_CreateGetDeleteList(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)

	id := wafCreateRule(t, h, "my-rule")
	assert.Equal(t, 1, waf.RuleCount(h.Backend.(*waf.InMemoryBackend)))

	// Get
	rec := wafDo(t, h, "GetRule", map[string]any{"RuleId": id})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	rule := resp["Rule"].(map[string]any)
	assert.Equal(t, "my-rule", rule["Name"])
	assert.Equal(t, id, rule["RuleId"])

	// List
	rec = wafDo(t, h, "ListRules", map[string]any{"Limit": 100})
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	rules := listResp["Rules"].([]any)
	assert.Len(t, rules, 1)

	// Delete
	token := wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteRule", map[string]any{
		"ChangeToken": token,
		"RuleId":      id,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, waf.RuleCount(h.Backend.(*waf.InMemoryBackend)))
}

func TestWAF_Rule_UpdatePredicates_InsertDelete(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	id := wafCreateRule(t, h, "pred-rule")
	ipSetID := wafCreateIPSet(t, h, "my-ipset")

	// Insert predicate
	token := wafGetToken(t, h)
	rec := wafDo(t, h, "UpdateRule", map[string]any{
		"ChangeToken": token,
		"RuleId":      id,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"Predicate": map[string]any{
					"Type":    "IPMatch",
					"DataId":  ipSetID,
					"Negated": false,
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify predicate
	rec = wafDo(t, h, "GetRule", map[string]any{"RuleId": id})
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	rule := resp["Rule"].(map[string]any)
	predicates := rule["Predicates"].([]any)
	require.Len(t, predicates, 1)
	pred := predicates[0].(map[string]any)
	assert.Equal(t, "IPMatch", pred["Type"])
	assert.Equal(t, ipSetID, pred["DataId"])

	// Delete predicate
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateRule", map[string]any{
		"ChangeToken": token,
		"RuleId":      id,
		"Updates": []map[string]any{
			{
				"Action": "DELETE",
				"Predicate": map[string]any{
					"Type":    "IPMatch",
					"DataId":  ipSetID,
					"Negated": false,
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetRule", map[string]any{"RuleId": id})
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	rule = resp["Rule"].(map[string]any)
	predicates = rule["Predicates"].([]any)
	assert.Empty(t, predicates)
}

func TestWAF_Rule_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateRule(t, h, "noop-insert-rule")
		ipSetID := wafCreateIPSet(t, h, "noop-insert-ipset")
		predicate := map[string]any{"Type": "IPMatch", "DataId": ipSetID, "Negated": false}

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateRule", map[string]any{
			"ChangeToken": token,
			"RuleId":      id,
			"Updates":     []map[string]any{{"Action": "INSERT", "Predicate": predicate}},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateRule", map[string]any{
			"ChangeToken": token,
			"RuleId":      id,
			"Updates":     []map[string]any{{"Action": "INSERT", "Predicate": predicate}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateRule(t, h, "noop-delete-rule")
		ipSetID := wafCreateIPSet(t, h, "noop-delete-ipset")
		predicate := map[string]any{"Type": "IPMatch", "DataId": ipSetID, "Negated": false}

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateRule", map[string]any{
			"ChangeToken": token,
			"RuleId":      id,
			"Updates":     []map[string]any{{"Action": "DELETE", "Predicate": predicate}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestRule_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   func(token string) map[string]any
		name   string
		action string
	}{
		{
			name:   "GetRule",
			action: "GetRule",
			body:   func(string) map[string]any { return map[string]any{"RuleId": "nonexistent"} },
		},
		{
			name:   "UpdateRule",
			action: "UpdateRule",
			body: func(token string) map[string]any {
				return map[string]any{"ChangeToken": token, "RuleId": "nonexistent"}
			},
		},
		{
			name:   "DeleteRule",
			action: "DeleteRule",
			body: func(token string) map[string]any {
				return map[string]any{"ChangeToken": token, "RuleId": "nonexistent"}
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
		})
	}
}
