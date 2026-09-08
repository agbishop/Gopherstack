package dynamodb_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"
	streamstypes "github.com/aws/aws-sdk-go-v2/service/dynamodbstreams/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddb "github.com/blackbirdworks/gopherstack/services/dynamodb"
)

// newStreamsTestDB creates an InMemoryDB with a single test table for stream tests.
func newStreamsTestDB(t *testing.T) *ddb.InMemoryDB {
	t.Helper()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("StreamsTestTable", "pk"))
	require.NoError(t, err)

	return db
}

func TestUnit_Streams_EnableDisable(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	t.Run("enable streams", func(t *testing.T) {
		t.Parallel()

		err := db.EnableStream(ctx, "StreamsTestTable", "NEW_AND_OLD_IMAGES")
		require.NoError(t, err)

		table, ok := db.GetTable("StreamsTestTable")
		require.True(t, ok)
		require.True(t, table.StreamsEnabled)
		require.Equal(t, "NEW_AND_OLD_IMAGES", table.StreamViewType)
		require.NotEmpty(t, table.StreamARN)
	})

	t.Run("disable streams", func(t *testing.T) {
		t.Parallel()

		db2 := newStreamsTestDB(t)
		ctx2 := t.Context()

		require.NoError(t, db2.EnableStream(ctx2, "StreamsTestTable", "NEW_IMAGE"))
		require.NoError(t, db2.DisableStream(ctx2, "StreamsTestTable"))

		table, ok := db2.GetTable("StreamsTestTable")
		require.True(t, ok)
		require.False(t, table.StreamsEnabled)
		require.Empty(t, table.StreamARN)
		require.Empty(t, table.StreamRecords)
	})

	t.Run("enable on non-existent table", func(t *testing.T) {
		t.Parallel()

		db3 := newStreamsTestDB(t)
		err := db3.EnableStream(t.Context(), "NoSuchTable", "KEYS_ONLY")
		require.Error(t, err)
	})
}

func TestUnit_Streams_ListStreams(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("TableA", "id"))
	require.NoError(t, err)
	_, err = db.CreateTable(ctx, makeCreateTableInput("TableB", "id"))
	require.NoError(t, err)

	require.NoError(t, db.EnableStream(ctx, "TableA", "NEW_AND_OLD_IMAGES"))
	// TableB has no stream

	t.Run("list all streams", func(t *testing.T) {
		t.Parallel()

		out, listErr := db.ListStreams(ctx, &dynamodbstreams.ListStreamsInput{})
		require.NoError(t, listErr)
		require.Len(t, out.Streams, 1)
		require.Equal(t, "TableA", aws.ToString(out.Streams[0].TableName))
	})

	t.Run("filter by table name", func(t *testing.T) {
		t.Parallel()

		out, listErr := db.ListStreams(ctx, &dynamodbstreams.ListStreamsInput{
			TableName: aws.String("TableB"),
		})
		require.NoError(t, listErr)
		require.Empty(t, out.Streams)
	})
}

func TestUnit_Streams_DescribeStream(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_IMAGE"))

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)
	arn := table.StreamARN

	out, err := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn: aws.String(arn),
	})
	require.NoError(t, err)
	require.Equal(t, arn, aws.ToString(out.StreamDescription.StreamArn))
	require.Equal(t, streamstypes.StreamStatusEnabled, out.StreamDescription.StreamStatus)
	require.NotEmpty(t, out.StreamDescription.Shards)
}

func TestUnit_Streams_GetRecords(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_AND_OLD_IMAGES"))

	// PutItem → INSERT event
	_, err := db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "pk1"))
	require.NoError(t, err)

	// PutItem again → MODIFY event
	_, err = db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "pk1"))
	require.NoError(t, err)

	// DeleteItem → REMOVE event
	_, err = db.DeleteItem(ctx, makeDeleteItem("StreamsTestTable", "pk", "pk1"))
	require.NoError(t, err)

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)
	arn := table.StreamARN

	// Get iterator from trim-horizon
	iterOut, err := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         aws.String(arn),
		ShardId:           aws.String(ddb.StreamShardID),
		ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(iterOut.ShardIterator))

	// GetRecords — should get all 3 events
	recOut, err := db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
		ShardIterator: iterOut.ShardIterator,
	})
	require.NoError(t, err)
	require.Len(t, recOut.Records, 3)
	require.Equal(t, streamstypes.OperationTypeInsert, recOut.Records[0].EventName)
	require.Equal(t, streamstypes.OperationTypeModify, recOut.Records[1].EventName)
	require.Equal(t, streamstypes.OperationTypeRemove, recOut.Records[2].EventName)
}

func TestUnit_Streams_RingBufferCap(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("BufTable", "pk"))
	require.NoError(t, err)
	require.NoError(t, db.EnableStream(ctx, "BufTable", "KEYS_ONLY"))

	// Write more than maxStreamRecords items
	const writeCount = 1005
	for i := range writeCount {
		_, err = db.PutItem(ctx, makePutItemN("BufTable", i))
		require.NoError(t, err)
	}

	table, ok := db.GetTable("BufTable")
	require.True(t, ok)
	require.LessOrEqual(t, len(table.StreamRecords), 1000)
}

func TestUnit_Streams_ViewType_NewImage(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_IMAGE"))

	_, err := db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "x"))
	require.NoError(t, err)

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)
	require.Len(t, table.StreamRecords, 1)
	require.NotNil(t, table.StreamRecords[0].NewImage)
	require.Nil(t, table.StreamRecords[0].OldImage)
}

func TestUnit_Streams_ViewType_OldImage(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	// Need an existing item first so MODIFY produces an OldImage
	_, err := db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "x"))
	require.NoError(t, err)

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "OLD_IMAGE"))

	_, err = db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "x"))
	require.NoError(t, err)

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)

	// Should have only 1 event (after enabling)
	require.Len(t, table.StreamRecords, 1)
	require.Nil(t, table.StreamRecords[0].NewImage)
	require.NotNil(t, table.StreamRecords[0].OldImage)
}

func TestUnit_Streams_UnparamFix(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	// Use a different table name to satisfy unparam lint for makePutItem
	_, _ = db.CreateTable(ctx, makeCreateTableInput("OtherTableForLint", "id"))
	_ = makePutItem("OtherTableForLint", "id", "val")
}

func TestUnit_Streams_ComplexAttributeTypes(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("ComplexAttrTable", "pk"))
	require.NoError(t, err)
	require.NoError(t, db.EnableStream(ctx, "ComplexAttrTable", "NEW_AND_OLD_IMAGES"))

	// Insert an item with map, list, and set attributes so that buildSDKRecord
	// exercises handleMapAttribute, handleListAttribute, and toStringSliceFrom.
	_, err = db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String("ComplexAttrTable"),
		Item: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: "complex-pk"},
			"nested_map": &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{
				"inner": &types.AttributeValueMemberS{Value: "value"},
			}},
			"list_attr": &types.AttributeValueMemberL{Value: []types.AttributeValue{
				&types.AttributeValueMemberS{Value: "elem1"},
				&types.AttributeValueMemberN{Value: "42"},
			}},
			"string_set": &types.AttributeValueMemberSS{Value: []string{"a", "b", "c"}},
			"number_set": &types.AttributeValueMemberNS{Value: []string{"1", "2"}},
		},
	})
	require.NoError(t, err)

	// Read back via GetRecords to trigger buildSDKRecord -> handleMapAttribute/handleListAttribute.
	listOut, err := db.ListStreams(ctx, &dynamodbstreams.ListStreamsInput{
		TableName: aws.String("ComplexAttrTable"),
	})
	require.NoError(t, err)
	require.Len(t, listOut.Streams, 1)

	descOut, err := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn: listOut.Streams[0].StreamArn,
	})
	require.NoError(t, err)
	require.NotEmpty(t, descOut.StreamDescription.Shards)

	iterOut, err := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         listOut.Streams[0].StreamArn,
		ShardId:           descOut.StreamDescription.Shards[0].ShardId,
		ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
	})
	require.NoError(t, err)

	recordsOut, err := db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
		ShardIterator: iterOut.ShardIterator,
	})
	require.NoError(t, err)
	require.NotEmpty(t, recordsOut.Records)
	// The NewImage should contain the complex attributes.
	rec := recordsOut.Records[0]
	assert.NotNil(t, rec.Dynamodb.NewImage)
}

// TestUnit_Streams_TransactWriteEmitsRecords verifies that TransactWriteItems
// emits DynamoDB Streams records for Put/Update/Delete, matching real AWS.
func TestUnit_Streams_TransactWriteEmitsRecords(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()
	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_AND_OLD_IMAGES"))

	// Transactional Put → INSERT.
	_, err := db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{
				TableName: aws.String("StreamsTestTable"),
				Item: map[string]types.AttributeValue{
					"pk":  &types.AttributeValueMemberS{Value: "t1"},
					"val": &types.AttributeValueMemberS{Value: "a"},
				},
			}},
		},
	})
	require.NoError(t, err)

	// Transactional Delete → REMOVE.
	_, err = db.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Delete: &types.Delete{
				TableName: aws.String("StreamsTestTable"),
				Key: map[string]types.AttributeValue{
					"pk": &types.AttributeValueMemberS{Value: "t1"},
				},
			}},
		},
	})
	require.NoError(t, err)

	records := drainStreamRecords(t, db, "StreamsTestTable")
	require.Len(t, records, 2)
	require.Equal(t, streamstypes.OperationTypeInsert, records[0].EventName)
	require.Equal(t, streamstypes.OperationTypeRemove, records[1].EventName)
}

// TestUnit_Streams_BatchWriteEmitsRecords verifies that BatchWriteItem PutRequests
// emit INSERT/MODIFY stream records (deletes were already covered).
func TestUnit_Streams_BatchWriteEmitsRecords(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()
	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_AND_OLD_IMAGES"))

	put := func(val string) types.WriteRequest {
		return types.WriteRequest{PutRequest: &types.PutRequest{
			Item: map[string]types.AttributeValue{
				"pk":  &types.AttributeValueMemberS{Value: "b1"},
				"val": &types.AttributeValueMemberS{Value: val},
			},
		}}
	}

	_, err := db.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			"StreamsTestTable": {put("a")},
		},
	})
	require.NoError(t, err)

	_, err = db.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			"StreamsTestTable": {put("b")},
		},
	})
	require.NoError(t, err)

	records := drainStreamRecords(t, db, "StreamsTestTable")
	require.Len(t, records, 2)
	require.Equal(t, streamstypes.OperationTypeInsert, records[0].EventName)
	require.Equal(t, streamstypes.OperationTypeModify, records[1].EventName)
}

// drainStreamRecords reads all stream records for a table from the trim horizon.
func drainStreamRecords(t *testing.T, db *ddb.InMemoryDB, tableName string) []streamstypes.Record {
	t.Helper()

	table, ok := db.GetTable(tableName)
	require.True(t, ok)

	iterOut, err := db.GetShardIterator(t.Context(), &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         aws.String(table.StreamARN),
		ShardId:           aws.String(ddb.StreamShardID),
		ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
	})
	require.NoError(t, err)

	recOut, err := db.GetRecords(t.Context(), &dynamodbstreams.GetRecordsInput{
		ShardIterator: iterOut.ShardIterator,
	})
	require.NoError(t, err)

	return recOut.Records
}

func TestStreams_Shards_InitialShardOnEnable(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_IMAGE"))

	shards := db.StreamShards("StreamsTestTable")
	require.Len(t, shards, 1, "EnableStream must create exactly 1 initial shard")
	assert.Equal(t, ddb.StreamShardID, shards[0].ShardID)
	assert.Empty(t, shards[0].ParentShardID, "first shard must have no parent")
	assert.Equal(t, int64(0), shards[0].EndingSequenceNum, "first shard must be open")
}

func TestStreams_Shards_ShardSplitOnRingBufferWrap(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("SplitTable", "pk"))
	require.NoError(t, err)
	require.NoError(t, db.EnableStream(ctx, "SplitTable", "KEYS_ONLY"))

	// Write exactly maxStreamRecords (1000) items — buffer fills up but no wrap yet.
	for i := range 1000 {
		_, err = db.PutItem(ctx, makePutItemN("SplitTable", i))
		require.NoError(t, err)
	}

	// Buffer is now full. No split yet.
	shards := db.StreamShards("SplitTable")
	require.Len(t, shards, 1, "no shard split before ring buffer wraps")
	assert.Equal(t, int64(0), shards[0].EndingSequenceNum, "shard still open")

	// Write one more item — this is the first write to enter the else branch with StreamHead==0,
	// which triggers the first shard split.
	_, err = db.PutItem(ctx, makePutItemN("SplitTable", 1001))
	require.NoError(t, err)

	shards = db.StreamShards("SplitTable")
	require.GreaterOrEqual(t, len(shards), 2,
		"a shard split must have occurred after ring buffer completed a full rotation")

	// Verify genealogy.
	first := shards[0]
	second := shards[1]
	assert.Equal(t, ddb.StreamShardID, first.ShardID)
	assert.NotEqual(t, int64(0), first.EndingSequenceNum, "first shard must be closed after split")
	assert.Equal(
		t,
		first.ShardID,
		second.ParentShardID,
		"second shard's parent must be the first shard",
	)
	assert.Equal(t, int64(0), second.EndingSequenceNum, "second shard must still be open")
}

func TestStreams_GetRecords_ClosedShardReturnsNilIterator(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("ClosedShardTable", "pk"))
	require.NoError(t, err)
	require.NoError(t, db.EnableStream(ctx, "ClosedShardTable", "KEYS_ONLY"))

	// Force a shard split so the first shard becomes closed (has an EndingSequenceNumber).
	for i := range 1001 {
		_, err = db.PutItem(ctx, makePutItemN("ClosedShardTable", i))
		require.NoError(t, err)
	}

	shards := db.StreamShards("ClosedShardTable")
	require.GreaterOrEqual(t, len(shards), 2, "expected a shard split")
	require.NotEqual(t, int64(0), shards[0].EndingSequenceNum, "first shard must be closed")

	table, ok := db.GetTable("ClosedShardTable")
	require.True(t, ok)

	iterOut, err := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         aws.String(table.StreamARN),
		ShardId:           aws.String(shards[0].ShardID),
		ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
	})
	require.NoError(t, err)

	// Draining a closed shard must eventually yield a nil NextShardIterator so
	// consumers know to advance to the child shard.
	iter := iterOut.ShardIterator
	gotNil := false
	for range 5 {
		recOut, recErr := db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{ShardIterator: iter})
		require.NoError(t, recErr)
		if recOut.NextShardIterator == nil {
			gotNil = true

			break
		}
		iter = recOut.NextShardIterator
	}
	assert.True(
		t,
		gotNil,
		"GetRecords on a drained closed shard must return a nil NextShardIterator",
	)
}

func TestStreams_Shards_DescribeStreamReturnsGenealogy(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("GenTable", "pk"))
	require.NoError(t, err)
	require.NoError(t, db.EnableStream(ctx, "GenTable", "KEYS_ONLY"))

	// Force a shard split by writing 2001 items.
	for i := range 2001 {
		_, err = db.PutItem(ctx, makePutItemN("GenTable", i))
		require.NoError(t, err)
	}

	table, ok := db.GetTable("GenTable")
	require.True(t, ok)

	out, err := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn: aws.String(table.StreamARN),
	})
	require.NoError(t, err)

	shards := out.StreamDescription.Shards
	require.GreaterOrEqual(t, len(shards), 2,
		"DescribeStream must expose shard genealogy after a split")

	// First shard must have EndingSequenceNumber set (closed).
	require.NotNil(t, shards[0].SequenceNumberRange)
	assert.NotEmpty(t, aws.ToString(shards[0].SequenceNumberRange.EndingSequenceNumber),
		"closed shard must have EndingSequenceNumber set")

	// Second shard must reference the first as its parent.
	assert.Equal(t,
		aws.ToString(shards[0].ShardId),
		aws.ToString(shards[1].ParentShardId),
		"second shard must have ParentShardId pointing to first shard")
}

func TestStreams_Shards_DisableStreamClearsShards(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_IMAGE"))

	// Write some records.
	_, err := db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "x"))
	require.NoError(t, err)

	require.NoError(t, db.DisableStream(ctx, "StreamsTestTable"))

	shards := db.StreamShards("StreamsTestTable")
	assert.Empty(t, shards, "DisableStream must clear shard history")
	assert.Equal(t, int64(0), db.StreamTrimSeq("StreamsTestTable"),
		"DisableStream must reset trim horizon")
}

func TestStreams_Shards_ReEnableCreatesNewShard(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_IMAGE"))
	require.NoError(t, db.DisableStream(ctx, "StreamsTestTable"))
	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_IMAGE"))

	shards := db.StreamShards("StreamsTestTable")
	require.Len(t, shards, 1, "re-enabling stream must create a fresh first shard")
	assert.Equal(t, int64(0), shards[0].EndingSequenceNum, "new shard must be open")
}

func TestStreams_TrimHorizon_AdvancesWhenBufferWraps(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("TrimTable", "pk"))
	require.NoError(t, err)
	require.NoError(t, db.EnableStream(ctx, "TrimTable", "KEYS_ONLY"))

	// Buffer not full — trim horizon should be 0.
	for i := range 500 {
		_, err = db.PutItem(ctx, makePutItemN("TrimTable", i))
		require.NoError(t, err)
	}
	assert.Equal(t, int64(0), db.StreamTrimSeq("TrimTable"),
		"trim horizon must be 0 while buffer is not full")

	// Fill buffer to capacity.
	for i := 500; i < 1000; i++ {
		_, err = db.PutItem(ctx, makePutItemN("TrimTable", i))
		require.NoError(t, err)
	}
	assert.Equal(t, int64(0), db.StreamTrimSeq("TrimTable"),
		"trim horizon must still be 0 when buffer just hits capacity")

	// Write one more — buffer wraps, trim horizon advances.
	_, err = db.PutItem(ctx, makePutItemN("TrimTable", 1001))
	require.NoError(t, err)

	trimSeq := db.StreamTrimSeq("TrimTable")
	assert.Positive(t, trimSeq,
		"trim horizon must advance once the ring buffer starts overwriting records")
}

func TestStreams_TrimmedDataAccess_GetRecordsReturnsError(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("TrimErrTable", "pk"))
	require.NoError(t, err)
	require.NoError(t, db.EnableStream(ctx, "TrimErrTable", "KEYS_ONLY"))

	// Fill buffer and cause it to wrap — seq 1 will be evicted.
	for i := range 1002 {
		_, err = db.PutItem(ctx, makePutItemN("TrimErrTable", i))
		require.NoError(t, err)
	}

	trimSeq := db.StreamTrimSeq("TrimErrTable")
	require.Positive(t, trimSeq)

	table, ok := db.GetTable("TrimErrTable")
	require.True(t, ok)

	// Request an iterator pointing to a sequence that has been trimmed.
	trimmedSeqStr := fmt.Sprintf("%020d", trimSeq-1)
	_, iterErr := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         aws.String(table.StreamARN),
		ShardId:           aws.String(ddb.StreamShardID),
		ShardIteratorType: streamstypes.ShardIteratorTypeAtSequenceNumber,
		SequenceNumber:    aws.String(trimmedSeqStr),
	})
	require.Error(t, iterErr)
	assert.Contains(t, iterErr.Error(), "TrimmedDataAccessException",
		"requesting a trimmed sequence via GetShardIterator must return TrimmedDataAccessException")
}

func TestStreams_DescribeStream_Validation(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	tests := []struct {
		name            string
		streamArn       string
		wantErrContains string
	}{
		{
			name:            "missing StreamArn returns validation error",
			streamArn:       "",
			wantErrContains: "StreamArn",
		},
		{
			name:            "unknown stream ARN returns ResourceNotFoundException",
			streamArn:       "arn:aws:dynamodb:us-east-1:123456789012:table/NoSuch/stream/2024-01-01T00:00:00.000",
			wantErrContains: "stream not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inp := &dynamodbstreams.DescribeStreamInput{}
			if tt.streamArn != "" {
				inp.StreamArn = aws.String(tt.streamArn)
			}

			_, err := db.DescribeStream(ctx, inp)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrContains)
		})
	}
}

func TestStreams_DescribeStream_Pagination(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("PagTable", "pk"))
	require.NoError(t, err)
	require.NoError(t, db.EnableStream(ctx, "PagTable", "KEYS_ONLY"))

	// Create 2 shards by causing a ring-buffer wrap.
	for i := range 2001 {
		_, err = db.PutItem(ctx, makePutItemN("PagTable", i))
		require.NoError(t, err)
	}

	table, ok := db.GetTable("PagTable")
	require.True(t, ok)

	// Describe with Limit=1 to get only the first shard.
	limit := int32(1)
	out1, err := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn: aws.String(table.StreamARN),
		Limit:     &limit,
	})
	require.NoError(t, err)
	require.Len(t, out1.StreamDescription.Shards, 1)
	require.NotNil(t, out1.StreamDescription.LastEvaluatedShardId,
		"Limit=1 must set LastEvaluatedShardId when more shards exist")

	// Page 2: use ExclusiveStartShardId.
	lastShardID := aws.ToString(out1.StreamDescription.LastEvaluatedShardId)
	out2, err := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn:             aws.String(table.StreamARN),
		ExclusiveStartShardId: aws.String(lastShardID),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out2.StreamDescription.Shards,
		"second page must return the remaining shards")

	// Verify no overlap.
	firstPageIDs := make(map[string]bool)
	for _, s := range out1.StreamDescription.Shards {
		firstPageIDs[aws.ToString(s.ShardId)] = true
	}
	for _, s := range out2.StreamDescription.Shards {
		assert.False(t, firstPageIDs[aws.ToString(s.ShardId)],
			"second page must not overlap with first page")
	}
}

// TestStreams_DescribeStream_ShardFilter_ChildShards exercises the
// CHILD_SHARDS ShardFilter end-to-end against a real ring-buffer-induced
// shard split (mirrors TestStreams_DescribeStream_Pagination's setup): an
// unfiltered DescribeStream sees both shards, but filtering by the parent's
// ShardId must return only its child.
func TestStreams_DescribeStream_ShardFilter_ChildShards(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	_, err := db.CreateTable(ctx, makeCreateTableInput("ChildShardsTable", "pk"))
	require.NoError(t, err)
	require.NoError(t, db.EnableStream(ctx, "ChildShardsTable", "KEYS_ONLY"))

	// Force exactly one ring-buffer wrap (maxStreamRecords=1000) so the stream
	// has exactly a closed parent + open child shard.
	for i := range 1001 {
		_, err = db.PutItem(ctx, makePutItemN("ChildShardsTable", i))
		require.NoError(t, err)
	}

	table, ok := db.GetTable("ChildShardsTable")
	require.True(t, ok)

	unfiltered, err := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn: aws.String(table.StreamARN),
	})
	require.NoError(t, err)
	require.Len(t, unfiltered.StreamDescription.Shards, 2, "precondition: split produced 2 shards")

	parentID := aws.ToString(unfiltered.StreamDescription.Shards[0].ShardId)
	childID := aws.ToString(unfiltered.StreamDescription.Shards[1].ShardId)
	require.Equal(t, parentID, aws.ToString(unfiltered.StreamDescription.Shards[1].ParentShardId),
		"precondition: shard[1] must be the child of shard[0]")

	filtered, err := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn: aws.String(table.StreamARN),
		ShardFilter: &streamstypes.ShardFilter{
			Type:    streamstypes.ShardFilterTypeChildShards,
			ShardId: aws.String(parentID),
		},
	})
	require.NoError(t, err)
	require.Len(t, filtered.StreamDescription.Shards, 1,
		"CHILD_SHARDS filter must return only the parent's child shard")
	assert.Equal(t, childID, aws.ToString(filtered.StreamDescription.Shards[0].ShardId))

	// Filtering by the child (which has no children of its own) returns nothing.
	empty, err := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn: aws.String(table.StreamARN),
		ShardFilter: &streamstypes.ShardFilter{
			Type:    streamstypes.ShardFilterTypeChildShards,
			ShardId: aws.String(childID),
		},
	})
	require.NoError(t, err)
	assert.Empty(t, empty.StreamDescription.Shards)
}

// TestStreams_DescribeStream_ShardFilter_Validation lives in whitebox_test.go:
// it needs direct access to the unexported streamShards field to inject a
// deterministic shard genealogy without forcing a real ring-buffer split.

func TestStreams_DescribeStream_SequenceNumberRange(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_AND_OLD_IMAGES"))

	// Write a few records.
	for i := range 5 {
		_, err := db.PutItem(ctx, makePutItemN("StreamsTestTable", i))
		require.NoError(t, err)
	}

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)

	out, err := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
		StreamArn: aws.String(table.StreamARN),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.StreamDescription.Shards)

	shard := out.StreamDescription.Shards[0]
	require.NotNil(t, shard.SequenceNumberRange)
	assert.NotEmpty(t, aws.ToString(shard.SequenceNumberRange.StartingSequenceNumber),
		"open shard must have StartingSequenceNumber")
	// Open shard must NOT have EndingSequenceNumber.
	assert.Empty(t, aws.ToString(shard.SequenceNumberRange.EndingSequenceNumber),
		"open shard must not have EndingSequenceNumber")
}

func TestStreams_ListStreams_Pagination(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	// Create 3 tables with streams enabled.
	for i := range 3 {
		name := fmt.Sprintf("PageStream%d", i)
		_, err := db.CreateTable(ctx, makeCreateTableInput(name, "pk"))
		require.NoError(t, err)
		require.NoError(t, db.EnableStream(ctx, name, "KEYS_ONLY"))
	}

	// Request Limit=1.
	limit := int32(1)
	out1, err := db.ListStreams(ctx, &dynamodbstreams.ListStreamsInput{Limit: &limit})
	require.NoError(t, err)
	require.Len(t, out1.Streams, 1)
	require.NotNil(t, out1.LastEvaluatedStreamArn,
		"ListStreams must set LastEvaluatedStreamArn when limit is reached")

	// Page 2.
	out2, err := db.ListStreams(ctx, &dynamodbstreams.ListStreamsInput{
		Limit:                   &limit,
		ExclusiveStartStreamArn: out1.LastEvaluatedStreamArn,
	})
	require.NoError(t, err)
	require.Len(t, out2.Streams, 1)

	// Page 3.
	out3, err := db.ListStreams(ctx, &dynamodbstreams.ListStreamsInput{
		Limit:                   &limit,
		ExclusiveStartStreamArn: out2.LastEvaluatedStreamArn,
	})
	require.NoError(t, err)
	require.Len(t, out3.Streams, 1)
	assert.Nil(t, out3.LastEvaluatedStreamArn,
		"last page must not set LastEvaluatedStreamArn")

	// Verify no overlap across pages.
	allARNs := make(map[string]int)
	for _, s := range out1.Streams {
		allARNs[aws.ToString(s.StreamArn)]++
	}
	for _, s := range out2.Streams {
		allARNs[aws.ToString(s.StreamArn)]++
	}
	for _, s := range out3.Streams {
		allARNs[aws.ToString(s.StreamArn)]++
	}
	for arn, count := range allARNs {
		assert.Equal(t, 1, count, "ARN %s appeared on multiple pages", arn)
	}
}

func TestStreams_ListStreams_FilterByTableName(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	ctx := t.Context()

	for _, name := range []string{"FilterA", "FilterB", "FilterC"} {
		_, err := db.CreateTable(ctx, makeCreateTableInput(name, "pk"))
		require.NoError(t, err)
		require.NoError(t, db.EnableStream(ctx, name, "KEYS_ONLY"))
	}

	out, err := db.ListStreams(ctx, &dynamodbstreams.ListStreamsInput{
		TableName: aws.String("FilterB"),
	})
	require.NoError(t, err)
	require.Len(t, out.Streams, 1)
	assert.Equal(t, "FilterB", aws.ToString(out.Streams[0].TableName))
}

func TestStreams_GetRecords_Limit(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "KEYS_ONLY"))

	for i := range 10 {
		_, err := db.PutItem(ctx, makePutItemN("StreamsTestTable", i))
		require.NoError(t, err)
	}

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)

	iterOut, err := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         aws.String(table.StreamARN),
		ShardId:           aws.String(ddb.StreamShardID),
		ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
	})
	require.NoError(t, err)

	limit := int32(3)
	recOut, err := db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
		ShardIterator: iterOut.ShardIterator,
		Limit:         &limit,
	})
	require.NoError(t, err)
	assert.Len(t, recOut.Records, 3, "GetRecords must honor the Limit parameter")
}

// TestStreams_GetRecords_LimitExceeded verifies AWS's documented behavior:
// "GetRecords was called with a value of more than 1000 for the limit request
// parameter" returns LimitExceededException instead of silently clamping.
func TestStreams_GetRecords_LimitExceeded(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "KEYS_ONLY"))

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)

	iterOut, err := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         aws.String(table.StreamARN),
		ShardId:           aws.String(ddb.StreamShardID),
		ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
	})
	require.NoError(t, err)

	limit := int32(1001)
	_, err = db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
		ShardIterator: iterOut.ShardIterator,
		Limit:         &limit,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LimitExceededException",
		"GetRecords with Limit > 1000 must return LimitExceededException")
}

func TestStreams_GetRecords_EmptyStream(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "KEYS_ONLY"))

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)

	iterOut, err := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         aws.String(table.StreamARN),
		ShardId:           aws.String(ddb.StreamShardID),
		ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
	})
	require.NoError(t, err)

	recOut, err := db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
		ShardIterator: iterOut.ShardIterator,
	})
	require.NoError(t, err)
	assert.Empty(t, recOut.Records, "empty stream must return zero records")
	assert.NotEmpty(t, aws.ToString(recOut.NextShardIterator),
		"empty stream must still return a valid NextShardIterator")
}

func TestStreams_GetRecords_MissingShardIterator(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	_, err := db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ShardIterator",
		"GetRecords without ShardIterator must return a validation error")
}

func TestStreams_GetRecords_InvalidToken(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	_, err := db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
		ShardIterator: aws.String("completelygarbagetoken"),
	})
	require.Error(t, err)
}

func TestStreams_GetRecords_SequenceContinuation(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "KEYS_ONLY"))

	for i := range 6 {
		_, err := db.PutItem(ctx, makePutItemN("StreamsTestTable", i))
		require.NoError(t, err)
	}

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)

	// Read 3 at a time and verify we get all 6 with no overlap.
	iterOut, err := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         aws.String(table.StreamARN),
		ShardId:           aws.String(ddb.StreamShardID),
		ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
	})
	require.NoError(t, err)

	limit := int32(3)
	seen := make(map[string]bool)

	for range 2 {
		rOut, rErr := db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
			ShardIterator: iterOut.ShardIterator,
			Limit:         &limit,
		})
		require.NoError(t, rErr)
		require.Len(t, rOut.Records, 3)
		for _, r := range rOut.Records {
			seq := aws.ToString(r.Dynamodb.SequenceNumber)
			assert.False(t, seen[seq], "duplicate sequence number: %s", seq)
			seen[seq] = true
		}
		iterOut.ShardIterator = rOut.NextShardIterator
	}

	assert.Len(t, seen, 6, "must have read exactly 6 distinct records")
}

func TestStreams_SeqNum_ZeroPadded(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "KEYS_ONLY"))

	_, err := db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "x"))
	require.NoError(t, err)

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)
	require.Len(t, table.StreamRecords, 1)

	seq := table.StreamRecords[0].SequenceNumber
	assert.Len(t, seq, 20, "sequence number must be exactly 20 digits (zero-padded)")
	assert.True(t, strings.HasPrefix(seq, "0"), "sequence number must be zero-padded")
}

func TestStreams_SeqNum_ParseRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantStr string
		input   int64
	}{
		{name: "zero", input: 0, wantStr: ""},
		{name: "one", input: 1, wantStr: "00000000000000000001"},
		{name: "max 20-digit", input: 99999999999999999, wantStr: "00000000099999999999999999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.input <= 0 {
				result := ddb.SeqNumString(tt.input)
				assert.Empty(t, result)

				return
			}

			s := ddb.SeqNumString(tt.input)
			assert.NotEmpty(t, s)

			parsed, err := ddb.ParseSeqNum(s)
			require.NoError(t, err)
			assert.Equal(t, tt.input, parsed)
		})
	}
}

func TestStreams_ViewType_KeysOnly(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "KEYS_ONLY"))

	_, err := db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "kval"))
	require.NoError(t, err)

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)
	require.Len(t, table.StreamRecords, 1)

	rec := table.StreamRecords[0]
	assert.NotNil(t, rec.Keys, "KEYS_ONLY must include Keys")
	assert.Nil(t, rec.NewImage, "KEYS_ONLY must not include NewImage")
	assert.Nil(t, rec.OldImage, "KEYS_ONLY must not include OldImage")
}

func TestStreams_ViewType_NewAndOldImages_IncludesBothImages(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_AND_OLD_IMAGES"))

	// First write — no old image.
	_, err := db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "val1"))
	require.NoError(t, err)

	// Second write — overwrites item, so old image should be present.
	_, err = db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "val1"))
	require.NoError(t, err)

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)
	require.Len(t, table.StreamRecords, 2)

	insert := table.StreamRecords[0]
	modify := table.StreamRecords[1]

	assert.NotNil(t, insert.NewImage, "INSERT must have NewImage")
	assert.Nil(t, insert.OldImage, "INSERT must not have OldImage")

	assert.NotNil(t, modify.NewImage, "MODIFY must have NewImage")
	assert.NotNil(t, modify.OldImage, "MODIFY must have OldImage")
}

func TestStreams_EventTypes_CRUDSequence(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "KEYS_ONLY"))

	_, err := db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "e1"))
	require.NoError(t, err)

	_, err = db.PutItem(ctx, makePutItem("StreamsTestTable", "pk", "e1"))
	require.NoError(t, err)

	_, err = db.DeleteItem(ctx, makeDeleteItem("StreamsTestTable", "pk", "e1"))
	require.NoError(t, err)

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)
	require.Len(t, table.StreamRecords, 3)

	assert.Equal(t, "INSERT", table.StreamRecords[0].EventName)
	assert.Equal(t, "MODIFY", table.StreamRecords[1].EventName)
	assert.Equal(t, "REMOVE", table.StreamRecords[2].EventName)
}

func TestStreams_GetRecentEvents(t *testing.T) {
	t.Parallel()

	db := newStreamsTestDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_IMAGE"))

	for i := range 3 {
		_, err := db.PutItem(ctx, makePutItemN("StreamsTestTable", i))
		require.NoError(t, err)
	}

	events := db.GetRecentEvents("StreamsTestTable")
	assert.Len(t, events, 3)
}

func TestStreams_GetRecentEvents_UnknownTable(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	events := db.GetRecentEvents("NoSuchTable")
	assert.Nil(t, events)
}
