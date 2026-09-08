package glacier

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// glacierSnapshotVersion identifies the shape of [backendSnapshot]. It must
// be bumped whenever a change to the registered tables (vaults, jobs,
// multipartUploads, vaultLocks -- see store_setup.go) or to the raw-map
// fields captured directly on backendSnapshot would make an older snapshot
// unsafe to decode as the current shape. Restore compares this against the
// persisted value and discards (rather than attempts to partially decode)
// any mismatch -- see Restore below. This guard, and the value itself
// (starting at 1), is new as of the Phase 3.3 pkgs/store conversion: no
// prior glacier snapshot carried a version field, so any snapshot taken
// before this conversion is -- correctly -- treated as incompatible on
// restore.
const glacierSnapshotVersion = 1

// multipartPartSnapshot captures the (still-raw) multipartParts map -- see
// store_setup.go's package doc for why it wasn't converted to a store.Table.
type multipartPartSnapshot struct {
	Key   uploadKey       `json:"key"`
	Parts []MultipartPart `json:"parts"`
}

// provisionedCapacitySnapshot captures the (still-raw) provisionedCapacity map.
type provisionedCapacitySnapshot struct {
	AccountID string                 `json:"accountID"`
	Caps      []*ProvisionedCapacity `json:"caps"`
}

// backendSnapshot is the top-level on-disk shape for the Glacier backend.
//
// Tables holds one JSON-encoded array per registered [store.Table] --
// "vaults", "jobs", "multipartUploads", "vaultLocks" -- produced directly by
// [store.Registry.SnapshotAll]. Every registered table's value type
// (Vault/Job/MultipartUpload/VaultLock) is fully JSON round-trippable with
// no non-serialisable live fields, so unlike services/sqs's DTO-registry
// pilot no separate ephemeral DTO registry is needed here: b.registry IS the
// table set persisted. MultipartParts, ProvisionedCapacity, and
// DataRetrievalPolicies remain plain maps (see store_setup.go) and are
// persisted directly alongside Tables, exactly as they were before this
// conversion.
type backendSnapshot struct {
	Tables                map[string]json.RawMessage    `json:"tables"`
	DataRetrievalPolicies map[string]string             `json:"dataRetrievalPolicies,omitempty"`
	MultipartParts        []multipartPartSnapshot       `json:"multipartParts"`
	ProvisionedCapacity   []provisionedCapacitySnapshot `json:"provisionedCapacity"`
	Version               int                           `json:"version"`
}

// Snapshot serialises the backend state to JSON.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		// The registered tables are plain JSON-friendly structs, so a marshal
		// failure here would indicate a programming error rather than bad
		// input data. Log and skip the snapshot rather than panic, matching
		// the persistence.Persistable contract (nil is skipped by the Manager).
		logger.Load(ctx).WarnContext(ctx, "glacier: snapshot table marshal failed", "error", err)

		return nil
	}

	snap := backendSnapshot{
		Version:             glacierSnapshotVersion,
		Tables:              tables,
		MultipartParts:      make([]multipartPartSnapshot, 0, len(b.multipartParts)),
		ProvisionedCapacity: make([]provisionedCapacitySnapshot, 0, len(b.provisionedCapacity)),
	}

	for k, parts := range b.multipartParts {
		snap.MultipartParts = append(snap.MultipartParts, multipartPartSnapshot{Key: k, Parts: parts})
	}

	for accountID, caps := range b.provisionedCapacity {
		snap.ProvisionedCapacity = append(snap.ProvisionedCapacity, provisionedCapacitySnapshot{
			AccountID: accountID,
			Caps:      caps,
		})
	}

	snap.DataRetrievalPolicies = make(map[string]string, len(b.dataRetrievalPolicies))
	maps.Copy(snap.DataRetrievalPolicies, b.dataRetrievalPolicies)

	return persistence.MarshalSnapshot(ctx, "glacier", snap)
}

// Restore loads backend state from a JSON snapshot.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "glacier", data, &snap); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if snap.Version != glacierSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption.
		logger.Load(ctx).WarnContext(ctx,
			"glacier: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", glacierSnapshotVersion)

		b.registry.ResetAll()
		b.multipartParts = make(map[uploadKey][]MultipartPart)
		b.multipartPartData = make(map[uploadKey]map[string][]byte)
		b.provisionedCapacity = make(map[string][]*ProvisionedCapacity)
		b.dataRetrievalPolicies = make(map[string]string)

		return nil
	}

	if err := b.registry.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("glacier: restore snapshot tables: %w", err)
	}

	b.multipartParts = make(map[uploadKey][]MultipartPart, len(snap.MultipartParts))
	for _, ps := range snap.MultipartParts {
		b.multipartParts[ps.Key] = ps.Parts
	}

	// multipartPartData (raw part bytes) is never persisted, like archiveData --
	// reset it so a restored backend doesn't carry over stale bytes from
	// whatever state it held before Restore was called.
	b.multipartPartData = make(map[uploadKey]map[string][]byte)

	b.provisionedCapacity = make(map[string][]*ProvisionedCapacity, len(snap.ProvisionedCapacity))
	for _, cs := range snap.ProvisionedCapacity {
		b.provisionedCapacity[cs.AccountID] = cs.Caps
	}

	if snap.DataRetrievalPolicies != nil {
		b.dataRetrievalPolicies = snap.DataRetrievalPolicies
	} else {
		b.dataRetrievalPolicies = make(map[string]string)
	}

	return nil
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	if mem, ok := h.Backend.(*InMemoryBackend); ok {
		return mem.Snapshot(ctx)
	}

	return nil
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	if mem, ok := h.Backend.(*InMemoryBackend); ok {
		return mem.Restore(ctx, data)
	}

	return nil
}
