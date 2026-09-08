package wafv2_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/wafv2"
)

func newTestHandler(t *testing.T) *wafv2.Handler {
	t.Helper()

	return wafv2.NewHandler(wafv2.NewInMemoryBackend("000000000000", "us-east-1"))
}

func doWafv2Request(
	t *testing.T,
	h *wafv2.Handler,
	target string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte

	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSWAF_20190729."+target)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetRequest(req)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestHandler_Name(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, "Wafv2", h.Name())
}

func TestBackend_Region(t *testing.T) {
	t.Parallel()

	b := wafv2.NewInMemoryBackend("000000000000", "eu-west-1")
	assert.Equal(t, "eu-west-1", b.Region())
}

func TestHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "matching_target", target: "AWSWAF_20190729.ListWebACLs", want: true},
		{name: "non_matching_target", target: "SageMaker.ListModels", want: false},
		{name: "empty_target", target: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			assert.Equal(t, tt.want, h.RouteMatcher()(c))
		})
	}
}

func TestHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Amz-Target", "AWSWAF_20190729.CreateWebACL")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.Equal(t, "CreateWebACL", h.ExtractResource(c))
}

func TestProvider_InitAndName(t *testing.T) {
	t.Parallel()

	p := &wafv2.Provider{}
	assert.Equal(t, "Wafv2", p.Name())

	h, err := p.Init(&service.AppContext{})
	require.NoError(t, err)
	assert.NotNil(t, h)
}

func TestHandler_UnknownOperation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(t, h, "UnknownOperation", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var result map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, "WAFInvalidOperationException", result["__type"])
}

func TestHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.GetSupportedOperations()

	tests := []struct {
		name string
		op   string
	}{
		{name: "AssociateWebACL", op: "AssociateWebACL"},
		{name: "DisassociateWebACL", op: "DisassociateWebACL"},
		{name: "GetWebACLForResource", op: "GetWebACLForResource"},
		{name: "UntagResource", op: "UntagResource"},
		{name: "TagResource", op: "TagResource"},
		{name: "ListTagsForResource", op: "ListTagsForResource"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Contains(t, ops, tt.op)
		})
	}
}

func TestHandler_ChaosMetadata(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "service_name", got: h.ChaosServiceName(), want: "wafv2"},
		{name: "region", got: h.ChaosRegions()[0], want: "us-east-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.got)
		})
	}
}

func TestHandler_MatchPriority(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, service.PriorityHeaderExact, h.MatchPriority())
}

func TestHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "valid_target", target: "AWSWAF_20190729.CreateWebACL", want: "CreateWebACL"},
		{name: "no_prefix", target: "CreateWebACL", want: "CreateWebACL"},
		{name: "empty", target: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

func TestBackend_Reset(t *testing.T) {
	t.Parallel()

	b := wafv2.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := wafv2.CreateWebACLSimple(b, "acl1", "REGIONAL", "", "ALLOW", nil)
	require.NoError(t, err)
	_, err = b.CreateIPSet(context.Background(), "set1", "REGIONAL", "", "IPV4", nil, nil)
	require.NoError(t, err)

	b.Reset()

	assert.Empty(t, b.ListWebACLs(context.Background()))
	assert.Empty(t, b.ListIPSets(context.Background()))
}

func TestHandler_ChaosOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.ChaosOperations()
	assert.Equal(t, h.GetSupportedOperations(), ops)
}

func TestLockTokenEnforcement(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id, realToken := createWebACLWithRules(t, h, "acl-lock", "REGIONAL")

	// Update with wrong lock token should fail.
	recBad := doWafv2Request(t, h, "UpdateWebACL", map[string]any{
		"Id":          id,
		"Name":        "acl-lock",
		"Scope":       "REGIONAL",
		"LockToken":   "wrong-token",
		"Description": "should fail",
	})
	assert.Equal(t, http.StatusBadRequest, recBad.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(recBad.Body.Bytes(), &errResp))
	assert.Equal(t, "WAFOptimisticLockException", errResp["__type"])

	// Update with correct lock token should succeed.
	recGood := doWafv2Request(t, h, "UpdateWebACL", map[string]any{
		"Id":          id,
		"Name":        "acl-lock",
		"Scope":       "REGIONAL",
		"LockToken":   realToken,
		"Description": "updated successfully",
	})
	assert.Equal(t, http.StatusOK, recGood.Code)
}

// TestLockTokenRequired_MissingTokenRejected: LockToken is a required member
// on every Update*/Delete* op across the WebACL/IPSet/RuleGroup/
// RegexPatternSet families (wafv2@v1.77.3 validators.go,
// validateOpUpdateWebACLInput et al. all call
// invalidParams.Add(smithy.NewErrParamRequired("LockToken"))), but the
// handlers only checked it for a mismatch when non-empty -- an omitted
// LockToken silently bypassed optimistic locking entirely.
func TestLockTokenRequired_MissingTokenRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(t *testing.T, h *wafv2.Handler) map[string]any
		name   string
		target string
	}{
		{
			name:   "update_web_acl",
			target: "UpdateWebACL",
			setup: func(t *testing.T, h *wafv2.Handler) map[string]any {
				t.Helper()
				id, _ := createWebACLWithRules(t, h, "acl-no-token", "REGIONAL")

				return map[string]any{"Id": id, "Name": "acl-no-token", "Scope": "REGIONAL", "Description": "x"}
			},
		},
		{
			name:   "delete_web_acl",
			target: "DeleteWebACL",
			setup: func(t *testing.T, h *wafv2.Handler) map[string]any {
				t.Helper()
				id, _ := createWebACLWithRules(t, h, "acl-no-token-del", "REGIONAL")

				return map[string]any{"Id": id, "Name": "acl-no-token-del", "Scope": "REGIONAL"}
			},
		},
		{
			name:   "update_ip_set",
			target: "UpdateIPSet",
			setup: func(t *testing.T, h *wafv2.Handler) map[string]any {
				t.Helper()
				id, _ := createIPSetHelper2(t, h, "ipset-no-token", nil)

				return map[string]any{
					"Id": id, "Name": "ipset-no-token", "Scope": "REGIONAL", "Addresses": []string{"10.0.0.0/8"},
				}
			},
		},
		{
			name:   "delete_ip_set",
			target: "DeleteIPSet",
			setup: func(t *testing.T, h *wafv2.Handler) map[string]any {
				t.Helper()
				id, _ := createIPSetHelper2(t, h, "ipset-no-token-del", nil)

				return map[string]any{"Id": id, "Name": "ipset-no-token-del", "Scope": "REGIONAL"}
			},
		},
		{
			name:   "update_rule_group",
			target: "UpdateRuleGroup",
			setup: func(t *testing.T, h *wafv2.Handler) map[string]any {
				t.Helper()
				id, _ := createRuleGroupHelper(t, h, "rg-no-token")

				return map[string]any{"Id": id, "Name": "rg-no-token", "Scope": "REGIONAL", "Description": "x"}
			},
		},
		{
			name:   "delete_rule_group",
			target: "DeleteRuleGroup",
			setup: func(t *testing.T, h *wafv2.Handler) map[string]any {
				t.Helper()
				id, _ := createRuleGroupHelper(t, h, "rg-no-token-del")

				return map[string]any{"Id": id, "Name": "rg-no-token-del", "Scope": "REGIONAL"}
			},
		},
		{
			name:   "update_regex_pattern_set",
			target: "UpdateRegexPatternSet",
			setup: func(t *testing.T, h *wafv2.Handler) map[string]any {
				t.Helper()
				id := createRegexPatternSetHelper(t, h, "rps-no-token")

				return map[string]any{"Id": id, "Name": "rps-no-token", "Scope": "REGIONAL", "Description": "x"}
			},
		},
		{
			name:   "delete_regex_pattern_set",
			target: "DeleteRegexPatternSet",
			setup: func(t *testing.T, h *wafv2.Handler) map[string]any {
				t.Helper()
				id := createRegexPatternSetHelper(t, h, "rps-no-token-del")

				return map[string]any{"Id": id, "Name": "rps-no-token-del", "Scope": "REGIONAL"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := tt.setup(t, h)

			rec := doWafv2Request(t, h, tt.target, body)
			assert.Equal(
				t, http.StatusBadRequest, rec.Code,
				"missing LockToken must be rejected: %s", rec.Body.String(),
			)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "WAFInvalidParameterException", resp["__type"])
		})
	}
}

// ---- Gap 4: Managed rule group catalog --------------------------------------

func TestVisibilityConfigMissingMetricName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-no-metric",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"CloudWatchMetricsEnabled": true,
			"MetricName":               "", // missing
			"SampledRequestsEnabled":   false,
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "WAFInvalidParameterException", resp["__type"])
}

// ---- Gap 7: RuleGroup Capacity enforcement ----------------------------------

func TestListPagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create 5 WebACLs (names are alphabetically ordered).
	for i := range 5 {
		rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
			"Name":          fmt.Sprintf("acl-%02d", i),
			"Scope":         "REGIONAL",
			"DefaultAction": map[string]any{"Allow": map[string]any{}},
			"VisibilityConfig": map[string]any{
				"MetricName": fmt.Sprintf("acl-%02d", i),
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// List with limit 3 — should get first page + NextMarker.
	page1Rec := doWafv2Request(t, h, "ListWebACLs", map[string]any{
		"Scope": "REGIONAL",
		"Limit": 3,
	})
	require.Equal(t, http.StatusOK, page1Rec.Code)

	var page1Resp map[string]any
	require.NoError(t, json.Unmarshal(page1Rec.Body.Bytes(), &page1Resp))

	acls1, ok := page1Resp["WebACLs"].([]any)
	require.True(t, ok)
	assert.Len(t, acls1, 3, "first page should have 3 items")

	nextMarker, ok := page1Resp["NextMarker"].(string)
	require.True(t, ok, "NextMarker should be present after first page")
	assert.NotEmpty(t, nextMarker)

	// Decode and verify it's base64.
	decoded, err := base64.StdEncoding.DecodeString(nextMarker)
	require.NoError(t, err)
	assert.NotEmpty(t, string(decoded))

	// Fetch second page.
	page2Rec := doWafv2Request(t, h, "ListWebACLs", map[string]any{
		"Scope":      "REGIONAL",
		"Limit":      3,
		"NextMarker": nextMarker,
	})
	require.Equal(t, http.StatusOK, page2Rec.Code)

	var page2Resp map[string]any
	require.NoError(t, json.Unmarshal(page2Rec.Body.Bytes(), &page2Resp))

	acls2, ok := page2Resp["WebACLs"].([]any)
	require.True(t, ok)
	assert.Len(t, acls2, 2, "second page should have 2 items")

	_, hasNextMarker := page2Resp["NextMarker"]
	assert.False(t, hasNextMarker, "second page should not have NextMarker")
}

// ---- Gap 22: Scope validation on Get/Update/Delete -------------------------

func TestScopeValidationOnGet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create a REGIONAL WebACL.
	createRec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":             "scope-test",
		"Scope":            "REGIONAL",
		"DefaultAction":    map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{"MetricName": "scope-test"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	// Get with wrong scope should fail.
	getRec := doWafv2Request(t, h, "GetWebACL", map[string]any{
		"Id":    id,
		"Scope": "CLOUDFRONT",
	})
	assert.Equal(t, http.StatusBadRequest, getRec.Code)

	// Get with correct scope should succeed.
	getRec2 := doWafv2Request(t, h, "GetWebACL", map[string]any{
		"Id":    id,
		"Scope": "REGIONAL",
	})
	assert.Equal(t, http.StatusOK, getRec2.Code)
}

// ---- Gap 23: APIKey base64 encoding ----------------------------------------

func TestErrorHeaderXAmznErrortype(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Trigger a not-found error.
	rec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": "nonexistent"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "WAFNonexistentItemException", rec.Header().Get("X-Amzn-Errortype"))

	// Trigger an invalid-parameter error.
	rec2 := doWafv2Request(t, h, "CreateWebACL", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
	assert.Equal(t, "WAFInvalidParameterException", rec2.Header().Get("X-Amzn-Errortype"))
}

// ---- Gap 15: GetSampledRequests validation ----------------------------------

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	digits := make([]byte, 0, 3)

	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	return string(digits)
}

func TestProvider_Init_NilCtx(t *testing.T) {
	t.Parallel()

	p := &wafv2.Provider{}
	_, err := p.Init(nil)
	require.ErrorIs(t, err, wafv2.ErrNilAppContext)
}

func TestHandler_Reset(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create a WebACL.
	doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":  "test-acl",
		"Scope": "REGIONAL",
	})

	// Verify it exists.
	rec := doWafv2Request(t, h, "ListWebACLs", map[string]any{"Scope": "REGIONAL"})
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items, _ := resp["WebACLs"].([]any)
	assert.Len(t, items, 1)

	// Reset.
	h.Reset()

	// Verify it's gone.
	rec = doWafv2Request(t, h, "ListWebACLs", map[string]any{"Scope": "REGIONAL"})
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items, _ = resp["WebACLs"].([]any)
	assert.Empty(t, items)
}

func TestBackend_AccountID(t *testing.T) {
	t.Parallel()

	b := wafv2.NewInMemoryBackend("123456789012", "us-west-2")
	assert.Equal(t, "123456789012", b.AccountID())
}

func TestHandler_Reset_Edges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(h *wafv2.Handler)
		verify func(t *testing.T, b *wafv2.InMemoryBackend)
		name   string
	}{
		{
			name:  "reset_empty_backend",
			setup: func(_ *wafv2.Handler) {},
			verify: func(t *testing.T, b *wafv2.InMemoryBackend) {
				t.Helper()
				assert.Equal(t, 0, wafv2.WebACLCount(b))
				assert.Equal(t, 0, wafv2.IPSetCount(b))
				assert.Equal(t, 0, wafv2.RegexPatternSetCount(b))
				assert.Equal(t, 0, wafv2.RuleGroupCount(b))
				assert.Equal(t, 0, wafv2.APIKeyCount(b))
			},
		},
		{
			name: "reset_after_partial_creation",
			setup: func(h *wafv2.Handler) {
				doWafv2Request(t, h, "CreateWebACL", map[string]any{"Name": "a", "Scope": "REGIONAL"})
				doWafv2Request(
					t,
					h,
					"CreateIPSet",
					map[string]any{"Name": "b", "Scope": "REGIONAL", "IPAddressVersion": "IPV4"},
				)
				doWafv2Request(t, h, "CreateRegexPatternSet", map[string]any{"Name": "c", "Scope": "REGIONAL"})
			},
			verify: func(t *testing.T, b *wafv2.InMemoryBackend) {
				t.Helper()
				assert.Equal(t, 0, wafv2.WebACLCount(b))
				assert.Equal(t, 0, wafv2.IPSetCount(b))
				assert.Equal(t, 0, wafv2.RegexPatternSetCount(b))
				assert.Equal(t, 0, wafv2.AssociationCount(b))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := wafv2.NewInMemoryBackend("000000000000", "us-east-1")
			h := wafv2.NewHandler(b)
			tt.setup(h)
			h.Reset()
			tt.verify(t, b)
		})
	}
}

func TestValidation_ResourceNamePattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		aclName    string
		wantStatus int
	}{
		{name: "valid_simple", aclName: "my-acl", wantStatus: http.StatusOK},
		{name: "valid_with_underscores", aclName: "My_ACL_01", wantStatus: http.StatusOK},
		{name: "valid_alphanumeric", aclName: "MyACL123", wantStatus: http.StatusOK},
		{name: "invalid_starts_with_dash", aclName: "-invalid", wantStatus: http.StatusBadRequest},
		{name: "invalid_has_space", aclName: "my acl", wantStatus: http.StatusBadRequest},
		{name: "invalid_has_dot", aclName: "my.acl", wantStatus: http.StatusBadRequest},
		{name: "invalid_has_slash", aclName: "my/acl", wantStatus: http.StatusBadRequest},
		{name: "invalid_starts_with_underscore", aclName: "_myacl", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
				"Name":          tt.aclName,
				"Scope":         "REGIONAL",
				"DefaultAction": map[string]any{"Allow": map[string]any{}},
			})
			assert.Equal(t, tt.wantStatus, rec.Code, "aclName=%q body=%s", tt.aclName, rec.Body.String())
		})
	}
}

// ---- Description max-length validation ---------------------------------------

func TestValidation_DescriptionMaxLength(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Exactly 256 characters — should succeed.
	desc256 := strings.Repeat("a", 256)
	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-desc-ok",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"Description":   desc256,
	})
	assert.Equal(t, http.StatusOK, rec.Code, "256-char description should be accepted: %s", rec.Body.String())

	// 257 characters — should fail.
	desc257 := strings.Repeat("a", 257)
	rec = doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-desc-too-long",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"Description":   desc257,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "257-char description should be rejected: %s", rec.Body.String())
}

// ---- Rule Name uniqueness enforcement ----------------------------------------

func TestValidation_ScopeMismatch(t *testing.T) {
	t.Parallel()

	t.Run("WebACL_regional_get_with_cloudfront_scope", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		// Create a REGIONAL WebACL, then attempt to retrieve it as CLOUDFRONT.
		id, _ := createWebACLHelper(t, h, "regional-acl", "REGIONAL")

		rec := doWafv2Request(t, h, "GetWebACL", map[string]any{
			"Id":    id,
			"Scope": "CLOUDFRONT",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code, "scope mismatch should return 400")

		var errResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
		assert.Equal(t, "WAFNonexistentItemException", errResp["__type"])
	})

	t.Run("WebACL_cloudfront_scope_create_and_get", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		// Create a CLOUDFRONT WebACL — uses the scope param with a non-REGIONAL value.
		id, _ := createWebACLHelper(t, h, "cf-acl", "CLOUDFRONT")
		require.NotEmpty(t, id)

		// Retrieve with matching scope — should succeed.
		rec := doWafv2Request(t, h, "GetWebACL", map[string]any{
			"Id":    id,
			"Scope": "CLOUDFRONT",
		})
		assert.Equal(t, http.StatusOK, rec.Code, "CLOUDFRONT WebACL should be retrievable with CLOUDFRONT scope")

		// Retrieve with wrong scope — should fail.
		rec = doWafv2Request(t, h, "GetWebACL", map[string]any{
			"Id":    id,
			"Scope": "REGIONAL",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code, "CLOUDFRONT WebACL with REGIONAL scope should fail")
	})

	t.Run("IPSet_scope_mismatch", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		id, _ := createIPSetHelper2(t, h, "my-ipset", nil)

		rec := doWafv2Request(t, h, "GetIPSet", map[string]any{
			"Id":    id,
			"Scope": "CLOUDFRONT",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code, "scope mismatch should return 400")
	})
}

// ---- DisassociateWebACL idempotency -----------------------------------------

// TestParity_DispatchTableBuiltOnce verifies the dispatch table is populated at
// construction time so no allocations happen per-request.
func TestDispatchTableBuiltOnce(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	dispatchLen := wafv2.HandlerDispatchOpsLen(h)
	supportedLen := wafv2.HandlerOpsLen(h)

	assert.Equal(t, supportedLen, dispatchLen, "ops dispatch table should cover all supported operations")
	assert.Positive(t, dispatchLen, "dispatch table should not be empty")
}

// TestParity_RegionCap verifies the region cap constant is exported with the expected value.
func TestRegionCap(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 100, wafv2.MaxRegions, "region cap must be 100")
	assert.Positive(t, wafv2.MaxRegions, "MaxRegions must be positive")
}
