package codebuild_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codebuild"
)

// TestHandler_ListProjects_SortOrderDescending verifies sortOrder=DESCENDING
// reverses the default name-ascending order, matching real AWS ListProjects.
func TestHandler_ListProjects_SortOrderDescending(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestProject(t, h, "sort-a")
	createTestProject(t, h, "sort-b")
	createTestProject(t, h, "sort-c")

	rec := doRequest(t, h, "ListProjects", map[string]any{"sortOrder": "DESCENDING"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		Projects []string `json:"projects"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, []string{"sort-c", "sort-b", "sort-a"}, out.Projects)
}

// TestHandler_ListProjects_InvalidNextToken verifies a malformed nextToken is
// rejected as InvalidInputException (400), rather than silently ignored.
func TestHandler_ListProjects_InvalidNextToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "ListProjects", map[string]any{"nextToken": "not-a-valid-token!!"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_ListProjects_SortByCreatedTime lives in whitebox_test.go: it
// needs direct access to unexported project timestamp fields to get a
// deterministic ordering independent of wall-clock creation time.

// TestHandler_ListFleets_MaxResultsPagination verifies maxResults/nextToken
// page through the fleet list, matching real AWS ListFleets pagination.
func TestHandler_ListFleets_MaxResultsPagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, name := range []string{"page-fleet-1", "page-fleet-2", "page-fleet-3"} {
		rec := doRequest(t, h, "CreateFleet", map[string]any{
			"name":            name,
			"baseCapacity":    1,
			"computeType":     "BUILD_GENERAL1_SMALL",
			"environmentType": "LINUX_CONTAINER",
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	firstRec := doRequest(t, h, "ListFleets", map[string]any{"maxResults": 2})
	require.Equal(t, http.StatusOK, firstRec.Code)

	var firstPage struct {
		NextToken string   `json:"nextToken"`
		Fleets    []string `json:"fleets"`
	}
	require.NoError(t, json.NewDecoder(firstRec.Body).Decode(&firstPage))
	assert.Len(t, firstPage.Fleets, 2)
	require.NotEmpty(t, firstPage.NextToken, "a partial page must carry a nextToken")

	secondRec := doRequest(t, h, "ListFleets", map[string]any{
		"maxResults": 2,
		"nextToken":  firstPage.NextToken,
	})
	require.Equal(t, http.StatusOK, secondRec.Code)

	var secondPage struct {
		NextToken string   `json:"nextToken"`
		Fleets    []string `json:"fleets"`
	}
	require.NoError(t, json.NewDecoder(secondRec.Body).Decode(&secondPage))
	assert.Len(t, secondPage.Fleets, 1)
	assert.Empty(t, secondPage.NextToken, "the final page must not carry a nextToken")

	// The two pages together must cover every fleet exactly once.
	all := append(append([]string{}, firstPage.Fleets...), secondPage.Fleets...)
	assert.Len(t, all, 3)
}

// TestHandler_ListBuildBatches_FilterByStatus verifies the filter.status
// request field narrows results to build batches in that status, matching
// real AWS's BuildBatchFilter.
func TestHandler_ListBuildBatches_FilterByStatus(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestProject(t, h, "batch-filter-proj")

	startRec1 := doRequest(t, h, "StartBuildBatch", map[string]any{"projectName": "batch-filter-proj"})
	require.Equal(t, http.StatusOK, startRec1.Code)

	var started1 struct {
		BuildBatch struct {
			ID string `json:"id"`
		} `json:"buildBatch"`
	}
	require.NoError(t, json.NewDecoder(startRec1.Body).Decode(&started1))

	stopRec := doRequest(t, h, "StopBuildBatch", map[string]any{"id": started1.BuildBatch.ID})
	require.Equal(t, http.StatusOK, stopRec.Code)

	startRec2 := doRequest(t, h, "StartBuildBatch", map[string]any{"projectName": "batch-filter-proj"})
	require.Equal(t, http.StatusOK, startRec2.Code)

	rec := doRequest(t, h, "ListBuildBatches", map[string]any{
		"filter": map[string]any{"status": "STOPPED"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		IDs []string `json:"ids"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	assert.Equal(t, []string{started1.BuildBatch.ID}, out.IDs)
}

// TestHandler_ListReports_FilterByStatus verifies the filter.status request
// field narrows ListReports/ListReportsForReportGroup results, matching real
// AWS's ReportFilter.
func TestHandler_ListReports_FilterByStatus(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rg, err := h.Backend.CreateReportGroup(
		"filter-rg", "TEST", codebuild.ReportExportConfig{ExportConfigType: "NO_EXPORT"}, nil,
	)
	require.NoError(t, err)
	rgArn := rg.Arn

	h.Backend.AddReportInternal(&codebuild.Report{
		Arn:            "arn:aws:codebuild:us-east-1:000000000000:report/filter-rg:ok",
		ReportGroupArn: rgArn,
		Status:         "SUCCEEDED",
	})
	h.Backend.AddReportInternal(&codebuild.Report{
		Arn:            "arn:aws:codebuild:us-east-1:000000000000:report/filter-rg:bad",
		ReportGroupArn: rgArn,
		Status:         "FAILED",
	})

	t.Run("list_reports", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, "ListReports", map[string]any{
			"filter": map[string]any{"status": "FAILED"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var out struct {
			Reports []string `json:"reports"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
		assert.Equal(t, []string{"arn:aws:codebuild:us-east-1:000000000000:report/filter-rg:bad"}, out.Reports)
	})

	t.Run("list_reports_for_report_group", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, "ListReportsForReportGroup", map[string]any{
			"reportGroupArn": rgArn,
			"filter":         map[string]any{"status": "SUCCEEDED"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var out struct {
			Reports []string `json:"reports"`
		}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
		assert.Equal(t, []string{"arn:aws:codebuild:us-east-1:000000000000:report/filter-rg:ok"}, out.Reports)
	})
}

// TestHandler_ListBuildsForProject_SortOrderBuildCountLimit verifies
// ListBuildsForProjectInput.SortOrder's documented limit (api_op_ListBuildsForProject.go:
// "If the project has more than 100 builds, setting the sort order will
// result in an error") is enforced, rather than silently sorting anyway.
func TestHandler_ListBuildsForProject_SortOrderBuildCountLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sortOrder  string
		buildCount int
		wantStatus int
	}{
		{
			name:       "over_100_builds_rejects_sort_order",
			buildCount: 101,
			sortOrder:  "ASCENDING",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "at_100_builds_allows_sort_order",
			buildCount: 100,
			sortOrder:  "ASCENDING",
			wantStatus: http.StatusOK,
		},
		{
			name:       "over_100_builds_without_sort_order_is_unaffected",
			buildCount: 101,
			sortOrder:  "",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createTestProject(t, h, "sort-limit-project")

			for range tt.buildCount {
				rec := doRequest(t, h, "StartBuild", map[string]any{"projectName": "sort-limit-project"})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			req := map[string]any{"projectName": "sort-limit-project"}
			if tt.sortOrder != "" {
				req["sortOrder"] = tt.sortOrder
			}

			rec := doRequest(t, h, "ListBuildsForProject", req)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
