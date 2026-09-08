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

// ---- Profile HTTP CRUD ----

func TestHandler_Profile_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createBody map[string]any
		name       string
		wantCreate int
	}{
		{
			name: "create and full lifecycle",
			createBody: map[string]any{
				"name":     "http-profile",
				"roleArns": []string{"arn:aws:iam::123456789012:role/MyRole"},
			},
			wantCreate: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create.
			rec := doREST(t, h, http.MethodPost, "/profiles", tt.createBody)
			assert.Equal(t, tt.wantCreate, rec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
			p := createResp["profile"].(map[string]any)
			id := p["profileId"].(string)
			assert.NotEmpty(t, id)

			// Get.
			recGet := doREST(t, h, http.MethodGet, "/profile/"+id, nil)
			assert.Equal(t, http.StatusOK, recGet.Code)

			// List.
			recList := doREST(t, h, http.MethodGet, "/profiles", nil)
			assert.Equal(t, http.StatusOK, recList.Code)

			// Update.
			dur := int32(3600)
			recUpdate := doREST(t, h, http.MethodPatch, "/profile/"+id, map[string]any{
				"name":            "renamed-profile",
				"durationSeconds": dur,
				"sessionPolicy":   "{}",
			})
			assert.Equal(t, http.StatusOK, recUpdate.Code)

			// Disable / Enable.
			recDisable := doREST(t, h, http.MethodPost, "/profile/"+id+"/disable", nil)
			assert.Equal(t, http.StatusOK, recDisable.Code)

			recEnable := doREST(t, h, http.MethodPost, "/profile/"+id+"/enable", nil)
			assert.Equal(t, http.StatusOK, recEnable.Code)

			// Delete.
			recDelete := doREST(t, h, http.MethodDelete, "/profile/"+id, nil)
			assert.Equal(t, http.StatusOK, recDelete.Code)

			// Get after delete → 404.
			recGetGone := doREST(t, h, http.MethodGet, "/profile/"+id, nil)
			assert.Equal(t, http.StatusNotFound, recGetGone.Code)
		})
	}
}

func TestHandler_Profile_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "get nonexistent profile → 404",
			method:     http.MethodGet,
			path:       "/profile/no-such-id",
			body:       nil,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "delete nonexistent profile → 404",
			method:     http.MethodDelete,
			path:       "/profile/no-such-id",
			body:       nil,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "enable nonexistent profile → 404",
			method:     http.MethodPost,
			path:       "/profile/no-such-id/enable",
			body:       nil,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "disable nonexistent profile → 404",
			method:     http.MethodPost,
			path:       "/profile/no-such-id/disable",
			body:       nil,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "update nonexistent profile → 404",
			method:     http.MethodPatch,
			path:       "/profile/no-such-id",
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

// TestHandler_CreateProfile_RoleArnsRequired proves that CreateProfile
// rejects a request whose body omits roleArns entirely with
// ValidationException, matching real AWS's required CreateProfileInput.RoleArns
// (see TestCreateProfile_RoleArnsRequired in profiles_test.go for the backend-
// level proof and its SDK citations).
func TestHandler_CreateProfile_RoleArnsRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createBody map[string]any
		name       string
		wantStatus int
	}{
		{
			name:       "roleArns key omitted is rejected",
			createBody: map[string]any{"name": "no-role-arns-profile"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "roleArns explicitly empty is accepted",
			createBody: map[string]any{"name": "empty-role-arns-profile", "roleArns": []string{}},
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doREST(t, h, http.MethodPost, "/profiles", tt.createBody)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_Profile_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		method     string
		wantStatus int
	}{
		{"create profile with invalid json", "/profiles", http.MethodPost, http.StatusBadRequest},
		{
			"update profile with invalid json",
			"/profile/some-id",
			http.MethodPatch,
			http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader([]byte(`{invalid`)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			require.NoError(t, h.Handler()(c))
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_Profile_DuplicateNameAllowed proves that creating two profiles
// with the same name succeeds, matching real AWS: CreateProfile only models
// ValidationException and AccessDeniedException (no ConflictException shape
// exists anywhere in the service). A prior version of this test asserted a
// 409 Conflict, which was gopherstack-invented behavior with a fabricated
// error code never returned by the real service.
func TestHandler_Profile_DuplicateNameAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{"duplicate profile name is accepted", http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := map[string]any{"name": "dup-http-profile", "roleArns": []string{}}
			rec1 := doREST(t, h, http.MethodPost, "/profiles", body)
			assert.Equal(t, tt.wantStatus, rec1.Code)

			rec2 := doREST(t, h, http.MethodPost, "/profiles", body)
			assert.Equal(t, tt.wantStatus, rec2.Code)

			var resp1, resp2 map[string]any
			require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &resp1))
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))

			p1 := resp1["profile"].(map[string]any)
			p2 := resp2["profile"].(map[string]any)
			assert.NotEqual(t, p1["profileId"], p2["profileId"])
		})
	}
}

// TestHandler_CreateProfile_EnabledFieldHonored proves that CreateProfile
// honors the request's enabled field instead of hardcoding it to true. Real
// AWS's CreateProfileInput.enabled ("Specifies whether the profile is
// enabled") is an optional field that a prior version of CreateProfile
// silently ignored -- the same ignored-input bug class CreateTrustAnchor had
// before an earlier audit pass fixed it.
func TestHandler_CreateProfile_EnabledFieldHonored(t *testing.T) {
	t.Parallel()

	tests := []struct {
		enabled     any
		name        string
		wantEnabled bool
	}{
		{name: "enabled omitted defaults true", enabled: nil, wantEnabled: true},
		{name: "enabled explicitly true", enabled: true, wantEnabled: true},
		{name: "enabled explicitly false is honored", enabled: false, wantEnabled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := map[string]any{"name": t.Name(), "roleArns": []string{}}

			if tt.enabled != nil {
				body["enabled"] = tt.enabled
			}

			rec := doREST(t, h, http.MethodPost, "/profiles", body)
			require.Equal(t, http.StatusCreated, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			p := resp["profile"].(map[string]any)
			assert.Equal(t, tt.wantEnabled, p["enabled"])
		})
	}
}

// TestHandler_Profile_AcceptRoleSessionName proves that
// acceptRoleSessionName round-trips through Create and Update, matching real
// AWS's CreateProfileInput/UpdateProfileInput/ProfileDetail.
// acceptRoleSessionName field.
func TestHandler_Profile_AcceptRoleSessionName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doREST(t, h, http.MethodPost, "/profiles", map[string]any{
		"name":                  "accept-role-session-profile",
		"roleArns":              []string{},
		"acceptRoleSessionName": true,
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	p := resp["profile"].(map[string]any)
	require.Equal(t, true, p["acceptRoleSessionName"])

	id := p["profileId"].(string)

	recUpdate := doREST(t, h, http.MethodPatch, "/profile/"+id, map[string]any{
		"acceptRoleSessionName": false,
	})
	require.Equal(t, http.StatusOK, recUpdate.Code)

	var updateResp map[string]any
	require.NoError(t, json.Unmarshal(recUpdate.Body.Bytes(), &updateResp))
	updated := updateResp["profile"].(map[string]any)
	assert.NotContains(t, updated, "acceptRoleSessionName",
		"false is the zero value and is omitted, matching requireInstanceProperties' existing convention")
}

// TestHandler_Profile_CreatedBy proves that Create populates createdBy with
// the backend's account ID, matching real AWS's ProfileDetail.createdBy
// ("The Amazon Web Services account that created the profile").
func TestHandler_Profile_CreatedBy(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doREST(t, h, http.MethodPost, "/profiles", map[string]any{
		"name":     "created-by-profile",
		"roleArns": []string{},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	p := resp["profile"].(map[string]any)
	assert.Equal(t, "000000000000", p["createdBy"])
}

// TestHandler_Profile_NoTagsFieldOnResponse proves that Create never emits a
// "tags" key on the profile response -- real AWS's ProfileDetail carries no
// tags field at all (tags are visible only via ListTagsForResource). A prior
// version of profileToJSON invented one.
func TestHandler_Profile_NoTagsFieldOnResponse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doREST(t, h, http.MethodPost, "/profiles", map[string]any{
		"name":     "no-tags-field-profile",
		"roleArns": []string{},
		"tags":     []map[string]any{{"key": "env", "value": "prod"}},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	p := resp["profile"].(map[string]any)
	assert.NotContains(t, p, "tags")
}

// TestHandler_ListProfiles_PageSizeQueryParam proves ListProfiles honors
// "pageSize", the real AWS wire query param every List operation's
// serializer emits (see
// aws-sdk-go-v2/service/rolesanywhere/serializers.go's
// awsRestjson1_serializeOpHttpBindingsListProfilesInput, which calls
// encoder.SetQuery("pageSize")) -- not the fictional "maxResults" this
// handler previously looked for instead.
func TestHandler_ListProfiles_PageSizeQueryParam(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 3 {
		doREST(t, h, http.MethodPost, "/profiles", map[string]any{
			"name":     "page-profile-" + string(rune('a'+i)),
			"roleArns": []string{"arn:aws:iam::123456789012:role/MyRole"},
		})
	}

	rec := doREST(t, h, http.MethodGet, "/profiles?pageSize=1", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items, _ := resp["profiles"].([]any)
	assert.Len(t, items, 1, "pageSize=1 must truncate the page to 1 item")
}
