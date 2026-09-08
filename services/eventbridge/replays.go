package eventbridge

import (
	"context"
	"fmt"
	"time"
)

// CancelReplay cancels a running or starting replay.
func (b *InMemoryBackend) CancelReplay(ctx context.Context, replayName string) (*Replay, error) {
	if replayName == "" {
		return nil, fmt.Errorf("%w: ReplayName is required", ErrInvalidParameter)
	}

	region := getRegionFromContext(ctx, b.region)

	b.mu.Lock("CancelReplay")
	defer b.mu.Unlock()

	replay, exists := b.replaysTable(region).Get(replayName)
	if !exists {
		return nil, fmt.Errorf("%w: replay %s not found", ErrNotFound, replayName)
	}

	if replay.State != "RUNNING" && replay.State != replayStateStarting {
		return nil, fmt.Errorf(
			"%w: replay %s is not in a cancellable state (current: %s)",
			ErrReplayNotCancellable,
			replayName,
			replay.State,
		)
	}

	replay.State = replayStateCancelling

	cp := *replay

	return &cp, nil
}

// DescribeReplay returns a single replay by name.
func (b *InMemoryBackend) DescribeReplay(ctx context.Context, name string) (*Replay, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: ReplayName is required", ErrInvalidParameter)
	}

	region := getRegionFromContext(ctx, b.region)

	b.mu.RLock("DescribeReplay")
	defer b.mu.RUnlock()

	replay, exists := b.replaysTable(region).Get(name)
	if !exists {
		return nil, fmt.Errorf("%w: replay %s not found", ErrNotFound, name)
	}

	cp := *replay

	return &cp, nil
}

// ListReplays returns replays optionally filtered by name prefix,
// EventSourceArn, and/or State, with pagination -- matching real
// ListReplaysInput's filter fields (eventbridge@v1.48.4
// api_op_ListReplays.go), previously parsed nowhere in this backend.
func (b *InMemoryBackend) ListReplays(
	ctx context.Context,
	namePrefix, eventSourceArn, state, nextToken string, limit int,
) ([]Replay, string, error) {
	region := getRegionFromContext(ctx, b.region)

	b.mu.RLock("ListReplays")
	defer b.mu.RUnlock()

	page, outToken := listNamedItems(
		b.replaysTable(region), namePrefix, eventSourceArn, state, nextToken, limit,
		func(r *Replay) string { return r.ReplayName },
		func(r *Replay) string { return r.EventSourceArn },
		func(r *Replay) string { return r.State },
		func(a, b Replay) bool { return a.ReplayName < b.ReplayName },
	)

	return page, outToken, nil
}

// StartReplay creates a new replay in the STARTING state.
func (b *InMemoryBackend) StartReplay(ctx context.Context, input StartReplayInput) (*Replay, error) {
	if input.ReplayName == "" {
		return nil, fmt.Errorf("%w: ReplayName is required", ErrInvalidParameter)
	}

	if input.EventSourceArn == "" {
		return nil, fmt.Errorf("%w: EventSourceArn is required", ErrInvalidParameter)
	}

	if !input.EventStartTime.IsZero() && !input.EventEndTime.IsZero() &&
		!input.EventStartTime.Before(input.EventEndTime) {
		return nil, fmt.Errorf(
			"%w: EventStartTime must be before EventEndTime",
			ErrInvalidParameter,
		)
	}

	region := getRegionFromContext(ctx, b.region)

	plan, err := b.startReplayLocked(region, input)
	if err != nil {
		return nil, err
	}

	// Deliver archived events asynchronously and mark the replay complete.
	if !b.closing.Load() {
		b.scheduleReplayWorker(plan, region, input.ReplayName)
	}

	return &plan.replay, nil
}

// replayDeliveryPlan carries the replay snapshot plus everything StartReplay
// needs to schedule the async delivery worker after releasing b.mu. Bundled
// into a struct (rather than startReplayLocked returning six-plus bare
// values) so adding a field -- as filterRuleARNs was -- doesn't require
// touching every positional return/call site.
type replayDeliveryPlan struct {
	ctx            context.Context
	filterRuleARNs map[string]struct{}
	targets        *DeliveryTargets
	workerSem      chan struct{}
	replay         Replay
	events         []EventEntry
	timeout        time.Duration
}

// startReplayLocked validates the new replay, records it, and collects the
// archived events to replay, all under b.mu. It returns the delivery plan
// StartReplay needs to schedule the async worker after releasing the lock.
// Extracted from StartReplay so the locked region is a plain method body
// rather than a function literal.
func (b *InMemoryBackend) startReplayLocked(
	region string,
	input StartReplayInput,
) (replayDeliveryPlan, error) {
	b.mu.Lock("StartReplay")
	defer b.mu.Unlock()

	replays := b.replaysTable(region)
	if replays.Has(input.ReplayName) {
		return replayDeliveryPlan{},
			fmt.Errorf("%w: replay %s already exists", ErrAlreadyExists, input.ReplayName)
	}

	// Validate destination ARN points to a known event bus.
	if input.Destination != nil && input.Destination.Arn != "" {
		found := false
		for _, bus := range b.busesTable(region).All() {
			if bus.Arn == input.Destination.Arn {
				found = true

				break
			}
		}
		if !found {
			return replayDeliveryPlan{}, fmt.Errorf(
				"%w: destination ARN %s does not match any event bus",
				ErrInvalidParameter,
				input.Destination.Arn,
			)
		}
	}

	// Find the archive by ARN (EventSourceArn points to an archive ARN).
	var archiveName string
	var archivePattern string
	for _, archive := range b.archivesTable(region).All() {
		if archive.ArchiveArn == input.EventSourceArn {
			archiveName = archive.ArchiveName
			archivePattern = archive.EventPattern

			break
		}
	}

	replay := &Replay{
		EventSourceArn:  input.EventSourceArn,
		EventStartTime:  input.EventStartTime,
		EventEndTime:    input.EventEndTime,
		Destination:     input.Destination,
		ReplayArn:       b.replayARN(input.ReplayName),
		ReplayName:      input.ReplayName,
		ReplayStartTime: time.Now(),
		State:           replayStateStarting,
		Description:     input.Description,
	}
	replays.Put(replay)

	// Collect archived events to replay filtered by time window and event pattern.
	eventsToReplay := b.filterArchivedEvents(
		region,
		archiveName,
		archivePattern,
		input.EventStartTime,
		input.EventEndTime,
	)

	return replayDeliveryPlan{
		replay:         *replay,
		events:         eventsToReplay,
		filterRuleARNs: buildFilterRuleARNs(input.Destination),
		targets:        b.deliveryTargets,
		workerSem:      b.workerSem,
		ctx:            b.ctx,
		timeout:        b.deliveryTimeout,
	}, nil
}

// buildFilterRuleARNs converts a ReplayDestination's FilterArns into the set
// buildDeliveryPlan checks membership against. Returns nil (no filtering, the
// AWS default of replaying to every matching rule) when dest is nil or
// FilterArns is empty.
func buildFilterRuleARNs(dest *ReplayDestination) map[string]struct{} {
	if dest == nil || len(dest.FilterArns) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(dest.FilterArns))
	for _, arn := range dest.FilterArns {
		set[arn] = struct{}{}
	}

	return set
}

// filterArchivedEvents returns archived events for the named archive filtered by
// time window [startTime, endTime) and optional event pattern.
// Must be called with b.mu held for reading.
func (b *InMemoryBackend) filterArchivedEvents(
	region, archiveName, pattern string,
	startTime, endTime time.Time,
) []EventEntry {
	if archiveName == "" {
		return nil
	}

	raw := b.archivedEventsStore(region)[archiveName]
	if len(raw) == 0 {
		return nil
	}

	result := make([]EventEntry, 0, len(raw))
	for _, e := range raw {
		t := time.Now()
		if e.Time != nil {
			t = *e.Time
		}
		if !startTime.IsZero() && t.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && !t.Before(endTime) {
			continue
		}
		if pattern != "" {
			envelope := buildEventEnvelope(e)
			if !matchPattern(pattern, envelope) {
				continue
			}
		}
		result = append(result, e)
	}

	return result
}

// scheduleReplayWorker launches a background goroutine that delivers archived events
// and then marks the replay COMPLETED. Extracted to reduce cognitive complexity of StartReplay.
func (b *InMemoryBackend) scheduleReplayWorker(plan replayDeliveryPlan, region, replayName string) {
	b.wg.Go(func() {
		select {
		case plan.workerSem <- struct{}{}:
			defer func() { <-plan.workerSem }()
		case <-plan.ctx.Done():
			return
		}

		if plan.targets != nil && len(plan.events) > 0 {
			b.deliverEvents(plan.ctx, region, plan.events, *plan.targets, plan.timeout, plan.filterRuleARNs)
		}

		b.mu.Lock("StartReplay-complete")
		defer b.mu.Unlock()

		r, ok := b.replaysTable(region).Get(replayName)
		if !ok {
			return
		}

		switch r.State {
		case replayStateStarting:
			r.State = replayStateCompleted
			r.ReplayEndTime = time.Now()
		case replayStateCancelling:
			r.State = replayStateCancelled
			r.ReplayEndTime = time.Now()
		}
	})
}

// AddReplayInternal adds a replay directly for testing.
func (b *InMemoryBackend) AddReplayInternal(replay *Replay) {
	b.mu.Lock("AddReplayInternal")
	defer b.mu.Unlock()

	if replay.ReplayArn == "" {
		replay.ReplayArn = b.replayARN(replay.ReplayName)
	}

	cp := *replay
	b.replaysTable(b.region).Put(&cp)
}
