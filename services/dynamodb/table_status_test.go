package dynamodb_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddb "github.com/blackbirdworks/gopherstack/services/dynamodb"
)

// createInput returns a minimal CreateTableInput for testing.
func createInput(name string) *sdk.CreateTableInput {
	return &sdk.CreateTableInput{
		TableName: aws.String(name),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(5),
			WriteCapacityUnits: aws.Int64(5),
		},
	}
}

func TestTableStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		tableName       string
		wantInitStatus  types.TableStatus
		wantFinalStatus types.TableStatus
		createDelay     time.Duration
		finalSleep      time.Duration
	}{
		{
			name:           "immediately_active",
			tableName:      "status-table",
			wantInitStatus: types.TableStatusActive,
		},
		{
			name:            "lifecycle_with_delay",
			tableName:       "lifecycle-table",
			createDelay:     80 * time.Millisecond,
			wantInitStatus:  types.TableStatusCreating,
			wantFinalStatus: types.TableStatusActive,
			finalSleep:      200 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := ddb.NewInMemoryDB()
			if tt.createDelay > 0 {
				db.SetCreateDelay(tt.createDelay)
			}

			out, err := db.CreateTable(t.Context(), createInput(tt.tableName))
			require.NoError(t, err)
			assert.Equal(t, tt.wantInitStatus, out.TableDescription.TableStatus)

			desc, err := db.DescribeTable(t.Context(), &sdk.DescribeTableInput{
				TableName: aws.String(tt.tableName),
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantInitStatus, desc.Table.TableStatus)

			if tt.finalSleep > 0 {
				time.Sleep(tt.finalSleep)

				desc2, err2 := db.DescribeTable(t.Context(), &sdk.DescribeTableInput{
					TableName: aws.String(tt.tableName),
				})
				require.NoError(t, err2)
				assert.Equal(t, tt.wantFinalStatus, desc2.Table.TableStatus,
					"expected ACTIVE after delay elapsed")
			}
		})
	}
}

// TestDeleteWhileCreating verifies that DeleteTable rejects a table still in CREATING
// state with ResourceInUseException (matching real AWS, which returns the same error
// "If a table is in CREATING or UPDATING states"), and that deletion succeeds normally
// once the table transitions to ACTIVE.
func TestDeleteWhileCreating(t *testing.T) {
	t.Parallel()

	db := ddb.NewInMemoryDB()
	db.SetCreateDelay(150 * time.Millisecond)

	out, err := db.CreateTable(t.Context(), createInput("timer-cancel-table"))
	require.NoError(t, err)
	require.Equal(t, types.TableStatusCreating, out.TableDescription.TableStatus)

	// Delete while still CREATING must be rejected.
	_, err = db.DeleteTable(t.Context(), &sdk.DeleteTableInput{
		TableName: aws.String("timer-cancel-table"),
	})
	require.Error(t, err)
	var ddbErr *ddb.Error
	require.ErrorAs(t, err, &ddbErr)
	assert.Contains(t, ddbErr.Type, "ResourceInUseException")

	require.Eventually(t, func() bool {
		desc, descErr := db.DescribeTable(t.Context(), &sdk.DescribeTableInput{
			TableName: aws.String("timer-cancel-table"),
		})

		return descErr == nil && desc.Table.TableStatus == types.TableStatusActive
	}, time.Second, 10*time.Millisecond, "table should become ACTIVE after the create delay elapses")

	// Now that the table is ACTIVE, deletion must succeed.
	_, err = db.DeleteTable(t.Context(), &sdk.DeleteTableInput{
		TableName: aws.String("timer-cancel-table"),
	})
	require.NoError(t, err)
}
