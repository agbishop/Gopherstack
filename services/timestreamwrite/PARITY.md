---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: timestreamwrite
sdk_module: aws-sdk-go-v2/service/timestreamwrite@v1.38.4
last_audit_commit: 4ad94a2e4
last_audit_date: 2026-08-29
overall: A            # wrapper-key/nested-shape sweep found and fixed one real gap (DataModelConfiguration/RecordVersion never modelled on CreateBatchLoadTask); rest of the surface re-verified clean
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateDatabase: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed — KmsKeyId now applied atomically at creation (was a separate UpdateDatabase call after CreateDatabase, causing a race window and LastUpdatedTime != CreationTime)"}
  DescribeDatabase: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDatabase: {wire: ok, errors: ok, state: ok, persist: ok, note: "KmsKeyId is a required field on the real UpdateDatabaseRequest; handler does not enforce non-empty. NOT changed — TestAccuracy2_UpdateDatabaseClearKmsKeyId deliberately locks in clearing the key via empty string, a considered emulator convenience. See gaps."}
  ListDatabases: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDatabase: {wire: ok, errors: ok, state: ok, persist: ok, note: "correctly rejects deletion while tables remain (ValidationException), matching AWS doc note"}
  CreateTable: {wire: ok, errors: ok, state: ok, persist: ok, note: "retention default 6h/73d and min/max bounds (1-8766h, 1-73000d) verified against botocore model"}
  DescribeTable: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateTable: {wire: ok, errors: ok, state: ok, persist: ok, note: "whole-struct replace semantics for RetentionProperties/MagneticStoreWriteProperties/Schema matches AWS (no deep merge)"}
  ListTables: {wire: ok, errors: fixed, state: ok, persist: ok, note: "gopherstack-4ly2 (2026-08-21): handler unconditionally required DatabaseName, but ListTablesInput marks no member required (api_op_ListTables.go, timestreamwrite@v1.38.4). Omitting DatabaseName now lists every table across every database (backend iterates b.tables directly instead of the per-database index); a prior test (TestHandler_ListTables_MissingDBName) asserted the wrong 400 and was corrected."}
  DeleteTable: {wire: ok, errors: ok, state: ok, persist: ok}
  WriteRecords: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (2026-09-04, gopherstack-8g5) — records with timestamps outside the memory-store retention window are now rejected via RejectedRecordsException when the table has no magnetic store write path (types/errors.go RejectedRecordsException doc: 'Records with timestamps that lie outside the retention duration of the memory store'); previously always accepted into the memory store regardless of age when EnableMagneticStoreWrites was unset/false. Also: CommonAttributes dimensions overlapping a record's own dimension names are now rejected with ValidationException instead of silently overridden (api_op_WriteRecords.go WriteRecordsInput.CommonAttributes doc: 'Dimensions may not overlap, or a ValidationException will be thrown'). Prior notes retained: negative Version rejected; RejectedRecords shape matches deserializers.go."}
  DescribeEndpoints: {wire: partial, errors: ok, state: ok, persist: n/a, note: "returns a non-empty Endpoints list (satisfies SDK's hard requirement), but Address is hardcoded \"localhost\" instead of echoing the request Host like the sibling timestreamquery service does. Verified this is inert in practice: aws-sdk-go-v2's DiscoverEndpoint middleware skips the call entirely whenever EndpointSourceCustom is set (i.e. whenever a BaseEndpoint/AWS_ENDPOINT_URL is configured, which all gopherstack/LocalStack clients do), and even when not skipped it only overrides req.URL.Host if the returned host matches the partition's DNS suffix (*.amazonaws.com) — \"localhost\" never qualifies either way. Left unchanged; see gaps."}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: partial, errors: partial, state: ok, persist: ok, note: "real API can return ResourceNotFoundException for an unknown ARN; backend silently no-ops. NOT changed — ListTagsForResource/UntagResource have no-error signatures and existing tests (TestInMemoryBackend_DeleteDatabase_CleansUpTags etc.) deliberately assert empty-not-error after a resource is deleted. Fixing would need a signature change (add error return) rippling through the interface and ~10 call sites for an ambiguous case (AWS's own DeleteDatabase doc says distributed retries may already return either ResourceNotFoundException or success — clients must treat them as equivalent). See gaps."}
  ListTagsForResource: {wire: ok, errors: partial, state: ok, persist: ok, note: "same ResourceNotFoundException gap as UntagResource, same rationale for not changing"}
  CreateBatchLoadTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed — DataModelConfiguration and RecordVersion, both real optional members of CreateBatchLoadTaskInput (api_op_CreateBatchLoadTask.go), were entirely unmodelled: the wire struct had no field for either, so a compliant client sending DataModelConfiguration (the normal way to specify the CSV-to-table column mapping) had it silently dropped, never stored, never echoed back by DescribeBatchLoadTask. Now modelled full-depth (DataModel/DataModelS3Configuration/DimensionMapping/MixedMeasureMapping/MultiMeasureMappings/MultiMeasureAttributeMapping) and threaded through Create->backend->Describe. ReportConfiguration is still a required field on the real CreateBatchLoadTaskRequest that the handler does not enforce as non-nil — NOT changed, same rationale as before (compliant clients always send it; smithy client-side validation blocks the request before it reaches the wire). ClientToken is accepted but not used for idempotent retry dedup — deferred, batch-load is not a highest-traffic family."}
  DescribeBatchLoadTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed — DescribeBatchLoadTaskOutput.BatchLoadTaskDescription now includes DataModelConfiguration (deserializers.go:2981's case \"DataModelConfiguration\"), previously missing from batchLoadTaskDescriptionView entirely"}
  ListBatchLoadTasks: {wire: ok, errors: ok, state: ok, persist: ok}
  ResumeBatchLoadTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (2026-09-06, gopherstack-3geb) — batch_load_tasks.go's ResumeBatchLoadTask doc comment claimed resumable from PENDING_RESUME or FAILED, contradicting its own guard (PROGRESS_STOPPED or FAILED). Both names are real types/enums.go BatchLoadStatus values (neither fabricated), and the pinned SDK's api_op_ResumeBatchLoadTask.go has no operation doc comment, so no AWS precondition could be verified either way. Resolved by aligning the comment to the code: TestInMemoryBackend_ResumeBatchLoadTask_ResumableStates already locked in PROGRESS_STOPPED/FAILED as resumable and PENDING_RESUME as not, so the code (and its test) reflect the considered behavior, not the stale comment. Comment-only change; behavior unchanged."}
families:
  errors: {status: ok, note: "fixed — ConflictException now returns HTTP 400 (was 409). awsJson1.0/1.1 has no per-exception HTTP status: every client-fault error is reported over 400 (server faults over 500), and the SDK resolves the concrete exception type from the body's __type field / X-Amzn-ErrorType header, never from the HTTP status (confirmed in aws-sdk-go-v2 deserializers.go's generic error dispatch, and matches the sibling verifiedpermissions/codestarconnections services and DynamoDB's own handler comment 'Most DynamoDB errors return 400')."
  wire-protocol: {status: ok, note: "fixed — response Content-Type was \"application/x-amz-json-1.1\"; real Timestream Write is protocol=json jsonVersion=1.0 (confirmed via botocore service-2.json metadata and aws-sdk-go-v2 serializers.go's httpBindingEncoder.SetHeader(\"Content-Type\").String(\"application/x-amz-json-1.0\")). Sibling timestreamquery already used 1.0 correctly. Target prefix \"Timestream_20181101.\" was already correct."}
  arn-building: {status: ok, note: "fixed — tableARN hand-formatted \"arn:aws:...\" (hardcoded \"aws\" partition) instead of using pkgs/arn.Build like databaseARN does in the same file. Now uses arn.Build for partition-correctness consistency (GovCloud/China regions would previously get a wrong partition on table ARNs but not database ARNs)."}
leaks: {status: clean, note: "closeAllTableMutexesLocked is correctly called on Reset, DeleteDatabase, DeleteTable, and before Restore discards the records map — no lockmetrics.RWMutex leak found. Persistence Snapshot/Restore round-trips databases, tables, batchLoadTasks (via store.Registry), plus the hand-rolled records/tags maps, nextTaskID, and rebuilds the per-table dedup index and mutex on Restore."}
gaps:
  - "UpdateDatabase does not enforce KmsKeyId as required (real UpdateDatabaseRequest marks it required) — not fixed, conflicts with an existing intentional test that uses empty string to clear the key (bd: file if desired)"
  - "UntagResource/ListTagsForResource never return ResourceNotFoundException for an unknown ARN (real API can) — not fixed, would require an interface signature change and conflicts with existing post-delete cleanup test assertions; AWS's own docs note the two outcomes are meant to be treated as equivalent for DeleteDatabase's ARN-cleanup race anyway (bd: file if desired)"
  - "CreateBatchLoadTask does not validate ReportConfiguration as required, and ClientToken is accepted but not used for idempotent dedup (bd: file if desired)"
  - "DescribeEndpoints Address is hardcoded \"localhost\" instead of echoing the request Host (sibling timestreamquery does echo it); verified inert for normal custom-endpoint usage, but would matter for tooling that inspects the raw response instead of relying on SDK routing (bd: file if desired, low priority)"
  - "Table.Schema.CompositePartitionKey[].EnforcementInRecord=REQUIRED (2026-08-29 pass, write-only-state FORWARD direction): confirmed real and stored -- validated at CreateTable/UpdateTable time (validateSchemaPartitionKeys) and correctly echoed on Describe -- but never read back by WriteRecords, so a record missing a dimension a table's schema marks REQUIRED is silently accepted instead of rejected. types/types.go's PartitionKey.EnforcementInRecord doc comment ('REQUIRED (dimension key must be specified)') confirms this is meant to gate writes, matching this campaign's 'a dropped request field is a disabled validation until proven otherwise' rule (same class as emr's SessionEnabled/fsx's SourceSnapshotARN). NOT fixed this pass: RejectedRecord.Reason is undocumented free text for this specific cause (the pinned SDK's RejectedRecord doc comment lists duplicate-version, retention-window, and size-limit causes, but not a missing-partition-key case; unlike an error CODE, which must byte-match for a typed client to classify it, Reason's exact wording isn't independently verifiable against the pinned SDK source or docs from this environment) and it's unclear whether the real failure mode is a per-record RejectedRecord vs. a whole-request ValidationException -- implementing enforcement risks fabricating the wire shape rather than confirming it, which this campaign explicitly treats as worse than an honest gap. Flagged for a follow-up pass with live-AWS access to confirm the exact failure shape (bd: file if desired)."
deferred: []
reaudit_2026-08-20: >
  Wrapper-key/nested-shape wire-parity sweep against the pinned
  aws-sdk-go-v2/service/timestreamwrite@v1.38.4 module (types/types.go, types/enums.go,
  serializers.go, deserializers.go, api_op_*.go). Confirmed the service is JSON-RPC 1.0
  (awsAwsjson10_* prefix, X-Amz-Target: Timestream_20181101.<Op>), so the restjson
  flat-body false-positive trap (gopherstack-cnhp) does not apply here: verified
  awsAwsjson10_deserializeOpDocument<Op>Output is both defined AND called (grep -c == 2)
  for WriteRecords/DescribeBatchLoadTask/DescribeTable/CreateBatchLoadTask/DescribeDatabase.
  Ran the full "This member is required" grep across types/types.go and every api_op_*.go
  and checked each hit against the handler; found one real bug class-(a) member-missing
  gap: CreateBatchLoadTaskInput.DataModelConfiguration and .RecordVersion (both real,
  optional root-level members serialized by serializers.go:1806
  awsAwsjson10_serializeOpDocumentCreateBatchLoadTaskInput) had no field at all in
  gopherstack's createBatchLoadTaskInput wire struct, so a compliant client sending
  DataModelConfiguration -- the normal way to specify a batch load task's CSV-to-table
  column mapping, not an edge case -- had it silently dropped on Create and never echoed
  back by Describe (deserializers.go:2981's case "DataModelConfiguration" on
  BatchLoadTaskDescription). Fixed: modelled the full DataModelConfiguration nest
  (DataModel/DataModelS3Configuration/DimensionMapping/MixedMeasureMapping/
  MultiMeasureMappings/MultiMeasureAttributeMapping, field names verified against
  serializers.go:1209-1566) in both models.go (backend) and handler_batch_load_tasks.go
  (wire), threaded through InMemoryBackend.CreateBatchLoadTask's new
  dataModelCfg/recordVersion parameters, and added
  TestCreateBatchLoadTask_DataModelConfigurationSDKRoundTrip
  (wire_sdk_roundtrip_test.go) driving the real aws-sdk-go-v2 client through
  CreateBatchLoadTask/DescribeBatchLoadTask over the real pkgs/service router. Proved by
  hand-revert: removing the one `v.DataModelConfiguration = toDataModelConfigView(...)`
  assignment reproduces exactly the predicted symptom (DescribeBatchLoadTask response has
  a nil DataModelConfiguration), confirmed, then restored byte-identical. Verified
  MeasureValue (string, Record.MeasureValue) vs MeasureValues (list of
  Name/Value/Type structs, Record.MeasureValues) are correctly split in
  handler_records.go/models.go, matching types.go:446-487 exactly -- no collision bug.
  Verified RejectedRecordsException's body shape (flat "RejectedRecords" key at the error
  body root, not nested; RejectedRecord{ExistingVersion,Reason,RecordIndex}) against
  deserializers.go:4485-4525 and :4385-4429 -- exact match, confirmed by
  TestWriteRecords_RecordsIngestedSDKRoundTrip and the pre-existing RejectedRecords unit
  tests. Layer 1 (wrapper key + nesting level) re-checked op by op against each op's own
  live deserializer for CreateDatabase/DescribeDatabase/ListDatabases/UpdateDatabase/
  CreateTable/DescribeTable/ListTables/UpdateTable/WriteRecords/DescribeEndpoints/
  TagResource/UntagResource/ListTagsForResource/CreateBatchLoadTask/DescribeBatchLoadTask/
  ListBatchLoadTasks/ResumeBatchLoadTask -- all flat (no extra wrapper nesting), all
  correct. Provenance check on the prior stamp: `git show -s --format=%ad ca3b796e` ==
  2026-07-23 == the prior last_audit_date exactly, no gap -- clean. No banned
  cyclop/gocyclo/gocognit/funlen nolints added (2 new `//nolint:dupl` on
  toDataModelView/dataModelFromInput, a genuinely mirrored bidirectional conversion pair,
  matching this repo's existing nolint:dupl convention e.g. services/swf/handler_activity_types.go).
  go build/vet/fix -diff/gofmt/test -race/golangci-lint all clean;
  fieldalignment -fix applied (also cleaned 3 pre-existing unrelated struct-literal
  field-order issues in handler_records_test.go/handler_tables_test.go/tables_test.go as
  a side effect, mechanical reordering only, no behavior change).
reaudit_2026-07-23: >
  Independent field-diff re-audit against the checked-out aws-sdk-go-v2/service/timestreamwrite@v1.35.19
  module (types/types.go, types/errors.go, deserializers.go, api_op_*.go). Confirmed still accurate:
  epoch-seconds timestamp wire fields (databaseView/tableView/batchLoadTaskDescriptionView all use
  manually-converted float64, never json.Marshal of a raw time.Time -- no epoch bug present);
  RejectedRecord{Reason,ExistingVersion,RecordIndex} matches types.RejectedRecord byte-for-byte;
  error-code/HTTP-400 mapping in handler.go's handleError matches the awsJson1.0 no-per-exception-status
  convention; UpdateDatabaseInput.KmsKeyId and CreateBatchLoadTaskInput.ReportConfiguration are indeed
  `// This member is required` in the generated SDK source (re-confirmed via grep this pass), so the two
  corresponding gaps below are genuinely unreachable via a compliant SDK client, not unverified claims.
  Cascade cleanup (DeleteDatabase/DeleteTable close per-table lockmetrics.RWMutex before dropping the
  records map, delete tags by ARN) and Snapshot/Restore round-trip re-read line by line -- no leak found,
  no ghost rows after delete. sdk_completeness_test.go's notImplemented list is empty, confirming no
  deferred resource family remains. No banned cyclop/gocyclo/gocognit/funlen nolints present. No
  gopherstack-invented ops/fields found. go build/vet/test -race/gofmt/golangci-lint all clean. No code
  changes made this pass -- see gaps below for the only known open items, each already unreachable via a
  compliant client or hedged for compatibility with an intentional existing test.
---

## Notes

Freeform: AWS-behavior specifics worth remembering, and any "looks-wrong-but-correct" traps
so the next auditor doesn't re-flag them.

- **`CreateBatchLoadTaskInput.DataModelConfiguration` is a real, commonly-used optional
  root-level member** (`api_op_CreateBatchLoadTask.go`), not the exotic edge case its
  omission from earlier sweeps might suggest — it's how a caller specifies the CSV
  source-column-to-Timestream-column mapping (`DataModel.DimensionMappings`/
  `MixedMeasureMappings`/`MultiMeasureMappings`) for a batch load. It nests
  `DataModelS3Configuration` too (an alternate way to supply the model via S3 instead of
  inline). Both directions (Create's request, Describe's
  `BatchLoadTaskDescription.DataModelConfiguration` response) are now modelled — see the
  2026-08-20 reaudit note above for field-name citations. If a future SDK bump adds new
  `DataModel` sub-fields, re-run the required-member grep against `DataModel`,
  `MixedMeasureMapping`, and `MultiMeasureAttributeMapping` specifically; those three
  shapes are the ones most likely to grow.

- **Protocol is `json` / `jsonVersion: "1.0"`** (confirmed via botocore
  `timestream-write/2018-11-01/service-2.json` metadata block), i.e. Content-Type
  `application/x-amz-json-1.0` and target prefix `Timestream_20181101.<Op>` (no version
  suffix on the prefix itself — the version lives in the Content-Type header only). Was
  previously wired to `application/x-amz-json-1.1`; fixed this sweep.

- **HTTP status codes for awsJson1.0/1.1 are NOT semantic.** None of Timestream Write's
  exception shapes (`ConflictException`, `ResourceNotFoundException`, `ValidationException`,
  `RejectedRecordsException`, etc.) declare an explicit `httpStatusCode` override in the
  service model. Per the protocol's default behavior (and confirmed by reading
  `aws-sdk-go-v2/service/timestreamwrite`'s generated `deserializers.go`), the client
  determines the concrete exception type purely from the response body's `__type` field
  (or the `X-Amzn-ErrorType` header) — **never** from the HTTP status code. The wire-correct
  convention used elsewhere in this codebase for awsJson1.0 services (verifiedpermissions,
  codestarconnections) and explicitly documented in dynamodb/handler.go's own comment
  ("Most DynamoDB errors return 400") is: 400 for every client fault, 500 for server faults.
  Do not "upgrade" any client-fault error to a REST-ish status like 404/409/422 for this
  service — it's wrong for this protocol family even though it feels natural.

- **`RejectedRecord` wire fields are `Reason`, `ExistingVersion`, `RecordIndex`** — verified
  byte-for-byte against `deserializers.go`'s
  `awsAwsjson10_deserializeDocumentRejectedRecord`. Matches exactly; no change needed.

- **Retention bounds**: `MemoryStoreRetentionPeriodInHours` is `[1, 8766]`,
  `MagneticStoreRetentionPeriodInDays` is `[1, 73000]` per the botocore model's
  `min`/`max` constraints on those shapes. Defaults when `RetentionProperties` is omitted
  at `CreateTable` time are 6 hours / 73 days. Both verified correct in this codebase;
  don't re-flag.

- **`DescribeEndpoints`'s `Address` value is inert for normal usage.** The
  aws-sdk-go-v2 `endpointdiscovery.DiscoverEndpoint` middleware (finalize step) skips the
  discovery call entirely whenever `awsmiddleware.GetEndpointSource(ctx) ==
  aws.EndpointSourceCustom` — which is the case for every client configured with a custom
  `BaseEndpoint`/`AWS_ENDPOINT_URL*`, i.e. every realistic gopherstack/LocalStack client.
  Even when the middleware does run, it only overrides `req.URL.Host` if the returned
  host has the partition's DNS suffix (`*.amazonaws.com` / dualstack equivalent) — a
  `localhost` or arbitrary hostname address is silently ignored either way. So while the
  Address value looks unfinished, it does not currently break SDK routing. Don't spend
  more effort here without a concrete failing scenario.

- **`CreateDatabase` + `KmsKeyId` is now atomic.** Previously the handler called
  `Backend.CreateDatabase` then, if `KmsKeyId` was set, a *second*
  `Backend.UpdateDatabase` call. This meant: (1) a race window where a concurrent
  `DescribeDatabase` could observe the database without its KMS key, and (2)
  `LastUpdatedTime` would always be strictly after `CreationTime` when `KmsKeyId` was
  supplied at creation (wrong — AWS's `CreateDatabaseRequest` accepts `KmsKeyId` directly,
  so a fresh database's `CreationTime` and `LastUpdatedTime` should be equal regardless).
  Fixed by threading `kmsKeyID` into `InMemoryBackend.CreateDatabase` directly.

## gopherstack-o7gx follow-up (2026-08-22): default error path emitted InternalServerError instead of the modeled fault

`handler.go`'s `handleError` default branch wrote `keyTypeField:
"InternalServerError"` for any unclassified 500. `timestreamwrite@v1.38.4`
`types/errors.go:66-89` models `InternalServerException` (`ErrorFault:
FaultServer`) as the service's dominant 5xx fault, wired into 16 of its 19
operation error switches in `deserializers.go` (84%). `"InternalServerError"`
appears nowhere in `types/errors.go`, so a real client's
`errors.As(&types.InternalServerException{})` never matched.

Fixed to `keyTypeField: "InternalServerException"`. The default branch is
reachable only when a backend error isn't NotFound/Conflict/
RejectedRecords, or classified via `ValidationException`'s `errInvalidRequest`/
`errUnknownAction`/`json.SyntaxError`/`json.UnmarshalTypeError` checks; no
currently-wired dispatch path leaves an error unclassified this way, so
there is no legitimately-constructed real SDK client request that reaches
this branch today. `TestHandleError_DefaultBranchEmitsInternalServerException`
(`handler_internal_error_test.go`, new, white-box `package timestreamwrite`)
drives `handleError` directly with a synthetic unmatched error and asserts
the JSON response's `__type` is `InternalServerException`; confirmed it
fails pre-fix with the old `"InternalServerError"` code (hand-reverted,
byte-identical restore after).

## gopherstack-wlo1 (2026-08-22): handleCBOR's own dispatch errors were never typed

`handleCBOR` (`handler.go`) is a separate hand-rolled path parallel to the
main `service.HandleTarget`-based dispatch (already typed and correct). Its
own method-not-allowed, missing/malformed-`X-Amz-Target`, ReadBody-failure,
and CBOR-response-encode-failure branches all wrote a bare
`c.String(http.Status..., "...")` -- text/plain. TimestreamWrite is
JSON-RPC 1.0 (`timestreamwrite@v1.38.4` `awsAwsjson10_` prefix; its error
decode goes through `restjson.GetErrorInfo`, `__type`/`message`), so a real
client saw `smithy.GenericAPIError{Code:"UnknownError"}` for any of these
four sites. Same class as `c6554e9f8`'s `pkgs/service.HandleTarget` finding,
missed here because `handleCBOR` never calls that shared helper.

Fixed: the method-not-allowed and missing-target branches now emit
`c.JSON(status, map[string]string{keyTypeField: "UnknownOperationException",
keyMessageField: "..."})` -- the same `__type`/`message` shape and constants
(`keyTypeField`/`keyMessageField`) `handleCBOR`'s own already-correct
"invalid CBOR body" branch uses three lines below, and the same
`"UnknownOperationException"` code `pkgs/service.HandleTarget` uses for its
analogous branches. The ReadBody-failure and CBOR-encode-failure branches
now emit `{__type: "InternalFailure", message: "internal server error"}`.

Proof: `aws-sdk-go-v2/service/timestreamwrite` never sends
`application/x-amz-cbor-1.1` itself. `TestHandleCBOR_WrongMethodSurfacesUnknownOperationException`
(`handler_cbor_dispatch_malformed_test.go`) drives a real client's
`WriteRecords` through a Finalize-stage middleware that rewrites the request
to CBOR content type and its HTTP method to GET post-signing (matched only
on the `WriteRecords` target, so the client's preliminary `DescribeEndpoints`
discovery call is left untouched). Hand-reverted `handler.go` to
`git show HEAD`, confirmed the test fails with `*json.SyntaxError: "invalid
character 'M' looking for beginning of value"`, restored the fix,
`md5sum`-confirmed byte-identical.

The ReadBody-failure and CBOR-encode-failure branches are fixed for
consistency but not independently client-proven: `readBodyBytes`
(handler.go) uses a local `io.LimitReader`/`io.ReadAll` that silently
truncates at its 10 MiB cap rather than erroring the way
`httputils.ReadBody`'s `http.MaxBytesReader` does, so an oversized body
lands in the (already-correct) CBOR-decode-failure branch instead, not this
one -- genuinely hard to trigger through a real client without a live I/O
error mid-read.

## 2026-08-29 pass: write-only-state sweep (gopherstack-6flj/21my), forward and reverse

Re-audited despite the extensive 2026-07-23/2026-08-20 history (per this
campaign's standing "a prior pass proves nothing" rule). Verified the
premise first: `git log` showed no drift since the 2026-08-20 wrapper-key
sweep; `sdk_module` (timestreamwrite@v1.38.4) matched the checked-out
module exactly (no SDK bump to re-audit); no `wire_field_fixes_test.go`
exists for this service (the equivalent file is `wire_sdk_roundtrip_test.go`
from the 2026-08-20 pass) -- confirmed with the harness owner this is the
service's established round-trip test file and appended nothing new to it
this pass, since no new fixable bug was found (see below).

Applied the write-only-state method in both directions:

- FORWARD (accepted-and-stored-but-never-read-back): re-verified
  `DataModelConfiguration`/`RecordVersion` (2026-08-20's fix) still have a
  live read path. Found one new, real, but NOT fixed instance:
  `Table.Schema.CompositePartitionKey[].EnforcementInRecord` (see gaps) --
  stored and correctly echoed on Describe, but never enforced by
  `WriteRecords`. Declined to implement enforcement because the exact wire
  failure shape (RejectedRecord free-text `Reason` vs. a whole-request
  `ValidationException`) isn't independently confirmable from the pinned
  SDK source or docs in this environment, and guessing risks fabricating
  wire behavior rather than fixing a confirmed one -- flagged as a gap for
  a follow-up pass with live-AWS access instead.
- REVERSE (response-computable-from-already-stored-state): field-diffed
  `BatchLoadTaskDescription` (types/types.go) against
  `batchLoadTaskDescriptionView` member-by-member -- exact match, no gap.
  Re-verified `WriteRecordsOutput.RecordsIngested.{Total,MemoryStore,
  MagneticStore}` is the correct nested-object shape (not flat), matching
  `types.RecordsIngested`.
- Ran a script diffing every `*Input` struct field against its usage sites
  in the same file (same method used for xray this pass). No genuine
  dropped-field hits: `CreateDatabaseInput`/`CreateTableInput`'s `Tags` are
  used (`validateTagInputs`/`tagsFromInput`); `CreateBatchLoadTaskInput`
  has no `Tags` member on the real SDK type at all, confirmed against
  `api_op_CreateBatchLoadTask.go` (nothing to drop).

No new fixable bug found and no fix applied this pass; `overall` remains A.
This is treated as a genuine clean result, not a failure to look hard
enough -- consistent with this campaign's outposts/dax precedent of a
service coming back clean after real effort, and distinct from a rubber-
stamp "looks fine" pass: three read paths were independently re-verified
end-to-end and one new real (if unconfirmable-in-shape) gap was found and
disclosed rather than silently passed over.

Ops NOT specifically re-audited this pass beyond the two directions above
(unchanged since 2026-08-20, no SDK drift): `ListDatabases`/`DeleteDatabase`,
`ListTables`/`DeleteTable`, `TagResource`/`UntagResource`/
`ListTagsForResource` (beyond re-confirming the documented
ResourceNotFoundException gap is unchanged), `ListBatchLoadTasks`/
`ResumeBatchLoadTask`, and `DescribeEndpoints` (beyond re-confirming the
documented hardcoded-Address gap is unchanged and still inert).

### 2026-08-31 (gopherstack-uox6, value-semantics-of-a-correctly-read-field pass)

`covledger -service timestreamwrite` reported no rows for every class. This
pass targets a different axis than the entries above: not "is the field
read" (already swept) but "does the code that reads it do the right thing
with it". Only two List ops carry any filter surface at all
(`aws-sdk-go-v2/service/timestreamwrite@v1.38.4`):

- `ListBatchLoadTasks.TaskStatus`: plain equality against
  `types.BatchLoadStatus`, no wildcard/negation/case language documented.
  `batch_load_tasks.go:90` compares `task.TaskStatus != statusFilter`
  correctly, and the six status constants
  (`batch_load_tasks.go:10-21`) match the SDK's `BatchLoadStatus` enum
  values verbatim.
- `ListTables.DatabaseName`: already handled deliberately
  (gopherstack-4ly2, cited in `handler_tables.go:269-271`) -- omitting it
  lists every table across every database, which is the real AWS
  behavior, not a bug in this class.

Neither op's `MaxResults` doc comment states a specific number (checked
both, plus every other List/Describe in this service), so the
narrowing/widening-default sub-shape that hit shield/ecs/kms has no
surface here. No range, date, size, or operator-grammar filter exists
anywhere in this service's pinned SDK. Zero bugs found; the service is
structurally too small (2 real filter parameters total) to carry most of
this class's known sub-shapes. No files changed.
