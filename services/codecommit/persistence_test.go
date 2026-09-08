package codecommit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/codecommit"
)

// TestInMemoryBackend_RestoreInvalidData verifies that malformed JSON is
// reported as an error rather than silently discarded or partially applied.
func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := codecommit.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend (including a version-less
// snapshot, which decodes with Version == 0) is discarded cleanly rather
// than partially decoded: the backend resets to empty state and Restore
// returns no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := codecommit.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	_, err := b.CreateRepository("seed-repo", "seed", "", nil)
	require.NoError(t, err)

	// A syntactically valid but version-mismatched snapshot.
	err = b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Empty(t, b.ListRepositories())

	_, err = b.GetRepository("seed-repo")
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero verifies that a
// snapshot with no version field at all decodes with Version == 0, which
// mismatches codecommitSnapshotVersion and is discarded the same way any
// other incompatible version is -- not partially applied. CodeCommit had no
// persistence at all before Phase 3.3, so this also covers "no snapshot
// format predates this one" -- any pre-existing on-disk data (there is none)
// would hit this same path.
func TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero(t *testing.T) {
	t.Parallel()

	b := codecommit.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	_, err := b.CreateRepository("seed-repo", "seed", "", nil)
	require.NoError(t, err)

	// A shape entirely lacking "version"/"tables" keys.
	err = b.Restore(t.Context(), []byte(`{"repositories":{"seed-repo":{"repositoryName":"seed-repo"}}}`))
	require.NoError(t, err)

	assert.Empty(t, b.ListRepositories())
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a Snapshot->Restore
// round trip across every store.Table-backed resource family the Phase 3.3
// conversion touched (repositories, approvalRuleTemplates, the composite-key
// branches/commits/files tables and their byRepo indexes, pullRequests, and
// the composite-key prApprovalRules table with its byPR index) plus every
// plain map left un-converted (repoTemplateAssoc, prApprovals, prOverrides,
// prOverriders, prEvents, commentReactions, fileHistory, triggers) and the
// ephemeral repositoriesByARN reverse index, which must be correctly rebuilt
// from the restored repositories table rather than persisted directly.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := codecommit.NewInMemoryBackend("111122223333", "us-west-2")

	repo, err := original.CreateRepository("repo-1", "a repo", "", map[string]string{"env": "test"})
	require.NoError(t, err)

	tmpl, err := original.CreateApprovalRuleTemplate("tmpl-1", "a template", `{"Version":"2018-11-08"}`)
	require.NoError(t, err)
	require.NoError(t, original.AssociateApprovalRuleTemplateWithRepository(
		tmpl.ApprovalRuleTemplateName, repo.RepositoryName,
	))

	commit1, _, _, err := original.CreateCommit(
		repo.RepositoryName, "main", "author", "author@example.com", "init", "",
		[]codecommit.PutFileEntry{{FilePath: "README.md", FileMode: "NORMAL", FileContent: []byte("hello")}}, nil,
	)
	require.NoError(t, err)

	require.NoError(t, original.PutRepositoryTriggers(repo.RepositoryName, []codecommit.RepositoryTrigger{
		{Name: "trigger-1", DestinationARN: "arn:aws:sns:us-west-2:111122223333:topic", Events: []string{"all"}},
	}))

	commit2, _, err := original.PutFile(
		repo.RepositoryName, "main", "docs/guide.md", []byte("guide"), codecommit.PutFileMetadata{},
	)
	require.NoError(t, err)
	_ = commit2

	pr, err := original.CreatePullRequest("a PR", "desc", "", []codecommit.PullRequestTarget{
		{RepositoryName: repo.RepositoryName, SourceReference: "refs/heads/main"},
	})
	require.NoError(t, err)

	rule, err := original.CreatePullRequestApprovalRule(pr.PullRequestID, "rule-1", `{"Version":"2018-11-08"}`)
	require.NoError(t, err)

	require.NoError(t, original.UpdatePullRequestApprovalState(
		pr.PullRequestID, "arn:aws:iam::111122223333:user/alice", "APPROVE",
	))
	require.NoError(t, original.OverridePullRequestApprovalRules(
		pr.PullRequestID, "OVERRIDE", "arn:aws:iam::111122223333:user/bob",
	))

	prComment, err := original.PostCommentForPullRequest(pr.PullRequestID, repo.RepositoryName, "pr comment")
	require.NoError(t, err)
	require.NoError(t, original.PutCommentReaction(prComment.CommentID, "THUMBSUP"))

	commitComment, err := original.PostCommentForComparedCommit(
		repo.RepositoryName, "", commit1.CommitID, "commit comment",
	)
	require.NoError(t, err)

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := codecommit.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	require.NoError(t, fresh.Restore(t.Context(), snap))

	// Scalars: accountID/region surface indirectly through a freshly created
	// resource (there is no direct accessor), nextPRCounter through the next
	// assigned pull request ID continuing from the restored counter.
	newRepo, err := fresh.CreateRepository("repo-2", "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "111122223333", newRepo.AccountID)
	assert.Contains(t, newRepo.ARN, "us-west-2")

	newPR, err := fresh.CreatePullRequest("second PR", "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "2", newPR.PullRequestID)

	// repositories table + Tags.
	gotRepo, err := fresh.GetRepository(repo.RepositoryName)
	require.NoError(t, err)
	assert.Equal(t, "a repo", gotRepo.Description)
	tagVals, err := fresh.ListTagsForResource(repo.ARN)
	require.NoError(t, err)
	assert.Equal(t, "test", tagVals["env"])

	// repositoriesByARN ephemeral reverse index, rebuilt from repositories.
	require.NoError(t, fresh.TagResource(repo.ARN, map[string]string{"team": "platform"}))
	tagVals, err = fresh.ListTagsForResource(repo.ARN)
	require.NoError(t, err)
	assert.Equal(t, "platform", tagVals["team"])

	// approvalRuleTemplates table + repoTemplateAssoc plain map.
	gotTmpl, err := fresh.GetApprovalRuleTemplate(tmpl.ApprovalRuleTemplateName)
	require.NoError(t, err)
	assert.Equal(t, "a template", gotTmpl.ApprovalRuleTemplateDescription)
	assocNames, err := fresh.ListAssociatedApprovalRuleTemplatesForRepository(repo.RepositoryName)
	require.NoError(t, err)
	assert.Equal(t, []string{tmpl.ApprovalRuleTemplateName}, assocNames)

	// branches table (composite key + byRepo index).
	branchNames, err := fresh.ListBranches(repo.RepositoryName)
	require.NoError(t, err)
	assert.Equal(t, []string{"main"}, branchNames)
	gotBranch, err := fresh.GetBranch(repo.RepositoryName, "main")
	require.NoError(t, err)
	assert.Equal(t, commit2.CommitID, gotBranch.CommitID)

	// commits table (composite key + byRepo index) + fileHistory plain map.
	gotCommit, err := fresh.GetCommit(repo.RepositoryName, commit1.CommitID)
	require.NoError(t, err)
	assert.Equal(t, "init", gotCommit.Message)
	history, err := fresh.ListFileCommitHistory(repo.RepositoryName, "README.md", "", 0)
	require.NoError(t, err)
	require.Len(t, history.Data, 1)
	assert.Equal(t, commit1.CommitID, history.Data[0].Commit.CommitID)

	// files table (composite key + byRepo index; RepoName hidden field).
	gotFile, err := fresh.GetFile(repo.RepositoryName, "", "docs/guide.md")
	require.NoError(t, err)
	assert.Equal(t, []byte("guide"), gotFile.FileContent)
	folder, err := fresh.GetFolder(repo.RepositoryName, "", "")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"README.md", "docs/guide.md"}, folder)
	blob, err := fresh.GetBlob(repo.RepositoryName, gotFile.BlobID)
	require.NoError(t, err)
	assert.Equal(t, []byte("guide"), blob)

	// triggers plain map.
	triggers, err := fresh.GetRepositoryTriggers(repo.RepositoryName)
	require.NoError(t, err)
	require.Len(t, triggers, 1)
	assert.Equal(t, "trigger-1", triggers[0].Name)

	// pullRequests table.
	gotPR, err := fresh.GetPullRequest(pr.PullRequestID)
	require.NoError(t, err)
	assert.Equal(t, "a PR", gotPR.Title)

	// prApprovalRules table (composite key + byPR index; PRID hidden field).
	evals, err := fresh.EvaluatePullRequestApprovalRules(pr.PullRequestID)
	require.NoError(t, err)
	require.Len(t, evals, 1)
	assert.Equal(t, rule.RuleName, evals[0].RuleName)

	// prApprovals plain map.
	approvals, err := fresh.GetPullRequestApprovalStates(pr.PullRequestID)
	require.NoError(t, err)
	require.Len(t, approvals, 1)
	assert.Equal(t, "APPROVE", approvals[0].ApprovalState)

	// prOverrides/prOverriders plain maps.
	overridden, overrider, err := fresh.GetPullRequestOverrideState(pr.PullRequestID)
	require.NoError(t, err)
	assert.True(t, overridden)
	assert.Equal(t, "arn:aws:iam::111122223333:user/bob", overrider)

	// prEvents plain map.
	events, err := fresh.DescribePullRequestEvents(pr.PullRequestID, "", "")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "PULL_REQUEST_APPROVAL_RULE_OVERRIDDEN", events[0].PullRequestEventType)

	// comments table (PRid/RepoName/AfterCommitID hidden fields) + commentReactions plain map.
	prComments, err := fresh.GetCommentsForPullRequest(pr.PullRequestID)
	require.NoError(t, err)
	require.Len(t, prComments, 1)
	assert.Equal(t, "pr comment", prComments[0].Content)
	reactions, err := fresh.GetCommentReactions(prComment.CommentID)
	require.NoError(t, err)
	require.Len(t, reactions, 1)
	assert.Equal(t, "THUMBSUP", reactions[0].Emoji)

	commitComments, err := fresh.GetCommentsForComparedCommit(repo.RepositoryName, commit1.CommitID)
	require.NoError(t, err)
	require.Len(t, commitComments, 1)
	assert.Equal(t, commitComment.CommentID, commitComments[0].CommentID)
}

// TestHandler_SnapshotRestoreDelegate verifies the Handler-level Snapshot and
// Restore -- which the persistence layer actually calls, per
// setupPersistence in cli.go -- correctly delegate to the backend.
func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	h := codecommit.NewHandler(codecommit.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion))

	_, err := h.Backend.CreateRepository("delegate-repo", "", "", nil)
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := codecommit.NewHandler(codecommit.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion))
	require.NoError(t, h2.Restore(t.Context(), snap))

	got, err := h2.Backend.GetRepository("delegate-repo")
	require.NoError(t, err)
	assert.Equal(t, "delegate-repo", got.RepositoryName)
}

// TestInMemoryBackend_Snapshot_EmptyBackend verifies Snapshot/Restore round
// trips cleanly on a backend with no data at all -- every plain map and
// registered table must decode back to empty, not nil-panic or error.
func TestInMemoryBackend_Snapshot_EmptyBackend(t *testing.T) {
	t.Parallel()

	original := codecommit.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := codecommit.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	require.NoError(t, fresh.Restore(t.Context(), snap))
	assert.Empty(t, fresh.ListRepositories())
	assert.Empty(t, fresh.ListApprovalRuleTemplates())
}
