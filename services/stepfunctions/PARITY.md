---
service: stepfunctions
sdk_module: aws-sdk-go-v2/service/sfn@v1.45.4
last_audit_commit: b989093b4
last_audit_date: 2026-08-21
overall: A            # Re-audit against `43aa6d65` baseline (2026-07-11 zero-drift pass). This
                       # pass found real drift/gaps despite the "zero drift" label: two commits
                       # ("Parity 4" efc42cbc, "Go refactoring 2" 9d7e36e0) landed on
                       # services/stepfunctions/ since 43aa6d65 and had ALREADY fixed the
                       # previously-documented SEVERE cli.go ECS/Glue/EventBridge wiring gap
                       # (confirmed: cli.go now calls SetECSIntegration/SetGlueIntegration/
                       # SetEventBridgeIntegration) -- that gap entry is removed below as
                       # resolved. This pass then deep-audited the 3 previously "spot-checked
                       # only" deferred families (state machine CRUD, activities, Distributed Map
                       # ItemReader) by field-diffing against aws-sdk-go-v2/service/sfn v1.40.8
                       # types (still pinned, unchanged) and found+fixed real gaps in each: (1)
                       # StartExecution/StartSyncExecution/DescribeStateMachine never resolved
                       # version- or alias-qualified stateMachineArn ARNs at all (a real,
                       # documented AWS feature -- weighted alias routing, version pinning --
                       # entirely unimplemented); (2) CreateStateMachine/UpdateStateMachine never
                       # returned stateMachineVersionArn/revisionId; (3) DescribeStateMachineVersion
                       # is a FABRICATED op with no counterpart in the real SDK (deleted -- see
                       # notes); (4) CreateActivity/DescribeActivity never supported
                       # encryptionConfiguration or tags (both real AWS fields); (5) SEVERE:
                       # asl.Executor's SetS3Reader (Distributed Map ItemReader S3 CSV/JSON/JSONL
                       # decoding, previously marked "spot-checked, appeared correct") was NEVER
                       # called anywhere in services/stepfunctions/ or cli.go -- an identical
                       # wiring-gap bug class to the just-resolved ECS/Glue/EventBridge one, fixed
                       # by adding a NewS3Integration adapter + cli.go wiring. Also fixed
                       # Retry.JitterStrategy enum validation (was silently permissive). All gates
                       # green (build/vet/gofmt/race-test/lint, 0 banned nolints).
ops:
  CreateStateMachine:
    wire: fixed
    errors: ok
    state: ok
    persist: ok
    note: >
      FIXED this pass: the response never echoed back stateMachineVersionArn
      when publish=true (AWS: "If you do not set the publish parameter to
      true, this field returns null value" -- implying it MUST be populated
      when publish=true; PublishStateMachineVersion's result was previously
      discarded). Also added versionDescription parsing + AWS's documented
      ValidationException when versionDescription is set with publish=false.
      STANDARD/EXPRESS, roleArn validation, tags, logging/tracing config
      unchanged and correct.
  UpdateStateMachine:
    wire: fixed
    errors: ok
    state: ok
    persist: ok
    note: >
      FIXED this pass: UpdateStateMachineOutput.RevisionId and
      .StateMachineVersionArn (publish=true only) were entirely absent --
      the backend method returned only (updateDate, error). Added
      StateMachine.RevisionID (opaque crypto/rand-generated token,
      regenerated every update, matching AWS's "compare between versions ...
      without performing a diff of the properties" semantics), changed
      UpdateStateMachine's signature to (updateDate, revisionID, error), and
      wired both new output fields + the same versionDescription/publish
      ValidationException as CreateStateMachine.
  DeleteStateMachine: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED (error-path sweep, 2026-08-29): raised a fabricated StateMachineDoesNotExist for a missing state machine; DeleteStateMachine's own deserializeOpError models only InvalidArn/ValidationException, so it is now idempotent on a missing state machine, matching AWS."}
  DescribeStateMachine:
    wire: fixed
    errors: ok
    state: ok
    persist: ok
    note: >
      FIXED this pass: did not support version-qualified ARNs at all. AWS:
      "This API action returns the details for a state machine version if
      the stateMachineArn you specify is a state machine version ARN" (and
      echoes the version ARN back as StateMachineArn, unlike execution
      start which always normalizes to the base ARN). This is the REAL
      mechanism AWS uses for fetching version details -- there is no
      separate DescribeStateMachineVersion operation in the actual API (see
      notes; that op was fabricated in this emulator and has been deleted).
      Also now returns the new RevisionID field.
  ListStateMachines: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-dv4s): response marshaled the full StateMachine struct per item, leaking definition/roleArn/status/revisionId/updatedDate/encryptionConfiguration/tracingConfiguration/loggingConfiguration -- real StateMachineListItem (types.go, sfn@v1.45.4) declares only creationDate/name/stateMachineArn/type. Prior 'wire: ok' verified required-field presence, not absence of extras. Now marshals a new stateMachineListItem view; page.Page[T] pagination unchanged."}
  DescribeStateMachineForExecution: {wire: ok, errors: ok, state: ok, persist: ok}
  PublishStateMachineVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteStateMachineVersion: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED (error-path sweep, 2026-08-29): raised a fabricated StateMachineVersionDoesNotExist (names no type anywhere in this SDK) for a missing version; DeleteStateMachineVersion's own deserializeOpError models only ConflictException/InvalidArn/ValidationException, so it is now idempotent on a missing version."}
  ListStateMachineVersions: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED (gopherstack-dv4s): response marshaled the full StateMachineVersion struct per item, leaking stateMachineArn/name/definition/roleArn/type/status/description/revisionId -- real StateMachineVersionListItem (types.go, sfn@v1.45.4) declares only creationDate/stateMachineVersionArn. Now marshals a new stateMachineVersionListItem view. ERRORS FIXED (error-path sweep, 2026-08-29): unlike its ListExecutions/ListStateMachineAliases siblings, this op's own deserializeOpError models no StateMachineDoesNotExist -- it now returns an empty page for an unknown stateMachineArn instead of raising."}
  CreateStateMachineAlias: {wire: ok, errors: fixed, state: ok, persist: ok, note: "routingConfiguration weighted versions validated. ERRORS FIXED (error-path sweep, 2026-08-29): raised fabricated StateMachineDoesNotExist/StateMachineAliasAlreadyExists codes naming no type in this SDK; now emits the modelled ResourceNotFound/ConflictException. NOTE: CreateStateMachineAliasInput has no stateMachineArn field on the real wire (AWS derives the target state machine from routingConfiguration's version ARNs) -- this backend still requires stateMachineArn explicitly, so a real typed client can never populate it and this op 404s through any conformant SDK client today. Pre-existing, unrelated to the error-code fix, left for a future pass (see Test_SDKRoundTrip_StateMachineAlias_UpdateDate's comment)."}
  UpdateStateMachineAlias: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED (error-path sweep, 2026-08-29): raised a fabricated StateMachineAliasDoesNotExist for a missing alias; now emits the modelled ResourceNotFound."}
  DeleteStateMachineAlias: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED (error-path sweep, 2026-08-29): raised a fabricated StateMachineAliasDoesNotExist for a missing alias; now emits the modelled ResourceNotFound."}
  DescribeStateMachineAlias: {wire: ok, errors: fixed, state: ok, persist: ok, note: "FIXED (error-path sweep, 2026-08-29): raised a fabricated StateMachineAliasDoesNotExist for a missing alias; now emits the modelled ResourceNotFound."}
  ListStateMachineAliases: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-dv4s): response reused stateMachineAliasEntry (the Describe/Create/Update shape), leaking name/description/routingConfiguration/updateDate -- real StateMachineAliasListItem (types.go, sfn@v1.45.4) declares only creationDate/stateMachineAliasArn. Now marshals a new, distinct stateMachineAliasListItem view; stateMachineAliasEntry stays as-is for Describe/Create/Update, which do carry all those fields."}
  TagResource: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "ERRORS FIXED (error-path sweep, 2026-08-29): the too-many-tags branch of validateTags raised a fabricated TagPolicyViolation; now emits the modelled TooManyTags. The key-too-long/empty-key/value-too-long branches still emit TagPolicyViolation, which also names no type in this SDK -- TagResource's own deserializeOpError models only InvalidArn/ResourceNotFound/TooManyTags, no exception matching a key/value length violation, so no replacement code is confirmed; left as-is per this sweep's restraint rule (report, don't invent). WIRE FIXED (gopherstack-2kph): sfnTagResourceInput.Tags was typed *tags.Tags (JSON object), but the real TagResourceInput.Tags field (sfn@v1.45.4 api_op_TagResource.go) is []types.Tag, serialized as an array of {key,value} objects (serializers.go:3140-3145, awsAwsjson10_serializeOpDocumentTagResourceInput). Every real client call 500'd (\"cannot unmarshal array into ... map[string]string\") and got retried 3x. Now typed []sfnTagEntry, matching the shape CreateStateMachine/CreateActivity's inline tags already used. UntagResource's TagKeys []string and ListTagsForResource's []types.Tag output were already correct and needed no change."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ValidateStateMachineDefinition:
    wire: fixed
    errors: ok
    state: ok
    persist: n/a
    note: >
      FIXED this pass (partial): added Retry.JitterStrategy enum validation
      (recursively, including nested Map/Parallel Iterator/ItemProcessor/
      Branches) -- AWS rejects anything other than "FULL"/"NONE"/omitted
      with ValidationException at Create/UpdateStateMachine time; this
      emulator previously accepted any string silently (bd: gopherstack-xtl,
      closed). Still JSON/structural validation only beyond that one check
      -- other deep ASL semantic checks (e.g. ToleratedFailure+INLINE
      combos) remain unimplemented, see gaps.
      FIXED 2026-08-21 (bd gopherstack-r80d, batch 10): the FAIL-path
      diagnostic map only ever set "message"/"code", leaving
      ValidateStateMachineDefinitionDiagnostic.Severity (types.go:1559-1586,
      required) as the client-side zero value on every invalid definition
      -- the common case this op exists to exercise, not an edge case.
      Fixed by adding "severity": "ERROR" (ValidateStateMachineDefinitionSeverity
      declares exactly ERROR/WARNING, enums.go:464-470; this handler only
      ever emits the FAIL/blocking case, never WARNING).
  StartExecution:
    wire: fixed
    errors: ok
    state: ok
    persist: ok
    note: >
      FIXED this pass (severe): StartExecution never resolved version- or
      alias-qualified stateMachineArn ARNs -- AWS documents all three input
      shapes (unqualified / stateMachineArn:N version / stateMachineArn:name
      alias) as valid, with alias ARNs applying weighted routing across 1-2
      versions. The pre-existing code did a direct, exact-match
      b.stateMachines.Get(stateMachineArn) lookup, so ANY qualified-ARN
      StartExecution call failed with StateMachineDoesNotExist even though
      CreateStateMachineAlias/PublishStateMachineVersion (the resource CRUD
      side) were fully implemented and previously marked "ok" -- the two
      halves of this feature were never connected. Added
      resolveExecutionTarget() (qualified_arn.go): resolves unqualified/
      version/alias ARNs to the target version's frozen
      Definition/RoleArn/Type, keyed by the BASE (unqualified) ARN for
      execution-ARN construction (AWS never carries a qualifier into
      execution ARNs), with weighted random version selection for 2-entry
      alias routing configs. Added Execution.StateMachineVersionArn/
      StateMachineAliasArn (AWS DescribeExecutionOutput fields, previously
      entirely absent), populated only when the qualifier was used, per
      AWS's documented null-when-unqualified semantics. ClientRequestToken
      idempotency and EXPRESS's immediate-name-reuse semantics remain
      unmodeled (bd: gopherstack-1sf).
  StartSyncExecution:
    wire: fixed
    errors: ok
    state: ok
    persist: ok
    note: >
      FIXED this pass: same qualified-ARN resolution gap as StartExecution,
      fixed via the same resolveExecutionTarget() helper.
  StopExecution: {wire: ok, errors: ok, state: ok, persist: ok, note: "cancels the execution's context via cancelFns; goroutine exits promptly"}
  RedriveExecution: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeExecution:
    wire: fixed
    errors: ok
    state: ok
    persist: ok
    note: >
      FIXED this pass: StateMachineVersionArn/StateMachineAliasArn added
      (see StartExecution). NOT fixed this pass (new finding, field-diffed
      against DescribeExecutionOutput, filed as bd: gopherstack-f5dc):
      RedriveStatus/RedriveStatusReason, MapRunArn (would need Distributed
      Map child-execution architecture this emulator doesn't have --
      iterations run in-process, not as separate Execution records),
      TraceHeader (X-Ray passthrough, not even parsed as a StartExecution
      input), and InputDetails/OutputDetails (CloudWatchEventsExecutionDataDetails,
      always {truncated:false} in practice) remain absent.
  ListExecutions:
    wire: fixed
    errors: fixed
    state: ok
    persist: ok
    note: >
      FIXED (gopherstack-dv4s): response marshaled the full Execution
      struct per item, leaking input/output/error/cause -- real
      ExecutionListItem (types.go, sfn@v1.45.4) has no such fields. Now
      marshals a new executionListItem view. NOTE: ExecutionListItem also
      declares itemCount/mapRunArn, which the domain Execution struct here
      does not track at all -- a separate missing-field gap (not
      over-wide), left for a future pass.

      ERRORS FIXED (error-path sweep, 2026-08-29): ListExecutions models
      StateMachineDoesNotExist but the backend never checked stateMachineArn
      existence at all -- an unknown ARN silently returned an empty page
      (missing-error: success where AWS raises). Now raises
      StateMachineDoesNotExist for an unknown stateMachineArn.
  GetExecutionHistory: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-21 (bd gopherstack-r80d, batch 10; closes the resource/region/parameters portion of gopherstack-996, open since 2026-07-05): TaskScheduledEventDetails.Region/Parameters (types.go: 1311-1339, both required) were never set at all -- RecordTaskScheduled only ever populated Resource/ResourceType. TaskSucceededEventDetails. Resource/ResourceType (types.go:1431-1450, required) and TaskFailedEventDetails.Resource/ResourceType (types.go:1289-1307, required) were also never set. All four are reachable on every normal Task-state execution, not an edge case. Fixed by threading state.Resource through RecordTaskSucceeded/RecordTaskFailed (asl/ executor.go's HistoryRecorder interface gained a resource param on both) and the resolved post-Parameters-template task input through RecordTaskScheduled for Parameters, with Region derived via the existing regionFromARN(resource, backend.region) helper (same one used for activity ARNs elsewhere in this package). gopherstack-996's remaining scope (TaskSubmitted/TaskStarted events for .sync/ waitForTaskToken integration patterns) is a structural gap, not a dropped-field bug -- this emulator never models those event kinds at all, so no HistoryEvent ever claims to be one; left open, see gaps."}
  CreateActivity:
    wire: fixed
    errors: ok
    state: ok
    persist: ok
    note: >
      FIXED this pass: encryptionConfiguration (real
      CreateActivityInput field, server-side KMS encryption) and tags (real
      CreateActivityInput field) were both entirely unparsed/unsupported.
      Added Activity.EncryptionConfiguration + SetActivityEncryptionConfiguration
      (mirrors SetStateMachineConfigurations' established pattern) and
      inline-tags handling (mirrors CreateStateMachine's h.setTags call).
      Kept CreateActivity(ctx, name)'s existing signature unchanged (~35
      call sites) rather than adding required params.
  DeleteActivity:
    wire: fixed
    errors: fixed
    state: ok
    persist: ok
    note: >
      LEAK FIX this pass: DeleteActivity never cleaned up h.tags for the
      deleted activity ARN (DeleteStateMachine's handler already did this
      for state machines) -- a permanent per-deleted-activity tombstone
      entry in the handler's tags map. Added the same tagsMu-guarded
      cleanup DeleteStateMachine uses.

      ERRORS FIXED (error-path sweep, 2026-08-29): raised a fabricated
      ActivityDoesNotExist for a missing activity; DeleteActivity's own
      deserializeOpError models only InvalidArn, so it is now idempotent on
      a missing activity, matching AWS.
  DescribeActivity: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now returns EncryptionConfiguration (see CreateActivity)"}
  ListActivities: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-dv4s): response marshaled the full Activity struct per item, leaking encryptionConfiguration -- real ActivityListItem (types.go, sfn@v1.45.4) declares only activityArn/creationDate/name. Now marshals a new activityListItem view."}
  GetActivityTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "long-poll with WaitTimeSeconds; task-token issuance"}
  SendTaskSuccess: {wire: ok, errors: ok, state: ok, persist: ok}
  SendTaskFailure: {wire: ok, errors: ok, state: ok, persist: ok}
  SendTaskHeartbeat: {wire: ok, errors: ok, state: ok, persist: ok, note: "States.HeartbeatTimeout enforced against HeartbeatSeconds"}
  DescribeMapRun:
    wire: fixed
    errors: fixed
    state: ok
    persist: ok
    note: >
      ERRORS FIXED (error-path sweep, 2026-08-29): raised a fabricated
      MapRunDoesNotExist for a missing map run -- names no type anywhere in
      this SDK. DescribeMapRun's own deserializeOpError models
      InvalidArn/ResourceNotFound; now emits the modelled ResourceNotFound.

      REVERSED 2026-08-21 (bd gopherstack-r80d, batch 10): a prior pass
      concluded ExecutionCounts having no backing field was "correctly so"
      because this emulator has no distributed-map child-execution model
      (iterations run in-process, not as separate Execution records) --
      true as far as it goes, but the conclusion drawn from it repeats the
      exact mistake this campaign's own candidates file documents and
      reverses for quicksight: "required-but-inapplicable means
      present-and-empty, not absent." ExecutionCounts is required on
      DescribeMapRunOutput (api_op_DescribeMapRun.go:57) and typed as a
      *types.MapRunExecutionCounts pointer client-side -- omitting the key
      entirely left that pointer nil, so a real client dereferencing
      out.ExecutionCounts.Total would nil-pointer panic, not just see a
      zero. Fixed by adding MapRunExecutionCounts (models.go, shape mirrors
      MapRunItemCounts) and an ExecutionCounts field on MapRun, always
      marshaled (no omitempty) with genuinely zero counts -- not fabricated,
      since no per-child-execution data exists to report, matching the
      real semantics of a Map Run that never started separate child
      workflow executions. Same structural gap still documented under
      DescribeExecution's MapRunArn note (bd: gopherstack-f5dc); that one is
      unaffected, ExecutionCounts.Total staying 0 doesn't imply any
      DISTRIBUTED-mode child-execution tracking exists. ItemCounts (a real,
      distinct field) remains present and populated as before.
  ListMapRuns: {wire: fixed, errors: fixed, state: ok, persist: ok, note: "FIXED (gopherstack-dv4s): response marshaled the full MapRun struct per item, leaking status/itemCounts/toleratedFailurePercentage/maxConcurrency/toleratedFailureCount/redriveCount/redriveDate -- real MapRunListItem (types.go, sfn@v1.45.4) declares only executionArn/mapRunArn/startDate/stateMachineArn/stopDate. Now marshals a new mapRunListItem view. ERRORS FIXED (error-path sweep, 2026-08-29): ListMapRuns models ExecutionDoesNotExist but the backend never checked executionArn existence -- an unknown ARN silently returned an empty page. Now raises ExecutionDoesNotExist for an unknown executionArn, with an OR-check against the mapRunsByExecution index so StartSyncExecution's EXPRESS executions -- never inserted into b.executions by design -- still list correctly."}
  UpdateMapRun: {wire: ok, errors: fixed, state: ok, persist: ok, note: "ToleratedFailureCount/Percentage on the MapRun *resource* API were already real; the ASL-definition-level Map state fields were fixed in a prior pass. ERRORS FIXED (error-path sweep, 2026-08-29): raised a fabricated MapRunDoesNotExist for a missing map run -- names no type anywhere in this SDK; now emits the modelled ResourceNotFound."}
  TestState: {wire: ok, errors: ok, state: ok, persist: n/a}
families:
  asl_task:
    status: fixed
    note: "FIXED (gopherstack-tdp6): a Task's \".sync\" Resource suffix was recognized (ErrSyncPatternUnsupported already rejected it for the services AWS documents as Request-Response-only) but for ECS runTask.sync/Glue startJobRun.sync -- the two services AWS DOES support \"Run a Job\" for -- the task was dispatched fire-and-forget and the state advanced immediately with the start-call response, never observing whether the job actually finished or failed. Now, when an optional ECSSyncWaiter/GlueSyncWaiter is wired (asl/executor.go; cli.go's wireStepFunctionsServiceIntegrations wires both whenever ECS/Glue are registered, via adapters in integrations.go polling ecs.DescribeTasks/glue.GetJobRun), invokeECSTask/invokeGlueTask poll every 25ms until the task reaches ECS's STOPPED or the job run reaches a terminal Glue JobRunState, bounded by ctx -- the same TimeoutSeconds-derived deadline every other Task type in this executor already honors, so a job that never completes doesn't hang the execution forever if TimeoutSeconds is set. On success the state's output is the service's own description of the completed job (ECS DescribeTasks shape / Glue GetJobRun's {\"JobRun\": ...} shape), not the start-call response, matching real AWS .sync. On failure (non-zero ECS container exit code; Glue JobRunState STOPPED/FAILED/TIMEOUT/ERROR/EXPIRED) the Task state fails via the existing FailError/Catch machinery. Unwired (no waiter set) stays exactly fire-and-forget, matching pre-fix behavior -- a silent no-op, never a hang. Batch, EMR, SageMaker, and nested Step Functions still have no completion signal and are out of scope; EMR/APIGateway .sync resources already fall through to a generic unsupported-integration TaskFailed (not ErrSyncPatternUnsupported specifically), which was true before this fix too and was left unchanged."
  asl_choice:
    status: ok
    note: "Unchanged this pass."
  asl_map:
    status: fixed
    note: >
      SEVERE FIX this pass: asl.Executor's ItemReader path
      (resolveItemsFromReader, S3 CSV/JSON/JSONL decoding) was already
      correctly implemented and unit-tested at the executor level with
      mocks (asl/intrinsics_extras_test.go's stubS3), and the prior pass's
      PARITY.md carried it forward as "deferred ... spot-checked only,
      appeared correct" -- but nothing in services/stepfunctions/ or cli.go
      ever called InMemoryBackend.SetS3Reader (which didn't even exist as a
      backend method). e.s3 was always nil in any real running gopherstack
      process, so EVERY Distributed Map ItemReader hard-failed with
      ErrS3ReaderNotConfigured -- an identical bug class to the
      ECS/Glue/EventBridge cli.go wiring gap a prior pass found and fixed
      (see notes below). Fixed by: adding InMemoryBackend.s3Reader field +
      SetS3Reader() (store.go), threading it through
      startExecutionLocked/runParsedExecution/StartSyncExecution/
      RedriveExecution alongside the other 6 integrations, adding
      s3Adapter/NewS3Integration (integrations.go, adapts
      services/s3.StorageBackend.GetObject to asl.S3Reader), and wiring
      cli.go's wireStepFunctionsServiceIntegrations to call
      sfnBk.SetS3Reader(sfnbackend.NewS3Integration(s3H.Backend)) alongside
      the existing SQS/SNS/DynamoDB/ECS/Glue/EventBridge calls. Verified
      end-to-end with a real StartExecution against a Map+ItemReader
      definition and a fake S3Reader (services/stepfunctions/s3_item_reader_test.go).
      ResultWriter (S3 write-out, bd: gopherstack-8j8) is now implemented:
      State.ResultWriter is parsed (parser.go), and
      Executor.exportMapResults (asl/result_writer.go) writes
      SUCCEEDED_n.json/FAILED_n.json plus a manifest.json to the wired
      S3Writer on Map completion, returning
      {MapRunArn, ResultWriterDetails:{Bucket,Key}} in place of inline
      results -- verified against AWS docs
      (input-output-resultwriter.html) for the ResultWriter/
      ResultWriterDetails/manifest.json shapes, since aws-sdk-go-v2's sfn
      client has no typed struct for any of these (they're ASL-JSON/
      execution-output only, never part of the control-plane API surface).
      Wired the same way as S3Reader: cli.go's
      wireStepFunctionsServiceIntegrations now also calls
      sfnBk.SetS3ResultWriter(sfnbackend.NewS3ResultWriterIntegration(s3H.Backend)).
      Known deviations: WriterConfig (Transformation/OutputType) is parsed
      but not applied -- only the plain S3-export shape is honored; per-item
      result records omit ExecutionArn/Name/StartDate/StopDate (real AWS
      backs each with a genuine child execution, gopherstack runs Map
      iterations as in-process sub-executors with no such resource to
      point to); the manifest folder segment uses gopherstack's own
      MapRunArn suffix (.../execName/stateName) rather than real AWS's
      Map:<uuid>, since gopherstack's MapRunArn was never shaped like
      AWS's to begin with. When no S3Writer is wired, or Parameters.Bucket
      is unset, the Map state degrades to its pre-existing inline-results
      behavior rather than failing. DEFERRED (unchanged):
      ItemProcessor.ProcessorConfig.Mode (bd: gopherstack-8im) remains
      unimplemented.
  asl_parallel:
    status: ok
    note: "Unchanged this pass."
  asl_wait:
    status: ok
    note: "Unchanged this pass."
  asl_pass_succeed_fail:
    status: ok
    note: "Unchanged this pass."
  asl_intrinsics:
    status: ok
    note: "Unchanged this pass. Non-AWS extras (informational, not a bug) still present -- see gaps."
  json_1_0_protocol:
    status: ok
    note: "Unchanged this pass."
  timestamps:
    status: ok
    note: >
      Pattern-hunt pass (timestamp encoding class, 2026-08-29): protocol
      confirmed JSON-RPC 1.0 (awsAwsjson10_* serializer prefix, sfn@v1.45.4)
      and every *time.Time deserializer call in deserializers.go is
      smithytime.ParseEpochSeconds -- 30 occurrences across
      types/types.go + api_op_*.go, 6 distinct member names (CreationDate,
      StartDate, RedriveDate, StopDate, Timestamp, UpdateDate), no per-field
      trait override. gopherstack already stores every one of these as a
      raw float64 (Unix epoch seconds) end to end -- models.go's own header
      comment documents this explicitly -- and every write site uses
      float64(time.Now().Unix()) or an equivalent, never a time.Time
      marshalled through encoding/json. 0 new wrong-format bugs found. This
      class was already the subject of a prior fix (Test_SDKRoundTrip_
      StateMachineAlias_UpdateDate / Test_SDKRoundTrip_
      DescribeStateMachineForExecution_UpdateDate in
      wire_updatedate_test.go, gopherstack-1ai8): DescribeStateMachineAlias/
      UpdateStateMachineAlias/DescribeStateMachineForExecution's UpdateDate
      wire tag was wrong and decoded nil through the real SDK client; both
      tests still pass against current code, reconfirming the fix holds.
      No Input struct in this SDK carries a *time.Time member, so there is
      no request-side parse direction to check for this service.
filter_semantics: {status: ok, note: "gopherstack-uox6 (value-semantics sweep, 2026-08-30): this service establishes no prior sweep of this kind. First, its protocol: aws-sdk-go-v2/service/sfn@v1.45.4's types package has NO Filter struct at all (grep of types/types.go) -- this API surface has almost no server-side filtering. The one real filter is ListExecutionsInput.StatusFilter (types.ExecutionStatus, a single-value equality field, not a list), applied at executions.go:643 via an exact bucket lookup -- no documented modifier to get wrong. Everything else this service's ~14 hand-rolled 'match' helpers implement is Amazon States Language Choice-state comparators (asl/executor.go), which decide whether a state's input satisfies a rule, not an SDK list filter, but the same right-field-wrong-algorithm risk applies: evaluateChoiceRule's And/Or/Not (correct all/any/negate), IsPresent/IsNull/IsString/IsNumeric/IsBoolean/IsTimestamp (each compares a computed bool against *rule.IsX with ==, correctly honoring both true and false rather than only checking truthiness), and the String/Numeric/Boolean/Timestamp -Equals/-LessThan/-GreaterThan/-LessThanEquals/-GreaterThanEquals families (each Path and literal variant) were all read and are correct. stringMatchesPattern/globMatch (StringMatches) is the one genuine wildcard comparator in this family -- verified against the ASL spec's documented semantics (its own doc comment: '*' matches zero or more chars, backslash escapes the next character, anchored both ends) via a real two-pointer backtracking implementation; correct, including the escape case. No bugs found -- clean verdict."}
gaps:
  - "Map Distributed Map ResultWriter's WriterConfig (Transformation/OutputType) is parsed but not applied, only the plain S3-export shape; per-item result records omit ExecutionArn/Name/StartDate/StopDate since gopherstack Map iterations aren't backed by real child executions (bd: gopherstack-8j8, implemented this pass -- see asl_map_and_distributed_map notes)"
  - "Map ItemProcessor.ProcessorConfig.Mode (INLINE/DISTRIBUTED) not parsed/validated (bd: gopherstack-8im)"
  - "StartExecution has no ClientRequestToken idempotency; EXPRESS's immediate-name-reuse semantics (vs STANDARD's reuse restriction) are not modeled (bd: gopherstack-1sf)"
  - "STALE, corrected 2026-08-23: resourceType/region/parameters were fixed by the 2026-08-21 batch-10 pass (see notes below, TaskScheduledEventDetails.Region/.Parameters and TaskSucceededEventDetails.Resource/.ResourceType) but this line was never updated after that fix landed -- re-verified live against models.go/execution_history.go this pass. Still genuinely open: TaskScheduledEventDetails.TimeoutInSeconds/HeartbeatInSeconds are never set (RecordTaskScheduled has no timeout/heartbeat value in scope to assign); no TaskSubmitted/TaskStarted history events are emitted for .sync/.waitForTaskToken Task states (bd: gopherstack-996)"
  - "STALE, corrected 2026-08-23 (manifest-harvest pass): re-read models.go/executions.go directly instead of trusting this note -- RedriveStatus, TraceHeader, InputDetails, and OutputDetails were already declared on Execution AND already assigned real values at every relevant transition (initializeExecutionRecord/finalizeExecutionRecordLocked/StopExecution/resetExecutionForRedrive); this line's claim that gopherstack-f5dc left them missing was wrong. RedriveStatusReason (real, AWS: 'When redriveStatus is NOT_REDRIVABLE, redriveStatusReason specifies the reason', api_op_DescribeExecution.go) WAS a genuine gap -- declared but never assigned, so real clients always decoded an empty string -- FIXED this pass: populated with AWS's exact documented reason strings ('Execution is RUNNING and cannot be redriven.' / 'Execution is SUCCEEDED and cannot be redriven.') at every NOT_REDRIVABLE transition and cleared at every REDRIVABLE one. MapRunArn remains genuinely absent: Execution.MapRunArn is never assigned anywhere in this backend because Map iterations aren't backed by real child executions -- same structural gap already tracked under bd gopherstack-8j8 above, not a separate bug. Proven via a real aws-sdk-go-v2/service/sfn client round trip (wire_redrivestatusreason_test.go), which also incidentally caught and fixed a second, unrelated real bug it exposed: a bare {\"Type\":\"Fail\"} state (Error/Cause both optional per the ASL spec) was silently recorded as SUCCEEDED, not FAILED, because asl.ExecutionResult had no way to distinguish 'failed with an empty error code' from 'succeeded' other than checking Error != \"\" -- fixed by adding ExecutionResult.Failed and switching every consumer (asl/executor.go's Parallel-branch and Map-iteration paths, executions.go's async and sync finalizers, handler_util.go's TestState) off the Error != \"\" check."
  - "Non-standard intrinsic functions (StringConcat, ArraySlice, MathSubtract, etc.) are accepted by this emulator but do not exist in real AWS Step Functions -- permissive superset, not a correctness bug against valid AWS definitions, but a definition that only works here would fail on real AWS (no bd filed; informational)"
  - "ListExecutions' new executionListItem view (gopherstack-dv4s) omits itemCount/mapRunArn, which real ExecutionListItem declares (types.go, sfn@v1.45.4) -- the domain Execution struct never tracked either field, a missing-field gap distinct from the over-wide leak this pass fixed (bd: unfiled)"
deferred: []
leaks: {status: clean, note: "StopExecution/DeleteStateMachine cancel the execution's context via b.cancelFns; Wait/waitForRetry/execSem/semaphore all select on ctx.Done(); Map/Parallel goroutines (wg.Go) all respect ctx cancellation. FIXED this pass: DeleteActivity leaked a permanent h.tags tombstone entry per deleted activity (see ops.DeleteActivity). No new goroutines introduced this pass (resolveExecutionTarget/S3Reader wiring are synchronous, no new goroutines)."}
---

## Notes

**2026-08-15 (gopherstack-3gbe):** investigated whether Step Functions
shares Omics' (gopherstack-keee) client-side host-prefix-rewrite
reachability gap. It does: **2 ops, one literal prefix, `sync-`** (TestState
`api_op_TestState.go:265`, StartSyncExecution
`api_op_StartSyncExecution.go:232`), confirmed against the pinned
`sfn@v1.45.4` module, exactly matching gopherstack-3gbe's filing.

No routing/auth code needed changing. `Handler.RouteMatcher`
(`handler.go:160`) matches on the `X-Amz-Target` header prefix
(`"AmazonStates."` or `"AWSStepFunctions."`), never `Host` or `Path`, so
header-based dispatch is structurally immune to the path-collision class
this bug family could otherwise cause. The reachability gap is a pure
client-side DNS/dial failure, same as Omics.

stepfunctions already had a real-SDK-client round trip
(`wire_updatedate_test.go`), but it never exercised TestState or
StartSyncExecution, so this family's real-client reachability had never
been proven either way. Added `host_prefix_reachability_test.go` following
`services/omics/host_prefix_reachability_test.go`'s before/after pattern: a
before-fix test proving the unmodified client can't dial either op, and an
after-fix test that drives TestState directly and StartSyncExecution (via
CreateStateMachine of an EXPRESS state machine, since StartSyncExecution is
EXPRESS-only) through a redial-to-the-real-listener transport, leaving the
SDK's real, un-disabled `sync-` rewrite intact on the wire, and asserts both
succeed with correctly decoded values. Gates green: build, vet, race,
`go fix -diff` (no diff), golangci-lint (0 findings).

**This pass's brief was to deep-audit the 3 previously "spot-checked only"
deferred families by actually field-diffing them against
aws-sdk-go-v2/service/sfn v1.40.8 types, not just re-asserting "appeared
correct".** All 3 had real, previously-undiscovered gaps:

**1. State machine CRUD -- qualified-ARN resolution was entirely missing.**
AWS's `StartExecutionInput.StateMachineArn` doc explicitly describes three
valid input shapes: unqualified, version-qualified (`stateMachineArn:N`),
and alias-qualified (`stateMachineArn:name`, with weighted routing across
1-2 versions). This emulator's `CreateStateMachineAlias`/
`PublishStateMachineVersion`/routing-weight-validation (the *resource* CRUD
side) were previously verified `ok` and are indeed correct -- but
`StartExecution`/`StartSyncExecution`/`DescribeStateMachine` never consulted
any of that data; they did a direct, exact-ARN `b.stateMachines.Get()`
lookup that always failed for a qualified ARN. **This is the same shape of
bug as the Map/asl_map disguised-stub found in a prior pass**: two halves of
a feature (resource management + resource consumption) were each
individually correct and individually tested, but never connected, so a
green test suite for either half never caught it. Fixed via
`resolveExecutionTarget()` in the new `qualified_arn.go`. **Trap for the
next auditor**: when a resource type has both a "create/configure" API
surface and a "consume/reference" API surface (aliases+StartExecution,
Map's ItemReader+the S3 integration below), audit them together -- a
family status of "ok" on the config side proves nothing about whether the
consuming side actually resolves what was configured.

**2. DescribeStateMachineVersion is a FABRICATED, non-AWS operation --
deleted.** Verified against the full `aws-sdk-go-v2/service/sfn@v1.40.8`
`api_op_*.go` file listing (37 files, 37 real operations): there is no
`DescribeStateMachineVersion`. AWS's real mechanism for fetching version
details is calling `DescribeStateMachine` with a version-qualified ARN (the
SDK's own `DescribeStateMachineOutput.CreationDate` doc literally says "For
a state machine version, creationDate is the date the version was created"
and `DescribeStateMachineInput.StateMachineArn`'s doc says "If you specify a
state machine version ARN, this API returns details about that version").
This emulator had invented a whole separate wire op (route in
`handler_state_machine_versions.go`, entry in `GetSupportedOperations()`,
`StorageBackend` interface method) for this instead of implementing the
real mechanism. **Fix**: removed the op from `GetSupportedOperations()` and
the handler's dispatch table, removed it from the `StorageBackend`
interface (so `Handler` can no longer route it), and instead extended
`DescribeStateMachine` itself to resolve a version-qualified ARN (echoing
the version ARN back as `StateMachineArn`, per AWS's documented behavior --
notably different from execution-start's base-ARN-always semantics). The
backend method `InMemoryBackend.DescribeStateMachineVersion` was left in
place as a plain internal helper (existing tests call it directly on the
concrete type) since it's harmless non-wire-surface Go code, not a
fabricated AWS operation -- only the wire-level op was deleted. This is the
"gopherstack-invented op, not in the real SDK, DELETE it" bug class from the
polly tagging-surface precedent.

**3. Distributed Map ItemReader S3 decoding: the SEVERE cli.go-style wiring
gap recurred, one level down.** A prior pass fixed cli.go never wiring
`SetECSIntegration`/`SetGlueIntegration`/`SetEventBridgeIntegration` despite
`asl.Executor` fully implementing and unit-testing those integrations. This
pass found the exact same bug class for S3: `asl.Executor.SetS3Reader` /
the `S3Reader` interface / `resolveItemsFromReader`'s CSV/JSON/JSONL
decoding were all correctly implemented and mock-tested
(`asl/intrinsics_extras_test.go`), but `InMemoryBackend.SetS3Reader` didn't
even exist, and nothing called it. Any real Distributed Map with an
`ItemReader` hard-failed with `ErrS3ReaderNotConfigured` in every actual
running gopherstack process. **Trap for the next auditor, restated from the
prior ECS/Glue/EventBridge finding because it just recurred**: an
`asl_*`-family or executor-level "ok"/mock-tested verdict proves the
*executor* dispatches correctly -- it says nothing about whether
`services/stepfunctions/`'s `InMemoryBackend` (let alone `cli.go`) actually
wires the concrete integration through. Every `asl.Executor.SetXIntegration`
call needs a matching audit trail: backend field -> `SetXIntegration`
method -> threaded through `startExecutionLocked`/`runParsedExecution`/
`StartSyncExecution`/`RedriveExecution` -> adapter in `integrations.go` ->
`cli.go` wiring call. Missing any link in that chain reproduces this bug
silently.

**4. Activities: encryptionConfiguration and tags were real, entirely
unparsed `CreateActivityInput` fields.** Field-diffed
`api_op_CreateActivity.go`/`api_op_DescribeActivity.go` against
`activities.go`/`handler_activities.go` and found both fields simply
absent -- not stubbed, just never referenced anywhere. Added
`Activity.EncryptionConfiguration` + `SetActivityEncryptionConfiguration`
(mirrors `SetStateMachineConfigurations`'s established optional-post-create-
config pattern) and inline-tags handling (mirrors `CreateStateMachine`'s).
Also found and fixed a real leak while in this code: `DeleteActivity`'s
handler never cleaned up `h.tags`, unlike `DeleteStateMachine`'s.

**RevisionId / StateMachineVersionArn on Create/UpdateStateMachine.** Field-
diffing `CreateStateMachineOutput`/`UpdateStateMachineOutput` found both
response types missing fields the emulator's own backend logic already had
the data for: `PublishStateMachineVersion`'s result was already being
computed when `publish=true` but thrown away (`_, _ =
h.Backend.PublishStateMachineVersion(...)`) instead of echoed back, and
`RevisionId` didn't exist as a concept anywhere in the backend. Added
`StateMachine.RevisionID` (opaque token, regenerated every
`UpdateStateMachine` call, absent/empty until the first update -- matching
AWS's documented "compare between versions ... without performing a diff"
semantics) and wired it + `StateMachineVersionArn` into both response
types, `DescribeStateMachine`, and added the `versionDescription`-requires-
`publish=true` `ValidationException` AWS documents for both ops.

**Retry.JitterStrategy validation** (bd: gopherstack-xtl, closed this pass):
added a recursive walk (`asl/parser.go`'s `validateJitterStrategies`, covers
nested `Iterator`/`ItemProcessor`/`Branches`) rejecting anything other than
`"FULL"`/`"NONE"`/omitted at `Parse` time, which is called from
`CreateStateMachine`/`UpdateStateMachine`/`ValidateStateMachineDefinition`/
`StartExecution`/`TestState` uniformly.

**Confirmed already-fixed (not by this pass): the previously-documented
SEVERE cli.go ECS/Glue/EventBridge wiring gap.** `git log` showed two
commits (`efc42cbc` "Parity 4", `9d7e36e0` "Go refactoring 2") landed on
`services/stepfunctions/` since the `43aa6d65` baseline despite the prior
pass's "zero drift" framing (that framing compared against a *different*,
even-older baseline, `ce30166a`) -- `cli.go` now calls
`sfnBk.SetECSIntegration`/`SetGlueIntegration`/`SetEventBridgeIntegration`
(verified at `cli.go`'s `wireStepFunctionsServiceIntegrations`, ~L3855-3867).
Removed the stale gap entry.

**Protocol**: json-1.0, unchanged this pass.

## Prior-pass notes (unchanged, retained for history)

See git history for this file's content as of commit `43aa6d65` for the
full prior-pass notes on: the Map `runMapItem` disguised-stub fix, Map/
Parallel Retry+Catch addition, `GetExecutionHistory` detail-object
population fix, the Catch error-output two-key-shape fix, and the
StartExecution-on-EXPRESS / StartSyncExecution-on-STANDARD error-code fixes.
Those `families:`/`ops:` verdicts are carried forward unchanged in the
front-matter above (marked `ok` / not re-noted) since this pass found no new
drift in them.

## 2026-08-21 pass (bd gopherstack-r80d, batch 10): required OUTPUT members never populated

`cmd/requiredoutputfields` puts stepfunctions at 54 required output fields
across 23 ops-with-required (37 ops total) -- the largest remaining
candidate not off-limits (sagemaker, 459 fields, is excluded: a concurrent
agent was mid-conversion of its inline request structs for most of this
session, confirmed via `git status` before and during this pass).

**Wire shape**: stepfunctions is NOT the "one wrapper key around a nested
domain object" shape pinpoint/bedrockagent/cleanrooms have -- most ops have
several top-level required scalar members directly (`CreateActivity`:
`activityArn`+`creationDate`; `DescribeMapRun`: nine required members
directly on the op's own output struct), and every response is built via
tagged Go structs (not `map[string]any` literals like s3tables/codecommit).
So neither of the two documented shapes from prior batches applies at the
op level -- but the flat 54-field/23-op count still badly undercounts the
real required surface, for a THIRD reason not yet named in this campaign:
stepfunctions' List ops return arrays of dedicated `*ListItem` structs
(`ActivityListItem`, `ExecutionListItem`, `MapRunListItem`,
`StateMachineAliasListItem`, `StateMachineListItem`,
`StateMachineVersionListItem`), and `GetExecutionHistory` returns an array
of polymorphic `HistoryEvent` structs whose `*EventDetails` sub-objects
(`TaskScheduledEventDetails`, `TaskSucceededEventDetails`,
`TaskFailedEventDetails`, plus nine more event-kind detail types this
emulator never emits) each carry their own required members --
`cmd/requiredoutputfields`'s flat per-op scan only sees each op's own
top-level `<Op>Output` struct and has no visibility into a list *element*
type's own requiredness, or a HistoryEvent's own nested detail structs. Read
every nested/list-item/detail type in `types.go` (extracted via a one-off
AST-style walk over every `type X struct { ... }` block, not a grep window,
per this campaign's own standing method warning) against `models.go` and
every handler that constructs one, not just the 23 ops the tool names.

**Trusted vs re-derived**: this file already had several prior passes
(2026-07-11 zero-drift baseline, 2026-07-23 re-audit, plus dv4s's List-item
over-wide-leak fixes for every `List*` op named above) that verified
`ListActivities`/`ListExecutions`/`ListMapRuns`/`ListStateMachines`/
`ListStateMachineAliases`/`ListStateMachineVersions` each already emit a
correctly-narrow `*ListItem` struct with all of ITS required members
present and non-omitempty -- re-spot-checked one (`ActivityListItem`) against
`types.go` rather than re-deriving all six from scratch; all six confirmed
still correct, 0 bugs there. Did NOT trust the existing `DescribeMapRun`
and `GetExecutionHistory` "wire: ok" verdicts for this specific bug class --
both were re-derived field-by-field and both were wrong (see below).

### 4 bugs found and fixed, all proven via real `aws-sdk-go-v2/service/sfn`
### client round-trip tests (`wire_output_required_r80d_test.go`)

1. **`TaskScheduledEventDetails.Region`/`.Parameters`** (types.go:1311-1339,
   both required). `RecordTaskScheduled` (execution_history.go) only ever
   set `Resource`/`ResourceType`; `Region`/`Parameters` were never assigned
   at all, decoding as empty strings on every real client's
   `GetExecutionHistory` call for any Task-state execution -- the common
   case, not an edge case. Fixed by threading the resolved (post-Parameters-
   template) task input through as `Parameters`, and deriving `Region` via
   the same `regionFromARN(resource, backend.region)` helper already used
   for activity ARNs (`activities.go:138`), applied here to the Task
   state's resource ARN instead.
2. **`TaskSucceededEventDetails.Resource`/`.ResourceType`**
   (types.go:1431-1450, both required). `RecordTaskSucceeded` never set
   either field. Fixed by threading `state.Resource` through from
   `executeTask`'s already-in-scope `state` (asl/executor.go).
3. **`TaskFailedEventDetails.Resource`/`.ResourceType`**
   (types.go:1289-1307, both required). `RecordTaskFailed` never set either
   field on either of its two call sites (the direct-failure path in
   `executeTask` and the Catcher path in `checkCatchers`), both of which
   already have `state` in scope. Fixed the same way.
   `asl.HistoryRecorder`'s `RecordTaskSucceeded`/`RecordTaskFailed` (an
   exported interface) both gained a `resource string` parameter to carry
   this through; the one other implementation (`mockHistoryRecorder`,
   asl/executor_test.go) was updated to match. `go build ./...`,
   `go vet -tags e2e ./...`, `go vet -tags integration ./...` all re-run
   repo-wide and clean.
4. **`DescribeMapRun`'s `ExecutionCounts`** (api_op_DescribeMapRun.go:57,
   required; `types.MapRunExecutionCounts`, types.go:841-906) had no backing
   field on `MapRun` at all -- see the REVERSED note on `DescribeMapRun`
   above for why the prior pass's "correctly so" verdict was wrong, and why
   the fix is a genuinely-zero (not fabricated) `MapRunExecutionCounts`,
   always marshaled. Verified via the AWS API reference page for
   `DescribeMapRun` (fetched to check whether it documents ExecutionCounts
   mirroring ItemCounts for non-distributed Map Runs before considering
   that derivation -- it does not make that claim anywhere, so this pass
   did not fabricate that mapping and used genuine zeros instead).

### Reviewed, not a bug

- **`ValidateStateMachineDefinitionDiagnostic.Severity`** counted above as
  fixed (added to the map), but two adjacent fields on the same struct were
  checked and found fine: `Code`/`Message` are both already populated
  (`"SCHEMA_VALIDATION_FAILED"`/`err.Error()`) -- `Code` is a free-form
  `*string` on the real type (types.go:1560-1563), not an enum, so an
  invented-looking constant string is not a fabrication concern the way an
  enum value would be.
- **`EncryptionConfiguration.Type`** (types.go, required within the struct)
  is only ever populated from a caller-supplied `EncryptionConfiguration` on
  `CreateStateMachine`/`UpdateStateMachine`/`CreateActivity` -- reviewed and
  left as-is: a real `aws-sdk-go-v2` client round trip constructing this
  input always sets `Type` (it is the one field the SDK's own type gives no
  zero-value-safe default for), so the omitempty-tagged zero-value case
  needs a deliberately malformed request to reach, not a normal round trip.
  Same "reviewed, not a bug" class the cleanrooms batch-8 note names
  (required on input too, never actually reachable-empty from a real
  client).
- **`stateMachineAliasEntry.UpdateDate`** (wire key `updateDate`, required
  on `UpdateStateMachineAlias`) is tagged `omitempty` but `UpdateStateMachineAlias`
  (aliases.go) unconditionally sets `alias.UpdatedDate = time.Now().Unix()`
  before returning -- the zero-value/omitted case is not reachable from any
  code path, so left as-is rather than "fixed but not proven."
- **`StopExecutionOutput.StopDate`** (`*float64`, no `omitempty`) is only
  ever nil in principle if `StopExecution`'s no-op branch (already-terminal
  execution) runs before `StopDate` was set -- traced every place
  `exec.Status` transitions off `RUNNING`
  (`finalizeExecutionRecordLocked`/`StopExecution` itself) and confirmed
  each one sets `StopDate` in the same statement, so the no-op branch is
  never reached with a nil `StopDate`. Not reachable, not a bug.
- **`RoutingConfigurationListItem`** (`StateMachineVersionArn`/`Weight`,
  both required) matches `AliasRoutingConfig` exactly, both fields already
  non-`omitempty`. Clean.
- **`ActivityListItem`/`ExecutionListItem`/`MapRunListItem`/
  `StateMachineAliasListItem`/`StateMachineListItem`/
  `StateMachineVersionListItem`** -- all six re-verified against
  `types.go` field-by-field (see "trusted vs re-derived" above); all
  required members present, non-`omitempty`. Clean, matches this file's
  existing `dv4s`-pass notes.

### Structurally unmodeled event kinds (disclosed, not fixed)

Nine `*EventDetails` types this pass read against `types.go` have required
members (`ActivityScheduledEventDetails.Resource`,
`LambdaFunctionScheduledEventDetails.Resource`,
`EvaluationFailedEventDetails.State`, `TaskStartedEventDetails.Resource`/
`.ResourceType`, `TaskSubmittedEventDetails.Resource`/`.ResourceType`,
`TaskStartFailedEventDetails.Resource`/`.ResourceType`,
`TaskSubmitFailedEventDetails.Resource`/`.ResourceType`,
`TaskTimedOutEventDetails.Resource`/`.ResourceType`) but this backend never
emits the corresponding `HistoryEventType` values at all -- `execution_history.go`'s
`historyRecorder` only ever produces
`TaskScheduled`/`TaskSucceeded`/`TaskFailed` (generic, resource-agnostic
event kinds) regardless of resource type or `.sync`/`.waitForTaskToken`
integration pattern, never the Lambda-specific
(`LambdaFunctionScheduled`/...), Activity-specific
(`ActivityScheduled`/...), or submitted/started/timed-out variants real AWS
emits for those patterns. Since no `HistoryEvent` in this emulator ever
claims to be one of these kinds, there is no reachable case where an
incomplete instance of one of these structs is returned -- this is the
`GetExecutionHistory` scope of bd gopherstack-996 that remains open (a
missing-feature gap, not a dropped-required-field bug), not something this
pass fixed or fabricated a partial implementation of. Also confirmed via
`asl.HistoryRecorder`'s interface: it has no method for any of these event
kinds, and adding real Lambda/Activity-specific event-type differentiation
(the `arn:aws:lambda:...` vs `arn:aws:states:::lambda:invoke` resource-form
distinction real AWS uses) is out of this cut's scope -- a wire-shape
feature gap, not a required-output-field bug, tracked at gopherstack-996.

Total for this pass: 54 required output fields plus every nested
list-item/history-event-detail struct read end to end across 23 ops with
required output fields, 4 real bugs found and fixed, all proven via real
`aws-sdk-go-v2/service/sfn` client round-trip tests
(`wire_output_required_r80d_test.go`), each hand-reverted (all 5 touched
files reverted to `HEAD` together, confirmed all 4 tests fail against the
pre-fix code), confirmed failing, restored, `md5sum`-verified byte-identical.

## 2026-08-23 pass (manifest harvest): stale gap notes corrected, RedriveStatusReason fixed, Fail-state-with-no-error-code bug found and fixed

Read this file's `gaps:` block against the current code instead of trusting
its prose. Two of the seven entries were stale:

- `DescribeExecutionOutput missing RedriveStatus/RedriveStatusReason/
  MapRunArn/TraceHeader/InputDetails/OutputDetails` (bd gopherstack-f5dc):
  four of the six named fields (`RedriveStatus`, `TraceHeader`,
  `InputDetails`, `OutputDetails`) were already declared on `Execution`
  *and* already assigned real values at every relevant transition
  (`initializeExecutionRecord`/`finalizeExecutionRecordLocked`/
  `StopExecution`/`resetExecutionForRedrive`, `executions.go`) -- the note
  was simply wrong. `RedriveStatusReason` was the one real gap: declared
  (`models.go`) but never assigned anywhere, so a real client always
  decoded an empty string regardless of `redriveStatus`. Fixed by
  populating it with AWS's exact documented reason strings
  (`api_op_DescribeExecution.go`: "Execution is RUNNING and cannot be
  redriven." / "Execution is SUCCEEDED and cannot be redriven.") at every
  `NOT_REDRIVABLE` transition, and clearing it at every `REDRIVABLE`
  transition. `MapRunArn` remains genuinely unset -- confirmed via grep
  that no code path ever assigns `Execution.MapRunArn` -- but that is the
  same structural gap already tracked under bd gopherstack-8j8 (Map
  iterations aren't backed by real child executions), not a distinct bug;
  folded into that entry rather than kept as a separate false gap.
- `TaskScheduledEventDetails/TaskSucceededEventDetails still omit
  resourceType/region/parameters/...` (bd gopherstack-996, written
  2026-07-11): `resourceType`/`region`/`parameters` were fixed by the
  2026-08-21 batch-10 pass above, which never updated this older note.
  `TimeoutInSeconds`/`HeartbeatInSeconds` and the missing
  TaskSubmitted/TaskStarted events remain genuinely open.

**Real bug found via the round-trip test written to prove the
`RedriveStatusReason` fix** (`wire_redrivestatusreason_test.go`, table
covering a `Pass`-only state machine and a bare `{"Type":"Fail"}` state
machine): the bare-`Fail` case decoded as `Status: SUCCEEDED`, not
`FAILED`. Root cause: `asl.Executor.Execute` turns a `FailError` into
`&ExecutionResult{Error: failErr.ErrCode, Cause: failErr.Cause}`, and every
consumer (`executions.go`'s async and sync finalizers, `handler_util.go`'s
TestState, and `asl/executor.go`'s own Parallel-branch and Map-iteration
result handling) treated `result.Error != ""` as the failure signal. `Fail`
states' `Error`/`Cause` are both optional per the ASL spec (real AWS: a
bare `{"Type":"Fail"}` still ends the execution as `FAILED`, just with
`error`/`cause` absent from the output), so an empty `ErrCode` was
indistinguishable from success -- a genuine wrong-answer bug reachable by
any minimal Fail state, not an edge case. Fixed by adding
`ExecutionResult.Failed bool`, set `true` on the `FailError` path, and
switching every one of the five `Error != ""` call sites listed above to
check `Failed` instead (`Error`/`Cause` are still copied through
unchanged, so already-passing tests that assert specific error text were
unaffected).

Proof: `wire_redrivestatusreason_test.go`'s two subtests fail against the
pre-fix code (`succeeded`: `RedriveStatusReason` expected non-empty, got
`""`; `failed`: `Status` expected `FAILED`, got `SUCCEEDED`, and
`RedriveStatus` expected `REDRIVABLE`, got `NOT_REDRIVABLE`) and pass after
it. Hand-reverted all four touched files
(`store.go`/`executions.go`/`asl/executor.go`/`handler_util.go`) to `HEAD`
via `cp`, confirmed both subtests fail with the errors quoted above,
restored the fix via `cp`, `md5sum`-verified byte-identical to the
pre-revert state. `go build ./...`, `go test ./services/stepfunctions/...`,
`golangci-lint run ./services/stepfunctions/...` (0 issues) all clean.
No persisted struct changed (`ExecutionResult` is an in-process handoff
type, never persisted; `Execution.RedriveStatusReason` was already a
persisted field, just newly populated).

## 2026-09-07 pass (bd gopherstack-2hdk): errtargetaudit's 4 class-A findings triaged, 3 false positives, 1 real gap left as a landmine

Audit tool (`cmd/errtargetaudit`) flagged 4 class-A findings for stepfunctions
(coverage warning: only 37/205 ops resolved to a handler, so treat as
unverified leads, not a clean bill). All 4 verified against
`aws-sdk-go-v2/service/sfn@v1.45.4` deserializers.go
(`awk "/deserializeOpError<Op>\(/,/^}/" deserializers.go | grep -oE
'"[A-Za-z0-9]+"'`), not trusted as-reported.

**Protocol**: `application/x-amz-json-1.0` (`handler.go`'s `handleError`
sets this Content-Type; `models.go` confirms AWS JSON 1.0). Errors are
shaped as `service.JSONErrorResponse{Type, Message}` via a single
service-wide `classifyError` table (`handler.go`) mapping each backend
sentinel `error` to one `(__type string, HTTP status)` pair -- there is no
per-operation filtering at this layer, so any backend call site can leak
any sentinel through any operation if the call site itself is wrong. No
override-helper pattern exists in this service (grepped for
override/reclassify/remap/translate in handler_*.go -- none); class-7
(handler overrides code at call site) does not apply here.

### Findings

1. **CreateActivity / ActivityDoesNotExist** (`activities.go:119`,
   `handler_activities.go:113`) -- **false positive, class 4** (guard
   cannot fire). `handleCreateActivity` calls
   `SetActivityEncryptionConfiguration(a.ActivityArn, ...)` with the ARN
   just returned by `CreateActivity` in the same request; the guard it
   trips (`ErrActivityDoesNotExist`) requires the activity to be absent,
   which is impossible moments after `b.activities.Put(a)`. Ground truth:
   `CreateActivity` declares `ActivityAlreadyExists, ActivityLimitExceeded,
   InvalidEncryptionConfiguration, InvalidName, KmsAccessDeniedException,
   KmsThrottlingException, TooManyTags, UnknownError` -- no
   `ActivityDoesNotExist`; correctly declared instead by `DescribeActivity`/
   `GetActivityTask`. Regression coverage already exists and asserts 200:
   `TestCreateActivity_EncryptionConfiguration`
   (`handler_activities_test.go`).
2. **CreateStateMachine / StateMachineDoesNotExist**
   (`handler_state_machines.go:117`, `state_machines.go:51`,
   `state_machine_versions.go:18`) -- **false positive, class 4**, same
   shape: `createStateMachineAction` calls
   `SetStateMachineConfigurations(sm.StateMachineArn, ...)` and (when
   `publish=true`) `PublishStateMachineVersion(sm.StateMachineArn, ...)`
   with the ARN just created in the same request; both guards trip
   `ErrStateMachineDoesNotExist`, which cannot fire on an ARN that was
   `Put` moments earlier under the same lock region. Ground truth:
   `CreateStateMachine` declares `ConflictException, InvalidArn,
   InvalidDefinition, InvalidEncryptionConfiguration,
   InvalidLoggingConfiguration, InvalidName, InvalidTracingConfiguration,
   KmsAccessDeniedException, KmsThrottlingException,
   StateMachineAlreadyExists, StateMachineDeleting,
   StateMachineLimitExceeded, StateMachineTypeNotSupported, TooManyTags,
   UnknownError, ValidationException` -- no `StateMachineDoesNotExist`;
   correctly declared instead by `DescribeStateMachine`/`ListExecutions`/
   `ListStateMachineAliases`/`PublishStateMachineVersion`/`StartExecution`.
   Regression coverage already exists and asserts 200:
   `TestHandler_CreateStateMachine_PropagatesTracingAndLogging`
   (`handler_tracing_logging_test.go`) and the `publish_true_creates_version`
   subtest of `TestHandler_CreateStateMachine_Publish`
   (`handler_state_machines_test.go`).
3. **DescribeStateMachineForExecution / StateMachineDoesNotExist**
   (`executions.go:776`, now `:790`) -- **real bug, left unfixed**. Ground
   truth: this op declares only `ExecutionDoesNotExist, InvalidArn,
   KmsAccessDeniedException, KmsInvalidStateException,
   KmsThrottlingException, UnknownError` -- no `StateMachineDoesNotExist`,
   and no code in that set semantically fits "the execution exists but its
   state machine's record is gone" (`ExecutionDoesNotExist` would be
   false: the execution *does* exist). Reachable, not theoretical: the
   function's `!hasSnapshot` fallback fires for any execution started
   before the last persistence restore, because `executionDefinitions` is
   deliberately excluded from `backendSnapshot` (`persistence.go`'s
   documented Phase-3.3 boundary) -- so a restart followed by deleting that
   execution's state machine hits this branch. The sibling branch three
   lines down (`hasSnapshot == true`, SM also gone) answers the identical
   real-world condition with a synthetic 200
   (`&StateMachine{StateMachineArn: ..., Definition: definition}`), which
   is the strongest same-function precedent for what AWS actually does
   here, but silently turning this branch's error into success needs its
   own evidence per the audit brief's evidence-trap rule, and expanding
   persistence to close the root cause (persist `executionDefinitions`,
   bump `sfnSnapshotVersion`) is out of scope for this pass. Left a
   landmine comment at the call site naming both candidate fixes and this
   issue (gopherstack-2hdk). No test asserts this path either way today.
4. **StartSyncExecution / InvalidDefinition** (`executions.go:118`) --
   **false positive**, a variant of class 4 (defensive re-validation of
   already-validated data, not "just created" but "already validated at
   an earlier lifecycle step"). `StartSyncExecution` re-parses
   `sm.Definition` with `asl.Parse` after pulling it from the store; but
   `CreateStateMachine` (`state_machines.go:86-88`) and
   `UpdateStateMachine` (`state_machines.go:259-262`) both already reject
   any definition that fails `asl.Parse` before it is ever stored, and
   every path that hands a definition to `StartSyncExecution` --
   unqualified ARN, version ARN (`stateMachineForVersionLocked` copies a
   `PublishStateMachineVersion` snapshot, itself copied from an
   already-validated `sm.Definition`), or alias ARN routed to a version --
   ultimately traces back to one of those two validated writes. So the
   second parse cannot fail via any documented API sequence. Ground truth:
   `StartSyncExecution` declares `InvalidArn, InvalidExecutionInput,
   InvalidName, KmsAccessDeniedException, KmsInvalidStateException,
   KmsThrottlingException, StateMachineDeleting, StateMachineDoesNotExist,
   StateMachineTypeNotSupported, UnknownError` -- no `InvalidDefinition`;
   correctly declared instead by `CreateStateMachine`/`TestState`/
   `UpdateStateMachine`. The identical `asl.Parse(definition)` ->
   `ErrInvalidDefinition` pattern also appears, unflagged by the tool
   (coverage gap, not a clean bill), in `startExecutionLocked`
   (`executions.go:305`, backs `StartExecution`) and
   `redriveExecutionLocked` (`executions.go:688`, backs
   `RedriveExecution`) -- same reasoning applies to both; neither op's
   declared set includes `InvalidDefinition` either. Regression coverage
   already exists and asserts success:
   `TestStartSyncExecution_Express_Succeeds`/
   `TestStartSyncExecution_Express_InputPayload`
   (`executions_asl_test.go`) plus the broader `StartExecution`/
   `RedriveExecution` happy-path suites in `executions_test.go`.

### A related, unflagged structural finding: deleting vs. missing collapsed

The audit brief specifically asked whether a **deleting** state machine is
modeled as distinct from a **missing** one. It is not: there is no
`ErrStateMachineDeleting` sentinel anywhere in this package, and no
`classifyError` mapping for AWS's `StateMachineDeleting` code (which
`CreateStateMachine`/`StartExecution`/`StartSyncExecution` all declare).
`DeleteStateMachine` (`state_machines.go:134-150`) sets
`sm.Status = statusDeleting` and then, in the same locked critical
section, immediately calls `b.stateMachines.Delete(arn)` -- deletion is
synchronous, so no other request can ever observe a state machine with
`Status == DELETING`; `CreateStateMachine`'s `sm.Status != statusDeleting`
duplicate-name guard (`state_machines.go:99`) is consequently dead code.
This is a real, documented AWS distinction this backend cannot produce
(async delete simply isn't modeled) -- not one of the 4 flagged findings,
too large a change to fold into this pass, worth its own tracked issue.

### Verification

- `GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/stepfunctions/...`: pass.
- `GOTOOLCHAIN=go1.26.6 golangci-lint run services/stepfunctions/...`: `0 issues.`
- No pre-existing test was found asserting a wrong error code for any of
  the 4 call sites (searched all `*_test.go` for
  `ActivityDoesNotExist`/`StateMachineDoesNotExist`/`InvalidDefinition`
  near `CreateActivity`/`CreateStateMachine`/`StartSyncExecution`/
  `DescribeStateMachineForExecution`); 0 corrected.
- Only change: a landmine comment on finding 3
  (`executions.go`, `DescribeStateMachineForExecution`) -- no functional
  line touched. Verified with `git stash` on just that file: builds and
  `go test -run TestDescribeStateMachineForExecution` pass identically
  with and without it, confirming it's documentation-only.
- No persisted struct was touched; `pkgs/persistence` guard not re-run
  (nothing to check -- no field added).
