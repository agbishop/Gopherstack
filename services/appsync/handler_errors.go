package appsync

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// handleError maps backend errors to appropriate HTTP responses.
func (h *Handler) handleError(ctx context.Context, c *echo.Context, op string, err error) error {
	log := logger.Load(ctx)
	log.ErrorContext(ctx, "AppSync operation failed", "operation", op, "error", err)

	if errors.Is(err, awserr.ErrNotFound) {
		return c.JSON(http.StatusNotFound, errorResponse("NotFoundException", err.Error()))
	}

	if errors.Is(err, awserr.ErrAlreadyExists) || errors.Is(err, awserr.ErrConflict) {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", err.Error()))
	}

	if errors.Is(err, ErrAPIKeyLimitExceeded) {
		return c.JSON(http.StatusBadRequest, errorResponse("ApiKeyLimitExceededException", err.Error()))
	}

	if errors.Is(err, ErrAPIKeyValidityOutOfBounds) {
		return c.JSON(http.StatusBadRequest, errorResponse("ApiKeyValidityOutOfBoundsException", err.Error()))
	}

	if errors.Is(err, ErrGraphQLSchemaInvalid) {
		return c.JSON(http.StatusBadRequest, errorResponse("GraphQLSchemaException", err.Error()))
	}

	if errors.Is(err, awserr.ErrInvalidParameter) {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", err.Error()))
	}

	if errors.Is(err, ErrInvalidSchema) {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", err.Error()))
	}

	if errors.Is(err, ErrValidation) {
		return c.JSON(http.StatusBadRequest, errorResponse("BadRequestException", err.Error()))
	}

	return c.JSON(
		http.StatusInternalServerError,
		errorResponse("InternalFailure", "internal error: "+err.Error()),
	)
}

// errorResponse builds a standard AppSync error response body.
func errorResponse(code, message string) map[string]any {
	return map[string]any{
		"message": message,
		keyCode:   code,
	}
}

// appsyncPaginate slices items using an integer-offset nextToken and returns the page
// and the token for the following page (empty string when exhausted).
// maxResults ≤ 0 means no limit; cap is applied by the caller.
func appsyncPaginate[T any](items []T, nextToken string, maxResults int) ([]T, string) {
	if len(items) == 0 {
		return items, ""
	}

	start := 0
	if nextToken != "" {
		if idx, err := strconv.Atoi(nextToken); err == nil && idx > 0 && idx < len(items) {
			start = idx
		}
	}

	if maxResults <= 0 {
		return items[start:], ""
	}

	end := start + maxResults
	if end >= len(items) {
		return items[start:], ""
	}

	return items[start:end], strconv.Itoa(end)
}
