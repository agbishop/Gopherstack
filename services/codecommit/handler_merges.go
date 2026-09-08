package codecommit

import (
	"encoding/json"
	"fmt"
)

type batchDescribeMergeConflictsInput struct {
	RepositoryName             string   `json:"repositoryName"`
	DestinationCommitSpecifier string   `json:"destinationCommitSpecifier"`
	SourceCommitSpecifier      string   `json:"sourceCommitSpecifier"`
	MergeOption                string   `json:"mergeOption"`
	FilePaths                  []string `json:"filePaths"`
}

// validMergeOptions are the AWS-accepted values for the mergeOption parameter.
func isValidMergeOption(opt string) bool {
	switch opt {
	case "FAST_FORWARD_MERGE", "SQUASH_MERGE", "THREE_WAY_MERGE":
		return true
	}

	return false
}

func (h *Handler) handleBatchDescribeMergeConflicts(body []byte) (any, error) {
	var in batchDescribeMergeConflictsInput
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if in.RepositoryName == "" {
		return nil, fmt.Errorf("%w: repositoryName is required", errInvalidRequest)
	}

	if in.DestinationCommitSpecifier == "" {
		return nil, fmt.Errorf("%w: destinationCommitSpecifier is required", errInvalidRequest)
	}

	if in.SourceCommitSpecifier == "" {
		return nil, fmt.Errorf("%w: sourceCommitSpecifier is required", errInvalidRequest)
	}

	if in.MergeOption == "" {
		return nil, fmt.Errorf("%w: mergeOption is required", errInvalidRequest)
	}

	if !isValidMergeOption(in.MergeOption) {
		return nil, fmt.Errorf(
			"%w: mergeOption must be FAST_FORWARD_MERGE, SQUASH_MERGE, or THREE_WAY_MERGE",
			ErrInvalidMergeOption,
		)
	}

	result, err := h.Backend.BatchDescribeMergeConflicts(
		in.RepositoryName,
		in.DestinationCommitSpecifier,
		in.SourceCommitSpecifier,
		in.MergeOption,
		in.FilePaths,
	)
	if err != nil {
		return nil, err
	}

	errs := result.Errors
	if errs == nil {
		errs = []ConflictError{}
	}

	return map[string]any{
		"conflicts":       result.Conflicts,
		keyDestCommitID:   result.DestinationCommitID,
		keySourceCommitID: result.SourceCommitID,
		keyErrors:         errs,
	}, nil
}

func (h *Handler) handleMergePullRequestByFastForward(body []byte) (any, error) {
	var req struct {
		PullRequestID  string `json:"pullRequestId"`
		RepositoryName string `json:"repositoryName"`
		SourceCommitID string `json:"sourceCommitId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	pr, err := h.Backend.MergePullRequestByFastForward(req.PullRequestID, req.RepositoryName, req.SourceCommitID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyPullRequest: pullRequestToMap(pr),
	}, nil
}

func (h *Handler) handleMergePullRequestBySquash(body []byte) (any, error) {
	var req struct {
		PullRequestID  string `json:"pullRequestId"`
		RepositoryName string `json:"repositoryName"`
		SourceCommitID string `json:"sourceCommitId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	pr, err := h.Backend.MergePullRequestBySquash(req.PullRequestID, req.RepositoryName, req.SourceCommitID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyPullRequest: pullRequestToMap(pr),
	}, nil
}

func (h *Handler) handleMergePullRequestByThreeWay(body []byte) (any, error) {
	var req struct {
		PullRequestID  string `json:"pullRequestId"`
		RepositoryName string `json:"repositoryName"`
		SourceCommitID string `json:"sourceCommitId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	pr, err := h.Backend.MergePullRequestByThreeWay(req.PullRequestID, req.RepositoryName, req.SourceCommitID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyPullRequest: pullRequestToMap(pr),
	}, nil
}

func (h *Handler) handleMergeBranchesByFastForward(body []byte) (any, error) {
	var req struct {
		RepositoryName             string `json:"repositoryName"`
		SourceCommitSpecifier      string `json:"sourceCommitSpecifier"`
		DestinationCommitSpecifier string `json:"destinationCommitSpecifier"`
		TargetBranch               string `json:"targetBranch"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.RepositoryName == "" {
		return nil, fmt.Errorf("%w: repositoryName is required", errInvalidRequest)
	}

	commit, err := h.Backend.MergeBranchesByFastForward(
		req.RepositoryName, req.SourceCommitSpecifier, req.DestinationCommitSpecifier, req.TargetBranch,
	)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyCommitID: commit.CommitID,
		keyTreeID:   commit.TreeID,
	}, nil
}

func (h *Handler) handleGetMergeOptions(body []byte) (any, error) {
	var req struct {
		RepositoryName             string `json:"repositoryName"`
		SourceCommitSpecifier      string `json:"sourceCommitSpecifier"`
		DestinationCommitSpecifier string `json:"destinationCommitSpecifier"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.RepositoryName == "" {
		return nil, fmt.Errorf("%w: repositoryName is required", errInvalidRequest)
	}

	options, err := h.Backend.GetMergeOptions(
		req.RepositoryName, req.SourceCommitSpecifier, req.DestinationCommitSpecifier,
	)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"mergeOptions":    options,
		keySourceCommitID: req.SourceCommitSpecifier,
		keyDestCommitID:   req.DestinationCommitSpecifier,
	}, nil
}

func (h *Handler) handleCreateUnreferencedMergeCommit(body []byte) (any, error) {
	var req struct {
		RepositoryName             string `json:"repositoryName"`
		SourceCommitSpecifier      string `json:"sourceCommitSpecifier"`
		DestinationCommitSpecifier string `json:"destinationCommitSpecifier"`
		MergeOption                string `json:"mergeOption"`
		AuthorName                 string `json:"authorName"`
		Email                      string `json:"email"`
		CommitMessage              string `json:"commitMessage"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.RepositoryName == "" {
		return nil, fmt.Errorf("%w: repositoryName is required", errInvalidRequest)
	}

	if req.MergeOption == "" {
		return nil, fmt.Errorf("%w: mergeOption is required", errInvalidRequest)
	}

	if !isValidMergeOption(req.MergeOption) {
		return nil, fmt.Errorf(
			"%w: mergeOption must be FAST_FORWARD_MERGE, SQUASH_MERGE, or THREE_WAY_MERGE",
			ErrInvalidMergeOption,
		)
	}

	commit, err := h.Backend.CreateUnreferencedMergeCommit(
		req.RepositoryName, req.SourceCommitSpecifier, req.DestinationCommitSpecifier,
		req.AuthorName, req.Email, req.CommitMessage,
	)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyCommitID: commit.CommitID,
		keyTreeID:   commit.TreeID,
	}, nil
}

// handleGetMergeCommit does not decode a mergeOption field: real
// GetMergeCommitInput has no such member (codecommit@v1.36.4
// api_op_GetMergeCommit.go / awsAwsjson11_serializeOpDocumentGetMergeCommitInput
// in serializers.go), so a real client never sends one.
func (h *Handler) handleGetMergeCommit(body []byte) (any, error) {
	var req struct {
		RepositoryName             string `json:"repositoryName"`
		SourceCommitSpecifier      string `json:"sourceCommitSpecifier"`
		DestinationCommitSpecifier string `json:"destinationCommitSpecifier"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.RepositoryName == "" {
		return nil, fmt.Errorf("%w: repositoryName is required", errInvalidRequest)
	}

	commit, err := h.Backend.GetMergeCommit(
		req.RepositoryName,
		req.SourceCommitSpecifier,
		req.DestinationCommitSpecifier,
	)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keySourceCommitID: req.SourceCommitSpecifier,
		keyDestCommitID:   req.DestinationCommitSpecifier,
		"mergedCommitId":  commit.CommitID,
	}, nil
}

func (h *Handler) handleGetMergeConflicts(body []byte) (any, error) {
	var req struct {
		RepositoryName             string `json:"repositoryName"`
		SourceCommitSpecifier      string `json:"sourceCommitSpecifier"`
		DestinationCommitSpecifier string `json:"destinationCommitSpecifier"`
		MergeOption                string `json:"mergeOption"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.RepositoryName == "" {
		return nil, fmt.Errorf("%w: repositoryName is required", errInvalidRequest)
	}

	if req.SourceCommitSpecifier == "" {
		return nil, fmt.Errorf("%w: sourceCommitSpecifier is required", errInvalidRequest)
	}

	if req.DestinationCommitSpecifier == "" {
		return nil, fmt.Errorf("%w: destinationCommitSpecifier is required", errInvalidRequest)
	}

	if req.MergeOption == "" {
		return nil, fmt.Errorf("%w: mergeOption is required", errInvalidRequest)
	}

	if !isValidMergeOption(req.MergeOption) {
		return nil, fmt.Errorf(
			"%w: mergeOption must be FAST_FORWARD_MERGE, SQUASH_MERGE, or THREE_WAY_MERGE",
			ErrInvalidMergeOption,
		)
	}

	mergeable, sourceCommitID, destCommitID, err := h.Backend.GetMergeConflicts(
		req.RepositoryName, req.SourceCommitSpecifier, req.DestinationCommitSpecifier, req.MergeOption,
	)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"mergeable":            mergeable,
		keySourceCommitID:      sourceCommitID,
		keyDestCommitID:        destCommitID,
		"conflictMetadataList": []any{},
	}, nil
}

// handleDescribeMergeConflicts describes merge conflicts for a single file by
// delegating to the same backend logic BatchDescribeMergeConflicts uses,
// scoped to one filePath. This validates the repository/required fields and
// reads real backend state instead of echoing the request back unexamined.
func (h *Handler) handleDescribeMergeConflicts(body []byte) (any, error) {
	var req struct {
		RepositoryName             string `json:"repositoryName"`
		DestinationCommitSpecifier string `json:"destinationCommitSpecifier"`
		SourceCommitSpecifier      string `json:"sourceCommitSpecifier"`
		MergeOption                string `json:"mergeOption"`
		FilePath                   string `json:"filePath"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	if req.RepositoryName == "" {
		return nil, fmt.Errorf("%w: repositoryName is required", errInvalidRequest)
	}

	if req.DestinationCommitSpecifier == "" {
		return nil, fmt.Errorf("%w: destinationCommitSpecifier is required", errInvalidRequest)
	}

	if req.SourceCommitSpecifier == "" {
		return nil, fmt.Errorf("%w: sourceCommitSpecifier is required", errInvalidRequest)
	}

	if req.FilePath == "" {
		return nil, fmt.Errorf("%w: filePath is required", errInvalidRequest)
	}

	if req.MergeOption == "" {
		return nil, fmt.Errorf("%w: mergeOption is required", errInvalidRequest)
	}

	if !isValidMergeOption(req.MergeOption) {
		return nil, fmt.Errorf(
			"%w: mergeOption must be FAST_FORWARD_MERGE, SQUASH_MERGE, or THREE_WAY_MERGE",
			ErrInvalidMergeOption,
		)
	}

	result, err := h.Backend.BatchDescribeMergeConflicts(
		req.RepositoryName, req.DestinationCommitSpecifier, req.SourceCommitSpecifier,
		req.MergeOption, []string{req.FilePath},
	)
	if err != nil {
		return nil, err
	}

	meta := ConflictMetadata{FilePath: req.FilePath}
	hunks := []MergeHunk{}
	if len(result.Conflicts) > 0 {
		meta = result.Conflicts[0].ConflictMetadata
		hunks = result.Conflicts[0].MergeHunks
	}

	return map[string]any{
		keyDestCommitID:    result.DestinationCommitID,
		keySourceCommitID:  result.SourceCommitID,
		"mergeHunks":       hunks,
		"conflictMetadata": meta,
	}, nil
}

type mergeBranchesRequest struct {
	RepositoryName             string `json:"repositoryName"`
	SourceCommitSpecifier      string `json:"sourceCommitSpecifier"`
	DestinationCommitSpecifier string `json:"destinationCommitSpecifier"`
	TargetBranch               string `json:"targetBranch"`
	CommitMessage              string `json:"commitMessage"`
	AuthorName                 string `json:"authorName"`
	Email                      string `json:"email"`
}

func (r mergeBranchesRequest) options() MergeBranchesOptions {
	return MergeBranchesOptions{
		TargetBranch:  r.TargetBranch,
		CommitMessage: r.CommitMessage,
		AuthorName:    r.AuthorName,
		Email:         r.Email,
	}
}

func (h *Handler) handleMergeBranchesBySquash(body []byte) (any, error) {
	var req mergeBranchesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.RepositoryName == "" {
		return nil, fmt.Errorf("%w: repositoryName is required", errInvalidRequest)
	}

	commit, err := h.Backend.MergeBranchesBySquash(
		req.RepositoryName, req.SourceCommitSpecifier, req.DestinationCommitSpecifier, req.options(),
	)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyCommitID: commit.CommitID,
		keyTreeID:   commit.TreeID,
	}, nil
}

func (h *Handler) handleMergeBranchesByThreeWay(body []byte) (any, error) {
	var req mergeBranchesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.RepositoryName == "" {
		return nil, fmt.Errorf("%w: repositoryName is required", errInvalidRequest)
	}

	commit, err := h.Backend.MergeBranchesByThreeWay(
		req.RepositoryName, req.SourceCommitSpecifier, req.DestinationCommitSpecifier, req.options(),
	)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyCommitID: commit.CommitID,
		keyTreeID:   commit.TreeID,
	}, nil
}
