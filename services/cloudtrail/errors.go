package cloudtrail

import "github.com/blackbirdworks/gopherstack/pkgs/awserr"

var (
	// ErrNotFound is returned when the requested resource does not exist.
	ErrNotFound = awserr.New("TrailNotFoundException", awserr.ErrNotFound)
	// ErrAlreadyExists is returned when a resource already exists.
	ErrAlreadyExists = awserr.New("TrailAlreadyExistsException", awserr.ErrConflict)
	// ErrValidation is returned when input validation fails.
	ErrValidation = awserr.New("InvalidParameterException", awserr.ErrInvalidParameter)
	// ErrChannelNotFound is returned when a channel is not found.
	ErrChannelNotFound = awserr.New("ChannelNotFoundException", awserr.ErrNotFound)
	// ErrEventDataStoreNotFound is returned when an event data store is not found.
	ErrEventDataStoreNotFound = awserr.New("EventDataStoreNotFoundException", awserr.ErrNotFound)
	// ErrResourceNotFound is returned when a tagged resource, or a dashboard,
	// is not found by ops whose own error model only types the generic
	// ResourceNotFoundException (AddTags/RemoveTags/DeleteDashboard/
	// GetDashboard/UpdateDashboard/StartDashboardRefresh) rather than a
	// resource-specific NotFound code.
	ErrResourceNotFound = awserr.New("ResourceNotFoundException", awserr.ErrNotFound)
	// ErrImportNotFound is returned when an import ID does not exist
	// (GetImport/StopImport).
	ErrImportNotFound = awserr.New("ImportNotFoundException", awserr.ErrNotFound)
	// ErrResourcePolicyNotFound is returned when no resource policy is
	// attached to a resource (GetResourcePolicy/DeleteResourcePolicy).
	ErrResourcePolicyNotFound = awserr.New("ResourcePolicyNotFoundException", awserr.ErrNotFound)
	// ErrChannelAlreadyExists is returned when CreateChannel targets a name
	// already in use.
	ErrChannelAlreadyExists = awserr.New("ChannelAlreadyExistsException", awserr.ErrAlreadyExists)
	// ErrEventDataStoreAlreadyExists is returned when CreateEventDataStore
	// targets a name already in use.
	ErrEventDataStoreAlreadyExists = awserr.New("EventDataStoreAlreadyExistsException", awserr.ErrAlreadyExists)
	// ErrDashboardConflict is returned when CreateDashboard targets a name
	// already in use. CreateDashboard's own error model has no
	// DashboardAlreadyExistsException; ConflictException is the closest
	// modelled code and the only one in its switch that fits (inference,
	// not verified against real AWS behavior).
	ErrDashboardConflict = awserr.New("ConflictException", awserr.ErrConflict)
	// ErrQueryIDNotFound is returned when a query ID does not exist or does not
	// map to a query (CancelQuery/DescribeQuery/GetQueryResults).
	ErrQueryIDNotFound = awserr.New("QueryIdNotFoundException", awserr.ErrNotFound)
	// ErrQueryInactive is returned when CancelQuery is called on a query that
	// is already in a terminal state (FINISHED/FAILED/TIMED_OUT/CANCELLED).
	ErrQueryInactive = awserr.New("InactiveQueryException", awserr.ErrInvalidParameter)
	// ErrTerminationProtected is returned when trying to delete a termination-protected resource.
	ErrTerminationProtected = awserr.New("EventDataStoreTerminationProtectedException", awserr.ErrConflict)
	// ErrInsightNotEnabled is returned when GetInsightSelectors is called on a trail with no
	// insight selectors configured. AWS returns InsightNotEnabledException in this case.
	ErrInsightNotEnabled = awserr.New("InsightNotEnabledException", awserr.ErrInvalidParameter)
	// ErrS3BucketNotFound is returned by CreateTrail/UpdateTrail when S3 is
	// wired (SetS3Backend) and the named S3BucketName does not exist. Real
	// CreateTrail/UpdateTrail both declare S3BucketDoesNotExistException
	// (cloudtrail@v1.58.4 deserializers.go's per-op error switches); see
	// PARITY.md for why it maps to 400 here.
	ErrS3BucketNotFound = awserr.New("S3BucketDoesNotExistException", awserr.ErrInvalidParameter)
)
