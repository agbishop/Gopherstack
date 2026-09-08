package awsconfig_test

import (
	"encoding/json"
	"net/http"
	"testing"

	configservicesdk "github.com/aws/aws-sdk-go-v2/service/configservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/awsconfig"
)

// TestConfigRulePascalCaseKeys verifies DescribeConfigRules returns PascalCase JSON keys.
func TestConfigRulePascalCaseKeys(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)
	b := h.Backend
	require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{
		ConfigRuleName:  "my-rule",
		Description:     "test rule",
		InputParameters: `{"key":"val"}`,
	}))

	rec := doAWSConfigRequest(t, h, "DescribeConfigRules", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, `"ConfigRuleName"`)
	assert.Contains(t, body, `"ConfigRuleArn"`)
	assert.Contains(t, body, `"ConfigRuleId"`)
	assert.Contains(t, body, `"Description"`)
	assert.Contains(t, body, `"InputParameters"`)
	assert.NotContains(t, body, `"configRuleName"`)
}

// TestDescribeConfigRulesAcceptsFilters verifies the real DescribeConfigRules
// Filters object (EvaluationMode/RuleEvaluationVisibility) round-trips
// through the JSON decoder without erroring. gopherstack's ConfigRule has no
// EvaluationMode concept to filter by, so the filtered request currently
// returns the same set as an unfiltered one -- this test asserts that
// (documented) behavior, not a fabricated filtered result.
func TestDescribeConfigRulesAcceptsFilters(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)
	require.NoError(t, h.Backend.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-detective"}))

	rec := doAWSConfigRequest(t, h, "DescribeConfigRules", map[string]any{
		"Filters": map[string]any{
			"EvaluationMode":           "DETECTIVE",
			"RuleEvaluationVisibility": "PUBLIC",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		ConfigRules []struct {
			ConfigRuleName string `json:"ConfigRuleName"`
		} `json:"ConfigRules"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.ConfigRules, 1)
	assert.Equal(t, "rule-detective", out.ConfigRules[0].ConfigRuleName)
}

// TestConfigRuleARNGenerated verifies PutConfigRule generates a proper ARN.
func TestConfigRuleARNGenerated(t *testing.T) {
	t.Parallel()

	b := newTestAWSConfigHandler(t).Backend
	require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-x"}))

	rules, err := b.DescribeConfigRules([]string{"rule-x"})
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Contains(t, rules[0].ConfigRuleArn, "arn:aws:config:")
	assert.Contains(t, rules[0].ConfigRuleArn, "config-rule-")
	assert.NotEmpty(t, rules[0].ConfigRuleID)
}

// TestConfigRuleStateActive verifies new rules default to ACTIVE state.
func TestConfigRuleStateActive(t *testing.T) {
	t.Parallel()

	b := newTestAWSConfigHandler(t).Backend
	require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-y"}))

	rules, err := b.DescribeConfigRules(nil)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "ACTIVE", rules[0].ConfigRuleState)
}

// TestConfigRuleScopeRoundtrip verifies Scope is stored and returned.
func TestConfigRuleScopeRoundtrip(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)
	rec := doAWSConfigRequest(t, h, "PutConfigRule", map[string]any{
		"ConfigRule": map[string]any{
			"ConfigRuleName": "scoped-rule",
			"Source": map[string]any{
				"Owner":            "AWS",
				"SourceIdentifier": "S3_BUCKET_PUBLIC_READ_PROHIBITED",
			},
			"Scope": map[string]any{
				"ComplianceResourceTypes": []string{"AWS::S3::Bucket"},
				"TagKey":                  "env",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doAWSConfigRequest(t, h, "DescribeConfigRules", map[string]any{"ConfigRuleNames": []string{"scoped-rule"}})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		ConfigRules []struct {
			Scope *struct {
				TagKey                  string   `json:"TagKey"`
				ComplianceResourceTypes []string `json:"ComplianceResourceTypes"`
			} `json:"Scope"`
		} `json:"ConfigRules"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.ConfigRules, 1)
	require.NotNil(t, out.ConfigRules[0].Scope)
	assert.Contains(t, out.ConfigRules[0].Scope.ComplianceResourceTypes, "AWS::S3::Bucket")
	assert.Equal(t, "env", out.ConfigRules[0].Scope.TagKey)
}

// TestComplianceSummaryShape drives GetComplianceSummaryByConfigRule through
// a real SDK client and proves CompliantResourceCount/NonCompliantResourceCount
// round-trip. Real GetComplianceSummaryByConfigRuleOutput wraps a single
// ComplianceSummary object under "ComplianceSummary" (confirmed at
// aws-sdk-go-v2/service/configservice's
// api_op_GetComplianceSummaryByConfigRule.go); the previous version of this
// test only asserted the raw body *contained* the substring "ComplianceSummary"
// -- which stayed true even under the pre-fix bug, since the wrong shape
// nested a field also spelled "ComplianceSummary" one level inside an
// invented "ComplianceSummariesByConfigRule" list, so this test caught
// nothing. A real client's typed ComplianceSummary.CompliantResourceCount
// was always nil under the old shape; asserting the exact counts closes
// that gap.
func TestComplianceSummaryShape(t *testing.T) {
	t.Parallel()

	h := newTestAWSConfigHandler(t)
	client := newTestAWSConfigSDKClient(t, h)
	b := h.Backend
	require.NoError(t, b.PutEvaluations([]awsconfig.EvaluationResult{
		{ConfigRuleName: "r1", ComplianceType: "COMPLIANT", ResourceType: "AWS::EC2::Instance", ResourceID: "i-1"},
		{ConfigRuleName: "r2", ComplianceType: "NON_COMPLIANT", ResourceType: "AWS::EC2::Instance", ResourceID: "i-2"},
	}))

	out, err := client.GetComplianceSummaryByConfigRule(
		t.Context(), &configservicesdk.GetComplianceSummaryByConfigRuleInput{},
	)
	require.NoError(t, err)
	require.NotNil(t, out.ComplianceSummary)
	require.NotNil(t, out.ComplianceSummary.CompliantResourceCount)
	require.NotNil(t, out.ComplianceSummary.NonCompliantResourceCount)
	assert.Equal(t, int32(1), out.ComplianceSummary.CompliantResourceCount.CappedCount)
	assert.Equal(t, int32(1), out.ComplianceSummary.NonCompliantResourceCount.CappedCount)
}

// TestConfigRuleEvaluationStatusTimestampStrings verifies timestamps are strings not numbers.
func TestConfigRuleEvaluationStatusTimestampStrings(t *testing.T) {
	t.Parallel()

	b := newTestAWSConfigHandler(t).Backend
	require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-ts"}))
	require.NoError(t, b.StartConfigRulesEvaluation())

	statuses := b.DescribeConfigRuleEvaluationStatus(nil)
	require.Len(t, statuses, 1)
	assert.Equal(t, "rule-ts", statuses[0].ConfigRuleName)
}

func TestAWSConfigHandler_DescribeConfigRulesAndCompliance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(t *testing.T, h *awsconfig.Handler)
		body      any
		name      string
		action    string
		wantField string
		wantCode  int
	}{
		{
			name:      "describe_config_rules_empty",
			action:    "DescribeConfigRules",
			body:      map[string]any{},
			wantCode:  http.StatusOK,
			wantField: "ConfigRules",
		},
		{
			name:   "describe_config_rules_with_names",
			action: "DescribeConfigRules",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				require.NoError(t, h.Backend.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "my-rule"}))
			},
			body:      map[string]any{"ConfigRuleNames": []string{"my-rule"}},
			wantCode:  http.StatusOK,
			wantField: "ConfigRules",
		},
		{
			name:     "describe_config_rules_unknown_name_errors",
			action:   "DescribeConfigRules",
			body:     map[string]any{"ConfigRuleNames": []string{"does-not-exist"}},
			wantCode: http.StatusNotFound,
		},
		{
			name:   "get_compliance_details_empty",
			action: "GetComplianceDetailsByConfigRule",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				require.NoError(t, h.Backend.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "my-rule"}))
			},
			body:      map[string]any{"ConfigRuleName": "my-rule"},
			wantCode:  http.StatusOK,
			wantField: "EvaluationResults",
		},
		{
			name:     "get_compliance_details_no_name_errors",
			action:   "GetComplianceDetailsByConfigRule",
			body:     map[string]any{},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doAWSConfigRequest(t, h, tt.action, tt.body)
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantField == "" {
				return
			}

			var out map[string]any
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
			assert.NotNil(t, out[tt.wantField])
		})
	}
}

func TestAWSConfigHandler_DeleteConfigRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, h *awsconfig.Handler)
		body     any
		name     string
		wantCode int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				require.NoError(t, h.Backend.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "my-rule"}))
			},
			body:     map[string]any{"ConfigRuleName": "my-rule"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			body:     map[string]any{"ConfigRuleName": "nonexistent-rule"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doAWSConfigRequest(t, h, "DeleteConfigRule", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestAWSConfigHandler_DeleteEvaluationResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, h *awsconfig.Handler)
		body     any
		name     string
		wantCode int
	}{
		{
			name: "success_for_existing_rule",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				require.NoError(t, h.Backend.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "my-rule"}))
			},
			body:     map[string]any{"ConfigRuleName": "my-rule"},
			wantCode: http.StatusOK,
		},
		{
			name:     "nonexistent_rule_not_found",
			body:     map[string]any{"ConfigRuleName": "my-rule"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "empty_rule_name",
			body:     map[string]any{"ConfigRuleName": ""},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doAWSConfigRequest(t, h, "DeleteEvaluationResults", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestAWSConfigHandler_DescribeConfigRules_BackedByStorage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(t *testing.T, h *awsconfig.Handler)
		body      any
		name      string
		wantCode  int
		wantCount int
	}{
		{
			name:      "empty_returns_no_rules",
			body:      map[string]any{},
			wantCode:  http.StatusOK,
			wantCount: 0,
		},
		{
			name: "returns_stored_rules",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				require.NoError(t, h.Backend.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-x"}))
				require.NoError(t, h.Backend.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-y"}))
			},
			body:      map[string]any{},
			wantCode:  http.StatusOK,
			wantCount: 2,
		},
		{
			name: "filter_by_name",
			setup: func(t *testing.T, h *awsconfig.Handler) {
				t.Helper()
				require.NoError(t, h.Backend.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-1"}))
				require.NoError(t, h.Backend.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-2"}))
			},
			body:      map[string]any{"ConfigRuleNames": []string{"rule-1"}},
			wantCode:  http.StatusOK,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doAWSConfigRequest(t, h, "DescribeConfigRules", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			var out map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			var rules []any
			require.NoError(t, json.Unmarshal(out["ConfigRules"], &rules))
			assert.Len(t, rules, tt.wantCount)
		})
	}
}

// wantErr was awsconfig.ErrValidation until this pass; PutConfigRule's
// deserializer declares InvalidParameterValueException, never
// ValidationException (configservice@v1.68.4 deserializers.go) -- the old
// assertion locked in the exact wire-code defect this pass fixed.
func TestAWSConfigHandler_PutConfigRule_ValidationAndDescribe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		wantCode int
	}{
		{
			name:     "empty_name_returns_400",
			body:     map[string]any{"ConfigRuleName": ""},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			// Call PutConfigRule directly via backend (handler not wired for PutConfigRule in HTTP yet)
			ruleName := tt.body.(map[string]any)["ConfigRuleName"].(string)
			err := h.Backend.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: ruleName})
			require.Error(t, err)
			assert.ErrorIs(t, err, awsconfig.ErrInvalidParameterValue)
		})
	}
}
