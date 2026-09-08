// Package appsync exposes unexported functions for use in external test packages.
// These exports exist solely for testing and must not be called from production code.
package appsync

import "encoding/json"

// RenderVTL is the exported test hook for the VTL template renderer.
// It is only exposed for package-level tests and should not be used in production code.
func RenderVTL(tmpl string, args map[string]any, result any) (string, error) {
	return renderVTL(tmpl, args, result)
}

// ToDynamoDBJSON is the exported test hook for the DynamoDB JSON formatter.
// It is only exposed for package-level tests and should not be used in production code.
func ToDynamoDBJSON(val any) string {
	return toDynamoDBJSON(val)
}

// SnapshotRegistry is the exported test hook for the backend's internal
// store.Registry.SnapshotAll. It exists solely so appsync_test can exercise
// the Phase 3.3 pkgs/store registry round trip in isolation -- i.e. without
// the explicit apiKeys handling, version guard, and Tags-close housekeeping
// that the production persistence.go's Snapshot/Restore layer adds on top --
// without needing an unexported field accessor.
func (b *InMemoryBackend) SnapshotRegistry() (map[string]json.RawMessage, error) {
	return b.registry.SnapshotAll()
}

// RestoreRegistry is the exported test hook for the backend's internal
// store.Registry.RestoreAll. See SnapshotRegistry.
func (b *InMemoryBackend) RestoreRegistry(data map[string]json.RawMessage) error {
	return b.registry.RestoreAll(data)
}

// SetAPIKeyExpiry directly overwrites an API key's Expires field, bypassing
// Create/UpdateAPIKey's 1-365-day expiry bounds. Real AWS's
// ApiKeyValidityOutOfBoundsException makes it impossible to create an
// already-expired key through the public API, but tests still need one to
// exercise ListAPIKeys/SweepExpiredAPIKeys' expiry filtering without waiting
// for a key to age out.
func (b *InMemoryBackend) SetAPIKeyExpiry(apiID, keyID string, expires int64) {
	b.mu.Lock("SetAPIKeyExpiry")
	defer b.mu.Unlock()

	if k := b.apiKeys[apiID][keyID]; k != nil {
		k.Expires = expires
	}
}
