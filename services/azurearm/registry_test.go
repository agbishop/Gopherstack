package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

// futureNamespaceID builds a ResourceID for a namespace with no registered
// ResourceProvider, exercising the generic pass-through path (AZURE.md
// section 10.1's requirement that every namespace, not just ones with a
// dedicated RP, round-trips PUT/GET/DELETE).
func futureNamespaceID(name string) azurearm.ResourceID {
	return azurearm.ResourceID{
		SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.SomeFutureThing",
		Types: []string{"widgets"}, Names: []string{name},
	}
}

func TestRegistry_GenericResourcePassthrough(t *testing.T) {
	t.Parallel()

	_, registry := azurearm.NewTestRegistryWithStorage()
	id := futureNamespaceID("w1")
	ctx := t.Context()

	body, created, err := registry.Put(ctx, id, map[string]any{
		"location": "westus",
		"tags":     map[string]any{"k": "v"},
	})
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, id.ARMID(), body["id"])
	assert.Equal(t, "w1", body["name"])
	assert.Equal(t, "Microsoft.SomeFutureThing/widgets", body["type"])
	assert.Equal(t, "westus", body["location"])

	_, updated, err := registry.Put(ctx, id, map[string]any{"location": "eastus"})
	require.NoError(t, err)
	assert.False(t, updated, "second PUT of the same resource is an update, not a create")

	got, err := registry.Get(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "eastus", got["location"])

	require.NoError(t, registry.Delete(ctx, id))

	_, err = registry.Get(ctx, id)
	require.ErrorIs(t, err, azurearm.ErrResourceNotFound)
}

func TestRegistry_GenericResourceList(t *testing.T) {
	t.Parallel()

	_, registry := azurearm.NewTestRegistryWithStorage()
	ctx := t.Context()

	_, _, err := registry.Put(ctx, futureNamespaceID("w1"), map[string]any{"location": "westus"})
	require.NoError(t, err)
	_, _, err = registry.Put(ctx, futureNamespaceID("w2"), map[string]any{"location": "westus"})
	require.NoError(t, err)

	listID := azurearm.ResourceID{
		SubscriptionID: "sub1", ResourceGroup: "rg1",
		Namespace: "Microsoft.SomeFutureThing", Types: []string{"widgets"},
	}

	values, err := registry.List(ctx, listID)
	require.NoError(t, err)
	assert.Len(t, values, 2)
}

func TestRegistry_DispatchesToRegisteredProvider(t *testing.T) {
	t.Parallel()

	_, registry := azurearm.NewTestRegistryWithStorage()
	ctx := t.Context()

	id := azurearm.ResourceID{
		SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.Storage",
		Types: []string{"storageAccounts"}, Names: []string{"acct1"},
	}

	body, created, err := registry.Put(ctx, id, map[string]any{"location": "westus"})
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, "StorageV2", body["kind"], "Storage RP's own response shape should be used, not the generic one")

	assert.Contains(t, registry.RegistryProviderNamespaces(), "Microsoft.Storage")
}

func TestRegistry_ListKeys_UnregisteredNamespace(t *testing.T) {
	t.Parallel()

	_, registry := azurearm.NewTestRegistryWithStorage()

	_, err := registry.ListKeys(t.Context(), futureNamespaceID("w1"))
	require.ErrorIs(t, err, azurearm.ErrResourceNotFound)
}
