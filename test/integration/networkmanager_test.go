package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	networkmanagersdk "github.com/aws/aws-sdk-go-v2/service/networkmanager"
	nmtypes "github.com/aws/aws-sdk-go-v2/service/networkmanager/types"
	smithy "github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createNetworkManagerClient returns a Network Manager client pointed at the shared test container.
func createNetworkManagerClient(t *testing.T) *networkmanagersdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return networkmanagersdk.NewFromConfig(cfg, func(o *networkmanagersdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// networkManagerCleanupCtx returns a context for use inside t.Cleanup
// callbacks. t.Context() must not be used there: Go 1.24+ cancels it
// before cleanups run.
func networkManagerCleanupCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// networkManagerErrorCode extracts the smithy error code from err, or "" if err isn't one.
func networkManagerErrorCode(err error) string {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode()
	}

	return ""
}

// TestIntegration_NetworkManager_GlobalNetworkSiteDeviceLinkLifecycle drives
// the Global-Networks container hierarchy end to end: create a global
// network, wait for it to reach AVAILABLE, then create a site/device/link
// and associate the device to the link. Also asserts the real
// ResourceNotFoundException for an unknown GlobalNetworkId.
func TestIntegration_NetworkManager_GlobalNetworkSiteDeviceLinkLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createNetworkManagerClient(t)

	gnOut, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{
		Description: aws.String("integ global network"),
	})
	require.NoError(t, err, "CreateGlobalNetwork should succeed")
	gnID := gnOut.GlobalNetwork.GlobalNetworkId
	assert.NotEmpty(t, aws.ToString(gnOut.GlobalNetwork.GlobalNetworkArn))
	assert.Equal(t, nmtypes.GlobalNetworkStatePending, gnOut.GlobalNetwork.State)

	t.Cleanup(func() {
		cctx, cancel := networkManagerCleanupCtx()
		defer cancel()

		_, _ = client.DeleteGlobalNetwork(cctx, &networkmanagersdk.DeleteGlobalNetworkInput{GlobalNetworkId: gnID})
	})

	require.Eventually(t, func() bool {
		out, dErr := client.DescribeGlobalNetworks(ctx, &networkmanagersdk.DescribeGlobalNetworksInput{
			GlobalNetworkIds: []string{aws.ToString(gnID)},
		})

		return dErr == nil && len(out.GlobalNetworks) == 1 &&
			out.GlobalNetworks[0].State == nmtypes.GlobalNetworkStateAvailable
	}, 5*time.Second, 100*time.Millisecond, "global network should reach AVAILABLE")

	siteOut, err := client.CreateSite(ctx, &networkmanagersdk.CreateSiteInput{
		GlobalNetworkId: gnID,
		Location:        &nmtypes.Location{Address: aws.String("1 Integ Way")},
	})
	require.NoError(t, err, "CreateSite should succeed")
	siteID := siteOut.Site.SiteId
	assert.Equal(t, "1 Integ Way", aws.ToString(siteOut.Site.Location.Address))

	deviceOut, err := client.CreateDevice(ctx, &networkmanagersdk.CreateDeviceInput{
		GlobalNetworkId: gnID, SiteId: siteID, Vendor: aws.String("Acme"),
	})
	require.NoError(t, err, "CreateDevice should succeed")
	deviceID := deviceOut.Device.DeviceId

	linkOut, err := client.CreateLink(ctx, &networkmanagersdk.CreateLinkInput{
		GlobalNetworkId: gnID, SiteId: siteID,
		Bandwidth: &nmtypes.Bandwidth{DownloadSpeed: aws.Int32(100), UploadSpeed: aws.Int32(10)},
	})
	require.NoError(t, err, "CreateLink should succeed")
	linkID := linkOut.Link.LinkId

	_, err = client.AssociateLink(ctx, &networkmanagersdk.AssociateLinkInput{
		GlobalNetworkId: gnID, DeviceId: deviceID, LinkId: linkID,
	})
	require.NoError(t, err, "AssociateLink should succeed")

	assocOut, err := client.GetLinkAssociations(ctx, &networkmanagersdk.GetLinkAssociationsInput{
		GlobalNetworkId: gnID, DeviceId: deviceID,
	})
	require.NoError(t, err, "GetLinkAssociations should succeed")
	require.Len(t, assocOut.LinkAssociations, 1)
	assert.Equal(t, aws.ToString(linkID), aws.ToString(assocOut.LinkAssociations[0].LinkId))

	// Real not-found error: an unknown GlobalNetworkId.
	_, err = client.CreateSite(ctx, &networkmanagersdk.CreateSiteInput{GlobalNetworkId: aws.String("nonexistent-gn")})
	require.Error(t, err, "CreateSite against an unknown GlobalNetworkId should fail")

	var nf *nmtypes.ResourceNotFoundException
	require.ErrorAs(t, err, &nf, "should surface as ResourceNotFoundException")
	assert.Equal(t, "ResourceNotFoundException", networkManagerErrorCode(err))
}

// TestIntegration_NetworkManager_CoreNetworkPolicyChangeSet drives the
// versioned core network policy lifecycle: PutCoreNetworkPolicy ->
// ExecuteCoreNetworkChangeSet -> a second PutCoreNetworkPolicy, then asserts
// GetCoreNetworkChangeSet/GetCoreNetworkChangeEvents return a REAL diff
// between the two policy documents (this pass's corenetworkpolicydiff.go),
// not an empty stub. Also asserts CoreNetworkPolicyException for malformed
// policy JSON.
func TestIntegration_NetworkManager_CoreNetworkPolicyChangeSet(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createNetworkManagerClient(t)

	gnOut, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
	require.NoError(t, err, "CreateGlobalNetwork should succeed")
	gnID := gnOut.GlobalNetwork.GlobalNetworkId

	cnOut, err := client.CreateCoreNetwork(ctx, &networkmanagersdk.CreateCoreNetworkInput{GlobalNetworkId: gnID})
	require.NoError(t, err, "CreateCoreNetwork should succeed")
	cnID := cnOut.CoreNetwork.CoreNetworkId
	assert.NotEmpty(t, aws.ToString(cnOut.CoreNetwork.CoreNetworkArn))

	t.Cleanup(func() {
		cctx, cancel := networkManagerCleanupCtx()
		defer cancel()

		_, _ = client.DeleteCoreNetwork(cctx, &networkmanagersdk.DeleteCoreNetworkInput{CoreNetworkId: cnID})
		_, _ = client.DeleteGlobalNetwork(cctx, &networkmanagersdk.DeleteGlobalNetworkInput{GlobalNetworkId: gnID})
	})

	policyV1 := `{"version":"2021.12","core-network-configuration":{"asn-ranges":["64512-64555"]},` +
		`"segments":[{"name":"prod","require-attachment-acceptance":false}]}`

	put1, err := client.PutCoreNetworkPolicy(ctx, &networkmanagersdk.PutCoreNetworkPolicyInput{
		CoreNetworkId: cnID, PolicyDocument: aws.String(policyV1),
	})
	require.NoError(t, err, "PutCoreNetworkPolicy (v1) should succeed")
	v1ID := put1.CoreNetworkPolicy.PolicyVersionId

	require.Eventually(t, func() bool {
		out, gErr := client.GetCoreNetworkPolicy(ctx, &networkmanagersdk.GetCoreNetworkPolicyInput{
			CoreNetworkId: cnID, PolicyVersionId: v1ID,
		})

		return gErr == nil && out.CoreNetworkPolicy.ChangeSetState == nmtypes.ChangeSetStateReadyToExecute
	}, 5*time.Second, 100*time.Millisecond, "v1's change set should become READY_TO_EXECUTE")

	_, err = client.ExecuteCoreNetworkChangeSet(ctx, &networkmanagersdk.ExecuteCoreNetworkChangeSetInput{
		CoreNetworkId: cnID, PolicyVersionId: v1ID,
	})
	require.NoError(t, err, "ExecuteCoreNetworkChangeSet should succeed")

	require.Eventually(t, func() bool {
		out, gErr := client.GetCoreNetworkPolicy(ctx, &networkmanagersdk.GetCoreNetworkPolicyInput{
			CoreNetworkId: cnID, Alias: nmtypes.CoreNetworkPolicyAliasLive,
		})

		return gErr == nil && aws.ToInt32(out.CoreNetworkPolicy.PolicyVersionId) == aws.ToInt32(v1ID)
	}, 5*time.Second, 100*time.Millisecond, "v1 should become the LIVE policy")

	policyV2 := `{"version":"2021.12","core-network-configuration":{"asn-ranges":["64512-64555"]},` +
		`"segments":[{"name":"prod","require-attachment-acceptance":true},` +
		`{"name":"dev","require-attachment-acceptance":false}]}`

	put2, err := client.PutCoreNetworkPolicy(ctx, &networkmanagersdk.PutCoreNetworkPolicyInput{
		CoreNetworkId: cnID, PolicyDocument: aws.String(policyV2),
	})
	require.NoError(t, err, "PutCoreNetworkPolicy (v2) should succeed")
	v2ID := put2.CoreNetworkPolicy.PolicyVersionId

	changeSetOut, err := client.GetCoreNetworkChangeSet(ctx, &networkmanagersdk.GetCoreNetworkChangeSetInput{
		CoreNetworkId: cnID, PolicyVersionId: v2ID,
	})
	require.NoError(t, err, "GetCoreNetworkChangeSet should succeed")
	require.NotEmpty(t, changeSetOut.CoreNetworkChanges, "a real diff between v1 and v2 must be non-empty")

	var sawAdd, sawModify bool

	for _, c := range changeSetOut.CoreNetworkChanges {
		assert.Equal(t, nmtypes.ChangeTypeCoreNetworkSegment, c.Type,
			"this pass's diff engine emits document-level CORE_NETWORK_SEGMENT changes for the segments section")

		switch c.Action {
		case nmtypes.ChangeActionAdd:
			sawAdd = true

			assert.Equal(t, "dev", aws.ToString(c.Identifier))
		case nmtypes.ChangeActionModify:
			sawModify = true

			assert.Equal(t, "prod", aws.ToString(c.Identifier))
		case nmtypes.ChangeActionRemove:
			t.Fatalf("unexpected REMOVE change for identifier %s", aws.ToString(c.Identifier))
		}
	}

	assert.True(t, sawAdd, "adding the dev segment should surface as a real ADD change")
	assert.True(t, sawModify, "changing prod's require-attachment-acceptance should surface as a real MODIFY change")

	eventsOut, err := client.GetCoreNetworkChangeEvents(ctx, &networkmanagersdk.GetCoreNetworkChangeEventsInput{
		CoreNetworkId: cnID, PolicyVersionId: v2ID,
	})
	require.NoError(t, err, "GetCoreNetworkChangeEvents should succeed")
	require.Len(t, eventsOut.CoreNetworkChangeEvents, len(changeSetOut.CoreNetworkChanges))

	for _, e := range eventsOut.CoreNetworkChangeEvents {
		assert.Equal(t, nmtypes.ChangeStatusNotStarted, e.Status, "v2's change set has not been executed yet")
	}

	// Real validation error: malformed policy JSON.
	_, err = client.PutCoreNetworkPolicy(ctx, &networkmanagersdk.PutCoreNetworkPolicyInput{
		CoreNetworkId: cnID, PolicyDocument: aws.String("{not valid json"),
	})
	require.Error(t, err, "malformed policy JSON should be rejected")

	var polErr *nmtypes.CoreNetworkPolicyException
	require.ErrorAs(t, err, &polErr, "should surface as CoreNetworkPolicyException")
}

// TestIntegration_NetworkManager_VpcAttachmentLifecycle drives
// CreateVpcAttachment against real EC2 VPC/Subnet ARNs (this pass's
// cross-service validation, cli.go's wireNetworkManagerEC2), then the
// CREATING -> AVAILABLE attachment state machine via AcceptAttachment.
// Also asserts real ResourceNotFoundException for VpcArn/SubnetArns that do
// not resolve to a real EC2 resource.
func TestIntegration_NetworkManager_VpcAttachmentLifecycle(t *testing.T) { //nolint:tparallel // sequential subtests
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createNetworkManagerClient(t)
	ec2Client := createEC2Client(t)

	gnOut, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
	require.NoError(t, err, "CreateGlobalNetwork should succeed")
	gnID := gnOut.GlobalNetwork.GlobalNetworkId

	cnOut, err := client.CreateCoreNetwork(ctx, &networkmanagersdk.CreateCoreNetworkInput{GlobalNetworkId: gnID})
	require.NoError(t, err, "CreateCoreNetwork should succeed")
	cnID := cnOut.CoreNetwork.CoreNetworkId

	t.Cleanup(func() {
		cctx, cancel := networkManagerCleanupCtx()
		defer cancel()

		_, _ = client.DeleteCoreNetwork(cctx, &networkmanagersdk.DeleteCoreNetworkInput{CoreNetworkId: cnID})
		_, _ = client.DeleteGlobalNetwork(cctx, &networkmanagersdk.DeleteGlobalNetworkInput{GlobalNetworkId: gnID})
	})

	vpcOut, err := ec2Client.CreateVpc(ctx, &ec2sdk.CreateVpcInput{CidrBlock: aws.String("10.55.0.0/16")})
	require.NoError(t, err, "CreateVpc should succeed")
	vpcID := aws.ToString(vpcOut.Vpc.VpcId)

	subnetOut, err := ec2Client.CreateSubnet(ctx, &ec2sdk.CreateSubnetInput{
		VpcId: aws.String(vpcID), CidrBlock: aws.String("10.55.1.0/24"),
	})
	require.NoError(t, err, "CreateSubnet should succeed")
	subnetID := aws.ToString(subnetOut.Subnet.SubnetId)

	t.Cleanup(func() {
		cctx, cancel := networkManagerCleanupCtx()
		defer cancel()

		_, _ = ec2Client.DeleteSubnet(cctx, &ec2sdk.DeleteSubnetInput{SubnetId: aws.String(subnetID)})
		_, _ = ec2Client.DeleteVpc(cctx, &ec2sdk.DeleteVpcInput{VpcId: aws.String(vpcID)})
	})

	vpcArn := "arn:aws:ec2:us-east-1:000000000000:vpc/" + vpcID
	subnetArn := "arn:aws:ec2:us-east-1:000000000000:subnet/" + subnetID

	t.Run("RejectsUnknownEC2References", func(t *testing.T) {
		tests := []struct {
			buildInput   func() *networkmanagersdk.CreateVpcAttachmentInput
			name         string
			wantResource string
		}{
			{
				name: "unknown vpc",
				buildInput: func() *networkmanagersdk.CreateVpcAttachmentInput {
					return &networkmanagersdk.CreateVpcAttachmentInput{
						CoreNetworkId: cnID,
						VpcArn:        aws.String("arn:aws:ec2:us-east-1:000000000000:vpc/vpc-doesnotexist"),
						SubnetArns:    []string{subnetArn},
					}
				},
				wantResource: "VPC",
			},
			{
				name: "unknown subnet",
				buildInput: func() *networkmanagersdk.CreateVpcAttachmentInput {
					return &networkmanagersdk.CreateVpcAttachmentInput{
						CoreNetworkId: cnID,
						VpcArn:        aws.String(vpcArn),
						SubnetArns:    []string{"arn:aws:ec2:us-east-1:000000000000:subnet/subnet-doesnotexist"},
					}
				},
				wantResource: "SUBNET",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				_, attachErr := client.CreateVpcAttachment(ctx, tc.buildInput())
				require.Error(t, attachErr, "a reference to a nonexistent EC2 resource should be rejected")

				var nf *nmtypes.ResourceNotFoundException
				require.ErrorAs(t, attachErr, &nf, "should surface as ResourceNotFoundException")
				assert.Equal(t, tc.wantResource, aws.ToString(nf.ResourceType))
			})
		}
	})

	// Positive: real EC2 VPC/subnet ARNs succeed.
	attOut, err := client.CreateVpcAttachment(ctx, &networkmanagersdk.CreateVpcAttachmentInput{
		CoreNetworkId: cnID, VpcArn: aws.String(vpcArn), SubnetArns: []string{subnetArn},
	})
	require.NoError(t, err, "CreateVpcAttachment with real EC2 VPC/subnet ARNs should succeed")
	attachmentID := attOut.VpcAttachment.Attachment.AttachmentId
	assert.Equal(t, nmtypes.AttachmentStatePendingAttachmentAcceptance, attOut.VpcAttachment.Attachment.State)

	_, err = client.AcceptAttachment(ctx, &networkmanagersdk.AcceptAttachmentInput{AttachmentId: attachmentID})
	require.NoError(t, err, "AcceptAttachment should succeed")

	require.Eventually(t, func() bool {
		out, gErr := client.GetVpcAttachment(ctx, &networkmanagersdk.GetVpcAttachmentInput{AttachmentId: attachmentID})

		return gErr == nil && out.VpcAttachment.Attachment.State == nmtypes.AttachmentStateAvailable
	}, 5*time.Second, 100*time.Millisecond, "VPC attachment should reach AVAILABLE via the CREATING state machine")

	_, err = client.DeleteAttachment(ctx, &networkmanagersdk.DeleteAttachmentInput{AttachmentId: attachmentID})
	require.NoError(t, err, "DeleteAttachment should succeed")
}

// TestIntegration_NetworkManager_ConnectAttachmentAndConnectPeer drives a
// Connect attachment (over a VPC transport attachment) and a Cloud WAN
// Connect peer terminating it, plus the real "parent attachment must exist
// and be type CONNECT" validation.
func TestIntegration_NetworkManager_ConnectAttachmentAndConnectPeer(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createNetworkManagerClient(t)
	ec2Client := createEC2Client(t)

	gnOut, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
	require.NoError(t, err)
	gnID := gnOut.GlobalNetwork.GlobalNetworkId

	cnOut, err := client.CreateCoreNetwork(ctx, &networkmanagersdk.CreateCoreNetworkInput{GlobalNetworkId: gnID})
	require.NoError(t, err)
	cnID := cnOut.CoreNetwork.CoreNetworkId

	t.Cleanup(func() {
		cctx, cancel := networkManagerCleanupCtx()
		defer cancel()

		_, _ = client.DeleteCoreNetwork(cctx, &networkmanagersdk.DeleteCoreNetworkInput{CoreNetworkId: cnID})
		_, _ = client.DeleteGlobalNetwork(cctx, &networkmanagersdk.DeleteGlobalNetworkInput{GlobalNetworkId: gnID})
	})

	vpcOut, err := ec2Client.CreateVpc(ctx, &ec2sdk.CreateVpcInput{CidrBlock: aws.String("10.66.0.0/16")})
	require.NoError(t, err)
	vpcID := aws.ToString(vpcOut.Vpc.VpcId)

	subnetOut, err := ec2Client.CreateSubnet(ctx, &ec2sdk.CreateSubnetInput{
		VpcId: aws.String(vpcID), CidrBlock: aws.String("10.66.1.0/24"),
	})
	require.NoError(t, err)
	subnetID := aws.ToString(subnetOut.Subnet.SubnetId)

	t.Cleanup(func() {
		cctx, cancel := networkManagerCleanupCtx()
		defer cancel()

		_, _ = ec2Client.DeleteSubnet(cctx, &ec2sdk.DeleteSubnetInput{SubnetId: aws.String(subnetID)})
		_, _ = ec2Client.DeleteVpc(cctx, &ec2sdk.DeleteVpcInput{VpcId: aws.String(vpcID)})
	})

	vpcArn := "arn:aws:ec2:us-east-1:000000000000:vpc/" + vpcID
	subnetArn := "arn:aws:ec2:us-east-1:000000000000:subnet/" + subnetID

	transportOut, err := client.CreateVpcAttachment(ctx, &networkmanagersdk.CreateVpcAttachmentInput{
		CoreNetworkId: cnID, VpcArn: aws.String(vpcArn), SubnetArns: []string{subnetArn},
	})
	require.NoError(t, err, "transport VPC attachment should succeed")
	transportID := transportOut.VpcAttachment.Attachment.AttachmentId

	connOut, err := client.CreateConnectAttachment(ctx, &networkmanagersdk.CreateConnectAttachmentInput{
		CoreNetworkId: cnID, EdgeLocation: aws.String("us-east-1"),
		TransportAttachmentId: transportID,
		Options:               &nmtypes.ConnectAttachmentOptions{Protocol: nmtypes.TunnelProtocolNoEncap},
	})
	require.NoError(t, err, "CreateConnectAttachment should succeed")
	connectAttachmentID := connOut.ConnectAttachment.Attachment.AttachmentId

	peerOut, err := client.CreateConnectPeer(ctx, &networkmanagersdk.CreateConnectPeerInput{
		ConnectAttachmentId: connectAttachmentID, PeerAddress: aws.String("10.0.0.1"),
	})
	require.NoError(t, err, "CreateConnectPeer should succeed")
	assert.Equal(t, aws.ToString(connectAttachmentID), aws.ToString(peerOut.ConnectPeer.ConnectAttachmentId))

	listOut, err := client.ListConnectPeers(ctx, &networkmanagersdk.ListConnectPeersInput{
		ConnectAttachmentId: connectAttachmentID,
	})
	require.NoError(t, err, "ListConnectPeers should succeed")

	found := false

	for _, p := range listOut.ConnectPeers {
		if aws.ToString(p.ConnectPeerId) == aws.ToString(peerOut.ConnectPeer.ConnectPeerId) {
			found = true
		}
	}

	assert.True(t, found, "created connect peer should be listed")

	// Real not-found: CreateConnectPeer against a non-CONNECT attachment.
	_, err = client.CreateConnectPeer(ctx, &networkmanagersdk.CreateConnectPeerInput{
		ConnectAttachmentId: transportID, PeerAddress: aws.String("10.0.0.2"),
	})
	require.Error(t, err, "CreateConnectPeer against a non-CONNECT attachment should fail")

	var nf *nmtypes.ResourceNotFoundException
	require.ErrorAs(t, err, &nf)

	_, err = client.DeleteConnectPeer(ctx, &networkmanagersdk.DeleteConnectPeerInput{
		ConnectPeerId: peerOut.ConnectPeer.ConnectPeerId,
	})
	require.NoError(t, err, "DeleteConnectPeer should succeed")
}

// TestIntegration_NetworkManager_Tagging drives TagResource/
// ListTagsForResource/UntagResource against a real GlobalNetwork ARN.
func TestIntegration_NetworkManager_Tagging(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createNetworkManagerClient(t)

	gnOut, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
	require.NoError(t, err)
	gnID := gnOut.GlobalNetwork.GlobalNetworkId
	gnArn := gnOut.GlobalNetwork.GlobalNetworkArn

	t.Cleanup(func() {
		cctx, cancel := networkManagerCleanupCtx()
		defer cancel()

		_, _ = client.DeleteGlobalNetwork(cctx, &networkmanagersdk.DeleteGlobalNetworkInput{GlobalNetworkId: gnID})
	})

	_, err = client.TagResource(ctx, &networkmanagersdk.TagResourceInput{
		ResourceArn: gnArn,
		Tags:        []nmtypes.Tag{{Key: aws.String("env"), Value: aws.String("integ")}},
	})
	require.NoError(t, err, "TagResource should succeed")

	listOut, err := client.ListTagsForResource(ctx, &networkmanagersdk.ListTagsForResourceInput{ResourceArn: gnArn})
	require.NoError(t, err, "ListTagsForResource should succeed")
	require.Len(t, listOut.TagList, 1)
	assert.Equal(t, "env", aws.ToString(listOut.TagList[0].Key))
	assert.Equal(t, "integ", aws.ToString(listOut.TagList[0].Value))

	_, err = client.UntagResource(ctx, &networkmanagersdk.UntagResourceInput{
		ResourceArn: gnArn, TagKeys: []string{"env"},
	})
	require.NoError(t, err, "UntagResource should succeed")

	listOut, err = client.ListTagsForResource(ctx, &networkmanagersdk.ListTagsForResourceInput{ResourceArn: gnArn})
	require.NoError(t, err)
	assert.Empty(t, listOut.TagList)
}

func TestIntegration_NetworkManager_ValidationErrors(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createNetworkManagerClient(t)

	tests := []struct {
		run     func(t *testing.T) error
		name    string
		wantErr bool
	}{
		{
			name: "describe_non_existent_global_network",
			run: func(t *testing.T) error {
				t.Helper()
				_, err := client.DescribeGlobalNetworks(ctx, &networkmanagersdk.DescribeGlobalNetworksInput{
					GlobalNetworkIds: []string{"global-network-nonexistent123"},
				})

				return err
			},
			wantErr: true,
		},
		{
			name: "get_non_existent_site",
			run: func(t *testing.T) error {
				t.Helper()
				_, err := client.GetSites(ctx, &networkmanagersdk.GetSitesInput{
					GlobalNetworkId: aws.String("global-network-nonexistent123"),
					SiteIds:         []string{"site-nonexistent123"},
				})

				return err
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.run(t)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestIntegration_NetworkManager_StartRouteAnalysis drives StartRouteAnalysis
// against real EC2 Transit Gateway route-table state (this pass's
// EC2Resolver-backed graph walk, routeanalysis.go), asserting a genuine
// CONNECTED verdict with a real PathComponent when an active route matches,
// and NO_DESTINATION_ARN_PROVIDED when neither endpoint carries a
// destination.
func TestIntegration_NetworkManager_StartRouteAnalysis(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createNetworkManagerClient(t)
	ec2Client := createEC2Client(t)

	gnOut, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
	require.NoError(t, err)
	gnID := gnOut.GlobalNetwork.GlobalNetworkId

	t.Cleanup(func() {
		cctx, cancel := networkManagerCleanupCtx()
		defer cancel()

		_, _ = client.DeleteGlobalNetwork(cctx, &networkmanagersdk.DeleteGlobalNetworkInput{GlobalNetworkId: gnID})
	})

	vpcOut, err := ec2Client.CreateVpc(ctx, &ec2sdk.CreateVpcInput{CidrBlock: aws.String("10.77.0.0/16")})
	require.NoError(t, err)
	vpcID := aws.ToString(vpcOut.Vpc.VpcId)

	subnetOut, err := ec2Client.CreateSubnet(ctx, &ec2sdk.CreateSubnetInput{
		VpcId: aws.String(vpcID), CidrBlock: aws.String("10.77.1.0/24"),
	})
	require.NoError(t, err)
	subnetID := aws.ToString(subnetOut.Subnet.SubnetId)

	tgwOut, err := ec2Client.CreateTransitGateway(ctx, &ec2sdk.CreateTransitGatewayInput{})
	require.NoError(t, err, "CreateTransitGateway should succeed")
	tgwID := aws.ToString(tgwOut.TransitGateway.TransitGatewayId)

	tgwAttOut, err := ec2Client.CreateTransitGatewayVpcAttachment(ctx, &ec2sdk.CreateTransitGatewayVpcAttachmentInput{
		TransitGatewayId: aws.String(tgwID), VpcId: aws.String(vpcID), SubnetIds: []string{subnetID},
	})
	require.NoError(t, err, "CreateTransitGatewayVpcAttachment should succeed")
	tgwAttachmentID := aws.ToString(tgwAttOut.TransitGatewayVpcAttachment.TransitGatewayAttachmentId)

	rtOut, err := ec2Client.CreateTransitGatewayRouteTable(ctx, &ec2sdk.CreateTransitGatewayRouteTableInput{
		TransitGatewayId: aws.String(tgwID),
	})
	require.NoError(t, err, "CreateTransitGatewayRouteTable should succeed")
	routeTableID := aws.ToString(rtOut.TransitGatewayRouteTable.TransitGatewayRouteTableId)

	_, err = ec2Client.AssociateTransitGatewayRouteTable(ctx, &ec2sdk.AssociateTransitGatewayRouteTableInput{
		TransitGatewayRouteTableId: aws.String(routeTableID), TransitGatewayAttachmentId: aws.String(tgwAttachmentID),
	})
	require.NoError(t, err, "AssociateTransitGatewayRouteTable should succeed")

	_, err = ec2Client.CreateTransitGatewayRoute(ctx, &ec2sdk.CreateTransitGatewayRouteInput{
		TransitGatewayRouteTableId: aws.String(routeTableID),
		DestinationCidrBlock:       aws.String("10.99.0.0/16"),
		TransitGatewayAttachmentId: aws.String(tgwAttachmentID),
	})
	require.NoError(t, err, "CreateTransitGatewayRoute should succeed")

	tgwAttArn := "arn:aws:ec2:us-east-1:000000000000:transit-gateway-attachment/" + tgwAttachmentID

	startOut, err := client.StartRouteAnalysis(ctx, &networkmanagersdk.StartRouteAnalysisInput{
		GlobalNetworkId: gnID,
		Source: &nmtypes.RouteAnalysisEndpointOptionsSpecification{
			TransitGatewayAttachmentArn: aws.String(tgwAttArn),
		},
		Destination: &nmtypes.RouteAnalysisEndpointOptionsSpecification{IpAddress: aws.String("10.99.0.5")},
	})
	require.NoError(t, err, "StartRouteAnalysis should succeed")
	analysisID := startOut.RouteAnalysis.RouteAnalysisId
	assert.Equal(t, nmtypes.RouteAnalysisStatusRunning, startOut.RouteAnalysis.Status)

	require.Eventually(t, func() bool {
		out, gErr := client.GetRouteAnalysis(ctx, &networkmanagersdk.GetRouteAnalysisInput{
			GlobalNetworkId: gnID, RouteAnalysisId: analysisID,
		})

		return gErr == nil && out.RouteAnalysis.Status == nmtypes.RouteAnalysisStatusCompleted
	}, 5*time.Second, 100*time.Millisecond, "route analysis should complete")

	finalOut, err := client.GetRouteAnalysis(ctx, &networkmanagersdk.GetRouteAnalysisInput{
		GlobalNetworkId: gnID, RouteAnalysisId: analysisID,
	})
	require.NoError(t, err)
	require.NotNil(t, finalOut.RouteAnalysis.ForwardPath)
	require.NotNil(t, finalOut.RouteAnalysis.ForwardPath.CompletionStatus)
	assert.Equal(
		t, nmtypes.RouteAnalysisCompletionResultCodeConnected,
		finalOut.RouteAnalysis.ForwardPath.CompletionStatus.ResultCode,
		"a real active EC2 TGW route to the destination CIDR should resolve CONNECTED",
	)
	assert.NotEmpty(
		t,
		finalOut.RouteAnalysis.ForwardPath.Path,
		"a real graph walk should populate a real PathComponent",
	)

	// Negative: neither endpoint carries a destination.
	startOut2, err := client.StartRouteAnalysis(ctx, &networkmanagersdk.StartRouteAnalysisInput{
		GlobalNetworkId: gnID,
		Source:          &nmtypes.RouteAnalysisEndpointOptionsSpecification{},
		Destination:     &nmtypes.RouteAnalysisEndpointOptionsSpecification{},
	})
	require.NoError(t, err)
	analysisID2 := startOut2.RouteAnalysis.RouteAnalysisId

	require.Eventually(t, func() bool {
		out, gErr := client.GetRouteAnalysis(ctx, &networkmanagersdk.GetRouteAnalysisInput{
			GlobalNetworkId: gnID, RouteAnalysisId: analysisID2,
		})

		return gErr == nil && out.RouteAnalysis.Status == nmtypes.RouteAnalysisStatusCompleted
	}, 5*time.Second, 100*time.Millisecond, "route analysis should complete")

	finalOut2, err := client.GetRouteAnalysis(ctx, &networkmanagersdk.GetRouteAnalysisInput{
		GlobalNetworkId: gnID, RouteAnalysisId: analysisID2,
	})
	require.NoError(t, err)
	assert.Equal(
		t, nmtypes.RouteAnalysisCompletionResultCodeNotConnected,
		finalOut2.RouteAnalysis.ForwardPath.CompletionStatus.ResultCode,
	)
	assert.Equal(
		t, nmtypes.RouteAnalysisCompletionReasonCodeNoDestinationArnProvided,
		finalOut2.RouteAnalysis.ForwardPath.CompletionStatus.ReasonCode,
	)
}
