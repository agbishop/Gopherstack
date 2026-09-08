---
service: emrserverless
sdk_module: aws-sdk-go-v2/service/emrserverless@v1.44.4
last_audit_commit: cfa41e2b0  # gopherstack-420 (2026-09-04): state-machine/precondition sweep (not the wire-shape re-derivation this field previously tracked -- see below); prior value adb374d97 was gopherstack-tuh5's fix commit for the 2026-08-13 ListApplications/ListJobRuns/ListSessions Get-field leaks
last_audit_date: 2026-09-04
overall: A
ops:
  CreateApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "config sub-object allowlist extended to cover every types.CreateApplicationInput sub-object (added identityCenterConfiguration/diskEncryptionConfiguration/jobLevelCostAllocationConfiguration/schedulerConfiguration -- previously silently dropped); clientToken idempotency retained from prior pass"}
  GetApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: stateDetails (a real, optional types.Application response field) was entirely absent from Application/applicationToMap -- now present-if-non-empty, matching the architecture field's convention; ExtraConfig sub-objects echoed"}
  ListApplications: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tuh5: was reusing applicationToMap (the full GetApplication converter) unscoped, leaking applicationId/tags plus every populated ExtraConfig sub-object (up to 14 keys -- maximumCapacity/networkConfiguration/autoStartConfiguration/etc, see applicationConfigFieldCount) that types.ApplicationSummary does not declare. The prior entry here verified only that ApplicationSummary's required fields were present and stopped there -- a one-direction check presented as a full wire verdict. Now emits types.ApplicationSummary (architecture/arn/createdAt/id/name/releaseLabel/state/stateDetails/type/updatedAt, confirmed against awsRestjson1_deserializeDocumentApplicationSummary) via a dedicated applicationSummaryToMap; pagination via pkgs-style opaque index token, states filter ok"}
  UpdateApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "PATCH merges config sub-objects into ExtraConfig (shallow per-top-level-key replace, matching AWS partial-update semantics); now covers the same extended sub-object allowlist as CreateApplication"}
  DeleteApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "rejects delete while STARTED/STARTING/STOPPING/CREATING; cascades job runs + sessions; cleans sessionTokens + jobRunTokens for the deleted app"}
  StartApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: state-machine switch no longer references the invented ApplicationStateTerminatedWithError sentinel (see gaps history -- deleted this pass, not a real ApplicationState enum value)"}
  StopApplication: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "2026-09-04 (gopherstack-420): api_op_StopApplication.go doc -- \"All scheduled and running jobs must be completed or cancelled before stopping an application\" -- was entirely unenforced; StopApplication only checked the application's own state, never its job runs, so an application with a SUBMITTED/RUNNING job run could be stopped (and then deleted, silently discarding an active job run's terminal state). Now rejects with the same ErrInvalidState used by the app-state checks (proven by TestStopApplication_RejectsWithActiveJobRuns). Same ApplicationStateTerminatedWithError cleanup as StartApplication carried over from a prior pass."}
  StartJobRun: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "fixed real wire-shape bug (prior pass): JobRun response (GetJobRun/ListJobRuns) was emitting the request-only field name \"executionRoleArn\" instead of the actual response field \"executionRole\" (confirmed against awsRestjson1_deserializeDocumentJobRun/JobRunSummary in the SDK's deserializers.go -- a real AWS SDK client parsing gopherstack's response would get a nil ExecutionRole). Also fixed (prior pass): the required response field createdBy was entirely absent (now populated with the execution role ARN as a best-effort substitute, matching the convention already used by ListJobRunAttempts); executionIamPolicy/executionTimeoutMinutes/retryPolicy (real StartJobRunInput fields) were silently dropped -- now stored and echoed, with executionTimeoutMinutes defaulting to 720 per the documented AWS behavior when unset. 2026-09-04 (gopherstack-420): types.AutoStartConfig.Enabled doc -- \"Enables the application to automatically start on job submission. Defaults to true\" -- was stored as an inert opaque passthrough (applicationConfigFields) but never read; StartJobRun accepted job runs on a CREATED/STOPPED application without ever transitioning app.State, so GetApplication kept reporting a stale non-STARTED state while a job actively ran under it (bug pattern: stale state after mutation). Now auto-starts the application by default and rejects with the new ErrConflict (ConflictException, modeled specifically on this op) only when the caller explicitly set autoStartConfiguration.enabled=false. Proven by TestStartJobRun_AutoStartsApplication and TestStartJobRun_RejectsWhenAutoStartDisabled."}
  GetJobRun: {wire: fixed, errors: ok, state: ok, persist: ok, note: "now returns executionRole (fixed key)/createdBy/executionTimeoutMinutes/jobDriver/configurationOverrides/executionIamPolicy/retryPolicy; 2026-08-21: required releaseLabel (types.JobRun) was dropped by an omitempty-style conditional whenever the owning application's own ReleaseLabel was an explicit empty string -- reachable, since CreateApplicationInput's validator only null-checks the ReleaseLabel pointer, never its content -- now always emitted; required jobDriver also now always emitted (fixed, not counted -- see gopherstack-r80d batch 20 note below for why no real client can observe the difference). See gopherstack-r80d batch 20 note below."}
  ListJobRuns: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tuh5: was reusing jobRunToMap (the full GetJobRun converter) unscoped, leaking jobRunId/tags/executionTimeoutMinutes/jobDriver/configurationOverrides/executionIamPolicy/retryPolicy, none of which types.JobRunSummary declares. Now emits types.JobRunSummary (applicationId/arn/attempt/attemptCreatedAt/attemptUpdatedAt/createdAt/createdBy/executionRole/id/mode/name/releaseLabel/state/stateDetails/type/updatedAt, confirmed against awsRestjson1_deserializeDocumentJobRunSummary) via a dedicated jobRunSummaryToMap; states filter + pagination ok; 2026-08-21: same required-releaseLabel-dropped bug as GetJobRun, same fix -- see gopherstack-r80d batch 20 note below"}
  CancelJobRun: {wire: ok, errors: fixed, state: ok, persist: ok, note: "route is DELETE /applications/{appId}/jobruns/{jobRunId}, confirmed correct; rejects terminal states. errors: fixed as a side effect of the ErrInvalidState code fix (see error_codes)."}
  GetDashboardForJobRun: {wire: ok, errors: ok, state: ok, persist: n/a, note: "synthesized console URL, no persisted state to round-trip"}
  ListJobRunAttempts: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "synthesizes a single attempt (0) from the job run; documented limitation, not a bug -- backend does not model retries. 2026-08-20: jobRunAttemptToMap was missing the \"mode\" key entirely -- types.JobRunAttemptSummary declares Mode (confirmed against awsRestjson1_deserializeDocumentJobRunAttemptSummary), but JobRunAttemptSummary (models.go) had no Mode field and the synthesized attempt (job_run_attempts.go) never copied jr.Mode onto it, so a real SDK client's JobRunAttempts[].Mode always decoded as the zero value regardless of the job run's actual BATCH/STREAMING mode. Fixed: added JobRunAttemptSummary.Mode, wired from jr.Mode, added to jobRunAttemptToMap. Proven by TestListJobRunAttempts_Mode_SDKRoundTrip (wire_sdk_roundtrip_test.go). 2026-08-21: the synthesized attempt's required releaseLabel/stateDetails (types.JobRunAttemptSummary) were hardcoded to empty string under a comment claiming neither was tracked by the backend -- false, both are already stored on the backing JobRun -- now mirrors jr.ReleaseLabel/jr.StateDetails. See gopherstack-r80d batch 20 note below."}
  GetResourceDashboard: {wire: ok, errors: ok, state: ok, persist: n/a}
  StartSession: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "field-diffed this pass against types.StartSessionInput/Output -- clientToken/executionRoleArn/configurationOverrides/idleTimeoutMinutes/name/tags all match; response root applicationId/arn/sessionId matches StartSessionOutput exactly. 2026-09-04 (gopherstack-420): api_op_StartSession.go doc -- \"The application must be in the STARTED state or have AutoStart enabled\" -- previously rejected unconditionally with ErrInvalidState whenever the application was not already STARTED, ignoring the documented AutoStart exception (which defaults to enabled, same AutoStartConfig.Enabled field as StartJobRun). Now mirrors StartJobRun's auto-start/ErrConflict handling. Proven by TestHandler_StartSession_AutoStartsApplication (auto-start succeeds) and the updated start_requires_started_application_when_autostart_disabled case (409 ConflictException only when explicitly disabled)."}
  GetSession: {wire: ok, errors: ok, state: ok, persist: ok, note: "field-diffed against awsRestjson1_deserializeDocumentSession: applicationId/arn/createdAt/createdBy/executionRoleArn (NOT executionRole -- Session uses the opposite field name from JobRun, confirmed via deserializers.go)/releaseLabel/sessionId/state/stateDetails/updatedAt (all required) plus startedAt/endedAt/idleTimeoutMinutes/configurationOverrides/tags all present and correctly keyed; sessionToMap needed no fix"}
  ListSessions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-tuh5: was reusing sessionToMap (the full GetSession converter) unscoped, leaking startedAt/endedAt/idleTimeoutMinutes/configurationOverrides/tags, none of which types.SessionSummary declares. The prior entry here verified only that SessionSummary's required fields were present and stopped there -- a one-direction check presented as a full wire verdict. Now emits types.SessionSummary (applicationId/arn/createdAt/createdBy/executionRoleArn/name/releaseLabel/sessionId/state/stateDetails/updatedAt, confirmed against awsRestjson1_deserializeDocumentSessionSummary) via a dedicated sessionSummaryToMap; states + createdAtAfter/Before filters + pagination ok"}
  TerminateSession: {wire: ok, errors: ok, state: ok, persist: ok, note: "response shape (applicationId/sessionId) matches TerminateSessionOutput exactly"}
  GetSessionEndpoint: {wire: ok, errors: ok, state: ok, persist: n/a, note: "response shape (applicationId/sessionId/endpoint/authToken/authTokenExpiresAt) matches GetSessionEndpointOutput exactly"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  route_matcher: {status: ok, note: "verified every op's REST path + HTTP method against emrserverless@v1.40.2 serializers.go: POST /applications, GET/PATCH/DELETE /applications/{id}, POST /applications/{id}/start|stop, POST/GET /applications/{id}/jobruns, GET/DELETE /applications/{id}/jobruns/{jobRunId}, GET .../dashboard, GET .../attempts, GET/POST/DELETE /tags/{resourceArn}, session sub-routes. All match; RouteMatcher's service-name disambiguation vs AppConfig (/applications collision) unaffected by this pass."}
  error_codes: {status: fixed, note: "2026-09-04 (gopherstack-420): ErrInvalidState carried the wire code \"RequestFailedException\", which is not a real emrserverless error type at all -- types/errors.go models only ConflictException/InternalServerException/ResourceNotFoundException/ServiceQuotaExceededException/ValidationException, confirmed by checking every state-precondition op's own deserializeOpError* switch (DeleteApplication/StartApplication/StopApplication/CancelJobRun/TerminateSession/GetSessionEndpoint all list ValidationException as their only client-error type, none lists RequestFailedException). A real SDK client would deserialize this as an untyped *smithy.GenericAPIError instead of any typed exception. Fixed: ErrInvalidState now carries ValidationException. A second sentinel, ErrConflict (ConflictException), was added for the new StartJobRun/StartSession auto-start-conflict path, since ConflictException is modeled specifically on those two ops. Current mapping: ErrNotFound->404 ResourceNotFoundException, ErrAlreadyExists->409 ConflictException, ErrValidation->400 ValidationException, ErrInvalidState->400 ValidationException, ErrConflict->409 ConflictException, default->500 InternalFailure."}
  timestamps: {status: ok, note: "all createdAt/updatedAt/startedAt/endedAt/authTokenExpiresAt/jobCreatedAt use epochSeconds() (float64 Unix seconds), matching restjson1 epoch-seconds timestamp serialization -- no ISO8601 string bugs found"}
  session_family: {status: fixed, note: "fully field-diffed against types.Session/SessionSummary and every session op's Input/Output shape in the SDK module; optional resource-usage fields (billedResourceUtilization/totalResourceUtilization/totalExecutionDurationSeconds/idleSince/networkConfiguration) are intentionally omitted since this backend does not simulate real resource billing, matching the same documented omission already accepted for JobRun/Application. This pass (gopherstack-tuh5): that field-diff covered presence of required fields but not absence of extras -- ListSessions was in fact leaking 5 Get-only members (see ops); a dedicated sessionSummaryToMap now scopes it correctly"}
  list_summary_shape: {status: fixed, note: "gopherstack-tuh5: ListApplications/ListJobRuns/ListSessions each reused their Get sibling's full converter (applicationToMap/jobRunToMap/sessionToMap) unscoped. Two prior audit entries (ListApplications, ListSessions) had verified only that each Summary type's required fields were present, and recorded wire: ok on that basis -- a correct check of one direction (presence) presented as a complete wire verdict; the other direction (absence of extras) was never checked, and gopherstack is a wire emulator seen by raw HTTP/non-SDK callers, not only SDK clients that happen to discard unrecognised keys. All three now have a dedicated *SummaryToMap converter built by reading that op's own types.*Summary struct and deserializer individually rather than assumed from a sibling; regression coverage in handler_list_summary_test.go asserts on the raw JSON body, not through an SDK client, which cannot observe this class of bug. codeartifact's sibling sweep in the same pass found a second bug class (a Summary member emitted under the wrong wire key, silently dropped by real deserializers) not present in emrserverless -- checked for here and not found: applicationSummaryToMap/jobRunSummaryToMap/sessionSummaryToMap key every field under the same name its own deserializer recognises."}
gaps:
  - "Fixed: JobRunState was missing the real SDK's QUEUED constant (types/enums.go:76-84 in aws-sdk-go-v2/service/emrserverless@v1.44.4, also emr-serverless/2021-07-13/service-2.json shapes.JobRunState, both list SUBMITTED/PENDING/SCHEDULED/RUNNING/SUCCESS/FAILED/CANCELLING/CANCELLED/QUEUED). Added JobRunStateQueued for enum completeness. The lifecycle itself is unaffected: StartJobRun still only ever produces SUBMITTED (or CANCELLED via explicit cancel) -- this backend does not model application capacity/scheduler configuration, which is the only real trigger for QUEUED (see JobRun.queuedDurationMilliseconds / SchedulerConfiguration.queueTimeoutMinutes in service-2.json), so nothing ever enters PENDING/SCHEDULED/RUNNING/SUCCESS/FAILED/CANCELLING/QUEUED either -- not just QUEUED. This is a self-consistent simplification (every client-polled field agrees the run stays SUBMITTED), not an instant-success bug; simulating job execution to make QUEUED observable is out of scope without job-lifecycle simulation (tracked separately if ever undertaken)."
deferred: []
leaks: {status: clean, note: "no goroutines/janitors in this service; sessionTokens/applicationTokens/jobRunTokens are plain in-memory maps cleaned up on DeleteApplication and full Reset(), and persisted/restored alongside the store.Table-backed resources -- no unbounded growth path found. Re-verified this pass: no new goroutines/tickers were introduced by the field additions."}
---

## Notes

Protocol: **restjson1**. All timestamps are epoch-seconds floats via `epochSeconds()`
(this package's local helper, semantically equivalent to `pkgs/awstime.Epoch` -- not
reused from that package because this handler builds ad-hoc `map[string]any` wire bodies
rather than typed structs; worth revisiting if `pkgs/awstime` gains a
`map[string]any`-friendly helper).

### Re-audit, 2026-08-20 (wrapper-key / nested-shape sweep)

Full independent re-derivation from the pinned SDK (v1.44.4, unchanged from the prior
audit's pin -- no staleness there): every op's HTTP method/path re-read from
`serializers.go`'s `SplitURI` calls, every response wrapper key and per-op live
deserializer path re-read from `deserializers.go` (confirmed restjson1's wrapped
`deserializeOpDocument<Op>Output` path is genuinely live -- called, not dead code -- for
all 17 ops with a body; the other 5 are correctly void), and the full field/case list of
`Application`/`ApplicationSummary`, `JobRun`/`JobRunSummary`,
`JobRunAttemptSummary`, `Session`/`SessionSummary` re-read from their own
`awsRestjson1_deserializeDocument*` switch statements rather than trusting this
manifest's prior claims.

Result: one real bug found (`ListJobRunAttempts`'s missing `mode`, see `ops` above) and
one manifest-provenance defect fixed (`last_audit_commit` was a leftover placeholder
pointing at an unrelated `resourcegroupstaggingapi` commit dated a month before
`last_audit_date`; corrected to the actual `adb374d97`). Everything else the prior audit
recorded held up under independent re-verification -- no additional wrong-key,
wrong-nesting, wrong-type, or wrong-enum-value bugs found. The three request/response
sibling-type pairs this sweep specifically targets
(`ImageConfiguration`/`ImageConfigurationInput`,
`IdentityCenterConfiguration`/`IdentityCenterConfigurationInput`, and the same pattern on
every other `applicationConfigFields` sub-object) cannot leak the request-only-field bug
class at all: this backend stores and echoes every sub-object as an opaque
`map[string]any` passthrough keyed by its AWS wire field name rather than reconstructing
a typed Go struct, so it only ever echoes back whatever subset of real field names the
caller itself sent -- there is no code path that could fabricate a field the caller never
sent.

### Real bugs found and fixed (2026-08-13 pass, gopherstack-tuh5 unless noted)

1. **JobRun response used the wrong wire field name for the execution role**
   (`handler.go`'s `jobRunToMap`). `GetJobRun`/`ListJobRuns` were emitting
   `"executionRoleArn": jr.ExecutionRoleArn`, but the real API's *response* field for
   both `types.JobRun` and `types.JobRunSummary` is `"executionRole"` --
   `"executionRoleArn"` is only the field name on the `StartJobRunInput` *request* body
   (confirmed by reading both `awsRestjson1_deserializeDocumentJobRun` and
   `awsRestjson1_deserializeDocumentJobRunSummary` in the SDK module's
   `deserializers.go`, and cross-checking that `Session`'s response, by contrast,
   genuinely does use `executionRoleArn` -- these two shapes use opposite field names on
   the same concept, a real AWS API inconsistency, not a gopherstack bug to "fix" on the
   Session side). A real AWS SDK client parsing gopherstack's previous `GetJobRun`
   response would silently get a `nil` `ExecutionRole` field. Fixed by changing the map
   key; the internal Go field name (`JobRun.ExecutionRoleArn`) was left unchanged since
   it is also legitimately used for the *request*-side parsing in `StartJobRun`.

2. **JobRun response was missing the required `createdBy` field entirely**
   (`models.go`, `handler.go`). `types.JobRun.CreatedBy` and
   `types.JobRunSummary.CreatedBy` are both marked required response members, but
   `JobRun` had no `CreatedBy` field at all and `jobRunToMap` never emitted the key. This
   backend does not model IAM principals, so (matching the pre-existing convention
   already used by `ListJobRunAttempts`' synthesized attempt) `StartJobRun` now sets
   `CreatedBy` to the execution role ARN as a best-effort substitute.

3. **`Application` had no `stateDetails` field** (`models.go`, `handler.go`).
   `types.Application.StateDetails` is a real (optional) response field that was
   entirely unmodeled -- `applicationToMap` never had a `stateDetails` key at all, so an
   application that legitimately reaches a state with details attached (e.g. a failure
   message) could never surface it. Added `Application.StateDetails` and wired it into
   `applicationToMap` as present-if-non-empty, matching the existing `architecture`
   field's convention.

4. **`executionIamPolicy`/`executionTimeoutMinutes`/`retryPolicy` silently dropped by
   `StartJobRun`** (`models.go`, `job_runs.go`, `handler.go`). All three are real
   `StartJobRunInput` fields (`types.JobRunExecutionIamPolicy`, `*int64`,
   `types.RetryPolicy`) that were accepted by nothing and never echoed back. Fixed with
   the same opaque-passthrough pattern already used for `jobDriver`/
   `configurationOverrides`: `JobRun.ExecutionIamPolicy`/`JobRun.RetryPolicy` (stored
   verbatim) and `JobRun.ExecutionTimeoutMinutes` (defaulted to 720 when unset, matching
   the real API's documented default: "If no timeout was specified, then it returns the
   default timeout of 720 minutes.").

5. **`CreateApplication`/`UpdateApplication` config-sub-object allowlist was missing
   four real fields** (`handler.go`'s `applicationConfigFields`):
   `identityCenterConfiguration`, `diskEncryptionConfiguration`,
   `jobLevelCostAllocationConfiguration`, `schedulerConfiguration`. All four are real
   `types.CreateApplicationInput`/`types.UpdateApplicationInput`/`types.Application`
   sub-objects that were silently dropped (the previous pass's `gaps` entry flagged
   these as known-missing; this pass closes that gap by extending the same generic
   opaque-passthrough mechanism already used for the other ten sub-objects). With this
   change, `applicationConfigFields` now covers every sub-object field on the real
   `CreateApplicationInput`/`UpdateApplicationInput` shapes -- no remaining allowlist
   gap.

6. **Deleted invented `ApplicationStateTerminatedWithError` ("TERMINATED_WITH_ERRORS")**
   (`models.go`, `applications.go`, `applications_test.go`). This was not a real
   `ApplicationState` enum value (`types/enums.go` only defines
   CREATING/CREATED/STARTING/STARTED/STOPPING/STOPPED/TERMINATED) -- it was dead code
   referenced only by `StartApplication`/`StopApplication`'s terminal-state `switch`
   statements and one test case, and no code path ever set an application to this state.
   A prior audit pass flagged but deliberately left this in place as low-priority
   cleanup; this pass deletes the constant, the two dead `switch` cases that referenced
   it, and the test case that exercised it, per the project's no-invented-enum-values
   rule.

### Verified correct (no bug, but worth recording so the next audit doesn't re-flag)

- **Session family, fully field-diffed this pass** (previously only spot-checked and
  listed under `deferred`): every op's request/response shape
  (`StartSession`/`GetSession`/`ListSessions`/`TerminateSession`/`GetSessionEndpoint`/
  `GetResourceDashboard`) was compared field-by-field against
  `types.Session`/`types.SessionSummary` and each op's generated `api_op_*.go`
  Input/Output struct. No bugs found. Notably, `Session`'s response field really is
  `executionRoleArn` (unlike `JobRun`'s `executionRole` -- see bug #1 above), so
  `sessionToMap` needed no change.
- **Route matcher**: every op's REST path + HTTP method (`serializers.go` in the SDK
  module) matches `parseEMRPath` exactly, including the tricky ones: `UpdateApplication`
  is `PATCH /applications/{id}` (not POST), `StartApplication`/`StopApplication` are
  `POST /applications/{id}/start|stop`, and `CancelJobRun` is
  `DELETE /applications/{id}/jobruns/{jobRunId}` (not a POST-based cancel action).
  `RouteMatcher()`'s extra `Authorization`-header service-name check (disambiguating
  from AppConfig, which also serves `/applications`) is untouched and still correct.
- **CreateApplication application-name uniqueness check**: gopherstack rejects a second
  `CreateApplication` with a name already in use (`ConflictException`). This is **not**
  documented AWS behavior (the real API does not enforce unique application names; only
  `clientToken` gives idempotency) but is left as pre-existing behavior -- `clientToken`
  replay (added in a prior pass) means retried requests no longer hit this check, which
  was the main practical failure mode.
- **Pagination**: `emrPaginate` (index-based opaque `nextToken`, `maxResults` 1-50
  bounds-checked with `ValidationException` on violation) matches AWS's paginated-list
  contract for `ListApplications`/`ListJobRuns`/`ListJobRunAttempts`/`ListSessions`.
- **Timestamps**: every response field that AWS serializes as REST-JSON `timestamp`
  (epoch-seconds number, not ISO8601 string) uses `epochSeconds()` consistently --
  `createdAt`, `updatedAt`, `startedAt`, `endedAt`, `jobCreatedAt`,
  `authTokenExpiresAt`. No ISO8601-string-where-epoch-expected bugs found.
- **Error code mapping**: `handleError` maps all four sentinel errors
  (`ErrNotFound`/`ErrAlreadyExists`/`ErrValidation`/`ErrInvalidState`) to the correct
  HTTP status + AWS error code; no missing gap found (not-found paths all correctly
  return `ResourceNotFoundException`/404, not falling through to the 500
  `InternalFailure` default).
- **`CreateApplicationInput.Name` is optional on the real API** (not marked
  `required` in `api_op_CreateApplication.go`), but gopherstack's `CreateApplication`
  rejects an empty name with `ValidationException`. This is a stricter-than-AWS
  defensive check, not an invented field/error (it doesn't reject anything a
  spec-compliant client would ever legitimately send in practice), so it was left as-is
  rather than loosened -- flagged here only for visibility, not as a gap.

### 2026-08-21: three required-output members dropped or hardcoded empty (gopherstack-r80d batch 20)

Verified as the largest remaining candidate after sagemaker (off-limits, an
unrelated conversion still uncommitted) via a fresh `cmd/requiredoutputfields`
run cross-checked against `services/_REQUIRED_OUTPUT_CANDIDATES.md` (both
agreed: emrserverless 25/22/14). The flat per-op scan (25 required fields
across 14 ops-with-required, 22 ops total) undercounts the real surface: an
AST-style walk of `types/types.go` (cross-checked three ways -- a
character-level brace matcher, a `go/parser` AST pass, and a raw
`grep -c "This member is required."`, all agreeing at 40 structs / 15 with
required members / 76 total required fields) finds `GetApplication`,
`UpdateApplication`, `GetJobRun`, `GetSession` each return a domain object
counted as **one** required field at the op level despite that object
itself carrying 7-11 required members, and `ListApplications`/`ListJobRuns`/
`ListJobRunAttempts`/`ListSessions` each return a list of such objects,
invisible to the per-op scan entirely -- the same "one wrapper key wraps a
whole domain object" class named by pinpoint/bedrockagent, compounded by
the "list of domain-struct elements" class named by omics/cleanrooms. The
gap (76 vs 25) is fully explained: 66 of the 76 domain-struct fields belong
to `Application`/`ApplicationSummary`/`JobRun`/`JobRunSummary`/
`JobRunAttemptSummary`/`Session`/`SessionSummary` (all reachable through
exactly the op-level wrapper fields above); the remaining 10
(`CloudWatchLoggingConfiguration`/`Configuration`/`Hive`/
`ImageConfiguration`/`InitialCapacityConfig`/`MaximumAllowedResources`/
`SparkSubmit`/`WorkerResourceConfig`) are all part of the
`applicationConfigFields`/`JobDriver`/`ConfigurationOverrides` opaque
echo-verbatim sub-objects this backend deliberately does not parse -- since
gopherstack never constructs these types itself (it stores and replays
whatever JSON the client sent), it cannot independently drop a required
member of one; if a client sends valid content, it survives untouched, and
if a client sends invalid/incomplete content that's a client bug the
backend has no way to detect or fabricate around. All 7 non-exempt domain
structs were read end to end against their handler's map-construction
function (`applicationToMap`/`applicationSummaryToMap`/`jobRunToMap`/
`jobRunSummaryToMap`/`sessionToMap`/`sessionSummaryToMap`/
`jobRunAttemptToMap` in `handler.go`/`session_handler.go`, plus
`job_run_attempts.go`'s attempt-construction), not grepped.

2 bugs counted, both `JobRun`/`JobRunSummary`/`JobRunAttemptSummary`'s
required `releaseLabel` dropped or fabricated-empty in a state a real
client can reach:

1. **`jobRunToMap`/`jobRunSummaryToMap` guarded `releaseLabel` behind
   `if jr.ReleaseLabel != ""`.** `JobRun.ReleaseLabel` is copied from the
   owning `Application.ReleaseLabel` at `StartJobRun` time
   (`job_runs.go`). `Application.ReleaseLabel` is itself copied verbatim
   from `CreateApplicationInput.ReleaseLabel`, whose real SDK
   `validateOpCreateApplicationInput` (validators.go) only checks
   `v.ReleaseLabel == nil` -- it never inspects the string's content, so a
   real client sending an explicit empty-string `ReleaseLabel` pointer
   passes client-side validation, reaches gopherstack (whose own
   `CreateApplication` only rejects an empty `name`/`type`, not an empty
   `releaseLabel`), and every job run started under that application then
   drops the required `releaseLabel` key entirely on `GetJobRun`/
   `ListJobRuns`. This is exactly the reachability wrinkle batch 19 named
   for cognitoidp: a validator's presence rules out nil, not content. Fixed
   by making both map builders always emit `releaseLabel` unconditionally
   (matching the convention `applicationToMap`/`sessionToMap` already used
   correctly for the same field). Proven via a real
   `aws-sdk-go-v2/service/emrserverless` client round trip
   (`TestGetJobRun_ReleaseLabelSurvivesEmptyApplicationReleaseLabel`,
   `wire_output_required_r80d_test.go`): `CreateApplication` with
   `ReleaseLabel: aws.String("")`, then `StartJobRun`/`GetJobRun`/
   `ListJobRuns`, asserting the typed `ReleaseLabel` field is non-nil
   (empty string, not omitted) on both the full and summary shapes.
   Hand-reverted (`handler.go` restored to `git show HEAD:...`), confirmed
   both assertions fail (`Expected value not to be nil`), restored,
   md5sum byte-identical.

2. **`ListJobRunAttempts`'s synthesized attempt hardcoded `releaseLabel`
   and `stateDetails` to `""`** (`job_run_attempts.go`), under a comment
   claiming neither field was "tracked by the backend, using sensible
   placeholders." Both claims were false: `JobRun.ReleaseLabel` and
   `JobRun.StateDetails` are both already stored on the backing `JobRun`
   this exact function reads six other fields from. Unlike bug #1, the
   wire key here was never dropped (the map literal always includes it) --
   this is a data-fidelity bug, not a dropped-key one, but it means real
   client-observable data was silently discarded for no reason on a
   required member. Fixed by reading `jr.ReleaseLabel`/`jr.StateDetails`
   directly, matching every other field in the same struct literal. Proven
   via a real SDK client round trip
   (`TestListJobRunAttempts_ReleaseLabelAndStateDetailsMirrorJobRun`):
   create an application with a real release label, start a job run,
   cancel it (which sets a real `StateDetails` message), then
   `ListJobRunAttempts` and assert the attempt's `ReleaseLabel`/
   `StateDetails` match the job run's, not empty. Hand-reverted,
   confirmed failing (`Not equal: expected "emr-6.6.0", actual ""`),
   restored, md5sum byte-identical.

**Fixed but NOT counted**: `jobRunToMap` also guarded `jobDriver` behind
`if jr.JobDriver != nil`. `JobDriver` is required on `types.JobRun` but
genuinely optional on `StartJobRunInput` (`validateOpStartJobRunInput` only
validates `JobDriver`'s content when non-nil, never requires it), so a real
client omitting it is a reachable state that dropped the required key --
the same class as bug #1. It was fixed (the key is now always present), but
unlike bug #1 this cannot be proven via any real client: reading
`awsRestjson1_deserializeDocumentJobDriver` shows its per-key `switch` over
the `"jobDriver"` object's own keys assigns nothing when that object is
empty, and the outer `awsRestjson1_deserializeDocumentJobRun`'s switch
skips the `"jobDriver"` case entirely when the key is absent from the
response body -- both paths leave the typed `JobDriver` field `nil` with no
observable difference. `TestGetJobRun_JobDriverKeyAlwaysPresent` documents
this (asserting the identical outcome under both configurations) rather
than asserting a provable regression, matching cognitoidp batch 19's
`AccountTakeoverActionType.Notify` precedent for a real-but-unprovable fix.

**Ruled out, not bugs**: `Application`/`ApplicationSummary`'s own
`releaseLabel` (unconditional in both `applicationToMap` and
`applicationSummaryToMap` already); `Session`/`SessionSummary`'s
`releaseLabel`/`stateDetails`/every other required member (`sessionToMap`/
`sessionSummaryToMap` build every required key unconditionally already,
confirmed by reading both functions in full); `JobRun`/`JobRunSummary`'s
own `stateDetails` (already unconditional in `jobRunToMap`/
`jobRunSummaryToMap`, unlike the neighboring `releaseLabel` bug -- read
carefully to confirm this wasn't the same bug twice); `GetSessionEndpoint`'s
5 required members (`applicationId`/`authToken`/`authTokenExpiresAt`/
`endpoint`/`sessionId`), all unconditional in `handleGetSessionEndpoint`;
`CancelJobRun`'s 2 required members, unconditional in
`handleCancelJobRun`; the 10 "echoed verbatim opaque sub-object" domain
structs named above, structurally exempt since gopherstack never
constructs their content itself. `JobRunAttemptSummary.Type` (not required
per the real SDK, confirmed against `types/types.go` -- gopherstack never
populates it, a data-completeness gap outside this cut's scope, not
flagged as a bug).

services/_REQUIRED_OUTPUT_CANDIDATES.md updated: emrserverless moved from
the ranked table into "Already examined" (settled-services count now 37,
2349 required output fields read end to end). networkmonitor (22,
ops=12/ops-with-required=7) is now the largest remaining candidate after
sagemaker (still off-limits this batch -- `git status` showed uncommitted
sagemaker changes both before and after this batch, from a concurrent
agent's in-flight conversion).

### 2026-08-29: independent re-sweep, GENUINELY CLEAN (gopherstack-6flj/21my)

No code changes since `last_audit_commit`; `git log adb374d97..HEAD --
services/emrserverless/` shows only the already-recorded 2026-08-20
wrapper-key sweep, the r80d batch-20 required-output cut, and an unrelated
IAM-enforcement test addition. Re-derived every struct's member list fresh
from its own `awsRestjson1_deserializeDocument*` case list in
`deserializers.go` rather than trusting the prior manifest's counts, and
checked write-only state both directions:

- **N of N member coverage, independently re-counted**: Application 25/25,
  ApplicationSummary 10/10, JobRun 30/30, JobRunSummary 16/16,
  JobRunAttemptSummary 15/15, Session 21/21, SessionSummary 11/11 -- every
  member each deserializer recognises is either emitted by the
  corresponding `*ToMap` builder or is a documented, disclosed omission
  (resource-utilization/timing fields this backend does not simulate:
  `attemptCreatedAt`/`attemptUpdatedAt`/`billedResourceUtilization`/
  `endedAt`/`imageConfiguration`/`networkConfiguration`/
  `queuedDurationMilliseconds`/`startedAt`/`totalExecutionDurationSeconds`/
  `totalResourceUtilization`/`workerTypeSpecifications` on `JobRun`;
  `billedResourceUtilization`/`idleSince`/`networkConfiguration`/
  `totalExecutionDurationSeconds`/`totalResourceUtilization` on `Session`
  -- all optional per the SDK, none required, matching the pattern already
  disclosed for the session family).
- **FORWARD (accept-and-drop)**: re-read every request body struct
  (`createApplicationBody`/`updateApplicationBody`/`startJobRunBody`/
  `startSessionBody`/`tagResourceBody`) against its real
  `*Input` struct in `api_op_*.go` -- every accepted field is either stored
  (directly or via the `applicationConfigFields` opaque-passthrough
  allowlist, still 14/14) or is request-plumbing with no backend field to
  drop (e.g. `clientToken`, consumed for idempotency). No new accept-and-
  never-store field found.
- **REVERSE (computable-but-unemitted)**: no stored field found without a
  reader; `Application.ExtraConfig`, `JobRun.JobDriver`/
  `ConfigurationOverrides`/`ExecutionIamPolicy`/`RetryPolicy`,
  `Session.ConfigurationOverrides` are all read back by their op's map
  builder.
- **Route matcher / HTTP bindings**: re-walked `parseEMRPath` against every
  op's `SplitURI`/method pair; unchanged and correct (verified 2026-08-20,
  re-confirmed here).
- **Enums**: `JobRunState`/`ApplicationState`/`SessionState` re-checked
  against `types/enums.go`; no invented or missing values found beyond what
  is already disclosed (`QUEUED` reachability gap, above).
- Tools: `enumcheck` run repo-wide, zero findings for `services/emrserverless/`.
  `go build`, `go vet ./...` (repo-wide), `go test -race -count=1
  ./services/emrserverless/...`, `golangci-lint run
  ./services/emrserverless/...` all clean, 0 issues.

Verdict: no bugs found this pass. This is the second independent
confirmation (after 2026-08-20's from-scratch re-derivation) that this
service's wire shape is correct in both directions.

### 2026-09-04: state-machine and precondition sweep, three bugs (gopherstack-420)

Prior passes (above) exhaustively verified wire *shape* -- field presence,
key names, nesting -- but had not independently re-derived the *behavioral*
preconditions and state-machine documented in each op's own doc comment.
This pass read every mutating op's `api_op_*.go` doc comment and per-op
error-code switch in `deserializers.go` fresh, cross-checked against
`types/errors.go`, and found three real bugs:

1. **`ErrInvalidState` carried a fabricated error code, `RequestFailedException`,
   which does not exist anywhere in this service's error model**
   (`errors.go`, `handler.go`). `types/errors.go` defines exactly five
   exception types for emrserverless: `ConflictException`/
   `InternalServerException`/`ResourceNotFoundException`/
   `ServiceQuotaExceededException`/`ValidationException`. Checked every op
   that returns `ErrInvalidState` (`DeleteApplication`/`StartApplication`/
   `StopApplication`/`CancelJobRun`/`TerminateSession`/`GetSessionEndpoint`)
   against its own `awsRestjson1_deserializeOpError<Op>` switch in
   `deserializers.go`: all six list `ValidationException` as their only
   client-error type, none lists `RequestFailedException` (that type
   belongs to unrelated services -- confirmed present in `acmpca`'s and
   `codepipeline`'s `types/errors.go`, absent from emrserverless's). A real
   AWS SDK client hitting any of these six state-violation paths would get
   an untyped `*smithy.GenericAPIError` instead of a recognised exception
   type, breaking any caller doing `errors.As(err, &types.ValidationException{})`.
   Fixed: `ErrInvalidState` now carries `ValidationException`. Proven via
   `TestHandler_ErrInvalidStateMapping`, which the prior version of this
   test itself encoded the bug into (asserted `"RequestFailedException"`
   as the *expected* value) -- fixed alongside the code. Hand-reverted
   (`handler.go`'s literal restored to `"RequestFailedException"`),
   confirmed the test fails (`expected: "ValidationException", actual:
   "RequestFailedException"`), restored, md5sum byte-identical.

2. **`StopApplication` never enforced its documented job-run precondition**
   (`applications.go`). `api_op_StopApplication.go`: "Stops a specified
   application and releases initial capacity if configured. All scheduled
   and running jobs must be completed or cancelled before stopping an
   application." The handler only checked the application's own state
   (already STOPPED/TERMINATED), never its job runs -- an application with
   a SUBMITTED or RUNNING job run could be stopped (and, since
   `DeleteApplication` cascade-deletes job runs unconditionally once an
   app is STOPPED/CREATED, subsequently deleted), silently discarding an
   active job run instead of rejecting the request as AWS documents. Fixed
   by rejecting with `ErrInvalidState` when any job run under the
   application (`jobRunsByApplication` index) is not in a terminal state
   (`isTerminalJobRunState`, the same helper `CancelJobRun` already used).
   Proven by `TestStopApplication_RejectsWithActiveJobRuns`. Hand-reverted
   (removed the job-run loop), confirmed the test fails ("An error is
   expected but got nil"), restored, md5sum byte-identical. Blast radius:
   `TestHandler_DeleteApplication_CleansUpJobRunsAndSessions` relied on
   stopping an application that still had a live (never-cancelled) job
   run -- updated to cancel the job run before stopping, matching the now-
   enforced precondition.

3. **`autoStartConfiguration` was a fully inert opaque passthrough --
   `StartJobRun`/`StartSession` never read it, and `StartJobRun` never
   auto-started a non-STARTED application, leaving `Application.State`
   stale after a job run began** (`applications.go`, `job_runs.go`,
   `session.go`). `types.AutoStartConfig.Enabled`'s doc: "Enables the
   application to automatically start on job submission. Defaults to
   true." `StartJobRun` accepted job runs against a CREATED or STOPPED
   application unconditionally (no state check at all) but never mutated
   `app.State`, so `GetApplication` kept reporting the application's old
   state indefinitely while a job actively ran under it -- the "stale
   cache / partial sync after mutation" pattern. Separately,
   `api_op_StartSession.go`'s doc ("The application must be in the
   STARTED state or have AutoStart enabled") was only half-implemented:
   `StartSession` unconditionally rejected a non-STARTED application with
   `ErrInvalidState`, never honoring the documented AutoStart exception.
   Fixed both: added `applicationAutoStartEnabled` (reads
   `ExtraConfig["autoStartConfiguration"]["enabled"]`, defaulting to `true`
   per the doc when absent or malformed), and both `StartJobRun` and
   `StartSession` now auto-start a non-STARTED application when enabled
   (the common case) and reject with a new sentinel, `ErrConflict`
   (`ConflictException`, 409) -- modeled specifically on both these ops,
   per each op's own error switch, unlike `ErrInvalidState`'s ops above --
   only when the caller explicitly set `enabled: false`. Proven by
   `TestStartJobRun_AutoStartsApplication`,
   `TestStartJobRun_RejectsWhenAutoStartDisabled`, and
   `TestHandler_StartSession_AutoStartsApplication`; the existing
   `start_requires_started_application` handler test case encoded the old
   unconditional-rejection behavior as correct and was rewritten to
   `start_requires_started_application_when_autostart_disabled` (creates
   the application with `autoStartConfiguration.enabled: false` and
   expects 409, not 400). Each new test hand-reverted and confirmed
   failing without its fix (state check comparisons and "error expected,
   got nil"), then restored, md5sums byte-identical.

**Deliberately not changed, and why:**
- `StartJobRun`'s modelled `ConflictException` was initially considered as
  evidence that submitting a job run to a non-STARTED application should
  unconditionally fail -- but `CreateApplication` and `StartSession` also
  model `ConflictException` despite having no comparable state constraint
  in prose, and both (like `StartJobRun`) accept a `ClientToken`; the more
  parsimonious explanation covering all three is AWS's standard
  idempotency-token-reuse-conflict semantics, not a state constraint. Only
  `StartSession`'s prose directly documents a state precondition, so only
  it (and, by the shared `AutoStartConfig` field, `StartJobRun`) got a
  state-conflict path -- `CreateApplication` was left untouched.
- `types.AutoStopConfig` ("configuration for an application to
  automatically stop after a certain amount of time being idle") is fully
  round-tripped (accepted on Create/Update, persisted in
  `Application.ExtraConfig`, echoed by Get/List -- proven by
  `TestHandler_CreateApplication_ConfigPassthrough` and
  `TestHandler_UpdateApplication_ConfigMerge`) but **enforcement** (actually
  stopping an idle application) remains unimplemented. Implementing
  enforcement would require simulating wall-clock idle time via a
  background ticker, which this service has none of (`leaks: {status:
  clean, note: "no goroutines/janitors in this service"}` above) --
  re-verified 2026-09-07 (gopherstack-3vyq): no `janitor.go`, no
  `service.BackgroundWorker`/`Shutdowner` implementation in `provider.go`,
  unlike e.g. amplify's `Janitor`/`WithJanitor`. The pattern is well
  established elsewhere in this repo (38 other services have a
  `janitor.go`) and wiring one in is not architecturally blocked, but
  building it is real, non-trivial work (needs an idle-since clock per
  application/job-run-count check, a ticker wired through
  `provider.Init`, and a `testing/synctest`-based test in the style of
  `services/sagemaker/lifecycle_test.go`) and is deferred, not attempted
  this pass; flagged here as a known, disclosed gap rather than
  guess-implemented. 2026-09-07: fixed a smaller, real defect found while
  re-checking this gap -- `idleTimeoutMinutes` has a documented AWS range
  ("Valid Range: Minimum value of 1. Maximum value of 10080", API
  reference; not stated in the Go SDK's doc comment, which gives only the
  default of 15) that gopherstack did not enforce at all, accepting e.g.
  `idleTimeoutMinutes: 0` or `10081` with a 200 instead of the documented
  `ValidationException`. Fixed via `validateAutoStopConfig`
  (`applications.go`), called from both `handleCreateApplication` and
  `handleUpdateApplication`; proven by
  `TestHandler_ErrValidationMapping`'s two new
  `autoStopConfiguration_idleTimeoutMinutes_*` cases and
  `TestHandler_UpdateApplication_AutoStopConfigValidation` (rejects 0 and
  10081, accepts the 1 and 10080 boundaries).
- `ApplicationState`'s `CREATING`/`STARTING`/`STOPPING` and `TERMINATED`
  remain unreachable (nothing in this backend ever sets them) -- this is
  the same class of simplification already disclosed for `JobRunState`'s
  `QUEUED`/`PENDING`/etc. (see `gaps`, above): every reachable state
  transition (`CreateApplication`->CREATED, `StartApplication`/auto-start
  ->STARTED, `StopApplication`->STOPPED) is instantaneous and
  self-consistent, so no client-observable staleness results. Not fixed;
  simulating asynchronous provisioning is out of scope for this pass.
- Verified and left as-is: `DeleteApplication`'s own precondition ("has to
  be in a stopped or created state in order to be deleted") is correctly
  enforced for every state this backend can actually reach (`CREATED`/
  `STARTED`/`STOPPED` -- `STARTING`/`STOPPING`/`CREATING`/`TERMINATED` are
  all unreachable per above, so the existing switch's handling of them,
  while technically over-broad against the doc's exact wording, is inert).

Tools: `GOTOOLCHAIN=go1.26.6 go build ./...`, `go vet
./services/emrserverless/...`, `go test -race -count=1
./services/emrserverless/...`, `go test -race -count=1
./services/cloudformation/... ./services/emr/...` (dependents), and
`golangci-lint run ./services/emrserverless/...` all clean, 0 issues.

### 2026-09-07: gopherstack-3vyq -- AutoStopConfig audit, idleTimeoutMinutes bounds fixed

Issue gopherstack-3vyq was filed title-only ("AutoStopConfig idle timeout is
accepted but never enforced; needs a background ticker this service
deliberately lacks") with no description. Re-derived the specifics:

- **"Deliberately" is documented, not an assumption.** The prior
  2026-09-04 pass already recorded this exact decision in the
  "Deliberately not changed" section above (`leaks.note`: "no
  goroutines/janitors in this service"), so the issue title accurately
  reflects a real, pre-existing, disclosed design decision -- it did not
  need to be re-litigated as a documentation gap.
- **AutoStopConfig round-trips correctly**, contrary to what "accepted but
  never enforced" alone might suggest as a defect class: it is not
  silently dropped. `CreateApplication`/`UpdateApplication` store it in
  `Application.ExtraConfig` via the same generic opaque-passthrough
  mechanism as the service's other 13 config sub-objects, and
  `GetApplication`/`ListApplications` echo it back --
  `TestHandler_CreateApplication_ConfigPassthrough` and
  `TestHandler_UpdateApplication_ConfigMerge` already proved this before
  this pass.
- **Real, fixable defect found and fixed**: `idleTimeoutMinutes` has a
  documented range on AWS's API reference page
  (docs.aws.amazon.com/emr-serverless/latest/APIReference/API_AutoStopConfig.html):
  "Valid Range: Minimum value of 1. Maximum value of 10080." (not stated
  in the pinned Go SDK's doc comment, `types/types.go:186-188`, which
  gives only the default of 15 minutes) -- gopherstack enforced no bound
  at all, accepting e.g. `0` or `10081` with `200 OK`. Fixed with
  `validateAutoStopConfig` (`applications.go`), wired into both
  `handleCreateApplication` and `handleUpdateApplication` in
  `handler.go`, returning the documented `ValidationException`/400 (same
  sentinel and status as every other input-validation failure in this
  service). Regression tests written first and confirmed failing against
  unmodified code (`expected: 400 / actual: 200`, `expected:
  "ValidationException" / actual: ""`) in
  `TestHandler_ErrValidationMapping`'s two new
  `autoStopConfiguration_idleTimeoutMinutes_*` cases, then in
  `TestHandler_UpdateApplication_AutoStopConfigValidation` (also proves
  the 1 and 10080 boundaries are still accepted, not off-by-one rejected).
- **Enforcement (a ticker that actually stops idle applications) is a
  real, deferred gap, not a fixable-now bug.** `provider.go` implements
  neither `service.BackgroundWorker` nor `service.Shutdowner`
  (`pkgs/service/service.go:111-124`), and there is no `janitor.go` in
  this directory, confirming the issue's premise. The pattern itself is
  not exotic -- 38 other services in this repo have a `janitor.go`, and
  amplify's `Janitor` (`services/amplify/janitor.go`) is a close
  structural match: a `pkgs/worker.NewGroup`-based ticker advancing
  resources out of a transient state, wired via `handler.WithJanitor` in
  its `provider.go`. Building the equivalent here (idle-since tracking
  per application, keyed off last job-run/session activity, ticked and
  transitioning STARTED->STOPPED, with a `testing/synctest`-based test
  along the lines of `services/sagemaker/lifecycle_test.go`) is feasible
  but is real, multi-file, non-mechanical work -- out of scope for this
  pass per the investigating issue's explicit scope note. Left as a
  disclosed gap (see the "Deliberately not changed" bullet above, updated
  this pass).

Tools re-run after the fix: `GOTOOLCHAIN=go1.27.0 go test -race
./services/emrserverless/...` and `GOTOOLCHAIN=go1.27.0 golangci-lint run
./services/emrserverless/...`, both clean, 0 issues.
