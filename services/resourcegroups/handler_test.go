package resourcegroups_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/resourcegroups"
)

func newTestResourceGroupsHandler(t *testing.T) *resourcegroups.Handler {
	t.Helper()

	return resourcegroups.NewHandler(resourcegroups.NewInMemoryBackend("000000000000", "us-east-1"))
}

func doResourceGroupsRequest(
	t *testing.T,
	h *resourcegroups.Handler,
	action string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	} else {
		bodyBytes = []byte("{}")
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "ResourceGroups."+action)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

// doResourceGroupsRequestRaw sends a raw (possibly invalid JSON) body to the handler.
func doResourceGroupsRequestRaw(
	t *testing.T,
	h *resourcegroups.Handler,
	action string,
	body []byte,
) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "ResourceGroups."+action)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

// doResourceTagsRequest makes a request to the /resources/{arn}/tags endpoint.
func doResourceTagsRequest(
	t *testing.T,
	h *resourcegroups.Handler,
	method, resourceARN string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	e := echo.New()
	path := "/resources/" + resourceARN + "/tags"
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Request().RequestURI = path

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

// doResourceGroupsRESTRequest makes requests via REST path.
func doResourceGroupsRESTRequest(
	t *testing.T,
	h *resourcegroups.Handler,
	path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	} else {
		bodyBytes = []byte("{}")
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestResourceGroupsHandler_UnknownAction(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	rec := doResourceGroupsRequest(t, h, "UnknownAction", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestUnknownOperationRejected verifies that unrecognized operation names are rejected.
func TestUnknownOperationRejected(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	rec := doResourceGroupsRequest(t, h, "NonExistentOperation", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestResourceGroupsHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{
			name:   "match",
			target: "ResourceGroups.ListGroups",
			want:   true,
		},
		{
			name:   "no_match",
			target: "Kinesis_20131202.CreateStream",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestResourceGroupsHandler(t)
			matcher := h.RouteMatcher()

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			c := e.NewContext(req, httptest.NewRecorder())

			assert.Equal(t, tt.want, matcher(c))
		})
	}
}

func TestResourceGroupsHandler_ProviderName(t *testing.T) {
	t.Parallel()

	p := &resourcegroups.Provider{}
	assert.Equal(t, "ResourceGroups", p.Name())
}

func TestResourceGroupsHandler_HandlerName(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	assert.Equal(t, "ResourceGroups", h.Name())
}

func TestResourceGroupsHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	ops := h.GetSupportedOperations()
	assert.Contains(t, ops, "CreateGroup")
	assert.Contains(t, ops, "DeleteGroup")
	assert.Contains(t, ops, "ListGroups")
	assert.Contains(t, ops, "GetGroup")
}

// TestGetSupportedOperationsSorted verifies the operations list is sorted and complete.
func TestGetSupportedOperationsSorted(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	ops := h.GetSupportedOperations()

	require.NotEmpty(t, ops)

	for i := 1; i < len(ops); i++ {
		assert.LessOrEqual(t, ops[i-1], ops[i], "operations should be sorted")
	}

	assert.Contains(t, ops, "UpdateAccountSettings")
	assert.Contains(t, ops, "CancelTagSyncTask")
	assert.Contains(t, ops, "StartTagSyncTask")
}

// TestHandlerOpsLen verifies the dispatch table is pre-built and contains the
// expected number of operations.
func TestHandlerOpsLen(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	assert.Equal(t, 20, resourcegroups.HandlerOpsLen(h))
}

func TestResourceGroupsHandler_MatchPriority(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	assert.Equal(t, 100, h.MatchPriority())
}

func TestResourceGroupsHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{
			name:   "with_target",
			target: "ResourceGroups.CreateGroup",
			want:   "CreateGroup",
		},
		{
			name: "no_target",
			want: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestResourceGroupsHandler(t)
			e := echo.New()

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tt.target != "" {
				req.Header.Set("X-Amz-Target", tt.target)
			}
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

func TestResourceGroupsHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "name_field",
			body: `{"Name":"my-group"}`,
			want: "my-group",
		},
		{
			name: "group_name_field",
			body: `{"GroupName":"other-group"}`,
			want: "other-group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestResourceGroupsHandler(t)
			e := echo.New()

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.want, h.ExtractResource(c))
		})
	}
}

func TestResourceGroupsHandler_ProviderInit(t *testing.T) {
	t.Parallel()

	p := &resourcegroups.Provider{}
	ctx := &service.AppContext{Logger: slog.Default()}
	svc, err := p.Init(ctx)
	require.NoError(t, err)
	assert.NotNil(t, svc)
	assert.Equal(t, "ResourceGroups", svc.Name())
}

// TestErrNilAppContext verifies the provider returns an error for a nil context.
func TestErrNilAppContext(t *testing.T) {
	t.Parallel()

	p := &resourcegroups.Provider{}
	_, err := p.Init(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, resourcegroups.ErrNilAppContext)
}

func TestResourceGroupsHandler_ResourceTagsMethodNotAllowed(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	rec := doResourceTagsRequest(t, h, http.MethodDelete,
		"arn:aws:resource-groups:us-east-1:000000000000:group/my-group", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestResourceGroupsHandler_RouteMatcherTagsPath(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	matcher := h.RouteMatcher()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet,
		"/resources/arn:aws:resource-groups:us-east-1:000000000000:group/my-group/tags", nil)
	req.RequestURI = "/resources/arn:aws:resource-groups:us-east-1:000000000000:group/my-group/tags"
	c := e.NewContext(req, httptest.NewRecorder())

	assert.True(t, matcher(c))
}

func TestResourceGroupsHandler_RESTMethodNotAllowed(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/groups", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestResourceGroupsHandler_RESTExtractOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		path   string
		method string
		want   string
	}{
		{name: "create_group", path: "/groups", method: http.MethodPost, want: "CreateGroup"},
		{name: "get_group", path: "/get-group", method: http.MethodPost, want: "GetGroup"},
		{
			name:   "tags_get",
			path:   "/resources/arn:aws:rg:us-east-1:123:group/g/tags",
			method: http.MethodGet,
			want:   "GetTags",
		},
		{
			name:   "tags_put",
			path:   "/resources/arn:aws:rg:us-east-1:123:group/g/tags",
			method: http.MethodPut,
			want:   "Tag",
		},
		{
			name:   "tags_patch",
			path:   "/resources/arn:aws:rg:us-east-1:123:group/g/tags",
			method: http.MethodPatch,
			want:   "Untag",
		},
		{
			name:   "tags_unknown_method",
			path:   "/resources/arn:aws:rg:us-east-1:123:group/g/tags",
			method: http.MethodDelete,
			want:   "Untag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestResourceGroupsHandler(t)
			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

func TestResourceGroupsHandler_SnapshotRestore(t *testing.T) {
	t.Parallel()

	h := newTestResourceGroupsHandler(t)
	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "persist-group"})

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := newTestResourceGroupsHandler(t)
	require.NoError(t, h2.Restore(t.Context(), snap))

	rec := doResourceGroupsRequest(t, h2, "GetGroup", map[string]any{"GroupName": "persist-group"})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestBadJSONRequest verifies that malformed JSON returns 400 across several operations.
func TestBadJSONRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
	}{
		{name: "CreateGroup", action: "CreateGroup"},
		{name: "GroupResources", action: "GroupResources"},
		{name: "StartTagSyncTask", action: "StartTagSyncTask"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestResourceGroupsHandler(t)
			rec := doResourceGroupsRequestRaw(t, h, tt.action, []byte("{invalid-json"))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestErrorShapes verifies consistent error structure for 404 and 400 across
// operation families. ListGroupingStatuses is deliberately absent: it
// declares no NotFoundException, so a nonexistent Group succeeds rather
// than 404s -- see its own test. CancelTagSyncTask also declares no
// NotFoundException but is left 404ing pending a confirmed remedy -- see
// its landmine comment in tagsync.go (gopherstack-m4k0).
func TestErrorShapes(t *testing.T) {
	t.Parallel()

	tests := []struct { //nolint:govet // field order optimized for readability
		name     string
		op       string
		body     map[string]any
		wantCode int
	}{
		{
			name:     "get_group_404",
			op:       "GetGroup",
			body:     map[string]any{"Group": "nonexistent"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "delete_group_404",
			op:       "DeleteGroup",
			body:     map[string]any{"Group": "ghost"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "group_resources_group_404",
			op:       "GroupResources",
			body:     map[string]any{"Group": "ghost", "ResourceArns": []string{"arn:aws:s3:::b"}},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "list_group_resources_404",
			op:       "ListGroupResources",
			body:     map[string]any{"Group": "ghost"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "create_group_invalid_name_400",
			op:       "CreateGroup",
			body:     map[string]any{"Name": "aws-not-allowed"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "start_task_no_group_400",
			op:       "StartTagSyncTask",
			body:     map[string]any{"RoleArn": "arn:aws:iam::000000000000:role/r"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "cancel_task_not_found_404",
			op:       "CancelTagSyncTask",
			body:     map[string]any{"TaskArn": "arn:aws:resource-groups:us-east-1:000000000000:tag-sync-task/ghost"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "get_task_not_found_404",
			op:       "GetTagSyncTask",
			body:     map[string]any{"TaskArn": "arn:aws:resource-groups:us-east-1:000000000000:tag-sync-task/ghost"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestResourceGroupsHandler(t)
			rec := doResourceGroupsRequest(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code, "op=%s body=%v resp=%s", tt.op, tt.body, rec.Body.String())
			// All errors include a "message" field.
			assert.Contains(t, rec.Body.String(), "message")
		})
	}
}

// TestResetClearsAllState verifies Handler.Reset wipes all backend state.
func TestResetClearsAllState(t *testing.T) {
	t.Parallel()

	b := resourcegroups.NewInMemoryBackend("000000000000", "us-east-1")
	h := resourcegroups.NewHandler(b)

	doResourceGroupsRequest(t, h, "CreateGroup", map[string]any{"Name": "g1"})
	doResourceGroupsRequest(t, h, "GroupResources", map[string]any{
		"Group":        "g1",
		"ResourceArns": []string{"arn:aws:s3:::b1"},
	})
	doResourceGroupsRequest(t, h, "StartTagSyncTask", map[string]any{
		"Group":   "g1",
		"RoleArn": "arn:aws:iam::000000000000:role/r",
	})

	h.Reset()

	assert.Equal(t, 0, resourcegroups.GroupCount(b))
	assert.Equal(t, 0, resourcegroups.GroupResourceCount(b))
	assert.Equal(t, 0, resourcegroups.TagSyncTaskCount(b))
}
