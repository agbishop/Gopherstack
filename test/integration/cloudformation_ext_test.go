package integration_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cloudformationsdk "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_CloudFormation_DriftDetection(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createCloudFormationClient(t)
	ctx := t.Context()

	stackName := "test-drift-" + cfnStackID()
	template := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"MyBucket": {"Type": "AWS::S3::Bucket", "Properties": {}}
		}
	}`

	_, err := client.CreateStack(ctx, &cloudformationsdk.CreateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(template),
	})
	require.NoError(t, err)

	waitForStackStatus(t, client, stackName, 10*time.Second)

	// DetectStackDrift
	detectOut, err := client.DetectStackDrift(ctx, &cloudformationsdk.DetectStackDriftInput{
		StackName: aws.String(stackName),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, detectOut.StackDriftDetectionId)

	// DescribeStackDriftDetectionStatus
	statusInput := &cloudformationsdk.DescribeStackDriftDetectionStatusInput{
		StackDriftDetectionId: detectOut.StackDriftDetectionId,
	}
	statusOut, err := client.DescribeStackDriftDetectionStatus(ctx, statusInput)
	require.NoError(t, err)
	assert.NotEmpty(t, statusOut.DetectionStatus)

	// DescribeStackResourceDrifts
	driftsOut, err := client.DescribeStackResourceDrifts(ctx, &cloudformationsdk.DescribeStackResourceDriftsInput{
		StackName: aws.String(stackName),
	})
	require.NoError(t, err)
	assert.NotNil(t, driftsOut.StackResourceDrifts)

	// Cleanup
	_, err = client.DeleteStack(ctx, &cloudformationsdk.DeleteStackInput{StackName: aws.String(stackName)})
	require.NoError(t, err)
}

func TestIntegration_CloudFormation_StackPolicy(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createCloudFormationClient(t)
	ctx := t.Context()

	stackName := "test-policy-" + cfnStackID()
	template := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"MyBucket": {"Type": "AWS::S3::Bucket", "Properties": {}}
		}
	}`

	_, err := client.CreateStack(ctx, &cloudformationsdk.CreateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(template),
	})
	require.NoError(t, err)

	waitForStackStatus(t, client, stackName, 10*time.Second)

	policy := `{"Statement":[{"Effect":"Allow","Action":"Update:*","Principal":"*","Resource":"*"}]}`

	// SetStackPolicy
	_, err = client.SetStackPolicy(ctx, &cloudformationsdk.SetStackPolicyInput{
		StackName:       aws.String(stackName),
		StackPolicyBody: aws.String(policy),
	})
	require.NoError(t, err)

	// GetStackPolicy
	policyOut, err := client.GetStackPolicy(ctx, &cloudformationsdk.GetStackPolicyInput{
		StackName: aws.String(stackName),
	})
	require.NoError(t, err)
	assert.Equal(t, policy, aws.ToString(policyOut.StackPolicyBody))

	// Cleanup
	_, err = client.DeleteStack(ctx, &cloudformationsdk.DeleteStackInput{StackName: aws.String(stackName)})
	require.NoError(t, err)
}

func TestIntegration_CloudFormation_GetTemplateSummary(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createCloudFormationClient(t)
	ctx := t.Context()

	template := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters": {
			"BucketName": {"Type": "String", "Default": "my-bucket"}
		},
		"Resources": {
			"MyBucket": {
				"Type": "AWS::S3::Bucket",
				"Properties": {"BucketName": {"Ref": "BucketName"}}
			}
		}
	}`

	summaryOut, err := client.GetTemplateSummary(ctx, &cloudformationsdk.GetTemplateSummaryInput{
		TemplateBody: aws.String(template),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, summaryOut.Parameters)
	assert.NotEmpty(t, summaryOut.ResourceTypes)
}

func TestIntegration_CloudFormation_EstimateTemplateCost(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createCloudFormationClient(t)
	ctx := t.Context()

	template := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"MyBucket": {"Type": "AWS::S3::Bucket", "Properties": {}}
		}
	}`

	costOut, err := client.EstimateTemplateCost(ctx, &cloudformationsdk.EstimateTemplateCostInput{
		TemplateBody: aws.String(template),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, aws.ToString(costOut.Url))
}

func TestIntegration_CloudFormation_DescribeAccountLimits(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createCloudFormationClient(t)
	ctx := t.Context()

	limitsOut, err := client.DescribeAccountLimits(ctx, &cloudformationsdk.DescribeAccountLimitsInput{})
	require.NoError(t, err)
	assert.NotEmpty(t, limitsOut.AccountLimits)
}

func TestIntegration_CloudFormation_CancelUpdateRollback(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createCloudFormationClient(t)
	ctx := t.Context()

	stackName := "test-cancel-" + cfnStackID()
	template := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"MyBucket": {"Type": "AWS::S3::Bucket", "Properties": {}}
		}
	}`

	_, err := client.CreateStack(ctx, &cloudformationsdk.CreateStackInput{
		StackName:    aws.String(stackName),
		TemplateBody: aws.String(template),
	})
	require.NoError(t, err)

	waitForStackStatus(t, client, stackName, 10*time.Second)

	// CancelUpdateStack is rejected on a CREATE_COMPLETE stack: real AWS
	// only allows cancelling a stack that is UPDATE_IN_PROGRESS.
	_, err = client.CancelUpdateStack(ctx, &cloudformationsdk.CancelUpdateStackInput{
		StackName: aws.String(stackName),
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "ValidationError", apiErr.ErrorCode())

	// ContinueUpdateRollback (no-op on CREATE_COMPLETE stack)
	_, err = client.ContinueUpdateRollback(ctx, &cloudformationsdk.ContinueUpdateRollbackInput{
		StackName: aws.String(stackName),
	})
	require.NoError(t, err)

	// Cleanup
	_, err = client.DeleteStack(ctx, &cloudformationsdk.DeleteStackInput{StackName: aws.String(stackName)})
	require.NoError(t, err)
}
