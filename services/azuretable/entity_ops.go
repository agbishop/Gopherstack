package azuretable

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/odatatable"
)

// devstoreAccountName is Azurite's well-known development storage account
// name, used to build odata.type values in fullmetadata responses (e.g.
// "devstoreaccount1.Tables"). See AZURE.md section 5 and pkgs/azureauth.
const devstoreAccountName = "devstoreaccount1"

// --- Path/key-predicate parsing ---
//
// The actual parsing logic lives in pkgs/odatatable (see interfaces.go's
// package doc comment); these thin wrappers keep handler.go's/table_ops.go's
// call sites referencing the same unexported names they always have.

func parseEntityKeyPredicate(predicate string) (string, string, bool) {
	return odatatable.ParseEntityKeyPredicate(predicate)
}

func unquoteODataString(s string) (string, bool) {
	return odatatable.UnquoteODataString(s)
}

func escapeODataKey(s string) string {
	return odatatable.EscapeODataKey(s)
}

// --- HTTP handlers ---

func (h *Handler) encodeEntity(info EntityInfo, table, level, selectParam string) map[string]any {
	return odatatable.EncodeEntity(info, table, level, selectParam, h.serviceEndpoint(), devstoreAccountName)
}

func (h *Handler) insertEntity(c *echo.Context, table string) error {
	r := c.Request()

	body, err := httputils.ReadBody(r)
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "InternalError", "Failed to read request body.")
	}

	partitionKey, rowKey, hasPK, hasRK, props, decErr := odatatable.DecodeEntityBody(body)
	if decErr != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidInput", "One of the request inputs is not valid.")
	}

	if !hasPK || !hasRK {
		return h.writeError(c, http.StatusBadRequest, "InvalidInput",
			"The values are not specified for all the key properties of the entity.")
	}

	info, err := h.Backend.InsertEntity(table, partitionKey, rowKey, props)

	switch {
	case err == nil:
	case errors.Is(err, ErrTableNotFound):
		return h.writeTableNotFoundError(c)
	case errors.Is(err, ErrEntityAlreadyExists):
		return h.writeError(c, http.StatusConflict, "EntityAlreadyExists", "The specified entity already exists.")
	default:
		return h.writeError(c, http.StatusInternalServerError, "InternalError", err.Error())
	}

	c.Response().Header().Set("ETag", info.ETag)

	if r.Header.Get("Prefer") == preferReturnNoContent {
		c.Response().Header().Set("Preference-Applied", preferReturnNoContent)

		return c.NoContent(http.StatusNoContent)
	}

	level := odataLevelFromAccept(r.Header.Get("Accept"))

	return h.writeJSON(c, http.StatusCreated, h.encodeEntity(info, table, level, ""))
}

func (h *Handler) getEntity(c *echo.Context, table, partitionKey, rowKey string) error {
	info, err := h.Backend.GetEntity(table, partitionKey, rowKey)

	switch {
	case err == nil:
	case errors.Is(err, ErrTableNotFound):
		return h.writeTableNotFoundError(c)
	case errors.Is(err, ErrEntityNotFound):
		return h.writeResourceNotFoundError(c)
	default:
		return h.writeError(c, http.StatusInternalServerError, "InternalError", err.Error())
	}

	c.Response().Header().Set("ETag", info.ETag)

	level := odataLevelFromAccept(c.Request().Header.Get("Accept"))

	return h.writeJSON(c, http.StatusOK, h.encodeEntity(info, table, level, c.QueryParam("$select")))
}

func (h *Handler) queryEntities(c *echo.Context, table string) error {
	top, topErr := odatatable.ParseTop(c.QueryParam("$top"))
	if topErr != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidInput", "The value for $top is invalid.")
	}

	var filter Node

	if filterParam := c.QueryParam("$filter"); filterParam != "" {
		node, parseErr := ParseFilter(filterParam)
		if parseErr != nil {
			return h.writeError(c, http.StatusBadRequest, "InvalidInput", "The specified $filter is invalid.")
		}

		filter = node
	}

	infos, err := h.Backend.QueryEntities(table, filter, top)
	if err != nil {
		return h.writeTableNotFoundError(c)
	}

	level := odataLevelFromAccept(c.Request().Header.Get("Accept"))
	selectParam := c.QueryParam("$select")

	values := make([]map[string]any, 0, len(infos))
	for _, info := range infos {
		values = append(values, h.encodeEntity(info, table, level, selectParam))
	}

	return h.writeJSON(c, http.StatusOK, map[string]any{"value": values})
}

func (h *Handler) replaceEntity(c *echo.Context, table, partitionKey, rowKey string) error {
	return h.putOrMergeEntity(c, table, partitionKey, rowKey, h.Backend.ReplaceEntity)
}

func (h *Handler) mergeEntity(c *echo.Context, table, partitionKey, rowKey string) error {
	return h.putOrMergeEntity(c, table, partitionKey, rowKey, h.Backend.MergeEntity)
}

// mutateEntityFunc is the shape ReplaceEntity/MergeEntity share, so
// putOrMergeEntity can dispatch to either one generically.
type mutateEntityFunc func(
	table, partitionKey, rowKey string, props map[string]EntityProperty, ifMatch string,
) (EntityInfo, error)

func (h *Handler) putOrMergeEntity(
	c *echo.Context, table, partitionKey, rowKey string, mutate mutateEntityFunc,
) error {
	r := c.Request()

	body, err := httputils.ReadBody(r)
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "InternalError", "Failed to read request body.")
	}

	_, _, _, _, props, decErr := odatatable.DecodeEntityBody(body)
	if decErr != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidInput", "One of the request inputs is not valid.")
	}

	ifMatch := r.Header.Get("If-Match")

	info, mutateErr := mutate(table, partitionKey, rowKey, props, ifMatch)

	switch {
	case mutateErr == nil:
	case errors.Is(mutateErr, ErrTableNotFound):
		return h.writeTableNotFoundError(c)
	case errors.Is(mutateErr, ErrEntityNotFound):
		return h.writeResourceNotFoundError(c)
	case errors.Is(mutateErr, ErrETagMismatch):
		return h.writeError(c, http.StatusPreconditionFailed, "UpdateConditionNotSatisfied",
			"The update condition specified in the request was not satisfied.")
	default:
		return h.writeError(c, http.StatusInternalServerError, "InternalError", mutateErr.Error())
	}

	c.Response().Header().Set("ETag", info.ETag)

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) deleteEntity(c *echo.Context, table, partitionKey, rowKey string) error {
	ifMatch := c.Request().Header.Get("If-Match")
	if ifMatch == "" {
		return h.writeError(c, http.StatusBadRequest, "InvalidInput", "An If-Match header is required for delete.")
	}

	err := h.Backend.DeleteEntity(table, partitionKey, rowKey, ifMatch)

	switch {
	case err == nil:
		return c.NoContent(http.StatusNoContent)
	case errors.Is(err, ErrTableNotFound):
		return h.writeTableNotFoundError(c)
	case errors.Is(err, ErrEntityNotFound):
		return h.writeResourceNotFoundError(c)
	case errors.Is(err, ErrETagMismatch):
		return h.writeError(c, http.StatusPreconditionFailed, "UpdateConditionNotSatisfied",
			"The update condition specified in the request was not satisfied.")
	default:
		return h.writeError(c, http.StatusInternalServerError, "InternalError", err.Error())
	}
}

// writeResourceNotFoundError maps a missing-entity StorageBackend error to
// the corresponding Azure error code/status.
func (h *Handler) writeResourceNotFoundError(c *echo.Context) error {
	return h.writeError(c, http.StatusNotFound, "ResourceNotFound", "The specified resource does not exist.")
}
