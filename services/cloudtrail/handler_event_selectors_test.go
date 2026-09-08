package cloudtrail_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudtrail"
)

// TestCloudTrailEventSelectors exercises PutEventSelectors and GetEventSelectors.
func TestCloudTrailEventSelectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "put_and_get_event_selectors",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				rec := doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "my-trail",
					"EventSelectors": []map[string]any{
						{
							"ReadWriteType":           "All",
							"IncludeManagementEvents": true,
							"DataResources":           []any{},
						},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.NotEmpty(t, resp["TrailARN"])
				selectors, ok := resp["EventSelectors"].([]any)
				require.True(t, ok)
				assert.Len(t, selectors, 1)
				// Now get event selectors
				getRec := doCloudTrailOp(t, h, "GetEventSelectors", map[string]any{
					"TrailName": "my-trail",
				})
				assert.Equal(t, http.StatusOK, getRec.Code)
				getResp := parseCloudTrailResp(t, getRec)
				assert.NotEmpty(t, getResp["TrailARN"])
				getSelectors, ok := getResp["EventSelectors"].([]any)
				require.True(t, ok)
				assert.Len(t, getSelectors, 1)
			},
		},
		{
			name: "get_event_selectors_empty",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				rec := doCloudTrailOp(t, h, "GetEventSelectors", map[string]any{
					"TrailName": "my-trail",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				selectors, ok := resp["EventSelectors"].([]any)
				require.True(t, ok)
				assert.Empty(t, selectors)
			},
		},
		{
			name: "put_event_selectors_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName":      "missing-trail",
					"EventSelectors": []any{},
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestCloudTrailHandler()
			tt.ops(t, h)
		})
	}
}

// TestCloudTrailEventConfiguration exercises GetEventConfiguration and
// PutEventConfiguration for both trails and event data stores, verifying the
// AWS wire shape (TrailARN/EventDataStoreArn, AggregationConfigurations,
// ContextKeySelectors, MaxEventSize) and that settings persist across calls.
func TestCloudTrailEventConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "trail_round_trip",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "evtcfg-trail",
					"S3BucketName": "bucket",
				})

				putRec := doCloudTrailOp(t, h, "PutEventConfiguration", map[string]any{
					"TrailName":    "evtcfg-trail",
					"MaxEventSize": "Large",
					"ContextKeySelectors": []map[string]any{
						{"Type": "RequestContext", "Equals": []string{"authparams"}},
					},
				})
				require.Equal(t, http.StatusOK, putRec.Code)
				putResp := parseCloudTrailResp(t, putRec)
				assert.NotEmpty(t, putResp["TrailARN"])
				assert.Equal(t, "Large", putResp["MaxEventSize"])
				_, hasEDSArn := putResp["EventDataStoreArn"]
				assert.False(t, hasEDSArn, "trail-scoped response must not include EventDataStoreArn")

				getRec := doCloudTrailOp(t, h, "GetEventConfiguration", map[string]any{
					"TrailName": "evtcfg-trail",
				})
				require.Equal(t, http.StatusOK, getRec.Code)
				getResp := parseCloudTrailResp(t, getRec)
				assert.Equal(t, "Large", getResp["MaxEventSize"])
				selectors, ok := getResp["ContextKeySelectors"].([]any)
				require.True(t, ok)
				assert.Len(t, selectors, 1)
			},
		},
		{
			name: "event_data_store_round_trip",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				edsRec := doCloudTrailOp(t, h, "CreateEventDataStore", map[string]any{"Name": "evtcfg-eds"})
				require.Equal(t, http.StatusOK, edsRec.Code)
				edsARN, _ := parseCloudTrailResp(t, edsRec)["EventDataStoreArn"].(string)

				putRec := doCloudTrailOp(t, h, "PutEventConfiguration", map[string]any{
					"EventDataStore": edsARN,
					"MaxEventSize":   "Standard",
				})
				require.Equal(t, http.StatusOK, putRec.Code)
				putResp := parseCloudTrailResp(t, putRec)
				assert.Equal(t, edsARN, putResp["EventDataStoreArn"])
				_, hasTrailARN := putResp["TrailARN"]
				assert.False(t, hasTrailARN, "EDS-scoped response must not include TrailARN")

				getRec := doCloudTrailOp(t, h, "GetEventConfiguration", map[string]any{
					"EventDataStore": edsARN,
				})
				require.Equal(t, http.StatusOK, getRec.Code)
				getResp := parseCloudTrailResp(t, getRec)
				assert.Equal(t, "Standard", getResp["MaxEventSize"])
			},
		},
		{
			name: "get_defaults_when_unset",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "evtcfg-unset-trail",
					"S3BucketName": "bucket",
				})

				rec := doCloudTrailOp(t, h, "GetEventConfiguration", map[string]any{
					"TrailName": "evtcfg-unset-trail",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.NotEmpty(t, resp["TrailARN"])
				_, hasMaxSize := resp["MaxEventSize"]
				assert.False(t, hasMaxSize, "MaxEventSize omitted when never configured")
			},
		},
		{
			name: "missing_both_resource_params",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "GetEventConfiguration", map[string]any{})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "trail_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "GetEventConfiguration", map[string]any{
					"TrailName": "does-not-exist",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			// A trail's ARN is deterministic from its user-chosen name
			// (arn.Build includes "trail/"+name), so deleting and
			// recreating a trail with the same name reuses the exact same
			// ARN. eventConfigs is keyed by that ARN and DeleteTrail never
			// purges it, so the recreated trail must not inherit the
			// deleted trail's configuration.
			name: "recreated_trail_does_not_inherit_deleted_trails_config",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "evtcfg-reused-name",
					"S3BucketName": "bucket",
				})
				putRec := doCloudTrailOp(t, h, "PutEventConfiguration", map[string]any{
					"TrailName":    "evtcfg-reused-name",
					"MaxEventSize": "Large",
				})
				require.Equal(t, http.StatusOK, putRec.Code)

				delRec := doCloudTrailOp(t, h, "DeleteTrail", map[string]any{
					"Name": "evtcfg-reused-name",
				})
				require.Equal(t, http.StatusOK, delRec.Code)

				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "evtcfg-reused-name",
					"S3BucketName": "bucket",
				})
				getRec := doCloudTrailOp(t, h, "GetEventConfiguration", map[string]any{
					"TrailName": "evtcfg-reused-name",
				})
				require.Equal(t, http.StatusOK, getRec.Code)
				getResp := parseCloudTrailResp(t, getRec)
				_, hasMaxSize := getResp["MaxEventSize"]
				assert.False(t, hasMaxSize, "recreated trail must not inherit the deleted trail's event configuration")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestCloudTrailHandler()
			tt.ops(t, h)
		})
	}
}

// TestCloudTrailInsightSelectorsEventDataStore verifies PutInsightSelectors
// and GetInsightSelectors work against an EventDataStore (not just
// TrailName), matching AWS's PutInsightSelectorsInput/GetInsightSelectorsInput
// which accept either parameter.
func TestCloudTrailInsightSelectorsEventDataStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "put_and_get_round_trip",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				edsRec := doCloudTrailOp(t, h, "CreateEventDataStore", map[string]any{"Name": "insights-eds"})
				require.Equal(t, http.StatusOK, edsRec.Code)
				edsARN, _ := parseCloudTrailResp(t, edsRec)["EventDataStoreArn"].(string)

				putRec := doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
					"EventDataStore": edsARN,
					"InsightSelectors": []map[string]any{
						{"InsightType": "ApiCallRateInsight"},
					},
				})
				require.Equal(t, http.StatusOK, putRec.Code)
				putResp := parseCloudTrailResp(t, putRec)
				assert.Equal(t, edsARN, putResp["EventDataStoreArn"])

				getRec := doCloudTrailOp(t, h, "GetInsightSelectors", map[string]any{
					"EventDataStore": edsARN,
				})
				require.Equal(t, http.StatusOK, getRec.Code)
				getResp := parseCloudTrailResp(t, getRec)
				assert.Equal(t, edsARN, getResp["EventDataStoreArn"])
				sels, ok := getResp["InsightSelectors"].([]any)
				require.True(t, ok)
				assert.Len(t, sels, 1)
			},
		},
		{
			name: "get_not_enabled",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				edsRec := doCloudTrailOp(t, h, "CreateEventDataStore", map[string]any{"Name": "insights-eds-none"})
				require.Equal(t, http.StatusOK, edsRec.Code)
				edsARN, _ := parseCloudTrailResp(t, edsRec)["EventDataStoreArn"].(string)

				rec := doCloudTrailOp(t, h, "GetInsightSelectors", map[string]any{
					"EventDataStore": edsARN,
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "put_missing_both_params",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
					"InsightSelectors": []map[string]any{{"InsightType": "ApiCallRateInsight"}},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "get_missing_both_params",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "GetInsightSelectors", map[string]any{})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestCloudTrailHandler()
			tt.ops(t, h)
		})
	}
}

// TestAdvancedEventSelectors verifies PutEventSelectors and GetEventSelectors with
// AdvancedEventSelectors support (mutually exclusive with basic EventSelectors).
func TestAdvancedEventSelectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "put_advanced_event_selectors_success",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "adv-trail",
					"S3BucketName": "bucket",
				})
				rec := doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "adv-trail",
					"AdvancedEventSelectors": []map[string]any{
						{
							"Name": "Log all S3 data events",
							"FieldSelectors": []map[string]any{
								{"Field": "eventCategory", "Equals": []string{"Data"}},
								{"Field": "resources.type", "Equals": []string{"AWS::S3::Object"}},
							},
						},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.NotEmpty(t, resp["TrailARN"])
				advSels, ok := resp["AdvancedEventSelectors"].([]any)
				require.True(t, ok, "response should contain AdvancedEventSelectors")
				assert.Len(t, advSels, 1)
				// Basic selectors should NOT be present when advanced are active.
				_, hasBasic := resp["EventSelectors"]
				assert.False(t, hasBasic, "basic EventSelectors should not be in response with advanced selectors")
			},
		},
		{
			name: "get_advanced_event_selectors",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "get-adv-trail",
					"S3BucketName": "bucket",
				})
				doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "get-adv-trail",
					"AdvancedEventSelectors": []map[string]any{
						{
							"Name": "Management events",
							"FieldSelectors": []map[string]any{
								{"Field": "eventCategory", "Equals": []string{"Management"}},
							},
						},
					},
				})
				rec := doCloudTrailOp(t, h, "GetEventSelectors", map[string]any{
					"TrailName": "get-adv-trail",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.NotEmpty(t, resp["TrailARN"])
				advSels, ok := resp["AdvancedEventSelectors"].([]any)
				require.True(t, ok)
				assert.Len(t, advSels, 1)
				sel := advSels[0].(map[string]any)
				assert.Equal(t, "Management events", sel["Name"])
			},
		},
		{
			name: "advanced_selectors_replace_basic_selectors",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "mutual-trail",
					"S3BucketName": "bucket",
				})
				// First put basic selectors.
				doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "mutual-trail",
					"EventSelectors": []map[string]any{
						{"ReadWriteType": "All", "IncludeManagementEvents": true},
					},
				})
				// Then put advanced selectors — should replace basic.
				rec := doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "mutual-trail",
					"AdvancedEventSelectors": []map[string]any{
						{
							"Name": "All data events",
							"FieldSelectors": []map[string]any{
								{"Field": "eventCategory", "Equals": []string{"Data"}},
							},
						},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				// Verify GetEventSelectors returns only advanced now.
				getRec := doCloudTrailOp(t, h, "GetEventSelectors", map[string]any{
					"TrailName": "mutual-trail",
				})
				getResp := parseCloudTrailResp(t, getRec)
				advSels, ok := getResp["AdvancedEventSelectors"].([]any)
				require.True(t, ok)
				assert.Len(t, advSels, 1)
				// Basic selectors should be empty now.
				basicSels, hasSels := getResp["EventSelectors"].([]any)
				if hasSels {
					assert.Empty(t, basicSels, "basic selectors should be empty after applying advanced")
				}
			},
		},
		{
			name: "basic_selectors_replace_advanced_selectors",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "reverse-mutual-trail",
					"S3BucketName": "bucket",
				})
				// Put advanced selectors first.
				doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "reverse-mutual-trail",
					"AdvancedEventSelectors": []map[string]any{
						{
							"Name": "Network activity",
							"FieldSelectors": []map[string]any{
								{"Field": "eventCategory", "Equals": []string{"NetworkActivity"}},
							},
						},
					},
				})
				// Now put basic selectors — should replace advanced.
				rec := doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "reverse-mutual-trail",
					"EventSelectors": []map[string]any{
						{"ReadWriteType": "WriteOnly", "IncludeManagementEvents": true},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				basicSels, ok := resp["EventSelectors"].([]any)
				require.True(t, ok)
				assert.Len(t, basicSels, 1)
			},
		},
		{
			name: "advanced_selectors_with_multiple_field_conditions",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "multi-cond-trail",
					"S3BucketName": "bucket",
				})
				rec := doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "multi-cond-trail",
					"AdvancedEventSelectors": []map[string]any{
						{
							"Name": "Specific S3 bucket",
							"FieldSelectors": []map[string]any{
								{"Field": "eventCategory", "Equals": []string{"Data"}},
								{"Field": "resources.type", "Equals": []string{"AWS::S3::Object"}},
								{"Field": "resources.ARN", "StartsWith": []string{"arn:aws:s3:::my-bucket/"}},
							},
						},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				advSels := resp["AdvancedEventSelectors"].([]any)
				require.Len(t, advSels, 1)
				sel := advSels[0].(map[string]any)
				fieldSels := sel["FieldSelectors"].([]any)
				assert.Len(t, fieldSels, 3)
			},
		},
		{
			name: "put_advanced_selectors_trail_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "nonexistent-trail",
					"AdvancedEventSelectors": []map[string]any{
						{
							"Name": "sel",
							"FieldSelectors": []map[string]any{
								{"Field": "eventCategory", "Equals": []string{"Data"}},
							},
						},
					},
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestCloudTrailHandler()
			tt.ops(t, h)
		})
	}
}

// TestInsightSelectors verifies PutInsightSelectors and GetInsightSelectors with
// correct HasInsightSelectors tracking.
func TestInsightSelectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "put_and_get_insight_selectors",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "insight-trail",
					"S3BucketName": "bucket",
				})
				rec := doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
					"TrailName": "insight-trail",
					"InsightSelectors": []map[string]any{
						{"InsightType": "ApiCallRateInsight"},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.NotEmpty(t, resp["TrailARN"])
				selectors, ok := resp["InsightSelectors"].([]any)
				require.True(t, ok)
				assert.Len(t, selectors, 1)
				sel := selectors[0].(map[string]any)
				assert.Equal(t, "ApiCallRateInsight", sel["InsightType"])
			},
		},
		{
			name: "put_insight_selectors_sets_has_insight_selectors",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "has-insight-trail",
					"S3BucketName": "bucket",
				})
				// Initially HasInsightSelectors should be false.
				descRec := doCloudTrailOp(t, h, "DescribeTrails", nil)
				descResp := parseCloudTrailResp(t, descRec)
				list := descResp["trailList"].([]any)
				require.Len(t, list, 1)
				trail := list[0].(map[string]any)
				assert.Equal(t, false, trail["HasInsightSelectors"])

				// Put insight selectors.
				doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
					"TrailName": "has-insight-trail",
					"InsightSelectors": []map[string]any{
						{"InsightType": "ApiErrorRateInsight"},
					},
				})
				// Now HasInsightSelectors should be true.
				descRec2 := doCloudTrailOp(t, h, "DescribeTrails", nil)
				descResp2 := parseCloudTrailResp(t, descRec2)
				list2 := descResp2["trailList"].([]any)
				require.Len(t, list2, 1)
				trail2 := list2[0].(map[string]any)
				assert.Equal(t, true, trail2["HasInsightSelectors"])
			},
		},
		{
			name: "clear_insight_selectors_causes_insight_not_enabled_error",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "clear-insight-trail",
					"S3BucketName": "bucket",
				})
				doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
					"TrailName": "clear-insight-trail",
					"InsightSelectors": []map[string]any{
						{"InsightType": "ApiCallRateInsight"},
					},
				})
				// Clear by passing empty list.
				doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
					"TrailName":        "clear-insight-trail",
					"InsightSelectors": []any{},
				})
				// AWS returns InsightNotEnabledException when no selectors are configured.
				rec := doCloudTrailOp(t, h, "GetInsightSelectors", map[string]any{
					"TrailName": "clear-insight-trail",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, "InsightNotEnabledException", resp["__type"])
			},
		},
		{
			name: "get_insight_selectors_returns_insight_not_enabled_on_new_trail",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "empty-insight-trail",
					"S3BucketName": "bucket",
				})
				// AWS returns InsightNotEnabledException when trail has no insight selectors.
				rec := doCloudTrailOp(t, h, "GetInsightSelectors", map[string]any{
					"TrailName": "empty-insight-trail",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, "InsightNotEnabledException", resp["__type"])
			},
		},
		{
			name: "put_insight_selectors_trail_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
					"TrailName": "missing-trail",
					"InsightSelectors": []map[string]any{
						{"InsightType": "ApiCallRateInsight"},
					},
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "get_insight_selectors_trail_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "GetInsightSelectors", map[string]any{
					"TrailName": "missing-trail",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "put_insight_selectors_missing_trail_name",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
					"InsightSelectors": []map[string]any{
						{"InsightType": "ApiCallRateInsight"},
					},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "both_insight_types",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "both-insights-trail",
					"S3BucketName": "bucket",
				})
				rec := doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
					"TrailName": "both-insights-trail",
					"InsightSelectors": []map[string]any{
						{"InsightType": "ApiCallRateInsight"},
						{"InsightType": "ApiErrorRateInsight"},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				sels := resp["InsightSelectors"].([]any)
				assert.Len(t, sels, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestCloudTrailHandler()
			tt.ops(t, h)
		})
	}
}

// TestHasCustomEventSelectorsTracking verifies HasCustomEventSelectors is updated
// correctly as event selectors are added and removed.
func TestHasCustomEventSelectorsTracking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "no_selectors_means_no_custom",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "no-custom-trail",
					"S3BucketName": "bucket",
				})
				rec := doCloudTrailOp(t, h, "GetTrail", map[string]any{
					"Name": "no-custom-trail",
				})
				resp := parseCloudTrailResp(t, rec)
				trail := resp["Trail"].(map[string]any)
				assert.Equal(t, false, trail["HasCustomEventSelectors"])
			},
		},
		{
			name: "basic_selectors_set_has_custom",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "basic-custom-trail",
					"S3BucketName": "bucket",
				})
				doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "basic-custom-trail",
					"EventSelectors": []map[string]any{
						{"ReadWriteType": "All", "IncludeManagementEvents": true},
					},
				})
				rec := doCloudTrailOp(t, h, "GetTrail", map[string]any{
					"Name": "basic-custom-trail",
				})
				resp := parseCloudTrailResp(t, rec)
				trail := resp["Trail"].(map[string]any)
				assert.Equal(t, true, trail["HasCustomEventSelectors"])
			},
		},
		{
			name: "advanced_selectors_set_has_custom",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "adv-custom-trail",
					"S3BucketName": "bucket",
				})
				doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "adv-custom-trail",
					"AdvancedEventSelectors": []map[string]any{
						{
							"Name": "sel",
							"FieldSelectors": []map[string]any{
								{"Field": "eventCategory", "Equals": []string{"Data"}},
							},
						},
					},
				})
				rec := doCloudTrailOp(t, h, "GetTrail", map[string]any{
					"Name": "adv-custom-trail",
				})
				resp := parseCloudTrailResp(t, rec)
				trail := resp["Trail"].(map[string]any)
				assert.Equal(t, true, trail["HasCustomEventSelectors"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestCloudTrailHandler()
			tt.ops(t, h)
		})
	}
}

// TestCloudTrailTrailInsightSelectorsSmoke covers PutInsightSelectors and GetInsightSelectors.
func TestCloudTrailTrailInsightSelectorsSmoke(t *testing.T) {
	t.Parallel()

	h := newTestCloudTrailHandler()

	// Create a trail.
	rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
		"Name":         "insights-trail",
		"S3BucketName": "bucket",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// PutInsightSelectors.
	rec = doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
		"TrailName": "insights-trail",
		"InsightSelectors": []map[string]any{
			{"InsightType": "ApiCallRateInsight"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseCloudTrailResp(t, rec)
	assert.NotEmpty(t, resp["TrailARN"])
	sels, ok := resp["InsightSelectors"].([]any)
	require.True(t, ok)
	assert.Len(t, sels, 1)

	// GetInsightSelectors.
	rec = doCloudTrailOp(t, h, "GetInsightSelectors", map[string]any{
		"TrailName": "insights-trail",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	getResp := parseCloudTrailResp(t, rec)
	assert.NotEmpty(t, getResp["TrailARN"])
	getSels, ok := getResp["InsightSelectors"].([]any)
	require.True(t, ok)
	assert.Len(t, getSels, 1)
	sel := getSels[0].(map[string]any)
	assert.Equal(t, "ApiCallRateInsight", sel["InsightType"])
}

// TestCloudTrailAdvancedEventSelectorsSmoke covers PutEventSelectors and
// GetEventSelectors with AdvancedEventSelectors.
func TestCloudTrailAdvancedEventSelectorsSmoke(t *testing.T) {
	t.Parallel()

	h := newTestCloudTrailHandler()

	// Create a trail.
	rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
		"Name":         "adv-trail-cov",
		"S3BucketName": "bucket",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// PutEventSelectors with AdvancedEventSelectors.
	rec = doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
		"TrailName": "adv-trail-cov",
		"AdvancedEventSelectors": []map[string]any{
			{
				"Name": "All management events",
				"FieldSelectors": []map[string]any{
					{"Field": "eventCategory", "Equals": []string{"Management"}},
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseCloudTrailResp(t, rec)
	assert.NotEmpty(t, resp["TrailARN"])
	advSels, ok := resp["AdvancedEventSelectors"].([]any)
	require.True(t, ok)
	assert.Len(t, advSels, 1)

	// GetEventSelectors returns AdvancedEventSelectors.
	rec = doCloudTrailOp(t, h, "GetEventSelectors", map[string]any{
		"TrailName": "adv-trail-cov",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	getResp := parseCloudTrailResp(t, rec)
	getAdvSels, ok := getResp["AdvancedEventSelectors"].([]any)
	require.True(t, ok)
	assert.Len(t, getAdvSels, 1)
}

// TestGetEventSelectorsAdvancedOnly verifies that GetEventSelectors does NOT
// return EventSelectors when AdvancedEventSelectors are active.
func TestGetEventSelectorsAdvancedOnly(t *testing.T) {
	t.Parallel()

	h := newTestCloudTrailHandler()

	doCloudTrailOp(t, h, "CreateTrail", map[string]any{
		"Name":         "ges-adv-trail",
		"S3BucketName": "bucket",
	})
	doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
		"TrailName": "ges-adv-trail",
		"AdvancedEventSelectors": []map[string]any{
			{
				"Name": "All S3 events",
				"FieldSelectors": []map[string]any{
					{"Field": "eventCategory", "Equals": []string{"Data"}},
				},
			},
		},
	})

	rec := doCloudTrailOp(t, h, "GetEventSelectors", map[string]any{
		"TrailName": "ges-adv-trail",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseCloudTrailResp(t, rec)
	advSels, ok := resp["AdvancedEventSelectors"].([]any)
	require.True(t, ok, "AdvancedEventSelectors must be present")
	assert.Len(t, advSels, 1)

	_, hasBasic := resp["EventSelectors"]
	assert.False(t, hasBasic, "EventSelectors must NOT be present when AdvancedEventSelectors are active")
}
