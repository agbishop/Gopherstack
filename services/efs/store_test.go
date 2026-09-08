package efs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/efs"
)

// TestBackendRegion tests the Region method.
func TestBackendRegion(t *testing.T) {
	t.Parallel()

	backend := efs.NewInMemoryBackend("123456789012", "us-east-1")
	assert.Equal(t, "us-east-1", backend.Region())
}

// TestReset verifies that Reset() clears all backend state.
func TestReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(b *efs.InMemoryBackend)
		name   string
		wantFS int
		wantMT int
		wantAP int
	}{
		{
			name:  "empty_backend_stays_empty",
			setup: func(_ *efs.InMemoryBackend) {},
		},
		{
			name: "resets_file_systems",
			setup: func(b *efs.InMemoryBackend) {
				_, err := b.CreateFileSystem(context.Background(), fsReq("t1"))
				require.NoError(t, err)
				_, err = b.CreateFileSystem(context.Background(), fsReq("t2"))
				require.NoError(t, err)
			},
		},
		{
			name: "resets_mount_targets",
			setup: func(b *efs.InMemoryBackend) {
				fs, err := b.CreateFileSystem(context.Background(), fsReq("t1"))
				require.NoError(t, err)
				_, err = b.CreateMountTarget(
					context.Background(),
					mtReq(fs.FileSystemID, "subnet-1"),
				)
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			tt.setup(b)
			b.Reset()

			assert.Equal(t, 0, efs.FileSystemCount(b))
			assert.Equal(t, 0, efs.MountTargetCount(b))
			assert.Equal(t, 0, efs.AccessPointCount(b))
			assert.Equal(t, 0, efs.ARNIndexSize(b))
		})
	}
}

// TestReset_AccountPreferences verifies that Reset() restores account
// preferences to the LONG_ID default, not the SHORT_ID zero value.
func TestReset_AccountPreferences(t *testing.T) {
	t.Parallel()

	b := newTestEFSBackend()

	before := b.DescribeAccountPreferences()
	require.Equal(t, "LONG_ID", before.ResourceIDType)

	_, err := b.PutAccountPreferences("SHORT_ID")
	require.NoError(t, err)
	require.Equal(t, "SHORT_ID", b.DescribeAccountPreferences().ResourceIDType)

	b.Reset()

	after := b.DescribeAccountPreferences()
	assert.Equal(t, "LONG_ID", after.ResourceIDType, "reset must restore LONG_ID, not the zero value")
}

// TestARNIndexes verifies the ARN index is populated and enables O(1) lookup.
func TestARNIndexes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		perform     func(b *efs.InMemoryBackend) (string, error)
		name        string
		wantARNSize int
	}{
		{
			name: "file_system_arn_indexed",
			perform: func(b *efs.InMemoryBackend) (string, error) {
				fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-arn"))
				if err != nil {
					return "", err
				}

				return fs.FileSystemArn, nil
			},
			wantARNSize: 1,
		},
		{
			name: "mount_target_arn_indexed",
			perform: func(b *efs.InMemoryBackend) (string, error) {
				fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-mt-arn"))
				if err != nil {
					return "", err
				}
				mt, err := b.CreateMountTarget(context.Background(), mtReq(fs.FileSystemID, "sn-1"))
				if err != nil {
					return "", err
				}

				return mt.MountTargetArn, nil
			},
			wantARNSize: 2,
		},
		{
			name: "access_point_arn_indexed",
			perform: func(b *efs.InMemoryBackend) (string, error) {
				fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-ap-arn"))
				if err != nil {
					return "", err
				}
				ap, err := b.CreateAccessPoint(context.Background(), apReq(fs.FileSystemID))
				if err != nil {
					return "", err
				}

				return ap.AccessPointArn, nil
			},
			wantARNSize: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			arnVal, err := tt.perform(b)
			require.NoError(t, err)
			assert.NotEmpty(t, arnVal)
			assert.Equal(t, tt.wantARNSize, efs.ARNIndexSize(b))
		})
	}
}

// TestSeedHelpers verifies AddFileSystemInternal, AddMountTargetInternal, AddAccessPointInternal.
func TestSeedHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantFS    int
		wantMT    int
		wantAP    int
		wantARNsz int
	}{
		{
			name:      "seed_one_of_each",
			wantFS:    1,
			wantMT:    1,
			wantAP:    1,
			wantARNsz: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()

			b.AddFileSystemInternal(&efs.FileSystem{
				FileSystemID:  "fs-seed01",
				FileSystemArn: "arn:aws:elasticfilesystem:us-east-1:123456789012:file-system/fs-seed01",
			})
			b.AddMountTargetInternal(&efs.MountTarget{
				MountTargetID:  "fsmt-seed01",
				MountTargetArn: "arn:aws:elasticfilesystem:us-east-1:123456789012:mount-target/fsmt-seed01",
				FileSystemID:   "fs-seed01",
			})
			b.AddAccessPointInternal(&efs.AccessPoint{
				AccessPointID:  "fsap-seed01",
				AccessPointArn: "arn:aws:elasticfilesystem:us-east-1:123456789012:access-point/fsap-seed01",
				FileSystemID:   "fs-seed01",
			})

			assert.Equal(t, tt.wantFS, efs.FileSystemCount(b))
			assert.Equal(t, tt.wantMT, efs.MountTargetCount(b))
			assert.Equal(t, tt.wantAP, efs.AccessPointCount(b))
			assert.Equal(t, tt.wantARNsz, efs.ARNIndexSize(b))
		})
	}
}

// TestExportCountHelpers verifies that export count helpers return correct values.
func TestExportCountHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		wantFS           int
		wantMT           int
		wantAP           int
		wantRepl         int
		wantBackupPolicy int
		wantFSPolicy     int
	}{
		{
			name:             "all_counts_correct",
			wantFS:           1,
			wantMT:           1,
			wantAP:           1,
			wantRepl:         1,
			wantBackupPolicy: 1,
			wantFSPolicy:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-counts-"+tt.name))
			require.NoError(t, err)

			_, err = b.CreateMountTarget(context.Background(), mtReq(fs.FileSystemID, "sn-1"))
			require.NoError(t, err)

			_, err = b.CreateAccessPoint(context.Background(), apReq(fs.FileSystemID))
			require.NoError(t, err)

			_, err = b.CreateReplicationConfiguration(
				context.Background(),
				fs.FileSystemID,
				[]efs.ReplicationDestination{
					{Region: "us-west-2"},
				},
			)
			require.NoError(t, err)

			err = b.PutBackupPolicy(context.Background(), fs.FileSystemID, "ENABLED")
			require.NoError(t, err)

			err = b.PutFileSystemPolicy(
				context.Background(),
				fs.FileSystemID,
				`{"Version":"2012-10-17"}`,
			)
			require.NoError(t, err)

			assert.Equal(t, tt.wantFS, efs.FileSystemCount(b))
			assert.Equal(t, tt.wantMT, efs.MountTargetCount(b))
			assert.Equal(t, tt.wantAP, efs.AccessPointCount(b))
			assert.Equal(t, tt.wantRepl, efs.ReplicationConfigCount(b))
			assert.Equal(t, tt.wantBackupPolicy, efs.BackupPolicyCount(b))
			assert.Equal(t, tt.wantFSPolicy, efs.FileSystemPolicyCount(b))
		})
	}
}

// TestPersistenceRoundTrip verifies Snapshot/Restore round-trips all state via the
// exported package API. Test_InMemoryBackend_SnapshotRestore_FullState in
// persistence_test.go covers the same mechanism in more depth (two regions,
// every raw-left map) from inside the package; this test pins the same
// behaviour is reachable from outside the package.
func TestPersistenceRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		wantFS int
		wantMT int
		wantAP int
	}{
		{name: "roundtrip_preserves_counts", wantFS: 1, wantMT: 1, wantAP: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-persist-"+tt.name))
			require.NoError(t, err)

			_, err = b.CreateMountTarget(context.Background(), mtReq(fs.FileSystemID, "sn-1"))
			require.NoError(t, err)

			_, err = b.CreateAccessPoint(context.Background(), apReq(fs.FileSystemID))
			require.NoError(t, err)

			snap := b.Snapshot(t.Context())
			require.NotNil(t, snap)

			b2 := newTestEFSBackend()
			err = b2.Restore(t.Context(), snap)
			require.NoError(t, err)

			assert.Equal(t, tt.wantFS, efs.FileSystemCount(b2))
			assert.Equal(t, tt.wantMT, efs.MountTargetCount(b2))
			assert.Equal(t, tt.wantAP, efs.AccessPointCount(b2))
			// ARN indexes should be rebuilt.
			assert.Positive(t, efs.ARNIndexSize(b2))
		})
	}
}
