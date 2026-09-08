package fsx

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

const (
	errFileSystemNotFound = "FileSystemNotFound"
	errBackupNotFound     = "BackupNotFound"
	// errValidation is BadRequest, not "ValidationError": real FSx has no
	// ValidationError exception type (see types/errors.go in the SDK). Its
	// generic client-error shape is BadRequest; CreateFileSystem-specific
	// gaps use the more specific MissingFileSystemConfiguration below.
	errValidation = "BadRequest"
)

var (
	// ErrFileSystemNotFound is returned when a file system does not exist.
	ErrFileSystemNotFound = awserr.New(errFileSystemNotFound, awserr.ErrNotFound)
	// ErrBackupNotFound is returned when a backup does not exist.
	ErrBackupNotFound = awserr.New(errBackupNotFound, awserr.ErrConflict)
	// ErrValidation is returned on invalid input (wire code: BadRequest).
	ErrValidation = awserr.New(errValidation, awserr.ErrInvalidParameter)
	// ErrMissingFileSystemConfiguration is returned when CreateFileSystem is
	// called for WINDOWS/ONTAP/OPENZFS without the required per-type
	// configuration block (WindowsConfiguration/OntapConfiguration/
	// OpenZFSConfiguration).
	ErrMissingFileSystemConfiguration = awserr.New("MissingFileSystemConfiguration", awserr.ErrInvalidParameter)
	// ErrMissingVolumeConfiguration is returned when CreateVolume is called
	// without the required per-type configuration block (OntapConfiguration
	// for VolumeType=ONTAP, OpenZFSConfiguration for VolumeType=OPENZFS) --
	// real CreateVolumeInput has no top-level FileSystemId/
	// StorageVirtualMachineId at all (fsx@v1.68.4 api_op_CreateVolume.go);
	// types.MissingVolumeConfiguration ("A volume configuration is required
	// for this operation.") is the real wire code for the absent case.
	ErrMissingVolumeConfiguration = awserr.New("MissingVolumeConfiguration", awserr.ErrInvalidParameter)
	// ErrTagInvalid is returned when a tag key or value fails validation.
	ErrTagInvalid = awserr.New("BadRequest", awserr.ErrInvalidParameter)
	// ErrTagLimitExceeded is returned when the 50-tag-per-resource limit is exceeded.
	ErrTagLimitExceeded = awserr.New("ServiceLimitExceeded", awserr.ErrInvalidParameter)

	// ErrSnapshotNotFound is returned when a snapshot does not exist.
	ErrSnapshotNotFound = awserr.New("SnapshotNotFound", awserr.ErrNotFound)
	// ErrStorageVirtualMachineNotFound is returned when an SVM does not exist.
	ErrStorageVirtualMachineNotFound = awserr.New("StorageVirtualMachineNotFound", awserr.ErrNotFound)
	// ErrVolumeNotFound is returned when a volume does not exist.
	ErrVolumeNotFound = awserr.New("VolumeNotFound", awserr.ErrNotFound)
	// ErrFileCacheNotFound is returned when a file cache does not exist.
	ErrFileCacheNotFound = awserr.New("FileCacheNotFound", awserr.ErrNotFound)
	// ErrDataRepositoryAssociationNotFound is returned when a DRA does not exist.
	ErrDataRepositoryAssociationNotFound = awserr.New("DataRepositoryAssociationNotFound", awserr.ErrNotFound)
	// ErrDataRepositoryTaskNotFound is returned when a DRT does not exist.
	ErrDataRepositoryTaskNotFound = awserr.New("DataRepositoryTaskNotFound", awserr.ErrNotFound)
	// ErrS3AccessPointNotFound carries wire code InvalidRequest, a type
	// CreateAndAttachS3AccessPoint's own switch declares (fsx@v1.68.4
	// deserializers.go deserializeOpErrorCreateAndAttachS3AccessPoint) --
	// but no current backend code path emits this sentinel; kept in case a
	// future CreateAndAttachS3AccessPoint validation needs it
	// (gopherstack-6flj/uox6 error-envelope sweep found its only two
	// former callers, in DescribeS3AccessPointAttachments and
	// DetachAndDeleteS3AccessPoint, were wrong -- see
	// ErrS3AccessPointAttachmentNotFound).
	ErrS3AccessPointNotFound = awserr.New("InvalidRequest", awserr.ErrNotFound)
	// ErrS3AccessPointAttachmentNotFound is returned by
	// DescribeS3AccessPointAttachments/DetachAndDeleteS3AccessPoint when the
	// named attachment does not exist. Both ops' own switches declare
	// S3AccessPointAttachmentNotFound, not the InvalidRequest wire code
	// ErrS3AccessPointNotFound carries -- InvalidRequest only fits
	// CreateAndAttachS3AccessPoint's own declared set (gopherstack-6flj/uox6
	// error-envelope sweep).
	ErrS3AccessPointAttachmentNotFound = awserr.New("S3AccessPointAttachmentNotFound", awserr.ErrNotFound)
	// ErrResourceNotFound is returned by the generic Tag/Untag/ListTagsForResource
	// operations when the given ResourceARN does not match any known FSx resource.
	// Real FSx uses the generic ResourceNotFound exception here, distinct from the
	// resource-type-specific *NotFound exceptions used by Describe/Delete ops.
	ErrResourceNotFound = awserr.New("ResourceNotFound", awserr.ErrNotFound)
	// ErrIncompatibleParameter is returned by CreateFileSystem when a second
	// request is received with a previously-used ClientRequestToken but
	// different parameter settings.
	ErrIncompatibleParameter = awserr.New("IncompatibleParameterError", awserr.ErrInvalidParameter)
	// ErrInvalidNetworkSettings is returned when a SubnetId/SecurityGroupId
	// supplied to CreateFileSystem doesn't match the real ID format.
	ErrInvalidNetworkSettings = awserr.New("InvalidNetworkSettings", awserr.ErrInvalidParameter)
	// ErrDataRepositoryTaskExecuting is returned by CreateDataRepositoryTask
	// when the target file system already has a task with Lifecycle
	// EXECUTING (fsx@v1.68.4 types/errors.go: "An existing data repository
	// task is currently executing on the file system. Wait until the
	// existing task has completed, then create the new task.").
	ErrDataRepositoryTaskExecuting = awserr.New("DataRepositoryTaskExecuting", awserr.ErrInvalidParameter)
	// ErrMissingFileCacheConfiguration is returned when CreateFileCache is
	// called without the required LustreConfiguration block (fsx@v1.68.4
	// types/errors.go: "A cache configuration is required for this
	// operation." -- FileCacheType is always LUSTRE, so this is the only
	// per-type config block CreateFileCache has).
	ErrMissingFileCacheConfiguration = awserr.New("MissingFileCacheConfiguration", awserr.ErrInvalidParameter)
)
