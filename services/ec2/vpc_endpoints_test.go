package ec2_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ec2"
)

func TestRejectVpcEndpointConnections(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	accepted, err := b.AcceptVpcEndpointConnections("vpce-svc-1", []string{"vpce-1"})
	require.NoError(t, err)
	require.Len(t, accepted, 1)

	unsuccessful, err := b.RejectVpcEndpointConnections("vpce-svc-1", []string{"vpce-1", "vpce-missing"})
	require.NoError(t, err)
	assert.Equal(t, []string{"vpce-missing"}, unsuccessful)

	conns := b.DescribeVpcEndpointConnections([]string{"vpce-svc-1"})
	require.Len(t, conns, 1)
	assert.Equal(t, "rejected", conns[0].State)

	_, err = b.RejectVpcEndpointConnections("", []string{"vpce-1"})
	require.ErrorIs(t, err, ec2.ErrInvalidParameter)

	_, err = b.RejectVpcEndpointConnections("vpce-svc-1", nil)
	require.ErrorIs(t, err, ec2.ErrInvalidParameter)
}

// TestDeleteVpcEndpoints verifies VPC endpoint deletion.
func TestDeleteVpcEndpoints(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	ep, err := b.CreateVpcEndpoint("vpc-default", "com.amazonaws.us-east-1.s3", "Gateway", nil)
	require.NoError(t, err)

	unsuccessful, err := b.DeleteVpcEndpoints([]string{ep.ID})
	require.NoError(t, err)
	assert.Empty(t, unsuccessful)

	remaining := b.DescribeVpcEndpoints(nil)
	assert.Empty(t, remaining)
}

// TestDeleteVpcEndpoints_RejectedForGatewayLoadBalancerWithRoutes locks
// real AWS's DeleteVpcEndpoints doc comment: "You can only delete Gateway
// Load Balancer endpoints when the routes that are associated with the
// endpoint are deleted." An Interface/Gateway endpoint with the same route
// table association is unaffected -- the guard is scoped to
// GatewayLoadBalancer endpoints only.
func TestDeleteVpcEndpoints_RejectedForGatewayLoadBalancerWithRoutes(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	rt, err := b.CreateRouteTable("vpc-default")
	require.NoError(t, err)

	gwlb, err := b.CreateVpcEndpointWithRouteTableIDs(
		"vpc-default", "com.amazonaws.us-east-1.gwlb-svc", "GatewayLoadBalancer",
		nil, []string{rt.ID},
	)
	require.NoError(t, err)

	unsuccessful, err := b.DeleteVpcEndpoints([]string{gwlb.ID})
	require.ErrorIs(t, err, ec2.ErrDependencyViolation)
	assert.Equal(t, []string{gwlb.ID}, unsuccessful)

	remaining := b.DescribeVpcEndpoints([]string{gwlb.ID})
	require.Len(t, remaining, 1, "endpoint must survive the rejected delete")

	// An Interface endpoint with the same route table association is not
	// subject to this guard.
	iface, err := b.CreateVpcEndpointWithRouteTableIDs(
		"vpc-default", "com.amazonaws.us-east-1.ec2", "Interface",
		nil, []string{rt.ID},
	)
	require.NoError(t, err)

	unsuccessful2, err := b.DeleteVpcEndpoints([]string{iface.ID})
	require.NoError(t, err)
	assert.Empty(t, unsuccessful2)

	// A Gateway Load Balancer endpoint with no route table associations is
	// not blocked.
	gwlbNoRoutes, err := b.CreateVpcEndpointWithRouteTableIDs(
		"vpc-default", "com.amazonaws.us-east-1.gwlb-svc2", "GatewayLoadBalancer",
		nil, nil,
	)
	require.NoError(t, err)

	unsuccessful3, err := b.DeleteVpcEndpoints([]string{gwlbNoRoutes.ID})
	require.NoError(t, err)
	assert.Empty(t, unsuccessful3)
}

// TestDeleteVpcEndpoints_NotFound verifies error on missing endpoint.

// TestDeleteVpcEndpoints_NotFound verifies error on missing endpoint.
func TestDeleteVpcEndpoints_NotFound(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	_, err := b.DeleteVpcEndpoints([]string{"vpce-nonexistent"})
	assert.Error(t, err)
}

// TestDeleteVpcEndpoints_EmptyList verifies error on empty list.

// TestDeleteVpcEndpoints_EmptyList verifies error on empty list.
func TestDeleteVpcEndpoints_EmptyList(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	_, err := b.DeleteVpcEndpoints(nil)
	assert.Error(t, err)
}

// TestReset_ClearsNewMaps verifies Reset clears snapshots and networkACLs.

// TestDescribeVpcEndpointServices verifies the endpoint services list.
func TestDescribeVpcEndpointServices(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")
	services := b.DescribeVpcEndpointServices()

	assert.NotEmpty(t, services)

	var foundS3 bool

	if slices.Contains(services, "com.amazonaws.us-east-1.s3") {
		foundS3 = true
	}

	assert.True(t, foundS3, "s3 endpoint service should be present")
}

// TestHTTP_DeleteVpcEndpoints verifies the HTTP handler for DeleteVpcEndpoints.
func TestHTTP_DeleteVpcEndpoints(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Create endpoint first
	rec := postForm(t, h, "Action=CreateVpcEndpoint&Version=2016-11-15"+
		"&VpcId=vpc-default&ServiceName=com.amazonaws.us-east-1.s3&VpcEndpointType=Gateway")
	require.Equal(t, http.StatusOK, rec.Code)

	b, ok := h.Backend.(*ec2.InMemoryBackend)
	require.True(t, ok)
	eps := b.DescribeVpcEndpoints(nil)
	require.Len(t, eps, 1)

	rec = postForm(t, h, "Action=DeleteVpcEndpoints&Version=2016-11-15&VpcEndpointId.1="+eps[0].ID)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestHandlerDescribeVpcEndpointServices verifies endpoint services.
func TestHandlerDescribeVpcEndpointServices(t *testing.T) {
	t.Parallel()

	h := newHandler()

	rec := postForm(t, h, "Action=DescribeVpcEndpointServices&Version=2016-11-15")
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "DescribeVpcEndpointServicesResponse")
	assert.Contains(t, rec.Body.String(), "s3")
}
