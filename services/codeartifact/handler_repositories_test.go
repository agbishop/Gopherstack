package codeartifact_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codeartifact"
)

func TestHandler_CreateRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		domain     string
		repo       string
		wantStatus int
	}{
		{
			name:       "success",
			domain:     "test-domain",
			repo:       "my-repo",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_domain",
			domain:     "",
			repo:       "my-repo",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_repo",
			domain:     "test-domain",
			repo:       "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "domain_not_found",
			domain:     "nonexistent-domain",
			repo:       "my-repo",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create domain for success cases.
			if tt.domain == "test-domain" {
				doRequest(t, h, http.MethodPost, "/v1/domain?domain=test-domain", nil)
			}

			path := "/v1/repository"
			sep := "?"
			if tt.domain != "" {
				path += sep + "domain=" + tt.domain
				sep = "&"
			}
			if tt.repo != "" {
				path += sep + "repository=" + tt.repo
			}

			rec := doRequest(t, h, http.MethodPost, path, map[string]any{
				"description": "test repo",
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				repo, _ := resp["repository"].(map[string]any)
				assert.Equal(t, tt.repo, repo["name"])
				assert.Equal(t, tt.domain, repo["domainName"])
				assert.NotEmpty(t, repo["arn"])
			}
		})
	}
}

func TestHandler_DescribeRepository(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=d1", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=d1&repository=r1", nil)

	rec := doRequest(t, h, http.MethodGet, "/v1/repository?domain=d1&repository=r1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	repo, _ := resp["repository"].(map[string]any)
	assert.Equal(t, "r1", repo["name"])
	assert.Equal(t, "d1", repo["domainName"])
}

func TestHandler_DeleteRepository(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=d2", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=d2&repository=r2", nil)

	delRec := doRequest(t, h, http.MethodDelete, "/v1/repository?domain=d2&repository=r2", nil)
	assert.Equal(t, http.StatusOK, delRec.Code)

	descRec := doRequest(t, h, http.MethodGet, "/v1/repository?domain=d2&repository=r2", nil)
	assert.Equal(t, http.StatusNotFound, descRec.Code)
}

func TestHandler_ListRepositoriesInDomain(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=d3", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=d3&repository=r3a", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=d3&repository=r3b", nil)

	rec := doRequest(t, h, http.MethodPost, "/v1/domain/repositories?domain=d3", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	repos, _ := resp["repositories"].([]any)
	assert.Len(t, repos, 2)
}

func TestHandler_ListRepositories(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=da", nil)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=db", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=da&repository=ra", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=db&repository=rb", nil)

	rec := doRequest(t, h, http.MethodPost, "/v1/repositories", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	repos, _ := resp["repositories"].([]any)
	assert.Len(t, repos, 2)
}

func TestHandler_GetRepositoryEndpoint(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=ep-domain", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=ep-domain&repository=ep-repo", nil)

	rec := doRequest(
		t,
		h,
		http.MethodGet,
		"/v1/repository/endpoint?domain=ep-domain&repository=ep-repo&format=npm",
		nil,
	)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["repositoryEndpoint"])
}

func TestHandler_AssociateExternalConnection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codeartifact.Handler)
		name       string
		path       string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *codeartifact.Handler) {
				doRequest(t, h, http.MethodPost, "/v1/domain?domain=ec-domain", nil)
				doRequest(t, h, http.MethodPost, "/v1/repository?domain=ec-domain&repository=ec-repo", nil)
			},
			path: "/v1/repository/external-connection" +
				"?domain=ec-domain&repository=ec-repo&external-connection=public:npmjs",
			wantStatus: http.StatusOK,
		},
		{
			name: "duplicate_connection",
			setup: func(h *codeartifact.Handler) {
				doRequest(t, h, http.MethodPost, "/v1/domain?domain=ec-dup", nil)
				doRequest(t, h, http.MethodPost, "/v1/repository?domain=ec-dup&repository=ec-repo2", nil)
				doRequest(
					t,
					h,
					http.MethodPost,
					"/v1/repository/external-connection?domain=ec-dup&repository=ec-repo2&external-connection=public:npmjs",
					nil,
				)
			},
			path:       "/v1/repository/external-connection?domain=ec-dup&repository=ec-repo2&external-connection=public:npmjs",
			wantStatus: http.StatusConflict,
		},
		{
			name:       "missing_domain",
			path:       "/v1/repository/external-connection?repository=ec-repo&external-connection=public:npmjs",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_repo",
			path:       "/v1/repository/external-connection?domain=ec-domain&external-connection=public:npmjs",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_connection",
			path:       "/v1/repository/external-connection?domain=ec-domain&repository=ec-repo",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "repo_not_found",
			path:       "/v1/repository/external-connection?domain=ec-domain&repository=nope&external-connection=public:npmjs",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, http.MethodPost, tt.path, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotNil(t, resp["repository"])
			}
		})
	}
}

func TestHandler_RepositoryPermissionsPolicy(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=rp-domain", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=rp-domain&repository=rp-repo", nil)

	// Get before put → not found.
	getRec1 := doRequest(
		t,
		h,
		http.MethodGet,
		"/v1/repository/permissions/policy?domain=rp-domain&repository=rp-repo",
		nil,
	)
	assert.Equal(t, http.StatusNotFound, getRec1.Code)

	// Put policy.
	putRec := doRequest(
		t,
		h,
		http.MethodPut,
		"/v1/repository/permissions/policy?domain=rp-domain&repository=rp-repo",
		map[string]any{
			"policyDocument": `{"Version":"2012-10-17","Statement":[]}`,
		},
	)
	assert.Equal(t, http.StatusOK, putRec.Code)

	var putResp map[string]any
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putResp))
	pol, _ := putResp["policy"].(map[string]any)
	assert.NotEmpty(t, pol["revision"])
	assert.NotEmpty(t, pol["resourceArn"])

	// Get after put.
	getRec2 := doRequest(
		t,
		h,
		http.MethodGet,
		"/v1/repository/permissions/policy?domain=rp-domain&repository=rp-repo",
		nil,
	)
	assert.Equal(t, http.StatusOK, getRec2.Code)

	// Delete. Real AWS serves DeleteRepositoryPermissionsPolicy on its own plural
	// "/v1/repository/permissions/policies" path, distinct from Get/Put's singular path.
	delRec := doRequest(
		t,
		h,
		http.MethodDelete,
		"/v1/repository/permissions/policies?domain=rp-domain&repository=rp-repo",
		nil,
	)
	assert.Equal(t, http.StatusOK, delRec.Code)

	// Delete again → not found.
	delRec2 := doRequest(
		t,
		h,
		http.MethodDelete,
		"/v1/repository/permissions/policies?domain=rp-domain&repository=rp-repo",
		nil,
	)
	assert.Equal(t, http.StatusNotFound, delRec2.Code)

	// Validation errors.
	assert.Equal(
		t,
		http.StatusBadRequest,
		doRequest(t, h, http.MethodDelete, "/v1/repository/permissions/policies?repository=rp-repo", nil).Code,
	)
	assert.Equal(
		t,
		http.StatusBadRequest,
		doRequest(t, h, http.MethodDelete, "/v1/repository/permissions/policies?domain=rp-domain", nil).Code,
	)
}

// TestHandler_RepositoryPermissionsPolicy_RevisionLocking proves PolicyRevision
// is enforced as optimistic locking on Put/DeleteRepositoryPermissionsPolicy.
// Per api_op_PutRepositoryPermissionsPolicy.go: "Sets the revision of the
// resource policy ... This revision is used for optimistic locking, which
// prevents others from overwriting your changes to the repository's resource
// policy." Both ops model ConflictException.
func TestHandler_RepositoryPermissionsPolicy_RevisionLocking(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=rplock-domain", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=rplock-domain&repository=rplock-repo", nil)

	putRec := doRequest(
		t, h, http.MethodPut,
		"/v1/repository/permissions/policy?domain=rplock-domain&repository=rplock-repo",
		map[string]any{"policyDocument": `{"Version":"2012-10-17","Statement":[]}`},
	)
	require.Equal(t, http.StatusOK, putRec.Code)
	var putResp map[string]any
	require.NoError(t, json.Unmarshal(putRec.Body.Bytes(), &putResp))
	pol, _ := putResp["policy"].(map[string]any)
	revision, _ := pol["revision"].(string)
	require.NotEmpty(t, revision)

	staleRec := doRequest(
		t, h, http.MethodPut,
		"/v1/repository/permissions/policy?domain=rplock-domain&repository=rplock-repo",
		map[string]any{
			"policyDocument": `{"Version":"2012-10-17","Statement":[{"Effect":"Deny"}]}`,
			"policyRevision": "wrong-revision",
		},
	)
	assert.Equal(t, http.StatusConflict, staleRec.Code)

	staleDeleteRec := doRequest(
		t,
		h,
		http.MethodDelete,
		"/v1/repository/permissions/policies?domain=rplock-domain&repository=rplock-repo&policy-revision=wrong-revision",
		nil,
	)
	assert.Equal(t, http.StatusConflict, staleDeleteRec.Code)

	getRec := doRequest(
		t, h, http.MethodGet,
		"/v1/repository/permissions/policy?domain=rplock-domain&repository=rplock-repo",
		nil,
	)
	require.Equal(t, http.StatusOK, getRec.Code)
	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	getPol, _ := getResp["policy"].(map[string]any)
	assert.NotContains(t, getPol["document"], "Deny")

	matchDeleteRec := doRequest(
		t, h, http.MethodDelete,
		"/v1/repository/permissions/policies?domain=rplock-domain&repository=rplock-repo&policy-revision="+revision,
		nil,
	)
	assert.Equal(t, http.StatusOK, matchDeleteRec.Code)
}

func TestHandler_DeleteRepositoryCascade(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=repcas-domain", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=repcas-domain&repository=repcas-repo", nil)
	doRequest(
		t, h, http.MethodGet,
		"/v1/package/version?domain=repcas-domain&repository=repcas-repo&format=npm&package=mypkg&version=1.0.0",
		nil,
	)

	delRec := doRequest(t, h, http.MethodDelete, "/v1/repository?domain=repcas-domain&repository=repcas-repo", nil)
	assert.Equal(t, http.StatusOK, delRec.Code)

	// Re-create repo - packages/versions should be gone.
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=repcas-domain&repository=repcas-repo", nil)

	// DeletePackage on a package that no longer exists returns 404.
	delPkgRec := doRequest(
		t, h, http.MethodDelete,
		"/v1/package?domain=repcas-domain&repository=repcas-repo&format=npm&package=mypkg",
		nil,
	)
	assert.Equal(t, http.StatusNotFound, delPkgRec.Code)
}

func TestHandler_ExternalConnectionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		connectionName string
		wantFormat     string
	}{
		{name: "npm", connectionName: "public:npmjs", wantFormat: "npm"},
		{name: "pypi", connectionName: "public:pypi", wantFormat: "pypi"},
		{name: "maven", connectionName: "public:maven-central", wantFormat: "maven"},
		{name: "nuget", connectionName: "public:nuget-org", wantFormat: "nuget"},
		{name: "cargo", connectionName: "public:crates-io", wantFormat: "cargo"},
		{name: "generic", connectionName: "public:unknown", wantFormat: "generic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, http.MethodPost, "/v1/domain?domain=fmt-domain", nil)
			doRequest(t, h, http.MethodPost, "/v1/repository?domain=fmt-domain&repository=fmt-repo", nil)

			rec := doRequest(
				t,
				h,
				http.MethodPost,
				"/v1/repository/external-connection?domain=fmt-domain&repository=fmt-repo&external-connection="+tt.connectionName,
				nil,
			)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			repo, _ := resp["repository"].(map[string]any)
			conns, _ := repo["externalConnections"].([]any)
			require.Len(t, conns, 1)
			conn, _ := conns[0].(map[string]any)
			assert.Equal(t, tt.wantFormat, conn["packageFormat"])
		})
	}
}

func TestHandler_ListRepositoriesInDomain_DomainNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/v1/domain/repositories?domain=nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_UpdateRepository_DescriptionPersists(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=ur-desc-domain", nil)
	doRequest(
		t, h, http.MethodPost, "/v1/repository?domain=ur-desc-domain&repository=ur-desc-repo",
		map[string]any{"description": "original"},
	)

	rec := doRequest(
		t, h, http.MethodPut, "/v1/repository?domain=ur-desc-domain&repository=ur-desc-repo",
		map[string]any{"description": "updated"},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	descRec := doRequest(t, h, http.MethodGet, "/v1/repository?domain=ur-desc-domain&repository=ur-desc-repo", nil)
	require.Equal(t, http.StatusOK, descRec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	repo, _ := resp["repository"].(map[string]any)
	assert.Equal(t, "updated", repo["description"])
}

func TestHandler_ExternalConnections_MultipleConnections(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupDomain(t, h, "multi-conn-domain")
	setupRepo(t, h, "multi-conn-domain", "multi-conn-repo")

	// Associate multiple external connections.
	for _, conn := range []string{"public:npmjs", "public:pypi", "public:maven-central"} {
		rec := doRequest(
			t,
			h,
			http.MethodPost,
			"/v1/repository/external-connection?domain=multi-conn-domain&repository=multi-conn-repo&external-connection="+conn,
			nil,
		)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// Verify 3 connections via DescribeRepository.
	descRec := doRequest(
		t, h, http.MethodGet,
		"/v1/repository?domain=multi-conn-domain&repository=multi-conn-repo",
		nil,
	)
	require.Equal(t, http.StatusOK, descRec.Code)
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	repo, _ := descResp["repository"].(map[string]any)
	conns, _ := repo["externalConnections"].([]any)
	assert.Len(t, conns, 3)

	// Disassociate one.
	disRec := doRequest(
		t,
		h,
		http.MethodDelete,
		"/v1/repository/external-connection?domain=multi-conn-domain"+
			"&repository=multi-conn-repo&external-connection=public:pypi",
		nil,
	)
	require.Equal(t, http.StatusOK, disRec.Code)
	var disResp map[string]any
	require.NoError(t, json.Unmarshal(disRec.Body.Bytes(), &disResp))
	disRepo, _ := disResp["repository"].(map[string]any)
	disConns, _ := disRepo["externalConnections"].([]any)
	assert.Len(t, disConns, 2)
}

func TestHandler_DisassociateExternalConnection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codeartifact.Handler)
		name       string
		path       string
		wantStatus int
	}{
		{
			name: "success_removes_connection",
			setup: func(h *codeartifact.Handler) {
				setupDomain(t, h, "dis-domain")
				setupRepo(t, h, "dis-domain", "dis-repo")
				doRequest(
					t,
					h,
					http.MethodPost,
					"/v1/repository/external-connection"+
						"?domain=dis-domain&repository=dis-repo&external-connection=public:npmjs",
					nil,
				)
			},
			path: "/v1/repository/external-connection" +
				"?domain=dis-domain&repository=dis-repo&external-connection=public:npmjs",
			wantStatus: http.StatusOK,
		},
		{
			name: "success_nonexistent_connection",
			setup: func(h *codeartifact.Handler) {
				setupDomain(t, h, "dis2-domain")
				setupRepo(t, h, "dis2-domain", "dis2-repo")
			},
			path: "/v1/repository/external-connection" +
				"?domain=dis2-domain&repository=dis2-repo&external-connection=public:npmjs",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_domain",
			path:       "/v1/repository/external-connection?repository=dis-repo&external-connection=public:npmjs",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_repo",
			path:       "/v1/repository/external-connection?domain=dis-domain&external-connection=public:npmjs",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_connection",
			path:       "/v1/repository/external-connection?domain=dis-domain&repository=dis-repo",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "repo_not_found",
			path: "/v1/repository/external-connection" +
				"?domain=dis-domain&repository=nope&external-connection=public:npmjs",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, http.MethodDelete, tt.path, nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				repo, _ := resp["repository"].(map[string]any)
				assert.NotNil(t, repo)
			}
		})
	}
}

func TestHandler_UpdateRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       any
		setup      func(h *codeartifact.Handler)
		name       string
		path       string
		wantStatus int
	}{
		{
			name: "success_update_description",
			setup: func(h *codeartifact.Handler) {
				setupDomain(t, h, "ur-domain")
				setupRepo(t, h, "ur-domain", "ur-repo")
			},
			path:       "/v1/repository?domain=ur-domain&repository=ur-repo",
			body:       map[string]any{"description": "updated repo description"},
			wantStatus: http.StatusOK,
		},
		{
			name: "success_no_body",
			setup: func(h *codeartifact.Handler) {
				setupDomain(t, h, "ur2-domain")
				setupRepo(t, h, "ur2-domain", "ur2-repo")
			},
			path:       "/v1/repository?domain=ur2-domain&repository=ur2-repo",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_domain",
			path:       "/v1/repository?repository=ur-repo",
			body:       map[string]any{"description": "updated"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_repo",
			path:       "/v1/repository?domain=ur-domain",
			body:       map[string]any{"description": "updated"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "repo_not_found",
			path:       "/v1/repository?domain=ur-domain&repository=nope",
			body:       map[string]any{"description": "test"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, http.MethodPut, tt.path, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				repo, _ := resp["repository"].(map[string]any)
				assert.NotNil(t, repo)
			}
		})
	}
}

func TestListRepositoriesInDomain_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=pag-domain", nil)

	for i := range 5 {
		doRequest(t, h, http.MethodPost, fmt.Sprintf("/v1/repository?domain=pag-domain&repository=repo-%02d", i), nil)
	}

	rec1 := doRequest(t, h, http.MethodGet, "/v1/domain/repositories?domain=pag-domain&max-results=2", nil)
	require.Equal(t, http.StatusOK, rec1.Code)

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))
	page1, _ := out1["repositories"].([]any)
	assert.Len(t, page1, 2)
	nextToken, ok := out1["nextToken"].(string)
	assert.True(t, ok && nextToken != "", "nextToken must be present after partial page")

	rec2 := doRequest(t, h, http.MethodGet,
		"/v1/domain/repositories?domain=pag-domain&max-results=2&next-token="+nextToken, nil)
	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))
	page2, _ := out2["repositories"].([]any)
	assert.Len(t, page2, 2)
}

// TestListPackages_Pagination verifies pagination on ListPackages.

func TestCreateRepository_UpstreamRepositories(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=up-domain", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=up-domain&repository=upstream-repo", nil)

	rec := doRequest(t, h, http.MethodPost, "/v1/repository?domain=up-domain&repository=my-repo",
		map[string]any{
			"upstreams": []map[string]any{
				{"repositoryName": "upstream-repo"},
			},
		},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	repo, _ := out["repository"].(map[string]any)
	upstreams, _ := repo["upstreams"].([]any)
	require.Len(t, upstreams, 1)
	entry, _ := upstreams[0].(map[string]any)
	assert.Equal(t, "upstream-repo", entry["repositoryName"])
}

// TestUpdateRepository_UpstreamRepositories verifies UpdateRepository accepts
// and persists upstreamRepositories changes.

func TestUpdateRepository_UpstreamRepositories(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=upd-domain", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=upd-domain&repository=upstream-repo", nil)
	doRequest(t, h, http.MethodPost, "/v1/repository?domain=upd-domain&repository=my-repo", nil)

	rec := doRequest(t, h, http.MethodPut, "/v1/repository?domain=upd-domain&repository=my-repo",
		map[string]any{
			"upstreams": []map[string]any{
				{"repositoryName": "upstream-repo"},
			},
		},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	repo, _ := out["repository"].(map[string]any)
	upstreams, _ := repo["upstreams"].([]any)
	require.Len(t, upstreams, 1)
	entry, _ := upstreams[0].(map[string]any)
	assert.Equal(t, "upstream-repo", entry["repositoryName"])
}

// TestListPackageGroups_Pagination verifies pagination on ListPackageGroups.
