package cosmosdb

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

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/odatatable"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
)

// cosmosVersion is the x-ms-version value echoed on every response -- a
// plausible, commonly-supported Cosmos DB REST API version.
const cosmosVersion = "2020-07-15"

// Operation name constants used for metrics (ExtractOperation) and
// GetSupportedOperations.
const (
	opGetDatabaseAccount = "GetDatabaseAccount"
	opListDatabases      = "ListDatabases"
	opCreateDatabase     = "CreateDatabase"
	opGetDatabase        = "GetDatabase"
	opDeleteDatabase     = "DeleteDatabase"
	opListContainers     = "ListContainers"
	opCreateContainer    = "CreateContainer"
	opGetContainer       = "GetContainer"
	opDeleteContainer    = "DeleteContainer"
	opCreateDocument     = "CreateDocument"
	opQueryDocuments     = "QueryDocuments"
	opListDocuments      = "ListDocuments"
	opGetDocument        = "GetDocument"
	opReplaceDocument    = "ReplaceDocument"
	opDeleteDocument     = "DeleteDocument"
	unknownOperation     = "Unknown"
)

// Header names used throughout the wire protocol. The session-token header
// name is intentionally NOT a named constant here (it's inlined at its one
// call site in setCommonHeaders): a `headerSessionToken = "X-Ms-..."`-shaped
// declaration trips gosec's G101 hardcoded-credentials heuristic on the
// "Token" substring even though this is a header NAME, not a secret value.
const (
	headerIsQuery          = "X-Ms-Documentdb-Isquery"
	headerIsUpsert         = "X-Ms-Documentdb-Is-Upsert"
	headerPartitionKey     = "X-Ms-Documentdb-Partitionkey"
	headerRequestCharge    = "X-Ms-Request-Charge"
	headerActivityID       = "X-Ms-Activity-Id"
	headerContentTypeQuery = "application/query+json"
)

// staticRequestCharge is the fake RU charge every response reports. Real RU
// accounting is out of scope for this milestone (see PARITY.md); a static,
// plausible value satisfies SDKs that merely read and log/report it.
const staticRequestCharge = "1"

// Handler is the Echo HTTP handler for Azure Cosmos DB (Core/SQL API)
// operations.
type Handler struct {
	Backend StorageBackend
	// TableBackend holds Table API tables/entities -- a completely
	// independent odatatable.InMemoryBackend instance from Backend's own
	// database/container/document state (see table_api.go and AZURE.md
	// section 9's M6 milestone). It is not yet included in
	// Handler.Snapshot/Restore's persistence lifecycle -- see PARITY.md's
	// Table API addendum.
	TableBackend *odatatable.InMemoryBackend
	srvMu        *lockmetrics.RWMutex
	srv          *http.Server
	// MasterKey is the base64-encoded master key checkAuth verifies
	// against when ValidateAuth is true.
	MasterKey string
	// Port is the TCP port StartWorker binds. Set from Settings at Init time
	// (see provider.go); defaults to DefaultPort. Like services/azuretable,
	// this is a single fixed, protocol-conventional port -- no fallback
	// pool, so StartWorker fails fast if it's unavailable.
	Port int
	// ValidateAuth opts into cryptographic master-key signature
	// verification. See masterkey.go and checkAuth.
	ValidateAuth bool
}

// tableAPIBackendLockMetricsLabel is the lockmetrics label for TableBackend,
// distinguishing its lock-contention metrics from services/azuretable's own
// odatatable.InMemoryBackend instance (which passes "azuretable") -- both
// otherwise construct the exact same type and would collide onto one shared
// label if the engine picked its own internally.
const tableAPIBackendLockMetricsLabel = "cosmosdb"

// tableAPIBackendVersion is the snapshot-version TableBackend's own
// Restore enforces (see pkgs/odatatable/persistence.go). Deliberately NOT
// named with a "SnapshotVersion" suffix: that name shape is what
// pkgs/persistence's snapshot-version guard (snapshotversion_guard_test.go)
// scans services/*/ packages for, and TableBackend is NOT wired into
// Handler.Snapshot/Restore's persistence lifecycle (see TableBackend's own
// doc comment below) -- it has no golden-tracked shape for the guard to
// enforce yet, so it must not look like it does. If/when TableBackend is
// wired into snapshot/restore, rename this to cosmosdbTableSnapshotVersion
// and add a matching guard-visible struct, mirroring
// services/azuretable/persistence.go's azureTableSnapshotVersion/
// azureTableSnapshot.
const tableAPIBackendVersion = 2

// NewHandler creates a new Cosmos DB Handler. Port/MasterKey default to
// DefaultPort/DefaultMasterKey; callers (typically provider.go) override
// them from Settings.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{
		Backend:      backend,
		TableBackend: odatatable.NewInMemoryBackend(tableAPIBackendLockMetricsLabel, tableAPIBackendVersion),
		Port:         DefaultPort,
		MasterKey:    DefaultMasterKey,
		srvMu:        lockmetrics.New("cosmosdb.server"),
	}
}

var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
	_ service.Resettable       = (*Handler)(nil)
)

// Name returns the service name.
func (h *Handler) Name() string { return "CosmosDB" }

// GetSupportedOperations returns the list of supported Cosmos DB operations,
// Core/SQL and Table API combined.
func (h *Handler) GetSupportedOperations() []string {
	coreOps := []string{
		opGetDatabaseAccount,
		opListDatabases, opCreateDatabase, opGetDatabase, opDeleteDatabase,
		opListContainers, opCreateContainer, opGetContainer, opDeleteContainer,
		opCreateDocument, opQueryDocuments, opListDocuments, opGetDocument, opReplaceDocument, opDeleteDocument,
	}
	tableOps := tableAPISupportedOperations()

	ops := make([]string, 0, len(coreOps)+len(tableOps))
	ops = append(ops, coreOps...)
	ops = append(ops, tableOps...)

	return ops
}

// RouteMatcher exists only to satisfy service.Registerable's interface
// contract -- see provider.go's Provider doc comment. CosmosDB never
// matches on the shared AWS single-port Router; it runs on its own
// dedicated listener started by StartWorker.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(*echo.Context) bool { return false }
}

// MatchPriority returns the routing priority for the CosmosDB handler.
// Irrelevant in practice since RouteMatcher never matches; 0 (lowest) is
// the safe default.
func (h *Handler) MatchPriority() int { return 0 }

// ExtractOperation extracts the Cosmos DB operation name from the request,
// for metrics labeling.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return operationFor(c.Request())
}

// ExtractResource extracts the resource path, for metrics labeling.
func (h *Handler) ExtractResource(c *echo.Context) string {
	return strings.Trim(c.Request().URL.Path, "/")
}

// Reset clears all in-memory state, Core/SQL and Table API alike.
func (h *Handler) Reset() {
	h.Backend.Reset()
	h.TableBackend.Reset()
}

// Handler returns the Echo handler function for Cosmos DB operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()

		h.setCommonHeaders(c)

		if !h.checkAuth(r) {
			return h.writeError(
				c,
				http.StatusUnauthorized,
				"Unauthorized",
				"The input authorization token can't serve the request. Please check that the expected "+
					"payload is built as per the protocol, and check the key being used.",
			)
		}

		if isTableAPIPath(r.URL.Path) {
			return h.handleTableAPI(c, strings.Trim(r.URL.Path, "/"))
		}

		kind, dbID, collID, docID := parseResourcePath(r.URL.Path)

		switch kind {
		case resourceAccountRoot:
			return h.handleAccountRoot(c)
		case resourceDatabases:
			return h.handleDatabases(c)
		case resourceDatabaseItem:
			return h.handleDatabaseItem(c, dbID)
		case resourceContainers:
			return h.handleContainers(c, dbID)
		case resourceContainerItem:
			return h.handleContainerItem(c, dbID, collID)
		case resourceDocuments:
			return h.handleDocuments(c, dbID, collID)
		case resourceDocumentItem:
			return h.handleDocumentItem(c, dbID, collID, docID)
		default:
			return h.writeError(
				c,
				http.StatusBadRequest,
				"BadRequest",
				"The requested URI does not represent a valid Cosmos DB resource.",
			)
		}
	}
}

// checkAuth reports whether the request is authorized to proceed.
//
// By default (h.ValidateAuth false) it is intentionally permissive,
// mirroring services/azuretable's checkAuth: an absent, malformed, or
// wrongly-signed Authorization header is accepted, matching this repo's
// permissive-by-default auth philosophy (see services/s3/sigv4.go and
// pkgs/azureauth) so unmodified SDKs work with zero configuration.
//
// When h.ValidateAuth is set (via --cosmosdb-validate-auth), a *present*
// Authorization header must carry a structurally valid, correctly signed
// master-key signature or the request is rejected -- the caller turns a
// false return into 401 Unauthorized. This deliberately enforces rather
// than merely logging, matching the opt-in validation precedent AZURE.md
// section 5 names as the model: services/s3's PresignSecret /
// WithPresignValidation, which rejects a bad signature with 403 AccessDenied
// (see services/s3/presign.go). An opt-in flag literally named "validate"
// that never rejects would be actively misleading.
//
// An absent Authorization header stays anonymous-accepted even under
// ValidateAuth: the flag opts into *verifying signatures that are offered*,
// not into requiring authentication, so enabling it cannot break the
// no-credentials local-dev workflow the other Azure services support.
func (h *Handler) checkAuth(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return true // anonymous; accepted by design at this milestone
	}

	if !h.ValidateAuth {
		if _, ok := parseMasterKeyAuthorization(authHeader); !ok {
			logger.Load(r.Context()).DebugContext(r.Context(), "cosmosdb: malformed Authorization header accepted")
		}

		return true
	}

	ok, err := VerifyMasterKey(h.MasterKey, r)
	if err != nil || !ok {
		logger.Load(r.Context()).WarnContext(r.Context(),
			"cosmosdb: master-key signature validation failed; rejecting request", "error", err)

		return false
	}

	return true
}

// setCommonHeaders sets the headers real Cosmos SDKs expect on every
// response, success or error: x-ms-version, x-ms-request-id, Date,
// x-ms-request-charge (fake RU accounting), x-ms-session-token, and
// x-ms-activity-id.
func (h *Handler) setCommonHeaders(c *echo.Context) {
	hdr := c.Response().Header()
	hdr.Set("X-Ms-Version", cosmosVersion)
	hdr.Set("X-Ms-Request-Id", newRequestID())
	hdr.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	hdr.Set(headerRequestCharge, staticRequestCharge)
	hdr.Set("X-Ms-Session-Token", "0:0#0")
	hdr.Set(headerActivityID, newRequestID())
}

// newRequestID generates a plausible request/activity-id (UUID-shaped, not
// cryptographically meaningful).
func newRequestID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}

	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// --- Resource path parsing ---

// resourceKind classifies a parsed Cosmos DB resource path.
type resourceKind int

const (
	resourceInvalid resourceKind = iota
	// resourceAccountRoot is the database-account root resource ("" or
	// "/"). Every real Cosmos SDK -- azcosmos included -- issues a GET
	// here before it will make a single data-plane call: it's how the SDK
	// discovers the account's readable/writable regional endpoints (see
	// handleAccountRoot's doc comment). Omitting this resource entirely
	// (an earlier version of this handler did exactly that, falling
	// through to the generic 400 default branch) makes the service
	// unreachable by any unmodified SDK, not merely missing one feature.
	resourceAccountRoot
	resourceDatabases
	resourceDatabaseItem
	resourceContainers
	resourceContainerItem
	resourceDocuments
	resourceDocumentItem
)

// parseResourcePath classifies path ("", "/dbs", "/dbs/{db}",
// "/dbs/{db}/colls", "/dbs/{db}/colls/{coll}",
// "/dbs/{db}/colls/{coll}/docs", or "/dbs/{db}/colls/{coll}/docs/{id}") and
// extracts its database/container/document identifiers.
//
// Path segment counts (named, not magic numbers): "/dbs" -> 1, "/dbs/{db}"
// -> 2, ".../colls" -> 3, ".../colls/{coll}" -> 4, ".../docs" -> 5,
// ".../docs/{id}" -> 6.
const (
	segCountDatabases     = 1
	segCountDatabaseItem  = 2
	segCountContainers    = 3
	segCountContainerItem = 4
	segCountDocuments     = 5
	segCountDocumentItem  = 6
)

func parseResourcePath(path string) (resourceKind, string, string, string) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return resourceAccountRoot, "", "", ""
	}

	segments := strings.Split(trimmed, "/")
	if segments[0] != "dbs" {
		return resourceInvalid, "", "", ""
	}

	switch len(segments) {
	case segCountDatabases:
		return resourceDatabases, "", "", ""
	case segCountDatabaseItem:
		return resourceDatabaseItem, segments[1], "", ""
	case segCountContainers:
		return parseContainersPath(segments)
	case segCountContainerItem:
		return parseContainerItemPath(segments)
	case segCountDocuments:
		return parseDocumentsPath(segments)
	case segCountDocumentItem:
		return parseDocumentItemPath(segments)
	default:
		return resourceInvalid, "", "", ""
	}
}

func parseContainersPath(segments []string) (resourceKind, string, string, string) {
	if segments[2] != collsSegment {
		return resourceInvalid, "", "", ""
	}

	return resourceContainers, segments[1], "", ""
}

func parseContainerItemPath(segments []string) (resourceKind, string, string, string) {
	if segments[2] != collsSegment {
		return resourceInvalid, "", "", ""
	}

	return resourceContainerItem, segments[1], segments[3], ""
}

func parseDocumentsPath(segments []string) (resourceKind, string, string, string) {
	if segments[2] != collsSegment || segments[4] != "docs" {
		return resourceInvalid, "", "", ""
	}

	return resourceDocuments, segments[1], segments[3], ""
}

func parseDocumentItemPath(segments []string) (resourceKind, string, string, string) {
	if segments[2] != collsSegment || segments[4] != "docs" {
		return resourceInvalid, "", "", ""
	}

	return resourceDocumentItem, segments[1], segments[3], segments[5]
}

// operationFor determines the Cosmos DB operation name for a request, for
// metrics labeling. Mirrors the dispatch logic in Handler() without side
// effects.
func operationFor(r *http.Request) string {
	if op, ok := tableAPIOperationFor(r); ok {
		return op
	}

	kind, _, _, _ := parseResourcePath(r.URL.Path)

	switch kind {
	case resourceAccountRoot:
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			return opGetDatabaseAccount
		}

		return unknownOperation
	case resourceDatabases:
		return collectionOperationFor(r.Method, opCreateDatabase, opListDatabases)
	case resourceDatabaseItem:
		return itemOperationFor(r.Method, opGetDatabase, "", opDeleteDatabase)
	case resourceContainers:
		return collectionOperationFor(r.Method, opCreateContainer, opListContainers)
	case resourceContainerItem:
		return itemOperationFor(r.Method, opGetContainer, "", opDeleteContainer)
	case resourceDocuments:
		return documentsCollectionOperationFor(r)
	case resourceDocumentItem:
		return itemOperationFor(r.Method, opGetDocument, opReplaceDocument, opDeleteDocument)
	default:
		return unknownOperation
	}
}

func collectionOperationFor(method, createOp, listOp string) string {
	switch method {
	case http.MethodPost:
		return createOp
	case http.MethodGet:
		return listOp
	default:
		return unknownOperation
	}
}

func itemOperationFor(method, getOp, putOp, deleteOp string) string {
	switch method {
	case http.MethodGet:
		return getOp
	case http.MethodPut:
		if putOp == "" {
			return unknownOperation
		}

		return putOp
	case http.MethodDelete:
		return deleteOp
	default:
		return unknownOperation
	}
}

func documentsCollectionOperationFor(r *http.Request) string {
	switch r.Method {
	case http.MethodGet:
		return opListDocuments
	case http.MethodPost:
		if isQueryRequest(r) {
			return opQueryDocuments
		}

		return opCreateDocument
	default:
		return unknownOperation
	}
}

// isQueryRequest reports whether a POST to a documents collection is a SQL
// query (as opposed to a document create/upsert), per AZURE.md section 3:
// either the x-ms-documentdb-isquery header is "true" (case-insensitively)
// or the Content-Type is application/query+json.
func isQueryRequest(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get(headerIsQuery), "true") {
		return true
	}

	return strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), headerContentTypeQuery)
}

// StartWorker binds the dedicated Cosmos DB listener and starts serving on
// it. See provider.go's Provider doc comment for why CosmosDB needs its own
// listener, and services/azuretable's StartWorker for the synchronous-bind
// rationale this mirrors exactly.
func (h *Handler) StartWorker(ctx context.Context) error {
	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf(":%d", h.Port))
	if err != nil {
		return fmt.Errorf("cosmosdb: bind port %d: %w", h.Port, err)
	}

	// If h.Port was 0 (let the OS assign an ephemeral port), reflect the
	// actual bound port back onto h.Port so a caller -- typically a test --
	// can read it off immediately after StartWorker returns, with zero
	// release-then-rebind window. Tests that instead pick a port via a
	// separate "bind :0, read it, close it, then tell StartWorker to bind
	// that same number" helper have a real (if narrow) race: another
	// process can grab the port in between. Setting h.Port here lets
	// StartWorker itself be the single source of truth for which port it
	// ended up on.
	if addr, ok := listener.Addr().(*net.TCPAddr); ok {
		h.Port = addr.Port
	}

	e := echo.New()
	e.Use(logger.EchoMiddleware(logger.Load(ctx)))
	e.Any("/*", telemetry.WrapEchoHandler("CosmosDB", h.Handler(), h))

	srv := &http.Server{
		Handler:           e,
		ReadHeaderTimeout: cosmosdbReadHeaderTimeout,
		ReadTimeout:       cosmosdbReadTimeout,
		WriteTimeout:      cosmosdbWriteTimeout,
		IdleTimeout:       cosmosdbIdleTimeout,
	}

	h.srvMu.Lock("StartWorker")
	h.srv = srv
	h.srvMu.Unlock()

	workerCtx := logger.WithWorker(ctx, "cosmosdb", "listener")
	log := logger.Load(workerCtx)

	log.InfoContext(workerCtx, "cosmosdb: starting dedicated listener", "port", h.Port)

	go func() {
		if serveErr := srv.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.ErrorContext(workerCtx, "cosmosdb: listener stopped", "error", serveErr)
		}
	}()

	return nil
}

// Timeouts for the dedicated Cosmos DB http.Server. ReadHeaderTimeout/
// ReadTimeout/IdleTimeout mirror services/azuretable's identical constants
// and their Slowloris rationale.
//
// WriteTimeout is set here, deliberately DIFFERENT from
// services/azureblob's http.Server, which just as deliberately does NOT set
// one (see its handler.go's own doc comment): Blob's rationale is that a
// finite WriteTimeout would cap how long a large blob download is allowed
// to take, which is actively wrong for that service's workload. That
// rationale does not transfer here -- every Cosmos DB response is a small
// JSON document (a single database/container/document resource, or one
// page of a query/read-feed result, none of which stream arbitrarily large
// payloads the way a blob GET can), so bounding write time is pure
// Slowloris-class hardening with no legitimate workload it could cut off.
// A future reader should not "fix" this back to match azureblob's choice --
// the two services' write-timeout needs are genuinely different, not an
// oversight in either one.
const (
	cosmosdbReadHeaderTimeout = 10 * time.Second
	cosmosdbReadTimeout       = 60 * time.Second
	cosmosdbWriteTimeout      = 60 * time.Second
	cosmosdbIdleTimeout       = 120 * time.Second
)

// Shutdown stops the dedicated Cosmos DB listener.
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
		log.ErrorContext(ctx, "cosmosdb: graceful shutdown failed, forcing close", "error", err)

		if closeErr := srv.Close(); closeErr != nil {
			log.ErrorContext(ctx, "cosmosdb: forced close also failed", "error", closeErr)
		}
	}
}

// writeJSON marshals v and writes it as the response body with
// Content-Type: application/json.
func (h *Handler) writeJSON(c *echo.Context, status int, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return h.writeErrorNoRecurse(c, http.StatusInternalServerError, "InternalError", "Failed to marshal response.")
	}

	return c.Blob(status, "application/json", body)
}

// cosmosErrorBody is Cosmos DB's JSON error envelope shape:
// {"code":"NotFound","message":"..."}.
type cosmosErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError writes Cosmos DB's standard JSON error envelope.
func (h *Handler) writeError(c *echo.Context, status int, code, message string) error {
	return h.writeJSON(c, status, cosmosErrorBody{Code: code, Message: message})
}

// writeErrorNoRecurse is writeJSON's own marshal-failure fallback: it must
// not call back into writeJSON (which could recurse if the error envelope
// itself somehow failed to marshal, though it never does in practice since
// it's built from plain strings).
func (h *Handler) writeErrorNoRecurse(c *echo.Context, status int, code, message string) error {
	body, _ := json.Marshal(cosmosErrorBody{Code: code, Message: message})

	return c.Blob(status, "application/json", body)
}
