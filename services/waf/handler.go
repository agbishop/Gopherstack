package waf

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	wafTargetPrefix = "AWSWAF_20150824."
	wafContentType  = "application/x-amz-json-1.1"

	keyChangeToken = "ChangeToken"
	keyRule        = "Rule"
)

// Handler serves WAF Classic JSON operations.
type Handler struct {
	Backend StorageBackend
	ops     map[string]func([]byte) (any, error)
}

// NewHandler creates a WAF Classic handler backed by b.
func NewHandler(b StorageBackend) *Handler {
	h := &Handler{Backend: b}
	h.ops = h.buildOps()

	return h
}

// Name returns the service name.
func (h *Handler) Name() string { return "WAF" }

// Reset clears backend state.
func (h *Handler) Reset() { h.Backend.Reset() }

// MatchPriority returns header matching priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// RouteMatcher matches WAF Classic X-Amz-Target headers.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), wafTargetPrefix)
	}
}

// ExtractOperation extracts the WAF Classic action from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	if !strings.HasPrefix(target, wafTargetPrefix) {
		return "Unknown"
	}

	return strings.TrimPrefix(target, wafTargetPrefix)
}

// ExtractResource extracts a resource identifier from the JSON body.
func (h *Handler) ExtractResource(_ *echo.Context) string {
	return ""
}

// GetSupportedOperations returns all implemented operation names.
func (h *Handler) GetSupportedOperations() []string {
	ops := make([]string, 0, len(h.ops))
	for name := range h.ops {
		ops = append(ops, name)
	}

	return ops
}

// Snapshot returns a serialized snapshot of the backend state.
func (h *Handler) Snapshot(ctx context.Context) []byte { return h.Backend.Snapshot(ctx) }

// Restore restores the backend state from a snapshot.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	return h.Backend.Restore(ctx, data)
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c, logger.Load(c.Request().Context()), h.Name(), wafContentType,
			h.GetSupportedOperations(), h.dispatch, h.handleError,
		)
	}
}

func (h *Handler) dispatch(_ context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, ErrInvalidParameter
	}

	result, err := fn(body)
	if err != nil {
		return nil, err
	}

	// Transition any ChangeToken present in the request body to INSYNC.
	// GetChangeTokenStatus is a read — it carries the token as a lookup key, not as a consumed token.
	if action != "GetChangeTokenStatus" {
		var tokenHolder struct {
			ChangeToken string `json:"ChangeToken"`
		}
		if jerr := json.Unmarshal(body, &tokenHolder); jerr == nil && tokenHolder.ChangeToken != "" {
			h.Backend.MarkChangeTokenUsed(tokenHolder.ChangeToken)
		}
	}

	return json.Marshal(result)
}

// paginate returns a window of items starting after nextMarker, limited to limit items,
// and the next page marker (last returned item's key), or "" if no more pages.
func paginate[T any](items []T, nextMarker string, limit int, keyFn func(T) string) ([]T, string) {
	const defaultLimit = 100
	if limit <= 0 {
		limit = defaultLimit
	}

	start := 0
	if nextMarker != "" {
		for i, item := range items {
			if keyFn(item) == nextMarker {
				start = i + 1

				break
			}
		}
	}
	items = items[start:]

	if len(items) > limit {
		return items[:limit], keyFn(items[limit-1])
	}

	return items, ""
}

func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	code := "WAFInternalErrorException"
	status := http.StatusInternalServerError

	switch {
	case errors.Is(err, ErrNotFound):
		code, status = errResourceNotFound, http.StatusBadRequest
	case errors.Is(err, ErrStaleToken):
		code, status = errStaleData, http.StatusBadRequest
	case errors.Is(err, ErrInvalidParameter):
		code, status = errInvalidParameter, http.StatusBadRequest
	case errors.Is(err, ErrReferencedItem):
		code, status = errReferencedItem, http.StatusBadRequest
	case errors.Is(err, ErrNonEmptyEntity):
		code, status = errNonEmptyEntity, http.StatusBadRequest
	case errors.Is(err, ErrInvalidOperation):
		code, status = errInvalidOperation, http.StatusBadRequest
	}

	return c.JSON(status, service.JSONErrorResponse{Type: code, Message: err.Error()})
}

func (h *Handler) buildOps() map[string]func([]byte) (any, error) {
	return map[string]func([]byte) (any, error){
		"GetChangeToken":       h.opGetChangeToken,
		"GetChangeTokenStatus": h.opGetChangeTokenStatus,

		"CreateWebACL": h.opCreateWebACL,
		"GetWebACL":    h.opGetWebACL,
		"UpdateWebACL": h.opUpdateWebACL,
		"DeleteWebACL": h.opDeleteWebACL,
		"ListWebACLs":  h.opListWebACLs,

		"CreateRule": h.opCreateRule,
		"GetRule":    h.opGetRule,
		"UpdateRule": h.opUpdateRule,
		"DeleteRule": h.opDeleteRule,
		"ListRules":  h.opListRules,

		"CreateIPSet": h.opCreateIPSet,
		"GetIPSet":    h.opGetIPSet,
		"UpdateIPSet": h.opUpdateIPSet,
		"DeleteIPSet": h.opDeleteIPSet,
		"ListIPSets":  h.opListIPSets,

		"CreateByteMatchSet": h.opCreateByteMatchSet,
		"GetByteMatchSet":    h.opGetByteMatchSet,
		"UpdateByteMatchSet": h.opUpdateByteMatchSet,
		"DeleteByteMatchSet": h.opDeleteByteMatchSet,
		"ListByteMatchSets":  h.opListByteMatchSets,

		"CreateSizeConstraintSet": h.opCreateSizeConstraintSet,
		"GetSizeConstraintSet":    h.opGetSizeConstraintSet,
		"UpdateSizeConstraintSet": h.opUpdateSizeConstraintSet,
		"DeleteSizeConstraintSet": h.opDeleteSizeConstraintSet,
		"ListSizeConstraintSets":  h.opListSizeConstraintSets,

		"CreateSqlInjectionMatchSet": h.opCreateSqlInjectionMatchSet,
		"GetSqlInjectionMatchSet":    h.opGetSqlInjectionMatchSet,
		"UpdateSqlInjectionMatchSet": h.opUpdateSqlInjectionMatchSet,
		"DeleteSqlInjectionMatchSet": h.opDeleteSqlInjectionMatchSet,
		"ListSqlInjectionMatchSets":  h.opListSqlInjectionMatchSets,

		"CreateXssMatchSet": h.opCreateXssMatchSet,
		"GetXssMatchSet":    h.opGetXssMatchSet,
		"UpdateXssMatchSet": h.opUpdateXssMatchSet,
		"DeleteXssMatchSet": h.opDeleteXssMatchSet,
		"ListXssMatchSets":  h.opListXssMatchSets,

		"CreateGeoMatchSet": h.opCreateGeoMatchSet,
		"GetGeoMatchSet":    h.opGetGeoMatchSet,
		"UpdateGeoMatchSet": h.opUpdateGeoMatchSet,
		"DeleteGeoMatchSet": h.opDeleteGeoMatchSet,
		"ListGeoMatchSets":  h.opListGeoMatchSets,

		"CreateRateBasedRule":         h.opCreateRateBasedRule,
		"GetRateBasedRule":            h.opGetRateBasedRule,
		"UpdateRateBasedRule":         h.opUpdateRateBasedRule,
		"DeleteRateBasedRule":         h.opDeleteRateBasedRule,
		"ListRateBasedRules":          h.opListRateBasedRules,
		"GetRateBasedRuleManagedKeys": h.opGetRateBasedRuleManagedKeys,

		"CreateRegexPatternSet": h.opCreateRegexPatternSet,
		"GetRegexPatternSet":    h.opGetRegexPatternSet,
		"UpdateRegexPatternSet": h.opUpdateRegexPatternSet,
		"DeleteRegexPatternSet": h.opDeleteRegexPatternSet,
		"ListRegexPatternSets":  h.opListRegexPatternSets,

		"CreateRegexMatchSet": h.opCreateRegexMatchSet,
		"GetRegexMatchSet":    h.opGetRegexMatchSet,
		"UpdateRegexMatchSet": h.opUpdateRegexMatchSet,
		"DeleteRegexMatchSet": h.opDeleteRegexMatchSet,
		"ListRegexMatchSets":  h.opListRegexMatchSets,

		"CreateRuleGroup":               h.opCreateRuleGroup,
		"GetRuleGroup":                  h.opGetRuleGroup,
		"UpdateRuleGroup":               h.opUpdateRuleGroup,
		"DeleteRuleGroup":               h.opDeleteRuleGroup,
		"ListRuleGroups":                h.opListRuleGroups,
		"ListActivatedRulesInRuleGroup": h.opListActivatedRulesInRuleGroup,
		"ListSubscribedRuleGroups":      h.opListSubscribedRuleGroups,

		"PutLoggingConfiguration":    h.opPutLoggingConfiguration,
		"GetLoggingConfiguration":    h.opGetLoggingConfiguration,
		"DeleteLoggingConfiguration": h.opDeleteLoggingConfiguration,
		"ListLoggingConfigurations":  h.opListLoggingConfigurations,

		"PutPermissionPolicy":    h.opPutPermissionPolicy,
		"GetPermissionPolicy":    h.opGetPermissionPolicy,
		"DeletePermissionPolicy": h.opDeletePermissionPolicy,

		"CreateWebACLMigrationStack": h.opCreateWebACLMigrationStack,

		"TagResource":         h.opTagResource,
		"UntagResource":       h.opUntagResource,
		"ListTagsForResource": h.opListTagsForResource,

		"GetSampledRequests": h.opGetSampledRequests,
	}
}

// --- helpers ---

func unmarshal(body []byte, v any) error {
	if len(body) == 0 {
		return nil
	}

	return json.Unmarshal(body, v)
}

func tagsToMap(tags []Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}

	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[t.Key] = t.Value
	}

	return m
}
