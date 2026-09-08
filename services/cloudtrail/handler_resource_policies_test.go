package cloudtrail_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudtrail"
)

// TestCloudTrailResourcePolicy exercises DeleteResourcePolicy.
func TestCloudTrailResourcePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "delete_resource_policy_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "DeleteResourcePolicy", map[string]any{
					"ResourceArn": "arn:aws:cloudtrail:us-east-1:123456789012:trail/my-trail",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			// A trail's ARN is deterministic from its user-chosen name, so
			// deleting and recreating a trail with the same name reuses the
			// same ARN. resourcePolicies is keyed by that ARN and
			// DeleteTrail must purge it, or the recreated trail silently
			// inherits the deleted trail's resource policy.
			name: "recreated_trail_does_not_inherit_deleted_trails_resource_policy",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				createRec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "respol-reused-name",
					"S3BucketName": "bucket",
				})
				require.Equal(t, http.StatusOK, createRec.Code)
				trailARN, _ := parseCloudTrailResp(t, createRec)["TrailARN"].(string)
				require.NotEmpty(t, trailARN)

				putRec := doCloudTrailOp(t, h, "PutResourcePolicy", map[string]any{
					"ResourceArn":    trailARN,
					"ResourcePolicy": `{"Version":"2012-10-17","Statement":[]}`,
				})
				require.Equal(t, http.StatusOK, putRec.Code)

				delRec := doCloudTrailOp(t, h, "DeleteTrail", map[string]any{"Name": "respol-reused-name"})
				require.Equal(t, http.StatusOK, delRec.Code)

				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "respol-reused-name",
					"S3BucketName": "bucket",
				})
				getRec := doCloudTrailOp(t, h, "GetResourcePolicy", map[string]any{"ResourceArn": trailARN})
				assert.Equal(t, http.StatusNotFound, getRec.Code,
					"recreated trail must not inherit the deleted trail's resource policy")
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
