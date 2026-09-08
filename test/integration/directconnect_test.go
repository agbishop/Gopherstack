package integration_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	dxsdk "github.com/aws/aws-sdk-go-v2/service/directconnect"
	dxtypes "github.com/aws/aws-sdk-go-v2/service/directconnect/types"
	ec2sdk "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	smithy "github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createDirectConnectClient returns a Direct Connect client pointed at the shared test container.
func createDirectConnectClient(t *testing.T) *dxsdk.Client {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err, "unable to load SDK config")

	return dxsdk.NewFromConfig(cfg, func(o *dxsdk.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}

// dxCleanupCtx returns a context for use inside t.Cleanup callbacks.
// t.Context() must not be used there: Go 1.24+ cancels it before cleanups run.
func dxCleanupCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// dxErrorCode extracts the smithy error code from err, or "" if err isn't one.
func dxErrorCode(err error) string {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode()
	}

	return ""
}

// createDxConnection is a small helper that creates a standard connection
// via the real SDK client, for tests whose focus is a different op.
func createDxConnection(ctx context.Context, t *testing.T, client *dxsdk.Client) *dxsdk.CreateConnectionOutput {
	t.Helper()

	out, err := client.CreateConnection(ctx, &dxsdk.CreateConnectionInput{
		Bandwidth:      aws.String("1Gbps"),
		ConnectionName: aws.String("integ-conn-" + uuid.NewString()[:8]),
		Location:       aws.String("EqDC2"),
	})
	require.NoError(t, err, "CreateConnection should succeed")

	t.Cleanup(func() {
		cctx, cancel := dxCleanupCtx()
		defer cancel()
		_, _ = client.DeleteConnection(cctx, &dxsdk.DeleteConnectionInput{ConnectionId: out.ConnectionId})
	})

	return out
}

// TestIntegration_DirectConnect_ConnectionAndLagLifecycle drives
// CreateConnection/CreateLag through a real aws-sdk-go-v2 client: the
// requested->pending->available async transition, LAG membership and its
// bandwidth-derived capacity limit (LimitExceededException), and
// disassociation/deletion.
func TestIntegration_DirectConnect_ConnectionAndLagLifecycle(t *testing.T) { //nolint:tparallel // sequential subtests
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createDirectConnectClient(t)

	conn := createDxConnection(ctx, t, client)
	require.Equal(t, dxtypes.ConnectionStateRequested, conn.ConnectionState)

	t.Run("AsyncTransitionAndUpdate", func(t *testing.T) { //nolint:paralleltest // sequential by design
		require.Eventually(t, func() bool {
			out, err := client.DescribeConnections(ctx, &dxsdk.DescribeConnectionsInput{
				ConnectionId: conn.ConnectionId,
			})

			return err == nil && len(out.Connections) == 1 &&
				out.Connections[0].ConnectionState == dxtypes.ConnectionStateAvailable
		}, 5*time.Second, 50*time.Millisecond, "connection should transition requested -> pending -> available")

		updated, err := client.UpdateConnection(ctx, &dxsdk.UpdateConnectionInput{
			ConnectionId:   conn.ConnectionId,
			ConnectionName: aws.String("integ-conn-renamed"),
			EncryptionMode: aws.String("should_encrypt"),
		})
		require.NoError(t, err, "UpdateConnection should succeed")
		assert.Equal(t, "integ-conn-renamed", aws.ToString(updated.ConnectionName))
		assert.Equal(t, "should_encrypt", aws.ToString(updated.EncryptionMode))
	})

	t.Run("LagMembershipAndCapacity", func(t *testing.T) { //nolint:paralleltest // sequential by design
		lag, err := client.CreateLag(ctx, &dxsdk.CreateLagInput{
			ConnectionsBandwidth: aws.String("1Gbps"),
			LagName:              aws.String("integ-lag-" + uuid.NewString()[:8]),
			Location:             aws.String("EqDC2"),
			NumberOfConnections:  4, // 1Gbps caps at 4 -- lagMaxConnsSmallPort
		})
		require.NoError(t, err, "CreateLag should succeed")
		require.Len(t, lag.Connections, 4)
		assert.Equal(t, dxtypes.LagStateRequested, lag.LagState)

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteLag(cctx, &dxsdk.DeleteLagInput{LagId: lag.LagId})
		})

		updatedLag, err := client.UpdateLag(ctx, &dxsdk.UpdateLagInput{
			LagId:        lag.LagId,
			MinimumLinks: 2,
		})
		require.NoError(t, err, "UpdateLag should succeed")
		assert.Equal(t, int32(2), updatedLag.MinimumLinks)

		// The LAG is already at its 4-connection cap for a 1Gbps port --
		// associating a 5th connection must be rejected with
		// LimitExceededException (PARITY.md wire-trap #8: reachable here with
		// no Tags input at all).
		_, err = client.AssociateConnectionWithLag(ctx, &dxsdk.AssociateConnectionWithLagInput{
			ConnectionId: conn.ConnectionId,
			LagId:        lag.LagId,
		})
		require.Error(t, err, "associating a 5th connection onto a full 1Gbps LAG should fail")
		assert.Equal(t, "LimitExceededException", dxErrorCode(err))

		describeOut, err := client.DescribeLags(ctx, &dxsdk.DescribeLagsInput{LagId: lag.LagId})
		require.NoError(t, err)
		require.Len(t, describeOut.Lags, 1)
		assert.Len(t, describeOut.Lags[0].Connections, 4)
	})

	t.Run("NotFoundErrors", func(t *testing.T) {
		tests := []struct {
			call func() error
			name string
		}{
			{
				name: "unknown connection",
				call: func() error {
					_, err := client.DeleteConnection(ctx, &dxsdk.DeleteConnectionInput{
						ConnectionId: aws.String("dxcon-doesnotexist"),
					})

					return err
				},
			},
			{
				name: "unknown lag",
				call: func() error {
					_, err := client.UpdateLag(ctx, &dxsdk.UpdateLagInput{
						LagId:        aws.String("dxlag-doesnotexist"),
						MinimumLinks: 1,
					})

					return err
				},
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				err := tc.call()
				require.Error(t, err, "operating on an unknown resource should fail")
				assert.Equal(t, "DirectConnectClientException", dxErrorCode(err))

				var clientErr *dxtypes.DirectConnectClientException
				require.ErrorAs(t, err, &clientErr, "error should deserialize as the real SDK exception type")
			})
		}
	})
}

// TestIntegration_DirectConnect_VirtualInterfaceLifecycle drives
// private/public/transit virtual interfaces and BGP peers through a real
// SDK client: the flattened-vs-nested output shapes (PARITY.md wire-trap
// #1), VLAN-uniqueness enforcement, and the cross-account Allocate*/
// Confirm* flow.
func TestIntegration_DirectConnect_VirtualInterfaceLifecycle(t *testing.T) { //nolint:tparallel // sequential subtests
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createDirectConnectClient(t)
	conn := createDxConnection(ctx, t, client)

	t.Run("PrivateVifAndBgpPeer", func(t *testing.T) { //nolint:paralleltest // sequential by design
		vif, err := client.CreatePrivateVirtualInterface(ctx, &dxsdk.CreatePrivateVirtualInterfaceInput{
			ConnectionId: conn.ConnectionId,
			NewPrivateVirtualInterface: &dxtypes.NewPrivateVirtualInterface{
				VirtualInterfaceName: aws.String("integ-private-vif"),
				Vlan:                 101,
				Asn:                  65010,
			},
		})
		require.NoError(t, err, "CreatePrivateVirtualInterface should succeed")
		require.NotNil(t, vif.VirtualInterfaceId)
		assert.Equal(t, dxtypes.VirtualInterfaceStatePending, vif.VirtualInterfaceState)
		assert.Equal(t, "private", aws.ToString(vif.VirtualInterfaceType))

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteVirtualInterface(
				cctx, &dxsdk.DeleteVirtualInterfaceInput{VirtualInterfaceId: vif.VirtualInterfaceId},
			)
		})

		// A second VIF reusing VLAN 101 on the same connection is rejected --
		// real AWS enforces VLAN uniqueness per physical connection.
		_, err = client.CreatePrivateVirtualInterface(ctx, &dxsdk.CreatePrivateVirtualInterfaceInput{
			ConnectionId: conn.ConnectionId,
			NewPrivateVirtualInterface: &dxtypes.NewPrivateVirtualInterface{
				VirtualInterfaceName: aws.String("integ-dup-vlan-vif"),
				Vlan:                 101,
			},
		})
		require.Error(t, err, "duplicate VLAN on the same connection should be rejected")
		assert.Equal(t, "DirectConnectClientException", dxErrorCode(err))

		peerOut, err := client.CreateBGPPeer(ctx, &dxsdk.CreateBGPPeerInput{
			VirtualInterfaceId: vif.VirtualInterfaceId,
			NewBGPPeer: &dxtypes.NewBGPPeer{
				AddressFamily: dxtypes.AddressFamilyIPv4,
				Asn:           65011,
			},
		})
		require.NoError(t, err, "CreateBGPPeer should succeed")
		require.NotNil(t, peerOut.VirtualInterface, "CreateBGPPeer nests its output, unlike Create*VirtualInterface")
		require.Len(t, peerOut.VirtualInterface.BgpPeers, 1)
		peerID := peerOut.VirtualInterface.BgpPeers[0].BgpPeerId

		require.Eventually(t, func() bool {
			out, descErr := client.DescribeVirtualInterfaces(ctx, &dxsdk.DescribeVirtualInterfacesInput{
				VirtualInterfaceId: vif.VirtualInterfaceId,
			})

			return descErr == nil && len(out.VirtualInterfaces) == 1 &&
				out.VirtualInterfaces[0].VirtualInterfaceState == dxtypes.VirtualInterfaceStateAvailable &&
				len(out.VirtualInterfaces[0].BgpPeers) == 1 &&
				out.VirtualInterfaces[0].BgpPeers[0].BgpPeerState == dxtypes.BGPPeerStateAvailable
		}, 5*time.Second, 50*time.Millisecond, "VIF and BGP peer should reach their available states")

		delPeerOut, err := client.DeleteBGPPeer(ctx, &dxsdk.DeleteBGPPeerInput{
			VirtualInterfaceId: vif.VirtualInterfaceId,
			BgpPeerId:          peerID,
		})
		require.NoError(t, err, "DeleteBGPPeer should succeed")
		require.Len(t, delPeerOut.VirtualInterface.BgpPeers, 1)
		assert.Equal(t, dxtypes.BGPPeerStateDeleting, delPeerOut.VirtualInterface.BgpPeers[0].BgpPeerState)
	})

	t.Run("PublicVifRouteFilterPrefixes", func(t *testing.T) { //nolint:paralleltest // sequential by design
		vif, err := client.CreatePublicVirtualInterface(ctx, &dxsdk.CreatePublicVirtualInterfaceInput{
			ConnectionId: conn.ConnectionId,
			NewPublicVirtualInterface: &dxtypes.NewPublicVirtualInterface{
				VirtualInterfaceName: aws.String("integ-public-vif"),
				Vlan:                 201,
				RouteFilterPrefixes:  []dxtypes.RouteFilterPrefix{{Cidr: aws.String("203.0.113.0/24")}},
			},
		})
		require.NoError(t, err, "CreatePublicVirtualInterface should succeed")
		assert.Equal(t, dxtypes.VirtualInterfaceStateVerifying, vif.VirtualInterfaceState,
			"public VIFs start in verifying, not pending")
		require.Len(t, vif.RouteFilterPrefixes, 1)
		assert.Equal(t, "203.0.113.0/24", aws.ToString(vif.RouteFilterPrefixes[0].Cidr))

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteVirtualInterface(
				cctx, &dxsdk.DeleteVirtualInterfaceInput{VirtualInterfaceId: vif.VirtualInterfaceId},
			)
		})

		require.Eventually(t, func() bool {
			out, descErr := client.DescribeVirtualInterfaces(ctx, &dxsdk.DescribeVirtualInterfacesInput{
				VirtualInterfaceId: vif.VirtualInterfaceId,
			})

			return descErr == nil && len(out.VirtualInterfaces) == 1 &&
				out.VirtualInterfaces[0].VirtualInterfaceState == dxtypes.VirtualInterfaceStateAvailable
		}, 5*time.Second, 50*time.Millisecond, "public VIF should transition verifying -> pending -> available")

		updated, err := client.UpdateVirtualInterfaceAttributes(ctx, &dxsdk.UpdateVirtualInterfaceAttributesInput{
			VirtualInterfaceId:   vif.VirtualInterfaceId,
			VirtualInterfaceName: aws.String("integ-public-vif-renamed"),
		})
		require.NoError(t, err, "UpdateVirtualInterfaceAttributes should succeed")
		assert.Equal(t, "integ-public-vif-renamed", aws.ToString(updated.VirtualInterfaceName))
	})

	t.Run("TransitVifRequiresGateway", func(t *testing.T) { //nolint:paralleltest // sequential by design
		gw, err := client.CreateDirectConnectGateway(ctx, &dxsdk.CreateDirectConnectGatewayInput{
			DirectConnectGatewayName: aws.String("integ-transit-vif-gw"),
		})
		require.NoError(t, err)

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteDirectConnectGateway(
				cctx, &dxsdk.DeleteDirectConnectGatewayInput{
					DirectConnectGatewayId: gw.DirectConnectGateway.DirectConnectGatewayId,
				},
			)
		})

		vif, err := client.CreateTransitVirtualInterface(ctx, &dxsdk.CreateTransitVirtualInterfaceInput{
			ConnectionId: conn.ConnectionId,
			NewTransitVirtualInterface: &dxtypes.NewTransitVirtualInterface{
				VirtualInterfaceName:   aws.String("integ-transit-vif"),
				Vlan:                   301,
				DirectConnectGatewayId: gw.DirectConnectGateway.DirectConnectGatewayId,
			},
		})
		require.NoError(t, err, "CreateTransitVirtualInterface should succeed")
		require.NotNil(t, vif.VirtualInterface, "CreateTransitVirtualInterfaceOutput nests VirtualInterface")
		assert.Equal(t, "transit", aws.ToString(vif.VirtualInterface.VirtualInterfaceType))

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteVirtualInterface(cctx, &dxsdk.DeleteVirtualInterfaceInput{
				VirtualInterfaceId: vif.VirtualInterface.VirtualInterfaceId,
			})
		})

		attachments, err := client.DescribeDirectConnectGatewayAttachments(
			ctx, &dxsdk.DescribeDirectConnectGatewayAttachmentsInput{
				DirectConnectGatewayId: gw.DirectConnectGateway.DirectConnectGatewayId,
			},
		)
		require.NoError(t, err)
		require.Len(t, attachments.DirectConnectGatewayAttachments, 1)
		assert.Equal(t, dxtypes.DirectConnectGatewayAttachmentTypeTransitVirtualInterface,
			attachments.DirectConnectGatewayAttachments[0].AttachmentType)
	})

	t.Run("CrossAccountAllocateConfirm", func(t *testing.T) { //nolint:paralleltest // sequential by design
		vif, err := client.AllocatePrivateVirtualInterface(ctx, &dxsdk.AllocatePrivateVirtualInterfaceInput{
			ConnectionId: conn.ConnectionId,
			OwnerAccount: aws.String("999999999999"),
			NewPrivateVirtualInterfaceAllocation: &dxtypes.NewPrivateVirtualInterfaceAllocation{
				VirtualInterfaceName: aws.String("integ-cross-account-vif"),
				Vlan:                 401,
			},
		})
		require.NoError(t, err, "AllocatePrivateVirtualInterface should succeed")
		assert.Equal(t, dxtypes.VirtualInterfaceStateConfirming, vif.VirtualInterfaceState,
			"an allocated cross-account VIF starts confirming, not pending")
		assert.Equal(t, "999999999999", aws.ToString(vif.OwnerAccount))

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteVirtualInterface(
				cctx, &dxsdk.DeleteVirtualInterfaceInput{VirtualInterfaceId: vif.VirtualInterfaceId},
			)
		})

		confirmOut, err := client.ConfirmPrivateVirtualInterface(
			ctx, &dxsdk.ConfirmPrivateVirtualInterfaceInput{VirtualInterfaceId: vif.VirtualInterfaceId},
		)
		require.NoError(t, err, "ConfirmPrivateVirtualInterface should succeed")
		assert.Equal(t, dxtypes.VirtualInterfaceStatePending, confirmOut.VirtualInterfaceState)

		require.Eventually(t, func() bool {
			out, descErr := client.DescribeVirtualInterfaces(ctx, &dxsdk.DescribeVirtualInterfacesInput{
				VirtualInterfaceId: vif.VirtualInterfaceId,
			})

			return descErr == nil && len(out.VirtualInterfaces) == 1 &&
				out.VirtualInterfaces[0].VirtualInterfaceState == dxtypes.VirtualInterfaceStateAvailable
		}, 5*time.Second, 50*time.Millisecond, "confirmed VIF should transition pending -> available")
	})
}

// TestIntegration_DirectConnect_GatewayAssociationsCrossService drives
// DirectConnectGateway creation, the same-account association and
// cross-account proposal/accept flows, and -- the highest-value assertion
// in this file -- proves CreateDirectConnectGatewayAssociation validates
// GatewayId against REAL services/ec2 state: a genuine EC2 VpnGateway/
// TransitGateway (created via the real EC2 SDK client against this same
// running server) is accepted, and a fabricated id that was never created
// in EC2 is rejected, exactly like real AWS would reject a reference to a
// gateway that doesn't exist.
//
//nolint:tparallel // sequential subtests
func TestIntegration_DirectConnect_GatewayAssociationsCrossService(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createDirectConnectClient(t)
	ec2Client := createEC2Client(t)

	t.Run("RealVpnGatewayAccepted_FakeOneRejected", func(t *testing.T) { //nolint:paralleltest // sequential by design
		vgwOut, err := ec2Client.CreateVpnGateway(ctx, &ec2sdk.CreateVpnGatewayInput{
			Type: ec2types.GatewayTypeIpsec1,
		})
		require.NoError(t, err, "EC2 CreateVpnGateway should succeed")
		require.NotNil(t, vgwOut.VpnGateway)
		vgwID := aws.ToString(vgwOut.VpnGateway.VpnGatewayId)
		require.NotEmpty(t, vgwID)

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = ec2Client.DeleteVpnGateway(cctx, &ec2sdk.DeleteVpnGatewayInput{VpnGatewayId: aws.String(vgwID)})
		})

		gw, err := client.CreateDirectConnectGateway(ctx, &dxsdk.CreateDirectConnectGatewayInput{
			DirectConnectGatewayName: aws.String("integ-vgw-assoc-gw"),
		})
		require.NoError(t, err)

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteDirectConnectGateway(cctx, &dxsdk.DeleteDirectConnectGatewayInput{
				DirectConnectGatewayId: gw.DirectConnectGateway.DirectConnectGatewayId,
			})
		})

		assoc, err := client.CreateDirectConnectGatewayAssociation(
			ctx, &dxsdk.CreateDirectConnectGatewayAssociationInput{
				DirectConnectGatewayId: gw.DirectConnectGateway.DirectConnectGatewayId,
				GatewayId:              aws.String(vgwID),
			},
		)
		require.NoError(t, err, "associating a real EC2 VpnGateway must succeed")
		require.NotNil(t, assoc.DirectConnectGatewayAssociation.AssociatedGateway)
		assert.Equal(t, dxtypes.GatewayTypeVirtualPrivateGateway,
			assoc.DirectConnectGatewayAssociation.AssociatedGateway.Type)
		assert.Equal(t, vgwID, aws.ToString(assoc.DirectConnectGatewayAssociation.VirtualGatewayId),
			"legacy VirtualGatewayId must stay in sync with AssociatedGateway for a VGW association")

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteDirectConnectGatewayAssociation(cctx, &dxsdk.DeleteDirectConnectGatewayAssociationInput{
				AssociationId: assoc.DirectConnectGatewayAssociation.AssociationId,
			})
		})

		// DescribeVirtualGateways proxies EC2's own VpnGateway list rather
		// than a duplicate store -- the real VGW just created must appear.
		vgws, err := client.DescribeVirtualGateways(ctx, &dxsdk.DescribeVirtualGatewaysInput{})
		require.NoError(t, err)

		found := false
		for _, v := range vgws.VirtualGateways {
			if aws.ToString(v.VirtualGatewayId) == vgwID {
				found = true

				break
			}
		}
		assert.True(t, found, "DescribeVirtualGateways should proxy the real EC2 VpnGateway")

		gw2, err := client.CreateDirectConnectGateway(ctx, &dxsdk.CreateDirectConnectGatewayInput{
			DirectConnectGatewayName: aws.String("integ-fake-vgw-gw"),
		})
		require.NoError(t, err)

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteDirectConnectGateway(cctx, &dxsdk.DeleteDirectConnectGatewayInput{
				DirectConnectGatewayId: gw2.DirectConnectGateway.DirectConnectGatewayId,
			})
		})

		// A VGW id that was never created in EC2 must be rejected -- this is
		// the cross-service validation itself, not the string-prefix
		// fallback: it proves the real EC2 backend was consulted.
		_, err = client.CreateDirectConnectGatewayAssociation(
			ctx, &dxsdk.CreateDirectConnectGatewayAssociationInput{
				DirectConnectGatewayId: gw2.DirectConnectGateway.DirectConnectGatewayId,
				GatewayId:              aws.String("vgw-" + uuid.NewString()[:8]),
			},
		)
		require.Error(t, err, "associating a VGW id that doesn't exist in EC2 must be rejected")
		assert.Equal(t, "DirectConnectClientException", dxErrorCode(err))
	})

	t.Run("RealTransitGatewayProposalAndAccept", func(t *testing.T) { //nolint:paralleltest // sequential by design
		tgwOut, err := ec2Client.CreateTransitGateway(ctx, &ec2sdk.CreateTransitGatewayInput{
			Description: aws.String("integ-dx-cross-service-tgw"),
		})
		require.NoError(t, err, "EC2 CreateTransitGateway should succeed")
		require.NotNil(t, tgwOut.TransitGateway)
		tgwID := aws.ToString(tgwOut.TransitGateway.TransitGatewayId)
		require.NotEmpty(t, tgwID)

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = ec2Client.DeleteTransitGateway(
				cctx, &ec2sdk.DeleteTransitGatewayInput{TransitGatewayId: aws.String(tgwID)},
			)
		})

		gw, err := client.CreateDirectConnectGateway(ctx, &dxsdk.CreateDirectConnectGatewayInput{
			DirectConnectGatewayName: aws.String("integ-tgw-proposal-gw"),
		})
		require.NoError(t, err)

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteDirectConnectGateway(cctx, &dxsdk.DeleteDirectConnectGatewayInput{
				DirectConnectGatewayId: gw.DirectConnectGateway.DirectConnectGatewayId,
			})
		})

		proposal, err := client.CreateDirectConnectGatewayAssociationProposal(
			ctx, &dxsdk.CreateDirectConnectGatewayAssociationProposalInput{
				DirectConnectGatewayId:           gw.DirectConnectGateway.DirectConnectGatewayId,
				DirectConnectGatewayOwnerAccount: aws.String("111111111111"),
				GatewayId:                        aws.String(tgwID),
			},
		)
		require.NoError(t, err, "proposing a real EC2 TransitGateway must succeed")
		assert.Equal(t, dxtypes.DirectConnectGatewayAssociationProposalStateRequested,
			proposal.DirectConnectGatewayAssociationProposal.ProposalState)

		accepted, err := client.AcceptDirectConnectGatewayAssociationProposal(
			ctx, &dxsdk.AcceptDirectConnectGatewayAssociationProposalInput{
				DirectConnectGatewayId:        gw.DirectConnectGateway.DirectConnectGatewayId,
				ProposalId:                    proposal.DirectConnectGatewayAssociationProposal.ProposalId,
				AssociatedGatewayOwnerAccount: aws.String("222222222222"),
			},
		)
		require.NoError(t, err, "AcceptDirectConnectGatewayAssociationProposal should succeed")
		require.NotNil(t, accepted.DirectConnectGatewayAssociation)
		assert.Equal(t, dxtypes.GatewayTypeTransitGateway,
			accepted.DirectConnectGatewayAssociation.AssociatedGateway.Type)

		t.Cleanup(func() {
			cctx, cancel := dxCleanupCtx()
			defer cancel()
			_, _ = client.DeleteDirectConnectGatewayAssociation(cctx, &dxsdk.DeleteDirectConnectGatewayAssociationInput{
				AssociationId: accepted.DirectConnectGatewayAssociation.AssociationId,
			})
		})

		updated, err := client.UpdateDirectConnectGatewayAssociation(
			ctx, &dxsdk.UpdateDirectConnectGatewayAssociationInput{
				AssociationId: accepted.DirectConnectGatewayAssociation.AssociationId,
				AddAllowedPrefixesToDirectConnectGateway: []dxtypes.RouteFilterPrefix{
					{Cidr: aws.String("10.2.0.0/16")},
				},
			},
		)
		require.NoError(t, err, "UpdateDirectConnectGatewayAssociation should succeed")
		require.Len(t, updated.DirectConnectGatewayAssociation.AllowedPrefixesToDirectConnectGateway, 1)

		// A fabricated transit gateway id, never created in EC2, must also be
		// rejected -- the proposal side of the same cross-service check.
		_, err = client.CreateDirectConnectGatewayAssociationProposal(
			ctx, &dxsdk.CreateDirectConnectGatewayAssociationProposalInput{
				DirectConnectGatewayId:           gw.DirectConnectGateway.DirectConnectGatewayId,
				DirectConnectGatewayOwnerAccount: aws.String("111111111111"),
				GatewayId:                        aws.String("tgw-" + uuid.NewString()[:8]),
			},
		)
		require.Error(t, err, "proposing a TGW id that doesn't exist in EC2 must be rejected")
		assert.Equal(t, "DirectConnectClientException", dxErrorCode(err))
	})
}

// TestIntegration_DirectConnect_Tagging drives TagResource/UntagResource/
// DescribeTags through a real SDK client across two different taggable
// resource kinds -- a regional Connection ARN (dxcon/) and the GLOBAL,
// no-region DirectConnectGateway ARN (dx-gateway/, PARITY.md's ARN
// section) -- plus the tag-count and duplicate-key wire validations.
func TestIntegration_DirectConnect_Tagging(t *testing.T) { //nolint:tparallel // sequential subtests
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	ctx := t.Context()
	client := createDirectConnectClient(t)
	conn := createDxConnection(ctx, t, client)
	connARN := "arn:aws:directconnect:us-east-1:000000000000:dxcon/" + aws.ToString(conn.ConnectionId)

	gw, err := client.CreateDirectConnectGateway(ctx, &dxsdk.CreateDirectConnectGatewayInput{
		DirectConnectGatewayName: aws.String("integ-tag-gw"),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cctx, cancel := dxCleanupCtx()
		defer cancel()
		_, _ = client.DeleteDirectConnectGateway(cctx, &dxsdk.DeleteDirectConnectGatewayInput{
			DirectConnectGatewayId: gw.DirectConnectGateway.DirectConnectGatewayId,
		})
	})

	// dx-gateway ARNs carry no region segment (a GLOBAL ARN, unlike every
	// other taggable resource kind in this service).
	gwID := aws.ToString(gw.DirectConnectGateway.DirectConnectGatewayId)
	gwARN := "arn:aws:directconnect::000000000000:dx-gateway/" + gwID

	t.Run("TagUntagAndBatchDescribe", func(t *testing.T) { //nolint:paralleltest // sequential by design
		_, tagErr := client.TagResource(ctx, &dxsdk.TagResourceInput{
			ResourceArn: aws.String(connARN),
			Tags:        []dxtypes.Tag{{Key: aws.String("env"), Value: aws.String("integ")}},
		})
		require.NoError(t, tagErr, "TagResource on a Connection should succeed")

		_, tagErr = client.TagResource(ctx, &dxsdk.TagResourceInput{
			ResourceArn: aws.String(gwARN),
			Tags:        []dxtypes.Tag{{Key: aws.String("owner"), Value: aws.String("integ")}},
		})
		require.NoError(t, tagErr, "TagResource on a global DirectConnectGateway ARN should succeed")

		// DescribeTags takes a BATCH of ARNs in one call (unlike the
		// single-ARN ListTagsForResource pattern most other services use).
		tagsOut, tagErr := client.DescribeTags(ctx, &dxsdk.DescribeTagsInput{
			ResourceArns: []string{connARN, gwARN},
		})
		require.NoError(t, tagErr)
		require.Len(t, tagsOut.ResourceTags, 2)

		byARN := make(map[string][]dxtypes.Tag, 2)
		for _, rt := range tagsOut.ResourceTags {
			byARN[aws.ToString(rt.ResourceArn)] = rt.Tags
		}
		require.Len(t, byARN[connARN], 1)
		assert.Equal(t, "env", aws.ToString(byARN[connARN][0].Key))
		require.Len(t, byARN[gwARN], 1)
		assert.Equal(t, "owner", aws.ToString(byARN[gwARN][0].Key))

		_, tagErr = client.UntagResource(ctx, &dxsdk.UntagResourceInput{
			ResourceArn: aws.String(connARN),
			TagKeys:     []string{"env"},
		})
		require.NoError(t, tagErr, "UntagResource should succeed")

		afterUntag, tagErr := client.DescribeTags(ctx, &dxsdk.DescribeTagsInput{ResourceArns: []string{connARN}})
		require.NoError(t, tagErr)
		require.Len(t, afterUntag.ResourceTags, 1)
		assert.Empty(t, afterUntag.ResourceTags[0].Tags)
	})

	t.Run("TagResourceValidationFailures", func(t *testing.T) {
		const overLimit = 51 // maxTagsPerResource (errors.go) is 50

		tooManyTags := make([]dxtypes.Tag, 0, overLimit)
		for i := range overLimit {
			tooManyTags = append(tooManyTags, dxtypes.Tag{
				Key:   aws.String("k" + strconv.Itoa(i)),
				Value: aws.String("v"),
			})
		}

		tests := []struct {
			resourceArn string
			wantCode    string
			name        string
			tags        []dxtypes.Tag
		}{
			{
				name:        "duplicate tag keys",
				resourceArn: connARN,
				tags: []dxtypes.Tag{
					{Key: aws.String("dup"), Value: aws.String("a")},
					{Key: aws.String("dup"), Value: aws.String("b")},
				},
				wantCode: "DuplicateTagKeysException",
			},
			{
				name:        "too many tags",
				resourceArn: connARN,
				tags:        tooManyTags,
				wantCode:    "TooManyTagsException",
			},
			{
				name:        "unknown resource arn",
				resourceArn: "arn:aws:directconnect:us-east-1:000000000000:dxcon/dxcon-doesnotexist",
				tags:        []dxtypes.Tag{{Key: aws.String("k"), Value: aws.String("v")}},
				wantCode:    "DirectConnectClientException",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				_, tagErr := client.TagResource(ctx, &dxsdk.TagResourceInput{
					ResourceArn: aws.String(tc.resourceArn),
					Tags:        tc.tags,
				})
				require.Error(t, tagErr, "invalid TagResource input should be rejected")
				assert.Equal(t, tc.wantCode, dxErrorCode(tagErr))
			})
		}
	})
}
