package azureblob

import "errors"

// Sentinel errors for Azure Blob Storage operations.
var (
	ErrContainerNotFound      = errors.New("azureblob: container not found")
	ErrContainerAlreadyExists = errors.New("azureblob: container already exists")
	ErrBlobNotFound           = errors.New("azureblob: blob not found")

	// ErrSnapshotContainerNull and ErrSnapshotBlobNull are returned by
	// Restore when a snapshot's "containers" map (or a container's "Blobs"
	// map) holds a JSON null entry, which decodes to a nil pointer that
	// would panic on first dereference if stored as-is. See persistence.go.
	ErrSnapshotContainerNull = errors.New("azureblob: restore snapshot: container is null")
	ErrSnapshotBlobNull      = errors.New("azureblob: restore snapshot: blob is null")
)
