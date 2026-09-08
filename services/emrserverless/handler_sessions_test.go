package emrserverless_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/emrserverless"
)

const sessionRoleARN = "arn:aws:iam::000000000000:role/interactive-session"

func createStartedApp(t *testing.T, h *emrserverless.Handler) string {
	t.Helper()

	appID := createApp(t, h, "interactive-app")
	rec := doRequest(t, h, http.MethodPost, "/applications/"+appID+"/start", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	return appID
}

func startSession(t *testing.T, h *emrserverless.Handler, appID, token string) (string, string) {
	t.Helper()

	rec := doRequest(t, h, http.MethodPost, "/applications/"+appID+"/sessions", map[string]any{
		"clientToken":        token,
		"executionRoleArn":   sessionRoleARN,
		"name":               "interactive",
		"idleTimeoutMinutes": 30,
		"tags":               map[string]string{"purpose": "notebook"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]string
	mustUnmarshal(t, rec, &out)
	require.NotEmpty(t, out["sessionId"])

	return out["sessionId"], out["arn"]
}

func TestHandler_SessionOperations(t *testing.T) {
	t.Parallel()

	type setupResult struct {
		appID     string
		sessionID string
	}
	tests := []struct {
		setup      func(*testing.T, *emrserverless.Handler) setupResult
		path       func(setupResult) string
		body       map[string]any
		name       string
		method     string
		wantField  string
		wantStatus int
	}{
		{
			name: "start_session",
			setup: func(t *testing.T, h *emrserverless.Handler) setupResult {
				t.Helper()

				return setupResult{appID: createStartedApp(t, h)}
			},
			method: http.MethodPost,
			path:   func(r setupResult) string { return "/applications/" + r.appID + "/sessions" },
			body: map[string]any{
				"clientToken":      "new-token",
				"executionRoleArn": sessionRoleARN,
			},
			wantStatus: http.StatusOK,
			wantField:  "sessionId",
		},
		{
			name: "get_session",
			setup: func(t *testing.T, h *emrserverless.Handler) setupResult {
				t.Helper()

				appID := createStartedApp(t, h)
				sessionID, _ := startSession(t, h, appID, "get-token")

				return setupResult{appID: appID, sessionID: sessionID}
			},
			method:     http.MethodGet,
			path:       func(r setupResult) string { return "/applications/" + r.appID + "/sessions/" + r.sessionID },
			wantStatus: http.StatusOK,
			wantField:  "session",
		},
		{
			name: "list_sessions",
			setup: func(t *testing.T, h *emrserverless.Handler) setupResult {
				t.Helper()

				appID := createStartedApp(t, h)
				startSession(t, h, appID, "list-token")

				return setupResult{appID: appID}
			},
			method:     http.MethodGet,
			path:       func(r setupResult) string { return "/applications/" + r.appID + "/sessions" },
			wantStatus: http.StatusOK,
			wantField:  "sessions",
		},
		{
			name: "get_session_endpoint",
			setup: func(t *testing.T, h *emrserverless.Handler) setupResult {
				t.Helper()

				appID := createStartedApp(t, h)
				sessionID, _ := startSession(t, h, appID, "endpoint-token")

				return setupResult{appID: appID, sessionID: sessionID}
			},
			method: http.MethodGet,
			path: func(r setupResult) string {
				return "/applications/" + r.appID + "/sessions/" + r.sessionID + "/endpoint"
			},
			wantStatus: http.StatusOK,
			wantField:  "authToken",
		},
		{
			name: "get_resource_dashboard",
			setup: func(t *testing.T, h *emrserverless.Handler) setupResult {
				t.Helper()

				appID := createStartedApp(t, h)
				sessionID, _ := startSession(t, h, appID, "dash-token")

				return setupResult{appID: appID, sessionID: sessionID}
			},
			method: http.MethodGet,
			path: func(r setupResult) string {
				return fmt.Sprintf(
					"/applications/%s/dashboard?resourceId=%s&resourceType=SESSION",
					r.appID,
					r.sessionID,
				)
			},
			wantStatus: http.StatusOK,
			wantField:  "url",
		},
		{
			name: "terminate_session",
			setup: func(t *testing.T, h *emrserverless.Handler) setupResult {
				t.Helper()

				appID := createStartedApp(t, h)
				sessionID, _ := startSession(t, h, appID, "terminate-token")

				return setupResult{appID: appID, sessionID: sessionID}
			},
			method:     http.MethodDelete,
			path:       func(r setupResult) string { return "/applications/" + r.appID + "/sessions/" + r.sessionID },
			wantStatus: http.StatusOK,
			wantField:  "sessionId",
		},
		{
			// autoStartConfiguration defaults to enabled (types.AutoStartConfig.
			// Enabled doc: "Defaults to true"), so a session start on a
			// non-STARTED application only fails when it's explicitly disabled.
			name: "start_requires_started_application_when_autostart_disabled",
			setup: func(t *testing.T, h *emrserverless.Handler) setupResult {
				t.Helper()

				rec := doRequest(t, h, http.MethodPost, "/applications", map[string]any{
					"name":                   "stopped-app",
					"type":                   "SPARK",
					"releaseLabel":           "emr-6.6.0",
					"autoStartConfiguration": map[string]any{"enabled": false},
				})
				require.Equal(t, http.StatusOK, rec.Code)

				var out map[string]string
				mustUnmarshal(t, rec, &out)
				require.NotEmpty(t, out["applicationId"])

				return setupResult{appID: out["applicationId"]}
			},
			method: http.MethodPost,
			path:   func(r setupResult) string { return "/applications/" + r.appID + "/sessions" },
			body: map[string]any{
				"clientToken":      "invalid-state-token",
				"executionRoleArn": sessionRoleARN,
			},
			wantStatus: http.StatusConflict,
			wantField:  "code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			result := tt.setup(t, h)
			rec := doRequest(t, h, tt.method, tt.path(result), tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
			var out map[string]any
			mustUnmarshal(t, rec, &out)
			assert.Contains(t, out, tt.wantField)
		})
	}
}

// TestHandler_StartSession_AutoStartsApplication verifies that starting a
// session on a never-started (CREATED) application implicitly starts it,
// matching aws-sdk-go-v2's api_op_StartSession.go doc: "The application must
// be in the STARTED state or have AutoStart enabled", and
// types.AutoStartConfig.Enabled's documented default of true.
func TestHandler_StartSession_AutoStartsApplication(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createApp(t, h, "session-autostart-app")

	rec := doRequest(t, h, http.MethodPost, "/applications/"+appID+"/sessions", map[string]any{
		"clientToken":      "autostart-token",
		"executionRoleArn": sessionRoleARN,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/applications/"+appID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	mustUnmarshal(t, rec, &out)
	app, ok := out["application"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "STARTED", app["state"])
}

func TestHandler_SessionIdempotencyAndTermination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createStartedApp(t, h)
	firstID, sessionARN := startSession(t, h, appID, "stable-token")
	secondID, _ := startSession(t, h, appID, "stable-token")
	assert.Equal(t, firstID, secondID)

	tagPath := "/tags/" + url.PathEscape(sessionARN)
	rec := doRequest(t, h, http.MethodPost, tagPath, map[string]any{"tags": map[string]string{"env": "dev"}})
	require.Equal(t, http.StatusOK, rec.Code)
	rec = doRequest(t, h, http.MethodGet, tagPath, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"env":"dev"`)

	rec = doRequest(t, h, http.MethodDelete, "/applications/"+appID+"/sessions/"+firstID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = doRequest(t, h, http.MethodGet, "/applications/"+appID+"/sessions/"+firstID+"/endpoint", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	rec = doRequest(
		t,
		h,
		http.MethodGet,
		"/applications/"+appID+"/dashboard?resourceId="+firstID+"&resourceType=SESSION",
		nil,
	)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_SessionFilteringAndPersistence(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createStartedApp(t, h)
	sessionID, _ := startSession(t, h, appID, "persist-token")
	doRequest(t, h, http.MethodDelete, "/applications/"+appID+"/sessions/"+sessionID, nil)
	startSession(t, h, appID, "running-token")

	rec := doRequest(t, h, http.MethodGet, "/applications/"+appID+"/sessions?states=TERMINATED", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var filtered struct {
		Sessions []map[string]any `json:"sessions"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &filtered))
	require.Len(t, filtered.Sessions, 1)
	assert.Equal(t, emrserverless.SessionStateTerminated, filtered.Sessions[0]["state"])

	restored := newTestHandler(t)
	require.NoError(t, restored.Restore(t.Context(), h.Snapshot(t.Context())))
	rec = doRequest(t, restored, http.MethodGet, "/applications/"+appID+"/sessions/"+sessionID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), emrserverless.SessionStateTerminated)
}

func TestHandler_SessionRoutes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method string
		path   string
		want   string
	}{
		{method: http.MethodPost, path: "/applications/a/sessions", want: "StartSession"},
		{method: http.MethodGet, path: "/applications/a/sessions", want: "ListSessions"},
		{method: http.MethodGet, path: "/applications/a/sessions/s", want: "GetSession"},
		{method: http.MethodDelete, path: "/applications/a/sessions/s", want: "TerminateSession"},
		{method: http.MethodGet, path: "/applications/a/sessions/s/endpoint", want: "GetSessionEndpoint"},
		{method: http.MethodGet, path: "/applications/a/dashboard", want: "GetResourceDashboard"},
	}
	h := newTestHandler(t)
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			e := echo.New()
			c := e.NewContext(httptest.NewRequest(tt.method, tt.path, nil), httptest.NewRecorder())
			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}

// --- parseSessionListQuery uncovered branches ---

func TestHandler_ListSessions_InvalidMaxResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		queryString string
		wantStatus  int
	}{
		{
			name:        "negative_max_results",
			queryString: "?maxResults=-1",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "zero_max_results",
			queryString: "?maxResults=0",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "non_numeric_max_results",
			queryString: "?maxResults=abc",
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID := createStartedApp(t, h)
			startSession(t, h, appID, "token-"+tt.name)

			rec := doRequest(t, h, http.MethodGet, "/applications/"+appID+"/sessions"+tt.queryString, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// --- parseQueryTime uncovered branches ---

func TestHandler_ListSessions_TimeFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		queryString string
		wantStatus  int
		wantCount   int
	}{
		{
			name:        "valid_created_at_after",
			queryString: "?createdAtAfter=" + time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
			wantStatus:  http.StatusOK,
			wantCount:   1, // session created now is after 1 hour ago
		},
		{
			name:        "valid_created_at_before",
			queryString: "?createdAtBefore=" + time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			wantStatus:  http.StatusOK,
			wantCount:   1, // session created now is before 1 hour from now
		},
		{
			name:        "invalid_created_at_after",
			queryString: "?createdAtAfter=not-a-date",
			wantStatus:  http.StatusBadRequest,
			wantCount:   0,
		},
		{
			name:        "invalid_created_at_before",
			queryString: "?createdAtBefore=not-a-date",
			wantStatus:  http.StatusBadRequest,
			wantCount:   0,
		},
		{
			name:        "created_at_after_filters_out_old_session",
			queryString: "?createdAtAfter=" + time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			wantStatus:  http.StatusOK,
			wantCount:   0,
		},
		{
			name:        "created_at_before_filters_out_new_session",
			queryString: "?createdAtBefore=" + time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
			wantStatus:  http.StatusOK,
			wantCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID := createStartedApp(t, h)
			startSession(t, h, appID, "time-token-"+tt.name)

			rec := doRequest(t, h, http.MethodGet, "/applications/"+appID+"/sessions"+tt.queryString, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				mustUnmarshal(t, rec, &out)
				sessions := out["sessions"].([]any)
				assert.Len(t, sessions, tt.wantCount)
			}
		})
	}
}

// --- ListSessions with pagination nextToken ---

func TestHandler_ListSessions_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createStartedApp(t, h)

	for i := range 3 {
		startSession(t, h, appID, "page-token-"+string(rune('a'+i)))
	}

	rec := doRequest(t, h, http.MethodGet, "/applications/"+appID+"/sessions?maxResults=2", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "nextToken")

	var out map[string]any
	mustUnmarshal(t, rec, &out)
	nextToken, ok := out["nextToken"].(string)
	require.True(t, ok)

	rec2 := doRequest(t, h, http.MethodGet, "/applications/"+appID+"/sessions?nextToken="+nextToken, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var out2 map[string]any
	mustUnmarshal(t, rec2, &out2)
	sessions := out2["sessions"].([]any)
	assert.NotEmpty(t, sessions)
}

// --- handleStartSession with invalid body ---

func TestHandler_StartSession_InvalidBody(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createStartedApp(t, h)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/applications/"+appID+"/sessions", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- TerminateSession error branches ---

func TestHandler_TerminateSession_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *emrserverless.Handler) (appID, sessionID string)
		name       string
		wantStatus int
	}{
		{
			name: "app_not_found",
			setup: func(_ *emrserverless.Handler) (string, string) {
				return "nonexistent-app", "nonexistent-session"
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "session_not_found",
			setup: func(h *emrserverless.Handler) (string, string) {
				appID := createStartedApp(t, h)

				return appID, "nonexistent-session"
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "already_terminated",
			setup: func(h *emrserverless.Handler) (string, string) {
				appID := createStartedApp(t, h)
				sessionID, _ := startSession(t, h, appID, "term-token")
				rec := doRequest(t, h, http.MethodDelete, "/applications/"+appID+"/sessions/"+sessionID, nil)
				require.Equal(t, http.StatusOK, rec.Code)

				return appID, sessionID
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID, sessionID := tt.setup(h)
			rec := doRequest(t, h, http.MethodDelete, "/applications/"+appID+"/sessions/"+sessionID, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// --- GetSession errors ---

func TestHandler_GetSession_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *emrserverless.Handler) (appID, sessionID string)
		name       string
		wantStatus int
	}{
		{
			name: "app_not_found",
			setup: func(_ *emrserverless.Handler) (string, string) {
				return "nonexistent-app", "nonexistent-session"
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "session_not_found",
			setup: func(h *emrserverless.Handler) (string, string) {
				appID := createStartedApp(t, h)

				return appID, "nonexistent-session"
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID, sessionID := tt.setup(h)
			rec := doRequest(t, h, http.MethodGet, "/applications/"+appID+"/sessions/"+sessionID, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// --- ListSessions errors ---

func TestHandler_ListSessions_AppNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/applications/nonexistent/sessions", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// --- GetSessionEndpoint errors ---

func TestHandler_GetSessionEndpoint_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *emrserverless.Handler) (appID, sessionID string)
		name       string
		wantStatus int
	}{
		{
			name: "app_not_found",
			setup: func(_ *emrserverless.Handler) (string, string) {
				return "nonexistent-app", "nonexistent-session"
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "session_not_found",
			setup: func(h *emrserverless.Handler) (string, string) {
				appID := createStartedApp(t, h)

				return appID, "nonexistent-session"
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID, sessionID := tt.setup(h)
			rec := doRequest(t, h, http.MethodGet, "/applications/"+appID+"/sessions/"+sessionID+"/endpoint", nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// --- GetResourceDashboard errors ---

func TestHandler_GetResourceDashboard_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "missing_resource_type",
			path:       "/applications/some-app/dashboard?resourceId=s1&resourceType=JOBRUN",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "app_not_found",
			path:       "/applications/nonexistent/dashboard?resourceId=s1&resourceType=SESSION",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodGet, tt.path, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// --- sessionToMap with EndedAt set and optional fields ---

func TestSessionToMap_WithTerminatedSession(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createStartedApp(t, h)
	sessionID, _ := startSession(t, h, appID, "map-term-token")

	// Terminate session so EndedAt is set.
	rec := doRequest(t, h, http.MethodDelete, "/applications/"+appID+"/sessions/"+sessionID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Retrieve the terminated session — endedAt should be present.
	rec = doRequest(t, h, http.MethodGet, "/applications/"+appID+"/sessions/"+sessionID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "endedAt")
}
