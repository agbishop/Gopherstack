package networkmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const networkManagerServiceName = "networkmanager"

var errUnknownPath = errors.New("unknown path")

// Handler is the HTTP handler for the AWS Network Manager API.
type Handler struct {
	Backend   *InMemoryBackend
	AccountID string
	Region    string
}

// NewHandler creates a new Network Manager handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	return &Handler{
		Backend:   backend,
		AccountID: backend.accountID,
		Region:    backend.region,
	}
}

// Name returns the service name.
func (h *Handler) Name() string { return "NetworkManager" }

// GetSupportedOperations returns every operation this handler routes -- all
// 95 operations on the aws-sdk-go-v2/service/networkmanager client, per
// sdk_completeness_test.go's empty exception list.
func (h *Handler) GetSupportedOperations() []string {
	table := h.routeTable()
	ops := make([]string, 0, len(table))
	seen := make(map[string]bool, len(table))

	for _, e := range table {
		if !seen[e.op] {
			seen[e.op] = true

			ops = append(ops, e.op)
		}
	}

	return ops
}

// Reset clears all stored state in the backend.
func (h *Handler) Reset() { h.Backend.Reset() }

// Shutdown stops the backend's scheduled state-transition timers so no timer
// goroutine outlives the service.
func (h *Handler) Shutdown(_ context.Context) { h.Backend.Close() }

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return networkManagerServiceName }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Region} }

// RouteMatcher returns a matcher that reports true iff the request resolves
// to a known route via matchRoute.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		segs := rawPathSegments(c.Request())

		// The tags/:arn pattern is a positional wildcard match, so it must
		// not claim another service's /tags/{arn} request -- only the ARN's
		// own service segment disambiguates the true owner.
		if len(segs) > 0 && segs[0] == "tags" {
			return httputils.MatchesTaggedResourceARN(c.Request().URL.Path, networkManagerServiceName)
		}

		_, _, ok := matchRoute(h.routeTable(), c.Request().Method, segs)

		return ok
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityPathVersioned }

// ExtractOperation extracts the operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	entry, _, ok := matchRoute(h.routeTable(), c.Request().Method, rawPathSegments(c.Request()))
	if !ok {
		return ""
	}

	return entry.op
}

// ExtractResource extracts the primary resource identifier from the
// request -- the ARN in the /tags/{resourceArn} or
// /resource-policy/{resourceArn} path, or empty for every other
// (mostly ID-addressed) operation.
func (h *Handler) ExtractResource(c *echo.Context) string {
	entry, params, ok := matchRoute(h.routeTable(), c.Request().Method, rawPathSegments(c.Request()))
	if !ok {
		return ""
	}

	if arn, hasArn := params["ResourceArn"]; hasArn &&
		(strings.HasPrefix(entry.pattern[0], "tags") || strings.HasPrefix(entry.pattern[0], "resource-policy")) {
		return arn
	}

	return ""
}

// routeParams is the set of path-parameter values extracted from a matched
// request path, keyed by the pattern's placeholder name (e.g.
// "GlobalNetworkId").
type routeParams map[string]string

type dispatchFunc func(ctx context.Context, r *http.Request, params routeParams, body []byte) ([]byte, error)

// route describes one (method, path-pattern) -> operation mapping. pattern
// segments starting with ":" are wildcards captured into routeParams under
// their name (sans the leading colon); every other segment must match
// literally.
type route struct {
	fn      dispatchFunc
	op      string
	method  string
	pattern []string
}

// matchRoute finds the route whose method and segment pattern match segs,
// returning its captured routeParams. Real REST resource paths (unlike
// mgn/resiliencehub's single-action-slug convention) need genuine
// positional path-parameter matching -- see PARITY.md's routing section:
// 36 GET/31 POST/19 DELETE/9 PATCH, with real nesting like
// "global-networks/{id}/devices/{id2}".
func matchRoute(table []route, method string, segs []string) (route, routeParams, bool) {
	for _, r := range table {
		if r.method != method || len(r.pattern) != len(segs) {
			continue
		}

		params, ok := matchPattern(r.pattern, segs)
		if ok {
			return r, params, true
		}
	}

	return route{}, nil, false
}

func matchPattern(pattern, segs []string) (routeParams, bool) {
	params := make(routeParams, len(pattern))

	for i, p := range pattern {
		if strings.HasPrefix(p, ":") {
			params[p[1:]] = segs[i]

			continue
		}

		if p != segs[i] {
			return nil, false
		}
	}

	return params, true
}

// Handler returns the Echo handler function for Network Manager requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "networkmanager: failed to read request body", "error", err)

			return h.handleError(c, err)
		}

		entry, params, ok := matchRoute(h.routeTable(), c.Request().Method, rawPathSegments(c.Request()))
		if !ok {
			return h.handleError(c, fmt.Errorf("%w: %s %s", errUnknownPath, c.Request().Method, c.Request().URL.Path))
		}

		result, dispErr := entry.fn(ctx, c.Request(), params, body)
		if dispErr != nil {
			log.ErrorContext(ctx, "networkmanager: operation failed", "op", entry.op, "error", dispErr)

			return h.handleError(c, dispErr)
		}

		if result == nil {
			return c.JSONBlob(http.StatusOK, []byte("{}"))
		}

		return c.JSONBlob(http.StatusOK, result)
	}
}

// handleError renders err as one of Network Manager's restjson1 error
// envelopes: {"Message": "..."} (PascalCase, matching this service's own
// wire convention -- see wire.go's doc comment) with an X-Amzn-Errortype
// header, plus the extra fields the concrete exception shape carries.
func (h *Handler) handleError(c *echo.Context, err error) error {
	status, errType := classifyError(err)

	body := map[string]any{"Message": err.Error()}

	if apiErr, ok := errors.AsType[*apiError](err); ok {
		addErrorFields(body, apiErr)
	}

	payload, _ := json.Marshal(body)

	c.Response().Header().Set("X-Amzn-Errortype", errType)

	return c.JSONBlob(status, payload)
}

// classifyError maps err to its HTTP status and wire exception type.
func classifyError(err error) (int, string) {
	switch {
	case errors.Is(err, errAccessDenied):
		return http.StatusForbidden, "AccessDeniedException"
	case errors.Is(err, errConflictSentinel):
		return http.StatusConflict, "ConflictException"
	case errors.Is(err, errCoreNetworkPolicy):
		return http.StatusBadRequest, "CoreNetworkPolicyException"
	case errors.Is(err, errInternalServer):
		return http.StatusInternalServerError, "InternalServerException"
	case errors.Is(err, errNotFoundSentinel):
		return http.StatusNotFound, "ResourceNotFoundException"
	case errors.Is(err, errQuotaExceeded):
		return http.StatusBadRequest, "ServiceQuotaExceededException"
	case errors.Is(err, errThrottling):
		return http.StatusTooManyRequests, "ThrottlingException"
	case errors.Is(err, errValidationSentinel):
		return http.StatusBadRequest, "ValidationException"
	case errors.Is(err, errUnknownPath):
		return http.StatusNotFound, "ResourceNotFoundException"
	default:
		return http.StatusInternalServerError, "InternalServerException"
	}
}

// addErrorFields populates body with apiErr's extra wire fields.
func addErrorFields(body map[string]any, apiErr *apiError) {
	if apiErr == nil {
		return
	}

	if apiErr.resourceID != "" {
		body["ResourceId"] = apiErr.resourceID
	}

	if apiErr.resourceType != "" {
		body["ResourceType"] = apiErr.resourceType
	}

	if apiErr.reason != "" {
		body["Reason"] = apiErr.reason
	}

	if apiErr.serviceCode != "" {
		body["ServiceCode"] = apiErr.serviceCode
	}

	if apiErr.limitCode != "" {
		body["LimitCode"] = apiErr.limitCode
	}

	if apiErr.policyErrors != nil {
		body["Errors"] = toCoreNetworkPolicyErrorsWire(apiErr.policyErrors)
	}
}

// rawPathSegments splits the raw (or decoded) URL path into non-empty
// segments, URL-decoding each segment individually so an encoded slash in
// an ARN-in-path parameter (e.g. /tags/{resourceArn},
// /customer-gateway-associations/{CustomerGatewayArn}) is preserved as a
// single segment -- matches services/mgn/outposts/resiliencehub/grafana's
// identically-named helper (the house fix for the ARN-in-path route-
// matching trap).
func rawPathSegments(r *http.Request) []string {
	rawPath := r.URL.RawPath
	if rawPath == "" {
		rawPath = r.URL.Path
	}

	rawPath = strings.TrimPrefix(rawPath, "/")
	parts := strings.Split(rawPath, "/")

	segments := make([]string, 0, len(parts))

	for _, p := range parts {
		if p == "" {
			continue
		}

		decoded, err := url.PathUnescape(p)
		if err != nil {
			decoded = p
		}

		segments = append(segments, decoded)
	}

	return segments
}

// queryMaxResults parses the "maxResults" query parameter, defaulting to
// defaultPageLimit when absent or invalid.
func queryMaxResults(q url.Values) int {
	raw := q.Get("maxResults")
	if raw == "" {
		return defaultPageLimit
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultPageLimit
	}

	return n
}

func queryNextToken(q url.Values) string { return q.Get("nextToken") }

// decodeJSONBody unmarshals body into v, treating an empty body as a no-op
// (leaving v at its zero value) rather than a JSON error -- several
// operations in this service take no JSON body fields at all (e.g.
// DeleteGlobalNetwork).
func decodeJSONBody(body []byte, v any) error {
	if len(body) == 0 {
		return nil
	}

	if err := json.Unmarshal(body, v); err != nil {
		return validationError("malformed request body: " + err.Error())
	}

	return nil
}

// marshalResponse marshals v, wrapping any error as an internal failure
// (marshaling our own well-typed wire structs should never fail).
func marshalResponse(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, internalServerError("networkmanager: marshal response: " + err.Error())
	}

	return data, nil
}

// routeTable builds the full (method, pattern) -> operation table by
// concatenating every family's own route slice -- one family per
// handler_<family>.go file, each named for the PARITY.md family table it
// implements (all 95 operations, none missing).
func (h *Handler) routeTable() []route {
	return concatRoutes(
		h.globalNetworksRoutes(),
		h.associationsRoutes(),
		h.connectPeersRoutes(),
		h.coreNetworksRoutes(),
		h.attachmentsRoutes(),
		h.peeringsRoutes(),
		h.routeAnalysisRoutes(),
		h.introspectionRoutes(),
		h.orgAccessRoutes(),
		h.resourcePolicyRoutes(),
		h.taggingRoutes(),
	)
}

// concatRoutes flattens groups into one preallocated slice -- shared by
// routeTable and every multi-sub-family route builder
// (globalNetworksRoutes/coreNetworksRoutes/attachmentsRoutes) so none of
// them repeats the same sum-then-append boilerplate.
func concatRoutes(groups ...[]route) []route {
	total := 0
	for _, g := range groups {
		total += len(g)
	}

	all := make([]route, 0, total)
	for _, g := range groups {
		all = append(all, g...)
	}

	return all
}
