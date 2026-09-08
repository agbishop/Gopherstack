package rdsdata //nolint:testpackage // needs access to the unexported region context key (see isolation_test.go).

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	persistTestAccountID = "000000000000"
	persistTestRegion    = "us-east-1"
)

// Test_Restore_InvalidData verifies that malformed JSON is reported as an
// error rather than silently discarded or partially applied.
func Test_Restore_InvalidData(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// Test_Restore_VersionMismatch verifies that a snapshot whose version doesn't
// match the current backend is discarded cleanly rather than partially
// decoded: every collection resets to empty and Restore returns no error.
func Test_Restore_VersionMismatch(t *testing.T) {
	t.Parallel()

	ctx := rdsdataCtxRegion(persistTestRegion)

	b := NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	_, err := b.BeginTransaction(ctx, "arn:aws:rds:us-east-1:000000000000:cluster:seed")
	require.NoError(t, err)

	// A syntactically valid but version-mismatched snapshot.
	err = b.Restore(t.Context(), []byte(`{"version":999,"transactions":{}}`))
	require.NoError(t, err)

	assert.Empty(t, b.ListTransactions(ctx))
	assert.Empty(t, b.ListExecutedStatements(ctx))
}

// Test_Restore_OldSnapshotDecodesAsZero verifies that a snapshot with no
// version field at all decodes with Version == 0, which mismatches
// rdsdataSnapshotVersion and is discarded the same way any other
// incompatible version is -- not partially applied.
func Test_Restore_OldSnapshotDecodesAsZero(t *testing.T) {
	t.Parallel()

	ctx := rdsdataCtxRegion(persistTestRegion)

	b := NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	_, err := b.BeginTransaction(ctx, "arn:aws:rds:us-east-1:000000000000:cluster:seed")
	require.NoError(t, err)

	err = b.Restore(t.Context(), []byte(`{"transactions":{}}`))
	require.NoError(t, err)

	assert.Empty(t, b.ListTransactions(ctx))
}

// regionPersistSeed is the state Test_SnapshotRestore_FullState seeds into
// one region, kept together so the post-restore assertions can refer back to
// what was originally set.
type regionPersistSeed struct {
	region string
	arn    string
	txID   string
	table  string
	sql    []string
}

// seedPersistRegion begins one transaction and executes two SQL statements
// (CREATE TABLE, then INSERT) against region under ctx, returning the state
// used so the caller can assert on it after a round-trip. The transaction is
// left open (uncommitted) so both the transaction and the statement log
// survive into the snapshot. table is unique per region (rather than derived
// from it) so two regions never collide on the same table name, even though
// each region's SQL runs against its own isolated engine database.
func seedPersistRegion(
	ctx context.Context, t *testing.T, b *InMemoryBackend, region, table string,
) regionPersistSeed {
	t.Helper()

	arn := "arn:aws:rds:" + region + ":000000000000:cluster:seed"

	txID, err := b.BeginTransaction(ctx, arn)
	require.NoError(t, err)

	create := "CREATE TABLE " + table + " (id INTEGER)"
	_, _, _, _, err = b.ExecuteStatement(ctx, arn, create, "")
	require.NoError(t, err)

	insert := "INSERT INTO " + table + " (id) VALUES (1)"
	_, _, _, _, err = b.ExecuteStatement(ctx, arn, insert, "")
	require.NoError(t, err)

	return regionPersistSeed{
		region: region,
		arn:    arn,
		txID:   txID,
		table:  table,
		sql:    []string{create, insert},
	}
}

// assertPersistRegionRestored verifies every field seedPersistRegion set for
// one region survived a Snapshot/Restore round-trip: the open transaction,
// the ordered statement log, and (via a fresh BeginTransaction) that the
// region's txCounter continued from where it left off rather than resetting.
func assertPersistRegionRestored(
	ctx context.Context, t *testing.T, b *InMemoryBackend, seed regionPersistSeed,
) {
	t.Helper()

	txns := b.ListTransactions(ctx)
	require.Len(t, txns, 1)
	assert.Contains(t, txns, seed.txID)

	stmts := b.ListExecutedStatements(ctx)
	require.Len(t, stmts, len(seed.sql))

	for i, sql := range seed.sql {
		assert.Equal(t, sql, stmts[i].SQL, "executed statement order must survive restore")
	}

	// txCounter must continue, not restart: the next transaction in this
	// region must be txn-000002, not a repeat of txn-000001.
	nextTxID, err := b.BeginTransaction(ctx, seed.arn)
	require.NoError(t, err)
	assert.Equal(t, "txn-000002", nextTxID)
}

// Test_SnapshotRestore_FullState is a full-state round-trip: it seeds an
// open transaction and an ordered statement log in two distinct regions,
// snapshots the backend, restores the snapshot into a fresh backend, and
// asserts every field survived -- including that region isolation is
// preserved (see store_setup.go for why transactions is region-nested) and
// that the restored SQL engine actually has the replayed table (proving
// executedStatements' order was preserved through the round-trip, since
// engine.replay depends on CREATE-then-INSERT ordering).
func Test_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	const (
		regionEast = "us-east-1"
		regionWest = "us-west-2"
	)

	ctxEast := rdsdataCtxRegion(regionEast)
	ctxWest := rdsdataCtxRegion(regionWest)

	b := NewInMemoryBackend(persistTestAccountID, regionEast)

	eastSeed := seedPersistRegion(ctxEast, t, b, regionEast, "t_east")
	westSeed := seedPersistRegion(ctxWest, t, b, regionWest, "t_west")

	data := b.Snapshot(t.Context())
	require.NotEmpty(t, data)

	restored := NewInMemoryBackend("", "")
	require.NoError(t, restored.Restore(t.Context(), data))

	assert.Equal(t, persistTestAccountID, restored.AccountID())
	assert.Equal(t, regionEast, restored.Region())

	assertPersistRegionRestored(ctxEast, t, restored, eastSeed)
	assertPersistRegionRestored(ctxWest, t, restored, westSeed)

	// The engine replay must have rebuilt each region's table from the
	// restored, order-preserved statement log: a SELECT against the table
	// created during seeding must see the row inserted right after it.
	records, _, _, _, err := restored.ExecuteStatement(ctxEast, eastSeed.arn, "SELECT id FROM "+eastSeed.table, "")
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.NotNil(t, records[0][0].LongValue)
	assert.Equal(t, int64(1), *records[0][0].LongValue)
}

// Test_SnapshotRestore_EmptyBackend verifies that snapshotting and restoring
// a backend with no transactions or statements at all round-trips cleanly to
// an empty backend, rather than erroring or leaving stale state.
func Test_SnapshotRestore_EmptyBackend(t *testing.T) {
	t.Parallel()

	ctx := rdsdataCtxRegion(persistTestRegion)

	b := NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	data := b.Snapshot(t.Context())
	require.NotNil(t, data)

	restored := NewInMemoryBackend("", "")
	require.NoError(t, restored.Restore(t.Context(), data))

	assert.Empty(t, restored.ListTransactions(ctx))
	assert.Empty(t, restored.ListExecutedStatements(ctx))
}

// Test_Restore_V1SnapshotBackfillsTransactionTimestamps verifies that a
// v1-shaped snapshot -- one predating Transaction.CreatedAt/LastActivityAt,
// so its transactions have neither key at all -- restores with both fields
// backfilled to non-zero, rather than reviving a transaction the Janitor
// (janitor.go) would immediately reap as 24-hour-stale.
func Test_Restore_V1SnapshotBackfillsTransactionTimestamps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{
			name: "single region",
			data: `{"version":1,"transactions":{"us-east-1":[` +
				`{"transactionId":"txn-000001","status":"active"}` +
				`]},"executedStatements":{},"txCounter":{"us-east-1":1},` +
				`"accountID":"000000000000","region":"us-east-1"}`,
		},
		{
			name: "multiple regions",
			data: `{"version":1,"transactions":{` +
				`"us-east-1":[{"transactionId":"txn-000001","status":"active"}],` +
				`"us-west-2":[{"transactionId":"txn-000001","status":"active"}]` +
				`},"executedStatements":{},"txCounter":{"us-east-1":1,"us-west-2":1},` +
				`"accountID":"000000000000","region":"us-east-1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewInMemoryBackend("", "")
			require.NoError(t, b.Restore(t.Context(), []byte(tt.data)))

			regions := []string{"us-east-1", "us-west-2"}
			for _, region := range regions {
				ctx := rdsdataCtxRegion(region)

				txns := b.ListTransactions(ctx)
				if _, seeded := txns["txn-000001"]; !seeded {
					continue
				}

				tx := txns["txn-000001"]
				assert.False(t, tx.CreatedAt.IsZero(), "%s: CreatedAt must be backfilled, not zero", region)
				assert.False(t, tx.LastActivityAt.IsZero(), "%s: LastActivityAt must be backfilled, not zero", region)
			}
		})
	}
}

// Test_Handler_SnapshotRestore verifies Handler.Snapshot/Restore delegate to
// the backend -- this is the dead-wiring fix: InMemoryBackend already
// implemented Snapshot/Restore, but Handler never delegated to them, so
// cli.go's generic setupPersistence (which type-asserts the registered
// service.Registerable, i.e. the Handler, for a Snapshot/Restore pair) never
// picked RDSData up.
func Test_Handler_SnapshotRestore(t *testing.T) {
	t.Parallel()

	ctx := rdsdataCtxRegion(persistTestRegion)

	backend := NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	h := NewHandler(backend)

	txID, err := backend.BeginTransaction(ctx, "arn:aws:rds:us-east-1:000000000000:cluster:handler")
	require.NoError(t, err)

	data := h.Snapshot(t.Context())
	require.NotEmpty(t, data)

	restoredBackend := NewInMemoryBackend("", "")
	restoredHandler := NewHandler(restoredBackend)
	require.NoError(t, restoredHandler.Restore(t.Context(), data))

	txns := restoredBackend.ListTransactions(ctx)
	require.Len(t, txns, 1)
	assert.Contains(t, txns, txID)
}
