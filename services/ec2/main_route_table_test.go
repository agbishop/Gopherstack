package ec2_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ec2"
)

// findMainRouteTable returns vpcID's main route table from a
// DescribeRouteTables result.
func findMainRouteTable(t *testing.T, b *ec2.InMemoryBackend, vpcID string) *ec2.RouteTable {
	t.Helper()

	for _, rt := range b.DescribeRouteTables(nil) {
		if rt.VPCID == vpcID && rt.Main {
			return rt
		}
	}

	return nil
}

// TestCreateVpc_MainRouteTable verifies CreateVpc creates a main route table
// with a local route for the VPC's CIDR and an implicit VPC-wide association
// (ec2@v1.319.1 types.RouteTableAssociation: "A subnet ID is not returned
// for an implicit association") -- gopherstack-y71o.
func TestCreateVpc_MainRouteTable(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("000000000000", "us-east-1")

	vpc, err := b.CreateVpc("10.50.0.0/16", "default")
	require.NoError(t, err)

	main := findMainRouteTable(t, b, vpc.ID)
	require.NotNil(t, main, "CreateVpc must create a main route table for the new VPC")

	require.Len(t, main.Routes, 1)
	assert.Equal(t, "10.50.0.0/16", main.Routes[0].DestinationCIDR)
	assert.Equal(t, "local", main.Routes[0].GatewayID)

	require.Len(t, main.Associations, 1)
	assert.True(t, main.Associations[0].Main)
	assert.Empty(t, main.Associations[0].SubnetID)
}

// TestDeleteVpc_MainRouteTableCascades verifies a freshly created VPC --
// which now carries only its own auto-created main route table -- still
// deletes cleanly. This is the landmine gopherstack-y71o calls out:
// registering the main table in routeTableIDsByVPC without a carve-out in
// vpcDependencyViolationLocked would make every DeleteVpc in the suite fail
// with a spurious DependencyViolation.
func TestDeleteVpc_MainRouteTableCascades(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("000000000000", "us-east-1")

	vpc, err := b.CreateVpc("10.51.0.0/16", "default")
	require.NoError(t, err)

	main := findMainRouteTable(t, b, vpc.ID)
	require.NotNil(t, main)

	require.NoError(t, b.DeleteVpc(vpc.ID), "DeleteVpc must succeed for a VPC with only its main route table")

	assert.Empty(t, b.DescribeRouteTables([]string{main.ID}), "main route table must be deleted with the VPC")
	assert.Empty(t, b.DescribeVpcs([]string{vpc.ID}))
}

// TestDeleteRouteTable_MainRouteTableRejected verifies DeleteRouteTable
// refuses to delete a VPC's main route table directly, matching real AWS
// (only DeleteVpc removes it).
func TestDeleteRouteTable_MainRouteTableRejected(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("000000000000", "us-east-1")

	vpc, err := b.CreateVpc("10.52.0.0/16", "default")
	require.NoError(t, err)

	main := findMainRouteTable(t, b, vpc.ID)
	require.NotNil(t, main)

	err = b.DeleteRouteTable(main.ID)
	require.Error(t, err)
	require.ErrorIs(t, err, ec2.ErrDependencyViolation)
	// The main-table guard and the "has subnet associations" guard both
	// return ErrDependencyViolation, and a main table always carries its
	// implicit association -- so ErrorIs alone can't tell them apart and
	// would pass even with the main-table guard deleted outright (the
	// associations check would fire instead, for the wrong reason). Assert
	// the message to pin the specific guard.
	assert.Contains(t, err.Error(), "is the main route table for",
		"must be rejected by the main-table guard, not the subnet-associations guard")
	assert.NotEmpty(t, b.DescribeRouteTables([]string{main.ID}))
}

// TestDisassociateRouteTable_MainAssociationRejected verifies the implicit
// main-route-table association cannot be disassociated -- doing so would
// leave a Main route table with no implicit association, an invariant break
// nothing else in this package expects.
func TestDisassociateRouteTable_MainAssociationRejected(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("000000000000", "us-east-1")

	vpc, err := b.CreateVpc("10.53.0.0/16", "default")
	require.NoError(t, err)

	main := findMainRouteTable(t, b, vpc.ID)
	require.NotNil(t, main)
	require.Len(t, main.Associations, 1)
	mainAssocID := main.Associations[0].ID

	err = b.DisassociateRouteTable(mainAssocID)
	require.Error(t, err)
	require.ErrorIs(t, err, ec2.ErrInvalidParameter)

	rts := b.DescribeRouteTables([]string{main.ID})
	require.Len(t, rts, 1)
	assert.Len(t, rts[0].Associations, 1, "implicit main association must survive the rejected disassociate")
}

// TestReplaceRouteTableAssociation_MainAssociationRejected verifies
// reassigning a VPC's main route table via the implicit association ID is
// rejected without mutating any state. The original lookup loop spliced the
// matched association out of its table before checking whether the move was
// valid, so a rejected implicit association was still destructively removed.
func TestReplaceRouteTableAssociation_MainAssociationRejected(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("000000000000", "us-east-1")

	vpc, err := b.CreateVpc("10.54.0.0/16", "default")
	require.NoError(t, err)

	other, err := b.CreateRouteTable(vpc.ID)
	require.NoError(t, err)

	main := findMainRouteTable(t, b, vpc.ID)
	require.NotNil(t, main)
	require.Len(t, main.Associations, 1)
	mainAssocID := main.Associations[0].ID

	_, err = b.ReplaceRouteTableAssociation(mainAssocID, other.ID)
	require.Error(t, err)
	require.ErrorIs(t, err, ec2.ErrInvalidParameter)

	rts := b.DescribeRouteTables([]string{main.ID})
	require.Len(t, rts, 1)
	require.Len(t, rts[0].Associations, 1, "implicit main association must not be removed by a rejected replace")
	assert.Equal(t, mainAssocID, rts[0].Associations[0].ID)

	rts = b.DescribeRouteTables([]string{other.ID})
	require.Len(t, rts, 1)
	assert.Empty(t, rts[0].Associations, "association must not have moved to the new table")
}

// TestHandler_DescribeRouteTables_MainAssociation verifies the wire response
// surfaces the implicit main association's <main>true</main> element
// (ec2@v1.319.1 deserializers.go awsEc2query_deserializeDocumentRouteTableAssociation
// reads a "main" element under each associationSet item).
func TestHandler_DescribeRouteTables_MainAssociation(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("000000000000", "us-east-1")
	h := ec2.NewHandler(b)
	h.AccountID = "000000000000"
	h.Region = "us-east-1"

	vpc, err := b.CreateVpc("10.55.0.0/16", "default")
	require.NoError(t, err)

	resp, err := ec2.ExportDispatch(h, url.Values{
		"Action":           {"DescribeRouteTables"},
		"Filter.1.Name":    {"vpc-id"},
		"Filter.1.Value.1": {vpc.ID},
	})
	require.NoError(t, err)
	assert.Contains(t, resp, "<main>true</main>", "implicit main association must report main=true")
}
