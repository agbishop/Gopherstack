package organizations

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

// Sentinel errors.
var (
	// ErrOrgNotFound is returned when no organization exists.
	ErrOrgNotFound = awserr.New(
		"AWSOrganizationsNotInUseException: organization not found",
		awserr.ErrNotFound,
	)
	// ErrOrgAlreadyExists is returned when an organization already exists.
	ErrOrgAlreadyExists = awserr.New(
		"AlreadyInOrganizationException: account is already a member of an organization",
		awserr.ErrAlreadyExists,
	)
	// ErrAccountNotFound is returned when an account does not exist.
	ErrAccountNotFound = awserr.New(
		"AccountNotFoundException: account not found",
		awserr.ErrNotFound,
	)
	// ErrOUNotFound is returned when an OU does not exist.
	ErrOUNotFound = awserr.New(
		"OrganizationalUnitNotFoundException: OU not found",
		awserr.ErrNotFound,
	)
	// ErrPolicyNotFound is returned when a policy does not exist.
	ErrPolicyNotFound = awserr.New("PolicyNotFoundException: policy not found", awserr.ErrNotFound)
	// ErrPolicyTypeAlreadyEnabled is returned when a policy type is already enabled.
	ErrPolicyTypeAlreadyEnabled = awserr.New(
		"PolicyTypeAlreadyEnabledException: policy type already enabled",
		awserr.ErrConflict,
	)
	// ErrPolicyTypeNotEnabled is returned when a policy type is not enabled.
	ErrPolicyTypeNotEnabled = awserr.New(
		"PolicyTypeNotEnabledException: policy type not enabled",
		awserr.ErrConflict,
	)
	// ErrCreateAccountStatusNotFound is returned when a create-account status is not found.
	ErrCreateAccountStatusNotFound = awserr.New(
		"CreateAccountStatusNotFoundException: create account status not found",
		awserr.ErrNotFound,
	)
	// ErrDuplicatePolicyAttachment is returned when a policy is already attached.
	ErrDuplicatePolicyAttachment = awserr.New(
		"DuplicatePolicyAttachmentException: policy already attached",
		awserr.ErrConflict,
	)
	// ErrPolicyNotAttached is returned when a policy is not attached to the target.
	ErrPolicyNotAttached = awserr.New(
		"PolicyNotAttachedException: policy not attached to target",
		awserr.ErrNotFound,
	)
	// ErrInvalidInput is returned for invalid input parameters.
	ErrInvalidInput = awserr.New("InvalidInputException: invalid input", awserr.ErrInvalidParameter)
	// ErrChildNotFound is returned when a child resource is not found.
	ErrChildNotFound = awserr.New("ChildNotFoundException: child not found", awserr.ErrNotFound)
	// ErrDelegatedAdminNotFound is returned when a delegated admin is not found.
	ErrDelegatedAdminNotFound = awserr.New(
		"AccountNotRegisteredException: account is not a registered delegated administrator",
		awserr.ErrNotFound,
	)
	// ErrDelegatedAdminAlreadyExists is returned when a delegated admin already exists.
	ErrDelegatedAdminAlreadyExists = awserr.New(
		"AccountAlreadyRegisteredException: account is already a delegated administrator",
		awserr.ErrAlreadyExists,
	)
	// ErrPolicyLimitExceeded is returned when the maximum number of policies per target is exceeded.
	ErrPolicyLimitExceeded = awserr.New(
		"ConstraintViolationException: maximum policies per target exceeded",
		awserr.ErrConflict,
	)
	// ErrHandshakeNotFound is returned when a handshake does not exist.
	ErrHandshakeNotFound = awserr.New(
		"HandshakeNotFoundException: handshake not found",
		awserr.ErrNotFound,
	)
	// ErrHandshakeConstraintViolation is returned when a handshake state transition is invalid.
	ErrHandshakeConstraintViolation = awserr.New(
		"HandshakeConstraintViolationException: handshake is not in a valid state for this transition",
		awserr.ErrConflict,
	)
	// ErrResourcePolicyNotFound is returned when no resource policy exists.
	ErrResourcePolicyNotFound = awserr.New(
		"ResourcePolicyNotFoundException: resource policy not found",
		awserr.ErrNotFound,
	)
	// ErrEffectivePolicyNotFound is returned when no effective policy exists for a target.
	ErrEffectivePolicyNotFound = awserr.New(
		"EffectivePolicyNotFoundException: no effective policy of the given type",
		awserr.ErrNotFound,
	)
	// ErrAccountAlreadyClosed is returned when an account is already in PENDING_CLOSURE or SUSPENDED state.
	ErrAccountAlreadyClosed = awserr.New(
		"ConstraintViolationException: account is already closed or pending closure",
		awserr.ErrConflict,
	)
	// ErrOUDepthLimitExceeded is returned when creating an OU would exceed the depth limit.
	ErrOUDepthLimitExceeded = awserr.New(
		"ConstraintViolationException: OU_DEPTH_LIMIT_EXCEEDED",
		awserr.ErrConflict,
	)
	// ErrDuplicateOrganizationalUnit is returned when an OU with the same name already exists under the same parent.
	ErrDuplicateOrganizationalUnit = awserr.New(
		"DuplicateOrganizationalUnitException: duplicate OU name",
		awserr.ErrAlreadyExists,
	)
	// ErrTargetNotFound is returned when a policy attachment target is not found.
	ErrTargetNotFound = awserr.New(
		"TargetNotFoundException: target not found",
		awserr.ErrNotFound,
	)
	// ErrServiceNotEnabled is returned when registering a delegated admin for a service not enabled for org access.
	ErrServiceNotEnabled = awserr.New(
		"ConstraintViolationException: service principal not enabled for service access",
		awserr.ErrConflict,
	)
	// ErrPolicyInUse is returned when attempting to delete a policy still attached to targets.
	ErrPolicyInUse = awserr.New(
		"PolicyInUseException: policy is still attached to one or more targets",
		awserr.ErrConflict,
	)
	// ErrOrganizationNotEmpty is returned when attempting to delete an org that still has member accounts.
	ErrOrganizationNotEmpty = awserr.New(
		"OrganizationNotEmptyException: organization still contains member accounts",
		awserr.ErrConflict,
	)
	// ErrDuplicateHandshake is returned when an open handshake already exists for the same target.
	ErrDuplicateHandshake = awserr.New(
		"DuplicateHandshakeException: a handshake already exists for the specified account",
		awserr.ErrAlreadyExists,
	)
	// ErrPolicyTypeAttached is returned when disabling a policy type that still has attached policies.
	ErrPolicyTypeAttached = awserr.New(
		"ConstraintViolationException: cannot disable policy type while policies of that type are attached",
		awserr.ErrConflict,
	)
	// ErrMalformedPolicyDocument is returned when a policy's Content is not valid JSON.
	ErrMalformedPolicyDocument = awserr.New(
		"MalformedPolicyDocumentException: policy content is not valid JSON",
		awserr.ErrInvalidParameter,
	)
	// ErrPolicyContentLimitExceeded is returned when a policy's Content exceeds the
	// maximum document size for its policy type.
	ErrPolicyContentLimitExceeded = awserr.New(
		"ConstraintViolationException: POLICY_CONTENT_LIMIT_EXCEEDED",
		awserr.ErrConflict,
	)
	// ErrTagLimitExceeded is returned when tagging a resource would exceed the
	// maximum of 50 tags per resource.
	ErrTagLimitExceeded = awserr.New(
		"ConstraintViolationException: MAX_TAG_LIMIT_EXCEEDED",
		awserr.ErrConflict,
	)
	// ErrInvalidSystemTags is returned when a caller-supplied tag key uses the
	// "aws:" prefix reserved for system tags.
	ErrInvalidSystemTags = awserr.New(
		"InvalidInputException: INVALID_SYSTEM_TAGS_PARAMETER",
		awserr.ErrInvalidParameter,
	)
	// ErrDuplicateTagKey is returned when the same tag key appears more than once
	// in a single request's tag list.
	ErrDuplicateTagKey = awserr.New(
		"InvalidInputException: DUPLICATE_TAG_KEY",
		awserr.ErrInvalidParameter,
	)
	// ErrInvalidTagKeyLength is returned when a tag key is empty or exceeds 128
	// characters (TagKey shape: min 1, max 128).
	ErrInvalidTagKeyLength = awserr.New(
		"InvalidInputException: tag key must be between 1 and 128 characters",
		awserr.ErrInvalidParameter,
	)
	// ErrInvalidTagValueLength is returned when a tag value exceeds 256
	// characters (TagValue shape: max 256).
	ErrInvalidTagValueLength = awserr.New(
		"InvalidInputException: tag value must not exceed 256 characters",
		awserr.ErrInvalidParameter,
	)
	// ErrResponsibilityTransferNotFound is returned when a responsibility
	// transfer does not exist (looked up by its own rt-... Id, not a
	// handshake Id).
	ErrResponsibilityTransferNotFound = awserr.New(
		"ResponsibilityTransferNotFoundException: responsibility transfer not found",
		awserr.ErrNotFound,
	)
	// ErrInvalidResponsibilityTransferTransition is returned by
	// TerminateResponsibilityTransfer when the transfer has never reached
	// ACCEPTED status -- declared on that op's own deserializeOpError switch
	// (deserializers.go), distinct from ErrResponsibilityTransferAlreadyInStatus.
	ErrInvalidResponsibilityTransferTransition = awserr.New(
		"InvalidResponsibilityTransferTransitionException: transfer is not in a state that can be terminated",
		awserr.ErrConflict,
	)
	// ErrResponsibilityTransferAlreadyInStatus is returned by
	// TerminateResponsibilityTransfer when the transfer already has an
	// EndTimestamp (already terminated).
	ErrResponsibilityTransferAlreadyInStatus = awserr.New(
		"ResponsibilityTransferAlreadyInStatusException: transfer has already ended",
		awserr.ErrConflict,
	)
	// ErrOrganizationalUnitNotEmpty is returned by DeleteOrganizationalUnit
	// when the OU still contains accounts or child OUs (own error code, not
	// InvalidInputException -- see DeleteOrganizationalUnit's per-op error
	// switch in deserializers.go).
	ErrOrganizationalUnitNotEmpty = awserr.New(
		"OrganizationalUnitNotEmptyException: organizational unit is not empty",
		awserr.ErrConflict,
	)
	// ErrMasterCannotLeaveOrganization is returned by LeaveOrganization and
	// RemoveAccountFromOrganization when called against the management
	// account (own error code, not InvalidInputException -- see those ops'
	// per-op error switches in deserializers.go).
	ErrMasterCannotLeaveOrganization = awserr.New(
		"MasterCannotLeaveOrganizationException: "+
			"the management account cannot leave or be removed from the organization",
		awserr.ErrConflict,
	)
	// ErrSourceParentNotFound is returned by MoveAccount when SourceParentId
	// does not identify a root or OU that currently holds the account (own
	// error code, not InvalidInputException -- see MoveAccount's per-op
	// error switch in deserializers.go).
	ErrSourceParentNotFound = awserr.New(
		"SourceParentNotFoundException: source parent not found",
		awserr.ErrNotFound,
	)
	// ErrDestinationParentNotFound is returned by MoveAccount when
	// DestinationParentId does not identify an existing root or OU (own
	// error code, not InvalidInputException -- see MoveAccount's per-op
	// error switch in deserializers.go).
	ErrDestinationParentNotFound = awserr.New(
		"DestinationParentNotFoundException: destination parent not found",
		awserr.ErrNotFound,
	)
	// ErrCannotRemoveDelegatedAdministratorFromOrg is returned by
	// RemoveAccountFromOrganization when the target account is still a
	// registered delegated administrator for some service (types/enums.go:
	// ConstraintViolationExceptionReasonCannotRemoveDelegatedAdministratorFromOrg;
	// RemoveAccountFromOrganization's doc comment: "must not be a delegated
	// administrator account ... you must first change the delegated
	// administrator account").
	ErrCannotRemoveDelegatedAdministratorFromOrg = awserr.New(
		"ConstraintViolationException: CANNOT_REMOVE_DELEGATED_ADMINISTRATOR_FROM_ORG",
		awserr.ErrConflict,
	)
	// ErrAccessDeniedManagedPolicy is returned by DeletePolicy and UpdatePolicy
	// for an AWS-managed policy (e.g. p-FullAWSAccess). Neither op's declared
	// error set (deserializers.go) includes ConstraintViolationException, so
	// AccessDeniedException -- declared on both -- is the only fit; see
	// types.PolicySummary.AwsManaged's doc comment ("you can attach the
	// policy ... but you cannot edit it").
	ErrAccessDeniedManagedPolicy = awserr.New(
		"AccessDeniedException: you don't have permissions to modify or delete an AWS managed policy",
		awserr.ErrConflict,
	)
)

// Ensure errors are used somewhere to satisfy linter.
var _ = errors.Is(ErrOrgNotFound, awserr.ErrNotFound)
