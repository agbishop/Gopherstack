package directconnect_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	directconnectsdk "github.com/aws/aws-sdk-go-v2/service/directconnect"
	"github.com/aws/aws-sdk-go-v2/service/directconnect/types"
	"github.com/stretchr/testify/require"
)

// TestCreateLag_NumberOfConnectionsExceedsBandwidthCap verifies the real,
// checkable Lag.NumberOfConnections cap from its own doc comment: at most
// 4 connections at 1Gbps/10Gbps, or 2 at 100Gbps/400Gbps.
func TestCreateLag_NumberOfConnectionsExceedsBandwidthCap(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	_, err := client.CreateLag(ctx, &directconnectsdk.CreateLagInput{
		ConnectionsBandwidth: aws.String("100Gbps"),
		LagName:              aws.String("too-big-lag"),
		Location:             aws.String("EqDC2"),
		NumberOfConnections:  3, // cap for 100Gbps is 2
	})
	require.Error(t, err)

	var clientErr *types.DirectConnectClientException
	require.ErrorAs(t, err, &clientErr)
}

// TestAssociateConnectionWithLag_CapacityLimitExceeded verifies
// AssociateConnectionWithLag is the one op where LimitExceededException is
// reachable without any Tags input at all (PARITY.md wire-trap #8): a LAG
// already at its bandwidth-derived connection cap rejects one more.
func TestAssociateConnectionWithLag_CapacityLimitExceeded(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	lag, err := client.CreateLag(ctx, &directconnectsdk.CreateLagInput{
		ConnectionsBandwidth: aws.String("100Gbps"),
		LagName:              aws.String("full-lag"),
		Location:             aws.String("EqDC2"),
		NumberOfConnections:  2, // already at the 100Gbps cap
	})
	require.NoError(t, err)
	require.Len(t, lag.Connections, 2)

	extra, err := client.CreateConnection(ctx, &directconnectsdk.CreateConnectionInput{
		Bandwidth:      aws.String("100Gbps"),
		ConnectionName: aws.String("extra-conn"),
		Location:       aws.String("EqDC2"),
	})
	require.NoError(t, err)

	_, err = client.AssociateConnectionWithLag(ctx, &directconnectsdk.AssociateConnectionWithLagInput{
		ConnectionId: extra.ConnectionId,
		LagId:        lag.LagId,
	})
	require.Error(t, err)

	var limitErr *types.LimitExceededException
	require.ErrorAs(t, err, &limitErr)
}

// TestDisassociateConnectionFromLag_MismatchIsClientError verifies
// disassociating a connection from a LAG it does not belong to is
// rejected, not silently accepted.
func TestDisassociateConnectionFromLag_MismatchIsClientError(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	lag, err := client.CreateLag(ctx, &directconnectsdk.CreateLagInput{
		ConnectionsBandwidth: aws.String("1Gbps"),
		LagName:              aws.String("lag-a"),
		Location:             aws.String("EqDC2"),
		NumberOfConnections:  1,
	})
	require.NoError(t, err)

	standalone, err := client.CreateConnection(ctx, &directconnectsdk.CreateConnectionInput{
		Bandwidth:      aws.String("1Gbps"),
		ConnectionName: aws.String("standalone-conn"),
		Location:       aws.String("EqDC2"),
	})
	require.NoError(t, err)

	_, err = client.DisassociateConnectionFromLag(ctx, &directconnectsdk.DisassociateConnectionFromLagInput{
		ConnectionId: standalone.ConnectionId,
		LagId:        lag.LagId,
	})
	require.Error(t, err)
}

// TestDeleteLag_ActiveConnectionRejected verifies DeleteLag enforces
// api_op_DeleteLag.go:12-13: "You cannot delete a LAG if it has active
// virtual interfaces or hosted connections".
func TestDeleteLag_ActiveConnectionRejected(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	lag, err := client.CreateLag(ctx, &directconnectsdk.CreateLagInput{
		ConnectionsBandwidth: aws.String("1Gbps"),
		LagName:              aws.String("lag-with-members"),
		Location:             aws.String("EqDC2"),
		NumberOfConnections:  1,
	})
	require.NoError(t, err)
	require.Len(t, lag.Connections, 1)

	_, err = client.DeleteLag(ctx, &directconnectsdk.DeleteLagInput{LagId: lag.LagId})
	require.Error(t, err)

	var clientErr *types.DirectConnectClientException
	require.ErrorAs(t, err, &clientErr)
}

// TestDisassociateConnectionFromLag_MinimumLinks verifies
// api_op_DisassociateConnectionFromLag.go:20-23: disassociating a
// connection that would drop the LAG below MinimumLinks fails, except when
// it is the LAG's last member (which is allowed, leaving an empty LAG).
func TestDisassociateConnectionFromLag_MinimumLinks(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	lag, err := client.CreateLag(ctx, &directconnectsdk.CreateLagInput{
		ConnectionsBandwidth: aws.String("1Gbps"),
		LagName:              aws.String("lag-min-links"),
		Location:             aws.String("EqDC2"),
		NumberOfConnections:  2,
	})
	require.NoError(t, err)
	require.Len(t, lag.Connections, 2)

	_, err = client.UpdateLag(ctx, &directconnectsdk.UpdateLagInput{
		LagId:        lag.LagId,
		MinimumLinks: 2,
	})
	require.NoError(t, err)

	first := lag.Connections[0].ConnectionId
	second := lag.Connections[1].ConnectionId

	_, err = client.DisassociateConnectionFromLag(ctx, &directconnectsdk.DisassociateConnectionFromLagInput{
		ConnectionId: first,
		LagId:        lag.LagId,
	})
	require.Error(t, err, "dropping to 1 connection with MinimumLinks=2 must fail")

	var clientErr *types.DirectConnectClientException
	require.ErrorAs(t, err, &clientErr)

	_, err = client.UpdateLag(ctx, &directconnectsdk.UpdateLagInput{
		LagId:        lag.LagId,
		MinimumLinks: 1,
	})
	require.NoError(t, err)

	_, err = client.DisassociateConnectionFromLag(ctx, &directconnectsdk.DisassociateConnectionFromLagInput{
		ConnectionId: first,
		LagId:        lag.LagId,
	})
	require.NoError(t, err, "1 remaining connection meets MinimumLinks=1")

	_, err = client.DisassociateConnectionFromLag(ctx, &directconnectsdk.DisassociateConnectionFromLagInput{
		ConnectionId: second,
		LagId:        lag.LagId,
	})
	require.NoError(t, err, "the last member of a LAG may always be disassociated, even below MinimumLinks")
}

// TestAssociateConnectionWithLag_ReassociationMinimumLinks verifies
// api_op_AssociateConnectionWithLag.go:17-20: re-associating a connection
// away from its current LAG into a different one fails if that would drop
// the original LAG below its MinimumLinks -- with NO last-member exception,
// unlike DisassociateConnectionFromLag (gopherstack-55so).
func TestAssociateConnectionWithLag_ReassociationMinimumLinks(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	origLag, err := client.CreateLag(ctx, &directconnectsdk.CreateLagInput{
		ConnectionsBandwidth: aws.String("1Gbps"),
		LagName:              aws.String("orig-lag"),
		Location:             aws.String("EqDC2"),
		NumberOfConnections:  2,
	})
	require.NoError(t, err)
	require.Len(t, origLag.Connections, 2)

	_, err = client.UpdateLag(ctx, &directconnectsdk.UpdateLagInput{
		LagId:        origLag.LagId,
		MinimumLinks: 2,
	})
	require.NoError(t, err)

	destLag, err := client.CreateLag(ctx, &directconnectsdk.CreateLagInput{
		ConnectionsBandwidth: aws.String("1Gbps"),
		LagName:              aws.String("dest-lag"),
		Location:             aws.String("EqDC2"),
		NumberOfConnections:  1,
	})
	require.NoError(t, err)

	toMove := origLag.Connections[0].ConnectionId

	_, err = client.AssociateConnectionWithLag(ctx, &directconnectsdk.AssociateConnectionWithLagInput{
		ConnectionId: toMove,
		LagId:        destLag.LagId,
	})
	require.Error(t, err, "moving a connection out of orig-lag would drop it to 1 connection with MinimumLinks=2")

	var clientErr *types.DirectConnectClientException
	require.ErrorAs(t, err, &clientErr)

	_, err = client.UpdateLag(ctx, &directconnectsdk.UpdateLagInput{
		LagId:        origLag.LagId,
		MinimumLinks: 1,
	})
	require.NoError(t, err)

	_, err = client.AssociateConnectionWithLag(ctx, &directconnectsdk.AssociateConnectionWithLagInput{
		ConnectionId: toMove,
		LagId:        destLag.LagId,
	})
	require.NoError(t, err, "dropping orig-lag to exactly MinimumLinks=1 must succeed")

	lastMember := origLag.Connections[1].ConnectionId

	_, err = client.AssociateConnectionWithLag(ctx, &directconnectsdk.AssociateConnectionWithLagInput{
		ConnectionId: lastMember,
		LagId:        destLag.LagId,
	})
	require.Error(t, err, "unlike DisassociateConnectionFromLag, re-association carries no last-member exception")

	require.ErrorAs(t, err, &clientErr)
}

// TestAssociateConnectionWithLag_BandwidthMismatch verifies
// api_op_AssociateConnectionWithLag.go:15-16: "its bandwidth must match the
// bandwidth for the LAG".
func TestAssociateConnectionWithLag_BandwidthMismatch(t *testing.T) {
	t.Parallel()

	_, client := newTestHandlerAndClient(t)
	ctx := t.Context()

	lag, err := client.CreateLag(ctx, &directconnectsdk.CreateLagInput{
		ConnectionsBandwidth: aws.String("1Gbps"),
		LagName:              aws.String("lag-1gbps"),
		Location:             aws.String("EqDC2"),
		NumberOfConnections:  1,
	})
	require.NoError(t, err)

	mismatched, err := client.CreateConnection(ctx, &directconnectsdk.CreateConnectionInput{
		Bandwidth:      aws.String("10Gbps"),
		ConnectionName: aws.String("10gbps-conn"),
		Location:       aws.String("EqDC2"),
	})
	require.NoError(t, err)

	_, err = client.AssociateConnectionWithLag(ctx, &directconnectsdk.AssociateConnectionWithLagInput{
		ConnectionId: mismatched.ConnectionId,
		LagId:        lag.LagId,
	})
	require.Error(t, err)

	var clientErr *types.DirectConnectClientException
	require.ErrorAs(t, err, &clientErr)
}
