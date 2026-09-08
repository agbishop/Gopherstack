---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: rdsdata
sdk_module: aws-sdk-go-v2/service/rdsdata@v1.35.4   # version audited against
last_audit_commit: deb6c42f                          # HEAD when this pass started (working tree, uncommitted)
last_audit_date: 2026-09-04
overall: A            # every op/family field-diffed against the real SDK source this pass
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  ExecuteStatement: {wire: ok, errors: ok, state: ok, persist: ok, note: >
    FormatRecordsAs=JSON, the full 14-field ColumnMetadata, resultSetOptions
    (decimalReturnType/longReturnType), and generatedFields (rowid-alias
    INSERTs) are all implemented for real this pass -- see Notes.
    continueAfterTimeout is accepted on the wire as a documented no-op (no
    statement timeouts exist to continue past).}
  BatchExecuteStatement: {wire: ok, errors: ok, state: ok, persist: ok, note: >
    One UpdateResult per parameter set; transaction id validated before any
    engine execution. GeneratedFields now populated per parameter set using
    the same rowid-alias detection as ExecuteStatement (see Notes).}
  BeginTransaction: {wire: ok, errors: ok, state: ok, persist: ok, note: >
    Opaque per-region sequential id (txn-NNNNNN); real engine-side sql.Tx
    opened alongside so statements tagged with the id share atomic visibility.}
  CommitTransaction: {wire: ok, errors: ok, state: ok, persist: ok, note: >
    Deletes the transaction from the region's table before returning, so
    reuse (execute/commit/rollback) correctly 400s with TransactionNotFoundException.}
  RollbackTransaction: {wire: ok, errors: ok, state: ok, persist: ok}
  ExecuteSql: {wire: ok, errors: ok, state: ok, persist: ok, note: >
    Deprecated op; executes for real against the same per-resource engine DB
    and records to the statement log like the other ops. resultFrame is now
    populated for query statements (records + resultSetMetadata), converted
    from the same engine row extraction ExecuteStatement uses, at the wire
    boundary into the older Value union (bigIntValue/bitValue, not
    longValue/booleanValue) -- gopherstack-7ows. Left nil for DML, which
    still only reports numberOfRecordsUpdated.}
# Families audited as a group (when per-op is impractical):
families:
  routing: {status: ok, note: >
    RouteMatcher gates on SigV4 service name ("rds-data") + one of the 6 fixed
    paths (/Execute, /BatchExecute, /BeginTransaction, /CommitTransaction,
    /RollbackTransaction, /ExecuteSql); verified against
    aws-sdk-go-v2/service/rdsdata's serializers.go request paths -- all match.}
  field_union: {status: ok, note: >
    Field{isNull,booleanValue,longValue,doubleValue,stringValue,blobValue,
    arrayValue} now models every member of the real Field union, including
    arrayValue (types.FieldMemberArrayValue / types.ArrayValue), fixed this
    pass -- see Notes. It is structurally present but functionally
    unreachable in a *result* (the pure-Go SQLite driver never produces an
    array-typed column), matching real AWS's own inability to emit one from
    ExecuteStatement/BatchExecuteStatement.}
  result_set_options: {status: ok, note: >
    resultSetOptions.{decimalReturnType,longReturnType} implemented this
    pass: previously accepted nowhere on the wire. See Notes for the exact
    shaping rules and the one deliberate default-behavior change this
    introduces.}
  transaction_lifecycle: {status: ok, note: >
    Verified id allocation, isolation across regions (isolation_test.go),
    commit/rollback removing the id from the active set so reuse 400s, and
    snapshot/restore round-tripping open transactions + the txCounter.
    gopherstack-02w (fixed this pass): BeginTransaction's doc comment
    (rdsdata@v1.35.4 api_op_BeginTransaction.go) states "A transaction can
    run for a maximum of 24 hours. A transaction is terminated and rolled
    back automatically after 24 hours" and "A transaction times out if no
    calls use its transaction ID in three minutes" -- neither was
    implemented, so a caller that began a transaction and never committed or
    rolled it back leaked it (and its engine-side *sql.Tx) forever. Fixed via
    janitor.go's Janitor: Transaction now carries CreatedAt/LastActivityAt
    (statements.go's touchTransactionLocked refreshes the latter on every
    ExecuteStatement/BatchExecuteStatement against that id), and a
    worker.Group-based sweep (wired through Handler.WithJanitor/StartWorker,
    provider.go) rolls back and evicts anything past either threshold. See
    janitor_test.go.}
  error_codes: {status: ok, note: >
    TransactionNotFoundException (400) and BadRequestException (400, via
    ErrValidation/errIsValidation) cover every error path this mock can
    produce; both are real modeled exceptions in types/errors.go.
    validateNoArrayParameters (fixed this pass) rejects an arrayValue
    parameter as BadRequestException, per real AWS's documented "Array
    parameters are not supported" -- the exact error class AWS returns for
    this case has not been independently verified against a live API call
    (the SDK doc comment states the constraint but not the wire error), so
    this is a best-effort, not a field-diffed, error mapping; flagged in
    items_still_open below rather than claimed as fully verified. No
    resourceArn/secretArn existence validation is performed (mock has no
    cluster registry), so NotFoundException/ForbiddenException/
    AccessDeniedException/ServiceUnavailableError/StatementTimeoutException
    are unreachable by design -- consistent with an emulator that doesn't
    simulate IAM or Aurora Serverless timeouts.}
gaps:                     # known divergences NOT fixed
  - "Database/Schema (ExecuteStatement, BatchExecuteStatement, BeginTransaction,
    ExecuteSql -- all 4 ops that carry them) are decoded off the wire and never
    read anywhere (cmd/reqfieldscan, 2026-08-30 pass: 8 of rdsdata's 9 flagged
    fields). Real AWS's Database overrides the database named by resourceArn's
    connection/secret, and Schema (PostgreSQL only) overrides search_path --
    both select *within* a resource. gopherstack's sqlEngine keys its one
    SQLite database per (region, resourceARN) only (engine.go's dbFor/dbKey);
    there is no per-resource multi-database or schema catalog for these
    fields to select into, matching this service's existing typeHint gap
    (see above) and its siblings' repeated honest-gap pattern in this
    campaign. Confirmed via grep: no `.Database`/`.Schema` selector anywhere
    in non-test source. Not fixed: modeling multiple named databases/schemas
    inside one engine instance is a real feature (SQLite ATTACH DATABASE per
    name, or a schema-qualified table namespace), not a field-read fix."
  - "SqlParameter.typeHint (DATE/DECIMAL/JSON/TIME/TIMESTAMP/UUID) is
    accepted on the wire but does not change bind behavior -- the mock
    SQLite engine has no distinct DATE/TIMESTAMP/UUID column types to
    convert strings into, so a DATE-hinted value binds identically to an
    unhinted string. Re-examined this pass and deliberately NOT implemented:
    real AWS's exact behavior for a malformed hinted value (which error
    class, and whether it's a request-time or DB-execution-time failure) is
    not independently verifiable without a live Aurora cluster, and
    inventing that mapping would risk exactly the kind of
    gopherstack-invented error semantics this audit is supposed to catch.
    Only matters if a test asserts on hint-driven type coercion or
    validation."
  - "ColumnMetadata.SchemaName/TableName/IsAutoIncrement/ArrayBaseColumnType
    are always zero-valued. database/sql's sql.ColumnType (the only
    introspection the pure-Go modernc.org/sqlite driver exposes) has no
    origin-table/schema/autoincrement accessor, so there is no real signal to
    populate them from without a hand-rolled SQL catalog query per column
    keyed by the column's origin table -- which sql.ColumnType also does not
    expose. (Contrast with generatedFields/UpdateResult, which needed the
    origin table but got it for free by parsing it out of the INSERT
    statement itself; a SELECT's result columns have no such textual anchor
    in the general case, e.g. `SELECT * FROM t JOIN u`.)"
leaks: {status: clean, note: >
  sqlEngine.reset() rolls back every open *sql.Tx and closes every resourceDB
  (including its keep-alive conn) before clearing the maps; Handler.Reset()
  delegates to Backend.Reset() which calls engine.reset(). The
  hasRowIDAliasColumn PRAGMA lookup runs synchronously on the same querier
  (already-held connection/tx) as the triggering statement, under the
  existing sqlEngine.mu, and is closed via `defer rows.Close()`.
  gopherstack-02w (fixed this pass): the prior "no goroutines, tickers, or
  other background work" framing missed the actual leak -- an unbounded
  `map[string]*Transaction` growing forever from transactions a caller began
  and never committed/rolled back, with no expiry. See transaction_lifecycle
  above; janitor.go now runs a background Janitor (a goroutine, started via
  Handler.StartWorker) that reaps them, matching the ticker-based pattern
  services/codebuild's janitor.go already uses -- this package is no longer
  goroutine-free, and that's the fix, not a regression.}
---

## Notes

**generatedFields (fixed this pass).** Previously always an empty array for
both ExecuteStatement and BatchExecuteStatement (flagged but left as a
"deliberate simplification" in the prior two audits, since it needed a 5th
backend-method return value threaded through ~30 call sites). Implemented
this pass: `StorageBackend.ExecuteStatement` now returns `([][]Field,
[]ColumnMetadata, int64, []Field, error)`; the new `[]Field` is
`generatedFieldsFor` (engine.go), which recognizes a simple, unquoted
`INSERT INTO <table>` statement, checks via `PRAGMA table_info(<table>)`
whether the table declares exactly one `INTEGER PRIMARY KEY` column (SQLite's
documented rowid alias -- https://sqlite.org/lang_createtable.html#rowid),
and if so surfaces `sql.Result.LastInsertId()` as a single `longValue`. Every
other case (no such column, a composite primary key, UPDATE/DELETE/DDL, or a
quoted/bracketed table identifier the regexp doesn't match) returns an empty
slice -- the same safe historical default. This is a real, verifiable
behavior (not a fabricated ID): it mirrors Aurora MySQL's AUTO_INCREMENT
generatedFields support, and real AWS's own doc comment confirms
`generatedFields` is meaningless for Aurora PostgreSQL. All ~35
`ExecuteStatement` call sites across the test suite were mechanically updated
to the new 5-return signature.

**resultSetOptions (fixed this pass).** Previously accepted nowhere on the
wire. Implemented per the exact SDK doc comments on
`types.ResultSetOptions`: `longReturnType` (default `LONG`, or `STRING`)
shapes INTEGER-affinity result columns; `decimalReturnType` (default
`STRING`, or `DOUBLE_OR_LONG`) shapes DECIMAL/NUMERIC-affinity result
columns. Threaded from the handler to the engine via a new
`resultSetOptionsContextKey` (store.go), mirroring the existing
`regionContextKey` pattern, rather than adding a rarely-used parameter to
`StorageBackend.ExecuteStatement` that nearly every call site would have to
pass a zero value for. **Deliberate default-behavior change:** implementing
the real default (`decimalReturnType=STRING`) means a DECIMAL/NUMERIC-affinity
column's value is now always rendered as a `stringValue` unless the caller
explicitly requests `DOUBLE_OR_LONG` -- previously such a column's Field
shape depended on whatever raw Go type the driver happened to scan
(int64/float64/string). This is intentional: it's what real AWS does by
default, and no existing test asserted a Field *value* shape for a
NUMERIC/DECIMAL-affinity column (only `TestEngine_ColumnMetadata_TypeAffinity`
asserted the `type` code, which is unaffected). A computed/literal column
with no declared type (e.g. `SELECT 42`, `COUNT(*)`) resolves to BLOB
affinity per `sqliteAffinity`'s existing rule 3, so resultSetOptions never
touches it -- consistent with pre-existing behavior, not a regression.

**arrayValue (fixed this pass).** `Field.ArrayValue *ArrayValue` and a new
`ArrayValue` struct (mirroring `types.ArrayValue`'s five members) were added
so a client sending `"arrayValue": {...}` in a parameter round-trips through
JSON instead of being silently dropped by `json.Unmarshal` (previously: the
unknown key was ignored and the parameter bound as an effective NULL). Real
AWS documents "Array parameters are not supported" for both
`ExecuteStatementInput.Parameters` and `BatchExecuteStatementInput.
ParameterSets`; `validateNoArrayParameters` (handler.go) now enforces that,
rejecting the request as `BadRequestException` before it reaches the engine.
See items_still_open for why the exact error class is a best-effort
inference rather than a verified fact.

**continueAfterTimeout (fixed this pass, wire-only).** Added to
`executeStatementRequest` so it round-trips instead of silently vanishing.
Remains a deliberate no-op: this mock has no statement-execution timeouts to
continue past, so there is no divergent behavior to implement -- consistent
with `StatementTimeoutException` being unreachable by design (see
error_codes family note).

**FormatRecordsAs, ColumnMetadata full shape, typeHint wire round-trip**
(fixed in the prior pass, unchanged this pass): see the two audits' worth of
history in git blame if needed; summary retained from the previous manifest
version below.

- `formatRecordsAs=JSON` on a SELECT statement (checked via the existing
  `isQuery` heuristic) omits `records`/`columnMetadata` and instead returns
  `formattedRecords`, a JSON string containing an array of row objects keyed
  by column name (Field union values unwrapped to native JSON; blobs
  base64-encoded). Invalid enum values are rejected as BadRequestException.
- `ColumnMetadata` carries the full real-AWS 14-field shape; `type`/
  `isSigned`/`isCaseSensitive` are derived from SQLite's documented type
  affinity algorithm (see `sqliteAffinity`); `nullable` and `precision`/
  `scale` reflect modernc.org/sqlite's driver limits (verified from driver
  source, not guessed).
- `SqlParameter.typeHint` round-trips on the wire; see gaps for why it still
  doesn't affect bind semantics.

**Trap for the next auditor:** `ExecuteStatement`/`BatchExecuteStatement`
degrade SQL the mock SQLite engine rejects (e.g. DML against a table that was
never created) to the historical empty-success envelope rather than
surfacing an error (`statements.go`'s `ExecuteStatement`/
`BatchExecuteStatement` swallow `b.engine.execute`'s error deliberately).
This looks like a swallowed bug on first read but is intentional,
pre-existing, documented behavior ("historical lenient behaviour") -- don't
re-flag it without checking the surrounding comments first.

**Trap for the next auditor #2:** a column named `"42"` is real, not a typo
-- SQLite's pure-Go driver names literal/expression result columns after
their source text when there's no explicit AS alias (e.g. `SELECT 42` yields
a column literally named `"42"`). `TestHandler_ExecuteStatement_
FormatRecordsAsJSON` reads the column name back dynamically for this reason
rather than asserting a fixed key.

**Trap for the next auditor #3:** `generatedFieldsFor`'s table-name regexp
(`insertIntoTableRe`) only matches a bare, unquoted identifier immediately
after `INSERT [OR <resolution>] INTO`. An INSERT against a quoted/
bracket-escaped table name (`INSERT INTO "my table"...`) silently degrades
to no generated fields rather than erroring -- this is the same safe-default
philosophy as the rest of the engine's lenient-fallback behavior, not an
oversight; don't "fix" it into attempting identifier unquoting without
checking whether that's actually needed by a real test first.

## gopherstack-o7gx (2026-08-22): ReadBody-failure path wrote untyped errors

`Handler()`'s `httputils.ReadBody` failure branch wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")` --
plain text, not JSON. rdsdata is restjson1 (confirmed from `rdsdata@v1.35.4`
deserializers.go's `awsRestjson1_deserializeOpError*` prefix), whose
client-side error decoder (`aws-sdk-go-v2@v1.43.4`
`aws/protocol/restjson.GetErrorInfo`) JSON-decodes the body for a
`code`/`__type` field; plain text doesn't decode, so a real client got
`*json.SyntaxError`, not even `UnknownError`.

Fixed by writing `{"__type": "InternalServerErrorException", "message":
"internal server error"}` instead (new `writeInternalServerError` helper).
`InternalServerErrorException` is rdsdata's own modeled internal error
(`rdsdata@v1.35.4` `types/errors.go:230`). Also promoted the file's
previously-inline `"__type"` literal to a `keyTypeField` constant (3
occurrences after this fix; `goconst` flagged it).

Proven with a real `aws-sdk-go-v2/service/rdsdata` client's
`ExecuteStatement`, whose `Sql` field alone exceeds
`httputils.MaxRequestBodyBytes` (16 MiB) -- legitimate SDK input.
`TestHandler_OversizedBodySurfacesInternalServerErrorException`
(`handler_oversized_body_test.go`) asserts `apiErr.ErrorCode() ==
"InternalServerErrorException"`; confirmed it fails pre-fix with
`*json.SyntaxError` (hand-reverted, byte-identical restore after).

NOT touched: `handleError`'s `errInvalidRequest`/`errUnknownAction`/
syntax/type-error catch-all and its `default:` fallback are themselves
untyped (`map[string]string{keyMessageField: err.Error()}`, no `__type`)
-- a pre-existing, separate gap in the genuine per-operation error path,
not the ReadBody-failure path this fix addresses. Left alone.
## rdsdata (this session, 2026-08-20)

Wrapper-key / nested-shape wire-parity sweep, the last service of a
160-service campaign. Independently re-derived every op's field list from
the pinned SDK (`aws-sdk-go-v2/service/rdsdata@v1.35.4`) rather than trusting
the prior audit's notes, per this campaign's method. **Result: zero bugs
found.** The prior two passes (see Notes above, and `d39bf33e4` 2026-08-11)
had already fixed every real gap this sweep would have caught (arrayValue,
generatedFields, resultSetOptions, ExecuteSql's resultFrame); this pass is a
from-scratch confirmation, not a rubber stamp.

- **Ops (6, confirmed against `ls api_op_*.go` at the pin, not assumed):**
  `BatchExecuteStatement`, `BeginTransaction`, `CommitTransaction`,
  `ExecuteSql`, `ExecuteStatement`, `RollbackTransaction`. `ExecuteSql`
  exists at v1.35.4 (marked Deprecated in its doc comment, not removed) and
  is fully implemented in gopherstack (`sql.go`), including the
  `resultFrame`/legacy `Value` union added 2026-08-11.
- **Protocol:** REST-JSON 1 (`awsRestjson1_*` in serializers.go/
  deserializers.go), matching `services/_PROTOCOLS.md`'s row for this
  service. All 6 ops are `POST` to a fixed, argument-free path (verified via
  `grep -n "SplitURI\|request.Method" serializers.go`): `/Execute` (L445),
  `/BatchExecute` (L44), `/BeginTransaction` (L157), `/CommitTransaction`
  (L253), `/ExecuteSql` (L344), `/RollbackTransaction` (L580) -- all match
  `handler.go`'s `pathExecute`/etc. constants and `RouteMatcher`/
  `ExtractOperation` exactly; no SigV4-path collisions found.
  `awsRestjson1_deserializeOpDocumentExecuteStatementOutput` (deserializers.go:939)
  read line-by-line: it's a real `map[string]interface{}` JSON walk with a
  per-key `switch`, not a passthrough -- no cnhp trap on this service.
- **`Field` union, 7 members (the brief said six -- off by one; the real
  union is `arrayValue`, `blobValue`, `booleanValue`, `doubleValue`,
  `isNull`, `longValue`, `stringValue`; confirmed via
  `types.Field` interface in types/types.go and both
  `awsRestjson1_serializeDocumentField` (serializers.go:748) and its
  deserializer counterpart (deserializers.go:2383-2484), same 7 keys, same
  spelling, both directions). `models.go`'s `Field` struct has all 7 as
  `omitempty` pointer/slice members; JSON keys match case-for-case.
  `engine.go`'s `fieldFromValue` (encode) and `fieldToValue` (decode, request
  side) each populate/read exactly one member per call -- verified by
  reading both function bodies, not just their signatures. NULL is
  distinguished from a zero value because every member is a `*T` (or `[]byte`
  for blob): a SQL NULL produces `Field{IsNull: &true}` (`v == nil` case in
  `fieldFromValue`), while e.g. an empty string produces
  `Field{StringValue: &""}` -- a non-nil pointer to the zero value, never
  confused with the null branch. Verified live: `SELECT 1.0/0.0` (a case the
  real serializer special-cases as `"NaN"`/`"Infinity"`/`"-Infinity"` string
  literals, serializers.go:769-780) returns SQL `NULL` from
  modernc.org/sqlite, not a float, so gopherstack's plain `*float64`
  `DoubleValue` (no NaN/Inf special-casing) is unreachable by the engine, not
  a latent bug -- confirmed by running the query against the actual driver,
  not assumed.
- **`ArrayValue` union, 5 members** (`arrayValues`, `booleanValues`,
  `doubleValues`, `longValues`, `stringValues` -- types.go's
  `ArrayValueMember*` set), matches `models.go`'s `ArrayValue` struct
  field-for-field. Recursion (`arrayValues []ArrayValue`) verified both
  directions: the SDK's `ArrayValueMemberArrayValues.Value []ArrayValue`
  recurses on the same interface; gopherstack's `ArrayValues []ArrayValue`
  recurses on the same concrete struct -- Go's `encoding/json` handles the
  self-referential struct natively on both encode and decode. Functionally
  unreachable in a *result* (same reasoning as the prior audit: the pure-Go
  driver never produces an array-typed column) and rejected on the *request*
  side by `validateNoArrayParameters` (handler.go:282, wired into both
  `ExecuteStatement` and `BatchExecuteStatement`'s per-parameter-set loop) --
  checked both call sites, not just one.
- **`formattedRecords`**: real AWS wire type is a JSON **string**
  (`FormattedRecords *string` in ExecuteStatementOutput, decoded via
  `value.(string)` type-assert at deserializers.go:966-971, not a nested
  object). gopherstack's `formatRecordsAsJSONString` (handler.go:391) builds
  a Go `string` and assigns it into the response map, which `json.Marshal`
  re-encodes as a JSON string literal -- correct type, verified against the
  deserializer's own type assertion rather than inferred from the field
  name.
- **Enums, both directions, all 4:** `DecimalReturnType` (`STRING`,
  `DOUBLE_OR_LONG`), `LongReturnType` (`STRING`, `LONG`), `RecordsFormatType`
  (`NONE`, `JSON`), `TypeHint` (`DATE`, `DECIMAL`, `JSON`, `TIME`,
  `TIMESTAMP`, `UUID`) -- all real typed enums in types/enums.go (not plain
  strings), all 2/2/2/6 values reproduced exactly in `handler.go`'s
  `decimalReturnType*`/`longReturnType*`/`formatRecordsAs*` constants and
  `models.go`'s `SQLParameter.TypeHint` doc comment, validated on request
  ingress (`validateResultSetOptions`, `validateFormatRecordsAs`) and emitted
  unchanged on egress. `TypeHint` is accepted but not bind-semantic (existing
  documented gap, unchanged this pass -- see gaps above).
- **Request/response field-list diff, all 6 ops, every member incl.
  optional, checked by type against the SDK Input/Output structs in their
  own `api_op_*.go`:** `ExecuteStatementInput` (11 fields),
  `ExecuteStatementOutput` (5 wire fields), `BatchExecuteStatementInput` (7),
  `BatchExecuteStatementOutput` (1), `BeginTransactionInput`/`Output` (4/1),
  `CommitTransactionInput`/`Output` (3/1), `RollbackTransactionInput`/
  `Output` (3/1), `ExecuteSqlInput`/`Output` (5/1, no `TransactionId` member
  on this legacy op -- confirmed absent, not omitted by oversight) -- all
  match gopherstack's request/response structs 1:1, no missing, no
  fabricated, no wrong-typed members found.
- **Error wire shape:** confirmed both `TransactionNotFoundException` and
  `BadRequestException` are members of every op's own
  `awsRestjson1_deserializeOpError<Op>` switch (spot-checked
  `ExecuteStatement`'s, deserializers.go:840-923) and are read via
  `X-Amzn-ErrorType` header falling back to a JSON-body `code`/`__type`/
  `message` triad (`restjson.GetErrorInfo`, aws-sdk-go-v2 internal
  decoder_util.go) -- gopherstack's `handleError` (handler.go:203) emits
  `__type` + `message` (lowercase, matching `GetErrorInfo`'s
  case-insensitive `Message` field match) with HTTP 400 for both, which is
  what the real SDK's error switch expects to parse successfully.
- **No wrong-key tests found to correct.** `handler_sdk_route_table_test.go`
  (added 2026-08-15, `69bbb940a`) independently re-derived the same 6
  method+path pairs from the same source this pass did and matches.
- **Provenance:** `last_audit_commit: 9419636f` / `last_audit_date:
  2026-07-23` from the prior stamp predates two real follow-up passes that
  changed this service's behavior without advancing the stamp:
  `d39bf33e4` (2026-08-11, `sdk_module` bumped v1.32.19->v1.35.4,
  `ExecuteSql` resultFrame implemented, +217 lines across models.go/sql.go/
  engine_test.go) and `69bbb940a` (2026-08-15, new route-table test, no
  behavior change). **The stamp did not advance across those passes** even
  though real work landed -- both commits' content was independently
  re-verified against the pinned SDK this pass rather than trusted, and both
  turned out correct. Advanced this pass to `914e8b59` / 2026-08-20.
- **Brief accuracy:** the brief's "`Field` is a union with six members" is
  off by one -- the real union has 7 (see above). "Six ops" and the
  `ExecuteSql`-still-exists question both checked out exactly as briefed.
- **Gates:** `go build ./services/rdsdata/...` clean; `go vet` clean;
  `go fix -diff` empty; `gofmt -l` empty; `go test -race ./services/rdsdata/...`
  ok (1.16s); `golangci-lint run ./services/rdsdata/...` 0 issues; no banned
  cyclop/gocyclo/gocognit/funlen nolints; `git status --short` shows nothing
  under `services/rdsdata/` touched (this pass made no code changes, only
  this PARITY.md stamp/notes update).

## 2026-08-30 (request-field axis sweep, gopherstack-4shm's class)

Ran `cmd/reqfieldscan -dir rdsdata`: dispatch table 6/6 resolved (100%, all
via the literal-decode path -- rdsdata never uses `service.JSONOpFunc`/
`service.WrapOp`), 9 unread fields flagged. **Result: zero bugs, all 9 honest
gaps.**

- `executeStatementRequest.ContinueAfterTimeout` (1 field): already
  documented (see `ExecuteStatement`'s `ops:` note above and the
  "continueAfterTimeout" Notes entry) -- accepted on the wire as a
  deliberate no-op, since this mock has no statement-execution timeouts to
  continue past. Re-confirmed, not re-opened.
- `Database`/`Schema` on `executeStatementRequest`, `batchExecuteStatementRequest`,
  `beginTransactionRequest`, `executeSQLRequest` (8 fields): newly documented
  this pass, see the `gaps:` entry above -- `sqlEngine.dbFor` keys one SQLite
  database per `(region, resourceARN)` only (`engine.go`), so there is no
  per-resource multi-database/schema catalog for these fields to select
  into. Matches this service's existing `typeHint` gap and its siblings'
  repeated pattern in this campaign of honest, no-backend-state gaps rather
  than defects.

No code changes this pass -- PARITY.md documentation only. Gates unaffected
(no source touched): `go build`, `go vet`, `go test -race`, `golangci-lint
run` all still green per the entries above.

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

Same defect and fix as the census in `cmd/reqfielddiff`/`cmd/reqfieldscan`
(ef0eef041, appsync e2643a6dd). This package's `Sql`/`SQL` acronym casing
gives it 1 op/handler pairs needing the ambiguous fold, 1 of them
genuine collisions between an exported backend method and the real
unexported handler: `ExecuteSql`.

Verified directly rather than assumed: ran the unpatched tool from
`ef0eef041~1` five times and diffed against the fixed tool at HEAD, for
both `cmd/reqfieldscan` and `cmd/reqfielddiff`. Both were byte-identical
across all 5 old runs and HEAD (6 SDK operations compared) -- the
determinism defect never flipped a finding here, because the resolution
that actually mattered (this package's dispatch-table union) already
carried the correct field set regardless of which fold candidate won.

Verdict: confirmed zero damage, not merely predicted.
