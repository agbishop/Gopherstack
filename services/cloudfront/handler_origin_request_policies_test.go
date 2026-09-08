package cloudfront_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudfront"
)

// TestOriginRequestPolicyConfig tests ORP creation with full config.
func TestOriginRequestPolicyConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantIn   string
		wantCode int
	}{
		{
			name: "create_with_headers_config",
			body: `<OriginRequestPolicyConfig>
				<Name>headers-policy</Name>
				<HeadersConfig>
					<HeaderBehavior>whitelist</HeaderBehavior>
					<Headers>
						<Items><Name>Accept</Name><Name>Accept-Language</Name></Items>
						<Quantity>2</Quantity>
					</Headers>
				</HeadersConfig>
				<CookiesConfig><CookieBehavior>none</CookieBehavior></CookiesConfig>
				<QueryStringsConfig><QueryStringBehavior>none</QueryStringBehavior></QueryStringsConfig>
			</OriginRequestPolicyConfig>`,
			wantCode: http.StatusCreated,
			wantIn:   "headers-policy",
		},
		{
			name: "create_with_cookies_config",
			body: `<OriginRequestPolicyConfig>
				<Name>cookies-policy</Name>
				<HeadersConfig><HeaderBehavior>none</HeaderBehavior></HeadersConfig>
				<CookiesConfig>
					<CookieBehavior>whitelist</CookieBehavior>
					<Cookies>
						<Items><Name>session</Name></Items>
						<Quantity>1</Quantity>
					</Cookies>
				</CookiesConfig>
				<QueryStringsConfig><QueryStringBehavior>all</QueryStringBehavior></QueryStringsConfig>
			</OriginRequestPolicyConfig>`,
			wantCode: http.StatusCreated,
			wantIn:   "cookies-policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newAuditBackend(t)
			h := cloudfront.NewHandler(b)
			rec := doReq(t, h, http.MethodPost, "/2020-05-31/origin-request-policy", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), tt.wantIn)
		})
	}
}

// TestOriginRequestPolicyCRUD covers origin request policy full lifecycle.
func TestOriginRequestPolicyCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*testing.T, *cloudfront.Handler) string
		check      func(*testing.T, *httptest.ResponseRecorder, string)
		headers    func(*testing.T, *cloudfront.Handler, string) map[string]string
		name       string
		method     string
		path       string
		body       []byte
		wantStatus int
	}{
		{
			name:   "create_orp",
			method: http.MethodPost,
			path:   "/2020-05-31/origin-request-policy",
			body: []byte(
				`<OriginRequestPolicyConfig>` +
					`<Name>my-orp</Name><Comment>comment</Comment>` +
					`</OriginRequestPolicyConfig>`,
			),
			setup: func(t *testing.T, _ *cloudfront.Handler) string {
				t.Helper()

				return ""
			},
			wantStatus: http.StatusCreated,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "OriginRequestPolicy")
				assert.Contains(t, rec.Body.String(), "my-orp")
				assert.NotEmpty(t, rec.Header().Get("Location"))
			},
		},
		{
			name:   "get_orp",
			method: http.MethodGet,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				p, err := h.Backend.CreateOriginRequestPolicy("get-orp", "")
				require.NoError(t, err)

				return "/2020-05-31/origin-request-policy/" + p.ID
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "get-orp")
			},
		},
		{
			name:   "get_orp_not_found",
			method: http.MethodGet,
			path:   "/2020-05-31/origin-request-policy/DOESNOTEXIST",
			body:   nil,
			setup: func(t *testing.T, _ *cloudfront.Handler) string {
				t.Helper()

				return ""
			},
			wantStatus: http.StatusNotFound,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "NoSuchOriginRequestPolicy")
			},
		},
		{
			name:   "get_orp_config",
			method: http.MethodGet,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				p, err := h.Backend.CreateOriginRequestPolicy("cfg-orp", "")
				require.NoError(t, err)

				return "/2020-05-31/origin-request-policy/" + p.ID + "/config"
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "OriginRequestPolicyConfig")
			},
		},
		{
			name:   "list_orps",
			method: http.MethodGet,
			path:   "/2020-05-31/origin-request-policy",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				_, err := h.Backend.CreateOriginRequestPolicy("list-orp", "")
				require.NoError(t, err)

				return ""
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "OriginRequestPolicyList")
				assert.Contains(t, rec.Body.String(), "list-orp")
			},
		},
		{
			name:   "update_orp",
			method: http.MethodPut,
			path:   "",
			body: []byte(
				`<OriginRequestPolicyConfig>` +
					`<Name>updated-orp</Name><Comment>new</Comment>` +
					`</OriginRequestPolicyConfig>`,
			),
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				p, err := h.Backend.CreateOriginRequestPolicy("orig-orp", "")
				require.NoError(t, err)

				// UpdateOriginRequestPolicy's real request syntax is
				// "PUT /2020-05-31/origin-request-policy/{Id}" -- bare ID, no
				// "/config" suffix (see parseCFOriginRequestPolicyPath's doc comment).
				return "/2020-05-31/origin-request-policy/" + p.ID
			},
			headers: func(t *testing.T, h *cloudfront.Handler, path string) map[string]string {
				t.Helper()
				id := strings.TrimPrefix(path, "/2020-05-31/origin-request-policy/")
				p, err := h.Backend.GetOriginRequestPolicy(id)
				require.NoError(t, err)

				return map[string]string{"If-Match": p.ETag}
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "updated-orp")
			},
		},
		{
			name:   "delete_orp",
			method: http.MethodDelete,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				p, err := h.Backend.CreateOriginRequestPolicy("del-orp", "")
				require.NoError(t, err)

				return "/2020-05-31/origin-request-policy/" + p.ID
			},
			headers: func(t *testing.T, h *cloudfront.Handler, path string) map[string]string {
				t.Helper()
				id := strings.TrimPrefix(path, "/2020-05-31/origin-request-policy/")
				p, err := h.Backend.GetOriginRequestPolicy(id)
				require.NoError(t, err)

				return map[string]string{"If-Match": p.ETag}
			},
			wantStatus: http.StatusNoContent,
			check:      func(t *testing.T, _ *httptest.ResponseRecorder, _ string) { t.Helper() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			path := tt.path

			if tt.setup != nil {
				if p := tt.setup(t, h); p != "" {
					path = p
				}
			}

			var hdrs map[string]string
			if tt.headers != nil {
				hdrs = tt.headers(t, h, path)
			}

			rec := doXMLWithHeaders(t, h, tt.method, path, tt.body, hdrs)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.check != nil {
				tt.check(t, rec, path)
			}
		})
	}
}

// TestOriginRequestPolicyWhitelistItems_WireRoundTrip proves that whitelisted
// header/cookie/query-string names survive a full Create -> Get -> GetConfig ->
// List round trip. Before this fix, every read response (orpResponseXML) emitted a
// bare <Quantity> with no <Headers>/<Cookies>/<QueryStrings> wrapper or <Items>
// list at all, so a real SDK client could never discover which names a policy
// actually whitelists.
func TestOriginRequestPolicyWhitelistItems_WireRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := []byte(`<OriginRequestPolicyConfig><Name>wire-orp</Name>` +
		`<HeadersConfig><HeaderBehavior>whitelist</HeaderBehavior>` +
		`<Headers><Items><Name>X-Custom-Header</Name></Items><Quantity>1</Quantity></Headers>` +
		`</HeadersConfig>` +
		`<CookiesConfig><CookieBehavior>whitelist</CookieBehavior>` +
		`<Cookies><Items><Name>session-id</Name></Items><Quantity>1</Quantity></Cookies>` +
		`</CookiesConfig>` +
		`<QueryStringsConfig><QueryStringBehavior>whitelist</QueryStringBehavior>` +
		`<QueryStrings><Items><Name>utm_source</Name></Items><Quantity>1</Quantity></QueryStrings>` +
		`</QueryStringsConfig>` +
		`</OriginRequestPolicyConfig>`)

	createRec := doXML(t, h, http.MethodPost, "/2020-05-31/origin-request-policy", body)
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())

	policies := h.Backend.ListOriginRequestPolicies()
	var created *cloudfront.OriginRequestPolicy
	for _, p := range policies {
		if p.Name == "wire-orp" {
			created = p
		}
	}
	require.NotNil(t, created)
	require.NotNil(t, created.HeadersConfig)
	require.NotNil(t, created.CookiesConfig)
	require.NotNil(t, created.QueryStringsConfig)
	assert.Equal(t, []string{"X-Custom-Header"}, created.HeadersConfig.Headers)
	assert.Equal(t, []string{"session-id"}, created.CookiesConfig.Cookies)
	assert.Equal(t, []string{"utm_source"}, created.QueryStringsConfig.QueryStrings)

	for _, path := range []string{
		"/2020-05-31/origin-request-policy/" + created.ID,
		"/2020-05-31/origin-request-policy/" + created.ID + "/config",
		"/2020-05-31/origin-request-policy",
	} {
		rec := doXML(t, h, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, rec.Code, path)
		body := rec.Body.String()
		assert.Contains(t, body, "<Name>X-Custom-Header</Name>", "path %s", path)
		assert.Contains(t, body, "<Name>session-id</Name>", "path %s", path)
		assert.Contains(t, body, "<Name>utm_source</Name>", "path %s", path)
	}
}

// TestUpdateOriginRequestPolicy_PartialConfigRejected pins the fix for a
// data-loss bug: updating with only one sub-config used to null out the
// other two instead of being rejected, silently discarding the caller's
// whitelisted headers/cookies/query strings.
func TestCreateOriginRequestPolicy_PartialConfigRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		partial *cloudfront.OriginRequestPolicyConfig
		name    string
	}{
		{
			name: "only_headers_config",
			partial: &cloudfront.OriginRequestPolicyConfig{
				HeadersConfig: &cloudfront.ORPHeadersConfig{HeaderBehavior: "none"},
			},
		},
		{
			name: "only_cookies_config",
			partial: &cloudfront.OriginRequestPolicyConfig{
				CookiesConfig: &cloudfront.ORPCookiesConfig{CookieBehavior: "none"},
			},
		},
		{
			name: "only_query_strings_config",
			partial: &cloudfront.OriginRequestPolicyConfig{
				QueryStringsConfig: &cloudfront.ORPQueryStringsConfig{QueryStringBehavior: "none"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			before := h.Backend.ListOriginRequestPolicies()

			_, err := h.Backend.CreateOriginRequestPolicy("orp-partial-create", "comment", tt.partial)
			require.ErrorIs(t, err, cloudfront.ErrValidation)

			assert.Len(t, h.Backend.ListOriginRequestPolicies(), len(before),
				"rejected create must not store a policy")
		})
	}
}

func TestUpdateOriginRequestPolicy_PartialConfigRejected(t *testing.T) {
	t.Parallel()

	headersCfg := &cloudfront.ORPHeadersConfig{
		HeaderBehavior: "whitelist",
		Headers:        []string{"X-Custom-Header"},
	}
	cookiesCfg := &cloudfront.ORPCookiesConfig{
		CookieBehavior: "whitelist",
		Cookies:        []string{"session-id"},
	}
	queryStringsCfg := &cloudfront.ORPQueryStringsConfig{
		QueryStringBehavior: "whitelist",
		QueryStrings:        []string{"utm_source"},
	}
	fullConfig := &cloudfront.OriginRequestPolicyConfig{
		HeadersConfig:      headersCfg,
		CookiesConfig:      cookiesCfg,
		QueryStringsConfig: queryStringsCfg,
	}

	tests := []struct {
		partial *cloudfront.OriginRequestPolicyConfig
		name    string
	}{
		{
			name: "only_headers_config",
			partial: &cloudfront.OriginRequestPolicyConfig{
				HeadersConfig: &cloudfront.ORPHeadersConfig{HeaderBehavior: "none"},
			},
		},
		{
			name: "only_cookies_config",
			partial: &cloudfront.OriginRequestPolicyConfig{
				CookiesConfig: &cloudfront.ORPCookiesConfig{CookieBehavior: "none"},
			},
		},
		{
			name: "only_query_strings_config",
			partial: &cloudfront.OriginRequestPolicyConfig{
				QueryStringsConfig: &cloudfront.ORPQueryStringsConfig{QueryStringBehavior: "none"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			created, err := h.Backend.CreateOriginRequestPolicy(
				"orp-partial-update",
				"orig comment",
				fullConfig,
			)
			require.NoError(t, err)

			_, err = h.Backend.UpdateOriginRequestPolicy(
				created.ID,
				"orp-partial-update",
				"new comment",
				tt.partial,
			)
			require.ErrorIs(t, err, cloudfront.ErrValidation)

			after, getErr := h.Backend.GetOriginRequestPolicy(created.ID)
			require.NoError(t, getErr)
			assert.Equal(
				t,
				created,
				after,
				"rejected update must leave the stored policy unchanged",
			)
		})
	}
}
