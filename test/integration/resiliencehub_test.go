package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	cloudformationsdk "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	ddbsdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ekssdk "github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	rdssdk "github.com/aws/aws-sdk-go-v2/service/rds"
	resiliencehubsdk "github.com/aws/aws-sdk-go-v2/service/resiliencehub"
	rhtypes "github.com/aws/aws-sdk-go-v2/service/resiliencehub/types"
	resourcegroupssdk "github.com/aws/aws-sdk-go-v2/service/resourcegroups"
	smithy "github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createResilienceHubClient returns a Resilience Hub client pointed at the shared test container.
func createResilienceHubClient(t *testing.T) *resiliencehubsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return resiliencehubsdk.NewFromConfig(cfg, func(o *resiliencehubsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// createResourceGroupsClient returns a Resource Groups client pointed at the shared test container.
func createResourceGroupsClient(t *testing.T) *resourcegroupssdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return resourcegroupssdk.NewFromConfig(cfg, func(o *resourcegroupssdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// rhCleanupCtx returns a context for use inside t.Cleanup callbacks.
// t.Context() must not be used there: Go 1.24+ cancels it before cleanups run.
func rhCleanupCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// rhErrorCode extracts the smithy error code from err, or "" if err isn't one.
func rhErrorCode(err error) string {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode()
	}

	return ""
}

// createRHApp creates a standard app via the real SDK client, for tests
// whose focus is a different op.
func createRHApp(ctx context.Context, t *testing.T, client *resiliencehubsdk.Client) string {
	t.Helper()

	out, err := client.CreateApp(ctx, &resiliencehubsdk.CreateAppInput{
		Name: aws.String("integ-app-" + uuid.NewString()[:8]),
	})
	require.NoError(t, err, "CreateApp should succeed")
	appArn := aws.ToString(out.App.AppArn)

	t.Cleanup(func() {
		cctx, cancel := rhCleanupCtx()
		defer cancel()
		_, _ = client.DeleteApp(cctx, &resiliencehubsdk.DeleteAppInput{
			AppArn: aws.String(appArn), ForceDelete: aws.Bool(true),
		})
	})

	return appArn
}

// fourDisruptionTypePolicy is a complete, valid Policy map covering all four
// required DisruptionType entries (Software/Hardware/AZ/Region) --
// CreateResiliencyPolicy rejects anything less (PARITY.md's validatePolicyMap
// judgment call).
func fourDisruptionTypePolicy(rtoSecs, rpoSecs int32) map[string]rhtypes.FailurePolicy {
	fp := rhtypes.FailurePolicy{RtoInSecs: rtoSecs, RpoInSecs: rpoSecs}

	return map[string]rhtypes.FailurePolicy{
		string(rhtypes.DisruptionTypeSoftware): fp,
		string(rhtypes.DisruptionTypeHardware): fp,
		string(rhtypes.DisruptionTypeAz):       fp,
		string(rhtypes.DisruptionTypeRegion):   fp,
	}
}

// TestIntegration_ResilienceHub_AppLifecycle drives CreateApp -> DescribeApp
// -> UpdateApp -> ListApps -> DeleteApp through a real SDK client.
//
//nolint:tparallel // sequential subtests
func TestIntegration_ResilienceHub_AppLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createResilienceHubClient(t)
	appName := "integ-app-lifecycle-" + uuid.NewString()[:8]

	createOut, err := client.CreateApp(ctx, &resiliencehubsdk.CreateAppInput{
		Name:        aws.String(appName),
		Description: aws.String("integration test app"),
	})
	require.NoError(t, err, "CreateApp should succeed")
	require.NotNil(t, createOut.App)
	appArn := aws.ToString(createOut.App.AppArn)
	assert.NotEmpty(t, appArn)
	assert.Equal(t, rhtypes.AppStatusTypeActive, createOut.App.Status)
	assert.Equal(t, rhtypes.AppComplianceStatusTypeNotAssessed, createOut.App.ComplianceStatus,
		"a freshly created app has never been assessed")

	t.Cleanup(func() {
		cctx, cancel := rhCleanupCtx()
		defer cancel()
		_, _ = client.DeleteApp(cctx, &resiliencehubsdk.DeleteAppInput{
			AppArn: aws.String(appArn), ForceDelete: aws.Bool(true),
		})
	})

	t.Run("DescribeAndUpdate", func(t *testing.T) { //nolint:paralleltest // sequential by design
		descOut, descErr := client.DescribeApp(ctx, &resiliencehubsdk.DescribeAppInput{AppArn: aws.String(appArn)})
		require.NoError(t, descErr)
		assert.Equal(t, appName, aws.ToString(descOut.App.Name))

		updateOut, updateErr := client.UpdateApp(ctx, &resiliencehubsdk.UpdateAppInput{
			AppArn:      aws.String(appArn),
			Description: aws.String("updated description"),
		})
		require.NoError(t, updateErr, "UpdateApp should succeed")
		assert.Equal(t, "updated description", aws.ToString(updateOut.App.Description))
	})

	t.Run("ListApps", func(t *testing.T) { //nolint:paralleltest // sequential by design
		listOut, listErr := client.ListApps(ctx, &resiliencehubsdk.ListAppsInput{Name: aws.String(appName)})
		require.NoError(t, listErr)
		require.Len(t, listOut.AppSummaries, 1)
		assert.Equal(t, appArn, aws.ToString(listOut.AppSummaries[0].AppArn))
	})

	t.Run("DeleteApp", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, delErr := client.DeleteApp(ctx, &resiliencehubsdk.DeleteAppInput{AppArn: aws.String(appArn)})
		require.NoError(t, delErr, "DeleteApp should succeed")

		_, descErr := client.DescribeApp(ctx, &resiliencehubsdk.DescribeAppInput{AppArn: aws.String(appArn)})
		require.Error(t, descErr, "describing a deleted app should fail")
		assert.Equal(t, "ResourceNotFoundException", rhErrorCode(descErr))
	})
}

// TestIntegration_ResilienceHub_AppVersionResourceLifecycle drives
// CreateAppVersionAppComponent/CreateAppVersionResource/
// UpdateAppVersionResource/DeleteAppVersionResource/PublishAppVersion
// through a real SDK client, proving the draft AppVersion state machine
// (mutations apply only to "draft"; PublishAppVersion snapshots it into a
// new numbered version).
//
//nolint:tparallel // sequential subtests
func TestIntegration_ResilienceHub_AppVersionResourceLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createResilienceHubClient(t)
	appArn := createRHApp(ctx, t, client)

	compOut, err := client.CreateAppVersionAppComponent(ctx, &resiliencehubsdk.CreateAppVersionAppComponentInput{
		AppArn: aws.String(appArn),
		Name:   aws.String("integ-component"),
		Type:   aws.String("AWS::ResilienceHub::AppComponent"),
	})
	require.NoError(t, err, "CreateAppVersionAppComponent should succeed")
	require.NotNil(t, compOut.AppComponent)

	t.Run("CreateUpdateListResource", func(t *testing.T) { //nolint:paralleltest // sequential by design
		resOut, createErr := client.CreateAppVersionResource(ctx, &resiliencehubsdk.CreateAppVersionResourceInput{
			AppArn:             aws.String(appArn),
			AppComponents:      []string{"integ-component"},
			LogicalResourceId:  &rhtypes.LogicalResourceId{Identifier: aws.String("MyQueue")},
			PhysicalResourceId: aws.String("arn:aws:sqs:us-east-1:000000000000:integ-queue"),
			ResourceType:       aws.String("AWS::SQS::Queue"),
		})
		require.NoError(t, createErr, "CreateAppVersionResource should succeed")
		require.NotNil(t, resOut.PhysicalResource)
		assert.Equal(t, "AWS::SQS::Queue", aws.ToString(resOut.PhysicalResource.ResourceType))

		updateOut, updateErr := client.UpdateAppVersionResource(ctx, &resiliencehubsdk.UpdateAppVersionResourceInput{
			AppArn:             aws.String(appArn),
			LogicalResourceId:  &rhtypes.LogicalResourceId{Identifier: aws.String("MyQueue")},
			PhysicalResourceId: aws.String("arn:aws:sqs:us-east-1:000000000000:integ-queue"),
			ResourceType:       aws.String("AWS::SQS::Queue"),
			Excluded:           aws.Bool(true),
		})
		require.NoError(t, updateErr, "UpdateAppVersionResource should succeed")
		assert.True(t, aws.ToBool(updateOut.PhysicalResource.Excluded))

		listOut, listErr := client.ListAppVersionResources(ctx, &resiliencehubsdk.ListAppVersionResourcesInput{
			AppArn: aws.String(appArn), AppVersion: aws.String("draft"),
		})
		require.NoError(t, listErr)
		assert.Len(t, listOut.PhysicalResources, 1)

		_, deleteErr := client.DeleteAppVersionResource(ctx, &resiliencehubsdk.DeleteAppVersionResourceInput{
			AppArn:            aws.String(appArn),
			LogicalResourceId: &rhtypes.LogicalResourceId{Identifier: aws.String("MyQueue")},
		})
		require.NoError(t, deleteErr, "DeleteAppVersionResource should succeed")

		listOut, listErr = client.ListAppVersionResources(ctx, &resiliencehubsdk.ListAppVersionResourcesInput{
			AppArn: aws.String(appArn), AppVersion: aws.String("draft"),
		})
		require.NoError(t, listErr)
		assert.Empty(t, listOut.PhysicalResources)
	})

	t.Run("PublishAndListVersions", func(t *testing.T) { //nolint:paralleltest // sequential by design
		pubOut, pubErr := client.PublishAppVersion(
			ctx,
			&resiliencehubsdk.PublishAppVersionInput{AppArn: aws.String(appArn)},
		)
		require.NoError(t, pubErr, "PublishAppVersion should succeed")
		assert.NotNil(t, pubOut.Identifier)
		assert.Equal(t, "1", aws.ToString(pubOut.AppVersion))

		listOut, listErr := client.ListAppVersions(
			ctx,
			&resiliencehubsdk.ListAppVersionsInput{AppArn: aws.String(appArn)},
		)
		require.NoError(t, listErr)

		versions := make([]string, 0, len(listOut.AppVersions))
		for _, v := range listOut.AppVersions {
			versions = append(versions, aws.ToString(v.AppVersion))
		}
		assert.Contains(t, versions, "draft")
		assert.Contains(t, versions, "1")
	})
}

// TestIntegration_ResilienceHub_ResiliencyPolicyLifecycle drives
// CreateResiliencyPolicy -> DescribeResiliencyPolicy -> UpdateResiliencyPolicy
// -> bind-to-app -> DeleteResiliencyPolicy conflict -> unbind -> delete.
//
//nolint:tparallel // sequential subtests
func TestIntegration_ResilienceHub_ResiliencyPolicyLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createResilienceHubClient(t)

	createOut, err := client.CreateResiliencyPolicy(ctx, &resiliencehubsdk.CreateResiliencyPolicyInput{
		PolicyName: aws.String("integ-policy-" + uuid.NewString()[:8]),
		Tier:       rhtypes.ResiliencyPolicyTierCritical,
		Policy:     fourDisruptionTypePolicy(600, 300),
	})
	require.NoError(t, err, "CreateResiliencyPolicy should succeed")
	require.NotNil(t, createOut.Policy)
	policyArn := aws.ToString(createOut.Policy.PolicyArn)
	require.Len(t, createOut.Policy.Policy, 4, "all four disruption types must be present on the wire")

	t.Cleanup(func() {
		cctx, cancel := rhCleanupCtx()
		defer cancel()
		_, _ = client.DeleteResiliencyPolicy(
			cctx,
			&resiliencehubsdk.DeleteResiliencyPolicyInput{PolicyArn: aws.String(policyArn)},
		)
	})

	t.Run("DescribeAndUpdate", func(t *testing.T) { //nolint:paralleltest // sequential by design
		descOut, descErr := client.DescribeResiliencyPolicy(ctx, &resiliencehubsdk.DescribeResiliencyPolicyInput{
			PolicyArn: aws.String(policyArn),
		})
		require.NoError(t, descErr)
		assert.Equal(t, rhtypes.ResiliencyPolicyTierCritical, descOut.Policy.Tier)

		updateOut, updateErr := client.UpdateResiliencyPolicy(ctx, &resiliencehubsdk.UpdateResiliencyPolicyInput{
			PolicyArn: aws.String(policyArn),
			Tier:      rhtypes.ResiliencyPolicyTierMissionCritical,
		})
		require.NoError(t, updateErr, "UpdateResiliencyPolicy should succeed")
		assert.Equal(t, rhtypes.ResiliencyPolicyTierMissionCritical, updateOut.Policy.Tier)
	})

	t.Run("DeleteWhileBoundIsConflict", func(t *testing.T) { //nolint:paralleltest // sequential by design
		appArn := createRHApp(ctx, t, client)

		_, bindErr := client.UpdateApp(ctx, &resiliencehubsdk.UpdateAppInput{
			AppArn: aws.String(appArn), PolicyArn: aws.String(policyArn),
		})
		require.NoError(t, bindErr, "binding the policy to the app should succeed")

		_, deleteErr := client.DeleteResiliencyPolicy(
			ctx, &resiliencehubsdk.DeleteResiliencyPolicyInput{PolicyArn: aws.String(policyArn)},
		)
		require.Error(t, deleteErr, "deleting a policy still bound to an app should fail")
		assert.Equal(t, "ConflictException", rhErrorCode(deleteErr))

		_, unbindErr := client.UpdateApp(ctx, &resiliencehubsdk.UpdateAppInput{
			AppArn: aws.String(appArn), ClearResiliencyPolicyArn: aws.Bool(true),
		})
		require.NoError(t, unbindErr, "unbinding via ClearResiliencyPolicyArn should succeed")

		descOut, descErr := client.DescribeApp(ctx, &resiliencehubsdk.DescribeAppInput{AppArn: aws.String(appArn)})
		require.NoError(t, descErr)
		assert.Empty(t, aws.ToString(descOut.App.PolicyArn), "ClearResiliencyPolicyArn must actually clear the binding")
	})
}

// TestIntegration_ResilienceHub_PolicyValidation tables CreateResiliencyPolicy
// validation across every required-field/enum failure mode.
func TestIntegration_ResilienceHub_PolicyValidation(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createResilienceHubClient(t)

	tests := []struct {
		mutate func(*resiliencehubsdk.CreateResiliencyPolicyInput)
		name   string
	}{
		{
			name: "missing hardware disruption type",
			mutate: func(in *resiliencehubsdk.CreateResiliencyPolicyInput) {
				delete(in.Policy, string(rhtypes.DisruptionTypeHardware))
			},
		},
		{
			name: "missing region disruption type",
			mutate: func(in *resiliencehubsdk.CreateResiliencyPolicyInput) {
				delete(in.Policy, string(rhtypes.DisruptionTypeRegion))
			},
		},
		{
			name: "invalid tier",
			mutate: func(in *resiliencehubsdk.CreateResiliencyPolicyInput) {
				in.Tier = "BOGUS"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := &resiliencehubsdk.CreateResiliencyPolicyInput{
				PolicyName: aws.String("integ-invalid-policy-" + uuid.NewString()[:8]),
				Tier:       rhtypes.ResiliencyPolicyTierCritical,
				Policy:     fourDisruptionTypePolicy(600, 300),
			}
			tt.mutate(in)

			_, err := client.CreateResiliencyPolicy(ctx, in)
			require.Error(t, err)

			var ve *rhtypes.ValidationException
			require.ErrorAs(t, err, &ve, "expected a real ValidationException from the SDK deserializer")
		})
	}
}

// TestIntegration_ResilienceHub_AssessmentLifecycle drives StartAppAssessment
// through its real Pending -> InProgress -> Success transition and proves the
// honest-gap posture: Summary is always nil, ResiliencyScore is always the
// documented placeholder -- never fabricated.
func TestIntegration_ResilienceHub_AssessmentLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createResilienceHubClient(t)
	appArn := createRHApp(ctx, t, client)

	startOut, err := client.StartAppAssessment(ctx, &resiliencehubsdk.StartAppAssessmentInput{
		AppArn: aws.String(appArn), AppVersion: aws.String("draft"), AssessmentName: aws.String("integ-assessment"),
	})
	require.NoError(t, err, "StartAppAssessment should succeed")
	require.NotNil(t, startOut.Assessment)
	assessmentArn := aws.ToString(startOut.Assessment.AssessmentArn)
	assert.Equal(t, rhtypes.AssessmentStatusPending, startOut.Assessment.AssessmentStatus)

	t.Cleanup(func() {
		cctx, cancel := rhCleanupCtx()
		defer cancel()
		_, _ = client.DeleteAppAssessment(
			cctx,
			&resiliencehubsdk.DeleteAppAssessmentInput{AssessmentArn: aws.String(assessmentArn)},
		)
	})

	var final *rhtypes.AppAssessment
	require.Eventually(t, func() bool {
		out, descErr := client.DescribeAppAssessment(ctx, &resiliencehubsdk.DescribeAppAssessmentInput{
			AssessmentArn: aws.String(assessmentArn),
		})
		if descErr != nil || out.Assessment == nil {
			return false
		}

		final = out.Assessment

		return out.Assessment.AssessmentStatus == rhtypes.AssessmentStatusSuccess
	}, 5*time.Second, 20*time.Millisecond, "assessment should transition Pending -> InProgress -> Success")

	require.NotNil(t, final)
	assert.Nil(t, final.Summary, "AssessmentSummary is Bedrock-LLM-backed and must never be fabricated")
	require.NotNil(t, final.ResiliencyScore)
	assert.InDelta(t, 0.0, final.ResiliencyScore.Score, 0, "ResiliencyScore.Score must stay the documented placeholder")
	assert.Equal(t, rhtypes.ComplianceStatusMissingPolicy, final.ComplianceStatus,
		"an app with no bound policy should read MissingPolicy per the coarse compliance rule")

	listOut, err := client.ListAppAssessments(
		ctx,
		&resiliencehubsdk.ListAppAssessmentsInput{AppArn: aws.String(appArn)},
	)
	require.NoError(t, err)
	found := false

	for _, s := range listOut.AssessmentSummaries {
		if aws.ToString(s.AssessmentArn) == assessmentArn {
			found = true

			break
		}
	}

	assert.True(t, found, "started assessment should appear in ListAppAssessments")

	_, err = client.DeleteAppAssessment(
		ctx,
		&resiliencehubsdk.DeleteAppAssessmentInput{AssessmentArn: aws.String(assessmentArn)},
	)
	require.NoError(t, err, "DeleteAppAssessment should succeed once terminal")
}

// TestIntegration_ResilienceHub_RecommendationFamilies tables the four
// List*Recommendations ops (SOP/alarm/test/component) plus
// BatchUpdateRecommendationStatus and the resource-grouping-recommendation
// family: all validate real backend state (an AssessmentArn or AppArn) but
// always return honestly-empty/always-failed content, since no
// recommendation-engine or ML-clustering output is derivable from the SDK.
func TestIntegration_ResilienceHub_RecommendationFamilies(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createResilienceHubClient(t)
	appArn := createRHApp(ctx, t, client)

	assessOut, assessErr := client.StartAppAssessment(ctx, &resiliencehubsdk.StartAppAssessmentInput{
		AppArn: aws.String(appArn), AppVersion: aws.String("draft"), AssessmentName: aws.String("integ-rec-assessment"),
	})
	require.NoError(t, assessErr)
	assessmentArn := aws.ToString(assessOut.Assessment.AssessmentArn)

	t.Run("ListRecommendations", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			call func() error
			name string
		}{
			{
				name: "alarm",
				call: func() error {
					_, callErr := client.ListAlarmRecommendations(
						ctx, &resiliencehubsdk.ListAlarmRecommendationsInput{AssessmentArn: aws.String(assessmentArn)},
					)

					return callErr
				},
			},
			{
				name: "sop",
				call: func() error {
					_, callErr := client.ListSopRecommendations(
						ctx, &resiliencehubsdk.ListSopRecommendationsInput{AssessmentArn: aws.String(assessmentArn)},
					)

					return callErr
				},
			},
			{
				name: "test",
				call: func() error {
					_, callErr := client.ListTestRecommendations(
						ctx, &resiliencehubsdk.ListTestRecommendationsInput{AssessmentArn: aws.String(assessmentArn)},
					)

					return callErr
				},
			},
			{
				name: "component",
				call: func() error {
					_, callErr := client.ListAppComponentRecommendations(
						ctx,
						&resiliencehubsdk.ListAppComponentRecommendationsInput{
							AssessmentArn: aws.String(assessmentArn),
						},
					)

					return callErr
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				callErr := tt.call()
				require.NoError(t, callErr, "a real assessment ARN should validate even though content is empty")
			})
		}
	})

	t.Run("unknown assessment ARN", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, listErr := client.ListAlarmRecommendations(ctx, &resiliencehubsdk.ListAlarmRecommendationsInput{
			AssessmentArn: aws.String("arn:aws:resiliencehub:us-east-1:000000000000:app-assessment/doesnotexist"),
		})
		require.Error(t, listErr)
		assert.Equal(t, "ResourceNotFoundException", rhErrorCode(listErr))
	})

	t.Run("BatchUpdateRecommendationStatusAlwaysFails", func(t *testing.T) {
		t.Parallel()

		out, batchErr := client.BatchUpdateRecommendationStatus(
			ctx,
			&resiliencehubsdk.BatchUpdateRecommendationStatusInput{
				AppArn: aws.String(appArn),
				RequestEntries: []rhtypes.UpdateRecommendationStatusRequestEntry{
					{
						EntryId:     aws.String("entry-1"),
						ReferenceId: aws.String("no-such-recommendation"),
						Excluded:    aws.Bool(true),
					},
				},
			},
		)
		require.NoError(t, batchErr)
		require.Len(
			t,
			out.FailedEntries,
			1,
			"no recommendation engine output exists, so every entry must fail honestly",
		)
		assert.Equal(t, "entry-1", aws.ToString(out.FailedEntries[0].EntryId))
	})

	t.Run("ResourceGroupingRecommendationTask", func(t *testing.T) {
		t.Parallel()

		taskOut, taskErr := client.StartResourceGroupingRecommendationTask(
			ctx, &resiliencehubsdk.StartResourceGroupingRecommendationTaskInput{AppArn: aws.String(appArn)},
		)
		require.NoError(t, taskErr)
		groupingID := aws.ToString(taskOut.GroupingId)

		require.Eventually(t, func() bool {
			out, descErr := client.DescribeResourceGroupingRecommendationTask(
				ctx, &resiliencehubsdk.DescribeResourceGroupingRecommendationTaskInput{
					AppArn: aws.String(appArn), GroupingId: aws.String(groupingID),
				},
			)

			return descErr == nil && out.Status == rhtypes.ResourcesGroupingRecGenStatusTypeSuccess
		}, 5*time.Second, 20*time.Millisecond, "grouping task should reach Success")

		listOut, err := client.ListResourceGroupingRecommendations(
			ctx, &resiliencehubsdk.ListResourceGroupingRecommendationsInput{AppArn: aws.String(appArn)},
		)
		require.NoError(t, err)
		assert.Empty(t, listOut.GroupingRecommendations, "no ML clustering output is ever fabricated")

		acceptOut, err := client.AcceptResourceGroupingRecommendations(
			ctx, &resiliencehubsdk.AcceptResourceGroupingRecommendationsInput{
				AppArn:  aws.String(appArn),
				Entries: []rhtypes.AcceptGroupingRecommendationEntry{{GroupingRecommendationId: aws.String("gr-1")}},
			},
		)
		require.NoError(t, err)
		require.Len(t, acceptOut.FailedEntries, 1, "no grouping recommendation was ever generated to accept")
	})
}

// TestIntegration_ResilienceHub_Tagging drives TagResource/UntagResource/
// ListTagsForResource across the three taggable resource kinds sharing the
// resiliencehub ARN namespace: App, ResiliencyPolicy, and AppAssessment.
func TestIntegration_ResilienceHub_Tagging(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createResilienceHubClient(t)
	appArn := createRHApp(ctx, t, client)

	policyOut, policyErr := client.CreateResiliencyPolicy(ctx, &resiliencehubsdk.CreateResiliencyPolicyInput{
		PolicyName: aws.String("integ-tag-policy-" + uuid.NewString()[:8]),
		Tier:       rhtypes.ResiliencyPolicyTierCritical,
		Policy:     fourDisruptionTypePolicy(600, 300),
	})
	require.NoError(t, policyErr)
	policyArn := aws.ToString(policyOut.Policy.PolicyArn)

	t.Cleanup(func() {
		cctx, cancel := rhCleanupCtx()
		defer cancel()
		_, _ = client.DeleteResiliencyPolicy(
			cctx,
			&resiliencehubsdk.DeleteResiliencyPolicyInput{PolicyArn: aws.String(policyArn)},
		)
	})

	assessOut, assessErr := client.StartAppAssessment(ctx, &resiliencehubsdk.StartAppAssessmentInput{
		AppArn: aws.String(appArn), AppVersion: aws.String("draft"), AssessmentName: aws.String("integ-tag-assessment"),
	})
	require.NoError(t, assessErr)
	assessmentArn := aws.ToString(assessOut.Assessment.AssessmentArn)

	t.Cleanup(func() {
		cctx, cancel := rhCleanupCtx()
		defer cancel()
		_, _ = client.DeleteAppAssessment(
			cctx,
			&resiliencehubsdk.DeleteAppAssessmentInput{AssessmentArn: aws.String(assessmentArn)},
		)
	})

	tests := []struct {
		name string
		arn  string
	}{
		{name: "app", arn: appArn},
		{name: "resiliency policy", arn: policyArn},
		{name: "app assessment", arn: assessmentArn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := client.TagResource(ctx, &resiliencehubsdk.TagResourceInput{
				ResourceArn: aws.String(tt.arn), Tags: map[string]string{"env": "integ"},
			})
			require.NoError(t, err, "TagResource should succeed")

			listOut, err := client.ListTagsForResource(
				ctx, &resiliencehubsdk.ListTagsForResourceInput{ResourceArn: aws.String(tt.arn)},
			)
			require.NoError(t, err)
			assert.Equal(t, "integ", listOut.Tags["env"])

			_, err = client.UntagResource(ctx, &resiliencehubsdk.UntagResourceInput{
				ResourceArn: aws.String(tt.arn), TagKeys: []string{"env"},
			})
			require.NoError(t, err, "UntagResource should succeed")

			listOut, err = client.ListTagsForResource(
				ctx, &resiliencehubsdk.ListTagsForResourceInput{ResourceArn: aws.String(tt.arn)},
			)
			require.NoError(t, err)
			assert.Empty(t, listOut.Tags)
		})
	}
}

// TestIntegration_ResilienceHub_NotFoundErrors tables ResourceNotFoundException
// across every resource kind this service addresses by ARN.
func TestIntegration_ResilienceHub_NotFoundErrors(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createResilienceHubClient(t)

	tests := []struct {
		call func() error
		name string
	}{
		{
			name: "app",
			call: func() error {
				_, err := client.DescribeApp(ctx, &resiliencehubsdk.DescribeAppInput{
					AppArn: aws.String("arn:aws:resiliencehub:us-east-1:000000000000:app/doesnotexist"),
				})

				return err
			},
		},
		{
			name: "resiliency policy",
			call: func() error {
				_, err := client.DescribeResiliencyPolicy(ctx, &resiliencehubsdk.DescribeResiliencyPolicyInput{
					PolicyArn: aws.String(
						"arn:aws:resiliencehub:us-east-1:000000000000:resiliency-policy/doesnotexist",
					),
				})

				return err
			},
		},
		{
			name: "app assessment",
			call: func() error {
				_, err := client.DescribeAppAssessment(ctx, &resiliencehubsdk.DescribeAppAssessmentInput{
					AssessmentArn: aws.String(
						"arn:aws:resiliencehub:us-east-1:000000000000:app-assessment/doesnotexist",
					),
				})

				return err
			},
		},
		{
			name: "recommendation template delete",
			call: func() error {
				_, err := client.DeleteRecommendationTemplate(ctx, &resiliencehubsdk.DeleteRecommendationTemplateInput{
					RecommendationTemplateArn: aws.String(
						"arn:aws:resiliencehub:us-east-1:000000000000:recommendation-template/doesnotexist",
					),
				})

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.call()
			require.Error(t, err)
			assert.Equal(t, "ResourceNotFoundException", rhErrorCode(err))

			var rnf *rhtypes.ResourceNotFoundException
			require.ErrorAs(t, err, &rnf, "expected a real ResourceNotFoundException from the SDK deserializer")
		})
	}
}

const rhCfnBucketTemplate = `{
	"AWSTemplateFormatVersion": "2010-09-09",
	"Resources": {
		"MyBucket": {
			"Type": "AWS::S3::Bucket",
			"Properties": {}
		}
	}
}`

// TestIntegration_ResilienceHub_ResourceMappingResolution proves
// ResolveAppVersionResources performs REAL cross-service resolution against
// this emulator's CloudFormation/Resource Groups/EKS backends (services/
// resiliencehub/cross_service.go) for CfnStack/ResourceGroup/EKS mapping
// types -- genuinely discovered resources, not fabricated ones -- while
// AppRegistryApp (no backing service in this tree) honestly stays
// unresolved. This is PARITY.md's single largest closed gap this pass.
func TestIntegration_ResilienceHub_ResourceMappingResolution(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createResilienceHubClient(t)
	cfnClient := createCloudFormationClient(t)
	rgClient := createResourceGroupsClient(t)
	eksClient := createEKSClient(t)

	placeholderPhysicalID := &rhtypes.PhysicalResourceId{
		Identifier: aws.String("placeholder"),
		Type:       rhtypes.PhysicalIdentifierTypeNative,
	}

	tests := []struct {
		mapping func(t *testing.T) rhtypes.ResourceMapping
		assert  func(t *testing.T, resources []rhtypes.PhysicalResource)
		name    string
	}{
		{
			name: "cfn stack",
			mapping: func(t *testing.T) rhtypes.ResourceMapping {
				t.Helper()

				stackName := "rh-cfn-" + uuid.NewString()[:8]
				_, err := cfnClient.CreateStack(ctx, &cloudformationsdk.CreateStackInput{
					StackName: aws.String(stackName), TemplateBody: aws.String(rhCfnBucketTemplate),
				})
				require.NoError(t, err)

				t.Cleanup(func() {
					cctx, cancel := rhCleanupCtx()
					defer cancel()
					_, _ = cfnClient.DeleteStack(
						cctx,
						&cloudformationsdk.DeleteStackInput{StackName: aws.String(stackName)},
					)
				})

				require.Eventually(t, func() bool {
					out, descErr := cfnClient.DescribeStackResource(ctx, &cloudformationsdk.DescribeStackResourceInput{
						StackName: aws.String(stackName), LogicalResourceId: aws.String("MyBucket"),
					})

					return descErr == nil && out.StackResourceDetail != nil &&
						out.StackResourceDetail.ResourceStatus == cftypes.ResourceStatusCreateComplete
				}, 10*time.Second, 50*time.Millisecond, "stack resource should reach CREATE_COMPLETE")

				return rhtypes.ResourceMapping{
					MappingType:        rhtypes.ResourceMappingTypeCfnStack,
					LogicalStackName:   aws.String(stackName),
					PhysicalResourceId: placeholderPhysicalID,
				}
			},
			assert: func(t *testing.T, resources []rhtypes.PhysicalResource) {
				t.Helper()
				require.Len(t, resources, 1, "the stack's one real S3 bucket resource should be discovered")
				assert.Equal(t, "AWS::S3::Bucket", aws.ToString(resources[0].ResourceType))
				assert.NotEmpty(t, aws.ToString(resources[0].PhysicalResourceId.Identifier))
			},
		},
		{
			name: "resource group",
			mapping: func(t *testing.T) rhtypes.ResourceMapping {
				t.Helper()

				groupName := "rh-rg-" + uuid.NewString()[:8]
				_, err := rgClient.CreateGroup(ctx, &resourcegroupssdk.CreateGroupInput{Name: aws.String(groupName)})
				require.NoError(t, err)

				t.Cleanup(func() {
					cctx, cancel := rhCleanupCtx()
					defer cancel()
					_, _ = rgClient.DeleteGroup(cctx, &resourcegroupssdk.DeleteGroupInput{Group: aws.String(groupName)})
				})

				memberARN := "arn:aws:ec2:us-east-1:000000000000:instance/i-" + uuid.NewString()[:8]
				_, err = rgClient.GroupResources(ctx, &resourcegroupssdk.GroupResourcesInput{
					Group: aws.String(groupName), ResourceArns: []string{memberARN},
				})
				require.NoError(t, err)

				return rhtypes.ResourceMapping{
					MappingType:        rhtypes.ResourceMappingTypeResourceGroup,
					ResourceGroupName:  aws.String(groupName),
					PhysicalResourceId: placeholderPhysicalID,
				}
			},
			assert: func(t *testing.T, resources []rhtypes.PhysicalResource) {
				t.Helper()
				require.Len(t, resources, 1, "the group's one real member resource should be discovered")
				assert.Equal(t, "AWS::EC2::Instance", aws.ToString(resources[0].ResourceType))
			},
		},
		{
			name: "eks cluster",
			mapping: func(t *testing.T) rhtypes.ResourceMapping {
				t.Helper()

				clusterName := "rh-eks-" + uuid.NewString()[:8]
				_, err := eksClient.CreateCluster(ctx, &ekssdk.CreateClusterInput{
					Name: aws.String(clusterName), Version: aws.String("1.27"),
					RoleArn:            aws.String("arn:aws:iam::000000000000:role/eks-role"),
					ResourcesVpcConfig: &ekstypes.VpcConfigRequest{SubnetIds: []string{"subnet-12345678"}},
				})
				require.NoError(t, err)

				t.Cleanup(func() {
					cctx, cancel := rhCleanupCtx()
					defer cancel()
					_, _ = eksClient.DeleteCluster(cctx, &ekssdk.DeleteClusterInput{Name: aws.String(clusterName)})
				})

				return rhtypes.ResourceMapping{
					MappingType:        rhtypes.ResourceMappingTypeEks,
					EksSourceName:      aws.String(clusterName + "/default"),
					PhysicalResourceId: placeholderPhysicalID,
				}
			},
			assert: func(t *testing.T, resources []rhtypes.PhysicalResource) {
				t.Helper()
				require.Len(t, resources, 1, "the named EKS cluster should be discovered")
				assert.Equal(t, "AWS::EKS::Cluster", aws.ToString(resources[0].ResourceType))
			},
		},
		{
			name: "app registry app stays unresolved",
			mapping: func(t *testing.T) rhtypes.ResourceMapping {
				t.Helper()

				return rhtypes.ResourceMapping{
					MappingType:        rhtypes.ResourceMappingTypeAppRegistryApp,
					AppRegistryAppName: aws.String("no-such-appregistry-app"),
					PhysicalResourceId: placeholderPhysicalID,
				}
			},
			assert: func(t *testing.T, resources []rhtypes.PhysicalResource) {
				t.Helper()
				assert.Empty(
					t,
					resources,
					"no services/appregistry backend exists in this tree -- must stay honestly unresolved",
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			appArn := createRHApp(ctx, t, client)

			mapping := tt.mapping(t)
			_, err := client.AddDraftAppVersionResourceMappings(
				ctx,
				&resiliencehubsdk.AddDraftAppVersionResourceMappingsInput{
					AppArn: aws.String(appArn), ResourceMappings: []rhtypes.ResourceMapping{mapping},
				},
			)
			require.NoError(t, err, "AddDraftAppVersionResourceMappings should succeed")

			_, err = client.ResolveAppVersionResources(ctx, &resiliencehubsdk.ResolveAppVersionResourcesInput{
				AppArn: aws.String(appArn), AppVersion: aws.String("draft"),
			})
			require.NoError(t, err, "ResolveAppVersionResources should succeed")

			require.Eventually(t, func() bool {
				out, statusErr := client.DescribeAppVersionResourcesResolutionStatus(ctx,
					&resiliencehubsdk.DescribeAppVersionResourcesResolutionStatusInput{
						AppArn: aws.String(appArn), AppVersion: aws.String("draft"),
					})

				return statusErr == nil && out.Status == rhtypes.ResourceResolutionStatusTypeSuccess
			}, 5*time.Second, 50*time.Millisecond, "resolution should reach Success")

			listOut, err := client.ListAppVersionResources(ctx, &resiliencehubsdk.ListAppVersionResourcesInput{
				AppArn: aws.String(appArn), AppVersion: aws.String("draft"),
			})
			require.NoError(t, err)

			tt.assert(t, listOut.PhysicalResources)
		})
	}
}

// importStatusOutput/importResolutionCheck shorten the long SDK output type
// name below the golines line-length limit.
type importStatusOutput = resiliencehubsdk.DescribeDraftAppVersionResourcesImportStatusOutput

type importResolutionCheck func(t *testing.T, statusOut *importStatusOutput, resources []rhtypes.PhysicalResource)

// TestIntegration_ResilienceHub_ImportResourcesResolution proves
// ImportResourcesToDraftAppVersion performs REAL cross-service resolution
// against this emulator's EC2/RDS/DynamoDB/EKS backends (services/
// resiliencehub/cross_service.go) for SourceArns/EksSources -- genuinely
// discovered resources, not fabricated ones. An ARN for a service this
// backend has no cross-service resolution for stays honestly unresolved
// (Success, no resource), while an ARN for a recognized, wired service whose
// resource does not exist fails the import with a real
// ResourceNotFoundException-shaped message (bd: gopherstack-8hw8).
func TestIntegration_ResilienceHub_ImportResourcesResolution(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createResilienceHubClient(t)
	ec2Client := createEC2Client(t)
	rdsClient := createRDSClient(t)
	ddbClient := createDynamoDBClient(t)
	eksClient := createEKSClient(t)

	ec2Out, err := ec2Client.RunInstances(ctx, &ec2sdk.RunInstancesInput{
		ImageId: aws.String("ami-12345678"), InstanceType: ec2types.InstanceTypeT2Micro,
		MinCount: aws.Int32(1), MaxCount: aws.Int32(1),
	})
	require.NoError(t, err, "RunInstances should succeed")
	instanceID := aws.ToString(ec2Out.Instances[0].InstanceId)
	instanceArn := "arn:aws:ec2:us-east-1:000000000000:instance/" + instanceID

	t.Cleanup(func() {
		cctx, cancel := rhCleanupCtx()
		defer cancel()
		_, _ = ec2Client.TerminateInstances(cctx, &ec2sdk.TerminateInstancesInput{InstanceIds: []string{instanceID}})
	})

	dbID := "rh-rds-" + uuid.NewString()[:8]
	rdsOut, err := rdsClient.CreateDBInstance(ctx, &rdssdk.CreateDBInstanceInput{
		DBInstanceIdentifier: aws.String(dbID), DBInstanceClass: aws.String("db.t3.micro"),
		Engine: aws.String("postgres"), MasterUsername: aws.String("admin"),
		MasterUserPassword: aws.String("password123"), AllocatedStorage: aws.Int32(20),
	})
	require.NoError(t, err, "CreateDBInstance should succeed")
	require.NotNil(t, rdsOut.DBInstance)
	// This emulator's DescribeDBInstances/CreateDBInstance XML response never
	// serializes DBInstanceArn (a real, separate, out-of-scope RDS gap this
	// resiliencehub test doesn't own) -- build the ARN the same way
	// services/rds's own internal Get*Arn helpers do (arn.Build("rds",
	// region, account, "db:"+id)) rather than depend on the empty wire field.
	rdsArn := "arn:aws:rds:us-east-1:000000000000:db:" + dbID

	t.Cleanup(func() {
		cctx, cancel := rhCleanupCtx()
		defer cancel()
		_, _ = rdsClient.DeleteDBInstance(cctx, &rdssdk.DeleteDBInstanceInput{
			DBInstanceIdentifier: aws.String(dbID), SkipFinalSnapshot: aws.Bool(true),
		})
	})

	tableName := "rh-ddb-" + uuid.NewString()[:8]
	ddbOut, err := ddbClient.CreateTable(ctx, &ddbsdk.CreateTableInput{
		TableName: aws.String(tableName),
		AttributeDefinitions: []ddbtypes.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: ddbtypes.ScalarAttributeTypeS},
		},
		KeySchema:   []ddbtypes.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: ddbtypes.KeyTypeHash}},
		BillingMode: ddbtypes.BillingModePayPerRequest,
	})
	require.NoError(t, err, "CreateTable should succeed")
	require.NotNil(t, ddbOut.TableDescription)
	// This emulator's CreateTableOutput never serializes TableArn (a real,
	// separate, out-of-scope DynamoDB gap this resiliencehub test doesn't
	// own -- DescribeTable does set it) -- build the ARN the same way
	// services/dynamodb's own CreateTable does internally (arn.Build(
	// "dynamodb", region, account, "table/"+name)) rather than depend on
	// the empty wire field.
	tableArn := "arn:aws:dynamodb:us-east-1:000000000000:table/" + tableName

	t.Cleanup(func() {
		cctx, cancel := rhCleanupCtx()
		defer cancel()
		_, _ = ddbClient.DeleteTable(cctx, &ddbsdk.DeleteTableInput{TableName: aws.String(tableName)})
	})

	clusterName := "rh-eks-import-" + uuid.NewString()[:8]
	_, err = eksClient.CreateCluster(ctx, &ekssdk.CreateClusterInput{
		Name: aws.String(clusterName), Version: aws.String("1.27"),
		RoleArn:            aws.String("arn:aws:iam::000000000000:role/eks-role"),
		ResourcesVpcConfig: &ekstypes.VpcConfigRequest{SubnetIds: []string{"subnet-12345678"}},
	})
	require.NoError(t, err, "CreateCluster should succeed")
	clusterArn := "arn:aws:eks:us-east-1:000000000000:cluster/" + clusterName

	t.Cleanup(func() {
		cctx, cancel := rhCleanupCtx()
		defer cancel()
		_, _ = eksClient.DeleteCluster(cctx, &ekssdk.DeleteClusterInput{Name: aws.String(clusterName)})
	})

	tests := []struct {
		assert     importResolutionCheck
		name       string
		wantStatus rhtypes.ResourceImportStatusType
		sourceArns []string
		eksSources []rhtypes.EksSource
	}{
		{
			name:       "ec2 instance resolves",
			sourceArns: []string{instanceArn},
			wantStatus: rhtypes.ResourceImportStatusTypeSuccess,
			assert: func(t *testing.T, _ *importStatusOutput, resources []rhtypes.PhysicalResource) {
				t.Helper()
				require.Len(t, resources, 1, "the real EC2 instance should be discovered")
				assert.Equal(t, "AWS::EC2::Instance", aws.ToString(resources[0].ResourceType))
			},
		},
		{
			name:       "rds db instance resolves",
			sourceArns: []string{rdsArn},
			wantStatus: rhtypes.ResourceImportStatusTypeSuccess,
			assert: func(t *testing.T, _ *importStatusOutput, resources []rhtypes.PhysicalResource) {
				t.Helper()
				require.Len(t, resources, 1, "the real RDS DB instance should be discovered")
				assert.Equal(t, "AWS::RDS::DBInstance", aws.ToString(resources[0].ResourceType))
			},
		},
		{
			name:       "dynamodb table resolves",
			sourceArns: []string{tableArn},
			wantStatus: rhtypes.ResourceImportStatusTypeSuccess,
			assert: func(t *testing.T, _ *importStatusOutput, resources []rhtypes.PhysicalResource) {
				t.Helper()
				require.Len(t, resources, 1, "the real DynamoDB table should be discovered")
				assert.Equal(t, "AWS::DynamoDB::Table", aws.ToString(resources[0].ResourceType))
			},
		},
		{
			name:       "eks cluster resolves",
			eksSources: []rhtypes.EksSource{{EksClusterArn: aws.String(clusterArn), Namespaces: []string{"default"}}},
			wantStatus: rhtypes.ResourceImportStatusTypeSuccess,
			assert: func(t *testing.T, _ *importStatusOutput, resources []rhtypes.PhysicalResource) {
				t.Helper()
				require.Len(t, resources, 1, "the real EKS cluster should be discovered")
				assert.Equal(t, "AWS::EKS::Cluster", aws.ToString(resources[0].ResourceType))
			},
		},
		{
			name:       "unrecognized service stays unresolved",
			sourceArns: []string{"arn:aws:lambda:us-east-1:000000000000:function:no-such-function"},
			wantStatus: rhtypes.ResourceImportStatusTypeSuccess,
			assert: func(t *testing.T, _ *importStatusOutput, resources []rhtypes.PhysicalResource) {
				t.Helper()
				assert.Empty(
					t, resources,
					"no cross-service resolution exists for lambda ARNs -- must stay honestly unresolved",
				)
			},
		},
		{
			name:       "nonexistent ec2 instance fails with resource not found",
			sourceArns: []string{"arn:aws:ec2:us-east-1:000000000000:instance/i-doesnotexist"},
			wantStatus: rhtypes.ResourceImportStatusTypeFailed,
			assert: func(t *testing.T, statusOut *importStatusOutput, resources []rhtypes.PhysicalResource) {
				t.Helper()
				assert.Empty(t, resources)
				assert.Contains(t, aws.ToString(statusOut.ErrorMessage), "i-doesnotexist")
				require.NotEmpty(t, statusOut.ErrorDetails)
				assert.Contains(t, aws.ToString(statusOut.ErrorDetails[0].ErrorMessage), "not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			appArn := createRHApp(ctx, t, client)

			_, importErr := client.ImportResourcesToDraftAppVersion(
				ctx,
				&resiliencehubsdk.ImportResourcesToDraftAppVersionInput{
					AppArn: aws.String(appArn), SourceArns: tt.sourceArns, EksSources: tt.eksSources,
				},
			)
			require.NoError(t, importErr, "ImportResourcesToDraftAppVersion should succeed")

			var statusOut *importStatusOutput

			require.Eventually(t, func() bool {
				out, statusErr := client.DescribeDraftAppVersionResourcesImportStatus(ctx,
					&resiliencehubsdk.DescribeDraftAppVersionResourcesImportStatusInput{AppArn: aws.String(appArn)})
				if statusErr != nil {
					return false
				}

				statusOut = out

				return out.Status == tt.wantStatus
			}, 5*time.Second, 50*time.Millisecond, "import should reach status %s", tt.wantStatus)

			listOut, listErr := client.ListAppVersionResources(ctx, &resiliencehubsdk.ListAppVersionResourcesInput{
				AppArn: aws.String(appArn), AppVersion: aws.String("draft"),
			})
			require.NoError(t, listErr)

			tt.assert(t, statusOut, listOut.PhysicalResources)
		})
	}
}
