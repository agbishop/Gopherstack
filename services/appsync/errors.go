package appsync

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrNotFound is returned when a resource is not found.
	ErrNotFound = awserr.New("NotFoundException", awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a resource already exists.
	ErrAlreadyExists = awserr.New("BadRequestException", awserr.ErrAlreadyExists)
	// ErrInvalidSchema is returned when the provided schema SDL is invalid.
	ErrInvalidSchema = errors.New("InvalidSchemaError")
	// ErrGraphQLSchemaInvalid is returned by GetIntrospectionSchema when the
	// stored schema has no valid parsed form to introspect. Its declared error
	// set (appsync@v1.56.4 deserializers.go) has GraphQLSchemaException but no
	// BadRequestException, unlike StartSchemaCreation's -- do not reuse
	// ErrInvalidSchema here (gopherstack-w4kf).
	ErrGraphQLSchemaInvalid = awserr.New("GraphQLSchemaException", awserr.ErrInvalidParameter)
	// ErrValidation is returned when input validation fails.
	ErrValidation = awserr.New("BadRequestException", awserr.ErrInvalidParameter)
	// ErrUnsupportedJSCode is returned when EvaluateCode is given an APPSYNC_JS
	// construct the emulator's evaluator does not support. The request is
	// well-formed, but the code uses features beyond the documented patterns the
	// emulator faithfully evaluates (the emulator does not embed a JS engine).
	ErrUnsupportedJSCode = awserr.New("BadRequestException", awserr.ErrInvalidParameter)
	// ErrAPIKeyLimitExceeded is returned by CreateAPIKey when an API already
	// has the maximum number of API keys (appsync@v1.56.4 types/errors.go:36:
	// "The API key exceeded a limit. Try your request again.").
	ErrAPIKeyLimitExceeded = awserr.New("ApiKeyLimitExceededException", awserr.ErrInvalidParameter)
	// ErrAPIKeyValidityOutOfBounds is returned by Create/UpdateAPIKey when the
	// requested expiry falls outside AWS's documented bounds (appsync@v1.56.4
	// types/errors.go:62-63: "The API key expiration must be set to a value
	// between 1 and 365 days from creation (for CreateApiKey) or from update
	// (for UpdateApiKey).").
	ErrAPIKeyValidityOutOfBounds = awserr.New("ApiKeyValidityOutOfBoundsException", awserr.ErrInvalidParameter)
)
