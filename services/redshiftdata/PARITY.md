---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: redshiftdata
sdk_module: aws-sdk-go-v2/service/redshiftdata@v1.43.4   # version audited against
last_audit_commit: eff9b1496                              # HEAD when this audit began (working tree, uncommitted)
last_audit_date: 2026-09-04
overall: A            # genuine wire-shape/field gaps found and fixed this pass
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  ExecuteStatement: {wire: ok, errors: ok, state: ok, persist: ok, note: >
    Sets Status=FINISHED synchronously (deterministic mock, acceptable per parity rules).
    FIXED 2026-08-20 (wrapper-key sweep, gopherstack-cnhp campaign): statementCreateResponse
    (shared by ExecuteStatement/BatchExecuteStatement) never emitted "Status" or
    "HasResultSet" -- both are real ExecuteStatementOutput members (confirmed against
    awsAwsjson11_deserializeOpDocumentExecuteStatementOutput's case list,
    aws-sdk-go-v2/service/redshiftdata@v1.43.4/deserializers.go), and Status is the single
    most behaviorally load-bearing field this op returns (this backend completes every
    statement synchronously to FINISHED, so a real typed client checking output.Status
    immediately after the call previously saw a zero-value "" instead). Regression test:
    TestExecuteStatement_Status_SDKRoundTrip (wire_sdk_roundtrip_test.go), hand-reverted to
    confirm it fails with Status=="" / HasResultSet==nil before the fix.
    Prior-pass fixes retained: (1) ClientToken is now real idempotency, not just accepted-and-dropped
    -- a retried request with the same ClientToken (scoped by op+region) replays the
    original statement Id instead of creating a second statement (idempotency.go), matching
    the "ensure the idempotency of the request" doc comment and the SDK client's
    auto-generation of a token when the caller omits one (api_op_ExecuteStatement.go);
    follows the identical precedent in services/scheduler/idempotency.go. (2) Database was
    unconditionally required (ValidationException if absent) -- more restrictive than real
    AWS: ExecuteStatementInput.Database's doc comment says only "required when
    authenticating using either Secrets Manager or temporary credentials" (conditional, not
    a hard trait), and validateOpExecuteStatementInput has no Database check at all (unlike
    ListDatabasesInput/ListSchemasInput/ListTablesInput/DescribeTableInput, which DO have
    "This member is required" + a matching validator check). Requirement removed to match.
    SessionId remains pure passthrough (no session-scoped state exists to gate minting a
    fresh id when absent, see gaps). SessionKeepAliveSeconds remains accepted-but-inert --
    see gaps. DbGroups still not returned (optional field, gap).}
  BatchExecuteStatement: {wire: ok, errors: ok, state: ok, persist: ok, note: >
    QueryString=Sqls[0] matches AWS; sub-statements built with fixed HasResultSet=false
    (AWS doesn't run real SQL so this is a simplification, see gaps). FIXED 2026-08-20:
    same statementCreateResponse Status/HasResultSet fix as ExecuteStatement above (shared
    function) -- BatchExecuteStatementOutput's deserializer has the identical "Status"/
    "HasResultSet" case list. Regression test: TestBatchExecuteStatement_Status_SDKRoundTrip.
    Prior-pass fixes retained: ClientToken idempotency and Database-required relaxation,
    identical treatment and identical SDK-source citations as ExecuteStatement above
    (BatchExecuteStatementInput's Database/ClientToken doc comments and validator are the
    same shape). SessionId/SessionKeepAliveSeconds: same treatment as ExecuteStatement.}
  DescribeStatement: {wire: ok, errors: ok, state: ok, persist: ok, note: >
    FIXED 2026-08-20 (wrapper-key sweep): statementToDescribeResponse emitted FOUR
    fabricated members with no case at all in the real DescribeStatementOutput
    deserializer -- confirmed against aws-sdk-go-v2/service/redshiftdata@v1.43.4's
    DescribeStatementOutput struct (api_op_DescribeStatement.go) and
    awsAwsjson11_deserializeOpDocumentDescribeStatementOutput's case list
    (deserializers.go): "IsBatchStatement", "StatementName", and "QueryStrings" are real
    members of the WIDER StatementData shape (the ListStatements item type, see
    statementToListItem) generalized onto the narrower DescribeStatementOutput where they
    don't exist; "WithEvent" doesn't exist on ANY response shape in the SDK at all -- it is
    request-only, on ExecuteStatementInput/BatchExecuteStatementInput. A real typed client
    silently drops unknown keys (not a hard failure), so this was wire pollution, not a
    (c)-class break. Deleted all four. Regression test:
    TestDescribeStatement_NoFabricatedFields (wire_sdk_roundtrip_test.go), hand-reverted to
    confirm each of the four reappears without the fix. Three pre-existing tests that
    asserted the wrong (fabricated) shape were corrected: TestHandler_DescribeStatement_AllFields
    (StatementName), TestHandler_BatchExecuteStatement_AllFields (IsBatchStatement/
    QueryStrings, moved the assertion to a ListStatements call where those fields actually
    belong), TestWithEvent_StoredAndReturned (renamed
    TestWithEvent_AcceptedButNotEchoed, inverted to assert absence).
    Prior-pass fixes retained: QueryParameters conditionally included when non-empty,
    SessionId conditionally included, Duration in nanoseconds, SubStatements
    RedshiftQueryId=0. RedshiftPid still not returned (optional field, always absent
    instead of 0, gap).}
  GetStatementResult: {wire: ok, errors: ok, state: ok, persist: n/a, note: "unchanged this pass; ColumnMetadata key length / Records stringValue union previously field-diffed correct. Records/ColumnMetadata are deterministic mock data (acceptable, no real query engine)"}
  GetStatementResultV2: {wire: ok, errors: ok, state: ok, persist: n/a, note: "unchanged this pass; Records []{CSVRecords} union / columnSize->length previously field-diffed correct"}
  CancelStatement: {wire: ok, errors: ok, state: ok, persist: ok, note: >
    Re-verified this pass (gopherstack-ilos): the handler already validates fully before
    mutating -- empty Id -> errMissingID, unknown Id -> ErrNotFound (ResourceNotFoundException,
    confirmed both TestCancelStatement_NotFound and the not-found case are already covered),
    only THEN checks terminal state and mutates. There is no missing-precondition bug here.
    It always errors with ErrTerminalState (ValidationException) in practice because
    ExecuteStatement/BatchExecuteStatement finish synchronously to FINISHED within the same
    call, so no statement is ever observed running by the time a client could call
    CancelStatement -- this is the correct, honest consequence of the synchronous-completion
    design, not a bug, and matches the real API's own doc comment ("Cancels a running query.
    To be canceled, a query must be running.", api_op_CancelStatement.go). See gaps.}
  ListStatements: {wire: ok, errors: ok, state: ok, persist: ok, note: >
    Fixed this pass: list items were missing QueryStrings (batch statements), QueryParameters,
    and SessionId, all three real StatementData members -- now conditionally included
    alongside the already-correct Id/Status/QueryString/IsBatchStatement/CreatedAt/UpdatedAt/
    ResultFormat/StatementName/SecretArn. FIXED 2026-08-13: deleted the six non-real fields
    (ClusterIdentifier/WorkgroupName/Database/DbUser/HasResultSet/Duration) that were being
    sent beyond StatementData -- see gaps below for the SDK citation. RoleLevel accepted but
    unused (see gaps). FIXED 2026-09-04 (gopherstack-2v1): ListStatementsInput's
    ClusterIdentifier/WorkgroupName field docs each unambiguously state "When
    providing ClusterIdentifier, then WorkgroupName can't be specified" (and the
    mirrored sentence on WorkgroupName) -- confirmed against
    aws-sdk-go-v2/service/redshiftdata@v1.43.4's api_op_ListStatements.go -- but
    handleListStatements never enforced it, silently returning an empty (not
    matching any statement) 200 for a request specifying both. ListSessions
    already enforces the identical constraint (ValidateListSessionsRequest,
    sessions.go) since its ListSessionsInput doc carries the same sentence;
    ListStatements was the gap. Neither field is required (ListStatementsInput
    has no "This member is required" trait on either, and no validateOp
    function exists for it in validators.go at all), so only the both-set case
    is now rejected, matching ValidateListSessionsRequest's precedent. New
    ValidateListStatementsConnectionTarget (models.go), wired into
    handleListStatements (handler_statements.go). Regression test:
    TestHandler_ListStatements_RejectsBothClusterAndWorkgroup
    (handler_statements_validation_test.go), hand-reverted to confirm it fails
    (200 instead of 400) before the fix.}
  ListDatabases: {wire: ok, errors: ok, state: gap, persist: n/a, note: >
    Fixed this pass: (1) Database is a required ExecuteStatementInput-style member on the
    real ListDatabasesInput (confirmed against api_op_ListDatabases.go's "This member is
    required" doc comment) but was never validated -- a request omitting it now correctly
    400s with ValidationException instead of silently returning the full demo list. (2)
    NextToken was unconditionally emitted as "" even when no more pages existed (the map
    literal always set the key); real NextToken is an optional *string that's nil, not an
    empty string, once fully paginated -- now conditionally included like every other
    List* op in this package. Still a static demo list, not backed by any real database
    registry (acceptable per parity rules, deterministic mock).}
  ListSchemas: {wire: ok, errors: ok, state: gap, persist: n/a, note: >
    Fixed this pass: Database-required validation added (same reasoning as ListDatabases).
    ConnectedDatabase field added to the request struct (previously entirely unhandled,
    a real ListSchemasInput member) -- accepted but does not affect filtering since this
    mock's demo schema list isn't modeled per-database, consistent with how ClusterIdentifier/
    WorkgroupName/DbUser/SecretArn are already accepted-but-unused identity/auth fields here.
    Static demo list + SQL LIKE pattern filter, unchanged otherwise.}
  ListTables: {wire: ok, errors: ok, state: gap, persist: n/a, note: >
    Fixed this pass: DELETED the invented TableType request field and its filter logic --
    field-diffed against api_op_ListTables.go/serializers.go and confirmed TableType does
    not exist anywhere in the real SDK (ListTablesInput only has ClusterIdentifier/
    ConnectedDatabase/Database/DbUser/MaxResults/NextToken/SchemaPattern/SecretArn/
    TablePattern/WorkgroupName). Removed TestHandler_ListTables_TableType_FiltersCorrectly
    (tested invented behavior) along with it. ConnectedDatabase field added (real member,
    was missing) and Database-required validation added (same reasoning as ListDatabases).}
  DescribeTable: {wire: ok, errors: ok, state: gap, persist: n/a, note: >
    Fixed this pass: ConnectedDatabase field added (real DescribeTableInput member, was
    missing) and Database-required validation added (same reasoning as ListDatabases).
    Prior-pass fix retained: TableName is a plain string (was a nested object). ColumnList
    is static demo data ignoring req.Schema/req.Table (acceptable mock).}
  ListSessions: {wire: ok, errors: ok, state: partial, persist: n/a, note: >
    NEW this pass (SDK added this op since v1.41.0; confirmed target
    "RedshiftData.ListSessions" against awsAwsjson11_serializeOpListSessions in
    aws-sdk-go-v2/service/redshiftdata@v1.43.0's serializers.go). This backend has no
    CreateSession/CloseSession API and does not store sessions as a first-class resource
    -- ListSessions derives SessionData by grouping stored Statement records that share a
    non-empty SessionID (groupSessions in sessions.go): ClusterIdentifier/WorkgroupName/
    Database/DbUser/UpdatedAt come from the most-recently-updated statement in the group,
    CreatedAt from the earliest. Field-diffed request/response against
    awsAwsjson11_serializeOpDocumentListSessionsInput and
    awsAwsjson11_deserializeDocumentSessionData in the same module. Validation added for
    the two documented mutual-exclusivity constraints (SessionId can't combine with
    Status/ClusterIdentifier/WorkgroupName/Database; ClusterIdentifier and WorkgroupName
    can't both be set) and for the Status enum. Pagination follows this package's
    NextToken-is-the-last-item's-ID convention (sessionPageStart), same as ListStatements.
    Status is real but always AVAILABLE in practice: BUSY is unreachable because
    ExecuteStatement/BatchExecuteStatement complete synchronously (same root cause as the
    pre-existing CancelStatement gap), and CLOSED is unreachable because
    SessionAliveSeconds/SessionTtl are not tracked (see gaps) so no expiry can ever fire.
    RoleLevel is accepted on the wire but not applied as a filter, identical to the
    pre-existing ListStatements RoleLevel gap. Sessions only appear here when the caller
    explicitly supplied SessionId to ExecuteStatement/BatchExecuteStatement -- this mock
    does not mint a session id when only SessionKeepAliveSeconds is given (pre-existing
    gap, see ExecuteStatement note above), so such sessions are invisible to ListSessions
    too; that is a real, if narrow, gap, hence state: partial rather than ok.}
# Families audited as a group (when per-op is impractical):
families:
  statement-lifecycle: {status: ok, note: "unchanged this pass -- SUBMITTED/PICKED/STARTED never observable -- ExecuteStatement/BatchExecuteStatement complete to FINISHED synchronously within the same call, so no client ever polls a non-terminal state (no hang bug). CancelStatement is real code but practically unreachable given synchronous completion (see gaps)."}
  persistence: {status: ok, note: >
    Handler.Snapshot/Restore delegate to InMemoryBackend.Snapshot/Restore (persistence.go);
    versioned snapshot (redshiftdataSnapshotVersion), ring buffer round-trips correctly
    (persistence_test.go). Statement gained a new SessionID field this pass
    (json:"sessionID,omitempty") and BatchExecuteStatement now populates the pre-existing
    Parameters field -- both are additive/omitempty on an already-JSON-serialized struct,
    so old snapshots decode safely with SessionID="" and no version bump was needed.}
  error_codes: {status: ok, note: >
    ValidationException and ResourceNotFoundException (types/errors.go) are the only two
    error types this backend can actually produce, and both are field-diffed this pass:
    ErrorFault Client -> HTTP 400 for both (confirmed against types/errors.go's
    ErrorFault() methods), matching handler.go's handleError mapping exactly.
    InternalServerException (ErrorFault Server -> HTTP 500) is used as the generic fallback
    for unclassified errors in handleError's default case -- also correct. See gaps for why
    ActiveStatementsExceededException/ActiveSessionsExceededException/
    DatabaseConnectionException/ExecuteStatementException/BatchExecuteStatementException/
    QueryTimeoutException are real modeled exceptions in the SDK but unreachable by design.}
gaps:
  - CancelStatement can never succeed against this backend: ExecuteStatement/BatchExecuteStatement set Status=FINISHED synchronously, so by the time a client calls CancelStatement the statement is always already terminal and CancelStatement always returns ErrTerminalState (ValidationException). This matches real AWS semantics ("To be canceled, a query must be running") given the backend's synchronous-completion design. Not fixed this pass -- would require modeling async statement execution (a state machine with a delay before reaching FINISHED), which is a larger behavioral change beyond a wire-shape/bug-fix pass.
  - "STALE ENTRY, superseded 2026-09-04: this used to say ValidateConnectionTarget was never called and the permissive behavior was deliberate. That verdict does not survive gopherstack-2v1's re-check -- commit 448dd7f82 (this same repo, dated 2026-09-04, already an ancestor of the branch this note is being written on) wired ValidateConnectionTarget into ExecuteStatement/BatchExecuteStatement for real (statements.go:32,96) and rewrote the three tests that had asserted the permissive behavior into TestHandler_ExecuteAndBatchExecuteStatement_RejectInvalidConnectionTarget, which now asserts rejection of both-set and neither-set. gopherstack-2v1 re-verified this against the SDK rather than trusting the prior commit's own claim: ExecuteStatementInput/BatchExecuteStatementInput's ClusterIdentifier/WorkgroupName field doc comments (api_op_ExecuteStatement.go/api_op_BatchExecuteStatement.go) only say each is 'required when connecting to a cluster/workgroup and authenticating using...' -- conditional per-field language, never the explicit 'When providing ClusterIdentifier, then WorkgroupName can't be specified' sentence that ListSessionsInput and ListStatementsInput both carry verbatim (confirmed absent via grep across every api_op_*.go in the module for 'can't be specified'/'cannot be specified'/'mutually exclusive'). So the both-set rejection on ExecuteStatement/BatchExecuteStatement is NOT literally spelled out in the SDK the way it is for ListSessions/ListStatements -- it rests on the reasonable but not textually-proven inference that the doc's three enumerated auth combinations (each naming exactly one of ClusterIdentifier/WorkgroupName) implies the pair is exclusive, consistent with every other op in this family that does state it explicitly. Left as-is (not reverted): defensible inference, matches this API family's own established pattern, already has deep test coverage, and was independently verified by 448dd7f82's own author against the unfixed code failing the same regression tests. Flagging the wire-shape distinction here so a future audit doesn't cite it as SDK-unambiguous when re-deriving parity for other services. See the ListStatements row above for a companion case (2026-09-04) where the identical constraint genuinely IS literally stated in the SDK and gopherstack was NOT enforcing it -- that one was a real, unambiguous gap and is now fixed."
  - DescribeStatement does not return RedshiftPid (optional field, always absent instead of 0); DbGroups not returned by ExecuteStatement/BatchExecuteStatement. Both are optional wire fields the real client zero-values when absent, so not a functional gap, just lower fidelity -- no group/pid registry exists in this mock to source real values from.
  - SessionKeepAliveSeconds is accepted on ExecuteStatement/BatchExecuteStatement's wire (unmarshalled into the request struct) but is accepted-then-silently-dropped: it never reaches the backend call and has no effect. Session keep-alive/expiry requires modeling time-bounded session lifetimes this in-memory backend does not have; inventing it risks fabricating undocumented AWS semantics not verifiable without a live cluster (same reasoning as rdsdata's typeHint gap). Relatedly, this mock does NOT mint a fresh SessionId when SessionKeepAliveSeconds>0 and no SessionId is supplied (real AWS would start a new session and return its id) -- SessionId here is pure passthrough of whatever the caller already provided, since there's no session-scoped state (temp tables, transaction visibility, etc.) that a minted id would actually gate. (ClientToken was in this same category through last pass -- now fixed, see ExecuteStatement/BatchExecuteStatement rows and idempotency.go.)
  - RoleLevel is parsed on ListStatements' and ListSessions' request bodies but never applied as a filter (accepted-then-silently-dropped: decoded into the request struct but never placed on ListStatementsFilter/ListSessionsFilter, so it never reaches the backend at all): real semantics are "true (default) = all statements/sessions this IAM role has run, false = only this IAM session's," but this mock has no per-caller-identity or per-IAM-session model, so there is no signal to filter on. All statements/sessions are visible regardless of RoleLevel, matching the "true" default in effect at all times.
  - ActiveStatementsExceededException/ActiveSessionsExceededException/ExecuteStatementException (modeled on ExecuteStatement's error deserializer) and BatchExecuteStatementException (BatchExecuteStatement's), DatabaseConnectionException/QueryTimeoutException (CancelStatement's), and ActiveWaitingRequestsExceededException (DescribeStatement's/GetStatementResult's/GetStatementResultV2's -- previously missing from this gap entry entirely) are all real modeled exception types, confirmed this pass by grepping each operation's awsAwsjson11_deserializeOpError<Op> function in aws-sdk-go-v2/service/redshiftdata@v1.43.4's deserializers.go for its strings.EqualFold(...) cases (NOT literal `case "X":` labels). All are unreachable by design in this backend: ExecuteStatement/BatchExecuteStatement always complete synchronously and successfully against in-memory demo data (no real cluster connection to fail, no concurrent-statement/session limit tracked, no waiting-request queue). Deliberately NOT implemented this pass: inventing trigger conditions (e.g. an arbitrary "N active statements" cap, or making some ClusterIdentifier/SecretArn values fail with DatabaseConnectionException) would fabricate gopherstack-only behavior with no real-AWS trigger to field-diff against -- consistent with rdsdata's precedent of leaving unreachable-by-design SDK exceptions undone rather than guessing.
  - "CLOSED 2026-08-13: ListStatements items included six fields (ClusterIdentifier, WorkgroupName, Database, DbUser, HasResultSet, Duration) that don't exist on the real StatementData shape at all. Evidence: aws-sdk-go-v2/service/redshiftdata@v1.43.4, types/types.go, checked 2026-08-13 -- types.StatementData's exhaustive field list is Id/CreatedAt/IsBatchStatement/QueryParameters/QueryString/QueryStrings/ResultFormat/SecretArn/SessionId/StatementName/Status/UpdatedAt; all 12 are now populated (statically or conditionally) by statementToListItem, no inverse (missing real field) found. The six fabricated fields are real DescribeStatementOutput members instead (a different, wider type -- statementToDescribeResponse legitimately keeps them). Deleted from statementToListItem (handler_statements.go). Raw-body regression test: TestListStatements_NoFabricatedFields (handler_statements_semantics_test.go)."
  - ListSessions (new this pass) never returns Status=BUSY or Status=CLOSED, and never returns SessionAliveSeconds/SessionTtl/CurrentStatementId at all: this backend executes every statement synchronously to a terminal state (no mid-flight window to observe BUSY/CurrentStatementId) and does not track SessionKeepAliveSeconds expiry (no SessionTtl to compare "now" against, so CLOSED can never be derived). Modeling any of these would require the same async-execution and keep-alive state machine already flagged as out-of-scope for CancelStatement/ClientToken/SessionKeepAliveSeconds above -- not invented here for the same reason. ListSessions also can't see sessions that were only ever referenced via SessionKeepAliveSeconds without an explicit SessionId (this mock doesn't mint one, see ExecuteStatement's note).
deferred:
  - none
leaks: {status: clean, note: "Janitor uses pkgs/worker.Group with TaskTimeout bounding; ticker stops cleanly via ctx.Done(); ring buffer + statements map bounded by maxStatementHistory and EvictExpiredStatements TTL sweep. New state this pass (Handler.idempotency, a safemap.Map) introduces no goroutine/ticker -- it's plain in-memory data, TTL-based lazy eviction on lookup (same pattern as services/scheduler/idempotency.go), and cleared on Handler.Reset."}
---

## Notes

### 2026-09-04 pass (gopherstack-2v1, parity sweep): ListStatements connection-target gap found and fixed; ExecuteStatement/BatchExecuteStatement mutual-exclusivity re-verified

Chartered specifically to re-check a prior campaign's "test encoding a bug"
finding: whether ExecuteStatement/BatchExecuteStatement's permissive
ClusterIdentifier+WorkgroupName handling was still true, and whether the SDK
unambiguously requires exactly one of the two. Found that commit 448dd7f82
(already on this branch, dated the same day) had already wired
`ValidateConnectionTarget` into both ops and rewritten the permissive tests --
the finding as originally briefed no longer exists in the tree. Re-derived the
justification independently against the SDK rather than trusting the prior
commit's message (see the superseded gaps entry above for the full citation):
the "not both" constraint is explicit, word-for-word, on `ListSessionsInput`
and `ListStatementsInput` ("When providing ClusterIdentifier, then
WorkgroupName can't be specified") but is nowhere in `ExecuteStatementInput`/
`BatchExecuteStatementInput`'s doc comments -- those only carry the weaker,
conditional "required when connecting to a cluster/workgroup and
authenticating using..." per-field language. The already-applied
ExecuteStatement/BatchExecuteStatement validation is a defensible inference
(matches the family's established pattern, has real test coverage) but is not
literally SDK-proven the way ListSessions/ListStatements's is -- left as-is,
flagged for a future audit's benefit rather than reverted.

That distinction directly located a real, previously undetected bug: despite
`ListStatementsInput` stating the exact same "can't be specified together"
sentence that `ListSessionsInput` does (and `ListSessions` already enforcing
it via `ValidateListSessionsRequest`, sessions.go), `handleListStatements`
(handler_statements.go) never validated it -- a request with both
ClusterIdentifier and WorkgroupName silently returned `200 {"Statements":[]}`
instead of `400 ValidationException`. Added
`ValidateListStatementsConnectionTarget` (models.go, only rejects both-set,
since neither field is required per the SDK) and wired it into
`handleListStatements`. Regression test:
`TestHandler_ListStatements_RejectsBothClusterAndWorkgroup`
(handler_statements_validation_test.go) -- hand-reverted the wiring (not the
validator) to confirm the `both_set_returns_400` subtest fails (200 instead of
400, `{"Statements":[]}` body) before the fix, then restored.

Also re-confirmed via `awk`-scoped per-op error-code switches in
`deserializers.go` that all four error codes this backend emits
(ValidationException/ResourceNotFoundException/InternalServerException,
already documented; no fourth code found) are present in every one of the 12
ops' own `awsAwsjson11_deserializeOpError<Op>` functions -- no cross-op
error-code bug. No cross-service wiring to services/redshift,
redshiftserverless, or secretsmanager exists for ClusterIdentifier/
WorkgroupName/SecretArn validation (already documented as accepted-but-unused
identity fields; confirmed no hook exists rather than assuming one should).
Janitor/ring-buffer/idempotency-cache leak surface re-read, no new issue
(matches the existing `leaks: clean` verdict).

Gates: `GOTOOLCHAIN=go1.26.6 go build ./services/redshiftdata/...`,
`go test -race -count=1 ./services/redshiftdata/...` (ok), `golangci-lint run
./services/redshiftdata/...` (0 issues), and the mandated dependents check
`go test -race -count=1 ./services/cloudformation/... ./services/redshift/...`
(both ok).

### 2026-08-21 pass (gopherstack-r80d batch 32): required-output-member audit, clean

Part of the mgn/redshiftdata/scheduler batch testing r80d's op-count-vs-
field-count hypothesis (see `services/_REQUIRED_OUTPUT_CANDIDATES.md`);
redshiftdata is one of the two 12-op control services (tied with
`scheduler` at 5 required output fields). Module resolves directly
(directory `redshiftdata` == SDK module `aws-sdk-go-v2/service/
redshiftdata@v1.43.4`, no override). `cmd/requiredoutputfields` found 5
required output fields across 5 ops (`DescribeStatement.Id`,
`GetStatementResult(V2).Records`, `ListSessions.Sessions`,
`ListStatements.Statements`); cross-checked via a standalone `go/ast`-style
walk of the same `api_op_*.go` files -- agreed exactly.

Every one of these 5 ops builds its response as a `map[string]any` literal
passed straight to `json.Marshal` (`statementToDescribeResponse`,
handler_statements.go:401; `handleGetStatementResult`'s inline literal,
handler_statements.go:174-201; `handleListSessions`/`handleListStatements`,
handler_sessions.go:13-58/handler_statements.go:263-312) -- the same
structural immunity batch 30/31 found in ssoadmin/mediatailor/shield/
translate: there is no struct tag for an `omitempty` mistake to hide behind.
`Id`/`Sessions`/`Statements` are all assigned unconditionally (`Sessions`/
`Statements` via `make([]map[string]any, 0, len(...))`, never nil).

Followed the required members one level below the flat scan (this
hypothesis's point): an AST walk of `redshiftdata@v1.43.4/types/types.go`
found `SessionData` (`CreatedAt`/`SessionId`/`Status`, reachable via
`ListSessions.Sessions`) and `StatementData`/`SubStatementData` (`Id`,
reachable via `ListStatements.Statements` and `DescribeStatement.
SubStatements`) each carry required members of their own. All are built via
the same unconditional map-literal pattern (`sessionToListItem`,
handler_sessions.go:75-100; the `SubStatements` loop, handler_statements.go:
461-486) with backend-generated, never-empty IDs (`uuid.NewString()`,
statements.go:110). `SqlParameter.{Name,Value}` (reachable via
`DescribeStatement.QueryParameters`, itself optional) round-trips through
gopherstack's own `SQLParameter{Name,Value}` with lowercase `name`/`value`
JSON tags -- confirmed correct, not a casing bug, by reading the real
`awsAwsjson11_deserializeDocumentSqlParameter`'s key switch directly (this
protocol lowercases some nested-object member names while keeping top-level
op fields PascalCase, e.g. `Id`/`Records`/`Sessions`/`Statements` stay
capitalized -- verified per-field against the deserializer rather than
assumed from one example).

0 bugs found; no code changes. Companion services this batch: `mgn` (95
ops, the batch's primary hypothesis test, also 0 bugs) and `scheduler` (2
bugs; see `services/scheduler/PARITY.md`).

Protocol: awsjson1.1 (single POST endpoint, `X-Amz-Target: RedshiftData.<Op>`). Verified
the exact target prefix against `aws-sdk-go-v2/service/redshiftdata@v1.41.0`'s
`serializers.go` (`httpBindingEncoder.SetHeader("X-Amz-Target").String("RedshiftData.<Op>")`)
-- `redshiftDataTargetPrefix = "RedshiftData."` in handler.go matches exactly.

### This pass (2026-08-20, wrapper-key / nested-shape sweep): audited every op's
response for wrong wrapper keys, wrong nesting, wrong JSON types, and fabricated members,
focused especially on the `Field`/`FormattedField` union family the campaign flagged as
highest-risk.

**`Field` union (`GetStatementResult`'s `Records [][]types.Field`): CLEAN.** Confirmed
against `awsAwsjson11_deserializeDocumentField` (deserializers.go): six discriminator
keys, `blobValue` (base64 string -> `[]byte`), `booleanValue` (JSON bool), `doubleValue`
(JSON number, or "NaN"/"Infinity"/"-Infinity" strings), `isNull` (JSON bool),
`longValue` (JSON number), `stringValue` (JSON string). gopherstack's demo row emits
`{"stringValue": "mock_value"}` -- correct key, correct JSON type (a bare string). No
`isNull` emission exists in this mock's static demo data (it never emits a NULL cell), so
NULL-vs-zero-value discrimination isn't exercised, but that's a data-coverage gap, not a
wire-shape bug -- the one row it does emit is field-diffed correct.

**`FormattedField` does NOT exist anywhere in `aws-sdk-go-v2/service/redshiftdata@v1.43.4`**
(confirmed: `grep -rn "FormattedField"` over the whole module cache directory returns zero
matches). The campaign brief's description of `GetStatementResultV2` returning
`Records [][]FormattedField` describes a *different, newer* SDK version than the one this
repo has pinned. At v1.43.4, `GetStatementResultV2Output.Records` is `[]types.QueryRecords`
-- a **different union family**, one member only (`CSVRecords`, a comma-joined string per
row), confirmed against `awsAwsjson11_deserializeDocumentQueryRecords`. gopherstack already
emits the correct shape here (`{"CSVRecords": "mock_value"}`, a prior-pass fix, unchanged
this pass) -- there is no `FormattedField`-shaped bug to find against the pinned version.
Re-audit this note if `go.mod` is ever bumped past v1.43.4: re-derive from
`api_op_GetStatementResultV2.go`/`deserializers.go` at the new pinned version rather than
trusting this description.

**Protocol / false-positive trap check**: confirmed `awsjson1.1`
(`awsAwsjson11_*` prefix, `X-Amz-Target: RedshiftData.<Op>`), and
`grep -c 'awsAwsjson11_deserializeOpDocument<Op>Output'` returns exactly 2 (defined AND
called) for all 12 ops -- the restjson flat-body trap does not apply, every op's response
genuinely routes through its `OpDocument` helper.

**`StatementData` vs `SubStatementData` vs `DescribeStatementOutput`**: three distinct
real shapes, confirmed field-by-field against their own struct definitions in
`types/types.go` / `api_op_DescribeStatement.go`. `statementToListItem` (StatementData) was
already exhaustively correct (prior pass). `statementToDescribeResponse`
(DescribeStatementOutput) had the fabricated-member bug fixed this pass (see the
DescribeStatement row above) -- it was leaking three real StatementData-only members
(IsBatchStatement/StatementName/QueryStrings) plus one member (WithEvent) that isn't a
response field on *any* shape in the SDK. The `SubStatements` sub-mapping inside
`statementToDescribeResponse` was independently verified clean against `SubStatementData`'s
own field list (Id/CreatedAt/Duration/Error/HasResultSet/QueryString/RedshiftQueryId/
ResultRows/ResultSize/Status/UpdatedAt) -- no extraneous fields there.

**`ExecuteStatementOutput`/`BatchExecuteStatementOutput`** (shared `statementCreateResponse`)
had the Status/HasResultSet omission bug fixed this pass (see ExecuteStatement/
BatchExecuteStatement rows above).

**Families re-verified CLEAN this pass, no changes**: `ListDatabases`/`ListSchemas`/
`ListTables` (plain `[]string` list bodies, `Databases`/`Schemas` deserializer keys
confirmed list-of-string not list-of-object; `TableMember`'s lowercase `name`/`schema`/
`type` keys match `keyName`/`keySchema`/`keyType` exactly); `DescribeTable` (`ColumnList`/
`TableName` keys, `ColumnMetadata` shape reused correctly); `SqlParameter` (lowercase
`name`/`value` keys, matches `SQLParameter`'s json tags exactly); `CancelStatement`
(`CancelStatementOutput.Status` is a **bare JSON boolean**, a different type from every
other "Status" field in this service which are string enums -- gopherstack correctly emits
`{"Status": true}`, not a string, avoiding the exact (c)-class trap the campaign brief
warned about); enum values (`StatementStatusString`/`StatusString`/`SessionStatusString`/
`ResultFormatString` -- every value gopherstack emits, `FINISHED`/`ABORTED`/`FAILED`/
`AVAILABLE`/`JSON`/`CSV`/`ALL`, is a real SDK constant, no invented enum values found);
`ListSessions`/`SessionData` (`sessionToListItem`'s emitted keys are an exact subset of the
real `SessionData` struct, the omitted `CurrentStatementId`/`SessionAliveSeconds`/
`SessionTtl` are already-documented gaps, not fabrications); error codes (unchanged,
re-confirmed `ValidationException`/`ResourceNotFoundException` both `FaultClient` -> 400).

### This pass (2026-08-10, gopherstack-ilos): worked the recorded follow-up ticket.

`sdk_module` was stale (recorded v1.43.0, `go.mod` pins v1.43.4) -- diffed the two module
cache directories (`diff -rq`) and confirmed only `CHANGELOG.md`/`go.mod`/`go.sum`/
`go_module_metadata.go` differ (v1.43.1-.4 were all dependency-only bumps per their
CHANGELOG entries, no operation/type changes), so the substance of the prior audit still
holds; corrected the version citation here, in README.md, and in handler_sessions.go's
doc comment.

Verdicts on the three recorded items:
1. **ClientToken/SessionKeepAliveSeconds/RoleLevel**: ClientToken was accepted-then-
   silently-dropped (decoded, never passed to the backend call) -- fixed for real, see the
   ExecuteStatement/BatchExecuteStatement rows and new `idempotency.go`. SessionKeepAliveSeconds
   and RoleLevel remain accepted-then-silently-dropped (RoleLevel is decoded but never even
   placed on the filter struct) -- both genuinely require modeling state (session TTL,
   per-IAM-session identity) this backend does not have; not invented, see gaps.
2. **CancelStatement**: re-tested the framing. It already validates fully before mutating
   (empty Id, not-found Id, terminal-state Id are all rejected before any write) and already
   rejects a statement ID that does not exist with ResourceNotFoundException
   (`TestCancelStatement_NotFound`, pre-existing and passing). There was no missing
   precondition to fix. It always errors in practice purely because
   ExecuteStatement/BatchExecuteStatement complete synchronously to FINISHED, so no statement
   is ever observably running -- an honest structural consequence, not a bug, and it matches
   real AWS's own doc comment ("To be canceled, a query must be running").
3. **ActiveStatements/Sessions/DatabaseConnection exceptions**: confirmed via
   `strings.EqualFold` (not literal `case` labels) in each operation's
   `awsAwsjson11_deserializeOpError<Op>` in `deserializers.go` that these ARE real,
   correctly-attributed modeled exceptions (ActiveStatementsExceededException/
   ActiveSessionsExceededException/ExecuteStatementException on ExecuteStatement,
   BatchExecuteStatementException on BatchExecuteStatement, DatabaseConnectionException/
   QueryTimeoutException on CancelStatement, and ActiveWaitingRequestsExceededException --
   previously undocumented here at all -- on DescribeStatement/GetStatementResult/
   GetStatementResultV2). All are unreachable by design (no concurrency/connection/queue
   modeling in this backend) and were left unimplemented rather than invented triggers --
   see gaps for the full reasoning.

Sweep fix beyond the three recorded items: **Database was unconditionally required on
ExecuteStatement/BatchExecuteStatement (ValidationException if absent), more restrictive
than real AWS.** `validateOpExecuteStatementInput`/`validateOpBatchExecuteStatementInput`
in `validators.go` have no Database check, and the field's doc comment says only
"required when authenticating using either Secrets Manager or temporary credentials" --
unlike `ListDatabasesInput`/`ListSchemasInput`/`ListTablesInput`/`DescribeTableInput`,
whose Database members carry "This member is required" AND a matching
`smithy.NewErrParamRequired("Database")` validator check. Removed the requirement from
`(*InMemoryBackend).ExecuteStatement`/`BatchExecuteStatement`; the two pre-existing tests
that locked in the wrong (over-strict) behavior,
`TestExecuteStatement_DatabaseRequired`/`TestBatchExecuteStatement_DatabaseRequired`, were
rewritten as `TestExecuteStatement_DatabaseOptional`/`TestBatchExecuteStatement_DatabaseOptional`
asserting the correct 200 response.

ClientToken idempotency: confirmed the pre-fix bug first (`TestExecuteStatement_ClientToken_ReplaysOnRetry`
failed with two distinct statement Ids from one ClientToken before the fix). Implemented a
`safemap.Map[string, idempotentStatement]` cache on `Handler`, keyed by
`op:region:clientToken`, with a 5-minute TTL matching the identical precedent in
`services/scheduler/idempotency.go`. Not persisted (matches scheduler: cleared on `Reset()`,
not part of `backendSnapshot` -- it's a retry-dedup cache, not backend state). No snapshot
version bump needed.

### Prior pass (2026-07-25): AWS SDK bump to v1.43.0 exposed one new operation,

`ListSessions`, which `TestSDKCompleteness` flagged as unhandled. Implemented for real
(not added to a `notImplemented` list): see the `ListSessions` row above for the full
field-diff and derivation summary. Confirmed the wire target string
(`RedshiftData.ListSessions`) and every request/response field name directly against
`aws-sdk-go-v2/service/redshiftdata@v1.43.0`'s `serializers.go`
(`awsAwsjson11_serializeOpDocumentListSessionsInput`) and `deserializers.go`
(`awsAwsjson11_deserializeDocumentSessionData`, `awsAwsjson11_deserializeOpDocumentListSessionsOutput`)
rather than inferring from the operation name. New files: `sessions.go` (backend
derivation: `SessionData`/`ListSessionsFilter` live in `models.go`,
`ValidateListSessionsRequest`/`groupSessions`/`(*InMemoryBackend).ListSessions`/filter+
pagination helpers live in `sessions.go`) and `handler_sessions.go` (request parsing +
wire-shape response building), mirroring the existing `statements.go`/`handler_statements.go`
pairing. No `Statement`/backend-interface signature changes were needed for
`ExecuteStatement`/`BatchExecuteStatement` -- `SessionID` was already threaded through from
a prior pass, and `SessionKeepAliveSeconds` is deliberately not newly wired through those
two ops for this op alone (see the `ListSessions` gaps entry) to avoid inconsistent
partial modeling. No snapshot version bump: `ListSessions` adds no new persisted field to
`Statement` (sessions are purely derived at read time from a `regionStore`'s existing
`statements` map, already fully covered by `backendSnapshot`/`regionSnapshot`), so
`redshiftdataSnapshotVersion` stays 1 and old snapshots decode unchanged.

### Prior pass (2026-07-23): field-diffed every op's Input/Output against
`aws-sdk-go-v2/service/redshiftdata@v1.41.0`'s `api_op_*.go`, `types/types.go`,
`types/errors.go`, and the `serializers.go`/`deserializers.go` wire-key tables. Real gaps
found and fixed (all in `handler_statements.go` / `handler_tables.go` / `handler_databases.go`
/ `statements.go` / `models.go` / `interfaces.go`):

1. **Invented field: `ListTablesInput.TableType`.** gopherstack accepted and filtered on a
   `TableType` request field with no counterpart anywhere in the real SDK -- confirmed by
   grepping `TableType` across the entire `redshiftdata@v1.41.0` module (zero matches) and
   reading `awsAwsjson11_serializeOpDocumentListTablesInput`'s full field list. Deleted the
   field, its filter branch in `filterDemoTables`, and the test built around it
   (`TestHandler_ListTables_TableType_FiltersCorrectly`) that was asserting invented
   behavior.

2. **`BatchExecuteStatementInput.Parameters` was never unmarshalled.** The real API shares
   one `Parameters` list across every SQL statement in a batch (confirmed against
   `awsAwsjson11_serializeOpDocumentBatchExecuteStatementInput`'s `Parameters` key, same
   shape as `ExecuteStatementInput.Parameters`), but `handleBatchExecuteStatement`'s request
   struct didn't even have a `Parameters` field -- any parameters a batch caller sent were
   silently discarded before reaching the backend. Added the field, threaded it through
   `StorageBackend.BatchExecuteStatement` into `Statement.Parameters` (a field that already
   existed and was already used by the single-statement `ExecuteStatement` path).

3. **`DescribeStatementOutput.QueryParameters` was never emitted.** Both `ExecuteStatement`
   and `BatchExecuteStatement` already stored the caller's `Parameters` on the `Statement`
   (confirmed the field existed in `models.go`), but `statementToDescribeResponse` never
   read it back out -- a client that submitted parameterized SQL could never see its own
   parameters echoed back via `DescribeStatement`, unlike real AWS (confirmed wire key
   `"QueryParameters"` and shape `SqlParametersList` against
   `awsAwsjson11_deserializeOpDocumentDescribeStatementOutput`'s `case "QueryParameters":`).
   Also added to `statementToListItem` (`StatementData.QueryParameters` is a real member
   too) and to `StatementData.QueryStrings` (was in `DescribeStatement` but missing from
   `ListStatements` items).

4. **`SessionId` was not modeled anywhere.** `ExecuteStatementInput.SessionId`,
   `BatchExecuteStatementInput.SessionId`, `ExecuteStatementOutput.SessionId`,
   `DescribeStatementOutput.SessionId`, and `StatementData.SessionId` are all real wire
   fields (confirmed against the serializers/deserializers) that gopherstack neither
   accepted nor returned. Added a `Statement.SessionID` field, threaded a `sessionID string`
   parameter through `StorageBackend.ExecuteStatement`/`BatchExecuteStatement`, and
   conditionally echo it on `ExecuteStatementOutput`/`BatchExecuteStatementOutput`/
   `DescribeStatementOutput`/`StatementData` (ListStatements items) whenever the caller
   supplied one. Deliberately does NOT mint a new session id when `SessionKeepAliveSeconds`
   is set without a `SessionId` (see gaps) -- pure passthrough only, to avoid inventing
   session-lifecycle semantics this mock has no state to back up.

5. **`ListDatabasesInput.Database` / `ListSchemasInput.Database` / `ListTablesInput.Database`
   / `DescribeTableInput.Database` are all documented "This member is required"** in their
   respective `api_op_*.go` files, but none of the four handlers validated it -- a request
   omitting `Database` silently returned the full static demo list/columns instead of a
   `ValidationException`. Added the same `Database is required` check `ExecuteStatement`/
   `BatchExecuteStatement` already had.

6. **`ListDatabasesOutput.NextToken`** was unconditionally set to `""` in the response map
   even when the demo database list fit on one page. Real `NextToken` is an optional
   `*string`, `nil` (omitted) once there are no more pages, not an empty string --
   `ListSchemas`/`ListTables`/`ListStatements` already followed the omit-when-empty
   convention; `ListDatabases` was the one outlier. Fixed to match.

7. **`ListSchemasInput.ConnectedDatabase` / `ListTablesInput.ConnectedDatabase` /
   `DescribeTableInput.ConnectedDatabase`** are real wire fields (cross-database query
   support) that were entirely absent from the corresponding request structs -- any value
   sent there was silently ignored by Go's `json.Unmarshal` (not an error, just dropped).
   Added the field to each struct for wire-shape completeness; left unused for filtering
   since this mock's demo schema/table/column lists aren't modeled per-database, consistent
   with how `ClusterIdentifier`/`WorkgroupName`/`DbUser`/`SecretArn` are already
   accepted-but-unused identity/auth fields on these same ops.

None of the existing unit tests asserted the missing `QueryParameters`/`SessionId` fields or
the invented `TableType` filter as *not* present, so these were real, previously-undetected
gaps rather than known-and-tested tradeoffs (the closest test, `TestParameters_AcceptedAndStored`,
only checked "request with Parameters returns 200 and an Id," never that `DescribeStatement`
echoed them back -- extended this pass to assert the round-trip).

### Prior pass (2026-07-13) bug classes (all in `handler.go`, retained, still correct):

1. **`ColumnMetadata` wire key `columnSize` does not exist in the real API.** The real
   field is `length` (int32) -- confirmed against
   `aws-sdk-go-v2/service/redshiftdata/deserializers.go`'s
   `awsAwsjson11_deserializeDocumentColumnMetadata` (`case "length":`). Sending
   `columnSize` meant the real SDK's deserializer silently dropped the value (its
   `default:` case just discards unknown keys), so every client parsing
   `GetStatementResult`, `GetStatementResultV2`, or `DescribeTable` would see
   `ColumnMetadata[i].Length == 0` regardless of what gopherstack sent. Renamed the
   constant `keyColumnSize` -> `keyLength` = `"length"` and updated all 7 call sites
   (`handleGetStatementResult`, `handleGetStatementResultV2`, `buildDemoColumns`).

2. **`DescribeTable`'s `TableName` was a nested `{schema, name, type}` object; the real
   `DescribeTableOutput.TableName` is a plain string.** Confirmed against
   `api_op_DescribeTable.go` (`TableName *string`) and its deserializer
   (`case "TableName":` decodes a bare string). The nested object was silently dropped
   by the real SDK the same way as (1), leaving `TableName` unset. Fixed to
   `"TableName": req.Table`.

3. **`Duration` was emitted in milliseconds; the real field is nanoseconds.** Both
   `DescribeStatementOutput.Duration` and `SubStatementData.Duration` are documented as
   "The amount of time in nanoseconds that the statement ran" (confirmed in both
   `aws-sdk-go-v2` and legacy `aws-sdk-go` v1 doc comments, and the wire key itself,
   `"Duration"`, already matched). Added `durationNanos(ms int64) int64` and applied
   it at the three wire-marshaling call sites (`statementToListItem`,
   `statementToDescribeResponse` top-level and its `SubStatements` loop).

4. **`GetStatementResultV2`'s `Records` was `[]string`; the real field is
   `[]types.QueryRecords`, a union whose only member is `CSVRecords` (a string).**
   Confirmed against the union deserializer
   `awsAwsjson11_deserializeDocumentQueryRecords` (`case "CSVRecords":`, `default:`
   treats anything else as `UnknownUnionMember`). Fixed to
   `[]map[string]any{{"CSVRecords": "mock_value"}}`.

**Looks-wrong-but-correct traps** (don't re-flag):
- `ExecuteStatement`/`BatchExecuteStatement` complete synchronously to `Status=FINISHED`
  in the same call. This is intentional (no real Redshift cluster to run SQL against
  asynchronously) and satisfies the "must reach a terminal state" parity rule trivially
  -- it is not a disguised hang bug.
- `ListStatements` default (`Status` omitted) returns only `FINISHED` statements
  (`matchesStatementStatus`) -- this was a deliberate choice from a prior sweep to match
  documented AWS default filtering behavior; don't flip without re-verifying against
  AWS docs.
- `ValidateConnectionTarget` exists in `models.go` but is intentionally never called
  from `ExecuteStatement`/`BatchExecuteStatement` -- there are named tests
  (`TestHandler_ExecuteStatement_AllowsBothClusterAndWorkgroup`,
  `TestHandler_ExecuteStatement_AllowsNeitherClusterNorWorkgroup`,
  `TestHandler_BatchExecuteStatement_AllowsNeitherClusterNorWorkgroup`) asserting the
  permissive behavior. Treat as a deliberate relaxation, not dead code to wire up.
- `DescribeTable`/`ListDatabases`/`ListSchemas`/`ListTables` return static demo data
  regardless of the requested database/schema/table -- acceptable per the "deterministic
  mock data OK, no real query engine" parity rule; not a stub since the ops still apply
  real filtering/pagination logic to that demo data, and now (this pass) real
  required-field validation too.
- `SessionId` is real and round-trips, but is NOT minted by this mock when absent -- don't
  "fix" `ExecuteStatement` to generate one whenever `SessionKeepAliveSeconds > 0` without
  re-reading the gaps entry above; there is no session-scoped state that would make a
  synthetic id meaningfully different from omitting it.
- `ListSessions` always reports `Status: "AVAILABLE"` and never emits `SessionAliveSeconds`/
  `SessionTtl`/`CurrentStatementId`. This is not an oversight to "complete" -- it is the
  direct, correct consequence of ExecuteStatement/BatchExecuteStatement completing
  synchronously (no BUSY window) and SessionKeepAliveSeconds not being tracked anywhere
  (no TTL to compare against for CLOSED). See the `ListSessions` gaps entry before changing
  this.

## gopherstack-o7gx (2026-08-22): ReadBody-failure path wrote untyped errors

`Handler()`'s `httputils.ReadBody` failure branch wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")`.
redshiftdata is awsjson1.1 (confirmed from `redshiftdata@v1.43.4`
deserializers.go's `awsAwsjson11_deserializeOpError*` prefix, not from
`services/_PROTOCOLS.md`), whose per-operation deserializer still calls
`aws/protocol/restjson.GetErrorInfo` to JSON-decode `__type`/`message` from
the body; plain text doesn't decode, so a real client got
`*json.SyntaxError`, not even `UnknownError`.

Fixed by writing `{"__type": "InternalServerException", "message":
"internal server error"}` (new `writeInternalServerError` helper),
matching this handler's own `handleError` `default:` fallback, which
already uses the same code -- modeled at `redshiftdata@v1.43.4`
`types/errors.go:183`.

Proven with a real `aws-sdk-go-v2/service/redshiftdata` client's
`ExecuteStatement`, whose `Sql` field alone exceeds
`httputils.MaxRequestBodyBytes` (16 MiB). Note: `ExtractResource` also
calls `httputils.ReadBody` earlier in the request lifecycle and swallows
its own error to `""`, so by the time `Handler()` reads the body it hits
"invalid Read on closed Body" rather than "request body too large" --
still the same `ReadBody`-failure branch, same fix.
`TestHandler_OversizedBodySurfacesInternalServerException`
(`handler_oversized_body_test.go`) asserts `apiErr.ErrorCode() ==
"InternalServerException"`; confirmed it fails pre-fix with
`*json.SyntaxError` (hand-reverted, byte-identical restore after).

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

**Two bugs found and fixed** in `paginateStrings` (`handler_databases.go` —
`ListDatabases`, `ListSchemas`, 2 ops) and `paginateMaps`
(`handler_tables.go` — `ListTables`, 1 op), both sharing identical logic:

1. **A new off-by-one, not matching Class A/B/C** — found by the boundary
   walk check itself, independent of any staleness: the encoder emits
   `page[limit]`, the name of the **first item of the next page**, as the
   token, but the decoder treated a match as "the last item already seen"
   and resumed at `i + 1`. Every page boundary silently dropped exactly one
   item, even on a plain, non-stale, back-to-back walk with nothing deleted
   in between — the "silent truncation" shape this whole campaign started
   from, not the stale-cursor shape its two later passes have mostly found.
   `TestPaginateStrings_BoundaryWalk`/`TestPaginateMaps_BoundaryWalk`
   (`pagination_arithmetic_internal_test.go`) fail pre-fix on this alone,
   with no cursor tampering involved.
2. **Class B** — a token naming no known item (a tampered/garbage
   `NextToken`; both backing lists are hardcoded, never-shrinking demo data,
   so a real deletion can't trigger this here, but a malformed client token
   can) left `start` at its zero-value default, restarting at page one
   instead of terminating.

Both are one root cause read two ways: the resume index should be "the
matched position, inclusive" (fixing #1), and the miss default should be
`len(all)`, not `0` (fixing #2). Fixed both helpers identically: on a
match, `start = i` (was `i + 1`); on a miss, `start` defaults to `len(all)`
(was `0`).

3 operations affected. Proven by unit tests against each helper directly,
all failing pre-fix (boundary walk, exact division, and cursor round trip
all failed due to the off-by-one; tampered-cursor failed separately), and
by `TestListDatabases_SDKRoundTrip_BoundaryWalkNoDrop` /
`_TamperedTokenTerminates` (`pagination_sdk_roundtrip_test.go`) through the
real `aws-sdk-go-v2/service/redshiftdata` client. The existing
`TestHandler_ListDatabases_NextToken_ResumesFromCursor` only asserted
`page2[0] != page1[0]`, which is still true when an item is silently
dropped in between — it would not have caught either bug.

All seven checks pass post-fix. `statementPageStart`/`sessionPageStart`
(`statements.go`/`sessions.go`) were also read as part of this census: both
already return `(int, error)` and error on a cursor miss instead of
defaulting to 0 — the found-flag-equivalent pattern this campaign
recommends elsewhere — so no change needed there.

Gates: `go build ./services/redshiftdata/...`,
`go vet ./services/redshiftdata/...` and `go vet ./...` (repo-wide, clean —
no signature changed), `go test -race -count=1
./services/redshiftdata/...`, `golangci-lint run ./services/redshiftdata/...`
(0 issues).

## 2026-08-30 anonymous-struct-decode sweep (gopherstack-4a8v): re-verified clean, no code change

`cmd/reqfieldscan`'s fifth dispatch shape (`service.JSONOpFunc` implemented
directly with anonymous inline request structs, no `WrapOp`) made this
service newly visible to that scanner and flagged 25 fields as unread.
Dispatch coverage: 12/12 (100%), both coverage lines identical, no guard
warning. The originating bd issue (gopherstack-4a8v) spot-checked
`ListDatabases`/`ListTables`/`DescribeTable`'s `WorkgroupName`/
`ClusterIdentifier`/`SecretArn`/`DBUser` fields and called them "genuine,
not tool noise" — that verdict does NOT survive re-verification against
this file's own 2026-08-21 audit (`last_audit_commit: ee8d5788f`): every
one of the 25 flagged fields across `ListDatabases`/`ListSchemas`/
`ListTables`/`DescribeTable` (`WorkgroupName`/`ClusterIdentifier`/
`SecretArn`/`DBUser`/`ConnectedDatabase`/`Schema`) is already the
documented `ops:` gap "accepted-but-unused... this mock's demo
[list/schema/table/column data] is not per-database/cluster/workgroup,
consistent with how ClusterIdentifier/WorkgroupName/DbUser/SecretArn are
already accepted-but-unused identity/auth fields here" (see the
`ListDatabases`/`ListSchemas`/`ListTables`/`DescribeTable` rows above).
Confirmed structurally, not just by the comment: `store.go`'s
`InMemoryBackend`/`regionStore` hold only `statements` (and, via
`sessions.go`, sessions derived from them) — there is no per-cluster or
per-workgroup database/schema/table registry to filter against at all, and
this API family (unlike `ExecuteStatement`'s real `ClusterIdentifier`/
`WorkgroupName` statement filtering in `statements.go:198,202`, which DOES
use them) has no Create/Register operation for databases/schemas/tables in
the real AWS API either — they're a live catalog query against a real
cluster this mock doesn't have.

The remaining flagged fields (`ListSessions`/`ListStatements.RoleLevel`,
`ExecuteStatement`/`BatchExecuteStatement.SessionKeepAliveSeconds`,
`GetStatementResultV2.NextToken`) are likewise pre-existing, already-dated
`gaps:` entries (RoleLevel: no per-IAM-identity model to filter on;
SessionKeepAliveSeconds: no session-expiry state machine; NextToken: this
mock's result sets are always exactly one row, so pagination is a
structural no-op, the same shape as `GetStatementResult`'s sibling gap).

**No code or test changes made to this service this pass.** All 25 flagged
fields are honest, already-documented structural limitations, not new
bugs — restraint per this campaign's own instructions, not a fabricated
clean bill. This service's earlier A-grade verdict holds.

Gates: not re-run (no change); `go build`/`go vet ./...` confirmed clean as
part of this session's repo-wide checks.
