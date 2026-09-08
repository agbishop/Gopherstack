package securityhub_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/securityhub"
)

// Batch-1 accuracy gap: CreateInsight is POST /insights, returns InsightArn.
func TestCreateInsightPath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/accounts", nil)

	rec := doRequest(t, h, http.MethodPost, "/insights", map[string]any{
		"Name":             "test-insight",
		"GroupByAttribute": "ResourceType",
		"Filters":          map[string]any{},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	arn, _ := resp["InsightArn"].(string)
	assert.NotEmpty(t, arn)
	assert.Contains(t, arn, "arn:aws:securityhub:")
	assert.Contains(t, arn, ":insight/")
}

// Batch-1 accuracy gap: GetInsights is POST /insights/get (not GET /insights).
func TestGetInsightsIsPOSTInsightsGet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/accounts", nil)

	rec := doRequest(t, h, http.MethodPost, "/insights/get", map[string]any{})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Contains(t, resp, "Insights")
}

// Batch-1 accuracy gap: GetInsightResults is GET /insights/results/{InsightArn}.
func TestGetInsightResultsIsGETInsightsResults(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/accounts", nil)

	// Create an insight
	createRec := doRequest(t, h, http.MethodPost, "/insights", map[string]any{
		"Name":             "result-test",
		"GroupByAttribute": "SeverityLabel",
		"Filters":          map[string]any{},
	})

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))

	insightArn, _ := createResp["InsightArn"].(string)
	require.NotEmpty(t, insightArn)

	rec := doRequest(t, h, http.MethodGet, "/insights/results/"+insightArn, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Response is wrapped in InsightResults key
	insightResults, hasWrapper := resp["InsightResults"]
	assert.True(t, hasWrapper, "GetInsightResults must return 'InsightResults' wrapper")

	if resultsMap, ok := insightResults.(map[string]any); ok {
		assert.Contains(t, resultsMap, "InsightArn")
		assert.Contains(t, resultsMap, "GroupByAttribute")
		assert.Contains(t, resultsMap, "ResultValues")
	}
}

// Batch-1 accuracy gap: UpdateInsight is PATCH /insights/{InsightArn}.
func TestUpdateInsightIsPATCHInsightsArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/accounts", nil)

	createRec := doRequest(t, h, http.MethodPost, "/insights", map[string]any{
		"Name":             "to-update",
		"GroupByAttribute": "ResourceType",
		"Filters":          map[string]any{},
	})

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))

	insightArn, _ := createResp["InsightArn"].(string)
	require.NotEmpty(t, insightArn)

	rec := doRequest(t, h, http.MethodPatch, "/insights/"+insightArn, map[string]any{
		"Name": "updated-name",
	})

	assert.Equal(t, http.StatusOK, rec.Code)
}

// Batch-1 accuracy gap: DeleteInsight is DELETE /insights/{InsightArn}, returns InsightArn.
func TestDeleteInsightIsDELETEInsightsArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/accounts", nil)

	createRec := doRequest(t, h, http.MethodPost, "/insights", map[string]any{
		"Name":             "to-delete",
		"GroupByAttribute": "ResourceType",
		"Filters":          map[string]any{},
	})

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))

	insightArn, _ := createResp["InsightArn"].(string)
	require.NotEmpty(t, insightArn)

	rec := doRequest(t, h, http.MethodDelete, "/insights/"+insightArn, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	deletedArn, _ := resp["InsightArn"].(string)
	assert.Equal(t, insightArn, deletedArn, "DeleteInsight must return the deleted InsightArn")
}

// TestParity_CreateInsightRequiresNameAndGroupByAttribute verifies that
// CreateInsight rejects requests with a missing Name or GroupByAttribute.
// Real AWS returns 400 for both; the emulator previously silently stored
// insights with empty required fields.
func TestCreateInsightRequiresNameAndGroupByAttribute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "absent_name_rejected",
			body: map[string]any{
				"GroupByAttribute": "ProductName",
				"Filters":          map[string]any{},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "empty_name_rejected",
			body: map[string]any{
				"Name":             "",
				"GroupByAttribute": "ProductName",
				"Filters":          map[string]any{},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "absent_group_by_attribute_rejected",
			body: map[string]any{
				"Name":    "my-insight",
				"Filters": map[string]any{},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "valid_insight_accepted",
			body: map[string]any{
				"Name":             "my-insight",
				"GroupByAttribute": "ProductName",
				"Filters":          map[string]any{},
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			enableHub(t, h)
			rec := doRequest(t, h, http.MethodPost, "/insights", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code,
				"CreateInsight status for case %q", tt.name)
		})
	}
}

func TestBackend_GetInsights_Pagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxResults int
		wantLen    int
		wantToken  bool
	}{
		{name: "get all insights", maxResults: 100, wantLen: 2, wantToken: false},
		{name: "paginate with max 1", maxResults: 1, wantLen: 1, wantToken: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := securityhub.NewInMemoryBackend("000000000000", "us-east-1")
			require.NoError(t, b.EnableHub(false, nil))

			_, err := b.CreateInsight("Insight1", "SeverityLabel", map[string]any{})
			require.NoError(t, err)
			_, err = b.CreateInsight("Insight2", "Type", map[string]any{})
			require.NoError(t, err)

			results, nextToken, err := b.GetInsights(nil, "", tc.maxResults)
			require.NoError(t, err)
			assert.Len(t, results, tc.wantLen)
			if tc.wantToken {
				assert.NotEmpty(t, nextToken)
			} else {
				assert.Empty(t, nextToken)
			}
		})
	}
}

func TestBackend_GetInsights_ByArn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantLen int
	}{
		{name: "get specific insight by ARN", wantLen: 1},
		{name: "get non-existent ARN returns empty", wantLen: 0},
	}

	b := securityhub.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b.EnableHub(false, nil))
	arn, err := b.CreateInsight("SomeInsight", "SeverityLabel", map[string]any{})
	require.NoError(t, err)

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var arns []string
			if i == 0 {
				arns = []string{arn}
			} else {
				arns = []string{"arn:nonexistent"}
			}
			results, _, err2 := b.GetInsights(arns, "", 100)
			require.NoError(t, err2)
			assert.Len(t, results, tc.wantLen)
		})
	}
}

func TestBackend_UpdateInsight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		newFilters map[string]any
		name       string
		newName    string
		newGroupBy string
		wantErrMsg string
		notFound   bool
	}{
		{
			name:    "update name",
			newName: "NewName",
		},
		{
			name:       "update groupByAttribute",
			newGroupBy: "Type",
		},
		{
			name:       "update filters",
			newFilters: map[string]any{"SeverityLabel": []any{"HIGH"}},
		},
		{
			name:       "not found returns error",
			notFound:   true,
			wantErrMsg: "not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := securityhub.NewInMemoryBackend("000000000000", "us-east-1")
			require.NoError(t, b.EnableHub(false, nil))

			var insightArn string
			if !tc.notFound {
				var err error
				insightArn, err = b.CreateInsight("Original", "SeverityLabel", map[string]any{})
				require.NoError(t, err)
			} else {
				insightArn = "arn:aws:securityhub:us-east-1:000000000000:insight/x/99"
			}

			err := b.UpdateInsight(insightArn, tc.newName, tc.newGroupBy, tc.newFilters)
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBackend_GetInsightResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantErrMsg string
		preCreate  bool
	}{
		{name: "get results for existing insight", preCreate: true},
		{name: "get results for missing insight", preCreate: false, wantErrMsg: "not found"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := securityhub.NewInMemoryBackend("000000000000", "us-east-1")
			require.NoError(t, b.EnableHub(false, nil))

			var insightArn string
			if tc.preCreate {
				var err error
				insightArn, err = b.CreateInsight("TestInsight", "SeverityLabel", map[string]any{})
				require.NoError(t, err)
			} else {
				insightArn = "arn:aws:securityhub:us-east-1:000000000000:insight/x/999"
			}

			result, err := b.GetInsightResults(insightArn)
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, insightArn, result.InsightArn)
				assert.Equal(t, "SeverityLabel", result.GroupByAttribute)
			}
		})
	}
}

func TestBackend_DeleteInsight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantErrMsg string
		preCreate  bool
	}{
		{name: "delete existing insight", preCreate: true},
		{
			name:       "delete non-existent insight returns error",
			preCreate:  false,
			wantErrMsg: "not found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := securityhub.NewInMemoryBackend("000000000000", "us-east-1")
			require.NoError(t, b.EnableHub(false, nil))

			var insightArn string
			if tc.preCreate {
				var err error
				insightArn, err = b.CreateInsight("ToDelete", "SeverityLabel", map[string]any{})
				require.NoError(t, err)
			} else {
				insightArn = "arn:aws:securityhub:us-east-1:000000000000:insight/x/999"
			}

			returnedArn, err := b.DeleteInsight(insightArn)
			if tc.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, insightArn, returnedArn)
				assert.Equal(t, 0, securityhub.InsightCount(b))
			}
		})
	}
}

// gopherstack-1qf: GetInsightResults always returned empty ResultValues
// ("no real aggregation in mock") regardless of the insight's
// GroupByAttribute/Filters or how many findings were imported.
func TestBackend_GetInsightResults_AggregatesFindings(t *testing.T) {
	t.Parallel()

	b := securityhub.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b.EnableHub(false, nil))

	_, failed, failedFindings := b.ImportFindings([]map[string]any{
		securityhub.ValidFinding(map[string]any{"Id": "f1", "Severity": map[string]any{"Label": "HIGH"}}),
		securityhub.ValidFinding(map[string]any{"Id": "f2", "Severity": map[string]any{"Label": "HIGH"}}),
		securityhub.ValidFinding(map[string]any{"Id": "f3", "Severity": map[string]any{"Label": "LOW"}}),
	})
	require.Zero(t, failed, "%v", failedFindings)

	insightArn, err := b.CreateInsight("by-severity", "SeverityLabel", map[string]any{})
	require.NoError(t, err)

	result, err := b.GetInsightResults(insightArn)
	require.NoError(t, err)

	counts := make(map[string]int)

	for _, rv := range result.ResultValues {
		val, _ := rv["GroupByAttributeValue"].(string)
		count, _ := rv["Count"].(int)
		counts[val] = count
	}

	assert.Equal(t, 2, counts["HIGH"])
	assert.Equal(t, 1, counts["LOW"])
}
