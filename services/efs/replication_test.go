package efs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/efs"
)

// TestDeleteReplication_ProtectionFlip verifies source protection resets to DISABLED.
func TestDeleteReplication_ProtectionFlip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		wantProtection string
	}{
		{
			name:           "protection_resets_to_disabled_after_delete",
			wantProtection: "DISABLED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-repl-prot-"+tt.name))
			require.NoError(t, err)

			_, err = b.CreateReplicationConfiguration(
				context.Background(),
				fs.FileSystemID,
				[]efs.ReplicationDestination{
					{Region: "us-west-2", Status: "ENABLED"},
				},
			)
			require.NoError(t, err)

			// After create, source should be REPLICATING.
			list, _, err := b.DescribeFileSystems(context.Background(), fs.FileSystemID, "", "", 0)
			require.NoError(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, "REPLICATING", list[0].ReplicationOverwriteProtection)

			// Delete the replication config.
			err = b.DeleteReplicationConfiguration(context.Background(), fs.FileSystemID)
			require.NoError(t, err)

			// Source protection should revert.
			list2, _, err := b.DescribeFileSystems(context.Background(), fs.FileSystemID, "", "", 0)
			require.NoError(t, err)
			require.Len(t, list2, 1)
			assert.Equal(t, tt.wantProtection, list2[0].ReplicationOverwriteProtection)
		})
	}
}

// TestCreateReplicationConfiguration_RequiresAvailableFileSystem verifies
// CreateReplicationConfiguration rejects requests while the source file
// system's lifecycle state is not "available" (efs@v1.44.4 types/errors.go:
// IncorrectFileSystemLifeCycleState, "Returned if the file system's lifecycle
// state is not \"available\"").
func TestCreateReplicationConfiguration_RequiresAvailableFileSystem(t *testing.T) {
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

			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-crc-lifecycle-"+tt.name))
			require.NoError(t, err)

			_, err = b.CreateReplicationConfiguration(
				context.Background(),
				fs.FileSystemID,
				[]efs.ReplicationDestination{{Region: "us-west-2"}},
			)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestUpdateFileSystemProtection_RequiresAvailableFileSystem verifies
// UpdateFileSystemProtection rejects requests while the file system's
// lifecycle state is not "available" (efs@v1.44.4 types/errors.go:
// IncorrectFileSystemLifeCycleState, "Returned if the file system's lifecycle
// state is not \"available\"").
func TestUpdateFileSystemProtection_RequiresAvailableFileSystem(t *testing.T) {
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

			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-ufp-lifecycle-"+tt.name))
			require.NoError(t, err)

			err = b.UpdateFileSystemProtection(context.Background(), fs.FileSystemID, "ENABLED")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
