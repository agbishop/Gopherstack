package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func TestWAF_ByteMatchSet_CreateGetUpdateDeleteList(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateByteMatchSet", map[string]any{
		"ChangeToken": token,
		"Name":        "my-bms",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	bmsMap := createResp["ByteMatchSet"].(map[string]any)
	id := bmsMap["ByteMatchSetId"].(string)
	require.NotEmpty(t, id)
	assert.Equal(t, 1, waf.ByteMatchSetCount(h.Backend.(*waf.InMemoryBackend)))

	// Get
	rec = wafDo(t, h, "GetByteMatchSet", map[string]any{"ByteMatchSetId": id})
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	bms := resp["ByteMatchSet"].(map[string]any)
	assert.Equal(t, "my-bms", bms["Name"])

	// Update: insert tuple
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateByteMatchSet", map[string]any{
		"ChangeToken":    token,
		"ByteMatchSetId": id,
		"Updates": []map[string]any{
			{
				"Action": "INSERT",
				"ByteMatchTuple": map[string]any{
					"FieldToMatch":         map[string]any{"Type": "URI"},
					"TargetString":         "/admin",
					"PositionalConstraint": "STARTS_WITH",
					"TextTransformation":   "NONE",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = wafDo(t, h, "GetByteMatchSet", map[string]any{"ByteMatchSetId": id})
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	bms = resp["ByteMatchSet"].(map[string]any)
	tuples := bms["ByteMatchTuples"].([]any)
	require.Len(t, tuples, 1)
	tuple := tuples[0].(map[string]any)
	assert.Equal(t, "/admin", tuple["TargetString"])
	assert.Equal(t, "STARTS_WITH", tuple["PositionalConstraint"])

	// List
	rec = wafDo(t, h, "ListByteMatchSets", map[string]any{"Limit": 100})
	require.Equal(t, http.StatusOK, rec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	sets := listResp["ByteMatchSets"].([]any)
	assert.Len(t, sets, 1)

	// Delete while non-empty must fail with WAFNonEmptyEntityException.
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteByteMatchSet", map[string]any{
		"ChangeToken":    token,
		"ByteMatchSetId": id,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "WAFNonEmptyEntityException", errType(t, rec.Body.Bytes()))

	// Remove the tuple, then delete succeeds.
	token = wafGetToken(t, h)
	rec = wafDo(t, h, "UpdateByteMatchSet", map[string]any{
		"ChangeToken":    token,
		"ByteMatchSetId": id,
		"Updates": []map[string]any{
			{
				"Action": "DELETE",
				"ByteMatchTuple": map[string]any{
					"FieldToMatch":         map[string]any{"Type": "URI"},
					"TargetString":         "/admin",
					"PositionalConstraint": "STARTS_WITH",
					"TextTransformation":   "NONE",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	token = wafGetToken(t, h)
	rec = wafDo(t, h, "DeleteByteMatchSet", map[string]any{
		"ChangeToken":    token,
		"ByteMatchSetId": id,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, waf.ByteMatchSetCount(h.Backend.(*waf.InMemoryBackend)))
}

func wafCreateByteMatchSet(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateByteMatchSet", map[string]any{"ChangeToken": token, "Name": name})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	m := resp["ByteMatchSet"].(map[string]any)
	id := m["ByteMatchSetId"].(string)
	require.NotEmpty(t, id)

	return id
}

func TestWAF_ByteMatchSet_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	tuple := map[string]any{
		"FieldToMatch":         map[string]any{"Type": "URI"},
		"TargetString":         "/admin",
		"PositionalConstraint": "STARTS_WITH",
		"TextTransformation":   "NONE",
	}

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateByteMatchSet(t, h, "noop-insert-bms")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateByteMatchSet", map[string]any{
			"ChangeToken":    token,
			"ByteMatchSetId": id,
			"Updates":        []map[string]any{{"Action": "INSERT", "ByteMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateByteMatchSet", map[string]any{
			"ChangeToken":    token,
			"ByteMatchSetId": id,
			"Updates":        []map[string]any{{"Action": "INSERT", "ByteMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateByteMatchSet(t, h, "noop-delete-bms")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateByteMatchSet", map[string]any{
			"ChangeToken":    token,
			"ByteMatchSetId": id,
			"Updates":        []map[string]any{{"Action": "DELETE", "ByteMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestWAF_ByteMatchSet_NotFound(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	rec := wafDo(t, h, "GetByteMatchSet", map[string]any{"ByteMatchSetId": "nope"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
