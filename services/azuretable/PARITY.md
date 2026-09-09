---
service: azuretable
sdk_module: azure-sdk-for-go/sdk/data/aztables@v1.4.1
last_audit_commit: 3219e576
last_audit_date: 2026-09-04
overall: C
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateTable: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /<account>/Tables, body {\"TableName\":\"..\"}. 201 + entity body, or 204 + Preference-Applied when Prefer: return-no-content is sent (aztables' default). 409 TableAlreadyExists on duplicate -- unlike services/azurequeue's CreateQueue, there is no metadata-identical-retry idempotency exception."}
  DeleteTable: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<account>/Tables('name'), including the '' escaped-quote form. 404 TableNotFound if absent."}
  ListTables: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /<account>/Tables, sorted by name. No prefix/continuation-token pagination -- returns all tables in one page."}
  InsertEntity: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /<account>/<table>. 201 + entity body + ETag, or 204 + Preference-Applied under Prefer: return-no-content. 409 EntityAlreadyExists; 404 TableNotFound. Missing PartitionKey/RowKey -> 400 InvalidInput; empty-string keys are accepted (matches real Azure)."}
  GetEntity: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /<account>/<table>(PartitionKey='..',RowKey='..'). 200 + entity + ETag; 404 ResourceNotFound. $select is honored for custom properties; PartitionKey/RowKey/Timestamp are always returned regardless of $select (see Notes)."}
  QueryEntities: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /<account>/<table>() or /<account>/<table>, both with optional $filter/$top/$select. Results ordered by (PartitionKey, RowKey). No continuation-token pagination -- returns everything matching in one page, capped by $top if given."}
  ReplaceEntity: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /<account>/<table>(..). No If-Match -> Insert-Or-Replace (upsert). If-Match: * -> unconditional replace, 404 ResourceNotFound if absent. If-Match: <etag> -> 412 UpdateConditionNotSatisfied on mismatch, 404 if absent. Always full-body replace (drops properties not in the new body)."}
  MergeEntity: {wire: ok, errors: ok, state: ok, persist: ok, note: "PATCH (aztables' real wire method), literal MERGE, or POST/PUT/PATCH carrying an X-Http-Method: MERGE tunneling header (honored only on those three methods, never GET/DELETE) /<account>/<table>(..), same If-Match semantics as ReplaceEntity but merges properties (unlisted properties survive) instead of replacing wholesale."}
  DeleteEntity: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /<account>/<table>(..). If-Match is mandatory (400 InvalidInput if absent) -- * or a specific ETag; 412 on mismatch, 404 ResourceNotFound if absent."}
  Batch: {wire: deferred, errors: ok, state: deferred, persist: n/a, note: "POST /<account>/$batch (multipart/mixed changesets) returns a clean 501 NotImplemented pointing at this file, rather than a confusing 404/400. See deferred section."}
families:
  auth: {status: partial, note: "Identical stance to services/azurequeue: checkAuth parses a present Authorization header via pkgs/azureauth.ParseAuthorizationHeader (structural only, SharedKey and SharedKeyLite both accepted). Verification is not enforced; an absent or malformed header is still accepted."}
  wire_protocol: {status: ok, note: "REST+JSON/OData. Honors the Accept header's odata= level (nometadata/minimalmetadata/fullmetadata), responding with a matching Content-Type. x-ms-version (pinned to 2019-02-02, the literal value azure-sdk-for-go/sdk/data/aztables' generated client sends and expects), x-ms-request-id, Date, and DataServiceVersion: 3.0; are set on every response; x-ms-error-code is set on every error."}
  odata_filter: {status: ok, note: "Hand-written lexer -> recursive-descent parser -> AST -> evaluator, extracted into pkgs/odatatable (filter.go/eval.go) per AZURE.md section 9's M6 milestone so services/cosmosdb's Table API could share it; re-exported here under this package's historical names (see interfaces.go). Modeled on services/dynamodb/expr. Supports eq/ne/lt/le/gt/ge, and/or/not, parentheses, and every literal form (quoted string with '' escape, integer, Int64 'L' suffix, float, true/false, datetime'..', guid'..', X'..'/binary'..'). Recursion is depth-bounded (maxFilterDepth=100), counted per genuine nesting level ('not' and '(...)' only, not per precedence-climbing layer -- see Notes) against stack-overflow DoS. A comparison against a missing property evaluates false, never an error; a parse error always surfaces as 400 InvalidInput, never a panic. Two Int64 operands are compared as int64 directly, not via a blanket float64 conversion, so magnitudes beyond 2^53 compare correctly (see Notes)."}
  edm_types: {status: ok, note: "Edm.String/Int32/Int64/Double/Boolean/DateTime/Guid/Binary all round-trip. Int64/DateTime/Guid/Binary emit a Prop@odata.type annotation on read (matching real Table Storage and aztables' own EDMEntity.MarshalJSON); Int32/Double/Boolean/String do not. Unannotated bare-number decode tries Int32 first, then falls back to Double, exactly mirroring aztables' own client-side inference -- a whole-number Double (e.g. 4.0) is therefore indistinguishable from an Int32 without an explicit annotation, an inherent wire-protocol ambiguity aztables itself has (its own MarshalJSON never annotates plain float64 either). EdmBinary values are deep-copied on every store/return path (insert, merge, get, query) so a caller mutating a []byte it passed in or received back can never alias/corrupt stored state."}
  etag_and_timestamp: {status: ok, note: "ETag format W/\"datetime'<url-encoded RFC3339, 7 fractional digits>'\" derived from Timestamp. Every mutation advances Timestamp by at least 100ns past its previous value even if the injected clock returns the same instant twice in a row, guaranteeing a distinct ETag on every write (see pkgs/odatatable/store.go's bumpTimestamp, as of the M6 extraction) -- covered by TestInMemoryBackend_ETagChangesOnEveryWrite."}
  persistence: {status: ok, note: "Snapshot version 2 (bumped from 1 -- see Notes). Edm.Int64 is snapshotted as a decimal string, not a bare JSON number: a float64 number loses precision above 2^53, corrupting large Int64 values across a save/restore cycle. Entity identity within a table is keyed by entityCompositeKey (a {PartitionKey, RowKey} struct, persisted via MarshalText as a JSON string array), not a NUL-delimited string, closing a key-collision class where two different (PartitionKey, RowKey) pairs could hash to the same delimited string. Restore initializes a table's Entities map when the snapshot carries \"Entities\": null (legal JSON, decodes to a nil map) rather than leaving it nil, which would panic the first time anything inserts into it."}
  routing_isolation: {status: ok, note: "Runs on its own dedicated *http.Server, bound synchronously in StartWorker to a fixed port (default 10002 via --azure-table-port/AZURE_TABLE_PORT, matching Azurite's own Table service port; no fallback pool -- fails fast if unavailable, mirroring services/azureblob and services/azurequeue). cli.go's reserveFixedServicePorts additionally reserves this port in the shared PortAlloc pool since 10002 sits inside --port-range-start/--port-range-end's own default range."}
  observability: {status: ok, note: "StartWorker wraps its Echo handler with telemetry.WrapEchoHandler so ExtractOperation/ExtractResource feed Prometheus metrics. InMemoryBackend uses *lockmetrics.RWMutex instead of raw sync.RWMutex, matching repo convention."}
gaps:
  - "No continuation-token pagination on List Tables or Query Entities -- both return every matching result in one page. x-ms-continuation-NextPartitionKey/NextRowKey response headers are not set."
  - "$select is honored for custom properties, but PartitionKey/RowKey/Timestamp are always returned regardless of the $select list (real Table Storage honors $select literally for these too); documented deviation chosen for simplicity and because every SDK round-trip needs the key properties anyway."
  - "A whole-number Edm.Double value (e.g. 4.0) round-trips as Edm.Int32 when written without an explicit @odata.type annotation -- an inherent ambiguity in the unannotated-number wire format that aztables' own client has too (see families.edm_types)."
  - "No SAS / Set-Get Table ACL support."
  - "No queue-style janitor: Table Storage entities have no TTL/expiry concept, so there is nothing to sweep (this is a deliberate scope decision, not an oversight -- see provider.go's Provider doc comment)."
  - "Auth verification is not enforced -- see families.auth."
  All gaps above are intentional MVP scope per AZURE.md's M2 entry (see AZURE.md section 8), not oversights.
deferred:
  - "Batch (POST /<account>/$batch, multipart/mixed changesets) is explicitly out of scope for this milestone -- returns a clean 501 NotImplemented rather than attempting a partial implementation. Matches services/azureblob's M0 multipart-upload deferral pattern."
  - "Continuation-token pagination for List Tables/Query Entities (see gaps)."
  - "Initial implementation pass (2026-09-04): seeded this service from scratch per AZURE.md M2 (see AZURE.md section 8). Structurally mirrors services/azurequeue's M1 implementation and PARITY.md format; no prior audit history to reconcile."
leaks: {status: clean, note: "The dedicated *http.Server started by StartWorker is stopped by Shutdown via srv.Shutdown(ctx) (falling back to srv.Close() on a graceful-shutdown error, both logged), mirroring services/azurequeue and cli.go's own top-level server lifecycle. No background goroutines beyond the listener itself -- there is no janitor (see gaps)."}
---

## Notes

### Why Azure Table gets its own port, separate from Blob and Queue
Azure Table's REST path shape (`/<account>/<resource>`) shares the same
ambiguity with Azure Blob and Queue that motivated each of their own
dedicated ports (see `services/azureblob/PARITY.md` and
`services/azurequeue/PARITY.md`'s identical notes, and AZURE.md section 4)
-- multiplexing Table onto either of their ports would reintroduce exactly
that collision. `StartWorker` synchronously binds a fixed port (default
`10002`, Azurite's own Table-service default, overridable via
`--azure-table-port`/`AZURE_TABLE_PORT`) before standing up its own
`*echo.Echo` + `*http.Server`, with no fallback into the shared
`--port-range-start`/`--port-range-end` `PortAlloc` pool if the bind fails.
`cli.go`'s `reserveFixedServicePorts` additionally reserves `10002` in that
shared pool at startup.

### No janitor
Table Storage entities carry no TTL, visibility timeout, or lease concept --
nothing analogous to Queue's message expiry or Blob's lease expiry. There is
therefore no background sweep to run, and (deliberately, unlike
`services/azurequeue`/`services/azureblob`) no `janitor.go` in this package.

### `$filter` grammar
```
expr       := orExpr
orExpr     := andExpr ('or' andExpr)*
andExpr    := unary ('and' unary)*
unary      := 'not' unary | primary
primary    := '(' expr ')' | comparison
comparison := operand ('eq'|'ne'|'lt'|'le'|'gt'|'ge') operand
operand    := identifier | literal
literal    := 'quoted string' (with '' escape) | integer | integer'L' | float | true | false
            | datetime'<RFC3339>' | guid'<uuid>' | X'<hex>' | binary'<base64>'
```
Implemented as a real lexer -> recursive-descent parser -> AST -> evaluator,
now living in `pkgs/odatatable` (`filter.go`/`eval.go`) since AZURE.md
section 9's M6 milestone extracted it out of this package so
`services/cosmosdb`'s Table API could share the same engine; this package
re-exports `Node`/`ParseFilter`/`EvaluateFilter` under their historical
names (see `interfaces.go`/`odata_filter.go`/`odata_filter_eval.go`).
Modeled on `services/dynamodb/expr`'s identical shape -- not string
matching. Recursion
depth is bounded (`maxFilterDepth = 100`) so a maliciously (or accidentally)
deeply-nested filter fails with a parse error instead of overflowing the
stack. Identifiers resolve against `PartitionKey`/`RowKey`/`Timestamp` first,
then custom properties; a comparison against a property the entity doesn't
have evaluates to `false`, matching real Table Storage semantics, never an
error. Comparisons are type-aware: numeric operands (Int32/Int64/Double) are
compared numerically regardless of which numeric type each side is, strings
lexicographically, datetimes chronologically, and booleans only support
`eq`/`ne`. A type mismatch between the two operands (e.g. a string compared
against a number) evaluates to `false` rather than erroring. A parse error of
any kind -- unbalanced parens, a trailing operator, an empty filter string, an
invalid literal -- surfaces as `400 InvalidInput`, never a panic and never a
500.

**Depth counting (fixed in review).** `parseOr` -> `parseAnd` -> `parseUnary`
-> `parsePrimary` is one precedence-climbing layer per grammar rule, not one
nesting level -- a bare `Age eq 1` with no parens or `not` anywhere still
passes through all four. An earlier version incremented the depth counter on
every one of those layers, so `maxFilterDepth = 100` was actually exhausted
by roughly 25 nested parentheses, not 100 -- a bound that didn't mean what
its name said. `depth` is now threaded through unchanged across the routine
same-level calls and incremented only at the two places genuine nesting
happens: `parseUnary`'s `not` branch and `parsePrimary`'s `(...)` branch.
`TestParseFilter_ModeratelyNestedParensAccepted` (50 levels, must parse) and
`TestParseFilter_DeepNestingBounded` (500 levels, must reject) cover both
sides of the corrected bound.

**Int64 comparison precision (fixed in review).** Comparing two integer
operands (Int32 and/or Int64) converts neither through `float64` when both
are integer-typed: it compares their `int64` values directly. A blanket
`float64(intVal)` conversion -- this package's original approach -- silently
rounds any Int64 magnitude beyond `2^53` (float64's mantissa width), so e.g.
`Big eq 9007199254740992L` would incorrectly match a stored
`9007199254740993`. A comparison involving an actual `Edm.Double` operand
still goes through `float64`, since Double itself is already an inexact
64-bit float with no wider exact common type to compare against. See
`TestEvaluateFilter_Int64PrecisionNotLostInComparison`.

### EDM property typing
See `families.edm_types` above. `models.go`'s `EntityProperty` carries an
explicit `EdmType` alongside its Go value so read/write and `$filter`
evaluation never have to guess; `entity_ops.go`'s `decodeProperty` mirrors
`azure-sdk-for-go/sdk/data/aztables`'s own `EDMEntity.UnmarshalJSON`
inference logic exactly (try `Edm.Int32` first for an unannotated bare
number, then fall through to the JSON value's natural type) so unmodified
SDK round trips match byte-for-byte in the cases that matter.

### ETag / Timestamp monotonicity
See `families.etag_and_timestamp` above -- this is exactly the bug class
M1's (`services/azurequeue`) review bots caught elsewhere in this project:
two mutations to the same entity within the same clock tick must not produce
identical ETags, so `store.go`'s `bumpTimestamp` forces at least a 100ns
forward step past the entity's previous `Timestamp` even when the injected
clock hasn't advanced.

## More

- [Full parity audit](PARITY.md)
- [All services](../../README.md#services)
