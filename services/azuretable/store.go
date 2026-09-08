package azuretable

import "github.com/blackbirdworks/gopherstack/pkgs/odatatable"

// InMemoryBackend implements StorageBackend using an in-memory map. It is a
// re-exported alias onto pkgs/odatatable.InMemoryBackend (see interfaces.go's
// package doc comment): the engine itself -- including its Snapshot/Restore
// persistence methods (see persistence.go) -- now lives there so
// services/cosmosdb's Table API can construct its own independent instance
// of the exact same backend.
type InMemoryBackend = odatatable.InMemoryBackend

// NewInMemoryBackend creates a new empty InMemoryBackend.
func NewInMemoryBackend() *InMemoryBackend {
	return odatatable.NewInMemoryBackend()
}
