package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func wafCreateRateBasedRule(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateRateBasedRule", map[string]any{
		"ChangeToken": token,
		"Name":        name,
		"MetricName":  name + "Metric",
		"RateKey":     "IP",
		"RateLimit":   int64(2000),
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ruleMap, ok := resp["Rule"].(map[string]any)
	require.True(t, ok)
	id, ok := ruleMap["RuleId"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func TestRateBasedRuleLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{"single rate-based rule CRUD"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)

			// Create
			ruleID := wafCreateRateBasedRule(t, h, "BlockBots")
			assert.Equal(t, 1, waf.RateBasedRuleCount(h.Backend.(*waf.InMemoryBackend)))

			// Get
			rec := wafDo(t, h, "GetRateBasedRule", map[string]any{"RuleId": ruleID})
			require.Equal(t, http.StatusOK, rec.Code)

			var getResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
			ruleMap := getResp["Rule"].(map[string]any)
			assert.Equal(t, "BlockBots", ruleMap["Name"])
			assert.Equal(t, "IP", ruleMap["RateKey"])
			assert.InEpsilon(t, float64(2000), ruleMap["RateLimit"], 0.001)

			// Update: add predicate
			token := wafGetToken(t, h)
			rec = wafDo(t, h, "UpdateRateBasedRule", map[string]any{
				"ChangeToken": token,
				"RuleId":      ruleID,
				"RateLimit":   int64(3000),
				"Updates": []map[string]any{
					{
						"Action": "INSERT",
						"Predicate": map[string]any{
							"DataId":  "some-ip-set-id",
							"Type":    "IPMatch",
							"Negated": false,
						},
					},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			// Get to verify updates
			rec = wafDo(t, h, "GetRateBasedRule", map[string]any{"RuleId": ruleID})
			require.Equal(t, http.StatusOK, rec.Code)
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
			ruleMap = getResp["Rule"].(map[string]any)
			assert.InEpsilon(t, float64(3000), ruleMap["RateLimit"], 0.001)
			preds := ruleMap["MatchPredicates"].([]any)
			assert.Len(t, preds, 1)

			// Delete predicate
			token = wafGetToken(t, h)
			rec = wafDo(t, h, "UpdateRateBasedRule", map[string]any{
				"ChangeToken": token,
				"RuleId":      ruleID,
				"RateLimit":   int64(0),
				"Updates": []map[string]any{
					{
						"Action": "DELETE",
						"Predicate": map[string]any{
							"DataId":  "some-ip-set-id",
							"Type":    "IPMatch",
							"Negated": false,
						},
					},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			// List
			rec = wafDo(t, h, "ListRateBasedRules", nil)
			require.Equal(t, http.StatusOK, rec.Code)
			var listResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
			rules := listResp["Rules"].([]any)
			assert.Len(t, rules, 1)

			// GetManagedKeys
			rec = wafDo(t, h, "GetRateBasedRuleManagedKeys", map[string]any{"RuleId": ruleID})
			require.Equal(t, http.StatusOK, rec.Code)
			var mkResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &mkResp))
			assert.NotNil(t, mkResp["ManagedKeys"])

			// Delete
			token = wafGetToken(t, h)
			rec = wafDo(t, h, "DeleteRateBasedRule", map[string]any{
				"ChangeToken": token,
				"RuleId":      ruleID,
			})
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, 0, waf.RateBasedRuleCount(h.Backend.(*waf.InMemoryBackend)))
		})
	}
}

func TestRateBasedRule_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	predicate := map[string]any{"DataId": "some-ip-set-id", "Type": "IPMatch", "Negated": false}

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		ruleID := wafCreateRateBasedRule(t, h, "noop-insert-rbr")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateRateBasedRule", map[string]any{
			"ChangeToken": token,
			"RuleId":      ruleID,
			"RateLimit":   int64(0),
			"Updates":     []map[string]any{{"Action": "INSERT", "Predicate": predicate}},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateRateBasedRule", map[string]any{
			"ChangeToken": token,
			"RuleId":      ruleID,
			"RateLimit":   int64(0),
			"Updates":     []map[string]any{{"Action": "INSERT", "Predicate": predicate}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		ruleID := wafCreateRateBasedRule(t, h, "noop-delete-rbr")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateRateBasedRule", map[string]any{
			"ChangeToken": token,
			"RuleId":      ruleID,
			"RateLimit":   int64(0),
			"Updates":     []map[string]any{{"Action": "DELETE", "Predicate": predicate}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestRateBasedRuleNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{name: "GetRateBasedRule not-found", action: "GetRateBasedRule", body: map[string]any{"RuleId": "no-such-id"}},
		{
			name:   "GetRateBasedRuleManagedKeys not-found",
			action: "GetRateBasedRuleManagedKeys",
			body:   map[string]any{"RuleId": "no-such-id"},
		},
		{
			name:   "DeleteRateBasedRule not-found",
			action: "DeleteRateBasedRule",
			body:   map[string]any{"ChangeToken": "t", "RuleId": "no-such-id"},
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
