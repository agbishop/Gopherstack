package apigateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthorizerCacheSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup         func(*authorizerCache)
		name          string
		key           string
		retainedKey   string
		evictedKey    string
		ttl           time.Duration
		wantHit       bool
		wantVal       bool
		checkEviction bool
	}{
		{
			name:    "stores_and_reads_entry",
			key:     "k1",
			ttl:     time.Minute,
			wantHit: true,
			wantVal: true,
		},
		{
			name: "evicts_lru_when_max_entries_reached",
			setup: func(cache *authorizerCache) {
				cache.set("a", true, time.Minute)
				cache.set("b", false, time.Minute)
				_, _ = cache.get("a")
			},
			key:           "c",
			ttl:           time.Minute,
			wantHit:       true,
			wantVal:       true,
			checkEviction: true,
			retainedKey:   "a",
			evictedKey:    "b",
		},
		{
			name: "zero_ttl_is_not_cached",
			key:  "k2",
			ttl:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cache := newAuthorizerCacheWithMaxEntries(2)
			if tt.setup != nil {
				tt.setup(cache)
			}

			cache.set(tt.key, tt.wantVal, tt.ttl)
			gotVal, gotHit := cache.get(tt.key)
			assert.Equal(t, tt.wantHit, gotHit)
			assert.Equal(t, tt.wantVal, gotVal)

			if tt.checkEviction {
				_, retainedHit := cache.get(tt.retainedKey)
				_, evictedHit := cache.get(tt.evictedKey)
				require.True(t, retainedHit)
				assert.False(t, evictedHit)
			}
		})
	}
}

func TestFindMatchingResource(t *testing.T) {
	t.Parallel()

	type findMatchingResourceArgs struct {
		requestPath string
		stageName   string
		resources   []Resource
	}

	type findMatchingResourceWant struct {
		params     map[string]string
		resourceID string
		wantNil    bool
	}

	tests := []struct {
		name string
		args findMatchingResourceArgs
		want findMatchingResourceWant
	}{
		{
			name: "exact_match_preferred_over_greedy",
			args: findMatchingResourceArgs{
				resources: []Resource{
					{ID: "greedy", Path: "/items/{proxy+}"},
					{ID: "exact", Path: "/items/special"},
				},
				requestPath: "/items/special",
				stageName:   "prod",
			},
			want: findMatchingResourceWant{
				resourceID: "exact",
				params:     map[string]string{},
			},
		},
		{
			name: "single_segment_parameter",
			args: findMatchingResourceArgs{
				resources: []Resource{
					{ID: "param", Path: "/items/{id}"},
				},
				requestPath: "/items/42",
				stageName:   "prod",
			},
			want: findMatchingResourceWant{
				resourceID: "param",
				params:     map[string]string{"id": "42"},
			},
		},
		{
			name: "stage_prefix_is_stripped",
			args: findMatchingResourceArgs{
				resources: []Resource{
					{ID: "param", Path: "/items/{id}"},
				},
				requestPath: "/prod/items/42",
				stageName:   "prod",
			},
			want: findMatchingResourceWant{
				resourceID: "param",
				params:     map[string]string{"id": "42"},
			},
		},
		{
			name: "non_terminal_greedy_pattern_is_ignored",
			args: findMatchingResourceArgs{
				resources: []Resource{
					{ID: "bad", Path: "/{proxy+}/suffix"},
				},
				requestPath: "/anything/suffix",
				stageName:   "prod",
			},
			want: findMatchingResourceWant{wantNil: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resource, params := findMatchingResource(tt.args.resources, tt.args.requestPath, tt.args.stageName)
			if tt.want.wantNil {
				assert.Nil(t, resource)
				assert.Nil(t, params)

				return
			}

			require.NotNil(t, resource)
			assert.Equal(t, tt.want.resourceID, resource.ID)
			assert.Equal(t, tt.want.params, params)
		})
	}
}

func TestResolveRequestParamSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		src    string
		setup  func(r *http.Request)
		expect string
	}{
		{
			name:   "static_value",
			src:    "myservice",
			setup:  func(_ *http.Request) {},
			expect: "myservice",
		},
		{
			name: "method_request_header",
			src:  "method.request.header.Authorization",
			setup: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer tok")
			},
			expect: "Bearer tok",
		},
		{
			name:   "method_request_header_missing",
			src:    "method.request.header.X-Missing",
			setup:  func(_ *http.Request) {},
			expect: "",
		},
		{
			name: "method_request_querystring",
			src:  "method.request.querystring.userId",
			setup: func(r *http.Request) {
				q := r.URL.Query()
				q.Set("userId", "42")
				r.URL.RawQuery = q.Encode()
			},
			expect: "42",
		},
		{
			name:   "unknown_method_type",
			src:    "method.request.body",
			setup:  func(_ *http.Request) {},
			expect: "",
		},
		{
			name:   "not_method_prefix",
			src:    "stageVariables.something",
			setup:  func(_ *http.Request) {},
			expect: "stageVariables.something",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "https://example.com/api?", nil)
			tt.setup(r)
			got := resolveRequestParamSource(r, tt.src)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestResolveResponseParamSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		src    string
		expect string
	}{
		{
			name:   "static_value",
			src:    "fixed-value",
			expect: "fixed-value",
		},
		{
			name:   "integration_response_header",
			src:    "integration.response.header.X-Amzn-Requestid",
			expect: "X-Amzn-Requestid",
		},
		{
			name:   "empty_string",
			src:    "",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveResponseParamSource(tt.src)
			assert.Equal(t, tt.expect, got)
		})
	}
}

func TestApplyIntegrationRequestParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		params        map[string]string
		setupIncoming func(r *http.Request)
		checkOutgoing func(t *testing.T, r *http.Request)
		name          string
	}{
		{
			name: "header_from_method_header",
			params: map[string]string{
				"integration.request.header.X-Api-Caller": "method.request.header.Authorization",
			},
			setupIncoming: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer token123")
			},
			checkOutgoing: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, "Bearer token123", r.Header.Get("X-Api-Caller"))
			},
		},
		{
			name: "querystring_from_method_querystring",
			params: map[string]string{
				"integration.request.querystring.user_id": "method.request.querystring.userId",
			},
			setupIncoming: func(r *http.Request) {
				q := r.URL.Query()
				q.Set("userId", "99")
				r.URL.RawQuery = q.Encode()
			},
			checkOutgoing: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, "99", r.URL.Query().Get("user_id"))
			},
		},
		{
			name: "static_header_value",
			params: map[string]string{
				"integration.request.header.X-Service": "myservice",
			},
			setupIncoming: func(_ *http.Request) {},
			checkOutgoing: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Equal(t, "myservice", r.Header.Get("X-Service"))
			},
		},
		{
			name: "missing_source_value_skipped",
			params: map[string]string{
				"integration.request.header.X-Missing": "method.request.header.X-Not-Present",
			},
			setupIncoming: func(_ *http.Request) {},
			checkOutgoing: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Empty(t, r.Header.Get("X-Missing"))
			},
		},
		{
			name: "non_integration_dest_ignored",
			params: map[string]string{
				"method.response.header.X-Foo": "bar",
			},
			setupIncoming: func(_ *http.Request) {},
			checkOutgoing: func(t *testing.T, r *http.Request) {
				t.Helper()
				assert.Empty(t, r.Header.Get("X-Foo"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			incoming := httptest.NewRequest(http.MethodGet, "https://example.com/api?", nil)
			tt.setupIncoming(incoming)

			outgoing := httptest.NewRequest(http.MethodGet, "https://upstream.example.com/", nil)
			applyIntegrationRequestParams(incoming, outgoing, tt.params)
			tt.checkOutgoing(t, outgoing)
		})
	}
}

// TestDeleteRestAPI_EvictsTrieCache verifies that deleting a REST API evicts its
// cached routing trie. RestApi IDs are freshly random on every CreateRestApi, so a
// stale h.trieCache entry from a deleted API can never be overwritten by a later
// Store -- left unfixed, every RestApi ever routed to and deleted stays in the
// process's memory for the life of the server.
func TestDeleteRestAPI_EvictsTrieCache(t *testing.T) {
	t.Parallel()

	backend := NewInMemoryBackend()
	h := NewHandler(backend)

	api, err := backend.CreateRestAPI(CreateRestAPIInput{Name: "leak-api"})
	require.NoError(t, err)

	// Prime the trie cache the same way a proxied request would.
	_, err = h.routingTrie(api.ID)
	require.NoError(t, err)

	_, cached := h.trieCache.Load(api.ID)
	require.True(t, cached, "trie cache should hold an entry after routingTrie")

	status, _, err := h.deleteRestAPIAction([]byte(`{"restApiId":"` + api.ID + `"}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, status)

	_, stillCached := h.trieCache.Load(api.ID)
	assert.False(t, stillCached, "trie cache entry must be evicted on DeleteRestApi")
}

func TestApplyIntegrationResponseParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		params      map[string]string
		checkResult func(t *testing.T, w *httptest.ResponseRecorder)
		name        string
	}{
		{
			name: "static_header",
			params: map[string]string{
				"method.response.header.X-Version": "v2",
			},
			checkResult: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				assert.Equal(t, "v2", w.Header().Get("X-Version"))
			},
		},
		{
			name: "integration_response_header_echo",
			params: map[string]string{
				"method.response.header.X-Request-Id": "integration.response.header.X-Amzn-Requestid",
			},
			checkResult: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				assert.Equal(t, "X-Amzn-Requestid", w.Header().Get("X-Request-ID"))
			},
		},
		{
			name: "non_method_response_dest_ignored",
			params: map[string]string{
				"integration.request.header.X-Foo": "bar",
			},
			checkResult: func(t *testing.T, w *httptest.ResponseRecorder) {
				t.Helper()
				assert.Empty(t, w.Header().Get("X-Foo"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			applyIntegrationResponseParams(w, tt.params)
			tt.checkResult(t, w)
		})
	}
}
