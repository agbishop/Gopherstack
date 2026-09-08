package wafv2

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	keyTypeField        = "__type"
	keyMessageField     = "message"
	keySummary          = "Summary"
	keyName             = "Name"
	keyARN              = "ARN"
	keyLockToken        = "LockToken"
	keyDescription      = "Description"
	keyVisibilityConfig = "VisibilityConfig"
	keyNextLockToken    = "NextLockToken"
	keyScope            = "Scope"
	keyIPAddressVersion = "IPAddressVersion"
	keyAddresses        = "Addresses"
	keyRules            = "Rules"
	keyCapacity         = "Capacity"
	keyVendorName       = "VendorName"
	keyLabelNamespace   = "LabelNamespace"
)
const (
	wafv2Service       = "wafv2"
	wafv2TargetPrefix  = "AWSWAF_20190729."
	wafv2MatchPriority = service.PriorityHeaderExact
	defaultActionAllow = "ALLOW"

	// maxAPIKeyTokenDomains is the AWS-imposed limit for token domains per API key.
	maxAPIKeyTokenDomains = 5

	// maxSampledRequestsItems is the maximum value for GetSampledRequests.MaxItems.
	maxSampledRequestsItems = 500

	// maxTopPathStatisticsLimit is the maximum value for
	// GetTopPathStatisticsByTraffic.Limit.
	maxTopPathStatisticsLimit = 100

	// maxTopTrafficBotsPerPath is the maximum value for
	// GetTopPathStatisticsByTraffic.NumberOfTopTrafficBotsPerPath.
	maxTopTrafficBotsPerPath = 10
)

var (
	errUnknownAction  = errors.New("unknown action")
	errInvalidRequest = errors.New("invalid request")
)

// requireIDNameScopeLockToken validates the Id/Name/Scope/LockToken members every
// Update*/Delete* op in the WebACL/IPSet/RuleGroup/RegexPatternSet families marks
// required (wafv2@v1.77.3 validators.go, e.g. validateOpUpdateWebACLInput). LockToken
// is required so an omitted token can't silently bypass optimistic locking.
func requireIDNameScopeLockToken(id, name, scope, lockToken string) error {
	if id == "" {
		return fmt.Errorf("%w: Id is required", errInvalidRequest)
	}

	if name == "" {
		return fmt.Errorf("%w: Name is required", errInvalidRequest)
	}

	if scope == "" {
		return fmt.Errorf("%w: Scope is required", errInvalidRequest)
	}

	if lockToken == "" {
		return fmt.Errorf("%w: LockToken is required", errInvalidRequest)
	}

	return nil
}

// dispatchFn is the signature every WAFv2 operation handler is normalized to for
// registration in the dispatch table.
type dispatchFn = func(context.Context, []byte) ([]byte, error)

// Handler is the HTTP handler for the AWS WAFv2 API.
type Handler struct {
	// Backend is the storage interface for WAFv2 operations.
	Backend StorageBackend
	// ops is the dispatch table built once at construction time.
	ops       map[string]dispatchFn
	AccountID string
	Region    string
}

// NewHandler creates a new WAFv2 handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{
		Backend:   backend,
		AccountID: backend.accountID,
		Region:    backend.region,
	}
	h.ops = h.buildDispatchTable()

	return h
}

// Reset clears all backend state.
func (h *Handler) Reset() { h.Backend.Reset() }

// Name returns the service name.
func (h *Handler) Name() string { return "Wafv2" }

// GetSupportedOperations returns the list of supported WAFv2 operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"AssociateWebACL",
		"CheckCapacity",
		"CreateAPIKey",
		"CreateIPSet",
		"CreateRegexPatternSet",
		"CreateRuleGroup",
		"CreateWebACL",
		"DeleteAPIKey",
		"DeleteFirewallManagerRuleGroups",
		"DeleteIPSet",
		"DeleteLoggingConfiguration",
		"DeletePermissionPolicy",
		"DeleteRegexPatternSet",
		"DeleteRuleGroup",
		"DeleteWebACL",
		"DescribeAllManagedProducts",
		"DescribeManagedProductsByVendor",
		"DescribeManagedRuleGroup",
		"DisassociateWebACL",
		"GenerateMobileSdkReleaseUrl",
		"GetDecryptedAPIKey",
		"GetIPSet",
		"GetLoggingConfiguration",
		"GetManagedRuleSet",
		"GetMobileSdkRelease",
		"GetPermissionPolicy",
		"GetRateBasedStatementManagedKeys",
		"GetRegexPatternSet",
		"GetRevenueStatistics",
		"GetRevenueStatisticsSummary",
		"GetRevenueStatisticsTimeSeries",
		"GetRuleGroup",
		"GetSampledRequests",
		"GetTopPathStatisticsByTraffic",
		"GetWebACL",
		"GetWebACLForResource",
		"ListAPIKeys",
		"ListAvailableManagedRuleGroupVersions",
		"ListAvailableManagedRuleGroups",
		"ListIPSets",
		"ListLoggingConfigurations",
		"ListManagedRuleSets",
		"ListMobileSdkReleases",
		"ListRegexPatternSets",
		"ListResourcesForWebACL",
		"ListRuleGroups",
		"ListSettlementRecords",
		"ListTagsForResource",
		"ListWebACLs",
		"PutLoggingConfiguration",
		"PutManagedRuleSetVersions",
		"PutPermissionPolicy",
		"TagResource",
		"UntagResource",
		"UpdateIPSet",
		"UpdateManagedRuleSetVersionExpiryDate",
		"UpdateRegexPatternSet",
		"UpdateRuleGroup",
		"UpdateWebACL",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return wafv2Service }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Region} }

// RouteMatcher returns a function that matches WAFv2 API requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), wafv2TargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return wafv2MatchPriority }

// ExtractOperation extracts the operation name from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")

	return strings.TrimPrefix(target, wafv2TargetPrefix)
}

// ExtractResource extracts the resource identifier from the request.
func (h *Handler) ExtractResource(c *echo.Context) string {
	return h.ExtractOperation(c)
}

// Handler returns the Echo handler function for WAFv2 requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		ctx := c.Request().Context()
		log := logger.Load(ctx)

		body, err := httputils.ReadBody(c.Request())
		if err != nil {
			log.ErrorContext(ctx, "wafv2: failed to read request body", "error", err)

			return h.handleError(c, err)
		}

		op := h.ExtractOperation(c)

		result, dispErr := h.dispatch(ctx, op, body)
		if dispErr != nil {
			return h.handleError(c, dispErr)
		}

		if result == nil {
			return c.JSONBlob(http.StatusOK, []byte("{}"))
		}

		return c.JSONBlob(http.StatusOK, result)
	}
}

// buildDispatchTable merges each operation family's dispatch map -- defined alongside
// that family's handlers (e.g. webACLDispatchOps in handler_web_acls.go,
// ipSetDispatchOps in handler_ip_sets.go) -- into the single table dispatch uses. Splitting
// registration by family keeps every contributing function small instead of one giant map
// literal enumerating all operations.
func (h *Handler) buildDispatchTable() map[string]dispatchFn {
	ops := make(map[string]dispatchFn)

	for _, group := range []map[string]dispatchFn{
		h.webACLDispatchOps(),
		h.ipSetDispatchOps(),
		h.tagsDispatchOps(),
		h.resourceAssociationDispatchOps(),
		h.ruleGroupDispatchOps(),
		h.apiKeyDispatchOps(),
		h.regexPatternSetDispatchOps(),
		h.loggingConfigDispatchOps(),
		h.permissionPolicyDispatchOps(),
		h.managedRuleSetDispatchOps(),
		h.rateBasedRuleDispatchOps(),
		h.managedRuleCatalogDispatchOps(),
		h.revenueStatisticsDispatchOps(),
	} {
		maps.Copy(ops, group)
	}

	return ops
}
func (h *Handler) dispatch(ctx context.Context, op string, body []byte) ([]byte, error) {
	fn, ok := h.ops[op]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnknownAction, op)
	}

	return fn(ctx, body)
}
func (h *Handler) handleError(c *echo.Context, err error) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	var errType string
	var statusCode int

	switch {
	case errors.Is(err, awserr.ErrNotFound):
		errType = "WAFNonexistentItemException"
		statusCode = http.StatusBadRequest
	case errors.Is(err, ErrOptimisticLock):
		errType = "WAFOptimisticLockException"
		statusCode = http.StatusBadRequest
	case errors.Is(err, ErrAssociatedItem):
		errType = "WAFAssociatedItemException"
		statusCode = http.StatusBadRequest
	case errors.Is(err, ErrLimitsExceeded):
		errType = "WAFLimitsExceededException"
		statusCode = http.StatusBadRequest
	case errors.Is(err, ErrTagOperation):
		errType = "WAFTagOperationException"
		statusCode = http.StatusBadRequest
	case errors.Is(err, ErrUnavailableEntity):
		errType = "WAFUnavailableEntityException"
		statusCode = http.StatusBadRequest
	case errors.Is(err, awserr.ErrConflict):
		errType = "WAFDuplicateItemException"
		statusCode = http.StatusBadRequest
	case errors.Is(err, ErrConfigurationWarning):
		errType = "WAFConfigurationWarningException"
		statusCode = http.StatusBadRequest
	case errors.Is(err, errInvalidRequest), errors.As(err, &syntaxErr), errors.As(err, &typeErr):
		errType = "WAFInvalidParameterException"
		statusCode = http.StatusBadRequest
	case errors.Is(err, errUnknownAction):
		errType = "WAFInvalidOperationException"
		statusCode = http.StatusBadRequest
	default:
		errType = "WAFInternalErrorException"
		statusCode = http.StatusInternalServerError
	}

	payload, _ := json.Marshal(map[string]string{
		keyTypeField:    errType,
		keyMessageField: err.Error(),
	})

	c.Response().Header().Set("X-Amzn-Errortype", errType)

	return c.JSONBlob(statusCode, payload)
}

// parseVisibilityConfig parses a stored VisibilityConfig JSON string or returns a default.
func parseVisibilityConfig(stored json.RawMessage, metricName string) map[string]any {
	if len(stored) > 0 {
		var m map[string]any
		if err := json.Unmarshal(stored, &m); err == nil {
			return m
		}
	}

	return map[string]any{
		"CloudWatchMetricsEnabled": false,
		"MetricName":               metricName,
		"SampledRequestsEnabled":   false,
	}
}

// paginateByName implements cursor-based pagination over name-sorted items.
// The cursor is base64(last-name-seen). Returns the page of items and the
// next marker (empty string if no more pages).
func paginateByName[T any](items []T, getName func(T) string, nextMarker string, limit int) ([]T, string) {
	// Decode cursor.
	startAfter := ""

	if nextMarker != "" {
		decoded, err := base64.StdEncoding.DecodeString(nextMarker)
		if err == nil {
			startAfter = string(decoded)
		}
	}

	// Skip items up to and including the cursor name.
	start := 0

	if startAfter != "" {
		for start < len(items) && getName(items[start]) <= startAfter {
			start++
		}
	}

	items = items[start:]

	// Apply limit.
	if limit <= 0 || limit > len(items) {
		return items, ""
	}

	page := items[:limit]
	lastName := getName(page[len(page)-1])
	newMarker := base64.StdEncoding.EncodeToString([]byte(lastName))

	return page, newMarker
}

// paginateByNameID is paginateByName's counterpart for a collection whose
// Name is not guaranteed unique (ManagedRuleSet: PutManagedRuleSetVersions
// keys strictly on the caller-supplied Id, unlike CreateWebACL/CreateIPSet/
// CreateRegexPatternSet/CreateRuleGroup's name-uniqueness check, so two
// ManagedRuleSets can share a Name). paginateByName's marker only encodes
// the last name seen and resumes by skipping every item whose name is <=
// that marker -- fine when Name is a total order, but when several items
// tie on Name it drops every item in the tie group after the first page
// boundary lands inside it, deterministically (not just map-order
// dependently), every time. The marker here also encodes the last id seen
// so a same-name tie resumes at the exact record already returned. items
// must already be sorted by (name, id): callers get that by falling
// through to id as the final comparison whenever name compares equal.
func paginateByNameID[T any](
	items []T, getName, getID func(T) string, nextMarker string, limit int,
) ([]T, string) {
	startAfterName, startAfterID := "", ""

	if nextMarker != "" {
		decoded, err := base64.StdEncoding.DecodeString(nextMarker)
		if err == nil {
			startAfterName, startAfterID, _ = strings.Cut(string(decoded), "\x00")
		}
	}

	start := 0

	if startAfterName != "" || startAfterID != "" {
		for start < len(items) {
			name, id := getName(items[start]), getID(items[start])
			if name > startAfterName || (name == startAfterName && id > startAfterID) {
				break
			}

			start++
		}
	}

	items = items[start:]

	if limit <= 0 || limit > len(items) {
		return items, ""
	}

	page := items[:limit]
	last := page[len(page)-1]
	newMarker := base64.StdEncoding.EncodeToString([]byte(getName(last) + "\x00" + getID(last)))

	return page, newMarker
}

// listResourceSummaries implements the shared scope-filter + paginate + summarize logic
// behind handleListWebACLs, handleListIPSets, handleListRegexPatternSets, and
// handleListRuleGroups: those four handlers are otherwise structurally identical aside
// from field access and the shape of each summary map, so they are expressed here as one
// generic helper instead of four near-duplicate functions.
func listResourceSummaries[T any](
	items []T,
	scope, nextMarker string,
	limit int,
	getScope func(T) string,
	getName func(T) string,
	buildSummary func(T) map[string]string,
) ([]map[string]string, string) {
	filtered := make([]T, 0, len(items))

	for _, item := range items {
		if scope != "" && getScope(item) != scope {
			continue
		}

		filtered = append(filtered, item)
	}

	page, newMarker := paginateByName(filtered, getName, nextMarker, limit)

	summaries := make([]map[string]string, 0, len(page))
	for _, item := range page {
		summaries = append(summaries, buildSummary(item))
	}

	return summaries, newMarker
}

// listFamilyRequest is the shared request shape for ListWebACLs, ListIPSets,
// ListRegexPatternSets, and ListRuleGroups.
type listFamilyRequest struct {
	Scope      string `json:"Scope"`
	NextMarker string `json:"NextMarker"`
	Limit      int    `json:"Limit"`
}

// handleListResourceFamily implements the shared unmarshal + scope-filter + paginate +
// marshal logic behind handleListWebACLs, handleListIPSets, handleListRegexPatternSets,
// and handleListRuleGroups. Those four handlers are otherwise identical aside from field
// access, the backend list call, and the shape of each item's summary, so this one generic
// helper -- combined with listResourceSummaries above -- replaces four near-duplicate
// functions with four thin callers (see e.g. handleListWebACLs in handler_web_acls.go).
func handleListResourceFamily[T any](
	body []byte,
	items []T,
	responseKey string,
	getScope func(T) string,
	getName func(T) string,
	buildSummary func(T) map[string]string,
) ([]byte, error) {
	var req listFamilyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	summaries, nextMarker := listResourceSummaries(
		items, req.Scope, req.NextMarker, req.Limit, getScope, getName, buildSummary,
	)

	resp := map[string]any{responseKey: summaries}
	if nextMarker != "" {
		resp["NextMarker"] = nextMarker
	}

	return json.Marshal(resp)
}
