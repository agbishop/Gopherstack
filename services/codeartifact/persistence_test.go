package codeartifact_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/codeartifact"
)

// TestInMemoryBackend_RestoreInvalidData verifies that malformed JSON is
// reported as an error rather than silently discarded or partially applied.
func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := codeartifact.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend is discarded cleanly rather than
// partially decoded: the backend resets to empty state and Restore returns
// no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := codeartifact.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	_, err := b.CreateDomain(t.Context(), "seed-domain", "", nil)
	require.NoError(t, err)

	// A syntactically valid but version-mismatched snapshot.
	err = b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Empty(t, b.ListDomains(t.Context()))

	_, err = b.DescribeDomain(t.Context(), "seed-domain")
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero verifies that a
// snapshot with no version field at all decodes with Version == 0, which
// mismatches codeartifactSnapshotVersion and is discarded the same way any
// other incompatible version is -- not partially applied. This also covers
// the pre-Phase-3.3 on-disk shape (region-nested maps keyed by resource
// type), which lacks "version"/"tables" entirely.
func TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero(t *testing.T) {
	t.Parallel()

	b := codeartifact.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	_, err := b.CreateDomain(t.Context(), "seed-domain", "", nil)
	require.NoError(t, err)

	// The pre-refactor shape: region-nested maps, no version/tables keys.
	oldShape := `{"domains":{"us-east-1":{"seed-domain":{"name":"seed-domain"}}},"region":"us-east-1"}`
	err = b.Restore(t.Context(), []byte(oldShape))
	require.NoError(t, err)

	assert.Empty(t, b.ListDomains(t.Context()))
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a Snapshot->Restore
// round trip across every store.Table-backed resource family the Phase 3.3
// conversion touched (domains, repositories -- both "clean" tables -- plus
// the "dirty" packageGroups, packages, packageVersions, repositoryPolicies,
// and domainPolicies tables, each carrying a hidden region/domainName/
// repoName field through a DTO) and the plain externalConnections map left
// unconverted.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := codeartifact.NewInMemoryBackend("111122223333", "us-west-2")
	ctx := t.Context()

	domain, err := original.CreateDomain(ctx, "domain-1", "", map[string]string{"env": "test"})
	require.NoError(t, err)

	repo, err := original.CreateRepository(
		ctx,
		"domain-1",
		"repo-1",
		"a repo",
		map[string]string{"team": "core"},
		[]string{"upstream-1"},
	)
	require.NoError(t, err)

	pg, err := original.CreatePackageGroup(
		ctx,
		"domain-1",
		"/npm/pkg/*",
		"a group",
		"contact@example.com",
		nil,
	)
	require.NoError(t, err)

	// Auto-creates a stub Package entry.
	_, err = original.DescribePackage(ctx, "domain-1", "repo-1", "npm", "", "pkg-1")
	require.NoError(t, err)

	pv, err := original.PublishPackageVersion(
		ctx,
		"domain-1",
		"repo-1",
		"npm",
		"",
		"pkg-1",
		"1.0.0",
		codeartifact.AssetInfo{
			Name:    "pkg-1-1.0.0.tgz",
			Size:    4,
			SHA256:  "abcd",
			Content: []byte("data"),
		},
	)
	require.NoError(t, err)

	_, err = original.AssociateExternalConnection(ctx, "domain-1", "repo-1", "public:npmjs")
	require.NoError(t, err)

	repoPol, err := original.PutRepositoryPermissionsPolicy(
		ctx,
		"domain-1",
		"repo-1",
		`{"Version":"2012-10-17"}`,
		"",
	)
	require.NoError(t, err)

	domainPol, err := original.PutDomainPermissionsPolicy(
		ctx,
		"domain-1",
		`{"Version":"2012-10-17"}`,
		"",
	)
	require.NoError(t, err)

	snap := original.Snapshot(ctx)
	require.NotNil(t, snap)

	fresh := codeartifact.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	require.NoError(t, fresh.Restore(ctx, snap))

	// Scalars: accountID/region surface indirectly through a freshly created
	// resource (there is no direct accessor).
	newDomain, err := fresh.CreateDomain(ctx, "domain-2", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "111122223333", newDomain.Owner)
	assert.Contains(t, newDomain.ARN, "us-west-2")

	// domains table ("clean") + Tags.
	gotDomain, err := fresh.DescribeDomain(ctx, "domain-1")
	require.NoError(t, err)
	assert.Equal(t, domain.ARN, gotDomain.ARN)
	tagVals, err := fresh.ListTagsForResource(ctx, domain.ARN)
	require.NoError(t, err)
	assert.Equal(t, "test", tagVals["env"])

	freshDomains := fresh.ListDomains(ctx)
	domainNames := make([]string, 0, len(freshDomains))
	for _, d := range freshDomains {
		domainNames = append(domainNames, d.Name)
	}
	assert.ElementsMatch(t, []string{"domain-1", "domain-2"}, domainNames)

	// repositories table ("clean") + Tags + upstreams.
	gotRepo, err := fresh.DescribeRepository(ctx, "domain-1", "repo-1")
	require.NoError(t, err)
	assert.Equal(t, "a repo", gotRepo.Description)
	assert.Equal(t, []string{"upstream-1"}, gotRepo.UpstreamRepositories)
	repoTagVals, err := fresh.ListTagsForResource(ctx, repo.ARN)
	require.NoError(t, err)
	assert.Equal(t, "core", repoTagVals["team"])

	// packageGroups table ("dirty": hidden region field via a DTO).
	gotPG, err := fresh.DescribePackageGroup(ctx, "domain-1", "/npm/pkg/*")
	require.NoError(t, err)
	assert.Equal(t, pg.ARN, gotPG.ARN)
	assert.Equal(t, "a group", gotPG.Description)

	// packages + packageVersions tables ("dirty": hidden region field).
	gotPkg, err := fresh.DescribePackage(ctx, "domain-1", "repo-1", "npm", "", "pkg-1")
	require.NoError(t, err)
	assert.Equal(t, "pkg-1", gotPkg.Name)

	versions, err := fresh.ListPackageVersions(ctx, "domain-1", "repo-1", "npm", "", "pkg-1", "", "")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, pv.Version, versions[0].Version)
	assert.Equal(t, "Published", versions[0].Status)
	require.Len(t, versions[0].Assets, 1)
	assert.Equal(t, "pkg-1-1.0.0.tgz", versions[0].Assets[0].Name)

	// externalConnections plain map (unconverted).
	conns := fresh.GetExternalConnections(ctx, "domain-1", "repo-1")
	require.Len(t, conns, 1)
	assert.Equal(t, "public:npmjs", conns[0].ExternalConnectionName)

	// repositoryPolicies table ("dirty": hidden region/domainName/repoName).
	gotRepoPol, err := fresh.GetRepositoryPermissionsPolicy(ctx, "domain-1", "repo-1")
	require.NoError(t, err)
	assert.Equal(t, repoPol.Document, gotRepoPol.Document)
	assert.Equal(t, repoPol.ResourceARN, gotRepoPol.ResourceARN)

	// domainPolicies table ("dirty": hidden region/domainName).
	gotDomainPol, err := fresh.GetDomainPermissionsPolicy(ctx, "domain-1")
	require.NoError(t, err)
	assert.Equal(t, domainPol.Document, gotDomainPol.Document)
	assert.Equal(t, domainPol.ResourceARN, gotDomainPol.ResourceARN)

	// DeleteDomain rejects a domain that still contains a repository
	// (api_op_DeleteDomain.go: "You cannot delete a domain that contains
	// repositories."); proves the restored repositoriesByRegion index is
	// intact post-restore, not just the primary domains lookup.
	_, err = fresh.DeleteDomain(ctx, "domain-1")
	require.ErrorIs(t, err, codeartifact.ErrAlreadyExists)

	// Remove the repository, then prove the domain-delete cascade (byRegion
	// indexes and hidden region/domainName/repoName fields on the restored
	// values rebuilt correctly, not just the primary lookup).
	_, err = fresh.DeleteRepository(ctx, "domain-1", "repo-1")
	require.NoError(t, err)
	_, err = fresh.DeleteDomain(ctx, "domain-1")
	require.NoError(t, err)
	_, err = fresh.DescribeRepository(ctx, "domain-1", "repo-1")
	require.Error(t, err)
	_, err = fresh.GetRepositoryPermissionsPolicy(ctx, "domain-1", "repo-1")
	require.Error(t, err)
	_, err = fresh.DescribeDomain(ctx, "domain-1")
	require.Error(t, err)
}
func TestHandler_Persistence(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=persist-domain", nil)
	doRequest(
		t,
		h,
		http.MethodPost,
		"/v1/repository?domain=persist-domain&repository=persist-repo",
		nil,
	)

	// Snapshot and restore.
	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	h2 := newTestHandler(t)
	require.NoError(t, h2.Restore(t.Context(), snap))

	descRec := doRequest(t, h2, http.MethodGet, "/v1/domain?domain=persist-domain", nil)
	assert.Equal(t, http.StatusOK, descRec.Code)

	repoRec := doRequest(
		t,
		h2,
		http.MethodGet,
		"/v1/repository?domain=persist-domain&repository=persist-repo",
		nil,
	)
	assert.Equal(t, http.StatusOK, repoRec.Code)
}

func TestHandler_NewOperations_Persistence(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/v1/domain?domain=persist2-domain", nil)
	doRequest(
		t,
		h,
		http.MethodPost,
		"/v1/repository?domain=persist2-domain&repository=persist2-repo",
		nil,
	)

	// Create package group.
	doRequest(
		t,
		h,
		http.MethodPost,
		"/v1/package-group?domain=persist2-domain",
		map[string]any{"packageGroup": "/npm/*"},
	)

	// Create package version entry.
	doRequest(
		t,
		h,
		http.MethodGet,
		"/v1/package/version?domain=persist2-domain&repository=persist2-repo&format=npm&package=react&version=18.0.0",
		nil,
	)

	// Associate external connection.
	doRequest(
		t,
		h,
		http.MethodPost,
		"/v1/repository/external-connection?domain=persist2-domain&repository=persist2-repo&external-connection=public:npmjs",
		nil,
	)

	// Put repository permissions policy.
	doRequest(
		t,
		h,
		http.MethodPut,
		"/v1/repository/permissions/policy?domain=persist2-domain&repository=persist2-repo",
		map[string]any{
			"policyDocument": `{"Version":"2012-10-17","Statement":[]}`,
		},
	)

	// Snapshot and restore.
	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	h2 := newTestHandler(t)
	require.NoError(t, h2.Restore(t.Context(), snap))

	// Verify package group survived.
	pgRec := doRequest(
		t,
		h2,
		http.MethodGet,
		"/v1/package-group?domain=persist2-domain&package-group=/npm/*",
		nil,
	)
	assert.Equal(t, http.StatusOK, pgRec.Code)

	// Verify package version survived.
	pvRec := doRequest(
		t,
		h2,
		http.MethodGet,
		"/v1/package/version?domain=persist2-domain&repository=persist2-repo&format=npm&package=react&version=18.0.0",
		nil,
	)
	assert.Equal(t, http.StatusOK, pvRec.Code)

	// Verify repository permissions policy survived.
	polRec := doRequest(
		t,
		h2,
		http.MethodGet,
		"/v1/repository/permissions/policy?domain=persist2-domain&repository=persist2-repo",
		nil,
	)
	assert.Equal(t, http.StatusOK, polRec.Code)
}

func TestCABackend_PersistenceRoundTrip(t *testing.T) {
	t.Parallel()

	b := codeartifact.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	_, err := b.CreateDomain(context.Background(), "snap-domain", "", nil)
	require.NoError(t, err)

	_, err = b.CreateRepository(context.Background(), "snap-domain", "snap-repo", "", nil, nil)
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := codeartifact.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	err = b2.Restore(t.Context(), snap)
	require.NoError(t, err)

	doms := b2.ListDomains(context.Background())
	require.Len(t, doms, 1)
}
