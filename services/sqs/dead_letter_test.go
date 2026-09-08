package sqs_test

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sqs"
)

func TestRedriveAllowPolicy(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	dlqURL := createQueueForTest(t, b, "dlq")
	dlqAttrs, err := b.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		QueueURL:       dlqURL,
		AttributeNames: []string{"QueueArn"},
	})
	require.NoError(t, err)
	dlqARN := dlqAttrs.Attributes["QueueArn"]

	rap, _ := json.Marshal(map[string]any{
		"redrivePermission": "byQueue",
		"sourceQueueArns":   []string{dlqARN},
	})

	_, err = b.CreateQueue(&sqs.CreateQueueInput{
		QueueName: "src",
		Endpoint:  testEndpoint,
		Attributes: map[string]string{
			"RedriveAllowPolicy": string(rap),
		},
	})
	require.NoError(t, err)
}

func TestRedrivePolicy_MovesToDLQAfterMaxReceiveCount(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	dlqURL, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: "dlq-target", Endpoint: "localhost"})
	require.NoError(t, err)

	dlqAttrs := b2getAttrs(t, b, dlqURL.QueueURL, "QueueArn")
	dlqARN := dlqAttrs["QueueArn"]

	redriveJSON, _ := json.Marshal(map[string]any{
		"deadLetterTargetArn": dlqARN,
		"maxReceiveCount":     2,
	})

	srcURL, err := b.CreateQueue(&sqs.CreateQueueInput{
		QueueName: "src-with-dlq",
		Endpoint:  "localhost",
		Attributes: map[string]string{
			"VisibilityTimeout": "0",
			"RedrivePolicy":     string(redriveJSON),
		},
	})
	require.NoError(t, err)

	b2send(t, b, srcURL.QueueURL, "will-go-to-dlq")

	// Receive 3 times (exceeds maxReceiveCount=2 → moves to DLQ on 3rd)
	for i := range 3 {
		msgs, recvErr := b.ReceiveMessage(&sqs.ReceiveMessageInput{
			QueueURL:            srcURL.QueueURL,
			MaxNumberOfMessages: 1,
			VisibilityTimeout:   0,
			AttributeNames:      []string{"All"},
		})
		require.NoError(t, recvErr)
		if i < 2 {
			require.Len(t, msgs.Messages, 1)
		}
	}

	// Source queue should be empty; DLQ should have the message
	srcMsgs := b2receive(t, b, srcURL.QueueURL, 1)
	assert.Empty(t, srcMsgs)

	dlqMsgs := b2receive(t, b, dlqURL.QueueURL, 1)
	require.Len(t, dlqMsgs, 1)
	assert.Equal(t, "will-go-to-dlq", dlqMsgs[0].Body)
}

func TestRedrivePolicy_SetViaSetQueueAttributes(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	dlqURL := b2createQueue(t, b, "dlq-setattr")
	dlqAttrs := b2getAttrs(t, b, dlqURL, "QueueArn")
	dlqARN := dlqAttrs["QueueArn"]

	srcURL := b2createQueue(t, b, "src-setattr")

	redriveJSON, _ := json.Marshal(map[string]any{
		"deadLetterTargetArn": dlqARN,
		"maxReceiveCount":     3,
	})

	err := b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
		QueueURL: srcURL,
		Attributes: map[string]string{
			"RedrivePolicy": string(redriveJSON),
		},
	})
	require.NoError(t, err)

	// Verify round-trip via GetQueueAttributes
	attrs := b2getAttrs(t, b, srcURL, "RedrivePolicy")
	var pol map[string]any
	require.NoError(t, json.Unmarshal([]byte(attrs["RedrivePolicy"]), &pol))
	assert.Equal(t, dlqARN, pol["deadLetterTargetArn"])
}

func TestListDeadLetterSourceQueues_SingleSource(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	dlqURL := b2createQueue(t, b, "ldlsq-dlq")
	dlqAttrs := b2getAttrs(t, b, dlqURL, "QueueArn")
	dlqARN := dlqAttrs["QueueArn"]

	makeRedriveAttr := func(arn string, count int) string {
		v, _ := json.Marshal(map[string]any{"deadLetterTargetArn": arn, "maxReceiveCount": count})

		return string(v)
	}

	srcURL1 := b2createQueue(t, b, "ldlsq-src1")
	require.NoError(t, b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
		QueueURL:   srcURL1,
		Attributes: map[string]string{"RedrivePolicy": makeRedriveAttr(dlqARN, 2)},
	}))

	srcURL2 := b2createQueue(t, b, "ldlsq-src2")
	require.NoError(t, b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
		QueueURL:   srcURL2,
		Attributes: map[string]string{"RedrivePolicy": makeRedriveAttr(dlqARN, 3)},
	}))

	// A third queue points to a different DLQ
	otherDLQ := b2createQueue(t, b, "ldlsq-other-dlq")
	srcURL3 := b2createQueue(t, b, "ldlsq-src3")
	otherDLQARN := b2getAttrs(t, b, otherDLQ, "QueueArn")["QueueArn"]
	require.NoError(t, b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
		QueueURL:   srcURL3,
		Attributes: map[string]string{"RedrivePolicy": makeRedriveAttr(otherDLQARN, 1)},
	}))

	out, err := b.ListDeadLetterSourceQueues(&sqs.ListDeadLetterSourceQueuesInput{QueueURL: dlqURL})
	require.NoError(t, err)
	assert.Len(t, out.QueueURLs, 2)
	assert.Contains(t, out.QueueURLs, srcURL1)
	assert.Contains(t, out.QueueURLs, srcURL2)
	assert.NotContains(t, out.QueueURLs, srcURL3)
}

func TestListDeadLetterSourceQueues_QueueNotFound(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	_, err := b.ListDeadLetterSourceQueues(&sqs.ListDeadLetterSourceQueuesInput{
		QueueURL: "http://localhost/000000000000/nonexistent",
	})
	require.ErrorIs(t, err, sqs.ErrQueueNotFound)
}

func TestRedriveAllowPolicy_AllowAll(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	policy, _ := json.Marshal(map[string]any{"redrivePermission": "allowAll"})

	_, err := b.CreateQueue(&sqs.CreateQueueInput{
		QueueName: "rap-allow-all",
		Endpoint:  "localhost",
		Attributes: map[string]string{
			"RedriveAllowPolicy": string(policy),
		},
	})
	require.NoError(t, err)
}

func TestRedriveAllowPolicy_DenyAll(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	policy, _ := json.Marshal(map[string]any{"redrivePermission": "denyAll"})

	_, err := b.CreateQueue(&sqs.CreateQueueInput{
		QueueName: "rap-deny-all",
		Endpoint:  "localhost",
		Attributes: map[string]string{
			"RedriveAllowPolicy": string(policy),
		},
	})
	require.NoError(t, err)
}

func TestRedriveAllowPolicy_ByQueue(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	policy, _ := json.Marshal(map[string]any{
		"redrivePermission": "byQueue",
		"sourceQueueArns":   []string{"arn:aws:sqs:us-east-1:123456789012:src-queue"},
	})

	_, err := b.CreateQueue(&sqs.CreateQueueInput{
		QueueName: "rap-by-queue",
		Endpoint:  "localhost",
		Attributes: map[string]string{
			"RedriveAllowPolicy": string(policy),
		},
	})
	require.NoError(t, err)
}

func TestRedriveAllowPolicy_AllowAllWithArns_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	policy, _ := json.Marshal(map[string]any{
		"redrivePermission": "allowAll",
		"sourceQueueArns":   []string{"arn:aws:sqs:us-east-1:123456789012:q"},
	})

	_, err := b.CreateQueue(&sqs.CreateQueueInput{
		QueueName: "rap-invalid",
		Endpoint:  "localhost",
		Attributes: map[string]string{
			"RedriveAllowPolicy": string(policy),
		},
	})
	require.Error(t, err) // allowAll + arns → invalid
}

func TestRedriveAllowPolicy_ByQueueTooManyArns_Rejected(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	arns := make([]string, 11)
	for i := range arns {
		arns[i] = fmt.Sprintf("arn:aws:sqs:us-east-1:123456789012:q%d", i)
	}
	policy, _ := json.Marshal(map[string]any{
		"redrivePermission": "byQueue",
		"sourceQueueArns":   arns,
	})

	_, err := b.CreateQueue(&sqs.CreateQueueInput{
		QueueName: "rap-toomany",
		Endpoint:  "localhost",
		Attributes: map[string]string{
			"RedriveAllowPolicy": string(policy),
		},
	})
	require.Error(t, err)
}

func TestRedriveAllowPolicy_RoundTrip(t *testing.T) {
	t.Parallel()
	b := b2newBackend(t)

	policy, _ := json.Marshal(map[string]any{"redrivePermission": "denyAll"})
	qURL := b2createQueue(t, b, "rap-roundtrip")

	require.NoError(t, b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
		QueueURL:   qURL,
		Attributes: map[string]string{"RedriveAllowPolicy": string(policy)},
	}))

	attrs := b2getAttrs(t, b, qURL, "RedriveAllowPolicy")
	assert.JSONEq(t, string(policy), attrs["RedriveAllowPolicy"])
}

// TestDLQ_AutoRedrive verifies that messages exceeding maxReceiveCount
// are automatically moved to the DLQ.
func TestDLQ_AutoRedrive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		maxReceiveCount int
		receiveAttempts int
		wantInDLQ       bool
	}{
		{
			name:            "message below maxReceiveCount stays in source",
			maxReceiveCount: 3,
			receiveAttempts: 2,
			wantInDLQ:       false,
		},
		{
			name:            "message at maxReceiveCount moves to DLQ",
			maxReceiveCount: 2,
			receiveAttempts: 2,
			wantInDLQ:       true,
		},
		{
			name:            "maxReceiveCount=1 triggers DLQ on first receive",
			maxReceiveCount: 1,
			receiveAttempts: 1,
			wantInDLQ:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)

			// Create DLQ.
			dlqOut, err := b.CreateQueue(&sqs.CreateQueueInput{
				QueueName: "dlq",
				Endpoint:  testEndpoint,
			})
			require.NoError(t, err)

			dlqARN := "arn:aws:sqs:us-east-1:000000000000:dlq"

			// Create source queue with redrive policy.
			srcOut, err := b.CreateQueue(&sqs.CreateQueueInput{
				QueueName: "src",
				Endpoint:  testEndpoint,
				Attributes: map[string]string{
					"RedrivePolicy": fmt.Sprintf(
						`{"deadLetterTargetArn":%q,"maxReceiveCount":%d}`,
						dlqARN, tc.maxReceiveCount,
					),
				},
			})
			require.NoError(t, err)

			// Wire the actual DLQ by re-applying the policy (ensures ARN resolution).
			err = b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
				QueueURL: srcOut.QueueURL,
				Attributes: map[string]string{
					"RedrivePolicy": fmt.Sprintf(
						`{"deadLetterTargetArn":%q,"maxReceiveCount":%d}`,
						dlqARN, tc.maxReceiveCount,
					),
				},
			})
			require.NoError(t, err)

			// Send a message.
			_, err = b.SendMessage(&sqs.SendMessageInput{
				QueueURL:    srcOut.QueueURL,
				MessageBody: "redrive-me",
			})
			require.NoError(t, err)

			// Receive and immediately return to queue (VT=0) N times.
			// Each ChangeMessageVisibility(0) re-queues the message immediately so the
			// next receive can pick it up and increment ApproximateReceiveCount again.
			for range tc.receiveAttempts {
				out, recvErr := b.ReceiveMessage(&sqs.ReceiveMessageInput{
					QueueURL:            srcOut.QueueURL,
					MaxNumberOfMessages: 1,
					VisibilityTimeout:   0,
				})
				if recvErr != nil || len(out.Messages) == 0 {
					break
				}

				// Always re-queue immediately so the next pass increments the count.
				_ = b.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
					QueueURL:          srcOut.QueueURL,
					ReceiptHandle:     out.Messages[0].ReceiptHandle,
					VisibilityTimeout: 0,
				})
			}

			// Trigger one more ReceiveMessage on the source queue. The DLQ redrive is lazy:
			// it happens inside pickVisibleMessages, so a final call is required to sweep
			// the now-expired in-flight message back to the visible queue and drain it to the DLQ.
			_, _ = b.ReceiveMessage(&sqs.ReceiveMessageInput{
				QueueURL:            srcOut.QueueURL,
				MaxNumberOfMessages: 1,
				VisibilityTimeout:   0,
			})

			// Check DLQ.
			dlqOut2, recvErr := b.ReceiveMessage(&sqs.ReceiveMessageInput{
				QueueURL:            dlqOut.QueueURL,
				MaxNumberOfMessages: 1,
			})
			require.NoError(t, recvErr)

			if tc.wantInDLQ {
				assert.Len(t, dlqOut2.Messages, 1, "message must be in DLQ after maxReceiveCount exceeded")
			} else {
				assert.Empty(t, dlqOut2.Messages, "message must NOT be in DLQ yet")
			}
		})
	}
}

// TestDLQ_AutoRedrive_SetsDeadLetterQueueSourceArn verifies that a message
// auto-redriven into a DLQ (MaxReceiveCount exceeded) carries the
// DeadLetterQueueSourceArn system attribute set to the ARN of the queue it
// was redriven from, matching real AWS's MessageSystemAttributeName enum
// (aws-sdk-go-v2/service/sqs/types.MessageSystemAttributeNameDeadLetterQueueSourceArn).
// A message that never leaves its source queue must not carry the attribute.
func TestDLQ_AutoRedrive_SetsDeadLetterQueueSourceArn(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	dlqOut, err := b.CreateQueue(&sqs.CreateQueueInput{
		QueueName: "dlq",
		Endpoint:  testEndpoint,
	})
	require.NoError(t, err)

	dlqARN := "arn:aws:sqs:us-east-1:000000000000:dlq"
	srcARN := "arn:aws:sqs:us-east-1:000000000000:src"

	srcOut, err := b.CreateQueue(&sqs.CreateQueueInput{
		QueueName: "src",
		Endpoint:  testEndpoint,
		Attributes: map[string]string{
			"RedrivePolicy": fmt.Sprintf(`{"deadLetterTargetArn":%q,"maxReceiveCount":1}`, dlqARN),
		},
	})
	require.NoError(t, err)

	_, err = b.SendMessage(&sqs.SendMessageInput{
		QueueURL:    srcOut.QueueURL,
		MessageBody: "redrive-me",
	})
	require.NoError(t, err)

	// First receive on the source queue: message is still in the source queue,
	// so it must NOT carry DeadLetterQueueSourceArn yet.
	srcRecv, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueURL:            srcOut.QueueURL,
		MaxNumberOfMessages: 1,
		VisibilityTimeout:   0,
		AttributeNames:      []string{"All"},
	})
	require.NoError(t, err)
	require.Len(t, srcRecv.Messages, 1)
	assert.NotContains(t, srcRecv.Messages[0].Attributes, "DeadLetterQueueSourceArn")

	// Re-queue immediately, then trigger the lazy redrive sweep with a second receive.
	require.NoError(t, b.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
		QueueURL:          srcOut.QueueURL,
		ReceiptHandle:     srcRecv.Messages[0].ReceiptHandle,
		VisibilityTimeout: 0,
	}))
	_, _ = b.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueURL:            srcOut.QueueURL,
		MaxNumberOfMessages: 1,
		VisibilityTimeout:   0,
	})

	dlqRecv, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueURL:            dlqOut.QueueURL,
		MaxNumberOfMessages: 1,
		AttributeNames:      []string{"All"},
	})
	require.NoError(t, err)
	require.Len(t, dlqRecv.Messages, 1, "message must have been auto-redriven to the DLQ")
	assert.Equal(t, srcARN, dlqRecv.Messages[0].Attributes["DeadLetterQueueSourceArn"],
		"DeadLetterQueueSourceArn must be the ARN of the queue the message was redriven from")
}

// TestSQS_DLQRedrive validates that StartMessageMoveTask, CancelMessageMoveTask,
// and ListMessageMoveTasks work correctly for the async DLQ redrive use case.
func TestSQS_DLQRedrive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup          func(t *testing.T, b *sqs.InMemoryBackend) (srcARN, destARN, srcURL string)
		wantErrIs      error
		name           string
		msgCount       int
		wantTaskHandle bool
		wantErr        bool
	}{
		{
			name:           "move_messages_from_dlq_to_source",
			msgCount:       3,
			wantTaskHandle: true,
			setup: func(t *testing.T, b *sqs.InMemoryBackend) (string, string, string) {
				t.Helper()

				dlqName := "dlq-redrive"
				srcName := "src-redrive"

				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: srcName, Endpoint: "localhost"})
				require.NoError(t, err)

				dlqARN := buildQueueARN(dlqName)
				srcARN := buildQueueARN(srcName)
				dlqURL := "http://localhost/000000000000/" + dlqName

				return dlqARN, srcARN, dlqURL
			},
		},
		{
			name:      "start_task_with_nonexistent_source_returns_error",
			msgCount:  0,
			wantErr:   true,
			wantErrIs: sqs.ErrQueueNotFound,
			setup: func(_ *testing.T, _ *sqs.InMemoryBackend) (string, string, string) {
				return buildQueueARN("nonexistent-dlq"), buildQueueARN("some-dest"), ""
			},
		},
		{
			name:      "start_second_task_for_same_source_returns_conflict",
			msgCount:  0, // setup already seeds messages
			wantErr:   true,
			wantErrIs: sqs.ErrMoveTaskAlreadyRunning,
			setup: func(t *testing.T, b *sqs.InMemoryBackend) (string, string, string) {
				t.Helper()

				dlqName := "dlq-conflict"
				destName := "dest-conflict"

				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: destName, Endpoint: "localhost"})
				require.NoError(t, err)

				dlqURL := "http://localhost/000000000000/" + dlqName

				dlqARN := buildQueueARN(dlqName)
				destARN := buildQueueARN(destName)

				// Pre-seed the source so the task stays RUNNING.
				for i := range 10 {
					_, sendErr := b.SendMessage(&sqs.SendMessageInput{
						QueueURL:    dlqURL,
						MessageBody: "conflict-" + strconv.Itoa(i),
					})
					require.NoError(t, sendErr)
				}

				// Start first task with rate limiting so it stays alive.
				_, err = b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
					SourceArn:                    dlqARN,
					DestinationArn:               destARN,
					MaxNumberOfMessagesPerSecond: 1,
				})
				require.NoError(t, err)

				// Return the same ARNs; the test will attempt to start a second task.
				return dlqARN, destARN, ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sqs.NewInMemoryBackend()
			t.Cleanup(b.Close)
			srcARN, destARN, srcURL := tt.setup(t, b)

			// Seed messages into the source queue if needed.
			for i := range tt.msgCount {
				_, err := b.SendMessage(&sqs.SendMessageInput{
					QueueURL:    srcURL,
					MessageBody: "msg-body-" + strconv.Itoa(i),
				})
				require.NoError(t, err)
			}

			out, err := b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
				SourceArn:      srcARN,
				DestinationArn: destARN,
			})

			if tt.wantErr {
				require.Error(t, err)

				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, out.TaskHandle)
		})
	}
}

func TestSQS_DLQRedrive_MovesMessages(t *testing.T) {
	t.Parallel()

	b := sqs.NewInMemoryBackend()
	t.Cleanup(b.Close)

	dlqName := "dlq-moves"
	destName := "dest-moves"

	_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
	require.NoError(t, err)

	_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: destName, Endpoint: "localhost"})
	require.NoError(t, err)

	dlqURL := "http://localhost/000000000000/" + dlqName
	destURL := "http://localhost/000000000000/" + destName
	dlqARN := buildQueueARN(dlqName)
	destARN := buildQueueARN(destName)

	const msgCount = 5

	for i := range msgCount {
		_, sendErr := b.SendMessage(&sqs.SendMessageInput{
			QueueURL:    dlqURL,
			MessageBody: "message-" + strconv.Itoa(i),
		})
		require.NoError(t, sendErr)
	}

	out, err := b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
		SourceArn:      dlqARN,
		DestinationArn: destARN,
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.TaskHandle)

	// Wait for the goroutine to drain the DLQ.
	// Per AWS semantics, TaskHandle is only populated for RUNNING tasks in the
	// ListMessageMoveTasks response, so we check Status directly.
	require.Eventually(t, func() bool {
		listOut, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
			SourceArn:  dlqARN,
			MaxResults: 1,
		})
		if listErr != nil {
			return false
		}

		for _, task := range listOut.Results {
			if task.Status == sqs.MoveTaskStatusCompleted {
				return true
			}
		}

		return false
	}, 5*time.Second, 50*time.Millisecond, "task should complete")

	// DLQ should now be empty.
	dlqCheck, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueURL:            dlqURL,
		MaxNumberOfMessages: 10,
	})
	require.NoError(t, err)
	assert.Empty(t, dlqCheck.Messages, "DLQ should be empty after redrive")

	// Destination queue should contain all moved messages.
	destCheck, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueURL:            destURL,
		MaxNumberOfMessages: 10,
	})
	require.NoError(t, err)
	assert.Len(t, destCheck.Messages, msgCount, "destination queue should have all messages")
}

func TestSQS_DLQRedrive_DefaultDestination(t *testing.T) {
	t.Parallel()

	b := sqs.NewInMemoryBackend()
	t.Cleanup(b.Close)

	dlqName := "dlq-default-dest"
	srcName := "src-default-dest"

	_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
	require.NoError(t, err)

	dlqARN := buildQueueARN(dlqName)
	dlqURL := "http://localhost/000000000000/" + dlqName
	srcURL := "http://localhost/000000000000/" + srcName

	// Create source queue with a RedrivePolicy pointing to the DLQ.
	redrivePolicy, _ := json.Marshal(map[string]any{
		"deadLetterTargetArn": dlqARN,
		"maxReceiveCount":     3,
	})

	_, err = b.CreateQueue(&sqs.CreateQueueInput{
		QueueName: srcName,
		Endpoint:  "localhost",
		Attributes: map[string]string{
			"RedrivePolicy": string(redrivePolicy),
		},
	})
	require.NoError(t, err)

	// Put a message in the DLQ.
	_, err = b.SendMessage(&sqs.SendMessageInput{QueueURL: dlqURL, MessageBody: "redrive-me"})
	require.NoError(t, err)

	// Start task without specifying DestinationArn — it should default to srcName.
	out, err := b.StartMessageMoveTask(&sqs.StartMessageMoveTaskInput{
		SourceArn: dlqARN,
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.TaskHandle)

	// Wait for completion.
	// Per AWS semantics, TaskHandle is only populated for RUNNING tasks.
	require.Eventually(t, func() bool {
		listOut, listErr := b.ListMessageMoveTasks(&sqs.ListMessageMoveTasksInput{
			SourceArn:  dlqARN,
			MaxResults: 1,
		})
		if listErr != nil {
			return false
		}

		for _, task := range listOut.Results {
			if task.Status == sqs.MoveTaskStatusCompleted {
				return true
			}
		}

		return false
	}, 5*time.Second, 50*time.Millisecond, "task should complete")

	// Source queue should have the re-driven message.
	srcCheck, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueURL:            srcURL,
		MaxNumberOfMessages: 1,
	})
	require.NoError(t, err)
	assert.Len(t, srcCheck.Messages, 1)

	if len(srcCheck.Messages) > 0 {
		assert.Equal(t, "redrive-me", srcCheck.Messages[0].Body)
	}
}

// TestDLQ_SelfReferentialRedrivePolicy_DoesNotDeadlock covers a queue whose
// RedrivePolicy names itself as the dead-letter target. tryRouteToDLQ locks
// q.dlq.mu while the caller already holds q.mu (Caller must hold q.mu, per
// its doc comment). When q.dlq == q, that second lock targets the same
// non-reentrant sync.Mutex the caller is already holding, so the call never
// returns — wedging q.mu forever and hanging every future operation on this
// queue. This is a genuine goroutine-level deadlock (blocked on a real
// sync.Mutex), not a timing issue, so synctest's fake-clock deadlock
// detection does not apply here (confirmed: the unfixed call hangs
// indefinitely rather than being reported by synctest) — a bounded real
// timeout via a completion channel is the correct tool.
func TestDLQ_SelfReferentialRedrivePolicy_DoesNotDeadlock(t *testing.T) {
	t.Parallel()

	b := newBackend(t)

	qURL := createTestQueue(t, b, "self-dlq")

	attrs, err := b.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		QueueURL:       qURL,
		AttributeNames: []string{"QueueArn"},
	})
	require.NoError(t, err)
	selfARN := attrs.Attributes["QueueArn"]

	require.NoError(t, b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
		QueueURL: qURL,
		Attributes: map[string]string{
			"RedrivePolicy": `{"deadLetterTargetArn":"` + selfARN + `","maxReceiveCount":1}`,
		},
	}))

	_, err = b.SendMessage(&sqs.SendMessageInput{QueueURL: qURL, MessageBody: "self-loop"})
	require.NoError(t, err)

	out, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueURL:            qURL,
		MaxNumberOfMessages: 1,
		VisibilityTimeout:   0,
	})
	require.NoError(t, err)
	require.Len(t, out.Messages, 1)

	// ApproximateReceiveCount is now 1, equal to MaxReceiveCount. Resetting
	// visibility to 0 drives tryRouteToDLQ inline from inside
	// ChangeMessageVisibility, which must not deadlock even though the DLQ
	// target is this same queue. Run off the test goroutine so an unfixed
	// deadlock reports as a test failure instead of hanging the whole suite.
	done := make(chan error, 1)
	go func() {
		done <- b.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
			QueueURL:          qURL,
			ReceiptHandle:     out.Messages[0].ReceiptHandle,
			VisibilityTimeout: 0,
		})
	}()

	select {
	case cmvErr := <-done:
		require.NoError(t, cmvErr)
	case <-time.After(5 * time.Second):
		t.Fatal("ChangeMessageVisibility deadlocked: queue configured as its own dead-letter target")
	}
}

func makeRedrivePolicy(dlqARN string, maxReceiveCount int) string {
	b, _ := json.Marshal(map[string]any{
		"deadLetterTargetArn": dlqARN,
		"maxReceiveCount":     maxReceiveCount,
	})

	return string(b)
}

// TestRedrivePolicy_DLQMovement merges the "policy set at creation" and
// "policy set via SetQueueAttributes" scenarios into one table-driven test.
func TestRedrivePolicy_DLQMovement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		msgBody           string
		maxReceiveCount   int
		setPolicyAtCreate bool
	}{
		{
			name:              "policy_set_at_creation",
			maxReceiveCount:   2,
			msgBody:           "hello",
			setPolicyAtCreate: true,
		},
		{
			name:              "policy_set_via_set_queue_attributes",
			maxReceiveCount:   1,
			msgBody:           "test",
			setPolicyAtCreate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sqs.NewInMemoryBackend()
			t.Cleanup(b.Close)

			dlqName := "dlq-" + tt.name
			mainName := "main-" + tt.name
			dlqARN := "arn:aws:sqs:us-east-1:000000000000:" + dlqName
			mainURL := "http://localhost/000000000000/" + mainName
			dlqURL := "http://localhost/000000000000/" + dlqName

			_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: dlqName, Endpoint: "localhost"})
			require.NoError(t, err)

			if tt.setPolicyAtCreate {
				_, err = b.CreateQueue(&sqs.CreateQueueInput{
					QueueName: mainName,
					Endpoint:  "localhost",
					Attributes: map[string]string{
						"RedrivePolicy": makeRedrivePolicy(dlqARN, tt.maxReceiveCount),
					},
				})
				require.NoError(t, err)
			} else {
				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: mainName, Endpoint: "localhost"})
				require.NoError(t, err)

				err = b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
					QueueURL: mainURL,
					Attributes: map[string]string{
						"RedrivePolicy": makeRedrivePolicy(dlqARN, tt.maxReceiveCount),
					},
				})
				require.NoError(t, err)
			}

			_, err = b.SendMessage(&sqs.SendMessageInput{QueueURL: mainURL, MessageBody: tt.msgBody})
			require.NoError(t, err)

			// Receive maxReceiveCount times — message should still be returned each time.
			for i := range tt.maxReceiveCount {
				out, receiveErr := b.ReceiveMessage(&sqs.ReceiveMessageInput{QueueURL: mainURL, MaxNumberOfMessages: 1})
				require.NoError(t, receiveErr)
				require.Len(t, out.Messages, 1, "receive %d should return message", i+1)

				visErr := b.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
					QueueURL:          mainURL,
					ReceiptHandle:     out.Messages[0].ReceiptHandle,
					VisibilityTimeout: 0,
				})
				require.NoError(t, visErr)
			}

			// Next receive should return empty — message has been moved to DLQ.
			out, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{QueueURL: mainURL, MaxNumberOfMessages: 1})
			require.NoError(t, err)
			assert.Empty(t, out.Messages, "message should have been moved to DLQ")

			// DLQ should contain the message.
			dlqOut, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{QueueURL: dlqURL, MaxNumberOfMessages: 1})
			require.NoError(t, err)
			require.Len(t, dlqOut.Messages, 1)
			assert.Equal(t, tt.msgBody, dlqOut.Messages[0].Body)
		})
	}
}

func TestRedrivePolicy_NoMovementWithoutDLQ(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		queueName  string
		msgBody    string
		iterations int
	}{
		{
			name:       "no_redrive_policy",
			queueName:  "plain-queue",
			msgBody:    "stay",
			iterations: 5,
		},
		{
			name:       "high_receive_count_no_dlq",
			queueName:  "plain-queue-high",
			msgBody:    "persistent",
			iterations: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sqs.NewInMemoryBackend()
			t.Cleanup(b.Close)

			_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: tt.queueName, Endpoint: "localhost"})
			require.NoError(t, err)

			qURL := "http://localhost/000000000000/" + tt.queueName

			_, err = b.SendMessage(&sqs.SendMessageInput{QueueURL: qURL, MessageBody: tt.msgBody})
			require.NoError(t, err)

			// Receive and re-enqueue iterations times — message must always come back.
			for i := range tt.iterations {
				out, receiveErr := b.ReceiveMessage(&sqs.ReceiveMessageInput{QueueURL: qURL, MaxNumberOfMessages: 1})
				require.NoError(t, receiveErr)
				require.Len(t, out.Messages, 1, "iteration %d", i)

				visErr := b.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
					QueueURL:          qURL,
					ReceiptHandle:     out.Messages[0].ReceiptHandle,
					VisibilityTimeout: 0,
				})
				require.NoError(t, visErr)
			}
		})
	}
}

func TestRedrivePolicy_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy string
	}{
		{
			name:   "malformed_json",
			policy: "{not valid json",
		},
		{
			name:   "empty_json_object_no_dlq_fields",
			policy: "{}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sqs.NewInMemoryBackend()
			t.Cleanup(b.Close)

			queueName := "bad-policy-" + tt.name

			_, err := b.CreateQueue(&sqs.CreateQueueInput{
				QueueName: queueName,
				Endpoint:  "localhost",
				Attributes: map[string]string{
					"RedrivePolicy": tt.policy,
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "Invalid value for the parameter RedrivePolicy.")
		})
	}
}

func TestListDeadLetterSourceQueues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, b *sqs.InMemoryBackend) (dlqURL string)
		name     string
		wantURLs []string
		wantErr  bool
	}{
		{
			name: "two_source_queues_point_to_dlq",
			setup: func(t *testing.T, b *sqs.InMemoryBackend) string {
				t.Helper()

				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: "dlq", Endpoint: "localhost"})
				require.NoError(t, err)

				dlqAttrs, err := b.GetQueueAttributes(&sqs.GetQueueAttributesInput{
					QueueURL:       "http://localhost/000000000000/dlq",
					AttributeNames: []string{"QueueArn"},
				})
				require.NoError(t, err)

				dlqARN := dlqAttrs.Attributes["QueueArn"]
				policy := makeRedrivePolicy(dlqARN, 3)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{
					QueueName:  "src-a",
					Endpoint:   "localhost",
					Attributes: map[string]string{"RedrivePolicy": policy},
				})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{
					QueueName:  "src-b",
					Endpoint:   "localhost",
					Attributes: map[string]string{"RedrivePolicy": policy},
				})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: "unrelated", Endpoint: "localhost"})
				require.NoError(t, err)

				return "http://localhost/000000000000/dlq"
			},
			wantURLs: []string{
				"http://localhost/000000000000/src-a",
				"http://localhost/000000000000/src-b",
			},
		},
		{
			name: "no_source_queues",
			setup: func(t *testing.T, b *sqs.InMemoryBackend) string {
				t.Helper()

				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: "lonely-dlq", Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: "plain", Endpoint: "localhost"})
				require.NoError(t, err)

				return "http://localhost/000000000000/lonely-dlq"
			},
			wantURLs: []string{},
		},
		{
			name: "dlq_not_found",
			setup: func(_ *testing.T, _ *sqs.InMemoryBackend) string {
				return "http://localhost/000000000000/nonexistent"
			},
			wantErr: true,
		},
		{
			name: "three_source_queues",
			setup: func(t *testing.T, b *sqs.InMemoryBackend) string {
				t.Helper()

				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: "page-dlq", Endpoint: "localhost"})
				require.NoError(t, err)

				dlqAttrs, err := b.GetQueueAttributes(&sqs.GetQueueAttributesInput{
					QueueURL:       "http://localhost/000000000000/page-dlq",
					AttributeNames: []string{"QueueArn"},
				})
				require.NoError(t, err)

				dlqARN := dlqAttrs.Attributes["QueueArn"]
				policy := makeRedrivePolicy(dlqARN, 2)

				for _, name := range []string{"pq-1", "pq-2", "pq-3"} {
					_, err = b.CreateQueue(&sqs.CreateQueueInput{
						QueueName:  name,
						Endpoint:   "localhost",
						Attributes: map[string]string{"RedrivePolicy": policy},
					})
					require.NoError(t, err)
				}

				return "http://localhost/000000000000/page-dlq"
			},
			wantURLs: []string{
				"http://localhost/000000000000/pq-1",
				"http://localhost/000000000000/pq-2",
				"http://localhost/000000000000/pq-3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sqs.NewInMemoryBackend()
			t.Cleanup(b.Close)
			dlqURL := tt.setup(t, b)

			out, err := b.ListDeadLetterSourceQueues(&sqs.ListDeadLetterSourceQueuesInput{
				QueueURL: dlqURL,
			})

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, sqs.ErrQueueNotFound)

				return
			}

			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantURLs, out.QueueURLs)
		})
	}
}

func TestListDeadLetterSourceQueues_Pagination(t *testing.T) {
	t.Parallel()

	b := sqs.NewInMemoryBackend()
	t.Cleanup(b.Close)

	_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: "p-dlq", Endpoint: "localhost"})
	require.NoError(t, err)

	dlqAttrs, err := b.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		QueueURL:       "http://localhost/000000000000/p-dlq",
		AttributeNames: []string{"QueueArn"},
	})
	require.NoError(t, err)

	dlqARN := dlqAttrs.Attributes["QueueArn"]
	policy := makeRedrivePolicy(dlqARN, 2)

	for _, name := range []string{"pp-1", "pp-2", "pp-3"} {
		_, err = b.CreateQueue(&sqs.CreateQueueInput{
			QueueName:  name,
			Endpoint:   "localhost",
			Attributes: map[string]string{"RedrivePolicy": policy},
		})
		require.NoError(t, err)
	}

	dlqURL := "http://localhost/000000000000/p-dlq"

	first, err := b.ListDeadLetterSourceQueues(&sqs.ListDeadLetterSourceQueuesInput{
		QueueURL:   dlqURL,
		MaxResults: 2,
	})
	require.NoError(t, err)
	assert.Len(t, first.QueueURLs, 2)
	assert.NotEmpty(t, first.NextToken)

	second, err := b.ListDeadLetterSourceQueues(&sqs.ListDeadLetterSourceQueuesInput{
		QueueURL:   dlqURL,
		MaxResults: 2,
		NextToken:  first.NextToken,
	})
	require.NoError(t, err)
	assert.Len(t, second.QueueURLs, 1)
	assert.Empty(t, second.NextToken)

	allURLs := make([]string, 0, len(first.QueueURLs)+len(second.QueueURLs))
	allURLs = append(allURLs, first.QueueURLs...)
	allURLs = append(allURLs, second.QueueURLs...)
	assert.ElementsMatch(t, []string{
		"http://localhost/000000000000/pp-1",
		"http://localhost/000000000000/pp-2",
		"http://localhost/000000000000/pp-3",
	}, allURLs)
}

func TestDLQ_Routing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, b *sqs.InMemoryBackend)
		name string
	}{
		{
			name: "LazyRouting_EagerApply",
			run: func(t *testing.T, b *sqs.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: "dlq", Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: "source", Endpoint: "localhost"})
				require.NoError(t, err)

				qURL := "http://localhost/000000000000/source"
				dlqURL := "http://localhost/000000000000/dlq"

				// 1. Send message
				_, err = b.SendMessage(&sqs.SendMessageInput{QueueURL: qURL, MessageBody: "hello"})
				require.NoError(t, err)

				// 2. Receive message (ReceiveCount goes to 1)
				out, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{QueueURL: qURL, MaxNumberOfMessages: 1})
				require.NoError(t, err)
				require.Len(t, out.Messages, 1)

				// 3. Return it to queue instantly via ChangeMessageVisibility
				err = b.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
					QueueURL:          qURL,
					ReceiptHandle:     out.Messages[0].ReceiptHandle,
					VisibilityTimeout: 0,
				})
				require.NoError(t, err)

				// 4. Set RedrivePolicy with maxReceiveCount=1
				dlqARN := "arn:aws:sqs:us-east-1:000000000000:dlq"
				err = b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
					QueueURL: qURL,
					Attributes: map[string]string{
						"RedrivePolicy": `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":1}`,
					},
				})
				require.NoError(t, err)

				// At this point, the message in `source` has ReceiveCount=1, which >= MaxReceiveCount=1.
				dlqOut, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{QueueURL: dlqURL, MaxNumberOfMessages: 1})
				require.NoError(t, err)

				require.Len(t, dlqOut.Messages, 1, "Message should be in DLQ immediately")
			},
		},
		{
			name: "ReceiveMaxRepro",
			run: func(t *testing.T, b *sqs.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: "dlq2", Endpoint: "localhost"})
				require.NoError(t, err)

				dlqARN := "arn:aws:sqs:us-east-1:000000000000:dlq2"
				_, err = b.CreateQueue(&sqs.CreateQueueInput{
					QueueName: "source2",
					Endpoint:  "localhost",
					Attributes: map[string]string{
						"RedrivePolicy": `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":1}`,
					},
				})
				require.NoError(t, err)

				qURL := "http://localhost/000000000000/source2"

				_, err = b.SendMessage(&sqs.SendMessageInput{QueueURL: qURL, MessageBody: "hello"})
				require.NoError(t, err)

				// If maxReceiveCount is 1, AWS says the FIRST receive gets it.
				out, err := b.ReceiveMessage(&sqs.ReceiveMessageInput{QueueURL: qURL, MaxNumberOfMessages: 1})
				require.NoError(t, err)

				require.Len(t, out.Messages, 1, "Message should be received successfully")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := sqs.NewInMemoryBackend()
			defer b.Close()
			tt.run(t, b)
		})
	}
}

// TestRedriveAllowPolicy_Enforcement verifies that a dead-letter queue's
// RedriveAllowPolicy attribute actually constrains which source queues may
// point their RedrivePolicy at it. Previously the attribute was accepted and
// shape-validated (validateRedriveAllowPolicy) but never enforced, making it
// a disguised stub: any value could be set with zero effect on behaviour.
func TestRedriveAllowPolicy_Enforcement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, b *sqs.InMemoryBackend)
		name string
	}{
		{
			name: "DenyAll_RejectsAnySourceQueue",
			run: func(t *testing.T, b *sqs.InMemoryBackend) {
				t.Helper()

				_, err := b.CreateQueue(&sqs.CreateQueueInput{
					QueueName: "denyall-dlq",
					Endpoint:  "localhost",
					Attributes: map[string]string{
						"RedriveAllowPolicy": `{"redrivePermission":"denyAll"}`,
					},
				})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: "denyall-source", Endpoint: "localhost"})
				require.NoError(t, err)

				dlqARN := "arn:aws:sqs:us-east-1:000000000000:denyall-dlq"
				err = b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
					QueueURL: "http://localhost/000000000000/denyall-source",
					Attributes: map[string]string{
						"RedrivePolicy": `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":3}`,
					},
				})
				require.Error(t, err, "denyAll must reject every source queue, including this one")
			},
		},
		{
			name: "ByQueue_AllowsListedSourceQueueArn",
			run: func(t *testing.T, b *sqs.InMemoryBackend) {
				t.Helper()

				sourceARN := "arn:aws:sqs:us-east-1:000000000000:byqueue-allowed-source"

				_, err := b.CreateQueue(&sqs.CreateQueueInput{
					QueueName: "byqueue-dlq",
					Endpoint:  "localhost",
					Attributes: map[string]string{
						"RedriveAllowPolicy": `{"redrivePermission":"byQueue","sourceQueueArns":["` + sourceARN + `"]}`,
					},
				})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{
					QueueName: "byqueue-allowed-source",
					Endpoint:  "localhost",
				})
				require.NoError(t, err)

				dlqARN := "arn:aws:sqs:us-east-1:000000000000:byqueue-dlq"
				err = b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
					QueueURL: "http://localhost/000000000000/byqueue-allowed-source",
					Attributes: map[string]string{
						"RedrivePolicy": `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":3}`,
					},
				})
				require.NoError(t, err, "the ARN listed in sourceQueueArns must be permitted")
			},
		},
		{
			name: "ByQueue_RejectsUnlistedSourceQueueArn",
			run: func(t *testing.T, b *sqs.InMemoryBackend) {
				t.Helper()

				_, err := b.CreateQueue(&sqs.CreateQueueInput{
					QueueName: "byqueue-strict-dlq",
					Endpoint:  "localhost",
					Attributes: map[string]string{
						"RedriveAllowPolicy": `{"redrivePermission":"byQueue",` +
							`"sourceQueueArns":["arn:aws:sqs:us-east-1:000000000000:some-other-queue"]}`,
					},
				})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: "byqueue-strict-source", Endpoint: "localhost"})
				require.NoError(t, err)

				dlqARN := "arn:aws:sqs:us-east-1:000000000000:byqueue-strict-dlq"
				err = b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
					QueueURL: "http://localhost/000000000000/byqueue-strict-source",
					Attributes: map[string]string{
						"RedrivePolicy": `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":3}`,
					},
				})
				require.Error(t, err, "an ARN absent from sourceQueueArns must be rejected")
			},
		},
		{
			name: "AllowAllDefault_PermitsAnySourceQueue",
			run: func(t *testing.T, b *sqs.InMemoryBackend) {
				t.Helper()

				// No RedriveAllowPolicy attribute at all — AWS's implicit default
				// is allowAll, so setting a RedrivePolicy must still succeed.
				_, err := b.CreateQueue(&sqs.CreateQueueInput{QueueName: "default-allow-dlq", Endpoint: "localhost"})
				require.NoError(t, err)

				_, err = b.CreateQueue(&sqs.CreateQueueInput{QueueName: "default-allow-source", Endpoint: "localhost"})
				require.NoError(t, err)

				dlqARN := "arn:aws:sqs:us-east-1:000000000000:default-allow-dlq"
				err = b.SetQueueAttributes(&sqs.SetQueueAttributesInput{
					QueueURL: "http://localhost/000000000000/default-allow-source",
					Attributes: map[string]string{
						"RedrivePolicy": `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":3}`,
					},
				})
				require.NoError(t, err)
			},
		},
		{
			name: "DenyAll_AlsoRejectedAtCreateQueueTime",
			run: func(t *testing.T, b *sqs.InMemoryBackend) {
				t.Helper()

				_, err := b.CreateQueue(&sqs.CreateQueueInput{
					QueueName: "create-time-dlq",
					Endpoint:  "localhost",
					Attributes: map[string]string{
						"RedriveAllowPolicy": `{"redrivePermission":"denyAll"}`,
					},
				})
				require.NoError(t, err)

				dlqARN := "arn:aws:sqs:us-east-1:000000000000:create-time-dlq"

				// Setting RedrivePolicy directly at CreateQueue time must go through
				// the same enforcement path as SetQueueAttributes.
				_, err = b.CreateQueue(&sqs.CreateQueueInput{
					QueueName: "create-time-source",
					Endpoint:  "localhost",
					Attributes: map[string]string{
						"RedrivePolicy": `{"deadLetterTargetArn":"` + dlqARN + `","maxReceiveCount":3}`,
					},
				})
				require.Error(t, err, "denyAll must also block RedrivePolicy set inline at CreateQueue time")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := sqs.NewInMemoryBackendWithConfig("000000000000", "us-east-1")
			defer b.Close()
			tt.run(t, b)
		})
	}
}
