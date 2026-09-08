package wafv2_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/wafv2"
)

func TestHandler_CreateRegexPatternSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
		wantID     bool
	}{
		{
			name: "valid",
			body: map[string]any{
				"Name":                  "my-regex",
				"Scope":                 "REGIONAL",
				"RegularExpressionList": []string{"^foo.*", "bar$"},
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name:       "missing_name",
			body:       map[string]any{"Scope": "REGIONAL"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_scope",
			body:       map[string]any{"Name": "my-regex"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate",
			body: map[string]any{
				"Name":  "dup-regex",
				"Scope": "REGIONAL",
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.name == "duplicate" {
				_, _ = h.Backend.CreateRegexPatternSet(context.Background(), "dup-regex", "REGIONAL", "", nil, nil)
			}

			rec := doWafv2Request(t, h, "CreateRegexPatternSet", tt.body)

			if tt.name == "duplicate" {
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				return
			}

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantID {
				var result map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
				summary, ok := result["Summary"].(map[string]any)
				require.True(t, ok)
				assert.NotEmpty(t, summary["Id"])
				assert.NotEmpty(t, summary["ARN"])
				assert.NotEmpty(t, summary["LockToken"])
			}
		})
	}
}

func TestHandler_DeleteRegexPatternSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*wafv2.Handler) (string, string)
		body       func(id, lockToken string) map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "existing",
			setup: func(h *wafv2.Handler) (string, string) {
				rps, _ := h.Backend.CreateRegexPatternSet(context.Background(), "my-regex", "REGIONAL", "", nil, nil)

				return rps.ID, rps.LockToken
			},
			body: func(id, lockToken string) map[string]any {
				return map[string]any{"Id": id, "Name": "my-regex", "Scope": "REGIONAL", "LockToken": lockToken}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "not_found",
			setup: func(_ *wafv2.Handler) (string, string) {
				return "nonexistent", "tok"
			},
			body: func(id, lockToken string) map[string]any {
				return map[string]any{"Id": id, "Name": "x", "Scope": "REGIONAL", "LockToken": lockToken}
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_id",
			setup: func(_ *wafv2.Handler) (string, string) {
				return "", "tok"
			},
			body: func(_, lockToken string) map[string]any {
				return map[string]any{"Name": "x", "Scope": "REGIONAL", "LockToken": lockToken}
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id, lockToken := tt.setup(h)
			rec := doWafv2Request(t, h, "DeleteRegexPatternSet", tt.body(id, lockToken))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestBackend_RegexPatternSetARN(t *testing.T) {
	t.Parallel()

	b := wafv2.NewInMemoryBackend("123456789012", "us-east-1")
	tests := []struct {
		name     string
		wantPart string
		scope    string
	}{
		{name: "regional_scope", scope: "REGIONAL", wantPart: "regional"},
		{name: "cloudfront_scope", scope: "CLOUDFRONT", wantPart: "global"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			arnStr := b.RegexPatternSetARN("my-regex", "myid", tt.scope)
			assert.Contains(t, arnStr, tt.wantPart)
			assert.Contains(t, arnStr, "regexpatternset")
		})
	}
}

func TestRegexPatternSetObjectShape(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// AWS object form: [{"RegexString": "..."}]
	recObj := doWafv2Request(t, h, "CreateRegexPatternSet", map[string]any{
		"Name":  "regex-obj",
		"Scope": "REGIONAL",
		"RegularExpressionList": []map[string]any{
			{"RegexString": "^foo"},
			{"RegexString": "bar$"},
		},
	})
	require.Equal(t, http.StatusOK, recObj.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(recObj.Body.Bytes(), &createResp))
	id := createResp["Summary"].(map[string]any)["Id"].(string)

	// Get and verify the entries are returned as objects.
	recGet := doWafv2Request(
		t, h, "GetRegexPatternSet", map[string]any{"Id": id, "Name": "regex-obj", "Scope": "REGIONAL"},
	)
	require.Equal(t, http.StatusOK, recGet.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(recGet.Body.Bytes(), &getResp))

	rps := getResp["RegexPatternSet"].(map[string]any)
	entries, ok := rps["RegularExpressionList"].([]any)
	require.True(t, ok)
	require.Len(t, entries, 2)

	entry := entries[0].(map[string]any)
	assert.Contains(t, entry, "RegexString")

	// Invalid regex should be rejected.
	recBadRegex := doWafv2Request(t, h, "CreateRegexPatternSet", map[string]any{
		"Name":  "bad-regex",
		"Scope": "REGIONAL",
		"RegularExpressionList": []map[string]any{
			{"RegexString": "[invalid"},
		},
	})
	assert.Equal(t, http.StatusBadRequest, recBadRegex.Code)
}

// ---- Gap 12: Full LoggingConfiguration round-trip --------------------------

func TestRegexPatternSet_MaxEntries(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// 11 entries — exceeds the max of 10.
	entries := make([]map[string]any, 11)
	for i := range entries {
		entries[i] = map[string]any{"RegexString": "^foo" + itoa(i)}
	}

	rec := doWafv2Request(t, h, "CreateRegexPatternSet", map[string]any{
		"Name":                  "too-many-regex",
		"Scope":                 "REGIONAL",
		"RegularExpressionList": entries,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "11 regex entries should exceed limit")

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "WAFLimitsExceededException", errResp["__type"])
}

func TestRegexPatternSet_ExactlyMaxEntries(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Exactly 10 entries — should be accepted.
	entries := make([]map[string]any, 10)
	for i := range entries {
		entries[i] = map[string]any{"RegexString": "^pattern" + itoa(i)}
	}

	rec := doWafv2Request(t, h, "CreateRegexPatternSet", map[string]any{
		"Name":                  "exact-max-regex",
		"Scope":                 "REGIONAL",
		"RegularExpressionList": entries,
	})
	assert.Equal(t, http.StatusOK, rec.Code, "exactly 10 regex entries should be accepted: %s", rec.Body.String())
}

// ---- WebACL CustomResponseBodies round-trip ---------------------------------

// createRegexPatternSetHelper creates a regex pattern set with REGIONAL scope and returns its ID.
func createRegexPatternSetHelper(t *testing.T, h *wafv2.Handler, name string) string {
	t.Helper()

	rec := doWafv2Request(t, h, "CreateRegexPatternSet", map[string]any{
		"Name":  name,
		"Scope": "REGIONAL",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	summary, ok := resp["Summary"].(map[string]any)
	require.True(t, ok)

	id, ok := summary["Id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, id)

	return id
}

func TestHandler_GetRegexPatternSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupName  string
		requestID  string
		wantField  string
		wantStatus int
	}{
		{
			name:       "found",
			setupName:  "my-regex",
			wantStatus: http.StatusOK,
			wantField:  "RegexPatternSet",
		},
		{
			name:       "missing_id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not_found",
			requestID:  "nonexistent-id",
			setupName:  "nonexistent-name",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			id := tt.requestID
			if tt.setupName != "" && tt.requestID == "" {
				id = createRegexPatternSetHelper(t, h, tt.setupName)
			}

			var body any
			if id != "" {
				body = map[string]any{"Id": id, "Name": tt.setupName, "Scope": "REGIONAL"}
			} else {
				body = map[string]any{}
			}

			rec := doWafv2Request(t, h, "GetRegexPatternSet", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantField != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Contains(t, resp, tt.wantField)
			}
		})
	}
}

func TestHandler_ListRegexPatternSets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		scope      string
		setup      []string
		wantCount  int
		wantStatus int
	}{
		{
			name:       "empty",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:       "list_all",
			setup:      []string{"rps-a", "rps-b"},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "filter_by_scope",
			setup:      []string{"rps-a", "rps-b"},
			scope:      "REGIONAL",
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "filter_no_match",
			setup:      []string{"rps-a"},
			scope:      "CLOUDFRONT",
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for _, name := range tt.setup {
				createRegexPatternSetHelper(t, h, name)
			}

			rec := doWafv2Request(t, h, "ListRegexPatternSets", map[string]any{"Scope": tt.scope})
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			items, ok := resp["RegexPatternSets"].([]any)
			require.True(t, ok)
			assert.Len(t, items, tt.wantCount)
		})
	}
}

func TestHandler_UpdateRegexPatternSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupName   string
		requestID   string
		description string
		wantStatus  int
	}{
		{
			name:        "update_description",
			setupName:   "my-rps",
			description: "updated description",
			wantStatus:  http.StatusOK,
		},
		{
			name:       "missing_id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not_found",
			requestID:  "nonexistent",
			setupName:  "nonexistent-name",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			id := tt.requestID
			if tt.setupName != "" && tt.requestID == "" {
				id = createRegexPatternSetHelper(t, h, tt.setupName)
			}

			var lockToken string
			if tt.name == "update_description" {
				existing, err := h.Backend.GetRegexPatternSet(context.Background(), id)
				require.NoError(t, err)
				lockToken = existing.LockToken
			}

			var body any
			if id != "" {
				body = map[string]any{
					"Id": id, "Name": tt.setupName, "Scope": "REGIONAL",
					"Description": tt.description, "LockToken": lockToken,
				}
			} else {
				body = map[string]any{}
			}

			rec := doWafv2Request(t, h, "UpdateRegexPatternSet", body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Contains(t, resp, "NextLockToken")
			}
		})
	}
}

func TestHandler_ScopeValidation_CreateRegexPatternSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		scope      string
		wantStatus int
	}{
		{name: "regional_valid", scope: "REGIONAL", wantStatus: http.StatusOK},
		{name: "cloudfront_valid", scope: "CLOUDFRONT", wantStatus: http.StatusOK},
		{name: "invalid_scope", scope: "BAD", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doWafv2Request(t, h, "CreateRegexPatternSet", map[string]any{
				"Name":  "test-rps",
				"Scope": tt.scope,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
