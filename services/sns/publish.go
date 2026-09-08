package sns

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/events"
)

// buildMessageResolver returns a function that picks the correct message body for a given protocol,
// respecting MessageStructure "json" per-protocol map when provided.
func buildMessageResolver(defaultMsg string, perProtocol map[string]string) func(string) string {
	return func(protocol string) string {
		if perProtocol == nil {
			return defaultMsg
		}

		if msg, ok := perProtocol[protocol]; ok {
			return msg
		}

		if msg, ok := perProtocol["default"]; ok {
			return msg
		}

		return defaultMsg
	}
}

// collectPublishTargets scans b.subscriptions for a given topicArn and returns
// subscription snapshots and HTTP/HTTPS deliveries to dispatch.
// Must be called with at least RLock held.
func (b *InMemoryBackend) collectPublishTargets(
	topicArn, subject string,
	resolveMsg func(string) string,
	attrs map[string]MessageAttribute,
) publishTargets {
	var out publishTargets

	// The topic is guaranteed to exist here: every caller holds at least
	// b.mu.RLock and has already validated topicArn via b.topics.Get/Has
	// before calling collectPublishTargets (see Publish). Looked up once
	// (rather than per-subscription) since store.Table.Get returns (v, ok)
	// and cannot be inlined into the httpDelivery literal below the way the
	// old raw map index could.
	var topicEffectivePolicy, sigVersion string
	if topic, ok := b.topics.Get(topicArn); ok {
		topicEffectivePolicy = topic.Attributes["EffectiveDeliveryPolicy"]
		sigVersion = resolveSignatureVersion(topic.Attributes[attrSignatureVersion])
	}

	for _, sub := range b.subscriptionsByTopic.Get(topicArn) {
		// Resolve the per-protocol message body for this subscription.
		// This must happen before filter evaluation when FilterPolicyScope=MessageBody,
		// because the body itself is the subject of the filter.
		msg := resolveMsg(sub.Protocol)

		// Apply filter policy. When FilterPolicyScope is "MessageBody", the filter
		// is evaluated against the message body parsed as a JSON object. The default
		// scope "MessageAttributes" (or unset) evaluates against message attributes.
		if sub.FilterPolicyScope == "MessageBody" {
			if !matchesFilterPolicyMessageBody(sub.parsedFilterPolicy, msg) {
				continue
			}
		} else {
			if !matchesParsedFilterPolicy(sub.parsedFilterPolicy, attrs) {
				continue
			}
		}

		if sub.Protocol == protocolHTTP || sub.Protocol == protocolHTTPS {
			out.httpDeliveries = append(out.httpDeliveries, httpDelivery{
				endpoint:             sub.Endpoint,
				body:                 msg,
				subject:              subject,
				topicARN:             topicArn,
				subscriptionARN:      sub.SubscriptionArn,
				rawDelivery:          sub.RawMessageDelivery,
				redrivePolicy:        sub.RedrivePolicy,
				deliveryPolicy:       sub.DeliveryPolicy,
				topicEffectivePolicy: topicEffectivePolicy,
				signatureVersion:     sigVersion,
				sqsSender:            b.sqsSender,
			})
		}

		// Email and email-json subscriptions have no network sink in a simulator;
		// record the delivery so it is observable (AWS would place it in an inbox).
		// Pending (unconfirmed) subscriptions are skipped, matching AWS which does
		// not deliver until the recipient confirms.
		if (sub.Protocol == protocolEmail || sub.Protocol == protocolEmailJSON) &&
			!sub.PendingConfirmation {
			out.emailDeliveries = append(out.emailDeliveries, EmailDelivery{
				EndpointEmail: sub.Endpoint,
				Protocol:      sub.Protocol,
				Subject:       subject,
				Message:       msg,
			})
		}

		out.subs = append(out.subs, events.SNSSubscriptionSnapshot{
			SubscriptionARN:    sub.SubscriptionArn,
			Protocol:           sub.Protocol,
			Endpoint:           sub.Endpoint,
			FilterPolicy:       sub.FilterPolicy,
			RawMessageDelivery: sub.RawMessageDelivery,
			RedrivePolicy:      sub.RedrivePolicy,
		})
	}

	return out
}

// Publish publishes a message to a topic and returns the message ID.
// HTTP/HTTPS subscriptions each receive an asynchronous best-effort delivery
// validateStructuredMessage validates a MessageStructure=json payload.
// Returns nil for non-json messageStructure values.
func validateStructuredMessage(message, messageStructure string) error {
	if messageStructure != "json" {
		return nil
	}

	var pm map[string]string
	if err := json.Unmarshal([]byte(message), &pm); err != nil {
		return fmt.Errorf(
			"%w: Invalid JSON in Message when MessageStructure is json: %s",
			ErrInvalidParameter,
			err.Error(),
		)
	}

	if _, ok := pm["default"]; !ok {
		return fmt.Errorf(
			"%w: Message must contain a 'default' key when MessageStructure is json",
			ErrInvalidParameter,
		)
	}

	return nil
}

// parsePerProtocolMessages parses a MessageStructure=json payload into a
// per-protocol map. Returns nil for non-json messageStructure values.
// Callers must have already validated the message with validateStructuredMessage.
func parsePerProtocolMessages(message, messageStructure string) map[string]string {
	if messageStructure != "json" {
		return nil
	}

	var pm map[string]string
	if err := json.Unmarshal([]byte(message), &pm); err != nil {
		return nil
	}

	return pm
}

// validatePublishMessage checks message size, subject format, structure, and
// attribute constraints before any backend lock is acquired.
func validatePublishMessage(
	message, subject, messageStructure string,
	attrs map[string]MessageAttribute,
) error {
	// AWS SNS counts the message body plus every attribute name + type + value
	// toward the 256 KiB cap.
	totalSize := len(message)
	for name, a := range attrs {
		totalSize += len(name) + len(a.DataType) + len(a.StringValue)
	}

	if totalSize > maxMessageSizeBytes {
		return fmt.Errorf(
			"%w: Message size exceeds SNS limit of %d bytes",
			ErrInvalidParameter,
			maxMessageSizeBytes,
		)
	}

	// AWS SNS rejects subjects longer than 100 characters or containing non-ASCII
	// printable characters (control characters, high-byte runes).
	if subject != "" {
		if len(subject) > maxSubjectLen {
			return fmt.Errorf(
				"%w: Subject must be no longer than %d characters",
				ErrInvalidParameter,
				maxSubjectLen,
			)
		}

		for _, r := range subject {
			if r < 0x20 || r > 0x7E {
				return fmt.Errorf(
					"%w: Subject contains invalid characters; must be printable ASCII",
					ErrInvalidParameter,
				)
			}
		}
	}

	if err := validateStructuredMessage(message, messageStructure); err != nil {
		return err
	}

	return validateMessageAttributes(attrs)
}

// dispatchHTTPDeliveries schedules asynchronous HTTP/HTTPS deliveries for each
// entry in deliveries. The closing check is evaluated once so that either all
// deliveries for a given Publish call are scheduled or none are, avoiding
// partial delivery when shutdown is in progress.
func (b *InMemoryBackend) dispatchHTTPDeliveries(deliveries []httpDelivery, client *http.Client) {
	if b.closing.Load() {
		return
	}

	ctx := b.svcCtx
	for _, d := range deliveries {
		b.deliveryWg.Go(func() {
			select {
			case b.workerSem <- struct{}{}:
				defer func() { <-b.workerSem }()
				deliverHTTPWithMeta(ctx, d, client, b)
			case <-ctx.Done():
				// Service is shutting down; drop this delivery rather than
				// blocking indefinitely on a full semaphore.
			}
		})
	}
}

// buildPublishedEvent constructs the SNSPublishedEvent broadcast to every
// non-HTTP delivery channel for a single Publish call: the SQS emitter, and
// the Lambda/Firehose/SMS/Application delivery fan-out below. The timestamp,
// RSA signature, and signing certificate URL are computed exactly once here so
// every channel carries an identical, verifiable notification envelope —
// matching real AWS SNS, which signs a message once per publish and reuses
// that signature across all destinations. Previously each delivery function
// received its own bare event with empty Timestamp/Signature/SigningCertURL
// fields, and the Lambda envelope fabricated a random-UUID "signature" instead.
// sigVersion is the topic's resolved SignatureVersion ("1" or "2", see
// resolveSignatureVersion); an empty value resolves to the AWS default
// ("1"/SHA1withRSA).
func (b *InMemoryBackend) buildPublishedEvent(
	topicArn, messageID, message, subject string,
	attrs map[string]MessageAttribute,
	subs []events.SNSSubscriptionSnapshot,
	sigVersion string,
) *events.SNSPublishedEvent {
	attrSnaps := make(map[string]events.SNSMessageAttributeSnapshot, len(attrs))
	for k, v := range attrs {
		attrSnaps[k] = events.SNSMessageAttributeSnapshot{
			DataType:    v.DataType,
			StringValue: v.StringValue,
		}
	}

	sigVersion = resolveSignatureVersion(sigVersion)

	ts := time.Now().UTC().Format(time.RFC3339)
	canonical := canonicalNotificationString(messageID, topicArn, subject, message, ts)
	sig := b.signer.signWithVersion(canonical, sigVersion)
	certURL := b.signer.certURL()

	return &events.SNSPublishedEvent{
		TopicARN:         topicArn,
		MessageID:        messageID,
		Message:          message,
		Subject:          subject,
		Subscriptions:    subs,
		Attributes:       attrSnaps,
		Timestamp:        ts,
		Signature:        sig,
		SignatureVersion: sigVersion,
		SigningCertURL:   certURL,
	}
}

// emitPublishedEvent broadcasts ev to the publish emitter (e.g. to SQS). It is
// a no-op when no emitter has been registered. b.emitter is captured under the
// read lock since SetPublishEmitter mutates it under the write lock.
func (b *InMemoryBackend) emitPublishedEvent(ev *events.SNSPublishedEvent) {
	b.mu.RLock("emitPublishedEvent")
	emitter := b.emitter
	b.mu.RUnlock()

	if emitter == nil {
		return
	}

	_ = emitter.Emit(b.svcCtx, ev)
}

// Publish delivers a message to all subscriptions of topicArn. HTTP/HTTPS
// subscriptions each receive an asynchronous best-effort delivery goroutine
// after the read lock is released to avoid lock starvation. Goroutines
// wait for a concurrency slot (up to snsMaxConcurrentDeliveries concurrent HTTP
// calls) or exit early if the backend is shutting down.
// All subscriptions are also broadcast via the publish emitter (e.g. to SQS).
func (b *InMemoryBackend) Publish(
	topicArn, message, subject, messageStructure string, attrs map[string]MessageAttribute,
) (string, error) {
	if err := validatePublishMessage(message, subject, messageStructure, attrs); err != nil {
		return "", err
	}

	var (
		archivePolicy string
		sigVersion    string
		messageID     string
		targets       publishTargets
		client        *http.Client
		pubErr        error
	)

	func() {
		b.mu.RLock("Publish")
		defer b.mu.RUnlock()

		topic, exists := b.topics.Get(topicArn)
		if !exists {
			pubErr = ErrTopicNotFound

			return
		}

		// Capture whether this topic archives messages (ArchivePolicy present).
		archivePolicy = topic.Attributes[attrArchivePolicy]
		sigVersion = resolveSignatureVersion(topic.Attributes[attrSignatureVersion])

		messageID = uuid.NewString()

		// resolveMsg returns the appropriate message body for a given protocol.
		resolveMsg := buildMessageResolver(message, parsePerProtocolMessages(message, messageStructure))

		// Build subscription snapshot and collect HTTP deliveries — all under RLock.
		targets = b.collectPublishTargets(topicArn, subject, resolveMsg, attrs)

		// Annotate HTTP deliveries with messageID, topicARN, and signer for SNS envelope/headers.
		signer := b.signer
		for i := range targets.httpDeliveries {
			targets.httpDeliveries[i].messageID = messageID
			targets.httpDeliveries[i].topicARN = topicArn
			targets.httpDeliveries[i].signer = signer
		}

		// Capture httpClient under the read lock to avoid data races with
		// concurrent SetHTTPDeliveryClient calls.
		client = b.httpClient

		// Release the read lock before performing any network I/O so that slow or
		// unresponsive HTTP endpoints do not block write operations on the backend.
	}()

	if pubErr != nil {
		return "", pubErr
	}

	// Archive the message when the topic has an ArchivePolicy (e.g. FIFO topics
	// with message retention). Archived messages are used for subscription replay.
	if archivePolicy != "" {
		b.archivePublishedMessage(topicArn, messageID, message, subject, attrs)
	}

	b.dispatchHTTPDeliveries(targets.httpDeliveries, client)

	b.recordEmailDeliveries(targets.emailDeliveries, messageID, topicArn)

	// Build the shared event once so every channel below carries the same
	// verifiable Timestamp/Signature/SigningCertURL (see buildPublishedEvent).
	ev := b.buildPublishedEvent(topicArn, messageID, message, subject, attrs, targets.subs, sigVersion)

	b.emitPublishedEvent(ev)
	b.deliverToLambdaSubscriptions(ev)
	b.deliverToFirehoseSubscriptions(ev)
	b.deliverToSMSSubscriptions(ev)
	b.deliverToApplicationSubscriptions(ev)

	return messageID, nil
}

// PublishToTargetArn publishes a message directly to a platform endpoint ARN.
// Returns ErrEndpointDisabled when the endpoint has Enabled=false, matching
// the AWS EndpointDisabled error that triggers automatic endpoint disabling.
// In the mock, no actual push delivery occurs beyond generating the message ID.
func (b *InMemoryBackend) PublishToTargetArn(
	targetArn, _ /* message */, _ /* subject */ string,
	_ map[string]MessageAttribute,
) (string, error) {
	b.mu.RLock("PublishToTargetArn")
	defer b.mu.RUnlock()

	ep, exists := b.platformEndpoints.Get(targetArn)
	if !exists {
		return "", ErrEndpointNotFound
	}

	if ep.Attributes["Enabled"] == boolFalseStr {
		return "", fmt.Errorf("%w: endpoint %s is disabled", ErrEndpointDisabled, targetArn)
	}

	return uuid.NewString(), nil
}

// PublishSMS publishes a message directly to a phone number via SMS.
// The delivery is recorded in smsDeliveries so tests can assert on it via DrainSMSDeliveries.
// Returns ErrOptedOut when the destination number has opted out of SMS messages.
// Returns ErrSandboxPhoneNotVerified when the number is in the SMS sandbox but not yet verified.
func (b *InMemoryBackend) PublishSMS(phoneNumber, message string) (string, error) {
	if !isValidE164(phoneNumber) {
		return "", fmt.Errorf(
			"%w: Invalid phone number; must be in E.164 format",
			ErrInvalidParameter,
		)
	}

	var (
		sandboxEntry *SandboxPhoneNumber
		optedOut     bool
	)

	func() {
		b.mu.RLock("PublishSMS-check")
		defer b.mu.RUnlock()

		sandboxEntry, _ = b.smsSandbox.Get(phoneNumber)
		optedOut = b.optedOutPhoneNumbers[phoneNumber]
	}()

	// Opted-out numbers must not receive SMS regardless of sandbox state.
	if optedOut {
		return "", fmt.Errorf(
			"%w: phone number %s has opted out of SMS messages",
			ErrOptedOut,
			phoneNumber,
		)
	}

	// When the number is registered in the sandbox, it must be verified before
	// SMS can be sent to it. Unverified numbers in the sandbox mirror real AWS
	// sandbox behaviour where only verified destinations are allowed.
	if sandboxEntry != nil && sandboxEntry.Status != "Verified" {
		return "", fmt.Errorf(
			"%w: phone number %s is registered in the SMS sandbox but has not been verified",
			ErrSandboxPhoneNotVerified, phoneNumber,
		)
	}

	msgID := uuid.New().String()

	b.mu.Lock("PublishSMS")
	defer b.mu.Unlock()

	b.smsDeliveries = appendBounded(b.smsDeliveries, SMSDelivery{
		PhoneNumber: phoneNumber,
		Message:     message,
		MessageID:   msgID,
	}, maxRecordedDeliveries)

	return msgID, nil
}

// DrainSMSDeliveries returns and clears all recorded SMS deliveries.
// This is intended for test assertions to verify SMS messages sent via PublishSMS.
func (b *InMemoryBackend) DrainSMSDeliveries() []SMSDelivery {
	b.mu.Lock("DrainSMSDeliveries")
	defer b.mu.Unlock()

	deliveries := b.smsDeliveries
	b.smsDeliveries = nil

	return deliveries
}

// DrainApplicationDeliveries returns and clears all recorded application-protocol
// deliveries. These are recorded when a topic publish fans out to application
// (mobile push endpoint) subscriptions so tests can assert delivery without a
// real push network.
func (b *InMemoryBackend) DrainApplicationDeliveries() []ApplicationDelivery {
	b.mu.Lock("DrainApplicationDeliveries")
	defer b.mu.Unlock()

	deliveries := b.applicationDeliveries
	b.applicationDeliveries = nil

	return deliveries
}

// recordEmailDeliveries annotates and stores email/email-json deliveries produced
// by a publish so they can later be drained for inspection.
func (b *InMemoryBackend) recordEmailDeliveries(
	deliveries []EmailDelivery,
	messageID, topicArn string,
) {
	if len(deliveries) == 0 {
		return
	}

	b.mu.Lock("recordEmailDeliveries")
	defer b.mu.Unlock()

	for i := range deliveries {
		deliveries[i].MessageID = messageID
		deliveries[i].TopicARN = topicArn
		b.emailDeliveries = appendBounded(b.emailDeliveries, deliveries[i], maxRecordedDeliveries)
	}
}

// DrainEmailDeliveries returns and clears all recorded email/email-json deliveries.
// AWS delivers these to a mailbox; gopherstack records them here so tests and the
// dashboard can confirm the message was delivered.
func (b *InMemoryBackend) DrainEmailDeliveries() []EmailDelivery {
	b.mu.Lock("DrainEmailDeliveries")
	defer b.mu.Unlock()

	deliveries := b.emailDeliveries
	b.emailDeliveries = nil

	return deliveries
}

// isValidBatchEntryID returns true if the batch entry ID is non-empty, at most
// maxBatchEntryIDLen characters, and contains only alphanumeric characters, hyphens,
// or underscores. Matches the AWS SNS batch entry ID constraints.
func isValidBatchEntryID(id string) bool {
	if id == "" || len(id) > maxBatchEntryIDLen {
		return false
	}

	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') &&
			c != '-' && c != '_' {
			return false
		}
	}

	return true
}
