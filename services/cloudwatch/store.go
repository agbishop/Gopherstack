package cloudwatch

import (
	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	statusTrainedInsufficient = "TRAINED_INSUFFICIENT_DATA"
	metricDataStatusComplete  = "Complete"
	statSum                   = "Sum"
)

const (
	keyAlarmName        = "AlarmName"
	keyAlarmDescription = "AlarmDescription"
	keyAlarmArn         = "AlarmArn"
)

const (
	cwDefaultListMetricsLimit               = 500
	cwDefaultDescribeAlarmsLimit            = 100
	cwDefaultAlarmHistoryLimit              = 100
	cwDefaultDescribeForMetricLimit         = 100
	cwDefaultListDashboardsLimit            = 300
	cwDefaultDescribeAnomalyDetectorLimit   = 100
	cwDefaultDescribeInsightRulesLimit      = 100
	cwDefaultDescribeAlarmContributorsLimit = 100
	cwDefaultListMetricStreamsLimit         = 500
	cwDefaultListAlarmMuteRulesLimit        = 100
	cwDefaultListManagedInsightRulesLimit   = 100
	cwMaxMetricDataPoints                   = 1000 // maximum data points retained per metric
	cwMaxMetricNamesPerNamespace            = 500  // maximum unique metric names per namespace
	cwMaxAlarmHistory                       = 100  // maximum alarm history entries per alarm
	cwMetricRetentionDays                   = 15   // data points older than this are evicted
	cwMaxCompositeEvalDepth                 = 10   // maximum recursion depth for composite alarm evaluation
	// cwMaxMetricDataPerRequest mirrors the AWS CloudWatch PutMetricData hard
	// limit on the number of MetricDatum entries accepted per request (1000).
	cwMaxMetricDataPerRequest = 1000
	// cwMaxTotalMetricRecords is a cluster-wide safety cap on distinct metric time series.
	cwMaxTotalMetricRecords = 10000

	alarmStateAlarm            = "ALARM"
	alarmStateOK               = "OK"
	alarmStateInsufficientData = "INSUFFICIENT_DATA"

	insightRuleStateEnabled = "ENABLED"

	historyTypeStateUpdate         = "StateUpdate"
	historyTypeConfigurationUpdate = "ConfigurationUpdate"
	historyTypeAction              = "Action"

	// alarmTypeLogAlarm is the AlarmType value for log alarms, alongside the
	// existing "MetricAlarm"/"CompositeAlarm" strings used throughout this
	// package for history entries, DescribeAlarms filtering, etc.
	alarmTypeLogAlarm = "LogAlarm"

	// defaultDatasetID is the only dataset identifier real CloudWatch
	// supports today (GetDataset/AssociateDatasetKmsKey/
	// DisassociateDatasetKmsKey all operate on this implicit dataset, which
	// exists for every account in every Region without needing to be
	// created).
	defaultDatasetID = "default"

	// otelEnrichmentSingletonKey is the fixed store key for the single,
	// account-level OTelEnrichmentState row.
	otelEnrichmentSingletonKey = "account"

	// otelEnrichmentStatusRunning/Stopped mirror
	// aws-sdk-go-v2/service/cloudwatch/types.OTelEnrichmentStatus's two real
	// values. Stopped is the default before StartOTelEnrichment is ever called.
	otelEnrichmentStatusRunning = "Running"
	otelEnrichmentStatusStopped = "Stopped"
)

// InMemoryBackend implements StorageBackend using in-memory maps.
// metrics is a two-level map: namespace -> composite-key -> *metricRecord.
// The composite key is produced by metricStorageKey(metricName, dims) so that
// different dimension sets for the same metric name are stored separately.
// metrics and alarmHistory are deliberately left as plain maps rather than
// store.Table -- see the comment block at the top of store_setup.go for why.
// Every other resource field is registered on registry -- see
// registerAllTables in store_setup.go.
type InMemoryBackend struct {
	snsPublisher     SNSPublisher
	firehosePutter   FirehosePutter
	asgExecutor      AutoScalingPolicyExecutor
	ec2Actioner      EC2InstanceActioner
	lambdaInvoker    LambdaInvoker
	registry         *store.Registry
	logAlarms        *store.Table[LogAlarm]
	insightRules     *store.Table[InsightRule]
	metricStreams    *store.Table[MetricStream]
	alarmMuteRules   *store.Table[AlarmMuteRule]
	datasets         *store.Table[Dataset]
	otelEnrichment   *store.Table[OTelEnrichmentState]
	metrics          map[string]map[string]*metricRecord
	dashboards       *store.Table[dashboardRecord]
	anomalyDetectors *store.Table[AnomalyDetector]
	compositeAlarms  *store.Table[CompositeAlarm]
	alarms           *store.Table[MetricAlarm]
	alarmHistory     map[string][]AlarmHistoryItem
	mu               *lockmetrics.RWMutex
	// alarmStateSubscribers holds the generic "notify me when alarm X changes
	// state" hooks registered via SubscribeAlarmStateChange, keyed by AlarmArn
	// then by a subscription id -- distinct from AlarmActions/OKActions/
	// InsufficientDataActions, which only fire ARNs the alarm's own owner
	// configured (gopherstack-x842, gopherstack-9939).
	alarmStateSubscribers map[string]map[uint64]func(newState string)
	region                string
	accountID             string
	// totalMetrics is the running count of distinct metric series across all
	// namespaces, maintained on insert/delete to avoid O(namespaces) walks (#60).
	totalMetrics int
	// alarmHistorySeq is a monotonic counter assigned to each AlarmHistoryItem
	// on append, used as a sort tiebreak alongside Timestamp: alarmHistory is a
	// plain map keyed by alarm name (unordered walk), and real-world history
	// items can share an identical Timestamp, so Timestamp alone is not a
	// unique sort key -- DescribeAlarmHistory's pagination would otherwise drop
	// or duplicate records at a page boundary across two calls.
	alarmHistorySeq uint64
	// alarmSubSeq is a monotonic id assigned to each SubscribeAlarmStateChange
	// registration so its unsubscribe func can find its own entry.
	alarmSubSeq uint64
}

// NewInMemoryBackend creates a new InMemoryBackend with default configuration.
func NewInMemoryBackend() *InMemoryBackend {
	return NewInMemoryBackendWithConfig(config.DefaultAccountID, config.DefaultRegion)
}

// NewInMemoryBackendWithConfig creates a new InMemoryBackend with given account and region.
func NewInMemoryBackendWithConfig(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		accountID:    accountID,
		region:       region,
		metrics:      make(map[string]map[string]*metricRecord),
		alarmHistory: make(map[string][]AlarmHistoryItem),
		registry:     store.NewRegistry(),
		mu:           lockmetrics.New("cloudwatch"),
	}

	registerAllTables(b)

	return b
}

// countTotalMetrics returns the total number of distinct metric time series
// across all namespaces. Uses the running counter (#60) maintained on insert.
// Caller must hold b.mu (at least read lock).
func (b *InMemoryBackend) countTotalMetrics() int {
	return b.totalMetrics
}

// SetSNSPublisher registers an SNS publisher used to fire alarm action notifications.
func (b *InMemoryBackend) SetSNSPublisher(pub SNSPublisher) {
	b.mu.Lock("SetSNSPublisher")
	defer b.mu.Unlock()
	b.snsPublisher = pub
}

// SetLambdaInvoker registers a Lambda invoker used to fire alarm action Lambda invocations.
func (b *InMemoryBackend) SetLambdaInvoker(inv LambdaInvoker) {
	b.mu.Lock("SetLambdaInvoker")
	defer b.mu.Unlock()
	b.lambdaInvoker = inv
}

// SetEC2Actioner registers an EC2 actioner used to execute arn:aws:automate EC2
// alarm actions (stop/terminate/reboot/recover).
func (b *InMemoryBackend) SetEC2Actioner(a EC2InstanceActioner) {
	b.mu.Lock("SetEC2Actioner")
	defer b.mu.Unlock()
	b.ec2Actioner = a
}

// SetAutoScalingExecutor registers an Auto Scaling executor used to run
// scaling-policy alarm actions.
func (b *InMemoryBackend) SetAutoScalingExecutor(e AutoScalingPolicyExecutor) {
	b.mu.Lock("SetAutoScalingExecutor")
	defer b.mu.Unlock()
	b.asgExecutor = e
}

// SetFirehosePutter registers a Firehose backend used to deliver matched
// metric-stream data to each stream's configured delivery stream.
func (b *InMemoryBackend) SetFirehosePutter(fh FirehosePutter) {
	b.mu.Lock("SetFirehosePutter")
	defer b.mu.Unlock()
	b.firehosePutter = fh
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.metrics = make(map[string]map[string]*metricRecord)
	b.alarmHistory = make(map[string][]AlarmHistoryItem)
	b.alarmHistorySeq = 0
	b.totalMetrics = 0
	b.registry.ResetAll()
}
