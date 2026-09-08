package cloudwatchlogs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

var errUnknownOperation = errors.New("UnknownOperationException")

// Handler is the Echo HTTP service handler for CloudWatch Logs operations.
type Handler struct {
	Backend StorageBackend
	ops     map[string]actionFn
	janitor *Janitor
	tags    map[string]*tags.Tags
	tagsMu  *lockmetrics.RWMutex
}

// NewHandler creates a new CloudWatch Logs handler.
func NewHandler(backend StorageBackend) *Handler {
	h := &Handler{
		Backend: backend,
		tags:    make(map[string]*tags.Tags),
		tagsMu:  lockmetrics.New("cwl.tags"),
	}
	h.ops = h.buildOps()

	return h
}

// WithJanitor attaches a background janitor to the handler.
// The janitor periodically evicts log events that have aged past their log
// group's retention policy. interval=0 uses the default of one minute.
// The optional taskTimeout bounds each sweep; 0 means no per-task timeout.
func (h *Handler) WithJanitor(interval time.Duration, taskTimeout ...time.Duration) *Handler {
	if memBackend, ok := h.Backend.(*InMemoryBackend); ok {
		j := NewJanitor(memBackend, interval)
		if len(taskTimeout) > 0 {
			j.TaskTimeout = taskTimeout[0]
		}

		h.janitor = j
	}

	return h
}

// StartWorker starts the background janitor if it is configured.
func (h *Handler) StartWorker(ctx context.Context) error {
	if h.janitor != nil {
		go h.janitor.Run(ctx)
	}

	return nil
}

// Name returns the service name.
func (h *Handler) Name() string { return "CloudWatchLogs" }

// GetSupportedOperations returns all mocked CloudWatch Logs operations.
func (h *Handler) GetSupportedOperations() []string {
	ops := append(cwlCoreOps(), cwlCompletenessOps()...)

	return append(ops, cwlLatestOps()...)
}

// cwlLatestOps returns CloudWatch Logs operations added in the parity-4 SDK
// bump (aws-sdk-go-v2/service/cloudwatchlogs v1.81.1): lookup tables, syslog
// configurations, and the account-level storage tier policy.
func cwlLatestOps() []string {
	return []string{
		"CreateLookupTable",
		"GetLookupTable",
		"UpdateLookupTable",
		"DeleteLookupTable",
		"DescribeLookupTables",
		"PutSyslogConfiguration",
		"ListSyslogConfigurations",
		"DeleteSyslogConfiguration",
		"GetStorageTierPolicy",
		"PutStorageTierPolicy",
	}
}

func cwlCoreOps() []string {
	return []string{
		"CreateLogGroup",
		"DeleteLogGroup",
		"DescribeLogGroups",
		"CreateLogStream",
		"DeleteLogStream",
		"DescribeLogStreams",
		"PutLogEvents",
		"GetLogEvents",
		"FilterLogEvents",
		"ListTagsLogGroup",
		"ListTagsForResource",
		"TagLogGroup",
		"UntagLogGroup",
		"TagResource",
		"UntagResource",
		"PutRetentionPolicy",
		"DeleteRetentionPolicy",
		"PutSubscriptionFilter",
		"DescribeSubscriptionFilters",
		"DeleteSubscriptionFilter",
		"StartQuery",
		"GetQueryResults",
		"StopQuery",
		"DescribeQueries",
		"AssociateKmsKey",
		"AssociateSourceToS3TableIntegration",
		"CancelExportTask",
		"CancelImportTask",
		"CreateDelivery",
		"CreateExportTask",
		"CreateImportTask",
		"CreateLogAnomalyDetector",
		"CreateScheduledQuery",
		"DeleteAccountPolicy",
		"DescribeExportTasks",
		"DescribeImportTasks",
		"DescribeDeliveries",
		"GetDelivery",
		"DeleteDelivery",
		"DeleteLogAnomalyDetector",
		"ListLogAnomalyDetectors",
		"UpdateLogAnomalyDetector",
		"DeleteScheduledQuery",
		"ListScheduledQueries",
		"UpdateScheduledQuery",
		"PutAccountPolicy",
		"DescribeAccountPolicies",
		"DisassociateKmsKey",
		"PutMetricFilter",
		"DescribeMetricFilters",
		"DeleteMetricFilter",
		"TestMetricFilter",
		"PutQueryDefinition",
		"DescribeQueryDefinitions",
		"DeleteQueryDefinition",
		"GetLogAnomalyDetector",
		"GetScheduledQuery",
		"GetLogGroupFields",
		"GetLogRecord",
		"ListAnomalies",
		"ListLogGroupsForQuery",
		"GetScheduledQueryHistory",
		"UpdateAnomaly",
		"ListLogGroups",
	}
}

// cwlCompletenessOps returns CloudWatch Logs operations added in the completeness pass.
func cwlCompletenessOps() []string {
	return []string{
		"DeleteDataProtectionPolicy",
		"DeleteDeliveryDestination",
		"DeleteDeliveryDestinationPolicy",
		"DeleteDeliverySource",
		"DeleteDestination",
		"DeleteIndexPolicy",
		"DeleteIntegration",
		"DeleteResourcePolicy",
		"DeleteTransformer",
		"DescribeConfigurationTemplates",
		"DescribeDeliveryDestinations",
		"DescribeDeliverySources",
		"DescribeDestinations",
		"DescribeFieldIndexes",
		"DescribeImportTaskBatches",
		"DescribeIndexPolicies",
		"DescribeResourcePolicies",
		"DisassociateSourceFromS3TableIntegration",
		"GetDataProtectionPolicy",
		"GetDeliveryDestination",
		"GetDeliveryDestinationPolicy",
		"GetDeliverySource",
		"GetIntegration",
		"GetLogFields",
		"GetLogObject",
		"GetTransformer",
		"ListAggregateLogGroupSummaries",
		"ListIntegrations",
		"ListSourcesForS3TableIntegration",
		"PutBearerTokenAuthentication",
		"PutDataProtectionPolicy",
		"PutDeliveryDestination",
		"PutDeliveryDestinationPolicy",
		"PutDeliverySource",
		"PutDestination",
		"PutDestinationPolicy",
		"PutIndexPolicy",
		"PutIntegration",
		"PutLogGroupDeletionProtection",
		"PutResourcePolicy",
		"PutTransformer",
		"StartLiveTail",
		"TestTransformer",
		"UpdateDeliveryConfiguration",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "logs" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this CloudWatch Logs instance handles.
func (h *Handler) ChaosRegions() []string { return []string{config.DefaultRegion} }

// RouteMatcher returns a matcher for CloudWatch Logs requests.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		target := c.Request().Header.Get("X-Amz-Target")

		return strings.HasPrefix(target, "Logs_20140328.")
	}
}

const cloudWatchLogsMatchPriority = 100

// MatchPriority returns the routing priority for the CloudWatch Logs handler.
func (h *Handler) MatchPriority() int { return cloudWatchLogsMatchPriority }

// ExtractOperation extracts the operation name from the X-Amz-Target header.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	parts := strings.Split(target, ".")
	const targetParts = 2
	if len(parts) == targetParts {
		return parts[1]
	}

	return "Unknown"
}

// ExtractResource extracts the resource name from the request body.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var data map[string]any
	if uerr := json.Unmarshal(body, &data); uerr != nil {
		return ""
	}

	for _, key := range []string{"logGroupName", "logStreamName"} {
		if v, ok := data[key].(string); ok && v != "" {
			return v
		}
	}

	return ""
}

// Handler returns the Echo handler function for CloudWatch Logs requests.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		region := httputils.ExtractRegionFromRequest(c.Request(), config.DefaultRegion)
		ctx := context.WithValue(c.Request().Context(), regionContextKey{}, region)
		c.SetRequest(c.Request().WithContext(ctx))

		return service.HandleTarget(
			c, logger.Load(ctx),
			"CloudWatchLogs", "application/x-amz-json-1.1",
			h.GetSupportedOperations(),
			h.dispatch,
			h.handleError,
		)
	}
}

type actionFn func(ctx context.Context, body []byte) (any, error)

// normalizeLogGroupIdentifier converts a log group identifier to a log group name.
// Log group identifiers may be ARNs (arn:...:log-group:<name>); in that case the
// log group name is extracted. Non-ARN identifiers are returned unchanged.
func normalizeLogGroupIdentifier(id string) string {
	const logGroupToken = ":log-group:"
	if idx := strings.LastIndex(id, logGroupToken); idx >= 0 {
		return id[idx+len(logGroupToken):]
	}

	return id
}

func (h *Handler) newOperationsActions() map[string]actionFn {
	return map[string]actionFn{
		"AssociateKmsKey":                     h.handleAssociateKmsKey,
		"AssociateSourceToS3TableIntegration": h.handleAssociateSourceToS3TableIntegration,
		"CancelExportTask":                    h.handleCancelExportTask,
		"CancelImportTask":                    h.handleCancelImportTask,
		"CreateDelivery":                      h.handleCreateDelivery,
		"CreateExportTask":                    h.handleCreateExportTask,
		"CreateImportTask":                    h.handleCreateImportTask,
		"CreateLogAnomalyDetector":            h.handleCreateLogAnomalyDetector,
		"CreateScheduledQuery":                h.handleCreateScheduledQuery,
		"DeleteAccountPolicy":                 h.handleDeleteAccountPolicy,
		"DescribeExportTasks":                 h.handleDescribeExportTasks,
		"DescribeImportTasks":                 h.handleDescribeImportTasks,
		"DescribeDeliveries":                  h.handleDescribeDeliveries,
		"GetDelivery":                         h.handleGetDelivery,
		"DeleteDelivery":                      h.handleDeleteDelivery,
		"DeleteLogAnomalyDetector":            h.handleDeleteLogAnomalyDetector,
		"ListLogAnomalyDetectors":             h.handleListLogAnomalyDetectors,
		"UpdateLogAnomalyDetector":            h.handleUpdateLogAnomalyDetector,
		"DeleteScheduledQuery":                h.handleDeleteScheduledQuery,
		"ListScheduledQueries":                h.handleListScheduledQueries,
		"UpdateScheduledQuery":                h.handleUpdateScheduledQuery,
		"PutAccountPolicy":                    h.handlePutAccountPolicy,
		"DescribeAccountPolicies":             h.handleDescribeAccountPolicies,
		"DisassociateKmsKey":                  h.handleDisassociateKmsKey,
		"PutMetricFilter":                     h.handlePutMetricFilter,
		"DescribeMetricFilters":               h.handleDescribeMetricFilters,
		"DeleteMetricFilter":                  h.handleDeleteMetricFilter,
		"TestMetricFilter":                    h.handleTestMetricFilter,
		"PutQueryDefinition":                  h.handlePutQueryDefinition,
		"DescribeQueryDefinitions":            h.handleDescribeQueryDefinitions,
		"DeleteQueryDefinition":               h.handleDeleteQueryDefinition,
		"GetLogAnomalyDetector":               h.handleGetLogAnomalyDetector,
		"GetScheduledQuery":                   h.handleGetScheduledQuery,
		"GetLogGroupFields":                   h.handleGetLogGroupFields,
		"GetLogRecord":                        h.handleGetLogRecord,
		"ListAnomalies":                       h.handleListAnomalies,
		"ListLogGroupsForQuery":               h.handleListLogGroupsForQuery,
		"GetScheduledQueryHistory":            h.handleGetScheduledQueryHistory,
		"UpdateAnomaly":                       h.handleUpdateAnomaly,
		"ListLogGroups":                       h.handleListLogGroups,
	}
}

func (h *Handler) buildOps() map[string]actionFn {
	table := make(map[string]actionFn)
	maps.Copy(table, h.logGroupActions())
	maps.Copy(table, h.logStreamActions())
	maps.Copy(table, h.logEventActions())
	maps.Copy(table, h.logTagActions())
	maps.Copy(table, h.retentionActions())
	maps.Copy(table, h.subscriptionFilterActions())
	maps.Copy(table, h.insightsActions())
	maps.Copy(table, h.newOperationsActions())
	maps.Copy(table, h.completenessActions())
	maps.Copy(table, h.latestOperationsActions())

	return table
}

// latestOperationsActions returns dispatch entries for the parity-4 SDK bump
// operations (see cwlLatestOps).
func (h *Handler) latestOperationsActions() map[string]actionFn {
	return map[string]actionFn{
		"CreateLookupTable":         h.handleCreateLookupTable,
		"GetLookupTable":            h.handleGetLookupTable,
		"UpdateLookupTable":         h.handleUpdateLookupTable,
		"DeleteLookupTable":         h.handleDeleteLookupTable,
		"DescribeLookupTables":      h.handleDescribeLookupTables,
		"PutSyslogConfiguration":    h.handlePutSyslogConfiguration,
		"ListSyslogConfigurations":  h.handleListSyslogConfigurations,
		"DeleteSyslogConfiguration": h.handleDeleteSyslogConfiguration,
		"GetStorageTierPolicy":      h.handleGetStorageTierPolicy,
		"PutStorageTierPolicy":      h.handlePutStorageTierPolicy,
	}
}

// dispatch routes the action to the correct handler function.
func (h *Handler) dispatch(ctx context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w:%s", errUnknownOperation, action)
	}

	response, err := fn(ctx, body)
	if err != nil {
		return nil, err
	}

	return json.Marshal(response)
}

// handleError writes a standardized JSON error response.
func (h *Handler) handleError(ctx context.Context, c *echo.Context, action string, reqErr error) error {
	log := logger.Load(ctx)
	c.Response().Header().Set("Content-Type", "application/x-amz-json-1.1")

	var errType string
	var statusCode int

	switch {
	case errors.Is(reqErr, ErrLogGroupNotFound), errors.Is(reqErr, ErrLogStreamNotFound),
		errors.Is(reqErr, ErrSubscriptionFilterNotFound), errors.Is(reqErr, ErrQueryNotFound),
		errors.Is(reqErr, ErrExportTaskNotFound), errors.Is(reqErr, ErrImportTaskNotFound),
		errors.Is(reqErr, ErrDeliveryNotFound), errors.Is(reqErr, ErrLogAnomalyDetectorNotFound),
		errors.Is(reqErr, ErrScheduledQueryNotFound), errors.Is(reqErr, ErrMetricFilterNotFound),
		errors.Is(reqErr, ErrQueryDefinitionNotFound),
		errors.Is(reqErr, ErrResourcePolicyNotFound), errors.Is(reqErr, ErrDeliveryDestinationNotFound),
		errors.Is(reqErr, ErrDeliverySourceNotFound), errors.Is(reqErr, ErrDestinationNotFound),
		errors.Is(reqErr, ErrIndexPolicyNotFound), errors.Is(reqErr, ErrTransformerNotFound),
		errors.Is(reqErr, ErrIntegrationNotFound), errors.Is(reqErr, ErrLookupTableNotFound),
		errors.Is(reqErr, ErrSyslogConfigurationNotFound), errors.Is(reqErr, ErrS3TableIntegrationNotFound):
		errType = "ResourceNotFoundException"
		statusCode = http.StatusNotFound
	case errors.Is(reqErr, ErrLogGroupAlreadyExists), errors.Is(reqErr, ErrLogStreamAlreadyExist),
		errors.Is(reqErr, ErrLookupTableAlreadyExists):
		errType = "ResourceAlreadyExistsException"
		statusCode = http.StatusConflict
	case errors.Is(reqErr, ErrDeliveryDestinationInUse), errors.Is(reqErr, ErrDeliverySourceInUse):
		errType = "ConflictException"
		statusCode = http.StatusConflict
	case errors.Is(reqErr, ErrSubscriptionFilterLimitExceed):
		errType = "LimitExceededException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, ErrOperationAborted):
		errType = "OperationAbortedException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, ErrInvalidOperation):
		errType = "InvalidOperationException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, ErrValidation):
		errType = "InvalidParameterException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, ErrValidationException):
		errType = "ValidationException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, ErrScheduledQueryLimitExceeded):
		errType = "ServiceQuotaExceededException"
		statusCode = http.StatusBadRequest
	case errors.Is(reqErr, errUnknownOperation):
		errType = "UnknownOperationException"
		statusCode = http.StatusBadRequest
	default:
		errType = "ServiceUnavailableException"
		statusCode = http.StatusInternalServerError
	}

	if statusCode == http.StatusInternalServerError {
		log.ErrorContext(ctx, "CloudWatchLogs internal error", "error", reqErr, "action", action)
	} else {
		log.WarnContext(ctx, "CloudWatchLogs request error", "error", reqErr, "action", action)
	}

	errResp := service.JSONErrorResponse{
		Type:    errType,
		Message: reqErr.Error(),
	}

	payload, _ := json.Marshal(errResp)

	return c.JSONBlob(statusCode, payload)
}

// Reset clears all in-memory state from the backend and the handler-level tag
// store. It is used by the POST /_gopherstack/reset endpoint for CI pipelines
// and rapid local development.
func (h *Handler) Reset() {
	if b, ok := h.Backend.(*InMemoryBackend); ok {
		b.Reset()
	}

	// Clear handler-level tag state so that tags don't bleed across test runs.
	h.tagsMu.Lock("Reset")
	defer h.tagsMu.Unlock()

	for _, t := range h.tags {
		t.Close()
	}

	h.tags = make(map[string]*tags.Tags)
}

const (
	completenessKeyLogGroupIdentifier = "logGroupIdentifier"
	completenessKeyPolicyDocument     = "policyDocument"
	completenessStatusActive          = "ACTIVE"
)

const (
	keyName                = "name"
	keyArn                 = "arn"
	keyDeliveryDestination = "deliveryDestination"
	keyDeliverySource      = "deliverySource"
	keyDestinationName     = "destinationName"
	keyTargetArn           = "targetArn"
	keyRoleArn             = "roleArn"
	keyIntegrationName     = "integrationName"
	keyIntegrationStatus   = "integrationStatus"
	keyIntegrationType     = "integrationType"
	keyAccessPolicy        = "accessPolicy"
	keyCreationTime        = "creationTime"
	keyLookupTableArn      = "lookupTableArn"
	keyLastUpdatedTime     = "lastUpdatedTime"
	keyState               = "state"
	keyScheduledQueryArn   = "scheduledQueryArn"
	keyAccountID           = "accountId"
)

// completenessActions returns dispatch entries for all previously notImplemented CloudWatch Logs operations.
func (h *Handler) completenessActions() map[string]actionFn {
	return map[string]actionFn{
		"DeleteDataProtectionPolicy":               h.handleDeleteDataProtectionPolicy,
		"DeleteDeliveryDestination":                h.handleDeleteDeliveryDestination,
		"DeleteDeliveryDestinationPolicy":          h.handleDeleteDeliveryDestinationPolicy,
		"DeleteDeliverySource":                     h.handleDeleteDeliverySource,
		"DeleteDestination":                        h.handleDeleteDestination,
		"DeleteIndexPolicy":                        h.handleDeleteIndexPolicy,
		"DeleteIntegration":                        h.handleDeleteIntegration,
		"DeleteResourcePolicy":                     h.handleDeleteResourcePolicy,
		"DeleteTransformer":                        h.handleDeleteTransformer,
		"DescribeConfigurationTemplates":           h.handleDescribeConfigurationTemplates,
		"DescribeDeliveryDestinations":             h.handleDescribeDeliveryDestinations,
		"DescribeDeliverySources":                  h.handleDescribeDeliverySources,
		"DescribeDestinations":                     h.handleDescribeDestinations,
		"DescribeFieldIndexes":                     h.handleDescribeFieldIndexes,
		"DescribeImportTaskBatches":                h.handleDescribeImportTaskBatches,
		"DescribeIndexPolicies":                    h.handleDescribeIndexPolicies,
		"DescribeResourcePolicies":                 h.handleDescribeResourcePolicies,
		"DisassociateSourceFromS3TableIntegration": h.handleDisassociateSourceFromS3TableIntegration,
		"GetDataProtectionPolicy":                  h.handleGetDataProtectionPolicy,
		"GetDeliveryDestination":                   h.handleGetDeliveryDestination,
		"GetDeliveryDestinationPolicy":             h.handleGetDeliveryDestinationPolicy,
		"GetDeliverySource":                        h.handleGetDeliverySource,
		"GetIntegration":                           h.handleGetIntegration,
		"GetLogFields":                             h.handleGetLogFields,
		"GetLogObject":                             h.handleGetLogObject,
		"GetTransformer":                           h.handleGetTransformer,
		"ListAggregateLogGroupSummaries":           h.handleListAggregateLogGroupSummaries,
		"ListIntegrations":                         h.handleListIntegrations,
		"ListSourcesForS3TableIntegration":         h.handleListSourcesForS3TableIntegration,
		"PutBearerTokenAuthentication":             h.handlePutBearerTokenAuthentication,
		"PutDataProtectionPolicy":                  h.handlePutDataProtectionPolicy,
		"PutDeliveryDestination":                   h.handlePutDeliveryDestination,
		"PutDeliveryDestinationPolicy":             h.handlePutDeliveryDestinationPolicy,
		"PutDeliverySource":                        h.handlePutDeliverySource,
		"PutDestination":                           h.handlePutDestination,
		"PutDestinationPolicy":                     h.handlePutDestinationPolicy,
		"PutIndexPolicy":                           h.handlePutIndexPolicy,
		"PutIntegration":                           h.handlePutIntegration,
		"PutLogGroupDeletionProtection":            h.handlePutLogGroupDeletionProtection,
		"PutResourcePolicy":                        h.handlePutResourcePolicy,
		"PutTransformer":                           h.handlePutTransformer,
		"StartLiveTail":                            h.handleStartLiveTail,
		"TestTransformer":                          h.handleTestTransformer,
		"UpdateDeliveryConfiguration":              h.handleUpdateDeliveryConfiguration,
	}
}

// cwlBackend returns the InMemoryBackend, or nil if the backend is not an InMemoryBackend.
func cwlBackend(h *Handler) *InMemoryBackend {
	b, _ := h.Backend.(*InMemoryBackend)

	return b
}
