package awsconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/awsconfig"
)

func TestAWSConfigBackend_DeleteConfigRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(t *testing.T, b *awsconfig.InMemoryBackend)
		name    string
		delName string
		wantErr bool
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "my-rule"}))
			},
			delName: "my-rule",
		},
		{
			name:    "not_found",
			delName: "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			err := b.DeleteConfigRule(tt.delName)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
		})
	}
}

// TestAWSConfigBackend_DeleteConfigRule_ClearsRemediationExceptions verifies
// that DeleteConfigRule clears remediation exceptions recorded under the
// rule name. Otherwise a new rule put back under the same (user-chosen,
// reusable) ConfigRuleName -- with a remediation configuration re-added --
// inherits the deleted rule's stale exceptions.
func TestAWSConfigBackend_DeleteConfigRule_ClearsRemediationExceptions(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "reused-rule"}))
	require.NoError(t, b.PutRemediationExceptions("reused-rule", []awsconfig.RemediationExceptionResourceKey{
		{ResourceType: "AWS::S3::Bucket", ResourceID: "bucket1"},
	}))
	require.Len(t, b.DescribeRemediationExceptions("reused-rule"), 1)

	require.NoError(t, b.DeleteConfigRule("reused-rule"))

	require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "reused-rule"}))

	assert.Empty(t, b.DescribeRemediationExceptions("reused-rule"),
		"recreated config rule must not inherit the deleted rule's remediation exceptions")
}

func TestAWSConfigBackend_DeleteEvaluationResults(t *testing.T) {
	t.Parallel()

	t.Run("existing_rule_clears_evaluations", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "my-rule"}))
		require.NoError(t, b.PutEvaluations([]awsconfig.EvaluationResult{
			{
				ConfigRuleName: "my-rule", ResourceType: "AWS::S3::Bucket",
				ResourceID: "b1", ComplianceType: "NON_COMPLIANT",
			},
		}))
		before, err := b.GetComplianceDetailsByConfigRule("my-rule", nil)
		require.NoError(t, err)
		require.Len(t, before, 1)

		require.NoError(t, b.DeleteEvaluationResults("my-rule"))

		after, err := b.GetComplianceDetailsByConfigRule("my-rule", nil)
		require.NoError(t, err)
		assert.Empty(t, after)
		assert.Empty(t, b.GetConfigRuleComplianceType("my-rule"))
	})

	t.Run("nonexistent_rule_errors", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		err := b.DeleteEvaluationResults("nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrNoSuchConfigRule)
	})

	t.Run("empty_name_is_validation_error", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		err := b.DeleteEvaluationResults("")
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrValidation)
	})
}

func TestAWSConfigBackend_DescribeConfigRules_WithStoredRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(t *testing.T, b *awsconfig.InMemoryBackend)
		name      string
		filter    []string
		wantNames []string
	}{
		{
			name: "returns_all_sorted",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-b"}))
				require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-a"}))
				require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-c"}))
			},
			wantNames: []string{"rule-a", "rule-b", "rule-c"},
		},
		{
			name: "filter_by_name",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-a"}))
				require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-b"}))
			},
			filter:    []string{"rule-a"},
			wantNames: []string{"rule-a"},
		},
		{
			name:      "empty_backend",
			wantNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			rules, err := b.DescribeConfigRules(tt.filter)
			require.NoError(t, err)
			names := make([]string, len(rules))
			for i, r := range rules {
				names[i] = r.ConfigRuleName
			}
			assert.Equal(t, tt.wantNames, names)
		})
	}
}

func TestAWSConfigBackend_DescribeConfigRules_UnknownNameErrors(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule-a"}))

	_, err := b.DescribeConfigRules([]string{"rule-a", "does-not-exist"})
	require.Error(t, err)
	assert.ErrorIs(t, err, awsconfig.ErrNoSuchConfigRule)
}

func TestAWSConfigBackend_GetComplianceDetailsByConfigRule_UnknownRuleErrors(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()

	_, err := b.GetComplianceDetailsByConfigRule("does-not-exist", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, awsconfig.ErrNoSuchConfigRule)
}

func TestDescribeConfigRuleEvaluationStatus(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	_ = b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule1"})
	_ = b.StartConfigRulesEvaluation()

	statuses := b.DescribeConfigRuleEvaluationStatus([]string{"rule1"})
	if len(statuses) != 1 || statuses[0].ConfigRuleName != "rule1" {
		t.Fatalf("DescribeConfigRuleEvaluationStatus: %v", statuses)
	}
}

func TestDescribeConfigRuleEvaluationStatus_All(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	_ = b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "r1"})
	_ = b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "r2"})
	_ = b.StartConfigRulesEvaluation()

	statuses := b.DescribeConfigRuleEvaluationStatus(nil)
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d: %v", len(statuses), statuses)
	}
}

func TestPutEvaluations(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()

	err := b.PutEvaluations([]awsconfig.EvaluationResult{
		{ConfigRuleName: "rule1", ComplianceType: "COMPLIANT"},
	})
	if err != nil {
		t.Fatalf("PutEvaluations: %v", err)
	}

	ct := b.GetConfigRuleComplianceType("rule1")
	if ct != "COMPLIANT" {
		t.Fatalf("expected COMPLIANT, got %q", ct)
	}
}

func TestPutExternalEvaluation(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()

	err := b.PutExternalEvaluation(awsconfig.EvaluationResult{
		ConfigRuleName: "rule1",
		ComplianceType: "NON_COMPLIANT",
	})
	if err != nil {
		t.Fatalf("PutExternalEvaluation: %v", err)
	}

	ct := b.GetConfigRuleComplianceType("rule1")
	if ct != "NON_COMPLIANT" {
		t.Fatalf("expected NON_COMPLIANT, got %q", ct)
	}
}

func TestDescribeComplianceByConfigRule(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	_ = b.PutEvaluations([]awsconfig.EvaluationResult{
		{ConfigRuleName: "r1", ComplianceType: "COMPLIANT"},
	})

	out := b.DescribeComplianceByConfigRule([]string{"r1"})
	if len(out) != 1 || out[0].Compliance.ComplianceType != "COMPLIANT" {
		t.Fatalf("DescribeComplianceByConfigRule: %v", out)
	}
}

func TestGetComplianceSummaryByConfigRule(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	out := b.GetComplianceSummaryByConfigRule()
	assert.Zero(t, out.CompliantResourceCount.CappedCount)
	assert.Zero(t, out.NonCompliantResourceCount.CappedCount)
}

// TestGetComplianceSummaryByConfigRule_Aggregates verifies the real AWS
// shape: GetComplianceSummaryByConfigRuleOutput carries a single
// ComplianceSummary object (CompliantResourceCount/NonCompliantResourceCount),
// not a list keyed by ComplianceType (confirmed at aws-sdk-go-v2/service/
// configservice's api_op_GetComplianceSummaryByConfigRule.go).
func TestGetComplianceSummaryByConfigRule_Aggregates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		evaluations      []awsconfig.EvaluationResult
		wantCompliant    int32
		wantNonCompliant int32
	}{
		{
			name: "all compliant",
			evaluations: []awsconfig.EvaluationResult{
				{ConfigRuleName: "r1", ComplianceType: "COMPLIANT"},
				{ConfigRuleName: "r2", ComplianceType: "COMPLIANT"},
			},
			wantCompliant:    2,
			wantNonCompliant: 0,
		},
		{
			name: "mixed",
			evaluations: []awsconfig.EvaluationResult{
				{ConfigRuleName: "r1", ComplianceType: "COMPLIANT"},
				{ConfigRuleName: "r2", ComplianceType: "NON_COMPLIANT"},
			},
			wantCompliant:    1,
			wantNonCompliant: 1,
		},
		{
			name: "not applicable ignored in counts",
			evaluations: []awsconfig.EvaluationResult{
				{ConfigRuleName: "r1", ComplianceType: "NOT_APPLICABLE"},
			},
			wantCompliant:    0,
			wantNonCompliant: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			require.NoError(t, b.PutEvaluations(tc.evaluations))

			got := b.GetComplianceSummaryByConfigRule()
			assert.Equal(t, tc.wantCompliant, got.CompliantResourceCount.CappedCount)
			assert.Equal(t, tc.wantNonCompliant, got.NonCompliantResourceCount.CappedCount)
		})
	}
}

func TestGetCustomRulePolicy_DefaultEmpty(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	policy := b.GetCustomRulePolicy("my-rule")
	if policy != "" {
		t.Fatalf("expected empty policy, got %q", policy)
	}
}

func TestDescribeAggregateComplianceByConfigRules(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	_ = b.PutEvaluations([]awsconfig.EvaluationResult{
		{ConfigRuleName: "r1", ComplianceType: "COMPLIANT"},
	})

	out := b.DescribeAggregateComplianceByConfigRules()
	if len(out) != 1 {
		t.Fatalf("expected 1, got %d", len(out))
	}
}

func TestAWSConfigBackend_DescribeComplianceByResource(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	require.NoError(t, b.PutEvaluations([]awsconfig.EvaluationResult{
		{
			ConfigRuleName: "r1", ResourceType: "AWS::S3::Bucket",
			ResourceID: "bucket-a", ComplianceType: "NON_COMPLIANT",
		},
		{
			ConfigRuleName: "r2", ResourceType: "AWS::S3::Bucket",
			ResourceID: "bucket-a", ComplianceType: "COMPLIANT",
		},
		{
			ConfigRuleName: "r1", ResourceType: "AWS::S3::Bucket",
			ResourceID: "bucket-b", ComplianceType: "COMPLIANT",
		},
	}))

	t.Run("no_filter_returns_all_resources", func(t *testing.T) {
		t.Parallel()

		out := b.DescribeComplianceByResource("", "", nil)
		require.Len(t, out, 2)
		assert.Equal(t, "bucket-a", out[0].ResourceID)
		assert.Equal(t, "NON_COMPLIANT", out[0].Compliance.ComplianceType)
		require.NotNil(t, out[0].Compliance.ComplianceContributorCount)
		assert.Equal(t, int32(1), out[0].Compliance.ComplianceContributorCount.CappedCount)
		assert.Equal(t, "bucket-b", out[1].ResourceID)
		assert.Equal(t, "COMPLIANT", out[1].Compliance.ComplianceType)
	})

	t.Run("filter_by_resource_id", func(t *testing.T) {
		t.Parallel()

		out := b.DescribeComplianceByResource("AWS::S3::Bucket", "bucket-b", nil)
		require.Len(t, out, 1)
		assert.Equal(t, "bucket-b", out[0].ResourceID)
	})

	t.Run("filter_by_compliance_type", func(t *testing.T) {
		t.Parallel()

		out := b.DescribeComplianceByResource("", "", []string{"NON_COMPLIANT"})
		require.Len(t, out, 1)
		assert.Equal(t, "bucket-a", out[0].ResourceID)
	})

	t.Run("no_evaluations_returns_empty", func(t *testing.T) {
		t.Parallel()

		empty := awsconfig.NewInMemoryBackend()
		assert.Empty(t, empty.DescribeComplianceByResource("", "", nil))
	})
}

func TestAWSConfigBackend_GetAggregateComplianceDetailsByConfigRule(t *testing.T) {
	t.Parallel()

	t.Run("unknown_aggregator_errors", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, err := b.GetAggregateComplianceDetailsByConfigRule(
			"does-not-exist", "rule1", "123456789012", "us-east-1", nil,
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrNoSuchAggregator)
	})

	t.Run("echoes_requested_account_and_region", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutConfigurationAggregator("agg1", nil, nil, nil))
		require.NoError(t, b.PutConfigRule(&awsconfig.ConfigRule{ConfigRuleName: "rule1"}))
		require.NoError(t, b.PutEvaluations([]awsconfig.EvaluationResult{
			{
				ConfigRuleName: "rule1", ResourceType: "AWS::S3::Bucket",
				ResourceID: "b1", ComplianceType: "NON_COMPLIANT",
			},
		}))

		results, err := b.GetAggregateComplianceDetailsByConfigRule(
			"agg1", "rule1", "999999999999", "eu-west-1", nil,
		)
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "999999999999", results[0].AccountID)
		assert.Equal(t, "eu-west-1", results[0].AwsRegion)
		assert.Equal(t, "NON_COMPLIANT", results[0].ComplianceType)
	})
}

func TestAWSConfigBackend_GetAggregateConfigRuleComplianceSummary(t *testing.T) {
	t.Parallel()

	t.Run("unknown_aggregator_errors", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, err := b.GetAggregateConfigRuleComplianceSummary("does-not-exist", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrNoSuchAggregator)
	})

	t.Run("groups_by_account_id_by_default", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutConfigurationAggregator("agg1", nil, nil, nil))
		require.NoError(t, b.PutEvaluations([]awsconfig.EvaluationResult{
			{ConfigRuleName: "rule1", ComplianceType: "COMPLIANT"},
		}))

		counts, err := b.GetAggregateConfigRuleComplianceSummary("agg1", "")
		require.NoError(t, err)
		require.Len(t, counts, 1)
		assert.Equal(t, "123456789012", counts[0].GroupName)
	})

	t.Run("groups_by_region_when_requested", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutConfigurationAggregator("agg1", nil, nil, nil))
		require.NoError(t, b.PutEvaluations([]awsconfig.EvaluationResult{
			{ConfigRuleName: "rule1", ComplianceType: "COMPLIANT"},
		}))

		counts, err := b.GetAggregateConfigRuleComplianceSummary("agg1", "AWS_REGION")
		require.NoError(t, err)
		require.Len(t, counts, 1)
		assert.Equal(t, "us-east-1", counts[0].GroupName)
	})
}
