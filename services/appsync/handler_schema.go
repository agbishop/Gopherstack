package appsync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// handleSchemaCreation handles /v1/apis/{apiId}/schemacreation.
func (h *Handler) handleSchemaCreation(ctx context.Context, c *echo.Context, apiID string) error {
	switch c.Request().Method {
	case http.MethodPost:
		return h.startSchemaCreation(ctx, c, apiID)
	case http.MethodGet:
		return h.getSchemaCreationStatus(ctx, c, apiID)
	default:
		return c.JSON(http.StatusMethodNotAllowed, errorResponse("MethodNotAllowed", "method not allowed"))
	}
}

// startSchemaCreation handles POST /v1/apis/{apiId}/schemacreation.
func (h *Handler) startSchemaCreation(ctx context.Context, c *echo.Context, apiID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse("InternalFailure", err.Error()))
	}

	var input struct {
		Definition string `json:"definition"`
	}

	if jsonErr := json.Unmarshal(body, &input); jsonErr != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", "invalid request body"))
	}

	// AWS SDK sends the definition as base64-encoded bytes.
	sdl := input.Definition
	if decoded, decErr := base64.StdEncoding.DecodeString(sdl); decErr == nil {
		sdl = string(decoded)
	}

	schema, schemaErr := h.Backend.StartSchemaCreation(apiID, sdl)
	if schemaErr != nil {
		return h.handleError(ctx, c, "StartSchemaCreation", schemaErr)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyStatus: schema.Status,
		"details": schema.Details,
	})
}

// getSchemaCreationStatus handles GET /v1/apis/{apiId}/schemacreation.
func (h *Handler) getSchemaCreationStatus(ctx context.Context, c *echo.Context, apiID string) error {
	schema, err := h.Backend.GetSchemaCreationStatus(apiID)
	if err != nil {
		return h.handleError(ctx, c, "GetSchemaCreationStatus", err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyStatus: schema.Status,
		"details": schema.Details,
	})
}

// getIntrospectionSchema handles GET /v1/apis/{apiId}/schema. includeDirectives
// defaults to true (unset or unparseable is treated as "not false"), matching
// standard GraphQL introspection's default of including known directives.
func (h *Handler) getIntrospectionSchema(ctx context.Context, c *echo.Context, apiID string) error {
	format := c.Request().URL.Query().Get("format")
	if format == "" {
		format = "SDL"
	}

	includeDirectives := c.Request().URL.Query().Get("includeDirectives") != "false"

	sdl, err := h.Backend.GetIntrospectionSchema(apiID, format, includeDirectives)
	if err != nil {
		return h.handleError(ctx, c, "GetIntrospectionSchema", err)
	}

	c.Response().Header().Set("Content-Type", "application/octet-stream")

	return c.Blob(http.StatusOK, "application/octet-stream", sdl)
}

// startSchemaMerge handles the real AWS SDK endpoint
// POST /v1/mergedApis/{mergedApiIdentifier}/sourceApiAssociations/{associationId}/merge.
//
// The previous implementation lived at the gopherstack-invented path
// /v1/apis/{apiId}/schemaMerge, keyed only by apiId, with an invented response shape
// ({sourceApiSchemaMetadata, status}). Neither the path, the request shape (the real
// op is keyed by BOTH mergedApiIdentifier and associationId -- a merge always targets
// one specific source API association, never "the merged API" as a whole), nor the
// response shape ({sourceApiAssociationStatus}, not {sourceApiSchemaMetadata, status})
// matched the real SDK, so the old endpoint has been removed rather than aliased: an
// apiId-only request has no way to recover the associationId the real operation
// requires.
func (h *Handler) startSchemaMerge(ctx context.Context, c *echo.Context, mergedAPIID, associationID string) error {
	status, err := h.Backend.StartSchemaMerge(mergedAPIID, associationID)
	if err != nil {
		return h.handleError(ctx, c, "StartSchemaMerge", err)
	}

	return c.JSON(http.StatusOK, map[string]any{"sourceApiAssociationStatus": status})
}
