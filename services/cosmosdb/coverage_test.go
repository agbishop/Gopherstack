package cosmosdb_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cosmosdb"
)

// reserveEphemeralPort binds and holds a real TCP port until the test ends.
func reserveEphemeralPort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })

	addr, ok := l.Addr().(*net.TCPAddr)
	require.True(t, ok)

	return addr.Port
}

func TestStartWorker_BindsAndServes(t *testing.T) {
	t.Parallel()

	backend := cosmosdb.NewInMemoryBackend()
	h := cosmosdb.NewHandler(backend)

	// Port 0 lets the OS assign a free ephemeral port directly inside
	// StartWorker's own net.Listen call -- no separate "bind :0, read the
	// port, close it, then tell StartWorker to bind that same number"
	// helper is needed (that pattern has a real release-then-rebind race:
	// another process can grab the port in the gap). StartWorker reflects
	// the actual bound port back onto h.Port before returning, so it's
	// available here with zero race window.
	ctx := t.Context()
	require.NoError(t, h.StartWorker(ctx))

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		h.Shutdown(shutdownCtx)
	})

	url := fmt.Sprintf("http://127.0.0.1:%d/dbs", h.Port)

	require.Eventually(t, func() bool {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if reqErr != nil {
			return false
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()

		return resp.StatusCode == http.StatusOK
	}, 2*time.Second, 10*time.Millisecond, "dedicated listener should become reachable")
}

func TestStartWorker_BindFailureIsSynchronous(t *testing.T) {
	t.Parallel()

	port := reserveEphemeralPort(t)

	h := cosmosdb.NewHandler(cosmosdb.NewInMemoryBackend())
	h.Port = port

	err := h.StartWorker(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bind port")
}

func TestShutdown_NilServerIsNoop(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	assert.NotPanics(t, func() {
		h.Shutdown(t.Context())
	})
}

func TestShutdown_ForcesCloseOnGracefulTimeout(t *testing.T) {
	t.Parallel()

	h := cosmosdb.NewHandler(cosmosdb.NewInMemoryBackend())
	h.Port = 0

	require.NoError(t, h.StartWorker(t.Context()))

	expiredCtx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()

	assert.NotPanics(t, func() {
		h.Shutdown(expiredCtx)
	})

	assert.NotPanics(t, func() {
		h.Shutdown(t.Context())
	})
}

func TestHandler_MetadataMethods(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	assert.Equal(t, "CosmosDB", h.Name())
	assert.NotEmpty(t, h.GetSupportedOperations())
	assert.Equal(t, 0, h.MatchPriority())

	matcher := h.RouteMatcher()
	assert.NotNil(t, matcher)
}

func TestHandler_ExtractOperationAndResource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestDatabase(t, h)
	createTestContainer(t, h)

	tests := []struct {
		name         string
		method       string
		path         string
		headers      map[string]string
		wantOp       string
		wantResource string
	}{
		{name: "list databases", method: http.MethodGet, path: "/dbs", wantOp: "ListDatabases", wantResource: "dbs"},
		{name: "create database", method: http.MethodPost, path: "/dbs", wantOp: "CreateDatabase", wantResource: "dbs"},
		{
			name: "get database", method: http.MethodGet, path: "/dbs/mydb",
			wantOp: "GetDatabase", wantResource: "dbs/mydb",
		},
		{
			name: "delete database", method: http.MethodDelete, path: "/dbs/mydb",
			wantOp: "DeleteDatabase", wantResource: "dbs/mydb",
		},
		{
			name: "list containers", method: http.MethodGet, path: "/dbs/mydb/colls",
			wantOp: "ListContainers", wantResource: "dbs/mydb/colls",
		},
		{
			name: "create container", method: http.MethodPost, path: "/dbs/mydb/colls",
			wantOp: "CreateContainer", wantResource: "dbs/mydb/colls",
		},
		{
			name: "get container", method: http.MethodGet, path: "/dbs/mydb/colls/mycoll",
			wantOp: "GetContainer", wantResource: "dbs/mydb/colls/mycoll",
		},
		{
			name: "delete container", method: http.MethodDelete, path: "/dbs/mydb/colls/mycoll",
			wantOp: "DeleteContainer", wantResource: "dbs/mydb/colls/mycoll",
		},
		{
			name: "list documents", method: http.MethodGet, path: "/dbs/mydb/colls/mycoll/docs",
			wantOp: "ListDocuments", wantResource: "dbs/mydb/colls/mycoll/docs",
		},
		{
			name: "create document", method: http.MethodPost, path: "/dbs/mydb/colls/mycoll/docs",
			wantOp: "CreateDocument", wantResource: "dbs/mydb/colls/mycoll/docs",
		},
		{
			name: "query document", method: http.MethodPost, path: "/dbs/mydb/colls/mycoll/docs",
			headers: map[string]string{"X-Ms-Documentdb-Isquery": "true"},
			wantOp:  "QueryDocuments", wantResource: "dbs/mydb/colls/mycoll/docs",
		},
		{
			name: "get document", method: http.MethodGet, path: "/dbs/mydb/colls/mycoll/docs/doc1",
			wantOp: "GetDocument", wantResource: "dbs/mydb/colls/mycoll/docs/doc1",
		},
		{
			name: "replace document", method: http.MethodPut, path: "/dbs/mydb/colls/mycoll/docs/doc1",
			wantOp: "ReplaceDocument", wantResource: "dbs/mydb/colls/mycoll/docs/doc1",
		},
		{
			name: "delete document", method: http.MethodDelete, path: "/dbs/mydb/colls/mycoll/docs/doc1",
			wantOp: "DeleteDocument", wantResource: "dbs/mydb/colls/mycoll/docs/doc1",
		},
		{name: "unknown", method: http.MethodPatch, path: "/dbs", wantOp: "Unknown", wantResource: "dbs"},
		{
			name: "account root", method: http.MethodGet, path: "/",
			wantOp: "GetDatabaseAccount", wantResource: "",
		},
		// "/foo" is a single path segment, not "dbs" -- Table API territory
		// (see table_api.go's isTableAPIPath), not an invalid Core/SQL path
		// anymore: GET against a table-shaped entity collection is a query.
		{name: "table api path", method: http.MethodGet, path: "/foo", wantOp: "QueryEntities", wantResource: "foo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), tt.method, tt.path, http.NoBody)
			require.NoError(t, err)

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			e := newEchoContextForRequest(t, req)
			assert.Equal(t, tt.wantOp, h.ExtractOperation(e))
			assert.Equal(t, tt.wantResource, h.ExtractResource(e))
		})
	}
}
