package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func wafCreateRegexPatternSet(t *testing.T, h *waf.Handler, name string) string {
	t.Helper()

	token := wafGetToken(t, h)
	rec := wafDo(t, h, "CreateRegexPatternSet", map[string]any{
		"ChangeToken": token,
		"Name":        name,
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	setMap, ok := resp["RegexPatternSet"].(map[string]any)
	require.True(t, ok)
	id, ok := setMap["RegexPatternSetId"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func TestRegexPatternSetLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{"single regex pattern set CRUD"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)

			// Create
			setID := wafCreateRegexPatternSet(t, h, "SqlPatterns")
			assert.Equal(t, 1, waf.RegexPatternSetCount(h.Backend.(*waf.InMemoryBackend)))

			// Get
			rec := wafDo(t, h, "GetRegexPatternSet", map[string]any{"RegexPatternSetId": setID})
			require.Equal(t, http.StatusOK, rec.Code)
			var getResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
			setMap := getResp["RegexPatternSet"].(map[string]any)
			assert.Equal(t, "SqlPatterns", setMap["Name"])
			assert.Empty(t, setMap["RegexPatternStrings"])

			// Update: insert patterns
			token := wafGetToken(t, h)
			rec = wafDo(t, h, "UpdateRegexPatternSet", map[string]any{
				"ChangeToken":       token,
				"RegexPatternSetId": setID,
				"Updates": []map[string]any{
					{"Action": "INSERT", "RegexPatternString": "(?i)select"},
					{"Action": "INSERT", "RegexPatternString": "(?i)union"},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = wafDo(t, h, "GetRegexPatternSet", map[string]any{"RegexPatternSetId": setID})
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
			setMap = getResp["RegexPatternSet"].(map[string]any)
			assert.Len(t, setMap["RegexPatternStrings"], 2)

			// Delete one pattern
			token = wafGetToken(t, h)
			rec = wafDo(t, h, "UpdateRegexPatternSet", map[string]any{
				"ChangeToken":       token,
				"RegexPatternSetId": setID,
				"Updates": []map[string]any{
					{"Action": "DELETE", "RegexPatternString": "(?i)select"},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = wafDo(t, h, "GetRegexPatternSet", map[string]any{"RegexPatternSetId": setID})
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
			setMap = getResp["RegexPatternSet"].(map[string]any)
			assert.Len(t, setMap["RegexPatternStrings"], 1)

			// List
			rec = wafDo(t, h, "ListRegexPatternSets", nil)
			require.Equal(t, http.StatusOK, rec.Code)
			var listResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
			sets := listResp["RegexPatternSets"].([]any)
			assert.Len(t, sets, 1)

			// Delete set while non-empty must fail with WAFNonEmptyEntityException.
			token = wafGetToken(t, h)
			rec = wafDo(t, h, "DeleteRegexPatternSet", map[string]any{
				"ChangeToken":       token,
				"RegexPatternSetId": setID,
			})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "WAFNonEmptyEntityException", errType(t, rec.Body.Bytes()))

			// Remove the remaining pattern, then delete succeeds.
			token = wafGetToken(t, h)
			rec = wafDo(t, h, "UpdateRegexPatternSet", map[string]any{
				"ChangeToken":       token,
				"RegexPatternSetId": setID,
				"Updates": []map[string]any{
					{"Action": "DELETE", "RegexPatternString": "(?i)union"},
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			token = wafGetToken(t, h)
			rec = wafDo(t, h, "DeleteRegexPatternSet", map[string]any{
				"ChangeToken":       token,
				"RegexPatternSetId": setID,
			})
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, 0, waf.RegexPatternSetCount(h.Backend.(*waf.InMemoryBackend)))
		})
	}
}

func TestRegexPatternSet_NoOpUpdatesRejected(t *testing.T) {
	t.Parallel()

	t.Run("insert_duplicate_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateRegexPatternSet(t, h, "noop-insert-pattern")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateRegexPatternSet", map[string]any{
			"ChangeToken":       token,
			"RegexPatternSetId": id,
			"Updates":           []map[string]any{{"Action": "INSERT", "RegexPatternString": "(?i)select"}},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		token = wafGetToken(t, h)
		rec = wafDo(t, h, "UpdateRegexPatternSet", map[string]any{
			"ChangeToken":       token,
			"RegexPatternSetId": id,
			"Updates":           []map[string]any{{"Action": "INSERT", "RegexPatternString": "(?i)select"}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})

	t.Run("delete_missing_rejected", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)
		id := wafCreateRegexPatternSet(t, h, "noop-delete-pattern")

		token := wafGetToken(t, h)
		rec := wafDo(t, h, "UpdateRegexPatternSet", map[string]any{
			"ChangeToken":       token,
			"RegexPatternSetId": id,
			"Updates":           []map[string]any{{"Action": "DELETE", "RegexPatternString": "(?i)select"}},
		})
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "WAFInvalidOperationException", errType(t, rec.Body.Bytes()))
	})
}

func TestRegexPatternSetNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "GetRegexPatternSet not-found",
			action: "GetRegexPatternSet",
			body:   map[string]any{"RegexPatternSetId": "no-such-id"},
		},
		{
			name:   "DeleteRegexPatternSet not-found",
			action: "DeleteRegexPatternSet",
			body:   map[string]any{"ChangeToken": "t", "RegexPatternSetId": "no-such-id"},
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
