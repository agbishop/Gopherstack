package lightsail_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lightsailsdk "github.com/aws/aws-sdk-go-v2/service/lightsail"
	lightsailtypes "github.com/aws/aws-sdk-go-v2/service/lightsail/types"
	"github.com/stretchr/testify/require"
)

// TestAttachInstancesToLoadBalancer_RejectsStoppedInstance proves
// AttachInstancesToLoadBalancer enforces api_op_AttachInstancesToLoadBalancer.go's
// InstanceNames doc: "An instance must be running before you can attach it
// to your load balancer." It also proves a rejected attach leaves the load
// balancer untouched (no partial attachment).
func TestAttachInstancesToLoadBalancer_RejectsStoppedInstance(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	ctx := t.Context()

	_, err := client.CreateInstances(ctx, &lightsailsdk.CreateInstancesInput{
		InstanceNames: []string{"lb-stopped-target"}, AvailabilityZone: aws.String("us-east-1a"),
		BlueprintId: aws.String("amazon_linux_2023"), BundleId: aws.String("nano_3_0"),
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		out, getErr := client.GetInstanceState(
			ctx,
			&lightsailsdk.GetInstanceStateInput{InstanceName: aws.String("lb-stopped-target")},
		)

		return getErr == nil && aws.ToString(out.State.Name) == "running"
	}, defaultAsyncWait, defaultAsyncPoll, "instance never reached running")

	_, err = client.StopInstance(ctx, &lightsailsdk.StopInstanceInput{InstanceName: aws.String("lb-stopped-target")})
	require.NoError(t, err)

	_, err = client.CreateLoadBalancer(ctx, &lightsailsdk.CreateLoadBalancerInput{
		LoadBalancerName: aws.String("lb-reject-stopped"), InstancePort: 80,
	})
	require.NoError(t, err)

	_, err = client.AttachInstancesToLoadBalancer(ctx, &lightsailsdk.AttachInstancesToLoadBalancerInput{
		LoadBalancerName: aws.String("lb-reject-stopped"), InstanceNames: []string{"lb-stopped-target"},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "must be running")

	lbOut, err := client.GetLoadBalancer(
		ctx,
		&lightsailsdk.GetLoadBalancerInput{LoadBalancerName: aws.String("lb-reject-stopped")},
	)
	require.NoError(t, err)
	require.Empty(t, lbOut.LoadBalancer.InstanceHealthSummary, "rejected attach must not leave a partial attachment")
}

// TestDeleteInstance_DetachesAttachedDisk proves DeleteInstance detaches any
// disk still attached to the instance being deleted. Instance names are
// freed and reusable on delete (unregisterNameLocked), so a disk left
// pointing at a deleted instance's name would silently attach itself to
// whatever unrelated instance is next created under that name, and would
// also become permanently undeletable (DeleteDisk refuses an attached disk,
// api_op_DeleteDisk.go: "The disk must be in the available state (not
// attached to a Lightsail instance).").
func TestDeleteInstance_DetachesAttachedDisk(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	ctx := t.Context()

	_, err := client.CreateInstances(ctx, &lightsailsdk.CreateInstancesInput{
		InstanceNames: []string{"host-del-disk"}, AvailabilityZone: aws.String("us-east-1a"),
		BlueprintId: aws.String("amazon_linux_2023"), BundleId: aws.String("nano_3_0"),
	})
	require.NoError(t, err)

	_, err = client.CreateDisk(ctx, &lightsailsdk.CreateDiskInput{
		DiskName: aws.String("disk-del-instance"), AvailabilityZone: aws.String("us-east-1a"), SizeInGb: aws.Int32(16),
	})
	require.NoError(t, err)

	_, err = client.AttachDisk(ctx, &lightsailsdk.AttachDiskInput{
		DiskName: aws.String("disk-del-instance"), InstanceName: aws.String("host-del-disk"),
		DiskPath: aws.String("/dev/xvdf"),
	})
	require.NoError(t, err)

	_, err = client.DeleteInstance(
		ctx,
		&lightsailsdk.DeleteInstanceInput{InstanceName: aws.String("host-del-disk")},
	)
	require.NoError(t, err)

	diskOut, err := client.GetDisk(ctx, &lightsailsdk.GetDiskInput{DiskName: aws.String("disk-del-instance")})
	require.NoError(t, err)
	require.False(t, aws.ToBool(diskOut.Disk.IsAttached), "disk must be detached once its instance is deleted")
	require.Empty(t, aws.ToString(diskOut.Disk.AttachedTo))
	require.Equal(t, lightsailtypes.DiskStateAvailable, diskOut.Disk.State)

	_, err = client.DeleteDisk(ctx, &lightsailsdk.DeleteDiskInput{DiskName: aws.String("disk-del-instance")})
	require.NoError(t, err, "a disk freed by its instance's deletion must itself be deletable")
}

// TestDeleteInstance_ReleasesAttachedStaticIP proves DeleteInstance detaches
// any static IP still attached to the instance being deleted, for the same
// stale-attachment/ghost-row reason as TestDeleteInstance_DetachesAttachedDisk.
func TestDeleteInstance_ReleasesAttachedStaticIP(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	ctx := t.Context()

	_, err := client.CreateInstances(ctx, &lightsailsdk.CreateInstancesInput{
		InstanceNames: []string{"host-del-ip"}, AvailabilityZone: aws.String("us-east-1a"),
		BlueprintId: aws.String("amazon_linux_2023"), BundleId: aws.String("nano_3_0"),
	})
	require.NoError(t, err)

	_, err = client.AllocateStaticIp(
		ctx,
		&lightsailsdk.AllocateStaticIpInput{StaticIpName: aws.String("ip-del-instance")},
	)
	require.NoError(t, err)

	_, err = client.AttachStaticIp(ctx, &lightsailsdk.AttachStaticIpInput{
		StaticIpName: aws.String("ip-del-instance"), InstanceName: aws.String("host-del-ip"),
	})
	require.NoError(t, err)

	_, err = client.DeleteInstance(ctx, &lightsailsdk.DeleteInstanceInput{InstanceName: aws.String("host-del-ip")})
	require.NoError(t, err)

	ipOut, err := client.GetStaticIp(ctx, &lightsailsdk.GetStaticIpInput{StaticIpName: aws.String("ip-del-instance")})
	require.NoError(t, err)
	require.False(t, aws.ToBool(ipOut.StaticIp.IsAttached), "static IP must be detached once its instance is deleted")
	require.Empty(t, aws.ToString(ipOut.StaticIp.AttachedTo))
}

// TestGetInstance_HardwareDisksReflectsAttachment proves GetInstance's
// Hardware.Disks (types.InstanceHardware: "The disks attached to the
// instance") is populated from AttachDisk/DetachDisk on the disk side,
// rather than being permanently empty regardless of attachment.
func TestGetInstance_HardwareDisksReflectsAttachment(t *testing.T) {
	t.Parallel()

	client := newTestClient(t)
	ctx := t.Context()

	_, err := client.CreateInstances(ctx, &lightsailsdk.CreateInstancesInput{
		InstanceNames: []string{"host-hw-disks"}, AvailabilityZone: aws.String("us-east-1a"),
		BlueprintId: aws.String("amazon_linux_2023"), BundleId: aws.String("nano_3_0"),
	})
	require.NoError(t, err)

	_, err = client.CreateDisk(ctx, &lightsailsdk.CreateDiskInput{
		DiskName: aws.String("disk-hw"), AvailabilityZone: aws.String("us-east-1a"), SizeInGb: aws.Int32(16),
	})
	require.NoError(t, err)

	beforeOut, err := client.GetInstance(ctx, &lightsailsdk.GetInstanceInput{InstanceName: aws.String("host-hw-disks")})
	require.NoError(t, err)
	require.Empty(t, beforeOut.Instance.Hardware.Disks)

	_, err = client.AttachDisk(ctx, &lightsailsdk.AttachDiskInput{
		DiskName: aws.String("disk-hw"), InstanceName: aws.String("host-hw-disks"), DiskPath: aws.String("/dev/xvdf"),
	})
	require.NoError(t, err)

	afterOut, err := client.GetInstance(ctx, &lightsailsdk.GetInstanceInput{InstanceName: aws.String("host-hw-disks")})
	require.NoError(t, err)
	require.Len(t, afterOut.Instance.Hardware.Disks, 1)
	require.Equal(t, "disk-hw", aws.ToString(afterOut.Instance.Hardware.Disks[0].Name))
	require.Equal(t, "/dev/xvdf", aws.ToString(afterOut.Instance.Hardware.Disks[0].Path))

	_, err = client.DetachDisk(ctx, &lightsailsdk.DetachDiskInput{DiskName: aws.String("disk-hw")})
	require.NoError(t, err)

	detachedOut, err := client.GetInstance(
		ctx,
		&lightsailsdk.GetInstanceInput{InstanceName: aws.String("host-hw-disks")},
	)
	require.NoError(t, err)
	require.Empty(t, detachedOut.Instance.Hardware.Disks)
}
