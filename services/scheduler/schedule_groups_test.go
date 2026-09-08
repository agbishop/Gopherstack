package scheduler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/scheduler"
)

func TestSchedulerHandler_CreateScheduleGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		wantCode int
		wantARN  bool
	}{
		{
			name:     "success",
			body:     map[string]any{"Name": "my-group", "Tags": wireTagsBody(map[string]string{"env": "test"})},
			wantCode: http.StatusOK,
			wantARN:  true,
		},
		{
			name:     "duplicate",
			body:     map[string]any{"Name": "my-group"},
			wantCode: http.StatusConflict,
		},
		{
			name:     "invalid_json",
			body:     nil,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			if tt.name == "duplicate" {
				doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{"Name": "my-group"})
			}

			var rec *httptest.ResponseRecorder
			if tt.body == nil {
				rec = doInvalidSchedulerRequest(t, h, "CreateScheduleGroup")
			} else {
				rec = doSchedulerRequest(t, h, "CreateScheduleGroup", tt.body)
			}

			assert.Equal(t, tt.wantCode, rec.Code)
			if tt.wantARN {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Contains(t, resp["ScheduleGroupArn"], "arn:aws:scheduler:")
				assert.Contains(t, resp["ScheduleGroupArn"], "schedule-group/my-group")
			}
		})
	}
}

// TestCreateScheduleGroup_NameValidation covers reserved-name and character-set
// rules for schedule group names.
func TestCreateScheduleGroup_NameValidation(t *testing.T) {
	t.Parallel()

	t.Run("reserves_default", func(t *testing.T) {
		t.Parallel()

		h := newTestSchedulerHandler(t)

		rec := doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{
			"Name": "default",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "ValidationException")
	})

	t.Run("invalid_chars", func(t *testing.T) {
		t.Parallel()

		h := newTestSchedulerHandler(t)

		rec := doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{
			"Name": "bad group!",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestCreateScheduleGroup_NameRequired(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	rec := doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestCreateScheduleGroup_DescriptionNotAccepted is the regression test for
// gopherstack-ui6k: real AWS's CreateScheduleGroupInput and GetScheduleGroupOutput
// carry no Description member (CreateScheduleGroupInput serializes only
// ClientToken/Tags; GetScheduleGroupOutput has Arn/CreationDate/
// LastModificationDate/Name/State). A Description sent on create must be silently
// ignored, not stored or echoed back.
func TestCreateScheduleGroup_DescriptionNotAccepted(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	rec := doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{
		"Name":        "described-group",
		"Description": "my group description",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	getRec := doSchedulerRequest(t, h, "GetScheduleGroup", map[string]any{"Name": "described-group"})
	require.Equal(t, http.StatusOK, getRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &resp))
	_, hasDescription := resp["Description"]
	assert.False(t, hasDescription, "GetScheduleGroupOutput has no Description member on real AWS")
}

func TestSchedulerHandler_GetScheduleGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		group    string
		wantName string
		wantCode int
	}{
		{
			name:     "default_group_exists",
			group:    "default",
			wantCode: http.StatusOK,
			wantName: "default",
		},
		{
			name:     "created_group",
			group:    "my-group",
			wantCode: http.StatusOK,
			wantName: "my-group",
		},
		{
			name:     "not_found",
			group:    "nonexistent",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			if tt.group == "my-group" {
				doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{"Name": "my-group"})
			}

			rec := doSchedulerRequest(t, h, "GetScheduleGroup", map[string]any{"Name": tt.group})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantName != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantName, resp["Name"])
				assert.Contains(t, resp["Arn"], "schedule-group/"+tt.wantName)
				assert.Equal(t, "ACTIVE", resp["State"])
				assert.NotEmpty(t, resp["CreationDate"])
				assert.NotEmpty(t, resp["LastModificationDate"])
			}
		})
	}
}

func TestGetScheduleGroup_ReturnsCreationDate(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "GetScheduleGroup", map[string]any{"Name": "default"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["CreationDate"])
	assert.NotEmpty(t, resp["LastModificationDate"])
}

// TestGetScheduleGroup_TimestampsAreEpochSeconds verifies GetScheduleGroup returns epoch seconds.
func TestGetScheduleGroup_TimestampsAreEpochSeconds(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "GetScheduleGroup", map[string]any{"Name": "default"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	var cd float64
	require.NoError(t, json.Unmarshal(out["CreationDate"], &cd), "CreationDate must be a JSON number")
	assert.Greater(t, cd, float64(0))
}

// TestGetScheduleGroup_OmitsTags verifies GetScheduleGroup does NOT include a Tags
// field: real AWS's GetScheduleGroupOutput has no such field -- tags for a
// schedule group are only ever fetched via ListTagsForResource, which this test
// also exercises to confirm the tags themselves were stored correctly.
func TestGetScheduleGroup_OmitsTags(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{
		"Name": "grp-with-tags",
		"Tags": wireTagsBody(map[string]string{"env": "test"}),
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSchedulerRequest(t, h, "GetScheduleGroup", map[string]any{"Name": "grp-with-tags"})
	require.Equal(t, http.StatusOK, rec2.Code)

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))
	_, hasTags := out["Tags"]
	assert.False(t, hasTags, "GetScheduleGroup must not include a Tags field (not in real GetScheduleGroupOutput)")

	var groupARN string
	require.NoError(t, json.Unmarshal(out["Arn"], &groupARN))

	listRec := doSchedulerRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": groupARN})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	tagsMap := wireTagsToMap(t, listResp["Tags"])
	assert.Equal(t, "test", tagsMap["env"])
}

func TestGetScheduleGroup_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	rec := doSchedulerRequest(t, h, "GetScheduleGroup", map[string]any{"Name": "no-such-group"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSchedulerHandler_DeleteScheduleGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		group    string
		setup    bool
		wantCode int
	}{
		{
			name:     "success",
			group:    "my-group",
			setup:    true,
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			group:    "nonexistent",
			setup:    false,
			wantCode: http.StatusNotFound,
		},
		{
			name:     "cannot_delete_default",
			group:    "default",
			setup:    false,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			if tt.setup {
				doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{"Name": tt.group})
			}

			rec := doSchedulerRequest(t, h, "DeleteScheduleGroup", map[string]any{"Name": tt.group})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				getRec := doSchedulerRequest(t, h, "GetScheduleGroup", map[string]any{"Name": tt.group})
				assert.Equal(t, http.StatusNotFound, getRec.Code)
			}
		})
	}
}

// TestDeleteScheduleGroup_Cascade verifies schedules are deleted when their group is deleted.
func TestDeleteScheduleGroup_Cascade(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	h := scheduler.NewHandler(b)

	// Create a group and schedules within it.
	_, err := b.CreateScheduleGroup(context.Background(), "grp-cascade", nil)
	require.NoError(t, err)

	createScheduleViaHandler(t, h, "s1", "grp-cascade", "rate(1 minute)")
	createScheduleViaHandler(t, h, "s2", "grp-cascade", "rate(2 minutes)")
	assert.Equal(t, 2, scheduler.ScheduleCount(b))

	// Delete the group - all schedules in the group should cascade-delete.
	rec := doSchedulerRequest(t, h, "DeleteScheduleGroup", map[string]any{"Name": "grp-cascade"})
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, 0, scheduler.ScheduleCount(b))
	assert.Equal(t, 0, scheduler.ScheduleGroupCount(b)-1) // default group remains
}

// TestDeleteScheduleGroup_DefaultForbidden verifies the default group cannot be deleted.
func TestDeleteScheduleGroup_DefaultForbidden(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "DeleteScheduleGroup", map[string]any{"Name": "default"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSchedulerHandler_ListScheduleGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		groupsToCreate []string
		wantMinCount   int
	}{
		{
			name:         "default_group_always_present",
			wantMinCount: 1,
		},
		{
			name:           "lists_all_groups",
			groupsToCreate: []string{"group-a", "group-b"},
			wantMinCount:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			for _, g := range tt.groupsToCreate {
				doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{"Name": g})
			}

			rec := doSchedulerRequest(t, h, "ListScheduleGroups", nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Contains(t, resp, "ScheduleGroups")
			groups, ok := resp["ScheduleGroups"].([]any)
			require.True(t, ok)
			assert.GreaterOrEqual(t, len(groups), tt.wantMinCount)
		})
	}
}

func TestListScheduleGroups_FilterByNamePrefix(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	b := h.Backend.(*scheduler.InMemoryBackend)

	_, err := b.CreateScheduleGroup(context.Background(), "prod-group", nil)
	require.NoError(t, err)
	_, err = b.CreateScheduleGroup(context.Background(), "dev-group", nil)
	require.NoError(t, err)

	rec := doSchedulerRequest(t, h, "ListScheduleGroups", map[string]any{"NamePrefix": "prod-"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	groups := resp["ScheduleGroups"].([]any)
	require.Len(t, groups, 1)
	assert.Equal(t, "prod-group", groups[0].(map[string]any)["Name"])
}

func TestListScheduleGroups_Sorted(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	b := h.Backend.(*scheduler.InMemoryBackend)

	_, err := b.CreateScheduleGroup(context.Background(), "zoo", nil)
	require.NoError(t, err)
	_, err = b.CreateScheduleGroup(context.Background(), "aardvark", nil)
	require.NoError(t, err)

	rec := doSchedulerRequest(t, h, "ListScheduleGroups", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	groups := resp["ScheduleGroups"].([]any)
	names := make([]string, len(groups))
	for i, g := range groups {
		names[i] = g.(map[string]any)["Name"].(string)
	}

	// "aardvark" comes before "default" alphabetically.
	assert.Equal(t, "aardvark", names[0])
	assert.Equal(t, "default", names[1])
	assert.Equal(t, "zoo", names[2])
}

// TestListScheduleGroups_TimestampsAreEpochSeconds verifies ListScheduleGroups returns epoch seconds.
func TestListScheduleGroups_TimestampsAreEpochSeconds(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "ListScheduleGroups", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		ScheduleGroups []map[string]json.RawMessage `json:"ScheduleGroups"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotEmpty(t, out.ScheduleGroups)

	var cd float64
	require.NoError(t, json.Unmarshal(out.ScheduleGroups[0]["CreationDate"], &cd))
	assert.Greater(t, cd, float64(0))
}

// TestListScheduleGroups_OmitsTags verifies ListScheduleGroups items do NOT
// include a Tags field: real AWS's ScheduleGroupSummary has no such field --
// group tags are only ever fetched via ListTagsForResource.
func TestListScheduleGroups_OmitsTags(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{
		"Name": "list-grp-tags",
		"Tags": wireTagsBody(map[string]string{"x": "y"}),
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doSchedulerRequest(t, h, "ListScheduleGroups", map[string]any{})
	require.Equal(t, http.StatusOK, rec2.Code)

	var out struct {
		ScheduleGroups []map[string]json.RawMessage `json:"ScheduleGroups"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))

	for _, g := range out.ScheduleGroups {
		var name string
		require.NoError(t, json.Unmarshal(g["Name"], &name))

		if name == "list-grp-tags" {
			_, hasTags := g["Tags"]
			assert.False(t, hasTags,
				"ListScheduleGroups item must not include a Tags field (not in real ScheduleGroupSummary)")

			return
		}
	}
	t.Fatal("group list-grp-tags not found in ListScheduleGroups")
}

// TestListScheduleGroups_MaxResultsPagination verifies MaxResults paginates schedule groups.
func TestListScheduleGroups_MaxResultsPagination(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	for _, name := range []string{"grp-a", "grp-b", "grp-c"} {
		rec := doSchedulerRequest(t, h, "CreateScheduleGroup", map[string]any{"Name": name})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// "default" + 3 groups = 4 total; page of 2 should yield a token.
	rec := doSchedulerRequest(t, h, "ListScheduleGroups", map[string]any{"MaxResults": "2"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	var groups []json.RawMessage
	require.NoError(t, json.Unmarshal(out["ScheduleGroups"], &groups))
	assert.Len(t, groups, 2)

	var token string
	require.NoError(t, json.Unmarshal(out["NextToken"], &token))
	assert.NotEmpty(t, token)
}

// TestRESTListScheduleGroups verifies GET /schedule-groups returns groups.
func TestRESTListScheduleGroups(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)

	rec := doRESTRequest(t, h, http.MethodGet, "/schedule-groups", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		ScheduleGroups []map[string]any `json:"ScheduleGroups"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotEmpty(t, out.ScheduleGroups, "default group should always be present")
}

func TestSchedulerHandler_ScheduleGroup_RESTRouting(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/schedule-groups/my-group", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	req2 := httptest.NewRequest(http.MethodGet, "/schedule-groups/my-group", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	err = h.Handler()(c2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec2.Code)
	var getResp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &getResp))
	assert.Equal(t, "my-group", getResp["Name"])

	req3 := httptest.NewRequest(http.MethodGet, "/schedule-groups", nil)
	rec3 := httptest.NewRecorder()
	c3 := e.NewContext(req3, rec3)
	err = h.Handler()(c3)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec3.Code)

	req4 := httptest.NewRequest(http.MethodDelete, "/schedule-groups/my-group", nil)
	rec4 := httptest.NewRecorder()
	c4 := e.NewContext(req4, rec4)
	err = h.Handler()(c4)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec4.Code)
}

func TestSchedulerHandler_RouteMatcher_ScheduleGroups(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	matcher := h.RouteMatcher()
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/schedule-groups", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	assert.True(t, matcher(c))
}

func TestSchedulerHandler_ScheduleGroup_ErrorStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "GetScheduleGroup_NotFound",
			action:   "GetScheduleGroup",
			body:     map[string]any{"Name": "nonexistent"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "DeleteScheduleGroup_NotFound",
			action:   "DeleteScheduleGroup",
			body:     map[string]any{"Name": "nonexistent"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "DeleteScheduleGroup_Default",
			action:   "DeleteScheduleGroup",
			body:     map[string]any{"Name": "default"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "UntagResource_NotFound",
			action: "UntagResource",
			body: map[string]any{
				"ResourceArn": "arn:aws:scheduler:us-east-1:000000000000:schedule/default/nope",
				"TagKeys":     []string{"k"},
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestSchedulerHandler(t)
			rec := doSchedulerRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestSchedulerHandler_ExtractOperation_ScheduleGroups(t *testing.T) {
	t.Parallel()

	h := newTestSchedulerHandler(t)
	e := echo.New()

	tests := []struct {
		name   string
		method string
		path   string
		wantOp string
	}{
		{name: "list_groups", method: http.MethodGet, path: "/schedule-groups", wantOp: "ListScheduleGroups"},
		{
			name:   "create_group",
			method: http.MethodPost,
			path:   "/schedule-groups/my-group",
			wantOp: "CreateScheduleGroup",
		},
		{name: "get_group", method: http.MethodGet, path: "/schedule-groups/my-group", wantOp: "GetScheduleGroup"},
		{
			name:   "delete_group",
			method: http.MethodDelete,
			path:   "/schedule-groups/my-group",
			wantOp: "DeleteScheduleGroup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tt.method, tt.path, nil)
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.wantOp, h.ExtractOperation(c))
		})
	}
}

// TestSchedulerBackend_SnapshotRestore_ScheduleGroups verifies schedule groups
// (including tags) round-trip through Snapshot/Restore.
func TestSchedulerBackend_SnapshotRestore_ScheduleGroups(t *testing.T) {
	t.Parallel()

	b := scheduler.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateScheduleGroup(context.Background(), "production", map[string]string{"env": "prod"})
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := scheduler.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	g, err := fresh.GetScheduleGroup(context.Background(), "production")
	require.NoError(t, err)
	assert.Equal(t, "production", g.Name)
	assert.Equal(t, "ACTIVE", g.State)

	def, err := fresh.GetScheduleGroup(context.Background(), "default")
	require.NoError(t, err)
	assert.Equal(t, "default", def.Name)
}
