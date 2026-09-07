package accessanalyzer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	accessAnalyzerService = "access-analyzer"
	matchPriority         = service.PriorityPathVersioned
	pathAnalyzer          = "analyzer"
	pathFinding           = "finding"
	pathTags              = "tags"
	pathResource          = "resource"
	pathScan              = "scan"
	pathEnable            = "enable"
	pathDisable           = "disable"

	opUnknown = "Unknown"

	segmentDepthResource     = 2
	segmentDepthSubResource  = 3
	segmentDepthLeafResource = 4

	// keyCreatedAt/keyUpdatedAt/keyAnalyzedAt/keyTags/keyStatus/
	// keyAnalyzerArn/keyResourceType are JSON wire keys shared by more than
	// one operation family's response builder (analyzer, archive-rule,
	// finding, analyzed-resource, access-preview, and generated-policy
	// serializers all set one or more of these), so they live here rather
	// than in any one handler_<family>.go.
	keyCreatedAt    = "createdAt"
	keyUpdatedAt    = "updatedAt"
	keyAnalyzedAt   = "analyzedAt"
	keyTags         = "tags"
	keyStatus       = "status"
	keyAnalyzerArn  = "analyzerArn"
	keyResourceType = "resourceType"
	keyResourceArn  = "resourceArn"
)

// Handler handles Access Analyzer HTTP requests.
type Handler struct {
	Backend StorageBackend
}

// NewHandler constructs a new Handler.
func NewHandler(b StorageBackend) *Handler {
	return &Handler{Backend: b}
}

// Name returns the service name.
func (h *Handler) Name() string { return "AccessAnalyzer" }

// Reset resets the backend.
func (h *Handler) Reset() { h.Backend.Reset() }

// GetSupportedOperations returns every operation this handler routes,
// across all operation families (analyzers, archive rules, findings,
// analyzed resources, generated policies, access previews, policy
// validation, tags).
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opCreateAnalyzer,
		opGetAnalyzer,
		opListAnalyzers,
		opDeleteAnalyzer,
		opUpdateAnalyzer,
		opCreateServiceLinkedAnalyzer,
		opDeleteServiceLinkedAnalyzer,
		opCreateArchiveRule,
		opGetArchiveRule,
		opListArchiveRules,
		opDeleteArchiveRule,
		opUpdateArchiveRule,
		opApplyArchiveRule,
		opGetFinding,
		opListFindings,
		opUpdateFindings,
		opGetFindingV2,
		opListFindingsV2,
		opGetFindingsStatistics,
		opGenerateFindingRecommendation,
		opGetFindingRecommendation,
		opGetAnalyzedResource,
		opListAnalyzedResources,
		opStartResourceScan,
		opStartPolicyGeneration,
		opGetGeneratedPolicy,
		opCancelPolicyGeneration,
		opListPolicyGenerations,
		opCreateAccessPreview,
		opGetAccessPreview,
		opListAccessPreviews,
		opListAccessPreviewFindings,
		opCheckAccessNotGranted,
		opCheckNoNewAccess,
		opCheckNoPublicAccess,
		opValidatePolicy,
		opTagResource,
		opUntagResource,
		opListTagsForResource,
	}
}

// RouteMatcher returns a function that matches Access Analyzer requests by path prefix.
// For /tags/{ARN} paths, only matches when the ARN belongs to Access Analyzer
// (i.e. contains ":access-analyzer:") to avoid intercepting tag requests for other services.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		path := c.Request().URL.Path

		if strings.HasPrefix(path, "/"+pathAnalyzer) {
			return true
		}

		// GetFinding/ListFindings/UpdateFindings live at /finding and
		// /finding/{id} (NOT nested under /analyzer/{name}/...); match on a
		// segment boundary so this does not also swallow /findingv2, which is
		// handled by its own prefix entry below.
		if path == "/"+pathFinding || strings.HasPrefix(path, "/"+pathFinding+"/") {
			return true
		}

		if after, ok := strings.CutPrefix(path, "/"+pathTags+"/"); ok {
			return strings.Contains(after, ":"+accessAnalyzerService+":")
		}

		for _, prefix := range []string{
			"/" + pathResource + "/" + pathScan,
			"/" + pathArchiveRuleRoot,
			"/" + pathAccessPreview,
			"/" + pathServiceLinkedAnalyzer,
			"/" + pathRecommendation + "/",
			"/" + pathFindingV2,
			"/" + pathPolicy + "/",
			"/" + pathAnalyzedResourceHyph,
		} {
			if path == prefix || strings.HasPrefix(path, prefix) {
				return true
			}
		}

		return false
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return matchPriority }

// ExtractOperation extracts the operation name from the request.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	op, _ := parseAllPaths(c.Request().Method, c.Request().URL.Path)

	return op
}

// ExtractResource extracts the resource identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	_, resource := parseAllPaths(c.Request().Method, c.Request().URL.Path)

	return resource
}

// parseAllPaths tries both the primary and extended path parsers.
func parseAllPaths(method, path string) (string, string) {
	op, resource := parseRESTPath(method, path)
	if op != opUnknown {
		return op, resource
	}

	if op2, resource2, ok := parseExtendedRESTPath(method, path); ok {
		return op2, resource2
	}

	return opUnknown, ""
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return h.handleREST(c)
	}
}

// handleREST dispatches the incoming REST request.
func (h *Handler) handleREST(c *echo.Context) error {
	ctx := c.Request().Context()
	log := logger.Load(ctx)

	op, _ := parseAllPaths(c.Request().Method, c.Request().URL.Path)

	if op == opUnknown {
		return c.JSON(http.StatusNotFound, errorBody("ResourceNotFoundException", "not found"))
	}

	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorBody("ValidationException", "failed to read body"))
	}

	result, statusCode, opErr := h.dispatch(ctx, op, c.Request().URL.Path, c.Request().URL.RawQuery, body)
	if opErr != nil {
		log.Error("access-analyzer operation error", "op", op, "err", opErr)

		return h.handleError(c, opErr)
	}

	if result == nil {
		return c.JSON(statusCode, struct{}{})
	}

	data, err := json.Marshal(result)
	if err != nil {
		return c.JSON(http.StatusInternalServerError,
			errorBody("InternalFailure", "failed to serialize response"))
	}

	c.Response().Header().Set("Content-Type", "application/json")

	return c.JSONBlob(statusCode, data)
}

// dispatch routes to the appropriate family-level dispatcher.
func (h *Handler) dispatch(
	_ context.Context,
	op, path, query string,
	body []byte,
) (any, int, error) {
	if result, code, ok, err := h.dispatchAnalyzerOps(op, path, query, body); ok {
		return result, code, err
	}

	if result, code, ok, err := h.dispatchArchiveRuleOps(op, path, query, body); ok {
		return result, code, err
	}

	if result, code, ok, err := h.dispatchFindingOps(op, path, query, body); ok {
		return result, code, err
	}

	if result, code, ok, err := h.dispatchAnalyzedResourceOps(op, path, query, body); ok {
		return result, code, err
	}

	if result, code, ok, err := h.dispatchGeneratedPolicyOps(op, path, query, body); ok {
		return result, code, err
	}

	if result, code, ok, err := h.dispatchAccessPreviewOps(op, path, query, body); ok {
		return result, code, err
	}

	if result, code, ok, err := h.dispatchPolicyValidationOps(op, path, query, body); ok {
		return result, code, err
	}

	return h.dispatchTagOps(op, path, query, body)
}

// handleError writes an error response.
func (h *Handler) handleError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrAnalyzerAlreadyExists), errors.Is(err, ErrArchiveRuleAlreadyExists):
		return c.JSON(http.StatusConflict, errorBody("ConflictException", err.Error()))
	case errors.Is(err, ErrAnalyzerNotFound), errors.Is(err, ErrArchiveRuleNotFound),
		errors.Is(err, ErrFindingNotFound),
		isNotFoundErr(err):
		return c.JSON(http.StatusNotFound, errorBody("ResourceNotFoundException", err.Error()))
	case errors.Is(err, ErrValidation):
		return c.JSON(http.StatusBadRequest, errorBody("ValidationException", err.Error()))
	case errors.Is(err, ErrMalformedPolicy):
		return c.JSON(http.StatusUnprocessableEntity, errorBody("UnprocessableEntityException", err.Error()))
	}

	return c.JSON(http.StatusInternalServerError, errorBody("InternalFailure", err.Error()))
}

func isNotFoundErr(err error) bool {
	var nfe *notFoundErr

	return errors.As(err, &nfe)
}

// ---- URL path parsing (top-level routers; delegate to per-family parsers) ----

// parseRESTPath maps an HTTP method + path to an operation name and resource identifier.
func parseRESTPath(method, path string) (string, string) {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	if len(segments) == 0 {
		return opUnknown, ""
	}

	switch segments[0] {
	case pathAnalyzer:
		return parseAnalyzerPath(method, segments)
	case pathFinding:
		return parseFindingPath(method, segments)
	case pathTags:
		return parseTagsPath(method, segments)
	case pathResource:
		if len(segments) >= 2 && segments[1] == pathScan && method == http.MethodPost {
			return opStartResourceScan, ""
		}
	}

	return opUnknown, ""
}

// parseExtendedRESTPath parses the REST paths added after the original
// analyzer/archive-rule/finding/tags surface: archive-rule (apply), access
// previews, service-linked analyzers, finding recommendations, analyzed
// resources, findings v2, and policy validation/generation.
// Returns (op, resource, ok) — ok=true means the path was handled.
func parseExtendedRESTPath(method, path string) (string, string, bool) {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segments) == 0 {
		return "", "", false
	}

	switch segments[0] {
	case pathArchiveRuleRoot:
		if len(segments) == 1 && method == http.MethodPut {
			return opApplyArchiveRule, "", true
		}

	case pathAccessPreview:
		return parseAccessPreviewPath(method, segments)

	case pathServiceLinkedAnalyzer:
		return parseServiceLinkedAnalyzerPath(method, segments)

	case pathRecommendation:
		return parseRecommendationPath(method, segments)

	case pathAnalyzedResourceHyph:
		switch method {
		case http.MethodGet:
			return opGetAnalyzedResource, "", true
		case http.MethodPost:
			return opListAnalyzedResources, "", true
		}

	case pathFindingV2:
		return parseFindingV2Path(method, segments)

	case pathPolicy:
		return parsePolicyPath(method, segments)
	}

	return "", "", false
}

// ---- shared path parameter extraction ----

// extractAnalyzerName extracts the analyzer name from a path.
// For /analyzer/{name}/... returns name.
func extractAnalyzerName(path string) string {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	for i, s := range segments {
		if s == pathAnalyzer && i+1 < len(segments) {
			return segments[i+1]
		}
	}

	return ""
}

// extractLastSegment extracts the last path segment after the given prefix segment.
// For /access-preview/{id}, extractLastSegment(path, "access-preview") returns the id.
func extractLastSegment(path, prefix string) string {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")

	for i, s := range segments {
		if s == prefix && i+1 < len(segments) {
			return segments[i+1]
		}
	}

	return ""
}

// queryParamValue reads a single value from a raw (still percent-encoded)
// URL query string, the form c.Request().URL.RawQuery and every op's own
// httpbinding-serialized request carries it in. ARNs contain ":" and "/",
// which the real SDK client always percent-encodes on the wire (verified
// against awsRestjson1_serializeOpHttpBindingsGenerateFindingRecommendationInput
// in aws-sdk-go-v2/service/accessanalyzer@v1.51.4 serializers.go), so the
// value must be unescaped before use -- an unescaped comparison against a
// decoded ARN stored by the backend never matches.
func queryParamValue(query, key string) string {
	prefix := key + "="

	for part := range strings.SplitSeq(query, "&") {
		v, ok := strings.CutPrefix(part, prefix)
		if !ok {
			continue
		}

		decoded, err := url.QueryUnescape(v)
		if err != nil {
			return v
		}

		return decoded
	}

	return ""
}

// errorBody constructs a JSON error payload.
func errorBody(code, message string) map[string]string {
	return map[string]string{
		"__type":  code,
		"message": message,
	}
}
