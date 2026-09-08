---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: codepipeline
sdk_module: aws-sdk-go-v2/service/codepipeline@v1.54.0   # version audited against
last_audit_commit: d50d1410                              # stale -- git usage disallowed this pass; see last_audit_date
last_audit_date: 2026-09-07
overall: A            # 2026-09-07 (gopherstack-3djp) errtargetaudit sweep: 7 class A findings,
# all "sentinel reference" mechanism (emitted code doesn't match any code the op's own
# deserializeOpError<Op> models). Verified each against codepipeline@v1.49.4's deserializers.go
# AND the live docs.aws.amazon.com/codepipeline/latest/APIReference Errors sections (both agree
# in every case: the emitted code is genuinely undeclared for that op). ALL 7 LEFT UNFIXED --
# a first pass fixed 3 (CreateCustomActionType/DeleteCustomActionType/DeletePipeline) by
# removing the guard and making the op idempotent-success, but that was WRONG and reverted
# after review: the only evidence offered was each op's live-docs Response Elements sentence
# "If the action is successful, the service sends back an HTTP 200 response with an empty HTTP
# body" -- but that exact sentence also appears verbatim on DisableStageTransition, which DOES
# declare and throw PipelineNotFoundException on a missing pipeline (confirmed by fetching its
# own live docs page). It is generic Response-shape boilerplate describing what a *successful*
# call returns, not evidence that a not-found/duplicate condition is swallowed into success --
# unlike workmail, where the campaign's actual bar (an explicit "does not return an error"/
# equivalent sentence, present on the fixed ops and absent from the ten left alone with the
# identical declared-set mismatch) was met. DeleteCustomActionType's cited doc paragraph
# ("you must use a string in the version field that has never been used before" to re-create
# after deletion) also does not support a duplicate-create upsert -- if anything it argues
# against silently accepting a reused key. All 3 reverted to original behavior; the 2
# accompanying test changes (TestHandler_CreateCustomActionType's "duplicate" case,
# TestHandler_DeleteCustomActionType's "not_found" case, TestHandler_DeletePipeline's "not
# found" case, all flipped 400->200) reverted with them, and the new
# TestCreateCustomActionType_Recreate_Overwrites test removed. Grouped by cause, all 7 now
# carry a landmine comment at the call site and are left for a follow-up bd issue:
#  - CreateCustomActionType emits InvalidStructureException on a duplicate
#    category/provider/version -- not in this op's modeled set (ConcurrentModificationException/
#    InvalidTagsException/LimitExceededException/TooManyTagsException/ValidationException only).
#    No declared code is a confirmed replacement. custom_action_types.go.
#  - DeleteCustomActionType emits ActionTypeNotFoundException on a nonexistent type -- not in
#    this op's modeled set (ConcurrentModificationException/ValidationException only). No
#    declared code is a confirmed replacement. custom_action_types.go.
#  - DeletePipeline emits PipelineNotFoundException on a nonexistent name -- not in this op's
#    modeled set (ConcurrentModificationException/ValidationException only). No declared code is
#    a confirmed replacement. pipelines.go.
#  - UpdatePipeline emits PipelineNotFoundException on a nonexistent pipeline name -- also not
#    in its modeled set (InvalidActionDeclarationException/InvalidBlockerDeclarationException/
#    InvalidStageDeclarationException/InvalidStructureException/LimitExceededException/
#    ValidationException only). InvalidStructureException is a plausible but unconfirmed
#    replacement. pipelines.go.
#  - OverrideStageCondition/RetryStageExecution/StopPipelineExecution (3 findings) all emit
#    PipelineExecutionNotFoundException for an unknown pipelineExecutionId -- that code is
#    declared by GetPipelineExecution/ListActionExecutions/ListRuleExecutions/RollbackStage but
#    NOT by these three ops. NotLatestPipelineExecutionException/StageNotRetryableException/
#    PipelineExecutionNotStoppableException are each a plausible but unconfirmed guess per op.
#    pipeline_state.go, pipelines.go.
# errtargetaudit re-run after the revert: codepipeline class A findings back at 7 (unchanged
# from the top of this pass, as expected -- no fix was retained). Gates:
# `go test -race -count=1 ./services/codepipeline/...` and
# `golangci-lint run ./services/codepipeline/...` both clean. Grade held at A (no regression;
# nothing was actually changed in the end besides the landmine comments).
# 2026-09-04 (gopherstack-ary): found and fixed a real "accepted but never done" bug -- DisableStageTransition wrote stageTransitions state that GetPipelineState echoed back, but runPipelineActions (action_engine.go) never consulted it at all, so a disabled inbound/outbound transition never actually stopped a pipeline execution from running straight through the "blocked" stage. Real SDK doc (DisableStageTransitionInput.TransitionType, api_op_DisableStageTransition.go): "prevented from transitioning into the stage ... (inbound), or prevented from transitioning from the stage after they have been processed ... (outbound)". Fixed: runPipelineActions now gates entry to a not-yet-started stage on its inbound transition, and gates advancing past a completed non-final stage on its outbound transition (the last stage's outbound is exempt -- no next stage exists for it to block into, and no SDK documentation covers that edge case, so gating it would be speculative); EnableStageTransition now re-drives runPipelineActions for the pipeline's InProgress executions so a parked run resumes without requiring another client call, mirroring the existing PutApprovalResult resume pattern. Tests: TestHandler_DisableStageTransition_BlocksInboundExecution, TestHandler_DisableStageTransition_BlocksOutboundExecution (pipeline_state_test.go), hand-reverted against `git show HEAD:<path>` on action_engine.go/pipeline_state.go, confirmed both fail with the predicted symptom (status "Succeeded" instead of "InProgress") against unmodified code, restored and md5sum-verified byte-identical. TestInMemoryBackend_SnapshotRestore_FullState (persistence_test.go) had to be adjusted -- it disabled the Inbound transition on a single-stage pipeline's only stage purely as a persistence fixture, which now (correctly) prevents that pipeline from ever starting; changed to Outbound, which has no next stage to gate on that pipeline so it stays a pure persistence fixture. Grade held at A -- fixed with hand-reverted proof, narrow and well-tested.
# 2026-08-29 errcodeaudit mapper-output sweep: cmd/errcodeaudit's new mapper-output
# extraction (handleError's errMapping table, handler.go) flagged 2 confident findings,
# both verified by hand against the pinned SDK and left unfixed, same restraint as
# codedeploy's dispatch-level unknown-action row (5e0b4978a). "ResourceInUseException"
# (handleError row for ErrResourceInUse, sole call site DeleteCustomActionType,
# custom_action_types.go) names no type codepipeline@v1.49.4/types/errors.go declares at
# all, and DeleteCustomActionType's own deserializeOpErrorDeleteCustomActionType
# (deserializers.go:560-602) models only ConcurrentModificationException/ValidationException
# -- neither fits "referenced by a pipeline". "InvalidActionException" (handleError row for
# errUnknownAction) is the dispatch()-level fallback for an unrecognized Action string --
# there is no operation to consult, structurally identical to codedeploy's own
# errUnknownAction row. Both rows now carry inline comments citing this. No code changed.
# 2026-08-19 wrapper-key/nested-shape sweep: 4 real bugs found and fixed, all previously invisible to raw-body tests. (1) GetPipelineState's StageState emitted a fabricated "outboundTransitionState" member -- real types.StageState has no such field at all (only inboundTransitionState is wire-visible). (2) GetPipelineState's inboundTransitionState used wrong keys "disabled"/"reason" -- real types.TransitionState is "enabled" (bool, INVERTED sense) / "disabledReason"; a real client's DisabledReason was always blank. (3) ListPipelines' PipelineSummary emitted a fabricated "pipelineArn" member -- real types.PipelineSummary has no ARN field at all (GetPipelineState is the only source of the ARN). (4) PutWebhook/ListWebhooks' AuthenticationConfiguration used lowercase "secretToken"/"allowedIPRange" on BOTH request parse and response emit -- real types.WebhookAuthConfiguration uses capitalized "SecretToken"/"AllowedIPRange" (uniquely, unlike every other WebhookDefinition member); a real client's IP/GITHUB_HMAC auth config was silently dropped on write and never echoed back on read. Also fixed incidentally while auditing GetPipelineState's wrapper key: its top-level Created/Updated (real, always-populated members) were never emitted at all. See families/ops notes below for citations and gopherstack-2mwl-style detail. No other bugs found across the other 30 ops swept (webhooks/customActionTypes/jobsAndThirdPartyJobs/ruleOps/pipeline CRUD/executions all independently re-diffed clean). Overall grade held at A -- these were real client-visible wire bugs but narrow in blast radius and now fixed with hand-reverted proof.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreatePipeline: {wire: ok, errors: ok, state: ok, persist: ok}
  GetPipeline: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdatePipeline: {wire: ok, errors: partial, state: ok, persist: ok, note: "version-mismatch ConflictException REMOVED (fixed, was gopherstack-invented -- real PipelineDeclaration.Version is documented as purely system-managed output, not an optimistic-concurrency input; UpdatePipeline now always succeeds and always increments version by 1, matching real AWS/CLI docs). 2026-09-07 (gopherstack-3djp): errtargetaudit flagged PipelineNotFoundException on a not-found guard -- confirmed not in this op's modeled error set, but left unfixed: no safe no-op fix exists for update-against-nothing, and no declared code is an obvious replacement. See PARITY.md header note and the landmine comment in pipelines.go."}
  DeletePipeline: {wire: ok, errors: partial, state: ok, persist: ok, note: "now also cascade-clears actionRevisions (fixed leak/stale-data bug, same class as the actionExecutions leak fixed in the prior pass). 2026-09-07 (gopherstack-3djp): errtargetaudit flagged PipelineNotFoundException on a not-found guard -- confirmed not in this op's modeled error set (ConcurrentModificationException/ValidationException only). Left unfixed: an initial pass removed the guard on the theory the live docs' 'HTTP 200 response with an empty HTTP body' sentence meant idempotent delete, but that sentence is generic Response-shape boilerplate present verbatim on DisableStageTransition too, which DOES throw PipelineNotFoundException -- not evidence either way. Reverted. See PARITY.md header note and the landmine comment in pipelines.go."}
  ListPipelines: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-19: fixed a fabricated 'pipelineArn' member on each PipelineSummary item -- real types.PipelineSummary (awsAwsjson11_deserializeDocumentPipelineSummary, deserializers.go) has no such field at all; removed from both the wire struct and the internal PipelineSummary Go type. TestListPipelines_IncludesARN (a pre-existing wrong-key test asserting the fabricated key was correct) rewritten as TestListPipelines_OmitsPipelineArn (pipelines_test.go)."}
  StartPipelineExecution: {wire: ok, errors: ok, state: ok, persist: ok, note: "now gates on the first unresolved Approval-category action (action_engine.go runPipelineActions) instead of always completing synchronously; StartTime/Trigger/ExecutionMode/ExecutionType now populated"}
  StopPipelineExecution: {wire: ok, errors: partial, state: ok, persist: ok, note: "now abandons (rather than silently orphaning) any action execution left InProgress on a pending approval gate, clearing its token so a stopped execution's approval can never be resurrected. 2026-09-07 (gopherstack-3djp): errtargetaudit flagged PipelineExecutionNotFoundException on an unknown executionID -- confirmed not in this op's modeled error set; left unfixed, no declared code is an obvious replacement. See PARITY.md header note."}
  GetPipelineExecution: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (was partial): now includes executionMode/executionType/trigger/rollbackMetadata, matching the real PipelineExecution shape exactly (verified field-by-field against awsAwsjson11_deserializeDocumentPipelineExecution -- this shape has NO startTime/lastUpdateTime, unlike PipelineExecutionSummary; an earlier draft of this fix incorrectly added them here too and was corrected before landing). ArtifactRevisions/Variables remain omitted -- no artifact-store or pipeline-variable resolution engine exists to populate them (see deferred)."}
  ListPipelineExecutions: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (was partial): summaries now include startTime/lastUpdateTime (epoch seconds)/executionMode/executionType/rollbackMetadata, verified field-by-field against awsAwsjson11_deserializeDocumentPipelineExecutionSummary (confirmed this shape has NO pipelineName/pipelineVersion, unlike the GetPipelineExecution detail shape). sourceRevisions/statusSummary/stopTrigger remain omitted -- no source-revision or stop-reason tracking exists (see deferred). 2026-08-29: FIXED a dropped-filter bug -- ListPipelineExecutionsInput.Filter (types.PipelineExecutionFilter, types/types.go:1661, serializers.go:3924/4326) was accepted nowhere; a real client's Filter.SucceededInStage.StageName request silently returned every execution instead of only the ones where that stage genuinely succeeded (the worse-than-empty wrong-answer class this campaign targets, not a silent drop to empty). Now applied via a new backend method, StageSucceededInExecution (pipeline_executions.go): a stage 'succeeded' when at least one action execution is recorded for it in that run and every one completed Succeeded. Test: TestListPipelineExecutions_SucceededInStageFilter_RealClient (wire_field_fixes_test.go)."}
  GetPipelineState: {wire: ok, errors: ok, state: ok, persist: n/a, note: "actionStates[].latestExecution now includes token/summary/lastStatusChange (fixed -- required for the real approval-token handshake: PutApprovalResult's token can ONLY come from here in real AWS); actionStates[].currentRevision now populated from PutActionRevision (fixed, was entirely absent). 2026-08-19: fixed 3 real wire bugs re-diffed against awsAwsjson11_deserializeDocumentGetPipelineStateOutput/StageState/TransitionState. (a) stageStates[].outboundTransitionState was a FABRICATED member -- real types.StageState has no such field (only inboundTransitionState is wire-visible, regardless of DisableStageTransition's Outbound transitionType); removed from the wire builder and the internal StageState Go type. (b) inboundTransitionState used wrong keys 'disabled'/'reason' -- real types.TransitionState is 'enabled' (bool, semantics INVERTED from our stored Disabled) / 'disabledReason'; a real client's TransitionState.DisabledReason was always blank (the unrecognized 'reason' key was silently dropped). (c) top-level created/updated (real, always-populated members) were never emitted at all -- added. New tests: TestGetPipelineState_InboundTransitionState, TestGetPipelineState_CreatedUpdated, TestGetPipelineState_OmitsOutboundTransitionState (pipeline_state_wire_test.go), all real-SDK-client round trips except the outbound-omission check (raw body, justified: real types.StageState has no field for the fabricated key to bind to, so a client round trip cannot observe its absence)."}
  ListActionExecutions: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: actionExecutions is no longer purely derived state safely rebuildable by StartPipelineExecution alone (an approval gate's token lives only on its ActionExecution record) so it is now persisted (backendSnapshot version bumped 1->2); correctly cleared on DeletePipeline. 2026-08-29: FIXED a dropped-filter bug -- ActionExecutionFilter.LatestInPipelineExecution (types.LatestInPipelineExecutionFilter, types/types.go:1409, serializers.go:2855/3786) was accepted nowhere; a real client narrowing via this member (instead of the flat Filter.PipelineExecutionId) got every action execution for the whole pipeline back, unfiltered. Fixed by resolving LatestInPipelineExecution.PipelineExecutionId into the same execution-ID filter the flat member already used -- this backend has no cross-execution 'latest run' history beyond a single execution's own action records, so StartTimeRange (Latest vs All) does not change the result; narrowing to the named execution is the real behavior this backend's data can honor without fabricating history it doesn't model. Test: TestListActionExecutions_LatestInPipelineExecutionFilter_RealClient (wire_field_fixes_test.go)."}
  PutActionRevision: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (was a stub: validated the pipeline exists, mutated nothing, always returned NewRevision=true). Now tracks the submitted ActionRevision per stage/action (surfaced via GetPipelineState), returns NewRevision=false on a repeat revisionId, and triggers a real, persisted pipeline execution (Trigger=PutActionRevision) via the same synchronous run engine as StartPipelineExecution. New ActionNotFoundException for an unknown stage/action (previously silently accepted)."}
  PutApprovalResult: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed multiple real bugs at once (see 'Bugs found and fixed this pass' below): the wire-shape field name mismatch (approvalResult -> result), the entirely-unparsed required token field, the RFC3339-string approvedAt (should be epoch seconds), and the complete absence of any state mutation. Now implements the real token-handshake: validates the action is an Approval-category action with an open (InProgress) approval request, matches token, and resumes (Approved) or fails (Rejected) the paused pipeline execution."}
  RetryStageExecution: {wire: ok, errors: partial, state: ok, persist: ok, note: "fixed (was a stub: fabricated an InProgress PipelineExecution response never written to executionsStore). Now requires an actually-Failed/Abandoned action in the given stage/execution (StageNotRetryableException otherwise, matching real AWS's real precondition), resets it (FAILED_ACTIONS) or the whole stage (ALL_ACTIONS), and resumes the SAME execution via the shared run engine. retryMode was previously parsed but silently dropped -- now threaded through and validated. 2026-09-07 (gopherstack-3djp): errtargetaudit flagged PipelineExecutionNotFoundException on an unknown executionID -- confirmed not in this op's modeled error set; left unfixed, no declared code is an obvious replacement. See PARITY.md header note."}
  RollbackStage: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (was a stub: fabricated an unpersisted InProgress PipelineExecution with a random new ID). Now requires the target execution to have actually succeeded through the given stage (UnableToRollbackStageException otherwise), and creates+persists a real ROLLBACK-type PipelineExecution with RollbackMetadata.RollbackTargetPipelineExecutionId set, replaying that stage's actions as Succeeded."}
  OverrideStageCondition: {wire: FIXED, errors: partial, state: partial, persist: n/a, note: "still validates-only (pipeline/stage/execution existence, conditionType enum) and mutates no modeled state -- see gaps. Fixed a real bug this pass: conditionType validation only accepted BEFORE_ENTRY, but the real types.ConditionType enum has exactly two values, BEFORE_ENTRY and ON_SUCCESS -- a real client requesting ON_SUCCESS override was wrongly rejected with ValidationException. Now both are accepted. Backend comment rewritten to precisely name the real mutation this op can't perform (StageState.{BeforeEntryConditionState,OnSuccessConditionState}.LatestExecution.Status -> Overridden) and exactly why (StageDeclaration here has no BeforeEntry/OnFailure/OnSuccess members at all -- CreatePipeline never parses them, so there is no condition state anywhere to flip). 2026-09-07 (gopherstack-3djp): errtargetaudit flagged PipelineExecutionNotFoundException on an unknown executionID -- confirmed not in this op's modeled error set; left unfixed, no declared code is an obvious replacement. See PARITY.md header note."}
  DisableStageTransition: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-04 (gopherstack-ary): fixed -- previously bookkeeping-only (wrote stageTransitions, echoed via GetPipelineState, but runPipelineActions never consulted it, so a disabled transition never stopped a run). Now enforced by runPipelineActions: a stage not yet entered is not started while its inbound transition is disabled; a completed non-final stage does not hand off to the next one while its outbound transition is disabled. See ops-block header note for full detail and SDK citation."}
  EnableStageTransition: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-04 (gopherstack-ary): now re-drives runPipelineActions for the pipeline's InProgress executions after clearing the disabled record, so a run parked at this gate resumes without a further client call (real AWS resumes automatically once re-enabled; this backend has no background poller, so EnableStageTransition itself must trigger the resume, mirroring PutApprovalResult's existing resume pattern)."}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDeployActionExecutionTargets: {wire: gap, errors: fixed, state: n/a, persist: n/a, note: "gopherstack-2wvq (2026-08-21): handler unconditionally required pipelineName, but ListDeployActionExecutionTargetsInput marks only ActionExecutionId required (codepipeline@v1.49.4 api_op_ListDeployActionExecutionTargets.go) -- pipelineName is an optional filter, and ActionExecutionId (previously unvalidated and discarded via `_ = executionID`) is the field a real client must send. Added a region-wide ActionExecutionId scan across actionExecutionsStore (no existing global index to reuse, unlike wafv2's ARN fix) so an ActionExecutionId-only request now resolves; new ActionExecutionNotFoundException when it matches nothing. wire remains gap: still no deploy-target model, so Targets is always an empty (but now correctly gated) list -- see gaps."}
families:
  webhooks: {status: FIXED, note: "Re-diffed against types.WebhookDefinition/ListWebhookItem/WebhookAuthConfiguration/WebhookFilterRule this pass (previously only spot-verified via test suite, not re-diffed). Definition/AuthenticationConfiguration/Filters/URL/ARN all confirmed correct. Found and fixed a real gap: PutWebhookInput.Tags was parsed into the handler's `in.Tags` but never passed to the backend (never stored) and never included in either PutWebhook's or ListWebhooks' response -- real PutWebhookOutput.Webhook/ListWebhookItem both carry Tags as a top-level member alongside Definition. Now stored (reusing the same Webhook.Tags map[string]string field TagResource/UntagResource/ListTagsForResource already used, previously write-only from PutWebhook's perspective) and returned under the real 'tags' key on both operations. ErrorCode/ErrorMessage (real ListWebhookItem members reporting third-party registration failures) remain unpopulated -- see gaps. LastTriggered is real-shaped as a string here but is never actually set by anything (no HTTP listener models an inbound webhook POST) so the type mismatch with the real *time.Time is latent, not yet observable -- flagged in Notes. 2026-08-19: re-diffed WebhookAuthConfiguration independently and found a real bug this pass's 'confirmed correct' claim missed -- real types.WebhookAuthConfiguration (awsAwsjson11_deserializeDocumentWebhookAuthConfiguration / awsAwsjson11_serializeDocumentWebhookAuthConfiguration) uses the CAPITALIZED wire keys 'SecretToken'/'AllowedIPRange', unlike every other WebhookDefinition/ListWebhookItem member (all lowerCamelCase). gopherstack used lowercase 'secretToken'/'allowedIPRange' on BOTH the PutWebhook request parse and the PutWebhook/ListWebhooks response emit (same Go struct, WebhookAuthConfig, backs both directions) -- a real client's IP/GITHUB_HMAC AuthenticationConfiguration was silently dropped server-side on write (unrecognized JSON key) and never echoed back on read. Fixed the struct's json tags. Two pre-existing wrong-key tests (TestHandler_Webhook_FullModel's 'webhook with filters and GITHUB_HMAC auth'/'webhook with IP auth' cases, webhooks_test.go) asserted the lowercase keys as correct fixtures without ever checking the response echoed the value back -- corrected to the real keys and strengthened to assert the round-tripped value. New test: TestPutWebhook_AuthenticationConfigurationRoundTrip (webhooks_wire_test.go), real-SDK-client round trip through PutWebhook+ListWebhooks."}
  customActionTypes: {status: FIXED, note: "gopherstack-ohm3 follow-up pass (2026-07-30). CreateCustomActionType/ListActionTypes use the legacy types.ActionType shape (id/actionConfigurationProperties/inputArtifactDetails/outputArtifactDetails/settings) and remain genuinely clean, unchanged. GetActionType/UpdateActionType use a DIFFERENT real shape, types.ActionTypeDeclaration (confirmed directly against api_op_GetActionType.go/api_op_UpdateActionType.go: GetActionTypeOutput.ActionType and UpdateActionTypeInput.ActionType are both *types.ActionTypeDeclaration, never *types.ActionType) -- the prior pass's claim that GetActionType was 'genuinely clean' verified it against the WRONG real type (ActionType instead of ActionTypeDeclaration); it had the identical bug UpdateActionType did. Both now build/parse the real ActionTypeDeclaration shape: Executor (*ActionTypeExecutor: Configuration+Type required, Type validated against the real ExecutorType enum {Lambda,JobWorker}, Configuration.LambdaExecutorConfiguration.LambdaFunctionArn required when present -- matches validateActionTypeExecutor/validateExecutorConfiguration/validateLambdaExecutorConfiguration exactly), Id (Category/Owner/Provider/Version all required, matching validateActionTypeIdentifier -- Owner is a required plain string here, NOT the ActionOwner enum ActionTypeId.Owner is), InputArtifactDetails/OutputArtifactDetails (required wrapper, present-vs-nil distinguished via pointer types since the real validator checks for nil not zero value), Description/Permissions/Properties/Urls (optional). New CustomActionType fields (Description/Executor/Permissions/Properties/Urls) store this; a record only ever created via the legacy path (never updated) has Executor/Permissions/Properties/Urls genuinely nil -- GetActionType omits them (matching the real serializer's own `if v.X != nil` behavior) rather than fabricating placeholder Executor data. UpdateActionType now does a REAL merge, not a blind overwrite: only the ActionTypeDeclaration-expressible fields are replaced; Settings/ConfigurationProperties/Tags (which this op's real input has no member to carry) are preserved from the existing record, verified by TestGetActionType_AfterUpdateActionType_ReturnsDeclarationData. ListActionTypesInput.RegionFilter is parsed but never applied (see gaps, low severity, unchanged). 2026-09-07 (gopherstack-3djp): errtargetaudit sweep confirmed 2 real error-code mismatches, both undeclared-code (not wrong-code) shapes: CreateCustomActionType's duplicate-key guard emits InvalidStructureException (not in this op's modeled set). DeleteCustomActionType's not-found guard emits ActionTypeNotFoundException (also not modeled). Left unfixed: an initial pass 'fixed' both by removing the guards, reasoning from the live docs' 'HTTP 200 response with an empty HTTP body' sentence and DeleteCustomActionType's restore-workflow paragraph -- neither holds up as evidence (the empty-body sentence is generic boilerplate present even on ops that DO throw not-found errors, e.g. DisableStageTransition; the restore-workflow paragraph explicitly requires a *never-used* version string, arguing against silent reuse, not for it). Reverted; both guards restored with landmine comments. See PARITY.md header note for full citations."}
  jobsAndThirdPartyJobs: {status: FIXED, note: "Re-diffed against types.Job/JobDetails/JobData/ThirdPartyJob/ThirdPartyJobDetails/ThirdPartyJobData this pass. Found and fixed: (1) PollForJobs' response only ever included {id, nonce} -- real Job also carries AccountId and Data{ActionTypeId, ...}; now includes both (GetJobDetails already included Data.ActionTypeId correctly, so this closes the same gap PollForJobs had). (2) PollForThirdPartyJobs' response used {id, nonce} -- real ThirdPartyJob is {ClientId, JobId}, a DIFFERENT shape: the field is named JobId (wire key 'jobId'), not 'id', and there is NO Nonce on this type at all -- real AWS deliberately withholds the nonce until GetThirdPartyJobDetails (gated behind the ClientId/clientToken pairing), so leaking it at poll time was also a real-data-exposure-shape bug, not just a naming one. Fixed the key name and removed the fabricated 'nonce'; ClientId itself remained unpopulated at the time (fixed 2026-09-06, gopherstack-1y6n -- see above). (3) GetThirdPartyJobDetails' response was missing Data entirely (real ThirdPartyJobDetails.Data.ActionTypeId) -- now included, matching the GetJobDetails fix pattern. Residual gaps (ActionConfiguration/ArtifactCredentials/ContinuationToken/EncryptionKey/InputArtifacts/OutputArtifacts/PipelineContext on JobData/ThirdPartyJobData, and PutJobFailureResult/PutThirdPartyJobFailureResult discarding FailureDetails.Message/Type entirely) are real and NOT fixed this pass -- see gaps. 2026-08-23: re-verified and fixed the FailureDetails gap. Confirmed against validators.go (validateFailureDetails: Message and Type are both `This member is required`) and serializers.go (awsAwsjson11_serializeDocumentFailureDetails: wire keys message/type/externalExecutionId) that a real client always sends Type and cannot omit it. gopherstack's putJobFailureResultInput.FailureDetails struct had no Type field at all (silently dropped on unmarshal, no error) and PutJobFailureResult's backend method discarded Message with `_ = message`; PutThirdPartyJobFailureResult delegated to the same method. Added Job.FailureMessage/Job.FailureType (internal-only, matching how Job.Status itself is already tracked but never echoed by GetJobDetails -- real AWS JobDetails has no Status/FailureMessage/FailureType member either), threaded Type through both handler input structs and both backend method signatures, and now store both on PutJobFailureResult. ExternalExecutionId (optional per the SDK, unlike Message/Type) remains unparsed -- see gaps."}
  stageTransitions: {status: FIXED, note: "2026-08-19: re-diffed against types.TransitionState/StageState for the first time (previously only spot-verified via test suite). Found and fixed 2 real bugs, see GetPipelineState op note above: a fabricated outboundTransitionState member, and wrong keys disabled/reason -> enabled(inverted)/disabledReason on inboundTransitionState. No longer deferred. 2026-09-04 (gopherstack-ary): that pass only re-diffed the WIRE shape -- the STATE dimension was never checked and was a real bug: DisableStageTransition never actually gated pipeline execution at all (see DisableStageTransition/EnableStageTransition op notes). Now fixed and enforced by runPipelineActions."}
  ruleOps: {status: partial, note: "Re-diffed against types.RuleType/RuleExecutionDetail/ListRuleTypesOutput/ListRuleExecutionsOutput this pass. ListRuleExecutions returning an empty list for a known pipeline is genuinely correct/honest, not a stub: this backend has no condition-rule engine anywhere (see OverrideStageCondition), so there is never a real rule execution to report -- confirmed by reading the backend method per parity-principles.md rule 4, not just grepping for an empty return. Found a real gap in ListRuleTypes: real types.RuleType requires InputArtifactDetails (ArtifactDetails{MinimumCount, MaximumCount}) as a non-optional member; this backend's ListRuleTypes only ever sets 'id', omitting it entirely. NOT fixed this pass -- see gaps for why (no verified-correct per-rule-type artifact count to populate it with)."}
  approvalGate: {status: ok, note: "NEW in the 2026-07-23 pass: StartPipelineExecution/PutActionRevision/PutApprovalResult/RetryStageExecution/RollbackStage all now share one action-run engine (action_engine.go) that gates on Approval-category actions with a real system-generated token, exposed only via GetPipelineState (matching real AWS -- there is no other way for a real client to obtain it). This closed 3 of the 4 gaps and all 3 deferred items from the 2026-07-12 audit at once, since they all stemmed from the SAME missing action-state machine. Unchanged, not re-diffed this pass."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "gopherstack-3djp (2026-09-07): OverrideStageCondition/RetryStageExecution/StopPipelineExecution all emit PipelineExecutionNotFoundException for an unknown pipelineExecutionId, a code confirmed NOT in any of their three modeled error sets (codepipeline@v1.49.4 deserializers.go) -- but no single declared code is an obvious, confirmed replacement (NotLatestPipelineExecutionException/StageNotRetryableException/PipelineExecutionNotStoppableException are each plausible, unconfirmed guesses). Needs a follow-up bd issue to pick (or independently verify) the correct code per op."
  - "gopherstack-3djp (2026-09-07): UpdatePipeline emits PipelineNotFoundException for a nonexistent pipeline name, also confirmed NOT in its modeled error set. An update against nothing has no benign no-op outcome; InvalidStructureException is a plausible but unconfirmed replacement. Needs a follow-up bd issue."
  - "gopherstack-3djp (2026-09-07): CreateCustomActionType emits InvalidStructureException on a duplicate category/provider/version, and DeleteCustomActionType emits ActionTypeNotFoundException on a nonexistent type, and DeletePipeline emits PipelineNotFoundException on a nonexistent name -- all three confirmed NOT in their op's modeled error set. A first pass 'fixed' all three by making them idempotent-success, reasoning from each op's live-docs 'HTTP 200 response with an empty HTTP body' Response Elements sentence -- reverted on review: that sentence is generic Response-shape boilerplate present verbatim even on ops that DO throw a not-found error (DisableStageTransition has the identical sentence and models PipelineNotFoundException), so it is not evidence of idempotent/upsert semantics by itself, unlike workmail's campaign-established bar (an explicit 'does not return an error'-equivalent sentence). Needs a follow-up bd issue per op; no safe remedy is currently evidenced for any of the three."
  - "OverrideStageCondition validates pipeline/stage/execution existence and conditionType but mutates no modeled state -- there is no condition-rule/before-entry-condition engine anywhere else in this backend to be inconsistent with (same class as ListRuleExecutions' deliberately scoped-down design). A full fix requires modeling BeforeEntry/OnFailure/OnSuccess as real StageDeclaration input (parsed by CreatePipeline) and StageState.{BeforeEntryConditionState,OnSuccessConditionState,OnFailureConditionState} as real, gating output that StartPipelineExecution/runPipelineActions produces and this op can then flip to Overridden -- a new subsystem, out of scope for this pass. See the rewritten backend comment in pipeline_state.go for the precise real mutation this would need to perform."
  - "ListActionTypes' RegionFilter request parameter is parsed but never applied -- low severity, since this backend already implicitly scopes ListActionTypes to the request-context region (there is no cross-region action-type catalog to filter within in the first place)."
  - "ListRuleTypes omits the real, required RuleType.InputArtifactDetails member entirely -- not fixed this pass because there is no AWS-documented deterministic MinimumCount/MaximumCount per rule provider (Deployment/LambdaInvoke/CloudWatchAlarm/VariableCheck) this pass could verify with confidence; guessing counts would be a fabrication, not a fix."
  - "webhooks: ListWebhookItem.ErrorCode/ErrorMessage (real members reporting third-party webhook-registration failures) are never populated -- this backend's RegisterWebhookWithThirdParty always succeeds, so there is genuinely never a failure to report (same honest-always-empty rationale as ListRuleExecutions)."
  - "jobsAndThirdPartyJobs: JobData/ThirdPartyJobData are only ever populated with ActionTypeId (fixed this pass, see families) -- ActionConfiguration, ArtifactCredentials (AWSSessionCredentials), ContinuationToken, EncryptionKey, InputArtifacts, OutputArtifacts, and PipelineContext are real members with no equivalent anywhere in this backend's Job model (no artifact-store, no STS-session-credential issuance, no pipeline-context propagation from the owning execution to its jobs). A real job worker driven against this backend could not actually do its job (fetch input artifacts, write output artifacts) from this data alone. Not fixed this pass -- large gap, same class as GetPipelineExecution's pre-existing ArtifactRevisions/Variables gap below."
  - "jobsAndThirdPartyJobs: 2026-08-23 -- PutJobFailureResult/PutThirdPartyJobFailureResult now capture and store FailureDetails.Message/Type on the Job record (Job.FailureMessage/Job.FailureType) instead of discarding Message and never parsing Type. FailureDetails.ExternalExecutionId (optional per the SDK) remains unparsed. The larger, still-open gap: neither Job nor JobDetails (the only read-back shapes for a job) has anywhere to surface a stored failure message in real AWS either -- failure detail surfacing happens via GetPipelineExecution/GetActionExecution-style action-execution records, which this service DOES model for normal pipeline actions (ActionExecution.Summary) but jobs (the job-worker-facing side of a custom/third-party action) are a separate, unlinked record here: Jobs are never created by real pipeline execution at all in this backend (the only writer is AddJobInternal, `for testing`), so PutJobSuccessResult has this identical gap for the success path too. Fixing this properly means modeling Job creation from runPipelineActions and linking Job records back to their originating ActionExecution, out of scope for this pass."
  - "ListDeployActionExecutionTargets always returns an empty list for a resolved execution -- no deploy-target model exists (documented in source, consistent with ListRuleExecutions' scoped-down design). gopherstack-2wvq (2026-08-21) fixed the over-validation that required pipelineName (see ops); this empty-Targets gap itself is unchanged."
  - "GetPipelineExecution/ListPipelineExecutions omit ArtifactRevisions/Variables/SourceRevisions/StatusSummary/StopTrigger -- no artifact-store content model, pipeline-variable resolution engine, or stop-reason tracking exists anywhere else in this backend to source real values from (all are optional fields, SDK-safe to omit)."
  - "handleError's ResourceInUseException (DeleteCustomActionType) and InvalidActionException (dispatch's unknown-action fallback) both name no type codepipeline@v1.49.4 declares; left unfixed because no operation's own deserializer models a matching code to substitute -- see the 2026-08-29 errcodeaudit note near the top of this file for full SDK citations."
  - "2026-09-04 (gopherstack-ary): built-in action providers never actually do anything -- runOneAction (action_engine.go) marks every non-Approval action Succeeded unconditionally regardless of ActionTypeID.Category/Provider/Owner. Action configurations are accepted and stored (CreatePipeline) but are otherwise inert. 2026-09-06 (gopherstack-cb9l) PARTIALLY FIXED: the two candidates with a real backing service and a clear synchronous success/failure signal are now wired. Build/CodeBuild's ProjectName (Configuration key, AWS-documented, not part of the SDK's opaque Configuration map[string]string) now calls codebuild.StartBuild via a new CodeBuildStarter interface (interfaces.go, wired in cli.go's wireCodePipelineCodeBuild); a project StartBuild can't find fails the action (matching real AWS's StartBuild ResourceNotFoundException), and acceptance alone is treated as success since this engine runs synchronously while CodeBuild's own emulator only ever completes a build asynchronously via its janitor (services/codebuild/janitor.go) -- and can never report a build FAILED at all in this emulator (checked: only SUCCEEDED/STOPPED are ever set). Invoke/Lambda's FunctionName now synchronously calls lambda.InvokeFunction (RequestResponse) via a new LambdaInvoker interface, same shape as the LambdaInvoker interface already repeated across sns/stepfunctions-asl/eventbridge/etc.; an invocation error (e.g. unknown function) fails the action. This does NOT model real AWS's actual mechanism for this action type -- the Lambda function receives a CodePipeline.job event and reports success/failure asynchronously via its own PutJobSuccessResult/PutJobFailureResult call (the same Job/JobDetails machinery this file's jobsAndThirdPartyJobs gap already documents as unlinked from real pipeline execution) -- so a function that runs fine but never calls back, or is buggy in a way that returns normally without erroring, is indistinguishable from success here. Deliberately NOT wired this pass: S3 source/deploy, CodeDeploy, and every other built-in provider -- still inert, unchanged. Fresh regression tests (action_engine_cross_service_test.go) hand-reverted action_engine.go against `git show HEAD:<path>`, confirmed both wired-failure cases fail with the predicted symptom (status \"Succeeded\" instead of \"Failed\") against the unmodified code, restored and diff -q verified byte-identical."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "OverrideStageCondition deep state modeling (see gaps) -- requires a condition-rule engine that does not exist anywhere in this backend."
  - "JobData/ThirdPartyJobData completeness (see gaps) -- requires an artifact-store content model and STS-style session-credential issuance, neither of which exist anywhere else in this backend."
  - "ArtifactRevisions/Variables/SourceRevisions/StatusSummary/StopTrigger completeness on GetPipelineExecution/ListPipelineExecutions -- requires an artifact-store content model / pipeline-variable resolution engine / stop-reason tracking, none of which exist anywhere else in this backend."
  - "Cross-service action dispatch (see gaps, 2026-09-04/2026-09-06) -- Build/CodeBuild and Invoke/Lambda now wired (gopherstack-cb9l); S3 source/deploy, CodeDeploy, and every other built-in provider remain a per-ActionTypeId dispatch subsystem this pass did not scope further."
leaks: {status: clean, note: "DeletePipeline now cascade-clears executionsStore, actionExecutionsStore, AND actionRevisionsStore (the last one is new this pass) for the deleted pipeline name; StopPipelineExecution now abandons+clears the token of any action execution left InProgress on a pending approval gate rather than leaving it silently unresolved forever; no goroutines/janitors in this service"}
---

## Notes

Protocol: awsjson1.1 (single POST endpoint, `X-Amz-Target: CodePipeline_20150709.<Op>`).
Confirmed 2026-08-19 from `deserializers.go`'s `awsAwsjson11_deserializeOpError<Op>`
function-name prefix (JSON-RPC, exact-match keys -- casing differences are real bugs, not
just style, unlike EC2-query/RestXML's case-insensitive decode).
Route matching in `handler.go`'s `RouteMatcher` is a simple header-prefix check; all 39
ops in `GetSupportedOperations()` are reachable through `dispatchTable()` -- verified no op
is registered in one list but missing from the other (unchanged this pass).

### 2026-08-19 wrapper-key/nested-shape sweep

All 39 ops enumerated from `GetSupportedOperations()` (handler.go) swept layer-1
(wrapper key) + layer-2 (per-item nested shape) against each op's own deserializer in
`deserializers.go` (SDK pinned `codepipeline@v1.49.4`), independent of this file's prior
claims. 4 real, previously-undetected bugs found and fixed (all narrow-blast-radius wire
bugs, not stubs) -- see the `GetPipelineState`/`ListPipelines` op notes and the `webhooks`/
`stageTransitions` family notes above for full citations. Two prior-pass raw-body tests
were found asserting the wrong (fabricated/miscased) keys as correct fixtures and were
corrected. All 4 fixes were proven by hand-revert (temporarily reintroducing the bug,
confirming the new test reproduces the exact predicted symptom, then restoring and
confirming a byte-identical diff). One incidental layer-3 fix (GetPipelineState's
Created/Updated, previously never emitted at all) was made while auditing that op's
wrapper key -- layer-3 (never-emitted members) was otherwise out of scope as a hunt this
pass. `ListWebhooks`'s top-level `NextToken` (capitalized, unlike every other paginated
op's lowercase `nextToken`) was independently re-verified and found ALREADY correct
(fixed in a prior pass) -- confirms it is a genuine, deliberate AWS wire quirk rather than
a fluke.

### Persistence: snapshot version bumped 1 -> 2

`actionExecutions` (per-pipeline action-execution history) and the new `actionRevisions`
(PutActionRevision tracking) are now persisted in `backendSnapshot` (`persistence.go`).
Previously `actionExecutions` was deliberately NOT persisted, on the reasoning that it was
purely derived state, safely rebuildable by re-running `StartPipelineExecution`. That
reasoning broke the moment an execution could genuinely pause mid-run: the approval gate's
system-generated `Token` lives ONLY on its `ActionExecution` record, so losing
`actionExecutions` across a snapshot/restore cycle would permanently strand any paused
execution -- there would be no way to ever resume it. `codepipelineSnapshotVersion` is bumped
to 2; any snapshot written by an older build is safely discarded on restore (never
partially/incorrectly decoded), per the existing version-guard contract in `Restore`.

### Bugs found and fixed this pass

1. **The approval-gate action-state machine did not exist**, which was the ROOT CAUSE of 3
   of the 4 `gaps` and all 3 `deferred` items from the 2026-07-12 audit simultaneously:
   `RetryStageExecution`/`RollbackStage`/`OverrideStageCondition`/`PutActionRevision`/
   `PutApprovalResult` all validated their pipeline existed and then either fabricated an
   unpersisted response or returned a correct-shaped void response, with no way to reach
   the real preconditions those ops require (a genuinely-failed stage, a genuinely-completed
   target execution, an open approval request) because `StartPipelineExecution` always
   marked every action `Succeeded` synchronously with no exceptions. Fixed by adding a shared
   run engine (`action_engine.go`'s `runPipelineActions`) that gates on the first unresolved
   Approval-category action: it is recorded `InProgress` with a fresh system-generated token
   and processing stops there, exactly mirroring the transient wait a real client observes.
   `PutApprovalResult` (`approvals.go`) now implements the real token handshake and
   resumes/fails the paused execution; `RetryStageExecution`/`RollbackStage`
   (`pipeline_state.go`) now have a genuine failed/succeeded stage to operate against and
   persist real, mutated state.

2. **`PutApprovalResult`'s wire shape used the wrong JSON key for its required `result`
   member** (`handler_approvals.go`): the handler parsed `approvalResult`, but the real
   SDK always serializes this member as `result`
   (verified against `awsAwsjson11_serializeOpDocumentPutApprovalResultInput` in the real
   SDK's `serializers.go`) -- a real client's request would have silently no-opped this
   field every time. The required `token` field was not parsed AT ALL (real AWS requires it
   to identify which open approval request is being resolved; obtaining it is only possible
   via `GetPipelineState`). Fixed: renamed to `result`, added `token` (now required,
   `errInvalidRequest` if missing).

3. **`PutApprovalResult`'s `approvedAt` was an RFC3339 string**, but every other timestamp on
   this service's awsjson1.1 wire is an epoch-seconds JSON number (the protocol's standard
   timestamp format, and the convention `pkgs/awstime.Epoch` exists to enforce elsewhere).
   Fixed: `approvedAt` is now `float64` epoch seconds.

4. **`GetPipelineExecution`/`ListPipelineExecutions` field-completeness fix introduced (and
   then self-corrected) a shape-conflation bug during this pass**: `PipelineExecution` (the
   `GetPipelineExecution` detail shape) and `PipelineExecutionSummary` (the
   `ListPipelineExecutions` list-item shape) are DIFFERENT SDK shapes with only partial field
   overlap -- `PipelineExecution` has no `startTime`/`lastUpdateTime` at all, while
   `PipelineExecutionSummary` has no `pipelineName`/`pipelineVersion` at all (verified
   field-by-field against `awsAwsjson11_deserializeDocumentPipelineExecution` and
   `awsAwsjson11_deserializeDocumentPipelineExecutionSummary`). An earlier draft of this
   fix built one shape by deriving the other via `delete()`, which leaked `startTime`/
   `lastUpdateTime` into the detail shape. Caught before landing by diffing against the real
   deserializers field-by-field (parity-principles.md rule 2) and corrected: two independent
   builders (`pipelineExecutionDetail`, `pipelineExecutionSummary` in
   `handler_pipeline_executions.go`) sharing only the sub-shapes that are genuinely identical
   (`trigger`, `rollbackMetadata`).

5. **`UpdatePipeline` rejected a pipeline-version mismatch with a fabricated
   `ConflictException`** -- flagged as a defensible-but-unverified judgment call in the prior
   audit pass. Verified this pass against the real SDK's field documentation:
   `PipelineDeclaration.Version` is documented as "the version number of the pipeline...
   incremented when a pipeline is updated" with no mention anywhere of the caller's input
   value being validated against the current version, and the real UpdatePipeline API/CLI
   documentation describes an update as always incrementing the version by exactly 1
   regardless of what was sent. This confirms it as gopherstack-invented behavior with no
   basis in the real API. Fixed: the version-mismatch check has been removed entirely;
   `UpdatePipeline` always succeeds and always increments the version by 1.

6. **`DeletePipeline` did not cascade-clear `actionRevisions`** (the new PutActionRevision
   tracking store added this pass) -- would have been an immediate re-introduction of the
   same stale-data leak class fixed for `actionExecutions` in the prior pass, for a
   same-named pipeline recreated after delete. Fixed as part of adding the store, not as a
   follow-up.

7. **`StopPipelineExecution` did not account for a genuinely-InProgress execution** (only
   possible now that the approval gate exists) -- would have left a stopped execution's
   pending approval action silently `InProgress` forever, with its token still valid and
   resurrectable by a later `PutApprovalResult` even after the execution was supposedly
   stopped. Fixed: stopping now abandons (`Abandoned` status) and clears the token of any
   action left `InProgress` on an approval gate.

### Traps for the next auditor (looks-wrong-but-correct)

- `ListRuleExecutions`, `ListRuleTypes`, and `ListDeployActionExecutionTargets`
  deliberately return empty/static data for a *known* pipeline and `ErrNotFound` for
  an unknown one -- still not a disguised stub (see `gaps`). `ListDeployActionExecutionTargets`
  no longer requires `pipelineName` (gopherstack-2wvq, 2026-08-21): a request identifying
  the execution by `ActionExecutionId` alone now resolves via a region-wide scan.
- `OverrideStageCondition` validates pipeline/stage/execution/conditionType (real backend
  logic, improved this pass) and otherwise mutates no state, which is the AWS-correct wire
  shape for this op (`OverrideStageConditionOutput` carries no fields). It remains the ONE
  op from the 2026-07-12 `gaps` list still validate-only, because there is genuinely no
  condition-rule/before-entry-condition state anywhere else in this backend for it to
  override -- unlike `PutApprovalResult`/`RetryStageExecution`/`RollbackStage`, which all
  became real once the approval-gate action-state machine existed to give them a genuine
  precondition to act on.
- `PipelineExecution.Status` values used here (`InProgress`, `Succeeded`, `Stopped`,
  `Failed` -- new this pass, reached via a rejected/never-retried approval) are all real
  `PipelineExecutionStatus` enum values. `Cancelled` and `Superseded` are still never
  produced (this backend has no path that supersedes a running execution with a newer one).
- `ActionExecution.Status` can now be `Abandoned` (real `ActionExecutionStatus` enum value),
  reached only via `StopPipelineExecution` stopping an execution paused on an approval gate.
- `PutApprovalResult`'s exact error precedence when NO open approval request exists at all
  for a stage/action (vs. one that already resolved) is an interpretive call: this backend
  returns `ApprovalAlreadyCompletedException` for both cases (no real AWS documentation
  distinguishes "never reached" from "already resolved" for this op) -- a defensible choice,
  flagged for verification against SDK integration tests in a future pass, same as the
  now-fixed `UpdatePipeline` version judgment call was.

(The "this pass"/"unchanged this pass" language in the two sections above refers to the
2026-07-23 audit. See the dated section below for the 2026-07-30 pass.)

### 2026-07-30 pass: re-diff of webhooks/customActionTypes/jobsAndThirdPartyJobs/ruleOps + OverrideStageCondition

This pass's brief was narrower than 2026-07-23's: (1) determine what `OverrideStageCondition`
should actually mutate and either fix it or document the gap plainly, and (2) re-diff the
families the 2026-07-23 pass explicitly flagged as "spot-verified via test suite, NOT
re-diffed against the SDK" (`webhooks`, `customActionTypes`, `jobsAndThirdPartyJobs`,
`ruleOps` -- `stageTransitions` was in that same flagged list but fell outside this pass's
explicit scope and remains un-re-diffed, see `deferred`).

**`OverrideStageCondition`**: confirmed via `types.ConditionType`/`types.ConditionExecutionStatus`/
`types.StageState` that the real mutation this op performs is flipping
`StageState.{BeforeEntryConditionState,OnSuccessConditionState}.LatestExecution.Status` to
`Overridden`. Confirmed via `types.StageDeclaration` that this backend's `StageDeclaration`
has no `BeforeEntry`/`OnFailure`/`OnSuccess` members at all -- `CreatePipeline` never parses
them, so there is no `Conditions`/`RuleDeclaration` state anywhere, and `StageState` here has
no `BeforeEntryConditionState`/`OnSuccessConditionState`/`OnFailureConditionState` to flip in
the first place. This is a genuine, confirmed architectural gap, not a disguised stub with a
self-serving comment: building it would mean a new condition-rule evaluation subsystem
(parsing stage `Conditions`, gating stage entry on rule results, tracking per-execution
`ConditionState`/`RuleState`), not a field patch. Left as an honest validate-only no-op, with
the backend comment rewritten to name the exact real mutation and exact reason it can't
happen. The one real, containable bug found and fixed: `conditionType` validation only ever
accepted `BEFORE_ENTRY`; the real enum also has `ON_SUCCESS`, which was being wrongly rejected
with `ValidationException`. Both values are now accepted.

**`webhooks`**: `PutWebhookInput.Tags` was parsed but silently dropped -- never stored, never
returned from either `PutWebhook` or `ListWebhooks`, even though the underlying
`Webhook.Tags` field already existed and was fully load-bearing for the *separate*
`TagResource`/`UntagResource`/`ListTagsForResource` API family. Fixed: `PutWebhook` now
stores the supplied tags (full-replace semantics on each call, consistent with how `Filters`
and `Authentication` are already treated as a declarative full-replace on every `PutWebhook`,
not a merge), and both `PutWebhook`'s and `ListWebhooks`' responses now include the real
`tags` member.

**`customActionTypes`**: `CreateCustomActionType`/`GetActionType`/`ListActionTypes` are
genuinely clean -- confirmed field-by-field against `types.ActionType`/`ActionTypeId`. The
severe finding is `UpdateActionType`: see `gaps` for the full breakdown. In short, AWS
introduced a newer "custom action type with a Lambda/JobWorker executor" model
(`ActionTypeDeclaration`, requiring `Executor`) for this specific operation, and this
backend's `UpdateActionType` still speaks the older `CreateCustomActionType`-era shape
(`id`/`settings`/`configurationProperties`/`inputArtifactDetails`/`outputArtifactDetails`),
missing `Executor` (required), `Permissions`, `Properties`, `Urls`, and `Description`
entirely. A real SDK client's `UpdateActionType` request would always include `Executor`
(client-side validation middleware requires it before the request is even serialized), so
this handler doesn't reject such a request -- it just silently ignores the executor
configuration a real caller sent, meaning the update "succeeds" while dropping the one thing
the caller was most likely trying to change. Not fixed this pass -- a new-subsystem-sized gap.

**`jobsAndThirdPartyJobs`**: found and fixed three real wire-shape bugs (see `families` for
the fix details): `PollForJobs` was missing `accountId`/`data.actionTypeId` (real `Job` has
both); `PollForThirdPartyJobs` used the wrong field name (`id` instead of the real `jobId`)
AND leaked `nonce`, a field the real `ThirdPartyJob` type doesn't have at all (real AWS
deliberately withholds the nonce until `GetThirdPartyJobDetails`, gated behind the
`ClientId`/`clientToken` pairing -- a real, if emulator-irrelevant, security-model detail);
`GetThirdPartyJobDetails` was missing `data.actionTypeId` entirely. 2026-08-23: also fixed
`PutJobFailureResult`/`PutThirdPartyJobFailureResult`, which parsed `FailureDetails.Message`
off the wire and then discarded it (`_ = message`), and never even had a `Type` field to parse
`FailureDetails.Type` into (a real client-required member, per `validateFailureDetails`) --
both are now stored on `Job.FailureMessage`/`Job.FailureType`. 2026-09-06 (gopherstack-1y6n):
also fixed the `ClientId`/`clientToken` gap called out below -- `PollForThirdPartyJobs` now
lazily issues `Job.ClientID` (a `uuid.NewString()`) the first time a job is polled, echoed as
`ThirdPartyJob.ClientId`, and `AcknowledgeThirdPartyJob`/`GetThirdPartyJobDetails`/
`PutThirdPartyJobSuccessResult`/`PutThirdPartyJobFailureResult` all now reject a
missing-or-mismatched `clientToken` with `InvalidClientTokenException` (confirmed modeled on
all four ops' deserializers). The real `ClientId`/`ClientToken` SDK doc comments are
byte-identical across all five members ("The clientToken portion of the clientId and
clientToken pair used to verify that the calling entity is allowed access to the job and its
details"), confirming the two are the same round-tripped value rather than something derived
from `ActionTypeId` (which many jobs can share). The larger residual gap
(`JobData`/`ThirdPartyJobData` missing `ActionConfiguration`/`ArtifactCredentials`/
`ContinuationToken`/`EncryptionKey`/`InputArtifacts`/`OutputArtifacts`/`PipelineContext`, and
neither `Job` nor `JobDetails` having anywhere to surface a stored failure message back to a
caller in real AWS either, since that requires linking `Job` to its originating
`ActionExecution` -- a subsystem that doesn't exist even for the success path, since `Job`
records are never created by real pipeline execution here, only by `AddJobInternal` for
tests) was not fixed -- see `gaps` for why each specifically can't be derived from what this
backend already models.

**`ruleOps`**: `ListRuleExecutions` returning an empty list is genuinely honest (confirmed by
reading the backend method, not just the empty-looking return -- same verification standard
`parity-principles.md` rule 4 asks for): this backend has no condition-rule engine anywhere
(see `OverrideStageCondition` above), so there is never a real rule execution to report.
`ListRuleTypes`, however, has a real gap: the real `types.RuleType` requires
`InputArtifactDetails`, and this backend's `ListRuleTypes` only ever populates `id`. Left
unfixed rather than guessed, since there is no AWS-documented deterministic
`MinimumCount`/`MaximumCount` per rule provider this pass could verify with confidence.

### 2026-07-30 follow-up pass: gopherstack-ohm3 (UpdateActionType real-shape fix)

Closed the SEVERE finding from the same-day audit pass above. Field-diffed
`types.ActionTypeDeclaration` and its `types.ActionTypeExecutor`/`types.ExecutorConfiguration`/
`types.JobWorkerExecutorConfiguration`/`types.LambdaExecutorConfiguration`/
`types.ActionTypeIdentifier`/`types.ActionTypeArtifactDetails`/`types.ActionTypePermissions`/
`types.ActionTypeProperty`/`types.ActionTypeUrls` members directly against
`aws-sdk-go-v2/service/codepipeline@v1.49.0`'s `types/types.go`, `serializers.go`,
`deserializers.go`, and `validators.go` (the last one to get required-vs-optional exactly
right, not guessed): `Executor`/`Id`/`InputArtifactDetails`/`OutputArtifactDetails` are
required on `ActionTypeDeclaration`; within `Executor`, `Configuration`/`Type` are required
and `Configuration.LambdaExecutorConfiguration.LambdaFunctionArn` is required whenever a
Lambda executor configuration is supplied; within `Id` (`ActionTypeIdentifier`),
`Category`/`Owner`/`Provider`/`Version` are all required (unlike the legacy `ActionTypeId`,
`Owner` here is a plain required string, not the `ActionOwner` enum).

**Also found while checking the read side (in scope per the ticket)**: `GetActionType` had
the identical bug. `api_op_GetActionType.go` confirms `GetActionTypeOutput.ActionType` is
`*types.ActionTypeDeclaration` -- the SAME newer shape `UpdateActionType` uses, not the legacy
`*types.ActionType` shape `CreateCustomActionType`/`ListActionTypes` use. The prior pass's
`customActionTypes` note asserting `GetActionType` was "genuinely clean" was verified against
the wrong real type. Fixed as part of this pass -- `handleGetActionType` now builds the real
`ActionTypeDeclaration` response shape, and now also validates `owner` as required (it wasn't
checked at all before), matching `validateOpGetActionTypeInput`.

New model types added (`models.go`): `ActionTypeExecutor`, `ActionTypeExecutorConfiguration`,
`JobWorkerExecutorConfig`, `LambdaExecutorConfig`, `ActionTypePermissions`,
`ActionTypeProperty` (distinct from the legacy `ActionConfigurationProperty` -- same role,
different real field set: `NoEcho`/`Optional` here vs `Required`/`Secret` there), and
`ActionTypeUrls` (distinct from the legacy `ActionTypeSettings` -- `ConfigurationUrl` has no
legacy equivalent, `ThirdPartyConfigurationUrl` has no declaration equivalent). `CustomActionType`
gained `Description`/`Executor`/`Permissions`/`Properties`/`Urls`, populated only by
`UpdateActionType` (never fabricated for records that only went through `CreateCustomActionType`
-- `GetActionType` omits `executor` etc. entirely for those, matching the real serializer's own
`if v.X != nil` omission behavior rather than inventing placeholder executor data).

**Merge, not overwrite**: `UpdateActionType`'s backend method used to fully replace the stored
record from whatever the handler built. Since the handler now only ever builds
`ActionTypeDeclaration`-shaped fields, a blind overwrite would have silently wiped the legacy
`Settings`/`ConfigurationProperties` set by `CreateCustomActionType` on every real client's
update -- data `ActionTypeDeclaration`'s real input has no member to even express clearing.
Real AWS's `UpdateActionType` cannot destroy data its own input shape can't carry, so this
backend now fetches the existing record and replaces only the `ActionTypeDeclaration`-owned
fields, leaving `Settings`/`ConfigurationProperties`/`Tags` untouched. Verified end-to-end by
`TestGetActionType_AfterUpdateActionType_ReturnsDeclarationData`
(`custom_action_types_test.go`): creates a type with legacy `settings`, calls `UpdateActionType`
with `executor`/`description`, confirms both the new declaration data (via `GetActionType`) and
the untouched legacy `settings` (via `ListActionTypes`) are present afterward.

**Grade**: this was the sole stated reason for the same-day A->B downgrade above ("Flagged as
the primary reason for this pass's overall grade (A->B)"). No other new gap was found or
introduced closing it. Restored to **A**.

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 1 confirmed bug

`GetPipelineState`: {wire: fixed} -- each stage's inbound/outbound transition
state emitted "disabled"/"reason"; real member (types.TransitionState,
deserializers.go) is "enabled" (inverted polarity) and "disabledReason".
Proven via `TestGetPipelineState_EnabledDisabledReasonKeys_RealClient`
(wire_field_fixes_y1zn_test.go), hand-reverted/confirmed-failing/restored/
`md5sum`-verified byte-identical.

## 2026-08-29 pass: dropped-filter sweep (gopherstack-6flj/21my class), 2 confirmed bugs

Re-swept `ListPipelineExecutions`/`ListActionExecutions` for the specific
class this campaign flags as worse than a silent drop: a request field
naming a filter, sort, or precondition that is accepted nowhere, silently
disabling the caller's filter rather than erroring or returning empty.
Confirmed by reading `ListPipelineExecutionsInput`/`ListActionExecutionsInput`
directly in the pinned SDK (`codepipeline@v1.49.4`, `api_op_*.go`) rather than
trusting this file's prior "wire: ok" claims on these two ops, which were
accurate for the response shape but never checked the request `Filter`
member at all.

1. **`ListPipelineExecutionsInput.Filter`** (`types.PipelineExecutionFilter`
   -> `SucceededInStage.StageName`, `types/types.go:1661`,
   `serializers.go:3924`/`4326`) was parsed nowhere in
   `listPipelineExecutionsInput` -- the struct had no `Filter` field at all.
   A real client filtering for executions where a given stage succeeded got
   every execution back unfiltered -- a plausible wrong answer, not an
   empty/error response. Fixed: added `pipelineExecutionFilter`/
   `succeededInStageFilter` wire types and a new backend method
   `StageSucceededInExecution` (mechanical definition: at least one action
   execution recorded for that stage in that run, and every one of them
   `Succeeded`) to `handleListPipelineExecutions`.
2. **`ListActionExecutionsInput.Filter.LatestInPipelineExecution`**
   (`types.LatestInPipelineExecutionFilter`, `types/types.go:1409`,
   `serializers.go:2855`/`3786`) was parsed nowhere in `actionExecutionFilter`
   -- only the flat `PipelineExecutionId` member was read. A real client
   narrowing via this (required-both-subfields) member instead of the flat
   one got every action execution for the whole pipeline back, unfiltered.
   Fixed: `LatestInPipelineExecution.PipelineExecutionId` now resolves into
   the same execution-ID filter the flat member already used. `StartTimeRange`
   (Latest vs All) is accepted but does not change the result -- this backend
   has no cross-execution "latest run" history distinct from a single
   execution's own flat action records, so narrowing to the named execution
   is the complete real behavior this backend's data can honor without
   fabricating a distinction it can't verify.

Both proven via a real `aws-sdk-go-v2` client round trip
(`wire_field_fixes_test.go`:
`TestListPipelineExecutions_SucceededInStageFilter_RealClient`,
`TestListActionExecutions_LatestInPipelineExecutionFilter_RealClient`),
hand-reverted against `git checkout --` on the two touched source files,
confirmed both new subtests fail with the exact predicted symptom (both
executions/actions returned instead of the filtered one) against unmodified
code, restored via a scratchpad copy, `md5sum`-verified byte-identical before
re-applying.

Not reached this pass (scope: `ListPipelineExecutions`/`ListActionExecutions`
request filters specifically, per this campaign's explicit hint that this
service "is full of such fields"): no further dropped filter/sort/precondition
fields were found on any other op -- `ListActionTypes.RegionFilter` (already
documented as a correct-by-absence gap) and `ListActionTypes.ActionOwnerFilter`
(confirmed applied, `handler_custom_action_types.go`) were spot-checked;
`ListPipelines`/`ListWebhooks`/`ListRuleExecutions`/`ListRuleTypes` have no
filter members in the real SDK to have dropped. The remaining ~35 ops were
not re-read this pass (out of scope: this pass targeted the filter/sort/
precondition bug class specifically, not a full re-sweep).

## 2026-08-30: enumcheck struct-field-hop fix (gopherstack-3dzb), 0 confirmed bugs
`cmd/enumcheck` gained struct-field-hop resolution (see xray/comprehend/
mediaconvert PARITY.md same-dated notes for the mechanics). Re-run across
the whole repo produced the same findings as before the fix -- nothing new
surfaced here or anywhere.

codepipeline's single hit, `rules.go:29`'s `"category": "Rule"` inside
`ListRuleTypes`, was manually verified against
`codepipeline@v1.49.4/types/types.go:2242-2248`: `RuleTypeId.Category` is
typed `RuleCategory`, whose ONLY real member (`types/enums.go:501`) is
`"Rule"` -- an exact match. The finding only fired because the wire key
"category" is ambiguous with the unrelated `ActionTypeId.Category`
(`ActionCategory`, Source/Build/Deploy/Test/Invoke/Approval/Compute, no
"Rule"). FALSE POSITIVE, not fixed: the emitted value is correct for the
struct actually being built here.

## 2026-08-31 directed sweep: request-key/silent-empty-default compound bug (gopherstack-uox6 territory), CLEAN

Regenerated the campaign's plural-heuristic candidate list against
`codepipeline@v1.49.4/serializers.go`: only `trigger`/`triggers`. Both hits
(`handler_pipeline_executions.go:105,129`) are response-output keys
(`"trigger": triggerObject(exec.Trigger)`), confirmed correct against
`deserializers.go`'s `case "trigger":` (both `PipelineExecutionSummary` and
`PipelineExecution`) -- not a bug, wrong axis (response, not request).

Went beyond the heuristic, focused on the two operations this file's own
prior entries call out as "full of such fields"
(`ListPipelineExecutions`/`ListActionExecutions`, already fixed) plus every
other filter-bearing decode struct: `ListActionTypes`
(`actionOwnerFilter`/`regionFilter`), `ListRuleTypes` (`regionFilter`),
`PollForJobs`/`PollForThirdPartyJobs` (`actionTypeId`/`maxBatchSize`).

Two findings, both correctly left unfixed:

- `ListRuleTypesInput` also declares a real `ruleOwnerFilter`
  (`serializers.go`'s `SetQuery("ruleOwnerFilter")`) that
  `listRuleTypesInput` doesn't even declare a field for. Checked whether
  this is the compound bug before touching anything: `types.RuleOwner`'s
  *only* enum member is `"AWS"` (`enums.go`), and this backend's
  `ListRuleTypes()` hardcodes every rule type's owner to `ruleOwnerAWS` --
  so no legal filter value can ever produce an observably different result
  than doing no filtering at all. Confirmed non-issue, not a disguised bug;
  left undeclared.
- `PollForJobsInput.QueryParam map[string]string` ("Only jobs whose action
  configuration matches the mapped value are returned") is a real,
  documented narrowing filter that `pollForJobsInput` doesn't declare and
  `PollForJobs` doesn't apply. Not fixed: this backend's `Job` struct
  tracks no per-job action-configuration data at all (`models.go`'s `Job`
  has `ActionTypeID`/`ID`/`PipelineName`/`Nonce`/`Status`/failure fields,
  nothing resembling action configuration), so there is no honest value to
  match `queryParam` against -- implementing it would mean fabricating a
  configuration data model this backend doesn't have, the same restraint
  this file's `RegionFilter`-on-`ListActionTypes` gap already documents.
  Recorded as a gap alongside it, not fixed.

No code changes this pass -- service verdict is CLEAN on this specific axis
across the ops checked. Gates re-run to confirm no regression: `go build`,
`go vet` (repo-wide), `go test -race -count=1`, `golangci-lint run` -- all
clean (`./services/codepipeline/...`), 0 diff.
