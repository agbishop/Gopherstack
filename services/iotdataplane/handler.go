package iotdataplane

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	iotDPMatchPriority = 88
	// maxPublishBodyBytes limits the size of MQTT publish request bodies.
	maxPublishBodyBytes = 128 * 1024
	// maxShadowBodyBytes limits the size of shadow document request bodies.
	maxShadowBodyBytes = 8 * 1024
	// retainedMessagePath is the URL path prefix for retained message operations.
	retainedMessagePath = "/retainedMessage"
	// retainedMessagePathSlash is the prefix used to match individual topic paths.
	retainedMessagePathSlash = retainedMessagePath + "/"
	// listThingsWithShadowsPath is the admin path for listing things that have shadows.
	listThingsWithShadowsPath = "/api/things/shadow/ListThingsWithShadows"
	// listNamedShadowsPrefix is the prefix for ListNamedShadowsForThing.
	listNamedShadowsPrefix = "/api/things/shadow/ListNamedShadowsForThing/"
	// adminConnectionsPath is the Gopherstack-only admin path for managing
	// connections via RegisterConnection/ListConnections, which have no real
	// AWS iotdataplane equivalent. Prefixed with /_admin/ to distinguish
	// from the AWS API namespace.
	adminConnectionsPath = "/_admin/connections"
	// adminConnectionsPathSlash is the prefix for individual connection operations.
	adminConnectionsPathSlash = adminConnectionsPath + "/"
	// connectionsPath is the real AWS wire path shared by GetConnection,
	// DeleteConnection, ListSubscriptions, and SendDirectMessage (all rooted
	// at /connections/{clientId}; see aws-sdk-go-v2/service/iotdataplane's
	// serializers for {Get,Delete}ConnectionInput,
	// {List}SubscriptionsInput, and SendDirectMessageInput). Unlike
	// RegisterConnection/ListConnections, these ARE real published AWS
	// operations, so they must remain reachable at their real wire paths in
	// addition to the gopherstack admin alias below (which only serves the
	// two non-AWS ops).
	connectionsPath = "/connections"
	// connectionsPathSlash is the prefix for individual connections at the real path.
	connectionsPathSlash = connectionsPath + "/"
	// connectionsSubSubscriptions is the ListSubscriptions sub-resource
	// segment: GET /connections/{clientId}/subscriptions.
	connectionsSubSubscriptions = "subscriptions"
	// connectionsSubMessages is the SendDirectMessage sub-resource segment:
	// POST /connections/{clientId}/messages.
	connectionsSubMessages = "messages"
	// iotDataServiceName is the SigV4 signing name for IoT Data Plane. The
	// real "/connections/{id}" wire path collides with Outposts'
	// GetConnection (see gopherstack-vpoh); RouteMatcher uses this to
	// disambiguate via httputils.ScopedPrefixMatch instead of a bare prefix.
	iotDataServiceName = "iotdata"
	// defaultPageSize is the default number of items returned per page (AWS default).
	defaultPageSize = 25
	// maxPageSize is the maximum number of items returned per page (AWS cap).
	maxPageSize = 100
	// defaultSubscriptionsPageSize is ListSubscriptions' own documented
	// default, distinct from defaultPageSize: "The maximum number of
	// subscriptions to return in a single request. By default, this is set
	// to 20." (aws-sdk-go-v2/service/iotdataplane@v1.35.4's
	// ListSubscriptionsInput.MaxResults doc comment).
	defaultSubscriptionsPageSize = 20

	keyError   = "error"
	keyMessage = "message"
	// errMethodNotAllowed is the AWS iotdataplane wire error code for a
	// combination of HTTP verb and URI that isn't supported (see
	// aws-sdk-go-v2/service/iotdataplane/types.MethodNotAllowedException).
	errMethodNotAllowed = "MethodNotAllowedException"
	opUnknown           = "Unknown"
	opDeleteConnection  = "DeleteConnection"
	opGetConnection     = "GetConnection"
	opListSubscriptions = "ListSubscriptions"
	opSendDirectMessage = "SendDirectMessage"

	// shadowPathParts is the number of parts when splitting a shadow URL on "/shadow".
	shadowPathParts = 2
)

// Handler is the Echo HTTP handler for IoT Data Plane operations.
type Handler struct {
	Backend StorageBackend
}

// NewHandler creates a new IoT Data Plane Handler.
func NewHandler(backend StorageBackend) *Handler {
	return &Handler{Backend: backend}
}

// Reset clears all handler state by delegating to the backend.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Name returns the service name.
func (h *Handler) Name() string { return "IoTDataPlane" }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opDeleteConnection,
		"DeleteThingShadow",
		opGetConnection,
		"GetRetainedMessage",
		"GetThingShadow",
		"ListConnections",
		"ListNamedShadowsForThing",
		"ListRetainedMessages",
		opListSubscriptions,
		"ListThingsWithShadows",
		"Publish",
		"RegisterConnection",
		opSendDirectMessage,
		"UpdateThingShadow",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return iotDataServiceName }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this IoT Data Plane instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a function matching IoT Data Plane requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		if strings.HasPrefix(path, "/topics/") ||
			isShadowPath(path) ||
			strings.HasPrefix(path, listNamedShadowsPrefix) ||
			path == listThingsWithShadowsPath ||
			path == adminConnectionsPath ||
			strings.HasPrefix(path, adminConnectionsPathSlash) ||
			path == retainedMessagePath ||
			strings.HasPrefix(path, retainedMessagePathSlash) {
			return true
		}

		// connectionsPathSlash ("/connections/") is also Outposts' real
		// GetConnection wire path -- gate on the SigV4 scope so a
		// correctly-signed Outposts request isn't swallowed here.
		return connectionsWireOperation(path, c.Request().Method) != "" &&
			httputils.ScopedPrefixMatch(c.Request(), path, connectionsPathSlash, iotDataServiceName)
	}
}

// isShadowPath returns true for paths of the form:
//   - /things/{thingName}/shadow                  (classic shadow)
//   - /things/{thingName}/shadow?name=...          (named shadow via query)
//   - /things/{thingName}/shadow/name/{shadowName} (named shadow via path)
func isShadowPath(path string) bool {
	after, ok := strings.CutPrefix(path, "/things/")
	if !ok {
		return false
	}

	// Accept /things/{name}/shadow/name/{shadowName} (path-style named shadow).
	if before, _, found := strings.Cut(after, "/shadow/name/"); found {
		// thingName must be non-empty.
		return len(before) > 0
	}

	// Must end with exactly /shadow or /shadow?... (no other trailing segments).
	parts := strings.SplitN(after, "/shadow", shadowPathParts)
	if len(parts) != shadowPathParts {
		return false
	}

	tail := parts[1]

	return tail == "" || strings.HasPrefix(tail, "?")
}

// MatchPriority returns the routing priority for the IoT Data Plane handler.
func (h *Handler) MatchPriority() int { return iotDPMatchPriority }

// splitConnectionsWirePath splits a real AWS "/connections/{clientId}[/sub]"
// path into its clientId and optional sub-resource segment (either "",
// connectionsSubSubscriptions, or connectionsSubMessages). ok is false when
// path doesn't have the connectionsPathSlash prefix or clientId is empty
// (e.g. bare "/connections/" or "/connections").
func splitConnectionsWirePath(path string) (string, string, bool) {
	after, hasPrefix := strings.CutPrefix(path, connectionsPathSlash)
	if !hasPrefix || after == "" {
		return "", "", false
	}

	clientID, sub, _ := strings.Cut(after, "/")

	return clientID, sub, clientID != ""
}

// connectionsWireOperation returns the AWS operation name for a real
// "/connections/..." wire-path request: GetConnection (GET, no sub-resource),
// DeleteConnection (DELETE, no sub-resource), ListSubscriptions (GET
// .../subscriptions), or SendDirectMessage (POST .../messages). Returns ""
// for any other method/path combination on this prefix -- notably POST
// /connections/{clientId} with no sub-resource, which has no real AWS
// equivalent (RegisterConnection only exists at the gopherstack-only
// adminConnectionsPath; see admin-only-extensions family in PARITY.md) and
// must keep falling through to the generic 404.
func connectionsWireOperation(path, method string) string {
	_, sub, ok := splitConnectionsWirePath(path)
	if !ok {
		return ""
	}

	switch {
	case sub == "" && method == http.MethodGet:
		return opGetConnection
	case sub == "" && method == http.MethodDelete:
		return opDeleteConnection
	case sub == connectionsSubSubscriptions && method == http.MethodGet:
		return opListSubscriptions
	case sub == connectionsSubMessages && method == http.MethodPost:
		return opSendDirectMessage
	default:
		return ""
	}
}

// extractConnectionClientID returns the clientId path segment for either the
// gopherstack admin connections path or a real AWS connections wire path
// (GetConnection/DeleteConnection/ListSubscriptions/SendDirectMessage).
func extractConnectionClientID(path string) string {
	if after, ok := strings.CutPrefix(path, adminConnectionsPathSlash); ok {
		return after
	}

	clientID, _, _ := splitConnectionsWirePath(path)

	return clientID
}

// extractConnectionOperation returns the operation name for /_admin/connections paths.
func extractConnectionOperation(path, method string) string {
	if path == adminConnectionsPath {
		if method == http.MethodGet {
			return "ListConnections"
		}

		return opUnknown
	}

	switch method {
	case http.MethodDelete:
		return opDeleteConnection
	case http.MethodPost:
		return "RegisterConnection"
	}

	return opUnknown
}

// extractShadowOperation returns the operation name for /things/{name}/shadow paths.
func extractShadowOperation(method string) string {
	switch method {
	case http.MethodGet:
		return "GetThingShadow"
	case http.MethodPost:
		return "UpdateThingShadow"
	case http.MethodDelete:
		return "DeleteThingShadow"
	}

	return opUnknown
}

// ExtractOperation returns the operation name.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	path := c.Request().URL.Path
	method := c.Request().Method
	switch {
	case strings.HasPrefix(path, "/topics/"):
		return "Publish"
	case strings.HasPrefix(path, listNamedShadowsPrefix):
		return "ListNamedShadowsForThing"
	case path == listThingsWithShadowsPath:
		return "ListThingsWithShadows"
	case path == adminConnectionsPath || strings.HasPrefix(path, adminConnectionsPathSlash):
		return extractConnectionOperation(path, method)
	case connectionsWireOperation(path, method) != "":
		return connectionsWireOperation(path, method)
	case path == retainedMessagePath && method == http.MethodGet:
		return "ListRetainedMessages"
	case strings.HasPrefix(path, retainedMessagePathSlash) && method == http.MethodGet:
		return "GetRetainedMessage"
	case isShadowPath(path):
		return extractShadowOperation(method)
	}

	return opUnknown
}

// ExtractResource extracts the topic or thing name from the URL path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	path := c.Request().URL.Path
	if after, ok := strings.CutPrefix(path, "/topics/"); ok {
		return after
	}

	if after, ok := strings.CutPrefix(path, listNamedShadowsPrefix); ok {
		return after
	}

	if path == listThingsWithShadowsPath {
		return ""
	}

	if path == adminConnectionsPath {
		return ""
	}

	if after, ok := strings.CutPrefix(path, adminConnectionsPathSlash); ok {
		return after
	}

	if clientID, _, ok := splitConnectionsWirePath(path); ok {
		return clientID
	}

	if path == retainedMessagePath {
		return ""
	}

	if after, ok := strings.CutPrefix(path, retainedMessagePathSlash); ok {
		return after
	}

	thingName, _ := parseShadowPath(path)

	return thingName
}

// parseShadowPath extracts thingName and shadowName from shadow URL paths.
// Supports both /things/{name}/shadow?name=... and /things/{name}/shadow/name/{shadowName}.
// shadowName is empty for the classic (unnamed) shadow.
func parseShadowPath(path string) (string, string) {
	trimmed := strings.TrimPrefix(path, "/things/")

	// Path-style named shadow: /things/{name}/shadow/name/{shadowName}
	if before, after, ok := strings.Cut(trimmed, "/shadow/name/"); ok {
		return before, after
	}

	// Classic or query-param named shadow: /things/{name}/shadow
	parts := strings.SplitN(trimmed, "/shadow", shadowPathParts)

	return parts[0], ""
}

// amznErrorTypeHeader carries the modeled exception type for the restjson1
// protocol. aws-sdk-go-v2's restjson.GetErrorInfo (aws/protocol/restjson/decoder_util.go)
// reads this header before falling back to a body "code"/"__type" field -- it does NOT
// read the "error" key this package's response bodies use for the type (that key is
// asserted by existing tests, so it stays; the header is additive), so without it every
// error here deserialized client-side as a generic UnknownError.
const amznErrorTypeHeader = "X-Amzn-Errortype"

// handleError maps backend errors to appropriate HTTP status codes. Types
// are verified per sentinel against this service's own deserializer error
// lists (iotdataplane@v1.35.4 deserializers.go), which use
// strings.EqualFold comparisons rather than literal case labels.
// ErrConnectionExists is the one exception: RegisterConnection is a
// gopherstack-only admin extension with no real AWS operation (see
// adminConnectionsPath's doc comment), so "ResourceAlreadyExistsException"
// isn't verified against any deserializer list -- no real SDK client can
// reach it.
func (h *Handler) handleError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrShadowNotFound),
		errors.Is(err, ErrRetainedMessageNotFound),
		errors.Is(err, ErrConnectionNotFound):
		c.Response().Header().Set(amznErrorTypeHeader, "ResourceNotFoundException")

		return c.JSON(http.StatusNotFound, map[string]string{
			keyError:   "ResourceNotFoundException",
			keyMessage: err.Error(),
		})
	case errors.Is(err, ErrVersionConflict):
		c.Response().Header().Set(amznErrorTypeHeader, "ConflictException")

		return c.JSON(http.StatusConflict, map[string]any{
			"code":     http.StatusConflict,
			keyError:   "ConflictException",
			keyMessage: err.Error(),
		})
	case errors.Is(err, ErrRequestTooLarge):
		c.Response().Header().Set(amznErrorTypeHeader, "RequestEntityTooLargeException")

		return c.JSON(http.StatusRequestEntityTooLarge, map[string]string{
			keyError:   "RequestEntityTooLargeException",
			keyMessage: err.Error(),
		})
	case errors.Is(err, ErrValidation):
		c.Response().Header().Set(amznErrorTypeHeader, "InvalidRequestException")

		return c.JSON(http.StatusBadRequest, map[string]string{
			keyError:   "InvalidRequestException",
			keyMessage: err.Error(),
		})
	case errors.Is(err, ErrConnectionExists):
		c.Response().Header().Set(amznErrorTypeHeader, "ResourceAlreadyExistsException")

		return c.JSON(http.StatusConflict, map[string]string{
			keyError:   "ResourceAlreadyExistsException",
			keyMessage: err.Error(),
		})
	default:
		c.Response().Header().Set(amznErrorTypeHeader, "InternalFailureException")

		return c.JSON(http.StatusInternalServerError, map[string]string{
			keyError:   "InternalFailureException",
			keyMessage: err.Error(),
		})
	}
}

// methodNotAllowedResponse writes the wire-accurate response for a
// combination of HTTP verb and URI this operation doesn't support.
// MethodNotAllowedException is modeled on every real iotdataplane operation
// that reaches this helper (iotdataplane@v1.35.4 deserializers.go).
func methodNotAllowedResponse(c *echo.Context) error {
	c.Response().Header().Set(amznErrorTypeHeader, errMethodNotAllowed)

	return c.JSON(http.StatusMethodNotAllowed, map[string]string{keyError: errMethodNotAllowed})
}

// invalidRequestResponse writes the wire-accurate response for a malformed
// request that never reaches the backend (so there's no sentinel error to
// route through handleError). InvalidRequestException is modeled on every
// real iotdataplane operation that reaches this helper.
func invalidRequestResponse(c *echo.Context, message string) error {
	c.Response().Header().Set(amznErrorTypeHeader, "InvalidRequestException")

	return c.JSON(http.StatusBadRequest, map[string]string{keyError: message})
}

// Handler returns the Echo handler function for IoT Data Plane operations.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		path := c.Request().URL.Path
		switch {
		case strings.HasPrefix(path, "/topics/"):
			return h.handlePublish(c)
		case strings.HasPrefix(path, listNamedShadowsPrefix):
			return h.handleListNamedShadows(c)
		case path == listThingsWithShadowsPath:
			return h.handleListThingsWithShadows(c)
		case path == adminConnectionsPath:
			return h.handleConnections(c)
		case strings.HasPrefix(path, adminConnectionsPathSlash):
			return h.handleConnectionByID(c)
		case connectionsWireOperation(path, c.Request().Method) != "":
			return h.handleConnectionsWire(c)
		case path == retainedMessagePath:
			return h.handleListRetainedMessages(c)
		case strings.HasPrefix(path, retainedMessagePathSlash):
			return h.handleGetRetainedMessage(c)
		case isShadowPath(path):
			return h.handleShadow(c)
		default:
			return c.JSON(http.StatusNotFound, map[string]string{keyError: "not found"})
		}
	}
}

// parsePageSize extracts the pagination page size from query params.
// pageSize is the primary parameter (AWS convention); maxResults is accepted as an alias.
// Returns the effective page size clamped to [1, maxPageSize], or defaultSize when absent.
func parsePageSize(q interface{ Get(string) string }, defaultSize int) int {
	// pageSize takes precedence over maxResults (AWS convention).
	raw := q.Get("pageSize")
	if raw == "" {
		raw = q.Get("maxResults")
	}

	if raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			if v > maxPageSize {
				return maxPageSize
			}

			return v
		}
	}

	return defaultSize
}

// findCursorIndex returns the start index for the given nextToken cursor in items.
// nextToken holds the first item of the next page (set to items[end] at write time),
// so the cursor item IS the first item of the new page — we start AT it (not after it).
// Returns 0 if the cursor is not found or is empty.
func findCursorIndex(items []string, cursor string) int {
	if cursor == "" {
		return 0
	}

	for i, item := range items {
		if item == cursor {
			return i
		}
	}

	return 0
}
