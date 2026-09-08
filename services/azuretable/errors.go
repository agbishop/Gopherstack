package azuretable

import "github.com/blackbirdworks/gopherstack/pkgs/odatatable"

// Sentinel errors for Azure Table Storage operations. These are re-exported
// from pkgs/odatatable (the shared entity CRUD/$filter engine extracted from
// this package -- see AZURE.md section 9's M6 milestone and
// pkgs/odatatable's package doc comment) rather than declared locally, so
// existing callers/tests referencing azuretable.ErrTableNotFound and friends
// keep working unchanged: errors.Is comparisons still succeed since each var
// here is the exact same error value odatatable's engine returns.
var (
	ErrTableNotFound       = odatatable.ErrTableNotFound
	ErrTableAlreadyExists  = odatatable.ErrTableAlreadyExists
	ErrEntityNotFound      = odatatable.ErrEntityNotFound
	ErrEntityAlreadyExists = odatatable.ErrEntityAlreadyExists
	ErrETagMismatch        = odatatable.ErrETagMismatch

	// ErrInvalidEntityKey is returned when a request omits PartitionKey or
	// RowKey entirely. Empty-string keys are accepted (matching real Azure
	// Table Storage); only an absent key is rejected. See entity_ops.go.
	ErrInvalidEntityKey = odatatable.ErrInvalidEntityKey

	// ErrInvalidEntityProperty is returned when an entity property's JSON
	// value cannot be decoded under its (explicit or inferred) EDM type.
	ErrInvalidEntityProperty = odatatable.ErrInvalidEntityProperty

	// ErrFilterParse and ErrFilterTooDeep are returned by ParseFilter. A
	// parse error always surfaces as 400 InvalidInput, never a panic or 500
	// -- see handler.go's queryEntities.
	ErrFilterParse   = odatatable.ErrFilterParse
	ErrFilterTooDeep = odatatable.ErrFilterTooDeep

	// ErrSnapshotTableNull and ErrSnapshotEntityNull are returned by Restore
	// when a snapshot's "tables" map (or a table's "Entities" map) holds a
	// JSON null entry, which decodes to a nil pointer that would panic on
	// first dereference if stored as-is. See persistence.go.
	ErrSnapshotTableNull  = odatatable.ErrSnapshotTableNull
	ErrSnapshotEntityNull = odatatable.ErrSnapshotEntityNull

	// ErrSnapshotTableNameMismatch is returned by Restore when a snapshot's
	// "tables" map key differs from that entry's storedTable.Name. Table
	// operations all key off the map, while ListTables reads Name -- a
	// mismatch would let those two views disagree about a table's identity.
	// See persistence.go.
	ErrSnapshotTableNameMismatch = odatatable.ErrSnapshotTableNameMismatch
)
