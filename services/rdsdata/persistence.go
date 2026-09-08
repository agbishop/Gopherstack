package rdsdata

import (
	"context"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// rdsdataSnapshotVersion identifies the shape of [backendSnapshot]. It must
// be bumped whenever a change to Transaction or backendSnapshot itself would
// make an older snapshot unsafe to decode as the current shape. Restore
// compares this against the persisted value and discards (every collection
// reset to empty, not a partial decode) any mismatch -- including a
// pre-Phase-3.3 snapshot, which has no version field and so decodes as 0 --
// see Restore below.
const rdsdataSnapshotVersion = 1

// backendSnapshot is the top-level on-disk shape for the RDS Data backend.
//
// Transactions is nested per-region (region -> that region's deterministic
// [store.Table.Snapshot] slice), matching services/mediastore's
// region-nested-map pattern: the set of regions is only known at runtime, so
// b.transactions is NOT registered on a *store.Registry
// (Registry.SnapshotAll/RestoreAll require a fixed, construction-time-known
// table-name set) and is captured/restored directly here instead. See
// store_setup.go for why transactions is region-nested in the first place,
// and for why executedStatements and txCounter remain plain maps.
type backendSnapshot struct {
	Transactions       map[string][]*Transaction      `json:"transactions"`
	ExecutedStatements map[string][]ExecutedStatement `json:"executedStatements"`
	TxCounter          map[string]int                 `json:"txCounter"`
	AccountID          string                         `json:"accountID"`
	Region             string                         `json:"region"`
	Version            int                            `json:"version"`
}

// Snapshot serialises the backend state to JSON.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	txCopy := make(map[string][]*Transaction, len(b.transactions))
	for region, tbl := range b.transactions {
		txCopy[region] = tbl.Snapshot()
	}

	stmtsCopy := make(map[string][]ExecutedStatement, len(b.executedStatements))
	for region, stmts := range b.executedStatements {
		cp := make([]ExecutedStatement, len(stmts))
		copy(cp, stmts)
		stmtsCopy[region] = cp
	}

	counterCopy := make(map[string]int, len(b.txCounter))
	maps.Copy(counterCopy, b.txCounter)

	snap := backendSnapshot{
		Version:            rdsdataSnapshotVersion,
		Transactions:       txCopy,
		ExecutedStatements: stmtsCopy,
		TxCounter:          counterCopy,
		AccountID:          b.accountID,
		Region:             b.defaultRegion,
	}

	return persistence.MarshalSnapshot(ctx, "rdsdata", snap)
}

// Restore loads backend state from a JSON snapshot.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "rdsdata", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != rdsdataSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption.
		logger.Load(ctx).WarnContext(ctx,
			"rdsdata: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", rdsdataSnapshotVersion)

		b.transactions = make(map[string]*store.Table[Transaction])
		b.executedStatements = make(map[string][]ExecutedStatement)
		b.txCounter = make(map[string]int)

		if b.engine == nil {
			b.engine = newSQLEngine()
		}

		b.engine.reset()

		return nil
	}

	ensureNonNilMaps(&snap)
	backfillTransactionTimestamps(snap.Transactions)

	b.transactions = make(map[string]*store.Table[Transaction])
	for region, items := range snap.Transactions {
		b.transactionsStore(region).Restore(items)
	}

	b.executedStatements = snap.ExecutedStatements
	b.txCounter = snap.TxCounter
	b.accountID = snap.AccountID
	b.defaultRegion = snap.Region

	// Rebuild the SQL engine from the recorded statement log so table state
	// survives a snapshot/restore cycle. Best-effort: parameterised writes
	// (whose bound values are not persisted) and statements trimmed from the
	// capped log cannot be replayed.
	if b.engine == nil {
		b.engine = newSQLEngine()
	}

	b.engine.reset()

	for region, stmts := range b.executedStatements {
		b.engine.replay(ctx, region, stmts)
	}

	return nil
}

// backfillTransactionTimestamps stamps CreatedAt/LastActivityAt with now for
// any transaction restored from a pre-CreatedAt/LastActivityAt (v1) snapshot,
// where those fields decode as the zero time.Time. Without this, the Janitor
// (janitor.go) would immediately reap a freshly restored transaction as
// 24-hour-stale.
func backfillTransactionTimestamps(txsByRegion map[string][]*Transaction) {
	now := time.Now()

	for _, txs := range txsByRegion {
		for _, tx := range txs {
			if tx.CreatedAt.IsZero() {
				tx.CreatedAt = now
			}

			if tx.LastActivityAt.IsZero() {
				tx.LastActivityAt = now
			}
		}
	}
}

// ensureNonNilMaps initialises nil maps in the snapshot to empty maps.
func ensureNonNilMaps(snap *backendSnapshot) {
	if snap.Transactions == nil {
		snap.Transactions = make(map[string][]*Transaction)
	}

	if snap.ExecutedStatements == nil {
		snap.ExecutedStatements = make(map[string][]ExecutedStatement)
	}

	if snap.TxCounter == nil {
		snap.TxCounter = make(map[string]int)
	}
}

// Snapshot implements persistence.Persistable by delegating to the backend.
//
// Dead-wiring fix (Phase 3.3): InMemoryBackend already implemented
// Snapshot/Restore, but Handler never delegated to them, so cli.go's generic
// setupPersistence (which type-asserts the registered service.Registerable,
// i.e. the Handler, for a Snapshot/Restore pair) never picked RDSData up --
// the backend's persistence logic existed but was never invoked. This
// delegation (matching the mediastore/codecommit/cleanrooms pattern) wires
// RDSData into persistence for the first time.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	return h.Backend.Snapshot(ctx)
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	return h.Backend.Restore(ctx, data)
}
