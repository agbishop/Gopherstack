package codecommit_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_CreateCommit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		repoName   string
		branchName string
		authorName string
		wantStatus int
	}{
		{
			name:       "success",
			repoName:   "repo",
			branchName: "main",
			authorName: "Alice",
			wantStatus: http.StatusOK,
		},
		{
			name:       "repo_not_found",
			repoName:   "missing-repo",
			branchName: "main",
			authorName: "Alice",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "missing_repo_name",
			repoName:   "",
			branchName: "main",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_branch_name",
			repoName:   "repo",
			branchName: "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.repoName == "repo" {
				rec := doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doRequest(t, h, "CreateCommit", map[string]any{
				"repositoryName": tt.repoName,
				"branchName":     tt.branchName,
				"authorName":     tt.authorName,
				"commitMessage":  "initial commit",
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["commitId"])
				assert.NotEmpty(t, resp["treeId"])
			}
		})
	}
}

// TestHandler_CreateCommit_FilesAddedBlobID verifies CreateCommitOutput.filesAdded reports the
// real per-file blob ID (a required AWS field) instead of a hardcoded empty string, and that
// each entry's blobId is independently usable via GetBlob.
func TestHandler_CreateCommit_FilesAddedBlobID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "cc-blob-repo"})

	rec := doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "cc-blob-repo",
		"branchName":     "main",
		"putFiles": []map[string]any{
			{"filePath": "a.txt", "fileContent": "YQ=="},
			{"filePath": "b.txt", "fileContent": "Yg=="},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	filesAdded, ok := resp["filesAdded"].([]any)
	require.True(t, ok)
	require.Len(t, filesAdded, 2)

	seen := map[string]string{}
	for _, raw := range filesAdded {
		entry, entryOK := raw.(map[string]any)
		require.True(t, entryOK)
		blobID, _ := entry["blobId"].(string)
		// FileMetadata's real wire key is "absolutePath", not "filePath"
		// (deserializers.go's awsAwsjson11_deserializeDocumentFileMetadata).
		filePath, _ := entry["absolutePath"].(string)
		assert.NotEmpty(t, blobID, "filesAdded[%s].blobId must be non-empty", filePath)
		seen[filePath] = blobID
	}
	assert.NotEqual(t, seen["a.txt"], seen["b.txt"], "each file must get its own distinct blob ID")

	// Each reported blob ID must round-trip through GetBlob.
	blobRec := doRequest(t, h, "GetBlob", map[string]any{
		"repositoryName": "cc-blob-repo",
		"blobId":         seen["a.txt"],
	})
	require.Equal(t, http.StatusOK, blobRec.Code)
}

// TestHandler_CreateCommit_FilesDeletedBlobID verifies CreateCommitOutput's
// filesDeleted entries report the real blob ID that was removed from the
// tree (mirroring the blobId fix already applied to filesAdded and to the
// standalone DeleteFile op), instead of omitting blobId entirely.
func TestHandler_CreateCommit_FilesDeletedBlobID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "cc-delete-blob-repo"})

	addRec := doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "cc-delete-blob-repo",
		"branchName":     "main",
		"putFiles": []map[string]any{
			{"filePath": "gone.txt", "fileContent": "Z29uZQ=="},
		},
	})
	require.Equal(t, http.StatusOK, addRec.Code)

	var addResp map[string]any
	require.NoError(t, json.Unmarshal(addRec.Body.Bytes(), &addResp))
	filesAdded, ok := addResp["filesAdded"].([]any)
	require.True(t, ok)
	require.Len(t, filesAdded, 1)
	addedEntry, ok := filesAdded[0].(map[string]any)
	require.True(t, ok)
	addedBlobID, _ := addedEntry["blobId"].(string)
	require.NotEmpty(t, addedBlobID)

	delRec := doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "cc-delete-blob-repo",
		"branchName":     "main",
		"deleteFiles": []map[string]any{
			{"filePath": "gone.txt"},
		},
	})
	require.Equal(t, http.StatusOK, delRec.Code)

	var delResp map[string]any
	require.NoError(t, json.Unmarshal(delRec.Body.Bytes(), &delResp))
	filesDeleted, ok := delResp["filesDeleted"].([]any)
	require.True(t, ok)
	require.Len(t, filesDeleted, 1)
	deletedEntry, ok := filesDeleted[0].(map[string]any)
	require.True(t, ok)
	// FileMetadata's real wire key is "absolutePath", not "filePath"
	// (deserializers.go's awsAwsjson11_deserializeDocumentFileMetadata) --
	// a client reading FilesDeleted[i].AbsolutePath previously always saw "".
	assert.Equal(t, "gone.txt", deletedEntry["absolutePath"])
	assert.Equal(t, addedBlobID, deletedEntry["blobId"],
		"filesDeleted[].blobId must report the blob that was removed from the tree")
}

func TestHandler_BatchGetCommits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		wantFoundCount int
		wantErrCount   int
		wantStatus     int
		seedCommits    int
		requestMissing bool
	}{
		{
			name:           "all_found",
			seedCommits:    2,
			wantFoundCount: 2,
			wantErrCount:   0,
			wantStatus:     http.StatusOK,
		},
		{
			name:           "partial_found",
			seedCommits:    1,
			requestMissing: true,
			wantFoundCount: 1,
			wantErrCount:   1,
			wantStatus:     http.StatusOK,
		},
		{
			name:       "missing_repo",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.name == "missing_repo" {
				rec := doRequest(t, h, "BatchGetCommits", map[string]any{
					"repositoryName": "",
					"commitIds":      []string{"abc"},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				return
			}

			rec := doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})
			require.Equal(t, http.StatusOK, rec.Code)

			var commitIDs []string
			for range tt.seedCommits {
				r := doRequest(t, h, "CreateCommit", map[string]any{
					"repositoryName": "repo",
					"branchName":     "main",
					"commitMessage":  "commit",
				})
				require.Equal(t, http.StatusOK, r.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(r.Body.Bytes(), &resp))
				commitIDs = append(commitIDs, resp["commitId"].(string))
			}

			if tt.requestMissing {
				commitIDs = append(commitIDs, "nonexistent-commit-id")
			}

			rec = doRequest(t, h, "BatchGetCommits", map[string]any{
				"repositoryName": "repo",
				"commitIds":      commitIDs,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			commits, _ := resp["commits"].([]any)
			assert.Len(t, commits, tt.wantFoundCount)

			errs, _ := resp["errors"].([]any)
			assert.Len(t, errs, tt.wantErrCount)
		})
	}
}

func TestHandler_BatchGetCommits_RepoNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "BatchGetCommits", map[string]any{
		"repositoryName": "nonexistent-repo",
		"commitIds":      []string{"abc123"},
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_BatchGetCommits_EmptyIDsRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "BatchGetCommits", map[string]any{
		"repositoryName": "repo",
		"commitIds":      []string{},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_BatchGetCommits_FullCommitMap(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "repo",
		"branchName":     "main",
		"authorName":     "Alice",
		"email":          "alice@example.com",
		"commitMessage":  "test commit",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	commitID := createResp["commitId"].(string)

	rec = doRequest(t, h, "BatchGetCommits", map[string]any{
		"repositoryName": "repo",
		"commitIds":      []string{commitID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	commits, ok := resp["commits"].([]any)
	require.True(t, ok)
	require.Len(t, commits, 1)

	c := commits[0].(map[string]any)
	assert.Equal(t, commitID, c["commitId"])
	assert.NotNil(t, c["author"], "author sub-object should be present")
	assert.NotNil(t, c["committer"], "committer sub-object should be present")

	author, ok := c["author"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Alice", author["name"])
	assert.Equal(t, "alice@example.com", author["email"])

	// parents should be a JSON array (not null) for first commit
	parents, ok := c["parents"].([]any)
	require.True(t, ok)
	assert.Empty(t, parents)
}

func TestHandler_BatchGetCommits_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		wantCode     int
		includeReal  bool
		includeFake  bool
		wantFoundLen int
		wantErrLen   int
	}{
		{
			name:         "real_commit_found",
			wantCode:     http.StatusOK,
			includeReal:  true,
			includeFake:  false,
			wantFoundLen: 1,
			wantErrLen:   0,
		},
		{
			name:         "fake_commit_in_errors",
			wantCode:     http.StatusOK,
			includeReal:  false,
			includeFake:  true,
			wantFoundLen: 0,
			wantErrLen:   1,
		},
		{
			name:         "mixed",
			wantCode:     http.StatusOK,
			includeReal:  true,
			includeFake:  true,
			wantFoundLen: 1,
			wantErrLen:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			setupRepoAndBranch(t, h, "repo")

			// Get a real commit ID.
			branchRec := doRequest(t, h, "GetBranch", map[string]any{
				"repositoryName": "repo",
				"branchName":     "main",
			})
			require.Equal(t, http.StatusOK, branchRec.Code)

			var brResp map[string]any
			require.NoError(t, json.Unmarshal(branchRec.Body.Bytes(), &brResp))
			realCommitID := brResp["branch"].(map[string]any)["commitId"].(string)

			var ids []string
			if tt.includeReal {
				ids = append(ids, realCommitID)
			}
			if tt.includeFake {
				ids = append(ids, "00000000-0000-0000-0000-000000000000")
			}

			rec := doRequest(t, h, "BatchGetCommits", map[string]any{
				"repositoryName": "repo",
				"commitIds":      ids,
			})
			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			commits := resp["commits"].([]any)
			errors := resp["errors"].([]any)
			assert.Len(t, commits, tt.wantFoundLen)
			assert.Len(t, errors, tt.wantErrLen)
		})
	}
}

func TestHandler_CommitParentTracking(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})
	require.Equal(t, http.StatusOK, rec.Code)

	// First commit — no parents
	rec = doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "repo",
		"branchName":     "main",
		"commitMessage":  "initial",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var firstResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &firstResp))
	firstCommitID := firstResp["commitId"].(string)

	// Second commit on same branch — should have firstCommitID as parent
	rec = doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "repo",
		"branchName":     "main",
		"commitMessage":  "second",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var secondResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &secondResp))
	secondCommitID := secondResp["commitId"].(string)

	// Fetch both commits via BatchGetCommits and check parent linkage
	rec = doRequest(t, h, "BatchGetCommits", map[string]any{
		"repositoryName": "repo",
		"commitIds":      []string{firstCommitID, secondCommitID},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var batchResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &batchResp))

	commits, ok := batchResp["commits"].([]any)
	require.True(t, ok)
	require.Len(t, commits, 2)

	// Find the second commit and verify it has the first as parent
	for _, raw := range commits {
		c := raw.(map[string]any)
		if c["commitId"].(string) == secondCommitID {
			parents, _ := c["parents"].([]any)
			require.Len(t, parents, 1)
			assert.Equal(t, firstCommitID, parents[0].(string))
		}
	}
}

// TestHandler_CreateCommit_ParentCommitIdOutdated verifies a stale
// parentCommitId is rejected with the real AWS exception type
// (ParentCommitIdOutdatedException). errCodeLookup (handler.go) was missing
// an entry for this sentinel until this pass, so the error fell through to
// a generic ValidationException instead of its real, SDK-matching type.
func TestHandler_CreateCommit_ParentCommitIdOutdated(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "outdated-repo"})

	rec := doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "outdated-repo",
		"branchName":     "main",
		"commitMessage":  "initial",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "outdated-repo",
		"branchName":     "main",
		"commitMessage":  "second",
		"parentCommitId": "not-the-real-tip",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ParentCommitIdOutdatedException", resp["__type"])
}

// TestHandler_CreateCommit_NoChange verifies CreateCommit rejects a putFiles
// entry whose content matches what's already at that path with
// NoChangeException, and that no commit is created as a side effect of the
// rejected attempt. Corrected from SameFileContentException (gopherstack-8pe4):
// that's CreateCommit's sibling PutFile's own declared exception for the same
// identical-content check; CreateCommit's declared error set has no
// SameFileContentException at all (verified against codecommit@v1.36.4's
// deserializeOpErrorCreateCommit) — its closest declared equivalent is
// NoChangeException.
func TestHandler_CreateCommit_NoChange(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "same-content-commit-repo"})

	putFiles := []map[string]any{
		{"filePath": "main.go", "fileContent": "cGFja2FnZSBtYWlu"}, // "package main"
	}
	rec := doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "same-content-commit-repo",
		"branchName":     "main",
		"commitMessage":  "initial",
		"putFiles":       putFiles,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var firstResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &firstResp))
	firstCommitID, _ := firstResp["commitId"].(string)
	require.NotEmpty(t, firstCommitID)

	rec = doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "same-content-commit-repo",
		"branchName":     "main",
		"commitMessage":  "no-op",
		"parentCommitId": firstCommitID,
		"putFiles":       putFiles,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "NoChangeException", errResp["__type"])

	branchRec := doRequest(t, h, "GetBranch", map[string]any{
		"repositoryName": "same-content-commit-repo", "branchName": "main",
	})
	require.Equal(t, http.StatusOK, branchRec.Code)

	var branchResp map[string]any
	require.NoError(t, json.Unmarshal(branchRec.Body.Bytes(), &branchResp))
	branch, ok := branchResp["branch"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, firstCommitID, branch["commitId"], "rejected commit must not move the branch tip")
}

// TestGetCommit_ParentsNotNull verifies that GetCommit always returns "parents" as a JSON
// array, never null. AWS always returns "parents": [] even for root commits.
func TestGetCommit_ParentsNotNull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		commitIdx  int // 0 = first (root), 1 = second (has parent)
		wantParent bool
	}{
		{
			name:       "root_commit_parents_empty_array",
			commitIdx:  0,
			wantParent: false,
		},
		{
			name:       "second_commit_parents_has_entry",
			commitIdx:  1,
			wantParent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

			// Create first commit (root — no parents).
			r1 := doRequest(t, h, "CreateCommit", map[string]any{
				"repositoryName": "repo",
				"branchName":     "main",
				"commitMessage":  "first",
			})
			require.Equal(t, http.StatusOK, r1.Code)

			var firstResp map[string]any
			require.NoError(t, json.Unmarshal(r1.Body.Bytes(), &firstResp))
			firstID := firstResp["commitId"].(string)

			// Create second commit (has first as parent).
			r2 := doRequest(t, h, "CreateCommit", map[string]any{
				"repositoryName": "repo",
				"branchName":     "main",
				"commitMessage":  "second",
			})
			require.Equal(t, http.StatusOK, r2.Code)

			var secondResp map[string]any
			require.NoError(t, json.Unmarshal(r2.Body.Bytes(), &secondResp))
			secondID := secondResp["commitId"].(string)

			ids := []string{firstID, secondID}
			target := ids[tt.commitIdx]

			rec := doRequest(t, h, "GetCommit", map[string]any{
				"repositoryName": "repo",
				"commitId":       target,
			})
			assert.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			commit, ok := resp["commit"].(map[string]any)
			require.True(t, ok, "commit field must be present")

			// parents must be a JSON array, never null.
			parents, ok := commit["parents"].([]any)
			require.True(t, ok, "parents must be a JSON array, not null")

			if tt.wantParent {
				require.Len(t, parents, 1)
				assert.Equal(t, firstID, parents[0].(string))
			} else {
				assert.Empty(t, parents, "root commit must have empty parents array")
			}
		})
	}
}

// TestGetCommit_FullFieldSet verifies GetCommit response matches commitToMap shape:
// commitId, treeId, message, parents (array), author, committer, additionalData.
func TestGetCommit_FullFieldSet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

	r := doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "repo",
		"branchName":     "main",
		"authorName":     "Alice",
		"email":          "alice@example.com",
		"commitMessage":  "test commit",
	})
	require.Equal(t, http.StatusOK, r.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &createResp))
	commitID := createResp["commitId"].(string)

	rec := doRequest(t, h, "GetCommit", map[string]any{
		"repositoryName": "repo",
		"commitId":       commitID,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	commit, ok := resp["commit"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, commitID, commit["commitId"])
	assert.NotEmpty(t, commit["treeId"])
	assert.Equal(t, "test commit", commit["message"])

	// parents must be [] not null for root commit.
	parents, ok := commit["parents"].([]any)
	require.True(t, ok, "parents must be JSON array, not null")
	assert.Empty(t, parents)

	// author and committer sub-objects must be present.
	author, ok := commit["author"].(map[string]any)
	require.True(t, ok, "author sub-object must be present")
	assert.Equal(t, "Alice", author["name"])
	assert.Equal(t, "alice@example.com", author["email"])

	committer, ok := commit["committer"].(map[string]any)
	require.True(t, ok, "committer sub-object must be present")
	assert.NotNil(t, committer["name"])
}

// TestHandler_GetCommit_CommitIDNotFound verifies GetCommit rejects an
// unresolvable commitId with CommitIdDoesNotExistException, not
// CommitDoesNotExistException (gopherstack-8pe4): GetCommit's own declared
// error set (codecommit@v1.36.4's deserializeOpErrorGetCommit) has
// CommitIdDoesNotExistException, a distinct exception from
// CommitDoesNotExistException — the code the other commit-specifier-resolving
// ops (CreateBranch, the merge family) correctly use for a different scenario.
func TestHandler_GetCommit_CommitIDNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "get-commit-notfound-repo"})

	rec := doRequest(t, h, "GetCommit", map[string]any{
		"repositoryName": "get-commit-notfound-repo",
		"commitId":       "0000000000000000000000000000000000000",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "CommitIdDoesNotExistException", resp["__type"])
}

// TestHandler_BatchGetCommits_ErrorCodeIsCommitIdDoesNotExist verifies the
// per-entry BatchGetCommitsError.errorCode for an unresolvable commit ID is
// CommitIdDoesNotExistException, not CommitDoesNotExistException
// (gopherstack-pfyr). This is document data in a 200 response, so it isn't
// constrained by BatchGetCommits' declared error set (confirmed empty of
// both codes in codecommit@v1.36.4's deserializeOpErrorBatchGetCommits), but
// api-2.json types both GetCommitInput.commitId and BatchGetCommitsError.commitId
// as the ObjectId shape — the same shape GetCommit uses for the field that
// throws CommitIdDoesNotExistException — while CreateBranchInput.commitId and
// every other CommitDoesNotExistException-throwing op use the distinct CommitId
// shape reserved for specifier-resolution fields.
func TestHandler_BatchGetCommits_ErrorCodeIsCommitIdDoesNotExist(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "batch-commits-errorcode-repo"})

	rec := doRequest(t, h, "BatchGetCommits", map[string]any{
		"repositoryName": "batch-commits-errorcode-repo",
		"commitIds":      []string{"0000000000000000000000000000000000000"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	errs, ok := resp["errors"].([]any)
	require.True(t, ok)
	require.Len(t, errs, 1)

	entry, ok := errs[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "CommitIdDoesNotExistException", entry["errorCode"])
}

func TestHandler_GetDifferences(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "diffs-repo"})

	rec := doRequest(t, h, "GetDifferences", map[string]any{
		"repositoryName":       "diffs-repo",
		"afterCommitSpecifier": "abc",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp["differences"])
}

func TestHandler_GetDifferences_RepoNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetDifferences", map[string]any{
		"repositoryName":       "no-repo",
		"afterCommitSpecifier": "abc",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGetDifferences_BlobObjectShape verifies GetDifferences returns afterBlob/beforeBlob
// as objects with blobId, path, and mode fields (not plain strings), matching real AWS behavior.
func TestGetDifferences_BlobObjectShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filePaths []string
		wantCount int
	}{
		{
			name:      "single_file",
			filePaths: []string{"README.md"},
			wantCount: 1,
		},
		{
			name:      "multiple_files",
			filePaths: []string{"src/main.go", "go.mod", "README.md"},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "diffs-repo"})

			putFiles := make([]map[string]any, 0, len(tt.filePaths))
			for _, fp := range tt.filePaths {
				putFiles = append(putFiles, map[string]any{
					"filePath":    fp,
					"fileContent": "aGVsbG8=",
				})
			}

			commitRec := doRequest(t, h, "CreateCommit", map[string]any{
				"repositoryName": "diffs-repo",
				"branchName":     "main",
				"putFiles":       putFiles,
			})
			require.Equal(t, http.StatusOK, commitRec.Code)

			var commitOut map[string]any
			require.NoError(t, json.Unmarshal(commitRec.Body.Bytes(), &commitOut))
			commitID := commitOut["commitId"].(string)

			rec := doRequest(t, h, "GetDifferences", map[string]any{
				"repositoryName":       "diffs-repo",
				"afterCommitSpecifier": commitID,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				Differences []map[string]any `json:"differences"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Len(t, resp.Differences, tt.wantCount)

			// Each difference must have afterBlob as an object, not a string.
			for i, diff := range resp.Differences {
				afterBlob, ok := diff["afterBlob"].(map[string]any)
				require.True(t, ok, "differences[%d].afterBlob must be an object", i)

				assert.NotEmpty(t, afterBlob["blobId"], "afterBlob.blobId must be non-empty")
				assert.NotEmpty(t, afterBlob["path"], "afterBlob.path must be non-empty")
				assert.NotEmpty(t, afterBlob["mode"], "afterBlob.mode must be non-empty")
				assert.Equal(t, "A", diff["changeType"], "changeType must be A for new files")
			}
		})
	}
}

// TestGetDifferences_FilePathInBlob verifies that afterBlob.path matches the committed
// file path, matching real AWS behavior.
func TestGetDifferences_FilePathInBlob(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "path-repo"})

	commitRec := doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "path-repo",
		"branchName":     "main",
		"putFiles": []map[string]any{
			{"filePath": "src/handler.go", "fileContent": "cGFja2FnZSBtYWlu"},
		},
	})
	require.Equal(t, http.StatusOK, commitRec.Code)

	var commitOut map[string]any
	require.NoError(t, json.Unmarshal(commitRec.Body.Bytes(), &commitOut))

	rec := doRequest(t, h, "GetDifferences", map[string]any{
		"repositoryName":       "path-repo",
		"afterCommitSpecifier": commitOut["commitId"].(string),
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Differences []map[string]any `json:"differences"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Differences, 1)

	afterBlob := resp.Differences[0]["afterBlob"].(map[string]any)
	assert.Equal(t, "src/handler.go", afterBlob["path"],
		"afterBlob.path must match the committed file path")
}

// TestGetDifferences_EmptyRepoReturnsEmptyArray verifies GetDifferences on
// an empty repo returns an empty array (not null), matching real AWS behavior.
func TestGetDifferences_EmptyRepoReturnsEmptyArray(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "empty-repo"})

	rec := doRequest(t, h, "GetDifferences", map[string]any{
		"repositoryName":       "empty-repo",
		"afterCommitSpecifier": "abc123",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	diffs, ok := resp["differences"].([]any)
	require.True(t, ok, "differences must be a JSON array, not null")
	assert.Empty(t, diffs)
}

// TestGetDifferences_Pagination verifies GetDifferences honors MaxResults and
// NextToken and returns every file exactly once across pages. It also locks
// the request/response field names: unlike every other paginated op in this
// service, GetDifferences capitalizes MaxResults/NextToken on the wire —
// verified against aws-sdk-go-v2/service/codecommit's generated
// (de)serializers (awsAwsjson11_serializeOpDocumentGetDifferencesInput /
// awsAwsjson11_deserializeOpDocumentGetDifferencesOutput).
func TestGetDifferences_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "page-repo"})

	putFiles := make([]map[string]any, 0, 5)
	for i := range 5 {
		putFiles = append(putFiles, map[string]any{
			"filePath":    fmt.Sprintf("file%d.txt", i),
			"fileContent": "aGVsbG8=",
		})
	}
	commitRec := doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "page-repo",
		"branchName":     "main",
		"putFiles":       putFiles,
	})
	require.Equal(t, http.StatusOK, commitRec.Code)

	var commitOut map[string]any
	require.NoError(t, json.Unmarshal(commitRec.Body.Bytes(), &commitOut))
	commitID := commitOut["commitId"].(string)

	seenPaths := map[string]bool{}
	nextToken := ""

	for range 10 {
		req := map[string]any{
			"repositoryName":       "page-repo",
			"afterCommitSpecifier": commitID,
			"MaxResults":           2,
		}
		if nextToken != "" {
			req["NextToken"] = nextToken
		}

		rec := doRequest(t, h, "GetDifferences", req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		diffs, ok := resp["differences"].([]any)
		require.True(t, ok)
		assert.LessOrEqual(t, len(diffs), 2, "each page must respect MaxResults")

		for _, raw := range diffs {
			d, dOK := raw.(map[string]any)
			require.True(t, dOK)
			afterBlob, blobOK := d["afterBlob"].(map[string]any)
			require.True(t, blobOK)
			path, _ := afterBlob["path"].(string)
			require.NotEmpty(t, path)
			assert.False(t, seenPaths[path], "path %s must not repeat across pages", path)
			seenPaths[path] = true
		}

		next, hasNext := resp["NextToken"].(string)
		if !hasNext || next == "" {
			break
		}
		nextToken = next
	}

	assert.Len(t, seenPaths, 5, "pagination must eventually surface every file exactly once")
}
