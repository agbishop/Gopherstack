package databrew

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

// AWS restjson1 exception type names, shared between the top-level
// handleError mapping and any op that needs to embed one directly in a
// partial-failure response (e.g. BatchDeleteRecipeVersion's
// []RecipeVersionErrorDetail).
const (
	errCodeResourceNotFound = "ResourceNotFoundException"
	errCodeValidation       = "ValidationException"
	errCodeConflict         = "ConflictException"
)

var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = awserr.New(errCodeResourceNotFound, awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a resource already exists.
	ErrAlreadyExists = awserr.New(errCodeConflict, awserr.ErrAlreadyExists)
	// ErrValidation is returned when input validation fails.
	ErrValidation = awserr.New(errCodeValidation, awserr.ErrInvalidParameter)
	// ErrConflict is returned when an operation conflicts with existing
	// resource state, e.g. deleting a resource still referenced by another.
	ErrConflict = awserr.New(errCodeConflict, awserr.ErrConflict)
)
