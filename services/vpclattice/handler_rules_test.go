package vpclattice_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRule_CRUD tests listener rules.
func TestRule_CRUD(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)

	// setup
	recSvc := doRequest(t, h, http.MethodPost, "/services", map[string]any{"name": "svc-r"})
	require.Equal(t, http.StatusCreated, recSvc.Code)
	svcID, _ := parseBody(t, recSvc)["id"].(string)

	recL := doRequest(t, h, http.MethodPost, "/services/"+svcID+"/listeners", map[string]any{
		"name":     "l1",
		"protocol": "HTTP",
		"defaultAction": map[string]any{
			"fixedResponse": map[string]any{"statusCode": 404},
		},
	})
	require.Equal(t, http.StatusCreated, recL.Code)
	listenerID, _ := parseBody(t, recL)["id"].(string)

	// create target group for forward rule
	recTG := doRequest(t, h, http.MethodPost, "/targetgroups", map[string]any{
		"name": "tg1",
		"type": "INSTANCE",
		"config": map[string]any{
			"protocol":      "HTTP",
			"port":          80,
			"vpcIdentifier": "vpc-123",
		},
	})
	require.Equal(t, http.StatusCreated, recTG.Code)
	tgID, _ := parseBody(t, recTG)["id"].(string)

	// create rule with forward action
	rec := doRequest(
		t,
		h,
		http.MethodPost,
		"/services/"+svcID+"/listeners/"+listenerID+"/rules",
		map[string]any{
			"name":     "rule1",
			"priority": 10,
			"action": map[string]any{
				"forward": map[string]any{
					"targetGroups": []any{
						map[string]any{"targetGroupIdentifier": tgID, "weight": 100},
					},
				},
			},
			"match": map[string]any{
				"httpMatch": map[string]any{
					"method": "GET",
					"pathMatch": map[string]any{
						"match":         map[string]any{"exact": "/api"},
						"caseSensitive": true,
					},
				},
			},
		},
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	rule := parseBody(t, rec)
	ruleID, _ := rule["id"].(string)
	require.NotEmpty(t, ruleID)
	assert.InDelta(t, float64(10), rule["priority"], 0)

	// get -- match.httpMatch.pathMatch must round-trip under the real wire
	// key ("pathMatch", not "path"); previously silently discarded on
	// create since extractPathMatch/ruleMatchToJSON both used "path".
	rec = doRequest(
		t,
		h,
		http.MethodGet,
		"/services/"+svcID+"/listeners/"+listenerID+"/rules/"+ruleID,
		nil,
	)
	assert.Equal(t, http.StatusOK, rec.Code)
	got := parseBody(t, rec)
	match, _ := got["match"].(map[string]any)
	httpMatch, _ := match["httpMatch"].(map[string]any)
	pathMatch, _ := httpMatch["pathMatch"].(map[string]any)
	require.NotNil(t, pathMatch, "pathMatch must round-trip under its real wire key")
	assert.Equal(t, true, pathMatch["caseSensitive"])
	pathMatchMatch, _ := pathMatch["match"].(map[string]any)
	assert.Equal(t, "/api", pathMatchMatch["exact"])

	// list (includes default rule)
	rec = doRequest(t, h, http.MethodGet, "/services/"+svcID+"/listeners/"+listenerID+"/rules", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	list := parseBody(t, rec)
	items, _ := list["items"].([]any)
	assert.Len(t, items, 2) // default + created

	// update
	rec = doRequest(
		t,
		h,
		http.MethodPatch,
		"/services/"+svcID+"/listeners/"+listenerID+"/rules/"+ruleID,
		map[string]any{
			"priority": 20,
		},
	)
	assert.Equal(t, http.StatusOK, rec.Code)
	updated := parseBody(t, rec)
	assert.InDelta(t, float64(20), updated["priority"], 0)

	// delete
	rec = doRequest(
		t,
		h,
		http.MethodDelete,
		"/services/"+svcID+"/listeners/"+listenerID+"/rules/"+ruleID,
		nil,
	)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// list now has only default rule
	rec = doRequest(t, h, http.MethodGet, "/services/"+svcID+"/listeners/"+listenerID+"/rules", nil)
	list = parseBody(t, rec)
	items, _ = list["items"].([]any)
	assert.Len(t, items, 1)
}

// TestUpdateRule_RejectsDefaultRule verifies UpdateRule rejects modifying a
// listener's default rule with a 400 ValidationException, matching real
// AWS's UpdateRule doc comment ("You can't modify a default listener rule.
// To modify a default listener rule, use UpdateListener." --
// aws-sdk-go-v2/service/vpclattice's api_op_UpdateRule.go). A non-default
// rule on the same listener must still be updatable, proving the guard
// targets only the default rule.
func TestUpdateRule_RejectsDefaultRule(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)

	recSvc := doRequest(t, h, http.MethodPost, "/services", map[string]any{"name": "svc-default-rule"})
	require.Equal(t, http.StatusCreated, recSvc.Code)
	svcID, _ := parseBody(t, recSvc)["id"].(string)

	recL := doRequest(t, h, http.MethodPost, "/services/"+svcID+"/listeners", map[string]any{
		"name":     "l1",
		"protocol": "HTTP",
		"defaultAction": map[string]any{
			"fixedResponse": map[string]any{"statusCode": 404},
		},
	})
	require.Equal(t, http.StatusCreated, recL.Code)
	listenerID, _ := parseBody(t, recL)["id"].(string)

	recRule := doRequest(
		t,
		h,
		http.MethodPost,
		"/services/"+svcID+"/listeners/"+listenerID+"/rules",
		map[string]any{
			"name":     "rule1",
			"priority": 10,
			"action": map[string]any{
				"fixedResponse": map[string]any{"statusCode": 200},
			},
			"match": map[string]any{
				"httpMatch": map[string]any{
					"pathMatch": map[string]any{"match": map[string]any{"exact": "/api"}},
				},
			},
		},
	)
	require.Equal(t, http.StatusCreated, recRule.Code)
	nonDefaultRuleID, _ := parseBody(t, recRule)["id"].(string)

	listRec := doRequest(t, h, http.MethodGet, "/services/"+svcID+"/listeners/"+listenerID+"/rules", nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	items, _ := parseBody(t, listRec)["items"].([]any)
	require.Len(t, items, 2)

	var defaultRuleID string

	for _, it := range items {
		m, _ := it.(map[string]any)
		if isDefault, _ := m["isDefault"].(bool); isDefault {
			defaultRuleID, _ = m["id"].(string)
		}
	}

	require.NotEmpty(t, defaultRuleID, "expected an auto-created default rule")

	rec := doRequest(
		t,
		h,
		http.MethodPatch,
		"/services/"+svcID+"/listeners/"+listenerID+"/rules/"+defaultRuleID,
		map[string]any{"priority": 5},
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "modifying the default rule must be rejected")

	rec = doRequest(
		t,
		h,
		http.MethodPatch,
		"/services/"+svcID+"/listeners/"+listenerID+"/rules/"+nonDefaultRuleID,
		map[string]any{"priority": 5},
	)
	assert.Equal(t, http.StatusOK, rec.Code, "non-default rule updates must still succeed")
}

// TestBatchUpdateRule tests batch rule updates.
func TestBatchUpdateRule(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)

	// setup
	recSvc := doRequest(t, h, http.MethodPost, "/services", map[string]any{"name": "svc-bu"})
	require.Equal(t, http.StatusCreated, recSvc.Code)
	svcID, _ := parseBody(t, recSvc)["id"].(string)

	recL := doRequest(t, h, http.MethodPost, "/services/"+svcID+"/listeners", map[string]any{
		"name":     "l-bu",
		"protocol": "HTTP",
		"defaultAction": map[string]any{
			"fixedResponse": map[string]any{"statusCode": 404},
		},
	})
	require.Equal(t, http.StatusCreated, recL.Code)
	listenerID, _ := parseBody(t, recL)["id"].(string)

	rec := doRequest(
		t,
		h,
		http.MethodPost,
		"/services/"+svcID+"/listeners/"+listenerID+"/rules",
		map[string]any{
			"name":     "r1",
			"priority": 10,
			"action":   map[string]any{"fixedResponse": map[string]any{"statusCode": 200}},
		},
	)
	require.Equal(t, http.StatusCreated, rec.Code)
	ruleID, _ := parseBody(t, rec)["id"].(string)

	// batch update
	rec = doRequest(
		t,
		h,
		http.MethodPatch,
		"/services/"+svcID+"/listeners/"+listenerID+"/rules",
		map[string]any{
			"rules": []any{
				map[string]any{"ruleIdentifier": ruleID, "priority": 50},
				map[string]any{"ruleIdentifier": "rule-notexist", "priority": 99},
			},
		},
	)
	assert.Equal(t, http.StatusOK, rec.Code)
	resp := parseBody(t, rec)
	successful, _ := resp["successful"].([]any)
	unsuccessful, _ := resp["unsuccessful"].([]any)
	assert.Len(t, successful, 1)
	assert.Len(t, unsuccessful, 1)
}

// TestBatchUpdateRuleFailureUsesFailureCodeFailureMessageKeys mirrors
// TestTargetFailureUsesFailureCodeFailureMessageKeys for RuleUpdateFailure,
// whose real wire keys are also "failureCode"/"failureMessage" rather than
// "code"/"message".
func TestBatchUpdateRuleFailureUsesFailureCodeFailureMessageKeys(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	recSvc := doRequest(t, h, http.MethodPost, "/services", map[string]any{"name": "svc-batch-fail-keys"})
	require.Equal(t, http.StatusCreated, recSvc.Code)
	svcID, _ := parseBody(t, recSvc)["id"].(string)

	recL := doRequest(t, h, http.MethodPost, "/services/"+svcID+"/listeners", map[string]any{
		"name":     "l1",
		"protocol": "HTTP",
		"defaultAction": map[string]any{
			"fixedResponse": map[string]any{"statusCode": 404},
		},
	})
	require.Equal(t, http.StatusCreated, recL.Code)
	listenerID, _ := parseBody(t, recL)["id"].(string)

	rec := doRequest(
		t,
		h,
		http.MethodPatch,
		"/services/"+svcID+"/listeners/"+listenerID+"/rules",
		map[string]any{
			"rules": []any{
				map[string]any{"ruleIdentifier": "rule-notexist", "priority": 99},
			},
		},
	)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseBody(t, rec)
	unsuccessful, _ := resp["unsuccessful"].([]any)
	require.Len(t, unsuccessful, 1)

	failure, _ := unsuccessful[0].(map[string]any)
	assert.NotEmpty(t, failure["failureCode"], "RuleUpdateFailure must use failureCode, not code")
	assert.NotEmpty(t, failure["failureMessage"], "RuleUpdateFailure must use failureMessage, not message")
	assert.Nil(t, failure["code"])
	assert.Nil(t, failure["message"])
}

// TestRuleSummaryIncludesTimestamps verifies that ListRules summary entries
// include createdAt/lastUpdatedAt, matching the real RuleSummary shape. The
// emulator previously omitted both fields.
func TestRuleSummaryIncludesTimestamps(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	recSvc := doRequest(t, h, http.MethodPost, "/services", map[string]any{"name": "svc-rule-summary-ts"})
	require.Equal(t, http.StatusCreated, recSvc.Code)
	svcID, _ := parseBody(t, recSvc)["id"].(string)

	recL := doRequest(t, h, http.MethodPost, "/services/"+svcID+"/listeners", map[string]any{
		"name":     "l1",
		"protocol": "HTTP",
		"defaultAction": map[string]any{
			"fixedResponse": map[string]any{"statusCode": 404},
		},
	})
	require.Equal(t, http.StatusCreated, recL.Code)
	listenerID, _ := parseBody(t, recL)["id"].(string)

	listRec := doRequest(t, h, http.MethodGet, "/services/"+svcID+"/listeners/"+listenerID+"/rules", nil)
	require.Equal(t, http.StatusOK, listRec.Code)
	items, _ := parseBody(t, listRec)["items"].([]any)
	require.Len(t, items, 1, "expected the auto-created default rule")

	summary, _ := items[0].(map[string]any)
	assert.NotEmpty(t, summary["createdAt"])
	assert.NotEmpty(t, summary["lastUpdatedAt"])
}
