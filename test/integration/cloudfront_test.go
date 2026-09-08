package integration_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// remain the only way to build the legacy DistributionConfig this test exercises.
//
//nolint:staticcheck // ForwardedValues/MinTTL are deprecated for CachePolicyId, but
func minimalCFDistributionConfig(callerRef, comment string) *cftypes.DistributionConfig {
	return &cftypes.DistributionConfig{
		CallerReference: aws.String(callerRef),
		Comment:         aws.String(comment),
		Enabled:         aws.Bool(true),
		Origins: &cftypes.Origins{
			Quantity: aws.Int32(1),
			Items: []cftypes.Origin{
				{
					Id:         aws.String("origin-1"),
					DomainName: aws.String("example.com"),
					S3OriginConfig: &cftypes.S3OriginConfig{
						OriginAccessIdentity: aws.String(""),
					},
				},
			},
		},
		DefaultCacheBehavior: &cftypes.DefaultCacheBehavior{
			ViewerProtocolPolicy: cftypes.ViewerProtocolPolicyAllowAll,
			TargetOriginId:       aws.String("origin-1"),
			ForwardedValues: &cftypes.ForwardedValues{
				QueryString: aws.Bool(false),
				Cookies: &cftypes.CookiePreference{
					Forward: cftypes.ItemSelectionNone,
				},
			},
			MinTTL: aws.Int64(0),
		},
	}
}

func TestIntegration_CloudFront_DistributionLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createCloudFrontClient(t)
	ctx := t.Context()

	callerRef := "ref-" + uuid.NewString()[:8]
	comment := "test-dist-" + uuid.NewString()[:8]

	// CreateDistribution
	createOut, err := client.CreateDistribution(ctx, &cloudfront.CreateDistributionInput{
		DistributionConfig: minimalCFDistributionConfig(callerRef, comment),
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.Distribution)

	distID := aws.ToString(createOut.Distribution.Id)
	require.NotEmpty(t, distID)
	assert.Equal(t, "Deployed", aws.ToString(createOut.Distribution.Status))

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		getOut, gErr := client.GetDistribution(cleanupCtx, &cloudfront.GetDistributionInput{
			Id: aws.String(distID),
		})
		if gErr != nil {
			return
		}

		_, _ = client.DeleteDistribution(cleanupCtx, &cloudfront.DeleteDistributionInput{
			Id:      aws.String(distID),
			IfMatch: getOut.ETag,
		})
	})

	// GetDistribution
	getOut, err := client.GetDistribution(ctx, &cloudfront.GetDistributionInput{
		Id: aws.String(distID),
	})
	require.NoError(t, err)
	require.NotNil(t, getOut.Distribution)
	assert.Equal(t, distID, aws.ToString(getOut.Distribution.Id))
	assert.Equal(t, comment, aws.ToString(getOut.Distribution.DistributionConfig.Comment))

	// ListDistributions
	listOut, err := client.ListDistributions(ctx, &cloudfront.ListDistributionsInput{})
	require.NoError(t, err)
	require.NotNil(t, listOut.DistributionList)

	found := false

	for _, d := range listOut.DistributionList.Items {
		if aws.ToString(d.Id) == distID {
			found = true

			break
		}
	}

	assert.True(t, found, "created distribution should appear in list")

	// CloudFront requires a distribution to be disabled before it can be
	// deleted (matching real AWS). Disable it first, then delete with the
	// ETag returned by the update.
	disableCfg := getOut.Distribution.DistributionConfig
	disableCfg.Enabled = aws.Bool(false)

	updOut, err := client.UpdateDistribution(ctx, &cloudfront.UpdateDistributionInput{
		Id:                 aws.String(distID),
		IfMatch:            getOut.ETag,
		DistributionConfig: disableCfg,
	})
	require.NoError(t, err)

	// DeleteDistribution
	_, err = client.DeleteDistribution(ctx, &cloudfront.DeleteDistributionInput{
		Id:      aws.String(distID),
		IfMatch: updOut.ETag,
	})
	require.NoError(t, err)

	// Verify deleted
	listOut2, err := client.ListDistributions(ctx, &cloudfront.ListDistributionsInput{})
	require.NoError(t, err)

	for _, d := range listOut2.DistributionList.Items {
		assert.NotEqual(
			t,
			distID,
			aws.ToString(d.Id),
			"deleted distribution should not appear in list",
		)
	}
}

// TestIntegration_CloudFront_DistributionStatusTransition proves
// UpdateDistribution's InProgress -> Deployed async transition
// (services/cloudfront/distributions.go's scheduleDistributionDeployed) is
// observable through the real SDK: UpdateDistribution's own response
// reports the real intermediate InProgress status, and a client polling
// GetDistribution afterward sees it settle back to Deployed on its own,
// with no further API call needed to drive it.
func TestIntegration_CloudFront_DistributionStatusTransition(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createCloudFrontClient(t)
	ctx := t.Context()

	callerRef := "ref-" + uuid.NewString()[:8]

	createOut, err := client.CreateDistribution(ctx, &cloudfront.CreateDistributionInput{
		DistributionConfig: minimalCFDistributionConfig(callerRef, "status-transition"),
	})
	require.NoError(t, err)
	distID := aws.ToString(createOut.Distribution.Id)
	require.Equal(t, "Deployed", aws.ToString(createOut.Distribution.Status))

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		getOut, gErr := client.GetDistribution(cleanupCtx, &cloudfront.GetDistributionInput{Id: aws.String(distID)})
		if gErr != nil {
			return
		}

		_, _ = client.DeleteDistribution(cleanupCtx, &cloudfront.DeleteDistributionInput{
			Id: aws.String(distID), IfMatch: getOut.ETag,
		})
	})

	getOut, err := client.GetDistribution(ctx, &cloudfront.GetDistributionInput{Id: aws.String(distID)})
	require.NoError(t, err)

	updatedConfig := getOut.Distribution.DistributionConfig
	updatedConfig.Comment = aws.String("updated-" + uuid.NewString()[:8])

	updOut, err := client.UpdateDistribution(ctx, &cloudfront.UpdateDistributionInput{
		Id: aws.String(distID), IfMatch: getOut.ETag, DistributionConfig: updatedConfig,
	})
	require.NoError(t, err)
	assert.Equal(t, "InProgress", aws.ToString(updOut.Distribution.Status),
		"UpdateDistribution should return the real intermediate InProgress status")

	require.Eventually(t, func() bool {
		out, gErr := client.GetDistribution(ctx, &cloudfront.GetDistributionInput{Id: aws.String(distID)})

		return gErr == nil && aws.ToString(out.Distribution.Status) == "Deployed"
	}, 5*time.Second, 50*time.Millisecond, "distribution should transition back to Deployed on its own")
}

func TestIntegration_CloudFront_GetDistributionNotFound(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createCloudFrontClient(t)
	ctx := t.Context()

	_, err := client.GetDistribution(ctx, &cloudfront.GetDistributionInput{
		Id: aws.String("DOESNOTEXIST"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NoSuchDistribution")
}
