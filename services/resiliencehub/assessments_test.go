package resiliencehub_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	resiliencehubsdk "github.com/aws/aws-sdk-go-v2/service/resiliencehub"
	"github.com/aws/aws-sdk-go-v2/service/resiliencehub/types"
	"github.com/stretchr/testify/require"
)

// TestStartAppAssessment_ComplianceStatusRule verifies the one documented
// coarse compliance rule this backend applies (see
// complianceStatusForPolicy's doc comment): MissingPolicy when no policy is
// bound, PolicyMet when one is -- never a fabricated score-derived status.
func TestStartAppAssessment_ComplianceStatusRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus types.ComplianceStatus
		bindPolicy bool
	}{
		{name: "no policy bound", bindPolicy: false, wantStatus: types.ComplianceStatusMissingPolicy},
		{name: "policy bound", bindPolicy: true, wantStatus: types.ComplianceStatusPolicyMet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, client := newTestHandlerAndClient(t)
			ctx := t.Context()

			createReq := &resiliencehubsdk.CreateAppInput{Name: aws.String("app")}

			if tt.bindPolicy {
				policyOut, err := client.CreateResiliencyPolicy(ctx, &resiliencehubsdk.CreateResiliencyPolicyInput{
					PolicyName: aws.String("p"), Tier: types.ResiliencyPolicyTierCritical,
					Policy: map[string]types.FailurePolicy{
						"Software": {RtoInSecs: 60, RpoInSecs: 60},
						"Hardware": {RtoInSecs: 60, RpoInSecs: 60},
						"AZ":       {RtoInSecs: 60, RpoInSecs: 60},
						"Region":   {RtoInSecs: 60, RpoInSecs: 60},
					},
				})
				require.NoError(t, err)
				createReq.PolicyArn = policyOut.Policy.PolicyArn
			}

			appOut, err := client.CreateApp(ctx, createReq)
			require.NoError(t, err)

			started, err := client.StartAppAssessment(ctx, &resiliencehubsdk.StartAppAssessmentInput{
				AppArn: appOut.App.AppArn, AppVersion: aws.String("draft"), AssessmentName: aws.String("a1"),
			})
			require.NoError(t, err)

			var final *resiliencehubsdk.DescribeAppAssessmentOutput

			require.Eventually(t, func() bool {
				var descErr error

				final, descErr = client.DescribeAppAssessment(
					ctx, &resiliencehubsdk.DescribeAppAssessmentInput{AssessmentArn: started.Assessment.AssessmentArn},
				)
				require.NoError(t, descErr)

				return final.Assessment.AssessmentStatus == types.AssessmentStatusSuccess
			}, defaultAsyncWait, defaultAsyncPoll)

			require.Equal(t, tt.wantStatus, final.Assessment.ComplianceStatus)
			require.Nil(t, final.Assessment.Summary, "AssessmentSummary must never be fabricated")
			require.InDelta(
				t,
				0.0,
				final.Assessment.ResiliencyScore.Score,
				0,
				"ResiliencyScore must be the documented placeholder",
			)
		})
	}
}

// TestDeleteAppAssessment_RejectsWhileRunning verifies DeleteAppAssessment is
// rejected (ConflictException) while the assessment is still Pending.
func TestDeleteAppAssessment_RejectsWhileRunning(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	appOut, err := client.CreateApp(ctx, &resiliencehubsdk.CreateAppInput{Name: aws.String("app")})
	require.NoError(t, err)

	started, err := client.StartAppAssessment(ctx, &resiliencehubsdk.StartAppAssessmentInput{
		AppArn: appOut.App.AppArn, AppVersion: aws.String("draft"), AssessmentName: aws.String("a1"),
	})
	require.NoError(t, err)

	_, err = client.DeleteAppAssessment(ctx, &resiliencehubsdk.DeleteAppAssessmentInput{
		AssessmentArn: started.Assessment.AssessmentArn,
	})
	require.Error(t, err)

	var conflict *types.ConflictException
	require.ErrorAs(t, err, &conflict)
}

// TestListAppAssessmentDrifts_UnknownArnStillEmpty verifies
// ListAppAssessmentComplianceDrifts/ListAppAssessmentResourceDrifts do NOT
// surface ResourceNotFoundException for an unknown AssessmentArn
// (gopherstack-ulsj) -- confirmed via the SDK's own deserializeOpError<Op>
// switch in deserializers.go that neither op's declared error set includes
// ResourceNotFoundException, unlike a sibling op that keys on the same
// resource (DescribeAppAssessment, which does declare it and still must).
func TestListAppAssessmentDrifts_UnknownArnStillEmpty(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	unknownArn := aws.String("arn:aws:resiliencehub:us-east-1:000000000000:app-assessment/nonexistent")

	compliance, err := client.ListAppAssessmentComplianceDrifts(
		ctx, &resiliencehubsdk.ListAppAssessmentComplianceDriftsInput{AssessmentArn: unknownArn},
	)
	require.NoError(t, err)
	require.Empty(t, compliance.ComplianceDrifts)

	resource, err := client.ListAppAssessmentResourceDrifts(
		ctx, &resiliencehubsdk.ListAppAssessmentResourceDriftsInput{AssessmentArn: unknownArn},
	)
	require.NoError(t, err)
	require.Empty(t, resource.ResourceDrifts)

	// DescribeAppAssessment, which keys on the same AssessmentArn, must still
	// reject the same unknown ARN -- proves the fix is scoped to these two
	// ops, not a service-wide relaxation of assessment-arn validation.
	_, err = client.DescribeAppAssessment(ctx, &resiliencehubsdk.DescribeAppAssessmentInput{AssessmentArn: unknownArn})
	require.Error(t, err)

	var nf *types.ResourceNotFoundException
	require.ErrorAs(t, err, &nf)
}
