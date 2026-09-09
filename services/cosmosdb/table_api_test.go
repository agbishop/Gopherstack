package cosmosdb_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandler_TableAPI_UnknownTableIs404 covers the routing decision
// table_api.go's isTableAPIPath makes: a single path segment that isn't
// "dbs" is Table API territory (a query against a table that doesn't exist),
// not an invalid Core/SQL URI.
func TestHandler_TableAPI_UnknownTableIs404(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodGet, "/notatable", nil, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "TableNotFound", rec.Header().Get("X-Ms-Error-Code"))
}

func TestHandler_TableAPI_TableLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// CreateTable.
	rec := doRequest(t, h, http.MethodPost, "/Tables", nil, []byte(`{"TableName":"mytable"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	body := decodeBody(t, rec)
	assert.Equal(t, "mytable", body["TableName"])

	// Duplicate create -> 409.
	rec = doRequest(t, h, http.MethodPost, "/Tables", nil, []byte(`{"TableName":"mytable"}`))
	assert.Equal(t, http.StatusConflict, rec.Code)

	// ListTables.
	rec = doRequest(t, h, http.MethodGet, "/Tables", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	list := decodeBody(t, rec)
	values, ok := list["value"].([]any)
	require.True(t, ok)
	require.Len(t, values, 1)

	// DeleteTable.
	rec = doRequest(t, h, http.MethodDelete, "/Tables('mytable')", nil, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	rec = doRequest(t, h, http.MethodDelete, "/Tables('mytable')", nil, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_TableAPI_EntityLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/Tables", nil, []byte(`{"TableName":"widgets"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	// InsertEntity.
	insertBody := []byte(`{"PartitionKey":"p","RowKey":"r","Count":42}`)
	rec = doRequest(t, h, http.MethodPost, "/widgets", nil, insertBody)
	require.Equal(t, http.StatusCreated, rec.Code)
	etag := rec.Header().Get("ETag")
	require.NotEmpty(t, etag)

	entity := decodeBody(t, rec)
	assert.InDelta(t, float64(42), entity["Count"], 0)

	// GetEntity.
	rec = doRequest(t, h, http.MethodGet, "/widgets(PartitionKey='p',RowKey='r')", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	got := decodeBody(t, rec)
	assert.Equal(t, "p", got["PartitionKey"])
	assert.Equal(t, "r", got["RowKey"])

	// QueryEntities with $filter.
	rec = doRequest(t, h, http.MethodGet, "/widgets()?$filter="+url.QueryEscape("Count eq 42"), nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	list := decodeBody(t, rec)
	values, ok := list["value"].([]any)
	require.True(t, ok)
	assert.Len(t, values, 1)

	// MergeEntity: only Name is set, Count survives.
	mergeBody := []byte(`{"PartitionKey":"p","RowKey":"r","Name":"widget"}`)
	rec = doRequest(t, h, http.MethodPatch, "/widgets(PartitionKey='p',RowKey='r')",
		map[string]string{"If-Match": "*"}, mergeBody)
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/widgets(PartitionKey='p',RowKey='r')", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	merged := decodeBody(t, rec)
	assert.Equal(t, "widget", merged["Name"])
	assert.InDelta(t, float64(42), merged["Count"], 0)

	// ReplaceEntity: Count is dropped since it's absent from the new body.
	replaceBody := []byte(`{"PartitionKey":"p","RowKey":"r","Name":"replaced"}`)
	rec = doRequest(t, h, http.MethodPut, "/widgets(PartitionKey='p',RowKey='r')",
		map[string]string{"If-Match": "*"}, replaceBody)
	require.Equal(t, http.StatusNoContent, rec.Code)
	newETag := rec.Header().Get("ETag")
	require.NotEmpty(t, newETag)

	rec = doRequest(t, h, http.MethodGet, "/widgets(PartitionKey='p',RowKey='r')", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	replaced := decodeBody(t, rec)
	assert.Equal(t, "replaced", replaced["Name"])
	_, hasCount := replaced["Count"]
	assert.False(t, hasCount, "replace must drop properties not present in the new body")

	// DeleteEntity with a wrong ETag -> 412.
	rec = doRequest(t, h, http.MethodDelete, "/widgets(PartitionKey='p',RowKey='r')",
		map[string]string{"If-Match": `W/"datetime'bogus'"`}, nil)
	assert.Equal(t, http.StatusPreconditionFailed, rec.Code)

	// DeleteEntity with the correct ETag.
	rec = doRequest(t, h, http.MethodDelete, "/widgets(PartitionKey='p',RowKey='r')",
		map[string]string{"If-Match": newETag}, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify gone.
	rec = doRequest(t, h, http.MethodGet, "/widgets(PartitionKey='p',RowKey='r')", nil, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_TableAPI_Batch_NotImplemented(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/$batch", nil, []byte(""))
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestHandler_TableAPI_Reset_ClearsTableBackend(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/Tables", nil, []byte(`{"TableName":"widgets"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	h.Reset()

	rec = doRequest(t, h, http.MethodGet, "/Tables", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	list := decodeBody(t, rec)
	values, ok := list["value"].([]any)
	require.True(t, ok)
	assert.Empty(t, values)
}
