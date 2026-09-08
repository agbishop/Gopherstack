package codecommit_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_GetPullRequestApprovalStates(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-1")

	rec := doRequest(t, h, "GetPullRequestApprovalStates", map[string]any{
		"pullRequestId": prID,
		"revisionId":    "rev1",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp["approvals"])
}

// TestHandler_RevisionIDRequired verifies revisionId is enforced as
// required on the five pull-request-approval operations that all declare
// it "This member is required" in the real SDK (codecommit@v1.36.4:
// api_op_GetPullRequestApprovalStates.go, api_op_GetPullRequestOverrideState.go,
// api_op_OverridePullRequestApprovalRules.go,
// api_op_UpdatePullRequestApprovalState.go,
// api_op_EvaluatePullRequestApprovalRules.go). All five previously decoded
// revisionId off the wire and never validated or used it at all.
func TestHandler_RevisionIDRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		extra  map[string]any
		name   string
		action string
	}{
		{name: "get_approval_states", action: "GetPullRequestApprovalStates"},
		{name: "get_override_state", action: "GetPullRequestOverrideState"},
		{
			name:   "override_approval_rules",
			action: "OverridePullRequestApprovalRules",
			extra:  map[string]any{"overrideStatus": "OVERRIDE"},
		},
		{
			name:   "update_approval_state",
			action: "UpdatePullRequestApprovalState",
			extra:  map[string]any{"approvalState": "APPROVE"},
		},
		{name: "evaluate_approval_rules", action: "EvaluatePullRequestApprovalRules"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			prID := setupPR(t, h, "repo")

			body := map[string]any{"pullRequestId": prID}
			maps.Copy(body, tt.extra)

			rec := doRequest(t, h, tt.action, body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_GetPullRequestApprovalStates_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		approveUsers int
		wantCode     int
	}{
		{name: "no_approvals", approveUsers: 0, wantCode: http.StatusOK},
		{name: "one_approval", approveUsers: 1, wantCode: http.StatusOK},
		{name: "three_approvals", approveUsers: 3, wantCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			prID := setupPR(t, h, "repo")

			for range tt.approveUsers {
				doRequest(t, h, "UpdatePullRequestApprovalState", map[string]any{
					"pullRequestId": prID,
					"revisionId":    "rev-1",
					"approvalState": "APPROVE",
				})
			}

			rec := doRequest(t, h, "GetPullRequestApprovalStates", map[string]any{
				"pullRequestId": prID,
				"revisionId":    "rev-1",
			})
			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			approvals := resp["approvals"].([]any)
			// Each UpdatePullRequestApprovalState with the same (empty) userARN just overwrites, so max 1.
			if tt.approveUsers > 0 {
				assert.NotEmpty(t, approvals)
			} else {
				assert.Empty(t, approvals)
			}
		})
	}
}

func TestHandler_UpdatePullRequestApprovalState(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-2")

	rec := doRequest(t, h, "UpdatePullRequestApprovalState", map[string]any{
		"pullRequestId": prID,
		"revisionId":    "rev1",
		"approvalState": "APPROVE",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_GetPullRequestOverrideState(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-3")

	rec := doRequest(t, h, "GetPullRequestOverrideState", map[string]any{
		"pullRequestId": prID,
		"revisionId":    "rev1",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["overridden"])
}

func TestHandler_GetPullRequestOverrideState_AfterOverride(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "repo")

	// Initially not overridden.
	rec := doRequest(t, h, "GetPullRequestOverrideState", map[string]any{
		"pullRequestId": prID,
		"revisionId":    "rev-1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["overridden"])

	// Override it.
	doRequest(t, h, "OverridePullRequestApprovalRules", map[string]any{
		"pullRequestId":  prID,
		"revisionId":     "rev-1",
		"overrideStatus": "OVERRIDE",
	})

	rec = doRequest(t, h, "GetPullRequestOverrideState", map[string]any{
		"pullRequestId": prID,
		"revisionId":    "rev-1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["overridden"])
}

func TestHandler_OverridePullRequestApprovalRules(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-4")

	rec := doRequest(t, h, "OverridePullRequestApprovalRules", map[string]any{
		"pullRequestId":  prID,
		"revisionId":     "rev1",
		"overrideStatus": "OVERRIDE",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_CreatePullRequestApprovalRule(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-8")

	rec := doRequest(t, h, "CreatePullRequestApprovalRule", map[string]any{
		"pullRequestId":       prID,
		"approvalRuleName":    "my-rule",
		"approvalRuleContent": `{"Version":"2018-11-08","Statements":[]}`,
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	rule := resp["approvalRule"].(map[string]any)
	assert.Equal(t, "my-rule", rule["approvalRuleName"])
}

func TestHandler_DeletePullRequestApprovalRule(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-9")

	doRequest(t, h, "CreatePullRequestApprovalRule", map[string]any{
		"pullRequestId":       prID,
		"approvalRuleName":    "delete-me",
		"approvalRuleContent": `{"Version":"2018-11-08","Statements":[]}`,
	})

	rec := doRequest(t, h, "DeletePullRequestApprovalRule", map[string]any{
		"pullRequestId":    prID,
		"approvalRuleName": "delete-me",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_UpdatePullRequestApprovalRuleContent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-10")

	doRequest(t, h, "CreatePullRequestApprovalRule", map[string]any{
		"pullRequestId":       prID,
		"approvalRuleName":    "update-rule",
		"approvalRuleContent": `{"Version":"2018-11-08","Statements":[]}`,
	})

	rec := doRequest(t, h, "UpdatePullRequestApprovalRuleContent", map[string]any{
		"pullRequestId":    prID,
		"approvalRuleName": "update-rule",
		"newRuleContent":   `{"Version":"2018-11-08","Statements":[{"Type":"Approvers","NumberOfApprovalsNeeded":2}]}`,
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_DescribePullRequestEvents(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-11")

	rec := doRequest(t, h, "DescribePullRequestEvents", map[string]any{
		"pullRequestId": prID,
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp["pullRequestEvents"])
}

func TestHandler_DescribePullRequestEvents_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		eventType  string
		wantEvents int
		wantCode   int
		doOverride bool
	}{
		{
			name:       "no_events",
			wantEvents: 0,
			wantCode:   http.StatusOK,
		},
		{
			name:       "with_override_event",
			doOverride: true,
			wantEvents: 1,
			wantCode:   http.StatusOK,
		},
		{
			name:       "matching_filter",
			doOverride: true,
			eventType:  "PULL_REQUEST_APPROVAL_RULE_OVERRIDDEN",
			wantEvents: 1,
			wantCode:   http.StatusOK,
		},
		{
			name:       "non_matching_filter",
			doOverride: true,
			eventType:  "PULL_REQUEST_CREATED",
			wantEvents: 0,
			wantCode:   http.StatusOK,
		},
		{
			name:      "invalid_filter",
			eventType: "NOT_A_REAL_EVENT_TYPE",
			wantCode:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			prID := setupPR(t, h, "repo")

			if tt.doOverride {
				doRequest(t, h, "OverridePullRequestApprovalRules", map[string]any{
					"pullRequestId":  prID,
					"revisionId":     "rev-1",
					"overrideStatus": "OVERRIDE",
				})
			}

			req := map[string]any{"pullRequestId": prID}
			if tt.eventType != "" {
				req["pullRequestEventType"] = tt.eventType
			}

			rec := doRequest(t, h, "DescribePullRequestEvents", req)
			assert.Equal(t, tt.wantCode, rec.Code)
			if tt.wantCode != http.StatusOK {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			events := resp["pullRequestEvents"].([]any)
			assert.Len(t, events, tt.wantEvents)
		})
	}
}

func TestHandler_EvaluatePullRequestApprovalRules(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "pr-repo-12")

	doRequest(t, h, "CreatePullRequestApprovalRule", map[string]any{
		"pullRequestId":       prID,
		"approvalRuleName":    "eval-rule",
		"approvalRuleContent": `{"Version":"2018-11-08","Statements":[]}`,
	})

	rec := doRequest(t, h, "EvaluatePullRequestApprovalRules", map[string]any{
		"pullRequestId": prID,
		"revisionId":    "rev1",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	eval := resp["evaluation"].(map[string]any)
	satisfied := eval["approvalRulesSatisfied"].([]any)
	require.Len(t, satisfied, 1)
	assert.Equal(t, "eval-rule", satisfied[0])
	assert.Empty(t, eval["approvalRulesNotSatisfied"])
	assert.Equal(t, true, eval["approved"])
	assert.Equal(t, false, eval["overridden"])
}

func TestHandler_EvaluatePullRequestApprovalRules_WithRules(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "repo")

	// Add two approval rules.
	doRequest(t, h, "CreatePullRequestApprovalRule", map[string]any{
		"pullRequestId":       prID,
		"approvalRuleName":    "rule-a",
		"approvalRuleContent": `{"Version":"2018-11-08"}`,
	})
	doRequest(t, h, "CreatePullRequestApprovalRule", map[string]any{
		"pullRequestId":       prID,
		"approvalRuleName":    "rule-b",
		"approvalRuleContent": `{"Version":"2018-11-08"}`,
	})

	rec := doRequest(t, h, "EvaluatePullRequestApprovalRules", map[string]any{
		"pullRequestId": prID,
		"revisionId":    "rev-1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	eval := resp["evaluation"].(map[string]any)
	satisfied := eval["approvalRulesSatisfied"].([]any)
	assert.Len(t, satisfied, 2)
}

func TestHandler_PullRequestApprovalRule_Lifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "repo")

	// Create approval rule.
	rec := doRequest(t, h, "CreatePullRequestApprovalRule", map[string]any{
		"pullRequestId":       prID,
		"approvalRuleName":    "my-rule",
		"approvalRuleContent": `{"Version":"2018-11-08","Statements":[]}`,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	rule := resp["approvalRule"].(map[string]any)
	assert.Equal(t, "my-rule", rule["approvalRuleName"])
	assert.NotEmpty(t, rule["approvalRuleId"])

	// Update content.
	rec = doRequest(t, h, "UpdatePullRequestApprovalRuleContent", map[string]any{
		"pullRequestId":    prID,
		"approvalRuleName": "my-rule",
		"newRuleContent":   `{"Version":"2018-11-08","Statements":[{"Type":"Approvers","NumberOfApprovalsNeeded":1}]}`,
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Evaluate.
	rec = doRequest(t, h, "EvaluatePullRequestApprovalRules", map[string]any{
		"pullRequestId": prID,
		"revisionId":    "rev-1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	eval := resp["evaluation"].(map[string]any)
	assert.Len(t, eval["approvalRulesSatisfied"].([]any), 1)

	// Delete rule.
	rec = doRequest(t, h, "DeletePullRequestApprovalRule", map[string]any{
		"pullRequestId":    prID,
		"approvalRuleName": "my-rule",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Evaluate — should be empty now.
	rec = doRequest(t, h, "EvaluatePullRequestApprovalRules", map[string]any{
		"pullRequestId": prID,
		"revisionId":    "rev-1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	eval = resp["evaluation"].(map[string]any)
	assert.Empty(t, eval["approvalRulesSatisfied"])
}
