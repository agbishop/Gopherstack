package rolesanywhere_test

// handler_test.go covers Handler metadata/routing plumbing plus wire-shape
// parity fixes made to handler.go:
//   - CreateTrustAnchor honoring the request's enabled field
//   - DeleteTrustAnchor/DeleteProfile returning the deleted resource
//   - UntagResource reading resourceArn/tagKeys from the JSON body (matching
//     the real aws-sdk-go-v2 wire shape) instead of query parameters
//   - notificationSettings/attributeMappings surfacing on every trust
//     anchor/profile read, not only the dedicated Put/Reset/Delete-mapping ops

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rolesanywhere"
)

// ---- test helpers ----

func newTestHandler(t *testing.T) *rolesanywhere.Handler {
	t.Helper()
	b := rolesanywhere.NewInMemoryBackend("000000000000", "us-east-1")

	return rolesanywhere.NewHandler(b)
}

// doREST sends an HTTP request to the handler and returns the response recorder.
func doREST(
	t *testing.T,
	h *rolesanywhere.Handler,
	method, path string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}

	e := echo.New()
	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

// ---- Handler metadata ----

func TestHandler_Name(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, "RolesAnywhere", h.Name())
}

func TestHandler_MatchPriority(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Positive(t, h.MatchPriority())
}

func TestHandler_Reset(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	// create a trust anchor via the handler, then reset
	rec := doREST(t, h, http.MethodPost, "/trustanchors", map[string]any{
		"name":   "reset-test",
		"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
	})
	assert.Equal(t, http.StatusCreated, rec.Code)

	h.Reset()

	// after reset, list should return empty
	rec2 := doREST(t, h, http.MethodGet, "/trustanchors", nil)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
	items, _ := resp["trustAnchors"].([]any)
	assert.Empty(t, items)
}

func TestHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.GetSupportedOperations()
	assert.NotEmpty(t, ops)
	assert.Contains(t, ops, "CreateTrustAnchor")
	assert.Contains(t, ops, "ImportCrl")
	assert.Contains(t, ops, "TagResource")
}

// ---- RouteMatcher ----

func TestHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		path      string
		wantMatch bool
	}{
		{"trustanchors collection", "/trustanchors", true},
		{"trustanchor by id", "/trustanchor/some-id", true},
		{"profiles collection", "/profiles", true},
		{"profile by id", "/profile/some-id", true},
		{"crls collection", "/crls", true},
		{"crl by id", "/crl/some-id", true},
		{"subjects collection", "/subjects", true},
		{"subject by id", "/subject/some-id", true},
		{"put-notifications-settings", "/put-notifications-settings", true},
		{"reset-notifications-settings", "/reset-notifications-settings", true},
		{"TagResource", "/TagResource", true},
		{"UntagResource", "/UntagResource", true},
		{"ListTagsForResource", "/ListTagsForResource", true},
		{"unknown path", "/something-else", false},
		{"ec2 path", "/instances", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			matcher := h.RouteMatcher()
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			got := matcher(c)
			assert.Equal(t, tt.wantMatch, got)
		})
	}
}

// ---- ExtractOperation / ExtractResource ----

func TestHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		wantOp string
	}{
		{
			"GET /trustanchors → ListTrustAnchors",
			http.MethodGet,
			"/trustanchors",
			"ListTrustAnchors",
		},
		{
			"POST /trustanchors → CreateTrustAnchor",
			http.MethodPost,
			"/trustanchors",
			"CreateTrustAnchor",
		},
		{
			"GET /trustanchor/id → GetTrustAnchor",
			http.MethodGet,
			"/trustanchor/abc",
			"GetTrustAnchor",
		},
		{
			"DELETE /trustanchor/id → DeleteTrustAnchor",
			http.MethodDelete,
			"/trustanchor/abc",
			"DeleteTrustAnchor",
		},
		{
			"PATCH /trustanchor/id → UpdateTrustAnchor",
			http.MethodPatch,
			"/trustanchor/abc",
			"UpdateTrustAnchor",
		},
		{
			"POST /trustanchor/id/enable → EnableTrustAnchor",
			http.MethodPost,
			"/trustanchor/abc/enable",
			"EnableTrustAnchor",
		},
		{
			"POST /trustanchor/id/disable → DisableTrustAnchor",
			http.MethodPost,
			"/trustanchor/abc/disable",
			"DisableTrustAnchor",
		},
		{"GET /profiles → ListProfiles", http.MethodGet, "/profiles", "ListProfiles"},
		{"POST /profiles → CreateProfile", http.MethodPost, "/profiles", "CreateProfile"},
		{"GET /profile/id → GetProfile", http.MethodGet, "/profile/abc", "GetProfile"},
		{"DELETE /profile/id → DeleteProfile", http.MethodDelete, "/profile/abc", "DeleteProfile"},
		{"PATCH /profile/id → UpdateProfile", http.MethodPatch, "/profile/abc", "UpdateProfile"},
		{
			"POST /profile/id/enable → EnableProfile",
			http.MethodPost,
			"/profile/abc/enable",
			"EnableProfile",
		},
		{
			"POST /profile/id/disable → DisableProfile",
			http.MethodPost,
			"/profile/abc/disable",
			"DisableProfile",
		},
		{"GET /crls → ListCrls", http.MethodGet, "/crls", "ListCrls"},
		{"POST /crls → ImportCrl", http.MethodPost, "/crls", "ImportCrl"},
		{"GET /crl/id → GetCrl", http.MethodGet, "/crl/abc", "GetCrl"},
		{"DELETE /crl/id → DeleteCrl", http.MethodDelete, "/crl/abc", "DeleteCrl"},
		{"PATCH /crl/id → UpdateCrl", http.MethodPatch, "/crl/abc", "UpdateCrl"},
		{"POST /crl/id/enable → EnableCrl", http.MethodPost, "/crl/abc/enable", "EnableCrl"},
		{"POST /crl/id/disable → DisableCrl", http.MethodPost, "/crl/abc/disable", "DisableCrl"},
		{"GET /subjects → ListSubjects", http.MethodGet, "/subjects", "ListSubjects"},
		{"GET /subject/id → GetSubject", http.MethodGet, "/subject/abc", "GetSubject"},
		{
			"PUT /profiles/id/mappings → PutAttributeMapping",
			http.MethodPut,
			"/profiles/abc/mappings",
			"PutAttributeMapping",
		},
		{
			"DELETE /profiles/id/mappings → DeleteAttributeMapping",
			http.MethodDelete,
			"/profiles/abc/mappings",
			"DeleteAttributeMapping",
		},
		{
			"PATCH /put-notifications-settings → PutNotificationSettings",
			http.MethodPatch,
			"/put-notifications-settings",
			"PutNotificationSettings",
		},
		{
			"PATCH /reset-notifications-settings → ResetNotificationSettings",
			http.MethodPatch,
			"/reset-notifications-settings",
			"ResetNotificationSettings",
		},
		{"POST /TagResource → TagResource", http.MethodPost, "/TagResource", "TagResource"},
		{"POST /UntagResource → UntagResource", http.MethodPost, "/UntagResource", "UntagResource"},
		{
			"GET /ListTagsForResource → ListTagsForResource",
			http.MethodGet,
			"/ListTagsForResource",
			"ListTagsForResource",
		},
		{"unknown path → Unknown", http.MethodGet, "/unknown-path", "Unknown"},
		{"wrong method on TagResource → Unknown", http.MethodGet, "/TagResource", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			op := h.ExtractOperation(c)
			assert.Equal(t, tt.wantOp, op)
		})
	}
}

func TestHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		method       string
		path         string
		wantResource string
	}{
		{"trustanchor by id returns id", http.MethodGet, "/trustanchor/my-id", "my-id"},
		{"trustanchors collection returns empty", http.MethodGet, "/trustanchors", ""},
		{"profile by id returns id", http.MethodGet, "/profile/p-id", "p-id"},
		{"crl by id returns id", http.MethodGet, "/crl/c-id", "c-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			e := echo.New()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			res := h.ExtractResource(c)
			assert.Equal(t, tt.wantResource, res)
		})
	}
}

// ---- Unknown operation → 404 ----

func TestHandler_UnknownOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"unknown path returns 404", http.MethodGet, "/unknown-path-xyz", http.StatusNotFound},
		{"wrong method on collection", http.MethodDelete, "/trustanchors", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doREST(t, h, tt.method, tt.path, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestParsePageParams_InvalidPageSize verifies a non-numeric pageSize
// query param yields a ValidationException rather than silently coercing to 0.
func TestParsePageParams_InvalidPageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{name: "valid_numeric", query: "?pageSize=2", wantStatus: http.StatusOK},
		{name: "non_numeric", query: "?pageSize=abc", wantStatus: http.StatusBadRequest},
		{name: "mixed", query: "?pageSize=1a2", wantStatus: http.StatusBadRequest},
		{name: "empty_ignored", query: "?pageSize=", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doREST(t, h, http.MethodGet, "/trustanchors"+tt.query, nil)
			assert.Equal(t, tt.wantStatus, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

func TestHandler_CreateTrustAnchor_EnabledFalse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body        map[string]any
		name        string
		wantEnabled bool
	}{
		{
			name: "enabled false is honored",
			body: map[string]any{
				"name":    "disabled-anchor",
				"source":  map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
				"enabled": false,
			},
			wantEnabled: false,
		},
		{
			name: "enabled true is honored",
			body: map[string]any{
				"name":    "enabled-anchor",
				"source":  map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
				"enabled": true,
			},
			wantEnabled: true,
		},
		{
			name: "omitted enabled defaults to true",
			body: map[string]any{
				"name":   "default-anchor",
				"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
			},
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doREST(t, h, http.MethodPost, "/trustanchors", tt.body)
			require.Equal(t, http.StatusCreated, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			ta := resp["trustAnchor"].(map[string]any)
			assert.Equal(t, tt.wantEnabled, ta["enabled"])
		})
	}
}

func TestHandler_DeleteTrustAnchor_ReturnsResource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	recCreate := doREST(t, h, http.MethodPost, "/trustanchors", map[string]any{
		"name":   "del-return-anchor",
		"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
	})
	require.Equal(t, http.StatusCreated, recCreate.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &createResp))
	id := createResp["trustAnchor"].(map[string]any)["trustAnchorId"].(string)

	recDelete := doREST(t, h, http.MethodDelete, "/trustanchor/"+id, nil)
	require.Equal(t, http.StatusOK, recDelete.Code)

	var deleteResp map[string]any
	require.NoError(t, json.Unmarshal(recDelete.Body.Bytes(), &deleteResp))
	ta, ok := deleteResp["trustAnchor"].(map[string]any)
	require.True(t, ok,
		"DeleteTrustAnchor response must carry the deleted trustAnchor, got: %s", recDelete.Body.String())
	assert.Equal(t, id, ta["trustAnchorId"])
}

func TestHandler_DeleteProfile_ReturnsResource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	recCreate := doREST(t, h, http.MethodPost, "/profiles", map[string]any{
		"name":     "del-return-profile",
		"roleArns": []string{"arn:aws:iam::000000000000:role/Test"},
	})
	require.Equal(t, http.StatusCreated, recCreate.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &createResp))
	id := createResp["profile"].(map[string]any)["profileId"].(string)

	recDelete := doREST(t, h, http.MethodDelete, "/profile/"+id, nil)
	require.Equal(t, http.StatusOK, recDelete.Code)

	var deleteResp map[string]any
	require.NoError(t, json.Unmarshal(recDelete.Body.Bytes(), &deleteResp))
	p, ok := deleteResp["profile"].(map[string]any)
	require.True(t, ok, "DeleteProfile response must carry the deleted profile, got: %s", recDelete.Body.String())
	assert.Equal(t, id, p["profileId"])
}

func TestHandler_UntagResource_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/UntagResource", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	require.NoError(t, h.Handler()(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_GetTrustAnchor_ShowsNotificationSettings(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	recCreate := doREST(t, h, http.MethodPost, "/trustanchors", map[string]any{
		"name":   "notif-visible-anchor",
		"source": map[string]any{"sourceType": "CERTIFICATE_BUNDLE"},
	})
	require.Equal(t, http.StatusCreated, recCreate.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &createResp))
	id := createResp["trustAnchor"].(map[string]any)["trustAnchorId"].(string)

	recPut := doREST(t, h, http.MethodPatch, "/put-notifications-settings", map[string]any{
		"trustAnchorId": id,
		"notificationSettings": []map[string]any{
			{"event": "CA_CERTIFICATE_EXPIRY", "channel": "ALL", "enabled": true, "threshold": 45},
		},
	})
	require.Equal(t, http.StatusOK, recPut.Code)

	// GetTrustAnchor (not just the PutNotificationSettings response itself)
	// must also surface the settings, since AWS's TrustAnchorDetail carries
	// notificationSettings on every read.
	recGet := doREST(t, h, http.MethodGet, "/trustanchor/"+id, nil)
	require.Equal(t, http.StatusOK, recGet.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(recGet.Body.Bytes(), &getResp))
	ta := getResp["trustAnchor"].(map[string]any)
	settings, ok := ta["notificationSettings"].([]any)
	require.True(t, ok, "GetTrustAnchor response must carry notificationSettings, got: %s", recGet.Body.String())
	require.Len(t, settings, 1)

	// ListTrustAnchors must show it too.
	recList := doREST(t, h, http.MethodGet, "/trustanchors", nil)
	require.Equal(t, http.StatusOK, recList.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(recList.Body.Bytes(), &listResp))
	items := listResp["trustAnchors"].([]any)
	require.Len(t, items, 1)
	listedTA := items[0].(map[string]any)
	assert.NotEmpty(t, listedTA["notificationSettings"])
}

func TestHandler_GetProfile_ShowsAttributeMappings(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	recCreate := doREST(t, h, http.MethodPost, "/profiles", map[string]any{
		"name":     "mapping-visible-profile",
		"roleArns": []string{"arn:aws:iam::000000000000:role/Test"},
	})
	require.Equal(t, http.StatusCreated, recCreate.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &createResp))
	id := createResp["profile"].(map[string]any)["profileId"].(string)

	recPut := doREST(t, h, http.MethodPut, "/profiles/"+id+"/mappings", map[string]any{
		"certificateField": "x509Subject",
		"mappingRules":     []map[string]any{{"specifier": "CN"}},
	})
	require.Equal(t, http.StatusOK, recPut.Code)

	// GetProfile (not just the PutAttributeMapping response itself) must
	// also surface the mapping, since AWS's ProfileDetail carries
	// attributeMappings on every read.
	recGet := doREST(t, h, http.MethodGet, "/profile/"+id, nil)
	require.Equal(t, http.StatusOK, recGet.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(recGet.Body.Bytes(), &getResp))
	p := getResp["profile"].(map[string]any)
	mappings, ok := p["attributeMappings"].([]any)
	require.True(t, ok, "GetProfile response must carry attributeMappings, got: %s", recGet.Body.String())
	require.Len(t, mappings, 1)

	// ListProfiles must show it too.
	recList := doREST(t, h, http.MethodGet, "/profiles", nil)
	require.Equal(t, http.StatusOK, recList.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(recList.Body.Bytes(), &listResp))
	items := listResp["profiles"].([]any)
	require.Len(t, items, 1)
	listedProfile := items[0].(map[string]any)
	assert.NotEmpty(t, listedProfile["attributeMappings"])
}
