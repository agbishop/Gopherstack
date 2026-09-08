package swf

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrNotFound is returned when a resource is not found.
	ErrNotFound = awserr.New("UnknownResourceFault", awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a resource already exists.
	ErrAlreadyExists = awserr.New("DomainAlreadyExistsFault", awserr.ErrAlreadyExists)
	// ErrDeprecated is returned when deprecating an already-deprecated domain.
	ErrDeprecated = errors.New("DomainDeprecatedFault")
	// ErrTypeAlreadyExists is returned when a workflow or activity type already exists.
	ErrTypeAlreadyExists = errors.New("TypeAlreadyExistsFault")
	// ErrTypeDeprecated is returned when a type is already deprecated.
	ErrTypeDeprecated = errors.New("TypeDeprecatedFault")
	// ErrTypeNotDeprecated is returned when deleting a type that has not been deprecated.
	ErrTypeNotDeprecated = errors.New("TypeNotDeprecatedFault")
	// ErrValidation is returned when a request parameter fails validation.
	ErrValidation = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrTooManyTags is returned when tag limits are exceeded.
	ErrTooManyTags = awserr.New("TooManyTagsFault", awserr.ErrInvalidParameter)
	// ErrOperationNotPermitted is returned for disallowed operations.
	ErrOperationNotPermitted = awserr.New("OperationNotPermittedFault", awserr.ErrConflict)
	// ErrWorkflowAlreadyStarted is returned when a workflow is already open.
	ErrWorkflowAlreadyStarted = awserr.New("WorkflowExecutionAlreadyStartedFault", awserr.ErrAlreadyExists)
)
