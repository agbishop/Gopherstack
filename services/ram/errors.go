package ram

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrValidation is returned when a request contains an invalid or missing parameter.
	// CreateResourceShare, AssociateResourceShare and DeletePermissionVersion --
	// the three ops that raise this -- all model InvalidParameterException for it
	// (ram@v1.39.4 deserializers.go, each op's own deserializeOpError switch).
	ErrValidation = awserr.New("invalid or missing parameter", awserr.ErrInvalidParameter)
	// ErrNotFound is returned when a resource share does not exist.
	ErrNotFound = awserr.New("UnknownResourceException", awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a resource share already exists.
	//
	// CreateResourceShare's own error model (ram@v1.39.4 deserializers.go
	// awsRestjson1_deserializeOpErrorCreateResourceShare) defines no
	// AlreadyExists-shaped exception at all -- real AWS RAM does not reject
	// duplicate resource-share names (only the ARN is unique), so this check
	// itself may not belong here. Left as-is per audit policy (no code
	// matches this op's failure, so none is invented); see gopherstack-101r
	// follow-up notes for whether to drop the duplicate-name rejection.
	ErrAlreadyExists = awserr.New("ResourceShareAlreadyExistsException", awserr.ErrConflict)
	// ErrPermissionAlreadyExists is returned when a customer-managed
	// permission with the same name already exists. CreatePermission's own
	// error model defines PermissionAlreadyExistsException for this.
	ErrPermissionAlreadyExists = awserr.New("permission already exists", awserr.ErrConflict)
	// ErrPermissionNotFound is returned when a permission does not exist.
	ErrPermissionNotFound = awserr.New("InvalidParameterException", awserr.ErrNotFound)
	// ErrInvitationNotFound is returned when an invitation does not exist.
	ErrInvitationNotFound = awserr.New(
		"ResourceShareInvitationArnNotFoundException",
		awserr.ErrNotFound,
	)
	// ErrInvitationAlreadyAccepted is returned when accepting an already-accepted invitation.
	ErrInvitationAlreadyAccepted = awserr.New(
		"ResourceShareInvitationAlreadyAcceptedException",
		awserr.ErrConflict,
	)
	// ErrInvitationAlreadyRejected is returned when accepting or rejecting an already-rejected invitation.
	ErrInvitationAlreadyRejected = awserr.New(
		"ResourceShareInvitationAlreadyRejectedException",
		awserr.ErrConflict,
	)
	// ErrInvitationExpired is returned when acting on an expired invitation.
	ErrInvitationExpired = awserr.New(
		"ResourceShareInvitationExpiredException",
		awserr.ErrConflict,
	)
	// ErrPermissionVersionNotFound is returned when a permission version does not exist.
	ErrPermissionVersionNotFound = awserr.New("InvalidParameterException", awserr.ErrNotFound)
	// ErrOperationNotPermitted is returned when an operation is not permitted on an AWS-managed resource.
	ErrOperationNotPermitted = awserr.New("OperationNotPermittedException", awserr.ErrConflict)
	// ErrPermissionInUse is returned when deleting a permission that is
	// associated with active shares. DeletePermission's own error model has
	// no PermissionInUseException -- it defines no InUse-shaped exception at
	// all -- but does define OperationNotPermittedException, matching both
	// its own doc comment and DeletePermission's ("only if it isn't attached
	// to any resource share").
	ErrPermissionInUse = awserr.New("permission is associated with one or more resource shares", awserr.ErrConflict)
	// ErrInvalidParameter is returned when a parameter value is out of the allowed range.
	ErrInvalidParameter = awserr.New("InvalidParameterException", awserr.ErrInvalidParameter)
	// ErrMalformedArn is returned when a resourceArns entry isn't ARN-shaped.
	// CreateResourceShare and AssociateResourceShare both model
	// MalformedArnException for this (ram@v1.39.4 deserializers.go,
	// awsRestjson1_deserializeOpErrorCreateResourceShare and
	// awsRestjson1_deserializeOpErrorAssociateResourceShare).
	ErrMalformedArn = awserr.New("MalformedArnException", awserr.ErrInvalidParameter)
)
