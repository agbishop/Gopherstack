package kinesis

import (
	"context"
	"errors"
	"math/big"
	"time"
)

// PutRecord writes a single record to a stream shard.
func (b *InMemoryBackend) PutRecord(ctx context.Context, input *PutRecordInput) (*PutRecordOutput, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("PutRecord")

	stream, exists := b.streams.Get(streamKey(region, input.StreamName))
	if !exists {
		b.mu.RUnlock()

		return nil, ErrStreamNotFound
	}
	stream.mu.Lock("PutRecord.stream")
	b.mu.RUnlock()
	defer stream.mu.Unlock()

	if b.isThroughputFaultActive(region, input.StreamName) {
		return nil, ErrProvisionedThroughputExceeded
	}

	// Reject writes if the stream is not active (e.g. CREATING/DELETING).
	if stream.Status != streamStatusActive {
		return nil, ErrInvalidArgument
	}

	// Validate partition key length (AWS requires 1–256 chars).
	if len(input.PartitionKey) == 0 || len(input.PartitionKey) > maxPartitionKeyLen {
		return nil, ErrInvalidArgument
	}

	// Enforce per-record data size limit (default 1 MiB; updatable via UpdateMaxRecordSize).
	maxSize := stream.MaxRecordSizeBytes
	if maxSize <= 0 {
		maxSize = defaultMaxRecordSizeBytes
	}
	if len(input.Data) > maxSize {
		return nil, ErrInvalidArgument
	}

	if len(stream.Shards) == 0 {
		return nil, ErrInvalidArgument
	}

	var shard *Shard
	if input.ExplicitHashKey != "" {
		routingHash := new(big.Int)
		if _, ok := routingHash.SetString(input.ExplicitHashKey, hashKeyDecimalBase); !ok {
			return nil, ErrInvalidArgument
		}
		// Validate range [0, 2^128-1].
		maxHashKey := new(big.Int).Sub(
			new(big.Int).Lsh(big.NewInt(1), maxHashKeyBits),
			big.NewInt(1),
		)
		if routingHash.Sign() < 0 || routingHash.Cmp(maxHashKey) > 0 {
			return nil, ErrInvalidArgument
		}
		shard = shardForHashKey(stream.Shards, routingHash)
	} else {
		shard = shardForPartitionKey(stream.Shards, input.PartitionKey)
	}
	if shard == nil {
		return nil, ErrInvalidArgument
	}

	seq := shard.nextSequenceNumber()
	record := &Record{
		PartitionKey:                input.PartitionKey,
		Data:                        input.Data,
		SequenceNumber:              seq,
		ApproximateArrivalTimestamp: time.Now(),
	}

	shard.Records.push(record)

	enc := stream.EncryptionType
	if enc == "" {
		enc = encryptionTypeNone
	}

	return &PutRecordOutput{
		ShardID:        shard.ID,
		SequenceNumber: seq,
		EncryptionType: enc,
	}, nil
}

// putRecordErrorCode maps a per-record PutRecord error to the AWS error code string
// that should appear in a PutRecords result entry.
func putRecordErrorCode(err error) string {
	if errors.Is(err, ErrProvisionedThroughputExceeded) {
		return "ProvisionedThroughputExceededException"
	}
	if errors.Is(err, ErrInvalidArgument) {
		return errTypeValidation
	}

	return errTypeInternalFailure
}

// PutRecords writes multiple records to a stream.
//
// Request-level validation errors (empty/oversized batch, unknown stream) fail
// the whole call with a single top-level exception, matching the AWS contract:
// only per-record issues (throughput, per-record validation) surface as
// per-entry ErrorCode/ErrorMessage inside a 200 response.
func (b *InMemoryBackend) PutRecords(ctx context.Context, input *PutRecordsInput) (*PutRecordsOutput, error) {
	// AWS PutRecords caps a request at 500 records and 5 MiB total payload
	// (sum of partition-key + data bytes across every entry), and rejects an
	// empty Records list outright (MinItems=1 in the SDK model).
	const (
		maxRecordsPerRequest = 500
		maxBatchPayloadBytes = 5 * 1024 * 1024
	)

	if len(input.Records) == 0 || len(input.Records) > maxRecordsPerRequest {
		return nil, ErrInvalidArgument
	}

	totalBytes := 0
	for _, r := range input.Records {
		totalBytes += len(r.PartitionKey) + len(r.Data)
	}

	if totalBytes > maxBatchPayloadBytes {
		return nil, ErrInvalidArgument
	}

	// Resolve the stream once up front: AWS fails the entire PutRecords call
	// with a top-level ResourceNotFoundException when the stream does not
	// exist, rather than reporting "InternalFailure" on every result entry.
	region := getRegion(ctx, b.region)
	b.mu.RLock("PutRecords.exists")
	_, exists := b.streams.Get(streamKey(region, input.StreamName))
	b.mu.RUnlock()
	if !exists {
		return nil, ErrStreamNotFound
	}

	results := make([]PutRecordsResultEntry, len(input.Records))
	failedCount := 0

	for i, entry := range input.Records {
		out, err := b.PutRecord(ctx, &PutRecordInput{
			StreamName:      input.StreamName,
			PartitionKey:    entry.PartitionKey,
			ExplicitHashKey: entry.ExplicitHashKey,
			Data:            entry.Data,
		})
		if err != nil {
			errCode := putRecordErrorCode(err)
			results[i] = PutRecordsResultEntry{
				ErrorCode:    errCode,
				ErrorMessage: err.Error(),
			}
			failedCount++
		} else {
			results[i] = PutRecordsResultEntry{
				ShardID:        out.ShardID,
				SequenceNumber: out.SequenceNumber,
			}
		}
	}

	return &PutRecordsOutput{
		Records:           results,
		FailedRecordCount: failedCount,
	}, nil
}

// GetRecords retrieves records starting at the given shard iterator position.
//
// The region is taken from the iterator token (encoded by GetShardIterator),
// not from ctx, so an iterator issued for one region always reads that region's
// records even if the GetRecords call carries a different ctx region.
func (b *InMemoryBackend) GetRecords(ctx context.Context, input *GetRecordsInput) (*GetRecordsOutput, error) {
	it, err := decodeIterator(input.ShardIterator)
	if err != nil {
		return nil, err
	}

	region := it.Region
	if region == "" {
		// Legacy token without an embedded region: fall back to the ctx region.
		region = getRegion(ctx, b.region)
	}

	b.mu.RLock("GetRecords")

	stream, exists := b.streams.Get(streamKey(region, it.StreamName))
	if !exists {
		b.mu.RUnlock()

		return nil, ErrStreamNotFound
	}
	stream.mu.RLock("GetRecords.stream")
	b.mu.RUnlock()
	defer stream.mu.RUnlock()

	if b.isThroughputFaultActive(region, it.StreamName) {
		return nil, ErrProvisionedThroughputExceeded
	}

	shard := findShard(stream.Shards, it.ShardID)

	if shard == nil {
		return nil, ErrInvalidArgument
	}

	limit := input.Limit
	if limit <= 0 {
		limit = defaultGetRecordsLimit
	}

	if limit > maxGetRecordsLimit {
		limit = maxGetRecordsLimit
	}

	enc := stream.EncryptionType
	if enc == "" {
		enc = encryptionTypeNone
	}

	start := min(it.Position, shard.Records.len())

	end := min(start+limit, shard.Records.len())

	// Apply 10 MiB response cap: stop before end if accumulated payload exceeds the limit.
	totalBytes := 0
	actualEnd := start
	results := make([]GetRecordResult, 0, end-start)
	for i := start; i < end; i++ {
		r := shard.Records.at(i)
		recordBytes := len(r.Data)
		if totalBytes+recordBytes > maxGetRecordsResponseBytes && len(results) > 0 {
			break
		}
		totalBytes += recordBytes
		results = append(results, GetRecordResult{
			Data:                        r.Data,
			PartitionKey:                r.PartitionKey,
			SequenceNumber:              r.SequenceNumber,
			ApproximateArrivalTimestamp: r.ApproximateArrivalTimestamp,
			EncryptionType:              enc,
		})
		actualEnd = i + 1
	}

	// Advance iterator position
	newIt := &ShardIterator{
		StreamName: it.StreamName,
		ShardID:    it.ShardID,
		Region:     region,
		Position:   actualEnd,
		CreatedAt:  time.Now(),
	}

	nextToken, childShards, err := nextIteratorAndChildShards(stream.Shards, shard, newIt, actualEnd)
	if err != nil {
		return nil, err
	}

	// MillisBehindLatest is the age of the last record in the shard (tip of stream).
	millisBehind := int64(0)
	if actualEnd < shard.Records.len() {
		millisBehind = time.Since(shard.Records.last().ApproximateArrivalTimestamp).Milliseconds()
	}

	return &GetRecordsOutput{
		Records:            results,
		NextShardIterator:  nextToken,
		ChildShards:        childShards,
		MillisBehindLatest: millisBehind,
	}, nil
}

// nextIteratorAndChildShards computes GetRecords' NextShardIterator and
// ChildShards together, since both are driven by the same end-of-shard
// condition: AWS returns an empty NextShardIterator once a Closed shard
// (from MergeShards/SplitShard) has been fully consumed, and ChildShards is
// populated "only when the end of the current shard is reached"
// (GetRecordsOutput's own doc comment) -- exactly that same condition.
func nextIteratorAndChildShards(
	shards []*Shard, shard *Shard, newIt *ShardIterator, actualEnd int,
) (string, []ChildShard, error) {
	if !shard.Closed || actualEnd < shard.Records.len() {
		nextToken, err := encodeIterator(newIt)
		if err != nil {
			return "", nil, err
		}

		return nextToken, nil, nil
	}

	return "", childShardsOf(shards, shard.ID), nil
}

// childShardsOf finds every shard directly descended from parentID (via
// ParentShardID or AdjacentParentShardID -- a merge child has both, a split
// child has only ParentShardID) and builds its real-AWS ChildShard entry,
// listing every parent that fed into it.
func childShardsOf(shards []*Shard, parentID string) []ChildShard {
	var children []ChildShard

	for _, s := range shards {
		if s.ParentShardID != parentID && s.AdjacentParentShardID != parentID {
			continue
		}

		var parents []string
		if s.ParentShardID != "" {
			parents = append(parents, s.ParentShardID)
		}
		if s.AdjacentParentShardID != "" {
			parents = append(parents, s.AdjacentParentShardID)
		}

		children = append(children, ChildShard{
			ShardID:           s.ID,
			HashKeyRangeStart: s.HashKeyRangeStart,
			HashKeyRangeEnd:   s.HashKeyRangeEnd,
			ParentShards:      parents,
		})
	}

	return children
}
