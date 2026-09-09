// Package azuretable provides a local, in-memory emulation of Azure Table
// Storage's REST+JSON/OData wire protocol (table CRUD plus entity
// insert/get/query/replace/merge/delete, including a hand-written $filter
// lexer/parser/evaluator), Azurite-compatible enough for unmodified
// azure-sdk-for-go clients to operate against. See AZURE.md and PARITY.md
// for scope and known gaps.
//
// The entity CRUD/$filter engine itself lives in pkgs/odatatable (extracted
// here in AZURE.md section 9's M6 milestone so services/cosmosdb's Table API
// could import the same engine instead of duplicating it); this package is a
// thin wire-protocol adapter over it -- HTTP routing/dispatch (handler.go),
// request/response shaping (table_ops.go/entity_ops.go), and re-exports of
// pkgs/odatatable's types/functions/errors under their historical
// azuretable-local names (models.go, errors.go, store.go) so this package's
// own public API is unaffected by the extraction.
//
// Unlike services/azurequeue and services/azureblob, this package has no
// janitor.go: Table Storage entities carry no TTL/expiry concept (there is
// no message-visibility or blob-lease analogue), so there is nothing for a
// background sweep to do.
package azuretable

import "github.com/blackbirdworks/gopherstack/pkgs/odatatable"

// Compile-time assertion: InMemoryBackend must implement StorageBackend.
var _ StorageBackend = (*InMemoryBackend)(nil)

// IfMatchAny is the wildcard If-Match value ("*") meaning "must currently
// exist, but match unconditionally on ETag" -- as opposed to an empty
// ifMatch (no header at all, meaning upsert semantics) or a specific ETag
// string (optimistic-concurrency match required).
const IfMatchAny = odatatable.IfMatchAny

// StorageBackend defines the interface for an Azure Table Storage backend.
// Re-exported from pkgs/odatatable.StorageBackend (see this file's package
// doc comment) so existing callers referencing azuretable.StorageBackend
// keep compiling unchanged.
//
// The ifMatch parameter on ReplaceEntity/MergeEntity/DeleteEntity threads
// through the three If-Match states the wire protocol distinguishes:
//   - "" (no If-Match header): upsert semantics for Replace/MergeEntity
//     (create if absent, otherwise mutate unconditionally); DeleteEntity's
//     caller (handler.go) never passes "" -- an absent If-Match on Delete is
//     rejected at the handler layer before the backend is ever called.
//   - IfMatchAny ("*"): the entity must exist, but any current ETag matches.
//   - any other string: the entity must exist AND its current ETag must
//     equal this value, else ErrETagMismatch.
type StorageBackend = odatatable.StorageBackend
