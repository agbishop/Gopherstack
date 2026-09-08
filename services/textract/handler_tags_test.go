package textract_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/textract"
)

// TestHandler_TagResource_HappyPath tests tagging and listing tags on an adapter.
func TestHandler_TagResource_HappyPath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "taggable-adapter",
		"FeatureTypes": []string{"FORMS"},
	})
	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	adapterID := createResp["AdapterId"]

	tagRec := doTextractRequest(t, h, "TagResource", map[string]any{
		"ResourceARN": adapterID,
		"Tags":        map[string]string{"env": "prod", "team": "ml"},
	})
	require.Equal(t, http.StatusOK, tagRec.Code)

	listRec := doTextractRequest(t, h, "ListTagsForResource", map[string]any{
		"ResourceARN": adapterID,
	})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	tags, ok := listResp["Tags"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "prod", tags["env"])
	assert.Equal(t, "ml", tags["team"])
}

// TestHandler_UntagResource_HappyPath tests removing tags from an adapter.
func TestHandler_UntagResource_HappyPath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "tagged-adapter",
		"FeatureTypes": []string{"FORMS"},
		"Tags":         map[string]string{"env": "dev", "team": "ml"},
	})
	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	adapterID := createResp["AdapterId"]

	untagRec := doTextractRequest(t, h, "UntagResource", map[string]any{
		"ResourceARN": adapterID,
		"TagKeys":     []string{"env"},
	})
	require.Equal(t, http.StatusOK, untagRec.Code)

	listRec := doTextractRequest(t, h, "ListTagsForResource", map[string]any{
		"ResourceARN": adapterID,
	})
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	tags, _ := listResp["Tags"].(map[string]any)
	_, hasEnv := tags["env"]
	assert.False(t, hasEnv)
	assert.Equal(t, "ml", tags["team"])
}

// TestHandler_TagResource_NotFound returns 400 ResourceNotFoundException for
// an unknown resource. Real AWS's deserializeOpErrorTagResource switch
// declares ResourceNotFoundException (not InvalidParameterException) for
// this case -- verified against aws-sdk-go-v2/service/textract
// deserializers.go, and the same holds for UntagResource/ListTagsForResource.
func TestHandler_TagResource_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTextractRequest(t, h, "TagResource", map[string]any{
		"ResourceARN": "nonexistent-arn",
		"Tags":        map[string]string{"k": "v"},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ResourceNotFoundException", errResp["__type"])
}

// TestHandler_UntagResourceAndListTagsForResource_NotFound verifies the same
// ResourceNotFoundException error code applies to UntagResource and
// ListTagsForResource for an unknown resource ARN.
func TestHandler_UntagResourceAndListTagsForResource_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "UntagResource",
			action: "UntagResource",
			body:   map[string]any{"ResourceARN": "nonexistent-arn", "TagKeys": []string{"k"}},
		},
		{
			name:   "ListTagsForResource",
			action: "ListTagsForResource",
			body:   map[string]any{"ResourceARN": "nonexistent-arn"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doTextractRequest(t, h, tt.action, tt.body)
			require.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, "ResourceNotFoundException", errResp["__type"])
		})
	}
}

// TestHandler_TagResource_ByARNRoundTrip verifies TagResource + ListTagsForResource
// round-trip using a real adapter ARN (built via BuildAdapterARN).
func TestHandler_TagResource_ByARNRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := h.Backend.(*textract.InMemoryBackend)

	createRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "arn-tag-adapter",
		"FeatureTypes": []string{"FORMS"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	adapterID := createResp["AdapterId"]

	// Build an ARN using the backend helper.
	arn := b.BuildAdapterARN(adapterID)
	assert.Contains(t, arn, adapterID)
	assert.Contains(t, arn, "arn:aws:textract:")

	// Tag via ARN.
	tagRec := doTextractRequest(t, h, "TagResource", map[string]any{
		"ResourceARN": arn,
		"Tags":        map[string]string{"env": "prod", "purpose": "accuracy-test"},
	})
	require.Equal(t, http.StatusOK, tagRec.Code)

	// List via ARN.
	listRec := doTextractRequest(t, h, "ListTagsForResource", map[string]any{
		"ResourceARN": arn,
	})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	tags, ok := listResp["Tags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "prod", tags["env"])
	assert.Equal(t, "accuracy-test", tags["purpose"])
}

// TestHandler_TagResource_AdapterVersionARN verifies tagging works via an
// adapter version ARN (built via BuildAdapterVersionARN).
func TestHandler_TagResource_AdapterVersionARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := h.Backend.(*textract.InMemoryBackend)

	createAdapterRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "version-tag-adapter",
		"FeatureTypes": []string{"QUERIES"},
	})
	require.Equal(t, http.StatusOK, createAdapterRec.Code)

	var createAdapterResp map[string]string
	require.NoError(t, json.Unmarshal(createAdapterRec.Body.Bytes(), &createAdapterResp))
	adapterID := createAdapterResp["AdapterId"]

	createVersionRec := doTextractRequest(t, h, "CreateAdapterVersion", map[string]any{
		"AdapterId": adapterID,
		"DatasetConfig": map[string]any{
			"ManifestS3Object": map[string]any{
				"Bucket": "test-dataset-bucket",
				"Name":   "manifest.json",
			},
		},
		"OutputConfig": map[string]any{
			"S3Bucket": "test-output-bucket",
		},
	})
	require.Equal(t, http.StatusOK, createVersionRec.Code)

	var createVersionResp map[string]string
	require.NoError(t, json.Unmarshal(createVersionRec.Body.Bytes(), &createVersionResp))
	adapterVersion := createVersionResp["AdapterVersion"]

	versionARN := b.BuildAdapterVersionARN(adapterID, adapterVersion)
	assert.Contains(t, versionARN, "version/")

	tagRec := doTextractRequest(t, h, "TagResource", map[string]any{
		"ResourceARN": versionARN,
		"Tags":        map[string]string{"version-tag": "v1"},
	})
	require.Equal(t, http.StatusOK, tagRec.Code)

	listRec := doTextractRequest(t, h, "ListTagsForResource", map[string]any{
		"ResourceARN": versionARN,
	})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	tags, ok := listResp["Tags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "v1", tags["version-tag"])
}
