package dynamodb_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbsdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dynamodb"
)

func wireTestKeySchema() ([]dynamodbtypes.KeySchemaElement, []dynamodbtypes.AttributeDefinition) {
	return []dynamodbtypes.KeySchemaElement{
		{AttributeName: aws.String("id"), KeyType: dynamodbtypes.KeyTypeHash},
	}, []dynamodbtypes.AttributeDefinition{
		{AttributeName: aws.String("id"), AttributeType: dynamodbtypes.ScalarAttributeTypeS},
	}
}

// TestCreateTable_SSESpecification_SurvivesWireConversion verifies that
// CreateTable's SSESpecification reaches the backend and is reflected in the
// response's SSEDescription. models.CreateTableInput previously had no
// SSESpecification field at all, so a client's KMS encryption request was
// silently dropped and the table was always created with default (disabled)
// encryption regardless of what was requested.
func TestCreateTable_SSESpecification_SurvivesWireConversion(t *testing.T) {
	t.Parallel()

	client := newTestDynamoDBClient(t, dynamodb.NewHandler(dynamodb.NewInMemoryDB()))
	keySchema, attrDefs := wireTestKeySchema()

	out, err := client.CreateTable(t.Context(), &dynamodbsdk.CreateTableInput{
		TableName:            aws.String("sse-table"),
		KeySchema:            keySchema,
		AttributeDefinitions: attrDefs,
		BillingMode:          dynamodbtypes.BillingModePayPerRequest,
		SSESpecification: &dynamodbtypes.SSESpecification{
			Enabled:        aws.Bool(true),
			SSEType:        dynamodbtypes.SSETypeKms,
			KMSMasterKeyId: aws.String("arn:aws:kms:us-east-1:000000000000:key/wire-test-key"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out.TableDescription.SSEDescription)

	assert.Equal(t, dynamodbtypes.SSETypeKms, out.TableDescription.SSEDescription.SSEType)
	assert.Equal(
		t,
		"arn:aws:kms:us-east-1:000000000000:key/wire-test-key",
		aws.ToString(out.TableDescription.SSEDescription.KMSMasterKeyArn),
	)
}

// TestCreateTable_TableId_ReturnedOnCreate verifies that CreateTable's own
// response includes TableId. buildCreateTableOutput assigns t.TableID at
// creation (used by DescribeTable) but never copied it into the
// CreateTableOutput it builds in the same call, so a caller reading TableId
// straight off the CreateTable response (rather than following up with
// DescribeTable) always saw it empty. Found while fixing gopherstack-rrtz's
// ImportTable TableId drop, which reads this same CreateTable response.
func TestCreateTable_TableId_ReturnedOnCreate(t *testing.T) {
	t.Parallel()

	client := newTestDynamoDBClient(t, dynamodb.NewHandler(dynamodb.NewInMemoryDB()))
	keySchema, attrDefs := wireTestKeySchema()

	createOut, err := client.CreateTable(t.Context(), &dynamodbsdk.CreateTableInput{
		TableName:            aws.String("table-id-table"),
		KeySchema:            keySchema,
		AttributeDefinitions: attrDefs,
		BillingMode:          dynamodbtypes.BillingModePayPerRequest,
	})
	require.NoError(t, err)
	require.NotEmpty(t, aws.ToString(createOut.TableDescription.TableId))

	descOut, err := client.DescribeTable(t.Context(), &dynamodbsdk.DescribeTableInput{
		TableName: aws.String("table-id-table"),
	})
	require.NoError(t, err)
	assert.Equal(t, aws.ToString(descOut.Table.TableId), aws.ToString(createOut.TableDescription.TableId))
}

// TestCreateTable_OnDemandThroughput_SurvivesWireConversion verifies that
// CreateTable's OnDemandThroughput reaches the backend and is reflected back on
// DescribeTable. models.CreateTableInput previously had no OnDemandThroughput
// field, and models.TableDescription had no OnDemandThroughput field either --
// even a backend fix to one side would still lose the value at the other.
func TestCreateTable_OnDemandThroughput_SurvivesWireConversion(t *testing.T) {
	t.Parallel()

	client := newTestDynamoDBClient(t, dynamodb.NewHandler(dynamodb.NewInMemoryDB()))
	keySchema, attrDefs := wireTestKeySchema()

	_, err := client.CreateTable(t.Context(), &dynamodbsdk.CreateTableInput{
		TableName:            aws.String("odt-table"),
		KeySchema:            keySchema,
		AttributeDefinitions: attrDefs,
		BillingMode:          dynamodbtypes.BillingModePayPerRequest,
		OnDemandThroughput: &dynamodbtypes.OnDemandThroughput{
			MaxReadRequestUnits:  aws.Int64(500),
			MaxWriteRequestUnits: aws.Int64(250),
		},
	})
	require.NoError(t, err)

	desc, err := client.DescribeTable(t.Context(), &dynamodbsdk.DescribeTableInput{
		TableName: aws.String("odt-table"),
	})
	require.NoError(t, err)
	require.NotNil(t, desc.Table.OnDemandThroughput, "OnDemandThroughput must survive CreateTable -> DescribeTable")

	assert.Equal(t, int64(500), aws.ToInt64(desc.Table.OnDemandThroughput.MaxReadRequestUnits))
	assert.Equal(t, int64(250), aws.ToInt64(desc.Table.OnDemandThroughput.MaxWriteRequestUnits))
}

// TestDescribeTable_TableSizeBytesAndCreationDateTime_SurviveWireConversion
// verifies two fields buildTableDescription always computes -- TableSizeBytes
// and CreationDateTime -- but models.TableDescription never declared, so
// FromSDKTableDescription silently discarded them on every DescribeTable call.
func TestDescribeTable_TableSizeBytesAndCreationDateTime_SurviveWireConversion(t *testing.T) {
	t.Parallel()

	client := newTestDynamoDBClient(t, dynamodb.NewHandler(dynamodb.NewInMemoryDB()))
	keySchema, attrDefs := wireTestKeySchema()

	_, err := client.CreateTable(t.Context(), &dynamodbsdk.CreateTableInput{
		TableName:            aws.String("size-table"),
		KeySchema:            keySchema,
		AttributeDefinitions: attrDefs,
		BillingMode:          dynamodbtypes.BillingModePayPerRequest,
	})
	require.NoError(t, err)

	desc, err := client.DescribeTable(t.Context(), &dynamodbsdk.DescribeTableInput{
		TableName: aws.String("size-table"),
	})
	require.NoError(t, err)

	require.NotNil(t, desc.Table.TableSizeBytes)
	assert.GreaterOrEqual(t, aws.ToInt64(desc.Table.TableSizeBytes), int64(0))
	require.NotNil(t, desc.Table.CreationDateTime, "CreationDateTime must survive the wire round-trip")
	assert.False(t, desc.Table.CreationDateTime.IsZero())
}

// TestUpdateTable_SurvivesWireConversion drives one UpdateTable mutation kind
// per subtest (AWS allows only one per call) and confirms it changed backend
// state visible through DescribeTable or DeleteTable. models.UpdateTableInput
// previously declared none of DeletionProtectionEnabled, TableClass,
// BillingMode, or SSESpecification, even though applyUpdateTableLocked already
// handles every one of them correctly when given a real SDK input.
func TestUpdateTable_SurvivesWireConversion(t *testing.T) {
	t.Parallel()

	t.Run("deletion protection", func(t *testing.T) {
		t.Parallel()

		client := newTestDynamoDBClient(t, dynamodb.NewHandler(dynamodb.NewInMemoryDB()))
		keySchema, attrDefs := wireTestKeySchema()

		_, err := client.CreateTable(t.Context(), &dynamodbsdk.CreateTableInput{
			TableName:            aws.String("dp-table"),
			KeySchema:            keySchema,
			AttributeDefinitions: attrDefs,
			BillingMode:          dynamodbtypes.BillingModePayPerRequest,
		})
		require.NoError(t, err)

		_, err = client.UpdateTable(t.Context(), &dynamodbsdk.UpdateTableInput{
			TableName:                 aws.String("dp-table"),
			DeletionProtectionEnabled: aws.Bool(true),
		})
		require.NoError(t, err)

		_, err = client.DeleteTable(t.Context(), &dynamodbsdk.DeleteTableInput{
			TableName: aws.String("dp-table"),
		})
		require.Error(t, err, "DeleteTable must be rejected once UpdateTable enabled deletion protection")
	})

	t.Run("table class", func(t *testing.T) {
		t.Parallel()

		client := newTestDynamoDBClient(t, dynamodb.NewHandler(dynamodb.NewInMemoryDB()))
		keySchema, attrDefs := wireTestKeySchema()

		_, err := client.CreateTable(t.Context(), &dynamodbsdk.CreateTableInput{
			TableName:            aws.String("class-table"),
			KeySchema:            keySchema,
			AttributeDefinitions: attrDefs,
			BillingMode:          dynamodbtypes.BillingModePayPerRequest,
		})
		require.NoError(t, err)

		_, err = client.UpdateTable(t.Context(), &dynamodbsdk.UpdateTableInput{
			TableName:  aws.String("class-table"),
			TableClass: dynamodbtypes.TableClassStandardInfrequentAccess,
		})
		require.NoError(t, err)

		desc, err := client.DescribeTable(t.Context(), &dynamodbsdk.DescribeTableInput{
			TableName: aws.String("class-table"),
		})
		require.NoError(t, err)
		require.NotNil(t, desc.Table.TableClassSummary)
		assert.Equal(
			t,
			dynamodbtypes.TableClassStandardInfrequentAccess,
			desc.Table.TableClassSummary.TableClass,
		)
	})

	t.Run("billing mode", func(t *testing.T) {
		t.Parallel()

		client := newTestDynamoDBClient(t, dynamodb.NewHandler(dynamodb.NewInMemoryDB()))
		keySchema, attrDefs := wireTestKeySchema()

		_, err := client.CreateTable(t.Context(), &dynamodbsdk.CreateTableInput{
			TableName:            aws.String("billing-table"),
			KeySchema:            keySchema,
			AttributeDefinitions: attrDefs,
			BillingMode:          dynamodbtypes.BillingModeProvisioned,
			ProvisionedThroughput: &dynamodbtypes.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(5),
				WriteCapacityUnits: aws.Int64(5),
			},
		})
		require.NoError(t, err)

		_, err = client.UpdateTable(t.Context(), &dynamodbsdk.UpdateTableInput{
			TableName:   aws.String("billing-table"),
			BillingMode: dynamodbtypes.BillingModePayPerRequest,
		})
		require.NoError(t, err)

		desc, err := client.DescribeTable(t.Context(), &dynamodbsdk.DescribeTableInput{
			TableName: aws.String("billing-table"),
		})
		require.NoError(t, err)
		require.NotNil(t, desc.Table.BillingModeSummary)
		assert.Equal(
			t,
			dynamodbtypes.BillingModePayPerRequest,
			desc.Table.BillingModeSummary.BillingMode,
		)
	})

	t.Run("sse specification", func(t *testing.T) {
		t.Parallel()

		client := newTestDynamoDBClient(t, dynamodb.NewHandler(dynamodb.NewInMemoryDB()))
		keySchema, attrDefs := wireTestKeySchema()

		_, err := client.CreateTable(t.Context(), &dynamodbsdk.CreateTableInput{
			TableName:            aws.String("update-sse-table"),
			KeySchema:            keySchema,
			AttributeDefinitions: attrDefs,
			BillingMode:          dynamodbtypes.BillingModePayPerRequest,
		})
		require.NoError(t, err)

		_, err = client.UpdateTable(t.Context(), &dynamodbsdk.UpdateTableInput{
			TableName: aws.String("update-sse-table"),
			SSESpecification: &dynamodbtypes.SSESpecification{
				Enabled:        aws.Bool(true),
				SSEType:        dynamodbtypes.SSETypeKms,
				KMSMasterKeyId: aws.String("arn:aws:kms:us-east-1:000000000000:key/update-test-key"),
			},
		})
		require.NoError(t, err)

		desc, err := client.DescribeTable(t.Context(), &dynamodbsdk.DescribeTableInput{
			TableName: aws.String("update-sse-table"),
		})
		require.NoError(t, err)
		require.NotNil(t, desc.Table.SSEDescription)
		assert.Equal(t, dynamodbtypes.SSETypeKms, desc.Table.SSEDescription.SSEType)
		assert.Equal(
			t,
			"arn:aws:kms:us-east-1:000000000000:key/update-test-key",
			aws.ToString(desc.Table.SSEDescription.KMSMasterKeyArn),
		)
	})
}
