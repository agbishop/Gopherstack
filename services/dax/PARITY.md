---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: dax
sdk_module: aws-sdk-go-v2/service/dax@v1.32.4   # awsjson1.1 protocol, target prefix AmazonDAXV3.
last_audit_commit: fa462f07c   # refreshed 2026-09-07 -- current HEAD at write time
last_audit_date: 2026-09-07
overall: A            # 2026-07-24: follow-up pass: closed all 3 previously-known gaps, killed both banned nolints
                      # 2026-07-31: pkgs/sdkcheck reverse check found ResetParameterGroup wrongly advertised/documented as a real SDK op (it isn't -- see its ops-block note); corrected, route left wired as internal test scaffolding. Grade held at A: unreachable by real traffic either way, since DAX dispatches purely by X-Amz-Target and no real client can send this target.
                      # 2026-08-10: control-plane sweep (gopherstack-mmqd). Fixed state-mutated-before-validation in UpdateCluster and UpdateParameterGroup, a wrong error fault code on 6 required-field checks, a fabricated Tags field on the Cluster wire response, 3 unvalidated @required fields (TagResource.Tags, UntagResource.TagKeys, UpdateParameterGroup.ParameterNameValues), and a missing per-subnet SupportedNetworkTypes field. See Notes.
                      # 2026-08-20: wrapper-key / nested-shape sweep. Fixed one fabricated SourceType enum value ("NODE") emitted for node-level Events; the real types.SourceType enum has exactly CLUSTER/PARAMETER_GROUP/SUBNET_GROUP. All other wrapper keys, nesting levels, and per-member shapes across all 20 ops verified clean against the pinned SDK. See Notes.
                      # 2026-08-29: write-only-state sweep (gopherstack-6flj/21my), forward+reverse, over clusters/parameter_groups/subnet_groups/tags/events control-plane files plus their handlers. No new bug found -- confirms the 2026-08-20 sweep's coverage still holds; no dax-specific commits landed between the two passes. See Notes.
                      # 2026-09-04: parity-sweep-2026-09-03 campaign. CreateCluster silently accepted an AvailabilityZones list whose length didn't match ReplicationFactor instead of rejecting it. See Notes.
                      # 2026-09-07: errtargetaudit sweep (gopherstack-dkr8). CreateSubnetGroup and UpdateSubnetGroup both typed InvalidParameterValueException for a bad/missing SubnetGroupName -- a code neither op's real deserializeOpError switch declares. Fixed: CreateSubnetGroup no longer format-validates the name at all (SubnetGroupName carries no such constraint in the real API, unlike ClusterName/ParameterGroupName); UpdateSubnetGroup's empty-name check now types SubnetGroupNotFoundFault, matching DeleteSubnetGroup's existing treatment of the identical condition. See Notes.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "IamRoleArn-required check now uses InvalidParameterValueException, not InvalidARNFault -- fixed 2026-08-10; AvailabilityZones length is now validated against ReplicationFactor -- fixed 2026-09-04, see Notes"}
  DescribeClusters: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "no longer mutates Description/PreferredMaintenanceWindow/SecurityGroupIDs before validating ParameterGroupName exists; ClusterName-required check now uses InvalidParameterValueException, not InvalidARNFault -- both fixed 2026-08-10, see Notes"}
  DeleteCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "ClusterName-required check now uses InvalidParameterValueException, not InvalidARNFault -- fixed 2026-08-10, see Notes"}
  IncreaseReplicationFactor: {wire: ok, errors: ok, state: ok, persist: ok, note: "ClusterName-required check now uses InvalidParameterValueException, not InvalidARNFault -- fixed 2026-08-10, see Notes"}
  DecreaseReplicationFactor: {wire: ok, errors: ok, state: ok, persist: ok, note: "ClusterName-required check now uses InvalidParameterValueException, not InvalidARNFault -- fixed 2026-08-10, see Notes"}
  RebootNode: {wire: ok, errors: ok, state: ok, persist: ok, note: async recovery is intentional -- matches real AWS's transient "rebooting" status, see Notes. ClusterName-required check now uses InvalidParameterValueException, not InvalidARNFault -- fixed 2026-08-10.}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "Tags is now enforced as @required (rejects a request that omits the field entirely) -- fixed 2026-08-10, see Notes"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "TagKeys is now enforced as @required -- fixed 2026-08-10, see Notes"}
  ListTags: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeParameterGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "no longer commits earlier-validated entries of a ParameterNameValues batch when a later entry is invalid (validate-then-apply, two passes); ParameterNameValues is now enforced as @required -- both fixed 2026-08-10, see Notes"}
  DeleteParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeParameters: {wire: ok, errors: ok, state: ok, persist: ok, note: Source filter (user/system) now applied -- fixed 2026-07-24, see Notes}
  DescribeDefaultParameters: {wire: ok, errors: ok, state: ok, persist: n/a}
  # ResetParameterGroup is intentionally NOT listed as an advertised SDK op
  # here. 2026-07-31 CORRECTION: the row that used to live at this position
  # ("wire: ok, ...") was inaccurate -- ResetParameterGroup is not a real AWS
  # DAX SDK operation at all (verified against botocore's dax
  # service-2.json: no such action exists in the 2017-04-19 model; the real
  # op list has no reset-to-defaults call for parameter groups). Caught by
  # pkgs/sdkcheck's reverse check (commit 12cfe14d5; gopherstack-vhw2
  # category A). DAX dispatches purely by X-Amz-Target header value through
  # the daxOperations table, so a real client can never send this target and
  # this route was already unreachable by real traffic; it stays wired as
  # internal test scaffolding, unadvertised. See handler.go's comment on the
  # GetSupportedOperations() entry. Same resolution as EMR's
  # ListTagsForResource and CloudFront's
  # GetFunctionAssociations/SetFunctionAssociations.
  CreateSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "each Subnet in the response now carries its own SupportedNetworkTypes field (types.Subnet has one distinct from SubnetGroup's) -- fixed 2026-08-10; no longer rejects a bad/missing SubnetGroupName with InvalidParameterValueException, a code the real op's error set doesn't declare and SubnetGroupName has no documented format constraint for -- fixed 2026-09-07, see Notes"}
  DescribeSubnetGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "empty-name check now types SubnetGroupNotFoundFault, not InvalidParameterValueException (a code the real op's error set doesn't declare), matching DeleteSubnetGroup -- fixed 2026-09-07, see Notes"}
  DeleteSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeEvents: {wire: ok, errors: ok, state: ok, persist: ok, note: "node-level events (RebootNode) now report SourceType CLUSTER, not the fabricated NODE value -- fixed 2026-08-20, see Notes"}
# Families audited as a group (when per-op is impractical):
families:
  cluster-lifecycle: {status: ok, note: "CreateCluster/DescribeClusters/UpdateCluster/DeleteCluster/IncreaseReplicationFactor/DecreaseReplicationFactor/RebootNode all mutate the real store.Table[Cluster], persist via backendSnapshot, and now emit the correct wire shape (Status key, epoch timestamps) -- see gaps for the 5 bugs found and fixed this pass."
  tags: {status: ok, note: "TagResource/UntagResource/ListTags mutate the ARN-keyed tags map and propagate cluster ARNs to Cluster.Tags; quota (50) and key/value length enforcement match AWS constraints. arnExists now recognizes cluster ARNs only (fixed 2026-07-24, see Notes) -- real DAX has no Arn field on ParameterGroup/SubnetGroup, so those were never taggable."}
  parameter-groups: {status: ok, note: "CreateParameterGroup/DescribeParameterGroups/UpdateParameterGroup/DeleteParameterGroup/DescribeParameters/DescribeDefaultParameters all real; UpdateParameterGroup correctly cascades pending-reboot + NodeIdsToReboot to dependent clusters; DescribeParameters now honors the request's Source filter (fixed 2026-07-24). ResetParameterGroup is NOT a real DAX op (see its ops-block note, corrected 2026-07-31) -- kept wired as internal test scaffolding only, unreachable by real clients."
  subnet-groups: {status: ok, note: "CreateSubnetGroup/DescribeSubnetGroups/UpdateSubnetGroup/DeleteSubnetGroup real; in-use protection (blocks delete while referenced by a cluster) verified. SupportedNetworkTypes now modeled (always [\"ipv4\"], fixed 2026-07-24, see Notes)."}
  events: {status: ok, note: "DescribeEvents ring buffer (1000 cap) is real; StartTime/EndTime/SourceName/SourceType filtering verified after fixing the epoch-seconds request-parsing bug."}
  dataplane: {status: deferred, note: "Binary DAX client protocol (services/dax/dataplane/) is a separate, extensively self-tested subsystem (936-line dataplane_integration_test.go + dataplane/*_test.go) not covered by this control-plane wire-shape sweep. Not audited this pass -- different reference material (aws-dax-go's binary encoding, not aws-sdk-go-v2/service/dax) would be needed."}
gaps: []                  # known divergences NOT fixed — link bd issue ids; all 3 prior gaps closed this pass
items_still_open:
  - "InsufficientClusterCapacityFault / ServiceLinkedRoleNotFoundFault (types.InsufficientClusterCapacityFault, types.ServiceLinkedRoleNotFoundFault) are real CreateCluster error types not modeled. Reason: both are account/infrastructure-state faults (missing DAX service-linked role; opportunistic hardware capacity shortage) with no deterministic, request-shape-driven trigger condition -- gopherstack tracks neither IAM service-linked-role state nor a hardware capacity pool. Inventing an arbitrary trigger (e.g. erroring above some ReplicationFactor) would itself be exactly the kind of fabricated, non-AWS-accurate behavior this audit exists to prevent. Left unmodeled; would need a deliberate design decision (e.g. a backend flag simulating SLR presence) before implementing."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - dataplane/ (binary DAX client protocol server, separate from the HTTP control-plane API audited here)
leaks: {status: clean, note: "CreateCluster/DeleteCluster/IncreaseReplicationFactor/DecreaseReplicationFactor/RebootNode each spawn a one-shot 1s-delay goroutine to simulate AWS's async state transition; every goroutine re-acquires b.mu, checks the resource still exists/is in the expected transient state, and exits -- no retry loop, no leaked goroutine. CreateCluster/DeleteCluster short-circuit synchronously under DAX_TEST_SYNC=1 for deterministic tests; Increase/Decrease/RebootNode intentionally do NOT (see Notes) and existing tests (TestRebootNodeRecovery) depend on the async path even under DAX_TEST_SYNC=1. DecreaseReplicationFactor's async goroutine now also clears the transient Cluster.NodeIDsToRemove list it sets (2026-07-24) -- verified no residual state after recovery via TestDecreaseReplicationFactorNodeIDsToRemoveClearsOnRecovery."}
---

## Notes

**Protocol**: DAX uses `awsjson1.1` (`X-Amz-Target: AmazonDAXV3.<Op>`), confirmed against the
SDK's `awsAwsjson11_*` (de)serializer function names in
`aws-sdk-go-v2/service/dax@v1.29.18/{serializers,deserializers}.go`.

**Bugs found and fixed this pass** (all in `services/dax/handler.go` unless noted):

1. **`clusterResponse.Status` wire key was `"ClusterStatus"`, should be `"Status"`.** The real
   deserializer (`awsAwsjson11_deserializeDocumentCluster`) only recognizes `"Status"`; any other
   key hits its `default: _, _ = key, value` case and is silently discarded, leaving the client's
   `Cluster.Status` `nil` for every Create/Describe/Update/Delete/RebootNode/IncreaseReplicationFactor/
   DecreaseReplicationFactor response. The pre-existing unit test (`TestHandlerCreateCluster`)
   asserted the *wrong* key (`cluster["ClusterStatus"]`) and passed, because it checked the
   handler's own (buggy) output rather than the real wire contract -- classic "unit tests are not
   parity proof" trap. Fixed, and the assertion now checks `"Status"`.

2. **Timestamps emitted as RFC3339 strings instead of epoch-seconds JSON numbers.** DAX's
   awsjson1.1 protocol uses the `unixTimestamp` wire format for `Node.NodeCreateTime` and
   `Event.Date` (confirmed via `smithytime.ParseEpochSeconds` in both deserializers); the real
   client rejects a JSON string here with `"expected TStamp to be a JSON Number, got string
   instead"`. `nodeResponse.NodeCreateTime` and `eventResponse.Date` are now `float64`, populated
   via `pkgs/awstime.Epoch`.

3. **`DescribeEventsInput.StartTime`/`EndTime` request fields were unmarshaled as RFC3339
   strings.** The real client serializes these as epoch-seconds JSON numbers
   (`smithytime.FormatEpochSeconds`), so any real SDK call passing `StartTime`/`EndTime` would fail
   `json.Unmarshal` with "cannot unmarshal number into Go struct field ... of type string" and
   surface as a wrongly-generic `SerializationException` instead of actually filtering events.
   Fixed: request fields are now `json.Number`, decoded via a local `parseEpochSeconds` helper
   (`time.Unix` construction; `pkgs/awstime` only has the encode direction, not decode).

4. **`paramGroupStatus.NodeIDsToReboot` wire key was `"NodeIDsToReboot"`, should be
   `"NodeIdsToReboot"`.** Case-sensitive key mismatch against
   `awsAwsjson11_deserializeDocumentParameterGroupStatus`; the real client would never see the
   pending-reboot node list. Fixed the JSON tag only -- the Go field name keeps the
   `NodeIDsToReboot` initialism spelling (golangci-lint `revive` var-naming requires `IDs`, not
   `Ids`, in Go identifiers; only the wire tag needs to match AWS's casing).

5. **`toClusterResponse` never copied `ParameterGroup.NodeIDsToReboot` into the wire response.**
   The backend (`UpdateParameterGroup` in `backend.go`) correctly computes and stores the
   pending-reboot node list on `Cluster.ParameterGroup.NodeIDsToReboot`, but the handler's
   `paramGroupStatus{...}` literal never read it -- a disguised no-op only visible at the HTTP
   layer (existing test `TestUpdateParameterGroupMarksPendingReboot` called the backend directly
   and never caught this). Fixed by adding the field to the literal; new
   `TestHandlerParameterGroupNodeIdsToRebootWireKey` exercises it end-to-end via `daxRequest`.

6. **`Parameter.ChangeType` value was `"requires-reboot"` (lowercase-hyphen), should be
   `"REQUIRES_REBOOT"`** (`services/dax/backend.go`, `buildParameter`). The real SDK's
   `types.ChangeType` enum only has two values, `"IMMEDIATE"` and `"REQUIRES_REBOOT"`; the
   deserializer stores whatever string arrives verbatim (no server-side validation), so a client
   comparing against `types.ChangeTypeRequiresReboot` would never match. Fixed the constant and
   the corresponding assertion in `backend_parity_test.go`.

**Traps confirmed NOT bugs (checked against the real deserializer, left alone)**:
- `Endpoint{Address,Port,URL}`, `Node{AvailabilityZone,Endpoint,NodeId,NodeStatus,
  ParameterGroupStatus}`, `NotificationConfiguration{TopicArn,TopicStatus}`,
  `SecurityGroupMembership{SecurityGroupIdentifier,Status}`, `SSEDescription{Status}`,
  `Parameter{AllowedValues,DataType,Description,IsModifiable,ParameterName,ParameterType,
  ParameterValue,Source}` (except ChangeType, see bug 6), `SubnetGroup{Description,
  SubnetGroupName,Subnets,VpcId}`, `Subnet{SubnetAvailabilityZone,SubnetIdentifier}`,
  `Tag{Key,Value}`, and all top-level output envelope keys (`Cluster`, `Clusters`, `ParameterGroup`,
  `ParameterGroups`, `SubnetGroup`, `SubnetGroups`, `Parameters`, `Tags`, `Events`, `NextToken`,
  `DeletionMessage`) all match the real serializers/deserializers exactly.
- `EncryptionTypeNone`/`EncryptionTypeTLS` ("NONE"/"TLS") and `IsModifiable` ("TRUE") enum values
  match `types.ClusterEndpointEncryptionType`/`types.IsModifiable` exactly.
- Error mapping (`mapError` in `handler.go`): every sentinel error maps to the AWS-documented fault
  code (`ClusterNotFoundFault`, `ParameterGroupAlreadyExistsFault`, `TagQuotaPerResourceExceeded`,
  etc.) via the `__type` envelope field, which is what `getProtocolErrorInfo` /
  `resolveProtocolErrorType` in the real deserializer read when `X-Amzn-ErrorType` is absent. HTTP
  status is uniformly 400 for client faults / 500 for `InternalFailure`; the real client only
  checks `>= 300` to decide "this is an error", so the exact 4xx code doesn't need to vary per
  fault the way REST protocols do.
- `RebootNode`/`IncreaseReplicationFactor`/`DecreaseReplicationFactor` intentionally do **not**
  short-circuit under `DAX_TEST_SYNC=1` the way `CreateCluster`/`DeleteCluster` do. I initially
  "fixed" this (reading `zz_testmain_test.go`'s doc comment, which claims reboot/replication-factor
  changes are covered) and it broke three real tests, including `TestRebootNodeRecovery` which
  deliberately sleeps 2s to assert the transient `"rebooting"` state is observable immediately
  after the call and recovers asynchronously. Reverted -- the async-only behavior is *more*
  AWS-accurate (a real `RebootNode` response shows the node still transitioning), and the
  `TestMain` comment is simply imprecise about which ops it covers. Left as-is.

## 2026-07-24 follow-up pass

Closed all 3 gaps left open by the 2026-07-12 audit, and removed both banned `cyclop` nolints
by decomposing to data-driven tables (field-diffed against
`aws-sdk-go-v2/service/dax@v1.29.18`, module downloaded read-only to the local mod cache for
diffing; `go.mod`/`go.sum` left untouched):

1. **`DescribeParameters` now honors the request's `Source` filter.** Confirmed against
   `types.DescribeParametersInput.Source` (`*string`, free text, doc example: `"system denotes
   a system-defined parameter"`) -- the real field has no enum; gopherstack's backend only ever
   produces `"user"`/`"system"` (never `"engine-default"`), so filtering on those two values is
   the correct, non-invented behavior. Wired through `InMemoryBackend.DescribeParameters`'s new
   `sourceFilter` parameter and the handler's `Source` request field.

2. **`Cluster.NetworkType`, `Cluster.NodeIdsToRemove`, and `SubnetGroup.SupportedNetworkTypes`
   are now modeled**, field-diffed against `types.Cluster`/`types.SubnetGroup`/
   `types.CreateClusterInput` and their wire keys confirmed in `serializers.go`/`deserializers.go`
   (`"NetworkType"`, `"NodeIdsToRemove"`, `"SupportedNetworkTypes"` all match exactly).
   - `NetworkType`: `CreateClusterInput` accepts `ipv4`/`ipv6`/`dual_stack`
     (`ErrInvalidParameterValue` on anything else), defaulting to `ipv4` when omitted --
     gopherstack subnet groups are always IPv4-only (no per-subnet CIDR/IP-family modeling), so
     `ipv4` is the only *correct* derived default; `UpdateClusterInput` does **not** have a
     `NetworkType` field in the real SDK, so it is create-only, matching AWS.
   - `NodeIdsToRemove`: transient, mirroring `NodeIdsToReboot`'s existing pattern -- populated on
     `Cluster` by `DecreaseReplicationFactor` with the node IDs that operation is removing (either
     the caller's explicit `NodeIDsToRemove` or the trailing nodes when unspecified), and cleared
     by the same 1s-delay async goroutine that already exists to flip `Status` back to
     `"available"`. No new goroutine, no new leak surface.
   - `SupportedNetworkTypes`: `SubnetGroup` always reports `["ipv4"]` (`NetworkTypeIPv4`) -- honest
     given gopherstack subnets have no real IP-family data to derive from; not fabricated as a
     configurable input since `CreateSubnetGroupInput`/`UpdateSubnetGroupInput` have no matching
     field in the real SDK either.

3. **Deleted the gopherstack-invented "parameter groups and subnet groups are taggable" behavior.**
   Confirmed by field-diffing `types.ParameterGroup`/`types.SubnetGroup` against `types.Cluster`:
   only `Cluster` has an `Arn`/`ClusterArn` field in the real SDK. `TagResource`/`UntagResource`/
   `ListTags` are documented as cluster-only operations for exactly this reason -- there is no ARN
   to tag on the other two resource types. `arnExists` (`tags.go`) no longer recognizes
   `parametergroup/`/`subnetgroup/` ARN prefixes; `TestTagResource`'s two corresponding subtests
   were converted from "tag succeeds" to "rejected as not found" (they were asserting invented
   behavior, not real AWS behavior).

4. **Both banned `//nolint:cyclop` uses in `handler.go` removed by decomposition, not suppression:**
   - `dispatch`'s 22-case operation switch became a `map[string]daxOpHandler` lookup
     (`daxOperations`) built from method expressions (`(*Handler).handleCreateCluster`, ...);
     `dispatch` itself is now a two-line map lookup.
   - `mapError`'s 20-case error-mapping switch became an ordered `[]errCodeMapping` table
     (`daxErrCodeMappings`) iterated with a single `errors.Is` loop. Ordering (specific sentinels
     before their generic `awserr.ErrNotFound`/`ErrConflict`/`ErrInvalidParameter` parents) is
     preserved exactly, since `errors.Is` still short-circuits on the first match and specific
     entries are listed first.
   - Both new tables are `gochecknoglobals`-exempted the same way `models.go`'s existing lookup
     tables are (package-level lookup table, immutable after init).

## 2026-08-10 control-plane sweep (gopherstack-mmqd)

Confirmed both recorded follow-up items are honest and out of scope, then swept the control
plane for the bug classes found repeatedly elsewhere in today's campaign (state mutated before
validation, wrong/missing required-field errors, fabricated or missing wire fields, allowlists
vs. SDK enums). All errors cited below are from botocore's `dax` `service-2.json`
(`2017-04-19`, matching the pinned `aws-sdk-go-v2/service/dax@v1.32.4`'s wire behavior) and that
SDK's generated `validators.go`/`deserializers.go`.

**Confirmed, not re-litigated:**

- `InsufficientClusterCapacityFault`/`ServiceLinkedRoleNotFoundFault`: both genuinely modeled as
  `strings.EqualFold`-matched error codes in the pinned SDK's deserializers (`deserializers.go`
  lines 127-128, 154-155 and throughout every cluster-mutating op's error switch) with no
  request-shape-driven trigger gopherstack could honestly derive -- left unmodeled, as recorded.
- `services/dax/dataplane/` is ~6900 LOC (`find services/dax/dataplane -name '*.go' | xargs wc -l`)
  implementing aws-dax-go's binary client protocol, a wholly separate wire format from the JSON
  control plane audited here -- confirmed out of scope, not touched.

**Bugs found and fixed this pass** (all backend/handler files under `services/dax/` unless noted):

1. **`UpdateCluster` mutated `Description`/`PreferredMaintenanceWindow`/`SecurityGroupIDs`
   directly on the live `*Cluster` (from `store.Table.Get`, not a copy) before validating that
   `ParameterGroupName` refers to an existing parameter group.** A request with a valid
   `PreferredMaintenanceWindow` and a bogus `ParameterGroupName` returned an error but still
   committed the maintenance-window change -- the caller's error implies nothing changed, but it
   did. Fixed by moving the `ParameterGroupName` existence check before any field is written
   (`clusters.go`). `TestUpdateClusterRejectedRequestDoesNotMutateState` fails on the pre-fix code
   with `PreferredMaintenanceWindow` observably changed after the rejected call.

2. **`UpdateParameterGroup` validated and wrote each `ParameterNameValues` entry in the same loop
   iteration**, so a 2-entry batch where entry 1 is valid and entry 2 is invalid committed entry 1
   to the live `ParameterGroup.Parameters` map before returning the error for entry 2. AWS rejects
   the whole request atomically. Fixed with a validate-then-apply split: `validateParameterNameValues`
   checks every entry first; only if all pass does a second loop write them (`parameter_groups.go`).
   `TestUpdateParameterGroupRejectedBatchDoesNotPartiallyApply` fails on the pre-fix code with
   `query-ttl-millis` observably changed despite the batch being rejected.

3. **Six required-field checks (`ClusterName` on `UpdateCluster`/`DeleteCluster`/
   `IncreaseReplicationFactor`/`DecreaseReplicationFactor`/`RebootNode`, `IamRoleArn` on
   `CreateCluster`) returned `ErrInvalidARN` (`InvalidARNFault`), a fault botocore declares only
   for `TagResource`/`UntagResource`/`ListTags`'s ARN-shaped `ResourceName` parameter** -- never
   for any of these six operations, all of which do declare `InvalidParameterValueException`. A
   real client checking for a specific typed fault (`errors.As(err, &types.ClusterNotFoundFault{})`
   -style) on one of these ops would see an untyped `smithy.GenericAPIError` instead of the
   documented error family. Fixed by switching all six sites to `ErrInvalidParameterValue`
   (`clusters.go`); `tags.go`'s three legitimate `ErrInvalidARN` uses (all on `ResourceName`) are
   untouched. `TestClusterOpsRequiredFieldErrorCode` fails on the pre-fix code for all six.

4. **`Cluster`'s wire response (`clusterResponse` in `handler_clusters.go`) serialized a `Tags`
   field that does not exist on the real `types.Cluster` shape.** Confirmed against botocore's
   `Cluster` shape (`ClusterName, Description, ClusterArn, TotalNodes, ActiveNodes, NodeType,
   Status, ClusterDiscoveryEndpoint, NodeIdsToRemove, Nodes, PreferredMaintenanceWindow,
   NotificationConfiguration, SubnetGroup, SecurityGroups, IamRoleArn, ParameterGroup,
   SSEDescription, ClusterEndpointEncryptionType, NetworkType` -- no `Tags` member) and the real
   deserializer (`awsAwsjson11_deserializeDocumentCluster`, which has no `"Tags"` case and would
   silently discard it). Harmless to a real client but not what AWS returns; tags are only ever
   readable via `ListTags`. Removed the field from the wire struct only -- the backend
   `Cluster.Tags` map is untouched (still the persistence/tag-propagation source of truth).
   `TestHandlerClusterResponseHasNoTagsField` fails on the pre-fix code; the pre-existing
   `TestHandlerCreateClusterTagsAsArray` asserted the fabricated field and was rewritten to verify
   tag storage via `ListTags` instead (the "unit tests are not parity proof" trap -- that test had
   locked in the bug it should have caught).

5. **Three `@required` fields were never validated, so omitting them from the request silently
   succeeded as a no-op** instead of being rejected: `TagResource.Tags` (`validators.go:603`),
   `UntagResource.TagKeys` (`validators.go:621`), and `UpdateParameterGroup.ParameterNameValues`
   (`validators.go:654`). All three check `nil`, not emptiness (matching the SDK client validator's
   own semantics: a present-but-empty array satisfies `@required`, only a wholly absent field
   doesn't), so each handler now checks the raw unmarshaled slice for `nil` before conversion --
   `UpdateParameterGroup`'s handler previously always allocated a non-nil `pvs` via `make(...)`
   regardless of whether the field was present, destroying the distinction before it reached the
   backend; fixed to only allocate when the source field is non-nil. Both `TestHandlerTagResourceRequiresTags`
   and `TestHandlerUntagResourceRequiresTagKeys` and `TestHandlerUpdateParameterGroupRequiresParameterNameValues`
   fail on the pre-fix code.

6. **`SubnetGroup.Subnets` entries had no per-subnet `SupportedNetworkTypes` field.** botocore's
   `Subnet` shape has its own `SupportedNetworkTypes` member (`NetworkTypeList`), distinct from
   `SubnetGroup`'s group-level field of the same name -- gopherstack modeled only the latter. Added
   `SubnetEntry.SupportedNetworkTypes` (always `["ipv4"]`, same honest derivation as the
   group-level field: gopherstack has no per-subnet CIDR/IP-family data), threaded through
   `subnetEntriesFromIDs` and the default subnet group seed. Additive `omitempty` field on an
   existing persisted type; `TestInMemoryBackend_SnapshotRestore_FullState` now asserts it
   round-trips. `TestHandlerSubnetGroupSubnetHasSupportedNetworkTypes` fails on the pre-fix code.

**Checked and found clean (no fix needed):**

- **Allowlist vs. SDK enum**: `ClusterEndpointEncryptionType` (`NONE`/`TLS`), `NetworkType`
  (`ipv4`/`ipv6`/`dual_stack`), `IsModifiable`, `ParameterType`, `SourceType`, `SSEStatus` in
  `types/enums.go` all match gopherstack's constants exactly -- no drift. `NodeType` is a free-text
  `*string` in the SDK (botocore shape `String`, no enum), not a smithy enum at all, so
  `validNodeTypes`'s 16-entry allowlist cannot be checked against an SDK-declared value set;
  left as-is rather than fabricating a source of truth that doesn't exist.
- **Tagging a nonexistent ARN**: `TagResource`/`UntagResource`/`ListTags` all call `arnExists`
  before any mutation; already fixed in the 2026-07-24 pass (parameter/subnet groups are
  correctly untaggable, matching the real SDK having no `Arn` field on those types).
- **IncreaseReplicationFactor/DecreaseReplicationFactor/RebootNode**: re-checked for the same
  mutate-before-validate pattern as bug 1 -- all three complete every validation (status check,
  factor bounds, `NodeIDsToRemove` existence/count via `removeSpecificNodes`, which returns a new
  slice rather than mutating in place) before touching the live cluster. Clean.
- **Lifecycle/status polling**: synchronous `CreateCluster`/`DeleteCluster` (`DAX_TEST_SYNC=1`)
  and the async 1s-delay paths both leave `DescribeClusters` reporting a status
  (`"available"`/`"creating"`/`"deleting"`/`"modifying"`/`"rebooting"`) a real client polling for
  `"available"` would accept; `DeleteCluster`'s async goroutine fully removes the cluster from
  `b.clusters` (subsequent `DescribeClusters` returns `ClusterNotFoundFault`, not a lingering
  `"deleting"` row) and cleans up `b.tags`. No live-status-after-deletion gap found.
- **DeleteParameterGroup/DeleteSubnetGroup**: both refuse deletion outright while referenced by a
  cluster (`ErrParameterGroupInUse`/`ErrSubnetGroupInUse`), so there is no path to a deleted
  resource still reporting a live status.

## 2026-08-20 wrapper-key / nested-shape sweep

Full wrapper-key/nesting-level/member-shape/enum-value sweep of all 20 advertised ops against
`aws-sdk-go-v2/service/dax@v1.32.4` (module cache path:
`$GOMODCACHE/github.com/aws/aws-sdk-go-v2/service/dax@v1.32.4`). Protocol reconfirmed as
`awsjson1.1` (`Content-Type: application/x-amz-json-1.1`, `X-Amz-Target: AmazonDAXV3.<Op>`,
`api_client.go`; every op's live decode path is `awsAwsjson11_deserializeOpDocument<Op>Output`,
confirmed present with both a definition and a call site for each op -- the restjson
flat-body false-positive trap this campaign is watching for does not apply to DAX's awsjson1.1
protocol).

**Required-member grep** (`grep -rn "This member is required" types/*.go api_op_*.go`): every hit
is on an *Input* (request) shape -- `CreateClusterInput{ClusterName, IamRoleArn, NodeType,
ReplicationFactor}`, `CreateParameterGroupInput.ParameterGroupName`,
`CreateSubnetGroupInput{SubnetGroupName, SubnetIds}`,
`De/IncreaseReplicationFactorInput{ClusterName, New/ReplicationFactor}`,
`DeleteClusterInput.ClusterName`, `DeleteParameterGroupInput.ParameterGroupName`,
`DeleteSubnetGroupInput.SubnetGroupName`, `DescribeParametersInput.ParameterGroupName`,
`ListTagsInput.ResourceName`, `RebootNodeInput{ClusterName, NodeId}`,
`Tag/UntagResourceInput{ResourceName, Tags/TagKeys}`, `UpdateClusterInput.ClusterName`,
`UpdateParameterGroupInput{ParameterGroupName, ParameterNameValues}`,
`UpdateSubnetGroupInput.SubnetGroupName`, and `types.SSESpecification.Enabled`. Every one of
these is validated in `services/dax/{clusters,parameter_groups,subnet_groups,tags}.go` (grepped
`is required` error sites and confirmed each maps 1:1 to an SDK `@required` field; none missing,
none extra). No required member on any *Output* shape exists at all in this SDK version -- so
there is no "populated" half of this check to fail.

**`ParameterGroup` vs `ParameterGroupStatus` vs `Cluster.ParameterGroup`**: all three distinct and
each matches its own deserializer. `types.ParameterGroup{Description, ParameterGroupName}`
(`types/types.go:224`) is the `CreateParameterGroup`/`UpdateParameterGroup` output shape, wire key
`"ParameterGroup"`, deserialized by `awsAwsjson11_deserializeDocumentParameterGroup`
(`deserializers.go:5017`) -- gopherstack's `parameterGroupResponse{ParameterGroupName,
Description}` (`handler_parameter_groups.go`) matches exactly, no extra/missing fields.
`types.ParameterGroupStatus{NodeIdsToReboot, ParameterApplyStatus, ParameterGroupName}`
(`types/types.go:240`) is the nested status object living at `Cluster.ParameterGroup`
(`types/types.go:69`), deserialized by `awsAwsjson11_deserializeDocumentParameterGroupStatus`
(`deserializers.go:5220`) under the `"ParameterGroup"` key inside the `Cluster` document
(`deserializers.go:3628`, case `"ParameterGroup"`) -- gopherstack's `paramGroupStatus{
ParameterGroupName, ParameterApplyStatus, NodeIDsToReboot}` (`handler_clusters.go`), nested inside
`clusterResponse.ParameterGroup *paramGroupStatus`, matches exactly. No cross-contamination
between the two Go types on either side.

**`Parameter` shared by `DescribeParameters`/`DescribeDefaultParameters`**: both real ops decode
their `"Parameters"` array with the identical `awsAwsjson11_deserializeDocumentParameterList` ->
`awsAwsjson11_deserializeDocumentParameter` (`deserializers.go:4900`), and both gopherstack
handlers (`handleDescribeParameters`/`handleDescribeDefaultParameters`,
`handler_parameter_groups.go`) route through the same `toParameterResponse`/`parameterResponse`
-- single shared shape both sides, no divergence.

**Bug found and fixed:**

1. **Fabricated `SourceType` enum value `"NODE"` on node-level `Event`s.** The real
   `types.SourceType` enum (`types/enums.go`) has exactly three values --
   `CLUSTER`/`PARAMETER_GROUP`/`SUBNET_GROUP` -- there is no `NODE` value, confirmed by grepping
   every `case` in `awsAwsjson11_deserializeDocumentEvent` (`deserializers.go:4026`) and
   `(types.SourceType).Values()`. `services/dax/models.go` defined an extra
   `EventSourceTypeNode = "NODE"` constant, used by `RebootNode`'s two `emitEventLocked` calls
   (`clusters.go:791,809`, both `SourceName` = the cluster name already, not the node ID) and one
   async-recovery call in `persistence.go:263`. A real client's `DescribeEvents` would decode this
   as `types.SourceType("NODE")`, silently diverging from every value the real service can ever
   emit -- caught by this sweep's "check every emitted enum against `types/enums.go`" rule. This
   claim was checked and wrongly cleared in the 2026-08-10 pass's "Allowlist vs. SDK enum" section
   (it verified the *values gopherstack advertises as constants* against the enum but missed that
   an extra, unused-by-that-check constant existed and was actually being emitted on the wire).
   Fixed by deleting `EventSourceTypeNode` and switching all three call sites to
   `EventSourceTypeCluster` (`models.go`, `clusters.go`, `persistence.go`) -- consistent with how
   every other cluster-scoped lifecycle event in this backend is already reported, and with the
   real API having no node-granularity `SourceType` to report through.
   `TestDescribeEvents_NodeRebootSourceType_SDKRoundTrip`
   (`services/dax/wire_sdk_roundtrip_test.go`) drives `CreateCluster` -> `RebootNode` ->
   `DescribeEvents` through the real `aws-sdk-go-v2/service/dax` client over
   `pkgs/service`'s router and asserts `daxtypes.SourceTypeCluster` on the reboot-initiated event.
   Hand-revert (`git stash` of the three fixed files) reproduced the exact predicted symptom
   (`expected: "CLUSTER", actual: "NODE"`); restoring the fix made the test pass again with an
   unchanged diff stat, confirming the revert round-tripped byte-identically.

**Families checked and confirmed clean this pass** (wrapper key, nesting, member set, and enum
values all verified against the live deserializer/serializer function bodies, not just
`types.go` struct shape): `Cluster` (including `ClusterDiscoveryEndpoint`/`Endpoint`,
`Nodes`/`Node`, `NotificationConfiguration`, `ParameterGroup`/`ParameterGroupStatus`,
`SecurityGroups`/`SecurityGroupMembership`, `SSEDescription`), `SubnetGroup`/`Subnet` (including
the per-subnet vs. group-level `SupportedNetworkTypes` distinction), `Tag`/`TagList`, `Event`, all
request shapes for all 20 ops (every `object.Key(...)` call in each op's
`awsAwsjson11_serializeOpDocument<Op>Input` diffed 1:1 against gopherstack's request struct
fields), and all output wrapper keys (`Cluster`, `Clusters`, `ParameterGroup`, `ParameterGroups`,
`SubnetGroup`, `SubnetGroups`, `Parameters`, `Tags`, `Events`, `DeletionMessage`, `NextToken`).

**Gap disclosed, not fixed:** `types.Parameter.NodeTypeSpecificValues`
([]types.NodeTypeSpecificValue, `types/types.go:203`, decoded by
`awsAwsjson11_deserializeDocumentNodeTypeSpecificValueList` inside
`awsAwsjson11_deserializeDocumentParameter`) is entirely unmodeled -- no field on gopherstack's
`Parameter` struct or `parameterResponse` wire struct, never populated. In practice this is inert:
gopherstack's only two parameters (`query-ttl-millis`, `record-ttl-millis`) are both
`ParameterType: DEFAULT`, and real DAX has no node-type-specific values for either, so the real
service also returns this field empty/absent for both -- adding an always-nil field would be
byte-identical on the wire (an omitted key and a present-but-empty list deserialize to the same
observable client-side state). Left unmodeled rather than adding dead code; would only become a
real gap if a future parameter needed node-type-specific values, which nothing in this backend's
design currently produces.

**Provenance check**: `last_audit_commit` `70eea523b6f0` (2026-08-10 pass) -> `git show -s
--format=%ad 70eea523b6f0` = `2026-08-10 19:49:57 -0500`, matching `last_audit_date: 2026-08-10`
exactly (same-day commit-vs-date pair, no gap) -- consistent, no false-stamp finding. That
commit's own diff is unrelated to `services/dax/` (a `servicediscovery` fix), which is expected
and not itself a red flag per this campaign's provenance rule (the schema defines the field as
HEAD-at-write-time, not "last commit that touched this service"). Stamp refreshed above to
current HEAD (`b8ef75b1e`) and today's date (2026-08-20) so the pair stays self-consistent.

## 2026-08-23 request-side accept-and-drop (gopherstack-n3zi)

DecreaseReplicationFactor dropped AvailabilityZones, a real body-bound member
naming which AZs to remove nodes from. Node.AvailabilityZone is real, populated
state, so this is accept-and-drop rather than a modelling gap. Proven by a
real-SDK-client round trip: asked to remove the us-east-1b node, the unfixed
code removed the trailing us-east-1c node instead.

## 2026-08-29 write-only-state sweep (gopherstack-6flj / gopherstack-21my)

Forward+reverse write-only-state sweep of the control-plane backend files (`clusters.go`,
`parameter_groups.go`, `subnet_groups.go`, `tags.go`, `events.go`) and their handlers
(`handler_clusters.go`, `handler_parameter_groups.go`, `handler_subnet_groups.go`,
`handler_tags.go`, `handler_events.go`), against `aws-sdk-go-v2/service/dax@v1.32.4`, on top
of the already-thorough 2026-08-10 and 2026-08-20 sweeps. No dax-specific commit landed
between those two prior passes and this one (`git log --oneline -- services/dax/` shows only
cross-service commits #2435/#2440 touching this service not at all), so this was a genuine
re-verification against unchanged code, not a stale-claim check.

**No new bug found.** Every field written by a Create/Update op was traced to a real read
path: `Cluster.Tags` (never surfaces on the wire -- correct, real `types.Cluster` has no
`Tags` field at all, confirmed at `types/types.go:11-83`); `NotificationConfiguration`'s
partial-update branch (`UpdateCluster` changing only `NotificationTopicStatus` on an existing
config without a new ARN); `ParameterGroup.NodeIDsToReboot`/`ParameterApplyStatus`
transitioning to `"pending-reboot"` on `UpdateParameterGroup` and surfacing on
`Cluster.ParameterGroup`; `Cluster.NodeIDsToRemove`'s transient in-flight-only lifecycle;
`SubnetGroup.VpcID`/`Subnets[].SupportedNetworkTypes`. `toClusterResponse` was re-diffed
field-by-field against the full `types.Cluster` struct (`types/types.go:11-83`) -- all 19 real
members present and correctly named (`ClusterDiscoveryEndpoint`, not the internal model's
`Endpoint` field name, confirmed still correctly retagged in `clusterResponse`).

**Zeroguard findings, disqualified:** `UpdateCluster`'s `PreferredMaintenanceWindow`/
`ParameterGroupName`/`NotificationTopicArn`/`NotificationTopicStatus` and
`UpdateSubnetGroup`'s `Description` are zero-check-guarded plain strings backing pointer SDK
fields ("empty means omitted, don't change"), which is the correct optional-update
convention this backend uses consistently (matches the real API's own pointer-nil-means-omit
semantics) -- not a meaningful-zero-value bug. `ClusterName`/`ParameterGroupName`/
`SubnetGroupName` "no zero-guard found" findings are required fields on their respective
Update inputs, validated explicitly before use; a zero-guard would be wrong here, not missing.

**Not reached this pass:** `dataplane/`, `dataplane_server.go`, `dataplane_integration_test.go`
(the DAX client-protocol data-plane emulation -- a different wire protocol than the
control-plane REST/JSON surface this campaign's bug class targets, out of scope); `store.go`/
`store_setup.go`/`persistence.go`/`provider.go` (read only incidentally).

**Gates:** `go build ./services/dax/...`, `go vet ./services/dax/...`,
`go test -race -count=1 ./services/dax/...` (pass, including `./services/dax/dataplane/...`),
`golangci-lint run --fix ./services/dax/...` (0 issues, no changes).

## 2026-08-29 indexed-list wire-key sweep (rds `Values.Value`/neptune `EventCategory` bug family, N/A)

Same check as memorydb (same campaign, same reasoning): confirmed DAX is JSON-RPC 1.1
(`awsAwsjson11_*` prefix, pinned dax@v1.32.4), so this service also decodes requests via
`encoding/json` into typed structs with no indexed `list.N` key parsing -- the structural precondition
for the rds/neptune bug family doesn't exist here either. Spot-checked slice-typed request fields
(`CreateCluster`/`DecreaseReplicationFactor`/`IncreaseReplicationFactor`/`UpdateCluster`/
`CreateParameterGroup`/`UpdateParameterGroup`/`DescribeParameters`) against `awsAwsjson11_serializeOpDocument<Op>Input`
in the pinned SDK -- all json tags match. Confirmed `DescribeEventsInput` (dax@v1.32.4
api_op_DescribeEvents.go) has no `EventCategories` field, so the neptune-specific variant doesn't apply.
No `[0]`/first-element-only truncation found in request-decode paths (the one `[0]` hit,
`vpcIDFromSubnets` in `subnet_groups.go`, derives a synthetic placeholder VPC ID from a subnet list and
isn't request filtering). This bug class doesn't apply to this service.

Gates: `go build ./services/dax/...`, `go vet ./services/dax/...` and `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/dax/...` (pass, including `./services/dax/dataplane/...`, no changes),
`golangci-lint run ./services/dax/...` (0 issues). No code changed this pass.

## 2026-08-29 pagination-helper arithmetic sweep (wrapper-key-sweep campaign)

Four bugs found and fixed, all the same class: an exact-match ("==") or bounded
(`n < len(all)`) cursor lookup that silently fell back to its zero-value default —
offset/index 0 — whenever the cursor didn't land on a currently-present element, instead
of resuming past it (a deleted item) or terminating (an exhausted/garbage token). Net
effect: a stale cursor handed the caller duplicate items already seen (deletion case) or
restarted pagination at page one forever (exhaustion case), rather than returning the
correct remainder or an empty page.

- `paginateList` (`store.go`, generic — backs `DescribeParameterGroups` and
  `DescribeSubnetGroups` via `describeNamedGroups`, 2 operations): fixed by searching for
  the first `getName(item) >= nextToken` instead of `==`, defaulting `start = len(all)` when
  no match is found (previously defaulted to 0).
- `paginateClusters` (`clusters.go` — `DescribeClusters`, 1 operation): same fix,
  `c.ClusterName >= nextToken` / default `len(all)`.
- `paginateParameters` (`parameter_groups.go` — `DescribeParameters` and
  `DescribeDefaultParameters`, 2 operations): its cursor is a plain decimal index
  (`strconv.Atoi`), not a name lookup, but the inner validation `idx >= 0 && idx < len(all)`
  rejected any out-of-range `idx` and left `start` at its zero-value default instead of
  falling through to the existing `if start >= len(all) { return empty }` guard — same
  net bug, different mechanism. Fixed by dropping the `idx < len(all)` half of the inner
  check and letting the outer guard do its job.
- `DescribeEvents` (`events.go`, 1 operation): identical `idx < len(filtered)` bug as
  `paginateParameters`, same fix.
- `ListTags` (`tags.go`, 1 operation): identical exact-match-cursor bug as `paginateList`/
  `paginateClusters` (sorted tag keys via `collections.SortedKeys`), same `>=`/default-to-end
  fix. Reachable in practice: `UntagResource` between two `ListTags` calls reproduces it.

7 operations affected total. Every fix is proven by a failing-then-passing unit test against
the helper directly (`pagination_arithmetic_test.go`, `tags_test.go`) plus one real
`aws-sdk-go-v2/service/dax` client round trip
(`pagination_sdk_roundtrip_test.go`: `ListTags`, deletes the cursor's tag between pages).

`paginateBlocks`-style "n < len" bug independently recurred in `services/textract` and was
fixed there too — see that service's PARITY.md; not the same helper, no shared root cause,
just the same mistake made twice.

**Not fixed, recorded only:** `ListReadSetUploadParts` in `services/omics/read_sets.go` has
the same exact-match-cursor shape (compares `strconv.Itoa(p.PartNumber) == nextToken`) but
was found unreachable in that service: there's no per-part delete, only whole-upload
delete/abort, so the named part can never go missing between calls. See omics's PARITY.md.

Gates: `go build ./services/dax/...`, `go vet ./services/dax/...` and `go vet ./...`
(repo-wide, clean), `go test -race -count=1 ./services/dax/...` (pass, including
`./services/dax/dataplane/...`), `golangci-lint run ./services/dax/...` (0 issues).

## 2026-09-04 parity-sweep-2026-09-03 campaign

Ran the campaign's two cheap mechanical checks across the whole control-plane package (never-
returned sentinels, parsed-then-dropped request fields) plus a Delete/Update precondition read
of every op's doc comment in `aws-sdk-go-v2/service/dax@v1.32.4`. All 16 sentinels in
`errors.go` are reachable from non-`errors.go`/non-handler backend code (`ErrSubnetGroupInUse`
and `ErrParameterGroupInUse` included -- both wired in `subnet_groups.go`/`parameter_groups.go`).
Every Describe* filter/pagination field (`ClusterNames`, `SubnetGroupNames`,
`ParameterGroupNames`, `Source`, `SourceName`/`SourceType`/`StartTime`/`EndTime`/`Duration` on
`DescribeEvents`) and every `CreateCluster` nested member (`SSESpecification`,
`ParameterGroupName`, `SecurityGroupIds`, `AvailabilityZones`, `NotificationTopicArn`) round-
trips through the backend already.

**Bug found and fixed:**

1. **`CreateCluster` accepted an `AvailabilityZones` list whose length didn't match
   `ReplicationFactor` instead of rejecting it.** `api_op_CreateCluster.go`'s
   `ReplicationFactor` doc: *"If the AvailabilityZones parameter is provided, its length must
   equal the ReplicationFactor parameter."* -- restated verbatim on the `AvailabilityZones`
   field's own doc comment. `CreateCluster`'s modeled error set (`deserializeOpErrorCreateCluster`)
   includes `InvalidParameterCombinationException`, the same fault gopherstack already uses for
   every other `ReplicationFactor`-adjacent parameter-combination check in this op (min/max
   bounds). Before this fix, `buildClusterNodes` silently handled a short list by falling back
   to a single default AZ for the remaining nodes (`clusters.go`, `az := b.Region + "a"` when
   `i >= len(input.AvailabilityZones)`) and silently ignored trailing entries in a long list --
   neither surfaced any error. `IncreaseReplicationFactorInput.AvailabilityZones`'s doc has no
   equivalent length constraint ("Use this parameter if you want to distribute the nodes across
   multiple AZs"), so that op and `DecreaseReplicationFactor` (which uses `AvailabilityZones` to
   select nodes to remove, not to size the cluster) were left unchanged -- this is a
   `CreateCluster`-only precondition, not a general rule for every op that carries the field.
   Fixed by adding `validateAvailabilityZonesLength` (`clusters.go`), called from
   `validateCreateCluster` before the lock is taken, same as every other `CreateCluster`
   validation. Decomposed into its own function rather than inlined, to stay under the banned
   `cyclop` threshold.
   `TestCreateClusterAvailabilityZonesLengthMustMatchReplicationFactor` (`clusters_test.go`)
   covers omitted/matching/fewer/more; neutering the guard (`if false && len(azs) > 0 && ...`)
   reproduces the bug -- both the fewer- and more-AZs subtests fail with `An error is expected
   but got nil`; restoring the guard passes again.

**Checked and found clean, no new fix:**

- `UpdateCluster`/`UpdateParameterGroup` re-read against the current code: validation
  (`ParameterGroupName` existence, `ParameterNameValues` batch) still runs before any field is
  written, consistent with the 2026-08-10 fix holding. `UpdateCluster.SecurityGroupIds` and
  `UpdateSubnetGroup.SubnetIds` are `len() > 0`-guarded (can't distinguish an explicit empty
  list from an omitted one) but neither op's doc states what an explicit empty list should do,
  so this wasn't escalated -- same "zero-guard, no doc evidence" reasoning the 2026-08-29 sweep
  already applied to `UpdateCluster`'s string fields.
- `DeleteSubnetGroup`/`DeleteParameterGroup` doc preconditions ("You cannot delete a subnet
  group/parameter group if it is associated with any DAX clusters") are enforced via
  `ErrSubnetGroupInUse`/`ErrParameterGroupInUse`, both reachable (see sentinel check above).
- `DecreaseReplicationFactor`'s doc precondition ("You cannot use DecreaseReplicationFactor to
  remove the last node") is enforced structurally: `minReplicationFactor = 1` rejects
  `NewReplicationFactor < 1` via the already-modeled `InvalidParameterCombinationException`.
- `NodeQuotaForClusterExceededFault`/`NodeQuotaForCustomerExceededFault` (modeled on
  `CreateCluster`/`IncreaseReplicationFactor`) have no doc comment in this SDK version to derive
  a trigger from -- left unmodeled, same reasoning as the 2026-08-10 pass's
  `InsufficientClusterCapacityFault`/`ServiceLinkedRoleNotFoundFault` finding.
- `cli.go`'s `wireTaggingDAX` (Resource Groups Tagging API cross-service wiring): DAX's
  single taggable resource kind (`cache/{name}` ARN segment) and its bare-error-vs-
  `(result, error)` adapter shape read correctly against `InMemoryBackend.TagResource`/
  `UntagResource`.
- Event ring buffer (`b.events`, `maxEventsPerBuffer` cap in `emitEventLocked`) and every
  `go func() { time.Sleep(...); ... }()` async status-flip goroutine (all guarded by an
  existence + expected-status check before mutating, all single-shot with a bounded sleep) --
  no unbounded growth, no leaked goroutine.

**Not reached this pass:** `services/dax/dataplane/` (~6900 LOC binary protocol, out of scope
per this campaign's own instructions -- see `DATAPLANE.md`; its `TestMain` already runs
`testleak.VerifyTestMain` against goroutine leaks). Performance dimension checked only by
inspection (no profiling run) -- the only O(n)-under-write-lock scans found are the
Delete*-in-use checks over `b.clusters.All()`, which are the same pattern every other
gopherstack service uses for this precondition and run only on already-infrequent delete calls.

Gates: `go build ./services/dax/...`, `go test -race -count=1 ./services/dax/...` (pass,
including `./services/dax/dataplane/...`), `golangci-lint run ./services/dax/...` (0 issues).

## 2026-09-07 errtargetaudit sweep (gopherstack-dkr8)

`cmd/errtargetaudit` flagged dax with 2 class A findings, both `code=InvalidParameterValueException`
on `services/dax/subnet_groups.go` (`CreateSubnetGroup:20` via the `validateResourceName`
constructor classifier, `UpdateSubnetGroup:72` via a direct sentinel reference). The tool's own
coverage line reads `21/77 (27%) resolved` with a coverage warning; that's not a dax gap -- the
tool attributes dax to two SDK modules (`dax`, `dynamodb`), and 77 is `dax`'s own 21 real ops plus
`dynamodb@v1.63.1`'s 58 (`services/dax/dataplane_server.go` imports gopherstack's own
`services/dynamodb` package for its `InMemoryDB` backend, to back the binary DAX-client data-plane
protocol -- a type/backend borrow, not an HTTP-JSON-dispatched dax op). All 21 real dax ops
resolved; coverage for the actual X-Amz-Target-dispatched surface is 21/21.

**Both findings confirmed real** against `aws-sdk-go-v2/service/dax@v1.32.4/deserializers.go`:

```
$ awk '/deserializeOpErrorCreateSubnetGroup\(/,/^}/' deserializers.go | grep -oE '"[A-Za-z0-9]+"'
"UnknownError" "InvalidSubnet" "ServiceLinkedRoleNotFoundFault" "SubnetGroupAlreadyExistsFault"
"SubnetGroupQuotaExceededFault" "SubnetNotAllowedFault" "SubnetQuotaExceededFault"

$ awk '/deserializeOpErrorUpdateSubnetGroup\(/,/^}/' deserializers.go | grep -oE '"[A-Za-z0-9]+"'
"UnknownError" "InvalidSubnet" "ServiceLinkedRoleNotFoundFault" "SubnetGroupNotFoundFault"
"SubnetInUse" "SubnetNotAllowedFault" "SubnetQuotaExceededFault"
```

Neither declares `InvalidParameterValueException` (not in `genericProtocolCodes` -- it's a real
per-op-declared exception type, `types.InvalidParameterValueException`, not a gateway-level
fallback). Protocol: `awsjson1.1`, `X-Amz-Target: AmazonDAXV3.<Op>`; this repo's handler shapes
every DAX fault as HTTP 400 with body `{"__type": "<code>", "message": "<msg>"}`
(`handler.go`'s `mapError`/`daxError`).

**Root cause: a global sentinel map (gopherstack-hdvu), used correctly everywhere except these two
call sites.** `daxErrCodeMappings` in `handler.go` maps the single sentinel `ErrInvalidParameterValue`
to `"InvalidParameterValueException"` for every op that returns it. `validateResourceName`
(`store.go`) -- a name-format validator shared by `CreateParameterGroup` and `CreateSubnetGroup` --
returns that sentinel unconditionally. It's correct for `CreateParameterGroup` (declares the code)
and for every `ClusterName`/`ParameterGroupName` required-field check elsewhere in the package
(`clusters.go`, `parameter_groups.go`, `handler_tags.go` -- all on ops that do declare it), but
wrong for `SubnetGroupName`: three independent, converging signals show the real API has no
format constraint on this field at all (unlike `ClusterName`'s documented "1-20 alphanumeric or
hyphens, first char a letter, no trailing/consecutive hyphens"):
1. `CreateSubnetGroupInput.SubnetGroupName`'s doc says only *"stored as a lowercase string"* --
   no format rule, vs. `ClusterName`'s explicit rule.
2. The SDK's own client-side validator (`validateOpCreateSubnetGroupInput`,
   `validateOpUpdateSubnetGroupInput` in `validators.go`) checks only presence (`v.SubnetGroupName
   == nil`), never shape -- same as `ClusterName`'s validator, so this alone doesn't distinguish
   them, but combined with (1) and (3) it does.
3. Neither op's declared error set has any code that could plausibly carry a "bad name" rejection.

This is a per-call-site fix, per the global-sentinel-map rule: `validateResourceName` itself is
untouched (still correct for `CreateParameterGroup`, its only other caller); only the
`CreateSubnetGroup` call site changes.

**Not the filter-as-key shape.** Both ops are mutates (Create/Update), not Describe/List, so the
"optional filter as must-exist key" pattern and its forced empty-list remedy don't apply; a
mutate-op remedy needs its own evidence (per resourcegroups `CancelTagSyncTask`'s reverted fix).
The evidence here is the three-way convergence above, plus an in-package precedent for the
empty-name sub-case specifically (see below) -- not an inherited List-op remedy.

**Fixed:**

1. **`CreateSubnetGroup`** (`subnet_groups.go:20`): removed the `validateResourceName` call
   entirely -- no code in the op's declared set fits a bad-or-missing name, and the name carries
   no documented/enforced format constraint to reject on. `SubnetIds` validation (`InvalidSubnet`,
   correctly declared) is untouched.
2. **`UpdateSubnetGroup`** (`subnet_groups.go:72`): the empty-`SubnetGroupName` check now types
   `ErrSubnetGroupNotFound` ("SubnetGroupNotFoundFault") instead of `ErrInvalidParameterValue`.
   This exactly mirrors `DeleteSubnetGroup`'s existing, already-correct handling of the identical
   condition four lines below in the same file (`subnet_groups.go:106-108`, untouched) --
   `SubnetGroupNotFoundFault` is confirmed declared for `UpdateSubnetGroup` above.

`validateResourceName`'s only other caller, `CreateParameterGroup` (`parameter_groups.go:12`), is
unchanged and still correct (`InvalidParameterValueException` is declared for that op). No dynamodb
identifier is touched or shared by this fix (`validateResourceName`/`ErrSubnetGroupNotFound` are
unexported to `services/dax`; `services/dynamodb` was read-only-checked and has no reference to
either).

**Pre-existing tests corrected (2, pinned the old wrong behaviour with no note):**

- `TestCreateSubnetGroup`'s "empty name" case (`subnet_groups_test.go`) asserted `wantErr: true`
  for `CreateSubnetGroup("", ...)`. Renamed to "empty name accepted" and now asserts the empty name
  round-trips, with a comment on the real-API evidence.
- `TestCreateSubnetGroupNameValidation` (`subnet_groups_test.go`) asserted `ErrInvalidParameterValue`
  for `"1sg"`/`"sg-"`/`"my--sg"`. Renamed to `TestCreateSubnetGroupNameNotFormatValidated`; all four
  cases (including the previously-valid one) now assert success, with a comment citing the
  deserializer/validator evidence.

**New regression tests:** `TestUpdateSubnetGroup/empty_name` (asserts `ErrSubnetGroupNotFound`),
`TestHandlerErrorMapping/SubnetGroupNotFoundFault_on_UpdateSubnetGroup_with_empty_name` (HTTP-level,
asserts `__type`), `TestHandlerCreateSubnetGroupNameNotFormatValidated` (HTTP-level, asserts 200 +
the malformed name round-trips).

**Per-line neuter, both confirmed:**
- Re-adding `validateResourceName(name, "SubnetGroupName")` to `CreateSubnetGroup` (reverting fix
  1): builds; fails exactly `TestHandlerCreateSubnetGroupNameNotFormatValidated`,
  `TestCreateSubnetGroup/empty_name_accepted`, and 3 of 4 `TestCreateSubnetGroupNameNotFormatValidated`
  subtests, all with `InvalidParameterValueException` errors -- as expected.
- Reverting `UpdateSubnetGroup`'s empty-name check to `ErrInvalidParameterValue` (reverting fix 2):
  builds; fails exactly `TestUpdateSubnetGroup/empty_name` and
  `TestHandlerErrorMapping/SubnetGroupNotFoundFault_on_UpdateSubnetGroup_with_empty_name`, both
  expecting `SubnetGroupNotFoundFault` but getting `InvalidParameterValueException` -- as expected.

Re-ran `cmd/errtargetaudit` after the fix: dax now reports **no class A findings** (down from 2);
the coverage line is unchanged (21/77, 21/21 of dax's own ops -- see explanation above).

Gates: `go test -race -count=1 ./services/dax/...` (pass, including
`./services/dax/dataplane/...`), `golangci-lint run services/dax/...` (0 issues).
