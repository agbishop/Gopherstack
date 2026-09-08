package wafv2_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/wafv2"
)

func TestHandler_CreateWebACL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
		wantID     bool
	}{
		{
			name: "valid",
			body: map[string]any{
				"Name":          "my-web-acl",
				"Scope":         "REGIONAL",
				"DefaultAction": map[string]any{"Allow": map[string]any{}},
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name: "missing_name",
			body: map[string]any{
				"Scope": "REGIONAL",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_scope",
			body: map[string]any{
				"Name": "my-web-acl",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doWafv2Request(t, h, "CreateWebACL", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantID {
				var result map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
				summary, ok := result["Summary"].(map[string]any)
				require.True(t, ok)
				assert.NotEmpty(t, summary["Id"])
				assert.NotEmpty(t, summary["LockToken"])
			}
		})
	}
}

func TestHandler_GetWebACL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*wafv2.Handler) string
		body       func(id string) map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "existing",
			setup: func(h *wafv2.Handler) string {
				w, _ := wafv2.CreateWebACLSimple(h.Backend, "my-acl", "REGIONAL", "", "ALLOW", nil)

				return w.ID
			},
			body: func(id string) map[string]any {
				return map[string]any{"Id": id, "Name": "my-acl", "Scope": "REGIONAL"}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "not_found",
			setup: func(_ *wafv2.Handler) string {
				return "nonexistent"
			},
			body: func(id string) map[string]any {
				return map[string]any{"Id": id, "Name": "x", "Scope": "REGIONAL"}
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id := tt.setup(h)
			rec := doWafv2Request(t, h, "GetWebACL", tt.body(id))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_UpdateWebACL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*wafv2.Handler) (string, string)
		body       func(id, lockToken string) map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "existing",
			setup: func(h *wafv2.Handler) (string, string) {
				w, _ := wafv2.CreateWebACLSimple(h.Backend, "my-acl", "REGIONAL", "", "ALLOW", nil)

				return w.ID, w.LockToken
			},
			body: func(id, lockToken string) map[string]any {
				return map[string]any{
					"Id":          id,
					"Name":        "my-acl",
					"Scope":       "REGIONAL",
					"LockToken":   lockToken,
					"Description": "updated",
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "not_found",
			setup: func(_ *wafv2.Handler) (string, string) {
				return "nonexistent", "tok"
			},
			body: func(id, lockToken string) map[string]any {
				return map[string]any{"Id": id, "Name": "x", "Scope": "REGIONAL", "LockToken": lockToken}
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id, lockToken := tt.setup(h)
			rec := doWafv2Request(t, h, "UpdateWebACL", tt.body(id, lockToken))
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var result map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
				assert.NotEmpty(t, result["NextLockToken"])
			}
		})
	}
}

func TestHandler_DeleteWebACL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*wafv2.Handler) (string, string)
		body       func(id, lockToken string) map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "existing",
			setup: func(h *wafv2.Handler) (string, string) {
				w, _ := wafv2.CreateWebACLSimple(h.Backend, "my-acl", "REGIONAL", "", "ALLOW", nil)

				return w.ID, w.LockToken
			},
			body: func(id, lockToken string) map[string]any {
				return map[string]any{"Id": id, "Name": "my-acl", "Scope": "REGIONAL", "LockToken": lockToken}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "not_found",
			setup: func(_ *wafv2.Handler) (string, string) {
				return "nonexistent", "tok"
			},
			body: func(id, lockToken string) map[string]any {
				return map[string]any{"Id": id, "Name": "x", "Scope": "REGIONAL", "LockToken": lockToken}
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id, lockToken := tt.setup(h)
			rec := doWafv2Request(t, h, "DeleteWebACL", tt.body(id, lockToken))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_ListWebACLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(*wafv2.Handler)
		name      string
		wantCount int
	}{
		{
			name:      "empty",
			setup:     func(_ *wafv2.Handler) {},
			wantCount: 0,
		},
		{
			name: "with_items",
			setup: func(h *wafv2.Handler) {
				_, _ = wafv2.CreateWebACLSimple(h.Backend, "acl1", "REGIONAL", "", "ALLOW", nil)
				_, _ = wafv2.CreateWebACLSimple(h.Backend, "acl2", "REGIONAL", "", "BLOCK", nil)
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.setup(h)

			rec := doWafv2Request(t, h, "ListWebACLs", map[string]any{"Scope": "REGIONAL"})
			assert.Equal(t, http.StatusOK, rec.Code)

			var result map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))

			list, ok := result["WebACLs"].([]any)
			require.True(t, ok)
			assert.Len(t, list, tt.wantCount)
		})
	}
}

func TestHandler_CreateWebACL_Duplicate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":  "my-acl",
		"Scope": "REGIONAL",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":  "my-acl",
		"Scope": "REGIONAL",
	})
	assert.Equal(t, http.StatusBadRequest, rec2.Code)

	var result map[string]string
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &result))
	assert.Equal(t, "WAFDuplicateItemException", result["__type"])
}

func TestHandler_GetWebACL_MissingID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Name": "my-acl", "Scope": "REGIONAL"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var result map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, "WAFInvalidParameterException", result["__type"])
}

func TestHandler_UpdateWebACL_MissingID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(
		t,
		h,
		"UpdateWebACL",
		map[string]any{"Name": "my-acl", "Scope": "REGIONAL", "LockToken": "tok"},
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_DeleteWebACL_MissingID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(
		t,
		h,
		"DeleteWebACL",
		map[string]any{"Name": "my-acl", "Scope": "REGIONAL", "LockToken": "tok"},
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_ListWebACLs_Scope_Filter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(*wafv2.Handler)
		name      string
		scope     string
		wantCount int
	}{
		{
			name: "filter_cloudfront",
			setup: func(h *wafv2.Handler) {
				_, _ = wafv2.CreateWebACLSimple(h.Backend, "regional-acl", "REGIONAL", "", "ALLOW", nil)
				_, _ = wafv2.CreateWebACLSimple(h.Backend, "cf-acl", "CLOUDFRONT", "", "ALLOW", nil)
			},
			scope:     "CLOUDFRONT",
			wantCount: 1,
		},
		{
			name: "no_filter_returns_all",
			setup: func(h *wafv2.Handler) {
				_, _ = wafv2.CreateWebACLSimple(h.Backend, "regional-acl", "REGIONAL", "", "ALLOW", nil)
				_, _ = wafv2.CreateWebACLSimple(h.Backend, "cf-acl", "CLOUDFRONT", "", "ALLOW", nil)
			},
			scope:     "",
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.setup(h)

			body := map[string]any{}
			if tt.scope != "" {
				body["Scope"] = tt.scope
			}

			rec := doWafv2Request(t, h, "ListWebACLs", body)
			assert.Equal(t, http.StatusOK, rec.Code)

			var result map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			list, ok := result["WebACLs"].([]any)
			require.True(t, ok)
			assert.Len(t, list, tt.wantCount)
		})
	}
}

func TestBackend_WebACLARN(t *testing.T) {
	t.Parallel()

	b := wafv2.NewInMemoryBackend("123456789012", "us-east-1")
	tests := []struct {
		name     string
		wantPart string
		scope    string
	}{
		{name: "regional_scope", scope: "REGIONAL", wantPart: "regional"},
		{name: "cloudfront_scope", scope: "CLOUDFRONT", wantPart: "global"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			arnStr := b.WebACLARN("my-acl", "myid", tt.scope)
			assert.Contains(t, arnStr, tt.wantPart)
			assert.Contains(t, arnStr, "webacl")
		})
	}
}

func TestHandler_GetWebACL_BlockDefaultAction(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	w, err := wafv2.CreateWebACLSimple(h.Backend, "blocked-acl", "REGIONAL", "", "BLOCK", nil)
	require.NoError(t, err)

	rec := doWafv2Request(t, h, "GetWebACL", map[string]any{
		"Id":    w.ID,
		"Name":  w.Name,
		"Scope": w.Scope,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	webACL, ok := result["WebACL"].(map[string]any)
	require.True(t, ok)
	defaultAction, ok := webACL["DefaultAction"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, defaultAction, "Block")
}

func TestHandler_CreateWebACL_MissingScope(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{"Name": "my-acl"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_GetWebACL_WithVisibilityConfig(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create WebACL via HTTP API to include VisibilityConfig.
	createRec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "my-acl",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"CloudWatchMetricsEnabled": true,
			"MetricName":               "my-acl",
			"SampledRequestsEnabled":   true,
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResult map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResult))
	summary := createResult["Summary"].(map[string]any)
	id := summary["Id"].(string)

	rec := doWafv2Request(t, h, "GetWebACL", map[string]any{
		"Id":    id,
		"Name":  "my-acl",
		"Scope": "REGIONAL",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	webACL, ok := result["WebACL"].(map[string]any)
	require.True(t, ok)
	vis, ok := webACL["VisibilityConfig"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, vis["CloudWatchMetricsEnabled"])
}

//nolint:unparam // scope kept for call-site readability; callers may vary it in future
func createWebACLWithRules(t *testing.T, h *wafv2.Handler, name, scope string) (string, string) {
	t.Helper()

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          name,
		"Scope":         scope,
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"CloudWatchMetricsEnabled": true,
			"MetricName":               name,
			"SampledRequestsEnabled":   true,
		},
		"Rules": []map[string]any{
			{
				"Name":     "rule-1",
				"Priority": 1,
				"Statement": map[string]any{
					"IPSetReferenceStatement": map[string]any{
						"ARN": "arn:aws:wafv2:us-east-1:000000000000:regional/ipset/test/abc",
					},
				},
				"Action": map[string]any{"Allow": map[string]any{}},
				"VisibilityConfig": map[string]any{
					"CloudWatchMetricsEnabled": false,
					"MetricName":               "rule-1",
					"SampledRequestsEnabled":   false,
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "createWebACLWithRules: %s", rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	summary := resp["Summary"].(map[string]any)

	return summary["Id"].(string), summary["LockToken"].(string)
}

func TestWebACLRulesRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id, _ := createWebACLWithRules(t, h, "acl-with-rules", "REGIONAL")

	rec := doWafv2Request(t, h, "GetWebACL", map[string]any{
		"Id":    id,
		"Name":  "acl-with-rules",
		"Scope": "REGIONAL",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	webACL := resp["WebACL"].(map[string]any)

	rules, ok := webACL["Rules"].([]any)
	require.True(t, ok, "Rules should be present in response")
	require.Len(t, rules, 1, "should have exactly 1 rule")

	rule := rules[0].(map[string]any)
	assert.Equal(t, "rule-1", rule["Name"])
	assert.InDelta(t, float64(1), rule["Priority"], 0)
}

// ---- Gap 2: DefaultAction as raw JSON round-trip ----------------------------

func TestDefaultActionRawJSONRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create with Allow.
	recAllow := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-allow",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-allow",
		},
	})
	require.Equal(t, http.StatusOK, recAllow.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(recAllow.Body.Bytes(), &createResp))
	idAllow := createResp["Summary"].(map[string]any)["Id"].(string)

	recGet := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": idAllow})
	require.Equal(t, http.StatusOK, recGet.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(recGet.Body.Bytes(), &getResp))
	da := getResp["WebACL"].(map[string]any)["DefaultAction"].(map[string]any)
	_, hasAllow := da["Allow"]
	assert.True(t, hasAllow, "DefaultAction should contain Allow key")

	// Create with Block.
	recBlock := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-block",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Block": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-block",
		},
	})
	require.Equal(t, http.StatusOK, recBlock.Code)

	var createRespBlock map[string]any
	require.NoError(t, json.Unmarshal(recBlock.Body.Bytes(), &createRespBlock))
	idBlock := createRespBlock["Summary"].(map[string]any)["Id"].(string)

	recGetBlock := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": idBlock})
	require.Equal(t, http.StatusOK, recGetBlock.Code)

	var getRespBlock map[string]any
	require.NoError(t, json.Unmarshal(recGetBlock.Body.Bytes(), &getRespBlock))
	daBlock := getRespBlock["WebACL"].(map[string]any)["DefaultAction"].(map[string]any)
	_, hasBlock := daBlock["Block"]
	assert.True(t, hasBlock, "DefaultAction should contain Block key")

	// Both Allow AND Block together should be rejected.
	recBoth := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":  "acl-both",
		"Scope": "REGIONAL",
		"DefaultAction": map[string]any{
			"Allow": map[string]any{},
			"Block": map[string]any{},
		},
	})
	assert.Equal(t, http.StatusBadRequest, recBoth.Code)
}

// ---- Gap 3: LockToken enforcement -------------------------------------------

func TestWebACLExtendedFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":             "extended-acl",
		"Scope":            "REGIONAL",
		"DefaultAction":    map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{"MetricName": "extended-acl"},
		"TokenDomains":     []string{"example.com", "api.example.com"},
		"CaptchaConfig": map[string]any{
			"ImmunityTimeProperty": map[string]any{"ImmunityTime": 300},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	getRec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	webACL := getResp["WebACL"].(map[string]any)

	// TokenDomains should be round-tripped.
	domains, ok := webACL["TokenDomains"].([]any)
	require.True(t, ok, "TokenDomains should be in response")
	assert.Len(t, domains, 2)

	// CaptchaConfig should be round-tripped.
	cc, ok := webACL["CaptchaConfig"].(map[string]any)
	require.True(t, ok, "CaptchaConfig should be in response")
	assert.Contains(t, cc, "ImmunityTimeProperty")
}

// ---- IPSet pagination -------------------------------------------------------

func TestCaptchaAction_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-captcha",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-captcha",
		},
		"Rules": []map[string]any{
			{
				"Name":     "captcha-rule",
				"Priority": 1,
				"Statement": map[string]any{
					"GeoMatchStatement": map[string]any{
						"CountryCodes": []string{"RU"},
					},
				},
				"Action": map[string]any{
					"Captcha": map[string]any{
						"CustomRequestHandling": map[string]any{
							"InsertHeaders": []map[string]any{
								{"Name": "x-captcha", "Value": "required"},
							},
						},
					},
				},
				"VisibilityConfig": map[string]any{"MetricName": "captcha-rule"},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "Captcha action: %s", rec.Body.String())

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	// Verify round-trip.
	getRec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	rules := getResp["WebACL"].(map[string]any)["Rules"].([]any)
	require.Len(t, rules, 1)

	action := rules[0].(map[string]any)["Action"].(map[string]any)
	_, hasCaptcha := action["Captcha"]
	assert.True(t, hasCaptcha, "Captcha action should round-trip")
}

func TestChallengeAction_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-challenge",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-challenge",
		},
		"Rules": []map[string]any{
			{
				"Name":     "challenge-rule",
				"Priority": 1,
				"Statement": map[string]any{
					"GeoMatchStatement": map[string]any{
						"CountryCodes": []string{"CN"},
					},
				},
				"Action": map[string]any{
					"Challenge": map[string]any{},
				},
				"VisibilityConfig": map[string]any{"MetricName": "challenge-rule"},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "Challenge action: %s", rec.Body.String())

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	getRec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	rules := getResp["WebACL"].(map[string]any)["Rules"].([]any)
	require.Len(t, rules, 1)

	action := rules[0].(map[string]any)["Action"].(map[string]any)
	_, hasChallenge := action["Challenge"]
	assert.True(t, hasChallenge, "Challenge action should round-trip")
}

func TestChallengeConfig_WebACLLevel(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-challenge-config",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-challenge-config",
		},
		"ChallengeConfig": map[string]any{
			"ImmunityTimeProperty": map[string]any{
				"ImmunityTime": 600,
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	getRec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	webACL := getResp["WebACL"].(map[string]any)

	challengeCfg, ok := webACL["ChallengeConfig"].(map[string]any)
	require.True(t, ok, "ChallengeConfig should be present in response")
	immunityTime := challengeCfg["ImmunityTimeProperty"].(map[string]any)
	assert.InDelta(t, float64(600), immunityTime["ImmunityTime"], 0)
}

// ---- LabelMatchStatement ----------------------------------------------------

func TestLabelMatchStatement_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-label-match",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-label-match",
		},
		"Rules": []map[string]any{
			{
				"Name":     "label-rule",
				"Priority": 1,
				"Statement": map[string]any{
					"LabelMatchStatement": map[string]any{
						"Scope": "LABEL",
						"Key":   "awswaf:managed:aws:core-rule-set:NoUserAgent_Header",
					},
				},
				"Action":           map[string]any{"Block": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "label-rule"},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "LabelMatchStatement: %s", rec.Body.String())

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	getRec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	rules := getResp["WebACL"].(map[string]any)["Rules"].([]any)
	require.Len(t, rules, 1)

	stmt := rules[0].(map[string]any)["Statement"].(map[string]any)
	lms, ok := stmt["LabelMatchStatement"].(map[string]any)
	require.True(t, ok, "LabelMatchStatement should round-trip")
	assert.Equal(t, "LABEL", lms["Scope"])
	assert.Equal(t, "awswaf:managed:aws:core-rule-set:NoUserAgent_Header", lms["Key"])
}

func TestLabelMatchStatement_NamespaceScope(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-label-ns",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-label-ns",
		},
		"Rules": []map[string]any{
			{
				"Name":     "ns-rule",
				"Priority": 1,
				"Statement": map[string]any{
					"LabelMatchStatement": map[string]any{
						"Scope": "NAMESPACE",
						"Key":   "awswaf:managed:aws:",
					},
				},
				"Action":           map[string]any{"Count": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "ns-rule"},
			},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code, "LabelMatchStatement NAMESPACE scope: %s", rec.Body.String())
}

// ---- AssociationConfig decryption parameters --------------------------------

func TestAssociationConfig_RequestBodyDecryption(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-assoc-config",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-assoc-config",
		},
		"AssociationConfig": map[string]any{
			"RequestBody": map[string]any{
				"APPLICATION_LOAD_BALANCER": map[string]any{
					"DefaultSizeInspectionLimit": "KB_8",
					"OversizeHandling":           "CONTINUE",
				},
				"API_GATEWAY": map[string]any{
					"DefaultSizeInspectionLimit": "KB_64",
					"OversizeHandling":           "MATCH",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "AssociationConfig: %s", rec.Body.String())

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	getRec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	webACL := getResp["WebACL"].(map[string]any)

	assocCfg, ok := webACL["AssociationConfig"].(map[string]any)
	require.True(t, ok, "AssociationConfig should be present in response")

	requestBody, ok := assocCfg["RequestBody"].(map[string]any)
	require.True(t, ok, "RequestBody should be present in AssociationConfig")

	alb, ok := requestBody["APPLICATION_LOAD_BALANCER"].(map[string]any)
	require.True(t, ok, "APPLICATION_LOAD_BALANCER config should be present")
	assert.Equal(t, "KB_8", alb["DefaultSizeInspectionLimit"])
	assert.Equal(t, "CONTINUE", alb["OversizeHandling"])
}

// ---- ManagedRuleGroupStatement in WebACL ------------------------------------

func TestWebACL_OverrideAction_None(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, rgARN := createRuleGroupHelper(t, h, "override-none-rg")

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-override-none",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-override-none",
		},
		"Rules": []map[string]any{
			{
				"Name":     "rg-rule",
				"Priority": 1,
				"Statement": map[string]any{
					"RuleGroupReferenceStatement": map[string]any{
						"ARN": rgARN,
					},
				},
				"OverrideAction": map[string]any{"None": map[string]any{}},
				"VisibilityConfig": map[string]any{
					"MetricName": "rg-rule",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "OverrideAction None: %s", rec.Body.String())
}

func TestWebACL_OverrideAction_Count(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, rgARN := createRuleGroupHelper(t, h, "override-count-rg")

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-override-count",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-override-count",
		},
		"Rules": []map[string]any{
			{
				"Name":     "rg-count-rule",
				"Priority": 1,
				"Statement": map[string]any{
					"RuleGroupReferenceStatement": map[string]any{
						"ARN": rgARN,
					},
				},
				"OverrideAction": map[string]any{"Count": map[string]any{}},
				"VisibilityConfig": map[string]any{
					"MetricName": "rg-count-rule",
				},
			},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code, "OverrideAction Count: %s", rec.Body.String())
}

// ---- IP set limit enforcement -----------------------------------------------

func TestWebACL_CustomResponseBodies_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-custom-resp",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-custom-resp",
		},
		"CustomResponseBodies": map[string]any{
			"BlockPage": map[string]any{
				"ContentType": "TEXT_HTML",
				"Content":     "<html><body>Access Denied</body></html>",
			},
		},
		"Rules": []map[string]any{
			{
				"Name":     "block-rule",
				"Priority": 1,
				"Statement": map[string]any{
					"IPSetReferenceStatement": map[string]any{
						"ARN": "arn:aws:wafv2:us-east-1:000000000000:regional/ipset/x/abc",
					},
				},
				"Action": map[string]any{
					"Block": map[string]any{
						"CustomResponse": map[string]any{
							"ResponseCode":          403,
							"CustomResponseBodyKey": "BlockPage",
						},
					},
				},
				"VisibilityConfig": map[string]any{"MetricName": "block-rule"},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "CustomResponseBodies: %s", rec.Body.String())

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	getRec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	webACL := getResp["WebACL"].(map[string]any)

	crb, ok := webACL["CustomResponseBodies"].(map[string]any)
	require.True(t, ok, "CustomResponseBodies should be present in response")
	assert.Contains(t, crb, "BlockPage")
}

// ---- ListResourcesForWebACL -------------------------------------------------

func TestWebACL_AllActionTypes(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, rgARN := createRuleGroupHelper(t, h, "multi-action-rg")

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "acl-all-actions",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
		"VisibilityConfig": map[string]any{
			"MetricName": "acl-all-actions",
		},
		"Rules": []map[string]any{
			{
				"Name":     "allow-rule",
				"Priority": 1,
				"Statement": map[string]any{
					"IPSetReferenceStatement": map[string]any{
						"ARN": "arn:aws:wafv2:us-east-1:000000000000:regional/ipset/allow/abc",
					},
				},
				"Action":           map[string]any{"Allow": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "allow-rule"},
			},
			{
				"Name":     "block-rule",
				"Priority": 2,
				"Statement": map[string]any{
					"IPSetReferenceStatement": map[string]any{
						"ARN": "arn:aws:wafv2:us-east-1:000000000000:regional/ipset/block/def",
					},
				},
				"Action":           map[string]any{"Block": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "block-rule"},
			},
			{
				"Name":     "count-rule",
				"Priority": 3,
				"Statement": map[string]any{
					"GeoMatchStatement": map[string]any{
						"CountryCodes": []string{"FR"},
					},
				},
				"Action":           map[string]any{"Count": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "count-rule"},
			},
			{
				"Name":     "captcha-rule",
				"Priority": 4,
				"Statement": map[string]any{
					"GeoMatchStatement": map[string]any{
						"CountryCodes": []string{"DE"},
					},
				},
				"Action":           map[string]any{"Captcha": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "captcha-rule"},
			},
			{
				"Name":     "challenge-rule",
				"Priority": 5,
				"Statement": map[string]any{
					"GeoMatchStatement": map[string]any{
						"CountryCodes": []string{"JP"},
					},
				},
				"Action":           map[string]any{"Challenge": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "challenge-rule"},
			},
			{
				"Name":     "rg-override-rule",
				"Priority": 6,
				"Statement": map[string]any{
					"RuleGroupReferenceStatement": map[string]any{"ARN": rgARN},
				},
				"OverrideAction":   map[string]any{"None": map[string]any{}},
				"VisibilityConfig": map[string]any{"MetricName": "rg-override-rule"},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "all action types: %s", rec.Body.String())

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	getRec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	rules := getResp["WebACL"].(map[string]any)["Rules"].([]any)
	assert.Len(t, rules, 6, "all 6 rules should round-trip")
}

// ---- WebACL update clears old rules -----------------------------------------

func TestWebACL_Update_ClearsOldRules(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id, lock := createWebACLWithRules(t, h, "acl-rule-clear", "REGIONAL")

	// Update with empty rules — should clear all rules.
	updateRec := doWafv2Request(t, h, "UpdateWebACL", map[string]any{
		"Id":            id,
		"Name":          "acl-rule-clear",
		"Scope":         "REGIONAL",
		"LockToken":     lock,
		"DefaultAction": map[string]any{"Block": map[string]any{}},
		"Rules":         []map[string]any{},
	})
	require.Equal(t, http.StatusOK, updateRec.Code, "update with empty rules: %s", updateRec.Body.String())

	getRec := doWafv2Request(t, h, "GetWebACL", map[string]any{"Id": id})
	require.Equal(t, http.StatusOK, getRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &resp))
	rules := resp["WebACL"].(map[string]any)["Rules"].([]any)
	assert.Empty(t, rules, "rules should be empty after clearing")
}

// createWebACLHelper creates a web ACL and returns its ID and ARN.
func createWebACLHelper(t *testing.T, h *wafv2.Handler, name, scope string) (string, string) {
	t.Helper()

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":  name,
		"Scope": scope,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	summary, ok := resp["Summary"].(map[string]any)
	require.True(t, ok)

	id, ok := summary["Id"].(string)
	require.True(t, ok)

	arn, ok := summary["ARN"].(string)
	require.True(t, ok)

	return id, arn
}

func TestHandler_ScopeValidation_CreateWebACL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		scope      string
		wantStatus int
	}{
		{name: "regional_valid", scope: "REGIONAL", wantStatus: http.StatusOK},
		{name: "cloudfront_valid", scope: "CLOUDFRONT", wantStatus: http.StatusOK},
		{name: "invalid_scope", scope: "INVALID", wantStatus: http.StatusBadRequest},
		{name: "empty_scope", scope: "", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
				"Name":  "test-acl",
				"Scope": tt.scope,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_DeleteWebACL_CascadeLogging(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create a WebACL.
	_, webACLArn := createWebACLHelper(t, h, "test-acl", "REGIONAL")

	// Put logging config for it.
	rec := doWafv2Request(t, h, "PutLoggingConfiguration", map[string]any{
		"LoggingConfiguration": map[string]any{"ResourceArn": webACLArn},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Delete the WebACL.
	var createResp map[string]any
	rec2 := doWafv2Request(t, h, "ListWebACLs", map[string]any{"Scope": "REGIONAL"})
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &createResp))
	webACLs, _ := createResp["WebACLs"].([]any)
	require.Len(t, webACLs, 1)
	webACL := webACLs[0].(map[string]any)
	webACLID := webACL["Id"].(string)
	webACLLockToken := webACL["LockToken"].(string)

	rec = doWafv2Request(t, h, "DeleteWebACL", map[string]any{
		"Id": webACLID, "Name": "test-acl", "Scope": "REGIONAL", "LockToken": webACLLockToken,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// The logging config should be gone.
	rec = doWafv2Request(t, h, "GetLoggingConfiguration", map[string]any{"ResourceArn": webACLArn})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeleteWebACL_FailsWhenAssociated(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	id, webACLARN := createWebACLHelper(t, h, "protected-acl", "REGIONAL")
	resourceARN := "arn:aws:elasticloadbalancing:us-east-1:000000000000:loadbalancer/app/my-lb/abc"

	rec := doWafv2Request(t, h, "AssociateWebACL", map[string]any{
		"WebACLArn":   webACLARN,
		"ResourceArn": resourceARN,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	w, err := h.Backend.GetWebACL(context.Background(), id)
	require.NoError(t, err)

	// Attempt delete while associated — should fail with WAFAssociatedItemException.
	rec = doWafv2Request(t, h, "DeleteWebACL", map[string]any{
		"Id":        id,
		"Name":      "protected-acl",
		"Scope":     "REGIONAL",
		"LockToken": w.LockToken,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "WAFAssociatedItemException", errResp["__type"])

	// After disassociation, delete should succeed.
	rec = doWafv2Request(t, h, "DisassociateWebACL", map[string]any{"ResourceArn": resourceARN})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doWafv2Request(t, h, "DeleteWebACL", map[string]any{
		"Id":        id,
		"Name":      "protected-acl",
		"Scope":     "REGIONAL",
		"LockToken": w.LockToken,
	})
	assert.Equal(t, http.StatusOK, rec.Code, "delete after disassociation should succeed: %s", rec.Body.String())
}

// ---- UpdateIPSet with empty addresses (clear) --------------------------------

func TestARN_CloudFrontScopeHasEmptyRegion(t *testing.T) {
	t.Parallel()

	b := wafv2.NewInMemoryBackend("123456789012", "us-east-1")

	// CLOUDFRONT ARN should NOT contain the region in the ARN.
	arnStr := b.WebACLARN("my-acl", "abc123", "CLOUDFRONT")
	assert.Contains(t, arnStr, "global/webacl/my-acl/abc123", "CLOUDFRONT ARN resource path")
	// Region segment should be empty: arn:aws:wafv2::123456789012:global/...
	assert.NotContains(t, arnStr, "us-east-1", "CLOUDFRONT ARN must not include region")

	// REGIONAL ARN should include the region.
	arnRegional := b.WebACLARN("my-acl", "abc123", "REGIONAL")
	assert.Contains(t, arnRegional, "us-east-1", "REGIONAL ARN must include region")
	assert.Contains(t, arnRegional, "regional/webacl/my-acl/abc123")
}

// ---- ListLoggingConfigurations returns stored configs -----------------------

func TestCreateWebACL_DuplicateNameScope(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createWebACLHelper(t, h, "dup-acl", "REGIONAL")

	rec := doWafv2Request(t, h, "CreateWebACL", map[string]any{
		"Name":          "dup-acl",
		"Scope":         "REGIONAL",
		"DefaultAction": map[string]any{"Allow": map[string]any{}},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "WAFDuplicateItemException", errResp["__type"])
}

// ---- LockToken changes on each UpdateWebACL ---------------------------------

func TestLockToken_RotatesOnUpdate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	// Use createWebACLWithRules (REGIONAL scope) which returns (id, lockToken).
	id, token1 := createWebACLWithRules(t, h, "lock-rotation", "REGIONAL")

	rec := doWafv2Request(t, h, "UpdateWebACL", map[string]any{
		"Id":          id,
		"Name":        "lock-rotation",
		"Scope":       "REGIONAL",
		"LockToken":   token1,
		"Description": "updated",
	})
	require.Equal(t, http.StatusOK, rec.Code, "update with correct token: %s", rec.Body.String())

	var updateResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
	token2 := updateResp["NextLockToken"].(string)

	assert.NotEmpty(t, token2)
	assert.NotEqual(t, token1, token2, "LockToken must rotate on every successful update")
}

// ---- IPSet name pattern validation ------------------------------------------
