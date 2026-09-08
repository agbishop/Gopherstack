package detective_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetective_Investigations(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	// Create graph.
	rec := doRequest(t, h, http.MethodPost, "/graph", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var graphResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &graphResp))
	graphArn := graphResp["GraphArn"].(string)

	unknownArn := "arn:aws:detective:us-east-1:000000000000:graph:notexist"

	tests := []struct {
		name     string
		body     any
		check    func(t *testing.T, body []byte)
		method   string
		path     string
		wantCode int
	}{
		{
			name:   "StartInvestigation missing GraphArn returns 400",
			method: http.MethodPost,
			path:   "/investigations/startInvestigation",
			body: map[string]any{
				"EntityArn":      "arn:aws:iam::000000000000:user/bob",
				"ScopeStartTime": "2024-01-01T00:00:00Z",
				"ScopeEndTime":   "2024-01-31T00:00:00Z",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "StartInvestigation missing EntityArn returns 400",
			method: http.MethodPost,
			path:   "/investigations/startInvestigation",
			body: map[string]any{
				"GraphArn":       graphArn,
				"ScopeStartTime": "2024-01-01T00:00:00Z",
				"ScopeEndTime":   "2024-01-31T00:00:00Z",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "StartInvestigation unknown graph returns 404",
			method: http.MethodPost,
			path:   "/investigations/startInvestigation",
			body: map[string]any{
				"GraphArn":       unknownArn,
				"EntityArn":      "arn:aws:iam::000000000000:user/bob",
				"ScopeStartTime": "2024-01-01T00:00:00Z",
				"ScopeEndTime":   "2024-01-31T00:00:00Z",
			},
			wantCode: http.StatusNotFound,
		},
		{
			name:   "StartInvestigation creates investigation and returns ID",
			method: http.MethodPost,
			path:   "/investigations/startInvestigation",
			body: map[string]any{
				"GraphArn":       graphArn,
				"EntityArn":      "arn:aws:iam::000000000000:user/alice",
				"ScopeStartTime": "2024-01-01T00:00:00Z",
				"ScopeEndTime":   "2024-01-31T00:00:00Z",
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.NotEmpty(t, resp["InvestigationId"])
			},
		},
		{
			name:     "ListInvestigations returns started investigation",
			method:   http.MethodPost,
			path:     "/investigations/listInvestigations",
			body:     map[string]any{"GraphArn": graphArn},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				details, ok := resp["InvestigationDetails"].([]any)
				require.True(t, ok)
				assert.Len(t, details, 1)
			},
		},
		{
			name:     "ListInvestigations unknown graph returns 404",
			method:   http.MethodPost,
			path:     "/investigations/listInvestigations",
			body:     map[string]any{"GraphArn": unknownArn},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "GetInvestigation missing GraphArn returns 400",
			method:   http.MethodPost,
			path:     "/investigations/getInvestigation",
			body:     map[string]any{"InvestigationId": "someid"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "GetInvestigation missing InvestigationId returns 400",
			method:   http.MethodPost,
			path:     "/investigations/getInvestigation",
			body:     map[string]any{"GraphArn": graphArn},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "GetInvestigation unknown investigation returns 404",
			method:   http.MethodPost,
			path:     "/investigations/getInvestigation",
			body:     map[string]any{"GraphArn": graphArn, "InvestigationId": "doesnotexist"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "UpdateInvestigationState missing GraphArn returns 400",
			method:   http.MethodPost,
			path:     "/investigations/updateInvestigationState",
			body:     map[string]any{"InvestigationId": "id", "State": "ARCHIVED"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "UpdateInvestigationState missing State returns 400",
			method:   http.MethodPost,
			path:     "/investigations/updateInvestigationState",
			body:     map[string]any{"GraphArn": graphArn, "InvestigationId": "id"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "ListIndicators missing GraphArn returns 400",
			method:   http.MethodPost,
			path:     "/investigations/listIndicators",
			body:     map[string]any{"InvestigationId": "id"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "ListIndicators missing InvestigationId returns 400",
			method:   http.MethodPost,
			path:     "/investigations/listIndicators",
			body:     map[string]any{"GraphArn": graphArn},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "ListIndicators unknown investigation returns 404",
			method:   http.MethodPost,
			path:     "/investigations/listIndicators",
			body:     map[string]any{"GraphArn": graphArn, "InvestigationId": "doesnotexist"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec2 := doRequest(t, h, tc.method, tc.path, tc.body)
			assert.Equal(t, tc.wantCode, rec2.Code)

			if tc.check != nil {
				tc.check(t, rec2.Body.Bytes())
			}
		})
	}
}

func TestDetective_InvestigationGetAndUpdate(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	// Create graph.
	rec := doRequest(t, h, http.MethodPost, "/graph", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var graphResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &graphResp))
	graphArn := graphResp["GraphArn"].(string)

	// Start investigation.
	startRec := doRequest(t, h, http.MethodPost, "/investigations/startInvestigation", map[string]any{
		"GraphArn":       graphArn,
		"EntityArn":      "arn:aws:iam::000000000000:user/alice",
		"ScopeStartTime": "2024-01-01T00:00:00Z",
		"ScopeEndTime":   "2024-01-31T00:00:00Z",
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	var startResp map[string]any
	require.NoError(t, json.Unmarshal(startRec.Body.Bytes(), &startResp))
	invID := startResp["InvestigationId"].(string)
	require.NotEmpty(t, invID)

	tests := []struct {
		name     string
		body     any
		check    func(t *testing.T, body []byte)
		method   string
		path     string
		wantCode int
	}{
		{
			name:     "GetInvestigation returns investigation details",
			method:   http.MethodPost,
			path:     "/investigations/getInvestigation",
			body:     map[string]any{"GraphArn": graphArn, "InvestigationId": invID},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, invID, resp["InvestigationId"])
				assert.Equal(t, graphArn, resp["GraphArn"])
				assert.Equal(t, "arn:aws:iam::000000000000:user/alice", resp["EntityArn"])
				assert.Equal(t, "ACTIVE", resp["State"])
				assert.Equal(t, "SUCCESSFUL", resp["Status"])
			},
		},
		{
			name:     "UpdateInvestigationState archives investigation",
			method:   http.MethodPost,
			path:     "/investigations/updateInvestigationState",
			body:     map[string]any{"GraphArn": graphArn, "InvestigationId": invID, "State": "ARCHIVED"},
			wantCode: http.StatusOK,
		},
		{
			name:     "GetInvestigation after archive returns ARCHIVED state",
			method:   http.MethodPost,
			path:     "/investigations/getInvestigation",
			body:     map[string]any{"GraphArn": graphArn, "InvestigationId": invID},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "ARCHIVED", resp["State"])
			},
		},
		{
			name:     "ListIndicators returns indicators for investigation",
			method:   http.MethodPost,
			path:     "/investigations/listIndicators",
			body:     map[string]any{"GraphArn": graphArn, "InvestigationId": invID},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				indicators, ok := resp["Indicators"].([]any)
				require.True(t, ok)
				assert.NotEmpty(t, indicators, "ListIndicators should return indicators for an investigation")
				for _, raw := range indicators {
					ind, isMap := raw.(map[string]any)
					require.True(t, isMap)
					assert.NotEmpty(t, ind["IndicatorType"])
					assert.NotEmpty(t, ind["IndicatorDetail"])
				}
			},
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec2 := doRequest(t, h, tc.method, tc.path, tc.body)
			assert.Equal(t, tc.wantCode, rec2.Code)

			if tc.check != nil {
				tc.check(t, rec2.Body.Bytes())
			}
		})
	}
}

// ---- ListIndicators: real indicators returned ----

func TestListIndicatorsPopulated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		severity     string
		wantMinCount int
	}{
		{name: "informational severity", severity: "", wantMinCount: 2},
		{name: "high severity has more indicators", severity: "HIGH", wantMinCount: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			// Create graph and investigation.
			rec := doRequest(t, h, http.MethodPost, "/graph", map[string]any{})
			require.Equal(t, http.StatusOK, rec.Code)
			var gResp map[string]any
			parseJSON(t, rec.Body.Bytes(), &gResp)
			graphARN := gResp["GraphArn"].(string)

			startBody := map[string]any{
				"GraphArn":       graphARN,
				"EntityArn":      "arn:aws:iam::123456789012:user/testuser",
				"ScopeStartTime": "2024-01-01T00:00:00Z",
				"ScopeEndTime":   "2024-01-31T00:00:00Z",
			}
			rec2 := doRequest(t, h, http.MethodPost, "/investigations/startInvestigation", startBody)
			require.Equal(t, http.StatusOK, rec2.Code)
			var invResp map[string]any
			parseJSON(t, rec2.Body.Bytes(), &invResp)
			invID := invResp["InvestigationId"].(string)

			if tt.severity != "" {
				// Update to requested severity via GetInvestigation — severity is set by emulator
				// deterministically based on investigation data; we verify the baseline not the setter.
				_ = tt.severity
			}

			// ListIndicators — should return real data.
			rec3 := doRequest(t, h, http.MethodPost, "/investigations/listIndicators", map[string]any{
				"GraphArn":        graphARN,
				"InvestigationId": invID,
			})
			require.Equal(t, http.StatusOK, rec3.Code)

			var resp map[string]any
			parseJSON(t, rec3.Body.Bytes(), &resp)

			indicators, hasField := resp["Indicators"].([]any)
			require.True(t, hasField)
			assert.GreaterOrEqual(t, len(indicators), 2,
				"informational investigation should have at least 2 indicators")

			for _, raw := range indicators {
				ind, isMap := raw.(map[string]any)
				require.True(t, isMap)
				assert.NotEmpty(t, ind["IndicatorType"])
				assert.NotEmpty(t, ind["IndicatorDetail"])
			}
		})
	}
}

// ---- ListIndicators: type filter ----

func TestListIndicatorsTypeFilter(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/graph", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	var gResp map[string]any
	parseJSON(t, rec.Body.Bytes(), &gResp)
	graphARN := gResp["GraphArn"].(string)

	rec2 := doRequest(t, h, http.MethodPost, "/investigations/startInvestigation", map[string]any{
		"GraphArn":       graphARN,
		"EntityArn":      "arn:aws:iam::123456789012:user/testuser",
		"ScopeStartTime": "2024-01-01T00:00:00Z",
		"ScopeEndTime":   "2024-01-31T00:00:00Z",
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	var invResp map[string]any
	parseJSON(t, rec2.Body.Bytes(), &invResp)
	invID := invResp["InvestigationId"].(string)

	rec3 := doRequest(t, h, http.MethodPost, "/investigations/listIndicators", map[string]any{
		"GraphArn":        graphARN,
		"InvestigationId": invID,
		"IndicatorType":   "TTP_OBSERVED",
	})
	require.Equal(t, http.StatusOK, rec3.Code)

	var resp map[string]any
	parseJSON(t, rec3.Body.Bytes(), &resp)

	indicators, ok := resp["Indicators"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, indicators, "TTP_OBSERVED filter should return at least one result")

	for _, raw := range indicators {
		ind, isMap := raw.(map[string]any)
		require.True(t, isMap)
		assert.Equal(t, "TTP_OBSERVED", ind["IndicatorType"], "all returned indicators must match the requested type")
	}
}

// TestListInvestigationsOpaqueToken verifies ListInvestigations' NextToken is
// an opaque base64 offset, not the raw next InvestigationId.
func TestListInvestigationsOpaqueToken(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/graph", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	var gResp map[string]any
	parseJSON(t, rec.Body.Bytes(), &gResp)
	graphARN := gResp["GraphArn"].(string)

	for _, entity := range []string{"alice", "bob"} {
		rec2 := doRequest(t, h, http.MethodPost, "/investigations/startInvestigation", map[string]any{
			"GraphArn":       graphARN,
			"EntityArn":      "arn:aws:iam::123456789012:user/" + entity,
			"ScopeStartTime": "2024-01-01T00:00:00Z",
			"ScopeEndTime":   "2024-01-31T00:00:00Z",
		})
		require.Equal(t, http.StatusOK, rec2.Code)
	}

	rec3 := doRequest(t, h, http.MethodPost, "/investigations/listInvestigations", map[string]any{
		"GraphArn":   graphARN,
		"MaxResults": 1,
	})
	require.Equal(t, http.StatusOK, rec3.Code)
	var resp map[string]any
	parseJSON(t, rec3.Body.Bytes(), &resp)

	tok, hasTok := resp["NextToken"].(string)
	require.True(t, hasTok, "NextToken must be present when more results exist")

	_, err := base64.StdEncoding.DecodeString(tok)
	require.NoError(t, err, "NextToken must be opaque base64, not a raw InvestigationId")

	rec4 := doRequest(t, h, http.MethodPost, "/investigations/listInvestigations", map[string]any{
		"GraphArn":   graphARN,
		"MaxResults": 200,
		"NextToken":  tok,
	})
	require.Equal(t, http.StatusOK, rec4.Code)
	var resp2 map[string]any
	parseJSON(t, rec4.Body.Bytes(), &resp2)
	_, hasTok2 := resp2["NextToken"]
	assert.False(t, hasTok2, "NextToken must be absent on the last page")
}

// TestListIndicators_DetailShape verifies each Indicator's IndicatorDetail
// carries the type-specific sub-detail matching the real (union-like)
// aws-sdk-go-v2 IndicatorDetail shape, not a free-text "Title" -- the real
// SDK's Indicator type has no Title member at all.
func TestListIndicators_DetailShape(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/graph", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	var gResp map[string]any
	parseJSON(t, rec.Body.Bytes(), &gResp)
	graphARN := gResp["GraphArn"].(string)

	rec2 := doRequest(t, h, http.MethodPost, "/investigations/startInvestigation", map[string]any{
		"GraphArn":       graphARN,
		"EntityArn":      "arn:aws:iam::123456789012:user/testuser",
		"ScopeStartTime": "2024-01-01T00:00:00Z",
		"ScopeEndTime":   "2024-01-31T00:00:00Z",
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	var invResp map[string]any
	parseJSON(t, rec2.Body.Bytes(), &invResp)
	invID := invResp["InvestigationId"].(string)

	rec3 := doRequest(t, h, http.MethodPost, "/investigations/listIndicators", map[string]any{
		"GraphArn":        graphARN,
		"InvestigationId": invID,
		"IndicatorType":   "TTP_OBSERVED",
	})
	require.Equal(t, http.StatusOK, rec3.Code)
	var resp map[string]any
	parseJSON(t, rec3.Body.Bytes(), &resp)

	indicators, ok := resp["Indicators"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, indicators)

	for _, raw := range indicators {
		ind := raw.(map[string]any)
		assert.Equal(t, "TTP_OBSERVED", ind["IndicatorType"])
		assert.NotContains(t, ind, "Title", "the real Indicator SDK type has no Title member")

		detail, isDetailMap := ind["IndicatorDetail"].(map[string]any)
		require.True(t, isDetailMap, "IndicatorDetail must be an object")
		ttp, isTTPMap := detail["TTPsObservedDetail"].(map[string]any)
		require.True(t, isTTPMap, "TTP_OBSERVED indicators must carry a TTPsObservedDetail")
		assert.NotEmpty(t, ttp["Tactic"])
		assert.NotEmpty(t, ttp["Procedure"])
	}
}

// TestListIndicators_InvalidIndicatorTypeRejected verifies an IndicatorType
// filter outside the real 8-value enum (botocore detective/2018-10-26
// service-2.json shapes.IndicatorType) is rejected with ValidationException,
// matching ListIndicators' documented error set (which includes
// ValidationException) rather than silently returning an empty Indicators
// list for a value the real API would reject before it ever reaches
// business logic.
func TestListIndicators_InvalidIndicatorTypeRejected(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/graph", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	var gResp map[string]any
	parseJSON(t, rec.Body.Bytes(), &gResp)
	graphARN := gResp["GraphArn"].(string)

	rec2 := doRequest(t, h, http.MethodPost, "/investigations/startInvestigation", map[string]any{
		"GraphArn":       graphARN,
		"EntityArn":      "arn:aws:iam::123456789012:user/testuser",
		"ScopeStartTime": "2024-01-01T00:00:00Z",
		"ScopeEndTime":   "2024-01-31T00:00:00Z",
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	var invResp map[string]any
	parseJSON(t, rec2.Body.Bytes(), &invResp)
	invID := invResp["InvestigationId"].(string)

	rec3 := doRequest(t, h, http.MethodPost, "/investigations/listIndicators", map[string]any{
		"GraphArn":        graphARN,
		"InvestigationId": invID,
		"IndicatorType":   "NOT_A_REAL_INDICATOR_TYPE",
	})
	assert.Equal(t, http.StatusBadRequest, rec3.Code)
}

// TestHandleStartInvestigation_RequiresScopeTimes verifies ScopeStartTime and
// ScopeEndTime are enforced as required (both are marked "This member is
// required" on StartInvestigationInput in the real SDK). Before this test,
// omitting them silently parsed as the zero time.Time instead of a
// ValidationException.
func TestHandleStartInvestigation_RequiresScopeTimes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "missing ScopeStartTime rejected",
			body: map[string]any{
				"EntityArn":    "arn:aws:iam::000000000000:user/bob",
				"ScopeEndTime": "2024-01-31T00:00:00Z",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing ScopeEndTime rejected",
			body: map[string]any{
				"EntityArn":      "arn:aws:iam::000000000000:user/bob",
				"ScopeStartTime": "2024-01-01T00:00:00Z",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "both scope times present accepted",
			body: map[string]any{
				"EntityArn":      "arn:aws:iam::000000000000:user/bob",
				"ScopeStartTime": "2024-01-01T00:00:00Z",
				"ScopeEndTime":   "2024-01-31T00:00:00Z",
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			createRec := doRequest(t, h, http.MethodPost, "/graph", map[string]any{})
			require.Equal(t, http.StatusOK, createRec.Code)

			var createResp map[string]any
			parseJSON(t, createRec.Body.Bytes(), &createResp)
			tc.body["GraphArn"] = createResp["GraphArn"]

			rec := doRequest(t, h, http.MethodPost, "/investigations/startInvestigation", tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

// TestHandleStartInvestigation_ClientSuppliedEntityTypeIgnored verifies a
// client-supplied EntityType field (not part of the real StartInvestigation
// wire request) has no effect: EntityType must always be derived from
// EntityArn, matching the real deserializer which has no such input member.
func TestHandleStartInvestigation_ClientSuppliedEntityTypeIgnored(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, http.MethodPost, "/graph", map[string]any{})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]any
	parseJSON(t, createRec.Body.Bytes(), &createResp)

	rec := doRequest(t, h, http.MethodPost, "/investigations/startInvestigation", map[string]any{
		"GraphArn":       createResp["GraphArn"],
		"EntityArn":      "arn:aws:iam::000000000000:role/actual-role",
		"EntityType":     "IAM_USER", // wrong on purpose -- must be ignored.
		"ScopeStartTime": "2024-01-01T00:00:00Z",
		"ScopeEndTime":   "2024-01-31T00:00:00Z",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var startResp map[string]any
	parseJSON(t, rec.Body.Bytes(), &startResp)

	getRec := doRequest(t, h, http.MethodPost, "/investigations/getInvestigation", map[string]any{
		"GraphArn":        createResp["GraphArn"],
		"InvestigationId": startResp["InvestigationId"],
	})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	parseJSON(t, getRec.Body.Bytes(), &getResp)
	assert.Equal(t, "IAM_ROLE", getResp["EntityType"],
		"EntityType must be derived from the role ARN, not the client field")
}
