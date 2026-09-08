package azuretable

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/azureauth"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/odatatable"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
)

// azureTableVersion is the x-ms-version value echoed on every response.
// Unlike services/azurequeue's azureQueueVersion (a merely plausible
// version string), this one is picked deliberately: it is the literal
// x-ms-version azure-sdk-for-go/sdk/data/aztables's generated client sends
// on every request (see its zz_table_client.go), so matching it exactly
// avoids any version-skew surprises for that SDK.
const azureTableVersion = "2019-02-02"

// dataServiceVersion is the DataServiceVersion header value real Azure Table
// Storage (and Azurite) set on every response.
const dataServiceVersion = "3.0;"

// Operation name constants used for metrics (ExtractOperation) and
// GetSupportedOperations.
const (
	opListTables     = "ListTables"
	opCreateTable    = "CreateTable"
	opDeleteTable    = "DeleteTable"
	opInsertEntity   = "InsertEntity"
	opGetEntity      = "GetEntity"
	opQueryEntities  = "QueryEntities"
	opReplaceEntity  = "ReplaceEntity"
	opMergeEntity    = "MergeEntity"
	opDeleteEntity   = "DeleteEntity"
	opBatch          = "Batch"
	unknownOperation = "Unknown"
)

// tablesResourceName is the fixed "Tables" collection resource segment used
// for table-CRUD operations (POST/GET /<account>/Tables, DELETE
// /<account>/Tables('name')).
const tablesResourceName = "Tables"

// batchResourceName is the fixed "$batch" resource segment. Batch
// (multipart/mixed changesets) is explicitly out of scope -- see
// handleBatch.
const batchResourceName = "$batch"

// mergeMethod is the literal, non-standard HTTP method aztables' generated
// client actually sends for a Merge Entity request as of some historical
// client/proxy versions (net/http's http.Method* constants don't include it
// since it isn't a registered standard method). Echo's e.Any("/*", ...)
// route in StartWorker matches it like any other method.
const mergeMethod = "MERGE"

// xHTTPMethodOverrideHeader is the method-tunneling header some older .NET
// clients and HTTP proxies send instead of (or alongside) a literal MERGE
// method -- see resolveTunneledMergeMethod.
const xHTTPMethodOverrideHeader = "X-Http-Method"

// OData metadata level names, negotiated via the request's Accept header
// (odataLevelFromAccept) and used throughout table_ops.go/entity_ops.go to
// vary response shape.
const (
	odataLevelNoMetadata      = odatatable.MetadataLevelNoMetadata
	odataLevelMinimalMetadata = odatatable.MetadataLevelMinimal
	odataLevelFullMetadata    = odatatable.MetadataLevelFull
)

// Handler is the Echo HTTP handler for Azure Table Storage operations.
type Handler struct {
	Backend StorageBackend
	srvMu   *lockmetrics.RWMutex
	srv     *http.Server
	// Endpoint is e.g. "http://127.0.0.1:10002" -- used to build
	// odata.metadata/odata.id URLs in entity/table responses.
	Endpoint string
	// Port is the TCP port StartWorker binds. Set from Settings at Init time
	// (see provider.go); defaults to DefaultPort. Like services/azurequeue,
	// this is a single fixed, protocol-conventional port -- there is no
	// fallback pool, so StartWorker fails fast if it's unavailable rather
	// than silently binding a different port.
	Port int
}

// NewHandler creates a new Azure Table Handler. Port defaults to
// DefaultPort; callers (typically provider.go) override it from Settings.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{
		Backend: backend,
		Port:    DefaultPort,
		srvMu:   lockmetrics.New("azuretable.server"),
	}
}

var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
	_ service.Resettable       = (*Handler)(nil)
)

// Name returns the service name.
func (h *Handler) Name() string { return "AzureTable" }

// GetSupportedOperations returns the list of supported Azure Table operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opListTables,
		opCreateTable,
		opDeleteTable,
		opInsertEntity,
		opGetEntity,
		opQueryEntities,
		opReplaceEntity,
		opMergeEntity,
		opDeleteEntity,
		opBatch,
	}
}

// RouteMatcher exists only to satisfy service.Registerable's interface
// contract: like services/azureblob and services/azurequeue, AzureTable
// deliberately never matches on the shared AWS single-port Router. It runs
// on its own dedicated listener started by StartWorker (see provider.go for
// the full rationale). Only RouteMatcher itself is inert, kept so *Handler
// satisfies service.Registerable.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(*echo.Context) bool { return false }
}

// MatchPriority returns the routing priority for the AzureTable handler.
// Irrelevant in practice since RouteMatcher never matches; 0 (lowest) is
// the safe default.
func (h *Handler) MatchPriority() int { return 0 }

// ExtractOperation extracts the Azure Table operation name from the
// request, for metrics labeling.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return operationFor(c.Request())
}

// ExtractResource extracts the table/entity resource identifier from the
// request path, for metrics labeling.
func (h *Handler) ExtractResource(c *echo.Context) string {
	_, resource := splitPath(c.Request().URL.Path)

	return resource
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Handler returns the Echo handler function for Azure Table operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		resolveTunneledMergeMethod(r)

		h.setCommonHeaders(c)
		h.checkAuth(r)

		account, resource := splitPath(r.URL.Path)
		if account == "" || resource == "" {
			return h.writeError(c, http.StatusBadRequest, "InvalidUri",
				"The requested URI does not represent any resource on the server.")
		}

		kind, name, inner := parseResource(resource)

		switch kind {
		case resourceBatch:
			return h.handleBatch(c)
		case resourceTablesCollection:
			return h.handleTablesCollection(c)
		case resourceTablesItem:
			return h.handleTablesItem(c, inner)
		case resourceEntityCollection:
			return h.handleEntityCollection(c, name)
		case resourceEntityItem:
			return h.handleEntityItem(c, name, inner)
		default:
			return h.writeError(c, http.StatusBadRequest, "InvalidUri",
				"The requested URI does not represent any resource on the server.")
		}
	}
}

// checkAuth is intentionally permissive, mirroring services/azurequeue's
// checkAuth exactly: it neither requires nor cryptographically verifies the
// Authorization header, matching this repo's permissive-by-default auth
// philosophy (see services/s3/sigv4.go). Any structurally-present
// "SharedKey ..."/"SharedKeyLite ..." header, or its absence, is accepted.
func (h *Handler) checkAuth(r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return // anonymous; accepted by design at this milestone
	}

	if _, ok := azureauth.ParseAuthorizationHeader(authHeader); !ok {
		// Structurally malformed; still accepted at this milestone, but
		// logged so the gap is visible rather than silently swallowed.
		logger.Load(r.Context()).DebugContext(r.Context(), "azuretable: malformed Authorization header accepted")
	}
}

// setCommonHeaders sets the headers real Azure SDKs expect on every
// response, success or error.
func (h *Handler) setCommonHeaders(c *echo.Context) {
	hdr := c.Response().Header()
	hdr.Set("X-Ms-Version", azureTableVersion)
	hdr.Set("X-Ms-Request-Id", newRequestID())
	hdr.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	hdr.Set("Dataserviceversion", dataServiceVersion)
}

// newRequestID generates a plausible request-id (UUID-shaped, not
// cryptographically meaningful) for the x-ms-request-id header.
func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}

	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// splitPath splits an Azure Table REST path ("/<account>/<resource>") into
// its two components. <resource> is left unparsed here (see parseResource):
// it may be "Tables", "Tables('name')", "$batch", "<table>", "<table>()", or
// "<table>(PartitionKey='..',RowKey='..')".
func splitPath(p string) (string, string) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", ""
	}

	before, after, ok := strings.Cut(p, "/")
	if !ok {
		return p, ""
	}

	return before, after
}

// resourceKind classifies a parsed resource path segment.
type resourceKind int

const (
	resourceInvalid resourceKind = iota
	resourceBatch
	resourceTablesCollection
	resourceTablesItem
	resourceEntityCollection
	resourceEntityItem
)

// parseResource classifies resource (the path segment after the account
// name) and extracts the table name and any parenthesized inner content.
// For resourceTablesItem, inner is the raw quoted table-name literal (e.g.
// "'foo'"); for resourceEntityItem, inner is the raw key predicate (e.g.
// "PartitionKey='p',RowKey='r'"); it is empty for every other kind.
func parseResource(resource string) (resourceKind, string, string) {
	if resource == batchResourceName {
		return resourceBatch, "", ""
	}

	idx := strings.IndexByte(resource, '(')
	if idx == -1 {
		if resource == tablesResourceName {
			return resourceTablesCollection, resource, ""
		}

		return resourceEntityCollection, resource, ""
	}

	if !strings.HasSuffix(resource, ")") {
		return resourceInvalid, "", ""
	}

	name := resource[:idx]
	inner := resource[idx+1 : len(resource)-1]

	if name == tablesResourceName {
		return resourceTablesItem, name, inner
	}

	if inner == "" {
		return resourceEntityCollection, name, ""
	}

	return resourceEntityItem, name, inner
}

// resolveTunneledMergeMethod rewrites r.Method in place to mergeMethod when
// the request carries an X-Http-Method: MERGE override header -- the
// method-tunneling convention some older .NET clients and HTTP proxies use
// instead of (or alongside) sending a literal MERGE method, which
// entityItemOperationFor/handleEntityItem already handle directly. The
// override is honored ONLY when the actual method is POST, PUT, or PATCH:
// tunneling is never allowed to turn a GET or DELETE into something else,
// which would otherwise be an auth-bypass-shaped footgun (e.g. a client or
// intermediary smuggling a mutating MERGE past something that only
// authorizes GET). The header value is compared case-insensitively since
// HTTP header values for this convention are not reliably cased.
//
// Called once at the very top of Handler()'s returned func, before
// dispatch, so every downstream consumer of r.Method -- the dispatch
// switches in handleEntityItem/entityItemOperationFor, and
// ExtractOperation's metrics labeling via operationFor -- sees the resolved
// method uniformly without duplicating this check.
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

// operationFor determines the Azure Table operation name for a request, for
// metrics labeling. Mirrors the dispatch logic in Handler() without side
// effects.
func operationFor(r *http.Request) string {
	_, resource := splitPath(r.URL.Path)
	kind, _, _ := parseResource(resource)

	switch kind {
	case resourceBatch:
		return opBatch
	case resourceTablesCollection:
		return tablesCollectionOperationFor(r.Method)
	case resourceTablesItem:
		if r.Method == http.MethodDelete {
			return opDeleteTable
		}

		return unknownOperation
	case resourceEntityCollection:
		return entityCollectionOperationFor(r.Method)
	case resourceEntityItem:
		return entityItemOperationFor(r.Method)
	default:
		return unknownOperation
	}
}

func tablesCollectionOperationFor(method string) string {
	switch method {
	case http.MethodPost:
		return opCreateTable
	case http.MethodGet:
		return opListTables
	default:
		return unknownOperation
	}
}

func entityCollectionOperationFor(method string) string {
	switch method {
	case http.MethodPost:
		return opInsertEntity
	case http.MethodGet:
		return opQueryEntities
	default:
		return unknownOperation
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
		return unknownOperation
	}
}

// serviceEndpoint returns the base URL used to build odata.metadata/odata.id
// values in responses.
func (h *Handler) serviceEndpoint() string {
	if h.Endpoint != "" {
		return h.Endpoint
	}

	return fmt.Sprintf("http://127.0.0.1:%d", h.Port)
}

func (h *Handler) handleTablesCollection(c *echo.Context) error {
	switch c.Request().Method {
	case http.MethodPost:
		return h.createTable(c)
	case http.MethodGet:
		return h.listTables(c)
	default:
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

func (h *Handler) handleTablesItem(c *echo.Context, quotedName string) error {
	if c.Request().Method != http.MethodDelete {
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
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
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

func (h *Handler) handleEntityItem(c *echo.Context, table, keyPredicate string) error {
	partitionKey, rowKey, ok := parseEntityKeyPredicate(keyPredicate)
	if !ok {
		return h.writeError(c, http.StatusBadRequest, "InvalidInput",
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
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

// handleBatch handles POST /<account>/$batch. Batch (multipart/mixed
// changesets) is explicitly out of scope for this milestone -- see
// PARITY.md's deferred section -- so this returns a clean 501 with a message
// pointing there, rather than a confusing 404/400 that would suggest a
// routing bug instead of a deliberate scope decision.
func (h *Handler) handleBatch(c *echo.Context) error {
	return h.writeError(c, http.StatusNotImplemented, "NotImplemented",
		"$batch (multipart/mixed changesets) is not implemented; see PARITY.md's deferred section.")
}

// StartWorker binds the dedicated Table listener and starts serving on it.
// See provider.go's Provider doc comment for why AzureTable needs its own
// listener instead of registering into the shared AWS Router, and
// services/azurequeue's StartWorker for the synchronous-bind rationale this
// mirrors exactly.
func (h *Handler) StartWorker(ctx context.Context) error {
	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf(":%d", h.Port))
	if err != nil {
		return fmt.Errorf("azuretable: bind port %d: %w", h.Port, err)
	}

	e := echo.New()
	e.Use(logger.EchoMiddleware(logger.Load(ctx)))
	e.Any("/*", telemetry.WrapEchoHandler("AzureTable", h.Handler(), h))

	srv := &http.Server{
		Handler:           e,
		ReadHeaderTimeout: azureTableReadHeaderTimeout,
		ReadTimeout:       azureTableReadTimeout,
		IdleTimeout:       azureTableIdleTimeout,
	}

	h.srvMu.Lock("StartWorker")
	h.srv = srv
	h.srvMu.Unlock()

	workerCtx := logger.WithWorker(ctx, "azuretable", "listener")
	log := logger.Load(workerCtx)

	log.InfoContext(workerCtx, "azuretable: starting dedicated listener", "port", h.Port)

	go func() {
		if serveErr := srv.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.ErrorContext(workerCtx, "azuretable: listener stopped", "error", serveErr)
		}
	}()

	return nil
}

// Timeouts for the dedicated Table http.Server. See services/azureblob's
// identical constants for the ReadTimeout/IdleTimeout Slowloris rationale.
const (
	azureTableReadHeaderTimeout = 10 * time.Second
	azureTableReadTimeout       = 60 * time.Second
	azureTableIdleTimeout       = 120 * time.Second
)

// Shutdown stops the dedicated Table listener. A graceful Shutdown error
// (e.g. its context expiring before active connections finish) is logged
// and followed by Close, which forcibly closes the listener and any
// remaining idle/active connections; any Close error is logged too rather
// than leaving the listener to leak silently.
func (h *Handler) Shutdown(ctx context.Context) {
	h.srvMu.Lock("Shutdown")
	srv := h.srv
	h.srv = nil
	h.srvMu.Unlock()

	if srv == nil {
		return
	}

	log := logger.Load(ctx)

	if err := srv.Shutdown(ctx); err != nil {
		log.ErrorContext(ctx, "azuretable: graceful shutdown failed, forcing close", "error", err)

		if closeErr := srv.Close(); closeErr != nil {
			log.ErrorContext(ctx, "azuretable: forced close also failed", "error", closeErr)
		}
	}
}

// odataLevelFromAccept picks the OData metadata level from an Accept header
// value, defaulting to "minimalmetadata" (real Azure Table Storage's own
// default) when unspecified or unrecognized.
func odataLevelFromAccept(accept string) string {
	switch {
	case strings.Contains(accept, "odata="+odataLevelNoMetadata):
		return odataLevelNoMetadata
	case strings.Contains(accept, "odata="+odataLevelFullMetadata):
		return odataLevelFullMetadata
	default:
		return odataLevelMinimalMetadata
	}
}

// writeJSON marshals v and writes it as the response body, with a
// Content-Type reflecting the request's negotiated OData metadata level, per
// AZURE.md/PARITY.md's wire-protocol notes.
func (h *Handler) writeJSON(c *echo.Context, status int, v any) error {
	level := odataLevelFromAccept(c.Request().Header.Get("Accept"))

	body, err := json.Marshal(v)
	if err != nil {
		return h.writeErrorNoRecurse(c, http.StatusInternalServerError, "InternalError", "Failed to marshal response.")
	}

	contentType := fmt.Sprintf("application/json;odata=%s;streaming=true;charset=utf-8", level)

	return c.Blob(status, contentType, body)
}

// writeError writes the standard Azure Table Storage JSON error envelope,
// plus the x-ms-error-code header real Azure Storage sets on every error
// response.
func (h *Handler) writeError(c *echo.Context, status int, code, message string) error {
	c.Response().Header().Set("X-Ms-Error-Code", code)

	return h.writeJSON(c, status, odataErrorEnvelope{
		Error: odataErrorDetail{
			Code:    code,
			Message: odataErrorMessage{Lang: "en-US", Value: message},
		},
	})
}

// writeErrorNoRecurse is writeJSON's own marshal-failure fallback: it must
// not call back into writeJSON (which could recurse if the error envelope
// itself somehow failed to marshal, though it never does in practice since
// it's built from plain strings).
func (h *Handler) writeErrorNoRecurse(c *echo.Context, status int, code, message string) error {
	c.Response().Header().Set("X-Ms-Error-Code", code)
	body, _ := json.Marshal(odataErrorEnvelope{
		Error: odataErrorDetail{Code: code, Message: odataErrorMessage{Lang: "en-US", Value: message}},
	})

	return c.Blob(status, "application/json;odata=minimalmetadata;streaming=true;charset=utf-8", body)
}

// writeTableNotFoundError maps a StorageBackend not-found error to the
// corresponding Azure error code/status.
func (h *Handler) writeTableNotFoundError(c *echo.Context) error {
	return h.writeError(c, http.StatusNotFound, "TableNotFound", "The table specified does not exist.")
}
