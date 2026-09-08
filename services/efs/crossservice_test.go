package efs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/efs"
)

// fakeEFSEC2Resolver is a test double for efs.EC2Resolver backed by static
// per-subnet VPC/AZ maps, standing in for the real services/ec2 backend
// cli.go wires in (see wireEFSCrossService in cli.go).
type fakeEFSEC2Resolver struct {
	vpc map[string]string
	az  map[string]string
}

func (f *fakeEFSEC2Resolver) SubnetExists(id string) bool {
	_, ok := f.vpc[id]

	return ok
}

func (f *fakeEFSEC2Resolver) SubnetVPC(id string) string { return f.vpc[id] }
func (f *fakeEFSEC2Resolver) SubnetAZ(id string) string  { return f.az[id] }

// TestCreateMountTarget_EC2Resolver_Placement verifies CreateMountTarget
// enforces api_op_CreateMountTarget.go's documented placement rule against a
// wired EC2Resolver: "Must belong to the same VPC as the subnets of the
// existing mount targets" and "Must not be in the same Availability Zone as
// any of the subnets of the existing mount targets" -- and stays permissive
// when no resolver is wired.
func TestCreateMountTarget_EC2Resolver_Placement(t *testing.T) {
	t.Parallel()

	resolver := &fakeEFSEC2Resolver{
		vpc: map[string]string{
			"subnet-a": "vpc-1", "subnet-b": "vpc-1", "subnet-c": "vpc-1", "subnet-d": "vpc-2",
		},
		az: map[string]string{
			"subnet-a": "us-east-1a", "subnet-b": "us-east-1b",
			"subnet-c": "us-east-1a", "subnet-d": "us-east-1b",
		},
	}

	tests := []struct {
		wantErr      error
		name         string
		secondSubnet string
		wireResolver bool
	}{
		{
			name:         "no_resolver_wired_accepts_conflicting_subnet",
			secondSubnet: "subnet-c",
			wireResolver: false,
			wantErr:      nil,
		},
		{
			name:         "different_az_same_vpc_allowed",
			secondSubnet: "subnet-b",
			wireResolver: true,
			wantErr:      nil,
		},
		{
			name:         "same_az_rejected",
			secondSubnet: "subnet-c",
			wireResolver: true,
			wantErr:      efs.ErrMountTargetConflict,
		},
		{
			name:         "different_vpc_rejected",
			secondSubnet: "subnet-d",
			wireResolver: true,
			wantErr:      efs.ErrMountTargetConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			if tt.wireResolver {
				b.SetEC2Resolver(resolver)
			}

			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-mt-placement-"+tt.name))
			require.NoError(t, err)

			_, err = b.CreateMountTarget(context.Background(), mtReq(fs.FileSystemID, "subnet-a"))
			require.NoError(t, err)

			_, err = b.CreateMountTarget(context.Background(), mtReq(fs.FileSystemID, tt.secondSubnet))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateMountTarget_EC2Resolver_UnknownSubnet verifies CreateMountTarget
// rejects a SubnetId the wired EC2Resolver doesn't recognize (efs@v1.44.4
// types/errors.go SubnetNotFound: "Returned if there is no subnet with ID
// SubnetId provided in the request"), and stays permissive when no resolver
// is wired.
func TestCreateMountTarget_EC2Resolver_UnknownSubnet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr      error
		name         string
		wireResolver bool
	}{
		{name: "no_resolver_wired_accepts_unknown_subnet", wireResolver: false, wantErr: nil},
		{name: "resolver_wired_rejects_unknown_subnet", wireResolver: true, wantErr: efs.ErrSubnetNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			if tt.wireResolver {
				b.SetEC2Resolver(&fakeEFSEC2Resolver{vpc: map[string]string{}, az: map[string]string{}})
			}

			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-mt-unknown-"+tt.name))
			require.NoError(t, err)

			_, err = b.CreateMountTarget(context.Background(), mtReq(fs.FileSystemID, "subnet-ghost"))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
