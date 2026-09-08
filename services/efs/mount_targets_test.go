package efs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/efs"
)

// TestDescribeMountTargets_Pagination verifies MaxItems/Marker pagination.
func TestDescribeMountTargets_Pagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		numMTs    int
		maxItems  int
		wantFirst int
		wantNext  bool
	}{
		{
			name:      "fits_in_one_page",
			numMTs:    2,
			maxItems:  5,
			wantFirst: 2,
			wantNext:  false,
		},
		{
			name:      "spans_multiple_pages",
			numMTs:    4,
			maxItems:  2,
			wantFirst: 2,
			wantNext:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-mt-page-"+tt.name))
			require.NoError(t, err)

			for i := range tt.numMTs {
				_, mtErr := b.CreateMountTarget(
					context.Background(),
					mtReq(fs.FileSystemID, "sn-"+string(rune('a'+i))),
				)
				require.NoError(t, mtErr)
			}

			list, nextMarker, err := b.DescribeMountTargets(
				context.Background(),
				fs.FileSystemID,
				"",
				"",
				tt.maxItems,
			)
			require.NoError(t, err)
			assert.Len(t, list, tt.wantFirst)

			if tt.wantNext {
				assert.NotEmpty(t, nextMarker)

				// Confirm the second page carries every remaining item (no items
				// lost at the page boundary) and none of the first page's items.
				list2, _, err2 := b.DescribeMountTargets(
					context.Background(), fs.FileSystemID, "", nextMarker, tt.maxItems,
				)
				require.NoError(t, err2)
				assert.Len(t, list2, tt.numMTs-tt.wantFirst)

				seen := make(map[string]bool, tt.numMTs)
				for _, mt := range list {
					seen[mt.MountTargetID] = true
				}
				for _, mt := range list2 {
					assert.False(t, seen[mt.MountTargetID], "mount target %s duplicated across pages", mt.MountTargetID)
					seen[mt.MountTargetID] = true
				}
				assert.Len(t, seen, tt.numMTs, "union of both pages must equal every created mount target")
			} else {
				assert.Empty(t, nextMarker)
			}
		})
	}
}

// TestDescribeMountTargets_UnknownFileSystemID_ReturnsNotFound locks a real
// AWS behavior: DescribeMountTargets' own declared error set (efs@v1.44.4
// deserializers.go, awsRestjson1_deserializeOpErrorDescribeMountTargets)
// includes FileSystemNotFound, so an unknown FileSystemId filter must raise
// it -- not silently return an empty list, which is indistinguishable from
// "this file system exists but has no mount targets".
func TestDescribeMountTargets_UnknownFileSystemID_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	b := newTestEFSBackend()

	_, _, err := b.DescribeMountTargets(context.Background(), "fs-does-not-exist", "", "", 0)
	require.ErrorIs(t, err, efs.ErrNotFound)
}

// TestDescribeMountTargets_AccessPointIdFilter verifies the backend
// DescribeAccessPoints can be used to resolve an access point to its file system,
// enabling the AccessPointId filter in the handler layer.
func TestDescribeMountTargets_AccessPointIdFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		apExists bool
	}{
		{name: "existing_access_point_resolves_to_fs", apExists: true},
		{name: "missing_access_point_returns_not_found", apExists: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, createErr := b.CreateFileSystem(context.Background(), fsReq("ap-resolve-"+tt.name))
			require.NoError(t, createErr)

			if tt.apExists {
				ap, apErr := b.CreateAccessPoint(context.Background(), apReq(fs.FileSystemID))
				require.NoError(t, apErr)

				// Access point should resolve to the same file system.
				aps, _, descErr := b.DescribeAccessPoints(context.Background(), "", ap.AccessPointID, "", 1)
				require.NoError(t, descErr)
				require.Len(t, aps, 1)
				assert.Equal(t, fs.FileSystemID, aps[0].FileSystemID)
			} else {
				_, _, descErr := b.DescribeAccessPoints(context.Background(), "", "fsap-missing", "", 1)
				require.ErrorIs(t, descErr, efs.ErrAccessPointNotFound)
			}
		})
	}
}

// TestCreateMountTarget_RequiresAvailableFileSystem verifies CreateMountTarget
// rejects requests while the file system's lifecycle state is not "available"
// (api_op_CreateMountTarget.go:29-30; botocore efs/service-2.json
// IncorrectFileSystemLifeCycleState: "Returned if the file system's lifecycle
// state is not \"available\"").
func TestCreateMountTarget_RequiresAvailableFileSystem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr       error
		name          string
		activateDelay time.Duration
	}{
		{name: "creating_state_rejected", activateDelay: time.Hour, wantErr: efs.ErrIncorrectFileSystemLifeCycleState},
		{name: "available_state_allowed", activateDelay: 0, wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			efs.SetFSActivationDelay(b, tt.activateDelay)

			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-mt-lifecycle-"+tt.name))
			require.NoError(t, err)

			_, err = b.CreateMountTarget(context.Background(), mtReq(fs.FileSystemID, "subnet-lifecycle"))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateMountTarget_OneZoneLimit verifies a One Zone file system can have only
// one mount target, while a standard (multi-AZ) file system can have more than one
// (api_op_CreateMountTarget.go:21-22: "You can create only one mount target for a
// One Zone file system.").
func TestCreateMountTarget_OneZoneLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantSecondErr        error
		name                 string
		availabilityZoneName string
	}{
		{
			name:                 "one_zone_second_target_rejected",
			availabilityZoneName: "us-east-1a",
			wantSecondErr:        efs.ErrMountTargetConflict,
		},
		{name: "multi_az_second_target_allowed", availabilityZoneName: "", wantSecondErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), efs.CreateFileSystemRequest{
				CreationToken:        "tok-mt-onezone-" + tt.name,
				AvailabilityZoneName: tt.availabilityZoneName,
			})
			require.NoError(t, err)

			_, err = b.CreateMountTarget(context.Background(), mtReq(fs.FileSystemID, "subnet-a"))
			require.NoError(t, err)

			_, err = b.CreateMountTarget(context.Background(), mtReq(fs.FileSystemID, "subnet-b"))
			if tt.wantSecondErr != nil {
				require.ErrorIs(t, err, tt.wantSecondErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
