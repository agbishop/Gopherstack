package ecr_test

// This file verifies the Backend and Snapshottable interface contracts and
// tests the Handler's behaviour when its Backend does or does not implement
// Snapshottable.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecr"
)

// compile-time assertion: InMemoryBackend satisfies Backend.
var _ ecr.Backend = (*ecr.InMemoryBackend)(nil)

// compile-time assertion: InMemoryBackend satisfies Snapshottable.
var _ ecr.Snapshottable = (*ecr.InMemoryBackend)(nil)

// ---- stubBackend -------------------------------------------------------

// stubBackend is a minimal Backend implementation that does NOT implement
// Snapshottable, used to verify that Handler gracefully skips persistence
// for non-snapshottable backends.
type stubBackend struct {
	repos map[string]*ecr.Repository
}

func newStubBackend() *stubBackend {
	return &stubBackend{repos: make(map[string]*ecr.Repository)}
}

func (s *stubBackend) CreateRepository(
	_ context.Context,
	name, _ string,
	_ bool,
	_, _ string,
) (*ecr.Repository, error) {
	if name == "" {
		return nil, ecr.ErrInvalidRepositoryName
	}

	if _, ok := s.repos[name]; ok {
		return nil, ecr.ErrRepositoryAlreadyExists
	}

	r := &ecr.Repository{RepositoryName: name, RegistryID: "000000000000"}
	s.repos[name] = r

	cp := *r

	return &cp, nil
}

func (s *stubBackend) DescribeRepositories(_ context.Context, names []string) ([]ecr.Repository, error) {
	if len(names) == 0 {
		out := make([]ecr.Repository, 0, len(s.repos))
		for _, r := range s.repos {
			out = append(out, *r)
		}

		return out, nil
	}

	out := make([]ecr.Repository, 0, len(names))

	for _, n := range names {
		r, ok := s.repos[n]
		if !ok {
			return nil, ecr.ErrRepositoryNotFound
		}

		out = append(out, *r)
	}

	return out, nil
}

func (s *stubBackend) DeleteRepository(_ context.Context, name string, _ bool) (*ecr.Repository, error) {
	r, ok := s.repos[name]
	if !ok {
		return nil, ecr.ErrRepositoryNotFound
	}

	delete(s.repos, name)

	cp := *r

	return &cp, nil
}

func (s *stubBackend) ProxyEndpoint() string { return "stub:5000" }
func (s *stubBackend) SetEndpoint(_ string)  {}

func (s *stubBackend) BatchCheckLayerAvailability(_ context.Context,
	_ string,
	_ []string,
) ([]ecr.LayerAvailability, []ecr.LayerFailure, error) {
	return []ecr.LayerAvailability{}, []ecr.LayerFailure{}, nil
}

func (s *stubBackend) BatchDeleteImage(_ context.Context,
	_ string,
	_ []ecr.ImageIdentifier,
) ([]ecr.ImageIdentifier, []ecr.ImageFailure, error) {
	return []ecr.ImageIdentifier{}, []ecr.ImageFailure{}, nil
}

func (s *stubBackend) BatchGetImage(
	_ context.Context,
	_ string,
	_ []ecr.ImageIdentifier,
) ([]ecr.Image, []ecr.ImageFailure, error) {
	return []ecr.Image{}, []ecr.ImageFailure{}, nil
}

func (s *stubBackend) DescribeImages(_ context.Context, _ string, _ []ecr.ImageIdentifier) ([]ecr.Image, error) {
	return []ecr.Image{}, nil
}

func (s *stubBackend) BatchGetRepositoryScanningConfiguration(_ context.Context,
	_ []string,
) ([]ecr.RepositoryScanningConfiguration, []ecr.RepositoryScanningConfigurationFailure, error) {
	return []ecr.RepositoryScanningConfiguration{}, []ecr.RepositoryScanningConfigurationFailure{}, nil
}

func (s *stubBackend) CompleteLayerUpload(
	_ context.Context,
	_ string,
	_ string,
	_ []string,
) (*ecr.CompleteLayerUploadResult, error) {
	return &ecr.CompleteLayerUploadResult{}, nil
}

func (s *stubBackend) GetDownloadURLForLayer(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (s *stubBackend) InitiateLayerUpload(_ context.Context, _ string) (*ecr.LayerUploadInitiation, error) {
	return &ecr.LayerUploadInitiation{}, nil
}

func (s *stubBackend) UploadLayerPart(
	_ context.Context,
	_, _ string,
	_, _ int64,
	_ []byte,
) (*ecr.LayerUploadPartResult, error) {
	return &ecr.LayerUploadPartResult{}, nil
}

func (s *stubBackend) CreatePullThroughCacheRule(
	_ context.Context,
	_, _, _, _, _, _ string,
) (*ecr.PullThroughCacheRule, error) {
	return &ecr.PullThroughCacheRule{}, nil
}

func (s *stubBackend) DescribePullThroughCacheRules(_ context.Context, _ []string) ([]ecr.PullThroughCacheRule, error) {
	return []ecr.PullThroughCacheRule{}, nil
}

func (s *stubBackend) CreateRepositoryCreationTemplate(_ context.Context,
	_ *ecr.RepositoryCreationTemplate,
) (*ecr.RepositoryCreationTemplate, error) {
	return &ecr.RepositoryCreationTemplate{}, nil
}

func (s *stubBackend) DeleteRepositoryCreationTemplate(
	_ context.Context,
	_ string,
) (*ecr.RepositoryCreationTemplate, error) {
	return &ecr.RepositoryCreationTemplate{}, nil
}

func (s *stubBackend) DescribeRepositoryCreationTemplates(
	_ context.Context,
	_ []string,
) ([]ecr.RepositoryCreationTemplate, error) {
	return []ecr.RepositoryCreationTemplate{}, nil
}

func (s *stubBackend) DeleteLifecyclePolicy(_ context.Context, _ string) (*ecr.LifecyclePolicyResult, error) {
	return &ecr.LifecyclePolicyResult{}, nil
}

func (s *stubBackend) GetLifecyclePolicy(_ context.Context, _ string) (*ecr.LifecyclePolicyResult, error) {
	return &ecr.LifecyclePolicyResult{}, nil
}

func (s *stubBackend) GetLifecyclePolicyPreview(
	_ context.Context,
	_ string,
) (*ecr.LifecyclePolicyPreviewResult, error) {
	return &ecr.LifecyclePolicyPreviewResult{}, nil
}

func (s *stubBackend) DeletePullThroughCacheRule(_ context.Context, _ string) (*ecr.PullThroughCacheRule, error) {
	return &ecr.PullThroughCacheRule{}, nil
}

func (s *stubBackend) UpdatePullThroughCacheRule(_ context.Context, _, _, _ string) (*ecr.PullThroughCacheRule, error) {
	return &ecr.PullThroughCacheRule{}, nil
}

func (s *stubBackend) ValidatePullThroughCacheRule(
	_ context.Context,
	_ string,
) (*ecr.ValidatePullThroughCacheRuleResult, error) {
	return &ecr.ValidatePullThroughCacheRuleResult{}, nil
}

func (s *stubBackend) DeleteRegistryPolicy(_ context.Context) (*ecr.RegistryPolicyResult, error) {
	return &ecr.RegistryPolicyResult{}, nil
}

func (s *stubBackend) DescribeRegistry(_ context.Context) (*ecr.RegistryDescription, error) {
	return &ecr.RegistryDescription{}, nil
}

func (s *stubBackend) GetRegistryPolicy(_ context.Context) (*ecr.RegistryPolicyResult, error) {
	return &ecr.RegistryPolicyResult{}, nil
}

func (s *stubBackend) GetRegistryScanningConfiguration(_ context.Context) (*ecr.RegistryScanningSettings, error) {
	return &ecr.RegistryScanningSettings{}, nil
}

func (s *stubBackend) PutLifecyclePolicy(_ context.Context, _ string, _ string) (*ecr.LifecyclePolicyResult, error) {
	return &ecr.LifecyclePolicyResult{}, nil
}

func (s *stubBackend) StartLifecyclePolicyPreview(
	_ context.Context,
	_, _ string,
) (*ecr.LifecyclePolicyPreviewResult, error) {
	return &ecr.LifecyclePolicyPreviewResult{}, nil
}

func (s *stubBackend) PutRegistryPolicy(_ context.Context, _ string) (*ecr.RegistryPolicyResult, error) {
	return &ecr.RegistryPolicyResult{}, nil
}

func (s *stubBackend) PutRegistryScanningConfiguration(_ context.Context,
	_ *ecr.RegistryScanningSettings,
) (*ecr.RegistryScanningSettings, error) {
	return &ecr.RegistryScanningSettings{}, nil
}

func (s *stubBackend) PutReplicationConfiguration(
	_ context.Context,
	_ *ecr.ReplicationConfig,
) (*ecr.ReplicationConfig, error) {
	return &ecr.ReplicationConfig{}, nil
}

func (s *stubBackend) GetRepositoryPolicy(_ context.Context, _ string) (*ecr.RepositoryPolicyResult, error) {
	return &ecr.RepositoryPolicyResult{}, nil
}

func (s *stubBackend) SetRepositoryPolicy(_ context.Context, _, _ string) (*ecr.RepositoryPolicyResult, error) {
	return &ecr.RepositoryPolicyResult{}, nil
}

func (s *stubBackend) DeleteRepositoryPolicy(_ context.Context, _ string) (*ecr.RepositoryPolicyResult, error) {
	return &ecr.RepositoryPolicyResult{}, nil
}

func (s *stubBackend) GetSigningConfiguration(_ context.Context) (*ecr.SigningSettings, error) {
	return &ecr.SigningSettings{}, nil
}

func (s *stubBackend) PutSigningConfiguration(_ context.Context, _ *ecr.SigningSettings) (*ecr.SigningSettings, error) {
	return &ecr.SigningSettings{}, nil
}

func (s *stubBackend) DeleteSigningConfiguration(_ context.Context) (*ecr.SigningSettings, error) {
	return &ecr.SigningSettings{}, nil
}

func (s *stubBackend) DescribeImageSigningStatus(_ context.Context,
	_ string,
	_ ecr.ImageIdentifier,
) (*ecr.ImageSigningStatusResult, error) {
	return &ecr.ImageSigningStatusResult{}, nil
}

func (s *stubBackend) DescribeImageScanFindings(
	_ context.Context, _ string, _ ecr.ImageIdentifier, _ int, _ string,
) (*ecr.ImageScanFindingsResult, string, error) {
	return &ecr.ImageScanFindingsResult{}, "", nil
}

func (s *stubBackend) StartImageScan(
	_ context.Context,
	_ string,
	_ ecr.ImageIdentifier,
) (*ecr.ImageScanStartResult, error) {
	return &ecr.ImageScanStartResult{}, nil
}

func (s *stubBackend) ListImages(_ context.Context, _, _, _ string) ([]ecr.ImageIdentifier, error) {
	return []ecr.ImageIdentifier{}, nil
}

func (s *stubBackend) ListImageReferrers(
	_ context.Context,
	_ string,
	_ ecr.ImageIdentifier,
) ([]ecr.ImageReferrer, error) {
	return []ecr.ImageReferrer{}, nil
}

func (s *stubBackend) PutImage(_ context.Context, _ string, _ ecr.Image) (*ecr.Image, error) {
	return &ecr.Image{}, nil
}

func (s *stubBackend) PutImageScanningConfiguration(_ context.Context,
	_ string,
	_ bool,
) (*ecr.RepositoryScanningConfiguration, error) {
	return &ecr.RepositoryScanningConfiguration{}, nil
}

func (s *stubBackend) PutImageTagMutability(_ context.Context,
	_, _ string,
	_ []ecr.ImageTagMutabilityExclusionFilter,
) (*ecr.Repository, error) {
	return &ecr.Repository{}, nil
}

func (s *stubBackend) DescribeImageReplicationStatus(_ context.Context,
	_ string,
	_ ecr.ImageIdentifier,
) (*ecr.ImageReplicationStatusResult, error) {
	return &ecr.ImageReplicationStatusResult{}, nil
}

func (s *stubBackend) UpdateImageStorageClass(_ context.Context,
	_ string,
	_ ecr.ImageIdentifier,
	_ string,
) (*ecr.ImageStorageClassResult, error) {
	return &ecr.ImageStorageClassResult{}, nil
}

func (s *stubBackend) GetAccountSetting(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (s *stubBackend) PutAccountSetting(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (s *stubBackend) RegisterPullTimeUpdateExclusion(
	_ context.Context,
	_ string,
) (*ecr.PullTimeUpdateExclusion, error) {
	return &ecr.PullTimeUpdateExclusion{}, nil
}

func (s *stubBackend) DeregisterPullTimeUpdateExclusion(
	_ context.Context,
	_ string,
) (*ecr.PullTimeUpdateExclusion, error) {
	return &ecr.PullTimeUpdateExclusion{}, nil
}

func (s *stubBackend) ListPullTimeUpdateExclusions(_ context.Context) ([]ecr.PullTimeUpdateExclusion, error) {
	return []ecr.PullTimeUpdateExclusion{}, nil
}

func (s *stubBackend) UpdateRepositoryCreationTemplate(_ context.Context,
	_ *ecr.RepositoryCreationTemplate,
) (*ecr.RepositoryCreationTemplate, error) {
	return &ecr.RepositoryCreationTemplate{}, nil
}

func (s *stubBackend) TagResource(_ context.Context, _ string, _ map[string]string) error {
	return nil
}

func (s *stubBackend) UntagResource(_ context.Context, _ string, _ []string) error {
	return nil
}

func (s *stubBackend) ListTagsForResource(_ context.Context, _ string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (s *stubBackend) Reset()            {}
func (s *stubBackend) Region() string    { return "us-east-1" }
func (s *stubBackend) AccountID() string { return "000000000000" }

// ---- tests --------------------------------------------------------------

// TestECR_Handler_AcceptsBackendInterface ensures Handler works with any
// Backend implementation, not just InMemoryBackend.
func TestECR_Handler_AcceptsBackendInterface(t *testing.T) {
	t.Parallel()

	h := ecr.NewHandler(newStubBackend(), nil)

	bodyBytes, err := json.Marshal(map[string]any{"repositoryName": "my-repo"})
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AmazonEC2ContainerRegistry_V20150921.CreateRepository")
	rec := httptest.NewRecorder()

	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	repo, ok := resp["repository"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "my-repo", repo["repositoryName"])
}

// TestECR_Handler_SnapshotNilForNonSnapshottable verifies that Snapshot returns
// nil (rather than panicking) when the backend does not implement Snapshottable.
func TestECR_Handler_SnapshotNilForNonSnapshottable(t *testing.T) {
	t.Parallel()

	h := ecr.NewHandler(newStubBackend(), nil)
	assert.Nil(t, h.Snapshot(t.Context()))
}

// TestECR_Handler_RestoreNoopForNonSnapshottable verifies that Restore is a
// no-op (no panic, no error) when the backend does not implement Snapshottable.
func TestECR_Handler_RestoreNoopForNonSnapshottable(t *testing.T) {
	t.Parallel()

	h := ecr.NewHandler(newStubBackend(), nil)
	require.NoError(t, h.Restore(t.Context(), []byte(`{"repos":{}}`)))
}

// TestECR_GetAuthorizationToken_ProxyEndpointFromBackend checks that the
// proxyEndpoint in the auth token response comes from the Backend interface.
func TestECR_GetAuthorizationToken_ProxyEndpointFromBackend(t *testing.T) {
	t.Parallel()

	h := ecr.NewHandler(newStubBackend(), nil)

	bodyBytes, err := json.Marshal(map[string]any{})
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Amz-Target", "AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken")
	rec := httptest.NewRecorder()

	require.NoError(t, h.Handler()(e.NewContext(req, rec)))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	authData := resp["authorizationData"].([]any)
	entry := authData[0].(map[string]any)
	assert.Equal(t, "https://stub:5000", entry["proxyEndpoint"])
}
