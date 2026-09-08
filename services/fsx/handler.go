package fsx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	fsxTargetPrefix = "AWSSimbaAPIService_v20180301."
	matchPriority   = service.PriorityHeaderExact

	opCreateFileSystem           = "CreateFileSystem"
	opCreateFileSystemFromBackup = "CreateFileSystemFromBackup"
	opDescribeFileSystems        = "DescribeFileSystems"
	opDeleteFileSystem           = "DeleteFileSystem"
	opUpdateFileSystem           = "UpdateFileSystem"
	opCreateBackup               = "CreateBackup"
	opDescribeBackups            = "DescribeBackups"
	opDeleteBackup               = "DeleteBackup"
	opCopyBackup                 = "CopyBackup"
	opTagResource                = "TagResource"
	opUntagResource              = "UntagResource"
	opListTagsForResource        = "ListTagsForResource"

	opAssociateFileSystemAliases    = "AssociateFileSystemAliases"
	opDisassociateFileSystemAliases = "DisassociateFileSystemAliases"
	opDescribeFileSystemAliases     = "DescribeFileSystemAliases"

	opCreateDataRepositoryAssociation = "CreateDataRepositoryAssociation"
	opDeleteDataRepositoryAssociation = "DeleteDataRepositoryAssociation"
	opDescribeDataRepositoryAssocs    = "DescribeDataRepositoryAssociations"
	opUpdateDataRepositoryAssociation = "UpdateDataRepositoryAssociation"

	opCancelDataRepositoryTask    = "CancelDataRepositoryTask"
	opCreateDataRepositoryTask    = "CreateDataRepositoryTask"
	opDescribeDataRepositoryTasks = "DescribeDataRepositoryTasks"

	opCreateFileCache    = "CreateFileCache"
	opDeleteFileCache    = "DeleteFileCache"
	opDescribeFileCaches = "DescribeFileCaches"
	opUpdateFileCache    = "UpdateFileCache"

	opCreateSnapshot              = "CreateSnapshot"
	opDeleteSnapshot              = "DeleteSnapshot"
	opDescribeSnapshots           = "DescribeSnapshots"
	opUpdateSnapshot              = "UpdateSnapshot"
	opCopySnapshotAndUpdateVolume = "CopySnapshotAndUpdateVolume"

	opCreateStorageVirtualMachine    = "CreateStorageVirtualMachine"
	opDeleteStorageVirtualMachine    = "DeleteStorageVirtualMachine"
	opDescribeStorageVirtualMachines = "DescribeStorageVirtualMachines"
	opUpdateStorageVirtualMachine    = "UpdateStorageVirtualMachine"

	opCreateVolume              = "CreateVolume"
	opCreateVolumeFromBackup    = "CreateVolumeFromBackup"
	opDeleteVolume              = "DeleteVolume"
	opDescribeVolumes           = "DescribeVolumes"
	opRestoreVolumeFromSnapshot = "RestoreVolumeFromSnapshot"
	opUpdateVolume              = "UpdateVolume"

	opCreateAndAttachS3AccessPoint     = "CreateAndAttachS3AccessPoint"
	opDetachAndDeleteS3AccessPoint     = "DetachAndDeleteS3AccessPoint"
	opDescribeS3AccessPointAttachments = "DescribeS3AccessPointAttachments"

	opDescribeSharedVpcConfiguration = "DescribeSharedVpcConfiguration"
	opUpdateSharedVpcConfiguration   = "UpdateSharedVpcConfiguration"

	opReleaseFileSystemNfsV3Locks     = "ReleaseFileSystemNfsV3Locks"
	opStartMisconfiguredStateRecovery = "StartMisconfiguredStateRecovery"
)

var errUnknownOperation = errors.New("UnsupportedOperation")

// Handler handles FSx HTTP requests.
type Handler struct {
	Backend StorageBackend
	ops     map[string]service.JSONOpFunc
}

// NewHandler constructs a new Handler.
func NewHandler(b StorageBackend) *Handler {
	h := &Handler{Backend: b}
	h.ops = h.buildOps()

	return h
}

// Name returns the service name.
func (h *Handler) Name() string { return "FSx" }

// Reset resets the backend.
func (h *Handler) Reset() { h.Backend.Reset() }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateFileSystem,
		opCreateFileSystemFromBackup,
		opDescribeFileSystems,
		opDeleteFileSystem,
		opUpdateFileSystem,
		opCreateBackup,
		opDescribeBackups,
		opDeleteBackup,
		opCopyBackup,
		opTagResource,
		opUntagResource,
		opListTagsForResource,
		opAssociateFileSystemAliases,
		opDisassociateFileSystemAliases,
		opDescribeFileSystemAliases,
		opCreateDataRepositoryAssociation,
		opDeleteDataRepositoryAssociation,
		opDescribeDataRepositoryAssocs,
		opUpdateDataRepositoryAssociation,
		opCancelDataRepositoryTask,
		opCreateDataRepositoryTask,
		opDescribeDataRepositoryTasks,
		opCreateFileCache,
		opDeleteFileCache,
		opDescribeFileCaches,
		opUpdateFileCache,
		opCreateSnapshot,
		opDeleteSnapshot,
		opDescribeSnapshots,
		opUpdateSnapshot,
		opCopySnapshotAndUpdateVolume,
		opCreateStorageVirtualMachine,
		opDeleteStorageVirtualMachine,
		opDescribeStorageVirtualMachines,
		opUpdateStorageVirtualMachine,
		opCreateVolume,
		opCreateVolumeFromBackup,
		opDeleteVolume,
		opDescribeVolumes,
		opRestoreVolumeFromSnapshot,
		opUpdateVolume,
		opCreateAndAttachS3AccessPoint,
		opDetachAndDeleteS3AccessPoint,
		opDescribeS3AccessPointAttachments,
		opDescribeSharedVpcConfiguration,
		opUpdateSharedVpcConfiguration,
		opReleaseFileSystemNfsV3Locks,
		opStartMisconfiguredStateRecovery,
	}
}

// RouteMatcher returns a matcher that accepts FSx requests by X-Amz-Target.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, fsxTargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return matchPriority }

// ExtractOperation extracts the FSx operation from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")

	return strings.TrimPrefix(target, fsxTargetPrefix)
}

// ExtractResource extracts a resource identifier from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil || len(body) == 0 {
		return ""
	}

	var req struct {
		FileSystemID string `json:"FileSystemId"`
		BackupID     string `json:"BackupId"`
		ResourceARN  string `json:"ResourceARN"`
	}

	_ = json.Unmarshal(body, &req)

	switch {
	case req.ResourceARN != "":
		return req.ResourceARN
	case req.FileSystemID != "":
		return req.FileSystemID
	case req.BackupID != "":
		return req.BackupID
	}

	return ""
}

// Snapshot returns a serialized snapshot of the backend state.
// It implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte { return h.Backend.Snapshot(ctx) }

// Restore restores the backend state from a snapshot.
// It implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	return h.Backend.Restore(ctx, data)
}

// Handler returns the echo handler func.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"FSx", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

func (h *Handler) buildOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		opCreateFileSystem:           service.WrapOp(h.handleCreateFileSystem),
		opCreateFileSystemFromBackup: service.WrapOp(h.handleCreateFileSystemFromBackup),
		opDescribeFileSystems:        service.WrapOp(h.handleDescribeFileSystems),
		opDeleteFileSystem:           service.WrapOp(h.handleDeleteFileSystem),
		opUpdateFileSystem:           service.WrapOp(h.handleUpdateFileSystem),
		opCreateBackup:               service.WrapOp(h.handleCreateBackup),
		opDescribeBackups:            service.WrapOp(h.handleDescribeBackups),
		opDeleteBackup:               service.WrapOp(h.handleDeleteBackup),
		opCopyBackup:                 service.WrapOp(h.handleCopyBackup),
		opTagResource:                service.WrapOp(h.handleTagResource),
		opUntagResource:              service.WrapOp(h.handleUntagResource),
		opListTagsForResource:        service.WrapOp(h.handleListTagsForResource),

		opAssociateFileSystemAliases:    service.WrapOp(h.handleAssociateFileSystemAliases),
		opDisassociateFileSystemAliases: service.WrapOp(h.handleDisassociateFileSystemAliases),
		opDescribeFileSystemAliases:     service.WrapOp(h.handleDescribeFileSystemAliases),

		opCreateDataRepositoryAssociation: service.WrapOp(h.handleCreateDataRepositoryAssociation),
		opDeleteDataRepositoryAssociation: service.WrapOp(h.handleDeleteDataRepositoryAssociation),
		opDescribeDataRepositoryAssocs:    service.WrapOp(h.handleDescribeDataRepositoryAssociations),
		opUpdateDataRepositoryAssociation: service.WrapOp(h.handleUpdateDataRepositoryAssociation),

		opCancelDataRepositoryTask:    service.WrapOp(h.handleCancelDataRepositoryTask),
		opCreateDataRepositoryTask:    service.WrapOp(h.handleCreateDataRepositoryTask),
		opDescribeDataRepositoryTasks: service.WrapOp(h.handleDescribeDataRepositoryTasks),

		opCreateFileCache:    service.WrapOp(h.handleCreateFileCache),
		opDeleteFileCache:    service.WrapOp(h.handleDeleteFileCache),
		opDescribeFileCaches: service.WrapOp(h.handleDescribeFileCaches),
		opUpdateFileCache:    service.WrapOp(h.handleUpdateFileCache),

		opCreateSnapshot:              service.WrapOp(h.handleCreateSnapshot),
		opDeleteSnapshot:              service.WrapOp(h.handleDeleteSnapshot),
		opDescribeSnapshots:           service.WrapOp(h.handleDescribeSnapshots),
		opUpdateSnapshot:              service.WrapOp(h.handleUpdateSnapshot),
		opCopySnapshotAndUpdateVolume: service.WrapOp(h.handleCopySnapshotAndUpdateVolume),

		opCreateStorageVirtualMachine:    service.WrapOp(h.handleCreateStorageVirtualMachine),
		opDeleteStorageVirtualMachine:    service.WrapOp(h.handleDeleteStorageVirtualMachine),
		opDescribeStorageVirtualMachines: service.WrapOp(h.handleDescribeStorageVirtualMachines),
		opUpdateStorageVirtualMachine:    service.WrapOp(h.handleUpdateStorageVirtualMachine),

		opCreateVolume:              service.WrapOp(h.handleCreateVolume),
		opCreateVolumeFromBackup:    service.WrapOp(h.handleCreateVolumeFromBackup),
		opDeleteVolume:              service.WrapOp(h.handleDeleteVolume),
		opDescribeVolumes:           service.WrapOp(h.handleDescribeVolumes),
		opRestoreVolumeFromSnapshot: service.WrapOp(h.handleRestoreVolumeFromSnapshot),
		opUpdateVolume:              service.WrapOp(h.handleUpdateVolume),

		opCreateAndAttachS3AccessPoint:     service.WrapOp(h.handleCreateAndAttachS3AccessPoint),
		opDetachAndDeleteS3AccessPoint:     service.WrapOp(h.handleDetachAndDeleteS3AccessPoint),
		opDescribeS3AccessPointAttachments: service.WrapOp(h.handleDescribeS3AccessPointAttachments),

		opDescribeSharedVpcConfiguration: service.WrapOp(h.handleDescribeSharedVpcConfiguration),
		opUpdateSharedVpcConfiguration:   service.WrapOp(h.handleUpdateSharedVpcConfiguration),

		opReleaseFileSystemNfsV3Locks:     service.WrapOp(h.handleReleaseFileSystemNfsV3Locks),
		opStartMisconfiguredStateRecovery: service.WrapOp(h.handleStartMisconfiguredStateRecovery),
	}
}

func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnknownOperation, action)
	}

	result, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(result)
}

func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	if code, ok := notFoundErrorCode(err); ok {
		return c.JSON(http.StatusBadRequest, errorResponse(code, err.Error()))
	}

	switch {
	case errors.Is(err, ErrTagLimitExceeded):
		return c.JSON(http.StatusBadRequest, errorResponse("ServiceLimitExceeded", err.Error()))
	case errors.Is(err, ErrTagInvalid):
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequest", err.Error()))
	case errors.Is(err, ErrMissingFileSystemConfiguration):
		return c.JSON(http.StatusBadRequest, errorResponse("MissingFileSystemConfiguration", err.Error()))
	case errors.Is(err, ErrMissingVolumeConfiguration):
		return c.JSON(http.StatusBadRequest, errorResponse("MissingVolumeConfiguration", err.Error()))
	case errors.Is(err, ErrIncompatibleParameter):
		return c.JSON(http.StatusBadRequest, errorResponse("IncompatibleParameterError", err.Error()))
	case errors.Is(err, ErrInvalidNetworkSettings):
		return c.JSON(http.StatusBadRequest, errorResponse("InvalidNetworkSettings", err.Error()))
	case errors.Is(err, ErrDataRepositoryTaskExecuting):
		return c.JSON(http.StatusBadRequest, errorResponse("DataRepositoryTaskExecuting", err.Error()))
	case errors.Is(err, ErrMissingFileCacheConfiguration):
		return c.JSON(http.StatusBadRequest, errorResponse("MissingFileCacheConfiguration", err.Error()))
	case errors.Is(err, awserr.ErrInvalidParameter):
		// BadRequest is real FSx's generic client-error code (see
		// types/errors.go in the SDK); there is no "ValidationError"
		// exception type in FSx.
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequest", err.Error()))
	case errors.Is(err, errUnknownOperation):
		return c.JSON(http.StatusBadRequest, errorResponse("UnsupportedOperation", err.Error()))
	default:
		return c.JSON(http.StatusInternalServerError, errorResponse("InternalFailure", err.Error()))
	}
}

// notFoundErrorCode maps the family of "not found" sentinel errors to their
// AWS error code. It is split out of handleError to keep that function's
// cyclomatic complexity bounded. The generic awserr.ErrNotFound check must
// stay last: every specific *NotFound sentinel also wraps it, so a more
// specific case earlier in this list must win.
func notFoundErrorCode(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrBackupNotFound):
		return "BackupNotFound", true
	case errors.Is(err, ErrSnapshotNotFound):
		return "SnapshotNotFound", true
	case errors.Is(err, ErrStorageVirtualMachineNotFound):
		return "StorageVirtualMachineNotFound", true
	case errors.Is(err, ErrVolumeNotFound):
		return "VolumeNotFound", true
	case errors.Is(err, ErrFileCacheNotFound):
		return "FileCacheNotFound", true
	case errors.Is(err, ErrDataRepositoryAssociationNotFound):
		return "DataRepositoryAssociationNotFound", true
	case errors.Is(err, ErrDataRepositoryTaskNotFound):
		return "DataRepositoryTaskNotFound", true
	case errors.Is(err, ErrS3AccessPointNotFound):
		return "InvalidRequest", true
	case errors.Is(err, ErrS3AccessPointAttachmentNotFound):
		return "S3AccessPointAttachmentNotFound", true
	case errors.Is(err, ErrResourceNotFound):
		return "ResourceNotFound", true
	case errors.Is(err, awserr.ErrNotFound):
		return "FileSystemNotFound", true
	default:
		return "", false
	}
}

func errorResponse(code, msg string) map[string]string {
	return map[string]string{"__type": code, "message": msg}
}
