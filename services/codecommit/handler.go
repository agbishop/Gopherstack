package codecommit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	keyRepositoryID     = "repositoryId"
	keyRepositoryName   = "repositoryName"
	keyCreationDate     = "creationDate"
	keyErrors           = "errors"
	keyMessage          = "message"
	keyCommitID         = "commitId"
	keyTreeID           = "treeId"
	keyLastModifiedDate = "lastModifiedDate"
	keyApprovalRuleTmpl = "approvalRuleTemplate"
	keyPullRequest      = "pullRequest"
	keyComment          = "comment"
	keySourceCommitID   = "sourceCommitId"
	keyDestCommitID     = "destinationCommitId"
	keyBlobID           = "blobId"
	keyFilePath         = "filePath"
	keyFileMode         = "fileMode"
	keyAfterCommitID    = "afterCommitId"
	keyPullRequestID    = "pullRequestId"
	keyAbsolutePath     = "absolutePath"
	keyApprovalRuleID   = "approvalRuleId"
	fileModeNormal      = "NORMAL"
)

const codecommitTargetPrefix = "CodeCommit_20150413."

var (
	errUnknownAction  = errors.New("unknown action")
	errInvalidRequest = errors.New("invalid request")
)

// paginateStrings slices a sorted string slice using the nextToken cursor and maxResults limit.
// The nextToken is an opaque decimal index into the slice.
// Returns the page and the next token (empty string if no more pages).
func paginateStrings(items []string, nextToken string, maxResults int) ([]string, string) {
	start := 0
	if nextToken != "" {
		if idx, err := strconv.Atoi(nextToken); err == nil && idx >= 0 {
			start = idx
		}
	}
	if start > len(items) {
		start = len(items)
	}
	end := len(items)
	if maxResults > 0 && start+maxResults < end {
		end = start + maxResults
	}
	page := items[start:end]
	token := ""
	if end < len(items) {
		token = strconv.Itoa(end)
	}

	return page, token
}

// Handler is the Echo HTTP handler for AWS CodeCommit operations.
type Handler struct {
	Backend *InMemoryBackend
	ops     map[string]func([]byte) (any, error)
}

// NewHandler creates a new CodeCommit handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{Backend: backend}
	h.ops = h.buildOps()

	return h
}

// Reset clears all handler and backend state.
func (h *Handler) Reset() {
	h.Backend.Reset()
}

// buildOps returns the dispatch table mapping action name to handler function.
func (h *Handler) buildOps() map[string]func([]byte) (any, error) {
	batchDisassoc := h.handleBatchDisassociateApprovalRuleTemplateFromRepositories

	return map[string]func([]byte) (any, error){
		"AssociateApprovalRuleTemplateWithRepository":           h.handleAssociateApprovalRuleTemplateWithRepository,
		"BatchAssociateApprovalRuleTemplateWithRepositories":    h.handleBatchAssociateApprovalRuleTemplateWithRepositories,
		"BatchDescribeMergeConflicts":                           h.handleBatchDescribeMergeConflicts,
		"BatchDisassociateApprovalRuleTemplateFromRepositories": batchDisassoc,
		"BatchGetCommits":            h.handleBatchGetCommits,
		"BatchGetRepositories":       h.handleBatchGetRepositories,
		"CreateApprovalRuleTemplate": h.handleCreateApprovalRuleTemplate,
		"CreateBranch":               h.handleCreateBranch,
		"CreateCommit":               h.handleCreateCommit,
		"CreatePullRequest":          h.handleCreatePullRequest,
		"CreateRepository":           h.handleCreateRepository,
		"GetRepository":              h.handleGetRepository,
		"DeleteRepository":           h.handleDeleteRepository,
		"ListRepositories":           h.handleListRepositories,
		"TagResource":                h.handleTagResource,
		"UntagResource":              h.handleUntagResource,
		"ListTagsForResource":        h.handleListTagsForResource,
		// Implemented ops
		"CreatePullRequestApprovalRule":                    h.handleCreatePullRequestApprovalRule,
		"CreateUnreferencedMergeCommit":                    h.handleCreateUnreferencedMergeCommit,
		"DeleteApprovalRuleTemplate":                       h.handleDeleteApprovalRuleTemplate,
		"DeleteBranch":                                     h.handleDeleteBranch,
		"DeleteCommentContent":                             h.handleDeleteCommentContent,
		"DeleteFile":                                       h.handleDeleteFile,
		"DeletePullRequestApprovalRule":                    h.handleDeletePullRequestApprovalRule,
		"DescribeMergeConflicts":                           h.handleDescribeMergeConflicts,
		"DescribePullRequestEvents":                        h.handleDescribePullRequestEvents,
		"DisassociateApprovalRuleTemplateFromRepository":   h.handleDisassociateApprovalRuleTemplateFromRepository,
		"EvaluatePullRequestApprovalRules":                 h.handleEvaluatePullRequestApprovalRules,
		"GetApprovalRuleTemplate":                          h.handleGetApprovalRuleTemplate,
		"GetBlob":                                          h.handleGetBlob,
		"GetBranch":                                        h.handleGetBranch,
		"GetComment":                                       h.handleGetComment,
		"GetCommentReactions":                              h.handleGetCommentReactions,
		"GetCommentsForComparedCommit":                     h.handleGetCommentsForComparedCommit,
		"GetCommentsForPullRequest":                        h.handleGetCommentsForPullRequest,
		"GetCommit":                                        h.handleGetCommit,
		"GetDifferences":                                   h.handleGetDifferences,
		"GetFile":                                          h.handleGetFile,
		"GetFolder":                                        h.handleGetFolder,
		"GetMergeCommit":                                   h.handleGetMergeCommit,
		"GetMergeConflicts":                                h.handleGetMergeConflicts,
		"GetMergeOptions":                                  h.handleGetMergeOptions,
		"GetPullRequest":                                   h.handleGetPullRequest,
		"GetPullRequestApprovalStates":                     h.handleGetPullRequestApprovalStates,
		"GetPullRequestOverrideState":                      h.handleGetPullRequestOverrideState,
		"GetRepositoryTriggers":                            h.handleGetRepositoryTriggers,
		"ListApprovalRuleTemplates":                        h.handleListApprovalRuleTemplates,
		"ListAssociatedApprovalRuleTemplatesForRepository": h.handleListAssociatedApprovalRuleTemplatesForRepository,
		"ListBranches":                                     h.handleListBranches,
		"ListFileCommitHistory":                            h.handleListFileCommitHistory,
		"ListPullRequests":                                 h.handleListPullRequests,
		"ListRepositoriesForApprovalRuleTemplate":          h.handleListRepositoriesForApprovalRuleTemplate,
		"MergeBranchesByFastForward":                       h.handleMergeBranchesByFastForward,
		"MergeBranchesBySquash":                            h.handleMergeBranchesBySquash,
		"MergeBranchesByThreeWay":                          h.handleMergeBranchesByThreeWay,
		"MergePullRequestByFastForward":                    h.handleMergePullRequestByFastForward,
		"MergePullRequestBySquash":                         h.handleMergePullRequestBySquash,
		"MergePullRequestByThreeWay":                       h.handleMergePullRequestByThreeWay,
		"OverridePullRequestApprovalRules":                 h.handleOverridePullRequestApprovalRules,
		"PostCommentForComparedCommit":                     h.handlePostCommentForComparedCommit,
		"PostCommentForPullRequest":                        h.handlePostCommentForPullRequest,
		"PostCommentReply":                                 h.handlePostCommentReply,
		"PutCommentReaction":                               h.handlePutCommentReaction,
		"PutFile":                                          h.handlePutFile,
		"PutRepositoryTriggers":                            h.handlePutRepositoryTriggers,
		"TestRepositoryTriggers":                           h.handleTestRepositoryTriggers,
		"UpdateApprovalRuleTemplateContent":                h.handleUpdateApprovalRuleTemplateContent,
		"UpdateApprovalRuleTemplateDescription":            h.handleUpdateApprovalRuleTemplateDescription,
		"UpdateApprovalRuleTemplateName":                   h.handleUpdateApprovalRuleTemplateName,
		"UpdateComment":                                    h.handleUpdateComment,
		"UpdateDefaultBranch":                              h.handleUpdateDefaultBranch,
		"UpdatePullRequestApprovalRuleContent":             h.handleUpdatePullRequestApprovalRuleContent,
		"UpdatePullRequestApprovalState":                   h.handleUpdatePullRequestApprovalState,
		"UpdatePullRequestDescription":                     h.handleUpdatePullRequestDescription,
		"UpdatePullRequestStatus":                          h.handleUpdatePullRequestStatus,
		"UpdatePullRequestTitle":                           h.handleUpdatePullRequestTitle,
		"UpdateRepositoryDescription":                      h.handleUpdateRepositoryDescription,
		"UpdateRepositoryEncryptionKey":                    h.handleUpdateRepositoryEncryptionKey,
		"UpdateRepositoryName":                             h.handleUpdateRepositoryName,
	}
}

// Name returns the service name.
func (h *Handler) Name() string { return "CodeCommit" }

// GetSupportedOperations returns the list of supported CodeCommit operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"AssociateApprovalRuleTemplateWithRepository",
		"BatchAssociateApprovalRuleTemplateWithRepositories",
		"BatchDescribeMergeConflicts",
		"BatchDisassociateApprovalRuleTemplateFromRepositories",
		"BatchGetCommits",
		"BatchGetRepositories",
		"CreateApprovalRuleTemplate",
		"CreateBranch",
		"CreateCommit",
		"CreatePullRequest",
		"CreatePullRequestApprovalRule",
		"CreateRepository",
		"CreateUnreferencedMergeCommit",
		"DeleteApprovalRuleTemplate",
		"DeleteBranch",
		"DeleteCommentContent",
		"DeleteFile",
		"DeletePullRequestApprovalRule",
		"DeleteRepository",
		"DescribeMergeConflicts",
		"DescribePullRequestEvents",
		"DisassociateApprovalRuleTemplateFromRepository",
		"EvaluatePullRequestApprovalRules",
		"GetApprovalRuleTemplate",
		"GetBlob",
		"GetBranch",
		"GetComment",
		"GetCommentReactions",
		"GetCommentsForComparedCommit",
		"GetCommentsForPullRequest",
		"GetCommit",
		"GetDifferences",
		"GetFile",
		"GetFolder",
		"GetMergeCommit",
		"GetMergeConflicts",
		"GetMergeOptions",
		"GetPullRequest",
		"GetPullRequestApprovalStates",
		"GetPullRequestOverrideState",
		"GetRepository",
		"GetRepositoryTriggers",
		"ListApprovalRuleTemplates",
		"ListAssociatedApprovalRuleTemplatesForRepository",
		"ListBranches",
		"ListFileCommitHistory",
		"ListPullRequests",
		"ListRepositories",
		"ListRepositoriesForApprovalRuleTemplate",
		"ListTagsForResource",
		"MergeBranchesByFastForward",
		"MergeBranchesBySquash",
		"MergeBranchesByThreeWay",
		"MergePullRequestByFastForward",
		"MergePullRequestBySquash",
		"MergePullRequestByThreeWay",
		"OverridePullRequestApprovalRules",
		"PostCommentForComparedCommit",
		"PostCommentForPullRequest",
		"PostCommentReply",
		"PutCommentReaction",
		"PutFile",
		"PutRepositoryTriggers",
		"TagResource",
		"TestRepositoryTriggers",
		"UntagResource",
		"UpdateApprovalRuleTemplateContent",
		"UpdateApprovalRuleTemplateDescription",
		"UpdateApprovalRuleTemplateName",
		"UpdateComment",
		"UpdateDefaultBranch",
		"UpdatePullRequestApprovalRuleContent",
		"UpdatePullRequestApprovalState",
		"UpdatePullRequestDescription",
		"UpdatePullRequestStatus",
		"UpdatePullRequestTitle",
		"UpdateRepositoryDescription",
		"UpdateRepositoryEncryptionKey",
		"UpdateRepositoryName",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "codecommit" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this CodeCommit instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches AWS CodeCommit requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), codecommitTargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// ExtractOperation extracts the CodeCommit operation from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	action := strings.TrimPrefix(target, codecommitTargetPrefix)
	if action == "" || action == target {
		return "Unknown"
	}

	return action
}

// ExtractResource extracts the repository name from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, readErr := httputils.ReadBody(c.Request())
	if readErr != nil {
		return ""
	}

	var input struct {
		RepositoryName string `json:"repositoryName"`
	}
	if jsonErr := json.Unmarshal(body, &input); jsonErr != nil {
		return ""
	}

	return input.RepositoryName
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c, logger.Load(c.Request().Context()),
			"CodeCommit", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

// dispatch routes the operation to the appropriate handler and marshals the response.
func (h *Handler) dispatch(_ context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnknownAction, action)
	}

	resp, err := fn(body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(resp)
}

// errCodeEntry maps a backend sentinel error to its AWS HTTP status and exception type.
type errCodeEntry struct {
	sentinel error
	errType  string
	code     int
}

// errCodeLookup is checked in order against err via errors.Is; the first match wins.
// Extend this table (not handleError's control flow) when a new backend sentinel error
// is introduced, so handleError itself never grows another branch.
//
//nolint:gochecknoglobals // static AWS error-code lookup table, same pattern as other services' errCodeLookup
var errCodeLookup = []errCodeEntry{
	{sentinel: ErrNotFound, code: http.StatusNotFound, errType: "RepositoryDoesNotExistException"},
	{sentinel: ErrAlreadyExists, code: http.StatusBadRequest, errType: "RepositoryNameExistsException"},
	{
		sentinel: ErrApprovalRuleTemplateNotFound,
		code:     http.StatusNotFound,
		errType:  "ApprovalRuleTemplateDoesNotExistException",
	},
	{
		sentinel: ErrApprovalRuleTemplateAlreadyExists,
		code:     http.StatusBadRequest,
		errType:  "ApprovalRuleTemplateNameAlreadyExistsException",
	},
	{sentinel: ErrBranchNotFound, code: http.StatusNotFound, errType: "BranchDoesNotExistException"},
	{sentinel: ErrBranchAlreadyExists, code: http.StatusBadRequest, errType: "BranchNameExistsException"},
	{sentinel: ErrCommitNotFound, code: http.StatusNotFound, errType: "CommitDoesNotExistException"},
	{sentinel: ErrCommitIDNotFound, code: http.StatusNotFound, errType: "CommitIdDoesNotExistException"},
	{sentinel: ErrFileNotFound, code: http.StatusNotFound, errType: "FileDoesNotExistException"},
	{sentinel: ErrBlobNotFound, code: http.StatusNotFound, errType: "BlobIdDoesNotExistException"},
	{sentinel: ErrCommentNotFound, code: http.StatusNotFound, errType: "CommentDoesNotExistException"},
	{sentinel: ErrApprovalRuleNotFound, code: http.StatusNotFound, errType: "ApprovalRuleDoesNotExistException"},
	{sentinel: ErrPullRequestNotFound, code: http.StatusNotFound, errType: "PullRequestDoesNotExistException"},
	{
		sentinel: ErrPullRequestAlreadyMerged,
		code:     http.StatusBadRequest,
		errType:  "PullRequestAlreadyClosedException",
	},
	{sentinel: ErrInvalidRepositoryName, code: http.StatusBadRequest, errType: "InvalidRepositoryNameException"},
	{
		sentinel: ErrMaxRepositoriesExceeded,
		code:     http.StatusBadRequest,
		errType:  "MaximumRepositoryNamesExceededException",
	},
	// These six sentinels are declared in errors.go and (except the last two,
	// which are currently unused pending SameFileContent/submodule-path
	// detection — see PARITY.md) actively returned by validateBranchName,
	// CreateCommit, and DeleteFile, but were missing from this table until
	// this pass: every one of them fell through to the generic 400
	// ValidationException below instead of its real AWS exception name.
	{sentinel: ErrBranchNameRequired, code: http.StatusBadRequest, errType: "BranchNameRequiredException"},
	{sentinel: ErrInvalidBranchName, code: http.StatusBadRequest, errType: "InvalidBranchNameException"},
	{sentinel: ErrParentCommitIDRequired, code: http.StatusBadRequest, errType: "ParentCommitIdRequiredException"},
	{sentinel: ErrParentCommitIDOutdated, code: http.StatusBadRequest, errType: "ParentCommitIdOutdatedException"},
	{sentinel: ErrSameFileContent, code: http.StatusBadRequest, errType: "SameFileContentException"},
	{sentinel: ErrNoChange, code: http.StatusBadRequest, errType: "NoChangeException"},
	{
		sentinel: ErrFilePathConflicts,
		code:     http.StatusBadRequest,
		errType:  "FilePathConflictsWithSubmodulePathException",
	},
	{
		sentinel: ErrInvalidPullRequestEventType,
		code:     http.StatusBadRequest,
		errType:  "InvalidPullRequestEventTypeException",
	},
	{sentinel: ErrValidation, code: http.StatusBadRequest, errType: "InvalidParameterException"},
	{sentinel: ErrInvalidMergeOption, code: http.StatusBadRequest, errType: "InvalidMergeOptionException"},
	{
		sentinel: ErrInvalidPullRequestStatus,
		code:     http.StatusBadRequest,
		errType:  "InvalidPullRequestStatusException",
	},
	{
		sentinel: ErrInvalidContinuationToken,
		code:     http.StatusBadRequest,
		errType:  "InvalidContinuationTokenException",
	},
	{sentinel: errInvalidRequest, code: http.StatusBadRequest, errType: "ValidationException"},
}

// handleError maps backend errors to HTTP error responses.
func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	code := http.StatusBadRequest
	errType := "ValidationException"

	for _, entry := range errCodeLookup {
		if errors.Is(err, entry.sentinel) {
			code = entry.code
			errType = entry.errType

			break
		}
	}

	return c.JSON(code, map[string]string{
		"__type":   errType,
		keyMessage: err.Error(),
	})
}
