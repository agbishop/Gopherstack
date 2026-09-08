package mwaa_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_PublishMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		name       string
		envName    string
		seed       bool
		wantStatus int
	}{
		{
			name:    "publish_metrics",
			envName: "publish-metrics-env",
			seed:    true,
			body: map[string]any{
				"MetricData": []map[string]any{
					{"MetricName": "TaskInstance", "Value": 1.0, "Unit": "Count"},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "publish_empty_metrics",
			envName: "publish-empty-metrics-env",
			seed:    true,
			body: map[string]any{
				"MetricData": []map[string]any{},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "env_not_found",
			envName: "nonexistent-metrics-env",
			seed:    false,
			body: map[string]any{
				"MetricData": []map[string]any{},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_json",
			envName:    "metrics-invalid-json-env",
			seed:       true,
			body:       nil,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerForTest(t)

			if tt.seed {
				seedRec := doMWAARequest(t, h, http.MethodPut, "/environments/"+tt.envName, map[string]any{
					"DagS3Path": "dags/", "ExecutionRoleArn": "arn:r", "SourceBucketArn": "arn:b",
					"NetworkConfiguration": networkConfigBody(),
				})
				require.Equal(t, http.StatusOK, seedRec.Code)
			}

			if tt.name == "invalid_json" {
				e := echo.New()
				req := httptest.NewRequest(
					http.MethodPost,
					"/metrics/environments/"+tt.envName,
					strings.NewReader("{invalid"),
				)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				require.NoError(t, h.Handler()(c))
				assert.Equal(t, tt.wantStatus, rec.Code)

				return
			}

			rec := doMWAARequest(t, h, http.MethodPost, "/metrics/environments/"+tt.envName, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_PublishMetrics_NotFound_ErrorType verifies the wire __type for an
// unknown environment is ValidationException, not ResourceNotFoundException:
// PublishMetrics's own awsRestjson1_deserializeOpErrorPublishMetrics switch
// (aws-sdk-go-v2/service/mwaa@v1.43.4/deserializers.go) recognizes only
// InternalServerException and ValidationException, unlike every other
// not-found-capable MWAA op.
func TestHandler_PublishMetrics_NotFound_ErrorType(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)

	rec := doMWAARequest(t, h, http.MethodPost, "/metrics/environments/nonexistent-metrics-env", map[string]any{
		"MetricData": []map[string]any{},
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var body struct {
		Message string `json:"message"`
		Type    string `json:"__type"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ValidationException", body.Type)
}

// TestHandler_PublishMetrics_BackendState verifies PublishMetrics actually
// mutates backend state (rather than asserting via an HTTP GetMetrics route --
// real MWAA has no GetMetrics operation; PublishMetrics is documented as
// "internal use only" with no corresponding read API, so gopherstack's
// StorageBackend.GetMetrics accessor is test-only introspection, not a wire op).
func TestHandler_PublishMetrics_BackendState(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)

	seedRec := doMWAARequest(t, h, http.MethodPut, "/environments/metrics-env", map[string]any{
		"DagS3Path": "dags/", "ExecutionRoleArn": "arn:r", "SourceBucketArn": "arn:b",
		"NetworkConfiguration": networkConfigBody(),
	})
	require.Equal(t, http.StatusOK, seedRec.Code)

	pubRec := doMWAARequest(t, h, http.MethodPost, "/metrics/environments/metrics-env", map[string]any{
		"MetricData": []map[string]any{
			{"MetricName": "TaskInstance", "Value": 5.0, "Unit": "Count"},
		},
	})
	require.Equal(t, http.StatusOK, pubRec.Code)

	data, err := h.Backend.GetMetrics(context.Background(), "metrics-env")
	require.NoError(t, err)
	assert.Len(t, data, 1)
}

// TestMetricsPath_GET_MethodNotAllowed verifies that GET on
// /metrics/environments/{Name} -- a path with no real MWAA read operation --
// is rejected as an unsupported verb rather than routed to a fabricated op.
func TestMetricsPath_GET_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)
	rec := doMWAARequest(t, h, http.MethodGet, "/metrics/environments/does-not-exist", nil)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// ─────────────────────────────────────────────────────────────
// 8. ListTagsForResource HTTP scenarios
// ─────────────────────────────────────────────────────────────

func TestHTTP_MetricsPublish_BackendState(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t)
	doMWAARequest(t, h, http.MethodPut, "/environments/http-metrics-full", map[string]any{
		"DagS3Path": "dags/", "ExecutionRoleArn": "arn:r", "SourceBucketArn": "arn:b",
		"NetworkConfiguration": networkConfigBody(),
	})

	pubRec := doMWAARequest(t, h, http.MethodPost, "/metrics/environments/http-metrics-full", map[string]any{
		"MetricData": []map[string]any{
			{"MetricName": "DagRunDuration", "Value": 123.4, "Unit": "Seconds"},
			{"MetricName": "ActiveDAGs", "Value": 5.0, "Unit": "Count"},
		},
	})
	require.Equal(t, http.StatusOK, pubRec.Code)

	data, err := h.Backend.GetMetrics(context.Background(), "http-metrics-full")
	require.NoError(t, err)
	assert.Len(t, data, 2)
}
