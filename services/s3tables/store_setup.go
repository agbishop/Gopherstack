package s3tables

// Code in this file supports Phase 3.3 of the datalayer refactor: every
// map[string]*T resource field on InMemoryBackend is registered exactly
// once, here, as a *store.Table[T]. See pkgs/store's package doc for the
// underlying primitive.
//
// tableBuckets, namespaces, and tables were previously nested
// bucket -> namespace -> table (tableBuckets flat by ARN, namespaces keyed
// by "bucketARN::namespace", tables flat by ARN with a separate
// map[string]string tableIndex mapping "bucketARN::namespace::name" back to
// a table ARN). All three flatten cleanly onto store.Table because every
// composite-key component (TableBucketARN, Namespace, Name/ARN) is already
// a real, wire-visible (non-json:"-") field on the value type itself -- no
// hidden field or DTO wrapper is needed, unlike services/workmail's
// org-nested tables or services/emr's region-nested ones.
//
//   - tableBuckets registers directly by TableBucket.ARN (services/sesv2's
//     clean-direct-register template): ARN is globally unique and is how
//     every bucket-level operation already looks buckets up.
//   - namespaces registers by the composite "tableBucketARN::namespace" key
//     (namespaceKey in backend.go) -- a namespace's own NamespaceID is
//     generated but never used as a lookup key; every namespace operation
//     is parent(bucket)-scoped -- with a "byBucket" *store.Index grouping
//     by TableBucketARN for the per-bucket scans ListNamespaces and
//     DeleteTableBucket's not-empty precondition (services/workmail/
//     services/emr's parent-nested composite-key + Index template).
//   - tables registers directly by Table.ARN, same as tableBuckets: ARN is
//     globally unique and is how table-replication/expiry/tag operations
//     already look tables up directly. Two additive secondary indexes cover
//     the remaining access patterns: "byComposite" (bucketARN::namespace::
//     name -> table) replaces the old tableIndex map outright, and
//     "byBucket" (TableBucketARN -> tables) serves ListTables and
//     DeleteNamespace's not-empty precondition, mirroring namespaces'
//     byBucket index.
//
// bucketReplication, tableReplication, and tableRecordExpiry hold value
// types (BucketReplicationConfig, TableReplicationConfig,
// TableRecordExpiryConfig) that carry no identity of their own -- they are
// keyed purely by the external map key (a bucket or table ARN the caller
// supplies). Each type has a json:"-" identity field (TableBucketARN,
// TableARN respectively; see their doc comments in models.go) so their
// store.Table has something to key on, but a plain json.Marshal always
// drops a json:"-" field, so none of the three is on b.registry --
// persistence.go builds a separate ephemeral DTO registry for them instead
// (the same technique services/codecommit and services/emr use for their
// identity-less/hidden-field resources).
//
// tags (map[string]map[string]string) is left as a plain map: it does not
// hold a *T value with an identity of its own, so it does not fit
// store.Table's keyed-collection shape. See persistence.go for how it
// round-trips alongside the tables above.
import (
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

func tableBucketKeyFn(v *TableBucket) string { return v.ARN }

func namespaceKeyFn(v *Namespace) string {
	return namespaceKey(v.TableBucketARN, joinNamespace(v.Namespace))
}

func namespaceBucketIndexKeyFn(v *Namespace) string { return v.TableBucketARN }

func tableKeyFn(v *Table) string { return v.ARN }

func tableCompositeIndexKeyFn(v *Table) string {
	return tableCompositeKey(v.TableBucketARN, joinNamespace(v.Namespace), v.Name)
}

func tableBucketIndexKeyFn(v *Table) string { return v.TableBucketARN }

func bucketReplicationKeyFn(v *BucketReplicationConfig) string { return v.TableBucketARN }

func tableReplicationKeyFn(v *TableReplicationConfig) string { return v.TableARN }

func tableRecordExpiryKeyFn(v *TableRecordExpiryConfig) string { return v.TableARN }

// registerAllTables registers every converted resource collection exactly
// once. It must be called during construction only (immediately after
// b.registry is created -- see NewInMemoryBackend), never on every Reset()
// -- store.Register panics on a duplicate name, so runtime resets go through
// b.registry.ResetAll() plus per-table Reset() calls instead (see
// InMemoryBackend.Reset in backend.go).
func registerAllTables(b *InMemoryBackend) {
	b.tableBuckets = store.Register(b.registry, "tableBuckets", store.New(tableBucketKeyFn))

	b.namespaces = store.Register(b.registry, "namespaces", store.New(namespaceKeyFn))
	b.namespacesByBucket = b.namespaces.AddIndex("byBucket", namespaceBucketIndexKeyFn)

	b.tables = store.Register(b.registry, "tables", store.New(tableKeyFn))
	b.tablesByComposite = b.tables.AddIndex("byComposite", tableCompositeIndexKeyFn)
	b.tablesByBucket = b.tables.AddIndex("byBucket", tableBucketIndexKeyFn)

	// bucketReplication, tableReplication, and tableRecordExpiry are
	// deliberately built with store.New only, NOT store.Register -- see
	// this file's doc comment.
	b.bucketReplication = store.New(bucketReplicationKeyFn)
	b.tableReplication = store.New(tableReplicationKeyFn)
	b.tableRecordExpiry = store.New(tableRecordExpiryKeyFn)
}
