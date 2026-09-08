package codecommit

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BatchDescribeMergeConflicts describes merge conflicts between two commits.
// This is a stub implementation — it returns empty conflicts since the backend
// does not track file-level content.
func (b *InMemoryBackend) BatchDescribeMergeConflicts(
	repositoryName, destinationCommitSpecifier, sourceCommitSpecifier, _ string,
	filePaths []string,
) (*BatchDescribeMergeConflictsResult, error) {
	b.mu.RLock("BatchDescribeMergeConflicts")
	defer b.mu.RUnlock()

	if !b.repositories.Has(repositoryName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repositoryName)
	}

	result := &BatchDescribeMergeConflictsResult{
		DestinationCommitID: destinationCommitSpecifier,
		SourceCommitID:      sourceCommitSpecifier,
		Conflicts:           []MergeConflict{},
	}

	if len(filePaths) > 0 {
		result.Conflicts = make([]MergeConflict, 0, len(filePaths))
		for _, fp := range filePaths {
			result.Conflicts = append(result.Conflicts, MergeConflict{
				ConflictMetadata: ConflictMetadata{
					FilePath:          fp,
					NumberOfConflicts: 0,
					ContentConflict:   false,
				},
				MergeHunks: []MergeHunk{},
			})
		}
	}

	return result, nil
}

// MergePullRequestByFastForward merges a pull request by fast-forward strategy.
func (b *InMemoryBackend) MergePullRequestByFastForward(
	prID, _ /* repoName */, _ /* sourceRef */ string,
) (*PullRequest, error) {
	b.mu.Lock("MergePullRequestByFastForward")
	defer b.mu.Unlock()

	pr, ok := b.pullRequests.Get(prID)
	if !ok {
		return nil, fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}
	if pr.PullRequestStatus == prStatusClosed {
		return nil, fmt.Errorf("%w: pull request %s is already closed", ErrPullRequestAlreadyMerged, prID)
	}
	pr.PullRequestStatus = prStatusClosed
	pr.LastActivityDate = time.Now().UTC()
	cp := *pr

	return &cp, nil
}

// MergePullRequestBySquash merges a pull request by squash strategy.
func (b *InMemoryBackend) MergePullRequestBySquash(
	prID, _ /* repoName */, _ /* sourceRef */ string,
) (*PullRequest, error) {
	b.mu.Lock("MergePullRequestBySquash")
	defer b.mu.Unlock()

	pr, ok := b.pullRequests.Get(prID)
	if !ok {
		return nil, fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}
	if pr.PullRequestStatus == prStatusClosed {
		return nil, fmt.Errorf("%w: pull request %s is already closed", ErrPullRequestAlreadyMerged, prID)
	}
	pr.PullRequestStatus = prStatusClosed
	pr.LastActivityDate = time.Now().UTC()
	cp := *pr

	return &cp, nil
}

// MergePullRequestByThreeWay merges a pull request by three-way strategy.
func (b *InMemoryBackend) MergePullRequestByThreeWay(
	prID, _ /* repoName */, _ /* sourceRef */ string,
) (*PullRequest, error) {
	b.mu.Lock("MergePullRequestByThreeWay")
	defer b.mu.Unlock()

	pr, ok := b.pullRequests.Get(prID)
	if !ok {
		return nil, fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}
	if pr.PullRequestStatus == prStatusClosed {
		return nil, fmt.Errorf("%w: pull request %s is already closed", ErrPullRequestAlreadyMerged, prID)
	}
	pr.PullRequestStatus = prStatusClosed
	pr.LastActivityDate = time.Now().UTC()
	cp := *pr

	return &cp, nil
}

// resolveCommitSpecifier resolves a branch name or full commit ID to a commit
// ID. Real AWS specifiers can also be a tag or HEAD; this backend has no tag
// concept and no separate HEAD pointer, so those are out of scope. Caller
// must hold at least the read lock.
func (b *InMemoryBackend) resolveCommitSpecifier(repoName, specifier string) (string, error) {
	if specifier == "" {
		return "", fmt.Errorf("%w: commit specifier is required", ErrValidation)
	}
	if branch, ok := b.branches.Get(branchKey(repoName, specifier)); ok {
		return branch.CommitID, nil
	}
	if _, ok := b.commits.Get(commitKey(repoName, specifier)); ok {
		return specifier, nil
	}

	return "", fmt.Errorf("%w: commit specifier %s not found", ErrCommitNotFound, specifier)
}

// MergeBranchesOptions carries the optional fields MergeBranchesBySquash and
// MergeBranchesByThreeWay accept beyond the two commit specifiers.
type MergeBranchesOptions struct {
	TargetBranch  string
	CommitMessage string
	AuthorName    string
	Email         string
}

// MergeBranchesBySquash merges two branches by creating a single new commit,
// on top of the destination tip, that represents the squashed result. Unlike
// a real 3-way merge, a squash commit has exactly one parent (the destination
// tip) — the source branch's individual commits are not preserved as parents.
// Content-level squashing is not modeled (see PARITY.md gaps): File has no
// per-branch identity, so there is no second version of a file to combine.
func (b *InMemoryBackend) MergeBranchesBySquash(
	repoName, sourceSpecifier, destSpecifier string, opts MergeBranchesOptions,
) (*Commit, error) {
	b.mu.Lock("MergeBranchesBySquash")
	defer b.mu.Unlock()

	if !b.repositories.Has(repoName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	if _, err := b.resolveCommitSpecifier(repoName, sourceSpecifier); err != nil {
		return nil, err
	}
	destCommitID, err := b.resolveCommitSpecifier(repoName, destSpecifier)
	if err != nil {
		return nil, err
	}

	targetBranch := opts.TargetBranch
	if targetBranch == "" {
		targetBranch = destSpecifier
	}
	message := opts.CommitMessage
	if message == "" {
		message = fmt.Sprintf("Squash merge of %s into %s", sourceSpecifier, destSpecifier)
	}

	commitID := uuid.NewString()
	commit := &Commit{
		CommitID:       commitID,
		TreeID:         uuid.NewString(),
		Message:        message,
		AuthorName:     opts.AuthorName,
		AuthorEmail:    opts.Email,
		CommitterName:  opts.AuthorName,
		CommitterEmail: opts.Email,
		RepositoryName: repoName,
		Parents:        []string{destCommitID},
		CreatedAt:      time.Now().UTC(),
	}
	b.commits.Put(commit)
	b.branches.Put(&Branch{BranchName: targetBranch, CommitID: commitID, RepositoryName: repoName})

	cp := *commit
	cp.Parents = append([]string(nil), commit.Parents...)

	return &cp, nil
}

// MergeBranchesByThreeWay merges two branches by creating a real merge
// commit with two parents (destination tip first, then source tip) — unlike
// MergeBranchesBySquash's single-parent commit and MergeBranchesByFastForward's
// pointer move. Content-level 3-way merge is not modeled (see PARITY.md gaps),
// same root cause as the squash strategy.
func (b *InMemoryBackend) MergeBranchesByThreeWay(
	repoName, sourceSpecifier, destSpecifier string, opts MergeBranchesOptions,
) (*Commit, error) {
	b.mu.Lock("MergeBranchesByThreeWay")
	defer b.mu.Unlock()

	if !b.repositories.Has(repoName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	sourceCommitID, err := b.resolveCommitSpecifier(repoName, sourceSpecifier)
	if err != nil {
		return nil, err
	}
	destCommitID, err := b.resolveCommitSpecifier(repoName, destSpecifier)
	if err != nil {
		return nil, err
	}

	targetBranch := opts.TargetBranch
	if targetBranch == "" {
		targetBranch = destSpecifier
	}
	message := opts.CommitMessage
	if message == "" {
		message = fmt.Sprintf("Merge %s into %s", sourceSpecifier, destSpecifier)
	}

	commitID := uuid.NewString()
	commit := &Commit{
		CommitID:       commitID,
		TreeID:         uuid.NewString(),
		Message:        message,
		AuthorName:     opts.AuthorName,
		AuthorEmail:    opts.Email,
		CommitterName:  opts.AuthorName,
		CommitterEmail: opts.Email,
		RepositoryName: repoName,
		Parents:        []string{destCommitID, sourceCommitID},
		CreatedAt:      time.Now().UTC(),
	}
	b.commits.Put(commit)
	b.branches.Put(&Branch{BranchName: targetBranch, CommitID: commitID, RepositoryName: repoName})

	cp := *commit
	cp.Parents = append([]string(nil), commit.Parents...)

	return &cp, nil
}

// MergeBranchesByFastForward merges branches by fast-forward: the target
// branch's tip is moved to point at the resolved source commit. Unlike
// Squash/ThreeWay, a fast-forward never creates a new commit object — that
// is the defining property of the strategy — so, unlike them, it returns the
// existing source commit rather than a fabricated one.
func (b *InMemoryBackend) MergeBranchesByFastForward(
	repoName, sourceRef, destinationRef, targetBranch string,
) (*Commit, error) {
	b.mu.Lock("MergeBranchesByFastForward")
	defer b.mu.Unlock()

	if !b.repositories.Has(repoName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	sourceCommitID, err := b.resolveCommitSpecifier(repoName, sourceRef)
	if err != nil {
		return nil, err
	}
	if _, destErr := b.resolveCommitSpecifier(repoName, destinationRef); destErr != nil {
		return nil, destErr
	}

	branch := targetBranch
	if branch == "" {
		branch = destinationRef
	}
	b.branches.Put(&Branch{
		BranchName:     branch,
		CommitID:       sourceCommitID,
		RepositoryName: repoName,
	})

	commit, ok := b.commits.Get(commitKey(repoName, sourceCommitID))
	if !ok {
		return nil, fmt.Errorf("%w: commit %s not found", ErrCommitNotFound, sourceCommitID)
	}
	cp := *commit
	cp.Parents = append([]string(nil), commit.Parents...)

	return &cp, nil
}

// GetMergeOptions returns the available merge options for two branches.
func (b *InMemoryBackend) GetMergeOptions(
	repoName, _ /* sourceRef */, _ /* destinationRef */ string,
) ([]string, error) {
	b.mu.RLock("GetMergeOptions")
	defer b.mu.RUnlock()

	if !b.repositories.Has(repoName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	return []string{"FAST_FORWARD_MERGE", "SQUASH_MERGE", "THREE_WAY_MERGE"}, nil
}

// CreateUnreferencedMergeCommit creates a new unreferenced merge commit.
func (b *InMemoryBackend) CreateUnreferencedMergeCommit(
	repoName, sourceCommitID, destinationCommitID, authorName, authorEmail, message string,
) (*Commit, error) {
	b.mu.Lock("CreateUnreferencedMergeCommit")
	defer b.mu.Unlock()

	if !b.repositories.Has(repoName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	if message == "" {
		message = "Unreferenced merge commit"
	}

	commitID := uuid.NewString()
	treeID := uuid.NewString()
	now := time.Now().UTC()
	commit := &Commit{
		CommitID:       commitID,
		TreeID:         treeID,
		Message:        message,
		AuthorName:     authorName,
		AuthorEmail:    authorEmail,
		CommitterName:  authorName,
		CommitterEmail: authorEmail,
		RepositoryName: repoName,
		Parents:        []string{sourceCommitID, destinationCommitID},
		CreatedAt:      now,
	}
	b.commits.Put(commit)
	cp := *commit
	cp.Parents = make([]string, len(commit.Parents))
	copy(cp.Parents, commit.Parents)

	return &cp, nil
}

// GetMergeCommit returns a commit that has both sourceCommitSpecifier and
// destinationCommitSpecifier as parents, or falls back to the most recent commit.
func (b *InMemoryBackend) GetMergeCommit(
	repoName, sourceCommitSpecifier, destinationCommitSpecifier string,
) (*Commit, error) {
	b.mu.RLock("GetMergeCommit")
	defer b.mu.RUnlock()

	if !b.repositories.Has(repoName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	repoCommits := b.commitsByRepo.Get(repoName)

	// Prefer a commit whose parents include both specifiers (real merge commit shape).
	for _, c := range repoCommits {
		hasSource, hasDest := false, false
		for _, p := range c.Parents {
			if p == sourceCommitSpecifier {
				hasSource = true
			}
			if p == destinationCommitSpecifier {
				hasDest = true
			}
		}
		if hasSource && hasDest {
			cp := *c
			cp.Parents = make([]string, len(c.Parents))
			copy(cp.Parents, c.Parents)

			return &cp, nil
		}
	}

	// Fallback: return the most recent commit.
	var latest *Commit
	for _, c := range repoCommits {
		if latest == nil || c.CreatedAt.After(latest.CreatedAt) {
			latest = c
		}
	}
	if latest != nil {
		cp := *latest
		cp.Parents = make([]string, len(latest.Parents))
		copy(cp.Parents, latest.Parents)

		return &cp, nil
	}

	return nil, fmt.Errorf("%w: no commits found in repository %s", ErrCommitNotFound, repoName)
}

// GetMergeConflicts resolves the source/destination commit specifiers and
// reports whether the merge is mergeable. It validates both specifiers refer
// to a real branch or commit (a real client-detectable error path this op
// previously skipped entirely). mergeable is always true and conflicts are
// always empty: this backend has no per-branch file identity to diff (see
// PARITY.md gaps) — the exception is FAST_FORWARD_MERGE, for which AWS's own
// documented contract is "conflicts always empty", so that case is genuinely
// correct rather than a gap.
func (b *InMemoryBackend) GetMergeConflicts(
	repoName, sourceCommitSpecifier, destCommitSpecifier, _ /* mergeOption */ string,
) (bool, string, string, error) {
	b.mu.RLock("GetMergeConflicts")
	defer b.mu.RUnlock()

	if !b.repositories.Has(repoName) {
		return false, "", "", fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	sourceCommitID, err := b.resolveCommitSpecifier(repoName, sourceCommitSpecifier)
	if err != nil {
		return false, "", "", err
	}
	destCommitID, err := b.resolveCommitSpecifier(repoName, destCommitSpecifier)
	if err != nil {
		return false, "", "", err
	}

	return true, sourceCommitID, destCommitID, nil
}
