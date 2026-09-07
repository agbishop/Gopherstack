package accessanalyzer

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrAnalyzerNotFound is returned when the named analyzer does not exist.
	ErrAnalyzerNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrAnalyzerAlreadyExists is returned when creating a duplicate analyzer.
	ErrAnalyzerAlreadyExists = awserr.New("ConflictException", awserr.ErrConflict)
	// ErrArchiveRuleNotFound is returned when the named archive rule does not exist.
	ErrArchiveRuleNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrArchiveRuleAlreadyExists is returned when creating a duplicate archive rule.
	ErrArchiveRuleAlreadyExists = awserr.New("ConflictException", awserr.ErrConflict)
	// ErrFindingNotFound is returned when a finding ID is not found.
	ErrFindingNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrValidation is returned on invalid input.
	ErrValidation = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrMalformedPolicy is returned when a policyDocument is not valid JSON.
	ErrMalformedPolicy = awserr.New("UnprocessableEntityException", awserr.ErrInvalidParameter)
)

// ErrPolicyGenerationNotFound is returned when a policy generation job is not found.
var ErrPolicyGenerationNotFound = newNotFoundErr("PolicyGenerationNotFound")

// ErrAccessPreviewNotFound is returned when an access preview is not found.
var ErrAccessPreviewNotFound = newNotFoundErr("AccessPreviewNotFound")

// ErrAnalyzedResourceNotFound is returned when an analyzed resource is not found.
var ErrAnalyzedResourceNotFound = newNotFoundErr("AnalyzedResourceNotFound")

func newNotFoundErr(msg string) error {
	return &notFoundErr{msg: msg}
}

type notFoundErr struct{ msg string } //nolint:errname // existing issue.

func (e *notFoundErr) Error() string { return e.msg }
