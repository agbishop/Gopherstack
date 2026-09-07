---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: glacier
sdk_module: aws-sdk-go-v2/service/glacier@v1.35.4
last_audit_commit: a073b2b1e2dbd50fb0f95ec57e5af0659ebb0d72
last_audit_date: 2026-08-29
overall: A            # wrapper-key/header/nested-shape sweep (2026-08-20): 1 real wire bug found and fixed (SelectParameters InputSerialization/OutputSerialization.Csv wire key was "Csv", real AWS is lowercase "csv"); 2 suspected wrapper-key bugs (GetVaultAccessPolicy/GetVaultNotifications) investigated and found to be FALSE POSITIVES -- gopherstack's existing flat shape was already correct, the wrapping helper in the real SDK's deserializers.go is dead code never reached from HandleDeserialize. All HTTP-header-bound response members (13 across 8 ops) audited against live HandleDeserialize/HttpBindings functions and found correct. Tree-hash algorithm cross-checked against the pinned SDK's own client-side implementation (internal/customizations/treehash.go), not just self-consistency.
                       # gopherstack-6flj/21my sweep (2026-08-29): 1 real bug found+fixed (ListJobs sorted by JobID instead of CreationDate/initiation-time -- see Notes). ListVaults/ListMultipartUploads/ListParts sort orders re-verified against real API docs (ASCII-by-name / no-guaranteed-order / by-range respectively) and found correct. DescribeCommands/DescribeDeployments-equivalent filters (statuscode/completed on ListJobs) re-verified honored. An existing test (TestSortedListJobs) was asserting the JobID-sort bug as correct behavior; fixed to assert CreationDate order instead.
ops:
  CreateVault:            {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeVault:          {wire: ok, errors: ok, state: ok, persist: ok, note: "GAP (disclosed 2026-09-07, gopherstack-x8em, not fixed this pass -- separate from DeleteVault's fix, filed separately): NumberOfArchives/SizeInBytes/LastInventoryDate are documented as-of-last-inventory (types.DescribeVaultOutput doc: 'The number of archives in the vault as of the last inventory date... returns null if an inventory has not yet run'), but this backend reports the LIVE v.NumberOfArchives/v.SizeInBytes counters instead. DeleteVault's fix added a separate NumberOfArchivesAtLastInventory field for its own check; DescribeVault was left untouched -- reusing that field here is a distinct, larger change (would also need SizeInBytes-at-inventory and null-vs-zero handling) out of this pass's scope."}
  DeleteVault:            {wire: ok, errors: ok, state: ok, persist: ok, note: "cascade-deletes jobs/uploads/lock; blocks per api_op_DeleteVault.go's documented as-of-last-inventory rule (archives at last inventory OR any write since), not the live archive count; this pass fixed a leak where cascade-deleting a vault's multipart uploads dropped the store.Table row but orphaned the raw multipartParts map entry (see Notes). gopherstack-ygfk: consults the vault's lock policy (checkVaultLockDelete) before deleting -- see families: vault_lock_enforcement. FIXED 2026-09-07 (gopherstack-x8em): was checking len(v.Archives) (live count) instead -- see Notes."}
  ListVaults:             {wire: ok, errors: ok, state: ok, persist: ok, note: "marker/limit pagination verified vs SDK Marker/VaultList shape. FIXED 2026-08-29 (gopherstack-6flj constrained-parameter sweep): an unset limit returned every vault instead of defaulting to the documented 10 -- see Notes."}
  UploadArchive:          {wire: ok, errors: ok, state: ok, persist: ok, note: "ArchiveId/Checksum/Location are header-only on real wire (confirmed via awsRestjson1_deserializeOpHttpBindingsUploadArchiveOutput); gopherstack sets all three headers correctly, body is a harmless bonus"}
  DeleteArchive:          {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-ygfk (THIS PASS): now consults the vault's lock policy (checkVaultLockDelete) before deleting -- see families: vault_lock_enforcement"}
  InitiateJob:            {wire: ok, errors: ok, state: ok, persist: ok, note: "response is header-only (X-Amz-Job-Id/x-amz-job-output-path/Location) on real wire; verified. This pass added real support for JobParameters.Type=select (SelectParameters/OutputLocation, full field validation, MissingParameterValueException vs InvalidParameterValueException distinguished) and JobParameters.InventoryRetrievalParameters (range inventory retrieval: StartDate/EndDate/Limit/Marker, validated) -- see Notes. gopherstack-sweep-2026-08-20: request-body SelectParameters.InputSerialization/OutputSerialization.Csv key case fixed (see bug 12, Notes) -- request-side unmarshal was unaffected (Go's case-insensitive JSON decode fallback), only response-side DescribeJob/ListJobs echo was broken"}
  DescribeJob:            {wire: ok, errors: ok, state: ok, persist: ok, note: "GlacierJobDescription now also carries JobOutputPath/OutputLocation/SelectParameters (select jobs) and a proper nested InventoryRetrievalParameters object (range inventory retrieval jobs) -- see Notes for the invented top-level Format field this replaced. gopherstack-sweep-2026-08-20 (bug 12): SelectParameters.InputSerialization/OutputSerialization.Csv wire key fixed from \"Csv\" to lowercase \"csv\" (confirmed via aws-sdk-go-v2/service/glacier@v1.35.4 deserializers.go:awsRestjson1_deserializeDocumentInputSerialization/OutputSerialization, `case \"csv\":`) -- a real SDK client's typed out.SelectParameters.InputSerialization.Csv was always nil before the fix. Proven via TestDescribeJob_SelectCsvSerialization_SDKRoundTrip (wire_sdk_roundtrip_test.go), hand-reverted and confirmed the exact nil-Csv symptom."}
  ListJobs:               {wire: ok, errors: ok, state: ok, persist: ok, note: "same describeJobResponse DTO as DescribeJob, same coverage applies, including bug 12's Csv key fix. FIXED 2026-08-29 (gopherstack-6flj/21my, bug 17): was sorted by JobID (a crypto/rand string with no relationship to creation order) instead of CreationDate ascending -- real ListJobs docs/example responses show ascending-by-initiation-time order. Now sort.SliceStable by CreationDate (fixed-width ISO-8601, so lexical == chronological). statuscode/completed query filters re-verified honored (handler_jobs.go). FIXED 2026-08-29 (gopherstack-6flj constrained-parameter sweep, separate finding): an unset limit returned every job instead of defaulting to the documented 50 -- see Notes."}
  GetJobOutput:           {wire: ok, errors: ok, state: ok, persist: ok, note: "archive-retrieval/inventory-retrieval unchanged; select jobs execute their SQL Expression for real against the stored archive and serve it directly (see select_jobs family note) -- a documented gopherstack convenience, not real AWS behavior (GetJobOutput's own docs cover only archive/inventory output, never Select)"}
  SetVaultNotifications:      {wire: ok, errors: ok, state: ok, persist: ok}
  GetVaultNotifications:      {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-sweep-2026-08-20: investigated as a suspected wrapper-key bug (a \"vaultNotificationConfig\"-wrapping OpDocument helper exists in deserializers.go) and found to be a FALSE POSITIVE -- that helper is dead code, the op's live HandleDeserialize decodes the body FLAT. gopherstack's existing flat response is correct; do not wrap it. Regression-guarded by TestGetVaultNotifications_SDKRoundTrip."}
  DeleteVaultNotifications:   {wire: ok, errors: ok, state: ok, persist: ok}
  SetVaultAccessPolicy:       {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-ygfk: stored and echoed, DELIBERATELY still not enforced -- unlike vault lock policy (see families: vault_lock_enforcement), a vault access policy's documented purpose is granting/restricting access by Principal (cross-account/-role access control), which this emulator cannot evaluate without per-request caller identity (tracked separately, gopherstack-cu4g). Disclosed, not approximated."}
  GetVaultAccessPolicy:       {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-sweep-2026-08-20: same false-positive investigation and outcome as GetVaultNotifications above -- a \"policy\"-wrapping OpDocument helper exists but is dead code; gopherstack's existing flat response is correct. Regression-guarded by TestGetVaultAccessPolicy_SDKRoundTrip."}
  DeleteVaultAccessPolicy:    {wire: ok, errors: ok, state: ok, persist: ok}
  AddTagsToVault:         {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForVault:       {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveTagsFromVault:    {wire: ok, errors: ok, state: ok, persist: ok}
  InitiateVaultLock:      {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-ygfk (THIS PASS): fixed two bugs found while wiring enforcement. (1) LockId was JSON-body-only; real AWS returns it via the x-amz-lock-id response header only (confirmed via awsRestjson1_deserializeOpHttpBindingsInitiateVaultLockOutput, which never touches the body) -- a real SDK client got a nil LockId and could never call CompleteVaultLock. Header now set; JSON body kept as a harmless bonus, same pattern as UploadArchive. (2) the request body's top-level JSON unmarshal error was silently discarded (_ = json.Unmarshal(...)), so a malformed request body was accepted with an empty Policy rather than rejected -- see families: vault_lock_enforcement for the policy-content validation fix alongside it."}
  AbortVaultLock:         {wire: ok, errors: ok, state: ok, persist: ok}
  CompleteVaultLock:      {wire: ok, errors: ok, state: ok, persist: ok}
  GetVaultLock:           {wire: ok, errors: ok, state: ok, persist: ok, note: "24h InProgress expiry verified"}
  GetDataRetrievalPolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FreeTier default matches AWS"}
  SetDataRetrievalPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  InitiateMultipartUpload:   {wire: ok, errors: ok, state: ok, persist: ok, note: "response header-only (Location/x-amz-multipart-upload-id) confirmed"}
  UploadMultipartPart:       {wire: ok, errors: ok, state: ok, persist: ok}
  CompleteMultipartUpload:   {wire: ok, errors: ok, state: ok, persist: ok, note: "response header-only (ArchiveId/Checksum/Location) confirmed, same as UploadArchive. GAP (disclosed, not fixed, out of this sweep's wire-shape scope): unlike UploadArchive, the X-Amz-Sha256-Tree-Hash request header is trusted verbatim (multipart_uploads.go's CompleteMultipartUpload) rather than recomputed from the concatenated part bytes and verified -- a request-validation gap, not a wrong wire shape."}
  AbortMultipartUpload:      {wire: ok, errors: ok, state: ok, persist: ok}
  ListMultipartUploads:      {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (gopherstack-6flj constrained-parameter sweep): an unset limit returned every upload instead of defaulting to the documented 50 -- see Notes."}
  ListParts:                 {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (gopherstack-6flj constrained-parameter sweep): an unset limit returned every part instead of defaulting to the documented 50 -- see Notes."}
  ListProvisionedCapacity:      {wire: ok, errors: ok, state: ok, persist: ok}
  PurchaseProvisionedCapacity:  {wire: ok, errors: ok, state: ok, persist: ok, note: "2-unit cap + monthly expiry verified"}
families:
  route_matching: {status: ok, note: "RouteMatcher + parseGlacierPath path/method table cross-checked against every literal opPath in serializers.go (SplitURI calls) -- all 32 ops match prefix+method; no unreachable-op bug found"}
  persistence: {status: ok, note: "Handler.Snapshot/Restore delegate to InMemoryBackend.Snapshot/Restore (persistence.go); registered snapshot version-guarded (glacierSnapshotVersion); cli.go wiring not touched/verified this pass (out of scope), but Handler exposes the exact Snapshot(ctx)[]byte / Restore(ctx,[]byte)error signature setupPersistence expects. This pass verified the new Job fields (SelectParameters/OutputLocation/JobOutputPath, InventoryRetrieval* range fields) round-trip through Snapshot/Restore (TestPersistenceRoundTrip_SelectAndRangeInventoryJobs) -- additive fields on an already-JSON-round-trippable struct, no snapshot version bump needed"}
  select_jobs: {status: ok, note: "IMPLEMENTED (2026-07-24 pass; S3 write-back added 2026-08-10). InitiateJob Type=select is fully validated (ArchiveId existence, SelectParameters.Expression/ExpressionType=SQL/InputSerialization.Csv/OutputSerialization.Csv all required with MissingParameterValueException vs InvalidParameterValueException distinguished per-field, OutputLocation.S3.BucketName required, Expression syntax-checked) and the SQL query is REALLY executed against the stored archive bytes (select.go/select_sql.go). RESOLVED (2026-08-10): the earlier 'no cross-service S3 write-back' framing was stale -- gopherstack has an S3 backend and this codebase wires cross-service S3 integrations routinely (DynamoDB/MGN/SageMaker precedent). A completed select job now writes its real S3 OutputLocation output when an S3 backend is wired (cli.go's wireGlacierS3): <prefix>/<jobID>/job.txt, results/1, result_manifest.txt (or errors/1 + error_manifest.txt on query failure), matching the exact key layout documented in glacier-select.md's 'S3 Glacier Select Output' section (awsdocs/amazon-glacier-developer-guide, doc_source/glacier-select.md:49-65). GetJobOutput continues to ALSO serve the same real (not stubbed) computed bytes directly as a documented gopherstack convenience -- confirmed via aws-sdk-go-v2/service/glacier@v1.35.4's GetJobOutput doc and api-job-output-get.md that real AWS's GetJobOutput contract covers only archive-retrieval and inventory-retrieval output, never Select, so there is no real behavior to cite for rejecting it there instead. See select_output.go and select.go's package doc."}
  range_inventory_retrieval: {status: ok, note: "IMPLEMENTED this pass (was deferred). InventoryRetrievalParameters (StartDate/EndDate/Limit/Marker) on InitiateJob is validated (ISO-8601 dates, positive-integer Limit) and echoed back correctly nested under InventoryRetrievalParameters on DescribeJob/ListJobs (inventory_retrieval.go). GetJobOutput's inventory listing is actually filtered by the stored parameters: StartDate inclusive / EndDate exclusive bound on Archive.CreationDate, Marker resumes strictly after the named ArchiveId, Limit caps the count -- filterArchivesForInventory, covered by TestGetJobOutput_InventoryRetrieval_{DateRangeFilters,Limit,Marker}."}
  vault_lock_enforcement: {status: ok, note: "FIXED this pass (gopherstack-ygfk, the mirror of gopherstack-cqy3's cloudformation stack-policy fix): InitiateVaultLock/SetVaultLock stored a policy on VaultLock.Policy and GetVaultLock echoed it, but neither DeleteArchive nor DeleteVault ever read it -- a Vault Lock policy denying deletion did nothing, the exact write-only-state class this issue tracks. Fixed via vault_lock_policy_eval.go (new): parses Statement[].{Effect,Action,Resource,Condition}, evaluated from checkVaultLockDelete (vault_lock.go) called by both DeleteArchive and DeleteVault before mutating any state, while the lock is InProgress OR Locked (AWS's documented 'test your policy before locking it down' workflow evaluates requests against an InProgress lock too, per vault-lock.html). Implemented: Effect=Deny only (Action glacier:DeleteArchive/glacier:DeleteVault with '*' wildcards, Resource the vault ARN with '*' wildcards per glacier-api-permissions-ref.html's vaults/example*, vaults/* patterns), plus the canonical glacier:ArchiveAgeInDays numeric condition (NumericLessThan/LessThanEquals/GreaterThan/GreaterThanEquals/Equals) against Archive.CreationDate -- the documented WORM/retention use case (vault-lock-policy.html Example 1: 'Deny Deletion Permissions for Archives Less Than 365 Days Old'). DISCLOSED, not approximated: Effect=Allow is parsed but grants nothing (no IAM baseline in this emulator for a resource policy to combine with, and AWS documents no CloudFormation-style default-deny-once-a-policy-exists rule for Glacier the way it does for stack policies -- fabricating one would risk blocking permitted deletes); Principal is parsed but not evaluated (no per-request caller identity, gopherstack-cu4g -- every AWS-documented Vault Lock example uses Principal '*' since the feature's whole point is 'prevent anyone, including the account owner'); the ResourceTag condition key (Example 2's legal-hold pattern) is not implemented (Glacier archives carry no tags in this emulator); only DeleteArchive/DeleteVault consult the policy, not UploadArchive/InitiateJob/other Vault-Lock-governable actions (out of scope for a deletion-protection pass). Evaluation semantics are TRANSCRIBED FROM AWS'S DOCUMENTATION (vault-lock.html, vault-lock-policy.html, glacier-api-permissions-ref.html), not the SDK -- the policy body is an opaque string with no wire type in aws-sdk-go-v2, same disclosure shape as cloudformation's stack policy. Also fixed: SetVaultLock now rejects malformed policy JSON at write time (previously accepted, would never have enforced anything even after this fix, same bug class as cloudformation's SetStackPolicy); InitiateVaultLock's request-body top-level JSON unmarshal error was silently discarded, now returns 400 (see ops: InitiateVaultLock for the two wire bugs -- missing x-amz-lock-id header, swallowed unmarshal error -- found while adding this and fixed alongside it). Verified via TestVaultLockPolicy_DeleteEnforcement (sdk_vault_lock_enforcement_test.go), driven through the real aws-sdk-go-v2 client: a blanket Deny blocks both DeleteArchive and DeleteVault and the resource is provably unchanged afterward (DescribeVault NumberOfArchives / a follow-up DescribeVault succeeding), a Deny scoped to a different vault or a different action does not block, the ArchiveAgeInDays condition both blocks and (once its threshold isn't met) permits, enforcement holds during the InProgress test window not only once Locked, no lock ever initiated allows deletion, and malformed policy JSON is rejected at InitiateVaultLock. Hand-reverted checkVaultLockDelete to a no-op and confirmed exactly the 4 refusal-asserting subtests fail while the 5 permitted-path subtests still pass, then restored."}
gaps:
  - select_sql_subset: "VERIFIED 2026-08-10 against awsdocs/amazon-glacier-developer-guide's doc_source/s3-glacier-select-sql-reference*.md (the real SQL reference, shared verbatim with S3 Select except where a page says '(Amazon S3 Select only)'). Correct-as-is: JOINs/subqueries are genuinely unsupported by real Glacier Select too ('Amazon S3 Select and S3 Glacier Select queries currently do not support subqueries or joins' -- s3-glacier-select-sql-reference-select.md), so gopherstack's lack of joins is not a gap. Real gaps (real Glacier Select supports these, gopherstack does not): CAST (s3-glacier-select-sql-reference-conversion.md: 'Amazon S3 Select and S3 Glacier Select support the following conversion functions: CAST' -- no '(S3 Select only)' qualifier), NOT/BETWEEN/IN/LIKE operators and arithmetic (+ - * %) (s3-glacier-select-sql-reference-operators.md's Logical/Comparison/Pattern-Matching/Math Operators sections), and COALESCE/NULLIF (s3-glacier-select-sql-reference-conditional.md). Closing these is moderate: BETWEEN/IN/LIKE/NOT extend select_sql.go's existing predicate grammar (parsePredicate/selectPredicateMatches) without new architecture; arithmetic and CAST need a real scalar-expression evaluator (select_sql.go's WHERE/SELECT-list values are currently bare column refs or literals, not expressions) -- a bigger, structural addition. Parenthesized/nested-boolean grouping has NO citable evidence either way: the real SQL reference's exhaustive 'Scalar Expressions' grammar list (literal | column_reference | unary_op expr | expr binary_op expr | func_name | BETWEEN | LIKE) never includes a generic '( expression )' grouping form, unlike CAST/IN/COALESCE's function-call parens, so gopherstack's flat OR-of-AND WHERE clause (no parenthesized override) is left as-is rather than extended speculatively -- do not add parenthesized grouping without a citable source. NOT extending speculatively per this pass's instructions; not implemented this pass."
  - "Vault Lock policy enforcement (gopherstack-ygfk) only evaluates Effect=Deny (Allow is a no-op -- no IAM baseline to grant against), ignores Principal (no per-request caller identity, gopherstack-cu4g), does not support the ResourceTag condition key (Glacier archives carry no tags here), and only gates DeleteArchive/DeleteVault (not UploadArchive/InitiateJob/other Vault-Lock-governable actions) -- see families: vault_lock_enforcement for the full disclosure. Vault ACCESS policies (SetVaultAccessPolicy) remain entirely unenforced -- their purpose is Principal-based access control, which needs the same caller-identity infrastructure gopherstack-cu4g is deciding, and is a different, larger gap than deletion protection."
deferred: []
leaks: {status: clean, note: "no goroutines/janitors in this service; retrievalDelay promotion is read-triggered (promoteJobIfReady), not a background timer. FIXED this pass: DeleteVault's multipart-upload cascade deleted the store.Table row but never the corresponding raw-map multipartParts[uploadKey] row (AbortMultipartUpload/CompleteMultipartUpload already did this correctly; DeleteVault's cascade loop did not) -- every vault deleted with an in-progress multipart upload left an orphaned parts row forever. Fixed in vaults.go's DeleteVault; regression test TestDeleteVault_CascadeCleansMultipartParts (leak_test.go)."}
---

## Notes

### `last_audit_commit` provenance (2026-08-20 sweep)

The manifest's previous `last_audit_commit`
(`f8ae77eb7c84189d9fca29cce357a9cfaf72fd9c`) was dated **2026-07-24** —
`git show -s --format=%ad` — while the manifest's own `last_audit_date` said
**2026-08-10**, and the manifest's own body documented an even later
**2026-08-14** pass (bugs 9–11, `vault_lock_enforcement`, commit
`665fb5fbb` "fix(glacier): Vault Lock policies were stored, echoed, and
never enforced", also dated 2026-08-14). So the cited sha was stale by
multiple substantive commits and roughly three weeks relative to the
manifest's own claimed audit date, and the manifest was *self-inconsistent*
(claiming a 2026-08-10 audit date while documenting 2026-08-14 work) —
the same stale-citation pattern this campaign previously caught on appmesh
and codeconnections. **Verdict: WRONG, now corrected.** `last_audit_commit`
is updated to this session's actual HEAD
(`a073b2b1e2dbd50fb0f95ec57e5af0659ebb0d72`, 2026-08-20).

Protocol: **restjson1** (AWS restJson1, not query-XML). Response bodies are JSON;
request/response IDs and checksums are carried in **headers**, not JSON body, for
UploadArchive / CompleteMultipartUpload / InitiateJob / InitiateMultipartUpload
(confirmed via `awsRestjson1_deserializeOpHttpBindings*Output` functions in the
real SDK's `deserializers.go` — these ops use header-only output shapes). Timestamps
are ISO-8601 strings (`2006-01-02T15:04:05.000Z`), never epoch numbers — confirmed
correct throughout (`formatDate` in models.go).

### Bugs fixed this pass

1. **`DescribeJob`/`ListJobs` missing `ArchiveSHA256TreeHash` wire field.**
   The real Glacier `GlacierJobDescription` shape has **two distinct** checksum
   fields: `ArchiveSHA256TreeHash` (checksum of the *entire archive*, archive
   metadata available as soon as the job exists) and `SHA256TreeHash` (checksum
   of the *retrieved range*, null while the job is `InProgress`, confirmed via
   the real deserializer's `case "ArchiveSHA256TreeHash":` / `case
   "SHA256TreeHash":` switch arms in `deserializers.go`). gopherstack's
   `describeJobResponse` only had `SHA256TreeHash` and set it eagerly at
   `InitiateJob` time regardless of completion state — so every real SDK client
   calling `DescribeJob`/`ListJobs` for a completed `ArchiveRetrieval` job got a
   **nil `ArchiveSHA256TreeHash`**, permanently losing the documented way to
   verify the full-archive checksum via `DescribeJob` (see the SDK's own
   `GetJobOutput` doc comment, which tells callers to cross-check downloaded
   chunks against `DescribeJob`'s archive checksum). Fixed: `Job` now carries
   `ArchiveSHA256TreeHash` (set immediately at `InitiateJob`, from archive
   metadata) separately from `SHA256TreeHash` (set only once
   `promoteJobIfReady` transitions the job to `Succeeded`), and
   `describeJobResponse` serializes both under their correct AWS field names.

2. **`GetJobOutput` missing `X-Amz-Archive-Description` response header.**
   For archive-retrieval jobs, real AWS returns the archive's description via
   the `x-amz-archive-description` response header (confirmed via
   `awsRestjson1_deserializeOpHttpBindingsGetJobOutputOutput`, which populates
   `GetJobOutputOutput.ArchiveDescription` purely from that header — there is
   no JSON-body equivalent). `handleArchiveJobOutput` never set this header, so
   `output.ArchiveDescription` was always nil for every archive download.
   Fixed: `Job` now carries `ArchiveDescription` (copied from the `Archive` at
   `InitiateJob` time — internal field, not part of the `DescribeJob` DTO,
   since AWS has no such field there), and `handleArchiveJobOutput` sets the
   header when non-empty.

### Bugs/gaps fixed this pass (2026-07-24)

3. **Select jobs (`Type=select`) were entirely unimplemented** — `InitiateJob`
   only recognized `archive-retrieval`/`inventory-retrieval`, so any real SDK
   client requesting a select job got a generic `InvalidParameterValueException`
   for an unrecognized `Type` instead of a working job. Implemented for real:
   full request-shape validation (`SelectParameters`/`OutputLocation` field-by-
   field against the real `JobParameters`/`SelectParameters`/`OutputLocation`/
   `S3Location`/`CSVInput`/`CSVOutput` types), a real SQL query engine
   (`select.go`, `select_sql.go`) that actually executes the `Expression`
   against the archive's CSV bytes, and correct `GlacierJobDescription` echo of
   `JobOutputPath`/`OutputLocation`/`SelectParameters`. See the `select_jobs`
   family note above for the one documented AWS-behavior deviation (GetJobOutput
   delivery in lieu of cross-service S3 write-back).

4. **Range inventory retrieval (`InventoryRetrievalParameters`) was entirely
   unimplemented** — the request field was silently dropped, so inventory jobs
   always returned the full vault inventory regardless of any
   `StartDate`/`EndDate`/`Limit`/`Marker` the caller specified, with no
   validation error to warn them. Implemented for real: validated parsing,
   correct nested-object echo on `DescribeJob`/`ListJobs` (see bug 5 below),
   and actual `CreationDate`-range/marker/limit filtering of the inventory
   returned by `GetJobOutput` (`inventory_retrieval.go`).

5. **`describeJobResponse.InventoryFormat` (`json:"Format"`) was a
   gopherstack-invented top-level field** — the real `GlacierJobDescription`
   type has **no top-level `Format` field** at all; `Format` only ever exists
   nested under `InventoryRetrievalParameters`. Per this campaign's "delete
   gopherstack-invented fields" rule, the top-level field is now gone,
   replaced by a real `InventoryRetrievalParameters` nested object (which also
   now carries `StartDate`/`EndDate`/`Limit`/`Marker`, previously entirely
   absent — see bug 4). (Previously this was logged as a "harmless, do not
   fix" trap because removing it without also implementing the real nested
   object would have been a net regression; it is safe now that the real
   field exists.)

6. **`DeleteVault` leaked `multipartParts` rows** (leak, not a wire bug) — see
   the `leaks` field above for detail; fixed in `vaults.go`.

### Bugs/gaps fixed this pass (2026-08-10)

7. **Select job SQL grammar accepted `LIMIT`, which real S3 Glacier Select
   explicitly does not support.** The prior pass's framing ("SQL subset
   mirrors real Glacier Select's own subset") was inherited rather than
   verified — checking `awsdocs/amazon-glacier-developer-guide`'s
   `doc_source/s3-glacier-select-sql-reference-select.md` shows `LIMIT` is
   documented as `(Amazon S3 Select only)`: "**S3 Glacier Select does not
   support the `LIMIT` clause**." A real Glacier Select client sending
   `SELECT * FROM archive LIMIT 5` gets rejected; gopherstack silently
   accepted and honored it — an over-permissive superset bug, not a
   documented-subset omission. Fixed: `select_sql.go`'s parser now rejects any
   `LIMIT` clause with the same `ErrSelectExpression`/`InvalidParameterValueException`
   path used for other malformed expressions, at `InitiateJob`-time syntax
   validation (matching real AWS's synchronous rejection). See the
   `select_sql_subset` gap entry above for the fuller SQL-grammar audit this
   also prompted (CAST/BETWEEN/IN/LIKE/NOT/arithmetic are real gaps in the
   other direction — Glacier Select supports them, gopherstack does not).

8. **Select job results were never written to the real S3 `OutputLocation`.**
   Resolved by wiring a real S3 backend (`cli.go`'s `wireGlacierS3`,
   `select_output.go`'s `materializeSelectOutput`) — see the `select_jobs`
   family note above for the full account of what changed and why the prior
   "no cross-service S3 write-back" framing was stale.

### Bugs fixed this pass (2026-08-14, gopherstack-ygfk)

9. **Vault Lock policy was stored, echoed by `GetVaultLock`, and never
   consulted by `DeleteArchive`/`DeleteVault`** — the security variant of the
   write-only-state class this issue tracks (mirrors `gopherstack-cqy3`'s
   CloudFormation stack-policy fix). A policy denying deletion of an archive
   or vault did nothing; the write succeeded, the read confirmed it, and the
   protection it names was cosmetic. Fixed: see the `vault_lock_enforcement`
   family entry above for the full implemented/disclosed breakdown, sourcing,
   and test evidence.

10. **`InitiateVaultLock`'s `LockId` was JSON-body-only.** Real AWS returns it
    exclusively via the `x-amz-lock-id` response header (confirmed via
    `awsRestjson1_deserializeOpHttpBindingsInitiateVaultLockOutput`, which
    never touches the body) — a real `aws-sdk-go-v2` client always got a nil
    `LockId` and could never call `CompleteVaultLock`. Found while writing an
    SDK-driven test for bug 9 above (the completion step failed with
    `"input member lockId must not be empty"`), not something a raw-HTTP test
    could have caught. Fixed: `handleVaultLock` now also sets the header;
    the JSON body is kept as a harmless bonus, same pattern already used for
    `UploadArchive`'s `x-amz-archive-id`.

11. **`InitiateVaultLock`'s request-body JSON unmarshal error was silently
    discarded** (`_ = json.Unmarshal(body, &req)`), so a malformed request
    body was accepted as an empty `Policy` instead of rejected. Fixed
    alongside `SetVaultLock` now also rejecting a malformed *inner* policy
    document at write time (previously accepted, would never have enforced
    anything even after bug 9's fix) — same bug class as `cloudformation`'s
    `SetStackPolicy` fix. Three existing tests used a non-JSON placeholder
    policy string (`"p"`) that only "worked" because the error was swallowed;
    updated to `"{}"`, and one test's inner-policy JSON was embedded
    unescaped (also only working by accident of the swallowed error) and is
    now properly escaped.

### Bugs fixed / findings this pass (2026-08-20, wrapper-key/header sweep)

12. **`SelectParameters.InputSerialization.Csv` / `OutputSerialization.Csv` wire
    key was `"Csv"`, real AWS is lowercase `"csv"`.** Confirmed via
    `aws-sdk-go-v2/service/glacier@v1.35.4`'s `deserializers.go`
    (`awsRestjson1_deserializeDocumentInputSerialization`/`...OutputSerialization`,
    both `case "csv":`) and `serializers.go` (both
    `object.Key("csv")`) — an anomaly among glacier's otherwise-PascalCase
    field names, live-checked (not the OpDocument-helper trap; `DescribeJob`'s
    document deserializer genuinely is the live path, see finding 13). Impact
    was one-directional: the *request*-parsing side (`InitiateJob`'s
    `initiateJobRequest`) was unaffected because Go's `encoding/json.Unmarshal`
    falls back to case-insensitive key matching, but the *response* side
    (`DescribeJob`/`ListJobs` echoing `SelectParameters` back) used
    `json.Marshal`, which always emits the exact tag — so a real SDK client's
    typed `out.SelectParameters.InputSerialization.Csv` /
    `.OutputSerialization.Csv` were always `nil` after a completed select job,
    even though gopherstack had real values to report. Fixed in `models.go`
    (`inputSerializationDTO`/`outputSerializationDTO`). Proven via
    `TestDescribeJob_SelectCsvSerialization_SDKRoundTrip`
    (`wire_sdk_roundtrip_test.go`) driven through a real
    `aws-sdk-go-v2/service/glacier` client; hand-reverted and confirmed the
    exact predicted symptom (`require.NotNil` on `Csv` failing with a `nil`
    value), then restored and confirmed the diff is byte-identical to the fix.

13. **False-positive investigated and correctly NOT fixed:
    `GetVaultAccessPolicy`/`GetVaultNotifications` do NOT need a wrapper key.**
    Both ops have an `awsRestjson1_deserializeOpDocument<Op>Output` helper in
    `deserializers.go` whose case list matches on a wrapper key (`"policy"` /
    `"vaultNotificationConfig"`) — following only that helper (as this pass
    initially did, applying it and updating tests to match) is precisely the
    dead-code trap `gopherstack-sdk-shape`'s SKILL.md and this campaign's
    `_WRAPPER_KEY_SWEEP_REMAINDER.md` warn about (previously caught on
    appmesh, `gopherstack-cnhp`). Reading each op's own live
    `HandleDeserialize` shows both actually call the nested document
    deserializer (`awsRestjson1_deserializeDocumentVaultAccessPolicy` /
    `...VaultNotificationConfig`) **directly on the raw decoded body**, never
    through the wrapping helper — so the real wire response is FLAT at the
    response root, exactly what gopherstack already emitted. The "fix" was
    applied, immediately falsification-tested with a real SDK round trip
    (which failed with all-nil typed fields despite a 200 and a body that
    *did* contain the data, just unwrapped), diagnosed, and fully reverted
    (`models.go`, `handler_vault_access_policy.go`,
    `handler_vault_notifications.go`, and both handler tests are byte-identical
    to pre-session). Regression-guarded going forward by
    `TestGetVaultAccessPolicy_SDKRoundTrip` /
    `TestGetVaultNotifications_SDKRoundTrip` (`wire_sdk_roundtrip_test.go`),
    which assert the real typed SDK client decodes non-nil `Policy` /
    `VaultNotificationConfig` from gopherstack's flat body. **Lesson for the
    next auditor**: for restjson1, before trusting any
    `deserializeOpDocument<Op>Output`'s case list, confirm the op's own
    `HandleDeserialize` actually calls it — `grep -c` counting only tells you
    the helper is *defined*, not that it's *reachable*.

14. **Header-bound response members audited against live `HandleDeserialize`
    (not the dead-code-prone document helpers), all correct, 0 bugs.** Every
    op with an `awsRestjson1_deserializeOpHttpBindings<Op>Output` function was
    enumerated from `deserializers.go` directly (not assumed from a sibling):
    `CreateVault` (`Location`), `UploadArchive`
    (`Location`/`x-amz-archive-id`/`x-amz-sha256-tree-hash`),
    `InitiateMultipartUpload` (`Location`/`x-amz-multipart-upload-id`),
    `UploadMultipartPart` (`x-amz-sha256-tree-hash`),
    `CompleteMultipartUpload`
    (`Location`/`x-amz-archive-id`/`x-amz-sha256-tree-hash`), `InitiateJob`
    (`Location`/`x-amz-job-id`/`x-amz-job-output-path`), `InitiateVaultLock`
    (`x-amz-lock-id`), `PurchaseProvisionedCapacity` (`x-amz-capacity-id`),
    `GetJobOutput`
    (`Accept-Ranges`/`Content-Range`/`Content-Type`/`x-amz-archive-description`/`x-amz-sha256-tree-hash`).
    All 9 ops' headers are present and correctly named in gopherstack's
    handlers (`handler_vaults.go`, `handler_archives.go`,
    `handler_multipart_uploads.go`, `handler_jobs.go`, `handler_vault_lock.go`,
    `handler_provisioned_capacity.go`); `GetJobOutput`'s `X-Amz-Archive-Description`
    and `ArchiveSHA256TreeHash` fixes from prior passes still hold. `GetJobOutput`
    also correctly returns `206 Partial Content` with a computed `Content-Range`
    for a `Range`-bound retrieval (`serveWithRange`, `handler_jobs.go`), matching
    real Glacier's ranged-retrieval status code.

15. **Tree-hash algorithm cross-checked against the pinned SDK's own
    client-side implementation**, not just round-trip self-consistency: read
    `aws-sdk-go-v2/service/glacier@v1.35.4/internal/customizations/treehash.go`
    (the middleware that auto-computes `X-Amz-Sha256-Tree-Hash` on the client
    when a caller doesn't set it) and confirmed gopherstack's
    `computeLeafHashes`/`reduceTreeHashes` (`handler_archives.go`) implement
    the identical algorithm: 1 MiB leaf chunks, pairwise SHA-256 concatenation
    up the tree, odd node carried unchanged to the next level. Existing tests
    (`TestTreeHash_EmptyBody`/`_SingleBlock`/`_TwoBlocks`,
    `handler_archives_test.go`) already assert against independently-computed
    `crypto/sha256` primitives rather than gopherstack's own recursive
    function, and every SDK-driven test in this service that calls
    `client.UploadArchive` with a real body (this pass's new tests included)
    exercises the strongest possible cross-check: the real SDK client
    computes and sends `X-Amz-Sha256-Tree-Hash` using *its own*
    `treehash.go` algorithm, and gopherstack's server-side `computeTreeHash`
    must produce the same value or the upload is rejected as a mismatch — all
    such uploads in this test suite succeed.

16. **Gap disclosed, not fixed (out of this sweep's scope — a request
    validation gap, not a wrong wire shape): `CompleteMultipartUpload` trusts
    the client-supplied `X-Amz-Sha256-Tree-Hash` header verbatim** rather than
    recomputing it from the concatenated part bytes and rejecting a mismatch
    the way `UploadArchive` does (`multipart_uploads.go`'s
    `CompleteMultipartUpload`). See the `CompleteMultipartUpload` op entry
    above.

### Bugs fixed / findings this pass (2026-08-29, gopherstack-6flj/21my sweep)

17. **`ListJobs` sorted by `JobID` instead of job initiation time.** The real
    `ListJobs` API (`api_op_ListJobs.go`'s doc comment: "The List Jobs operation
    ... returns a list of these jobs sorted by job initiation time") and its own
    reference-doc example responses (both examples' `JobList` entries appear in
    ascending `CreationDate` order) confirm the real sort key is `CreationDate`,
    ascending. gopherstack's `ListJobs` (`jobs.go`) instead sorted by `JobID` --
    a string generated via `crypto/rand` (`generateID`) with **zero**
    relationship to creation order, so the returned order was effectively
    random relative to what a real client would see. This is the class of bug
    this campaign specifically flags: a dropped/wrong SORT key, not a dropped
    filter. Fixed: `sort.SliceStable` by `CreationDate` (a fixed-width
    ISO-8601 string via `formatDate`, so lexical string comparison is
    equivalent to chronological order; `SliceStable` keeps ties deterministic).
    An existing test, `TestSortedListJobs` (`jobs_test.go`), was asserting the
    buggy JobID-sort as the expected behavior -- exactly the
    "existing-tests-can-be-wrong" trap this campaign warns about -- and was
    fixed alongside to assert `CreationDate` order instead. Proven via
    `TestListJobs_SortedByInitiationTime_SDKRoundTrip`
    (`wire_sdk_roundtrip_test.go`), driven through a real
    `aws-sdk-go-v2/service/glacier` client: three jobs are initiated (so
    insertion order and JobID-lexical order both differ from the intended
    result), their `CreationDate`s are backdated (via a new
    `SetJobCreationDate` test-only export) to a third, deliberately different
    order, and `ListJobs` is asserted to return exactly that
    `CreationDate`-ascending order. Confirmed failing against the pre-fix code
    (returned JobID-lexical order instead) before the fix, and passing after.

    **Sort-order cross-check on siblings, done this pass (not previously
    recorded in this file):** `ListVaults` re-confirmed correct against the
    real API doc's explicit "The list returned in the response is ASCII-sorted
    by vault name" (gopherstack sorts by `VaultName`, `vaults.go`).
    `ListMultipartUploads` re-confirmed correct against the real API doc's
    explicit "The list returned in the List Multipart Upload response has no
    guaranteed order" (gopherstack's `MultipartUploadID`-lexical sort is a
    valid, deterministic choice under that contract). `ListParts` re-confirmed
    correct against the real API doc's explicit "Amazon Glacier returns the
    part list sorted by range you specified in each part upload" (gopherstack
    sorts by `RangeInBytes` start, `multipart_uploads.go`). The vault-inventory
    `ArchiveList` (served via `GetJobOutput` for `InventoryRetrieval` jobs,
    `archives.go`'s `ListArchives`, sorted by `ArchiveID`) has **no** citable
    real-API statement of guaranteed order either way (its doc page states no
    ordering guarantee), so its existing `ArchiveID`-lexical sort is left
    as-is per this campaign's do-not-fabricate rule -- not flagged as a bug,
    also not asserted as definitely correct.

    **Filter re-verification, done this pass:** `ListJobs`' `statuscode`
    (`InProgress`/`Succeeded`/`Failed`) and `completed`
    (`true`/`false`) query-parameter filters (`handler_jobs.go`) re-confirmed
    honored, not silently dropped -- these are the closest glacier analogue to
    the "list ops carry markers, limits and status filters" risk this
    campaign specifically calls out for this service.

18. **`ListJobs`/`ListMultipartUploads`/`ListParts`/`ListVaults` all left
    `limit` unbounded when the client sent none, instead of the SDK's own
    documented per-op default.** Confirmed protocol as `awsRestjson1_*` from
    `serializers.go`'s function prefixes (per this campaign's warning not to
    assume glacier's protocol from its neighbours -- it genuinely is
    REST-JSON, just with header/query-heavy bindings typical of an older REST
    API). Each op's own `api_op_List*.go` doc comment states an explicit
    default: `ListJobs`/`ListMultipartUploads`/`ListParts` all say "The
    default limit is 50"; `ListVaults` says "The default limit is 10". Every
    one of the four handlers' pagination code (`paginateJobList`/
    `paginateUploadList`/`paginatePartList` in `handler_jobs.go`/
    `handler_multipart_uploads.go`, and `handleListVaults` in
    `handler_vaults.go`) had the same shape: `if limitStr == "" { return
    items, nil, nil }` -- an empty `?limit` short-circuited straight past the
    cap entirely, returning every item unbounded (and no `Marker`, so a real
    paginating client would see one page and stop, silently missing anything
    beyond the true default page). This is the same bug class already fixed
    campaign-wide in mq/wafv2/emr/rekognition/ses (item 10 in the brief): the
    default was simply never applied, not merely mis-valued. Fixed by
    defaulting `n` to the new `defaultListJobsLimit`/`defaultListUploadsLimit`/
    `defaultListVaultsLimit` constants (50/50/10, `handler.go`) before the
    `?limit` branch, so an explicit `?limit` still overrides it exactly as
    before and only the omitted case changed.

    New tests in `list_filter_params_test.go`, each driven through the real
    `aws-sdk-go-v2/service/glacier` client and confirmed to fail against
    unmodified code first (returned every seeded item with no `Marker`
    instead of capping at the default and returning one):
    `TestListJobs_DefaultLimit`, `TestListMultipartUploads_DefaultLimit`,
    `TestListParts_DefaultLimit`, `TestListVaults_DefaultLimit`. The
    `ListParts` test needed 51 parts without the cost of 51 real 1&nbsp;MiB
    uploads + tree-hash computation, so a new internal test-seeding helper,
    `AddMultipartPartInternal` (`multipart_uploads.go`), was added alongside
    the pre-existing `AddVaultInternal`/`AddJobInternal`/
    `AddMultipartUploadInternal` (same convention: bypass real upload
    mechanics, write directly into the backend's raw `multipartParts` map).

### Traps for the next auditor

- **Dead-code `deserializeOpDocument<Op>Output` wrapper helpers.**
  `GetVaultAccessPolicy` and `GetVaultNotifications` each have a
  `awsRestjson1_deserializeOpDocument<Op>Output` function in
  `deserializers.go` whose case list matches a wrapper key (`"policy"` /
  `"vaultNotificationConfig"`) — but neither op's actual
  `HandleDeserialize` calls it; both call the nested document deserializer
  directly on the raw body instead, so the real wire shape is FLAT, not
  wrapped. Always confirm which function `HandleDeserialize` itself calls
  before trusting an OpDocument helper's case list — see finding 13 above
  (2026-08-20) for the full account of this pass tripping over exactly that
  trap, catching it via a real SDK round-trip test, and reverting.
- `UploadArchive` / `CompleteMultipartUpload` / `InitiateJob` /
  `InitiateMultipartUpload` responses carry a JSON body in gopherstack
  (`uploadArchiveResponse`, `completeMultipartUploadResponse`,
  `initiateJobResponse`, `initiateMultipartUploadResponse`) even though real
  AWS returns an **empty body** for these ops (all data is in headers). This is
  intentional and harmless: the real SDK's `awsRestjson1_deserializeOp*`
  handlers for these ops never call the JSON-body document deserializer, only
  the HTTP-bindings (header) one, so the body is simply never parsed by a real
  client. Do not flag the body-in-a-header-only-op pattern as a bug.
- `ErrResourceInUse` → `ResourceInUseException` and `ErrVaultNotEmpty` /
  `ErrLockConflict` / `ErrLockAlreadyLocked` → `ConflictException` /
  `InvalidParameterValueException`, and (added this pass) `ErrVaultLockDenied`
  → `AccessDeniedException`, are **not** modeled exception types in
  `aws-sdk-go-v2/service/glacier/types/errors.go` (the SDK only models
  `InsufficientCapacityException`, `InvalidParameterValueException`,
  `LimitExceededException`, `MissingParameterValueException`,
  `NoLongerSupportedException`, `PolicyEnforcedException`,
  `RequestTimeoutException`, `ResourceNotFoundException`,
  `ServiceUnavailableException`). `AccessDeniedException` is documented (the
  real error-responses table: "Returned if there was an attempt to access a
  resource not allowed by an IAM policy", 403) even though the SDK doesn't
  model it as a typed error. `PolicyEnforcedException` IS SDK-typed but is a
  different feature entirely -- it covers data-retrieval-rate-limit denials
  (`GetDataRetrievalPolicy`/`SetDataRetrievalPolicy`), not Vault Lock policy
  denials; do not repurpose it for `checkVaultLockDelete`. Real clients still
  get a working `smithy.GenericAPIError` with the correct `Code`/`Message`/HTTP
  status for any unmodeled code (falls through to the generic-error `default:`
  branch in every
  `awsRestjson1_deserializeOpError*` function) — this is NOT a bug, just an
  SDK modeling gap on AWS's side that gopherstack correctly works around.
- Route matching (`RouteMatcher` + `parseGlacierPath`) was cross-checked
  against every literal `httpbinding.SplitURI(...)` path string in the real
  SDK's `serializers.go` (32 matches, one per op) plus HTTP verb per branch —
  no unreachable-op bug found this pass, unlike several other services hit by
  that bug class.
- The Select job SQL engine (`select_sql.go`) supports a **subset** of SQL,
  not full ANSI SQL — but do not assume the subset boundary without checking
  the real SQL reference first (a prior pass's "mirrors real Glacier Select's
  own subset" claim turned out partly wrong — see the `select_sql_subset` gap
  entry above, and bug 7). Verified as of 2026-08-10: joins/subqueries are a
  correct omission (real Glacier Select doesn't support them either); `LIMIT`
  is correctly rejected (real Glacier Select doesn't support it, unlike S3
  Select); `CAST`, `NOT`/`BETWEEN`/`IN`/`LIKE`, arithmetic operators, and
  `COALESCE`/`NULLIF` are real, uncited-as-fixed gaps (real Glacier Select
  supports all of them); parenthesized/nested-boolean grouping has no
  evidence either way and is deliberately left unextended. See `select.go`'s
  package doc comment for the exact grammar and citations.
- Select job results are served via `GetJobOutput` in gopherstack IN ADDITION
  TO the real S3 `OutputLocation` write-back (`select_output.go`,
  `materializeSelectOutput`, wired via `cli.go`'s `wireGlacierS3`) added
  2026-08-10. GetJobOutput serving Select output directly is NOT modeled by
  real AWS (its own docs cover only archive/inventory output), but there is
  also no citable real error behavior to reject it with — so this remains a
  harmless, documented convenience layered on top of the now-real S3 delivery
  path, not a replacement for it. Do not remove the S3 write-back thinking
  GetJobOutput alone is sufficient; do not "fix" GetJobOutput by rejecting
  Select jobs without a cited real error code to match.

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

Audited this package's marker-based pagination for the Class A/B/C shapes
found elsewhere in this campaign. No bug found.

Three helpers — `paginateUploadList`/`paginatePartList`
(`handler_multipart_uploads.go` — `ListMultipartUploads`/`ListParts`) and
`paginateJobList` (`handler_jobs.go` — `ListJobs`), explicitly marked
`//nolint:dupl` as sharing identical structure — search for the marker's
named item by exact equality (`MultipartUploadID`/`RangeInBytes`/`JobID`)
and, on a miss, set `items = items[:0]`: an **empty** result, not index 0.
That's already the safe default this campaign's Class B/C fix recommends
elsewhere, so a stale or tampered marker terminates instead of looping —
this is the one hand-rolled pattern of the eight services audited this pass
that got the miss case right on the first try.

All three take `*echo.Context` directly rather than a value that's cheap to
unit-test, so this was verified through the real
`aws-sdk-go-v2/service/glacier` client (`pagination_arithmetic_test.go`) —
a boundary walk of `ListJobs` one item at a time reproduces every seeded
job, and a marker naming no known job returns an empty page rather than
restarting. `ListJobs` was taken as representative of all three given the
identical, `nolint:dupl`-acknowledged structure; the other two were not
independently unit-tested this pass.

Gates: `go build ./services/glacier/...`, `go vet ./services/glacier/...`
and `go vet ./...` (repo-wide, clean), `go test -race -count=1
./services/glacier/...`, `golangci-lint run ./services/glacier/...` (0
issues). No production code changed this pass — test-only additions
confirming correctness.

## 2026-09-07 DeleteVault as-of-last-inventory fix (gopherstack-x8em)

`api_op_DeleteVault.go`'s doc comment, quoted verbatim: "Amazon Glacier will
delete a vault only if there are no archives in the vault as of the last
inventory and there have been no writes to the vault since the last
inventory." Two documented conditions, not one. `types.DescribeVaultOutput`
confirms `NumberOfArchives`/`LastInventoryDate` are themselves as-of-last-
inventory values ("The number of archives in the vault as of the last
inventory date... This field will return null if an inventory has not yet
run on the vault").

The filed issue's own framing was half wrong: it claimed "a vault whose
archives were all uploaded since the last inventory can be deleted." The
doc's second clause -- no writes since last inventory -- says the opposite:
such a vault is still blocked, because uploading after the inventory is
itself a write since that inventory. The doc wins; see
`TestDeleteVault_InventorySemantics/write_after_zero_archive_inventory_still_empty_now`.

Was checking `len(v.Archives) > 0` (the live archive count) instead of
either documented condition. This backend already tracked
`Vault.LastInventoryDate`, set when an inventory-retrieval job is initiated
(`jobs.go`'s `applyJobTypeSpecifics`) -- an existing, already-established
trigger point, not a new invented schedule. Reused that same point to add
two fields: `NumberOfArchivesAtLastInventory` (snapshots the live count
there) and `WriteSinceLastInventory` (cleared there, set on any archive
add/remove -- `UploadArchive`/`DeleteArchive`/`CompleteMultipartUpload`).
`DeleteVault` now rejects iff `NumberOfArchivesAtLastInventory > 0 ||
WriteSinceLastInventory`. A vault that has never had an inventory run and
never had a write defaults both fields to zero/false, matching
`NumberOfArchives`'s documented null-at-creation baseline, so an untouched
fresh vault stays trivially deletable.

No fabricated timer or periodic refresh was added: the emulator has no
background inventory process and none was invented; `LastInventoryDate`
only advances on an explicit `InitiateJob(inventory-retrieval)` call, same
as before this fix.

Separate, disclosed, NOT fixed this pass: `DescribeVault` reports the LIVE
`NumberOfArchives`/`SizeInBytes` where AWS documents as-of-inventory values
-- see the `DescribeVault` ops row. Filed separately; not the same one-line
change as `DeleteVault`'s fix (that used a new field purpose-built for the
delete check; `DescribeVault` would need its own as-of-inventory
size/count/null handling).

Pre-existing tests corrected (2, strengthened not weakened):
`TestDeleteVault_RejectsNonEmpty` and `TestDeleteVault_NotEmpty_Returns409`
set up their non-empty vault via `AddVaultInternal(&Vault{VaultName: ...})`
+ `AddArchiveInternal(...)`, which populates `v.Archives` directly without
touching the new inventory-tracking fields -- under the new semantics that
vault would be (wrongly) deletable. Both now also set
`NumberOfArchivesAtLastInventory: 1` on the `AddVaultInternal` vault so they
keep testing genuine non-empty-vault rejection under the new model, still
asserting the same 409.

New regression tests, all driven through the HTTP handler
(`handler_vaults_test.go`'s `TestDeleteVault_InventorySemantics`, 3
subtests): `archives_at_last_inventory_but_currently_empty` (upload, run
inventory, delete the archive, vault now empty live but still 409) and
`write_after_zero_archive_inventory_still_empty_now` (inventory while
empty, upload+delete after, still 409 despite zero archives at inventory)
both FAIL against unmodified code (expected 409, got 204 -- confirmed by
reverting the `vaults.go` check alone, running, and restoring);
`zero_archives_at_inventory_no_writes_since` (204, and a second untouched
vault is confirmed present) passes both before and after since it never
exercised the bug. Rejected-path subtests also assert the declared error
code (`ConflictException`, from the existing `ErrVaultNotEmpty`) and that
`DescribeVault` still returns the vault afterward.

Error code: no new error was introduced. `ErrVaultNotEmpty` already maps to
`ConflictException` (`errors.go`/`handler.go`'s `writeBackendError`), kept
as-is. Raw `deserializeOpErrorDeleteVault` extraction (for reference; it
has no `ConflictException` case, which is expected -- that switch only
picks a typed Go error struct, an unmatched code still round-trips as
`smithy.GenericAPIError` with the real `Code`/`Message`):
`UnknownError`, `InvalidParameterValueException`,
`MissingParameterValueException`, `NoLongerSupportedException`,
`ResourceNotFoundException`, `ServiceUnavailableException`.

Persisted-field guard: `Vault` gained two fields
(`NumberOfArchivesAtLastInventory`, `WriteSinceLastInventory`). Ran
`go test ./pkgs/persistence/... -run TestSnapshotVersionGuard` read-only (no
`-update`, no version bump): it reports "glacier: backendSnapshot fields
changed without a version bump; golden is out of date, run with -update to
refresh it (this is bookkeeping, not a version-bump case: every old field is
still present unchanged, so the diff is additive only and needs no bump)" --
purely additive, golden refresh only, left for the golden owner.

Gates: `go build ./services/glacier/...` clean; `go test -race -count=1
./services/glacier/...` PASS; `golangci-lint run services/glacier/...` 0
issues (one `fieldalignment` finding in the new test's table struct, fixed
by hand-reordering fields, no `-fix` used).

**Addendum**: the tests above reached the two new `Vault` fields only via
`AddVaultInternal`/direct backend calls, leaving `jobs.go:217-218` (the
`InitiateJob(inventory-retrieval)` snapshot/reset that actually makes the
model coherent) uncovered -- either line could be deleted or hard-coded and
the suite stayed green. Closed by two more HTTP-driven tests:
`TestDeleteVault_InventoryRefresh_ClearsWriteSinceFlag` (pins `jobs.go:218`:
write, delete, DeleteVault rejected; InitiateJob(inventory-retrieval);
DeleteVault now succeeds) and
`TestDeleteVault_InventoryRefresh_SnapshotsArchiveCount` (pins
`jobs.go:217`: upload, InitiateJob(inventory-retrieval), DeleteVault
rejected -- asserted immediately, before any further write, so only the
snapshotted count can be driving it). Each neutered line was hand-reverted
individually, confirmed to fail only its matching new test (all others
green), and restored.
