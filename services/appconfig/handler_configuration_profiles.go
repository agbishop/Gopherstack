package appconfig

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

func (h *Handler) handleCreateConfigurationProfile(c *echo.Context, applicationID string) error {
	var req struct {
		Tags             map[string]string `json:"Tags"`
		Name             string            `json:"Name"`
		Description      string            `json:"Description"`
		LocationURI      string            `json:"LocationUri"`
		Type             string            `json:"Type"`
		RetrievalRoleArn string            `json:"RetrievalRoleArn"`
		KmsKeyIdentifier string            `json:"KmsKeyIdentifier"`
		Validators       []Validator       `json:"Validators"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(
			http.StatusBadRequest,
			map[string]string{keyMessageField: errInvalidRequestBody},
		)
	}

	profile, err := h.Backend.CreateConfigurationProfile(
		applicationID,
		req.Name,
		req.Description,
		req.LocationURI,
		req.Type,
		req.RetrievalRoleArn,
		req.KmsKeyIdentifier,
		req.Validators,
		req.Tags,
	)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return notFoundResponse(c, err)
		}

		// CreateConfigurationProfile models only BadRequestException,
		// InternalServerException, ResourceNotFoundException and
		// ServiceQuotaExceededException (appconfig@v1.48.4 deserializers.go:267) --
		// no ConflictException, so a name collision maps to BadRequestException here.
		if errors.Is(err, awserr.ErrAlreadyExists) || errors.Is(err, awserr.ErrInvalidParameter) {
			return badRequestResponse(c, err)
		}

		return internalServerErrorResponse(c, err)
	}

	return c.JSON(http.StatusCreated, profile)
}

func (h *Handler) handleGetConfigurationProfile(
	c *echo.Context,
	applicationID, profileID string,
) error {
	profile, err := h.Backend.GetConfigurationProfile(applicationID, profileID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return notFoundResponse(c, err)
		}

		return internalServerErrorResponse(c, err)
	}

	return c.JSON(http.StatusOK, profile)
}

func (h *Handler) handleListConfigurationProfiles(c *echo.Context, applicationID string) error {
	nextToken, maxResults := appConfigPaginationParams(c)
	profileType := c.Request().URL.Query().Get("type")
	profiles, outToken, err := h.Backend.ListConfigurationProfiles(
		applicationID,
		nextToken,
		profileType,
		maxResults,
	)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return notFoundResponse(c, err)
		}

		return internalServerErrorResponse(c, err)
	}

	summaries := make([]ConfigurationProfileSummary, 0, len(profiles))
	for _, p := range profiles {
		summaries = append(summaries, configurationProfileToSummary(p))
	}

	resp := map[string]any{keyItems: summaries}
	if outToken != "" {
		resp["NextToken"] = outToken
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleUpdateConfigurationProfile(
	c *echo.Context,
	applicationID, profileID string,
) error {
	var req struct {
		Name             *string      `json:"Name"`
		Description      *string      `json:"Description"`
		RetrievalRoleArn *string      `json:"RetrievalRoleArn"`
		KmsKeyIdentifier *string      `json:"KmsKeyIdentifier"`
		Validators       *[]Validator `json:"Validators"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(
			http.StatusBadRequest,
			map[string]string{keyMessageField: errInvalidRequestBody},
		)
	}

	profile, err := h.Backend.UpdateConfigurationProfile(
		applicationID,
		profileID,
		req.Name,
		req.Description,
		req.RetrievalRoleArn,
		req.KmsKeyIdentifier,
		req.Validators,
	)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return notFoundResponse(c, err)
		}

		return internalServerErrorResponse(c, err)
	}

	return c.JSON(http.StatusOK, profile)
}

func (h *Handler) handleDeleteConfigurationProfile(
	c *echo.Context,
	applicationID, profileID string,
) error {
	if rejected, err := rejectInvalidDeletionProtectionCheck(c); rejected {
		return err
	}

	if err := h.Backend.DeleteConfigurationProfile(applicationID, profileID); err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return notFoundResponse(c, err)
		}

		return internalServerErrorResponse(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}
