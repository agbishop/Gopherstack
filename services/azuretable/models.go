package azuretable

import "github.com/blackbirdworks/gopherstack/pkgs/odatatable"

// Below are re-exported type aliases onto pkgs/odatatable's identical
// types -- the shared entity CRUD/$filter engine extracted from this
// package (see AZURE.md section 9's M6 milestone and pkgs/odatatable's
// package doc comment). Aliasing (rather than a fresh local type) keeps
// every existing azuretable.EntityProperty{} literal, json.Marshal round
// trip, and struct field access in this package's own tests compiling and
// behaving identically, since an alias is the exact same type as what it
// aliases.

// EdmType identifies an entity property's OData EDM type.
type EdmType = odatatable.EdmType

// EntityProperty is a single typed entity property value.
type EntityProperty = odatatable.EntityProperty

// TableInfo is a read-only snapshot of a table's metadata.
type TableInfo = odatatable.TableInfo

// EntityInfo is a read-only snapshot of an entity.
type EntityInfo = odatatable.EntityInfo

// Supported EDM property types. EdmString is the default for a bare JSON
// string with no "@odata.type" annotation; EdmInt32 is the default for a
// bare, whole-number JSON number. See entity_ops.go's decodeProperty for the
// exact inference rules (mirroring azure-sdk-for-go/sdk/data/aztables's own
// EDMEntity.UnmarshalJSON, so unmodified SDK round trips match).
const (
	EdmString   = odatatable.EdmString
	EdmInt32    = odatatable.EdmInt32
	EdmInt64    = odatatable.EdmInt64
	EdmDouble   = odatatable.EdmDouble
	EdmBoolean  = odatatable.EdmBoolean
	EdmDateTime = odatatable.EdmDateTime
	EdmGUID     = odatatable.EdmGUID
	EdmBinary   = odatatable.EdmBinary
)

// --- OData wire error envelope ---
//
// {"odata.error":{"code":"TableNotFound","message":{"lang":"en-US","value":"..."}}}
//
// This is Azure Table Storage's own HTTP error envelope shape, distinct from
// (and not shared with) Cosmos DB's Table API error envelope, so it stays
// local rather than moving into pkgs/odatatable.

type odataErrorMessage struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type odataErrorDetail struct {
	Message odataErrorMessage `json:"message"`
	Code    string            `json:"code"`
}

type odataErrorEnvelope struct {
	Error odataErrorDetail `json:"odata.error"`
}
