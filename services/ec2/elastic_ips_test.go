package ec2_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ec2"
)

func TestMoveAddressToVpcAndDescribeMovingAddresses(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	addr, err := b.AllocateAddress()
	require.NoError(t, err)

	moved, err := b.MoveAddressToVpc(addr.PublicIP)
	require.NoError(t, err)
	assert.Equal(t, addr.AllocationID, moved.AllocationID)

	statuses := b.DescribeMovingAddresses(nil)
	require.Len(t, statuses, 1)
	assert.Equal(t, addr.PublicIP, statuses[0].PublicIP)
	assert.Equal(t, "movingToVpc", statuses[0].MoveStatus)

	filtered := b.DescribeMovingAddresses([]string{addr.PublicIP})
	require.Len(t, filtered, 1)

	none := b.DescribeMovingAddresses([]string{"1.2.3.4"})
	assert.Empty(t, none)

	_, err = b.MoveAddressToVpc("9.9.9.9")
	require.ErrorIs(t, err, ec2.ErrPublicIPNotFound)
}

func TestElasticIPOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		op      string
		wantErr bool
	}{
		{name: "allocate", op: "allocate", wantErr: false},
		{name: "describe_all", op: "describe_all", wantErr: false},
		{name: "associate_disassociate", op: "associate_disassociate", wantErr: false},
		{name: "release", op: "release", wantErr: false},
		{name: "release_nonexistent", op: "release_nonexistent", wantErr: true},
		{name: "associate_bad_alloc", op: "associate_bad_alloc", wantErr: true},
		{name: "associate_bad_instance", op: "associate_bad_instance", wantErr: true},
		{name: "disassociate_nonexistent", op: "disassociate_nonexistent", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			switch tt.op {
			case "allocate":
				addr, err := b.AllocateAddress()
				require.NoError(t, err)
				assert.NotEmpty(t, addr.AllocationID)
				assert.NotEmpty(t, addr.PublicIP)

			case "describe_all":
				_, err := b.AllocateAddress()
				require.NoError(t, err)
				addrs := b.DescribeAddresses(nil)
				assert.NotEmpty(t, addrs)

			case "associate_disassociate":
				addr, err := b.AllocateAddress()
				require.NoError(t, err)
				instances, err := b.RunInstances("ami-123", "t2.micro", "", 1)
				require.NoError(t, err)
				assocID, err := b.AssociateAddress(addr.AllocationID, instances[0].ID)
				require.NoError(t, err)
				assert.NotEmpty(t, assocID)
				err = b.DisassociateAddress(assocID)
				require.NoError(t, err)

			case "release":
				addr, err := b.AllocateAddress()
				require.NoError(t, err)
				err = b.ReleaseAddress(addr.AllocationID)
				require.NoError(t, err)
				addrs := b.DescribeAddresses([]string{addr.AllocationID})
				assert.Empty(t, addrs)

			case "release_nonexistent":
				err := b.ReleaseAddress("eipalloc-nonexistent")
				require.Error(t, err)

			case "associate_bad_alloc":
				instances, err := b.RunInstances("ami-123", "t2.micro", "", 1)
				require.NoError(t, err)
				_, err = b.AssociateAddress("eipalloc-nonexistent", instances[0].ID)
				require.Error(t, err)

			case "associate_bad_instance":
				addr, err := b.AllocateAddress()
				require.NoError(t, err)
				_, err = b.AssociateAddress(addr.AllocationID, "i-nonexistent")
				require.Error(t, err)

			case "disassociate_nonexistent":
				err := b.DisassociateAddress("eipassoc-nonexistent")
				require.Error(t, err)
			}
		})
	}
}

// TestReleaseAddress_InUse verifies AWS's ReleaseAddress guard: an
// associated Elastic IP cannot be released until it is disassociated.
func TestReleaseAddress_InUse(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	addr, err := b.AllocateAddress()
	require.NoError(t, err)

	instances, err := b.RunInstances("ami-123", "t2.micro", "", 1)
	require.NoError(t, err)

	assocID, err := b.AssociateAddress(addr.AllocationID, instances[0].ID)
	require.NoError(t, err)

	err = b.ReleaseAddress(addr.AllocationID)
	require.ErrorIs(t, err, ec2.ErrAddressInUse)
	assert.NotEmpty(t, b.DescribeAddresses([]string{addr.AllocationID}),
		"address must survive a failed ReleaseAddress")

	require.NoError(t, b.DisassociateAddress(assocID))
	require.NoError(t, b.ReleaseAddress(addr.AllocationID))
}
