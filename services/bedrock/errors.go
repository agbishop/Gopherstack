package bedrock

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a resource already exists.
	ErrAlreadyExists = awserr.New("ConflictException", awserr.ErrAlreadyExists)
	// ErrValidation is returned when request validation fails.
	ErrValidation = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrResourceInUse is returned when a resource cannot be deleted because
	// dependent artifacts still reference it.
	ErrResourceInUse = awserr.New("ResourceInUseException", awserr.ErrConflict)
)
