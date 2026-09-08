package cloudfront_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cfsdk "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudfront"
)

func newSentinelTestHandler(t *testing.T) *cloudfront.Handler {
	t.Helper()

	backend := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")

	return cloudfront.NewHandler(backend)
}

// TestAssociateDistributionWebACL_UnknownDistribution_EntityNotFound proves
// AssociateDistributionWebACL reports an unknown distribution ID via the
// code its own deserializer models. cloudfront@v1.67.4 deserializers.go's
// awsRestxml_deserializeOpErrorAssociateDistributionWebACL switch models
// EntityNotFound, not NoSuchDistribution -- unlike most distribution ops.
func TestAssociateDistributionWebACL_UnknownDistribution_EntityNotFound(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.AssociateDistributionWebACL(t.Context(), &cfsdk.AssociateDistributionWebACLInput{
		Id:        aws.String("NOSUCHDIST"),
		WebACLArn: aws.String("arn:aws:wafv2:us-east-1:123456789012:global/webacl/x/1"),
	})
	require.Error(t, err)

	var enf *types.EntityNotFound
	require.ErrorAsf(t, err, &enf, "expected a real EntityNotFound from the SDK deserializer, got %v", err)
}

// TestTagResource_UnknownARN_NoSuchResource proves TagResource reports an
// unrecognized ARN via the code its own deserializer models.
// cloudfront@v1.67.4 deserializers.go's awsRestxml_deserializeOpErrorTagResource
// switch models NoSuchResource, not NoSuchDistribution.
func TestTagResource_UnknownARN_NoSuchResource(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.TagResource(t.Context(), &cfsdk.TagResourceInput{
		Resource: aws.String("arn:aws:cloudfront::123456789012:distribution/NOSUCHDIST"),
		Tags: &types.Tags{
			Items: []types.Tag{{Key: aws.String("k"), Value: aws.String("v")}},
		},
	})
	require.Error(t, err)

	var nsr *types.NoSuchResource
	require.ErrorAsf(t, err, &nsr, "expected a real NoSuchResource from the SDK deserializer, got %v", err)
}

// TestGetConnectionGroup_UnknownID_EntityNotFound proves GetConnectionGroup
// reports an unknown ID via EntityNotFound, not a fabricated
// "NoSuchConnectionGroup" -- confirmed against
// awsRestxml_deserializeOpErrorGetConnectionGroup, whose switch has no case
// for that code (it does not exist anywhere in the pinned SDK).
func TestGetConnectionGroup_UnknownID_EntityNotFound(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.GetConnectionGroup(t.Context(), &cfsdk.GetConnectionGroupInput{
		Identifier: aws.String("no-such-cg"),
	})
	require.Error(t, err)

	var enf *types.EntityNotFound
	require.ErrorAsf(t, err, &enf, "expected a real EntityNotFound from the SDK deserializer, got %v", err)
}

// TestGetDistributionTenant_UnknownID_EntityNotFound is
// TestGetConnectionGroup_UnknownID_EntityNotFound's sibling for distribution
// tenants -- confirmed against
// awsRestxml_deserializeOpErrorGetDistributionTenant.
func TestGetDistributionTenant_UnknownID_EntityNotFound(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.GetDistributionTenant(t.Context(), &cfsdk.GetDistributionTenantInput{
		Identifier: aws.String("no-such-tenant"),
	})
	require.Error(t, err)

	var enf *types.EntityNotFound
	require.ErrorAsf(t, err, &enf, "expected a real EntityNotFound from the SDK deserializer, got %v", err)
}

// TestGetTrustStore_UnknownID_EntityNotFound is
// TestGetConnectionGroup_UnknownID_EntityNotFound's sibling for trust
// stores -- confirmed against awsRestxml_deserializeOpErrorGetTrustStore.
func TestGetTrustStore_UnknownID_EntityNotFound(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.GetTrustStore(t.Context(), &cfsdk.GetTrustStoreInput{
		Identifier: aws.String("no-such-truststore"),
	})
	require.Error(t, err)

	var enf *types.EntityNotFound
	require.ErrorAsf(t, err, &enf, "expected a real EntityNotFound from the SDK deserializer, got %v", err)
}

// TestGetVpcOrigin_UnknownID_EntityNotFound is
// TestGetConnectionGroup_UnknownID_EntityNotFound's sibling for VPC
// origins -- confirmed against awsRestxml_deserializeOpErrorGetVpcOrigin.
func TestGetVpcOrigin_UnknownID_EntityNotFound(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.GetVpcOrigin(t.Context(), &cfsdk.GetVpcOriginInput{
		Id: aws.String("no-such-vpc-origin"),
	})
	require.Error(t, err)

	var enf *types.EntityNotFound
	require.ErrorAsf(t, err, &enf, "expected a real EntityNotFound from the SDK deserializer, got %v", err)
}

// TestUpdateDomainAssociation_UnknownTargetDistribution_EntityNotFound
// proves UpdateDomainAssociation reports an unknown target distribution ID
// via EntityNotFound, not NoSuchDistribution -- confirmed against
// awsRestxml_deserializeOpErrorUpdateDomainAssociation.
func TestUpdateDomainAssociation_UnknownTargetDistribution_EntityNotFound(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.UpdateDomainAssociation(t.Context(), &cfsdk.UpdateDomainAssociationInput{
		Domain: aws.String("example.com"),
		TargetResource: &types.DistributionResourceId{
			DistributionId: aws.String("NOSUCHDIST"),
		},
	})
	require.Error(t, err)

	var enf *types.EntityNotFound
	require.ErrorAsf(t, err, &enf, "expected a real EntityNotFound from the SDK deserializer, got %v", err)
}

// TestGetAnycastIpList_UnknownID_EntityNotFound proves GetAnycastIpList
// reports an unknown ID via EntityNotFound, not a fabricated
// "NoSuchAnycastIPList" -- confirmed against
// awsRestxml_deserializeOpErrorGetAnycastIpList, whose switch has no case
// for that code (it does not exist anywhere in the pinned SDK).
func TestGetAnycastIpList_UnknownID_EntityNotFound(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.GetAnycastIpList(t.Context(), &cfsdk.GetAnycastIpListInput{
		Id: aws.String("no-such-anycast-ip-list"),
	})
	require.Error(t, err)

	var enf *types.EntityNotFound
	require.ErrorAsf(t, err, &enf, "expected a real EntityNotFound from the SDK deserializer, got %v", err)
}

// TestUpdateAnycastIpList_UnknownID_EntityNotFound is
// TestGetAnycastIpList_UnknownID_EntityNotFound's sibling for Update --
// confirmed against awsRestxml_deserializeOpErrorUpdateAnycastIpList.
func TestUpdateAnycastIpList_UnknownID_EntityNotFound(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.UpdateAnycastIpList(t.Context(), &cfsdk.UpdateAnycastIpListInput{
		Id:      aws.String("no-such-anycast-ip-list"),
		IfMatch: aws.String("any-etag"),
	})
	require.Error(t, err)

	var enf *types.EntityNotFound
	require.ErrorAsf(t, err, &enf, "expected a real EntityNotFound from the SDK deserializer, got %v", err)
}

// TestDeleteAnycastIpList_UnknownID_EntityNotFound is
// TestGetAnycastIpList_UnknownID_EntityNotFound's sibling for Delete --
// confirmed against awsRestxml_deserializeOpErrorDeleteAnycastIpList.
func TestDeleteAnycastIpList_UnknownID_EntityNotFound(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.DeleteAnycastIpList(t.Context(), &cfsdk.DeleteAnycastIpListInput{
		Id:      aws.String("no-such-anycast-ip-list"),
		IfMatch: aws.String("any-etag"),
	})
	require.Error(t, err)

	var enf *types.EntityNotFound
	require.ErrorAsf(t, err, &enf, "expected a real EntityNotFound from the SDK deserializer, got %v", err)
}

// TestCreateKeyGroup_UnknownPublicKey_InvalidArgument proves CreateKeyGroup
// reports a nonexistent referenced public key via InvalidArgument, the only
// client-fault code its own deserializer models -- not a fabricated
// "NoSuchPublicKey" (that code is real for GetPublicKey/UpdatePublicKey/
// DeletePublicKey, but CreateKeyGroup's own switch, confirmed against
// awsRestxml_deserializeOpErrorCreateKeyGroup, has no case for it).
func TestCreateKeyGroup_UnknownPublicKey_InvalidArgument(t *testing.T) {
	t.Parallel()

	h := newSentinelTestHandler(t)
	client := newTestCloudFrontClient(t, h)

	_, err := client.CreateKeyGroup(t.Context(), &cfsdk.CreateKeyGroupInput{
		KeyGroupConfig: &types.KeyGroupConfig{
			Name:  aws.String("kg1"),
			Items: []string{"no-such-public-key"},
		},
	})
	require.Error(t, err)

	var ia *types.InvalidArgument
	require.ErrorAsf(t, err, &ia, "expected a real InvalidArgument from the SDK deserializer, got %v", err)
}
