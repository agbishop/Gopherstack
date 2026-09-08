package integration_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2sdk "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ELBv2Audit_CapacityReservation exercises the round-trip between
// ModifyCapacityReservation and DescribeCapacityReservation. AWS persists the
// requested minimum capacity and the last-modified time; a fresh load balancer
// should report no reserved capacity until ModifyCapacityReservation is called.
func TestIntegration_ELBv2Audit_CapacityReservation(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createELBv2Client(t)
	ctx := t.Context()

	subnetIDs, _ := mustCreateELBv2NetworkPrereqs(t, "10.93", 2)

	createOut, err := client.CreateLoadBalancer(ctx, &elbv2sdk.CreateLoadBalancerInput{
		Name:    aws.String("it-audit-cap-lb"),
		Scheme:  elbv2types.LoadBalancerSchemeEnumInternetFacing,
		Type:    elbv2types.LoadBalancerTypeEnumApplication,
		Subnets: subnetIDs,
	})
	require.NoError(t, err)
	require.Len(t, createOut.LoadBalancers, 1)
	lbArn := aws.ToString(createOut.LoadBalancers[0].LoadBalancerArn)

	// Before any modification, no minimum capacity is reserved.
	descBefore, err := client.DescribeCapacityReservation(ctx, &elbv2sdk.DescribeCapacityReservationInput{
		LoadBalancerArn: aws.String(lbArn),
	})
	require.NoError(t, err)
	assert.Nil(t, descBefore.MinimumLoadBalancerCapacity)

	// Reserve a minimum capacity.
	const reservedUnits int32 = 100

	modOut, err := client.ModifyCapacityReservation(ctx, &elbv2sdk.ModifyCapacityReservationInput{
		LoadBalancerArn: aws.String(lbArn),
		MinimumLoadBalancerCapacity: &elbv2types.MinimumLoadBalancerCapacity{
			CapacityUnits: aws.Int32(reservedUnits),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, modOut.MinimumLoadBalancerCapacity)
	assert.Equal(t, reservedUnits, aws.ToInt32(modOut.MinimumLoadBalancerCapacity.CapacityUnits))

	// DescribeCapacityReservation now reflects the modification.
	descAfter, err := client.DescribeCapacityReservation(ctx, &elbv2sdk.DescribeCapacityReservationInput{
		LoadBalancerArn: aws.String(lbArn),
	})
	require.NoError(t, err)
	require.NotNil(t, descAfter.MinimumLoadBalancerCapacity)
	assert.Equal(t, reservedUnits, aws.ToInt32(descAfter.MinimumLoadBalancerCapacity.CapacityUnits))
	require.NotNil(t, descAfter.LastModifiedTime)
	assert.False(t, descAfter.LastModifiedTime.IsZero())

	// Cleanup.
	_, _ = client.DeleteLoadBalancer(ctx, &elbv2sdk.DeleteLoadBalancerInput{
		LoadBalancerArn: aws.String(lbArn),
	})
}

// TestIntegration_ELBv2Audit_IpPools verifies that ModifyIpPools persists the IPAM
// pool configuration and that the change is reflected back via DescribeLoadBalancers.
func TestIntegration_ELBv2Audit_IpPools(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createELBv2Client(t)
	ctx := t.Context()

	subnetIDs, _ := mustCreateELBv2NetworkPrereqs(t, "10.94", 2)

	createOut, err := client.CreateLoadBalancer(ctx, &elbv2sdk.CreateLoadBalancerInput{
		Name:    aws.String("it-audit-ippool-lb"),
		Scheme:  elbv2types.LoadBalancerSchemeEnumInternetFacing,
		Type:    elbv2types.LoadBalancerTypeEnumApplication,
		Subnets: subnetIDs,
	})
	require.NoError(t, err)
	require.Len(t, createOut.LoadBalancers, 1)
	lbArn := aws.ToString(createOut.LoadBalancers[0].LoadBalancerArn)

	const poolID = "ipam-pool-0123456789abcdef0"

	modOut, err := client.ModifyIpPools(ctx, &elbv2sdk.ModifyIpPoolsInput{
		LoadBalancerArn: aws.String(lbArn),
		IpamPools: &elbv2types.IpamPools{
			Ipv4IpamPoolId: aws.String(poolID),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, modOut.IpamPools)
	assert.Equal(t, poolID, aws.ToString(modOut.IpamPools.Ipv4IpamPoolId))

	// DescribeLoadBalancers reflects the assigned IPAM pool.
	descOut, err := client.DescribeLoadBalancers(ctx, &elbv2sdk.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{lbArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut.LoadBalancers, 1)
	require.NotNil(t, descOut.LoadBalancers[0].IpamPools)
	assert.Equal(t, poolID, aws.ToString(descOut.LoadBalancers[0].IpamPools.Ipv4IpamPoolId))

	// Removing the IPv4 pool clears it.
	_, err = client.ModifyIpPools(ctx, &elbv2sdk.ModifyIpPoolsInput{
		LoadBalancerArn: aws.String(lbArn),
		RemoveIpamPools: []elbv2types.RemoveIpamPoolEnum{elbv2types.RemoveIpamPoolEnumIpv4},
	})
	require.NoError(t, err)

	descOut2, err := client.DescribeLoadBalancers(ctx, &elbv2sdk.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{lbArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut2.LoadBalancers, 1)
	if descOut2.LoadBalancers[0].IpamPools != nil {
		assert.Empty(t, aws.ToString(descOut2.LoadBalancers[0].IpamPools.Ipv4IpamPoolId))
	}

	// Cleanup.
	_, _ = client.DeleteLoadBalancer(ctx, &elbv2sdk.DeleteLoadBalancerInput{
		LoadBalancerArn: aws.String(lbArn),
	})
}
