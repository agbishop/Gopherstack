---
service: athena
sdk_module: aws-sdk-go-v2/service/athena@v1.60.4
last_audit_commit: c47d785b7
last_audit_date: 2026-08-28
overall: A            # genuine wire-shape fixes found in a previously well-built, well-tested service
                       # 2026-08-28 (gopherstack-6flj write-only-state sweep): CreateWorkGroup silently
                       # dropped Configuration.EngineConfiguration/MonitoringConfiguration entirely (no
                       # model field existed); EngineConfiguration.Classifications was missing too,
                       # affecting the pre-existing StartSession path as well since real AWS reuses one
                       # EngineConfiguration type for both. Fixed with a real-client round-trip test. See
                       # the WorkGroup op row and Notes.
                       # 2026-08-21 (gopherstack-1vv2): fixed UpdateWorkGroup wholesale-replacing
                       # Configuration with the narrower ConfigurationUpdates payload, destroying
                       # fields (ResultConfiguration/EngineVersion/etc.) any single-field Update
                       # didn't mention. See the WorkGroup op row.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  BatchGetPreparedStatement: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — request field was StatementNames, real wire is PreparedStatementNames; response field was UnprocessedStatementNames, real wire is UnprocessedPreparedStatementNames. Op was silently non-functional for real SDK clients (request always parsed as an empty name list)."}
  GetSessionEndpoint: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED — response was {SessionEndpoint: url}; real shape is {EndpointUrl, AuthToken, AuthTokenExpirationTime} (all three required). Client previously got a fully empty result."}
  CreatePresignedNotebookUrl: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED — response was {NotebookSessionUrl: url}; real shape is {NotebookUrl, AuthToken, AuthTokenExpirationTime} (all three required). Same class of bug as GetSessionEndpoint; both now share backend.newSessionAuthToken()."}
  GetResourceDashboard: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED — was a disguised no-op ignoring the required ResourceARN input and returning {ResourceDashboard: {}}; real shape is {Url: string}. Now validates ResourceARN is non-empty (InvalidRequestException otherwise) and returns a synthesized dashboard URL."}
  StartQueryExecution: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-zgfq) — ResultConfiguration.OutputLocation and EncryptionConfiguration were validated, stored, and echoed back, but no S3 object was ever written, so a client that ran a query and then fetched the result file from OutputLocation found nothing. Added athena.S3Storer + SetS3Backend, wired in cli.go's wireAthenaS3 from services/s3's real PutObject (no adapter needed -- s3.InMemoryBackend.PutObject already matches the interface). On a succeeded execution, writes an object to \"<OutputLocation>/<QueryExecutionId>.csv\": DISCLOSED APPROXIMATION, not a verified wire shape -- the pinned SDK (types.ResultConfiguration.OutputLocation doc) states only that results are stored under that S3 location, and documents neither an object key nor a file format. The .csv body is a plain header-row-then-rows encoding/csv dump of the query's result columns; SSE_S3/SSE_KMS map to the object's ServerSideEncryption/SSEKMSKeyId, CSE_KMS (client-side) is accepted but not actually encrypted (no KMS simulation exists to encrypt against). When S3 is unwired (every test that constructs the backend directly), writeResultObject is a no-op and StartQueryExecution's existing store/echo behavior is unchanged."}
  StopQueryExecution: {wire: ok, errors: ok, state: ok, persist: ok}
  GetQueryExecution: {wire: ok, errors: ok, state: ok, persist: ok, note: "Query lifecycle is synchronous (QUEUED/RUNNING never observed) — StartQueryExecution runs the statement inline and stores a terminal SUCCEEDED/FAILED state before returning, so SDK poll loops never hang."}
  GetQueryResults: {wire: ok, errors: ok, state: ok, persist: ok, note: "ResultSet/Row/Datum/ColumnInfo shapes verified against awsAwsjson11 deserializers; header row only on first page, matching AWS."}
  ListQueryExecutions: {wire: ok, errors: ok, state: ok, persist: ok, note: "opaque-token pagination via pkgs' page-token codec"}
  BatchGetQueryExecution: {wire: ok, errors: ok, state: ok, persist: ok}
  WorkGroup (Create/Get/List/Update/Delete): {wire: ok, errors: ok, state: ok, persist: fixed, note: "FIXED 2026-08-28 (gopherstack-6flj) — WorkGroupConfiguration had no EngineConfiguration or MonitoringConfiguration field at all (both real members on types.WorkGroupConfiguration/types.WorkGroupConfigurationUpdates), so CreateWorkGroup/UpdateWorkGroup silently dropped them; added both, wired through MergeInto for Update's partial-update semantics. Also fixed EngineConfiguration.Classifications ([]types.Classification), missing from the shared EngineConfiguration model used by both WorkGroup and Session. IdentityCenterConfiguration/ManagedQueryResultsConfiguration/QueryResultsS3AccessGrantsConfiguration remain unmodeled -- see gaps. FIXED (2026-07-23) — WorkGroup carried an invented Tags field (real GetWorkGroupOutput.WorkGroup has none; tags are TagResource/ListTagsForResource-only) that also went stale the moment TagResource/UntagResource were called, since those never touched it. Field removed; CreateWorkGroup's Tags input now flows only into resourceTags. Also FIXED (previous pass) — ResultConfiguration.ACLConfiguration was tagged json:\"ACLConfiguration\"; real wire key is \"AclConfiguration\". 2026-08-21 (gopherstack-1vv2): persist was accept-and-corrupt — UpdateWorkGroupInput.ConfigurationUpdates is types.WorkGroupConfigurationUpdates, a partial-update shape a real client only ever sends the changed fields of, but the handler decoded it into the same WorkGroupConfiguration type as Create and the backend wholesale-replaced wg.Configuration with it -- so any single-field Update (e.g. just EnforceWorkGroupConfiguration) silently erased ResultConfiguration/EngineVersion/etc. set at Create. Fixed: new WorkGroupConfigurationUpdates type (pointer scalars, so omitted is distinguishable from explicit false/0/empty) with a MergeInto that only touches fields actually present. See TestHandler_UpdateWorkGroup_PreservesUnmentionedConfiguration. ResultConfigurationUpdates' Remove* explicit-clear flags remain unmodeled -- separate gap, not fixed this pass."}
  NamedQuery (Create/Get/List/BatchGet/Delete/Update): {wire: ok, errors: ok, state: ok, persist: ok}
  DataCatalog (Create/Get/List/Update/Delete): {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-07-23) — CreateDataCatalogOutput/DeleteDataCatalogOutput now populate the optional DataCatalog object (SDK v1.57.2) with the created/just-deleted record. Also FIXED — DataCatalog carried the same invented Tags field as WorkGroup (see above); removed, CreateDataCatalog's Tags input now flows only into resourceTags."}
  PreparedStatement (Create/Get/List/BatchGet/Delete/Update): {wire: ok, errors: ok, state: ok, persist: ok}
  CapacityReservation (Create/Get/List/Update/Cancel/Delete): {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-07-23) — CapacityReservation carried the same invented Tags field as WorkGroup/DataCatalog, but worse: CreateCapacityReservation had never built an ARN or written to resourceTags at all, so a capacity reservation's tags were previously unreachable via TagResource/ListTagsForResource entirely (no arn.Build call existed for this resource kind). Added InMemoryBackend.capacityReservationARN and wired Create/Delete to mirror/cascade-clean resourceTags like WorkGroup/DataCatalog already did."}
  CapacityAssignmentConfiguration (Put/Get): {wire: ok, errors: ok, state: ok, persist: ok}
  Notebook (Create/Delete/Export/Import/Update/UpdateMetadata/GetMetadata/ListMetadata): {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-07-23) — CreateNotebookInput carried an invented Tags field; the real CreateNotebookInput has only Name/WorkGroup/ClientRequestToken (unlike WorkGroup/DataCatalog/CapacityReservation, notebooks cannot be tagged at creation in the real API). Removed; a client sending Tags anyway (as no real SDK client would) is now harmlessly ignored rather than silently accepted. A notebook remains taggable after creation via TagResource against its ARN."}
  Session (Start/Get/GetStatus/Terminate/List/ListNotebookSessions): {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-28 (gopherstack-6flj) — EngineConfiguration.Classifications ([]types.Classification{Name,Properties}) was missing from the shared EngineConfiguration model, affecting StartSession the same way it affected CreateWorkGroup; see the WorkGroup row. FIXED (gopherstack-cgq3) — StartSession was missing the real optional MonitoringConfiguration field (types.MonitoringConfiguration: CloudWatchLoggingConfiguration/ManagedLoggingConfiguration/S3LoggingConfiguration, per GetSessionOutput.MonitoringConfiguration). Now accepted, stored on Session, and echoed by GetSession, matching the real API's own StartSession->GetSession round trip. StartSession's own request struct also still carries a SessionConfiguration field with no counterpart on the real StartSessionInput (only GetSessionOutput has SessionConfiguration, and it's workgroup-derived there, not client-supplied) — out of this fix's scope, left as-is and noted here for a future pass."}
  Calculation (Start/Get/GetStatus/GetCode/Stop/List): {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-21 (gopherstack-us9u kind-mismatch sweep) -- CalculationStatistics.Progress was int64, hardcoded to 100 on every calculation; the real types.CalculationStatistics.Progress is *string (deserializers.go case \"Progress\": expected DescriptionString to be of type string), so every real SDK client's GetCalculationExecutionStatus/GetCalculationExecution call failed outright since Progress is always populated. Fixed by changing the field to string (now \"COMPLETED\"). Proven via a real aws-sdk-go-v2/service/athena client round trip (wire_calculation_progress_test.go), hand-reverted/confirmed-failing (expected DescriptionString to be of type string, got json.Number instead)/restored, md5sum-verified byte-identical."}
  Database/TableMetadata (Get/List): {wire: ok, errors: ok, state: ok, persist: ok, note: "'dirty' tables round-trip through the DTO registry in persistence.go; verified by persistence_test.go (the store_setup_test.go filename this note previously cited does not exist in the tree — stale reference, the coverage itself is real and passing). FIXED (gopherstack-yabd) — GetDatabase/ListDatabases/GetTableMetadata/ListTableMetadata never branched on a DataCatalog's Type, so a GLUE-type catalog (including the built-in AwsDataCatalog, which real AWS backs with the account's Glue Data Catalog) was always served from Athena's own internal database/table simulation, never from services/glue. Added GlueMetadataSource (interfaces.go) + SetGlueMetadataSource, wired in cli.go's wireAthenaGlue from services/glue's real GetDatabase/GetDatabases/GetTable/GetTables. When unwired (e.g. every test that constructs the backend directly), GLUE-type catalogs keep falling back to the internal simulation — permissive by default, matching this repo's cross-service hook convention."}
  Tags (Tag/Untag/ListTagsForResource): {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-07-23) — TagResource/UntagResource/ListTagsForResource now validate ResourceARN resolves to a currently existing taggable resource (workgroup/datacatalog/capacity-reservation/notebook, parsed from the ARN's kind/id resource segment), returning InvalidRequestException (ErrNotFound) otherwise instead of silently no-oping or returning an empty tag list. ListTagsForResource now also honors MaxResults/NextToken pagination (previously ignored both, always returning every tag in one response)."}
  ListEngineVersions: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED (2026-07-23) — EngineVersionDescriptor carried a fabricated AuthEngineVersion field that does not exist on the real EngineVersion type (only SelectedEngineVersion/EffectiveEngineVersion); removed."}
  ListApplicationDPUSizes: {wire: ok, errors: ok, state: ok, persist: n/a}
  ListExecutors: {wire: ok, errors: ok, state: ok, persist: n/a}
  GetQueryRuntimeStatistics: {wire: ok, errors: ok, state: ok, persist: n/a}
families:
  pagination: {status: ok, note: "FIXED (2026-07-23) — WorkGroups/NamedQueries/DataCatalogs/PreparedStatements/(new) ListTagsForResource all sort + NextToken/MaxResults correctly, and an unrecognized/stale NextToken (e.g. its boundary item was deleted between calls) now resumes at the first surviving item at-or-after the boundary (pagination.paginationStart, mutation-stable via sort.Search) instead of silently restarting the page from offset 0 and re-emitting already-consumed results. Locked in by TestListWorkGroups_Pagination_StaleTokenResumesStably."}
  janitor/leaks: {status: clean, note: "worker.Group-based ticker with ctx cancellation; sweeps queryExecutions+queryResults, sessions, calculations under RLock-collect/Lock-delete with re-verification to avoid racing a concurrent revival. No goroutine leak risk found."}
gaps:
  - DeleteDataCatalogInput.DeleteCatalogOnly (real SDK v1.57.2 field, FEDERATED-catalog-only) is not modeled as a request input; gopherstack does not simulate the underlying CFN Stack/Lambda/Glue Connection resources a FEDERATED catalog's deletion would otherwise need to selectively preserve, so the flag would have no observable effect either way in this emulator. Not a wire-shape break (an extra unrecognized request field is harmlessly ignored). (bd: unfiled)
  - "WorkGroupConfiguration.IdentityCenterConfiguration/ManagedQueryResultsConfiguration/QueryResultsS3AccessGrantsConfiguration (real members on types.WorkGroupConfiguration/types.WorkGroupConfigurationUpdates, confirmed 2026-08-28 via serializers.go) remain unmodeled — each is a substantial real feature (IAM Identity Center-gated workgroups, Athena-managed query-result-object lifecycle, S3 Access Grants) this emulator does not simulate end to end, not a quick wire-shape passthrough. WorkGroup.IdentityCenterApplicationArn (the paired response field) likewise unmodeled. (bd: unfiled)"
  - "QueryExecution.SubstatementType (real *string member on types.QueryExecution, e.g. further classifying a DDL StatementType as CTAS) is not modeled — found 2026-08-28 field-diffing types.QueryExecution, not fixed this pass; low-value single descriptive field. (bd: unfiled)"
deferred:
  - none — full routed-op surface re-audited this pass (base + extended dispatch tables, 70 ops total)
leaks: {status: clean, note: "janitor uses pkgs/worker.Group with proper ctx.Done() teardown; no raw goroutines spawned elsewhere in the service. New capacityReservationARN-based resourceTags entries are cascade-deleted on DeleteCapacityReservation (TestInMemoryBackend_DeleteCapacityReservation_CascadesTags), matching the existing WorkGroup/DataCatalog cascade-delete behavior — no ghost tag rows after delete."}
---

## Notes

**Protocol**: awsjson1.1 (`application/x-amz-json-1.1`, single POST endpoint,
`X-Amz-Target: AmazonAthena.<Op>` dispatch). Route matcher
(`strings.HasPrefix(target, "AmazonAthena")`) is correct and was verified against
the real SDK's target prefix.

**Bugs fixed this pass** (all genuine wire-shape breaks verified against
`aws-sdk-go-v2/service/athena@v1.57.2`'s generated `serializers.go`/`deserializers.go`,
not against gopherstack's own output):

1. **`ResultConfiguration.ACLConfiguration` JSON tag was `"ACLConfiguration"`**;
   the real deserializer switches on the exact-case key `"AclConfiguration"`.
   Any GetWorkGroup/GetQueryExecution/StartQueryExecution response carrying this
   field was silently dropped by a real SDK client. `backend.go`.

2. **`BatchGetPreparedStatement` request field was `"StatementNames"`**; real
   wire key is `"PreparedStatementNames"` — a real client's request always
   parsed to an empty name list, so the op was non-functional. The response's
   `"UnprocessedStatementNames"` was likewise wrong (`"UnprocessedPreparedStatementNames"`
   is correct). `handler.go`.

3. **`GetSessionEndpoint` returned `{"SessionEndpoint": url}`**; the real
   `GetSessionEndpointOutput` has three *required* fields:
   `EndpointUrl`, `AuthToken`, `AuthTokenExpirationTime` (epoch seconds). A real
   client got a completely empty result. `handler_extra.go` + `backend_extra.go`.

4. **`CreatePresignedNotebookUrl` returned `{"NotebookSessionUrl": url}`**; the
   real `CreatePresignedNotebookUrlOutput` has three *required* fields:
   `NotebookUrl`, `AuthToken`, `AuthTokenExpirationTime`. Same bug class as #3;
   both now share `backend.newSessionAuthToken()` (a stateless synthesized
   bearer-token helper — neither op models real authentication server-side).
   `handler.go` + `backend.go`.

5. **`GetResourceDashboard` ignored its `ResourceARN` input entirely** and
   returned a fabricated `{"ResourceDashboard": {}}` envelope; the real op's
   input requires `ResourceARN` and its output is a single required `Url`
   string field. This was a disguised no-op per the no-stub rule — fixed to
   validate `ResourceARN` and return a synthesized dashboard URL.
   `handler_extra.go` + `backend_extra.go` (new `GetResourceDashboard` method,
   added to `StorageBackend`).

**Bugs fixed 2026-07-23** (this pass; verified against
`aws-sdk-go-v2/service/athena@v1.57.2`'s `types/types.go` and per-op
`api_op_*.go` files, not against gopherstack's own output). Four of these are
gopherstack-INVENTED fields with no counterpart on the real SDK type — the
no-stub/no-invented-field rule requires deleting them, not documenting them
as harmless:

6. **`WorkGroup`/`DataCatalog`/`CapacityReservation` each carried an invented
   `Tags map[string]string` field**, serialized into `GetWorkGroup`/
   `ListWorkGroups`/`GetDataCatalog`/`ListDataCatalogs`/`GetCapacityReservation`/
   `ListCapacityReservations` responses. AWS's real `types.WorkGroup`,
   `types.DataCatalog`, and `types.CapacityReservation` carry no `Tags` field
   at all — real Athena manages tags exclusively through `TagResource`/
   `UntagResource`/`ListTagsForResource`, never echoing them back on the
   resource itself. Worse than a harmless extra field: gopherstack's copy went
   stale the instant `TagResource`/`UntagResource` were called against the
   same resource, since those ops only ever touched the separate
   `resourceTags` map, never the resource's own `.Tags` field — so
   `GetWorkGroup` and `ListTagsForResource` could disagree about a
   workgroup's tags. All three `Tags` fields removed; `Create*`'s `Tags`
   input now flows only into `resourceTags`. `models.go`, `work_groups.go`,
   `data_catalogs.go`, `capacity_reservations.go`.

7. **`CreateNotebookInput` carried an invented `Tags []Tag` field.** The real
   `CreateNotebookInput` has only `Name`/`WorkGroup`/`ClientRequestToken` — no
   real AWS SDK client can populate `Tags` on notebook creation (unlike
   `CreateWorkGroup`/`CreateDataCatalog`/`CreateCapacityReservation`, which do
   accept `Tags` in the real API and were left as-is). Removed the field and
   the `tags` parameter from `InMemoryBackend.CreateNotebook`; a notebook
   remains taggable after creation via `TagResource` against its ARN.
   `handler_notebooks.go`, `notebooks.go`, `interfaces.go`.

8. **`EngineVersionDescriptor` carried an invented `AuthEngineVersion`
   field.** The real `types.EngineVersion` has exactly two fields
   (`EffectiveEngineVersion`, `SelectedEngineVersion`); `AuthEngineVersion`
   does not exist on it. Removed. `models.go`, `sessions.go`.

9. **`CreateDataCatalogOutput`/`DeleteDataCatalogOutput` never populated their
   optional `DataCatalog` field.** The real SDK v1.57.2 output types for both
   ops carry `DataCatalog *types.DataCatalog` (the created record for Create,
   the just-deleted record for Delete); gopherstack returned an empty
   `struct{}{}` body, so a real client's `output.DataCatalog` was always
   `nil`. `CreateDataCatalog`/`DeleteDataCatalog` on `InMemoryBackend` now
   return `(*DataCatalog, error)` and the handler wires the result into the
   response. `data_catalogs.go`, `handler_data_catalogs.go`, `interfaces.go`.

10. **`TagResource`/`UntagResource`/`ListTagsForResource` never validated that
    `ResourceARN` resolved to an existing resource**, and `ListTagsForResource`
    ignored its `MaxResults`/`NextToken` inputs entirely. Real AWS returns
    `InvalidRequestException` for an unknown/malformed `ResourceARN` and
    paginates `ListTagsForResource` like every other List op. Added
    `resourceExistsForARN`, which parses the ARN's `kind/id` resource segment
    and checks the corresponding table (`workgroup`, `datacatalog`,
    `capacity-reservation`, or `notebook`); wired into all three ops. This
    also surfaced (and fixed) that capacity reservations had never been wired
    into `resourceTags` via an ARN at all — `CreateCapacityReservation` built
    no ARN and wrote nothing to `resourceTags`, so a capacity reservation's
    tags were previously unreachable through `TagResource`/
    `ListTagsForResource` regardless of validation. `tags.go`,
    `handler_tags.go`, `capacity_reservations.go`, `store.go`.

11. **`ListWorkGroups`/`ListNamedQueries`/`ListDataCatalogs`/
    `ListPreparedStatements` (and the new `ListTagsForResource`) silently
    restarted pagination from offset 0 whenever `NextToken` did not exactly
    match an existing item's key** (e.g. the boundary item was deleted
    between calls) — re-emitting already-consumed results to the caller
    instead of erroring or resuming correctly. Added
    `pagination.paginationStart` (a `sort.Search`-based mutation-stable
    resume, mirroring `pageTokenCodec.paginateQueryExecutionIDs`'s existing
    approach for `ListQueryExecutions`) and switched all five call sites to
    it. `pagination.go`, `work_groups.go`, `named_queries.go`,
    `data_catalogs.go`, `prepared_statements.go`, `tags.go`.

**Looks-wrong-but-correct traps** (do not re-flag):

- `StartQueryExecution` runs the statement *synchronously* and stores a
  terminal `SUCCEEDED`/`FAILED` state before the op returns — there is no
  `QUEUED`/`RUNNING` window. This is intentional: it guarantees an SDK poll
  loop against `GetQueryExecution` never hangs, and matches how a fast local
  emulator should behave.
- `handleGetQueryResults`'s `UpdateCount` is always `0` at the top level of the
  response (not nested under `ResultSet`) — this matches
  `GetQueryResultsOutput`'s real shape (`UpdateCount` is a sibling of
  `ResultSet`, not a child).
- `TagResource`/`UntagResource`-style ops returning `struct{}{}` (empty JSON
  object) is correct for AWS Athena's void outputs — confirmed against each
  op's deserializer (no case statements at all = an empty response body is
  fully valid). `CreateDataCatalog`/`DeleteDataCatalog` are NOT in this
  category as of 2026-07-23 — see bug #9 above; they carry an optional
  `DataCatalog` field gopherstack now populates.
- Prepared-statement and calculation/session lookups use
  `ResourceNotFoundException` (`ErrResourceNotFound`); workgroup/named-query/
  data-catalog/query-execution/capacity-reservation/notebook lookups use
  `InvalidRequestException` (`ErrNotFound`). This split is intentional and
  matches AWS's documented per-resource exception types, not an inconsistency.

**Persistence**: `persistence.go` (added recently, verified intact) round-trips
all "clean" `store.Table`-backed resources through `b.registry.SnapshotAll`/
`RestoreAll`, and the two "dirty" tables (`databases`, `tables`, which need a
`Catalog`/`Database` identity their value types deliberately exclude from JSON)
through an ephemeral DTO registry. `queryResults`, `tableData`, and
`resourceTags` are persisted explicitly as documented on `backendSnapshot`.
The three ops fixed in the previous pass (`GetSessionEndpoint`,
`CreatePresignedNotebookUrl`, `GetResourceDashboard`) are all pure/derived (no
new backend fields), so nothing new needed wiring into `Snapshot`/`Restore`.
This pass (2026-07-23) removed the `Tags` field from `WorkGroup`/
`DataCatalog`/`CapacityReservation` (round-trip-safe: `encoding/json` silently
drops a removed struct field on both marshal and unmarshal, so an
old-shaped snapshot restores cleanly) and added `capacityReservationARN`-keyed
entries to the already-persisted `resourceTags` map (no new top-level
`backendSnapshot` field required) — round trip verified by
`TestInMemoryBackend_TagResource_CapacityReservation` and
`TestInMemoryBackend_DeleteCapacityReservation_CascadesTags`.

## 2026-08-21 (gopherstack-hjdd): snapshot-version guard, unbumped retype

`athenaSnapshotVersion` bumped 1 -> 2. `d83f4b5d3` retyped
`CalculationExecution.Statistics.Progress` (nested inside the registered `calculations`
table's value type) from `int64` to `string`, matching the real deserializer's type
switch, without bumping the snapshot version. A pre-fix (v1) snapshot's numeric
`"Progress"` no longer unmarshals into the new string field at all -- `RestoreAll` now
errors outright rather than silently losing data, but the whole backend then fails to
restore, which the version guard exists to convert into a clean, recoverable "discard and
start empty" instead.

Found via `pkgs/persistence`'s snapshot-version guard, extended this session
(gopherstack-hjdd) to recursively expand fields of every type reached through a
`store.Register`/`store.New` table registration.

**Proof:** `Test_InMemoryBackend_Restore_V1CalculationProgressDiscarded` (persistence_test.go)
builds a v1-shaped `calculations` snapshot with a numeric `Statistics.Progress` and asserts
`Restore` succeeds (discarding cleanly) rather than erroring. Hand-reverted to version 1:
the same test then fails with `Restore` returning `json: cannot unmarshal number into Go
struct field CalculationStatistics.Statistics.Progress of type string`, confirming the
symptom; restored and `md5sum`-verified byte-identical.

**Gates:** `go build`, `go vet` (default/e2e/integration), `gofmt -l` (clean), `go test -race`
(pass), `golangci-lint run` (0 issues).

## 2026-08-28 pass (gopherstack-6flj): write-only-state sweep

An existing `wire_field_fixes_test.go` (1 test, `ResultReuseInformation`
nesting) marked this service PARTIAL, not finished, per this campaign's own
established rule; `wire_calculation_progress_test.go` (a second, separate
dedicated test) was also present and likewise not treated as proof of
completeness. Ran the write-only-state method first: for every write op this
task named (`UpdateWorkGroup`, `UpdateDataCatalog`, `UpdateNamedQuery`,
`UpdatePreparedStatement`, `UpdateNotebookMetadata`, the capacity-reservation
ops), field-diffed the real `aws-sdk-go-v2/service/athena@v1.60.4`
request/response types directly against gopherstack's models (never against
gopherstack's own prior output).

`UpdateDataCatalog`/`UpdateNamedQuery`/`UpdatePreparedStatement`/
`UpdateNotebookMetadata`/`CreateCapacityReservation`/
`UpdateCapacityReservation`/`PutCapacityAssignmentConfiguration` all
field-diffed clean: every accepted field is genuinely stored and every
stored field has a real read path (`GetDataCatalog`/`GetNamedQuery`/
`GetPreparedStatement`/`GetNotebookMetadata`/`GetCapacityReservation`).
`NamedQuery`/`PreparedStatement`/`DataCatalog`/`TableMetadata`/`Database`
model shapes all match `types.go` exactly, field for field.

**One genuine bug found and fixed**, in `WorkGroup`/`Session` — the two
resources that share Athena's `EngineConfiguration` type:

1. **`WorkGroupConfiguration` had no `EngineConfiguration` or
   `MonitoringConfiguration` field at all.** Both are real members of
   `types.WorkGroupConfiguration` (`serializers.go`'s
   `awsAwsjson11_serializeDocumentWorkGroupConfiguration` "EngineConfiguration"/
   "MonitoringConfiguration" cases) and of the partial-update
   `types.WorkGroupConfigurationUpdates` shape. A real client configuring a
   Spark-notebook workgroup's default engine sizing
   (`CoordinatorDpuSize`/`DefaultExecutorDpuSize`/`MaxConcurrentDpus`) or
   log delivery (`CloudWatchLoggingConfiguration`/etc.) on `CreateWorkGroup`
   had it silently dropped before ever reaching the backend — accepted,
   never stored, an accept-then-drop bug on the primary method's own list.
   Fixed: added both fields to `WorkGroupConfiguration` and
   `WorkGroupConfigurationUpdates` (`models.go`), reusing the identical
   `EngineConfiguration`/`MonitoringConfiguration` types this service
   already defines for `Session` (confirmed the real SDK genuinely shares
   one generated type for both uses, not two separately-named ones), and
   extended `MergeInto` so `UpdateWorkGroup`'s partial-update semantics
   (established by the 2026-08-21 `gopherstack-1vv2` pass) cover the two
   new fields the same way as every other member.
2. **`EngineConfiguration.Classifications` was missing from the model
   entirely.** `types.EngineConfiguration` has a `Classifications
   []types.Classification{Name, Properties}` member (a real, commonly-used
   Spark/EMR-style named-configuration-block list) with no counterpart in
   gopherstack's `EngineConfiguration` model — silently dropped on both
   `CreateWorkGroup` and the pre-existing `StartSession`, since real AWS
   reuses this one type for both. Fixed: added `Classification` (new type)
   and `EngineConfiguration.Classifications` (`models.go`); flows through
   automatically on both `WorkGroup` and `Session` since both already wire
   `EngineConfiguration` straight through with no per-field handler code.

Swept one hop further into `types.QueryExecution` and found
`SubstatementType` (a real `*string`, further classifying a DDL
`StatementType`, e.g. `CTAS`) also unmodeled — a genuinely lower-value
single descriptive field, documented as a new `gaps:` entry rather than
fixed this pass given the time this sweep already spent on the two real
accept-then-drop bugs above.

`WorkGroupConfiguration.IdentityCenterConfiguration`/
`ManagedQueryResultsConfiguration`/`QueryResultsS3AccessGrantsConfiguration`
(and `WorkGroup.IdentityCenterApplicationArn`) were also confirmed present
on the real type this pass but are NOT fixed — each represents a
substantial real feature (IAM Identity Center-gated workgroup access,
Athena-managed query-result-object lifecycle, S3 Access Grants) that would
need real design work to simulate, not a wire-shape passthrough; documented
as `gaps:` entries. `IdentityCenterConfiguration`/
`ManagedQueryResultsConfiguration` specifically were already flagged
unmodeled by the 2026-08-21 pass; this pass additionally confirmed
`QueryResultsS3AccessGrantsConfiguration` belongs in the same bucket.

Round-trip test: `TestCreateWorkGroup_EngineAndMonitoringConfiguration_RealClient`
(`wire_field_fixes_test.go`), driving the real `aws-sdk-go-v2/service/athena`
client through `CreateWorkGroup` → `GetWorkGroup` → `UpdateWorkGroup` →
`GetWorkGroup`, asserting `EngineConfiguration`/`MonitoringConfiguration`/
`Classifications` all round-trip and that `UpdateWorkGroup`'s
`ConfigurationUpdates` still merges rather than wholesale-replaces (guarding
against a regression of the 2026-08-21 `gopherstack-1vv2` fix). Hand-verified
to fail against the pre-fix `models.go` (`git stash` of only that file) and
pass after.

`enumcheck` (`go run ./cmd/enumcheck`) reports 0 findings for athena.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/athena/...`).

## 2026-08-28 — wrapper-key-sweep: request-side member-name bugs (acceptguard)

`cmd/acceptguard` flagged two request-side bugs in `services/athena/`:

1. `CreateDataCatalog`/`UpdateDataCatalog` read a top-level `ConnectionType`
   request field. Neither `CreateDataCatalogInput` nor `UpdateDataCatalogInput`
   has such a member (`athena@v1.60.4` `api_op_{Create,Update}DataCatalog.go`)
   -- `ConnectionType` exists only on the response types (`DataCatalog`,
   `DataCatalogSummary`). Real AWS derives it from the `"connection-type"`
   key inside the `Parameters` map for a `FEDERATED` catalog (documented on
   `CreateDataCatalogInput.Parameters`). A real client can only ever set
   `Parameters["connection-type"]`, so the response field was always empty.
   Fixed by removing `ConnectionType` from both wire input structs and both
   backend signatures (`CreateDataCatalog`/`UpdateDataCatalog` dropped the
   `connectionType string` parameter), deriving it instead from
   `params[dataCatalogConnectionTypeParam]` (`"connection-type"`,
   `data_catalogs.go`).
2. `StartSession` read a top-level `SessionConfiguration` object.
   `StartSessionInput` has no such member (`api_op_StartSession.go`) --
   `SessionConfiguration` exists only on `GetSessionOutput`, and real AWS
   derives it server-side from the real top-level `ExecutionRole` and
   `SessionIdleTimeoutInMinutes` request fields, which gopherstack didn't
   read at all. Fixed by replacing the `SessionConfiguration` wire field
   with `ExecutionRole`/`SessionIdleTimeoutInMinutes` (matching the real
   wire keys) and building the internal `SessionConfiguration` from them in
   the handler (`IdleTimeoutSeconds = minutes * 60`, since gopherstack's
   internal model tracks seconds).

Both proven via a real `aws-sdk-go-v2/service/athena` client round trip in
`wire_field_fixes_test.go`: `TestCreateDataCatalog_ConnectionTypeRealClient`
(Create/Update with `Parameters: {"connection-type": ...}` → `GetDataCatalog`
echoes it) and `TestStartSession_ExecutionRoleRealClient` (`StartSession`
with `ExecutionRole`/`SessionIdleTimeoutInMinutes` → `GetSession` echoes
both, converted to seconds). Hand-reverted `data_catalogs.go`,
`handler_data_catalogs.go`, `handler_sessions.go`, `interfaces.go` (plus
`export_test.go`'s now-mismatched call site) together, confirmed both tests
fail pre-fix (`ConnectionType`/`ExecutionRole` empty on the real client's
response), restored the fix.

`handler_data_catalogs_test.go`'s `TestHandler_DataCatalog_FederatedStatus`
and `TestHandler_DataCatalog_ListIncludesStatus` sent the wrong top-level
`ConnectionType` request key directly as raw JSON -- updated both to send
`Parameters: {"connection-type": ...}`, the real derivation path.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/athena/...`).

## 2026-08-29 (pagination-arithmetic sweep, wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)

Census: `paginationStart` (`pagination.go`) is a shared threshold-search cursor
(`sort.Search` for the first key `>= boundary`, defaulting a full miss to `n` — the
end of the collection, never `0`) used by `ListNamedQueries`, `ListDataCatalogs`,
`ListPreparedStatements`, `ListWorkGroups`, `ListTagsForResource`. It is already the
appmesh-style safe-by-construction pattern this campaign is looking for — the bug class
(Class B/C: a scan-miss defaulting to offset 0) cannot be expressed here. Separately,
`pageTokenCodec.paginateQueryExecutionIDs` (`ListQueryExecutions`) uses
`sort.SearchStrings`, the same threshold-search shape, over an opaque HMAC-signed token.
`GetQueryResults` (`sql.go`) uses a plain numeric row-offset token, clamped
(`offset >= len(res.rows)` returns an empty page) before every slice — safe against
Class A. No hand-rolled equality-scan cursor exists anywhere in this service; this
service does not import `pkgs/page` (its own threshold-search codec/helper predates
and supersedes it). Verdict: correct, no bug found.

Added `pagination_arithmetic_test.go`: a real `aws-sdk-go-v2` typed-client boundary
walk over `ListWorkGroups` (N=7 workgroups + the default "primary", page size 3,
concatenation checked for completeness/no-dupes). The pre-existing
`TestListWorkGroups_Pagination_StaleTokenResumesStably`
(`handler_work_groups_test.go`) already covers the stale-cursor case end-to-end
(delete the boundary workgroup a token names, resume, assert it lands on the next
surviving item rather than restarting at offset 0) — a genuinely strong existing test,
not a gap.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` — all clean
(`./services/athena/...`).

## 2026-08-31: PARITY-gap targeting (gopherstack-6flj/21my)

Queue derivation: real `List*` ops in athena@v1.60.4 (16 total, athena has zero `Describe*`
ops) whose full name never appears verbatim anywhere in this file. Mechanical grep gave 5:
`ListCalculationExecutions`, `ListDatabases`, `ListNotebookMetadata`, `ListSessions`,
`ListTableMetadata`. All 5 turned out to be false positives from this file's grouped-row
notation (e.g. `Session (Start/Get/GetStatus/Terminate/List/ListNotebookSessions)` names
`ListSessions` only as the bare abbreviation `List`) -- the rows themselves already carry
`wire: ok`. Per this session's explicit "do not trust existing notes" mandate, verified
each independently against athena@v1.60.4's own deserializers/types rather than accepting
the `wire: ok` claim at face value.

`ListCalculationExecutions`, `ListDatabases`, `ListTableMetadata`, `ListNotebookMetadata`
came back genuinely clean: wrapper keys (`Calculations`/`DatabaseList`/`TableMetadataList`/
`NotebookMetadataList`) and every per-item field name verified byte-exact against
`awsAwsjson11_deserializeDocument*` (JSON protocol, case-sensitive, confirmed from
deserializers.go's function prefix -- no case-folding risk here unlike cloudfront's XML).

**`ListSessions` was not clean.** One real bug spanning both `GetSession` and
`ListSessions`, found by diffing the List item type against its singular Get sibling per
this issue's core heuristic:

1. **`GetSession`'s response (`handler_sessions.go`) emitted `s.NotebookVersion` under the
   `"EngineVersion"` key.** Real `GetSessionOutput.EngineVersion` (api_op_GetSession.go) is
   a distinct required-shape `*string` member (e.g. `"PySpark engine version 3"`), unrelated
   to `NotebookVersion` -- a wrong VALUE under a correct key and correct Go type (both
   strings), so no wrapper-key or type-mismatch sweep would have caught it; only a real
   client asserting the decoded value does. Fixed to emit the `pysparkEngineV3` constant
   (this backend's Sessions API is Spark-only, matching real Athena-for-Spark semantics).

2. **`SessionSummary` (`models.go`) had no `EngineVersion` field at all.** Real
   `types.SessionSummary.EngineVersion` (deserializers.go
   `awsAwsjson11_deserializeDocumentSessionSummary`, case `"EngineVersion"`) is a NESTED
   `*types.EngineVersion` OBJECT (`SelectedEngineVersion`/`EffectiveEngineVersion`) --  the
   same shape `ListEngineVersions` already returns -- not the flat string `GetSession` uses
   for the same English name. A genuine Get/List type divergence in the real API itself,
   not a gopherstack bug in isolation, but gopherstack modeled neither side's version of the
   field for `SessionSummary`, so `ListSessions`/`ListNotebookSessions` always decoded
   `EngineVersion` nil. Fixed: added the field, factored a shared `sessionSummaryOf` builder
   (`sessions.go`) used by both list ops, populated with
   `{SelectedEngineVersion: pysparkEngineV3, EffectiveEngineVersion: pysparkEngineV3}`.

Test: `TestSession_EngineVersion_RealClient` (`wire_field_fixes_test.go`), drives the real
aws-sdk-go-v2 client through StartSession -> GetSession -> ListSessions and asserts both the
flat `GetSession.EngineVersion` string and the nested `ListSessions[].EngineVersion` object.
Verified failing pre-fix by hand-revert (`GetSession.EngineVersion` decoded empty string;
`ListSessions[0].EngineVersion` decoded nil).

**Recorded, not fixed** (different axis -- state never tracked at all, not a naming
mismatch, so out of this issue's wire-shape scope): `TableMetadata.CreateTime`/
`.LastAccessTime` (real, optional `*time.Time` members, confirmed present in gopherstack's
own model with correct JSON tags) are never populated by any of the 3 call sites that
construct a `TableMetadata` (`ddl.go` x2, `store.go` x1) -- always the float64 zero value,
which `omitempty` then drops from the wire entirely. Right key, right type, just never
computed; a real client's `GetTableMetadata`/`ListTableMetadata.CreateTime` is always absent
regardless of when the table was actually created. Also recorded: `ListDatabases`/
`ListTableMetadata` (`handler_databases.go`) read no `NextToken`/`MaxResults` from their
request at all and the backend methods take no pagination parameters -- a real client's
`MaxResults` is silently ignored and every result page is unbounded. This is the same
never-honoured-pagination class already tracked in the `families: pagination` section above
for other ops, not a wrapper-key/per-item-name bug; not fixed this pass (bd: unfiled).

No hard-decode-error or panic findings this batch. No case-only mismatches (JSON protocol
here, not XML -- a case mismatch would be a hard failure, not silently tolerated, and none
was found). Pages fetched this batch: 0 (module cache used throughout).

Gates (`services/athena/` only, plus repo-wide `go vet`): `go build ./...` clean;
`go vet ./...` clean; `go test -race -count=1 ./services/athena/...` clean;
`golangci-lint run ./services/athena/...` 0 issues (one `golines -m 120` reformat applied
to `sessions.go` after the fix, scoped to the touched lines only). No `nolint` directives in
any file touched this batch (`models.go`, `sessions.go`, `handler_sessions.go`,
`wire_field_fixes_test.go`). `models.go` was touched (new `SessionSummary.EngineVersion`
field) -- `SessionSummary` is a derived list-view type, not part of `backendSnapshot`
(confirmed against `persistence.go`), so no snapshot version bump was needed;
`TestSnapshotVersionGuard` run anyway per this session's mandate and passed.
