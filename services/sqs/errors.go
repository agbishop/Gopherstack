package sqs

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

// Sentinel errors for SQS operations.
var (
	ErrQueueNotFound            = awserr.New("AWS.SimpleQueueService.NonExistentQueue", awserr.ErrNotFound)
	ErrQueueAlreadyExists       = awserr.New("QueueAlreadyExists", awserr.ErrAlreadyExists)
	ErrInvalidAttribute         = errors.New("InvalidAttributeValue")
	ErrInvalidBatchEntry        = errors.New("AWS.SimpleQueueService.EmptyBatchRequest")
	ErrReceiptHandleInvalid     = errors.New("ReceiptHandleIsInvalid")
	ErrMessageNotInflight       = errors.New("MessageNotInflight")
	ErrTooManyEntriesInBatch    = errors.New("AWS.SimpleQueueService.TooManyEntriesInBatchRequest")
	ErrBatchEntryIDsNotDistinct = errors.New("AWS.SimpleQueueService.BatchEntryIdsNotDistinct")
	ErrUnknownAction            = errors.New("InvalidAction")
	ErrMessageTooLarge          = errors.New("MessageTooLarge")
	ErrInvalidWaitTime          = errors.New("InvalidParameterValue")
	ErrInvalidVisibilityTimeout = errors.New("InvalidParameterValue.VisibilityTimeout")
	ErrMissingMessageGroupID    = errors.New("InvalidParameterValue.MissingMessageGroupID")
	ErrMissingDeduplicationID   = errors.New("InvalidParameterValue.MissingDeduplicationID")
	ErrTaskHandleInvalid        = errors.New("InvalidParameterValue.TaskHandle")
	ErrInvalidPermissionLabel   = errors.New("InvalidParameterValue.PermissionLabel")
	// ErrMoveTaskAlreadyRunning maps to the errTypeInvalidParameterValue wire code
	// (see handler.go's sqsPermErrorDetails): neither StartMessageMoveTask nor
	// CancelMessageMoveTask's own deserializeOpError recognizes a Conflict-named
	// exception -- sqs@v1.46.4 types/errors.go has none at all.
	ErrMoveTaskAlreadyRunning = errors.New("InvalidParameterValue.MoveTaskAlreadyRunning")
	// ErrMoveTaskNotRunning is returned by CancelMessageMoveTask when the referenced
	// task exists but is not in RUNNING or CANCELLING status.
	ErrMoveTaskNotRunning = errors.New("InvalidParameterValue.MoveTaskNotRunning")
	// ErrInvalidPermissionActions is returned by AddPermission when Actions is empty.
	ErrInvalidPermissionActions = errors.New("InvalidParameterValue.PermissionActions")
	// ErrInvalidPermissionAccountIDs is returned by AddPermission when AWSAccountIDs is empty.
	ErrInvalidPermissionAccountIDs = errors.New("InvalidParameterValue.PermissionAccountIDs")
	// ErrInvalidSourceArn is returned by StartMessageMoveTask when SourceArn is empty or invalid.
	ErrInvalidSourceArn = errors.New("InvalidParameterValue.SourceArn")
	// ErrInvalidMaxMessagesPerSecond is returned by StartMessageMoveTask when
	// MaxNumberOfMessagesPerSecond is negative.
	ErrInvalidMaxMessagesPerSecond = errors.New("InvalidParameterValue.MaxNumberOfMessagesPerSecond")
	// ErrInvalidDelaySeconds is returned by SendMessage when DelaySeconds is out of range (0-900).
	ErrInvalidDelaySeconds = errors.New("InvalidParameterValue.DelaySeconds")
	// ErrInvalidQueueName is returned when a queue name does not conform to AWS naming rules.
	ErrInvalidQueueName = errors.New("InvalidParameterValue.QueueName")
	// ErrInvalidMessageBody is returned when the message body is empty.
	ErrInvalidMessageBody = errors.New("InvalidParameterValue.MessageBody")
	// ErrInvalidMaxMessages is returned when MaxNumberOfMessages is outside [1, 10].
	ErrInvalidMaxMessages = errors.New("InvalidParameterValue.MaxNumberOfMessages")
	// ErrPurgeQueueInProgress is returned when PurgeQueue is called within 60s of a previous purge.
	ErrPurgeQueueInProgress = errors.New("AWS.SimpleQueueService.PurgeQueueInProgress")
	// ErrOverLimit is returned when an operation would exceed an AWS-imposed quota
	// (e.g. too many in-flight messages, too many permissions, too many queues).
	ErrOverLimit = errors.New("OverLimit")
	// ErrBatchRequestTooLong is returned when SendMessageBatch's combined payload
	// (bodies + attribute names/types/values) exceeds the per-batch byte limit
	// (matches the per-queue MaximumMessageSize, default 256 KiB).
	ErrBatchRequestTooLong = errors.New("AWS.SimpleQueueService.BatchRequestTooLong")
	// ErrInvalidMessageAttributeValue is returned when a message attribute has an
	// invalid DataType or its value does not match the declared type.
	ErrInvalidMessageAttributeValue = errors.New("InvalidParameterValue.MessageAttribute")
	// ErrInvalidAttributeName is returned when SetQueueAttributes attempts to
	// change an immutable attribute such as FifoQueue.
	ErrInvalidAttributeName = errors.New("InvalidAttributeName")
	// ErrFIFODelayNotSupported is returned when a SendMessage or batch entry for
	// a FIFO queue specifies a non-zero DelaySeconds (FIFO queues do not support
	// per-message delays).
	ErrFIFODelayNotSupported = errors.New("InvalidParameterValue.FIFODelaySeconds")
	// ErrQueueDeletedRecently is returned by CreateQueue when a queue with the
	// same name (in the same region) was deleted less than 60 seconds ago,
	// matching aws-sdk-go-v2/service/sqs/types.QueueDeletedRecently: you must
	// wait 60 seconds after deleting a queue before creating another with the
	// same name.
	ErrQueueDeletedRecently = errors.New("AWS.SimpleQueueService.QueueDeletedRecently")
)

// InvalidParameterError represents an InvalidParameterValue error with a dynamic message.
type InvalidParameterError struct {
	Message string
}

func (e *InvalidParameterError) Error() string { return e.Message }
