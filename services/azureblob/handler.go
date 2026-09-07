package azureblob

import (
	"context"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/azureauth"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
)

// azureBlobVersion is the x-ms-version value echoed on every response. It is
// a plausible, well-formed Azure Storage REST API version string -- picked so
// azure-sdk-for-go's blob client parses it without erroring, not because
// gopherstack implements that exact API version's full feature set.
const azureBlobVersion = "2021-08-06"

// blockBlobType is the only x-ms-blob-type this MVP accepts/serves (see
// PARITY.md known gaps: no append/page blob support).
const blockBlobType = "BlockBlob"

// Operation name constants used for metrics (ExtractOperation) and
// GetSupportedOperations.
const (
	opListContainers    = "ListContainers"
	opCreateContainer   = "CreateContainer"
	opDeleteContainer   = "DeleteContainer"
	opListBlobs         = "ListBlobs"
	opPutBlob           = "PutBlob"
	opGetBlob           = "GetBlob"
	opGetBlobProperties = "GetBlobProperties"
	opDeleteBlob        = "DeleteBlob"
	unknownOperation    = "Unknown"
)

// Handler is the Echo HTTP handler for Azure Blob Storage operations.
type Handler struct {
	Backend StorageBackend
	srvMu   *lockmetrics.RWMutex
	srv     *http.Server
	// Endpoint is e.g. "http://127.0.0.1:10000" -- used to build
	// ServiceEndpoint in list responses.
	Endpoint string
	// Port is the TCP port StartWorker binds. Set from Settings at Init time
	// (see provider.go); defaults to DefaultPort. Unlike a per-resource
	// ephemeral allocation, this is a single fixed, protocol-conventional
	// port (mirroring services/iot's MQTT broker) -- there is no fallback
	// pool, so StartWorker fails fast if it's unavailable rather than
	// silently binding a different port.
	Port int
}

// NewHandler creates a new Azure Blob Handler. Port defaults to DefaultPort;
// callers (typically provider.go) override it from Settings.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{
		Backend: backend,
		Port:    DefaultPort,
		srvMu:   lockmetrics.New("azureblob.server"),
	}
}

var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
	_ service.Resettable       = (*Handler)(nil)
)

// Name returns the service name.
func (h *Handler) Name() string { return "AzureBlob" }

// GetSupportedOperations returns the list of supported Azure Blob operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opListContainers,
		opCreateContainer,
		opDeleteContainer,
		opListBlobs,
		opPutBlob,
		opGetBlob,
		opGetBlobProperties,
		opDeleteBlob,
	}
}

// RouteMatcher exists only to satisfy service.Registerable's interface
// contract: AzureBlob deliberately never matches on the shared AWS
// single-port Router. It runs on its own dedicated listener started by
// StartWorker (see provider.go for the full rationale). AzureBlob's
// Provider IS registered in cli.go's getMostRecentServiceProviders like
// every other service -- startBackgroundWorkers calls StartWorker via the
// service.BackgroundWorker interface regardless of routing, which is how
// the dedicated listener comes up. Only RouteMatcher itself is inert, kept
// so *Handler satisfies service.Registerable.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(*echo.Context) bool { return false }
}

// MatchPriority returns the routing priority for the AzureBlob handler.
// Irrelevant in practice since RouteMatcher never matches; 0 (lowest) is
// the safe default.
func (h *Handler) MatchPriority() int { return 0 }

// ExtractOperation extracts the Azure Blob operation name from the request,
// for metrics labeling.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return operationFor(c.Request())
}

// ExtractResource extracts the container/blob resource identifier from the
// request path, for metrics labeling.
func (h *Handler) ExtractResource(c *echo.Context) string {
	_, container, blob := splitPath(c.Request().URL.Path)
	if blob != "" {
		return container + "/" + blob
	}

	return container
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Handler returns the Echo handler function for Azure Blob operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()

		h.setCommonHeaders(c)
		h.checkAuth(r)

		account, container, blob := splitPath(r.URL.Path)
		if account == "" {
			return h.writeError(c, http.StatusBadRequest, "InvalidUri",
				"The requested URI does not represent any resource on the server.")
		}

		switch {
		case container == "":
			return h.handleAccountLevel(c)
		case blob == "":
			return h.handleContainerLevel(c, container)
		default:
			return h.handleBlobLevel(c, container, blob)
		}
	}
}

// checkAuth is intentionally permissive: it neither requires nor
// cryptographically verifies the Authorization header, matching this repo's
// permissive-by-default auth philosophy (see services/s3/sigv4.go). Any
// structurally-present "SharedKey ..." header, or its absence, is accepted.
//
// pkgs/azureauth.ParseAuthorizationHeader is used to prove a real Azure SDK's
// Authorization header round-trips through this package (account name /
// scheme extraction), but a malformed or absent header is still accepted --
// matching services/s3's own opt-in verification stance. Rejecting invalid
// signatures via azureauth.VerifySharedKey is deliberately deferred past M0
// (see AZURE.md section 5): it needs to be exercised against real SDK
// request shapes first, the same way S3's WithPresignValidation is opt-in
// rather than on-by-default.
func (h *Handler) checkAuth(r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return // anonymous; accepted by design at this milestone
	}

	if _, ok := azureauth.ParseAuthorizationHeader(authHeader); !ok {
		// Structurally malformed; still accepted at this milestone, but
		// logged so the gap is visible rather than silently swallowed.
		logger.Load(r.Context()).DebugContext(r.Context(), "azureblob: malformed Authorization header accepted at M0")
	}
}

// setCommonHeaders sets the headers real Azure SDKs expect on every
// response, success or error.
func (h *Handler) setCommonHeaders(c *echo.Context) {
	hdr := c.Response().Header()
	hdr.Set("X-Ms-Version", azureBlobVersion)
	hdr.Set("X-Ms-Request-Id", newRequestID())
	hdr.Set("Date", time.Now().UTC().Format(http.TimeFormat))
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

// maxPathSegments bounds splitPath's strings.SplitN call: account, container,
// and everything else as blob (blob names may contain "/" themselves).
const maxPathSegments = 3

// blobSegmentIndex is the index (and, as a length check, the minimum part
// count) at which a blob segment is present in splitPath's parts slice.
const blobSegmentIndex = 2

// splitPath splits an Azure Blob REST path ("/<account>/<container>/<blob>")
// into its three components. blob may itself contain "/" (Azure blob names
// may include virtual-directory separators), so it is never split further.
func splitPath(p string) (string, string, string) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", "", ""
	}

	parts := strings.SplitN(p, "/", maxPathSegments)
	account := parts[0]

	var container, blob string

	if len(parts) > 1 {
		container = parts[1]
	}

	if len(parts) > blobSegmentIndex {
		blob = parts[blobSegmentIndex]
	}

	return account, container, blob
}

// Query-parameter names and values shared by operationFor and the actual
// request handlers (handleAccountLevel/handleContainerLevel).
const (
	queryRestype = "restype"
	queryComp    = "comp"

	restypeContainer = "container"
	compList         = "list"
)

// operationFor determines the Azure Blob operation name for a request, for
// metrics labeling. Mirrors the dispatch logic in handleAccountLevel/
// handleContainerLevel/handleBlobLevel without side effects. Split into one
// helper per path level (account/container/blob) to keep each branch small.
func operationFor(r *http.Request) string {
	_, container, blob := splitPath(r.URL.Path)

	switch {
	case blob != "":
		return blobOperationFor(r.Method)
	case container != "":
		return containerOperationFor(r)
	default:
		return accountOperationFor(r)
	}
}

// accountOperationFor covers the one account-level operation, List
// Containers (GET /<account>?comp=list).
func accountOperationFor(r *http.Request) string {
	if r.Method == http.MethodGet && r.URL.Query().Get(queryComp) == compList {
		return opListContainers
	}

	return unknownOperation
}

// containerOperationFor covers the three container-scoped operations:
// Create Container, Delete Container, and List Blobs.
func containerOperationFor(r *http.Request) string {
	restype := r.URL.Query().Get(queryRestype)
	comp := r.URL.Query().Get(queryComp)

	switch {
	case r.Method == http.MethodPut && restype == restypeContainer:
		return opCreateContainer
	case r.Method == http.MethodDelete && restype == restypeContainer:
		return opDeleteContainer
	case r.Method == http.MethodGet && restype == restypeContainer && comp == compList:
		return opListBlobs
	default:
		return unknownOperation
	}
}

// blobOperationFor covers the four blob-scoped operations, dispatched purely
// by HTTP method (mirrors handleBlobLevel).
func blobOperationFor(method string) string {
	switch method {
	case http.MethodPut:
		return opPutBlob
	case http.MethodGet:
		return opGetBlob
	case http.MethodHead:
		return opGetBlobProperties
	case http.MethodDelete:
		return opDeleteBlob
	default:
		return unknownOperation
	}
}

// serviceEndpoint returns the ServiceEndpoint attribute value for
// EnumerationResults responses.
func (h *Handler) serviceEndpoint() string {
	if h.Endpoint != "" {
		return h.Endpoint
	}

	return fmt.Sprintf("http://127.0.0.1:%d", h.Port)
}

// handleAccountLevel serves GET /<account>?comp=list (List Containers).
func (h *Handler) handleAccountLevel(c *echo.Context) error {
	r := c.Request()
	if r.Method != http.MethodGet || c.QueryParam(queryComp) != compList {
		return h.writeError(c, http.StatusBadRequest, "InvalidQueryParameterValue",
			"A query parameter is not supported for this operation.")
	}

	containers := h.Backend.ListContainers()
	result := enumerationResults{
		ServiceEndpoint: h.serviceEndpoint(),
		Containers:      &containersList{Container: make([]containerEntry, 0, len(containers))},
	}

	for _, ci := range containers {
		result.Containers.Container = append(result.Containers.Container, containerEntry{
			Name: ci.Name,
			Properties: containerProperties{
				LastModified: ci.CreatedAt.Format(http.TimeFormat),
				Etag:         computeContainerETag(ci.Name, ci.CreatedAt),
			},
		})
	}

	return h.writeXML(c, http.StatusOK, result)
}

// handleContainerLevel serves the three container-scoped operations: Create
// Container, Delete Container, and List Blobs.
func (h *Handler) handleContainerLevel(c *echo.Context, container string) error {
	r := c.Request()
	restype := c.QueryParam(queryRestype)
	comp := c.QueryParam(queryComp)

	switch {
	case r.Method == http.MethodPut && restype == restypeContainer:
		return h.createContainer(c, container)
	case r.Method == http.MethodDelete && restype == restypeContainer:
		return h.deleteContainer(c, container)
	case r.Method == http.MethodGet && restype == restypeContainer && comp == compList:
		return h.listBlobs(c, container)
	default:
		return h.writeError(c, http.StatusBadRequest, "InvalidQueryParameterValue",
			"A query parameter is not supported for this operation.")
	}
}

func (h *Handler) createContainer(c *echo.Context, container string) error {
	if err := h.Backend.CreateContainer(container); err != nil {
		if errors.Is(err, ErrContainerAlreadyExists) {
			return h.writeError(c, http.StatusConflict, "ContainerAlreadyExists",
				"The specified container already exists.")
		}

		return h.writeError(c, http.StatusInternalServerError, "InternalError", err.Error())
	}

	return c.NoContent(http.StatusCreated)
}

func (h *Handler) deleteContainer(c *echo.Context, container string) error {
	if err := h.Backend.DeleteContainer(container); err != nil {
		return h.writeError(c, http.StatusNotFound, "ContainerNotFound",
			"The specified container does not exist.")
	}

	return c.NoContent(http.StatusAccepted)
}

func (h *Handler) listBlobs(c *echo.Context, container string) error {
	blobs, err := h.Backend.ListBlobs(container)
	if err != nil {
		return h.writeError(c, http.StatusNotFound, "ContainerNotFound",
			"The specified container does not exist.")
	}

	result := enumerationResults{
		ServiceEndpoint: h.serviceEndpoint(),
		ContainerName:   container,
		Blobs:           &blobsList{Blob: make([]blobEntry, 0, len(blobs))},
	}

	for _, bi := range blobs {
		result.Blobs.Blob = append(result.Blobs.Blob, blobEntry{
			Name: bi.Name,
			Properties: blobProperties{
				LastModified:  bi.LastModified.Format(http.TimeFormat),
				Etag:          bi.ETag,
				ContentLength: bi.ContentLength,
				ContentType:   bi.ContentType,
				BlobType:      blockBlobType,
			},
		})
	}

	return h.writeXML(c, http.StatusOK, result)
}

// handleBlobLevel dispatches the four blob-scoped operations by HTTP method.
func (h *Handler) handleBlobLevel(c *echo.Context, container, blob string) error {
	switch c.Request().Method {
	case http.MethodPut:
		return h.putBlob(c, container, blob)
	case http.MethodGet:
		return h.getBlob(c, container, blob)
	case http.MethodHead:
		return h.headBlob(c, container, blob)
	case http.MethodDelete:
		return h.deleteBlob(c, container, blob)
	default:
		return h.writeError(c, http.StatusMethodNotAllowed, "UnsupportedHttpVerb",
			"The resource doesn't support the specified HTTP verb.")
	}
}

func (h *Handler) putBlob(c *echo.Context, container, blob string) error {
	r := c.Request()

	if r.Header.Get("X-Ms-Blob-Type") != blockBlobType {
		return h.writeError(c, http.StatusBadRequest, "InvalidHeaderValue",
			"The value for one of the HTTP headers is not in the correct format "+
				"(x-ms-blob-type must be BlockBlob; only block blobs are supported).")
	}

	body, err := httputils.ReadBody(r)
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "InternalError",
			"Failed to read request body.")
	}

	info, err := h.Backend.PutBlob(container, blob, body, r.Header.Get("Content-Type"))
	if err != nil {
		return h.writeError(c, http.StatusNotFound, "ContainerNotFound",
			"The specified container does not exist.")
	}

	hdr := c.Response().Header()
	hdr.Set("ETag", info.ETag)
	hdr.Set("Last-Modified", info.LastModified.Format(http.TimeFormat))

	return c.NoContent(http.StatusCreated)
}

func (h *Handler) getBlob(c *echo.Context, container, blob string) error {
	r := c.Request()

	info, data, err := h.Backend.GetBlob(container, blob)
	if err != nil {
		return h.writeBlobNotFoundError(c, err)
	}

	h.setBlobHeaders(c, info)

	// x-ms-range takes precedence over the standard Range header when both
	// are present, matching real Azure Blob's documented behavior -- the
	// azure-storage-blob (Python) and @azure/storage-blob (JS) SDKs' generated
	// Get Blob operation sends x-ms-range exclusively (never Range), so
	// accepting only "Range" made GetBlob's single-range support unreachable
	// from those SDKs' default download path even though the Go SDK (which
	// does send "Range") passed.
	rangeHeader := r.Header.Get("X-Ms-Range")
	if rangeHeader == "" {
		rangeHeader = r.Header.Get("Range")
	}

	if rangeHeader == "" {
		return c.Blob(http.StatusOK, contentTypeOrDefault(info.ContentType), data)
	}

	start, end, ok := parseRange(rangeHeader, int64(len(data)))
	if !ok {
		c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(data)))

		return h.writeError(c, http.StatusRequestedRangeNotSatisfiable, "InvalidRange",
			"The range specified is invalid for the current size of the resource.")
	}

	hdr := c.Response().Header()
	hdr.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
	hdr.Set("Content-Length", strconv.FormatInt(end-start+1, 10))

	return c.Blob(http.StatusPartialContent, contentTypeOrDefault(info.ContentType), data[start:end+1])
}

func (h *Handler) headBlob(c *echo.Context, container, blob string) error {
	info, err := h.Backend.HeadBlob(container, blob)
	if err != nil {
		return h.writeBlobNotFoundError(c, err)
	}

	h.setBlobHeaders(c, info)

	return c.NoContent(http.StatusOK)
}

func (h *Handler) deleteBlob(c *echo.Context, container, blob string) error {
	if err := h.Backend.DeleteBlob(container, blob); err != nil {
		return h.writeBlobNotFoundError(c, err)
	}

	return c.NoContent(http.StatusAccepted)
}

// writeBlobNotFoundError maps a StorageBackend not-found error (either the
// container or the blob may be missing) to the corresponding Azure error code.
func (h *Handler) writeBlobNotFoundError(c *echo.Context, err error) error {
	if errors.Is(err, ErrContainerNotFound) {
		return h.writeError(c, http.StatusNotFound, "ContainerNotFound",
			"The specified container does not exist.")
	}

	return h.writeError(c, http.StatusNotFound, "BlobNotFound", "The specified blob does not exist.")
}

// setBlobHeaders sets the properties common to Get Blob, Get Blob Properties,
// and (implicitly, via the caller) any other blob-body response.
func (h *Handler) setBlobHeaders(c *echo.Context, info BlobInfo) {
	hdr := c.Response().Header()
	hdr.Set("ETag", info.ETag)
	hdr.Set("Last-Modified", info.LastModified.Format(http.TimeFormat))
	hdr.Set("Content-Length", strconv.FormatInt(info.ContentLength, 10))
	hdr.Set("X-Ms-Blob-Type", blockBlobType)
	hdr.Set("Accept-Ranges", "bytes")

	if info.ContentType != "" {
		hdr.Set("Content-Type", info.ContentType)
	}
}

func contentTypeOrDefault(ct string) string {
	if ct == "" {
		return "application/octet-stream"
	}

	return ct
}

// parseRange parses an HTTP "Range: bytes=start-end" header (also supporting
// the open-ended "bytes=start-" and suffix "bytes=-N" forms) against a
// resource of the given size. Only a single range is supported (Azure Get
// Blob does not support multi-range requests). Returns ok=false if the
// header is absent, malformed, or unsatisfiable for size.
func parseRange(header string, size int64) (int64, int64, bool) {
	const prefix = "bytes="

	spec, found := strings.CutPrefix(header, prefix)
	if !found {
		return 0, 0, false
	}

	// Reject multi-range requests (a comma indicates more than one range);
	// this backend only serves the first/only range.
	if strings.Contains(spec, ",") {
		return 0, 0, false
	}

	before, after, found := strings.Cut(spec, "-")
	if !found {
		return 0, 0, false
	}

	if before == "" {
		// Suffix form: "bytes=-N" means the last N bytes.
		n, err := strconv.ParseInt(after, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}

		if n > size {
			n = size
		}

		return size - n, size - 1, size > 0
	}

	start, err := strconv.ParseInt(before, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}

	if after == "" {
		return start, size - 1, true
	}

	end, err := strconv.ParseInt(after, 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}

	if end >= size {
		end = size - 1
	}

	return start, end, true
}

// writeXML marshals v and writes it as the response body. v is marshaled
// without an XML header/prolog: echo's XMLBlob prepends xml.Header itself,
// so marshaling one here would duplicate it (see services/sqs/PARITY.md for
// the same trap hit and fixed there).
func (h *Handler) writeXML(c *echo.Context, status int, v any) error {
	body, err := xml.Marshal(v)
	if err != nil {
		return h.writeError(c, http.StatusInternalServerError, "InternalError", "Failed to marshal response.")
	}

	return c.XMLBlob(status, body)
}

// writeError writes a standard Azure Storage REST error body.
func (h *Handler) writeError(c *echo.Context, status int, code, message string) error {
	return h.writeXML(c, status, azureError{Code: code, Message: message})
}

// Timeouts for the dedicated Blob http.Server. ReadHeaderTimeout alone only
// bounds how long a client may take to send headers; without ReadTimeout and
// IdleTimeout a slow-body client (e.g. a stalled PUT blob upload) or a client
// that opens a connection and never closes it can hold a handler goroutine
// and connection open indefinitely (a Slowloris-style resource exhaustion).
// WriteTimeout is deliberately not set: it would bound the full
// request-to-response-complete duration, which for a large GetBlob download
// depends on the client's own read rate, not just server-side work.
const (
	azureBlobReadHeaderTimeout = 10 * time.Second
	azureBlobReadTimeout       = 60 * time.Second
	azureBlobIdleTimeout       = 120 * time.Second
)

// StartWorker binds the dedicated Blob listener and starts serving on it.
// See provider.go's Provider doc comment for why AzureBlob needs its own
// listener instead of registering into the shared AWS Router.
//
// Binding is synchronous: net.Listen returns before StartWorker does, so a
// bind failure is returned to the caller directly instead of only being
// logged from the background goroutine after startup has already reported
// success. This mirrors services/iot's MQTT broker (services/iot/broker.go),
// gopherstack's existing precedent for a service with a fixed,
// protocol-conventional default port: bind exactly the configured port
// (h.Port, from Settings -- see settings.go/provider.go) and fail fast if
// that's unavailable, rather than silently falling back into the shared
// --port-range-start/--port-range-end PortAlloc pool used for on-demand
// ephemeral resources elsewhere (Lambda function URLs, ElastiCache). A
// fallback there would be just as surprising as picking a different default
// port number outright: either way, an SDK relying on the well-known default
// (UseDevelopmentStorage=true-style config) would silently end up talking to
// the wrong port. Failing fast surfaces the conflict instead.
func (h *Handler) StartWorker(ctx context.Context) error {
	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf(":%d", h.Port))
	if err != nil {
		return fmt.Errorf("azureblob: bind port %d: %w", h.Port, err)
	}

	e := echo.New()
	e.Use(logger.EchoMiddleware(logger.Load(ctx)))
	e.Any("/*", telemetry.WrapEchoHandler("AzureBlob", h.Handler(), h))

	srv := &http.Server{
		Handler:           e,
		ReadHeaderTimeout: azureBlobReadHeaderTimeout,
		ReadTimeout:       azureBlobReadTimeout,
		IdleTimeout:       azureBlobIdleTimeout,
	}

	h.srvMu.Lock("StartWorker")
	h.srv = srv
	h.srvMu.Unlock()

	workerCtx := logger.WithWorker(ctx, "azureblob", "listener")
	log := logger.Load(workerCtx)

	log.InfoContext(workerCtx, "azureblob: starting dedicated listener", "port", h.Port)

	go func() {
		if serveErr := srv.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.ErrorContext(workerCtx, "azureblob: listener stopped", "error", serveErr)
		}
	}()

	return nil
}

// Shutdown stops the dedicated Blob listener. A graceful Shutdown error
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
		log.ErrorContext(ctx, "azureblob: graceful shutdown failed, forcing close", "error", err)

		if closeErr := srv.Close(); closeErr != nil {
			log.ErrorContext(ctx, "azureblob: forced close also failed", "error", closeErr)
		}
	}
}
