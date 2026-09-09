package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

func TestParseGenericResourcePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		wantID      azurearm.ResourceID
		expectError bool
	}{
		{
			name: "simple storage account, lowercase resourcegroups",
			path: "/subscriptions/sub1/resourcegroups/rg1/providers/Microsoft.Storage/storageAccounts/acct1",
			wantID: azurearm.ResourceID{
				SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.Storage",
				Types: []string{"storageAccounts"}, Names: []string{"acct1"},
			},
		},
		{
			name: "mixed-case resourceGroups accepted",
			path: "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/acct1",
			wantID: azurearm.ResourceID{
				SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.Storage",
				Types: []string{"storageAccounts"}, Names: []string{"acct1"},
			},
		},
		{
			name: "nested child resource type",
			path: "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.ServiceBus/namespaces/ns1/queues/q1",
			wantID: azurearm.ResourceID{
				SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.ServiceBus",
				Types: []string{"namespaces", "queues"}, Names: []string{"ns1", "q1"},
			},
		},
		{
			name: "doubly nested child resource type",
			path: "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.DocumentDB/" +
				"databaseAccounts/acct1/sqlDatabases/db1/containers/c1",
			wantID: azurearm.ResourceID{
				SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.DocumentDB",
				Types: []string{"databaseAccounts", "sqlDatabases", "containers"},
				Names: []string{"acct1", "db1", "c1"},
			},
		},
		{name: "too short", path: "/subscriptions/sub1/resourceGroups/rg1", expectError: true},
		{name: "not subscriptions", path: "/subscription/sub1/resourceGroups/rg1/providers/ns/t/n", expectError: true},
		{
			name:        "not providers segment",
			path:        "/subscriptions/sub1/resourceGroups/rg1/provider/ns/t/n",
			expectError: true,
		},
		{
			name:        "odd number of trailing segments",
			path:        "/subscriptions/sub1/resourceGroups/rg1/providers/ns/t1/n1/t2",
			expectError: true,
		},
		{name: "empty path", path: "", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			id, err := azurearm.ParseGenericResourcePath(tt.path)
			if tt.expectError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantID, id)
		})
	}
}

func TestResourceID_ResourceTypeAndLeaf(t *testing.T) {
	t.Parallel()

	id, err := azurearm.ParseGenericResourcePath(
		"/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.ServiceBus/namespaces/ns1/queues/q1")
	require.NoError(t, err)

	assert.Equal(t, "Microsoft.ServiceBus/namespaces/queues", id.ResourceType())
	assert.Equal(t, "queues", id.LeafType())
	assert.Equal(t, "q1", id.LeafName())
}

func TestResourceID_ARMID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		wantID string
		id     azurearm.ResourceID
	}{
		{
			name: "single type/name",
			id: azurearm.ResourceID{
				SubscriptionID: "sub1", ResourceGroup: "rg1", Namespace: "Microsoft.Storage",
				Types: []string{"storageAccounts"}, Names: []string{"acct1"},
			},
			wantID: "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/acct1",
		},
		{
			name: "canonicalizes resourceGroups casing regardless of parse-time casing",
			id: azurearm.ResourceID{
				SubscriptionID: "sub1", ResourceGroup: "RG1", Namespace: "Microsoft.Storage",
				Types: []string{"storageAccounts"}, Names: []string{"acct1"},
			},
			wantID: "/subscriptions/sub1/resourceGroups/RG1/providers/Microsoft.Storage/storageAccounts/acct1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.wantID, tt.id.ARMID())
		})
	}
}

func TestParseGenericResourceListPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		path      string
		wantNS    string
		wantType  string
		wantHasRG bool
		expectErr bool
	}{
		{
			name:      "subscription-scoped list",
			path:      "/subscriptions/sub1/providers/Microsoft.Storage/storageAccounts",
			wantHasRG: false, wantNS: "Microsoft.Storage", wantType: "storageAccounts",
		},
		{
			name:      "resource-group-scoped list",
			path:      "/subscriptions/sub1/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts",
			wantHasRG: true, wantNS: "Microsoft.Storage", wantType: "storageAccounts",
		},
		{name: "malformed", path: "/subscriptions/sub1/providers", expectErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			id, hasRG, err := azurearm.ParseGenericResourceListPath(tt.path)
			if tt.expectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantHasRG, hasRG)
			assert.Equal(t, tt.wantNS, id.Namespace)
			assert.Equal(t, []string{tt.wantType}, id.Types)
		})
	}
}
