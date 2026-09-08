package eventbridge_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/eventbridge"
)

// TestDelivery_FallsBackToBusDLQWhenTargetHasNone proves a target with no
// DeadLetterConfig of its own lands its exhausted-retry event on the bus's
// configured DLQ instead of being silently dropped -- gopherstack-azq8.
func TestDelivery_FallsBackToBusDLQWhenTargetHasNone(t *testing.T) {
	t.Parallel()

	dlqSink := newMockSQSSender()
	dlqARN := "arn:aws:sqs:us-east-1:123456789012:bus-dlq"
	targetARN := "arn:aws:sqs:us-east-1:123456789012:my-queue"

	sender := &auditFailingSQSSender{delegate: dlqSink, failARN: targetARN}
	b := newBackend()
	b.SetDeliveryTargets(&eventbridge.DeliveryTargets{SQS: sender})

	ctx := context.Background()

	_, err := b.CreateEventBus(ctx, eventbridge.CreateEventBusParams{
		Name:             "bus-with-dlq",
		DeadLetterConfig: &eventbridge.DeadLetterConfig{Arn: dlqARN},
	})
	require.NoError(t, err)

	_, err = b.PutRule(ctx, eventbridge.PutRuleInput{
		Name:         "rule",
		EventBusName: "bus-with-dlq",
		EventPattern: `{"source":["dlq-fallback-test"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	_, err = b.PutTargets(ctx, "rule", "bus-with-dlq", []eventbridge.Target{
		{
			ID:          "t1",
			Arn:         targetARN,
			RetryPolicy: &eventbridge.RetryPolicy{MaximumRetryAttempts: 0},
		},
	})
	require.NoError(t, err)

	b.PutEvents(ctx, []eventbridge.EventEntry{
		{Source: "dlq-fallback-test", DetailType: "T", Detail: `{}`, EventBusName: "bus-with-dlq"},
	})

	require.Eventually(t, func() bool {
		return len(dlqSink.MessagesFor(dlqARN)) > 0
	}, 2*time.Second, 10*time.Millisecond, "bus DLQ should have received the failed event")
}
