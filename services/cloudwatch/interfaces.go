package cloudwatch

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// SNSPublisher can publish a message to an SNS topic by ARN.
type SNSPublisher interface {
	PublishToTopic(topicARN, message string) error
}

// LambdaInvoker can invoke a Lambda function by ARN or name.
type LambdaInvoker interface {
	InvokeFunction(
		ctx context.Context,
		name string,
		invocationType string,
		payload []byte,
	) ([]byte, int, error)
}

// EC2InstanceActioner executes EC2 alarm actions (arn:aws:automate:<region>:ec2:*)
// against the instances named by the alarm's InstanceId dimension.
type EC2InstanceActioner interface {
	StopInstances(instanceIDs []string) error
	TerminateInstances(instanceIDs []string) error
	RebootInstances(instanceIDs []string) error
}

// AutoScalingPolicyExecutor executes an Auto Scaling scaling-policy alarm action
// (arn:aws:autoscaling:...:scalingPolicy:...).
type AutoScalingPolicyExecutor interface {
	ExecuteScalingPolicy(autoScalingGroupName, policyName string) error
}

// FirehosePutter is the subset of Firehose operations a metric stream needs to
// deliver matched metric data to its configured delivery stream.
type FirehosePutter interface {
	PutRecordBatch(ctx context.Context, streamName string, records [][]byte) (int, error)
}

// StorageBackend is the interface for the CloudWatch in-memory store.
type StorageBackend interface {
	PutMetricData(namespace string, data []MetricDatum) error
	GetMetricStatistics(
		namespace, metricName string,
		dimensions []Dimension,
		startTime, endTime time.Time,
		period int32,
		statistics []string,
		extendedStatistics []string,
	) ([]Datapoint, error)
	GetMetricData(
		queries []MetricDataQuery,
		startTime, endTime time.Time,
	) ([]MetricDataResult, error)
	ListMetrics(
		namespace, metricName string,
		dimensions []Dimension,
		recentlyActive, nextToken string,
		maxResults int,
	) (page.Page[Metric], error)
	PutMetricAlarm(alarm *MetricAlarm) error
	PutCompositeAlarm(alarm *CompositeAlarm) error
	PutLogAlarm(alarm *LogAlarm) error
	DescribeAlarms(
		alarmNames []string,
		alarmTypes []string,
		alarmNamePrefix, stateValue, nextToken string,
		maxRecords int,
		actionPrefix, childrenOfAlarmName, parentsOfAlarmName string,
	) (page.Page[MetricAlarm], page.Page[CompositeAlarm], page.Page[LogAlarm], error)
	DescribeAlarmsForMetric(
		namespace, metricName string,
		dimensions []Dimension,
		alarmNames []string,
		nextToken string,
		maxRecords int,
	) (page.Page[MetricAlarm], error)
	DescribeAlarmHistory(
		alarmName string, alarmTypes []string, historyItemType, nextToken, scanBy string,
		startDate, endDate time.Time,
		maxRecords int,
	) (page.Page[AlarmHistoryItem], error)
	DeleteAlarms(alarmNames []string) error
	SetAlarmState(
		ctx context.Context,
		alarmName, stateValue, stateReason, stateReasonData string,
	) error
	EnableAlarmActions(alarmNames []string) error
	DisableAlarmActions(alarmNames []string) error
	PutDashboard(name, body string) ([]DashboardValidationMessage, error)
	GetDashboard(name string) (DashboardEntry, string, error)
	ListDashboards(prefix, nextToken string) (page.Page[DashboardEntry], error)
	DeleteDashboards(names []string) error
	PutAlarmMuteRule(rule *AlarmMuteRule) error
	DeleteAlarmMuteRule(name string) error
	GetAlarmMuteRule(name string) (*AlarmMuteRule, error)
	PutAnomalyDetector(detector *AnomalyDetector) error
	DeleteAnomalyDetector(namespace, metricName, stat string, dims []Dimension) error
	DescribeAnomalyDetectors(
		namespace, metricName, nextToken string,
		maxResults int,
	) (page.Page[AnomalyDetector], error)
	DeleteInsightRules(ruleNames []string) ([]InsightRuleFailure, error)
	PutInsightRule(rule *InsightRule) error
	GetInsightRule(name string) (*InsightRule, error)
	DescribeInsightRules(nextToken string, maxResults int) (page.Page[InsightRule], error)
	DisableInsightRules(ruleNames []string) ([]InsightRuleFailure, error)
	EnableInsightRules(ruleNames []string) ([]InsightRuleFailure, error)
	PutMetricStream(stream *MetricStream) error
	GetMetricStream(name string) (*MetricStream, error)
	ListMetricStreams(nextToken string, maxResults int) (page.Page[MetricStream], error)
	DeleteMetricStream(name string) error
	DescribeAlarmContributors(alarmName, nextToken string) (page.Page[AlarmContributor], error)
	StartMetricStreams(names []string) error
	StopMetricStreams(names []string) error
	ListAlarmMuteRules(
		nextToken string,
		maxResults int,
		alarmName string,
		statuses []string,
	) (page.Page[AlarmMuteRule], error)
	ListManagedInsightRules(
		resourceARN, nextToken string,
		maxResults int,
	) (page.Page[InsightRule], error)
	GetDataset(identifier string) (Dataset, error)
	AssociateDatasetKmsKey(identifier, kmsKeyArn string) error
	DisassociateDatasetKmsKey(identifier string) error
	GetOTelEnrichment() (string, error)
	StartOTelEnrichment() error
	StopOTelEnrichment() error
}
