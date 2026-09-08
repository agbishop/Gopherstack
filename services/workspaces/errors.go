package workspaces

import (
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

const (
	errResourceNotFound       = "ResourceNotFoundException"
	errInvalidParameterValues = "InvalidParameterValuesException"
	errResourceAlreadyExists  = "ResourceAlreadyExistsException"
	errInvalidResourceState   = "InvalidResourceStateException"
	// errOperationNotSupported is used as a per-item FailedRequest ErrorCode
	// (RebootWorkspaces/RebuildWorkspaces), not wrapped in an awserr sentinel:
	// both operations report a bad-state workspace as one failed item in an
	// otherwise-200 batch response, not as a request-level HTTP error.
	errOperationNotSupported = "OperationNotSupportedException"
)

var (
	// ErrWorkspaceNotFound is returned when a workspace does not exist.
	ErrWorkspaceNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrInvalidParameter is returned on invalid input.
	ErrInvalidParameter = awserr.New(errInvalidParameterValues, awserr.ErrInvalidParameter)

	// errDirectoryAlreadyRegistered is returned by RegisterWorkspaceDirectory
	// when the directory is already registered.
	errDirectoryAlreadyRegistered = awserr.New(errResourceAlreadyExists, awserr.ErrAlreadyExists)
	// errDirectoryHasWorkspaces is returned by DeregisterWorkspaceDirectory
	// when WorkSpaces are still registered to the directory. Real AWS: any
	// WorkSpaces registered to the directory must be removed first, before
	// the directory itself can be deregistered.
	errDirectoryHasWorkspaces = awserr.New(errInvalidResourceState, awserr.ErrConflict)

	// errPoolRunningModeRequiresStopped is returned by UpdateWorkspacesPool
	// when RunningMode is set on a pool that isn't STOPPED. Real AWS:
	// "The running mode can only be updated when the pool is in a stopped
	// state" (UpdateWorkspacesPoolInput.RunningMode doc comment).
	errPoolRunningModeRequiresStopped = awserr.New(errInvalidResourceState, awserr.ErrConflict)

	// errDirectoryNotFound is returned by the directory-properties Modify*
	// operations for a DirectoryId that was never registered via
	// RegisterWorkspaceDirectory.
	errDirectoryNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
)

var (
	errIpGroupNotFound = awserr.New( //nolint:revive,staticcheck // existing issue.
		errResourceNotFound,
		awserr.ErrNotFound,
	)

	// errIPGroupAssociatedWithDirectory is returned by DeleteIpGroup when the
	// group is still associated with a directory. Real AWS: "You cannot
	// delete an IP access control group that is associated with a
	// directory." Real AWS's own deserializer declares ResourceAssociatedException
	// for this case, but this package's generic error dispatch (handler.go's
	// handleError) only distinguishes NotFound/InvalidParameter/AlreadyExists/
	// Conflict, so this reuses errInvalidResourceState like every other
	// still-in-use conflict in this package (e.g. errDirectoryHasWorkspaces).
	errIPGroupAssociatedWithDirectory = awserr.New(errInvalidResourceState, awserr.ErrConflict)

	errConnAliasNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	errBundleNotFound    = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	errImageNotFound     = awserr.New(errResourceNotFound, awserr.ErrNotFound)

	// errBundleInUse is returned by DeleteWorkspaceBundle when a WorkSpace
	// still references the bundle. Real AWS models ResourceAssociatedException
	// for this operation; reused as errInvalidResourceState/Conflict for the
	// same reason as errIPGroupAssociatedWithDirectory.
	errBundleInUse = awserr.New(errInvalidResourceState, awserr.ErrConflict)
	// errImageInUse is returned by DeleteWorkspaceImage when a bundle still
	// references the image. Real AWS: "To delete an image, you must first
	// delete any bundles that are associated with the image"
	// (api_op_DeleteWorkspaceImage.go doc comment); ResourceAssociatedException
	// is modelled for this operation.
	errImageInUse = awserr.New(errInvalidResourceState, awserr.ErrConflict)
	// errConnAliasInUse is returned by DeleteConnectionAlias when the alias is
	// still shared with an account or associated with a directory. Real AWS:
	// "You can delete a connection alias only after it is no longer shared
	// with any accounts or associated with any directories"
	// (api_op_DeleteConnectionAlias.go doc comment).
	errConnAliasInUse      = awserr.New(errInvalidResourceState, awserr.ErrConflict)
	errPoolNotFound        = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	errPoolSessionNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	errAddInNotFound       = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	errAccountLinkNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
)
