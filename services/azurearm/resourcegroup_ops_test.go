package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

func TestInMemoryBackend_PutResourceGroup(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()

	rg, created := b.PutResourceGroup("rg1", "westus", map[string]string{"env": "dev"})
	assert.True(t, created)
	assert.Equal(t, "rg1", rg.Name)
	assert.Equal(t, "westus", rg.Location)

	rg2, created2 := b.PutResourceGroup("rg1", "", nil)
	assert.False(t, created2, "second PUT should be an update")
	assert.Equal(t, "westus", rg2.Location, "empty location on update preserves the existing one")
}

func TestInMemoryBackend_GetResourceGroup(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()
	_, _ = b.PutResourceGroup("RG1", "eastus", nil)

	tests := []struct {
		name        string
		lookup      string
		expectError bool
	}{
		{name: "exact case", lookup: "RG1"},
		{name: "case-insensitive lookup", lookup: "rg1"},
		{name: "not found", lookup: "missing", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rg, err := b.GetResourceGroup(tt.lookup)
			if tt.expectError {
				require.ErrorIs(t, err, azurearm.ErrResourceGroupNotFound)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "RG1", rg.Name)
		})
	}
}

func TestInMemoryBackend_DeleteResourceGroup(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()
	_, _ = b.PutResourceGroup("rg1", "eastus", nil)

	require.NoError(t, b.DeleteResourceGroup("RG1"), "delete should be case-insensitive")
	assert.False(t, b.ResourceGroupExists("rg1"))

	err := b.DeleteResourceGroup("rg1")
	require.ErrorIs(t, err, azurearm.ErrResourceGroupNotFound)
}

func TestInMemoryBackend_ListResourceGroups(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()
	_, _ = b.PutResourceGroup("zeta", "eastus", nil)
	_, _ = b.PutResourceGroup("alpha", "westus", nil)

	groups := b.ListResourceGroups()
	require.Len(t, groups, 2)
	assert.Equal(t, "alpha", groups[0].Name, "list should be sorted by name")
	assert.Equal(t, "zeta", groups[1].Name)
}

func TestInMemoryBackend_ResourceGroupExists(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()
	assert.False(t, b.ResourceGroupExists("rg1"))

	_, _ = b.PutResourceGroup("rg1", "eastus", nil)
	assert.True(t, b.ResourceGroupExists("rg1"))
	assert.True(t, b.ResourceGroupExists("RG1"), "existence check should be case-insensitive")
}

func TestResourceGroup_Body(t *testing.T) {
	t.Parallel()

	g := azurearm.ResourceGroup{Name: "rg1", Location: "westus", Tags: map[string]string{"k": "v"}}
	body := g.Body("sub1")

	assert.Equal(t, "/subscriptions/sub1/resourceGroups/rg1", body["id"])
	assert.Equal(t, "rg1", body["name"])
	assert.Equal(t, "Microsoft.Resources/resourceGroups", body["type"])
	assert.Equal(t, "westus", body["location"])
	assert.Equal(t, map[string]string{"k": "v"}, body["tags"])

	props, ok := body["properties"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Succeeded", props["provisioningState"])
}
