package cloudwatchlogs

import "context"

// SubscriptionDeliverer delivers encoded log event payloads to a subscription filter destination.
type SubscriptionDeliverer interface {
	// DeliverLogEvents delivers a gzipped, base64-encoded CloudWatch Logs payload to destinationArn.
	DeliverLogEvents(ctx context.Context, destinationArn string, payload []byte) error
}

// SubscriptionDelivererFunc is a function adapter for SubscriptionDeliverer.
type SubscriptionDelivererFunc func(ctx context.Context, destinationArn string, payload []byte) error

// DeliverLogEvents implements SubscriptionDeliverer.
func (f SubscriptionDelivererFunc) DeliverLogEvents(
	ctx context.Context,
	destinationArn string,
	payload []byte,
) error {
	return f(ctx, destinationArn, payload)
}

// MetricEmitter emits a CloudWatch metric data point.
// It is implemented by the CloudWatch backend and injected into InMemoryBackend
// so that metric filter matches on PutLogEvents can be forwarded to CloudWatch.
type MetricEmitter interface {
	// EmitMetric records a single metric data point with the given namespace, name, value, and unit.
	EmitMetric(namespace, name string, value float64, unit string) error
}

// MetricEmitterFunc is a function adapter for MetricEmitter.
type MetricEmitterFunc func(namespace, name string, value float64, unit string) error

// EmitMetric implements MetricEmitter.
func (f MetricEmitterFunc) EmitMetric(namespace, name string, value float64, unit string) error {
	return f(namespace, name, value, unit)
}

// StorageBackend is the interface for a CloudWatch Logs in-memory store.
type StorageBackend interface {
	CreateLogGroup(ctx context.Context, name, logGroupClass, kmsKeyID string) (*LogGroup, error)
	DeleteLogGroup(ctx context.Context, name string) error
	DescribeLogGroups(
		ctx context.Context,
		prefix, nextToken string,
		limit int,
	) ([]LogGroup, string, error)
	CreateLogStream(ctx context.Context, groupName, streamName string) (*LogStream, error)
	DeleteLogStream(ctx context.Context, groupName, streamName string) error
	DescribeLogStreams(
		ctx context.Context,
		groupName, prefix, nextToken, orderBy string,
		descending bool,
		limit int,
	) ([]LogStream, string, error)
	PutLogEvents(
		ctx context.Context, groupName, streamName, sequenceToken string, events []InputLogEvent,
	) (*PutLogEventsResult, error)
	GetLogEvents(
		ctx context.Context,
		groupName, streamName string,
		startTime, endTime *int64,
		limit int,
		nextToken string,
		startFromHead bool,
	) (
		[]OutputLogEvent, string, string, error)
	FilterLogEvents(ctx context.Context, p FilterLogEventsParams) (
		[]FilteredLogEvent, string, []SearchedLogStream, error)
	PutSubscriptionFilter(
		ctx context.Context, groupName, filterName, filterPattern, destinationArn, roleArn, distribution string,
	) error
	DescribeSubscriptionFilters(
		ctx context.Context,
		groupName, filterNamePrefix, nextToken string,
		limit int,
	) (
		[]SubscriptionFilter, string, error)
	DeleteSubscriptionFilter(ctx context.Context, groupName, filterName string) error
	SetRetentionPolicy(ctx context.Context, groupName string, days *int32) error
	StartQuery(
		ctx context.Context, queryID, queryString string, logGroupNames []string, startTime, endTime int64,
	) (*QueryInfo, error)
	GetQueryResults(queryID string) ([][]ResultField, QueryStatistics, QueryStatus, error)
	StopQuery(queryID string) error
	DescribeQueries(
		logGroupName, statusFilter, nextToken string,
		maxResults int,
	) ([]QueryInfo, string, error)

	// AssociateKmsKey associates a KMS key with a log group or query results resource.
	AssociateKmsKey(logGroupName, resourceIdentifier, kmsKeyID string) error
	// AssociateSourceToS3TableIntegration associates a data source with an S3 table integration.
	AssociateSourceToS3TableIntegration(
		integrationArn, dataSourceName, dataSourceType string,
	) (string, error)
	// DisassociateSourceFromS3TableIntegration removes a source association by identifier.
	DisassociateSourceFromS3TableIntegration(identifier string) error
	// CancelExportTask cancels a pending or running export task.
	CancelExportTask(taskID string) error
	// CancelImportTask cancels a running import task.
	CancelImportTask(importID string) (*ImportTask, error)
	// CreateDelivery creates a delivery between a delivery source and destination.
	CreateDelivery(
		deliverySourceName, deliveryDestinationArn, fieldDelimiter string,
		recordFields []string,
		s3Config *DeliveryS3Configuration,
		tags map[string]string,
	) (*Delivery, error)
	// CreateExportTask creates an asynchronous export task to S3.
	CreateExportTask(
		ctx context.Context,
		taskName, logGroupName, logStreamNamePrefix, destination, destinationPrefix string,
		from, to int64,
	) (string, error)
	// CreateImportTask creates an import task from a CloudTrail Lake event data store.
	CreateImportTask(ctx context.Context, importRoleArn, importSourceArn string) (*ImportTask, error)
	// CreateLogAnomalyDetector creates an anomaly detector for one or more log groups.
	CreateLogAnomalyDetector(
		logGroupArnList []string,
		detectorName, evaluationFrequency, filterPattern, kmsKeyID string,
		anomalyVisibilityTime int64,
	) (string, error)
	// CreateScheduledQuery creates a scheduled CloudWatch Logs Insights query.
	CreateScheduledQuery(p ScheduledQueryCreateParams) (string, error)
	// DeleteAccountPolicy deletes a CloudWatch Logs account-level policy.
	DeleteAccountPolicy(policyName, policyType string) error
	// DescribeExportTasks lists export tasks optionally filtered by task ID or status.
	DescribeExportTasks(
		taskID, statusCode string,
		limit int,
		nextToken string,
	) ([]ExportTask, string, error)
	// DescribeImportTasks lists import tasks optionally filtered by task ID.
	DescribeImportTasks(taskID string, limit int, nextToken string) ([]ImportTask, string, error)
	// DescribeDeliveries lists deliveries with pagination.
	DescribeDeliveries(limit int, nextToken string) ([]Delivery, string, error)
	// GetDelivery returns a single delivery by ID.
	GetDelivery(id string) (*Delivery, error)
	// DeleteDelivery deletes a delivery by ID.
	DeleteDelivery(id string) error
	// DeleteLogAnomalyDetector deletes a log anomaly detector.
	DeleteLogAnomalyDetector(detectorArn string) error
	// ListLogAnomalyDetectors lists anomaly detectors, optionally filtered by log group ARN.
	ListLogAnomalyDetectors(
		filterLogGroupArnList []string,
		limit int,
		nextToken string,
	) ([]LogAnomalyDetector, string, error)
	// UpdateLogAnomalyDetector updates evaluation frequency and/or anomaly
	// visibility time, and pauses/resumes the detector via enabled.
	UpdateLogAnomalyDetector(
		detectorArn, evaluationFrequency string,
		anomalyVisibilityTime int64,
		enabled bool,
	) error
	// DeleteScheduledQuery deletes a scheduled query by ARN.
	DeleteScheduledQuery(scheduledQueryArn string) error
	// ListScheduledQueries lists all scheduled queries with pagination.
	ListScheduledQueries(limit int, nextToken string) ([]ScheduledQuery, string, error)
	// UpdateScheduledQuery updates the state of a scheduled query.
	UpdateScheduledQuery(scheduledQueryArn, state string) error
	// PutAccountPolicy creates or updates an account-level policy.
	PutAccountPolicy(
		policyName, policyType, policyDocument, scope, selectionCriteria string,
	) (*AccountPolicy, error)
	// DescribeAccountPolicies returns account-level policies, optionally filtered.
	DescribeAccountPolicies(
		policyType, policyName string,
		accountIdentifiers []string,
		limit int,
		nextToken string,
	) ([]AccountPolicy, string, error)
	// DisassociateKmsKey removes the KMS key association from a log group or resource.
	DisassociateKmsKey(logGroupName, resourceIdentifier string) error
	// PutMetricFilter creates or updates a metric filter for a log group.
	PutMetricFilter(
		ctx context.Context, logGroupName, filterName, filterPattern string, transformations []MetricTransformation,
	) error
	// DescribeMetricFilters lists metric filters with optional filters.
	DescribeMetricFilters(
		ctx context.Context,
		logGroupName, filterNamePrefix, metricName, metricNamespace, nextToken string,
		limit int,
	) ([]MetricFilter, string, error)
	// DeleteMetricFilter deletes a metric filter from a log group.
	DeleteMetricFilter(ctx context.Context, logGroupName, filterName string) error
	// TestMetricFilter tests a metric filter pattern against provided log event messages.
	TestMetricFilter(
		filterPattern string,
		logEventMessages []string,
	) ([]MetricFilterMatchRecord, error)
	// PutQueryDefinition creates or updates a query definition.
	PutQueryDefinition(
		name, queryString, queryDefinitionID string,
		logGroupNames []string,
		parameters []QueryParameter,
	) (string, error)
	// DescribeQueryDefinitions lists query definitions optionally filtered by name prefix.
	DescribeQueryDefinitions(
		queryDefinitionNamePrefix string,
		limit int,
		nextToken string,
	) ([]QueryDefinition, string, error)
	// DeleteQueryDefinition deletes a query definition by ID.
	DeleteQueryDefinition(queryDefinitionID string) error
	// GetLogAnomalyDetector returns the anomaly detector with the given ARN.
	GetLogAnomalyDetector(detectorArn string) (*LogAnomalyDetector, error)
	// GetScheduledQuery returns the scheduled query with the given ARN.
	GetScheduledQuery(scheduledQueryArn string) (*ScheduledQuery, error)
	// GetLogGroupFields returns the fields discovered in log events sampled from
	// the log group, each with the percentage of sampled events that contained
	// it. timeSec (epoch seconds), when non-nil, centers an 8-minutes-either-side
	// sampling window; when nil, the most recent 15 minutes up to now is sampled.
	GetLogGroupFields(ctx context.Context, logGroupName string, timeSec *int64) ([]LogGroupField, error)
	// GetLogRecord returns a single log event by its log record pointer.
	GetLogRecord(ctx context.Context, logRecordPointer string) (map[string]string, error)
	// ListAnomalies lists anomalies for the given anomaly detector ARN with pagination.
	ListAnomalies(anomalyDetectorArn string, limit int, nextToken string) ([]Anomaly, string, error)
	// ListLogGroupsForQuery returns the log group names used in a specific query.
	ListLogGroupsForQuery(queryID string) ([]string, error)
	// GetScheduledQueryHistory returns the execution history for a scheduled query.
	GetScheduledQueryHistory(
		scheduledQueryArn string,
		nextToken string,
		maxResults int,
	) ([]ScheduledQueryRunSummary, string, error)
	// UpdateAnomaly updates anomaly suppression settings, by anomalyID or,
	// when anomalyID is empty, every anomaly sharing patternID.
	UpdateAnomaly(anomalyID, anomalyDetectorArn, suppressionType, patternID string) error
	// ListLogGroups is the newer paginated list operation, equivalent to DescribeLogGroups.
	ListLogGroups(
		ctx context.Context,
		namePrefix, nextToken string,
		limit int,
	) ([]LogGroup, string, error)
}
