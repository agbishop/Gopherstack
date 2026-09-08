package iot_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iot"
)

// TestBatch2_FleetMetric tests fleet metric lifecycle.
func TestFleetMetric(t *testing.T) {
	t.Parallel()
	h := newIoTHandler(t)

	// Create
	out := iotOK(t, h, http.MethodPut, "/fleet-metric/my-metric", map[string]any{
		"queryString": "SELECT * FROM 'iot/+/data'",
		"period":      300,
	})
	if out["metricName"] != "my-metric" {
		t.Errorf("metricName mismatch: %v", out)
	}

	// Describe
	out2 := iotOK(t, h, http.MethodGet, "/fleet-metric/my-metric", nil)
	if out2["metricName"] != "my-metric" {
		t.Errorf("describe mismatch: %v", out2)
	}

	// List
	out3 := iotOK(t, h, http.MethodGet, "/fleet-metrics", nil)
	metrics, _ := out3["fleetMetrics"].([]any)
	if len(metrics) != 1 {
		t.Errorf("expected 1 metric, got %d", len(metrics))
	}

	// Update
	iotOK(t, h, http.MethodPatch, "/fleet-metric/my-metric", map[string]any{
		"description": "updated",
	})

	// Delete
	iotOK(t, h, http.MethodDelete, "/fleet-metric/my-metric", nil)

	iotExpectError(t, h, "/fleet-metric/my-metric")
}

func TestUpdateFleetMetricExpectedVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expectedVer int
		wantStatus  int
	}{
		{"matching_version_succeeds", 1, http.StatusOK},
		{"stale_version_conflicts", 99, http.StatusConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newIoTHandler(t)
			iotOK(t, h, http.MethodPut, "/fleet-metric/my-metric", map[string]any{
				"queryString": "SELECT * FROM 'iot/+/data'",
				"period":      300,
			})

			rec := iotRequest(t, h, http.MethodPatch, "/fleet-metric/my-metric", map[string]any{
				"description":     "updated",
				"expectedVersion": tt.expectedVer,
			})

			require.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// DeleteFleetMetricInput.ExpectedVersion (iot@v1.77.4/api_op_DeleteFleetMetric.go:39-40),
// expectedVersion is a QUERY parameter
// (awsRestjson1_serializeOpHttpBindingsDeleteFleetMetricInput), and its
// deserializeOpError switch declares a VersionConflictException case.
func TestDeleteFleetMetricExpectedVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expectedVer int
		wantStatus  int
	}{
		{"matching_version_succeeds", 1, http.StatusOK},
		{"stale_version_conflicts", 99, http.StatusConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newIoTHandler(t)
			iotOK(t, h, http.MethodPut, "/fleet-metric/my-metric", map[string]any{
				"queryString": "SELECT * FROM 'iot/+/data'",
				"period":      300,
			})

			path := fmt.Sprintf("/fleet-metric/my-metric?expectedVersion=%d", tt.expectedVer)
			rec := iotRequest(t, h, http.MethodDelete, path, nil)

			require.Equal(t, tt.wantStatus, rec.Code, "body: %s", rec.Body.String())

			if tt.wantStatus == http.StatusOK {
				iotExpectError(t, h, "/fleet-metric/my-metric")
			} else {
				iotOK(t, h, http.MethodGet, "/fleet-metric/my-metric", nil)
			}
		})
	}
}

// TestFleetMetric_AggregationAndUpdateFields proves the fields that were
// silently dropped by the anonymous-inline-struct request decoders (pre
// named-type conversion) now round-trip: CreateFleetMetric's required
// aggregationField/aggregationType, and UpdateFleetMetric's indexName/
// queryVersion/unit/aggregationField/aggregationType (gopherstack-5wj0
// documented indexName/aggregationType/aggregationField/queryVersion/unit
// as dropped on Update; this also catches the same gap on Create, which
// gopherstack-5wj0 did not flag).
func TestFleetMetric_AggregationAndUpdateFields(t *testing.T) {
	t.Parallel()

	t.Run("create_persists_aggregation_fields", func(t *testing.T) {
		t.Parallel()
		h := newIoTHandler(t)

		iotOK(t, h, http.MethodPut, "/fleet-metric/my-metric", map[string]any{
			"queryString":      "SELECT * FROM 'iot/+/data'",
			"period":           300,
			"aggregationField": "connectivity.disconnectReason",
			"aggregationType": map[string]any{
				"name":   "Statistics",
				"values": []any{"average"},
			},
		})

		out := iotOK(t, h, http.MethodGet, "/fleet-metric/my-metric", nil)
		require.Equal(t, "connectivity.disconnectReason", out["aggregationField"])
		aggType, ok := out["aggregationType"].(map[string]any)
		require.True(t, ok, "aggregationType missing from response: %v", out)
		assert.Equal(t, "Statistics", aggType["name"])
	})

	t.Run("update_applies_previously_dropped_fields", func(t *testing.T) {
		t.Parallel()
		h := newIoTHandler(t)

		iotOK(t, h, http.MethodPut, "/fleet-metric/my-metric", map[string]any{
			"queryString": "SELECT * FROM 'iot/+/data'",
			"period":      300,
		})

		iotOK(t, h, http.MethodPatch, "/fleet-metric/my-metric", map[string]any{
			"indexName":        "AWS_Things",
			"queryVersion":     "2017-09-30",
			"unit":             "Count",
			"aggregationField": "connectivity.disconnectReason",
			"aggregationType": map[string]any{
				"name": "Cardinality",
			},
		})

		out := iotOK(t, h, http.MethodGet, "/fleet-metric/my-metric", nil)
		assert.Equal(t, "AWS_Things", out["indexName"])
		assert.Equal(t, "2017-09-30", out["queryVersion"])
		assert.Equal(t, "Count", out["unit"])
		assert.Equal(t, "connectivity.disconnectReason", out["aggregationField"])
		aggType, ok := out["aggregationType"].(map[string]any)
		require.True(t, ok, "aggregationType missing from response: %v", out)
		assert.Equal(t, "Cardinality", aggType["name"])
	})
}

// TestBatch2_CustomMetric tests custom metric lifecycle.
func TestCustomMetric(t *testing.T) {
	t.Parallel()
	h := newIoTHandler(t)

	// Create
	out := iotOK(t, h, http.MethodPost, "/custom-metric/my-custom-metric", map[string]any{
		"metricType":  "number",
		"displayName": "My Metric",
	})
	if out["metricName"] != "my-custom-metric" {
		t.Errorf("metricName mismatch: %v", out)
	}

	// Describe
	out2 := iotOK(t, h, http.MethodGet, "/custom-metric/my-custom-metric", nil)
	if out2["metricName"] != "my-custom-metric" {
		t.Errorf("describe mismatch: %v", out2)
	}

	// List
	out3 := iotOK(t, h, http.MethodGet, "/custom-metric", nil)
	names, _ := out3["metricNames"].([]any)
	if len(names) != 1 {
		t.Errorf("expected 1 custom metric, got %d", len(names))
	}

	// Update
	iotOK(t, h, http.MethodPatch, "/custom-metric/my-custom-metric", map[string]any{
		"displayName": "Updated",
	})

	// Delete
	iotOK(t, h, http.MethodDelete, "/custom-metric/my-custom-metric", nil)

	iotExpectError(t, h, "/custom-metric/my-custom-metric")
}

// TestBatch2_Dimension tests dimension lifecycle.
func TestDimension(t *testing.T) {
	t.Parallel()
	h := newIoTHandler(t)

	// Create
	out := iotOK(t, h, http.MethodPost, "/dimensions/my-dim", map[string]any{
		"type":         "TOPIC_FILTER",
		"stringValues": []any{"iot/sensors/+"},
	})
	if out["name"] != "my-dim" {
		t.Errorf("name mismatch: %v", out)
	}

	// Describe
	out2 := iotOK(t, h, http.MethodGet, "/dimensions/my-dim", nil)
	if out2["name"] != "my-dim" {
		t.Errorf("describe mismatch: %v", out2)
	}

	// List
	out3 := iotOK(t, h, http.MethodGet, "/dimensions", nil)
	names, _ := out3["dimensionNames"].([]any)
	if len(names) != 1 {
		t.Errorf("expected 1 dimension, got %d", len(names))
	}

	// Update
	iotOK(t, h, http.MethodPatch, "/dimensions/my-dim", map[string]any{
		"stringValues": []any{"iot/updated/+"},
	})

	// Delete
	iotOK(t, h, http.MethodDelete, "/dimensions/my-dim", nil)

	iotExpectError(t, h, "/dimensions/my-dim")
}

func TestListMetricValues(t *testing.T) {
	t.Parallel()

	t.Run("round_trip", func(t *testing.T) {
		t.Parallel()

		h, b := newRefHandler()
		b.AddThingInternal(iot.Thing{ThingName: "my-thing"})

		count := int64(5)
		b.AddMetricValueInternal("my-thing", "aws:num-connections", iot.MetricDatapoint{
			Timestamp: 100,
			Value:     &iot.MetricValueData{Count: &count},
		})

		rec := doRefRequest(t, h, http.MethodGet,
			"/metric-values?thingName=my-thing&metricName=aws:num-connections", nil, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "\"count\":5")
	})

	t.Run("unknown_thing_404", func(t *testing.T) {
		t.Parallel()

		h, _ := newRefHandler()

		rec := doRefRequest(t, h, http.MethodGet,
			"/metric-values?thingName=no-such&metricName=aws:num-connections", nil, nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	// guards types.MetricValue's real numbers/strings members (v1.77.4),
	// previously entirely unmodeled on MetricValueData.
	t.Run("numbers_and_strings_survive", func(t *testing.T) {
		t.Parallel()

		h, b := newRefHandler()
		b.AddThingInternal(iot.Thing{ThingName: "num-str-thing"})

		b.AddMetricValueInternal("num-str-thing", "custom:multi", iot.MetricDatapoint{
			Timestamp: 100,
			Value: &iot.MetricValueData{
				Numbers: []float64{1.5, 2.5},
				Strings: []string{"a", "b"},
			},
		})

		rec := doRefRequest(t, h, http.MethodGet,
			"/metric-values?thingName=num-str-thing&metricName=custom:multi", nil, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"numbers":[1.5,2.5]`)
		assert.Contains(t, rec.Body.String(), `"strings":["a","b"]`)
	})
}
