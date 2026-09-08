package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

func TestSubscriptionBody(t *testing.T) {
	t.Parallel()

	body := azurearm.SubscriptionBody("sub1", "My Sub")

	assert.Equal(t, "/subscriptions/sub1", body["id"])
	assert.Equal(t, "sub1", body["subscriptionId"])
	assert.Equal(t, "My Sub", body["displayName"])
	assert.Equal(t, "Enabled", body["state"])
}

func TestTenantBody(t *testing.T) {
	t.Parallel()

	body := azurearm.TenantBody("tenant1")

	assert.Equal(t, "/tenants/tenant1", body["id"])
	assert.Equal(t, "tenant1", body["tenantId"])
	assert.NotEmpty(t, body["defaultDomain"])
}

func TestInMemoryBackend_ProviderRegistration(t *testing.T) {
	t.Parallel()

	b := azurearm.NewInMemoryBackend()

	assert.False(t, b.IsProviderRegistered("sub1", "Microsoft.Storage"))
	assert.Equal(t, "NotRegistered", b.ProviderRegistrationState("sub1", "Microsoft.Storage"))

	b.RegisterProvider("sub1", "Microsoft.Storage")

	assert.True(t, b.IsProviderRegistered("sub1", "Microsoft.Storage"))
	assert.Equal(t, "Registered", b.ProviderRegistrationState("sub1", "Microsoft.Storage"))
	assert.False(t, b.IsProviderRegistered("sub2", "Microsoft.Storage"), "registration is per-subscription")
}

func TestProviderBody(t *testing.T) {
	t.Parallel()

	types := []azurearm.ResourceTypeDef{{Type: "storageAccounts", APIVersions: []string{"2023-01-01"}}}
	body := azurearm.ProviderBody("sub1", "Microsoft.Storage", "Registered", types)

	assert.Equal(t, "/subscriptions/sub1/providers/Microsoft.Storage", body["id"])
	assert.Equal(t, "Microsoft.Storage", body["namespace"])
	assert.Equal(t, "Registered", body["registrationState"])

	resourceTypes, ok := body["resourceTypes"].([]map[string]any)
	if ok {
		if len(resourceTypes) > 0 {
			assert.Equal(t, "storageAccounts", resourceTypes[0]["resourceType"])
		}
	}
}
