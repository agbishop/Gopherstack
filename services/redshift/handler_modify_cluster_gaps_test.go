package redshift_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gopherstack-igsa gap 1: ModifyCluster previously did not model
// PubliclyAccessible, VpcSecurityGroupIds, Port or ClusterVersion at all --
// none of the four request params were parsed, so a real client setting any
// of them would see the change silently dropped. Real ModifyClusterInput
// (redshift@v1.65.4 api_op_ModifyCluster.go) documents:
//
//   - Port *int32: "The option to change the port of an Amazon Redshift
//     cluster."
//   - VpcSecurityGroupIds []string: "A list of virtual private cloud (VPC)
//     security groups to be associated with the cluster. This change is
//     asynchronously applied as soon as possible." -- i.e. not gated by
//     ApplyImmediately.
//   - ClusterVersion *string: "The new version number of the Amazon Redshift
//     engine to upgrade to."
//   - PubliclyAccessible *bool: "If true, the cluster can be accessed from a
//     public network... Default: false"
//
// types.PendingModifiedValues (types/types.go:1491) lists ClusterVersion and
// PubliclyAccessible but not Port or VpcSecurityGroupIds, matching this
// backend's split: Port/VpcSecurityGroupIds apply unconditionally,
// ClusterVersion/PubliclyAccessible are gated by ApplyImmediately like the
// pre-existing NodeType/NumberOfNodes/Encrypted fields.

// TestModifyCluster_Port_AppliesUnconditionally verifies Port is accepted
// and applied even when ApplyImmediately=false (real Port has no entry in
// PendingModifiedValues, unlike NodeType/NumberOfNodes).
func TestModifyCluster_Port_AppliesUnconditionally(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=port-cluster")

	rec := postRedshiftForm(t, h,
		"Action=ModifyCluster&Version=2012-12-01&ClusterIdentifier=port-cluster"+
			"&Port=8192&ApplyImmediately=false")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "<Port>8192</Port>")

	rec = postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=port-cluster")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<Port>8192</Port>")
}

// TestModifyCluster_VpcSecurityGroupIds_AppliesUnconditionally verifies
// VpcSecurityGroupIds is accepted and applied even when ApplyImmediately is
// false, echoed back as VpcSecurityGroups>VpcSecurityGroup members.
func TestModifyCluster_VpcSecurityGroupIds_AppliesUnconditionally(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=vpcsg-cluster")

	rec := postRedshiftForm(t, h,
		"Action=ModifyCluster&Version=2012-12-01&ClusterIdentifier=vpcsg-cluster"+
			"&VpcSecurityGroupIds.VpcSecurityGroupId.1=sg-111&"+
			"VpcSecurityGroupIds.VpcSecurityGroupId.2=sg-222&ApplyImmediately=false")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "sg-111")
	assert.Contains(t, rec.Body.String(), "sg-222")

	rec = postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=vpcsg-cluster")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "sg-111")
	assert.Contains(t, rec.Body.String(), "sg-222")
}

// TestModifyCluster_ClusterVersion_PendingGating verifies ClusterVersion is
// applied immediately when ApplyImmediately=true, but left unapplied
// (pending, not visible on the live cluster) when ApplyImmediately=false --
// matching real types.PendingModifiedValues.ClusterVersion.
func TestModifyCluster_ClusterVersion_PendingGating(t *testing.T) {
	t.Parallel()

	h := newRedshiftHandler()
	postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=ver-cluster")

	rec := postRedshiftForm(t, h,
		"Action=ModifyCluster&Version=2012-12-01&ClusterIdentifier=ver-cluster"+
			"&ClusterVersion=1.0&ApplyImmediately=true")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "<ClusterVersion>1.0</ClusterVersion>")

	rec = postRedshiftForm(t, h,
		"Action=ModifyCluster&Version=2012-12-01&ClusterIdentifier=ver-cluster"+
			"&ClusterVersion=2.0&ApplyImmediately=false")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=ver-cluster")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<ClusterVersion>1.0</ClusterVersion>")
	assert.NotContains(t, rec.Body.String(), "<ClusterVersion>2.0</ClusterVersion>")
}

// TestModifyCluster_PubliclyAccessible_TriState verifies PubliclyAccessible
// behaves like the pre-existing Encrypted tri-state: omitting the field
// leaves the setting unchanged, and it is gated by ApplyImmediately like
// real types.PendingModifiedValues.PubliclyAccessible.
func TestModifyCluster_PubliclyAccessible_TriState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		modifyBody             string
		wantPubliclyAccessible string
	}{
		{
			name:                   "explicit_true_applies_immediately",
			modifyBody:             "&PubliclyAccessible=true&ApplyImmediately=true",
			wantPubliclyAccessible: "<PubliclyAccessible>true</PubliclyAccessible>",
		},
		{
			name:                   "unset_leaves_unchanged",
			modifyBody:             "&NodeType=ra3.xlplus&ApplyImmediately=true",
			wantPubliclyAccessible: "<PubliclyAccessible>false</PubliclyAccessible>",
		},
		{
			name:                   "explicit_true_not_applied_immediately_stays_false",
			modifyBody:             "&PubliclyAccessible=true&ApplyImmediately=false",
			wantPubliclyAccessible: "<PubliclyAccessible>false</PubliclyAccessible>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRedshiftHandler()
			postRedshiftForm(t, h, "Action=CreateCluster&Version=2012-12-01&ClusterIdentifier=pubacc-cluster")

			rec := postRedshiftForm(t, h,
				"Action=ModifyCluster&Version=2012-12-01&ClusterIdentifier=pubacc-cluster"+tt.modifyBody)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			rec = postRedshiftForm(t, h, "Action=DescribeClusters&Version=2012-12-01&ClusterIdentifier=pubacc-cluster")
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantPubliclyAccessible)
		})
	}
}
