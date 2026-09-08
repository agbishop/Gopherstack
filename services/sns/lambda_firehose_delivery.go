package sns

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/events"
)

// snsLambdaEnvelope is the JSON payload delivered to a Lambda function from SNS.
// It wraps the published message in an AWS-spec Records array.
type snsLambdaRecord struct {
	Sns snsLambdaNotification `json:"Sns"`
	// EventSource and EventVersion match the AWS SNS → Lambda envelope spec.
	EventVersion         string `json:"EventVersion"`
	EventSource          string `json:"EventSource"`
	EventSubscriptionArn string `json:"EventSubscriptionArn"`
}

type snsLambdaNotification struct {
	MessageAttributes map[string]snsLambdaMessageAttr `json:"MessageAttributes"`
	Type              string                          `json:"Type"`
	MessageID         string                          `json:"MessageId"`
	TopicArn          string                          `json:"TopicArn"`
	Subject           string                          `json:"Subject,omitempty"`
	Message           string                          `json:"Message"`
	Timestamp         string                          `json:"Timestamp"`
	SignatureVersion  string                          `json:"SignatureVersion"`
	Signature         string                          `json:"Signature"`
	SigningCertURL    string                          `json:"SigningCertUrl"`
	UnsubscribeURL    string                          `json:"UnsubscribeUrl"`
}

type snsLambdaMessageAttr struct {
	Type  string `json:"Type"`
	Value string `json:"Value"`
}

// subscriptionUnsubscribeURL builds the AWS-shape unsubscribe link embedded in
// notification envelopes: https://sns.<region>.amazonaws.com/?Action=Unsubscribe&SubscriptionArn=<arn>.
// The region is derived from topicARN; it falls back to "us-east-1" for a
// malformed ARN so the URL is always well-formed.
func subscriptionUnsubscribeURL(topicARN, subscriptionARN string) string {
	region := arnRegion(topicARN)
	if region == "" {
		region = "us-east-1"
	}

	return "https://sns." + region + ".amazonaws.com/?Action=Unsubscribe&SubscriptionArn=" + subscriptionARN
}

// buildLambdaPayload constructs the SNS → Lambda invocation payload per the AWS spec.
// The Timestamp/Signature/SigningCertURL are the real values computed once per
// Publish call (see buildPublishedEvent) rather than fabricated per-delivery, so a
// Lambda function that verifies the SNS signature (as AWS recommends) succeeds
// against the same envelope other channels (SQS, HTTP/HTTPS) receive.
func buildLambdaPayload(
	ev *events.SNSPublishedEvent,
	sub events.SNSSubscriptionSnapshot,
) []byte {
	attrs := make(map[string]snsLambdaMessageAttr, len(ev.Attributes))
	for k, v := range ev.Attributes {
		attrs[k] = snsLambdaMessageAttr{Type: v.DataType, Value: v.StringValue}
	}

	record := snsLambdaRecord{
		EventVersion:         "1.0",
		EventSource:          "aws:sns",
		EventSubscriptionArn: sub.SubscriptionARN,
		Sns: snsLambdaNotification{
			Type:              messageTypeNotification,
			MessageID:         ev.MessageID,
			TopicArn:          ev.TopicARN,
			Subject:           ev.Subject,
			Message:           ev.Message,
			Timestamp:         ev.Timestamp,
			SignatureVersion:  resolveSignatureVersion(ev.SignatureVersion),
			Signature:         ev.Signature,
			SigningCertURL:    ev.SigningCertURL,
			UnsubscribeURL:    subscriptionUnsubscribeURL(ev.TopicARN, sub.SubscriptionARN),
			MessageAttributes: attrs,
		},
	}

	payload, err := json.Marshal(map[string]any{"Records": []any{record}})
	if err != nil {
		return []byte(`{"Records":[]}`)
	}

	return payload
}

// buildFirehoseEnvelope wraps the published message in the same SNS Notification
// JSON envelope AWS sends to SQS/HTTP subscribers, for delivery to a Firehose
// subscription whose RawMessageDelivery attribute is false (the AWS default).
// Reuses the Timestamp/Signature/SigningCertURL computed once for this publish.
func buildFirehoseEnvelope(ev *events.SNSPublishedEvent, sub events.SNSSubscriptionSnapshot) []byte {
	env := snsHTTPNotification{
		Type:             messageTypeNotification,
		MessageID:        ev.MessageID,
		TopicArn:         ev.TopicARN,
		Message:          ev.Message,
		Timestamp:        ev.Timestamp,
		SignatureVersion: resolveSignatureVersion(ev.SignatureVersion),
		Signature:        ev.Signature,
		SigningCertURL:   ev.SigningCertURL,
		UnsubscribeURL:   subscriptionUnsubscribeURL(ev.TopicARN, sub.SubscriptionARN),
	}
	if ev.Subject != "" {
		env.Subject = ev.Subject
	}

	enc, err := json.Marshal(env)
	if err != nil {
		return []byte(ev.Message)
	}

	return enc
}

// deliverToLambdaSubscriptions invokes each Lambda-protocol subscription endpoint.
// On invocation failure, the message is forwarded to the subscription DLQ when
// a RedrivePolicy is configured and a SQSSender is wired.
func (b *InMemoryBackend) deliverToLambdaSubscriptions(ev *events.SNSPublishedEvent) {
	var (
		lambda               LambdaInvoker
		sqsSender            SQSSender
		topicEffectivePolicy string
	)

	func() {
		b.mu.RLock("lambda-topic-policy")
		defer b.mu.RUnlock()

		lambda = b.lambdaBackend
		sqsSender = b.sqsSender

		if topic, ok := b.topics.Get(ev.TopicARN); ok {
			topicEffectivePolicy = topic.Attributes["EffectiveDeliveryPolicy"]
		}
	}()

	if lambda == nil {
		return
	}

	for _, sub := range ev.Subscriptions {
		if sub.Protocol != protocolLambda {
			continue
		}

		numRetries := getRetryConfig(topicEffectivePolicy, sub.DeliveryPolicy, protocolLambda)
		payload := buildLambdaPayload(ev, sub)
		var err error

		for i := 0; i <= numRetries; i++ {
			_, _, err = lambda.InvokeFunction(b.svcCtx, sub.Endpoint, snsLambdaInvocationType, payload)
			if err == nil {
				b.logDeliveryStatus(b.svcCtx, ev.TopicARN, protocolLambda, sub.Endpoint, "SUCCESS", nil)

				break
			}
		}

		if err != nil {
			b.logDeliveryStatus(b.svcCtx, ev.TopicARN, protocolLambda, sub.Endpoint, "FAILURE", err)
			if sub.RedrivePolicy != "" && sqsSender != nil {
				sendLambdaDLQ(b.svcCtx, sqsSender, sub.RedrivePolicy, ev.Message)
			}
		}
	}
}

// deliverToFirehoseSubscriptions puts each Firehose-protocol subscription message as a batch record.
// When the subscription's RawMessageDelivery attribute is false (the AWS default), the record is
// the full SNS Notification JSON envelope, matching real AWS SNS→Firehose delivery; RawMessageDelivery=true
// delivers the bare message body. On delivery failure, the message is forwarded to the subscription DLQ
// when a RedrivePolicy is configured and a SQSSender is wired — matching the HTTP/HTTPS and Lambda paths.
func (b *InMemoryBackend) deliverToFirehoseSubscriptions(ev *events.SNSPublishedEvent) {
	var (
		firehose             FirehosePutter
		sqsSender            SQSSender
		topicEffectivePolicy string
	)

	func() {
		b.mu.RLock("firehose-topic-policy")
		defer b.mu.RUnlock()

		firehose = b.firehoseBackend
		sqsSender = b.sqsSender

		if topic, ok := b.topics.Get(ev.TopicARN); ok {
			topicEffectivePolicy = topic.Attributes["EffectiveDeliveryPolicy"]
		}
	}()

	if firehose == nil {
		return
	}

	for _, sub := range ev.Subscriptions {
		if sub.Protocol != protocolFirehose {
			continue
		}

		b.deliverFirehoseSubscription(ev, sub, firehose, sqsSender, topicEffectivePolicy)
	}
}

// deliverFirehoseSubscription delivers ev to a single Firehose-protocol
// subscription, retrying per the effective delivery policy and forwarding to
// the subscription DLQ on final failure. Extracted from
// deliverToFirehoseSubscriptions to reduce cognitive complexity.
func (b *InMemoryBackend) deliverFirehoseSubscription(
	ev *events.SNSPublishedEvent,
	sub events.SNSSubscriptionSnapshot,
	firehose FirehosePutter,
	sqsSender SQSSender,
	topicEffectivePolicy string,
) {
	streamName := firehoseStreamNameFromARN(sub.Endpoint)
	if streamName == "" {
		return
	}

	record := []byte(ev.Message)
	if !sub.RawMessageDelivery {
		record = buildFirehoseEnvelope(ev, sub)
	}

	numRetries := getRetryConfig(topicEffectivePolicy, sub.DeliveryPolicy, protocolFirehose)

	var err error

	for i := 0; i <= numRetries; i++ {
		_, err = firehose.PutRecordBatch(streamName, [][]byte{record})
		if err == nil {
			b.logDeliveryStatus(b.svcCtx, ev.TopicARN, protocolFirehose, sub.Endpoint, "SUCCESS", nil)

			return
		}
	}

	b.logDeliveryStatus(b.svcCtx, ev.TopicARN, protocolFirehose, sub.Endpoint, "FAILURE", err)

	if sub.RedrivePolicy != "" && sqsSender != nil {
		sendLambdaDLQ(b.svcCtx, sqsSender, sub.RedrivePolicy, string(record))
	}
}

// firehoseStreamNameFromARN extracts the delivery stream name from a Firehose ARN.
// ARN format: arn:aws:firehose:<region>:<account>:deliverystream/<name>.
func firehoseStreamNameFromARN(endpoint string) string {
	const prefix = "deliverystream/"
	if _, after, ok := strings.Cut(endpoint, prefix); ok {
		return after
	}

	// Fall back to last path segment (URL-style endpoint).
	parts := strings.Split(endpoint, "/")

	return parts[len(parts)-1]
}

// deliverToSMSSubscriptions delivers a topic publish to all SMS-protocol subscriptions.
// Each delivery is recorded via PublishSMS so it is observable through DrainSMSDeliveries.
// Opt-out and sandbox verification checks are enforced by PublishSMS; errors are silently
// dropped (best-effort, matching AWS SNS behaviour for non-critical delivery channels).
func (b *InMemoryBackend) deliverToSMSSubscriptions(ev *events.SNSPublishedEvent) {
	for _, sub := range ev.Subscriptions {
		if sub.Protocol != protocolSMS {
			continue
		}
		_, _ = b.PublishSMS(sub.Endpoint, ev.Message)
	}
}

// deliverToApplicationSubscriptions delivers a topic publish to all application-protocol
// subscriptions (mobile push platform endpoints). Enabled endpoints generate a recorded
// ApplicationDelivery observable via DrainApplicationDeliveries. Disabled or missing
// endpoints are silently skipped (best-effort), matching AWS SNS behaviour.
func (b *InMemoryBackend) deliverToApplicationSubscriptions(ev *events.SNSPublishedEvent) {
	for _, sub := range ev.Subscriptions {
		if sub.Protocol != protocolApplication {
			continue
		}

		var enabled bool

		func() {
			b.mu.RLock("deliverToApplicationSubscriptions")
			defer b.mu.RUnlock()

			ep, exists := b.platformEndpoints.Get(sub.Endpoint)
			enabled = exists && ep.Attributes["Enabled"] != boolFalseStr
		}()

		if !enabled {
			continue
		}

		msgID := uuid.NewString()

		func() {
			b.mu.Lock("deliverToApplicationSubscriptions-record")
			defer b.mu.Unlock()

			b.applicationDeliveries = appendBounded(b.applicationDeliveries, ApplicationDelivery{
				EndpointARN: sub.Endpoint,
				Message:     ev.Message,
				MessageID:   msgID,
			}, maxRecordedDeliveries)
		}()
	}
}

// sendLambdaDLQ forwards a failed Lambda delivery to the DLQ configured in redrivePolicy.
// It is a no-op when the SQSSender is nil or the policy cannot be parsed.
func sendLambdaDLQ(ctx context.Context, sender SQSSender, redrivePolicy, body string) {
	var policy struct {
		DeadLetterTargetArn string `json:"deadLetterTargetArn"`
	}
	if err := json.Unmarshal([]byte(redrivePolicy), &policy); err != nil || policy.DeadLetterTargetArn == "" {
		return
	}
	_ = sender.SendMessageToQueue(ctx, policy.DeadLetterTargetArn, body)
}
