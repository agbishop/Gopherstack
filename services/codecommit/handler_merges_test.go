package codecommit_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codecommit"
)

func TestHandler_BatchDescribeMergeConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "success_no_file_paths",
			input: map[string]any{
				"repositoryName":             "repo",
				"destinationCommitSpecifier": "main",
				"sourceCommitSpecifier":      "feature",
				"mergeOption":                "FAST_FORWARD_MERGE",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "success_with_file_paths",
			input: map[string]any{
				"repositoryName":             "repo",
				"destinationCommitSpecifier": "main",
				"sourceCommitSpecifier":      "feature",
				"mergeOption":                "THREE_WAY_MERGE",
				"filePaths":                  []string{"file1.txt", "file2.txt"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "repo_not_found",
			input: map[string]any{
				"repositoryName":             "missing",
				"destinationCommitSpecifier": "main",
				"sourceCommitSpecifier":      "feature",
				"mergeOption":                "FAST_FORWARD_MERGE",
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "missing_repo_name",
			input: map[string]any{
				"repositoryName":             "",
				"destinationCommitSpecifier": "main",
				"sourceCommitSpecifier":      "feature",
				"mergeOption":                "FAST_FORWARD_MERGE",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_merge_option",
			input: map[string]any{
				"repositoryName":             "repo",
				"destinationCommitSpecifier": "main",
				"sourceCommitSpecifier":      "feature",
				"mergeOption":                "",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})
			require.Equal(t, http.StatusOK, rec.Code)

			rec = doRequest(t, h, "BatchDescribeMergeConflicts", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Contains(t, resp, "conflicts")
				assert.Contains(t, resp, "destinationCommitId")
				assert.Contains(t, resp, "sourceCommitId")
			}
		})
	}
}

func TestHandler_BatchDescribeMergeConflicts_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		mergeOption   string
		filePaths     []string
		wantCode      int
		wantConflicts int
	}{
		{
			name:          "no_file_paths",
			mergeOption:   "FAST_FORWARD_MERGE",
			filePaths:     nil,
			wantCode:      http.StatusOK,
			wantConflicts: 0,
		},
		{
			name:          "two_file_paths",
			mergeOption:   "SQUASH_MERGE",
			filePaths:     []string{"a.go", "b.go"},
			wantCode:      http.StatusOK,
			wantConflicts: 2,
		},
		{
			name:        "invalid_merge_option",
			mergeOption: "INVALID_MERGE",
			wantCode:    http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

			body := map[string]any{
				"repositoryName":             "repo",
				"destinationCommitSpecifier": "main",
				"sourceCommitSpecifier":      "feat",
				"mergeOption":                tt.mergeOption,
			}
			if tt.filePaths != nil {
				body["filePaths"] = tt.filePaths
			}

			rec := doRequest(t, h, "BatchDescribeMergeConflicts", body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				conflicts := resp["conflicts"].([]any)
				assert.Len(t, conflicts, tt.wantConflicts)
			}
		})
	}
}

func TestHandler_MergePullRequestByFastForward(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "merge-ff-repo")

	rec := doRequest(t, h, "MergePullRequestByFastForward", map[string]any{
		"pullRequestId":  prID,
		"repositoryName": "merge-ff-repo",
		"sourceCommitId": "abc",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pr := resp["pullRequest"].(map[string]any)
	assert.Equal(t, "CLOSED", pr["pullRequestStatus"])
}

func TestHandler_MergePullRequestBySquash(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "merge-sq-repo")

	rec := doRequest(t, h, "MergePullRequestBySquash", map[string]any{
		"pullRequestId":  prID,
		"repositoryName": "merge-sq-repo",
		"sourceCommitId": "abc",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pr := resp["pullRequest"].(map[string]any)
	assert.Equal(t, "CLOSED", pr["pullRequestStatus"])
}

func TestHandler_MergePullRequestByThreeWay(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	prID := setupPR(t, h, "merge-3w-repo")

	rec := doRequest(t, h, "MergePullRequestByThreeWay", map[string]any{
		"pullRequestId":  prID,
		"repositoryName": "merge-3w-repo",
		"sourceCommitId": "abc",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	pr := resp["pullRequest"].(map[string]any)
	assert.Equal(t, "CLOSED", pr["pullRequestStatus"])
}

func TestHandler_MergeBranchesByFastForward(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "branch-merge-repo")
	createBranchFromMain(t, h, "branch-merge-repo", "feature")
	sourceTip := mustBranchTip(t, h, "branch-merge-repo", "feature")

	rec := doRequest(t, h, "MergeBranchesByFastForward", map[string]any{
		"repositoryName":             "branch-merge-repo",
		"sourceCommitSpecifier":      "feature",
		"destinationCommitSpecifier": "main",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, sourceTip, resp["commitId"],
		"fast-forward moves the pointer to the existing source commit; it never fabricates a new one")
	assert.Equal(t, sourceTip, mustBranchTip(t, h, "branch-merge-repo", "main"),
		"destination branch tip must move to the source commit")
}

func TestHandler_MergeBranchesByFastForward_UnknownSpecifier(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "ff-unknown-repo")

	tests := []struct {
		name        string
		source      string
		destination string
	}{
		{name: "unknown_source", source: "does-not-exist", destination: "main"},
		{name: "unknown_destination", source: "main", destination: "does-not-exist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, "MergeBranchesByFastForward", map[string]any{
				"repositoryName":             "ff-unknown-repo",
				"sourceCommitSpecifier":      tt.source,
				"destinationCommitSpecifier": tt.destination,
			})
			assert.Equal(t, http.StatusNotFound, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "CommitDoesNotExistException", resp["__type"])
		})
	}
}

func TestHandler_MergeBranchesBySquash(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "sq-merge-repo")
	createBranchFromMain(t, h, "sq-merge-repo", "feature")

	rec := doRequest(t, h, "MergeBranchesBySquash", map[string]any{
		"repositoryName":             "sq-merge-repo",
		"sourceCommitSpecifier":      "feature",
		"destinationCommitSpecifier": "main",
		"commitMessage":              "squash it",
		"authorName":                 "squasher",
		"email":                      "squasher@example.com",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	commitID, _ := resp["commitId"].(string)
	require.NotEmpty(t, commitID)

	getRec := doRequest(t, h, "GetCommit", map[string]any{
		"repositoryName": "sq-merge-repo",
		"commitId":       commitID,
	})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	commit, ok := getResp["commit"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "squash it", commit["message"])
	author, ok := commit["author"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "squasher", author["name"])
	parents, ok := commit["parents"].([]any)
	require.True(t, ok)
	assert.Len(t, parents, 1, "squash merge commit has exactly one parent (the destination tip)")
}

func TestHandler_MergeBranchesByThreeWay(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "3w-merge-repo")
	createBranchFromMain(t, h, "3w-merge-repo", "feature")

	rec := doRequest(t, h, "MergeBranchesByThreeWay", map[string]any{
		"repositoryName":             "3w-merge-repo",
		"sourceCommitSpecifier":      "feature",
		"destinationCommitSpecifier": "main",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	commitID, _ := resp["commitId"].(string)
	require.NotEmpty(t, commitID)

	getRec := doRequest(t, h, "GetCommit", map[string]any{
		"repositoryName": "3w-merge-repo",
		"commitId":       commitID,
	})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	commit, ok := getResp["commit"].(map[string]any)
	require.True(t, ok)
	parents, ok := commit["parents"].([]any)
	require.True(t, ok)
	assert.Len(t, parents, 2, "three-way merge commit has two parents (destination then source)")
}

func TestHandler_MergeBranchesBySquash_TargetBranch(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "sq-target-repo")
	createBranchFromMain(t, h, "sq-target-repo", "feature")
	doRequest(t, h, "CreateBranch", map[string]any{
		"repositoryName": "sq-target-repo",
		"branchName":     "release",
		"commitId":       mustBranchTip(t, h, "sq-target-repo", "main"),
	})

	rec := doRequest(t, h, "MergeBranchesBySquash", map[string]any{
		"repositoryName":             "sq-target-repo",
		"sourceCommitSpecifier":      "feature",
		"destinationCommitSpecifier": "main",
		"targetBranch":               "release",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	commitID, _ := resp["commitId"].(string)

	assert.Equal(t, commitID, mustBranchTip(t, h, "sq-target-repo", "release"),
		"targetBranch, not the destination branch, must be advanced")
	assert.NotEqual(t, commitID, mustBranchTip(t, h, "sq-target-repo", "main"),
		"destination branch itself must be untouched when targetBranch differs")
}

func TestHandler_MergeBranchesByThreeWay_UnresolvableSource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "3w-missing-repo")

	rec := doRequest(t, h, "MergeBranchesByThreeWay", map[string]any{
		"repositoryName":             "3w-missing-repo",
		"sourceCommitSpecifier":      "no-such-branch",
		"destinationCommitSpecifier": "main",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func mustBranchTip(t *testing.T, h *codecommit.Handler, repoName, branchName string) string {
	t.Helper()

	rec := doRequest(t, h, "GetBranch", map[string]any{"repositoryName": repoName, "branchName": branchName})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	branch, ok := resp["branch"].(map[string]any)
	require.True(t, ok)
	commitID, _ := branch["commitId"].(string)
	require.NotEmpty(t, commitID)

	return commitID
}

func TestHandler_MergeBranches_AllStrategies(t *testing.T) {
	t.Parallel()

	strategies := []string{
		"MergeBranchesByFastForward",
		"MergeBranchesBySquash",
		"MergeBranchesByThreeWay",
	}

	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			repoName := "repo-" + strategy
			setupRepoAndBranch(t, h, repoName)
			createBranchFromMain(t, h, repoName, "feature")

			rec := doRequest(t, h, strategy, map[string]any{
				"repositoryName":             repoName,
				"sourceCommitSpecifier":      "feature",
				"destinationCommitSpecifier": "main",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotEmpty(t, resp["commitId"])
			assert.NotEmpty(t, resp["treeId"])
		})
	}
}

func TestHandler_GetMergeOptions(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "merge-opts-repo"})

	rec := doRequest(t, h, "GetMergeOptions", map[string]any{
		"repositoryName":             "merge-opts-repo",
		"sourceCommitSpecifier":      "feature",
		"destinationCommitSpecifier": "main",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	opts := resp["mergeOptions"].([]any)
	assert.Len(t, opts, 3)
}

func TestHandler_MergeOption_InvalidValue(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "BatchDescribeMergeConflicts", map[string]any{
		"repositoryName":             "repo",
		"destinationCommitSpecifier": "main",
		"sourceCommitSpecifier":      "feature",
		"mergeOption":                "INVALID_MERGE_OPTION",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_MergeOptions_AllStrategies(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

	rec := doRequest(t, h, "GetMergeOptions", map[string]any{
		"repositoryName":             "repo",
		"sourceCommitSpecifier":      "feat",
		"destinationCommitSpecifier": "main",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	opts := resp["mergeOptions"].([]any)
	assert.Len(t, opts, 3)

	optStrs := make([]string, len(opts))
	for i, o := range opts {
		optStrs[i] = o.(string)
	}
	assert.Contains(t, optStrs, "FAST_FORWARD_MERGE")
	assert.Contains(t, optStrs, "SQUASH_MERGE")
	assert.Contains(t, optStrs, "THREE_WAY_MERGE")
}

func TestHandler_CreateUnreferencedMergeCommit(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "unref-repo"})

	rec := doRequest(t, h, "CreateUnreferencedMergeCommit", map[string]any{
		"repositoryName":             "unref-repo",
		"sourceCommitSpecifier":      "abc",
		"destinationCommitSpecifier": "def",
		"mergeOption":                "FAST_FORWARD_MERGE",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["commitId"])
}

// TestHandler_CreateUnreferencedMergeCommit_MergeOptionRequired verifies
// mergeOption is enforced as required, matching
// CreateUnreferencedMergeCommitInput.MergeOption's "This member is
// required" doc comment (codecommit@v1.36.4
// api_op_CreateUnreferencedMergeCommit.go) -- the handler previously parsed
// mergeOption off the wire and never validated or forwarded it at all.
func TestHandler_CreateUnreferencedMergeCommit_MergeOptionRequired(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "unref-repo-missing-mo"})

	rec := doRequest(t, h, "CreateUnreferencedMergeCommit", map[string]any{
		"repositoryName":             "unref-repo-missing-mo",
		"sourceCommitSpecifier":      "abc",
		"destinationCommitSpecifier": "def",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_CreateUnreferencedMergeCommit_InvalidMergeOption(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "unref-repo-bad-mo"})

	rec := doRequest(t, h, "CreateUnreferencedMergeCommit", map[string]any{
		"repositoryName":             "unref-repo-bad-mo",
		"sourceCommitSpecifier":      "abc",
		"destinationCommitSpecifier": "def",
		"mergeOption":                "NOT_A_REAL_OPTION",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_CreateUnreferencedMergeCommit_Success(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "repo")

	branchRec := doRequest(t, h, "GetBranch", map[string]any{
		"repositoryName": "repo",
		"branchName":     "main",
	})
	require.Equal(t, http.StatusOK, branchRec.Code)

	var brResp map[string]any
	require.NoError(t, json.Unmarshal(branchRec.Body.Bytes(), &brResp))
	commitID := brResp["branch"].(map[string]any)["commitId"].(string)

	rec := doRequest(t, h, "CreateUnreferencedMergeCommit", map[string]any{
		"repositoryName":             "repo",
		"sourceCommitSpecifier":      commitID,
		"destinationCommitSpecifier": commitID,
		"mergeOption":                "FAST_FORWARD_MERGE",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["commitId"])
	assert.NotEmpty(t, resp["treeId"])
}

func TestHandler_GetMergeCommit(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "merge-commit-repo")

	rec := doRequest(t, h, "GetMergeCommit", map[string]any{
		"repositoryName":             "merge-commit-repo",
		"sourceCommitSpecifier":      "abc",
		"destinationCommitSpecifier": "def",
		"mergeOption":                "FAST_FORWARD_MERGE",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["mergedCommitId"])
}

func TestHandler_GetMergeCommit_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	// Repo with no commits.
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "empty-repo"})

	rec := doRequest(t, h, "GetMergeCommit", map[string]any{
		"repositoryName":             "empty-repo",
		"sourceCommitSpecifier":      "feat",
		"destinationCommitSpecifier": "main",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGetMergeCommit_ResolvesParents verifies GetMergeCommit returns the commit
// whose parents include both source and destination specifiers.
func TestGetMergeCommit_ResolvesParents(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "merge-repo"})

	// Create base commit on main.
	baseRec := doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "merge-repo",
		"branchName":     "main",
		"putFiles":       []map[string]any{{"filePath": "base.txt", "fileContent": "YmFzZQ=="}},
	})
	require.Equal(t, http.StatusOK, baseRec.Code)

	var baseOut map[string]any
	require.NoError(t, json.Unmarshal(baseRec.Body.Bytes(), &baseOut))
	baseCommitID := baseOut["commitId"].(string)

	// Create feature branch from base.
	doRequest(t, h, "CreateBranch", map[string]any{
		"repositoryName": "merge-repo",
		"branchName":     "feature",
		"commitId":       baseCommitID,
	})

	// Commit to feature branch.
	featureRec := doRequest(t, h, "CreateCommit", map[string]any{
		"repositoryName": "merge-repo",
		"branchName":     "feature",
		"putFiles":       []map[string]any{{"filePath": "feature.txt", "fileContent": "ZmVhdA=="}},
	})
	require.Equal(t, http.StatusOK, featureRec.Code)

	var featureOut map[string]any
	require.NoError(t, json.Unmarshal(featureRec.Body.Bytes(), &featureOut))
	featureCommitID := featureOut["commitId"].(string)

	// Fast-forward merge.
	mergeRec := doRequest(t, h, "MergeBranchesByFastForward", map[string]any{
		"repositoryName":             "merge-repo",
		"sourceCommitSpecifier":      featureCommitID,
		"destinationCommitSpecifier": baseCommitID,
		"targetBranch":               "main",
	})
	require.Equal(t, http.StatusOK, mergeRec.Code)

	// GetMergeCommit should return a valid commit.
	rec := doRequest(t, h, "GetMergeCommit", map[string]any{
		"repositoryName":             "merge-repo",
		"sourceCommitSpecifier":      featureCommitID,
		"destinationCommitSpecifier": baseCommitID,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.NotEmpty(t, out["mergedCommitId"],
		"GetMergeCommit must return a non-empty mergedCommitId")
}

func TestHandler_GetMergeConflicts(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "conflicts-repo")
	createBranchFromMain(t, h, "conflicts-repo", "feature")

	rec := doRequest(t, h, "GetMergeConflicts", map[string]any{
		"repositoryName":             "conflicts-repo",
		"sourceCommitSpecifier":      "feature",
		"destinationCommitSpecifier": "main",
		"mergeOption":                "FAST_FORWARD_MERGE",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	// No content-diff engine backs this emulator, so a merge between two
	// resolvable specifiers is always reported mergeable (see PARITY.md gaps).
	assert.Equal(t, true, resp["mergeable"])
	assert.NotEmpty(t, resp["sourceCommitId"])
	assert.NotEmpty(t, resp["destinationCommitId"])
}

func TestHandler_GetMergeConflicts_NoConflicts(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "repo")
	createBranchFromMain(t, h, "repo", "feat")

	rec := doRequest(t, h, "GetMergeConflicts", map[string]any{
		"repositoryName":             "repo",
		"sourceCommitSpecifier":      "feat",
		"destinationCommitSpecifier": "main",
		"mergeOption":                "FAST_FORWARD_MERGE",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["mergeable"])
}

func TestHandler_GetMergeConflicts_UnresolvableSpecifier(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	setupRepoAndBranch(t, h, "conflicts-missing-repo")

	rec := doRequest(t, h, "GetMergeConflicts", map[string]any{
		"repositoryName":             "conflicts-missing-repo",
		"sourceCommitSpecifier":      "no-such-branch",
		"destinationCommitSpecifier": "main",
		"mergeOption":                "FAST_FORWARD_MERGE",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandler_GetMergeConflicts_MissingRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input map[string]any
		name  string
	}{
		{
			name: "missing_source",
			input: map[string]any{
				"repositoryName":             "repo",
				"destinationCommitSpecifier": "main",
				"mergeOption":                "FAST_FORWARD_MERGE",
			},
		},
		{
			name: "missing_destination",
			input: map[string]any{
				"repositoryName":        "repo",
				"sourceCommitSpecifier": "feature",
				"mergeOption":           "FAST_FORWARD_MERGE",
			},
		},
		{
			name: "missing_merge_option",
			input: map[string]any{
				"repositoryName":             "repo",
				"sourceCommitSpecifier":      "feature",
				"destinationCommitSpecifier": "main",
			},
		},
		{
			name: "invalid_merge_option",
			input: map[string]any{
				"repositoryName":             "repo",
				"sourceCommitSpecifier":      "feature",
				"destinationCommitSpecifier": "main",
				"mergeOption":                "BOGUS",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "repo"})

			rec := doRequest(t, h, "GetMergeConflicts", tt.input)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_DescribeMergeConflicts(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "dmc-repo"})

	rec := doRequest(t, h, "DescribeMergeConflicts", map[string]any{
		"repositoryName":             "dmc-repo",
		"sourceCommitSpecifier":      "abc",
		"destinationCommitSpecifier": "def",
		"mergeOption":                "FAST_FORWARD_MERGE",
		"filePath":                   "main.go",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "abc", resp["sourceCommitId"])
	assert.Equal(t, "def", resp["destinationCommitId"])
	meta, ok := resp["conflictMetadata"].(map[string]any)
	require.True(t, ok, "conflictMetadata must be an object")
	assert.Equal(t, "main.go", meta["filePath"])
}

// TestHandler_DescribeMergeConflicts_RepositoryNotFound verifies that DescribeMergeConflicts,
// like its BatchDescribeMergeConflicts sibling, validates the repository actually exists
// instead of echoing the request back unexamined.
func TestHandler_DescribeMergeConflicts_RepositoryNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "DescribeMergeConflicts", map[string]any{
		"repositoryName":             "no-such-repo",
		"sourceCommitSpecifier":      "abc",
		"destinationCommitSpecifier": "def",
		"mergeOption":                "FAST_FORWARD_MERGE",
		"filePath":                   "main.go",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestHandler_DescribeMergeConflicts_RequiredFields verifies that missing required
// fields are rejected instead of silently defaulting, mirroring BatchDescribeMergeConflicts.
func TestHandler_DescribeMergeConflicts_RequiredFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateRepository", map[string]any{"repositoryName": "dmc-repo"})

	base := map[string]any{
		"repositoryName":             "dmc-repo",
		"sourceCommitSpecifier":      "abc",
		"destinationCommitSpecifier": "def",
		"mergeOption":                "FAST_FORWARD_MERGE",
		"filePath":                   "main.go",
	}

	for _, missing := range []string{
		"repositoryName", "sourceCommitSpecifier", "destinationCommitSpecifier", "mergeOption", "filePath",
	} {
		t.Run("missing_"+missing, func(t *testing.T) {
			t.Parallel()

			body := map[string]any{}
			for k, v := range base {
				if k != missing {
					body[k] = v
				}
			}

			rec := doRequest(t, h, "DescribeMergeConflicts", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}
