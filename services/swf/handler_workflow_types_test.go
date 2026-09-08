package swf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ListWorkflowTypes_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)

	// Register domain first.
	doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "wf-domain", "description": "test"})

	for _, wt := range []string{"wf-a", "wf-b", "wf-c"} {
		rec := doSWFRequest(t, h, "RegisterWorkflowType", map[string]any{
			"domain":  "wf-domain",
			"name":    wt,
			"version": "1.0",
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	tests := []struct {
		body          map[string]any
		name          string
		wantMinCount  int
		wantNextToken bool
	}{
		{
			name:         "all workflow types",
			body:         map[string]any{"domain": "wf-domain", "registrationStatus": "REGISTERED"},
			wantMinCount: 3,
		},
		{
			name: "paginated maximumPageSize=1",
			body: map[string]any{
				"domain":             "wf-domain",
				"registrationStatus": "REGISTERED",
				"maximumPageSize":    1,
			},
			wantMinCount:  1,
			wantNextToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doSWFRequest(t, h, "ListWorkflowTypes", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseSWFResp(t, rec)

			infos, ok := resp["typeInfos"].([]any)
			require.True(t, ok)
			assert.GreaterOrEqual(t, len(infos), tt.wantMinCount)

			if tt.wantNextToken {
				assert.NotEmpty(t, resp["nextPageToken"])
			}
		})
	}
}

func TestHandler_ListWorkflowTypes_TokenChaining(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)

	doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "chain-domain", "description": "test"})

	for _, wt := range []string{"wf-1", "wf-2", "wf-3"} {
		doSWFRequest(t, h, "RegisterWorkflowType", map[string]any{
			"domain":  "chain-domain",
			"name":    wt,
			"version": "1.0",
		})
	}

	// First page.
	rec := doSWFRequest(t, h, "ListWorkflowTypes", map[string]any{
		"domain":             "chain-domain",
		"registrationStatus": "REGISTERED",
		"maximumPageSize":    2,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	page1 := parseSWFResp(t, rec)

	infos1 := page1["typeInfos"].([]any)
	assert.Len(t, infos1, 2)

	nextToken, ok := page1["nextPageToken"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, nextToken)

	// Second page using the token.
	rec2 := doSWFRequest(t, h, "ListWorkflowTypes", map[string]any{
		"domain":             "chain-domain",
		"registrationStatus": "REGISTERED",
		"maximumPageSize":    2,
		"nextPageToken":      nextToken,
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	page2 := parseSWFResp(t, rec2)

	infos2 := page2["typeInfos"].([]any)
	assert.GreaterOrEqual(t, len(infos2), 1)

	// No duplicates between pages.
	names1 := make(map[string]bool)
	for _, wt := range infos1 {
		wm := wt.(map[string]any)
		wtRef := wm["workflowType"].(map[string]any)
		names1[wtRef["name"].(string)] = true
	}

	for _, wt := range infos2 {
		wm := wt.(map[string]any)
		wtRef := wm["workflowType"].(map[string]any)
		assert.False(t, names1[wtRef["name"].(string)], "duplicate workflow type in page 2")
	}
}

func TestHandler_DescribeWorkflowType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		wantType string
		wantName string
		setup    []setupAction
		wantCode int
	}{
		{
			name: "found_registered",
			setup: []setupAction{
				{action: "RegisterWorkflowType", body: map[string]any{
					"domain": "d1", "name": "wf1", "version": "1.0",
				}},
			},
			body:     map[string]any{"domain": "d1", "workflowType": map[string]any{"name": "wf1", "version": "1.0"}},
			wantCode: http.StatusOK,
			wantType: "REGISTERED",
			wantName: "wf1",
		},
		{
			name: "not_found",
			body: map[string]any{
				"domain":       "d1",
				"workflowType": map[string]any{"name": "missing", "version": "1.0"},
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSWFHandler(t)
			doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "d1"})
			for _, s := range tt.setup {
				doSWFRequest(t, h, s.action, s.body)
			}

			rec := doSWFRequest(t, h, "DescribeWorkflowType", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantName != "" {
				resp := parseSWFResp(t, rec)
				typeInfo, ok := resp["typeInfo"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantType, typeInfo["status"])
				wt := typeInfo["workflowType"].(map[string]any)
				assert.Equal(t, tt.wantName, wt["name"])
			}
		})
	}
}

func TestHandler_DeprecateWorkflowType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		wantType string
		setup    []setupAction
		wantCode int
	}{
		{
			name: "success",
			setup: []setupAction{
				{action: "RegisterWorkflowType", body: map[string]any{
					"domain": "d1", "name": "wf1", "version": "1.0",
				}},
			},
			body:     map[string]any{"domain": "d1", "workflowType": map[string]any{"name": "wf1", "version": "1.0"}},
			wantCode: http.StatusOK,
		},
		{
			name: "not_found",
			body: map[string]any{
				"domain":       "d1",
				"workflowType": map[string]any{"name": "missing", "version": "1.0"},
			},
			wantCode: http.StatusNotFound,
			wantType: "UnknownResourceFault",
		},
		{
			name: "already_deprecated",
			setup: []setupAction{
				{action: "RegisterWorkflowType", body: map[string]any{
					"domain": "d1", "name": "wf-dep", "version": "1.0",
				}},
				{action: "DeprecateWorkflowType", body: map[string]any{
					"domain": "d1", "workflowType": map[string]any{"name": "wf-dep", "version": "1.0"},
				}},
			},
			body: map[string]any{
				"domain":       "d1",
				"workflowType": map[string]any{"name": "wf-dep", "version": "1.0"},
			},
			wantCode: http.StatusBadRequest,
			wantType: "TypeDeprecatedFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSWFHandler(t)
			doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "d1"})
			for _, s := range tt.setup {
				doSWFRequest(t, h, s.action, s.body)
			}

			rec := doSWFRequest(t, h, "DeprecateWorkflowType", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantType != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"])
			}
		})
	}
}

func TestHandler_DeprecateWorkflowType_ThenDescribeShowsDeprecated(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)

	doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "d1"})
	doSWFRequest(t, h, "RegisterWorkflowType", map[string]any{
		"domain": "d1", "name": "wf1", "version": "1.0",
	})

	rec := doSWFRequest(t, h, "DeprecateWorkflowType", map[string]any{
		"domain": "d1", "workflowType": map[string]any{"name": "wf1", "version": "1.0"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSWFRequest(t, h, "DescribeWorkflowType", map[string]any{
		"domain": "d1", "workflowType": map[string]any{"name": "wf1", "version": "1.0"},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	resp := parseSWFResp(t, rec2)
	typeInfo := resp["typeInfo"].(map[string]any)
	assert.Equal(t, "DEPRECATED", typeInfo["status"])
}

func TestHandler_UndeprecateWorkflowType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		setup    []setupAction
		wantCode int
	}{
		{
			name: "success",
			setup: []setupAction{
				{action: "RegisterWorkflowType", body: map[string]any{"domain": "d1", "name": "wf1", "version": "1.0"}},
				{
					action: "DeprecateWorkflowType",
					body: map[string]any{
						"domain":       "d1",
						"workflowType": map[string]any{"name": "wf1", "version": "1.0"},
					},
				},
			},
			body:     map[string]any{"domain": "d1", "workflowType": map[string]any{"name": "wf1", "version": "1.0"}},
			wantCode: http.StatusOK,
		},
		{
			name: "not_found",
			body: map[string]any{
				"domain":       "d1",
				"workflowType": map[string]any{"name": "missing", "version": "1.0"},
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSWFHandler(t)
			doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "d1"})
			for _, s := range tt.setup {
				doSWFRequest(t, h, s.action, s.body)
			}

			rec := doSWFRequest(t, h, "UndeprecateWorkflowType", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestHandler_DeleteWorkflowType_RequiresDeprecatedFirst verifies
// DeleteWorkflowType rejects deleting a type that hasn't been deprecated yet
// (real AWS: "Prior to deletion, workflow types must first be deprecated"),
// succeeds once deprecated, and 404s on an unknown type.
func TestHandler_DeleteWorkflowType_RequiresDeprecatedFirst(t *testing.T) {
	t.Parallel()

	h := newTestSWFHandler(t)
	doSWFRequest(t, h, "RegisterDomain", map[string]any{"name": "d1"})
	doSWFRequest(t, h, "RegisterWorkflowType", map[string]any{"domain": "d1", "name": "wf1", "version": "1.0"})

	body := map[string]any{
		"domain":       "d1",
		"workflowType": map[string]any{"name": "wf1", "version": "1.0"},
	}

	// Not yet deprecated -> TypeNotDeprecatedFault.
	rec := doSWFRequest(t, h, "DeleteWorkflowType", body)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	errResp := parseSWFResp(t, rec)
	assert.Equal(t, "TypeNotDeprecatedFault", errResp["__type"])

	// Deprecate, then delete should succeed.
	doSWFRequest(t, h, "DeprecateWorkflowType", map[string]any{
		"domain": "d1", "workflowType": map[string]any{"name": "wf1", "version": "1.0"},
	})
	rec = doSWFRequest(t, h, "DeleteWorkflowType", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Unknown type -> UnknownResourceFault.
	rec = doSWFRequest(t, h, "DeleteWorkflowType", body)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
