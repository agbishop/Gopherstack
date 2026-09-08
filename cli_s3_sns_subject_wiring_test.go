package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	s3backend "github.com/blackbirdworks/gopherstack/services/s3"
	snsbackend "github.com/blackbirdworks/gopherstack/services/sns"
	sqsbackend "github.com/blackbirdworks/gopherstack/services/sqs"
)

// TestS3SNSPublisherAdapter_ForwardsSubject drives the real production types cli.go wires
// together for S3-to-SNS bucket notifications: s3SNSPublisherAdapter (as constructed by
// wireS3Notifications), the real SNS InMemoryBackend, and the real SNS->SQS delivery path
// (as wired by wireSNSToSQS), then reads the delivered message back off a real SQS queue --
// the same envelope a real AWS client would receive.
//
// Regression test for gopherstack-91tc: s3SNSPublisherAdapter.PublishToTopic hardcoded an
// empty Subject into every SNS publish instead of forwarding the "S3Notification" subject
// that services/s3/notification.go's dispatchToTopic passes it, so every S3 bucket
// notification delivered to SNS lost its Subject.
func TestS3SNSPublisherAdapter_ForwardsSubject(t *testing.T) {
	t.Parallel()

	snsBk := snsbackend.NewInMemoryBackend()
	snsH := snsbackend.NewHandler(snsBk)

	sqsBk := sqsbackend.NewInMemoryBackend()
	t.Cleanup(sqsBk.Close)
	sqsH := sqsbackend.NewHandler(sqsBk)

	// The exact production wiring call cli.go's setup makes to connect SNS publishes to
	// subscribed SQS queues.
	wireSNSToSQS(snsH, sqsH)

	topic, err := snsBk.CreateTopic("s3-notification-topic", nil)
	require.NoError(t, err)

	queueOut, err := sqsBk.CreateQueue(&sqsbackend.CreateQueueInput{
		QueueName: "s3-notification-queue",
		Endpoint:  "localhost:8000",
	})
	require.NoError(t, err)

	queueARN := "arn:aws:sqs:" + config.DefaultRegion + ":" + config.DefaultAccountID + ":s3-notification-queue"
	_, err = snsBk.Subscribe(topic.TopicArn, "sqs", queueARN, "")
	require.NoError(t, err)

	// Build the dispatcher exactly as wireS3Notifications does in cli.go, with the real
	// s3SNSPublisherAdapter targeting the real SNS backend.
	targets := &s3backend.NotificationTargets{
		SNSPublisher: &s3SNSPublisherAdapter{backend: snsBk},
	}
	dispatcher := s3backend.NewNotificationDispatcher(targets, config.DefaultRegion)

	notifXML := `<NotificationConfiguration><TopicConfiguration>` +
		`<Topic>` + topic.TopicArn + `</Topic>` +
		`<Event>s3:ObjectCreated:*</Event></TopicConfiguration></NotificationConfiguration>`

	dispatcher.DispatchObjectCreated(t.Context(), "notif-bucket", "notif-key", "etag123", 42, notifXML)

	var receivedBody string
	require.Eventually(t, func() bool {
		out, recvErr := sqsBk.ReceiveMessage(&sqsbackend.ReceiveMessageInput{
			QueueURL:            queueOut.QueueURL,
			MaxNumberOfMessages: 1,
		})
		if recvErr != nil || len(out.Messages) == 0 {
			return false
		}

		receivedBody = out.Messages[0].Body

		return true
	}, 5*time.Second, 50*time.Millisecond,
		"an S3 bucket notification dispatched to an SNS topic must be delivered to a "+
			"subscribed SQS queue through the real cli.go composition root's wiring")

	var envelope struct {
		Subject string `json:"Subject"`
	}
	require.NoError(t, json.Unmarshal([]byte(receivedBody), &envelope))
	require.Equal(t, "S3Notification", envelope.Subject,
		"s3SNSPublisherAdapter must forward the S3 notification dispatcher's subject to SNS "+
			"Publish instead of hardcoding an empty one")
}
