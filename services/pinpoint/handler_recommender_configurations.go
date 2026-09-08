package pinpoint

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// extractRecommendersCollectionOp returns the operation for the recommenders collection.
func extractRecommendersCollectionOp(method string) string {
	switch method {
	case http.MethodPost:
		return "CreateRecommenderConfiguration"
	case http.MethodGet:
		return "GetRecommenderConfigurations"
	}

	return unknownOperation
}

// extractRecommenderResourceOp returns the operation for a specific recommender resource.
func extractRecommenderResourceOp(method, path string) string {
	recommenderID := strings.TrimPrefix(path, "/v1/recommenders/")
	if recommenderID == "" {
		return unknownOperation
	}

	switch method {
	case http.MethodGet:
		return "GetRecommenderConfiguration"
	case http.MethodPut:
		return "UpdateRecommenderConfiguration"
	case http.MethodDelete:
		return "DeleteRecommenderConfiguration"
	}

	return unknownOperation
}

// dispatchRecommenders routes /v1/recommenders requests (POST=create, GET=list).
func (h *Handler) dispatchRecommenders(c *echo.Context) error {
	switch c.Request().Method {
	case http.MethodPost:
		return h.handleCreateRecommenderConfiguration(c)
	case http.MethodGet:
		return h.handleGetRecommenderConfigurations(c)
	}

	return writeErrorResponse(
		c,
		http.StatusMethodNotAllowed,
		"MethodNotAllowedException",
		"method not allowed",
	)
}

func (h *Handler) dispatchRecommenderByID(c *echo.Context, recommenderID string) error {
	switch c.Request().Method {
	case http.MethodGet:
		return h.handleGetRecommenderConfiguration(c, recommenderID)
	case http.MethodPut:
		return h.handleUpdateRecommenderConfiguration(c, recommenderID)
	case http.MethodDelete:
		return h.handleDeleteRecommenderConfiguration(c, recommenderID)
	}

	return writeErrorResponse(
		c,
		http.StatusMethodNotAllowed,
		"MethodNotAllowedException",
		"method not allowed",
	)
}

// handleCreateRecommenderConfiguration handles POST /v1/recommenders.
func (h *Handler) handleCreateRecommenderConfiguration(c *echo.Context) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(
			c,
			http.StatusBadRequest,
			"BadRequestException",
			"failed to read request body",
		)
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req createRecommenderConfigRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(
			c,
			http.StatusBadRequest,
			"BadRequestException",
			"invalid request body",
		)
	}

	if strings.TrimSpace(req.RecommendationProviderRoleArn) == "" {
		return writeErrorResponse(
			c,
			http.StatusBadRequest,
			"BadRequestException",
			"RecommendationProviderRoleArn is required",
		)
	}

	if strings.TrimSpace(req.RecommendationProviderURI) == "" {
		return writeErrorResponse(
			c,
			http.StatusBadRequest,
			"BadRequestException",
			"RecommendationProviderUri is required",
		)
	}

	r, backendErr := h.Backend.CreateRecommenderConfiguration(req)
	if backendErr != nil {
		return writeErrorResponse(
			c,
			http.StatusInternalServerError,
			"InternalServerErrorException",
			backendErr.Error(),
		)
	}

	httputils.WriteJSON(
		c.Request().Context(),
		c.Response(),
		http.StatusCreated,
		recommenderConfigResponse{
			Attributes:                    r.Attributes,
			ID:                            r.ID,
			Name:                          r.Name,
			Description:                   r.Description,
			RecommendationProviderIDType:  r.RecommendationProviderIDType,
			RecommendationProviderRoleArn: r.RecommendationProviderRoleARN,
			RecommendationProviderURI:     r.RecommendationProviderURI,
			RecommendationsPerMessage:     r.RecommendationsPerMessage,
			CreationDate:                  r.CreationDate,
			LastModifiedDate:              r.LastModifiedDate,
		},
	)

	return nil
}

// ──────────────────────────────────────────────────
// Sub-resource dispatch helpers
// ──────────────────────────────────────────────────

// handleGetRecommenderConfiguration handles GET /v1/recommenders/{recommenderId}.
func (h *Handler) handleGetRecommenderConfiguration(c *echo.Context, recommenderID string) error {
	r, err := h.Backend.GetRecommenderConfiguration(recommenderID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(
			c,
			http.StatusInternalServerError,
			"InternalServerErrorException",
			err.Error(),
		)
	}

	httputils.WriteJSON(
		c.Request().Context(),
		c.Response(),
		http.StatusOK,
		toRecommenderConfigResponse(r),
	)

	return nil
}

// handleGetRecommenderConfigurations handles GET /v1/recommenders.
func (h *Handler) handleGetRecommenderConfigurations(c *echo.Context) error {
	recommenders, err := h.Backend.GetRecommenderConfigurations()
	if err != nil {
		return writeErrorResponse(
			c,
			http.StatusInternalServerError,
			"InternalServerErrorException",
			err.Error(),
		)
	}

	items := make([]recommenderConfigResponse, 0, len(recommenders))

	for _, r := range recommenders {
		items = append(items, toRecommenderConfigResponse(r))
	}

	httputils.WriteJSON(
		c.Request().Context(),
		c.Response(),
		http.StatusOK,
		recommenderConfigsListResponse{Item: items},
	)

	return nil
}

// handleUpdateRecommenderConfiguration handles PUT /v1/recommenders/{recommenderId}.
func (h *Handler) handleUpdateRecommenderConfiguration(
	c *echo.Context,
	recommenderID string,
) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return writeErrorResponse(
			c,
			http.StatusBadRequest,
			"BadRequestException",
			"failed to read request body",
		)
	}

	if !checkPayloadSize(c, body, maxInvocationPayloadBytes) {
		return nil
	}

	var req createRecommenderConfigRequest
	if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
		return writeErrorResponse(
			c,
			http.StatusBadRequest,
			"BadRequestException",
			"invalid request body",
		)
	}

	r, backendErr := h.Backend.UpdateRecommenderConfiguration(recommenderID, req)
	if backendErr != nil {
		if errors.Is(backendErr, awserr.ErrNotFound) {
			return writeErrorResponse(
				c,
				http.StatusNotFound,
				"NotFoundException",
				backendErr.Error(),
			)
		}

		return writeErrorResponse(
			c,
			http.StatusInternalServerError,
			"InternalServerErrorException",
			backendErr.Error(),
		)
	}

	httputils.WriteJSON(
		c.Request().Context(),
		c.Response(),
		http.StatusOK,
		toRecommenderConfigResponse(r),
	)

	return nil
}

// handleDeleteRecommenderConfiguration handles DELETE /v1/recommenders/{recommenderId}.
func (h *Handler) handleDeleteRecommenderConfiguration(
	c *echo.Context,
	recommenderID string,
) error {
	r, err := h.Backend.DeleteRecommenderConfiguration(recommenderID)
	if err != nil {
		if errors.Is(err, awserr.ErrNotFound) {
			return writeErrorResponse(c, http.StatusNotFound, "NotFoundException", err.Error())
		}

		return writeErrorResponse(
			c,
			http.StatusInternalServerError,
			"InternalServerErrorException",
			err.Error(),
		)
	}

	httputils.WriteJSON(
		c.Request().Context(),
		c.Response(),
		http.StatusOK,
		toRecommenderConfigResponse(r),
	)

	return nil
}

// ──────────────────────────────────────────────────
// Job list handlers
// ──────────────────────────────────────────────────

func toRecommenderConfigResponse(r *RecommenderConfiguration) recommenderConfigResponse {
	return recommenderConfigResponse{
		Attributes:                    r.Attributes,
		ID:                            r.ID,
		Name:                          r.Name,
		Description:                   r.Description,
		RecommendationProviderIDType:  r.RecommendationProviderIDType,
		RecommendationProviderRoleArn: r.RecommendationProviderRoleARN,
		RecommendationProviderURI:     r.RecommendationProviderURI,
		RecommendationsPerMessage:     r.RecommendationsPerMessage,
		CreationDate:                  r.CreationDate,
		LastModifiedDate:              r.LastModifiedDate,
	}
}
