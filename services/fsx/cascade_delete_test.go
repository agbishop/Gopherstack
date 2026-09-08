package fsx_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/fsx"
)

// TestFSx_DeleteFileSystem_CascadesToChildren verifies that DeleteFileSystem
// removes every child resource real AWS also tears down when a non-ONTAP
// file system is deleted: directly-attached volumes (e.g. the OpenZFS
// root/child volume), those volumes' snapshots, and data repository
// associations. ONTAP is excluded here -- see
// TestFSx_DeleteFileSystem_ONTAP_RejectedWithChildren -- because real AWS
// requires an ONTAP file system's SVMs and volumes to be deleted first
// rather than cascading them.
func TestFSx_DeleteFileSystem_CascadesToChildren(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := fsx.GetBackend(h)
	fsID := createFS(t, h, "OPENZFS")

	volRec := doFSxRequest(t, h, "CreateVolume", map[string]any{
		"VolumeType":           "OPENZFS",
		"Name":                 "vol1",
		"OpenZFSConfiguration": map[string]any{"ParentVolumeId": openZFSRootVolumeID(t, h, fsID)},
	})
	require.Equal(t, http.StatusOK, volRec.Code)
	volID := decodeField(t, volRec, "Volume")["VolumeId"].(string)

	snapRec := doFSxRequest(t, h, "CreateSnapshot", map[string]any{
		"VolumeId": volID,
		"Name":     "snap1",
	})
	require.Equal(t, http.StatusOK, snapRec.Code)

	draRec := doFSxRequest(t, h, "CreateDataRepositoryAssociation", map[string]any{
		"FileSystemId":       fsID,
		"FileSystemPath":     "/data",
		"DataRepositoryPath": "s3://bucket/data",
	})
	require.Equal(t, http.StatusOK, draRec.Code)

	require.Equal(t, 2, fsx.VolumeCount(b), "OpenZFS root volume + child volume")
	require.Equal(t, 1, fsx.SnapshotCount(b))
	require.Equal(t, 1, fsx.DRACount(b))

	delRec := doFSxRequest(t, h, "DeleteFileSystem", map[string]any{"FileSystemId": fsID})
	require.Equal(t, http.StatusOK, delRec.Code)

	assert.Equal(t, 0, fsx.FileSystemCount(b))
	assert.Equal(t, 0, fsx.VolumeCount(b), "volumes must be cascade-deleted")
	assert.Equal(t, 0, fsx.SnapshotCount(b), "snapshot must be cascade-deleted")
	assert.Equal(t, 0, fsx.DRACount(b), "data repository association must be cascade-deleted")
}

// TestFSx_DeleteFileSystem_ONTAP_RejectedWithChildren verifies that, unlike
// every other file system type, DeleteFileSystem on an ONTAP file system is
// rejected while it still has an SVM or volume attached (real AWS:
// "To delete an Amazon FSx for NetApp ONTAP file system, first delete all
// the volumes and storage virtual machines (SVMs) on the file system.") and
// succeeds once they are removed.
func TestFSx_DeleteFileSystem_ONTAP_RejectedWithChildren(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := fsx.GetBackend(h)
	fsID := createFS(t, h, "ONTAP")

	svmRec := doFSxRequest(t, h, "CreateStorageVirtualMachine", map[string]any{
		"FileSystemId": fsID,
		"Name":         "svm1",
	})
	require.Equal(t, http.StatusOK, svmRec.Code)
	svmID := decodeField(t, svmRec, "StorageVirtualMachine")["StorageVirtualMachineId"].(string)

	volRec := doFSxRequest(t, h, "CreateVolume", map[string]any{
		"VolumeType":         "ONTAP",
		"Name":               "vol1",
		"OntapConfiguration": map[string]any{"StorageVirtualMachineId": svmID},
	})
	require.Equal(t, http.StatusOK, volRec.Code)
	volID := decodeField(t, volRec, "Volume")["VolumeId"].(string)

	// Rejected while the volume exists.
	delRec := doFSxRequest(t, h, "DeleteFileSystem", map[string]any{"FileSystemId": fsID})
	require.Equal(t, http.StatusBadRequest, delRec.Code)
	assert.Equal(t, 1, fsx.FileSystemCount(b))

	delVolRec := doFSxRequest(t, h, "DeleteVolume", map[string]any{"VolumeId": volID})
	require.Equal(t, http.StatusOK, delVolRec.Code)

	// Still rejected while the (now empty) SVM exists.
	delRec = doFSxRequest(t, h, "DeleteFileSystem", map[string]any{"FileSystemId": fsID})
	require.Equal(t, http.StatusBadRequest, delRec.Code)

	delSVMRec := doFSxRequest(t, h, "DeleteStorageVirtualMachine",
		map[string]any{"StorageVirtualMachineId": svmID})
	require.Equal(t, http.StatusOK, delSVMRec.Code)

	// Succeeds once both are gone.
	delRec = doFSxRequest(t, h, "DeleteFileSystem", map[string]any{"FileSystemId": fsID})
	require.Equal(t, http.StatusOK, delRec.Code)
	assert.Equal(t, 0, fsx.FileSystemCount(b))
}

// TestFSx_DeleteFileSystem_DoesNotCascadeToBackups verifies that
// DeleteFileSystem leaves backups alone: real AWS FSx backups persist
// independently of the file system they were taken from.
func TestFSx_DeleteFileSystem_DoesNotCascadeToBackups(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := fsx.GetBackend(h)
	fsID := createFS(t, h, "LUSTRE")

	backupRec := doFSxRequest(t, h, "CreateBackup", map[string]any{"FileSystemId": fsID})
	require.Equal(t, http.StatusOK, backupRec.Code)

	delRec := doFSxRequest(t, h, "DeleteFileSystem", map[string]any{"FileSystemId": fsID})
	require.Equal(t, http.StatusOK, delRec.Code)

	assert.Equal(t, 1, fsx.BackupCount(b), "backups must survive file system deletion")
}

// TestFSx_DeleteVolume_CascadesToSnapshots verifies that DeleteVolume removes
// snapshots of that volume, so no ghost Snapshot row (pointing at a
// now-nonexistent VolumeId) survives.
func TestFSx_DeleteVolume_CascadesToSnapshots(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := fsx.GetBackend(h)
	fsID := createFS(t, h, "OPENZFS")

	volRec := doFSxRequest(t, h, "CreateVolume", map[string]any{
		"VolumeType":           "OPENZFS",
		"Name":                 "vol1",
		"OpenZFSConfiguration": map[string]any{"ParentVolumeId": openZFSRootVolumeID(t, h, fsID)},
	})
	require.Equal(t, http.StatusOK, volRec.Code)
	volID := decodeField(t, volRec, "Volume")["VolumeId"].(string)

	snapRec := doFSxRequest(t, h, "CreateSnapshot", map[string]any{
		"VolumeId": volID,
		"Name":     "snap1",
	})
	require.Equal(t, http.StatusOK, snapRec.Code)
	require.Equal(t, 1, fsx.SnapshotCount(b))

	delRec := doFSxRequest(t, h, "DeleteVolume", map[string]any{"VolumeId": volID})
	require.Equal(t, http.StatusOK, delRec.Code)

	assert.Equal(t, 0, fsx.SnapshotCount(b), "snapshot must be cascade-deleted")
}

// TestFSx_DeleteStorageVirtualMachine_RejectedWithVolumes verifies that
// DeleteStorageVirtualMachine refuses while it still hosts a volume (real
// AWS: "Prior to deleting an SVM, you must delete all non-root volumes in
// the SVM, otherwise the operation will fail.") and succeeds once the
// volume is removed.
func TestFSx_DeleteStorageVirtualMachine_RejectedWithVolumes(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := fsx.GetBackend(h)
	fsID := createFS(t, h, "ONTAP")

	svmRec := doFSxRequest(t, h, "CreateStorageVirtualMachine", map[string]any{
		"FileSystemId": fsID,
		"Name":         "svm1",
	})
	require.Equal(t, http.StatusOK, svmRec.Code)
	svmID := decodeField(t, svmRec, "StorageVirtualMachine")["StorageVirtualMachineId"].(string)

	volRec := doFSxRequest(t, h, "CreateVolume", map[string]any{
		"VolumeType":         "ONTAP",
		"Name":               "vol1",
		"OntapConfiguration": map[string]any{"StorageVirtualMachineId": svmID},
	})
	require.Equal(t, http.StatusOK, volRec.Code)
	volID := decodeField(t, volRec, "Volume")["VolumeId"].(string)
	require.Equal(t, 1, fsx.VolumeCount(b))

	// Rejected while the volume exists.
	delRec := doFSxRequest(t, h, "DeleteStorageVirtualMachine", map[string]any{"StorageVirtualMachineId": svmID})
	require.Equal(t, http.StatusBadRequest, delRec.Code)
	assert.Equal(t, 1, fsx.VolumeCount(b))

	delVolRec := doFSxRequest(t, h, "DeleteVolume", map[string]any{"VolumeId": volID})
	require.Equal(t, http.StatusOK, delVolRec.Code)

	// Succeeds once the volume is gone.
	delRec = doFSxRequest(t, h, "DeleteStorageVirtualMachine", map[string]any{"StorageVirtualMachineId": svmID})
	require.Equal(t, http.StatusOK, delRec.Code)
}
