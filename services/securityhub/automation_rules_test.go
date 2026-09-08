package securityhub_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/securityhub"
)

// Batch-1 accuracy gap: CreateAutomationRule is POST /automationrules/create, returns RuleArn.
func TestCreateAutomationRulePath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/automationrules/create", map[string]any{
		"RuleName":    "auto-archive-low",
		"Description": "Archive LOW severity findings",
		"RuleOrder":   1,
		"RuleStatus":  "ENABLED",
		"Criteria":    map[string]any{},
		"Actions":     []map[string]any{{"Type": "FINDING_FIELDS_UPDATE"}},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	ruleArn, _ := resp["RuleArn"].(string)
	assert.NotEmpty(t, ruleArn)
	assert.Contains(t, ruleArn, "arn:aws:securityhub:")
	assert.Contains(t, ruleArn, ":automation-rule/")
	assert.Contains(t, resp, "CreatedAt")
}

// Batch-1 accuracy gap: ListAutomationRules is GET /automationrules/list.
func TestListAutomationRulesIsGETAutomationrulesList(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/automationrules/create", map[string]any{
		"RuleName":   "rule-1",
		"RuleOrder":  1,
		"RuleStatus": "ENABLED",
		"Criteria":   map[string]any{},
		"Actions":    []map[string]any{},
	})

	rec := doRequest(t, h, http.MethodGet, "/automationrules/list", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	rules, _ := resp["AutomationRulesMetadata"].([]any)
	assert.Len(t, rules, 1)

	r0 := rules[0].(map[string]any)
	assert.NotEmpty(t, r0["RuleArn"])
	assert.Equal(t, "rule-1", r0["RuleName"])
	assert.Equal(t, "ENABLED", r0["RuleStatus"])
}

// Batch-1 accuracy gap: BatchGetAutomationRules is POST /automationrules/get.
func TestBatchGetAutomationRulesPath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, http.MethodPost, "/automationrules/create", map[string]any{
		"RuleName":   "batch-get-test",
		"RuleOrder":  1,
		"RuleStatus": "ENABLED",
		"Criteria":   map[string]any{"SeverityLabel": []any{map[string]any{"Value": "HIGH", "Comparison": "EQUALS"}}},
		"Actions":    []map[string]any{},
	})

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	ruleArn, _ := createResp["RuleArn"].(string)

	rec := doRequest(t, h, http.MethodPost, "/automationrules/get", map[string]any{
		"AutomationRulesArns": []string{ruleArn},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	rules, _ := resp["Rules"].([]any)
	assert.Len(t, rules, 1)

	r0 := rules[0].(map[string]any)
	assert.Equal(t, ruleArn, r0["RuleArn"])
	assert.Contains(t, r0, "Criteria")
	assert.Contains(t, r0, "Actions")
}

// Batch-1 accuracy gap: BatchDeleteAutomationRules is POST /automationrules/delete.
func TestBatchDeleteAutomationRulesPath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, http.MethodPost, "/automationrules/create", map[string]any{
		"RuleName":   "to-delete-rule",
		"RuleOrder":  1,
		"RuleStatus": "ENABLED",
		"Criteria":   map[string]any{},
		"Actions":    []map[string]any{},
	})

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	ruleArn, _ := createResp["RuleArn"].(string)

	rec := doRequest(t, h, http.MethodPost, "/automationrules/delete", map[string]any{
		"AutomationRulesArns": []string{ruleArn},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Contains(t, resp, "ProcessedAutomationRules")
	assert.Contains(t, resp, "UnprocessedAutomationRules")

	processed, _ := resp["ProcessedAutomationRules"].([]any)
	assert.Len(t, processed, 1)
}

// TestParity_CreateAutomationRule_ResponseFields verifies that CreateAutomationRule
// returns RuleStatus, RuleOrder, and RuleName in addition to RuleArn.
func TestCreateAutomationRule_ResponseFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	enableHub(t, h)

	rec := doRequest(t, h, http.MethodPost, "/automationrules/create", map[string]any{
		"RuleName":    "my-rule",
		"RuleOrder":   float64(5),
		"RuleStatus":  "ENABLED",
		"IsTerminal":  false,
		"Criteria":    map[string]any{},
		"Actions":     []any{},
		"Description": "test rule",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["RuleArn"], "RuleArn must be present")
	assert.NotEmpty(t, resp["CreatedAt"], "CreatedAt must be present")
	assert.Equal(t, "my-rule", resp["RuleName"])
	assert.Equal(t, "ENABLED", resp["RuleStatus"])
	assert.InDelta(t, float64(5), resp["RuleOrder"], 0)
}

// TestCreateAutomationRule_TagsReachTagStore verifies that Tags supplied on
// CreateAutomationRule (confirmed accepted by the real
// CreateAutomationRuleRequest, botocore securityhub/2018-10-26) reach the
// same store ListTagsForResource reads (gopherstack-2mwl).
func TestCreateAutomationRule_TagsReachTagStore(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	enableHub(t, h)

	rec := doRequest(t, h, http.MethodPost, "/automationrules/create", map[string]any{
		"RuleName":  "tagged-rule",
		"RuleOrder": float64(1),
		"Criteria":  map[string]any{},
		"Actions":   []any{},
		"Tags":      map[string]any{"team": "sec"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ruleArn, _ := resp["RuleArn"].(string)
	require.NotEmpty(t, ruleArn)

	tagRec := doRequest(t, h, http.MethodGet, "/tags/"+ruleArn, nil)
	require.Equal(t, http.StatusOK, tagRec.Code)

	var tagResp map[string]any
	require.NoError(t, json.Unmarshal(tagRec.Body.Bytes(), &tagResp))
	tags, _ := tagResp["Tags"].(map[string]any)
	assert.Equal(t, "sec", tags["team"])
}

// TestParity_BatchUpdateAutomationRules_CriteriaAndActions verifies that
// BatchUpdateAutomationRules persists updates to Criteria and Actions fields.
func TestBatchUpdateAutomationRules_CriteriaAndActions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	enableHub(t, h)

	// Create a rule
	createRec := doRequest(t, h, http.MethodPost, "/automationrules/create", map[string]any{
		"RuleName":  "rule-to-update",
		"RuleOrder": float64(1),
		"Criteria":  map[string]any{},
		"Actions":   []any{},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	ruleArn := createResp["RuleArn"].(string)

	// Update its Criteria and Actions
	newCriteria := map[string]any{"SeverityLabel": []any{map[string]any{"Value": "CRITICAL", "Comparison": "EQUALS"}}}
	newActions := []any{map[string]any{"Type": "FINDING_FIELDS_UPDATE"}}

	updateRec := doRequest(t, h, http.MethodPatch, "/automationrules/update", map[string]any{
		"UpdateAutomationRulesRequestItems": []any{
			map[string]any{
				"RuleArn":  ruleArn,
				"Criteria": newCriteria,
				"Actions":  newActions,
			},
		},
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	// Get the rule to verify criteria/actions were persisted
	getRec := doRequest(t, h, http.MethodPost, "/automationrules/get", map[string]any{
		"AutomationRulesArns": []string{ruleArn},
	})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	rules, _ := getResp["Rules"].([]any)
	require.Len(t, rules, 1)

	rule := rules[0].(map[string]any)
	criteria, ok := rule["Criteria"].(map[string]any)
	require.True(t, ok, "Criteria must be present after update")
	assert.Contains(t, criteria, "SeverityLabel", "updated Criteria must be stored")

	actions, ok := rule["Actions"].([]any)
	require.True(t, ok, "Actions must be present after update")
	assert.Len(t, actions, 1, "updated Actions must be stored")
}

// TestParity_UpdateAutomationRuleV2_ActionsApplied verifies that updating a
// V2 automation rule's Actions field actually applies the new actions. JSON
// bodies decode array elements as []any (not []map[string]any); a direct
// type assertion to []map[string]any silently drops the update.
func TestUpdateAutomationRuleV2_ActionsApplied(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, http.MethodPost, "/automationrulesv2/create", map[string]any{
		"RuleName":   "my-rule",
		"RuleStatus": "ENABLED",
		"Criteria":   map[string]any{},
		"Actions":    []any{},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	identifier, _ := createResp["RuleId"].(string)
	require.NotEmpty(t, identifier)

	updateRec := doRequest(t, h, http.MethodPatch, "/automationrulesv2/"+identifier, map[string]any{
		"Actions": []any{
			map[string]any{
				"Type": "FINDING_FIELDS_UPDATE",
				"FindingFieldsUpdate": map[string]any{
					"Comment": "updated by rule",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updateResp map[string]any
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updateResp))

	actions, _ := updateResp["Actions"].([]any)
	require.Len(t, actions, 1, "Actions update must be applied, not silently dropped")

	action, _ := actions[0].(map[string]any)
	assert.Equal(t, "FINDING_FIELDS_UPDATE", action["Type"])
}

func TestBackend_BatchUpdateAutomationRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		wantProcessed int
		wantUnproc    int
	}{
		{
			name:          "update existing rule",
			wantProcessed: 1,
			wantUnproc:    0,
		},
		{
			name:          "update non-existent rule",
			wantProcessed: 0,
			wantUnproc:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := securityhub.NewInMemoryBackend("000000000000", "us-east-1")
			ruleArn, _ := b.CreateAutomationRule(map[string]any{
				"RuleName":   "MyRule",
				"RuleStatus": "ENABLED",
				"RuleOrder":  float64(1),
			})

			var updates []map[string]any
			if tc.wantProcessed > 0 {
				updates = []map[string]any{{
					"RuleArn":     ruleArn,
					"RuleName":    "UpdatedName",
					"Description": "Updated desc",
					"RuleStatus":  "DISABLED",
					"RuleOrder":   float64(2),
					"IsTerminal":  true,
				}}
			} else {
				updates = []map[string]any{{
					"RuleArn":  "arn:aws:securityhub:us-east-1:000000000000:automation-rule/99999",
					"RuleName": "Ghost",
				}}
			}

			processed, unprocessed := b.BatchUpdateAutomationRules(updates)
			assert.Len(t, processed, tc.wantProcessed)
			assert.Len(t, unprocessed, tc.wantUnproc)
		})
	}
}

func TestHandler_BatchUpdateAutomationRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{name: "batch update automation rules", wantCode: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			// Create a rule first
			createRec := doRequest(t, h, http.MethodPost, "/automationrules/create", map[string]any{
				"RuleName":   "TestRule",
				"RuleStatus": "ENABLED",
				"RuleOrder":  float64(1),
				"Criteria":   map[string]any{},
				"Actions":    []any{},
			})
			require.Equal(t, http.StatusOK, createRec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
			ruleArn, _ := createResp["RuleArn"].(string)
			require.NotEmpty(t, ruleArn)

			rec := doRequest(t, h, http.MethodPatch, "/automationrules/update", map[string]any{
				"UpdateAutomationRulesRequestItems": []any{
					map[string]any{
						"RuleArn":     ruleArn,
						"RuleName":    "UpdatedRule",
						"Description": "Updated",
					},
				},
			})
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

func TestHandler_AutomationRuleV2_GetNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{name: "get non-existent automation rule V2", wantCode: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodGet, "/automationrulesv2/nonexistent-rule", nil)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

func TestHandler_AutomationRuleV2_UpdateNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{name: "update non-existent automation rule V2", wantCode: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rec := doRequest(
				t,
				h,
				http.MethodPatch,
				"/automationrulesv2/nonexistent-rule",
				map[string]any{
					"RuleName": "Updated",
				},
			)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

func TestHandler_AutomationRuleV2_DeleteNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{name: "delete non-existent automation rule V2", wantCode: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodDelete, "/automationrulesv2/nonexistent-rule", nil)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

func TestAutomationRulesV2(t *testing.T) {
	t.Parallel()

	type step struct {
		body   any
		check  func(t *testing.T, code int, resp map[string]any) string
		name   string
		method string
		path   string
	}

	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "Create Get List Update Delete AutomationRuleV2",
			steps: []step{
				{
					name:   "create",
					method: http.MethodPost,
					path:   "/automationrulesv2/create",
					body: map[string]any{
						"RuleName":   "TestRuleV2",
						"RuleStatus": "ENABLED",
						"RuleOrder":  1.0,
					},
					check: func(t *testing.T, code int, resp map[string]any) string {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
						id, _ := resp["RuleId"].(string)
						assert.NotEmpty(t, id)
						assert.Equal(t, "TestRuleV2", resp["RuleName"])

						return id
					},
				},
				{
					name:   "list",
					method: http.MethodGet,
					path:   "/automationrulesv2/list",
					body:   nil,
					check: func(t *testing.T, code int, resp map[string]any) string {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
						rules, _ := resp["Rules"].([]any)
						assert.Len(t, rules, 1)
						r, _ := rules[0].(map[string]any)

						return r["RuleId"].(string)
					},
				},
				{
					name:   "get",
					method: http.MethodGet,
					path:   "/automationrulesv2/rule-v2-1",
					body:   nil,
					check: func(t *testing.T, code int, resp map[string]any) string {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
						assert.Equal(t, "TestRuleV2", resp["RuleName"])

						return ""
					},
				},
				{
					name:   "update",
					method: http.MethodPatch,
					path:   "/automationrulesv2/rule-v2-1",
					body: map[string]any{
						"RuleName": "UpdatedRuleV2",
					},
					check: func(t *testing.T, code int, resp map[string]any) string {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
						assert.Equal(t, "UpdatedRuleV2", resp["RuleName"])

						return ""
					},
				},
				{
					name:   "delete",
					method: http.MethodDelete,
					path:   "/automationrulesv2/rule-v2-1",
					body:   nil,
					check: func(t *testing.T, code int, _ map[string]any) string {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)

						return ""
					},
				},
				{
					name:   "get after delete returns 404",
					method: http.MethodGet,
					path:   "/automationrulesv2/rule-v2-1",
					body:   nil,
					check: func(t *testing.T, code int, _ map[string]any) string {
						t.Helper()
						assert.Equal(t, http.StatusNotFound, code)

						return ""
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			for _, s := range tc.steps {
				rec := doRequest(t, h, s.method, s.path, s.body)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				s.check(t, rec.Code, resp)
			}
		})
	}
}

// gopherstack-1qf: automation rules were pure CRUD -- Criteria/Actions were
// stored and echoed back but never evaluated against imported findings.
// AWS documents that BatchImportFindings itself cannot set
// Note/UserDefinedFields/VerificationState/Workflow "since they're managed
// by Security Hub customers/automation rules" (findings.go's
// findingCustomerManagedFields doc comment) -- automation rules are the
// mechanism that manages them, so a rule's FINDING_FIELDS_UPDATE action must
// actually apply on import for that architecture to have any real effect.
func TestBatchImportFindings_AutomationRuleFires(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		criteriaTitle string
		wantSeverity  string
	}{
		{
			name:          "matching_finding_severity_updated_by_rule",
			criteriaTitle: "Test finding",
			wantSeverity:  "CRITICAL",
		},
		{
			name:          "non_matching_finding_untouched",
			criteriaTitle: "some other title entirely",
			wantSeverity:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, http.MethodPost, "/accounts", map[string]any{"EnableDefaultStandards": false})

			doRequest(t, h, http.MethodPost, "/automationrules/create", map[string]any{
				"RuleName":   "escalate-test-findings",
				"RuleOrder":  1,
				"RuleStatus": "ENABLED",
				"IsTerminal": true,
				"Criteria": map[string]any{
					"Title": []map[string]any{
						{"Value": tc.criteriaTitle, "Comparison": "EQUALS"},
					},
				},
				"Actions": []map[string]any{
					{
						"Type": "FINDING_FIELDS_UPDATE",
						"FindingFieldsUpdate": map[string]any{
							"Severity": map[string]any{"Label": "CRITICAL"},
						},
					},
				},
			})

			rec := doRequest(t, h, http.MethodPost, "/findings/import", map[string]any{
				"Findings": []any{securityhub.ValidFinding(nil)},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = doRequest(t, h, http.MethodPost, "/findings", map[string]any{"Filters": map[string]any{}})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				Findings []map[string]any `json:"Findings"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Len(t, resp.Findings, 1)

			severity, _ := resp.Findings[0]["Severity"].(map[string]any)
			gotLabel, _ := severity["Label"].(string)
			assert.Equal(t, tc.wantSeverity, gotLabel)
		})
	}
}
