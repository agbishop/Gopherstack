package azuretable

import "github.com/blackbirdworks/gopherstack/pkgs/odatatable"

// InMemoryBackend implements StorageBackend using an in-memory map. It is a
// re-exported alias onto pkgs/odatatable.InMemoryBackend (see interfaces.go's
// package doc comment): the engine itself -- including its Snapshot/Restore
// persistence methods (see persistence.go) -- now lives there so
// services/cosmosdb's Table API can construct its own independent instance
// of the exact same backend.
type InMemoryBackend = odatatable.InMemoryBackend

// azureTableLockMetricsLabel is this service's lockmetrics label, passed to
// odatatable.NewInMemoryBackend so Azure Table's lock-contention metrics stay
// distinguishable from services/cosmosdb's own Table API backend, which
// passes its own "cosmosdb" label -- both otherwise construct the exact same
// odatatable.InMemoryBackend type and would collide onto one shared
// "odatatable" metrics label if the engine picked its own label internally.
const azureTableLockMetricsLabel = "azuretable"

// NewInMemoryBackend creates a new empty InMemoryBackend.
func NewInMemoryBackend() *InMemoryBackend {
	return odatatable.NewInMemoryBackend(azureTableLockMetricsLabel, azureTableSnapshotVersion)
}
