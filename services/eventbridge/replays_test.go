package eventbridge_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/eventbridge"
)

func TestStartReplay_ValidatesDestination(t *testing.T) {
	t.Parallel()
	b := newBackend()

	b.AddArchiveInternal(&eventbridge.Archive{
		ArchiveName:    "test-archive",
		ArchiveArn:     "arn:aws:events:us-east-1:123456789012:archive/test-archive",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:event-bus/default",
		State:          "ACTIVE",
	})

	_, err := b.StartReplay(context.Background(), eventbridge.StartReplayInput{
		ReplayName:     "replay-1",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:archive/test-archive",
		EventStartTime: time.Now().Add(-time.Hour),
		EventEndTime:   time.Now(),
		Destination: &eventbridge.ReplayDestination{
			Arn: "arn:aws:events:us-east-1:123456789012:event-bus/nonexistent",
		},
	})
	require.ErrorIs(t, err, eventbridge.ErrInvalidParameter)
}

func TestStartReplay_ValidDestinationAccepted(t *testing.T) {
	t.Parallel()
	b := newBackend()

	b.AddArchiveInternal(&eventbridge.Archive{
		ArchiveName:    "test-archive",
		ArchiveArn:     "arn:aws:events:us-east-1:123456789012:archive/test-archive",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:event-bus/default",
		State:          "ACTIVE",
	})

	defaultBusARN := "arn:aws:events:us-east-1:123456789012:event-bus/default"

	replay, err := b.StartReplay(context.Background(), eventbridge.StartReplayInput{
		ReplayName:     "replay-2",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:archive/test-archive",
		EventStartTime: time.Now().Add(-time.Hour),
		EventEndTime:   time.Now(),
		Destination:    &eventbridge.ReplayDestination{Arn: defaultBusARN},
	})
	require.NoError(t, err)
	assert.Equal(t, "STARTING", replay.State)
}

func TestStartReplay_NoDestinationAccepted(t *testing.T) {
	t.Parallel()
	b := newBackend()

	b.AddArchiveInternal(&eventbridge.Archive{
		ArchiveName:    "arc",
		ArchiveArn:     "arn:aws:events:us-east-1:123456789012:archive/arc",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:event-bus/default",
		State:          "ACTIVE",
	})

	replay, err := b.StartReplay(context.Background(), eventbridge.StartReplayInput{
		ReplayName:     "replay-3",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:archive/arc",
		EventStartTime: time.Now().Add(-time.Hour),
		EventEndTime:   time.Now(),
	})
	require.NoError(t, err)
	assert.Equal(t, "STARTING", replay.State)
}

func TestReplay_CancelNonRunningFails(t *testing.T) {
	t.Parallel()
	b := newBackend()

	b.AddArchiveInternal(&eventbridge.Archive{
		ArchiveName:    "arc",
		ArchiveArn:     "arn:aws:events:us-east-1:123456789012:archive/arc",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:event-bus/default",
		State:          "ACTIVE",
	})

	_, err := b.StartReplay(context.Background(), eventbridge.StartReplayInput{
		ReplayName:     "my-replay",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:archive/arc",
		EventStartTime: time.Now().Add(-time.Hour),
		EventEndTime:   time.Now(),
	})
	require.NoError(t, err)

	// Wait for the background goroutine to finish the replay.
	require.Eventually(t, func() bool {
		r, descErr := b.DescribeReplay(context.Background(), "my-replay")

		return descErr == nil && r.State == "COMPLETED"
	}, 5*time.Second, 10*time.Millisecond)

	// Cancelling a COMPLETED replay must fail.
	_, err = b.CancelReplay(context.Background(), "my-replay")
	require.ErrorIs(t, err, eventbridge.ErrReplayNotCancellable)
}

func TestReplay_DuplicateNameFails(t *testing.T) {
	t.Parallel()
	b := newBackend()

	b.AddArchiveInternal(&eventbridge.Archive{
		ArchiveName:    "arc2",
		ArchiveArn:     "arn:aws:events:us-east-1:123456789012:archive/arc2",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:event-bus/default",
		State:          "ACTIVE",
	})

	_, err := b.StartReplay(context.Background(), eventbridge.StartReplayInput{
		ReplayName:     "dup-replay",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:archive/arc2",
		EventStartTime: time.Now().Add(-time.Hour),
		EventEndTime:   time.Now(),
	})
	require.NoError(t, err)

	_, err = b.StartReplay(context.Background(), eventbridge.StartReplayInput{
		ReplayName:     "dup-replay",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:archive/arc2",
		EventStartTime: time.Now().Add(-time.Hour),
		EventEndTime:   time.Now(),
	})
	require.ErrorIs(t, err, eventbridge.ErrAlreadyExists)
}

func TestReplay_ListWithPrefix(t *testing.T) {
	t.Parallel()
	b := newBackend()

	b.AddArchiveInternal(&eventbridge.Archive{
		ArchiveName:    "arc3",
		ArchiveArn:     "arn:aws:events:us-east-1:123456789012:archive/arc3",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:event-bus/default",
		State:          "ACTIVE",
	})

	for _, name := range []string{"prod-replay-1", "prod-replay-2", "dev-replay-1"} {
		_, err := b.StartReplay(context.Background(), eventbridge.StartReplayInput{
			ReplayName:     name,
			EventSourceArn: "arn:aws:events:us-east-1:123456789012:archive/arc3",
			EventStartTime: time.Now().Add(-time.Hour),
			EventEndTime:   time.Now(),
		})
		require.NoError(t, err)
	}

	replays, _, err := b.ListReplays(context.Background(), "prod-", "", "", "", 0)
	require.NoError(t, err)
	assert.Len(t, replays, 2)
}

func TestStartReplay_TimeOrdering(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	earlier := now.Add(-2 * time.Hour)
	later := now.Add(2 * time.Hour)

	tests := []struct {
		start   time.Time
		end     time.Time
		wantErr error
		name    string
	}{
		{name: "start before end ok", start: earlier, end: now, wantErr: nil},
		{name: "start equals end rejected", start: now, end: now, wantErr: eventbridge.ErrInvalidParameter},
		{name: "start after end rejected", start: later, end: earlier, wantErr: eventbridge.ErrInvalidParameter},
		{name: "both zero ok", start: time.Time{}, end: time.Time{}, wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend()
			_, err := b.StartReplay(context.Background(), eventbridge.StartReplayInput{
				ReplayName:     "r-" + tt.name,
				EventSourceArn: "arn:aws:events:us-east-1:123:archive/my-archive",
				EventStartTime: tt.start,
				EventEndTime:   tt.end,
			})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReplayCRUD(t *testing.T) {
	t.Parallel()

	t.Run("describe not found", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		_, err := b.DescribeReplay(context.Background(), "missing")
		require.ErrorIs(t, err, eventbridge.ErrNotFound)
	})

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		b := newBackend()

		start := time.Now().Add(-2 * time.Hour)
		end := time.Now().Add(-1 * time.Hour)

		created, err := b.StartReplay(context.Background(), eventbridge.StartReplayInput{
			ReplayName:     "my-replay",
			EventSourceArn: "arn:aws:events:us-east-1:123:archive/my-archive",
			EventStartTime: start,
			EventEndTime:   end,
		})
		require.NoError(t, err)
		assert.Equal(t, "my-replay", created.ReplayName)
		assert.Equal(t, "STARTING", created.State)

		got, err := b.DescribeReplay(context.Background(), "my-replay")
		require.NoError(t, err)
		assert.Equal(t, "my-replay", got.ReplayName)

		replays, _, err := b.ListReplays(context.Background(), "my-", "", "", "", 0)
		require.NoError(t, err)
		assert.Len(t, replays, 1)

		// Duplicate name rejected.
		_, err = b.StartReplay(context.Background(), eventbridge.StartReplayInput{
			ReplayName:     "my-replay",
			EventSourceArn: "arn:aws:events:us-east-1:123:archive/my-archive",
		})
		require.ErrorIs(t, err, eventbridge.ErrAlreadyExists)
	})
}

// TestCancelReplay_NotFound verifies non-existent replay returns ErrNotFound.
// TestCancelReplay covers not-found, terminal-state rejection, and the
// RUNNING/STARTING -> CANCELLING transition.
func TestCancelReplay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		seedState    string // empty means no replay is seeded (not-found case)
		wantErr      error
		wantState    string
		wantNoReplay bool
	}{
		{name: "not_found", wantNoReplay: true, wantErr: eventbridge.ErrNotFound},
		{name: "completed_is_terminal", seedState: "COMPLETED", wantErr: eventbridge.ErrReplayNotCancellable},
		{name: "cancelled_is_terminal", seedState: "CANCELLED", wantErr: eventbridge.ErrReplayNotCancellable},
		{name: "failed_is_terminal", seedState: "FAILED", wantErr: eventbridge.ErrReplayNotCancellable},
		{name: "running_transitions_to_cancelling", seedState: "RUNNING", wantState: "CANCELLING"},
		{name: "starting_transitions_to_cancelling", seedState: "STARTING", wantState: "CANCELLING"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := eventbridge.NewInMemoryBackend()
			if !tt.wantNoReplay {
				b.AddReplayInternal(&eventbridge.Replay{ReplayName: "r1", State: tt.seedState})
			}

			name := "r1"
			if tt.wantNoReplay {
				name = "no-such-replay"
			}

			replay, err := b.CancelReplay(context.Background(), name)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantState, replay.State)
		})
	}
}

// TestStartReplay_FilterArnsRestrictsDelivery proves
// StartReplayInput.Destination.FilterArns restricts replay delivery to only
// the named rule ARNs, even when other rules on the destination bus also
// match the replayed event's pattern -- matching real AWS's documented
// FilterArns semantics ("A list of ARNs for rules to replay events to").
// Previously FilterArns was silently dropped (not even modeled on
// ReplayDestination), so a replay always fanned out to every matching rule.
func TestStartReplay_FilterArnsRestrictsDelivery(t *testing.T) {
	t.Parallel()

	sqsMock := newMockSQSSender()
	b := setupDeliveryBackend(t, sqsMock, newMockLambdaInvoker())
	ctx := context.Background()

	defaultBus, err := b.DescribeEventBus(ctx, "")
	require.NoError(t, err)
	defaultBusARN := defaultBus.Arn
	queueA := "arn:aws:sqs:us-east-1:123456789012:queue-a"
	queueB := "arn:aws:sqs:us-east-1:123456789012:queue-b"

	_, err = b.CreateArchive(ctx, eventbridge.CreateArchiveInput{
		ArchiveName:    "filter-arns-archive",
		EventSourceArn: defaultBusARN,
	})
	require.NoError(t, err)

	ruleA, err := b.PutRule(ctx, eventbridge.PutRuleInput{
		Name:         "filter-rule-a",
		EventPattern: `{"source":["filter.test"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	ruleB, err := b.PutRule(ctx, eventbridge.PutRuleInput{
		Name:         "filter-rule-b",
		EventPattern: `{"source":["filter.test"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	_, err = b.PutTargets(ctx, ruleA.Name, "", []eventbridge.Target{{ID: "t1", Arn: queueA}})
	require.NoError(t, err)
	_, err = b.PutTargets(ctx, ruleB.Name, "", []eventbridge.Target{{ID: "t1", Arn: queueB}})
	require.NoError(t, err)

	// PutEvents both archives the event (async-captured synchronously under
	// the backend lock, see captureEventInArchives) and live-delivers it to
	// both rules -- that live fan-out is NOT subject to FilterArns, only the
	// replay is, so both queues get exactly one message from this call.
	_, err = b.PutEvents(ctx, []eventbridge.EventEntry{
		{Source: "filter.test", DetailType: "t", Detail: `{}`},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(sqsMock.MessagesFor(queueA)) == 1 && len(sqsMock.MessagesFor(queueB)) == 1
	}, 2*time.Second, 10*time.Millisecond, "live PutEvents delivery must reach both rules")

	archive, err := b.DescribeArchive(ctx, "filter-arns-archive")
	require.NoError(t, err)

	replay, err := b.StartReplay(ctx, eventbridge.StartReplayInput{
		ReplayName:     "filter-arns-replay",
		EventSourceArn: archive.ArchiveArn,
		Destination: &eventbridge.ReplayDestination{
			Arn:        defaultBusARN,
			FilterArns: []string{ruleA.Arn},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "STARTING", replay.State)

	require.Eventually(t, func() bool {
		r, descErr := b.DescribeReplay(ctx, "filter-arns-replay")

		return descErr == nil && r.State == "COMPLETED"
	}, 2*time.Second, 10*time.Millisecond, "replay must complete")

	// Replay must have delivered only to rule A's target (filtered in); rule
	// B's target must be untouched by the replay (filtered out) even though
	// its pattern matches too.
	assert.Len(t, sqsMock.MessagesFor(queueA), 2, "queue A: 1 live + 1 replayed")
	assert.Len(t, sqsMock.MessagesFor(queueB), 1, "queue B: 1 live only, replay excluded by FilterArns")
}

// blockingSQSSender blocks each SendMessageToQueue call until proceed is
// closed, and signals started the first time a call arrives -- used to hold
// a replay's async delivery worker open so a test can call CancelReplay
// while delivery is still in flight, deterministically instead of racing a
// background goroutine.
type blockingSQSSender struct {
	started chan struct{}
	proceed chan struct{}
}

func newBlockingSQSSender() *blockingSQSSender {
	return &blockingSQSSender{started: make(chan struct{}), proceed: make(chan struct{})}
}

func (b *blockingSQSSender) SendMessageToQueue(_ context.Context, _, _ string) error {
	close(b.started)
	<-b.proceed

	return nil
}

// TestCancelReplay_InFlightReplayReachesCancelled proves that cancelling a
// replay whose delivery worker is still running eventually settles in the
// terminal CANCELLED state, not stuck in CANCELLING forever. Real AWS moves
// a cancelled replay from CANCELLING to CANCELLED once cancellation
// completes (DescribeReplay's documented State enum includes CANCELLED).
// Previously CancelReplay only ever set CANCELLING: the async delivery
// worker's completion step checked exclusively for State == STARTING before
// writing COMPLETED, so once CancelReplay ran first the replay never left
// CANCELLING -- accepted on the wire, but the cancellation never finished.
func TestCancelReplay_InFlightReplayReachesCancelled(t *testing.T) {
	t.Parallel()
	b := newBackend()
	ctx := context.Background()

	defaultBus, err := b.DescribeEventBus(ctx, "")
	require.NoError(t, err)
	defaultBusARN := defaultBus.Arn

	_, err = b.CreateArchive(ctx, eventbridge.CreateArchiveInput{
		ArchiveName:    "cancel-inflight-archive",
		EventSourceArn: defaultBusARN,
	})
	require.NoError(t, err)

	// Archive the event before any rule exists, so this PutEvents does not
	// also fan out live delivery to the blocking target below.
	_, err = b.PutEvents(ctx, []eventbridge.EventEntry{
		{Source: "cancel.inflight.test", DetailType: "t", Detail: `{}`},
	})
	require.NoError(t, err)

	sender := newBlockingSQSSender()
	b.SetDeliveryTargets(&eventbridge.DeliveryTargets{SQS: sender})

	rule, err := b.PutRule(ctx, eventbridge.PutRuleInput{
		Name:         "cancel-inflight-rule",
		EventPattern: `{"source":["cancel.inflight.test"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	_, err = b.PutTargets(ctx, rule.Name, "", []eventbridge.Target{
		{ID: "t1", Arn: "arn:aws:sqs:us-east-1:123456789012:cancel-inflight-queue"},
	})
	require.NoError(t, err)

	archive, err := b.DescribeArchive(ctx, "cancel-inflight-archive")
	require.NoError(t, err)

	replay, err := b.StartReplay(ctx, eventbridge.StartReplayInput{
		ReplayName:     "cancel-inflight-replay",
		EventSourceArn: archive.ArchiveArn,
		Destination:    &eventbridge.ReplayDestination{Arn: defaultBusARN},
	})
	require.NoError(t, err)
	require.Equal(t, "STARTING", replay.State)

	// Wait until the replay's async worker is blocked inside delivery, then
	// cancel it while it's still in flight.
	select {
	case <-sender.started:
	case <-time.After(2 * time.Second):
		t.Fatal("replay delivery never started")
	}

	cancelled, err := b.CancelReplay(ctx, "cancel-inflight-replay")
	require.NoError(t, err)
	assert.Equal(t, "CANCELLING", cancelled.State)

	close(sender.proceed)

	require.Eventually(t, func() bool {
		r, descErr := b.DescribeReplay(ctx, "cancel-inflight-replay")

		return descErr == nil && r.State != "STARTING" && r.State != "CANCELLING"
	}, 2*time.Second, 10*time.Millisecond, "replay must leave the CANCELLING state")

	final, err := b.DescribeReplay(ctx, "cancel-inflight-replay")
	require.NoError(t, err)
	assert.Equal(t, "CANCELLED", final.State, "a cancelled in-flight replay must settle as CANCELLED, not COMPLETED")
}

// TestDescribeReplay_EchoesDestinationAndDescription proves DescribeReplay
// echoes Destination and Description -- both real DescribeReplayOutput
// members that ListReplays (real AWS's types.Replay) does not have.
// Previously the Replay model had neither field, so StartReplayInput.
// Description/Destination were silently discarded on every describe.
func TestDescribeReplay_EchoesDestinationAndDescription(t *testing.T) {
	t.Parallel()
	b := newBackend()
	ctx := context.Background()

	b.AddArchiveInternal(&eventbridge.Archive{
		ArchiveName:    "describe-echo-archive",
		ArchiveArn:     "arn:aws:events:us-east-1:123456789012:archive/describe-echo-archive",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:event-bus/default",
		State:          "ACTIVE",
	})

	defaultBusARN := "arn:aws:events:us-east-1:123456789012:event-bus/default"

	_, err := b.StartReplay(ctx, eventbridge.StartReplayInput{
		ReplayName:     "describe-echo-replay",
		EventSourceArn: "arn:aws:events:us-east-1:123456789012:archive/describe-echo-archive",
		Description:    "my replay description",
		Destination:    &eventbridge.ReplayDestination{Arn: defaultBusARN},
	})
	require.NoError(t, err)

	replay, err := b.DescribeReplay(ctx, "describe-echo-replay")
	require.NoError(t, err)
	assert.Equal(t, "my replay description", replay.Description)
	require.NotNil(t, replay.Destination)
	assert.Equal(t, defaultBusARN, replay.Destination.Arn)
}
