package appconfig

import (
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrApplicationNotFound is returned when the requested application does not exist.
	ErrApplicationNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrEnvironmentNotFound is returned when the requested environment does not exist.
	ErrEnvironmentNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrConfigurationProfileNotFound is returned when the requested configuration profile does not exist.
	ErrConfigurationProfileNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrHostedConfigVersionNotFound is returned when the requested hosted configuration version does not exist.
	ErrHostedConfigVersionNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrDeploymentStrategyNotFound is returned when the requested deployment strategy does not exist.
	ErrDeploymentStrategyNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrDeploymentNotFound is returned when the requested deployment does not exist.
	ErrDeploymentNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrExtensionNotFound is returned when the requested extension does not exist.
	ErrExtensionNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrExtensionAssociationNotFound is returned when the requested extension association does not exist.
	ErrExtensionAssociationNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrExtensionAlreadyExists is returned when an extension with the same name already exists.
	ErrExtensionAlreadyExists = awserr.New("ConflictException", awserr.ErrAlreadyExists)
	// ErrBadRequest is returned when a required field is missing or invalid.
	ErrBadRequest = awserr.New("BadRequestException", awserr.ErrInvalidParameter)
	// ErrConflict is returned when a resource with the same name already exists.
	ErrConflict = awserr.New("ConflictException", awserr.ErrAlreadyExists)
	// ErrPayloadTooLarge is returned when a hosted configuration version exceeds the maximum size.
	ErrPayloadTooLarge = awserr.New("PayloadTooLargeException", awserr.ErrInvalidParameter)
	// ErrExperimentDefinitionNotFound is returned when the requested experiment definition does not exist.
	ErrExperimentDefinitionNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrExperimentRunNotFound is returned when the requested experiment run does not exist.
	ErrExperimentRunNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrDeletionProtected is returned when DeleteEnvironment/DeleteConfigurationProfile is
	// blocked because AppConfigData recorded a GetLatestConfiguration call for the resource
	// within the deletion-protection interval (gopherstack-z4v1).
	ErrDeletionProtected = awserr.New("ConflictException", awserr.ErrConflict)
)
