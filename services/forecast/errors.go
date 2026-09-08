package forecast

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrNotFound is returned when a requested Forecast resource is absent.
	ErrNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a Forecast resource name already exists.
	ErrAlreadyExists = awserr.New("ResourceAlreadyExistsException", awserr.ErrConflict)
	// ErrValidation is returned when a Forecast request is invalid.
	ErrValidation = awserr.New("InvalidInputException", awserr.ErrInvalidParameter)
	// ErrInvalidNextToken is returned when a List* NextToken cannot be decoded.
	// Real Amazon Forecast models InvalidNextTokenException on every List operation.
	ErrInvalidNextToken = awserr.New("InvalidNextTokenException", awserr.ErrInvalidParameter)
	// ErrResourceInUse is returned when a Delete* operation targets a resource
	// that is not yet in a deletable state (e.g. still CREATE_PENDING). Real
	// Amazon Forecast models ResourceInUseException on every Delete operation
	// (e.g. "You can delete only predictor that have a status of ACTIVE or
	// CREATE_FAILED", per the DeletePredictor API doc).
	ErrResourceInUse = awserr.New("ResourceInUseException", awserr.ErrConflict)
	// ErrTagLimitExceeded is returned when TagResource would leave a resource
	// with more tags than the documented per-resource maximum (see
	// maxTagsPerResource in tags.go). Documentation-sourced, not SDK-verified.
	ErrTagLimitExceeded = awserr.New("LimitExceededException", awserr.ErrInvalidParameter)
)
