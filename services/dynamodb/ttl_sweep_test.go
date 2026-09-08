package dynamodb_test

import (
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodb_sdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"
	streamstypes "github.com/aws/aws-sdk-go-v2/service/dynamodbstreams/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dynamodb"
)

func TestJanitor_TTLSweep(t *testing.T) {
	t.Parallel()

	now := time.Now().Unix()

	tests := []struct {
		name          string
		ttlAttr       string
		items         []map[string]types.AttributeValue
		wantCount     int
		wantEvictions int
	}{
		{
			name:    "EvictExpiredItems",
			ttlAttr: "expires",
			items: []map[string]types.AttributeValue{
				{
					"pk":      &types.AttributeValueMemberS{Value: "past"},
					"expires": &types.AttributeValueMemberN{Value: "-100"},
				},
				{
					"pk":      &types.AttributeValueMemberS{Value: "future"},
					"expires": &types.AttributeValueMemberN{Value: "10000000000"},
				},
			},
			wantCount:     1,
			wantEvictions: 1,
		},
		{
			name:    "NoTTLenabled",
			ttlAttr: "",
			items: []map[string]types.AttributeValue{
				{
					"pk":      &types.AttributeValueMemberS{Value: "item1"},
					"expires": &types.AttributeValueMemberN{Value: "1"},
				},
			},
			wantCount:     1,
			wantEvictions: 0,
		},
		{
			name:    "MissingAttribute",
			ttlAttr: "expires",
			items: []map[string]types.AttributeValue{
				{
					"pk": &types.AttributeValueMemberS{Value: "no-ttl"},
				},
			},
			wantCount:     1,
			wantEvictions: 0,
		},
		{
			name:    "InvalidAttributeType",
			ttlAttr: "expires",
			items: []map[string]types.AttributeValue{
				{
					"pk":      &types.AttributeValueMemberS{Value: "bad-type"},
					"expires": &types.AttributeValueMemberS{Value: "not-a-number"},
				},
			},
			wantCount:     1,
			wantEvictions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := dynamodb.NewInMemoryDB()
			ctx := t.Context()

			// Create table
			tableName := "Table_" + tt.name
			_, err := db.CreateTable(ctx, &dynamodb_sdk.CreateTableInput{
				TableName: aws.String(tableName),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
				},
				AttributeDefinitions: []types.AttributeDefinition{
					{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
				},
			})
			require.NoError(t, err)

			// Enable TTL if specified
			if tt.ttlAttr != "" {
				_, err = db.UpdateTimeToLive(ctx, &dynamodb_sdk.UpdateTimeToLiveInput{
					TableName: aws.String(tableName),
					TimeToLiveSpecification: &types.TimeToLiveSpecification{
						AttributeName: aws.String(tt.ttlAttr),
						Enabled:       aws.Bool(true),
					},
				})
				require.NoError(t, err)
			}

			// Add items
			for _, item := range tt.items {
				if tt.ttlAttr != "" {
					if v, ok := item[tt.ttlAttr].(*types.AttributeValueMemberN); ok {
						if v.Value == "-100" {
							v.Value = strconv.FormatInt(now-100, 10)
						}
					}
				}

				_, err = db.PutItem(ctx, &dynamodb_sdk.PutItemInput{
					TableName: aws.String(tableName),
					Item:      item,
				})
				require.NoError(t, err)
			}

			// Trigger sweep via exported helper
			j := dynamodb.NewJanitor(db, dynamodb.Settings{JanitorInterval: time.Hour})
			j.SweepTTL(ctx)

			// Verify results
			scan, err := db.Scan(ctx, &dynamodb_sdk.ScanInput{
				TableName: aws.String(tableName),
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, int(scan.Count), "Item count mismatch for %s", tt.name)
		})
	}
}

// TestJanitor_TTLGracePeriod_DelaysEviction verifies that the TTL sweep
// honours dynamodb.TTLGracePeriod: an item past its TTL survives sweeps run
// within the grace window and is only evicted once the grace period elapses.
//
// Not parallel: mutates the package-level dynamodb.TTLGracePeriod, which
// must not race with other tests relying on its zero-value default.
//
//nolint:paralleltest // mutates a package-level global, see comment above
func TestJanitor_TTLGracePeriod_DelaysEviction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		orig := dynamodb.TTLGracePeriod
		//nolint:reassign // deliberately overriding the test-configurable grace period
		dynamodb.TTLGracePeriod = 5 * time.Minute
		defer func() {
			//nolint:reassign // restoring the original value
			dynamodb.TTLGracePeriod = orig
		}()

		db := dynamodb.NewInMemoryDB()
		ctx := t.Context()

		const tableName = "GraceTable"

		_, err := db.CreateTable(ctx, &dynamodb_sdk.CreateTableInput{
			TableName: aws.String(tableName),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
			},
			AttributeDefinitions: []types.AttributeDefinition{
				{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
			},
		})
		require.NoError(t, err)

		_, err = db.UpdateTimeToLive(ctx, &dynamodb_sdk.UpdateTimeToLiveInput{
			TableName: aws.String(tableName),
			TimeToLiveSpecification: &types.TimeToLiveSpecification{
				AttributeName: aws.String("expires"),
				Enabled:       aws.Bool(true),
			},
		})
		require.NoError(t, err)

		expiredAt := time.Now().Add(-time.Second).Unix()
		_, err = db.PutItem(ctx, &dynamodb_sdk.PutItemInput{
			TableName: aws.String(tableName),
			Item: map[string]types.AttributeValue{
				"pk":      &types.AttributeValueMemberS{Value: "item-1"},
				"expires": &types.AttributeValueMemberN{Value: strconv.FormatInt(expiredAt, 10)},
			},
		})
		require.NoError(t, err)

		j := dynamodb.NewJanitor(db, dynamodb.Settings{JanitorInterval: time.Hour})

		table, ok := db.GetTable(tableName)
		require.True(t, ok)

		j.SweepTTL(ctx)
		assert.Len(t, table.GetItems(), 1, "item should survive a sweep run within the grace period")

		time.Sleep(dynamodb.TTLGracePeriod + time.Minute)
		synctest.Wait()

		j.SweepTTL(ctx)
		assert.Empty(t, table.GetItems(), "item should be evicted once the grace period elapses")
	})
}

func TestJanitor_TTLSweep_StreamRecords(t *testing.T) {
	t.Parallel()

	now := time.Now().Unix()

	tests := []struct {
		name              string
		streamViewType    string
		expiredPK         string
		activePK          string
		wantRemoveEventPK string
		wantRecordCount   int
	}{
		{
			name:              "EmitsRemoveRecordForExpiredItem_NewAndOldImages",
			streamViewType:    "NEW_AND_OLD_IMAGES",
			expiredPK:         "expired-item",
			activePK:          "active-item",
			wantRecordCount:   1,
			wantRemoveEventPK: "expired-item",
		},
		{
			name:              "EmitsRemoveRecordForExpiredItem_OldImageOnly",
			streamViewType:    "OLD_IMAGE",
			expiredPK:         "expired-item",
			activePK:          "active-item",
			wantRecordCount:   1,
			wantRemoveEventPK: "expired-item",
		},
		{
			name:              "NoStreamRecord_WhenNoExpiry",
			streamViewType:    "NEW_AND_OLD_IMAGES",
			expiredPK:         "",
			activePK:          "active-item",
			wantRecordCount:   0,
			wantRemoveEventPK: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			db := dynamodb.NewInMemoryDB()
			tableName := "TTLStreamTable_" + tt.name

			// Create table with streams enabled.
			_, err := db.CreateTable(ctx, &dynamodb_sdk.CreateTableInput{
				TableName: aws.String(tableName),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
				},
				AttributeDefinitions: []types.AttributeDefinition{
					{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
				},
				StreamSpecification: &types.StreamSpecification{
					StreamEnabled:  aws.Bool(true),
					StreamViewType: types.StreamViewType(tt.streamViewType),
				},
			})
			require.NoError(t, err)

			// Enable TTL.
			_, err = db.UpdateTimeToLive(ctx, &dynamodb_sdk.UpdateTimeToLiveInput{
				TableName: aws.String(tableName),
				TimeToLiveSpecification: &types.TimeToLiveSpecification{
					AttributeName: aws.String("expires"),
					Enabled:       aws.Bool(true),
				},
			})
			require.NoError(t, err)

			// Insert an expired item if configured.
			if tt.expiredPK != "" {
				_, err = db.PutItem(ctx, &dynamodb_sdk.PutItemInput{
					TableName: aws.String(tableName),
					Item: map[string]types.AttributeValue{
						"pk": &types.AttributeValueMemberS{Value: tt.expiredPK},
						"expires": &types.AttributeValueMemberN{
							Value: strconv.FormatInt(now-100, 10),
						},
					},
				})
				require.NoError(t, err)
			}

			// Insert a non-expired item.
			_, err = db.PutItem(ctx, &dynamodb_sdk.PutItemInput{
				TableName: aws.String(tableName),
				Item: map[string]types.AttributeValue{
					"pk":      &types.AttributeValueMemberS{Value: tt.activePK},
					"expires": &types.AttributeValueMemberN{Value: strconv.FormatInt(now+3600, 10)},
				},
			})
			require.NoError(t, err)

			// Locate the stream.
			listOut, err := db.ListStreams(ctx, &dynamodbstreams.ListStreamsInput{
				TableName: aws.String(tableName),
			})
			require.NoError(t, err)
			require.Len(t, listOut.Streams, 1)
			streamARN := listOut.Streams[0].StreamArn

			descOut, err := db.DescribeStream(ctx, &dynamodbstreams.DescribeStreamInput{
				StreamArn: streamARN,
			})
			require.NoError(t, err)
			require.NotEmpty(t, descOut.StreamDescription.Shards)
			shardID := descOut.StreamDescription.Shards[0].ShardId

			// Trigger the TTL sweep.
			j := dynamodb.NewJanitor(db, dynamodb.Settings{JanitorInterval: time.Hour})
			j.SweepTTL(ctx)

			// Read all records from the beginning and filter for REMOVE events.
			iterOut, err := db.GetShardIterator(ctx, &dynamodbstreams.GetShardIteratorInput{
				StreamArn:         streamARN,
				ShardId:           shardID,
				ShardIteratorType: streamstypes.ShardIteratorTypeTrimHorizon,
			})
			require.NoError(t, err)

			recordsOut, err := db.GetRecords(ctx, &dynamodbstreams.GetRecordsInput{
				ShardIterator: iterOut.ShardIterator,
			})
			require.NoError(t, err)

			// Count REMOVE events only; INSERT events from PutItem calls are ignored.
			var removeRecords []streamstypes.Record
			for _, r := range recordsOut.Records {
				if r.EventName == streamstypes.OperationTypeRemove {
					removeRecords = append(removeRecords, r)
				}
			}

			assert.Len(t, removeRecords, tt.wantRecordCount, "unexpected REMOVE record count")

			if tt.wantRemoveEventPK != "" && len(removeRecords) > 0 {
				rec := removeRecords[0]
				assert.Equal(t, streamstypes.OperationTypeRemove, rec.EventName)

				switch tt.streamViewType {
				case "NEW_AND_OLD_IMAGES", "OLD_IMAGE":
					require.NotNil(
						t,
						rec.Dynamodb.OldImage,
						"OldImage should be present for view type %s",
						tt.streamViewType,
					)
					assert.Equal(
						t,
						tt.wantRemoveEventPK,
						rec.Dynamodb.OldImage["pk"].(*streamstypes.AttributeValueMemberS).Value,
					)
					assert.Nil(t, rec.Dynamodb.NewImage, "NewImage should be nil for REMOVE event")
				}
			}
		})
	}
}
