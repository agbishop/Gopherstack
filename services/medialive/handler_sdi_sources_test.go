package medialive_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/medialive"
)

func TestSdiSource_CRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/prod/sdiSources", map[string]any{
		"name": "sdi-1", "type": "SINGLE", "mode": "QUADRANT",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	created := decodeBody(t, rec.Body.Bytes())["sdiSource"].(map[string]any)
	id := created["id"].(string)
	assert.Equal(t, "IDLE", created["state"])

	rec = doRequest(t, h, http.MethodGet, "/prod/sdiSources/"+id, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodPut, "/prod/sdiSources/"+id, map[string]any{"name": "sdi-upd"})
	require.Equal(t, http.StatusOK, rec.Code)
	got := decodeBody(t, rec.Body.Bytes())["sdiSource"].(map[string]any)
	assert.Equal(t, "sdi-upd", got["name"])

	rec = doRequest(t, h, http.MethodGet, "/prod/sdiSources", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Len(t, decodeBody(t, rec.Body.Bytes())["sdiSources"].([]any), 1)

	rec = doRequest(t, h, http.MethodDelete, "/prod/sdiSources/"+id, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/prod/sdiSources/"+id, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSdiSource_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		method   string
		path     string
		wantCode int
	}{
		{
			name: "create without name", method: http.MethodPost, path: "/prod/sdiSources",
			body: map[string]any{}, wantCode: http.StatusBadRequest,
		},
		{
			name: "describe missing", method: http.MethodGet,
			path: "/prod/sdiSources/missing", wantCode: http.StatusNotFound,
		},
		{
			name: "delete missing", method: http.MethodDelete,
			path: "/prod/sdiSources/missing", wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rec := doRequest(t, h, tc.method, tc.path, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

// TestSdiSource_DeleteRejectsWhenAttachedToInput locks in a fix for
// gopherstack-1um: DeleteSdiSource had no attachment guard at all, so an
// SdiSource attached to an input could be deleted -- real DeleteSdiSource
// requires it not be attached to any input first (api_op_DeleteSdiSource.go:
// "The SdiSource must not be part of any SidSourceMapping and must not be
// attached to any input."). Gopherstack's CreateInput/UpdateInput don't yet
// wire the real SdiSources request field, so the attachment is forced
// directly here.
func TestSdiSource_DeleteRejectsWhenAttachedToInput(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/prod/sdiSources", map[string]any{
		"name": "sdi-attached", "type": "SINGLE", "mode": "QUADRANT",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	id := decodeBody(t, rec.Body.Bytes())["sdiSource"].(map[string]any)["id"].(string)

	medialive.ForceSdiSourceInputs(h.Backend.(*medialive.InMemoryBackend), id, []string{"input-1"})

	rec = doRequest(t, h, http.MethodDelete, "/prod/sdiSources/"+id, nil)
	assert.Equal(t, http.StatusConflict, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/prod/sdiSources/"+id, nil)
	assert.Equal(t, http.StatusOK, rec.Code, "sdiSource must still exist after the rejected delete")

	// Detaching allows delete to proceed.
	medialive.ForceSdiSourceInputs(h.Backend.(*medialive.InMemoryBackend), id, nil)

	rec = doRequest(t, h, http.MethodDelete, "/prod/sdiSources/"+id, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}
