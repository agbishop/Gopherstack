package elasticsearch_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestElasticsearchHandler_Tags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		domain    string
		wantCount int
	}{
		{
			name:      "add_and_list_tags",
			domain:    "tag-domain-al",
			operation: "add_list",
			wantCount: 2,
		},
		{
			name:      "remove_tag",
			domain:    "tag-domain-rm",
			operation: "remove",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler()
			domainARN := createDomainAndGetARN(t, h, tt.domain)

			addResp := doRequest(t, h, http.MethodPost, "/2015-01-01/tags", map[string]any{
				"ARN": domainARN,
				"TagList": []map[string]string{
					{"Key": "env", "Value": "prod"},
					{"Key": "team", "Value": "platform"},
				},
			})
			addResp.Body.Close()

			if tt.operation == "remove" {
				removeResp := doRequest(t, h, http.MethodPost, "/2015-01-01/tags-removal", map[string]any{
					"ARN":     domainARN,
					"TagKeys": []string{"env"},
				})
				removeResp.Body.Close()
			}

			listResp := doRequest(t, h, http.MethodGet, "/2015-01-01/tags?arn="+domainARN, nil)
			defer listResp.Body.Close()
			assert.Equal(t, http.StatusOK, listResp.StatusCode)

			var listOut map[string]any
			require.NoError(t, json.NewDecoder(listResp.Body).Decode(&listOut))

			tagList, ok := listOut["TagList"].([]any)
			require.True(t, ok)
			assert.Len(t, tagList, tt.wantCount)
		})
	}
}

func TestElasticsearchHandler_Tags_InvalidBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		wantCode int
	}{
		{
			name:     "add_tags_invalid_json",
			path:     "/2015-01-01/tags",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "remove_tags_invalid_json",
			path:     "/2015-01-01/tags-removal",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler()

			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader("not-json"))
			req.Header.Set("Content-Type", "application/json")

			rw := httptest.NewRecorder()
			h.ServeHTTP(rw, req)

			assert.Equal(t, tt.wantCode, rw.Code)
		})
	}
}

func TestElasticsearchHandler_ListTags_UnknownARN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arn  string
	}{
		{
			name: "constructed_arn",
			arn:  "arn:aws:es:us-east-1:123456789012:domain/nonexistent",
		},
		{
			name: "no_such_domain",
			arn:  "arn:aws:es:us-east-1:123456789012:domain/no-such-domain",
		},
		{
			name: "empty_arn",
			arn:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			resp := doRequest(t, h, http.MethodGet, "/2015-01-01/tags?arn="+tt.arn, nil)
			defer resp.Body.Close()

			// AWS returns 404 for unknown domain ARN (not 200 with empty list).
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	}
}

// TestElasticsearchHandler_TagsFixedRouteWorks verifies the fixed-path ops
// dispatch table serves GET /2015-01-01/tags for an unknown ARN as 404.
func TestElasticsearchHandler_TagsFixedRouteWorks(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	resp := doRequest(t, h, http.MethodGet, "/2015-01-01/tags?arn=nonexistent", nil)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestElasticsearchHandler_TagKeyLength verifies tag key length constraints (1-128 chars).
func TestElasticsearchHandler_TagKeyLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		key        string
		wantStatus int
	}{
		{name: "empty-key", key: "", wantStatus: http.StatusBadRequest},
		{name: "max-key", key: strings.Repeat("k", 128), wantStatus: http.StatusOK},
		{name: "key-too-long", key: strings.Repeat("k", 129), wantStatus: http.StatusBadRequest},
		{name: "valid-key", key: "env", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			arn := createDomainAndGetARN(t, h, "tagkey-"+tt.name[:min(3, len(tt.name))])

			resp := doRequest(t, h, http.MethodPost, "/2015-01-01/tags", map[string]any{
				"ARN":     arn,
				"TagList": []map[string]string{{"Key": tt.key, "Value": "val"}},
			})
			defer resp.Body.Close()
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

// TestElasticsearchHandler_TagValueLength verifies tag value length constraint (0-256 chars).
func TestElasticsearchHandler_TagValueLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		value      string
		wantStatus int
	}{
		{name: "empty-val", value: "", wantStatus: http.StatusOK},
		{name: "max-val", value: strings.Repeat("v", 256), wantStatus: http.StatusOK},
		{name: "val-too-long", value: strings.Repeat("v", 257), wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			arn := createDomainAndGetARN(t, h, "tagval-"+tt.name[:min(3, len(tt.name))])

			resp := doRequest(t, h, http.MethodPost, "/2015-01-01/tags", map[string]any{
				"ARN":     arn,
				"TagList": []map[string]string{{"Key": "env", "Value": tt.value}},
			})
			defer resp.Body.Close()
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

// TestElasticsearchHandler_TagMaxPerResource verifies max 50 tags per resource.
func TestElasticsearchHandler_TagMaxPerResource(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	arn := createDomainAndGetARN(t, h, "tagmax-dom")

	tags50 := make([]map[string]string, 50)
	for i := range tags50 {
		tags50[i] = map[string]string{"Key": fmt.Sprintf("tag-key-%02d", i), "Value": "v"}
	}

	resp := doRequest(t, h, http.MethodPost, "/2015-01-01/tags", map[string]any{
		"ARN":     arn,
		"TagList": tags50,
	})
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Adding one new tag exceeds the 50-tag limit.
	resp2 := doRequest(t, h, http.MethodPost, "/2015-01-01/tags", map[string]any{
		"ARN":     arn,
		"TagList": []map[string]string{{"Key": "overflow-key", "Value": "x"}},
	})
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp2.StatusCode)
}

// TestElasticsearchHandler_TagUpdateExistingKeyNoLimit verifies updating an
// existing tag key does not count as new.
func TestElasticsearchHandler_TagUpdateExistingKeyNoLimit(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	arn := createDomainAndGetARN(t, h, "tagupd-dom")

	tags50 := make([]map[string]string, 50)
	for i := range tags50 {
		tags50[i] = map[string]string{"Key": fmt.Sprintf("tag-key-%02d", i), "Value": "v"}
	}

	resp := doRequest(t, h, http.MethodPost, "/2015-01-01/tags", map[string]any{
		"ARN":     arn,
		"TagList": tags50,
	})
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Updating an existing key does not add a new tag -- should succeed.
	resp2 := doRequest(t, h, http.MethodPost, "/2015-01-01/tags", map[string]any{
		"ARN":     arn,
		"TagList": []map[string]string{{"Key": tags50[0]["Key"], "Value": "new-val"}},
	})
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

// TestElasticsearchHandler_SortedListTagsResponse verifies that the HTTP
// ListTags response is sorted by key.
func TestElasticsearchHandler_SortedListTagsResponse(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	arn := createDomainAndGetARN(t, h, "sorted-tags-d")

	doRequest(t, h, http.MethodPost, "/2015-01-01/tags", map[string]any{
		"ARN": arn,
		"TagList": []map[string]string{
			{"Key": "zzz", "Value": "last"},
			{"Key": "aaa", "Value": "first"},
			{"Key": "mmm", "Value": "middle"},
		},
	}).Body.Close()

	resp := doRequest(t, h, http.MethodGet, "/2015-01-01/tags?arn="+arn, nil)
	defer resp.Body.Close()

	var out struct {
		TagList []struct {
			Key   string `json:"Key"`
			Value string `json:"Value"`
		} `json:"TagList"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Len(t, out.TagList, 3)
	assert.Equal(t, "aaa", out.TagList[0].Key)
	assert.Equal(t, "mmm", out.TagList[1].Key)
	assert.Equal(t, "zzz", out.TagList[2].Key)
}

// TestElasticsearchHandler_AddTags_DuplicateKeyRejected verifies that AddTags
// rejects a tag list containing duplicate keys with ValidationException,
// matching AWS behaviour.
func TestElasticsearchHandler_AddTags_DuplicateKeyRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		domainName string
		name       string
		tags       []map[string]string
		wantCode   int
	}{
		{
			name:       "no_duplicates_accepted",
			domainName: "tag-dup-nodup",
			tags:       []map[string]string{{"Key": "env", "Value": "prod"}, {"Key": "team", "Value": "ops"}},
			wantCode:   http.StatusOK,
		},
		{
			name:       "duplicate_key_rejected",
			domainName: "tag-dup-dupkey",
			tags:       []map[string]string{{"Key": "env", "Value": "prod"}, {"Key": "env", "Value": "dev"}},
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "three_tags_one_duplicate_rejected",
			domainName: "tag-dup-three",
			tags: []map[string]string{
				{"Key": "a", "Value": "1"},
				{"Key": "b", "Value": "2"},
				{"Key": "a", "Value": "3"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:       "single_tag_accepted",
			domainName: "tag-dup-solo",
			tags:       []map[string]string{{"Key": "solo", "Value": "v"}},
			wantCode:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			arn := createDomainAndGetARN(t, h, tt.domainName)

			resp := doRequest(t, h, http.MethodPost, "/2015-01-01/tags", map[string]any{
				"ARN":     arn,
				"TagList": tt.tags,
			})
			defer resp.Body.Close()

			assert.Equal(t, tt.wantCode, resp.StatusCode)
			if tt.wantCode == http.StatusBadRequest {
				var out map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
				assert.Contains(t, out["message"], "Duplicate tag key")
			}
		})
	}
}

// TestElasticsearchHandler_AddRemoveTags_UnknownARN verifies AddTags and
// RemoveTags reject an ARN that does not resolve to any domain with
// ValidationException (400), matching real AWS: neither op's deserializer
// (elasticsearchservice@v1.45.4 deserializers.go,
// awsRestjson1_deserializeOpErrorAddTags / ...RemoveTags) declares
// ResourceNotFoundException -- only BaseException/InternalException/
// ValidationException(+LimitExceededException on AddTags). Before the fix
// both handlers discarded the backend's ErrDomainNotFound and always wrote
// 200 OK; a non-empty TagList also risked a nil-map panic in AddTags
// (maps.Copy into the nil map returned by ListTags on the same unknown ARN).
func TestElasticsearchHandler_AddRemoveTags_UnknownARN(t *testing.T) {
	t.Parallel()

	const unknownARN = "arn:aws:es:us-east-1:123456789012:domain/no-such-domain"

	tests := []struct {
		body map[string]any
		name string
		path string
	}{
		{
			name: "add_tags_unknown_arn",
			path: "/2015-01-01/tags",
			body: map[string]any{
				"ARN":     unknownARN,
				"TagList": []map[string]string{{"Key": "env", "Value": "prod"}},
			},
		},
		{
			name: "remove_tags_unknown_arn",
			path: "/2015-01-01/tags-removal",
			body: map[string]any{
				"ARN":     unknownARN,
				"TagKeys": []string{"env"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			resp := doRequest(t, h, http.MethodPost, tt.path, tt.body)
			defer resp.Body.Close()

			require.Equal(t, http.StatusBadRequest, resp.StatusCode)

			var out map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
			assert.Contains(t, out["message"], "domain not found")
		})
	}
}
