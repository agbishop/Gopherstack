package azurearm

// This file exposes internals needed by the blackbox (azurearm_test) test
// suite, following this repo's export_test.go convention (see e.g.
// services/cosmosdb) so tests can stay in package azurearm_test rather than
// reaching for whitebox package azurearm.

// RegistryProviderNamespaces returns the namespaces of every registered
// dedicated ResourceProvider (test-only alias for Registry.Providers, kept
// separate in case Providers' own semantics change).
func (r *Registry) RegistryProviderNamespaces() []string {
	return r.Providers()
}

// NewTestRegistryWithStorage builds a Registry over a fresh
// InMemoryBackend with a StorageProvider registered, matching Provider.Init's
// wiring, for handler-level tests that don't need a full Provider/CLI.
func NewTestRegistryWithStorage() (*InMemoryBackend, *Registry) {
	backend := NewInMemoryBackend()
	registry := NewRegistry(backend)
	registry.Register(NewStorageProvider(StorageEndpointConfig{}, nil))

	return backend, registry
}
