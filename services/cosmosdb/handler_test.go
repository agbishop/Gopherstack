package cosmosdb_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cosmosdb"
)

func newTestHandler(t *testing.T) *cosmosdb.Handler {
	t.Helper()

	backend := cosmosdb.NewInMemoryBackend()

	return cosmosdb.NewHandler(backend)
}

// doRequest builds an echo context for method/path/headers/body and invokes
// the handler directly, mirroring services/azuretable's doRequest.
func doRequest(
	t *testing.T, h *cosmosdb.Handler, method, path string, headers map[string]string, body []byte,
) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, http.NoBody)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))

	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &m))

	return m
}

func TestHandler_CommonHeaders(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/dbs", nil, nil)

	assert.NotEmpty(t, rec.Header().Get("X-Ms-Version"))
	assert.NotEmpty(t, rec.Header().Get("X-Ms-Request-Id"))
	assert.NotEmpty(t, rec.Header().Get("Date"))
	assert.Equal(t, "1", rec.Header().Get("X-Ms-Request-Charge"))
	assert.NotEmpty(t, rec.Header().Get("X-Ms-Session-Token"))
	assert.NotEmpty(t, rec.Header().Get("X-Ms-Activity-Id"))
}

func TestHandler_InvalidURI(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		name string
		path string
	}{
		// "/foo" (a single path segment, not "dbs") used to be an invalid
		// Core/SQL URI; it is now a valid Table API entity-collection path
		// (see table_api.go's isTableAPIPath and
		// TestHandler_TableAPI_UnknownTableIs404) -- AZURE.md section 9's M6
		// milestone repurposed exactly this path shape, so it is no longer
		// covered here.
		{name: "too many segments", path: "/dbs/a/colls/b/docs/c/extra"},
		{name: "colls typo", path: "/dbs/a/bogus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, http.MethodGet, tt.path, nil, nil)
			assert.Equal(t, http.StatusBadRequest, rec.Code, tt.name)
		})
	}
}

func TestHandler_DatabaseLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createBody, err := json.Marshal(map[string]string{"id": "mydb"})
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPost, "/dbs", nil, createBody)
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("ETag"))

	body := decodeBody(t, rec)
	assert.Equal(t, "mydb", body["id"])
	assert.NotEmpty(t, body["_rid"])

	// Duplicate create -> 409.
	rec = doRequest(t, h, http.MethodPost, "/dbs", nil, createBody)
	assert.Equal(t, http.StatusConflict, rec.Code)

	// Get.
	rec = doRequest(t, h, http.MethodGet, "/dbs/mydb", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Get missing -> 404.
	rec = doRequest(t, h, http.MethodGet, "/dbs/nope", nil, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// List.
	rec = doRequest(t, h, http.MethodGet, "/dbs", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	list := decodeBody(t, rec)
	dbs, ok := list["Databases"].([]any)
	require.True(t, ok)
	assert.Len(t, dbs, 1)
	assert.InDelta(t, 1, list["_count"], 0)

	// Delete.
	rec = doRequest(t, h, http.MethodDelete, "/dbs/mydb", nil, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	rec = doRequest(t, h, http.MethodDelete, "/dbs/mydb", nil, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_CreateDatabase_InvalidInput(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		name string
		body string
	}{
		{name: "not json", body: "not json"},
		{name: "empty id", body: `{"id":""}`},
		{name: "missing id", body: `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, http.MethodPost, "/dbs", nil, []byte(tt.body))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// createTestDatabase creates database "mydb" and returns the handler.
func createTestDatabase(t *testing.T, h *cosmosdb.Handler) {
	t.Helper()

	body, err := json.Marshal(map[string]string{"id": "mydb"})
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPost, "/dbs", nil, body)
	require.Equal(t, http.StatusCreated, rec.Code)
}

func createTestContainer(t *testing.T, h *cosmosdb.Handler) {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"id":           "mycoll",
		"partitionKey": map[string]any{"paths": []string{"/pk"}, "kind": "Hash"},
	})
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPost, "/dbs/mydb/colls", nil, body)
	require.Equal(t, http.StatusCreated, rec.Code)
}

func TestHandler_ContainerLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestDatabase(t, h)

	body, err := json.Marshal(map[string]any{
		"id":           "mycoll",
		"partitionKey": map[string]any{"paths": []string{"/pk"}, "kind": "Hash"},
	})
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPost, "/dbs/mydb/colls", nil, body)
	require.Equal(t, http.StatusCreated, rec.Code)

	respBody := decodeBody(t, rec)
	assert.Equal(t, "mycoll", respBody["id"])
	pkDef, ok := respBody["partitionKey"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Hash", pkDef["kind"])

	// Duplicate -> 409.
	rec = doRequest(t, h, http.MethodPost, "/dbs/mydb/colls", nil, body)
	assert.Equal(t, http.StatusConflict, rec.Code)

	// Against missing database -> 404.
	rec = doRequest(t, h, http.MethodPost, "/dbs/nope/colls", nil, body)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Get.
	rec = doRequest(t, h, http.MethodGet, "/dbs/mydb/colls/mycoll", nil, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// List.
	rec = doRequest(t, h, http.MethodGet, "/dbs/mydb/colls", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	list := decodeBody(t, rec)
	colls, ok := list["DocumentCollections"].([]any)
	require.True(t, ok)
	assert.Len(t, colls, 1)

	// Delete.
	rec = doRequest(t, h, http.MethodDelete, "/dbs/mydb/colls/mycoll", nil, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/dbs/mydb/colls/mycoll", nil, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_CreateContainer_InvalidPartitionKey(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestDatabase(t, h)

	body, err := json.Marshal(map[string]any{
		"id":           "mycoll",
		"partitionKey": map[string]any{"paths": []string{"/a", "/b"}, "kind": "Hash"},
	})
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPost, "/dbs/mydb/colls", nil, body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_CreateContainer_EmptyPartitionKeyPath covers store.go's
// backend-level rejection of an empty partitionKey.paths[0] (a single path
// that's an empty string slips past the handler's len(paths) != 1 check,
// which only counts entries, and must be caught by the backend too, mapped
// to 400 BadRequest rather than falling through to a 500 InternalError).
func TestHandler_CreateContainer_EmptyPartitionKeyPath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestDatabase(t, h)

	body, err := json.Marshal(map[string]any{
		"id":           "mycoll",
		"partitionKey": map[string]any{"paths": []string{""}, "kind": "Hash"},
	})
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPost, "/dbs/mydb/colls", nil, body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_CreateDocument_TrailingContentRejected covers
// decodeJSONObject's rejection of a body carrying more than one JSON value
// (e.g. two concatenated objects): json.Decoder.Decode only consumes the
// first value and silently ignores anything after it by default, which
// would let a malformed or concatenated body through as if it were a
// single well-formed document.
func TestHandler_CreateDocument_TrailingContentRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestDatabase(t, h)
	createTestContainer(t, h)

	for _, trailing := range []string{`{"id":"1","pk":"a"}{}`, `{"id":"1","pk":"a"}]`, `{"id":"1","pk":"a"}}`} {
		rec := doRequest(t, h, http.MethodPost, "/dbs/mydb/colls/mycoll/docs", nil, []byte(trailing))
		assert.Equal(t, http.StatusBadRequest, rec.Code, trailing)
	}
}

func TestHandler_DocumentLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestDatabase(t, h)
	createTestContainer(t, h)

	docBody, err := json.Marshal(map[string]any{"id": "doc1", "pk": "partA", "value": 42})
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPost, "/dbs/mydb/colls/mycoll/docs", nil, docBody)
	require.Equal(t, http.StatusCreated, rec.Code)
	etag := rec.Header().Get("ETag")
	require.NotEmpty(t, etag)

	created := decodeBody(t, rec)
	assert.Equal(t, "doc1", created["id"])
	assert.NotEmpty(t, created["_rid"])
	assert.NotEmpty(t, created["_self"])
	assert.NotEmpty(t, created["_attachments"])

	// Duplicate create (no upsert) -> 409.
	rec = doRequest(t, h, http.MethodPost, "/dbs/mydb/colls/mycoll/docs", nil, docBody)
	assert.Equal(t, http.StatusConflict, rec.Code)

	// Upsert succeeds.
	rec = doRequest(t, h, http.MethodPost, "/dbs/mydb/colls/mycoll/docs",
		map[string]string{"X-Ms-Documentdb-Is-Upsert": "true"}, docBody)
	assert.Equal(t, http.StatusCreated, rec.Code)

	// Get requires partition key header.
	rec = doRequest(t, h, http.MethodGet, "/dbs/mydb/colls/mycoll/docs/doc1", nil, nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	pkHeader := map[string]string{"X-Ms-Documentdb-Partitionkey": `["partA"]`}

	rec = doRequest(t, h, http.MethodGet, "/dbs/mydb/colls/mycoll/docs/doc1", pkHeader, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	got := decodeBody(t, rec)
	assert.InDelta(t, 42, got["value"], 0.0001)

	// Get missing document -> 404.
	rec = doRequest(t, h, http.MethodGet, "/dbs/mydb/colls/mycoll/docs/nope", pkHeader, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Replace with wrong If-Match -> 412.
	replaceBody, err := json.Marshal(map[string]any{"pk": "partA", "value": 100})
	require.NoError(t, err)

	wrongEtagHeaders := map[string]string{
		"X-Ms-Documentdb-Partitionkey": `["partA"]`, "If-Match": `"bogus"`,
	}
	rec = doRequest(t, h, http.MethodPut, "/dbs/mydb/colls/mycoll/docs/doc1", wrongEtagHeaders, replaceBody)
	assert.Equal(t, http.StatusPreconditionFailed, rec.Code)

	// Unconditional replace succeeds and drops unrelated fields.
	rec = doRequest(t, h, http.MethodPut, "/dbs/mydb/colls/mycoll/docs/doc1", pkHeader, replaceBody)
	require.Equal(t, http.StatusOK, rec.Code)

	replaced := decodeBody(t, rec)
	assert.InDelta(t, 100, replaced["value"], 0.0001)
	newEtag := rec.Header().Get("ETag")
	assert.NotEqual(t, etag, newEtag)

	// Delete with wrong ETag fails.
	rec = doRequest(t, h, http.MethodDelete, "/dbs/mydb/colls/mycoll/docs/doc1", wrongEtagHeaders, nil)
	assert.Equal(t, http.StatusPreconditionFailed, rec.Code)

	// Delete with correct ETag succeeds.
	correctEtagHeaders := map[string]string{
		"X-Ms-Documentdb-Partitionkey": `["partA"]`, "If-Match": newEtag,
	}
	rec = doRequest(t, h, http.MethodDelete, "/dbs/mydb/colls/mycoll/docs/doc1", correctEtagHeaders, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify gone.
	rec = doRequest(t, h, http.MethodGet, "/dbs/mydb/colls/mycoll/docs/doc1", pkHeader, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestPartitionKeyFromHeader_ExactlyOneScalarElement covers the fix
// requiring the x-ms-documentdb-partitionkey header's JSON array to carry
// exactly one scalar element: an empty array must not be silently treated
// as a null partition key ([null] is the correct way to express that), and
// an array with more than one element must not be silently truncated to
// its first element -- either of which used to succeed and could route a
// request against the wrong document with no error at all.
func TestPartitionKeyFromHeader_ExactlyOneScalarElement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		header  string
		wantErr bool
	}{
		{name: "single string element", header: `["foo"]`, wantErr: false},
		{name: "single null element is the valid way to express a null PK", header: `[null]`, wantErr: false},
		{name: "empty array is rejected, not treated as null", header: `[]`, wantErr: true},
		{name: "two elements is rejected, not truncated", header: `["a","b"]`, wantErr: true},
		{name: "object element is rejected (not a scalar)", header: `[{"x":1}]`, wantErr: true},
		{name: "array element is rejected (not a scalar)", header: `[[1,2]]`, wantErr: true},
		{name: "trailing content after the array is rejected, not silently ignored", header: `["a"]{}`, wantErr: true},
		{name: "unmatched closing bracket after the array is rejected", header: `["a"]]`, wantErr: true},
		{name: "unmatched closing brace after the array is rejected", header: `["a"]}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "/dbs/a/colls/b/docs/c", http.NoBody)
			require.NoError(t, err)
			req.Header.Set("X-Ms-Documentdb-Partitionkey", tt.header)

			_, pkErr := cosmosdb.PartitionKeyFromHeader(req)
			if tt.wantErr {
				require.Error(t, pkErr)
			} else {
				require.NoError(t, pkErr)
			}
		})
	}
}

func TestHandler_ReadFeed(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestDatabase(t, h)
	createTestContainer(t, h)

	for _, id := range []string{"a", "b"} {
		body, err := json.Marshal(map[string]any{"id": id, "pk": "x"})
		require.NoError(t, err)

		rec := doRequest(t, h, http.MethodPost, "/dbs/mydb/colls/mycoll/docs", nil, body)
		require.Equal(t, http.StatusCreated, rec.Code)
	}

	rec := doRequest(t, h, http.MethodGet, "/dbs/mydb/colls/mycoll/docs", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	feed := decodeBody(t, rec)
	assert.InDelta(t, 2, feed["_count"], 0)
}

func TestHandler_Query(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestDatabase(t, h)
	createTestContainer(t, h)

	for i, name := range []string{"alice", "bob"} {
		body, err := json.Marshal(map[string]any{"id": name, "pk": "x", "n": i})
		require.NoError(t, err)

		rec := doRequest(t, h, http.MethodPost, "/dbs/mydb/colls/mycoll/docs", nil, body)
		require.Equal(t, http.StatusCreated, rec.Code)
	}

	queryBody, err := json.Marshal(map[string]any{"query": "SELECT * FROM c WHERE c.id = 'alice'"})
	require.NoError(t, err)

	rec := doRequest(t, h, http.MethodPost, "/dbs/mydb/colls/mycoll/docs",
		map[string]string{"X-Ms-Documentdb-Isquery": "True", "Content-Type": "application/query+json"}, queryBody)
	require.Equal(t, http.StatusOK, rec.Code)

	result := decodeBody(t, rec)
	assert.InDelta(t, 1, result["_count"], 0)

	// A malformed query yields 400, never a panic.
	badBody, err := json.Marshal(map[string]any{"query": "NOT VALID SQL AT ALL ((("})
	require.NoError(t, err)

	rec = doRequest(t, h, http.MethodPost, "/dbs/mydb/colls/mycoll/docs",
		map[string]string{"X-Ms-Documentdb-Isquery": "true"}, badBody)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Trailing content after the query request object is rejected, not
	// silently ignored (mirrors partitionKeyFromHeader's identical guard),
	// including unmatched-delimiter forms dec.More() alone wouldn't catch.
	trailingQueries := []string{
		`{"query": "SELECT * FROM c"}{}`,
		`{"query": "SELECT * FROM c"}]`,
		`{"query": "SELECT * FROM c"}}`,
	}
	for _, trailing := range trailingQueries {
		rec = doRequest(t, h, http.MethodPost, "/dbs/mydb/colls/mycoll/docs",
			map[string]string{"X-Ms-Documentdb-Isquery": "true"}, []byte(trailing))
		assert.Equal(t, http.StatusBadRequest, rec.Code, trailing)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPut, "/dbs", nil, nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandler_ResetClearsState(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestDatabase(t, h)

	h.Reset()

	rec := doRequest(t, h, http.MethodGet, "/dbs/mydb", nil, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestParseResourcePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		wantDB   string
		wantColl string
		wantDoc  string
		wantKind int
	}{
		{name: "account root", path: "/", wantKind: 1},
		{name: "account root, no leading slash", path: "", wantKind: 1},
		{name: "databases", path: "/dbs", wantKind: 2},
		{name: "database item", path: "/dbs/mydb", wantKind: 3, wantDB: "mydb"},
		{name: "containers", path: "/dbs/mydb/colls", wantKind: 4, wantDB: "mydb"},
		{name: "container item", path: "/dbs/mydb/colls/mycoll", wantKind: 5, wantDB: "mydb", wantColl: "mycoll"},
		{name: "documents", path: "/dbs/mydb/colls/mycoll/docs", wantKind: 6, wantDB: "mydb", wantColl: "mycoll"},
		{
			name: "document item", path: "/dbs/mydb/colls/mycoll/docs/doc1", wantKind: 7,
			wantDB: "mydb", wantColl: "mycoll", wantDoc: "doc1",
		},
		{name: "not dbs", path: "/foo", wantKind: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			kind, db, coll, doc := cosmosdb.ParseResourcePath(tt.path)
			assert.Equal(t, tt.wantKind, kind)
			assert.Equal(t, tt.wantDB, db)
			assert.Equal(t, tt.wantColl, coll)
			assert.Equal(t, tt.wantDoc, doc)
		})
	}
}

func TestIsQueryRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		headers map[string]string
		name    string
		want    bool
	}{
		{name: "isquery true", headers: map[string]string{"X-Ms-Documentdb-Isquery": "True"}, want: true},
		{name: "isquery lowercase", headers: map[string]string{"X-Ms-Documentdb-Isquery": "true"}, want: true},
		{name: "content type", headers: map[string]string{"Content-Type": "application/query+json"}, want: true},
		{name: "neither", headers: map[string]string{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/dbs/a/colls/b/docs", http.NoBody)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			assert.Equal(t, tt.want, cosmosdb.IsQueryRequest(req))
		})
	}
}

// TestHandler_ValidateAuthEnforcement covers the opt-in master-key
// enforcement path. The flag deliberately behaves like services/s3's
// PresignSecret/WithPresignValidation (the precedent AZURE.md section 5
// names): OFF by default and fully permissive, but when ON a *present*
// Authorization header must verify or the request is rejected -- an opt-in
// flag named "validate" that only logged would be actively misleading.
// An absent header stays anonymous-accepted either way, so enabling the
// flag can never break the no-credentials local-dev workflow.
//
// The signed case reuses masterkey_test.go's hand-computed known-answer
// vector (independently reproducible: base64(HMAC-SHA256(base64decode(
// DefaultMasterKey), "get\ndbs\ndbs/mydb\nthu, 01 jan 1970 00:00:00 gmt\n\n"))).
// That database does not exist, so a passing signature yields 404, not 200 --
// the assertion is precisely "not 401", which is what distinguishes an
// auth rejection from ordinary resource handling.
func TestHandler_ValidateAuthEnforcement(t *testing.T) {
	t.Parallel()

	const goodSig = "0UrOUjNuyWU/2xulf8ZyCV7Yf/Yr0BeqSlr7CJyEWhI="

	tests := []struct {
		name         string
		authHeader   string
		validateAuth bool
		wantStatus   int
	}{
		{
			name:       "permissive by default accepts a garbage signature",
			authHeader: "type=master&ver=1.0&sig=not-a-real-signature",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "permissive by default accepts a structurally malformed header",
			authHeader: "total-nonsense",
			wantStatus: http.StatusNotFound,
		},
		{
			name:         "validate on rejects a garbage signature",
			authHeader:   "type=master&ver=1.0&sig=not-a-real-signature",
			validateAuth: true,
			wantStatus:   http.StatusUnauthorized,
		},
		{
			name:         "validate on rejects a structurally malformed header",
			authHeader:   "total-nonsense",
			validateAuth: true,
			wantStatus:   http.StatusUnauthorized,
		},
		{
			name:         "validate on still accepts an anonymous request",
			authHeader:   "",
			validateAuth: true,
			wantStatus:   http.StatusNotFound,
		},
		{
			name:         "validate on accepts a correctly signed request",
			authHeader:   "type=master&ver=1.0&sig=" + goodSig,
			validateAuth: true,
			wantStatus:   http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			h.ValidateAuth = tt.validateAuth

			headers := map[string]string{"X-Ms-Date": "Thu, 01 Jan 1970 00:00:00 GMT"}
			if tt.authHeader != "" {
				headers["Authorization"] = url.QueryEscape(tt.authHeader)
			}

			rec := doRequest(t, h, http.MethodGet, "/dbs/mydb", headers, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
