package codecommit

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// PutFileMetadata carries PutFile's optional commit-authoring fields
// (commitMessage/name/email/fileMode/parentCommitId on AWS's PutFileInput).
// Previously dropped entirely: the resulting commit always got a synthetic
// "Add <path>" message and no author/committer identity, and concurrent
// writers were never checked against parentCommitId.
type PutFileMetadata struct {
	FileMode       string
	AuthorName     string
	AuthorEmail    string
	CommitMessage  string
	ParentCommitID string
}

// PutFile stores a file and creates a commit. It returns the new commit and
// the blob ID of the stored file content (AWS's PutFileOutput.BlobId is a
// required field, so callers must round-trip this into GetBlob).
//
// parentCommitId is optional here, matching CreateCommit's established
// convention in this backend (see CreateCommit's doc comment): when
// provided, it is checked against the branch tip; when omitted, no race
// detection happens, same relaxation CreateCommit already makes versus AWS's
// stricter "required for a non-empty branch" rule.
func (b *InMemoryBackend) PutFile(
	repoName, branchName, filePath string, content []byte, meta PutFileMetadata,
) (*Commit, string, error) {
	b.mu.Lock("PutFile")
	defer b.mu.Unlock()

	if !b.repositories.Has(repoName) {
		return nil, "", fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	if existing, ok := b.files.Get(fileKey(repoName, filePath)); ok && bytes.Equal(existing.FileContent, content) {
		return nil, "", fmt.Errorf(
			"%w: file %s content is unchanged", ErrSameFileContent, filePath,
		)
	}

	var currentTip string
	if branchName != "" {
		if br, branchOK := b.branches.Get(branchKey(repoName, branchName)); branchOK {
			currentTip = br.CommitID
		}
	}

	if meta.ParentCommitID != "" && currentTip != "" && meta.ParentCommitID != currentTip {
		return nil, "", fmt.Errorf(
			"%w: parentCommitId %s does not match current branch tip %s",
			ErrParentCommitIDOutdated, meta.ParentCommitID, currentTip,
		)
	}

	commitID := uuid.NewString()
	treeID := uuid.NewString()
	now := time.Now().UTC()

	fileMode := meta.FileMode
	if fileMode == "" {
		fileMode = fileModeDefault
	}

	blobID := uuid.NewString()
	b.files.Put(&File{
		FilePath: filePath,
		// CommitSpecifier is the commit that produced this version of the
		// file — AWS's GetFile/GetFileOutput.CommitId documents it as "the
		// full commit ID of the commit that contains the content". It must
		// NOT be the branch name (a prior bug here stored branchName,
		// meaning GetFile after PutFile returned the branch name where AWS
		// clients expect a commit ID).
		CommitSpecifier: commitID,
		BlobID:          blobID,
		FileMode:        fileMode,
		FileContent:     content,
		RepoName:        repoName,
	})
	b.recordFileHistory(repoName, filePath, commitID, blobID)

	message := meta.CommitMessage
	if message == "" {
		message = "Add " + filePath
	}

	var parents []string
	if currentTip != "" {
		parents = []string{currentTip}
	}

	commit := &Commit{
		CommitID:       commitID,
		TreeID:         treeID,
		Message:        message,
		AuthorName:     meta.AuthorName,
		AuthorEmail:    meta.AuthorEmail,
		CommitterName:  meta.AuthorName,
		CommitterEmail: meta.AuthorEmail,
		RepositoryName: repoName,
		Parents:        parents,
		CreatedAt:      now,
	}
	b.commits.Put(commit)

	// Update branch tip
	if branchName != "" {
		b.branches.Put(&Branch{
			BranchName:     branchName,
			CommitID:       commitID,
			RepositoryName: repoName,
		})
	}

	cp := *commit

	return &cp, blobID, nil
}

// GetFile retrieves a file by repository, commit specifier, and path.
func (b *InMemoryBackend) GetFile(repoName, _ /* commitSpecifier */, filePath string) (*File, error) {
	b.mu.RLock("GetFile")
	defer b.mu.RUnlock()

	if !b.repositories.Has(repoName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	f, ok := b.files.Get(fileKey(repoName, filePath))
	if !ok {
		return nil, fmt.Errorf("%w: file %s not found", ErrFileNotFound, filePath)
	}
	cp := *f

	return &cp, nil
}

// GetFolder lists file paths under a folder path.
func (b *InMemoryBackend) GetFolder(repoName, _ /* commitSpecifier */, folderPath string) ([]string, error) {
	b.mu.RLock("GetFolder")
	defer b.mu.RUnlock()

	if !b.repositories.Has(repoName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	repoFiles := b.filesByRepo.Get(repoName)
	var paths []string
	prefix := folderPath
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	for _, f := range repoFiles {
		fp := f.FilePath
		if prefix == "" || fp == folderPath || len(fp) > len(prefix) && fp[:len(prefix)] == prefix {
			paths = append(paths, fp)
		}
	}
	sort.Strings(paths)

	return paths, nil
}

// GetFolderFiles returns file metadata (path, blobId, fileMode) for files under a folder path.
// This provides richer info than GetFolder for handler responses matching the AWS API shape.
func (b *InMemoryBackend) GetFolderFiles(repoName, _ /* commitSpecifier */, folderPath string) ([]*File, error) {
	b.mu.RLock("GetFolderFiles")
	defer b.mu.RUnlock()

	if !b.repositories.Has(repoName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	repoFiles := b.filesByRepo.Get(repoName)
	var files []*File
	prefix := folderPath
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	for _, f := range repoFiles {
		fp := f.FilePath
		if prefix == "" || fp == folderPath || len(fp) > len(prefix) && fp[:len(prefix)] == prefix {
			cp := *f
			files = append(files, &cp)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].FilePath < files[j].FilePath
	})

	return files, nil
}

// DeleteFileMetadata carries DeleteFile's authoring fields (commitMessage/
// name/email on AWS's DeleteFileInput, alongside the already-enforced
// parentCommitId). Previously commitMessage/name/email were dropped
// entirely: the resulting commit always got a synthetic "Delete <path>"
// message and no author/committer identity.
type DeleteFileMetadata struct {
	ParentCommitID string
	AuthorName     string
	AuthorEmail    string
	CommitMessage  string
}

// DeleteFile removes a file and creates a delete commit. It returns the new
// commit and the blob ID of the removed file (AWS's DeleteFileOutput.BlobId
// is a required field reporting the blob that was taken out of the tree).
// AWS rejects deletion of a path that does not exist with
// FileDoesNotExistException, so callers must not be able to fabricate a
// delete commit for a file that was never there.
//
// parentCommitId is a required field on AWS's DeleteFileInput (unlike
// CreateCommit, where it is optional) — verified against
// aws-sdk-go-v2/service/codecommit's validators.go, which client-side rejects
// a DeleteFileInput with a nil ParentCommitId before ever making a request.
// Real AWS documents it as "must be the HEAD commit for the branch", so a
// non-empty value that does not match the current branch tip is rejected the
// same way CreateCommit rejects a stale parentCommitId.
func (b *InMemoryBackend) DeleteFile(
	repoName, branchName, filePath string, meta DeleteFileMetadata,
) (*Commit, string, error) {
	b.mu.Lock("DeleteFile")
	defer b.mu.Unlock()

	if !b.repositories.Has(repoName) {
		return nil, "", fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	existing, ok := b.files.Get(fileKey(repoName, filePath))
	if !ok {
		return nil, "", fmt.Errorf("%w: file %s not found", ErrFileNotFound, filePath)
	}
	blobID := existing.BlobID

	var currentTip string
	if branchName != "" {
		if br, branchOK := b.branches.Get(branchKey(repoName, branchName)); branchOK {
			currentTip = br.CommitID
		}
	}

	if meta.ParentCommitID == "" {
		return nil, "", fmt.Errorf(
			"%w: parentCommitId is required and must be the current tip of branch %s",
			ErrParentCommitIDRequired, branchName,
		)
	}
	if currentTip != "" && meta.ParentCommitID != currentTip {
		return nil, "", fmt.Errorf(
			"%w: parentCommitId %s does not match current branch tip %s",
			ErrParentCommitIDOutdated, meta.ParentCommitID, currentTip,
		)
	}

	b.files.Delete(fileKey(repoName, filePath))

	commitID := uuid.NewString()
	treeID := uuid.NewString()
	now := time.Now().UTC()

	message := meta.CommitMessage
	if message == "" {
		message = "Delete " + filePath
	}

	var parents []string
	if currentTip != "" {
		parents = []string{currentTip}
	}

	commit := &Commit{
		CommitID:       commitID,
		TreeID:         treeID,
		Message:        message,
		AuthorName:     meta.AuthorName,
		AuthorEmail:    meta.AuthorEmail,
		CommitterName:  meta.AuthorName,
		CommitterEmail: meta.AuthorEmail,
		RepositoryName: repoName,
		Parents:        parents,
		CreatedAt:      now,
	}
	b.commits.Put(commit)
	b.recordFileHistory(repoName, filePath, commitID, blobID)

	// Update branch tip
	if branchName != "" {
		b.branches.Put(&Branch{
			BranchName:     branchName,
			CommitID:       commitID,
			RepositoryName: repoName,
		})
	}

	cp := *commit

	return &cp, blobID, nil
}

// GetBlob returns the content of a blob by blobID.
func (b *InMemoryBackend) GetBlob(repoName, blobID string) ([]byte, error) {
	b.mu.RLock("GetBlob")
	defer b.mu.RUnlock()

	if !b.repositories.Has(repoName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	repoFiles := b.filesByRepo.Get(repoName)
	for _, f := range repoFiles {
		if f.BlobID == blobID {
			result := make([]byte, len(f.FileContent))
			copy(result, f.FileContent)

			return result, nil
		}
	}

	return nil, fmt.Errorf("%w: blob %s not found", ErrBlobNotFound, blobID)
}

// listFileCommitHistoryDefaultMaxResults is applied when the caller does not
// supply a positive maxResults.
const listFileCommitHistoryDefaultMaxResults = 100

// ListFileCommitHistory returns a page of FileVersionEntry describing the
// commits that touched the given filePath, oldest first, each paired with
// the blob ID that commit wrote for the path (empty when that commit deleted
// it). When filePath is empty (real AWS marks FilePath required, but a raw
// HTTP client could omit it), every commit in the repository is returned
// instead, with FilePath/BlobID left empty since no single path applies.
func (b *InMemoryBackend) ListFileCommitHistory(
	repoName, filePath, nextToken string, maxResults int,
) (page.Page[FileVersionEntry], error) {
	if err := page.ValidateToken(nextToken); err != nil {
		return page.Page[FileVersionEntry]{}, fmt.Errorf("%w: invalid nextToken", ErrInvalidContinuationToken)
	}

	b.mu.RLock("ListFileCommitHistory")
	defer b.mu.RUnlock()

	if !b.repositories.Has(repoName) {
		return page.Page[FileVersionEntry]{}, fmt.Errorf("%w: repository %s not found", ErrNotFound, repoName)
	}

	var entries []FileVersionEntry
	if filePath == "" {
		entries = b.allRepoCommitVersions(repoName)
	} else {
		entries = b.fileVersionsForPath(repoName, filePath)
	}

	return page.New(entries, nextToken, maxResults, listFileCommitHistoryDefaultMaxResults), nil
}

// allRepoCommitVersions builds one FileVersionEntry per commit in repoName,
// oldest first, with no specific FilePath/BlobID (used only for the
// filePath=="" convenience case — see ListFileCommitHistory). Caller must
// hold at least a read lock.
func (b *InMemoryBackend) allRepoCommitVersions(repoName string) []FileVersionEntry {
	// Index.Get's returned slice is owned by the index and must not be
	// mutated (including by sort.Slice) — copy before sorting.
	indexed := b.commitsByRepo.Get(repoName)
	repoCommits := make([]*Commit, len(indexed))
	copy(repoCommits, indexed)
	sort.Slice(repoCommits, func(i, j int) bool {
		return repoCommits[i].CreatedAt.Before(repoCommits[j].CreatedAt)
	})

	entries := make([]FileVersionEntry, 0, len(repoCommits))
	for _, c := range repoCommits {
		cp := *c
		cp.Parents = append([]string(nil), c.Parents...)
		entries = append(entries, FileVersionEntry{Commit: &cp})
	}

	linkRevisionChildren(entries)

	return entries
}

// fileVersionsForPath builds one FileVersionEntry per fileHistory record for
// repoName/filePath, oldest first. Caller must hold at least a read lock.
func (b *InMemoryBackend) fileVersionsForPath(repoName, filePath string) []FileVersionEntry {
	history := b.fileHistory[repoName][filePath]
	entries := make([]FileVersionEntry, 0, len(history))

	for _, h := range history {
		c, exists := b.commits.Get(commitKey(repoName, h.CommitID))
		if !exists {
			continue
		}
		cp := *c
		cp.Parents = append([]string(nil), c.Parents...)
		entries = append(entries, FileVersionEntry{Commit: &cp, FilePath: filePath, BlobID: h.BlobID})
	}

	linkRevisionChildren(entries)

	return entries
}

// linkRevisionChildren sets each entry's RevisionChildren to the commit ID of
// the next (more recent) entry, matching AWS's FileVersion.RevisionChildren
// doc ("array of commit IDs that contain more recent versions of this
// file"). Our history is a simple oldest-first chain (no branch-aware
// versioning — see the models.go doc on File), so each entry has at most one
// child; the last entry has none. Must run over the full, unpaginated slice
// so a page boundary never truncates a still-valid child reference.
func linkRevisionChildren(entries []FileVersionEntry) {
	for i := range entries {
		if i+1 < len(entries) {
			entries[i].RevisionChildren = []string{entries[i+1].Commit.CommitID}
		} else {
			entries[i].RevisionChildren = []string{}
		}
	}
}
