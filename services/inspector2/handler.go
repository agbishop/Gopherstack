package inspector2

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	inspector2ServiceName = "inspector2"
	matchPriority         = service.PriorityPathVersioned

	opEnable                = "Enable"
	opDisable               = "Disable"
	opBatchGetAccountStatus = "BatchGetAccountStatus"
	opCreateFilter          = "CreateFilter"
	opUpdateFilter          = "UpdateFilter"
	opDeleteFilter          = "DeleteFilter"
	opListFilters           = "ListFilters"
	opListFindings          = "ListFindings"
	opGetConfiguration      = "GetConfiguration"
	opUpdateConfiguration   = "UpdateConfiguration"
	opTagResource           = "TagResource"
	opUntagResource         = "UntagResource"
	opListTagsForResource   = "ListTagsForResource"
	opUnknown               = "Unknown"

	pathEnable              = "/enable"
	pathDisable             = "/disable"
	pathStatusBatchGet      = "/status/batch/get"
	pathFiltersCreate       = "/filters/create"
	pathFiltersUpdate       = "/filters/update"
	pathFiltersDelete       = "/filters/delete"
	pathFiltersList         = "/filters/list"
	pathFindingsList        = "/findings/list"
	pathConfigurationGet    = "/configuration/get"
	pathConfigurationUpdate = "/configuration/update"
	pathTagsPrefix          = "/tags/"

	keyAccounts           = "accounts"
	keyAccountID          = "accountId"
	keyAccountIDs         = "accountIds"
	keyResourceStatus     = "resourceStatus"
	keyResourceState      = "resourceState"
	keyStatus             = "status"
	keyFailedAccounts     = "failedAccounts"
	keyArn                = "arn"
	keyErrorCode          = "errorCode"
	keyErrorMessage       = "errorMessage"
	keyName               = "name"
	keyUpdatedAt          = "updatedAt"
	keyType               = "type"
	keyFindingArn         = "findingArn"
	keyCreatedAt          = "createdAt"
	keyScanConfigurations = "scanConfigurations"
	keyLevel              = "level"
	keyResourceID         = "resourceId"
	keySeverityCounts     = "severityCounts"
	keyAggregationType    = "aggregationType"
	keyResponses          = "responses"
)

// Handler handles Inspector2 HTTP requests.
type Handler struct {
	Backend StorageBackend
}

// NewHandler constructs a new Handler.
func NewHandler(b StorageBackend) *Handler {
	return &Handler{Backend: b}
}

// Name returns the service name.
func (h *Handler) Name() string { return "Inspector2" }

// Reset resets the backend.
func (h *Handler) Reset() { h.Backend.Reset() }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	base := []string{ //nolint:prealloc // existing issue.
		opEnable,
		opDisable,
		opBatchGetAccountStatus,
		opCreateFilter,
		opUpdateFilter,
		opDeleteFilter,
		opListFilters,
		opListFindings,
		opGetConfiguration,
		opUpdateConfiguration,
		opTagResource,
		opUntagResource,
		opListTagsForResource,
	}

	return append(base, extendedOps()...)
}

// onceRouteMatchPrefixes lazily builds the list of URL path prefixes
// RouteMatcher accepts, exactly once. Kept as a data table (rather than a
// long ||-chain) so RouteMatcher itself stays a simple loop instead of
// tripping cyclomatic-complexity lint thresholds.
//
// pathEnable/pathDisable are deliberately NOT in this list: real Inspector2
// only has the exact fixed paths POST /enable and POST /disable (confirmed
// by the {method, path} dispatch table below, which has no children under
// either). As a prefix, "/enable"/"/disable" wrongly swallow any other
// service's operation-named path starting with those letters (e.g. Account
// Management's POST /enableRegion, /disableRegion) -- see RouteMatcher's
// exact-match check for these two.
//
//nolint:gochecknoglobals // read-only package-level lookup table, built once via sync.OnceValue
var onceRouteMatchPrefixes = sync.OnceValue(func() []string {
	return []string{
		"/status/",
		"/filters/",
		"/findings/",
		"/configuration/",
		pathTagsPrefix + "arn:aws:inspector2:",
		"/members/",
		"/delegatedadminaccounts/",
		"/organizationconfiguration/",
		"/ec2deepinspection",
		"/encryptionkey/",
		"/cis/",
		"/cissession/",
		"/codesecurity/",
		"/reporting/",
		"/sbomexport/",
		"/coverage/",
		"/findings/aggregation/",
		"/usage/",
		"/accountpermissions/",
		"/vulnerabilities/",
		"/codesnippet/",
		"/freetrialinfo/",
		"/cluster/",
		"/connector/",
		"/connectorscanconfiguration",
	}
})

// ambiguousRouteMatchPrefixes are onceRouteMatchPrefixes entries that also
// prefix-match another registered service's real paths -- SecurityHub's
// BatchImportFindings is POST /findings/import (starts with "/findings/")
// and CreateMembers-family ops live under /members/{action}; Omics'
// GetConfiguration/DeleteConfiguration live under /configuration/{name}
// (confirmed against aws-sdk-go-v2/service/omics's serializers.go SplitURI
// calls) -- all of which this handler's plain prefix check would otherwise
// swallow before the other service's (tied-priority, later-registered)
// matcher ever runs (gopherstack-op3e). Gated by isInspector2Request instead
// of narrowing the prefix, since real Inspector2 also uses these exact
// prefixes (e.g. /findings/list, /configuration/get).
var ambiguousRouteMatchPrefixes = map[string]bool{ //nolint:gochecknoglobals // read-only lookup data
	"/findings/":      true,
	"/members/":       true,
	"/configuration/": true,
}

// RouteMatcher returns a matcher that accepts Inspector2 REST paths.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		if path == pathEnable || path == pathDisable {
			return true
		}

		for _, prefix := range onceRouteMatchPrefixes() {
			if !strings.HasPrefix(path, prefix) {
				continue
			}

			if ambiguousRouteMatchPrefixes[prefix] && !isInspector2Request(c) {
				continue
			}

			return true
		}

		return false
	}
}

// isInspector2Request checks the Authorization header for the inspector2 signing service.
func isInspector2Request(c *echo.Context) bool {
	auth := c.Request().Header.Get("Authorization")

	return strings.Contains(auth, "/"+inspector2ServiceName+"/")
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return matchPriority }

// ExtractOperation extracts the operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	method := c.Request().Method
	path := c.Request().URL.Path

	if op := classifyPath(method, path); op != opUnknown {
		return op
	}

	if op := classifyExtendedPath(method, path); op != opUnknown {
		return op
	}

	return opUnknown
}

// ExtractResource extracts the resource identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	path := c.Request().URL.Path

	if resource, ok := strings.CutPrefix(path, pathTagsPrefix); ok {
		return resource
	}

	return path
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return h.handleREST(c)
	}
}

func (h *Handler) handleREST(c *echo.Context) error {
	path := c.Request().URL.Path
	method := c.Request().Method

	switch classifyPath(method, path) {
	case opEnable:
		return h.handleToggle(c, true)
	case opDisable:
		return h.handleToggle(c, false)
	case opBatchGetAccountStatus:
		return h.handleBatchGetAccountStatus(c)
	case opCreateFilter:
		return h.handleCreateFilter(c)
	case opUpdateFilter:
		return h.handleUpdateFilter(c)
	case opDeleteFilter:
		return h.handleDeleteFilter(c)
	case opListFilters:
		return h.handleListFilters(c)
	case opListFindings:
		return h.handleListFindings(c)
	case opGetConfiguration:
		return h.handleGetConfiguration(c)
	case opUpdateConfiguration:
		return h.handleUpdateConfiguration(c)
	case opListTagsForResource:
		return h.handleListTagsForResource(c)
	case opTagResource:
		return h.handleTagResource(c)
	case opUntagResource:
		return h.handleUntagResource(c)
	}

	if handled, err := h.handleExtendedOps(c); handled {
		return err
	}

	log := logger.Load(c.Request().Context())
	log.Debug("inspector2: unhandled request", "method", method, "path", path)

	return c.JSON(http.StatusNotImplemented, map[string]string{
		"__type":  "NotImplementedException",
		"message": "Operation not implemented: " + method + " " + path,
	})
}

// onceCoreRoutes lazily builds the method+path -> operation lookup table for
// classifyPath's exact-match routes (everything except the /tags/ prefix
// routes, which vary by method rather than by exact path -- see
// classifyTagsPath), exactly once.
//
//nolint:gochecknoglobals // read-only package-level lookup table, built once via sync.OnceValue
var onceCoreRoutes = sync.OnceValue(func() map[routeKey]string {
	return map[routeKey]string{
		{http.MethodPost, pathEnable}:              opEnable,
		{http.MethodPost, pathDisable}:             opDisable,
		{http.MethodPost, pathStatusBatchGet}:      opBatchGetAccountStatus,
		{http.MethodPost, pathFiltersCreate}:       opCreateFilter,
		{http.MethodPost, pathFiltersUpdate}:       opUpdateFilter,
		{http.MethodPost, pathFiltersDelete}:       opDeleteFilter,
		{http.MethodPost, pathFiltersList}:         opListFilters,
		{http.MethodPost, pathFindingsList}:        opListFindings,
		{http.MethodPost, pathConfigurationGet}:    opGetConfiguration,
		{http.MethodPost, pathConfigurationUpdate}: opUpdateConfiguration,
	}
})

// classifyPath maps method+path to its core operation name (the routes
// handled directly by handleREST's switch), falling back to
// classifyTagsPath for the /tags/ routes, or opUnknown if nothing matches.
func classifyPath(method, path string) string {
	if op, ok := onceCoreRoutes()[routeKey{method: method, path: path}]; ok {
		return op
	}

	return classifyTagsPath(method, path)
}

// classifyTagsPath handles the /tags/<resource-arn> routes, whose operation
// is determined by HTTP method rather than by an exact path match.
func classifyTagsPath(method, path string) string {
	if !strings.HasPrefix(path, pathTagsPrefix) {
		return opUnknown
	}

	switch method {
	case http.MethodGet:
		return opListTagsForResource
	case http.MethodPost:
		return opTagResource
	case http.MethodDelete:
		return opUntagResource
	default:
		return opUnknown
	}
}

// filterListRequest is the shared shape of the filterCriteria/maxResults/
// nextToken list requests used by ListFindings and ListCoverage. SortCriteria
// is only meaningful for ListFindings -- ListCoverageInput has no such member
// (api_op_ListCoverage.go, inspector2@v1.54.1), so it is simply absent from
// that request body and ignored here.
type filterListRequest struct {
	FilterCriteria map[string]any    `json:"filterCriteria"`
	SortCriteria   *findingSortInput `json:"sortCriteria,omitempty"`
	NextToken      string            `json:"nextToken"`
	MaxResults     int32             `json:"maxResults"`
}

// findingSortInput is ListFindingsInput.SortCriteria's wire shape
// (api_op_ListFindings.go, inspector2@v1.54.1: field/sortOrder).
type findingSortInput struct {
	Field     string `json:"field"`
	SortOrder string `json:"sortOrder"`
}

// decodeFilterListRequest reads and decodes a filterListRequest. On a malformed
// body it returns ok=false after writing the appropriate error response.
func decodeFilterListRequest(c *echo.Context) (filterListRequest, bool) {
	var req filterListRequest

	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		_ = c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid body"))

		return req, false
	}

	if len(body) > 0 {
		if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
			_ = c.JSON(http.StatusBadRequest, errorResponse("ValidationException", "invalid JSON"))

			return req, false
		}
	}

	return req, true
}

// extractResourceARN extracts the resource ARN from the URL path.
func extractResourceARN(path string) string {
	resource, _ := strings.CutPrefix(path, pathTagsPrefix)

	if resource == path {
		return ""
	}

	return resource
}

// errorResponse builds a standard Inspector2 error JSON body.
func errorResponse(code, message string) map[string]string {
	return map[string]string{
		"__type":  code,
		"message": message,
	}
}

// mapError translates backend errors to HTTP responses.
func (h *Handler) mapError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, awserr.ErrNotFound):
		return c.JSON(http.StatusNotFound, errorResponse(errResourceNotFound, err.Error()))
	case errors.Is(err, awserr.ErrConflict):
		return c.JSON(http.StatusConflict, errorResponse(errConflict, err.Error()))
	case errors.Is(err, awserr.ErrInvalidParameter):
		return c.JSON(http.StatusBadRequest, errorResponse(errValidation, err.Error()))
	default:
		log := logger.Load(c.Request().Context())
		log.Error("inspector2: unexpected error", "err", err)

		return c.JSON(
			http.StatusInternalServerError,
			errorResponse("InternalServerException", "internal error"),
		)
	}
}
