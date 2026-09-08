package appsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// createGraphqlAPI handles POST /v1/apis.
func (h *Handler) createGraphqlAPI(ctx context.Context, c *echo.Context) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse("InternalFailure", err.Error()))
	}

	var input struct {
		Tags                              map[string]string                  `json:"tags"`
		UserPoolConfig                    *UserPoolConfig                    `json:"userPoolConfig"`
		OpenIDConnectConfig               *OpenIDConnectConfig               `json:"openIDConnectConfig"`
		LambdaAuthorizerConfig            *LambdaAuthorizerConfig            `json:"lambdaAuthorizerConfig"`
		LogConfig                         *LogConfig                         `json:"logConfig"`
		Name                              string                             `json:"name"`
		AuthenticationType                string                             `json:"authenticationType"`
		APIType                           string                             `json:"apiType"`
		Visibility                        string                             `json:"visibility"`
		IntrospectionConfig               string                             `json:"introspectionConfig"`
		OwnerContact                      string                             `json:"ownerContact"`
		AdditionalAuthenticationProviders []AdditionalAuthenticationProvider `json:"additionalAuthenticationProviders"`
		QueryDepthLimit                   int32                              `json:"queryDepthLimit"`
		ResolverCountLimit                int32                              `json:"resolverCountLimit"`
		XrayEnabled                       bool                               `json:"xrayEnabled"`
	}

	if jsonErr := json.Unmarshal(body, &input); jsonErr != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", "invalid request body"))
	}

	if input.Name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", "name is required"))
	}

	authType := AuthenticationType(input.AuthenticationType)
	if authType == "" {
		authType = AuthTypeAPIKey
	}

	cfg := &GraphqlAPIConfig{
		UserPoolConfig:         input.UserPoolConfig,
		OpenIDConnectConfig:    input.OpenIDConnectConfig,
		LambdaAuthorizerConfig: input.LambdaAuthorizerConfig,
		LogConfig:              input.LogConfig,
		IntrospectionConfig:    input.IntrospectionConfig,
		OwnerContact:           input.OwnerContact,
		QueryDepthLimit:        input.QueryDepthLimit,
		ResolverCountLimit:     input.ResolverCountLimit,
	}

	api, createErr := h.Backend.CreateGraphqlAPI(
		input.Name,
		authType,
		input.XrayEnabled,
		input.APIType,
		input.Visibility,
		input.AdditionalAuthenticationProviders,
		input.Tags,
		cfg,
	)
	if createErr != nil {
		return h.handleError(ctx, c, "CreateGraphqlApi", createErr)
	}

	return c.JSON(http.StatusCreated, map[string]any{keyGraphqlAPI: api})
}

// listGraphqlAPIs handles GET /v1/apis.
func (h *Handler) listGraphqlAPIs(ctx context.Context, c *echo.Context) error {
	q := c.Request().URL.Query()
	apiType := q.Get("apiType")
	nextToken := q.Get("nextToken")
	maxResults, _ := strconv.Atoi(q.Get("maxResults"))
	owner := q.Get("owner")

	apis, err := h.Backend.ListGraphqlAPIs(apiType)
	if err != nil {
		return h.handleError(ctx, c, "ListGraphqlApis", err)
	}

	// gopherstack simulates a single AWS account, so every API is
	// CURRENT_ACCOUNT; OTHER_ACCOUNTS never matches anything.
	if owner == "OTHER_ACCOUNTS" {
		apis = nil
	}

	page, tok := appsyncPaginate(apis, nextToken, maxResults)
	out := map[string]any{"graphqlApis": page}
	if tok != "" {
		out["nextToken"] = tok
	}

	return c.JSON(http.StatusOK, out)
}

// getGraphqlAPI handles GET /v1/apis/{apiId}.
func (h *Handler) getGraphqlAPI(ctx context.Context, c *echo.Context, apiID string) error {
	api, err := h.Backend.GetGraphqlAPI(apiID)
	if err != nil {
		return h.handleError(ctx, c, "GetGraphqlApi", err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyGraphqlAPI: api})
}

// deleteGraphqlAPI handles DELETE /v1/apis/{apiId}.
func (h *Handler) deleteGraphqlAPI(ctx context.Context, c *echo.Context, apiID string) error {
	if err := h.Backend.DeleteGraphqlAPI(apiID); err != nil {
		return h.handleError(ctx, c, "DeleteGraphqlApi", err)
	}

	return c.NoContent(http.StatusNoContent)
}

// handleGraphQL handles POST /v1/apis/{apiId}/graphql — the GraphQL execution endpoint.
func (h *Handler) handleGraphQL(ctx context.Context, c *echo.Context, apiID string) error {
	if c.Request().Method != http.MethodPost {
		return c.JSON(http.StatusMethodNotAllowed, errorResponse("MethodNotAllowed", "method not allowed"))
	}

	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse("InternalFailure", err.Error()))
	}

	req, parseErr := parseGraphQLRequest(body)
	if parseErr != nil {
		return c.JSON(http.StatusBadRequest, graphqlResponse{
			Errors: []graphqlError{{Message: parseErr.Error()}},
		})
	}

	auth := GraphQLAuth{
		APIKey:    c.Request().Header.Get("X-Api-Key"),
		AuthToken: c.Request().Header.Get("Authorization"),
		Request:   c.Request(),
	}

	result, execErr := h.Backend.ExecuteGraphQL(ctx, apiID, req.Query, req.OperationName, req.Variables, auth)
	if execErr != nil {
		if errors.Is(execErr, ErrUnauthorized) {
			// Real AppSync: 401 with a plain {"message":"Unauthorized"} body for a
			// transport-level auth failure, distinct from the 200 + GraphQL
			// errors[].errorType "Unauthorized" shape used for resolver-level
			// (@aws_auth field) authorization failures.
			return c.JSON(http.StatusUnauthorized, map[string]string{"message": ErrUnauthorized.Error()})
		}

		return c.JSON(http.StatusOK, graphqlResponse{
			Errors: []graphqlError{{Message: execErr.Error()}},
		})
	}

	return c.JSON(http.StatusOK, graphqlResponse{Data: result})
}

// updateGraphqlAPI handles PATCH /v1/apis/{apiId}.
func (h *Handler) updateGraphqlAPI(ctx context.Context, c *echo.Context, apiID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse("InternalFailure", err.Error()))
	}

	var input struct {
		UserPoolConfig                    *UserPoolConfig                    `json:"userPoolConfig"`
		OpenIDConnectConfig               *OpenIDConnectConfig               `json:"openIDConnectConfig"`
		LambdaAuthorizerConfig            *LambdaAuthorizerConfig            `json:"lambdaAuthorizerConfig"`
		LogConfig                         *LogConfig                         `json:"logConfig"`
		XrayEnabled                       *bool                              `json:"xrayEnabled"`
		Name                              string                             `json:"name"`
		AuthenticationType                string                             `json:"authenticationType"`
		Visibility                        string                             `json:"visibility"`
		IntrospectionConfig               string                             `json:"introspectionConfig"`
		OwnerContact                      string                             `json:"ownerContact"`
		AdditionalAuthenticationProviders []AdditionalAuthenticationProvider `json:"additionalAuthenticationProviders"`
		QueryDepthLimit                   int32                              `json:"queryDepthLimit"`
		ResolverCountLimit                int32                              `json:"resolverCountLimit"`
	}

	if jsonErr := json.Unmarshal(body, &input); jsonErr != nil {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", "invalid request body"))
	}

	cfg := &GraphqlAPIConfig{
		UserPoolConfig:         input.UserPoolConfig,
		OpenIDConnectConfig:    input.OpenIDConnectConfig,
		LambdaAuthorizerConfig: input.LambdaAuthorizerConfig,
		LogConfig:              input.LogConfig,
		IntrospectionConfig:    input.IntrospectionConfig,
		OwnerContact:           input.OwnerContact,
		QueryDepthLimit:        input.QueryDepthLimit,
		ResolverCountLimit:     input.ResolverCountLimit,
	}

	api, updateErr := h.Backend.UpdateGraphqlAPI(
		apiID,
		input.Name,
		AuthenticationType(input.AuthenticationType),
		input.XrayEnabled,
		input.Visibility,
		input.AdditionalAuthenticationProviders,
		cfg,
	)
	if updateErr != nil {
		return h.handleError(ctx, c, "UpdateGraphqlApi", updateErr)
	}

	return c.JSON(http.StatusOK, map[string]any{keyGraphqlAPI: api})
}

// handleEnvironmentVariables handles GET and PUT /v1/apis/{apiId}/environmentVariables.
func (h *Handler) handleEnvironmentVariables(ctx context.Context, c *echo.Context, apiID string) error {
	switch c.Request().Method {
	case http.MethodGet:
		envVars, err := h.Backend.GetGraphqlAPIEnvironmentVariables(apiID)
		if err != nil {
			return h.handleError(ctx, c, "GetGraphqlApiEnvironmentVariables", err)
		}

		return c.JSON(http.StatusOK, map[string]any{keyEnvironmentVariables: envVars})
	case http.MethodPut:
		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorResponse("InternalFailure", err.Error()))
		}

		var input struct {
			EnvironmentVariables map[string]string `json:"environmentVariables"`
		}

		if jsonErr := json.Unmarshal(body, &input); jsonErr != nil {
			return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", "invalid request body"))
		}

		envVars, putErr := h.Backend.PutGraphqlAPIEnvironmentVariables(apiID, input.EnvironmentVariables)
		if putErr != nil {
			return h.handleError(ctx, c, "PutGraphqlApiEnvironmentVariables", putErr)
		}

		return c.JSON(http.StatusOK, map[string]any{keyEnvironmentVariables: envVars, "apiId": apiID})
	default:
		return c.JSON(http.StatusMethodNotAllowed, errorResponse("MethodNotAllowed", "method not allowed"))
	}
}
