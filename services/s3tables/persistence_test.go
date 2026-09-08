package s3tables_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/s3tables"
)

const (
	persistTestAccountID = "000000000000"
	persistTestRegion    = "us-east-1"
)

// persistTestIDs holds every identifier newFullyPopulatedBackend generates,
// so a restored backend's data can be looked back up by the tests below.
type persistTestIDs struct {
	bucketARN string
	tableARN  string
}

// newFullyPopulatedBackend creates a backend with one populated entry in
// every store.Table (tableBuckets, namespaces, tables, bucketReplication,
// tableReplication, tableRecordExpiry -- see store_setup.go's
// registerAllTables) plus the one raw map persistence.go persists directly
// (tags), so a Snapshot from it exercises the entire persisted surface of
// the backend. S3 Tables had a Snapshot/Restore pair on InMemoryBackend
// before this conversion but no Handler delegation (dead wiring -- see
// persistence.go's Handler.Snapshot doc comment), so nothing here was ever
// actually persisted in practice.
func newFullyPopulatedBackend(t *testing.T) (*s3tables.InMemoryBackend, persistTestIDs) {
	t.Helper()

	b := s3tables.NewInMemoryBackend(persistTestAccountID, persistTestRegion)

	tb, err := b.CreateTableBucket("acme-bucket", s3tables.CreateTableBucketOptions{})
	require.NoError(t, err)

	_, err = b.CreateNamespace(tb.ARN, []string{"acme_ns"})
	require.NoError(t, err)

	table, err := b.CreateTable(tb.ARN, []string{"acme_ns"}, "acme_table", "ICEBERG", s3tables.CreateTableOptions{})
	require.NoError(t, err)

	_, err = b.PutTableBucketReplication(tb.ARN, "arn:aws:iam::000000000000:role/repl",
		[]s3tables.ReplicationRule{
			{Destinations: []s3tables.ReplicationDestination{
				{DestinationTableBucketARN: "arn:aws:s3tables:us-west-2:000000000000:bucket/dest"},
			}},
		}, "")
	require.NoError(t, err)

	require.NoError(t, b.PutTableRecordExpirationConfiguration(table.ARN, &s3tables.TableRecordExpiryConfig{
		Status: "enabled",
	}))

	_, err = b.PutTableReplication(table.ARN, "arn:aws:iam::000000000000:role/repl", nil, "")
	require.NoError(t, err)
	require.NoError(t, b.TagResource(table.ARN, map[string]string{"env": "test"}))

	return b, persistTestIDs{bucketARN: tb.ARN, tableARN: table.ARN}
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a full
// Snapshot->Restore round trip across every store.Table (tableBuckets,
// namespaces, tables, bucketReplication, tableReplication,
// tableRecordExpiry) and the one raw map (tags) persistence.go persists.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original, ids := newFullyPopulatedBackend(t)
	ctx := t.Context()

	snap := original.Snapshot(ctx)
	require.NotNil(t, snap)

	fresh := s3tables.NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	require.NoError(t, fresh.Restore(ctx, snap))

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{name: "tableBuckets table", run: func(t *testing.T) {
			t.Helper()

			tb, err := fresh.GetTableBucket(ids.bucketARN)
			require.NoError(t, err)
			assert.Equal(t, "acme-bucket", tb.Name)
		}},
		{name: "namespaces table", run: func(t *testing.T) {
			t.Helper()

			ns, err := fresh.GetNamespace(ids.bucketARN, []string{"acme_ns"})
			require.NoError(t, err)
			assert.Equal(t, []string{"acme_ns"}, ns.Namespace)

			pg, err := fresh.ListNamespaces(ids.bucketARN, s3tables.ListNamespacesParams{})
			require.NoError(t, err)
			require.Len(t, pg.Data, 1)
		}},
		{name: "tables table (primary + byComposite + byBucket indexes)", run: func(t *testing.T) {
			t.Helper()

			table, err := fresh.GetTable(ids.bucketARN, []string{"acme_ns"}, "acme_table")
			require.NoError(t, err)
			assert.Equal(t, ids.tableARN, table.ARN)

			pg, err := fresh.ListTables(ids.bucketARN, "", s3tables.ListTablesParams{})
			require.NoError(t, err)
			require.Len(t, pg.Data, 1)
			assert.Equal(t, "acme_table", pg.Data[0].Name)
		}},
		{name: "bucketReplication table", run: func(t *testing.T) {
			t.Helper()

			cfg, err := fresh.GetTableBucketReplication(ids.bucketARN)
			require.NoError(t, err)
			require.Len(t, cfg.Rules, 1)
			require.Len(t, cfg.Rules[0].Destinations, 1)
			assert.Equal(t,
				"arn:aws:s3tables:us-west-2:000000000000:bucket/dest",
				cfg.Rules[0].Destinations[0].DestinationTableBucketARN,
			)
		}},
		{name: "tableRecordExpiry table", run: func(t *testing.T) {
			t.Helper()

			cfg, err := fresh.GetTableRecordExpirationConfiguration(ids.tableARN)
			require.NoError(t, err)
			assert.Equal(t, "enabled", cfg.Status)
		}},
		{name: "tableReplication table", run: func(t *testing.T) {
			t.Helper()

			require.Equal(t, 1, s3tables.TableReplicationCount(fresh))

			cfg, err := fresh.GetTableReplicationConfig(ids.tableARN)
			require.NoError(t, err)
			assert.Equal(t, "arn:aws:iam::000000000000:role/repl", cfg.Role)
		}},
		{name: "tags raw map", run: func(t *testing.T) {
			t.Helper()

			tags, err := fresh.ListTagsForResource(ids.tableARN)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"env": "test"}, tags)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}

	assert.Equal(t, persistTestRegion, fresh.Region())
	assert.Equal(t, persistTestAccountID, fresh.AccountID())
}

// TestInMemoryBackend_DeleteTableBucketPrecondition_PostRestore verifies
// that DeleteTableBucket's and DeleteNamespace's non-empty preconditions
// (which query the tablesByBucket and namespacesByBucket secondary
// indexes) still work correctly against indexes rebuilt by Restore, not
// just ones populated by live Put calls. Real S3 Tables requires deleting
// children before their parent (see DeleteTable/DeleteNamespace/
// DeleteTableBucket doc comments); this backend no longer cascade-deletes.
func TestInMemoryBackend_DeleteTableBucketPrecondition_PostRestore(t *testing.T) {
	t.Parallel()

	original, ids := newFullyPopulatedBackend(t)
	ctx := t.Context()

	snap := original.Snapshot(ctx)
	require.NotNil(t, snap)

	fresh := s3tables.NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	require.NoError(t, fresh.Restore(ctx, snap))

	err := fresh.DeleteTableBucket(ids.bucketARN)
	require.ErrorIs(t, err, s3tables.ErrTableBucketNotEmpty)

	err = fresh.DeleteNamespace(ids.bucketARN, []string{"acme_ns"})
	require.ErrorIs(t, err, s3tables.ErrNamespaceNotEmpty)

	require.NoError(t, fresh.DeleteTable(ids.bucketARN, []string{"acme_ns"}, "acme_table", ""))
	require.NoError(t, fresh.DeleteNamespace(ids.bucketARN, []string{"acme_ns"}))
	require.NoError(t, fresh.DeleteTableBucket(ids.bucketARN))

	assert.Equal(t, 0, s3tables.BucketCount(fresh))
	assert.Equal(t, 0, s3tables.NamespaceCount(fresh))
	assert.Equal(t, 0, s3tables.TableCount(fresh))

	_, err = fresh.GetTable(ids.bucketARN, []string{"acme_ns"}, "acme_table")
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend is discarded cleanly rather than
// partially decoded: the backend resets to empty state and Restore returns
// no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b, _ := newFullyPopulatedBackend(t)
	ctx := t.Context()

	err := b.Restore(ctx, []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Equal(t, 0, s3tables.BucketCount(b))
	assert.Equal(t, 0, s3tables.NamespaceCount(b))
	assert.Equal(t, 0, s3tables.TableCount(b))
	assert.Equal(t, 0, s3tables.BucketReplicationCount(b))
	assert.Equal(t, 0, s3tables.TableReplicationCount(b))
	assert.Equal(t, 0, s3tables.TableRecordExpiryCount(b))
}

// TestInMemoryBackend_RestoreAbsentVersion verifies a pre-Phase-3.3 snapshot
// (no "version" field at all, which decodes as Version == 0) is treated as a
// version mismatch, exactly like an explicit incompatible version.
func TestInMemoryBackend_RestoreAbsentVersion(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	require.NoError(t, b.Restore(t.Context(), []byte(`{"accountID":"123456789012","region":"us-west-2"}`)))

	assert.Equal(t, 0, s3tables.BucketCount(b))
	assert.Equal(t, persistTestAccountID, b.AccountID())
	assert.Equal(t, persistTestRegion, b.Region())
}

// TestInMemoryBackend_RestoreInvalidData verifies malformed JSON surfaces as
// an error rather than being silently discarded (that path is reserved for a
// syntactically valid but version-mismatched snapshot; see
// TestInMemoryBackend_RestoreVersionMismatch).
func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := s3tables.NewInMemoryBackend(persistTestAccountID, persistTestRegion)
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestHandler_SnapshotRestoreDelegate verifies Handler.Snapshot/Restore
// delegate to the backend -- the wiring cli.go's generic setupPersistence
// relies on. S3 Tables previously had InMemoryBackend.Snapshot/Restore but
// no Handler delegation, so it was never registered with the persistence
// manager at all; see persistence.go.
func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	b, ids := newFullyPopulatedBackend(t)
	h := s3tables.NewHandler(b)

	snap := h.Snapshot(ctx)
	require.NotNil(t, snap)

	h2 := s3tables.NewHandler(s3tables.NewInMemoryBackend(persistTestAccountID, persistTestRegion))
	require.NoError(t, h2.Restore(ctx, snap))

	table, err := h2.Backend.GetTable(ids.bucketARN, []string{"acme_ns"}, "acme_table")
	require.NoError(t, err)
	assert.Equal(t, "acme_table", table.Name)
}
