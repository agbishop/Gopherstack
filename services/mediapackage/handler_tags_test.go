package mediapackage_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	channelID := createTestChannel(t, h)

	// Get ARN
	rec := doRequest(t, h, http.MethodGet, "/channels/"+channelID, nil)
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	resourceARN := descResp["arn"].(string)

	// TagResource
	rec = doRequest(t, h, http.MethodPost, "/tags/"+resourceARN, map[string]any{
		"tags": map[string]any{"env": "prod", "team": "platform"},
	})
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// ListTagsForResource
	rec = doRequest(t, h, http.MethodGet, "/tags/"+resourceARN, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	tags := listResp["tags"].(map[string]any)
	assert.Equal(t, "prod", tags["env"])
	assert.Equal(t, "platform", tags["team"])

	// UntagResource
	req := httptest.NewRequest(http.MethodDelete, "/tags/"+resourceARN+"?tagKeys=env", nil)
	rec2 := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec2)
	require.NoError(t, h.Handler()(c))
	assert.Equal(t, http.StatusNoContent, rec2.Code)

	// Verify tag removed
	rec = doRequest(t, h, http.MethodGet, "/tags/"+resourceARN, nil)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	tags = listResp["tags"].(map[string]any)
	assert.NotContains(t, tags, "env")
	assert.Equal(t, "platform", tags["team"])
}

// TestTags_CreatedAtChannelCreation verifies that tags supplied at channel
// creation are returned by ListTagsForResource (not just DescribeChannel).
func TestTags_CreatedAtChannelCreation(t *testing.T) {
	t.Parallel()

	tests := []struct { //nolint:govet // test-only struct; layout not performance-critical
		creationTags map[string]any
		name         string
		wantTags     map[string]string
	}{
		{
			name:         "tags at creation visible via ListTagsForResource",
			creationTags: map[string]any{"env": "prod", "team": "video"},
			wantTags:     map[string]string{"env": "prod", "team": "video"},
		},
		{
			name:         "no tags at creation returns empty map",
			creationTags: nil,
			wantTags:     map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, arn := createChannelWithTags(t, h, "ch-tag-create", tc.creationTags)

			code, resp := doRequestJSON(t, h, http.MethodGet, "/tags/"+arn, nil)
			require.Equal(t, http.StatusOK, code)

			tags, ok := resp["tags"].(map[string]any)
			require.True(t, ok)

			got := make(map[string]string, len(tags))
			for k, v := range tags {
				got[k] = v.(string)
			}

			assert.Equal(t, tc.wantTags, got)
		})
	}
}

// TestTags_TagResourceReflectsInDescribeChannel verifies that tags added
// via TagResource appear in subsequent DescribeChannel responses.
func TestTags_TagResourceReflectsInDescribeChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addTags map[string]any
		name    string
		wantKey string
		wantVal string
	}{
		{
			name:    "TagResource tag visible in DescribeChannel",
			addTags: map[string]any{"env": "staging"},
			wantKey: "env",
			wantVal: "staging",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, arn := createChannelWithTags(t, h, "ch-tag-describe", nil)

			// Add tag via TagResource
			code, _ := doRequestJSON(t, h, http.MethodPost, "/tags/"+arn, map[string]any{"tags": tc.addTags})
			require.Equal(t, http.StatusNoContent, code)

			// Check DescribeChannel shows the tag
			code, resp := doRequestJSON(t, h, http.MethodGet, "/channels/ch-tag-describe", nil)
			require.Equal(t, http.StatusOK, code)

			tags, ok := resp["tags"].(map[string]any)
			require.True(t, ok, "tags field should be present in DescribeChannel")
			assert.Equal(t, tc.wantVal, tags[tc.wantKey])
		})
	}
}

// TestTags_UntagResourceReflectsInDescribeChannel verifies that tags removed
// via UntagResource no longer appear in DescribeChannel.
func TestTags_UntagResourceReflectsInDescribeChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		initialTags map[string]any
		removeKey   string
		keepKey     string
	}{
		{
			name:        "UntagResource removes tag from DescribeChannel",
			initialTags: map[string]any{"env": "prod", "team": "video"},
			removeKey:   "env",
			keepKey:     "team",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, arn := createChannelWithTags(t, h, "ch-untag-describe", tc.initialTags)

			// Remove one tag
			code, _ := doRequestJSON(t, h, http.MethodDelete, "/tags/"+arn+"?tagKeys="+tc.removeKey, nil)
			require.Equal(t, http.StatusNoContent, code)

			// Check DescribeChannel no longer shows removed tag
			code, resp := doRequestJSON(t, h, http.MethodGet, "/channels/ch-untag-describe", nil)
			require.Equal(t, http.StatusOK, code)

			tags, ok := resp["tags"].(map[string]any)
			require.True(t, ok)
			assert.NotContains(t, tags, tc.removeKey, "removed tag should not appear in DescribeChannel")
			assert.Contains(t, tags, tc.keepKey, "retained tag should still appear in DescribeChannel")
		})
	}
}

// TestTags_OriginEndpointCreationTags verifies that tags supplied at
// origin endpoint creation are returned by ListTagsForResource.
func TestTags_OriginEndpointCreationTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		creationTags map[string]any
		name         string
		wantCount    int
	}{
		{
			name:         "endpoint creation tags visible via ListTagsForResource",
			creationTags: map[string]any{"tier": "premium"},
			wantCount:    1,
		},
		{
			name:         "no endpoint creation tags returns empty",
			creationTags: nil,
			wantCount:    0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			chID, _ := createChannelWithTags(t, h, "ch-ep-tags", nil)
			epARN := createEndpointWithTags(t, h, chID, "ep-tags", tc.creationTags)

			code, resp := doRequestJSON(t, h, http.MethodGet, "/tags/"+epARN, nil)
			require.Equal(t, http.StatusOK, code)

			tags, ok := resp["tags"].(map[string]any)
			require.True(t, ok)
			assert.Len(t, tags, tc.wantCount)
		})
	}
}

// TestTags_TagResourceTargetsCorrectResourceAmongMany verifies that
// TagResource/UntagResource sync the resource-level Tags field of the
// specific channel or origin endpoint the ARN names, not a sibling with a
// different ID -- the invariant findChannelByARN/findOriginEndpointByARN
// must preserve now that they derive the target ID directly from the ARN's
// "<resourceType>/<id>" segment instead of scanning every row.
func TestTags_TagResourceWithColonInResourceID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, arn := createChannelWithTags(t, h, "ch:with:colons", nil)

	code, _ := doRequestJSON(t, h, http.MethodPost, "/tags/"+arn, map[string]any{
		"tags": map[string]any{"owner": "team-a"},
	})
	require.Equal(t, http.StatusNoContent, code)

	_, resp := doRequestJSON(t, h, http.MethodGet, "/channels/ch:with:colons", nil)
	tags, _ := resp["tags"].(map[string]any)
	assert.Equal(t, "team-a", tags["owner"])
}

func TestTags_TagResourceTargetsCorrectResourceAmongMany(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, arnA := createChannelWithTags(t, h, "ch-a", nil)
	chB, _ := createChannelWithTags(t, h, "ch-b", nil)
	epA := createEndpointWithTags(t, h, "ch-a", "ep-a", nil)
	createEndpointWithTags(t, h, chB, "ep-b", nil)

	code, _ := doRequestJSON(t, h, http.MethodPost, "/tags/"+arnA, map[string]any{
		"tags": map[string]any{"owner": "team-a"},
	})
	require.Equal(t, http.StatusNoContent, code)

	code, _ = doRequestJSON(t, h, http.MethodPost, "/tags/"+epA, map[string]any{
		"tags": map[string]any{"owner": "team-a"},
	})
	require.Equal(t, http.StatusNoContent, code)

	_, respA := doRequestJSON(t, h, http.MethodGet, "/channels/ch-a", nil)
	tagsA, _ := respA["tags"].(map[string]any)
	assert.Equal(t, "team-a", tagsA["owner"])

	_, respB := doRequestJSON(t, h, http.MethodGet, "/channels/ch-b", nil)
	tagsB, _ := respB["tags"].(map[string]any)
	assert.NotContains(t, tagsB, "owner", "tagging ch-a must not sync onto ch-b")

	_, respEpA := doRequestJSON(t, h, http.MethodGet, "/origin_endpoints/ep-a", nil)
	epTagsA, _ := respEpA["tags"].(map[string]any)
	assert.Equal(t, "team-a", epTagsA["owner"])

	_, respEpB := doRequestJSON(t, h, http.MethodGet, "/origin_endpoints/ep-b", nil)
	epTagsB, _ := respEpB["tags"].(map[string]any)
	assert.NotContains(t, epTagsB, "owner", "tagging ep-a must not sync onto ep-b")
}

// TestTags_ListChannelsIncludesTags verifies that ListChannels response
// includes tags set at creation time.
func TestTags_ListChannelsIncludesTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantTag string
		wantVal string
	}{
		{
			name:    "list channels response includes creation tags",
			wantTag: "env",
			wantVal: "prod",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createChannelWithTags(t, h, "ch-list-tags", map[string]any{"env": "prod"})

			code, resp := doRequestJSON(t, h, http.MethodGet, "/channels", nil)
			require.Equal(t, http.StatusOK, code)

			channels, ok := resp["channels"].([]any)
			require.True(t, ok)
			require.Len(t, channels, 1)

			ch := channels[0].(map[string]any)
			tags, ok := ch["tags"].(map[string]any)
			require.True(t, ok, "tags should be present in ListChannels entry")
			assert.Equal(t, tc.wantVal, tags[tc.wantTag])
		})
	}
}
