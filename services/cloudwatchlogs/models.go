package cloudwatchlogs

import "time"

// LogGroupClass constants match the AWS CloudWatch Logs API enum.
const (
	LogGroupClassStandard         = "STANDARD"
	LogGroupClassInfrequentAccess = "INFREQUENT_ACCESS"
)

// LogGroup represents a CloudWatch Logs log group.
type LogGroup struct {
	RetentionInDays *int32 `json:"retentionInDays,omitempty"`
	// BearerTokenAuthenticationEnabled mirrors types.LogGroup's field of the
	// same name (types.go:1366), set via PutBearerTokenAuthentication. nil
	// until explicitly set (matches a log group that was never touched by
	// PutBearerTokenAuthentication -- AWS's own doc doesn't say this
	// defaults to a concrete true/false).
	BearerTokenAuthenticationEnabled *bool  `json:"bearerTokenAuthenticationEnabled,omitempty"`
	LogGroupName                     string `json:"logGroupName"`
	Arn                              string `json:"arn"`
	LogGroupClass                    string `json:"logGroupClass,omitempty"`
	KmsKeyID                         string `json:"kmsKeyId,omitempty"`
	// region is the AWS region this group lives under. It is unexported (never
	// marshaled, so the wire response shape is unaffected) and exists solely so
	// the store.Table[LogGroup] holding every region's groups can derive a
	// region-qualified primary key purely from the value itself -- see
	// groupTableKey in store_setup.go. Persistence round-trips it through the
	// separate logGroupSnapshot DTO (see persistence.go) since an unexported
	// field cannot survive Table.Snapshot's json.Marshal on its own.
	region            string
	CreationTime      int64 `json:"creationTime"`
	StoredBytes       int64 `json:"storedBytes"`
	MetricFilterCount int32 `json:"metricFilterCount"`
}

// LogStream represents a CloudWatch Logs log stream.
type LogStream struct {
	FirstEventTimestamp *int64 `json:"firstEventTimestamp,omitempty"`
	LastEventTimestamp  *int64 `json:"lastEventTimestamp,omitempty"`
	LastIngestionTime   *int64 `json:"lastIngestionTime,omitempty"`
	LogStreamName       string `json:"logStreamName"`
	Arn                 string `json:"arn"`
	UploadSequenceToken string `json:"uploadSequenceToken"`
	// region and logGroupName are unexported identity metadata (see LogGroup.region)
	// letting store.Table[LogStream] key every region+group's streams from the
	// value alone. events holds this stream's log events inline (matching the
	// sqs/s3 pattern of nesting child records on the parent entity rather than a
	// separate table) and is likewise unexported; both round-trip through the
	// logStreamSnapshot DTO in persistence.go.
	region       string
	logGroupName string
	events       []*OutputLogEvent
	CreationTime int64 `json:"creationTime"`
	StoredBytes  int64 `json:"storedBytes"`
}

// InputLogEvent represents a single log event for PutLogEvents.
type InputLogEvent struct {
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// OutputLogEvent represents a single log event returned by GetLogEvents.
type OutputLogEvent struct {
	Message       string `json:"message"`
	Ptr           string `json:"ptr,omitempty"`
	IngestionTime int64  `json:"ingestionTime"`
	Timestamp     int64  `json:"timestamp"`
}

// FilteredLogEvent represents a single matched event returned by FilterLogEvents.
// Unlike OutputLogEvent (used by GetLogEvents), it carries the originating log
// stream name and a unique eventId, matching the AWS FilteredLogEvent shape.
type FilteredLogEvent struct {
	EventID       string `json:"eventId"`
	LogStreamName string `json:"logStreamName"`
	Message       string `json:"message"`
	IngestionTime int64  `json:"ingestionTime"`
	Timestamp     int64  `json:"timestamp"`
}

// SearchedLogStream indicates whether a log stream was searched completely by
// FilterLogEvents. AWS deprecated populating this list (it returns empty) but the
// field remains part of the response shape.
type SearchedLogStream struct {
	LogStreamName      string `json:"logStreamName"`
	SearchedCompletely bool   `json:"searchedCompletely"`
}

// LogGroupField is a field name and estimated percentage of log events that contain the field.
type LogGroupField struct {
	Name    string `json:"name"`
	Percent int32  `json:"percent"`
}

// Anomaly state constants match the real aws-sdk-go-v2 types.State enum
// (types.StateActive/StateSuppressed/types.StateBaseline).
const (
	AnomalyStateActive     = "Active"
	AnomalyStateSuppressed = "Suppressed"
	AnomalyStateBaseline   = "Baseline"
)

// AnomalyLogSample is one sample log event considered part of an anomaly.
// Mirrors aws-sdk-go-v2 types.LogEvent (message/timestamp).
type AnomalyLogSample struct {
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// PatternToken describes one token making up an anomaly's identifying
// pattern. Mirrors aws-sdk-go-v2 types.PatternToken.
type PatternToken struct {
	Enumerations         map[string]int64 `json:"enumerations,omitempty"`
	IsDynamic            *bool            `json:"isDynamic,omitempty"`
	InferredTokenName    string           `json:"inferredTokenName,omitempty"`
	TokenString          string           `json:"tokenString,omitempty"`
	DynamicTokenPosition int32            `json:"dynamicTokenPosition,omitempty"`
}

// Anomaly represents a detected log anomaly. Field-diffed against
// aws-sdk-go-v2 types.Anomaly: the wire key for lifecycle state is "state"
// (types.Anomaly.State, values Active/Suppressed/Baseline), not
// "suppressedState" -- a previous revision used a made-up key holding the
// raw suppressionType request value ("LIMITED"/"INFINITE"/a
// gopherstack-invented "NO_SUPPRESSION"), which is not a real wire member at
// all, so a real client's State field always deserialized empty. This
// backend has no pattern-detection engine (anomalies are only ever
// synthesized via the AddAnomalyInternal test seam, never generated from
// real log content -- see anomaly_detectors.go), so Histogram/LogSamples/
// PatternID/PatternString/PatternTokens are never computed here; they are
// modeled so a caller-seeded anomaly (test or otherwise) round-trips them,
// but this backend supplies no analysis to fill them in on its own.
type Anomaly struct {
	IsPatternLevelSuppression *bool              `json:"isPatternLevelSuppression,omitempty"`
	Suppressed                *bool              `json:"suppressed,omitempty"`
	Histogram                 map[string]int64   `json:"histogram,omitempty"`
	Description               string             `json:"description"`
	State                     string             `json:"state,omitempty"`
	PatternID                 string             `json:"patternId,omitempty"`
	PatternString             string             `json:"patternString,omitempty"`
	PatternRegex              string             `json:"patternRegex,omitempty"`
	Priority                  string             `json:"priority,omitempty"`
	AnomalyID                 string             `json:"anomalyId"`
	AnomalyDetectorArn        string             `json:"anomalyDetectorArn"`
	PatternTokens             []PatternToken     `json:"patternTokens,omitempty"`
	LogSamples                []AnomalyLogSample `json:"logSamples,omitempty"`
	FirstSeen                 int64              `json:"firstSeen"`
	LastSeen                  int64              `json:"lastSeen"`
	SuppressedDate            int64              `json:"suppressedDate,omitempty"`
	SuppressedUntil           int64              `json:"suppressedUntil,omitempty"`
	Active                    bool               `json:"active"`
}

// ScheduledQueryRunSummary describes a single scheduled query execution.
type ScheduledQueryRunSummary struct {
	Arn            string `json:"arn"`
	FailureReason  string `json:"failureReason,omitempty"`
	RunStatus      string `json:"runStatus"`
	ExecutionTime  int64  `json:"executionTime"`
	InvocationTime int64  `json:"invocationTime"`
}

// Distribution constants for subscription filter event routing.
const (
	DistributionRandom      = "Random"
	DistributionByLogStream = "ByLogStream"
)

// SubscriptionFilter represents a CloudWatch Logs subscription filter.
type SubscriptionFilter struct {
	FilterPattern  string `json:"filterPattern"`
	FilterName     string `json:"filterName"`
	LogGroupName   string `json:"logGroupName"`
	DestinationArn string `json:"destinationArn"`
	RoleArn        string `json:"roleArn,omitempty"`
	Distribution   string `json:"distribution,omitempty"`
	// region is unexported identity metadata (see LogGroup.region) letting
	// store.Table[SubscriptionFilter] key every region+group's filters from the
	// value alone; it round-trips through the subscriptionFilterSnapshot DTO.
	region       string
	CreationTime int64 `json:"creationTime"`
}

// subscriptionLogEvent is one event in a subscription filter delivery payload.
type subscriptionLogEvent struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// subscriptionPayload is the CloudWatch Logs subscription filter delivery payload.
type subscriptionPayload struct {
	SubscriptionFilters []string               `json:"subscriptionFilters"`
	MessageType         string                 `json:"messageType"`
	Owner               string                 `json:"owner"`
	LogGroup            string                 `json:"logGroup"`
	LogStream           string                 `json:"logStream"`
	LogEvents           []subscriptionLogEvent `json:"logEvents"`
}

// QueryStatus represents the lifecycle status of a Logs Insights query.
type QueryStatus string

const (
	QueryStatusScheduled QueryStatus = "Scheduled"
	QueryStatusRunning   QueryStatus = "Running"
	QueryStatusComplete  QueryStatus = "Complete"
	QueryStatusFailed    QueryStatus = "Failed"
	QueryStatusCancelled QueryStatus = "Cancelled"
)

// ResultField is a single field in a Logs Insights result row.
type ResultField struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// QueryStatistics contains execution statistics for a Logs Insights query.
type QueryStatistics struct {
	BytesScanned   float64 `json:"bytesScanned"`
	RecordsMatched float64 `json:"recordsMatched"`
	RecordsScanned float64 `json:"recordsScanned"`
}

// QueryInfo contains metadata about a Logs Insights query.
type QueryInfo struct {
	QueryID      string      `json:"queryId"`
	QueryString  string      `json:"queryString"`
	LogGroupName string      `json:"logGroupName,omitempty"`
	Status       QueryStatus `json:"status"`
	CreateTime   int64       `json:"createTime"`
}

// ExportTask represents a CloudWatch Logs export task.
type ExportTask struct {
	Status              string `json:"status"`
	TaskID              string `json:"taskId"`
	LogGroupName        string `json:"logGroupName"`
	Destination         string `json:"destination"`
	DestinationPrefix   string `json:"destinationPrefix,omitempty"`
	LogStreamNamePrefix string `json:"logStreamNamePrefix,omitempty"`
	TaskName            string `json:"taskName,omitempty"`
	StatusMessage       string `json:"statusMessage,omitempty"`
	Region              string `json:"-"`
	From                int64  `json:"from"`
	To                  int64  `json:"to"`
	CreationTime        int64  `json:"creationTime"`
	CompletionTime      int64  `json:"completionTime,omitempty"`
}

// ImportTask represents a CloudWatch Logs import task (from CloudTrail Lake).
// Status values must be one of the real aws-sdk-go-v2 types.ImportStatus enum
// members: IN_PROGRESS, CANCELLED, COMPLETED, FAILED (NOT "ACTIVE"/"SUCCEEDED",
// which are not real CloudWatch Logs import statuses). The wire key for this
// field is importStatus (aws-sdk-go-v2 types.Import.ImportStatus), not status;
// ImportRoleArn is accepted on CreateImportTask but is deliberately not part
// of the real Import describe/list shape (types.Import has no such field), so
// it is kept here for this backend's own bookkeeping but is excluded when
// serialized to the wire (see wireImportTask in handler_export_tasks.go).
type ImportTask struct {
	ImportID             string `json:"importId"`
	ImportSourceArn      string `json:"importSourceArn"`
	ImportRoleArn        string `json:"-"`
	ImportDestinationArn string `json:"importDestinationArn"`
	Status               string `json:"importStatus"`
	CreationTime         int64  `json:"creationTime"`
	LastUpdatedTime      int64  `json:"lastUpdatedTime"`
}

// DeliveryS3Configuration mirrors the real S3DeliveryConfiguration shape
// (aws-sdk-go-v2 types.S3DeliveryConfiguration): parameters that apply only
// when a delivery's destination is an S3 bucket.
type DeliveryS3Configuration struct {
	SuffixPath               string `json:"suffixPath,omitempty"`
	EnableHiveCompatiblePath bool   `json:"enableHiveCompatiblePath,omitempty"`
}

// Delivery represents a CloudWatch Logs delivery configuration.
// CreationTime is backend bookkeeping only (used to order DescribeDeliveries)
// and must never reach the wire: the real Delivery type (aws-sdk-go-v2
// types.Delivery) has no creationTime member at all -- an earlier revision
// fabricated one.
type Delivery struct {
	Tags                    map[string]string        `json:"tags,omitempty"`
	S3DeliveryConfiguration *DeliveryS3Configuration `json:"s3DeliveryConfiguration,omitempty"`
	ID                      string                   `json:"id"`
	Arn                     string                   `json:"arn"`
	DeliverySourceName      string                   `json:"deliverySourceName"`
	DeliveryDestinationArn  string                   `json:"deliveryDestinationArn"`
	DeliveryDestinationType string                   `json:"deliveryDestinationType,omitempty"`
	FieldDelimiter          string                   `json:"fieldDelimiter,omitempty"`
	RecordFields            []string                 `json:"recordFields,omitempty"`
	CreationTime            int64                    `json:"-"`
}

// LogAnomalyDetector represents a CloudWatch Logs anomaly detector.
// LogAnomalyDetector represents a CloudWatch Logs log anomaly detector.
// Field-diffed against aws-sdk-go-v2 types.AnomalyDetector /
// GetLogAnomalyDetectorOutput: the status wire key is anomalyDetectorStatus,
// not detectorStatus (a previous version of this struct used the wrong key,
// so a real SDK client would always see an empty/unknown status).
// evaluationLookback and filterAnomalies were also removed here: neither
// exists anywhere in the real SDK (types package, any api_op_*.go input, or
// any doc comment) -- they were invented fields with no wire representation,
// never read anywhere in this codebase either (grep found only their own
// declaration), so they were dead weight rather than a real gap.
type LogAnomalyDetector struct {
	AnomalyDetectorArn    string   `json:"anomalyDetectorArn"`
	DetectorName          string   `json:"detectorName,omitempty"`
	AnomalyDetectorStatus string   `json:"anomalyDetectorStatus,omitempty"`
	EvaluationFrequency   string   `json:"evaluationFrequency,omitempty"`
	FilterPattern         string   `json:"filterPattern,omitempty"`
	KmsKeyID              string   `json:"kmsKeyId,omitempty"`
	LogGroupArnList       []string `json:"logGroupArnList"`
	AnomalyVisibilityTime int64    `json:"anomalyVisibilityTime,omitempty"`
	CreationTimeStamp     int64    `json:"creationTimeStamp"`
	LastModifiedTimeStamp int64    `json:"lastModifiedTimeStamp,omitempty"`
}

// ScheduledQueryDestinationConfig mirrors the real DestinationConfiguration
// shape (aws-sdk-go-v2 types.DestinationConfiguration, types.go:773): a
// scheduled query's results can go to an S3 bucket or a lookup table.
// Neither member is required by the real type (no top-level required check
// in validateDestinationConfiguration, validators.go:2451).
type ScheduledQueryDestinationConfig struct {
	S3Configuration          *ScheduledQueryS3Configuration          `json:"s3Configuration,omitempty"`
	LookupTableConfiguration *ScheduledQueryLookupTableConfiguration `json:"lookupTableConfiguration,omitempty"`
}

// ScheduledQueryS3Configuration mirrors the real S3Configuration shape used
// by ScheduledQueryDestinationConfig.
type ScheduledQueryS3Configuration struct {
	DestinationIdentifier string `json:"destinationIdentifier"`
	RoleArn               string `json:"roleArn"`
	KmsKeyID              string `json:"kmsKeyId,omitempty"`
	OwnerAccountID        string `json:"ownerAccountId,omitempty"`
}

// ScheduledQueryLookupTableConfiguration mirrors the real
// LookupTableConfiguration shape (aws-sdk-go-v2 types.LookupTableConfiguration,
// types.go:1561) used as the alternative DestinationConfiguration member to
// S3Configuration.
type ScheduledQueryLookupTableConfiguration struct {
	Tags        map[string]string `json:"tags,omitempty"`
	TableName   string            `json:"tableName"`
	RoleArn     string            `json:"roleArn"`
	Description string            `json:"description,omitempty"`
	KmsKeyID    string            `json:"kmsKeyId,omitempty"`
}

// ScheduledQuery represents a CloudWatch Logs scheduled query, field-diffed
// against GetScheduledQueryOutput (confirmed via api_op_GetScheduledQuery.go
// and its deserializer). The wire key for the identifying ARN is
// scheduledQueryArn, not arn -- a previous version of this struct used the
// wrong key, so a real SDK client's ScheduledQueryArn field would always
// deserialize empty. A previous version also modeled only 6 of
// GetScheduledQueryOutput's ~20 members; this now covers the full set.
// ScheduleType is not client-settable (CreateScheduledQueryInput/
// UpdateScheduledQueryInput have no scheduleType member) -- every
// backend-created query is CUSTOMER_MANAGED, since AWS_MANAGED queries are
// pre-provisioned by AWS itself, not created through this API.
type ScheduledQuery struct {
	DestinationConfiguration *ScheduledQueryDestinationConfig `json:"destinationConfiguration,omitempty"`
	ScheduleExpression       string                           `json:"scheduleExpression,omitempty"`
	QueryString              string                           `json:"queryString"`
	State                    string                           `json:"state"`
	Name                     string                           `json:"name"`
	Timezone                 string                           `json:"timezone,omitempty"`
	LastExecutionStatus      string                           `json:"lastExecutionStatus,omitempty"`
	ScheduleType             string                           `json:"scheduleType,omitempty"`
	ScheduledQueryArn        string                           `json:"scheduledQueryArn"`
	QueryLanguage            string                           `json:"queryLanguage,omitempty"`
	Description              string                           `json:"description,omitempty"`
	ExecutionRoleArn         string                           `json:"executionRoleArn,omitempty"`
	LogGroupIdentifiers      []string                         `json:"logGroupIdentifiers,omitempty"`
	ScheduleEndTime          int64                            `json:"scheduleEndTime,omitempty"`
	LastUpdatedTime          int64                            `json:"lastUpdatedTime,omitempty"`
	CreationTime             int64                            `json:"creationTime"`
	LastTriggeredTime        int64                            `json:"lastTriggeredTime,omitempty"`
	StartTimeOffset          int64                            `json:"startTimeOffset,omitempty"`
	EndTimeOffset            int64                            `json:"endTimeOffset,omitempty"`
	ScheduleStartTime        int64                            `json:"scheduleStartTime,omitempty"`
}

// AccountPolicy represents a CloudWatch Logs account-level policy,
// field-diffed against aws-sdk-go-v2 types.AccountPolicy: accountId and
// lastUpdatedTime were both missing from a previous revision, so a real
// client's AccountId/LastUpdatedTime fields were always left unpopulated.
type AccountPolicy struct {
	PolicyName        string `json:"policyName"`
	PolicyType        string `json:"policyType"`
	PolicyDocument    string `json:"policyDocument,omitempty"`
	Scope             string `json:"scope,omitempty"`
	SelectionCriteria string `json:"selectionCriteria,omitempty"`
	AccountID         string `json:"accountId,omitempty"`
	LastUpdatedTime   int64  `json:"lastUpdatedTime,omitempty"`
}

// RejectedLogEventsInfo describes log events that were rejected by PutLogEvents.
// Field names/wire keys match aws-sdk-go-v2 types.RejectedLogEventsInfo exactly:
// TooOldLogEventEndIndex (not "...StartIndex") is the exclusive end index of the
// too-old run, mirroring ExpiredLogEventEndIndex's exclusive-end semantics.
type RejectedLogEventsInfo struct {
	TooNewLogEventStartIndex *int32 `json:"tooNewLogEventStartIndex,omitempty"`
	TooOldLogEventEndIndex   *int32 `json:"tooOldLogEventEndIndex,omitempty"`
	ExpiredLogEventEndIndex  *int32 `json:"expiredLogEventEndIndex,omitempty"`
}

// PutLogEventsResult is the result of a PutLogEvents call.
type PutLogEventsResult struct {
	RejectedLogEventsInfo *RejectedLogEventsInfo `json:"rejectedLogEventsInfo,omitempty"`
	NextSequenceToken     string                 `json:"nextSequenceToken"`
}

// MetricTransformation describes how to extract a metric from a log event.
type MetricTransformation struct {
	Dimensions      map[string]string `json:"dimensions,omitempty"`
	DefaultValue    *float64          `json:"defaultValue,omitempty"`
	MetricNamespace string            `json:"metricNamespace"`
	MetricName      string            `json:"metricName"`
	MetricValue     string            `json:"metricValue"`
	Unit            string            `json:"unit,omitempty"`
}

// MetricFilter represents a CloudWatch Logs metric filter.
type MetricFilter struct {
	RetentionInDays       *int32 `json:"retentionInDays,omitempty"`
	FilterPattern         string `json:"filterPattern"`
	FilterName            string `json:"filterName"`
	LogGroupName          string `json:"logGroupName"`
	region                string
	MetricTransformations []MetricTransformation `json:"metricTransformations"`
	CreationTime          int64                  `json:"creationTime"`
}

// MetricFilterMatchRecord represents one event that matched a TestMetricFilter call.
type MetricFilterMatchRecord struct {
	ExtractedValues map[string]string `json:"extractedValues"`
	EventMessage    string            `json:"eventMessage"`
	EventNumber     int64             `json:"eventNumber"`
}

// QueryParameter is a named placeholder substituted into a QueryDefinition's
// query string at execution time (types.QueryParameter).
type QueryParameter struct {
	Name         string `json:"name"`
	DefaultValue string `json:"defaultValue,omitempty"`
	Description  string `json:"description,omitempty"`
}

// QueryDefinition represents a saved CloudWatch Logs Insights query
// definition. Field-diffed against aws-sdk-go-v2 types.QueryDefinition
// (gopherstack-enpq): Parameters (a real, accepted PutQueryDefinitionInput
// member) had no Go field at all, so a real client's Parameters were
// silently dropped on every PutQueryDefinition call and never echoed back
// by DescribeQueryDefinitions. QueryLanguage is also modeled: this backend
// has no query-language detection, so it always reports the default CWLI
// (PutQueryDefinitionInput itself has no queryLanguage member to set it
// from -- confirmed against api_op_PutQueryDefinition.go).
type QueryDefinition struct {
	QueryDefinitionID string           `json:"queryDefinitionId"`
	Name              string           `json:"name"`
	QueryString       string           `json:"queryString"`
	QueryLanguage     string           `json:"queryLanguage,omitempty"`
	LogGroupNames     []string         `json:"logGroupNames,omitempty"`
	Parameters        []QueryParameter `json:"parameters,omitempty"`
	LastModified      int64            `json:"lastModified"`
}

// ResourcePolicy represents a CloudWatch Logs resource policy. Field-diffed
// against aws-sdk-go-v2 types.ResourcePolicy (gopherstack-enpq):
// ResourceArn/PolicyScope/RevisionId/LastUpdatedTime were all missing, so a
// real client's fields were always left unpopulated -- confirmed against
// awsAwsjson11_deserializeDocumentResourcePolicy (deserializers.go), which
// recognizes all four wire keys.
type ResourcePolicy struct {
	PolicyName      string `json:"policyName"`
	PolicyDocument  string `json:"policyDocument"`
	ResourceArn     string `json:"resourceArn,omitempty"`
	PolicyScope     string `json:"policyScope,omitempty"`
	RevisionID      string `json:"revisionId,omitempty"`
	LastUpdatedTime int64  `json:"lastUpdatedTime,omitempty"`
}

// DeliveryDestination represents a CloudWatch Logs delivery destination.
// These json tags describe this backend's own internal snapshot/persistence
// encoding, not the AWS wire shape: the real aws-sdk-go-v2 types.
// DeliveryDestination nests the target resource ARN under a
// deliveryDestinationConfiguration.destinationResourceArn object (not a bare
// string under that key, as TargetArn's tag alone would suggest) and carries
// a separate deliveryDestinationType enum field; Policy is not part of this
// type at all on the wire (it is returned by the dedicated
// GetDeliveryDestinationPolicy operation instead). See
// deliveryDestinationWireShape in handler_deliveries.go for the actual
// AWS-shaped response mapping used by the handlers.
type DeliveryDestination struct {
	CreatedAt               time.Time         `json:"-"`
	Tags                    map[string]string `json:"tags,omitempty"`
	Name                    string            `json:"name"`
	Arn                     string            `json:"arn"`
	OutputFormat            string            `json:"outputFormat,omitempty"`
	TargetArn               string            `json:"deliveryDestinationConfiguration,omitempty"`
	DeliveryDestinationType string            `json:"deliveryDestinationType,omitempty"`
	Policy                  string            `json:"policy,omitempty"`
}

// DeliverySource represents a CloudWatch Logs delivery source.
// Service is not client-supplied (aws-sdk-go-v2 types.PutDeliverySourceInput
// has no such field); AWS derives it server-side from the ARN of the
// resource that is actually sending logs (types.DeliverySource.Service doc:
// "The Amazon Web Services service that is sending logs"), so this backend
// derives it the same way -- see serviceFromARN in deliveries.go.
type DeliverySource struct {
	CreatedAt    time.Time         `json:"-"`
	Tags         map[string]string `json:"tags,omitempty"`
	Name         string            `json:"name"`
	Arn          string            `json:"arn"`
	LogType      string            `json:"logType,omitempty"`
	Service      string            `json:"service,omitempty"`
	ResourceArns []string          `json:"resourceArns,omitempty"`
}

// CWLDestination represents a CloudWatch Logs log routing destination.
type CWLDestination struct {
	CreatedAt       time.Time `json:"-"`
	DestinationName string    `json:"destinationName"`
	TargetArn       string    `json:"targetArn"`
	RoleArn         string    `json:"roleArn"`
	AccessPolicy    string    `json:"accessPolicy,omitempty"`
	Arn             string    `json:"arn"`
}

// IndexPolicy represents a CloudWatch Logs field index policy. Field-diffed
// against aws-sdk-go-v2 types.IndexPolicy (gopherstack-enpq): Source had no
// Go member at all, so a real client's Source field was always left
// unpopulated even though it is always present on the wire. PolicyName is
// correctly omitted here -- the real type's own doc comment says
// "Responses about log group-level field index policies don't have this
// field, because those policies don't have names", and PutIndexPolicy/
// DescribeIndexPolicies (the only two ops that populate this backend's
// index-policy store) are log-group-scoped, never account-scoped.
type IndexPolicy struct {
	LogGroupIdentifier string `json:"logGroupIdentifier"`
	PolicyDocument     string `json:"policyDocument"`
	Source             string `json:"source,omitempty"`
	LastUpdateTime     int64  `json:"lastUpdateTime,omitempty"`
}

// Transformer represents a CloudWatch Logs log transformer.
type Transformer struct {
	CreatedAt          time.Time        `json:"-"`
	LogGroupIdentifier string           `json:"logGroupIdentifier"`
	Processors         []map[string]any `json:"transformerConfig"`
}

// CWLIntegration represents a CloudWatch Logs integration (e.g. OpenSearch).
type CWLIntegration struct {
	CreatedAt                time.Time                 `json:"-"`
	OpenSearchResourceConfig *OpenSearchResourceConfig `json:"openSearchResourceConfig,omitempty"`
	Name                     string                    `json:"integrationName"`
	Type                     string                    `json:"integrationType"`
	Status                   string                    `json:"integrationStatus"`
}

// OpenSearchResourceConfig mirrors types.OpenSearchResourceConfig
// (types.go:1977). DashboardViewerPrincipals/DataSourceRoleArn/RetentionDays
// are required whenever the OpenSearch ResourceConfig branch is used
// (validateOpenSearchResourceConfig, validators.go).
type OpenSearchResourceConfig struct {
	RetentionDays             *int32
	DataSourceRoleArn         string
	ApplicationArn            string
	KmsKeyArn                 string
	DashboardViewerPrincipals []string
}

// GroupingIdentifier is a key-value pair identifying a data-source
// characteristic used to group an aggregate log group summary
// (aws-sdk-go-v2 types.GroupingIdentifier; key format uses dot notation,
// e.g. "dataSource.Name"/"dataSource.Type"/"dataSource.Format").
type GroupingIdentifier struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// AggregateLogGroupSummary describes one bucket of log groups grouped by a
// data-source characteristic (aws-sdk-go-v2 types.AggregateLogGroupSummary,
// confirmed against deserializers.go's
// awsAwsjson11_deserializeDocumentAggregateLogGroupSummary: "groupingIdentifiers"
// + "logGroupCount", not a per-log-group record). A previous revision
// modeled this as per-log-group fields (LogGroupName/LogGroupArn/
// LogGroupClass/StoredBytes/LogEventCount) that do not exist anywhere on the
// real type, so a real client's GroupingIdentifiers/LogGroupCount were
// always empty/nil regardless of what this backend returned. This backend
// has no per-log-group data-source classification (dataSource.Name/Type/
// Format) to group by, so GroupingIdentifiers is honestly left empty rather
// than fabricated; LogGroupCount is real (see gaps in PARITY.md).
type AggregateLogGroupSummary struct {
	GroupingIdentifiers []GroupingIdentifier `json:"groupingIdentifiers"`
	LogGroupCount       int32                `json:"logGroupCount"`
}

// TestTransformerOutput is a single transformed log event result. It mirrors the
// AWS TransformedLogRecord shape, carrying both the original and transformed
// message plus the 1-based event number.
type TestTransformerOutput struct {
	EventMessage            string `json:"eventMessage"`
	TransformedEventMessage string `json:"transformedEventMessage"`
	EventNumber             int64  `json:"eventNumber"`
}

// LookupTable represents a CloudWatch Logs lookup table. Field-diffed against
// aws-sdk-go-v2 types.LookupTable (the metadata shape returned by
// DescribeLookupTables) and GetLookupTableOutput (the full-content shape,
// which additionally carries TableBody). Unlike several other CloudWatch
// Logs "ingest from elsewhere" resources, CreateLookupTable's CSV content
// (TableBody) is supplied directly in the request body -- there is no S3
// reference anywhere in this operation's real input/output shape
// (CreateLookupTableInput.TableBody and UpdateLookupTableInput.TableBody are
// both a plain *string of CSV text, verified against the real SDK's
// serializers.go) -- so this backend stores and parses the real CSV content
// (see lookup_tables.go's parseLookupTableCSV) rather than modeling a
// reference to data it never actually reads.
type LookupTable struct {
	LookupTableArn  string   `json:"lookupTableArn"`
	LookupTableName string   `json:"lookupTableName"`
	Description     string   `json:"description,omitempty"`
	KmsKeyID        string   `json:"kmsKeyId,omitempty"`
	TableBody       string   `json:"tableBody"`
	TableFields     []string `json:"tableFields"`
	RecordsCount    int64    `json:"recordsCount"`
	SizeBytes       int64    `json:"sizeBytes"`
	CreatedAt       int64    `json:"createdAt"`
	LastUpdatedTime int64    `json:"lastUpdatedTime"`
}

// syslogSourceTypeVPCE is the only real aws-sdk-go-v2 types.SyslogSourceType
// enum member ("VPCE"): VPC-endpoint ingestion is the only syslog source this
// API supports today.
const syslogSourceTypeVPCE = "VPCE"

// SyslogConfiguration represents a CloudWatch Logs syslog ingestion
// configuration for a log group. Field-diffed against aws-sdk-go-v2
// types.SyslogConfiguration (createdAt/logGroupArn/sourceType/vpcEndpointId).
// PutSyslogConfigurationInput/DeleteSyslogConfigurationInput both require
// LogGroupIdentifier but only optionally accept VpcEndpointId, so this
// backend models at most one syslog configuration per log group (Put
// replaces), matching the per-log-group-identifier keying this codebase
// already uses for IndexPolicy/Transformer (see indexPolicyKeyFn/
// transformerKeyFn in store_setup.go).
type SyslogConfiguration struct {
	LogGroupIdentifier string `json:"logGroupIdentifier"`
	LogGroupArn        string `json:"logGroupArn"`
	SourceType         string `json:"sourceType"`
	VpcEndpointID      string `json:"vpcEndpointId,omitempty"`
	CreatedAt          int64  `json:"createdAt"`
}

// StorageTier constants match the real aws-sdk-go-v2 types.StorageTier enum
// (StorageTierStandard/StorageTierIntelligentTiering).
const (
	StorageTierStandard           = "STANDARD"
	StorageTierIntelligentTiering = "INTELLIGENT_TIERING"
)

// queryLanguageCWLI is the default query language: PutQueryDefinitionInput
// has no queryLanguage member, so every query definition this backend
// creates is the classic CloudWatch Logs Insights QL (types.QueryLanguageCwli).
const queryLanguageCWLI = "CWLI"

// PolicyScope constants match the real aws-sdk-go-v2 types.PolicyScope enum.
const (
	policyScopeAccount  = "ACCOUNT"
	policyScopeResource = "RESOURCE"
)

// indexSourceLogGroup matches types.IndexSourceLogGroup: PutIndexPolicy/
// DescribeIndexPolicies only ever manage log-group-scoped index policies in
// this backend, so every IndexPolicy they return has this Source. An
// account-wide FIELD_INDEX_POLICY (PutAccountPolicy) does not fall back
// into DescribeIndexPolicies here -- disclosed, not fixed, see PARITY.md.
const indexSourceLogGroup = "LOG_GROUP"

// StorageTierPolicy represents the account-level CloudWatch Logs storage
// tier policy. Field-diffed against aws-sdk-go-v2
// GetStorageTierPolicyOutput/PutStorageTierPolicyOutput: both are
// account-scoped, not per-log-group -- GetStorageTierPolicyInput carries no
// fields at all, and PutStorageTierPolicyInput carries only StorageTier (no
// LogGroupIdentifier) -- so this is a singleton, not attached to any
// individual log group. It is a related-but-distinct axis from LogGroup's
// existing LogGroupClass (STANDARD/INFREQUENT_ACCESS, set per log group at
// CreateLogGroup time, gating feature availability): StorageTier
// (STANDARD/INTELLIGENT_TIERING) instead governs whether CloudWatch Logs
// automatically moves already-ingested data between storage tiers
// account-wide over time to optimize cost. Nothing in the real API wires the
// two together (GetStorageTierPolicyOutput carries no log-group-class
// information and LogGroup carries no storage-tier information), so this
// backend keeps them as two independent concepts rather than inventing a
// dependency between them.
type StorageTierPolicy struct {
	StorageTier     string `json:"storageTier"`
	LastUpdatedTime int64  `json:"lastUpdatedTime,omitempty"`
}
