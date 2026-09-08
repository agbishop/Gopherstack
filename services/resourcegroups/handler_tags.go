package resourcegroups

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

const (
	keyArn = "Arn"
)

// tagResourceInput is the JSON request body for tagging a resource.
type tagResourceInput struct {
	Tags map[string]string `json:"Tags"`
}

// untagResourceInput is the JSON request body for untagging a resource.
type untagResourceInput struct {
	Keys []string `json:"Keys"`
}

// handleTagRequest handles PUT /resources/{Arn}/tags (Tag operation).
func (h *Handler) handleTagRequest(ctx context.Context, c *echo.Context, log *slog.Logger, resourceARN string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		log.ErrorContext(ctx, "failed to read Tag request body", "error", err)

		return h.handleError(ctx, c, "Tag", err)
	}

	var in tagResourceInput

	if err = json.Unmarshal(body, &in); err != nil {
		return h.handleError(ctx, c, "Tag", errInvalidRequest)
	}

	tagMap, err := h.Backend.AddTagsByARN(ctx, resourceARN, in.Tags)
	if err != nil {
		return h.handleError(ctx, c, "Tag", err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyArn: resourceARN,
		"Tags": tagMap,
	})
}

// handleUntagRequest handles DELETE /resources/{Arn}/tags (Untag operation).
// Keys may come from query params or request body.
func (h *Handler) handleUntagRequest(ctx context.Context, c *echo.Context, log *slog.Logger, resourceARN string) error {
	keys, err := h.extractUntagKeys(ctx, c, log)
	if err != nil {
		return h.handleError(ctx, c, "Untag", err)
	}

	if err = h.Backend.RemoveTagsByARN(ctx, resourceARN, keys); err != nil {
		return h.handleError(ctx, c, "Untag", err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyArn: resourceARN,
		"Keys": keys,
	})
}

// extractUntagKeys parses tag keys from query params or body for the Untag
// operation, returning the raw unwritten error so handleUntagRequest can map
// and write it exactly once. It used to write its own error body via
// h.handleError and return that (always-nil, on a successful write) result
// directly; handleUntagRequest tested that nil and fell through to call
// Backend.RemoveTagsByARN with keys == nil, writing a second body on top of
// the already-committed one (gopherstack-7opw, the gopherstack-8haq shape).
func (h *Handler) extractUntagKeys(ctx context.Context, c *echo.Context, log *slog.Logger) ([]string, error) {
	keysParam := c.Request().URL.Query().Get("keys")
	if keysParam != "" {
		return strings.Split(keysParam, ","), nil
	}

	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		log.ErrorContext(ctx, "failed to read Untag request body", "error", err)

		return nil, err
	}

	if len(body) == 0 {
		return nil, nil
	}

	var in untagResourceInput
	if err = json.Unmarshal(body, &in); err != nil {
		return nil, errInvalidRequest
	}

	return in.Keys, nil
}

// handleResourceTags routes GET/PUT/DELETE/PATCH /resources/{Arn}/tags to the
// GetTags, Tag, and Untag operations respectively.
func (h *Handler) handleResourceTags(ctx context.Context, c *echo.Context) error {
	resourceARN := arnFromResourceTagsPath(c.Request().URL.Path)
	log := logger.Load(ctx)

	switch c.Request().Method {
	case http.MethodGet:
		tagMap, err := h.Backend.GetTagsByARN(ctx, resourceARN)
		if err != nil {
			return h.handleError(ctx, c, "GetTags", err)
		}

		return c.JSON(http.StatusOK, map[string]any{
			keyArn: resourceARN,
			"Tags": tagMap,
		})

	case http.MethodPut:
		return h.handleTagRequest(ctx, c, log, resourceARN)

	case http.MethodDelete:
		return h.handleUntagRequest(ctx, c, log, resourceARN)

	case http.MethodPatch:
		// PATCH kept as compat alias for existing tests; AWS uses DELETE.
		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "failed to read Untag request body", "error", err)

			return h.handleError(ctx, c, "Untag", err)
		}

		var in untagResourceInput

		if err = json.Unmarshal(body, &in); err != nil {
			return h.handleError(ctx, c, "Untag", errInvalidRequest)
		}

		if err = h.Backend.RemoveTagsByARN(ctx, resourceARN, in.Keys); err != nil {
			return h.handleError(ctx, c, "Untag", err)
		}

		return c.JSON(http.StatusOK, map[string]any{
			keyArn: resourceARN,
			"Keys": in.Keys,
		})

	default:
		return c.NoContent(http.StatusMethodNotAllowed)
	}
}
