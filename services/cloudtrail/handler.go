package cloudtrail

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	cloudtrailMatchPriority         = service.PriorityHeaderExact
	cloudtrailTargetPrefix          = "CloudTrail_20131101."
	keyTrailARN                     = "TrailARN"
	keyName                         = "Name"
	keyQueryID                      = "QueryId"
	keyQueryStatus                  = "QueryStatus"
	keyChannelArn                   = "ChannelArn"
	keySource                       = "Source"
	keyDestinations                 = "Destinations"
	keyDashboardArn                 = "DashboardArn"
	keyStatus                       = "Status"
	keyEDSArn                       = "EventDataStoreArn"
	keyImportID                     = "ImportId"
	keyImportStatus                 = "ImportStatus"
	keyResourceArn                  = "ResourceArn"
	keyCreatedTimestamp             = "CreatedTimestamp"
	keyUpdatedTimestamp             = "UpdatedTimestamp"
	keyInsightSelectors             = "InsightSelectors"
	keyType                         = "Type"
	keyTerminationProtectionEnabled = "TerminationProtectionEnabled"
	keyKey                          = "Key"
	keyValue                        = "Value"
	statusEnabled                   = "ENABLED"
	statusDisabled                  = "DISABLED"
)

var errInvalidRequest = errors.New("invalid request")

// Handler is the Echo HTTP handler for AWS CloudTrail operations (JSON-1.1 protocol).
type Handler struct {
	ops     map[string]func(*echo.Context, []byte) error
	Backend *InMemoryBackend
}

// NewHandler creates a new CloudTrail handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{Backend: backend}
	h.ops = h.buildOps()

	return h
}

// Name returns the service name.
func (h *Handler) Name() string { return "CloudTrail" }

// GetSupportedOperations returns the list of supported CloudTrail operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"AddTags",
		"CancelQuery",
		"CreateChannel",
		"CreateDashboard",
		"CreateEventDataStore",
		"CreateTrail",
		"DeleteChannel",
		"DeleteDashboard",
		"DeleteEventDataStore",
		"DeleteResourcePolicy",
		"DeleteTrail",
		"DeregisterOrganizationDelegatedAdmin",
		"DescribeQuery",
		"DescribeTrails",
		"DisableFederation",
		"EnableFederation",
		"GenerateQuery",
		"GetChannel",
		"GetDashboard",
		"GetEventConfiguration",
		"GetEventDataStore",
		"GetEventSelectors",
		"GetImport",
		"GetInsightSelectors",
		"GetQueryResults",
		"GetResourcePolicy",
		"GetTrail",
		"GetTrailStatus",
		"ListChannels",
		"ListDashboards",
		"ListEventDataStores",
		"ListImportFailures",
		"ListImports",
		"ListInsightsData",
		"ListInsightsMetricData",
		"ListPublicKeys",
		"ListQueries",
		"ListTags",
		"ListTrails",
		"LookupEvents",
		"PutEventConfiguration",
		"PutEventSelectors",
		"PutInsightSelectors",
		"PutResourcePolicy",
		"RegisterOrganizationDelegatedAdmin",
		"RemoveTags",
		"RestoreEventDataStore",
		"SearchSampleQueries",
		"StartDashboardRefresh",
		"StartEventDataStoreIngestion",
		"StartImport",
		"StartLogging",
		"StartQuery",
		"StopEventDataStoreIngestion",
		"StopImport",
		"StopLogging",
		"UpdateChannel",
		"UpdateDashboard",
		"UpdateEventDataStore",
		"UpdateTrail",
	}
}

// RecordManagementEvent implements service.CloudTrailRecorder, allowing the
// central service registry to reach this live backend directly (no second,
// disconnected CloudTrail backend is created).
func (h *Handler) RecordManagementEvent(ev service.CloudTrailEventInput) {
	h.Backend.RecordManagementEvent(ev)
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "cloudtrail" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this CloudTrail instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches AWS CloudTrail JSON requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, cloudtrailTargetPrefix)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return cloudtrailMatchPriority }

// ExtractOperation extracts the CloudTrail operation name from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")

	return strings.TrimPrefix(target, cloudtrailTargetPrefix)
}

// ExtractResource extracts the primary resource identifier from the request body.
func (h *Handler) ExtractResource(_ *echo.Context) string {
	return ""
}

// Handler returns the Echo handler function for CloudTrail requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		log := logger.Load(c.Request().Context())
		operation := h.ExtractOperation(c)

		log.Debug("cloudtrail request", "operation", operation)

		var body []byte
		if c.Request().Body != nil {
			decoder := json.NewDecoder(c.Request().Body)
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err == nil {
				body = raw
			}
		}

		return h.dispatch(c, operation, body)
	}
}

// Reset clears the backend state (test helper).
func (h *Handler) Reset() {
	h.Backend.Reset()
}

func (h *Handler) dispatch(c *echo.Context, operation string, body []byte) error {
	if fn, ok := h.ops[operation]; ok {
		return fn(c, body)
	}

	return c.JSON(
		http.StatusBadRequest,
		errResp("InvalidParameterCombinationException", "unknown operation: "+operation),
	)
}

func (h *Handler) buildOps() map[string]func(*echo.Context, []byte) error {
	return map[string]func(*echo.Context, []byte) error{
		"AddTags":                              h.handleAddTags,
		"CancelQuery":                          h.handleCancelQuery,
		"CreateChannel":                        h.handleCreateChannel,
		"CreateDashboard":                      h.handleCreateDashboard,
		"CreateEventDataStore":                 h.handleCreateEventDataStore,
		"CreateTrail":                          h.handleCreateTrail,
		"DeleteChannel":                        h.handleDeleteChannel,
		"DeleteDashboard":                      h.handleDeleteDashboard,
		"DeleteEventDataStore":                 h.handleDeleteEventDataStore,
		"DeleteResourcePolicy":                 h.handleDeleteResourcePolicy,
		"DeleteTrail":                          h.handleDeleteTrail,
		"DeregisterOrganizationDelegatedAdmin": h.handleDeregisterOrganizationDelegatedAdmin,
		"DescribeQuery":                        h.handleDescribeQuery,
		"DescribeTrails":                       h.handleDescribeTrails,
		"DisableFederation":                    h.handleDisableFederation,
		"EnableFederation":                     h.handleEnableFederation,
		"GenerateQuery":                        h.handleGenerateQuery,
		"GetChannel":                           h.handleGetChannel,
		"GetDashboard":                         h.handleGetDashboard,
		"GetEventConfiguration":                h.handleGetEventConfiguration,
		"GetEventDataStore":                    h.handleGetEventDataStore,
		"GetEventSelectors":                    h.handleGetEventSelectors,
		"GetImport":                            h.handleGetImport,
		"GetInsightSelectors":                  h.handleGetInsightSelectors,
		"GetQueryResults":                      h.handleGetQueryResults,
		"GetResourcePolicy":                    h.handleGetResourcePolicy,
		"GetTrail":                             h.handleGetTrail,
		"GetTrailStatus":                       h.handleGetTrailStatus,
		"ListChannels":                         h.handleListChannels,
		"ListDashboards":                       h.handleListDashboards,
		"ListEventDataStores":                  h.handleListEventDataStores,
		"ListImportFailures":                   h.handleListImportFailures,
		"ListImports":                          h.handleListImports,
		"ListInsightsData":                     h.handleListInsightsData,
		"ListInsightsMetricData":               h.handleListInsightsMetricData,
		"ListPublicKeys":                       h.handleListPublicKeys,
		"ListQueries":                          h.handleListQueries,
		"ListTags":                             h.handleListTags,
		"ListTrails":                           h.handleListTrails,
		"LookupEvents":                         h.handleLookupEvents,
		"PutEventConfiguration":                h.handlePutEventConfiguration,
		"PutEventSelectors":                    h.handlePutEventSelectors,
		"PutInsightSelectors":                  h.handlePutInsightSelectors,
		"PutResourcePolicy":                    h.handlePutResourcePolicy,
		"RegisterOrganizationDelegatedAdmin":   h.handleRegisterOrganizationDelegatedAdmin,
		"RemoveTags":                           h.handleRemoveTags,
		"RestoreEventDataStore":                h.handleRestoreEventDataStore,
		"SearchSampleQueries":                  h.handleSearchSampleQueries,
		"StartDashboardRefresh":                h.handleStartDashboardRefresh,
		"StartEventDataStoreIngestion":         h.handleStartEventDataStoreIngestion,
		"StartImport":                          h.handleStartImport,
		"StartLogging":                         h.handleStartLogging,
		"StartQuery":                           h.handleStartQuery,
		"StopEventDataStoreIngestion":          h.handleStopEventDataStoreIngestion,
		"StopImport":                           h.handleStopImport,
		"StopLogging":                          h.handleStopLogging,
		"UpdateChannel":                        h.handleUpdateChannel,
		"UpdateDashboard":                      h.handleUpdateDashboard,
		"UpdateEventDataStore":                 h.handleUpdateEventDataStore,
		"UpdateTrail":                          h.handleUpdateTrail,
	}
}

// errorMapping maps a sentinel error to the HTTP status and __type code
// this service's real deserializeOpError<Op> switches expect.
type errorMapping struct {
	sentinel error
	code     string
	status   int
}

//nolint:gochecknoglobals // package-level lookup table for error mapping
var errorMappings = []errorMapping{
	{ErrNotFound, "TrailNotFoundException", http.StatusNotFound},
	{ErrChannelNotFound, "ChannelNotFoundException", http.StatusNotFound},
	{ErrEventDataStoreNotFound, "EventDataStoreNotFoundException", http.StatusNotFound},
	{ErrResourceNotFound, "ResourceNotFoundException", http.StatusNotFound},
	{ErrImportNotFound, "ImportNotFoundException", http.StatusNotFound},
	{ErrResourcePolicyNotFound, "ResourcePolicyNotFoundException", http.StatusNotFound},
	{ErrChannelAlreadyExists, "ChannelAlreadyExistsException", http.StatusConflict},
	{ErrEventDataStoreAlreadyExists, "EventDataStoreAlreadyExistsException", http.StatusConflict},
	{ErrDashboardConflict, "ConflictException", http.StatusConflict},
	{ErrQueryIDNotFound, "QueryIdNotFoundException", http.StatusNotFound},
	{ErrQueryInactive, "InactiveQueryException", http.StatusBadRequest},
	{ErrTerminationProtected, "EventDataStoreTerminationProtectedException", http.StatusConflict},
	{ErrInsightNotEnabled, "InsightNotEnabledException", http.StatusBadRequest},
	{ErrS3BucketNotFound, "S3BucketDoesNotExistException", http.StatusBadRequest},
	{ErrAlreadyExists, "TrailAlreadyExistsException", http.StatusConflict},
	{ErrValidation, "InvalidParameterException", http.StatusBadRequest},
	{errInvalidRequest, "InvalidParameterCombinationException", http.StatusBadRequest},
}

func (h *Handler) handleError(c *echo.Context, err error) error {
	for _, m := range errorMappings {
		if errors.Is(err, m.sentinel) {
			return c.JSON(m.status, errResp(m.code, err.Error()))
		}
	}

	return c.JSON(http.StatusInternalServerError, errResp("InternalFailure", err.Error()))
}

func errResp(code, msg string) map[string]string {
	return map[string]string{"__type": code, "message": msg}
}
