package efs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/efs"
)

// TestDescribeAccessPoints_UnknownFileSystemID_ReturnsNotFound locks a real
// AWS behavior: DescribeAccessPoints' own declared error set (efs@v1.44.4
// deserializers.go, awsRestjson1_deserializeOpErrorDescribeAccessPoints)
// includes FileSystemNotFound, so an unknown FileSystemId filter must raise
// it -- not silently return an empty list, which is indistinguishable from
// "this file system exists but has no access points".
func TestDescribeAccessPoints_UnknownFileSystemID_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	b := newTestEFSBackend()

	_, _, err := b.DescribeAccessPoints(context.Background(), "fs-does-not-exist", "", "", 0)
	require.ErrorIs(t, err, efs.ErrNotFound)
}

// TestAccessPointPosixUser verifies PosixUser is stored and returned.
func TestAccessPointPosixUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		posixUser *efs.PosixUser
		name      string
	}{
		{
			name: "posix_user_persisted",
			posixUser: &efs.PosixUser{
				UID:           1000,
				GID:           1001,
				SecondaryGids: []int64{2000, 2001},
			},
		},
		{
			name:      "no_posix_user_ok",
			posixUser: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-ap-posix-"+tt.name))
			require.NoError(t, err)

			ap, err := b.CreateAccessPoint(context.Background(), efs.CreateAccessPointRequest{
				FileSystemID: fs.FileSystemID,
				PosixUser:    tt.posixUser,
			})
			require.NoError(t, err)

			if tt.posixUser != nil {
				require.NotNil(t, ap.PosixUser)
				assert.Equal(t, tt.posixUser.UID, ap.PosixUser.UID)
				assert.Equal(t, tt.posixUser.GID, ap.PosixUser.GID)
				assert.Equal(t, tt.posixUser.SecondaryGids, ap.PosixUser.SecondaryGids)
			} else {
				assert.Nil(t, ap.PosixUser)
			}
		})
	}
}

// TestAccessPointRootDirectory verifies RootDirectory is stored and validated.
//
// wantErrIs was efs.ErrValidation ("ValidationException") until this pass;
// CreateAccessPoint declares BadRequest ("Returned if the request is malformed
// or contains an error such as an invalid parameter value or a missing required
// parameter"), never ValidationException (efs@v1.44.4 deserializers.go/
// types/errors.go) -- the old assertion locked in the exact wire-code defect
// this pass fixed.
func TestAccessPointRootDirectory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs     error
		rootDirectory *efs.RootDirectory
		name          string
		wantErr       bool
	}{
		{
			name: "root_path_no_creation_info_ok",
			rootDirectory: &efs.RootDirectory{
				Path: "/",
			},
		},
		{
			name: "non_root_path_with_creation_info_ok",
			rootDirectory: &efs.RootDirectory{
				Path: "/data",
				CreationInfo: &efs.CreationInfo{
					OwnerUID:    1000,
					OwnerGID:    1001,
					Permissions: "0755",
				},
			},
		},
		{
			name: "non_root_path_without_creation_info_invalid",
			rootDirectory: &efs.RootDirectory{
				Path: "/data",
			},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
		{
			name:          "nil_root_directory_ok",
			rootDirectory: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-ap-rd-"+tt.name))
			require.NoError(t, err)

			_, err = b.CreateAccessPoint(context.Background(), efs.CreateAccessPointRequest{
				FileSystemID:  fs.FileSystemID,
				RootDirectory: tt.rootDirectory,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, tt.wantErrIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestAccessPointClientTokenIdempotency verifies ClientToken deduplication.
func TestAccessPointClientTokenIdempotency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		clientToken string
		wantSameAP  bool
	}{
		{
			name:        "same_client_token_returns_existing",
			clientToken: "ct-abc123",
			wantSameAP:  true,
		},
		{
			name:        "no_client_token_creates_new",
			clientToken: "",
			wantSameAP:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-ap-ct-"+tt.name))
			require.NoError(t, err)

			ap1, err := b.CreateAccessPoint(context.Background(), efs.CreateAccessPointRequest{
				FileSystemID: fs.FileSystemID,
				ClientToken:  tt.clientToken,
			})
			require.NoError(t, err)

			ap2, err := b.CreateAccessPoint(context.Background(), efs.CreateAccessPointRequest{
				FileSystemID: fs.FileSystemID,
				ClientToken:  tt.clientToken,
			})
			require.NoError(t, err)

			if tt.wantSameAP {
				assert.Equal(t, ap1.AccessPointID, ap2.AccessPointID)
				// Only one AP should exist.
				var aps []*efs.AccessPoint
				aps, _, err = b.DescribeAccessPoints(
					context.Background(),
					fs.FileSystemID,
					"",
					"",
					0,
				)
				require.NoError(t, err)
				assert.Len(t, aps, 1)
			} else {
				assert.NotEqual(t, ap1.AccessPointID, ap2.AccessPointID)
			}
		})
	}
}

// TestDescribeAccessPoints_Pagination verifies MaxResults/NextToken pagination.
func TestDescribeAccessPoints_Pagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		numAPs    int
		maxItems  int
		wantFirst int
		wantNext  bool
	}{
		{
			name:      "single_page",
			numAPs:    3,
			maxItems:  10,
			wantFirst: 3,
			wantNext:  false,
		},
		{
			name:      "two_pages",
			numAPs:    5,
			maxItems:  3,
			wantFirst: 3,
			wantNext:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-ap-page-"+tt.name))
			require.NoError(t, err)

			for range tt.numAPs {
				_, apErr := b.CreateAccessPoint(context.Background(), apReq(fs.FileSystemID))
				require.NoError(t, apErr)
			}

			list, nextToken, err := b.DescribeAccessPoints(
				context.Background(),
				fs.FileSystemID,
				"",
				"",
				tt.maxItems,
			)
			require.NoError(t, err)
			assert.Len(t, list, tt.wantFirst)

			if tt.wantNext {
				assert.NotEmpty(t, nextToken)

				// Confirm the second page carries every remaining item (no items
				// lost at the page boundary) and none of the first page's items.
				list2, _, err2 := b.DescribeAccessPoints(
					context.Background(), fs.FileSystemID, "", nextToken, tt.maxItems,
				)
				require.NoError(t, err2)
				assert.Len(t, list2, tt.numAPs-tt.wantFirst)

				seen := make(map[string]bool, tt.numAPs)
				for _, ap := range list {
					seen[ap.AccessPointID] = true
				}
				for _, ap := range list2 {
					assert.False(t, seen[ap.AccessPointID], "access point %s duplicated across pages", ap.AccessPointID)
					seen[ap.AccessPointID] = true
				}
				assert.Len(t, seen, tt.numAPs, "union of both pages must equal every created access point")
			} else {
				assert.Empty(t, nextToken)
			}
		})
	}
}

// TestCreateAccessPoint_RequiresAvailableFileSystem verifies CreateAccessPoint
// rejects requests while the file system's lifecycle state is not "available"
// (efs@v1.44.4 types/errors.go: IncorrectFileSystemLifeCycleState, "Returned if
// the file system's lifecycle state is not \"available\"").
func TestCreateAccessPoint_RequiresAvailableFileSystem(t *testing.T) {
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

			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-ap-lifecycle-"+tt.name))
			require.NoError(t, err)

			_, err = b.CreateAccessPoint(context.Background(), apReq(fs.FileSystemID))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
