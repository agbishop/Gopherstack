package appconfig_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	appconfigsdk "github.com/aws/aws-sdk-go-v2/service/appconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appconfig"
)

func TestHandler_ConfigurationProfile_CRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"name":"prof-app"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var app appconfig.Application
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))

	// Create profile.
	profileBody := []byte(`{"name":"my-config","locationUri":"hosted","type":"AWS.Freeform"}`)
	rec = doRequest(
		t,
		h,
		http.MethodPost,
		"/applications/"+app.ID+"/configurationprofiles",
		profileBody,
	)
	require.Equal(t, http.StatusCreated, rec.Code)

	var profile appconfig.ConfigurationProfile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &profile))
	assert.Equal(t, "my-config", profile.Name)

	// Get profile.
	rec = doRequest(
		t,
		h,
		http.MethodGet,
		"/applications/"+app.ID+"/configurationprofiles/"+profile.ID,
		nil,
	)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Delete profile.
	rec = doRequest(
		t,
		h,
		http.MethodDelete,
		"/applications/"+app.ID+"/configurationprofiles/"+profile.ID,
		nil,
	)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandler_ListConfigurationProfiles_HTTP(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"name":"list-prof-app"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var app appconfig.Application
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))

	// Create two profiles.
	for _, name := range []string{"prof-1", "prof-2"} {
		body := []byte(`{"name":"` + name + `","locationUri":"hosted","type":"AWS.Freeform"}`)
		rec = doRequest(
			t,
			h,
			http.MethodPost,
			"/applications/"+app.ID+"/configurationprofiles",
			body,
		)
		require.Equal(t, http.StatusCreated, rec.Code)
	}

	// List.
	rec = doRequest(t, h, http.MethodGet, "/applications/"+app.ID+"/configurationprofiles", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_UpdateConfigurationProfile_HTTP(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"name":"upd-prof-app"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var app appconfig.Application
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))

	profileBody := []byte(`{"name":"old-name","locationUri":"hosted","type":"AWS.Freeform"}`)
	rec = doRequest(
		t,
		h,
		http.MethodPost,
		"/applications/"+app.ID+"/configurationprofiles",
		profileBody,
	)
	require.Equal(t, http.StatusCreated, rec.Code)

	var profile appconfig.ConfigurationProfile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &profile))

	// Update.
	rec = doRequest(
		t,
		h,
		http.MethodPatch,
		"/applications/"+app.ID+"/configurationprofiles/"+profile.ID,
		[]byte(`{"name":"new-name","description":"updated"}`),
	)
	assert.Equal(t, http.StatusOK, rec.Code)

	var updated appconfig.ConfigurationProfile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "new-name", updated.Name)
}

// TestHandler_UpdateConfigurationProfile_RetrievalRoleArnAndValidators verifies
// that UpdateConfigurationProfile applies RetrievalRoleArn and Validators
// when present (previously silently dropped -- the backend only accepted
// Name/Description) and preserves Description when it is omitted, matching
// real UpdateConfigurationProfileInput's optional members.
func TestHandler_UpdateConfigurationProfile_RetrievalRoleArnAndValidators(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"Name":"upd-prof-app2"}`))
	require.Equal(t, http.StatusCreated, rec.Code)

	var app appconfig.Application
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))

	profileBody := []byte(
		`{"Name":"old-name","Description":"keep-me","LocationUri":"hosted","Type":"AWS.Freeform"}`,
	)
	rec = doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/configurationprofiles", profileBody)
	require.Equal(t, http.StatusCreated, rec.Code)

	var profile appconfig.ConfigurationProfile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &profile))

	rec = doRequest(t, h, http.MethodPatch,
		"/applications/"+app.ID+"/configurationprofiles/"+profile.ID,
		[]byte(`{"RetrievalRoleArn":"arn:aws:iam::123456789012:role/retrieval",`+
			`"Validators":[{"Type":"JSON_SCHEMA","Content":"{}"}]}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	var updated appconfig.ConfigurationProfile
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, "arn:aws:iam::123456789012:role/retrieval", updated.RetrievalRoleArn,
		"RetrievalRoleArn must be applied, not silently dropped")
	require.Len(t, updated.Validators, 1, "Validators must be applied, not silently dropped")
	assert.Equal(t, "keep-me", updated.Description, "omitted Description must be preserved")
}

// TestKmsKeyIdentifierViaSDKClient proves CreateConfigurationProfileInput's
// real KmsKeyIdentifier member (appconfig@v1.48.4
// api_op_CreateConfigurationProfile.go) is no longer silently discarded and
// is echoed back on Create/Get/Update. KmsKeyArn is deliberately left
// unmodeled (no honest ARN-resolution source, see ConfigurationProfile's
// doc comment) so it is asserted absent, not present.
func TestKmsKeyIdentifierViaSDKClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestAppConfigClient(t, h)

	appOut, err := client.CreateApplication(t.Context(), &appconfigsdk.CreateApplicationInput{
		Name: aws.String("kms-id-app"),
	})
	require.NoError(t, err)

	createOut, err := client.CreateConfigurationProfile(t.Context(), &appconfigsdk.CreateConfigurationProfileInput{
		ApplicationId:    appOut.Id,
		Name:             aws.String("kms-id-profile"),
		LocationUri:      aws.String("hosted"),
		KmsKeyIdentifier: aws.String("alias/my-key"),
	})
	require.NoError(t, err)
	assert.Equal(t, "alias/my-key", aws.ToString(createOut.KmsKeyIdentifier))
	assert.Empty(t, aws.ToString(createOut.KmsKeyArn), "no honest ARN source, must stay absent")

	getOut, err := client.GetConfigurationProfile(t.Context(), &appconfigsdk.GetConfigurationProfileInput{
		ApplicationId:          appOut.Id,
		ConfigurationProfileId: createOut.Id,
	})
	require.NoError(t, err)
	assert.Equal(t, "alias/my-key", aws.ToString(getOut.KmsKeyIdentifier))

	updateOut, err := client.UpdateConfigurationProfile(t.Context(), &appconfigsdk.UpdateConfigurationProfileInput{
		ApplicationId:          appOut.Id,
		ConfigurationProfileId: createOut.Id,
		KmsKeyIdentifier:       aws.String("alias/rotated-key"),
	})
	require.NoError(t, err)
	assert.Equal(t, "alias/rotated-key", aws.ToString(updateOut.KmsKeyIdentifier))
}

// TestHandler_CreateConfigurationProfile_TagsAppliedInline verifies that
// Tags sent inline on CreateConfigurationProfileInput are visible via
// ListTagsForResource immediately after creation -- previously
// CreateConfigurationProfile's handler never bound or forwarded the Tags
// field at all, so tags set at create time silently vanished (bd
// gopherstack-lcan).
func TestHandler_CreateConfigurationProfile_TagsAppliedInline(t *testing.T) {
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
				[]byte(`{"Name":"tagged-prof-app-`+tt.name+`"}`))
			require.Equal(t, http.StatusCreated, appRec.Code)

			var app appconfig.Application
			require.NoError(t, json.Unmarshal(appRec.Body.Bytes(), &app))

			body, err := json.Marshal(map[string]any{
				"Name":        "tagged-prof-" + tt.name,
				"LocationUri": "hosted",
				"Type":        "AWS.Freeform",
				"Tags":        tt.tags,
			})
			require.NoError(t, err)

			rec := doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/configurationprofiles", body)
			require.Equal(t, http.StatusCreated, rec.Code)

			var profile appconfig.ConfigurationProfile
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &profile))

			resourceArn := "arn:aws:appconfig:us-east-1:123456789012:application/" +
				app.ID + "/configurationprofile/" + profile.ID
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

func TestHandler_ConfigProfile_HTTP_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		pathSuffix string
		wantStatus int
	}{
		{
			name:       "get profile not found",
			method:     http.MethodGet,
			pathSuffix: "/applications/nonexistent/configurationprofiles/prof-1",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "list profiles app not found",
			method:     http.MethodGet,
			pathSuffix: "/applications/nonexistent/configurationprofiles",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "delete profile not found",
			method:     http.MethodDelete,
			pathSuffix: "/applications/nonexistent/configurationprofiles/prof-1",
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

// seedExperimentDefinitionHTTP drives CreateApplication/CreateEnvironment/
// CreateConfigurationProfile through the real router path (doRequest, not
// the backend directly) so experiment definition handler tests exercise
// full wire-shape parsing end to end, returning the application/
// environment/configuration-profile IDs.
func seedExperimentDefinitionHTTP(t *testing.T, h *appconfig.Handler) (string, string, string) {
	t.Helper()

	appRec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"Name":"exp-http-app"}`))
	require.Equal(t, http.StatusCreated, appRec.Code)

	var app struct {
		ID string `json:"Id"`
	}
	require.NoError(t, json.Unmarshal(appRec.Body.Bytes(), &app))

	envRec := doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/environments",
		[]byte(`{"Name":"exp-http-env"}`))
	require.Equal(t, http.StatusCreated, envRec.Code)

	var env struct {
		ID string `json:"Id"`
	}
	require.NoError(t, json.Unmarshal(envRec.Body.Bytes(), &env))

	profRec := doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/configurationprofiles",
		[]byte(`{"Name":"exp-http-profile","LocationUri":"hosted","Type":"AWS.AppConfig.FeatureFlags"}`))
	require.Equal(t, http.StatusCreated, profRec.Code)

	var prof struct {
		ID string `json:"Id"`
	}
	require.NoError(t, json.Unmarshal(profRec.Body.Bytes(), &prof))

	return app.ID, env.ID, prof.ID
}

// TestHandler_ExperimentDefinition_CRUD drives CreateExperimentDefinition/
// GetExperimentDefinition/ListExperimentDefinitions/
// UpdateExperimentDefinition/DeleteExperimentDefinition through the real
// router path, asserting the REST-JSON wire shapes field-diffed against
// aws-sdk-go-v2/service/appconfig@v1.48.4's Create/Get/List/Update/
// DeleteExperimentDefinitionOutput.
func TestHandler_ExperimentDefinition_CRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	appID, envID, profID := seedExperimentDefinitionHTTP(t, h)

	createBody := `{
		"Name": "http-def",
		"AudienceRule": "true",
		"FlagKey": "flag1",
		"EnvironmentIdentifier": "` + envID + `",
		"ConfigurationProfileIdentifier": "` + profID + `",
		"Control": {"FlagValue": {"Enabled": false}, "Weight": 50},
		"Treatments": [{"FlagValue": {"Enabled": true}, "Weight": 50}]
	}`
	createRec := doRequest(t, h, http.MethodPost, "/applications/"+appID+"/experimentdefinitions", []byte(createBody))
	require.Equal(t, http.StatusCreated, createRec.Code)

	var created appconfig.ExperimentDefinition
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, appID, created.ApplicationID)
	assert.Equal(t, envID, created.EnvironmentID)
	assert.Equal(t, profID, created.ConfigurationProfileID)
	assert.Equal(t, "IDLE", created.Status)
	assert.Equal(t, "Control", created.Control.Key)
	require.Len(t, created.Treatments, 1)
	assert.Equal(t, "Treatment1", created.Treatments[0].Key)

	getRec := doRequest(t, h, http.MethodGet, "/applications/"+appID+"/experimentdefinitions/"+created.ID, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var got appconfig.ExperimentDefinition
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &got))
	assert.Equal(t, created.ID, got.ID)

	listRec := doRequest(t, h, http.MethodGet, "/experimentdefinitions?application_identifier="+appID, nil)
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp struct {
		Items []appconfig.ExperimentDefinition `json:"Items"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	require.Len(t, listResp.Items, 1)
	assert.Equal(t, created.ID, listResp.Items[0].ID)

	updateRec := doRequest(t, h, http.MethodPatch, "/applications/"+appID+"/experimentdefinitions/"+created.ID,
		[]byte(`{"AudienceRule":"country == 'US'"}`))
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updated appconfig.ExperimentDefinition
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updated))
	assert.Equal(t, "country == 'US'", updated.AudienceRule)

	deleteRec := doRequest(t, h, http.MethodDelete, "/applications/"+appID+"/experimentdefinitions/"+created.ID, nil)
	require.Equal(t, http.StatusNoContent, deleteRec.Code)

	afterDeleteRec := doRequest(t, h, http.MethodGet, "/applications/"+appID+"/experimentdefinitions/"+created.ID, nil)
	require.Equal(t, http.StatusOK, afterDeleteRec.Code, "default delete_type archives rather than destroying")

	var afterDelete appconfig.ExperimentDefinition
	require.NoError(t, json.Unmarshal(afterDeleteRec.Body.Bytes(), &afterDelete))
	assert.Equal(t, "ARCHIVED", afterDelete.Status)

	// FlagKey is validated against the profile's actual feature flag
	// content (see feature_flags.go) once one has been uploaded, through
	// the real router path: POST .../hostedconfigurationversions carries
	// the raw content as its body (httpPayload, not a JSON-wrapped field).
	hcvRec := doRequest(t, h, http.MethodPost,
		"/applications/"+appID+"/configurationprofiles/"+profID+"/hostedconfigurationversions",
		[]byte(`{"version":"1","flags":{"flag1":{"name":"Flag One"}},"values":{"flag1":{"enabled":true}}}`))
	require.Equal(t, http.StatusCreated, hcvRec.Code)

	unknownFlagBody := `{
		"Name": "http-def-unknown-flag",
		"AudienceRule": "true",
		"FlagKey": "flag2",
		"EnvironmentIdentifier": "` + envID + `",
		"ConfigurationProfileIdentifier": "` + profID + `",
		"Control": {"FlagValue": {"Enabled": false}, "Weight": 50},
		"Treatments": [{"FlagValue": {"Enabled": true}, "Weight": 50}]
	}`
	unknownFlagRec := doRequest(
		t, h, http.MethodPost, "/applications/"+appID+"/experimentdefinitions", []byte(unknownFlagBody),
	)
	assert.Equal(t, http.StatusBadRequest, unknownFlagRec.Code)

	knownFlagBody := `{
		"Name": "http-def-known-flag",
		"AudienceRule": "true",
		"FlagKey": "flag1",
		"EnvironmentIdentifier": "` + envID + `",
		"ConfigurationProfileIdentifier": "` + profID + `",
		"Control": {"FlagValue": {"Enabled": false}, "Weight": 50},
		"Treatments": [{"FlagValue": {"Enabled": true}, "Weight": 50}]
	}`
	knownFlagRec := doRequest(
		t, h, http.MethodPost, "/applications/"+appID+"/experimentdefinitions", []byte(knownFlagBody),
	)
	assert.Equal(t, http.StatusCreated, knownFlagRec.Code)
}

// TestHandler_ExperimentDefinition_HTTP_Errors covers not-found and
// validation error status codes across the experiment-definition routes.
func TestHandler_ExperimentDefinition_HTTP_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		pathSuffix string
		body       []byte
		wantStatus int
	}{
		{
			name:       "get definition not found",
			method:     http.MethodGet,
			pathSuffix: "/applications/nonexistent/experimentdefinitions/def-1",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "delete definition not found",
			method:     http.MethodDelete,
			pathSuffix: "/applications/nonexistent/experimentdefinitions/def-1",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "create missing required fields",
			method:     http.MethodPost,
			pathSuffix: "/applications/nonexistent/experimentdefinitions",
			body:       []byte(`{}`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "list account-wide with no filters",
			method:     http.MethodGet,
			pathSuffix: "/experimentdefinitions",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.method, tt.pathSuffix, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_DeleteConfigurationProfile_DeletionProtectionCheck proves the
// "X-Amzn-Deletion-Protection-Check" header (appconfig@v1.48.4
// serializers.go:1121, DeleteConfigurationProfileInput.DeletionProtectionCheck)
// is actually read: a recognized types.DeletionProtectionCheck value still
// deletes (newTestHandler's fixture never enables account-level deletion
// protection nor wires an AppConfigData sibling, so checkDeletionProtectionLocked
// allows every one of these regardless of header -- see
// TestHandler_DeleteConfigurationProfile_DeletionProtectionCheck_EnforcesRecentRead
// in deletion_protection_test.go for the actual blocking path,
// gopherstack-z4v1), but an unrecognized value -- which real AppConfig would
// reject -- now gets a BadRequestException instead of being silently ignored.
func TestHandler_DeleteConfigurationProfile_DeletionProtectionCheck(t *testing.T) {
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

			rec := doRequest(t, h, http.MethodPost, "/applications", []byte(`{"name":"dpc-prof-app"}`))
			require.Equal(t, http.StatusCreated, rec.Code)

			var app appconfig.Application
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &app))

			rec = doRequest(t, h, http.MethodPost, "/applications/"+app.ID+"/configurationprofiles",
				[]byte(`{"name":"dpc-profile","locationUri":"hosted","type":"AWS.Freeform"}`))
			require.Equal(t, http.StatusCreated, rec.Code)

			var profile appconfig.ConfigurationProfile
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &profile))

			path := "/applications/" + app.ID + "/configurationprofiles/" + profile.ID

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
				assert.Equal(t, http.StatusNotFound, getRec.Code, "profile should have been deleted")
			} else {
				assert.Equal(t, http.StatusOK, getRec.Code, "profile must survive a rejected delete")
			}
		})
	}
}
