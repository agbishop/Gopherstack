package eventbridge_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/eventbridge"
)

func TestCreateAndDescribeEventBus(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	bus, err := b.CreateEventBus(
		context.Background(),
		eventbridge.CreateEventBusParams{Name: "my-bus", Description: "a test bus"},
	)
	require.NoError(t, err)
	assert.Equal(t, "my-bus", bus.Name)
	assert.Contains(t, bus.Arn, "my-bus")

	got, err := b.DescribeEventBus(context.Background(), "my-bus")
	require.NoError(t, err)
	assert.Equal(t, "my-bus", got.Name)
	assert.Equal(t, "a test bus", got.Description)
}

func TestCreateEventBusAlreadyExists(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, err := b.CreateEventBus(
		context.Background(),
		eventbridge.CreateEventBusParams{Name: "dup-bus"},
	)
	require.NoError(t, err)

	_, err = b.CreateEventBus(
		context.Background(),
		eventbridge.CreateEventBusParams{Name: "dup-bus"},
	)
	require.ErrorIs(t, err, eventbridge.ErrEventBusAlreadyExists)
}

func TestDeleteEventBus(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, err := b.CreateEventBus(
		context.Background(),
		eventbridge.CreateEventBusParams{Name: "to-delete"},
	)
	require.NoError(t, err)

	err = b.DeleteEventBus(context.Background(), "to-delete")
	require.NoError(t, err)

	_, err = b.DescribeEventBus(context.Background(), "to-delete")
	require.ErrorIs(t, err, eventbridge.ErrEventBusNotFound)
}

func TestDeleteEventBus_ClearsPolicyOnRecreate(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	ctx := context.Background()

	_, err := b.CreateEventBus(ctx, eventbridge.CreateEventBusParams{Name: "reused-bus"})
	require.NoError(t, err)

	require.NoError(t, b.PutPermission(ctx, eventbridge.PutPermissionInput{
		EventBusName: "reused-bus",
		StatementID:  "AllowAccount",
		Action:       "events:PutEvents",
		Principal:    "111122223333",
	}))

	policy, err := b.GetEventBusPolicy(ctx, "reused-bus")
	require.NoError(t, err)
	require.NotEmpty(t, policy)

	require.NoError(t, b.DeleteEventBus(ctx, "reused-bus"))

	_, err = b.CreateEventBus(ctx, eventbridge.CreateEventBusParams{Name: "reused-bus"})
	require.NoError(t, err)

	recreatedPolicy, err := b.GetEventBusPolicy(ctx, "reused-bus")
	require.NoError(t, err)
	assert.Empty(t, recreatedPolicy)
}

func TestDeleteDefaultEventBusFails(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	err := b.DeleteEventBus(context.Background(), "default")
	require.ErrorIs(t, err, eventbridge.ErrCannotDeleteDefaultBus)
}

func TestListEventBuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		prefix        string
		wantFirstName string
		setupBuses    []string
		wantCount     int
	}{
		{
			name:       "All",
			setupBuses: []string{"alpha", "beta"},
			prefix:     "",
			// default + alpha + beta
			wantCount: 3,
		},
		{
			name:          "Prefix",
			setupBuses:    []string{"prod-bus", "dev-bus"},
			prefix:        "prod",
			wantCount:     1,
			wantFirstName: "prod-bus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			for _, name := range tt.setupBuses {
				_, _ = b.CreateEventBus(
					context.Background(),
					eventbridge.CreateEventBusParams{Name: name},
				)
			}

			buses, next, err := b.ListEventBuses(context.Background(), tt.prefix, "", 0)
			require.NoError(t, err)
			assert.Empty(t, next)
			assert.Len(t, buses, tt.wantCount)
			if tt.wantFirstName != "" {
				assert.Equal(t, tt.wantFirstName, buses[0].Name)
			}
		})
	}
}

func TestDescribeEventBus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		busName  string
		wantName string
	}{
		{name: "Default", busName: "default", wantName: "default"},
		// empty name should resolve to default
		{name: "EmptyName", busName: "", wantName: "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

			bus, err := b.DescribeEventBus(context.Background(), tt.busName)
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, bus.Name)
		})
	}
}

func TestPutAndListRules(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	input := eventbridge.PutRuleInput{
		Name:         "my-rule",
		EventBusName: "default",
		EventPattern: `{"source":["my.app"]}`,
		State:        "ENABLED",
	}
	rule, err := b.PutRule(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "my-rule", rule.Name)
	assert.Equal(t, "ENABLED", rule.State)
	assert.Contains(t, rule.Arn, "my-rule")

	rules, next, err := b.ListRules(context.Background(), "default", "", "", 0)
	require.NoError(t, err)
	assert.Empty(t, next)
	assert.Len(t, rules, 1)
}

func TestDescribeRule(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, err := b.PutRule(
		context.Background(),
		eventbridge.PutRuleInput{
			Name:         "r1",
			Description:  "desc",
			EventPattern: `{"source":["test"]}`,
		},
	)
	require.NoError(t, err)

	rule, err := b.DescribeRule(context.Background(), "r1", "")
	require.NoError(t, err)
	assert.Equal(t, "r1", rule.Name)
	assert.Equal(t, "desc", rule.Description)
}

func TestDeleteRule(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, err := b.PutRule(
		context.Background(),
		eventbridge.PutRuleInput{Name: "del-rule", ScheduleExpression: "rate(1 minute)"},
	)
	require.NoError(t, err)

	err = b.DeleteRule(context.Background(), "del-rule", "")
	require.NoError(t, err)

	_, err = b.DescribeRule(context.Background(), "del-rule", "")
	require.ErrorIs(t, err, eventbridge.ErrRuleNotFound)
}

func TestEnableDisableRule(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, err := b.PutRule(
		context.Background(),
		eventbridge.PutRuleInput{
			Name:         "toggle-rule",
			State:        "ENABLED",
			EventPattern: `{"source":["test"]}`,
		},
	)
	require.NoError(t, err)

	err = b.DisableRule(context.Background(), "toggle-rule", "")
	require.NoError(t, err)

	rule, err := b.DescribeRule(context.Background(), "toggle-rule", "")
	require.NoError(t, err)
	assert.Equal(t, "DISABLED", rule.State)

	err = b.EnableRule(context.Background(), "toggle-rule", "")
	require.NoError(t, err)

	rule, err = b.DescribeRule(context.Background(), "toggle-rule", "")
	require.NoError(t, err)
	assert.Equal(t, "ENABLED", rule.State)
}

func TestPutAndListTargets(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, err := b.PutRule(
		context.Background(),
		eventbridge.PutRuleInput{Name: "rule-with-targets", ScheduleExpression: "rate(1 minute)"},
	)
	require.NoError(t, err)

	targets := []eventbridge.Target{
		{ID: "t1", Arn: "arn:aws:lambda:us-east-1:123456789012:function:my-func"},
		{ID: "t2", Arn: "arn:aws:sqs:us-east-1:123456789012:my-queue"},
	}

	failed, err := b.PutTargets(context.Background(), "rule-with-targets", "", targets)
	require.NoError(t, err)
	assert.Empty(t, failed)

	got, next, err := b.ListTargetsByRule(context.Background(), "rule-with-targets", "", "", 0)
	require.NoError(t, err)
	assert.Empty(t, next)
	assert.Len(t, got, 2)
}

func TestRemoveTargets(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, err := b.PutRule(
		context.Background(),
		eventbridge.PutRuleInput{Name: "rule-remove", ScheduleExpression: "rate(1 minute)"},
	)
	require.NoError(t, err)

	_, err = b.PutTargets(context.Background(), "rule-remove", "", []eventbridge.Target{
		{ID: "t1", Arn: "arn:aws:lambda:us-east-1:123456789012:function:fn"},
	})
	require.NoError(t, err)

	failed, err := b.RemoveTargets(context.Background(), "rule-remove", "", []string{"t1"})
	require.NoError(t, err)
	assert.Empty(t, failed)

	got, _, err := b.ListTargetsByRule(context.Background(), "rule-remove", "", "", 0)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestPutEvents(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	now := time.Now()
	entries := []eventbridge.EventEntry{
		{Source: "my.app", DetailType: "UserCreated", Detail: `{"userId":"123"}`, Time: &now},
		{Source: "my.app", DetailType: "UserDeleted", Detail: `{"userId":"456"}`},
	}

	results, err := b.PutEvents(context.Background(), entries)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	for _, r := range results {
		assert.NotEmpty(t, r.EventID)
		assert.Empty(t, r.ErrorCode)
	}

	log := b.GetEventLog(context.Background())
	assert.Len(t, log, 2)
}

func TestEventLogMaxSize(t *testing.T) {
	t.Parallel()
	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	// Put 1100 events (in batches of 10, AWS's per-request PutEvents cap);
	// log should cap at 1000.
	const totalEvents = 1100

	const maxBatch = 10

	entry := eventbridge.EventEntry{Source: "s", DetailType: "t", Detail: "{}"}
	for sent := 0; sent < totalEvents; sent += maxBatch {
		n := min(maxBatch, totalEvents-sent)
		batch := make([]eventbridge.EventEntry, n)
		for i := range batch {
			batch[i] = entry
		}
		_, err := b.PutEvents(context.Background(), batch)
		require.NoError(t, err)
	}

	log := b.GetEventLog(context.Background())
	assert.Len(t, log, 1000)
}

func TestPutRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		input     eventbridge.PutRuleInput
		name      string
		wantState string
	}{
		{
			name: "DefaultState",
			input: eventbridge.PutRuleInput{
				Name:         "no-state-rule",
				EventPattern: `{"source":["test"]}`,
			},
			wantState: "ENABLED",
		},
		{
			name: "UnknownBus",
			input: eventbridge.PutRuleInput{
				Name:         "r",
				EventBusName: "nonexistent",
				EventPattern: `{"source":["test"]}`,
			},
			wantErr: eventbridge.ErrEventBusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

			rule, err := b.PutRule(context.Background(), tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantState, rule.State)
		})
	}
}

func TestScheduler_LastFiredCleanupOnDeleteRule(t *testing.T) {
	t.Parallel()

	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	defer b.Close()

	rule, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:               "sched-rule",
		ScheduleExpression: "rate(1 minute)",
		State:              "ENABLED",
		EventBusName:       "default",
	})
	require.NoError(t, err)

	// Seed lastFired as the scheduler would after init.
	lastFired := map[string]time.Time{
		rule.Arn: time.Now().Add(-2 * time.Minute),
	}

	// Delete the rule then run a scheduler tick.
	err = b.DeleteRule(context.Background(), "sched-rule", "default")
	require.NoError(t, err)

	sched := eventbridge.NewScheduler(b, 0)
	sched.ProcessTickForTest(t.Context(), time.Now(), lastFired)

	// The stale entry for the deleted rule must have been purged.
	_, still := lastFired[rule.Arn]
	assert.False(t, still, "lastFired should not contain entries for deleted rules")
}

func TestBackend_Close(t *testing.T) {
	t.Parallel()

	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	// Close should return without blocking even when no goroutines are active.
	b.Close()
}

func TestBackend_ResetRestoresDefaultEventBus(t *testing.T) {
	t.Parallel()

	b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	// Create a user-defined event bus and a rule.
	_, err := b.CreateEventBus(
		context.Background(),
		eventbridge.CreateEventBusParams{Name: "user-bus"},
	)
	require.NoError(t, err)

	_, err = b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "user-rule",
		EventPattern: `{"source":["test"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	// Reset clears user data.
	b.Reset()

	// User bus must be gone.
	_, err = b.DescribeEventBus(context.Background(), "user-bus")
	require.Error(t, err)

	// Default event bus must still exist so PutRule works.
	_, err = b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "post-reset-rule",
		EventBusName: "default",
		EventPattern: `{"source":["test"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err, "default event bus must be available after Reset")

	// Default bus must appear in ListEventBuses.
	buses, _, err := b.ListEventBuses(context.Background(), "", "", 0)
	require.NoError(t, err)
	assert.Len(t, buses, 1, "only the default bus should exist after Reset")
	assert.Equal(t, "default", buses[0].Name)
}

// blockedLambdaInvoker completely ignores context cancellation: it blocks on
// exit until the caller closes that channel. started is closed once the first
// InvokeFunction call begins, so tests can synchronize on it.
// This is used to drive the shutdown-timeout path in Close().
type blockedLambdaInvoker struct {
	started chan struct{}
	exit    chan struct{}
	once    sync.Once
}

func newBlockedLambdaInvoker() *blockedLambdaInvoker {
	return &blockedLambdaInvoker{
		started: make(chan struct{}),
		exit:    make(chan struct{}),
	}
}

func (h *blockedLambdaInvoker) InvokeFunction(
	_ context.Context,
	_ string,
	_ string,
	_ []byte,
) ([]byte, int, error) {
	h.once.Do(func() { close(h.started) })
	<-h.exit

	return []byte(`{}`), 200, nil
}

// contextAwareLambdaInvoker blocks until its context is cancelled. started is
// closed once the first InvokeFunction call begins.
// This is used to drive the delivery-timeout path via [context.WithTimeout].
type contextAwareLambdaInvoker struct {
	started chan struct{}
	once    sync.Once
}

func newContextAwareLambdaInvoker() *contextAwareLambdaInvoker {
	return &contextAwareLambdaInvoker{
		started: make(chan struct{}),
	}
}

func (h *contextAwareLambdaInvoker) InvokeFunction(
	ctx context.Context, _ string, _ string, _ []byte,
) ([]byte, int, error) {
	h.once.Do(func() { close(h.started) })
	<-ctx.Done()

	return nil, 0, ctx.Err()
}

func TestBackend_Close_ReturnsAfterShutdownTimeout_WhenDeliveryIsHung(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		shutdownTimeout time.Duration
		wantMaxDuration time.Duration
	}{
		{
			// deliveryTimeout is disabled (0) so only shutdownTimeout limits Close.
			// The blocked invoker ignores context — goroutines stay stuck until
			// exit is signalled after the test, forcing the shutdown-timeout path.
			name:            "close_returns_within_100ms_shutdown_timeout",
			shutdownTimeout: 100 * time.Millisecond,
			wantMaxDuration: 500 * time.Millisecond,
		},
		{
			name:            "close_returns_within_200ms_shutdown_timeout",
			shutdownTimeout: 200 * time.Millisecond,
			wantMaxDuration: 800 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			invoker := newBlockedLambdaInvoker()
			// Unblock the invoker goroutine after the test so it can terminate.
			defer close(invoker.exit)

			b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			b.SetShutdownTimeout(tt.shutdownTimeout)
			// Disable per-delivery timeout so only the shutdown timeout limits Close.
			b.SetDeliveryTimeout(0)
			b.SetDeliveryTargets(&eventbridge.DeliveryTargets{Lambda: invoker})

			_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:         "hung-rule",
				EventPattern: `{"source":["hung-test"]}`,
				State:        "ENABLED",
			})
			require.NoError(t, err)

			_, err = b.PutTargets(
				context.Background(),
				"hung-rule",
				"default",
				[]eventbridge.Target{
					{ID: "t1", Arn: "arn:aws:lambda:us-east-1:123456789012:function:my-fn"},
				},
			)
			require.NoError(t, err)

			b.PutEvents(context.Background(), []eventbridge.EventEntry{
				{Source: "hung-test", DetailType: "T", Detail: `{}`, EventBusName: "default"},
			})

			// Wait until delivery has actually started so Close() is guaranteed to
			// encounter an in-flight goroutine that is stuck in InvokeFunction.
			select {
			case <-invoker.started:
			case <-time.After(5 * time.Second):
				t.Fatal("delivery did not start within 5s")
			}

			start := time.Now()
			b.Close()
			elapsed := time.Since(start)

			assert.GreaterOrEqual(t, elapsed, tt.shutdownTimeout,
				"Close should wait at least shutdownTimeout before giving up, took %s", elapsed)
			assert.Less(t, elapsed, tt.wantMaxDuration,
				"Close should return within %s, took %s", tt.wantMaxDuration, elapsed)
		})
	}
}

func TestBackend_DeliveryTimeout_ContextPassedToTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		deliveryTimeout time.Duration
		wantMaxClose    time.Duration
	}{
		{
			// Invoker blocks until its context is cancelled. With a small
			// deliveryTimeout the delivery context fires quickly, the goroutine
			// unblocks, and Close() can return well before shutdownTimeout.
			name:            "50ms_delivery_timeout_unblocks_goroutine",
			deliveryTimeout: 50 * time.Millisecond,
			wantMaxClose:    500 * time.Millisecond,
		},
		{
			name:            "100ms_delivery_timeout_unblocks_goroutine",
			deliveryTimeout: 100 * time.Millisecond,
			wantMaxClose:    800 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			invoker := newContextAwareLambdaInvoker()

			b := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			b.SetDeliveryTimeout(tt.deliveryTimeout)
			b.SetShutdownTimeout(2 * time.Second)
			b.SetDeliveryTargets(&eventbridge.DeliveryTargets{Lambda: invoker})

			_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:         "timeout-rule",
				EventPattern: `{"source":["timeout-test"]}`,
				State:        "ENABLED",
			})
			require.NoError(t, err)

			_, err = b.PutTargets(
				context.Background(),
				"timeout-rule",
				"default",
				[]eventbridge.Target{
					{ID: "t1", Arn: "arn:aws:lambda:us-east-1:123456789012:function:my-fn"},
				},
			)
			require.NoError(t, err)

			b.PutEvents(context.Background(), []eventbridge.EventEntry{
				{Source: "timeout-test", DetailType: "T", Detail: `{}`, EventBusName: "default"},
			})

			// Wait until delivery has actually started before calling Close() so
			// the test is guaranteed to exercise the delivery-timeout path.
			select {
			case <-invoker.started:
			case <-time.After(5 * time.Second):
				t.Fatal("delivery did not start within 5s")
			}

			start := time.Now()
			b.Close()
			elapsed := time.Since(start)

			// The per-delivery context fires (deliveryTimeout) before the
			// shutdownTimeout, so Close() should return promptly.
			assert.Less(t, elapsed, tt.wantMaxClose,
				"Close should return promptly when delivery context is cancelled, took %s", elapsed)
		})
	}
}

func TestPutRule_PatternCompilationCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantSecondError error
		name            string
		firstPattern    string
		secondPattern   string
		wantCacheSize   int
	}{
		{
			name:          "same_pattern_reuses_cached_compilation",
			firstPattern:  `{"source":["svc-a"]}`,
			secondPattern: `{"source":["svc-a"]}`,
			wantCacheSize: 1,
		},
		{
			name:          "different_patterns_cache_separately",
			firstPattern:  `{"source":["svc-a"]}`,
			secondPattern: `{"source":["svc-b"]}`,
			wantCacheSize: 2,
		},
		{
			name:            "invalid_pattern_returns_invalid_parameter",
			firstPattern:    `{"source":["svc-a"]}`,
			secondPattern:   `{"source":[}`,
			wantCacheSize:   1,
			wantSecondError: eventbridge.ErrInvalidParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

			_, err := backend.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:         "rule-1",
				EventPattern: tt.firstPattern,
			})
			require.NoError(t, err)

			_, err = backend.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:         "rule-2",
				EventPattern: tt.secondPattern,
			})
			if tt.wantSecondError != nil {
				require.ErrorIs(t, err, tt.wantSecondError)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantCacheSize, backend.PatternCacheSize())
		})
	}
}

func TestPutRule_RuleIndexUpdatedOnRuleUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		firstSource string
		nextSource  string
	}{
		{
			name:        "updated_rule_stops_matching_old_source",
			firstSource: "source-a",
			nextSource:  "source-b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sqsSender := newMockSQSSender()
			backend := eventbridge.NewInMemoryBackend()
			backend.SetDeliveryTargets(&eventbridge.DeliveryTargets{SQS: sqsSender})

			_, err := backend.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:         "idx-rule",
				EventPattern: `{"source":["` + tt.firstSource + `"],"detail-type":["first"]}`,
				State:        "ENABLED",
			})
			require.NoError(t, err)

			_, err = backend.PutTargets(
				context.Background(),
				"idx-rule",
				"default",
				[]eventbridge.Target{
					{ID: "t1", Arn: "arn:aws:sqs:us-east-1:123456789012:index-queue"},
				},
			)
			require.NoError(t, err)

			_, err = backend.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:         "idx-rule",
				EventPattern: `{"source":["` + tt.nextSource + `"],"detail-type":["first"]}`,
				State:        "ENABLED",
			})
			require.NoError(t, err)

			backend.PutEvents(context.Background(), []eventbridge.EventEntry{
				{Source: tt.firstSource, DetailType: "first", Detail: `{}`},
				{Source: tt.nextSource, DetailType: "first", Detail: `{}`},
			})

			require.Eventually(t, func() bool {
				return len(
					sqsSender.MessagesFor("arn:aws:sqs:us-east-1:123456789012:index-queue"),
				) == 1
			}, 2*time.Second, 10*time.Millisecond)
		})
	}
}

func TestArchiveJanitor_SweepOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		archiveName    string
		retentionDays  int
		age            time.Duration
		expectArchived bool
	}{
		{
			name:           "expired_archive_is_removed",
			archiveName:    "expired",
			retentionDays:  1,
			age:            48 * time.Hour,
			expectArchived: false,
		},
		{
			name:           "archive_without_retention_is_kept",
			archiveName:    "keep-forever",
			retentionDays:  0,
			age:            365 * 24 * time.Hour,
			expectArchived: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			_, err := backend.CreateArchive(context.Background(), eventbridge.CreateArchiveInput{
				ArchiveName:    tt.archiveName,
				EventSourceArn: "arn:aws:events:us-east-1:123456789012:event-bus/default",
				RetentionDays:  tt.retentionDays,
			})
			require.NoError(t, err)

			err = backend.SetArchiveCreationTimeForTest(tt.archiveName, time.Now().Add(-tt.age))
			require.NoError(t, err)

			janitor := eventbridge.NewArchiveJanitor(backend, time.Millisecond)
			janitor.SweepOnce(t.Context())

			if tt.expectArchived {
				assert.Equal(t, 1, backend.ArchiveCount())
			} else {
				assert.Equal(t, 0, backend.ArchiveCount())
			}
		})
	}
}

func newBackend() *eventbridge.InMemoryBackend {
	return eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
}

func TestListPagination(t *testing.T) {
	t.Parallel()

	t.Run("list archives empty returns empty slice not nil", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		got, next, err := b.ListArchives(context.Background(), "", "", "", "", 0)
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.Empty(t, next)
	})

	t.Run("list connections empty returns empty slice not nil", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		got, next, err := b.ListConnections(context.Background(), "", "", "", 0)
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.Empty(t, next)
	})

	t.Run("list endpoints empty returns empty slice not nil", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		got, next, err := b.ListEndpoints(context.Background(), "", "", 0)
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.Empty(t, next)
	})

	t.Run("list replays empty returns empty slice not nil", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		got, next, err := b.ListReplays(context.Background(), "", "", "", "", 0)
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.Empty(t, next)
	})

	t.Run("list API destinations empty returns empty slice not nil", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		got, next, err := b.ListAPIDestinations(context.Background(), "", "", "", 0)
		require.NoError(t, err)
		assert.Empty(t, got)
		assert.Empty(t, next)
	})

	t.Run("list rule names by target with no targets returns empty", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		got, _, err := b.ListRuleNamesByTarget(
			context.Background(),
			"arn:aws:lambda:us-east-1:123:function:fn",
			"",
			"",
			0,
		)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

// TestSeedHelpers verifies all seed helpers increment their counts.
func TestSeedHelpers(t *testing.T) {
	t.Parallel()

	b := eventbridge.NewInMemoryBackend()
	now := time.Now()

	b.AddAPIDestinationInternal(&eventbridge.APIDestination{Name: "d1"})
	b.AddArchiveInternal(&eventbridge.Archive{ArchiveName: "a1"})
	b.AddConnectionInternal(&eventbridge.Connection{Name: "c1"})
	b.AddEndpointInternal(&eventbridge.Endpoint{Name: "e1"})
	b.AddEventSourceInternal(
		&eventbridge.EventSource{Name: "es1", State: "PENDING", CreationTime: now},
	)
	b.AddReplayInternal(&eventbridge.Replay{ReplayName: "r1", State: "RUNNING"})
	b.AddPartnerSourceInternal(&eventbridge.PartnerEventSource{Name: "p1"})

	assert.Equal(t, 1, b.APIDestinationCount())
	assert.Equal(t, 1, b.ArchiveCount())
	assert.Equal(t, 1, b.ConnectionCount())
	assert.Equal(t, 1, b.EndpointCount())
	assert.Equal(t, 1, b.EventSourceCount())
	assert.Equal(t, 1, b.ReplayCount())
	assert.Equal(t, 1, b.PartnerSourceCount())
}

// TestExportCountHelpers verifies count helpers start at 0.
func TestExportCountHelpers(t *testing.T) {
	t.Parallel()

	b := eventbridge.NewInMemoryBackend()

	assert.Equal(t, 0, b.APIDestinationCount())
	assert.Equal(t, 0, b.ArchiveCount())
	assert.Equal(t, 0, b.ConnectionCount())
	assert.Equal(t, 0, b.EndpointCount())
	assert.Equal(t, 0, b.EventSourceCount())
	assert.Equal(t, 0, b.ReplayCount())
	assert.Equal(t, 0, b.PartnerSourceCount())
}

// TestSeedHelper_DeepCopy verifies seed helpers store deep copies.
func TestSeedHelper_DeepCopy(t *testing.T) {
	t.Parallel()

	b := eventbridge.NewInMemoryBackend()
	orig := &eventbridge.APIDestination{Name: "d1", Description: "original"}
	b.AddAPIDestinationInternal(orig)

	// Mutate original after seeding
	orig.Description = "mutated"

	// Count should still be 1 and internal copy should have original value
	assert.Equal(t, 1, b.APIDestinationCount())
}

// TestBackend_ConcurrentReadNoRace is a regression test for a class of data
// races where a read-only method -- holding only b.mu.RLock() -- lazily
// initialised a per-region resource map (e.g. b.busePolicies[region] = ...)
// exactly like the Put*/Create* methods that hold the full b.mu.Lock().
// Concurrent RLock-holding readers could both observe a nil entry for a
// region no writer had touched for that particular map and race to write to
// the same outer map, which is a data race on a plain Go map regardless of
// the RWMutex, since RLock does not serialize readers against each other.
//
// Each case hammers one such read path from many workers against a bus that
// exists (so the read path passes its not-found check) but whose
// region-scoped side map (busePolicies, archivedEvents) has never been
// written. Run with `go test -race` to catch a regression of this class; it
// must pass cleanly with no race detected.
func TestBackend_ConcurrentReadNoRace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(b *eventbridge.InMemoryBackend, ctx context.Context)
		call  func(b *eventbridge.InMemoryBackend, ctx context.Context) error
		name  string
	}{
		{
			name: "get_event_bus_policy",
			setup: func(b *eventbridge.InMemoryBackend, ctx context.Context) {
				_, err := b.CreateEventBus(
					ctx,
					eventbridge.CreateEventBusParams{Name: "concurrent-bus"},
				)
				require.NoError(t, err)
			},
			call: func(b *eventbridge.InMemoryBackend, ctx context.Context) error {
				_, err := b.GetEventBusPolicy(ctx, "concurrent-bus")

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := eventbridge.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			ctx := context.Background()
			tt.setup(backend, ctx)

			const workers = 16
			const opsPerWorker = 25

			var wg sync.WaitGroup
			for range workers {
				wg.Go(func() {
					for range opsPerWorker {
						_ = tt.call(backend, ctx)
					}
				})
			}
			wg.Wait()
		})
	}
}
