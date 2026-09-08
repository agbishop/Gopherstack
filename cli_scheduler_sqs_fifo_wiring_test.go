package main

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	schedulerbackend "github.com/blackbirdworks/gopherstack/services/scheduler"
	sqsbackend "github.com/blackbirdworks/gopherstack/services/sqs"
)

// TestInitializeServices_SchedulerSQSFIFOWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) and the real scheduler runner
// worker loop rather than invoking SetSQSFIFOSender directly.
//
// Regression test for the scheduler SetSQSFIFOSender wiring gap: without it,
// invokeSQSTarget falls back to the plain (non-FIFO) SendMessageToQueue path, which a real
// FIFO queue rejects for lacking a MessageGroupId (sqs.ErrMissingMessageGroupID). A schedule
// targeting a FIFO queue with MessageGroupId therefore never delivered a message.
func TestInitializeServices_SchedulerSQSFIFOWiring(t *testing.T) {
	t.Parallel()

	cli := &CLI{AccountID: "000000000000", Region: "us-east-1"}
	appCtx := &service.AppContext{
		Logger:     slog.Default(),
		Config:     cli,
		JanitorCtx: t.Context(),
	}
	cli.faultStore = chaos.NewFaultStore()

	services, err := initializeServices(appCtx)
	require.NoError(t, err)

	byName := serviceByName(services)

	schedH, ok := byName["Scheduler"].(*schedulerbackend.Handler)
	require.True(t, ok, "Scheduler handler must be registered")

	sqsH, ok := byName["SQS"].(*sqsbackend.Handler)
	require.True(t, ok, "SQS handler must be registered")

	sqsBk, ok := sqsH.Backend.(*sqsbackend.InMemoryBackend)
	require.True(t, ok, "SQS backend must be an InMemoryBackend")

	queueName := "scheduler-sqs-fifo-wiring-queue.fifo"
	createOut, err := sqsBk.CreateQueue(&sqsbackend.CreateQueueInput{
		QueueName: queueName,
		Endpoint:  "localhost",
		Attributes: map[string]string{
			"FifoQueue":                 "true",
			"ContentBasedDeduplication": "true",
		},
	})
	require.NoError(t, err)

	attrsOut, err := sqsBk.GetQueueAttributes(&sqsbackend.GetQueueAttributesInput{
		QueueURL:       createOut.QueueURL,
		AttributeNames: []string{"QueueArn"},
	})
	require.NoError(t, err)
	queueARN := attrsOut.Attributes["QueueArn"]
	require.NotEmpty(t, queueARN)

	schedBk, ok := schedH.Backend.(*schedulerbackend.InMemoryBackend)
	require.True(t, ok, "Scheduler backend must be an InMemoryBackend")

	_, err = schedBk.CreateSchedule(
		t.Context(),
		"scheduler-sqs-fifo-wiring-schedule", "", "rate(1 minute)", "", "",
		schedulerbackend.Target{
			ARN:     queueARN,
			RoleARN: "arn:aws:iam::000000000000:role/r",
			SqsParameters: &schedulerbackend.SqsParameters{
				MessageGroupID: "wiring-group",
			},
			Input: "sqs-fifo-wiring-payload",
		},
		"ENABLED", schedulerbackend.FlexibleTimeWindow{Mode: "OFF"},
	)
	require.NoError(t, err)

	require.NoError(t, schedH.StartWorker(t.Context()))
	t.Cleanup(func() { schedH.Shutdown(t.Context()) })

	require.Eventually(t, func() bool {
		out, recvErr := sqsBk.ReceiveMessage(&sqsbackend.ReceiveMessageInput{
			QueueURL:            createOut.QueueURL,
			MaxNumberOfMessages: 1,
		})

		return recvErr == nil && len(out.Messages) > 0
	}, 10*time.Second, 100*time.Millisecond,
		"a schedule targeting a FIFO SQS queue with MessageGroupId must actually deliver a "+
			"message through the real cli.go composition root's scheduler wiring "+
			"(wireSchedulerMessaging's SetSQSFIFOSender), not silently fall back to a non-FIFO "+
			"send that a real FIFO queue rejects")
}
