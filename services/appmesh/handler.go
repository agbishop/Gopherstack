package appmesh

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	appmeshPathPrefix = "/v20190125/"

	pathSegMeshes         = "meshes"
	pathSegVirtualNodes   = "virtualNodes"
	pathSegVirtualRouters = "virtualRouters"
	// Route paths use singular "virtualRouter" (AWS API quirk).
	pathSegVirtualRouter = "virtualRouter"
	pathSegRoutes        = "routes"
	pathSegVirtualSvcs   = "virtualServices"
	pathSegVirtualGWs    = "virtualGateways"
	// Gateway route paths use singular "virtualGateway" (AWS API quirk).
	pathSegVirtualGW     = "virtualGateway"
	pathSegGatewayRoutes = "gatewayRoutes"
	pathSegTags          = "tags"
	pathSegTag           = "tag"
	pathSegUntag         = "untag"

	keyCode    = "code"
	keyMessage = "message"

	defaultMaxResults = 100

	keyArn                = "arn"
	keyCreatedAt          = "createdAt"
	keyLastUpdatedAt      = "lastUpdatedAt"
	keyMeshOwner          = "meshOwner"
	keyResourceOwner      = "resourceOwner"
	keyVersion            = "version"
	keyMeshName           = "meshName"
	keyMetadata           = "metadata"
	keySpec               = "spec"
	keyStatus             = "status"
	keyVirtualRouterName  = "virtualRouterName"
	keyVirtualGatewayName = "virtualGatewayName"
	keyVirtualNodeName    = "virtualNodeName"
	keyVirtualServiceName = "virtualServiceName"
	keyRouteName          = "routeName"
	keyGatewayRouteName   = "gatewayRouteName"
	opUnknown             = "Unknown"

	// Path segment counts for URL depth matching.
	segsCollection       = 1 // /meshes
	segsSingle           = 2 // /meshes/{name}
	segsSubCollection    = 3 // /meshes/{name}/virtualNodes
	segsSubSingle        = 4 // /meshes/{name}/virtualNodes/{id} or min for nested
	segsNestedCollection = 5 // /meshes/{name}/virtualRouter/{id}/routes
	segsNestedSingle     = 6 // /meshes/{name}/virtualRouter/{id}/routes/{id}
)

// Handler handles App Mesh HTTP requests.
type Handler struct {
	Backend StorageBackend
}

// NewHandler constructs a new Handler.
func NewHandler(b StorageBackend) *Handler {
	return &Handler{Backend: b}
}

// Name returns the service name.
func (h *Handler) Name() string { return "AppMesh" }

// Reset resets the backend.
func (h *Handler) Reset() { h.Backend.Reset() }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"CreateMesh",
		"DescribeMesh",
		"UpdateMesh",
		"DeleteMesh",
		"ListMeshes",
		"CreateVirtualNode",
		"DescribeVirtualNode",
		"UpdateVirtualNode",
		"DeleteVirtualNode",
		"ListVirtualNodes",
		"CreateVirtualRouter",
		"DescribeVirtualRouter",
		"UpdateVirtualRouter",
		"DeleteVirtualRouter",
		"ListVirtualRouters",
		"CreateRoute",
		"DescribeRoute",
		"UpdateRoute",
		"DeleteRoute",
		"ListRoutes",
		"CreateVirtualService",
		"DescribeVirtualService",
		"UpdateVirtualService",
		"DeleteVirtualService",
		"ListVirtualServices",
		"CreateVirtualGateway",
		"DescribeVirtualGateway",
		"UpdateVirtualGateway",
		"DeleteVirtualGateway",
		"ListVirtualGateways",
		"CreateGatewayRoute",
		"DescribeGatewayRoute",
		"UpdateGatewayRoute",
		"DeleteGatewayRoute",
		"ListGatewayRoutes",
		"TagResource",
		"UntagResource",
		"ListTagsForResource",
	}
}

// RouteMatcher returns a function that matches App Mesh REST API requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().URL.Path, appmeshPathPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityPathVersioned }

// ExtractOperation returns the operation name for the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return parseOperation(c.Request().Method, c.Request().URL.Path)
}

// ExtractResource returns the primary resource identifier.
func (h *Handler) ExtractResource(c *echo.Context) string {
	segs := splitPath(c.Request().URL.Path)
	if len(segs) >= segsSingle && segs[0] == pathSegMeshes {
		return segs[1]
	}

	return ""
}

// Handler returns the echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		log := logger.Load(c.Request().Context())
		method := c.Request().Method
		path := c.Request().URL.Path
		segs := splitPath(path)
		log.DebugContext(c.Request().Context(), "AppMesh request", "method", method, "path", path)

		if len(segs) == 0 {
			return c.JSON(http.StatusNotFound, errResp("NotFoundException", "not found"))
		}

		switch segs[0] {
		case pathSegTags:

			return h.handleListTags(c)
		case pathSegTag:

			return h.handleTagResource(c)
		case pathSegUntag:

			return h.handleUntagResource(c)
		case pathSegMeshes:

			return h.handleMeshes(c, segs)
		}

		return c.JSON(http.StatusNotFound, errResp("NotFoundException", "not found"))
	}
}

// ─── Wire serialization (shared) ───

func metaToWire(m ResourceMeta) map[string]any {
	return map[string]any{
		keyArn:           m.Arn,
		keyCreatedAt:     m.CreatedAt.Unix(),
		keyLastUpdatedAt: m.UpdatedAt.Unix(),
		keyMeshOwner:     m.MeshOwner,
		keyResourceOwner: m.ResourceOwner,
		"uid":            m.UID,
		keyVersion:       m.Version,
	}
}

// specOrEmpty returns a JSON object, ensuring we never return null.
func specOrEmpty(spec json.RawMessage) any {
	if len(spec) == 0 {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(spec, &v); err != nil {
		return map[string]any{}
	}

	return v
}

// ─── Request/response helpers ───

type tagInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func tagsToMap(tags []tagInput) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[t.Key] = t.Value
	}

	return m
}

// maxResourceNameLen is the real App Mesh API's ResourceName shape max length
// (botocore service-2.json: {"type": "string", "max": 255, "min": 1}), shared
// by meshName/virtualNodeName/virtualRouterName/routeName/virtualServiceName/
// virtualGatewayName/gatewayRouteName.
const maxResourceNameLen = 255

// isValidResourceName reports whether name satisfies the real API's
// ResourceName length constraints (1-255 chars). Names outside this range are
// rejected with BadRequestException rather than silently accepted.
func isValidResourceName(name string) bool {
	return len(name) >= 1 && len(name) <= maxResourceNameLen
}

func listParams(c *echo.Context) (int32, string) {
	nextToken := c.QueryParam("nextToken")
	maxResults := int32(defaultMaxResults)
	// AWS App Mesh list operations bind max page size to the "limit" query
	// param (see e.g. ListMeshesInput.Limit), not "maxResults".
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if limit, err := strconv.ParseInt(limitStr, 10, 32); err == nil && limit > 0 {
			maxResults = int32(limit)
		}
	}

	return maxResults, nextToken
}

func listResp(key string, items []any, nextToken string) map[string]any {
	resp := map[string]any{key: items}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return resp
}

func errResp(code, message string) map[string]any {
	return map[string]any{keyCode: code, keyMessage: message}
}

func methodNotAllowed(c *echo.Context) error {
	return c.JSON(http.StatusMethodNotAllowed, errResp("MethodNotAllowedException", "method not allowed"))
}

// checkMeshOwner rejects a request whose meshOwner query parameter names an
// account other than this backend's single account. Omitted, or equal to the
// caller's own account, is the documented default and passes through.
func (h *Handler) checkMeshOwner(c *echo.Context) error {
	owner := c.QueryParam(keyMeshOwner)
	if owner == "" || owner == h.Backend.AccountID() {
		return nil
	}

	return ErrMeshOwnerMismatch
}

// mapErr maps backend errors to the correct HTTP status codes.
func (h *Handler) mapErr(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, awserr.ErrNotFound):

		return c.JSON(http.StatusNotFound, errResp("NotFoundException", err.Error()))
	case errors.Is(err, awserr.ErrAlreadyExists):

		return c.JSON(http.StatusConflict, errResp("ConflictException", err.Error()))
	case errors.Is(err, awserr.ErrConflict):

		return c.JSON(http.StatusConflict, errResp("ResourceInUseException", err.Error()))
	case errors.Is(err, ErrTooManyTags):

		return c.JSON(http.StatusBadRequest, errResp("TooManyTagsException", err.Error()))
	case errors.Is(err, ErrMeshOwnerMismatch):

		return c.JSON(http.StatusForbidden, errResp("ForbiddenException", err.Error()))
	case errors.Is(err, awserr.ErrInvalidParameter):

		return c.JSON(http.StatusBadRequest, errResp("BadRequestException", err.Error()))
	default:

		return c.JSON(http.StatusInternalServerError, errResp("InternalServerErrorException", err.Error()))
	}
}

// ─── Path parsing / operation-name resolution ───

// splitPath splits a /v20190125/... path into segments after the version prefix.
func splitPath(path string) []string {
	path = strings.TrimPrefix(path, "/v20190125/")
	path = strings.TrimPrefix(path, "/v20190125")
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	var segs []string
	for _, p := range parts {
		if p != "" {
			segs = append(segs, p)
		}
	}

	return segs
}

// parseOperation derives the operation name for observability.
func parseOperation(method, path string) string {
	segs := splitPath(path)
	if len(segs) == 0 {
		return opUnknown
	}

	switch segs[0] {
	case pathSegTags:

		return "ListTagsForResource"
	case pathSegTag:

		return "TagResource"
	case pathSegUntag:

		return "UntagResource"
	case pathSegMeshes:

		return parseMeshOp(method, segs)
	}

	return opUnknown
}

func parseMeshOp(method string, segs []string) string {
	if op := parseMeshTopLevel(method, segs); op != "" {
		return op
	}
	if len(segs) < segsSubCollection {
		return opUnknown
	}

	return parseMeshSubResource(method, segs)
}

// parseMeshTopLevel handles /meshes and /meshes/{name}.
func parseMeshTopLevel(method string, segs []string) string {
	switch len(segs) {
	case segsCollection:
		switch method {
		case http.MethodPut:

			return "CreateMesh"
		case http.MethodGet:

			return "ListMeshes"
		}
	case segsSingle:
		switch method {
		case http.MethodGet:

			return "DescribeMesh"
		case http.MethodPut:

			return "UpdateMesh"
		case http.MethodDelete:

			return "DeleteMesh"
		}
	}

	return ""
}

// parseMeshSubResource handles sub-resources under /meshes/{name}/.
func parseMeshSubResource(method string, segs []string) string {
	switch segs[2] {
	case pathSegVirtualNodes:

		return parseSubOp(method, segs, segsSubCollection, "VirtualNode", "VirtualNodes")
	case pathSegVirtualRouters:

		return parseSubOp(method, segs, segsSubCollection, "VirtualRouter", "VirtualRouters")
	case pathSegVirtualRouter:
		if len(segs) >= segsNestedCollection && segs[4] == pathSegRoutes {
			return parseSubOp(method, segs, segsNestedCollection, "Route", "Routes")
		}
	case pathSegVirtualSvcs:

		return parseSubOp(method, segs, segsSubCollection, "VirtualService", "VirtualServices")
	case pathSegVirtualGWs:

		return parseSubOp(method, segs, segsSubCollection, "VirtualGateway", "VirtualGateways")
	case pathSegVirtualGW:
		if len(segs) >= segsNestedCollection && segs[4] == pathSegGatewayRoutes {
			return parseSubOp(method, segs, segsNestedCollection, "GatewayRoute", "GatewayRoutes")
		}
	}

	return opUnknown
}

// parseSubOp returns an operation name for a sub-resource segment starting at idx.
func parseSubOp(method string, segs []string, idx int, singular, plural string) string {
	if len(segs) == idx {
		switch method {
		case http.MethodPut:

			return "Create" + singular
		case http.MethodGet:

			return "List" + plural
		}
	} else {
		switch method {
		case http.MethodGet:

			return "Describe" + singular
		case http.MethodPut:

			return "Update" + singular
		case http.MethodDelete:

			return "Delete" + singular
		}
	}

	return opUnknown
}
