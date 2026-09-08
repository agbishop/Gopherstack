package kinesis_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnhancedMonitoring(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	streamName := "monitor-stream"

	rec := doRequest(t, h, "CreateStream", map[string]any{
		"StreamName": streamName,
		"ShardCount": 1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Enable monitoring.
	rec = doRequest(t, h, "EnableEnhancedMonitoring", map[string]any{
		"StreamName":        streamName,
		"ShardLevelMetrics": []string{"IncomingBytes", "OutgoingRecords"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var enableResp struct {
		StreamName               string   `json:"StreamName"`
		CurrentShardLevelMetrics []string `json:"CurrentShardLevelMetrics"`
		DesiredShardLevelMetrics []string `json:"DesiredShardLevelMetrics"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &enableResp))
	assert.Equal(t, streamName, enableResp.StreamName)
	assert.Empty(t, enableResp.CurrentShardLevelMetrics)
	assert.ElementsMatch(t, []string{"IncomingBytes", "OutgoingRecords"}, enableResp.DesiredShardLevelMetrics)

	// Disable one metric.
	rec = doRequest(t, h, "DisableEnhancedMonitoring", map[string]any{
		"StreamName":        streamName,
		"ShardLevelMetrics": []string{"IncomingBytes"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var disableResp struct {
		StreamName               string   `json:"StreamName"`
		CurrentShardLevelMetrics []string `json:"CurrentShardLevelMetrics"`
		DesiredShardLevelMetrics []string `json:"DesiredShardLevelMetrics"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &disableResp))
	assert.ElementsMatch(t, []string{"IncomingBytes", "OutgoingRecords"}, disableResp.CurrentShardLevelMetrics)
	assert.Equal(t, []string{"OutgoingRecords"}, disableResp.DesiredShardLevelMetrics)
}

// TestEnhancedMonitoring_StreamARN verifies EnableEnhancedMonitoringOutput and
// DisableEnhancedMonitoringOutput both carry StreamARN (real, optional member
// of both Output shapes, api_op_Enable/DisableEnhancedMonitoring.go), not just
// StreamName.
func TestEnhancedMonitoring_StreamARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	streamName := "monitor-arn-stream"

	setupRec := doRequest(t, h, "CreateStream", map[string]any{
		"StreamName": streamName,
		"ShardCount": 1,
	})
	require.Equal(t, http.StatusOK, setupRec.Code)

	tests := []struct {
		action string
	}{
		{action: "EnableEnhancedMonitoring"},
		{action: "DisableEnhancedMonitoring"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, tt.action, map[string]any{
				"StreamName":        streamName,
				"ShardLevelMetrics": []string{"IncomingBytes"},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				StreamARN string `json:"StreamARN"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotEmpty(t, resp.StreamARN)
			assert.Contains(t, resp.StreamARN, streamName)
		})
	}
}
