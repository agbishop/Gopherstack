package networkmonitor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// networkmonitorSnapshotVersion identifies the shape of [backendSnapshot]. It
// must be bumped whenever a change to a registered table's value type or
// backendSnapshot itself would make an older snapshot unsafe to decode as the
// current shape. Restore compares this against the persisted value and
// discards (registry.ResetAll, not a partial decode) any mismatch -- see
// Restore below. This is the first version: the pre-Phase-3.3 snapshot shape
// (`{"monitors": {...}, "next_probe_seq": N}`, region-nested maps, no version
// field at all) also fails this check and is discarded rather than misread,
// matching the safe behaviour across any snapshot-format change.
const networkmonitorSnapshotVersion = 1

// backendSnapshot is the top-level on-disk shape for the Network Monitor
// backend. Tables holds one JSON-encoded array per registered store.Table
// (just "monitors"), produced by [store.Registry.SnapshotAll].
type backendSnapshot struct {
	Tables       map[string]json.RawMessage `json:"tables"`
	NextProbeSeq int64                      `json:"next_probe_seq"`
	Version      int                        `json:"version"`
}

// Snapshot serialises the backend state to JSON.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "networkmonitor: snapshot table marshal failed", "error", err)

		return nil
	}

	snap := backendSnapshot{
		Version:      networkmonitorSnapshotVersion,
		Tables:       tables,
		NextProbeSeq: b.nextProbeSeq,
	}

	return persistence.MarshalSnapshot(ctx, "networkmonitor", snap)
}

// Restore loads backend state from a JSON snapshot.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "networkmonitor", data, &snap); err != nil {
		return err
	}

	if b.mu == nil {
		b.mu = lockmetrics.New("networkmonitor")
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != networkmonitorSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption.
		logger.Load(ctx).WarnContext(ctx,
			"networkmonitor: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", networkmonitorSnapshotVersion)

		b.registry.ResetAll()
		b.nextProbeSeq = 0

		return nil
	}

	if err := b.registry.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("networkmonitor: restore snapshot tables: %w", err)
	}

	b.nextProbeSeq = snap.NextProbeSeq

	return nil
}
