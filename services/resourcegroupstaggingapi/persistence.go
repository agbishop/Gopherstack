package resourcegroupstaggingapi

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// resourcegroupstaggingapiSnapshotVersion identifies the shape of
// [backendSnapshot]. It must be bumped whenever a change to a DTO type, a
// registered table's value type, or backendSnapshot itself would make an
// older snapshot unsafe to decode as the current shape. Restore compares
// this against the persisted value and discards (registry.ResetAll, not a
// partial decode) any mismatch -- see Restore below. This is the first
// version to use the store.Registry-backed shape (the pre-Phase-3.3 shape
// had no version field at all, so it decodes as 0 here and is discarded the
// same way any other incompatible snapshot is).
const resourcegroupstaggingapiSnapshotVersion = 1

// reportStateSnapshot is a DTO used ONLY for Snapshot/Restore of the "dirty"
// reportStates table (see store_setup.go's file doc comment):
// reportCreationState.Region is tagged json:"-" on the live type (it exists
// solely so store.Table's keyFn has something to key on), so marshaling the
// live type directly would silently drop it and unmarshaling into it would
// leave it permanently empty, corrupting the table's primary key on restore.
// This DTO gives Region a real JSON tag instead, mirroring the technique
// services/codecommit uses for its own dirty tables.
//
// StartedAt mirrors reportCreationState.startedAt (also otherwise unexported
// and un-persisted): without it, a restored report's RUNNING->SUCCEEDED and
// staleness checks in DescribeReportCreation both compare against a zero
// time.Time, so every restored report immediately reports NO REPORT
// regardless of its real status. Persisted as RFC3339, the same pattern
// services/sagemaker's persistedCluster.CreationTime uses.
type reportStateSnapshot struct {
	Region     string `json:"region"`
	S3Location string `json:"s3Location"`
	StartDate  string `json:"startDate"`
	Status     string `json:"status"`
	StartedAt  string `json:"startedAt"`
}

func reportStateSnapshotKeyFn(v *reportStateSnapshot) string { return v.Region }

func toReportStateSnapshot(v *reportCreationState) *reportStateSnapshot {
	return &reportStateSnapshot{
		Region:     v.Region,
		S3Location: v.S3Location,
		StartDate:  v.StartDate,
		Status:     v.Status,
		StartedAt:  v.startedAt.Format(time.RFC3339Nano),
	}
}

func fromReportStateSnapshot(ctx context.Context, v *reportStateSnapshot) *reportCreationState {
	startedAt, err := time.Parse(time.RFC3339Nano, v.StartedAt)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx,
			"resourcegroupstaggingapi: failed to parse report startedAt, treating as zero time",
			"region", v.Region, "error", err)
	}

	return &reportCreationState{
		Region:     v.Region,
		S3Location: v.S3Location,
		StartDate:  v.StartDate,
		Status:     v.Status,
		startedAt:  startedAt,
	}
}

// buildPersistenceDTORegistry constructs the ephemeral DTO registry used by
// both Snapshot and Restore for the reportStates dirty table. It is built
// fresh on every call, mirroring services/codecommit's
// buildPersistenceDTORegistry.
func buildPersistenceDTORegistry() (*store.Registry, *store.Table[reportStateSnapshot]) {
	dtoReg := store.NewRegistry()
	reportStateDTOs := store.Register(dtoReg, "reportStates", store.New(reportStateSnapshotKeyFn))

	return dtoReg, reportStateDTOs
}

// backendSnapshot is the serializable form of InMemoryBackend state.
//
// Tables holds one JSON-encoded array per registered table name -- today
// just the DTO table built above for reportStates (see store_setup.go's
// registerAllTables doc for why it is "dirty" rather than registered
// directly on b.registry).
//
// Providers, taggers, and untaggers are runtime callbacks and cannot be
// serialized; they must be re-registered after a Restore. The per-region
// resource cache is likewise never persisted -- it is a rebuildable TTL
// cache, not part of durable state.
type backendSnapshot struct {
	Tables    map[string]json.RawMessage `json:"tables"`
	AccountID string                     `json:"accountID"`
	Region    string                     `json:"region"`
	Version   int                        `json:"version"`
}

// Snapshot serializes the backend state to JSON.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "resourcegroupstaggingapi: snapshot table marshal failed", "error", err)

		return nil
	}

	dtoReg, reportStateDTOs := buildPersistenceDTORegistry()

	for _, s := range b.reportStates.Snapshot() {
		reportStateDTOs.Put(toReportStateSnapshot(s))
	}

	dtoTables, err := dtoReg.SnapshotAll()
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "resourcegroupstaggingapi: snapshot DTO table marshal failed", "error", err)

		return nil
	}

	maps.Copy(tables, dtoTables)

	snap := backendSnapshot{
		Version:   resourcegroupstaggingapiSnapshotVersion,
		Tables:    tables,
		AccountID: b.accountID,
		Region:    b.defaultRegion,
	}

	return persistence.MarshalSnapshot(ctx, "resourcegroupstaggingapi", snap)
}

// Restore loads backend state from a JSON snapshot produced by Snapshot.
// Providers, taggers, and untaggers are runtime callbacks that cannot be
// serialized; they are always cleared by this call (regardless of whether
// the snapshot's version matches) and must be re-registered (e.g. via
// wireResourceGroupsTagging) afterward to re-enable cross-service tag
// operations. The per-region resource cache is likewise always invalidated.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "resourcegroupstaggingapi", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	b.providers = nil
	b.filteredProviders = nil
	b.taggers = nil
	b.untaggers = nil
	clear(b.caches)

	if snap.Version != resourcegroupstaggingapiSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption.
		logger.Load(ctx).WarnContext(ctx,
			"resourcegroupstaggingapi: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", resourcegroupstaggingapiSnapshotVersion)

		b.registry.ResetAll()
		b.reportStates.Reset()

		return nil
	}

	if err := b.registry.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("resourcegroupstaggingapi: restore snapshot tables: %w", err)
	}

	if err := b.restoreDirtyTables(ctx, snap.Tables); err != nil {
		return err
	}

	b.accountID = snap.AccountID
	b.defaultRegion = snap.Region

	return nil
}

// restoreDirtyTables rebuilds the reportStates dirty table (see
// store_setup.go's registerAllTables doc) from tables via the ephemeral DTO
// registry, factored out of Restore to keep Restore's own cognitive
// complexity low. Callers must hold b.mu.Lock.
func (b *InMemoryBackend) restoreDirtyTables(ctx context.Context, tables map[string]json.RawMessage) error {
	dtoReg, reportStateDTOs := buildPersistenceDTORegistry()
	if err := dtoReg.RestoreAll(tables); err != nil {
		return fmt.Errorf("resourcegroupstaggingapi: restore snapshot DTO tables: %w", err)
	}

	b.reportStates.Reset()
	for _, s := range reportStateDTOs.Snapshot() {
		b.reportStates.Put(fromReportStateSnapshot(ctx, s))
	}

	return nil
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	return h.Backend.Snapshot(ctx)
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	return h.Backend.Restore(ctx, data)
}
