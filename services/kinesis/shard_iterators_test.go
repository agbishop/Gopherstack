package kinesis_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kinesis"
)

func TestGetShardIterator_ByARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateStream", map[string]any{"StreamName": "gsi-arn-stream", "ShardCount": 1})

	b := h.Backend.(*kinesis.InMemoryBackend)
	desc, err := b.DescribeStream(context.Background(), &kinesis.DescribeStreamInput{StreamName: "gsi-arn-stream"})
	require.NoError(t, err)
	require.NotEmpty(t, desc.Shards)
	shardID := desc.Shards[0].ShardID

	tests := []struct {
		body map[string]any
		name string
	}{
		{
			name: "by_name",
			body: map[string]any{
				"StreamName":        "gsi-arn-stream",
				"ShardId":           shardID,
				"ShardIteratorType": "TRIM_HORIZON",
			},
		},
		{
			name: "by_arn",
			body: map[string]any{
				"StreamARN":         desc.StreamARN,
				"ShardId":           shardID,
				"ShardIteratorType": "TRIM_HORIZON",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, "GetShardIterator", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				ShardIterator string `json:"ShardIterator"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotEmpty(t, resp.ShardIterator)
		})
	}
}

func TestGetShardIterator_AllTypes(t *testing.T) {
	t.Parallel()

	b := newParityBackend(t)
	ctx := context.Background()

	createParityStream(t, b, "iter-types", 1)

	seqs := make([]string, 0, 5)
	timestamps := make([]time.Time, 0, 5)

	for i := range 5 {
		time.Sleep(time.Millisecond)
		out, err := b.PutRecord(ctx, &kinesis.PutRecordInput{
			StreamName:   "iter-types",
			PartitionKey: "pk",
			Data:         fmt.Appendf(nil, "r%d", i),
		})
		require.NoError(t, err)
		seqs = append(seqs, out.SequenceNumber)
		timestamps = append(timestamps, time.Now())
	}

	shardID := "shardId-000000000000"

	t.Run("TRIM_HORIZON reads from offset 0", func(t *testing.T) {
		t.Parallel()

		itOut, err := b.GetShardIterator(ctx, &kinesis.GetShardIteratorInput{
			StreamName:        "iter-types",
			ShardID:           shardID,
			ShardIteratorType: "TRIM_HORIZON",
		})
		require.NoError(t, err)

		rOut, err := b.GetRecords(ctx, &kinesis.GetRecordsInput{ShardIterator: itOut.ShardIterator})
		require.NoError(t, err)
		require.Len(t, rOut.Records, 5)
		assert.Equal(t, seqs[0], rOut.Records[0].SequenceNumber)
	})

	t.Run("LATEST returns no records", func(t *testing.T) {
		t.Parallel()

		itOut, err := b.GetShardIterator(ctx, &kinesis.GetShardIteratorInput{
			StreamName:        "iter-types",
			ShardID:           shardID,
			ShardIteratorType: "LATEST",
		})
		require.NoError(t, err)

		rOut, err := b.GetRecords(ctx, &kinesis.GetRecordsInput{ShardIterator: itOut.ShardIterator})
		require.NoError(t, err)
		assert.Empty(t, rOut.Records)
	})

	t.Run("AT_SEQUENCE_NUMBER starts at the given seq", func(t *testing.T) {
		t.Parallel()

		itOut, err := b.GetShardIterator(ctx, &kinesis.GetShardIteratorInput{
			StreamName:             "iter-types",
			ShardID:                shardID,
			ShardIteratorType:      "AT_SEQUENCE_NUMBER",
			StartingSequenceNumber: seqs[2],
		})
		require.NoError(t, err)

		rOut, err := b.GetRecords(ctx, &kinesis.GetRecordsInput{ShardIterator: itOut.ShardIterator})
		require.NoError(t, err)
		require.NotEmpty(t, rOut.Records)
		assert.Equal(t, seqs[2], rOut.Records[0].SequenceNumber)
	})

	t.Run("AFTER_SEQUENCE_NUMBER starts after the given seq", func(t *testing.T) {
		t.Parallel()

		itOut, err := b.GetShardIterator(ctx, &kinesis.GetShardIteratorInput{
			StreamName:             "iter-types",
			ShardID:                shardID,
			ShardIteratorType:      "AFTER_SEQUENCE_NUMBER",
			StartingSequenceNumber: seqs[2],
		})
		require.NoError(t, err)

		rOut, err := b.GetRecords(ctx, &kinesis.GetRecordsInput{ShardIterator: itOut.ShardIterator})
		require.NoError(t, err)
		require.NotEmpty(t, rOut.Records)
		assert.Equal(t, seqs[3], rOut.Records[0].SequenceNumber)
	})

	t.Run("AT_TIMESTAMP returns records at or after timestamp", func(t *testing.T) {
		t.Parallel()

		itOut, err := b.GetShardIterator(ctx, &kinesis.GetShardIteratorInput{
			StreamName:        "iter-types",
			ShardID:           shardID,
			ShardIteratorType: "AT_TIMESTAMP",
			Timestamp:         &timestamps[3],
		})
		require.NoError(t, err)

		rOut, err := b.GetRecords(ctx, &kinesis.GetRecordsInput{ShardIterator: itOut.ShardIterator})
		require.NoError(t, err)
		assert.NotEmpty(t, rOut.Records)
	})
}

func TestGetShardIteratorBadIteratorType(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create stream
	rec := doRequest(t, h, "CreateStream", map[string]any{
		"StreamName": "bad-iter-stream",
		"ShardCount": 1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Get shard ID
	rec = doRequest(t, h, "DescribeStream", map[string]any{"StreamName": "bad-iter-stream"})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp struct {
		StreamDescription struct {
			Shards []struct {
				ShardID string `json:"ShardId"`
			} `json:"Shards"`
		} `json:"StreamDescription"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	shardID := descResp.StreamDescription.Shards[0].ShardID

	rec = doRequest(t, h, "GetShardIterator", map[string]any{
		"StreamName":        "bad-iter-stream",
		"ShardId":           shardID,
		"ShardIteratorType": "INVALID_TYPE",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetShardIteratorNonExistentShard(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateStream", map[string]any{
		"StreamName": "no-shard-stream",
		"ShardCount": 1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "GetShardIterator", map[string]any{
		"StreamName":        "no-shard-stream",
		"ShardId":           "shardId-not-real",
		"ShardIteratorType": "TRIM_HORIZON",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetShardIteratorAtTimestamp(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateStream", map[string]any{
		"StreamName": "ts-stream",
		"ShardCount": 1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Get shard ID
	rec = doRequest(t, h, "DescribeStream", map[string]any{"StreamName": "ts-stream"})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp struct {
		StreamDescription struct {
			Shards []struct {
				ShardID string `json:"ShardId"`
			} `json:"Shards"`
		} `json:"StreamDescription"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	require.Len(t, descResp.StreamDescription.Shards, 1)
	shardID := descResp.StreamDescription.Shards[0].ShardID

	// Put a record.
	doRequest(t, h, "PutRecord", map[string]any{
		"StreamName":   "ts-stream",
		"PartitionKey": "pk",
		"Data":         []byte("hello"),
	})

	// Get shard iterator at current time (should include the record).
	tsBefore := float64(0) // epoch = all records
	rec = doRequest(t, h, "GetShardIterator", map[string]any{
		"StreamName":        "ts-stream",
		"ShardId":           shardID,
		"ShardIteratorType": "AT_TIMESTAMP",
		"Timestamp":         tsBefore,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var iterResp struct {
		ShardIterator string `json:"ShardIterator"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &iterResp))
	assert.NotEmpty(t, iterResp.ShardIterator)

	// GetRecords should return the record.
	rec = doRequest(t, h, "GetRecords", map[string]any{
		"ShardIterator": iterResp.ShardIterator,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var getResp struct {
		Records []struct {
			Data []byte `json:"Data"`
		} `json:"Records"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
	assert.Len(t, getResp.Records, 1)
	assert.Equal(t, []byte("hello"), getResp.Records[0].Data)
}

// TestGetShardIterator_AtTimestampRequiresTimestamp verifies AT_TIMESTAMP
// rejects a genuinely omitted Timestamp field with InvalidArgumentException
// instead of silently treating it as position 0 (epoch/TRIM_HORIZON). An
// explicit Timestamp of 0 (present on the wire) is still accepted -- see
// TestGetShardIteratorAtTimestamp.
func TestGetShardIterator_AtTimestampRequiresTimestamp(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateStream", map[string]any{"StreamName": "ts-required-stream", "ShardCount": 1})

	rec := doRequest(t, h, "DescribeStream", map[string]any{"StreamName": "ts-required-stream"})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp struct {
		StreamDescription struct {
			Shards []struct {
				ShardID string `json:"ShardId"`
			} `json:"Shards"`
		} `json:"StreamDescription"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	shardID := descResp.StreamDescription.Shards[0].ShardID

	rec = doRequest(t, h, "GetShardIterator", map[string]any{
		"StreamName":        "ts-required-stream",
		"ShardId":           shardID,
		"ShardIteratorType": "AT_TIMESTAMP",
		// Timestamp deliberately omitted.
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp struct {
		Type string `json:"__type"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "InvalidArgumentException", errResp.Type)
}

// TestGetShardIterator_AtTimestampNilRejectedAtBackend is the backend-level
// counterpart of TestGetShardIterator_AtTimestampRequiresTimestamp, verifying
// a nil GetShardIteratorInput.Timestamp is rejected directly (not just when
// routed through the JSON-omission path).
func TestGetShardIterator_AtTimestampNilRejectedAtBackend(t *testing.T) {
	t.Parallel()

	b := newParityBackend(t)
	createParityStream(t, b, "ts-nil-stream", 1)

	shardsOut, err := b.ListShards(context.Background(), &kinesis.ListShardsInput{StreamName: "ts-nil-stream"})
	require.NoError(t, err)
	require.NotEmpty(t, shardsOut.Shards)

	_, err = b.GetShardIterator(context.Background(), &kinesis.GetShardIteratorInput{
		StreamName:        "ts-nil-stream",
		ShardID:           shardsOut.Shards[0].ShardID,
		ShardIteratorType: "AT_TIMESTAMP",
		Timestamp:         nil,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, kinesis.ErrInvalidArgument)
}

// TestGetRecords_NextShardIteratorHasExpiry verifies GetRecords' chained
// NextShardIterator carries a CreatedAt timestamp, exactly like the initial
// token GetShardIterator issues. AWS documents every shard iterator --
// including a NextShardIterator returned by GetRecords, not just the first
// one from GetShardIterator -- as expiring 5 minutes after being returned to
// the requester. decodeIterator's expiry check (shard_iterators.go) only
// fires when CreatedAt is non-zero, so a token minted with a zero CreatedAt
// bypasses ExpiredIteratorException forever.
func TestGetRecords_NextShardIteratorHasExpiry(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateStream", map[string]any{"StreamName": "next-iter-ttl-stream", "ShardCount": 1})

	b := h.Backend.(*kinesis.InMemoryBackend)
	ctx := context.Background()

	desc, err := b.DescribeStream(ctx, &kinesis.DescribeStreamInput{StreamName: "next-iter-ttl-stream"})
	require.NoError(t, err)
	require.NotEmpty(t, desc.Shards)
	shardID := desc.Shards[0].ShardID

	_, err = b.PutRecord(ctx, &kinesis.PutRecordInput{
		StreamName:   "next-iter-ttl-stream",
		PartitionKey: "pk",
		Data:         []byte("hello"),
	})
	require.NoError(t, err)

	itOut, err := b.GetShardIterator(ctx, &kinesis.GetShardIteratorInput{
		StreamName:        "next-iter-ttl-stream",
		ShardID:           shardID,
		ShardIteratorType: "TRIM_HORIZON",
	})
	require.NoError(t, err)

	recOut, err := b.GetRecords(ctx, &kinesis.GetRecordsInput{ShardIterator: itOut.ShardIterator})
	require.NoError(t, err)
	require.NotEmpty(t, recOut.NextShardIterator)

	raw, err := base64.StdEncoding.DecodeString(recOut.NextShardIterator)
	require.NoError(t, err)

	var next kinesis.ShardIterator
	require.NoError(t, json.Unmarshal(raw, &next))

	assert.False(t, next.CreatedAt.IsZero(),
		"NextShardIterator must carry a CreatedAt so ExpiredIteratorException can ever fire on a chained token")
	assert.WithinDuration(t, time.Now(), next.CreatedAt, 5*time.Second)
}
