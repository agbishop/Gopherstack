package eventbridge_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	eventbridgesdk "github.com/aws/aws-sdk-go-v2/service/eventbridge"
	eventbridgetypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/eventbridge"
)

// TestPutRule_PerBusLimit_RealClient drives PutRule past the 300-rule
// per-bus limit through the real aws-sdk-go-v2 eventbridge client and
// asserts errors.As unwraps to *types.LimitExceededException. PutRule's own
// deserializeOpError switch (eventbridge@v1.48.4 deserializers.go) declares
// [ConcurrentModificationException, InternalException,
// InvalidEventPatternException, LimitExceededException,
// ManagedRuleException, ResourceNotFoundException] -- no
// ResourceLimitExceededException at all, and that string appears nowhere
// else in the module either, so a real client can never get a typed
// exception for the code this backend emits today.
func TestPutRule_PerBusLimit_RealClient(t *testing.T) {
	t.Parallel()

	b := newBackend()

	const limit = 300
	for i := range limit {
		_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
			Name:         fmt.Sprintf("rule-%d", i),
			EventPattern: `{"source":["x"]}`,
			State:        "ENABLED",
		})
		require.NoError(t, err, "rule %d should be created", i)
	}

	client := newTestEventBridgeClient(t, eventbridge.NewHandler(b))

	_, err := client.PutRule(t.Context(), &eventbridgesdk.PutRuleInput{
		Name:         aws.String("rule-overflow"),
		EventPattern: aws.String(`{"source":["x"]}`),
	}, func(o *eventbridgesdk.Options) { o.RetryMaxAttempts = 1 })
	require.Error(t, err)

	var limitExceeded *eventbridgetypes.LimitExceededException
	require.ErrorAs(t, err, &limitExceeded,
		"expected a real LimitExceededException from the SDK deserializer, got: %v", err)
}

// TestCreateEventBus_AccountLimit_RealClient is the CreateEventBus sibling
// of TestPutRule_PerBusLimit_RealClient: CreateEventBus shares the same
// eventbridge.ErrResourceLimitExceeded sentinel and the same
// handler_dispatch.go wire mapping, and its own deserializeOpError switch
// also declares LimitExceededException, not ResourceLimitExceededException.
func TestCreateEventBus_AccountLimit_RealClient(t *testing.T) {
	t.Parallel()

	b := newBackend()

	const limit = 200
	for i := range limit {
		_, err := b.CreateEventBus(
			context.Background(),
			eventbridge.CreateEventBusParams{Name: fmt.Sprintf("bus-%d", i)},
		)
		require.NoError(t, err, "bus %d should be created", i)
	}

	client := newTestEventBridgeClient(t, eventbridge.NewHandler(b))

	_, err := client.CreateEventBus(t.Context(), &eventbridgesdk.CreateEventBusInput{
		Name: aws.String("bus-overflow"),
	}, func(o *eventbridgesdk.Options) { o.RetryMaxAttempts = 1 })
	require.Error(t, err)

	var limitExceeded *eventbridgetypes.LimitExceededException
	require.ErrorAs(t, err, &limitExceeded,
		"expected a real LimitExceededException from the SDK deserializer, got: %v", err)
}
