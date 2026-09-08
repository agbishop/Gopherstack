package ec2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReset_ClearsAccountAndSideMapState locks the gopherstack-tl4v fix: Reset
// re-initialises every plain-map/pointer/scalar field on InMemoryBackend, not
// just niIPv6Addresses (the field the issue title named). Every field below
// is populated through its real backend method (the same one an AWS API call
// would drive), Reset is called once, and each field is asserted individually
// so a regression names itself instead of hiding behind one shared assertion.
func TestReset_ClearsAccountAndSideMapState(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend("123456789012", "us-east-1")

	vol, err := b.CreateVolume("us-east-1a", "gp2", 20, "")
	require.NoError(t, err)
	snap, err := b.CreateSnapshot(vol.ID, "reset-leak-test")
	require.NoError(t, err)
	eni, err := b.CreateNetworkInterface("subnet-default", "reset-leak-test")
	require.NoError(t, err)
	insts, err := b.RunInstances("ami-test", "t2.micro", "", 1)
	require.NoError(t, err)
	instanceID := insts[0].ID
	pc, err := b.CreateVpcPeeringConnection("vpc-default", "vpc-default")
	require.NoError(t, err)
	svcCfg, err := b.CreateVpcEndpointServiceConfiguration(false, nil)
	require.NoError(t, err)

	// ---- populate every field via the real backend method an AWS op drives ----

	_, err = b.AssignIpv6Addresses(eni.ID, 2)
	require.NoError(t, err)
	require.NoError(t, b.ModifySnapshotTier(snap.SnapshotID, "archive"))
	// ModifySnapshotAttribute (handler_snapshots.go) is a pre-existing stub
	// that never writes b.snapshotAttributes -- a separate parity gap, not
	// this issue. Populate the field directly (this test is in-package) so
	// Reset's handling of it is still covered.
	b.snapshotAttributes[snap.SnapshotID] = map[string]string{"createVolumePermission": "public"}
	_, err = b.AssociateSecurityGroupVpc("sg-default", "vpc-default")
	require.NoError(t, err)
	require.NoError(t, b.ModifyVpcTenancy("vpc-default", "dedicated"))
	require.NoError(t, b.ModifyVpcPeeringConnectionOptions(
		pc.VpcPeeringConnectionID, PeeringConnectionOptions{AllowDNSResolutionFromRemoteVPC: true},
	))
	_, err = b.AssociateSubnetCidrBlock("subnet-default", "2001:db8::/64")
	require.NoError(t, err)
	_, unsuccessful := b.ModifyInstanceCreditSpecification(
		[]InstanceCreditSpec{{InstanceID: instanceID, CPUCredits: "unlimited"}},
	)
	require.Empty(t, unsuccessful)
	require.NoError(t, b.ModifyInstanceMetadataDefaults("required", "enabled", "enabled", 2))
	b.RegisterInstanceEventNotificationAttributes(true)
	require.NoError(t, b.ModifyIDFormat("instance", true))
	_, err = b.ModifyVpcEndpointServicePermissions(
		svcCfg.ServiceID, []string{"arn:aws:iam::123456789012:root"}, nil,
	)
	require.NoError(t, err)
	_, err = b.CreateSubnetCidrReservation("subnet-default", "172.31.0.0/28", "prefix", "reset-leak-test")
	require.NoError(t, err)
	require.NoError(t, b.DisableImage("ami-leaktest"))
	require.NoError(t, b.EnableImageDeprecation("ami-leaktest", "2030-01-01T00:00:00Z"))
	require.NoError(t, b.EnableImageDeregistrationProtection("ami-leaktest"))
	require.NoError(t, b.ModifyImageAttribute("ami-leaktest", "description", "reset-leak-test"))
	require.NoError(t, b.EnableVgwRoutePropagation("rtb-leaktest", "vgw-leaktest"))
	require.NoError(t, b.EnableFastLaunch("ami-leaktest", FastLaunchConfig{}))
	require.NoError(t, b.EnableFastSnapshotRestores([]string{snap.SnapshotID}, []string{"us-east-1a"}))
	_, err = b.CreateSpotDatafeedSubscription("leak-bucket", "leak-prefix")
	require.NoError(t, err)
	_, _, err = b.CreateFleet(FleetCreateInput{})
	require.NoError(t, err)
	_, err = b.RequestSpotFleet(SpotFleetRequestConfig{
		LaunchSpecifications: []SpotFleetLaunchSpecification{{ImageID: "ami-leaktest", InstanceType: "t2.micro"}},
	})
	require.NoError(t, err)
	require.NoError(t, b.EnableSnapshotBlockPublicAccess("block-new-sharing"))
	require.NoError(t, b.ModifyEbsDefaultKmsKeyID("arn:aws:kms:us-east-1:123456789012:key/leak-test"))
	require.NoError(t, b.EnableImageBlockPublicAccess("block-new-sharing"))
	require.NoError(t, b.ModifyDefaultCreditSpecification("unlimited"))
	b.EnableEbsEncryptionByDefault()
	b.EnableSerialConsoleAccess()

	fields := []struct {
		isEmpty func() bool
		name    string
	}{
		{func() bool { return len(b.niIPv6Addresses) == 0 }, "niIPv6Addresses"},
		{func() bool { return len(b.snapshotTiers) == 0 }, "snapshotTiers"},
		{func() bool { return len(b.snapshotAttributes) == 0 }, "snapshotAttributes"},
		{func() bool { return len(b.sgVpcAssociations) == 0 }, "sgVpcAssociations"},
		{func() bool { return len(b.vpcTenancy) == 0 }, "vpcTenancy"},
		{func() bool { return len(b.vpcPeeringOptions) == 0 }, "vpcPeeringOptions"},
		{func() bool { return len(b.subnetCIDRAssociations) == 0 }, "subnetCIDRAssociations"},
		{func() bool { return len(b.instanceCreditSpecs) == 0 }, "instanceCreditSpecs"},
		{func() bool { return b.instanceMetadataDefaults == nil }, "instanceMetadataDefaults"},
		{func() bool { return b.instanceEventNotifAttrs == nil }, "instanceEventNotifAttrs"},
		{func() bool { return len(b.idFormatSettings) == 0 }, "idFormatSettings"},
		{func() bool { return len(b.vpcEndpointServicePermissions) == 0 }, "vpcEndpointServicePermissions"},
		{func() bool { return len(b.subnetCIDRReservations) == 0 }, "subnetCIDRReservations"},
		{func() bool { return len(b.imageDisabled) == 0 }, "imageDisabled"},
		{func() bool { return len(b.imageDeprecated) == 0 }, "imageDeprecated"},
		{func() bool { return len(b.imageDeregistrationProtection) == 0 }, "imageDeregistrationProtection"},
		{func() bool { return len(b.imageAttributes) == 0 }, "imageAttributes"},
		{func() bool { return len(b.vgwRoutePropagation) == 0 }, "vgwRoutePropagation"},
		{func() bool { return len(b.fastLaunchImages) == 0 }, "fastLaunchImages"},
		{func() bool { return len(b.fastSnapshotRestores) == 0 }, "fastSnapshotRestores"},
		{func() bool { return b.spotDatafeed == nil }, "spotDatafeed"},
		{func() bool { return len(b.fleetHistory) == 0 }, "fleetHistory"},
		{func() bool { return len(b.spotFleetHistory) == 0 }, "spotFleetHistory"},
		{func() bool { return b.snapshotBlockPublicAccess == "" }, "snapshotBlockPublicAccess"},
		{func() bool { return b.ebsDefaultKmsKeyID == "" }, "ebsDefaultKmsKeyID"},
		{func() bool { return b.imageBlockPublicAccess == "" }, "imageBlockPublicAccess"},
		{func() bool { return b.defaultCreditSpec == "" }, "defaultCreditSpec"},
		{func() bool { return !b.ebsEncryptionByDefault }, "ebsEncryptionByDefault"},
		{func() bool { return !b.serialConsoleAccess }, "serialConsoleAccess"},
	}

	for _, f := range fields {
		require.False(t, f.isEmpty(), "setup failed to populate %s -- test cannot prove anything about it", f.name)
	}

	b.Reset()

	for _, f := range fields {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			assert.True(t, f.isEmpty(), "Reset did not clear %s", f.name)
		})
	}
}
