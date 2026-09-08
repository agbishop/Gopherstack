package appconfig

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

func (h *Handler) handleCreateEnvironment(c *echo.Context, applicationID string) error {
	var req struct {
		Tags        map[string]string `json:"Tags"`
		Name        string            `json:"Name"`
		Description string            `json:"Description"`
		Monitors    []Monitor         `json:"Monitors"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(
			http.StatusBadRequest,
			map[string]string{keyMessageField: errInvalidRequestBody},
		)
	}

	env, err := h.Backend.CreateEnvironment(applicationID, req.Name, req.Description, req.Monitors, req.Tags)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return notFoundResponse(c, err)
		}

		// CreateEnvironment models only BadRequestException, InternalServerException,
		// ResourceNotFoundException and ServiceQuotaExceededException
		// (appconfig@v1.48.4 deserializers.go:767) -- no ConflictException, so a name
		// collision maps to BadRequestException here.
		if errors.Is(err, awserr.ErrAlreadyExists) || errors.Is(err, awserr.ErrInvalidParameter) {
			return badRequestResponse(c, err)
		}

		return internalServerErrorResponse(c, err)
	}

	return c.JSON(http.StatusCreated, environmentToOutput(*env))
}

func (h *Handler) handleGetEnvironment(c *echo.Context, applicationID, environmentID string) error {
	env, err := h.Backend.GetEnvironment(applicationID, environmentID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return notFoundResponse(c, err)
		}

		return internalServerErrorResponse(c, err)
	}

	return c.JSON(http.StatusOK, environmentToOutput(*env))
}

func (h *Handler) handleListEnvironments(c *echo.Context, applicationID string) error {
	nextToken, maxResults := appConfigPaginationParams(c)
	envs, outToken, err := h.Backend.ListEnvironments(applicationID, nextToken, maxResults)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return notFoundResponse(c, err)
		}

		return internalServerErrorResponse(c, err)
	}

	items := make([]environmentOutput, 0, len(envs))
	for _, env := range envs {
		items = append(items, environmentToOutput(env))
	}

	resp := map[string]any{keyItems: items}
	if outToken != "" {
		resp["NextToken"] = outToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleUpdateEnvironment(
	c *echo.Context,
	applicationID, environmentID string,
) error {
	var req struct {
		Name        *string    `json:"Name"`
		Description *string    `json:"Description"`
		Monitors    *[]Monitor `json:"Monitors"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(
			http.StatusBadRequest,
			map[string]string{keyMessageField: errInvalidRequestBody},
		)
	}

	env, err := h.Backend.UpdateEnvironment(
		applicationID, environmentID, req.Name, req.Description, req.Monitors,
	)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return notFoundResponse(c, err)
		}

		return internalServerErrorResponse(c, err)
	}

	return c.JSON(http.StatusOK, environmentToOutput(*env))
}

func (h *Handler) handleDeleteEnvironment(
	c *echo.Context,
	applicationID, environmentID string,
) error {
	if rejected, err := rejectInvalidDeletionProtectionCheck(c); rejected {
		return err
	}

	check := c.Request().Header.Get(deletionProtectionCheckHeader)
	if err := h.Backend.DeleteEnvironment(applicationID, environmentID, check); err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return notFoundResponse(c, err)
		}

		if errors.Is(err, awserr.ErrConflict) {
			return conflictResponse(c, err)
		}

		return internalServerErrorResponse(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}
