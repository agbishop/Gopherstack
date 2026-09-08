package servicediscovery_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_TagsLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doSDRequest(t, h, "CreateHttpNamespace", map[string]any{
		"Name": "tag-ns",
		"Tags": []map[string]string{{"Key": "env", "Value": "test"}},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	opID := createResp["OperationId"].(string)

	opRec := doSDRequest(t, h, "GetOperation", map[string]any{"OperationId": opID})
	require.Equal(t, http.StatusOK, opRec.Code)

	var opResp map[string]any
	require.NoError(t, json.Unmarshal(opRec.Body.Bytes(), &opResp))
	operation := opResp["Operation"].(map[string]any)
	targets := operation["Targets"].(map[string]any)
	nsID := targets["NAMESPACE"].(string)

	nsRec := doSDRequest(t, h, "GetNamespace", map[string]any{"Id": nsID})
	require.Equal(t, http.StatusOK, nsRec.Code)

	var nsResp map[string]any
	require.NoError(t, json.Unmarshal(nsRec.Body.Bytes(), &nsResp))
	arn := nsResp["Namespace"].(map[string]any)["Arn"].(string)

	rec := doSDRequest(t, h, "ListTagsForResource", map[string]any{"ResourceARN": arn})
	require.Equal(t, http.StatusOK, rec.Code)

	tagRec := doSDRequest(t, h, "TagResource", map[string]any{
		"ResourceARN": arn,
		"Tags":        []map[string]string{{"Key": "team", "Value": "platform"}},
	})
	assert.Equal(t, http.StatusOK, tagRec.Code)

	untagRec := doSDRequest(t, h, "UntagResource", map[string]any{
		"ResourceARN": arn,
		"TagKeys":     []string{"env"},
	})
	assert.Equal(t, http.StatusOK, untagRec.Code)
}

func TestHandler_TagsErrors(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		body       any
		name       string
		op         string
		wantStatus int
	}{
		{
			name:       "list_tags_missing_arn",
			op:         "ListTagsForResource",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "tag_resource_missing_arn",
			op:         "TagResource",
			body:       map[string]any{"Tags": []map[string]string{}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "untag_resource_missing_arn",
			op:         "UntagResource",
			body:       map[string]any{"TagKeys": []string{"env"}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "list_tags_not_found",
			op:   "ListTagsForResource",
			body: map[string]any{
				"ResourceARN": "arn:aws:servicediscovery:us-east-1:000000000000:" +
					"namespace/ns-does-not-exist",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doSDRequest(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_TagResource_TooManyTags verifies that exceeding the 50-tag limit
// returns the dedicated TooManyTagsException error, not a generic InvalidInput.
func TestHandler_TagResource_TooManyTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tags := make([]map[string]string, 0, 51)
	for i := range 51 {
		tags = append(tags, map[string]string{"Key": fmt.Sprintf("k%d", i), "Value": "v"})
	}

	rec := doSDRequest(t, h, "CreateHttpNamespace", map[string]any{
		"Name": "too-many-tags-ns",
		"Tags": tags,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "TooManyTagsException")
}

// TestHandler_TagResource_TooManyTags_Merged verifies TagResource rejects a
// call that would push an EXISTING resource's total tag count over the
// 50-tag quota, even though each individual TagResource call stays under the
// limit -- per TooManyTagsException's doc comment ("The maximum number of
// tags that can be applied to a resource is 50"), the quota is on the
// resource, not the request.
func TestHandler_TagResource_TooManyTags_Merged(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doSDRequest(t, h, "CreateHttpNamespace", map[string]any{"Name": "merged-tags-ns"})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	opID := createResp["OperationId"].(string)

	opRec := doSDRequest(t, h, "GetOperation", map[string]any{"OperationId": opID})
	require.Equal(t, http.StatusOK, opRec.Code)

	var opResp map[string]any
	require.NoError(t, json.Unmarshal(opRec.Body.Bytes(), &opResp))
	nsID := opResp["Operation"].(map[string]any)["Targets"].(map[string]any)["NAMESPACE"].(string)

	getRec := doSDRequest(t, h, "GetNamespace", map[string]any{"Id": nsID})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	arn := getResp["Namespace"].(map[string]any)["Arn"].(string)

	firstBatch := make([]map[string]string, 0, 40)
	for i := range 40 {
		firstBatch = append(firstBatch, map[string]string{"Key": fmt.Sprintf("a%d", i), "Value": "v"})
	}

	tagRec := doSDRequest(t, h, "TagResource", map[string]any{"ResourceARN": arn, "Tags": firstBatch})
	require.Equal(t, http.StatusOK, tagRec.Code, "first 40-tag batch should succeed: %s", tagRec.Body.String())

	secondBatch := make([]map[string]string, 0, 20)
	for i := range 20 {
		secondBatch = append(secondBatch, map[string]string{"Key": fmt.Sprintf("b%d", i), "Value": "v"})
	}

	rec := doSDRequest(t, h, "TagResource", map[string]any{"ResourceARN": arn, "Tags": secondBatch})
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"40 existing + 20 new tags exceeds the 50-tag resource quota: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "TooManyTagsException")
}

// TestMapToTagEntriesSorted verifies that tag entries are sorted
// deterministically in ListTagsForResource -- the only op that returns tags
// (real ListServices/CreateService never include a Tags field).
func TestMapToTagEntriesSorted(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doSDRequest(t, h, "CreateService", map[string]any{
		"Name": "sort-svc",
		"Tags": []map[string]any{
			{"Key": "zzz", "Value": "last"},
			{"Key": "aaa", "Value": "first"},
			{"Key": "mmm", "Value": "middle"},
		},
	})
	require.Equal(t, 200, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	svcARN := createResp["Service"].(map[string]any)["Arn"].(string)

	tagsRec := doSDRequest(t, h, "ListTagsForResource", map[string]any{"ResourceARN": svcARN})
	require.Equal(t, 200, tagsRec.Code)

	var tagsResp map[string]any
	require.NoError(t, json.Unmarshal(tagsRec.Body.Bytes(), &tagsResp))

	tags := tagsResp["Tags"].([]any)
	require.Len(t, tags, 3)
	assert.Equal(t, "aaa", tags[0].(map[string]any)["Key"])
	assert.Equal(t, "mmm", tags[1].(map[string]any)["Key"])
	assert.Equal(t, "zzz", tags[2].(map[string]any)["Key"])
}

// TestHandler_ErrResourceNotFoundTagging verifies that tagging a non-existent
// resource ARN returns the correct error type.
func TestHandler_ErrResourceNotFoundTagging(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSDRequest(t, h, "ListTagsForResource", map[string]any{
		"ResourceARN": "arn:aws:servicediscovery:us-east-1:000000000000:namespace/does-not-exist",
	})
	assert.Equal(t, 400, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// TestHandler_TagResourceNotFound verifies TagResource returns 400 for unknown ARN.
func TestHandler_TagResourceNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSDRequest(t, h, "TagResource", map[string]any{
		"ResourceARN": "arn:aws:servicediscovery:us-east-1:000000000000:service/no-such",
		"Tags":        []map[string]any{{"Key": "k", "Value": "v"}},
	})
	assert.Equal(t, 400, rec.Code)
}

// TestHandler_UntagResourceNotFound verifies UntagResource returns 400 for unknown ARN.
func TestHandler_UntagResourceNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSDRequest(t, h, "UntagResource", map[string]any{
		"ResourceARN": "arn:aws:servicediscovery:us-east-1:000000000000:namespace/no-such",
		"TagKeys":     []string{"k"},
	})
	assert.Equal(t, 400, rec.Code)
}
