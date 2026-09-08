package odatatable

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// snapshotVersion identifies the shape of backendSnapshot. Must be bumped
// whenever a change to storedTable/storedEntity would make an older
// snapshot unsafe to decode as the current shape; Restore compares this
// against the persisted value and discards (rather than partially decodes)
// any mismatch, mirroring services/azurequeue and services/azureblob.
//
// This is the same value (2) services/azuretable's own
// azureTableSnapshotVersion carried before this package existed: the shape
// hasn't changed, only its package.
const snapshotVersion = 2

// backendSnapshot is the top-level on-disk shape for an InMemoryBackend.
// Tables serialises directly (no DTO layer): storedTable/storedEntity have
// no unexported fields, so encoding/json round-trips them as-is
// (EntityProperty's own MarshalJSON/UnmarshalJSON handle its typed Value
// field -- see models.go).
type backendSnapshot struct {
	Tables  map[string]*storedTable `json:"tables"`
	Version int                     `json:"version"`
}

// Snapshot serialises the backend state to JSON. It implements
// persistence.Persistable, so an InMemoryBackend can be registered directly
// as a snapshot participant, or delegated to by a caller's own Handler-level
// Snapshot method (see services/azuretable/persistence.go and
// services/cosmosdb/persistence.go for that delegation pattern).
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	snap := backendSnapshot{
		Version: snapshotVersion,
		Tables:  b.tables,
	}

	return persistence.MarshalSnapshot(ctx, "odatatable", snap)
}

// Restore loads backend state from a JSON snapshot. It implements
// persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "odatatable", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != snapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- discard cleanly and start
		// empty instead of erroring, since this is an expected, recoverable
		// condition (e.g. upgrading gopherstack across a snapshot-format
		// change), not data corruption. Mirrors services/azurequeue and
		// services/azureblob.
		logger.Load(ctx).WarnContext(ctx,
			"odatatable: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", snapshotVersion)

		b.tables = make(map[string]*storedTable)

		return nil
	}

	if snap.Tables == nil {
		snap.Tables = make(map[string]*storedTable)
	}

	if err := validateSnapshotTables(snap.Tables); err != nil {
		return err
	}

	b.tables = snap.Tables

	return nil
}

// validateSnapshotTables rejects a snapshot whose "tables" map (or any
// table's "Entities" map) holds a JSON null entry -- which decodes to a nil
// pointer that would panic on first dereference if stored as-is -- or whose
// map key disagrees with the entry's own Name field, mirroring
// services/azurequeue's identical Restore validation. It also initializes
// any table whose own "Entities" map is JSON `null` (legal JSON, decodes to
// a nil Go map, not a nil pointer -- so it isn't rejected above) to an empty
// map: a nil map is safe to range over and read from, but assigning into
// one (as InsertEntity/ReplaceEntity/MergeEntity all do) panics. Mirrors the
// same nil-map init this function's caller already does for a nil top-level
// "tables" map.
func validateSnapshotTables(tables map[string]*storedTable) error {
	for name, t := range tables {
		if t == nil {
			return fmt.Errorf("%w: %q", ErrSnapshotTableNull, name)
		}

		if t.Name != name {
			return fmt.Errorf("%w: map key %q, Name %q", ErrSnapshotTableNameMismatch, name, t.Name)
		}

		for key, e := range t.Entities {
			if e == nil {
				return fmt.Errorf("%w: key %v in table %q", ErrSnapshotEntityNull, key, name)
			}
		}

		if t.Entities == nil {
			t.Entities = make(map[entityCompositeKey]*storedEntity)
		}
	}

	return nil
}
