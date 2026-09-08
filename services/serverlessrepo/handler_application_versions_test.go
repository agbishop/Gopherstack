package serverlessrepo_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/serverlessrepo"
)

func TestHandler_CreateApplicationVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		path     string
		wantCode int
	}{
		{
			name: "creates version successfully",
			path: "/applications/my-app/versions/1.0.0",
			body: map[string]any{
				"sourceCodeUrl": "https://github.com/example/my-app",
			},
			wantCode: http.StatusCreated,
		},
		{
			// CreateApplicationVersion models no NotFoundException (deserializers.go
			// awsRestjson1_deserializeOpErrorCreateApplicationVersion): an unknown
			// ApplicationId is a BadRequestException, not a 404.
			name:     "app not found returns bad request",
			path:     "/applications/not-found/versions/1.0.0",
			body:     map[string]any{"sourceCodeUrl": "https://example.com"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "duplicate version returns conflict",
			path:     "/applications/my-app/versions/1.0.0",
			body:     map[string]any{"sourceCodeUrl": "https://example.com"},
			wantCode: http.StatusConflict,
		},
		{
			name:     "missing source URL returns bad request",
			path:     "/applications/my-app/versions/2.0.0",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "0.1.0", nil, "", "", "")
			require.NoError(t, err)

			if tt.wantCode == http.StatusConflict {
				_, err = h.Backend.CreateApplicationVersion("my-app", "1.0.0", "https://example.com", "")
				require.NoError(t, err)
			}

			rec := doServerlessRepoRequest(t, h, http.MethodPut, tt.path, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusCreated {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "1.0.0", resp["semanticVersion"])
				assert.NotNil(t, resp["parameterDefinitions"])
				assert.NotNil(t, resp["requiredCapabilities"])
			}
		})
	}
}

func TestCreateApplicationVersion_Returns201(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "0.1.0", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPut, "/applications/my-app/versions/1.0.0", map[string]any{
		"sourceCodeUrl": "https://github.com/example",
	})
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestCreateApplicationVersion_HasRequiredFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "0.1.0", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPut, "/applications/my-app/versions/1.0.0", map[string]any{
		"sourceCodeUrl": "https://github.com/example",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp["parameterDefinitions"], "parameterDefinitions must be present")
	assert.NotNil(t, resp["requiredCapabilities"], "requiredCapabilities must be present")
	assert.Equal(t, true, resp["resourcesSupported"])
}

func TestCreateApplicationVersion_MissingURLs(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "0.1.0", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPut, "/applications/my-app/versions/1.0.0", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateApplicationVersion_ArchiveURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body            map[string]any
		name            string
		semanticVersion string
	}{
		{
			name:            "archive only",
			semanticVersion: "1.0.0",
			body:            map[string]any{"sourceCodeArchiveUrl": "s3://bucket/archive.zip"},
		},
		{
			name:            "archive with provided template",
			semanticVersion: "1.0.1",
			body: map[string]any{
				"sourceCodeArchiveUrl": "s3://bucket/archive.zip",
				"templateUrl":          "s3://bucket/template.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreateApplication("versions-app", "desc", "author", "", "", nil, "", "", "")
			require.NoError(t, err)

			path := "/applications/versions-app/versions/" + tt.semanticVersion
			rec := doServerlessRepoRequest(t, h, http.MethodPut, path, tt.body)
			require.Equal(t, http.StatusCreated, rec.Code)

			var response map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
			assert.Equal(t, "s3://bucket/archive.zip", response["sourceCodeArchiveUrl"])
			assert.NotEmpty(t, response["templateUrl"])
		})
	}
}

func TestCreateApplicationVersion_TemplateURLOnly(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("tv-only-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPut,
		"/applications/tv-only-app/versions/1.0.0",
		map[string]any{"templateUrl": "s3://bucket/tmpl.yaml"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "s3://bucket/tmpl.yaml", resp["templateUrl"])
}

// TestCreateApplicationVersion_TemplateBody locks CreateApplicationVersion's handling of the
// templateBody wire field (aws-sdk-go-v2's CreateApplicationVersionInput.TemplateBody): it is
// accepted as an alternative to templateUrl, satisfies the "at least one of sourceCodeUrl,
// sourceCodeArchiveUrl or templateUrl" requirement on its own, and synthesizes a templateUrl
// as the real service would after uploading the inline content to S3. Supplying both
// templateBody and templateUrl is a BadRequestException per the real API doc.
func TestCreateApplicationVersion_TemplateBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		appName  string
		wantCode int
	}{
		{
			name:     "templateBody alone satisfies the required-field check and synthesizes templateUrl",
			appName:  "tbody-alone-app",
			body:     map[string]any{"templateBody": "AWSTemplateFormatVersion: '2010-09-09'"},
			wantCode: http.StatusCreated,
		},
		{
			name:    "templateBody and templateUrl together is a bad request",
			appName: "tbody-both-app",
			body: map[string]any{
				"templateBody": "AWSTemplateFormatVersion: '2010-09-09'",
				"templateUrl":  "s3://bucket/template.yaml",
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreateApplication(tt.appName, "desc", "author", "", "", nil, "", "", "")
			require.NoError(t, err)

			path := "/applications/" + tt.appName + "/versions/1.0.0"
			rec := doServerlessRepoRequest(t, h, http.MethodPut, path, tt.body)
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusCreated {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["templateUrl"])
			}
		})
	}
}

func TestCreateApplicationVersion_DuplicateReturns409(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("dup-ver-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	path := "/applications/dup-ver-app/versions/1.0.0"
	body := map[string]any{"sourceCodeUrl": "https://example.com"}

	rec1 := doServerlessRepoRequest(t, h, http.MethodPut, path, body)
	require.Equal(t, http.StatusCreated, rec1.Code)

	rec2 := doServerlessRepoRequest(t, h, http.MethodPut, path, body)
	assert.Equal(t, http.StatusConflict, rec2.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &errResp))
	assert.Equal(t, "ConflictException", errResp["__type"])
}

func TestCreateApplicationVersion_InvalidSemanticVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
	}{
		{name: "no_dots", version: "1"},
		{name: "one_dot", version: "1.0"},
		{name: "v_prefix", version: "v1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "", nil, "", "", "")
			require.NoError(t, err)

			path := "/applications/my-app/versions/" + tt.version
			rec := doServerlessRepoRequest(t, h, http.MethodPut, path, map[string]any{
				"sourceCodeUrl": "https://github.com/example",
			})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestCreateApplicationVersion_GeneratesTemplateURL(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodPut, "/applications/my-app/versions/1.0.0", map[string]any{
		"sourceCodeUrl": "https://github.com/example",
	})
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["templateUrl"], "templateUrl should be auto-generated from sourceCodeUrl")
}

func TestCreateApplicationVersion_UpdatesLatestVersion(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("track-app", "desc", "author", "", "1.0.0", nil, "", "", "")
	require.NoError(t, err)

	// Create initial version with source URL so it's stored in appVersions
	_, err = h.Backend.CreateApplicationVersion("track-app", "1.0.0", "https://example.com", "")
	require.NoError(t, err)

	// Create newer version
	_, err = h.Backend.CreateApplicationVersion("track-app", "2.0.0", "https://example.com", "")
	require.NoError(t, err)

	// GetApplication without semanticVersion query should return the latest (2.0.0)
	rec := doServerlessRepoRequest(t, h, http.MethodGet, "/applications/track-app", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	version, ok := resp["version"].(map[string]any)
	require.True(t, ok, "version field must be present")
	assert.Equal(t, "2.0.0", version["semanticVersion"], "should return latest created version")
}

func TestCreateApplicationVersion_FullVersionDataInGetApplication(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("fv-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	_, err = h.Backend.CreateApplicationVersion("fv-app", "3.1.4", "https://github.com/example/repo", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodGet, "/applications/fv-app", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	version, ok := resp["version"].(map[string]any)
	require.True(t, ok, "version must be embedded in GetApplication response")
	assert.Equal(t, "3.1.4", version["semanticVersion"])
	assert.NotEmpty(t, version["templateUrl"], "templateUrl must be present after version creation")
	assert.Equal(t, "https://github.com/example/repo", version["sourceCodeUrl"])
	assert.True(t, version["resourcesSupported"].(bool))
}

func TestHandler_ListApplicationVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*serverlessrepo.Handler)
		name     string
		appName  string
		wantLen  int
		wantCode int
	}{
		{
			name:     "empty versions list",
			appName:  "my-app",
			wantLen:  0,
			wantCode: http.StatusOK,
		},
		{
			name:    "list with versions",
			appName: "my-app",
			setup: func(h *serverlessrepo.Handler) {
				_, _ = h.Backend.CreateApplicationVersion("my-app", "1.0.0", "https://example.com", "")
				_, _ = h.Backend.CreateApplicationVersion("my-app", "2.0.0", "https://example.com", "")
			},
			wantLen:  2,
			wantCode: http.StatusOK,
		},
		{
			name:     "app not found returns 404",
			appName:  "not-found",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "0.1.0", nil, "", "", "")
			require.NoError(t, err)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doServerlessRepoRequest(t, h, http.MethodGet, "/applications/"+tt.appName+"/versions", nil)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				versions, ok := resp["versions"].([]any)
				require.True(t, ok)
				assert.Len(t, versions, tt.wantLen)
			}
		})
	}
}

func TestListApplicationVersions_SemanticVersionFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	for _, v := range []string{"1.0.0", "2.0.0", "3.0.0"} {
		_, err = h.Backend.CreateApplicationVersion("my-app", v, "https://github.com/example", "")
		require.NoError(t, err)
	}

	rec := doServerlessRepoRequest(t, h, http.MethodGet, "/applications/my-app/versions?semanticVersion=2.0.0", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	versions, ok := resp["versions"].([]any)
	require.True(t, ok)
	assert.Len(t, versions, 1, "filter should return only 1 matching version")
	assert.Equal(t, "2.0.0", versions[0].(map[string]any)["semanticVersion"])
}

func TestListApplicationVersions_MaxItems(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	for _, v := range []string{"1.0.0", "2.0.0", "3.0.0"} {
		_, err = h.Backend.CreateApplicationVersion("my-app", v, "https://github.com/example", "")
		require.NoError(t, err)
	}

	rec := doServerlessRepoRequest(t, h, http.MethodGet, "/applications/my-app/versions?maxItems=2", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	versions, ok := resp["versions"].([]any)
	require.True(t, ok)
	assert.Len(t, versions, 2)
	assert.NotNil(t, resp["nextToken"])
}

func TestListApplicationVersions_PaginationNextToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("pag-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	for _, v := range []string{"1.0.0", "2.0.0", "3.0.0", "4.0.0"} {
		_, err = h.Backend.CreateApplicationVersion("pag-app", v, "https://example.com", "")
		require.NoError(t, err)
	}

	// Page 1
	rec := doServerlessRepoRequest(t, h, http.MethodGet, "/applications/pag-app/versions?maxItems=2", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var r1 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &r1))
	v1 := r1["versions"].([]any)
	assert.Len(t, v1, 2)
	nt, ok := r1["nextToken"].(string)
	require.True(t, ok)

	// Page 2
	rec2 := doServerlessRepoRequest(
		t, h, http.MethodGet,
		"/applications/pag-app/versions?maxItems=2&nextToken="+nt,
		nil,
	)
	require.Equal(t, http.StatusOK, rec2.Code)

	var r2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &r2))
	v2 := r2["versions"].([]any)
	assert.Len(t, v2, 2)
	assert.Nil(t, r2["nextToken"])
}

// TestListApplicationVersions_SummaryShape locks the ListApplicationVersions summary to
// exactly the real AWS SAR VersionSummary shape (applicationId, creationTime,
// semanticVersion, sourceCodeUrl -- see
// aws-sdk-go-v2/service/serverlessapplicationrepository/types.VersionSummary). It must NOT
// include resourcesSupported: that field only exists on the full Version shape returned by
// GetApplication/CreateApplication/CreateApplicationVersion, not on VersionSummary.
func TestListApplicationVersions_SummaryShape(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("vs-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	_, err = h.Backend.CreateApplicationVersion("vs-app", "1.0.0", "https://example.com", "")
	require.NoError(t, err)

	rec := doServerlessRepoRequest(t, h, http.MethodGet, "/applications/vs-app/versions", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	versions, ok := resp["versions"].([]any)
	require.True(t, ok)
	require.Len(t, versions, 1)

	v := versions[0].(map[string]any)
	assert.Equal(t, "1.0.0", v["semanticVersion"])
	assert.Equal(t, "https://example.com", v["sourceCodeUrl"])
	assert.NotEmpty(t, v["applicationId"])
	assert.NotEmpty(t, v["creationTime"])

	_, exists := v["resourcesSupported"]
	assert.False(t, exists, "resourcesSupported is not part of the real VersionSummary shape and must not be emitted")
}

func TestListApplicationVersions_ARNForm(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("ver-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	_, err = h.Backend.CreateApplicationVersion("ver-app", "1.0.0", "https://github.com/example", "")
	require.NoError(t, err)

	path := arnPathFor("ver-app") + "/versions"
	rec := doServerlessRepoRequestEncoded(t, h, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	versions, ok := resp["versions"].([]any)
	require.True(t, ok)
	assert.Len(t, versions, 1)
}

func TestAddVersionInternal(t *testing.T) {
	t.Parallel()

	b := serverlessrepo.NewInMemoryBackend(testAccountID, "us-east-1")
	b.AddApplicationInternal("my-app", "desc", "author")
	v := b.AddVersionInternal("my-app", "1.0.0")
	require.NotNil(t, v)
	assert.Equal(t, "1.0.0", v.SemanticVersion)
	assert.Equal(t, 1, serverlessrepo.VersionCount(b, "my-app"))
}

func TestAddVersionInternal_MissingApp(t *testing.T) {
	t.Parallel()

	b := serverlessrepo.NewInMemoryBackend(testAccountID, "us-east-1")
	v := b.AddVersionInternal("non-existent", "1.0.0")
	assert.Nil(t, v)
}

func TestAddVersionInternal_SeedHelper(t *testing.T) {
	t.Parallel()

	b := serverlessrepo.NewInMemoryBackend(testAccountID, "us-east-1")
	_, err := b.CreateApplication("my-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	v := b.AddVersionInternal("my-app", "3.0.0")
	require.NotNil(t, v)
	assert.Equal(t, "3.0.0", v.SemanticVersion)
	assert.Equal(t, 1, serverlessrepo.VersionCount(b, "my-app"))
}

func TestVersionResponse_ResourcesSupported(t *testing.T) {
	t.Parallel()

	b := serverlessrepo.NewInMemoryBackend(testAccountID, "us-east-1")
	_, _ = b.CreateApplication("my-app", "desc", "author", "", "", nil, "", "", "")
	v, err := b.CreateApplicationVersion("my-app", "1.0.0", "https://example.com", "")
	require.NoError(t, err)

	assert.True(t, v.ResourcesSupported)
	assert.NotNil(t, v.ParameterDefinitions)
	assert.NotNil(t, v.RequiredCapabilities)
}

func TestGetApplicationVersion_NotFound(t *testing.T) {
	t.Parallel()

	b := serverlessrepo.NewInMemoryBackend(testAccountID, "us-east-1")
	_, err := b.CreateApplication("my-app", "desc", "author", "", "", nil, "", "", "")
	require.NoError(t, err)

	_, err = b.GetApplicationVersion("my-app", "99.99.99")
	assert.Error(t, err, "GetApplicationVersion for unknown version should return error")
}
