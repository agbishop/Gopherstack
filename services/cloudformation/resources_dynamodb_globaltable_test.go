package cloudformation_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
	ddbbackend "github.com/blackbirdworks/gopherstack/services/dynamodb"
)

// TestDeleteDynamoDBGlobalTable_RemovesReplicaTables confirms DeleteStack's teardown of an
// AWS::DynamoDB::GlobalTable resource actually deletes the replica tables CreateGlobalTable
// provisioned in every region, instead of leaving them (and the global table registration
// itself) as ghost rows. DynamoDB table names are user-chosen and reusable, so a stack
// recreated with the same TableName after a delete would otherwise hit
// GlobalTableAlreadyExistsException on the very next CreateStack.
func TestDeleteDynamoDBGlobalTable_RemovesReplicaTables(t *testing.T) {
	t.Parallel()

	backends := newServiceBackends()
	rc := cloudformation.NewResourceCreator(backends)

	props := map[string]any{
		"TableName": "unit-cfn-global-table",
		"Replicas": []any{
			map[string]any{"Region": "us-east-1"},
			map[string]any{"Region": "eu-west-1"},
		},
	}

	physID, err := rc.Create(t.Context(), "MyGlobalTable", "AWS::DynamoDB::GlobalTable", props, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, physID)

	_, err = backends.DynamoDB.Backend.DescribeGlobalTable(t.Context(), &awsddb.DescribeGlobalTableInput{
		GlobalTableName: aws.String("unit-cfn-global-table"),
	})
	require.NoError(t, err, "precondition: global table must exist after create")

	for _, region := range []string{"us-east-1", "eu-west-1"} {
		_, err = backends.DynamoDB.Backend.DescribeTable(
			ddbbackend.WithRegion(t.Context(), region),
			&awsddb.DescribeTableInput{TableName: aws.String("unit-cfn-global-table")},
		)
		require.NoErrorf(t, err, "precondition: replica table must exist in %s after create", region)
	}

	err = rc.Delete(t.Context(), "AWS::DynamoDB::GlobalTable", physID, props)
	require.NoError(t, err)

	_, err = backends.DynamoDB.Backend.DescribeGlobalTable(t.Context(), &awsddb.DescribeGlobalTableInput{
		GlobalTableName: aws.String("unit-cfn-global-table"),
	})
	require.Error(t, err, "global table registration must not survive DeleteStack as a ghost row")

	for _, region := range []string{"us-east-1", "eu-west-1"} {
		_, err = backends.DynamoDB.Backend.DescribeTable(
			ddbbackend.WithRegion(t.Context(), region),
			&awsddb.DescribeTableInput{TableName: aws.String("unit-cfn-global-table")},
		)
		require.Errorf(t, err, "replica table in %s must not survive DeleteStack as a ghost row", region)
	}
}
