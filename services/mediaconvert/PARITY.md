---
service: mediaconvert
sdk_module: aws-sdk-go-v2/service/mediaconvert@v1.97.1
last_audit_commit: b451ad0d
last_audit_date: 2026-08-29
overall: A            # 2026-08-29 (wrapper-key-sweep, constraint-not-honoured class): ListQueues/
                      # ListJobTemplates/ListPresets never read ListBy (NAME/CREATION_DATE) at
                      # all -- always returned name-sorted regardless of the caller's choice;
                      # SearchJobs never read InputFile at all -- status/queue/order worked but
                      # a client scoping to one input file got every job. Both fixed; see the
                      # four ops: entries and wire_list_by_test.go/search_test.go.
                      # 2026-08-19: LastShareDetails type-confusion bug (object vs *string) found and fixed this pass -- see Notes
                      # 2026-07-24: genuine wire-breaking bugs found and fixed this pass
                      # 2026-07-31: pkgs/sdkcheck reverse check re-flagged UpdateJob, which the 2026-07-24 pass had already correctly identified as not-a-real-op (see Notes) but left ADVERTISED in GetSupportedOperations()/ChaosOperations() -- i.e. the finding was documented but not actually corrected. Now removed from the advertised list; route stays wired as internal test scaffolding, unreachable by real clients either way. See its Notes entry and handler.go's opUpdateJob comment.
ops:
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "was reading arn from URL path (always empty since real client sends POST /tags with arn in JSON body); fixed to read arn from body"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "was routed on DELETE with tagKeys from query string; real op is PUT with tagKeys in JSON body -- real SDK calls 404'd before this fix"}
  CreateJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "input field was jobEngineVersionRequested; real wire key is jobEngineVersion (response field IS jobEngineVersionRequested -- request/response names differ). This pass: statusUpdateInterval/simulateReservedQueue were parsed from the request body but silently overridden with hardcoded defaults (SECONDS_60/DISABLED) instead of the caller's value -- fixed via CreateJobFull's new JobCreateExtras parameter"}
  CreateQueue: {wire: ok, errors: ok, state: ok, persist: ok, note: "input field was reservationPlan; real wire key is reservationPlanSettings (response field IS reservationPlan). gopherstack-gt9o: maximumConcurrentFeeds (*int32, added since v1.87.3) now stored and echoed via QueueCreateExtras -- previously silently dropped, see Notes. gopherstack-7bxb: concurrentJobs was a plain int (json omitempty), collapsing 'not set' and 'explicit 0' into the same wire value; now *int matching the real *int32 member, and the SUBMITTED->PROGRESSING transition now honors it as a per-queue concurrency cap -- see Notes"}
  UpdateQueue: {wire: ok, errors: ok, state: ok, persist: ok, note: "reservationPlanSettings field name fixed; concurrentJobs and reservationPlanSettings were entirely unsupported on update (silently dropped), now applied. gopherstack-gt9o: maximumConcurrentFeeds now applied too -- previously silently dropped, see Notes. gopherstack-7bxb: concurrentJobs retyped to *int (see CreateQueue note and Notes)"}
  StartJobsQuery: {wire: ok, errors: ok, state: ok, persist: ok, note: "output field was queryId; real wire key is id"}
  GetJobsQueryResults: {wire: ok, errors: ok, state: ok, persist: ok, note: "added missing status field (JobsQueryStatus); always COMPLETE since this backend resolves queries synchronously"}
  GetJob: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-21 (gopherstack-us9u kind-mismatch sweep) -- Job.LastShareDetails was emitted as a {shareToken, sharedAt} object; the real types.Job.LastShareDetails is a bare *string (types.go), so every real SDK client's GetJob call failed outright once a job had ever been shared via CreateResourceShare. Fixed via a MarshalJSON/UnmarshalJSON pair on Job projecting LastShareDetails to its ShareToken as a plain string at the wire boundary (SharedAt has no documented place in the real string form and is dropped rather than invented into it), keeping the richer ShareDetails struct for internal/domain use. Proven via a real aws-sdk-go-v2/service/mediaconvert client round trip (wire_last_share_details_test.go), hand-reverted/confirmed-failing (expected __string to be of type string, got map[string]interface {} instead)/restored, md5sum-verified byte-identical."}
  ListJobs: {wire: ok, errors: ok, state: ok, persist: ok, note: "extra non-AWS totalCount field in response; additive, harmless to real clients"}
  CancelJob: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateJobTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: added accelerationSettings/hopDestinations/statusUpdateInterval, which the real CreateJobTemplateInput wire shape accepts but JobTemplate previously had no fields for (silently dropped) -- see CreateJobTemplateFull"}
  GetJobTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  ListJobTemplates: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack batch8 (2026-08-23): now paginates via pkgs/page.New (real NextToken), previously truncated via limitSlice with no continuation token ever returned. 2026-08-29 (wrapper-key-sweep): ListBy (NAME/CREATION_DATE, documented default NAME) was never read at all -- handler always returned name-sorted order regardless of the caller's choice. Now honored; SYSTEM is a valid enum value but this backend never creates SYSTEM-type templates (CreateJobTemplate always sets Type=CUSTOM), so there is nothing for it to filter to -- documented gap, not silently mishandled."}
  UpdateJobTemplate: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: added accelerationSettings/hopDestinations/statusUpdateInterval support via UpdateJobTemplateFull -- previously silently dropped despite the real UpdateJobTemplateInput accepting them (was the last remaining gap for this family)"}
  DeleteJobTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePreset: {wire: ok, errors: ok, state: ok, persist: ok}
  GetPreset: {wire: ok, errors: ok, state: ok, persist: ok}
  ListPresets: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack batch8 (2026-08-23): now paginates via pkgs/page.New (real NextToken), previously truncated via limitSlice with no continuation token ever returned. 2026-08-29 (wrapper-key-sweep): same ListBy gap as ListJobTemplates -- never read, now honored (NAME/CREATION_DATE); SYSTEM undocumented gap for the same reason (no SYSTEM-type presets ever created)."}
  UpdatePreset: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePreset: {wire: ok, errors: ok, state: ok, persist: ok}
  GetQueue: {wire: ok, errors: ok, state: ok, persist: ok}
  ListQueues: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack batch8 (2026-08-23): now paginates via pkgs/page.New (real NextToken), previously truncated via limitSlice with no continuation token ever returned. 2026-08-29 (wrapper-key-sweep): ListBy (NAME/CREATION_DATE, documented default NAME) was never read -- now honored."}
  DeleteQueue: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeEndpoints: {wire: ok, errors: ok, state: ok, persist: n/a, note: "this pass: real op is POST-only with maxResults/nextToken/mode in a JSON body -- gopherstack previously answered any HTTP method and ignored the body. Fixed: route now requires POST (GET/other methods 404 as unknown operation, matching real-client behavior against a real endpoint), and the body is parsed (mode/maxResults honored; nextToken accepted but there is never a next page since exactly one synthetic endpoint ever exists)"}
  GetPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeletePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateCertificate: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateCertificate: {wire: ok, errors: ok, state: ok, persist: ok}
  ListVersions: {wire: ok, errors: ok, state: ok, persist: n/a}
  Probe: {wire: ok, errors: ok, state: ok, persist: n/a}
  SearchJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "extra non-AWS totalCount not present -- SearchJobsOutput matches wire shape exactly. 2026-08-29 (wrapper-key-sweep): InputFile query param (\"provide your input file URL or your partial input file name\") was never read at all -- status/queue/order were applied but inputFile was silently ignored, so a client scoping a search to one input file got every job instead. Now matched via substring against settings.inputs[].fileInput (jobMatchesInputFile, jobs.go) -- the one field path this op documents, read from the otherwise-opaque Settings map this service already round-trips verbatim."}
  CreateResourceShare: {wire: partial, errors: ok, state: ok, persist: ok, note: "real input also requires supportCaseId; not validated/stored (harmless, output is void). 2026-08-19: this op's side effect (Job.LastShareDetails) was a critical type-confusion bug -- see gaps->fixed below"}
families:
  queue: {status: ok, note: "CreateQueue/GetQueue/ListQueues/UpdateQueue/DeleteQueue verified op-by-op against restjson1 serializers; reservationPlanSettings wire-name bug fixed on both create and update. FIXED 2026-08-23 (gopherstack batch8): ListQueues now paginates via pkgs/page.New (real NextToken) -- see Notes"}
  jobTemplate: {status: ok, note: "verified op-by-op; this pass closed the AccelerationSettings/HopDestinations/StatusUpdateInterval gap on both Create and Update (CreateJobTemplateFull/UpdateJobTemplateFull) -- family is now full field parity, no open gaps. FIXED 2026-08-23 (gopherstack batch8): ListJobTemplates now paginates via pkgs/page.New (real NextToken) -- see Notes"}
  job: {status: ok, note: "CreateJob/GetJob/ListJobs/CancelJob verified; jobEngineVersion wire-name bug fixed; this pass also fixed CreateJob silently overriding statusUpdateInterval/simulateReservedQueue with hardcoded defaults instead of applying the caller's request values; UpdateJob is a gopherstack-only extension, unadvertised as of 2026-07-31 (see notes). 2026-08-19: Job.LastShareDetails type-confusion bug fixed (see gaps->fixed and Notes)"}
  preset: {status: ok, note: "verified op-by-op, full field parity. FIXED 2026-08-23 (gopherstack batch8): ListPresets now paginates via pkgs/page.New (real NextToken) -- see Notes"}
  tags: {status: ok, note: "TagResource/UntagResource/ListTagsForResource: two critical wire bugs fixed (see gaps->fixed above); this is the class of bug parity-principles.md warns about (ARN routing) but the actual defect here was ARN-in-body vs ARN-in-URL and DELETE-vs-PUT method, not slash-escaping"}
  jobsQuery: {status: ok, note: "StartJobsQuery/GetJobsQueryResults: id/status wire-name bugs fixed"}
  endpoints/policy/certificates/misc: {status: ok, note: "DescribeEndpoints/GetPolicy/PutPolicy/DeletePolicy/AssociateCertificate/DisassociateCertificate/ListVersions/Probe/SearchJobs/CreateResourceShare verified op-by-op; this pass closed the DescribeEndpoints method/body gap (now POST-only, body parsed)"}
gaps:
  - Queue.ServiceOverrides is typed map[string]any in gopherstack vs a real []types.ServiceOverride list on the wire; currently dormant (CreateQueueInput has no serviceOverrides input member in the real API, so the field can never be populated by a real client) but the type would emit the wrong JSON shape (object instead of array) if ever populated internally. Re-verified this pass against aws-sdk-go-v2/service/mediaconvert@v1.97.1 (pin corrected from the stale v1.87.3 recorded here by gopherstack-u8my): still no serviceOverrides member on CreateQueueInput or UpdateQueueInput, so this remains genuinely unreachable/harmless -- left as-is rather than reshaping a field no real client can ever populate.
  - "FIXED by gopherstack-gt9o: CreateQueueInput/UpdateQueueInput's MaximumConcurrentFeeds *int32 member (Elemental Inference feed concurrency, added since v1.87.3) now read, stored, and echoed. See Notes."
  - "FIXED by gopherstack-7bxb: Queue.ConcurrentJobs was a plain int with json:\"concurrentJobs,omitempty\" -- a client that never sent the field and one that sent concurrentJobs:0 were indistinguishable (both stored/echoed as absent). Real CreateQueueInput/UpdateQueueInput/types.Queue.ConcurrentJobs is *int32 (api_op_CreateQueue.go:42, api_op_UpdateQueue.go:40, types/types.go:8622). Now *int, matching the MaximumConcurrentFeeds pattern above. Also: the janitor's SUBMITTED->PROGRESSING admission check (advanceSubmittedLocked, already gating on Queue.Status==PAUSED) now gates on ConcurrentJobs too -- a job stays SUBMITTED while its queue already has ConcurrentJobs jobs PROGRESSING, matching the field's own doc (\"the maximum number of jobs your queue can process concurrently\"). Not enforced: account/per-account-plus-per-queue Service Quota limits referenced in the same doc text (this backend has no account-quota-config model, matching the EFS FileSystemLimitExceeded precedent) and any minimum-value validation on ConcurrentJobs (none found in the pinned SDK's generated code, so none was invented). See Notes."
  - "FIXED 2026-08-19: Job.LastShareDetails was typed *ShareDetails{ShareToken,SharedAt} (a nested object) in gopherstack; the real wire type is *string (types.Job.LastShareDetails, aws-sdk-go-v2/service/mediaconvert@v1.97.1 types/types.go:6202; deserializers.go:19625 expects value.(string)). A real SDK client's GetJob/ListJobs/SearchJobs deserializer fails the ENTIRE call with a DeserializationError ('expected __string to be of type string, got map[string]interface {} instead') for any job that has ever been resource-shared -- not a silently-dropped field, a hard failure. Fixed by changing the field to *string (JSON-encoded share token/timestamp as the string's content, since the real field's content format is AWS-internal/undocumented) in models.go, and rebuilding it in resource_shares.go's CreateResourceShare. See Notes."
  - "Not fixed, disclosed: real Job has an ElementalInferenceConfiguration member (types.go:6157, {Features []ElementalInferenceFeature, Feeds []ElementalInferenceFeed}) that gopherstack's Job struct has no field for at all -- found incidentally while checking Job's deserializer case list for wrong keys, not by hunting missing members (Layer 3 is out of scope as a hunt per this sweep's brief). Not an input to CreateJobInput (absent from serializers.go entirely), so it is AWS-backend-computed metadata derived from analyzing the job's Settings tree -- which gopherstack treats as an opaque map[string]any passthrough (see deferred, below). Populating it correctly would require either fabricating values (bans the no-stub rule) or parsing the opaque settings tree for Elemental Inference feature/feed usage, which is out of scope here."
  - "FIXED 2026-08-23 (gopherstack batch8): ListQueues/ListJobTemplates/ListPresets used to truncate to maxResults via limitSlice with no nextToken ever returned, unlike ListJobs/SearchJobs, which already used pkgs/page.New -- see families note and Notes section for full detail. ListVersions/DescribeEndpoints remain their own separate (already-correct) pagination shapes, unaffected."
  - "Not fixed, disclosed: real ListQueuesOutput also carries totalConcurrentJobs/unallocatedConcurrentJobs (deserializers.go, ListQueues doc-output case list) that gopherstack's ListQueues response never emits. Layer 3, out of scope as a hunt."
  - "Noted, not a bug: Job/Queue/JobTemplate/Preset all carry a gopherstack-only Tags map[string]string field, serialized under \"tags\" in Get/List/Create responses. The real wire types (types.Job/types.Queue/types.JobTemplate/types.Preset) have no Tags member at all -- tags are request-only (CreateJobInput/CreateQueueInput/etc. accept them, confirmed via serializers.go's \"tags\" Key() calls) and otherwise surfaced only via ListTagsForResource. This is additive-and-unknown to the real deserializer's default case (same class as the pre-existing ListJobs.totalCount note below), so it is harmless, not a wire-shape bug -- left as-is."
deferred:
  - JobSettings/JobTemplateSettings/PresetSettings deep-structure field-level validation (gopherstack stores these as opaque map[string]any and round-trips them verbatim, which is the established pattern for this service; no validation of e.g. OutputGroups internals was audited). 2026-08-19: re-confirmed this is the correct characterization -- it is a structural boundary, not a gap: gopherstack echoes back whatever JSON the client sent for these three fields, so a wrong key inside the settings tree round-trips consistently and this backend cannot detect wire-shape defects there by construction. Established before reading any codec-level type, per this pass's brief. ElementalInferenceConfiguration (see gaps, above) is downstream of this same boundary.
leaks: {status: clean, note: "janitor.go uses pkgs/worker.Group.Ticker bound to ctx cancellation; no goroutine/map leaks found. lockmetrics.RWMutex used as the single coarse backend lock; safemap not used (not applicable, all backend collections are cross-map transactional and correctly share the coarse lock). Re-verified this pass: no new goroutines/tickers/maps introduced by the CreateJob/CreateJobTemplate/UpdateJobTemplate/DescribeEndpoints fixes; all new code paths run synchronously under the existing b.mu lock or (DescribeEndpoints) hold no lock at all since it reads no mutable backend state. 2026-08-19: CreateResourceShare's LastShareDetails fix (json.Marshal call) runs synchronously under the existing b.mu lock exactly like the rest of CreateResourceShare -- no new goroutines/tickers/maps."}
---

## Notes

- Protocol: restjson1, paths under `/2017-08-29/...`. Errors are returned as JSON
  `{"__type": "<Code>Exception", "message": "..."}` with an HTTP status matching the
  code (400/404/409/500); the real `restjson.GetErrorInfo` reads either `code` or
  `__type` from the body, so this shape round-trips correctly with the real client.
- Timestamps are already epoch-seconds floats (`epochSeconds` helper) everywhere,
  matching `pkgs/awstime.Epoch`-style behavior expected by the JSON protocol -- no
  epoch/ISO8601 bug found in this service (unlike other services previously audited).
- **Request/response field-name asymmetry is a recurring MediaConvert wire trap**:
  several fields have *different* names on the input vs. the output resource:
  - `CreateJobInput.jobEngineVersion` (request) vs. `Job.jobEngineVersionRequested`
    (response).
  - `CreateQueueInput.reservationPlanSettings` / `UpdateQueueInput.reservationPlanSettings`
    (request) vs. `Queue.reservationPlan` (response).
  Before this pass, gopherstack's input structs mistakenly used the *response* field
  name for the *request* JSON tag, so a real `aws-sdk-go-v2` client's request body
  never matched the field the handler unmarshaled into -- these fields were silently
  dropped on every real CreateJob/CreateQueue/UpdateQueue call. Fixed by giving the
  input structs the correct request-side JSON tags (`handler.go`: `createJobInput`,
  `createQueueInput`, `updateQueueInput`).
- **TagResource/UntagResource were the most severe bugs found**: the real
  `TagResource` operation is `POST /2017-08-29/tags` with the target ARN in the JSON
  body (`{"arn": ..., "tags": {...}}`), not in the URL. gopherstack was pulling the
  ARN from the URL suffix (`route.resource`), which is always empty for the real
  request shape -- every real-SDK `TagResource` call was silently tagging the
  empty-string resource key instead of the intended one. Separately, the real
  `UntagResource` operation is `PUT /2017-08-29/tags/{Arn}` with `tagKeys` in a JSON
  body; gopherstack routed it on `DELETE` with `tagKeys` as a repeated query
  parameter, so real SDK `UntagResource` calls never matched any route and always
  404'd (`Unknown operation`). Both fixed in `handler.go` (`parseTagRoute`,
  `handleTagResource`, `handleUntagResource`) -- see `TestMediaConvert_ExtractOperation`
  route-matcher-level cases and `TestMediaConvert_Tags` for the corrected wire shape.
- **StartJobsQuery/GetJobsQueryResults wire-name bug**: `StartJobsQueryOutput`'s sole
  member is `id`, not `queryId` -- gopherstack returned `{"queryId": ...}`, which a
  real SDK client's deserializer (looking for `id`) would silently leave nil, breaking
  the entire `StartJobsQuery` → `GetJobsQueryResults` polling workflow for real
  clients. Fixed the JSON tag; also added the missing `status` field on
  `GetJobsQueryResultsOutput` (always `COMPLETE` since this backend resolves queries
  synchronously inside `GetJobsQueryResults`, not asynchronously like real AWS).
- **`UpdateJob` is not a real MediaConvert operation** (confirmed by grepping the
  `aws-sdk-go-v2/service/mediaconvert` SDK: no `UpdateJobInput`/`UpdateJobOutput`/
  `Client.UpdateJob` exist, and botocore's mediaconvert service-2.json has no PUT
  route under `/jobs/{id}`). gopherstack still routes `PUT /2017-08-29/jobs/{id}` to
  an `UpdateJob` handler. This is harmless (no real SDK client can ever construct such
  a call, since the SDK exposes no method for it) and is a gopherstack-only extension,
  not AWS parity surface. **2026-07-31 correction:** this note correctly identified the
  problem back in 2026-07-24 but the "left in place" resolution was incomplete --
  `GetSupportedOperations()`/`ChaosOperations()` still *advertised* `UpdateJob` as
  supported SDK surface, which pkgs/sdkcheck's reverse check (commit 12cfe14d5;
  gopherstack-vhw2 category A) correctly re-flagged. The route itself stays wired as
  internal test scaffolding (still unreachable by real clients, still not gratuitous
  churn to delete), but it is no longer advertised — see handler.go's opUpdateJob
  comment. Same resolution as EMR's ListTagsForResource and CloudFront's
  GetFunctionAssociations/SetFunctionAssociations.
- `ListJobsOutput`/`SearchJobsOutput`: gopherstack's `ListJobs` response includes an
  extra `totalCount` field not present in the real API shape. This is additive-only
  (unknown JSON fields are ignored by `aws-sdk-go-v2`'s deserializer) so it does not
  break real clients; left as-is. `SearchJobsOutput` has no such extra field and
  matches the real shape exactly.
- Locking: single `lockmetrics.RWMutex` (`b.mu`) guards all backend maps/tables,
  consistent with the pkgs-catalog.md rule (coarse lock at the cross-map-transaction
  boundary). `safemap` is correctly not used here since every resource type
  (queues/jobTemplates/jobs/presets/tags/queueCounters/tokenIndex) participates in
  cross-map invariants (e.g. CreateJob updates jobs + queueCounters + tokenIndex
  atomically).
- Job state machine is real (not a disguised no-op): `janitor.go`'s ticker calls
  `AdvanceJobPhase`, which walks SUBMITTED → PROGRESSING(PROBING → TRANSCODING →
  UPLOADING) → COMPLETE, updating per-queue counters and populating
  `OutputGroupDetails` at completion. `CancelJob` only allows cancellation from
  SUBMITTED/PROGRESSING, matching real AWS semantics. Persistence
  (`Snapshot`/`Restore`) is wired through `Handler.Snapshot`/`Restore` delegating to
  `InMemoryBackend`, versioned (`mediaconvertSnapshotVersion`), confirmed non-dead
  (see the doc comment on `Handler.Snapshot` explaining why this delegation matters).

## 2026-07-24 pass -- closed all remaining gaps/deferred-whole-family items

- **`JobTemplate` gained `AccelerationSettings`/`HopDestinations`/`StatusUpdateInterval`**
  (`models.go`). The real `CreateJobTemplateInput`/`UpdateJobTemplateInput` wire
  shapes both accept these three fields (confirmed against
  `aws-sdk-go-v2/service/mediaconvert@v1.97.1`'s `api_op_CreateJobTemplate.go` /
  `api_op_UpdateJobTemplate.go`), but `JobTemplate` previously had no fields to
  hold them, so a real SDK client setting e.g. `AccelerationSettings` on
  `CreateJobTemplateInput` had it silently dropped -- the response would never
  reflect it. Fixed by adding the fields to `JobTemplate`, threading them through
  new `CreateJobTemplateFull`/`UpdateJobTemplateFull` backend methods (the
  existing `CreateJobTemplate`/`UpdateJobTemplate` signatures are preserved as
  thin wrappers so no caller outside this fix needed to change), and parsing them
  in `handler_job_templates.go`'s `createJobTemplateInput`/`updateJobTemplateInput`.
  `StatusUpdateInterval` defaults to `SECONDS_60` when unset, matching the
  behavior `Job` already had. `cloneJobTemplate` deep-copies the new pointer/slice
  fields so returned copies can't alias backend state (mirrors `cloneJob`'s
  existing pattern for the identical `Job` fields).
- **`CreateJob` was silently overriding `statusUpdateInterval`/`simulateReservedQueue`
  with hardcoded defaults** (`SECONDS_60`/`DISABLED`) instead of applying the
  caller's request values -- both are real, accepted `CreateJobInput` members
  (confirmed against `api_op_CreateJob.go`) that `handler_jobs.go`'s
  `createJobInput` never even parsed from the request body. Fixed by adding both
  fields to `createJobInput` and threading them into `CreateJobFull` via a new
  variadic `JobCreateExtras` trailing parameter (`jobs.go`) -- variadic so the
  ~20 pre-existing `CreateJobFull(...)` call sites across the test suite keep
  compiling unchanged (Go allows omitting a trailing variadic argument entirely).
  `CreateJobFull`'s body was split into `buildNewJobLocked` to stay under the
  `funlen` budget after the added logic.
- **`DescribeEndpoints` now POST-only with its JSON body parsed.** The real
  operation's serializer (`serializers.go`'s
  `awsRestjson1_serializeOpDescribeEndpoints`) hardcodes `request.Method = "POST"`
  and sends `{maxResults, mode, nextToken}` in the body; gopherstack previously
  matched the `/2017-08-29/endpoints` path on *any* HTTP method and never read the
  body. Fixed: the route now requires POST (other methods fall through to
  `opUnknown` → 404, matching what a real client would see hitting a real
  MediaConvert endpoint with the wrong method), and `handleDescribeEndpoints` now
  parses and honors `maxResults` (caps the returned list) and accepts `mode`/
  `nextToken` for wire accuracy. Behavior is otherwise unchanged: gopherstack
  always has exactly one synthetic endpoint (the host the request arrived on), so
  `mode=DEFAULT` vs `mode=GET_ONLY` can't observably differ here, and there is
  never a next page.
- **Re-verified, left unchanged as genuinely non-actionable**: `Queue.ServiceOverrides`
  (dormant -- no real input member exists to ever populate it) and
  `CreateResourceShare`'s missing `supportCaseId` validation (output is void, so
  this is unobservable to a real client either way). Both re-checked against the
  v1.87.3 SDK at that time; re-confirmed still accurate against the now-current
  v1.97.1 pin by the gopherstack-u8my pass (`Queue.ServiceOverrides` -- see `gaps`
  above, which also now notes a new, separate `MaximumConcurrentFeeds` gap found
  by that same pass).
- No leaks introduced: all new code (`buildNewJobLocked`, `CreateJobTemplateFull`,
  `UpdateJobTemplateFull`, `handleDescribeEndpoints`'s body parsing) runs
  synchronously with no new goroutines, tickers, or maps -- `CreateJobTemplateFull`/
  `UpdateJobTemplateFull` execute under the existing coarse `b.mu` lock exactly
  like their pre-existing counterparts, and `handleDescribeEndpoints` touches no
  backend state at all.

## 2026-08-11 pass (gopherstack-gt9o) -- MaximumConcurrentFeeds no longer dropped

- **`CreateQueueInput`/`UpdateQueueInput.MaximumConcurrentFeeds` now wired
  end to end.** Confirmed against `aws-sdk-go-v2/service/mediaconvert@v1.97.1`:
  `MaximumConcurrentFeeds *int32` on both inputs (`api_op_CreateQueue.go:47-49`,
  wire key `maximumConcurrentFeeds`, `serializers.go:635` doc-serializer,
  same key on `UpdateQueueInput`'s at `serializers.go:2940`), and on the
  `Queue` response resource (`deserializers.go:24653`'s shared
  `awsRestjson1_deserializeDocumentQueue`, field set at line 96 of that
  function's body). Threaded through as `*int` (not a plain `int`, unlike the
  pre-existing `ConcurrentJobs`) specifically so a caller-supplied `0` stays
  distinguishable from "not supplied" — `models.go`'s `Queue.MaximumConcurrentFeeds`,
  `queues.go`'s new `QueueCreateExtras{MaximumConcurrentFeeds *int}` (a
  variadic trailing parameter on `CreateQueueFull`, same pattern
  `JobCreateExtras` already established for `CreateJobFull` above, so the
  ~6 pre-existing `CreateQueueFull` call sites keep compiling unchanged), and
  a new 6th parameter on `UpdateQueue` (that method has exactly one caller,
  `handler_queues.go`, so no variadic trick was needed there). `cloneQueue`
  deep-copies the pointer (`cloneIntPtr`) so returned `Queue` values can't
  alias backend state, matching the existing `ReservationPlan`/`ServiceOverrides`
  clone pattern. No `mediaconvertSnapshotVersion` bump: `Queue` already
  round-trips through the generic `store.Table` JSON snapshot, and the new
  field is `*int` with `json:"maximumConcurrentFeeds,omitempty"` — additive
  and omitted when nil, so old snapshots restore unchanged.
  `TestMediaConvert_CreateQueue_MaximumConcurrentFeeds`/
  `TestMediaConvert_UpdateQueue_MaximumConcurrentFeeds` (`queues_test.go`)
  and `TestPersistence_NewFieldsRoundTrip` (`persistence_test.go`) cover it.
- Not attempted as a general mechanism fix, unlike the parallel mediatailor
  fix in the same issue: `createQueueInput`/`updateQueueInput` are
  hand-modeled Go structs (typed fields, not a generic pass-through map), so
  there is no equivalent of mediatailor's "exclude known-handled keys"
  inversion available here — every field this service accepts has to be
  declared on the struct one way or another. The real fix for "SDK bump adds
  a field, gopherstack silently drops it" in a hand-modeled service is the
  `pkgs/sdkcheck`-style diff sweep that found this gap in the first place
  (gopherstack-u8my), not a code-level mechanism change.

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 1 confirmed bug

`Probe`: {wire: fixed} -- each result was double-wrapped as
`{"probeResult": {"container": ..., "inputFile": ...}}`; ProbeOutput.ProbeResults
(api_op_Probe.go) is `[]types.ProbeResult` directly -- each item IS the
Container/Metadata/TrackMappings object, not wrapped under a "probeResult"
key, and there is no "inputFile" echo member at all. Proven via
`TestProbe_ResultContainsContainer` (probe_test.go, strengthened in place),
hand-reverted/confirmed-failing/restored/`md5sum`-verified byte-identical.

## 2026-08-21 (gopherstack-hjdd): snapshot-version guard, unbumped retype

`mediaconvertSnapshotVersion` bumped 1 -> 2. `d83f4b5d3` gave `Job` (the registered
`jobs` table's value type) a custom `MarshalJSON`/`UnmarshalJSON` pair that renders
`LastShareDetails` as the real deserializer's bare string instead of the previous
`{shareToken, sharedAt}` object, without bumping the snapshot version. A pre-fix (v1)
snapshot's object no longer unmarshals into the new string field at all -- `RestoreAll`
now errors outright rather than silently losing data, but the whole backend then fails
to restore, which the version guard exists to convert into a clean, recoverable
"discard and start empty" instead.

Found via `pkgs/persistence`'s snapshot-version guard, extended this session
(gopherstack-hjdd) to recursively expand fields of every type reached through a
`store.Register`/`store.New` table registration.

**Proof:** `TestInMemoryBackend_RestoreV1JobLastShareDetailsDiscarded` (persistence_test.go)
builds a v1-shaped `jobs` snapshot with an object-shaped `lastShareDetails` and asserts
`Restore` succeeds (discarding cleanly) rather than erroring. Hand-reverted to version 1:
the same test then fails with `Restore` returning `json: cannot unmarshal object into Go
struct field .lastShareDetails of type string`, confirming the symptom; restored and
`md5sum`-verified byte-identical.

**Gates:** `go build`, `go vet` (default/e2e/integration), `gofmt -l` (clean), `go test -race`
(pass), `golangci-lint run` (0 issues).
## 2026-08-19 pass -- wrapper-key/nested-shape sweep, LastShareDetails type-confusion bug fixed

- **Enumerated all 33 real ops** (`ls api_op_*.go` against the pinned
  `aws-sdk-go-v2/service/mediaconvert@v1.97.1`) and confirmed they match
  `Handler.GetSupportedOperations()` exactly, including `UpdateJob`'s
  correct absence.
- **Confirmed protocol**: restjson1, from `deserializers.go`'s
  `awsRestjson1_*` function prefix and `api_client.go`'s `ServiceID`. Ran
  the false-positive-trap check from this sweep's brief on every op: unlike
  appmesh's singular ops (dead `deserializeOpDocument<Op>Output`, flat
  body), EVERY mediaconvert op with a response body genuinely calls its
  `deserializeOpDocument<Op>Output` helper -- the wrapper key (`job`,
  `queue`, `jobTemplate`, `preset`, `policy`, `endpoints`, `resourceTags`,
  `probeResults`, `id`, ...) is live for all of them, confirmed by reading
  `GetJob`'s deserializer body directly (it decodes into `map[string]interface{}`
  then calls `awsRestjson1_deserializeOpDocumentGetJobOutput`, which switches
  on `case "job":`). gopherstack's envelope wrapper keys in `handler_jobs.go`,
  `handler_queues.go`, `handler_job_templates.go`, `handler_presets.go`,
  `handler_policy.go`, `handler_endpoints.go`, `handler_tags.go` were checked
  against this and all matched.
- **Found and fixed: `Job.LastShareDetails` type confusion (object vs
  `*string`)**. gopherstack's `models.go` had
  `LastShareDetails *ShareDetails{ShareToken, SharedAt}` -- a nested JSON
  object. The real wire member (`types.Job.LastShareDetails`,
  `aws-sdk-go-v2/service/mediaconvert@v1.97.1` `types/types.go:6202`) is a
  plain `*string`; `deserializers.go:19625` (inside
  `awsRestjson1_deserializeDocumentJob`) does
  `jtv, ok := value.(string); ... sv.LastShareDetails = ptr.String(jtv)` and
  returns a hard error if the JSON value isn't a string. This is the
  *right-key-wrong-type* bug class the campaign has been finding elsewhere,
  and it is the severe direction: it doesn't drop a field, it fails the
  **entire** `GetJob`/`ListJobs`/`SearchJobs` deserialization for any job
  that has ever been resource-shared, with `err != nil` on the real SDK
  client (`operation error MediaConvert: GetJob, ... deserialization failed,
  ... expected __string to be of type string, got map[string]interface {}
  instead`) -- proved directly against the real SDK client, see below.
  - Fix: `models.go`'s `Job.LastShareDetails` is now `*string`;
    `resource_shares.go`'s `CreateResourceShare` now JSON-encodes
    `{shareToken, sharedAt}` into that string (the real field's exact
    content format is AWS-internal/undocumented, so a JSON blob preserves
    the same information without claiming a specific real format).
  - New test: `Test_SDKRoundTrip_LastShareDetails`
    (`services/mediaconvert/wire_shape_test.go`), which stands up the real
    `aws-sdk-go-v2/service/mediaconvert` client against this package's
    `Handler` through `pkgs/service`'s registry/router (same pattern as
    `services/acm/wire_field_additions_test.go`) and calls
    `CreateJob` → `CreateResourceShare` → `GetJob`, asserting the typed
    client's `GetJob` call succeeds and `Job.LastShareDetails` round-trips
    as a non-empty string.
  - **Hand-revert proof**: reverted `Job.LastShareDetails` to
    `*ShareDetails` and `CreateResourceShare` to the old struct-literal
    assignment; `Test_SDKRoundTrip_LastShareDetails` failed with exactly
    the predicted symptom (`GetJob after CreateResourceShare must decode
    cleanly through the real SDK client: Received unexpected error:
    operation error MediaConvert: GetJob, https response error
    StatusCode: 200, ... deserialization failed, failed to decode response
    body with invalid JSON, expected __string to be of type string, got
    map[string]interface {} instead`). Also independently reproduced the
    same failure via a standalone `httptest.Server` serving a canned body
    to the real SDK client (buggy body → `ERROR: ... expected __string ...
    got map[string]interface {} instead`; fixed body → `OK`). Restored the
    fix; `models.go`/`resource_shares.go`/`resource_shares_test.go` are
    byte-identical (`diff`) to their pre-revert state; `gofmt -l` empty;
    `go build`/`go test -race` pass.
  - Also fixed the existing unit test `resource_shares_test.go`'s
    `TestCreateResourceShare_SetsShareStatus`, which had asserted the wrong
    (object) shape (`got.LastShareDetails.ShareToken`/`.SharedAt`) -- now
    asserts against the JSON-string content instead.
- **Verified clean (no bugs found)**: `Queue`/`ReservationPlan` (both
  field-for-field against `deserializeDocumentQueue`/
  `deserializeDocumentReservationPlan`), `JobTemplate`, `Preset`, `Policy`,
  `Timing`, `OutputDetail`/`OutputGroupDetail`/`VideoDetail`,
  `QueueTransition`, `JobMessages`, `HopDestination`, `WarningGroup`,
  `AccelerationSettings`, `StartJobsQuery`/`GetJobsQueryResults` (id/status
  keys, already-fixed from the 2026-07-24 pass, re-confirmed),
  `DescribeEndpoints`, `Probe`, `PutPolicy`/`GetPolicy`/`DeletePolicy`,
  `AssociateCertificate`/`DisassociateCertificate` (correctly void),
  `ListTagsForResource` (`resourceTags` wrapper, correct).
- **Summary-vs-full type-confusion pattern (7 hits elsewhere this
  campaign) structurally does not apply here**: confirmed
  `ListQueuesOutput.Queues []types.Queue`, `ListJobsOutput.Jobs []types.Job`,
  `SearchJobsOutput.Jobs []types.Job`,
  `ListJobTemplatesOutput.JobTemplates []types.JobTemplate`,
  `ListPresetsOutput.Presets []types.Preset` all reuse the exact same full
  type as their `Get*` counterpart -- MediaConvert has no separate
  "summary" struct for any of these resources, unlike fis/apprunner/ram/
  amplify/kinesis/codeconnections.
- **Settings-tree boundary re-confirmed before reading any codec type**:
  `Job.Settings`/`JobTemplate.Settings`/`Preset.Settings` are
  `map[string]any` opaque passthrough in `models.go`. This backend cannot
  detect a wrong key/shape/type/value bug anywhere inside `JobSettings`/
  `OutputGroups`/`VideoDescription`/etc. by construction -- it echoes
  back whatever JSON the client sent. Per this sweep's brief, established
  as a structural boundary before spending any effort on codec-specific
  members, and the codec-level tree (`Job.Settings`'s contents) was
  correctly NOT hand-verified this pass.
- **Found incidentally, not fixed, disclosed**: real `types.Job` has an
  `ElementalInferenceConfiguration *ElementalInferenceConfiguration`
  member (`types/types.go:6157`, `{Features, Feeds}`) that gopherstack's
  `Job` struct has no field for at all. Found while reading
  `deserializeDocumentJob`'s case list for wrong-key bugs, not by hunting
  missing members (out of scope as a hunt per this sweep's brief).
  `ElementalInferenceConfiguration` is absent from `CreateJobInput`'s
  serializer entirely, so it's AWS-backend-computed output metadata derived
  from Elemental Inference feature/feed usage inside the job's `Settings` --
  i.e. downstream of the opaque-passthrough boundary above. Populating it
  meaningfully would require either fabricating values (bans this repo's
  no-stub rule) or parsing the opaque settings tree, both out of scope
  here; left as a disclosed gap.
- **Found incidentally, not fixed, disclosed**: `ListQueues`/
  `ListJobTemplates`/`ListPresets` (`handler_queues.go:92-105`,
  `handler_job_templates.go:101-129`, `handler_presets.go:80-105`) all
  truncate their result to `maxResults` via `limitSlice` and never return
  `nextToken`, unlike `ListJobs`/`SearchJobs`/`ListVersions`/
  `DescribeEndpoints`, which use `pkgs/page.New` or otherwise wire a real
  continuation token. A real client with more queues/templates/presets
  than fit in one page can never retrieve the remainder. A missing-member
  gap (Layer 3), out of scope as a hunt per this sweep's brief, but
  flagged because it's systemic across three list ops -- worth a
  follow-up issue. Real `ListQueuesOutput` also has
  `totalConcurrentJobs`/`unallocatedConcurrentJobs` members gopherstack
  never emits; same category, noted alongside.
- **`last_audit_commit` sanity check**: the prior value, `911ff167`, is
  `git show -s --format=%ad 911ff167` → `2026-07-13`, an 18-day gap from
  the `last_audit_date: 2026-07-31` it was recorded against, and its
  commit message (`parity(pipes): fix tag limit, ...`) is for a different
  service entirely (pipes). Per this sweep's brief, an unrelated-service
  commit is schema-legal on its own, but the date gap combined with a
  commit message about a wholly different, unrelated fix is the kind of
  mismatch worth flagging (this is how a fabricated appmesh manifest was
  caught this campaign, `gopherstack-1i5l`/`gopherstack-z31a`) -- no
  evidence here of fabrication (the actual audit prose in this file is
  detailed, specific, and independently verified correct against the SDK
  during this pass), most likely just a stale/copy-paste commit reference
  left over from an earlier templating pass. Updated to current HEAD
  (`b451ad0d`, 2026-08-19) as part of this pass.
- Gates: `go build`, `go vet`, `go fix -diff` (empty), `gofmt -l` (empty),
  `go test -race` (pass), `golangci-lint run` (0 issues) -- all green
  after the fix.

## 2026-08-23 (gopherstack batch8) — ListQueues/ListJobTemplates/ListPresets pagination fixed

**Bug**: `handleListQueues`/`handleListJobTemplates`/`handleListPresets`
(`handler_queues.go`/`handler_job_templates.go`/`handler_presets.go`) all
truncated their result to `maxResults` via the local `limitSlice` helper and
never returned a `NextToken` -- unlike `ListJobs`/`SearchJobs`
(`handler_jobs.go`/`handler_search.go`), which already used `pkgs/page.New`
for real cursor-based pagination. All three real outputs carry a `NextToken`
member (confirmed against `aws-sdk-go-v2/service/mediaconvert@v1.97.1`'s
`api_op_ListQueues.go`/`api_op_ListJobTemplates.go`/`api_op_ListPresets.go`),
each documenting `MaxResults` as "up to twenty ... at one time" -- the exact
"pagination-ignored" bug class: a real client with more than one page's worth
of queues/job templates/presets could create all of them successfully but
could never retrieve the remainder past the first (unbounded) response.

**Fix**: all three now call `page.New(items, nextToken, maxResults,
defaultListPageSize)` (the same helper `ListJobs`/`SearchJobs` already used,
`pkgs/page`), with `defaultListPageSize` (renamed from the job-specific
`defaultJobsPageSize`, value unchanged at 20) shared across all four list ops
since real AWS documents the same "up to twenty" default for all of them.
`queuesListOutput`/`jobTemplatesListOutput`/`presetsListOutput` each gained a
`NextToken string \`json:"nextToken,omitempty"\`` field, matching
`jobsListOutput`'s existing shape. The now-dead `limitSlice` helper was
removed (nothing else called it).

**Proof**: `TestListOps_Pagination` (`wire_list_pagination_test.go`, 3
subtests, one per op) drives the real `aws-sdk-go-v2/service/mediaconvert`
client end to end: creates 25 resources, calls the List op with no
`MaxResults`, asserts exactly 20 come back with a non-empty `NextToken`, then
calls again with that token and asserts the remaining 5 with an empty
`NextToken`. Hand-reverted `handler_queues.go`/`handler_job_templates.go`/
`handler_presets.go`/`handler.go`/`handler_jobs.go`/`handler_search.go` (the
last two needed reverting together since they share the renamed
`defaultListPageSize` constant) to `git show HEAD:...` and confirmed all
three subtests fail exactly as predicted: every List call returns all 25
items in one response with `NextToken` never set. Restored; `md5sum`
byte-identical to the pre-revert files for all six.

Existing test suites (`handler_queues_test.go` et al.) create fewer than 20
resources per test, so none needed updating -- they were never exercising
the truncation-with-no-continuation-token bug in the first place.

Gates: `go build ./...`, `go vet ./services/mediaconvert/...`, `gofmt -l`
(clean), `go test -race -count=1 ./services/mediaconvert/...` (pass),
`golangci-lint run ./services/mediaconvert/...` (0 issues). No persisted
struct changed -- this is response-shape-only, no backend/model field
touched, no snapshot version bump needed.

## 2026-08-28 — wrapper-key-sweep: CreateQueue accepted and echoed a phantom ServiceOverrides field (acceptguard)

acceptguard flagged `createQueueInput.ServiceOverrides` (`handler_queues.go:76`, read in
`handleCreateQueue`) as matching no member of any real Input in the module. Confirmed against
mediaconvert@v1.97.1's `CreateQueueInput` (`api_op_CreateQueue.go`): `Name`/`ConcurrentJobs`/
`Description`/`MaximumConcurrentFeeds`/`PricingPlan`/`ReservationPlanSettings`/`Status`/`Tags`
only — no such member. The real `Queue` output type has no `ServiceOverrides` either
(`types/types.go`), so this was fabricated on **both** the request and response sides: a prior
version accepted it at creation and echoed it back under `"serviceOverrides"` on every `Queue`
response.

Fixed by removing `ServiceOverrides` from `createQueueInput` (`handler_queues.go`), the `Queue`
struct (`models.go`), the `CreateQueueFull` backend signature/interface
(`queues.go`/`interfaces.go`), and its clone logic (`cloneQueue`); `deepCloneMap` itself stays
(still used by job templates/jobs/presets settings maps).

A typed-client fail-before test isn't constructible — the real `CreateQueueInput`/`Queue` Go
structs never had this field, so a real client's request/response are identical before and
after. Proof is a raw-body test instead (`TestCreateQueue_RawServiceOverridesFieldIgnored`,
`wire_field_fixes_test.go`, new file): posting `{"serviceOverrides": {...}}` to `CreateQueue`
must not appear in the create response or a follow-up `GetQueue`. Hand-reverted
`handler_queues.go`/`interfaces.go`/`models.go`/`queues.go` (and the callers in
`persistence_test.go`/`queues_test.go` that passed the now-removed parameter), confirmed the
raw-body test fails (the field round-tripped on both create and get), restored. A companion
real-SDK test (`TestCreateQueue_RealSDKHasNoServiceOverrides`) proves `CreateQueue`/`GetQueue`
still work end to end through a typed client.

**Test judgement**: `queues_test.go`'s `TestCreateQueue_ServiceOverrides` and
`TestCreateQueue_ServiceOverridesDeepCopy` — a well-tested fabrication, matching this sweep's
appstream/pipes precedent — asserted the phantom field stored and deep-copied correctly. Both
removed (the feature doesn't exist). `TestInMemoryBackend_SnapshotRestore_FullState`
(`persistence_test.go`) asserted `gotQueue.ServiceOverrides` was non-nil after a restore round
trip — that assertion removed, the rest of the test (queue/job-template/job/preset snapshot
coverage) is unaffected.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` — all clean
(`./services/mediaconvert/...`).

## 2026-08-30: enumcheck struct-field-hop fix (gopherstack-3dzb), 0 confirmed bugs
`cmd/enumcheck` gained struct-field-hop resolution (see xray/codepipeline/
comprehend PARITY.md same-dated notes for the mechanics). Re-run across the
whole repo produced the same findings as before the fix -- nothing new
surfaced here or anywhere.

mediaconvert's single hit, `handler_probe.go:33`'s
`"container": {"format": "mp4"}` inside `handleProbe`, was manually
verified against `mediaconvert@v1.97.1/types/types.go:2460`: `Container.Format`
is typed `types.Format`, and `FormatMp4 Format = "mp4"`
(`types/enums.go:4060`) -- an exact match. The finding only fired because
the wire key "format" is ambiguous with the unrelated `WaveSettings.Format`
(`types.WavFormat`: RIFF/RF64/EXTENSIBLE). FALSE POSITIVE, not fixed: the
emitted value is correct for the struct actually being built here.

## 2026-09-06 pass (gopherstack-7bxb): ConcurrentJobs zero/unset ambiguity fixed, enforcement added

Filed title-only ("CreateQueueInput.ConcurrentJobs is never enforced; the
zero value is ambiguous between no-limit and zero-concurrency"), so both
claims were re-derived from the SDK and this backend's own job-lifecycle
code rather than assumed.

**Claim 2 (zero-value ambiguity): confirmed, fixed.** Real
`CreateQueueInput.ConcurrentJobs`/`UpdateQueueInput.ConcurrentJobs`/
`types.Queue.ConcurrentJobs` are all `*int32`
(`api_op_CreateQueue.go:42`, `api_op_UpdateQueue.go:40`,
`types/types.go:8622`). gopherstack's `models.go` had `ConcurrentJobs int
json:"concurrentJobs,omitempty"` -- a plain int with `omitempty` cannot
represent "client explicitly sent 0" as distinct from "client never sent
the field" (both marshal to `{}`; verified with a standalone repro
comparing the old and new struct shapes -- see this issue's test evidence).
Retyped to `*int`, mirroring the `MaximumConcurrentFeeds` fix from
gopherstack-gt9o exactly: `CreateQueueFull`'s `concurrentJobs` parameter is
now `*int` (all ~6 call sites updated), `UpdateQueue`'s pre-existing `*int`
parameter now clones into the new pointer field via `cloneIntPtr` instead
of dereferencing into a plain int, and `cloneQueue` deep-copies the pointer
so returned `Queue` values can't alias backend state (same pattern as
`ReservationPlan`/`MaximumConcurrentFeeds`). No `mediaconvertSnapshotVersion`
bump made -- see the guard note below, this is a judgment call left to the
maintainer, not made unilaterally.

**Claim 1 (never enforced): confirmed, and actionable here** (unlike a
typical "this mock doesn't model job execution" case). This backend already
has a real job lifecycle: `Job.Status` moves SUBMITTED -> PROGRESSING ->
COMPLETE/ERROR/CANCELED via `janitor.go`'s ticker
(`StartJanitor`/`AdvanceJobPhase`), and already tracks a live per-queue
PROGRESSING/SUBMITTED count (`queueJobCounter`,
`adjustQueueCounterLocked`/`getQueueCounterLocked`) that `GetQueue`/
`ListQueues` expose as `progressingJobsCount`/`submittedJobsCount`. The
SUBMITTED->PROGRESSING transition (`advanceSubmittedLocked`) already gated
admission on `Queue.Status == PAUSED` (per `Queue.Status`'s own doc: "if you
pause a queue, jobs in that queue won't begin"). `ConcurrentJobs`'s doc
text -- "the maximum number of jobs your queue can process concurrently" --
describes the identical shape of constraint, so the gate was extended
(refactored into `queueAdmitsLocked` to keep `nestif` complexity down): a
SUBMITTED job on a queue whose current PROGRESSING count has already
reached `*Queue.ConcurrentJobs` stays SUBMITTED until a slot frees (a job
completing or being canceled decrements the PROGRESSING counter, already
wired via `completeJobLocked`/`CancelJob`). `nil` ConcurrentJobs (never
set) means unlimited, matching "not set" on the real `*int32`; an explicit
`0` means zero concurrency (no job on that queue ever starts) -- the more
defensible reading now that the representation can actually say so, and
nothing in the pinned SDK documents 0 as meaning anything else.

Deliberately NOT done, and why: (1) no rejection was added to `CreateJob`.
`ServiceQuotaExceededException` is on both `CreateQueue`'s and `CreateJob`'s
deserializer error case lists (see error-extraction evidence below), but it
is generic -- nothing ties it specifically to a queue's ConcurrentJobs, and
real MediaConvert's documented behavior for `Status: PAUSED` (the existing,
proven-analogous case) is "jobs won't begin", not "CreateJob is rejected";
the same shape of block-not-reject was applied here rather than inventing a
new rejection path with no wire evidence for it. (2) Account-level and
per-account "Maximum concurrent jobs" Service Quotas, which `ConcurrentJobs`'s
own doc text references as the constraint on the value you may set, are not
modeled or enforced -- this backend has no account-quota-configuration
model to hang a real threshold off of, the same reasoning
`services/efs/PARITY.md`'s `FileSystemLimitExceeded`/`AccessPointLimitExceeded`
entry already establishes for this repo. (3) No minimum-value validation
was added for `ConcurrentJobs` (e.g. rejecting a negative value) -- the
pinned SDK ships no generated validator for this field, so there was no
verified external fact to enforce.

Error extraction (`deserializeOpErrorCreateQueue`/`deserializeOpErrorCreateJob`,
`deserializers.go`, `[A-Za-z0-9]+` to keep digit-bearing codes):
```
CreateQueue: UnknownError, BadRequestException, ConflictException, ForbiddenException,
             InternalServerErrorException, NotFoundException, ServiceQuotaExceededException,
             TooManyRequestsException
CreateJob:   UnknownError, BadRequestException, ConflictException, ForbiddenException,
             InternalServerErrorException, NotFoundException, ServiceQuotaExceededException,
             TooManyRequestsException
```

**Snapshot-version guard: FIRED, read-only, not resolved by this pass.**
`go test ./pkgs/persistence/ -run TestSnapshotVersionGuard` fails against
this change: retyping `Queue.ConcurrentJobs` from `int` to `*int` is an
existing-field type change, not the additive case the guard otherwise
special-cases (unlike `MaximumConcurrentFeeds`, which was a brand-new
field). The retype is backward-compatible in practice -- `encoding/json`
happily decodes a bare JSON number into a `*int` field, and the only
behavioral difference on an old snapshot is that a queue whose
`ConcurrentJobs` was implicitly 0 (the old wire format's `omitempty` never
wrote it) now restores as `nil` instead of `0`, which is the more correct
reading since the old format could never actually distinguish the two --
but per this task's instructions the guard was run read-only and left for
the maintainer to bump `mediaconvertSnapshotVersion` (currently 3) and
re-run with `-update`, not decided here.

Tests: `TestAdvanceJobPhase_ConcurrentJobsLimitBlocksExcessJobs` and
`TestAdvanceJobPhase_NilConcurrentJobsIsUnlimited` (`janitor_test.go`, new)
cover enforcement; `TestCreateQueue_ConcurrentJobsZeroDistinctFromUnset`
(`queues_test.go`, new) covers the zero/unset wire distinction end-to-end
through the handler. `TestCreateQueue_ConcurrentJobs` (`queues_test.go`) and
`TestPersistence_NewFieldsRoundTrip` (`persistence_test.go`) were
pre-existing tests asserting `ConcurrentJobs` as a plain value; both
corrected in place to assert through the pointer instead of weakened or
deleted.
