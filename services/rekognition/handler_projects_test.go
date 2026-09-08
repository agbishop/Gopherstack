package rekognition_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rekognition"
)

func TestProjects(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		body     any
		setup    func(h *rekognition.Handler)
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "CreateProject returns ARN",
			action:   "CreateProject",
			body:     map[string]any{"ProjectName": "my-project"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Contains(t, resp["ProjectArn"], "arn:aws:rekognition:")
				assert.Contains(t, resp["ProjectArn"], "project/my-project")
			},
		},
		{
			name:     "CreateProject missing name returns error",
			action:   "CreateProject",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DeleteProject returns DELETING",
			action: "DeleteProject",
			setup: func(h *rekognition.Handler) {
				rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "del-proj"})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.NotEmpty(t, resp["message"])
			},
		},
		{
			name:     "DeleteProject missing ARN returns error",
			action:   "DeleteProject",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DescribeProjects returns projects",
			action: "DescribeProjects",
			setup: func(h *rekognition.Handler) {
				doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "desc-proj"})
			},
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				descriptions, ok := resp["ProjectDescriptions"].([]any)
				require.True(t, ok)
				assert.GreaterOrEqual(t, len(descriptions), 1)
			},
		},
		{
			name:     "DescribeProjects empty list returns empty slice",
			action:   "DescribeProjects",
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				descriptions, ok := resp["ProjectDescriptions"].([]any)
				require.True(t, ok)
				assert.Empty(t, descriptions)
			},
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			if tc.setup != nil {
				tc.setup(h)
			}

			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code, tc.name)

			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestProject_DeleteSuccess(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	// Create project
	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "to-delete"})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	projectARN := createResp["ProjectArn"].(string)

	// Delete it
	rec = doRequest(t, h, "DeleteProject", map[string]any{"ProjectArn": projectARN})
	assert.Equal(t, http.StatusOK, rec.Code)

	var deleteResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &deleteResp))
	assert.Equal(t, "DELETING", deleteResp["Status"])

	// DescribeProjects should no longer return it
	rec = doRequest(t, h, "DescribeProjects", map[string]any{"ProjectArns": []string{projectARN}})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	descriptions := descResp["ProjectDescriptions"].([]any)
	assert.Empty(t, descriptions)
}

// DeleteProject rejects a project that still has project versions with
// ResourceInUseException -- DeleteProjectInput's own doc comment
// (api_op_DeleteProject.go): "To delete a project you must first delete all
// models or adapters associated with the project.".
func TestProject_DeleteWithVersions_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "has-versions"})
	require.Equal(t, http.StatusOK, rec.Code)

	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projectARN := projResp["ProjectArn"].(string)

	rec = doRequest(t, h, "CreateProjectVersion", map[string]any{
		"ProjectArn":   projectARN,
		"VersionName":  "v1",
		"OutputConfig": map[string]any{"S3Bucket": "my-bucket"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DeleteProject", map[string]any{"ProjectArn": projectARN})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ResourceInUseException", errResp["__type"])
}

func TestProjectPolicies(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	// Create project
	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "policy-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projectARN := projResp["ProjectArn"].(string)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "PutProjectPolicy creates policy",
			action: "PutProjectPolicy",
			body: map[string]any{
				"ProjectArn":     projectARN,
				"PolicyName":     "my-policy",
				"PolicyDocument": `{"Version":"2012-10-17"}`,
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.NotEmpty(t, resp["PolicyRevisionId"])
			},
		},
		{
			name:     "ListProjectPolicies returns policy",
			action:   "ListProjectPolicies",
			body:     map[string]any{"ProjectArn": projectARN},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				policies, ok := resp["ProjectPolicies"].([]any)
				require.True(t, ok)
				assert.Len(t, policies, 1)
			},
		},
		{
			name:   "DeleteProjectPolicy removes policy",
			action: "DeleteProjectPolicy",
			body: map[string]any{
				"ProjectArn": projectARN,
				"PolicyName": "my-policy",
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.action, tc.body) //nolint:govet // existing issue.
			assert.Equal(t, tc.wantCode, rec.Code, tc.name)

			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

// TestDeleteProject_CascadesProjectPolicies verifies DeleteProject removes
// the project's ProjectPolicies too, per DeleteProjectInput's own doc
// comment (api_op_DeleteProject.go): "Be aware that deleting a given
// project will also delete any ProjectPolicies associated with that
// project".
func TestDeleteProject_CascadesProjectPolicies(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "policy-cascade-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projectARN := projResp["ProjectArn"].(string)

	rec = doRequest(t, h, "PutProjectPolicy", map[string]any{
		"ProjectArn":     projectARN,
		"PolicyName":     "my-policy",
		"PolicyDocument": `{"Version":"2012-10-17"}`,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DeleteProject", map[string]any{"ProjectArn": projectARN})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "ListProjectPolicies", map[string]any{"ProjectArn": projectARN})
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	policies, ok := listResp["ProjectPolicies"].([]any)
	require.True(t, ok)
	assert.Empty(t, policies, "ProjectPolicies must be cascade-deleted with the project")
}
