package cloudwatch

import (
	"fmt"
	"testing"
	"time"
)

// TestDescribeAlarmHistory_PaginationStableAcrossTiedTimestamps proves that
// DescribeAlarmHistory's pagination is reproducible even when many history
// items share an identical Timestamp (whole-value tie, not just whole-second).
// The source (b.alarmHistory) is a map keyed by alarm name, so two calls walk
// alarm names in different random orders; DescribeAlarmHistory sorts only by
// Timestamp, which is not unique, so sort.Slice (unstable) can order tied
// items differently across calls. Paging with a small window then drops or
// duplicates records at the page boundary.
func TestDescribeAlarmHistory_PaginationStableAcrossTiedTimestamps(t *testing.T) {
	t.Parallel()

	const numAlarms = 8

	tied := time.Now().UTC()

	for iter := range 30 {
		b := NewInMemoryBackend()

		wantIDs := make(map[string]bool, numAlarms)

		for i := range numAlarms {
			name := fmt.Sprintf("alarm-%d", i)
			// appendHistory assigns each item a real, unique seq (mirroring
			// production insertion order); only the Timestamp is forced to
			// collide here, matching a genuine clock-resolution tie between
			// history events recorded for different alarms.
			b.appendHistory(name, "MetricAlarm", "StateUpdate", "tied timestamp test", "")
			b.alarmHistory[name][0].Timestamp = tied
			wantIDs[name] = true
		}

		got := make(map[string]int, numAlarms)

		var next string

		for {
			page, err := b.DescribeAlarmHistory("", nil, "", next, "", time.Time{}, time.Time{}, 3)
			if err != nil {
				t.Fatalf("iter %d: DescribeAlarmHistory: %v", iter, err)
			}

			for _, it := range page.Data {
				got[it.AlarmName]++
			}

			if page.Next == "" {
				break
			}

			next = page.Next
		}

		if len(got) != numAlarms {
			t.Fatalf("iter %d: got %d distinct alarm names across pages, want %d: %v", iter, len(got), numAlarms, got)
		}

		for name, count := range got {
			if count != 1 {
				t.Fatalf("iter %d: alarm %q appeared %d times across pages (want exactly 1)", iter, name, count)
			}
		}

		for name := range wantIDs {
			if got[name] != 1 {
				t.Fatalf("iter %d: alarm %q missing from paginated results", iter, name)
			}
		}
	}
}
