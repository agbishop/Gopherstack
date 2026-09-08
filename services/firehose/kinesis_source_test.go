package firehose_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/services/firehose"
)

var errAccessDenied = errors.New("access denied")

// mockKinesisReader is a simple in-memory Kinesis reader for testing the poller.
type mockKinesisReader struct {
	listErr  error
	getErr   error
	shards   []string
	records  [][]byte
	position int
	mu       sync.Mutex
}

func (m *mockKinesisReader) ListShards(_ string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.listErr != nil {
		return nil, m.listErr
	}

	if len(m.shards) == 0 {
		return []string{"shardId-000000000000"}, nil
	}

	return append([]string{}, m.shards...), nil
}

func (m *mockKinesisReader) GetShardIterator(_, _ string) (string, error) {
	return "iter:0", nil
}

func (m *mockKinesisReader) GetRecords(_ string, limit int) ([][]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getErr != nil {
		return nil, "", m.getErr
	}

	start := m.position
	if start >= len(m.records) {
		// No records yet; return empty with continuation iterator.
		return nil, "iter:end", nil
	}

	end := min(start+limit, len(m.records))

	batch := append([][]byte{}, m.records[start:end]...)
	m.position = end

	// Return empty next iterator when all records consumed (simulates shard close).
	nextIter := "iter:end"
	if m.position < len(m.records) {
		nextIter = "iter:more"
	}

	return batch, nextIter, nil
}

func (m *mockKinesisReader) addRecords(recs ...[]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.records = append(m.records, recs...)
}

// Ensure compile-time interface check.
var _ firehose.KinesisReader = (*mockKinesisReader)(nil)

// newFirehoseBackend creates a backend for testing.
func newFirehoseBackend(t *testing.T) *firehose.InMemoryBackend {
	t.Helper()

	return firehose.NewInMemoryBackend("123456789012", "us-east-1")
}

// totalRecords reads TotalRecords from the stream Metrics.
// Records slice is always nil in DescribeDeliveryStream (cleared by streamCopy),
// but Metrics is a value-copy and reflects the live counters.
func totalRecords(t *testing.T, b *firehose.InMemoryBackend, streamName string) int64 {
	t.Helper()

	stream, err := b.DescribeDeliveryStream(context.TODO(), streamName)
	if err != nil {
		return 0
	}

	return stream.Metrics.TotalRecords
}

func TestFirehose_KinesisSource_PollerDeliversSingleRecord(t *testing.T) {
	t.Parallel()

	b := newFirehoseBackend(t)
	kinesis := &mockKinesisReader{}
	kinesis.addRecords([]byte("record-1"))

	b.SetKinesisBackend(kinesis)

	streamARN := "arn:aws:kinesis:us-east-1:123456789012:stream/my-stream"
	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name:               "poll-stream",
		DeliveryStreamType: "KinesisStreamAsSource",
		Source: &firehose.SourceDescription{
			KinesisStreamSourceDescription: &firehose.KinesisStreamSourceDescription{
				KinesisStreamARN: streamARN,
			},
		},
	})
	require.NoError(t, err)

	// Wait for the poller to deliver the record.
	require.Eventually(t, func() bool {
		return totalRecords(t, b, "poll-stream") >= 1
	}, 3*time.Second, 50*time.Millisecond, "poller should deliver records from Kinesis to Firehose")

	assert.Equal(t, int64(1), totalRecords(t, b, "poll-stream"))
}

func TestFirehose_KinesisSource_PollerDeliversManyRecords(t *testing.T) {
	t.Parallel()

	b := newFirehoseBackend(t)
	kinesis := &mockKinesisReader{}
	kinesis.addRecords([]byte("a"), []byte("b"), []byte("c"))

	b.SetKinesisBackend(kinesis)

	streamARN := "arn:aws:kinesis:us-east-1:123456789012:stream/multi-stream"
	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name:               "multi-poll-stream",
		DeliveryStreamType: "KinesisStreamAsSource",
		Source: &firehose.SourceDescription{
			KinesisStreamSourceDescription: &firehose.KinesisStreamSourceDescription{
				KinesisStreamARN: streamARN,
			},
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return totalRecords(t, b, "multi-poll-stream") >= 3
	}, 3*time.Second, 50*time.Millisecond, "poller should deliver all 3 records")

	assert.Equal(t, int64(3), totalRecords(t, b, "multi-poll-stream"))
}

func TestFirehose_KinesisSource_DeleteStopsPoller(t *testing.T) {
	t.Parallel()

	b := newFirehoseBackend(t)
	kinesis := &mockKinesisReader{} // no records, infinite polling

	b.SetKinesisBackend(kinesis)

	streamARN := "arn:aws:kinesis:us-east-1:123456789012:stream/stop-stream"
	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name:               "stop-poll-stream",
		DeliveryStreamType: "KinesisStreamAsSource",
		Source: &firehose.SourceDescription{
			KinesisStreamSourceDescription: &firehose.KinesisStreamSourceDescription{
				KinesisStreamARN: streamARN,
			},
		},
	})
	require.NoError(t, err)

	// Wait a bit then delete.
	time.Sleep(50 * time.Millisecond)

	err = b.DeleteDeliveryStream(context.TODO(), "stop-poll-stream")
	require.NoError(t, err)

	// Verify stream is gone and no panic.
	_, err = b.DescribeDeliveryStream(context.TODO(), "stop-poll-stream")
	assert.Error(t, err)
}

func TestFirehose_KinesisSource_NoBackendDoesNotStart(t *testing.T) {
	t.Parallel()

	b := newFirehoseBackend(t)
	// no kinesis backend wired

	streamARN := "arn:aws:kinesis:us-east-1:123456789012:stream/no-backend"
	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name:               "no-backend-stream",
		DeliveryStreamType: "KinesisStreamAsSource",
		Source: &firehose.SourceDescription{
			KinesisStreamSourceDescription: &firehose.KinesisStreamSourceDescription{
				KinesisStreamARN: streamARN,
			},
		},
	})
	require.NoError(t, err)

	// Stream is created successfully, no goroutine started, no panic.
	assert.Equal(t, int64(0), totalRecords(t, b, "no-backend-stream"))
}

func TestFirehose_KinesisSource_ResetCancelsPoller(t *testing.T) {
	t.Parallel()

	b := newFirehoseBackend(t)
	kinesis := &mockKinesisReader{} // no records, infinite polling

	b.SetKinesisBackend(kinesis)

	streamARN := "arn:aws:kinesis:us-east-1:123456789012:stream/reset-stream"
	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name:               "reset-poll-stream",
		DeliveryStreamType: "KinesisStreamAsSource",
		Source: &firehose.SourceDescription{
			KinesisStreamSourceDescription: &firehose.KinesisStreamSourceDescription{
				KinesisStreamARN: streamARN,
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, 1, firehose.PollerCount(b), "poller must be tracked immediately after CreateDeliveryStream")

	b.Reset()

	assert.Equal(t, 0, firehose.PollerCount(b),
		"Reset must cancel and forget every running Kinesis source poller, not just clear the streams table")
}

func TestFirehose_KinesisSource_NoBackendLogsWarning(t *testing.T) {
	t.Parallel()

	b := newFirehoseBackend(t)
	// no kinesis backend wired

	var logBuf bytes.Buffer
	testLogger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := logger.Save(context.TODO(), testLogger)

	streamARN := "arn:aws:kinesis:us-east-1:123456789012:stream/no-backend-logged"
	_, err := b.CreateDeliveryStream(ctx, firehose.CreateDeliveryStreamInput{
		Name:               "no-backend-logged-stream",
		DeliveryStreamType: "KinesisStreamAsSource",
		Source: &firehose.SourceDescription{
			KinesisStreamSourceDescription: &firehose.KinesisStreamSourceDescription{
				KinesisStreamARN: streamARN,
			},
		},
	})
	require.NoError(t, err)

	assert.Contains(t, logBuf.String(), "no Kinesis backend wired",
		"a stream created without a wired Kinesis backend must warn, not silently drop ingestion forever")
}

func TestFirehose_KinesisSource_DirectPutUnaffected(t *testing.T) {
	t.Parallel()

	b := newFirehoseBackend(t)
	kinesis := &mockKinesisReader{}
	b.SetKinesisBackend(kinesis)

	// Direct-put stream should work as before; no poller started.
	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "direct-put-stream",
	})
	require.NoError(t, err)

	err = b.PutRecord(context.TODO(), "direct-put-stream", []byte("hello"))
	require.NoError(t, err)

	// Metrics are copied (not cleared) by streamCopy so TotalRecords is reliable.
	assert.Equal(t, int64(1), totalRecords(t, b, "direct-put-stream"))
}

func TestFirehose_KinesisSource_ListShardsError_NoBlock(t *testing.T) {
	t.Parallel()

	b := newFirehoseBackend(t)
	kinesis := &mockKinesisReader{listErr: errAccessDenied}
	b.SetKinesisBackend(kinesis)

	streamARN := "arn:aws:kinesis:us-east-1:123456789012:stream/error-stream"
	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name:               "error-stream",
		DeliveryStreamType: "KinesisStreamAsSource",
		Source: &firehose.SourceDescription{
			KinesisStreamSourceDescription: &firehose.KinesisStreamSourceDescription{
				KinesisStreamARN: streamARN,
			},
		},
	})
	require.NoError(t, err, "CreateDeliveryStream must succeed even when Kinesis polling will fail")

	// Give the goroutine time to attempt and fail.
	time.Sleep(100 * time.Millisecond)

	// No panic, no records.
	assert.Equal(t, int64(0), totalRecords(t, b, "error-stream"))
}
