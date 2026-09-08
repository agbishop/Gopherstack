package azurearm

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// armSnapshotVersion identifies the shape of armSnapshot. Must be bumped
// whenever a change to ResourceGroup/Resource would make an older snapshot
// unsafe to decode as the current shape; Restore discards (rather than
// partially decodes) any mismatch, mirroring services/cosmosdb.
const armSnapshotVersion = 1

// armSnapshot is the top-level on-disk shape for the ARM backend. It stores
// ONLY ARM-side metadata (resource groups, generic resources, provider
// registration state) -- never a second copy of any data-plane service's own
// state (AZURE.md section 10.9). Storage accounts (owned by rp_storage.go's
// StorageProvider, not InMemoryBackend) are snapshotted separately by
// Handler.Snapshot/Restore, which also persists the registry's Storage RP
// state.
type armSnapshot struct {
	ResourceGroups      map[string]*ResourceGroup  `json:"resourceGroups"`
	Resources           map[string]*Resource       `json:"resources"`
	RegisteredProviders map[string]map[string]bool `json:"registeredProviders"`
	Version             int                        `json:"version"`
}

// Snapshot serializes the backend state to JSON. It implements
// persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	snap := armSnapshot{
		Version:             armSnapshotVersion,
		ResourceGroups:      b.resourceGroups,
		Resources:           b.resources,
		RegisteredProviders: b.registeredProviders,
	}

	return persistence.MarshalSnapshot(ctx, "azurearm", snap)
}

// Restore loads backend state from a JSON snapshot. It implements
// persistence.Persistable. Restore must be idempotent against the data
// plane (AZURE.md section 10.9): this method only ever restores ARM's own
// metadata, never re-creates or otherwise touches any data-plane service's
// state.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap armSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "azurearm", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != armSnapshotVersion {
		logger.Load(ctx).WarnContext(ctx,
			"azurearm: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", armSnapshotVersion)

		b.resourceGroups = make(map[string]*ResourceGroup)
		b.resources = make(map[string]*Resource)
		b.registeredProviders = make(map[string]map[string]bool)

		return nil
	}

	if snap.ResourceGroups == nil {
		snap.ResourceGroups = make(map[string]*ResourceGroup)
	}

	if snap.Resources == nil {
		snap.Resources = make(map[string]*Resource)
	}

	if snap.RegisteredProviders == nil {
		snap.RegisteredProviders = make(map[string]map[string]bool)
	}

	if err := validateSnapshotResourceGroups(snap.ResourceGroups); err != nil {
		return err
	}

	if err := validateSnapshotResources(snap.Resources); err != nil {
		return err
	}

	b.resourceGroups = snap.ResourceGroups
	b.resources = snap.Resources
	b.registeredProviders = snap.RegisteredProviders

	return nil
}

// validateSnapshotResourceGroups rejects a snapshot whose "resourceGroups"
// map holds a JSON null entry, which decodes to a nil pointer that would
// panic on first dereference if stored as-is (same class of bug
// services/cosmosdb's persistence.go guards against).
func validateSnapshotResourceGroups(groups map[string]*ResourceGroup) error {
	for name, g := range groups {
		if g == nil {
			return fmt.Errorf("%w: %q", ErrSnapshotResourceGroupNull, name)
		}
	}

	return nil
}

func validateSnapshotResources(resources map[string]*Resource) error {
	for key, r := range resources {
		if r == nil {
			return fmt.Errorf("%w: %q", ErrSnapshotResourceNull, key)
		}
	}

	return nil
}

// Snapshot implements persistence.Persistable by delegating to the backend
// and the registry's Storage RP.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	return h.Backend.Snapshot(ctx)
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	if err := h.Backend.Restore(ctx, data); err != nil {
		return fmt.Errorf("azurearm: restore snapshot: %w", err)
	}

	return nil
}
