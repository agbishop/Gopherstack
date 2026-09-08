package wafv2_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/wafv2"
)

func TestHandler_CreateIPSet(t *testing.T) {
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
				"Name":             "my-ipset",
				"Scope":            "REGIONAL",
				"IPAddressVersion": "IPV4",
				"Addresses":        []string{"1.2.3.4/32"},
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name: "missing_name",
			body: map[string]any{
				"Scope":            "REGIONAL",
				"IPAddressVersion": "IPV4",
				"Addresses":        []string{},
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doWafv2Request(t, h, "CreateIPSet", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantID {
				var result map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
				summary, ok := result["Summary"].(map[string]any)
				require.True(t, ok)
				assert.NotEmpty(t, summary["Id"])
			}
		})
	}
}

func TestHandler_GetIPSet(t *testing.T) {
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
				s, _ := h.Backend.CreateIPSet(context.Background(), "my-ipset", "REGIONAL", "", "IPV4", nil, nil)

				return s.ID
			},
			body: func(id string) map[string]any {
				return map[string]any{"Id": id, "Name": "my-ipset", "Scope": "REGIONAL"}
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
			rec := doWafv2Request(t, h, "GetIPSet", tt.body(id))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_UpdateIPSet(t *testing.T) {
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
				s, _ := h.Backend.CreateIPSet(context.Background(), "my-ipset", "REGIONAL", "", "IPV4", nil, nil)

				return s.ID, s.LockToken
			},
			body: func(id, lockToken string) map[string]any {
				return map[string]any{
					"Id":        id,
					"Name":      "my-ipset",
					"Scope":     "REGIONAL",
					"LockToken": lockToken,
					"Addresses": []string{"10.0.0.0/8"},
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
				return map[string]any{
					"Id":        id,
					"Name":      "x",
					"Scope":     "REGIONAL",
					"LockToken": lockToken,
					"Addresses": []string{},
				}
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id, lockToken := tt.setup(h)
			rec := doWafv2Request(t, h, "UpdateIPSet", tt.body(id, lockToken))
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var result map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
				assert.NotEmpty(t, result["NextLockToken"])
			}
		})
	}
}

func TestHandler_DeleteIPSet(t *testing.T) {
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
				s, _ := h.Backend.CreateIPSet(context.Background(), "my-ipset", "REGIONAL", "", "IPV4", nil, nil)

				return s.ID, s.LockToken
			},
			body: func(id, lockToken string) map[string]any {
				return map[string]any{"Id": id, "Name": "my-ipset", "Scope": "REGIONAL", "LockToken": lockToken}
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
			rec := doWafv2Request(t, h, "DeleteIPSet", tt.body(id, lockToken))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_ListIPSets(t *testing.T) {
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
				_, _ = h.Backend.CreateIPSet(context.Background(), "set1", "REGIONAL", "", "IPV4", nil, nil)
				_, _ = h.Backend.CreateIPSet(context.Background(), "set2", "REGIONAL", "", "IPV6", nil, nil)
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.setup(h)

			rec := doWafv2Request(t, h, "ListIPSets", map[string]any{"Scope": "REGIONAL"})
			assert.Equal(t, http.StatusOK, rec.Code)

			var result map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))

			list, ok := result["IPSets"].([]any)
			require.True(t, ok)
			assert.Len(t, list, tt.wantCount)
		})
	}
}

func TestHandler_CreateIPSet_Duplicate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateIPSet", map[string]any{
		"Name":             "my-set",
		"Scope":            "REGIONAL",
		"IPAddressVersion": "IPV4",
		"Addresses":        []string{},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doWafv2Request(t, h, "CreateIPSet", map[string]any{
		"Name":             "my-set",
		"Scope":            "REGIONAL",
		"IPAddressVersion": "IPV4",
		"Addresses":        []string{},
	})
	assert.Equal(t, http.StatusBadRequest, rec2.Code)

	var result map[string]string
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &result))
	assert.Equal(t, "WAFDuplicateItemException", result["__type"])
}

func TestHandler_GetIPSet_MissingID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(t, h, "GetIPSet", map[string]any{"Name": "my-set", "Scope": "REGIONAL"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_UpdateIPSet_MissingID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(
		t,
		h,
		"UpdateIPSet",
		map[string]any{"Name": "my-set", "Scope": "REGIONAL", "LockToken": "tok"},
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_DeleteIPSet_MissingID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(
		t,
		h,
		"DeleteIPSet",
		map[string]any{"Name": "my-set", "Scope": "REGIONAL", "LockToken": "tok"},
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_ListIPSets_Scope_Filter(t *testing.T) {
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
				_, _ = h.Backend.CreateIPSet(context.Background(), "regional-set", "REGIONAL", "", "IPV4", nil, nil)
				_, _ = h.Backend.CreateIPSet(context.Background(), "cf-set", "CLOUDFRONT", "", "IPV4", nil, nil)
			},
			scope:     "CLOUDFRONT",
			wantCount: 1,
		},
		{
			name: "no_filter_returns_all",
			setup: func(h *wafv2.Handler) {
				_, _ = h.Backend.CreateIPSet(context.Background(), "regional-set", "REGIONAL", "", "IPV4", nil, nil)
				_, _ = h.Backend.CreateIPSet(context.Background(), "cf-set", "CLOUDFRONT", "", "IPV4", nil, nil)
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

			rec := doWafv2Request(t, h, "ListIPSets", body)
			assert.Equal(t, http.StatusOK, rec.Code)

			var result map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			list, ok := result["IPSets"].([]any)
			require.True(t, ok)
			assert.Len(t, list, tt.wantCount)
		})
	}
}

func TestBackend_IPSetARN(t *testing.T) {
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
			arnStr := b.IPSetARN("my-set", "myid", tt.scope)
			assert.Contains(t, arnStr, tt.wantPart)
			assert.Contains(t, arnStr, "ipset")
		})
	}
}

func TestHandler_CreateIPSet_MissingScope(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doWafv2Request(
		t,
		h,
		"CreateIPSet",
		map[string]any{"Name": "my-set", "IPAddressVersion": "IPV4", "Addresses": []string{}},
	)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func createIPSetHelper2(
	t *testing.T,
	h *wafv2.Handler,
	name string,
	addrs []string,
) (string, string) {
	t.Helper()

	body := map[string]any{
		"Name":             name,
		"Scope":            "REGIONAL",
		"IPAddressVersion": "IPV4",
		"Addresses":        addrs,
	}

	rec := doWafv2Request(t, h, "CreateIPSet", body)
	require.Equal(t, http.StatusOK, rec.Code, "createIPSetHelper2: %s", rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	summary := resp["Summary"].(map[string]any)

	return summary["Id"].(string), summary["LockToken"].(string)
}

// ---- Gap 1: WebACL Rules round-trip ----------------------------------------

func TestIPSetCIDRValidation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Invalid CIDR.
	recBad := doWafv2Request(t, h, "CreateIPSet", map[string]any{
		"Name":             "bad-cidrs",
		"Scope":            "REGIONAL",
		"IPAddressVersion": "IPV4",
		"Addresses":        []string{"not-a-cidr"},
	})
	assert.Equal(t, http.StatusBadRequest, recBad.Code)

	// IPv4 CIDR in IPv6 set.
	recMix := doWafv2Request(t, h, "CreateIPSet", map[string]any{
		"Name":             "ipv4-in-ipv6",
		"Scope":            "REGIONAL",
		"IPAddressVersion": "IPV6",
		"Addresses":        []string{"1.2.3.4/32"},
	})
	assert.Equal(t, http.StatusBadRequest, recMix.Code)

	// Valid IPv4.
	recOK := doWafv2Request(t, h, "CreateIPSet", map[string]any{
		"Name":             "valid-v4",
		"Scope":            "REGIONAL",
		"IPAddressVersion": "IPV4",
		"Addresses":        []string{"10.0.0.0/8", "192.168.1.0/24"},
	})
	assert.Equal(t, http.StatusOK, recOK.Code)

	// Valid IPv6.
	recV6 := doWafv2Request(t, h, "CreateIPSet", map[string]any{
		"Name":             "valid-v6",
		"Scope":            "REGIONAL",
		"IPAddressVersion": "IPV6",
		"Addresses":        []string{"2001:db8::/32"},
	})
	assert.Equal(t, http.StatusOK, recV6.Code)
}

// ---- Gap 10/11: RegexPatternSet object shape --------------------------------

func TestIPSetListPagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 5 {
		rec := doWafv2Request(t, h, "CreateIPSet", map[string]any{
			"Name":             fmt.Sprintf("ipset-%02d", i),
			"Scope":            "REGIONAL",
			"IPAddressVersion": "IPV4",
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	page1Rec := doWafv2Request(t, h, "ListIPSets", map[string]any{
		"Scope": "REGIONAL",
		"Limit": 2,
	})
	require.Equal(t, http.StatusOK, page1Rec.Code)

	var page1Resp map[string]any
	require.NoError(t, json.Unmarshal(page1Rec.Body.Bytes(), &page1Resp))

	sets, _ := page1Resp["IPSets"].([]any)
	assert.Len(t, sets, 2)

	nextMarker, _ := page1Resp["NextMarker"].(string)
	assert.NotEmpty(t, nextMarker)
}

// ---- RuleGroup scope validation --------------------------------------------

func TestIPSetUpdateCIDRValidation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id, lockToken := createIPSetHelper2(t, h, "update-set", nil)

	// Update with invalid CIDR.
	recBad := doWafv2Request(t, h, "UpdateIPSet", map[string]any{
		"Id":        id,
		"Name":      "update-set",
		"Scope":     "REGIONAL",
		"LockToken": lockToken,
		"Addresses": []string{"not-valid"},
	})
	assert.Equal(t, http.StatusBadRequest, recBad.Code)

	// Update with wrong IP version.
	recMix := doWafv2Request(t, h, "UpdateIPSet", map[string]any{
		"Id":        id,
		"Name":      "update-set",
		"Scope":     "REGIONAL",
		"LockToken": lockToken,
		"Addresses": []string{"2001:db8::/32"}, // IPv6 in IPv4 set
	})
	assert.Equal(t, http.StatusBadRequest, recMix.Code)

	// Update with valid CIDR.
	recOK := doWafv2Request(t, h, "UpdateIPSet", map[string]any{
		"Id":        id,
		"Name":      "update-set",
		"Scope":     "REGIONAL",
		"LockToken": lockToken,
		"Addresses": []string{"10.0.0.0/8"},
	})
	assert.Equal(t, http.StatusOK, recOK.Code)
}

func TestIPSet_MaxEntriesRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Build 10001 unique /32 CIDRs (more than the 10000 limit).
	addrs := make([]string, 10001)
	for i := range addrs {
		addrs[i] = "10." +
			string(rune('0'+i/1000000%10)) +
			"." + string(rune('0'+i/1000%10)) +
			"." + string(rune('0'+i%1000/10)) + "/32"
	}

	// Reset to proper CIDR format.
	addrs = make([]string, 10001)
	for i := range addrs {
		a := i / (256 * 256)
		b := (i / 256) % 256
		c := i % 256
		addrs[i] = "10." + itoa(a) + "." + itoa(b) + "." + itoa(c) + "/32"
	}

	rec := doWafv2Request(t, h, "CreateIPSet", map[string]any{
		"Name":             "too-many-ips",
		"Scope":            "REGIONAL",
		"IPAddressVersion": "IPV4",
		"Addresses":        addrs,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "10001 IPs should exceed limit")

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "WAFLimitsExceededException", errResp["__type"])
}

func TestIPSet_NearMaxEntries(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Exactly 10000 entries should be accepted.
	addrs := make([]string, 10000)
	for i := range addrs {
		a := i / (256 * 256)
		b := (i / 256) % 256
		c := i % 256
		addrs[i] = "10." + itoa(a) + "." + itoa(b) + "." + itoa(c) + "/32"
	}

	rec := doWafv2Request(t, h, "CreateIPSet", map[string]any{
		"Name":             "max-ips",
		"Scope":            "REGIONAL",
		"IPAddressVersion": "IPV4",
		"Addresses":        addrs,
	})
	assert.Equal(t, http.StatusOK, rec.Code, "exactly 10000 IPs should be accepted: %s", rec.Body.String())
}

// ---- Regex pattern set: max entries enforcement -----------------------------

func TestHandler_ScopeValidation_CreateIPSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		scope            string
		ipAddressVersion string
		wantStatus       int
	}{
		{name: "regional_ipv4", scope: "REGIONAL", ipAddressVersion: "IPV4", wantStatus: http.StatusOK},
		{name: "cloudfront_ipv6", scope: "CLOUDFRONT", ipAddressVersion: "IPV6", wantStatus: http.StatusOK},
		{name: "invalid_scope", scope: "BAD", ipAddressVersion: "IPV4", wantStatus: http.StatusBadRequest},
		{
			name:             "invalid_ip_version",
			scope:            "REGIONAL",
			ipAddressVersion: "BADVERSION",
			wantStatus:       http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doWafv2Request(t, h, "CreateIPSet", map[string]any{
				"Name":             "test-ipset",
				"Scope":            tt.scope,
				"IPAddressVersion": tt.ipAddressVersion,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestUpdateIPSet_ClearAddresses(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id, lockToken := createIPSetHelper2(t, h, "my-ipset", []string{"10.0.0.0/8"})

	// Update with empty addresses list should clear addresses (not be ignored).
	rec := doWafv2Request(t, h, "UpdateIPSet", map[string]any{
		"Id":        id,
		"Name":      "my-ipset",
		"Scope":     "REGIONAL",
		"LockToken": lockToken,
		"Addresses": []string{},
	})
	require.Equal(t, http.StatusOK, rec.Code, "UpdateIPSet with empty addresses: %s", rec.Body.String())

	var updateResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
	newLockToken := updateResp["NextLockToken"].(string)

	// Verify addresses were cleared.
	rec = doWafv2Request(t, h, "GetIPSet", map[string]any{"Id": id, "Name": "my-ipset", "Scope": "REGIONAL"})
	require.Equal(t, http.StatusOK, rec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
	ipSet := getResp["IPSet"].(map[string]any)
	addresses, _ := ipSet["Addresses"].([]any)
	assert.Empty(t, addresses, "addresses should be empty after clearing")

	_ = newLockToken
}

// ---- AssociateWebACL is idempotent (second call replaces) --------------------

func TestValidation_IPSetNamePattern(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doWafv2Request(t, h, "CreateIPSet", map[string]any{
		"Name":             "my.invalid.ipset",
		"Scope":            "REGIONAL",
		"IPAddressVersion": "IPV4",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "IPSet name with dots should be rejected")
}

// ---- RuleGroup name pattern validation --------------------------------------
