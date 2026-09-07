package efs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	opUnknown = "Unknown"
	keyTags   = "Tags"
)

const (
	opUpdateFileSystem          = "UpdateFileSystem"
	opTagResource               = "TagResource"
	opPutLifecycleConfiguration = "PutLifecycleConfiguration"
	opPutBackupPolicy           = "PutBackupPolicy"
	opPutFileSystemPolicy       = "PutFileSystemPolicy"
)

const (
	keyFileSystemID   = "FileSystemId"
	keyLifeCycleState = "LifeCycleState"
	keyOwnerID        = "OwnerId"
	keyStatus         = "Status"
)

const (
	opCreateFileSystem                  = "CreateFileSystem"
	opDescribeFileSystems               = "DescribeFileSystems"
	opDeleteFileSystem                  = "DeleteFileSystem"
	opCreateMountTarget                 = "CreateMountTarget"
	opDescribeMountTargets              = "DescribeMountTargets"
	opDeleteMountTarget                 = "DeleteMountTarget"
	opCreateAccessPoint                 = "CreateAccessPoint"
	opDescribeAccessPoints              = "DescribeAccessPoints"
	opDeleteAccessPoint                 = "DeleteAccessPoint"
	opListTagsForResource               = "ListTagsForResource"
	opDescribeLifecycleConfiguration    = "DescribeLifecycleConfiguration"
	opCreateReplicationConfiguration    = "CreateReplicationConfiguration"
	opCreateTags                        = "CreateTags"
	opDeleteFileSystemPolicy            = "DeleteFileSystemPolicy"
	opDeleteReplicationConfiguration    = "DeleteReplicationConfiguration"
	opDeleteTags                        = "DeleteTags"
	opDescribeAccountPreferences        = "DescribeAccountPreferences"
	opDescribeBackupPolicy              = "DescribeBackupPolicy"
	opDescribeFileSystemPolicy          = "DescribeFileSystemPolicy"
	opDescribeMountTargetSecurityGroups = "DescribeMountTargetSecurityGroups"
	opDescribeReplicationConfigurations = "DescribeReplicationConfigurations"
	opDescribeTags                      = "DescribeTags"
	opModifyMountTargetSecurityGroups   = "ModifyMountTargetSecurityGroups"
	opPutAccountPreferences             = "PutAccountPreferences"
	opUntagResource                     = "UntagResource"
	opUpdateFileSystemProtection        = "UpdateFileSystemProtection"
)

const (
	efsMatchPriority = service.PriorityPathVersioned

	pathFileSystems  = "/2015-02-01/file-systems"
	pathMountTargets = "/2015-02-01/mount-targets"
	pathAccessPoints = "/2015-02-01/access-points"
	// pathTags is the legacy DescribeTags path ("/2015-02-01/tags/{FileSystemId}",
	// GET only). It is distinct from pathResourceTags: real aws-sdk-go-v2 sends
	// TagResource/UntagResource/ListTagsForResource to "/2015-02-01/resource-tags/{ResourceId}"
	// (see serializers.go in aws-sdk-go-v2/service/efs), not under pathTags. Routing
	// those three ops under pathTags -- as this handler previously did -- makes them
	// unreachable by real SDK clients, since the RouteMatcher never sees a request
	// land on "/2015-02-01/tags/...".
	pathTags         = "/2015-02-01/tags"
	pathResourceTags = "/2015-02-01/resource-tags"
	pathCreateTags   = "/2015-02-01/create-tags"
	pathDeleteTags   = "/2015-02-01/delete-tags"
	pathAccountPrefs = "/2015-02-01/account-preferences"

	// subresourcePathParts is the number of segments when splitting a path with a sub-resource.
	subresourcePathParts = 2

	defaultMaxItems = 10
)

// Handler is the Echo HTTP handler for AWS EFS operations (REST-JSON protocol).
type Handler struct {
	Backend *InMemoryBackend
	ops     map[string]struct{}
}

// NewHandler creates a new EFS handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{Backend: backend}
	h.buildOps()

	return h
}

// buildOps pre-builds the set of supported operation names for fast lookup.
func (h *Handler) buildOps() {
	supported := h.GetSupportedOperations()
	h.ops = make(map[string]struct{}, len(supported))

	for _, op := range supported {
		h.ops[op] = struct{}{}
	}
}

// Reset clears all backend state.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// Name returns the service name.
func (h *Handler) Name() string { return "EFS" }

// GetSupportedOperations returns the list of supported EFS operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateFileSystem,
		opDescribeFileSystems,
		opDeleteFileSystem,
		opUpdateFileSystem,
		opCreateMountTarget,
		opDescribeMountTargets,
		opDeleteMountTarget,
		opCreateAccessPoint,
		opDescribeAccessPoints,
		opDeleteAccessPoint,
		opTagResource,
		opListTagsForResource,
		opDescribeLifecycleConfiguration,
		opPutLifecycleConfiguration,
		opCreateReplicationConfiguration,
		opCreateTags,
		opDeleteFileSystemPolicy,
		opDeleteReplicationConfiguration,
		opDeleteTags,
		opDescribeAccountPreferences,
		opDescribeBackupPolicy,
		opPutBackupPolicy,
		opDescribeFileSystemPolicy,
		opPutFileSystemPolicy,
		opDescribeMountTargetSecurityGroups,
		opDescribeReplicationConfigurations,
		opDescribeTags,
		opModifyMountTargetSecurityGroups,
		opPutAccountPreferences,
		opUntagResource,
		opUpdateFileSystemProtection,
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "efs" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this EFS instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches AWS EFS REST requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		return path == pathFileSystems ||
			strings.HasPrefix(path, pathFileSystems+"/") ||
			path == pathMountTargets ||
			strings.HasPrefix(path, pathMountTargets+"/") ||
			path == pathAccessPoints ||
			strings.HasPrefix(path, pathAccessPoints+"/") ||
			strings.HasPrefix(path, pathResourceTags+"/") ||
			strings.HasPrefix(path, pathTags+"/") ||
			strings.HasPrefix(path, pathCreateTags+"/") ||
			strings.HasPrefix(path, pathDeleteTags+"/") ||
			path == pathAccountPrefs
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return efsMatchPriority }

// efsRoute holds parsed information from an EFS REST request path.
type efsRoute struct {
	resource  string
	operation string
}

// parseEFSPath maps HTTP method + path to an operation name and resource ID.
func parseEFSPath(method, rawPath string) efsRoute {
	path, _ := url.PathUnescape(rawPath)

	switch {
	case strings.HasPrefix(path, pathFileSystems):
		return parseFileSystemRoute(method, strings.TrimPrefix(path, pathFileSystems))
	case strings.HasPrefix(path, pathMountTargets):
		return parseMountTargetRoute(method, strings.TrimPrefix(path, pathMountTargets))
	case strings.HasPrefix(path, pathAccessPoints):
		return parseAccessPointRoute(method, strings.TrimPrefix(path, pathAccessPoints))
	case strings.HasPrefix(path, pathResourceTags+"/"):
		return parseResourceTagsRoute(method, strings.TrimPrefix(path, pathResourceTags+"/"))
	case strings.HasPrefix(path, pathTags+"/"):
		return parseLegacyTagsRoute(method, strings.TrimPrefix(path, pathTags+"/"))
	case strings.HasPrefix(path, pathCreateTags+"/"):
		return parseCreateTagsRoute(method, strings.TrimPrefix(path, pathCreateTags+"/"))
	case strings.HasPrefix(path, pathDeleteTags+"/"):
		return parseDeleteTagsRoute(method, strings.TrimPrefix(path, pathDeleteTags+"/"))
	case path == pathAccountPrefs:
		return parseAccountPrefsRoute(method)
	}

	return efsRoute{operation: opUnknown}
}

func parseFileSystemRoute(method, suffix string) efsRoute {
	id := strings.TrimPrefix(suffix, "/")

	switch {
	case id == "":
		switch method {
		case http.MethodPost:
			return efsRoute{operation: opCreateFileSystem}
		case http.MethodGet:
			return efsRoute{operation: opDescribeFileSystems}
		}
	case id == "replication-configurations":
		if method == http.MethodGet {
			return efsRoute{operation: opDescribeReplicationConfigurations}
		}
	case !strings.Contains(id, "/"):
		// Treat the single segment as a file system ID.
		switch method {
		case http.MethodGet:
			return efsRoute{operation: opDescribeFileSystems, resource: id}
		case http.MethodDelete:
			return efsRoute{operation: opDeleteFileSystem, resource: id}
		case http.MethodPut:
			return efsRoute{operation: opUpdateFileSystem, resource: id}
		}
	default:
		return parseFileSystemSubRoute(method, id)
	}

	return efsRoute{operation: opUnknown}
}

func parseFileSystemSubRoute(method, id string) efsRoute {
	// Sub-resource paths: /{fileSystemId}/{subresource}
	parts := strings.SplitN(id, "/", subresourcePathParts)
	if len(parts) < subresourcePathParts {
		return efsRoute{operation: opUnknown}
	}

	fsID, sub := parts[0], parts[1]

	switch sub {
	case "lifecycle-configuration":
		return parseLifecycleConfigRoute(method, fsID)
	case "replication-configuration":
		return parseReplicationConfigRoute(method, fsID)
	case "policy":
		return parseFileSystemPolicyRoute(method, fsID)
	case "backup-policy":
		if method == http.MethodGet {
			return efsRoute{operation: opDescribeBackupPolicy, resource: fsID}
		}
		if method == http.MethodPut {
			return efsRoute{operation: opPutBackupPolicy, resource: fsID}
		}
	case "protection":
		if method == http.MethodPut {
			return efsRoute{operation: opUpdateFileSystemProtection, resource: fsID}
		}
	}

	return efsRoute{operation: opUnknown}
}

func parseLifecycleConfigRoute(method, fsID string) efsRoute {
	switch method {
	case http.MethodGet:
		return efsRoute{operation: opDescribeLifecycleConfiguration, resource: fsID}
	case http.MethodPut:
		return efsRoute{operation: opPutLifecycleConfiguration, resource: fsID}
	}

	return efsRoute{operation: opUnknown}
}

func parseReplicationConfigRoute(method, fsID string) efsRoute {
	switch method {
	case http.MethodPost:
		return efsRoute{operation: opCreateReplicationConfiguration, resource: fsID}
	case http.MethodDelete:
		return efsRoute{operation: opDeleteReplicationConfiguration, resource: fsID}
	}

	return efsRoute{operation: opUnknown}
}

func parseFileSystemPolicyRoute(method, fsID string) efsRoute {
	switch method {
	case http.MethodGet:
		return efsRoute{operation: opDescribeFileSystemPolicy, resource: fsID}
	case http.MethodPut:
		return efsRoute{operation: opPutFileSystemPolicy, resource: fsID}
	case http.MethodDelete:
		return efsRoute{operation: opDeleteFileSystemPolicy, resource: fsID}
	}

	return efsRoute{operation: opUnknown}
}

func parseMountTargetRoute(method, suffix string) efsRoute {
	id := strings.TrimPrefix(suffix, "/")

	switch {
	case id == "":
		switch method {
		case http.MethodPost:
			return efsRoute{operation: opCreateMountTarget}
		case http.MethodGet:
			return efsRoute{operation: opDescribeMountTargets}
		}
	case !strings.Contains(id, "/"):
		switch method {
		case http.MethodGet:
			return efsRoute{operation: opDescribeMountTargets, resource: id}
		case http.MethodDelete:
			return efsRoute{operation: opDeleteMountTarget, resource: id}
		}
	default:
		// Sub-resource paths: /{mountTargetId}/{subresource}
		parts := strings.SplitN(id, "/", subresourcePathParts)
		if len(parts) >= subresourcePathParts && parts[1] == "security-groups" {
			switch method {
			case http.MethodGet:
				return efsRoute{operation: opDescribeMountTargetSecurityGroups, resource: parts[0]}
			case http.MethodPut:
				return efsRoute{operation: opModifyMountTargetSecurityGroups, resource: parts[0]}
			}
		}
	}

	return efsRoute{operation: opUnknown}
}

func parseAccessPointRoute(method, suffix string) efsRoute {
	id := strings.TrimPrefix(suffix, "/")
	if id == "" {
		switch method {
		case http.MethodPost:
			return efsRoute{operation: opCreateAccessPoint}
		case http.MethodGet:
			return efsRoute{operation: opDescribeAccessPoints}
		}
	} else if !strings.Contains(id, "/") {
		switch method {
		case http.MethodGet:
			return efsRoute{operation: opDescribeAccessPoints, resource: id}
		case http.MethodDelete:
			return efsRoute{operation: opDeleteAccessPoint, resource: id}
		}
	}

	return efsRoute{operation: opUnknown}
}

// parseResourceTagsRoute maps requests under pathResourceTags
// ("/2015-02-01/resource-tags/{ResourceId}") to TagResource / ListTagsForResource /
// UntagResource, matching the real aws-sdk-go-v2 REST bindings for those three ops.
func parseResourceTagsRoute(method, resourceID string) efsRoute {
	switch method {
	case http.MethodPost:
		return efsRoute{operation: opTagResource, resource: resourceID}
	case http.MethodGet:
		return efsRoute{operation: opListTagsForResource, resource: resourceID}
	case http.MethodDelete:
		return efsRoute{operation: opUntagResource, resource: resourceID}
	}

	return efsRoute{operation: opUnknown}
}

// parseLegacyTagsRoute maps requests under pathTags ("/2015-02-01/tags/{FileSystemId}")
// to the deprecated DescribeTags op. It is GET-only on the real API; POST/DELETE at
// this path are not bound to any operation.
func parseLegacyTagsRoute(method, fileSystemID string) efsRoute {
	if method == http.MethodGet {
		return efsRoute{operation: opDescribeTags, resource: fileSystemID}
	}

	return efsRoute{operation: opUnknown}
}

func parseCreateTagsRoute(method, fileSystemID string) efsRoute {
	if method == http.MethodPost {
		return efsRoute{operation: opCreateTags, resource: fileSystemID}
	}

	return efsRoute{operation: opUnknown}
}

func parseDeleteTagsRoute(method, fileSystemID string) efsRoute {
	if method == http.MethodPost {
		return efsRoute{operation: opDeleteTags, resource: fileSystemID}
	}

	return efsRoute{operation: opUnknown}
}

func parseAccountPrefsRoute(method string) efsRoute {
	switch method {
	case http.MethodGet:
		return efsRoute{operation: opDescribeAccountPreferences}
	case http.MethodPut:
		return efsRoute{operation: opPutAccountPreferences}
	}

	return efsRoute{operation: opUnknown}
}

// ExtractOperation extracts the EFS operation name from the REST path.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	r := parseEFSPath(c.Request().Method, c.Request().URL.Path)

	return r.operation
}

// ExtractResource extracts the primary resource identifier from the URL path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	r := parseEFSPath(c.Request().Method, c.Request().URL.Path)

	return r.resource
}

// Handler returns the Echo handler function for EFS requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		log := logger.Load(c.Request().Context())
		route := parseEFSPath(c.Request().Method, c.Request().URL.Path)

		log.Debug("efs request", "operation", route.operation, "resource", route.resource)

		var body []byte
		if c.Request().Body != nil {
			decoder := json.NewDecoder(c.Request().Body)
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err == nil {
				body = raw
			}
		}

		return h.dispatch(c, route, body)
	}
}

// contextWithRegion returns the request context with the resolved AWS region attached
// under regionContextKey so that backend operations are routed to the correct region.
func (h *Handler) contextWithRegion(c *echo.Context) context.Context {
	region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())

	return context.WithValue(c.Request().Context(), regionContextKey{}, region)
}

func (h *Handler) dispatch(c *echo.Context, route efsRoute, body []byte) error {
	if ok, err := h.dispatchFileSystemOps(c, route, body); ok {
		return err
	}

	if ok, err := h.dispatchMountTargetAndAccessPointOps(c, route, body); ok {
		return err
	}

	if ok, err := h.dispatchTagAndMiscOps(c, route, body); ok {
		return err
	}

	return c.JSON(
		http.StatusNotFound,
		errResp("UnsupportedOperation", "unknown operation: "+route.operation),
	)
}

func (h *Handler) dispatchFileSystemOps(
	c *echo.Context,
	route efsRoute,
	body []byte,
) (bool, error) {
	switch route.operation {
	case opCreateFileSystem:
		return true, h.handleCreateFileSystem(c, body)
	case opDescribeFileSystems:
		return true, h.handleDescribeFileSystems(c, route.resource)
	case opDeleteFileSystem:
		return true, h.handleDeleteFileSystem(c, route.resource)
	case opUpdateFileSystem:
		return true, h.handleUpdateFileSystem(c, route.resource, body)
	case opDescribeLifecycleConfiguration:
		return true, h.handleDescribeLifecycleConfiguration(c, route.resource)
	case opPutLifecycleConfiguration:
		return true, h.handlePutLifecycleConfiguration(c, route.resource, body)
	case opCreateReplicationConfiguration:
		return true, h.handleCreateReplicationConfiguration(c, route.resource, body)
	case opDeleteReplicationConfiguration:
		return true, h.handleDeleteReplicationConfiguration(c, route.resource)
	case opDescribeReplicationConfigurations:
		return true, h.handleDescribeReplicationConfigurations(c)
	case opDescribeFileSystemPolicy:
		return true, h.handleDescribeFileSystemPolicy(c, route.resource)
	case opPutFileSystemPolicy:
		return true, h.handlePutFileSystemPolicy(c, route.resource, body)
	case opDeleteFileSystemPolicy:
		return true, h.handleDeleteFileSystemPolicy(c, route.resource)
	case opDescribeBackupPolicy:
		return true, h.handleDescribeBackupPolicy(c, route.resource)
	case opPutBackupPolicy:
		return true, h.handlePutBackupPolicy(c, route.resource, body)
	}

	return false, nil
}

func (h *Handler) dispatchMountTargetAndAccessPointOps(
	c *echo.Context,
	route efsRoute,
	body []byte,
) (bool, error) {
	switch route.operation {
	case opCreateMountTarget:
		return true, h.handleCreateMountTarget(c, body)
	case opDescribeMountTargets:
		return true, h.handleDescribeMountTargets(c, route.resource)
	case opDeleteMountTarget:
		return true, h.handleDeleteMountTarget(c, route.resource)
	case opDescribeMountTargetSecurityGroups:
		return true, h.handleDescribeMountTargetSecurityGroups(c, route.resource)
	case opModifyMountTargetSecurityGroups:
		return true, h.handleModifyMountTargetSecurityGroups(c, route.resource, body)
	case opCreateAccessPoint:
		return true, h.handleCreateAccessPoint(c, body)
	case opDescribeAccessPoints:
		return true, h.handleDescribeAccessPoints(c, route.resource)
	case opDeleteAccessPoint:
		return true, h.handleDeleteAccessPoint(c, route.resource)
	}

	return false, nil
}

func (h *Handler) dispatchTagAndMiscOps(
	c *echo.Context,
	route efsRoute,
	body []byte,
) (bool, error) {
	switch route.operation {
	case opTagResource:
		return true, h.handleTagResource(c, route.resource, body)
	case opListTagsForResource, opDescribeTags:
		return true, h.handleListTagsForResource(c, route.resource)
	case opUntagResource:
		return true, h.handleUntagResource(c, route.resource)
	case opCreateTags:
		return true, h.handleCreateTags(c, route.resource, body)
	case opDeleteTags:
		return true, h.handleDeleteTags(c, route.resource, body)
	case opDescribeAccountPreferences:
		return true, h.handleDescribeAccountPreferences(c)
	case opPutAccountPreferences:
		return true, h.handlePutAccountPreferences(c, body)
	case opUpdateFileSystemProtection:
		return true, h.handleUpdateFileSystemProtection(c, route.resource, body)
	}

	return false, nil
}

// errClassification maps an error sentinel to the EFS error code and HTTP
// status handleError reports for it.
type errClassification struct {
	err    error
	code   string
	status int
}

// Adding a 15th case to handleError's old switch tripped cyclop; a
// data-driven lookup keeps handleError itself at a single loop+return
// regardless of how many error codes EFS grows.
func efsErrClassifications() []errClassification {
	return []errClassification{
		{ErrValidation, "ValidationException", http.StatusBadRequest},
		{ErrBadRequest, "BadRequest", http.StatusBadRequest},
		{ErrInvalidPolicy, "InvalidPolicyException", http.StatusBadRequest},
		{ErrTooManyRequests, "TooManyRequests", http.StatusTooManyRequests},
		{ErrFileSystemInUse, "FileSystemInUse", http.StatusConflict},
		{ErrMountTargetConflict, "MountTargetConflict", http.StatusConflict},
		{ErrIncorrectFileSystemLifeCycleState, "IncorrectFileSystemLifeCycleState", http.StatusConflict},
		{ErrSecurityGroupLimitExceeded, "SecurityGroupLimitExceeded", http.StatusBadRequest},
		{ErrNotFound, "FileSystemNotFound", http.StatusNotFound},
		{ErrMountTargetNotFound, "MountTargetNotFound", http.StatusNotFound},
		{ErrAccessPointNotFound, "AccessPointNotFound", http.StatusNotFound},
		{ErrSubnetNotFound, "SubnetNotFound", http.StatusNotFound},
		{ErrAlreadyExists, "FileSystemAlreadyExists", http.StatusConflict},
		{ErrReplicationConfigExists, "ConflictException", http.StatusConflict},
		{ErrPolicyNotFound, "PolicyNotFound", http.StatusNotFound},
	}
}

func (h *Handler) handleError(c *echo.Context, err error) error {
	for _, ec := range efsErrClassifications() {
		if errors.Is(err, ec.err) {
			c.Response().Header().Set("x-amzn-ErrorType", ec.code)

			return c.JSON(ec.status, errResp(ec.code, err.Error()))
		}
	}

	c.Response().Header().Set("x-amzn-ErrorType", "InternalServerError")

	return c.JSON(http.StatusInternalServerError, errResp("InternalServerError", err.Error()))
}

func errResp(code, msg string) map[string]string {
	return map[string]string{"ErrorCode": code, "Message": msg}
}

type tagEntry struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

func tagsFromEntries(entries []tagEntry) map[string]string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.Key] = e.Value
	}

	return m
}

// tagsToEntries converts a tag map to sorted entries for deterministic output.
func tagsToEntries(m map[string]string) []tagEntry {
	entries := make([]tagEntry, 0, len(m))
	for k, v := range m {
		entries = append(entries, tagEntry{Key: k, Value: v})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })

	return entries
}

// describeListResponse is a generic helper that builds a paginated JSON response
// for Describe* list endpoints, eliminating duplication between mount targets
// and access points.
func describeListResponse[T any](
	c *echo.Context,
	h *Handler,
	listFn func(ctx context.Context, fsID, itemID, marker string, maxItems int) ([]*T, string, error),
	toResp func(*T) map[string]any,
	itemID, idQueryKey, markerKey, maxKey, respListKey, nextKey string,
) error {
	fsID := c.Request().URL.Query().Get(keyFileSystemID)
	if itemID == "" {
		itemID = c.Request().URL.Query().Get(idQueryKey)
	}

	marker := c.Request().URL.Query().Get(markerKey)
	maxItems := queryInt(c, maxKey)

	results, nextMarker, err := listFn(h.contextWithRegion(c), fsID, itemID, marker, maxItems)
	if err != nil {
		return h.handleError(c, err)
	}

	items := make([]map[string]any, 0, len(results))
	for _, item := range results {
		items = append(items, toResp(item))
	}

	resp := map[string]any{
		respListKey: items,
	}
	if nextMarker != "" {
		resp[nextKey] = nextMarker
	}

	return c.JSON(http.StatusOK, resp)
}

// queryInt reads a query parameter as an int, returning defaultMaxItems if absent or
// invalid. Every EFS list op pages at the same default size, so the default isn't a
// caller-supplied parameter (unparam flags a parameter that's always the same value
// across every call site as dead weight).
func queryInt(c *echo.Context, key string) int {
	s := c.Request().URL.Query().Get(key)
	if s == "" {
		return defaultMaxItems
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return defaultMaxItems
	}

	return v
}
