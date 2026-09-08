package ssm

import (
	"errors"
)

var (
	ErrParameterNotFound        = errors.New("ParameterNotFound")
	ErrParameterVersionNotFound = errors.New("ParameterVersionNotFound")
	ErrParameterAlreadyExists   = errors.New("ParameterAlreadyExists")
	ErrInvalidKeyID             = errors.New("InvalidKeyId")
	ErrCiphertextTooShort       = errors.New("ciphertext too short")
	ErrValidationException      = errors.New("ValidationException")
	ErrDocumentAlreadyExists    = errors.New("DocumentAlreadyExists")
	ErrDocumentNotFound         = errors.New("DocumentNotFound")
	ErrInvalidDocumentVersion   = errors.New("InvalidDocumentVersion")
	ErrCommandNotFound          = errors.New("CommandNotFound")
	// ErrInvalidActivationID is returned when an ActivationId doesn't match any
	// known activation (DeleteActivation). "ActivationNotFound" is not a real
	// AWS SSM error code — DeleteActivation's own deserializer
	// (ssm@v1.73.4 deserializers.go) models InvalidActivationId for this case.
	ErrInvalidActivationID                = errors.New("InvalidActivationId")
	ErrAssociationNotFound                = errors.New("AssociationDoesNotExist")
	ErrMaintenanceWindowNotFound          = errors.New("DoesNotExistException")
	ErrMaintenanceWindowExecutionNotFound = errors.New("DoesNotExistException")
	ErrOpsItemNotFound                    = errors.New("OpsItemNotFoundException")
	ErrOpsMetadataNotFound                = errors.New("OpsMetadataNotFoundException")
	ErrPatchBaselineNotFound              = errors.New("DoesNotExistException")
	ErrOpsMetadataAlreadyExists           = errors.New("OpsMetadataAlreadyExistsException")
	ErrHierarchyLevelLimitExceeded        = errors.New("HierarchyLevelLimitExceededException")
	ErrParameterMaxVersionLimitExceeded   = errors.New("ParameterMaxVersionLimitExceeded")
	// ErrAccessRequestNotFound is returned when GetAccessToken is called with
	// an AccessRequestId that was never created by StartAccessRequest.
	ErrAccessRequestNotFound = errors.New("ResourceNotFoundException")
	// ErrPatchBaselineInUse is returned by DeletePatchBaseline when the
	// baseline is still registered to one or more patch groups.
	// ResourceInUseException is DeletePatchBaseline's own declared exception
	// for exactly this case (ssm@v1.73.4 deserializers.go), matching the API
	// reference doc comment: error returned if an attempt is made to delete
	// a patch baseline that is registered for a patch group.
	ErrPatchBaselineInUse = errors.New("ResourceInUseException")
)
var (
	ErrResourceDataSyncNotFound    = errors.New("ResourceDataSyncNotFoundException")
	ErrAutomationExecutionNotFound = errors.New("AutomationExecutionNotFoundException")
	ErrExecutionPreviewNotFound    = errors.New("ExecutionPreviewNotFoundException")
	// ErrResourcePolicyNotFound and ErrResourcePolicyConflict are the two real
	// exceptions declared for PutResourcePolicy/DeleteResourcePolicy
	// (ssm@v1.73.4 types/errors.go) around a PolicyId/PolicyHash mismatch.
	ErrResourcePolicyNotFound = errors.New("ResourcePolicyNotFoundException")
	ErrResourcePolicyConflict = errors.New("ResourcePolicyConflictException")
	ErrResourceDataSyncExists = errors.New("ResourceDataSyncAlreadyExistsException")
)
var (
	// ErrInventoryNotFound is returned when inventory for a type is not found.
	ErrInventoryNotFound = errors.New("InventoryTypeNotFound")
	// ErrDocumentVersionNotFound is returned when a document version is not found.
	ErrDocumentVersionNotFound = errors.New("InvalidDocumentVersion")
	// ErrInvalidAggregator is returned by ListNodesSummary when Aggregators is
	// missing or empty. InvalidAggregatorException is one of the op's own
	// declared exceptions (awsAwsjson11_deserializeOpErrorListNodesSummary,
	// ssm@v1.73.4 deserializers.go), not the generic ValidationException.
	ErrInvalidAggregator = errors.New("InvalidAggregatorException")
	// ErrDocumentStillShared is returned by DeleteDocument when the document
	// still has active AccountIdsToAdd shares. InvalidDocumentOperation is one
	// of DeleteDocument's own declared exceptions (ssm@v1.73.4
	// deserializers.go:2225-2226), whose message reads: you attempted to
	// delete a document while it is still shared, and must stop sharing it
	// first.
	ErrDocumentStillShared = errors.New("InvalidDocumentOperation")
	// ErrInvalidResourceID is returned by the resource-tagging ops
	// (AddTagsToResource/RemoveTagsFromResource/ListTagsForResource) when the
	// target resource doesn't exist. Their own deserializers model
	// InvalidResourceId for this, not the per-resource NotFound sentinel
	// (e.g. ErrParameterNotFound) that GetParameter/PutParameter use.
	ErrInvalidResourceID = errors.New("InvalidResourceId")
	// ErrParameterNamePattern is returned by PutParameter when Name fails its
	// length/character/reserved-prefix/hierarchy checks. ParameterPatternMismatchException
	// is PutParameter's own declared exception for this
	// (ssm@v1.73.4 deserializers.go:13901), not the generic ValidationException.
	ErrParameterNamePattern = errors.New("ParameterPatternMismatchException")
	// ErrUnsupportedParameterType is returned by PutParameter when Type isn't
	// String, StringList, or SecureString. UnsupportedParameterType is
	// PutParameter's own declared exception for this
	// (ssm@v1.73.4 deserializers.go:13910).
	ErrUnsupportedParameterType = errors.New("UnsupportedParameterType")
	// ErrInvalidAllowedPattern is returned by PutParameter when AllowedPattern
	// is malformed or Value doesn't match it. InvalidAllowedPatternException is
	// PutParameter's own declared exception for this
	// (ssm@v1.73.4 deserializers.go:13880).
	ErrInvalidAllowedPattern = errors.New("InvalidAllowedPatternException")
)
