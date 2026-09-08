package athena_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/athena"
)

func TestHandler_CreateWorkGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantErr    bool
	}{
		{
			name:       "success",
			body:       `{"Name":"test-wg","Description":"desc","State":"ENABLED"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "duplicate",
			body:       `{"Name":"test-wg","Description":"desc"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "bytes_scanned_cutoff_below_minimum_rejected",
			body:       `{"Name":"too-small-cutoff","Configuration":{"BytesScannedCutoffPerQuery":1024}}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "bytes_scanned_cutoff_above_minimum_accepted",
			body:       `{"Name":"good-cutoff","Configuration":{"BytesScannedCutoffPerQuery":20971520}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "bytes_scanned_cutoff_zero_means_unlimited",
			body:       `{"Name":"unlimited-cutoff","Configuration":{"BytesScannedCutoffPerQuery":0}}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := newTestHandler(t)

			if tt.name == "duplicate" {
				_ = doRequest(t, handler, "CreateWorkGroup", `{"Name":"test-wg"}`)
			}

			rec := doRequest(t, handler, "CreateWorkGroup", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantErr {
				var errResp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.NotEmpty(t, errResp["__type"])
			}
		})
	}
}

func TestHandler_GetWorkGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		workGroup  string
		wantName   string
		wantStatus int
	}{
		{
			name:       "success_primary",
			workGroup:  "primary",
			wantStatus: http.StatusOK,
			wantName:   "primary",
		},
		{
			name:       "not_found",
			workGroup:  "nonexistent",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := `{"WorkGroup":"` + tt.workGroup + `"}`
			rec := doRequest(t, h, "GetWorkGroup", body)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantName != "" {
				var resp map[string]map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantName, resp["WorkGroup"]["Name"])
			}
		})
	}
}

func TestHandler_ListWorkGroups(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "ListWorkGroups", `{}`)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string][]map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp["WorkGroups"]), 1)

	found := false
	for _, wg := range resp["WorkGroups"] {
		if wg["Name"] == "primary" {
			found = true

			break
		}
	}

	assert.True(t, found, "primary workgroup should be in list")
}

func TestHandler_DeleteWorkGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		workGroup  string
		wantStatus int
	}{
		{
			name:       "success",
			workGroup:  "deletable",
			wantStatus: http.StatusOK,
		},
		{
			name:       "protected_primary",
			workGroup:  "primary",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not_found",
			workGroup:  "does-not-exist",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.name == "success" {
				createRec := doRequest(t, h, "CreateWorkGroup", `{"Name":"deletable"}`)
				assert.Equal(t, http.StatusOK, createRec.Code)
			}

			rec := doRequest(t, h, "DeleteWorkGroup", `{"WorkGroup":"`+tt.workGroup+`"}`)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// --- NamedQuery tests ---

func TestHandler_UpdateWorkGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.name == "success" {
				_ = doRequest(t, h, "CreateWorkGroup", `{"Name":"upd-wg"}`)
				rec := doRequest(t, h, "UpdateWorkGroup",
					`{"WorkGroup":"upd-wg","Description":"updated","State":"DISABLED"}`)
				assert.Equal(t, tt.wantStatus, rec.Code)
			} else {
				rec := doRequest(t, h, "UpdateWorkGroup",
					`{"WorkGroup":"no-such-wg","Description":"x"}`)
				assert.Equal(t, tt.wantStatus, rec.Code)
			}
		})
	}
}

// --- Additional NamedQuery tests ---

func TestHandler_CreateWorkGroup_WithTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "with_tags",
			body:       `{"Name":"tagged-wg","Tags":[{"Key":"env","Value":"test"}]}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateWorkGroup", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_GetWorkGroup_NoInventedTagsField locks in that GetWorkGroup's
// response WorkGroup object never carries a "Tags" key -- AWS's real
// GetWorkGroupOutput.WorkGroup has no such field; tags set at creation time
// are visible only through ListTagsForResource. A previous version of this
// service echoed creation-time tags back on the WorkGroup object itself,
// which was a gopherstack-invented addition to the wire shape (and, worse,
// went stale the moment TagResource/UntagResource were called against the
// same workgroup, since those only ever touched the separate tag store).
func TestHandler_GetWorkGroup_NoInventedTagsField(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateWorkGroup",
		`{"Name":"tagged-wg2","Tags":[{"Key":"env","Value":"test"}]}`)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "GetWorkGroup", `{"WorkGroup":"tagged-wg2"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	_, hasTags := resp["WorkGroup"]["Tags"]
	assert.False(t, hasTags, "WorkGroup response must not carry an invented Tags field")

	const wgARN = "arn:aws:athena:us-east-1:000000000000:workgroup/tagged-wg2"
	rec = doRequest(t, h, "ListTagsForResource", `{"ResourceARN":"`+wgARN+`"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var tagsResp map[string][]map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tagsResp))
	require.Len(t, tagsResp["Tags"], 1, "the tag set at creation must still be visible via ListTagsForResource")
	assert.Equal(t, "env", tagsResp["Tags"][0]["Key"])
}

func TestHandler_CreateWorkGroup_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "missing_name",
			body:       `{"Description":"no name"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateWorkGroup", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_WorkGroupConfiguration_EnforceAndExtras(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := `{
		"Name":"wg-enforce",
		"Configuration":{
			"EnforceWorkGroupConfiguration":true,
			"EnableMinimumEncryptionConfiguration":true,
			"ExecutionRole":"arn:aws:iam::000000000000:role/AthenaRole",
			"AdditionalConfiguration":"{\"spark.conf\":\"value\"}",
			"CustomerContentEncryptionConfiguration":{"KmsKey":"arn:aws:kms:us-east-1:000000000000:key/test"}
		}
	}`
	rec := doRequest(t, h, "CreateWorkGroup", body)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "GetWorkGroup", `{"WorkGroup":"wg-enforce"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wg := resp["WorkGroup"].(map[string]any)
	cfg := wg["Configuration"].(map[string]any)

	assert.Equal(t, true, cfg["EnforceWorkGroupConfiguration"])
	assert.Equal(t, true, cfg["EnableMinimumEncryptionConfiguration"])
	assert.Equal(t, "arn:aws:iam::000000000000:role/AthenaRole", cfg["ExecutionRole"])
	assert.NotEmpty(t, cfg["AdditionalConfiguration"])

	cce := cfg["CustomerContentEncryptionConfiguration"].(map[string]any)
	assert.Equal(t, "arn:aws:kms:us-east-1:000000000000:key/test", cce["KmsKey"])

	assert.NotZero(t, wg["CreationTime"])
}

func TestHandler_WorkGroupSummary_IncludesFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_ = doRequest(t, h, "CreateWorkGroup", `{
		"Name":"wg-summary",
		"Description":"my workgroup",
		"Configuration":{"EngineVersion":{"SelectedEngineVersion":"Athena engine version 3"}}
	}`)

	rec := doRequest(t, h, "ListWorkGroups", `{}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string][]map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	var found map[string]any
	for _, wg := range resp["WorkGroups"] {
		if wg["Name"] == "wg-summary" {
			found = wg

			break
		}
	}
	require.NotNil(t, found, "wg-summary not in list")

	assert.Equal(t, "my workgroup", found["Description"])
	assert.NotZero(t, found["CreationTime"])
	ev := found["EngineVersion"].(map[string]any)
	assert.Equal(t, "Athena engine version 3", ev["SelectedEngineVersion"])
}

func TestWorkGroup_Lifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, h *athena.Handler)
		name string
	}{
		{
			name: "create_response_is_empty_object",
			fn: func(t *testing.T, h *athena.Handler) {
				t.Helper()
				rec := a1Do(t, h, "CreateWorkGroup", `{"Name":"audit-wg","Description":"d","State":"ENABLED"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				// AWS CreateWorkGroup returns an empty JSON object {}.
				m := a1Unmarshal(t, rec)
				assert.Empty(t, m, "CreateWorkGroup must return empty object")
			},
		},
		{
			name: "get_response_shape",
			fn: func(t *testing.T, h *athena.Handler) {
				t.Helper()
				a1Do(t, h, "CreateWorkGroup", `{"Name":"audit-wg","Description":"desc","State":"ENABLED"}`)
				rec := a1Do(t, h, "GetWorkGroup", `{"WorkGroup":"audit-wg"}`)
				require.Equal(t, http.StatusOK, rec.Code)

				m := a1Unmarshal(t, rec)
				wg, ok := m["WorkGroup"].(map[string]any)
				require.True(t, ok, "WorkGroup key must be present")
				assert.Equal(t, "audit-wg", wg["Name"])
				assert.Equal(t, "ENABLED", wg["State"])
				assert.Equal(t, "desc", wg["Description"])
				assert.NotZero(t, wg["CreationTime"], "CreationTime must be set")
			},
		},
		{
			name: "list_response_shape",
			fn: func(t *testing.T, h *athena.Handler) {
				t.Helper()
				a1Do(t, h, "CreateWorkGroup", `{"Name":"audit-wg","Description":"desc"}`)
				rec := a1Do(t, h, "ListWorkGroups", `{}`)
				require.Equal(t, http.StatusOK, rec.Code)

				m := a1Unmarshal(t, rec)
				wgs, ok := m["WorkGroups"].([]any)
				require.True(t, ok, "WorkGroups key must be present")
				require.GreaterOrEqual(t, len(wgs), 1)

				var found map[string]any
				for _, item := range wgs {
					s, _ := item.(map[string]any)
					if s["Name"] == "audit-wg" {
						found = s

						break
					}
				}
				require.NotNil(t, found, "created workgroup must appear in list")
				assert.NotZero(t, found["CreationTime"])
				assert.Equal(t, "ENABLED", found["State"])
			},
		},
		{
			name: "update_state_to_disabled",
			fn: func(t *testing.T, h *athena.Handler) {
				t.Helper()
				a1Do(t, h, "CreateWorkGroup", `{"Name":"audit-wg","State":"ENABLED"}`)
				rec := a1Do(t, h, "UpdateWorkGroup", `{"WorkGroup":"audit-wg","State":"DISABLED"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				m := a1Unmarshal(t, rec)
				assert.Empty(t, m, "UpdateWorkGroup must return empty object")

				rec = a1Do(t, h, "GetWorkGroup", `{"WorkGroup":"audit-wg"}`)
				wg := a1Unmarshal(t, rec)["WorkGroup"].(map[string]any)
				assert.Equal(t, "DISABLED", wg["State"])
			},
		},
		{
			name: "configuration_present_when_set",
			fn: func(t *testing.T, h *athena.Handler) {
				t.Helper()
				a1Do(
					t,
					h,
					"CreateWorkGroup",
					`{"Name":"cfg-wg","Configuration":{"EnforceWorkGroupConfiguration":true,`+
						`"ExecutionRole":"arn:aws:iam::000000000000:role/AthenaRole"}}`,
				)
				rec := a1Do(t, h, "GetWorkGroup", `{"WorkGroup":"cfg-wg"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				wg := a1Unmarshal(t, rec)["WorkGroup"].(map[string]any)
				cfg, ok := wg["Configuration"].(map[string]any)
				require.True(t, ok, "Configuration must be present when set")
				assert.Equal(t, true, cfg["EnforceWorkGroupConfiguration"])
				assert.Equal(t, "arn:aws:iam::000000000000:role/AthenaRole", cfg["ExecutionRole"])
			},
		},
		{
			name: "delete_then_not_found",
			fn: func(t *testing.T, h *athena.Handler) {
				t.Helper()
				a1Do(t, h, "CreateWorkGroup", `{"Name":"del-wg"}`)
				rec := a1Do(t, h, "DeleteWorkGroup", `{"WorkGroup":"del-wg"}`)
				require.Equal(t, http.StatusOK, rec.Code)

				rec = a1Do(t, h, "GetWorkGroup", `{"WorkGroup":"del-wg"}`)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				m := a1Unmarshal(t, rec)
				assert.NotEmpty(t, m["__type"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := a1Handler(t)
			tt.fn(t, h)
		})
	}
}

func TestWorkGroup_StateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		action     string
		body       string
		wantStatus int
	}{
		{
			name:       "create_enabled_state_accepted",
			action:     "CreateWorkGroup",
			body:       `{"Name":"wg-a","State":"ENABLED"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "create_disabled_state_accepted",
			action:     "CreateWorkGroup",
			body:       `{"Name":"wg-b","State":"DISABLED"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "create_empty_state_defaults_to_enabled",
			action:     "CreateWorkGroup",
			body:       `{"Name":"wg-c","State":""}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "create_invalid_state_returns_400",
			action:     "CreateWorkGroup",
			body:       `{"Name":"wg-d","State":"ACTIVE"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "update_invalid_state_returns_400",
			action:     "UpdateWorkGroup",
			body:       `{"WorkGroup":"primary","State":"UNKNOWN"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "update_valid_state_accepted",
			action:     "UpdateWorkGroup",
			body:       `{"WorkGroup":"primary","State":"DISABLED"}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := athena.NewHandler(athena.NewInMemoryBackend("", ""))
			rec := doRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusBadRequest {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				assert.Contains(t, errResp["__type"], "InvalidRequestException",
					"invalid State must return InvalidRequestException")
			}
		})
	}
}

// TestListWorkGroups_Pagination verifies ListWorkGroups MaxResults/NextToken.
func TestListWorkGroups_Pagination(t *testing.T) {
	t.Parallel()

	b := athena.NewInMemoryBackend("", "")
	h := athena.NewHandler(b)

	// Create 3 extra workgroups (primary exists by default → 4 total).
	for _, wg := range []string{"wg1", "wg2", "wg3"} {
		rec := athenaDoPass5(t, h, "CreateWorkGroup", fmt.Sprintf(`{"Name":%q}`, wg))
		require.Equal(t, http.StatusOK, rec.Code)
	}

	tests := []struct {
		name          string
		body          string
		wantLen       int
		wantNextToken bool
	}{
		{
			name:          "page1_two_results",
			body:          `{"MaxResults":2}`,
			wantLen:       2,
			wantNextToken: true,
		},
		{
			name:          "no_limit_returns_all",
			body:          `{}`,
			wantLen:       4,
			wantNextToken: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := athenaDoPass5(t, h, "ListWorkGroups", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)
			m := athenaUnmarshalPass5(t, rec)
			wgs, _ := m["WorkGroups"].([]any)
			assert.Len(t, wgs, tt.wantLen)
			if tt.wantNextToken {
				assert.NotEmpty(t, m["NextToken"])
			} else {
				assert.Empty(t, m["NextToken"])
			}
		})
	}
}

// TestListWorkGroups_Pagination_StaleTokenResumesStably verifies that a
// NextToken whose boundary workgroup was deleted between calls resumes at
// the next surviving workgroup instead of silently restarting the page from
// offset 0 (which would re-emit already-consumed results to the caller).
func TestListWorkGroups_Pagination_StaleTokenResumesStably(t *testing.T) {
	t.Parallel()

	b := athena.NewInMemoryBackend("", "")
	h := athena.NewHandler(b)

	// Sorted order with the default "primary": primary, wg1, wg2, wg3, wg4.
	for _, wg := range []string{"wg1", "wg2", "wg3", "wg4"} {
		rec := athenaDoPass5(t, h, "CreateWorkGroup", fmt.Sprintf(`{"Name":%q}`, wg))
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec := athenaDoPass5(t, h, "ListWorkGroups", `{"MaxResults":2}`)
	require.Equal(t, http.StatusOK, rec.Code)
	page1 := athenaUnmarshalPass5(t, rec)
	nextToken, _ := page1["NextToken"].(string)
	require.Equal(t, "wg2", nextToken, "boundary of the first page must be the 3rd sorted name")

	// Delete the boundary workgroup the stale token points at.
	rec = athenaDoPass5(t, h, "DeleteWorkGroup", `{"WorkGroup":"wg2"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = athenaDoPass5(t, h, "ListWorkGroups", fmt.Sprintf(`{"MaxResults":2,"NextToken":%q}`, nextToken))
	require.Equal(t, http.StatusOK, rec.Code)
	page2 := athenaUnmarshalPass5(t, rec)
	wgs, _ := page2["WorkGroups"].([]any)
	require.Len(t, wgs, 2)

	names := make([]string, len(wgs))
	for i, w := range wgs {
		names[i] = w.(map[string]any)["Name"].(string)
	}
	assert.Equal(t, []string{"wg3", "wg4"}, names,
		"must resume after the deleted boundary, not restart from offset 0")
}

// TestHandler_DeleteWorkGroup_RecursiveDeleteOption guards
// DeleteWorkGroupInput.RecursiveDeleteOption: "The option to delete the
// workgroup and its contents even if the workgroup contains any named
// queries, query executions, or notebooks." Without it, deleting a
// non-empty workgroup must be rejected instead of silently orphaning its
// named queries/query executions/notebooks (they would otherwise reference
// a workgroup that no longer exists).
func TestHandler_DeleteWorkGroup_RecursiveDeleteOption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		seed       func(t *testing.T, h *athena.Handler, wg string)
		name       string
		wantStatus int
		recursive  bool
	}{
		{
			name: "named_query_blocks_delete",
			seed: func(t *testing.T, h *athena.Handler, wg string) {
				t.Helper()
				rec := doRequest(t, h, "CreateNamedQuery",
					`{"Name":"nq","Database":"db","QueryString":"SELECT 1","WorkGroup":"`+wg+`"}`)
				require.Equal(t, http.StatusOK, rec.Code)
			},
			recursive:  false,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "named_query_recursive_delete_succeeds",
			seed: func(t *testing.T, h *athena.Handler, wg string) {
				t.Helper()
				rec := doRequest(t, h, "CreateNamedQuery",
					`{"Name":"nq","Database":"db","QueryString":"SELECT 1","WorkGroup":"`+wg+`"}`)
				require.Equal(t, http.StatusOK, rec.Code)
			},
			recursive:  true,
			wantStatus: http.StatusOK,
		},
		{
			name: "query_execution_blocks_delete",
			seed: func(t *testing.T, h *athena.Handler, wg string) {
				t.Helper()
				rec := doRequest(t, h, "StartQueryExecution",
					`{"QueryString":"SELECT 1","WorkGroup":"`+wg+`"}`)
				require.Equal(t, http.StatusOK, rec.Code)
			},
			recursive:  false,
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "notebook_blocks_delete",
			seed: func(t *testing.T, h *athena.Handler, wg string) {
				t.Helper()
				rec := doRequest(t, h, "CreateNotebook", `{"WorkGroup":"`+wg+`","Name":"nb"}`)
				require.Equal(t, http.StatusOK, rec.Code)
			},
			recursive:  false,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_workgroup_deletes_without_recursive",
			seed:       func(t *testing.T, _ *athena.Handler, _ string) { t.Helper() },
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			const wg = "recursive-wg"
			rec := doRequest(t, h, "CreateWorkGroup", `{"Name":"`+wg+`"}`)
			require.Equal(t, http.StatusOK, rec.Code)

			tt.seed(t, h, wg)

			body := `{"WorkGroup":"` + wg + `"}`
			if tt.recursive {
				body = `{"WorkGroup":"` + wg + `","RecursiveDeleteOption":true}`
			}

			rec = doRequest(t, h, "DeleteWorkGroup", body)
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())

			if tt.wantStatus == http.StatusOK {
				rec = doRequest(t, h, "GetWorkGroup", `{"WorkGroup":"`+wg+`"}`)
				assert.Equal(t, http.StatusBadRequest, rec.Code, "workgroup must be gone")
			} else {
				rec = doRequest(t, h, "GetWorkGroup", `{"WorkGroup":"`+wg+`"}`)
				assert.Equal(t, http.StatusOK, rec.Code, "workgroup must survive a rejected delete")
			}
		})
	}
}

// TestHandler_DeleteWorkGroup_RecursiveDeleteCascadesContents locks in that a
// recursive DeleteWorkGroup does not leave the named query/query execution it
// contained behind as a ghost row pointing at a now-nonexistent workgroup.
func TestHandler_DeleteWorkGroup_RecursiveDeleteCascadesContents(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	const wg = "cascade-wg"
	rec := doRequest(t, h, "CreateWorkGroup", `{"Name":"`+wg+`"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "CreateNamedQuery",
		`{"Name":"nq","Database":"db","QueryString":"SELECT 1","WorkGroup":"`+wg+`"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	nqID := jsonField(t, rec.Body.Bytes(), "NamedQueryId")

	rec = doRequest(t, h, "StartQueryExecution", `{"QueryString":"SELECT 1","WorkGroup":"`+wg+`"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	qeID := jsonField(t, rec.Body.Bytes(), "QueryExecutionId")

	rec = doRequest(t, h, "DeleteWorkGroup", `{"WorkGroup":"`+wg+`","RecursiveDeleteOption":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = doRequest(t, h, "GetNamedQuery", `{"NamedQueryId":"`+nqID+`"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "named query must not survive a recursive delete")

	rec = doRequest(t, h, "GetQueryExecution", `{"QueryExecutionId":"`+qeID+`"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "query execution must not survive a recursive delete")
}

// TestHandler_UpdateWorkGroup_PreservesUnmentionedConfiguration guards
// against gopherstack-1vv2: UpdateWorkGroupInput.ConfigurationUpdates is
// types.WorkGroupConfigurationUpdates, a partial-update shape -- a real
// client only ever sends the fields it's changing. Wholesale-replacing the
// stored Configuration with that payload used to silently erase every field
// the request didn't mention (here, ResultConfiguration and EngineVersion
// set at Create) on any single-field Update.
func TestHandler_UpdateWorkGroup_PreservesUnmentionedConfiguration(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createBody := `{
		"Name": "preserve-cfg-wg",
		"Configuration": {
			"ResultConfiguration": {"OutputLocation": "s3://my-bucket/results/"},
			"EngineVersion": {"SelectedEngineVersion": "Athena engine version 3"},
			"EnforceWorkGroupConfiguration": false
		}
	}`
	rec := doRequest(t, h, "CreateWorkGroup", createBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// A real client's Update payload only ever carries the field it's
	// changing -- here, just EnforceWorkGroupConfiguration.
	updateBody := `{
		"WorkGroup": "preserve-cfg-wg",
		"ConfigurationUpdates": {"EnforceWorkGroupConfiguration": true}
	}`
	rec = doRequest(t, h, "UpdateWorkGroup", updateBody)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = doRequest(t, h, "GetWorkGroup", `{"WorkGroup":"preserve-cfg-wg"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wg, ok := resp["WorkGroup"].(map[string]any)
	require.True(t, ok)
	cfg, ok := wg["Configuration"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, true, cfg["EnforceWorkGroupConfiguration"], "the update's own field must apply")

	resultConfig, ok := cfg["ResultConfiguration"].(map[string]any)
	require.True(t, ok, "ResultConfiguration must survive an Update that never mentioned it")
	assert.Equal(t, "s3://my-bucket/results/", resultConfig["OutputLocation"])

	engineVersion, ok := cfg["EngineVersion"].(map[string]any)
	require.True(t, ok, "EngineVersion must survive an Update that never mentioned it")
	assert.Equal(t, "Athena engine version 3", engineVersion["SelectedEngineVersion"])
}
