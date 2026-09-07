package appsync

import (
	"fmt"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// StartSchemaCreation parses and stores the schema SDL for an API.
func (b *InMemoryBackend) StartSchemaCreation(apiID, sdl string) (*Schema, error) {
	b.mu.Lock("StartSchemaCreation")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return nil, fmt.Errorf("%w: api %s not found", ErrNotFound, apiID)
	}

	// Validate and parse the schema.
	parsed, gqlErr := gqlparser.LoadSchema(&ast.Source{
		Name:  "schema.graphql",
		Input: sdl,
	})
	if gqlErr != nil {
		schema := &Schema{
			APIID:   apiID,
			SDL:     sdl,
			Status:  SchemaStatusFailed,
			Details: gqlErr.Error(),
		}
		b.schemas.Put(schema)

		return nil, fmt.Errorf("%w: %s", ErrInvalidSchema, gqlErr.Error())
	}

	schema := &Schema{
		APIID:        apiID,
		SDL:          sdl,
		Status:       SchemaStatusActive,
		parsedSchema: parsed,
	}
	b.schemas.Put(schema)

	cp := *schema

	return &cp, nil
}

// GetSchemaCreationStatus returns the current schema creation status for an API.
func (b *InMemoryBackend) GetSchemaCreationStatus(apiID string) (*Schema, error) {
	b.mu.RLock("GetSchemaCreationStatus")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, fmt.Errorf("%w: api %s not found", ErrNotFound, apiID)
	}

	schema, ok := b.schemas.Get(apiID)
	if !ok {
		return &Schema{
			APIID:  apiID,
			Status: SchemaStatusNotApplicable,
		}, nil
	}

	cp := *schema

	return &cp, nil
}

// GetIntrospectionSchema returns the schema for an API in the requested
// format. format must be SDL or JSON (types.OutputType, aws-sdk-go-v2
// appsync@v1.56.4 types/enums.go:535-541); an empty format defaults to SDL,
// matching the handler's own default for an omitted query parameter. Any
// other value is rejected the same way CreateType already rejects an
// unrecognized TypeDefinitionFormat (isValidTypeFormat).
//
// includeDirectives controls only the JSON output's top-level "directives"
// list. SDL is always returned as the raw stored text verbatim -- honoring
// includeDirectives there would mean re-serializing the schema instead of
// returning what was stored, and is not attempted.
func (b *InMemoryBackend) GetIntrospectionSchema(apiID, format string, includeDirectives bool) ([]byte, error) {
	b.mu.RLock("GetIntrospectionSchema")
	defer b.mu.RUnlock()

	if format == "" {
		format = string(TypeFormatSDL)
	}

	if !isValidTypeFormat(TypeDefinitionFormat(format)) {
		// Landmine (gopherstack-w4kf): none of this op's declared errors
		// (GraphQLSchemaException/InternalFailureException/NotFoundException/
		// UnauthorizedException) fits a malformed format value -- BadRequestException
		// stays wrong on the wire here; no safe swap found.
		return nil, fmt.Errorf("%w: invalid format %q, must be SDL or JSON", ErrValidation, format)
	}

	schema, ok := b.schemas.Get(apiID)
	if !ok {
		return nil, fmt.Errorf("%w: schema not found for api %s", ErrNotFound, apiID)
	}

	if format == string(TypeFormatSDL) {
		return []byte(schema.SDL), nil
	}

	if schema.parsedSchema == nil {
		return nil, fmt.Errorf(
			"%w: schema for api %s has no valid parsed schema for JSON introspection",
			ErrGraphQLSchemaInvalid, apiID,
		)
	}

	return marshalIntrospectionSchema(schema.parsedSchema, includeDirectives)
}
