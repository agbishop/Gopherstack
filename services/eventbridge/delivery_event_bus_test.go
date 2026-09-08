package eventbridge_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/eventbridge"
)

// TestDelivery_EventBusTarget_RoutesToTargetBusRules covers gopherstack-9iva:
// a rule targeting another event bus's ARN must actually deliver -- the
// routed event should re-enter the target bus's own rule matching and fan
// out to that bus's targets.
func TestDelivery_EventBusTarget_RoutesToTargetBusRules(t *testing.T) {
	t.Parallel()

	sqsMock := newMockSQSSender()
	backend := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	backend.SetDeliveryTargets(&eventbridge.DeliveryTargets{SQS: sqsMock})

	targetBus, err := backend.CreateEventBus(context.Background(), eventbridge.CreateEventBusParams{
		Name: "downstream-bus",
	})
	require.NoError(t, err)

	_, err = backend.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "downstream-rule",
		EventBusName: "downstream-bus",
		EventPattern: `{"source": ["cross.bus.test"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	const queueARN = "arn:aws:sqs:us-east-1:123456789012:downstream-queue"
	_, err = backend.PutTargets(context.Background(), "downstream-rule", "downstream-bus",
		[]eventbridge.Target{{ID: "t1", Arn: queueARN}})
	require.NoError(t, err)

	_, err = backend.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "router-rule",
		EventPattern: `{"source": ["cross.bus.test"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	_, err = backend.PutTargets(context.Background(), "router-rule", "default",
		[]eventbridge.Target{{ID: "t1", Arn: targetBus.Arn}})
	require.NoError(t, err)

	_, err = backend.PutEvents(context.Background(), []eventbridge.EventEntry{
		{Source: "cross.bus.test", DetailType: "Evt", Detail: `{"key":"value"}`},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(sqsMock.MessagesFor(queueARN)) > 0
	}, 2*time.Second, 10*time.Millisecond)

	msgs := sqsMock.MessagesFor(queueARN)
	assert.Len(t, msgs, 1)
	assert.Contains(t, msgs[0], "cross.bus.test")
}

// TestDelivery_EventBusTarget_UnmatchedRuleOnTargetBusNoDelivery is the
// negative case: routing to a real target bus whose own rule does not match
// must not deliver anything (proves routing goes through real rule matching,
// not a blind forward).
func TestDelivery_EventBusTarget_UnmatchedRuleOnTargetBusNoDelivery(t *testing.T) {
	t.Parallel()

	sqsMock := newMockSQSSender()
	backend := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	backend.SetDeliveryTargets(&eventbridge.DeliveryTargets{SQS: sqsMock})

	targetBus, err := backend.CreateEventBus(context.Background(), eventbridge.CreateEventBusParams{
		Name: "downstream-bus-2",
	})
	require.NoError(t, err)

	_, err = backend.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "downstream-rule-2",
		EventBusName: "downstream-bus-2",
		EventPattern: `{"source": ["never.matches"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	const queueARN = "arn:aws:sqs:us-east-1:123456789012:downstream-queue-2"
	_, err = backend.PutTargets(context.Background(), "downstream-rule-2", "downstream-bus-2",
		[]eventbridge.Target{{ID: "t1", Arn: queueARN}})
	require.NoError(t, err)

	_, err = backend.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "router-rule-2",
		EventPattern: `{"source": ["cross.bus.test.2"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	_, err = backend.PutTargets(context.Background(), "router-rule-2", "default",
		[]eventbridge.Target{{ID: "t1", Arn: targetBus.Arn}})
	require.NoError(t, err)

	_, err = backend.PutEvents(context.Background(), []eventbridge.EventEntry{
		{Source: "cross.bus.test.2", DetailType: "Evt", Detail: `{}`},
	})
	require.NoError(t, err)

	// The routed event should land in the target bus's event log even though
	// no rule there matches it -- proving RouteEventToBus really re-enters
	// PutEvents rather than silently no-oping.
	require.Eventually(t, func() bool {
		return backend.EventLogLen() >= 2
	}, 2*time.Second, 10*time.Millisecond)

	assert.Empty(t, sqsMock.MessagesFor(queueARN))
}

// TestRouteEventToBus_CrossAccountDropped covers the structural limit noted
// in gopherstack-9iva: this backend models a single AWS account, so a target
// ARN naming another account's event bus cannot be resolved and must be
// dropped (not error, not panic) -- matching how any other target ARN this
// backend cannot deliver to is handled (deliverToTarget's default case).
func TestRouteEventToBus_CrossAccountDropped(t *testing.T) {
	t.Parallel()

	backend := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	envelope := map[string]any{
		"source":      "cross.account.test",
		"detail-type": "Evt",
		"detail":      map[string]any{},
	}

	failed := backend.RouteEventToBus(context.Background(),
		"arn:aws:events:us-east-1:999999999999:event-bus/other-account-bus", envelope)

	assert.False(t, failed)
	assert.Zero(t, backend.EventLogLen())
}

// TestRouteEventToBus_HopLimitStopsLoop guards against a misconfigured
// cross-bus cycle (a bus's rule targeting itself, directly or transitively)
// recursing forever: once the hop count reaches the limit, RouteEventToBus
// must drop the event instead of calling PutEvents again.
func TestRouteEventToBus_HopLimitStopsLoop(t *testing.T) {
	t.Parallel()

	backend := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	targetBus, err := backend.CreateEventBus(context.Background(), eventbridge.CreateEventBusParams{
		Name: "loop-bus",
	})
	require.NoError(t, err)

	ctx := eventbridge.EventBusHopsContextForTest(context.Background(), eventbridge.MaxEventBusRoutingHopsForTest)

	envelope := map[string]any{
		"source":      "loop.test",
		"detail-type": "Evt",
		"detail":      map[string]any{},
	}

	failed := backend.RouteEventToBus(ctx, targetBus.Arn, envelope)

	assert.False(t, failed)
	assert.Zero(t, backend.EventLogLen())
}
