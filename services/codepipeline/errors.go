package codepipeline

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrNotFound is returned when a pipeline resource does not exist.
	ErrNotFound = awserr.New("PipelineNotFoundException", awserr.ErrNotFound)
	// ErrPipelineNameInUse is returned when a pipeline with the same name already exists.
	ErrPipelineNameInUse = awserr.New("PipelineNameInUseException", awserr.ErrAlreadyExists)
	// ErrAlreadyExists is returned when a non-pipeline resource with the same key already exists.
	ErrAlreadyExists = awserr.New("InvalidStructureException", awserr.ErrAlreadyExists)
	// ErrActionTypeNotFound is returned when a requested custom action type does not exist.
	ErrActionTypeNotFound = awserr.New("ActionTypeNotFoundException", awserr.ErrNotFound)
	// ErrJobNotFound is returned when a requested job does not exist.
	ErrJobNotFound = awserr.New("JobNotFoundException", awserr.ErrNotFound)
	// ErrWebhookNotFound is returned when a requested webhook does not exist.
	ErrWebhookNotFound = awserr.New("WebhookNotFoundException", awserr.ErrNotFound)
	// ErrValidation is returned when request input fails validation.
	ErrValidation = awserr.New("ValidationException", awserr.ErrInvalidParameter)
	// ErrConflict is returned on optimistic-concurrency version mismatch.
	ErrConflict = awserr.New("ConflictException", awserr.ErrConflict)
	// ErrResourceInUse is returned when a resource is referenced by another resource.
	ErrResourceInUse = awserr.New("ResourceInUseException", awserr.ErrAlreadyExists)
	// ErrResourceNotFound is returned for non-pipeline ARNs (e.g. webhook ARNs).
	ErrResourceNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrStageNotFound is returned when a stage name does not exist in a pipeline.
	ErrStageNotFound = awserr.New("StageNotFoundException", awserr.ErrNotFound)
	// ErrInvalidStructure is returned for structural pipeline validation errors.
	ErrInvalidStructure = awserr.New("InvalidStructureException", awserr.ErrInvalidParameter)
	// ErrExecutionNotFound is returned when a requested pipeline execution ID does not exist.
	ErrExecutionNotFound = awserr.New("PipelineExecutionNotFoundException", awserr.ErrNotFound)
	// ErrVersionNotFound is returned when a requested pipeline version does not exist.
	ErrVersionNotFound = awserr.New("PipelineVersionNotFoundException", awserr.ErrNotFound)
	// ErrActionNotFound is returned when a stage/action name does not exist,
	// or (for PutApprovalResult) does not identify an Approval-category action.
	ErrActionNotFound = awserr.New("ActionNotFoundException", awserr.ErrNotFound)
	// ErrInvalidApprovalToken is returned when PutApprovalResult's token does
	// not match the pending approval request's system-generated token.
	ErrInvalidApprovalToken = awserr.New("InvalidApprovalTokenException", awserr.ErrInvalidParameter)
	// ErrApprovalAlreadyCompleted is returned when PutApprovalResult targets
	// an action with no open (InProgress) approval request.
	ErrApprovalAlreadyCompleted = awserr.New("ApprovalAlreadyCompletedException", awserr.ErrConflict)
	// ErrStageNotRetryable is returned when RetryStageExecution targets a
	// stage/execution pair with no failed action to retry.
	ErrStageNotRetryable = awserr.New("StageNotRetryableException", awserr.ErrInvalidParameter)
	// ErrUnableToRollbackStage is returned when RollbackStage's target
	// execution never completed the given stage successfully.
	ErrUnableToRollbackStage = awserr.New("UnableToRollbackStageException", awserr.ErrInvalidParameter)
	// ErrActionExecutionNotFound is returned when ListDeployActionExecutionTargets'
	// ActionExecutionId does not match any recorded action execution.
	ErrActionExecutionNotFound = awserr.New("ActionExecutionNotFoundException", awserr.ErrNotFound)
	// ErrInvalidClientToken is returned when a third-party job operation's
	// clientToken does not match the ClientId issued for that job by
	// PollForThirdPartyJobs.
	ErrInvalidClientToken = awserr.New("InvalidClientTokenException", awserr.ErrInvalidParameter)
)
