package pinpoint

import (
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrAppNotFound is returned when a Pinpoint application is not found.
	ErrAppNotFound = awserr.New("NotFoundException: app not found", awserr.ErrNotFound)
	// ErrValidation is returned when request parameters fail validation.
	ErrValidation = awserr.New("BadRequestException", awserr.ErrInvalidParameter)
	// ErrAlreadyExists is returned when a resource with the same key already exists.
	ErrAlreadyExists = awserr.New("ConflictException: resource already exists", awserr.ErrConflict)
	// ErrJourneyActive is returned when attempting to modify an ACTIVE journey's activities.
	ErrJourneyActive = awserr.New(
		"ConflictException: journey is ACTIVE and cannot be modified",
		awserr.ErrConflict,
	)
)
