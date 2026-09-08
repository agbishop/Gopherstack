package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func wafCreateRegexMatchSet(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateRegexMatchSet", map[string]any{
		"ChangeToken": token,
		"Name":        name,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	setMap, ok := resp["RegexMatchSet"].(map[string]any)
	require.True(t, ok)
	id, ok := setMap["RegexMatchSetId"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func TestRegexMatchSetLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{"single regex match set CRUD"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)

			patternSetID := wafCreateRegexPatternSet(t, h, "Patterns")

			// Create
			setID := wafCreateRegexMatchSet(t, h, "SqlMatchSet")
			assert.Equal(t, 1, waf.RegexMatchSetCount(h.Backend.(*waf.InMemoryBackend)))

			// Get
			rec := wafDo(t, h, "GetRegexMatchSet", map[string]any{"RegexMatchSetId": setID})
			require.Equal(t, http.StatusOK, rec.Code)
			var getResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
			setMap := getResp["RegexMatchSet"].(map[string]any)
			assert.Equal(t, "SqlMatchSet", setMap["Name"])

			// Update: insert tuple
			token := wafGetToken(t, h)
			rec = wafDo(t, h, "UpdateRegexMatchSet", map[string]any{
				"ChangeToken":     token,
				"RegexMatchSetId": setID,
				"Updates": []map[string]any{
					{
						"Action": "INSERT",
						"RegexMatchTuple": map[string]any{
							"FieldToMatch":       map[string]any{"Type": "QUERY_STRING"},
							"TextTransformation": "URL_DECODE",
							"RegexPatternSetId":  patternSetID,
						},
					},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = wafDo(t, h, "GetRegexMatchSet", map[string]any{"RegexMatchSetId": setID})
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
			setMap = getResp["RegexMatchSet"].(map[string]any)
			tuples := setMap["RegexMatchTuples"].([]any)
			assert.Len(t, tuples, 1)

			// Delete tuple
			token = wafGetToken(t, h)
			rec = wafDo(t, h, "UpdateRegexMatchSet", map[string]any{
				"ChangeToken":     token,
				"RegexMatchSetId": setID,
				"Updates": []map[string]any{
					{
						"Action": "DELETE",
						"RegexMatchTuple": map[string]any{
							"FieldToMatch":       map[string]any{"Type": "QUERY_STRING"},
							"TextTransformation": "URL_DECODE",
							"RegexPatternSetId":  patternSetID,
						},
					},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			// List
			rec = wafDo(t, h, "ListRegexMatchSets", nil)
			require.Equal(t, http.StatusOK, rec.Code)
			var listResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
			sets := listResp["RegexMatchSets"].([]any)
			assert.Len(t, sets, 1)

			// Delete set
			token = wafGetToken(t, h)
			rec = wafDo(t, h, "DeleteRegexMatchSet", map[string]any{
				"ChangeToken":     token,
				"RegexMatchSetId": setID,
			})
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, 0, waf.RegexMatchSetCount(h.Backend.(*waf.InMemoryBackend)))
		})
	}
}

func TestRegexMatchSet_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		patternSetID := wafCreateRegexPatternSet(t, h, "noop-insert-patterns")
		setID := wafCreateRegexMatchSet(t, h, "noop-insert-match")
		tuple := map[string]any{
			"FieldToMatch":       map[string]any{"Type": "QUERY_STRING"},
			"TextTransformation": "URL_DECODE",
			"RegexPatternSetId":  patternSetID,
		}

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateRegexMatchSet", map[string]any{
			"ChangeToken":     token,
			"RegexMatchSetId": setID,
			"Updates":         []map[string]any{{"Action": "INSERT", "RegexMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateRegexMatchSet", map[string]any{
			"ChangeToken":     token,
			"RegexMatchSetId": setID,
			"Updates":         []map[string]any{{"Action": "INSERT", "RegexMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		patternSetID := wafCreateRegexPatternSet(t, h, "noop-delete-patterns")
		setID := wafCreateRegexMatchSet(t, h, "noop-delete-match")
		tuple := map[string]any{
			"FieldToMatch":       map[string]any{"Type": "QUERY_STRING"},
			"TextTransformation": "URL_DECODE",
			"RegexPatternSetId":  patternSetID,
		}

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateRegexMatchSet", map[string]any{
			"ChangeToken":     token,
			"RegexMatchSetId": setID,
			"Updates":         []map[string]any{{"Action": "DELETE", "RegexMatchTuple": tuple}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestRegexMatchSetNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "GetRegexMatchSet not-found",
			action: "GetRegexMatchSet",
			body:   map[string]any{"RegexMatchSetId": "no-such-id"},
		},
		{
			name:   "DeleteRegexMatchSet not-found",
			action: "DeleteRegexMatchSet",
			body:   map[string]any{"ChangeToken": "t", "RegexMatchSetId": "no-such-id"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)
			rec := wafDo(t, h, tc.action, tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}
