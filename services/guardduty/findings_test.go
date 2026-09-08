package guardduty_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/guardduty"
)

func TestFindings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, h *guardduty.Handler, detectorID string)
		name string
	}{
		{
			name: "create_sample_and_list",
			fn: func(t *testing.T, h *guardduty.Handler, detectorID string) {
				t.Helper()
				rec := doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/create", map[string]any{
					"findingTypes": []string{"UnauthorizedAccess:IAMUser/ConsoleLoginSuccess.B"},
				})
				require.Equal(t, http.StatusOK, rec.Code)

				rec = doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings", nil)
				require.Equal(t, http.StatusOK, rec.Code)

				var lr map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lr))
				ids, ok := lr["findingIds"].([]any)
				require.True(t, ok)
				assert.Len(t, ids, 1)
			},
		},
		{
			name: "get_findings_by_id",
			fn: func(t *testing.T, h *guardduty.Handler, detectorID string) {
				t.Helper()
				doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/create", map[string]any{
					"findingTypes": []string{"UnauthorizedAccess:IAMUser/ConsoleLoginSuccess.B"},
				})

				// List to get IDs
				rec := doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings", nil)
				var lr map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lr))
				ids := lr["findingIds"].([]any)

				// Get by ID
				rec = doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/get", map[string]any{
					"findingIds": []string{ids[0].(string)},
				})
				require.Equal(t, http.StatusOK, rec.Code)

				var gr map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &gr))
				findings, ok := gr["findings"].([]any)
				require.True(t, ok)
				require.Len(t, findings, 1)
			},
		},
		{
			name: "archive_and_unarchive",
			fn: func(t *testing.T, h *guardduty.Handler, detectorID string) {
				t.Helper()
				doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/create", map[string]any{
					"findingTypes": []string{"UnauthorizedAccess:IAMUser/ConsoleLoginSuccess.B"},
				})

				rec := doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings", nil)
				var lr map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lr))
				ids := lr["findingIds"].([]any)
				fid := ids[0].(string)

				// Archive
				rec = doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/archive", map[string]any{
					"findingIds": []string{fid},
				})
				require.Equal(t, http.StatusOK, rec.Code)

				// Verify archived
				rec = doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/get", map[string]any{
					"findingIds": []string{fid},
				})
				var gr map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &gr))
				findings := gr["findings"].([]any)
				f := findings[0].(map[string]any)
				svc := f["service"].(map[string]any)
				assert.Equal(t, true, svc["archived"])

				// Unarchive
				rec = doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/unarchive", map[string]any{
					"findingIds": []string{fid},
				})
				require.Equal(t, http.StatusOK, rec.Code)

				rec = doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/get", map[string]any{
					"findingIds": []string{fid},
				})
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &gr))
				findings = gr["findings"].([]any)
				f = findings[0].(map[string]any)
				svc = f["service"].(map[string]any)
				assert.Equal(t, false, svc["archived"])
			},
		},
		{
			name: "get_findings_statistics",
			fn: func(t *testing.T, h *guardduty.Handler, detectorID string) {
				t.Helper()
				doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/create", map[string]any{
					"findingTypes": []string{
						"UnauthorizedAccess:IAMUser/ConsoleLoginSuccess.B",
						"Backdoor:EC2/DenialOfService.Tcp",
					},
				})

				rec := doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/statistics", nil)
				require.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotNil(t, resp["findingStatistics"])
			},
		},
		{
			name: "update_findings_feedback",
			fn: func(t *testing.T, h *guardduty.Handler, detectorID string) {
				t.Helper()
				doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/create", map[string]any{
					"findingTypes": []string{"UnauthorizedAccess:IAMUser/ConsoleLoginSuccess.B"},
				})

				rec := doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings", nil)
				var lr map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lr))
				ids := lr["findingIds"].([]any)

				rec = doRequest(t, h, http.MethodPost, "/detector/"+detectorID+"/findings/feedback", map[string]any{
					"findingIds": []string{ids[0].(string)},
					"feedback":   "USEFUL",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			detectorID := createTestDetector(t, h)
			tt.fn(t, h, detectorID)
		})
	}
}

func TestListFindings_Empty_State(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id := createTestDetector(t, h)

	rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/findings", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	raw, exists := resp["findingIds"]
	require.True(t, exists, "ListFindings must include 'findingIds' key")
	assert.NotNil(t, raw, "findingIds must be [] not null when empty")

	ids, ok := raw.([]any)
	require.True(t, ok, "findingIds must be an array, got %T", raw)
	assert.Empty(t, ids, "findingIds must be empty []")
}

func TestGetFindings_Empty_Input(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id := createTestDetector(t, h)

	rec := doRequest(t, h, http.MethodPost, "/detector/"+id+"/findings/get",
		map[string]any{"findingIds": []string{}})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	raw, exists := resp["findings"]
	require.True(t, exists, "GetFindings must include 'findings' key")
	assert.NotNil(t, raw, "findings must be [] not null when empty input")

	findings, ok := raw.([]any)
	require.True(t, ok, "findings must be an array, got %T", raw)
	assert.Empty(t, findings, "findings must be empty []")
}

// TestGetFindingsStatistics_SeverityKeys verifies that countBySeverity
// uses the actual finding severity float as string key (e.g. "5.0") rather than
// the former incorrect label strings "Low"/"Medium"/"High".
func TestGetFindingsStatistics_SeverityKeys(t *testing.T) {
	t.Parallel()

	b := guardduty.NewInMemoryBackend("111111111111", "us-east-1")
	det, err := b.CreateDetector(true, "", nil, nil)
	require.NoError(t, err)
	detID := det.DetectorID

	// CreateSampleFindings produces findings with severity 5.0 by default.
	require.NoError(t, b.CreateSampleFindings(detID, nil))

	stats, err := b.GetFindingsStatistics(detID, guardduty.FindingStatisticsQuery{})
	require.NoError(t, err)

	fs, ok := stats["findingStatistics"].(map[string]any)
	require.True(t, ok, "findingStatistics must be a map")

	cbs, ok := fs["countBySeverity"].(map[string]int)
	require.True(t, ok, "countBySeverity must be map[string]int")

	// Key must be numeric severity string, not a label.
	assert.Contains(t, cbs, "5.0", "expected severity key '5.0'")
	assert.NotContains(t, cbs, "Low", "must not use label keys")
	assert.NotContains(t, cbs, "Medium", "must not use label keys")
	assert.NotContains(t, cbs, "High", "must not use label keys")
	assert.Equal(t, 1, cbs["5.0"])
}

// TestGetFindingsStatistics_Empty verifies an empty detector returns
// an empty countBySeverity map, not one pre-populated with zero-count labels.
func TestGetFindingsStatistics_Empty(t *testing.T) {
	t.Parallel()

	b := guardduty.NewInMemoryBackend("111111111111", "us-east-1")
	det, err := b.CreateDetector(true, "", nil, nil)
	require.NoError(t, err)

	stats, err := b.GetFindingsStatistics(det.DetectorID, guardduty.FindingStatisticsQuery{})
	require.NoError(t, err)

	fs := stats["findingStatistics"].(map[string]any)
	cbs := fs["countBySeverity"].(map[string]int)
	assert.Empty(t, cbs, "no findings → empty countBySeverity")
}

// TestUpdateFindingsFeedback verifies feedback is stored on the finding
// and updatedAt is refreshed — previously this was a complete stub.
func TestUpdateFindingsFeedback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		feedback string
	}{
		{"useful", "USEFUL"},
		{"not_useful", "NOT_USEFUL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := guardduty.NewInMemoryBackend("111111111111", "us-east-1")
			det, err := b.CreateDetector(true, "", nil, nil)
			require.NoError(t, err)
			detID := det.DetectorID

			require.NoError(t, b.CreateSampleFindings(detID, nil))

			ids, _, err := b.ListFindings(detID, guardduty.FindingsQuery{})
			require.NoError(t, err)
			require.NotEmpty(t, ids)

			before := time.Now().Add(-time.Second)
			require.NoError(t, b.UpdateFindingsFeedback(detID, ids, tc.feedback))

			findings, err := b.GetFindings(detID, ids)
			require.NoError(t, err)
			require.Len(t, findings, len(ids))

			for _, f := range findings {
				assert.Equal(t, tc.feedback, f.Service.UserFeedback,
					"userFeedback must be stored on finding")

				updatedAt, parseErr := time.Parse(time.RFC3339, f.UpdatedAt)
				require.NoError(t, parseErr, "updatedAt must be RFC3339")
				assert.True(t, updatedAt.After(before),
					"updatedAt must be refreshed after feedback")
			}
		})
	}
}

// TestArchiveFindings_UpdatesTimestamp verifies that ArchiveFindings
// refreshes the finding's updatedAt — previously it was not updated.
func TestArchiveFindings_UpdatesTimestamp(t *testing.T) {
	t.Parallel()

	b := guardduty.NewInMemoryBackend("111111111111", "us-east-1")
	det, err := b.CreateDetector(true, "", nil, nil)
	require.NoError(t, err)
	detID := det.DetectorID

	require.NoError(t, b.CreateSampleFindings(detID, nil))

	ids, _, err := b.ListFindings(detID, guardduty.FindingsQuery{})
	require.NoError(t, err)
	require.NotEmpty(t, ids)

	before := time.Now().Add(-time.Second)

	require.NoError(t, b.ArchiveFindings(detID, ids))

	findings1, err := b.GetFindings(detID, ids)
	require.NoError(t, err)

	for _, f := range findings1 {
		assert.True(t, f.Service.Archived, "finding must be archived")

		updatedAt, parseErr := time.Parse(time.RFC3339, f.UpdatedAt)
		require.NoError(t, parseErr)
		assert.True(t, updatedAt.After(before),
			"updatedAt must be refreshed on archive")
	}
}

// TestUnarchiveFindings_UpdatesTimestamp verifies that UnarchiveFindings
// refreshes the finding's updatedAt — previously it was not updated.
func TestUnarchiveFindings_UpdatesTimestamp(t *testing.T) {
	t.Parallel()

	b := guardduty.NewInMemoryBackend("111111111111", "us-east-1")
	det, err := b.CreateDetector(true, "", nil, nil)
	require.NoError(t, err)
	detID := det.DetectorID

	require.NoError(t, b.CreateSampleFindings(detID, nil))

	ids, _, err := b.ListFindings(detID, guardduty.FindingsQuery{})
	require.NoError(t, err)
	require.NotEmpty(t, ids)

	// Archive first.
	require.NoError(t, b.ArchiveFindings(detID, ids))

	before := time.Now().Add(-time.Second)
	require.NoError(t, b.UnarchiveFindings(detID, ids))

	findings, err := b.GetFindings(detID, ids)
	require.NoError(t, err)

	for _, f := range findings {
		assert.False(t, f.Service.Archived, "finding must be unarchived")

		updatedAt, parseErr := time.Parse(time.RFC3339, f.UpdatedAt)
		require.NoError(t, parseErr)
		assert.True(t, updatedAt.After(before),
			"updatedAt must be refreshed on unarchive")
	}
}

// TestCreateSampleFindings_AppliesArchiveFilter verifies that a saved
// filter's Action ("the action that is to be applied to the findings that
// match the filter") is actually applied to findings, not merely stored and
// echoed back on Get/List/Update -- CreateSampleFindings is this backend's
// only finding-creation path, so it is the only place Action can ever take
// effect.
func TestCreateSampleFindings_AppliesArchiveFilter(t *testing.T) {
	t.Parallel()

	const findingType = "UnauthorizedAccess:IAMUser/ConsoleLoginSuccess.B"

	b := guardduty.NewInMemoryBackend("111111111111", "us-east-1")
	det, err := b.CreateDetector(true, "", nil, nil)
	require.NoError(t, err)
	detID := det.DetectorID

	_, err = b.CreateFilter(detID, "archive-console-logins", "", "ARCHIVE", 1,
		map[string]any{"criterion": map[string]any{
			"type": map[string]any{"equals": []string{findingType}},
		}}, nil)
	require.NoError(t, err)

	require.NoError(t, b.CreateSampleFindings(detID, []string{findingType}))

	ids, _, err := b.ListFindings(detID, guardduty.FindingsQuery{})
	require.NoError(t, err)
	require.Len(t, ids, 1)

	findings, err := b.GetFindings(detID, ids)
	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.True(t, findings[0].Service.Archived,
		"a finding matching an ARCHIVE filter must be created already archived")
}

// TestGetFindingsStatistics_Handler verifies the HTTP layer returns
// findingStatistics with numeric severity keys.
func TestGetFindingsStatistics_Handler(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	detID := createTestDetector(t, h)

	// Create sample findings via HTTP.
	rec := doRequest(t, h, http.MethodPost, "/detector/"+detID+"/findings/create", map[string]any{
		"findingTypes": []string{"Recon:IAMUser/TorIPCaller"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodPost, "/detector/"+detID+"/findings/statistics", map[string]any{
		"findingStatisticTypes": []string{"COUNT_BY_SEVERITY"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	fs, ok := resp["findingStatistics"].(map[string]any)
	require.True(t, ok)

	cbs, ok := fs["countBySeverity"].(map[string]any)
	require.True(t, ok)

	// Must have a numeric severity key, not "Low"/"Medium"/"High".
	assert.NotContains(t, cbs, "Low")
	assert.NotContains(t, cbs, "High")
	assert.NotContains(t, cbs, "Medium")
}
