package wafv2_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/wafv2"
)

func TestHandler_CheckCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
		wantMinCap int
	}{
		{
			name:       "no_rules",
			body:       map[string]any{"Scope": "REGIONAL", "Rules": []any{}},
			wantStatus: http.StatusOK,
			wantMinCap: 0,
		},
		{
			name: "two_rules",
			body: map[string]any{
				"Scope": "REGIONAL",
				"Rules": []any{
					map[string]any{"Name": "rule1"},
					map[string]any{"Name": "rule2"},
				},
			},
			wantStatus: http.StatusOK,
			wantMinCap: 2,
		},
		{
			name:       "missing_scope",
			body:       map[string]any{"Rules": []any{}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doWafv2Request(t, h, "CheckCapacity", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var result map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
				consumed, ok := result["Capacity"].(float64)
				require.True(t, ok)
				assert.GreaterOrEqual(t, int(consumed), tt.wantMinCap)
			}
		})
	}
}

func TestHandler_CreateRuleGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
		wantID     bool
	}{
		{
			name: "valid",
			body: map[string]any{
				"Name":     "my-rulegroup",
				"Scope":    "REGIONAL",
				"Capacity": 100,
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name:       "missing_name",
			body:       map[string]any{"Scope": "REGIONAL", "Capacity": 10},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_scope",
			body:       map[string]any{"Name": "my-rulegroup", "Capacity": 10},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doWafv2Request(t, h, "CreateRuleGroup", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantID {
				var result map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
				summary, ok := result["Summary"].(map[string]any)
				require.True(t, ok)
				assert.NotEmpty(t, summary["Id"])
				assert.NotEmpty(t, summary["ARN"])
				assert.NotEmpty(t, summary["LockToken"])
			}
		})
	}
}

func TestHandler_CreateRuleGroup_Duplicate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
		"Name":     "dup-group",
		"Scope":    "REGIONAL",
		"Capacity": 10,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
		"Name":     "dup-group",
		"Scope":    "REGIONAL",
		"Capacity": 10,
	})
	assert.Equal(t, http.StatusBadRequest, rec2.Code)

	var result map[string]string
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &result))
	assert.Equal(t, "WAFDuplicateItemException", result["__type"])
}

func TestHandler_DeleteFirewallManagerRuleGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*wafv2.Handler) (arnStr, lockToken string)
		name       string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *wafv2.Handler) (string, string) {
				w, _ := wafv2.CreateWebACLSimple(h.Backend, "my-acl", "REGIONAL", "", "ALLOW", nil)

				return h.Backend.WebACLARN(w.Name, w.ID, w.Scope), w.LockToken
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing_arn",
			setup: func(_ *wafv2.Handler) (string, string) {
				return "", "tok"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "not_found",
			setup: func(_ *wafv2.Handler) (string, string) {
				return "arn:aws:wafv2:us-east-1:000000000000:regional/webacl/nonexistent/badid", "tok"
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			arnStr, lockToken := tt.setup(h)
			body := map[string]any{"WebACLLockToken": lockToken}

			if arnStr != "" {
				body["WebACLArn"] = arnStr
			}

			rec := doWafv2Request(t, h, "DeleteFirewallManagerRuleGroups", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var result map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
				assert.NotEmpty(t, result["NextWebACLLockToken"])
			}
		})
	}
}

func TestBackend_RuleGroupARN(t *testing.T) {
	t.Parallel()

	b := wafv2.NewInMemoryBackend("123456789012", "us-east-1")
	tests := []struct {
		name     string
		wantPart string
		scope    string
	}{
		{name: "regional_scope", scope: "REGIONAL", wantPart: "regional"},
		{name: "cloudfront_scope", scope: "CLOUDFRONT", wantPart: "global"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			arnStr := b.RuleGroupARN("my-rg", "myid", tt.scope)
			assert.Contains(t, arnStr, tt.wantPart)
			assert.Contains(t, arnStr, "rulegroup")
		})
	}
}

func TestRuleGroupCapacityValidation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Capacity too low.
	recLow := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
		"Name":     "rg-low",
		"Scope":    "REGIONAL",
		"Capacity": 0,
	})
	assert.Equal(t, http.StatusBadRequest, recLow.Code)

	// Capacity too high.
	recHigh := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
		"Name":     "rg-high",
		"Scope":    "REGIONAL",
		"Capacity": 1501,
	})
	assert.Equal(t, http.StatusBadRequest, recHigh.Code)

	// Valid capacity.
	recValid := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
		"Name":     "rg-valid",
		"Scope":    "REGIONAL",
		"Capacity": 100,
	})
	assert.Equal(t, http.StatusOK, recValid.Code)
}

// ---- Gap 9: IPSet CIDR validation ------------------------------------------

func TestDeleteRuleGroupReferencedByWebACL(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create a RuleGroup.
	rgRec := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
		"Name":     "my-rg",
		"Scope":    "REGIONAL",
		"Capacity": 10,
	})
	require.Equal(t, http.StatusOK, rgRec.Code)

	var rgResp map[string]any
	require.NoError(t, json.Unmarshal(rgRec.Body.Bytes(), &rgResp))
	summary := rgResp["Summary"].(map[string]any)
	rgID := summary["Id"].(string)
	rgARN := summary["ARN"].(string)
	rgLockToken := summary["LockToken"].(string)

	// Create a WebACL that references the RuleGroup.
	aclRec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":             "acl-with-rg",
		"Scope":            "REGIONAL",
		"DefaultAction":    map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{"MetricName": "acl-with-rg"},
		"Rules": []map[string]any{
			{
				"Name":     "use-rule-group",
				"Priority": 1,
				"Statement": map[string]any{
					"RuleGroupReferenceStatement": map[string]any{
						"ARN": rgARN,
					},
				},
				"OverrideAction":   map[string]any{"None": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "use-rule-group"},
			},
		},
	})
	require.Equal(t, http.StatusOK, aclRec.Code)

	// Try to delete the RuleGroup — should fail with WAFAssociatedItemException.
	delRec := doWafv2Request(t, h, "DeleteRuleGroup", map[string]any{
		"Id":        rgID,
		"Name":      "my-rg",
		"Scope":     "REGIONAL",
		"LockToken": rgLockToken,
	})
	assert.Equal(t, http.StatusBadRequest, delRec.Code)

	var delResp map[string]any
	require.NoError(t, json.Unmarshal(delRec.Body.Bytes(), &delResp))
	assert.Equal(t, "WAFAssociatedItemException", delResp["__type"])
}

// ---- Gap 20: Tag validation ------------------------------------------------

func TestRuleGroupScopeValidationOnGet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
		"Name":     "rg-scope",
		"Scope":    "REGIONAL",
		"Capacity": 10,
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	// Get with wrong scope should fail.
	getRec := doWafv2Request(t, h, "GetRuleGroup", map[string]any{
		"Id":    id,
		"Scope": "CLOUDFRONT",
	})
	assert.Equal(t, http.StatusBadRequest, getRec.Code)

	// Get with correct scope should succeed.
	getRec2 := doWafv2Request(t, h, "GetRuleGroup", map[string]any{
		"Id":    id,
		"Scope": "REGIONAL",
	})
	assert.Equal(t, http.StatusOK, getRec2.Code)
}

// ---- IPSet update CIDR validation ------------------------------------------

func TestRuleGroup_UpdateRules(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
		"Name":     "updatable-rg",
		"Scope":    "REGIONAL",
		"Capacity": 50,
		"Rules": []map[string]any{
			{
				"Name":     "r1",
				"Priority": 1,
				"Statement": map[string]any{
					"IPSetReferenceStatement": map[string]any{
						"ARN": "arn:aws:wafv2:us-east-1:000000000000:regional/ipset/x/abc",
					},
				},
				"Action":           map[string]any{"Allow": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "r1"},
			},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	rgID := createResp["Summary"].(map[string]any)["Id"].(string)
	rgLock := createResp["Summary"].(map[string]any)["LockToken"].(string)

	// Update with 2 rules.
	updateRec := doWafv2Request(t, h, "UpdateRuleGroup", map[string]any{
		"Id":        rgID,
		"Name":      "updatable-rg",
		"Scope":     "REGIONAL",
		"LockToken": rgLock,
		"Rules": []map[string]any{
			{
				"Name":     "r1",
				"Priority": 1,
				"Statement": map[string]any{
					"IPSetReferenceStatement": map[string]any{
						"ARN": "arn:aws:wafv2:us-east-1:000000000000:regional/ipset/x/abc",
					},
				},
				"Action":           map[string]any{"Block": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "r1"},
			},
			{
				"Name":     "r2",
				"Priority": 2,
				"Statement": map[string]any{
					"IPSetReferenceStatement": map[string]any{
						"ARN": "arn:aws:wafv2:us-east-1:000000000000:regional/ipset/y/def",
					},
				},
				"Action":           map[string]any{"Count": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "r2"},
			},
		},
	})
	require.Equal(t, http.StatusOK, updateRec.Code, "UpdateRuleGroup: %s", updateRec.Body.String())

	// Verify 2 rules stored.
	getRec := doWafv2Request(t, h, "GetRuleGroup", map[string]any{"Id": rgID, "Scope": "REGIONAL"})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	rules := getResp["RuleGroup"].(map[string]any)["Rules"].([]any)
	assert.Len(t, rules, 2)
}

func TestListRuleGroups_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 5 {
		rec := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
			"Name":     "rg-" + itoa(i),
			"Scope":    "REGIONAL",
			"Capacity": 10,
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	page1Rec := doWafv2Request(t, h, "ListRuleGroups", map[string]any{
		"Scope": "REGIONAL",
		"Limit": 3,
	})
	require.Equal(t, http.StatusOK, page1Rec.Code)

	var page1Resp map[string]any
	require.NoError(t, json.Unmarshal(page1Rec.Body.Bytes(), &page1Resp))

	groups1, _ := page1Resp["RuleGroups"].([]any)
	assert.Len(t, groups1, 3)

	nextMarker, ok := page1Resp["NextMarker"].(string)
	require.True(t, ok, "NextMarker should be present after first page")

	page2Rec := doWafv2Request(t, h, "ListRuleGroups", map[string]any{
		"Scope":      "REGIONAL",
		"Limit":      3,
		"NextMarker": nextMarker,
	})
	require.Equal(t, http.StatusOK, page2Rec.Code)

	var page2Resp map[string]any
	require.NoError(t, json.Unmarshal(page2Rec.Body.Bytes(), &page2Resp))

	groups2, _ := page2Resp["RuleGroups"].([]any)
	assert.Len(t, groups2, 2)

	_, hasNextMarker := page2Resp["NextMarker"]
	assert.False(t, hasNextMarker, "second page should have no NextMarker")
}

// ---- WebACL with multiple rules: all action types ---------------------------

// createRuleGroupHelper creates a rule group with REGIONAL scope and returns its ID and ARN.
func createRuleGroupHelper(t *testing.T, h *wafv2.Handler, name string) (string, string) {
	t.Helper()

	rec := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
		"Name":     name,
		"Scope":    "REGIONAL",
		"Capacity": 10,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	summary, ok := resp["Summary"].(map[string]any)
	require.True(t, ok)

	id, ok := summary["Id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	arn, ok := summary["ARN"].(string)
	require.True(t, ok)

	return id, arn
}

func TestHandler_GetRuleGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupName  string
		requestID  string
		wantField  string
		wantStatus int
	}{
		{
			name:       "found",
			setupName:  "my-rg",
			wantStatus: http.StatusOK,
			wantField:  "RuleGroup",
		},
		{
			name:       "missing_id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not_found",
			requestID:  "nonexistent-id",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			id := tt.requestID
			if tt.setupName != "" {
				id, _ = createRuleGroupHelper(t, h, tt.setupName)
			}

			var body any
			if id != "" {
				body = map[string]any{"Id": id}
			} else {
				body = map[string]any{}
			}

			rec := doWafv2Request(t, h, "GetRuleGroup", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantField != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Contains(t, resp, tt.wantField)
			}
		})
	}
}

func TestHandler_ListRuleGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		scope      string
		setup      []string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "empty",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:       "two_groups",
			setup:      []string{"rg-a", "rg-b"},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "filter_scope_match",
			setup:      []string{"rg-a"},
			scope:      "REGIONAL",
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name:       "filter_scope_no_match",
			setup:      []string{"rg-a"},
			scope:      "CLOUDFRONT",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for _, name := range tt.setup {
				createRuleGroupHelper(t, h, name)
			}

			rec := doWafv2Request(t, h, "ListRuleGroups", map[string]any{"Scope": tt.scope})
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			items, ok := resp["RuleGroups"].([]any)
			require.True(t, ok)
			assert.Len(t, items, tt.wantCount)
		})
	}
}

func TestHandler_UpdateRuleGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupName   string
		requestID   string
		description string
		wantStatus  int
	}{
		{
			name:        "update_description",
			setupName:   "my-rg",
			description: "updated",
			wantStatus:  http.StatusOK,
		},
		{
			name:       "missing_id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not_found",
			requestID:  "nonexistent",
			setupName:  "nonexistent-name",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			id := tt.requestID
			var lockToken string
			if tt.setupName != "" && tt.requestID == "" {
				id, _ = createRuleGroupHelper(t, h, tt.setupName)
				rg, err := h.Backend.GetRuleGroup(context.Background(), id)
				require.NoError(t, err)
				lockToken = rg.LockToken
			}

			var body any
			if id != "" {
				body = map[string]any{
					"Id": id, "Name": tt.setupName, "Scope": "REGIONAL",
					"Description": tt.description, "LockToken": lockToken,
				}
			} else {
				body = map[string]any{}
			}

			rec := doWafv2Request(t, h, "UpdateRuleGroup", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Contains(t, resp, "NextLockToken")
			}
		})
	}
}

func TestHandler_ScopeValidation_CreateRuleGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		scope      string
		wantStatus int
	}{
		{name: "regional_valid", scope: "REGIONAL", wantStatus: http.StatusOK},
		{name: "cloudfront_valid", scope: "CLOUDFRONT", wantStatus: http.StatusOK},
		{name: "invalid_scope", scope: "BAD", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
				"Name":     "test-rg",
				"Scope":    tt.scope,
				"Capacity": 10,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestCheckCapacity_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rules      []map[string]any
		wantExact  int
		wantStatus int
	}{
		{
			name:       "nil_rules",
			rules:      nil,
			wantExact:  0,
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty_rules",
			rules:      []map[string]any{},
			wantExact:  0,
			wantStatus: http.StatusOK,
		},
		{
			name:       "single_rule",
			rules:      []map[string]any{{"Name": "r1"}},
			wantExact:  1,
			wantStatus: http.StatusOK,
		},
		{
			name: "five_rules",
			rules: []map[string]any{
				{"Name": "r1"},
				{"Name": "r2"},
				{"Name": "r3"},
				{"Name": "r4"},
				{"Name": "r5"},
			},
			wantExact:  5,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_scope",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			var body map[string]any
			if tt.name == "missing_scope" {
				body = map[string]any{"Rules": tt.rules}
			} else {
				body = map[string]any{"Scope": "REGIONAL", "Rules": tt.rules}
			}

			rec := doWafv2Request(t, h, "CheckCapacity", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var result map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))

				consumed, ok := result["Capacity"].(float64)
				require.True(t, ok)
				assert.Equal(t, tt.wantExact, int(consumed))
			}
		})
	}
}

func TestCheckCapacity_NilRules(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// nil rules → capacity 0.
	rec := doWafv2Request(t, h, "CheckCapacity", map[string]any{
		"Scope": "REGIONAL",
		"Rules": nil,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	capacity, _ := resp["Capacity"].(float64)
	assert.InDelta(t, float64(0), capacity, 0, "nil rules capacity should be 0")

	// Empty rules → capacity 0.
	rec = doWafv2Request(t, h, "CheckCapacity", map[string]any{
		"Scope": "REGIONAL",
		"Rules": []any{},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	capacity, _ = resp["Capacity"].(float64)
	assert.InDelta(t, float64(0), capacity, 0, "empty rules capacity should be 0")
}

// ---- CreateWebACL duplicate Name+Scope returns WAFDuplicateItemException ----

func TestValidation_RuleGroupNamePattern(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateRuleGroup", map[string]any{
		"Name":     "invalid name!",
		"Scope":    "REGIONAL",
		"Capacity": 10,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "RuleGroup name with spaces/special chars should be rejected")
}

// ---- DescribeManagedRuleGroup: catalog hit returns real data, miss returns WAFNonexistentItemException ----
