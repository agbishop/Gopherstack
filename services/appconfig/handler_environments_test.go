package appconfig_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appconfig"
)

func TestHandler_Environment_CRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create app.
	rec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"name":"env-app"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var app appconfig.Application
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))

	// Create env.
	rec = doRequest(
		t,
		h,
		http.MethodPost,
		"/applications/"+app.ID+"/environments",
		[]byte(`{"name":"production"}`),
	)
	require.Equal(t, http.StatusCreated, rec.Code)

	var env appconfig.Environment
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Equal(t, "production", env.Name)
	assert.Equal(t, app.ID, env.ApplicationID)

	// Get env.
	rec = doRequest(t, h, http.MethodGet, "/applications/"+app.ID+"/environments/"+env.ID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// List envs.
	rec = doRequest(t, h, http.MethodGet, "/applications/"+app.ID+"/environments", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Delete env.
	rec = doRequest(t, h, http.MethodDelete, "/applications/"+app.ID+"/environments/"+env.ID, nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_UpdateEnvironment_HTTP(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create app and env.
	rec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"name":"upd-env-app"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var app appconfig.Application
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))

	rec = doRequest(
		t,
		h,
		http.MethodPost,
		"/applications/"+app.ID+"/environments",
		[]byte(`{"name":"staging"}`),
	)
	require.Equal(t, http.StatusCreated, rec.Code)

	var env appconfig.Environment
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))

	// Update env.
	rec = doRequest(
		t,
		h,
		http.MethodPatch,
		"/applications/"+app.ID+"/environments/"+env.ID,
		[]byte(`{"name":"production","description":"updated"}`),
	)
	assert.Equal(t, http.StatusOK, rec.Code)

	var updated appconfig.Environment
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "production", updated.Name)
}

// TestHandler_UpdateEnvironment_OmittedFieldsPreserved verifies that
// UpdateEnvironment leaves Description and Monitors unchanged when they are
// omitted from the request, and that a present Monitors list does replace
// the existing one -- matching UpdateEnvironmentInput's optional members.
func TestHandler_UpdateEnvironment_OmittedFieldsPreserved(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"Name":"upd-env-app2"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var app appconfig.Application
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))

	rec = doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/environments",
		[]byte(`{"Name":"staging","Description":"keep-me","Monitors":[{"AlarmArn":"arn:aws:cloudwatch:alarm1"}]}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var env appconfig.Environment
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Monitors, 1)

	// Update only Name; Description and Monitors are omitted.
	rec = doRequest(t, h, http.MethodPatch, "/applications/"+app.ID+"/environments/"+env.ID,
		[]byte(`{"Name":"production"}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	var updated appconfig.Environment
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "production", updated.Name)
	assert.Equal(t, "keep-me", updated.Description, "omitted Description must be preserved")
	require.Len(t, updated.Monitors, 1, "omitted Monitors must be preserved")
	assert.Equal(t, "arn:aws:cloudwatch:alarm1", updated.Monitors[0].AlarmArn)

	// A present (even empty) Monitors list replaces the existing one.
	rec = doRequest(t, h, http.MethodPatch, "/applications/"+app.ID+"/environments/"+env.ID,
		[]byte(`{"Monitors":[]}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	var cleared appconfig.Environment
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &cleared))
	assert.Empty(t, cleared.Monitors, "an explicit empty Monitors list must replace the existing one")
}

func TestHandler_Environment_HTTP_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		pathSuffix string
		wantStatus int
	}{
		{
			name:       "get env not found",
			method:     http.MethodGet,
			pathSuffix: "/applications/nonexistent/environments/env-1",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "list envs app not found",
			method:     http.MethodGet,
			pathSuffix: "/applications/nonexistent/environments",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "delete env not found",
			method:     http.MethodDelete,
			pathSuffix: "/applications/nonexistent/environments/env-1",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.method, tt.pathSuffix, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_CreateEnvironment_TagsAppliedInline verifies that Tags sent
// inline on CreateEnvironmentInput are visible via ListTagsForResource
// immediately after creation -- previously CreateEnvironment's handler
// never bound or forwarded the Tags field at all, so tags set at create
// time silently vanished (bd gopherstack-lcan).
func TestHandler_CreateEnvironment_TagsAppliedInline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tags map[string]string
		name string
	}{
		{
			name: "tags_applied_at_create",
			tags: map[string]string{"env": "prod", "team": "platform"},
		},
		{
			name: "no_tags_is_not_an_error",
			tags: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			appRec := doRequest(t, h, http.MethodPost, "/applications",
				[]byte(`{"Name":"tagged-env-app-`+tt.name+`"}`))
			require.Equal(t, http.StatusCreated, appRec.Code)

			var app appconfig.Application
			require.NoError(t, json.Unmarshal(appRec.Body.Bytes(), &app))

			body, err := json.Marshal(map[string]any{
				"Name": "tagged-env-" + tt.name,
				"Tags": tt.tags,
			})
			require.NoError(t, err)

			rec := doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/environments", body)
			require.Equal(t, http.StatusCreated, rec.Code)

			var env appconfig.Environment
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))

			resourceArn := "arn:aws:appconfig:us-east-1:123456789012:application/" +
				app.ID + "/environment/" + env.ID
			tagsRec := doRequest(t, h, http.MethodGet, "/tags/"+resourceArn, nil)
			require.Equal(t, http.StatusOK, tagsRec.Code)

			var tagsResp struct {
				Tags map[string]string `json:"Tags"`
			}
			require.NoError(t, json.Unmarshal(tagsRec.Body.Bytes(), &tagsResp))

			if len(tt.tags) == 0 {
				assert.Empty(t, tagsResp.Tags)
			} else {
				assert.Equal(t, tt.tags, tagsResp.Tags)
			}
		})
	}
}

// TestParity_EnvironmentStateIsReadyForDeployment verifies that a newly created
// environment has State "READY_FOR_DEPLOYMENT", matching the real AWS AppConfig API.
func TestHandler_EnvironmentStateIsReadyForDeployment(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	appRec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"Name":"state-app"}`))
	require.Equal(t, http.StatusCreated, appRec.Code)

	var app struct {
		ID string `json:"Id"`
	}
	require.NoError(t, json.Unmarshal(appRec.Body.Bytes(), &app))

	tests := []struct {
		name string
		want string
		body []byte
	}{
		{
			name: "newly created environment is READY_FOR_DEPLOYMENT",
			body: []byte(`{"Name":"prod"}`),
			want: "READY_FOR_DEPLOYMENT",
		},
		{
			name: "second environment also is READY_FOR_DEPLOYMENT",
			body: []byte(`{"Name":"staging"}`),
			want: "READY_FOR_DEPLOYMENT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, http.MethodPost,
				"/applications/"+app.ID+"/environments", tt.body)
			require.Equal(t, http.StatusCreated, rec.Code)

			var env struct {
				State string `json:"State"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
			assert.Equal(t, tt.want, env.State,
				"environment State must be READY_FOR_DEPLOYMENT (SCREAMING_SNAKE_CASE)")
		})
	}
}

// TestParity_EnvironmentStateRoundTrip verifies the state is persisted correctly
// through Get and List after creation.
func TestHandler_EnvironmentStateRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	appRec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"Name":"rt-app"}`))
	require.Equal(t, http.StatusCreated, appRec.Code)

	var app struct {
		ID string `json:"Id"`
	}
	require.NoError(t, json.Unmarshal(appRec.Body.Bytes(), &app))

	envRec := doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/environments",
		[]byte(`{"Name":"staging"}`))
	require.Equal(t, http.StatusCreated, envRec.Code)

	var env struct {
		ID    string `json:"Id"`
		State string `json:"State"`
	}
	require.NoError(t, json.Unmarshal(envRec.Body.Bytes(), &env))
	assert.Equal(t, "READY_FOR_DEPLOYMENT", env.State)
	require.NotEmpty(t, env.ID)

	// GetEnvironment must also return correct state.
	getRec := doRequest(t, h, http.MethodGet,
		"/applications/"+app.ID+"/environments/"+env.ID, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getEnv struct {
		State string `json:"State"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getEnv))
	assert.Equal(t, "READY_FOR_DEPLOYMENT", getEnv.State, "GetEnvironment must also return READY_FOR_DEPLOYMENT")

	// ListEnvironments must also return correct state.
	listRec := doRequest(t, h, http.MethodGet, "/applications/"+app.ID+"/environments", nil)
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp struct {
		Items []struct {
			State string `json:"State"`
		} `json:"Items"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	require.Len(t, listResp.Items, 1)
	assert.Equal(t, "READY_FOR_DEPLOYMENT", listResp.Items[0].State,
		"ListEnvironments must also return READY_FOR_DEPLOYMENT")
}

// TestHandler_DeleteEnvironment_DeletionProtectionCheck proves the
// "X-Amzn-Deletion-Protection-Check" header (appconfig@v1.48.4
// serializers.go:1268, DeleteEnvironmentInput.DeletionProtectionCheck) is
// actually read: a recognized types.DeletionProtectionCheck value still
// deletes (this backend tracks no access-recency state to block on, so
// APPLY/ACCOUNT_DEFAULT behave like BYPASS -- see PARITY.md), but an
// unrecognized value -- which real AppConfig would reject -- now gets a
// BadRequestException instead of being silently ignored.
func TestHandler_DeleteEnvironment_DeletionProtectionCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		headerVal  string
		wantStatus int
	}{
		{name: "absent", headerVal: "", wantStatus: http.StatusNoContent},
		{name: "bypass", headerVal: "BYPASS", wantStatus: http.StatusNoContent},
		{name: "apply", headerVal: "APPLY", wantStatus: http.StatusNoContent},
		{name: "account default", headerVal: "ACCOUNT_DEFAULT", wantStatus: http.StatusNoContent},
		{name: "unrecognized value", headerVal: "NONSENSE", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"name":"dpc-app"}`))
			require.Equal(t, http.StatusCreated, rec.Code)

			var app appconfig.Application
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))

			rec = doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/environments",
				[]byte(`{"name":"dpc-env"}`))
			require.Equal(t, http.StatusCreated, rec.Code)

			var env appconfig.Environment
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))

			path := "/applications/" + app.ID + "/environments/" + env.ID

			var delRec *httptest.ResponseRecorder
			if tt.headerVal == "" {
				delRec = doRequest(t, h, http.MethodDelete, path, nil)
			} else {
				delRec = doRequestWithHeader(t, h, http.MethodDelete, path,
					"X-Amzn-Deletion-Protection-Check", tt.headerVal, nil)
			}
			assert.Equal(t, tt.wantStatus, delRec.Code)

			getRec := doRequest(t, h, http.MethodGet, path, nil)
			if tt.wantStatus == http.StatusNoContent {
				assert.Equal(t, http.StatusNotFound, getRec.Code, "environment should have been deleted")
			} else {
				assert.Equal(t, http.StatusOK, getRec.Code, "environment must survive a rejected delete")
			}
		})
	}
}
