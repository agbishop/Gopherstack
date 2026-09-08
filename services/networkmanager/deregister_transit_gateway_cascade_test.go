package networkmanager_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	networkmanagersdk "github.com/aws/aws-sdk-go-v2/service/networkmanager"
	"github.com/aws/aws-sdk-go-v2/service/networkmanager/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/networkmanager"
)

// fakeEC2Resolver implements networkmanager.EC2Resolver for tests that need
// DeregisterTransitGateway's real cascade (gopherstack-3fkj) exercised
// without a real services/ec2 backend. Every Resolve* accepts unconditionally
// -- these tests aren't exercising ARN validation -- except
// CustomerGatewayArnsForTransitGateway, which answers from a caller-supplied
// map, mirroring the real EC2Resolver's services/ec2 VpnConnection lookup.
type fakeEC2Resolver struct {
	cgwArnsByTgwArn map[string][]string
}

func (f *fakeEC2Resolver) ResolveVpc(string) bool                       { return true }
func (f *fakeEC2Resolver) ResolveSubnet(string) bool                    { return true }
func (f *fakeEC2Resolver) ResolveCustomerGateway(string) bool           { return true }
func (f *fakeEC2Resolver) ResolveTransitGateway(string) bool            { return true }
func (f *fakeEC2Resolver) ResolveVpnConnection(string) bool             { return true }
func (f *fakeEC2Resolver) ResolveTransitGatewayConnectPeer(string) bool { return true }
func (f *fakeEC2Resolver) ResolveTransitGatewayRouteTable(string) bool  { return true }

func (f *fakeEC2Resolver) TransitGatewayRouteTableForAttachment(string) (string, bool) {
	return "", false
}

func (f *fakeEC2Resolver) TransitGatewayRoutes(string) []networkmanager.EC2TransitGatewayRoute {
	return nil
}

func (f *fakeEC2Resolver) CustomerGatewayArnsForTransitGateway(transitGatewayArn string) []string {
	return f.cgwArnsByTgwArn[transitGatewayArn]
}

// TestDeregisterTransitGateway_CascadesScopedCustomerGatewayAssociations proves
// gopherstack-3fkj's fix: DeregisterTransitGateway removes the
// CustomerGatewayAssociation for a customer gateway the wired EC2Resolver
// reports as attached to the deregistered transit gateway, and leaves an
// association for a DIFFERENT transit gateway alone -- the cascade is scoped,
// not blanket.
func TestDeregisterTransitGateway_CascadesScopedCustomerGatewayAssociations(t *testing.T) {
	t.Parallel()

	h, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	cgwArnCascaded := "arn:aws:ec2:us-east-1:000000000000:customer-gateway/cgw-0000000000000001"
	cgwArnUntouched := "arn:aws:ec2:us-east-1:000000000000:customer-gateway/cgw-0000000000000002"
	tgwArnDeregistered := "arn:aws:ec2:us-east-1:000000000000:transit-gateway/tgw-0000000000000001"
	tgwArnOther := "arn:aws:ec2:us-east-1:000000000000:transit-gateway/tgw-0000000000000002"

	h.Backend.SetEC2Resolver(&fakeEC2Resolver{
		cgwArnsByTgwArn: map[string][]string{tgwArnDeregistered: {cgwArnCascaded}},
	})

	gn, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
	require.NoError(t, err)

	device, err := client.CreateDevice(
		ctx,
		&networkmanagersdk.CreateDeviceInput{GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId},
	)
	require.NoError(t, err)

	for _, tgwArn := range []string{tgwArnDeregistered, tgwArnOther} {
		_, regErr := client.RegisterTransitGateway(ctx, &networkmanagersdk.RegisterTransitGatewayInput{
			GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, TransitGatewayArn: aws.String(tgwArn),
		})
		require.NoError(t, regErr)
	}

	for _, cgwArn := range []string{cgwArnCascaded, cgwArnUntouched} {
		_, assocErr := client.AssociateCustomerGateway(ctx, &networkmanagersdk.AssociateCustomerGatewayInput{
			GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, CustomerGatewayArn: aws.String(cgwArn),
			DeviceId: device.Device.DeviceId,
		})
		require.NoError(t, assocErr)
	}

	require.Eventually(t, func() bool {
		l, listErr := client.GetCustomerGatewayAssociations(ctx, &networkmanagersdk.GetCustomerGatewayAssociationsInput{
			GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
		})
		if listErr != nil || len(l.CustomerGatewayAssociations) != 2 {
			return false
		}

		for _, a := range l.CustomerGatewayAssociations {
			if a.State != types.CustomerGatewayAssociationStateAvailable {
				return false
			}
		}

		return true
	}, defaultAsyncWait, defaultAsyncPoll)

	_, err = client.DeregisterTransitGateway(ctx, &networkmanagersdk.DeregisterTransitGatewayInput{
		GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, TransitGatewayArn: aws.String(tgwArnDeregistered),
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		l, listErr := client.GetCustomerGatewayAssociations(ctx, &networkmanagersdk.GetCustomerGatewayAssociationsInput{
			GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
		})

		return listErr == nil && len(l.CustomerGatewayAssociations) == 1 &&
			aws.ToString(l.CustomerGatewayAssociations[0].CustomerGatewayArn) == cgwArnUntouched &&
			l.CustomerGatewayAssociations[0].State == types.CustomerGatewayAssociationStateAvailable
	}, defaultAsyncWait, defaultAsyncPoll, "cascaded association should be gone; other TGW's association must survive")
}

// TestDeregisterTransitGateway_NilResolverLeavesAssociationsUntouched proves the
// nil-resolver rule: ~150 services stand up backends in tests with no
// cross-service hooks wired, and cli.go has not wired
// CustomerGatewayArnsForTransitGateway into networkManagerEC2ResolverAdapter as
// of this pass (gopherstack-3fkj), so a nil ec2Resolver must behave exactly as
// it did before this pass -- no cascade, no error, no panic.
func TestDeregisterTransitGateway_NilResolverLeavesAssociationsUntouched(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	cgwArn := "arn:aws:ec2:us-east-1:000000000000:customer-gateway/cgw-0000000000000003"
	tgwArn := "arn:aws:ec2:us-east-1:000000000000:transit-gateway/tgw-0000000000000003"

	gn, err := client.CreateGlobalNetwork(ctx, &networkmanagersdk.CreateGlobalNetworkInput{})
	require.NoError(t, err)

	device, err := client.CreateDevice(
		ctx,
		&networkmanagersdk.CreateDeviceInput{GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId},
	)
	require.NoError(t, err)

	_, err = client.RegisterTransitGateway(ctx, &networkmanagersdk.RegisterTransitGatewayInput{
		GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, TransitGatewayArn: aws.String(tgwArn),
	})
	require.NoError(t, err)

	_, err = client.AssociateCustomerGateway(ctx, &networkmanagersdk.AssociateCustomerGatewayInput{
		GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, CustomerGatewayArn: aws.String(cgwArn),
		DeviceId: device.Device.DeviceId,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		l, listErr := client.GetCustomerGatewayAssociations(ctx, &networkmanagersdk.GetCustomerGatewayAssociationsInput{
			GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
		})

		return listErr == nil && len(l.CustomerGatewayAssociations) == 1 &&
			l.CustomerGatewayAssociations[0].State == types.CustomerGatewayAssociationStateAvailable
	}, defaultAsyncWait, defaultAsyncPoll)

	_, err = client.DeregisterTransitGateway(ctx, &networkmanagersdk.DeregisterTransitGatewayInput{
		GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId, TransitGatewayArn: aws.String(tgwArn),
	})
	require.NoError(t, err)

	require.Never(t, func() bool {
		l, listErr := client.GetCustomerGatewayAssociations(ctx, &networkmanagersdk.GetCustomerGatewayAssociationsInput{
			GlobalNetworkId: gn.GlobalNetwork.GlobalNetworkId,
		})

		return listErr != nil || len(l.CustomerGatewayAssociations) != 1 ||
			l.CustomerGatewayAssociations[0].State != types.CustomerGatewayAssociationStateAvailable
	}, defaultAsyncWait, defaultAsyncPoll, "nil ec2Resolver must never cascade -- association must stay untouched")
}
