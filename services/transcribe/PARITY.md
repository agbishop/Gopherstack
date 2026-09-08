---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: transcribe
sdk_module: aws-sdk-go-v2/service/transcribe@v1.64.0   # version audited against
last_audit_commit:                                # unknown: gopherstack-6flj wrapper-key sweep pass ran without git access at write time, never backfilled -- gopherstack-33in
last_audit_date: 2026-09-05
overall: A            # A = genuine fixes found; B = already-accurate, proven op-by-op
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  StartTranscriptionJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "added LanguageIdSettings threading; removed invented top-level OutputBucketName/OutputKey (not real TranscriptionJob fields -- output location only lives in Transcript.TranscriptFileUri). 2026-09-05: FIXED four required-member gaps -- ContentRedaction.RedactionOutput was validated only when present, never required (real member is required, types.go ContentRedaction.RedactionOutput); Settings.ShowSpeakerLabels/MaxSpeakerLabels and ShowAlternatives/MaxAlternatives were each only bounds-checked one direction, never enforced as a required pair (types.go Settings doc: 'If you enable ShowSpeakerLabels...you must also include MaxSpeakerLabels', and the MaxSpeakerLabels-implies-ShowSpeakerLabels reverse direction, same pattern for Alternatives)."}
  GetTranscriptionJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fixes as Start; deferred-job polling advances QUEUED->IN_PROGRESS->COMPLETED correctly"}
  ListTranscriptionJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "added JobNameContains filter + missing TranscriptionJobSummary fields (StartTime, IdentifyLanguage, IdentifyMultipleLanguages, IdentifiedLanguageScore, ContentRedaction, ModelSettings, LanguageCodes, ToxicityDetection, OutputLocationType)"}
  DeleteTranscriptionJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "forgets resource tags"}
  StartCallAnalyticsJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj: CallAnalyticsSettings.LanguageIdSettings was entirely unmodeled (zero grep hits) -- added map[string]LanguageIDSettings field, flows through automatically since Settings is passed by reference"}
  GetCallAnalyticsJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-6flj: same LanguageIdSettings fix (shared Settings type)"}
  ListCallAnalyticsJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "added JobNameContains filter + missing StartTime on CallAnalyticsJobSummary. gopherstack-6flj: confirmed CallAnalyticsJobDetails/Skipped (per-summary) still a disclosed gap, see gaps:"}
  DeleteCallAnalyticsJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "forgets resource tags"}
  CreateCallAnalyticsCategory: {wire: ok, errors: ok, state: ok, persist: ok, note: "CategoryProperties now includes CreateTime/LastUpdateTime/Tags (were silently dropped). gopherstack-6flj: all 4 Rule filter types (NonTalkTimeFilter/InterruptionFilter/TranscriptFilter/SentimentFilter) were missing AbsoluteTimeRange/RelativeTimeRange sub-parameters entirely -- added both shared types, threaded through since CallAnalyticsRule flows by reference. 2026-09-05: FIXED -- Rules was entirely unvalidated: the real member is required (validators.go validateOpCreateCallAnalyticsCategoryInput, 'Rules is required'), bounded to 1-20 entries ('you must create between 1 and 20 rules', api_op_CreateCallAnalyticsCategory.go doc), and each populated TranscriptFilter/SentimentFilter had zero validation of its own required sub-fields (Targets/TranscriptFilterType, Sentiments). A client omitting Rules, sending 0 or 21+ rules, or an incomplete filter got HTTP 200 with silently accepted garbage."}
  GetCallAnalyticsCategory: {wire: ok, errors: ok, state: ok, persist: ok, note: "same CategoryProperties fix. gopherstack-6flj: same AbsoluteTimeRange/RelativeTimeRange fix"}
  UpdateCallAnalyticsCategory: {wire: ok, errors: ok, state: ok, persist: ok, note: "same CategoryProperties fix. gopherstack-6flj: same AbsoluteTimeRange/RelativeTimeRange fix. 2026-09-05: same Rules required/1-20/sub-field validation fix as Create (validators.go validateOpUpdateCallAnalyticsCategoryInput requires Rules identically)"}
  DeleteCallAnalyticsCategory: {wire: ok, errors: ok, state: ok, persist: ok, note: "forgets resource tags"}
  ListCallAnalyticsCategories: {wire: ok, errors: ok, state: ok, persist: ok, note: "same CategoryProperties fix. gopherstack-6flj: same AbsoluteTimeRange/RelativeTimeRange fix"}
  CreateLanguageModel: {wire: ok, errors: ok, state: ok, persist: ok, note: "output now echoes InputDataConfig (was dropped). 2026-09-05: FIXED -- InputDataConfig itself was only validated (S3Uri/DataAccessRoleArn required) when non-nil, but the real member is required on the request (validators.go validateOpCreateLanguageModelInput: 'InputDataConfig is required'); a client omitting it entirely got HTTP 200 with no training data configured."}
  DeleteLanguageModel: {wire: ok, errors: ok, state: ok, persist: ok, note: "forgets resource tags"}
  DescribeLanguageModel: {wire: ok, errors: ok, state: ok, persist: ok, note: "added FailureReason field to LanguageModel (was missing entirely)"}
  ListLanguageModels: {wire: ok, errors: ok, state: ok, persist: ok, note: "added NameContains filter"}
  CreateVocabulary: {wire: ok, errors: ok, state: ok, persist: ok, note: "output now includes LastModifiedTime + FailureReason (both were missing)"}
  GetVocabulary: {wire: ok, errors: ok, state: ok, persist: ok, note: "output now includes FailureReason"}
  UpdateVocabulary: {wire: ok, errors: ok, state: ok, persist: ok, note: "output now includes LastModifiedTime"}
  DeleteVocabulary: {wire: ok, errors: ok, state: ok, persist: ok, note: "forgets resource tags"}
  ListVocabularies: {wire: ok, errors: ok, state: ok, persist: ok, note: "added NameContains filter + top-level Status field (echoes StateEquals, per real ListVocabulariesOutput). gopherstack-6flj: per-item VocabularyInfo was missing LastModifiedTime entirely (shared type also used by ListMedicalVocabularies) -- fixed"}
  CreateVocabularyFilter: {wire: ok, errors: ok, state: ok, persist: ok, note: "output now includes LastModifiedTime (was missing)"}
  GetVocabularyFilter: {wire: ok, errors: ok, state: ok, persist: ok, note: "output now includes DownloadUri + LastModifiedTime (both were missing entirely -- a client could not previously fetch a filter's contents via Get)"}
  UpdateVocabularyFilter: {wire: ok, errors: ok, state: ok, persist: ok, note: "output now includes LastModifiedTime"}
  DeleteVocabularyFilter: {wire: ok, errors: ok, state: ok, persist: ok, note: "forgets resource tags"}
  ListVocabularyFilters: {wire: ok, errors: ok, state: ok, persist: ok, note: "added NameContains filter + LastModifiedTime on VocabularyFilterInfo (was missing)"}
  CreateMedicalVocabulary: {wire: ok, errors: ok, state: ok, persist: ok, note: "output now includes LastModifiedTime + FailureReason"}
  GetMedicalVocabulary: {wire: ok, errors: ok, state: ok, persist: ok, note: "output now includes FailureReason"}
  UpdateMedicalVocabulary: {wire: ok, errors: ok, state: ok, persist: ok, note: "output now includes LastModifiedTime"}
  DeleteMedicalVocabulary: {wire: ok, errors: ok, state: ok, persist: ok, note: "forgets resource tags"}
  ListMedicalVocabularies: {wire: ok, errors: ok, state: ok, persist: ok, note: "added NameContains filter + top-level Status field. gopherstack-6flj: same VocabularyInfo.LastModifiedTime fix as ListVocabularies (shared real type)"}
  StartMedicalScribeJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "removed invented top-level OutputBucketName; added synthesized MedicalScribeOutput (ClinicalDocumentUri/TranscriptFileUri), a real field gopherstack omitted entirely. gopherstack-6flj FLAGSHIP FIX: ClinicalNoteGenerationSettings was wire-tagged at the TOP LEVEL of both the request and MedicalScribeJob response; the real StartMedicalScribeJobInput has NO top-level member of that name at all -- it exists only nested under Settings (types.MedicalScribeSettings.ClinicalNoteGenerationSettings, types/types.go:1058). Confirmed the real deserializer's default case silently skips unrecognized top-level keys (no error), so this was a true silent-empty wrapper-key bug in both directions. Moved the field into MedicalScribeSettings; the top-level field on both wire structs and the backend MedicalScribeJob struct was removed. 2026-09-05 FLAGSHIP FIX: Settings was entirely unvalidated for its documented cross-field constraints (api_op_StartMedicalScribeJob.go doc: 'Settings...must set exactly one of ShowSpeakerLabels or ChannelIdentification to true. If ShowSpeakerLabels is true, MaxSpeakerLabels must also be set' and 'ChannelDefinitions...should be set if and only if the ChannelIdentification value of Settings is set to true') -- Settings itself wasn't even required (validators.go requires it), so a request with neither flag set, both flags set, ShowSpeakerLabels without MaxSpeakerLabels, or ChannelIdentification without matching ChannelDefinitions all got HTTP 200. Also added the missing MedicalScribeChannelDefinition.ParticipantRole required+enum check (types.go: 'This member is required', PATIENT/CLINICIAN)."}
  GetMedicalScribeJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fixes as Start. gopherstack-6flj: same ClinicalNoteGenerationSettings nesting fix"}
  ListMedicalScribeJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (was partial): summary now trimmed to the real MedicalScribeJobSummary fields (no more Media/Settings/Tags/ChannelDefinitions leaking through) + added JobNameContains filter"}
  DeleteMedicalScribeJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "forgets resource tags"}
  StartMedicalTranscriptionJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "removed invented top-level OutputBucketName/OutputKey. FIXED 2026-08-11 -- request field was wire-tagged MedicalContentIdentificationType (the shape's type name, not its wire field name); the real StartMedicalTranscriptionJobRequest field is ContentIdentificationType, so every real client's value was silently discarded. medicalTranscriptionJobSummary (ListMedicalTranscriptionJobs) already used the correct name. 2026-09-05: FIXED -- LanguageCode was validated against the full 75-code supportedLanguageCodes() allowlist shared with StartTranscriptionJob, but this op's doc is explicit: 'US English (en-US) is the only valid value for medical transcription jobs. Any other value you enter for language code results in a BadRequestException error' (api_op_StartMedicalTranscriptionJob.go). Any non-en-US code (e.g. fr-FR) was previously accepted."}
  GetMedicalTranscriptionJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as Start. FIXED 2026-08-11 -- response field (shared medicalTranscriptionJobOutput struct) had the same MedicalContentIdentificationType/ContentIdentificationType wire-name bug on the response side"}
  ListMedicalTranscriptionJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (was partial): summary now trimmed to the real MedicalTranscriptionJobSummary fields, plus added the previously-missing OutputLocationType/ContentIdentificationType/Specialty/Type/StartTime fields + JobNameContains filter"}
  DeleteMedicalTranscriptionJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "forgets resource tags"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  vocabulary_get_lastmodified: {status: ok, note: "unchanged this pass"}
  list_namecontains_filters: {status: ok, note: "NEW this pass: NameContains/JobNameContains was completely unimplemented on all 7 List ops that document it (ListVocabularies, ListMedicalVocabularies, ListVocabularyFilters, ListTranscriptionJobs, ListMedicalTranscriptionJobs, ListMedicalScribeJobs, ListCallAnalyticsJobs, ListLanguageModels); a client filtering by name substring got the unfiltered full list back. Fixed via matchesNameContains (case-insensitive substring, store.go) threaded through every backend List method + StorageBackend interface + handler input struct."}
  language_id_settings: {status: ok, note: "NEW this pass: LanguageIdSettings (StartTranscriptionJobInput field + TranscriptionJob.LanguageIdSettings response field, used for per-language custom-vocabulary/model selection under IdentifyLanguage/IdentifyMultipleLanguages) was entirely unimplemented -- not in the input struct, not stored, not echoed. Added end-to-end."}
  invented_output_fields_removed: {status: ok, note: "NEW this pass: TranscriptionJob/MedicalTranscriptionJob/MedicalScribeJob Get+Start responses previously echoed top-level OutputBucketName/OutputKey fields that do not exist on the real TranscriptionJob/MedicalTranscriptionJob/MedicalScribeJob response shapes (confirmed against types.go -- output location is only ever surfaced via the nested Transcript/MedicalScribeOutput URIs). Removed from all three wire-output structs; the backend structs keep the fields internally to compute the synthetic S3 URIs."}
  max_results_honored: {status: ok, note: "FIXED this pass (gopherstack-5or5): MaxResults was accepted on the wire but silently discarded on all 9 List* ops (ListTranscriptionJobs, ListVocabularies, ListVocabularyFilters, ListMedicalVocabularies, ListMedicalScribeJobs, ListCallAnalyticsCategories, ListMedicalTranscriptionJobs, ListCallAnalyticsJobs, ListLanguageModels) -- page size was always the fixed transcribeDefaultPageSize=100 constant. Field-diffed the real API reference for every List op: all 9 document identical bounds, 'Valid Range: Minimum value of 1. Maximum value of 100', default of 5 when omitted. paginateList/clampMaxResults (store.go) now honor a caller-supplied MaxResults clamped to [1,100]; threaded through all 9 backend methods + StorageBackend interface + handler input structs. gopherstack intentionally keeps the larger transcribeDefaultPageSize=100 (not AWS's documented default of 5) when MaxResults is omitted -- real SDK clients always page via NextToken regardless of page size, so a larger unrequested default page is non-breaking and was already gopherstack's established (if previously unintentional) behavior."}
  language_id_settings_validation: {status: ok, note: "FIXED this pass (gopherstack-5or5, partial): LanguageIdSettings previously had zero validation. Added: map size <= 5 entries ('Map Entries: Maximum number of 5 items'), keys must be supported language codes, and LanguageModelName sub-parameter is rejected when IdentifyMultipleLanguages is set ('multi-language identification doesn't support custom language models', per StartTranscriptionJob docs). Deliberately NOT enforced: AWS only *recommends* (does not require) also supplying LanguageOptions alongside LanguageIdSettings ('It's recommended that you include LanguageOptions when using LanguageIdSettings') -- the original issue described this as a hard cross-validation gap, but the real API doc language is a recommendation, not a rejection rule, so adding a hard error here would be inventing behavior the real service doesn't have."}
  language_code_allowlist_derived: {status: ok, note: "FIXED this pass (gopherstack-z6e7): supportedLanguageCodes() was a hardcoded 42-entry list; re-diffing against the pinned SDK's types.LanguageCode.Values() (transcribe@v1.58.4, types/enums.go:259) found 75 missing codes, not the 12 the triggering issue described -- the earlier gap note undercounted. Fixed by deriving supportedLanguageCodes() directly from sdktypes.LanguageCode(\"\").Values() (validation.go) instead of hand-copying, so it cannot drift again on a future SDK bump. Confirmed no reverse direction: every one of the old 42 hardcoded codes is a subset of the SDK enum (no code gopherstack accepted that AWS rejects). Also audited every other hand-maintained allowlist in the service (MediaFormat, VocabularyFilterMethod, RedactionType, RedactionOutput, SubtitleFormat, CallAnalyticsInputType, BaseModelName, MedicalSpecialty, MedicalType, MedicalContentIdentificationType) against their SDK enums -- all matched exactly, none drifted. Regression test: transcription_jobs_test.go's every_sdk_enum_code_accepted iterates types.LanguageCode.Values() directly against StartTranscriptionJob."}
  filter_value_semantics: {status: ok, note: "2026-08-30 (gopherstack-uox6 value-semantics pass, CLEAN -- no bug found): audited every List op's filter matching, this service's declared-but-previously-unexamined axis. All 9 backend List methods (ListVocabularies, ListMedicalVocabularies, ListVocabularyFilters, ListTranscriptionJobs, ListMedicalTranscriptionJobs, ListMedicalScribeJobs, ListCallAnalyticsJobs, ListLanguageModels, ListCallAnalyticsCategories -- the last has no filter params at all) use a uniform, correct AND-of-(equality-on-Status/StateEquals, matchesNameContains-substring) shape; matchesNameContains (store.go) is case-insensitive per its own doc citation of the AWS 'the search is not case sensitive' wording, confirmed against each caller with no per-caller disagreement (the shared-matcher-with-disagreeing-callers shape from other services' passes does not apply here -- every List op's Status/StateEquals/NameContains semantics match verbatim). No enum-mismatch: Status/StateEquals values are compared directly against internally-stored state strings, not a separately-validated user enum, so there is no unrecognized-value branch to get wrong. VocabularyFilterMethod (transcription_jobs.go) is validated against supportedVocabularyFilterMethods() but never applied to transcript content -- confirmed this is the same genuine-impossibility class as ContentRedaction and the language-model axis: transcript_synthesis.go's deriveTranscriptText/synthesizeTranscriptJSON produce wholly synthetic placeholder text (job name + media filename), so there is no real transcript content for a filter method to act on; not a value-applied-wrong bug, already covered by this service's standing synthetic-content disclosure."}
gaps:
  - "CallAnalyticsJobDetails (skipped-analytics-feature reporting) on CallAnalyticsJobSummary/CallAnalyticsJob is not implemented -- gopherstack's synthetic backend never skips any Call Analytics feature, so this optional field would always be absent/empty in a real scenario too; low priority. Re-checked this pass (gopherstack-5or5): still true, still no backing data to populate Skipped[] truthfully, left undone rather than fabricated. Re-confirmed gopherstack-6flj (2026-08-15): still zero grep hits, still no backing data source; disclosed not fixed."
  - "MedicalScribeContext (StartMedicalScribeJobInput patient-context field) and MedicalScribeContextProvided (response echo of whether it was supplied) are not implemented. Since gopherstack never accepts MedicalScribeContext, MedicalScribeContextProvided would always be false, and awsjson1.1 omits false bool fields on the wire (matching the omitted-field behavior already produced by not implementing it) -- low priority, not client-breaking. Re-checked this pass (gopherstack-5or5): still true. Re-confirmed gopherstack-6flj (2026-08-15): still unimplemented; a safe superset (real client that sets MedicalScribeContext gets no error, just a false-negative on the Provided echo), same category as xray's Sampling/SamplingStrategy no-op disclosure."
deferred: []
leaks: {status: clean, note: "no goroutines/janitors in this service; Snapshot/Restore delegate cleanly to InMemoryBackend; Handler.Snapshot/Restore already exposed. New backend struct fields (LanguageIdSettings, FailureReason x3, MedicalScribeOutput synthesis) are all pure additive struct fields going through the existing generic store.Table snapshot/restore path (store_setup.go) -- no new tables, no new lock paths, no persistence.go changes needed."}
---

## Notes

Protocol: awsjson1.1, single POST endpoint, `X-Amz-Target: Transcribe.<Op>`. Route
matcher (`RouteMatcher`/`ExtractOperation` in handler.go) is a simple prefix match on
the target header — verified all 43 ops are reachable via `TestSDKCompleteness`
(sdk_completeness_test.go), which fails if the upstream SDK adds an op gopherstack
hasn't wired up. No stub registration issues found (single `buildOps()` map, no
overwrite-order hazard).

### Bug found and fixed #1 — RFC3339 string timestamps instead of epoch-seconds numbers

Amazon Transcribe's json-1.1 protocol serializes all `time.Time` shapes (CreationTime,
StartTime, CompletionTime, CreateTime, LastModifiedTime) as epoch-seconds JSON
*numbers* — confirmed directly against the real SDK's `deserializers.go`, which calls
`smithytime.ParseEpochSeconds(f64)` on every one of these fields across
TranscriptionJob, TranscriptionJobSummary, CallAnalyticsJob, CallAnalyticsJobSummary,
CategoryProperties, LanguageModel, MedicalScribeJob(Summary),
MedicalTranscriptionJob(Summary), VocabularyFilterInfo, VocabularyInfo.

Before this fix, every timestamp field across TranscriptionJob, CallAnalyticsJob,
MedicalScribeJob, MedicalTranscriptionJob, and LanguageModel outputs was formatted as
an RFC3339 **string** (`time.RFC3339`) instead. Per `pkgs/awstime`'s doc comment, real
`aws-sdk-go-v2` deserializers *reject* an RFC3339 string in this position outright
("expected Timestamp to be a JSON Number, got string instead") — meaning every single
real SDK client calling GetTranscriptionJob, ListTranscriptionJobs,
GetCallAnalyticsJob, ListCallAnalyticsJobs, GetMedicalScribeJob,
GetMedicalTranscriptionJob, or DescribeLanguageModel/ListLanguageModels against
gopherstack would fail to unmarshal the response. This was the widest-blast-radius bug
found this sweep — it affected every read path across five of the eight resource
families. Fixed by switching all six affected output builders
(`buildTranscriptionJobOutput`, `buildCallAnalyticsJobOutput`,
`buildMedicalScribeJobOutput`, `buildMedicalTranscriptionJobOutput`,
`toLanguageModelOutput`, and the two List summary loops) to `*float64` fields
populated via `pkgs/awstime.Epoch`. GetVocabulary/GetMedicalVocabulary's
`LastModifiedTime` was already a `*float64` (via raw `.Unix()`), so it wasn't broken,
but was switched to `awstime.Epoch` too for sub-second precision and to match the new
house style.

Regression test: `TestTranscribe_TimestampFields_AreJSONNumbers` and
`TestTranscribe_DescribeLanguageModel_TimestampFieldsAreJSONNumbers` in
handler_test.go decode each timestamp field as `encoding/json.Number` and fail if it
was emitted as a quoted string.

### Bug found and fixed #2 — Tags silently dropped on 5 Create ops; creation-time Tags on all 9 ops never observable via ListTagsForResource

Real AWS Transcribe's `CreateVocabulary`, `CreateVocabularyFilter`,
`CreateMedicalVocabulary`, `CreateLanguageModel`, and `CreateCallAnalyticsCategory`
inputs *all* carry a `Tags []Tag` field (confirmed in the real SDK's
`api_op_Create*.go`), and per AWS docs these become real resource tags immediately,
retrievable only via `ListTagsForResource` (none of GetVocabulary,
DescribeLanguageModel, GetVocabularyFilter, GetMedicalVocabulary, or
GetCallAnalyticsCategory echo Tags back — confirmed against the real SDK's output
structs). Before this fix, gopherstack's request-input structs for all five of these
ops had no `Tags` field at all — a client tagging a vocabulary/filter/language
model/category at creation time got HTTP 200 with the tags completely discarded, no
error, no trace. `CreateMedicalVocabulary`'s backend signature didn't even have a
place to put a tags parameter (positional-string-args method).

Separately, `StartTranscriptionJob`/`StartCallAnalyticsJob`/`StartMedicalScribeJob`/
`StartMedicalTranscriptionJob` *did* already thread `Tags` into the stored job struct
(so Get/List job calls that echo `Tags` directly worked), but never synced into the
ARN-keyed `resourceTags` map used by `TagResource`/`ListTagsForResource` — so
`ListTagsForResource(jobArn)` right after `StartTranscriptionJob(..., Tags: {...})`
returned empty, another AWS-behavior mismatch (this exact "Tags aren't synced from
Start to ListTagsForResource" pattern is systemic across other gopherstack services
too, e.g. services/comprehend — flagged as a cross-service follow-up in `gaps`, not
fixed outside transcribe per this task's scope).

Fixed by:
- Adding `Tags map[string]string` fields to the `Vocabulary`, `VocabularyFilter`,
  `LanguageModel`, `CallAnalyticsCategory`, and `MedicalVocabulary` backend structs.
- Adding a `tags map[string]string` parameter to `CreateMedicalVocabulary`'s backend
  signature (`StorageBackend` interface + `InMemoryBackend` + the 2 call sites in
  persistence_test.go/handler_test.go that called it positionally).
- Threading `Tags` through the 5 affected request-input structs in handler.go
  (`createVocabularyInput`, `createVocabularyFilterInput`,
  `createMedicalVocabularyInput`, `createLanguageModelInput`,
  `createCallAnalyticsCategoryInput`).
- Adding `resourceARN(resourceType, name string) string` (using `pkgs/arn.Build` +
  `pkgs/config.DefaultRegion`) with resource-type segments confirmed against the real
  SDK's `TagResource`/`ListTagsForResource`/`UntagResource` doc-comment example
  (`arn:aws:transcribe:us-west-2:111122223333:transcription-job/transcription-job-name`)
  for `transcription-job`, and AWS's standard IAM-policy resource-ARN naming
  convention for the other eight types (`call-analytics-job`,
  `call-analytics-category`, `medical-scribe-job`, `medical-transcription-job`,
  `vocabulary`, `vocabulary-filter`, `medical-vocabulary`, `language-model`).
- `recordResourceTagsLocked`/`forgetResourceTagsLocked` helpers in backend.go, called
  (while already holding `b.mu`) after every successful Create/Start of a taggable
  resource, and on every Delete (so a deleted resource's tags don't linger and get
  returned by `ListTagsForResource` for an ARN that no longer maps to anything).

Regression tests: `TestBackend_CreationTags_SyncToResourceARN` (table-driven, all 9
taggable creation ops) and `TestBackend_Delete_ForgetsResourceTags` /
`TestBackend_CreationWithoutTags_LeavesResourceTagsEmpty` in the new backend_test.go.

### Bug found and fixed #3 (2026-07-24 sweep) — NameContains filters, LanguageIdSettings, invented output fields, thin summary/output shapes

This pass re-field-diffed every op against `aws-sdk-go-v2/service/transcribe@v1.55.0`'s
generated `api_op_*.go`/`types.go` (not just the previously-audited output timestamp/tag
issues) and found several real, previously-unnoticed wire-shape gaps:

1. **NameContains/JobNameContains completely unimplemented** on all 7 List ops that
   document it (`ListVocabularies`, `ListMedicalVocabularies`, `ListVocabularyFilters`,
   `ListTranscriptionJobs`, `ListMedicalTranscriptionJobs`, `ListMedicalScribeJobs`,
   `ListCallAnalyticsJobs`, `ListLanguageModels`) — a real client filtering by name
   substring silently got back the full unfiltered list. Fixed with a shared
   `matchesNameContains` helper (`store.go`, case-insensitive substring per AWS's "the
   search is not case sensitive" doc wording) threaded through every backend `List*`
   method, the `StorageBackend` interface, and every list handler's input struct.

2. **LanguageIdSettings entirely missing** — real `StartTranscriptionJobInput` and the
   `TranscriptionJob` response both carry a `LanguageIdSettings
   map[string]LanguageIdSettings` field (per-language custom vocabulary/model/filter
   selection under `IdentifyLanguage`/`IdentifyMultipleLanguages`), explicitly called
   out for verification in this pass's task brief. gopherstack had no such field
   anywhere — not in the input struct, not on the backend `TranscriptionJob`, not
   echoed. Added end-to-end (`LanguageIDSettings map[string]LanguageIDSettings` on the
   backend struct, threaded through `StartTranscriptionJob`'s input and
   `transcriptionJobOutput`).

3. **Invented top-level `OutputBucketName`/`OutputKey` response fields.** The real
   `TranscriptionJob`, `MedicalTranscriptionJob`, and `MedicalScribeJob` response types
   (confirmed against `types.go`) have **no** `OutputBucketName`/`OutputKey` fields at
   all — the output location is only ever surfaced via the nested
   `Transcript.TranscriptFileUri` (or `MedicalScribeOutput.*Uri`). gopherstack's three
   `Get*`/`Start*` wire-output structs echoed these back at the top level regardless —
   a gopherstack-invented field not present in the real SDK, per this task's hard
   constraint to delete such fields. Removed from all three output structs; the
   *backend* structs keep the fields (needed internally to compute the synthetic S3
   URIs), only the wire response was trimmed.

4. **`MedicalScribeJob` responses never included `MedicalScribeOutput`** — real AWS
   returns `MedicalScribeOutput{ClinicalDocumentUri, TranscriptFileUri}` once a job
   reaches `COMPLETED` (required fields on that type). gopherstack synthesized a
   transcript URI for every other job family (`Transcript.TranscriptFileUri`) but never
   did the equivalent for Medical Scribe jobs, meaning a client polling
   `GetMedicalScribeJob` on a completed job had no way to locate its output at all.
   Added `buildMedicalScribeOutputLocations`, synthesizing both URIs the same way
   `buildTranscriptURI`/`buildMedicalTranscriptURI` already do for the other job kinds.

5. **`ListMedicalScribeJobs`/`ListMedicalTranscriptionJobs` summary wire-shape
   deviation** (previously flagged `partial` in this manifest, not fixed) — both
   reused the full `Get*` output shape as their List summary, which is a strict
   superset of the real `MedicalScribeJobSummary`/`MedicalTranscriptionJobSummary`
   fields (leaking `Media`, `Settings`, `Tags`, `ChannelDefinitions`, etc.). Fixed by
   introducing dedicated `medicalScribeJobSummary`/`medicalTranscriptionJobSummary`
   wire types matching the real summary shapes field-for-field (including the
   previously-absent `OutputLocationType`/`ContentIdentificationType`/`Specialty`/
   `Type`/`StartTime` on the medical-transcription summary).

6. **Several thinner-than-real output shapes**, each missing real, documented response
   fields:
   - `TranscriptionJobSummary` was missing `StartTime`, `IdentifyLanguage`,
     `IdentifyMultipleLanguages`, `IdentifiedLanguageScore`, `ContentRedaction`,
     `ModelSettings`, `LanguageCodes`, `ToxicityDetection`, and `OutputLocationType`
     (added a `outputLocationType` helper deriving `CUSTOMER_BUCKET`/`SERVICE_BUCKET`
     from whether `OutputBucketName` was set, matching AWS's documented semantics).
   - `CallAnalyticsJobSummary` was missing `StartTime`.
   - `CategoryProperties` (Call Analytics category Create/Get/Update/List) was missing
     `CreateTime`, `LastUpdateTime`, and `Tags` entirely — real
     `CreateCallAnalyticsCategoryOutput`/etc. include all three.
   - `CreateVocabulary`/`CreateMedicalVocabulary` outputs were missing
     `LastModifiedTime` and `FailureReason`; `UpdateVocabulary`/`UpdateMedicalVocabulary`
     were missing `LastModifiedTime`; `GetVocabulary`/`GetMedicalVocabulary` were
     missing `FailureReason`.
   - `GetVocabularyFilterOutput` was missing **both** `DownloadUri` and
     `LastModifiedTime` — meaning a real client had no way to fetch a vocabulary
     filter's contents via `GetVocabularyFilter` at all, since gopherstack simply never
     returned the URI. `CreateVocabularyFilterOutput`/`UpdateVocabularyFilterOutput`/
     `VocabularyFilterInfo` (the `ListVocabularyFilters` element type) were all missing
     `LastModifiedTime`.
   - `ListVocabularies`/`ListMedicalVocabularies` were missing the top-level `Status`
     field (echoes the `StateEquals` request filter, per the real
     `ListVocabulariesOutput`/`ListMedicalVocabulariesOutput` shape).
   - `CreateLanguageModelOutput` was missing `InputDataConfig`; the `LanguageModel`
     type itself (and therefore `DescribeLanguageModel`/`ListLanguageModels`) was
     missing `FailureReason` — added the field to the backend struct and threaded it
     through (always empty in this synthetic backend, since models never fail, but the
     field must exist on the wire for real client unmarshaling to match the schema).

Regression tests (one file per family, table-driven, `t.Parallel()`, no shared
subtest state): `TestListTranscriptionJobs_JobNameContains`,
`TestTranscriptionJob_LanguageIdSettings_Echoed`,
`TestTranscriptionJob_OutputBucketNotInResponse`,
`TestListVocabularies_NameContains`,
`TestCreateVocabulary_LastModifiedTimeAndFailureReasonEchoed`,
`TestListVocabularies_EchoesStatusFilter`, `TestListVocabularyFilters_NameContains`,
`TestVocabularyFilter_LastModifiedTimeAndDownloadUri`,
`TestListMedicalVocabularies_NameContains`,
`TestMedicalVocabulary_LastModifiedTimeAndFailureReason`,
`TestListLanguageModels_NameContains`, `TestCreateLanguageModel_EchoesInputDataConfig`,
`TestCallAnalyticsCategory_CreateTimeAndLastUpdateTimeEchoed`,
`TestListCallAnalyticsJobs_JobNameContainsAndStartTime`,
`TestListMedicalScribeJobs_JobNameContainsAndSummaryShape`,
`TestMedicalScribeJob_OutputURIsPresentWhenCompleted`,
`TestListMedicalTranscriptionJobs_JobNameContainsAndSummaryShape`.

### Bug found and fixed #4 (gopherstack-6flj wrapper-key sweep, 2026-08-15) — nesting/never-modelled member gaps in Call Analytics and Medical Scribe

Scripted extraction of every List/Describe/Get op's real deserializer `case` keys
(19 ops) against `transcribe@v1.58.4`'s `deserializers.go`, plus every reachable
nested type, found four real, previously-undiscovered gaps beyond the wrapper-key
layer this issue otherwise targets:

1. **`VocabularyInfo.LastModifiedTime` never emitted** on `ListVocabularies` and
   `ListMedicalVocabularies` — both real ops share the exact same
   `types.VocabularyInfo` item type (`LanguageCode`, `LastModifiedTime`,
   `VocabularyName`, `VocabularyState`), confirmed at `api_op_ListVocabularies.go`
   and `api_op_ListMedicalVocabularies.go`, both typed `[]types.VocabularyInfo`.
   gopherstack's `vocabularySummary`/`medicalVocabularySummary` trimmed it out on
   both siblings identically. Fixed on both.

2. **`CallAnalyticsSettings.LanguageIdSettings` never modelled at all** (zero grep
   hits anywhere in the service, distinct from the already-fixed
   `TranscriptionJob`-level `LanguageIdSettings`) — confirmed real on
   `types.CallAnalyticsJobSettings` (`types/types.go:246`), request and response
   sides share the same `Settings *CallAnalyticsSettings` field passed by
   reference, so a real client's `StartCallAnalyticsJob` value was silently
   dropped and never echoed back. Added the field; no handler plumbing needed
   since `Settings` flows through unchanged.

3. **All four Call Analytics category rule filter types missing
   `AbsoluteTimeRange`/`RelativeTimeRange`** — `NonTalkTimeFilter`,
   `InterruptionFilter`, `TranscriptFilter`, and `SentimentFilter` each carry both
   sub-parameters on the real SDK (`types/types.go:1428,557,1865,1571`), none
   modelled. Added both shared types (`AbsoluteTimeRange`, `RelativeTimeRange`)
   and the fields on all four filters; `CallAnalyticsRule` is reused directly as
   the wire type both directions, so no handler plumbing needed.

   Also confirmed, while diffing: `NonTalkTimeFilter.ParticipantRole` in
   gopherstack is an **extra** field the real `types.NonTalkTimeFilter` does not
   have (unlike its three siblings, which do carry `ParticipantRole`) — harmless
   over-modeling, not reachable by a real client (an unrecognized request field
   is simply not settable on the real typed input, and the real response
   deserializer for this specific filter has no case for it), left in place
   rather than risk breaking `TestCreateCallAnalyticsCategory_Rules` for a
   cosmetic cleanup. See "looks-wrong-but-correct traps" below.

4. **`ClinicalNoteGenerationSettings` emitted/accepted at the wrong nesting
   level** (the flagship find this pass) — see the `StartMedicalScribeJob`/
   `GetMedicalScribeJob` ops notes above. This is the exact "nested shape emitted
   flat" bug class this issue calls out as hardest to find: the wrapper key name
   gopherstack used (`ClinicalNoteGenerationSettings`) was spelled correctly and
   present in both request and response, so a naming diff alone would have
   missed it — only comparing which *level* of the object graph carried the key
   surfaced it. Confirmed via the real deserializer's `default: _, _ = key, value`
   case (silent skip of unrecognized top-level keys, not a hard error) that this
   was silent-empty in both directions, not a client-visible crash.

Regression tests (real `aws-sdk-go-v2/service/transcribe` client through the full
router, `wire_field_fixes_test.go`): `TestListVocabularies_LastModifiedTime_RealClient`,
`TestListMedicalVocabularies_LastModifiedTime_RealClient`,
`TestCallAnalyticsSettings_LanguageIdSettings_RealClient`,
`TestCallAnalyticsRule_TimeRanges_RealClient`,
`TestMedicalScribeSettings_ClinicalNoteGenerationSettings_RealClient`. Each fix was
hand-reverted individually, confirmed to fail with the exact predicted symptom
(a nil/missing round-tripped value, not a decode error — awsjson1.1 tolerates
unknown fields), then restored byte-identical (`git diff` compared against the
pre-revert hunks).

Both `CallAnalyticsJobDetails`/`Skipped` and `MedicalScribeContext`/
`MedicalScribeContextProvided` (see `gaps:` below) were re-confirmed still
genuinely unimplemented this pass — not fixed, since both require modeling new
backend-tracked concepts (skipped-analytics-feature reporting; patient-context
input) with no existing data source, out of scope for a wire-shape sweep.

### Bug found and fixed #5 (2026-09-05 required-member sweep) — six required-member and cross-field validation gaps

Re-read every relevant `validateOp*`/nested `validate*` function in
`transcribe@v1.58.4/validators.go` literally (per this pass's mandate: a
validator testing `== nil` does not reject an empty non-nil value, and a
required member with no corresponding gopherstack check lets a convenience
default slip through where the real API rejects). Found six real gaps, all
of the same class — a documented "this member is required" or "if X then Y
is also required" constraint that gopherstack either didn't check at all or
only checked in one direction:

1. **`ContentRedaction.RedactionOutput`** is required
   (`validateContentRedaction`, validators.go:851) but gopherstack's
   `validateContentRedaction` (transcription_jobs.go) only validated its enum
   value when non-empty, never rejecting an absent one. A client setting
   `ContentRedaction.RedactionType` without `RedactionOutput` got HTTP 200.

2. **`StartMedicalTranscriptionJob.LanguageCode`** must be `en-US` only
   ("US English (en-US) is the only valid value for medical transcription
   jobs. Any other value you enter for language code results in a
   BadRequestException error," api_op_StartMedicalTranscriptionJob.go) —
   gopherstack validated it against the full 75-code Transcribe language
   allowlist shared with `StartTranscriptionJob` instead, so e.g. `fr-FR` was
   silently accepted.

3. **`CreateLanguageModel.InputDataConfig`** is a required top-level member
   (validators.go:1102, `validateOpCreateLanguageModelInput`) but
   gopherstack's `CreateLanguageModel` only validated `S3Uri`/
   `DataAccessRoleArn` *inside* `InputDataConfig` when the pointer was
   non-nil, never rejecting a request that omitted `InputDataConfig`
   entirely.

4. **`StartMedicalScribeJob.Settings`** was entirely unvalidated for its
   documented cross-field constraints (api_op_StartMedicalScribeJob.go: "a
   MedicalScribeSettings object that must set exactly one of
   ShowSpeakerLabels or ChannelIdentification to true. If ShowSpeakerLabels
   is true, MaxSpeakerLabels must also be set" and "ChannelDefinitions...
   should be set if and only if the ChannelIdentification value of Settings
   is set to true") — `Settings` itself wasn't even required
   (validators.go:1520 requires it), so a request with neither flag set,
   both flags set, `ShowSpeakerLabels` without `MaxSpeakerLabels`, or
   `ChannelIdentification` without matching `ChannelDefinitions` all got
   HTTP 200 with the mismatch silently ignored. This is the flagship find
   this pass: it is the same "documented mutual-exclusivity constraint,
   unenforced" bug class as sagemakerruntime's `Body`/`InputLocation`.
   `MedicalScribeChannelDefinition.ParticipantRole` (required,
   `PATIENT`/`CLINICIAN`, types.go:821) was also unvalidated.

5. **`StartTranscriptionJob.Settings`**' `ShowSpeakerLabels`/
   `MaxSpeakerLabels` and `ShowAlternatives`/`MaxAlternatives` pairs have the
   identical bidirectional "must be set together" doc (types.go `Settings`:
   "If you specify the MaxSpeakerLabels field, you must set the
   ShowSpeakerLabels field to true" / "If you enable ShowSpeakerLabels in
   your request, you must also include MaxSpeakerLabels", and the same
   pattern for `ShowAlternatives`/`MaxAlternatives`). gopherstack only
   bounds-checked the Max* field when both were non-zero/true, silently
   accepting either field set alone.

6. **`CreateCallAnalyticsCategory`/`UpdateCallAnalyticsCategory.Rules`** is a
   required member (validators.go:1075,1665) bounded to 1-20 entries ("you
   must create between 1 and 20 rules," api_op_CreateCallAnalyticsCategory.go)
   — gopherstack validated neither presence nor count, and never checked a
   populated `TranscriptFilter`'s (`Targets`/`TranscriptFilterType`, both
   required, types.go:1865) or `SentimentFilter`'s (`Sentiments`, required,
   types.go:1571) own required sub-fields.

Fixed all six by adding the missing required/paired checks directly in each
op's existing validation path (`transcription_jobs.go`,
`medical_transcription_jobs.go`, `language_models.go`, `medical_scribe.go`,
`call_analytics.go`); no wire-shape or persistence changes were needed since
these are pure input-validation gaps.

Regression tests (table-driven subtests, one per gap, in the existing
per-op `_test.go` files):
`TestContentRedaction_Validation/missing_redaction_output_rejected`,
`TestStartMedicalTranscriptionJob_SpecialtyType/non_en_us_language_code_rejected`,
`TestCreateLanguageModel_InputDataConfig/input_data_config_required`,
`TestStartMedicalScribeJob_RequiredFields/{missing_settings_rejected,
both_speaker_labels_and_channel_identification_rejected,
neither_speaker_labels_nor_channel_identification_rejected,
show_speaker_labels_without_max_speaker_labels_rejected,
channel_identification_without_channel_definitions_rejected}`,
`TestSettings_Validation/{show_speaker_labels_without_max_speaker_labels_rejected,
max_speaker_labels_without_show_speaker_labels_rejected,
show_alternatives_without_max_alternatives_rejected,
max_alternatives_without_show_alternatives_rejected}`,
`TestCreateCallAnalyticsCategory_Rules/{missing_rules_rejected,
empty_rules_rejected,too_many_rules_rejected,
transcript_filter_missing_targets_rejected,
sentiment_filter_missing_sentiments_rejected}`. Each guard was neutered
individually (fix line reverted to the pre-change content, confirmed the
targeted subtest fails with the predicted "expected error but got nil"
symptom, then restored byte-identical) before being trusted as a real fix.

A wide set of pre-existing tests across `medical_scribe_test.go`,
`call_analytics_test.go`, `language_models_test.go`, `handler_test.go`,
`tags_test.go`, `persistence_test.go`, `wire_field_fixes_test.go`, and
`wire_field_fixes_g8k9_test.go` previously exercised
`StartMedicalScribeJob`/`CreateLanguageModel`/`CreateCallAnalyticsCategory`
with fixtures that omitted these now-required fields (relying on the old
convenience-default behavior incidentally, not asserting it as a documented
contract) — all were updated to supply valid values, since their actual
test intent (tag sync, snapshot round-trip, timestamp shape, summary
trimming, etc.) is unrelated to the fields now being enforced.

### Looks-wrong-but-correct traps (don't re-flag)

- `NonTalkTimeFilter.ParticipantRole` (`models.go`) is a gopherstack-only extra
  field the real `types.NonTalkTimeFilter` does not carry (its three siblings —
  `InterruptionFilter`/`TranscriptFilter`/`SentimentFilter` — genuinely do have
  `ParticipantRole`). Harmless: a real client cannot set it (not in the typed
  input) and the real deserializer has no case for it on this specific filter, so
  it's simply unreachable, not a leak or a correctness bug. Left in place
  (gopherstack-6flj, 2026-08-15) rather than risk an unrelated test break for a
  cosmetic removal.

- `ErrVocabularyNotFound` (GetVocabulary's "not found" path) deliberately maps to
  `BadRequestException` (400), not `NotFoundException` (404) — this is intentional,
  documented AWS behavior for missing vocabularies specifically, per the comment on
  `ErrVocabularyNotFound` in backend.go. Every other resource kind's "not found"
  correctly maps to `NotFoundException`.
- `StartTranscriptionJob` with `JobExecutionSettings.AllowDeferredExecution=true`
  intentionally starts a job in `QUEUED` and advances it one state per
  `GetTranscriptionJob` poll (`QUEUED` → `IN_PROGRESS` → `COMPLETED`) via
  `advanceDeferredTranscriptionJob` — this is a deliberate state-machine simulating
  deferred execution, not a stuck/no-op job. All other Start*Job paths complete
  synchronously (no real ASR, but a deterministic mock transcript is generated and the
  job lands directly in `COMPLETED`), which is correct per this audit's scope (mock
  transcript content is acceptable; only lifecycle/wire/error/routing bugs are in
  scope).
- `paginateList`'s `nextToken` is a plain string-encoded integer offset. This is fine:
  real AWS clients never parse `NextToken` — it's opaque by contract — so this doesn't
  need to match any particular AWS-internal format.
