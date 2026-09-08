package ce_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wireError decodes the AWS JSON-protocol error envelope (__type/message).
type wireError struct {
	Type    string `json:"__type"`
	Message string `json:"message"`
}

// TestGetCostAndUsage_InvalidGranularityRejected verifies that a Granularity
// value outside the real Smithy enum (botocore ce/2017-10-25's Granularity
// shape: DAILY/MONTHLY/HOURLY) is rejected with ValidationError instead of
// silently passing through to buildTimeBuckets, which falls back to DAILY
// bucketing for anything it doesn't recognize (cost_usage.go's default case).
func TestGetCostAndUsage_InvalidGranularityRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		granularity string
	}{
		{name: "weekly_not_a_real_value", granularity: "WEEKLY"},
		{name: "typo", granularity: "DAILYY"},
		{name: "lowercase_daily_wrong_case", granularity: "daily"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "GetCostAndUsage", map[string]any{
				"TimePeriod":  map[string]string{"Start": "2024-01-01", "End": "2024-01-08"},
				"Granularity": tt.granularity,
				"Metrics":     []string{"BlendedCost"},
			})
			require.Equal(t, http.StatusBadRequest, rec.Code)

			var wireErr wireError
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&wireErr))
			assert.Equal(t, "ValidationError", wireErr.Type)
		})
	}
}

// TestGetCostAndUsage_HourlyGranularityBucketsHourly verifies that
// Granularity=HOURLY, a valid enum value, produces hour-resolution buckets
// rather than being silently treated as DAILY. Before the fix, buildTimeBuckets'
// switch (cost_usage.go:143) had no HOURLY case, so a well-formed HOURLY
// request over a single day returned exactly one DAILY-sized bucket instead
// of 24 hourly ones -- a valid enum value producing wrong data, not merely
// an unrejected bad one.
func TestGetCostAndUsage_HourlyGranularityBucketsHourly(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetCostAndUsage", map[string]any{
		"TimePeriod":  map[string]string{"Start": "2024-01-01", "End": "2024-01-02"},
		"Granularity": "HOURLY",
		"Metrics":     []string{"BlendedCost"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		ResultsByTime []struct {
			TimePeriod map[string]string `json:"TimePeriod"`
		} `json:"ResultsByTime"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&out))

	require.Len(t, out.ResultsByTime, 24, "one bucket per hour across a single day")
	assert.Equal(t, "2024-01-01T00:00:00Z", out.ResultsByTime[0].TimePeriod["Start"])
	assert.Equal(t, "2024-01-01T01:00:00Z", out.ResultsByTime[0].TimePeriod["End"])
	assert.Equal(t, "2024-01-01T23:00:00Z", out.ResultsByTime[23].TimePeriod["Start"])
	assert.Equal(t, "2024-01-02T00:00:00Z", out.ResultsByTime[23].TimePeriod["End"])
}
