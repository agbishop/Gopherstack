package personalize

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDatasetGroupToMap_FailureReason locks gopherstack-7z3p: the SDK
// deserializer reads DatasetGroup.failureReason
// (aws-sdk-go-v2/service/personalize@v1.50.4/deserializers.go:11024,
// awsAwsjson11_deserializeDocumentDatasetGroup), but datasetGroupToMap (the
// DescribeDatasetGroup response builder) used to omit the field
// unconditionally even when the backend model carried a value -- unlike its
// sibling datasetGroupSummaryToMap (ListDatasetGroups), which already
// included it. There is no live path in this backend that sets
// DatasetGroup.FailureReason to non-empty (gopherstack-h3th: CREATE FAILED
// is unreachable-by-construction in this synchronous emulator), so this
// constructs the struct directly rather than driving it through the HTTP
// API.
func TestDatasetGroupToMap_FailureReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		failureReason string
	}{
		{name: "present", failureReason: "boom"},
		{name: "absent", failureReason: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dg := &DatasetGroup{
				DatasetGroupArn: "arn:aws:personalize:us-east-1:000000000000:dataset-group/g",
				Name:            "g",
				Status:          statusActive,
				FailureReason:   tt.failureReason,
			}
			m := datasetGroupToMap(dg)

			if tt.failureReason == "" {
				assert.NotContains(t, m, "failureReason")

				return
			}
			assert.Equal(t, tt.failureReason, m["failureReason"])
		})
	}
}
