package iotanalytics_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandler_TagOperations covers ListTagsForResource/TagResource/UntagResource.
func TestHandler_TagOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		op         string
		wantStatus int
	}{
		{
			name:       "list_tags_success",
			op:         "list",
			wantStatus: http.StatusOK,
		},
		{
			name:       "tag_resource",
			op:         "tag",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "untag_resource",
			op:         "untag",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "list_tags_missing_arn",
			op:         "list_missing_arn",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "untag_missing_tagkeys",
			op:         "untag_missing_keys",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create a channel so we have a valid ARN.
			createRec := doRequest(
				t,
				h,
				http.MethodPost,
				"/channels",
				map[string]string{"channelName": "tag_test_channel"},
			)
			require.Equal(t, http.StatusOK, createRec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
			arn, _ := createResp["channelArn"].(string)
			require.NotEmpty(t, arn)

			switch tt.op {
			case "list":
				rec := doRequest(t, h, http.MethodGet, "/tags?resourceArn="+arn, nil)
				assert.Equal(t, tt.wantStatus, rec.Code)

			case "tag":
				rec := doRequest(t, h, http.MethodPost, "/tags?resourceArn="+arn, map[string]any{
					"tags": []map[string]string{{"key": "env", "value": "test"}},
				})
				assert.Equal(t, tt.wantStatus, rec.Code)

			case "untag":
				rec := doRequest(t, h, http.MethodDelete, "/tags?resourceArn="+arn+"&tagKeys=env", nil)
				assert.Equal(t, tt.wantStatus, rec.Code)

			case "list_missing_arn":
				rec := doRequest(t, h, http.MethodGet, "/tags", nil)
				assert.Equal(t, tt.wantStatus, rec.Code)

			case "untag_missing_keys":
				rec := doRequest(t, h, http.MethodDelete, "/tags?resourceArn="+arn, nil)
				assert.Equal(t, tt.wantStatus, rec.Code)
			}
		})
	}
}

// TestHandler_CreateResource_InvalidTagsRejected verifies that Create* operations validate
// tags up front, the same way TagResource does, instead of persisting a resource with tags
// that would be rejected had they been attached via a separate TagResource call.
func TestHandler_CreateResource_InvalidTagsRejected(t *testing.T) {
	t.Parallel()

	invalidTags := []map[string]string{{"key": "aws:reserved", "value": "v"}}
	validTags := []map[string]string{{"key": "env", "value": "prod"}}

	tests := []struct {
		body func(tags []map[string]string) map[string]any
		name string
		path string
	}{
		{
			name: "channel",
			path: "/channels",
			body: func(tags []map[string]string) map[string]any {
				return map[string]any{"channelName": "invtag_ch", "tags": tags}
			},
		},
		{
			name: "datastore",
			path: "/datastores",
			body: func(tags []map[string]string) map[string]any {
				return map[string]any{"datastoreName": "invtag_ds", "tags": tags}
			},
		},
		{
			name: "dataset",
			path: "/datasets",
			body: func(tags []map[string]string) map[string]any {
				return map[string]any{"datasetName": "invtag_dset", "tags": tags}
			},
		},
		{
			name: "pipeline",
			path: "/pipelines",
			body: func(tags []map[string]string) map[string]any {
				return map[string]any{
					"pipelineName":       "invtag_pl",
					"tags":               tags,
					"pipelineActivities": validPipelineActivitiesBody(),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, http.MethodPost, tt.path, tt.body(invalidTags))
			require.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, "InvalidRequestException", errResp["__type"])

			okRec := doRequest(t, h, http.MethodPost, tt.path, tt.body(validTags))
			assert.Equal(t, http.StatusOK, okRec.Code)
		})
	}
}

// TestHandler_TagResource_Returns204 verifies TagResource returns HTTP 204 (not 200).
func TestHandler_TagResource_Returns204(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tags       []map[string]string
		wantStatus int
	}{
		{
			name:       "valid_tag_returns_204",
			tags:       []map[string]string{{"key": "env", "value": "prod"}},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "invalid_key_returns_400",
			tags:       []map[string]string{{"key": "aws:reserved", "value": "v"}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/channels", map[string]string{"channelName": "tag204ch"})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			arn, _ := resp["channelArn"].(string)
			require.NotEmpty(t, arn)

			tagRec := doRequest(t, h, http.MethodPost, "/tags?resourceArn="+arn, map[string]any{"tags": tt.tags})
			assert.Equal(t, tt.wantStatus, tagRec.Code)
		})
	}
}

// TestHandler_TagValidation verifies tag key/value length limits.
func TestHandler_TagValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tags       []map[string]string
		wantStatus int
	}{
		{
			name:       "valid_tags",
			tags:       []map[string]string{{"key": "env", "value": "test"}},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "key_128_chars_ok",
			tags:       []map[string]string{{"key": strings.Repeat("k", 128), "value": "v"}},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "key_129_chars_rejected",
			tags:       []map[string]string{{"key": strings.Repeat("k", 129), "value": "v"}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "value_256_chars_ok",
			tags:       []map[string]string{{"key": "k", "value": strings.Repeat("v", 256)}},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "value_257_chars_rejected",
			tags:       []map[string]string{{"key": "k", "value": strings.Repeat("v", 257)}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_key_rejected",
			tags:       []map[string]string{{"key": "", "value": "v"}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, http.MethodPost, "/channels", map[string]any{"channelName": "tag_val_ch"})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			arn, _ := resp["channelArn"].(string)
			require.NotEmpty(t, arn)

			tagRec := doRequest(t, h, http.MethodPost, "/tags?resourceArn="+arn, map[string]any{
				"tags": tt.tags,
			})
			assert.Equal(t, tt.wantStatus, tagRec.Code)
		})
	}
}

// TestHandler_ListTagsForResource_EmptyNotError verifies empty tags returns [] not 404.
func TestHandler_ListTagsForResource_EmptyNotError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		kind   string
		body   map[string]any
		arnKey string
	}{
		{
			name:   "channel_no_tags",
			kind:   "channel",
			body:   map[string]any{"channelName": "notag_ch"},
			arnKey: "channelArn",
		},
		{
			name:   "datastore_no_tags",
			kind:   "datastore",
			body:   map[string]any{"datastoreName": "notag_ds"},
			arnKey: "datastoreArn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			var createPath string
			switch tt.kind {
			case "channel":
				createPath = "/channels"
			case "datastore":
				createPath = "/datastores"
			}

			rec := doRequest(t, h, http.MethodPost, createPath, tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
			arn, _ := createResp[tt.arnKey].(string)
			require.NotEmpty(t, arn)

			tagRec := doRequest(t, h, http.MethodGet, "/tags?resourceArn="+arn, nil)
			assert.Equal(t, http.StatusOK, tagRec.Code)

			var tagResp map[string]any
			require.NoError(t, json.Unmarshal(tagRec.Body.Bytes(), &tagResp))
			tags, ok := tagResp["tags"].([]any)
			require.True(t, ok, "tags field must be array")
			assert.Empty(t, tags, "newly created resource must have empty tag array")
		})
	}
}

// TestHandler_TagsInDescribeResponse verifies tags appear in describe responses for
// resources created with tags.
func TestHandler_TagsInDescribeResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		path   string
		create string
	}{
		{
			name:   "channel_with_tags",
			path:   "/channels/tagged_ch",
			create: "/channels",
			body: map[string]any{
				"channelName": "tagged_ch",
				"tags":        []map[string]string{{"key": "env", "value": "prod"}},
			},
		},
		{
			name:   "pipeline_with_tags",
			path:   "/pipelines/tagged_pipe",
			create: "/pipelines",
			body: map[string]any{
				"pipelineName":       "tagged_pipe",
				"tags":               []map[string]string{{"key": "team", "value": "alpha"}},
				"pipelineActivities": validPipelineActivitiesBody(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, tt.create, tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			rec2 := doRequest(t, h, http.MethodGet, tt.path, nil)
			require.Equal(t, http.StatusOK, rec2.Code)
			assert.Contains(t, rec2.Body.String(), `"tags"`)
		})
	}
}
