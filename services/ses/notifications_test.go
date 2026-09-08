package ses_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ses"
)

// mockSESSNSPublisher captures published messages for assertions.
type mockSESSNSPublisher struct {
	calls []mockSESSNSCall
}

type mockSESSNSCall struct {
	topicARN string
	message  string
}

func (m *mockSESSNSPublisher) PublishToTopic(topicARN, message string) error {
	m.calls = append(m.calls, mockSESSNSCall{topicARN: topicARN, message: message})

	return nil
}

// TestSendEmail_PublishesToEventDestinationSNSTopic covers the
// configuration-set event-destination path (gopherstack-y6rv): a bounce
// outcome must publish to every enabled event destination whose
// MatchingEventTypes includes "bounce", using the doc-sourced "eventType"
// top-level field name (event publishing, not identity notifications).
func TestSendEmail_PublishesToEventDestinationSNSTopic(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("s@example.com"))
	require.NoError(t, b.CreateConfigurationSet("cs-bounce"))
	require.NoError(t, b.CreateConfigurationSetEventDestination("cs-bounce", ses.EventDestination{
		Name:               "bounce-dest",
		SNSTopicARN:        "arn:aws:sns:us-east-1:123456789012:bounce-topic",
		MatchingEventTypes: []string{"bounce"},
		Enabled:            true,
	}))

	pub := &mockSESSNSPublisher{}
	b.SetSNSPublisher(pub)

	_, err := b.SendEmail(ses.SendEmailInput{
		From:                 "s@example.com",
		To:                   []string{"bounce@simulator.amazonses.com"},
		Subject:              "test",
		BodyText:             "body",
		ConfigurationSetName: "cs-bounce",
	})
	require.NoError(t, err)

	require.Len(t, pub.calls, 1)
	assert.Equal(t, "arn:aws:sns:us-east-1:123456789012:bounce-topic", pub.calls[0].topicARN)
	assert.Contains(t, pub.calls[0].message, `"eventType":"bounce"`)
	assert.Contains(t, pub.calls[0].message, `"bounceType":"Permanent"`)
}

// TestSendEmail_EventDestination_DisabledOrNonMatching_NoPublish confirms
// the event-destination path respects Enabled and MatchingEventTypes rather
// than publishing to every configured destination regardless.
func TestSendEmail_EventDestination_DisabledOrNonMatching_NoPublish(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("s@example.com"))
	require.NoError(t, b.CreateConfigurationSet("cs-filtered"))
	require.NoError(t, b.CreateConfigurationSetEventDestination("cs-filtered", ses.EventDestination{
		Name:               "disabled-dest",
		SNSTopicARN:        "arn:aws:sns:us-east-1:123456789012:disabled-topic",
		MatchingEventTypes: []string{"bounce"},
		Enabled:            false,
	}))
	require.NoError(t, b.CreateConfigurationSetEventDestination("cs-filtered", ses.EventDestination{
		Name:               "complaint-only-dest",
		SNSTopicARN:        "arn:aws:sns:us-east-1:123456789012:complaint-topic",
		MatchingEventTypes: []string{"complaint"},
		Enabled:            true,
	}))

	pub := &mockSESSNSPublisher{}
	b.SetSNSPublisher(pub)

	_, err := b.SendEmail(ses.SendEmailInput{
		From:                 "s@example.com",
		To:                   []string{"bounce@simulator.amazonses.com"},
		Subject:              "test",
		BodyText:             "body",
		ConfigurationSetName: "cs-filtered",
	})
	require.NoError(t, err)

	assert.Empty(t, pub.calls)
}

// TestSendEmail_PublishesToIdentityNotificationTopic covers the identity
// bounce/complaint/delivery notification-topic path (gopherstack-y6rv): a
// bounce outcome must publish to the identity's SetIdentityNotificationTopic
// BounceTopic, using the doc-sourced "notificationType" top-level field name.
func TestSendEmail_PublishesToIdentityNotificationTopic(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("s@example.com"))
	require.NoError(t, b.SetIdentityNotificationTopic(
		"s@example.com", "Bounce", "arn:aws:sns:us-east-1:123456789012:identity-bounce",
	))

	pub := &mockSESSNSPublisher{}
	b.SetSNSPublisher(pub)

	_, err := b.SendEmail(ses.SendEmailInput{
		From:     "s@example.com",
		To:       []string{"bounce@simulator.amazonses.com"},
		Subject:  "test",
		BodyText: "body",
	})
	require.NoError(t, err)

	require.Len(t, pub.calls, 1)
	assert.Equal(t, "arn:aws:sns:us-east-1:123456789012:identity-bounce", pub.calls[0].topicARN)
	assert.Contains(t, pub.calls[0].message, `"notificationType":"Bounce"`)
}

// TestSendEmail_DeliveryNotification_OrdinarySend covers the Delivery leg:
// a send that neither bounces nor complains still publishes a Delivery
// notification to the identity's DeliveryTopic, matching real SES firing a
// delivery notification for every successfully delivered message.
func TestSendEmail_DeliveryNotification_OrdinarySend(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("s@example.com"))
	require.NoError(t, b.SetIdentityNotificationTopic(
		"s@example.com", "Delivery", "arn:aws:sns:us-east-1:123456789012:identity-delivery",
	))

	pub := &mockSESSNSPublisher{}
	b.SetSNSPublisher(pub)

	_, err := b.SendEmail(ses.SendEmailInput{
		From:     "s@example.com",
		To:       []string{"ok@example.com"},
		Subject:  "test",
		BodyText: "body",
	})
	require.NoError(t, err)

	require.Len(t, pub.calls, 1)
	assert.Contains(t, pub.calls[0].message, `"notificationType":"Delivery"`)
}

// TestSendEmail_NoSNSPublisherWired_StaysPermissive proves the critical
// no-op-by-default contract: with both a matching event destination and an
// identity notification topic configured, but no SNSPublisher ever wired
// (SetSNSPublisher never called -- the ~150-service unwired-hook norm),
// SendEmail must still succeed exactly as it did before this feature
// existed. An unwired hook must never reject or error.
func TestSendEmail_NoSNSPublisherWired_StaysPermissive(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.VerifyEmailIdentity("s@example.com"))
	require.NoError(t, b.CreateConfigurationSet("cs-nowire"))
	require.NoError(t, b.CreateConfigurationSetEventDestination("cs-nowire", ses.EventDestination{
		Name:               "dest",
		SNSTopicARN:        "arn:aws:sns:us-east-1:123456789012:topic",
		MatchingEventTypes: []string{"bounce"},
		Enabled:            true,
	}))
	require.NoError(t, b.SetIdentityNotificationTopic(
		"s@example.com", "Bounce", "arn:aws:sns:us-east-1:123456789012:identity-topic",
	))

	msgID, err := b.SendEmail(ses.SendEmailInput{
		From:                 "s@example.com",
		To:                   []string{"bounce@simulator.amazonses.com"},
		Subject:              "test",
		BodyText:             "body",
		ConfigurationSetName: "cs-nowire",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, msgID)

	email, err := b.GetEmailByID(msgID)
	require.NoError(t, err)
	assert.True(t, email.Bounced)
}
