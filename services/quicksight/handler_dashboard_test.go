package quicksight_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/quicksight"
)

// ---- Dashboard extras tests ---- //nolint:godot // existing issue.
func TestQuickSight_DashboardExtras(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	// Need a dashboard
	rec := doRequest(t, h, http.MethodPost, accountPath("/dashboards/dash1"), map[string]any{
		"Name": "Dashboard1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	tests := []struct {
		body       any
		name       string
		method     string
		path       string
		wantKey    string
		wantStatus int
	}{
		{
			name:       "describe dashboard definition",
			method:     http.MethodGet,
			path:       accountPath("/dashboards/dash1/definition"),
			wantStatus: http.StatusOK,
			wantKey:    "DashboardId",
		},
		{
			name:       "describe dashboard permissions",
			method:     http.MethodGet,
			path:       accountPath("/dashboards/dash1/permissions"),
			wantStatus: http.StatusOK,
			wantKey:    "DashboardId",
		},
		{
			name:       "update dashboard permissions",
			method:     http.MethodPut,
			path:       accountPath("/dashboards/dash1/permissions"),
			body:       map[string]any{"GrantPermissions": []any{}, "RevokePermissions": []any{}},
			wantStatus: http.StatusOK,
			wantKey:    "DashboardId",
		},
		{
			name:       "update dashboard published version",
			method:     http.MethodPut,
			path:       accountPath("/dashboards/dash1/versions/1"),
			wantStatus: http.StatusOK,
			wantKey:    "DashboardId",
		},
		{
			name:       "update dashboard links",
			method:     http.MethodPut,
			path:       accountPath("/dashboards/dash1/linked-entities"),
			body:       map[string]any{"LinkEntities": []any{}},
			wantStatus: http.StatusOK,
			wantKey:    "DashboardArn",
		},
		{
			name:       "start dashboard snapshot job",
			method:     http.MethodPost,
			path:       accountPath("/dashboards/dash1/snapshot-jobs"),
			body:       map[string]any{"SnapshotJobId": "snap1"},
			wantStatus: http.StatusOK,
			wantKey:    "SnapshotJobId",
		},
		{
			name:       "describe dashboard snapshot job",
			method:     http.MethodGet,
			path:       accountPath("/dashboards/dash1/snapshot-jobs/snap1"),
			wantStatus: http.StatusOK,
			wantKey:    "JobStatus",
		},
		{
			name:       "describe dashboard snapshot job result",
			method:     http.MethodGet,
			path:       accountPath("/dashboards/dash1/snapshot-jobs/snap1/result"),
			wantStatus: http.StatusOK,
			wantKey:    "Result",
		},
		{
			name:       "get dashboard embed url",
			method:     http.MethodGet,
			path:       accountPath("/dashboards/dash1/embed-url") + "?creds-type=QUICKSIGHT",
			wantStatus: http.StatusOK,
			wantKey:    "EmbedUrl",
		},
		{
			// Real StartDashboardSnapshotJobScheduleOutput carries no data
			// fields beyond RequestId/Status; it must not echo back a
			// fabricated DashboardId.
			name:       "start dashboard snapshot job schedule",
			method:     http.MethodPost,
			path:       accountPath("/dashboards/dash1/schedules/sched1"),
			wantStatus: http.StatusOK,
			wantKey:    "RequestId",
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.method, tc.path, tc.body) //nolint:govet // existing issue.
			assert.Equal(t, tc.wantStatus, rec.Code, "status")
			if tc.wantKey != "" {
				body := parseBody(t, rec)
				assert.Contains(t, body, tc.wantKey)
			}
		})
	}
}

// ---- Dashboard tests ----

func TestQuickSight_Dashboards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		setup    func(h *quicksight.Handler)
		check    func(t *testing.T, body map[string]any)
		name     string
		method   string
		path     string
		wantCode int
	}{
		{
			name:     "CreateDashboard returns ARN and version 1",
			method:   http.MethodPost,
			path:     accountPath("/dashboards/dash1"),
			body:     map[string]any{"Name": "My Dashboard"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				t.Helper()
				assert.Equal(t, "dash1", body["DashboardId"])
				assert.Contains(t, body["Arn"], "arn:aws:quicksight:us-east-1:000000000000:dashboard/dash1")
				assert.Contains(t, body["VersionArn"], "/version/1")
			},
		},
		{
			name:   "CreateDashboard duplicate returns 409",
			method: http.MethodPost,
			path:   accountPath("/dashboards/dup-dash"),
			setup: func(h *quicksight.Handler) {
				doRequest(t, h, http.MethodPost, accountPath("/dashboards/dup-dash"), map[string]any{"Name": "x"})
			},
			body:     map[string]any{"Name": "x"},
			wantCode: http.StatusConflict,
		},
		{
			name:   "DescribeDashboard returns dashboard",
			method: http.MethodGet,
			path:   accountPath("/dashboards/dash2"),
			setup: func(h *quicksight.Handler) {
				doRequest(t, h, http.MethodPost, accountPath("/dashboards/dash2"), map[string]any{"Name": "D2"})
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				t.Helper()
				d, ok := body["Dashboard"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "dash2", d["DashboardId"])
				v, ok := d["Version"].(map[string]any)
				require.True(t, ok)
				assert.InDelta(t, float64(1), v["VersionNumber"], 0)
			},
		},
		{
			name:     "DescribeDashboard missing returns 404",
			method:   http.MethodGet,
			path:     accountPath("/dashboards/notexist"),
			wantCode: http.StatusNotFound,
		},
		{
			name:   "UpdateDashboard increments version",
			method: http.MethodPut,
			path:   accountPath("/dashboards/dash3"),
			setup: func(h *quicksight.Handler) {
				doRequest(t, h, http.MethodPost, accountPath("/dashboards/dash3"), map[string]any{"Name": "D3"})
			},
			body:     map[string]any{"Name": "D3-Updated"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				t.Helper()
				assert.Contains(t, body["VersionArn"], "/version/2")
			},
		},
		{
			name:   "ListDashboardVersions returns all versions",
			method: http.MethodGet,
			path:   accountPath("/dashboards/dash4/versions"),
			setup: func(h *quicksight.Handler) {
				doRequest(t, h, http.MethodPost, accountPath("/dashboards/dash4"), map[string]any{"Name": "D4"})
				doRequest(t, h, http.MethodPut, accountPath("/dashboards/dash4"), map[string]any{"Name": "D4-v2"})
				doRequest(t, h, http.MethodPut, accountPath("/dashboards/dash4"), map[string]any{"Name": "D4-v3"})
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				t.Helper()
				items, ok := body["DashboardVersionSummaryList"].([]any)
				require.True(t, ok)
				assert.Len(t, items, 3)
			},
		},
		{
			name:   "DeleteDashboard removes dashboard",
			method: http.MethodDelete,
			path:   accountPath("/dashboards/dash5"),
			setup: func(h *quicksight.Handler) {
				doRequest(t, h, http.MethodPost, accountPath("/dashboards/dash5"), map[string]any{"Name": "D5"})
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "ListDashboards returns dashboards",
			method: http.MethodGet,
			path:   accountPath("/dashboards"),
			setup: func(h *quicksight.Handler) {
				doRequest(t, h, http.MethodPost, accountPath("/dashboards/da"), map[string]any{"Name": "A"})
				doRequest(t, h, http.MethodPost, accountPath("/dashboards/db"), map[string]any{"Name": "B"})
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body map[string]any) {
				t.Helper()
				items, ok := body["DashboardSummaryList"].([]any)
				require.True(t, ok)
				assert.Len(t, items, 2)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tc.setup != nil {
				tc.setup(h)
			}
			rec := doRequest(t, h, tc.method, tc.path, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, parseBody(t, rec))
			}
		})
	}
}

// TestQuickSight_DeleteDashboard_SpecificVersion mirrors
// TestQuickSight_DeleteTemplate_SpecificVersion (handler_templates_test.go).
// DeleteDashboard with a VersionNumber currently validates-and-no-ops (see
// dashboard.go's DeleteDashboard doc comment, gopherstack-86y) because this
// backend has no per-version storage -- but the deleted version number must
// still stop being reported as live: ListDashboardVersions must drop it, and
// a repeat delete of the same version must now 404 rather than keep
// succeeding, matching DeleteTemplate's real removal (templates.go's
// DeleteTemplate deletes t.Versions[versionNumber], so a second delete of the
// same version already 404s there).
func TestQuickSight_DeleteDashboard_SpecificVersion(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, accountPath("/dashboards/dvdash"), map[string]any{"Name": "D1"})
	doRequest(t, h, http.MethodPut, accountPath("/dashboards/dvdash"), map[string]any{"Name": "D2"})

	delRec := doRequest(t, h, http.MethodDelete, accountPath("/dashboards/dvdash?version-number=1"), nil)
	require.Equal(t, http.StatusOK, delRec.Code)

	// Dashboard itself still exists (version 2 remains).
	describeRec := doRequest(t, h, http.MethodGet, accountPath("/dashboards/dvdash"), nil)
	require.Equal(t, http.StatusOK, describeRec.Code)

	// Version 1 must no longer be reported as a live version.
	versionsRec := doRequest(t, h, http.MethodGet, accountPath("/dashboards/dvdash/versions"), nil)
	require.Equal(t, http.StatusOK, versionsRec.Code)
	items, ok := parseBody(t, versionsRec)["DashboardVersionSummaryList"].([]any)
	require.True(t, ok)
	for _, item := range items {
		v, itemOK := item.(map[string]any)
		require.True(t, itemOK)
		assert.NotEqual(t, float64(1), v["VersionNumber"], "deleted version 1 must not be listed")
	}

	// A repeat delete of the same, already-deleted version must 404, not
	// silently succeed again.
	redeleteRec := doRequest(t, h, http.MethodDelete, accountPath("/dashboards/dvdash?version-number=1"), nil)
	assert.Equal(t, http.StatusNotFound, redeleteRec.Code)
}

// ---- UpdateDashboardPublishedVersion ----

func TestQuickSight_UpdateDashboardPublishedVersion(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, accountPath("/dashboards/dash1"), map[string]any{"Name": "D1"})
	require.Equal(t, http.StatusOK, rec.Code)

	// Bump to version 2.
	rec = doRequest(t, h, http.MethodPut, accountPath("/dashboards/dash1"), map[string]any{"Name": "D1v2"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodPut, accountPath("/dashboards/dash1/versions/2"), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	body := parseBody(t, rec)
	assert.Equal(t, "dash1", body["DashboardId"])
	assert.Contains(t, body["DashboardArn"], "dashboard/dash1")

	// A version number that was never created (3) doesn't exist.
	rec = doRequest(t, h, http.MethodPut, accountPath("/dashboards/dash1/versions/3"), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	body = parseBody(t, rec)
	assert.Equal(t, "ResourceNotFoundException", body["Code"])

	// Unknown dashboard.
	rec = doRequest(t, h, http.MethodPut, accountPath("/dashboards/nope/versions/1"), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestQuickSight_UpdateDashboardPublishedVersion_DeletedVersion pins the
// DeletedVersions guard: publishing a version that DeleteDashboard already
// removed must 404, not succeed.
func TestQuickSight_UpdateDashboardPublishedVersion_DeletedVersion(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, accountPath("/dashboards/pubdel"), map[string]any{"Name": "D1"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodPut, accountPath("/dashboards/pubdel"), map[string]any{"Name": "D2"})
	require.Equal(t, http.StatusOK, rec.Code)

	delRec := doRequest(t, h, http.MethodDelete, accountPath("/dashboards/pubdel?version-number=1"), nil)
	require.Equal(t, http.StatusOK, delRec.Code)

	rec = doRequest(t, h, http.MethodPut, accountPath("/dashboards/pubdel/versions/1"), nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	body := parseBody(t, rec)
	assert.Equal(t, "ResourceNotFoundException", body["Code"])
}

// ---- UpdateDashboardLinks ----

func TestQuickSight_UpdateDashboardLinks(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, accountPath("/dashboards/dash1"), map[string]any{"Name": "D1"})
	require.Equal(t, http.StatusOK, rec.Code)

	linkArn := "arn:aws:quicksight:us-east-1:000000000000:analysis/a1"
	rec = doRequest(t, h, http.MethodPut, accountPath("/dashboards/dash1/linked-entities"), map[string]any{
		"LinkEntities": []any{linkArn},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	body := parseBody(t, rec)
	assert.Contains(t, body["DashboardArn"], "dashboard/dash1")
	linkEntities, ok := body["LinkEntities"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{linkArn}, linkEntities)
	assert.NotContains(t, body, "DashboardId")
}
