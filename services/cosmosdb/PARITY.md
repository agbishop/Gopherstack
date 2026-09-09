---
service: cosmosdb
sdk_module: azure-sdk-for-go/sdk/data/azcosmos@v1.5.0
last_audit_commit: 043fe8d10
last_audit_date: 2026-09-05
overall: C
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  GetDatabaseAccount: {wire: ok, errors: ok, state: n/a, persist: n/a, note: "GET (and HEAD) / (or empty path) -- the database-account root resource. Every real Cosmos SDK, azcosmos included, issues this call before it will make a single data-plane call, to discover the account's readable/writable regional endpoints (azure-sdk-for-go/sdk/data/azcosmos's globalEndpointManager.GetAccountProperties); omitting this resource entirely made the service unreachable by any unmodified SDK until this was added -- an integration-test-only failure mode no unit test could catch, since unit tests call the handler directly rather than going through a real SDK's client-construction path. Returns writableLocations/readableLocations (one entry each, region name \"South Central US\" purely for verisimilitude), enableMultipleWriteLocations: false, and userConsistencyPolicy.defaultConsistencyLevel: \"Session\". databaseAccountEndpoint in each location is built from the REQUEST's own Host header, never from the server's configured port: under testcontainers (or any port-mapping/proxy setup) the container's configured port is frequently not the port a client actually connects through, so advertising the configured port instead of echoing the client's own Host would make the SDK \"discover\" a location pointing at a port nothing is listening on and redirect every subsequent request there -- see TestHandler_AccountRoot_EndpointEchoesRequestHost."}
  CreateDatabase: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /dbs, body {\"id\":\"..\"}. 201 + database body (id/_rid/_self/_etag/_ts/_colls/_users). 409 Conflict on duplicate id."}
  GetDatabase: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /dbs/{db}. 200 + database body; 404 NotFound if absent."}
  ListDatabases: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /dbs -> {\"_rid\":\"\",\"Databases\":[...],\"_count\":N}, sorted by id. No continuation-token pagination -- returns everything in one page."}
  DeleteDatabase: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /dbs/{db}. 204; cascades to delete every container/document beneath it. 404 NotFound if absent."}
  CreateContainer: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /dbs/{db}/colls, body {\"id\":\"..\",\"partitionKey\":{\"paths\":[\"/pk\"],\"kind\":\"Hash\"}}. Exactly one non-empty partition-key path is required (400 BadRequest otherwise -- checked at both the handler and the backend, so an empty-string path element can't slip past a bare length check -- hierarchical/multi-path partition keys are out of scope, see gaps). 201 + container body; 409 Conflict on duplicate id; 404 NotFound if the database doesn't exist."}
  GetContainer: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /dbs/{db}/colls/{coll}. 200 + container body (including its partitionKey definition); 404 if absent."}
  ListContainers: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /dbs/{db}/colls -> {\"_rid\":\"\",\"DocumentCollections\":[...],\"_count\":N}, sorted by id. No continuation-token pagination."}
  DeleteContainer: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /dbs/{db}/colls/{coll}. 204; cascades to delete every document in it. 404 if absent."}
  CreateDocument: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /dbs/{db}/colls/{coll}/docs. Also serves as Upsert Document when x-ms-documentdb-is-upsert: true is set. id is taken from the body if present (must be a non-empty string) or server-generated (UUID v4) if absent; partition key value is extracted from the body per the container's declared path (missing -> null partition key, matching real Cosmos's undefined-partition-key behavior). 201 + document body (id/_rid/_self/_etag/_ts/_attachments overlaid on the stored fields) + ETag header; 409 Conflict on duplicate (id, partitionKey) without upsert; 404 if database/container missing; 400 BadRequest on malformed JSON or a non-string id."}
  QueryDocuments: {wire: ok, errors: ok, state: ok, persist: n/a, note: "POST /dbs/{db}/colls/{coll}/docs with x-ms-documentdb-isquery: True (case-insensitive) and/or Content-Type: application/query+json, body {\"query\":\"...\",\"parameters\":[{\"name\":\"@p\",\"value\":...}]}. Runs the hand-written SQL-subset engine (sql_tokenizer.go/sql_parser.go/sql_exec.go) cross-partition (scans every document in the container). Response {\"_rid\":\"\",\"Documents\":[...],\"_count\":N}. A parse error surfaces as 400 BadRequest, never a panic. See families.sql_query for the supported grammar."}
  ListDocuments: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /dbs/{db}/colls/{coll}/docs (read feed). Response {\"_rid\":\"\",\"Documents\":[...],\"_count\":N}, sorted by id. No continuation-token pagination -- returns every document in one page."}
  GetDocument: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /dbs/{db}/colls/{coll}/docs/{id}. Requires x-ms-documentdb-partitionkey (a JSON array carrying EXACTLY ONE scalar partition key value, e.g. [\"foo\"] or [null]) -- 400 BadRequest if absent, malformed, empty ([]), carrying more than one element, or carrying a non-scalar (object/array) element; an earlier version silently truncated a multi-element array to its first entry instead of rejecting it. 200 + document body + ETag; 404 if not found."}
  ReplaceDocument: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /dbs/{db}/colls/{coll}/docs/{id}. Requires x-ms-documentdb-partitionkey like GetDocument. If-Match absent -> unconditional replace (upsert semantics: creates if absent); If-Match: <etag> -> 412 PreconditionFailed on mismatch. Always full-body replace (drops fields not present in the new body); id/partition key are fixed by the path/header, not the body -- but if the replacement body itself declares a value at the container's partition-key path, it MUST agree with the header's partition key (400 BadRequest on a mismatch), so a document can never end up stored under one partition while its own body claims another."}
  DeleteDocument: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /dbs/{db}/colls/{coll}/docs/{id}. Requires x-ms-documentdb-partitionkey. If-Match absent -> unconditional delete OF AN EXISTING DOCUMENT (still 404 if the document is already absent -- Delete, unlike Replace, has no upsert concept, so an earlier version's absent-plus-no-If-Match short-circuit that returned 204 for a document that never existed was a real bug); If-Match: <etag> -> 412 on mismatch. 404 if not found."}
families:
  auth: {status: partial, note: "masterkey.go implements Cosmos's master-key HMAC-SHA256 scheme (parseMasterKeyAuthorization/masterKeyStringToSign/signMasterKey/VerifyMasterKey), independently of pkgs/azureauth per AZURE.md section 3 (the two schemes' header shape and canonicalization are different enough not to share). Verification is opt-in (Settings.ValidateAuth, default false). With the flag OFF -- the default, mirroring services/azuretable's and services/azureblob's parse-don't-validate stance -- any Authorization header is accepted. With the flag ON, a *present* header must carry a structurally valid, correctly signed master-key signature or the request is rejected with 401 Unauthorized, matching the opt-in-validation precedent AZURE.md section 5 names as the model (services/s3's PresignSecret/WithPresignValidation, which rejects with 403 AccessDenied). An absent Authorization header stays anonymous-accepted even under ValidateAuth: the flag opts into verifying offered signatures, not into requiring authentication, so it cannot break the no-credentials local-dev workflow. See masterkey_test.go's hand-computed known-answer vector (TestVerifyMasterKey_KnownAnswerVector) for proof the algorithm itself is correct."}
  wire_protocol: {status: ok, note: "REST+JSON (no XML anywhere). x-ms-version (pinned to 2020-07-15), x-ms-request-id, Date, x-ms-request-charge (static \"1\" -- fake RU accounting, see gaps), x-ms-session-token (static \"0:0#0\"), and x-ms-activity-id are set on every response. Errors use Cosmos's {\"code\":\"...\",\"message\":\"...\"} JSON envelope with the matching HTTP status (never XML, never Table Storage's odata.error envelope)."}
  sql_query: {status: partial, note: "Hand-written tokenizer -> recursive-descent parser -> AST -> executor (sql_tokenizer.go/sql_parser.go/sql_exec.go), modeled directly on services/azuretable/odata_filter.go's and services/s3/select_sql_*.go's identical shape (per AZURE.md section 6's explicit reuse plan). Supports SELECT * and SELECT <path>[.<path>...] [AS alias] projections (top-level and dotted/nested fields); FROM <alias> [<realias>] (a single source, no JOINs) -- every field reference MUST be qualified with whichever alias is in effect (an unqualified field, or one qualified with an unknown alias, is a parse error, not silently resolved); WHERE with =, !=/<>, <, <=, >, >=, AND, OR, NOT, parentheses, IS NULL/IS NOT NULL, string/number/bool/null literals, and @param bindings; ORDER BY <path> [ASC|DESC] (multiple keys, left-to-right tie-break); TOP n; and OFFSET n LIMIT n. Recursion is depth-bounded (maxQueryDepth=100, counted per genuine nesting level exactly like odata_filter.go's corrected discipline) against stack-overflow DoS. A parse error always surfaces as 400 BadRequest, never a panic. WHERE evaluation implements real Cosmos SQL's three-valued logic (sqlTriState: true/false/undefined), not a simplified two-valued model: a missing field or unbound @param is Undefined (not false), and Undefined does NOT flip to true under NOT -- `NOT (c.missing = 1)` still excludes the row, matching real Cosmos. AND/OR follow standard SQL 3VL. IS NULL/IS NOT NULL are the one place Undefined is not propagated: both evaluate to a definite false against an undefined operand, matching real Cosmos. A resolved comparison between mismatched types still evaluates to a definite false (this emulator does not replicate every nuance of Cosmos's type-coercion rules). Numeric literals and comparisons use json.Number (parsed as int64 first, falling back to float64 only when either side isn't an exact integer) rather than a blanket float64 conversion, so magnitudes beyond 2^53 compare correctly (TestExecuteQuery_Int64PrecisionInComparison). Query execution is always cross-partition (scans every document in the container) -- see gaps for what's NOT supported (subqueries, JOIN, aggregates, built-in functions, array/object literals)."}
  edm_fidelity: {status: ok, note: "Document bodies are decoded (decodeJSONObject) and deep-copied (deepCopyBody/deepCopyJSONValue) via json.Decoder.UseNumber throughout the create/replace/get/query/snapshot paths, so a JSON number beyond float64's 53-bit mantissa (e.g. 9007199254740993) never silently loses precision -- covered end-to-end by TestInMemoryBackend_Int64PrecisionPreservedThroughStorage, TestBackend_SnapshotRestore_Int64PrecisionRoundTrip, and TestExecuteQuery_Int64PrecisionInComparison, at the math.MaxInt64/MinInt64 boundaries too."}
  etag_and_timestamp: {status: ok, note: "ETag format \"<hex-encoded UnixNano>\" derived from a document's Timestamp (models.go's etagFor) -- an opaque quoted token real SDKs never parse, only echo back via If-Match. Every mutation advances Timestamp by at least 100ns past its previous value even if the injected clock returns the same instant twice in a row (store.go's bumpTimestampFrom), guaranteeing a distinct ETag on every write -- covered by TestInMemoryBackend_ETagChangesOnEveryWrite."}
  aliasing: {status: ok, note: "Every document body that crosses the backend boundary (incoming request body, outgoing DocumentInfo.Body, snapshot round-trip) is deep-copied via deepCopyBody/deepCopyJSONValue -- a full marshal/unmarshal round trip through json.Decoder.UseNumber, not a shallow map copy -- so a caller mutating what it passed in or received back can never alias/corrupt stored state, closing the exact bug class AZURE.md's process rules flag from M2."}
  composite_keys: {status: ok, note: "Documents are keyed within a container by documentCompositeKey, a {PartitionKeyJSON, ID} struct (persisted via MarshalText as a JSON string array), not a delimiter-joined string -- closing the same NUL/delimiter-collision class services/azuretable's entityCompositeKey closes for (PartitionKey, RowKey)."}
  persistence: {status: ok, note: "Snapshot version 1. storedDocument's Body field marshals/unmarshals through a json.RawMessage wire shape (storedDocumentWire) specifically so Restore re-decodes it with json.Decoder.UseNumber -- a derived (default) UnmarshalJSON would silently decode every JSON number as float64, corrupting any snapshot value beyond 2^53 on every restart. Restore tolerates a JSON null \"databases\"/\"Containers\"/\"Documents\" map (legal JSON, decodes to a nil Go map, not a nil pointer) by initializing it to an empty map rather than leaving it nil, which would panic the first time anything inserted into it -- this was a real M2 bug in services/azuretable and is guarded here from day one (TestBackend_Restore_NilNestedMapsAreInitialized)."}
  routing_isolation: {status: ok, note: "Runs on its own dedicated *http.Server, bound synchronously in StartWorker to a fixed port (default 8081 via --cosmosdb-port/COSMOSDB_PORT, the real Cosmos DB Local Emulator's own published default -- NOT any of Azurite's 10000/10001/10002; no fallback pool -- fails fast if unavailable). Unlike AzureBlob/AzureQueue/AzureTable, 8081 sits OUTSIDE --port-range-start/--port-range-end's own default range (10000-10100), exactly like services/iot's MQTT broker default (1883) -- but cli.go's reserveFixedServicePorts still reserves it in the shared PortAlloc pool, since a custom operator-configured range could include 8081. See cli_cosmosdb_port_reservation_test.go's table, which (unlike AzureTable's) covers both the outside-range default case and a custom in-range case."}
  observability: {status: ok, note: "StartWorker wraps its Echo handler with telemetry.WrapEchoHandler so ExtractOperation/ExtractResource feed Prometheus metrics. InMemoryBackend uses *lockmetrics.RWMutex instead of raw sync.RWMutex, matching repo convention."}
  table_api: {status: partial, note: "Cosmos DB's Table API (AZURE.md section 9's M6 milestone): table_api.go/table_api_ops.go serve Table/entity CRUD and $filter query on this same fixed port, routed by path shape (a single path segment that isn't \"dbs\" -- see isTableAPIPath) rather than a new port, since real Cosmos disambiguates Table vs. Core/SQL the same way on one hostname. Delegates entirely to pkgs/odatatable -- the exact same entity CRUD/$filter engine services/azuretable imports -- via its own independent TableBackend (*odatatable.InMemoryBackend) instance, so Table API state is disjoint from Core/SQL's database/container/document state. Auth is Cosmos's existing master-key HMAC scheme, unchanged, applied identically to both surfaces. See this file's Table API addendum (below the deferred section) for what's shared vs. what's new, and services/azuretable/PARITY.md for the engine's own known gaps rather than duplicating that list here."}
gaps:
  - "No RU (request unit) accounting -- x-ms-request-charge is a static \"1\" on every response, per AZURE.md section 2's explicit MVP scope note (\"MVP can return static/fake values rather than real RU accounting\")."
  - "No continuation-token pagination on List Databases/List Containers/List Documents/Query Documents -- all return every matching result in one page."
  - "Hierarchical (multi-path) partition keys are not supported -- CreateContainer requires exactly one partitionKey.paths entry (400 BadRequest otherwise)."
  - "SQL query subset does not support subqueries, JOIN, aggregate functions (COUNT/SUM/AVG/MIN/MAX), built-in scalar/array/object functions (e.g. CONTAINS, STARTSWITH, ARRAY_CONTAINS), GROUP BY, or array/object literals in projections/WHERE -- see families.sql_query for exactly what IS supported."
  - "Query execution is always cross-partition (no partition-key-scoped optimization); functionally correct but not representative of a real Cosmos query's RU cost profile."
  - "TLS is not implemented -- plain HTTP only. Clients must connect to http://localhost:8081 (or, for SDKs that default to HTTPS against local emulators, disable SSL verification / point explicitly at plain HTTP). A self-signed cert is a documented non-goal for this milestone, per AZURE.md section 5."
  - "No container-level DefaultTimeToLive (TTL) enforcement -- documents never expire, and there is deliberately no janitor.go (see provider.go's Provider doc comment)."
  - "Database/container ETags are static (derived from their RID, never versioned) since neither resource has an \"update in place\" operation in this milestone."
  - "Auth verification is off by default (permissive, matching the other three Azure services); --cosmosdb-validate-auth opts into enforcing it, but even then only for requests that actually send an Authorization header -- anonymous requests are always accepted. See families.auth."
  - "Table API state (TableBackend) is not yet included in Handler.Snapshot/Restore's persistence lifecycle -- only Core/SQL's Backend is snapshotted today. Table API tables/entities do not survive a snapshot/restore cycle. See this file's Table API addendum below."
  All gaps above are intentional MVP scope per AZURE.md's M3/M6 entries (see AZURE.md section 8), not oversights.
deferred:
  - "Continuation-token pagination (see gaps)."
  - "RU accounting, TLS/self-signed cert support, and container TTL enforcement (see gaps)."
  - "SQL subset completeness beyond what's listed in families.sql_query (subqueries, JOIN, aggregates, built-in functions -- see gaps)."
  - "Table API's $batch (multipart/mixed changesets) -- matches Azure Table Storage's own M2 deferral; see this file's Table API addendum and services/azuretable/PARITY.md."
  - "Initial implementation pass (2026-09-05): seeded this service from scratch per AZURE.md M3 (see AZURE.md section 8). This completes the full 4-service Azure milestone plan (Blob, Queue, Table, Cosmos); M4 (docs/polish, cross-SDK e2e) is the only unstarted item. Structurally mirrors services/azuretable's implementation and PARITY.md format; the SQL query engine borrows services/azuretable/odata_filter.go's and services/s3/select_sql_*.go's tokenizer/parser/AST/executor shape directly, per AZURE.md section 6's explicit reuse plan."
  - "M6 (Table API, see AZURE.md section 8): table_api.go/table_api_ops.go added, delegating to the newly-extracted pkgs/odatatable engine -- see this file's Table API addendum below."
leaks: {status: clean, note: "The dedicated *http.Server started by StartWorker is stopped by Shutdown via srv.Shutdown(ctx) (falling back to srv.Close() on a graceful-shutdown error, both logged), mirroring services/azuretable and cli.go's own top-level server lifecycle. No background goroutines beyond the listener itself -- there is no janitor (see gaps)."}
---

## Notes

### Why Cosmos DB gets its own port, and why that port is 8081 (not another Azurite-style number)
Every other Azure service in this repo (Blob 10000, Queue 10001, Table 10002)
mirrors Azurite's own multi-port convention, because Azurite emulates all
three. The real **Cosmos DB Local Emulator** is a separate tool with its own
separate published default port, **8081** -- not part of the Azurite family
at all. Picking 8081 (rather than continuing the 10003 pattern) matches
AZURE.md section 4's stated intent ("Cosmos its own port, mirroring the real
emulator's fixed 8081 default") and means `UseDevelopmentStorage`-style
zero-configuration client setups pointed at the real emulator's documented
endpoint (`https://localhost:8081`, minus the TLS -- see below) work
unmodified against gopherstack too.

One consequence: 8081 sits **outside** `--port-range-start`/
`--port-range-end`'s own default range (10000-10100), exactly like
`services/iot`'s MQTT broker default (1883) -- see AZURE.md section 4's
discussion of that precedent. `StartWorker` still binds it synchronously
with no fallback pool (fails fast on conflict), and `cli.go`'s
`reserveFixedServicePorts` still calls `alloc.Reserve(cli.CosmosDB.Port,
"cosmosdb")` even though that's a no-op against the shipped defaults: the
range is user-configurable, so an operator who narrows it to (say)
`8000-8100` needs PortAlloc to know 8081 is spoken for.
`cli_cosmosdb_port_reservation_test.go`'s table is therefore the inverse of
`cli_azuretable_port_reservation_test.go`'s: the *default*-port case is
`wantBlockedFromPool: false`, and a *separate* custom-range case
demonstrates the reservation actually taking effect.

### TLS is a documented non-goal
Real Cosmos SDKs often default to HTTPS even against local emulators. This
milestone serves plain HTTP only (`http://localhost:8081`); clients must
either be pointed explicitly at that scheme or configured to disable SSL
verification, matching AZURE.md section 5's stated MVP scope. A self-signed
cert on the Cosmos listener remains a stretch goal, not attempted here.

### Master-key auth: why it isn't in pkgs/azureauth
AZURE.md section 3 states this explicitly: Cosmos's master-key HMAC scheme
is different enough from Blob/Queue/Table's shared SharedKey scheme
(different header shape -- a single URL-encoded `type=...&ver=...&sig=...`
string, not `Scheme account:sig` -- and a different, simpler
canonicalization with no header-block canonicalization step) that sharing a
package wasn't worth it. `masterkey.go` mirrors `pkgs/azureauth`'s internal
shape (a parse function, a canonicalization function, a sign function, an
opt-in verify function) as unexported package-level code in
`services/cosmosdb` instead. The canonicalization is exactly:

```text
lowercase(verb) + "\n" + lowercase(resourceType) + "\n" + resourceId +
"\n" + lowercase(x-ms-date) + "\n" + lowercase(date-header) + "\n"
```

(`resourceId`'s casing is preserved -- it identifies an exact resource
path). `masterkey_test.go`'s `TestVerifyMasterKey_KnownAnswerVector` proves
the HMAC-SHA256 signing algorithm itself is correct against a hand-computed
vector (independently verified via Python's `hmac`/`hashlib`), even though
enforcement (`--cosmosdb-validate-auth`) is opt-in and off by default.

### SQL query grammar
```text
query      := SELECT [TOP number] selectList FROM ident [ident]
              [WHERE expr] [ORDER BY orderList] [OFFSET number LIMIT number]
selectList := '*' | projection (',' projection)*
projection := path [AS ident]
path       := ident ('.' ident)*
orderList  := orderItem (',' orderItem)*
orderItem  := path [ASC|DESC]
expr       := orExpr
orExpr     := andExpr ('OR' andExpr)*
andExpr    := unary ('AND' unary)*
unary      := 'NOT' unary | primary
primary    := '(' expr ')' | operand 'IS' ['NOT'] 'NULL' | comparison
comparison := operand ('='|'!='|'<>'|'<'|'<='|'>'|'>=') operand
operand    := path | '@param' | 'string' | number | true | false | null
```
Implemented as a real lexer -> recursive-descent parser -> AST -> executor
(`sql_tokenizer.go`/`sql_parser.go`/`sql_exec.go`), directly modeled on
`services/azuretable/odata_filter.go`'s identical shape (which is itself
modeled on `services/dynamodb/expr`) and `services/s3/select_sql_*.go`'s SQL
tokenizer/parser/executor precedent -- per AZURE.md section 6's explicit
instruction to reuse that muscle rather than invent a third parsing
approach. Recursion depth is bounded (`maxQueryDepth = 100`, counted only at
genuine nesting points -- `NOT` and `(...)` -- not per precedence-climbing
layer, replicating `odata_filter.go`'s own corrected discipline) so a
maliciously deep query fails with a parse error instead of overflowing the
stack (`TestParseQuery_DeepNestingBounded`/
`TestParseQuery_ModeratelyNestedParensAccepted`). A parse error of any kind
always surfaces as `400 BadRequest`, never a panic and never a 500.

The leading identifier in every path (`c` in `c.age`, or a bare `c`) is
always treated as the query's single source alias and stripped before
evaluation -- this emulator has no JOINs, so there is exactly one row
source, and the alias name itself is not validated against the `FROM`
clause's declared alias (any leading segment is accepted and discarded).

### Int64/float64 precision (the M2 bug class, guarded from day one)
Every JSON-number-bearing path in this package -- request body decode
(`decodeJSONObject`), backend storage/return (`deepCopyBody`/
`deepCopyJSONValue`), snapshot persistence (`storedDocumentWire`), and SQL
comparison (`compareJSONNumbers`) -- uses `json.Number` via
`json.Decoder.UseNumber`, never a bare `map[string]any` decode (which always
produces `float64` and silently rounds any integer magnitude beyond
`2^53`). `compareJSONNumbers` specifically tries `Int64()` on both operands
first and only falls back to `Float64()` when at least one isn't an exact
integer, so `WHERE c.big = 9007199254740993` distinguishes that value from
`9007199254740992` correctly. Covered end-to-end by
`TestInMemoryBackend_Int64PrecisionPreservedThroughStorage`,
`TestBackend_SnapshotRestore_Int64PrecisionRoundTrip`, and
`TestExecuteQuery_Int64PrecisionInComparison`, at `math.MaxInt64`/
`math.MinInt64` boundaries.

### Aliasing and composite keys (the other two M2 bug classes)
`deepCopyBody`/`deepCopyJSONValue` (a full marshal/unmarshal round trip, not
a shallow `maps.Copy`) guards every document body that crosses the backend
boundary, so a caller mutating a map it passed in or got back can never
corrupt stored state. `documentCompositeKey` is a `{PartitionKeyJSON, ID}`
struct (persisted via `MarshalText` as a JSON string array), never a
delimiter-joined string, closing the same NUL-byte collision class
`services/azuretable`'s `entityCompositeKey` closes.

### Table API (M6)
Cosmos DB's Table API (`table_api.go`/`table_api_ops.go`) is a second wire
surface on this same fixed port (8081), added in AZURE.md section 9's M6
milestone. It is **not** a reimplementation: it delegates entity CRUD,
`$filter` parsing, and `$filter` evaluation entirely to `pkgs/odatatable`,
the exact same shared engine `services/azuretable` imports (see that
package's doc comment for the extraction this milestone performed). This
service's own `families.table_api` entry above covers what's new here
(routing, its own independent `TableBackend` instance, auth reuse); for the
engine's own known gaps -- pagination, `$select`'s PartitionKey/RowKey/
Timestamp behavior, the Edm.Double/Int32 unannotated-number ambiguity, `$batch`
deferral, and so on -- see **`services/azuretable/PARITY.md`** directly
rather than duplicating that list here: every gap recorded there applies
here identically, since it's the same code.

The one gap specific to *this* service's Table API integration (not the
engine itself) is persistence: `TableBackend` is not yet wired into
`Handler.Snapshot`/`Restore` (see this file's own `gaps` list above) --
`pkgs/odatatable.InMemoryBackend` already implements `Snapshot`/`Restore`
(the same methods `services/azuretable` relies on), so wiring it in is a
follow-up, not a redesign.

## More

- [Full parity audit](PARITY.md)
- [services/azuretable/PARITY.md](../azuretable/PARITY.md) -- the shared `pkgs/odatatable` entity/$filter engine's own known gaps
- [All services](../../README.md#services)
