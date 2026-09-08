package ses

import (
	"encoding/json"
	"slices"
	"strings"
	"time"
)

// sesNotificationMail is the "mail" object of an SES SNS notification.
// Field names are sourced from the AWS SES Developer Guide
// (https://docs.aws.amazon.com/ses/latest/dg/notification-contents.html),
// NOT the pinned aws-sdk-go-v2 module -- these payloads are consumed by SNS
// subscribers, never decoded by the SES API client, so no SDK type exists to
// verify field names against. Only the fields every documented example
// includes are modeled; optional/conditional ones this backend has no real
// data for (headers, sourceArn, sourceIp, sendingAccountId, callerIdentity)
// are omitted rather than fabricated.
type sesNotificationMail struct {
	Timestamp   string   `json:"timestamp"`
	MessageID   string   `json:"messageId"`
	Source      string   `json:"source"`
	Destination []string `json:"destination"`
}

// sesBouncedRecipient is one entry of a bounce object's "bouncedRecipients"
// list. Doc-sourced (see sesNotificationMail); action/status/diagnosticCode
// are DSN-derived optional fields this backend has no DSN to populate, so
// only the always-present emailAddress field is modeled.
type sesBouncedRecipient struct {
	EmailAddress string `json:"emailAddress"`
}

// sesBounceDetail is the "bounce" object. Doc-sourced (see
// sesNotificationMail). bounceType/bounceSubType are always reported as
// "Permanent"/"General" (a hard bounce) -- classifySimulatedRecipients does
// not distinguish bounce subtypes, so this is a disclosed simplification,
// not a fabricated distinction.
type sesBounceDetail struct {
	BounceType        string                `json:"bounceType"`
	BounceSubType     string                `json:"bounceSubType"`
	Timestamp         string                `json:"timestamp"`
	BouncedRecipients []sesBouncedRecipient `json:"bouncedRecipients"`
}

// sesComplainedRecipient is one entry of a complaint object's
// "complainedRecipients" list. Doc-sourced (see sesNotificationMail).
type sesComplainedRecipient struct {
	EmailAddress string `json:"emailAddress"`
}

// sesComplaintDetail is the "complaint" object. Doc-sourced (see
// sesNotificationMail).
type sesComplaintDetail struct {
	Timestamp            string                   `json:"timestamp"`
	ComplainedRecipients []sesComplainedRecipient `json:"complainedRecipients"`
}

// sesDeliveryDetail is the "delivery" object. Doc-sourced (see
// sesNotificationMail). processingTimeMillis/smtpResponse/reportingMTA/
// remoteMtaIp are omitted -- this backend has no simulated delivery latency
// or remote MTA to report.
type sesDeliveryDetail struct {
	Timestamp  string   `json:"timestamp"`
	Recipients []string `json:"recipients"`
}

// buildSESNotificationPayload assembles the top-level JSON object. The
// top-level field name differs by delivery path per the AWS SES Developer
// Guide: identity notification topics (SetIdentityNotificationTopic) use
// "notificationType"; configuration-set event destinations (event
// publishing) use "eventType" instead -- "If you set up event publishing,
// this field is named eventType." (same doc page as sesNotificationMail).
func buildSESNotificationPayload(
	topFieldName, topFieldValue string,
	mail sesNotificationMail,
	bounce *sesBounceDetail,
	complaint *sesComplaintDetail,
	delivery *sesDeliveryDetail,
) []byte {
	data := map[string]any{
		topFieldName: topFieldValue,
		"mail":       mail,
	}

	switch {
	case bounce != nil:
		data["bounce"] = bounce
	case complaint != nil:
		data["complaint"] = complaint
	case delivery != nil:
		data["delivery"] = delivery
	}

	// map[string]any marshaling of these concrete field types cannot fail.
	bs, _ := json.Marshal(data)

	return bs
}

// sesNotificationTargets carries the SNS publisher and this send's
// notification destinations, collected under b.mu so the actual SNS publish
// can happen after the lock is released -- mirrors cloudwatch's
// alarmActionDeps pattern (services/cloudwatch/alarm_actions.go).
type sesNotificationTargets struct {
	publisher     SNSPublisher
	notifType     string
	identityTopic string
	destTopics    []string
}

// collectNotificationTargetsLocked determines this send's outcome (Bounce or
// Complaint per classifySimulatedRecipients, otherwise Delivery) and gathers
// the identity's matching notification topic plus the SNS topic ARNs of any
// enabled configuration-set event destinations whose MatchingEventTypes
// includes it. The identity lookup is an exact match on the From address --
// unlike isVerifiedLocked, it does not fall back to a verified domain
// identity. The caller MUST hold b.mu.
func (b *InMemoryBackend) collectNotificationTargetsLocked(
	from, configSetName string, bounced, complained bool,
) sesNotificationTargets {
	t := sesNotificationTargets{publisher: b.snsPublisher}

	switch {
	case bounced:
		t.notifType = notifTypeBounce
	case complained:
		t.notifType = notifTypeComplaint
	default:
		t.notifType = notifTypeDelivery
	}

	if rec, ok := b.identities.Get(from); ok {
		switch t.notifType {
		case notifTypeBounce:
			t.identityTopic = rec.BounceTopic
		case notifTypeComplaint:
			t.identityTopic = rec.ComplaintTopic
		case notifTypeDelivery:
			t.identityTopic = rec.DeliveryTopic
		}
	}

	if configSetName == "" {
		return t
	}

	// EventDestination.MatchingEventTypes uses the lowercase EventType enum
	// (isValidEventType), unlike the capitalized NotificationType enum
	// (isValidNotificationType) t.notifType is drawn from -- both enums'
	// values are identical apart from case for Bounce/Complaint/Delivery.
	eventType := strings.ToLower(t.notifType)

	for _, d := range b.eventDestinationsByConfigSet.Get(configSetName) {
		if d.Enabled && d.SNSTopicARN != "" && slices.Contains(d.MatchingEventTypes, eventType) {
			t.destTopics = append(t.destTopics, d.SNSTopicARN)
		}
	}

	return t
}

// publishEmailNotifications delivers an SNS notification for a send's
// outcome to the identity's configured notification topic and any matching
// configuration-set event destinations. A no-op when no SNSPublisher has
// been wired (SetSNSPublisher never called) or neither target is
// configured -- an unwired SES backend must behave exactly as before this
// feature existed. The caller must NOT hold b.mu.
func (b *InMemoryBackend) publishEmailNotifications(email Email, t sesNotificationTargets) {
	if t.publisher == nil || (t.identityTopic == "" && len(t.destTopics) == 0) {
		return
	}

	mail := sesNotificationMail{
		Timestamp:   email.Timestamp.UTC().Format(time.RFC3339),
		MessageID:   email.MessageID,
		Source:      email.From,
		Destination: allRecipients(email.To, email.Cc, email.Bcc),
	}

	var (
		bounce    *sesBounceDetail
		complaint *sesComplaintDetail
		delivery  *sesDeliveryDetail
	)

	switch t.notifType {
	case notifTypeBounce:
		recipients := make([]sesBouncedRecipient, 0, len(mail.Destination))
		for _, addr := range mail.Destination {
			recipients = append(recipients, sesBouncedRecipient{EmailAddress: addr})
		}

		bounce = &sesBounceDetail{
			BounceType:        "Permanent",
			BounceSubType:     "General",
			Timestamp:         mail.Timestamp,
			BouncedRecipients: recipients,
		}
	case notifTypeComplaint:
		recipients := make([]sesComplainedRecipient, 0, len(mail.Destination))
		for _, addr := range mail.Destination {
			recipients = append(recipients, sesComplainedRecipient{EmailAddress: addr})
		}

		complaint = &sesComplaintDetail{
			Timestamp:            mail.Timestamp,
			ComplainedRecipients: recipients,
		}
	default:
		delivery = &sesDeliveryDetail{
			Timestamp:  mail.Timestamp,
			Recipients: mail.Destination,
		}
	}

	if t.identityTopic != "" {
		payload := buildSESNotificationPayload("notificationType", t.notifType, mail, bounce, complaint, delivery)
		_ = t.publisher.PublishToTopic(t.identityTopic, string(payload))
	}

	if len(t.destTopics) > 0 {
		eventPayload := buildSESNotificationPayload(
			"eventType", strings.ToLower(t.notifType), mail, bounce, complaint, delivery,
		)
		for _, topic := range t.destTopics {
			_ = t.publisher.PublishToTopic(topic, string(eventPayload))
		}
	}
}
