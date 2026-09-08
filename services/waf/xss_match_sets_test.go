package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func TestWAF_XssMatchSet_CreateGetUpdateDeleteList(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateXssMatchSet", map[string]any{
		"ChangeToken": token,
		"Name":        "my-xss",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	xmsMap := createResp["XssMatchSet"].(map[string]any)
	id := xmsMap["XssMatchSetId"].(string)
	require.NotEmpty(t, id)

	// Update
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateXssMatchSet", map[string]any{
		"ChangeToken":   token,
		"XssMatchSetId": id,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"XssMatchTuple": map[string]any{
					"FieldToMatch":       map[string]any{"Type": "BODY"},
					"TextTransformation": "HTML_ENTITY_DECODE",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetXssMatchSet", map[string]any{"XssMatchSetId": id})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	xms := resp["XssMatchSet"].(map[string]any)
	tuples := xms["XssMatchTuples"].([]any)
	require.Len(t, tuples, 1)
	assert.Equal(t, "HTML_ENTITY_DECODE", tuples[0].(map[string]any)["TextTransformation"])

	// List
	rec = wafDo(t, h, "ListXssMatchSets", map[string]any{"Limit": 100})
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	sets := listResp["XssMatchSets"].([]any)
	assert.Len(t, sets, 1)

	// Delete while non-empty must fail with WAFNonEmptyEntityException.
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteXssMatchSet", map[string]any{
		"ChangeToken":   token,
		"XssMatchSetId": id,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "WAFNonEmptyEntityException", errType(t, rec.Body.Bytes()))

	// Remove the tuple, then delete succeeds.
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateXssMatchSet", map[string]any{
		"ChangeToken":   token,
		"XssMatchSetId": id,
		"Updates": []map[string]any{
			{
				"Action": "DELETE",
				"XssMatchTuple": map[string]any{
					"FieldToMatch":       map[string]any{"Type": "BODY"},
					"TextTransformation": "HTML_ENTITY_DECODE",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	token = wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteXssMatchSet", map[string]any{
		"ChangeToken":   token,
		"XssMatchSetId": id,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, waf.XssMatchSetCount(h.Backend.(*waf.InMemoryBackend)))
}

func wafCreateXSSMatchSet(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateXssMatchSet", map[string]any{"ChangeToken": token, "Name": name})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	m := resp["XssMatchSet"].(map[string]any)
	id := m["XssMatchSetId"].(string)
	require.NotEmpty(t, id)

	return id
}

func TestWAF_XssMatchSet_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	tuple := map[string]any{
		"FieldToMatch":       map[string]any{"Type": "BODY"},
		"TextTransformation": "HTML_ENTITY_DECODE",
	}

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateXSSMatchSet(t, h, "noop-insert-xss")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateXssMatchSet", map[string]any{
			"ChangeToken":   token,
			"XssMatchSetId": id,
			"Updates":       []map[string]any{{"Action": "INSERT", "XssMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateXssMatchSet", map[string]any{
			"ChangeToken":   token,
			"XssMatchSetId": id,
			"Updates":       []map[string]any{{"Action": "INSERT", "XssMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateXSSMatchSet(t, h, "noop-delete-xss")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateXssMatchSet", map[string]any{
			"ChangeToken":   token,
			"XssMatchSetId": id,
			"Updates":       []map[string]any{{"Action": "DELETE", "XssMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestWAF_XssMatchSet_NotFound(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	rec := wafDo(t, h, "GetXssMatchSet", map[string]any{"XssMatchSetId": "nope"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
