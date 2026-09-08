package ce_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetCostAndUsage_UnparseableTimePeriodRejected verifies that a
// TimePeriod.Start/.End that doesn't match the wire model's YearMonthDay
// pattern (botocore ce/2017-10-25's YearMonthDay shape) is rejected with
// ValidationError. Before the fix, buildTimeBuckets (cost_usage.go:136-141)
// silently returned zero buckets when time.Parse("2006-01-02", ...) failed,
// so a malformed date produced a 200 with an empty ResultsByTime instead of
// a rejection.
func TestGetCostAndUsage_UnparseableTimePeriodRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		start string
		end   string
	}{
		{name: "start_not_a_date", start: "not-a-date", end: "2024-01-08"},
		{name: "end_not_a_date", start: "2024-01-01", end: "not-a-date"},
		{name: "start_wrong_format_slashes", start: "01/01/2024", end: "2024-01-08"},
		{name: "start_month_out_of_range", start: "2024-13-01", end: "2024-01-08"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "GetCostAndUsage", map[string]any{
				"TimePeriod":  map[string]string{"Start": tt.start, "End": tt.end},
				"Granularity": "DAILY",
				"Metrics":     []string{"BlendedCost"},
			})
			require.Equal(t, http.StatusBadRequest, rec.Code)

			var wireErr wireError
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&wireErr))
			assert.Equal(t, "ValidationError", wireErr.Type)
		})
	}
}
