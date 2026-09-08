package s3

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// notificationConfiguration mirrors the AWS S3 XML notification configuration
// stored in StoredBucket.NotificationConfig.
type notificationConfiguration struct {
	EventBridgeConfiguration *eventBridgeConfiguration `xml:"EventBridgeConfiguration"`
	QueueConfigurations      []queueConfiguration      `xml:"QueueConfiguration"`
	TopicConfigurations      []topicConfiguration      `xml:"TopicConfiguration"`
	LambdaConfigurations     []lambdaConfiguration     `xml:"CloudFunctionConfiguration"`
}

// eventBridgeConfiguration represents the EventBridge notification configuration element.
// Its presence (non-nil) enables delivery of all S3 events to the default EventBridge event bus.
type eventBridgeConfiguration struct{}

type queueConfiguration struct {
	QueueID string             `xml:"Id"`
	Queue   string             `xml:"Queue"`
	Events  []string           `xml:"Event"`
	Filter  notificationFilter `xml:"Filter"`
}

type topicConfiguration struct {
	TopicID string             `xml:"Id"`
	Topic   string             `xml:"Topic"`
	Events  []string           `xml:"Event"`
	Filter  notificationFilter `xml:"Filter"`
}

type lambdaConfiguration struct {
	LambdaID  string             `xml:"Id"`
	CloudFunc string             `xml:"CloudFunction"`
	Events    []string           `xml:"Event"`
	Filter    notificationFilter `xml:"Filter"`
}

// notificationFilter mirrors the S3 <Filter> element in a notification configuration.
type notificationFilter struct {
	S3Key s3KeyFilter `xml:"S3Key"`
}

// s3KeyFilter mirrors the S3 <S3Key> element containing one or more filter rules.
type s3KeyFilter struct {
	Rules []filterRule `xml:"FilterRule"`
}

// filterRule mirrors a single <FilterRule> with a Name (prefix or suffix) and Value.
type filterRule struct {
	Name  string `xml:"Name"`
	Value string `xml:"Value"`
}

// keyMatchesFilter returns true when the object key satisfies all filter rules.
// An empty rule set matches every key. Unknown rule names fail closed.
func keyMatchesFilter(key string, filter notificationFilter) bool {
	for _, rule := range filter.S3Key.Rules {
		switch strings.ToLower(rule.Name) {
		case "prefix":
			if !strings.HasPrefix(key, rule.Value) {
				return false
			}
		case "suffix":
			if !strings.HasSuffix(key, rule.Value) {
				return false
			}
		default:
			// Unknown rule name; fail closed to avoid unintentionally broadening delivery.
			return false
		}
	}

	return true
}

// NotificationDispatcher delivers S3 event notifications to configured targets.
type NotificationDispatcher interface {
	// DispatchObjectCreated sends an s3:ObjectCreated:Put notification for a PutObject operation.
	DispatchObjectCreated(
		ctx context.Context,
		bucket, key, etag string,
		size int64,
		notifXML string,
	)
	// DispatchObjectCopied sends an s3:ObjectCreated:Copy notification for a CopyObject operation.
	DispatchObjectCopied(ctx context.Context, bucket, key, etag string, size int64, notifXML string)
	// DispatchObjectPosted sends an s3:ObjectCreated:Post notification for a
	// browser-style POST Object upload.
	DispatchObjectPosted(ctx context.Context, bucket, key, etag string, size int64, notifXML string)
	// DispatchObjectCompleted sends an s3:ObjectCreated:CompleteMultipartUpload notification.
	DispatchObjectCompleted(
		ctx context.Context,
		bucket, key, etag string,
		size int64,
		notifXML string,
	)
	// DispatchObjectDeleted sends an s3:ObjectRemoved notification for the given object.
	DispatchObjectDeleted(ctx context.Context, bucket, key, notifXML string)
	// DispatchObjectRestorePost sends an s3:ObjectRestore:Post notification for a
	// RestoreObject request (object restore initiation).
	DispatchObjectRestorePost(ctx context.Context, bucket, key, notifXML string)
}

// NotificationTargets holds concrete delivery clients for each supported target type.
type NotificationTargets struct {
	SQSSender            SQSSender
	SNSPublisher         SNSPublisher
	LambdaInvoker        LambdaInvoker
	EventBridgePublisher EventBridgePublisher
}

// SQSSender sends a message body to an SQS queue identified by ARN.
type SQSSender interface {
	SendMessageToQueue(ctx context.Context, queueARN, messageBody string) error
}

// SNSPublisher publishes a message to an SNS topic identified by ARN.
type SNSPublisher interface {
	PublishToTopic(ctx context.Context, topicARN, message, subject string) error
}

// LambdaInvoker invokes a Lambda function by name or ARN with a JSON payload.
type LambdaInvoker interface {
	InvokeFunction(
		ctx context.Context,
		name, invocationType string,
		payload []byte,
	) ([]byte, int, error)
}

// EventBridgePublisher publishes an S3 event to the default EventBridge event bus.
type EventBridgePublisher interface {
	PublishS3Event(ctx context.Context, source, detailType, detail string)
}

// s3EventRecord is the standard AWS S3 event notification record structure.
type s3EventRecord struct {
	EventTime    string          `json:"eventTime"`
	EventSource  string          `json:"eventSource"`
	AwsRegion    string          `json:"awsRegion"`
	EventName    string          `json:"eventName"`
	EventVersion string          `json:"eventVersion"`
	S3           s3EventRecordS3 `json:"s3"`
}

type s3EventRecordS3 struct {
	Bucket          s3EventBucket `json:"bucket"`
	S3SchemaVersion string        `json:"s3SchemaVersion"`
	ConfigurationID string        `json:"configurationId"`
	Object          s3EventObject `json:"object"`
}

type s3EventBucket struct {
	ARN  string `json:"arn"`
	Name string `json:"name"`
}

type s3EventObject struct {
	ETag      string `json:"eTag,omitempty"`
	Key       string `json:"key"`
	Sequencer string `json:"sequencer"`
	Size      int64  `json:"size,omitempty"`
}

// buildS3EventPayload builds the standard S3 event notification JSON payload.
func buildS3EventPayload(
	eventName, configID, region, bucket, key, etag string,
	size int64,
) (string, error) {
	record := s3EventRecord{
		EventVersion: "2.1",
		EventSource:  "aws:s3",
		AwsRegion:    region,
		EventTime:    time.Now().UTC().Format(time.RFC3339Nano),
		EventName:    eventName,
		S3: s3EventRecordS3{
			S3SchemaVersion: "1.0",
			ConfigurationID: configID,
			Bucket: s3EventBucket{
				Name: bucket,
				ARN:  arn.BuildS3(bucket),
			},
			Object: s3EventObject{
				Key:       key,
				ETag:      etag,
				Size:      size,
				Sequencer: fmt.Sprintf("%016X", time.Now().UnixNano()),
			},
		},
	}
	envelope := map[string]any{"Records": []s3EventRecord{record}}

	b, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// ebDetail is the EventBridge event detail for S3 events.
type ebDetail struct {
	Bucket          ebDetailBucket `json:"bucket"`
	Version         string         `json:"version"`
	RequestID       string         `json:"request-id"`
	Requester       string         `json:"requester,omitempty"`
	SourceIPAddress string         `json:"source-ip-address,omitempty"`
	Reason          string         `json:"reason"`
	Object          ebDetailObject `json:"object"`
}

type ebDetailBucket struct {
	Name string `json:"name"`
}

type ebDetailObject struct {
	ETag      string `json:"etag,omitempty"`
	Key       string `json:"key"`
	Sequencer string `json:"sequencer"`
	Size      int64  `json:"size,omitempty"`
}

// buildEventBridgeDetail constructs the EventBridge event detail JSON for an S3 event.
func buildEventBridgeDetail(bucket, key, etag, reason string, size int64) (string, error) {
	detail := ebDetail{
		Version: "0",
		Bucket:  ebDetailBucket{Name: bucket},
		Object: ebDetailObject{
			Key:       key,
			ETag:      etag,
			Size:      size,
			Sequencer: fmt.Sprintf("%016X", time.Now().UnixNano()),
		},
		RequestID: uuid.New().String(),
		Reason:    reason,
	}

	b, err := json.Marshal(detail)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

// detailTypeFromEventName maps an S3 event name to the EventBridge detail-type string.
func detailTypeFromEventName(eventName string) string {
	switch {
	case strings.HasPrefix(eventName, "s3:ObjectCreated:"):
		return "Object Created"
	case strings.HasPrefix(eventName, "s3:ObjectRemoved:"):
		return "Object Deleted"
	case strings.HasPrefix(eventName, "s3:ObjectRestore:"):
		return "Object Restore Initiated"
	default:
		return "S3 Event"
	}
}

// reasonFromEventName derives the AWS-style operation reason from an S3 event name
// (e.g. "s3:ObjectCreated:Put" → "PutObject").
func reasonFromEventName(eventName string) string {
	const minEventParts = 3 // S3 event names have format "s3:Category:Operation"

	parts := strings.Split(eventName, ":")
	if len(parts) < minEventParts {
		return eventName
	}

	switch parts[2] {
	case "Put":
		return "PutObject"
	case "Copy":
		return "CopyObject"
	case opCompleteMultipartUpload:
		return opCompleteMultipartUpload
	case "Delete", "DeleteMarkerCreated":
		return "DeleteObject"
	default:
		return parts[2]
	}
}

// eventMatches returns true when the event name matches the rule event pattern.
// AWS event patterns use wildcards like "s3:ObjectCreated:*".
func eventMatches(pattern, eventName string) bool {
	if pattern == eventName {
		return true
	}
	// Support trailing wildcard: "s3:ObjectCreated:*" matches "s3:ObjectCreated:Put"
	if before, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(eventName, before)
	}

	return false
}

// inMemoryNotificationDispatcher delivers S3 event notifications using in-process targets.
type inMemoryNotificationDispatcher struct {
	targets *NotificationTargets
	region  string
}

// NewNotificationDispatcher creates a NotificationDispatcher that delivers
// events to the provided in-process targets (SQS, SNS, Lambda).
func NewNotificationDispatcher(targets *NotificationTargets, region string) NotificationDispatcher {
	return &inMemoryNotificationDispatcher{targets: targets, region: region}
}

func (d *inMemoryNotificationDispatcher) DispatchObjectCreated(
	ctx context.Context,
	bucket, key, etag string,
	size int64,
	notifXML string,
) {
	d.dispatch(ctx, "s3:ObjectCreated:Put", bucket, key, etag, size, notifXML)
}

func (d *inMemoryNotificationDispatcher) DispatchObjectCopied(
	ctx context.Context,
	bucket, key, etag string,
	size int64,
	notifXML string,
) {
	d.dispatch(ctx, "s3:ObjectCreated:Copy", bucket, key, etag, size, notifXML)
}

func (d *inMemoryNotificationDispatcher) DispatchObjectPosted(
	ctx context.Context,
	bucket, key, etag string,
	size int64,
	notifXML string,
) {
	d.dispatch(ctx, "s3:ObjectCreated:Post", bucket, key, etag, size, notifXML)
}

func (d *inMemoryNotificationDispatcher) DispatchObjectCompleted(
	ctx context.Context,
	bucket, key, etag string,
	size int64,
	notifXML string,
) {
	d.dispatch(ctx, "s3:ObjectCreated:CompleteMultipartUpload", bucket, key, etag, size, notifXML)
}

func (d *inMemoryNotificationDispatcher) DispatchObjectDeleted(
	ctx context.Context,
	bucket, key, notifXML string,
) {
	d.dispatch(ctx, "s3:ObjectRemoved:Delete", bucket, key, "", 0, notifXML)
}

func (d *inMemoryNotificationDispatcher) DispatchObjectRestorePost(
	ctx context.Context,
	bucket, key, notifXML string,
) {
	d.dispatch(ctx, "s3:ObjectRestore:Post", bucket, key, "", 0, notifXML)
}

func (d *inMemoryNotificationDispatcher) dispatch(
	ctx context.Context,
	eventName, bucket, key, etag string,
	size int64,
	notifXML string,
) {
	if notifXML == "" {
		return
	}

	var cfg notificationConfiguration
	if err := xml.Unmarshal([]byte(notifXML), &cfg); err != nil {
		return
	}

	for _, qc := range cfg.QueueConfigurations {
		d.dispatchToQueue(ctx, qc, eventName, bucket, key, etag, size)
	}

	for _, tc := range cfg.TopicConfigurations {
		d.dispatchToTopic(ctx, tc, eventName, bucket, key, etag, size)
	}

	for _, lc := range cfg.LambdaConfigurations {
		d.dispatchToLambda(ctx, lc, eventName, bucket, key, etag, size)
	}

	if cfg.EventBridgeConfiguration != nil {
		d.dispatchToEventBridge(ctx, eventName, bucket, key, etag, size)
	}
}

func (d *inMemoryNotificationDispatcher) dispatchToQueue(
	ctx context.Context,
	qc queueConfiguration,
	eventName, bucket, key, etag string,
	size int64,
) {
	if !matchesAnyEvent(qc.Events, eventName) || !keyMatchesFilter(key, qc.Filter) {
		return
	}

	payload, err := buildS3EventPayload(eventName, qc.QueueID, d.region, bucket, key, etag, size)
	if err != nil {
		return
	}

	if d.targets != nil && d.targets.SQSSender != nil {
		_ = d.targets.SQSSender.SendMessageToQueue(ctx, qc.Queue, payload)
	}
}

func (d *inMemoryNotificationDispatcher) dispatchToTopic(
	ctx context.Context,
	tc topicConfiguration,
	eventName, bucket, key, etag string,
	size int64,
) {
	if !matchesAnyEvent(tc.Events, eventName) || !keyMatchesFilter(key, tc.Filter) {
		return
	}

	payload, err := buildS3EventPayload(eventName, tc.TopicID, d.region, bucket, key, etag, size)
	if err != nil {
		return
	}

	if d.targets != nil && d.targets.SNSPublisher != nil {
		_ = d.targets.SNSPublisher.PublishToTopic(ctx, tc.Topic, payload, "S3Notification")
	}
}

func (d *inMemoryNotificationDispatcher) dispatchToLambda(
	ctx context.Context,
	lc lambdaConfiguration,
	eventName, bucket, key, etag string,
	size int64,
) {
	if !matchesAnyEvent(lc.Events, eventName) || !keyMatchesFilter(key, lc.Filter) {
		return
	}

	payload, err := buildS3EventPayload(eventName, lc.LambdaID, d.region, bucket, key, etag, size)
	if err != nil {
		return
	}

	if d.targets != nil && d.targets.LambdaInvoker != nil {
		_, _, _ = d.targets.LambdaInvoker.InvokeFunction(
			ctx,
			lc.CloudFunc,
			"Event",
			[]byte(payload),
		)
	}
}

// matchesAnyEvent returns true if eventName matches any of the given event patterns.
func matchesAnyEvent(patterns []string, eventName string) bool {
	for _, p := range patterns {
		if eventMatches(p, eventName) {
			return true
		}
	}

	return false
}

func (d *inMemoryNotificationDispatcher) dispatchToEventBridge(
	ctx context.Context,
	eventName, bucket, key, etag string,
	size int64,
) {
	if d.targets == nil || d.targets.EventBridgePublisher == nil {
		return
	}

	detailType := detailTypeFromEventName(eventName)
	reason := reasonFromEventName(eventName)

	detail, err := buildEventBridgeDetail(bucket, key, etag, reason, size)
	if err != nil {
		return
	}

	d.targets.EventBridgePublisher.PublishS3Event(ctx, "aws.s3", detailType, detail)
}
