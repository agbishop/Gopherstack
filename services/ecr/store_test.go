package ecr_test

// store_test.go — tests for store.go: InMemoryBackend construction, endpoint
// accessors, Reset, and the seed/count test-only helpers exported for other
// test files via export_test.go.

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecr"
)

// newBackend creates a fresh InMemoryBackend for testing. It is used widely
// across this package's test files as the canonical backend fixture.
func newBackend(t *testing.T) *ecr.InMemoryBackend {
	t.Helper()

	return ecr.NewInMemoryBackend("123456789012", "us-east-1", "localhost:9999")
}

// makeRepo creates a repository directly on b via the real CreateRepository op.
func makeRepo(t *testing.T, b *ecr.InMemoryBackend, name string) *ecr.Repository {
	t.Helper()

	repo, err := b.CreateRepository(context.Background(), name, "", false, "", "")
	require.NoError(t, err)

	return repo
}

// mustUploadLayer performs a real InitiateLayerUpload -> UploadLayerPart ->
// CompleteLayerUpload flow directly against b and returns the digest ECR
// computed from data. This is the canonical way to seed an "available" layer
// in backend-level tests: CompleteLayerUpload no longer accepts a "direct
// digest" shortcut (an uploadId with no live InitiateLayerUpload session) --
// it now returns UploadNotFoundException for one, and EmptyUploadException
// for a live session that received no UploadLayerPart calls.
func mustUploadLayer(t *testing.T, b *ecr.InMemoryBackend, repoName string, data []byte) string {
	t.Helper()

	init, err := b.InitiateLayerUpload(context.Background(), repoName)
	require.NoError(t, err)

	_, err = b.UploadLayerPart(context.Background(), repoName, init.UploadID, 0, int64(len(data))-1, data)
	require.NoError(t, err)

	result, err := b.CompleteLayerUpload(context.Background(), repoName, init.UploadID, nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.LayerDigest)

	return result.LayerDigest
}

// makeImage builds an Image fixture with the given digest and tag.
func makeImage(digest, tag string) ecr.Image {
	return ecr.Image{
		ImageDigest:   digest,
		ImageManifest: fmt.Sprintf(`{"schemaVersion":2,"digest":"%s"}`, digest),
		ImageID:       ecr.ImageIdentifier{ImageDigest: digest, ImageTag: tag},
	}
}

// TestECR_InMemoryBackend_ProxyEndpoint verifies the ProxyEndpoint accessor.
func TestECR_InMemoryBackend_ProxyEndpoint(t *testing.T) {
	t.Parallel()

	b := ecr.NewInMemoryBackend("000000000000", "us-east-1", "initial:5000")
	assert.Equal(t, "initial:5000", b.ProxyEndpoint())

	b.SetEndpoint("updated:9000")
	assert.Equal(t, "updated:9000", b.ProxyEndpoint())
}

// TestECR_Backend_SetEndpoint verifies that SetEndpoint affects RepositoryURI.
func TestECR_Backend_SetEndpoint(t *testing.T) {
	t.Parallel()

	backend := ecr.NewInMemoryBackend("000000000000", "us-east-1", "")
	backend.SetEndpoint("localhost:9000")

	repo, err := backend.CreateRepository(context.Background(), "my-repo", "", false, "", "")
	require.NoError(t, err)
	assert.Contains(t, repo.RepositoryURI, "localhost:9000")
}

// TestReset_ClearsAllBackendState verifies that Reset clears all backend state.
func TestReset_ClearsAllBackendState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		populate func(b *ecr.InMemoryBackend)
		name     string
		wantZero bool
	}{
		{
			name:     "empty_backend",
			wantZero: true,
		},
		{
			name: "populated_backend",
			populate: func(b *ecr.InMemoryBackend) {
				b.AddRepositoryInternal(ecr.Repository{
					RepositoryName: "repo1",
					RepositoryARN:  "arn:aws:ecr:us-east-1:123456789012:repository/repo1",
				})
				b.AddLifecyclePolicyInternal("repo1", `{"rules":[]}`)
			},
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "localhost:5000")
			if tt.populate != nil {
				tt.populate(b)
			}

			b.Reset()

			assert.Equal(t, tt.wantZero, b.RepositoryCount() == 0)
			assert.Equal(t, tt.wantZero, b.LifecyclePolicyCount() == 0)
		})
	}
}

// TestReset_MultipleCycles verifies that Reset can be called multiple times.
func TestReset_MultipleCycles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cycles int
	}{
		{name: "single_reset", cycles: 1},
		{name: "triple_reset", cycles: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "localhost:5000")
			b.AddRepositoryInternal(ecr.Repository{RepositoryName: "r1", RepositoryARN: "arn:r1"})

			for i := range tt.cycles {
				b.Reset()
				assert.Equal(t, 0, b.RepositoryCount(), "cycle %d", i)
			}
		})
	}
}

// TestReset_ClearsRepoUploadIndex verifies that Reset clears repoUploadIndex,
// the same ephemeral in-progress-upload bookkeeping that Restore already
// clears (see persistence.go's Restore). Without this, repoUploadIndex grows
// unboundedly across repeated Reset cycles on a long-running server: each
// InitiateLayerUpload call for a never-since-deleted repository name adds an
// entry that nothing but DeleteRepository ever removes.
func TestReset_ClearsRepoUploadIndex(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	makeRepo(t, b, "r1")

	_, err := b.InitiateLayerUpload(context.Background(), "r1")
	require.NoError(t, err)
	require.Positive(t, b.RepoUploadIndexCount())

	b.Reset()

	assert.Equal(t, 0, b.RepoUploadIndexCount())
}

// TestSeedHelpers_AddRepositoryAndLifecyclePolicy verifies AddRepositoryInternal
// and AddLifecyclePolicyInternal, the test-only seed helpers exported via
// export_test.go.
func TestSeedHelpers_AddRepositoryAndLifecyclePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		repoName     string
		policyText   string
		wantRepos    int
		wantPolicies int
	}{
		{
			name:         "seed_repo_and_policy",
			repoName:     "seeded-repo",
			policyText:   `{"rules":[]}`,
			wantRepos:    1,
			wantPolicies: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "localhost:5000")
			b.AddRepositoryInternal(ecr.Repository{
				RepositoryName: tt.repoName,
				RepositoryARN:  "arn:aws:ecr:us-east-1:123456789012:repository/" + tt.repoName,
			})
			b.AddLifecyclePolicyInternal(tt.repoName, tt.policyText)

			assert.Equal(t, tt.wantRepos, b.RepositoryCount())
			assert.Equal(t, tt.wantPolicies, b.LifecyclePolicyCount())
		})
	}
}

// TestExportCountHelpers_RepositoryAndLayerCounts verifies RepositoryCount and
// UploadedLayerCount, the test-only count helpers exported via export_test.go.
func TestExportCountHelpers_RepositoryAndLayerCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		repos      int
		wantRepos  int
		wantLayers int
	}{
		{
			name:       "empty_backend",
			repos:      0,
			wantRepos:  0,
			wantLayers: 0,
		},
		{
			name:       "two_repos",
			repos:      2,
			wantRepos:  2,
			wantLayers: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "localhost:5000")
			for i := range tt.repos {
				name := fmt.Sprintf("repo%d", i)
				b.AddRepositoryInternal(ecr.Repository{
					RepositoryName: name,
					RepositoryARN:  "arn:r-" + name,
				})
			}

			assert.Equal(t, tt.wantRepos, b.RepositoryCount())
			assert.Equal(t, tt.wantLayers, b.UploadedLayerCount())
		})
	}
}

// TestExportCountHelpers_ImageAndTemplateCounts verifies ImageCount,
// PullThroughCacheRuleCount, RepositoryCreationTemplateCount, and
// LifecyclePolicyCount grow as expected as state is seeded.
func TestExportCountHelpers_ImageAndTemplateCounts(t *testing.T) {
	t.Parallel()

	backend := ecr.NewInMemoryBackend("000000000000", "us-east-1", "localhost:8000")

	assert.Equal(t, 0, backend.ImageCount())
	assert.Equal(t, 0, backend.PullThroughCacheRuleCount())
	assert.Equal(t, 0, backend.RepositoryCreationTemplateCount())
	assert.Equal(t, 0, backend.LifecyclePolicyCount())

	backend.AddImageInternal("repo1", ecr.Image{
		ImageDigest:    "sha256:count1",
		ImageID:        ecr.ImageIdentifier{ImageDigest: "sha256:count1"},
		RepositoryName: "repo1",
	})
	assert.Equal(t, 1, backend.ImageCount())

	backend.AddImageInternal("repo1", ecr.Image{
		ImageDigest:    "sha256:count2",
		ImageID:        ecr.ImageIdentifier{ImageDigest: "sha256:count2"},
		RepositoryName: "repo1",
	})
	assert.Equal(t, 2, backend.ImageCount())
}
