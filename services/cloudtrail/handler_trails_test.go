package cloudtrail_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/services/cloudtrail"
)

// TestCloudTrailCRUD exercises CreateTrail, GetTrail, DescribeTrails,
// UpdateTrail, and DeleteTrail through the JSON handler.
func TestCloudTrailCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "create_trail",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, "my-trail", resp["Name"])
				assert.NotEmpty(t, resp["TrailARN"])
			},
		},
		{
			name: "create_trail_already_exists",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				assert.Equal(t, http.StatusConflict, rec.Code)
			},
		},
		{
			name: "create_trail_missing_name",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"S3BucketName": "my-bucket",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create_trail_missing_bucket",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name": "my-trail",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "create_trail_s3_key_prefix_at_max_length",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
					"S3KeyPrefix":  strings.Repeat("a", 200),
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "create_trail_s3_key_prefix_over_max_length",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
					"S3KeyPrefix":  strings.Repeat("a", 201),
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, "InvalidS3PrefixException", resp["__type"])
			},
		},
		{
			name: "get_trail",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				rec := doCloudTrailOp(t, h, "GetTrail", map[string]any{
					"Name": "my-trail",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				trail, ok := resp["Trail"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "my-trail", trail["Name"])
			},
		},
		{
			name: "get_trail_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "GetTrail", map[string]any{
					"Name": "missing-trail",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "describe_trails",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "trail-a",
					"S3BucketName": "bucket-a",
				})
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "trail-b",
					"S3BucketName": "bucket-b",
				})
				rec := doCloudTrailOp(t, h, "DescribeTrails", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				list, ok := resp["trailList"].([]any)
				require.True(t, ok)
				assert.Len(t, list, 2)
			},
		},
		{
			name: "describe_trails_by_name",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "trail-a",
					"S3BucketName": "bucket-a",
				})
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "trail-b",
					"S3BucketName": "bucket-b",
				})
				rec := doCloudTrailOp(t, h, "DescribeTrails", map[string]any{
					"trailNameList": []string{"trail-a"},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				list, ok := resp["trailList"].([]any)
				require.True(t, ok)
				assert.Len(t, list, 1)
				item := list[0].(map[string]any)
				assert.Equal(t, "trail-a", item["Name"])
			},
		},
		{
			name: "update_trail",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "old-bucket",
				})
				rec := doCloudTrailOp(t, h, "UpdateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "new-bucket",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, "new-bucket", resp["S3BucketName"])
			},
		},
		{
			name: "update_trail_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "UpdateTrail", map[string]any{
					"Name": "missing-trail",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "update_trail_s3_key_prefix_at_max_length",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				rec := doCloudTrailOp(t, h, "UpdateTrail", map[string]any{
					"Name":        "my-trail",
					"S3KeyPrefix": strings.Repeat("a", 200),
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, strings.Repeat("a", 200), resp["S3KeyPrefix"])
			},
		},
		{
			name: "update_trail_s3_key_prefix_over_max_length",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				rec := doCloudTrailOp(t, h, "UpdateTrail", map[string]any{
					"Name":        "my-trail",
					"S3KeyPrefix": strings.Repeat("a", 201),
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, "InvalidS3PrefixException", resp["__type"])
			},
		},
		{
			name: "delete_trail",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				rec := doCloudTrailOp(t, h, "DeleteTrail", map[string]any{
					"Name": "my-trail",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				// Verify deleted
				rec2 := doCloudTrailOp(t, h, "GetTrail", map[string]any{
					"Name": "my-trail",
				})
				assert.Equal(t, http.StatusNotFound, rec2.Code)
			},
		},
		{
			name: "delete_trail_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "DeleteTrail", map[string]any{
					"Name": "missing-trail",
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

// TestCloudTrailLogging exercises StartLogging, StopLogging, and GetTrailStatus.
func TestCloudTrailLogging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "start_logging",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				rec := doCloudTrailOp(t, h, "StartLogging", map[string]any{
					"Name": "my-trail",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				// Verify status
				statusRec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
					"Name": "my-trail",
				})
				assert.Equal(t, http.StatusOK, statusRec.Code)
				resp := parseCloudTrailResp(t, statusRec)
				assert.Equal(t, true, resp["IsLogging"])
			},
		},
		{
			name: "stop_logging",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "my-trail",
					"S3BucketName": "my-bucket",
				})
				doCloudTrailOp(t, h, "StartLogging", map[string]any{"Name": "my-trail"})
				rec := doCloudTrailOp(t, h, "StopLogging", map[string]any{
					"Name": "my-trail",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				// Verify status
				statusRec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
					"Name": "my-trail",
				})
				resp := parseCloudTrailResp(t, statusRec)
				assert.Equal(t, false, resp["IsLogging"])
			},
		},
		{
			name: "start_logging_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "StartLogging", map[string]any{
					"Name": "missing-trail",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "stop_logging_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "StopLogging", map[string]any{
					"Name": "missing-trail",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "get_trail_status_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
					"Name": "missing-trail",
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

// TestCloudTrailListTrails exercises the ListTrails operation.
func TestCloudTrailListTrails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "list_trails_empty",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "ListTrails", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				trails, ok := resp["Trails"].([]any)
				require.True(t, ok)
				assert.Empty(t, trails)
			},
		},
		{
			name: "list_trails_with_data",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "trail-x",
					"S3BucketName": "bucket-x",
				})
				rec := doCloudTrailOp(t, h, "ListTrails", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				trails, ok := resp["Trails"].([]any)
				require.True(t, ok)
				assert.Len(t, trails, 1)
				trail := trails[0].(map[string]any)
				assert.Equal(t, "trail-x", trail["Name"])
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

// TestCloudTrailTrailWithAllFields creates a trail with optional fields set.
func TestCloudTrailTrailWithAllFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "create_with_tags_and_options",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":                       "full-trail",
					"S3BucketName":               "bucket",
					"S3KeyPrefix":                "logs/",
					"SnsTopicName":               "my-topic",
					"IncludeGlobalServiceEvents": true,
					"IsMultiRegionTrail":         true,
					"EnableLogFileValidation":    true,
					"TagsList": []map[string]string{
						{"Key": "Env", "Value": "prod"},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, "full-trail", resp["Name"])
				assert.Equal(t, "logs/", resp["S3KeyPrefix"])
				assert.Equal(t, true, resp["IsMultiRegionTrail"])
				assert.Equal(t, true, resp["LogFileValidationEnabled"])
			},
		},
		{
			name: "get_trail_by_arn",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				createRec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "arn-trail",
					"S3BucketName": "bucket",
				})
				createResp := parseCloudTrailResp(t, createRec)
				trailARN := createResp["TrailARN"].(string)
				rec := doCloudTrailOp(t, h, "GetTrail", map[string]any{
					"Name": trailARN,
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				trail, ok := resp["Trail"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "arn-trail", trail["Name"])
			},
		},
		{
			name: "describe_trails_by_arn",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				createRec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "arn-trail-2",
					"S3BucketName": "bucket",
				})
				createResp := parseCloudTrailResp(t, createRec)
				trailARN := createResp["TrailARN"].(string)
				rec := doCloudTrailOp(t, h, "DescribeTrails", map[string]any{
					"trailNameList": []string{trailARN},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				list, ok := resp["trailList"].([]any)
				require.True(t, ok)
				assert.Len(t, list, 1)
			},
		},
		{
			name: "delete_by_arn",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				createRec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "arn-del-trail",
					"S3BucketName": "bucket",
				})
				createResp := parseCloudTrailResp(t, createRec)
				trailARN := createResp["TrailARN"].(string)
				rec := doCloudTrailOp(t, h, "DeleteTrail", map[string]any{
					"Name": trailARN,
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "start_logging_by_arn",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				createRec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "log-arn-trail",
					"S3BucketName": "bucket",
				})
				createResp := parseCloudTrailResp(t, createRec)
				trailARN := createResp["TrailARN"].(string)
				rec := doCloudTrailOp(t, h, "StartLogging", map[string]any{
					"Name": trailARN,
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "update_trail_boolean_fields",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "bool-trail",
					"S3BucketName": "bucket",
				})
				boolTrue := true
				boolFalse := false
				rec := doCloudTrailOp(t, h, "UpdateTrail", map[string]any{
					"Name":                       "bool-trail",
					"IncludeGlobalServiceEvents": boolTrue,
					"IsMultiRegionTrail":         boolFalse,
					"EnableLogFileValidation":    boolTrue,
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "update_trail_missing_name",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "UpdateTrail", map[string]any{
					"S3BucketName": "bucket",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "remove_tags_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "RemoveTags", map[string]any{
					"ResourceId": "arn:aws:cloudtrail:us-east-1:123456789012:trail/missing",
					"TagsList":   []map[string]string{{"Key": "Env"}},
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "put_event_selectors_with_data_resources",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "data-trail",
					"S3BucketName": "bucket",
				})
				rec := doCloudTrailOp(t, h, "PutEventSelectors", map[string]any{
					"TrailName": "data-trail",
					"EventSelectors": []map[string]any{
						{
							"ReadWriteType":           "All",
							"IncludeManagementEvents": true,
							"DataResources": []map[string]any{
								{"Type": "AWS::S3::Object", "Values": []string{"arn:aws:s3:::my-bucket/"}},
							},
						},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "get_event_selectors_not_found",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "GetEventSelectors", map[string]any{
					"TrailName": "missing-trail",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "list_tags_empty_resources",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "ListTags", map[string]any{
					"ResourceIdList": []string{},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				tagList, ok := resp["ResourceTagList"].([]any)
				require.True(t, ok)
				assert.Empty(t, tagList)
			},
		},
		{
			name: "add_tags_by_name",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "named-tag-trail",
					"S3BucketName": "bucket",
				})
				rec := doCloudTrailOp(t, h, "AddTags", map[string]any{
					"ResourceId": "named-tag-trail",
					"TagsList":   []map[string]string{{"Key": "K", "Value": "V"}},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "stop_logging_by_arn",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				createRec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "stop-arn-trail",
					"S3BucketName": "bucket",
				})
				createResp := parseCloudTrailResp(t, createRec)
				trailARN := createResp["TrailARN"].(string)
				doCloudTrailOp(t, h, "StartLogging", map[string]any{"Name": trailARN})
				rec := doCloudTrailOp(t, h, "StopLogging", map[string]any{
					"Name": trailARN,
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "get_trail_status_by_arn",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				createRec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "status-arn-trail",
					"S3BucketName": "bucket",
				})
				createResp := parseCloudTrailResp(t, createRec)
				trailARN := createResp["TrailARN"].(string)
				rec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
					"Name": trailARN,
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "update_trail_optional_string_fields",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "str-trail",
					"S3BucketName": "bucket",
				})
				rec := doCloudTrailOp(t, h, "UpdateTrail", map[string]any{
					"Name":                      "str-trail",
					"SnsTopicName":              "topic",
					"CloudWatchLogsLogGroupArn": "arn:aws:logs:us-east-1:123:log-group:test",
					"CloudWatchLogsRoleArn":     "arn:aws:iam::123:role/test",
					"KMSKeyId":                  "arn:aws:kms:us-east-1:123:key/abc",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, "arn:aws:logs:us-east-1:123:log-group:test", resp["CloudWatchLogsLogGroupArn"])
				assert.Equal(t, "arn:aws:iam::123:role/test", resp["CloudWatchLogsRoleArn"])
				// The wire key is "KmsKeyId" (matching the real aws-sdk-go-v2
				// deserializer's exact case), not "KMSKeyId".
				assert.Equal(t, "arn:aws:kms:us-east-1:123:key/abc", resp["KmsKeyId"])
			},
		},
		{
			name: "trail_map_with_all_optional_fields",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":                      "optional-trail",
					"S3BucketName":              "bucket",
					"S3KeyPrefix":               "prefix/",
					"SnsTopicName":              "my-sns",
					"CloudWatchLogsLogGroupArn": "arn:logs",
					"CloudWatchLogsRoleArn":     "arn:role",
					"KMSKeyId":                  "arn:kms",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, "prefix/", resp["S3KeyPrefix"])
				assert.Equal(t, "my-sns", resp["SnsTopicName"])
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

// TestCloudTrailListTrailsSortedOutput verifies deterministic sorted output.
func TestCloudTrailListTrailsSortedOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "list_trails_sorted_by_arn",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				for _, n := range []string{"trail-z", "trail-a", "trail-m"} {
					doCloudTrailOp(t, h, "CreateTrail", map[string]any{
						"Name": n, "S3BucketName": "bucket",
					})
				}
				rec := doCloudTrailOp(t, h, "ListTrails", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				list, ok := resp["Trails"].([]any)
				require.True(t, ok)
				require.Len(t, list, 3)
				// Verify sorted order by comparing ARNs (which contain the name)
				firstName := list[0].(map[string]any)["Name"].(string)
				secondName := list[1].(map[string]any)["Name"].(string)
				thirdName := list[2].(map[string]any)["Name"].(string)
				assert.Less(t, firstName, secondName)
				assert.Less(t, secondName, thirdName)
			},
		},
		{
			name: "describe_trails_sorted_by_name",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				for _, n := range []string{"trail-z", "trail-a", "trail-m"} {
					doCloudTrailOp(t, h, "CreateTrail", map[string]any{
						"Name": n, "S3BucketName": "bucket",
					})
				}
				rec := doCloudTrailOp(t, h, "DescribeTrails", nil)
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				list, ok := resp["trailList"].([]any)
				require.True(t, ok)
				require.Len(t, list, 3)
				names := []string{
					list[0].(map[string]any)["Name"].(string),
					list[1].(map[string]any)["Name"].(string),
					list[2].(map[string]any)["Name"].(string),
				}
				assert.Equal(t, []string{"trail-a", "trail-m", "trail-z"}, names)
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

// TestGetTrailStatusFullResponse verifies that GetTrailStatus returns the full
// AWS-accurate status object including timing fields.
func TestGetTrailStatusFullResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "status_before_logging_has_no_times",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "status-trail",
					"S3BucketName": "bucket",
				})
				rec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
					"Name": "status-trail",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, false, resp["IsLogging"])
				// No timing fields before logging starts.
				_, hasStart := resp["StartLoggingTime"]
				assert.False(t, hasStart, "StartLoggingTime should not be set before StartLogging")
			},
		},
		{
			name: "status_after_start_logging_has_start_time",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "logging-trail",
					"S3BucketName": "bucket",
				})
				doCloudTrailOp(t, h, "StartLogging", map[string]any{
					"Name": "logging-trail",
				})
				rec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
					"Name": "logging-trail",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, true, resp["IsLogging"])
				assert.NotNil(t, resp["StartLoggingTime"], "StartLoggingTime should be set after StartLogging")
				assert.NotNil(t, resp["LatestDeliveryTime"], "LatestDeliveryTime should be set after StartLogging")
			},
		},
		{
			name: "status_after_stop_logging_has_stop_time",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "stop-trail",
					"S3BucketName": "bucket",
				})
				doCloudTrailOp(t, h, "StartLogging", map[string]any{"Name": "stop-trail"})
				doCloudTrailOp(t, h, "StopLogging", map[string]any{"Name": "stop-trail"})
				rec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
					"Name": "stop-trail",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, false, resp["IsLogging"])
				assert.NotNil(t, resp["StopLoggingTime"], "StopLoggingTime should be set after StopLogging")
				assert.NotNil(t, resp["StartLoggingTime"], "StartLoggingTime should remain set")
			},
		},
		{
			name: "status_by_arn_returns_correct_status",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				createRec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "arn-status-trail",
					"S3BucketName": "bucket",
				})
				createResp := parseCloudTrailResp(t, createRec)
				trailARN := createResp["TrailARN"].(string)

				doCloudTrailOp(t, h, "StartLogging", map[string]any{"Name": trailARN})
				rec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
					"Name": trailARN,
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				assert.Equal(t, true, resp["IsLogging"])
			},
		},
		{
			name: "get_status_not_found_returns_404",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
					"Name": "missing-trail",
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

// TestTrailFields verifies that the trail response includes all expected AWS fields.
func TestTrailFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ops  func(t *testing.T, h *cloudtrail.Handler)
		name string
	}{
		{
			name: "trail_has_has_insight_selectors_field",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "fields-trail",
					"S3BucketName": "bucket",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				_, hasField := resp["HasInsightSelectors"]
				assert.True(t, hasField, "trail should have HasInsightSelectors field")
				assert.Equal(t, false, resp["HasInsightSelectors"])
			},
		},
		{
			name: "trail_has_is_organization_trail_field",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "org-fields-trail",
					"S3BucketName": "bucket",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				_, hasField := resp["IsOrganizationTrail"]
				assert.True(t, hasField, "trail should have IsOrganizationTrail field")
				assert.Equal(t, false, resp["IsOrganizationTrail"])
			},
		},
		{
			name: "describe_trails_includes_has_insight_selectors",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":         "desc-insight-trail",
					"S3BucketName": "bucket",
				})
				doCloudTrailOp(t, h, "PutInsightSelectors", map[string]any{
					"TrailName": "desc-insight-trail",
					"InsightSelectors": []map[string]any{
						{"InsightType": "ApiCallRateInsight"},
					},
				})
				rec := doCloudTrailOp(t, h, "DescribeTrails", nil)
				resp := parseCloudTrailResp(t, rec)
				list := resp["trailList"].([]any)
				require.Len(t, list, 1)
				trail := list[0].(map[string]any)
				assert.Equal(t, true, trail["HasInsightSelectors"])
			},
		},
		{
			name: "get_trail_includes_all_standard_fields",
			ops: func(t *testing.T, h *cloudtrail.Handler) {
				t.Helper()
				doCloudTrailOp(t, h, "CreateTrail", map[string]any{
					"Name":                       "complete-trail",
					"S3BucketName":               "my-bucket",
					"S3KeyPrefix":                "prefix/",
					"SnsTopicName":               "my-topic",
					"IncludeGlobalServiceEvents": true,
					"IsMultiRegionTrail":         true,
					"EnableLogFileValidation":    true,
				})
				rec := doCloudTrailOp(t, h, "GetTrail", map[string]any{
					"Name": "complete-trail",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseCloudTrailResp(t, rec)
				trail := resp["Trail"].(map[string]any)
				assert.NotEmpty(t, trail["TrailARN"])
				assert.NotEmpty(t, trail["HomeRegion"])
				assert.Equal(t, true, trail["IncludeGlobalServiceEvents"])
				assert.Equal(t, true, trail["IsMultiRegionTrail"])
				assert.Equal(t, true, trail["LogFileValidationEnabled"])
				assert.Equal(t, false, trail["HasCustomEventSelectors"])
				assert.Equal(t, false, trail["HasInsightSelectors"])
				assert.Equal(t, false, trail["IsOrganizationTrail"])
				assert.Equal(t, "prefix/", trail["S3KeyPrefix"])
				assert.Equal(t, "my-topic", trail["SnsTopicName"])
				assert.NotEmpty(t, trail["SnsTopicARN"])
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

// TestGetTrailStatusTimeLoggingStarted verifies that GetTrailStatus returns
// TimeLoggingStarted as a string field when logging is active.
func TestGetTrailStatusTimeLoggingStarted(t *testing.T) {
	t.Parallel()

	h := newTestCloudTrailHandler()

	doCloudTrailOp(t, h, "CreateTrail", map[string]any{
		"Name":         "time-start-trail",
		"S3BucketName": "bucket",
	})
	doCloudTrailOp(t, h, "StartLogging", map[string]any{
		"Name": "time-start-trail",
	})

	rec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
		"Name": "time-start-trail",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseCloudTrailResp(t, rec)
	assert.True(t, resp["IsLogging"].(bool))
	assert.NotNil(t, resp["StartLoggingTime"], "StartLoggingTime (float) must be present")
	assert.NotEmpty(t, resp["TimeLoggingStarted"], "TimeLoggingStarted (string) must be present")

	_, isString := resp["TimeLoggingStarted"].(string)
	assert.True(t, isString, "TimeLoggingStarted must be a string timestamp")
}

// TestGetTrailStatusTimeLoggingStopped verifies that GetTrailStatus returns
// TimeLoggingStopped as a string field after StopLogging.
func TestGetTrailStatusTimeLoggingStopped(t *testing.T) {
	t.Parallel()

	h := newTestCloudTrailHandler()

	doCloudTrailOp(t, h, "CreateTrail", map[string]any{
		"Name":         "time-stop-trail",
		"S3BucketName": "bucket",
	})
	doCloudTrailOp(t, h, "StartLogging", map[string]any{"Name": "time-stop-trail"})
	doCloudTrailOp(t, h, "StopLogging", map[string]any{"Name": "time-stop-trail"})

	rec := doCloudTrailOp(t, h, "GetTrailStatus", map[string]any{
		"Name": "time-stop-trail",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseCloudTrailResp(t, rec)
	assert.NotNil(t, resp["StopLoggingTime"], "StopLoggingTime (float) must be present")
	assert.NotEmpty(t, resp["TimeLoggingStopped"], "TimeLoggingStopped (string) must be present")

	_, isString := resp["TimeLoggingStopped"].(string)
	assert.True(t, isString, "TimeLoggingStopped must be a string timestamp")
}

// TestListTrails_NextTokenPagination verifies ListTrails honors a
// caller-supplied NextToken (previously always returned every trail in one
// page, silently ignoring any NextToken from a prior call).
func TestListTrails_NextTokenPagination(t *testing.T) {
	t.Parallel()

	h := newTestCloudTrailHandler()

	for i := range 3 {
		rec := doCloudTrailOp(t, h, "CreateTrail", map[string]any{
			"Name":         fmt.Sprintf("trail-%d", i),
			"S3BucketName": "bucket",
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec := doCloudTrailOp(t, h, "ListTrails", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseCloudTrailResp(t, rec)
	trails, ok := resp["Trails"].([]any)
	require.True(t, ok)
	assert.Len(t, trails, 3)
	assert.Nil(t, resp["NextToken"], "no NextToken when everything fits in the default page")

	// Skip the first 2 (sorted-by-ARN) trails via an opaque NextToken -- the
	// same token shape page.New produces internally -- and verify the
	// returned page actually starts from the offset instead of ignoring it.
	rec = doCloudTrailOp(t, h, "ListTrails", map[string]any{"NextToken": page.EncodeToken(2)})
	require.Equal(t, http.StatusOK, rec.Code)
	resp = parseCloudTrailResp(t, rec)
	trails, ok = resp["Trails"].([]any)
	require.True(t, ok)
	assert.Len(t, trails, 1, "NextToken skipping 2 of 3 trails must leave exactly 1")
}
