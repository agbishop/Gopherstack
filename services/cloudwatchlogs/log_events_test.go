package cloudwatchlogs_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatchlogs"
)

func TestCloudWatchLogsBackend_PutLogEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		setup   func(t *testing.T, b *cloudwatchlogs.InMemoryBackend)
		name    string
		group   string
		stream  string
		events  []cloudwatchlogs.InputLogEvent
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, _ = b.CreateLogGroup(context.Background(), "grp", "", "")
				_, _ = b.CreateLogStream(context.Background(), "grp", "stream")
			},
			group:  "grp",
			stream: "stream",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "first", Timestamp: 1000},
				{Message: "second", Timestamp: 2000},
			},
		},
		{
			name:    "group_not_found",
			group:   "nonexistent",
			stream:  "stream",
			wantErr: cloudwatchlogs.ErrLogGroupNotFound,
		},
		{
			name: "stream_not_found",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, _ = b.CreateLogGroup(context.Background(), "grp", "", "")
			},
			group:   "grp",
			stream:  "nonexistent",
			wantErr: cloudwatchlogs.ErrLogStreamNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			if tt.setup != nil {
				tt.setup(t, b)
			}

			token, err := b.PutLogEvents(context.Background(), tt.group, tt.stream, "", tt.events)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, token)
		})
	}
}

func TestCloudWatchLogsBackend_GetLogEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr           error
		setup             func(t *testing.T, b *cloudwatchlogs.InMemoryBackend)
		startTime         *int64
		endTime           *int64
		name              string
		group             string
		stream            string
		nextToken         string
		wantFirstMessage  string
		limit             int
		wantCount         int
		wantNonEmptyFwBwd bool
	}{
		{
			name: "all_events",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, _ = b.CreateLogGroup(context.Background(), "grp", "", "")
				_, _ = b.CreateLogStream(context.Background(), "grp", "stream")
				_, _ = b.PutLogEvents(
					context.Background(),
					"grp",
					"stream",
					"",
					[]cloudwatchlogs.InputLogEvent{
						{Message: "msg1", Timestamp: 1000},
						{Message: "msg2", Timestamp: 2000},
						{Message: "msg3", Timestamp: 3000},
					},
				)
			},
			group:             "grp",
			stream:            "stream",
			wantCount:         3,
			wantNonEmptyFwBwd: true,
		},
		{
			name: "time_filter",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, _ = b.CreateLogGroup(context.Background(), "grp", "", "")
				_, _ = b.CreateLogStream(context.Background(), "grp", "stream")
				_, _ = b.PutLogEvents(
					context.Background(),
					"grp",
					"stream",
					"",
					[]cloudwatchlogs.InputLogEvent{
						{Message: "old", Timestamp: 100},
						{Message: "new", Timestamp: 5000},
					},
				)
			},
			group:            "grp",
			stream:           "stream",
			startTime:        int64Ptr(1000),
			wantCount:        1,
			wantFirstMessage: "new",
		},
		{
			name:    "group_not_found",
			group:   "nonexistent",
			stream:  "stream",
			wantErr: cloudwatchlogs.ErrLogGroupNotFound,
		},
		{
			name: "stream_not_found",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) {
				t.Helper()
				_, _ = b.CreateLogGroup(context.Background(), "grp", "", "")
			},
			group:   "grp",
			stream:  "nonexistent",
			wantErr: cloudwatchlogs.ErrLogStreamNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			if tt.setup != nil {
				tt.setup(t, b)
			}

			evts, fwd, bwd, err := b.GetLogEvents(
				context.Background(),
				tt.group,
				tt.stream,
				tt.startTime,
				tt.endTime,
				tt.limit,
				tt.nextToken,
				true,
			)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Len(t, evts, tt.wantCount)

			if tt.wantNonEmptyFwBwd {
				assert.NotEmpty(t, fwd)
				assert.NotEmpty(t, bwd)
			}

			if tt.wantFirstMessage != "" && tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirstMessage, evts[0].Message)
			}
		})
	}
}

func TestCloudWatchLogsBackend_GetLogEvents_Pagination(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, _ = b.CreateLogGroup(context.Background(), "grp", "", "")
	_, _ = b.CreateLogStream(context.Background(), "grp", "stream")
	_, _ = b.PutLogEvents(context.Background(), "grp", "stream", "", []cloudwatchlogs.InputLogEvent{
		{Message: "a", Timestamp: 1},
		{Message: "b", Timestamp: 2},
		{Message: "c", Timestamp: 3},
	})

	evts, fwd, _, err := b.GetLogEvents(
		context.Background(),
		"grp",
		"stream",
		nil,
		nil,
		2,
		"",
		true,
	)
	require.NoError(t, err)
	assert.Len(t, evts, 2)

	evts2, _, _, err := b.GetLogEvents(
		context.Background(),
		"grp",
		"stream",
		nil,
		nil,
		2,
		fwd,
		true,
	)
	require.NoError(t, err)
	assert.Len(t, evts2, 1)
}

func TestCloudWatchLogsBackend_PutLogEvents_UpdatesTimestamps(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, _ = b.CreateLogGroup(context.Background(), "grp", "", "")
	_, _ = b.CreateLogStream(context.Background(), "grp", "s")

	_, _ = b.PutLogEvents(context.Background(), "grp", "s", "", []cloudwatchlogs.InputLogEvent{
		{Message: "a", Timestamp: 500},
		{Message: "b", Timestamp: 1500},
	})

	streams, _, err := b.DescribeLogStreams(context.Background(), "grp", "", "", "", false, 0)
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.NotNil(t, streams[0].FirstEventTimestamp)
	require.NotNil(t, streams[0].LastEventTimestamp)
	assert.Equal(t, int64(500), *streams[0].FirstEventTimestamp)
	assert.Equal(t, int64(1500), *streams[0].LastEventTimestamp)
}

func int64Ptr(v int64) *int64 { return new(v) }

func TestCloudWatchLogsBackend_PutLogEvents_EventCap(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackend()
	_, err := b.CreateLogGroup(context.Background(), "g", "", "")
	require.NoError(t, err)
	_, err = b.CreateLogStream(context.Background(), "g", "s")
	require.NoError(t, err)

	// Write MaxEventsPerStream + 500 events (guaranteed overflow) in batches.
	const batchSize = 1000
	const total = cloudwatchlogs.MaxEventsPerStream + 500
	now := time.Now().UnixMilli()
	written := 0
	for written < total {
		size := batchSize
		if written+size > total {
			size = total - written
		}
		events := make([]cloudwatchlogs.InputLogEvent, size)
		for j := range size {
			events[j] = cloudwatchlogs.InputLogEvent{
				Message:   fmt.Sprintf("msg-%d", written+j),
				Timestamp: now + int64(written+j),
			}
		}
		_, putErr := b.PutLogEvents(context.Background(), "g", "s", "", events)
		require.NoError(t, putErr)
		written += size
	}

	// Exactly MaxEventsPerStream events should remain (the newest ones).
	got, _, _, err := b.GetLogEvents(
		context.Background(),
		"g",
		"s",
		nil,
		nil,
		cloudwatchlogs.MaxEventsPerStream+1000,
		"",
		true,
	)
	require.NoError(t, err)
	assert.Len(t, got, cloudwatchlogs.MaxEventsPerStream)

	// The oldest events (msg-0 through msg-499) should have been dropped.
	// The newest events should be present: msg-500 through msg-10499.
	assert.Equal(t, fmt.Sprintf("msg-%d", 500), got[0].Message)
	assert.Equal(t, fmt.Sprintf("msg-%d", total-1), got[len(got)-1].Message)

	// FirstEventTimestamp should reflect the oldest retained event.
	streams, _, sErr := b.DescribeLogStreams(context.Background(), "g", "", "", "", false, 10)
	require.NoError(t, sErr)
	require.Len(t, streams, 1)
	require.NotNil(t, streams[0].FirstEventTimestamp)
	assert.Equal(t, now+500, *streams[0].FirstEventTimestamp)
}

func TestCloudWatchLogsBackend_GetLogEvents_StartFromHead(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackend()
	_, err := b.CreateLogGroup(context.Background(), "g", "", "")
	require.NoError(t, err)
	_, err = b.CreateLogStream(context.Background(), "g", "s")
	require.NoError(t, err)

	// Put 5 events with recent timestamps (ascending).
	base := time.Now().UnixMilli()
	events := []cloudwatchlogs.InputLogEvent{
		{Message: "e1", Timestamp: base + 1},
		{Message: "e2", Timestamp: base + 2},
		{Message: "e3", Timestamp: base + 3},
		{Message: "e4", Timestamp: base + 4},
		{Message: "e5", Timestamp: base + 5},
	}
	_, err = b.PutLogEvents(context.Background(), "g", "s", "", events)
	require.NoError(t, err)

	// startFromHead=true, limit=2: should return oldest 2 events.
	got, _, _, err := b.GetLogEvents(context.Background(), "g", "s", nil, nil, 2, "", true)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "e1", got[0].Message)
	assert.Equal(t, "e2", got[1].Message)

	// startFromHead=false (AWS default), limit=2: should return newest 2 events.
	got, _, _, err = b.GetLogEvents(context.Background(), "g", "s", nil, nil, 2, "", false)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "e4", got[0].Message)
	assert.Equal(t, "e5", got[1].Message)

	// nextToken takes precedence over startFromHead.
	got, _, _, err = b.GetLogEvents(context.Background(), "g", "s", nil, nil, 2, "0", false)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "e1", got[0].Message)
}

func TestCloudWatchLogsBackend_StoredBytesTracking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		messages        []string
		wantStreamBytes int64
		wantGroupBytes  int64
	}{
		{
			name:            "tracks_bytes_on_put",
			messages:        []string{"hello", "world"},
			wantStreamBytes: 10, // len("hello") + len("world")
			wantGroupBytes:  10,
		},
		{
			name:            "single_message",
			messages:        []string{"test"},
			wantStreamBytes: 4,
			wantGroupBytes:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackend()
			_, err := b.CreateLogGroup(context.Background(), "g", "", "")
			require.NoError(t, err)
			_, err = b.CreateLogStream(context.Background(), "g", "s")
			require.NoError(t, err)

			events := make([]cloudwatchlogs.InputLogEvent, len(tt.messages))
			for i, m := range tt.messages {
				events[i] = cloudwatchlogs.InputLogEvent{Message: m, Timestamp: int64(i + 1)}
			}
			_, err = b.PutLogEvents(context.Background(), "g", "s", "", events)
			require.NoError(t, err)

			streams, _, err := b.DescribeLogStreams(
				context.Background(),
				"g",
				"",
				"",
				"",
				false,
				10,
			)
			require.NoError(t, err)
			require.Len(t, streams, 1)
			assert.Equal(t, tt.wantStreamBytes, streams[0].StoredBytes)

			groups, _, err := b.DescribeLogGroups(context.Background(), "", "", 10)
			require.NoError(t, err)
			require.Len(t, groups, 1)
			assert.Equal(t, tt.wantGroupBytes, groups[0].StoredBytes)
		})
	}

	t.Run("delete_stream_subtracts_bytes", func(t *testing.T) {
		t.Parallel()

		b := cloudwatchlogs.NewInMemoryBackend()
		_, err := b.CreateLogGroup(context.Background(), "g", "", "")
		require.NoError(t, err)
		_, err = b.CreateLogStream(context.Background(), "g", "s")
		require.NoError(t, err)

		_, err = b.PutLogEvents(context.Background(), "g", "s", "", []cloudwatchlogs.InputLogEvent{
			{Message: "hello", Timestamp: 1},
		})
		require.NoError(t, err)

		groups, _, err := b.DescribeLogGroups(context.Background(), "", "", 10)
		require.NoError(t, err)
		require.Len(t, groups, 1)
		assert.Equal(t, int64(5), groups[0].StoredBytes)

		err = b.DeleteLogStream(context.Background(), "g", "s")
		require.NoError(t, err)

		groups, _, err = b.DescribeLogGroups(context.Background(), "", "", 10)
		require.NoError(t, err)
		require.Len(t, groups, 1)
		assert.Equal(t, int64(0), groups[0].StoredBytes)
	})
}

func TestCloudWatchLogsBackend_GetLogRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		setup   func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) string
		name    string
		pointer string
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *cloudwatchlogs.InMemoryBackend) string {
				t.Helper()
				_, err := b.CreateLogGroup(context.Background(), "g", "", "")
				require.NoError(t, err)
				_, err = b.CreateLogStream(context.Background(), "g", "s")
				require.NoError(t, err)
				_, err = b.PutLogEvents(
					context.Background(),
					"g",
					"s",
					"",
					[]cloudwatchlogs.InputLogEvent{
						{Message: "hello world", Timestamp: 1000},
					},
				)
				require.NoError(t, err)
				// Get the ptr from GetLogEvents
				evts, _, _, err := b.GetLogEvents(
					context.Background(),
					"g",
					"s",
					nil,
					nil,
					10,
					"",
					true,
				)
				require.NoError(t, err)
				require.Len(t, evts, 1)

				return evts[0].Ptr
			},
		},
		{
			name:    "invalid_pointer",
			pointer: "not-base64!@#",
			wantErr: cloudwatchlogs.ErrValidation,
		},
		{
			name:    "empty_pointer",
			wantErr: cloudwatchlogs.ErrValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackend()
			ptr := tt.pointer
			if tt.setup != nil {
				ptr = tt.setup(t, b)
			}

			record, err := b.GetLogRecord(context.Background(), ptr)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Contains(t, record, "@message")
			assert.Contains(t, record, "@timestamp")
			assert.Equal(t, "hello world", record["@message"])
		})
	}
}

func TestCloudWatchLogsBackend_LogRecordPtrInOutputEvent(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackend()
	_, err := b.CreateLogGroup(context.Background(), "g", "", "")
	require.NoError(t, err)
	_, err = b.CreateLogStream(context.Background(), "g", "s")
	require.NoError(t, err)
	_, err = b.PutLogEvents(context.Background(), "g", "s", "", []cloudwatchlogs.InputLogEvent{
		{Message: "msg1", Timestamp: 100},
		{Message: "msg2", Timestamp: 200},
	})
	require.NoError(t, err)

	evts, _, _, err := b.GetLogEvents(context.Background(), "g", "s", nil, nil, 10, "", true)
	require.NoError(t, err)
	require.Len(t, evts, 2)

	for i, ev := range evts {
		assert.NotEmpty(t, ev.Ptr, "event %d should have a Ptr", i)
		// Each pointer should be decodable and map back to an event.
		record, rerr := b.GetLogRecord(context.Background(), ev.Ptr)
		require.NoError(t, rerr, "event %d pointer should be decodable", i)
		assert.Equal(t, ev.Message, record["@message"])
	}
}

func TestCloudWatchLogsBackend_PutLogEvents_RejectedLogEventsInfo(t *testing.T) {
	t.Parallel()

	now := time.Now().UnixMilli()
	tooOld := now - 15*24*60*60*1000 // 15 days ago (beyond 14d hard cap)
	tooNew := now + 3*60*60*1000     // 3 hours in the future

	tests := []struct {
		wantErr      error
		name         string
		events       []cloudwatchlogs.InputLogEvent
		wantAccepted int
		wantTooOld   bool
		wantTooNew   bool
		wantExpired  bool
	}{
		{
			name: "all_valid",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "ok", Timestamp: now},
			},
			wantAccepted: 1,
		},
		{
			name: "too_new_rejected",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "ok", Timestamp: now},
				{Message: "future", Timestamp: tooNew},
			},
			wantAccepted: 1,
			wantTooNew:   true,
		},
		{
			name: "too_old_rejected",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "old", Timestamp: tooOld},
				{Message: "ok", Timestamp: now},
			},
			wantAccepted: 1,
			wantTooOld:   true,
		},
		{
			name: "message_too_large",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: string(make([]byte, 256*1024+1)), Timestamp: now},
			},
			wantErr: cloudwatchlogs.ErrValidation,
		},
		{
			name: "synthetic_timestamps_accepted",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "test", Timestamp: 1},
				{Message: "test2", Timestamp: 1000},
			},
			wantAccepted: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackend()
			_, err := b.CreateLogGroup(context.Background(), "g", "", "")
			require.NoError(t, err)
			_, err = b.CreateLogStream(context.Background(), "g", "s")
			require.NoError(t, err)

			result, err := b.PutLogEvents(context.Background(), "g", "s", "", tt.events)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			got, _, _, err := b.GetLogEvents(
				context.Background(),
				"g",
				"s",
				nil,
				nil,
				1000,
				"",
				true,
			)
			require.NoError(t, err)
			assert.Len(t, got, tt.wantAccepted)

			if tt.wantTooOld || tt.wantTooNew || tt.wantExpired {
				require.NotNil(t, result.RejectedLogEventsInfo)
				if tt.wantTooOld {
					assert.NotNil(t, result.RejectedLogEventsInfo.TooOldLogEventEndIndex)
				}
				if tt.wantTooNew {
					assert.NotNil(t, result.RejectedLogEventsInfo.TooNewLogEventStartIndex)
				}
				if tt.wantExpired {
					assert.NotNil(t, result.RejectedLogEventsInfo.ExpiredLogEventEndIndex)
				}
			} else {
				assert.Nil(t, result.RejectedLogEventsInfo)
			}
		})
	}
}

// ---- PutLogEvents chronological-order and 24-hour-span batch constraints ----
//
// aws-sdk-go-v2 cloudwatchlogs.PutLogEvents doc comment: "A batch of log
// events in a single request must be in a chronological order. Otherwise,
// the operation fails." and "For valid events (within 14 days in the past to
// 2 hours in future), the time span in a single batch cannot exceed 24
// hours. Otherwise, the operation fails." Both are whole-request failures
// (InvalidParameterException), unlike the too-old/too-new/expired
// per-event classification captured in RejectedLogEventsInfo.

func TestCloudWatchLogsBackend_PutLogEvents_ChronologicalOrderAndSpan(t *testing.T) {
	t.Parallel()

	now := time.Now().UnixMilli()

	tests := []struct {
		wantErr error
		name    string
		events  []cloudwatchlogs.InputLogEvent
	}{
		{
			name: "out_of_order_realistic_timestamps_rejected",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "second", Timestamp: now},
				{Message: "first", Timestamp: now - 60_000},
			},
			wantErr: cloudwatchlogs.ErrValidation,
		},
		{
			name: "span_over_24h_rejected",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "old", Timestamp: now - 25*60*60*1000},
				{Message: "recent", Timestamp: now},
			},
			wantErr: cloudwatchlogs.ErrValidation,
		},
		{
			name: "span_exactly_24h_accepted",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "old", Timestamp: now - 24*60*60*1000},
				{Message: "recent", Timestamp: now},
			},
		},
		{
			name: "chronological_realistic_timestamps_accepted",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "first", Timestamp: now - 60_000},
				{Message: "second", Timestamp: now},
			},
		},
		{
			name: "equal_timestamps_are_not_out_of_order",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "a", Timestamp: now},
				{Message: "b", Timestamp: now},
			},
		},
		{
			name: "synthetic_out_of_order_timestamps_bypass_check",
			events: []cloudwatchlogs.InputLogEvent{
				{Message: "second", Timestamp: 2000},
				{Message: "first", Timestamp: 1000},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackend()
			_, err := b.CreateLogGroup(context.Background(), "g", "", "")
			require.NoError(t, err)
			_, err = b.CreateLogStream(context.Background(), "g", "s")
			require.NoError(t, err)

			result, err := b.PutLogEvents(context.Background(), "g", "s", "", tt.events)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				got, _, _, gErr := b.GetLogEvents(context.Background(), "g", "s", nil, nil, 1000, "", true)
				require.NoError(t, gErr)
				assert.Empty(t, got, "a whole-batch failure must not store any events")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
		})
	}
}

// ---- Item 3: SequenceToken is ignored (matches current AWS behavior) ----
//
// aws-sdk-go-v2 cloudwatchlogs.PutLogEvents doc: "The sequence token is now
// ignored in PutLogEvents actions. PutLogEvents actions are always accepted and
// never return InvalidSequenceTokenException or DataAlreadyAcceptedException
// even if the sequence token is not valid." So every case below must succeed
// regardless of whether the supplied token matches the stream's actual length,
// and NextSequenceToken must reflect the real post-append event count rather
// than echoing (or validating against) the caller-supplied token.

func TestCloudWatchLogsBackend_PutLogEvents_SequenceTokenIgnored(t *testing.T) {
	t.Parallel()

	now := time.Now().UnixMilli()

	tests := []struct {
		name          string
		sequenceToken string
		setupEvents   int
	}{
		{
			name:          "no_token",
			setupEvents:   0,
			sequenceToken: "",
		},
		{
			name:          "matching_token",
			setupEvents:   2,
			sequenceToken: "2",
		},
		{
			name:          "stale_token_still_accepted",
			setupEvents:   2,
			sequenceToken: "99",
		},
		{
			name:          "token_on_empty_stream_matching",
			setupEvents:   0,
			sequenceToken: "0",
		},
		{
			name:          "token_on_empty_stream_wrong_still_accepted",
			setupEvents:   0,
			sequenceToken: "5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatchlogs.NewInMemoryBackend()
			_, err := b.CreateLogGroup(context.Background(), "g", "", "")
			require.NoError(t, err)
			_, err = b.CreateLogStream(context.Background(), "g", "s")
			require.NoError(t, err)

			for i := range tt.setupEvents {
				_, err = b.PutLogEvents(
					context.Background(),
					"g",
					"s",
					"",
					[]cloudwatchlogs.InputLogEvent{
						{Message: fmt.Sprintf("event-%d", i), Timestamp: now},
					},
				)
				require.NoError(t, err)
			}

			result, err := b.PutLogEvents(
				context.Background(),
				"g",
				"s",
				tt.sequenceToken,
				[]cloudwatchlogs.InputLogEvent{
					{Message: "new event", Timestamp: now},
				},
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, strconv.Itoa(tt.setupEvents+1), result.NextSequenceToken)
		})
	}
}

// TestCloudWatchLogsBackend_GetLogEvents_StaleTokenPastEnd verifies GetLogEvents
// does not panic when nextToken names an offset beyond the current event count,
// e.g. because retention swept older events out from under a token minted
// before the sweep, or because of a corrupted/adversarial nextToken.
func TestCloudWatchLogsBackend_GetLogEvents_StaleTokenPastEnd(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, _ = b.CreateLogGroup(context.Background(), "grp", "", "")
	_, _ = b.CreateLogStream(context.Background(), "grp", "stream")
	_, _ = b.PutLogEvents(context.Background(), "grp", "stream", "", []cloudwatchlogs.InputLogEvent{
		{Message: "a", Timestamp: 1},
		{Message: "b", Timestamp: 2},
		{Message: "c", Timestamp: 3},
	})

	staleToken := base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(100)))

	require.NotPanics(t, func() {
		evts, _, _, err := b.GetLogEvents(
			context.Background(),
			"grp",
			"stream",
			nil,
			nil,
			2,
			staleToken,
			true,
		)
		require.NoError(t, err)
		assert.Empty(t, evts)
	})
}

// TestCloudWatchLogsBackend_FilterLogEvents_StaleTokenPastEnd is the
// FilterLogEvents analogue of the GetLogEvents test above: FilterLogEvents
// computes startIdx/end in a different order (end is computed before
// startIdx is clamped), but the clamp still lands before the slice
// operation, so an out-of-range token degrades to an empty page rather than
// panicking. This test pins that down instead of leaving it as inspection.
func TestCloudWatchLogsBackend_FilterLogEvents_StaleTokenPastEnd(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, _ = b.CreateLogGroup(context.Background(), "grp", "", "")
	_, _ = b.CreateLogStream(context.Background(), "grp", "stream")
	_, _ = b.PutLogEvents(context.Background(), "grp", "stream", "", []cloudwatchlogs.InputLogEvent{
		{Message: "a", Timestamp: 1},
		{Message: "b", Timestamp: 2},
		{Message: "c", Timestamp: 3},
	})

	staleToken := base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(100)))

	require.NotPanics(t, func() {
		evts, _, _, err := b.FilterLogEvents(context.Background(), cloudwatchlogs.FilterLogEventsParams{
			GroupName: "grp",
			NextToken: staleToken,
		})
		require.NoError(t, err)
		assert.Empty(t, evts)
	})
}

// TestCloudWatchLogsBackend_GetLogEvents_BackwardTokenReturnsPrecedingWindow
// proves nextBackwardToken actually pages backward through history, rather
// than re-reading forward from the same offset. Real AWS: "nextBackwardToken
// -- The token for the next set of items in the backward direction" returns
// the window immediately BEFORE the one it was issued from. Before this fix,
// GetLogEvents decoded every token (forward or backward) as a plain forward
// read offset, so feeding a nextBackwardToken back in replayed the exact same
// page forever instead of revealing older events -- a client scrolling
// backward through a log stream's history via GetLogEvents could never see
// anything before the first page it received.
func TestCloudWatchLogsBackend_GetLogEvents_BackwardTokenReturnsPrecedingWindow(t *testing.T) {
	t.Parallel()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	_, err := b.CreateLogGroup(context.Background(), "grp", "", "")
	require.NoError(t, err)
	_, err = b.CreateLogStream(context.Background(), "grp", "stream")
	require.NoError(t, err)

	events := make([]cloudwatchlogs.InputLogEvent, 6)
	for i := range events {
		events[i] = cloudwatchlogs.InputLogEvent{Message: fmt.Sprintf("m%d", i), Timestamp: int64(1000 + i)}
	}
	_, err = b.PutLogEvents(context.Background(), "grp", "stream", "", events)
	require.NoError(t, err)

	// Initial call with startFromHead=false (the AWS default): the last 2 events, m4/m5.
	page1, _, bwd1, err := b.GetLogEvents(context.Background(), "grp", "stream", nil, nil, 2, "", false)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, "m4", page1[0].Message)
	assert.Equal(t, "m5", page1[1].Message)

	// Paging backward from there must return the PRECEDING window, m2/m3 --
	// not a repeat of m4/m5.
	page2, _, bwd2, err := b.GetLogEvents(context.Background(), "grp", "stream", nil, nil, 2, bwd1, false)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, "m2", page2[0].Message)
	assert.Equal(t, "m3", page2[1].Message)

	// One more page back reaches the oldest events, m0/m1.
	page3, _, bwd3, err := b.GetLogEvents(context.Background(), "grp", "stream", nil, nil, 2, bwd2, false)
	require.NoError(t, err)
	require.Len(t, page3, 2)
	assert.Equal(t, "m0", page3[0].Message)
	assert.Equal(t, "m1", page3[1].Message)

	// At the start of the stream, backward paging is a fixed point: same
	// token, empty page.
	page4, _, bwd4, err := b.GetLogEvents(context.Background(), "grp", "stream", nil, nil, 2, bwd3, false)
	require.NoError(t, err)
	assert.Empty(t, page4)
	assert.Equal(t, bwd3, bwd4)
}
