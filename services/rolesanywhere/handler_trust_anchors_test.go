package rolesanywhere_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- Trust Anchor HTTP CRUD ----

func TestHandler_TrustAnchor_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createBody map[string]any
		name       string
		wantCreate int
		wantGet    int
	}{
		{
			name: "create and get trust anchor",
			createBody: map[string]any{
				"name":   "anchor-http-test",
				"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
			},
			wantCreate: http.StatusCreated,
			wantGet:    http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create.
			rec := doREST(t, h, http.MethodPost, "/trustanchors", tt.createBody)
			assert.Equal(t, tt.wantCreate, rec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
			ta := createResp["trustAnchor"].(map[string]any)
			id := ta["trustAnchorId"].(string)
			assert.NotEmpty(t, id)

			// Get.
			recGet := doREST(t, h, http.MethodGet, "/trustanchor/"+id, nil)
			assert.Equal(t, tt.wantGet, recGet.Code)

			// List.
			recList := doREST(t, h, http.MethodGet, "/trustanchors", nil)
			assert.Equal(t, http.StatusOK, recList.Code)

			// Update.
			recUpdate := doREST(
				t,
				h,
				http.MethodPatch,
				"/trustanchor/"+id,
				map[string]any{"name": "renamed-anchor"},
			)
			assert.Equal(t, http.StatusOK, recUpdate.Code)

			// Enable / Disable.
			recDisable := doREST(t, h, http.MethodPost, "/trustanchor/"+id+"/disable", nil)
			assert.Equal(t, http.StatusOK, recDisable.Code)

			recEnable := doREST(t, h, http.MethodPost, "/trustanchor/"+id+"/enable", nil)
			assert.Equal(t, http.StatusOK, recEnable.Code)

			// Delete.
			recDelete := doREST(t, h, http.MethodDelete, "/trustanchor/"+id, nil)
			assert.Equal(t, http.StatusOK, recDelete.Code)

			// Get after delete → 404.
			recGetGone := doREST(t, h, http.MethodGet, "/trustanchor/"+id, nil)
			assert.Equal(t, http.StatusNotFound, recGetGone.Code)
		})
	}
}

func TestHandler_TrustAnchor_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		method     string
		raw        []byte
		wantStatus int
	}{
		{
			name:       "create trust anchor with invalid json",
			path:       "/trustanchors",
			method:     http.MethodPost,
			raw:        []byte(`{invalid`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "update trust anchor with invalid json",
			path:       "/trustanchor/some-id",
			method:     http.MethodPatch,
			raw:        []byte(`{invalid`),
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader(tt.raw))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			require.NoError(t, h.Handler()(c))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_TrustAnchor_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "get nonexistent → 404",
			method:     http.MethodGet,
			path:       "/trustanchor/no-such-id",
			body:       nil,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "delete nonexistent → 404",
			method:     http.MethodDelete,
			path:       "/trustanchor/no-such-id",
			body:       nil,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "enable nonexistent → 404",
			method:     http.MethodPost,
			path:       "/trustanchor/no-such-id/enable",
			body:       nil,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "disable nonexistent → 404",
			method:     http.MethodPost,
			path:       "/trustanchor/no-such-id/disable",
			body:       nil,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "update nonexistent → 404",
			method:     http.MethodPatch,
			path:       "/trustanchor/no-such-id",
			body:       map[string]any{"name": "x"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doREST(t, h, tt.method, tt.path, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_TrustAnchor_DuplicateNameAllowed proves that creating two
// trust anchors with the same name succeeds, matching real AWS: Roles
// Anywhere's CreateTrustAnchor only models ValidationException and
// AccessDeniedException (no ConflictException shape exists anywhere in the
// service), and the two resulting resources are distinguished by their
// generated IDs/ARNs, not by name. A prior version of this test asserted a
// 409 Conflict, which was gopherstack-invented behavior with a fabricated
// error code never returned by the real service.
func TestHandler_TrustAnchor_DuplicateNameAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{"duplicate name is accepted", http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := map[string]any{
				"name":   "dup-anchor-http",
				"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
			}
			rec1 := doREST(t, h, http.MethodPost, "/trustanchors", body)
			assert.Equal(t, tt.wantStatus, rec1.Code)

			rec2 := doREST(t, h, http.MethodPost, "/trustanchors", body)
			assert.Equal(t, tt.wantStatus, rec2.Code)

			var resp1, resp2 map[string]any
			require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &resp1))
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))

			ta1 := resp1["trustAnchor"].(map[string]any)
			ta2 := resp2["trustAnchor"].(map[string]any)
			assert.NotEqual(t, ta1["trustAnchorId"], ta2["trustAnchorId"])
		})
	}
}

// TestHandler_TrustAnchor_NotFoundOnTagsField proves that Create/Get never
// emit a "tags" key on the trust anchor response -- real AWS's
// TrustAnchorDetail carries no tags field at all (tags are visible only via
// ListTagsForResource). A prior version of trustAnchorToJSON invented one.
func TestHandler_TrustAnchor_NoTagsFieldOnResponse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doREST(t, h, http.MethodPost, "/trustanchors", map[string]any{
		"name":   "no-tags-field-anchor",
		"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
		"tags":   []map[string]any{{"key": "env", "value": "prod"}},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ta := resp["trustAnchor"].(map[string]any)
	assert.NotContains(t, ta, "tags")

	// The creation-time tag must still be visible via ListTagsForResource.
	arn := ta["trustAnchorArn"].(string)
	recTags := doREST(t, h, http.MethodGet, "/ListTagsForResource?resourceArn="+arn, nil)
	require.Equal(t, http.StatusOK, recTags.Code)

	var tagsResp map[string]any
	require.NoError(t, json.Unmarshal(recTags.Body.Bytes(), &tagsResp))
	tags := tagsResp["tags"].([]any)
	require.Len(t, tags, 1)
	tag := tags[0].(map[string]any)
	assert.Equal(t, "env", tag["key"])
	assert.Equal(t, "prod", tag["value"])
}

// TestHandler_CreateTrustAnchor_NotificationSettings proves that
// notificationSettings passed to CreateTrustAnchor are applied at creation
// time and visible on the create response, matching real AWS's
// CreateTrustAnchorInput.notificationSettings.
func TestHandler_CreateTrustAnchor_NotificationSettings(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doREST(t, h, http.MethodPost, "/trustanchors", map[string]any{
		"name":   "notif-at-create-http",
		"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
		"notificationSettings": []map[string]any{
			{"event": "CA_CERTIFICATE_EXPIRY", "enabled": true},
		},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ta := resp["trustAnchor"].(map[string]any)

	settings, ok := ta["notificationSettings"].([]any)
	require.True(t, ok, "expected notificationSettings on create response: %v", ta)
	require.Len(t, settings, 1)

	setting := settings[0].(map[string]any)
	assert.Equal(t, "CA_CERTIFICATE_EXPIRY", setting["event"])
}

// ---- List TrustAnchors with pagination ----

func TestHandler_ListTrustAnchors_Pagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantCount  int
	}{
		{"no pagination returns all", "", http.StatusOK, 3},
		{"pageSize=1 returns 1", "?pageSize=1", http.StatusOK, 1},
		{"pageSize=2 returns 2", "?pageSize=2", http.StatusOK, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			for i := range 3 {
				doREST(t, h, http.MethodPost, "/trustanchors", map[string]any{
					"name":   "anchor-page-" + string(rune('a'+i)),
					"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
				})
			}
			rec := doREST(t, h, http.MethodGet, "/trustanchors"+tt.query, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			items := resp["trustAnchors"].([]any)
			assert.Len(t, items, tt.wantCount)
		})
	}
}
