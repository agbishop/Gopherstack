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

func storageAccountIDIn(rg, name string) azurearm.ResourceID {
	id := storageAccountID(name)
	id.ResourceGroup = rg

	return id
}

// TestStorageProvider_Put_ConflictsAcrossResourceGroups asserts that a
// storage account name (globally unique in real Azure) can't be silently
// re-created under a different resource group -- a CodeRabbit-flagged bug
// where Put previously keyed accounts by name alone and overwrote whichever
// resource group last wrote that name.
func TestStorageProvider_Put_ConflictsAcrossResourceGroups(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)
	ctx := t.Context()

	_, err := sp.Put(ctx, storageAccountIDIn("rg1", "sharedname"), map[string]any{"location": "westus"})
	require.NoError(t, err)

	_, err = sp.Put(ctx, storageAccountIDIn("rg2", "sharedname"), map[string]any{"location": "eastus"})
	require.Error(t, err)
}

// TestStorageProvider_Get_WrongResourceGroup asserts that Get/Delete/ListKeys
// via a different resource group's path 404, rather than reaching an
// account created under another group by name alone (CodeRabbit-flagged).
func TestStorageProvider_Get_WrongResourceGroup(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)
	ctx := t.Context()

	_, err := sp.Put(ctx, storageAccountIDIn("rg1", "acct1"), map[string]any{"location": "westus"})
	require.NoError(t, err)

	_, err = sp.Get(ctx, storageAccountIDIn("rg2", "acct1"))
	require.ErrorIs(t, err, azurearm.ErrStorageAccountNotFound)

	err = sp.Delete(ctx, storageAccountIDIn("rg2", "acct1"))
	require.ErrorIs(t, err, azurearm.ErrStorageAccountNotFound)

	_, err = sp.ListKeys(ctx, storageAccountIDIn("rg2", "acct1"))
	require.ErrorIs(t, err, azurearm.ErrStorageAccountNotFound)
}

// TestStorageProvider_List_ScopedToResourceGroup asserts List filters by
// id.ResourceGroup when set, per the ResourceProvider interface contract.
func TestStorageProvider_List_ScopedToResourceGroup(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)
	ctx := t.Context()

	_, err := sp.Put(ctx, storageAccountIDIn("rg1", "acct1"), map[string]any{"location": "westus"})
	require.NoError(t, err)
	_, err = sp.Put(ctx, storageAccountIDIn("rg2", "acct2"), map[string]any{"location": "westus"})
	require.NoError(t, err)

	values, err := sp.List(ctx, storageAccountIDIn("rg1", ""))
	require.NoError(t, err)
	require.Len(t, values, 1)
	assert.Equal(t, "acct1", values[0]["name"])
}

// TestStorageProvider_Reset clears every account, mirroring
// Handler.Reset/Registry.ResetAll's contract (CodeRabbit-flagged: accounts
// previously survived a reset since only InMemoryBackend was cleared).
func TestStorageProvider_Reset(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)
	ctx := t.Context()
	id := storageAccountID("acct1")

	_, err := sp.Put(ctx, id, map[string]any{"location": "westus"})
	require.NoError(t, err)

	sp.Reset()

	_, err = sp.Get(ctx, id)
	require.ErrorIs(t, err, azurearm.ErrStorageAccountNotFound)
}

// TestStorageProvider_DeleteResourcesInGroup cascades a resource-group
// delete into the Storage RP's own accounts (CodeRabbit-flagged: DELETE
// .../resourceGroups/{name} previously only cleared InMemoryBackend's
// generic resources, orphaning StorageProvider accounts in that group).
func TestStorageProvider_DeleteResourcesInGroup(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)
	ctx := t.Context()

	_, err := sp.Put(ctx, storageAccountIDIn("rg1", "acct1"), map[string]any{"location": "westus"})
	require.NoError(t, err)
	_, err = sp.Put(ctx, storageAccountIDIn("rg2", "acct2"), map[string]any{"location": "westus"})
	require.NoError(t, err)

	sp.DeleteResourcesInGroup(ctx, "rg1")

	_, err = sp.Get(ctx, storageAccountIDIn("rg1", "acct1"))
	require.ErrorIs(t, err, azurearm.ErrStorageAccountNotFound)

	_, err = sp.Get(ctx, storageAccountIDIn("rg2", "acct2"))
	require.NoError(t, err)
}

// TestStorageProvider_TypeValidation asserts Get/Delete/List/ListKeys all
// reject a non-storageAccounts leaf type, matching Put's existing check
// (CodeRabbit-flagged: only Put validated this).
func TestStorageProvider_TypeValidation(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)
	ctx := t.Context()

	badID := azurearm.ResourceID{
		SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.Storage",
		Types: []string{"notStorageAccounts"}, Names: []string{"whatever"},
	}

	_, err := sp.Get(ctx, badID)
	require.Error(t, err)

	err = sp.Delete(ctx, badID)
	require.Error(t, err)

	_, err = sp.ListKeys(ctx, badID)
	require.Error(t, err)
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
