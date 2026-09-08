package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	kinesisbackend "github.com/blackbirdworks/gopherstack/services/kinesis"
)

// TestKinesisReaderAdapter_ResolvesStreamRegionFromARN is the regression
// test for gopherstack-qowd: kinesisReaderAdapter used to call the Kinesis
// backend with context.Background(), so a Kinesis-to-Lambda event source
// mapping always resolved the stream in the account default region, no
// matter which region the mapping's EventSourceArn actually named. Two
// streams sharing a name but living in different regions -- and different
// shard counts -- make a region mix-up directly observable: GetShardIDs and
// GetShardIterator must resolve each stream's own region, not silently fall
// back to the default.
func TestKinesisReaderAdapter_ResolvesStreamRegionFromARN(t *testing.T) {
	t.Parallel()

	bk := kinesisbackend.NewInMemoryBackendWithConfig("000000000000", "us-east-1")

	require.NoError(t, bk.CreateStream(context.Background(), &kinesisbackend.CreateStreamInput{
		StreamName: "shared-stream",
		ShardCount: 1,
	}))

	euCtx, _ := kinesisbackend.ContextAndNameFromStreamARN(
		context.Background(), "arn:aws:kinesis:eu-west-1:000000000000:stream/shared-stream",
	)
	require.NoError(t, bk.CreateStream(euCtx, &kinesisbackend.CreateStreamInput{
		StreamName: "shared-stream",
		ShardCount: 3,
	}))

	adapter := &kinesisReaderAdapter{backend: bk}

	const (
		usARN = "arn:aws:kinesis:us-east-1:000000000000:stream/shared-stream"
		euARN = "arn:aws:kinesis:eu-west-1:000000000000:stream/shared-stream"
	)

	usShards, err := adapter.GetShardIDs(usARN)
	require.NoError(t, err)
	require.Len(t, usShards, 1, "the us-east-1 stream must resolve its own shard count")

	euShards, err := adapter.GetShardIDs(euARN)
	require.NoError(t, err)
	require.Len(t, euShards, 3,
		"the eu-west-1 stream must resolve its own shard count, not fall back to the account default region")

	// euShards[2] only exists on the 3-shard eu-west-1 stream. If
	// GetShardIterator silently resolved the default-region (1-shard)
	// stream instead, this shard ID would not be found there.
	_, err = adapter.GetShardIterator(euARN, euShards[2], "TRIM_HORIZON", "")
	require.NoError(t, err, "GetShardIterator must resolve the eu-west-1 stream, not the account default region")
}
