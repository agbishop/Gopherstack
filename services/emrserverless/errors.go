package emrserverless

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a resource already exists.
	ErrAlreadyExists = awserr.New("ConflictException", awserr.ErrAlreadyExists)
	// ErrValidation is returned when input validation fails.
	ErrValidation = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrInvalidState is returned when an operation is not valid for the resource's
	// current state. emrserverless models no "RequestFailedException" for any
	// operation (types/errors.go only defines ConflictException/
	// InternalServerException/ResourceNotFoundException/
	// ServiceQuotaExceededException/ValidationException); ValidationException is
	// the only client-error type every state-precondition op below (DeleteApplication/
	// StartApplication/StopApplication/CancelJobRun/TerminateSession/
	// GetSessionEndpoint) actually models.
	ErrInvalidState = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrConflict is returned when starting a job run or session on an
	// application that is not STARTED and whose autoStartConfiguration
	// disables implicit start (types.AutoStartConfig.Enabled defaults to
	// true). ConflictException is modeled on StartJobRun/StartSession for
	// exactly this "conflict in the current state of the resource".
	ErrConflict = awserr.New("ConflictException", awserr.ErrConflict)
)
