---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: neptune
sdk_module: aws-sdk-go-v2/service/neptune@v1.48.4
last_audit_commit: 087cb59186751418d9d49b88434f13cf214c7609
last_audit_date: 2026-09-04
overall: A            # every previously-open gap this pass either genuinely fixed or re-verified as correct-as-is
                      # 2026-09-04 (gopherstack-12v delete-precondition sweep): swept all 9 Delete* ops against their SDK doc comments. Two real missing-precondition bugs found and fixed: (1) DeleteDBInstance never enforced DeletionProtection -- the field was entirely unmodeled on DBInstance (zero references anywhere), despite being a real member of types.DBInstance/CreateDBInstanceInput/ModifyDBInstanceInput (neptune@v1.48.4 types/types.go:705, "The instance can't be deleted when deletion protection is enabled") and named explicitly in DeleteDBInstance's own doc comment ("or if it has deletion protection enabled"); now modeled end-to-end (Create/Modify/Delete/wire) and enforced with the same ErrInvalidDBInstanceStateFault the sibling only-instance-in-cluster check already used. (2) DeleteGlobalCluster never inspected GlobalClusterMembers at all, despite its own doc comment ("The primary and all secondary clusters must already be detached or deleted first") and DeleteGlobalCluster's own deserializer modeling InvalidGlobalClusterStateFault as a typed error for this exact op -- now rejects deletion while members remain attached. Fixing (2) surfaced a real ghost-row bug as a direct consequence: DeleteDBCluster never cleared a departing member's entry from its global cluster's GlobalClusterMembers list (only RemoveFromGlobalCluster did), so a member cluster deleted directly (not via RemoveFromGlobalCluster) would have permanently blocked its global cluster from ever being deleted after fix (2) landed -- fixed via new detachDeletedClusterFromGlobalCluster, justified directly by the same doc sentence ("detached OR deleted" names deletion as a sufficient alternative to detaching). Separately, a tag-store ghost-row sweep (Create*/AddTagsToResource vs Delete* symmetry, same bug class as the pre-existing DBCluster/DBClusterParameterGroup/DBSubnetGroup/DBClusterSnapshot cascade-clean convention) found DeleteDBParameterGroup, DeleteDBClusterEndpoint, DeleteEventSubscription, and DeleteGlobalCluster all left their resource's tags behind on delete -- all four Create their resource with a deterministic (name-derived) ARN and support AddTagsToResource at create time, so a resource recreated under the same identifier would silently inherit the deleted resource's tags; all four fixed. DeleteDBClusterSnapshot's documented "must be in the available state" precondition was checked and found unenforceable-as-a-bug: every snapshot this backend ever creates is set to "available" synchronously and no code path ever assigns any other status, so the precondition can never actually fire -- same class of reasoning as the existing RebootDBInstance note (a synchronous emulator collapsing an async AWS status window). Not fixed, disclosed as a gap: CreateDBInstanceInput carries a real, optional DBSubnetGroupName member ("A DB subnet group to associate with this DB instance", api_op_CreateDBInstance.go:130) independent from the parent cluster's, but the handler never parses it (DBInstance.DBSubnetGroupName is only ever inherited from the cluster at create time) -- a discarded-parameter bug, not a delete-precondition bug, out of scope for this pass's focus; DeleteDBSubnetGroup's existing in-use check (against clusters, not instances) is a reasonable proxy given that gap, not a bug in its own right. See TestHandler_DeleteDBInstance_DeletionProtectionEnabled, TestDeleteGlobalCluster_MembersAttachedRoundTrip, TestDeleteGlobalCluster_MemberDeletedDirectlyRoundTrip, TestDelete_NoGhostTags.
                      # 2026-07-31 (browser parity pass): RouteMatcher checked only the User-Agent header for the "api/neptune" marker, which a browser cannot set (Fetch spec forbids scripts from setting User-Agent) -- the AWS SDK for JavaScript in a browser puts its SDK identification in X-Amz-User-Agent instead, so every browser dashboard Neptune request (@aws-sdk/client-neptune) fell through unmatched. Also confirmed the marker itself needed case-insensitive matching: the JS SDK's serviceId-derived marker is "api/Neptune" (PascalCase), not aws-sdk-go-v2's lowercase "api/neptune". Fixed via the new pkgs/service.MatchesUserAgentMarker helper, shared with the identical bug class fixed the same pass in mediastoredata/docdb/appsync. Grade held at A: fixed, not deferred.
                      # 2026-08-11 (gopherstack-gt9o NetworkType pass): closed the recorded NetworkType/SupportedNetworkTypes/NetworkTypeNotSupportedFault gap. NetworkType threaded end-to-end for DBCluster (CreateDBCluster/ModifyDBCluster input, IPV4 default, Describe echo) and DBInstance (inherited from parent cluster at create time, no input member of its own -- verified absent from CreateDBInstanceInput/ModifyDBInstanceInput). SupportedNetworkTypes modeled on DBSubnetGroup/OrderableDBInstanceOption but deliberately left empty (no CIDR data to derive it honestly) and NetworkTypeNotSupportedFault deliberately left unwired (no state to detect the real trigger condition) -- see gaps below for the reasoning on both. Snapshot version constant unchanged (1); additive omitempty fields only.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
families:
  DBCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "ClusterCreateTime was hardcoded to a fixed 2024-01-01 literal for every cluster (fixed: real timestamp per creation, including restore paths which previously omitted it and DBClusterResourceID entirely). FailoverDBCluster was a disguised no-op (fixed: real writer/reader promotion via DBClusterMembers.IsClusterWriter, with TargetDBInstanceIdentifier support and InvalidDBClusterStateFault when no reader exists). PromoteReadReplicaDBCluster re-verified this pass against the SDK: its own doc comment on both the operation and its DBClusterIdentifier field says 'Not supported.' -- gopherstack's describe-only echo (no state mutation) is therefore the CORRECT behavior for a genuinely-unsupported op, not a stub; reclassified from gap to ok. NetworkType FIXED this pass: gained on CreateDBCluster/ModifyDBCluster input (neptune@v1.48.4 api_op_CreateDBCluster.go:171/api_op_ModifyDBCluster.go:136, plain *string wire member 'NetworkType') and echoed on Describe; unspecified-on-create defaults to IPV4 per the SDK's documented default (api_op_CreateDBCluster.go:161), matching real AWS always answering a concrete value. Accepted as any string, not validated against IPV4/DUAL (no smithy enum backs it). 2026-08-29 (write-only-state sweep): member-count check against types.DBCluster's own deserializer (awsAwsquery_deserializeDocumentDBCluster, 44 of 44 members enumerated) found GlobalClusterIdentifier -- a real DBCluster response member -- was completely unmodeled: zero struct field, so DescribeDBClusters could never echo it even though the GlobalCluster family already tracks membership relations on the other side (global_clusters.go's own doc comment even names this exact gap: 'real Neptune clusters join via CreateDBCluster's GlobalClusterIdentifier at creation time, which this backend does not model'). Worse, CreateDBClusterInput's real, optional GlobalClusterIdentifier member (api_op_CreateDBCluster.go:129) was entirely unparsed by CreateDBCluster -- discarded input, not just a missing echo. Fixed: DBCluster gained the field (json/xml GlobalClusterIdentifier,omitempty); CreateDBCluster now parses it, requires the named global cluster to already exist (GlobalClusterNotFound otherwise), and attaches the new cluster as a member (writer if the global cluster has no members yet, reader otherwise) via new attachClusterToGlobalClusterLocked. Reciprocal fixes to the write side found by the same sweep: CreateGlobalCluster's SourceDBClusterIdentifier path set the GlobalCluster's own member list but never the source DBCluster's new field (fixed); promoteGlobalClusterWriter's attach-an-unresolved-but-real-cluster path (Failover/SwitchoverGlobalCluster) had the same gap (fixed); RemoveFromGlobalCluster/DeleteGlobalCluster never cleared the field on departing/deleted members (fixed, via new clusterByARNLocked/clusterIdentifierFromARN ARN-to-cluster resolution). See TestCreateDBCluster_JoinsExistingGlobalCluster, TestCreateDBCluster_JoinNonexistentGlobalCluster, TestCreateGlobalCluster_WithSource_SetsMemberClusterField, TestRemoveFromGlobalCluster_ClearsMemberClusterField (wire_field_fixes_test.go). EngineMode (accepted on CreateDBCluster's opts and echoed on every DBCluster response) is NOT a real member of types.DBCluster or CreateDBClusterInput at all under any name (zero grep hits in types.go/api_op_CreateDBCluster.go/api_op_ModifyDBCluster.go) -- an invented field, but DORMANT: no real typed client can ever populate the request side (the field doesn't exist on CreateDBClusterInput to set), and an unrecognized response element is silently skipped by the real XML deserializer's default case, so it costs nothing to a real caller. Not removed this pass (unreachable, and removing it risks disturbing internal test helpers/AddClusterInternal that may reference it) -- flagged here rather than fixed. FIXED 2026-09-04 (gopherstack-12v): DeleteDBCluster never removed a departing global-cluster member from its global cluster's GlobalClusterMembers list (only RemoveFromGlobalCluster did) -- see the top-level 2026-09-04 note above."}
  DBInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "InstanceCreateTime field was entirely absent from the model/wire shape (fixed: added and populated on create). RebootDBInstance intentionally stays a state-preserving op (matches AWS's eventual-consistency behavior for reboot; DescribeDBInstances shows 'available' immediately either way). NetworkType FIXED this pass: CreateDBInstanceInput/ModifyDBInstanceInput carry no NetworkType member of their own (verified against the SDK -- absent from both input structs), matching the doc comment on DBInstance.NetworkType ('Inherited from the DB cluster'); now captured from the parent cluster's NetworkType at instance-create time and echoed on Describe. CreateDBInstance FIXED this pass (gopherstack-uhsb): Engine is a required CreateDBInstanceInput member documented 'Valid Values: neptune', but the handler never read it at all -- any value silently had zero effect since the backend hardcodes DBInstance.Engine to \"neptune\" regardless. Rather than continuing to ignore the field, an explicit Engine value that isn't \"neptune\" is now rejected with InvalidParameterValue (no typed exception exists for this in CreateDBInstance's error switch, so it falls through to the same generic-error path every other unmodeled InvalidParameterValue case already uses) -- same reasoning as the elasticache ApplyImmediately=false precedent: validating and rejecting the one AWS-documented illegal case is more faithful than silently accepting anything. Engine omitted or \"neptune\" is unaffected. FIXED 2026-09-04 (gopherstack-12v): DeletionProtection was entirely unmodeled and unenforced on DeleteDBInstance -- see the top-level 2026-09-04 note above."}
  DBClusterParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: ModifyDBClusterParameterGroup/ResetDBClusterParameterGroup were disguised no-ops -- they validated the group and the Parameters.Parameter.N.* the real client sends, then discarded every value, so DescribeDBClusterParameters always answered empty regardless of what was 'set'. Added a real per-group ParameterValue override store (parameter_catalog.go) seeded against a documented Neptune engine-parameter catalog (neptune_query_timeout, neptune_enable_audit_log, neptune_streams, neptune_result_cache, neptune_dfe_query_engine, neptune_ml_iam_role, neptune_lab_mode, neptune_shard_hash_partitions), enforcing the real static-parameter/pending-reboot ApplyMethod rule and the non-modifiable-parameter rule, with ResetAllParameters and per-parameter reset both wired to real state. DescribeEngineDefaultClusterParameters now returns that catalog instead of an always-empty list. Delete cascades the override store (no ghost rows)."}
  DBParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "Same fix as DBClusterParameterGroup, sharing the catalog/override-store logic in parameter_catalog.go (real Neptune parameter names are shared across both instance- and cluster-level groups). FIXED 2026-09-04 (gopherstack-12v): DeleteDBParameterGroup left its tag-store entry behind on delete (ghost row) -- see the top-level 2026-09-04 note above."}
  DBSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "SupportedNetworkTypes modeled (real StringList wire shape) but never populated -- see the gaps entry below for why."}
  ClusterSnapshot: {wire: ok, errors: ok, state: ok, persist: ok, note: "Multiple real bugs fixed this pass: (1) SnapshotCreateTime/ClusterCreateTime fields were entirely absent from the model; (2) CopyDBClusterSnapshot silently dropped Port/AllocatedStorage/KmsKeyID/IAMDatabaseAuthenticationEnabled/PercentProgress instead of copying them from the source; (3) ModifyDBClusterSnapshotAttribute/DescribeDBClusterSnapshotAttributes were a disguised no-op pair (Modify validated params and discarded them; Describe always returned an empty attribute list) AND Modify's response body omitted the required *Result XML element entirely, which makes the real aws-sdk-go-v2 client fail every call with a smithy.DeserializationError even though gopherstack answered HTTP 200 -- both fixed with a real RestoreAttributeValues store on DBClusterSnapshot, correct list-item wire shape (AttributeValues is a repeated <AttributeValue> list, was a single string), and the correct ValuesToAdd.AttributeValue.N / ValuesToRemove.AttributeValue.N wire param names (was ValuesToAdd.member.N, which a real client never sends, so Modify's add/remove would have silently no-opped forever even after the rest of the fix)."}
  EventSubscription: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "DescribeEvents FIXED this pass (see the top-level Events family below) -- it is dispatched from this family's handler file but is not itself an EventSubscription op, so it is tracked separately. 2026-08-15 (gopherstack-6flj): CustomerAwsId was never modeled (zero grep hits) despite the backend already tracking accountID for ARN construction -- fixed and emitted (CreateEventSubscription now sets it; wire converter carries it as omitempty). FIXED 2026-08-30 (gopherstack-2jj4): CreateEventSubscription never parsed EventCategories at all, see Notes. FIXED 2026-09-04 (gopherstack-12v): DeleteEventSubscription left its tag-store entry behind on delete (ghost row) -- see the top-level 2026-09-04 note above."}
  GlobalCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: ModifyGlobalCluster/FailoverGlobalCluster/SwitchoverGlobalCluster were disguised no-ops (validated the global cluster and returned an unchanged clone). ModifyGlobalCluster's interface signature didn't even accept the new values a caller sent -- now applies DeletionProtection/EngineVersion/NewGlobalClusterIdentifier (rename, including ARN) for real. Failover/Switchover now flip GlobalClusterMembers[].IsWriter to promote TargetDbClusterIdentifier -- when the target already is a tracked member it is promoted directly; when it resolves to a real DB cluster in the account but was never attached (this backend has no separate 'join global cluster' op the way real Neptune's CreateDBCluster-time GlobalClusterIdentifier attachment works), it is attached as the new writer, demoting the prior one; a target this backend cannot resolve at all is left as a no-op rather than erroring, since it cannot distinguish a legitimate not-yet-modeled cross-region secondary from a typo. CreateGlobalCluster/DescribeGlobalClusters/DeleteGlobalCluster/RemoveFromGlobalCluster were already real. 2026-08-15 (gopherstack-6flj): DatabaseName was never modeled anywhere in the service (zero grep hits) despite being a real, optional CreateGlobalClusterInput member -- fixed: threaded from CreateGlobalCluster's form value through the backend and echoed (omitempty) by every global-cluster response op. FailoverState (real, transient in-process failover/switchover record) intentionally left unmodeled -- this backend's Failover/Switchover apply member promotion synchronously with no in-process window to observe, so there is nothing honest to populate it with (same reasoning already applied to RebootDBInstance elsewhere in this file); fabricating a status would invent a transition this backend cannot distinguish. CreateGlobalClusterInput's EngineVersion/DeletionProtection/StorageEncrypted are also silently ignored at create time (only ever settable via ModifyGlobalCluster or derived from an attached source cluster) -- disclosed, not fixed this pass; each carries real validation/interaction surface deserving its own pass rather than a same-session bolt-on. 2026-08-29: the 'this backend has no separate join global cluster op' limitation named above is now FIXED -- CreateDBCluster's GlobalClusterIdentifier member is modeled (see the DBCluster family note above), so a cluster actually can join an existing global cluster as a first-class create-time op now, not just via Failover/Switchover's best-effort attach-as-writer fallback. CreateGlobalCluster/RemoveFromGlobalCluster/DeleteGlobalCluster/promoteGlobalClusterWriter were also all missing the reciprocal write back to the member DBCluster's own (now-existing) GlobalClusterIdentifier field -- fixed, see the DBCluster family note. FIXED 2026-09-04 (gopherstack-12v): DeleteGlobalCluster never rejected a global cluster with attached members, and never cleaned up its own ARN's tag-store entry on delete -- see the top-level 2026-09-04 note above."}
  ClusterEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "DeleteDBClusterEndpoint returned an empty response body; the real DeleteDBClusterEndpointOutput echoes the deleted endpoint's fields as a flat (non-nested) payload and the SDK deserializer hard-fails without a *Result element -- fixed (backend now returns the deleted endpoint; handler renders it under DeleteDBClusterEndpointResult, matching CreateDBClusterEndpointResponse's existing flat-under-Result shape). ModifyDBClusterEndpoint FIXED this pass: it silently ignored StaticMembers.member.N/ExcludedMembers.member.N even though the real API accepts and applies them -- now replaces the respective member list when a non-empty list is supplied (nil vs explicitly-empty is indistinguishable on this wire format, matching CreateDBClusterEndpoint's existing convention for the same two fields). FIXED 2026-09-04 (gopherstack-12v): DeleteDBClusterEndpoint left its tag-store entry behind on delete (ghost row) -- see the top-level 2026-09-04 note above."}
  Tags: {wire: ok, errors: ok, state: ok, persist: ok}
  Events: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: DescribeEvents always returned an empty list -- there was no event log backing this backend at all, so the response was empty regardless of what a caller had actually done (a genuine gap, not a legitimate no-op: AWS's own DescribeEvents surfaces real account activity). Added a bounded per-region event log (events.go, maxEventsLogPerRegion=500) fed by recordEvent calls from the key cluster/instance/snapshot lifecycle mutators (create/delete/start/stop/failover), with SourceIdentifier/SourceType/StartTime/EndTime/Duration/EventCategories filtering matching DescribeEventsInput's real fields (AWS's default 60-minute lookback window is honored when neither StartTime nor Duration is given)."}
  Maintenance: {wire: ok, errors: ok, state: ok, persist: ok, note: "ApplyPendingMaintenanceAction's response body omitted the required ApplyPendingMaintenanceActionResult/ResourcePendingMaintenanceActions element -- same GetElement-hard-fail bug class as ModifyDBClusterSnapshotAttribute and DeleteDBClusterEndpoint above -- fixed (now echoes ResourceIdentifier back). FIXED FURTHER this pass: added a real pending-maintenance-action queue (maintenance.go), keyed by resource ARN -> action name, since real AWS populates this from system-side upgrade/security-patch availability data this backend has no equivalent of; AddPendingMaintenanceActionInternal seeds it for callers/tests the same way AddClusterInternal/AddSnapshotInternal/AddParameterGroupInternal seed their resources. ApplyPendingMaintenanceAction now genuinely mutates CurrentApplyDate/OptInStatus per AWS's immediate/next-maintenance/undo-opt-in semantics (validated as an enum), and DescribePendingMaintenanceActions returns genuinely-queued actions filtered by the db-cluster-id/db-instance-id Filters AWS documents, never emitting an empty ResourcePendingMaintenanceActions entry (matching AWS). Also corrected the DescribePendingMaintenanceActionsResult XML shape while touching these types: it was a flat single-level list wrongly tagging items as <ResourcePendingMaintenanceActions> with bare Action/Description fields; real AWS nests a per-resource <ResourcePendingMaintenanceActions><ResourceIdentifier/><PendingMaintenanceActionDetails><PendingMaintenanceAction>..., now also carrying AutoAppliedAfterDate/CurrentApplyDate/ForcedApplyDate/OptInStatus."}
  StaticCatalog: {status: ok, note: "DescribeDBEngineVersions, DescribeOrderableDBInstanceOptions, DescribeValidDBInstanceModifications -- correctly modeled as static/hardcoded catalog data (not a stub; there is no per-account mutable state for engine version catalogs). DescribeEngineDefault(Cluster)Parameters moved out of this family this pass: they now return the real parameter catalog (see DBParameterGroup/DBClusterParameterGroup above) instead of an always-empty list, which was a genuine gap masquerading as static-catalog behavior -- an empty catalog is not the same thing as a hardcoded non-empty one. FIXED this pass (gopherstack-uhsb): DescribeOrderableDBInstanceOptions took `_ url.Values` and ignored Engine/EngineVersion/DBInstanceClass entirely, so a filtered request always got the full unfiltered catalog back with a 200 -- now genuinely filters the static catalog by each non-empty parameter (no typed exception exists for an unmatched/unknown Engine in this op's error switch, so an empty result set is correct, not an invented error). DescribeValidDBInstanceModifications had two real bugs, also fixed this pass: (1) DBInstanceIdentifier is a required input (api_op_DescribeValidDBInstanceModifications.go) but was ignored -- neither required-ness nor instance existence was checked, so a nonexistent/omitted identifier silently got a 200 with fabricated data instead of the documented DBInstanceNotFound; now validated via the existing DescribeDBInstances existence check. (2) The response was wire-shape-wrong: types.ValidDBInstanceModificationsMessage (neptune@v1.48.4 types/types.go:1608) has exactly one field, `Storage []ValidStorageOptions` (IopsToStorageRatio/ProvisionedIops/StorageSize/StorageType, all doc'd 'Not applicable. In Neptune the storage type is managed at the DB Cluster level.') -- gopherstack was instead emitting a fabricated `ValidProcessorFeatures>AvailableProcessorFeature` list of DB instance classes, an element name that does not exist anywhere in the real deserializer's switch (deserializers.go:23143 only recognizes 'Storage'), so a real client's decoder silently skipped the entire payload via its default-case Skip() and always saw an empty message regardless of what gopherstack sent. Now emits the correctly-named (always-empty) Storage list -- matches Neptune's own 'not applicable' semantics honestly instead of a mislabeled, unreachable fake."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "SupportedNetworkTypes on DBSubnetGroup/OrderableDBInstanceOption is modeled (field exists, real StringList wire shape via xmlSupportedNetworkTypeList) but permanently left empty (nil pointer, omitted from the wire): this backend tracks subnets as opaque ID strings only (no IPv4/IPv6 CIDR data) and the orderable-options catalog is static/hardcoded with no per-instance-class capability source, so there is no honest basis to compute AWS's real derived value -- inventing IPV4/DUAL support a client could filter on would be worse than omitting the field. Not fixable without modeling real subnet CIDR data."
  - "NetworkTypeNotSupportedFault (neptune@v1.48.4 types/errors.go:1417, wire code \"NetworkTypeNotSupported\") is intentionally NOT wired into errors.go's lookup table. Real AWS raises it when a requested NetworkType is incompatible with the target DB subnet group's actual IPv4/IPv6 CIDR support -- this backend has no CIDR data (see SupportedNetworkTypes gap above) to genuinely detect that condition, and inventing a rejection rule would be the more-restrictive-than-AWS bug class this repo explicitly avoids. NetworkType itself is accepted as any string (client-side SDK type is a bare *string, not a smithy enum -- verified: no NetworkType entry in aws-sdk-go-v2/service/neptune/types/enums.go), never validated against IPV4/DUAL."
  - "RestoreDBClusterFromSnapshot/RestoreDBClusterToPointInTime do not accept or echo NetworkType, consistent with their existing minimal option surface (already missing StorageType/HostedZoneID/MasterUsername/etc., a pre-existing gap out of scope for this pass). CreateDBCluster/ModifyDBCluster do carry NetworkType (the SDK input member exists only on these 4 ops; only the 2 implemented ones were wired)."
  - "2026-08-15 (gopherstack-6flj): GlobalCluster.FailoverState (real, transient in-process failover/switchover record) is not modeled. Failover/Switchover apply member promotion synchronously with no in-process window this backend can honestly report a status for -- omitting it is more accurate than fabricating a pending/failing-over/complete value."
  - "2026-08-15 (gopherstack-6flj): CreateGlobalClusterInput's EngineVersion/DeletionProtection/StorageEncrypted are silently ignored at create time (EngineVersion only ever comes from an attached source cluster or a hardcoded default; DeletionProtection is only settable later via ModifyGlobalCluster; StorageEncrypted is only ever derived from a source cluster) -- discarded input, disclosed rather than fixed this pass since each has real validation/interaction surface deserving its own pass."
  - "2026-09-04 (gopherstack-12v): CreateDBInstanceInput.DBSubnetGroupName is a real, optional, per-instance member (api_op_CreateDBInstance.go:130, \"A DB subnet group to associate with this DB instance\") independent from the parent DB cluster's own subnet group, but the handler never parses it -- DBInstance.DBSubnetGroupName is only ever inherited from the cluster at create time. A discarded-parameter bug, not a delete-precondition bug; out of scope for this pass's delete-precondition focus. Consequence: DeleteDBSubnetGroup's in-use check (against DB clusters, not DB instances, despite the SDK doc naming DB instances) is a reasonable proxy given this gap rather than a bug in its own right -- this backend never models an instance-level subnet group independent from its cluster's, so the two checks are currently equivalent."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - The Neptune engine-parameter catalog in parameter_catalog.go (8 parameters) is a documented, representative approximation, not a byte-for-byte mirror of AWS's real DescribeEngineDefaultParameters catalog (which is server-side data, not part of the SDK, and was not independently verified against a live Neptune account this pass) -- functionally correct (real persistence, real validation, real Describe reflection) but the exact parameter set/count may not match AWS's live catalog.
  - GlobalCluster Failover/Switchover member-promotion for a target that is neither an existing member, an ARN, nor a locally-known DB cluster identifier is a silent no-op rather than an error -- real AWS would reject an unresolvable target, but this backend has no "join global cluster" operation to have modeled a genuine not-yet-attached secondary, so it cannot distinguish that case from a typo without one.
leaks: {status: clean, note: "No goroutines, timers, or janitors in this service (still true after this pass's additions -- the new pending-maintenance-action queue and events log are plain maps/slices guarded by the existing coarse b.mu, not background workers). Handler exposes Snapshot(ctx)/Restore(ctx, []byte) with the exact required signatures, delegating straight to InMemoryBackend's own Snapshot/Restore (persistence.go); the compile-time `var _ StorageBackend = (*InMemoryBackend)(nil)` assertion in interfaces.go keeps this from silently drifting. All 9 region-qualified resource collections plus the global (partition-scoped) GlobalCluster table round-trip through backendSnapshot; clusterRoles/tags/parameterOverrides/clusterParameterOverrides/pendingMaintenanceActions/eventsLog (raw maps, not store.Table) persist directly. DeleteDBParameterGroup/DeleteDBClusterParameterGroup now cascade-delete their parameter override entry (parameterOverrides/clusterParameterOverrides) the same way DeleteDBCluster already cascaded instances/endpoints/tags -- no ghost override rows survive a deleted group."}
---

## Notes

Protocol: **query/xml**, single POST with `Action=<OpName>&Version=2014-10-31` form
params (`RouteMatcher` checks `Content-Type: application/x-www-form-urlencoded`,
`api/neptune` present in either `User-Agent` or `X-Amz-User-Agent` -- see
`pkgs/service.MatchesUserAgentMarker` and the 2026-07-31 browser-parity note above --
and `Version=2014-10-31` in the parsed
body -- it does NOT check `Action` itself, so any Neptune-shaped POST is claimed
before dispatch validates the action name). Response root element name is
`<OpName>Response` with an `xmlns="http://rds.amazonaws.com/doc/2014-10-31/"`
attribute (Neptune's API reuses RDS's 2014-10-31 XML namespace verbatim -- this
is not a copy-paste bug, it's how the real service actually answers).

**List wire shape**: Neptune (like RDS) uses *named* list-item elements, not the
generic query-protocol `<member>` wrapper -- e.g. `<DBClusters><DBCluster>...`,
not `<DBClusters><member>...`. Verified against the SDK's generated
`deserializeDocument*List` functions op-by-op for DBCluster, DBInstance,
DBClusterSnapshot, DBSubnetGroup, DBClusterParameterGroup, DBClusterMember,
EventSubscription, GlobalCluster. All matched already; this is NOT the "10 ops
nested one level too deep" bug class RDS hit -- Neptune's response-root nesting
(`<XResponse><XResult><Field>...`) was already correct everywhere audited.

**The real, recurring bug class this pass**: several "void" response types
looked like legitimate no-payload query-protocol responses (an empty
`<OpResponse xmlns="..."/>`) but the real AWS operation is NOT void -- it wraps
a `<OpResult>` element the SDK's deserializer unconditionally fetches via
`decoder.GetElement("OpResult")`, and a *missing* element there is a hard
`smithy.DeserializationError` on the client side, not a zero-valued struct.
This is easy to miss by eyeballing gopherstack's own code (an empty struct
"looks" like a correct void response) and easy to miss by grepping ("stub
hunting" flags real logic that ends in an empty return as correct per
parity-principles rule #4) -- the only way to catch it is checking the SDK's
per-op Output struct AND its `HandleDeserialize` to see whether `GetElement`
is called unconditionally. Confirmed broken (now fixed) for
`ModifyDBClusterSnapshotAttribute`, `ApplyPendingMaintenanceAction`, and
`DeleteDBClusterEndpoint`. Confirmed genuinely void (left alone, verified no
`GetElement` call in the op's `HandleDeserialize`) for: `DeleteDBSubnetGroup`,
`DeleteDBClusterParameterGroup`, `DeleteDBParameterGroup`,
`AddTagsToResource`, `RemoveTagsFromResource`, `AddRoleToDBCluster`,
`RemoveRoleFromDBCluster`. A `Test_SDKRoundTrip_*` suite
(`handler_sdk_roundtrip_test.go`) now drives the real `aws-sdk-go-v2/service/neptune`
client against an `httptest.Server`-wrapped handler for the three fixed ops,
specifically to catch this class of bug in future regressions -- string-matching
the raw XML body (as most other tests in this package do) cannot catch it,
since the offending response *looks* fine as a string.

**Error code strings are NOT `TypeName` minus a mechanical suffix.** Neptune's
generated API model is inconsistent about keeping "Fault" on the wire `<Code>`
value: `DBClusterNotFoundFault` keeps it (`"DBClusterNotFoundFault"`), but
`DBInstanceNotFoundFault` drops it (`"DBInstanceNotFound"`). Verified every
code string against each fault type's `ErrorCode()` method in
`aws-sdk-go-v2/service/neptune/types/errors.go` AND cross-checked against the
actual per-operation `EqualFold` switch in `deserializers.go` (the type exists
doesn't mean every op error-switches on it). Fixed 9 wrong entries in
`neptuneErrorCode()`: `DBInstanceNotFound(Fault)`,
`DBInstanceAlreadyExists(Fault)`, `DBSubnetGroupAlreadyExists(Fault)`,
`SubscriptionNotFound(Fault)`, `SubscriptionAlreadyExist(Fault)`,
`InvalidDBInstanceState(Fault)`. The **cluster-parameter-group family is the
most surprising case**: there is no distinct
`DBClusterParameterGroupAlreadyExists`/`...NotFound` fault for
`CreateDBClusterParameterGroup`/`DeleteDBClusterParameterGroup`/
`ModifyDBClusterParameterGroup`/`ResetDBClusterParameterGroup` -- they all
reuse the plain (non-cluster) `DBParameterGroupAlreadyExists`/
`DBParameterGroupNotFound` codes. A `DBClusterParameterGroupNotFound` fault
does exist in the model, but only for *other* ops that reference a cluster's
parameter group by name (`CreateDBCluster`/`ModifyDBCluster`/
`RestoreDBCluster*`) -- this backend doesn't validate that reference, so it
never needs that code. Getting this wrong doesn't break the HTTP
status/message a client sees, but it silently breaks `errors.As(err,
&types.DBInstanceNotFoundFault{})`-style typed error matching in any caller
that relies on it, which is invisible to string-matching-based tests (exactly
why every test in this package that asserts on an error code was re-verified
against the corrected strings, not just left passing against the old wrong
ones).

**Timestamps** are emitted as ISO8601/RFC3339 strings (`ClusterCreateTime`,
`SnapshotCreateTime`, `InstanceCreateTime`), matching `smithytime.ParseDateTime`
on the client side -- Neptune's query/xml protocol does NOT use epoch-seconds
numbers (that's a json/rest-json protocol convention; `pkgs/awstime.Epoch` does
not apply here). `ClusterCreateTime` was previously a hardcoded
`"2024-01-01T00:00:00Z"` literal shared by every cluster ever created -- fixed
to `time.Now().UTC().Format(time.RFC3339)` at actual creation time (also
applied to the two restore paths, which previously omitted the field --
and `DBClusterResourceID` -- entirely).

**Looks-wrong-but-correct**: `RebootDBInstance` and `PromoteReadReplicaDBCluster`
returning state-preserving responses without registering a transient
"rebooting"/"available" status flip is intentional-ish, not a stub in the
disguised-no-op sense -- AWS's own reboot is asynchronous and a `Describe`
issued moments later typically already shows `available` again, so a
synchronous emulator collapsing that window to a no-op is a reasonable
approximation. `PromoteReadReplicaDBCluster` is listed as a gap above because
Neptune doesn't really have RDS-style standalone read-replica instances to
promote (its cross-region replication is entirely global-cluster-based), so
what this op should even mutate in this backend is genuinely unclear without
more real-AWS testing -- flagged for the next pass rather than guessed at.

## 2026-07-23 pass: closing the 6 gaps / 2 deferred items from 2026-07-12

All six gaps and both deferred items from the prior audit were re-examined
against the SDK and either genuinely fixed (real state mutation + correct
wire shape) or reclassified to `ok` with evidence -- none were reclassified
on a no-stub basis alone.

**PromoteReadReplicaDBCluster is not a stub -- the SDK says the op itself is
"Not supported."** `api_op_PromoteReadReplicaDBCluster.go`'s doc comment on
both `(*Client).PromoteReadReplicaDBCluster` and
`PromoteReadReplicaDBClusterInput.DBClusterIdentifier` reads exactly "Not
supported." -- this is RDS's shared Aurora API surface carried into Neptune's
model without a Neptune-specific implementation behind it. gopherstack's
existing describe-only echo (no state mutation) is therefore the *correct*
emulation of a genuinely-inert real operation, not a disguised no-op; this
was previously flagged as an open gap because the correct behavior was
"genuinely unclear without more real-AWS testing" -- reading the SDK's own
doc comment settles it without needing a live account.

**The parameter-value-store gap (DBParameterGroup/DBClusterParameterGroup)
required a real catalog, not just a map.** Modify/Reset validated the group
and parsed `Parameters.Parameter.N.{ParameterName,ParameterValue,ApplyMethod}`
(confirmed against `awsAwsquery_serializeDocumentParametersList` in the SDK's
serializers.go -- the list member wrapper is `Parameter`, not the generic
query-protocol `member`) but then discarded every value, so
DescribeDBParameters/DescribeDBClusterParameters and
DescribeEngineDefaultParameters/DescribeEngineDefaultClusterParameters always
answered with an empty list. Fixed with a per-group `ParameterValue` override
store (`region|groupName` -> parameter name -> value, following the same
plain-nested-map convention as `clusterRoles`/`tags`) merged against a new
canonical parameter catalog (`parameter_catalog.go`) on every Describe. The
catalog also let two more real AWS rules become enforceable for the first
time: static parameters (`neptune_enable_audit_log`, `neptune_streams`,
`neptune_shard_hash_partitions`) require `ApplyMethod=pending-reboot`
(`ApplyMethod=immediate` is rejected, matching real AWS's
static/dynamic-ApplyMethod compatibility rule), and
`neptune_shard_hash_partitions` -- a system-controlled parameter -- rejects
modification outright (`IsModifiable=false`). AWS's exact default parameter
catalog is server-side data, not part of the SDK, so the 8-parameter set
modeled here is a documented representative approximation (see `deferred`
above), not a verified byte-for-byte mirror.

**GlobalCluster Modify/Failover/Switchover needed real member-list mutation,
not just an accepted request.** `ModifyGlobalCluster`'s *interface signature*
previously took only a `globalClusterID` -- it could not have applied a
caller's `DeletionProtection`/`EngineVersion` even if it wanted to, since
those values were never passed in. Fixed by adding `GlobalClusterModifyOptions`
(also covering `NewGlobalClusterIdentifier`, i.e. rename, which real
AWS's `ModifyGlobalClusterInput` also accepts) and real
`GlobalClusterMembers[].IsWriter` promotion in Failover/Switchover.
Real AWS requires `TargetDbClusterIdentifier` to already be an attached
secondary; this backend has no equivalent of Neptune's actual attachment
mechanism (a DB cluster joins a global database via `CreateDBCluster`'s
`GlobalClusterIdentifier` parameter, which this backend's `CreateDBCluster`
does not model), so a target that resolves to a real local DB cluster but
isn't yet a tracked member is attached as the new writer as the closest
achievable analogue, and a target this backend cannot resolve at all is a
no-op rather than a rejection (see `deferred`).

**ModifyDBClusterEndpoint's StaticMembers/ExcludedMembers gap was a pure
parsing omission.** The real API's `StaticMembers`/`ExcludedMembers` list
params serialize with the generic query-protocol `member` wrapper
(`awsAwsquery_serializeDocumentStringList`, confirmed against
serializers.go), same as most other Neptune list params elsewhere in this
codebase already handle via `parseMemberList` -- the handler simply never
called it for these two fields. Fixed.

**The pending-maintenance-action queue and the event log are the two
"structurally missing feature" gaps** (as opposed to "wrong wire shape" or
"discards input") -- there was no backing store for either at all, so
`DescribePendingMaintenanceActions`/`DescribeEvents` were always empty
regardless of real activity. Both AWS features are populated by data this
in-memory emulator doesn't generate on its own (system-side
upgrade/patch-availability data for maintenance actions; real account event
history for events), so both now have `AddPendingMaintenanceActionInternal`-
style seeding (maintenance.go) or are fed by `recordEvent` calls placed at
the point of real state changes this backend already performs (events.go:
cluster/instance create+delete, cluster start+stop+failover, snapshot
create) -- genuine, queryable, filterable state rather than a disguised
no-op in either direction.

**2026-08-22 (gopherstack-bahs) -- RouteMatcher's read-failure branch was
finally safe to flip, but only after a second real bug underneath it.**
gopherstack-3a8t found neptune was one of only 2 of 17 body-reading
RouteMatchers that already gate on a body-independent signal
(`service.MatchesUserAgentMarker(r.Header, "api/neptune")`, verified
against the pinned `neptune@v1.48.4/api_client.go:640` `AddSDKAgentKeyValue`
call) before ever reading the body -- so, unlike the other 15 (which are
form-urlencoded services indistinguishable from each other except by the
body's own `Action`/`Version`), claiming on a `ReadBody` failure here could
not misroute a sibling service's oversized request. That earlier pass tried
exactly that (`return false` -> `return true`) and reverted it: neptune's
`ExtractOperation`/`ExtractResource`/`Handler()` all called `r.ParseForm()`
directly, and `net/http`'s own `ParseForm` (`net/http/request.go`) sets
`r.PostForm` to a non-nil empty `url.Values` even when the underlying read
fails (`if r.PostForm == nil { r.PostForm = make(url.Values) }` runs
unconditionally after the failed `parsePostForm` call). The telemetry
wrapper (`pkgs/telemetry/echo_wrapper.go`) calls the observer's
`ExtractOperation` before `Handler()` runs, so that first `ParseForm` call
correctly saw the read error -- but the *second* call, inside `Handler()`,
found `r.PostForm` already non-nil and skipped re-parsing entirely,
returning `nil` with an empty form. `Handler()` then saw `Action == ""` and
answered `MissingAction` (400) instead of `InternalFailure` (500).

Verified this diagnosis two ways before touching anything: read
`net/http`'s `ParseForm`/`parsePostForm` source directly (confirmed the
non-nil-empty-`PostForm`-on-error caching), and reproduced it concretely by
applying only the matcher flip on top of the unmigrated `ParseForm` call
sites -- `TestHandler_OversizedBodySurfacesInternalFailure` failed with
`MissingAction`, matching the prior pass's report exactly.

**The fix**: migrated all three call sites
(`ExtractOperation`/`ExtractResource`/`Handler()`) from `r.ParseForm()` to
`httputils.ReadBody` + `url.ParseQuery`, mirroring elasticache's own
pattern from the 3a8t pass. `httputils.ReadBody` was already hardened (that
same pass) to cache a read failure on `r.Body` the same way it already
cached a success, so every one of these three calls -- however many run
before `Handler()` -- now sees the identical real error instead of a
silently-emptied form on the second-and-later calls. With that landmine
gone, `RouteMatcher`'s `ReadBody`-failure branch was changed from `return
false` to `return true`: safe unconditionally at that point in the function
since the `MatchesUserAgentMarker` check immediately above it has already
established ownership. `MatchPriority` untouched.

Proof: `TestHandler_OversizedBodySurfacesInternalFailure` in
`handler_oversized_body_test.go` drives a real neptune SDK client through
`service.NewRegistry`/`service.NewServiceRouter`, confirmed failing
pre-fix with `UnknownError` (matcher unchanged) and, at the
matcher-only-flip intermediate step, with `MissingAction` (ParseForm
caching bug); passes now with `InternalFailure`.
`TestHandler_NormalSizedBodyStillRoutes` is the regression guard for a
normal-sized request still routing and succeeding. Hand-reverted
`services/neptune/handler.go` to the pre-fix `git show HEAD:...` version
and back, confirmed byte-identical via `md5sum`. Gates run clean: `go build
./...`, `go vet`, `gofmt -l` (empty), `go test -race ./services/neptune/...`,
`golangci-lint run ./services/neptune/...` (0 issues after adding an
`unknownOp` constant elasticache already uses, to keep `goconst` happy with
the third `"Unknown"` literal the migration introduced).

**2026-08-29 (wire-key sweep, gopherstack-6flj/21my class):** Write-only-state
sweep of the DBCluster/GlobalCluster families, member counts derived from
types.DBCluster's own deserializer case list (44 of 44) rather than trusting
this file's prior "wire: ok" grade.

- Found and fixed: DBCluster.GlobalClusterIdentifier (real response member)
  was completely unmodeled -- no struct field at all, so it could never be
  echoed regardless of what the GlobalCluster family tracked on its own side.
  CreateDBClusterInput.GlobalClusterIdentifier (real, optional request
  member) was also entirely unparsed -- discarded input. Both fixed
  end-to-end, including the reciprocal write-back CreateGlobalCluster/
  RemoveFromGlobalCluster/DeleteGlobalCluster/promoteGlobalClusterWriter all
  needed once the field existed to write into. See DBCluster/GlobalCluster
  family notes above and TestCreateDBCluster_JoinsExistingGlobalCluster et
  al. in wire_field_fixes_test.go.
- Found, disclosed, not fixed: DBCluster.EngineMode is an invented field
  (zero grep hits anywhere in types.go or either Create/ModifyDBClusterInput)
  -- classified DORMANT, since no real typed client can ever set the request
  side and an unrecognized response element is silently skipped by the real
  deserializer.
- Not reached this pass: DBInstance/ClusterSnapshot/DBSubnetGroup/
  DBClusterParameterGroup/DBParameterGroup/ClusterEndpoint/EventSubscription/
  Events/Maintenance/StaticCatalog families -- PARITY.md already documents
  recent, detailed field-diffed passes for these (grade A, last_audit_date
  2026-08-11) and this session's time budget went to DBCluster/GlobalCluster
  instead of re-verifying already-recent work across the full 161-op surface.

- **ERROR path re-verified against `cmd/errcodeaudit`'s near-miss sweep (this session)**:
  the tool flags 12 `errors.go` sentinel literals (`DBClusterNotFound`,
  `DBClusterAlreadyExists`, `DBSubnetGroupNotFound`, `DBClusterParameterGroupAlreadyExists`,
  `DBClusterSnapshotNotFound`, `DBClusterSnapshotAlreadyExists`, `DBClusterEndpointNotFound`,
  `DBClusterEndpointAlreadyExists`, `SubscriptionAlreadyExists`, `GlobalClusterNotFound`,
  `GlobalClusterAlreadyExists`, `InvalidDBInstanceStateFault`) as absent from neptune's real
  type/deserializer set. All are **tool false positives**: every backend error routes
  through the single `handleOpError`→`neptuneErrorCode()` mapping table in handler.go, which
  already carries the SDK-verified code for each sentinel (documented inline with the exact
  Fault-suffix trap this campaign targets — e.g. `DBInstanceNotFound` genuinely has no
  `Fault` suffix while `DBClusterNotFoundFault` does) and is the sole path to the wire; the
  `errors.go` literal is only ever used for `errors.Is` identity. No new fix needed.

## 2026-08-29 -- exhaustive indexed-list/filter-key request-parameter sweep

Every request-side indexed-list or filter-key parse site enumerated against
its own operation's serializer in `neptune@v1.48.4` (a different surface from
the 2026-08-15 response-wrapper-key pass above: this is what the handler
*reads off incoming requests*, not what it *writes into responses*).

**30 of 30 call sites checked, all resolved by hand** (small enough surface
that scripting wasn't needed): 9 `parseMemberList` call sites, 6
`parseNeptuneFilterValue(s)` call sites, and 15 more through five small
fixed-key helpers (`parseTagEntries` x8, `parseTagKeyMembers` x1,
`parseSubnetIDMembers` x2, `parseSourceIDMembers` x1, `parseParameterEntries`
x3) -- each helper's hardcoded key verified once against its serializer,
since every call site shares the same literal key.

**Two real bugs found, both fixed:**

1. **Wrong inner element name (shape 3).** `ModifyEventSubscription` and
   `DescribeEvents` both read `EventCategories.member.N`. The real serializer
   (`awsAwsquery_serializeDocumentEventCategoriesList`, serializers.go:4971-4972)
   wraps each entry in `EventCategory`, not the generic `member` -- so a real
   client's `EventCategories` was silently dropped on both ops. Notably,
   the sibling `SourceIds` field on `CreateEventSubscription` was *already*
   fixed to `SourceIds.SourceId.N` (see the comment on `parseSourceIDMembers`)
   while this identically-shaped field was not -- confirms the "don't infer
   from a sibling fix" warning cuts both ways.
2. **Wrong cardinality, list read as scalar (shape 2).** `parseNeptuneFilterValue`
   read only `Filters.Filter.N.Values.Value.1`; the real serializer
   (`awsAwsquery_serializeDocumentFilterValueList`, serializers.go:5012-5013)
   makes `Values` a repeated `Value` list of arbitrary length, so a filter
   with 2+ values behaved like a 1-value filter and silently excluded
   matches on every value after the first. Affected `DescribeDBClusters`
   (engine/engine-version/status), `DescribeDBInstances` (db-cluster-id),
   and `DescribePendingMaintenanceActions` (db-cluster-id/db-instance-id).
   Renamed to `parseNeptuneFilterValues` (returns `[]string`); `DBClusterFilters`
   fields and the two other filter parameters widened to `[]string`, matched
   via `slices.Contains`.

**Everything else already correct**, including several call sites carrying
an inline comment citing the exact serializer line that had *already* fixed
this same bug class in an earlier pass (`SourceIds.SourceId.N`,
`StaticMembers`/`ExcludedMembers.member.N`, `SubnetIds.SubnetIdentifier.N`) --
those are why this pass found only 2 new bugs rather than the higher count
an untouched service would show.

**FIXED 2026-08-30 (gopherstack-2jj4)**, previously left alone as a missing
feature: `CreateEventSubscription` never parsed `EventCategories` from the
request at all (real, optional input member, confirmed on
`CreateEventSubscriptionInput`) -- a parameter never read, not a wrong key.
`CreateEventSubscriptionInput`'s own serializer
(`awsAwsquery_serializeOpDocumentCreateEventSubscriptionInput`,
serializers.go:5967-5972) calls the identical
`awsAwsquery_serializeDocumentEventCategoriesList` used by
`ModifyEventSubscription` -- confirmed on this op's own serializer, not
inferred from that sibling -- so the wire key is the same
`EventCategories.EventCategory.N` shape (a wrapped list, not a bare
`member.N`), not a bare-vs-wrapped mismatch requiring different handling.
`handleCreateEventSubscription` now parses it via the same `parseMemberList`
helper and threads it through a widened `CreateEventSubscription` backend
signature (`sourceIDs, eventCategories []string`) into
`EventSubscription.EventCategoriesList`. Proven via
`TestCreateEventSubscription_EventCategories`
(wire_field_fixes_indexedlist_test.go), confirmed failing pre-fix (empty
list) via a real SDK client asserting the decoded
`EventSubscription.EventCategoriesList` on both the immediate response and a
subsequent `DescribeEventSubscriptions`.

Tests: `wire_field_fixes_indexedlist_test.go`, all three driving the real
typed SDK client and asserting on the decoded response. Confirmed failing
against unmodified code first (`git stash` of just the fixed source files,
run, `git stash pop`).

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run`
all clean for `services/neptune`; repo-wide `go vet` clean except a
pre-existing, uncommitted route53 signature mismatch from a concurrently
active agent working in that directory (not touched here).

## gopherstack-21my per-item field pass (2026-08-31, paired with s3control)

Confirmed protocol from `deserializers.go` before starting:
`awsAwsquery_deserializeOpError*` functions everywhere in the pinned SDK, so
neptune is `awsAwsquery_` -- the case-only class is live.

Prior passes (2026-08-15/29, documented above) already ran a real per-item
member-count check for `DBCluster` (44/44 against
`awsAwsquery_deserializeDocumentDBCluster`) and a wrapping-shape sweep
across the whole named-list-item convention this service uses. This pass
extended the same member-count technique to the families PARITY.md had
previously graded `wire: ok` from older, less exhaustive audits rather than
a byte-for-byte deserializer diff: `DBInstance` (54 real members),
`DBClusterSnapshot` (20), `DBSubnetGroup` (7), `DBClusterParameterGroup`
(4), `Parameter` (9), `EventSubscription` (10), `DBClusterEndpoint` (10).

**BUG (fixed), hard-failure-adjacent: `DBInstance` never emitted the
top-level `DbInstancePort` member.** `types.DBInstance` (neptune@v1.48.4
types/types.go:694) carries `DbInstancePort *int32` as a field genuinely
distinct from the nested `Endpoint.Port` -- the real deserializer
case-switches on `"DbInstancePort"` separately from `"Endpoint"`
(`awsAwsquery_deserializeDocumentDBInstance`). `xmlDBInstance`
(handler_db_instances.go) only ever emitted the port nested under
`Endpoint>Port`; the top-level field was absent from the struct entirely
(not a naming bug -- the element was never there), so every real client's
`DBInstance.DbInstancePort` decoded `nil` regardless of the tracked port.
Since it's a pointer field in the client's type, this is the "pointer
decodes nil" signature this campaign has called out as more serious than a
blank string -- any caller dereferencing it without a nil check panics.
Fixed: `toXMLInstance` now also sets the new `DBInstancePort` field
(`xml:"DbInstancePort"`) from the same `inst.Port` value already used for
`Endpoint>Port` -- single shared conversion function, so `CreateDBInstance`,
`DescribeDBInstances`, and every other DBInstance-emitting op get it at
once; no sibling disagreement possible here since there's only one
converter. Test: `TestCreateDBInstance_DbInstancePort`
(wire_field_fixes_test.go), creates a cluster+instance via the real SDK
client, asserts `DbInstancePort` is non-nil and equals 8182 on both the
`CreateDBInstance` response and a subsequent `DescribeDBInstances`;
confirmed failing pre-fix (`require.NotNil` failed, field was nil).

**Everything else checked this pass came back clean at the per-item
layer**: `DBClusterSnapshot` (14 of 20 real members emitted; the other 6 --
`AvailabilityZones`, `LicenseModel`, `MasterUsername`,
`SourceDBClusterSnapshotArn`, `StorageType`, plus `DBClusterSnapshot`
itself already known -- have no backing field anywhere on the
`DBClusterSnapshot` backend model, genuine unmodeled gaps, not
tracked-but-dropped), `DBSubnetGroup` (6 of 7; `SupportedNetworkTypes` is
the pre-existing documented gap above), `DBClusterParameterGroup` (4 of 4),
`Parameter` (9 of 9), `EventSubscription` (10 of 10), `DBClusterEndpoint`
(10 of 10, including confirming the odd-looking but genuinely correct
per-item element name `DBClusterEndpointList` -- verified against
`awsAwsquery_deserializeDocumentDBClusterEndpointList`, which really does
case-switch on that literal string for each list member, not `member` or
`DBClusterEndpoint`).

No case-only mismatch found this pass. No sibling (singular vs. list)
disagreement found for any family checked -- neptune's DBInstance/
DBClusterSnapshot/DBClusterParameterGroup/Parameter/EventSubscription/
DBClusterEndpoint all route Create and Describe/List through the same
shared XML struct and, in most cases, the same converter function, which
structurally forecloses the "list emits a subset of what singular emits"
trap this campaign found repeatedly elsewhere.

Not reached this pass: `GlobalCluster` (already had its own 2026-08-29
member-count pass), `ClusterEndpoint`'s request-parsing side (already swept
2026-08-29), `Events`/`Maintenance`/`StaticCatalog` families.

Gates: see the combined s3control+neptune gate note in
`services/s3control/PARITY.md`'s matching 2026-08-31 entry -- both services
build/vet/test/lint clean; repo-wide `go vet ./...` currently fails only in
`services/ec2` (a different agent's concurrent in-progress, uncommitted
work, confirmed via `git status --short services/ec2`), not touched here.

## 2026-08-31 -- gopherstack-6flj/21my: ops never named in this file

Computed the queue directly: every `List*`/`Describe*` op in
`neptune@v1.48.4`'s `api_op_*.go` files whose literal name never appears
anywhere in this PARITY.md. Seven such ops: `DescribeDBClusterEndpoints`,
`DescribeDBClusterParameterGroups`, `DescribeDBClusterSnapshots`,
`DescribeDBParameterGroups`, `DescribeDBSubnetGroups`,
`DescribeEventCategories`, `ListTagsForResource`. Protocol re-confirmed:
`awsAwsquery_` throughout `deserializers.go` (query/XML,
`strings.EqualFold` case-folded), matching this file's existing Notes.

Four of the seven (`DescribeDBClusterEndpoints`, `DescribeDBClusterParameterGroups`,
`DescribeDBClusterSnapshots`, `DescribeDBSubnetGroups`) already had a
per-item member-count sweep against their family types (`DBClusterEndpoint`
10/10, `DBClusterParameterGroup` 4/4, `DBClusterSnapshot` 14/20 tracked,
`DBSubnetGroup` 6/7 tracked) recorded under the "gopherstack-21my per-item
field pass (2026-08-31, paired with s3control)" section above -- that pass
checked the type, this pass independently re-verified the wrapper key each
of these four *operations* emits it under (`DBClusterEndpoints`,
`DBClusterParameterGroups`, `DBClusterSnapshots`, `DBSubnetGroups`, all
confirmed against `awsAwsquery_deserializeOpDocumentDescribe*Output`) --
all four correct, no new findings.

`DescribeDBParameterGroups`, `DescribeEventCategories`,
`ListTagsForResource` had not been swept at either layer before. Full
wrapper-key and per-item sweep:

- `DescribeDBParameterGroups`: wraps `DBParameterGroups` (correct); item
  type `DBParameterGroup` has exactly 4 real members
  (`awsAwsquery_deserializeDocumentDBParameterGroup`,
  deserializers.go:15751-15790) -- `DBParameterGroupArn`,
  `DBParameterGroupFamily`, `DBParameterGroupName`, `Description`, all 4
  emitted correctly (`xmlDBParameterGroup`, handler_parameter_groups.go).
  Clean.
- `DescribeEventCategories`: wraps `EventCategoriesMapList` (correct); item
  wrapper `EventCategoriesMap` (correct) with `SourceType`+`EventCategories`
  (a nested `EventCategory`-wrapped list, correct) -- all confirmed against
  `awsAwsquery_deserializeDocumentEventCategoriesMapList`/
  `...EventCategoriesMap`/`...EventCategoriesList`. This op's response is
  static/hardcoded (4 source types), which is legitimate static-catalog
  behavior, same class already documented for `DescribeDBEngineVersions`
  elsewhere in this file -- not a stub. Clean.
- `ListTagsForResource`: wraps `TagList` (correct); item `Tag` has 2
  members, `Key`/`Value` (`awsAwsquery_deserializeDocumentTag`,
  deserializers.go:22863-22876), both emitted correctly via the shared
  `pkgs/tags.KV` type. Clean.

**No bugs found in this batch.** No wrapper-key mismatch, no per-item field
mismatch, no transposition, no case-only mismatch, no element emitted that
isn't a real member, no hard decode error. All seven ops in this queue are
genuinely clean at both layers -- consistent with this service's very
recent (same-day) deep per-item sweep already having covered most of the
adjacent surface (`DBInstance.DbInstancePort` fix, etc., documented above).

No web pages fetched this pass (SDK lookups went through the pinned module
cache only).

Gates: `go build ./services/neptune/...`, `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/neptune/...` (pass, no new
tests -- no bug found to write a regression test for), `golangci-lint run
./services/neptune/...` (0 issues). No source changes this pass.

## 2026-09-07 -- gopherstack-ucus: CreateDBInstance dropped DBSubnetGroupName

`handleCreateDBInstance` never read `DBSubnetGroupName` off the wire, so a
caller-supplied value was silently discarded and every instance reported
its cluster's subnet group instead, even when the request explicitly asked
for a different one.

`api_op_CreateDBInstance.go`'s doc comment for the field ("A DB subnet
group to associate with this DB instance. If there is no DB subnet group,
then it is a non-VPC DB instance.") does not itself document
inherit-from-cluster as the default for an omitted value -- so the
cluster-inheritance behavior already in this backend is this backend's own
reasonable interpretation (every Neptune instance belongs to a cluster),
not something asserted by the SDK doc. The real, doc-confirmed defect was
narrower: an *explicit* value being ignored outright. Verdict: bug is
"ignoring an explicit value", not "inheriting when omitted" -- the
omitted-value behavior is unchanged by this fix.

Fix: `DBInstanceCreateOptions` gained a `DBSubnetGroupName` field, the
handler now parses it off the form (`vals.Get("DBSubnetGroupName")`), and
`CreateDBInstance` validates it against the existing subnet-group store
(`ErrSubnetGroupNotFound`, wire code `DBSubnetGroupNotFoundFault`,
confirmed present in `deserializeOpErrorCreateDBInstance`) before letting
an explicit non-empty value win over the cluster's; an omitted value still
falls back to the cluster's, as before. The cluster-inherit branch was
extracted into `instanceClusterInherited` to keep `CreateDBInstance` under
the gocognit threshold.

Not touched: `xmlDBInstance.DBSubnetGroupName` still serializes as a bare
`<DBSubnetGroup>name</DBSubnetGroup>` string, while real
`types.DBInstance.DBSubnetGroup` is a full `*DBSubnetGroup` struct (name,
description, VPC ID, subnets). Resolving the stored name into that full
shape on `DescribeDBInstances`/`CreateDBInstance` responses is a separate,
larger gap this pass did not take on -- flagging for a follow-up issue.

Regression tests (`handler_db_instances_test.go`,
`TestHandler_CreateDBInstance_DBSubnetGroupName`, HTTP-handler level):
`explicit_name_overrides_cluster`, `omitted_defaults_to_cluster`,
`nonexistent_name_rejected`. Reverting the fix while keeping the tests
failed 2 of 3 (`explicit_name_overrides_cluster`,
`nonexistent_name_rejected`; `omitted_defaults_to_cluster` passed both
before and after, since that path was never broken).

Gates: `go test -race -count=1 ./services/neptune/...` (pass);
`golangci-lint run ./services/neptune/...` (0 issues). `go test -count=1
./pkgs/persistence/...` run read-only, unaffected -- no field was added to
a persisted struct (`DBInstanceCreateOptions` is a transient parameter
struct, never part of the persistence DTO registry).
