package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

// TestStorageProvider_NilDataPlaneUsesNoop asserts NewStorageProvider(nil)
// falls back to a nil-safe no-op StorageAccounts implementation that never
// errors, so a real adapter isn't required for the Storage RP to function
// (AZURE.md section 10.6's interfaces.go contract).
func TestStorageProvider_NilDataPlaneUsesNoop(t *testing.T) {
	t.Parallel()

	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, nil)

	_, err := sp.Put(t.Context(), storageAccountID("acct1"), map[string]any{"location": "westus"})
	require.NoError(t, err)

	err = sp.Delete(t.Context(), storageAccountID("acct1"))
	require.NoError(t, err)
}

type fakeStorageAccounts struct {
	registered []string
	deleted    []string
}

func (f *fakeStorageAccounts) RegisterAccount(name string) error {
	f.registered = append(f.registered, name)

	return nil
}

func (f *fakeStorageAccounts) DeleteAccount(name string) error {
	f.deleted = append(f.deleted, name)

	return nil
}

// TestStorageProvider_CallsDataPlaneAdapter asserts a real StorageAccounts
// adapter is invoked on create/delete, the forward-compat seam M10's
// per-account namespacing is expected to use.
func TestStorageProvider_CallsDataPlaneAdapter(t *testing.T) {
	t.Parallel()

	fake := &fakeStorageAccounts{}
	sp := azurearm.NewStorageProvider(azurearm.StorageEndpointConfig{}, fake)

	_, err := sp.Put(t.Context(), storageAccountID("acct1"), map[string]any{"location": "westus"})
	require.NoError(t, err)
	assert.Equal(t, []string{"acct1"}, fake.registered)

	// Updating an existing account should not re-register it.
	_, err = sp.Put(t.Context(), storageAccountID("acct1"), map[string]any{"location": "eastus"})
	require.NoError(t, err)
	assert.Equal(t, []string{"acct1"}, fake.registered, "update should not call RegisterAccount again")

	require.NoError(t, sp.Delete(t.Context(), storageAccountID("acct1")))
	assert.Equal(t, []string{"acct1"}, fake.deleted)
}
