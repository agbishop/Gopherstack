package cloudwatchlogs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// validatePutLogEventsBatch checks the batch size constraints for PutLogEvents.
func validatePutLogEventsBatch(events []InputLogEvent) error {
	// AWS PutLogEvents limits per request:
	//   * up to 10,000 events
	//   * up to 1 MiB total batch size; each event is counted as
	//     len(message) + 26 bytes of overhead.
	const (
		maxEventsPerBatch  = 10000
		maxBatchBytes      = 1024 * 1024
		eventOverheadBytes = 26
	)

	if len(events) > maxEventsPerBatch {
		return fmt.Errorf("%w: PutLogEvents accepts at most %d events per request",
			ErrValidation, maxEventsPerBatch)
	}

	totalBytes := 0
	for i, e := range events {
		msgLen := len(e.Message)
		if msgLen > putLogEventsMaxMessageBytes {
			return fmt.Errorf(
				"%w: event at index %d exceeds maximum message size of %d bytes",
				ErrValidation, i, putLogEventsMaxMessageBytes,
			)
		}

		totalBytes += msgLen + eventOverheadBytes
	}

	if totalBytes > maxBatchBytes {
		return fmt.Errorf("%w: PutLogEvents batch size %d exceeds %d-byte limit",
			ErrValidation, totalBytes, maxBatchBytes)
	}

	return validateChronologicalOrder(events)
}

// validateChronologicalOrder rejects the whole PutLogEvents call when the
// batch is not in non-decreasing timestamp order, matching the current
// aws-sdk-go-v2 PutLogEvents doc comment: "A batch of log events in a single
// request must be in a chronological order. Otherwise, the operation
// fails." Unlike the too-old/too-new/expired classification in
// classifyLogEvents (which rejects individual events but still accepts the
// rest of the batch), an ordering violation fails the entire request before
// any event is stored. Synthetic test timestamps (before
// minRealisticTimestampMs) are exempted, matching the same bypass used
// throughout this file for fixture data authored with arbitrary timestamps.
func validateChronologicalOrder(events []InputLogEvent) error {
	for i := 1; i < len(events); i++ {
		prev, cur := events[i-1].Timestamp, events[i].Timestamp
		if prev < minRealisticTimestampMs || cur < minRealisticTimestampMs {
			continue
		}

		if cur < prev {
			return fmt.Errorf(
				"%w: log events in a single PutLogEvents request must be in chronological order",
				ErrValidation,
			)
		}
	}

	return nil
}

// putLogEventsMaxSpanMs is the documented maximum timestamp span across the
// *valid* events (those that survive the too-old/too-new/expired
// classification) within a single PutLogEvents batch. Per the SDK doc
// comment: "the time span in a single batch cannot exceed 24 hours.
// Otherwise, the operation fails".
const putLogEventsMaxSpanMs = 24 * 60 * 60 * 1000

// validateEventSpan rejects the whole PutLogEvents call when the valid
// (accepted) events span more than 24 hours from oldest to newest. Like
// validateChronologicalOrder, this fails the entire request rather than
// rejecting individual events. Synthetic test timestamps are excluded from
// the span calculation for the same fixture-friendliness reason.
func validateEventSpan(events []InputLogEvent) error {
	var minTS, maxTS int64

	found := false

	for _, e := range events {
		if e.Timestamp < minRealisticTimestampMs {
			continue
		}

		if !found {
			minTS, maxTS = e.Timestamp, e.Timestamp
			found = true

			continue
		}

		if e.Timestamp < minTS {
			minTS = e.Timestamp
		}

		if e.Timestamp > maxTS {
			maxTS = e.Timestamp
		}
	}

	if found && maxTS-minTS > putLogEventsMaxSpanMs {
		return fmt.Errorf(
			"%w: the time span of valid log events in a single PutLogEvents request cannot exceed 24 hours",
			ErrValidation,
		)
	}

	return nil
}

// rejectedTracker accumulates the three RejectedLogEventsInfo fields while
// classifyLogEvents walks a PutLogEvents batch in order. Per the real API
// (aws-sdk-go-v2 types.RejectedLogEventsInfo):
//   - TooNewLogEventStartIndex is the INCLUSIVE index of the first too-new event.
//   - TooOldLogEventEndIndex and ExpiredLogEventEndIndex are EXCLUSIVE end
//     indices: "events at index < N were too-old/expired". Batches are expected
//     to be chronologically ordered, so too-old/expired events form a prefix and
//     the "end" is one past the last matching index seen.
type rejectedTracker struct {
	tooOldEnd   *int32
	tooNewStart *int32
	expiredEnd  *int32
}

func (t *rejectedTracker) track(
	ts int64,
	idx int32,
	retentionCutoffMs, hardCutoff, futureLimit int64,
) bool {
	if ts > futureLimit {
		if t.tooNewStart == nil {
			t.tooNewStart = &idx
		}

		return false
	}

	// Reject events older than the 14-day hard cap.
	if ts < hardCutoff {
		end := idx + 1
		if t.tooOldEnd == nil || end > *t.tooOldEnd {
			t.tooOldEnd = &end
		}

		return false
	}

	// Events beyond the group's retention window are marked as expired
	// in the response but are still stored; the janitor evicts them later.
	if retentionCutoffMs > hardCutoff && ts < retentionCutoffMs {
		end := idx + 1
		if t.expiredEnd == nil || end > *t.expiredEnd {
			t.expiredEnd = &end
		}
	}

	return true
}

// classifyLogEvents splits events into accepted and rejected based on timestamp windows.
// It returns the accepted events and rejected-event index pointers for the response.
func classifyLogEvents(
	events []InputLogEvent,
	retentionCutoffMs, hardCutoff, futureLimit int64,
) ([]InputLogEvent, *RejectedLogEventsInfo) {
	acceptedEvents := make([]InputLogEvent, 0, len(events))
	var tracker rejectedTracker

	for i, e := range events {
		idx := int32(i)

		// Synthetic test timestamps (before Sep 2001) bypass window validation.
		if e.Timestamp < minRealisticTimestampMs {
			acceptedEvents = append(acceptedEvents, e)

			continue
		}

		if tracker.track(e.Timestamp, idx, retentionCutoffMs, hardCutoff, futureLimit) {
			acceptedEvents = append(acceptedEvents, e)
		}
	}

	var rejectedInfo *RejectedLogEventsInfo
	if tracker.tooNewStart != nil || tracker.tooOldEnd != nil || tracker.expiredEnd != nil {
		rejectedInfo = &RejectedLogEventsInfo{
			TooNewLogEventStartIndex: tracker.tooNewStart,
			TooOldLogEventEndIndex:   tracker.tooOldEnd,
			ExpiredLogEventEndIndex:  tracker.expiredEnd,
		}
	}

	return acceptedEvents, rejectedInfo
}

// PutLogEvents appends log events to a stream and returns a PutLogEventsResult.
// sequenceToken is accepted for wire compatibility but, matching current AWS
// behavior (see aws-sdk-go-v2 cloudwatchlogs.PutLogEvents doc: "The sequence
// token is now ignored in PutLogEvents actions. PutLogEvents actions are always
// accepted and never return InvalidSequenceTokenException or
// DataAlreadyAcceptedException even if the sequence token is not valid."), it is
// never validated: PutLogEvents accepts concurrent, unordered, or stale tokens.
// Events with timestamps outside the allowed window are tracked in RejectedLogEventsInfo.
func (b *InMemoryBackend) PutLogEvents(
	ctx context.Context,
	groupName, streamName, _ string,
	events []InputLogEvent,
) (*PutLogEventsResult, error) {
	if err := validatePutLogEventsBatch(events); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	var lockErr error
	var nextToken string
	var rejectedInfo *RejectedLogEventsInfo
	var eventsForDelivery []InputLogEvent
	var filtersForDelivery []*SubscriptionFilter
	var metricMatches []metricFilterMatch
	var emitter MetricEmitter

	func() {
		b.mu.Lock("PutLogEvents")
		defer b.mu.Unlock()

		group, groupExists := b.groupGet(region, groupName)
		if !groupExists {
			lockErr = fmt.Errorf("%w: Log group %s not found", ErrLogGroupNotFound, groupName)

			return
		}

		stream, streamExists := b.streamGet(region, groupName, streamName)
		if !streamExists {
			lockErr = fmt.Errorf("%w: Log stream %s not found", ErrLogStreamNotFound, streamName)

			return
		}

		now := time.Now().UnixMilli()

		// Determine the retention cutoff for timestamp validation.
		var retentionCutoffMs int64
		if group.RetentionInDays != nil && *group.RetentionInDays > 0 {
			retentionCutoffMs = now - int64(*group.RetentionInDays)*msPerDay
		} else {
			retentionCutoffMs = now - putLogEventsMaxEventAgeMs
		}

		hardCutoff := now - putLogEventsMaxEventAgeMs
		futureLimit := now + putLogEventsFutureWindowMs

		var acceptedEvents []InputLogEvent
		acceptedEvents, rejectedInfo = classifyLogEvents(
			events,
			retentionCutoffMs,
			hardCutoff,
			futureLimit,
		)

		if spanErr := validateEventSpan(acceptedEvents); spanErr != nil {
			lockErr = spanErr

			return
		}

		b.appendEvents(group, stream, now, acceptedEvents)

		stream.LastIngestionTime = &now
		nextToken = strconv.FormatInt(int64(len(stream.events)), 10)

		// Collect matching subscription filters and metric filter matches while holding the lock.
		filters := b.matchingFilters(region, groupName, acceptedEvents)
		eventsForDelivery = append([]InputLogEvent(nil), acceptedEvents...)
		filtersForDelivery = cloneSubscriptionFilters(filters)
		metricMatches = b.matchingMetricFilters(region, groupName, acceptedEvents)
		emitter = b.metricEmitter
	}()
	if lockErr != nil {
		return nil, lockErr
	}

	// Emit CloudWatch metrics for matched metric filters (no lock held).
	if len(metricMatches) > 0 && emitter != nil {
		b.emitMetricFilterMatches(emitter, metricMatches)
	}

	b.scheduleFilterDelivery(groupName, streamName, eventsForDelivery, filtersForDelivery)

	return &PutLogEventsResult{
		NextSequenceToken:     nextToken,
		RejectedLogEventsInfo: rejectedInfo,
	}, nil
}

// scheduleFilterDelivery asynchronously delivers accepted log events to matching
// subscription filters. No-ops when there are no filters or no deliverer configured.
func (b *InMemoryBackend) scheduleFilterDelivery(
	groupName, streamName string,
	events []InputLogEvent,
	filters []*SubscriptionFilter,
) {
	if len(filters) == 0 || b.deliverer == nil {
		return
	}
	b.wg.Go(func() {
		// Acquire a worker slot or abort if the backend is shutting down.
		select {
		case b.workerSem <- struct{}{}:
			defer func() { <-b.workerSem }()
		case <-b.ctx.Done():
			return
		}
		b.deliverToFilters(
			b.ctx,
			groupName,
			streamName,
			b.accountID,
			events,
			filters,
			b.deliverer,
			b.deliveryTimeout,
		)
	})
}

// appendEvents writes events into the stream, updates stream timestamp metadata,
// and enforces the per-stream event cap.
// Must be called while holding the backend write lock.
// Note: validateChronologicalOrder only rejects a single non-chronological
// *batch*; separate PutLogEvents calls to the same stream may still arrive
// with a later call carrying older timestamps than an earlier one (and
// synthetic sub-minRealisticTimestampMs test timestamps bypass ordering
// checks entirely), so min/max timestamp tracking must inspect all events
// rather than assume the stream's events slice is globally sorted.
func (b *InMemoryBackend) appendEvents(
	group *LogGroup, stream *LogStream, now int64, events []InputLogEvent,
) {
	groupName, streamName := stream.logGroupName, stream.LogStreamName
	for _, ev := range events {
		idx := len(stream.events)
		ptr := base64.StdEncoding.EncodeToString(
			[]byte(groupName + "/" + streamName + "/" + strconv.Itoa(idx)),
		)
		out := &OutputLogEvent{
			IngestionTime: now,
			Message:       ev.Message,
			Timestamp:     ev.Timestamp,
			Ptr:           ptr,
		}
		stream.events = append(stream.events, out)

		msgLen := int64(len(ev.Message))
		stream.StoredBytes += msgLen
		group.StoredBytes += msgLen

		if stream.FirstEventTimestamp == nil || ev.Timestamp < *stream.FirstEventTimestamp {
			ts := ev.Timestamp
			stream.FirstEventTimestamp = &ts
		}
		if stream.LastEventTimestamp == nil || ev.Timestamp > *stream.LastEventTimestamp {
			ts := ev.Timestamp
			stream.LastEventTimestamp = &ts
		}
	}

	// Enforce per-stream event cap: keep only the most recent maxEventsPerStream events.
	if cur := stream.events; len(cur) > maxEventsPerStream {
		stream.events = cur[len(cur)-maxEventsPerStream:]
		// Recalculate metadata from the remaining events: since events may have
		// out-of-order timestamps, the dropped events might include the global
		// min/max, so we must re-scan rather than assume positional ordering.
		updateStreamTimestamps(stream, stream.events)
	}
}

// logEventsTokenBackwardMarker distinguishes a backward-paging GetLogEvents
// token from a forward one. encodeNextToken (store.go), used by every other
// paginated op in this package, has no such marker -- a plain base64(decimal)
// token decodes here as forward, for backward compatibility with tokens
// issued before this marker existed.
const logEventsTokenBackwardMarker = 'B'

// encodeLogEventsToken encodes a GetLogEvents pagination cursor with its
// direction, so a nextBackwardToken fed back in can be told apart from a
// nextForwardToken -- see parseLogEventsToken.
func encodeLogEventsToken(idx int, backward bool) string {
	payload := strconv.Itoa(idx)
	if backward {
		payload = string(logEventsTokenBackwardMarker) + payload
	}

	return base64.StdEncoding.EncodeToString([]byte(payload))
}

// parseLogEventsToken decodes a GetLogEvents cursor back to its offset and
// direction. Unmarked (legacy/plain-decimal) tokens decode as forward.
func parseLogEventsToken(token string) (int, bool) {
	if token == "" {
		return 0, false
	}

	decoded := token
	if d, err := base64.StdEncoding.DecodeString(token); err == nil {
		decoded = string(d)
	}

	backward := strings.HasPrefix(decoded, string(logEventsTokenBackwardMarker))
	if backward {
		decoded = decoded[1:]
	}

	idx := 0
	if n, err := strconv.Atoi(decoded); err == nil && n >= 0 {
		idx = n
	}

	return idx, backward
}

// GetLogEvents returns events for a stream with optional time bounds, limit, and pagination.
// startFromHead controls the iteration direction:
//   - true  (start from oldest): begin at the oldest matching event.
//   - false (AWS default when no nextToken is provided): begin at the newest events.
//
// Once pagination begins, the AWS SDK passes back whichever of nextForwardToken/
// nextBackwardToken it wants to continue with; the two are NOT interchangeable
// offsets into the same forward read -- nextBackwardToken must return the window
// immediately preceding the one it was issued from, not repeat it. This backend's
// tokens (see encodeLogEventsToken/parseLogEventsToken) carry that direction so
// GetLogEvents can tell them apart; startFromHead is ignored once a token is given.
func (b *InMemoryBackend) GetLogEvents(
	ctx context.Context,
	groupName, streamName string,
	startTime, endTime *int64,
	limit int,
	nextToken string,
	startFromHead bool,
) ([]OutputLogEvent, string, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetLogEvents")
	defer b.mu.RUnlock()

	if !b.groupHas(region, groupName) {
		return nil, "", "", fmt.Errorf("%w: Log group %s not found", ErrLogGroupNotFound, groupName)
	}

	stream, exists := b.streamGet(region, groupName, streamName)
	if !exists {
		return nil, "", "", fmt.Errorf(
			"%w: Log stream %s not found",
			ErrLogStreamNotFound,
			streamName,
		)
	}

	filtered := filterByTime(stream.events, startTime, endTime)

	if limit <= 0 {
		limit = defaultEventLimit
	}

	var tokenIdx int
	var backward bool
	if nextToken != "" {
		// An explicit token always takes precedence over startFromHead.
		tokenIdx, backward = parseLogEventsToken(nextToken)
	} else if !startFromHead {
		// No token + startFromHead=false (the AWS default): begin at the last page.
		if len(filtered) > limit {
			tokenIdx = len(filtered) - limit
		}
	}
	// nextToken=="" && startFromHead=true: tokenIdx stays 0 (oldest first).

	// A stale or adversarial token can name an offset past the current event
	// count (e.g. retention swept older events out from under it); clamp
	// before slicing so it degrades to an empty page instead of panicking.
	tokenIdx = min(tokenIdx, len(filtered))

	var startIdx, end int
	if backward {
		// A backward token names the start of the window it was issued for;
		// paging backward from there returns the window immediately before it.
		end = tokenIdx
		startIdx = max(0, tokenIdx-limit)
	} else {
		startIdx = tokenIdx
		end = min(tokenIdx+limit, len(filtered))
	}

	page := filtered[startIdx:end]

	fwdToken := encodeLogEventsToken(end, false)
	bwdToken := encodeLogEventsToken(startIdx, true)

	result := make([]OutputLogEvent, len(page))
	for i, e := range page {
		result[i] = *e
	}

	return result, fwdToken, bwdToken, nil
}

// FilterLogEventsParams holds the inputs for InMemoryBackend.FilterLogEvents.
type FilterLogEventsParams struct {
	StartTime           *int64
	EndTime             *int64
	GroupName           string
	FilterPattern       string
	NextToken           string
	LogStreamNamePrefix string
	StreamNames         []string
	Limit               int
}

// taggedEvent pairs a stored event with the name of the stream it came from so
// FilterLogEvents can populate the logStreamName field on each FilteredLogEvent.
type taggedEvent struct {
	ev     *OutputLogEvent
	stream string
}

// FilterLogEvents searches events across streams in a group with an optional
// filter pattern. Results are interleaved across streams and sorted by event
// timestamp (ascending), matching AWS behaviour. The returned events carry the
// originating logStreamName and a deterministic eventId.
func (b *InMemoryBackend) FilterLogEvents(
	ctx context.Context,
	p FilterLogEventsParams,
) ([]FilteredLogEvent, string, []SearchedLogStream, error) {
	// AWS rejects requests that set both logStreamNames and logStreamNamePrefix.
	if len(p.StreamNames) > 0 && p.LogStreamNamePrefix != "" {
		return nil, "", nil, fmt.Errorf(
			"%w: logStreamNames and logStreamNamePrefix are mutually exclusive", ErrValidation)
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("FilterLogEvents")
	defer b.mu.RUnlock()

	if !b.groupHas(region, p.GroupName) {
		return nil, "", nil, fmt.Errorf(
			"%w: Log group %s not found",
			ErrLogGroupNotFound,
			p.GroupName,
		)
	}

	// Compile the filter pattern once before iterating over events so that
	// wildcard regexes are not recompiled for every event.
	var compiled *compiledFilterPattern
	if p.FilterPattern != "" {
		compiled = compileFilterPattern(p.FilterPattern)
	}

	streamOrder := b.filterStreamOrderLocked(region, p.GroupName, p.StreamNames)
	if p.LogStreamNamePrefix != "" {
		streamOrder = filterStreamsByPrefix(streamOrder, p.LogStreamNamePrefix)
	}

	var all []taggedEvent

	for _, sName := range streamOrder {
		stream, ok := b.streamGet(region, p.GroupName, sName)
		if !ok {
			continue
		}
		for _, ev := range stream.events {
			if compiled != nil && !compiled.matches(ev.Message) {
				continue
			}
			all = append(all, taggedEvent{ev: ev, stream: sName})
		}
	}

	all = filterTaggedByTime(all, p.StartTime, p.EndTime)
	// Interleave across streams: AWS returns matched events sorted by timestamp.
	// A stable sort preserves per-stream ingestion order for equal timestamps.
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].ev.Timestamp < all[j].ev.Timestamp
	})

	startIdx := parseNextToken(p.NextToken)
	limit := p.Limit
	if limit <= 0 {
		limit = defaultEventLimit
	}

	end := startIdx + limit
	var outToken string
	if end < len(all) {
		outToken = encodeNextToken(end)
	} else {
		end = len(all)
	}
	if startIdx > len(all) {
		startIdx = len(all)
	}

	page := all[startIdx:end]
	result := make([]FilteredLogEvent, len(page))
	for i, te := range page {
		result[i] = FilteredLogEvent{
			EventID:       filteredEventID(p.GroupName, te.stream, te.ev),
			LogStreamName: te.stream,
			Message:       te.ev.Message,
			IngestionTime: te.ev.IngestionTime,
			Timestamp:     te.ev.Timestamp,
		}
	}

	// AWS deprecated searchedLogStreams (it returns an empty list). We mirror
	// that contract rather than fabricating data clients should not rely on.
	return result, outToken, []SearchedLogStream{}, nil
}

// filterStreamsByPrefix returns only the stream names that start with prefix,
// preserving order.
func filterStreamsByPrefix(streams []string, prefix string) []string {
	out := make([]string, 0, len(streams))
	for _, s := range streams {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}

	return out
}

// filterTaggedByTime applies the start/end time window to tagged events.
func filterTaggedByTime(events []taggedEvent, startTime, endTime *int64) []taggedEvent {
	if startTime == nil && endTime == nil {
		return events
	}

	out := make([]taggedEvent, 0, len(events))
	for _, te := range events {
		if startTime != nil && te.ev.Timestamp < *startTime {
			continue
		}
		if endTime != nil && te.ev.Timestamp > *endTime {
			continue
		}
		out = append(out, te)
	}

	return out
}

// filteredEventID derives a deterministic, opaque event ID for a filtered event.
// AWS returns a 56-character numeric eventId; we reuse the event's stable byte
// pointer so the same event always yields the same ID without storing extra state.
func filteredEventID(groupName, streamName string, ev *OutputLogEvent) string {
	if ev.Ptr != "" {
		return ev.Ptr
	}

	return base64.StdEncoding.EncodeToString(
		fmt.Appendf(nil, "%s/%s/%d/%d", groupName, streamName, ev.Timestamp, ev.IngestionTime))
}

func filterByTime(events []*OutputLogEvent, startTime, endTime *int64) []*OutputLogEvent {
	if startTime == nil && endTime == nil {
		return events
	}

	out := make([]*OutputLogEvent, 0, len(events))
	for _, ev := range events {
		if startTime != nil && ev.Timestamp < *startTime {
			continue
		}
		if endTime != nil && ev.Timestamp > *endTime {
			continue
		}
		out = append(out, ev)
	}

	return out
}

// filterStreamOrderLocked returns the ordered list of stream names to iterate
// for FilterLogEvents. When streamNames is empty, all streams in the group are
// returned in sorted order. When streamNames is non-empty, only the requested
// names that exist in the group are returned, in sorted order, deduplicated.
// Caller must hold b.mu (read or write).
func (b *InMemoryBackend) filterStreamOrderLocked(
	region, groupName string,
	streamNames []string,
) []string {
	groupStreams := b.streamsInGroup(region, groupName)

	if len(streamNames) == 0 {
		names := make([]string, len(groupStreams))
		for i, s := range groupStreams {
			names[i] = s.LogStreamName
		}
		sort.Strings(names)

		return names
	}

	existing := make(map[string]bool, len(groupStreams))
	for _, s := range groupStreams {
		existing[s.LogStreamName] = true
	}

	seen := make(map[string]bool, len(streamNames))
	out := make([]string, 0, len(streamNames))

	for _, s := range streamNames {
		if seen[s] {
			continue
		}

		seen[s] = true

		if existing[s] {
			out = append(out, s)
		}
	}

	sort.Strings(out)

	return out
}

// GetLogRecord returns a single log event by its log record pointer.
// The pointer is the base64-encoded "<groupName>/<streamName>/<index>" string.
func (b *InMemoryBackend) GetLogRecord(
	ctx context.Context,
	logRecordPointer string,
) (map[string]string, error) {
	if logRecordPointer == "" {
		return nil, fmt.Errorf("%w: logRecordPointer is required", ErrValidation)
	}

	raw, err := base64.StdEncoding.DecodeString(logRecordPointer)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid logRecordPointer: %w", ErrValidation, err)
	}

	const pointerParts = 3
	parts := strings.SplitN(string(raw), "/", pointerParts)
	if len(parts) < pointerParts {
		return nil, fmt.Errorf("%w: invalid logRecordPointer format", ErrValidation)
	}

	groupName := parts[0]
	streamName := parts[1]
	idx, parseErr := strconv.Atoi(parts[2])
	if parseErr != nil || idx < 0 {
		return nil, fmt.Errorf("%w: invalid logRecordPointer index", ErrValidation)
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("GetLogRecord")
	defer b.mu.RUnlock()

	if !b.groupHas(region, groupName) {
		return nil, fmt.Errorf("%w: Log group %s not found", ErrLogGroupNotFound, groupName)
	}

	stream, exists := b.streamGet(region, groupName, streamName)
	if !exists {
		return nil, fmt.Errorf("%w: Log stream %s not found", ErrLogStreamNotFound, streamName)
	}

	if idx >= len(stream.events) {
		return nil, fmt.Errorf("%w: log record index %d out of range", ErrValidation, idx)
	}

	ev := stream.events[idx]
	result := map[string]string{
		keyMessageField:  ev.Message,
		keyTimestamp:     strconv.FormatInt(ev.Timestamp, 10),
		keyIngestionTime: strconv.FormatInt(ev.IngestionTime, 10),
		keyLogStream:     streamName,
		"@logGroup":      groupName,
	}

	return result, nil
}

// DiscoverLogFields returns the set of field names discovered from the log
// events stored for the given log group. It always includes the system fields
// (@timestamp, @message, @ingestionTime, @logStream) and additionally parses
// any JSON-formatted event messages to surface their top-level keys. The
// returned slice is sorted for deterministic output. The log group must exist.
func (b *InMemoryBackend) DiscoverLogFields(
	ctx context.Context,
	logGroupName string,
) ([]string, error) {
	if logGroupName == "" {
		return nil, fmt.Errorf("%w: logGroupName is required", ErrValidation)
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("DiscoverLogFields")
	defer b.mu.RUnlock()

	if !b.groupHas(region, logGroupName) {
		return nil, fmt.Errorf("%w: Log group %s not found", ErrLogGroupNotFound, logGroupName)
	}

	fieldSet := map[string]struct{}{
		keyTimestamp:     {},
		keyMessageField:  {},
		keyIngestionTime: {},
		keyLogStream:     {},
	}

	for _, stream := range b.streamsInGroup(region, logGroupName) {
		for _, ev := range stream.events {
			for _, name := range jsonMessageFields(ev.Message) {
				fieldSet[name] = struct{}{}
			}
		}
	}

	fields := make([]string, 0, len(fieldSet))
	for name := range fieldSet {
		fields = append(fields, name)
	}
	sort.Strings(fields)

	return fields, nil
}

// jsonMessageFields returns the sorted top-level keys of a log event message if
// the message is a JSON object. Non-JSON messages yield no extra fields.
func jsonMessageFields(message string) []string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" || trimmed[0] != '{' {
		return nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return nil
	}

	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}

	return keys
}
