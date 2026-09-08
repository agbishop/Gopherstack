package dynamodb_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/dynamodb"
)

// TestDeleteTable_ClearsFISReplicationPause verifies that DeleteTable clears the
// FIS global-table-pause-replication fault for the deleted table. TableArn is
// built deterministically from the table name, so a table recreated with the
// same name previously inherited a stale "replication paused" state left
// behind by fisReplicationPaused.
func TestDeleteTable_ClearsFISReplicationPause(t *testing.T) {
	t.Parallel()

	db := newInMemoryTestDB(t)
	h := dynamodb.NewHandler(db)
	ctx := t.Context()

	const tableName = "ghost-row-table"
	createOnDemandTestTable(t, db, tableName)

	descOut, err := db.DescribeTable(ctx, &sdkddb.DescribeTableInput{TableName: aws.String(tableName)})
	require.NoError(t, err)
	tableARN := aws.ToString(descOut.Table.TableArn)

	require.NoError(t, h.ExecuteFISAction(ctx, service.FISActionExecution{
		ActionID: "aws:dynamodb:global-table-pause-replication",
		Targets:  []string{tableARN},
	}))
	require.True(t, db.IsReplicationPaused(tableARN))

	_, err = db.DeleteTable(ctx, &sdkddb.DeleteTableInput{TableName: aws.String(tableName)})
	require.NoError(t, err)

	createOnDemandTestTable(t, db, tableName)

	descOut2, err := db.DescribeTable(ctx, &sdkddb.DescribeTableInput{TableName: aws.String(tableName)})
	require.NoError(t, err)
	newARN := aws.ToString(descOut2.Table.TableArn)

	require.Equal(t, tableARN, newARN, "TableArn must be deterministic for the same table name")
	assert.False(t, db.IsReplicationPaused(newARN),
		"recreated table must not inherit the deleted table's FIS replication-pause state")
}
