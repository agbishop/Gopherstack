package codecommit

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

const (
	errRepoDoesNotExist             = "RepositoryDoesNotExistException"
	errApprovalRuleTemplateNotExist = "ApprovalRuleTemplateDoesNotExistException"
)

var (
	// ErrNotFound is returned when a requested resource does not exist.
	ErrNotFound = awserr.New(errRepoDoesNotExist, awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a resource already exists.
	ErrAlreadyExists = awserr.New("RepositoryNameExistsException", awserr.ErrConflict)
	// ErrValidation is returned when input validation fails.
	ErrValidation = awserr.New("InvalidParameterException", awserr.ErrInvalidParameter)
	// ErrApprovalRuleTemplateNotFound is returned when an approval rule template is not found.
	ErrApprovalRuleTemplateNotFound = awserr.New(errApprovalRuleTemplateNotExist, awserr.ErrNotFound)
	// ErrApprovalRuleTemplateAlreadyExists is returned when an approval rule template already exists.
	ErrApprovalRuleTemplateAlreadyExists = awserr.New(
		"ApprovalRuleTemplateNameAlreadyExistsException",
		awserr.ErrConflict,
	)
	// ErrBranchNotFound is returned when a branch is not found.
	ErrBranchNotFound = awserr.New("BranchDoesNotExistException", awserr.ErrNotFound)
	// ErrBranchAlreadyExists is returned when a branch already exists.
	ErrBranchAlreadyExists = awserr.New("BranchNameExistsException", awserr.ErrConflict)
	// ErrCommitNotFound is returned when a commit specifier fails to resolve
	// (e.g. a branch/merge op looking up a commit by name or ID).
	ErrCommitNotFound = awserr.New("CommitDoesNotExistException", awserr.ErrNotFound)
	// ErrCommitIDNotFound is returned by GetCommit specifically: its real AWS
	// error for an unresolvable commitId is CommitIdDoesNotExistException, a
	// distinct exception from CommitDoesNotExistException (verified against
	// codecommit@v1.36.4's deserializeOpErrorGetCommit).
	ErrCommitIDNotFound = awserr.New("CommitIdDoesNotExistException", awserr.ErrNotFound)
	// ErrPullRequestNotFound is returned when a pull request is not found.
	ErrPullRequestNotFound = awserr.New("PullRequestDoesNotExistException", awserr.ErrNotFound)
	// ErrPullRequestAlreadyMerged is returned when a PR is already merged.
	ErrPullRequestAlreadyMerged = awserr.New("PullRequestAlreadyClosedException", awserr.ErrConflict)
	// ErrInvalidRepositoryName is returned when a repository name is invalid.
	ErrInvalidRepositoryName = awserr.New("InvalidRepositoryNameException", awserr.ErrInvalidParameter)
	// ErrMaxRepositoriesExceeded is returned when too many repositories are requested.
	ErrMaxRepositoriesExceeded = awserr.New("MaximumRepositoryNamesExceededException", awserr.ErrInvalidParameter)
	// ErrBranchNameRequired is returned when a branch name is missing.
	ErrBranchNameRequired = awserr.New("BranchNameRequiredException", awserr.ErrInvalidParameter)
	// ErrInvalidBranchName is returned when a branch name contains invalid characters.
	ErrInvalidBranchName = awserr.New("InvalidBranchNameException", awserr.ErrInvalidParameter)
	// ErrParentCommitIDRequired is returned when parentCommitId is missing for a branch with commits.
	ErrParentCommitIDRequired = awserr.New("ParentCommitIdRequiredException", awserr.ErrInvalidParameter)
	// ErrParentCommitIDOutdated is returned when parentCommitId doesn't match branch tip.
	ErrParentCommitIDOutdated = awserr.New("ParentCommitIdOutdatedException", awserr.ErrConflict)
	// ErrSameFileContent is returned by PutFile when the written content is
	// identical to what's already at that path.
	ErrSameFileContent = awserr.New("SameFileContentException", awserr.ErrConflict)
	// ErrNoChange is returned by CreateCommit when a putFiles entry's content
	// is identical to what's already at that path: CreateCommit's own
	// declared error set has no SameFileContentException (that's PutFile-only,
	// verified against codecommit@v1.36.4's deserializeOpErrorCreateCommit),
	// its closest declared equivalent is NoChangeException ("no changes will
	// be made to the repository as a result of this commit").
	ErrNoChange = awserr.New("NoChangeException", awserr.ErrConflict)
	// ErrFilePathConflicts is returned when a file path conflicts with an existing path.
	ErrFilePathConflicts = awserr.New("FilePathConflictsWithSubmodulePathException", awserr.ErrConflict)
	// ErrFileNotFound is returned when a file path does not exist in the repository.
	ErrFileNotFound = awserr.New("FileDoesNotExistException", awserr.ErrNotFound)
	// ErrBlobNotFound is returned when a blob ID does not exist in the repository.
	ErrBlobNotFound = awserr.New("BlobIdDoesNotExistException", awserr.ErrNotFound)
	// ErrCommentNotFound is returned when a comment ID does not exist.
	ErrCommentNotFound = awserr.New("CommentDoesNotExistException", awserr.ErrNotFound)
	// ErrApprovalRuleNotFound is returned when a pull request approval rule does not exist.
	ErrApprovalRuleNotFound = awserr.New("ApprovalRuleDoesNotExistException", awserr.ErrNotFound)
	// ErrInvalidPullRequestEventType is returned when pullRequestEventType is not a recognized enum value.
	ErrInvalidPullRequestEventType = awserr.New("InvalidPullRequestEventTypeException", awserr.ErrInvalidParameter)
	// ErrInvalidMergeOption is returned when mergeOption is not one of the three enum values.
	// Code is InvalidMergeOptionException -- BatchDescribeMergeConflicts, CreateUnreferencedMergeCommit,
	// DescribeMergeConflicts, and GetMergeConflicts all declare it (codecommit@v1.36.4
	// deserializers.go); none of the four declares InvalidParameterException.
	ErrInvalidMergeOption = awserr.New("InvalidMergeOptionException", awserr.ErrInvalidParameter)
	// ErrInvalidPullRequestStatus is returned when pullRequestStatus is not OPEN or CLOSED.
	// Code is InvalidPullRequestStatusException -- ListPullRequests and UpdatePullRequestStatus
	// both declare it (codecommit@v1.36.4 deserializers.go); neither declares
	// InvalidParameterException.
	ErrInvalidPullRequestStatus = awserr.New("InvalidPullRequestStatusException", awserr.ErrInvalidParameter)
	// ErrInvalidContinuationToken is returned when a nextToken/NextToken fails to decode.
	// Code is InvalidContinuationTokenException -- GetDifferences and ListFileCommitHistory
	// both declare it (codecommit@v1.36.4 deserializers.go); neither declares
	// InvalidParameterException.
	ErrInvalidContinuationToken = awserr.New("InvalidContinuationTokenException", awserr.ErrInvalidParameter)
	// ErrInvalidActorArn is returned when DescribePullRequestEventsInput.ActorArn is
	// not a well-formed ARN. Code is InvalidActorArnException, declared by
	// DescribePullRequestEvents specifically (codecommit@v1.36.4 deserializers.go);
	// real AWS also declares ActorDoesNotExistException for a well-formed ARN that
	// doesn't correspond to any account principal -- not implemented here (would
	// require cross-service coupling to IAM's user/role store, out of scope).
	ErrInvalidActorArn = awserr.New("InvalidActorArnException", awserr.ErrInvalidParameter)
)

// repoNameRe matches valid CodeCommit repository names: alphanumeric, _, -, .
var repoNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// branchNameRe matches valid CodeCommit branch names.
// Branch names may contain alphanumeric characters, slashes, dashes, underscores, and dots.
// They may not begin or end with a slash, and may not contain consecutive slashes.
var branchNameRe = regexp.MustCompile(`^[a-zA-Z0-9._\-/]+$`)

// actorArnRe matches the general ARN shape (arn:partition:service:region:account-id:resource).
var actorArnRe = regexp.MustCompile(
	`^arn:(aws|aws-cn|aws-us-gov|aws-iso|aws-iso-b):[a-zA-Z0-9-]+:[a-zA-Z0-9-]*:\d*:.+$`,
)

// validateActorArn returns ErrInvalidActorArn if arn is non-empty and not a
// well-formed ARN. An empty arn (the filter omitted) is valid -- absence, not shape.
func validateActorArn(arn string) error {
	if arn == "" {
		return nil
	}
	if !actorArnRe.MatchString(arn) {
		return fmt.Errorf("%w: actorArn %q is not a valid ARN", ErrInvalidActorArn, arn)
	}

	return nil
}

// validateBranchName returns an error if the branch name is empty or contains invalid characters.
func validateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: branch name is required", ErrBranchNameRequired)
	}
	if len(name) > maxBranchNameLength {
		return fmt.Errorf("%w: branch name must be 256 characters or fewer", ErrInvalidBranchName)
	}
	if !branchNameRe.MatchString(name) {
		return fmt.Errorf("%w: branch name contains invalid characters", ErrInvalidBranchName)
	}
	// No leading/trailing slash; no consecutive slashes
	if name[0] == '/' || name[len(name)-1] == '/' {
		return fmt.Errorf("%w: branch name may not begin or end with a slash", ErrInvalidBranchName)
	}
	if strings.Contains(name, "//") {
		return fmt.Errorf("%w: branch name may not contain consecutive slashes", ErrInvalidBranchName)
	}

	return nil
}

// ValidateRepositoryName returns an error if name is not a valid CodeCommit repository name.
func ValidateRepositoryName(name string) error {
	if len(name) == 0 || len(name) > 100 {
		return fmt.Errorf("%w: repository name must be between 1 and 100 characters", ErrInvalidRepositoryName)
	}
	if !repoNameRe.MatchString(name) {
		return fmt.Errorf(
			"%w: repository name may only contain alphanumeric characters, underscores, hyphens, and periods",
			ErrInvalidRepositoryName,
		)
	}

	return nil
}
