---
service: memorydb
sdk_module: aws-sdk-go-v2/service/memorydb@v1.36.4
last_audit_commit: dbf9633c9                      # this pass (2026-08-23, request-side sweep) fixed UpdateCluster; commit hash not yet known at edit time
last_audit_date: 2026-08-23
overall: A            # 2026-08-15 (gopherstack-6flj): wrapper-key/nested-shape sweep of all 18 L+D+G ops
                       # (scripted key extraction against deserializers.go/serializers.go for all 18
                       # ops + every reachable nested type). Top-level wrapper keys were mostly clean,
                       # but this pass found real bugs one and two levels deeper: Cluster.IpDiscovery
                       # was wire-tagged "IPDiscovery" (case-sensitive awsjson1.1, a real cross-op
                       # bug -- every DescribeClusters/Create/Update/Delete/BatchUpdateCluster/
                       # FailoverShard response silently zeroed it for a real client); DescribeMultiRegionParameters'
                       # response list was emitted under "Parameters" instead of the real
                       # "MultiRegionParameters"; DescribeMultiRegionParameters' AND
                       # DescribeMultiRegionParameterGroups' request name filter was read under
                       # "ParameterGroupName" instead of the real "MultiRegionParameterGroupName" (a
                       # different key, not a casing near-miss) -- required on the former (every real
                       # client request failed outright), optional on the latter (the name filter was
                       # silently ignored, returning every group). Also: Snapshot.ClusterConfiguration
                       # was missing the real MultiRegionClusterName/MultiRegionParameterGroupName
                       # members entirely (never modeled, distinct from the same-named
                       # Cluster-level field already tracked correctly); MultiRegionCluster was
                       # missing the real NumberOfShards response member and its CreateMultiRegionCluster
                       # request-side NumShards counterpart (a discarded input feeding directly into
                       # the missing response field); DescribeReservedNodes was missing the real
                       # Duration/ReservedNodesOfferingId request filters entirely. Pagination
                       # (MaxResults/NextToken) was parsed but never consulted on 7 of 15 Describe ops;
                       # fixed for 6 (DescribeEngineVersions, DescribeReservedNodes,
                       # DescribeReservedNodesOfferings, DescribeMultiRegionClusters,
                       # DescribeMultiRegionParameterGroups, DescribeMultiRegionParameters) using the
                       # existing paginateItems helper; DescribeEvents' pagination gap is left
                       # unfixed and disclosed (see gaps) because its result order is not
                       # deterministic across calls, which would make a cursor unsound. All fixes
                       # verified via a real aws-sdk-go-v2 client through the router
                       # (wire_field_fixes_test.go); each hand-reverted individually and confirmed to
                       # fail with the exact predicted symptom before being restored.
                       # 2026-08-10 (gopherstack-yusn): re-verified all 3 recorded gaps against live code, not just this file's prose -- all 3 still held. Fixed the two provable ones: BatchUpdateCluster accepted any ServiceUpdateNameToApply (including nonexistent names) and always succeeded, doing nothing with it; DescribeServiceUpdates's ClusterNames filter was parsed and never consulted, and the response shape didn't match AWS's real one-row-per-(update,cluster) structure at all. ClusterConfiguration.Shards (snapshot per-shard metadata) and DescribeSnapshotsInput.ShowDetail remain gaps: still genuinely un-derivable without fabricating shard sizes/slot ranges this backend doesn't track (see gaps). Sweeping for the "accepts a name for a resource that doesn't exist, reports success" class found 2 more real instances: UpdateCluster's ACLName and Create/UpdateMultiRegionCluster's MultiRegionParameterGroupName were both applied with zero FK check, unlike every sibling Create op's ACLName/SubnetGroupName/ParameterGroupName checks -- fixed the same way. Checked the tag-routing registry (tags.go) against every resource kind's create/delete path: single arnToResource index, no second store to disagree with it, clean.
                       # 2026-07-31: pkgs/sdkcheck reverse check found ExportSnapshot wrongly advertised/documented as a real SDK op (it isn't -- MemoryDB has no export-to-S3 API at all; see its ops-block note). Corrected, route left wired as internal test scaffolding. Grade held at A: unreachable by real traffic either way, since MemoryDB dispatches purely by X-Amz-Target and no real client can send this target.
                       # 2026-07-23: this pass: field-diffed every core response/request wire type
                       # (Cluster, MultiRegionCluster/RegionalCluster, ReservedNode/
                       # ReservedNodesOffering, User, SubnetGroup/Subnet, ParameterGroup/
                       # Parameter/MultiRegionParameter, Snapshot, EngineVersion,
                       # ServiceUpdate) against deserializers.go's authoritative case
                       # lists. Found and fixed 10 gopherstack-INVENTED fields/filters that
                       # do not exist anywhere in the real SDK (deleted per the no-fabrication
                       # rule), 6 real fields missing from the wire shape (added), a
                       # HTTP-status-code gap (confirmed via aws-sdk-go v1's api-2.json model
                       # and fixed), 2 latent Source-not-set bugs, a request/response
                       # value-space mismatch, and implemented the previously-deferred
                       # Cluster.Status creating->available lifecycle (opt-in, default-off).
                       # 2026-08-29: errcodeaudit ERROR-path sweep. 3 confident findings
                       # (writeBackendError's generic awserr.ErrNotFound/ErrAlreadyExists/ErrConflict
                       # fallback cases, emitting fabricated ResourceNotFoundException/
                       # ResourceInUseException/InvalidRequestException -- none exist in MemoryDB's
                       # SDK, which has no generic bucket exceptions at all, every fault is
                       # resource-specific). Verified NOT live: every currently-defined sentinel of
                       # each category is already caught by the specific errCodeLookup table above
                       # this fallback (exhaustively grepped errors.go), so these 3 branches are
                       # dead code today. Left unchanged: even if reached, there is no correct
                       # generic replacement code to invent (MemoryDB genuinely has none). Flagged as
                       # a landmine for a future sentinel added without a matching errCodeLookup row.
# 2026-08-30 sort-totality sweep (Class F: a sort that exists but is not total,
# and Class G: parallel result lists truncated independently). Reviewed every
# sort.Slice call site across every paginated listing (acls/clusters/snapshots/
# multi_region_clusters/parameter_groups/multi-region parameter objects/
# subnet_groups/users/reserved_nodes/service_updates/events/tags). Every one
# sorts on that resource's own real unique Name/ID (or, where Name alone could
# repeat across a broader scope -- events.go's Date, service_updates.go's
# ServiceUpdateName, multi_region_clusters.go's cross-region cluster listing --
# a composite key ending in a field that IS unique in that scope: Region+Name,
# ServiceUpdateName+ClusterName, Date+SourceName+Message) -- already total by
# construction, not newly fixed. No non-unique, tiebreak-free sort key found.
# Confirmed no listing in this service returns two-or-more collections the API
# defines as one ordered sequence truncated independently. No code changes.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: clusterObject dropped 3 fabricated fields (Tags, MultiRegionParameterGroupName, NumberOfReplicasPerShard -- none exist on real types.Cluster, confirmed via awsAwsjson11_deserializeDocumentCluster's 29-key case list); added the real MultiRegionClusterName request field (was parsed nowhere) with FK validation against an existing multi-Region cluster; Status now supports the opt-in creating->available lifecycle overlay (see families.lifecycle)."}
  DescribeClusters: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now uses the paginateItems helper (handler.go) like every other list op, replacing the hand-rolled cursor loop; Status overlay applied per lifecycle.go. 2026-08-15 (gopherstack-6flj): fixed clusterObject.IpDiscovery, wire-tagged \"IPDiscovery\" (wrong case) -- confirmed via deserializers.go's exact case \"IpDiscovery\": switch match (awsjson1.1 is case-sensitive), so a real client's IpDiscovery was always empty. Shared clusterObject, so this also affected Create/Update/Delete/BatchUpdateCluster/FailoverShard responses."}
  DeleteCluster: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateCluster: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "fixed (gopherstack-yusn): ACLName was assigned to the cluster with no existence check -- unlike CreateCluster, which validates it -- so an UpdateCluster naming a nonexistent ACL silently gave the cluster a dangling ACLName instead of failing with ACLNotFoundFault. Now validated the same way CreateCluster does. Fixed 2026-08-23 (request-side sweep): decode struct had no Engine/ParameterGroupName/SecurityGroupIds members at all (all three real UpdateClusterInput fields) -- every real client update to any of them silently no-opped. Added, with the same ParameterGroupName existence check CreateCluster already does. See Notes."}
  BatchUpdateCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-yusn): ServiceUpdateNameToApply was parsed into the request but never passed to the backend at all -- any name, including one matching no known service update, always succeeded for every found cluster (real AWS fault for an unknown name: ServiceUpdateNotFoundFault, confirmed in botocore's BatchUpdateCluster.errors). Now validated against b.serviceUpdates and, on success, recorded per-cluster on the new Cluster.AppliedServiceUpdates map (additive, persisted)."}
  FailoverShard: {wire: ok, errors: ok, state: ok, persist: ok, note: "no-op failover simulation (event only); acceptable for a mock, matches other services' failover stubs"}
  ListAllowedNodeTypeUpdates: {wire: ok, errors: ok, state: ok, persist: n/a, note: "re-verified: ListAllowedNodeTypeUpdatesOutput is exactly ScaleUpNodeTypes/ScaleDownNodeTypes, matches"}
  CreateACL: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified: aclObject (ARN, Clusters, MinimumEngineVersion, Name, PendingChanges, Status, UserNames) matches types.ACL's 7-key deserializer case list exactly"}
  DescribeACLs: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteACL: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateACL: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: subnetGroupObject/subnetEntry were missing SupportedNetworkTypes (both group- and subnet-level) and each Subnet's AvailabilityZone -- all confirmed real fields via awsAwsjson11_deserializeDocumentSubnetGroup/...Subnet. Added with sensible mock defaults (ipv4-only, round-robin AZs derived from the group's ARN region)."}
  DescribeSubnetGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: userObject dropped a fabricated \"Engine\" field -- confirmed absent from types.User's 7-key deserializer case list (AccessString, ACLNames, ARN, Authentication, MinimumEngineVersion, Name, Status)"}
  DescribeUsers: {wire: partial, errors: ok, state: ok, persist: ok, note: "2026-08-15 (gopherstack-6flj): DescribeUsersInput.Filters ([]types.Filter, a generic Name/Values matcher) is a real, never-modeled request member (confirmed via api_op_DescribeUsers.go) -- disclosed, not implemented: the SDK's own doc comment gives no enumerated set of valid Filter.Name values to implement against honestly, so a generic matcher risks fabricating semantics AWS never documented for this op."}
  DeleteUser: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateUser: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified: parameterGroupObject (ARN, Description, Family, Name) matches types.ParameterGroup's 4-key deserializer case list exactly"}
  DescribeParameterGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeParameters: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed: parameterObject dropped fabricated \"ChangeType\"/\"Source\" fields -- confirmed absent from types.Parameter's 6-key deserializer case list (AllowedValues, DataType, Description, MinimumEngineVersion, Name, Value). FIXED 2026-08-29 (cursor-pagination sweep): DescribeParametersOutput.NextToken (declared on input and output, api_op_DescribeParameters.go) was never populated -- no pagination applied at all, and UpdateParameterGroup accepts arbitrary parameter names (not validated against the known catalogue), so the ~37-entry built-in default set is not provably bounded. Now routed through the shared paginateItems helper like every other list op in this package."}
  ResetParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTags: {wire: ok, errors: ok, state: ok, persist: n/a}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateSnapshot: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: snapshotObject's top-level \"SnapshotCreationTime\" and \"SnapshotType\" were both fabricated at this level -- confirmed types.Snapshot's real deserializer case list is only ARN, ClusterConfiguration, DataTiering, KmsKeyId, Name, Source, Status (7 keys). SnapshotCreationTime actually belongs to types.ShardDetail, nested inside ClusterConfiguration.Shards (not modeled, see gaps); SnapshotType duplicated Source and was deleted service-wide (internal Snapshot.SnapshotType field removed too). Added the real, previously-missing DataTiering field, populated from the source cluster. 2026-08-15 (gopherstack-6flj): snapshotClusterConfig (real types.ClusterConfiguration) was missing the real MultiRegionClusterName/MultiRegionParameterGroupName members entirely -- confirmed via types.go, distinct from the already-tracked Cluster.MultiRegionClusterName at a different level. Added; MultiRegionClusterName copied straight off the source cluster, MultiRegionParameterGroupName resolved through the cluster's MultiRegionCluster FK (snapshotClusterConfigFor, shared by CreateSnapshot/seedAutomatedSnapshotLocked/DeleteCluster's final-snapshot path). snapshotClusterConfig also backs Snapshot's own persistence (json.Marshal(snap)); only new fields with fresh tags were added, nothing retagged."}
  DescribeSnapshots: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: Source request filter previously string-compared directly against internal automated/manual storage values, but DescribeSnapshotsInput.Source's real accepted values are \"system\"/\"user\" (per its own doc comment) -- a real client's Source=system/user would have matched zero snapshots. normalizeSnapshotSource (snapshots.go) now maps system->automated, user->manual, while still leniently accepting automated/manual directly."}
  CopySnapshot: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: dst snapshot never set Source at all (only the now-deleted SnapshotType), so a Source-filtered DescribeSnapshots would never match a copied snapshot -- a real state bug, not just wire-label. Now sets Source and carries DataTiering forward from the source snapshot."}
  DeleteSnapshot: {wire: ok, errors: ok, state: ok, persist: ok}
  # ExportSnapshot is intentionally NOT listed as an advertised SDK op here.
  # 2026-07-31 CORRECTION: the row that used to live at this position ("wire:
  # ok, ...", "mock export ... matches other services") was inaccurate --
  # ExportSnapshot is not a real AWS MemoryDB SDK operation at all (verified
  # against botocore's memorydb service-2.json: only CopySnapshot/
  # CreateSnapshot/DeleteSnapshot/DescribeSnapshots exist under the snapshot
  # family; MemoryDB, unlike ElastiCache, has no export-to-S3 API). Caught by
  # pkgs/sdkcheck's reverse check (commit 12cfe14d5; gopherstack-vhw2 category
  # A). MemoryDB dispatches purely by X-Amz-Target header value, so a real
  # client can never send this target and the route (a validate-and-return
  # no-op) was already unreachable by real traffic; it stays wired as
  # internal test scaffolding, unadvertised. See handler.go's comment on the
  # GetSupportedOperations() entry. Same resolution as DAX's
  # ResetParameterGroup and EMR's ListTagsForResource.
  DescribeEngineVersions: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed: engineVersionObject dropped a fabricated \"Description\" field -- confirmed absent from types.EngineVersionInfo's 4-key deserializer case list (Engine, EnginePatchVersion, EngineVersion, ParameterGroupFamily); kept internally on the EngineVersion model as seed-table documentation only. 2026-08-15 (gopherstack-6flj): MaxResults/NextToken were parsed but never consulted -- every call returned the full static catalog in one page. Fixed via paginateItems, cursor = Engine+\"|\"+EngineVersion (unique within the static catalog)."}
  DescribeEvents: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified: eventObject (Date, Message, SourceName, SourceType) matches types.Event's 4-key deserializer case list exactly. FIXED 2026-08-29 (cursor-pagination sweep): DescribeEventsOutput.NextToken was never populated -- no pagination applied at all, even though events accumulate up to maxEvents=1000 per region (store.go) and DescribeEvents concatenates across every region. Events have no unique name field, so this uses pkgs/page (index-offset cursor) rather than this package's name-keyed paginateItems; results are now sorted deterministically (Date, then SourceName, then Message) since map iteration over the per-region event store is otherwise randomized and pagination requires stable ordering across calls. FIXED 2026-08-30 (wrapper-key sweep): the 'concatenates across every region' behavior just described was itself the bug, not a documented feature -- DescribeEvents discarded its ctx parameter and ranged over every region's event log unconditionally, so any caller in any region saw every other region's events too, even though every event-appending call site already stores events under the correct request-derived region. Now scoped to getRegion(ctx, b.defaultRegion); see gaps entry below for the proof."}
  CreateMultiRegionCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: multiRegionClusterObject was missing the real \"Clusters\" ([]RegionalCluster) and \"TLSEnabled\" fields -- both confirmed on types.MultiRegionCluster. Clusters is now populated from actual per-Region Cluster records referencing this multi-Region cluster by name (RegionalClustersFor, multi_region_clusters.go). Also fixed (gopherstack-yusn): MultiRegionParameterGroupName was stored with no existence check, unlike the equivalent ACLName/SubnetGroupName/ParameterGroupName FKs on CreateCluster; now validated against b.multiRegionParameterGroups (ErrMultiRegionParameterGroupNotFound). 2026-08-15 (gopherstack-6flj): NumShards was a real CreateMultiRegionClusterInput member (confirmed via api_op_CreateMultiRegionCluster.go) that wasn't even in the request struct -- a discarded input, silently defaulting every multi-Region cluster to an unreported 0 shards. Added, defaults to 1 (matching CreateCluster's own default) when unset, validated 1-500 like CreateCluster."}
  DeleteMultiRegionCluster: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeMultiRegionClusters: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: ShowClusterDetails was parsed but never gated anything (multiRegionClusterObject had no Clusters field to gate); now mirrors DescribeClusters' ShowShardDetails convention -- Clusters is populated only when ShowClusterDetails is true. 2026-08-15 (gopherstack-6flj): multiRegionClusterObject was missing the real NumberOfShards response member entirely (types.MultiRegionCluster, confirmed via its 11-key deserializer case list) -- added, sourced from the new MultiRegionCluster.NumShards field (see CreateMultiRegionCluster). MaxResults/NextToken were also parsed but never consulted; fixed via paginateItems, cursor = MultiRegionClusterName."}
  UpdateMultiRegionCluster: {wire: partial, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-yusn): same MultiRegionParameterGroupName gap as CreateMultiRegionCluster -- accepted and stored any name, including nonexistent ones, with no FK check. Now validated. 2026-08-15 (gopherstack-6flj): NOT fixed, disclosed -- real UpdateMultiRegionClusterInput also has ShardConfiguration (*types.ShardConfigurationRequest, a resharding request) and UpdateStrategy (\"coordinated\"/\"uncoordinated\") members that this request struct doesn't model at all. Implementing this honestly needs the same in-progress-resharding state ClusterPendingUpdates.Resharding would need (see gaps) -- out of scope for this pass; downgraded wire: partial rather than fabricated."}
  ListAllowedMultiRegionClusterUpdates: {wire: ok, errors: ok, state: ok, persist: n/a}
  DescribeMultiRegionParameterGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified: multiRegionParameterGroupObject (ARN, Description, Family, Name) matches types.MultiRegionParameterGroup's 4-key deserializer case list exactly. 2026-08-15 (gopherstack-6flj): fixed -- the request's optional name filter was read under \"ParameterGroupName\"; the real key (confirmed via api_op_DescribeMultiRegionParameterGroups.go and its serializer) is \"MultiRegionParameterGroupName\", a different key entirely, not a casing variant. Silent bug: a real client's name filter was always ignored, returning every group instead of one. MaxResults/NextToken were also parsed but never consulted; fixed via paginateItems, cursor = Name."}
  DescribeMultiRegionParameters: {wire: ok, errors: ok, state: partial, persist: n/a, note: "fixed: previously reused parameterObject (types.Parameter's shape), silently dropping \"Source\" -- confirmed types.MultiRegionParameter is a DISTINCT shape from types.Parameter that additionally carries Source (values: user | system | engine-default). New multiRegionParameterObject type added for this op only. 2026-08-15 (gopherstack-6flj): fixed two stacked bugs -- the response list was wire-tagged \"Parameters\" instead of the real \"MultiRegionParameters\" (confirmed via deserializers.go's OpDocumentOutput case list; the sibling plain DescribeParameters genuinely does use \"Parameters\", so this was a sibling-trap, not a copy-paste of a shared bug), and the request's REQUIRED group-name field was read under \"ParameterGroupName\" instead of the real \"MultiRegionParameterGroupName\" -- every real client's request previously failed InvalidParameterValueException outright, so this op was fully broken for any real caller. Also added the real, optional Source filter to the request struct (parsed, not applied -- see families.fabricated_fields note on multi-region-parameter DataType/Source synthesis; downgraded state: partial for that reason) and MaxResults/NextToken pagination via paginateItems, cursor = Name."}
  DescribeReservedNodes: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: describeReservedNodesRequest dropped a fabricated \"ReservedNodeId\" filter -- real DescribeReservedNodesInput has only ReservationId (confirmed via api_op_DescribeReservedNodes.go), no ReservedNodeId at all. 2026-08-15 (gopherstack-6flj): that same input ALSO has real Duration and ReservedNodesOfferingId filters (confirmed same file) that were never modeled at all -- zero grep hits, not removed by the prior pass, just never added. Added and wired to the existing per-reservation Duration/ReservedNodesOfferingId fields, filtered the same way DescribeReservedNodesOfferings already filters its own. MaxResults/NextToken were also parsed but never consulted; fixed via paginateItems, cursor = ReservationId (matches the existing sort key)."}
  DescribeReservedNodesOfferings: {wire: ok, errors: ok, state: ok, persist: n/a, note: "fixed: ReservedNodesOffering dropped a fabricated \"UsagePrice\" field -- confirmed absent from types.ReservedNodesOffering's 6-key deserializer case list (Duration, FixedPrice, NodeType, OfferingType, RecurringCharges, ReservedNodesOfferingId). 2026-08-15 (gopherstack-6flj): MaxResults/NextToken were parsed but never consulted; fixed via paginateItems, cursor = ReservedNodesOfferingId."}
  PurchaseReservedNodesOffering: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: ReservedNode had a fabricated \"ReservedNodeId\" field (used as the internal store key and filter target) and was MISSING the real \"ReservedNodesOfferingId\" field entirely -- confirmed types.ReservedNode has no ReservedNodeId at all (11-key deserializer case list: ARN, Duration, FixedPrice, NodeCount, NodeType, OfferingType, RecurringCharges, ReservationId, ReservedNodesOfferingId, StartTime, State). Also fixed a values-swapped bug where the response's ReservationId field actually held the offering ID and vice versa. Also dropped the fabricated \"UsagePrice\" field (same as the offering type)."}
  DescribeServiceUpdates: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-yusn): confirmed via AWS API reference that a real response has one entry PER (update, cluster) pair -- every entry carries ClusterName. This backend previously returned the 2 seeded update definitions as a flat, cluster-less list regardless of what existed, and ClusterNames was parsed but never consulted (a nonexistent cluster name still returned every update). Now fans each update definition out against every cluster whose Engine matches (the only real, non-fabricated link available -- MemoryDB updates are engine-scoped, as the seed data's own Description text says), filling in ClusterName and flipping Status to \"complete\" once BatchUpdateCluster has applied it to that cluster. NodesUpdated is added to the wire shape but left empty -- this backend has no per-node update tracking, so it's honestly omitted rather than fabricated."}
families:
  errors: {status: ok, note: "systemic fix (prior pass): writeBackendError previously collapsed every NotFound/AlreadyExists/Conflict error into fabricated generic types that do not exist anywhere in MemoryDB's real API surface. errCodeLookup (handler.go) maps each of the package's 19 sentinel errors to its confirmed real __type."}
  http_status_codes: {status: ok, note: "fixed this pass: errCodeLookup's HTTP statuses were a 404/409/400 categorization (NotFound->404, AlreadyExists/InUse->409); confirmed via aws-sdk-go v1's model (models/apis/memorydb/2021-01-01/api-2.json) that every one of MemoryDB's ~53 exception shapes has an empty \"error\" trait ({}, no httpStatusCode override) -- the JSON-protocol default for an unoverridden client-fault shape is 400, meaning real AWS returns 400 uniformly for every fault in this service, never 404 or 409. Also confirmed deserializers.go resolves error identity purely from the __type/code field (resolveProtocolErrorType), never from response.StatusCode, so this had zero effect on real-client error-type resolution either way -- but the status code itself was wrong. errCodeLookup and the coarse category-based fallback in writeBackendError both now use http.StatusBadRequest uniformly; ~59 existing test assertions (404/409 -> 400) updated across every affected test file, including 2 raw-int-literal (404/409, not http.Status*) test files that a naive grep for the named constants would have missed."}
  fabricated_fields: {status: ok, note: "systemic sweep this pass: field-diffed every core wire type's Go struct against its own deserializers.go case list (the authoritative source -- types.go doc comments alone were double-checked against this since a prior finding showed a doc-comment-derived field can still be wrong about which TYPE it belongs to). Found and DELETED 10 gopherstack-invented fields/filters that appear nowhere in the real SDK: ReservedNode.ReservedNodeId (used as the internal store/filter key -- also removed from describeReservedNodesRequest), ReservedNode.UsagePrice, ReservedNodesOffering.UsagePrice, Cluster response's Tags/MultiRegionParameterGroupName/NumberOfReplicasPerShard (3 fields), User response's Engine, Parameter's ChangeType/Source (2 fields), Snapshot response's top-level SnapshotType, Snapshot response's top-level SnapshotCreationTime (the real field of that name belongs to a different, nested type -- types.ShardDetail -- not top-level Snapshot), EngineVersionInfo response's Description. Also ADDED 6 real fields that were missing: ReservedNode.ReservedNodesOfferingId, SubnetGroup/Subnet.SupportedNetworkTypes + Subnet.AvailabilityZone (3 fields), Snapshot.DataTiering, ServiceUpdate.Engine, MultiRegionCluster.Clusters + TLSEnabled (2 fields), MultiRegionParameter.Source (via a new distinct multiRegionParameterObject type, since types.MultiRegionParameter is NOT the same shape as types.Parameter). Every deletion/addition is cited against its specific deserializers.go function and confirmed field/absent-field."}
  lifecycle: {status: ok, note: "implemented this pass (was gap: \"Cluster.Status is always available immediately\"): goroutine-free creating->available status overlay, mirroring services/elasticache/lifecycle.go's mechanism exactly. A Cluster records a transient PendingStatus + AvailableAt deadline (markCreatingLocked); every read/write path that surfaces a Cluster (clusterView) overlays the transient status until the backend clock (injectable via SetClock, for deterministic tests) passes the deadline. Default lifecycleDelay is zero -> transitions are instant, so this is 100% backward compatible with every pre-existing test; opt in via SetLifecycleDelay. Does not (yet) implement deleting-state simulation or shard/node-level transitions, only cluster creating->available -- scoped to exactly what the prior pass's gap called out."}
  timestamps: {status: ok, note: "prior pass: 5 TStamp wire-shape bugs fixed (Event.Date, ReservedNode.StartTime, ServiceUpdate.ReleaseDate/AutoUpdateStartDate, and what was believed to be Snapshot.SnapshotCreationTime). This pass found the last one was fixed at the WRONG location in the wire shape -- SnapshotCreationTime is not a top-level Snapshot field at all; see fabricated_fields. The epoch-seconds format fix itself was correct, just misplaced; the field is now deleted from the top level rather than moved to its real location (types.ShardDetail nested in ClusterConfiguration.Shards), which remains unmodeled -- see gaps."}
  pointer_aliasing: {status: ok, note: "prior pass, still holds: Create*/Copy*/Export* ops clone before returning."}
  persistence: {status: ok, note: "Handler exposes Snapshot(ctx)/Restore(ctx,[]byte) delegating straight to InMemoryBackend; backendSnapshot versioning (memorydbSnapshotVersion, still 1) unaffected by this pass's field additions/removals -- all additive/subtractive struct field changes are backward/forward compatible with encoding/json's default zero-value behavior, no version bump needed."}
  route_matcher: {status: ok, note: "unchanged this pass: single X-Amz-Target-prefixed POST endpoint, all GetSupportedOperations entries reachable through dispatch (structurally immune to the path-segment-router bug class -- flat X-Amz-Target dispatch, not path-segment matching)."}
  pagination: {status: ok, note: "2026-08-15 (gopherstack-6flj): 7 of 15 Describe ops parsed MaxResults/NextToken into their request struct but never called paginateItems (handler.go) -- every call returned the full result set in one page regardless of MaxResults. Fixed 6 (DescribeEngineVersions, DescribeReservedNodes, DescribeReservedNodesOfferings, DescribeMultiRegionClusters, DescribeMultiRegionParameterGroups, DescribeMultiRegionParameters), all backed by statically-ordered or explicitly-sorted results, so a name-based cursor is sound. DescribeEvents left unfixed at the time -- see gaps for the 2026-08-29 resolution (deterministic sort added, pagination now wired). 2026-08-29 (cursor-pagination sweep): DescribeParameters was a previously-unnoticed 8th unpaginated op, now fixed (paginateItems). Also found and fixed a severe pre-existing bug in paginateItems itself: findStartIndex resumed one index past the matching item instead of at it, silently dropping exactly one item at every page boundary across all 14 paginated ops (not just the newly-fixed ones) since nextToken encodes the next page's first item inclusively, not the previous page's last item exclusively. See TestPaginateItems_NoSkipAcrossPages (whitebox_test.go)."}
gaps:                     # known divergences NOT fixed this pass
  - "2026-08-30 (wrapper-key sweep): CreateClusterInput.SnapshotArns ([]string, real field confirmed at api_op_CreateCluster.go -- 'the list of Amazon Resource Names (ARN) that uniquely identify the RDB snapshot files stored in Amazon S3 ... used to populate the new cluster') is declared on createClusterRequest (models_clusters.go) but never read anywhere in CreateCluster (clusters.go): a request-driven exhaustive-reference sweep of every *Request/*Input struct's fields across this service found this as the sole unread field. Not a misread key -- this is CreateCluster's second, S3-backed restore path, distinct from the fully-implemented SnapshotName path (an existing in-account Snapshot object, matched by name and applied via applySnapshotRestoreConfig). This backend has no S3 integration and holds no data for an externally-uploaded RDB file, so there is nothing honest to import; silently accepting and ignoring the ARNs (current behavior) is preferred over fabricating imported cluster state. Same missing-backend-data class as the pre-existing ClusterConfiguration.Shards/DescribeSnapshotsInput.ShowDetail gaps above, not fixed for the same reason."
  - "ClusterConfiguration.Shards ([]ShardDetail) is not modeled: real AWS's Snapshot.ClusterConfiguration carries a full per-shard array (Configuration/ShardConfiguration sub-object with Slots/ReplicaCount, Name, Size, SnapshotCreationTime -- confirmed via types.ShardDetail and its deserializer). snapshotClusterConfig has none of this. Re-checked 2026-08-10 (gopherstack-yusn): the backend DOES track a shard COUNT (Cluster.NumShards/NumReplicasPerShard) and derives synthetic Name/Slots/Nodes for DescribeClusters' ShowShardDetails (buildShards, handler_clusters.go) -- but ShardDetail.Size (the shard's snapshot data size) is never tracked anywhere and has no honest derivation, and reusing buildShards' evenly-split synthetic Slots for permanent snapshot metadata would fabricate historical per-shard data no real resharding/slot-migration event produced. Still not fixed: Size is genuinely absent, and Slots would have to be invented for this specific field even though a similar synthesis is tolerated for the live-cluster ShowShardDetails view; fabricating either violates the no-stub rule."
  - "ServiceUpdate.NodesUpdated is not modeled: real AWS's field lists which nodes a per-cluster service update instance has updated. This backend has no per-node update tracking (buildShards' node identities are synthesized per-request, not persisted per-node state), so there is nothing honest to report; the wire field exists (added 2026-08-10) but is always empty rather than fabricated. ClusterName/per-cluster fanout and the ClusterNames filter ARE now modeled -- see DescribeServiceUpdates/BatchUpdateCluster fixed in this pass."
  - "DescribeSnapshotsInput.ShowDetail (real field; per AWS's doc comment it gates whether the per-shard configuration -- ClusterConfiguration.Shards -- is included in the response, NOT ClusterConfiguration itself, which is always present) is not implemented. Tied to the Shards gap above: since Shards can't be honestly populated (Size/Slots not derivable without fabrication), wiring a ShowDetail flag that gates an always-empty Shards list would just be a second parsed-and-ignored request field: not implemented, rather than added as a no-op."
  - "2026-08-15 (gopherstack-6flj): ClusterPendingUpdates.Resharding (real member, types.ReshardingStatus{SlotMigration{ProgressPercentage}}, confirmed via deserializers.go's 3-key ClusterPendingUpdates case list -- ACLs/Resharding/ServiceUpdates) is not modeled on pendingUpdatesObject at all. Same root cause as the UpdateMultiRegionCluster ShardConfiguration gap above: UpdateCluster/UpdateMultiRegionCluster apply a shard-count change synchronously with no in-progress-resharding state (grep for \"reshard\" in this service: zero hits outside this note), so there is nothing to honestly report -- the field would always be absent/nil either way, identical to a real AWS response at rest with no resharding in flight. Not added as a dead always-nil field; disclosed instead."
  - "RESOLVED (2026-08-30, wrapper-key sweep). 2026-08-15 (gopherstack-6flj): DescribeEvents' MaxResults/NextToken are parsed but not consulted -- every call returns the full matching event log in one page. UPDATE 2026-08-29 (cursor-pagination sweep): the pagination half is now fixed -- DescribeEvents (events.go) now sorts its result deterministically (Date, then SourceName, then Message) before pkgs/page.New paginates it, resolving the 'non-deterministic order makes a cursor unsound' blocker this note originally raised. The cross-region leakage this note also flagged was UNCHANGED and still open at that point: DescribeEvents still iterated b.events (a map keyed by region) without scoping to the calling request's region at all -- the new sort made that already-cross-region result deterministically ORDERED, it did not stop the leak. UPDATE 2026-08-30 (wrapper-key sweep): the leak itself is now fixed -- DescribeEvents derives region := getRegion(ctx, b.defaultRegion) and ranges over only b.events[region]. Every event-appending call site (CreateCluster/DeleteCluster/CreateACL/CreateSnapshot/CreateUser/etc.) already called appendEventLocked with the correct request-derived region, so only this read path needed scoping. Proven via TestDescribeEvents_RegionIsolation_RealClient (events_region_isolation_test.go), driving two real aws-sdk-go-v2 clients signed for us-east-1/us-west-2 against the same handler; confirmed failing (each region saw the other's cluster-creation event) against the unfixed code first."
  - "2026-08-15 (gopherstack-6flj): DescribeUsersInput.Filters -- see DescribeUsers op note above."
deferred:                 # consciously not audited this pass (scope) -- next pass targets
  - "Byte-for-byte audit of nested shardObject/nodeObject beyond the fields already spot-checked (Name, Status, Slots, Nodes, NumberOfNodes on Shard; AvailabilityZone, CreateTime, Endpoint, Name, Status on Node) -- these matched exactly against types.Shard/types.Node's deserializer case lists when checked this pass, but the full request-shape interaction with real Slots math (16384 keyspace distribution) was not independently verified against live AWS."
  - "MultiRegionCluster.Clusters' RegionalCluster.Status semantics beyond \"reflects the underlying Cluster.Status\" -- real AWS may report a distinct Region-membership status (e.g. \"active\"/\"creating\"/\"deleting\" scoped to the multi-Region relationship itself) rather than just mirroring the Regional cluster's own Status; not independently confirmable without live AWS."
  - "PurchaseReservedNodesOffering / DescribeReservedNodesOfferings RecurringCharges frequency/amount realism (values are mock placeholders, e.g. \"Hourly\") -- wire shape (RecurringChargeAmount/RecurringChargeFrequency) confirmed correct, but the actual pricing data is illustrative, not derived from any real AWS price list (same caveat as every other service's pricing mocks)."
leaks: {status: clean, note: "no goroutines, timers, or janitor loops added this pass -- the new lifecycle.go overlay mechanism is pure functions over stored fields (PendingStatus/AvailableAt/clock), identical in shape to services/elasticache's proven goroutine-free design; b.mu remains the sole coarse lock (still a plain sync.RWMutex, not lockmetrics.RWMutex -- a pre-existing convention deviation from the pkgs/lockmetrics rule, not introduced or fixed this pass, flagged for a future pass since changing the lock type across ~30 call sites is out of scope for a wire-parity pass)."}
---

## Notes

**Protocol**: awsjson1.1 (`X-Amz-Target: AmazonMemoryDB.<Op>`), single POST endpoint.
Confirmed against `aws-sdk-go-v2/service/memorydb@v1.33.12`'s `deserializers.go`/`serializers.go`.

**This pass's method**: for every core wire type, extracted the authoritative field list
directly from its own `awsAwsjson11_deserializeDocument<Type>` function in `deserializers.go`
(the literal `case "FieldName":` list a real client's JSON deserializer recognizes) rather than
trusting `types.go`'s doc comments alone, then diffed that list 1:1 against gopherstack's Go
struct. This caught a class of bug the prior pass's doc-comment-based review missed: a field
name can be *real* (e.g. `SnapshotCreationTime`) while being attached to the *wrong type* in
gopherstack's wire shape (it belongs to `types.ShardDetail`, nested inside
`Snapshot.ClusterConfiguration.Shards`, not top-level `Snapshot`) -- syntactically identical to
a fabricated field from a wire-correctness standpoint (a real client's top-level `Snapshot`
deserializer has no such key and would simply ignore it), but a different root cause than an
outright invention like `ReservedNode.ReservedNodeId`.

**Ten fabricated fields deleted, six real fields added** -- full list and citations in
`families.fabricated_fields` above. Two of the fabricated fields were more than cosmetic:
`ReservedNode.ReservedNodeId` was the *store key* (used for lookups/filtering), and removing it
required re-keying `store_setup.go`'s `reservedNodeKeyFn` onto the real `ReservationId` field and
fixing a values-swapped bug in `PurchaseReservedNodesOffering` (the response's `ReservationId`
field actually held the *offering* ID and vice versa -- a real state-correctness bug, not just a
label issue). `Snapshot.SnapshotType` (fabricated, duplicated `Source`) was set independently of
`Source` at two call sites (`CopySnapshot`, `DeleteClusterWithSnapshot`'s final-snapshot path)
that never set `Source` at all -- meaning a `Source`-filtered `DescribeSnapshots` would silently
never match snapshots created via those two paths. Deleting `SnapshotType` and consolidating on
`Source` as the single source of truth fixed this as a side effect.

**`DescribeSnapshotsInput.Source`'s real values are `"system"`/`"user"`, not
`"automated"`/`"manual"`**: confirmed via `api_op_DescribeSnapshots.go`'s doc comment ("If set to
system... If set to user..."), while the *response* field `Snapshot.Source` documents its own
values as `"automated"`/`"manual"`. This is a genuine asymmetry in real AWS's own API (the
request-side filter accepts different strings than what the response echoes back). A real client
filtering by `Source: "system"` would previously have matched zero snapshots, since this backend
string-compared the raw filter value against its internal `"automated"`/`"manual"` storage.
`normalizeSnapshotSource` (`snapshots.go`) now maps the real request values to the internal
storage convention while still leniently accepting `"automated"`/`"manual"` directly (a caller
passing the response-side value back in as a filter is a reasonable, harmless thing to support).

**HTTP status codes are uniformly 400, not 404/409/400 by category**: confirmed via
`aws-sdk-go` v1's model file (`models/apis/memorydb/2021-01-01/api-2.json`) -- every one of
MemoryDB's ~53 exception shapes has an empty `"error"` trait (no `httpStatusCode` override), and
the JSON-protocol default for an unoverridden client-fault shape is 400. This has zero effect on
a real `aws-sdk-go-v2` client's typed-error resolution (`deserializers.go` resolves error
identity purely from the `__type`/`code` field, confirmed by reading the top of every
`awsAwsjson11_deserializeOpError*` function -- `response.StatusCode` is never consulted for that
purpose), but the status code on the wire itself was wrong relative to real AWS. Fixed in both
`errCodeLookup` and the coarse category-based fallback in `writeBackendError`; ~59 existing test
assertions across 13 test files updated from 404/409 to 400 (including two files using raw int
literals `404`/`409` rather than the named `http.Status*` constants, which a naive
grep-and-replace on the constant names alone would have missed).

**Cluster.Status now supports an opt-in creating->available lifecycle** (`lifecycle.go`),
closing the prior pass's largest deferred gap. Mirrors `services/elasticache/lifecycle.go`'s
proven goroutine-free design exactly: `SetLifecycleDelay`/`SetClock` are no-ops by default (zero
delay = instant transition, identical to every pre-existing test's expectations), and only
activate the `PendingStatus`/`AvailableAt` overlay when a test explicitly opts in. Scoped
narrowly to Cluster creation (the exact gap that was called out) -- does not add deleting-state
simulation, shard/node-level transitions, or apply to other resource types.

**Error `__type` is resource-specific, never generic** (prior pass, still holds): MemoryDB's
error model (`types/errors.go`) defines ~55 fault types and *zero* generic ones.
`errCodeLookup` (`handler.go`) maps each of the package's 19 sentinel errors to its confirmed
real `__type`.

**In-use state faults use `Invalid*StateFault`, not a made-up `*InUseFault`** (prior pass, still
holds): `DeleteSubnetGroup` is the one exception with a dedicated `SubnetGroupInUseFault`.

**Timestamps are epoch-seconds JSON numbers, not RFC3339 strings** (prior pass, still holds, see
`families.timestamps` above for this pass's SnapshotCreationTime correction).

**`Authentication.Type` output enum is `password | no-password | iam`** (prior pass, still
holds), never `no-password-required`.

**Not real bugs, ruled out this pass** (documented so a future auditor doesn't re-flag them):
`ACL`/`ParameterGroup`/`MultiRegionParameterGroup`/`Event`/`ListAllowedNodeTypeUpdatesOutput`
wire shapes were all re-verified field-for-field against their deserializers.go case lists and
found to already match exactly, with zero fabricated or missing fields -- these are genuinely
clean, not merely unaudited. (2026-07-31 correction: the note previously here comparing
`ExportSnapshot`'s mock export to "every other service's snapshot-export mock" was itself
wrong -- `ExportSnapshot` is not a real MemoryDB operation at all; see its ops-block entry
above.) `b.mu` being a plain `sync.RWMutex` rather
than `pkgs/lockmetrics.RWMutex` is a pre-existing convention deviation, not a leak or
correctness bug (every lock path is still properly `defer`-released) -- flagged under `leaks`
for a future pass rather than churned here, since retrofitting the metrics wrapper across ~30
call sites is unrelated to wire-shape parity and carries its own regression risk.

## 2026-08-23: UpdateCluster silently dropped Engine, ParameterGroupName, SecurityGroupIds

Request-side sweep (accept-and-drop class -- the response side got a deep
sweep on 2026-08-15, gopherstack-6flj; the request side had never been
checked). `updateClusterRequest` (`models_clusters.go`) had no `Engine`,
`ParameterGroupName`, or `SecurityGroupIds` members at all, though all
three are real `UpdateClusterInput` fields (api_op_UpdateCluster.go:44,
87, 93) and `createClusterRequest` already accepts and applies all three on
Create. Every real client that used `UpdateCluster` to change a cluster's
engine, swap its parameter group, or update its security groups got a
silent no-op with a 200 response.

Fixed: added all three fields to `updateClusterRequest`; wired
`Engine`/`ParameterGroupName` into `applyClusterStringUpdates` and
`SecurityGroupIds` into `applyClusterUpdates` (`clusters.go`); added the
same `ParameterGroupName` existence check `CreateCluster` already performs
(`ErrParameterGroupNotFound` if the named group doesn't exist).

Proof: `TestUpdateCluster_EngineParameterGroupSecurityGroups_RoundTrip`
(`wire_update_cluster_test.go`) drives the real
`aws-sdk-go-v2/service/memorydb` client through CreateParameterGroup ->
CreateCluster -> UpdateCluster(Engine, ParameterGroupName,
SecurityGroupIds) -> DescribeClusters, asserting all three landed.
Confirmed it fails pre-fix (Engine stayed "redis", ParameterGroupName
stayed empty, SecurityGroups stayed empty) -- hand-reverted via `cp`,
md5sum-identical restore after.

**Not a bug, checked and ruled out**: the same sweep also flagged
`updateClusterRequest.IPDiscovery`'s json tag ("IPDiscovery") as
mismatching the real wire key "IpDiscovery" -- the same casing bug
2026-08-15 fixed on the *response* side. Verified directly
(`encoding/json` test snippet) that Go's `json.Unmarshal`, unlike
aws-sdk-go-v2's generated case-sensitive deserializer, falls back to a
case-insensitive field match when no exact tag match exists -- so a real
client's `"IpDiscovery"` key was already landing in the `"IPDiscovery"`-tagged
field correctly, and `createClusterRequest` carried the identical
"wrong-looking" tag with no behavioral effect either. This asymmetry
(request-side decode is Go's lenient stdlib; response-side encode is read
by AWS's case-sensitive generated client code) means a decode-struct
casing mismatch is not, by itself, evidence of a request-side bug -- only a
genuinely different key name (as in the UpdateServiceAttributes fix this
same pass, servicediscovery/PARITY.md) is. Retagged both `IPDiscovery`
fields to the correct casing anyway for self-documentation/consistency
with `clusterObject`, but this is not counted as a functional fix.

## 2026-08-29 indexed-list wire-key sweep (rds `Values.Value`/neptune `EventCategory` bug family, N/A)

Checked whether the rds `Filters.Filter.N.Values.Value.M` / neptune `EventCategories.EventCategory.N`
bug family (a wrong *inner element name* in an XML/Query-protocol indexed list, or a hand-parsed request
key mismatched against the SDK's own field name) recurs here. MemoryDB is JSON-RPC 1.1 (confirmed:
`awsAwsjson11_*` serializer prefix in the pinned memorydb@v1.36.4 SDK), so requests decode via
`encoding/json` into typed Go structs (`models_*.go`) -- there is no indexed `list.N`-style key parsing at
all; JSON arrays decode natively. The structural precondition for this bug class (a hand-built indexed
key string that can name the wrong wrapper element) doesn't exist on the request-decode path. Spot-checked
every request struct with a slice-typed field (`CreateACL`/`UpdateACL`/`TagResource`/`UntagResource`/
`BatchUpdateCluster`/`CreateSubnetGroup`/`UpdateSubnetGroup`/`DescribeServiceUpdates`/
`PurchaseReservedNodesOffering`/`CreateMultiRegionCluster`/`UpdateParameterGroup`/`CreateUser`/
`UpdateUser`) against the pinned SDK's `awsAwsjson11_serializeOpDocument<Op>Input` `object.Key(...)`
calls -- all json tags match the real wire field name. Confirmed `DescribeEventsInput` (memorydb@v1.36.4
api_op_DescribeEvents.go) has no `EventCategories` field at all, so the neptune-specific variant is
structurally impossible here. No list truncated to its first element (checked `statusFilter` in
`service_updates.go`, uses `slices.Contains` over the full slice). This bug class doesn't apply to this
service.

Gates: `go build ./services/memorydb/...`, `go vet ./services/memorydb/...` and `go vet ./...`
(repo-wide, clean), `go test -race -count=1 ./services/memorydb/...` (pass, no changes), `golangci-lint
run ./services/memorydb/...` (0 issues). No code changed this pass.

## 2026-08-29 cursor-pagination audit -- CRITICAL: shared paginateItems() dropped one item per page boundary

This is the most severe finding of this pass, and it is not the "cursor never set" class the
sweep was primarily hunting -- it is worse: the cursor WAS set, correctly advertised a next
page, and following it silently dropped exactly one item at every single page boundary,
across every one of this package's 14 paginated list operations.

`paginateItems` (`handler.go`) is this service's one shared, generic list-pagination helper
-- 13 pre-existing callers (`GetACLs`, `DescribeEngineVersions`, `DescribeClusters`,
`DescribeMultiRegionClusters`, `DescribeMultiRegionParameterGroups`,
`DescribeMultiRegionParameters`, `DescribeReservedNodes`,
`DescribeReservedNodesOfferings`, `DescribeServiceUpdates`, `DescribeSnapshots`,
`DescribeSubnetGroups`, `DescribeUsers`, `DescribeParameterGroups`) plus, as of this pass,
`DescribeParameters` (14th). It encodes `NextToken` as the name of the first item of the
*next* page (`nextToken = getName(items[limit])`, inclusive), but its decode half
(`findStartIndex`) resumed at `i+1` -- the index *after* the matching item -- silently
dropping the very item the token named. A 5-item `MaxResults=1` walk returned items
`a, c, e`: `b` and `d` vanish with no error, no short page, nothing a client could detect.

Caught by `TestDescribeParameters_Pagination` (added for the newly-fixed `DescribeParameters`
op): expected the second page to hold the collection's remainder, got one fewer item than
expected. Root-caused to `findStartIndex`, not `DescribeParameters` itself. Fixed
`findStartIndex` to return `i` instead of `i+1`, which transitively fixes all 14 operations.
Confirmed by temporarily reverting the one-line fix and re-running the new direct unit test
(`TestPaginateItems_NoSkipAcrossPages`, `whitebox_test.go` -- in-package so it can call the
unexported helper directly): fails with exactly the `a,c,e` skip pattern pre-fix, passes
post-fix. Full `go test -race -count=1 ./services/memorydb/...` suite (all pre-existing
tests, including every one of the 13 other `paginateItems` callers' own tests) still passes
post-fix -- no existing test had the wrong skip-one behavior baked in as an expected result,
meaning this bug shipped silently undetected until this pass.

Two response cursors newly fixed to be populated at all (see per-op notes above and `gaps`):
`DescribeEvents` (no pagination applied whatsoever; not provably bounded -- up to 1000 events
per region, concatenated across every region; also required adding a deterministic sort,
since the pre-existing PARITY note correctly identified that this op's un-region-scoped map
iteration made ordering non-deterministic across calls, which this sweep's added sort now
resolves for pagination soundness -- the underlying cross-region leak that note also flagged
was separate and left open by this pass; RESOLVED 2026-08-30, wrapper-key sweep -- see the
`DescribeEvents` op entry and its `gaps` note) and `DescribeParameters` (no pagination
applied whatsoever; not provably bounded because `UpdateParameterGroup` accepts arbitrary new
parameter names with no validation against the known catalogue -- an adjacent gap, not fixed
this pass, but it defeats the "compile-time catalogue" argument that would otherwise have let
this cursor stay legitimately unpopulated).

No other response structs declaring `NextToken` were found unaccounted for (16 total across
`models_*.go`; all 16 now correctly populated: 14 pre-existing correct + 2 fixed this pass).

Tests: `services/memorydb/events_test.go` gained `TestDescribeEvents_Pagination`;
`services/memorydb/handler_parameter_groups_test.go` gained
`TestDescribeParameters_Pagination`; `services/memorydb/whitebox_test.go` gained
`TestPaginateItems_NoSkipAcrossPages`. All confirmed failing against unmodified code (the
first two via feature absence, the third via a temporary one-line revert) before the fix.

Gates: `go build ./...`, `go vet ./...` (repo-wide, clean), `go test -race -count=1
./services/memorydb/...` (pass, full suite including all pre-existing pagination tests),
`golangci-lint run ./services/memorydb/...` (0 issues).

## 2026-08-30 wrapper-key sweep: exhaustive request-field-read audit, one gap found (no bugs)

Method, independent of prior passes' per-op notes: derived the operation list straight from
`GetSupportedOperations()` (handler.go:48-105) rather than trusting this file's own prose --
46 strings registered, 45 real (`ExportSnapshot` deliberately unadvertised, see its own note
above). Then, for every `*Request`/`*Input` struct across every non-test `.go` file in this
package, cross-referenced each JSON-tagged field against a combined text search of the whole
package for `.FieldName` usage anywhere (handler, backend, or elsewhere) -- catching the
declared-but-never-read shape without trusting any single file's local context. Confirmed
protocol directly from the pinned SDK: `awsAwsjson11_*` prefix throughout
`memorydb@v1.36.4/deserializers.go` -- plain JSON-RPC 1.1 over `X-Amz-Target`, no legacy/query
path exists for this service to be reachable through.

**Result: exactly one unread field across the whole package** -- `createClusterRequest.
SnapshotArns` (see `gaps` above); ruled a missing-backend-data gap, not a bug, and documented
there rather than fixed. Everything else this sweep's structural scan flagged (`occ<=1`) turned
out to be a normal single legitimate read once cross-checked against the combined-package text
(the per-file-only version of this scan false-positived heavily on request structs defined in
`models_*.go` and consumed in a different `handler_*.go`/`*.go` file -- corrected before trusting
results).

**Negative checks, explicitly (per campaign brief, not previously logged this way in this
file):**
- **Listing that never consults its store**: none. Every `handle(List|Describe|Get)*` function
  calls `h.Backend.*`; scripted check across all `handler_*.go`, zero exceptions.
- **Handler that discards its entire request**: none. Every handler with a `body []byte` +
  `json.Unmarshal` decode path references at least one `req.Field` afterward; scripted check,
  zero exceptions.
- **Filter's value consumed without checking the filter's name**: `statusFilter`
  (`service_updates.go`) was the one candidate with filter-shaped semantics; re-confirmed (see
  2026-08-29 indexed-list-sweep note above) it uses `slices.Contains` over the full requested
  set, not a single-name assumption.
- **Ordering / tie-prone sorts**: the 14-op `paginateItems` cursor bug (see above) was this
  service's real instance of the class and is already fixed. Did not find an additional
  unfixed tie-prone sort this pass; every remaining paginated list's sort key (ClusterName,
  ACLName, SubnetGroupName, UserName, ParameterGroupName, SnapshotName, ReservationId) is the
  store's own unique key, so no tiebreak is needed regardless of walk order.

**Go-type spot-check**: `DataTiering` is `*bool` on the request side
(`createClusterRequest.DataTiering`, matching `CreateClusterInput.DataTiering *bool` in the
pinned SDK) and correctly converted to the real response-side `DataTieringStatus` string enum
("true"/"false") by `resolveDataTiering` before being written to `clusterObject.DataTiering
string` -- confirmed not a bool-for-enum mismatch on either side of the wire.

Gates: `go build ./services/memorydb/...`, `go vet ./services/memorydb/...` and `go vet ./...`
(repo-wide), `go test -race -count=1 ./services/memorydb/...`, `golangci-lint run
./services/memorydb/...`. No code changed this pass -- documentation-only (`gaps` entry above).

## 2026-09-07 errtargetaudit triage (gopherstack-me2v): 4 class A findings, 0 real bugs, 1 landmine

Screened this file first for a prior errtargetaudit/error-envelope entry -- none existed;
this block was genuinely untriaged.

Tool output: `operations with SDK ground truth: 45, resolved: 45, with an emission found: 40`
(no coverage warning; 45/45 resolved). 4 class A findings, grouped by root cause:

1. **`DescribeACLs`/`UpdateACL` attributed `ClusterNotFoundFault`** (sentinel reference,
   clusters.go). Both handlers call `h.Backend.DescribeClusters(ctx, "")` to compute an ACL's
   `Clusters` field and discard the error (`allClusters, _ := ...`). The sentinel the tool
   traced (`DescribeClusters`'s own `if name != "" { ... return nil, ErrClusterNotFound }`)
   cannot fire on this call path (name is always `""`), and even if it could, the return value
   is thrown away before `writeBackendError` ever sees it. False positive, doubly impossible --
   one-hop callee trace over a shared lookup helper called with a fixed empty filter.
2. **`DescribeUsers` attributed `ACLNotFoundFault`** (sentinel reference, acls.go): identical
   shape -- `handleDescribeUsers` calls `h.Backend.DescribeACLs(ctx, "")` and discards the
   error to compute `allACLs` for cluster-membership lookups. Same double impossibility. False
   positive.
3. **`CreateCluster` attributed `SnapshotNotFoundFault`** (sentinel reference, clusters.go,
   restore-from-`SnapshotName` branch): this one *is* CreateCluster's own code, not a
   mis-attributed callee, and the guard can fire (an arbitrary caller-supplied `SnapshotName`
   that doesn't exist in the store). Raw extraction from the pinned SDK
   (`deserializeOpErrorCreateCluster` in `memorydb@v1.36.4/deserializers.go`) confirms
   `CreateCluster`'s declared error set has no `SnapshotNotFoundFault` at all -- unlike
   `ACLName`/`SubnetGroupName`/`ParameterGroupName`/`MultiRegionClusterName` above it in the
   same function, which each have their own declared NotFound fault
   (`ACLNotFoundFault`/`SubnetGroupNotFoundFault`/`ParameterGroupNotFoundFault`/
   `MultiRegionClusterNotFoundFault`, all present in the same declared set). Real mismatch,
   no safe remedy: left a landmine comment at the call site (clusters.go) naming
   `InvalidParameterValueException` as the most likely real-AWS candidate without guessing a
   fix, and pinned current behavior (`SnapshotNotFoundFault`, unendorsed) with a new
   regression test, `TestErrCode_CreateCluster_SnapshotNotFound` (errcode_test.go).

Protocol confirmed: plain JSON-RPC 1.1 (`awsAwsjson11_*` symbols throughout
`memorydb@v1.36.4/deserializers.go`), routed by `X-Amz-Target`. Handler error shape
(`writeError` in handler.go) writes the standard AWS JSON 1.1 envelope,
`{"__type": errType, "message": message}`, status uniformly 400 for every declared fault
(confirmed against the aws-sdk-go v1 model: every one of MemoryDB's exception shapes has an
empty `error` trait). `writeBackendError` prefers an exact sentinel-to-fault mapping table
(`errCodeLookup`) before falling back to a coarse `awserr`-category switch.

**No pre-existing test asserted a wrong code for any of the 3 false positives** -- checked
`handler_acls_test.go`/`handler_users_test.go`/`handler_clusters_test.go` directly. All 3
false-positive call paths (`DescribeACLs`/`UpdateACL`/`DescribeUsers` computing their
"belongs to these clusters/ACLs" fields against an otherwise-empty store) are already
exercised by existing passing tests (`TestHandler_ACL_CRUD`'s "describe ACLs" case,
`TestHandler_UpdateACL`'s "updates ACL" case, `TestHandler_DescribeUsers_All`) -- if the
discarded-error/guard-cannot-fire reasoning above were wrong, those pre-existing tests would
already be failing with 400 instead of 200. No new tests added for the 3 dismissals; one new
test added for the landmine (see above).

**Fixed vs left**: 0 code-behavior changes. Landmine comment only (clusters.go, non-functional
-- confirmed `go build` and `golangci-lint` stay clean with it present). Neutered the new test
by swapping in a wrong expected code: compiled fine, test failed as expected
(`WRONG_CODE_FOR_NEUTER_CHECK` vs actual `SnapshotNotFoundFault`), then reverted.

Re-ran the tool after the change: still `4` class A findings for memorydb, same 4 ops/codes
(expected -- the fix here was a comment + a pinning test, not a code-emission change; the 3
false positives are inherent to the tool's one-hop trace over a shared helper, and the 4th is
a documented landmine, not a fix).

Gates: `go test -race -count=1 ./services/memorydb/...` (pass), `golangci-lint run
services/memorydb/...` (0 issues).

## 2026-09-07 gopherstack-2i0c: re-derived and confirmed the asymmetry, no remedy found

Re-verified the `CreateCluster` / `SnapshotNotFoundFault` finding above (item 3) rather
than trusting the prior pass's summary. Re-extracted the declared set directly from
`memorydb@v1.36.4/deserializers.go`'s `awsAwsjson11_deserializeOpErrorCreateCluster` (18
codes, matches the bd issue's list exactly) and cross-checked it against the live
`API_CreateCluster.html` Errors section: identical 18 codes, no `SnapshotNotFoundFault`.
This rules out model staleness as the explanation for the asymmetry -- it is a genuine
AWS modelling choice, not an artifact of the pinned SDK version.

Confirmed `SnapshotNotFoundFault` ("The specified snapshot does not exist.",
`types/errors.go`) *is* declared by `CopySnapshot` and `DeleteSnapshot` (both take a
`SnapshotName`), just not by `CreateCluster` -- textbook right-code-wrong-op. No doc
sentence anywhere (pinned SDK field comment, `docs-2.json`, or the live API reference)
states what `CreateCluster` actually returns for an unresolvable `SnapshotName`; the
`InvalidParameterValueException` candidate named in the landmine remains unconfirmed by
any such sentence -- its doc text ("The specified parameter value is not valid.") is
generic, not specific to this case. No fix applied; sharpened the landmine comment
(clusters.go) with this verification instead of leaving it as a bare assertion.

`TestErrCode_CreateCluster_SnapshotNotFound` left unchanged -- still pins the current
(unendorsed) `SnapshotNotFoundFault` behavior, correctly, since nothing changed.

Gates: `GOTOOLCHAIN=go1.26.6 go test -race ./services/memorydb/...` ok;
`GOTOOLCHAIN=go1.26.6 golangci-lint run ./services/memorydb/...` 0 issues. Re-ran
`cmd/errtargetaudit`: same finding, same line, confirming no emission change.

## 2026-09-08: writeError nil-on-write fall-through sweep (gopherstack-246v) -- clean

Part of the 12-service sweep for the elasticache class bug (gopherstack-8haq): a helper
that rejects a request via the local response writer and *returns* that writer's result
hands a caller doing `if err != nil { return err }` a `nil`, since the writer returns nil
after a successful write -- the rejection is silently skipped and the operation continues.

**Base writer**: `writeError` (`handler.go:536`) returns `c.JSON(status, errorResponse{...})`
directly -- nil on a successful write. `writeBackendError` (`handler.go:467`, a method)
wraps it: a loop over `errCodeLookup` plus a fallback switch, every branch `return
writeError(...)`.

**Method (mechanical).** A `go/parser`/`go/ast` script over every non-test `.go` file (46
files) found every function with a `return`-statement whose result is a bare call to
`writeError`, then fixed-point-expanded to any function bare-returning a call to an
already-found member -- 53 functions discovered: `writeBackendError`, all ~46 `handleXxx`
op handlers, and the 6 `dispatch`/`dispatchXxxOps` routing functions.

**Dispatch verified, not assumed.** `dispatch` and its sub-dispatchers use the same
`(bool, error)` handled-tuple chain as iotwireless/memorydb's sibling services:
`Handler()` -> `dispatch` -> `dispatchCoreOps`/`dispatchNewOps` -> `dispatchSnapshotAnd
EngineOps`/`dispatchMultiRegionOps`/`dispatchParameterAndShardOps`, each level doing `if
handled, result := h.dispatchX(...); handled { return result }` -- branches on the
`handled` bool, never on `result != nil`, so a matched op that rejected via `writeError`
(returning nil) still propagates by the bare `return result` inside the `handled` branch.
Read all 3 such sites (`handler.go:209-266`) confirming zero exceptions.
`dispatchCoreOps` itself resolves via a flat `memorydbCoreOps` map and returns `fn(h, ctx,
c, body)` directly.

Every call site of `writeError` and `writeBackendError` across the package (174 total) was
enumerated: 171 are direct `return writeError(...)` / `return writeBackendError(...)` /
`return h.handleXxx(...)` sites; the other 3 are the `(handled, result)` dispatch-chain
assigns above, verified safe. Zero `_ =` discards, zero stored-single-value-and-`!=
nil`-checked sites. Independently confirmed by grepping every non-test-file occurrence of
`writeError(`/`writeBackendError(` outside their own definitions: every one is immediately
preceded by `return` on the same line.

**No instance of the broken shape exists in memorydb.** No code changed. Gates re-run for
the record: `GOTOOLCHAIN=go1.27.0 golangci-lint run ./services/memorydb/...` 0 issues;
`GOTOOLCHAIN=go1.27.0 go test -race ./services/memorydb/...` ok.
