package macie2_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/macie2"
)

// TestMacie2_Delete_ClearsTags verifies DeleteAllowList and DeleteFindingsFilter
// clear their entry in the tags map. isKnownARN gates TagResource/
// ListTagsForResource by resource existence, so the leak is not reachable
// through those ops post-delete; it is observable instead through the tags
// map's persistence -- it is written into Snapshot() verbatim, so a leaked
// entry grows the persisted file without bound on create/delete churn.
func TestMacie2_Delete_ClearsTags(t *testing.T) {
	t.Parallel()

	t.Run("allow list", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doRequest(t, h, http.MethodPost, "/allow-lists", map[string]any{
			"clientToken": "tok-tag-leak",
			"name":        "tag-leak-list",
			"criteria":    map[string]any{"regex": "test"},
		})
		require.Equal(t, http.StatusOK, rec.Code)
		var created map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
		arnStr, id := created["arn"], created["id"]
		require.NotEmpty(t, arnStr)

		rec = doRequest(t, h, http.MethodPost, "/tags/"+arnStr, map[string]any{"tags": map[string]string{"k": "v"}})
		require.Equal(t, http.StatusOK, rec.Code)

		otherRec := doRequest(t, h, http.MethodPost, "/allow-lists", map[string]any{
			"clientToken": "tok-tag-leak-sibling",
			"name":        "tag-leak-list-sibling",
			"criteria":    map[string]any{"regex": "test"},
		})
		require.Equal(t, http.StatusOK, otherRec.Code)
		var otherCreated map[string]string
		require.NoError(t, json.Unmarshal(otherRec.Body.Bytes(), &otherCreated))
		otherArn := otherCreated["arn"]
		otherRec = doRequest(
			t, h, http.MethodPost, "/tags/"+otherArn, map[string]any{"tags": map[string]string{"k": "v"}},
		)
		require.Equal(t, http.StatusOK, otherRec.Code)

		rec = doRequest(t, h, http.MethodDelete, "/allow-lists/"+id, nil)
		require.Equal(t, http.StatusOK, rec.Code)

		assertMacie2SnapshotTagsCleared(t, h, arnStr)

		// Deleting one allow list must not disturb another's tags.
		rec = doRequest(t, h, http.MethodGet, "/tags/"+otherArn, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		var otherTags map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &otherTags))
		assert.NotEmpty(t, otherTags["tags"])
	})

	t.Run("findings filter", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doRequest(t, h, http.MethodPost, "/findingsfilters", map[string]any{
			"name":            "tag-leak-filter",
			"action":          "ARCHIVE",
			"findingCriteria": map[string]any{"criterion": map[string]any{}},
		})
		require.Equal(t, http.StatusOK, rec.Code)
		var created map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
		arnStr, id := created["arn"], created["id"]
		require.NotEmpty(t, arnStr)

		rec = doRequest(t, h, http.MethodPost, "/tags/"+arnStr, map[string]any{"tags": map[string]string{"k": "v"}})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = doRequest(t, h, http.MethodDelete, "/findingsfilters/"+id, nil)
		require.Equal(t, http.StatusOK, rec.Code)

		assertMacie2SnapshotTagsCleared(t, h, arnStr)
	})
}

func assertMacie2SnapshotTagsCleared(t *testing.T, h *macie2.Handler, arnStr string) {
	t.Helper()

	var probe struct {
		Tags map[string]map[string]string `json:"tags"`
	}
	require.NoError(t, json.Unmarshal(h.Snapshot(t.Context()), &probe))
	assert.NotContains(t, probe.Tags, arnStr)
}
