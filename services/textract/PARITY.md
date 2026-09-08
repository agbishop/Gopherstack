---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: textract
sdk_module: aws-sdk-go-v2/service/textract@v1.43.4   # bumped from v1.41.0 pin; AdaptersConfig/HumanLoopConfig field-diffed this pass
last_audit_commit: a8a59e4273e                        # HEAD as of the 2026-08-20 pass; see provenance note below
last_audit_date: 2026-08-20
overall: A            # 2026-08-20: wrapper-key/nested-shape sweep. Two real pattern-(a) fixes
                      # (AnalyzeIDDetections.Geometry fabricated field removed; Extraction.IdentityDocument
                      # missing field added), both latent/never-emitted in current mock data -- see the
                      # dated section below. Block/enums/sync-vs-async pairs/adapters/lending/expense/tags
                      # families re-verified clean. Grade held at A. last_audit_commit provenance for the
                      # PRIOR entry (8c56f4eb9) found unreliable -- see provenance verdict below.
                      # 2026-08-07 (gopherstack-n1bo): implemented AdaptersConfig validation (real Adapter/AdapterVersion
                      # state) and HumanLoopConfig required-field validation for AnalyzeDocument/StartDocumentAnalysis,
                      # closing the gap left by the 2026-07-24 pass below. HumanLoopActivationOutput's actual
                      # activation decision reclassified into structural_gaps (see there for why). Grade held at A.
                      # 2026-07-24: full field-diff pass this sweep found and fixed 9 genuine wire/error-code bugs
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  DetectDocumentText: {wire: ok, errors: ok, state: ok, persist: n/a, note: "synchronous, deterministic mock Blocks. 2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced when S3 is wired -- see Notes."}
  AnalyzeDocument: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FeatureTypes + QueriesConfig validated; errors FIXED to InvalidParameterException (real SDK has no ValidationException case for this op). 2026-08-07 (gopherstack-n1bo): AdaptersConfig.Adapters now validated against real Adapter/AdapterVersion backend state (InvalidParameterException for an unknown adapter or version -- NOT ResourceNotFoundException, the trap: this op's real error set has no such case, unlike GetAdapter/UpdateAdapter/etc.); HumanLoopConfig's two required members validated; HumanLoopActivationOutput wired into the response shape but always omitted -- see structural_gaps for why activation itself can't be computed. 2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced when S3 is wired -- see Notes."}
  AnalyzeExpense: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED — ExpenseField.Currency/Type now ExpenseCurrency/ExpenseType, not ExpenseDetection. 2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced when S3 is wired -- see Notes."}
  AnalyzeID: {wire: ok, errors: ok, state: ok, persist: n/a, note: "2026-08-20: FIXED — AnalyzeIDDetections.Geometry was a fabricated field (real types.AnalyzeIDDetections has only Text/Confidence/NormalizedValue). Never actually populated (always nil, omitempty), so it never leaked on the wire in practice; removed for wire-shape correctness. 2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced per DocumentPages entry when S3 is wired -- see Notes."}
  StartDocumentTextDetection: {wire: ok, errors: ok, state: ok, persist: ok, note: "ClientRequestToken idempotency wired; errors FIXED to InvalidParameterException. 2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced when S3 is wired -- see Notes."}
  GetDocumentTextDetection: {wire: ok, errors: ok, state: ok, persist: ok, note: "NextToken pagination via paginateBlocks; errors FIXED to InvalidParameterException"}
  StartDocumentAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "ClientRequestToken idempotency wired; errors FIXED to InvalidParameterException. 2026-08-07 (gopherstack-n1bo): AdaptersConfig (this op's only adapter/human-loop-related field -- StartDocumentAnalysisInput has no HumanLoopConfig member) now validated the same way as AnalyzeDocument's. 2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced when S3 is wired -- see Notes."}
  GetDocumentAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "errors FIXED to InvalidParameterException"}
  StartExpenseAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "errors FIXED to InvalidParameterException. 2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced when S3 is wired -- see Notes."}
  GetExpenseAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — MaxResults/NextToken were accepted but silently ignored (ExpenseDocuments never paginated, no NextToken in response); now paginates via pkgs/page. errors FIXED to InvalidParameterException"}
  StartLendingAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "errors FIXED to InvalidParameterException. 2026-09-06 (gopherstack-eshx): InvalidS3ObjectException now enforced when S3 is wired -- see Notes."}
  GetLendingAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — same missing-pagination gap as GetExpenseAnalysis, now paginates Results via pkgs/page. errors FIXED to InvalidParameterException"}
  GetLendingAnalysisSummary: {wire: ok, errors: ok, state: ok, persist: ok, note: "errors FIXED to InvalidParameterException"}
  CreateAdapter: {wire: ok, errors: ok, state: ok, persist: ok, note: ClientRequestToken dedup; FeatureTypes restricted to FORMS/QUERIES}
  GetAdapter: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — CreationTime was an RFC3339 string (\"2006-01-02T15:04:05Z\"); real SDK deserializer requires epoch-seconds JSON number (unixTimestamp format). Not-found error FIXED from InvalidParameterException to ResourceNotFoundException"}
  UpdateAdapter: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — response had an invented Tags field (real UpdateAdapterOutput has no Tags member); CreationTime FIXED to epoch-seconds. Not-found error FIXED to ResourceNotFoundException"}
  ListAdapters: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — input was an empty struct silently dropping AfterCreationTime/BeforeCreationTime/MaxResults/NextToken; now filters + paginates via pkgs/page and echoes NextToken. CreationTime FIXED to epoch-seconds"}
  DeleteAdapter: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascades to delete all adapter versions; not-found error FIXED to ResourceNotFoundException"}
  CreateAdapterVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "CREATION_IN_PROGRESS -> ACTIVE lifecycle; adapter-not-found error FIXED to ResourceNotFoundException. 2026-09-06 (gopherstack-u416): InvalidS3ObjectException now enforced against DatasetConfig.ManifestS3Object when S3 is wired -- missed by gopherstack-eshx's sweep of the 8 request-side ops; see Notes."}
  GetAdapterVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — EvaluationMetrics was a single flat struct; real GetAdapterVersionOutput.EvaluationMetrics is a []AdapterVersionEvaluationMetric list (Baseline + AdapterVersion sub-scores per FeatureType). CreationTime FIXED to epoch-seconds. Not-found error FIXED to ResourceNotFoundException"}
  ListAdapterVersions: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED — response had an invented top-level AdapterId (real ListAdapterVersionsOutput has none); each AdapterVersionOverview entry was missing its own AdapterId (now added). Input FIXED to accept AfterCreationTime/BeforeCreationTime/MaxResults/NextToken (previously AdapterId-only, silently dropping the rest) and paginates via pkgs/page. CreationTime FIXED to epoch-seconds. Not-found error FIXED to ResourceNotFoundException. gopherstack-2wvq (2026-08-21): ListAdapterVersionsInput declares no required members at all (textract@v1.43.4 api_op_ListAdapterVersions.go: AdapterId is a plain optional filter, 'A string containing a unique ID for the adapter to match for when listing adapter versions') -- the handler previously rejected any request without AdapterId with a 400, making the documented all-adapters listing unreachable. Backend now lists across every adapter in the region when AdapterId is empty, reusing the existing adaptersByRegion + adapterVersionsByAdapter secondary indexes (no new index needed, cheaper than this issue's original sizing of 'a real cross-adapter index'). Sorted by AdapterID then AdapterVersion -- the SDK documents no ordering -- so pkgs/page's offset tokens stay stable across the merged, freshly-rebuilt set; verified with a small-MaxResults pagination test asserting the paged union equals the whole set with no duplicates. Checked CreateAdapterVersion/UpdateAdapter for an enabling accept-and-drop the other direction: found none -- both already store/require everything this newly-reachable path reads (AdapterId, FeatureTypes, Status, CreationTime). Response shape (AdapterVersionOverview) was already correct on the newly-reachable path, nothing to fix there."}
  DeleteAdapterVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "not-found error FIXED to ResourceNotFoundException"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "resolves adapter or adapter-version ARN; not-found error FIXED to ResourceNotFoundException"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "not-found error FIXED to ResourceNotFoundException"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "not-found error FIXED to ResourceNotFoundException"}
# Families audited as a group (when per-op is impractical):
families:
  async-job-lifecycle: {status: ok, note: "IN_PROGRESS -> SUCCEEDED transition via runDelayed goroutine tracked by sync.WaitGroup, cancelled/awaited on Handler.Shutdown; verified for DocumentAnalysis, TextDetection, ExpenseAnalysis, LendingAnalysis, AdapterVersion creation"}
  adapter-arn-resolution: {status: ok, note: "TagResource/UntagResource/ListTagsForResource resolve either a bare adapter ID, an adapter ARN, or an adapter-version ARN (checks /version/ suffix first)"}
  wire-shapes: {status: ok, note: "Block, ExpenseDocument/ExpenseField/ExpenseCurrency/ExpenseType/ExpenseGroupProperty, IdentityDocument/IdentityDocumentField, LendingResult/LendingDocument/LendingField/Prediction/Extraction, AdapterVersionEvaluationMetric/EvaluationMetric, DocumentMetadata field names cross-checked field-by-field against aws-sdk-go-v2/service/textract@v1.41.0/types this pass (types.go, api_op_*.go, and deserializers.go's per-op error switches read directly from the downloaded module in GOMODCACHE)"}
  error-codes: {status: ok, note: "Every op's real deserializeOpError<Op> switch in deserializers.go enumerated to confirm exact declared error set. Two systemic bugs fixed: (1) adapter/adapter-version/tag not-found used InvalidParameterException, real code is ResourceNotFoundException; (2) generic parameter-validation failures on the 13 non-adapter ops (AnalyzeDocument/AnalyzeExpense/AnalyzeID/DetectDocumentText/Get*/Start*) used ValidationException, but those ops' real deserializers have no ValidationException case at all -- only the adapter-management + Tag*/ListTagsForResource ops declare it. handler.go's handleError is now operation-aware (opsWithoutValidationException lookup) to route correctly per op."}
  timestamps: {status: ok, note: "FIXED — Adapter/AdapterVersion CreationTime was serialized as an RFC3339 string across GetAdapter/UpdateAdapter/ListAdapters/GetAdapterVersion/ListAdapterVersions; the awsjson1.1 protocol's unixTimestamp format requires epoch-seconds JSON numbers (confirmed via deserializers.go's smithytime.ParseEpochSeconds(f64) call sites). Now uses pkgs/awstime.Epoch throughout. DocumentJob/ExpenseJob/LendingJob.CreationTime were never wire-exposed (internal only), so unaffected."}
  pagination: {status: ok, note: "FIXED — GetExpenseAnalysis/GetLendingAnalysis accepted MaxResults/NextToken but silently ignored them (no NextToken in response, no truncation); ListAdapters/ListAdapterVersions accepted no pagination fields at all. All four now paginate via pkgs/page.New and echo NextToken, matching GetDocumentAnalysis/GetDocumentTextDetection's pre-existing PaginateBlocks pattern."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "Geometry.RotationAngle (types.Geometry) was added to the Geometry struct (*float64, omitempty) for wire-shape completeness but nothing in synthetic_blocks.go ever populates it -- always nil/omitted. Harmless (matches real AWS behavior when a document has no detected rotation), flagging only so a future auditor doesn't assume it's untested/forgotten."
  - "2026-08-20: Extraction.IdentityDocument (types.Extraction, aws-sdk-go-v2/service/textract@v1.43.4/types/types.go:613-625) was missing entirely -- gopherstack's Extraction only had LendingDocument/ExpenseDocument. Added the field (*IdentityDocument, omitempty) for wire-shape completeness, but nothing in lending_analysis.go's syntheticLendingResults() ever classifies a page as an identity document, so it stays nil/omitted, matching the RotationAngle precedent above. Real AnalyzeLending can return this when a lending package includes an ID page; gopherstack's mock lending flow always returns a fixed PAYSTUB/LendingDocument result."
structural_gaps:
  - "AnalyzeDocument/StartDocumentAnalysis's HumanLoopConfig-triggered activation decision (AnalyzeDocumentOutput.HumanLoopActivationOutput) cannot be computed: real AWS evaluates the referenced FlowDefinition's HumanLoopActivationConditionsConfig, a JsonPath-based rules engine over per-block confidence scores that lives in SageMaker Augmented AI, a service gopherstack does not model the condition-evaluation semantics of anywhere (SageMaker's own FlowDefinition resource is tracked, but only as a CRUD record, not as an executable condition set). Approximating activation (e.g. a fixed probability, or a made-up confidence threshold) would fabricate a business decision, not derive one from held state -- exactly the failure mode this campaign removes. What IS buildable and built this pass (gopherstack-n1bo): HumanLoopConfig's two required members (FlowDefinitionArn, HumanLoopName) are validated, InvalidParameterException on either missing; HumanLoopActivationOutput is correctly omitted (nil) rather than fabricated. (bd: gopherstack-n1bo)"
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "Full byte-for-byte Block field audit beyond the fields cross-checked this pass (BlockType, ColumnIndex, ColumnSpan, Confidence, EntityTypes, Geometry, Id, Page, Query, Relationships, RowIndex, RowSpan, SelectionStatus, Text, TextType) — spot-checked per BlockType variant (WORD/LINE/PAGE/TABLE/CELL/KEY_VALUE_SET/QUERY/QUERY_RESULT/SIGNATURE/LAYOUT_*), not exhaustively fuzzed."
leaks: {status: clean, note: "runDelayed goroutines are tracked by InMemoryBackend.wg and bound to svcCtx; Handler.Shutdown cancels + waits. Verified via existing TestInMemoryBackend_Shutdown table (all 4 job families x zero-delay/shutdown-cancels cases). No new goroutines, tickers, or maps introduced this pass; pagination is a pure function over already-owned clones (pkgs/page.New), no additional state to leak."}
---

## Notes

Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: Textract.<Op>` dispatch. All 25
ops in the real SDK are routed and reachable (`GetSupportedOperations()` / `buildOps()` match
1:1). `RouteMatcher` is a simple header-prefix check; nothing to trap there.

### Bugs fixed this pass (2026-07-24)

This pass field-diffed every gopherstack type and every op's declared error set against the
real `aws-sdk-go-v2/service/textract@v1.41.0` module (downloaded into GOMODCACHE and read
directly: `types/types.go`, every `api_op_*.go` Input/Output struct, and every
`awsAwsjson11_deserializeOpError<Op>` switch in `deserializers.go`). This is a materially
deeper audit than the previous pass's spot-checks and found 9 distinct real bugs:

1. **`Block.ColumnHeader` was a fabricated field** with no counterpart in the real SDK's
   `types.Block` (which has no such member at all — column-header status is conveyed purely
   via `EntityTypes` containing `"COLUMN_HEADER"`, which `synthetic_blocks.go` was *also*
   already setting). Deleted the field from `models.go` and its two set-sites in
   `synthetic_blocks.go`'s `buildTablesBlocks`.

2. **`LendingField` had a fabricated shape.** gopherstack modeled `Type *LendingDetection`,
   `ValueDetection *LendingDetection` (singular), and an invented `PageNumber int`. The real
   `types.LendingField` is `{KeyDetection *LendingDetection, Type *string, ValueDetections
   []LendingDetection}` — `Type` is a bare string, `ValueDetections` is plural, and there is
   no `PageNumber` member at all. Fixed the struct and `syntheticLendingResults()`.

3. **`PageClassification.PageType`/`PageNumber` used the wrong element type.**
   gopherstack used `[]LendingDetection` (`Text`/`Geometry`/`SelectionStatus`/`Confidence`);
   the real `types.PageClassification` uses `[]Prediction` (`Value`/`Confidence` only — no
   `Text`, no `Geometry`). The classified value was therefore under the wrong JSON key
   (`Text` instead of `Value`). Added a `Prediction` type and fixed both fields.

4. **`ExpenseField.Currency`/`Type` used the wrong element type.** gopherstack used
   `*ExpenseDetection` (`Text`/`Geometry`/`Confidence`) for both; the real `types.ExpenseField`
   uses `*ExpenseCurrency` (`Code`/`Confidence`, no `Text`/`Geometry`) for `Currency` and
   `*ExpenseType` (`Text`/`Confidence`, no `Geometry`) for `Type`. Added both types and fixed
   `ExpenseField` + `syntheticExpenseDocument()`.

5. **`AdapterVersion.EvaluationMetrics` was a single flat struct**; the real
   `GetAdapterVersionOutput.EvaluationMetrics` is `[]AdapterVersionEvaluationMetric`, one
   entry per adapter `FeatureType`, each carrying independent `Baseline` and `AdapterVersion`
   sub-scores (`EvaluationMetric{F1Score,Precision,Recall}`). Renamed the old struct to
   `EvaluationMetric` (matching the real sub-score type name), added
   `AdapterVersionEvaluationMetric`, and `adapter_versions.go` now builds one entry per
   `adapter.FeatureTypes` element via a new `buildEvaluationMetrics` helper.

6. **Adapter/AdapterVersion `CreationTime` was serialized as an RFC3339 string**
   (`.Format("2006-01-02T15:04:05Z")`) across `GetAdapter`, `UpdateAdapter`,
   `ListAdapters`(`adapterSummary`), `GetAdapterVersion`, and
   `ListAdapterVersions`(`adapterVersionSummary`). The awsjson1.1 protocol's `unixTimestamp`
   format requires an epoch-seconds JSON number — confirmed directly against
   `deserializers.go`'s `smithytime.ParseEpochSeconds(f64)` call sites for every one of these
   fields. A real Textract SDK client would reject every one of these five responses with
   "expected Timestamp to be a JSON Number, got string instead". Fixed via
   `pkgs/awstime.Epoch`, matching the bug class flagged in this campaign's known traps.

7. **Adapter/adapter-version/tag not-found errors used the wrong error code.**
   `ErrAdapterNotFound`/`ErrAdapterVersionNotFound` were wired to `InvalidParameterException`;
   every real op that can return a not-found for these resources
   (`GetAdapter`/`UpdateAdapter`/`DeleteAdapter`/`DeleteAdapterVersion`/`GetAdapterVersion`/
   `CreateAdapterVersion`/`ListAdapterVersions`/`TagResource`/`UntagResource`/
   `ListTagsForResource`) declares `ResourceNotFoundException` in its deserializer switch, not
   `InvalidParameterException`. Fixed in `errors.go` and `handler.go`.

8. **Generic parameter-validation errors used the wrong error code on 13 ops.**
   gopherstack's `errInvalidRequest` sentinel unconditionally mapped to `ValidationException`
   for every op. But `AnalyzeDocument`, `AnalyzeExpense`, `AnalyzeID`, `DetectDocumentText`,
   and every `Get*`/`Start*` job op have **no `ValidationException` case at all** in their real
   deserializer switches — only `InvalidParameterException`. Only the adapter-management ops
   (`CreateAdapter`/`CreateAdapterVersion`/`Delete*`/`Get`/`List Adapter*`/`UpdateAdapter`) and
   the Tag ops declare `ValidationException`. `handler.go`'s `handleError` is now
   operation-aware (`opsWithoutValidationException` lookup keyed by the dispatch action name,
   which `service.HandleTarget` already threads through) so each op returns the error code its
   real deserializer actually recognizes.

9. **`UpdateAdapterOutput` had an invented `Tags` field**, and **`ListAdapterVersionsOutput`
   had an invented top-level `AdapterId` field while missing the required per-entry
   `AdapterId` on each `AdapterVersionOverview`.** Real `UpdateAdapterOutput` has no `Tags`
   member at all (unlike `GetAdapterOutput`, which does). Real `ListAdapterVersionsOutput` is
   just `{AdapterVersions, NextToken}` — no top-level `AdapterId` — while each
   `AdapterVersionOverview` entry does carry its own `AdapterId`, which gopherstack's
   `adapterVersionSummary` omitted. Fixed both.

Additionally, two previously-silent pagination gaps were closed: `GetExpenseAnalysis` and
`GetLendingAnalysis` both accept `MaxResults`/`NextToken` in their real inputs and echo
`NextToken` in their outputs, but gopherstack's handlers parsed these fields and then never
used them — `ExpenseDocuments`/`Results` were always returned in full with no `NextToken`.
`ListAdapters` and `ListAdapterVersions` didn't even parse `MaxResults`/`NextToken`/
`AfterCreationTime`/`BeforeCreationTime` at all. All four now paginate via `pkgs/page.New`
(the catalog's preferred generic pagination helper for new call sites — the pre-existing
`Blocks` pagination in `synthetic_blocks.go`'s `paginateBlocks`/`PaginateBlocks` was left
alone since it already worked and touching it wasn't necessary for this fix) and
`ListAdapters`/`ListAdapterVersions` additionally now filter on `AfterCreationTime`/
`BeforeCreationTime`.

`textractSnapshotVersion` was bumped 1 → 2: the `AdapterVersion.EvaluationMetrics`,
`LendingField`, and `PageClassification` shape changes above are all persisted-DTO shape
changes (these types round-trip through `backendSnapshot` via `DocumentJob`/`LendingJob`/
`AdapterVersion`), so a v1 snapshot must be discarded rather than partially decoded against
the v2 shape.

All 9 fixes are covered by new table-driven tests (kept in their existing per-op-family test
files rather than new files, per the "ONE test file per source/op-family" convention already
established): `handler_document_analysis_test.go` (ColumnHeader deletion, InvalidParameterException
error codes), `handler_lending_analysis_test.go` (LendingField/PageClassification wire shape,
GetLendingAnalysis pagination), `handler_expense_analysis_test.go` (ExpenseCurrency/ExpenseType
wire shape, GetExpenseAnalysis pagination), `handler_adapters_test.go` (CreationTime
epoch-seconds, ListAdapters pagination, UpdateAdapter Tags absence),
`handler_adapter_versions_test.go` (EvaluationMetrics list shape rewritten, CreationTime
epoch-seconds, ListAdapterVersions pagination + per-entry AdapterId, ResourceNotFoundException
codes), and `handler_tags_test.go` (ResourceNotFoundException for Tag/Untag/ListTagsForResource).
Five pre-existing tests that asserted the old (wrong) error codes were corrected in place
(`TestHandler_UpdateAdapter_NotFound`, `TestHandler_DeleteAdapterVersion_GetReturnsErrorAfterDelete`,
`TestHandler_HandleError_AdapterNotFound`, and two subtests of
`TestHandler_ErrorEnvelope_TypeAndMessage`), and `TestHandler_AdapterVersion_EvaluationMetrics`
was rewritten for the new list shape.

### 2026-08-20 pass (wrapper-key / nested-shape sweep)

Full re-derivation of protocol (JSON-RPC 1.1, `awsAwsjson11_*`, confirmed via
`X-Amz-Target: Textract.<Op>` in `serializers.go` and `deserializeOpDocument<Op>Output`
both defined AND called for every op -- the restjson flat-decode false-positive trap does
not apply to awsjson1.1), all 25 ops enumerated against
`aws-sdk-go-v2/service/textract@v1.43.4`'s `api_op_*.go` files (1:1 match, nothing added
upstream since the last pin), `Block`'s full field list and every emitted `BlockType`/
`EntityType`/`RelationshipType`/`SelectionStatus`/`TextType` enum value checked against
`types/enums.go`, and every sync/async Output struct pair (`AnalyzeDocument` vs
`GetDocumentAnalysis`, `DetectDocumentText` vs `GetDocumentTextDetection`, `AnalyzeExpense`
vs `GetExpenseAnalysis`, `StartLendingAnalysis`'s `GetLendingAnalysis`/
`GetLendingAnalysisSummary`) diffed field-by-field. Two real findings, both pattern (a)
(member generalized from/missing relative to a sibling type), neither with an observable
wire symptom in gopherstack's current mock data (both are always-nil fields, so the fix and
its hand-revert produce byte-identical wire output except in a synthetic test that force-
populates the field):

1. **`AnalyzeIDDetections.Geometry` was fabricated.** The real
   `types.AnalyzeIDDetections` (`types/types.go:135-150`) has only `Text` (required),
   `Confidence`, `NormalizedValue` -- no `Geometry`, unlike the sibling
   `LendingDetection`/`ExpenseDetection`/`SignatureDetection` types which do carry
   `Geometry`. `id_analysis.go`'s `syntheticIDDocument` never set it (always nil,
   `omitempty` hides it), so this never actually leaked onto the wire, but the field
   itself had no basis in the real SDK and was a landmine for future population. Removed
   from `models.go`. New test: `TestHandler_AnalyzeID_NoFabricatedGeometry`
   (`handler_id_analysis_test.go`) asserts via raw JSON body (not the typed SDK client,
   which has no case for an unrecognized key and would silently ignore it) that no
   `IdentityDocumentFields[].Type`/`ValueDetection` object carries a `Geometry` key.
   Hand-revert: re-added the field to the struct AND force-populated it in
   `id_analysis.go` (a real client can't observe an unpopulated fabricated field, so the
   struct-only revert alone wouldn't reproduce a symptom) -- test failed with the leaked
   `"Geometry":{"BoundingBox":...}` key exactly as predicted; reverted, `models.go`
   confirmed byte-identical to the pre-revert state via diff.

2. **`Extraction.IdentityDocument` was missing** (found incidentally while diffing
   `Extraction`'s field list, not from a Layer-3 hunt). Real `types.Extraction`
   (`types/types.go:613-625`) has `ExpenseDocument`, `IdentityDocument`, `LendingDocument`;
   gopherstack's `Extraction` only had the first and third. Added `IdentityDocument
   *IdentityDocument` for wire-shape completeness (`models.go`). `lending_analysis.go`'s
   `syntheticLendingResults()` never classifies a page as an identity document, so like
   `Geometry.RotationAngle` above, this stays nil/omitted in practice -- documented as a
   gap, not claimed as a behavioral fix.

**`last_audit_commit` provenance verdict: the prior citation (`8c56f4eb9`, dated
2026-08-07, matching the prior `last_audit_date` exactly) is misleading.** The PARITY.md
content it labeled "2026-08-07 (gopherstack-n1bo)" -- including the AdaptersConfig/
HumanLoopConfig work described there -- was actually committed in `d39bf33e4` ("Chore/parity
upgrade (#2414)"), dated **2026-08-11**, four days later. `git merge-base 8c56f4eb9
d39bf33e4` resolves to `e88712a92` (d39bf33e4's own parent) -- `8c56f4eb9` sits on a sibling
branch (`chore/parity-upgrade`, an unrelated dynamodb/rds fix) that is **not an ancestor**
of the commit that actually produced the audited state. The date match was coincidental,
not proof of provenance; ancestry, not just date proximity, is the reliable check. This
pass's own citation (`a8a59e4273e`) is today's actual HEAD, avoiding the same trap.

**SDK version check**: `go.mod` pins `v1.43.4`; the manifest header already said
`v1.43.4` correctly. The **prose** under the 2026-07-24 section still says
`aws-sdk-go-v2/service/textract@v1.41.0` -- this is accurate as *historical narration* of
what that pass actually read at the time (before the later `v1.41.0 -> v1.43.4` bump
recorded in the header's own comment), not a live claim about the current pin, so it is
not a manifest inconsistency.

**"FIXED" claim re-derivation**: spot-re-verified bug #8 (op-aware `InvalidParameterException`
vs `ValidationException`) against `GetDocumentAnalysis`'s live `deserializeOpError` switch
this pass (`deserializers.go:1433-1497`) -- confirmed `InvalidJobIdException`,
`InvalidParameterException` present, no `ValidationException` case, matching the claim.
No other "FIXED" claims were found to fail re-derivation in the portions of the manifest
covered this pass (see the report's coverage disclosure for what was and wasn't
re-verified).

### Traps for the next auditor

- `LendingJob.Warnings` / `ExpenseJob.Warnings` / `DocumentJob.Warnings` are
  modeled, cloned, and persisted, but nothing in this backend currently
  populates them with real content (always nil in practice, `omitempty`
  hides them). This is intentional latent extensibility, not a bug in
  itself.
- `regionFromARN`/`resolveARNToAdapter`/`resolveARNToAdapterVersion` do a
  manual ARN parse rather than using `pkgs/arn`'s parser for the *read* path
  (write path — `buildAdapterARN`/`buildAdapterVersionARN` — does use
  `pkgs/arn.Build`). `pkgs/arn` still has no ARN-parsing helper as of this
  audit, only building, so the hand-rolled `lastIndex`/`contains` parse is
  not a reuse violation.
- `cloneAdapter`/`cloneExpenseJob`/`cloneLendingJob`/`cloneJob`/`cloneAdapterVersion`
  are shallow `cp := *j` plus explicit deep-copies of slice/map fields only;
  pointer fields like `OutputConfig`/`NotificationChannel`/`DatasetConfig` are
  intentionally left aliased since nothing mutates them in place after
  Start/Create — don't "fix" this into a full deep clone without checking for
  actual mutation-after-share bugs first. `cloneAdapterVersion` now also
  deep-copies the new `EvaluationMetrics []AdapterVersionEvaluationMetric`
  slice (element-wise `copy`, not a deep copy of the pointer fields inside
  each element, matching the existing shallow-clone convention).
- If a future pass adds `AdaptersConfig`/`HumanLoopConfig` support to
  `AnalyzeDocument`/`StartDocumentAnalysis` (see `gaps` above), remember
  `AnalyzeDocument`'s real error set has `HumanLoopQuotaExceededException`
  but *no* `ResourceNotFoundException` — an invalid `AdaptersConfig` adapter
  reference should almost certainly surface as `InvalidParameterException`,
  not `ResourceNotFoundException`, to stay consistent with bug #8 above.
- `opsWithoutValidationException` in `handler.go` is keyed by the exact
  dispatch action string (`X-Amz-Target` suffix), not by Go sentinel error
  type — if a new op is added, its entry (or lack of one) must be verified
  against that op's real `deserializeOpError<Op>` switch, not assumed from
  a sibling op.

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

**Bug found and fixed:** `paginateBlocks` (`synthetic_blocks.go`, exported as
`PaginateBlocks` — backs `GetDocumentAnalysis` and `GetDocumentTextDetection`, 2 operations)
decoded `nextToken` to an offset but only accepted it when `n >= 0 && n < len(blocks)`; any
out-of-range `n` (a token exactly at or past the current block count — the value this same
helper would itself emit for an exhausted final page, or simply a stale/tampered token) left
`offset` at its zero-value default. The existing `if offset >= len(blocks) { return empty }`
guard immediately below can then never fire, since `offset` was already reset to a
now-in-range 0 — so pagination silently restarted at block one instead of returning empty.
Fixed by dropping the `n < len(blocks)` half of the inner validation and letting the outer
guard do its job — the same fix (and the same root mistake, independently made) as
`services/dax`'s `paginateParameters`/`DescribeEvents` this same pass.

Proof: `TestPaginateBlocks_CursorPastEndDoesNotRestart` /
`TestPaginateBlocks_CursorExactlyAtEndDoesNotRestart`
(pagination_arithmetic_internal_test.go, unit, calls `paginateBlocks` directly) and
`TestGetDocumentTextDetection_SDKRoundTrip_CursorAtEndDoesNotRestart`
(pagination_sdk_roundtrip_test.go, real `aws-sdk-go-v2/service/textract` client) both fail
pre-fix and pass post-fix.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/textract/...`).

## 2026-08-31: error-envelope sweep (gopherstack-uox6, errtargetaudit)

`errtargetaudit -dir textract` reported 4 class-A findings, all
`code=InvalidJobIdException mechanism=sentinel reference`, ops
`[StartDocumentAnalysis StartDocumentTextDetection StartExpenseAnalysis
StartLendingAnalysis]`. This service's error matching is
`awsAwsjson11_deserializeOpError<Op>` (per-op switch in
`deserializers.go`). Verified each Start op's own switch
(textract@v1.43.4): all four declare exactly `[AccessDeniedException,
BadDocumentException, DocumentTooLargeException,
IdempotentParameterMismatchException, InternalServerError,
InvalidKMSKeyException, InvalidParameterException,
InvalidS3ObjectException* (StartDocumentAnalysis only), LimitExceededException,
ProvisionedThroughputExceededException, ThrottlingException,
UnsupportedDocumentException]` — no `InvalidJobIdException`; only the
Get* ops declare that. All 4 real, 0 false positives.

**Root cause (1 shape, all 4 ops)**: each `StartXWithOptions` backend
method (`document_analysis.go`, `document_detection.go`,
`expense_analysis.go`, `lending_analysis.go`) has a post-write readback
guard — `if result == nil { return ..., ErrJobNotFound }` — for the case
where the job just stored under `b.mu.Lock` is gone by the time a second,
independent `b.mu.RLock` reads it back (`trimJobsIfNeeded` evicting it,
e.g. `maxJobs==0`). This is a server-side invariant violation (a race, not
a client-supplied bad job ID), traced to gopherstack-0ho6's original nil-
deref fix, which picked `ErrJobNotFound` as "the reasonable existing
sentinel" without checking it against these ops' own declared set. Fixed
by introducing `errJobEvictedBeforeReadback` (`errors.go`), which
deliberately matches none of `handleError`'s `errors.Is` cases and so
falls through to the pre-existing `default: InternalServerError` branch —
already correct and declared by all four ops — rather than inventing a new
mapping.

4 existing table tests
(`TestInMemoryBackend_Start{DocumentAnalysis,DocumentTextDetection,
Expense Analysis,LendingAnalysis}WithOptions_TrimmedBeforeReadback`) had a
`zero_cap` case asserting `require.ErrorIs(t, err, awserr.ErrNotFound)`;
corrected to `require.Error` + `require.NotErrorIs(t, err,
awserr.ErrNotFound)` (2 assertions instead of 1 — the original single
`ErrorIs` call implicitly also required `err != nil`, which a bare
`NotErrorIs` would not have caught on its own). New
`TestStartOps_TrimmedBeforeReadback_RealClient`
(`errtargetaudit_start_ops_test.go`, 4 subtests) drives the real
`aws-sdk-go-v2/service/textract` client with `NewInMemoryBackendWithCap(0)`
and asserts `errors.As` unwraps to `*types.InternalServerError`, not
`*types.InvalidJobIdException`; confirmed all 4 fail against unmodified
code.

Gates: `go build`, `go vet` (repo-wide, clean), `go test -race -count=1`,
`golangci-lint run` — all clean (`./services/textract/...`).

## 2026-09-06: InvalidS3ObjectException wiring (gopherstack-eshx)

`AnalyzeDocument`/`AnalyzeExpense`/`AnalyzeID`/`DetectDocumentText`/`StartDocumentAnalysis`/
`StartDocumentTextDetection`/`StartExpenseAnalysis`/`StartLendingAnalysis` all declare
`InvalidS3ObjectException` in their real `deserializeOpError<Op>` switch (verified via the
digit-safe `awk`+`grep -oE '"[A-Za-z0-9]+"'` extraction against
`textract@v1.43.4/deserializers.go`, not the earlier `[A-Za-z]+` pattern that silently drops
S3-named codes — see gopherstack-jkpi). Doc comment (types/errors.go:310):
"Amazon Textract is unable to access the S3 object that's specified in the request." None of
these ops previously checked a Document/DocumentLocation's S3Object against real S3 state at
all — any bucket/key, existent or not, was accepted.

Note: the 2026-08-31 error-envelope sweep (Notes above) recorded
`InvalidS3ObjectException* (StartDocumentAnalysis only)` for the four Start ops -- re-verified
this pass with the same digit-safe extraction and all four (`StartDocumentAnalysis`,
`StartDocumentTextDetection`, `StartExpenseAnalysis`, `StartLendingAnalysis`) declare it
identically. That earlier annotation was incorrect; corrected here rather than editing the
historical entry.

Followed the `services/cloudtrail` `S3Backend`/`SetS3Backend`/`wireXxxS3` precedent
(gopherstack-g9b4): `S3Backend` interface (`interfaces.go`, `HeadObject` only) implemented
directly by `s3.InMemoryBackend` (no adapter), `InMemoryBackend.SetS3Backend` setter
(`store.go`), `Handler.checkS3Object` (`handler.go`) calling it, wired from `cli.go`'s
`wireTextractS3` in `wireStorageAndSecretsIntegrations`. Unwired (no `SetS3Backend` call) or
Backend not `*InMemoryBackend`: no-op, matching this repo's unwired-hook-stays-permissive
convention. A `Document`/`DocumentPages` entry carrying inline `Bytes` instead of an S3Object
(`bucket==""`) is never checked — `Document.Bytes` is a real alternative-member of the Document
union (`Document.S3Object`/`Document.Bytes` in `types.go`), not an S3 reference. Only existence
is checked (`s3.InMemoryBackend.HeadObject`, which itself distinguishes a missing bucket from a
missing key) — "unreadable" or "wrong format" would require decoding the object, which this
mock does not and should not simulate.

`DocumentLocation` (`StartDocumentAnalysis`/`StartDocumentTextDetection`/
`StartExpenseAnalysis`/`StartLendingAnalysis`) has no `Bytes` alternative in the real SDK — only
`S3Object` — so every Start* request already required a non-empty bucket/key before this
change; the new check runs after that existing requirement.

Regression tests: `services/textract/s3_object_test.go` —
`TestSyncDocumentOps_S3ObjectValidation` (table: AnalyzeDocument/AnalyzeExpense/
DetectDocumentText), `TestAnalyzeID_S3ObjectValidation` (per-DocumentPages-entry),
`TestStartOps_S3ObjectValidation` (table: all four Start ops), each asserting a missing
bucket/key is rejected (400, `InvalidS3ObjectException`) and an existing one succeeds (200,
proving the check does not reject everything). `TestAnalyzeDocument_UnwiredS3StaysPermissive` /
`TestStartDocumentAnalysis_UnwiredS3StaysPermissive` prove the unwired path stays permissive.
`TestAnalyzeDocument_InlineBytesUnaffected` proves inline `Bytes` is never checked. All fail
against a neutered `checkS3Object` (HeadObject called but its result discarded) and pass
against the fix. `cli_textract_rekognition_s3_wiring_test.go` (root package) drives the real
`initializeServices` composition root end-to-end.

Gates: `go build ./...`, `go test -race -count=1 ./services/textract/...` and `.` (root),
`golangci-lint run ./ services/rekognition/... services/textract/...` — all clean.

## 2026-09-06: CreateAdapterVersion InvalidS3ObjectException gap (gopherstack-u416)

`CreateAdapterVersion` also declares `InvalidS3ObjectException` (re-verified with the
digit-safe `awk`+`grep -oE '"[A-Za-z0-9]+"'` extraction against
`deserializeOpErrorCreateAdapterVersion` in `textract@v1.43.4/deserializers.go`) but was missed
by the gopherstack-eshx sweep above, which covered only the 8 ops taking a Document/
DocumentLocation. Unlike those, `CreateAdapterVersion` takes its S3 reference through
`CreateAdapterVersionInput.DatasetConfig`, an `AdapterVersionDatasetConfig`
(`textract@v1.43.4/types/types.go:78`): `ManifestS3Object *S3Object`. Confirmed
`handleCreateAdapterVersion` (`handler_adapter_versions.go`) does parse `DatasetConfig` off the
wire (`createAdapterVersionInput.DatasetConfig`, required, passed through to
`CreateAdapterVersionWithOptions`) — so unlike rekognition's `IndexFaces`/`CreateDataset`
(never parse their Image-shaped field, structural gap, no check added), this op had a real,
reachable gap.

Added the same `checkS3Object` call used by the 8 existing ops, gated on
`DatasetConfig.ManifestS3Object != nil` (the real SDK's
`validateOpCreateAdapterVersionInput`/no `validateAdapterVersionDatasetConfig` means
`ManifestS3Object` itself is not client-required, only `DatasetConfig`). Existence only, same
as eshx — no attempt to validate manifest contents.

Regression tests in `s3_object_test.go`: `TestCreateAdapterVersion_S3ObjectValidation` (missing
bucket/key rejected 400 `InvalidS3ObjectException`, existing one succeeds 200) and
`TestCreateAdapterVersion_UnwiredS3StaysPermissive`. Verified `TestCreateAdapterVersion_S3ObjectValidation`
fails against the pre-fix `handleCreateAdapterVersion` (git-show revert, confirmed still
compiles, confirmed test failure, restored byte-identical).

Gates: `go test -race ./services/textract/...`, `golangci-lint run services/textract/...` —
both clean.
