---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: translate
sdk_module: aws-sdk-go-v2/service/translate@v1.36.4
last_audit_commit: 1efb1a758
last_audit_date: 2026-08-20
# PROVENANCE NOTE (2026-08-20): the 2026-08-20 sweep initially reported this
# manifest's previous stamp (2d47b51d4 / 2026-07-29) as failed provenance,
# because 2d47b51d4 is an ec2 commit that never touched services/translate/.
# That verdict was WRONG and is retracted here. The schema defines
# last_audit_commit as HEAD when the manifest was written, not as a commit
# touching this service, and 2d47b51d4 is dated 2026-07-29 -- exactly the
# recorded audit date. Three sibling manifests (shield, applicationautoscaling,
# apigatewaymanagementapi) cite the same sha with the same date, the signature
# of one legitimate batch audit that day. The stamp was correct.
# Third over-application of the provenance heuristic in this campaign; see
# gopherstack-z31a for the only test that actually discriminates.
overall: A            # genuine fixes: invented error code, wrong error-per-op, missing validation, stuck CREATING/UPDATING, dropped ParallelDataProperties.EncryptionKey
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  ImportTerminology: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - wrong error type (InvalidRequestException, not modeled for this op) for Name/TerminologyData/MergeStrategy/Directionality validation, now InvalidParameterValueException; TerminologyData was silently defaulted instead of validated required; added TerminologyData.Format enum + 10MB file size limit (LimitExceededException) + 50-tag limit (TooManyTagsException). 2026-08-20: wrapper-key sweep re-verified TerminologyProperties field-by-field against types.TerminologyProperties; AuxiliaryDataLocation (import-error-file S3 location) correctly omitted since this emulator never produces import errors/warnings, confirmed against botocore's ImportTerminologyResponse doc (no member marked required). FIXED 2026-08-23 (gopherstack-v71s): MergeStrategy accepted an empty value (`mergeStrategy != \"\" && mergeStrategy != \"OVERWRITE\"`) even though ImportTerminologyInput marks it \"This member is required\" (api_op_ImportTerminology.go) -- gopherstack was looser than AWS, not stricter, so this was the opposite direction from the InvalidRequestException-vs-InvalidParameterValueException fixes above. Now rejects an empty/absent MergeStrategy with InvalidParameterValueException, same error class this op already uses for every other required-field violation (Name/TerminologyData). Checked sibling translate ops (`handler_translation.go`'s Profanity gate) for the same empty-string-passes shape: Profanity is a genuinely optional TranslationSettings member (no \"required\" doc comment), so it correctly stays permissive -- not the same bug."}
  GetTerminology: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - missing Name now InvalidParameterValueException (this op has no InvalidRequestException in its modeled error list). 2026-08-20: TerminologyDataLocation{RepositoryType,Location} both present (both required per that shape's own \"required\" list); AuxiliaryDataLocation correctly omitted for the same no-errors-modeled reason as ImportTerminology."}
  DeleteTerminology: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - same InvalidParameterValueException correction as GetTerminology"}
  ListTerminologies: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateParallelData: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - resource now starts CREATING and advances to ACTIVE on GetParallelData poll (previously ACTIVE immediately, skipping the async state real AWS goes through); added ParallelDataConfig.Format enum + 50-tag limit; name-conflict error corrected from invented ResourceInUseException to real ConflictException"}
  GetParallelData: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - missing Name now InvalidParameterValueException (was InvalidRequestException, not modeled for this op); now advances CREATING/UPDATING -> ACTIVE one step per call (DescribeTextTranslationJob's advance-on-poll convention). 2026-08-20 wrapper-key sweep: fixed - parallelDataToMap (handler_parallel_data.go) never emitted ParallelDataProperties.EncryptionKey even though CreateParallelData accepts+persists it and the sibling terminologyToMap emits the analogous field for GetTerminology; proven with a real-SDK round-trip test (wire_sdk_roundtrip_test.go). AuxiliaryDataLocation/LatestUpdateAttemptAuxiliaryDataLocation correctly omitted (import/update-error S3 locations; this emulator never produces import/update errors, and neither member is required per GetParallelDataResponse)."}
  UpdateParallelData: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - LatestUpdateAttemptStatus/LatestUpdateAttemptAt are now real tracked per-attempt state (UPDATING -> ACTIVE via GetParallelData poll) instead of a hardcoded ACTIVE constant; added ParallelDataConfig.Format enum validation. 2026-09-04 parity-bug sweep: fixed - updating a resource still CREATING or UPDATING from a prior call was allowed to silently clobber it instead of raising ConcurrentModificationException (types/errors.go: 'Another modification is being made. That modification must complete before you can make your change.'), which this op models. The previous gaps entry calling ConcurrentModificationException undeterministic across the whole service was wrong for this op specifically: the emulator's own CREATING/UPDATING->ACTIVE async window (advance-on-GetParallelData-poll) is a deterministic, already-tracked state, not a real concurrent-write race. See TestUpdateParallelData_ConcurrentModification."}
  DeleteParallelData: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - missing Name now ResourceNotFoundException (this op models no validation exception at all, not even InvalidParameterValueException)"}
  ListParallelData: {wire: ok, errors: ok, state: ok, persist: ok, note: "confirmed pure read: does not advance CREATING/UPDATING state, matching ListTextTranslationJobs precedent"}
  StartTextTranslationJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - request had zero required-field validation (DataAccessRoleArn/InputDataConfig/OutputDataConfig/SourceLanguageCode/TargetLanguageCodes could all be omitted and a job would still be created); added InvalidRequestException for missing required fields, UnsupportedLanguagePairException for unrecognized language codes, ResourceNotFoundException when TerminologyNames/ParallelDataNames reference a resource that doesn't exist, and Settings enum validation (Brevity not supported for batch jobs per the API reference, unlike TranslateText/TranslateDocument)"}
  StopTextTranslationJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - missing JobId now ResourceNotFoundException (was InvalidRequestException, not modeled for this op). 2026-09-04 parity-bug sweep: fixed - a second Stop on a job that already left the stoppable window (already STOP_REQUESTED, or any terminal status) fabricated InvalidRequestException, an error this op does not model at all (its error set is ResourceNotFoundException/TooManyRequestsException/InternalServerException only, deserializers.go). Stop is now idempotent for those states: it reports the job's current status back unchanged, with no error. Replaces TestStopTextTranslationJob_StateGuard (which asserted the fabricated error) with TestStopTextTranslationJob_Idempotent."}
  DescribeTextTranslationJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - same ResourceNotFoundException correction as StopTextTranslationJob"}
  ListTextTranslationJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - Filter.JobStatus accepted any string silently matching zero jobs instead of rejecting unrecognized values; added InvalidFilterException validation against the JobStatus enum. gopherstack-wksw (2026-08-29, constraint-not-honoured sweep): the previous entry above covered only Filter.JobStatus -- Filter.JobName/SubmittedAfterTime/SubmittedBeforeTime (the other 3 of 4 real TextTranslationJobFilter members, api_op_ListTextTranslationJobs.go/types.go) were never read by the handler at all and the backend method didn't even accept them, so a real client's name or time-window request silently returned every job in the account. Separately, sort order was `sort.Strings(ids)` over JobID (a random UUID) -- arbitrary, not the documented order. Fixed: handler now decodes all 4 filter fields (textTranslationJobFilterFromMap, handler_text_translation_jobs.go) and enforces the Filter doc comment's 'you can only set one filter at a time' (InvalidFilterException if >1 is set); backend (matchesJobFilter/sortJobs, text_translation_jobs.go) applies JobName/JobStatus as exact match and SubmittedAfterTime/SubmittedBeforeTime as open time-bound filters, sorting ascending (oldest-first) only for SubmittedBeforeTime and descending (newest-first) otherwise -- both directions are explicitly documented on TextTranslationJobFilter's own SubmittedAfterTime/SubmittedBeforeTime doc comments; the no-time-filter default descending order is this pass's judgment call (undocumented case), noted in code. Proven via TestListTextTranslationJobs_SDKRoundTrip_Filters (wire_sdk_roundtrip_test.go), a real aws-sdk-go-v2 client round trip, confirmed failing pre-fix on all 4 subtests (JobName returned all 3 jobs instead of 1; SubmittedAfterTime/SubmittedBeforeTime returned all 3 instead of 2; default order came back in random UUID order, not newest-first)."}
  TranslateText: {wire: ok, errors: ok, state: n/a, persist: n/a, note: "fixed - TerminologyNames referencing a nonexistent terminology was silently ignored instead of erroring (real AWS models ResourceNotFoundException for exactly this, the operation's only named-resource reference); added TextSizeLimitExceededException (10,000-byte sync quota), UnsupportedLanguagePairException (language code not in the supported list), and Settings.Formality/Profanity/Brevity enum validation"}
  TranslateDocument: {wire: ok, errors: ok, state: n/a, persist: n/a, note: "fixed - Document.ContentType (a required member of Document) was read from the wire nowhere at all and never validated; added ContentType required check, LimitExceededException (100,000-byte document size quota -- this op models LimitExceededException, not TextSizeLimitExceededException, for size overflow), UnsupportedLanguagePairException, the same TerminologyNames ResourceNotFoundException fix as TranslateText, and Settings enum validation"}
  ListLanguages: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed - DisplayLanguageCode accepted any string; real Translate models a fixed 10-value enum (de/en/es/fr/it/ja/ko/pt/zh/zh-TW) distinct from the ~75 translation-target language codes this op itself returns; added UnsupportedDisplayLanguageCodeException"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - missing ResourceArn now InvalidParameterValueException (was InvalidRequestException, not modeled for this op); added 50-tag limit (existing+new union) -> TooManyTagsException"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - same InvalidParameterValueException correction as TagResource"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed - same InvalidParameterValueException correction as TagResource"}
# Families audited as a group (when per-op is impractical):
families:
  terminology: {status: ok, note: "ImportTerminology/GetTerminology/DeleteTerminology/ListTerminologies verified against TerminologyProperties/TerminologyDataLocation shapes and api-2.json's per-op error lists; error-code-per-op, Format enum, file size limit, and tag limit fixed"}
  parallel_data: {status: ok, note: "Create/Get/Update/Delete/List verified against ParallelDataProperties/ParallelDataDataLocation shapes and CreateParallelDataOutput/UpdateParallelDataOutput/DeleteParallelDataOutput; async CREATING/UPDATING lifecycle gap from the previous audit is now fixed (advance-on-GetParallelData-poll, mirroring advanceJob). 2026-08-20: fixed a dropped ParallelDataProperties.EncryptionKey member in parallelDataToMap; see GetParallelData note."}
  translation_jobs: {status: ok, note: "Start/Stop/Describe/List verified against TextTranslationJobProperties/JobDetails and api-2.json's per-op error lists; StartTextTranslationJob's missing required-field/language-pair/resource-reference validation fixed, error-code-per-op fixed for Stop/Describe"}
  translation: {status: ok, note: "TranslateText/TranslateDocument verified against TranslateTextOutput/TranslateDocumentOutput/AppliedTerminology/TranslationSettings shapes and Amazon Translate's guidelines/quotas page; missing terminology-reference validation, size limits, language-pair validation, ContentType, and Settings enum validation all fixed"}
  tags: {status: ok, note: "TagResource/UntagResource/ListTagsForResource verified against Tag{Key,Value} shape; error-code-per-op and 50-tag limit fixed"}
gaps:
  - "IMPOSSIBLE (re-confirmed gopherstack-llun): TranslateText/TranslateDocument echo SourceLanguageCode literally as 'auto' when omitted, instead of resolving it to a detected language code the way real AWS does (via an internal Comprehend call). Real language detection would require fabricating a plausible-looking detected language for arbitrary input text with no ground truth to check it against -- that is worse than an honest 'auto' echo, not better. Left as a mock limitation per parity principles (translation itself is inherently mocked)."
  - "ALREADY COVERED BY CHAOS (verified gopherstack-llun; CORRECTED 2026-09-04 for UpdateParallelData, see its ops entry): DetectedLanguageLowConfidenceException, TooManyRequestsException, InternalServerException, and ServiceUnavailableException (plus ConcurrentModificationException for every op EXCEPT UpdateParallelData) are real modeled errors for several ops but have no deterministic backend-state trigger in this synchronous, single-lock, unbounded in-memory emulator (no rate limiting, no enforced per-account resource quotas, no real concurrent-write races, no real Comprehend-backed language detection). Concretely verified this pass: translate.Handler implements ChaosServiceName() -> \"translate\" and ChaosOperations() -> h.GetSupportedOperations() (handler.go), and pkgs/chaos.Middleware is wired globally via registry.Use(chaos.Middleware(faultStore)) in cli.go -- it matches purely on the request's SigV4 service name + X-Amz-Target operation + region and injects an arbitrary caller-specified FaultError{Code, StatusCode}, never touching backend state. A fault rule such as {\"service\":\"translate\",\"error\":{\"code\":\"DetectedLanguageLowConfidenceException\",\"statusCode\":400}} deterministically returns that exact typed error to a real aws-sdk-go-v2 client on any operation, with zero backend code changes. Matches services/comprehend's documented precedent for the same class of unmodeled-but-real exceptions; proven end-to-end against a real containerized client in test/integration/chaos_test.go. DeleteParallelData also models ConcurrentModificationException with the same 'modification in progress' semantics, but no doc sentence on DeleteParallelData itself confirms delete is blocked during CREATING/UPDATING the way UpdateParallelData's fix does -- left ungated per the no-invented-guards rule; flagging for a future pass with stronger evidence."
  - "IMPOSSIBLE (re-confirmed gopherstack-llun): EncryptionKey.Type (KMS-only enum) and EncryptionKey.Id are accepted without validation across ImportTerminology/CreateParallelData/UpdateParallelData's OutputDataConfig.EncryptionKey. Encryption is inert in this mock (nothing is ever actually encrypted, no KMS cross-service key-existence check exists elsewhere in this pass's scope either), so the field has no real behavior to validate against -- adding an enum check here would be validation theater, not a wire-accuracy fix. Low-value/low-risk gap, left as-is."
  - "VALUE-CORRECTNESS, DISCLOSED NOT FIXED (2026-08-20 wrapper-key sweep): DeleteParallelData returns pd.Status as it stood immediately before deletion (e.g. ACTIVE), never the DELETING value real AWS documents for 'the status of the parallel data deletion' (DeleteParallelDataResponse.Status, botocore service-2.json). This is a right-key/right-type/questionable-VALUE issue, not a shape break -- ACTIVE is still a valid ParallelDataStatus enum member, so no client-side deserialization failure results -- and fixing it properly would need a transient DELETING state in the lifecycle model (delete marks DELETING, a later poll/janitor actually removes the row), which is lifecycle-state-machine work out of scope for a wrapper-key/nesting sweep. Left as-is; flagging for a future targeted pass."
  - "MISSING NON-REQUIRED MEMBERS, DISCLOSED NOT FIXED (2026-08-20 wrapper-key sweep): TerminologyProperties.SkippedTermCount and .Message, and ParallelDataProperties.FailedRecordCount/ImportedDataSize/ImportedRecordCount/SkippedRecordCount/.Message are real optional response members this emulator never populates (terminologyToMap/parallelDataToMap omit them entirely rather than emitting a zero value). None are marked required in types.TerminologyProperties/types.ParallelDataProperties, and populating them honestly would require modeling per-record import/skip counters the backend doesn't track today -- Layer-3-scope, left as a disclosed gap rather than fabricated."
  - "SEMANTIC, DISCLOSED NOT FIXED (2026-08-20 wrapper-key sweep): TextTranslationJobProperties.JobDetails is always {TranslatedDocumentsCount:0, DocumentsWithErrorsCount:0, InputDocumentsCount:0} regardless of job size (jobToMap, handler_text_translation_jobs.go) -- the wrapper key and nested field names are correct (verified against types.JobDetails), but the values are a hardcoded stub since this emulator never actually reads/counts documents in the InputDataConfig S3 location. Semantic gap, not a wire-shape bug; left as-is."
  - "SEMANTIC, DISCLOSED NOT FIXED (gopherstack-wksw, 2026-08-29 constraint-not-honoured sweep): ListLanguages' DisplayLanguageCode is validated against the real 10-value enum (fixed by a prior pass, see ops entry) but never actually applied -- knownLanguages() (handler_languages.go) returns every LanguageName in English regardless of the requested DisplayLanguageCode, since this emulator has no localized name table for the ~75 x 10 language/display-language combinations real AWS serves. The response's own DisplayLanguageCode field correctly echoes what was requested, so a client can tell what it asked for; only the LanguageName strings themselves don't follow it. Structural gap (no i18n data modeled anywhere in this service), not a filter/pagination bug -- left as-is rather than fabricating partial translations for a handful of languages."
deferred: []
leaks: {status: clean, note: "no goroutines/janitors in this service; job lifecycle advances synchronously inside DescribeTextTranslationJob and parallel-data lifecycle advances synchronously inside GetParallelData, both under the existing backend mutex, no new background state"}
---

## Notes

### 2026-08-29 constraint-not-honoured sweep (gopherstack-wksw)

New bug class for this campaign: a parameter constraining a result (filter/sort/page
limit) present in the real Input but not correctly honoured. All 5 collection-returning
ops (`ListLanguages`, `ListParallelData`, `ListTagsForResource`, `ListTerminologies`,
`ListTextTranslationJobs`) read against their own `api_op_List*.go` in
`translate@v1.36.4`. Confirmed JSON-RPC 1.1 (`awsAwsjson11_*`), every member body-bound
(no `serializeOpHttpBindings<Op>Input` function exists for any op in this service).

**1 real bug found and fixed** -- `ListTextTranslationJobs.Filter` (see its ops entry
above for full detail): 3 of 4 real filter fields never plumbed at all, plus a wrong sort
order. This is the deepest finding in this service for this class: the previous pass's
`ListTextTranslationJobs` entry read as "fixed" and specifically named `Filter.JobStatus`,
which was genuinely fixed -- but nothing in that entry said the sibling fields were even
checked, and they weren't plumbed. Matches this campaign's chokepoint lesson: a fix that
lands correctly on `Filter.JobStatus` says nothing about `Filter.JobName`/
`SubmittedAfterTime`/`SubmittedBeforeTime` on the same struct.

**1 gap newly disclosed** (not a fix for this class, a data-completeness gap surfaced
while checking `ListLanguages.DisplayLanguageCode`): see its own gaps entry above.

**Confirmed already correct**: `ListLanguages.MaxResults` (no documented default in the
SDK's own doc comment -- `store.go`'s internal default of 500 isn't a violation of an
unstated contract); `ListParallelData`/`ListTerminologies` (`MaxResults`/`NextToken`
only, no filter fields on either op's real Input, default page size 100 from the shared
`paginate` helper matches every sibling in this service); `ListTagsForResource` (no
pagination member on the real Input at all -- `ResourceArn` only, correctly has none).

Test style: real `aws-sdk-go-v2/service/translate` client round trip
(`TestListTextTranslationJobs_SDKRoundTrip_Filters`, `wire_sdk_roundtrip_test.go`) via the
existing `newTestTranslateSDKClient` helper -- deliberately not a hand-built request,
since this is exactly the "never plumbed" bug class the campaign brief calls out as most
likely to be missed by a hand-built request that already omits the field the same way the
bug does. Confirmed failing pre-fix on all 4 subtests. Timestamps controlled via a new
test-only `SetJobSubmittedAtForTest` in the pre-existing `export_test.go` (not a new file)
rather than real wall-clock sleeps between job creations. `go vet ./...` (repo-wide, since
`ListTextTranslationJobs`'s backend signature changed from
`(statusFilter string, maxResults int, nextToken string)` to
`(filter TextTranslationJobFilter, maxResults int, nextToken string)` -- 3 pre-existing
call sites in `persistence_test.go`/`text_translation_jobs_test.go` updated),
`go test -race -count=1 ./services/translate/...`, and
`golangci-lint run ./services/translate/...` (0 issues after decomposing the backend
filter/sort logic into `matchesJobFilter`/`sortJobs` to stay under `gocognit`'s ceiling,
and the handler's Filter decoding into `textTranslationJobFilterFromMap` to fix a
`govet` shadow warning and a `nestif` complexity flag -- no `//nolint` used) all clean.

- 2026-08-22, gopherstack-r80d batch 31 (required-output-member audit):
  translate (6 required output fields / 19 ops, 2 ops-with-required per a
  fresh `cmd/requiredoutputfields` run, cross-checked against an independent
  standalone `go/ast` walk of `translate@v1.36.4`'s `api_op_*.go` files --
  both agreed exactly at 6, lowest op-count of this batch's six-way tie).
  Module resolved directly: directory `translate` == SDK module
  `aws-sdk-go-v2/service/translate@v1.36.4` per `go.mod`, no sibling-module
  ambiguity for this name.

  `TranslateText` (`SourceLanguageCode`/`TargetLanguageCode`/`TranslatedText`,
  all `*string`) and `TranslateDocument` (same two language codes plus
  `*types.TranslatedDocument`, itself requiring `Content []byte`,
  types.go:512-520) are both built as a `map[string]any` literal
  (`translateText`/`translateDocument`, handler_translation.go:50-102,
  109-172) passed straight to `json.Marshal` by the shared op dispatcher
  (`json.Marshal(output)`, handler.go:154) -- not a tagged struct, matching
  batch 30's ssoadmin/mediatailor/shield finding for the same reason: there
  is no struct tag for an `omitempty` mistake to hide behind, so shape 1 of
  this campaign's bug class cannot occur syntactically here. Checked shape 2:
  `TargetLanguageCode` is validated non-empty before either function runs
  (`"TargetLanguageCode is required"`, handler_translation.go:59,113);
  `SourceLanguageCode` defaults to `"auto"` rather than being left empty
  (handler_translation.go:65-67,116-118) if the request omits it (gopherstack
  is intentionally more lenient here than the real SDK's own client-side
  validator, which requires it non-nil -- a pre-existing, separately-scoped
  input-validation gap, not this cut's target); `TranslatedText` is derived
  from `Text`, itself required non-empty (handler_translation.go:52-54), so
  it can only gain a language prefix or terminology substitutions, never
  become empty; `TranslatedDocument.Content` mirrors `Document.Content`
  byte-for-byte through a base64 round trip (handler_translation.go:138-147) --
  an explicitly empty (but present) `Content` is honest given a real client
  can legally send one (the real SDK's `validateDocument`, translate's
  validators.go, only null-checks `Content`/`ContentType`, not length), and
  the map-literal `"Content"` key is written unconditionally regardless.
  Result: 0 bugs. No code changes.
Protocol: **awsjson1.1** (single POST endpoint, `X-Amz-Target: AWSShineFrontendService_20170701.<Op>`,
`application/x-amz-json-1.1`) — confirmed against `translateTargetPrefix` in handler.go and the real
SDK's `httpBindingEncoder.SetHeader("Content-Type").String("application/x-amz-json-1.1")` in
serializers.go. Route matcher is a simple header-prefix check; unit tests call `Handler()` directly so
they exercise it for real (not bypassed).

### Real bugs found and fixed this sweep

This sweep's headline finding is that error **codes** were wrong far more often than error **triggers**
were missing: the previous audit (6eeaefc, 2026-07-13) got wire shapes and state-machine bugs right but
never field-diffed each operation's `errors: [...]` array in
`aws-sdk-go@v1.55.5/models/apis/translate/2017-07-01/api-2.json` against what the handler actually threw.
Every op's error set is enforced by a **per-operation** switch in the real SDK's generated
`deserializers.go` (`awsAwsjson11_deserializeOpError<Op>`), not a shared service-wide error table — an
error whose wire code isn't in that specific operation's switch falls through to a generic/untyped API
error client-side instead of the typed exception a caller's `errors.As` would expect.

1. **Invented error code: `ResourceInUseException`**. `ErrConflict` (used by `CreateParallelData`'s
   name-conflict check) mapped to the wire type `"ResourceInUseException"` — a real exception in
   *Comprehend's* API (this codebase's `services/comprehend` legitimately uses it) but **absent entirely**
   from Amazon Translate's exception set (confirmed against
   `aws-sdk-go-v2/service/translate/types/errors.go`, which has no such type, and the full
   `errors.EXCEPTION` list in `translate`'s `api-2.json`). Real Translate models `ConflictException` for
   exactly this case (`CreateParallelData`/`UpdateParallelData`'s error lists). Fixed by renaming the
   sentinel's wire string; this looks like the comprehend-convention sentinel name (`ErrConflict`) was
   copied into translate along with its Comprehend-specific wire string without checking whether Translate
   actually models the same exception.

2. **Wrong error type used for 8 operations** (`GetParallelData`, `DeleteParallelData`,
   `StopTextTranslationJob`, `DescribeTextTranslationJob`, `TagResource`, `UntagResource`,
   `ListTagsForResource`, and `ImportTerminology`/`GetTerminology`/`DeleteTerminology`). The handler used
   one shared `ErrValidation` ("InvalidRequestException") for every "required field missing" case
   service-wide, but several operations' modeled error lists don't include `InvalidRequestException` at
   all:
   - `ImportTerminology`/`GetTerminology`/`DeleteTerminology`/`TagResource`/`UntagResource`/
     `ListTagsForResource`/`GetParallelData` model `InvalidParameterValueException` but never
     `InvalidRequestException` → added `ErrInvalidParameter` and rewired these six operations to use it.
   - `DeleteParallelData`/`StopTextTranslationJob`/`DescribeTextTranslationJob` model **neither**
     `InvalidRequestException` nor `InvalidParameterValueException` — only `ResourceNotFoundException` (plus
     `ConcurrentModificationException`/`TooManyRequestsException`/`InternalServerException`, none of which
     have a deterministic trigger here). A missing key on these three ops now surfaces as
     `ResourceNotFoundException`, matching the only client-error type the real operation ever returns.

3. **`CreateParallelData`/`UpdateParallelData` skipped the async `CREATING`/`UPDATING` lifecycle**
   (carried over from the previous audit's documented gap, now fixed). `CreateParallelData` set
   `Status: "ACTIVE"` immediately; real `CreateParallelDataOutput`'s API reference documents "When the
   resource is ready for you to use, the status is `ACTIVE`" — implying it is *not* immediately ACTIVE.
   Fixed with the same "advance on poll" convention `DescribeTextTranslationJob`'s `advanceJob` already
   established for translation jobs: the resource now starts `CREATING` and `GetParallelData` advances it
   to `ACTIVE` one call later (`advanceParallelData` in parallel_data.go, `GetParallelData` now takes
   `b.mu.Lock()` instead of `RLock()` for the same reason `DescribeTextTranslationJob` does).
   `UpdateParallelData`'s `LatestUpdateAttemptStatus` was hardcoded to the literal string `"ACTIVE"` in the
   handler regardless of actual state — a disguised no-op that happened to always be correct only because
   there was no failure path and the resource never left ACTIVE in the first place. Fixed by adding real
   `LatestUpdateAttemptStatus`/`LatestUpdateAttemptAt` fields to the `ParallelData` struct: `Update` now
   sets `UPDATING`, and the same `GetParallelData` poll advances it to `ACTIVE`.

4. **`StartTextTranslationJob` had zero required-field validation**. `DataAccessRoleArn`, `InputDataConfig`
   (with its own required `S3Uri`+`ContentType`), `OutputDataConfig.S3Uri`, `SourceLanguageCode`, and
   `TargetLanguageCodes` are all required members of `StartTextTranslationJobRequest` (api-2.json), but the
   handler read every one of them with a blind `.(string)`/`.( map[string]any)` type assertion and silently
   proceeded with zero-value defaults on a miss — a job would be created with an empty
   `DataAccessRoleArn`/`InputDataConfig`/etc. and no client-visible error. Fixed with explicit required-field
   checks (`InvalidRequestException`, which this op does model).

5. **Referenced `TerminologyNames`/`ParallelDataNames` were never validated to exist** across
   `TranslateText`, `TranslateDocument`, and `StartTextTranslationJob`. All three model
   `ResourceNotFoundException`, and `TerminologyNames`/`ParallelDataNames` are the *only* named-resource
   references any of them make — `LookupTerminologies` silently skipped missing names instead of erroring,
   and `StartTextTranslationJob` didn't check `ParallelDataNames` at all. A previous-sweep unit test
   (`TestTranslateTextIncludesAppliedTerminologies`'s `unknown_terminology_name_omitted_from_applied` case)
   had actually encoded this bug as expected behavior; corrected to expect `ResourceNotFoundException`
   (see handler_translation_test.go's `TestTranslateText_UnknownTerminologyRejected`).

6. **No language-pair validation anywhere**. `TranslateText`, `TranslateDocument`, and
   `StartTextTranslationJob` all model `UnsupportedLanguagePairException`, but any string was accepted as a
   language code. Fixed by validating non-`auto` source and all target codes against
   `knownLanguageCodesTable` (the same ~75-language list `ListLanguages` serves).

7. **No synchronous size-quota enforcement**. Amazon Translate's guidelines/quotas page documents a
   10,000-byte limit on `TranslateText`'s `Text` (`TextSizeLimitExceededException`, confirmed against
   `BoundedLengthString`'s `max` in the smithy model) and a 100,000-byte limit on `TranslateDocument`'s
   `Document.Content` (`LimitExceededException` — this op does *not* model
   `TextSizeLimitExceededException`, unlike `TranslateText`) and a 10 MB limit on `ImportTerminology`'s
   `TerminologyData.File` (also `LimitExceededException`, confirmed against `TerminologyFile`'s `max`).
   None were enforced; all three now are.

8. **`Document.ContentType` was never read, validated, or required** despite being a required member of
   `Document` (api-2.json) — `TranslateDocument` only ever looked at `Document.Content`. Fixed with a
   required-field check.

9. **`Settings.Formality`/`Profanity`/`Brevity` were echoed back verbatim with no enum validation** (real
   enums: `FORMAL|INFORMAL`, `MASK`, `ON` respectively) across all three translation-settings-accepting
   operations, and `StartTextTranslationJob`'s API reference specifically documents `Brevity` as
   "not supported" for batch jobs (unlike `TranslateText`/`TranslateDocument`, which both support it) — a
   distinction the previous, settings-blind code couldn't have honored. Fixed with `validSettingsEnums`.

10. **`ListTextTranslationJobs`'s `Filter.JobStatus` and `ListLanguages`'s `DisplayLanguageCode` accepted
    any string.** `Filter.JobStatus` models `InvalidFilterException` for an unrecognized value (previously
    just silently matched zero jobs); `DisplayLanguageCode` models `UnsupportedDisplayLanguageCodeException`
    against a **fixed 10-value enum** (`de/en/es/fr/it/ja/ko/pt/zh/zh-TW`) that is deliberately much smaller
    than the ~75 translation-target language codes `ListLanguages` itself returns — a distinction easy to
    miss without reading the smithy model directly. Both now validate.

11. **`TerminologyData.Format`/`ParallelDataConfig.Format` accepted any string** instead of the modeled
    `CSV|TMX|TSV` enum (both shapes share the identical three-value enum). `ImportTerminology` additionally
    silently defaulted an entirely-omitted `TerminologyData` to an empty CSV terminology instead of
    triggering the backend's own (previously unreachable) "TerminologyData is required" check.

12. **`TooManyTagsException` (the real 50-tag-per-resource limit) was never enforced** on
    `ImportTerminology`, `CreateParallelData`, or `TagResource`, despite all three modeling it.

### Traps for the next auditor (looks-wrong-but-correct)

- `DescribeTextTranslationJob` and `GetParallelData` both take `b.mu.Lock()` (write lock), not `RLock()` —
  they mutate job/parallel-data state via `advanceJob`/`advanceParallelData` on every call. This is
  intentional, not a leftover. `ListTextTranslationJobs`/`ListParallelData` deliberately do NOT advance
  state (real List operations are pure reads).
- `persistence_test.go`'s `assertJobRestored` reads the restored job via `ListTextTranslationJobs` rather
  than `DescribeTextTranslationJob` specifically to avoid the assertion itself advancing the job's status
  as a side effect — don't "simplify" this back to Describe.
- Tests sending `TerminologyData.File`/`Document.Content` use the `b64()` helper (handler_test.go) instead
  of literal strings; a `File`/`Content` value that isn't valid base64 is correctly rejected as
  `InvalidRequestException`, so any new test must encode it.
- `ErrValidation` ("InvalidRequestException") is intentionally still used by `CreateParallelData`,
  `UpdateParallelData`, `StartTextTranslationJob`, `ListTextTranslationJobs`, `TranslateText`, and
  `TranslateDocument` — these six DO model `InvalidRequestException`. Don't blanket-replace it with
  `ErrInvalidParameter` service-wide; the split is per-operation and field-diffed against api-2.json's
  per-op `errors: [...]` arrays, not a stylistic preference.
- `LookupTerminologies`'s signature changed from `(names []string) []*Terminology` to
  `(names []string) ([]*Terminology, error)` — it now errors on any name that doesn't exist rather than
  silently skipping it. Both call sites (`translateText`/`translateDocument`) were updated; if a new op
  ever needs terminology lookup, don't revert to the old skip-missing behavior without re-checking whether
  that op also models `ResourceNotFoundException`.

## 2026-08-20 — wrapper-key / nested-shape wire-parity sweep

Scope: verify every emitted top-level response key, nesting level, JSON type, and enum value for all 19
ops against `aws-sdk-go-v2/service/translate@v1.36.4` directly (not the previous audit's own notes), plus
the `This member is required` grep across every type this service emits and the three
request/response-split shapes (`Document`/`TranslatedDocument`, `TerminologyData`/`TerminologyProperties`/
`TerminologyDataLocation`, `ParallelDataConfig`/`ParallelDataProperties`/`ParallelDataDataLocation`).

**Protocol reconfirmed**: `awsjson1.1` (JSON-RPC). `grep -c awsAwsjson11_deserializeOpDocument<Op>Output
deserializers.go` returns 2 (defined + called) for 18 of 19 ops; `DeleteTerminologyOutput` has no members at
all (empty struct) so the OpDocument helper doesn't exist for it — expected, not a gap. The restjson
flat-body false-positive trap this session's brief warned about does not apply: translate is JSON-RPC, and
`awsAwsjson11_*` always routes through the live OpDocument helper for every non-empty output.

**`This member is required` grep result**: only `TranslateTextOutput` (`TranslatedText`,
`SourceLanguageCode`, `TargetLanguageCode`) and `TranslateDocumentOutput` (`TranslatedDocument`,
`SourceLanguageCode`, `TargetLanguageCode`) mark any Output-struct member required — every other op's
Output struct has zero required top-level members. All six are modeled and populated
(`handler_translation.go`'s `translateText`/`translateDocument`). Required members on nested types
(`EncryptionKey.Id`/`.Type`, `InputDataConfig.ContentType`/`.S3Uri`, `OutputDataConfig.S3Uri`,
`Language.LanguageCode`/`.LanguageName`, `Tag.Key`/`.Value`, `TerminologyDataLocation`/
`ParallelDataDataLocation`'s `RepositoryType`/`Location`, `TranslatedDocument.Content`,
`Document.Content`/`.ContentType` (request-only), `TerminologyData.File`/`.Format` (request-only)) were all
confirmed modeled and populated everywhere they're emitted.

**One real bug found and fixed**: `ParallelDataProperties.EncryptionKey` was silently dropped from
`GetParallelData`/`ListParallelData` responses. `CreateParallelData` accepts and persists `EncryptionKey`
onto the `ParallelData` backend struct (`parallel_data.go:73`), and the sibling `terminologyToMap`
(`handler_terminologies.go:182-187`) already surfaces the analogous field for `GetTerminology`/
`ListTerminologies` — but `parallelDataToMap` (`handler_parallel_data.go`) never emitted an `EncryptionKey`
key at all, so a real client's `GetParallelDataOutput.ParallelDataProperties.EncryptionKey` deserialized as
`nil` regardless of what was set at creation. This is the sweep's dominant bug class (a): a member modeled
on one type and correctly emitted for a wider/sibling shape, silently missing from the narrower one. Fixed
in `handler_parallel_data.go`'s `parallelDataToMap`; proven with a real-SDK-client round-trip test,
`TestGetParallelData_SDKRoundTrip_EncryptionKey` (`wire_sdk_roundtrip_test.go`), which creates a parallel
data resource with an `EncryptionKey`, calls the real `translatesdk` client's `GetParallelData`, and asserts
`out.ParallelDataProperties.EncryptionKey.Id`/`.Type` are non-nil and correct. Hand-reverted the fix: the
test failed with `Expected value not to be nil` at the `EncryptionKey` assertion, exactly the predicted
symptom; restored, confirmed the diff returned to the intended one-hunk addition.

**Verified clean (no wire bug)**: `GetTerminology`/`ImportTerminology` correctly omit
`AuxiliaryDataLocation`, and `GetParallelData` correctly omits `AuxiliaryDataLocation`/
`LatestUpdateAttemptAuxiliaryDataLocation` — all three are import/update-error-file S3 locations that only
populate when the real service encounters errors/warnings in the input file, which this emulator (no error
path in import/update) never does; confirmed via botocore's `translate/2017-07-01/service-2.json` doc
strings and that none of these members are marked required in their respective `*Response` shapes. The
three request/response splits named in this session's brief were all reconfirmed clean: `Document`
(request: `Content`+`ContentType`) never leaks into `TranslatedDocument` (response: `Content` only, and
that's all `handler_translation.go`'s `translateDocument` emits); `TerminologyData` (request-only: `File`+
`Format`+`Directionality`) never appears in any response, only its already-correct `TerminologyProperties`/
`TerminologyDataLocation` counterparts do; `ParallelDataConfig` correctly nests *inside*
`ParallelDataProperties` (both request- and response-side per the real shape) with no confusion against the
separate `ParallelDataDataLocation` S3-location shape. `TranslationSettings` (used identically on both the
request `Settings` and response `AppliedSettings`) showed no field leakage in either direction since it's
the literal same type both ways in the real SDK too.

**`last_audit_commit` provenance verdict: FAILED, then corrected.** The prior manifest cited
`last_audit_commit: 2d47b51d4` / `last_audit_date: 2026-07-29`. `git show -s --format=%ad 2d47b51d4` does
date to 2026-07-29, but `git show --stat 2d47b51d4` shows its actual content is
`fix(ec2): RestoreImageFromRecycleBin no longer reports success for a no-op` — a wholly unrelated EC2 fix,
never touching `services/translate/`. The real translate audit commit matching this manifest's prose
(per-op error taxonomy, parallel-data CREATING/UPDATING lifecycle, terminology/language-pair validation) is
`afe5bb500` (`fix(translate): per-op error taxonomy, parallel-data lifecycle, validation, terminology
checks`), dated **2026-07-24 — five days before** the claimed `last_audit_date`, exactly the
days-to-weeks-before tell this session's brief warned about. `last_audit_commit`/`last_audit_date` above are
now corrected to the real commit and today's date. SDK version check: `sdk_module` header
(`aws-sdk-go-v2/service/translate@v1.36.4`) matches `go.mod` exactly; the manifest's prose citations of
`aws-sdk-go@v1.55.5/models/apis/translate/2017-07-01/api-2.json` are a different (v1, model-only) package
used solely to read the smithy `api-2.json` source and don't restate the pinned v2 client version, so this
is not the header/prose mismatch pattern flagged elsewhere this session. Every "fixed" error-taxonomy claim
spot-checked this pass (all `InvalidParameterValueException`-vs-`InvalidRequestException`-vs-neither splits,
and the `ResourceInUseException`→`ConflictException` correction) re-derived correctly against
`translate/2017-07-01/service-2.json`'s per-operation `errors: [...]` arrays.

**Gates**: `go build ./services/translate/...`, `go vet ./services/translate/...`, `go fix -diff
./services/translate/...`, `gofmt -l services/translate/` all clean/empty; `go test -race
./services/translate/...` passes (2.5s); `golangci-lint run ./services/translate/...` reports `0 issues`.

## Equality-matched-cursor restart sweep (2026-08-30)

Every paginated listing in this service (`ListTerminologies`, `ListParallelData`,
`ListTextTranslationJobs`, `ListLanguages`) resumed a `NextToken` by scanning for the
item whose key equalled the token and left `start` at 0 on no match -- an unresolvable
token (a forged/stale value, or a deleted terminology/parallel-data resource) restarted
pagination at page one instead of erroring or truncating.

Fixed by defaulting the miss to the end of the collection (empty final page) in both
`store.go`'s shared `paginate[T]` (serves all three `ListTerminologies`/
`ListParallelData`/`ListTextTranslationJobs`) and `handler_languages.go`'s
`listLanguages`. Threshold search (resume at the first key `>` the token) was not used:
`ListTerminologies`/`ListParallelData` are sorted by the same field the cursor carries
(`Name`) and would have supported it, but the shared `paginate` helper also serves
`ListTextTranslationJobs`, which is sorted by `SubmittedAt` with a `JobID` tiebreak --
not by `JobID` (the cursor field) -- so a threshold search on the shared helper would
have been wrong for that caller. `ListLanguages`'s built-in `knownLanguages()` table is
sorted by `LanguageName`, not `LanguageCode` (the cursor field), so threshold search was
invalid there too. `ListLanguages`'s and `ListTextTranslationJobs`'s built-ins/jobs have
no delete operation, so the hostile test for those two forges an unresolvable token
rather than deleting an item; `ListTerminologies`/`ListParallelData` genuinely delete
the cursor's item mid-page (`DeleteTerminology`/`DeleteParallelData` both exist).

Confirmed no other pagination bug class present: every listing sorts before paginating
(no never-sorted walk), and `NextToken`/`Marker` handling elsewhere in this service
(`ListTagsForResource`) has no pagination member on the real Input at all, so it's
correctly unpaginated rather than missing pagination.

New tests (`handler_pagination_restart_test.go`, all confirmed failing pre-fix):
`TestListTerminologies_Pagination_DeletedMidPage`,
`TestListParallelData_Pagination_DeletedMidPage`,
`TestListTextTranslationJobs_Pagination_StaleTokenDoesNotRestart`,
`TestListLanguages_Pagination_StaleTokenDoesNotRestart`. Prior pagination coverage
(`TestListTerminologies_Pagination`, `TestListParallelData_Pagination`,
`TestListLanguages_Pagination`) only ever exercised the happy path where every named
cursor still resolves -- none deleted an item or forged a token between pages.

**Gates**: `go build ./services/translate/...`, `go vet ./services/translate/...`,
`go test -race -count=1 ./services/translate/...` all pass; `golangci-lint run
./services/translate/...` reports 0 issues.
