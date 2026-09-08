package forecast_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTags_UnknownResourceNotFound verifies that TagResource, UntagResource,
// and ListTagsForResource return ResourceNotFoundException for an ARN that
// was never created, matching the real Amazon Forecast API (which models
// ResourceNotFoundException on all three operations). Previously these
// operations accepted any ARN and silently wrote/read an orphaned tag map
// entry instead of validating the resource exists.
func TestTags_UnknownResourceNotFound(t *testing.T) {
	t.Parallel()

	unknownARN := "arn:aws:forecast:us-east-1:000000000000:dataset-group/never-created"

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "tag_resource",
			action: "TagResource",
			body: map[string]any{
				"ResourceArn": unknownARN,
				"Tags":        []any{map[string]any{"Key": "env", "Value": "test"}},
			},
		},
		{
			name:   "untag_resource",
			action: "UntagResource",
			body:   map[string]any{"ResourceArn": unknownARN, "TagKeys": []any{"env"}},
		},
		{
			name:   "list_tags_for_resource",
			action: "ListTagsForResource",
			body:   map[string]any{"ResourceArn": unknownARN},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			code, resp := request(t, h, tt.action, tt.body)
			assert.Equal(t, http.StatusBadRequest, code)
			assert.Equal(t, "ResourceNotFoundException", resp["__type"])
		})
	}
}

// TestTags_RoundTrip verifies TagResource/ListTagsForResource/UntagResource
// Key+Value shape across a tag/list/untag/list cycle.
func TestTags_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newHandler()

	code, created := request(t, h, "CreateDatasetGroup", map[string]any{
		"DatasetGroupName": "tag-group", "Domain": "RETAIL",
	})
	require.Equal(t, http.StatusOK, code)
	arn := created["DatasetGroupArn"].(string)

	// Tag
	code, _ = request(t, h, "TagResource", map[string]any{
		"ResourceArn": arn,
		"Tags": []any{
			map[string]any{"Key": "env", "Value": "test"},
			map[string]any{"Key": "owner", "Value": "audit"},
		},
	})
	require.Equal(t, http.StatusOK, code)

	// List tags
	code, m := request(t, h, "ListTagsForResource", map[string]any{"ResourceArn": arn})
	require.Equal(t, http.StatusOK, code)
	tags, ok := m["Tags"].([]any)
	require.True(t, ok)
	require.Len(t, tags, 2)

	tagMap := make(map[string]string)
	for _, tag := range tags {
		t2 := tag.(map[string]any)
		tagMap[t2["Key"].(string)] = t2["Value"].(string)
	}
	assert.Equal(t, "test", tagMap["env"])
	assert.Equal(t, "audit", tagMap["owner"])

	// Untag
	code, _ = request(t, h, "UntagResource", map[string]any{
		"ResourceArn": arn,
		"TagKeys":     []any{"env"},
	})
	require.Equal(t, http.StatusOK, code)

	_, m = request(t, h, "ListTagsForResource", map[string]any{"ResourceArn": arn})
	tags = m["Tags"].([]any)
	assert.Len(t, tags, 1)
	assert.Equal(t, "owner", tags[0].(map[string]any)["Key"])
}

// tagKVs builds n {"Key":"k<i>","Value":"v<i>"} entries starting at offset,
// the []any shape TagResource's Tags field expects.
func tagKVs(n, offset int) []any {
	out := make([]any, n)
	for i := range n {
		k := fmt.Sprintf("k%d", offset+i)
		out[i] = map[string]any{"Key": k, "Value": "v" + k}
	}

	return out
}

// TestTagResource_LimitExceeded verifies Amazon Forecast's documented 50
// tag-per-resource maximum (https://docs.aws.amazon.com/forecast/latest/dg/limits.html,
// "Maximum number of tags you can add to a resource | 50 | No"): the check
// is on the resource's resulting tag set, not the incoming request size, and
// re-tagging an existing key is an update rather than an addition.
func TestTagResource_LimitExceeded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantType   string
		newTags    []any
		preTagKeys int
		wantCode   int
	}{
		{
			name:       "exactly_50_succeeds",
			preTagKeys: 0,
			newTags:    tagKVs(50, 0),
			wantCode:   http.StatusOK,
		},
		{
			name:       "51_fails",
			preTagKeys: 0,
			newTags:    tagKVs(51, 0),
			wantCode:   http.StatusBadRequest,
			wantType:   "LimitExceededException",
		},
		{
			name:       "accumulation_45_plus_10_fails",
			preTagKeys: 45,
			newTags:    tagKVs(10, 45),
			wantCode:   http.StatusBadRequest,
			wantType:   "LimitExceededException",
		},
		{
			name:       "retagging_existing_key_at_limit_succeeds",
			preTagKeys: 50,
			newTags:    []any{map[string]any{"Key": "k0", "Value": "updated"}},
			wantCode:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			code, created := request(t, h, "CreateDatasetGroup", map[string]any{
				"DatasetGroupName": "tag-limit-" + tt.name, "Domain": "RETAIL",
			})
			require.Equal(t, http.StatusOK, code)
			arn := created["DatasetGroupArn"].(string)

			if tt.preTagKeys > 0 {
				code, _ = request(t, h, "TagResource", map[string]any{
					"ResourceArn": arn,
					"Tags":        tagKVs(tt.preTagKeys, 0),
				})
				require.Equal(t, http.StatusOK, code)
			}

			code, resp := request(t, h, "TagResource", map[string]any{
				"ResourceArn": arn,
				"Tags":        tt.newTags,
			})
			assert.Equal(t, tt.wantCode, code)
			if tt.wantType != "" {
				assert.Equal(t, tt.wantType, resp["__type"])
			}

			if tt.name == "retagging_existing_key_at_limit_succeeds" {
				_, list := request(t, h, "ListTagsForResource", map[string]any{"ResourceArn": arn})
				listedTags := list["Tags"].([]any)
				require.Len(t, listedTags, 50)
				for _, tag := range listedTags {
					m := tag.(map[string]any)
					if m["Key"] == "k0" {
						assert.Equal(t, "updated", m["Value"])
					}
				}
			}
		})
	}
}
