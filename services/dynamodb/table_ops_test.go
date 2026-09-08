package dynamodb_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/blackbirdworks/gopherstack/services/dynamodb/models"

	"github.com/blackbirdworks/gopherstack/services/dynamodb"

	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodb_sdk "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *dynamodb.InMemoryDB)
		validate func(*testing.T, *dynamodb.InMemoryDB, any, error)
		run      func(*dynamodb.InMemoryDB) (any, error)
		name     string
	}{
		{
			name: "CreateTable_Success",
			run: func(db *dynamodb.InMemoryDB) (any, error) {
				input := models.CreateTableInput{
					TableName: "TestTable",
					KeySchema: []models.KeySchemaElement{
						{AttributeName: "pk", KeyType: models.KeyTypeHash},
					},
					AttributeDefinitions: []models.AttributeDefinition{
						{AttributeName: "pk", AttributeType: "S"},
					},
				}
				sdkInput := models.ToSDKCreateTableInput(&input)

				return db.CreateTable(t.Context(), sdkInput)
			},
			validate: func(t *testing.T, _ *dynamodb.InMemoryDB, resp any, err error) {
				t.Helper()
				require.NoError(t, err)
				output := resp.(*dynamodb_sdk.CreateTableOutput)
				assert.Equal(t, "TestTable", aws.ToString(output.TableDescription.TableName))
			},
		},
		{
			name: "CreateTable_AlreadyExists",
			setup: func(t *testing.T, db *dynamodb.InMemoryDB) {
				t.Helper()
				createTable(t, db, "ExistingTable")
			},
			run: func(db *dynamodb.InMemoryDB) (any, error) {
				input := models.CreateTableInput{
					TableName: "ExistingTable",
					KeySchema: []models.KeySchemaElement{
						{AttributeName: "pk", KeyType: models.KeyTypeHash},
					},
					AttributeDefinitions: []models.AttributeDefinition{
						{AttributeName: "pk", AttributeType: "S"},
					},
				}
				sdkInput := models.ToSDKCreateTableInput(&input)

				return db.CreateTable(t.Context(), sdkInput)
			},
			validate: func(t *testing.T, _ *dynamodb.InMemoryDB, _ any, err error) {
				t.Helper()
				require.Error(t, err)
			},
		},
		{
			name: "DescribeTable_Success",
			setup: func(t *testing.T, db *dynamodb.InMemoryDB) {
				t.Helper()
				createTable(t, db, "TestTable")
			},
			run: func(db *dynamodb.InMemoryDB) (any, error) {
				input := models.DescribeTableInput{TableName: "TestTable"}
				sdkInput := models.ToSDKDescribeTableInput(&input)

				return db.DescribeTable(t.Context(), sdkInput)
			},
			validate: func(t *testing.T, _ *dynamodb.InMemoryDB, resp any, err error) {
				t.Helper()
				require.NoError(t, err)
				output := resp.(*dynamodb_sdk.DescribeTableOutput)
				assert.Equal(t, "TestTable", aws.ToString(output.Table.TableName))
				assert.Equal(t, "ACTIVE", string(output.Table.TableStatus))
			},
		},
		{
			name: "DescribeTable_NotFound",
			run: func(db *dynamodb.InMemoryDB) (any, error) {
				input := models.DescribeTableInput{TableName: "NonExistent"}
				sdkInput := models.ToSDKDescribeTableInput(&input)

				return db.DescribeTable(t.Context(), sdkInput)
			},
			validate: func(t *testing.T, _ *dynamodb.InMemoryDB, _ any, err error) {
				t.Helper()
				require.Error(t, err)
			},
		},
		{
			name: "ListTables_Success",
			setup: func(t *testing.T, db *dynamodb.InMemoryDB) {
				t.Helper()
				createTable(t, db, "Table1")
			},
			run: func(db *dynamodb.InMemoryDB) (any, error) {
				return db.ListTables(t.Context(), &dynamodb_sdk.ListTablesInput{})
			},
			validate: func(t *testing.T, _ *dynamodb.InMemoryDB, resp any, err error) {
				t.Helper()
				require.NoError(t, err)
				output := resp.(*dynamodb_sdk.ListTablesOutput)
				assert.Contains(t, output.TableNames, "Table1")
			},
		},
		{
			name: "DeleteTable_Success",
			setup: func(t *testing.T, db *dynamodb.InMemoryDB) {
				t.Helper()
				createTable(t, db, "DeleteMe")
			},
			run: func(db *dynamodb.InMemoryDB) (any, error) {
				input := models.DeleteTableInput{TableName: "DeleteMe"}
				sdkInput := models.ToSDKDeleteTableInput(&input)

				return db.DeleteTable(t.Context(), sdkInput)
			},
			validate: func(t *testing.T, db *dynamodb.InMemoryDB, _ any, err error) {
				t.Helper()
				require.NoError(t, err)
				// Verify deletion by trying to describe it
				descInput := models.DescribeTableInput{TableName: "DeleteMe"}
				sdkDesc := models.ToSDKDescribeTableInput(&descInput)
				_, err = db.DescribeTable(t.Context(), sdkDesc)
				require.Error(t, err)
			},
		},
		{
			name: "DeleteTable_WithGSI_NilProvisionedThroughput",
			setup: func(t *testing.T, db *dynamodb.InMemoryDB) {
				t.Helper()
				// Create a table with a GSI but no ProvisionedThroughput (on-demand billing)
				_, err := db.CreateTable(t.Context(), &dynamodb_sdk.CreateTableInput{
					TableName: aws.String("GSITable"),
					AttributeDefinitions: []types.AttributeDefinition{
						{
							AttributeName: aws.String("pk"),
							AttributeType: types.ScalarAttributeTypeS,
						},
						{
							AttributeName: aws.String("gsiPK"),
							AttributeType: types.ScalarAttributeTypeS,
						},
					},
					KeySchema: []types.KeySchemaElement{
						{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
					},
					GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
						{
							IndexName: aws.String("GSI1"),
							KeySchema: []types.KeySchemaElement{
								{AttributeName: aws.String("gsiPK"), KeyType: types.KeyTypeHash},
							},
							Projection: &types.Projection{
								ProjectionType: types.ProjectionTypeAll,
							},
						},
					},
					BillingMode: types.BillingModePayPerRequest,
				})
				require.NoError(t, err)
			},
			run: func(db *dynamodb.InMemoryDB) (any, error) {
				input := models.DeleteTableInput{TableName: "GSITable"}
				sdkInput := models.ToSDKDeleteTableInput(&input)

				return db.DeleteTable(t.Context(), sdkInput)
			},
			validate: func(t *testing.T, db *dynamodb.InMemoryDB, _ any, err error) {
				t.Helper()
				require.NoError(t, err)
				// Verify deletion
				descInput := models.DescribeTableInput{TableName: "GSITable"}
				sdkDesc := models.ToSDKDescribeTableInput(&descInput)
				_, err = db.DescribeTable(t.Context(), sdkDesc)
				require.Error(t, err)
			},
		},
		{
			name: "DeleteTable_NotFound",
			run: func(db *dynamodb.InMemoryDB) (any, error) {
				input := models.DeleteTableInput{TableName: "NonExistent"}
				sdkInput := models.ToSDKDeleteTableInput(&input)

				return db.DeleteTable(t.Context(), sdkInput)
			},
			validate: func(t *testing.T, _ *dynamodb.InMemoryDB, _ any, err error) {
				t.Helper()
				require.Error(t, err)
				// Verify it's a ResourceNotFoundException (returns as HTTP 400, not 404)
				if ddbErr, ok := errors.AsType[*dynamodb.Error](err); ok {
					assert.Contains(t, ddbErr.Type, "ResourceNotFoundException")
				}
			},
		},
		{
			name: "DeleteTable_Cleanup",
			setup: func(t *testing.T, db *dynamodb.InMemoryDB) {
				t.Helper()
				createTable(t, db, "CleanupTable")
			},
			run: func(db *dynamodb.InMemoryDB) (any, error) {
				input := models.DeleteTableInput{TableName: "CleanupTable"}
				sdkInput := models.ToSDKDeleteTableInput(&input)

				return db.DeleteTable(t.Context(), sdkInput)
			},
			validate: func(t *testing.T, _ *dynamodb.InMemoryDB, _ any, err error) {
				t.Helper()
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := dynamodb.NewInMemoryDB()
			if tt.setup != nil {
				tt.setup(t, db)
			}

			resp, err := tt.run(db)

			if tt.validate != nil {
				tt.validate(t, db, resp, err)
			}
		})
	}
}

func createTable(t *testing.T, db *dynamodb.InMemoryDB, name string) {
	t.Helper()
	input := models.CreateTableInput{
		TableName: name,
		KeySchema: []models.KeySchemaElement{
			{AttributeName: "id", KeyType: models.KeyTypeHash},
		},
		AttributeDefinitions: []models.AttributeDefinition{
			{AttributeName: "id", AttributeType: "S"},
		},
	}
	_, err := db.CreateTable(t.Context(), models.ToSDKCreateTableInput(&input))
	require.NoError(t, err)
}

func TestBillingMode_PayPerRequest_Persisted(t *testing.T) {
	t.Parallel()
	db := newInMemoryTestDB(t)
	ctx := context.Background()
	createOnDemandTestTable(t, db, "BillingOD")

	out, err := db.DescribeTable(ctx, &dynamodb_sdk.DescribeTableInput{
		TableName: aws.String("BillingOD"),
	})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}

	if out.Table.BillingModeSummary == nil {
		t.Fatal("expected BillingModeSummary, got nil")
	}

	if out.Table.BillingModeSummary.BillingMode != types.BillingModePayPerRequest {
		t.Errorf("expected PAY_PER_REQUEST, got %s", out.Table.BillingModeSummary.BillingMode)
	}
}

func TestBillingMode_UpdateTable_Persisted(t *testing.T) {
	t.Parallel()
	db := newInMemoryTestDB(t)
	ctx := context.Background()
	createSimpleTestTable(t, db, "BillingSwitch")

	_, err := db.UpdateTable(ctx, &dynamodb_sdk.UpdateTableInput{
		TableName:   aws.String("BillingSwitch"),
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		t.Fatalf("UpdateTable: %v", err)
	}

	out, err := db.DescribeTable(ctx, &dynamodb_sdk.DescribeTableInput{
		TableName: aws.String("BillingSwitch"),
	})
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}

	if out.Table.BillingModeSummary == nil {
		t.Fatal("expected BillingModeSummary")
	}

	if out.Table.BillingModeSummary.BillingMode != types.BillingModePayPerRequest {
		t.Errorf("expected PAY_PER_REQUEST, got %s", out.Table.BillingModeSummary.BillingMode)
	}
}

// TestCreateTable_BillingMode_EnumValidation asserts ValidationException for unknown BillingMode values.
func TestCreateTable_BillingMode_EnumValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		billingMode types.BillingMode
		name        string
		wantContain string
		wantErr     bool
	}{
		{
			name:        "valid_provisioned",
			billingMode: types.BillingModeProvisioned,
			wantErr:     false,
		},
		{
			name:        "valid_pay_per_request",
			billingMode: types.BillingModePayPerRequest,
			wantErr:     false,
		},
		{
			name:        "empty_billing_mode_defaults_to_provisioned",
			billingMode: "",
			wantErr:     false,
		},
		{
			name:        "invalid_billing_mode_rejected",
			billingMode: "INVALID_MODE",
			wantErr:     true,
			wantContain: "billingMode",
		},
		{
			name:        "typo_billing_mode_rejected",
			billingMode: "PAY_PER_USE",
			wantErr:     true,
			wantContain: "billingMode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := newInMemoryTestDB(t)
			ctx := context.Background()

			input := &dynamodb_sdk.CreateTableInput{
				TableName: aws.String("billing-test-" + tt.name),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
				},
				AttributeDefinitions: []types.AttributeDefinition{
					{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
				},
				BillingMode: tt.billingMode,
			}

			if tt.billingMode == types.BillingModeProvisioned || tt.billingMode == "" {
				input.ProvisionedThroughput = &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(1),
					WriteCapacityUnits: aws.Int64(1),
				}
			}

			_, err := db.CreateTable(ctx, input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantContain)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestUpdateTable_GSI_Ceiling asserts LimitExceededException when adding a 21st GSI.
func TestUpdateTable_GSI_Ceiling(t *testing.T) {
	t.Parallel()

	db := newInMemoryTestDB(t)
	ctx := context.Background()

	const gsiCount = 20
	gsis := make([]types.GlobalSecondaryIndex, gsiCount)
	attrDefs := make([]types.AttributeDefinition, 0, 1+len(gsis))
	attrDefs = append(attrDefs, types.AttributeDefinition{
		AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS,
	})

	for i := range gsis {
		an := fmt.Sprintf("gk%d", i)
		attrDefs = append(attrDefs, types.AttributeDefinition{
			AttributeName: aws.String(an), AttributeType: types.ScalarAttributeTypeS,
		})
		gsis[i] = types.GlobalSecondaryIndex{
			IndexName: aws.String(fmt.Sprintf("gsi-%d", i)),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String(an), KeyType: types.KeyTypeHash},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
			ProvisionedThroughput: &types.ProvisionedThroughput{
				ReadCapacityUnits:  aws.Int64(1),
				WriteCapacityUnits: aws.Int64(1),
			},
		}
	}

	_, err := db.CreateTable(ctx, &dynamodb_sdk.CreateTableInput{
		TableName: aws.String("gsi-ceiling-table"),
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		},
		AttributeDefinitions:   attrDefs,
		GlobalSecondaryIndexes: gsis,
		BillingMode:            types.BillingModeProvisioned,
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits: aws.Int64(5), WriteCapacityUnits: aws.Int64(5),
		},
	})
	require.NoError(t, err)

	_, err = db.UpdateTable(ctx, &dynamodb_sdk.UpdateTableInput{
		TableName: aws.String("gsi-ceiling-table"),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("extra_gk"), AttributeType: types.ScalarAttributeTypeS},
		},
		GlobalSecondaryIndexUpdates: []types.GlobalSecondaryIndexUpdate{
			{Create: &types.CreateGlobalSecondaryIndexAction{
				IndexName: aws.String("gsi-21"),
				KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("extra_gk"), KeyType: types.KeyTypeHash},
				},
				Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(1),
					WriteCapacityUnits: aws.Int64(1),
				},
			}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LimitExceededException")
}

func TestDescribeTable_BillingMode_PayPerRequest(t *testing.T) {
	t.Parallel()

	backend := dynamodb.NewInMemoryDB()
	handler := dynamodb.NewHandler(backend)

	code, _ := invokeOp(t, handler, "CreateTable", map[string]any{
		"TableName": "PayPerRequestTest",
		"KeySchema": []map[string]any{{"AttributeName": "pk", "KeyType": "HASH"}},
		"AttributeDefinitions": []map[string]any{
			{"AttributeName": "pk", "AttributeType": "S"},
		},
		"BillingMode": "PAY_PER_REQUEST",
	})
	require.Equal(t, http.StatusOK, code)

	code2, resp := invokeOp(t, handler, "DescribeTable", map[string]any{
		"TableName": "PayPerRequestTest",
	})
	require.Equal(t, http.StatusOK, code2)

	tableDesc, _ := resp["Table"].(map[string]any)
	require.NotNil(t, tableDesc)

	billingMode, _ := tableDesc["BillingModeSummary"].(map[string]any)
	require.NotNil(t, billingMode)
	assert.Equal(t, "PAY_PER_REQUEST", billingMode["BillingMode"])
}

// TestBatchWriteItem_GlobalTablePropagation verifies that BatchWriteItem puts and
// deletes propagate to all global table replicas.
