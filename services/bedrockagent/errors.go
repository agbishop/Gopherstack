package bedrockagent

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a resource with the given name already exists.
	ErrAlreadyExists = awserr.New("ConflictException", awserr.ErrAlreadyExists)
	// ErrValidation is returned for invalid request parameters.
	ErrValidation = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrResourceInUse is returned when deletion is blocked because a
	// dependent resource (e.g. an alias) still references the target, and
	// the caller did not pass skipResourceInUseCheck=true.
	ErrResourceInUse = awserr.New("ConflictException", awserr.ErrConflict)
)
