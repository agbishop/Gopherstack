package integration_test

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/data/aztables"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createCosmosDBTableServiceClient returns an azure-sdk-for-go Table service
// client pointed at the shared test container's Cosmos DB port
// (cosmosDBEndpoint in main_test.go), exercising Cosmos DB's Table API
// rather than services/azuretable's own dedicated Table Storage port -- see
// AZURE.md section 9's M6 milestone. Skips the calling test if that port
// could not be determined (mirrors createAzureTableServiceClient).
//
// Unlike createAzureTableServiceClient, no account name is appended to the
// service URL: real Cosmos DB's Table API disambiguates Table vs. Core/SQL
// traffic by path shape on one hostname, not by an Azurite-style
// account-name path segment (see services/cosmosdb/table_api.go's
// isTableAPIPath doc comment), and gopherstack's Cosmos DB emulation mirrors
// that. The credential itself is unused for anything beyond building a
// structurally valid Authorization header: services/cosmosdb's checkAuth is
// permissive by default (matching every other Azure service here) unless
// --cosmosdb-validate-auth is set, which this test container doesn't set.
func createCosmosDBTableServiceClient(t *testing.T) *aztables.ServiceClient {
	t.Helper()

	if cosmosDBEndpoint == "" {
		t.Skip("Cosmos DB endpoint not available (mapped port could not be determined)")
	}

	cred, err := aztables.NewSharedKeyCredential(azureTableDevAccountName, azureTableDevAccountKey)
	require.NoError(t, err, "unable to build SharedKeyCredential")

	client, err := aztables.NewServiceClientWithSharedKey(cosmosDBEndpoint, cred, nil)
	require.NoError(t, err, "unable to construct Cosmos DB Table API service client")

	return client
}

// TestIntegration_CosmosDB_TableAPI_TableAndEntityLifecycle mirrors
// TestIntegration_AzureTable_TableAndEntityLifecycle (azuretable_test.go)
// almost exactly -- the whole point of AZURE.md section 9's M6 milestone is
// that Cosmos DB's Table API is wire-identical to Table Storage's, so an
// unmodified aztables client pointed at the Cosmos DB endpoint instead
// should pass the same lifecycle assertions unchanged.
func TestIntegration_CosmosDB_TableAPI_TableAndEntityLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)
	service := createCosmosDBTableServiceClient(t)
	ctx := t.Context()

	tableName := "testtable" + uuid.NewString()[:8]
	tableClient := service.NewClient(tableName)

	// CreateTable.
	_, err := tableClient.CreateTable(ctx, nil)
	require.NoError(t, err)

	// ListTables: created table should appear.
	found := false

	pager := service.NewListTablesPager(nil)
	for pager.More() {
		page, pageErr := pager.NextPage(ctx)
		require.NoError(t, pageErr)

		for _, tbl := range page.Tables {
			if tbl.Name != nil && *tbl.Name == tableName {
				found = true
			}
		}
	}

	assert.True(t, found, "created table should appear in ListTables")

	// Insert entity with a mixed-EDM-type property set.
	entity := aztables.EDMEntity{
		Entity: aztables.Entity{PartitionKey: "partition1", RowKey: "row1"},
		Properties: map[string]any{
			"StringProp": "hello",
			"IntProp":    int32(42),
			"BoolProp":   true,
			"DoubleProp": 3.14,
			"Int64Prop":  aztables.EDMInt64(9223372036854775807),
			"GuidProp":   aztables.EDMGUID("550e8400-e29b-41d4-a716-446655440000"),
			"BinaryProp": aztables.EDMBinary([]byte("gopherstack")),
		},
	}

	marshaled, err := entity.MarshalJSON()
	require.NoError(t, err)

	_, err = tableClient.AddEntity(ctx, marshaled, nil)
	require.NoError(t, err)

	// GetEntity: verify the mixed-EDM-type round trip.
	getResp, err := tableClient.GetEntity(ctx, "partition1", "row1", nil)
	require.NoError(t, err)

	var got aztables.EDMEntity
	require.NoError(t, got.UnmarshalJSON(getResp.Value))
	assert.Equal(t, "hello", got.Properties["StringProp"])
	assert.Equal(t, int32(42), got.Properties["IntProp"])
	assert.InDelta(t, 3.14, got.Properties["DoubleProp"], 0.0001)
	assert.Equal(t, aztables.EDMInt64(9223372036854775807), got.Properties["Int64Prop"])
	assert.Equal(t, aztables.EDMGUID("550e8400-e29b-41d4-a716-446655440000"), got.Properties["GuidProp"])
	assert.Equal(t, aztables.EDMBinary([]byte("gopherstack")), got.Properties["BinaryProp"])

	// Query with $filter.
	filter := "PartitionKey eq 'partition1' and IntProp eq 42"
	listPager := tableClient.NewListEntitiesPager(&aztables.ListEntitiesOptions{Filter: &filter})

	queryFound := false

	for listPager.More() {
		page, pageErr := listPager.NextPage(ctx)
		require.NoError(t, pageErr)

		queryFound = queryFound || len(page.Entities) > 0
	}

	assert.True(t, queryFound, "query with $filter should return the inserted entity")

	// MergeEntity: only StringProp changes; other properties survive.
	mergeEntity := aztables.EDMEntity{
		Entity:     aztables.Entity{PartitionKey: "partition1", RowKey: "row1"},
		Properties: map[string]any{"StringProp": "merged"},
	}

	mergeMarshaled, err := mergeEntity.MarshalJSON()
	require.NoError(t, err)

	_, err = tableClient.UpdateEntity(
		ctx,
		mergeMarshaled,
		&aztables.UpdateEntityOptions{UpdateMode: aztables.UpdateModeMerge},
	)
	require.NoError(t, err)

	getResp, err = tableClient.GetEntity(ctx, "partition1", "row1", nil)
	require.NoError(t, err)

	var afterMerge aztables.EDMEntity
	require.NoError(t, afterMerge.UnmarshalJSON(getResp.Value))
	assert.Equal(t, "merged", afterMerge.Properties["StringProp"])
	assert.Equal(t, int32(42), afterMerge.Properties["IntProp"], "merge must not drop unrelated properties")

	// ReplaceEntity: drops unrelated properties.
	replaceEntity := aztables.EDMEntity{
		Entity:     aztables.Entity{PartitionKey: "partition1", RowKey: "row1"},
		Properties: map[string]any{"StringProp": "replaced"},
	}

	replaceMarshaled, err := replaceEntity.MarshalJSON()
	require.NoError(t, err)

	replaceResp, err := tableClient.UpdateEntity(
		ctx, replaceMarshaled, &aztables.UpdateEntityOptions{UpdateMode: aztables.UpdateModeReplace},
	)
	require.NoError(t, err)

	getResp, err = tableClient.GetEntity(ctx, "partition1", "row1", nil)
	require.NoError(t, err)

	var afterReplace aztables.EDMEntity
	require.NoError(t, afterReplace.UnmarshalJSON(getResp.Value))
	assert.Equal(t, "replaced", afterReplace.Properties["StringProp"])
	_, hasIntProp := afterReplace.Properties["IntProp"]
	assert.False(t, hasIntProp, "replace must drop properties not present in the new body")

	// Conditional delete with a wrong ETag: expect an error.
	wrongETag := azcore.ETag(`W/"datetime'bogus'"`)
	_, err = tableClient.DeleteEntity(ctx, "partition1", "row1", &aztables.DeleteEntityOptions{IfMatch: &wrongETag})
	require.Error(t, err)

	// Delete with the correct ETag.
	_, err = tableClient.DeleteEntity(
		ctx,
		"partition1",
		"row1",
		&aztables.DeleteEntityOptions{IfMatch: &replaceResp.ETag},
	)
	require.NoError(t, err)

	// Verify gone.
	_, err = tableClient.GetEntity(ctx, "partition1", "row1", nil)
	require.Error(t, err)

	// DeleteTable.
	_, err = tableClient.Delete(ctx, nil)
	require.NoError(t, err)

	// Verify gone.
	found = false

	pager = service.NewListTablesPager(nil)
	for pager.More() {
		page, pageErr := pager.NextPage(ctx)
		require.NoError(t, pageErr)

		for _, tbl := range page.Tables {
			if tbl.Name != nil && *tbl.Name == tableName {
				found = true
			}
		}
	}

	assert.False(t, found, "deleted table should no longer appear in ListTables")
}
