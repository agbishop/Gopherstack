package cloudwatchlogs

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ExportSink is the minimal S3 write surface needed to materialise export tasks.
// The CloudWatch Logs export feature writes gzipped log data to a destination S3
// bucket; injecting a sink keeps the backend decoupled from the S3 service while
// still performing real writes when wired at the server level.
type ExportSink interface {
	// PutObject writes body to the given bucket under key. Implementations must
	// treat the call as a create/overwrite of a single object.
	PutObject(ctx context.Context, bucket, key string, body []byte) error
}

// defaultExportPrefix is the S3 key prefix AWS uses when none is supplied.
const defaultExportPrefix = "exportedlogs"

// exportTimeFormat is the timestamp layout prefixed to each exported log line,
// matching the AWS export line format "timestamp message".
const exportTimeFormat = "2006-01-02T15:04:05.000Z"

// SetExportSink configures the S3 sink used to materialise export tasks. When a
// sink is set, CreateExportTask writes matching events to S3 and completes the
// task synchronously; without one, tasks advance by janitor age (legacy path).
func (b *InMemoryBackend) SetExportSink(sink ExportSink) {
	b.mu.Lock("SetExportSink")
	defer b.mu.Unlock()

	b.exportSink = sink
}

// runExport gathers the log events for a task's group/stream-prefix within its
// [from,to) window, writes one gzipped object per stream to the destination
// bucket using the AWS key layout, and returns the number of events exported.
func (b *InMemoryBackend) runExport(ctx context.Context, task *ExportTask) (int, error) {
	prefix := task.DestinationPrefix
	if prefix == "" {
		prefix = defaultExportPrefix
	}

	streams := b.collectExportStreams(task)

	total := 0
	for _, s := range streams {
		body, err := gzipExportEvents(s.events)
		if err != nil {
			return total, err
		}

		key := fmt.Sprintf("%s/%s/%s/000000.gz", strings.Trim(prefix, "/"), task.TaskID, s.name)
		if putErr := b.exportSink.PutObject(ctx, task.Destination, key, body); putErr != nil {
			return total, fmt.Errorf("export write failed: %w", putErr)
		}

		total += len(s.events)
	}

	return total, nil
}

// exportStream holds the ordered events destined for a single S3 object.
type exportStream struct {
	name   string
	events []*OutputLogEvent
}

// collectExportStreams returns per-stream event slices in the task window,
// filtered by the task's log-stream-name prefix and sorted by timestamp.
func (b *InMemoryBackend) collectExportStreams(task *ExportTask) []exportStream {
	b.mu.RLock("collectExportStreams")
	defer b.mu.RUnlock()

	region := task.Region
	if region == "" {
		region = b.region
	}

	groupStreams := b.streamsInGroup(region, task.LogGroupName)
	streamsByName := make(map[string]*LogStream, len(groupStreams))
	names := make([]string, 0, len(groupStreams))
	for _, s := range groupStreams {
		if task.LogStreamNamePrefix == "" || strings.HasPrefix(s.LogStreamName, task.LogStreamNamePrefix) {
			names = append(names, s.LogStreamName)
			streamsByName[s.LogStreamName] = s
		}
	}

	sort.Strings(names)

	out := make([]exportStream, 0, len(names))
	for _, name := range names {
		if evts := exportWindowEvents(streamsByName[name].events, task.From, task.To); len(evts) > 0 {
			out = append(out, exportStream{name: name, events: evts})
		}
	}

	return out
}

// exportWindowEvents returns a timestamp-sorted copy of the events in [from,to).
func exportWindowEvents(events []*OutputLogEvent, from, to int64) []*OutputLogEvent {
	out := make([]*OutputLogEvent, 0, len(events))
	for _, ev := range events {
		if ev.Timestamp >= from && ev.Timestamp < to {
			out = append(out, ev)
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })

	return out
}

// gzipExportEvents renders events as "timestamp message" lines and gzips them,
// matching the AWS S3 export object payload.
func gzipExportEvents(events []*OutputLogEvent) ([]byte, error) {
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	for _, ev := range events {
		ts := time.UnixMilli(ev.Timestamp).UTC().Format(exportTimeFormat)
		if _, err := fmt.Fprintf(gz, "%s %s\n", ts, ev.Message); err != nil {
			_ = gz.Close()

			return nil, fmt.Errorf("gzip write failed: %w", err)
		}
	}

	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("gzip close failed: %w", err)
	}

	return buf.Bytes(), nil
}
