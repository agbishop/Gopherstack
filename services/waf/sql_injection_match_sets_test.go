package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func TestWAF_SqlInjectionMatchSet_CreateGetUpdateDeleteList(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateSqlInjectionMatchSet", map[string]any{
		"ChangeToken": token,
		"Name":        "my-sqli",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	simsMap := createResp["SqlInjectionMatchSet"].(map[string]any)
	id := simsMap["SqlInjectionMatchSetId"].(string)
	require.NotEmpty(t, id)

	// Update
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateSqlInjectionMatchSet", map[string]any{
		"ChangeToken":            token,
		"SqlInjectionMatchSetId": id,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"SqlInjectionMatchTuple": map[string]any{
					"FieldToMatch":       map[string]any{"Type": "QUERY_STRING"},
					"TextTransformation": "URL_DECODE",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetSqlInjectionMatchSet", map[string]any{"SqlInjectionMatchSetId": id})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	sims := resp["SqlInjectionMatchSet"].(map[string]any)
	tuples := sims["SqlInjectionMatchTuples"].([]any)
	require.Len(t, tuples, 1)
	tuple := tuples[0].(map[string]any)
	assert.Equal(t, "URL_DECODE", tuple["TextTransformation"])
	fm := tuple["FieldToMatch"].(map[string]any)
	assert.Equal(t, "QUERY_STRING", fm["Type"])

	// List
	rec = wafDo(t, h, "ListSqlInjectionMatchSets", map[string]any{"Limit": 100})
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	sets := listResp["SqlInjectionMatchSets"].([]any)
	assert.Len(t, sets, 1)

	// Delete while non-empty must fail with WAFNonEmptyEntityException.
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteSqlInjectionMatchSet", map[string]any{
		"ChangeToken":            token,
		"SqlInjectionMatchSetId": id,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "WAFNonEmptyEntityException", errType(t, rec.Body.Bytes()))

	// Remove the tuple, then delete succeeds.
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateSqlInjectionMatchSet", map[string]any{
		"ChangeToken":            token,
		"SqlInjectionMatchSetId": id,
		"Updates": []map[string]any{
			{
				"Action": "DELETE",
				"SqlInjectionMatchTuple": map[string]any{
					"FieldToMatch":       map[string]any{"Type": "QUERY_STRING"},
					"TextTransformation": "URL_DECODE",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	token = wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteSqlInjectionMatchSet", map[string]any{
		"ChangeToken":            token,
		"SqlInjectionMatchSetId": id,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, waf.SqlInjectionMatchSetCount(h.Backend.(*waf.InMemoryBackend)))
}

func wafCreateSQLInjectionMatchSet(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateSqlInjectionMatchSet", map[string]any{"ChangeToken": token, "Name": name})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	m := resp["SqlInjectionMatchSet"].(map[string]any)
	id := m["SqlInjectionMatchSetId"].(string)
	require.NotEmpty(t, id)

	return id
}

func TestWAF_SqlInjectionMatchSet_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	tuple := map[string]any{
		"FieldToMatch":       map[string]any{"Type": "QUERY_STRING"},
		"TextTransformation": "URL_DECODE",
	}

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateSQLInjectionMatchSet(t, h, "noop-insert-sqli")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateSqlInjectionMatchSet", map[string]any{
			"ChangeToken":            token,
			"SqlInjectionMatchSetId": id,
			"Updates":                []map[string]any{{"Action": "INSERT", "SqlInjectionMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateSqlInjectionMatchSet", map[string]any{
			"ChangeToken":            token,
			"SqlInjectionMatchSetId": id,
			"Updates":                []map[string]any{{"Action": "INSERT", "SqlInjectionMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateSQLInjectionMatchSet(t, h, "noop-delete-sqli")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateSqlInjectionMatchSet", map[string]any{
			"ChangeToken":            token,
			"SqlInjectionMatchSetId": id,
			"Updates":                []map[string]any{{"Action": "DELETE", "SqlInjectionMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestWAF_SqlInjectionMatchSet_NotFound(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	rec := wafDo(t, h, "GetSqlInjectionMatchSet", map[string]any{"SqlInjectionMatchSetId": "nope"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
