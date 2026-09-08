package cloudfront_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudfront"
)

// TestKVSStatusAndTimestamp verifies KVS create/describe carry a
// READY status and a non-empty last-modified time.
func TestKVSStatusAndTimestamp(t *testing.T) {
	t.Parallel()

	b := newB(t)
	kvs, err := b.CreateKeyValueStore("kvs-status", "c", nil)
	require.NoError(t, err)
	assert.Equal(t, "READY", kvs.Status)
	assert.NotEmpty(t, kvs.LastModifiedTime)

	got, err := b.GetKeyValueStore(kvs.ID)
	require.NoError(t, err)
	assert.Equal(t, "READY", got.Status)
	assert.NotEmpty(t, got.LastModifiedTime)
}

// TestUpdateKeyValueStore tests that UpdateKeyValueStore works correctly.
func TestUpdateKeyValueStore(t *testing.T) {
	t.Parallel()
	h := newCFHandler(t)
	const prefix = "/2020-05-31/"

	// Create KVS.
	createRec := cfRequest(
		t,
		h,
		http.MethodPost,
		prefix+"key-value-store",
		`<CreateKeyValueStoreRequest><Name>test-kvs</Name><Comment>initial</Comment></CreateKeyValueStoreRequest>`,
	)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	kvsID := extractXMLID(t, createRec.Body.String())
	if kvsID == "" {
		t.Fatal("no KVS ID in create response")
	}
	etag := createRec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag in create response")
	}
	if !strings.Contains(createRec.Body.String(), "<Status>READY</Status>") {
		t.Errorf("expected READY status in create response, got: %s", createRec.Body.String())
	}

	// Update without If-Match must be rejected with 412.
	noMatch := cfRequest(
		t,
		h,
		http.MethodPut,
		prefix+"key-value-store/"+kvsID,
		`<UpdateKeyValueStoreRequest><Name>test-kvs</Name><Comment>x</Comment></UpdateKeyValueStoreRequest>`,
	)
	if noMatch.Code != http.StatusPreconditionFailed {
		t.Errorf("update without If-Match: want 412, got %d", noMatch.Code)
	}

	// Update with matching If-Match succeeds.
	updateRec := cfRequestWithBodyHeaders(
		t,
		h,
		http.MethodPut,
		prefix+"key-value-store/"+kvsID,
		`<UpdateKeyValueStoreRequest><Name>test-kvs</Name><Comment>updated</Comment></UpdateKeyValueStoreRequest>`,
		map[string]string{"If-Match": etag},
	)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}
	if !strings.Contains(updateRec.Body.String(), "updated") {
		t.Errorf("expected updated comment in response, got: %s", updateRec.Body.String())
	}
}

// TestInMemoryBackend_KVSDataPlane tests KVS data plane backend methods directly.
func TestInMemoryBackend_KVSDataPlane(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(*testing.T, *cloudfront.InMemoryBackend)
		name string
	}{
		{
			name: "put_get_value",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				kvs, err := b.CreateKeyValueStore("kvs", "", nil)
				require.NoError(t, err)
				_, err = b.PutKVSValue(kvs.ID, "key1", "val1", "")
				require.NoError(t, err)
				val, _, err := b.GetKVSValue(kvs.ID, "key1")
				require.NoError(t, err)
				assert.Equal(t, "val1", val)
			},
		},
		{
			name: "delete_value",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				kvs, err := b.CreateKeyValueStore("kvs", "", nil)
				require.NoError(t, err)
				_, err = b.PutKVSValue(kvs.ID, "key1", "val1", "")
				require.NoError(t, err)
				_, err = b.DeleteKVSValue(kvs.ID, "key1", "")
				require.NoError(t, err)
				_, _, err = b.GetKVSValue(kvs.ID, "key1")
				require.Error(t, err)
			},
		},
		{
			name: "list_values_sorted",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				kvs, err := b.CreateKeyValueStore("kvs", "", nil)
				require.NoError(t, err)
				_, err = b.PutKVSValue(kvs.ID, "beta", "2", "")
				require.NoError(t, err)
				_, err = b.PutKVSValue(kvs.ID, "alpha", "1", "")
				require.NoError(t, err)
				items, _, err := b.ListKVSValues(kvs.ID)
				require.NoError(t, err)
				assert.Len(t, items, 2)
				assert.Equal(t, "alpha", items[0].Key)
				assert.Equal(t, "beta", items[1].Key)
			},
		},
		{
			name: "batch_update_keys",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				kvs, err := b.CreateKeyValueStore("kvs", "", nil)
				require.NoError(t, err)
				_, err = b.PutKVSValue(kvs.ID, "del-me", "gone", "")
				require.NoError(t, err)
				puts := []*cloudfront.KVSItem{{Key: "new", Value: "added"}}
				_, err = b.UpdateKVSValues(kvs.ID, "", puts, []string{"del-me"})
				require.NoError(t, err)
				items, _, err := b.ListKVSValues(kvs.ID)
				require.NoError(t, err)
				assert.Len(t, items, 1)
				assert.Equal(t, "new", items[0].Key)
			},
		},
		{
			name: "etag_mismatch_rejected",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				kvs, err := b.CreateKeyValueStore("kvs", "", nil)
				require.NoError(t, err)
				_, err = b.PutKVSValue(kvs.ID, "k", "v", "wrong-etag")
				require.Error(t, err)
			},
		},
		{
			name: "etag_match_accepted",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				kvs, err := b.CreateKeyValueStore("kvs", "", nil)
				require.NoError(t, err)
				etag1, err := b.PutKVSValue(kvs.ID, "k", "v1", "")
				require.NoError(t, err)
				_, err = b.PutKVSValue(kvs.ID, "k", "v2", etag1)
				require.NoError(t, err)
				val, _, err := b.GetKVSValue(kvs.ID, "k")
				require.NoError(t, err)
				assert.Equal(t, "v2", val)
			},
		},
		{
			name: "get_not_found",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				kvs, err := b.CreateKeyValueStore("kvs", "", nil)
				require.NoError(t, err)
				_, _, err = b.GetKVSValue(kvs.ID, "missing")
				require.Error(t, err)
			},
		},
		{
			name: "kvs_not_found",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				_, _, err := b.GetKVSValue("nope", "key")
				require.Error(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newAuditBackend(t)
			tt.run(t, b)
		})
	}
}

// TestKeyValueStoreCRUD covers the full Key Value Store lifecycle via the HTTP handler.
func TestKeyValueStoreCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(*testing.T, *cloudfront.Handler) string
		check       func(*testing.T, *httptest.ResponseRecorder, string)
		headersFunc func(*testing.T, *cloudfront.Handler, string) map[string]string
		name        string
		method      string
		path        string
		body        []byte
		wantStatus  int
	}{
		{
			name:   "create_key_value_store",
			method: http.MethodPost,
			path:   "/2020-05-31/key-value-store",
			body: []byte(
				`<CreateKeyValueStoreRequest><Name>my-kvs</Name><Comment>test</Comment></CreateKeyValueStoreRequest>`,
			),
			setup: func(t *testing.T, _ *cloudfront.Handler) string {
				t.Helper()

				return ""
			},
			wantStatus: http.StatusCreated,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<KeyValueStore")
				assert.NotEmpty(t, rec.Header().Get("Location"))
				assert.NotEmpty(t, rec.Header().Get("ETag"))
			},
		},
		{
			name:   "list_key_value_stores",
			method: http.MethodGet,
			path:   "/2020-05-31/key-value-store",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				_, err := h.Backend.CreateKeyValueStore("list-kvs", "comment", nil)
				require.NoError(t, err)

				return ""
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<KeyValueStoreList")
				assert.Contains(t, rec.Body.String(), "<Quantity>1</Quantity>")
			},
		},
		{
			name:   "get_key_value_store",
			method: http.MethodGet,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				kvs, err := h.Backend.CreateKeyValueStore("get-kvs", "comment", nil)
				require.NoError(t, err)

				return "/2020-05-31/key-value-store/" + kvs.ID
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<KeyValueStore")
				assert.NotEmpty(t, rec.Header().Get("ETag"))
			},
		},
		{
			name:   "delete_key_value_store",
			method: http.MethodDelete,
			path:   "",
			body:   nil,
			setup: func(t *testing.T, h *cloudfront.Handler) string {
				t.Helper()
				kvs, err := h.Backend.CreateKeyValueStore("del-kvs", "delete me", nil)
				require.NoError(t, err)

				return "/2020-05-31/key-value-store/" + kvs.ID
			},
			headersFunc: func(t *testing.T, h *cloudfront.Handler, path string) map[string]string {
				t.Helper()
				parts := strings.Split(strings.TrimRight(path, "/"), "/")
				id := parts[len(parts)-1]
				kvs, err := h.Backend.GetKeyValueStore(id)
				require.NoError(t, err)

				return map[string]string{"If-Match": kvs.ETag}
			},
			wantStatus: http.StatusNoContent,
			check:      nil,
		},
		{
			name:   "get_key_value_store_not_found",
			method: http.MethodGet,
			path:   "/2020-05-31/key-value-store/doesnotexist",
			body:   nil,
			setup: func(t *testing.T, _ *cloudfront.Handler) string {
				t.Helper()

				return ""
			},
			wantStatus: http.StatusNotFound,
			check: func(t *testing.T, rec *httptest.ResponseRecorder, _ string) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "<Error>")
			},
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
			if tt.headersFunc != nil {
				hdrs = tt.headersFunc(t, h, path)
			}
			rec := doXMLWithHeaders(t, h, tt.method, path, tt.body, hdrs)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.check != nil {
				tt.check(t, rec, path)
			}
		})
	}
}

// TestInMemoryBackend_KeyValueStore tests Key Value Store backend operations directly.
func TestInMemoryBackend_KeyValueStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(*testing.T, *cloudfront.InMemoryBackend)
		name string
	}{
		{
			name: "create_get_list_delete",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				kvs, err := b.CreateKeyValueStore("kvs-name", "comment", nil)
				require.NoError(t, err)
				assert.NotEmpty(t, kvs.ID)

				got, err := b.GetKeyValueStore(kvs.ID)
				require.NoError(t, err)
				assert.Equal(t, "kvs-name", got.Name)

				list := b.ListKeyValueStores()
				assert.Len(t, list, 1)

				require.NoError(t, b.DeleteKeyValueStore(kvs.ID))
				_, err = b.GetKeyValueStore(kvs.ID)
				require.Error(t, err)
			},
		},
		{
			name: "get_not_found",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				_, err := b.GetKeyValueStore("doesnotexist")
				require.Error(t, err)
			},
		},
		{
			name: "delete_not_found",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				err := b.DeleteKeyValueStore("doesnotexist")
				require.Error(t, err)
			},
		},
		{
			name: "delete_clears_data_plane_state",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend) {
				t.Helper()
				kvs, err := b.CreateKeyValueStore("kvs-ghost", "c", nil)
				require.NoError(t, err)
				_, err = b.PutKVSValue(kvs.ID, "k", "v", "")
				require.NoError(t, err)

				require.NoError(t, b.DeleteKeyValueStore(kvs.ID))

				snap := b.Snapshot(t.Context())
				var decoded struct {
					KeyValueStoreData map[string]map[string]string `json:"keyValueStoreData"`
					KeyValueDataETags map[string]string            `json:"keyValueDataETags"`
				}
				require.NoError(t, json.Unmarshal(snap, &decoded))
				assert.NotContains(t, decoded.KeyValueStoreData, kvs.ID,
					"deleted KVS's data-plane values must not survive in the persisted snapshot")
				assert.NotContains(t, decoded.KeyValueDataETags, kvs.ID,
					"deleted KVS's data-plane ETag must not survive in the persisted snapshot")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")
			tt.run(t, b)
		})
	}
}
