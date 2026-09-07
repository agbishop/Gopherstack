---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: comprehend
sdk_module: aws-sdk-go-v2/service/comprehend@v1.43.4
last_audit_commit: cb5dac6ff
last_audit_date: 2026-08-29
overall: A            # 2026-08-29 (later same day, wrapper-key/constraint-parameter sweep): the
                      # enum-VALUE pass below claimed Filter support was "NEW" and complete across
                      # every List*Jobs/List<Resource>s op -- it was, except ListFlywheelIterationHistory,
                      # whose own dedicated listIterations handler (unlike every other List op, which
                      # routes through the shared listJobs/listResources generic functions) never checked
                      # Filter at all. Fixed -- see StartFlywheelIteration/.../ListFlywheelIterationHistory
                      # row below.
                      # 2026-08-29: enum-VALUE sweep (no bug-class checklist used --
                      # deliberately compared from first principles against the pinned SDK
                      # instead of the campaign's known bug classes). Found and fixed FOUR
                      # real bugs the prior wrapper-key/nested-shape sweeps missed, none of
                      # which cmd/enumcheck (the purpose-built static enum-value checker)
                      # flags even now, because the wrong value flows through a struct field
                      # (Resource.Status) rather than a literal at the map-key call site --
                      # confirmed by re-running enumcheck against the pre-fix code via a
                      # scoped `git stash`, see wire_sdk_roundtrip_test.go's new tests:
                      # (1) EndpointProperties.Status was the invented literal "ACTIVE" --
                      # real types.EndpointStatus (enums.go:248-256) has no such value, only
                      # IN_SERVICE/CREATING/UPDATING/DELETING/FAILED. A real client's
                      # waiter (Status == types.EndpointStatusInService) would never fire.
                      # (2) FlywheelProperties.Status was the invented literal "READY" --
                      # real types.FlywheelStatus (enums.go:352-360) has no such value; the
                      # correct steady-state value is ACTIVE.
                      # (3) DatasetProperties.Status was also the invented literal "READY" --
                      # real types.DatasetStatus (enums.go:63-69) has only
                      # CREATING/COMPLETED/FAILED; DatasetProperties.Status's own doc comment
                      # is explicit ("the status changes to COMPLETED").
                      # (4) FlywheelIterationProperties.Status was emitted under the WRONG
                      # WIRE KEY entirely ("FlywheelIterationStatus", not "Status" --
                      # confirmed against awsAwsjson11_deserializeDocumentFlywheelIterationProperties,
                      # deserializers.go:16022, whose switch has no "FlywheelIterationStatus"
                      # case at all) AND the value progression used the wrong enum
                      # (SUBMITTED->IN_PROGRESS->COMPLETED, the JobStatus vocabulary) instead
                      # of the real TRAINING->EVALUATING->COMPLETED/FAILED/STOP_REQUESTED/
                      # STOPPED (types.FlywheelIterationStatus, enums.go:325-334) --
                      # FlywheelIterationStatus does not share JobStatus's vocabulary even
                      # though both happen to use the generic SUBMITTED/IN_PROGRESS words for
                      # some other enums in this same service (ModelStatus is a fifth,
                      # different vocabulary again -- see Notes). Root cause common to all
                      # four: this service invented one generic SUBMITTED/IN_PROGRESS/
                      # COMPLETED/FAILED status vocabulary and reused it for every
                      # status-shaped field, without checking that each real AWS enum type
                      # (EndpointStatus/FlywheelStatus/DatasetStatus/FlywheelIterationStatus/
                      # ModelStatus/JobStatus) has ITS OWN distinct string values -- only
                      # JobStatus (used correctly for the 9 async detection-job families)
                      # actually matches that vocabulary; the other five don't.
                      # Fifth, related but UNREACHABLE finding left unfixed (see Notes):
                      # ModelStatus (DocumentClassifier/EntityRecognizerProperties.Status)
                      # is ALSO wrong in the same way (IN_PROGRESS/FAILED instead of real
                      # TRAINING/IN_ERROR) but initialResourceStatus always fast-forwards
                      # these two resource types straight to TRAINED, so the wrong
                      # intermediate values can never actually reach a client today -- flagged
                      # as a landmine for if that fast-forward is ever removed, not fixed as
                      # live code.
                      # Sixth bug fixed: EndpointProperties.CurrentInferenceUnits (a real
                      # member, types.go:1230-1284) was never populated at all -- resourceMap
                      # only ever echoed the request's own DesiredInferenceUnits key back
                      # verbatim under its own name. Seventh: UpdateEndpoint's
                      # DesiredModelArn/DesiredDataAccessRoleArn were stored as brand-new,
                      # never-reconciled Configuration keys by UpdateResource's generic
                      # maps.Copy, so a real model swap via UpdateEndpoint left the ORIGINAL
                      # (stale) ModelArn/DataAccessRoleArn in every subsequent
                      # Describe/List response forever, alongside a permanent phantom
                      # "Desired*" pending-update pair matching nothing actually in
                      # progress. Both fixed via new applyEndpointConvergence()
                      # (handler_resources.go), matching this service's existing
                      # fast-forward-to-terminal-state pattern (no async update lag modeled
                      # anywhere else in this service either).
                      #
                      # 2026-08-20: wrapper-key/nested-shape sweep. Two real bugs fixed:
                      # (1) detectTargetedSentiment built each types.TargetedSentimentEntity
                      # with Text/Score/BeginOffset/EndOffset/Type hung directly off the
                      # entity root (via matchResult()) -- those five fields don't exist on
                      # TargetedSentimentEntity at all (it has only DescriptiveMentionIndex+
                      # Mentions per types/types.go:2799); they belong one level down, on
                      # each types.TargetedSentimentMention inside Mentions. Also missing:
                      # DescriptiveMentionIndex was never populated. Harmless to a real client
                      # (awsAwsjson11_deserializeDocumentTargetedSentimentEntity's default
                      # case silently drops unrecognized keys, deserializers.go:20064) but a
                      # genuine wire-shape divergence affecting both DetectTargetedSentiment
                      # and BatchDetectTargetedSentiment (same detector, reused via batch()).
                      # (2) classifierMetadata()/recognizerMetadata() emitted fabricated
                      # MicroPrecision/MicroRecall/Precision/Recall only, types/types.go
                      # ~line 2100s) or on the top-level types.EntityRecognizerEvaluationMetrics
                      # service already modeled correctly. An existing test
                      # (TestDocumentClassifierMetadataPresentWhenTrained) asserted the
                      # fabricated fields were present -- corrected, not just the source.
                      # See services/comprehend/wire_sdk_roundtrip_test.go for round-trip
                      # proof of both fixes and PARITY.md Notes below for detail. Provenance
                      # note: the prior stamp (last_audit_commit 2d47b51d4, dated 2026-07-29)
                      # predated last_audit_date (2026-08-13) by ~2 weeks -- the actual
                      # 2026-08-13 content landed in 69bbb940a (2026-08-15); refreshed to
                      # current HEAD so the pair is self-consistent again.
                      # 2026-08-13: closed gopherstack-wl0s (required-presence validation):
                      # CreateFlywheel's DataAccessRoleArn/DataLakeS3Uri and CreateEndpoint's
                      # DesiredInferenceUnits were stored and echoed via the generic-CRUD
                      # CreateResource passthrough but never required present. DataAccessRoleArn
                      # is fixed even though the originating audit named only DataLakeS3Uri/
                      # DesiredInferenceUnits -- it's required by validateOpCreateFlywheelInput
                      # too. See "Required-presence validation on CreateFlywheel/CreateEndpoint"
                      # note below.
                      # 2026-07-29: fabricated op family deleted, wire-shape/error-code bugs fixed, prior gaps closed
                      # 2026-07-31: pkgs/sdkcheck reverse check found five more phantoms this pass missed: BatchDetectPiiEntities (no Batch form of PII detection exists at all), DeleteDataset (datasets are immutable -- no real Delete op), GetFlywheelIteration (fabricated alias for the real DescribeFlywheelIteration, which was already correctly wired), StopDocumentClassificationJob and StopTopicsDetectionJob (2 of the 9 async job families have no real Stop op). All five were generated unintentionally by this service's generic CRUD/job-family builders (buildOperations/asyncJobSpecs/resourceSpecs) applying a uniform op set to families that are NOT uniform in the real API. Fixed via new jobSpec.noStop/resourceSpec.noDelete flags (see handler.go/handler_jobs.go/handler_resources.go) rather than hardcoded exclusion lists, so future job/resource families default to the correct (non-uniform) op set. GetFlywheelIteration's row below and the BatchDetect*/Stop*DetectionJob wildcard rows previously implied uniformity that did not exist; corrected. Grade held at A: all five are unreachable by real clients regardless (Comprehend dispatches by X-Amz-Target), and the routes/backend methods are harmless generic-factory reuse, not one-off invented logic.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  DetectSentiment: {wire: ok, errors: ok, state: ok, persist: n/a, note: "synchronous, deterministic word-list mock is acceptable; LanguageCode now required+validated (12-lang enum), Text now enforces the real 5KB limit -> TextSizeLimitExceededException"}
  DetectEntities: {wire: ok, errors: ok, state: ok, persist: n/a, note: "LanguageCode correctly optional (EndpointArn alternative per real API) but format-validated when supplied; Text enforces 100KB limit"}
  DetectKeyPhrases: {wire: ok, errors: ok, state: ok, persist: n/a, note: "LanguageCode required+validated; Text enforces 100KB limit"}
  DetectPiiEntities: {wire: ok, errors: ok, state: ok, persist: n/a, note: "LanguageCode required+validated; Text enforces 100KB limit"}
  DetectSyntax: {wire: ok, errors: ok, state: ok, persist: n/a, note: "LanguageCode validated against the narrower 6-value SyntaxLanguageCode enum (types.LanguageCode's 12 values do NOT all apply here); Text enforces 5KB limit"}
  DetectDominantLanguage: {wire: ok, errors: ok, state: ok, persist: n/a, note: "correctly has no LanguageCode field; Text enforces 100KB limit"}
  DetectToxicContent: {wire: ok, errors: ok, state: ok, persist: n/a, note: "ResultList/Labels/Toxicity field names verified against types.ToxicLabels; LanguageCode required+English-only per real doc comment despite the general enum type; TextSegments now enforces 1KB-per-segment/10KB-total"}
  DetectTargetedSentiment: {wire: ok, errors: ok, state: ok, persist: n/a, note: "LanguageCode required+English-only per real doc comment; Text enforces 5KB limit. FIXED 2026-08-20: TargetedSentimentEntity previously carried Text/Score/BeginOffset/EndOffset/Type at the wrong nesting level (entity root instead of nested inside Mentions[]) and never populated DescriptiveMentionIndex -- see header note. Also fixes BatchDetectTargetedSentiment, which reuses the same detector."}
  ClassifyDocument: {wire: ok, errors: ok, state: ok, persist: n/a, note: "correctly has no LanguageCode field; Text enforces 100KB limit"}
  ContainsPiiEntities: {wire: ok, errors: ok, state: ok, persist: n/a, note: "LanguageCode required+validated; Text enforces 100KB limit"}
  BatchDetect-family (Sentiment/Entities/KeyPhrases/Syntax/DominantLanguage/TargetedSentiment -- 6 families excluding PiiEntities): {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED: TextList>25 items now rejected whole-request with BatchSizeLimitExceededException (was silently accepted); per-item >5KB now becomes a BatchItemError entry (ErrorCode/ErrorMessage/Index) in ErrorList instead of being ignored, matching every Batch*Output doc comment's 'if there are no errors in the batch, the ErrorList is empty' partial-failure semantics; shared LanguageCode validated once per request against the correct per-op allowed set (BatchDetectSyntax: 6-lang, BatchDetectTargetedSentiment: English-only, others: 12-lang). 2026-07-31 CORRECTION: this row's \"BatchDetect*\" wildcard previously implied all Detect* ops have a Batch form -- PiiEntities does not (no BatchDetectPiiEntities on the real SDK client at all); a prior pass had fabricated it, now removed (see header note)."}
  StartDetectionJob-family (9 families): {wire: ok, errors: ok, state: ok, persist: ok, note: "Tags correctly seed b.tags[JobArn] (prior fix, re-verified); NEW this pass: TooManyTagsException (>50 initial tags) and KmsKeyValidationException (malformed VolumeKmsKeyId) enforced before job creation"}
  DescribeDetectionJob-family: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED wire-shape bug: per-family *Properties field sets now field-diffed individually (see jobSpec/jobMap) -- e.g. DocumentClassificationJobProperties carries FlywheelArn+VolumeKmsKeyId+VpcConfig but NO LanguageCode, PiiEntitiesDetectionJobProperties carries Mode+RedactionConfig but NO VolumeKmsKeyId/VpcConfig, TopicsDetectionJobProperties carries NumberOfTopics but NO LanguageCode; previously every family emitted the SAME field set regardless of its real shape. FIXED error-code bug: job-not-found now returns JobNotFoundException, not ResourceNotFoundException (confirmed against every awsAwsjson11_deserializeOpErrorDescribe*Job case in the SDK's deserializers.go). FIXED field-name bug: failure description field is 'Message' on every real *Properties shape, not 'FailureReason' (no such field exists on any of them -- a failed job's description was previously always lost on the wire). NEW: Filter (JobName/JobStatus/SubmitTimeBefore/SubmitTimeAfter) now supported on List*Jobs, previously ignored entirely."}
  ListDetectionJobs-family: {wire: ok, errors: ok, state: ok, persist: ok, note: "see Describe*DetectionJob for the per-family field-set fix and new Filter support"}
  StopDetectionJob-family (7 of 9 families -- NOT DocumentClassificationJob or TopicsDetectionJob): {wire: ok, errors: ok, state: ok, persist: ok, note: "correctly rejects stop on terminal states with InvalidRequestException; not-found now JobNotFoundException (see Describe*DetectionJob). 2026-07-31 CORRECTION: this row's wildcard previously implied all 9 job families have a Stop op -- 2 do not (StopDocumentClassificationJob/StopTopicsDetectionJob do not exist on the real SDK client); a prior pass's generic job-family builder had fabricated them uniformly, now excluded via jobSpec.noStop (see header note)."}
  CreateDocumentClassifier/CreateEntityRecognizer: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED: deleted the fabricated CreateDocumentClassifierVersion/CreateEntityRecognizerVersion op family -- no such operations exist in the real SDK (confirmed: no matching api_op_*.go files); a new version is created by calling these SAME ops again with the same name and a new VersionName, which they already supported generically. NEW: TooManyTagsException/KmsKeyValidationException (ModelKmsKeyId) enforced; DocumentClassifierProperties/EntityRecognizerProperties now populate TrainingStartTime/TrainingEndTime/ClassifierMetadata/RecognizerMetadata (deterministic synthetic values, only once status=TRAINED, matching real semantics) -- closes last pass's documented gap. 2026-08-20: a sweep proposed removing F1Score/MicroF1Score from ClassifierMetadata and F1Score from RecognizerMetadata's top-level EvaluationMetrics as fabricated. That was WRONG and was reverted before commit -- types.ClassifierEvaluationMetrics really does carry Accuracy/F1Score/HammingLoss/MicroF1Score/MicroPrecision/MicroRecall/Precision/Recall, and types.EntityRecognizerEvaluationMetrics really does carry F1Score/Precision/Recall, identical to the per-entity-type types.EntityTypesEvaluationMetrics. The existing shared helper and the existing test were correct as they stood."}
  DescribeDocumentClassifier/DescribeEntityRecognizer: {wire: ok, errors: ok, state: ok, persist: ok, note: "SubmitTime/EndTime field names correct; see CreateDocumentClassifier/CreateEntityRecognizer for the removed fabricated Version ops and new metadata fields"}
  ListDocumentClassifiers/ListEntityRecognizers: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW: Filter (Name/Status/SubmitTimeBefore/SubmitTimeAfter) now supported, previously ignored entirely"}
  DeleteDocumentClassifier/DeleteEntityRecognizer: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateEndpoint/DescribeEndpoint/ListEndpoints/UpdateEndpoint/DeleteEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "CreationTime/LastModifiedTime correct (prior fix, re-verified); NEW: ListEndpoints Filter (ModelArn/Status/CreationTimeBefore/CreationTimeAfter) now supported. 2026-08-13 (gopherstack-wl0s): DesiredInferenceUnits now required present (requiredResourceFields, store.go). FIXED 2026-08-29: Status was the invented literal ACTIVE (real types.EndpointStatus has no such value -- IN_SERVICE is correct); CurrentInferenceUnits was never populated; UpdateEndpoint's DesiredModelArn/DesiredDataAccessRoleArn never converged onto ModelArn/DataAccessRoleArn -- see header note and applyEndpointConvergence (handler_resources.go)."}
  CreateFlywheel/DescribeFlywheel/ListFlywheels/UpdateFlywheel/DeleteFlywheel: {wire: ok, errors: ok, state: ok, persist: ok, note: "CreationTime/LastModifiedTime + FlywheelSummaryList list-wrapper correct (prior fixes, re-verified); ListFlywheels Filter (Status/CreationTimeBefore/CreationTimeAfter) supported (prior pass). FIXED this pass (gopherstack-sw2q): CreateFlywheelInput.DataSecurityConfig (confirmed against types.DataSecurityConfig -- the ONLY Create*/resource op whose input has this field; CreateDatasetInput has no DataSecurityConfig at all, a dataset inherits its flywheel's config) carries its own DataLakeKmsKeyId/ModelKmsKeyId/VolumeKmsKeyId, independent of and previously unchecked by this op's top-level KMS validation -- now validated via validateDataSecurityConfigKmsKeys (store.go), raising KmsKeyValidationException for a malformed value in any of the three. 2026-08-13 (gopherstack-wl0s): DataAccessRoleArn/DataLakeS3Uri now required present (requiredResourceFields, store.go) -- DataAccessRoleArn wasn't named by the originating audit but is required too. FIXED 2026-08-29: Status was the invented literal READY (real types.FlywheelStatus has no such value -- ACTIVE is correct); see header note."}
  CreateDataset/DescribeDataset/ListDatasets: {wire: ok, errors: ok, state: ok, persist: ok, note: "CreationTime/EndTime correct (prior fix, re-verified); NEW: ListDatasets Filter (DatasetType/Status/CreationTimeBefore/CreationTimeAfter) now supported. This row deliberately excludes Delete: real Comprehend has no DeleteDataset operation at all (datasets are immutable once created). 2026-07-31: the code previously advertised/dispatched a fabricated \"DeleteDataset\" op contradicting this row's own scope -- fixed via resourceSpec.noDelete (see header note); TestResourceCRUDAndTags' dataset case updated to assert persistence instead of exercising the fabricated delete. FIXED 2026-08-29: Status was the invented literal READY (real types.DatasetStatus has no such value -- COMPLETED is correct, per that field's own doc comment); see header note."}
  StartFlywheelIteration/DescribeFlywheelIteration/ListFlywheelIterationHistory: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-07-31 CORRECTION: this row previously also listed \"GetFlywheelIteration\" as if it were a second real op -- it is not; the real SDK operation is DescribeFlywheelIteration only (no Client.GetFlywheelIteration). A prior pass registered both names against the same handler; \"GetFlywheelIteration\" was a fabricated alias, now removed (real name was already wired) -- see header note. FIXED 2026-08-29: FlywheelIterationProperties.Status was emitted under the wrong wire key (\"FlywheelIterationStatus\", real key is \"Status\") with the wrong enum vocabulary (SUBMITTED/IN_PROGRESS instead of real TRAINING/EVALUATING/COMPLETED) -- see header note. FIXED 2026-08-29 (wrapper-key/constraint sweep): ListFlywheelIterationHistoryInput.Filter (types.FlywheelIterationFilter: CreationTimeBefore/CreationTimeAfter, own doc comment api_op_ListFlywheelIterationHistory.go) was parsed by nothing at all -- listIterations (handler_flywheels.go) built its item list directly from the backend with no filter check, unlike every List*Jobs/List<Resource>s sibling which routes Filter through matchesJobFilter/matchesResourceFilter. Now applied via matchesIterationFilter, reusing filterTime's epoch-seconds decode. GAP (not fixed, disclosed below): EvaluatedModelArn/EvaluatedModelMetrics/EvaluationManifestS3Prefix/TrainedModelArn/TrainedModelMetrics remain unmodeled."}
  TagResource/UntagResource/ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "covers job ARNs too (prior fix); NEW: TagResource now enforces TooManyTagsException when the merged (existing+new) tag count would exceed 50"}
  ImportModel: {wire: ok, errors: ok, state: ok, persist: ok, note: "resourceType correctly derived from SourceModelArn (prior fix, re-verified)"}
  ListDocumentClassifierSummaries/ListEntityRecognizerSummaries: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED: now groups resources by Name into one summary row per distinct name with an aggregated NumberOfVersions and the most-recently-created resource as the 'latest version' -- previously emitted one row per stored resource with NumberOfVersions hardcoded to 1, which became visibly wrong once real multi-version classifiers/recognizers were reachable (see the fabricated-Version-op removal above)"}
  StopTrainingDocumentClassifier/StopTrainingEntityRecognizer: {wire: ok, errors: ok, state: ok, persist: ok}
  Put/Describe/DeleteResourcePolicy: {wire: ok, errors: ok, state: ok, persist: ok, note: "revision-conflict checked via ResourceInUseException, matches AWS optimistic-concurrency semantics"}
# Families audited as a group (when per-op is impractical):
families:
  routing: {status: ok, note: "RouteMatcher/ExtractOperation verified against X-Amz-Target: Comprehend_20171127.<Op> prefix; sdk_completeness_test.go confirms every SDK op is routed (no notImplemented entries needed) -- also re-confirms the deleted fabricated Version ops were never part of the real SDK surface this test checks against, so removing them didn't regress completeness"}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "2026-08-29: FlywheelIterationProperties is missing 5 of its 11 real members (EvaluatedModelArn/EvaluatedModelMetrics/EvaluationManifestS3Prefix/TrainedModelArn/TrainedModelMetrics -- confirmed against awsAwsjson11_deserializeDocumentFlywheelIterationProperties's own 11-case switch, deserializers.go:16022). Left unfixed deliberately: unlike ClassifierMetadata/RecognizerMetadata's synthetic accuracy NUMBERS (an established, precedented pattern in this file for a fake-but-plausible metric on a resource that genuinely exists), these five fields are mostly ARN IDENTIFIERS (EvaluatedModelArn/TrainedModelArn) pointing at a trained-model resource this emulator's flywheel-iteration flow never actually creates. Fabricating a plausible-looking model ARN with no backing resource risks becoming a NEW bug (a client that then calls DescribeDocumentClassifier/DescribeEntityRecognizer on that ARN gets a 404 that looks like data corruption, worse than the field being honestly absent). Fix requires either wiring iteration completion to actually create a backing model resource, or accepting the same kind of opaque-but-honest gap already on file for VpcConfig/RedactionConfig above -- a materially bigger unit of work than the Status wire-key/enum fix landed this pass, deferred rather than half-done."
  - "2026-08-29 LANDMINE (currently unreachable, not fixed as live code): DocumentClassifierProperties.Status/EntityRecognizerProperties.Status use the SAME wrong SUBMITTED/IN_PROGRESS/FAILED vocabulary as the three fixed bugs above -- real types.ModelStatus (types/enums.go:502-513) is SUBMITTED/TRAINING/DELETING/STOP_REQUESTED/STOPPED/IN_ERROR/TRAINED/TRAINED_WITH_WARNING, i.e. TRAINING not IN_PROGRESS and IN_ERROR not FAILED. advanceTrainingResource (store.go) still contains this wrong transition. It is UNREACHABLE today only because initialResourceStatus unconditionally fast-forwards resourceTypeDocClassifier/resourceTypeEntityRecognizer straight to TRAINED on create (CI-timeout workaround, intentional and documented elsewhere in this file) -- SUBMITTED/IN_PROGRESS/FAILED are dead states no code path can reach via the public API. Not fixed this pass because it changes zero observable client behavior today; flagged so the next person who removes or conditionalizes that fast-forward doesn't silently reintroduce a live wrong-enum bug."
  - "IMPOSSIBLE (re-confirmed gopherstack-sw2q): VpcConfig (types.VpcConfig: SecurityGroupIds+Subnets, both smithy-required) and RedactionConfig (types.RedactionConfig: MaskCharacter/MaskMode enum MASK|REPLACE_WITH_PII_ENTITY_TYPE/PiiEntityTypes) are passed through opaquely (whatever the caller sent, verbatim) rather than sub-field-validated. Diffed this pass against types.go: DataSecurityConfig's gap was a genuine, precedented one (three KMS key fields matching the exact validateKmsKeyID pattern already applied to top-level ModelKmsKeyId/VolumeKmsKeyId elsewhere) and is now FIXED (see CreateFlywheel). VpcConfig/RedactionConfig are different in kind: enforcing their required-member/enum shape would mean implementing generic smithy-required-field and enum validation for an arbitrary nested passthrough object with no existing precedent anywhere else in this service (or, per applicationautoscaling's PARITY.md, in the broader codebase's general philosophy of not over-validating optional nested sub-shapes). Wire-shape correctness of the echo itself is not at risk -- these fields are stored and echoed byte-for-byte unmodified, never renamed or restructured, so a real client round-trips exactly what it sent. Left as an honestly-documented gap, not implemented, to avoid inventing a new validation convention unilaterally."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "ALREADY COVERED BY CHAOS (verified gopherstack-sw2q): ResourceLimitExceededException/ResourceUnavailableException/TooManyRequestsException/ConcurrentModificationException are real modeled errors for several ops here (confirmed against deserializers.go's per-op error-case switches) but have no non-fabricated deterministic backend-state trigger in this emulator: no rate limiting is implemented anywhere in gopherstack per-service, no fixed per-account resource quota is documented precisely enough to emulate without risking false failures on legitimate high-volume test/integration usage, and ConcurrentModificationException describes a real-AWS eventual-consistency race that cannot occur under this backend's single coarse lock. Concretely verified this pass: comprehend.Handler implements ChaosServiceName() -> \"comprehend\" and ChaosOperations() -> h.GetSupportedOperations() (handler.go), and pkgs/chaos.Middleware is wired globally via registry.Use(chaos.Middleware(faultStore)) in cli.go, matching purely on the request's SigV4 service name + X-Amz-Target operation + region and injecting an arbitrary caller-specified FaultError{Code, StatusCode} without touching backend state. A fault rule such as {\"service\":\"comprehend\",\"error\":{\"code\":\"TooManyRequestsException\",\"statusCode\":429}} deterministically returns that exact typed error to a real aws-sdk-go-v2 client on any operation, with zero backend code changes -- proven end-to-end against a real containerized client in test/integration/chaos_test.go. Error-code wiring in errors.go/handler.go intentionally does not include backend sentinels for these four; the chaos mechanism is the correct, non-fabricated way to exercise them, not a backend-state workaround."
leaks: {status: clean, note: "no goroutines/timers spawned by this service; job/resource lifecycle advances synchronously on each Describe/List poll (advanceJob/advanceTrainingResource), no background janitor to leak. Confirmed unchanged this pass -- no new goroutines/timers were introduced by any of this pass's fixes."}
---

## Notes

Freeform: AWS-behavior specifics worth remembering (exact algorithms, wire quirks,
error-message text, protocol = query-XML / REST-XML / REST-JSON / json-1.0), and any
"looks-wrong-but-correct" traps so the next auditor doesn't re-flag them.

- Protocol is awsjson1.1 (`X-Amz-Target: Comprehend_20171127.<Op>`,
  `application/x-amz-json-1.1`). All error bodies use `{"__type": "<Code>", "message": "..."}`
  via `service.JSONErrorResponse`; HTTP status is 400 for every client-fault exception used
  here (all of `ResourceNotFoundException`/`ResourceInUseException`/`InvalidRequestException`/
  `JobNotFoundException`/`TooManyTagsException`/`BatchSizeLimitExceededException`/
  `TextSizeLimitExceededException`/`UnsupportedLanguageException`/`KmsKeyValidationException`
  have `smithy.ErrorFault() == FaultClient`; only `InternalServerException` is `FaultServer`
  and stays the unmapped 500 default) -- none of these carry an `@httpError` override to
  404/409/etc., so 400-for-everything-but-InternalServerException is correct for this
  protocol, not a bug.

- **A prior audit pass invented an entire fabricated resource-op family: "DocumentClassifierVersion"/
  "EntityRecognizerVersion".** `resourceSpecs()` had dedicated entries for these, generating 8
  operation names (`CreateDocumentClassifierVersion`, `DescribeDocumentClassifierVersion`,
  `ListDocumentClassifierVersions`, `DeleteDocumentClassifierVersion`, and the EntityRecognizer
  equivalents) that **do not exist in the real AWS SDK** -- confirmed by the complete absence of
  matching `api_op_*.go` files in `aws-sdk-go-v2/service/comprehend`. The real API creates a new
  version of an existing classifier/recognizer by calling `CreateDocumentClassifier`/
  `CreateEntityRecognizer` again with the SAME name and a new `VersionName`
  (`CreateDocumentClassifierInput.VersionName`/`CreateEntityRecognizerInput.VersionName` are both
  real, optional fields) -- there is no separate operation. `createResource()`'s generic handling
  already threaded `VersionName` through for every spec, so the base `"DocumentClassifier"`/
  `"EntityRecognizer"` entries in `resourceSpecs()` handle versioning correctly without a
  separate resource type; the fabricated entries and their now-orphaned `resourceTypeDocClassifierVersion`/
  `resourceTypeEntityRecognizerVer` constants have been deleted. **If you ever see a
  resourceSpecs()/asyncJobSpecs() entry whose op-name prefix has no matching real operation
  in the SDK's operation list, treat it as suspect and cross-check `api_op_*.go` before trusting it.**
  `sdkcheck.CheckCompleteness` (used by `sdk_completeness_test.go`) only checks for MISSING
  coverage of real ops, not for EXTRA fabricated ones -- it would never have caught this.

- **Async job `*Properties` shapes are NOT uniform across the 9 job families**, the same bug
  class as the resource-Properties timestamp-field split documented below, but affecting more
  fields: field-diffed against every real `*JobProperties` struct in
  `aws-sdk-go-v2/service/comprehend/types`, the split is:
  - `DocumentClassificationJobProperties`: `DocumentClassifierArn` + `FlywheelArn` +
    `VolumeKmsKeyId` + `VpcConfig`, but **no `LanguageCode`**.
  - `EntitiesDetectionJobProperties`: `EntityRecognizerArn` + `FlywheelArn` + `LanguageCode` +
    `VolumeKmsKeyId` + `VpcConfig`.
  - `KeyPhrasesDetectionJobProperties`/`SentimentDetectionJobProperties`/
    `TargetedSentimentDetectionJobProperties`: `LanguageCode` + `VolumeKmsKeyId` + `VpcConfig`,
    no classifier/recognizer/flywheel ARN fields.
  - `PiiEntitiesDetectionJobProperties`: `LanguageCode` + `Mode` + `RedactionConfig`, but
    **no `VolumeKmsKeyId`/`VpcConfig` at all**.
  - `TopicsDetectionJobProperties`: `NumberOfTopics` + `VolumeKmsKeyId` + `VpcConfig`, but
    **no `LanguageCode`**.
  - `DominantLanguageDetectionJobProperties`: `VolumeKmsKeyId` + `VpcConfig` only, **no
    `LanguageCode`** (it detects the language).
  - `EventsDetectionJobProperties`: `LanguageCode` + `TargetEventTypes`, no KMS/VPC fields.
  `jobSpec` (handler.go) now carries one bool per optional field
  (`hasLanguageCode`/`hasDocumentClassifierArn`/`hasEntityRecognizerArn`/`hasFlywheelArn`/
  `hasVolumeKmsKeyID`/`hasVpcConfig`/`hasTargetEventTypes`/`hasPiiMode`/`hasNumberOfTopics`),
  set per-family in `asyncJobSpecs()` (handler_jobs.go), and `jobMap()` gates each field on its
  flag. Previously every family emitted the SAME fixed field set (including e.g.
  `EntityRecognizerArn` and `LanguageCode` on `DocumentClassificationJobProperties`, which the
  real shape doesn't have) -- harmless-looking extra JSON keys the real SDK's client just
  ignores, but genuinely missing keys (like `FlywheelArn` on `EntitiesDetectionJobProperties`)
  left real fields permanently nil for any caller.

- **The failure-description field on every one of those 9 `*Properties` shapes is `Message`,
  not `FailureReason`.** No `FailureReason` field exists on any real `*JobProperties` struct.
  `jobMap()` previously emitted `"FailureReason": job.FailureReason`; a client unmarshalling a
  FAILED job's Describe/List response into the real SDK's generated struct would see this key
  simply dropped (no matching field), so `Message` was always nil -- the failure reason text was
  entirely unreachable from client code despite being computed correctly server-side. Fixed by
  changing the wire key only (the internal Go field `Job.FailureReason` is unchanged, it is
  purely a wire-key rename in `jobMap()`).

- **`Describe*DetectionJob`/`Stop*DetectionJob` return `JobNotFoundException` for an unknown
  job ID, not `ResourceNotFoundException`.** Confirmed against every
  `awsAwsjson11_deserializeOpErrorDescribe*Job`/`awsAwsjson11_deserializeOpErrorStop*Job` case
  in the SDK's generated `deserializers.go` -- `JobNotFoundException` is a distinct modeled
  exception used only by job Describe/Stop, while every resource family's Describe/Delete still
  correctly uses `ResourceNotFoundException`. `InMemoryBackend.DescribeJob`/`StopJob` now wrap
  `ErrJobNotFound` instead of `ErrNotFound`.

- **`List*Jobs`/`List<Resource>s` now support the `Filter` request field**, previously parsed
  and silently ignored entirely (only `NextToken`/`MaxResults` were read). Every job family's
  real `Filter` type (`JobFilter`/`SentimentDetectionJobFilter`/...) shares the same
  `JobName`/`JobStatus`/`SubmitTimeBefore`/`SubmitTimeAfter` shape (`matchesJobFilter` in
  handler_jobs.go). Resource family `Filter` types are NOT uniform (`matchesResourceFilter` in
  handler_resources.go): `DocumentClassifierFilter`/`EntityRecognizerFilter` key on
  name+`SubmitTime*`, `EndpointFilter` keys on `ModelArn`+`CreationTime*`,
  `FlywheelFilter`/`DatasetFilter` key on `CreationTime*` only (no name field), and
  `DatasetFilter` additionally has `DatasetType`. `SubmitTimeBefore`/`SubmitTimeAfter`/
  `CreationTimeBefore`/`CreationTimeAfter` arrive as epoch-seconds JSON numbers (same
  awsjson1.1 timestamp encoding as every other timestamp field here) -- `filterTime()` decodes
  them the same way `awstime.Epoch` encodes them on the way out.

- **`BatchDetect*` now enforces both real batch limits** (field-diffed from every
  `Batch*Input`/`Batch*Output` doc comment): `TextList` over 25 items is a whole-request
  `BatchSizeLimitExceededException` (nothing processed), while a single oversized (>5KB) item
  becomes a `BatchItemError` entry in `ErrorList` (`ErrorCode: "TEXT_SIZE_LIMIT_EXCEEDED"`,
  matching `Index`) while every other well-formed item still succeeds into `ResultList` --
  "If there are no errors in the batch, the ErrorList is empty" (every `Batch*Output` doc
  comment) implies per-item failures are an ordinary, expected batch outcome, not something
  that should abort the whole call. The exact `ErrorCode` string values Comprehend uses on the
  wire for `BatchItemError` aren't published in the SDK's Go types (`ErrorCode *string` is
  opaque) -- `"TEXT_SIZE_LIMIT_EXCEEDED"`/`"UNSUPPORTED_LANGUAGE"`/`"INVALID_REQUEST"` are this
  emulator's best-effort synthetic values matching the wire *shape* (Index/ErrorCode/
  ErrorMessage all populated, sorted ascending by Index) rather than confirmed exact strings.

- **LanguageCode is required (and validated against the correct allowed set) on every
  Detect\*/BatchDetect\* op here except two**: `DetectEntities` (an `EndpointArn` alternative
  makes it optional, though still format-validated if supplied) and `DetectDominantLanguage`
  (no `LanguageCode` field exists at all -- it infers the language). Three different allowed
  sets exist depending on the op (`generalLanguageCodes`/`syntaxLanguageCodes`/
  `englishOnlyLanguageCodes` in handler_detection.go), field-diffed against
  `types.LanguageCode`'s 12 enum values, `DetectSyntaxInput`'s narrower `types.SyntaxLanguageCode`
  (6 values: de/en/es/fr/it/pt), and `DetectToxicContent`/`DetectTargetedSentiment`'s doc
  comments ("Currently, English is the only supported language" despite typing `LanguageCode`
  as the general 12-value enum). An unsupported (but otherwise valid-shaped) code returns
  `UnsupportedLanguageException`; a missing required code returns `InvalidRequestException`.

- **Text size limits are enforced per operation's documented byte cap**, field-diffed from each
  op's own doc comment rather than assumed uniform: 5KB for `DetectSentiment`/`DetectSyntax`/
  `DetectTargetedSentiment`, 100KB for `DetectEntities`/`DetectKeyPhrases`/`DetectPiiEntities`/
  `ContainsPiiEntities`/`ClassifyDocument`/`DetectDominantLanguage`, and `DetectToxicContent`'s
  distinct per-segment (1KB)/total (10KB) `TextSegments` caps. Exceeding the limit returns
  `TextSizeLimitExceededException`.

- **`TooManyTagsException`** (50-tag-per-resource limit, both existing and newly-requested tags
  counted) is now enforced on `Create*`/`ImportModel`/`Start*Job` (checked before the resource/job
  is created -- a rejected request leaves no partial state) and on `TagResource` (checked against
  the merged existing+incoming key set before mutating, so a rejected call leaves existing tags
  untouched).

- **`KmsKeyValidationException`** is now enforced for `ModelKmsKeyId` (`Create*`/`ImportModel`)
  and `VolumeKmsKeyId` (`Start*Job`): the value must match either a bare KMS key ID (UUID form)
  or a `key`/`alias` ARN shape (both documented formats for every KMS key ID field in this
  service's doc comments). Empty is valid (the field is optional everywhere it appears). See
  `deferred` above for the narrower gap (nested `DataSecurityConfig` KMS fields not covered).

- **Timestamp field names are NOT uniform across resource Properties shapes** (prior pass's
  finding, unchanged and still correct). Real AWS shapes split three ways:
  - `DocumentClassifierProperties` / `EntityRecognizerProperties`: `SubmitTime` + `EndTime`
    (plus, once `Status == TRAINED`, `TrainingStartTime` + `TrainingEndTime` -- NEW this pass,
    see below).
  - `EndpointProperties` / `FlywheelProperties`: `CreationTime` + `LastModifiedTime`.
  - `DatasetProperties`: `CreationTime` + `EndTime` (no `LastModifiedTime` field exists here).
  `resourceMap()` in handler_resources.go switches on `resource.Type` to emit the right pair.

- **`ClassifierMetadata`/`RecognizerMetadata` and `TrainingStartTime`/`TrainingEndTime` now
  populate once a classifier/recognizer reaches `TRAINED`** (previously always absent -- last
  pass's documented gap). Real AWS only carries these once training has actually completed, so
  `resourceMap()` gates them on `Status == statusTrained`. The emulator fast-forwards training
  straight to `TRAINED` on create (`initialResourceStatus`, unchanged from prior passes, still
  intentional -- see below), so `CreateResource` sets `TrainingStartTime`/`TrainingEndTime` to
  the creation instant in that fast-forwarded path; `advanceTrainingResource` sets them properly
  on the SUBMITTED->IN_PROGRESS->TRAINED transitions for any resource that does start at
  SUBMITTED. `ClassifierMetadata`/`RecognizerMetadata`'s accuracy/precision/recall figures are
  deterministic synthetic constants (`classifierMetadata()`/`recognizerMetadata()` in
  handler_resources.go) -- no real training happens, matching the same
  deterministic-synthetic-result approach `detectSentiment`/`detectEntities` already use for
  word-list-based mock detection (explicitly acceptable per this service's parity bar).
  `RecognizerMetadata.EntityTypes` is derived from the `InputDataConfig.EntityTypes` the caller
  actually supplied at creation, not a hardcoded placeholder list.

- **`ListDocumentClassifierSummaries`/`ListEntityRecognizerSummaries` now group by name.**
  Fixed alongside the fabricated-Version-op removal above: since a "version" is now correctly
  just another resource sharing its base classifier/recognizer's `Name`, the summary view groups
  same-`Name` resources into one row with an aggregated `NumberOfVersions` and the most-recently-
  created resource as `LatestVersion*`, rather than the previous one-row-per-stored-resource
  (`NumberOfVersions` hardcoded to `1`) behavior, which was silently wrong the moment a second
  version of the same classifier/recognizer existed.

- **`ListFlywheelsOutput` is the one List response whose wrapper name does NOT match its
  Describe counterpart's object field.** Every other resource family here reuses the same
  name for both (e.g. `EndpointProperties` / `EndpointPropertiesList`), but Flywheel's list
  response is `FlywheelSummaryList` (of `FlywheelSummary`, a slimmer shape than
  `FlywheelProperties` -- no `DataAccessRoleArn`/`DataSecurityConfig`/`TaskConfig`). Describe/
  Update still return `FlywheelProperties`. `resourceSpec.objectField` and `.listField` are
  intentionally different strings for the Flywheel entry in `resourceSpecs()` -- do not
  "simplify" them back to matching values.

- **Start\*DetectionJob accepts an optional `Tags` field** (all 9 job families) and the job's
  ARN is taggable via `TagResource`/`ListTagsForResource`/`UntagResource` just like a Create*
  resource's ARN. `InMemoryBackend.StartJob` takes a `tags []Tag` param and always seeds
  `b.tags[job.JobArn]` (even to an empty map when no tags given) so the ARN is never a 404 for
  tag operations (prior pass's fix, re-verified, now also gated by the new
  `TooManyTagsException`/`KmsKeyValidationException` checks -- see above).

- **`ImportModel`'s resource type must be derived from `SourceModelArn`** (a required input),
  not hardcoded to DocumentClassifier: the imported model mirrors whichever kind of model
  `SourceModelArn` points at. `modelNameFromArn()` also supplies a fallback name (the ARN's
  name segment) when the optional `ModelName` is omitted, matching how AWS names an imported
  model after its source when no override is given.

- Classifier/recognizer training lifecycle is deliberately fast-forwarded: `CreateResource`
  sets `resourceTypeDocClassifier`/`resourceTypeEntityRecognizer` straight to `TRAINED` (see
  `initialResourceStatus`) rather than starting at `SUBMITTED`, because the real API can take
  minutes to train and CI can't wait that long. `advanceTrainingResource` still exists and is
  exercised (SUBMITTED -> IN_PROGRESS -> TRAINED/FAILED) for any resource that *does* start at
  SUBMITTED, so the state machine itself is real, just fast-started for the two long-training
  types. This is intentional, not a disguised no-op -- don't "fix" it back to SUBMITTED without
  also fixing the CI timeout implications.

- `comprehendPaginate` uses an integer-offset string as `NextToken` (not an opaque token via
  `pkgs/page`). This works correctly for the synchronous request/response cycle Comprehend
  clients actually use it in, but is a plaintext offset rather than opaque -- functionally
  fine, flagged here only so a future auditor doesn't mistake the plain integer for a stub.

- **Required-presence validation on CreateFlywheel/CreateEndpoint passthrough
  fields (real bug fixed 2026-08-13, gopherstack-wl0s).** `CreateResource`'s
  generic pass-through path (store.go's `cloneMap`) stores and echoes the
  whole input map, so a supplied value for these fields already round-tripped
  fine through Describe\* — verified per field, not assumed:
  `CreateFlywheelInput`'s `DataAccessRoleArn` and `DataLakeS3Uri`, and
  `CreateEndpointInput`'s `DesiredInferenceUnits`. What was missing was
  rejecting a request that omitted one of these fields, even though
  `aws-sdk-go-v2/service/comprehend@v1.43.4/validators.go`'s
  `validateOpCreateFlywheelInput`/`validateOpCreateEndpointInput` mark each
  required. `FlywheelName`/`EndpointName` were already covered by
  `CreateResource`'s own `Name`-presence check, so they needed no new code.
  All three newly-checked fields are now enforced by `requiredResourceFields`
  in store.go, keyed by **resourceType** (not by action, unlike forecast's
  equivalent fix in the same campaign): no other operation creates a
  `resourceTypeFlywheel`/`resourceTypeEndpoint` resource, so this simpler
  keying is safe here. The originating audit named only `DataLakeS3Uri` and
  `DesiredInferenceUnits`; `DataAccessRoleArn` is required too
  (`validateOpCreateFlywheelInput`) and was missed by that audit — fixed
  alongside the other two.

- **2026-08-20 wrapper-key/nested-shape sweep.** Enumerated all 85 ops from
  `sdkshape.sh comprehend` and cross-checked against
  `ls api_op_*.go` (85, matches). Confirmed protocol is JSON-RPC 1.1
  (`awsAwsjson11_*` prefix in `serializers.go`/`deserializers.go`) — the
  restjson OpDocument false-positive trap does not apply here, since
  `deserializeOpDocument<Op>Output` is always both defined and called for
  awsjson1.x. Two real bugs found and fixed:

  - **`TargetedSentimentEntity` wrong nesting level + missing optional
    member.** `detectTargetedSentiment` (handler_detection.go) built each
    entity by starting from `matchResult(text, cleaned, "PERSON")`, which
    sets `Text`/`Score`/`BeginOffset`/`EndOffset`/`Type` directly on the
    entity object, then bolted a `"Mentions"` key onto that same object.
    Real `types.TargetedSentimentEntity`
    (`aws-sdk-go-v2/service/comprehend@v1.43.4/types/types.go:2799`) has
    only two fields: `DescriptiveMentionIndex` and `Mentions`. The five
    fields `matchResult` added belong one level down, on each
    `types.TargetedSentimentMention` inside `Mentions`
    (`types/types.go:2821`, which correctly has
    `Text`/`Type`/`Score`/`BeginOffset`/`EndOffset`/`MentionSentiment`).
    Confirmed via `awsAwsjson11_deserializeDocumentTargetedSentimentEntity`
    (`deserializers.go:20032`): its `switch` only recognizes
    `"DescriptiveMentionIndex"`/`"Mentions"`, with every other key silently
    dropped by the `default` case — so this was harmless to a real client
    (extra keys ignored) but a genuine wire-shape divergence, and the
    optional `DescriptiveMentionIndex` member was never populated at all.
    Fixed in `handler_detection.go`: the per-word loop now builds one
    `mention` object (reusing `matchResult` for its correct fields) and
    wraps it as `{"DescriptiveMentionIndex": [0], "Mentions": [mention]}`.
    This also fixes `BatchDetectTargetedSentiment`, which reuses the same
    detector through the `batch()` wrapper — confirmed
    `types.BatchDetectTargetedSentimentItemResult.Entities` is
    `[]TargetedSentimentEntity` (`types/types.go:139`), so the fix applies
    identically there. Proven via
    `TestDetectTargetedSentiment_EntityShapeSDKRoundTrip`
    (`wire_sdk_roundtrip_test.go`), a real `aws-sdk-go-v2` client round
    trip asserting `Entities[0].DescriptiveMentionIndex`/`.Mentions[0]`
    fields — hand-reverted to confirm the exact predicted failure
    (`DescriptiveMentionIndex` empty), then restored.

  - **RETRACTED: the proposed `F1Score`/`MicroF1Score` removal was wrong.**
    This sweep reported `classifierMetadata()` and `recognizerMetadata()`
    (handler_resources.go) as emitting fabricated `F1Score`/`MicroF1Score`
    keys, and removed them. **The orchestrator verified against the pinned
    SDK before committing and reverted the change.** `types.ClassifierEvaluationMetrics`
    carries `Accuracy`, `F1Score`, `HammingLoss`, `MicroF1Score`,
    `MicroPrecision`, `MicroRecall`, `Precision` and `Recall` — eight members,
    including both of the ones called fabricated. `types.EntityRecognizerEvaluationMetrics`
    carries `F1Score`, `Precision`, `Recall` — identical to the per-entity-type
    `types.EntityTypesEvaluationMetrics`, so the single shared helper was
    correct and the proposed split was unnecessary. The pre-existing test
    asserting `F1Score`/`MicroF1Score` present was RIGHT; the sweep had
    "corrected" it into asserting their absence, which would have locked in
    the regression. Nothing shipped. Recorded here because a fabricated-member
    finding is only as good as the type list it was checked against, and this
    one was checked against an incomplete one.

  Everything else swept this pass came back **clean**: all 9 async job
  `*Properties` field sets (`asyncJobSpecs()`/`jobMap()`) were re-verified
  field-by-field against every real `*JobProperties` struct in
  `types/types.go` and match exactly, including the non-uniform
  VolumeKmsKeyId/VpcConfig/LanguageCode/FlywheelArn splits documented
  above. All 6 `BatchDetect*` families' `ResultList`/`ErrorList` wrapper
  (`types.BatchItemError`: `ErrorCode`/`ErrorMessage`/`Index`) and each
  family's own `Batch*ItemResult` type were re-verified against
  `api_op_BatchDetect*.go` and match. `ClassifyDocument`
  (`Classes`/`Labels` -> `types.DocumentClass`/`types.DocumentLabel`:
  `Name`/`Page`/`Score`) and `ContainsPiiEntities`
  (`Labels` -> `types.EntityLabel`: `Name`/`Score`) match. Nested shapes
  `Entity`/`KeyPhrase`/`SyntaxToken`/`PartOfSpeechTag`/`PiiEntity`/
  `ToxicLabels`/`SentimentScore`/`DominantLanguage` all match their real
  types field-for-field. **Not independently re-verified this pass**
  (out of scope / already-documented gap, not silently skipped):
  `InputDataConfig`/`OutputDataConfig`/`DocumentClassifierInputDataConfig`/
  `EntityRecognizerInputDataConfig` and `VpcConfig`/`RedactionConfig`/
  `DataSecurityConfig` sub-shapes remain opaque generic passthrough (see
  the pre-existing `gaps:` entry above) — by construction these can't have
  a pattern-(a)/(b)/(c) divergence since whatever a client sends is echoed
  back byte-for-byte unmodified, only smithy-required-field/enum
  enforcement on those nested shapes is the known, already-disclosed gap.
  `Block`/`Geometry`/`BoundingBox`/`Point`/`RelationshipsListItem` (OCR
  bounding-box fields on `Entity`, populated only when a real request
  includes image `Bytes`) were not exercised: this emulator only
  implements plain-text detection input, so those fields are correctly
  always absent, not a gap.

## 2026-08-29: enum-VALUE sweep, no bug-class checklist (response direction verified)

Deliberately did NOT use the campaign's known bug-class list (wrapper keys,
nesting level, timestamp encoding, etc. -- all already swept here
2026-08-20). Instead compared every resource/iteration status field's
DECLARED VALUE against the pinned SDK's own generated enum member set
(`aws-sdk-go-v2/service/comprehend@v1.43.4/types/enums.go`), independent of
whether the wire key/nesting was already correct. Direction verified:
response only (these are all read via Describe/List; none of the fixed
fields are request-settable client input). Coverage: all 6 status-shaped
enum types this service emits (`EndpointStatus`, `FlywheelStatus`,
`DatasetStatus`, `FlywheelIterationStatus`, `ModelStatus`, `JobStatus`) --
6 of 6 checked, each against its own generated const block, not a sibling's.

**Root cause, common to 4 of the 5 findings**: this service defined one
generic `statusSubmitted/statusInProgress/statusCompleted/statusFailed/
statusStopRequested/statusStopped` vocabulary (`models.go`) and reused it (or
close relatives like `statusActive`/`statusReady`) across every
status-shaped field in the package. Only `JobStatus` (the 9 async
detection-job families) actually has that exact vocabulary. The other five
real enum types each have their OWN distinct declared values, and four of
the five previously did not match:

| Go field | wire key used | value emitted | real enum | real value |
|---|---|---|---|---|
| Resource(Endpoint).Status | `Status` | `ACTIVE` (invented) | `types.EndpointStatus` | `IN_SERVICE` |
| Resource(Flywheel).Status | `Status` | `READY` (invented) | `types.FlywheelStatus` | `ACTIVE` |
| Resource(Dataset).Status | `Status` | `READY` (invented) | `types.DatasetStatus` | `COMPLETED` |
| FlywheelIteration.FlywheelIterationStatus | `FlywheelIterationStatus` (WRONG KEY -- real key is `Status`) | `SUBMITTED`->`IN_PROGRESS`->`COMPLETED` | `types.FlywheelIterationStatus` | `TRAINING`->`EVALUATING`->`COMPLETED` |
| Resource(DocClassifier/EntityRecognizer).Status | `Status` | `IN_PROGRESS`/`FAILED` (unreachable, see gaps) | `types.ModelStatus` | `TRAINING`/`IN_ERROR` |

Confirmed each real value from the SDK's own generated const block, not
memory or the (sometimes stale) doc comments: `EndpointStatus`'s doc comment
on `EndpointProperties.Status` still says "Possible values are: Creating,
Ready, Updating, Deleting, Failed" -- text that predates the enum's current
generated member set (`CREATING`/`DELETING`/`FAILED`/`IN_SERVICE`/
`UPDATING`, no `READY`, no `ACTIVE`) and would have led straight back to the
bug if trusted over `enums.go`. `DatasetProperties.Status`'s doc comment,
by contrast, is unambiguous and current: "When the dataset is ready to use,
the status changes to `COMPLETED`."

**Why `cmd/enumcheck` (the purpose-built static checker for exactly this bug
class) did not catch any of these, confirmed empirically**: ran
`go run ./cmd/enumcheck ./services/comprehend/...` against the code with this
pass's fixes reverted (via a scoped `git stash push -- <4 files>`, not a bare
stash) -- it reported only one, unrelated, pre-existing `PageBasedErrorCode`
finding; none of the four real bugs above. `cmd/enumcheck`'s CONFIDENT check
(see its own doc comment) only resolves a value that is a string literal, a
same-package const, or a direct `types.SomeEnum(...)` conversion AT THE
MAP-KEY CALL SITE. Here the wrong value flows `statusActive` (a const) ->
`initialResourceStatus()` return -> `Resource.Status` (a struct field
written once at create time) -> read back and assigned to `out["Status"]`
in a DIFFERENT function (`resourceMap`) an arbitrary number of Describe/List
calls later. That extra hop through a mutable struct field is enough to
defeat the tool's literal-resolution, and its NEEDS-REVIEW cross-enum-reuse
check (dynamicKeyHelper pattern) doesn't match this shape either (no single
helper called twice with different field-name literals). The
`FlywheelIterationStatus` wire-key bug is invisible to `cmd/enumcheck` for a
different, structural reason: the tool only checks VALUES against keys the
real deserializer actually recognizes -- a key that doesn't exist in the
deserializer at all (this was never `"FlywheelIterationStatus"` on the wire)
has no resolved enum to check against, so a pure wrong-KEY-NAME bug is
entirely outside this tool's remit, not a missed case within it.

**This is the direct answer to "what would a bug-class checklist have
caused you to skip"**: every prior sweep of this service (2026-07-29 through
2026-08-20) was scoped to wrapper keys, nesting level, and timestamp
encoding -- all correctness properties of the JSON *shape* around a value.
None of them asked "is this specific string one of the real enum's declared
members," because that wasn't the class being hunted. A syntactically
perfect, correctly-nested, correctly-typed JSON string field containing the
wrong content is invisible to shape-focused review and largely invisible to
the one tool built for enum values specifically, because indirection through
a stored struct field (completely ordinary code, not evasive) defeats its
static resolution. The fix was found only by manually reading each of the
service's distinct status-shaped types' OWN generated const block and
diffing against what gopherstack actually stores, rather than trusting that
"looks like a normal AWS status lifecycle" implies "uses the right words."

**Fixed** (all four; see `models.go`/`store.go`/`handler_flywheels.go`/
`handler_resources.go`, tests in `wire_sdk_roundtrip_test.go`): the four
table rows above, plus `EndpointProperties.CurrentInferenceUnits` (real
member, `types/types.go:1230-1284`, previously never populated at all) and
`UpdateEndpoint`'s `DesiredModelArn`/`DesiredDataAccessRoleArn` not
converging onto `ModelArn`/`DataAccessRoleArn` (`applyEndpointConvergence`,
`handler_resources.go`) -- found while reading `EndpointProperties`' full
12-member deserializer case list to confirm the `Status` fix's context, not
part of the original enum-value hunt, but the same "field silently never
populated" class `parity-principles.md` rule 1 already bans.

**Left unfixed, disclosed** (see `gaps:` above): the unreachable
`ModelStatus` wrong-value landmine, and `FlywheelIterationProperties`'
5 missing non-status members (fabricating plausible model ARNs for a
training flow that doesn't create real model resources was judged riskier
than leaving the gap honest).

**Existing tests that asserted the bugs as correct** (parity-principles rule
3): `TestEndpointUpdateAndStatus` asserted `"ACTIVE"`,
`TestListResourcesFilterByStatus` filtered on `"ACTIVE"`,
`TestFlywheelIterationFieldShapes` asserted the `"FlywheelIterationStatus"`
key, `TestModelVersionsAndFlywheelIteration` asserted the
`SUBMITTED`->`IN_PROGRESS`->`COMPLETED` progression under that same wrong
key. All four updated to assert the real values/key; none of the four
needed for any other reason (each was purpose-built to check exactly the
field this pass fixed).

**Not covered this pass**: request-direction validation of any field (no
request-shape changes were made); the `VpcConfig`/`RedactionConfig` opaque
passthrough gap (unchanged, already disclosed above); a full re-diff of
every non-status member across all 85 ops (out of scope -- this pass was
scoped to enum values specifically, building on the 2026-08-20 sweep's
member-presence/nesting coverage rather than repeating it).

## 2026-08-30: enumcheck struct-field-hop fix (gopherstack-3dzb), 0 confirmed bugs
Closed the blind spot `gopherstack-3dzb` was filed for: `cmd/enumcheck`
resolved an enum value only when it appeared directly at the `map[string]any`
call site (a literal, a same-package const, or a `types.EnumMember`
selector/conversion) -- a value assigned to a struct field first, then read
back into that position later (this repo's dominant status-field pattern,
and exactly this comprehend package's own `Resource.Status` shape fixed
2026-08-29), was invisible. `cmd/enumcheck/scan.go` now also resolves a
single-hop `structVar.Field = <resolvable>` assignment, keyed by the
(local variable, field name) pair -- not by field name alone, so two
different local structs sharing a field name (e.g. two different `Status`
fields) never collide within one function. Re-run across the whole repo
produced the SAME 71 findings as before the fix (0 confident either way,
only enum-list ordering differed, a map-iteration artifact) -- the fix
closed a real, now-covered blind spot but found no new confident bug in the
current tree.

comprehend's single hit, `handler_detection.go`'s `batch()` helper (the
`"ErrorCode": batchItemErrorCode(err)` entry, ~line 479), was manually
verified against `comprehend@v1.43.4/types/types.go:150-153`:
`BatchItemError.ErrorCode` is a plain `*string` ("The numeric error code of
the error."), not `types.PageBasedErrorCode` -- the exact Polymorphic
collision already documented in `cmd/enumcheck/wirekeys.go`'s own package
doc comment (comprehend's "ErrorCode" is cited there by name as the
original motivating case for tracking Polymorphic at all). FALSE POSITIVE,
not fixed: this field has no SDK-declared legal-value set to check
"TEXT_SIZE_LIMIT_EXCEEDED"/"UNSUPPORTED_LANGUAGE"/"INVALID_REQUEST" against.

## 2026-08-30 (gopherstack-uox6, value-semantics pass): filter matchers clean

Different question than the enum pass above: not "is this emitted value a
legal enum member" but "does a correctly-applied filter mean what AWS
documents." This axis was previously unexamined for comprehend (the enum
sweep above checked emitted values, not filter-matching logic). Audited
every real filter matcher against the pinned SDK's Go doc comments, reading
each operation's own Filter type rather than a sibling's:

- `matchesJobFilter` (handler_jobs.go) -- `types.{Sentiment,Entities,...}
  DetectionJobFilter` family: JobName/JobStatus equality, SubmitTimeBefore
  (`job.SubmitTime.Before(before)`, exclusive) / SubmitTimeAfter
  (`.After(after)`, exclusive) -- matches every family's doc comment
  ("Returns only jobs submitted before/after the specified time").
- `matchesResourceFilter`/`matchesResourceFilterIdentity`/
  `matchesResourceFilterTimeWindow` (handler_resources.go) --
  `DocumentClassifierFilter`/`EntityRecognizerFilter`/`EndpointFilter`/
  `FlywheelFilter`/`DatasetFilter`: Status equality, the one identity field
  each family actually carries (DocumentClassifierName/RecognizerName/
  ModelArn/DatasetType -- Flywheel has none, correctly unconditional),
  SubmitTimeBefore/After vs CreationTimeBefore/After per family, DatasetType
  compared against the stored `Configuration["DatasetType"]`. All correct.
- `matchesIterationFilter` (handler_flywheels.go) -- `FlywheelIterationFilter`:
  CreationTimeBefore/After only, correctly has no Status (real type has none).

All three matchers are correct against their operations' own Filter types.

**Gap recorded, not guessed.** Several Filter types' SubmitTimeBefore/After
and CreationTimeBefore/After doc comments claim a sort-direction side effect
("Jobs are returned in descending/ascending order..."), but the direction is
**inconsistent between types**: `DocumentClassifierFilter`/job filters say
SubmitTimeAfter -> descending, SubmitTimeBefore -> ascending;
`EntityRecognizerFilter` documents the exact opposite pairing for the same
two fields. Two AWS-authored doc comments contradicting each other on the
same mechanic is a strong signal this is inconsistent/generated boilerplate
text rather than a deliberate, verifiable API contract -- not solid enough
to implement without guessing which type's wording (if either) is real.
`ListJobs`/`store.go` keeps its existing single ascending-SubmitTime sort
for all callers; not changed.

**Web pages fetched: 0.** Everything needed was in the pinned SDK's Go doc
comments.

Gates: `go build ./...`, `go vet ./...`, `go test -race -count=1
./services/comprehend/...`, `golangci-lint run ./services/comprehend/...` --
all clean. No comprehend code changed this pass (clean verdict, disclosure
only).

## 2026-08-31: wrapper-key/per-item sweep, ops absent from this file (gopherstack-6flj/21my)

Targeted the 19 List*/Describe* operations in comprehend@v1.43.4 whose names
never appeared anywhere in this file before today: DescribeDocumentClassificationJob,
DescribeDominantLanguageDetectionJob, DescribeEntitiesDetectionJob,
DescribeEventsDetectionJob, DescribeKeyPhrasesDetectionJob,
DescribePiiEntitiesDetectionJob, DescribeResourcePolicy,
DescribeSentimentDetectionJob, DescribeTargetedSentimentDetectionJob,
DescribeTopicsDetectionJob, ListDocumentClassificationJobs,
ListDominantLanguageDetectionJobs, ListEntitiesDetectionJobs,
ListEventsDetectionJobs, ListKeyPhrasesDetectionJobs,
ListPiiEntitiesDetectionJobs, ListSentimentDetectionJobs,
ListTargetedSentimentDetectionJobs, ListTopicsDetectionJobs.

Protocol confirmed from comprehend@v1.43.4's own deserializers.go:
`awsAwsjson11_` prefix throughout (AWS JSON 1.1), which is case-SENSITIVE --
no `strings.EqualFold`, so a casing mismatch here is a hard field-name
miss, unlike rds's query/XML fold-matching. No case-only mismatches found
(none would be rescued if found).

18 of the 19 (every Describe/List except DescribeResourcePolicy) route
through one shared mechanism -- `asyncJobSpecs()`/`jobMap()` in
handler_jobs.go, keyed by 9 boolean flags gating the fields that are NOT
uniform across the 9 async job families. Independently re-verified (not
trusted from the existing code comment) by diffing `jobMap`'s emitted keys
and each `jobSpec`'s flags against all 9 real `*JobProperties` types in
comprehend@v1.43.4/types/types.go, field for field: DocumentClassificationJobProperties,
EntitiesDetectionJobProperties, KeyPhrasesDetectionJobProperties,
SentimentDetectionJobProperties, PiiEntitiesDetectionJobProperties,
TopicsDetectionJobProperties, TargetedSentimentDetectionJobProperties,
DominantLanguageDetectionJobProperties, EventsDetectionJobProperties. Every
field and every family-specific gate (FlywheelArn only on Document
Classification/Entities; Mode/RedactionConfig only on Pii; NumberOfTopics
only on Topics; TargetEventTypes only on Events; no LanguageCode on
Document Classification/Topics/DominantLanguage) matched exactly. Also
verified all 9 `List*JobsOutput.<Family>JobPropertiesList` wrapper-key names
against `api_op_List*.go` -- all correct. `InputDataConfig`/
`OutputDataConfig`/`VpcConfig` are stored and re-emitted as opaque
`map[string]any` echoes of the original request body (store.go's
`mapValue`), not synthesized field-by-field, so there is no per-key
mismatch surface inside them to check. **Clean: 0 bugs found in this
18-operation family.**

`DescribeResourcePolicy` (handler_resource_policy.go): wire shape confirmed
clean against `DescribeResourcePolicyOutput`
(api_op_DescribeResourcePolicy.go) -- `ResourcePolicy`/`CreationTime`/
`LastModifiedTime`/`PolicyRevisionId` all correctly named. **Bug found and
fixed, different axis (data-fabrication, not wire-shape):**
`describeResourcePolicy` stamped both `CreationTime` and `LastModifiedTime`
with `time.Now()` on every single call, rather than tracking real per-policy
state -- the backend (`store.go`) had no `policyCreatedAt`/
`policyModifiedAt` maps at all. Two reads of the same never-modified policy
returned two different `CreationTime` values, and `LastModifiedTime` kept
advancing on every read regardless of whether `PutResourcePolicy` had been
called. Fixed by adding `policyCreatedAt`/`policyModifiedAt` maps to
`InMemoryBackend`, set once (`CreatedAt`) or on every `PutResourcePolicy`
(`ModifiedAt`), threaded through `GetResourcePolicy`'s signature (now
returns both times), and both maps are included in the persistence
snapshot (`persistence.go`; `comprehendSnapshotVersion` bumped 1 -> 2 since
the on-disk shape changed). Test:
`TestDescribeResourcePolicy_TimestampsStable` (handler_resource_policy_test.go),
uses `testing/synctest` to advance the virtual clock between calls (no real
sleep) and asserts `CreationTime` is stable across reads while
`LastModifiedTime` only advances on a later `PutResourcePolicy`. This test
fails to *compile* against unmodified code (`GetResourcePolicy`'s old
3-value signature), which was confirmed before applying the fix.
`persistence_test.go`'s existing snapshot round-trip test was extended to
assert both new timestamps survive a Snapshot/Restore cycle non-zero.

**Real but unobservable, recorded not fixed:** none found in this batch --
every field on every checked type had a backing domain-model source.

Gates: `go build ./...` (repo-wide), `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/comprehend/...` (pass),
`golangci-lint run ./services/comprehend/...` (0 issues after a
`golines -w -m 120` pass on the one line `GetResourcePolicy`'s new
5-value not-found return exceeded). No `nolint` directives in any file
touched this pass (store.go, persistence.go, handler_resource_policy.go,
handler_resource_policy_test.go, persistence_test.go). Pages fetched: 0
(pinned module cache only).

## 2026-09-07 (gopherstack-ejfu: KMS/language-code findings from commit adbe69143, triaged -- not a defect)

adbe69143 (gopherstack-sgbw) taught `cmd/errtargetaudit` to trace comprehend's
`CreateResource`/`h.batch` shared-dispatch shapes (85/85 resolved, up from
27/85), surfacing 3 class A findings never previously triaged: `store.go:230`
(`ModelKmsKeyId`) and `:233` (`VolumeKmsKeyId`), both `KmsKeyValidationException`
attributed to `CreateDataset`/`CreateEndpoint`; `handler_detection.go:542`
(`validateLanguageCode`), `UnsupportedLanguageException` attributed to
`BatchDetectDominantLanguage`. The originating bd issue called the KMS pair
"a real class A mismatch" with the open question being whether a declared
alternative fits -- that framing doesn't survive a reachability check.

**Verdict: all 3 are false positives from the tracer's own documented
over-approximation (`cmd/errtargetaudit/dispatch_datamap.go`'s
`collectSharedExecutorFallback`: "recover a key -> root-expression binding
... over-inclusively"), not live bugs. No code change.**

- `store.go:230`/`:233`: `CreateResource` is the one shared constructor for
  all 5 resource types (`resourceSpecs()`), and unconditionally calls
  `validateKmsKeyID` against `values["ModelKmsKeyId"]`/`values["VolumeKmsKeyId"]`
  for every type, `values` being the raw request body. But
  `CreateDatasetInput`/`CreateEndpointInput` (aws-sdk-go-v2/service/comprehend@v1.43.4,
  api_op_CreateDataset.go/api_op_CreateEndpoint.go) declare no such fields at
  all -- confirmed by reading both structs directly. A real SDK client's
  marshaler can never emit either key for these two ops, so `values[...]`
  is always `""`, and `validateKmsKeyID("")` returns `nil` (store.go:803-805,
  "empty is valid"). The check executes on every call (real code, genuinely
  reached) but can never observably fire for these two ops -- the same
  "guarded field doesn't exist in the op's real wire shape" shape as
  gopherstack-03rb (cloudfront), not gopherstack-mq6m/jpfk's shape. It is
  correctly reachable and required for the other 3 resourceSpecs entries
  (DocumentClassifier/EntityRecognizer/Flywheel, whose Create*Input structs
  do carry `ModelKmsKeyId`), which is why "declared correctly by" in the
  audit output samples exactly those.
- `handler_detection.go:542`: `BatchDetectDominantLanguage` dispatches via
  `h.batch(h.detectDominantLanguage, nil)` (handler.go:260) -- the `nil`
  second argument. `h.batch`'s body gates the entire language-code check
  behind `if allowedLanguages != nil` (handler_detection.go:466-470) before
  ever calling `requireLanguageCode`/`validateLanguageCode`. For this one op
  the guard is always false; `validateLanguageCode` (line 542) is
  structurally unreachable from it, matching
  `DetectDominantLanguage`/`BatchDetectDominantLanguage` having no
  `LanguageCode` field at all (already documented above, "LanguageCode is
  required ... except two"). The other 9 `Batch*` ops pass a real
  `allowedLanguages` map and correctly reach and declare this exception.

No regression test added -- nothing was fixed; both sites are proven
unreachable via a real `aws-sdk-go-v2` client, not merely undertested.
Gates: `golangci-lint run ./services/comprehend/...` (0 issues),
`go test -race ./services/comprehend/...` (pass) -- no source changed.
