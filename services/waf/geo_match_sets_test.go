package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func TestWAF_GeoMatchSet_CreateGetUpdateDeleteList(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateGeoMatchSet", map[string]any{
		"ChangeToken": token,
		"Name":        "my-geo",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	gmsMap := createResp["GeoMatchSet"].(map[string]any)
	id := gmsMap["GeoMatchSetId"].(string)
	require.NotEmpty(t, id)

	// Update: insert constraint
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateGeoMatchSet", map[string]any{
		"ChangeToken":   token,
		"GeoMatchSetId": id,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"GeoMatchConstraint": map[string]any{
					"Type":  "Country",
					"Value": "CN",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetGeoMatchSet", map[string]any{"GeoMatchSetId": id})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	gms := resp["GeoMatchSet"].(map[string]any)
	constraints := gms["GeoMatchConstraints"].([]any)
	require.Len(t, constraints, 1)
	c := constraints[0].(map[string]any)
	assert.Equal(t, "Country", c["Type"])
	assert.Equal(t, "CN", c["Value"])

	// Add second country, then delete first
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateGeoMatchSet", map[string]any{
		"ChangeToken":   token,
		"GeoMatchSetId": id,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"GeoMatchConstraint": map[string]any{
					"Type":  "Country",
					"Value": "RU",
				},
			},
			{
				"Action": "DELETE",
				"GeoMatchConstraint": map[string]any{
					"Type":  "Country",
					"Value": "CN",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetGeoMatchSet", map[string]any{"GeoMatchSetId": id})
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	gms = resp["GeoMatchSet"].(map[string]any)
	constraints = gms["GeoMatchConstraints"].([]any)
	require.Len(t, constraints, 1)
	assert.Equal(t, "RU", constraints[0].(map[string]any)["Value"])

	// List
	rec = wafDo(t, h, "ListGeoMatchSets", map[string]any{"Limit": 100})
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	sets := listResp["GeoMatchSets"].([]any)
	assert.Len(t, sets, 1)

	// Delete while non-empty must fail with WAFNonEmptyEntityException.
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteGeoMatchSet", map[string]any{
		"ChangeToken":   token,
		"GeoMatchSetId": id,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "WAFNonEmptyEntityException", errType(t, rec.Body.Bytes()))

	// Remove the remaining constraint, then delete succeeds.
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateGeoMatchSet", map[string]any{
		"ChangeToken":   token,
		"GeoMatchSetId": id,
		"Updates": []map[string]any{
			{
				"Action": "DELETE",
				"GeoMatchConstraint": map[string]any{
					"Type":  "Country",
					"Value": "RU",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	token = wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteGeoMatchSet", map[string]any{
		"ChangeToken":   token,
		"GeoMatchSetId": id,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, waf.GeoMatchSetCount(h.Backend.(*waf.InMemoryBackend)))
}

func wafCreateGeoMatchSet(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateGeoMatchSet", map[string]any{"ChangeToken": token, "Name": name})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	m := resp["GeoMatchSet"].(map[string]any)
	id := m["GeoMatchSetId"].(string)
	require.NotEmpty(t, id)

	return id
}

func TestWAF_GeoMatchSet_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	constraint := map[string]any{"Type": "Country", "Value": "CN"}

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateGeoMatchSet(t, h, "noop-insert-geo")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateGeoMatchSet", map[string]any{
			"ChangeToken":   token,
			"GeoMatchSetId": id,
			"Updates":       []map[string]any{{"Action": "INSERT", "GeoMatchConstraint": constraint}},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateGeoMatchSet", map[string]any{
			"ChangeToken":   token,
			"GeoMatchSetId": id,
			"Updates":       []map[string]any{{"Action": "INSERT", "GeoMatchConstraint": constraint}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateGeoMatchSet(t, h, "noop-delete-geo")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateGeoMatchSet", map[string]any{
			"ChangeToken":   token,
			"GeoMatchSetId": id,
			"Updates":       []map[string]any{{"Action": "DELETE", "GeoMatchConstraint": constraint}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestWAF_GeoMatchSet_NotFound(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	rec := wafDo(t, h, "GetGeoMatchSet", map[string]any{"GeoMatchSetId": "nope"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
