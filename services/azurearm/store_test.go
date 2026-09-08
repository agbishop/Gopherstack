package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

func TestInMemoryBackend_Reset(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()
	_, _ = b.PutResourceGroup("rg1", "westus", nil)
	b.RegisterProvider("sub1", "Microsoft.Storage")

	registry := azurearm.NewRegistry(b)
	_, _, err := registry.Put(t.Context(), azurearm.ResourceID{
		SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.SomeFutureThing",
		Types: []string{"widgets"}, Names: []string{"w1"},
	}, map[string]any{"location": "westus"})
	require.NoError(t, err)

	b.Reset()

	assert.Empty(t, b.ListResourceGroups())
	assert.False(t, b.IsProviderRegistered("sub1", "Microsoft.Storage"))

	_, err = registry.Get(t.Context(), azurearm.ResourceID{
		SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.SomeFutureThing",
		Types: []string{"widgets"}, Names: []string{"w1"},
	})
	require.ErrorIs(t, err, azurearm.ErrResourceNotFound)
}

func TestNewInMemoryBackend(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()
	require.NotNil(t, b)
	assert.Empty(t, b.ListResourceGroups())
}
