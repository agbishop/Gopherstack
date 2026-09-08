package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

func TestResource_Body(t *testing.T) {
	t.Parallel()

	r := azurearm.Resource{
		ID: azurearm.ResourceID{
			SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.Foo",
			Types: []string{"bars"}, Names: []string{"b1"},
		},
		Location:   "westus",
		Tags:       map[string]string{"k": "v"},
		Properties: map[string]any{"custom": "value"},
		SKU:        map[string]any{"name": "Basic"},
		Kind:       "SomeKind",
	}

	body := r.Body()

	assert.Equal(t, "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Foo/bars/b1", body["id"])
	assert.Equal(t, "b1", body["name"])
	assert.Equal(t, "Microsoft.Foo/bars", body["type"])
	assert.Equal(t, "westus", body["location"])
	assert.Equal(t, map[string]string{"k": "v"}, body["tags"])
	assert.Equal(t, map[string]any{"name": "Basic"}, body["sku"])
	assert.Equal(t, "SomeKind", body["kind"])

	props, ok := body["properties"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "value", props["custom"])
	assert.Equal(t, "Succeeded", props["provisioningState"])
}

func TestResource_Body_NoTagsDefaultsToEmptyMap(t *testing.T) {
	t.Parallel()

	r := azurearm.Resource{
		ID: azurearm.ResourceID{
			SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.Foo",
			Types: []string{"bars"}, Names: []string{"b1"},
		},
	}

	body := r.Body()
	assert.Equal(t, map[string]string{}, body["tags"])
	assert.NotContains(t, body, "sku")
	assert.NotContains(t, body, "kind")
}

func TestWithRequestHostAndRequestHostFromContext(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	assert.Equal(t, "", azurearm.RequestHostFromContext(ctx))

	ctx = azurearm.WithRequestHost(ctx, "example.com")
	assert.Equal(t, "example.com", azurearm.RequestHostFromContext(ctx))
}
