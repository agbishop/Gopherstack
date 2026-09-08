package directconnect_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	directconnectsdk "github.com/aws/aws-sdk-go-v2/service/directconnect"
	"github.com/aws/aws-sdk-go-v2/service/directconnect/types"
	"github.com/stretchr/testify/require"
)

// TestHandlerReset verifies Handler.Reset() clears backend state (used by
// the POST /_gopherstack/reset endpoint for CI pipelines), exercising
// newTestHandlerAndClient's Handler return value.
func TestHandlerReset(t *testing.T) {
	t.Parallel()

	h, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	created := createTestConnection(t, client)

	h.Reset()

	out, err := client.DescribeConnections(ctx, &directconnectsdk.DescribeConnectionsInput{
		ConnectionId: created.ConnectionId,
	})
	require.NoError(t, err)
	require.Empty(t, out.Connections)
}

// TestRoundTripConnectionLifecycle drives CreateConnection through the real
// SDK client and asserts the full requested->pending->available auto
// transition, catching wire-shape bugs a hand-rolled JSON assertion would
// miss (this service's lowerCamelCase keys, and the flat, non-nested
// Connection output shape).
func TestRoundTripConnectionLifecycle(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	created, err := client.CreateConnection(ctx, &directconnectsdk.CreateConnectionInput{
		Bandwidth:      aws.String("1Gbps"),
		ConnectionName: aws.String("test-conn"),
		Location:       aws.String("EqDC2"),
		Tags:           []types.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
	})
	require.NoError(t, err)
	require.NotNil(t, created.ConnectionId)
	require.Equal(t, types.ConnectionStateRequested, created.ConnectionState)
	require.Equal(t, "test-conn", aws.ToString(created.ConnectionName))
	require.Len(t, created.Tags, 1)

	require.Eventually(t, func() bool {
		out, descErr := client.DescribeConnections(ctx, &directconnectsdk.DescribeConnectionsInput{
			ConnectionId: created.ConnectionId,
		})

		return descErr == nil && len(out.Connections) == 1 &&
			out.Connections[0].ConnectionState == types.ConnectionStateAvailable
	}, defaultAsyncWait, defaultAsyncPoll)

	updated, err := client.UpdateConnection(ctx, &directconnectsdk.UpdateConnectionInput{
		ConnectionId:   created.ConnectionId,
		ConnectionName: aws.String("renamed-conn"),
	})
	require.NoError(t, err)
	require.Equal(t, "renamed-conn", aws.ToString(updated.ConnectionName))

	deleted, err := client.DeleteConnection(ctx, &directconnectsdk.DeleteConnectionInput{
		ConnectionId: created.ConnectionId,
	})
	require.NoError(t, err)
	require.Equal(t, types.ConnectionStateDeleting, deleted.ConnectionState)
}

// TestRoundTripLagLifecycle exercises CreateLag/DescribeLags/UpdateLag/
// DeleteLag, including the nested Connections[] list built from live
// Connection records.
func TestRoundTripLagLifecycle(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	lag, err := client.CreateLag(ctx, &directconnectsdk.CreateLagInput{
		ConnectionsBandwidth: aws.String("1Gbps"),
		LagName:              aws.String("test-lag"),
		Location:             aws.String("EqDC2"),
		NumberOfConnections:  2,
	})
	require.NoError(t, err)
	require.Len(t, lag.Connections, 2)
	require.Equal(t, types.LagStateRequested, lag.LagState)

	describeOut, err := client.DescribeLags(ctx, &directconnectsdk.DescribeLagsInput{LagId: lag.LagId})
	require.NoError(t, err)
	require.Len(t, describeOut.Lags, 1)
	require.Len(t, describeOut.Lags[0].Connections, 2)

	updated, err := client.UpdateLag(ctx, &directconnectsdk.UpdateLagInput{
		LagId:        lag.LagId,
		MinimumLinks: 1,
	})
	require.NoError(t, err)
	require.Equal(t, int32(1), updated.MinimumLinks)

	// DeleteLag rejects a LAG with active member connections
	// (api_op_DeleteLag.go:12-13); disassociate both before deleting.
	for _, c := range lag.Connections {
		_, err = client.DisassociateConnectionFromLag(ctx, &directconnectsdk.DisassociateConnectionFromLagInput{
			ConnectionId: c.ConnectionId,
			LagId:        lag.LagId,
		})
		require.NoError(t, err)
	}

	deleted, err := client.DeleteLag(ctx, &directconnectsdk.DeleteLagInput{LagId: lag.LagId})
	require.NoError(t, err)
	require.Equal(t, types.LagStateDeleting, deleted.LagState)
}

// createTestConnection is a small helper that creates a standard connection
// via the real SDK client, for tests whose focus is a different op.
func createTestConnection(t *testing.T, client *directconnectsdk.Client) *directconnectsdk.CreateConnectionOutput {
	t.Helper()

	out, err := client.CreateConnection(t.Context(), &directconnectsdk.CreateConnectionInput{
		Bandwidth:      aws.String("1Gbps"),
		ConnectionName: aws.String("conn-for-vif"),
		Location:       aws.String("EqDC2"),
	})
	require.NoError(t, err)

	return out
}

// TestRoundTripPrivateVirtualInterface exercises the FLATTENED VIF output
// shape (PARITY.md wire-trap #1) end to end, including the dual Asn/AsnLong
// zero-out rule (wire-trap #5) for a 4-byte ASN, VLAN uniqueness
// enforcement, and CreateBGPPeer's NESTED VirtualInterface wrapper.
func TestRoundTripPrivateVirtualInterface(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	conn := createTestConnection(t, client)

	const fourByteAsn = int64(4200000001) // exceeds math.MaxInt32, the 4-byte case

	vif, err := client.CreatePrivateVirtualInterface(ctx, &directconnectsdk.CreatePrivateVirtualInterfaceInput{
		ConnectionId: conn.ConnectionId,
		NewPrivateVirtualInterface: &types.NewPrivateVirtualInterface{
			VirtualInterfaceName: aws.String("test-private-vif"),
			Vlan:                 100,
			AsnLong:              aws.Int64(fourByteAsn),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, vif.VirtualInterfaceId)
	require.Equal(t, types.VirtualInterfaceStatePending, vif.VirtualInterfaceState)
	require.Equal(t, "private", aws.ToString(vif.VirtualInterfaceType))
	require.Equal(t, int32(0), vif.Asn, "legacy Asn must zero out for a 4-byte AsnLong value")
	require.Equal(t, fourByteAsn, aws.ToInt64(vif.AsnLong))

	// A second VIF with the same VLAN on the same connection must be
	// rejected (VLAN uniqueness per connection).
	_, err = client.CreatePrivateVirtualInterface(ctx, &directconnectsdk.CreatePrivateVirtualInterfaceInput{
		ConnectionId: conn.ConnectionId,
		NewPrivateVirtualInterface: &types.NewPrivateVirtualInterface{
			VirtualInterfaceName: aws.String("dup-vlan-vif"),
			Vlan:                 100,
		},
	})
	require.Error(t, err)

	var clientErr *types.DirectConnectClientException
	require.ErrorAs(t, err, &clientErr)

	peerOut, err := client.CreateBGPPeer(ctx, &directconnectsdk.CreateBGPPeerInput{
		VirtualInterfaceId: vif.VirtualInterfaceId,
		NewBGPPeer: &types.NewBGPPeer{
			AddressFamily: types.AddressFamilyIPv4,
			Asn:           65000,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, peerOut.VirtualInterface)
	require.Len(t, peerOut.VirtualInterface.BgpPeers, 1)
	require.Equal(t, int32(65000), peerOut.VirtualInterface.BgpPeers[0].Asn)

	require.Eventually(t, func() bool {
		out, descErr := client.DescribeVirtualInterfaces(ctx, &directconnectsdk.DescribeVirtualInterfacesInput{
			VirtualInterfaceId: vif.VirtualInterfaceId,
		})

		return descErr == nil && len(out.VirtualInterfaces) == 1 &&
			out.VirtualInterfaces[0].VirtualInterfaceState == types.VirtualInterfaceStateAvailable
	}, defaultAsyncWait, defaultAsyncPoll)

	delOut, err := client.DeleteVirtualInterface(ctx, &directconnectsdk.DeleteVirtualInterfaceInput{
		VirtualInterfaceId: vif.VirtualInterfaceId,
	})
	require.NoError(t, err)
	require.Equal(t, types.VirtualInterfaceStateDeleting, delOut.VirtualInterfaceState)
}

// TestRoundTripTransitVirtualInterface exercises the NESTED VIF output
// shape (CreateTransitVirtualInterfaceOutput wraps a *VirtualInterface,
// unlike private/public's flattened shape -- PARITY.md wire-trap #1).
func TestRoundTripTransitVirtualInterface(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	conn := createTestConnection(t, client)

	gw, err := client.CreateDirectConnectGateway(ctx, &directconnectsdk.CreateDirectConnectGatewayInput{
		DirectConnectGatewayName: aws.String("test-gw"),
	})
	require.NoError(t, err)

	vif, err := client.CreateTransitVirtualInterface(ctx, &directconnectsdk.CreateTransitVirtualInterfaceInput{
		ConnectionId: conn.ConnectionId,
		NewTransitVirtualInterface: &types.NewTransitVirtualInterface{
			VirtualInterfaceName:   aws.String("test-transit-vif"),
			Vlan:                   200,
			DirectConnectGatewayId: gw.DirectConnectGateway.DirectConnectGatewayId,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, vif.VirtualInterface)
	require.Equal(t, "transit", aws.ToString(vif.VirtualInterface.VirtualInterfaceType))
	require.NotNil(t, vif.VirtualInterface.AmazonSideAsn, "transit VIF should inherit its gateway's AmazonSideAsn")

	attachments, err := client.DescribeDirectConnectGatewayAttachments(
		ctx, &directconnectsdk.DescribeDirectConnectGatewayAttachmentsInput{
			DirectConnectGatewayId: gw.DirectConnectGateway.DirectConnectGatewayId,
		},
	)
	require.NoError(t, err)
	require.Len(t, attachments.DirectConnectGatewayAttachments, 1)
	require.Equal(t, types.DirectConnectGatewayAttachmentTypeTransitVirtualInterface,
		attachments.DirectConnectGatewayAttachments[0].AttachmentType)
}

// TestRoundTripPublicVirtualInterfaceAndRouteFilterPrefixes exercises the
// public-VIF-only RouteFilterPrefixes field and its verifying->pending->
// available startup chain.
func TestRoundTripPublicVirtualInterfaceAndRouteFilterPrefixes(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	conn := createTestConnection(t, client)

	vif, err := client.CreatePublicVirtualInterface(ctx, &directconnectsdk.CreatePublicVirtualInterfaceInput{
		ConnectionId: conn.ConnectionId,
		NewPublicVirtualInterface: &types.NewPublicVirtualInterface{
			VirtualInterfaceName: aws.String("test-public-vif"),
			Vlan:                 300,
			RouteFilterPrefixes:  []types.RouteFilterPrefix{{Cidr: aws.String("203.0.113.0/24")}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, types.VirtualInterfaceStateVerifying, vif.VirtualInterfaceState)
	require.Len(t, vif.RouteFilterPrefixes, 1)
	require.Equal(t, "203.0.113.0/24", aws.ToString(vif.RouteFilterPrefixes[0].Cidr))

	require.Eventually(t, func() bool {
		out, descErr := client.DescribeVirtualInterfaces(ctx, &directconnectsdk.DescribeVirtualInterfacesInput{
			VirtualInterfaceId: vif.VirtualInterfaceId,
		})

		return descErr == nil && len(out.VirtualInterfaces) == 1 &&
			out.VirtualInterfaces[0].VirtualInterfaceState == types.VirtualInterfaceStateAvailable
	}, defaultAsyncWait, defaultAsyncPoll)
}

// TestRoundTripCrossAccountAllocateConfirm exercises the cross-account
// Allocate*/Confirm* flow: a VIF allocated with an OwnerAccount starts in
// "confirming" and does NOT auto-progress until Confirm* is called.
func TestRoundTripCrossAccountAllocateConfirm(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	conn := createTestConnection(t, client)

	vif, err := client.AllocatePrivateVirtualInterface(ctx, &directconnectsdk.AllocatePrivateVirtualInterfaceInput{
		ConnectionId: conn.ConnectionId,
		OwnerAccount: aws.String("999999999999"),
		NewPrivateVirtualInterfaceAllocation: &types.NewPrivateVirtualInterfaceAllocation{
			VirtualInterfaceName: aws.String("cross-account-vif"),
			Vlan:                 400,
		},
	})
	require.NoError(t, err)
	require.Equal(t, types.VirtualInterfaceStateConfirming, vif.VirtualInterfaceState)
	require.Equal(t, "999999999999", aws.ToString(vif.OwnerAccount))

	// Give the (nonexistent, since Confirm hasn't been called) auto-timer a
	// moment to prove it does NOT fire while still confirming.
	out, err := client.DescribeVirtualInterfaces(ctx, &directconnectsdk.DescribeVirtualInterfacesInput{
		VirtualInterfaceId: vif.VirtualInterfaceId,
	})
	require.NoError(t, err)
	require.Equal(t, types.VirtualInterfaceStateConfirming, out.VirtualInterfaces[0].VirtualInterfaceState)

	confirmOut, err := client.ConfirmPrivateVirtualInterface(
		ctx, &directconnectsdk.ConfirmPrivateVirtualInterfaceInput{VirtualInterfaceId: vif.VirtualInterfaceId},
	)
	require.NoError(t, err)
	require.Equal(t, types.VirtualInterfaceStatePending, confirmOut.VirtualInterfaceState)

	require.Eventually(t, func() bool {
		descOut, descErr := client.DescribeVirtualInterfaces(ctx, &directconnectsdk.DescribeVirtualInterfacesInput{
			VirtualInterfaceId: vif.VirtualInterfaceId,
		})

		return descErr == nil && len(descOut.VirtualInterfaces) == 1 &&
			descOut.VirtualInterfaces[0].VirtualInterfaceState == types.VirtualInterfaceStateAvailable
	}, defaultAsyncWait, defaultAsyncPoll)
}

// TestRoundTripGatewayAssociationAndProposal exercises
// CreateDirectConnectGatewayAssociation's addressing-mode check, the
// cross-account proposal/accept flow, and UpdateDirectConnectGatewayAssociation's
// allowed-prefix delta.
func TestRoundTripGatewayAssociationAndProposal(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	gw, err := client.CreateDirectConnectGateway(ctx, &directconnectsdk.CreateDirectConnectGatewayInput{
		DirectConnectGatewayName: aws.String("assoc-test-gw"),
	})
	require.NoError(t, err)

	assoc, err := client.CreateDirectConnectGatewayAssociation(
		ctx, &directconnectsdk.CreateDirectConnectGatewayAssociationInput{
			DirectConnectGatewayId: gw.DirectConnectGateway.DirectConnectGatewayId,
			GatewayId:              aws.String("vgw-abcd1234"),
			AddAllowedPrefixesToDirectConnectGateway: []types.RouteFilterPrefix{
				{Cidr: aws.String("10.0.0.0/16")},
			},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, assoc.DirectConnectGatewayAssociation.AssociatedGateway)
	require.Equal(
		t,
		types.GatewayTypeVirtualPrivateGateway,
		assoc.DirectConnectGatewayAssociation.AssociatedGateway.Type,
	)
	require.Equal(t, "vgw-abcd1234", aws.ToString(assoc.DirectConnectGatewayAssociation.VirtualGatewayId),
		"legacy VirtualGatewayId must stay in sync with AssociatedGateway for a VGW association")

	updated, err := client.UpdateDirectConnectGatewayAssociation(
		ctx, &directconnectsdk.UpdateDirectConnectGatewayAssociationInput{
			AssociationId: assoc.DirectConnectGatewayAssociation.AssociationId,
			AddAllowedPrefixesToDirectConnectGateway: []types.RouteFilterPrefix{
				{Cidr: aws.String("10.1.0.0/16")},
			},
		},
	)
	require.NoError(t, err)
	require.Len(t, updated.DirectConnectGatewayAssociation.AllowedPrefixesToDirectConnectGateway, 2)

	// Cross-account proposal flow against a second gateway.
	gw2, err := client.CreateDirectConnectGateway(ctx, &directconnectsdk.CreateDirectConnectGatewayInput{
		DirectConnectGatewayName: aws.String("proposal-test-gw"),
	})
	require.NoError(t, err)

	proposal, err := client.CreateDirectConnectGatewayAssociationProposal(
		ctx, &directconnectsdk.CreateDirectConnectGatewayAssociationProposalInput{
			DirectConnectGatewayId:           gw2.DirectConnectGateway.DirectConnectGatewayId,
			DirectConnectGatewayOwnerAccount: aws.String("111111111111"),
			GatewayId:                        aws.String("tgw-abcd1234"),
		},
	)
	require.NoError(t, err)
	require.Equal(
		t,
		types.DirectConnectGatewayAssociationProposalStateRequested,
		proposal.DirectConnectGatewayAssociationProposal.ProposalState,
	)

	accepted, err := client.AcceptDirectConnectGatewayAssociationProposal(
		ctx, &directconnectsdk.AcceptDirectConnectGatewayAssociationProposalInput{
			DirectConnectGatewayId:        gw2.DirectConnectGateway.DirectConnectGatewayId,
			ProposalId:                    proposal.DirectConnectGatewayAssociationProposal.ProposalId,
			AssociatedGatewayOwnerAccount: aws.String("222222222222"),
		},
	)
	require.NoError(t, err)
	require.NotNil(t, accepted.DirectConnectGatewayAssociation)
	require.Equal(t, types.GatewayTypeTransitGateway, accepted.DirectConnectGatewayAssociation.AssociatedGateway.Type)
}

// TestRoundTripTaggingAndDescribeTags exercises TagResource/UntagResource
// and DescribeTags' BATCH-shaped (multi-ARN) request, unlike the
// single-ARN ListTagsForResource pattern most other services use.
func TestRoundTripTaggingAndDescribeTags(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	conn := createTestConnection(t, client)
	connARN := "arn:aws:directconnect:us-east-1:000000000000:dxcon/" + aws.ToString(conn.ConnectionId)

	_, err := client.TagResource(ctx, &directconnectsdk.TagResourceInput{
		ResourceArn: aws.String(connARN),
		Tags:        []types.Tag{{Key: aws.String("k1"), Value: aws.String("v1")}},
	})
	require.NoError(t, err)

	tagsOut, err := client.DescribeTags(ctx, &directconnectsdk.DescribeTagsInput{
		ResourceArns: []string{connARN},
	})
	require.NoError(t, err)
	require.Len(t, tagsOut.ResourceTags, 1)
	require.Len(t, tagsOut.ResourceTags[0].Tags, 1)
	require.Equal(t, "k1", aws.ToString(tagsOut.ResourceTags[0].Tags[0].Key))

	_, err = client.UntagResource(ctx, &directconnectsdk.UntagResourceInput{
		ResourceArn: aws.String(connARN),
		TagKeys:     []string{"k1"},
	})
	require.NoError(t, err)

	tagsOut2, err := client.DescribeTags(ctx, &directconnectsdk.DescribeTagsInput{
		ResourceArns: []string{connARN},
	})
	require.NoError(t, err)
	require.Empty(t, tagsOut2.ResourceTags[0].Tags)
}

// TestRoundTripMacSecKeyAssociation exercises AssociateMacSecKey (raw
// Cak/Ckn, synthesizing a SecretARN) and DisassociateMacSecKey (which
// requires that synthesized SecretARN to identify the key -- PARITY.md).
func TestRoundTripMacSecKeyAssociation(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	conn := createTestConnection(t, client)

	assoc, err := client.AssociateMacSecKey(ctx, &directconnectsdk.AssociateMacSecKeyInput{
		ConnectionId: conn.ConnectionId,
		Cak:          aws.String("0123456789abcdef0123456789abcdef"),
		Ckn:          aws.String("0123456789abcdef0123456789abcdef"),
	})
	require.NoError(t, err)
	require.Len(t, assoc.MacSecKeys, 1)
	require.NotEmpty(t, aws.ToString(assoc.MacSecKeys[0].SecretARN))

	secretARN := aws.ToString(assoc.MacSecKeys[0].SecretARN)

	disassoc, err := client.DisassociateMacSecKey(ctx, &directconnectsdk.DisassociateMacSecKeyInput{
		ConnectionId: conn.ConnectionId,
		SecretARN:    aws.String(secretARN),
	})
	require.NoError(t, err)
	require.Len(t, disassoc.MacSecKeys, 1)
}

// TestRoundTripBgpFailoverTest exercises StartBgpFailoverTest/
// StopBgpFailoverTest/ListVirtualInterfaceTestHistory without waiting out a
// real multi-minute duration (see bgpfailover.go).
func TestRoundTripBgpFailoverTest(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	conn := createTestConnection(t, client)

	vif, err := client.CreatePrivateVirtualInterface(ctx, &directconnectsdk.CreatePrivateVirtualInterfaceInput{
		ConnectionId: conn.ConnectionId,
		NewPrivateVirtualInterface: &types.NewPrivateVirtualInterface{
			VirtualInterfaceName: aws.String("failover-vif"),
			Vlan:                 500,
		},
	})
	require.NoError(t, err)

	peer, err := client.CreateBGPPeer(ctx, &directconnectsdk.CreateBGPPeerInput{
		VirtualInterfaceId: vif.VirtualInterfaceId,
		NewBGPPeer:         &types.NewBGPPeer{Asn: 65001},
	})
	require.NoError(t, err)

	testOut, err := client.StartBgpFailoverTest(ctx, &directconnectsdk.StartBgpFailoverTestInput{
		VirtualInterfaceId:    vif.VirtualInterfaceId,
		TestDurationInMinutes: aws.Int32(60),
	})
	require.NoError(t, err)
	require.NotNil(t, testOut.VirtualInterfaceTest)
	require.Nil(t, testOut.VirtualInterfaceTest.EndTime)

	confirmDown, err := client.DescribeVirtualInterfaces(ctx, &directconnectsdk.DescribeVirtualInterfacesInput{
		VirtualInterfaceId: vif.VirtualInterfaceId,
	})
	require.NoError(t, err)
	require.Equal(t, types.VirtualInterfaceStateTesting, confirmDown.VirtualInterfaces[0].VirtualInterfaceState)
	require.Equal(t, types.BGPStatusDown, confirmDown.VirtualInterfaces[0].BgpPeers[0].BgpStatus)
	require.Equal(
		t,
		peer.VirtualInterface.BgpPeers[0].BgpPeerId,
		confirmDown.VirtualInterfaces[0].BgpPeers[0].BgpPeerId,
	)

	stopOut, err := client.StopBgpFailoverTest(
		ctx, &directconnectsdk.StopBgpFailoverTestInput{VirtualInterfaceId: vif.VirtualInterfaceId},
	)
	require.NoError(t, err)
	require.NotNil(t, stopOut.VirtualInterfaceTest.EndTime)

	history, err := client.ListVirtualInterfaceTestHistory(
		ctx, &directconnectsdk.ListVirtualInterfaceTestHistoryInput{VirtualInterfaceId: vif.VirtualInterfaceId},
	)
	require.NoError(t, err)
	require.Len(t, history.VirtualInterfaceTestHistory, 1)
}

// TestRoundTripStaticReferenceData exercises the three static/derived-data
// ops (DescribeLocations, DescribeCustomerMetadata,
// DescribeRouterConfiguration).
func TestRoundTripStaticReferenceData(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	locs, err := client.DescribeLocations(ctx, &directconnectsdk.DescribeLocationsInput{})
	require.NoError(t, err)
	require.NotEmpty(t, locs.Locations)

	meta, err := client.DescribeCustomerMetadata(ctx, &directconnectsdk.DescribeCustomerMetadataInput{})
	require.NoError(t, err)
	require.Equal(t, types.NniPartnerTypeNonPartner, meta.NniPartnerType)
	require.Empty(t, meta.Agreements)

	conn := createTestConnection(t, client)
	vif, err := client.CreatePrivateVirtualInterface(ctx, &directconnectsdk.CreatePrivateVirtualInterfaceInput{
		ConnectionId: conn.ConnectionId,
		NewPrivateVirtualInterface: &types.NewPrivateVirtualInterface{
			VirtualInterfaceName: aws.String("router-config-vif"),
			Vlan:                 600,
		},
	})
	require.NoError(t, err)

	routerConfig, err := client.DescribeRouterConfiguration(
		ctx, &directconnectsdk.DescribeRouterConfigurationInput{VirtualInterfaceId: vif.VirtualInterfaceId},
	)
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(routerConfig.CustomerRouterConfig))
	require.NotNil(t, routerConfig.Router)
}

// TestRoundTripLoa exercises DescribeLoa (flattened, deprecated shape) and
// DescribeConnectionLoa (nested shape) both returning a well-formed
// placeholder PDF (PARITY.md wire-trap #7).
func TestRoundTripLoa(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	conn := createTestConnection(t, client)

	flat, err := client.DescribeLoa(ctx, &directconnectsdk.DescribeLoaInput{ConnectionId: conn.ConnectionId})
	require.NoError(t, err)
	require.NotEmpty(t, flat.LoaContent)
	require.Contains(t, string(flat.LoaContent), "%PDF-1.4")

	// DescribeConnectionLoa itself is deprecated in the real SDK (in favor of
	// DescribeLoa, the OPPOSITE direction PARITY.md's wire-trap #7 first
	// assumed -- corrected there), but it is still a real, listed operation
	// this handler must serve.
	nested, err := client.DescribeConnectionLoa( //nolint:staticcheck // deliberately exercising a deprecated-but-real op
		ctx,
		&directconnectsdk.DescribeConnectionLoaInput{ConnectionId: conn.ConnectionId},
	)
	require.NoError(t, err)
	require.NotNil(t, nested.Loa)
	require.NotEmpty(t, nested.Loa.LoaContent)
}

// TestRoundTripErrorMapping exercises this service's thin, 5-shape error
// model: a not-found connection surfaces as the one generic
// DirectConnectClientException, not a typed ResourceNotFoundException
// (which does not exist in this SDK -- PARITY.md).
func TestRoundTripErrorMapping(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	_, err := client.DeleteConnection(ctx, &directconnectsdk.DeleteConnectionInput{
		ConnectionId: aws.String("dxcon-doesnotexist"),
	})
	require.Error(t, err)

	var clientErr *types.DirectConnectClientException
	require.ErrorAs(t, err, &clientErr)
}
