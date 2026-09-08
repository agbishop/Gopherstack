package lightsail_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lightsailsdk "github.com/aws/aws-sdk-go-v2/service/lightsail"
	lightsailtypes "github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/stretchr/testify/require"
)

// TestDeleteBucket_RequiresForceDeleteForAccessKeys proves DeleteBucket
// enforces one of ForceDelete's own documented conditions
// (api_op_DeleteBucket.go: "You must force delete the bucket if it has one
// of the following conditions: ... The bucket has access keys.").
func TestDeleteBucket_RequiresForceDeleteForAccessKeys(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	ctx := t.Context()

	_, err := client.CreateBucket(ctx, &lightsailsdk.CreateBucketInput{
		BucketName: aws.String("bucket-with-keys"), BundleId: aws.String("small_1_0"),
	})
	require.NoError(t, err)

	_, err = client.CreateBucketAccessKey(
		ctx,
		&lightsailsdk.CreateBucketAccessKeyInput{BucketName: aws.String("bucket-with-keys")},
	)
	require.NoError(t, err)

	_, err = client.DeleteBucket(ctx, &lightsailsdk.DeleteBucketInput{BucketName: aws.String("bucket-with-keys")})
	require.Error(t, err)
	require.ErrorContains(t, err, "access keys")

	_, err = client.DeleteBucket(ctx, &lightsailsdk.DeleteBucketInput{
		BucketName: aws.String("bucket-with-keys"), ForceDelete: aws.Bool(true),
	})
	require.NoError(t, err, "ForceDelete must override the access-keys guard")
}

// TestDeleteBucket_RequiresForceDeleteForDistributionOrigin proves
// DeleteBucket enforces another of ForceDelete's documented conditions
// (api_op_DeleteBucket.go: "The bucket is the origin of a distribution.").
func TestDeleteBucket_RequiresForceDeleteForDistributionOrigin(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	ctx := t.Context()

	_, err := client.CreateBucket(ctx, &lightsailsdk.CreateBucketInput{
		BucketName: aws.String("bucket-as-origin"), BundleId: aws.String("small_1_0"),
	})
	require.NoError(t, err)

	_, err = client.CreateDistribution(ctx, &lightsailsdk.CreateDistributionInput{
		DistributionName: aws.String("origin-dist"), BundleId: aws.String("small_1_0"),
		Origin:               &lightsailtypes.InputOrigin{Name: aws.String("bucket-as-origin")},
		DefaultCacheBehavior: &lightsailtypes.CacheBehavior{Behavior: lightsailtypes.BehaviorEnum("cache")},
	})
	require.NoError(t, err)

	_, err = client.DeleteBucket(ctx, &lightsailsdk.DeleteBucketInput{BucketName: aws.String("bucket-as-origin")})
	require.Error(t, err)
	require.ErrorContains(t, err, "origin of distribution")

	_, err = client.DeleteBucket(ctx, &lightsailsdk.DeleteBucketInput{
		BucketName: aws.String("bucket-as-origin"), ForceDelete: aws.Bool(true),
	})
	require.NoError(t, err, "ForceDelete must override the distribution-origin guard")
}
