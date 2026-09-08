package cloudwatchlogs

import "errors"

var (
	ErrLogGroupNotFound              = errors.New("ResourceNotFoundException")
	ErrLogGroupAlreadyExists         = errors.New("ResourceAlreadyExistsException")
	ErrLogStreamNotFound             = errors.New("ResourceNotFoundException")
	ErrLogStreamAlreadyExist         = errors.New("ResourceAlreadyExistsException")
	ErrSubscriptionFilterNotFound    = errors.New("ResourceNotFoundException")
	ErrSubscriptionFilterLimitExceed = errors.New("LimitExceededException")
	ErrQueryNotFound                 = errors.New("ResourceNotFoundException")
	ErrExportTaskNotFound            = errors.New("ResourceNotFoundException")
	ErrImportTaskNotFound            = errors.New("ResourceNotFoundException")
	ErrValidation                    = errors.New("InvalidParameterException")
	// ErrValidationException is returned for the operations whose own
	// awsAwsjson11_deserializeOpError<Op> switch declares ValidationException
	// rather than InvalidParameterException as its client-error code -- this
	// is a whole family, not a handful: ListAggregateLogGroupSummaries plus
	// every Delivery/DeliveryDestination/DeliverySource/ScheduledQuery/
	// S3TableIntegration operation (confirmed per-op against
	// aws-sdk-go-v2/service/cloudwatchlogs@v1.81.1/deserializers.go). Older
	// ops in this service declare InvalidParameterException instead, which is
	// what ErrValidation above maps to.
	ErrValidationException = errors.New("ValidationException")
	// ErrScheduledQueryLimitExceeded is CreateScheduledQuery's own quota-exceeded
	// case; its deserializer declares ServiceQuotaExceededException, not
	// ValidationException or InvalidParameterException.
	ErrScheduledQueryLimitExceeded = errors.New("ServiceQuotaExceededException")
	ErrDeliveryNotFound            = errors.New("ResourceNotFoundException")
	ErrLogAnomalyDetectorNotFound  = errors.New("ResourceNotFoundException")
	ErrScheduledQueryNotFound      = errors.New("ResourceNotFoundException")
	ErrMetricFilterNotFound        = errors.New("ResourceNotFoundException")
	ErrQueryDefinitionNotFound     = errors.New("ResourceNotFoundException")
	ErrOperationAborted            = errors.New("OperationAbortedException")
	ErrInvalidOperation            = errors.New("InvalidOperationException")
)

var (
	ErrResourcePolicyNotFound      = errors.New("ResourceNotFoundException")
	ErrDeliveryDestinationNotFound = errors.New("ResourceNotFoundException")
	ErrDeliverySourceNotFound      = errors.New("ResourceNotFoundException")
	ErrDestinationNotFound         = errors.New("ResourceNotFoundException")
	ErrIndexPolicyNotFound         = errors.New("ResourceNotFoundException")
	ErrTransformerNotFound         = errors.New("ResourceNotFoundException")
	ErrIntegrationNotFound         = errors.New("ResourceNotFoundException")
	ErrS3TableIntegrationNotFound  = errors.New("ResourceNotFoundException")
	// ErrDeliveryDestinationInUse and ErrDeliverySourceInUse are returned by
	// DeleteDeliveryDestination/DeleteDeliverySource when a Delivery still
	// references them -- both ops' own deserializeOpError declares
	// ConflictException (aws-sdk-go-v2/service/cloudwatchlogs@v1.81.1
	// deserializers.go).
	ErrDeliveryDestinationInUse = errors.New("ConflictException")
	ErrDeliverySourceInUse      = errors.New("ConflictException")
)

var (
	ErrLookupTableNotFound         = errors.New("ResourceNotFoundException")
	ErrLookupTableAlreadyExists    = errors.New("ResourceAlreadyExistsException")
	ErrSyslogConfigurationNotFound = errors.New("ResourceNotFoundException")
)
