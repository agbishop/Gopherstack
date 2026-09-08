package serverlessrepo_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/serverlessrepo"
)

func TestHandler_CreateCloudFormationTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		appName  string
		wantCode int
	}{
		{
			name:    "creates template successfully",
			appName: "my-app",
			body: map[string]any{
				"semanticVersion": "1.0.0",
			},
			wantCode: http.StatusCreated,
		},
		{
			name:     "app not found returns 404",
			appName:  "not-found",
			body:     map[string]any{},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "1.0.0", nil, "", "", "")
			require.NoError(t, err)

			rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/"+tt.appName+"/templates", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusCreated {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["templateId"])
				assert.Equal(t, "ACTIVE", resp["status"])
			}
		})
	}
}

func TestCreateCloudFormationTemplate_Returns201(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "1.0.0", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/my-app/templates", map[string]any{
		"semanticVersion": "1.0.0",
	})
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestCreateCloudFormationTemplate_HasTemplateURL(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "1.0.0", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/my-app/templates", map[string]any{})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["templateUrl"])
	assert.NotEmpty(t, resp["expirationTime"])
}

func TestCreateCloudFormationTemplate_OmitsSemanticVersionWhenEmpty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("tmpl-no-sv", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/tmpl-no-sv/templates",
		map[string]any{}) // no semanticVersion
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	_, hasSV := resp["semanticVersion"]
	assert.False(t, hasSV, "semanticVersion must be absent from response when not provided")
	assert.NotEmpty(t, resp["templateId"])
	assert.NotEmpty(t, resp["templateUrl"])
}

func TestCreateCloudFormationTemplate_IncludesSemanticVersionWhenProvided(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("tmpl-sv", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/tmpl-sv/templates",
		map[string]any{"semanticVersion": "2.0.0"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "2.0.0", resp["semanticVersion"])
}

func TestHandler_GetCloudFormationTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		appName    string
		templateID string
		wantCode   int
	}{
		{
			name:     "gets template successfully",
			appName:  "my-app",
			wantCode: http.StatusOK,
		},
		{
			name:       "template not found returns 404",
			appName:    "my-app",
			templateID: "non-existent-template",
			wantCode:   http.StatusNotFound,
		},
		{
			name:       "app not found returns 404",
			appName:    "not-found",
			templateID: "some-template",
			wantCode:   http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "1.0.0", nil, "", "", "")
			require.NoError(t, err)

			templateID := tt.templateID

			if tt.wantCode == http.StatusOK {
				tmpl, tmplErr := h.Backend.CreateCloudFormationTemplate("my-app", "1.0.0")
				require.NoError(t, tmplErr)
				templateID = tmpl.TemplateID
			}

			rec := doServerlessRepoRequest(
				t,
				h,
				http.MethodGet,
				"/applications/"+tt.appName+"/templates/"+templateID,
				nil,
			)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["templateId"])
				assert.Equal(t, "ACTIVE", resp["status"])
			}
		})
	}
}

func TestGetCloudFormationTemplate_ActiveStatus(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "1.0.0", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/my-app/templates", map[string]any{})
	require.Equal(t, http.StatusCreated, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	templateID := createResp["templateId"].(string)

	rec2 := doServerlessRepoRequest(t, h, http.MethodGet,
		"/applications/my-app/templates/"+templateID, nil)
	require.Equal(t, http.StatusOK, rec2.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &getResp))
	assert.Equal(t, "ACTIVE", getResp["status"])
}

func TestGetCloudFormationTemplate_OmitsSemanticVersionWhenEmpty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("get-tmpl-no-sv", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	createRec := doServerlessRepoRequest(t, h, http.MethodPost,
		"/applications/get-tmpl-no-sv/templates", map[string]any{})
	require.Equal(t, http.StatusCreated, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	templateID := createResp["templateId"].(string)

	getRec := doServerlessRepoRequest(t, h, http.MethodGet,
		"/applications/get-tmpl-no-sv/templates/"+templateID, nil)
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	_, hasSV := getResp["semanticVersion"]
	assert.False(t, hasSV, "semanticVersion must be absent from GetCloudFormationTemplate when not set")
}

func TestGetCloudFormationTemplate_ExpiredStatus(t *testing.T) {
	t.Parallel()

	b := serverlessrepo.NewInMemoryBackend(testAccountID, "us-east-1")
	_, err := b.CreateApplication("exp-tmpl-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	expired := serverlessrepo.AddExpiredTemplateInternal(b, "exp-tmpl-app", "1.0.0")
	require.NotNil(t, expired)

	h := serverlessrepo.NewHandler(b)
	rec := doServerlessRepoRequest(t, h, http.MethodGet,
		"/applications/exp-tmpl-app/templates/"+expired.TemplateID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "EXPIRED", resp["status"])
	assert.Equal(t, "1.0.0", resp["semanticVersion"])
}

func TestGetCloudFormationTemplate_Fields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("tmpl-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/tmpl-app/templates", map[string]any{
		"semanticVersion": "1.0.0",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	templateID := createResp["templateId"].(string)

	rec2 := doServerlessRepoRequest(t, h, http.MethodGet,
		"/applications/tmpl-app/templates/"+templateID, nil)
	require.Equal(t, http.StatusOK, rec2.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &getResp))
	assert.Equal(t, "ACTIVE", getResp["status"])
	assert.Equal(t, "1.0.0", getResp["semanticVersion"])
	assert.NotEmpty(t, getResp["templateUrl"])
	assert.NotEmpty(t, getResp["creationTime"])
	assert.NotEmpty(t, getResp["expirationTime"])
	assert.NotEmpty(t, getResp["applicationId"])
}

func TestHandler_CreateCloudFormationChangeSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		appName  string
		wantCode int
	}{
		{
			name:    "creates change set successfully",
			appName: "my-app",
			body: map[string]any{
				"stackName":       "my-stack",
				"semanticVersion": "1.0.0",
			},
			wantCode: http.StatusCreated,
		},
		{
			name:    "creates change set with custom name",
			appName: "my-app",
			body: map[string]any{
				"stackName":     "my-stack",
				"changeSetName": "my-changeset",
			},
			wantCode: http.StatusCreated,
		},
		{
			name:     "missing stackName returns bad request",
			appName:  "my-app",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			// CreateCloudFormationChangeSet models no NotFoundException (deserializers.go
			// awsRestjson1_deserializeOpErrorCreateCloudFormationChangeSet): an unknown
			// ApplicationId is a BadRequestException, not a 404.
			name:    "app not found returns bad request",
			appName: "not-found",
			body: map[string]any{
				"stackName": "my-stack",
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "1.0.0", nil, "", "", "")
			require.NoError(t, err)

			rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/"+tt.appName+"/changesets", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusCreated {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["changeSetId"])
				assert.NotEmpty(t, resp["stackId"])
			}
		})
	}
}

func TestCreateCloudFormationChangeSet_Returns201(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "1.0.0", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/my-app/changesets", map[string]any{
		"stackName": "my-stack",
	})
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestCreateCloudFormationChangeSet_CustomName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "1.0.0", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/my-app/changesets", map[string]any{
		"stackName":     "my-stack",
		"changeSetName": "my-custom-changeset",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["changeSetId"].(string), "my-custom-changeset")
}

func TestCreateCloudFormationChangeSet_CapabilitiesAndTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		capability string
		wantStatus int
	}{
		{name: "iam", capability: "CAPABILITY_IAM", wantStatus: http.StatusCreated},
		{name: "named iam", capability: "CAPABILITY_NAMED_IAM", wantStatus: http.StatusCreated},
		{name: "auto expand", capability: "CAPABILITY_AUTO_EXPAND", wantStatus: http.StatusCreated},
		{name: "resource policy", capability: "CAPABILITY_RESOURCE_POLICY", wantStatus: http.StatusCreated},
		{name: "invalid", capability: "CAPABILITY_UNKNOWN", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreateApplication("deploy-app", "desc", "author", "", "", nil, "", "", "")
			require.NoError(t, err)

			rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/deploy-app/changesets", map[string]any{
				"stackName":    "stack",
				"capabilities": []string{tt.capability},
				"tags":         []map[string]string{{"key": "stage", "value": "test"}},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusCreated {
				assert.Contains(t, string(h.Snapshot(t.Context())), `"capabilities":["`+tt.capability+`"]`)
				assert.Contains(t, string(h.Snapshot(t.Context())), `"tags":[{"key":"stage","value":"test"}]`)
			}
		})
	}
}

// TestCreateCloudFormationChangeSet_TemplateID locks cross-validation of the templateId
// wire field on CreateCloudFormationChangeSet (aws-sdk-go-v2's
// CreateCloudFormationChangeSetInput.TemplateId, "The UUID returned by
// CreateCloudFormationTemplate"): a templateId that was actually returned by a prior
// CreateCloudFormationTemplate call for the same application is accepted, while an unknown
// templateId, or one that belongs to a different application, is rejected as a
// BadRequestException -- CreateCloudFormationChangeSet models no NotFoundException
// (deserializers.go awsRestjson1_deserializeOpErrorCreateCloudFormationChangeSet).
func TestCreateCloudFormationChangeSet_TemplateID(t *testing.T) {
	t.Parallel()

	t.Run("valid templateId from a prior CreateCloudFormationTemplate is accepted", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		_, err := h.Backend.CreateApplication("cs-tid-app", "desc", "author", "", "", nil, "", "", "")
		require.NoError(t, err)

		tmpl, err := h.Backend.CreateCloudFormationTemplate("cs-tid-app", "")
		require.NoError(t, err)

		rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/cs-tid-app/changesets", map[string]any{
			"stackName":  "my-stack",
			"templateId": tmpl.TemplateID,
		})
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("unknown templateId is rejected as bad request", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		_, err := h.Backend.CreateApplication("cs-tid-unknown-app", "desc", "author", "", "", nil, "", "", "")
		require.NoError(t, err)

		rec := doServerlessRepoRequest(
			t, h, http.MethodPost, "/applications/cs-tid-unknown-app/changesets", map[string]any{
				"stackName":  "my-stack",
				"templateId": "does-not-exist",
			})
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "BadRequestException", resp["__type"])
	})

	t.Run("templateId belonging to a different application is rejected as bad request", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		_, err := h.Backend.CreateApplication("cs-tid-owner-app", "desc", "author", "", "", nil, "", "", "")
		require.NoError(t, err)
		_, err = h.Backend.CreateApplication("cs-tid-other-app", "desc", "author", "", "", nil, "", "", "")
		require.NoError(t, err)

		tmpl, err := h.Backend.CreateCloudFormationTemplate("cs-tid-owner-app", "")
		require.NoError(t, err)

		rec := doServerlessRepoRequest(
			t, h, http.MethodPost, "/applications/cs-tid-other-app/changesets", map[string]any{
				"stackName":  "my-stack",
				"templateId": tmpl.TemplateID,
			})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestCreateCloudFormationChangeSet_OmitsSemanticVersionWhenEmpty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("cs-no-sv", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/cs-no-sv/changesets",
		map[string]any{"stackName": "my-stack"}) // no semanticVersion
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	_, hasSV := resp["semanticVersion"]
	assert.False(t, hasSV, "semanticVersion must be absent from response when not provided")
	assert.NotEmpty(t, resp["changeSetId"])
	assert.NotEmpty(t, resp["stackId"])
}

func TestCreateCloudFormationChangeSet_IncludesSemanticVersionWhenProvided(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("cs-sv", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/cs-sv/changesets",
		map[string]any{"stackName": "stack", "semanticVersion": "3.1.4"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "3.1.4", resp["semanticVersion"])
}

func TestCreateCloudFormationChangeSet_ResponseFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("cf-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPost, "/applications/cf-app/changesets", map[string]any{
		"stackName":       "my-stack",
		"semanticVersion": "1.0.0",
		"capabilities":    []string{"CAPABILITY_IAM"},
		"tags":            []map[string]string{{"key": "env", "value": "test"}},
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["changeSetId"])
	assert.NotEmpty(t, resp["stackId"])
	assert.Equal(t, "1.0.0", resp["semanticVersion"])
	assert.Contains(t, resp["changeSetId"].(string), "cloudformation")
	assert.Contains(t, resp["stackId"].(string), "cloudformation")
}

func TestCreateCloudFormationChangeSet_ARNForm(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("cs-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	path := arnPathFor("cs-app") + "/changesets"
	rec := doServerlessRepoRequestEncoded(t, h, http.MethodPost, path, map[string]any{
		"stackName": "test-stack",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["changeSetId"].(string), "cloudformation", "changeSetId should be a CloudFormation ARN")
	assert.Contains(t, resp["stackId"].(string), "cloudformation", "stackId should be a CloudFormation ARN")
}
