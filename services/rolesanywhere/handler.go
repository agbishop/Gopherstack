package rolesanywhere

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	rolesAnywhereService = "rolesanywhere"
	matchPriority        = service.PriorityPathVersioned

	pathTrustanchors       = "trustanchors"
	pathTrustanchor        = "trustanchor"
	pathProfiles           = "profiles"
	pathProfile            = "profile"
	pathCrls               = "crls"
	pathCrl                = "crl"
	pathSubjects           = "subjects"
	pathSubject            = "subject"
	pathMappings           = "mappings"
	pathPutNotifications   = "put-notifications-settings"
	pathResetNotifications = "reset-notifications-settings"
	pathEnable             = "enable"
	pathDisable            = "disable"
	pathTagResource        = "TagResource"
	pathUntagResource      = "UntagResource"
	pathListTags           = "ListTagsForResource"

	opCreateTrustAnchor         = "CreateTrustAnchor"
	opGetTrustAnchor            = "GetTrustAnchor"
	opListTrustAnchors          = "ListTrustAnchors"
	opDeleteTrustAnchor         = "DeleteTrustAnchor"
	opUpdateTrustAnchor         = "UpdateTrustAnchor"
	opEnableTrustAnchor         = "EnableTrustAnchor"
	opDisableTrustAnchor        = "DisableTrustAnchor"
	opCreateProfile             = "CreateProfile"
	opGetProfile                = "GetProfile"
	opListProfiles              = "ListProfiles"
	opDeleteProfile             = "DeleteProfile"
	opUpdateProfile             = "UpdateProfile"
	opEnableProfile             = "EnableProfile"
	opDisableProfile            = "DisableProfile"
	opImportCrl                 = "ImportCrl"
	opGetCrl                    = "GetCrl"
	opListCrls                  = "ListCrls"
	opUpdateCrl                 = "UpdateCrl"
	opDeleteCrl                 = "DeleteCrl"
	opEnableCrl                 = "EnableCrl"
	opDisableCrl                = "DisableCrl"
	opGetSubject                = "GetSubject"
	opListSubjects              = "ListSubjects"
	opPutAttributeMapping       = "PutAttributeMapping"
	opDeleteAttributeMapping    = "DeleteAttributeMapping"
	opPutNotificationSettings   = "PutNotificationSettings"
	opResetNotificationSettings = "ResetNotificationSettings"
	opTagResource               = "TagResource"
	opUntagResource             = "UntagResource"
	opListTagsForResource       = "ListTagsForResource"
	opUnknown                   = "Unknown"

	keyTrustAnchor  = "trustAnchor"
	keyTrustAnchors = "trustAnchors"
	keyProfile      = "profile"
	keyProfiles     = "profiles"
	keyCrl          = "crl"
	keyCrls         = "crls"
	keySubject      = "subject"
	keySubjects     = "subjects"
	keyTags         = "tags"

	// URL path segment depth constants.
	segmentDepthResource    = 2 // /prefix/{id}
	segmentDepthSubResource = 3 // /prefix/{id}/action
	segmentDepthMapping     = 3 // /profiles/{id}/mappings

	// minSegmentsForResource is the minimum number of path segments for a resource op.
	minSegmentsForResource = 2
)

// Handler handles Roles Anywhere HTTP requests.
type Handler struct {
	Backend StorageBackend
}

// NewHandler constructs a new Handler.
func NewHandler(b StorageBackend) *Handler {
	return &Handler{Backend: b}
}

// Name returns the service name.
func (h *Handler) Name() string { return "RolesAnywhere" }

// Reset resets the backend.
func (h *Handler) Reset() { h.Backend.Reset() }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateTrustAnchor,
		opGetTrustAnchor,
		opListTrustAnchors,
		opDeleteTrustAnchor,
		opUpdateTrustAnchor,
		opEnableTrustAnchor,
		opDisableTrustAnchor,
		opCreateProfile,
		opGetProfile,
		opListProfiles,
		opDeleteProfile,
		opUpdateProfile,
		opEnableProfile,
		opDisableProfile,
		opImportCrl,
		opGetCrl,
		opListCrls,
		opUpdateCrl,
		opDeleteCrl,
		opEnableCrl,
		opDisableCrl,
		opGetSubject,
		opListSubjects,
		opPutAttributeMapping,
		opDeleteAttributeMapping,
		opPutNotificationSettings,
		opResetNotificationSettings,
		opTagResource,
		opUntagResource,
		opListTagsForResource,
	}
}

// RouteMatcher returns a function that matches Roles Anywhere requests by path.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		return strings.HasPrefix(path, "/"+pathTrustanchors) ||
			strings.HasPrefix(path, "/"+pathTrustanchor+"/") ||
			strings.HasPrefix(path, "/"+pathProfiles) ||
			strings.HasPrefix(path, "/"+pathProfile+"/") ||
			strings.HasPrefix(path, "/"+pathCrls) ||
			strings.HasPrefix(path, "/"+pathCrl+"/") ||
			strings.HasPrefix(path, "/"+pathSubjects) ||
			strings.HasPrefix(path, "/"+pathSubject+"/") ||
			path == "/"+pathPutNotifications ||
			path == "/"+pathResetNotifications ||
			path == "/"+pathTagResource ||
			path == "/"+pathUntagResource ||
			path == "/"+pathListTags
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return matchPriority }

// ExtractOperation extracts the operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	op, _ := parseRESTPath(c.Request().Method, c.Request().URL.Path)

	return op
}

// ExtractResource extracts the resource identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	_, resource := parseRESTPath(c.Request().Method, c.Request().URL.Path)

	return resource
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return h.handleREST(c)
	}
}

// regionFromRequest resolves the AWS region for a request from its SigV4
// credential scope, falling back to the backend's default region.
func (h *Handler) regionFromRequest(c *echo.Context) string {
	return httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())
}

func (h *Handler) handleREST(c *echo.Context) error {
	ctx := context.WithValue(c.Request().Context(), regionContextKey{}, h.regionFromRequest(c))
	log := logger.Load(ctx)

	op, _ := parseRESTPath(c.Request().Method, c.Request().URL.Path)

	if op == opUnknown {
		return c.JSON(http.StatusNotFound, errBody("ResourceNotFoundException", "not found"))
	}

	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(http.StatusBadRequest, errBody("ValidationException", "failed to read body"))
	}

	result, statusCode, opErr := h.dispatch(ctx, op, c.Request().URL.Path, c.Request().URL.RawQuery, body)
	if opErr != nil {
		log.Error("rolesanywhere operation error", "op", op, "err", opErr)

		return h.handleError(c, opErr)
	}

	if result == nil {
		return c.JSON(statusCode, struct{}{})
	}

	data, jsonErr := json.Marshal(result)
	if jsonErr != nil {
		return c.JSON(http.StatusInternalServerError, errBody("InternalFailure", "serialization failed"))
	}

	c.Response().Header().Set("Content-Type", "application/json")

	return c.JSONBlob(statusCode, data)
}

func (h *Handler) dispatch(
	ctx context.Context,
	op, path, query string,
	body []byte,
) (any, int, error) {
	// Trust anchor and profile share identical CRUD op sets; a map-based dispatch
	// keeps cyclomatic complexity low while avoiding structurally-identical functions.
	handlers := map[string]func() (any, int, error){
		opCreateTrustAnchor:  func() (any, int, error) { return h.handleCreateTrustAnchor(ctx, body) },
		opGetTrustAnchor:     func() (any, int, error) { return h.handleGetTrustAnchor(ctx, path) },
		opListTrustAnchors:   func() (any, int, error) { return h.handleListTrustAnchors(ctx, query) },
		opDeleteTrustAnchor:  func() (any, int, error) { return h.handleDeleteTrustAnchor(ctx, path) },
		opUpdateTrustAnchor:  func() (any, int, error) { return h.handleUpdateTrustAnchor(ctx, path, body) },
		opEnableTrustAnchor:  func() (any, int, error) { return h.handleEnableTrustAnchor(ctx, path) },
		opDisableTrustAnchor: func() (any, int, error) { return h.handleDisableTrustAnchor(ctx, path) },
		opCreateProfile:      func() (any, int, error) { return h.handleCreateProfile(ctx, body) },
		opGetProfile:         func() (any, int, error) { return h.handleGetProfile(ctx, path) },
		opListProfiles:       func() (any, int, error) { return h.handleListProfiles(ctx, query) },
		opDeleteProfile:      func() (any, int, error) { return h.handleDeleteProfile(ctx, path) },
		opUpdateProfile:      func() (any, int, error) { return h.handleUpdateProfile(ctx, path, body) },
		opEnableProfile:      func() (any, int, error) { return h.handleEnableProfile(ctx, path) },
		opDisableProfile:     func() (any, int, error) { return h.handleDisableProfile(ctx, path) },
	}

	if fn, ok := handlers[op]; ok {
		return fn()
	}

	if result, code, ok, err := h.dispatchCrlOps(ctx, op, path, query, body); ok {
		return result, code, err
	}

	if result, code, ok, err := h.dispatchSubjectOps(ctx, op, path, query); ok {
		return result, code, err
	}

	if result, code, ok, err := h.dispatchMappingOps(ctx, op, path, query, body); ok {
		return result, code, err
	}

	if result, code, ok, err := h.dispatchNotificationOps(ctx, op, body); ok {
		return result, code, err
	}

	return h.dispatchTagOps(ctx, op, query, body)
}

func (h *Handler) dispatchTagOps(ctx context.Context, op, query string, body []byte) (any, int, error) {
	switch op {
	case opTagResource:
		return h.handleTagResource(ctx, body)
	case opUntagResource:
		return h.handleUntagResource(ctx, body)
	case opListTagsForResource:
		return h.handleListTagsForResource(ctx, query)
	}

	return nil, http.StatusNotFound, nil
}

func (h *Handler) dispatchCrlOps(ctx context.Context, op, path, query string, body []byte) (any, int, bool, error) {
	switch op {
	case opImportCrl:
		r, c, e := h.handleImportCrl(ctx, body)

		return r, c, true, e
	case opGetCrl:
		r, c, e := h.handleGetCrl(ctx, path)

		return r, c, true, e
	case opListCrls:
		r, c, e := h.handleListCrls(ctx, query)

		return r, c, true, e
	case opUpdateCrl:
		r, c, e := h.handleUpdateCrl(ctx, path, body)

		return r, c, true, e
	case opDeleteCrl:
		r, c, e := h.handleDeleteCrl(ctx, path)

		return r, c, true, e
	case opEnableCrl:
		r, c, e := h.handleEnableCrl(ctx, path)

		return r, c, true, e
	case opDisableCrl:
		r, c, e := h.handleDisableCrl(ctx, path)

		return r, c, true, e
	}

	return nil, 0, false, nil
}

func (h *Handler) dispatchSubjectOps(ctx context.Context, op, path, query string) (any, int, bool, error) {
	switch op {
	case opGetSubject:
		r, c, e := h.handleGetSubject(ctx, path)

		return r, c, true, e
	case opListSubjects:
		r, c, e := h.handleListSubjects(ctx, query)

		return r, c, true, e
	}

	return nil, 0, false, nil
}

func (h *Handler) dispatchMappingOps(ctx context.Context, op, path, query string, body []byte) (any, int, bool, error) {
	switch op {
	case opPutAttributeMapping:
		r, c, e := h.handlePutAttributeMapping(ctx, path, body)

		return r, c, true, e
	case opDeleteAttributeMapping:
		r, c, e := h.handleDeleteAttributeMapping(ctx, path, query)

		return r, c, true, e
	}

	return nil, 0, false, nil
}

func (h *Handler) dispatchNotificationOps(ctx context.Context, op string, body []byte) (any, int, bool, error) {
	switch op {
	case opPutNotificationSettings:
		r, c, e := h.handlePutNotificationSettings(ctx, body)

		return r, c, true, e
	case opResetNotificationSettings:
		r, c, e := h.handleResetNotificationSettings(ctx, body)

		return r, c, true, e
	}

	return nil, 0, false, nil
}

// handleError writes an error response.
func (h *Handler) handleError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrTrustAnchorNotFound),
		errors.Is(err, ErrProfileNotFound),
		errors.Is(err, ErrCrlNotFound),
		errors.Is(err, ErrSubjectNotFound),
		errors.Is(err, ErrResourceNotFound):
		return c.JSON(http.StatusNotFound, errBody("ResourceNotFoundException", err.Error()))
	case errors.Is(err, ErrTooManyTags):
		return c.JSON(http.StatusBadRequest, errBody("TooManyTagsException", err.Error()))
	case errors.Is(err, ErrValidation):
		return c.JSON(http.StatusBadRequest, errBody("ValidationException", err.Error()))
	}

	return c.JSON(http.StatusInternalServerError, errBody("InternalFailure", err.Error()))
}

// ---- routing ----

func parseRESTPath(method, path string) (string, string) {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(segments) == 0 {
		return opUnknown, ""
	}

	if op, id := parseEntityPath(method, segments); op != opUnknown {
		return op, id
	}

	return parseTagPaths(method, segments)
}

// parseEntityPath handles /trustanchors, /trustanchor/*, /profiles, /profile/*, /crls, /crl/*, etc.
func parseEntityPath(method string, segments []string) (string, string) {
	switch segments[0] {
	case pathTrustanchors:
		return parseTrustAnchorsCollection(method)
	case pathTrustanchor:
		return parseTrustAnchorPath(method, segments)
	case pathProfiles:
		if len(segments) >= segmentDepthMapping {
			return parseProfilesMappingPath(method, segments)
		}

		return parseProfilesCollection(method)
	case pathProfile:
		return parseProfilePath(method, segments)
	case pathCrls:
		return parseCrlsCollection(method)
	case pathCrl:
		return parseCrlPath(method, segments)
	case pathSubjects:
		return parseSubjectsCollection(method)
	case pathSubject:
		return parseSubjectPath(method, segments)
	}

	return opUnknown, ""
}

func parseTrustAnchorsCollection(method string) (string, string) {
	switch method {
	case http.MethodGet:
		return opListTrustAnchors, ""
	case http.MethodPost:
		return opCreateTrustAnchor, ""
	}

	return opUnknown, ""
}

func parseProfilesCollection(method string) (string, string) {
	switch method {
	case http.MethodGet:
		return opListProfiles, ""
	case http.MethodPost:
		return opCreateProfile, ""
	}

	return opUnknown, ""
}

func parseProfilesMappingPath(method string, segments []string) (string, string) {
	// /profiles/{id}/mappings
	if len(segments) != segmentDepthMapping || segments[2] != pathMappings {
		return opUnknown, ""
	}

	id := segments[1]

	switch method {
	case http.MethodPut:
		return opPutAttributeMapping, id
	case http.MethodDelete:
		return opDeleteAttributeMapping, id
	}

	return opUnknown, ""
}

// parseTagPaths handles /TagResource, /UntagResource, /ListTagsForResource, and notification routing.
func parseTagPaths(method string, segments []string) (string, string) {
	switch segments[0] {
	case pathTagResource:
		if method == http.MethodPost {
			return opTagResource, ""
		}
	case pathUntagResource:
		if method == http.MethodPost {
			return opUntagResource, ""
		}
	case pathListTags:
		if method == http.MethodGet {
			return opListTagsForResource, ""
		}
	case pathPutNotifications:
		if method == http.MethodPatch {
			return opPutNotificationSettings, ""
		}
	case pathResetNotifications:
		if method == http.MethodPatch {
			return opResetNotificationSettings, ""
		}
	}

	return opUnknown, ""
}

func parseTrustAnchorPath(method string, segments []string) (string, string) {
	if len(segments) < minSegmentsForResource {
		return opUnknown, ""
	}

	id := segments[1]

	switch len(segments) {
	case segmentDepthResource:
		switch method {
		case http.MethodGet:
			return opGetTrustAnchor, id
		case http.MethodDelete:
			return opDeleteTrustAnchor, id
		case http.MethodPatch:
			return opUpdateTrustAnchor, id
		}
	case segmentDepthSubResource:
		switch segments[2] {
		case pathEnable:
			if method == http.MethodPost {
				return opEnableTrustAnchor, id
			}
		case pathDisable:
			if method == http.MethodPost {
				return opDisableTrustAnchor, id
			}
		}
	}

	return opUnknown, ""
}

func parseProfilePath(method string, segments []string) (string, string) {
	if len(segments) < minSegmentsForResource {
		return opUnknown, ""
	}

	id := segments[1]

	switch len(segments) {
	case segmentDepthResource:
		switch method {
		case http.MethodGet:
			return opGetProfile, id
		case http.MethodDelete:
			return opDeleteProfile, id
		case http.MethodPatch:
			return opUpdateProfile, id
		}
	case segmentDepthSubResource:
		switch segments[2] {
		case pathEnable:
			if method == http.MethodPost {
				return opEnableProfile, id
			}
		case pathDisable:
			if method == http.MethodPost {
				return opDisableProfile, id
			}
		}
	}

	return opUnknown, ""
}

func parseCrlsCollection(method string) (string, string) {
	switch method {
	case http.MethodGet:
		return opListCrls, ""
	case http.MethodPost:
		return opImportCrl, ""
	}

	return opUnknown, ""
}

func parseCrlPath(method string, segments []string) (string, string) {
	if len(segments) < minSegmentsForResource {
		return opUnknown, ""
	}

	id := segments[1]

	switch len(segments) {
	case segmentDepthResource:
		switch method {
		case http.MethodGet:
			return opGetCrl, id
		case http.MethodDelete:
			return opDeleteCrl, id
		case http.MethodPatch:
			return opUpdateCrl, id
		}
	case segmentDepthSubResource:
		switch segments[2] {
		case pathEnable:
			if method == http.MethodPost {
				return opEnableCrl, id
			}
		case pathDisable:
			if method == http.MethodPost {
				return opDisableCrl, id
			}
		}
	}

	return opUnknown, ""
}

func parseSubjectsCollection(method string) (string, string) {
	if method == http.MethodGet {
		return opListSubjects, ""
	}

	return opUnknown, ""
}

func parseSubjectPath(method string, segments []string) (string, string) {
	if len(segments) < minSegmentsForResource {
		return opUnknown, ""
	}

	id := segments[1]

	if len(segments) == segmentDepthResource && method == http.MethodGet {
		return opGetSubject, id
	}

	return opUnknown, ""
}

// extractID extracts the ID segment from a path like /trustanchor/{id} or /trustanchor/{id}/enable.
func extractID(path, prefix string) string {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	for i, s := range segments {
		if s == prefix && i+1 < len(segments) {
			return segments[i+1]
		}
	}

	return ""
}

// parsePageParams extracts nextToken and pageSize from a query string.
// pageSize, not maxResults, is the real wire query param every
// ListProfiles/ListTrustAnchors/ListCrls/ListSubjects request carries (see
// aws-sdk-go-v2/service/rolesanywhere/serializers.go's
// awsRestjson1_serializeOpHttpBindingsList*Input, which all call
// encoder.SetQuery("pageSize")); a real SDK client's page size was
// previously silently ignored here.
func parsePageParams(query string) (string, int, error) {
	var nextToken string

	var pageSize int

	for part := range strings.SplitSeq(query, "&") {
		if after, ok := strings.CutPrefix(part, "nextToken="); ok {
			nextToken = after
		}

		if after, ok := strings.CutPrefix(part, "pageSize="); ok {
			if after == "" {
				continue
			}

			// AWS rejects a non-numeric pageSize with ValidationException
			// rather than silently coercing it to zero / dropping non-digits.
			n, err := strconv.Atoi(after)
			if err != nil || n < 0 {
				return "", 0, ErrValidation
			}

			pageSize = n
		}
	}

	return nextToken, pageSize, nil
}

func errBody(code, message string) map[string]string {
	return map[string]string{
		"__type":  code,
		"message": message,
	}
}
