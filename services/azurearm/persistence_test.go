package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

func TestInMemoryBackend_SnapshotRestoreRoundTrip(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()
	_, _ = b.PutResourceGroup("rg1", "westus", map[string]string{"env": "dev"})
	b.RegisterProvider("sub1", "Microsoft.Storage")

	registry := azurearm.NewRegistry(b)
	_, _, err := registry.Put(t.Context(), azurearm.ResourceID{
		SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.SomeFutureThing",
		Types: []string{"widgets"}, Names: []string{"w1"},
	}, map[string]any{"location": "westus"})
	require.NoError(t, err)

	data := b.Snapshot(t.Context())
	require.NotEmpty(t, data)

	restored := azurearm.NewInMemoryBackend()
	require.NoError(t, restored.Restore(t.Context(), data))

	rg, err := restored.GetResourceGroup("rg1")
	require.NoError(t, err)
	assert.Equal(t, "westus", rg.Location)

	assert.True(t, restored.IsProviderRegistered("sub1", "Microsoft.Storage"))

	restoredRegistry := azurearm.NewRegistry(restored)
	_, err = restoredRegistry.Get(t.Context(), azurearm.ResourceID{
		SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.SomeFutureThing",
		Types: []string{"widgets"}, Names: []string{"w1"},
	})
	require.NoError(t, err)
}

func TestInMemoryBackend_Restore_EmptySnapshot(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()
	require.NoError(t, b.Restore(t.Context(), []byte(`{"version":1}`)))
	assert.Empty(t, b.ListResourceGroups())
}

func TestInMemoryBackend_Restore_IncompatibleVersionStartsEmpty(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()
	_, _ = b.PutResourceGroup("rg1", "westus", nil)

	require.NoError(t, b.Restore(t.Context(), []byte(`{"version":999}`)))
	assert.Empty(t, b.ListResourceGroups(), "an incompatible version should discard state and start empty")
}

func TestHandler_SnapshotRestore(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	sub := h.Settings.SubscriptionID

	_, _ = doRequest(t, h, "PUT", "/subscriptions/"+sub+"/resourcegroups/rg1", []byte(`{"location":"westus"}`))

	data := h.Snapshot(t.Context())
	require.NotEmpty(t, data)

	h2 := newTestHandler(t)
	require.NoError(t, h2.Restore(t.Context(), data))

	status, body := doRequest(t, h2, "GET", "/subscriptions/"+sub+"/resourcegroups/rg1", nil)
	require.Equal(t, 200, status)
	assert.Equal(t, "rg1", body["name"])
}
