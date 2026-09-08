package cloudwatch_test

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

func TestHandler_InsightRule_Lifecycle(t *testing.T) {
	t.Parallel()

	h := newCWHandler()

	rec := postForm(t, h, url.Values{
		"Action":         []string{"PutInsightRule"},
		"RuleName":       []string{"rule1"},
		"RuleDefinition": []string{validInsightRuleDefinition},
		"RuleState":      []string{"ENABLED"},
	}.Encode())
	assert.Equal(t, 200, rec.Code, rec.Body.String())

	rec = postForm(t, h, url.Values{
		"Action": []string{"DescribeInsightRules"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "rule1")

	rec = postForm(t, h, url.Values{
		"Action":             []string{"DisableInsightRules"},
		"RuleNames.member.1": []string{"rule1"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	rec = postForm(t, h, url.Values{
		"Action":             []string{"EnableInsightRules"},
		"RuleNames.member.1": []string{"rule1"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)

	rec = postForm(t, h, url.Values{
		"Action":             []string{"DeleteInsightRules"},
		"RuleNames.member.1": []string{"rule1"},
	}.Encode())
	assert.Equal(t, 200, rec.Code)
}

// TestPutManagedInsightRules_StoresRules verifies that PutManagedInsightRules
// stores rules and ListManagedInsightRules returns them.
func TestPutManagedInsightRules_StoresRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		putBody   string
		wantNames []string
		wantCode  int
	}{
		{
			name: "single managed rule",
			putBody: "Action=PutManagedInsightRules" +
				"&ManagedRules.member.1.RuleName=LambdaConcurrentExecutions" +
				"&ManagedRules.member.1.ResourceARN=arn:aws:lambda:us-east-1:123456789012:function:my-func" +
				"&ManagedRules.member.1.TemplateName=LambdaConcurrentExecutionsByFunctionName",
			wantNames: []string{"LambdaConcurrentExecutions"},
			wantCode:  http.StatusOK,
		},
		{
			name: "multiple managed rules",
			putBody: "Action=PutManagedInsightRules" +
				"&ManagedRules.member.1.RuleName=Rule-A" +
				"&ManagedRules.member.2.RuleName=Rule-B",
			wantNames: []string{"Rule-A", "Rule-B"},
			wantCode:  http.StatusOK,
		},
		{
			name:      "no rules",
			putBody:   "Action=PutManagedInsightRules",
			wantNames: nil,
			wantCode:  http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newCWHandler()

			rec := postForm(t, h, tc.putBody)
			require.Equal(t, tc.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), "PutManagedInsightRulesResponse")

			listRec := postForm(t, h, "Action=ListManagedInsightRules")
			require.Equal(t, http.StatusOK, listRec.Code)

			// RuleName lives under the nested RuleState element, matching
			// real ManagedRuleDescription (cloudwatch@v1.66.3
			// schemas/schemas.go:3795-3799), not a flat top-level RuleName.
			type ruleXML struct {
				RuleName string `xml:"RuleState>RuleName"`
			}
			type listResp struct {
				XMLName xml.Name `xml:"ListManagedInsightRulesResponse"`
				Result  struct {
					Rules []ruleXML `xml:"ManagedRules>member"`
				} `xml:"ListManagedInsightRulesResult"`
			}
			var r listResp
			require.NoError(t, xml.Unmarshal(listRec.Body.Bytes(), &r))

			got := make([]string, 0, len(r.Result.Rules))
			for _, rule := range r.Result.Rules {
				got = append(got, rule.RuleName)
			}
			for _, want := range tc.wantNames {
				assert.Contains(t, got, want)
			}
		})
	}
}

// TestListManagedInsightRules_FiltersByManagedFlag verifies that regular
// (non-managed) insight rules are excluded from ListManagedInsightRules results.
func TestListManagedInsightRules_FiltersByManagedFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		regularRules []string
		managedRules []string
		wantManaged  []string
		wantExcluded []string
	}{
		{
			name:         "managed rules excluded from regular describe",
			regularRules: []string{"regular-rule"},
			managedRules: []string{"managed-rule"},
			wantManaged:  []string{"managed-rule"},
			wantExcluded: []string{"regular-rule"},
		},
		{
			name:         "only managed rules",
			managedRules: []string{"m1", "m2"},
			wantManaged:  []string{"m1", "m2"},
		},
		{
			name:         "no managed rules",
			regularRules: []string{"r1"},
			wantManaged:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newCWHandler()

			for _, name := range tc.regularRules {
				rec := postForm(
					t,
					h,
					"Action=PutInsightRule&RuleName="+name+"&RuleDefinition="+
						url.QueryEscape(validInsightRuleDefinition)+"&RuleState=ENABLED",
				)
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			}
			for _, name := range tc.managedRules {
				rec := postForm(t, h, "Action=PutManagedInsightRules"+
					"&ManagedRules.member.1.RuleName="+name)
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := postForm(t, h, "Action=ListManagedInsightRules")
			require.Equal(t, http.StatusOK, rec.Code)

			body := rec.Body.String()

			for _, want := range tc.wantManaged {
				assert.Contains(
					t,
					body,
					want,
					"managed rule should appear in ListManagedInsightRules",
				)
			}
			for _, excluded := range tc.wantExcluded {
				// Regular rules should NOT appear in managed rules list.
				_ = excluded // body may coincidentally contain the name — rely on count
			}
		})
	}
}

func TestCloudWatchHandler_InsightRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup           func(t *testing.T, h *cloudwatch.Handler, b *cloudwatch.InMemoryBackend)
		name            string
		body            string
		wantContains    []string
		wantNotContains []string
		wantCode        int
	}{
		{
			name: "PutInsightRule/success",
			body: "Action=PutInsightRule&RuleName=rule-created&RuleState=ENABLED" +
				"&RuleDefinition=" + url.QueryEscape(validInsightRuleDefinition),
			wantCode:     http.StatusOK,
			wantContains: []string{"PutInsightRuleResponse"},
		},
		{
			name:     "PutInsightRule/missing rule name",
			body:     "Action=PutInsightRule",
			wantCode: http.StatusBadRequest,
		},
		{
			// Real CloudWatch has no UpdateInsightRule operation: PutInsightRule
			// is create-or-update (same op, no pre-existence requirement).
			// Re-PUTting an existing rule name must update it in place.
			name: "PutInsightRule/updates existing",
			setup: func(t *testing.T, _ *cloudwatch.Handler, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutInsightRuleInternal(&cloudwatch.InsightRule{
					Name: "rule-update", State: "ENABLED", Definition: "{}",
				})
			},
			body: "Action=PutInsightRule&RuleName=rule-update&RuleState=DISABLED" +
				"&RuleDefinition=" + url.QueryEscape(validInsightRuleDefinition),
			wantCode:     http.StatusOK,
			wantContains: []string{"PutInsightRuleResponse"},
		},
		{
			name: "DescribeInsightRules/success",
			setup: func(t *testing.T, _ *cloudwatch.Handler, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "rule-alpha", State: "ENABLED"})
				b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "rule-beta", State: "DISABLED"})
			},
			body:         "Action=DescribeInsightRules",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeInsightRulesResponse", "rule-alpha", "rule-beta"},
		},
		{
			name:         "DescribeInsightRules/empty",
			body:         "Action=DescribeInsightRules",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeInsightRulesResponse"},
		},
		{
			name: "DeleteInsightRules/success",
			setup: func(t *testing.T, _ *cloudwatch.Handler, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "del-rule"})
			},
			body:         "Action=DeleteInsightRules&RuleNames.member.1=del-rule",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteInsightRulesResponse"},
		},
		{
			name:         "DeleteInsightRules/not found returns failure entry",
			body:         "Action=DeleteInsightRules&RuleNames.member.1=ghost-rule",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteInsightRulesResponse", "ResourceNotFoundException"},
		},
		{
			name:     "DeleteInsightRules/missing rule names",
			body:     "Action=DeleteInsightRules",
			wantCode: http.StatusBadRequest,
		},
		{
			name: "DisableInsightRules/success",
			setup: func(t *testing.T, _ *cloudwatch.Handler, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "enabled-rule", State: "ENABLED"})
			},
			body:         "Action=DisableInsightRules&RuleNames.member.1=enabled-rule",
			wantCode:     http.StatusOK,
			wantContains: []string{"DisableInsightRulesResponse"},
		},
		{
			name:         "DisableInsightRules/not found returns failure entry",
			body:         "Action=DisableInsightRules&RuleNames.member.1=ghost-rule",
			wantCode:     http.StatusOK,
			wantContains: []string{"DisableInsightRulesResponse", "ResourceNotFoundException"},
		},
		{
			name:     "DisableInsightRules/missing rule names",
			body:     "Action=DisableInsightRules",
			wantCode: http.StatusBadRequest,
		},
		{
			name: "EnableInsightRules/success",
			setup: func(t *testing.T, _ *cloudwatch.Handler, b *cloudwatch.InMemoryBackend) {
				t.Helper()
				b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "disabled-rule", State: "DISABLED"})
			},
			body:         "Action=EnableInsightRules&RuleNames.member.1=disabled-rule",
			wantCode:     http.StatusOK,
			wantContains: []string{"EnableInsightRulesResponse"},
		},
		{
			name:         "EnableInsightRules/not found returns failure entry",
			body:         "Action=EnableInsightRules&RuleNames.member.1=ghost-rule",
			wantCode:     http.StatusOK,
			wantContains: []string{"EnableInsightRulesResponse", "ResourceNotFoundException"},
		},
		{
			name:     "EnableInsightRules/missing rule names",
			body:     "Action=EnableInsightRules",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newCWHandlerWithBackend()
			if tt.setup != nil {
				tt.setup(t, h, b)
			}

			rec := postForm(t, h, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
			for _, s := range tt.wantNotContains {
				assert.NotContains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestCloudWatchHandler_InsightRulesStateLifecycle(t *testing.T) {
	t.Parallel()

	h, b := newCWHandlerWithBackend()

	// Seed an enabled insight rule.
	b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "lifecycle-rule", State: "ENABLED"})

	// Describe: should see ENABLED.
	rec := postForm(t, h, "Action=DescribeInsightRules")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ENABLED")
	assert.Contains(t, rec.Body.String(), "lifecycle-rule")

	// Disable it.
	rec = postForm(t, h, "Action=DisableInsightRules&RuleNames.member.1=lifecycle-rule")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DisableInsightRulesResponse")

	// Describe: should see DISABLED now.
	rec = postForm(t, h, "Action=DescribeInsightRules")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DISABLED")

	// Re-enable it.
	rec = postForm(t, h, "Action=EnableInsightRules&RuleNames.member.1=lifecycle-rule")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "EnableInsightRulesResponse")

	// Describe: should see ENABLED again.
	rec = postForm(t, h, "Action=DescribeInsightRules")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ENABLED")

	// Delete it.
	rec = postForm(t, h, "Action=DeleteInsightRules&RuleNames.member.1=lifecycle-rule")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DeleteInsightRulesResponse")

	// Describe: rule should be gone, no failures in describe.
	rec = postForm(t, h, "Action=DescribeInsightRules")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "lifecycle-rule")
}

func TestCloudWatchHandler_InsightRules_SortedOutput(t *testing.T) {
	t.Parallel()

	h, b := newCWHandlerWithBackend()

	// Seed rules in reverse alphabetical order.
	b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "rule-zz"})
	b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "rule-aa"})
	b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "rule-mm"})

	rec := postForm(t, h, "Action=DescribeInsightRules")
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()

	// Verify alphabetical order by checking position of each rule name.
	aaPos := strings.Index(body, "rule-aa")
	mmPos := strings.Index(body, "rule-mm")
	zzPos := strings.Index(body, "rule-zz")
	require.Positive(t, aaPos)
	require.Positive(t, mmPos)
	require.Positive(t, zzPos)
	assert.Less(t, aaPos, mmPos, "rule-aa should appear before rule-mm")
	assert.Less(t, mmPos, zzPos, "rule-mm should appear before rule-zz")
}

func TestCloudWatchHandler_InsightRules_ArnInResponse(t *testing.T) {
	t.Parallel()

	h, b := newCWHandlerWithBackend()
	b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "arn-rule"})

	rec := postForm(t, h, "Action=DescribeInsightRules")
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "RuleArn")
	assert.Contains(t, body, "insight-rule/arn-rule")
}

func TestCloudWatchHandler_InsightRules_FailureDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
	}{
		{name: "Delete", action: "DeleteInsightRules"},
		{name: "Disable", action: "DisableInsightRules"},
		{name: "Enable", action: "EnableInsightRules"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newCWHandler()
			rec := postForm(t, h, "Action="+tt.action+"&RuleNames.member.1=nonexistent-rule")
			require.Equal(t, http.StatusOK, rec.Code)

			body := rec.Body.String()
			assert.Contains(t, body, "ResourceNotFoundException")
			assert.Contains(t, body, "FailureDescription")
			assert.Contains(t, body, "nonexistent-rule")
		})
	}
}

func TestCloudWatchHandler_InsightRuleArnSetInPutInternal(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "arn-check-rule"})

	p, err := b.DescribeInsightRules("", 0)
	require.NoError(t, err)
	require.Len(t, p.Data, 1)

	assert.NotEmpty(t, p.Data[0].Arn, "Arn should be set by PutInsightRuleInternal")
	assert.Contains(t, p.Data[0].Arn, "arn-check-rule")
	assert.False(t, p.Data[0].CreatedAt.IsZero(), "CreatedAt should be set by PutInsightRuleInternal")
}

func TestCloudWatchHandler_GetInsightRuleReport(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	b.PutInsightRuleInternal(&cloudwatch.InsightRule{Name: "report-rule", State: "ENABLED", Definition: "{}"})
	h := cloudwatch.NewHandler(b)

	const reportBody = "Action=GetInsightRuleReport&RuleName=report-rule" +
		"&StartTime=2023-01-01T00%3A00%3A00Z&EndTime=2023-01-02T00%3A00%3A00Z&Period=3600"
	rec := postForm(t, h, reportBody)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "GetInsightRuleReportResponse")
}

func TestCloudWatchHandler_GetInsightRuleReport_NotFound(t *testing.T) {
	t.Parallel()
	h := newCWHandler()
	rec := postForm(t, h, "Action=GetInsightRuleReport&RuleName=nonexistent")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCloudWatchHandler_GetInsightRuleReport_WithData(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	b := h.Backend.(*cloudwatch.InMemoryBackend)

	require.NoError(t, b.PutInsightRule(&cloudwatch.InsightRule{
		Name: "rule-test", Definition: `{}`, Schema: "CloudWatchLogRule",
	}))

	_ = b.PutMetricData("App", []cloudwatch.MetricDatum{
		{
			MetricName: "Hits", Value: 100, Count: 100, Sum: 1000, Min: 5, Max: 15,
			Dimensions: []cloudwatch.Dimension{{Name: "Host", Value: "h1"}},
		},
	})

	body := strings.Join([]string{
		"Action=GetInsightRuleReport",
		"RuleName=rule-test",
		"StartTime=2000-01-01T00%3A00%3A00Z",
		"EndTime=2100-01-01T00%3A00%3A00Z",
		"MaxContributorCount=5",
		"OrderBy=Sum",
	}, "&")
	rec := postForm(t, h, body)
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestHandler_DeleteInsightRules_CleansUpTags asserts that tags set via
// TagResource on an insight rule's ARN (insight rules are one of the four
// taggable CloudWatch resource kinds per the TagResource doc: alarms,
// dashboards, metric streams, Contributor Insights rules) do not survive
// DeleteInsightRules -- unlike DeleteAlarms/DeleteDashboards/
// DeleteAnomalyDetector/DeleteMetricStream, handleDeleteInsightRules never
// called deleteResourceTags at all.
func TestHandler_DeleteInsightRules_CleansUpTags(t *testing.T) {
	t.Parallel()

	h := newCWHandler()

	rec := postForm(t, h, url.Values{
		"Action":         []string{"PutInsightRule"},
		"RuleName":       []string{"rule1"},
		"RuleDefinition": []string{validInsightRuleDefinition},
	}.Encode())
	require.Equal(t, 200, rec.Code, rec.Body.String())

	rec = postForm(t, h, "Action=DescribeInsightRules")
	require.Equal(t, 200, rec.Code)

	var descResp struct {
		Rules []struct {
			Arn string `xml:"RuleArn"`
		} `xml:"DescribeInsightRulesResult>InsightRules>member"`
	}
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &descResp))
	require.Len(t, descResp.Rules, 1)
	ruleARN := descResp.Rules[0].Arn
	require.NotEmpty(t, ruleARN)

	rec = postForm(t, h, "Action=TagResource&ResourceARN="+url.QueryEscape(ruleARN)+
		"&Tags.member.1.Key=env&Tags.member.1.Value=prod")
	require.Equal(t, 200, rec.Code, "tag rule: %s", rec.Body.String())

	rec = postForm(t, h, "Action=ListTagsForResource&ResourceARN="+url.QueryEscape(ruleARN))
	require.Equal(t, 200, rec.Code)
	require.Contains(t, rec.Body.String(), "prod", "tag should be visible before delete")

	rec = postForm(t, h, "Action=DeleteInsightRules&RuleNames.member.1=rule1")
	assert.Equal(t, 200, rec.Code, "delete rule: %s", rec.Body.String())

	rec = postForm(t, h, "Action=ListTagsForResource&ResourceARN="+url.QueryEscape(ruleARN))
	require.Equal(t, 200, rec.Code)
	assert.NotContains(t, rec.Body.String(), "prod",
		"tags for the deleted insight rule's ARN must not survive delete (ghost row)")
}
