package ec2_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ec2"
)

// TestDeregisterImage_ClearsImageAttribute verifies that deregistering an AMI
// clears any attribute set via ModifyImageAttribute -- otherwise
// GetImageAttribute (which never validates the image still exists) keeps
// answering with the deregistered AMI's stale data forever.
func TestDeregisterImage_ClearsImageAttribute(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	img, err := b.RegisterImage("ghost-ami", "d", "x86_64")
	require.NoError(t, err)
	require.NoError(t, b.ModifyImageAttribute(img.ImageID, "description", "sensitive"))
	require.Equal(t, "sensitive", b.GetImageAttribute(img.ImageID, "description"))

	require.NoError(t, b.DeregisterImage(img.ImageID))

	assert.Empty(t, b.GetImageAttribute(img.ImageID, "description"),
		"a deregistered AMI's attributes must not remain readable")
}

// TestDeregisterImage_ClearsFastLaunch verifies that deregistering an AMI
// removes it from DescribeFastLaunchImages, which scans the whole
// fastLaunchImages map with no check that the AMI still exists.
func TestDeregisterImage_ClearsFastLaunch(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	img, err := b.RegisterImage("ghost-ami-fl", "d", "x86_64")
	require.NoError(t, err)
	require.NoError(t, b.EnableFastLaunch(img.ImageID, ec2.FastLaunchConfig{ResourceType: "launch-template"}))
	require.Len(t, b.DescribeFastLaunchImages(nil), 1)

	require.NoError(t, b.DeregisterImage(img.ImageID))

	for _, item := range b.DescribeFastLaunchImages(nil) {
		assert.NotEqual(t, img.ImageID, item.ImageID,
			"a deregistered AMI must not still report fast-launch state")
	}
}

// TestDeleteSnapshot_ClearsFastSnapshotRestores verifies that deleting a
// snapshot removes its fast-snapshot-restore entries, which
// DescribeFastSnapshotRestores lists by scanning the whole map with no check
// that the snapshot still exists.
func TestDeleteSnapshot_ClearsFastSnapshotRestores(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	vol, err := b.CreateVolume("us-east-1a", "gp3", 8, "")
	require.NoError(t, err)
	snap, err := b.CreateSnapshot(vol.ID, "d")
	require.NoError(t, err)
	require.NoError(t, b.EnableFastSnapshotRestores([]string{snap.SnapshotID}, []string{"us-east-1a"}))
	require.Len(t, b.DescribeFastSnapshotRestores(), 1)

	require.NoError(t, b.DeleteSnapshot(snap.SnapshotID))

	for _, item := range b.DescribeFastSnapshotRestores() {
		assert.NotEqual(t, snap.SnapshotID, item.SnapshotID,
			"a deleted snapshot must not still report fast-snapshot-restore state")
	}
}

// TestReleaseAddress_ClearsAddressTransfers verifies that releasing an
// Elastic IP removes any pending transfer, which DescribeAddressTransfers
// lists by scanning the whole map with no check that the address still
// exists.
func TestReleaseAddress_ClearsAddressTransfers(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	addr, err := b.AllocateAddress()
	require.NoError(t, err)
	_, err = b.EnableAddressTransfer(addr.AllocationID, "999999999999")
	require.NoError(t, err)
	require.Len(t, b.DescribeAddressTransfers(nil), 1)

	require.NoError(t, b.ReleaseAddress(addr.AllocationID))

	for _, item := range b.DescribeAddressTransfers(nil) {
		assert.NotEqual(t, addr.AllocationID, item.AllocationID,
			"a released address must not still report a pending transfer")
	}
}

// TestDeleteSecurityGroup_ClearsVpcAssociations verifies that deleting a
// security group removes its cross-VPC associations, which
// DescribeSecurityGroupVpcAssociations lists by scanning the whole map with
// no check that the security group still exists.
func TestDeleteSecurityGroup_ClearsVpcAssociations(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	vpc1, err := b.CreateVpc("10.0.0.0/16", "default")
	require.NoError(t, err)
	vpc2, err := b.CreateVpc("10.1.0.0/16", "default")
	require.NoError(t, err)
	sg, err := b.CreateSecurityGroup("ghost-sg", "d", vpc1.ID)
	require.NoError(t, err)
	_, err = b.AssociateSecurityGroupVpc(sg.ID, vpc2.ID)
	require.NoError(t, err)
	require.Len(t, b.DescribeSecurityGroupVpcAssociations(nil), 1)

	require.NoError(t, b.DeleteSecurityGroup(sg.ID))

	for _, item := range b.DescribeSecurityGroupVpcAssociations(nil) {
		assert.NotEqual(t, sg.ID, item.SGID,
			"a deleted security group must not still report VPC associations")
	}
}

// TestDeleteVpc_ClearsDefaultSecurityGroupVpcAssociations verifies that
// DeleteVpc's cascade -- which deletes the VPC's default security group
// directly rather than through DeleteSecurityGroup -- still performs that
// function's sgVpcAssociations cleanup, instead of bypassing it.
func TestDeleteVpc_ClearsDefaultSecurityGroupVpcAssociations(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	vpc1, err := b.CreateVpc("10.0.0.0/16", "default")
	require.NoError(t, err)
	vpc2, err := b.CreateVpc("10.1.0.0/16", "default")
	require.NoError(t, err)

	var defaultSGID string
	for _, sg := range b.DescribeSecurityGroups(nil) {
		if sg.VPCID == vpc1.ID && sg.Name == "default" {
			defaultSGID = sg.ID
		}
	}
	require.NotEmpty(t, defaultSGID, "CreateVpc must auto-create a default security group")

	_, err = b.AssociateSecurityGroupVpc(defaultSGID, vpc2.ID)
	require.NoError(t, err)
	require.Len(t, b.DescribeSecurityGroupVpcAssociations(nil), 1)

	require.NoError(t, b.DeleteVpc(vpc1.ID))

	for _, item := range b.DescribeSecurityGroupVpcAssociations(nil) {
		assert.NotEqual(t, defaultSGID, item.SGID,
			"a VPC's auto-deleted default security group must not still report VPC associations")
	}
}

// TestDeleteSubnet_ClearsCidrReservations verifies that deleting a subnet
// removes its CIDR reservations, since GetSubnetCidrReservations never
// checks that the subnet still exists.
func TestDeleteSubnet_ClearsCidrReservations(t *testing.T) {
	t.Parallel()

	b := ec2.NewInMemoryBackend("123456789012", "us-east-1")

	vpc, err := b.CreateVpc("10.0.0.0/16", "default")
	require.NoError(t, err)
	subnet, err := b.CreateSubnet(vpc.ID, "10.0.1.0/24", "us-east-1a")
	require.NoError(t, err)
	_, err = b.CreateSubnetCidrReservation(subnet.ID, "10.0.1.0/28", "prefix", "d")
	require.NoError(t, err)

	reservations, err := b.GetSubnetCidrReservations(subnet.ID)
	require.NoError(t, err)
	require.Len(t, reservations, 1)

	require.NoError(t, b.DeleteSubnet(subnet.ID))

	reservations, err = b.GetSubnetCidrReservations(subnet.ID)
	require.NoError(t, err)
	assert.Empty(t, reservations, "a deleted subnet must not still report CIDR reservations")
}
