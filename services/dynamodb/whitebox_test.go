package dynamodb

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	sdktypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"
	streamstypes "github.com/aws/aws-sdk-go-v2/service/dynamodbstreams/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"
)

// newWhiteboxStreamsDB creates an InMemoryDB with a single test table for stream tests.
func newWhiteboxStreamsDB(t *testing.T) *InMemoryDB {
	t.Helper()

	db := NewInMemoryDB()
	ctx := t.Context()

	rc, wc := int64(5), int64(5)
	_, err := db.CreateTable(ctx, &sdkdynamodb.CreateTableInput{
		TableName: aws.String("StreamsTestTable"),
		KeySchema: []sdktypes.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: sdktypes.KeyTypeHash},
		},
		AttributeDefinitions: []sdktypes.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: sdktypes.ScalarAttributeTypeS},
		},
		ProvisionedThroughput: &sdktypes.ProvisionedThroughput{
			ReadCapacityUnits:  &rc,
			WriteCapacityUnits: &wc,
		},
	})
	require.NoError(t, err)

	return db
}

func setStreamShards(db *InMemoryDB, tableName string, shards []StreamShard) {
	db.mu.RLock("test.setStreamShards")
	tbl, _ := db.tables.Get(tableKey(db.defaultRegion, tableName))
	db.mu.RUnlock()

	if tbl == nil {
		return
	}

	tbl.mu.Lock("test.setStreamShards.table")
	defer tbl.mu.Unlock()

	tbl.StreamShards = shards
}

// TestStreams_DescribeStream_ShardFilter_Validation covers the ShardFilter
// input-validation branches (unsupported Type, missing ShardId) using a
// deterministic 2-shard genealogy injected directly, avoiding the cost of
// forcing a real ring-buffer split for every case.
func TestStreams_DescribeStream_ShardFilter_Validation(t *testing.T) {
	t.Parallel()

	db := newWhiteboxStreamsDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "KEYS_ONLY"))

	setStreamShards(db, "StreamsTestTable", []StreamShard{
		{ShardID: "shardId-parent", StartingSequenceNum: 1, EndingSequenceNum: 5},
		{ShardID: "shardId-child", ParentShardID: "shardId-parent", StartingSequenceNum: 6},
	})

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)

	tests := []struct {
		name            string
		filter          *streamstypes.ShardFilter
		wantErrContains string
		wantShardIDs    []string
	}{
		{
			name:         "nil filter returns all shards",
			filter:       nil,
			wantShardIDs: []string{"shardId-parent", "shardId-child"},
		},
		{
			name: "CHILD_SHARDS with matching parent returns only child",
			filter: &streamstypes.ShardFilter{
				Type:    streamstypes.ShardFilterTypeChildShards,
				ShardId: aws.String("shardId-parent"),
			},
			wantShardIDs: []string{"shardId-child"},
		},
		{
			name: "unsupported filter type is rejected",
			filter: &streamstypes.ShardFilter{
				Type:    "BOGUS_TYPE",
				ShardId: aws.String("shardId-parent"),
			},
			wantErrContains: "Invalid ShardFilter Type",
		},
		{
			name: "CHILD_SHARDS without ShardId is rejected",
			filter: &streamstypes.ShardFilter{
				Type: streamstypes.ShardFilterTypeChildShards,
			},
			wantErrContains: "ShardFilter.ShardId is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, descErr := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
				StreamArn:   aws.String(table.StreamARN),
				ShardFilter: tt.filter,
			})

			if tt.wantErrContains != "" {
				require.Error(t, descErr)
				assert.Contains(t, descErr.Error(), tt.wantErrContains)

				return
			}

			require.NoError(t, descErr)

			gotIDs := make([]string, 0, len(out.StreamDescription.Shards))
			for _, s := range out.StreamDescription.Shards {
				gotIDs = append(gotIDs, aws.ToString(s.ShardId))
			}
			assert.ElementsMatch(t, tt.wantShardIDs, gotIDs)
		})
	}
}

// TestStreams_GetRecords_ExpiredIteratorException locks the previously
// untestable ExpiredIteratorException path end-to-end: obtain a real shard
// iterator via GetShardIterator, advance the backend's injected clock past
// the 15-minute TTL, then confirm GetRecords rejects the now-expired token
// with ExpiredIteratorException (not a generic ValidationException).
func TestStreams_GetRecords_ExpiredIteratorException(t *testing.T) {
	t.Parallel()

	db := newWhiteboxStreamsDB(t)
	ctx := t.Context()

	fakeNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	db.iteratorStore.SetClock(func() time.Time { return fakeNow })

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "NEW_AND_OLD_IMAGES"))

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)

	iterOut, err := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         aws.String(table.StreamARN),
		ShardId:           aws.String(StreamShardID),
		ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
	})
	require.NoError(t, err)

	// Advance the fake clock past the 15-minute shard-iterator TTL.
	fakeNow = fakeNow.Add(15*time.Minute + time.Second)

	_, err = db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
		ShardIterator: iterOut.ShardIterator,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")

	var apiErr *Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "com.amazonaws.dynamodb.v20120810#ExpiredIteratorException", apiErr.Type)
}

// TestStreams_JanitorSweep_PreservesRecordsUnder24Hours is a regression guard:
// AWS's TrimmedDataAccessException doc says records are only subject to
// removal once their age "exceeds" the 24 hour retention limit
// (aws-sdk-go-v2/service/dynamodbstreams/types/errors.go). A prior version of
// sweepTableStreamRecordsLocked, once >=50% of the ring was tombstoned,
// discarded the *entire* ring buffer and set streamTrimSeq past every
// record -- including ones still under 24h old -- making them wrongly
// unreadable via GetRecords.
func TestStreams_JanitorSweep_PreservesRecordsUnder24Hours(t *testing.T) {
	t.Parallel()

	db := newWhiteboxStreamsDB(t)
	ctx := t.Context()

	require.NoError(t, db.EnableStream(ctx, "StreamsTestTable", "KEYS_ONLY"))

	table, ok := db.GetTable("StreamsTestTable")
	require.True(t, ok)

	now := time.Now()
	old := now.Add(-25 * time.Hour).Unix()

	table.mu.Lock("test-setup")
	for i := range 3 {
		table.streamSeq++
		table.StreamRecords = append(table.StreamRecords, models.StreamRecord{
			EventID:                     fmt.Sprintf("old-%d", i),
			SequenceNumber:              seqNumString(table.streamSeq),
			ApproximateCreationDateTime: old,
		})
	}
	table.streamSeq++
	freshSeq := table.streamSeq
	table.StreamRecords = append(table.StreamRecords, models.StreamRecord{
		EventID:                     "fresh",
		SequenceNumber:              seqNumString(freshSeq),
		ApproximateCreationDateTime: now.Unix(),
	})
	table.mu.Unlock()

	j := NewJanitor(db, Settings{JanitorInterval: time.Hour})
	j.SweepOnce(ctx)

	iterOut, err := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
		StreamArn:         aws.String(table.StreamARN),
		ShardId:           aws.String(StreamShardID),
		ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
	})
	require.NoError(t, err)

	recOut, err := db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
		ShardIterator: iterOut.ShardIterator,
	})
	require.NoError(t, err)
	require.Len(t, recOut.Records, 1, "sweep must not discard the record still under 24h old")
	assert.Equal(t, "fresh", aws.ToString(recOut.Records[0].EventID))
}
