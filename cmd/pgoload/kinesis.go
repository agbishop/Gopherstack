package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	kinesistypes "github.com/aws/aws-sdk-go-v2/service/kinesis/types"
)

const (
	kinesisStreamName = "pgoload-stream"
	kinesisShardCount = 1

	kinesisPutBatchSize = 10
	kinesisPayloadSize  = 256
)

// errStreamNotActive is returned when the Kinesis stream fails to reach the
// ACTIVE status within setupTimeout.
var errStreamNotActive = errors.New("kinesis stream did not become active in time")

// ensureKinesisStream creates kinesisStreamName, tolerating one that already
// exists, waits for it to become ACTIVE, and returns its first shard's ID
// for the GetRecords op.
func ensureKinesisStream(ctx context.Context, cl *kinesis.Client, log *slog.Logger) (string, error) {
	_, err := cl.CreateStream(ctx, &kinesis.CreateStreamInput{
		StreamName: aws.String(kinesisStreamName),
		ShardCount: aws.Int32(kinesisShardCount),
	})
	if err != nil {
		if _, ok := errors.AsType[*kinesistypes.ResourceInUseException](err); !ok {
			return "", fmt.Errorf("create stream %s: %w", kinesisStreamName, err)
		}

		log.InfoContext(ctx, "kinesis stream already exists", "stream", kinesisStreamName)
	}

	shardID, err := waitStreamActive(ctx, cl, log)
	if err != nil {
		return "", err
	}

	return shardID, nil
}

func waitStreamActive(ctx context.Context, cl *kinesis.Client, log *slog.Logger) (string, error) {
	deadline := time.Now().Add(setupTimeout)

	for time.Now().Before(deadline) {
		out, err := cl.DescribeStreamSummary(ctx, &kinesis.DescribeStreamSummaryInput{
			StreamName: aws.String(kinesisStreamName),
		})
		if err == nil && out.StreamDescriptionSummary != nil &&
			out.StreamDescriptionSummary.StreamStatus == kinesistypes.StreamStatusActive {
			shards, shardsErr := cl.ListShards(ctx, &kinesis.ListShardsInput{StreamName: aws.String(kinesisStreamName)})
			if shardsErr != nil || len(shards.Shards) == 0 {
				return "", fmt.Errorf("list shards for %s: %w", kinesisStreamName, shardsErr)
			}

			return aws.ToString(shards.Shards[0].ShardId), nil
		}

		t := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			t.Stop()

			return "", fmt.Errorf("waiting for stream %s: %w", kinesisStreamName, ctx.Err())
		case <-t.C:
		}
	}

	log.WarnContext(ctx, "gave up waiting for stream to become active", "stream", kinesisStreamName)

	return "", fmt.Errorf("%w: stream=%s", errStreamNotActive, kinesisStreamName)
}

// kinesisWorker repeatedly runs a mix of Kinesis operations, staggered by
// workerID, until ctx is done.
func kinesisWorker(
	ctx context.Context,
	cl *kinesis.Client,
	res *resources,
	workerID int,
	c *opCounter,
	log *slog.Logger,
) {
	ops := []opFunc{
		func(ctx context.Context, workerID, i int) error { return kinesisPutRecordOp(ctx, cl, workerID, i) },
		func(ctx context.Context, workerID, i int) error { return kinesisPutRecordOp(ctx, cl, workerID, i) },
		func(ctx context.Context, workerID, i int) error { return kinesisPutRecordsOp(ctx, cl, workerID, i) },
		func(ctx context.Context, _, _ int) error {
			return kinesisGetRecordsOp(ctx, cl, res.kinesisShardID)
		},
		func(ctx context.Context, _, _ int) error { return kinesisDescribeStreamOp(ctx, cl) },
	}

	runOpLoop(ctx, workerID, ops, c, "kinesis", log)
}

func kinesisPartitionKey(workerID, i int) string {
	return fmt.Sprintf("worker-%d-%d", workerID, i)
}

func kinesisPutRecordOp(ctx context.Context, cl *kinesis.Client, workerID, i int) error {
	_, err := cl.PutRecord(ctx, &kinesis.PutRecordInput{
		StreamName:   aws.String(kinesisStreamName),
		PartitionKey: aws.String(kinesisPartitionKey(workerID, i)),
		Data:         []byte(strings.Repeat("k", kinesisPayloadSize)),
	})

	return err
}

func kinesisPutRecordsOp(ctx context.Context, cl *kinesis.Client, workerID, base int) error {
	records := make([]kinesistypes.PutRecordsRequestEntry, 0, kinesisPutBatchSize)
	for j := range kinesisPutBatchSize {
		records = append(records, kinesistypes.PutRecordsRequestEntry{
			PartitionKey: aws.String(kinesisPartitionKey(workerID, base+j)),
			Data:         []byte(strings.Repeat("k", kinesisPayloadSize)),
		})
	}

	_, err := cl.PutRecords(ctx, &kinesis.PutRecordsInput{
		StreamName: aws.String(kinesisStreamName),
		Records:    records,
	})

	return err
}

// kinesisGetRecordsOp fetches a fresh shard iterator and reads from it,
// mirroring how a real consumer polls: two wire calls per iteration.
func kinesisGetRecordsOp(ctx context.Context, cl *kinesis.Client, shardID string) error {
	iter, err := cl.GetShardIterator(ctx, &kinesis.GetShardIteratorInput{
		StreamName:        aws.String(kinesisStreamName),
		ShardId:           aws.String(shardID),
		ShardIteratorType: kinesistypes.ShardIteratorTypeLatest,
	})
	if err != nil {
		return fmt.Errorf("get shard iterator: %w", err)
	}

	if _, getErr := cl.GetRecords(ctx, &kinesis.GetRecordsInput{ShardIterator: iter.ShardIterator}); getErr != nil {
		return fmt.Errorf("get records: %w", getErr)
	}

	return nil
}

func kinesisDescribeStreamOp(ctx context.Context, cl *kinesis.Client) error {
	_, err := cl.DescribeStreamSummary(ctx, &kinesis.DescribeStreamSummaryInput{
		StreamName: aws.String(kinesisStreamName),
	})

	return err
}
