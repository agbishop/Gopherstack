// table_api.go implements Azure Cosmos DB's Table API: a second wire surface
// served from the same fixed Gateway port as Core/SQL (8081, see settings.go
// and AZURE.md section 9's M6 milestone), wire-identical to
// services/azuretable's own OData/JSON entity model and $filter grammar --
// differing only in auth (Cosmos's existing master-key HMAC, unchanged --
// see masterkey.go and checkAuth in handler.go) and the fact that it needs
// no separate account path segment (real Cosmos Table API disambiguates
// Table vs. Core/SQL traffic by path shape on one hostname, not by an
// account-name path prefix the way services/azuretable's Azurite-style
// addressing does).
//
// The entity CRUD/$filter engine itself is NOT reimplemented or duplicated
// here: both this file and services/azuretable import the same
// pkgs/odatatable package (see that package's doc comment for the extraction
// this milestone performed). This file and table_api_ops.go are a thin wire
// adapter over it, mirroring services/azuretable's handler.go/table_ops.go/
// entity_ops.go split as closely as the two services' differing resource
// hierarchies allow.
//
// $batch (multipart/mixed changesets) remains deferred, matching Table
// Storage's own M2 deferral -- see PARITY.md.

package cosmosdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/odatatable"
)

// tableAPIAccountName is the fake account name used to build a fullmetadata
// "odata.type" value in Table API responses (e.g. "cosmosdb.Tables"). Real
// Cosmos DB's Table API endpoint carries the real account name in its
// hostname (https://<account>.table.cosmos.azure.com); since gopherstack
// collapses every account onto one fixed port with no per-account routing,
// this is a fixed, arbitrary placeholder -- exactly the role
// services/azuretable's devstoreAccountName plays for Azurite-style
// addressing.
const tableAPIAccountName = "cosmosdb"

// dataServiceVersion is the DataServiceVersion header value real Azure
// Table Storage (and Cosmos DB's Table API) sets on every response. See
// services/azuretable/handler.go's identical constant.
const dataServiceVersion = "3.0;"

// preferReturnNoContent is the Prefer request header value that makes a
// Create Table/Insert Entity respond 204 (with Preference-Applied echoed
// back) instead of 201 + body. See services/azuretable/table_ops.go's
// identical constant.
const preferReturnNoContent = "return-no-content"

// tablesResourceName is the fixed "Tables" collection resource segment used
// for table-CRUD operations (POST/GET /Tables, DELETE /Tables('name')).
const tablesResourceName = "Tables"

// batchResourceName is the fixed "$batch" resource segment. Batch
// (multipart/mixed changesets) is explicitly out of scope -- see
// handleTableBatch.
const batchResourceName = "$batch"

// mergeMethod is the literal, non-standard HTTP method aztables' generated
// client sends for a Merge Entity request. See
// services/azuretable/handler.go's identical constant.
const mergeMethod = "MERGE"

// xHTTPMethodOverrideHeader is the method-tunneling header some older .NET
// clients and HTTP proxies send instead of (or alongside) a literal MERGE
// method. See resolveTunneledMergeMethod.
const xHTTPMethodOverrideHeader = "X-Http-Method"

// tableAPIOperation name constants used for metrics (ExtractOperation) and
// GetSupportedOperations.
const (
	opListTables      = "ListTables"
	opCreateTable     = "CreateTable"
	opDeleteTable     = "DeleteTable"
	opInsertEntity    = "InsertEntity"
	opGetEntity       = "GetEntity"
	opQueryEntities   = "QueryEntities"
	opReplaceEntity   = "ReplaceEntity"
	opMergeEntity     = "MergeEntity"
	opDeleteEntity    = "DeleteEntity"
	opTableAPIBatch   = "TableAPIBatch"
	unknownTableAPIOp = "Unknown"
)

// tableAPISupportedOperations lists the Table API operations
// GetSupportedOperations appends to Core/SQL's own list.
func tableAPISupportedOperations() []string {
	return []string{
		opListTables, opCreateTable, opDeleteTable,
		opInsertEntity, opGetEntity, opQueryEntities, opReplaceEntity, opMergeEntity, opDeleteEntity,
		opTableAPIBatch,
	}
}

// isTableAPIPath reports whether path (the raw request URL path) addresses
// the Table API rather than Core/SQL's "/dbs/..." resource hierarchy. Every
// Table API resource -- "/Tables", "/Tables('name')", "/$batch", "/<table>",
// "/<table>()", or "/<table>(PartitionKey='..',RowKey='..')" -- is exactly
// one path segment (any "(...)" suffix is part of that same segment, not a
// second one), which is never true of a Core/SQL path: those are either the
// account root ("" or "/") or start with a literal "dbs" segment followed by
// at least one more. A single segment that happens to literally be "dbs" is
// therefore Core/SQL's (list/create databases), not Table API's -- excluded
// here so parseResourcePath's own dispatch still owns it.
func isTableAPIPath(path string) bool {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" || trimmed == "dbs" {
		return false
	}

	return !strings.Contains(trimmed, "/")
}

// tableResourceKind classifies a parsed Table API resource path segment.
type tableResourceKind int

const (
	tableResourceInvalid tableResourceKind = iota
	tableResourceBatch
	tableResourceTablesCollection
	tableResourceTablesItem
	tableResourceEntityCollection
	tableResourceEntityItem
)

// parseTableResource classifies resource (the single path segment
// isTableAPIPath matched) and extracts the table name and any parenthesized
// inner content. For tableResourceTablesItem, inner is the raw quoted
// table-name literal (e.g. "'foo'"); for tableResourceEntityItem, inner is
// the raw key predicate (e.g. "PartitionKey='p',RowKey='r'"); it is empty
// for every other kind. Mirrors services/azuretable/handler.go's
// parseResource exactly (minus the leading-account-segment split, which
// Table API on Cosmos doesn't have -- see isTableAPIPath's doc comment).
func parseTableResource(resource string) (tableResourceKind, string, string) {
	if resource == batchResourceName {
		return tableResourceBatch, "", ""
	}

	idx := strings.IndexByte(resource, '(')
	if idx == -1 {
		if resource == tablesResourceName {
			return tableResourceTablesCollection, resource, ""
		}

		return tableResourceEntityCollection, resource, ""
	}

	if !strings.HasSuffix(resource, ")") {
		return tableResourceInvalid, "", ""
	}

	name := resource[:idx]
	inner := resource[idx+1 : len(resource)-1]

	if name == tablesResourceName {
		return tableResourceTablesItem, name, inner
	}

	if inner == "" {
		return tableResourceEntityCollection, name, ""
	}

	return tableResourceEntityItem, name, inner
}

// resolveTunneledMergeMethod rewrites r.Method in place to mergeMethod when
// the request carries an X-Http-Method: MERGE override header. See
// services/azuretable/handler.go's identical function for the full
// rationale (including why the override is only honored for POST/PUT/PATCH).
func resolveTunneledMergeMethod(r *http.Request) {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return
	}

	if strings.EqualFold(r.Header.Get(xHTTPMethodOverrideHeader), mergeMethod) {
		r.Method = mergeMethod
	}
}

// handleTableAPI dispatches a Table API request (resource is the single
// path segment isTableAPIPath already matched) to its operation handler.
func (h *Handler) handleTableAPI(c *echo.Context, resource string) error {
	r := c.Request()
	resolveTunneledMergeMethod(r)

	c.Response().Header().Set("Dataserviceversion", dataServiceVersion)

	kind, name, inner := parseTableResource(resource)

	switch kind {
	case tableResourceBatch:
		return h.handleTableBatch(c)
	case tableResourceTablesCollection:
		return h.handleTablesCollection(c)
	case tableResourceTablesItem:
		return h.handleTablesItem(c, inner)
	case tableResourceEntityCollection:
		return h.handleEntityCollection(c, name)
	case tableResourceEntityItem:
		return h.handleEntityItem(c, name, inner)
	default:
		return h.writeTableError(c, http.StatusBadRequest, "InvalidUri",
			"The requested URI does not represent any resource on the server.")
	}
}

// tableAPIOperationFor determines the Table API operation name for a
// request, for metrics labeling. Mirrors handleTableAPI's dispatch without
// side effects. Returns ("", false) if path isn't a Table API path at all.
func tableAPIOperationFor(r *http.Request) (string, bool) {
	if !isTableAPIPath(r.URL.Path) {
		return "", false
	}

	trimmed := strings.Trim(r.URL.Path, "/")
	kind, _, _ := parseTableResource(trimmed)

	switch kind {
	case tableResourceBatch:
		return opTableAPIBatch, true
	case tableResourceTablesCollection:
		return tablesCollectionOperationFor(r.Method), true
	case tableResourceTablesItem:
		if r.Method == http.MethodDelete {
			return opDeleteTable, true
		}

		return unknownTableAPIOp, true
	case tableResourceEntityCollection:
		return entityCollectionOperationFor(r.Method), true
	case tableResourceEntityItem:
		return entityItemOperationFor(r.Method), true
	default:
		return unknownTableAPIOp, true
	}
}

func tablesCollectionOperationFor(method string) string {
	switch method {
	case http.MethodPost:
		return opCreateTable
	case http.MethodGet:
		return opListTables
	default:
		return unknownTableAPIOp
	}
}

func entityCollectionOperationFor(method string) string {
	switch method {
	case http.MethodPost:
		return opInsertEntity
	case http.MethodGet:
		return opQueryEntities
	default:
		return unknownTableAPIOp
	}
}

func entityItemOperationFor(method string) string {
	switch method {
	case http.MethodGet:
		return opGetEntity
	case http.MethodPut:
		return opReplaceEntity
	case http.MethodPatch, mergeMethod:
		return opMergeEntity
	case http.MethodDelete:
		return opDeleteEntity
	default:
		return unknownTableAPIOp
	}
}

func (h *Handler) handleTablesCollection(c *echo.Context) error {
	switch c.Request().Method {
	case http.MethodPost:
		return h.createTable(c)
	case http.MethodGet:
		return h.listTables(c)
	default:
		return h.writeTableError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

func (h *Handler) handleTablesItem(c *echo.Context, quotedName string) error {
	if c.Request().Method != http.MethodDelete {
		return h.writeTableError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}

	return h.deleteTable(c, quotedName)
}

func (h *Handler) handleEntityCollection(c *echo.Context, table string) error {
	switch c.Request().Method {
	case http.MethodPost:
		return h.insertEntity(c, table)
	case http.MethodGet:
		return h.queryEntities(c, table)
	default:
		return h.writeTableError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

func (h *Handler) handleEntityItem(c *echo.Context, table, keyPredicate string) error {
	partitionKey, rowKey, ok := odatatable.ParseEntityKeyPredicate(keyPredicate)
	if !ok {
		return h.writeTableError(c, http.StatusBadRequest, "InvalidInput",
			"The specified entity key predicate is invalid.")
	}

	switch c.Request().Method {
	case http.MethodGet:
		return h.getEntity(c, table, partitionKey, rowKey)
	case http.MethodPut:
		return h.replaceEntity(c, table, partitionKey, rowKey)
	case http.MethodPatch, mergeMethod:
		return h.mergeEntity(c, table, partitionKey, rowKey)
	case http.MethodDelete:
		return h.deleteEntity(c, table, partitionKey, rowKey)
	default:
		return h.writeTableError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

// handleTableBatch handles POST /$batch. Batch (multipart/mixed changesets)
// is explicitly out of scope for this milestone, matching Table Storage's
// own M2 deferral -- see PARITY.md's deferred section.
func (h *Handler) handleTableBatch(c *echo.Context) error {
	return h.writeTableError(c, http.StatusNotImplemented, "NotImplemented",
		"$batch (multipart/mixed changesets) is not implemented; see PARITY.md's deferred section.")
}

// --- Table CRUD ---

// createTableBody is the request body shape for POST /Tables.
type createTableBody struct {
	TableName string `json:"TableName"`
}

func (h *Handler) createTable(c *echo.Context) error {
	r := c.Request()

	body, err := httputils.ReadBody(r)
	if err != nil {
		return h.writeTableError(c, http.StatusInternalServerError, "InternalError", "Failed to read request body.")
	}

	var req createTableBody
	if unmarshalErr := json.Unmarshal(body, &req); unmarshalErr != nil {
		return h.writeTableError(c, http.StatusBadRequest, "InvalidInput", "The input is not valid.")
	}

	if req.TableName == "" {
		return h.writeTableError(c, http.StatusBadRequest, "InvalidInput", "TableName must not be empty.")
	}

	if createErr := h.TableBackend.CreateTable(req.TableName); createErr != nil {
		if errors.Is(createErr, odatatable.ErrTableAlreadyExists) {
			return h.writeTableError(c, http.StatusConflict, "TableAlreadyExists",
				"The table specified already exists.")
		}

		return h.writeTableError(c, http.StatusInternalServerError, "InternalError", createErr.Error())
	}

	if r.Header.Get("Prefer") == preferReturnNoContent {
		c.Response().Header().Set("Preference-Applied", preferReturnNoContent)

		return c.NoContent(http.StatusNoContent)
	}

	level := tableODataLevelFromAccept(r.Header.Get("Accept"))

	return h.writeTableJSON(c, http.StatusCreated, h.tableEntityBody(req.TableName, level))
}

func (h *Handler) listTables(c *echo.Context) error {
	infos := h.TableBackend.ListTables()
	level := tableODataLevelFromAccept(c.Request().Header.Get("Accept"))

	values := make([]map[string]any, 0, len(infos))
	for _, ti := range infos {
		values = append(values, h.tableEntityBody(ti.Name, level))
	}

	return h.writeTableJSON(c, http.StatusOK, map[string]any{"value": values})
}

func (h *Handler) deleteTable(c *echo.Context, quotedName string) error {
	name, ok := odatatable.UnquoteODataString(quotedName)
	if !ok {
		return h.writeTableError(c, http.StatusBadRequest, "InvalidInput", "The specified table name is invalid.")
	}

	if err := h.TableBackend.DeleteTable(name); err != nil {
		return h.writeTableNotFoundError(c)
	}

	return c.NoContent(http.StatusNoContent)
}

// tableEntityBody builds a Table Storage/Table API table entity's OData
// JSON body, varying by metadata level, mirroring
// services/azuretable/table_ops.go's identical function.
func (h *Handler) tableEntityBody(name, level string) map[string]any {
	m := map[string]any{"TableName": name}

	if level == odatatable.MetadataLevelNoMetadata {
		return m
	}

	endpoint := h.tableServiceEndpoint()
	m["odata.metadata"] = endpoint + "/$metadata#Tables/@Element"

	if level == odatatable.MetadataLevelFull {
		m["odata.type"] = tableAPIAccountName + ".Tables"
		m["odata.id"] = endpoint + "/Tables('" + odatatable.EscapeODataKey(name) + "')"
		m["odata.editLink"] = "Tables('" + odatatable.EscapeODataKey(name) + "')"
	}

	return m
}

// tableServiceEndpoint returns the base URL used to build
// odata.metadata/odata.id values in Table API responses. Unlike
// services/azuretable's Handler, CosmosDB's Handler has no separately
// configurable Endpoint override (Core/SQL's own wire responses never embed
// a self-referential URL -- see container_ops.go's containerBody, which
// only ever emits a path-relative "_self") -- Table API's OData metadata
// fields need one, though, so this derives it from h.Port the same way
// services/azuretable's serviceEndpoint does when no override is set.
func (h *Handler) tableServiceEndpoint() string {
	return fmt.Sprintf("http://127.0.0.1:%d", h.Port)
}

// tableODataLevelFromAccept picks the OData metadata level from an Accept
// header value, defaulting to "minimalmetadata" (real Azure Table Storage's
// own default) when unspecified or unrecognized. Mirrors
// services/azuretable/handler.go's odataLevelFromAccept exactly.
func tableODataLevelFromAccept(accept string) string {
	switch {
	case strings.Contains(accept, "odata="+odatatable.MetadataLevelNoMetadata):
		return odatatable.MetadataLevelNoMetadata
	case strings.Contains(accept, "odata="+odatatable.MetadataLevelFull):
		return odatatable.MetadataLevelFull
	default:
		return odatatable.MetadataLevelMinimal
	}
}

// --- Table API OData wire error envelope ---
//
// {"odata.error":{"code":"TableNotFound","message":{"lang":"en-US","value":"..."}}}
//
// This is Azure Table Storage's own error envelope shape (Table API is
// wire-identical), distinct from Cosmos DB's Core/SQL cosmosErrorBody --
// each surface keeps its own real wire shape rather than forcing one onto
// the other.

type tableODataErrorMessage struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type tableODataErrorDetail struct {
	Message tableODataErrorMessage `json:"message"`
	Code    string                 `json:"code"`
}

type tableODataErrorEnvelope struct {
	Error tableODataErrorDetail `json:"odata.error"`
}

// writeTableJSON marshals v and writes it as the response body, with a
// Content-Type reflecting the request's negotiated OData metadata level.
func (h *Handler) writeTableJSON(c *echo.Context, status int, v any) error {
	level := tableODataLevelFromAccept(c.Request().Header.Get("Accept"))

	body, err := json.Marshal(v)
	if err != nil {
		return h.writeTableErrorNoRecurse(c, http.StatusInternalServerError, "InternalError",
			"Failed to marshal response.")
	}

	contentType := "application/json;odata=" + level + ";streaming=true;charset=utf-8"

	return c.Blob(status, contentType, body)
}

// writeTableError writes the standard Azure Table Storage JSON error
// envelope, plus the x-ms-error-code header real Azure Storage/Cosmos DB's
// Table API set on every error response.
func (h *Handler) writeTableError(c *echo.Context, status int, code, message string) error {
	c.Response().Header().Set("X-Ms-Error-Code", code)

	return h.writeTableJSON(c, status, tableODataErrorEnvelope{
		Error: tableODataErrorDetail{
			Code:    code,
			Message: tableODataErrorMessage{Lang: "en-US", Value: message},
		},
	})
}

// writeTableErrorNoRecurse is writeTableJSON's own marshal-failure fallback:
// it must not call back into writeTableJSON.
func (h *Handler) writeTableErrorNoRecurse(c *echo.Context, status int, code, message string) error {
	c.Response().Header().Set("X-Ms-Error-Code", code)
	body, _ := json.Marshal(tableODataErrorEnvelope{
		Error: tableODataErrorDetail{Code: code, Message: tableODataErrorMessage{Lang: "en-US", Value: message}},
	})

	return c.Blob(status, "application/json;odata=minimalmetadata;streaming=true;charset=utf-8", body)
}

// writeTableNotFoundError maps a missing-table TableBackend error to the
// corresponding Azure error code/status.
func (h *Handler) writeTableNotFoundError(c *echo.Context) error {
	return h.writeTableError(c, http.StatusNotFound, "TableNotFound", "The table specified does not exist.")
}
