package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/azureauth"
	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

func storageAccountID(name string) azurearm.ResourceID {
	return azurearm.ResourceID{
		SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.Storage",
		Types: []string{"storageAccounts"}, Names: []string{name},
	}
}

func TestStorageProvider_PutGetDelete(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)
	ctx := t.Context()
	id := storageAccountID("acct1")

	body, err := sp.Put(ctx, id, map[string]any{
		"location": "westus",
		"tags":     map[string]any{"env": "dev"},
		"sku":      map[string]any{"name": "Standard_GRS"},
		"kind":     "BlobStorage",
	})
	require.NoError(t, err)
	assert.Equal(t, "acct1", body["name"])
	assert.Equal(t, "Microsoft.Storage/storageAccounts", body["type"])
	assert.Equal(t, "westus", body["location"])
	assert.Equal(t, "BlobStorage", body["kind"])

	sku, ok := body["sku"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Standard_GRS", sku["name"])

	props, ok := body["properties"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Succeeded", props["provisioningState"])

	endpoints, ok := props["primaryEndpoints"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, endpoints["blob"], "acct1")
	assert.Contains(t, endpoints["queue"], "acct1")
	assert.Contains(t, endpoints["table"], "acct1")

	got, err := sp.Get(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, body["id"], got["id"])

	require.NoError(t, sp.Delete(ctx, id))

	_, err = sp.Get(ctx, id)
	require.ErrorIs(t, err, azurearm.ErrStorageAccountNotFound)
}

func TestStorageProvider_Get_NotFound(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)

	_, err := sp.Get(t.Context(), storageAccountID("missing"))
	require.ErrorIs(t, err, azurearm.ErrStorageAccountNotFound)
}

func TestStorageProvider_Delete_NotFound(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)

	err := sp.Delete(t.Context(), storageAccountID("missing"))
	require.ErrorIs(t, err, azurearm.ErrStorageAccountNotFound)
}

func TestStorageProvider_List(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)
	ctx := t.Context()

	_, err := sp.Put(ctx, storageAccountID("zeta"), map[string]any{"location": "westus"})
	require.NoError(t, err)
	_, err = sp.Put(ctx, storageAccountID("alpha"), map[string]any{"location": "westus"})
	require.NoError(t, err)

	values, err := sp.List(ctx, storageAccountID(""))
	require.NoError(t, err)
	require.Len(t, values, 2)
	assert.Equal(t, "alpha", values[0]["name"])
	assert.Equal(t, "zeta", values[1]["name"])
}

// TestStorageProvider_ListKeys_MatchesRealARMShape asserts the response
// shape verified against learn.microsoft.com's "Storage Accounts - List
// Keys" REST API documentation:
// {"keys":[{"keyName","value","permissions"}, ...]}, with exactly two keys
// (key1/key2), "permissions":"Full" (matching the KeyPermission enum, not
// "FULL"), and values equal to pkgs/azureauth's well-known devstoreaccount1
// development key.
func TestStorageProvider_ListKeys_MatchesRealARMShape(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)
	ctx := t.Context()
	id := storageAccountID("acct1")

	_, err := sp.Put(ctx, id, map[string]any{"location": "westus"})
	require.NoError(t, err)

	resp, err := sp.ListKeys(ctx, id)
	require.NoError(t, err)

	keys, ok := resp["keys"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, keys, 2)

	assert.Equal(t, "key1", keys[0]["keyName"])
	assert.Equal(t, "key2", keys[1]["keyName"])

	for _, k := range keys {
		assert.Equal(t, "Full", k["permissions"])
		assert.Equal(t, azureauth.DefaultAccountKey, k["value"])
	}
}

func TestStorageProvider_ListKeys_NotFound(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)

	_, err := sp.ListKeys(t.Context(), storageAccountID("missing"))
	require.ErrorIs(t, err, azurearm.ErrStorageAccountNotFound)
}

func TestStorageProvider_EndpointOverrides(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{
		BlobOverride:  "http://blob.example.com",
		QueueOverride: "http://queue.example.com",
		TableOverride: "http://table.example.com",
	}, nil)

	body, err := sp.Put(t.Context(), storageAccountID("acct1"), map[string]any{"location": "westus"})
	require.NoError(t, err)

	props, ok := body["properties"].(map[string]any)
	require.True(t, ok)
	endpoints, ok := props["primaryEndpoints"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "http://blob.example.com/acct1/", endpoints["blob"])
	assert.Equal(t, "http://queue.example.com/acct1/", endpoints["queue"])
	assert.Equal(t, "http://table.example.com/acct1/", endpoints["table"])
}

func TestStorageProvider_NamespaceAndResourceTypes(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)

	assert.Equal(t, "Microsoft.Storage", sp.Namespace())

	types := sp.ResourceTypes()
	require.Len(t, types, 1)
	assert.Equal(t, "storageAccounts", types[0].Type)
	assert.NotEmpty(t, types[0].APIVersions)
}
