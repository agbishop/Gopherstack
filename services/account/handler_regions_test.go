package account_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/account"
)

func TestHandler_ListRegions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filter    []string
		wantCount int
	}{
		{name: "no_filter", wantCount: 10},
		{name: "filter_enabled_default", filter: []string{"ENABLED_BY_DEFAULT"}, wantCount: 8},
		{name: "filter_enabled", filter: []string{"ENABLED"}, wantCount: 2},
		{name: "filter_disabled", filter: []string{"DISABLED"}, wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := map[string]any{}

			if tt.filter != nil {
				body["RegionOptStatusContains"] = tt.filter
			}

			rec := doRequest(t, h, "/listRegions", body)
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				Regions []account.Region `json:"Regions"`
			}
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
			assert.Len(t, out.Regions, tt.wantCount)
		})
	}
}

func TestHandler_ListRegions_RegionFieldsPresent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "/listRegions", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		Regions []map[string]any `json:"Regions"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotEmpty(t, out.Regions)

	for _, r := range out.Regions {
		assert.Contains(t, r, "RegionName")
		assert.Contains(t, r, "RegionOptStatus")
	}
}

func TestHandler_ListRegions_Alphabetical(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "/listRegions", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		Regions []account.Region `json:"Regions"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
	require.NotEmpty(t, out.Regions)

	for i := 1; i < len(out.Regions); i++ {
		assert.Less(t, out.Regions[i-1].RegionName, out.Regions[i].RegionName)
	}
}

// TestHandler_ListRegions_Pagination verifies that MaxResults limits page
// size and that the returned NextToken allows full traversal with no
// duplication or gaps.
func TestHandler_ListRegions_Pagination(t *testing.T) {
	t.Parallel()

	const totalRegions = 10

	tests := []struct {
		name       string
		maxResults int
		wantPages  int
	}{
		{"maxResults_1", 1, totalRegions},
		{"maxResults_3", 3, 4},
		{"maxResults_4", 4, 3},
		{"maxResults_10_exact", 10, 1},
		{"maxResults_50_exceeds_total", 50, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			seen := map[string]int{}
			nextToken := ""
			pages := 0

			for {
				body := map[string]any{"MaxResults": tc.maxResults}
				if nextToken != "" {
					body["NextToken"] = nextToken
				}

				rec := doRequest(t, h, "/listRegions", body)
				require.Equal(t, http.StatusOK, rec.Code)

				var resp struct {
					NextToken string           `json:"NextToken"`
					Regions   []map[string]any `json:"Regions"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				require.LessOrEqual(t, len(resp.Regions), tc.maxResults)

				for _, r := range resp.Regions {
					name, _ := r["RegionName"].(string)
					seen[name]++
				}

				pages++
				nextToken = resp.NextToken

				if nextToken == "" {
					break
				}

				require.Less(t, pages, 20, "pagination must terminate")
			}

			assert.Equal(t, tc.wantPages, pages)
			assert.Len(t, seen, totalRegions)

			for name, count := range seen {
				assert.Equalf(t, 1, count, "region %s visited %d times", name, count)
			}
		})
	}
}

// TestHandler_ListRegions_InvalidNextToken verifies an undecodable pagination
// cursor surfaces as ValidationException/400 through the full handler stack.
func TestHandler_ListRegions_InvalidNextToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "/listRegions", map[string]any{"NextToken": "not-valid-base64!!"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "ValidationException", rec.Header().Get("X-Amzn-Errortype"))
}

func TestHandler_ListRegions_InvalidMaxResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxResults int
	}{
		{name: "negative", maxResults: -1},
		{name: "over_max", maxResults: 51},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "/listRegions", map[string]any{"MaxResults": tt.maxResults})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_GetRegionOptStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		regionName     string
		wantStatus     string
		wantHTTPStatus int
	}{
		{
			name: "enabled_by_default", regionName: "us-east-1",
			wantStatus: "ENABLED_BY_DEFAULT", wantHTTPStatus: http.StatusOK,
		},
		{
			name: "opt_in_enabled", regionName: "af-south-1",
			wantStatus: "ENABLED", wantHTTPStatus: http.StatusOK,
		},
		{
			name: "non_opt_in_enabled_by_default", regionName: "ap-southeast-1",
			wantStatus: "ENABLED_BY_DEFAULT", wantHTTPStatus: http.StatusOK,
		},
		{name: "unknown_region", regionName: "zz-fake-1", wantHTTPStatus: http.StatusBadRequest},
		{name: "missing_region_name", regionName: "", wantHTTPStatus: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			body := map[string]any{}
			if tc.regionName != "" {
				body["RegionName"] = tc.regionName
			}

			rec := doRequest(t, h, "/getRegionOptStatus", body)
			require.Equal(t, tc.wantHTTPStatus, rec.Code)

			if tc.wantStatus != "" {
				var out map[string]any
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))
				assert.Equal(t, tc.regionName, out["RegionName"])
				assert.Equal(t, tc.wantStatus, out["RegionOptStatus"])
			}
		})
	}
}

func TestHandler_EnableDisableRegion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		regionName string
	}{
		{name: "af-south-1", regionName: "af-south-1"},
		{name: "ap-east-1", regionName: "ap-east-1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			statusRec := doRequest(t, h, "/getRegionOptStatus", map[string]any{"RegionName": tc.regionName})
			require.Equal(t, http.StatusOK, statusRec.Code)

			var statusOut map[string]any
			require.NoError(t, json.NewDecoder(statusRec.Body).Decode(&statusOut))
			assert.Equal(t, "ENABLED", statusOut["RegionOptStatus"])

			disableRec := doRequest(t, h, "/disableRegion", map[string]any{"RegionName": tc.regionName})
			assert.Equal(t, http.StatusOK, disableRec.Code)

			afterDisableRec := doRequest(t, h, "/getRegionOptStatus", map[string]any{"RegionName": tc.regionName})
			require.Equal(t, http.StatusOK, afterDisableRec.Code)

			var afterDisable map[string]any
			require.NoError(t, json.NewDecoder(afterDisableRec.Body).Decode(&afterDisable))
			assert.Equal(t, "DISABLED", afterDisable["RegionOptStatus"])

			enableRec := doRequest(t, h, "/enableRegion", map[string]any{"RegionName": tc.regionName})
			assert.Equal(t, http.StatusOK, enableRec.Code)

			afterEnableRec := doRequest(t, h, "/getRegionOptStatus", map[string]any{"RegionName": tc.regionName})
			require.Equal(t, http.StatusOK, afterEnableRec.Code)

			var afterEnable map[string]any
			require.NoError(t, json.NewDecoder(afterEnableRec.Body).Decode(&afterEnable))
			assert.Equal(t, "ENABLED", afterEnable["RegionOptStatus"])
		})
	}
}

func TestHandler_EnableDisableRegion_DefaultRegionRejected(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/enableRegion", "/disableRegion"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, path, map[string]any{"RegionName": "us-east-1"})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_EnableDisableRegion_MissingRegionName(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/enableRegion", "/disableRegion"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, path, map[string]any{})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestHandler_EnableDisableRegion_UnknownRegionName verifies an unrecognized
// RegionName is reported as ValidationException/400, not
// ResourceNotFoundException/404: EnableRegion and DisableRegion's real
// modeled error sets (verified against aws-sdk-go-v2/service/account and the
// public API reference) contain AccessDeniedException, ConflictException,
// InternalServerException, TooManyRequestsException and ValidationException
// -- ResourceNotFoundException is not a possible error for either operation.
func TestHandler_EnableDisableRegion_UnknownRegionName(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/enableRegion", "/disableRegion"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, path, map[string]any{"RegionName": "zz-invalid-1"})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "ValidationException", rec.Header().Get("X-Amzn-Errortype"))
		})
	}
}
