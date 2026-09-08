package rolesanywhere_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandler_ListTrustAnchors_Pagination_DeletedMidPage proves that deleting
// the trust anchor a cursor names does not restart pagination at page one.
// TestHandler_ListTrustAnchors_Pagination only ever exercised page sizes and
// never followed a nextToken into a second page.
func TestHandler_ListTrustAnchors_Pagination_DeletedMidPage(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 5 {
		doREST(t, h, http.MethodPost, "/trustanchors", map[string]any{
			"name":   "anchor-" + string(rune('a'+i)),
			"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
		})
	}

	rec := doREST(t, h, http.MethodGet, "/trustanchors?pageSize=2", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp1 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp1))
	items1, _ := resp1["trustAnchors"].([]any)
	require.Len(t, items1, 2)

	page1IDs := map[string]bool{}
	for _, item := range items1 {
		entry, _ := item.(map[string]any)
		page1IDs[entry["trustAnchorId"].(string)] = true
	}

	nextToken, ok := resp1["nextToken"].(string)
	require.True(t, ok)
	require.NotEmpty(t, nextToken)

	recDel := doREST(t, h, http.MethodDelete, "/trustanchor/"+nextToken, nil)
	require.Equal(t, http.StatusOK, recDel.Code)

	rec = doREST(t, h, http.MethodGet, "/trustanchors?pageSize=2&nextToken="+nextToken, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp2))
	items2, _ := resp2["trustAnchors"].([]any)

	for _, item := range items2 {
		entry, _ := item.(map[string]any)
		id, _ := entry["trustAnchorId"].(string)
		assert.False(t, page1IDs[id], "cursor must not restart pagination at page one after its item is deleted")
	}
}

// TestHandler_ListTrustAnchors_Pagination_TiedNamesTotalOrder proves that
// list ordering (sorted by Name, a non-unique display field) still resolves
// deterministically when two trust anchors share the same Name -- otherwise
// map-iteration order could shuffle tied entries between calls and drop or
// duplicate results across pages.
func TestHandler_ListTrustAnchors_Pagination_TiedNamesTotalOrder(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for range 6 {
		doREST(t, h, http.MethodPost, "/trustanchors", map[string]any{
			"name":   "dup-name",
			"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
		})
	}

	seen := map[string]bool{}
	nextToken := ""

	for range 10 {
		path := "/trustanchors?pageSize=2"
		if nextToken != "" {
			path += "&nextToken=" + nextToken
		}

		rec := doREST(t, h, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		items, _ := resp["trustAnchors"].([]any)

		for _, item := range items {
			entry, _ := item.(map[string]any)
			id, _ := entry["trustAnchorId"].(string)
			assert.False(t, seen[id], "tied Name entries must not repeat across pages")
			seen[id] = true
		}

		nextToken, _ = resp["nextToken"].(string)
		if nextToken == "" {
			break
		}
	}

	assert.Len(t, seen, 6, "every tied-name trust anchor must be visited exactly once")
}
