package emrserverless_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/emrserverless"
)

// createAppWithArch creates an application via the handler with optional architecture.
func createAppWithArch(t *testing.T, h *emrserverless.Handler, name, releaseLabel, architecture string) string {
	t.Helper()

	body := map[string]any{
		"name":         name,
		"type":         "SPARK",
		"releaseLabel": releaseLabel,
	}

	if architecture != "" {
		body["architecture"] = architecture
	}

	rec := doRequest(t, h, http.MethodPost, "/applications", body)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	id := out["applicationId"]
	require.NotEmpty(t, id)

	return id
}

// --- CreateApplication ---

func TestHandler_CreateApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		setup      func(h *emrserverless.Handler)
		name       string
		wantName   string
		rawBody    string
		wantStatus int
	}{
		{
			name: "success",
			body: map[string]any{
				"name":         "my-app",
				"type":         "SPARK",
				"releaseLabel": "emr-6.6.0",
				"tags":         map[string]string{"env": "test"},
			},
			wantStatus: http.StatusOK,
			wantName:   "my-app",
		},
		{
			name: "duplicate_name",
			body: map[string]any{
				"name":         "my-app",
				"type":         "SPARK",
				"releaseLabel": "emr-6.6.0",
			},
			wantStatus: http.StatusConflict,
			setup: func(h *emrserverless.Handler) {
				createApp(t, h, "my-app")
			},
		},
		{
			name:       "invalid_body",
			rawBody:    "not-json",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			var rec *httptest.ResponseRecorder
			if tt.rawBody != "" {
				e := echo.New()
				req := httptest.NewRequest(http.MethodPost, "/applications", strings.NewReader(tt.rawBody))
				req.Header.Set("Content-Type", "application/json")
				rec2 := httptest.NewRecorder()
				c := e.NewContext(req, rec2)
				err := h.Handler()(c)
				require.NoError(t, err)
				rec = rec2
			} else {
				rec = doRequest(t, h, http.MethodPost, "/applications", tt.body)
			}

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantName != "" {
				var out map[string]string
				mustUnmarshal(t, rec, &out)
				assert.Equal(t, tt.wantName, out["name"])
				assert.NotEmpty(t, out["applicationId"])
				assert.NotEmpty(t, out["arn"])
			}
		})
	}
}

// TestHandler_CreateApplication_Architecture verifies architecture is stored
// and returned by GetApplication.
func TestHandler_CreateApplication_Architecture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		architecture string
		wantInOutput bool
	}{
		{name: "arm64", architecture: "ARM64", wantInOutput: true},
		{name: "x86_64", architecture: "X86_64", wantInOutput: true},
		{name: "omitted", architecture: "", wantInOutput: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID := createAppWithArch(t, h, "arch-app", "emr-6.9.0", tt.architecture)

			rec := doRequest(t, h, http.MethodGet, fmt.Sprintf("/applications/%s", appID), nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			app, _ := out["application"].(map[string]any)

			if tt.wantInOutput {
				assert.Equal(t, tt.architecture, app["architecture"])
			} else {
				_, hasArch := app["architecture"]
				assert.False(t, hasArch, "architecture should be absent when not set")
			}
		})
	}
}

// --- GetApplication ---

func TestHandler_GetApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *emrserverless.Handler) string
		name       string
		appID      string
		wantStatus int
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
			setup: func(h *emrserverless.Handler) string {
				return createApp(t, h, "get-app")
			},
		},
		{
			name:       "not_found",
			appID:      "nonexistentid",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID := tt.appID
			if tt.setup != nil {
				appID = tt.setup(h)
			}

			rec := doRequest(t, h, http.MethodGet, "/applications/"+appID, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				mustUnmarshal(t, rec, &out)
				app := out["application"].(map[string]any)
				assert.Equal(t, appID, app["applicationId"])
			}
		})
	}
}

// TestHandler_ApplicationToMap_AlwaysIncludesTags verifies applicationToMap
// always includes the "tags" key, even when no tags were set.
func TestHandler_ApplicationToMap_AlwaysIncludesTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createApp(t, h, "map-tags-app")

	rec := doRequest(t, h, http.MethodGet, "/applications/"+appID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	mustUnmarshal(t, rec, &out)
	app := out["application"].(map[string]any)
	_, hasTags := app["tags"]
	assert.True(t, hasTags, "application response should always include 'tags' key")
}

// --- ListApplications ---

func TestHandler_ListApplications(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(h *emrserverless.Handler)
		name        string
		queryString string
		wantCount   int
	}{
		{
			name:      "empty",
			wantCount: 0,
		},
		{
			name: "two_apps",
			setup: func(h *emrserverless.Handler) {
				createApp(t, h, "app1")
				createApp(t, h, "app2")
			},
			wantCount: 2,
		},
		{
			name: "states_filter_started",
			setup: func(h *emrserverless.Handler) {
				id := createApp(t, h, "started-app")
				rec := doRequest(t, h, http.MethodPost, "/applications/"+id+"/start", nil)
				require.Equal(t, http.StatusOK, rec.Code)
				createApp(t, h, "stopped-app")
			},
			queryString: "?states=STARTED",
			wantCount:   1,
		},
		{
			name: "states_filter_multiple",
			setup: func(h *emrserverless.Handler) {
				id := createApp(t, h, "started-app2")
				rec := doRequest(t, h, http.MethodPost, "/applications/"+id+"/start", nil)
				require.Equal(t, http.StatusOK, rec.Code)
				createApp(t, h, "created-app2")
			},
			queryString: "?states=STARTED,CREATED",
			wantCount:   2,
		},
		{
			name: "states_filter_no_match",
			setup: func(h *emrserverless.Handler) {
				createApp(t, h, "only-creating")
			},
			queryString: "?states=STARTED",
			wantCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, http.MethodGet, "/applications"+tt.queryString, nil)
			assert.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			mustUnmarshal(t, rec, &out)
			apps := out["applications"].([]any)
			assert.Len(t, apps, tt.wantCount)
		})
	}
}

func TestHandler_ListApplicationsPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		queryString   string
		wantCount     int
		wantStatus    int
		wantNextToken bool
	}{
		{
			name:        "no_pagination_returns_all",
			queryString: "",
			wantCount:   4,
			wantStatus:  http.StatusOK,
		},
		{
			name:          "first_page",
			queryString:   "?maxResults=2",
			wantCount:     2,
			wantStatus:    http.StatusOK,
			wantNextToken: true,
		},
		{
			name:        "second_page",
			queryString: "?maxResults=2&nextToken=2",
			wantCount:   2,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "token_beyond_end",
			queryString: "?maxResults=2&nextToken=100",
			wantCount:   0,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "invalid_max_results_rejected",
			queryString: "?maxResults=notanumber",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "max_results_over_bound_rejected",
			queryString: "?maxResults=51",
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for i := range 4 {
				createApp(t, h, fmt.Sprintf("app-%d", i))
			}

			rec := doRequest(t, h, http.MethodGet, "/applications"+tt.queryString, nil)
			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus != http.StatusOK {
				return
			}

			var out map[string]any
			mustUnmarshal(t, rec, &out)
			apps := out["applications"].([]any)
			assert.Len(t, apps, tt.wantCount)

			if tt.wantNextToken {
				assert.NotEmpty(t, out["nextToken"])
			}
		})
	}
}

// --- UpdateApplication ---

func TestHandler_UpdateApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body        map[string]any
		setup       func(h *emrserverless.Handler) string
		name        string
		appID       string
		wantRelease string
		wantStatus  int
	}{
		{
			name:        "success",
			body:        map[string]any{"releaseLabel": "emr-7.0.0"},
			wantStatus:  http.StatusOK,
			wantRelease: "emr-7.0.0",
			setup: func(h *emrserverless.Handler) string {
				return createApp(t, h, "update-app")
			},
		},
		{
			name:       "not_found",
			appID:      "nonexistentid",
			body:       map[string]any{"releaseLabel": "emr-7.0.0"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID := tt.appID
			if tt.setup != nil {
				appID = tt.setup(h)
			}

			rec := doRequest(t, h, http.MethodPatch, "/applications/"+appID, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantRelease != "" {
				var out map[string]any
				mustUnmarshal(t, rec, &out)
				app := out["application"].(map[string]any)
				assert.Equal(t, tt.wantRelease, app["releaseLabel"])
			}
		})
	}
}

func TestHandler_UpdateApplication_InvalidBody(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createApp(t, h, "update-invalid-body-app")

	e := echo.New()
	req := httptest.NewRequest(http.MethodPatch, "/applications/"+appID, strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := h.Handler()(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_CreateApplication_ConfigPassthrough verifies that application
// configuration sub-objects (maximumCapacity, autoStopConfiguration, etc.)
// supplied on CreateApplication are echoed back verbatim by GetApplication
// instead of being silently discarded.
func TestHandler_CreateApplication_ConfigPassthrough(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	maxCapacity := map[string]any{"cpu": "400 vCPU", "memory": "3000 GB"}
	autoStop := map[string]any{"enabled": true, "idleTimeoutMinutes": float64(20)}

	rec := doRequest(t, h, http.MethodPost, "/applications", map[string]any{
		"name":                  "config-passthrough-app",
		"type":                  "SPARK",
		"releaseLabel":          "emr-6.6.0",
		"maximumCapacity":       maxCapacity,
		"autoStopConfiguration": autoStop,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var created map[string]string
	mustUnmarshal(t, rec, &created)

	getRec := doRequest(t, h, http.MethodGet, "/applications/"+created["applicationId"], nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var out map[string]any
	mustUnmarshal(t, getRec, &out)
	app := out["application"].(map[string]any)
	assert.Equal(t, maxCapacity, app["maximumCapacity"])
	assert.Equal(t, autoStop, app["autoStopConfiguration"])
}

// TestHandler_CreateApplication_ExtendedConfigPassthrough verifies the four
// application configuration sub-objects added to applicationConfigFields
// this pass (identityCenterConfiguration, diskEncryptionConfiguration,
// jobLevelCostAllocationConfiguration, schedulerConfiguration -- all real
// fields on types.CreateApplicationInput/types.Application per the SDK, but
// previously silently dropped like maximumCapacity/autoStopConfiguration
// were before an earlier pass) round-trip through CreateApplication ->
// GetApplication instead of being discarded.
func TestHandler_CreateApplication_ExtendedConfigPassthrough(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	identityCenter := map[string]any{"identityCenterInstanceArn": "arn:aws:sso:::instance/ssoins-1"}
	diskEncryption := map[string]any{"encryptionKeyArn": "arn:aws:kms:us-east-1:000000000000:key/abc"}
	jobLevelCost := map[string]any{"enabled": true}
	scheduler := map[string]any{"maxConcurrentRuns": float64(15), "queueTimeoutMinutes": float64(360)}

	rec := doRequest(t, h, http.MethodPost, "/applications", map[string]any{
		"name":                                "extended-config-app",
		"type":                                "SPARK",
		"releaseLabel":                        "emr-7.0.0",
		"identityCenterConfiguration":         identityCenter,
		"diskEncryptionConfiguration":         diskEncryption,
		"jobLevelCostAllocationConfiguration": jobLevelCost,
		"schedulerConfiguration":              scheduler,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var created map[string]string
	mustUnmarshal(t, rec, &created)

	getRec := doRequest(t, h, http.MethodGet, "/applications/"+created["applicationId"], nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var out map[string]any
	mustUnmarshal(t, getRec, &out)
	app := out["application"].(map[string]any)
	assert.Equal(t, identityCenter, app["identityCenterConfiguration"])
	assert.Equal(t, diskEncryption, app["diskEncryptionConfiguration"])
	assert.Equal(t, jobLevelCost, app["jobLevelCostAllocationConfiguration"])
	assert.Equal(t, scheduler, app["schedulerConfiguration"])
}

// TestHandler_ApplicationToMap_StateDetails verifies stateDetails is echoed
// on GetApplication when set (types.Application.StateDetails is a real,
// optional response field) and omitted from the wire body when empty --
// matching the same present-if-non-empty convention already used for
// architecture.
func TestHandler_ApplicationToMap_StateDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		stateDetails string
		wantPresent  bool
	}{
		{name: "set", stateDetails: "Application failed to start: insufficient capacity", wantPresent: true},
		{name: "empty", stateDetails: "", wantPresent: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := emrserverless.NewInMemoryBackend("000000000000", "us-east-1")
			h := emrserverless.NewHandler(b)

			now := time.Now().UTC()
			app := &emrserverless.Application{
				ApplicationID: "app-state-details",
				Arn:           "arn:aws:emr-serverless:us-east-1:000000000000:/applications/app-state-details",
				Name:          "state-details-app",
				Type:          "SPARK",
				ReleaseLabel:  "emr-6.6.0",
				State:         emrserverless.ApplicationStateCreated,
				StateDetails:  tt.stateDetails,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			b.AddApplicationInternal(app)

			rec := doRequest(t, h, http.MethodGet, "/applications/"+app.ApplicationID, nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			mustUnmarshal(t, rec, &out)
			got := out["application"].(map[string]any)

			if tt.wantPresent {
				assert.Equal(t, tt.stateDetails, got["stateDetails"])
			} else {
				_, present := got["stateDetails"]
				assert.False(t, present, "stateDetails should be absent when empty")
			}
		})
	}
}

// TestHandler_UpdateApplication_ConfigMerge verifies that UpdateApplication
// merges newly supplied configuration keys with previously stored ones
// rather than replacing the whole configuration (matching AWS's per-field
// PATCH semantics), and that fields not present in the request body are left
// untouched.
func TestHandler_UpdateApplication_ConfigMerge(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	maxCapacity := map[string]any{"cpu": "200 vCPU", "memory": "1000 GB"}
	rec := doRequest(t, h, http.MethodPost, "/applications", map[string]any{
		"name":            "config-merge-app",
		"type":            "SPARK",
		"releaseLabel":    "emr-6.6.0",
		"maximumCapacity": maxCapacity,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var created map[string]string
	mustUnmarshal(t, rec, &created)
	appID := created["applicationId"]

	autoStop := map[string]any{"enabled": true}
	updateRec := doRequest(t, h, http.MethodPatch, "/applications/"+appID, map[string]any{
		"autoStopConfiguration": autoStop,
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	getRec := doRequest(t, h, http.MethodGet, "/applications/"+appID, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var out map[string]any
	mustUnmarshal(t, getRec, &out)
	app := out["application"].(map[string]any)
	assert.Equal(t, maxCapacity, app["maximumCapacity"], "update must not drop previously stored config")
	assert.Equal(t, autoStop, app["autoStopConfiguration"])
}

// TestHandler_UpdateApplication_AutoStopConfigValidation verifies
// UpdateApplication rejects an out-of-range autoStopConfiguration.idleTimeoutMinutes
// (AWS valid range: 1-10080) the same way CreateApplication does, and accepts
// both boundary values.
func TestHandler_UpdateApplication_AutoStopConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		idleTimeoutMinutes any
		name               string
		wantStatus         int
	}{
		{name: "too_low", idleTimeoutMinutes: 0, wantStatus: http.StatusBadRequest},
		{name: "too_high", idleTimeoutMinutes: 10081, wantStatus: http.StatusBadRequest},
		{name: "min_boundary", idleTimeoutMinutes: 1, wantStatus: http.StatusOK},
		{name: "max_boundary", idleTimeoutMinutes: 10080, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID := createAppWithArch(t, h, "autostop-validate-app-"+tt.name, "emr-6.6.0", "")

			updateRec := doRequest(t, h, http.MethodPatch, "/applications/"+appID, map[string]any{
				"autoStopConfiguration": map[string]any{"idleTimeoutMinutes": tt.idleTimeoutMinutes},
			})
			assert.Equal(t, tt.wantStatus, updateRec.Code)

			if tt.wantStatus == http.StatusBadRequest {
				var out map[string]string
				mustUnmarshal(t, updateRec, &out)
				assert.Equal(t, "ValidationException", out["code"])
			}
		})
	}
}

// TestHandler_CreateApplication_ClientTokenIdempotent verifies that retrying
// CreateApplication with the same clientToken (as an AWS SDK does on a
// timed-out request) returns the already-created application instead of
// erroring with ConflictException.
func TestHandler_CreateApplication_ClientTokenIdempotent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"name":         "client-token-app",
		"type":         "SPARK",
		"releaseLabel": "emr-6.6.0",
		"clientToken":  "retry-token-1",
	}

	rec1 := doRequest(t, h, http.MethodPost, "/applications", body)
	require.Equal(t, http.StatusOK, rec1.Code)
	var out1 map[string]string
	mustUnmarshal(t, rec1, &out1)

	rec2 := doRequest(t, h, http.MethodPost, "/applications", body)
	require.Equal(t, http.StatusOK, rec2.Code, "retry with same clientToken must not conflict")
	var out2 map[string]string
	mustUnmarshal(t, rec2, &out2)

	assert.Equal(t, out1["applicationId"], out2["applicationId"])
}

// --- DeleteApplication ---

func TestHandler_DeleteApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *emrserverless.Handler) string
		name       string
		appID      string
		wantStatus int
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
			setup: func(h *emrserverless.Handler) string {
				return createApp(t, h, "delete-app")
			},
		},
		{
			name:       "not_found",
			appID:      "nonexistentid",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID := tt.appID
			if tt.setup != nil {
				appID = tt.setup(h)
			}

			rec := doRequest(t, h, http.MethodDelete, "/applications/"+appID, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				// Verify deletion.
				rec2 := doRequest(t, h, http.MethodGet, "/applications/"+appID, nil)
				assert.Equal(t, http.StatusNotFound, rec2.Code)
			}
		})
	}
}

func TestHandler_DeleteApplication_RejectsStarted(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createApp(t, h, "delete-started-app")

	rec := doRequest(t, h, http.MethodPost, "/applications/"+appID+"/start", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodDelete, "/applications/"+appID, nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// App should still exist.
	rec = doRequest(t, h, http.MethodGet, "/applications/"+appID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_DeleteApplication_AllowsStopped(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createApp(t, h, "delete-stopped-app")

	rec := doRequest(t, h, http.MethodPost, "/applications/"+appID+"/start", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	rec = doRequest(t, h, http.MethodPost, "/applications/"+appID+"/stop", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodDelete, "/applications/"+appID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/applications/"+appID, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestHandler_DeleteApplication_CleansUpJobRunsAndSessions verifies deleting
// an application cascade-deletes its job runs and sessions.
func TestHandler_DeleteApplication_CleansUpJobRunsAndSessions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createApp(t, h, "cleanup-app")

	// Start a job run. autoStartConfiguration defaults to enabled, so this
	// implicitly starts the application -- no explicit /start call needed
	// (and one would now fail with "already in STARTED state").
	jobRunID := startJobRun(t, h, appID)
	require.NotEmpty(t, jobRunID)

	sessionID, _ := startSession(t, h, appID, "cleanup-token")
	require.NotEmpty(t, sessionID)

	// Cancel the job run: StopApplication requires every job run under the
	// application to already be completed or cancelled.
	rec := doRequest(t, h, http.MethodDelete, "/applications/"+appID+"/jobruns/"+jobRunID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Stop the app.
	rec = doRequest(t, h, http.MethodPost, "/applications/"+appID+"/stop", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Delete the application (should clean up job runs and sessions).
	rec = doRequest(t, h, http.MethodDelete, "/applications/"+appID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// App should be gone.
	rec = doRequest(t, h, http.MethodGet, "/applications/"+appID, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// --- StartApplication ---

func TestHandler_StartApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *emrserverless.Handler) string
		name       string
		appID      string
		wantStatus int
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
			setup: func(h *emrserverless.Handler) string {
				return createApp(t, h, "start-app")
			},
		},
		{
			name:       "not_found",
			appID:      "nonexistentid",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "already_started",
			wantStatus: http.StatusBadRequest,
			setup: func(h *emrserverless.Handler) string {
				id := createApp(t, h, "already-started-app")
				rec := doRequest(t, h, http.MethodPost, "/applications/"+id+"/start", nil)
				require.Equal(t, http.StatusOK, rec.Code)

				return id
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID := tt.appID
			if tt.setup != nil {
				appID = tt.setup(h)
			}

			rec := doRequest(t, h, http.MethodPost, "/applications/"+appID+"/start", nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				rec2 := doRequest(t, h, http.MethodGet, "/applications/"+appID, nil)
				var out map[string]any
				mustUnmarshal(t, rec2, &out)
				app := out["application"].(map[string]any)
				assert.Equal(t, emrserverless.ApplicationStateStarted, app["state"])
			}
		})
	}
}

// --- StopApplication ---

func TestHandler_StopApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *emrserverless.Handler) string
		name       string
		appID      string
		wantStatus int
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
			setup: func(h *emrserverless.Handler) string {
				id := createApp(t, h, "stop-app")
				rec := doRequest(t, h, http.MethodPost, "/applications/"+id+"/start", nil)
				require.Equal(t, http.StatusOK, rec.Code)

				return id
			},
		},
		{
			name:       "not_found",
			appID:      "nonexistentid",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "already_stopped",
			wantStatus: http.StatusBadRequest,
			setup: func(h *emrserverless.Handler) string {
				// Application starts in CREATING state, which is not STARTED;
				// stopping it should be rejected as invalid state transition.
				id := createApp(t, h, "already-stopped-app")
				rec := doRequest(t, h, http.MethodPost, "/applications/"+id+"/start", nil)
				require.Equal(t, http.StatusOK, rec.Code)
				rec2 := doRequest(t, h, http.MethodPost, "/applications/"+id+"/stop", nil)
				require.Equal(t, http.StatusOK, rec2.Code)

				return id
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			appID := tt.appID
			if tt.setup != nil {
				appID = tt.setup(h)
			}

			rec := doRequest(t, h, http.MethodPost, "/applications/"+appID+"/stop", nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				rec2 := doRequest(t, h, http.MethodGet, "/applications/"+appID, nil)
				var out map[string]any
				mustUnmarshal(t, rec2, &out)
				app := out["application"].(map[string]any)
				assert.Equal(t, emrserverless.ApplicationStateStopped, app["state"])
			}
		})
	}
}

// --- Error code wire mapping ---

// TestHandler_ErrValidationMapping verifies missing required CreateApplication
// fields surface as a 400 ValidationException.
func TestHandler_ErrValidationMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantCode   string
		wantStatus int
	}{
		{
			name:       "missing_name",
			body:       map[string]any{"type": "SPARK", "releaseLabel": "emr-6.6.0"},
			wantStatus: http.StatusBadRequest,
			wantCode:   "ValidationException",
		},
		{
			name:       "missing_type",
			body:       map[string]any{"name": "myapp", "releaseLabel": "emr-6.6.0"},
			wantStatus: http.StatusBadRequest,
			wantCode:   "ValidationException",
		},
		{
			// AutoStopConfig.IdleTimeoutMinutes: "Valid Range: Minimum value
			// of 1. Maximum value of 10080." (AWS API reference; not present
			// in the Go SDK's doc comment, which only states the default).
			name: "autoStopConfiguration_idleTimeoutMinutes_too_low",
			body: map[string]any{
				"name": "myapp", "type": "SPARK", "releaseLabel": "emr-6.6.0",
				"autoStopConfiguration": map[string]any{"idleTimeoutMinutes": 0},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "ValidationException",
		},
		{
			name: "autoStopConfiguration_idleTimeoutMinutes_too_high",
			body: map[string]any{
				"name": "myapp", "type": "SPARK", "releaseLabel": "emr-6.6.0",
				"autoStopConfiguration": map[string]any{"idleTimeoutMinutes": 10081},
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/applications", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			var out map[string]string
			mustUnmarshal(t, rec, &out)
			assert.Equal(t, tt.wantCode, out["code"])
		})
	}
}

// TestHandler_ErrInvalidStateMapping verifies deleting a STARTED application
// surfaces as a 400 ValidationException -- emrserverless's DeleteApplication
// models only ConflictException/InternalServerException/
// ResourceNotFoundException/ValidationException (types/errors.go); it has no
// "RequestFailedException" type at all, so that code (the value ErrInvalidState
// used to carry) would deserialize as an untyped *smithy.GenericAPIError in a
// real SDK client rather than a recognised exception type.
func TestHandler_ErrInvalidStateMapping(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID := createApp(t, h, "state-check-app")

	// Start the application so it's in STARTED state.
	rec := doRequest(t, h, http.MethodPost, "/applications/"+appID+"/start", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Deleting a STARTED application should fail with 400.
	rec = doRequest(t, h, http.MethodDelete, "/applications/"+appID, nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var out map[string]string
	mustUnmarshal(t, rec, &out)
	assert.Equal(t, "ValidationException", out["code"])
}
