package ecr_test

// repositories_test.go — verifies repositories.go: CreateRepository (naming,
// encryption, tag-mutability, tags, identity fields), DescribeRepositories
// (filtering, pagination, sorting), and DeleteRepository (force flag, cascade
// cleanup of tags/lifecycle policies/layer uploads).

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecr"
)

func TestECR_CreateRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    map[string]any
		name     string
		wantCode int
		wantARN  bool
		wantURI  bool
	}{
		{
			name:     "success",
			input:    map[string]any{"repositoryName": "my-repo"},
			wantCode: http.StatusOK,
			wantARN:  true,
			wantURI:  true,
		},
		{
			name:     "missing repository name",
			input:    map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doECRRequest(t, h, "CreateRepository", tt.input)

			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantARN || tt.wantURI {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				repo, ok := resp["repository"].(map[string]any)
				require.True(t, ok)

				if tt.wantARN {
					assert.NotEmpty(t, repo["repositoryArn"])
				}

				if tt.wantURI {
					assert.NotEmpty(t, repo["repositoryUri"])
				}
			}
		})
	}
}

func TestECR_CreateRepository_AlreadyExists(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doECRRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "my-repo"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doECRRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "my-repo"})
	require.Equal(t, http.StatusBadRequest, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "RepositoryAlreadyExistsException")
}

func TestECR_DescribeRepositories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		repos    []string
		filter   []string
		wantCode int
		wantLen  int
	}{
		{
			name:     "list all",
			repos:    []string{"repo-a", "repo-b"},
			filter:   nil,
			wantCode: http.StatusOK,
			wantLen:  2,
		},
		{
			name:     "filter by name",
			repos:    []string{"repo-a", "repo-b"},
			filter:   []string{"repo-a"},
			wantCode: http.StatusOK,
			wantLen:  1,
		},
		{
			name:     "filter not found",
			repos:    []string{"repo-a"},
			filter:   []string{"repo-missing"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for _, repoName := range tt.repos {
				rec := doECRRequest(
					t,
					h,
					"CreateRepository",
					map[string]any{"repositoryName": repoName},
				)
				require.Equal(t, http.StatusOK, rec.Code)
			}

			input := map[string]any{}
			if tt.filter != nil {
				input["repositoryNames"] = tt.filter
			}

			rec := doECRRequest(t, h, "DescribeRepositories", input)
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantLen > 0 {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				repos, ok := resp["repositories"].([]any)
				require.True(t, ok)
				assert.Len(t, repos, tt.wantLen)
			}
		})
	}
}

func TestECR_DeleteRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		create   string
		delete   string
		wantCode int
	}{
		{
			name:     "success",
			create:   "my-repo",
			delete:   "my-repo",
			wantCode: http.StatusOK,
		},
		{
			name:     "not found",
			create:   "",
			delete:   "nonexistent",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.create != "" {
				rec := doECRRequest(
					t,
					h,
					"CreateRepository",
					map[string]any{"repositoryName": tt.create},
				)
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doECRRequest(
				t,
				h,
				"DeleteRepository",
				map[string]any{"repositoryName": tt.delete},
			)
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				repo, ok := resp["repository"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.delete, repo["repositoryName"])
			}
		})
	}
}

func TestDescribeRepositories_SortedOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		repos     []string
		wantOrder []string
	}{
		{
			name:      "alphabetical_order",
			repos:     []string{"zebra", "alpha", "mango"},
			wantOrder: []string{"alpha", "mango", "zebra"},
		},
		{
			name:      "single_repo",
			repos:     []string{"only-one"},
			wantOrder: []string{"only-one"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "localhost:5000")
			for _, name := range tt.repos {
				b.AddRepositoryInternal(ecr.Repository{
					RepositoryName: name,
					RepositoryARN:  "arn:aws:ecr:us-east-1:123456789012:repository/" + name,
				})
			}

			repos, err := b.DescribeRepositories(context.Background(), nil)
			require.NoError(t, err)

			got := make([]string, 0, len(repos))
			for _, r := range repos {
				got = append(got, r.RepositoryName)
			}

			assert.Equal(t, tt.wantOrder, got)
		})
	}
}

func TestDeleteRepository_Cascade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		repoName string
		arn      string
	}{
		{
			name:     "cascade_delete",
			repoName: "cascade-repo",
			arn:      "arn:aws:ecr:us-east-1:123456789012:repository/cascade-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "localhost:5000")
			b.AddRepositoryInternal(ecr.Repository{
				RepositoryName: tt.repoName,
				RepositoryARN:  tt.arn,
			})
			b.AddLifecyclePolicyInternal(tt.repoName, `{"rules":[]}`)
			require.NoError(t, b.TagResource(context.Background(), tt.arn, map[string]string{"env": "prod"}))

			assert.Equal(t, 1, b.LifecyclePolicyCount())

			_, err := b.DeleteRepository(context.Background(), tt.repoName, false)
			require.NoError(t, err)

			assert.Equal(t, 0, b.RepositoryCount())
			assert.Equal(t, 0, b.LifecyclePolicyCount())

			tags, err := b.ListTagsForResource(context.Background(), tt.arn)
			require.NoError(t, err)
			assert.Empty(t, tags)
		})
	}
}

func TestCreateRepository_ImageTagMutability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		repoName           string
		imageTagMutability string
		wantMutability     string
	}{
		{
			name:               "explicit_immutable",
			repoName:           "immutable-repo",
			imageTagMutability: "IMMUTABLE",
			wantMutability:     "IMMUTABLE",
		},
		{
			name:               "default_mutable",
			repoName:           "default-repo",
			imageTagMutability: "",
			wantMutability:     "MUTABLE",
		},
		{
			name:               "explicit_mutable",
			repoName:           "explicit-mutable",
			imageTagMutability: "MUTABLE",
			wantMutability:     "MUTABLE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "localhost:5000")
			repo, err := b.CreateRepository(context.Background(), tt.repoName, tt.imageTagMutability, false, "", "")
			require.NoError(t, err)
			assert.Equal(t, tt.wantMutability, repo.ImageTagMutability)
		})
	}
}

func TestRepositoryCRUD_EncryptionVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		repoName       string
		encryptionType string
		kmsKey         string
	}{
		{
			name:           "aes256_default",
			repoName:       "repo-aes",
			encryptionType: "AES256",
		},
		{
			name:           "kms_encryption",
			repoName:       "repo-kms",
			encryptionType: "KMS",
			kmsKey:         "arn:aws:kms:us-east-1:123456789012:key/abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			repo, err := b.CreateRepository(context.Background(), tt.repoName, "", false, tt.encryptionType, tt.kmsKey)
			require.NoError(t, err)
			assert.Equal(t, tt.repoName, repo.RepositoryName)
			assert.NotEmpty(t, repo.RepositoryARN)

			repos, err := b.DescribeRepositories(context.Background(), nil)
			require.NoError(t, err)
			found := false
			for _, r := range repos {
				if r.RepositoryName == tt.repoName {
					found = true
					assert.Equal(t, tt.encryptionType, r.EncryptionType)
					if tt.kmsKey != "" {
						assert.Equal(t, tt.kmsKey, r.KMSKey)
					}
				}
			}
			assert.True(t, found, "created repo must appear in DescribeRepositories")

			_, err = b.DeleteRepository(context.Background(), tt.repoName, false)
			require.NoError(t, err)
			assert.Equal(t, 0, b.RepositoryCount())
		})
	}
}

func TestDeleteRepository_NonEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		force    bool
		wantCode int
	}{
		{name: "no_force_rejected", force: false, wantCode: http.StatusBadRequest},
		{name: "force_succeeds", force: true, wantCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create repo.
			rec := doECRRequest(t, h, "CreateRepository",
				map[string]any{"repositoryName": "nonempty-repo"})
			require.Equal(t, http.StatusOK, rec.Code)

			// Push an image so the repo is non-empty.
			rec = doECRRequest(t, h, "PutImage", map[string]any{
				"repositoryName": "nonempty-repo",
				"imageManifest":  `{"schemaVersion":2}`,
				"imageTag":       "v1",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			// Attempt delete.
			rec = doECRRequest(t, h, "DeleteRepository", map[string]any{
				"repositoryName": "nonempty-repo",
				"force":          tt.force,
			})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "RepositoryNotEmptyException")
			}
		})
	}
}

func TestDeleteRepository_CleansUpLayerUploads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		uploadsBefore int
	}{
		{name: "no_uploads", uploadsBefore: 0},
		{name: "single_upload", uploadsBefore: 1},
		{name: "many_uploads", uploadsBefore: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "localhost:5000")
			ctx := context.Background()

			_, err := b.CreateRepository(ctx, "myrepo", "MUTABLE", false, "", "")
			require.NoError(t, err)

			for range tt.uploadsBefore {
				_, err = b.InitiateLayerUpload(ctx, "myrepo")
				require.NoError(t, err)
			}
			require.Equal(t, tt.uploadsBefore, b.LayerUploadCount())

			_, err = b.DeleteRepository(ctx, "myrepo", false)
			require.NoError(t, err)

			assert.Equal(t, 0, b.LayerUploadCount(),
				"all layer uploads must be removed when repository is deleted")
		})
	}
}

func TestCreateRepository_KMSKey_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "kms-repo",
		"encryptionConfiguration": map[string]any{
			"encryptionType": "KMS",
			"kmsKey":         "arn:aws:kms:us-east-1:123456789012:key/mrk-abc123",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo, ok := out["repository"].(map[string]any)
	require.True(t, ok, "response should have repository field")

	enc, ok := repo["encryptionConfiguration"].(map[string]any)
	require.True(t, ok, "encryptionConfiguration should be present")
	assert.Equal(t, "KMS", enc["encryptionType"])
	assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/mrk-abc123", enc["kmsKey"])
}

func TestCreateRepository_KMSKey_PresentInDescribe(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "kms-repo2",
		"encryptionConfiguration": map[string]any{
			"encryptionType": "KMS",
			"kmsKey":         "arn:aws:kms:us-east-1:123456789012:key/key-002",
		},
	})

	rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"repositoryNames": []string{"kms-repo2"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repos, _ := out["repositories"].([]any)
	require.Len(t, repos, 1)

	repo, _ := repos[0].(map[string]any)
	enc, ok := repo["encryptionConfiguration"].(map[string]any)
	require.True(t, ok, "DescribeRepositories must return encryptionConfiguration")
	assert.Equal(t, "KMS", enc["encryptionType"])
	assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/key-002", enc["kmsKey"],
		"kmsKey must be persisted and returned by DescribeRepositories")
}

func TestCreateRepository_KMSKey_Backend_DeepCopy(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	repo, err := b.CreateRepository(context.Background(), "kms-deep", "MUTABLE", false, "KMS",
		"arn:aws:kms:us-east-1:123456789012:key/mrk-copy")
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/mrk-copy", repo.KMSKey)

	// Mutating returned copy must not affect stored state.
	repo.KMSKey = "mutated"
	repos, err := b.DescribeRepositories(context.Background(), []string{"kms-deep"})
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/mrk-copy", repos[0].KMSKey,
		"returned copy mutation must not affect stored state")
}

func TestCreateRepository_DefaultEncryptionType_IsAES256(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "default-enc",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo := out["repository"].(map[string]any)
	enc, ok := repo["encryptionConfiguration"].(map[string]any)
	require.True(t, ok, "encryptionConfiguration must always be present")
	assert.Equal(t, "AES256", enc["encryptionType"],
		"default encryption type must be AES256")
}

func TestCreateRepository_KMSKey_AbsentWhenNotSet(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "no-kms",
		"encryptionConfiguration": map[string]any{
			"encryptionType": "AES256",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo := out["repository"].(map[string]any)
	enc, _ := repo["encryptionConfiguration"].(map[string]any)
	kmsKey, hasKey := enc["kmsKey"]
	if hasKey {
		assert.Empty(t, kmsKey, "kmsKey must be absent or empty for AES256 repos")
	}
}

func TestDeleteRepository_NonEmpty_WithoutForce_Returns400(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "nonempty-repo-2")
	mustPutImage(t, h, "nonempty-repo-2", "latest", `{"schemaVersion":2}`)

	rec := doAccuracy(t, h, "DeleteRepository", map[string]any{
		"repositoryName": "nonempty-repo-2",
		// force omitted (defaults to false)
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"deleting a non-empty repo without force must return 400")

	out := parseAccuracy(t, rec)
	errType, _ := out["__type"].(string)
	assert.Equal(t, "RepositoryNotEmptyException", errType)
}

func TestDeleteRepository_NonEmpty_WithForce_Succeeds(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "force-repo")
	mustPutImage(t, h, "force-repo", "v1", `{"schemaVersion":2}`)
	mustPutImage(t, h, "force-repo", "v2", `{"schemaVersion":2,"v":"2"}`)

	rec := doAccuracy(t, h, "DeleteRepository", map[string]any{
		"repositoryName": "force-repo",
		"force":          true,
	})
	require.Equal(t, http.StatusOK, rec.Code,
		"force-delete of non-empty repo must succeed")

	out := parseAccuracy(t, rec)
	repo, _ := out["repository"].(map[string]any)
	assert.Equal(t, "force-repo", repo["repositoryName"])
}

func TestDeleteRepository_Empty_WithoutForce_Succeeds(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "empty-del-repo")

	rec := doAccuracy(t, h, "DeleteRepository", map[string]any{
		"repositoryName": "empty-del-repo",
	})
	require.Equal(t, http.StatusOK, rec.Code,
		"empty repo can be deleted without force")
}

func TestDeleteRepository_Force_False_ExplicitlySet(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "explicit-noforce")
	mustPutImage(t, h, "explicit-noforce", "img", `{"schemaVersion":2}`)

	rec := doAccuracy(t, h, "DeleteRepository", map[string]any{
		"repositoryName": "explicit-noforce",
		"force":          false,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"explicit force=false must still block non-empty repo deletion")
}

// TestDeleteRepository_BackendEnforcesForceCheck calls the backend directly,
// bypassing the handler, to prove the empty/force check now lives inside
// DeleteRepository's own write-lock acquisition rather than in a separate
// pre-check (gopherstack-e4qn: the old handler-side DescribeImages/Delete
// split left a TOCTOU window for a concurrent PutImage).
func TestDeleteRepository_BackendEnforcesForceCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pushImage bool
		force     bool
		wantErr   bool
	}{
		{name: "nonempty_rejected_without_force", pushImage: true, force: false, wantErr: true},
		{name: "empty_succeeds_without_force", pushImage: false, force: false, wantErr: false},
		{name: "nonempty_succeeds_with_force", pushImage: true, force: true, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newAccuracyBackend()
			ctx := context.Background()

			_, err := b.CreateRepository(ctx, "toctou-repo", "MUTABLE", false, "", "")
			require.NoError(t, err)

			if tt.pushImage {
				_, err = b.PutImage(ctx, "toctou-repo", ecr.Image{
					ImageManifest: `{"schemaVersion":2}`,
					ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
				})
				require.NoError(t, err)
			}

			_, err = b.DeleteRepository(ctx, "toctou-repo", tt.force)
			if tt.wantErr {
				require.ErrorIs(t, err, ecr.ErrRepositoryNotEmpty)

				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDeleteRepository_Cascades_LayerUploads(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "cascade-repo-2", "MUTABLE", false, "", "")
	require.NoError(t, err)

	// Initiate a layer upload for the repo.
	result, err := b.InitiateLayerUpload(context.Background(), "cascade-repo-2")
	require.NoError(t, err)
	uploadID := result.UploadID

	// Record how many layer uploads exist before deletion.
	assert.Equal(t, 1, b.LayerUploadCount(), "one in-progress upload expected before delete")

	// Delete the repository.
	_, err = b.DeleteRepository(context.Background(), "cascade-repo-2", false)
	require.NoError(t, err)

	assert.Equal(t, 0, b.LayerUploadCount(),
		"DeleteRepository must clean up in-progress layer uploads for the deleted repo (upload %s)", uploadID)
}

func TestDescribeRepositories_MaxResults_LimitsResponse(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	for _, name := range []string{"alpha", "bravo", "charlie", "delta", "echo"} {
		mustCreateRepo(t, h, name)
	}

	rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"maxResults": 2,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repos, _ := out["repositories"].([]any)
	assert.Len(t, repos, 2, "maxResults=2 must return at most 2 repositories")
	assert.NotEmpty(t, out["nextToken"], "nextToken must be present when more repos exist")
}

func TestDescribeRepositories_NextToken_ContinuesPagination(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	for _, name := range []string{"aaa", "bbb", "ccc", "ddd", "eee"} {
		mustCreateRepo(t, h, name)
	}

	// Fetch first page.
	rec1 := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"maxResults": 3,
	})
	require.Equal(t, http.StatusOK, rec1.Code)
	out1 := parseAccuracy(t, rec1)
	page1, _ := out1["repositories"].([]any)
	require.Len(t, page1, 3)
	token, _ := out1["nextToken"].(string)
	require.NotEmpty(t, token)

	// Fetch second page using the token.
	rec2 := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"nextToken": token,
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	out2 := parseAccuracy(t, rec2)
	page2, _ := out2["repositories"].([]any)
	assert.NotEmpty(t, page2, "second page must have results")
}

func TestDescribeRepositories_FullPagination_CoversAll(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	names := []string{"r1", "r2", "r3", "r4", "r5", "r6"}
	for _, name := range names {
		mustCreateRepo(t, h, name)
	}

	seen := map[string]bool{}
	token := ""

	for {
		body := map[string]any{"maxResults": 2}
		if token != "" {
			body["nextToken"] = token
		}

		rec := doAccuracy(t, h, "DescribeRepositories", body)
		require.Equal(t, http.StatusOK, rec.Code)

		out := parseAccuracy(t, rec)
		page, _ := out["repositories"].([]any)

		for _, r := range page {
			repo := r.(map[string]any)
			seen[repo["repositoryName"].(string)] = true
		}

		next, _ := out["nextToken"].(string)
		if next == "" {
			break
		}

		token = next
	}

	assert.Len(t, seen, len(names), "full pagination must enumerate all repositories exactly once")
	for _, name := range names {
		assert.True(t, seen[name], "repo %s must appear in paginated results", name)
	}
}

func TestDescribeRepositories_NoNextToken_AtEnd(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "only-one-repo")

	rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"maxResults": 10,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	nextToken, hasToken := out["nextToken"]
	if hasToken {
		assert.Empty(t, nextToken, "nextToken must be absent or empty on the last page")
	}
}

func TestDescribeRepositories_SortedByName(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	for _, name := range []string{"zzz", "aaa", "mmm", "bbb"} {
		mustCreateRepo(t, h, name)
	}

	rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repos, _ := out["repositories"].([]any)
	require.Len(t, repos, 4)

	names := make([]string, 0, len(repos))
	for _, r := range repos {
		names = append(names, r.(map[string]any)["repositoryName"].(string))
	}

	assert.Equal(t, []string{"aaa", "bbb", "mmm", "zzz"}, names,
		"DescribeRepositories must return repos sorted alphabetically by name")
}

func TestDescribeRepositories_ReturnedSlice_Isolated(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "iso-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	repos1, err := b.DescribeRepositories(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, repos1, 1)

	// Mutate the returned struct — must not affect stored state.
	repos1[0].RepositoryName = "mutated-name"
	repos1[0].ImageTagMutability = "IMMUTABLE"

	repos2, err := b.DescribeRepositories(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, repos2, 1)
	assert.Equal(t, "iso-repo", repos2[0].RepositoryName,
		"mutating the returned slice must not affect the stored repository")
	assert.Equal(t, "MUTABLE", repos2[0].ImageTagMutability,
		"mutating returned copy must not affect stored imageTagMutability")
}

func TestRepository_ExclusionFilters_DeepCopy(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "filter-repo", "MUTABLE", false, "", "")
	require.NoError(t, err)

	filters := []ecr.ImageTagMutabilityExclusionFilter{
		{Filter: "v1.*", FilterType: "WILDCARD"},
	}
	_, err = b.PutImageTagMutability(context.Background(), "filter-repo", "IMMUTABLE", filters)
	require.NoError(t, err)

	// Mutate the original filter slice.
	filters[0].Filter = "mutated"

	repos, err := b.DescribeRepositories(context.Background(), []string{"filter-repo"})
	require.NoError(t, err)
	require.Len(t, repos, 1)
	require.Len(t, repos[0].ImageTagMutabilityExclusionFilters, 1)
	assert.Equal(t, "v1.*", repos[0].ImageTagMutabilityExclusionFilters[0].Filter,
		"stored ExclusionFilters must be isolated from caller mutations")
}

func TestCreateRepository_ARN_Format(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "my/namespace/app",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo := out["repository"].(map[string]any)

	arn, _ := repo["repositoryArn"].(string)
	assert.Contains(t, arn, "arn:aws:ecr:", "ARN must begin with arn:aws:ecr:")
	assert.Contains(t, arn, "123456789012", "ARN must contain the account ID")
	assert.Contains(t, arn, "my/namespace/app", "ARN must contain the repository name")
}

func TestCreateRepository_URI_Format(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "uri-test-repo",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo := out["repository"].(map[string]any)

	uri, _ := repo["repositoryUri"].(string)
	assert.Contains(t, uri, "uri-test-repo", "repositoryUri must contain the repository name")
}

func TestCreateRepository_RegistryID_Present(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "registry-id-check",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo := out["repository"].(map[string]any)

	registryID, _ := repo["registryId"].(string)
	assert.Equal(t, "123456789012", registryID,
		"registryId must equal the account ID configured on the backend")
}

func TestDescribeRepositories_ByName_ReturnsOnly_Requested(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	for _, name := range []string{"cat", "dog", "fish"} {
		mustCreateRepo(t, h, name)
	}

	rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"repositoryNames": []string{"cat", "fish"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repos, _ := out["repositories"].([]any)
	require.Len(t, repos, 2)

	names := map[string]bool{}
	for _, r := range repos {
		names[r.(map[string]any)["repositoryName"].(string)] = true
	}
	assert.True(t, names["cat"])
	assert.True(t, names["fish"])
	assert.False(t, names["dog"], "dog must not be in the response when not requested")
}

func TestDescribeRepositories_UnknownName_Returns404(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"repositoryNames": []string{"does-not-exist"},
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestCreateRepository_AlreadyExists_Returns400(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "dup-repo")

	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "dup-repo",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	out := parseAccuracy(t, rec)
	assert.Equal(t, "RepositoryAlreadyExistsException", out["__type"])
}

func TestCreateRepository_ImageTagMutability_IMMUTABLE(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName":     "immutable-create",
		"imageTagMutability": "IMMUTABLE",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo, _ := out["repository"].(map[string]any)
	assert.Equal(t, "IMMUTABLE", repo["imageTagMutability"])
}

func TestCreateRepository_DefaultImageTagMutability_MUTABLE(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "default-mut",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo, _ := out["repository"].(map[string]any)
	assert.Equal(t, "MUTABLE", repo["imageTagMutability"],
		"default imageTagMutability must be MUTABLE")
}

func TestCreateRepository_ScanOnPush_True(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "scan-on-push",
		"imageScanningConfiguration": map[string]any{
			"scanOnPush": true,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo, _ := out["repository"].(map[string]any)
	scanCfg, _ := repo["imageScanningConfiguration"].(map[string]any)
	assert.Equal(t, true, scanCfg["scanOnPush"])
}

func TestCreateRepository_NamespaceSlash(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "org/team/app",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo, _ := out["repository"].(map[string]any)
	assert.Equal(t, "org/team/app", repo["repositoryName"])
	arn, _ := repo["repositoryArn"].(string)
	assert.Contains(t, arn, "org/team/app", "ARN must contain full namespaced name")
	uri, _ := repo["repositoryUri"].(string)
	assert.Contains(t, uri, "org/team/app", "URI must contain full namespaced name")
}

func TestCreateRepository_KMS_RoundTrip_Backend(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	kmsKey := "arn:aws:kms:us-east-1:123456789012:key/mrk-xyz"
	repo, err := b.CreateRepository(context.Background(), "kms-be", "MUTABLE", false, "KMS", kmsKey)
	require.NoError(t, err)
	assert.Equal(t, "KMS", repo.EncryptionType)
	assert.Equal(t, kmsKey, repo.KMSKey)

	repos, err := b.DescribeRepositories(context.Background(), []string{"kms-be"})
	require.NoError(t, err)
	assert.Equal(t, kmsKey, repos[0].KMSKey,
		"KMS key must persist in DescribeRepositories")
}

func TestCreateRepository_CreatedAt_IsPresent(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "created-at-repo",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo, _ := out["repository"].(map[string]any)
	createdAt, ok := repo["createdAt"].(float64)
	require.True(t, ok, "createdAt must be a numeric Unix timestamp")
	assert.Greater(t, createdAt, float64(0), "createdAt must be non-zero")
}

func TestRepository_RegistryID_MatchesAccountID(t *testing.T) {
	t.Parallel()

	b := ecr.NewInMemoryBackend("111111111111", "us-west-2", "")
	h := ecr.NewHandler(b, nil)

	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "reg-id-check",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo, _ := out["repository"].(map[string]any)
	assert.Equal(t, "111111111111", repo["registryId"])
}

func TestRepository_ARN_ContainsRegion(t *testing.T) {
	t.Parallel()

	b := ecr.NewInMemoryBackend("123456789012", "ap-northeast-1", "")
	h := ecr.NewHandler(b, nil)

	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "arn-region",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo, _ := out["repository"].(map[string]any)
	arn, _ := repo["repositoryArn"].(string)
	assert.Contains(t, arn, "ap-northeast-1", "ARN must contain the configured region")
}

func TestRepository_URI_ContainsEndpoint(t *testing.T) {
	t.Parallel()

	b := ecr.NewInMemoryBackend("123456789012", "us-east-1", "myregistry.local:5000")
	h := ecr.NewHandler(b, nil)

	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "uri-endpoint",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repo, _ := out["repository"].(map[string]any)
	uri, _ := repo["repositoryUri"].(string)
	assert.Contains(t, uri, "myregistry.local:5000",
		"repositoryUri must use the configured endpoint")
	assert.Contains(t, uri, "uri-endpoint")
}

func TestDescribeRepositories_Returns_EncryptionConfiguration(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "enc-check",
		"encryptionConfiguration": map[string]any{
			"encryptionType": "AES256",
		},
	})

	rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"repositoryNames": []string{"enc-check"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repos, _ := out["repositories"].([]any)
	require.Len(t, repos, 1)
	repo, _ := repos[0].(map[string]any)
	enc, ok := repo["encryptionConfiguration"].(map[string]any)
	require.True(t, ok, "encryptionConfiguration must be present in DescribeRepositories")
	assert.Equal(t, "AES256", enc["encryptionType"])
}

func TestDescribeRepositories_EmptyRequest_ReturnsAll(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	for _, name := range []string{"repo-x", "repo-y", "repo-z"} {
		mustCreateRepo(t, h, name)
	}

	rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repos, _ := out["repositories"].([]any)
	assert.Len(t, repos, 3)
}

func TestCreateRepository_Empty_Name_Returns400(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	rec := doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName": "",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeleteRepository_Clears_TagIndex(t *testing.T) {
	t.Parallel()

	b := newAccuracyBackend()
	_, err := b.CreateRepository(context.Background(), "del-idx", "MUTABLE", false, "", "")
	require.NoError(t, err)

	_, err = b.PutImage(context.Background(), "del-idx", ecr.Image{
		ImageManifest: `{"schemaVersion":2}`,
		ImageID:       ecr.ImageIdentifier{ImageTag: "v1"},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, b.RepoTagCount("del-idx"))

	_, err = b.DeleteRepository(context.Background(), "del-idx", true)
	require.NoError(t, err)

	assert.Equal(t, 0, b.RepoTagCount("del-idx"),
		"DeleteRepository must clear the tagIndex for that repo")
}

func TestDeleteRepository_Force_Clears_MultiTag(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()

	doAccuracy(t, h, "CreateRepository", map[string]any{
		"repositoryName":     "force-multi",
		"imageTagMutability": "MUTABLE",
	})

	manifest := `{"schemaVersion":2,"multi":true}`
	mustPutImage(t, h, "force-multi", "t1", manifest)
	mustPutImage(t, h, "force-multi", "t2", manifest)

	rec := doAccuracy(t, h, "DeleteRepository", map[string]any{
		"repositoryName": "force-multi",
		"force":          true,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Repository must be gone.
	descRec := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"repositoryNames": []string{"force-multi"},
	})
	assert.Equal(t, http.StatusNotFound, descRec.Code)
}

func TestDescribeRepositories_Pagination_NextToken(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	for i := range 5 {
		mustCreateRepo(t, h, "page-repo-"+string(rune('0'+i)))
	}

	// First page: maxResults=2.
	rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"maxResults": 2,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	repos, _ := out["repositories"].([]any)
	require.Len(t, repos, 2)
	nextToken, _ := out["nextToken"].(string)
	require.NotEmpty(t, nextToken, "maxResults=2 with 5 repos must return a nextToken")

	// Second page.
	rec2 := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"maxResults": 2,
		"nextToken":  nextToken,
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	out2 := parseAccuracy(t, rec2)
	repos2, _ := out2["repositories"].([]any)
	require.Len(t, repos2, 2)

	// Page 1 and page 2 must not overlap.
	page1Names := make(map[string]bool)
	for _, r := range repos {
		rm := r.(map[string]any)
		page1Names[rm["repositoryName"].(string)] = true
	}
	for _, r := range repos2 {
		rm := r.(map[string]any)
		assert.False(t, page1Names[rm["repositoryName"].(string)], "page 2 must not overlap page 1")
	}
}

func TestDescribeRepositories_Pagination_LastPage_NoNextToken(t *testing.T) {
	t.Parallel()

	h := newAccuracyHandler()
	mustCreateRepo(t, h, "last-page-a")
	mustCreateRepo(t, h, "last-page-b")

	rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{
		"maxResults": 10,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	out := parseAccuracy(t, rec)
	nextToken, hasToken := out["nextToken"].(string)
	if hasToken {
		assert.Empty(t, nextToken, "last page must not return a nextToken")
	}
}

// TestDescribeRepositories_OpaqueNextToken verifies that the nextToken
// emitted by DescribeRepositories is base64-opaque and round-trips correctly.
func TestDescribeRepositories_OpaqueNextToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		repoNames  []string
		maxResults int
		wantNext   bool
	}{
		{
			name:       "no_next_token_when_results_fit",
			repoNames:  []string{"alpha", "beta"},
			maxResults: 10,
			wantNext:   false,
		},
		{
			name:       "next_token_emitted_when_truncated",
			repoNames:  []string{"alpha", "beta", "gamma"},
			maxResults: 2,
			wantNext:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newHandlerWithBackend()
			ctx := context.Background()

			for _, name := range tt.repoNames {
				_, err := b.CreateRepository(ctx, name, "MUTABLE", false, "", "")
				require.NoError(t, err)
			}

			rec := doAccuracy(t, h, "DescribeRepositories", map[string]any{
				"maxResults": tt.maxResults,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			out := parseAccuracy(t, rec)
			nextToken, _ := out["nextToken"].(string)

			if !tt.wantNext {
				assert.Empty(t, nextToken, "should not emit nextToken when all results fit")

				return
			}

			require.NotEmpty(t, nextToken, "should emit nextToken when truncated")

			// Token must be valid base64.
			decoded, err := base64.StdEncoding.DecodeString(nextToken)
			require.NoError(t, err, "nextToken must be valid base64")

			// The decoded value must be a known repo name — opaque to callers,
			// but internally the cursor is the repo name.
			cursorName := string(decoded)
			assert.Contains(t, tt.repoNames, cursorName, "decoded cursor must be a repo name")

			// Round-trip: using the token must return the next page.
			rec2 := doAccuracy(t, h, "DescribeRepositories", map[string]any{
				"maxResults": tt.maxResults,
				"nextToken":  nextToken,
			})
			require.Equal(t, http.StatusOK, rec2.Code)

			out2 := parseAccuracy(t, rec2)
			repos2, _ := out2["repositories"].([]any)
			assert.NotEmpty(t, repos2, "second page must contain repositories")

			// The two pages together must cover all repos.
			repos1, _ := out["repositories"].([]any)
			assert.Equal(t, len(tt.repoNames), len(repos1)+len(repos2))
		})
	}
}
