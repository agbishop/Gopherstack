service: opsworks
sdk_module: aws-sdk-go-v2/service/opsworks@v1.31.0   # exists in the module cache but is NOT
                                                       # a go.mod dependency of this repo (see
                                                       # note below) — audited by reading the
                                                       # module source directly, not via import.
last_audit_commit: 5f0e2722b
last_audit_date: 2026-08-15
# gopherstack-6flj/21my re-sweep (2026-08-29): spot-checked filter/sort-drop risk
# on the ops most exposed to it (DescribeCommands' CommandIds/DeploymentId/
# InstanceId, DescribeDeployments' DeploymentIds/AppId/StackId,
# DescribeLoadBasedAutoScaling's LayerIds, DescribeTimeBasedAutoScaling's
# InstanceIds -- all confirmed honoring the FULL filter list, not truncated to
# the first element the way the already-fixed DescribeElasticLoadBalancers
# LayerIds bug was) plus a fresh member-count re-verification of Command (10 of
# 10 SDK deserializer cases) and StackSummary (6 of 6). No new bug found this
# pass -- see Notes for what was and wasn't re-checked; this was a targeted
# spot-check against the prior 4 passes' exhaustive per-item field-diff, not a
# from-scratch re-audit of all 74 ops.
overall: B            # re-audited live (gopherstack-vjj2) after the 2026-06-03..2026-08-08
                       # unreachability window closed; 2 more real bugs found+fixed via live
                       # HTTP requests, but there is still no SDK-driven test/integration/
                       # suite for this service, so it does not clear this repo's A bar
                       # (gopherstack-parity-audit skill: "A = full integration-suite proof +
                       # every buildable gap closed"). The prior "A" predates this rubric
                       # clarification and was also never exercised by a live request.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateStack: {wire: ok, errors: ok, state: ok, persist: ok, note: "now validates all 4 required members (Name/Region/DefaultInstanceProfileArn/ServiceRoleArn); wire no longer emits invented 'Status' field; now also accepts+echoes VpcId/Attributes/ConfigurationManager/ChefConfiguration (gopherstack-4uhx) -- rest of the optional surface (AgentVersion, CustomCookbooksSource, CustomJson, DefaultAvailabilityZone, DefaultOs, DefaultRootDeviceType, DefaultSshKeyName, DefaultSubnetId, HostnameTheme, UseCustomCookbooks, UseOpsworksSecurityGroups) remains unmodeled, see deferred"}
  DescribeStacks: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateStack: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteStack: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-09-07 (gopherstack-bael): missing delete precondition -- api_op_DeleteStack.go: \"You must first delete all instances, layers, and apps or deregister registered instances.\" Previously cascaded unconditionally; now returns ValidationException while any instance, layer, or app remains. Deployments/permissions/volumes/RDS/ECS associations (none of which carry a documented precondition) still cascade once that guard clears."}
  CloneStack: {wire: ok, errors: ok, state: ok, persist: ok}
  StartStack: {wire: ok, errors: ok, state: ok, persist: ok}
  StopStack: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateLayer: {wire: ok, errors: ok, state: ok, persist: ok, note: "now validates all 4 required members and restricts Type to the real LayerType enum"}
  DescribeLayers: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateLayer: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteLayer: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-09-04 (gopherstack-2rx): missing delete precondition -- api_op_DeleteLayer.go: \"You must first stop and then delete all associated instances or unassign registered instances.\" Previously deleted the layer unconditionally, leaving any instance still assigned to it (storedInstance.LayerID) with a dangling reference to a deleted layer. Now returns ValidationException if any instance is still assigned."}
  CreateInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "now validates all 3 required members (StackId/LayerIds/InstanceType) and that the target layer exists -- previously silently accepted a nonexistent layer ID"}
  RegisterInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "now validates the required StackId member (gopherstack-4uhx) -- an empty StackId previously fell through to the stack-lookup's ResourceNotFoundException instead of ValidationException"}
  DeregisterInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  AssignInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "now verifies the target layer exists AND belongs to the same stack as the instance -- previously accepted any layer ID, including nonexistent or cross-stack ones, without checking either. Also now enforces AWS's documented business rule (api_op_AssignInstance.go: \"You cannot use this action with instances that were created with OpsWorks Stacks\") via storedInstance.Registered (gopherstack-4uhx)"}
  UnassignInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeInstances: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED wire bug -- was emitting a singular 'LayerId' string; real types.Instance has a plural 'LayerIds' []string member, so a real SDK client's Instance.LayerIds field would never have populated from this backend's old response"}
  UpdateInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-09-04 (gopherstack-2rx): missing delete precondition -- api_op_DeleteInstance.go: \"You must stop an instance before you can delete it.\" Previously deleted an instance regardless of Status; now returns ValidationException unless status is 'stopped' (both CreateInstance/RegisterInstance already default new instances to 'stopped', so this only bites an instance that was StartInstance'd and never stopped again)."}
  StartInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  StopInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  RebootInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateApp: {wire: ok, errors: ok, state: ok, persist: ok, note: "now validates all 3 required members and restricts Type to the real AppType enum; wire no longer emits invented 'Arn' field (real types.App has none)"}
  DescribeApps: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateApp: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteApp: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDeployment: {wire: ok, errors: ok, state: ok, persist: ok, note: "synchronous — completes with Status=successful immediately, no stuck 'running' state"}
  DescribeDeployments: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeCommands: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED -- now restricted to stack/layer ARNs only, matching the real API's documented 'stack or layer's ARN' contract; previously also accepted instance/app ARNs (not real taggable resources)"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "same stack/layer-only restriction as TagResource"}
  ListTags: {wire: ok, errors: ok, state: ok, persist: ok, note: "paginated via sorted-key nextToken; same stack/layer-only restriction as TagResource"}
families:
  UserProfile: {status: ok, note: "CreateUserProfile/DeleteUserProfile/DescribeUserProfiles/UpdateUserProfile/DescribeMyUserProfile/UpdateMyUserProfile all mutate real state and persist"}
  ElasticLoadBalancer: {status: ok, note: "Attach/Detach/Describe all real. FIXED 2026-08-15 (gopherstack-6flj wrapper-key sweep): DescribeElasticLoadBalancers' real, plural LayerIds filter member (confirmed against aws-sdk-go-v2/service/opsworks@v1.31.0's api_op_DescribeElasticLoadBalancers.go) was silently discarded -- the handler truncated it to only its first element and the backend method's own parameter was named `_`, never read at all. Now honors the full list via slices.Contains. Per-item fields AvailabilityZones/Ec2InstanceIds/SubnetIds/VpcId remain unmodeled on the wire -- see gaps, structural (this backend has no VPC/subnet/EC2-instance model to source them from)."}
  ElasticIp: {status: ok, note: "Register/Deregister/Associate/Disassociate/Describe/Update all real. FIXED 2026-08-15 (gopherstack-6flj wrapper-key sweep): RegisterElasticIpInput's real, required StackId member was entirely unmodeled -- the handler instead read a fabricated 'Region' field that does not exist on the real input at all, and an empty/missing StackId was never rejected (200 instead of the real API's required-member ValidationException). Now validates StackId is present (and that the referenced stack exists) and threads it through to DescribeElasticIps' real StackId filter member, which was also previously discarded. StackId is kept as an internal-only field (storedElasticIP/ElasticIP.StackID) and deliberately never serialized on the wire -- the real types.ElasticIp has no StackId member."}
  Volume: {status: ok, note: "Register/Deregister/Assign/Unassign/Describe/Update all real. DescribeVolumes now also filters by StackId (real DescribeVolumesInput supports it; this backend previously silently dropped the parameter). Wire no longer emits invented 'StackId' field (real types.Volume has none). AssignVolume now verifies the instance belongs to the same stack the volume was registered with. RegisterVolume now validates the required StackId member (gopherstack-4uhx). AssignVolume's own required VolumeId member is now pre-validated for emptiness too (FIXED 2026-08-23, batch14) -- an empty VolumeId now returns ValidationException instead of falling through to ResourceNotFoundException."}
  RdsDbInstance: {status: ok, note: "Register/Deregister/Describe/Update all real. RegisterRdsDbInstance now validates all 4 required members (StackId/RdsDbInstanceArn/DbUser/DbPassword) and the wire now echoes DbPassword back as the literal '*****FILTERED*****' AWS always returns (gopherstack-4uhx). Engine and MissingOnRds remain unmodeled -- see gaps, this is structural (would need cross-service wiring to the rds backend, out of this package's scope)."}
  EcsCluster: {status: fixed, note: "Register/Deregister/Describe all real. FIXED 2026-08-08: DescribeEcsClusters wire emitted an invented 'Status' field -- real types.EcsCluster (SDK v1.31.0) has no such member, only EcsClusterArn/EcsClusterName/StackId/RegisteredAt. Removed from the wire; internal storedEcsCluster.Status kept for bookkeeping only. RegisterEcsCluster now also validates the required StackId member (gopherstack-4uhx), alongside the already-validated EcsClusterArn. FIXED 2026-08-30: unfiltered DescribeEcsClusters paginated over unsorted Go map order, dropping/duplicating clusters across a page walk -- now sorted by EcsClusterArn before pagination (see ops family note above)."}
  Permission: {status: ok, note: "SetPermission/DescribePermissions real, composite-keyed by stackID+iamUserArn. SetPermission now validates both required members (StackId/IamUserArn) -- previously accepted an empty IamUserArn with no error at all, and an empty StackId fell through to ResourceNotFoundException instead of ValidationException. Level is now also restricted to the API's documented closed set (deny/show/deploy/manage/iam_only) -- previously accepted any string (gopherstack-4uhx)."}
  AutoScaling: {status: ok, note: "SetTimeBasedAutoScaling/DescribeTimeBasedAutoScaling/SetLoadBasedAutoScaling/DescribeLoadBasedAutoScaling all real"}
  Misc: {status: fixed, note: "GrantAccess/DescribeServiceErrors(always empty, correct)/DescribeRaidArrays(always empty, correct)/DescribeOperatingSystems(static list) all match AWS's actual mostly-static/deprecated-service behavior. GetHostnameSuggestion FIXED 2026-08-08 (see gaps-closed note below) -- was entirely unaudited by the previous pass despite being in GetSupportedOperations. DescribeStackProvisioningParameters FIXED 2026-08-15 (gopherstack-6flj): the real, dedicated top-level AgentInstallerUrl member was also being duplicated under a fabricated 'AgentInstallerUrl' key inside the free-form Parameters map, which no real response ever carries -- Parameters is now returned empty (honest: this backend tracks none of AWS's real internal agent-bootstrap keys) rather than containing an invented one. FIXED 2026-08-30: DescribeAgentVersions's static list is real AWS behavior, but its ConfigurationManager filter was dropped entirely -- see ops family note above."}
gaps:                     # divergences from the real API, not fixed this pass
  - "ElasticLoadBalancer responses omit AvailabilityZones/Ec2InstanceIds/SubnetIds/VpcId -- all real, optional types.ElasticLoadBalancer members, but this backend's ElasticLoadBalancer domain struct has no VPC/subnet/EC2-instance concept at all to source them from (only ElasticLoadBalancerName/Region/DNSName/StackID/LayerID are tracked). Structural, same class as the App/Layer/Instance optional-surface gaps below, not fixed this pass (gopherstack-6flj)."
  - "RdsDbInstance responses still omit Engine and MissingOnRds (DbPassword is now fixed, see ops.RdsDbInstance -- gopherstack-4uhx). Both remaining fields are real (optional) members of types.RdsDbInstance, but neither has a source: Engine is not a RegisterRdsDbInstance input member at all (nothing to derive it from without inventing a value), and MissingOnRds requires simulated drift detection against a real RDS instance's existence, which is a cross-service concern this package has no model for (this backend does not talk to services/rds). Both are genuinely structural, not a scope choice -- modeling them would require either fabricating data (banned) or wiring opsworks to query the rds service backend by ARN, which is out of services/opsworks's bounds."
  - "FIXED 2026-08-23 (batch14): AssignVolume's required VolumeId member (RegisterVolume's own required StackId was fixed in gopherstack-4uhx, see families.Volume) is now pre-validated for emptiness -- an empty VolumeId now returns ValidationException instead of falling through to the volume-lookup's ResourceNotFoundException. Confirmed against aws-sdk-go-v2/service/opsworks@v1.31.0's api_op_AssignVolume.go / validateOpAssignVolumeInput (VolumeId required, InstanceId not). TestAssignVolumeValidation (volumes_test.go), hand-reverted to confirm it fails with 404 ResourceNotFoundException pre-fix."
  - "No test/integration/*_parity_test.go suite exists for opsworks (the deprecated SDK isn't a go.mod dependency, so a client-driven integration test needs either vendoring it or hand-rolling raw HTTP requests in the integration-test harness style). This is why overall stays at B rather than A per the gopherstack-parity-audit skill's rubric, even though this pass's live-HTTP verification covered all 73 ops. Building that suite is a real, nontrivial follow-on task, not done this pass."
  - "Error responses (handleError, all branches) are sent with Content-Type: application/json rather than application/x-amz-json-1.1, unlike success responses which correctly get the awsjson1.1 content type from service.HandleTarget. Confirmed harmless for a real aws-sdk-go-v2 client -- deserializers.go's awsAwsjson11_deserializeOpError* functions key off the X-Amzn-ErrorType header and the body's __type/message fields, never Content-Type -- but it's still a wire divergence from a real server. This is a repo-wide pattern (shared by roughly half the awsjson1.1 services grepped, not opsworks-specific), so left unfixed here as out of this pass's bounded scope."
deferred:                 # consciously not audited/implemented this pass (scope)
  - "CreateStack's VpcId/Attributes/ConfigurationManager/ChefConfiguration are now modeled (gopherstack-4uhx), but the rest of CreateStack's optional surface (AgentVersion, CustomCookbooksSource, CustomJson, DefaultAvailabilityZone, DefaultOs, DefaultRootDeviceType, DefaultSshKeyName, DefaultSubnetId, HostnameTheme, UseCustomCookbooks, UseOpsworksSecurityGroups) is not, and CreateLayer/CreateApp/CreateInstance's full optional surfaces (CloudWatchLogsConfiguration, LifecycleEventConfiguration, VolumeConfigurations, AppSource, DataSources, Environment, SslConfiguration, BlockDeviceMappings, etc.) remain entirely unmodeled -- only the fields this backend's Handler already decodes were audited for wire-shape correctness."
leaks: {status: clean, note: "No goroutines, timers, or background schedulers in this package — every op is synchronous, so there is nothing to leak. Confirmed no time.AfterFunc/go func/Ticker usage anywhere in services/opsworks/."}
---

## Notes

**Registration (2026-08-08, gopherstack-91e0)**: this package had no `Provider{}`
entry in `cli.go`'s `getServiceProviders` chain from 2026-06-03 (an accidental
drop during an unrelated FSx-service PR's rebase-conflict resolution, per git
history) until this pass. Every audit above, including the grade below, was
performed against code that could not actually receive a request. Now
registered and wired into `wireResourceGroupsTagging`
(`wireTaggingOpsWorks`); `cli_service_registration_test.go` (repo root) now
fails the build if a `services/*/` directory ever again lacks a provider
entry.

**Protocol**: awsjson1.1 (`application/x-amz-json-1.1`), single POST endpoint,
`X-Amz-Target: OpsWorks_20130218.<Op>` dispatch. Route matcher
(`RouteMatcher`) checks the target-prefix only; `ExtractOperation` trims the
prefix and dispatch looks the trimmed name up in `h.ops` (built once at
`NewHandler`/`Reset` from `buildOps()`). `GetSupportedOperations()` and the
dispatch-table keys are asserted equal by `sdk_completeness_test.go` — keep
both lists in sync when adding an op.

**Timestamps**: `CreatedAt`/`CompletedAt`/etc. are ISO8601 strings, not
epoch-seconds numbers — confirmed against the real SDK's
`types.Stack.CreatedAt *string` (and equivalent fields across
Layer/Instance/App/Deployment/Command/EcsCluster), unlike some other
JSON-1.1 services in this repo that use `pkgs/awstime.Epoch`. Do not "fix"
this to epoch format; that would be the actual bug.

The Go SDK types this field as a bare `*string` (no smithy timestamp shape,
no client-side format enforcement), so the exact character format is
whatever the real server emits — confirmed via the AWS CLI's own doc
examples (`aws-cli 2.4.18` opsworks `describe-stacks`/`describe-apps`/
`describe-instances` reference pages): always UTC, always a literal
`"+00:00"` suffix, e.g. `"2013-08-01T22:53:42+00:00"` — never `"Z"`.

(gopherstack-c29a, 2026-08-10) The previous fix here formatted with the Go
layout string `"2006-01-02T15:04:05+00:00"`, which is NOT a Go reference
layout — `-0700`/`-07:00`/`Z0700`/`Z07:00` are the only recognized offset
tokens, so `+00:00` was emitted as literal text regardless of the input
`time.Time`'s actual zone/offset, and the H:M:S digits were never converted
to UTC either. A `time.Time` in any non-UTC zone printed its own local
wall-clock digits with a falsely-claimed `+00:00` suffix. Fixed via
`formatOpsWorksTime` (handler.go): `t.UTC().Format("2006-01-02T15:04:05-07:00")`
— `.UTC()` normalizes the clock digits, and `-07:00` (not `Z07:00`) is the
Go reference token that renders a numeric offset unconditionally, matching
the real API's `+00:00`-not-`Z` convention at zero offset. This is a
wire-shape fix, not a persistence-format change: the persisted snapshot
serializes the underlying `time.Time` struct fields directly via
`encoding/json`'s default `MarshalJSON` (RFC3339Nano), never through this
function, which only touches the outgoing API response — so
`opsworksSnapshotVersion` did not need bumping and old snapshots still
restore.

**SDK availability**: `github.com/aws/aws-sdk-go-v2/service/opsworks@v1.31.0`
genuinely exists (AWS still generates deprecated-service SDK clients). It is
**not** a dependency of this repo's go.mod/go.sum; this pass fetched it into a
scratch module (`go get` in a throwaway `go.mod` under a temp dir) to read the
real `types.go`/`api_op_*.go` source directly, then discarded the scratch
module. Future audits: do the same, or read
`$(go env GOMODCACHE)/github.com/aws/aws-sdk-go-v2/service/opsworks@<version>`
directly if it's already present in the module cache — do not trust only this
backend's own JSON output as the source of truth for wire shape.

## This pass's fixes (2026-07-23)

Full field-diff of every op's request/response shape against the real SDK's
`types.go`/`api_op_*.go` source (v1.31.0), plus a full read of the previous
audit's `gaps`/`deferred` lists, turned up three previously-known items to
close and three newly-found wire-shape bugs:

1. **`InstancesCount` field names (previously-known gap, now fixed)** —
   `DescribeStackSummary`'s `InstancesCount` used invented field names
   (`Total`, `Starting`, `Stopping`-without-the-rest) that don't exist on the
   real `types.InstancesCount`. Replaced with the exact 19-field real set
   (`Assigning`, `Booting`, `ConnectionLost`, `Deregistering`, `Online`,
   `Pending`, `Rebooting`, `Registered`, `Registering`, `Requested`,
   `RunningSetup`, `SetupFailed`, `ShuttingDown`, `StartFailed`,
   `StopFailed`, `Stopped`, `Stopping`, `Terminated`, `Terminating`,
   `Unassigning` — confirmed against `types.go`). This backend's instance
   state machine only ever produces `online`/`stopped` (see
   `StartInstance`/`StopInstance`), so only `Online`/`Stopped` are ever
   non-zero; the rest exist purely for wire-shape completeness.

2. **Invented `Status`/`StackArn` fields (previously-known gap, now fixed)**
   — `stacksToJSON` emitted a `Status` field not on the real `types.Stack`;
   `DescribeStackProvisioningParameters` emitted a `StackArn` field not on
   the real `DescribeStackProvisioningParametersOutput` (which has only
   `AgentInstallerUrl`/`Parameters`). Both removed from the wire, and the
   now-dead `storedStack.Status` field (only ever set to the constant
   `"running"` and never read anywhere else) was deleted rather than kept as
   inert bookkeeping.

3. **Missing required-field validation (previously-known gap, now fixed for
   the flagged ops)** — `CreateStack` (`Name`/`Region`/
   `DefaultInstanceProfileArn`/`ServiceRoleArn`), `CreateLayer`
   (`Name`/`Shortname`/`StackId`/`Type`-restricted-to-enum), `CreateApp`
   (`Name`/`StackId`/`Type`-restricted-to-enum), and `CreateInstance`
   (`StackId`/`LayerIds`/`InstanceType`) now reject requests missing a
   real-API "This member is required" field with `ValidationException`,
   matching what a real AWS server would do for a raw/non-SDK caller that
   bypasses the SDK's client-side required-field check. `CreateInstance` also
   gained a check that its target layer actually exists (previously
   silently accepted a nonexistent layer ID with no error at all).

4. **NEW: `Instance.LayerIds` wire-shape bug** — `instancesToJSON` emitted a
   singular `"LayerId": "<string>"` field. The real `types.Instance` has no
   such member — only a plural `LayerIds []string`. A real
   `aws-sdk-go-v2` client's `Instance.LayerIds` field would therefore never
   have populated from this backend's `DescribeInstances` response (the
   client silently ignores the unknown `LayerId` key). Fixed by wrapping
   this backend's single-layer-per-instance internal model into a one- or
   zero-element `LayerIds` array on the wire.

5. **NEW: invented `App.Arn` / `Volume.StackId` wire fields** — `appsToJSON`
   emitted an `Arn` field; the real `types.App` has no `Arn` member (apps are
   not independently ARN-addressable in real OpsWorks). `volumesToJSON`
   emitted a `StackId` field; the real `types.Volume` has no `StackId`
   member either. Both removed from the wire (the internal `App.Arn` /
   `Volume.StackID` Go struct fields are kept for internal bookkeeping —
   `Volume.StackID` now also powers `DescribeVolumes`' new `StackId` filter
   — just no longer serialized).

6. **NEW: `TagResource`/`UntagResource`/`ListTags` accepted non-taggable
   ARNs** — the real API's `TagResourceInput`/`UntagResourceInput`/
   `ListTagsInput` doc comments all say "The stack or layer's Amazon
   Resource Number (ARN)" — apps and instances are not independently
   taggable resources on the real API. This backend's `resourceExists`
   previously also matched `:instance/` and `:app/` ARNs, silently allowing
   tagging operations AWS does not support. Restricted to `:stack/`/`:layer/`
   only; tagging an instance or app ARN now returns
   `ResourceNotFoundException`, same as tagging any other nonexistent/
   unsupported resource.

7. **NEW: missing `DescribeVolumes` `StackId` filter** — the real
   `DescribeVolumesInput` supports filtering by `StackId` in addition to
   `InstanceId`/`RaidArrayId`/`VolumeIds`; this backend's `DescribeVolumes`
   signature didn't even accept a stack ID. Added (the `volumesByStack`
   index already existed for `deleteStackAssociations`, so this reused
   existing infrastructure).

8. **NEW: `AssignInstance`/`AssignVolume` cross-stack + existence checks
   (closes the previously-known "deferred" item)** — `AssignInstance`
   previously accepted any `layerIDs[0]` value, including a nonexistent
   layer ID or one belonging to an unrelated stack, with no validation at
   all. `AssignVolume` accepted any instance regardless of which stack it
   belonged to. Both now verify the target exists and belongs to the same
   stack (`AssignInstance` returns `ResourceNotFoundException` for a
   nonexistent layer, `ValidationException` for a cross-stack one;
   `AssignVolume` returns `ValidationException` for a cross-stack instance).
   `UnassignInstance` takes no layer parameter, so there was nothing to
   cross-validate there.

9. **Hygiene: removed dead/invalid status constants** — `instanceStatusStarting`
   (`"starting"`) and `instanceStatusStopping` (`"stopping"`) were unused
   after a previous pass's state-machine fix (nothing ever sets an instance
   to either status anymore) and `"starting"` was never a valid AWS
   OpsWorks instance-status value to begin with (see the real
   `types.Instance.Status` doc comment's enum list). `deploymentStatusRunning`
   was likewise dead (constant defined, never referenced) since
   `CreateDeployment` commits synchronously to `deploymentStatusSuccessful`.
   All three deleted rather than left as inert dead code.

None of these required a `go.mod`/`go.sum` change, a new goroutine/timer, or
touching `cli.go`. All changes stayed within `services/opsworks/`.

## Live re-audit (2026-08-08, gopherstack-vjj2)

The 2026-07-23 pass above (and its `A` grade) was performed while opsworks had
no `Provider{}` entry in `cli.go` — see the Registration note — so nothing in
it had ever been exercised by an actual HTTP request. This pass drove real
requests through a locally built+running server (`bin/gopherstack serve
--persist`, via `.claude/skills/run-gopherstack/driver.sh`) with raw `curl`
setting `X-Amz-Target: OpsWorks_20130218.<Op>` and
`Content-Type: application/x-amz-json-1.1`, since the AWS CLI's bundled
botocore has no `opsworks` service model at all (deprecated service, dropped
from newer botocore data) and can't be used here.

**All 73 ops in `GetSupportedOperations()` were called live and dispatched
correctly** — `buildOps()`'s map and the supported-ops list stayed in sync
(as `sdk_completeness_test.go` already asserted), and no op was found present
in one but missing from the other; there is no "unreachable via dispatch"
finding here; every failure found was a request/response-shape bug in an
otherwise-reachable op, not a routing gap.

Error mapping was checked at the wire, not just "an error occurred": a
missing-required-field `CreateStack` and a bad `Type` enum on `CreateLayer`
both came back `400 ValidationException`; `DescribeStacks`/`GetHostnameSuggestion`
on a nonexistent ID came back `404 ResourceNotFoundException`; an unrecognized
`X-Amz-Target` action came back `400 ValidationException` (not 501) with a
descriptive message — all correct per the real SDK's
`awsAwsjson11_deserializeOpError*` functions (confirmed by reading
`deserializers.go` from the fetched-into-scratch-module SDK source, not
assumed).

Persistence was checked through the real snapshot path, not `Snapshot()`
called directly: started the server with `--persist`, created a stack +
layer + a stack tag, stopped the server (triggers `cli.go`'s
`defer persistManager.SaveAll(ctx)` on SIGTERM), confirmed
`$GOPHERSTACK_DATA_DIR/OpsWorks` was written, restarted, and confirmed
`DescribeStacks`/`DescribeLayers`/`ListTags` returned the same stack, layer,
and tag — a real restart round-trip, not a unit-test `Restore(Snapshot())`
call.

Two real bugs were found this way and fixed (both proven with a failing test
against the pre-fix code in a `git worktree`, per this task's verification
requirement, before being fixed):

1. **`GetHostnameSuggestion` decoded the wrong field and would fail for
   every real client.** The real `GetHostnameSuggestionInput` (confirmed in
   `api_op_GetHostnameSuggestion.go`) has exactly one member, `LayerId`
   (required) — there is no `StackId` on this request at all. This backend's
   handler decoded a `StackId` from the body and its backend method
   `GetHostnameSuggestion(stackID, _ string)` looked up by that `stackID`
   while silently discarding the `layerID` argument entirely. A real SDK
   client only ever sends `LayerId`, so `stackID` would always be empty and
   the call would always fail with `ResourceNotFoundException` — this op was
   functionally broken for every real caller, and the previous pass's
   `PARITY.md` doesn't mention it at all (missed entirely, despite being in
   `GetSupportedOperations()`). Fixed: `StorageBackend.GetHostnameSuggestion`
   now takes only `layerID`, looks it up against `b.layers`, and the handler
   no longer decodes a `StackId`. The response now also echoes back
   `LayerId`, matching the real `GetHostnameSuggestionOutput`'s
   `Hostname`/`LayerId` pair (previously only `Hostname` was returned).
   `stacks_test.go`'s `TestGetHostnameSuggestion` sent only `StackId` and so
   was accidentally testing the bug's contract rather than the real one; it
   now creates a layer and sends `LayerId`, and a copy of the fixed test run
   against the pre-fix code (via `git worktree`) reproduced the live 404.

2. **`DescribeEcsClusters` emitted an invented `Status` field.** The real
   `types.EcsCluster` (confirmed in `types/types.go`) has exactly
   `EcsClusterArn`/`EcsClusterName`/`StackId`/`RegisteredAt` — no `Status`
   member. `ecsClustersToJSON` was serializing this backend's internal
   `storedEcsCluster.Status` (always `"registered"`) onto the wire anyway.
   Removed from the JSON output; the internal field is kept for bookkeeping
   only, matching how `App.Arn`/`Volume.StackId` were handled in the prior
   pass. `ecs_clusters_test.go` gained an `assert.NotContains(t, c, "Status")`
   assertion, confirmed failing against the pre-fix code in a worktree before
   the fix.

Every other response struct returned by a live call in this pass (`Stack`,
`Layer`, `Instance`, `App`, `Deployment`, `Command`, `Volume`, `ElasticIp`,
`ElasticLoadBalancer`, `RdsDbInstance`, `Permission`, `UserProfile`,
`StackSummary`/`InstancesCount`, `TimeBasedAutoScalingConfiguration`,
`LoadBasedAutoScalingConfiguration`, `TemporaryCredential`, `AgentVersion`,
`OperatingSystem`, `DescribeStackProvisioningParametersOutput`) was
field-diffed against the same fetched SDK source and found to emit only a
subset of the real fields — no further invented fields found.

**Why the grade is `B`, not `A`:** every op now proven reachable, correctly
routed, correctly error-mapped at the HTTP boundary, and round-tripping
through real persistence — the specific things this task was scoped to
check. But there is still no `test/integration/*_parity_test.go` suite for
opsworks (blocked on the SDK not being a `go.mod` dependency — see the SDK
availability note above), and the `gopherstack-parity-audit` skill's own
rubric requires that suite for `A`. The prior `A` was ungrounded on two
counts: it predates ever handling a live request, and (independent of that)
it was never actually backed by the integration-suite proof the rubric
requires. `B` reflects genuine, now-verified accuracy without overclaiming
the untouched integration-test gap.

## gopherstack-6flj wrapper-key sweep (2026-08-15)

Swept fresh, per gopherstack-t0gq's instruction: a prior session on this same
service was killed mid-verification by an API session limit and stashed
(`stash@{0}`) rather than committed, since its work built but failed
`TestElasticIps/RegisterElasticIp_without_StackId_returns_400` (expected 400,
got 200) with nothing hand-reverted or verified. That stash was read
read-only as a hint, never popped/applied. Independently re-derived and
re-verified all findings below against the real SDK from scratch.

**The ambiguous test, resolved:** `RegisterElasticIp_without_StackId_returns_400`
did not exist at `HEAD` (`git show HEAD:services/opsworks/elastic_ips_test.go
| grep StackId` — no match), so it was not a pre-existing test the stashed
session broke. It was a *new* test that correctly caught a real,
previously-missing required-field validation: `RegisterElasticIpInput` has
`ElasticIp` and `StackId` both `"This member is required"`, and no `Region`
member at all (confirmed against
`aws-sdk-go-v2/service/opsworks@v1.31.0`'s `api_op_RegisterElasticIp.go`).
The stashed backend accepted `StackId` as a new parameter but never validated
it was non-empty, so its own new test correctly failed. Verdict: **(b)**, not
(a). This closes gopherstack-t0gq for opsworks.

**SDK availability:** `aws-sdk-go-v2/service/opsworks@v1.31.0` is present in
the local module cache (`$(go env GOMODCACHE)`) but is **not** a `go.mod`
dependency of this repo — confirmed via `grep opsworks go.mod go.sum`
(no hits). All wire-shape verification below reads the cached module source
directly, per the SDK-availability note above; no `go get` or `go.mod` edit
was made. There is still no real-SDK-client test for this service (0 before
and after this pass) for the same reason.

**Protocol:** `awsAwsjson11` exclusively (confirmed via `serializers.go`'s
`awsAwsjson11_serializeOp*` function names and every deserializer's own
prefix). Case-sensitive plain Go string `switch key { case "Xxx": ... }` on
decoded JSON keys (confirmed reading
`awsAwsjson11_deserializeDocumentElasticIp`, `...DescribeElasticLoadBalancers`
and others), not `smithyxml`'s `EqualFold`. All `EqualFold` hits in this SDK
version (`grep -n EqualFold deserializers.go`) are in the `errorCode`
matching branches only (`case strings.EqualFold("ResourceNotFoundException",
errorCode)` etc.), never in a body-field-key switch. No second client:
`go.mod`/`go.sum` have zero `opsworks` references, and this package's own
`sdk_completeness_test.go` documents the same absence.

**Router:** single top-level `X-Amz-Target` prefix match
(`RouteMatcher`/`ExtractOperation`), one flat `buildOps()` dispatch map —
no second-layer router to desync from `GetSupportedOperations()` the way
elasticsearch's two-level dispatch could (`sdk_completeness_test.go` already
asserts the two lists match exactly). Not a source of bugs here.

**Phantom ops:** none. `GetSupportedOperations()`'s 74 op names were diffed
1:1 against every `api_op_*.go` file in the pinned module — no gopherstack op
absent from the real SDK, and no real op absent from gopherstack.

**3 real bugs found and fixed**, all layer-2/5 (discarded input + missing
validation + fabricated member), none previously flagged in this file's
`gaps`/`deferred`:

1. **RegisterElasticIp** (`elastic_ips.go`, `handler_elastic_ips.go`,
   `interfaces.go`): fabricated `Region` request member (not real; region is
   always the stack's own) replaced with the real, required `StackId`;
   `StackId`'s required-ness is now validated (`ValidationException` on
   empty, matching this service's established validate-then-existence-check
   pattern from `RegisterInstance`/`RegisterVolume`/`RegisterRdsDBInstance`).
   `StackId` is captured internally (`storedElasticIP`/`ElasticIP.StackID`)
   but deliberately **not** put on the wire — real `types.ElasticIp` has no
   `StackId` member.

2. **DescribeElasticIps** (`elastic_ips.go`, `handler_elastic_ips.go`,
   `interfaces.go`): real `StackId` filter member (confirmed
   `api_op_DescribeElasticIps.go`) was entirely discarded — every call
   ignored it and returned every IP regardless of stack. Now honored.

3. **DescribeElasticLoadBalancers** (`elastic_load_balancers.go`,
   `handler_elastic_load_balancers.go`, `interfaces.go`): real, plural
   `LayerIds` filter member (confirmed
   `api_op_DescribeElasticLoadBalancers.go`) was truncated to its first
   element by the handler and then the backend's own parameter was literally
   named `_` — never read at all. Now filters against the full list via
   `slices.Contains`.

**1 more real bug, same pass, different op family:**

4. **DescribeStackProvisioningParameters** (`stacks.go`,
   `handler_stacks.go`, `interfaces.go`): the real, dedicated top-level
   `AgentInstallerUrl` member was correctly emitted, but its value was *also*
   duplicated under a fabricated `"AgentInstallerUrl"` key inside the
   free-form `Parameters` map — a key no real response ever puts there.
   `Parameters` now returns empty (honest — this backend tracks none of
   AWS's real internal agent-bootstrap keys) rather than an invented one.

**Sibling/per-item field check (layer 2):** every `List`/`Describe`/`Get` op
in `GetSupportedOperations()` (24 of 74 ops) had its top-level wrapper key
diffed against the real deserializer's own top-level case list — all correct
(`EcsClusters`, `Apps`, `Commands`, `Deployments`, `ElasticIps`,
`ElasticLoadBalancers`, `Instances`, `Layers`,
`LoadBasedAutoScalingConfigurations`, `MyUserProfile`->`UserProfile`,
`OperatingSystems`, `Permissions`, `RaidArrays`, `RdsDbInstances`,
`ServiceErrors`, `AgentInstallerUrl`+`Parameters`, `StackSummary`, `Stacks`,
`TimeBasedAutoScalingConfigurations`, `UserProfiles`, `Volumes`, `Hostname`+
`LayerId`, `Tags`). Every per-item `*ToJSON` conversion function (21 of them)
was also field-diffed against its real deserializer's `case "Xxx":` list —
every field gopherstack *does* emit uses the real key name. The large
remaining gaps (most of `App`/`Layer`/`Instance`/`Stack`/`Volume`/`Deployment`'s
optional surface, `ElasticLoadBalancer`'s VPC-ish fields) are all structural
— the domain model genuinely doesn't track the value — and match this file's
existing `deferred`/`gaps` entries (or were newly added to them this pass,
see `ElasticLoadBalancer`'s new gap above); none are a "value already held
but never emitted" bug.

**Persistence check:** `storedElasticIP` (`models.go`) doubles as the
snapshot/restore persistence DTO (`store.Table[storedElasticIP]`, see
`persistence.go`). The new `StackID` field was added there (not retagged —
existing fields/tags untouched), so old snapshots restore unchanged with
`StackID` defaulting to `""`; no version bump needed. No `json:"-"` was
applied to anything in this pass.

**Tests:** `TestElasticIps/RegisterElasticIp_without_StackId_returns_400`
(new), `TestElasticIps/DescribeElasticIps_filters_by_StackId` (new),
`TestElasticLoadBalancers/DescribeElasticLoadBalancers_filters_by_LayerIds`
(new), plus an assertion added to
`TestDescribeStackProvisioningParameters` guarding against the fabricated
`Parameters.AgentInstallerUrl` key. All 4 fixes hand-reverted individually
(no git-mutating commands used — the harness's own file edits stood in for
`git stash`) and confirmed to fail with the exact predicted symptom before
being restored byte-identical: (1) `StackId` validation removed →
`RegisterElasticIp_without_StackId_returns_400` got 404 instead of 400 (falls
through to `ErrStackNotFound` instead of `ErrValidation` — different wrong
code than the stashed session saw, but still not the expected 400,
confirming the validation gap); (2) `StackId` filter removed from
`DescribeElasticIps` → 2 IPs returned instead of 1; (3) `LayerIds` filter
removed from `DescribeElasticLoadBalancers` → 2 ELBs returned instead of 1;
(4) fabricated `Parameters.AgentInstallerUrl` re-added → assertion failed as
predicted.

**Real-client test ratio:** 0 before and after this pass (SDK not a `go.mod`
dependency, see above — matches this repo's documented exception for
services with no pinned client). All new tests are raw-body/`doTarget`-style,
consistent with every other test in this package.

**Gates:** scoped `go build`/`go vet ./services/opsworks/...` clean; full
`go build ./...`/`go vet ./...` clean for this package (interface signature
changes propagate; `directoryservice` was a live, separately-owned sibling
mid-edit throughout this session — confirmed via repeated `git status`, never
touched, and its transient build breaks were its own, not caused by this
pass); `go test -race -count=1 ./services/opsworks/...` and
`./pkgs/...` green; `go fix -diff` clean (no diff); `golangci-lint run
./services/opsworks/...` 0 issues (1 `golines` line-length finding fixed by
hand during the pass); 0 `cyclop`/`gocyclo`/`gocognit`/`funlen` nolints
(grep-confirmed, none added or pre-existing).

No subagents used. No git-mutating commands run — orchestrator must
commit/push. `git status` re-checked before every edit batch; only
`services/opsworks/*` files touched by this pass.

## 2026-08-20 — gopherstack-jqh2 pass 4: SDK route table test added

Re-extracted all 74 OpsWorks ops from `opsworks@v1.31.0` serializers.go
(`X-Amz-Target: OpsWorks_20130218.<Op>` in each
`awsAwsjson11_serializeOp<Op>.HandleSerialize`) and cross-diffed against
both existing op-name tables: `GetSupportedOperations()`'s literal slice and
`buildOps()`'s dispatch map. All three (SDK, GetSupportedOperations,
buildOps) match exactly, 74/74, zero drift in any pairing — no shape-3
parallel-table bug. Added `handler_sdk_route_table_test.go`
(`TestExtractOperation_SDKRouteTable`), which builds a real X-Amz-Target
request per op and drives it through the real `Handler()`, asserting it
does not fall through to the "unknown action" dispatch-miss branch
(handler.go:262-263). No stale PARITY.md entries found.

## 2026-08-23 — gopherstack-n3zi: first typed-client tests (10/74 ops)

Chosen as the demonstration service for gopherstack-n3zi's measurement pass
(a new `cmd/clientcoverage` AST census of every `_test.go` under
`test/integration/` and `services/` that constructs a real
`aws-sdk-go-v2` client and calls an op on it) because it measured 0/74 ops
ever invoked by a typed client anywhere in the repo, on the census's most
trustworthy (`direct`) opcensus resolution tier -- not the raw top-ranked
gap, several of which (`bedrock`, `redshift`) turned out to have their own
`opcensus` op-list defects (see gopherstack-n3zi's notes) that make their
numbers unreliable.

Added `sdk_roundtrip_helper_test.go` (`newRoundTripClient`/`newTestClient`,
the same `httptest.Server` + `pkgs/service` router pattern used by
`appmesh`/`grafana`) and `sdk_roundtrip_test.go`: three round-trip tests
covering `CreateStack`, `DescribeStacks`, `UpdateStack`, `DeleteStack`,
`TagResource`, `ListTags`, `UntagResource`, `CreateLayer`, `DescribeLayers`,
`DeleteLayer` -- each asserting real field values read back through the
real SDK deserializer, not just a 200. All 10 passed against the existing
handler/backend with no wire-shape bug found this time; the value here is
closing the blindness, not a bug this pass happened to catch.

This also required adding `aws-sdk-go-v2/service/opsworks` as a direct
`go.mod` dependency (`go get .../service/opsworks@v1.31.0`) -- it was
previously only present transitively in the module cache, referenced by
comment-only audits above, never imported. Since OpsWorks is AWS-deprecated,
every SDK call now carries an `SA1019` staticcheck notice; exempted per-file
in `.golangci.yml` (same precedent as `iotanalytics/handler_create_tags_test.go`)
rather than sprinkling inline nolints.

Remaining 64 ops (instances, apps, deployments, permissions, ECS/RDS/volume
registration, load-based/time-based auto scaling, commands, agent versions,
user profiles, service errors) are still only exercised via `doTarget`-style
raw-body tests -- this pass does not change `overall: B`; the "no SDK-driven
suite" line in that block predates it and is now a partial rather than
total gap. Not attempted further: this was a coverage-measurement
demonstration, not a full remediation pass (see gopherstack-n3zi).

Gates: `go build ./...`, `go vet ./services/opsworks/...`, `gofmt -l` clean;
`go test -race ./services/opsworks/...` green; `golangci-lint run
./services/opsworks/...` 0 issues after fixing (govet shadow, tparallel,
golines, unparam; SA1019 exempted per above). No
`cyclop`/`gocyclo`/`gocognit`/`funlen` nolints added.

## 2026-08-29 pass: campaign class audit (constraining parameter never honoured)

Measured 23 Describe/List/Get operations against the pinned SDK
(opsworks@v1.31.0). Unlike most services in this campaign, opsworks
constrains by ID-list ("only describe these AppIds/InstanceIds/...") rather
than a `Filters` array, and every ID-list parameter across all 23 ops was
already correctly honoured (verified: DescribeApps, DescribeCommands,
DescribeDeployments, DescribeInstances, DescribeLayers,
DescribeElasticLoadBalancers, DescribePermissions, DescribeUserProfiles all
filter/scope correctly; DescribeLoadBasedAutoScaling/
DescribeTimeBasedAutoScaling's LayerIds/InstanceIds are the ID list to
describe, not an optional filter, and are used as such). Two real findings:

- **DescribeEcsClusters**: declares `MaxResults`/`NextToken`
  (api_op_DescribeEcsClusters.go) but `handleDescribeEcsClusters` never read
  either field from the request body -- always returned every cluster.
  Fixed with `pkgs/page.New`, defaulting to 100 (the real doc comment
  specifies no default for this deprecated op).
- **DescribeVolumes**: `RaidArrayId` was parsed from the request body and
  passed to the backend method, which discarded it via a blank identifier
  (`_ string`) -- a documented, deliberate no-op, since this backend never
  models RAID arrays (`DescribeRaidArrays` always returns empty; no
  `CreateRaidArray` operation exists in the real API either). Fixed by
  honouring the parameter rather than ignoring it: a non-empty `RaidArrayId`
  now excludes every volume (correct, since no volume ever carries that
  association) instead of silently returning every volume in the
  stack/instance regardless of the constraint.

Tests: `list_filter_params_test.go`, driven through the real SDK client
(`newTestClient`) -- `TestDescribeEcsClusters_Pagination` (two-page
round-trip, cursor carries the remainder),
`TestDescribeVolumes_RaidArrayIDExcludesAll`. Both fail against pre-fix code
(confirmed by reverting handler_ecs_clusters.go/volumes.go only).

FIXED 2026-08-30 (wrapper-key-sweep), two more real findings in this same
family, both missed by the pass above:

- **DescribeAgentVersions**: declares a `ConfigurationManager`
  ({Name,Version}) filter (api_op_DescribeAgentVersions.go, wire key
  "ConfigurationManager") that `handleDescribeAgentVersions` never read at
  all -- always returned the full static 2-entry catalog regardless of the
  filter. Now filtered by Name/Version against the catalog.
- **DescribeEcsClusters** (unfiltered path, no StackId): the MaxResults/
  NextToken fix above paginates via `pkgs/page.New`, whose own doc comment
  requires "a fully sorted slice" because its cursor is a raw positional
  index -- but the backend fed it `b.ecsClusters.All()` directly, and
  `pkgs/store.Table.All`'s doc comment says its order is Go map order,
  unspecified from one call to the next. Walking every page with the
  returned NextToken could silently drop or duplicate clusters (confirmed:
  a 25-cluster/page-size-5 walk dropped/duplicated clusters on 5/5 runs
  pre-fix). Fixed by sorting the result by EcsClusterArn (the table's unique
  primary key, so no tie-break record-id is needed) before pagination. The
  StackId-filtered path was unaffected -- it already goes through
  ecsClustersByStack, an append-ordered secondary index.

Test: `TestDescribeAgentVersions_ConfigurationManagerFilterRealClient`,
`TestDescribeEcsClusters_PaginationStableOrderRealClient`
(wire_field_fixes_test.go), both driven through the real SDK client and
confirmed to fail against pre-fix code (the pagination-order test failed on
5/5 runs pre-fix; passed 8/8 post-fix).

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

Same defect and fix as the census in `cmd/reqfielddiff`/`cmd/reqfieldscan`
(ef0eef041, appsync e2643a6dd). This package's `ElasticIp`/`ElasticIP` and `RdsDb`/`RdsDB` acronym casing
gives it 9 op/handler pairs needing the ambiguous fold, 9 of them
genuine collisions between an exported backend method and the real
unexported handler: `AssociateElasticIp`, `DeregisterElasticIp`, `DeregisterRdsDbInstance`, `DescribeRdsDbInstances`, `DisassociateElasticIp`, `RegisterElasticIp`, `RegisterRdsDbInstance`, `UpdateElasticIp`, `UpdateRdsDbInstance`.

Verified directly rather than assumed: ran the unpatched tool from
`ef0eef041~1` five times and diffed against the fixed tool at HEAD, for
both `cmd/reqfieldscan` and `cmd/reqfielddiff`. Both were byte-identical
across all 5 old runs and HEAD (74 SDK operations compared) -- the
determinism defect never flipped a finding here, because the resolution
that actually mattered (this package's dispatch-table union) already
carried the correct field set regardless of which fold candidate won.

Verdict: confirmed zero damage, not merely predicted.

## 2026-09-04 — gopherstack-2rx: delete-precondition sweep

Read the full doc comments (not grepped) for every `Delete*` op against
`aws-sdk-go-v2/service/opsworks@v1.31.0`: `DeleteStack`, `DeleteLayer`,
`DeleteInstance`, `DeleteApp`, `DeleteUserProfile`. Per-op error-code
switches confirmed (`ResourceNotFoundException`/`ValidationException` only,
for all five).

**2 real, previously-unfixed bugs found and fixed** (neither had a
pre-existing regression test locking in a contrary behavior):

1. **`DeleteInstance`** (`instances.go`): `api_op_DeleteInstance.go` --
   "You must stop an instance before you can delete it." The backend
   deleted any instance regardless of `Status`. Now looks the instance up
   first and returns `ValidationException` unless `Status == "stopped"`.
   Instances default to `stopped` on both `CreateInstance` and
   `RegisterInstance` (see their doc comments), so this only bites an
   instance a caller `StartInstance`'d and never stopped again. Test:
   `TestInstance/DeleteInstance_on_an_online_instance_returns_ValidationException`
   (`instances_test.go`) -- hand-reverted (`DeleteInstance` back to
   unconditional `b.instances.Delete`) and confirmed it fails with 200
   instead of 400 pre-fix; restored and confirmed byte-identical to the fix
   via `grep -A16 DeleteInstance instances.go`.

2. **`DeleteLayer`** (`layers.go`): `api_op_DeleteLayer.go` -- "You must
   first stop and then delete all associated instances or unassign
   registered instances." The backend deleted the layer row unconditionally,
   with no check for instances still referencing it. Since this backend
   models an instance's layer membership as a single `storedInstance.LayerID`
   field (not a table row), the pre-fix behavior was a genuine ghost-reference
   bug: an instance created via `CreateInstance` or assigned via
   `AssignInstance` into a now-deleted layer kept `LayerID` pointing at a
   nonexistent layer, which `DescribeInstances`' `LayerIds` filter and wire
   output would still surface. Now looks the layer up first (for its
   `StackID`, to scope the scan via the existing `instancesByStack` index)
   and returns `ValidationException` if any instance's `LayerID` still
   matches. Test:
   `TestLayer/DeleteLayer_with_an_associated_instance_returns_ValidationException`
   (`layers_test.go`) -- hand-reverted and confirmed it fails with 200
   instead of 400 pre-fix (instance count after the failed delete attempt
   also asserted at 1, confirming no partial mutation); restored and
   confirmed byte-identical via the same grep-diff method.

Neither fix touches `deleteStackResources`/`deleteStackAssociations`
(`stacks.go`) -- `DeleteStack`'s cascade deletes rows directly via
`b.layers.Delete`/`b.instances.Delete` on the store tables, not through the
now-precondition-guarded `DeleteLayer`/`DeleteInstance` backend methods, so
`TestDeleteStackCascade` (`stacks_test.go`) is unaffected and still passes.

**`DeleteStack` precondition -- considered, deliberately NOT changed.**
`api_op_DeleteStack.go` states the same kind of precondition: "You must
first delete all instances, layers, and apps or deregister registered
instances." Read literally and consistently with the `DeleteInstance`/
`DeleteLayer` fixes above, a strict reading would make a non-empty
`DeleteStack` return `ValidationException` instead of cascading. This
backend instead cascades (`deleteStackResources`/`deleteStackAssociations`),
and that behavior is **not** an oversight -- it is locked in by an existing,
passing regression test (`TestDeleteStackCascade`, `stacks_test.go`,
predating this pass) that asserts a 200 and full cascade cleanup. Reversing
it now would silently invalidate that prior, deliberate design decision
based on a doc-comment reading alone, which this task's own rules caution
against ("A new delete precondition can break a teardown path"). Checked
`services/cloudformation` for any coupling that reversing it could break
(`grep -rln opsworks services/cloudformation/` -- zero hits), so that
specific historical risk doesn't apply here, but the tension between the
SDK's literal wording and the established cascade-on-delete convention used
throughout this repo (see e.g. `vpclattice`, `workmail`, `amplify` commit
history) is a genuine design-decision fork, not a "SDK is silent" case and
not an unambiguous bug either -- flagging as **cannot be resolved without a
product decision** rather than guessing. Filed as a follow-up rather than
changed unilaterally.

**`DeleteApp`/`DeleteUserProfile`**: doc comments carry no delete
precondition at all (`api_op_DeleteApp.go`: "Deletes a specified app.";
`api_op_DeleteUserProfile.go`: "Deletes a user profile."). No SDK basis to
add one; NOT CHECKED for ghost-row leaks in `permissions`/other tables
keyed by `IamUserArn` (SDK doesn't document a cascade contract there
either, so no fix attempted -- would need a product decision, same as
`DeleteStack` above).

**Gates**: `GOTOOLCHAIN=go1.26.6 go build ./...` clean;
`GOTOOLCHAIN=go1.26.6 go test -race -count=1 ./services/opsworks/...` and
`./services/cloudformation/...` (dependent per this task's instructions)
both green; `GOTOOLCHAIN=go1.26.6 golangci-lint run ./services/opsworks/...`
`0 issues.`; no `cyclop`/`gocyclo`/`gocognit`/`funlen` nolints added.

No subagents used. No git-mutating commands run -- orchestrator must
commit/push. Only `services/opsworks/*` files touched (test + backend
methods + this file); `services/kms` (concurrently edited by another agent
per this task's instructions) never touched.

## 2026-09-07 — gopherstack-bael: DeleteStack precondition resolved

The 2026-09-04 pass above (gopherstack-2rx) flagged `DeleteStack`'s cascade
vs. its doc comment's stated precondition as a design-decision fork needing
a product call, and filed it as a follow-up rather than changing it
unilaterally. This is that follow-up.

Re-read `api_op_DeleteStack.go` verbatim: "You must first delete all
instances, layers, and apps or deregister registered instances." This is a
precondition, not mere description, and it matches the family: `DeleteLayer`
("You must first stop and then delete all associated instances or unassign
registered instances.") and `DeleteInstance` ("You must stop an instance
before you can delete it.") both state preconditions and were already fixed
to enforce them (gopherstack-2rx, above). `DeleteApp` and
`DeleteUserProfile` -- the two ops with no children of their own -- carry no
precondition at all. The family is consistent: every op with children
documents a precondition; leaf ops don't. That consistency, plus the
verbatim wording, settles it -- cascading was the bug; the doc comment (both
AWS's and this backend's now-former "deletes a stack and all its child
resources") was correct and the code was wrong.

Searched `services/opsworks/*_test.go` for every `DeleteStack` call before
changing anything: `TestDeleteStackCascade` (`stacks_test.go`) was the only
one that ever gave a stack live children before deleting it. The other
three sites (`handler_test.go`'s nonexistent-ID case,
`stacks_test.go`'s "DeleteStack removes stack" case, and
`sdk_roundtrip_test.go`'s `TestStackLifecycle_RoundTrip`) all delete a
childless stack, so the new precondition doesn't touch them. Also checked
`test/integration/` -- no opsworks tests exist there. One existing test
affected, and it was the one this issue named.

`TestDeleteStackCascade` (`stacks_test.go`) rewritten from a single case
asserting cascade-and-succeed into a combined refusal case (layer+instance+
app all present -- refused, zero mutation), three isolated refusal cases
(see "Coverage gap closed" below), and a success case proving deletion
works once all three are individually removed. Hand-reverted the guard in
`DeleteStack` (`stacks.go`) back to unconditional `b.deleteStackChildren`
and reran: the combined refusal subtest failed (400 expected, got 200; body
`{}` not containing `ValidationException`; all four count assertions failed
1 actual 0) while the success subtest still passed (it doesn't depend on
the guard) -- confirming the refusal subtest is not hollow. Restored and
confirmed byte-identical via `git diff services/opsworks/stacks.go`.

Fix (`stacks.go`, `DeleteStack`): added a guard checking
`instancesByStack`/`layersByStack`/`appsByStack` for the target `stackID`
before calling `deleteStackChildren`, returning `ErrValidation` if any is
non-empty. Deployments, permissions, volumes, RDS instances, and ECS
clusters carry no documented precondition and still cascade once the guard
clears -- there is no way to delete them individually via a documented
opsworks-DeleteX contract with the same "must delete first" wording, so
cascading those remains the only faithful reading.

**Coverage gap closed (same day).** The initial combined-fixture case
(layer+instance+app together) did not individually pin each of the guard's
three `||` clauses: dropping the `appsByStack` clause alone still passed
every test, because the other two clauses fired on the same fixture. Added
three isolated cases -- stack with only a layer, only an instance (via
`RegisterInstance`, which unlike `CreateInstance` takes no `LayerId` and so
is the only way to construct a layer-free instance in this backend), and
only an app -- each dropping to 200/no-`ValidationException` if its
matching clause is removed. Verified per clause: dropped
`instancesByStack` alone -- only `..._with_only_an_instance_..._` failed
(400->200, `{}` without `ValidationException`, instance count 1->0), other
four subtests still passed. Dropped `layersByStack` alone -- only
`..._with_only_a_layer_..._` failed the same way. Dropped `appsByStack`
alone -- only `..._with_only_an_app_..._` failed the same way. All three
dropped versions still compiled (`go build ./services/opsworks/...`).
Restored the full three-clause guard and confirmed `git diff
services/opsworks/stacks.go` byte-identical to the fix above.

**Gates**: `GOTOOLCHAIN=go1.26.6 go test -race ./services/opsworks/...`
green; `GOTOOLCHAIN=go1.26.6 golangci-lint run services/opsworks/...`
`0 issues.`. No subagents used. No git-mutating commands run --
orchestrator must commit/push. Only `services/opsworks/*` files touched;
`services/s3control/` never read or written.
