package main

import (
	"log/slog"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	sdkddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	ddbbackend "github.com/blackbirdworks/gopherstack/services/dynamodb"
	kinesisbackend "github.com/blackbirdworks/gopherstack/services/kinesis"
)

// TestInitializeServices_DynamoDBKinesisWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) rather than invoking
// wireDynamoDBKinesis directly, so that deleting the wiring call from
// wireStorageAndSecretsIntegrations -- not just breaking the helper function itself -- is what
// this test is sensitive to. Same shape as cli_firehose_kinesis_wiring_test.go.
//
// Regression test for gopherstack-eouu: a table with EnableKinesisStreamingDestination
// configured must actually forward mutation records to the destination Kinesis stream, not
// silently accept the destination (reporting ACTIVE with a real stream ARN) and never deliver
// anything. Before the fix, SetKinesisEmitter was never called from cli.go, so
// db.kinesisEmitter stayed nil in a real running server and this test's Kinesis stream stayed
// empty forever.
func TestInitializeServices_DynamoDBKinesisWiring(t *testing.T) {
	t.Parallel()

	cli := &CLI{AccountID: "000000000000", Region: "us-east-1"}
	appCtx := &service.AppContext{
		Logger:     slog.Default(),
		Config:     cli,
		JanitorCtx: t.Context(),
	}
	cli.faultStore = chaos.NewFaultStore()

	services, err := initializeServices(appCtx)
	require.NoError(t, err)

	byName := serviceByName(services)

	ddbH, ok := byName["DynamoDB"].(*ddbbackend.DynamoDBHandler)
	require.True(t, ok, "DynamoDB handler must be registered")

	ddbBk, ok := ddbH.Backend.(*ddbbackend.InMemoryDB)
	require.True(t, ok, "DynamoDB backend must be an InMemoryDB")

	kinesisH, ok := byName["Kinesis"].(*kinesisbackend.Handler)
	require.True(t, ok, "Kinesis handler must be registered")

	kinesisBk, ok := kinesisH.Backend.(*kinesisbackend.InMemoryBackend)
	require.True(t, ok, "Kinesis backend must be an InMemoryBackend")

	ctx := t.Context()

	streamName := "dynamodb-kinesis-wiring-stream"
	require.NoError(t, kinesisBk.CreateStream(ctx, &kinesisbackend.CreateStreamInput{
		StreamName: streamName,
		ShardCount: 1,
	}))

	tableName := "dynamodb-kinesis-wiring-table"
	_, err = ddbBk.CreateTable(ctx, &sdkddb.CreateTableInput{
		TableName: aws.String(tableName),
		KeySchema: []sdkddbtypes.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: sdkddbtypes.KeyTypeHash},
		},
		AttributeDefinitions: []sdkddbtypes.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: sdkddbtypes.ScalarAttributeTypeS},
		},
		BillingMode: sdkddbtypes.BillingModePayPerRequest,
		StreamSpecification: &sdkddbtypes.StreamSpecification{
			StreamEnabled:  aws.Bool(true),
			StreamViewType: sdkddbtypes.StreamViewTypeNewAndOldImages,
		},
	})
	require.NoError(t, err)

	streamARN := "arn:aws:kinesis:us-east-1:000000000000:stream/" + streamName
	_, err = ddbBk.EnableKinesisStreamingDestination(ctx, &sdkddb.EnableKinesisStreamingDestinationInput{
		TableName: aws.String(tableName),
		StreamArn: aws.String(streamARN),
	})
	require.NoError(t, err)

	itemValue := "dynamodb-kinesis-wiring-payload"
	_, err = ddbBk.PutItem(ctx, &sdkddb.PutItemInput{
		TableName: aws.String(tableName),
		Item: map[string]sdkddbtypes.AttributeValue{
			"pk":      &sdkddbtypes.AttributeValueMemberS{Value: "item-1"},
			"payload": &sdkddbtypes.AttributeValueMemberS{Value: itemValue},
		},
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return kinesisStreamContains(t, kinesisBk, streamName, itemValue)
	}, 10*time.Second, 100*time.Millisecond,
		"a table mutation must reach its Kinesis streaming destination through the actual "+
			"cli.go composition root's wiring (wireDynamoDBKinesis), not just wired via the "+
			"helper called directly")
}
