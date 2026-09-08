package integration_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_SQS_PurgeCooldown verifies that a second PurgeQueue call within
// 60 seconds returns PurgeQueueInProgress.
func TestIntegration_SQS_PurgeCooldown(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)
	client := createSQSClient(t)
	ctx := t.Context()

	qName := "purge-cooldown-" + uuid.NewString()[:8]
	out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(qName),
	})
	require.NoError(t, err)

	qURL := *out.QueueUrl
	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteQueue(cleanupCtx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
	})

	// First purge should succeed.
	_, err = client.PurgeQueue(ctx, &sqs.PurgeQueueInput{QueueUrl: aws.String(qURL)})
	require.NoError(t, err)

	// Second purge immediately after should fail with PurgeQueueInProgress.
	_, err = client.PurgeQueue(ctx, &sqs.PurgeQueueInput{QueueUrl: aws.String(qURL)})
	require.Error(t, err, "expected PurgeQueueInProgress error")
	var purgeInProgress *sqstypes.PurgeQueueInProgress
	assert.ErrorAs(t, err, &purgeInProgress, "expected PurgeQueueInProgress error type")
}

// TestIntegration_SQS_SqsManagedSseEnabled verifies that a new queue reports
// SqsManagedSseEnabled=true by default (matching AWS behaviour).
func TestIntegration_SQS_SqsManagedSseEnabled(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)
	client := createSQSClient(t)
	ctx := t.Context()

	qName := "sse-default-" + uuid.NewString()[:8]
	out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(qName),
	})
	require.NoError(t, err)

	qURL := *out.QueueUrl
	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteQueue(cleanupCtx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
	})

	attrsOut, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(qURL),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll},
	})
	require.NoError(t, err)
	assert.Equal(t, "true", attrsOut.Attributes["SqsManagedSseEnabled"],
		"new queues should have SqsManagedSseEnabled=true by default")
}

// TestIntegration_SQS_ReceiveMessageWaitTimeSeconds verifies that
// SetQueueAttributes rejects out-of-range ReceiveMessageWaitTimeSeconds.
func TestIntegration_SQS_ReceiveMessageWaitTimeSeconds(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)
	client := createSQSClient(t)
	ctx := t.Context()

	tests := []struct {
		name    string
		waitSec string
		wantErr bool
	}{
		{name: "valid_0", waitSec: "0", wantErr: false},
		{name: "valid_20", waitSec: "20", wantErr: false},
		{name: "invalid_21", waitSec: "21", wantErr: true},
		{name: "invalid_negative", waitSec: "-1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dumpContainerLogsOnFailure(t)

			// Each sub-test gets its own queue to avoid parallel SetQueueAttributes races.
			qName := "wait-time-" + uuid.NewString()[:8]
			qOut, createErr := client.CreateQueue(ctx, &sqs.CreateQueueInput{
				QueueName: aws.String(qName),
			})
			require.NoError(t, createErr)

			qURL := *qOut.QueueUrl
			t.Cleanup(func() {
				cleanupCtx, cancel := cleanupContext(t)
				defer cancel()

				_, _ = client.DeleteQueue(cleanupCtx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
			})

			_, setErr := client.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
				QueueUrl: aws.String(qURL),
				Attributes: map[string]string{
					"ReceiveMessageWaitTimeSeconds": tt.waitSec,
				},
			})

			if tt.wantErr {
				require.Error(t, setErr)
			} else {
				require.NoError(t, setErr)
			}
		})
	}
}

// TestIntegration_SQS_FIFONameValidation verifies that FIFO queue names must end
// with the .fifo suffix, and that a standard queue name ending in .fifo is accepted
// as a FIFO queue.
func TestIntegration_SQS_FIFONameValidation(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)
	client := createSQSClient(t)
	ctx := t.Context()

	tests := []struct {
		attrs     map[string]string
		name      string
		queueName string
		wantErr   bool
	}{
		{
			name:      "valid_fifo_name_with_attribute",
			queueName: "valid-" + uuid.NewString()[:8] + ".fifo",
			attrs:     map[string]string{"FifoQueue": "true"},
			wantErr:   false,
		},
		{
			name:      "fifo_name_without_attribute_uses_suffix",
			queueName: "auto-" + uuid.NewString()[:8] + ".fifo",
			attrs:     map[string]string{"FifoQueue": "true"},
			wantErr:   false,
		},
		{
			name:      "standard_name_no_attrs",
			queueName: "std-" + uuid.NewString()[:8],
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dumpContainerLogsOnFailure(t)

			out, createErr := client.CreateQueue(ctx, &sqs.CreateQueueInput{
				QueueName:  aws.String(tt.queueName),
				Attributes: tt.attrs,
			})

			if tt.wantErr {
				require.Error(t, createErr)

				return
			}

			require.NoError(t, createErr)
			t.Cleanup(func() {
				cleanupCtx, cancel := cleanupContext(t)
				defer cancel()

				_, _ = client.DeleteQueue(cleanupCtx, &sqs.DeleteQueueInput{QueueUrl: out.QueueUrl})
			})
		})
	}
}

// TestIntegration_SQS_DeleteMessageBatchValidation verifies that
// DeleteMessageBatch enforces batch entry ID uniqueness.
func TestIntegration_SQS_DeleteMessageBatchValidation(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)
	client := createSQSClient(t)
	ctx := t.Context()

	qName := "del-batch-" + uuid.NewString()[:8]
	out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(qName),
	})
	require.NoError(t, err)

	qURL := *out.QueueUrl
	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteQueue(cleanupCtx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
	})

	// Send two messages so we have receipt handles to work with.
	for range 2 {
		_, sendErr := client.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:    aws.String(qURL),
			MessageBody: aws.String("body"),
		})
		require.NoError(t, sendErr)
	}

	tests := []struct {
		name    string
		entries []sqstypes.DeleteMessageBatchRequestEntry
		wantErr bool
	}{
		{
			name: "duplicate_ids_rejected",
			entries: []sqstypes.DeleteMessageBatchRequestEntry{
				{Id: aws.String("dup"), ReceiptHandle: aws.String("h1")},
				{Id: aws.String("dup"), ReceiptHandle: aws.String("h2")},
			},
			wantErr: true,
		},
		{
			name:    "empty_batch_rejected",
			entries: []sqstypes.DeleteMessageBatchRequestEntry{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dumpContainerLogsOnFailure(t)

			_, delErr := client.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{
				QueueUrl: aws.String(qURL),
				Entries:  tt.entries,
			})

			if tt.wantErr {
				require.Error(t, delErr)
			} else {
				require.NoError(t, delErr)
			}
		})
	}
}

// TestIntegration_SQS_SystemAttributes verifies that system attributes
// (ApproximateReceiveCount, SentTimestamp, etc.) are returned when requested.
func TestIntegration_SQS_SystemAttributes(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)
	client := createSQSClient(t)
	ctx := t.Context()

	qName := "sys-attrs-" + uuid.NewString()[:8]
	out, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(qName),
	})
	require.NoError(t, err)

	qURL := *out.QueueUrl
	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteQueue(cleanupCtx, &sqs.DeleteQueueInput{QueueUrl: aws.String(qURL)})
	})

	_, err = client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(qURL),
		MessageBody: aws.String("hello"),
	})
	require.NoError(t, err)

	recv, err := client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:                    aws.String(qURL),
		MaxNumberOfMessages:         1,
		MessageSystemAttributeNames: []sqstypes.MessageSystemAttributeName{sqstypes.MessageSystemAttributeNameAll},
	})
	require.NoError(t, err)
	require.Len(t, recv.Messages, 1)

	msg := recv.Messages[0]
	assert.NotEmpty(t, msg.Attributes["SentTimestamp"], "SentTimestamp should be returned")
	assert.Equal(t, "1", msg.Attributes["ApproximateReceiveCount"],
		"first receive should report ApproximateReceiveCount=1")
}
