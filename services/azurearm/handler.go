package azurearm

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/aadauth"
	"github.com/blackbirdworks/gopherstack/pkgs/devtls"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
)

// Operation name constants used for metrics (ExtractOperation) and
// GetSupportedOperations.
const (
	opMetadataEndpoints   = "MetadataEndpoints"
	opOpenIDConfiguration = "OpenIDConfiguration"
	opInstanceDiscovery   = "InstanceDiscovery"
	opToken               = "Token"
	opListSubscriptions   = "ListSubscriptions"
	opGetSubscription     = "GetSubscription"
	opListTenants         = "ListTenants"
	opListProviders       = "ListProviders"
	opGetProvider         = "GetProvider"
	opRegisterProvider    = "RegisterProvider"
	opPutResourceGroup    = "PutResourceGroup"
	opGetResourceGroup    = "GetResourceGroup"
	opDeleteResourceGroup = "DeleteResourceGroup"
	opListResourceGroups  = "ListResourceGroups"
	opPutResource         = "PutResource"
	opGetResource         = "GetResource"
	opDeleteResource      = "DeleteResource"
	opListResources       = "ListResources"
	opListKeys            = "ListKeys"
	unknownOperation      = "Unknown"
)

// Handler is the Echo HTTP handler for the ARM emulation's dedicated
// listener.
type Handler struct {
	Backend  *InMemoryBackend
	Registry *Registry
	Settings Settings
	Issuer   *aadauth.Issuer

	srvMu *lockmetrics.RWMutex
	srv   *http.Server
	// Port is the TCP port StartWorker binds. Set from Settings at Init time.
	Port int
}

// NewHandler creates a new ARM Handler.
func NewHandler(backend *InMemoryBackend, registry *Registry, issuer *aadauth.Issuer, settings Settings) *Handler {
	return &Handler{
		Backend:  backend,
		Registry: registry,
		Settings: settings,
		Issuer:   issuer,
		Port:     settings.Port,
		srvMu:    lockmetrics.New("azurearm.server"),
	}
}

var (
	_ service.BackgroundWorker = (*Handler)(nil)
	_ service.Shutdowner       = (*Handler)(nil)
	_ service.Resettable       = (*Handler)(nil)
)

// Name returns the service name.
func (h *Handler) Name() string { return "AzureARM" }

// GetSupportedOperations returns the list of supported ARM operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opMetadataEndpoints, opOpenIDConfiguration, opInstanceDiscovery, opToken,
		opListSubscriptions, opGetSubscription, opListTenants,
		opListProviders, opGetProvider, opRegisterProvider,
		opPutResourceGroup, opGetResourceGroup, opDeleteResourceGroup, opListResourceGroups,
		opPutResource, opGetResource, opDeleteResource, opListResources, opListKeys,
	}
}

// RouteMatcher exists only to satisfy service.Registerable's interface
// contract: AzureARM deliberately never matches on the shared AWS
// single-port Router, exactly like services/azureblob/azurequeue/azuretable/
// cosmosdb -- it runs on its own dedicated HTTPS listener started by
// StartWorker. Only RouteMatcher itself is inert.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(*echo.Context) bool { return false }
}

// MatchPriority returns the routing priority for the AzureARM handler.
// Irrelevant in practice since RouteMatcher never matches; 0 is the safe
// default.
func (h *Handler) MatchPriority() int { return 0 }

// ExtractOperation extracts the ARM operation name from the request, for
// metrics labeling.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return operationFor(c.Request())
}

// ExtractResource extracts a resource identifier from the request path, for
// metrics labeling.
func (h *Handler) ExtractResource(c *echo.Context) string {
	return c.Request().URL.Path
}

// Reset clears all in-memory ARM state.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Handler returns the Echo handler function for ARM operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		path := strings.TrimSuffix(r.URL.Path, "/")
		segs := splitARMPath(path)

		host := hostFromHostHeader(r.Host)
		ctx := WithRequestHost(r.Context(), host)
		c.SetRequest(r.WithContext(ctx))

		return h.dispatch(c, segs)
	}
}

// dispatch routes on the request path's shape. A single large switch (rather
// than per-route Echo registrations) mirrors services/azureblob's
// splitPath-based dispatch and is what makes the generic resource plane's
// "one path walker, no per-type routes" requirement (AZURE.md section 10.1)
// possible.
func (h *Handler) dispatch(c *echo.Context, segs []string) error {
	switch {
	case len(segs) == 2 && segs[0] == "metadata" && segs[1] == "endpoints":
		return h.handleMetadataEndpoints(c)
	case len(segs) == 2 && segs[0] == "common" && segs[1] == "discovery":
		// handled by discovery/instance below (3 segs); this case is
		// unreachable but kept for readability of the shape.
		return h.writeNotFound(c)
	case len(segs) == 3 && segs[0] == "common" && segs[1] == "discovery" && segs[2] == "instance":
		return h.handleInstanceDiscovery(c)
	case len(segs) == 4 && segs[1] == "v2.0" && segs[2] == ".well-known" && segs[3] == "openid-configuration":
		return h.handleOpenIDConfiguration(c, segs[0])
	case len(segs) == 3 && segs[1] == "oauth2" && segs[2] == "token":
		return h.handleToken(c, segs[0])
	case len(segs) == 4 && segs[1] == "oauth2" && segs[2] == "v2.0" && segs[3] == "token":
		return h.handleToken(c, segs[0])
	case len(segs) == 4 && segs[1] == "discovery" && segs[2] == "v2.0" && segs[3] == "keys":
		return h.handleJWKS(c)
	default:
		return h.dispatchARMResource(c, segs)
	}
}

// dispatchARMResource handles every /subscriptions/... and /tenants shape:
// subscriptions/tenants list+get, provider registration, resource-group
// CRUD, and the generic resource plane (PUT/GET/DELETE/listKeys).
func (h *Handler) dispatchARMResource(c *echo.Context, segs []string) error { //nolint:cyclop // one dispatcher table, not meaningfully splittable
	switch {
	case len(segs) == 1 && strings.EqualFold(segs[0], "tenants"):
		return h.handleListTenants(c)
	case len(segs) == 1 && strings.EqualFold(segs[0], subscriptionsSegment):
		return h.handleListSubscriptions(c)
	case len(segs) == 2 && strings.EqualFold(segs[0], subscriptionsSegment):
		return h.handleGetSubscription(c, segs[1])
	case len(segs) == 3 && strings.EqualFold(segs[0], subscriptionsSegment) && strings.EqualFold(segs[2], providersSegment):
		return h.handleListProviders(c, segs[1])
	case len(segs) == 4 && strings.EqualFold(segs[0], subscriptionsSegment) && strings.EqualFold(segs[2], providersSegment):
		return h.handleGetProvider(c, segs[1], segs[3])
	case len(segs) == 5 && strings.EqualFold(segs[0], subscriptionsSegment) &&
		strings.EqualFold(segs[2], providersSegment) && strings.EqualFold(segs[4], "register") && c.Request().Method == http.MethodPost:
		return h.handleRegisterProvider(c, segs[1], segs[3])
	case len(segs) == 3 && strings.EqualFold(segs[0], subscriptionsSegment) && strings.EqualFold(segs[2], resourceGroupsSegment):
		return h.handleListResourceGroups(c, segs[1])
	case len(segs) == 4 && strings.EqualFold(segs[0], subscriptionsSegment) && strings.EqualFold(segs[2], resourceGroupsSegment):
		return h.handleResourceGroup(c, segs[1], segs[3])
	case len(segs) >= 6 && strings.EqualFold(segs[len(segs)-1], "listKeys") && c.Request().Method == http.MethodPost:
		return h.handleListKeys(c, segs[:len(segs)-1])
	case len(segs) == 5 && strings.EqualFold(segs[0], subscriptionsSegment) && strings.EqualFold(segs[2], providersSegment):
		return h.handleListResources(c, segs)
	case len(segs) == 7 && strings.EqualFold(segs[0], subscriptionsSegment) &&
		strings.EqualFold(segs[2], resourceGroupsSegment) && strings.EqualFold(segs[4], providersSegment):
		return h.handleListResources(c, segs)
	case len(segs) >= 7 && strings.EqualFold(segs[0], subscriptionsSegment) &&
		strings.EqualFold(segs[2], resourceGroupsSegment) && strings.EqualFold(segs[4], providersSegment):
		return h.handleGenericResource(c)
	default:
		return h.writeNotFound(c)
	}
}

// writeNotFound writes a generic 404 error envelope for an unrecognized path.
func (h *Handler) writeNotFound(c *echo.Context) error {
	return h.writeError(c, http.StatusNotFound, "NotFound", "The requested resource was not found.")
}

// operationFor derives a coarse operation name from the request, for
// metrics labeling.
func operationFor(r *http.Request) string {
	path := strings.TrimSuffix(r.URL.Path, "/")
	segs := splitARMPath(path)

	switch {
	case len(segs) == 0:
		return unknownOperation
	case len(segs) == 2 && segs[0] == "metadata":
		return opMetadataEndpoints
	case strings.Contains(path, "openid-configuration"):
		return opOpenIDConfiguration
	case strings.Contains(path, "discovery/instance"):
		return opInstanceDiscovery
	case strings.Contains(path, "oauth2"):
		return opToken
	case strings.HasSuffix(path, "/listKeys"):
		return opListKeys
	case len(segs) >= 1 && strings.EqualFold(segs[0], "tenants"):
		return opListTenants
	default:
		return operationForSubscriptionPath(r.Method, segs)
	}
}

func operationForSubscriptionPath(method string, segs []string) string {
	switch {
	case len(segs) <= 2:
		return opListSubscriptions
	case len(segs) == 3 && strings.EqualFold(segs[2], providersSegment):
		return opListProviders
	case len(segs) == 4 && strings.EqualFold(segs[2], providersSegment):
		return opGetProvider
	case len(segs) == 5 && strings.EqualFold(segs[2], providersSegment):
		return opRegisterProvider
	case len(segs) == 3 && strings.EqualFold(segs[2], resourceGroupsSegment):
		return opListResourceGroups
	case len(segs) == 4 && strings.EqualFold(segs[2], resourceGroupsSegment):
		return methodOperation(method, opPutResourceGroup, opGetResourceGroup, opDeleteResourceGroup)
	default:
		return methodOperation(method, opPutResource, opGetResource, opDeleteResource)
	}
}

func methodOperation(method, put, get, del string) string {
	switch method {
	case http.MethodPut:
		return put
	case http.MethodDelete:
		return del
	default:
		return get
	}
}

// hostFromHostHeader strips a port from a host:port string.
func hostFromHostHeader(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}

	return host
}

// baseURLFor builds this ARM listener's own externally-visible base URL
// (scheme://host:port) from the request, always https (AZURE.md section
// 10.8).
func (h *Handler) baseURLFor(r *http.Request) string {
	return fmt.Sprintf("https://%s:%d", hostFromHostHeader(r.Host), h.Port)
}

// decodeJSONBody decodes r's body as a JSON object. An empty body decodes to
// an empty (non-nil) map.
func decodeJSONBody(r *http.Request) (map[string]any, error) {
	if r.ContentLength == 0 {
		return map[string]any{}, nil
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRequestBody, err)
	}

	if body == nil {
		body = map[string]any{}
	}

	return body, nil
}

// writeJSON writes v as the JSON response body with the given status.
func (h *Handler) writeJSON(c *echo.Context, status int, v any) error {
	err := c.JSON(status, v)
	if err != nil {
		return fmt.Errorf("azurearm: write JSON response: %w", err)
	}

	return nil
}

// writeError writes the ARM error envelope {"error":{"code","message"}}.
func (h *Handler) writeError(c *echo.Context, status int, code, message string) error {
	return h.writeJSON(c, status, map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
}

// errorDetails maps a sentinel error to its ARM error code, message, and
// HTTP status, mirroring services/sqs's errorDetails pattern.
func errorDetails(err error) errorEntry {
	table := []struct {
		err   error
		entry errorEntry
	}{
		{ErrResourceGroupNotFound, errorEntry{"ResourceGroupNotFound", "Resource group not found.", http.StatusNotFound}},
		{ErrResourceNotFound, errorEntry{"ResourceNotFound", "The resource was not found.", http.StatusNotFound}},
		{ErrStorageAccountNotFound, errorEntry{"ResourceNotFound", "The storage account was not found.", http.StatusNotFound}},
		{ErrSubscriptionNotFound, errorEntry{"SubscriptionNotFound", "Subscription not found.", http.StatusNotFound}},
		{ErrProviderNotFound, errorEntry{"ProviderNotFound", "Resource provider not found.", http.StatusNotFound}},
		{ErrInvalidResourceID, errorEntry{"InvalidResourceId", "The resource ID is malformed.", http.StatusBadRequest}},
		{ErrInvalidRequestBody, errorEntry{"InvalidRequestContent", "The request body is malformed.", http.StatusBadRequest}},
	}

	for _, e := range table {
		if errors.Is(err, e.err) {
			return e.entry
		}
	}

	return errorEntry{"InternalError", err.Error(), http.StatusInternalServerError}
}

// writeAPIError writes the ARM error envelope for err, using errorDetails to
// determine its code/message/status.
func (h *Handler) writeAPIError(c *echo.Context, err error) error {
	e := errorDetails(err)

	return h.writeError(c, e.status, e.code, e.message)
}

// parseFormOrJSONBody parses an application/x-www-form-urlencoded body
// (client-credentials token requests always use this content type) into a
// url.Values.
func parseFormOrJSONBody(r *http.Request) (url.Values, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("azurearm: parse form body: %w", err)
	}

	return r.Form, nil
}

// apiVersionFromQuery returns the request's api-version query parameter,
// parsed and echoed but never branched on for response-body shape (except
// the metadata endpoint, which is hard-pinned to metadataAPIVersion) --
// AZURE.md section 10.1.
func apiVersionFromQuery(r *http.Request) string {
	return r.URL.Query().Get("api-version")
}

// azureARMReadHeaderTimeout bounds how long the server waits to read request
// headers, matching services/azureblob's own timeout value.
const azureARMReadHeaderTimeout = 5 * time.Second

// StartWorker binds AzureARM's dedicated fixed port and serves HTTPS with a
// self-signed certificate (pkgs/devtls), synchronously, failing fast if the
// port is unavailable rather than falling back into the shared PortAlloc
// pool -- exactly like services/azureblob/azurequeue/azuretable/cosmosdb's
// StartWorker (see AZURE.md section 10.7). Unlike those, ARM serves HTTPS
// unconditionally: azurerm's metadata_host handling hardcodes
// "https://" (AZURE.md section 10.8), so a plain-HTTP listener here would
// make provider initialization fail outright, not merely warn.
func (h *Handler) StartWorker(ctx context.Context) error {
	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf(":%d", h.Port))
	if err != nil {
		return fmt.Errorf("azurearm: bind port %d: %w", h.Port, err)
	}

	cert, err := devtls.GenerateSelfSignedCert()
	if err != nil {
		_ = listener.Close()

		return fmt.Errorf("azurearm: generate self-signed certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	tlsListener := tls.NewListener(listener, tlsConfig)

	e := echo.New()
	e.Use(logger.EchoMiddleware(logger.Load(ctx)))
	e.Any("/*", telemetry.WrapEchoHandler("AzureARM", h.Handler(), h))

	srv := &http.Server{
		Handler:           e,
		ReadHeaderTimeout: azureARMReadHeaderTimeout,
	}

	h.srvMu.Lock("StartWorker")
	h.srv = srv
	h.srvMu.Unlock()

	workerCtx := logger.WithWorker(ctx, "azurearm", "listener")
	log := logger.Load(workerCtx)

	log.InfoContext(workerCtx, "azurearm: starting dedicated HTTPS listener", "port", h.Port)

	go func() {
		if serveErr := srv.Serve(tlsListener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.ErrorContext(workerCtx, "azurearm: listener stopped", "error", serveErr)
		}
	}()

	return nil
}

// Shutdown stops the dedicated ARM listener, mirroring
// services/azureblob.Handler.Shutdown's graceful-then-forced-close shape.
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
		log.ErrorContext(ctx, "azurearm: graceful shutdown failed, forcing close", "error", err)

		if closeErr := srv.Close(); closeErr != nil {
			log.ErrorContext(ctx, "azurearm: forced close also failed", "error", closeErr)
		}
	}
}
