package firehose

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

const (
	kinesisPollerInterval   = time.Second
	kinesisPollerBatchLimit = 100
	kinesisARNSplitParts    = 2 // SplitN limit: prefix + stream-name
)

// launchKinesisPoller starts one goroutine per shard for the given Kinesis source stream.
// It stores the cancel func in b.pollerCancel[region][streamName] so DeleteDeliveryStream can stop it.
func (b *InMemoryBackend) launchKinesisPoller(region, firehoseStream, kinesisStreamARN string) {
	ctx, cancel := context.WithCancel(b.svcCtx)

	func() {
		b.mu.Lock("launchKinesisPoller")
		defer b.mu.Unlock()

		b.pollerStore(region)[firehoseStream] = cancel
	}()

	go b.pollKinesisStream(ctx, region, firehoseStream, kinesisStreamARN)
}

// pollKinesisStream lists shards and starts a per-shard polling loop.
func (b *InMemoryBackend) pollKinesisStream(
	ctx context.Context,
	region, firehoseStream, kinesisStreamARN string,
) {
	streamName := kinesisStreamNameFromARN(kinesisStreamARN)
	if streamName == "" {
		streamName = kinesisStreamARN
	}

	shards, err := b.kinesisBackend.ListShards(streamName)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "firehose kinesis poller: ListShards failed",
			"region", region, "stream", firehoseStream, "kinesis", streamName, "error", err)

		return
	}

	for _, shardID := range shards {
		sid := shardID
		go b.pollKinesisShard(ctx, region, firehoseStream, streamName, sid)
	}
}

// pollKinesisShard reads records from a single Kinesis shard and injects them into the Firehose stream.
func (b *InMemoryBackend) pollKinesisShard(
	ctx context.Context,
	region, firehoseStream, kinesisStream, shardID string,
) {
	iter, err := b.kinesisBackend.GetShardIterator(kinesisStream, shardID)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "firehose kinesis poller: GetShardIterator failed",
			"region", region, "stream", firehoseStream, "shard", shardID, "error", err)

		return
	}

	for {
		if isDone(ctx) {
			return
		}

		records, nextIter, getErr := b.kinesisBackend.GetRecords(iter, kinesisPollerBatchLimit)
		if getErr != nil {
			logger.Load(ctx).WarnContext(ctx, "firehose kinesis poller: GetRecords failed",
				"region", region, "stream", firehoseStream, "shard", shardID, "error", getErr)

			if waitOrDone(ctx, kinesisPollerInterval) {
				return
			}

			continue
		}

		for _, rec := range records {
			if injectErr := b.injectKinesisRecord(region, firehoseStream, rec); injectErr != nil {
				logger.Load(ctx).WarnContext(ctx, "firehose kinesis poller: inject failed",
					"region", region, "stream", firehoseStream, "error", injectErr)
			}
		}

		if nextIter == "" {
			return
		}

		iter = nextIter

		if len(records) == 0 && waitOrDone(ctx, kinesisPollerInterval) {
			return
		}
	}
}

func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// waitOrDone sleeps for d or until ctx is done. Returns true if ctx was cancelled.
func waitOrDone(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return true
	case <-t.C:
		return false
	}
}

// injectKinesisRecord appends a record to a KinesisStreamAsSource stream, bypassing the
// DirectPut type guard that PutRecord enforces.
func (b *InMemoryBackend) injectKinesisRecord(region, streamName string, data []byte) error {
	b.mu.Lock("injectKinesisRecord")
	defer b.mu.Unlock()

	s, ok := b.streams.Get(regionKey(region, streamName))
	if !ok {
		return fmt.Errorf("%w: stream %s not found in region %s", ErrNotFound, streamName, region)
	}

	s.Records = append(s.Records, data)
	s.Metrics.TotalRecords++
	s.Metrics.TotalBytes += int64(len(data))

	return nil
}

// kinesisStreamNameFromARN extracts the stream name from a Kinesis ARN.
// ARN format: arn:aws:kinesis:<region>:<account>:stream/<name>.
func kinesisStreamNameFromARN(kinesisARN string) string {
	if !strings.Contains(kinesisARN, ":stream/") {
		return ""
	}

	parts := strings.SplitN(kinesisARN, ":stream/", kinesisARNSplitParts)
	if len(parts) == kinesisARNSplitParts {
		return parts[1]
	}

	return ""
}
