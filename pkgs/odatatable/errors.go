package odatatable

import "errors"

// Sentinel errors for the shared OData Table entity/$filter engine. These
// were originally services/azuretable's own sentinel errors; they moved here
// verbatim (message text aside) when services/azuretable's entity CRUD/
// $filter logic was extracted into this package so services/cosmosdb's Table
// API (see AZURE.md section 9's M6) could import the same engine instead of
// duplicating it. services/azuretable re-exports every one of these as a
// package-level var of the same name for backward compatibility with its
// existing exported API and tests.
var (
	ErrTableNotFound       = errors.New("odatatable: table not found")
	ErrTableAlreadyExists  = errors.New("odatatable: table already exists")
	ErrEntityNotFound      = errors.New("odatatable: entity not found")
	ErrEntityAlreadyExists = errors.New("odatatable: entity already exists")
	ErrETagMismatch        = errors.New("odatatable: etag mismatch")

	// ErrInvalidEntityKey is returned when a request omits PartitionKey or
	// RowKey entirely. Empty-string keys are accepted (matching real Azure
	// Table Storage); only an absent key is rejected. See codec.go.
	ErrInvalidEntityKey = errors.New("odatatable: PartitionKey and RowKey are required")

	// ErrInvalidEntityProperty is returned when an entity property's JSON
	// value cannot be decoded under its (explicit or inferred) EDM type.
	ErrInvalidEntityProperty = errors.New("odatatable: invalid entity property value")

	// ErrFilterParse and ErrFilterTooDeep are returned by ParseFilter. A
	// parse error always surfaces as 400 InvalidInput, never a panic or 500
	// -- see each caller's queryEntities-shaped handler.
	ErrFilterParse   = errors.New("odatatable: $filter parse error")
	ErrFilterTooDeep = errors.New("odatatable: $filter expression nested too deeply")

	// ErrSnapshotTableNull and ErrSnapshotEntityNull are returned by Restore
	// when a snapshot's "tables" map (or a table's "Entities" map) holds a
	// JSON null entry, which decodes to a nil pointer that would panic on
	// first dereference if stored as-is. See persistence.go.
	ErrSnapshotTableNull  = errors.New("odatatable: restore snapshot: table is null")
	ErrSnapshotEntityNull = errors.New("odatatable: restore snapshot: entity is null")

	// ErrSnapshotTableNameMismatch is returned by Restore when a snapshot's
	// "tables" map key differs from that entry's storedTable.Name. Table
	// operations all key off the map, while ListTables reads Name -- a
	// mismatch would let those two views disagree about a table's identity.
	// See persistence.go.
	ErrSnapshotTableNameMismatch = errors.New("odatatable: restore snapshot: table map key does not match Name")

	// ErrSnapshotEntityKeyMismatch is returned by Restore when a table's
	// "Entities" map key differs from that entry's own derived
	// (PartitionKey, RowKey) key. Entity operations all key off the map, so
	// a mismatch would let a stored entity be reachable under one identity
	// while its own fields claim another. See persistence.go.
	ErrSnapshotEntityKeyMismatch = errors.New(
		"odatatable: restore snapshot: entity map key does not match PartitionKey/RowKey",
	)

	// ErrMalformedEntityCompositeKey is returned by
	// entityCompositeKey.UnmarshalText when the persisted key does not
	// decode to exactly two JSON string elements. See models.go.
	ErrMalformedEntityCompositeKey = errors.New("odatatable: malformed entity composite key")
)
