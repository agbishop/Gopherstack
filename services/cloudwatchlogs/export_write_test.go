package cloudwatchlogs_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatchlogs"
)

// fakeExportSink captures objects written by the export task.
type fakeExportSink struct {
	objects map[string][]byte
	err     error
	bucket  string
	mu      sync.Mutex
}

func newFakeExportSink() *fakeExportSink {
	return &fakeExportSink{objects: make(map[string][]byte)}
}

func (f *fakeExportSink) PutObject(_ context.Context, bucket, key string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return f.err
	}

	f.bucket = bucket
	f.objects[key] = body

	return nil
}

func (f *fakeExportSink) get(key string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	b, ok := f.objects[key]

	return b, ok
}

func gunzip(t *testing.T, data []byte) string {
	t.Helper()

	gr, err := gzip.NewReader(bytes.NewReader(data))
	require.NoError(t, err)

	out, err := io.ReadAll(gr)
	require.NoError(t, err)
	require.NoError(t, gr.Close())

	return string(out)
}

func exportBackend(t *testing.T) *cloudwatchlogs.InMemoryBackend {
	t.Helper()

	b := cloudwatchlogs.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	_, err := b.CreateLogGroup(context.Background(), "/grp", "", "")
	require.NoError(t, err)
	_, err = b.CreateLogStream(context.Background(), "/grp", "streamA")
	require.NoError(t, err)
	_, err = b.CreateLogStream(context.Background(), "/grp", "streamB")
	require.NoError(t, err)

	_, err = b.PutLogEvents(context.Background(), "/grp", "streamA", "", []cloudwatchlogs.InputLogEvent{
		{Message: "hello", Timestamp: 1500},
		{Message: "world", Timestamp: 1700},
		{Message: "too-late", Timestamp: 5000},
	})
	require.NoError(t, err)
	_, err = b.PutLogEvents(context.Background(), "/grp", "streamB", "", []cloudwatchlogs.InputLogEvent{
		{Message: "other", Timestamp: 1600},
	})
	require.NoError(t, err)

	return b
}

func TestExportTask_WritesGzippedObjects(t *testing.T) {
	t.Parallel()

	b := exportBackend(t)
	sink := newFakeExportSink()
	b.SetExportSink(sink)

	taskID, err := b.CreateExportTask(context.Background(), "t", "/grp", "", "dest-bucket", "myprefix", 1000, 2000)
	require.NoError(t, err)
	require.NotEmpty(t, taskID)

	keyA := "myprefix/" + taskID + "/streamA/000000.gz"
	keyB := "myprefix/" + taskID + "/streamB/000000.gz"

	dataA, ok := sink.get(keyA)
	require.True(t, ok, "streamA object should exist")
	dataB, ok := sink.get(keyB)
	require.True(t, ok, "streamB object should exist")

	assert.Equal(t, "dest-bucket", sink.bucket)

	// streamA: only hello(1500) and world(1700) are in [1000,2000); too-late(5000) excluded.
	lines := strings.Split(strings.TrimRight(gunzip(t, dataA), "\n"), "\n")
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "hello")
	assert.Contains(t, lines[1], "world")

	assert.Contains(t, gunzip(t, dataB), "other")

	// Task completes synchronously with COMPLETED status.
	tasks, _, err := b.DescribeExportTasks(taskID, "", 10, "")
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "COMPLETED", tasks[0].Status)
	assert.Positive(t, tasks[0].CompletionTime)
}

func TestExportTask_StreamPrefixFilter(t *testing.T) {
	t.Parallel()

	b := exportBackend(t)
	sink := newFakeExportSink()
	b.SetExportSink(sink)

	taskID, err := b.CreateExportTask(context.Background(), "t", "/grp", "streamA", "dest", "p", 1000, 6000)
	require.NoError(t, err)

	_, okA := sink.get("p/" + taskID + "/streamA/000000.gz")
	_, okB := sink.get("p/" + taskID + "/streamB/000000.gz")
	assert.True(t, okA, "streamA exported")
	assert.False(t, okB, "streamB filtered out by prefix")
}

func TestExportTask_DefaultPrefix(t *testing.T) {
	t.Parallel()

	b := exportBackend(t)
	sink := newFakeExportSink()
	b.SetExportSink(sink)

	taskID, err := b.CreateExportTask(context.Background(), "t", "/grp", "streamA", "dest", "", 1000, 2000)
	require.NoError(t, err)

	_, ok := sink.get("exportedlogs/" + taskID + "/streamA/000000.gz")
	assert.True(t, ok, "default prefix exportedlogs used")
}

func TestExportTask_NoSinkStaysPending(t *testing.T) {
	t.Parallel()

	b := exportBackend(t)

	taskID, err := b.CreateExportTask(context.Background(), "t", "/grp", "", "dest", "p", 1000, 2000)
	require.NoError(t, err)

	tasks, _, err := b.DescribeExportTasks(taskID, "", 10, "")
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "PENDING", tasks[0].Status)
}
