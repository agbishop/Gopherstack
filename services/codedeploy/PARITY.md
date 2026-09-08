---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: codedeploy
sdk_module: aws-sdk-go-v2/service/codedeploy@v1.43.0   # version audited against; corrected from stale v1.37.0 pin
last_audit_commit: 59ab8f6a                             # HEAD when this manifest was written
last_audit_date: 2026-08-10
overall: A            # A = genuine fixes found; B = already-accurate, proven op-by-op
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  GetApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "createTime was UnixMilli int64, fixed to awstime.Epoch float64"}
  ListApplications: {wire: ok, errors: ok, state: ok, persist: ok, note: "TRIAGED (gopherstack-a250): input was a literal struct{}; real ListApplicationsInput (api_op_ListApplications.go) has an optional NextToken member, discarded. NOT wired: this service never truncates any List* response (verified across all 8 List ops in this file, not just the struct{} ones -- ListDeployments/ListOnPremisesInstances/ListDeploymentGroups/etc. have no pagination either), so NextToken has no continuation state to represent and no caller-supplied token could ever produce a response different from the unconditional full list already returned. Inert given this backend's list model, not a fabrication candidate -- see gaps."}
  DeleteApplication: {wire: ok, errors: partial, state: ok, persist: ok, note: "TRIAGED (gopherstack-3pz8): DeleteApplication's own deserializer models no ApplicationDoesNotExistException at all (only ApplicationNameRequiredException/InvalidApplicationNameException/InvalidRoleException) -- the not-found path is provably wrong, but whether the real op is a silent idempotent success or maps to a different code is unconfirmed (doc page shows only generic '200 empty body' boilerplate, not a semantic idempotent-delete sentence). Landmine comment left at the call site; unfixed pending evidence"}
  UpdateApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  BatchGetApplications: {wire: ok, errors: n/a, state: ok, persist: ok, note: "same createTime fix as GetApplication"}
  CreateDeploymentGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-455l): Ec2TagFilters/Ec2TagSet and OnPremisesInstanceTagFilters/OnPremisesTagSet were both accepted unconditionally though both ops model 'only one of these data types can be used in a single call' (InvalidEC2TagCombinationException/InvalidOnPremisesTagCombinationException, types/errors.go:1579-1580,2122-2123); now rejected when both halves of a pair are set. Also added Ec2TagFilters[].Type validation against the real KEY_ONLY|VALUE_ONLY|KEY_AND_VALUE enum (InvalidEC2TagException, types/errors.go:1606) -- empty Type stays legal since validators.go does not require EC2TagFilter.Type. OnPremisesInstanceTagFilters[].Type is NOT validated: no matching 'invalid format' error is modeled on this op (InvalidTagFilterException belongs to ListOnPremisesInstances only)"}
  GetDeploymentGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (6flj wrapper-key sweep): real DeploymentGroupInfo has 23 keys (deserializers.go's awsAwsjson11_deserializeDocumentDeploymentGroupInfo); gopherstack had 20, missing lastAttemptedDeployment/lastSuccessfulDeployment/targetRevision. Added InMemoryBackend.LastDeploymentsForGroup deriving both from real per-group deployment history (Deployment.CreateTime/Status/Revision, already tracked); targetRevision taken from the most-recently-ATTEMPTED deployment's own revision (the plain reading of 'target' -- the SDK's own doc comment does not distinguish attempted-vs-successful, so this specific choice is an interpretation, disclosed here, not independently confirmed against a live account)"}
  ListDeploymentGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDeploymentGroup: {wire: ok, errors: partial, state: ok, persist: ok, note: "TRIAGED (gopherstack-3pz8): DeleteDeploymentGroup's own deserializer models neither ApplicationDoesNotExistException nor DeploymentGroupDoesNotExistException (only *NameRequiredException/Invalid*NameException/InvalidRoleException) -- both current not-found paths are provably wrong, remedy (silent success vs. a different code) unconfirmed. Landmine comments left at both call sites; unfixed pending evidence"}
  UpdateDeploymentGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-455l): same Ec2TagFilters/Ec2TagSet and OnPremisesInstanceTagFilters/OnPremisesTagSet mutual-exclusion + Ec2TagFilters[].Type validation as CreateDeploymentGroup -- see that entry"}
  BatchGetDeploymentGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "same lastAttemptedDeployment/lastSuccessfulDeployment/targetRevision fix as GetDeploymentGroup (shared deploymentGroupOutputWithHistory converter)"}
  CreateDeployment: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: fileExistsBehavior was accepted and stored unvalidated (any garbage string round-tripped); now validated against the real DISALLOW|OVERWRITE|RETAIN enum, InvalidFileExistsBehaviorException (confirmed in CreateDeployment's own error set, deserializers.go) for anything else"}
  GetDeployment: {wire: ok, errors: ok, state: ok, persist: ok, note: "createTime/completeTime were UnixMilli int64, fixed to awstime.Epoch float64"}
  ListDeployments: {wire: ok, errors: n/a, state: ok, persist: ok, note: "createTimeRange.start/end request fields were parsed as epoch-millis (time.UnixMilli), fixed to epoch-seconds float64 matching smithytime.FormatEpochSeconds. FIXED (gopherstack-3qel): externalId was modeled on ListDeploymentsInput but never parsed, so the filter was silently ignored. Nothing in this backend can populate a deployment's ExternalID -- CreateDeploymentInput has no such field (api_op_CreateDeployment.go); the real value only ever comes from AWS-side CodePipeline/CloudFormation integrations, per DeploymentInfo.ExternalId's own doc comment (types/types.go:420) -- so now parses/filters on it, correctly returning zero results for any non-empty value instead of ignoring it"}
  StopDeployment: {wire: ok, errors: ok, state: ok, persist: ok, note: "output status was returning the deployment's own status literal (Stopped), fixed to the real StopStatus enum (Succeeded); deployment status itself still correctly becomes Stopped. FIXED (6flj wrapper-key sweep): real StopDeploymentOutput also has statusMessage (deserializers.go's awsAwsjson11_deserializeOpDocumentStopDeploymentOutput case 'statusMessage'), never modeled at all; added, text sourced verbatim from the SDK's own doc comment for the Succeeded StopStatus value"}
  ContinueDeployment: {wire: ok, errors: ok, state: ok, note: "FIXED this pass: READY_WAIT/TERMINATION_WAIT are not DeploymentStatus values at all (they're ContinueDeploymentInput.DeploymentWaitType, an input enum) -- the prior 'blue/green wait-state' framing conflated the two. The real gap was narrower: ContinueDeployment accepted a deployment in ANY status and deploymentWaitType was read off the wire and never validated or used. Added the real precondition (status must be Ready, else DeploymentIsNotInReadyStateException/DeploymentAlreadyCompletedException per types/errors.go:221,556-557) and deploymentWaitType enum validation (InvalidDeploymentWaitTypeException). Since CreateDeployment completes synchronously and no op ever sets status=Ready, ContinueDeployment now always errors in practice -- which is the honest behavior for a backend with no genuine blue/green wait state, not a regression"}
  SkipWaitTimeForInstanceTermination: {wire: ok, errors: ok, state: ok, persist: n/a, note: "was missing the deploymentId existence check every sibling deployment-scoped op has; fixed"}
  BatchGetDeployments: {wire: ok, errors: n/a, state: ok, persist: ok, note: "same createTime/completeTime fix as GetDeployment"}
  BatchGetDeploymentInstances: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass: resolves real on-premises instances matched against the deployment group's tag filters (matchesOnPremisesTargeting), status derived from the deployment's own Status via targetStatusForDeployment instead of hardcoded Succeeded; unmatched/never-registered IDs silently omitted matching the codebase's Batch* convention"}
  BatchGetDeploymentTargets: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass: instanceTarget/ecsTarget/lambdaTarget union resolved from real deployment group config (on-prem tag matching / ECSServices list / single Lambda target); deploymentTargetType + nested member wire shape verified against types.DeploymentTarget deserializer"}
  GetDeploymentInstance: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass: real lookup against computed targets, InstanceDoesNotExistException for a real-but-non-participating or unknown instance instead of fabricating a match for any ID"}
  GetDeploymentTarget: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass: real lookup against computed targets, new DeploymentTargetDoesNotExistException (404) sentinel for an unknown target ID instead of fabricating a match"}
  ListDeploymentInstances: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass: returns the real matched instanceTarget IDs for Server/Lambda platforms; empty for ECS (no per-instance concept) -- previously always empty regardless of real targets"}
  ListDeploymentTargets: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass: returns the real computed target IDs (sorted) -- previously always empty regardless of real targets"}
  PutLifecycleEventHookExecutionStatus: {wire: ok, errors: ok, state: ok, persist: n/a, note: "was missing the deploymentId existence check every sibling deployment-scoped op has; fixed"}
  CreateDeploymentConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  GetDeploymentConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "createTime was UnixMilli int64, fixed to awstime.Epoch float64"}
  ListDeploymentConfigs: {wire: ok, errors: n/a, state: ok, persist: ok, note: "TRIAGED (gopherstack-a250): same NextToken-inert finding as ListApplications -- see that row."}
  DeleteDeploymentConfig: {wire: ok, errors: partial, state: ok, persist: ok, note: "TRIAGED (gopherstack-3pz8): DeleteDeploymentConfig's own deserializer models no DeploymentConfigDoesNotExistException (only DeploymentConfigInUseException/DeploymentConfigNameRequiredException/InvalidDeploymentConfigNameException/InvalidOperationException) -- the not-found path is provably wrong. InvalidOperationException (already used a few lines below for the built-in-config case) is a plausible candidate but its description is generic, not evidence this case reuses it; silent success is equally plausible. Landmine comment left at the call site; unfixed pending evidence"}
  RegisterApplicationRevision: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: persists to a real applicationRevisions store.Table keyed by (appName, canonical revision JSON); re-registering an already-known revision refreshes description, preserves original registerTime"}
  GetApplicationRevision: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: reads the persisted revision, populates revisionInfo (GenericRevisionInfo: description/registerTime/firstUsedTime/lastUsedTime/deploymentGroups, field names+epoch-seconds verified against deserializers.go), new RevisionDoesNotExistException (404) for an unregistered revision instead of echoing the request back"}
  ListApplicationRevisions: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: returns real registered revisions for the application with deployed/s3Bucket/s3KeyPrefix/sortBy/sortOrder filtering; previously always empty since nothing was ever persisted"}
  BatchGetApplicationRevisions: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: genericRevisionInfo populated for revisions that are actually registered, omitted for ones that are not, instead of echoing the input unconditionally with no real lookup"}
  DeleteGitHubAccountToken: {wire: ok, errors: ok, state: ok, persist: ok}
  ListGitHubAccountTokenNames: {wire: ok, errors: n/a, state: ok, persist: ok, note: "TRIAGED (gopherstack-a250): same NextToken-inert finding as ListApplications -- see that row."}
  RegisterOnPremisesInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  DeregisterOnPremisesInstance: {wire: ok, errors: partial, state: ok, persist: ok, note: "ErrOnPremisesInstanceNotFound had the wrong error code (InstanceNameRequiredException) and no errorMappings entry at all, so it fell through to 500 ServiceException; fixed to InstanceDoesNotExistException + 404. TRIAGED (gopherstack-3pz8): that fix itself is still wrong -- DeregisterOnPremisesInstance's own deserializer models neither InstanceDoesNotExistException nor any not-found code at all (only InstanceNameRequiredException/InvalidInstanceNameException). Remedy (silent success vs. a different code) unconfirmed; landmine comment left at the call site, unfixed pending evidence"}
  GetOnPremisesInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "same registerTime/deregisterTime epoch fix + errorMappings fix as DeregisterOnPremisesInstance. FIXED (6flj wrapper-key sweep): real InstanceInfo has 7 keys (deserializers.go's awsAwsjson11_deserializeDocumentInstanceInfo); gopherstack had 6, missing instanceArn. Added InMemoryBackend.OnPremisesInstanceARN, reusing the same 'instance:<name>' resource format already used for the identical resource type's InstanceTarget.TargetArn (deployment_instances.go). FIXED (gopherstack-3pz8): not-found path used ErrOnPremisesInstanceNotFound (InstanceDoesNotExistException), but GetOnPremisesInstance's own deserializer models InstanceNotRegisteredException instead (confirmed against both deserializers.go and the API_GetOnPremisesInstance.html doc page's Errors section); new ErrOnPremisesInstanceNotRegistered sentinel wired in"}
  ListOnPremisesInstances: {wire: ok, errors: n/a, state: ok, persist: ok}
  BatchGetOnPremisesInstances: {wire: ok, errors: n/a, state: ok, persist: ok, note: "same registerTime/deregisterTime epoch fix; same instanceArn fix as GetOnPremisesInstance"}
  AddTagsToOnPremisesInstances: {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveTagsFromOnPremisesInstances: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (6flj wrapper-key sweep, request-side, not independently observable): real TagResourceInput uses PascalCase (ResourceArn/Tags) -- the shared generic tagging shape, unlike this service's own camelCase convention -- fixed for wire-shape correctness though pkgs/service's encoding/json.Unmarshal already bound the old lowercase-tagged fields via its case-insensitive fallback, so this was never a live request-side bug"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "same PascalCase (ResourceArn/TagKeys) request-side fix as TagResource, same non-observability caveat"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "FLAGSHIP FIX (6flj wrapper-key sweep): response was wire-tagged json:\"tags\" (lowercase); real deserializer (deserializers.go:20417, awsAwsjson11_deserializeOpDocumentListTagsForResourceOutput) switches on the case-sensitive PascalCase key \"Tags\" -- this protocol has zero body-field EqualFold calls, so a lowercase key was silently dropped by EVERY real client's ListTagsForResource call regardless of what had actually been tagged. This is the one op family in the service using AWS's shared generic tagging shape (PascalCase) instead of CodeDeploy's own camelCase convention. Fixed response (live bug) and request (ResourceArn, not independently observable -- see TagResource note) sides. Two pre-existing tests (tags_test.go) had decoded the response with a local json:\"tags\" (lowercase) struct -- because both sides used plain encoding/json with its case-insensitive fallback, those tests would have passed identically whether or not the bug was fixed, so they provided zero signal on this bug either way; updated for accuracy, but real verification is wire_field_fixes_test.go's real-SDK-client test, whose response decode goes through the actual case-sensitive generated deserializer"}
  DeleteResourcesByExternalId: {wire: ok, errors: n/a, state: ok, persist: n/a, note: "no resource-by-external-id tracking exists anywhere in this backend (or any other gopherstack service); an idempotent no-op matches real AWS's own best-effort cleanup semantics"}
families:
  Application: {status: ok, note: "verified wire shapes (applicationId/applicationName/computePlatform/createTime), error codes, and persistence against aws-sdk-go-v2/service/codedeploy@v1.37.0 deserializers.go"}
  DeploymentGroup: {status: ok, note: "full field-by-field mapping (blue/green, alarms, triggers, ECS, tag sets, load balancer info) verified against dgToOutput/dgInputFromWire"}
  Deployment: {status: ok, note: "lifecycle is synchronous: CreateDeployment immediately sets status=Succeeded with completeTime = now+5s, which is a deliberate simplification (documented in Notes) rather than a bug"}
  DeploymentConfig: {status: ok, note: "9 built-in AWS default configs correctly seeded and protected from deletion (DeploymentConfigInUseException)"}
  Tags: {status: ok, note: "ARN-based dispatch to application/deploymentgroup tag stores verified; on-premises instance tagging is a separate, also-correct path"}
  OnPremisesInstance: {status: ok, note: "registerTime/deregisterTime epoch fix + error-code fix (earlier pass); this pass decomposed matchesTagFilters (banned gocognit nolint) and added matchesTagSetGroups/matchesOnPremisesTargeting, reused by the new deployment-target computation"}
  ApplicationRevision: {status: ok, note: "FIXED this pass: real applicationRevisions store.Table (composite key appName+canonical-revision-JSON, byApplication index), wired into backendSnapshot as a 'clean' table (no live tags.Tags field). RegisterApplicationRevision persists; CreateDeployment auto-registers an unseen revision and stamps FirstUsedTime/LastUsedTime/DeploymentGroups (touchApplicationRevisionForDeployment); DeleteApplication cascades deletes (deleteApplicationRevisions), UpdateApplication rename moves revisions to the new app name (renameApplicationRevisions) -- no ghost rows in either case"}
  DeploymentTarget: {status: ok, note: "FIXED this pass: GetDeploymentTarget/ListDeploymentTargets/BatchGetDeploymentTargets/GetDeploymentInstance/ListDeploymentInstances/BatchGetDeploymentInstances all resolve from deploymentTargets(), a real (not fabricated) computation over the deployment's owning deployment group: matched on-premises instances (Server), one target per configured ECS service (ECS), or the single Lambda target (Lambda) real AWS always has exactly one of for that platform. Target Status is mapped from the deployment's own current Status via targetStatusForDeployment instead of a hardcoded literal. FIXED THIS PASS (previously the known limitation): Ec2TagFilters/Ec2TagSet now resolve against real services/ec2 instances via cross_service.go's lazy SetAppConfig wiring (same pattern as services/mgn), matching non-terminated EC2 instances whose TagsForResource tags satisfy the deployment group's targeting config (matchesEc2Targeting, mirroring matchesOnPremisesTargeting's Ec2TagSet-precedes-Ec2TagFilters rule). Falls back to zero EC2-side targets when the EC2 backend isn't wired (e.g. unit tests constructing InMemoryBackend directly) -- documented, not fabricated. CONFIRMED ACCURATE (6flj wrapper-key sweep): the real DeploymentTarget union's 5th member, cloudFormationTarget, is deliberately never modeled -- this backend has no CloudFormation blue/green stack-set integration anywhere, so it can never be populated honestly; the code's own doc comment already stated this, promoted into this manifest for visibility rather than left code-only."}
  cross-service: {status: fixed, note: "codedeploy now resolves the services/ec2 backend on demand via cross_service.go's siblingServices interface (GetEC2Handler), matched structurally against *CLI -- the same lazy SetAppConfig pattern services/mgn, services/grafana, and services/resiliencehub already use. *CLI already exposed GetEC2Handler() (cli.go:1134) for those services, so this needed zero cli.go changes: only provider.go gained one line (backend.SetAppConfig(ctx.Config)) and a new services/codedeploy/cross_service.go file."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "gopherstack-a250: ListApplications/ListDeploymentConfigs/ListGitHubAccountTokenNames had literal struct{} inputs discarding a real (optional) NextToken member each. Not wired: no List* op in this service ever truncates its response (confirmed across all 8), so there is no continuation state for NextToken to represent -- see the three ops' notes above. If this service ever adds real MaxResults-driven truncation to any List op, these three should be revisited together, not in isolation. RE-CONFIRMED (6flj wrapper-key sweep): the same inertness holds, unchanged, for the other 5 List ops this pass touched (ListApplicationRevisions/ListDeploymentGroups/ListDeploymentInstances/ListDeploymentTargets/ListOnPremisesInstances) plus ListTagsForResource -- an accurate, still-current prior note, not argued-away."
  - "gopherstack-6flj: ApplicationInfo (GetApplication/BatchGetApplications) never emits gitHubAccountName/linkedToGitHub -- both real (deserializers.go's awsAwsjson11_deserializeDocumentApplicationInfo). Not fixed: CreateApplicationInput/UpdateApplicationInput have no member to ever set either (this is legacy console-driven GitHub OAuth linking with no public request parameter), so this backend can never produce anything but the Go zero value for either. Since omitempty suppresses a zero-value field identically whether or not the struct field exists, adding it would be a pure source change with zero wire-byte effect -- disclosed rather than added as dead code."
  - "gopherstack-6flj: InstanceSummary/InstanceTarget/ECSTarget/LambdaTarget (GetDeploymentInstance/GetDeploymentTarget/BatchGet* siblings) never emit lifecycleEvents (real on all four types); ECSTarget also never emits taskSetsInfo, LambdaTarget also never emits lambdaFunctionInfo. Not fixed: PutLifecycleEventHookExecutionStatus is a pure echo (validates the deployment exists, stores nothing), so this backend has zero real per-target lifecycle-hook-execution state ever, for any target type; same story for ECS task-set orchestration and Lambda alias-shift data -- neither is modeled anywhere. Same zero-wire-effect reasoning as the ApplicationInfo gap above -- disclosed, not added as dead code."
  - "gopherstack-6flj: RevisionLocation never models the deprecated legacy 'string'/RawString revision member (deserializers.go's awsAwsjson11_deserializeDocumentRevisionLocation case \"string\", RevisionLocationType=String, Lambda-deployment-only raw YAML/JSON revisions). S3Location/GitHubLocation/AppSpecContent cover every revision path this backend's CreateDeployment/RegisterApplicationRevision can construct; the SDK's own doc comment marks this member's underlying concept as legacy. No honest non-empty value to emit -- disclosed, not fixed."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "StopDeployment accepts a deployment in any status (including already-terminal Succeeded/Failed/Stopped) and unconditionally overwrites it to Stopped. Real AWS's StopDeployment error set includes DeploymentAlreadyCompletedException (confirmed in StopDeployment's own deserializer switch, deserializers.go:5294) for exactly this case. NOT fixed this pass: unlike ContinueDeployment (which had no legitimate success path in this backend either way, since blue/green Ready state is never reached), StopDeployment's current behavior is relied on by existing tests and the general 'stop a just-created deployment' UX this mock supports; enforcing the precondition would make StopDeployment permanently non-functional given CreateDeployment's synchronous-completion design, which is the same underlying lifecycle gap noted below, not a narrow validation fix. (bd: unfiled)"
  - "CreateDeployment completing synchronously (status=Succeeded immediately, no Created/Queued/InProgress/Baking/Ready window) is a deliberate simplification, not a bug (see Notes) -- but it does mean no deployment in this backend can ever reach the real blue/green Ready wait-state, so ContinueDeployment's now-correct precondition check (see ops table) will always reject it. Modeling a genuine in-progress/waiting lifecycle so blue/green deployments can actually reach Ready is a larger rearchitecture, deliberately out of scope for this pass."
leaks: {status: clean, note: "no goroutines/janitors in this service; Reset/Snapshot/Restore all close tags.Tags handles correctly on the three dirty tables (applications, deploymentGroups, onPremisesInstances). The new applicationRevisions table carries no live handles (no tags.Tags field), so it needs no Close() calls -- registered as a 'clean' store.Table on b.registry like deployments/deploymentConfigs, reset via registry.ResetAll()."}
---

## Notes

- **Protocol**: awsjson1.1, single POST endpoint, `X-Amz-Target: CodeDeploy_20141006.<Op>`
  dispatch via `RouteMatcher`/`ExtractOperation` in handler.go. Verified every op in
  `GetSupportedOperations()` has a `dispatchTable()` entry and is reachable — no stub
  registrations, no ops silently dropped.

- **Epoch-seconds timestamp bug (the big one this pass)**: every `Timestamp` shape in
  CodeDeploy's model (`createTime`, `completeTime`, `registerTime`, `deregisterTime`, and the
  `createTimeRange.start`/`end` *request* filter) is serialized by the real SDK as an
  epoch-**seconds** JSON number (`smithytime.FormatEpochSeconds` / parsed with
  `smithytime.ParseEpochSeconds` — confirmed by reading
  `aws-sdk-go-v2/service/codedeploy@v1.37.0/deserializers.go` and `serializers.go` directly).
  The handler was using `time.Time.UnixMilli()` (epoch **milliseconds**, `int64`) for every
  response timestamp and `time.UnixMilli()` to parse the request-side range filter — a
  1000x wire-format mismatch in both directions. Fixed by switching every timestamp field to
  `float64` and using `pkgs/awstime.Epoch()` for output / a small `epochSecondsToTime` helper
  (mirrors the `secretsmanager` package's `time.Unix(0, int64(sec*float64(time.Second)))`
  pattern) for input. Proven with a real-SDK-client round-trip test
  (`handler_sdk_roundtrip_test.go`) rather than just unit-level JSON assertions, since a
  scale-wrong-but-well-typed number silently decodes to a garbage `time.Time` instead of
  erroring — exactly the kind of bug unit tests miss (see parity-principles.md rule 3).

- **StopDeploymentOutput.status is a *different* enum than Deployment.status**: real AWS's
  `StopStatus` enum is only ever `"Pending"` or `"Succeeded"` — it describes the outcome of
  the *stop request itself* (this backend performs it synchronously, so always
  `"Succeeded"`). The Deployment's own resulting lifecycle status (`"Stopped"`) is a
  completely separate field returned by `GetDeployment`. The handler was previously reusing
  `statusStopped` for both, which is not a valid `StopStatus` value and would fail real SDK
  unmarshaling into the `types.StopStatus` type in strict validation paths. This is a
  "looks-right-but-is-wrong" trap: don't re-flag `GetDeployment` returning `"Stopped"` after
  a stop — that part was and remains correct.

- **`ErrOnPremisesInstanceNotFound` sentinel had the wrong error code baked in**
  (`"InstanceNameRequiredException"`, which is actually a *different* real AWS exception for
  a missing/empty instance name) **and no `errorMappings` entry at all**, so any not-found
  lookup on an on-premises instance (`GetOnPremisesInstance`, `DeregisterOnPremisesInstance`)
  fell through to the generic 500 `ServiceException` branch regardless of the sentinel's own
  code — the exact "missing errCodeLookup entry" bug class called out in
  parity-principles.md rule 2. Fixed the sentinel's code to `InstanceDoesNotExistException`
  (confirmed against `types.InstanceDoesNotExistException` in the real SDK) and added the
  `errorMappings` row.

- **`SkipWaitTimeForInstanceTermination` and `PutLifecycleEventHookExecutionStatus` skipped
  the deployment-existence check** every sibling deployment-scoped op
  (`GetDeploymentInstance`, `GetDeploymentTarget`, `ListDeploymentInstances`,
  `ListDeploymentTargets`, `ContinueDeployment`, `StopDeployment`) performs via
  `h.Backend.GetDeployment(...)`. Both previously returned 200 OK for a nonexistent
  `deploymentId`. Fixed to match the sibling pattern; real AWS returns
  `DeploymentDoesNotExistException` in both cases.

- **Deliberate simplification, not a bug**: `CreateDeployment` sets `Status: "Succeeded"` and
  `CompleteTime: now + 5s` immediately at creation time rather than modeling a genuine
  `Created → Queued → InProgress → Succeeded` progression over time. `ListDeployments`'
  status filter and `StopDeployment`'s transition to `"Stopped"` both work correctly against
  this synchronous model. Don't re-flag this as a "stuck deployment" bug — it's the opposite
  problem (instant-complete, not stuck), and every consumer-visible field derived from it
  (`deploymentOverview`, status filters) is internally consistent.

- **`DeleteResourcesByExternalId` empty-envelope return is correct**, not a stub: this
  backend has no external-id-linked-resource tracking anywhere (nothing populates it), and
  the real AWS operation itself is a best-effort async cleanup with no required side effect
  visible to the caller synchronously. This is the "void-result op" pattern from
  parity-principles.md rule 4, not a disguised no-op.

- **ApplicationRevision family de-stubbed (this pass)**: `RegisterApplicationRevision`,
  `GetApplicationRevision`, `ListApplicationRevisions`, and `BatchGetApplicationRevisions`
  previously had no backing store at all — `RegisterApplicationRevision` validated the
  application existed and then discarded the revision, so `GetApplicationRevision` echoed the
  request straight back, `ListApplicationRevisions` always returned an empty list, and
  `BatchGetApplicationRevisions` echoed each requested revision with no real lookup. Added an
  `applicationRevisions *store.Table[ApplicationRevision]` keyed by
  `appName + "\x00" + canonical-JSON(RevisionLocation)` (`applicationRevisionKey` in
  store_setup.go — two `RevisionLocation` values with identical fields always produce the same
  key, matching how real CodeDeploy deduplicates revision registrations) with a `byApplication`
  index. Registered directly on `b.registry` as a "clean" table (no live `*tags.Tags` field, no
  DTO wrapper needed — see persistence.go's dirty-table doc comment). `CreateDeployment` now
  calls `touchApplicationRevisionForDeployment`, which auto-registers an unseen revision (real
  CodeDeploy auto-registers revisions supplied directly to `CreateDeployment`) and stamps
  `FirstUsedTime`/`LastUsedTime`/`DeploymentGroups` — the last of which also removes the
  deployment group from every *other* revision of the same application, since a deployment
  group targets exactly one revision at a time. `DeleteApplication`/`UpdateApplication` gained
  `deleteApplicationRevisions`/`renameApplicationRevisions` cascades so no ghost revision rows
  survive an app delete or outlive an app rename under the old name. Wire shapes
  (`genericRevisionInfo`/`revisionInfo`/`revisionLocation` field names, `GenericRevisionInfo`'s
  `registerTime`/`firstUsedTime`/`lastUsedTime` as epoch-seconds `float64`) verified against
  `aws-sdk-go-v2/service/codedeploy@v1.37.0/deserializers.go`'s
  `awsAwsjson11_deserializeDocumentGenericRevisionInfo`/`...RevisionInfo` and proven with a real
  SDK-client round trip (`Test_SDKRoundTrip_ApplicationRevision_EpochSeconds`). New sentinel
  `ErrRevisionNotFound` → `RevisionDoesNotExistException`, 404 (confirmed against
  `types.RevisionDoesNotExistException`).

- **Deployment-instance/target family de-stubbed (this pass)**: `GetDeploymentInstance`,
  `GetDeploymentTarget`, `ListDeploymentInstances`, `ListDeploymentTargets`,
  `BatchGetDeploymentInstances`, and `BatchGetDeploymentTargets` previously fabricated a
  `Succeeded` record for literally any requested ID (`Get*`/`Batch*`) or always returned an
  empty list (`List*`), regardless of whether that ID (or any target at all) actually existed.
  Added `(b *InMemoryBackend) deploymentTargets(d *Deployment) []DeploymentTargetRecord`
  (deployment_instances.go), computed on read from the deployment's owning deployment group's
  *real* configuration rather than persisted as a separate table (so it always reflects current
  on-premises instance/tag state, and needs no snapshot-version bump): for `ComputePlatform ==
  "Server"`, one `instanceTarget` per registered, non-deregistered on-premises instance whose
  tags satisfy the deployment group's `OnPremisesInstanceTagFilters`/`OnPremisesTagSet` (new
  `matchesOnPremisesTargeting`/`matchesTagSetGroups` helpers in on_premises_instances.go); for
  `"ECS"`, one `ecsTarget` per `ECSServices` entry (`clusterName/serviceName` as the deterministic
  `TargetID`); for `"Lambda"`, exactly one `lambdaTarget` keyed by the deployment ID itself
  (real CodeDeploy Lambda/ECS deployments always have exactly one and, respectively,
  `len(ECSServices)` targets — confirmed against
  `types.DeploymentTargetListSizeExceededException`'s doc comment). Every target's `Status` is
  derived from the deployment's own current `Status` via `targetStatusForDeployment`
  (`Succeeded`/`Failed`/`Skipped`/`InProgress`) instead of a hardcoded literal that ignored
  what actually happened to the deployment (e.g. a stopped deployment's targets now correctly
  report `Skipped`, proven by `TestDeploymentTargets_StatusTracksDeploymentStatus`). `Get*`/
  `GetDeploymentTarget` on an unresolvable ID now returns the new
  `ErrDeploymentTargetNotFound` → `DeploymentTargetDoesNotExistException` (404) sentinel instead
  of fabricating a match; `Batch*` silently omits unresolvable IDs, matching this codebase's
  established Batch* convention (`BatchGetApplications` etc.). `DeploymentTarget` union wire
  shape (`deploymentTargetType` PascalCase enum values `InstanceTarget`/`ECSTarget`/
  `LambdaTarget`, nested `instanceTarget`/`ecsTarget`/`lambdaTarget` member field names,
  `lastUpdatedAt` as epoch-seconds `float64`) verified against
  `awsAwsjson11_deserializeDocumentDeploymentTarget`/`...InstanceTarget`/`...ECSTarget`/
  `...LambdaTarget` and proven with a real SDK-client round trip
  (`Test_SDKRoundTrip_DeploymentTarget_EpochSeconds`). KNOWN, DOCUMENTED LIMITATION: this
  backend has no live EC2 instance registry, so `Ec2TagFilters`/`Ec2TagSet` resolve zero
  targets — only the on-premises side of `"Server"` targeting is modeled (see gaps).

- **ContinueDeployment's real gap was narrower than "blue/green lifecycle" (this pass)**:
  `READY_WAIT`/`TERMINATION_WAIT` are `types.DeploymentWaitType` values (an input parameter
  on `ContinueDeploymentInput`), not `types.DeploymentStatus` values — a deployment's status
  is never `"READY_WAIT"`; the closest status is the separate `"Ready"` value. The actual
  fixable gap: `ContinueDeployment` accepted a deployment in any status and never read
  `deploymentWaitType` off the wire at all. Added the real precondition
  (`types/errors.go:556-557` `DeploymentIsNotInReadyStateException`, `:221`
  `DeploymentAlreadyCompletedException`) and wait-type enum validation
  (`InvalidDeploymentWaitTypeException`). `CreateDeployment`'s synchronous completion means
  no deployment here ever reaches `"Ready"`, so `ContinueDeployment` now always errors —
  correct given this backend's lifecycle, not a regression from a previously-working feature.

- **Ec2TagFilters/Ec2TagSet were stored but never evaluated, not dropped on input (this
  pass)**: `CreateDeploymentGroup`/`UpdateDeploymentGroup` always persisted them correctly
  (`dg.Ec2TagFilters = input.Ec2TagFilters` et al.) and `GetDeploymentGroup` echoed them back
  — the gap was entirely in `deploymentTargets()`, which had no EC2 instance data to match
  against. Wired `services/codedeploy` to `services/ec2` via the same lazy
  `SetAppConfig`/`siblingServices` pattern `services/mgn` uses (`cross_service.go`); `*CLI`
  already exposed `GetEC2Handler()` for that pattern, so no `cli.go` changes were needed.

- **CreateDeployment's `fileExistsBehavior` was accepted and persisted with no enum check
  (this pass)**: any string round-tripped through `GetDeployment`/`BatchGetDeployments`. Real
  AWS defines `InvalidFileExistsBehaviorException` specifically for `CreateDeployment`
  (confirmed in its deserializer's error switch); added validation against
  `DISALLOW`/`OVERWRITE`/`RETAIN`.

- **Two banned `//nolint:gocognit,cyclop,funlen` removed via decomposition (this pass)**:
  `dgToOutput`/`dgInputFromWire` in handler_deployment_groups.go were single ~130-line
  functions doing wire-format conversion for every optional deployment-group sub-structure
  (load balancer info, blue/green config, alarms, auto-rollback, EC2/on-premises tag sets)
  inline. Split each into one `dgXToOutput`/`dgXFromWire` helper per sub-structure (7 helpers
  each direction); the top-level functions now just assemble the struct literal and loop over
  the flat slice fields, with zero behavior change (proven by the existing
  `deployment_groups_test.go` suite passing unmodified). One more banned
  `//nolint:gocognit` removed from `matchesTagFilters` in on_premises_instances.go by
  extracting the per-filter `KEY_ONLY`/`VALUE_ONLY`/default switch into `matchesOneTagFilter`
  (also fixed its comment, which incorrectly called the default case "EQUALS" — the real
  `TagFilterType` enum value is `KEY_AND_VALUE`, confirmed against `types.TagFilterType` in
  enums.go; the code was already correct, only the comment was wrong). All three were the
  full set of banned nolints flagged for this service — `grep -rnE
  'nolint:[a-z,]*(cyclop|gocyclo|gocognit|funlen)' services/codedeploy/` now returns empty.

- **ERROR path verified against `cmd/errcodeaudit`'s near-miss sweep (this pass)**. Four
  confident findings, two distinct wrong constants each reused at multiple call sites:
  - `ErrValidation` ("InvalidParameterValueException", not a real CodeDeploy type at all) was
    reused across four semantically unrelated failures. Each now uses the code its own
    op's `deserializeOpError<Op>` actually models: `CreateDeploymentConfig`'s bad
    `computePlatform` → the pre-existing `ErrInvalidComputePlatform`
    (`InvalidComputePlatformException`, was wired to the wrong sentinel);
    `BatchGetApplicationRevisions`' >25-revision case → new `ErrBatchLimitExceeded`
    (`BatchLimitExceededException`); `RegisterOnPremisesInstance`'s malformed (not missing)
    instance name → new `ErrInvalidInstanceName` (`InvalidInstanceNameException`);
    `TagResource`'s reserved-prefix/oversized-key/oversized-value tag rejection → new
    `ErrInvalidTagsToAdd` (`InvalidTagsToAddException`).
  - `errInvalidRequest` ("InvalidRequestException", also not a real type) backed the
    "field is required" check at ~35 call sites across nearly every operation in this
    package — the single-wrong-constant-reused pattern this campaign looks for. Each op
    models its own distinct `<Field>RequiredException`; replaced with nine per-field
    sentinels (`ErrApplicationNameRequired`, `ErrDeploymentGroupNameRequired`,
    `ErrDeploymentIDRequired`, `ErrInstanceIDRequired`, `ErrDeploymentTargetIDRequired`,
    `ErrDeploymentConfigNameRequired`, `ErrInstanceNameRequired`, `ErrResourceArnRequired`,
    `ErrGitHubTokenNameRequired`), each verified against the specific op(s) that raise it,
    splitting combined two-field checks (e.g. `CreateDeploymentGroup`'s
    applicationName+deploymentGroupName) into two ordered checks so each field gets its own
    correct code. `errInvalidRequest` itself is now unused and removed.
  - `errUnknownAction` (dispatch-miss for an unrecognized `Action`) still maps to
    `InvalidRequestException` — deliberately left unfixed: no CodeDeploy operation models
    this condition (there is no operation to consult; the routed action itself is
    unrecognized), so inventing a replacement code would be exactly the fabrication this
    campaign exists to remove. This is the one remaining confident `errcodeaudit` finding
    for this service.
  - **Adjacent bug found while in this code, not from the tool**: `DeleteDeploymentConfig`'s
    built-in-config guard used `ErrDeploymentConfigInUse` (`DeploymentConfigInUseException`)
    — a real CodeDeploy exception, but the wrong one for this op.
    `DeleteDeploymentConfig`'s own deserializer models `InvalidOperationException` for
    exactly this case; `DeploymentConfigInUseException` is only modeled by
    `AddTagsToOnPremisesInstances`/`RemoveTagsFromOnPremisesInstances`/
    `UpdateDeploymentGroup`'s tag-limit case, none of which this backend ever triggered it
    from. Renamed to `ErrDeploymentConfigIsDefault` mapped to `InvalidOperationException`.
    Same root cause as `ErrOnPremisesInstanceNotFound` above: a real code borrowed from the
    wrong operation in the same family.
  - Same trap caught `ErrTagLimitExceeded` (`TagLimitExceededException`, also real but only
    modeled by the three ops above, never `TagResource`): `TagResource`'s own too-many-tags
    check now uses `ErrInvalidTagsToAdd` like its other tag-content rejections.
  - Five existing tests asserted the fabricated/misrouted codes as correct and are fixed:
    `TestDeploymentConfigs_DefaultsCannotDelete` (renamed assertion from
    `DeploymentConfigInUseException` to `InvalidOperationException`),
    `TestDeploymentConfigs_ErrValidationMapping` (renamed to
    `TestDeploymentConfigs_ErrInvalidComputePlatformMapping`, asserts
    `InvalidComputePlatformException`), two cases in `TestOnPremisesInstance`-adjacent table
    tests in `on_premises_instances_test.go` (now assert `InvalidInstanceNameException`), and
    `TestTags_ResourceTagLimits`/`TestTags_ResourceExceedsMaxTags` in `tags_test.go` (now
    assert `InvalidTagsToAddException`).
  - New coverage driving the real typed SDK client end-to-end, asserting the specific typed
    exception via `errors.As` (not string/presence checks):
    `error_codes_fixes_test.go`.

- **Re-verified independently, 2026-08-30 (gopherstack-r3pr, no code change)**: re-ran
  `cmd/errcodeaudit`; `errUnknownAction` → `InvalidRequestException` (handler.go:285) is
  still the only confident finding. Confirmed against `types/errors.go`
  (aws-sdk-go-v2/service/codedeploy@v1.38.4): no `InvalidRequestException` type exists
  anywhere in the module, and none of its 47 `deserializeOpError<Op>` functions could —
  an unrecognized routed `Action` string doesn't correspond to any real CodeDeploy
  operation, so there is no operation's own deserializer to consult. Left unfixed, per
  the existing comment at the call site; verdict unchanged.

- **Triaged, 2026-09-07 (gopherstack-3pz8, `errtargetaudit` class A findings)**: 6
  findings, all `domain=Handler mechanism=sentinel reference`, grouped into 4 root
  causes by code. Verified each against `awk "/deserializeOpError<Op>\(/,/^}/" \
  deserializers.go | grep -oE '"[A-Za-z0-9]+"'` plus the matching
  `docs.aws.amazon.com/codedeploy/latest/APIReference/API_<Op>.html` Errors section.
  No override-helper exists anywhere in this package (`grep -rn "errors.Is(err"
  *.go` → only handler.go's own dispatch loop), so class-7 (handler overrides the
  code) does not apply to any of the six.
  - **Fixed (1, unambiguous)**: `GetOnPremisesInstance` emitted
    `InstanceDoesNotExistException` (`ErrOnPremisesInstanceNotFound`) for a missing
    instance, but its own deserializer models `InstanceNotRegisteredException`
    instead — confirmed both in `deserializers.go` and the doc page's Errors list
    ("The specified on-premises instance is not registered."). Exactly one
    declared code fit, so this was a clean sentinel swap: new
    `ErrOnPremisesInstanceNotRegistered` sentinel, wired into `errorMappings`,
    call site in `on_premises_instances.go` updated. Pre-existing test
    `TestOnPremisesInstances_NotFoundErrorMapping/GetOnPremisesInstance` had been
    asserting the wrong code (`InstanceDoesNotExistException`) since it was
    written to close a *different* prior gap (missing errorMappings entry
    entirely, causing a 500) — fixed to assert `InstanceNotRegisteredException`.
    Added `TestGetOnPremisesInstance_NotRegisteredNotDoesNotExist` asserting the
    correct code, the wrong code's absence, and that a failed lookup creates no
    instance.
  - **Left unfixed (5, evidence gap)**: `DeleteApplication` /
    `ApplicationDoesNotExistException`, `DeleteDeploymentGroup` /
    `ApplicationDoesNotExistException`, `DeleteDeploymentGroup` /
    `DeploymentGroupDoesNotExistException`, `DeleteDeploymentConfig` /
    `DeploymentConfigDoesNotExistException`, and `DeregisterOnPremisesInstance` /
    `InstanceDoesNotExistException`. Each op's own deserializer models *no*
    not-found code at all for the missing-resource case (confirmed above) — the
    declared-set mismatch proves the current 404 is wrong, but does not by itself
    prove the remedy is silent success: all four ops' doc pages carry only the
    generic "If the action is successful, the service sends back an HTTP 200
    response with an empty HTTP body" boilerplate (the same sentence that
    appears on `codepipeline`'s `DisableStageTransition`, which does still error
    on a missing resource), not a semantic idempotent-delete sentence like
    workmail's genuinely-earned "Deleting already deleted and non-existing rules
    does not produce an error." `DeleteDeploymentConfig` additionally has an
    ambiguous candidate (`InvalidOperationException`, already used a few lines
    below for the built-in-config case) that is plausible but not evidenced.
    Landmine comments left at all five call sites naming the gap and citing this
    issue; no behavior changed for these five. Neutered the one line that was
    changed (`GetOnPremisesInstance`'s sentinel swap, and separately the new
    `errorMappings` entry) — each reversion compiled and made the corresponding
    test fail as expected, then was restored.
  - `go test -race -count=1 ./services/codedeploy/...` and `golangci-lint run
    services/codedeploy/...` (`0 issues.`) both pass after the fix.

- **Re-verified independently, 2026-09-07 (gopherstack-l81f, no code change)**: re-derived
  each of the five left-unfixed ops' declared sets individually (not by count) via
  `awk "/deserializeOpError<Op>\(/,/^}/" deserializers.go | grep -oE '"[A-Za-z0-9]+"'`
  against codedeploy@v1.38.4 — `DeleteApplication` {ApplicationNameRequiredException,
  InvalidApplicationNameException, InvalidRoleException}, `DeleteDeploymentGroup`
  {ApplicationNameRequiredException, DeploymentGroupNameRequiredException,
  InvalidApplicationNameException, InvalidDeploymentGroupNameException,
  InvalidRoleException}, `DeleteDeploymentConfig` {DeploymentConfigInUseException,
  DeploymentConfigNameRequiredException, InvalidDeploymentConfigNameException,
  InvalidOperationException}, `DeregisterOnPremisesInstance`
  {InstanceNameRequiredException, InvalidInstanceNameException} — all confirmed to
  contain no not-found code, matching 3pz8 exactly. Read every declared candidate's
  doc comment in `types/errors.go`: none reads as not-found (`InvalidOperationException`
  is "An invalid operation was detected." — no word-for-word link to a missing
  resource). Independently fetched the live `API_<Op>.html` reference pages for all
  four (`DeleteApplication`, `DeleteDeploymentGroup`, `DeleteDeploymentConfig`,
  `DeregisterOnPremisesInstance`) and confirmed each still carries only the generic
  "If the action is successful, the service sends back an HTTP 200 response with an
  empty HTTP body" boilerplate — no idempotent-delete sentence anywhere, same
  conclusion 3pz8 reached. `cmd/errtargetaudit -dir codedeploy` still reports exactly
  these 5 class-A findings, byte-for-byte the same emission sites/codes as 3pz8's
  table. Verdict unchanged: no safe remedy for any of the five; landmine comments at
  all five call sites left as-is (already accurate and complete). No `.go` files
  touched for this issue.
