package verifiedpermissions

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrPolicyStoreNotFound is returned when a policy store is not found.
	ErrPolicyStoreNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrPolicyNotFound is returned when a policy is not found.
	ErrPolicyNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrPolicyTemplateNotFound is returned when a policy template is not found.
	ErrPolicyTemplateNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrIdentitySourceNotFound is returned when an identity source is not found.
	ErrIdentitySourceNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrSchemaNotFound is returned when no schema has been set for a policy store.
	ErrSchemaNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrValidation is returned when input fails validation.
	ErrValidation = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrConflict is returned when a resource conflict prevents an operation.
	ErrConflict = awserr.New("ConflictException", awserr.ErrConflict)
	// ErrPolicyStoreDeletionProtected is returned when DeletePolicyStore is
	// called on a store with deletion protection enabled. Real AWS models
	// this as InvalidStateException, not ConflictException (SDK
	// types/errors.go doc comment on InvalidStateException: "The policy
	// store can't be deleted because deletion protection is enabled.").
	ErrPolicyStoreDeletionProtected = awserr.New("InvalidStateException", awserr.ErrConflict)
	// ErrTooManyTags is returned when TagResource would push a resource's tag
	// count over the 50-tag limit. Real AWS only declares TooManyTagsException
	// for TagResource -- CreatePolicyStore's tag-count overflow stays a plain
	// ValidationException (ErrValidation), per the SDK's per-op error models.
	ErrTooManyTags = awserr.New("TooManyTagsException", awserr.ErrInvalidParameter)
)
