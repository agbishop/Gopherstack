package macie2

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrNotEnabled is returned when Macie is not enabled.
	ErrNotEnabled = awserr.New(errMacieNotEnabled, awserr.ErrNotFound)
	// ErrAllowListNotFound is returned when an allow list does not exist.
	ErrAllowListNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrSessionAlreadyExists is returned when Macie is already enabled.
	ErrSessionAlreadyExists = awserr.New(errConflictException, awserr.ErrConflict)
	// ErrAllowListAlreadyExists is returned when an allow list already exists.
	ErrAllowListAlreadyExists = awserr.New(errConflictException, awserr.ErrConflict)
	// ErrCustomDataIDNotFound is returned when a custom data identifier does not exist.
	ErrCustomDataIDNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrFindingsFilterNotFound is returned when a findings filter does not exist.
	ErrFindingsFilterNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrFindingNotFound is returned when a finding does not exist.
	ErrFindingNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrTaggedResourceNotFound is returned when a tag operation targets an unknown resource ARN.
	ErrTaggedResourceNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrValidation is returned on invalid input.
	ErrValidation = awserr.New(errValidation, awserr.ErrInvalidParameter)
)

var (
	// ErrClassificationJobNotFound is returned when a classification job does not exist.
	ErrClassificationJobNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrMemberNotFound is returned when a member does not exist.
	ErrMemberNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrMemberAlreadyExists is returned when a member already exists.
	ErrMemberAlreadyExists = awserr.New(errConflictException, awserr.ErrConflict)
	// ErrInvitationNotFound is returned when an invitation does not exist.
	ErrInvitationNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrOrgAdminNotFound is returned when an org admin account does not exist.
	ErrOrgAdminNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrClassificationScopeNotFound is returned when a classification scope does not exist.
	ErrClassificationScopeNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrSensitivityTemplateNotFound is returned when a sensitivity inspection template does not exist.
	ErrSensitivityTemplateNotFound = awserr.New(errResourceNotFound, awserr.ErrNotFound)
	// ErrJobStatusTransition is returned when UpdateClassificationJob's requested
	// JobStatus isn't valid for the job's current status (api_op_UpdateClassificationJob.go).
	ErrJobStatusTransition = awserr.New(errConflictException, awserr.ErrConflict)
)
