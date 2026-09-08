package awsconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/awsconfig"
)

// TestPutConformancePack_TemplateBody_DeploysConfigRules exercises
// parseConformancePackConfigRules indirectly through PutConformancePack (the
// parser itself is unexported), verifying a JSON conformance-pack template's
// AWS::Config::ConfigRule resources become real, evaluable config rules.
func TestPutConformancePack_TemplateBody_DeploysConfigRules(t *testing.T) {
	t.Parallel()

	const template = `{
		"Resources": {
			"S3VersioningRule": {
				"Type": "AWS::Config::ConfigRule",
				"Properties": {
					"ConfigRuleName": "s3-versioning-enabled",
					"Source": {"Owner": "AWS", "SourceIdentifier": "S3_BUCKET_VERSIONING_ENABLED"}
				}
			},
			"UnrelatedResource": {
				"Type": "AWS::S3::Bucket",
				"Properties": {}
			}
		}
	}`

	b := awsconfig.NewInMemoryBackend()
	require.NoError(t, b.PutConformancePack("pack1", "", "", template, "", "", nil))

	rules, err := b.DescribeConfigRules([]string{"s3-versioning-enabled"})
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.NotNil(t, rules[0].Source)
	assert.Equal(t, "S3_BUCKET_VERSIONING_ENABLED", rules[0].Source.SourceIdentifier)

	// The evaluation engine treats it as a real managed rule.
	require.NoError(t, b.PutResourceConfig(
		"AWS::S3::Bucket", "b1", `{"VersioningConfiguration":{"Status":"Enabled"}}`,
	))
	require.NoError(t, b.StartConfigRulesEvaluation())
	assert.Equal(t, "COMPLIANT", b.GetConfigRuleComplianceType("s3-versioning-enabled"))
}

// TestPutConformancePack_TemplateBody_DerivesNameFromLogicalID verifies a
// template resource without an explicit ConfigRuleName gets one derived from
// the pack name and logical ID.
func TestPutConformancePack_TemplateBody_DerivesNameFromLogicalID(t *testing.T) {
	t.Parallel()

	const template = `{
		"Resources": {
			"MyRule": {
				"Type": "AWS::Config::ConfigRule",
				"Properties": {"Source": {"Owner": "AWS", "SourceIdentifier": "ENCRYPTED_VOLUMES"}}
			}
		}
	}`

	b := awsconfig.NewInMemoryBackend()
	require.NoError(t, b.PutConformancePack("mypack", "", "", template, "", "", nil))

	rules, err := b.DescribeConfigRules([]string{"mypack-MyRule"})
	require.NoError(t, err)
	require.Len(t, rules, 1)
}

// TestPutConformancePack_TemplateBody_Unparsable deploys zero rules rather
// than erroring, since PutConformancePack does not require a valid template
// to succeed in this emulator (see parseConformancePackConfigRules's doc
// comment).
func TestPutConformancePack_TemplateBody_Unparsable(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	require.NoError(t, b.PutConformancePack("pack1", "", "", "not: valid: json: at: all", "", "", nil))

	rules, err := b.DescribeConfigRules(nil)
	require.NoError(t, err)
	assert.Empty(t, rules)
}

// TestPutConformancePack_TemplateBody_YAML verifies a YAML conformance-pack
// template (real AWS Config's documented alternative to JSON -- "You can use
// a YAML template...") deploys config rules the same way a JSON template
// does (gopherstack-ag85).
func TestPutConformancePack_TemplateBody_YAML(t *testing.T) {
	t.Parallel()

	const template = `
Resources:
  S3VersioningRule:
    Type: AWS::Config::ConfigRule
    Properties:
      ConfigRuleName: s3-versioning-enabled-yaml
      Source:
        Owner: AWS
        SourceIdentifier: S3_BUCKET_VERSIONING_ENABLED
  UnrelatedResource:
    Type: AWS::S3::Bucket
    Properties: {}
`

	b := awsconfig.NewInMemoryBackend()
	require.NoError(t, b.PutConformancePack("pack1", "", "", template, "", "", nil))

	rules, err := b.DescribeConfigRules([]string{"s3-versioning-enabled-yaml"})
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.NotNil(t, rules[0].Source)
	assert.Equal(t, "S3_BUCKET_VERSIONING_ENABLED", rules[0].Source.SourceIdentifier)
}

// TestPutConformancePack_MultipleTemplateSourcesRejected verifies real AWS
// Config's "specify only one of TemplateBody, TemplateS3Uri, or
// TemplateSSMDocumentDetails" constraint is enforced (gopherstack-ag85).
//
// wantErr was awsconfig.ErrValidation until this pass; PutConformancePack's
// deserializer declares InvalidParameterValueException, never
// ValidationException (configservice@v1.68.4 deserializers.go) -- the old
// assertion locked in the exact wire-code defect this pass fixed.
func TestPutConformancePack_MultipleTemplateSourcesRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		templateBody  string
		templateS3URI string
		ssmDocName    string
	}{
		{
			name:          "body_and_s3uri",
			templateBody:  `{"Resources":{}}`,
			templateS3URI: "s3://bucket/template.yaml",
		},
		{
			name:         "body_and_ssm",
			templateBody: `{"Resources":{}}`,
			ssmDocName:   "my-ssm-doc",
		},
		{
			name:          "s3uri_and_ssm",
			templateS3URI: "s3://bucket/template.yaml",
			ssmDocName:    "my-ssm-doc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			err := b.PutConformancePack("pack1", "", "", tt.templateBody, tt.templateS3URI, tt.ssmDocName, nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, awsconfig.ErrInvalidParameterValue)
		})
	}
}

// TestPutConformancePack_TemplateS3Uri_AcceptedButDeploysNoRules documents
// the current honest gap: gopherstack has no S3 fetcher wired into this
// service, so a TemplateS3Uri-sourced pack is accepted (not rejected) but
// deploys zero rules -- see PARITY.md.
func TestPutConformancePack_TemplateS3Uri_AcceptedButDeploysNoRules(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	require.NoError(t, b.PutConformancePack("pack1", "", "", "", "s3://bucket/template.yaml", "", nil))

	rules, err := b.DescribeConfigRules(nil)
	require.NoError(t, err)
	assert.Empty(t, rules)
}

// TestPutConformancePack_TemplateBody_UpdateReplacesRuleSet verifies updating
// a pack's template drops rules no longer present (cascade-deleting their
// evaluations) and adds newly-present ones.
func TestPutConformancePack_TemplateBody_UpdateReplacesRuleSet(t *testing.T) {
	t.Parallel()

	const v1 = `{"Resources":{"RuleA":{"Type":"AWS::Config::ConfigRule",
		"Properties":{"ConfigRuleName":"rule-a","Source":{"Owner":"AWS","SourceIdentifier":"ENCRYPTED_VOLUMES"}}}}}`
	const v2 = `{"Resources":{"RuleB":{"Type":"AWS::Config::ConfigRule",
		"Properties":{"ConfigRuleName":"rule-b","Source":{"Owner":"AWS","SourceIdentifier":"ENCRYPTED_VOLUMES"}}}}}`

	b := awsconfig.NewInMemoryBackend()
	require.NoError(t, b.PutConformancePack("pack1", "", "", v1, "", "", nil))

	rules, err := b.DescribeConfigRules(nil)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "rule-a", rules[0].ConfigRuleName)

	require.NoError(t, b.PutConformancePack("pack1", "", "", v2, "", "", nil))

	rules, err = b.DescribeConfigRules(nil)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "rule-b", rules[0].ConfigRuleName)
}
