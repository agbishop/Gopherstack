package networkmanager_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	networkmanagersdk "github.com/aws/aws-sdk-go-v2/service/networkmanager"
	"github.com/aws/aws-sdk-go-v2/service/networkmanager/types"
	"github.com/stretchr/testify/require"
)

// TestPrecondition_DeleteAndAssociateGuards drives every referential-
// integrity precondition this pass added: DeleteSite/DeleteDevice/DeleteLink/
// DeleteGlobalNetwork/DisassociateLink refusing a delete while a dependent
// reference exists, and AssociateLink/AssociateCustomerGateway/
// AssociateTransitGatewayConnectPeer/AssociateConnectPeer refusing an
// association the SDK doc says is invalid. Each case is a real SDK
// ConflictException per api_op_<Op>.go's doc comment.
func TestPrecondition_DeleteAndAssociateGuards(t *testing.T) {
	t.Parallel()

	cases := []struct {
		run  func(t *testing.T, client *networkmanagersdk.Client)
		name string
	}{
		{
			name: "delete_site_with_device",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
				require.NoError(t, err)

				site, err := client.CreateSite(ctx, &networkmanagersdk.CreateSiteInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
				})
				require.NoError(t, err)

				_, err = client.CreateDevice(ctx, &networkmanagersdk.CreateDeviceInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, SiteId: site.Site.SiteId,
				})
				require.NoError(t, err)

				_, err = client.DeleteSite(ctx, &networkmanagersdk.DeleteSiteInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, SiteId: site.Site.SiteId,
				})
				requireConflict(t, err)
			},
		},
		{
			name: "delete_site_with_link",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
				require.NoError(t, err)

				site, err := client.CreateSite(ctx, &networkmanagersdk.CreateSiteInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
				})
				require.NoError(t, err)

				_, err = client.CreateLink(ctx, &networkmanagersdk.CreateLinkInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, SiteId: site.Site.SiteId,
					Bandwidth: &types.Bandwidth{DownloadSpeed: aws.Int32(1), UploadSpeed: aws.Int32(1)},
				})
				require.NoError(t, err)

				_, err = client.DeleteSite(ctx, &networkmanagersdk.DeleteSiteInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, SiteId: site.Site.SiteId,
				})
				requireConflict(t, err)
			},
		},
		{
			name: "delete_device_with_link_association",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, device, link := setupSiteDeviceLink(t, client)

				_, err := client.AssociateLink(ctx, &networkmanagersdk.AssociateLinkInput{
					GlobalNetworkId: gn, DeviceId: device, LinkId: link,
				})
				require.NoError(t, err)

				_, err = client.DeleteDevice(ctx, &networkmanagersdk.DeleteDeviceInput{
					GlobalNetworkId: gn, DeviceId: device,
				})
				requireConflict(t, err)
			},
		},
		{
			name: "delete_device_with_customer_gateway_association",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
				require.NoError(t, err)

				device, err := client.CreateDevice(ctx, &networkmanagersdk.CreateDeviceInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
				})
				require.NoError(t, err)

				cgwArn := "arn:aws:ec2:us-east-1:000000000000:customer-gateway/cgw-precondition"
				_, err = client.AssociateCustomerGateway(ctx, &networkmanagersdk.AssociateCustomerGatewayInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, CustomerGatewayArn: aws.String(cgwArn),
					DeviceId: device.Device.DeviceId,
				})
				require.NoError(t, err)

				_, err = client.DeleteDevice(ctx, &networkmanagersdk.DeleteDeviceInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, DeviceId: device.Device.DeviceId,
				})
				requireConflict(t, err)
			},
		},
		{
			name: "delete_link_with_device_association",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, device, link := setupSiteDeviceLink(t, client)

				_, err := client.AssociateLink(ctx, &networkmanagersdk.AssociateLinkInput{
					GlobalNetworkId: gn, DeviceId: device, LinkId: link,
				})
				require.NoError(t, err)

				_, err = client.DeleteLink(ctx, &networkmanagersdk.DeleteLinkInput{
					GlobalNetworkId: gn, LinkId: link,
				})
				requireConflict(t, err)
			},
		},
		{
			// This case's fixture necessarily also satisfies DeleteLink's
			// device-association guard (AssociateCustomerGateway's own
			// "link must be associated with device" precondition, added the
			// same pass, requires AssociateLink first) -- neutering proved
			// the CGW-only branch still fires live when the device-branch
			// is disabled, but with both guards enabled the device-branch
			// preempts it (case order). Kept as doc-correct defense in
			// depth; not independently observable via the public API given
			// this pass's other guards.
			name: "delete_link_with_customer_gateway_association",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, device, link := setupSiteDeviceLink(t, client)

				_, err := client.AssociateLink(ctx, &networkmanagersdk.AssociateLinkInput{
					GlobalNetworkId: gn, DeviceId: device, LinkId: link,
				})
				require.NoError(t, err)

				cgwArn := "arn:aws:ec2:us-east-1:000000000000:customer-gateway/cgw-precondition-2"
				_, err = client.AssociateCustomerGateway(ctx, &networkmanagersdk.AssociateCustomerGatewayInput{
					GlobalNetworkId: gn, CustomerGatewayArn: aws.String(cgwArn),
					DeviceId: device, LinkId: link,
				})
				require.NoError(t, err)

				_, err = client.DeleteLink(ctx, &networkmanagersdk.DeleteLinkInput{
					GlobalNetworkId: gn, LinkId: link,
				})
				requireConflict(t, err)
			},
		},
		{
			name: "delete_global_network_with_site",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
				require.NoError(t, err)

				_, err = client.CreateSite(ctx, &networkmanagersdk.CreateSiteInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
				})
				require.NoError(t, err)

				_, err = client.DeleteGlobalNetwork(ctx, &networkmanagersdk.DeleteGlobalNetworkInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
				})
				requireConflict(t, err)
			},
		},
		{
			name: "delete_global_network_with_transit_gateway_registration",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
				require.NoError(t, err)

				_, err = client.RegisterTransitGateway(ctx, &networkmanagersdk.RegisterTransitGatewayInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
					TransitGatewayArn: aws.String(
						"arn:aws:ec2:us-east-1:000000000000:transit-gateway/tgw-precondition",
					),
				})
				require.NoError(t, err)

				_, err = client.DeleteGlobalNetwork(ctx, &networkmanagersdk.DeleteGlobalNetworkInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
				})
				requireConflict(t, err)
			},
		},
		{
			name: "disassociate_link_with_customer_gateway_association",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, device, link := setupSiteDeviceLink(t, client)

				_, err := client.AssociateLink(ctx, &networkmanagersdk.AssociateLinkInput{
					GlobalNetworkId: gn, DeviceId: device, LinkId: link,
				})
				require.NoError(t, err)

				cgwArn := "arn:aws:ec2:us-east-1:000000000000:customer-gateway/cgw-precondition-3"
				_, err = client.AssociateCustomerGateway(ctx, &networkmanagersdk.AssociateCustomerGatewayInput{
					GlobalNetworkId: gn, CustomerGatewayArn: aws.String(cgwArn),
					DeviceId: device, LinkId: link,
				})
				require.NoError(t, err)

				_, err = client.DisassociateLink(ctx, &networkmanagersdk.DisassociateLinkInput{
					GlobalNetworkId: gn, DeviceId: device, LinkId: link,
				})
				requireConflict(t, err)
			},
		},
		{
			name: "associate_link_device_and_link_in_different_sites",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
				require.NoError(t, err)

				siteA, err := client.CreateSite(ctx, &networkmanagersdk.CreateSiteInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
				})
				require.NoError(t, err)

				siteB, err := client.CreateSite(ctx, &networkmanagersdk.CreateSiteInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
				})
				require.NoError(t, err)

				device, err := client.CreateDevice(ctx, &networkmanagersdk.CreateDeviceInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, SiteId: siteA.Site.SiteId,
				})
				require.NoError(t, err)

				link, err := client.CreateLink(ctx, &networkmanagersdk.CreateLinkInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, SiteId: siteB.Site.SiteId,
					Bandwidth: &types.Bandwidth{DownloadSpeed: aws.Int32(1), UploadSpeed: aws.Int32(1)},
				})
				require.NoError(t, err)

				_, err = client.AssociateLink(ctx, &networkmanagersdk.AssociateLinkInput{
					GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
					DeviceId:        device.Device.DeviceId, LinkId: link.Link.LinkId,
				})
				requireConflict(t, err)
			},
		},
		{
			name: "associate_customer_gateway_link_not_associated_with_device",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, device, link := setupSiteDeviceLink(t, client)

				cgwArn := "arn:aws:ec2:us-east-1:000000000000:customer-gateway/cgw-precondition-4"
				_, err := client.AssociateCustomerGateway(ctx, &networkmanagersdk.AssociateCustomerGatewayInput{
					GlobalNetworkId: gn, CustomerGatewayArn: aws.String(cgwArn),
					DeviceId: device, LinkId: link,
				})
				requireConflict(t, err)
			},
		},
		{
			name: "associate_transit_gateway_connect_peer_link_not_associated_with_device",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, device, link := setupSiteDeviceLink(t, client)

				tgwCpArn := "arn:aws:ec2:us-east-1:000000000000:transit-gateway-connect-peer/tgw-cp-precondition"
				_, err := client.AssociateTransitGatewayConnectPeer(
					ctx, &networkmanagersdk.AssociateTransitGatewayConnectPeerInput{
						GlobalNetworkId: gn, DeviceId: device,
						TransitGatewayConnectPeerArn: aws.String(tgwCpArn), LinkId: link,
					},
				)
				requireConflict(t, err)
			},
		},
		{
			name: "associate_connect_peer_link_not_associated_with_device",
			run: func(t *testing.T, client *networkmanagersdk.Client) {
				t.Helper()

				ctx := t.Context()

				gn, device, link := setupSiteDeviceLink(t, client)

				cn, err := client.CreateCoreNetwork(ctx, &networkmanagersdk.CreateCoreNetworkInput{GlobalNetworkId: gn})
				require.NoError(t, err)

				vpcAttachment, err := client.CreateVpcAttachment(ctx, &networkmanagersdk.CreateVpcAttachmentInput{
					CoreNetworkId: cn.CoreNetwork.CoreNetworkId,
					VpcArn:        aws.String("arn:aws:ec2:us-east-1:000000000000:vpc/vpc-precondition"),
					SubnetArns:    []string{"arn:aws:ec2:us-east-1:000000000000:subnet/subnet-precondition"},
				})
				require.NoError(t, err)

				connectAttachment, err := client.CreateConnectAttachment(
					ctx, &networkmanagersdk.CreateConnectAttachmentInput{
						CoreNetworkId: cn.CoreNetwork.CoreNetworkId, EdgeLocation: aws.String("us-east-1"),
						Options:               &types.ConnectAttachmentOptions{Protocol: types.TunnelProtocolNoEncap},
						TransportAttachmentId: vpcAttachment.VpcAttachment.Attachment.AttachmentId,
					},
				)
				require.NoError(t, err)

				connectPeer, err := client.CreateConnectPeer(ctx, &networkmanagersdk.CreateConnectPeerInput{
					ConnectAttachmentId: connectAttachment.ConnectAttachment.Attachment.AttachmentId,
					PeerAddress:         aws.String("10.0.0.1"),
				})
				require.NoError(t, err)

				_, err = client.AssociateConnectPeer(ctx, &networkmanagersdk.AssociateConnectPeerInput{
					GlobalNetworkId: gn, DeviceId: device,
					ConnectPeerId: connectPeer.ConnectPeer.ConnectPeerId, LinkId: link,
				})
				requireConflict(t, err)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, client := newTestHandlerAndClient(t)
			tc.run(t, client)
		})
	}
}

// setupSiteDeviceLink creates a global network, a site, a device and a link
// all sharing that site, returning the global network/device/link ids -- the
// common fixture most precondition cases build on.
func setupSiteDeviceLink(
	t *testing.T, client *networkmanagersdk.Client,
) (*string, *string, *string) {
	t.Helper()

	ctx := t.Context()

	gnOut, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
	require.NoError(t, err)

	siteOut, err := client.CreateSite(ctx, &networkmanagersdk.CreateSiteInput{
		GlobalNetworkId: gnOut.GlobalNetwork.GlobalNetworkId,
	})
	require.NoError(t, err)

	deviceOut, err := client.CreateDevice(ctx, &networkmanagersdk.CreateDeviceInput{
		GlobalNetworkId: gnOut.GlobalNetwork.GlobalNetworkId, SiteId: siteOut.Site.SiteId,
	})
	require.NoError(t, err)

	linkOut, err := client.CreateLink(ctx, &networkmanagersdk.CreateLinkInput{
		GlobalNetworkId: gnOut.GlobalNetwork.GlobalNetworkId, SiteId: siteOut.Site.SiteId,
		Bandwidth: &types.Bandwidth{DownloadSpeed: aws.Int32(1), UploadSpeed: aws.Int32(1)},
	})
	require.NoError(t, err)

	return gnOut.GlobalNetwork.GlobalNetworkId, deviceOut.Device.DeviceId, linkOut.Link.LinkId
}

// requireConflict asserts err is a real ConflictException as the pinned
// aws-sdk-go-v2 client would decode it -- not just a non-nil error, since a
// wire-shape regression (e.g. falling back to *smithy.GenericAPIError) would
// otherwise slip through the same assertion.
func requireConflict(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)

	var conflict *types.ConflictException
	require.ErrorAs(t, err, &conflict)
}
