---
service: docdb
sdk_module: aws-sdk-go-v2/service/docdb@v1.51.4
last_audit_commit: 04b49136
last_audit_date: 2026-08-29
overall: A            # 2026-07-31 pass: 3 real feature gaps closed (GlobalCluster members, real events log, real pending-maintenance queue), 2 disguised no-op bugs fixed (ResetDBClusterParameterGroup, CreateEventSubscription arg-swap), 1 wire-field gap fixed (EventSubscription response), 2 cosmetic gaps closed
                      # gopherstack-6flj (2026-08-15): 5 derived wire-field fixes (InstanceCreateTime + 5 snapshot fields copied from tracked source-cluster/source-snapshot state + CopyDBClusterSnapshot's discarded Tags/CopyTags), 2 fabricated wire fields removed (DBClusterSnapshot's bogus DBClusterArn, GlobalCluster's bogus SourceDBClusterIdentifier), 9 real gaps disclosed (see gaps: list) -- see the pass's own Notes section at the end of this file for full detail. Grade held at A.
                      # 2026-08-29 (wrapper-key/request-direction sweep, gopherstack-6flj follow-up): checked the REQUEST direction, not just response wire shape, for every List/Describe/Get op's real Input struct members (filter/sort/time-range/pagination/precondition), since a prior "wire: ok" here only ever meant response-side. FOUND AND FIXED 4 real dropped-filter bugs: DescribeDBClusters/DescribeDBInstances/DescribeGlobalClusters/DescribePendingMaintenanceActions all silently ignored their real, AWS-documented "Filters" member (db-cluster-id / db-instance-id, the only Describe*Input.Filters values these 4 ops document as actually supported -- the other 12 Describe*/List*/Get* ops in this service document Filters as "This parameter is not currently supported" in the pinned SDK itself, so their no-op status is correct AWS behavior, not a bug). DescribeDBInstances additionally had a WRONG mechanism masking the bug: the handler read a `DBClusterIdentifier` query param that does not exist anywhere on the real DescribeDBInstancesInput struct, so no real client's cluster-scoping ever reached the backend by any path. New services/docdb/filters.go implements the real wire key format `Filters.Filter.N.Name`/`Filters.Filter.N.Values.Value.M` (confirmed against docdb@v1.51.4 serializers.go's awsAwsquery_serializeDocumentFilterList/FilterValueList -- NOT `Values.member.M`, the format services/rds's own filter parser uses, which appears to itself be wrong relative to the real wire; left untouched, out of this pass's scope). Proven via 4 new Test_SDKRoundTrip_*_Filters tests driving the real typed aws-sdk-go-v2/service/docdb client, each including a non-matching record the filter must EXCLUDE.
                      # 2026-07-31 (browser parity pass): RouteMatcher checked only the User-Agent header for the "api/docdb" marker, which a browser cannot set (Fetch spec forbids scripts from setting User-Agent) -- the AWS SDK for JavaScript in a browser puts its SDK identification in X-Amz-User-Agent instead, so every browser dashboard DocDB request (@aws-sdk/client-docdb) fell through unmatched. Also confirmed the marker itself needed case-insensitive matching: the JS SDK's serviceId-derived marker is "api/DocDB" (PascalCase), not aws-sdk-go-v2's lowercase "api/docdb". Fixed via the new pkgs/service.MatchesUserAgentMarker helper, shared with the identical bug class fixed the same pass in mediastoredata/neptune/appsync. Grade held at A: fixed, not deferred.
ops:
  # DBCluster family
  CreateDBCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed prior pass: AvailabilityZones + VpcSecurityGroupIds request field names were wrong (see families.DBCluster). This pass: now records a real activity-log event on create (see Events family)."}
  DescribeDBClusters: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed prior pass: AvailabilityZones response was over-nested (extra <Name> child). FIXED 2026-08-29 (request direction): Filters (db-cluster-id, the only documented supported filter) was parsed nowhere -- every real client's Filter was silently dropped and every cluster returned regardless of it. See filters.go/filterDBClusters."}
  DeleteDBCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: now records a real activity-log event on delete"}
  ModifyDBCluster: {wire: ok, errors: ok, state: ok, persist: ok}
  StopDBCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: now records a real activity-log event"}
  StartDBCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: now records a real activity-log event"}
  FailoverDBCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: now records a real activity-log event"}
  RestoreDBClusterFromSnapshot: {wire: ok, errors: ok, state: ok, persist: ok}
  RestoreDBClusterToPointInTime: {wire: ok, errors: ok, state: ok, persist: ok}
  # DBInstance family
  CreateDBInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed prior pass: error codes were DBInstanceNotFoundFault/DBInstanceAlreadyExistsFault, real wire codes have no Fault suffix. Prior pass: now records a real activity-log event on create. THIS PASS (gopherstack-6flj): types.DBInstance.InstanceCreateTime (real, optional member per awsAwsquery_deserializeDocumentDBInstance) was declared on no field at all -- unlike its DBCluster.ClusterCreateTime sibling, which already tracked+emitted the equivalent. Added DBInstance.InstanceCreateTime, stamped at CreateDBInstance time, same pattern as ClusterCreateTime."}
  DescribeDBInstances: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS (gopherstack-6flj): now emits InstanceCreateTime (see CreateDBInstance). Disclosed, not fixed -- 7 further real, optional types.DBInstance members with zero backing state anywhere in this backend: CertificateDetails/DbiResourceId/LatestRestorableTime/PendingModifiedValues/PerformanceInsightsEnabled/PerformanceInsightsKMSKeyId/StatusInfos (Performance Insights, read-replica status, and a stable synthetic resource-id scheme are all distinct unimplemented features, not wire-shape gaps). FIXED 2026-08-29 (request direction): two bugs. (1) Filters (db-cluster-id, db-instance-id, both real documented filters) was parsed nowhere. (2) the handler's cluster-scoping was sourced from a `DBClusterIdentifier` query param that does not exist at all on the real DescribeDBInstancesInput struct (confirmed absent in api_op_DescribeDBInstances.go) -- a real client's request never carries that key by any name, so this was a dead mechanism, not a working-but-wrong one. Both replaced by filters.go/filterDBInstances reading the real Filters wire member."}
  DeleteDBInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: now records a real activity-log event on delete"}
  ModifyDBInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  RebootDBInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  # DBSubnetGroup family
  CreateDBSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed prior pass: SubnetIds request field name was SubnetIds.member.N, real is SubnetIds.SubnetIdentifier.N -- every subnet ID from a real client was silently dropped"}
  DescribeDBSubnetGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDBSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ModifyDBSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed prior pass: same SubnetIds field-name bug as Create"}
  # DBClusterParameterGroup family (AWS reuses the plain RDS DBParameterGroup fault codes here, not DBClusterParameterGroup...Fault)
  CreateDBClusterParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed prior pass: error codes were DBClusterParameterGroupNotFoundFault/AlreadyExistsFault, real wire codes are DBParameterGroupNotFound/DBParameterGroupAlreadyExists (no Cluster, no Fault)"}
  DescribeDBClusterParameterGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDBClusterParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ModifyDBClusterParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed prior pass: Parameters request field name was Parameters.member.N.ParameterName, real is Parameters.Parameter.N.ParameterName -- every parameter from a real client was silently ignored (disguised no-op hidden by the wrong field name). Already had a real per-group ParameterValue override store (map[string]string on DBClusterParameterGroup) -- confirmed NOT a disguised no-op unlike the sibling ResetDBClusterParameterGroup bug found this pass."}
  CopyDBClusterParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ResetDBClusterParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: was a disguised no-op -- validated the group and returned an unchanged clone without ever touching pg.Parameters, so ResetAllParameters=true or a per-parameter Parameters list from a real client silently did nothing. Now parses ResetAllParameters + Parameters.Parameter.N.ParameterName (reusing the same wire member name ModifyDBClusterParameterGroup uses) and genuinely clears the requested override(s) back to the engine default."}
  DescribeDBClusterParameters: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: ApplyMethod field added (was entirely absent from the wire response -- cosmetic gap closed, AWS's Parameter shape always carries it). this pass (constraint-parameter audit): also fixed -- the Source query filter (docdb@v1.51.4 api_op_DescribeDBClusterParameters.go:58-60, \"return only parameters for a specific source\") was read nowhere; every call returned every parameter regardless of Source=user/system requested. Now filters by exact match against each Parameter's own Source field."}
  DescribeEngineDefaultClusterParameters: {wire: ok, errors: n/a, state: ok, persist: n/a, note: "this pass: ApplyMethod field added, same fix as DescribeDBClusterParameters"}
  # DBClusterSnapshot family
  CreateDBClusterSnapshot: {wire: fixed, errors: ok, state: ok, persist: ok, note: "prior pass: now records a real activity-log event on create. FIXED THIS PASS (gopherstack-6flj), 2 bugs: (1) response wrongly emitted a bare DBClusterArn -- confirmed against awsAwsquery_deserializeDocumentDBClusterSnapshot that the real types.DBClusterSnapshot has NO such member (only DBClusterSnapshotArn); a real client's generated deserializer silently drops unknown elements, so this was over-emission, not a functional bug -- removed from the wire struct only, the backend field itself is retained for CopyDBClusterSnapshot's own internal use. (2) 5 real, backend-already-tracked-on-the-source-cluster members were never copied onto the snapshot at all: AvailabilityZones/KmsKeyId/MasterUsername/Port/ClusterCreateTime. Derived from the source DBCluster record at creation time (same derive-from-already-tracked-state class as this issue's prior passes)."}
  DescribeDBClusterSnapshots: {wire: ok, errors: ok, state: ok, persist: ok, note: "THIS PASS (gopherstack-6flj): reflects the CreateDBClusterSnapshot/CopyDBClusterSnapshot wire fixes. Disclosed, not fixed -- VpcId (real, resolvable only via an extra DBSubnetGroup lookup through the source cluster's DBSubnetGroupName, not attempted this pass) and StorageType (real, but no storage-tiering feature modeled at all). this pass (constraint-parameter audit): checked IncludePublic/IncludeShared -- both real filters, both currently unenforced. Judged structurally unobservable, not fixed: this is a single-account emulator with no cross-account snapshot visibility to reveal, and DBClusterSnapshotAttribute/ModifyDBClusterSnapshotAttribute track a snapshot's restore-attribute values but nothing here models a second account whose own DescribeDBClusterSnapshots call would need to see this account's public/shared snapshots. Every snapshot this account can already see is already returned by default (it's always the owner), so the filters have no observable effect to get wrong."}
  DeleteDBClusterSnapshot: {wire: ok, errors: ok, state: fixed, persist: ok, note: "this pass: now records a real activity-log event on delete. FIXED 2026-09-05 (ghost-row-after-delete sweep): left the snapshotAttributes side-table entry (region|DBClusterSnapshotIdentifier-keyed, see ModifyDBClusterSnapshotAttribute) behind on delete -- a recreated snapshot under the same user-chosen identifier inherited the deleted snapshot's restore-attribute grants (an access-control artefact, same class as the elasticsearch vpcAccess finding in 6806b0f10). Now calls the new snapshotAttributesDelete helper alongside the existing tags cleanup."}
  CopyDBClusterSnapshot: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "prior pass: copy previously omitted a fresh SnapshotCreateTime (left zero-valued) -- now stamps the copy's own creation time. FIXED THIS PASS (gopherstack-6flj), a real discarded-input bug: the request's CopyTags (\"Set to true to copy all tags from the source cluster snapshot to the target\") and Tags members were parsed by neither the handler nor the backend at all, so a real client's CopyTags=true request was a silent no-op -- the copy always ended up with zero tags. Now reads both; an explicit Tags value takes precedence over CopyTags when both are given (the SDK doc comment states no precedence rule for this combination, so this is an interpretation, not a confirmed AWS rule -- disclosed as such). Also added the missing SourceDBClusterSnapshotArn response member (real, populated from the source snapshot's own ARN) and the same 5 source-derived fields CreateDBClusterSnapshot gained (copied from the source SNAPSHOT here, not the cluster, since Copy has no direct cluster reference)."}
  DescribeDBClusterSnapshotAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  ModifyDBClusterSnapshotAttribute: {wire: ok, errors: ok, state: ok, persist: ok}
  # EventSubscription family
  CreateEventSubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed prior pass: error codes were SubscriptionNotFoundFault/SubscriptionAlreadyExistFault, real wire codes are SubscriptionNotFound/SubscriptionAlreadyExist (no Fault). FIXED this pass, two bugs: (1) the handler passed sourceIDs/eventCategories to Backend.CreateEventSubscription in the wrong positional order (the backend signature is (eventCategories, sourceIDs)), so a real client's SourceIds silently came back as EventCategoriesList and vice versa -- invisible to every pre-existing test since none checked both lists in one request; (2) Enabled was accepted on the wire but never parsed/stored/echoed -- now defaults to true (AWS's default for a new subscription) when unspecified and is a real, mutable field."}
  DescribeEventSubscriptions: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: response now carries EventCategoriesList/EventSubscriptionArn/Enabled/CustomerAwsId/SubscriptionCreationTime, all previously entirely absent from xmlEventSubscription (see families.EventSubscription)"}
  DeleteEventSubscription: {wire: ok, errors: ok, state: ok, persist: ok}
  ModifyEventSubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "this pass: Enabled is now a real, wire-visible mutation (was silently dropped, same gap as Create)"}
  AddSourceIdentifierToSubscription: {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveSourceIdentifierFromSubscription: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeEventCategories: {wire: ok, errors: n/a, state: ok, persist: n/a}
  DescribeEvents: {wire: ok, errors: n/a, state: ok, persist: ok, note: "FIXED this pass: previously always returned an empty event list (no real event log was modeled at all). Added a bounded per-region event log (events_log.go, maxEventsLogPerRegion=500) fed by recordEvent calls from the key cluster/instance/snapshot lifecycle mutators (create/delete/stop/start/failover), with SourceIdentifier/SourceType/StartTime/EndTime/Duration/EventCategories filtering matching DescribeEventsInput's real fields (AWS's default 60-minute lookback window honored when neither StartTime nor Duration is given). Mirrors the already-completed neptune service's identical fix."}
  # GlobalCluster family
  CreateGlobalCluster: {wire: fixed, errors: ok, state: ok, persist: ok, note: "prior pass: SourceDBClusterIdentifier is now resolved (as an ARN or a bare identifier looked up in the caller's region) and, when it names a real cluster, added as the initial writer GlobalClusterMember. FIXED THIS PASS (gopherstack-6flj): the response wrongly echoed a bare SourceDBClusterIdentifier -- confirmed against awsAwsquery_deserializeDocumentGlobalCluster that the real types.GlobalCluster response type has NO such member (it exists only on CreateGlobalClusterInput, the request). A real client's generated deserializer silently drops unknown elements, so this was over-emission, not a functional bug -- removed from the wire struct only; the backend's GlobalCluster.SourceDBClusterID field is retained (used internally for the initial-member bootstrap already described above)."}
  DescribeGlobalClusters: {wire: ok, errors: ok, state: ok, persist: ok, note: "prior pass: GlobalClusterMembers now reflects real membership instead of always answering an empty list. THIS PASS (gopherstack-6flj): reflects the CreateGlobalCluster fabricated-field fix. Disclosed, not fixed -- 4 further real, optional types.GlobalCluster members with zero backing state: DatabaseName (SDK doc comment gives no docdb-specific semantics to derive from), FailoverState (only populated during an in-progress switchover/failover; every mutation in this backend completes synchronously, so there is never an honest non-empty value), GlobalClusterResourceId (a stable synthetic immutable resource-id scheme, not modeled), TagList (global clusters are not wired into the generic per-ARN tags store the way DBCluster/DBInstance/DBClusterSnapshot are). FIXED 2026-08-29 (request direction): Filters (db-cluster-id -- the doc comment names it that even though it targets the global cluster's own identifier, not a member DBCluster) was parsed nowhere. See filters.go/filterGlobalClusters."}
  DeleteGlobalCluster: {wire: ok, errors: ok, state: ok, persist: ok}
  ModifyGlobalCluster: {wire: ok, errors: ok, state: ok, persist: ok}
  FailoverGlobalCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: TargetDbClusterIdentifier now genuinely promotes a member to writer (or attaches a resolvable-but-not-yet-tracked real cluster as the new writer, demoting the prior one) via promoteGlobalClusterWriter -- previously a pure status-flip no-op with respect to membership"}
  SwitchoverGlobalCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: same real member-promotion fix as FailoverGlobalCluster (this backend has no failure window distinguishing the two operations' data-loss guarantees, so both share promoteGlobalClusterWriter)"}
  RemoveFromGlobalCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: DbClusterIdentifier (accepted as ARN or bare identifier) now genuinely deletes the matching GlobalClusterMember -- previously a pure no-op since no member list existed to remove from"}
  # Tags
  ListTagsForResource: {wire: ok, errors: n/a, state: ok, persist: ok}
  AddTagsToResource: {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveTagsFromResource: {wire: ok, errors: n/a, state: ok, persist: ok}
  # Misc/static
  DescribeDBEngineVersions: {wire: ok, errors: n/a, state: ok, persist: n/a}
  DescribeOrderableDBInstanceOptions: {wire: ok, errors: n/a, state: n/a, persist: n/a, note: "FIXED this pass: the static 4-row catalog (docdb only has one Engine value across 2 EngineVersions x 2 DBInstanceClasses) previously ignored the Engine/EngineVersion/DBInstanceClass request filters entirely (handler took `_ url.Values`), so a filtered request always got back all 4 rows with a 200 instead of the narrowed (possibly empty) set a real client would see. Now genuinely filters the catalog by each non-empty parameter; no typed exception exists for an unknown Engine in this op's error switch (awsAwsquery_deserializeOpErrorDescribeOrderableDBInstanceOptions is default-only), so an unmatched filter correctly yields an empty list, not an invented error."}
  DescribeCertificates: {wire: ok, errors: n/a, state: n/a, persist: n/a}
  ApplyPendingMaintenanceAction: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass: previously validated params but never checked whether the action was actually queued, and always answered an empty PendingMaintenanceActionDetails regardless of OptInType. Added a real per-resource-ARN pending-action queue (pending_maintenance.go) with AddPendingMaintenanceActionInternal to seed it (mirroring AWS's own system-side upgrade/patch-availability data this backend has no equivalent of), enforcing immediate/next-maintenance/undo-opt-in semantics for real against CurrentApplyDate/OptInStatus. Applying an action never queued for a resource is a harmless no-op (matches AWS's own opt-in semantics), not an error and not a fabricated entry. Mirrors the already-completed neptune service's identical fix."}
  DescribePendingMaintenanceActions: {wire: ok, errors: n/a, state: ok, persist: ok, note: "FIXED this pass: previously always returned an empty list; now reflects the real queue (see ApplyPendingMaintenanceAction), filtered by ResourceIdentifier when given, never emitting an entry with an empty PendingMaintenanceActionDetails (matches AWS). FIXED 2026-08-29 (request direction): Filters (db-cluster-id, db-instance-id) was parsed nowhere, so a real client's Filter never narrowed the ResourceIdentifier-keyed queue. filters.go/filterPendingMaintenanceActions extracts each entry's ARN-embedded identifier for comparison since ResourceIdentifier is always a full ARN."}
families:
  DBCluster: {status: ok, note: "3 confirmed wire bugs fixed prior pass (response AvailabilityZones over-nesting; AvailabilityZones/VpcSecurityGroupIds request field names). This pass: added real activity-log event recording (create/delete/stop/start/failover) feeding the now-real DescribeEvents -- core state machine unchanged and still real (status transitions, deletion-protection guard, final-snapshot-on-delete, param/subnet group FK checks)."}
  DBInstance: {status: ok, note: "error-code bug fixed prior pass: DBInstanceNotFoundFault/DBInstanceAlreadyExistsFault -> DBInstanceNotFound/DBInstanceAlreadyExists (no Fault suffix). This pass: added real activity-log event recording (create/delete). CreateDBInstance/ModifyDBInstance/DeleteDBInstance/RebootDBInstance state mutation and DBClusterMember/writer derivation (GetClusterMembers) remain real."}
  DBClusterParameterGroup: {status: ok, note: "prior pass fixed the DBParameterGroup...-not-DBClusterParameterGroup...-Fault error-code family and the ModifyDBClusterParameterGroup Parameters.Parameter.N wire-field-name bug. THIS PASS found and fixed a second disguised no-op in the same family: ResetDBClusterParameterGroup validated the group and returned an unchanged clone without ever touching pg.Parameters -- neither ResetAllParameters=true nor a per-parameter Parameters list from a real client did anything. Now genuinely clears the requested override(s). Also closed the cosmetic ApplyMethod-field gap on DescribeDBClusterParameters/DescribeEngineDefaultClusterParameters."}
  DBSubnetGroup: {status: ok, note: "2 bugs fixed prior pass: SubnetIds.member.N vs SubnetIds.SubnetIdentifier.N field-name bug; DBSubnetGroupAlreadyExistsFault vs DBSubnetGroupAlreadyExists asymmetric Fault-suffix bug. No changes this pass -- re-verified as correct-as-is."}
  DBClusterSnapshot: {status: ok, note: "wire shapes and error codes verified correct prior pass, no changes needed there. THIS PASS: added real activity-log event recording (create/delete) and fixed CopyDBClusterSnapshot's missing fresh SnapshotCreateTime (cosmetic gap, now closed)."}
  EventSubscription: {status: ok, note: "error-code bug fixed prior pass (SubscriptionNotFoundFault/SubscriptionAlreadyExistFault -> no-Fault, singular \"Exist\"). THIS PASS found and fixed two real bugs: (1) a genuine sourceIDs/eventCategories argument-order swap in handleCreateEventSubscription's call into the backend -- both are []string so nothing type-checked it away, and no pre-existing test exercised both lists in the same request to catch it; (2) xmlEventSubscription entirely omitted EventCategoriesList/EventSubscriptionArn/Enabled/CustomerAwsId/SubscriptionCreationTime -- a real client reading back the categories or ARN it just set on Create always saw them silently dropped even though the backend tracked EventCategories correctly internally. Enabled is now a real, request-accepted, backend-stored, wire-echoed field on both Create and Modify (previously accepted on neither end)."}
  GlobalCluster: {status: ok, note: "FIXED this pass, closing the prior pass's flagged gap: types.GlobalCluster.GlobalClusterMembers now has a real backing field (GlobalClusterMember: DBClusterArn/IsWriter/Readers/SynchronizationStatus). CreateGlobalCluster attaches a resolvable SourceDBClusterIdentifier as the initial writer; FailoverGlobalCluster/SwitchoverGlobalCluster genuinely promote TargetDbClusterIdentifier via promoteGlobalClusterWriter (attaching a resolvable-but-not-yet-tracked real cluster as the new writer when it isn't already a member, matching the already-completed neptune service's identical precedent); RemoveFromGlobalCluster genuinely deletes the matching member. A target this backend cannot resolve at all (neither an existing member, an ARN, nor a known local cluster identifier) is left as a no-op rather than erroring, for the same reason neptune's precedent gives: this backend has no separate \"join global cluster\" operation (real DocDB clusters join via CreateDBCluster-time GlobalClusterIdentifier attachment, not modeled here, matching neptune) to have modeled a genuine not-yet-attached secondary, so it cannot distinguish that case from a typo."}
  Tags: {status: ok, note: "AddTagsToResource/RemoveTagsFromResource/ListTagsForResource verified real (region-scoped ARN keying via regionFromARN, upsert-by-key semantics). Wire shape (TagList>Tag, flat Key/Value) matches awsAwsquery_deserializeDocumentTagList exactly. No changes this pass."}
  ClusterEndpoint: {status: n/a, note: "VERIFIED this pass, not a gap: real Amazon DocumentDB has NO cluster-endpoint API at all (no CreateDBClusterEndpoint/ModifyDBClusterEndpoint/DeleteDBClusterEndpoint/DescribeDBClusterEndpoints anywhere in aws-sdk-go-v2/service/docdb@v1.48.11 -- confirmed by listing every api_op_*.go file in the module). This is an RDS/Neptune-only feature this campaign's task description generically mentioned for the RDS-cluster family, but DocDB's own API surface genuinely does not have it. gopherstack correctly has zero cluster-endpoint code for this service; adding any would be inventing an op that doesn't exist on the real wire."}
gaps:
  - "CHECKED 2026-09-07 (gopherstack-z1sd triage), found NOT A BUG: DBClusterSnapshot.Status is set to statusAvailable synchronously in both CreateDBClusterSnapshot and CopyDBClusterSnapshot (db_cluster_snapshots.go) and never any other value -- this backend does not even declare a 'creating'/'copying' status constant for snapshots (grepped models.go/store.go: only statusAvailable and statusDeleting exist, and statusDeleting is a DBCluster-only state). Per this package's own leaks: note, there are no goroutines/tickers anywhere, so there is no async window in which an intermediate status could ever be observed -- unreachable by construction, not a tracked-but-unemitted value. Same reasoning already on record for the sibling neptune service's identical situation (neptune/PARITY.md's DeleteDBClusterSnapshot precondition note, gopherstack-12v: 'every snapshot this backend ever creates is set to \"available\" synchronously and no code path ever assigns any other status') and for gopherstack-h3th/gopherstack-9ojs/gopherstack-0c1r precedent (synchronous emulator collapsing an async AWS status window). Recording here since this service had not previously disclosed it."
  # gopherstack-6flj pass (2026-08-15): disclosed, not fabricated. Each is a
  # real, optional response member with zero backing state anywhere in this
  # backend -- adding a hardcoded/guessed value would be exactly the
  # fabrication parity-principles #1 forbids, and omitempty makes a
  # present-but-always-empty field byte-identical on the wire to an absent
  # one, so modelling them as always-empty would also be zero-effect churn.
  - "DBCluster: AssociatedRoles/CloneGroupId/DbClusterResourceId/EarliestRestorableTime/IOOptimizedNextAllowedModificationTime/LatestRestorableTime/MasterUserSecret/NetworkType/PercentProgress/ServerlessV2ScalingConfiguration/StorageType -- IAM role association, Secrets-Manager-managed credentials, IO-optimized storage tiering, dual-stack networking, and DocDB Serverless v2 are all distinct unimplemented features with no backend state to derive from. CHECKED 2026-09-07 (gopherstack-didn, following the rds twin gopherstack-uao2/1cjz that closed the identical gap in that service): ReplicationSourceIdentifier/ReadReplicaIdentifiers remain dead scaffolding for an unbuilt feature -- confirmed NOT a mechanical port of the rds fix, the two SDKs genuinely diverge here. Both fields are real on docdb's own DBCluster (docdb@v1.51.4 types/types.go:260 ReplicationSourceIdentifier *string; :243 ReadReplicaIdentifiers []string; doc comments read 'Contains the identifier of the source cluster if this cluster is a secondary cluster' and 'Contains one or more identifiers of the secondary clusters that are associated with this cluster' respectively), but unlike rds -- whose CreateDBClusterInput takes ReplicationSourceIdentifier directly (api_op_CreateDBCluster.go:812) -- docdb's CreateDBClusterInput has NO such member at all (grepped api_op_CreateDBCluster.go and every serializer: zero request-side hits; ReplicationSourceIdentifier appears only in the response deserializer, deserializers.go:10265). docdb also has no PromoteReadReplicaDBCluster operation whatsoever (no api_op_PromoteReadReplicaDBCluster.go; the SDK's only 'Promote' hit anywhere is FailoverGlobalCluster's own doc comment). Both fields' 'secondary cluster' wording ties them to Global Clusters, not to an Aurora-style direct replica-cluster create path: the real mechanism that populates them is CreateDBCluster-time GlobalClusterIdentifier attachment (joining an existing global cluster as a non-writer secondary) -- which this file already discloses, twice, as deliberately unmodeled (the GlobalCluster family note above and the unresolvable-Failover/Switchover-target gap below), matching the already-completed neptune service's identical precedent. Building that attachment path now, as a side effect of porting rds's single-flat-field fix, would be materially larger scope than uao2's rds change (a new create-time parameter plus real Global Cluster member wiring, not a mechanical port) and would contradict rather than close this file's own already-recorded scope decision. NOT FIXED this pass; no .go changes made. CreateDBCluster's own declared error list (deserializeOpErrorCreateDBCluster) does include DBClusterNotFoundFault, but with no ReplicationSourceIdentifier parameter on the wire to validate, there is nothing for that fault to guard here."
  - "DBInstance: CertificateDetails/DbiResourceId/LatestRestorableTime/PendingModifiedValues/PerformanceInsightsEnabled/PerformanceInsightsKMSKeyId/StatusInfos -- Performance Insights and read-replica status are unimplemented features; DbiResourceId needs a stable synthetic resource-id scheme this pass did not design."
  - "DBClusterSnapshot: VpcId (resolvable via an extra DBSubnetGroup lookup through the source cluster's DBSubnetGroupName -- plausible but not attempted this pass) and StorageType (no storage-tiering feature modeled)."
  - "DBSubnetGroup: SupportedNetworkTypes (dual-stack/IPv4-only support, unmodeled)."
  - "Parameter (DescribeDBClusterParameters/DescribeEngineDefaultClusterParameters): AllowedValues/MinimumEngineVersion -- real members, but this pass found no authoritative source (SDK doc comments give no enumerated values) for the correct per-parameter content of the static built-in parameter catalog (clusterParameterDefaults). Guessing plausible-looking values (e.g. \"enabled,disabled\" for a boolean param) would be exactly the invention parity-principles #1 forbids."
  - "Certificate (DescribeCertificates): CertificateArn -- real member with a well-known real-AWS ARN format (arn:aws:rds:<region>::cert:<identifier>), but no in-repo precedent (checked services/rds, which has no DescribeCertificates at all) confirms it, so left disclosed per this issue's derive-or-disclose rule rather than reconstructed from memory."
  - "GlobalCluster: DatabaseName/FailoverState/GlobalClusterResourceId/TagList -- see DescribeGlobalClusters note above."
  - "RESOLVED 2026-08-29, refining the prior framing: of the 16 Describe*/List* ops with a request-side Filters member, only 4 (DescribeDBClusters, DescribeDBInstances, DescribeGlobalClusters, DescribePendingMaintenanceActions) document an actually-supported filter Name in the pinned SDK's own Input doc comments -- all 4 are now fixed, see their ops: entries and filters.go. The other 12 ops' Filters doc comment reads verbatim 'This parameter is not currently supported' in docdb@v1.51.4 (DescribeCertificates, DescribeDBClusterParameterGroups, DescribeDBClusterParameters, DescribeDBClusterSnapshots, DescribeDBEngineVersions, DescribeDBSubnetGroups, DescribeEngineDefaultClusterParameters, DescribeEventCategories, DescribeEventSubscriptions, DescribeEvents, DescribeOrderableDBInstanceOptions, ListTagsForResource) -- their Filters being a no-op in gopherstack is therefore correct AWS behavior, not a gap, and implementing filter-matching for them would be inventing behavior real AWS itself does not have."
deferred:
  - GlobalCluster member-promotion for a Failover/Switchover target that is neither an existing member, an ARN, nor a locally-known DB cluster identifier is a silent no-op rather than an error -- real AWS would reject an unresolvable target, but this backend has no "join global cluster" operation to have modeled a genuine not-yet-attached secondary (same documented precedent as the already-completed neptune service), so it cannot distinguish that case from a typo without one.
leaks: {status: clean, note: "no goroutines, no time.After/NewTicker/Tick anywhere in the package (still true after this pass's additions -- the new pending-maintenance-action queue and events log in pending_maintenance.go/events_log.go are plain maps guarded by the existing single lockmetrics.RWMutex, not background workers); backend is a synchronous in-memory store, Snapshot/Restore correctly delegate through Handler for cli.go's setupPersistence registration. eventsLog is bounded per region (maxEventsLogPerRegion=500, oldest entries trimmed) so it cannot grow unbounded in a long-lived process. Both new maps round-trip through backendSnapshot (persistence.go) alongside the pre-existing Tags map -- verified by TestPersistenceRoundTrip_NewState. pendingMaintenanceActions/eventsLog are deliberately NOT cascade-cleared on cluster/instance/snapshot delete: an activity-log event must remain visible after its source resource is gone (that's the point of an activity log, matching AWS's own event-retention behavior), and a queued maintenance action against a since-deleted resource is inert (never returned to anyone querying by the now-nonexistent resource identifier) rather than a live leak -- same precedent as the already-completed neptune service."}
---

## Notes

Protocol: query/XML (`Version=2014-10-31`), single POST with `Action=` form param, same
family as RDS and Neptune (all three descend from a shared Smithy model lineage). Response
root element is `<{Action}Response>` with a required `<{Action}Result>` child wrapping the
payload for every op that returns data -- verified every response type in handler.go carries
this (`xml:"...Result>Field"` or a `Result` struct tagged `xml:"...Result"`), so no response
is missing the `*Result` wrapper the SDK's `decoder.GetElement("...Result")` unconditionally
requires (the neptune/rds bug class the prior audit was specifically asked to check for).

**Prior-pass bug class (fixed, re-verified this pass): AWS's inconsistent wire-code naming
across DocDB's own resource families.** Three sub-patterns, all confirmed directly against
`deserializers.go`'s `awsAwsquery_deserializeOpError*` switches (never trust the Go SDK type
name -- the `Fault` suffix is a Go naming convention, not necessarily what's on the wire):
(1) most DocDB-native resources (DBCluster, DBClusterSnapshot, GlobalCluster) keep the
`Fault` suffix on the wire; (2) DBInstance and one DBSubnetGroup case drop it (asymmetric
even within DBSubnetGroup itself); (3) DBClusterParameterGroup operations reuse the
RDS-inherited plain `DBParameterGroupNotFound`/`DBParameterGroupAlreadyExists` codes, and
EventSubscription similarly uses bare `SubscriptionNotFound`/`SubscriptionAlreadyExist`
(singular "Exist", no Fault). No new instances of this bug class found this pass.

**Prior-pass bug class (fixed, re-verified this pass): wrong request member-element
names.** `AvailabilityZones`, `VpcSecurityGroupIds`, `SubnetIds`, and `Parameters` (on
`ModifyDBClusterParameterGroup`) each use a resource-specific XML list member name rather
than the generic `member` most other DocDB lists use. Getting the member name wrong means
`url.Values.Get(key)` never finds the value under any key the parser tries, so the field
silently parses as empty/nil with no error raised anywhere. `ResetDBClusterParameterGroup`
reuses the same `Parameters.Parameter.N.ParameterName` wire shape (confirmed via
`awsAwsquery_serializeDocumentParametersList`, shared with Modify) -- the new
`parseDBClusterParameterNames` helper added this pass reads it correctly from the start.

**This pass's dominant bug class: disguised no-ops and missing response fields, not wire
member-name mistakes.** Two ops (`ResetDBClusterParameterGroup`,
`CreateEventSubscription`'s sourceIDs/eventCategories argument order) validated their inputs
correctly and returned a plausible-looking 200 OK while silently doing nothing (or the wrong
thing) with real caller-supplied data -- both invisible to `rr.Code == 400`-style or
single-field `Contains` assertions, which is exactly why parity-principles rule #3 requires
SDK-driven round-trip checks rather than trusting green unit tests alone. One family
(`EventSubscription`'s response shape) was missing wire fields entirely
(`EventCategoriesList`/`EventSubscriptionArn`/`Enabled`/`CustomerAwsId`/
`SubscriptionCreationTime`) despite the backend already tracking the underlying data
correctly -- a pure serialization gap, not a state-machine bug.

**Three real feature gaps closed this pass, mirroring the already-completed neptune
service's identical fixes for the same DocDB/Neptune/RDS-family operations:**
`GlobalCluster.GlobalClusterMembers` (real member tracking via `global_clusters.go`'s
`promoteGlobalClusterWriter`), `DescribeEvents` (real bounded per-region event log via
`events_log.go`), and `ApplyPendingMaintenanceAction`/`DescribePendingMaintenanceActions`
(real per-resource-ARN pending-action queue via `pending_maintenance.go`, seeded for tests
via `AddPendingMaintenanceActionInternal` the same way `AddDBClusterInternal` et al. seed
other resources). None of these are goroutines/tickers -- all are plain maps guarded by the
existing coarse `lockmetrics.RWMutex`, matching the pkgs-catalog locking rule.

**Verified NOT a gap:** DocDB has no cluster-endpoint API at all in the real SDK (unlike
RDS/Neptune) -- confirmed by enumerating every `api_op_*.go` file in
`aws-sdk-go-v2/service/docdb@v1.48.11`. gopherstack correctly has zero code for this feature
in the docdb service; this was independently field-diffed this pass, not assumed.

## gopherstack-6flj pass (2026-08-15): wrapper-key / nested-field sweep

Method: extracted every real response document's field set from
`docdb@v1.51.4`'s `deserializers.go` (`awsAwsquery_deserializeOpDocument*`/
`awsAwsquery_deserializeDocument*`, matched on `strings.EqualFold("Name", ...)`
calls -- paren-balance-aware Python walker, same tool used across this
issue's other services) and every request document's field set from
`serializers.go` (`.Key("Name")` calls), then diffed both against every
`handler_*.go` wire struct/decode struct in this package. **Protocol note:**
docdb is genuine `awsAwsquery`/XML -- decode is case-INSENSITIVE
(`strings.EqualFold`), so a casing near-miss alone is not a bug here (unlike
this issue's `awsjson1.1` services); a wrong member NAME, a fabricated
member, or a missing member still is.

**Derived fixes (5, all from state the backend already tracked elsewhere) --
kept separate from the disclosed list above:**
1. `DBInstance.InstanceCreateTime` -- mirrors the existing
   `DBCluster.ClusterCreateTime` pattern in the same file family.
2. `DBClusterSnapshot.{AvailabilityZones,KmsKeyId,MasterUsername,Port,
   ClusterCreateTime}` on `CreateDBClusterSnapshot` -- copied from the
   source `DBCluster` record already in hand at snapshot-creation time.
3. Same 5 fields on `CopyDBClusterSnapshot` -- copied from the source
   *snapshot* record (Copy has no direct cluster reference).
4. `DBClusterSnapshot.SourceDBClusterSnapshotArn` on `CopyDBClusterSnapshot`
   -- the source snapshot's own ARN was already in hand (`src.DBClusterSnapshotArn`).
5. `CopyDBClusterSnapshot`'s `CopyTags`/`Tags` request members -- a real
   discarded-input bug (not just wire-shape): neither was parsed at all, so
   a real client's "copy the source's tags" request silently did nothing.

**2 fabricated (over-wide) wire fields removed, both raw-body-only
observable** (a real client's generated deserializer silently ignores
unknown elements, so neither was independently observable via the typed
SDK client -- proven instead by `TestDescribeDBClusterSnapshots_NoFabricatedDBClusterArn`/
`TestCreateGlobalCluster_NoFabricatedSourceDBClusterIdentifier` inspecting
the raw XML body):
1. `DBClusterSnapshot` emitted a bare `DBClusterArn` that
   `types.DBClusterSnapshot` does not have (only `DBClusterSnapshotArn`).
2. `GlobalCluster`'s response emitted `SourceDBClusterIdentifier`, which is
   a `CreateGlobalClusterInput` REQUEST member only -- `types.GlobalCluster`
   (the response type) has no such member.

Both fabricated fields derive from real ARN-shaped backend state (not
credential-shaped/sensitive data) and were harmless in practice (silently
dropped by any real client) -- classified as hygiene fixes, not real-data
leaks. Neither backend model field was removed, only the wire emission (the
model fields are still used internally: `DBClusterSnapshot.DBClusterArn` by
`CopyDBClusterSnapshot`, `GlobalCluster.SourceDBClusterID` by
`CreateGlobalCluster`'s initial-member bootstrap).

**9 real gaps disclosed, not fabricated** -- see the `gaps:` list above,
split from the derived-fix list for the same reason: each is a real,
optional response member (or, for the service-wide `Filters` gap, a request
member) with zero backing state in this backend, where inventing a plausible
value would be exactly what parity-principles #1 forbids.

**Symmetric pair checked separately (a real asymmetry, not a trap missed):**
`DBCluster.ReplicationSourceIdentifier` (real member, echoed) vs.
`DBCluster.ReadReplicaIdentifiers` (real member, declared+cloned but never
set) -- both are always empty for the same root cause (no
create-as-replica/global-cluster-secondary code path exists), but one is
wired to the wire and the other isn't even though nothing can ever populate
either. Confirmed via `grep`, not assumed.

**Tests:** 3 new real-`aws-sdk-go-v2`-client round-trip tests
(`handler_sdk_roundtrip_test.go`) for the 5 derived fixes, plus 2 raw-body
tests (`handler_db_cluster_snapshots_test.go`/`handler_global_clusters_test.go`)
for the 2 fabricated-field removals, disclosed as raw-body-only per the
reasoning above. All 6 fixes hand-reverted individually, confirmed to fail
with the exact predicted symptom (missing/nil field for the derived fixes,
`0 tags`/empty `SourceDBClusterSnapshotArn` for the discarded-input fix, the
fabricated element literally present in the raw XML for the two removals),
then restored and confirmed **byte-identical** against a saved pre-revert
`git diff` baseline.

**Gates:** `go build` (scoped `./services/docdb/...` + full `./...`, since
`CopyDBClusterSnapshot`'s signature grew 2 params), `go vet`, `go test -race`
(docdb + `pkgs/...`), `go fix -diff` (no diff), `golangci-lint run
./services/docdb/...` (0 issues, no new `//nolint`, no
cyclop/gocyclo/gocognit/funlen), all green.

`last_audit_commit` NOT re-pointed -- this pass's method (deserializer/
serializer field-set extraction against every `handler_*.go` wire struct) is
narrower/deeper than the 2026-07-31 audit's op-by-op wire/errors/state/persist
method, matching this issue's own established precedent for the same
situation (mediatailor/memorydb/codedeploy passes).

**2026-08-22 (gopherstack-bahs) -- RouteMatcher's read-failure branch was
finally safe to flip, but only after a second real bug underneath it.**
gopherstack-3a8t found docdb was one of only 2 of 17 body-reading
RouteMatchers that already gate on a body-independent signal
(`service.MatchesUserAgentMarker(r.Header, "api/docdb")`, verified against
the pinned `docdb@v1.51.4/api_client.go:641` `AddSDKAgentKeyValue` call)
before ever reading the body -- so, unlike the other 15 (which are
form-urlencoded services indistinguishable from each other except by the
body's own `Action`/`Version`), claiming on a `ReadBody` failure here could
not misroute a sibling service's oversized request. That earlier pass tried
exactly that (`return false` -> `return true`) and reverted it: docdb's
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
`handler_oversized_body_test.go` drives a real docdb SDK client through
`service.NewRegistry`/`service.NewServiceRouter`, confirmed failing
pre-fix with `UnknownError` (matcher unchanged) and, at the
matcher-only-flip intermediate step, with `MissingAction` (ParseForm
caching bug); passes now with `InternalFailure`.
`TestHandler_NormalSizedBodyStillRoutes` is the regression guard for a
normal-sized request still routing and succeeding. Hand-reverted
`services/docdb/handler.go` to the pre-fix `git show HEAD:...` version and
back, confirmed byte-identical via `md5sum`. Gates run clean: `go build
./...`, `go vet`, `gofmt -l` (empty), `go test -race ./services/docdb/...`,
`golangci-lint run ./services/docdb/...` (0 issues after adding an
`unknownOp` constant elasticache already uses, to keep `goconst` happy with
the third `"Unknown"` literal the migration introduced).

## 2026-09-05 (parity sweep, gopherstack): re-verified two recent delete-guard fixes, found and fixed one more ghost row

Checked in as part of a repo-wide parity sweep (branch `chore/parity-sweep-2026-09-03`). `last_audit_commit`
NOT re-pointed -- narrower/deeper pass than a full op-by-op re-derivation, same precedent as the
2026-08-15/2026-08-22/2026-08-29 passes above.

Re-verified (not re-derived from scratch, trusted per this file's own header convention since the files were
unchanged since `04b491369` except by two commits explicitly re-checked): `DeleteGlobalCluster`'s
`GlobalClusterMembers`-non-empty guard (commit `ca3a1e21f`) against `api_op_DeleteGlobalCluster.go`'s doc
comment ("The primary and secondary clusters must already be detached or deleted before attempting to delete
a global cluster.") and its modeled `InvalidGlobalClusterStateFault` (confirmed present in
`awsAwsquery_deserializeOpErrorDeleteGlobalCluster`); and `DeleteEventSubscription`'s tags cleanup (commit
`6806b0f10`). Both correct as landed.

**Found and fixed one more instance of the same ghost-row-after-delete class those two commits were sweeping
for, which they did not catch in this service:** `DeleteDBClusterSnapshot` cleared the snapshot itself and
its tags but never its `snapshotAttributes` side-table entry (`store_setup.go:snapshotAttributesKeyFn`, keyed
by `region|DBClusterSnapshotIdentifier`, populated by `ModifyDBClusterSnapshotAttribute`'s restore-permission
grants). `DBClusterSnapshotIdentifier` is user-chosen and freed for reuse on delete, so
`CreateDBClusterSnapshot`/`CopyDBClusterSnapshot` recreating a snapshot under a previously-deleted identifier
silently inherited the old snapshot's cross-account restore grants via `DescribeDBClusterSnapshotAttributes`
-- an access-control artefact, not merely stale data (same discomfort level as the elasticsearch `vpcAccess`
finding in `6806b0f10`). Fixed with a new `snapshotAttributesDelete` helper (`store.go`) called from
`DeleteDBClusterSnapshot` (`db_cluster_snapshots.go`) alongside the existing tags cleanup. Regression test
`TestDeleteDBClusterSnapshot_ClearsAttributes` (`handler_db_cluster_snapshots_test.go`): create cluster +
snapshot, grant a restore attribute, delete, recreate under the same identifier, assert the grant is gone.
Confirmed failing pre-fix with the stale grant present verbatim in the XML body; passes post-fix.

Also checked and found clean this pass: every `sort.Slice` comparator in the package sorts on a field that is
a table/map primary key (identifier, ARN, or subscription name) recomputed fresh on every Describe call, so
the tie-prone-sort class (`pkgs/store.Index.remove`'s last-element swap reordering tied rows) does not apply
here -- there are no ties to have. `DeleteDBCluster`/`DeleteDBInstance` preconditions re-checked directly
against `api_op_DeleteDBCluster.go`/`api_op_DeleteDBInstance.go`: DocDB's `DeleteDBInstance` doc comment
carries no "last instance in cluster" constraint (unlike the neptune gap `ca3a1e21f` fixed in a sibling
service), so gopherstack correctly does not enforce one here. `cli.go`'s `wireTaggingDocDB` cross-service hook
re-checked: keys off `HasTaggableResource`, which checks the real resource tables, not the tags/attributes
side-maps, so it is unaffected by this fix in either direction.

Gates: `GOTOOLCHAIN=go1.26.6 go build ./services/docdb/...`, `go vet`, `gofmt -l` (empty), `go test -race
-count=1 ./services/docdb/...`, `golangci-lint run ./services/docdb/...` (0 issues) -- all green before and
after the fix (baseline already clean; this pass added one bug, one fix, one test, no lint/format changes
needed).

## 2026-08-29 indexed-list wire-key sweep (rds `Values.Value`/neptune `EventCategory` bug family, clean)

Enumerated every hand-parsed indexed-list query key in this service -- every `vals.Get(fmt.Sprintf(...))`
call site (16 sites across `filters.go`, `handler_db_cluster_parameter_groups.go`,
`handler_db_cluster_snapshots.go`, `handler_db_subnet_groups.go`, `handler_db_clusters.go`,
`handler_tags.go`, `handler_events.go`) -- and resolved all 16 against their own operation's
`awsAwsquery_serializeOpDocument<Op>Input`/nested list serializer in the pinned docdb@v1.51.4 SDK. 16-of-16
resolved by direct serializer read. All 16 correct, including `EventCategories.EventCategory.N` and
`SourceIds.SourceId.N` (`handler_events.go`, cross-checked against
`awsAwsquery_serializeDocumentEventCategoriesList`/`awsAwsquery_serializeDocumentSourceIdsList`) and
`Filters.Filter.N.Values.Value.M` (`filters.go`, already fixed and cited by commit `6160e4dad`) -- this
service had already been swept for exactly this bug class. No list truncated to its first element (every
loop terminates on first empty index, not a fixed `.1`/`[0]` read). No Create/Modify divergence:
`parseAvailabilityZones`/`parseVpcSecurityGroupIDs`/`parseCloudwatchEnableLogTypes`/
`parseCloudwatchDisableLogTypes` are each called from both `CreateDBCluster` and `ModifyDBCluster`.
`DeleteDBInstance`/`DescribeCertificates`/`DescribeDBEngineVersions`/pending-maintenance/global-cluster ops
carry no list-typed request fields, so there was no indexed-parsing surface to check there. This bug
class appears exhausted in docdb.

Gates: `go build ./services/docdb/...`, `go vet ./services/docdb/...` and `go vet ./...` (repo-wide,
clean), `go test -race -count=1 ./services/docdb/...` (pass, no changes), `golangci-lint run
./services/docdb/...` (0 issues). No code changed this pass.
