# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: rds
sdk_module: aws-sdk-go-v2/service/rds@v1.124.1
last_audit_commit:                                # unknown: pass ran without git access at write time (git use was out of scope), never backfilled -- gopherstack-33in
last_audit_date: 2026-07-25
overall: A              # RESTORED A->A (gopherstack-vhw2 strict-phantom-check pass, 2026-08-05):
                       # both defects behind the 2026-07-31 A->A- downgrade (recorded verbatim
                       # below) are resolved, and nothing new was found in their place.
                       # (1) "DescribeCustomDBEngineVersions" -- the fabricated action -- was
                       # already removed from the wire surface by the 2026-07-31 pass itself:
                       # handler_dispatch.go's dispatchExtended16 doc comment (~:577) documents it
                       # as deliberately unrouted, DescribeDBEngineVersions merges in custom engine
                       # versions instead, and it no longer appears in GetSupportedOperations() or
                       # in the sdkcheck reverse-phantom output. (2) "GetPerformanceInsightsMetrics"
                       # -- the one operation still flagged by the reverse check -- is no longer an
                       # undisclosed gap: pkgs/sdkcheck/check.go now has a documented, per-client
                       # phantomAllowlist (closing gopherstack-vhw2), and this operation is listed
                       # under "*rds.Client" with the same justification already on record in the
                       # performance_insights family note and the gaps: entry below (the real
                       # operation is GetResourceMetrics, on a separate "pi" SDK client this repo
                       # does not depend on; kept wired because it is real, seeded, non-stub
                       # functionality with no wire-accurate replacement to redirect callers to).
                       # The reverse phantom check is now a hard failure repo-wide (no more
                       # reporting-only tb.Logf) -- rds passes it cleanly via that allowlist entry,
                       # an explicit, reviewed exception rather than a silent tolerance. Both
                       # issues were documentation/wire-accuracy gaps, not functionality
                       # regressions, and both are now fixed for real: TestSDKCompleteness
                       # (services/rds/dispatch_test.go) is green against the strict check, and
                       # DescribeCustomDBEngineVersions's removal already had regression coverage
                       # from the prior pass (TestDescribeCustomDBEngineVersions_ViaHandler/
                       # _NotAdvertised). No other issue was found this pass; the pre-existing,
                       # unrelated gaps below (DescribeDBEngineVersions/
                       # DescribeOrderableDBInstanceOptions pagination,
                       # DescribeServerlessV2PlatformVersions' honestly-empty catalog) are the same
                       # ones this service already carried the last time it held A, and did not
                       # block that grade then either.
                       #
                       # Everything from here through the next "Everything below this line" marker
                       # is retained history from the 2026-07-31 A->A- downgrade pass, kept verbatim:
                       #
                       # Everything below this line is the PRIOR (2026-07-25) A audit's own
                       # overall note, kept verbatim for history: this pass closed all three gaps
                       # carried forward from the 2026-07-23 A- audit -- each was previously
                       # deferred as "too invasive"; that
                       # deferral is retracted, all three are fixed for real with regression
                       # tests, not just re-labeled:
                       # (1) CASE-SENSITIVE IDENTIFIERS (real gap, now fixed). Added
                       # pkgs/strs (Fold/Equal/ContainsFold), a new shared package (not
                       # RDS-specific) for case-insensitive string comparison, and normalized
                       # every store boundary for the six identifier families real AWS folds
                       # to lowercase internally: DBInstanceIdentifier, DBClusterIdentifier,
                       # DBSnapshotIdentifier, DBClusterSnapshotIdentifier,
                       # DBParameterGroupName, DBClusterParameterGroupName (the last shared by
                       # both parameterGroups and clusterParameterGroups tables). Every
                       # store.Table[V] keyFn for these six now folds through strs.Fold, and
                       # every raw-string Get/Has/Delete call site (116 across
                       # db_instances.go/db_clusters.go/db_snapshots.go/cluster_snapshots.go/
                       # parameter_groups.go/cluster_parameter_groups.go/roles.go/
                       # maintenance.go/log_files.go/lifecycle.go/activity_stream.go/
                       # automated_backups.go/cluster_endpoints.go) now folds its argument too
                       # -- Get/Has/Delete do NOT invoke keyFn themselves (see pkgs/store's
                       # package doc), so folding only the keyFn would have normalized Put but
                       # left every lookup still case-sensitive. Also fixed the satellite
                       # snapshotAttributes/clusterSnapshotAttributes tables (same identifier
                       # family, would otherwise have re-introduced the same bug one level
                       # removed), Describe* Filters matching on identifier-shaped filter
                       # names (containsFold), and every plain (non-Table) map keyed off these
                       # identifiers that a delete call could touch under a different casing
                       # than create used (tags ARN, instanceRoles/clusterRoles,
                       # instanceReadyAt/clusterReadyAt, automatedBackups) by keying those off
                       # the just-fetched resource's canonical (as-created) identifier instead
                       # of the raw caller-supplied string. See Notes and the
                       # TestCreate{DBInstance,DBCluster,DBSnapshot}_CaseInsensitiveIdentifier /
                       # TestClusterSnapshot_Duplicate / TestDBParameterGroup_Duplicate /
                       # TestClusterPG_CaseInsensitiveIdentifier table tests.
                       # (2) ENGINE NOT VALIDATED (real gap, now fixed). Added
                       # validateDBInstanceEngine/validateDBClusterEngine (engine_versions.go),
                       # checking against validDBInstanceEngines/validDBClusterEngines --
                       # field-diffed from aws-sdk-go-v2's CreateDBInstanceInput/
                       # CreateDBClusterInput "Valid Values" doc comments (24 engines for
                       # instances incl. RDS Custom/Db2/SQL Server; 5 for clusters --
                       # Aurora/Multi-AZ/Neptune only, so e.g. "oracle-ee" is valid for
                       # CreateDBInstance but InvalidParameterValue for CreateDBCluster).
                       # Empty Engine is still accepted (defaulted, as before -- Engine is
                       # technically required on real AWS but many existing callers relied on
                       # the default, and that default always resolves to a valid engine).
                       # Fixing this surfaced a REAL pre-existing bug in three tests
                       # (performance_insights_test.go x2, dispatch_test.go) that called
                       # CreateDBInstance with its positional args in the wrong order
                       # (engine/instanceClass swapped, i.e. Engine="db.t3.micro"); the calls
                       # "worked" only because nothing validated Engine before, exactly
                       # demonstrating why this was a real gap and not a cosmetic one. Fixed
                       # those three call sites and one more test (persistence_test.go) that
                       # used the invalid legacy engine string "aurora" (not documented as
                       # valid on the current SDK) instead of "aurora-mysql". See Notes and
                       # TestCreateDBInstance_EngineValidation / TestCreateDBCluster_EngineValidation.
                       # (3) DBShardGroup/Integration PARTIAL FIELD COVERAGE (real gap, now
                       # fixed). DBShardGroup gained DBShardGroupArn/DBShardGroupResourceId
                       # wire fields (PubliclyAccessible already existed on the Go struct but
                       # was never serialized) -- and, since field-diffing the SINGLE Create
                       # response surfaced that Delete/Modify/Reboot were ALSO missing most of
                       # these fields (not just the three named in the prior gap), all four
                       # ops' XML responses were brought up to the real SDK's full flat
                       # DBShardGroup field set. Integration gained KMSKeyId/CreateTime/Tags/
                       # Errors on the wire for Create/Delete/Modify (Tags backed by the same
                       # shared ARN-keyed tags map every other RDS resource uses --
                       # DeleteIntegration now also cascade-cleans its tags, closing a leak
                       # that would otherwise have opened the moment Tags became reachable).
                       # See Notes and TestDBShardGroup_WireFieldsPresentOnAllOps /
                       # TestIntegration_WireFieldsPresentOnAllOps.
ops:
  DeleteDBInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDBCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascade-delete of cluster endpoints/tags FIXED this pass — see leaks"}
  StartActivityStream: {wire: ok, errors: ok, state: ok, persist: ok, note: "de-deferred this pass — see families/activity_streams"}
  StopActivityStream: {wire: ok, errors: ok, state: ok, persist: ok, note: "de-deferred this pass — see families/activity_streams"}
  ModifyActivityStream: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire-shape + error-code bugs FIXED this pass — see families/activity_streams"}
  DescribeDBInstances: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDBInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  ModifyDBInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  StartDBInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  StopDBInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  RebootDBInstance: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDBInstanceReadReplica: {wire: ok, errors: ok, state: ok, persist: ok}
  PromoteReadReplica: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDBCluster: {wire: ok, errors: ok, state: ok, persist: ok}
  ModifyDBCluster: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeDBClusters: {wire: ok, errors: ok, state: ok, persist: ok, note: "Filters support added this pass — see families/describe_filters"}
  CreateDBSnapshot: {wire: ok, errors: ok, state: ok, persist: ok}
  CopyDBSnapshot: {wire: ok, errors: ok, state: ok, persist: ok}
  RestoreDBInstanceFromDBSnapshot: {wire: ok, errors: ok, state: ok, persist: ok}
  RestoreDBInstanceToPointInTime: {wire: ok, errors: ok, state: ok, persist: ok}
  RestoreDBInstanceFromS3: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "gopherstack-afi1: 3 of 7 required members -- S3IngestionRoleArn, SourceEngine, SourceEngineVersion (rds@v1.124.1 api_op_RestoreDBInstanceFromS3.go:84,91,100) -- were dropped by the handler (vals.Get only read DBInstanceIdentifier/Engine/DBInstanceClass/S3BucketName); Engine and DBInstanceClass were also unvalidated as required despite being so on the wire. All 7 now validated present (InvalidParameterValue -- this op's own deserializeOpError switch has no validation-style exception, same convention already used for the pre-existing DBInstanceIdentifier/S3BucketName checks). The 3 new fields describe the S3 ingestion source only; DBInstance's real response shape (types/types.go) has no members for them, so they're validated but not persisted -- nothing to echo them into."}
  CreateDBClusterSnapshot: {wire: ok, errors: ok, state: ok, persist: ok}
  RestoreDBClusterFromS3: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "gopherstack-afi1: same bug class as RestoreDBInstanceFromS3 -- 3 of 7 required members (S3IngestionRoleArn, SourceEngine, SourceEngineVersion; rds@v1.124.1 api_op_RestoreDBClusterFromS3.go:90,97,105,114) were dropped by the handler; Engine and MasterUsername were also unvalidated as required. All 7 now validated present (InvalidParameterValue, same no-declared-validation-exception convention as RestoreDBInstanceFromS3's own deserializeOpError switch). Validated but not persisted -- DBCluster's real response shape has no members for the S3-ingestion-source fields."}
  CreateDBParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ModifyDBParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  ResetDBParameterGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDBSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateOptionGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDBShardGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire nesting bug fixed this pass — see gaps/Notes"}
  DeleteDBShardGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire nesting bug fixed this pass — see gaps/Notes"}
  ModifyDBShardGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire nesting bug fixed this pass — see gaps/Notes"}
  RebootDBShardGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire nesting bug fixed this pass — see gaps/Notes"}
  DescribeDBShardGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "list shape was already correct"}
  CreateIntegration: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire nesting bug fixed this pass — see gaps/Notes"}
  DeleteIntegration: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire nesting bug fixed this pass — see gaps/Notes"}
  ModifyIntegration: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire nesting bug fixed this pass — see gaps/Notes"}
  DescribeIntegrations: {wire: ok, errors: ok, state: ok, persist: ok, note: "list shape was already correct"}
  CreateCustomDBEngineVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire nesting + wrong field name fixed this pass — see gaps/Notes"}
  DeleteCustomDBEngineVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire nesting bug fixed this pass — see gaps/Notes"}
  ModifyCustomDBEngineVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "wire nesting + wrong field name fixed this pass — see gaps/Notes"}
  CreateTenantDatabase: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified this pass — real AWS output nests under <TenantDatabase>, matches gopherstack"}
  DeleteTenantDatabase: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified this pass"}
  ModifyTenantDatabase: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified this pass"}
  DescribeTenantDatabases: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified this pass"}
  CreateDBSecurityGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified this pass — real AWS output nests under <DBSecurityGroup>, matches gopherstack"}
  AuthorizeDBSecurityGroupIngress: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified this pass"}
  RevokeDBSecurityGroupIngress: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified this pass"}
  ModifyCertificates: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified this pass — nests under <Certificate>, matches"}
  ModifyDBRecommendation: {wire: ok, errors: ok, state: ok, persist: ok, note: "verified this pass — nests under <DBRecommendation>, matches"}
  CreateDBProxy/DeleteDBProxy/ModifyDBProxy: {wire: ok, errors: ok, state: ok, persist: ok, note: "spot-verified this pass — nests under <DBProxy>, matches (family already ok per prior audits)"}
  PurchaseReservedDBInstancesOffering: {wire: ok, errors: ok, state: ok, persist: ok, note: "spot-verified this pass — nests under <ReservedDBInstance>, matches"}
  DescribeServerlessV2PlatformVersions: {wire: ok, errors: ok, state: partial, persist: n/a, note: >
    NEW this pass (SDK bump to v1.123.0 added this op; TestSDKCompleteness flagged it).
    Confirmed request/response member names directly against
    aws-sdk-go-v2/service/rds@v1.123.0's api_op_DescribeServerlessV2PlatformVersions.go
    (request: DefaultOnly, Engine, Filters, IncludeAll, Marker, MaxRecords,
    ServerlessV2PlatformVersion; response: Marker, ServerlessV2PlatformVersions) and
    types.ServerlessV2PlatformVersionInfo/types.ServerlessV2FeaturesSupport (go doc). Engine
    is validated against the two values that file's own "Valid Values" doc comment
    documents (aurora-mysql, aurora-postgresql) -- InvalidParameterValue otherwise, matching
    validateDBInstanceEngine/validateDBClusterEngine's existing pattern. Filters is
    accepted-but-ignored (that action's own doc comment says "This parameter isn't
    currently supported", i.e. real AWS itself doesn't implement it -- same precedent as
    the DescribeEvents Filters correction noted in the 2026-07-23 pass below). Uses the
    shared paginateDescribe/Marker/MaxRecords convention (no per-op MaxRecords bound
    enforced beyond the generic >0 check, consistent with every other Describe op in this
    file -- none of them enforce the SDK doc's specific numeric range either).
    state: partial because the catalog this backend serves is deliberately EMPTY: unlike
    DBEngineVersion or DBMajorEngineVersion, ServerlessV2PlatformVersionInfo.
    ServerlessV2PlatformVersion is a plain *string with no SDK-side enum or constant list of
    real version numbers anywhere in the installed module -- inventing plausible-looking
    version strings ("3", "4", ...) and descriptions would fabricate data indistinguishable
    from genuine AWS output with nothing in this SDK module to verify them against. Returns
    an honestly empty ServerlessV2PlatformVersions list instead (see reference_data.go's doc
    comment on DescribeServerlessV2PlatformVersions). The Engine/version/DefaultOnly/
    IncludeAll filtering logic is real code, not dead code -- it is written to apply
    correctly the moment genuine catalog rows are ever added, just currently applied to zero
    rows. New files: none (added to the existing reference_data.go/handler_reference_data.go
    pairing, same as DescribeDBMajorEngineVersions/DescribeSourceRegions).}
families:
  db_instance_lifecycle: {status: ok, note: "creating->available->modifying/deleting state machine via instanceReadyAt + self-terminating reconciler goroutine (backend.go scheduleReconcilerLocked); verified transitions, DeletionProtection guard, already-deleting guard"}
  db_cluster_lifecycle: {status: ok, note: "cluster members, reader/writer endpoint synthesis, ServerlessV2ScalingConfiguration, start/stop/failover/reboot all mutate real state"}
  snapshots_manual_automated: {status: ok, note: "CreateDBSnapshot/CopyDBSnapshot/Delete/Describe/Restore all real; SnapshotType manual vs automated distinguished; final-snapshot-on-delete gap fixed this pass (see Notes)"}
  parameter_groups: {status: ok, note: "apply-method immediate vs pending-reboot honored in ModifyDBParameterGroup/ApplyPendingMaintenanceAction path; Reset/Copy real"}
  ApplyPendingMaintenanceAction: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "FIXED this pass (gopherstack-u90v): handler only read ResourceIdentifier/ApplyAction; OptInType is required (rds@v1.124.1 api_op_ApplyPendingMaintenanceAction.go:53-63, three legal values -- immediate/next-maintenance/undo-opt-in, from the field's own doc comment since it's an untyped *string on the real SDK, not a generated enum). Now required and validated against that enum (InvalidParameterValue if absent or unrecognized -- this op's own deserializeOpError switch has only InvalidDBClusterStateFault/InvalidDBInstanceState/ResourceNotFoundFault, none of which fit a missing-parameter case, so this uses the same ErrInvalidParameter/\"InvalidParameterValue\" convention already used elsewhere in this handler for query-parameter validation). OptInType's actual immediate/next-window/undo semantics remain unimplemented: this backend has no mechanism anywhere that generates a real pending maintenance action for any resource (DescribePendingMaintenanceActions is hardcoded to return none), so there is no real state for OptInType to act on -- rejecting invalid values instead of silently accepting them is judged more faithful than fabricating apply/schedule behavior with nothing to schedule against, consistent with the equivalent elasticache fix earlier this session. Lowest severity of this pass's five fixes, as flagged in the originating issue."}
  subnet_groups: {status: ok, note: "CRUD verified against DBSubnetGroup shape"}
  option_groups: {status: ok, note: "CRUD + Copy + option add/remove real"}
  read_replicas: {status: ok, note: "source linkage bidirectional (ReplicaSourceDBInstanceIdentifier / ReadReplicaIdentifiers), promote clears linkage, cross-region replica path uses defaults when source not locally resolvable"}
  events_and_subscriptions: {status: ok, note: "ring-buffered Events (maxEvents cap prevents unbounded growth); EventSubscription CRUD + source-identifier add/remove real"}
  engine_versions_and_orderable_options: {status: ok, note: "DescribeDBEngineVersions/DescribeOrderableDBInstanceOptions/DescribeDBMajorEngineVersions all backed by real (small, static) catalogs — not a stub since callers get consistent, well-shaped data; no engine-name validation on Create (see gaps). UPDATED (parity-5/phantom-triage, 2026-07-31): DescribeDBEngineVersions now also merges in custom engine versions (previously only reachable via the fabricated DescribeCustomDBEngineVersions action) — see custom_db_engine_versions family and overall: header."}
  tags: {status: ok, note: "AddTagsToResource/RemoveTagsFromResource/ListTagsForResource use pkgs/tags-style per-ARN map, cleaned up on every delete path (instance, cluster, snapshot, option group, param group, cluster endpoint — verified via TestRDSBackend_TagsCleanedUpOnDelete table). VERIFIED CLEAN (wrapper-key sweep, 2026-08-29): checked for the stepfunctions-class bug (a Tags field typed as a Go map when the SDK sends an array, or vice versa). rds@v1.124.1 serializers.go:12403-12408/17967-17972 confirm AddTagsToResource.Tags serializes as Tags.Tag.N.Key/Value (awsAwsquery_serializeDocumentTagList, array element name 'Tag') and RemoveTagsFromResource.TagKeys as TagKeys.member.N (awsAwsquery_serializeDocumentKeyList, array element name 'member') — handler_tags.go's parseTagEntries/parseTagKeyMembers already parse exactly these wrapper names. Confirmed via TestTagResourceFamily_SDKRoundTrip (tag_resource_sdk_test.go) driving the real SDK client through AddTagsToResource/RemoveTagsFromResource/ListTagsForResource."}
  pagination: {status: ok, note: "Marker/MaxRecords via pkgs/page.Page[T] (paginateDescribe) — consistent across all Describe* ops; DescribeDBClusterSnapshots and DescribeEvents were missing pagination entirely (returned every row regardless of MaxRecords) — FIXED this pass, see Notes"}
  describe_filters: {status: ok, note: "DescribeDBInstances Filters (db-cluster-id/db-instance-id/dbi-resource-id/domain/engine) added prior pass; DescribeDBClusters (clone-group-id/db-cluster-id/db-cluster-resource-id/domain/engine), DescribeDBSnapshots (db-instance-id/db-snapshot-id/dbi-resource-id/snapshot-type/engine), and DescribeDBClusterSnapshots (db-cluster-id/db-cluster-snapshot-id/snapshot-type/engine) Filters added a prior pass. DescribeEvents Filters intentionally left unimplemented: the real aws-sdk-go-v2 DescribeEventsInput.Filters doc comment reads literally 'This parameter isn't currently supported' — the emulator already matches real AWS by accepting-but-ignoring it, which is NOT a gap (prior ledger incorrectly listed it as one). FIXED THIS PASS (wrapper-key sweep, 2026-08-29): the shared parseDescribeFilters (handler_db_instances.go, request-direction, all 4 filtered ops) read Filters.Filter.N.Values.member.M — rds@v1.124.1 serializers.go:11730 awsAwsquery_serializeDocumentFilterValueList's array element name is 'Value', never 'member', so a real client's Filters values never reached the parser and every filtered Describe call silently returned an unfiltered (in this parser's specific empty-values-list case, actually an OVER-filtered/empty) result. Corrected to Values.Value.M; see Notes."}
  global_clusters: {status: ok, note: "Create/Modify/Delete/Describe + Remove/Failover/SwitchoverGlobalCluster real"}
  blue_green_deployments: {status: ok, note: "Create/Describe/Delete/Switchover real (refinement1)"}
  db_proxies: {status: ok, note: "proxy/proxy-target/proxy-target-group/proxy-endpoint CRUD real (refinement3)"}
  reserved_instances: {status: ok, note: "Purchase + Describe(Offerings) real"}
  performance_insights: {status: ok, note: "GetPerformanceInsightsMetrics requires seeded data via SetPerformanceInsightsData — not a fabricated-on-the-fly stub; batch3_test.go.rej/.patch cruft from a prior sweep's already-applied fix removed this pass. CAVEAT (parity-5/phantom-triage, 2026-07-31): 'GetPerformanceInsightsMetrics' is not a real operation name on either client — real AWS Performance Insights functionality is GetResourceMetrics, on a separate 'pi' SDK client with its own endpoint/protocol (not in this repo's go.mod), not an RDS client operation. Real RDS SDK clients would never send this Action, and real 'pi' clients would never reach this handler. Kept wired (real, useful functionality; no wire-shape-accurate replacement exists to redirect it to) but the sdkcheck reverse check (gopherstack-vhw2) correctly flags it as a phantom against the RDS client and will keep doing so until this is either renamed/reshaped to match a real op or moved to a dedicated pi service."}
  error_codes: {status: ok, note: "awserr sentinels map to correct AWS fault codes with correct HTTP status (400, uniformly, per the AWS Query-protocol convention — status does not vary by fault type, only the <Code> element does) via rdsErrorCode() in handler_dispatch.go. FIXED this pass: field-diffed the whole mapping table against aws-sdk-go-v2's types/errors.go ErrorCode() methods (the ground truth for wire codes) and found (a) a systemic missing-'Fault'-suffix bug on DBClusterNotFound(Fault)/DBClusterAlreadyExists(Fault)/DBClusterSnapshotNotFound(Fault)/DBClusterSnapshotAlreadyExists(Fault)/DBClusterEndpointNotFound(Fault)/DBClusterEndpointAlreadyExists(Fault)/DBClusterAutomatedBackupNotFound(Fault)/GlobalClusterNotFound(Fault)/GlobalClusterAlreadyExists(Fault)/BlueGreenDeploymentNotFound(Fault)/BlueGreenDeploymentAlreadyExists(Fault)/IntegrationNotFound(Fault)/IntegrationAlreadyExists(Fault)/OptionGroupNotFound(Fault)/OptionGroupAlreadyExists(Fault) — 15 codes total, each individually confirmed against the real SDK since AWS is inconsistent about the suffix (DBInstanceNotFound genuinely has none); and (b) ErrDBProxyAlreadyExists/ErrDBProxyEndpointAlreadyExists/ErrCannotDeleteDefaultProxyEndpoint/ErrActivityStreamAlreadyStarted/ErrActivityStreamNotStarted had NO entry in the mapping table at all, so errors.Is never matched and these fell through to an unmapped code → 500 InternalFailure instead of the correct 400 client error. See Notes and TestRDSErrorCodes_FaultSuffix (error_codes_test.go)."}
  leaks: {status: ok, note: "single reconciler goroutine per backend; self-terminates when instanceReadyAt/clusterReadyAt both empty (no ticker leak); FOUND and FIXED this pass: DeleteDBCluster did not cascade-delete the deleted cluster's custom cluster endpoints (or their tags) — a real ghost-row leak, see top-level leaks: entry below"}
  instance_iam_roles: {status: ok, note: "FIXED 2026-08-12 (gopherstack-i101): AddRoleToDBInstance/RemoveRoleFromDBInstance dropped the required FeatureName (rds@v1.124.1 api_op_AddRoleToDBInstance.go:39-43 marks it required; it selects the feature slot, e.g. S3_INTEGRATION vs SQLSERVER_AUDIT), so two roles associated for different features on the same instance collapsed into one. instanceRoles is now map[instanceID]map[featureName]roleARN instead of map[instanceID][]roleARN; Add sets/replaces the (instance, feature) slot, Remove only clears it when the stored role ARN for that feature matches the one requested. Snapshot version bumped 1->2 since this field's on-wire JSON shape changed. Scoped to instance roles only -- see cluster_iam_roles below for the cluster-side follow-up (gopherstack-1jkv)."}
  cluster_iam_roles: {status: partial, note: "FIXED 2026-08-13 (gopherstack-1jkv, the cluster-side follow-up to gopherstack-i101): two defects. (1) Write side: AddRoleToDBCluster/RemoveRoleFromDBCluster never read FeatureName from the request at all (not even into a discarded local), and clusterRoles stored only a flat []string of role ARNs deduped by slices.Contains(ARN) -- so associating the same RoleArn with a second, different, explicitly-supplied FeatureName was silently dropped as a duplicate no-op, discarding that association with no error and no trace. FeatureName is now tracked: clusterRoles is map[clusterID][]DBClusterRole{RoleArn,FeatureName,Status}; AddRoleToDBCluster replaces the existing association for the same FeatureName (mirrors i101's instance-side 'replace the feature slot' semantics) and RemoveRoleFromDBCluster only removes the (FeatureName, RoleArn) pair that matches exactly. Snapshot version bumped 2->3 (map[string][]string cannot decode as map[string][]DBClusterRole; the version-mismatch guard discards ALL rds state, not just this field). (2) Read side, found independently while verifying the fix: DescribeDBClusters (and every other cluster-returning op) never emitted AssociatedRoles at all -- xmlDBCluster had no such field, so clusterRoles data was invisible on the wire regardless of how it was stored. Added AssociatedRoles (wrapped DBClusterRole list; element names/nesting confirmed against deserializers.go's awsAwsquery_deserializeDocumentDBClusterRole(s), not just the type's doc comment) via a new ClusterAssociatedRoles backend accessor threaded through toXMLCluster at all 12 call sites. status: partial because of an unresolved half: FeatureName is OPTIONAL on both cluster ops (rds@v1.124.1 api_op_AddRoleToDBCluster.go:39-43 does not mark it required, unlike the instance op), so real AWS's behavior when a client adds two different roles while omitting FeatureName on both is unverified -- neither the doc comment nor the deserializer's error-code switch (which does confirm DBClusterRoleAlreadyExists/DBClusterRoleNotFound/DBClusterRoleQuotaExceeded exist as possible faults, deserializers.go:93-97, but not the exact dedup key that triggers them) settles it. PLACEHOLDER (not SDK-verified): this repo treats the omitted-FeatureName case as its own bucket keyed by (FeatureName==\"\", RoleArn) -- two different roles both added without FeatureName both persist, matching this emulator's pre-fix behavior for that specific bucket rather than guessing new collapsing or new AlreadyExists-error semantics. See TestAddRoleToDBCluster_OmittedFeatureNamePlaceholder_RealSDKClient (cluster_roles_sdk_test.go) and gopherstack-1jkv, which stays open pending real-AWS evidence."}
  db_shard_groups: {status: ok, note: "Aurora Limitless shard groups — CRUD + Reboot real state; wire-shape bug (extra nesting) on Create/Delete/Modify/Reboot fixed a prior pass. THIS pass: field-diffed against the real DBShardGroup output structs and added the previously-missing DBShardGroupArn/DBShardGroupResourceId/PubliclyAccessible to ALL FOUR mutating ops' XML responses (not just Create) — field coverage now complete. See Notes and TestDBShardGroup_WireFieldsPresentOnAllOps."}
  integrations: {status: ok, note: "zero-ETL Redshift integrations — CRUD real state; wire-shape bug (extra nesting) on Create/Delete/Modify fixed a prior pass. THIS pass: added the previously-missing KMSKeyId/CreateTime/Tags/Errors to Create/Delete/Modify's XML responses (backed by the shared per-ARN tags map, with cascade-cleanup on delete) — field coverage now complete. See Notes and TestIntegration_WireFieldsPresentOnAllOps."}
  custom_db_engine_versions: {status: ok, note: "wire-shape bug (extra nesting + wrong field name for description) on Create/Delete/Modify FIXED this pass, see gaps/Notes. FIXED (parity-5/phantom-triage, 2026-07-31): the 'Describe' side of this family was a fabricated operation — 'DescribeCustomDBEngineVersions' is not a real RDS action; the real API returns custom engine versions from DescribeDBEngineVersions (see that op's row/family), distinguished only by their Engine value. Removed the fabricated action/handler/response shape from the wire surface (a prior pass's own test had asserted it 'should' be in GetSupportedOperations, encoding the defect); DescribeDBEngineVersions now merges in custom engine versions so the real op actually surfaces them. See overall: header and TestDescribeCustomDBEngineVersions_ViaHandler/_NotAdvertised."}
  tenant_databases: {status: ok, note: "re-verified this pass against the real SDK's CreateTenantDatabaseOutput/DeleteTenantDatabaseOutput/ModifyTenantDatabaseOutput shapes (these DO nest under <TenantDatabase>, unlike shard groups/integrations) — no bug found, ledger's prior 'spot-checked only' caveat is now resolved to ok"}
  db_security_groups: {status: ok, note: "re-verified this pass (EC2-Classic legacy) — CreateDBSecurityGroupOutput/AuthorizeDBSecurityGroupIngressOutput/RevokeDBSecurityGroupIngressOutput all nest under <DBSecurityGroup> in the real SDK, matches gopherstack; no bug found, ledger's prior 'spot-checked only' caveat is now resolved to ok"}
  activity_streams: {status: ok, note: "de-deferred this pass: field-diffed Start/Stop/ModifyActivityStream against aws-sdk-go-v2's StartActivityStreamOutput/StopActivityStreamOutput/ModifyActivityStreamOutput. Start/Stop already matched (flat KinesisStreamName/KmsKeyId/Status/Mode/ApplyImmediately fields, correct — these ops were never affected by the shard-group/integration nesting bug class since their outputs were always flat in gopherstack). ModifyActivityStream had a real disguised-stub bug: it emitted an invented <AuditPolicy> element that does not exist on the real output (the real field is PolicyStatus, of type ActivityStreamPolicyStatus) and omitted the real KinesisStreamName/Mode members — FIXED, see Notes. Also fixed: cluster-not-found on all three ops returned InvalidParameterValue instead of the correct DBClusterNotFoundFault. Test coverage was previously zero for this family; added activity_stream_test.go (lifecycle, not-found, and backend-error-path tests)."}
gaps:
  - "FIXED 2026-09-07 (gopherstack-1cjz, closes a gap the gopherstack-uao2 entry below opened and flagged in its own text: PromoteReadReplicaDBCluster left the promoted cluster still claiming ReplicationSourceIdentifier and left its former source still listing it in ReadReplicaIdentifiers, since uao2 wired that linkage through Create/Delete but not Promote. Mirrors the instance-level PromoteReadReplica (db_instances.go), which already cleared both sides: promote now strips the promoted cluster's ID from its source's ReadReplicaIdentifiers (idEqual-compared against the canonical DBClusterIdentifier, matching Delete's own comparison) and clears the promoted cluster's own ReplicationSourceIdentifier. Guards for a source that no longer exists (uao2 established deleting a source orphans its replicas rather than refusing or cascading, so a promoted replica may have no live source -- promote must not error in that case, and does not). Regression coverage: TestPromoteReadReplicaDBCluster_ClearsLinkage (two replicas, positively asserts the survivor stays in the source's ReadReplicaIdentifiers while the promoted one is gone, avoiding the omitempty-hides-empty-either-way hollow-test trap uao2's own first delete-cascade test fell into) and TestPromoteReadReplicaDBCluster_OrphanedSource (db_clusters_operations_test.go) -- both confirmed to fail against unmodified code."
  - "FIXED 2026-09-07 (gopherstack-uao2, the fix for the 2026-09-07 gopherstack-z1sd triage entry recorded below verbatim). DBCluster now carries ReplicationSourceIdentifier and ReadReplicaIdentifiers (models.go), CreateDBCluster parses ReplicationSourceIdentifier (via DBClusterOptions, mirroring how AvailabilityZones/BacktrackWindow are already create-only fields threaded through that shared options struct) and requires the named source cluster to already exist (DBClusterNotFoundFault otherwise -- CreateDBCluster's own deserializeOpError declares that fault, confirmed by grep), and both directions are now on the wire (ReplicationSourceIdentifier flat, ReadReplicaIdentifiers wrapped -- wire shape and element names confirmed against deserializers.go's awsAwsquery_deserializeDocumentDBCluster/awsAwsquery_deserializeDocumentReadReplicaIdentifierList, which differ from the instance-level ReadReplicaDBInstanceIdentifiers>ReadReplicaDBInstanceIdentifier wrapping -- clusters use ReadReplicaIdentifiers>ReadReplicaIdentifier instead). Mirrors db_instances.go's CreateDBInstanceReadReplica pattern exactly, including its delete-time behavior: deleting a replica cluster removes it from its source's ReadReplicaIdentifiers (DeleteDBClusterWithOptions); deleting a source cluster while replicas exist is NOT refused and does NOT cascade-clear the replicas' ReplicationSourceIdentifier -- they orphan, matching CreateDBInstanceReadReplica's own instance-level precedent exactly (no doc evidence for either a refusal or a cascade exists at either level, and this repo declines to invent either without it). Two things deliberately NOT touched this pass, scope-fenced to the linkage itself: (1) PromoteReadReplicaDBCluster (already existed pre-fix) does not clear ReplicationSourceIdentifier or remove the promoted cluster from its former source's ReadReplicaIdentifiers, unlike the instance-level PromoteReadReplica which does both -- so promoting a replica cluster now leaves stale/incorrect linkage data instead of the previously-inert no-op it was; flagged, not fixed, needs its own bd issue. (2) DBInstance's cluster-crossing fields (ReadReplicaSourceDBClusterIdentifier/ReadReplicaDBClusterIdentifiers, types.go:2308/2298, needed only when an instance's replication source/target is a cluster rather than another instance) remain unmodeled -- out of scope for this pass, which was cluster-to-cluster linkage only. The docdb twin of this exact gap (services/docdb/PARITY.md) was left unfixed on purpose -- a separate service, separate bd issue territory, not touched here. Regression coverage: TestRDSHandler_FormActions_Clusters/CreateDBCluster_ReplicationSourceIdentifier(_NotFound), .../DescribeDBClusters_ReadReplicaIdentifiers, .../DeleteDBCluster_ReplicaRemovedFromSourceReadReplicaIdentifiers, .../DeleteDBCluster_SourceDeletionOrphansReplica (form_actions_cluster_test.go) -- all four confirmed to fail against the pre-fix source. Prior OPEN entry, kept verbatim for history: 'OPEN 2026-09-07 (gopherstack-z1sd triage): DBCluster has no ReplicationSourceIdentifier or ReadReplicaIdentifiers field at all (real SDK: aws-sdk-go-v2/service/rds@v1.124.1 types/types.go:1123 DBCluster.ReplicationSourceIdentifier *string \"The identifier of the source DB cluster if this DB cluster is a read replica\"; types.go:1107 DBCluster.ReadReplicaIdentifiers []string [corrected from the triage note's :1103 -- re-verified this pass]), and CreateDBClusterInput never parses the real ReplicationSourceIdentifier form field (api_op_CreateDBCluster.go:812) -- grepped handler_db_clusters.go's handleCreateDBCluster, no such vals.Get call exists. So an Aurora cluster that is itself a cross-region/binlog read replica of another Aurora cluster (the CreateDBCluster ReplicationSourceIdentifier path) is entirely unmodeled at the cluster level -- only instance-to-instance replication is (see the read_replicas: family note below, and CreateDBInstanceReadReplica/PromoteReadReplica). DBInstance is also missing the cluster-crossing fields ReadReplicaSourceDBClusterIdentifier/ReadReplicaDBClusterIdentifiers (types.go:2308/2298, needed when a DB instance's replication source or target is a cluster rather than another instance). This is the identical gap already disclosed for the docdb service (services/docdb/PARITY.md gaps: \"ReadReplicaIdentifiers is declared on the DBCluster model ... but CreateDBCluster has no ReplicationSourceIdentifier/create-as-replica code path at all ... dead scaffolding for an unbuilt feature\") -- rds has the identical situation but had not previously disclosed it. Fix is local to this service and has a working precedent to mirror: db_instances.go's CreateDBInstanceReadReplica already threads ReplicaSourceDBInstanceIdentifier/ReadReplicaIdentifiers bidirectionally between two DBInstance records; the same pattern (add the fields, parse ReplicationSourceIdentifier in handleCreateDBCluster, link source<->replica DBCluster records) would close this at the cluster level. Not attempted this pass (triage only, no .go writes).'"
  - "NEW since v1.123.0 (found by gopherstack-u8my's pin-correction pass, not fixed): DBInstance/DBInstanceAutomatedBackup gained StorageOperationPercentProgress/StorageOperationStatus (Initializing/Optimizing progress reporting for an in-progress storage scaling op). Not modeled -- but the real fields only appear at all while a storage operation is actively in progress, and this backend applies storage modifications synchronously (no async storage-scaling state machine exists), so there is never a real in-progress state to report; same structural category as other transient-progress fields this file already treats as correctly omittable rather than a stub. (needs bd issue if a future pass wants a cosmetic 'briefly show Optimizing' simulation)"
  - GetPerformanceInsightsMetrics does not correspond to a real operation name/shape on
    either the RDS SDK client or the Performance Insights ("pi") SDK client (real op:
    GetResourceMetrics, different client, different endpoint/protocol). Kept wired since
    it is real, seeded (SetPerformanceInsightsData), non-stub functionality with no
    accurate replacement to redirect callers to, but it will never be reachable by a
    genuine AWS SDK client under either service and sdkcheck (gopherstack-vhw2) correctly
    flags it as a phantom. See performance_insights family note. (parity-5/phantom-triage,
    2026-07-31)
  - DescribeDBEngineVersions/DescribeOrderableDBInstanceOptions do not implement
    MaxRecords/Marker pagination (they return every matching row in one response). This
    was already true before this pass; noted now because the fabricated
    DescribeCustomDBEngineVersions action (removed this pass, see overall: header) DID
    paginate via paginateDescribe, and its removal drops that pagination behavior for the
    custom-engine-version subset with no replacement — a real (if pre-existing and
    unrelated-to-phantoms) gap worth a follow-up if a real client's engine-version catalog
    ever grows large enough to matter. (parity-5/phantom-triage, 2026-07-31)
  - DescribeServerlessV2PlatformVersions (new this pass, 2026-07-25) always returns an
    empty ServerlessV2PlatformVersions list. The installed SDK module documents no
    enumerable list of real platform version numbers/descriptions to derive from
    (ServerlessV2PlatformVersion is a plain *string on the wire, unlike e.g. the Engine
    field which does have a documented closed set of valid values, which IS validated).
    Inventing specific version strings would fabricate data with nothing in this SDK
    module to verify them against. See the ops: entry for full reasoning; re-review if a
    future SDK/API model version publishes an authoritative version list.
  # All three gaps carried in the 2026-07-23 A- audit were closed for real this pass
  # (2026-07-24), with regression tests, not just re-labeled deferrals -- see the
  # overall: header and Notes for full detail on each:
  #   - DB instance/cluster/snapshot/parameter-group identifiers are now case-insensitive
  #     (pkgs/strs + normalizeID at every store boundary for the six identifier families).
  #   - CreateDBInstance/CreateDBCluster now validate Engine against the real SDK's
  #     documented "Valid Values" lists (validateDBInstanceEngine/validateDBClusterEngine).
  #   - DBShardGroup/Integration field coverage is complete: DBShardGroupArn/
  #     DBShardGroupResourceId/PubliclyAccessible now wired on all four DBShardGroup
  #     mutating ops; Integration gained KMSKeyId/CreateTime/Tags/Errors on Create/Delete/
  #     Modify.
  # Historical record of two already-fixed (prior-pass) items, kept for context:
  # - [FIXED, prior pass] CreateDBShardGroup/DeleteDBShardGroup/ModifyDBShardGroup/RebootDBShardGroup and CreateIntegration/DeleteIntegration/ModifyIntegration and CreateCustomDBEngineVersion/DeleteCustomDBEngineVersion/ModifyCustomDBEngineVersion (10 ops total) previously wrapped their response fields one XML level too deep (e.g. `<CreateDBShardGroupResult><DBShardGroup><DBShardGroupIdentifier>...`) when the real aws-sdk-go-v2 output for all 10 is a FLAT shape with no such wrapper (`<CreateDBShardGroupResult><DBShardGroupIdentifier>...`) — see Notes. A real aws-sdk-go-v2 client's query-XML deserializer only looks for named fields as direct children of the `<XxxResult>` element, so every field on these 10 ops (including the identifier needed to address the resource in a follow-up call) previously came back empty/zero to a real SDK client, even though the emulator's backend state was correct.
  # - [FIXED, prior pass] CreateCustomDBEngineVersion/ModifyCustomDBEngineVersion additionally serialized the description field under the wrong element name (`DatabaseInstallationFilesS3BucketName` instead of `DBEngineVersionDescription`) — see Notes.
deferred: []
leaks: {status: fixed, note: "FOUND and FIXED this pass: DeleteDBCluster (DeleteDBClusterWithOptions in db_clusters.go) removed the cluster itself but did NOT cascade-delete its custom DB cluster endpoints or their tags — DescribeDBClusterEndpoints kept returning ghost rows pointing at a deleted cluster forever, and b.clusterEndpoints only ever shrank via an explicit DeleteDBClusterEndpoint call, so the map grew unboundedly across create/delete cycles in any long-running client (exactly the 'no ghost map rows after delete — cascade-clean instances/endpoints on cluster delete' invariant this audit was scoped to check). Fixed by adding deleteClusterEndpointsLocked (db_clusters.go), called from DeleteDBClusterWithOptions under the existing b.mu write lock, alongside the pre-existing tags/fisFailoverFaults/clusterRoles cleanup. Regression tests: TestDeleteDBCluster_CascadeDeletesClusterEndpoints (cluster_endpoints_test.go, verifies via DescribeDBClusterEndpoints) and a new cluster_endpoint_cascade_via_cluster_delete case added to the existing TestRDSBackend_TagsCleanedUpOnDelete table (tags_test.go). Separately re-verified this pass and still clean: the single reconciler goroutine (lifecycle.go:scheduleReconcilerLocked) is per-backend, started lazily, and exits its own loop once both instanceReadyAt and clusterReadyAt are empty (ticker.Stop() deferred); the two FIS fault-injection goroutines in fault_injection.go/handler_db_clusters.go are ctx-bound (one blocks on ctx.Done(), the other races a time.Timer against ctx.Done(), both Stop()/cleanup correctly). No time.Sleep/context.Background()-rooted unbounded goroutine patterns found in non-test files."}

## Notes

- **2026-08-13 pass (gopherstack-afi1): RestoreDBInstanceFromS3/RestoreDBClusterFromS3
  dropped 3 of 7 required members each.** From the "five ops drop the fields that define
  what they do" required-member sweep. Both handlers read `vals.Get(...)` for only 4 of
  each op's 7 required members -- `S3IngestionRoleArn`/`SourceEngine`/
  `SourceEngineVersion` (identical field set on both ops) were never read at all, and
  `Engine`/`DBInstanceClass`/`MasterUsername` (already read) were not validated as
  required despite being so on the wire. Confirmed against the pinned
  `aws-sdk-go-v2/service/rds@v1.124.1`'s query-protocol serializer
  (`awsAwsquery_serializeOpDocumentRestoreDBInstanceFromS3Input`/
  `...RestoreDBClusterFromS3Input` in `serializers.go`) for the exact case-sensitive
  query-parameter names (`url.Values.Get` is case-sensitive exact-string for this
  protocol) -- all seven per op match the Go SDK field names verbatim, no casing
  surprises. Both real response shapes (`types.DBInstance`/`types.DBCluster`) have no
  members for the three S3-ingestion-source fields, so they're validated present but not
  persisted, matching this file's existing "no real state to echo into" convention (see
  e.g. the `ApplyPendingMaintenanceAction`/`instance_iam_roles` families above). Added
  `TestRestoreDBInstanceFromS3_RealSDKClient`/`TestRestoreDBClusterFromS3_RealSDKClient`
  (`restore_from_s3_sdk_test.go`) driving the real `aws-sdk-go-v2/service/rds` client end
  to end -- the existing backend-level table tests only ever exercised
  `b.RestoreDB*FromS3` directly with hand-typed Go strings, which would pass identically
  whether or not the handler's `vals.Get` keys actually matched the SDK's serialized
  query-parameter names; the SDK-driven tests are what actually prove the wire names are
  right. Both pre-existing table tests (`TestRestoreDBInstanceFromS3`/
  `TestRestoreDBClusterFromS3`) previously didn't supply the three dropped fields at all
  (nothing to, since the handler-level bug is specifically about the HTTP decode layer,
  not the backend signature) -- extended to cover all seven required members individually
  once the backend signature grew the three new required parameters.

- Protocol: RDS uses the AWS Query (XML) protocol, version `2014-10-31`, XML namespace
  `http://rds.amazonaws.com/doc/2014-10-31/`. Every response wraps in
  `<ActionResponse><ActionResult>...</ActionResult></ActionResponse>` except where the op's
  SDK output has no members (in which case an empty result element is still correct — do not
  flag as a stub).

- **2026-07-25 pass summary.** The Go SDK modules were bumped (aws-sdk-go-v2/service/rds
  v1.116.2 -> v1.123.0), and `TestSDKCompleteness` flagged exactly one new operation:
  `DescribeServerlessV2PlatformVersions`. Implemented for real (not added to a
  `notImplemented` list): routing wired into `dispatchExtended16`
  (`handler_dispatch.go`), `GetSupportedOperations` updated (`handler_supported_ops.go`),
  request parsing + XML response building added to `handler_reference_data.go` (new
  `describeServerlessV2PlatformVersionsResponse`/`xmlServerlessV2PlatformVersionInfo`/
  `xmlServerlessV2FeaturesSupport`/`xmlServerlessV2VersionList` types, paired with the
  new `ServerlessV2PlatformVersionInfo`/`ServerlessV2FeaturesSupport` Go types in
  `models.go`), and backend logic added to `reference_data.go`
  (`(*InMemoryBackend).DescribeServerlessV2PlatformVersions`,
  `validServerlessV2Engines`, `staticServerlessV2PlatformVersions`). Confirmed every
  request/response member name and the two valid `Engine` values directly against
  `aws-sdk-go-v2/service/rds@v1.123.0`'s `api_op_DescribeServerlessV2PlatformVersions.go`
  and `go doc .../rds/types ServerlessV2PlatformVersionInfo` rather than inferring from
  the operation name. No new files: both files already existed and already housed the
  closest sibling ops (`DescribeDBMajorEngineVersions`, `DescribeSourceRegions`). See the
  `DescribeServerlessV2PlatformVersions` `ops:` entry and the matching `gaps:` entry above
  for why the served catalog is honestly empty rather than populated with invented
  version numbers, and `reference_data_test.go`'s `TestDescribeServerlessV2PlatformVersions`
  / `TestHandler_DescribeServerlessV2PlatformVersions` table tests for coverage (the
  latter asserts the wire shape through the real router path: form-encoded request ->
  `Handler.Handler()` -> XML response, using the existing `doAccuracyRDS`/
  `newAccuracyRDSHandler` harness). `overall` stays `A`: the operation is genuinely
  routed, validated, paginated, and persistence-transparent (no new mutable state was
  added, so nothing new needed wiring into `Snapshot`/`Restore`) -- the empty catalog is
  an honest reflection of what the installed SDK actually documents, not a shortcut.

- **2026-07-23 pass summary.** This pass targeted the items the prior ledger flagged as gaps
  (Filters coverage) and deferred (Activity Streams), plus a fresh leak/error-code audit per
  the campaign's standing invariants. Six independent, verified fixes:

  1. **Ghost-row leak (found + fixed).** `DeleteDBClusterWithOptions` (`db_clusters.go`) deleted
     the cluster but left its custom `DBClusterEndpoint`s (and their tags) behind forever —
     `DescribeDBClusterEndpoints` kept returning rows for a deleted cluster, and `b.clusterEndpoints`
     only ever shrank via an explicit `DeleteDBClusterEndpoint` call. Fixed with a new
     `deleteClusterEndpointsLocked` helper invoked from the existing delete path. See the
     top-level `leaks:` entry for the full writeup and test names.

  2. **`Filters` support added to `DescribeDBClusters`, `DescribeDBSnapshots`,
     `DescribeDBClusterSnapshots`.** Mirrors the `DescribeDBInstances` Filters pattern added in a
     prior pass (`parseDescribeFilters` + an `isKnownXFilterName`/`applyXFilters`/
     `matchesAllXFilters` trio per op, each field-diffed against the real SDK's documented
     `Supported Filters` list for that op). Unmodeled-but-real filter names (`clone-group-id`,
     `domain`) are accepted and vacuously match everything, mirroring the existing `domain`
     precedent on `DescribeDBInstances`. **Correction to the prior ledger:** `DescribeEvents`
     Filters was listed as a gap, but `DescribeEventsInput.Filters`'s doc comment in
     aws-sdk-go-v2 literally reads "This parameter isn't currently supported" — real AWS itself
     doesn't implement it, so the emulator's existing accept-and-ignore behavior there was
     already correct; this is not a gap and has been removed from the gaps list.

  3. **Missing pagination added to `DescribeDBClusterSnapshots` and `DescribeEvents`.** Both
     returned every row unconditionally regardless of `MaxRecords`/`Marker`, even though both
     real outputs (`DescribeDBClusterSnapshotsOutput`, `DescribeEventsOutput`) carry a `Marker`
     field. Wired through the existing `paginateDescribe` helper for consistency with every other
     Describe op. `DescribeEvents` sorts by `(CreatedAt, SourceIdentifier)` for a stable order.

  4. **Missing wire fields added**, confirmed against `aws-sdk-go-v2/service/rds@v1.116.2`'s
     `types.DBCluster`/`types.DBClusterSnapshot`/`types.DBSnapshot`: `DBCluster.DbClusterResourceId`,
     `DBClusterSnapshot.DbClusterResourceId` + `SnapshotType` (the latter was previously never set
     at all on cluster snapshots, unlike instance snapshots which already distinguished
     manual/automated), and `DBSnapshot.DbiResourceId`. `CopyDBClusterSnapshot`/`CopyDBSnapshot`
     were also missing several fields their real outputs carry (`EngineVersion`,
     `PercentProgress`, `StorageEncrypted` on cluster-snapshot copy; `SnapshotType` on
     snapshot copy) — filled in alongside the new fields since the same struct literals needed
     touching anyway.

  5. **Activity Streams de-deferred** (`activity_stream.go`, `handler_activity_stream.go`).
     `StartActivityStream`/`StopActivityStream` already matched the real flat
     `StartActivityStreamOutput`/`StopActivityStreamOutput` shapes. `ModifyActivityStream` had a
     disguised-stub bug: it serialized an invented `<AuditPolicy>` XML element that does not
     exist anywhere on the real `ModifyActivityStreamOutput` (verified against
     `aws-sdk-go-v2/service/rds@v1.116.2/api_op_ModifyActivityStream.go`) — the real field for
     the policy lock state is `PolicyStatus`. A real SDK client parsing this response would never
     see the value the emulator was trying to communicate, since it was under the wrong XML tag
     entirely. Fixed by renaming the field to `PolicyStatus` and adding the real
     `KinesisStreamName`/`Mode` members that were also missing. Separately, all three ops
     returned `InvalidParameterValue` for a nonexistent cluster instead of the correct
     `DBClusterNotFoundFault` — fixed to use `ErrClusterNotFound`. This family had zero test
     coverage before this pass; added `activity_stream_test.go`.

  6. **Systemic error-code "Fault"-suffix bug (found + fixed) in `rdsErrorCode()`**
     (`handler_dispatch.go`). AWS is inconsistent about whether a wire error code carries a
     trailing "Fault" (`DBInstanceNotFound` has none; `DBClusterNotFoundFault` does), so each
     entry below was individually confirmed against `aws-sdk-go-v2/service/rds@v1.116.2/types/errors.go`'s
     generated `ErrorCode()` methods — the authoritative source for what a real RDS server puts
     on the wire — rather than assumed from a uniform convention. Fifteen codes were missing the
     suffix real AWS uses: `DBClusterNotFoundFault`, `DBClusterAlreadyExistsFault`,
     `DBClusterSnapshotNotFoundFault`, `DBClusterSnapshotAlreadyExistsFault`,
     `DBClusterEndpointNotFoundFault`, `DBClusterEndpointAlreadyExistsFault`,
     `DBClusterAutomatedBackupNotFoundFault`, `GlobalClusterNotFoundFault`,
     `GlobalClusterAlreadyExistsFault`, `BlueGreenDeploymentNotFoundFault`,
     `BlueGreenDeploymentAlreadyExistsFault`, `IntegrationNotFoundFault`,
     `IntegrationAlreadyExistsFault`, `OptionGroupNotFoundFault`, `OptionGroupAlreadyExistsFault`.
     Separately and more severely: `ErrDBProxyAlreadyExists`, `ErrDBProxyEndpointAlreadyExists`,
     `ErrCannotDeleteDefaultProxyEndpoint`, `ErrActivityStreamAlreadyStarted`, and
     `ErrActivityStreamNotStarted` had **no entry at all** in the mapping table — since each
     `awserr.New(...)` sentinel is a distinct `*wrappedError` pointer even when two sentinels
     share the same message string, `errors.Is(opErr, m.sentinel)` never matched any mapping
     entry for these five, so `rdsErrorCode()` returned `""` and `handleOpError` fell through to
     a **500 InternalFailure** instead of the correct 400 client error — exactly the
     "missing errCodeLookup entries → not-found errors surfacing as 500 InternalFailure" bug
     class from `.claude/memories/parity-principles.md` #2, just for conflict/already-exists
     faults instead of not-found ones. Regression test: `TestRDSErrorCodes_FaultSuffix`
     (`error_codes_test.go`), 15 table cases covering every fixed family end-to-end through the
     HTTP handler. The four newly-extracted string constants this fix needed to stay
     `goconst`-clean (`filterNameDBClusterID`, `filterNameDBInstanceID`, `filterNameEngine`,
     `filterNameDomain`, `filterNameDbiResourceID`, `filterNameSnapshotType`,
     `snapshotTypeManual`, `errCodeInvalidDBClusterStateFault`) live in `shared.go`.

- **DeleteDBInstance / DeleteDBCluster final-snapshot contract (fixed this pass).** Real AWS
  requires exactly one of `SkipFinalSnapshot=true` or a non-empty `FinalDBSnapshotIdentifier`
  (`FinalDBClusterSnapshotIdentifier` for clusters); supplying both is
  `InvalidParameterCombination`, as is supplying neither. Before this pass the emulator's
  `DeleteDBInstance`/`DeleteDBCluster` took only an identifier and silently behaved as if
  `SkipFinalSnapshot=true` always — no validation, and no final snapshot was ever created even
  when a client explicitly asked for one. This is exactly the "disguised stub" bug class from
  `.claude/memories/parity-principles.md` #4: the delete itself was real (removed real state),
  but a whole documented, commonly-exercised parameter contract was silently ignored.
  Fixed by adding `DeleteDBInstanceWithOptions`/`DeleteDBClusterWithOptions` (additive — the
  existing `DeleteDBInstance(id)`/`DeleteDBCluster(id)` single-arg methods are kept unchanged
  and now delegate with `skipFinalSnapshot=true` since they are called by
  `services/cloudformation/resources_phase2.go` and `resources_phase4.go`, which are outside
  this audit's edit scope). AWS resolves the target resource before validating the snapshot
  parameter combination — a delete against a nonexistent instance/cluster returns
  `DBInstanceNotFound`/`DBClusterNotFound` even when `SkipFinalSnapshot`/
  `FinalDBSnapshotIdentifier` are also invalid or missing; order the checks accordingly (existence
  first) or existing/incoming client integration tests that intentionally omit both params
  against a missing resource will regress to the wrong error code.

- **DescribeDBInstances Filters (fixed this pass, partial).** AWS's DescribeDBInstances
  accepts `Filters.Filter.N.Name` / `Filters.Filter.N.Values.member.M` with recognized names
  `db-cluster-id`, `db-instance-id`, `dbi-resource-id`, `domain`, `engine`. Multiple values
  within one filter OR together; multiple filters AND together. An unrecognized filter name
  is `InvalidParameterValue`. `domain` is accepted (to avoid rejecting otherwise-valid client
  requests) but is not modeled as a real predicate since this emulator has no Directory
  Service domain-membership state — every instance vacuously "matches" a domain filter. The
  same Filters shape is documented on `DescribeDBClusters`, `DescribeDBSnapshots`,
  `DescribeDBClusterSnapshots`, and `DescribeEvents` but was out of scope to also implement
  this pass; see gaps.

- **`newManualSnapshotLocked` / `newManualClusterSnapshotLocked`.** Both `CreateDBSnapshot`/
  `CreateDBClusterSnapshot` and the new delete-time final-snapshot path build the same
  `DBSnapshot`/`DBClusterSnapshot` shape, so the struct-building logic was extracted into a
  `*Locked` helper (caller must already hold `b.mu`) shared by both call sites — avoids
  duplicating the AWS shape twice and risking the two copies drifting apart.

- **Case-sensitive identifiers (FIXED 2026-07-24, was a documented gap through three prior
  audits).** Every resource map in `backend.go` (`instances`, `clusters`, `snapshots`,
  `parameterGroups`, ...) used to be keyed by the identifier string exactly as given. Real
  RDS identifiers are case-insensitive persistent handles (AWS lower-cases them internally),
  so `CreateDBInstance("MyDB", ...)` followed by `CreateDBInstance("mydb", ...)` collides
  with `DBInstanceAlreadyExistsFault` in real AWS. Three prior audits deferred this as
  "normalizing touches every resource map across ~30K LOC, too invasive for a scoped pass" —
  that deferral is retracted; scope/invasiveness is not a valid reason to leave a confirmed
  wire-behavior gap open. Fixed by adding `github.com/blackbirdworks/gopherstack/pkgs/strs`
  (a new, non-RDS-specific shared package: `Fold`/`Equal`/`ContainsFold`, case-fold string
  helpers any service emulating a case-insensitive AWS identifier can reuse instead of
  hand-rolling `strings.ToLower`/`EqualFold`) and normalizing every store boundary for the six
  identifier families real AWS folds: `DBInstanceIdentifier`, `DBClusterIdentifier`,
  `DBSnapshotIdentifier`, `DBClusterSnapshotIdentifier`, `DBParameterGroupName`,
  `DBClusterParameterGroupName` (the last shared by both the `parameterGroups` and
  `clusterParameterGroups` tables, which both store `*DBParameterGroup`).

  The mechanism: `pkgs/store`'s `Table[V].Get/Has/Delete` index the map directly and do
  **not** invoke the table's `keyFn` on the lookup argument (only `Put`/`Restore` do) — see
  `pkgs/store`'s package doc. So normalizing only each table's `keyFn` (e.g.
  `instancesKeyFn`) would have folded case on `Put` but left every `Get`/`Has`/`Delete` call
  site still comparing raw, unfolded strings — a half-fix that looks done but isn't. Every one
  of the 116 non-test call sites across `db_instances.go`/`db_clusters.go`/
  `db_snapshots.go`/`cluster_snapshots.go`/`parameter_groups.go`/
  `cluster_parameter_groups.go`/`roles.go`/`maintenance.go`/`log_files.go`/`lifecycle.go`/
  `activity_stream.go`/`automated_backups.go`/`cluster_endpoints.go` was updated to fold its
  argument through `normalizeID` (a local `services/rds` wrapper around `strs.Fold`) before
  calling `Get`/`Has`/`Delete`.

  Three secondary correctness issues found and fixed along the way, all necessary to avoid
  the exact "half-normalized service...worse than the current consistent-but-wrong behavior"
  trap the original gap write-up warned about:
    1. The `snapshotAttributes`/`clusterSnapshotAttributes` satellite tables are keyed by the
       same case-insensitive `DBSnapshotIdentifier`/`DBClusterSnapshotIdentifier` as the
       primary snapshot tables; left unnormalized, `ModifyDBSnapshotAttribute("MySnap", ...)`
       followed by `DescribeDBSnapshotAttributes("mysnap")` would have wrongly reported no
       attributes even though the snapshot lookup itself now succeeds case-insensitively.
       Fixed by normalizing these two tables' keyFns too.
    2. `Describe{DBInstances,DBClusters,DBSnapshots,DBClusterSnapshots}` and
       `DescribeDBClusterEndpoints`'s `Filters`/secondary-ID matching used
       `slices.Contains`/`==` (case-sensitive) against identifier-shaped filter values; added
       `containsFold`/`idEqual` (services/rds wrappers around the new `strs` helpers) for the
       identifier-family filters specifically (not `engine`, `snapshot-type`, or the
       AWS-generated resource-ID filters like `dbi-resource-id`, which are not part of this
       gap).
    3. Auxiliary plain (non-`Table`) maps keyed off these identifiers — the ARN used to key
       `b.tags`, `instanceRoles`/`clusterRoles`, `instanceReadyAt`/`clusterReadyAt`,
       `automatedBackups` — are not normalized themselves, so a delete/role/reboot call issued
       under different casing than the resource's create call would have found the resource
       (now case-insensitive) but then written/deleted the wrong (or a fresh, ghost) entry in
       one of these maps. Fixed by keying these off the just-fetched resource's *canonical*
       (as-created) identifier field instead of the raw caller-supplied argument, at every
       call site that fetches the resource first (`DeleteDBInstance`, `DeleteDBCluster`,
       `ModifyDBInstance`, `RebootDBInstance`, `RebootDBCluster`, `FailoverDBCluster`,
       `PromoteReadReplica`, `AddRoleToDBInstance`/`RemoveRoleFromDBInstance`,
       `AddRoleToDBCluster`/`RemoveRoleFromDBCluster`, `StartActivityStream`).

  Regression tests (table cases covering: same-case duplicate still collides;
  lower/upper/mixed-case duplicate collides; a genuinely distinct id does not collide;
  Describe finds the resource under a different case AND still echoes back the *original*
  creation-time casing on the wire; Delete works under a different case and the resource is
  actually gone afterward): `TestCreateDBInstance_CaseInsensitiveIdentifier`
  (`db_instances_operations_test.go`), `TestCreateDBCluster_CaseInsensitiveIdentifier`
  (`db_clusters_operations_test.go`), `TestCreateDBSnapshot_CaseInsensitiveIdentifier`
  (`db_snapshots_test.go`), `TestClusterSnapshot_Duplicate` (extended into a table test,
  `cluster_snapshots_test.go`), `TestDBParameterGroup_Duplicate` (extended into a table test,
  `parameter_groups_test.go`), `TestClusterPG_CaseInsensitiveIdentifier`
  (`cluster_parameter_groups_test.go`).

- **Engine not validated (FIXED 2026-07-24, was a documented gap through prior audits).**
  `CreateDBInstance`/`CreateDBCluster` used to accept any string as `Engine`; real AWS returns
  `InvalidParameterValue` for a value outside its documented list. Fixed with
  `validateDBInstanceEngine`/`validateDBClusterEngine` (`engine_versions.go`) checked against
  `validDBInstanceEngines`/`validDBClusterEngines`, field-diffed from
  `aws-sdk-go-v2/service/rds@v1.116.2`'s `CreateDBInstanceInput`/`CreateDBClusterInput`
  `Engine` field doc comments (the ground truth here, since `Engine` is a plain `*string` on
  the wire with no SDK-side enum type to lean on) — 24 values for instances (including the
  less-common RDS Custom/Db2/SQL Server engines this emulator's `DescribeDBEngineVersions`
  catalog doesn't otherwise seed), 5 for clusters (`aurora-mysql`, `aurora-postgresql`,
  `mysql`, `postgres`, `neptune` — clusters are narrower since they're Aurora/Multi-AZ/Neptune
  only, so e.g. `oracle-ee` is valid for `CreateDBInstance` but must be rejected for
  `CreateDBCluster`). An empty `Engine` is still accepted and defaulted (unchanged prior
  behavior — `CreateDBInstance` defaults to `postgres`, `CreateDBCluster` to
  `aurora-postgresql`, both always valid), matching real AWS's *documented* values rather than
  its `required`-field strictness, since existing callers throughout this codebase rely on
  the default.

  Fixing this surfaced a real, previously-hidden bug: `performance_insights_test.go` (two
  call sites) and `dispatch_test.go` called `CreateDBInstance` with two positional arguments
  swapped (`Engine="db.t3.micro"`, an instance class, not an engine), which only ever
  "worked" because nothing validated `Engine` — a live demonstration of why this was a real
  gap, not a cosmetic one. Fixed those three call sites (corrected argument order) plus
  `persistence_test.go`, which used the legacy engine string `"aurora"` (not in the current
  SDK's documented `CreateDBClusterInput.Engine` values) instead of `"aurora-mysql"`.
  Regression tests: `TestCreateDBInstance_EngineValidation`
  (`db_instances_operations_test.go`), `TestCreateDBCluster_EngineValidation`
  (`db_clusters_operations_test.go`), both table-driven over valid values (including the
  less-common ones) and invalid ones (a made-up string; an instance-only engine passed to
  `CreateDBCluster`; an engine-class-confusion regression guard for the exact bug class found
  in the three fixed tests).

- **DBShardGroup/Integration field coverage (FIXED 2026-07-24, was a documented gap through
  prior audits).** `DBShardGroup` didn't model `DBShardGroupArn`/`DBShardGroupResourceId` at
  all, and modeled `PubliclyAccessible` on the Go struct but never serialized it onto any
  response. `Integration` didn't model `Tags`/`Errors` at all, and modeled `KmsKeyID`/
  `CreatedAt` on the Go struct but never serialized them. Field-diffed against
  `aws-sdk-go-v2/service/rds@v1.116.2`'s `types.DBShardGroup`/`types.Integration` and each
  op's output struct:
    - `DBShardGroup`: field-diffing `CreateDBShardGroupOutput` surfaced that
      `Delete`/`Modify`/`RebootDBShardGroupOutput` carry the exact same full flat field set
      (`ComputeRedundancy`, `DBClusterIdentifier`, `DBShardGroupArn`, `DBShardGroupIdentifier`,
      `DBShardGroupResourceId`, `Endpoint`, `MaxACU`, `MinACU`, `PubliclyAccessible`, `Status`)
      — not just Create — so all four ops' XML response structs
      (`handler_shard_groups.go`) were brought up to the full set, not just the three fields
      the prior gap write-up named. `TagList` also exists on all four real outputs but was
      left out of scope (not named in the original gap, and — unlike `Integration`'s `Tags`,
      see below — `CreateDBShardGroupInput` accepting inline `Tags` would need new
      request-parsing work this pass didn't extend to).
    - `Integration`: added `KMSKeyId`/`CreateTime`/`Tags`/`Errors` to `Create`/`Delete`/
      `ModifyIntegrationOutput` (verified all three carry the same full field set as
      `types.Integration` itself, the same pattern as DBShardGroup above). `Tags` is backed
      by the SAME shared per-ARN `b.tags` map every other RDS resource in this emulator uses
      (via `ListTagsForResource`), not a new inline field — consistent with the fact that no
      `Create*` handler in this service parses `Tags.Tag.N` at creation time (a pre-existing,
      systemic design choice across the whole service, not an `Integration`-specific gap, so
      not changed here). Adding `Tags` to the wire made `DeleteIntegration`'s missing
      `b.tags` cleanup a live leak (ghost tag-map entries keyed by a deleted integration's
      ARN) where before it was inert; fixed by cascade-deleting `b.tags[intg.IntegrationArn]`
      in `DeleteIntegration` (`integrations.go`) alongside the existing integration-row
      delete. `IntegrationError`/`ErrorCode`/`ErrorMessage` types added to `models.go` to back
      the new `Errors` field (populated as empty by default — this emulator has no failure
      simulation for integrations, so `Errors` is always `[]` today, but the field is now
      genuinely present and correctly shaped on the wire for any future caller/test that sets
      it).

  Regression tests assert against the raw XML response body (not just the backend struct),
  since this exact bug class — a correctly-populated Go value that never reaches the wire
  because the XML struct doesn't carry the field — is a "disguised stub" that backend-only
  assertions would miss entirely: `TestDBShardGroup_WireFieldsPresentOnAllOps`
  (`shard_groups_test.go`, covers Create/Modify/Reboot/Delete in sequence against one
  resource) and `TestIntegration_WireFieldsPresentOnAllOps` (`integrations_test.go`, covers
  Create/Modify/Delete, plus tags added via the standard `AddTagsToResource` flow to prove the
  `Tags` wiring end-to-end).

- The stray `batch3_test.go.rej` / `batch3_test_pi.patch` files (tracked in git, dated the day
  before this audit) were leftover artifacts of a previously-applied patch — diffed against
  `batch3_test.go` and confirmed the patch's content (deterministic-vs-flaky
  `TestPerformanceInsights_*` fixes) was already live in the tracked test file. Removed as
  dead weight; not a behavior change.

- **DBShardGroup / Integration / CustomDBEngineVersion single-object response nesting bug
  (fixed this pass).** `services/rds/handler_completeness.go` modeled the responses for
  `CreateDBShardGroup`, `DeleteDBShardGroup`, `ModifyDBShardGroup`, `RebootDBShardGroup`,
  `CreateIntegration`, `DeleteIntegration`, `ModifyIntegration`, `CreateCustomDBEngineVersion`,
  `DeleteCustomDBEngineVersion`, and `ModifyCustomDBEngineVersion` the same way as the
  well-established `CreateDBInstance`/`CreateDBCluster`/etc. pattern: a scalar wrapper struct
  nested one level under the result, e.g.
  `xml:"CreateDBShardGroupResult>DBShardGroup"`. That pattern is correct for DBInstance/
  DBCluster/DBSnapshot/etc. because `CreateDBInstanceOutput` genuinely nests its payload under
  a `DBInstance *types.DBInstanceType` field. But `CreateDBShardGroupOutput`,
  `CreateIntegrationOutput`, and `CreateCustomDBEngineVersionOutput` (verified directly against
  `aws-sdk-go-v2/service/rds@v1.116.2`'s `api_op_*.go` output structs and `deserializers.go`)
  are all **flat** — `ComputeRedundancy`, `DBShardGroupIdentifier`, `IntegrationName`, `Engine`,
  etc. sit directly on the output struct, not inside a nested sub-object. Confirmed against
  `awsAwsquery_deserializeOpDocumentCreateDBShardGroupOutput` in `deserializers.go`: the
  generated deserializer's `switch` only matches field names as *direct children* of the
  `<CreateDBShardGroupResult>` node; an unrecognized child element name (like a stray
  `<DBShardGroup>` wrapper) falls through unmatched and its entire subtree — including the real
  field values one level deeper — is silently skipped. A real aws-sdk-go-v2 client calling any
  of these 10 ops against the old code would therefore get back a `*XxxOutput` with every field
  zero-valued, including the identifier fields (`DBShardGroupIdentifier`, `IntegrationName`,
  `Engine`/`EngineVersion`) client code typically needs from a Create response to address the
  resource in a follow-up call — a silent, high-impact wire break despite the backend state
  being entirely real (this is the "disguised stub" bug class from
  `.claude/memories/parity-principles.md` #2/#4: a real-looking nested-struct response that is
  wrong purely in its XML chain depth). `DescribeDBShardGroups`/`DescribeIntegrations` were
  NOT affected — those really do return a list (`DBShardGroups []types.DBShardGroup` /
  `Integrations []types.Integration`), so the existing `xmlDBShardGroupList`/
  `xmlIntegrationList` nesting is correct and was left unchanged; `toXMLDBShardGroup`/
  `toXMLIntegration` helpers are still used for that list path. `TenantDatabase` and
  `DBSecurityGroup` responses were checked against the same risk and found NOT to have this bug
  — their real SDK outputs (`CreateTenantDatabaseOutput.TenantDatabase *types.TenantDatabase`,
  `CreateDBSecurityGroupOutput.DBSecurityGroup *types.DBSecurityGroup`) genuinely do nest, so
  gopherstack's existing nested-wrapper responses for those were already correct.
  Fix: the 10 broken response structs now carry each field with the full
  `xml:"CreateDBShardGroupResult>FieldName"` chain individually (same technique already used
  a few lines below for `ModifyCurrentDBClusterCapacityResult`), so Go's `encoding/xml` emits
  all fields as flat siblings under one `<XxxResult>` element instead of nesting them under an
  extra wrapper element. Also fixed in the same pass: `CreateCustomDBEngineVersion`/
  `ModifyCustomDBEngineVersion` serialized their description field under the wrong element name
  entirely (`DatabaseInstallationFilesS3BucketName`, which isn't even a field on the real
  output) instead of the real `DBEngineVersionDescription` — fixed alongside the nesting change.
  `TestCreateCustomDBEngineVersionCRUD` in `accuracy_test.go` had encoded the old, wrong nested
  shape as its expected XML structure (exactly the "unit tests are not parity proof" trap from
  `.claude/memories/parity-principles.md` #3 — the test was green because it was written against
  the emulator's own bugged output, not against the real SDK shape); updated to assert the flat
  shape instead. Added `TestCreateDBShardGroup_WireShapeIsFlat` and
  `TestCreateIntegration_WireShapeIsFlat` regression tests that unmarshal the HTTP response body
  with the real (flat) field layout to guard against regressing back to the nested shape.

- No goroutine/ticker/map leaks found. The single background reconciler
  (`scheduleReconcilerLocked`) is started lazily per backend on first
  `CreateDBInstance`/`ModifyDBInstance`/etc. and exits its own `for` loop once
  `len(b.instanceReadyAt) == 0 && len(b.clusterReadyAt) == 0`, so it does not run forever in a
  backend that settles into a steady state. `events` is capped at `maxEvents` (ring-buffer
  trim). No `context.Background()`-rooted unbounded goroutines outside `fis.go` (chaos
  fault-injection, out of this pass's scope) were found in non-test `.go` files.

**2026-08-22 (gopherstack-ifzn) -- RouteMatcher swallowed a body-read failure as a 404,
masking Handler()'s already-typed error**: same shape as autoscaling's entry (see that
entry or gopherstack-3a8t for the full survey/rationale). `RouteMatcher` now falls back to
`service.MatchesUserAgentMarker(r.Header, "api/rds")` (verified against the pinned
`rds@v1.124.1/api_client.go:641` `AddSDKAgentKeyValue` call) only on the `ReadBody` failure
branch. Migrated `ExtractOperation`/`ExtractResource`/`Handler()`
(`handler_dispatch.go`) off `r.ParseForm()` onto `httputils.ReadBody`+`url.ParseQuery`, per
the docdb/neptune precedent (gopherstack-bahs). Also retyped `Handler()`'s read-failure
branch from `ValidationException` (400, a caller-fault code) to `InternalFailure` (500,
this service's own existing code for internal errors) -- a body-read failure is not the
caller's fault. Proof: `TestHandler_OversizedBodySurfacesInternalFailure` in
`handler_oversized_body_test.go` drives a real RDS SDK client through
`service.NewRegistry`/`service.NewServiceRouter`, confirmed failing pre-fix with
`UnknownError`; passes now with `InternalFailure`. `TestHandler_NormalSizedBodyStillRoutes`
is the regression guard. Gates: `go build`, `go vet`, `gofmt -l` (clean), `go test -race
./services/rds/...` (pass), `golangci-lint run ./services/rds/...` (0 issues).

**2026-08-29 (wrapper-key sweep, gopherstack-101r family) -- DescribeDBInstances/DescribeDBClusters/
DescribeDBSnapshots/DescribeDBClusterSnapshots Filters silently discarded a real client's filter
values (REQUEST direction)**: the shared `parseDescribeFilters` (`handler_db_instances.go`) parsed
`Filters.Filter.N.Name` correctly but read values from `Filters.Filter.N.Values.member.M`. Confirmed
against `rds@v1.124.1` `serializers.go:11730` `awsAwsquery_serializeDocumentFilterValueList` --
`array := value.Array("Value")` -- the real aws-sdk-go-v2 client always sends
`Filters.Filter.N.Values.Value.M`; `member` never appears on the wire for this shape (same bug class
already fixed in `services/docdb/filters.go`, `6160e4dad`). Every one of the 4 ops sharing this parser
was affected identically; all 4 already implemented exactly the AWS-documented filter names (verified
per-op against each op's own `DescribeXxxInput.Filters` doc comment in `api_op_DescribeXxx.go`) via
`isKnownDBXxxFilterName`/`matchesAllDBXxxFilters`, so no filter-name coverage changed, only the value
parsing key.

Triage of every other rds SDK operation carrying a `Filters []types.Filter` member (43 total,
`api_op_*.go` doc comments read individually): 22 are "This parameter isn't currently supported" per
AWS's own doc comment (correctly left unimplemented, matching real AWS's accept-but-ignore behavior)
and 21 document real supported filter names. Of those 21, only the 4 above have any filter-matching
logic implemented in this backend at all; the other 17 (DescribeBlueGreenDeployments,
DescribeDBClusterAutomatedBackups, DescribeDBClusterBacktracks, DescribeDBClusterEndpoints,
DescribeDBClusterParameters, DescribeDBEngineVersions, DescribeDBInstanceAutomatedBackups,
DescribeDBParameters, DescribeDBRecommendations, DescribeDBShardGroups,
DescribeDBSnapshotTenantDatabases, DescribeEngineDefaultParameters, DescribeExportTasks,
DescribeGlobalClusters, DescribeIntegrations, DescribePendingMaintenanceActions,
DescribeTenantDatabases) silently ignore the `Filters` parameter entirely -- a real, pre-existing gap,
but a "Filters not implemented" feature gap distinct from this pass's "Filters implemented with the
wrong wire key" bug; left alone rather than inventing 17 new filter behaviors under this fix's scope.

Repo-wide sweep for the same idiom (`Values.member` / `Filters.Filter.N` and, more broadly, any
query-protocol filter parser reading an indexed `Values`-shaped array) verified each hit against that
service's own pinned SDK serializer rather than assuming: `elbv2` (`handler_listener_rules.go`),
`elasticbeanstalk` (`handler_platforms.go`), `iam` (`handler.go`, `handler_account.go`),
`autoscaling` (`handler_tags.go`), and `ec2` (`handler_filters.go`, `handler_tags.go`,
`handler_local_gateway.go`) all correctly use their own service's real array element name (`member`
for the AWS-query-protocol services above, confirmed against each one's own
`awsAwsquery_serializeDocument*` array-encoding call; the EC2-query-protocol flat `Filter.N.Value.M`
for ec2, confirmed against `awsEc2query_serializeDocumentFilter`'s `object.FlatKey("Value")`) --
none of these needed a fix. `redshift/handler_advisor.go`'s `nodeConfigFilterValue` scans for any key
prefixed `.Values.` rather than hardcoding a spelling, so it isn't vulnerable to this bug class either
way. `services/neptune/handler.go` already uses the correct `Filters.Filter.N.Values.Value.1` spelling
(off-limits this session -- another agent editing it concurrently -- but nothing to report there for
this bug). `services/kafka/` has no `Filters.Filter`/`Values.member` idiom at all (off-limits, nothing
found). `services/docdb/filters.go` is the reference implementation (off-limits, already correct).

Existing tests asserting the wrong spelling as correct (fixed this pass, all raw-`url.Values` tests
that bypass the real SDK serializer so they'd silently "pass" against either spelling as long as the
handler's own parser matched): `Test_DescribeDBInstances_Filters` (`db_instances_test.go`) and
`TestDescribeDBClusters_Filters`/`TestDescribeDBSnapshots_Filters`/
`TestDescribeDBClusterSnapshots_Filters` (`describe_filters_test.go`) all built
`Filters.Filter.N.Values.member.M` query strings by hand; updated to `Values.Value.M`. Added
`TestDescribeDBInstances_Filters_RealClient` (`wire_field_fixes_rdssweep2_test.go`), which drives
`DescribeDBInstances` through the real `aws-sdk-go-v2` client with an `engine=mysql` filter against
one matching and one excluded instance -- confirmed failing against the unmodified parser (returned
zero instances, not just failing to exclude the postgres one) before the fix, passing after.

Gates: `go build ./services/rds/...`, `go build ./...` (repo-wide, no signature changes but checked
per this session's constraints), `go vet ./services/rds/...`, `go test -race -count=1
./services/rds/...` (pass), `golangci-lint run --fix ./services/rds/...` (0 issues).

- **ERROR path re-verified against `cmd/errcodeaudit`'s near-miss sweep (this session)**:
  the tool flags 18 `errors.go` sentinel literals (`DBSubnetGroupNotFound`,
  `OptionGroupNotFound`, `OptionGroupAlreadyExists`, `DBClusterNotFound`,
  `DBClusterAlreadyExists`, `DBClusterSnapshotNotFound`, `DBClusterSnapshotAlreadyExists`,
  `DBClusterEndpointNotFound`, `DBClusterEndpointAlreadyExists`, `GlobalClusterNotFound`,
  `GlobalClusterAlreadyExists`, `BlueGreenDeploymentNotFound`,
  `BlueGreenDeploymentAlreadyExists`, `IntegrationNotFound`, `IntegrationAlreadyExists`,
  `DBClusterAutomatedBackupNotFound`, `DBProxyAlreadyExists`, `DBProxyEndpointAlreadyExists`)
  as absent from rds's real type/deserializer set. All are **tool false positives** against
  current code: every backend error routes through the single
  `handleOpError`→`rdsErrorCode()` mapping table in handler_dispatch.go, which already
  carries the correct code for each of these 18 sentinels — most were the specific
  missing-`Fault`-suffix bug the mapping table's earlier fix pass found and fixed,
  `DBProxyAlreadyExists`/`DBProxyEndpointAlreadyExists` were the separate missing-table-entry
  bug that same pass fixed — see this file's earlier `error_codes` entry ("FIXED this pass:
  field-diffed the whole mapping table...").
  The `errors.go` literal (the tool's extraction target) is only ever used for `errors.Is`
  identity, never reaches the wire. No new fix needed.

## 2026-08-30 -- filter VALUE-SEMANTICS sweep (gopherstack-uox6 class: a filter field that is
read, applied, and wrong -- distinct from the wrapper-key/wire-completeness axis swept above).
Two real bugs found and fixed in the four filter matchers this service already implements
(DescribeDBInstances/DescribeDBClusters/DescribeDBSnapshots/DescribeDBClusterSnapshots); no
other filter-bearing surface in rds was touched (see the still-current "17 ops silently ignore
Filters entirely" note above -- unchanged, out of this class, not re-investigated this pass).

1. **db-cluster-id and db-instance-id filters rejected ARN-form values.** Each op's own
   `Filters` doc comment in `aws-sdk-go-v2/service/rds@v1.124.1` says these two filter names
   accept "identifiers and ... Amazon Resource Names (ARNs)" -- confirmed individually for
   `DescribeDBInstances` (`db-cluster-id`, `db-instance-id`), `DescribeDBClusters`
   (`db-cluster-id`), `DescribeDBSnapshots` (`db-instance-id`), and
   `DescribeDBClusterSnapshots` (`db-cluster-id`). The other filter names on these same four
   ops (`db-snapshot-id`, `db-cluster-snapshot-id`, `dbi-resource-id`, `db-cluster-resource-id`,
   `engine`, `domain`, `clone-group-id`) each document "Accepts ... identifiers" only, with no
   ARN wording -- confirmed by reading each name's own doc line individually, not assumed from
   the two that do. `matchesAllDBInstanceFilters`/`matchesAllDBClusterFilters`/
   `matchesAllDBSnapshotFilters`/`matchesAllDBClusterSnapshotFilters` compared every filter
   value with a bare-identifier `containsFold`, so a real client passing an ARN (e.g. copied
   from another API response's `DBInstanceArn`/`DBClusterArn` field) matched nothing even
   though the identified resource existed -- under-matching. Fixed by adding
   `containsFoldIDOrARN` (`shared.go`), which normalizes each candidate value through the
   existing `rdsIDFromARN` helper (already used for this exact ID-or-ARN idiom at
   `handler_db_clusters.go:777`, `handler_fault_injection.go:51`, `maintenance.go:52`) before
   the fold-compare, and switching only the `db-cluster-id`/`db-instance-id` match arms in the
   four `matchesAll*Filters` functions to call it. The other filter names in the same switches
   are untouched -- ARN acceptance was added only where each op's own doc comment states it.
   Tests: added an "accepts ARN form" case per op (`db_instances_test.go`,
   `describe_filters_test.go` x3), each confirmed failing against unmodified code first (empty
   result where the ARN's identified resource should have matched) and passing after the fix.
   `Test_DescribeDBInstances_Filters` also gained a `db-cluster-id`-with-plain-identifier case,
   since the prior suite's own doc comment claimed db-cluster-id/dbi-resource-id coverage that
   the case table never actually exercised.

2. **DescribeDBLogFiles' FileSize filter was off-by-one at the boundary.** The op's own doc
   comment: "Filters the available log files for files larger than the specified size" --
   strictly greater than. `LogFileFilter.FileSize`'s matcher (`log_files.go`) excluded only
   `f.Size < filter.FileSize`, i.e. kept files `>= FileSize` ("at least", not "larger than"), so
   a log file whose size exactly equalled the filter value was wrongly included. Fixed the
   comparison to `f.Size <= filter.FileSize` (exclude). This is a self-contained doc/code
   mismatch, not a shared-matcher question: `FileLastWritten`'s own doc ("written since the
   specified date") is inclusive-since and was already correct, left alone. New test
   `TestDescribeDBLogFiles_FileSizeFilterIsStrictlyGreaterThan` (`log_files_test.go`, new file)
   drives the real seeded log files through the handler, reads back an actual file size, then
   filters on that exact value and asserts no returned file has that size -- confirmed failing
   against unmodified code (the boundary file was returned) before the fix.

Both bugs are UNDER-MATCHING (direction 1 of the four: a documented modifier/value form
honoured too narrowly, so records the real service would return are excluded).

**Filter axes checked and found already correct, not just skipped**: within the same four
matchers, the AND-across-filters / OR-within-a-filter's-Values combining rule (verified across
all filter names in all four switches), case-insensitive identifier matching via
`containsFold`/`strs`, and the `isKnown*FilterName` unrecognized-name rejection (all four
return `InvalidParameterValue`, matching AWS) were all read against each op's own doc comment
and are correct -- no change made to any of them.

**Gaps considered and left alone, not fabricated**: `domain` (DescribeDBInstances/
DescribeDBClusters) and `clone-group-id` (DescribeDBClusters) remain accepted-but-vacuous, as
already documented above -- no Directory Service/clone-group state exists in this backend to
match against, and inventing one would be exactly the fabrication this class warns against.

**Web pages fetched this pass**: none. Every filter semantic checked (ARN-vs-identifier
wording, FileSize/FileLastWritten comparison direction) was resolved from the pinned
`aws-sdk-go-v2/service/rds@v1.124.1` Go doc comments in the module cache, per this class's own
"where the documentation lives" guidance.

**Services also considered this pass, found already correct on this exact axis (docdb) or
already exhaustively covered by prior passes (identitystore), so left untouched**:
- `docdb`: `filters.go`'s `matchesIdentifierOrARN` already normalizes ARN-form
  `db-cluster-id`/`db-instance-id` filter values via `identifierFromARN` before comparing --
  confirmed against `DescribeDBClusters`/`DescribeDBInstances`/`DescribeGlobalClusters`/
  `DescribePendingMaintenanceActions`'s own doc comments in `docdb@v1.51.4`, all four of which
  document ARN acceptance for these two filter names and are handled correctly. `events_log.go`'s
  `eventMatches` time-window comparison (`e.Date` vs `filter.StartTime`/`EndTime`, both
  `time.RFC3339`) was checked for the self-inconsistency sub-shape found elsewhere in this
  campaign (nanoseconds-vs-seconds, ISO8601-vs-epoch) and is consistent: the wire always emits
  and accepts `smithytime.FormatDateTime` (RFC3339 with optional fractional seconds), and Go's
  `time.Parse(time.RFC3339, ...)` accepts that fractional form. No bug found; no code changed in
  docdb this pass.
- `identitystore`: `users.go`/`groups.go`'s filter matchers (`matchUserSingleValueFilter`,
  `matchUserMultiValueFilter`, `groupMatchesFilter`) already carry this exact class's
  no-default-matches-everything fix from the 2026-07-25 pass (see that entry above), and were
  re-verified exhaustively against botocore's current model that same pass. Not re-audited line
  by line this session beyond confirming no new filter surface exists (`GetUserId`/`GetGroupId`
  use direct O(1) index lookups, not a filter matcher, so they're outside this class).

Gates: `go build`/`go vet` (rds, docdb, identitystore — clean; repo-wide `go vet ./...` clean,
no cross-service callers touched, no signature changes), `go test -race -count=1
./services/rds/...` and `./services/docdb/... ./services/identitystore/...` (all pass),
`golangci-lint run ./services/rds/...` (0 issues, no `--fix` needed).

## Handler-collision determinism re-audit (2026-08-31, gopherstack-id70)

Re-checked for damage from the handler-resolution defect fixed in
`ef0eef041`. Built the unpatched `cmd/reqfieldscan`/`cmd/reqfielddiff` from
`ef0eef041~1` in a worktree, ran both five times against this package, and
diffed against HEAD.

`cmd/reqfieldscan`: byte-identical across all 5 old runs and HEAD.
`cmd/reqfielddiff`: 580 findings in every one of the 5 old runs and at
HEAD, op.field key sets identical. ZERO DAMAGE -- notable since rds is
query-protocol, the shape family carrying most of this campaign's true
findings elsewhere.

## reqfielddiff suppressed-findings validation pass (2026-08-31, gopherstack-uox6 method, `4daec002d`)

`4daec002d` fixed `cmd/reqfielddiff` counting a backend method's *response*
struct as declaring *request* fields, which had been silently cancelling real
findings repo-wide. rds's tier-1 count moved 143 -> 149. Diffed `-dir rds`
output at HEAD against the same command built from `4daec002d~1` (worktree,
removed after) to isolate exactly the 6 newly surfaced tier-1 findings (all
"documented default; a sibling operation in this service declares the same
field"):

- `CreateDBInstanceReadReplica.DBParameterGroupName` / `.OptionGroupName`
- `RestoreDBInstanceFromDBSnapshot.DBParameterGroupName`
- `RestoreDBInstanceFromS3.DBParameterGroupName` / `.OptionGroupName`
- `RestoreDBInstanceToPointInTime.DBParameterGroupName`

**Precision on this newly surfaced set: 6/6 real (100%).** All six were
confirmed by reading the handler: none of the four handlers read
`DBParameterGroupName`/`OptionGroupName` from `url.Values` at all, while
their sibling `CreateDBInstance`/`ModifyDBInstance` both do (checked, per
this class's "check the sibling operations" rule). No false positives, so no
blind-spot shape applies -- this batch is higher precision than the 85%
(34/40) measured on the old, narrower reqfielddiff set, though a sample of 6
is too small to read much into the gap.

**Two adjacent tier-3 findings fixed opportunistically**, found while reading
the same handlers for the confirmed set (not part of the tier-1 diff, so not
counted in the precision figure above): `RestoreDBInstanceFromDBSnapshot.
OptionGroupName` and `RestoreDBInstanceToPointInTime.OptionGroupName` were
equally unread, in the exact same functions already being edited.

**A response wire-shape bug was found and fixed en route, load-bearing for
all of the above**: `xmlDBParamGroupsWrapper` wrapped a single
`DBParameterGroupStatus` element instead of a repeated `DBParameterGroup`
list (confirmed against `awsAwsquery_deserializeDocumentDBParameterGroupStatusList`,
`deserializers.go:37336`, in `rds@v1.124.1`). `DBInstance.DBParameterGroups`
was therefore *always* empty to a real client on every RDS operation, not
just the four fixed here -- a correctly-populated backend field with no path
to the wire. Fixed by changing the wrapper to a list keyed `DBParameterGroup`
and adding `ParameterApplyStatus="in-sync"` (matching the existing
`OptionGroupMembership` "always applied immediately" convention). Confirmed
via a real-SDK-client repro test before the fix (`DBParameterGroups`
decoded to `[]` for a plain `CreateDBInstance` call with an explicit
`DBParameterGroupName`) and after (decodes correctly); the throwaway repro
test was not committed.

**Defaults implemented vs. declared-only, and why**:
- `RestoreDBInstanceFromDBSnapshot.DBParameterGroupName` and
  `RestoreDBInstanceFromS3.DBParameterGroupName`: doc states a simple,
  non-contingent default ("the default DBParameterGroup for the specified DB
  engine") that matches a convention already established in this codebase
  (`db_clusters.go:34`, `"default." + engine`) -- implemented as
  `"default." + engine`, applied beneath the handler in the backend.
- `CreateDBInstanceReadReplica.DBParameterGroupName`: doc's default is
  explicitly *contingent* (same-Region replica -> source's group;
  cross-Region -> engine default) -- per this class's own guard rail
  ("declare the field and invent no default" for contingent defaults), only
  the explicit-value path was wired; the absent case is unchanged (empty).
- `CreateDBInstanceReadReplica.OptionGroupName`,
  `RestoreDBInstanceFromS3.OptionGroupName`,
  `RestoreDBInstanceFromDBSnapshot.OptionGroupName`,
  `RestoreDBInstanceToPointInTime.OptionGroupName`: no established
  "default option group" convention exists anywhere in this codebase, and
  `CreateDBInstance` itself -- the sibling that already reads
  `OptionGroupName` -- implements no default for it either. Declaring one
  here would be *more* than the primary Create operation does. Left
  declared-only (explicit value honored, absent case stays empty).
- `RestoreDBInstanceToPointInTime.DBParameterGroupName`: pre-existing code
  already defaulted unconditionally to the source instance's group (the bug
  was that an explicit override could never take effect). The current SDK
  doc text for this field actually says "the default DBParameterGroup for
  the specified DB engine," not "the source's" -- a discrepancy from the
  pre-existing default, left unchanged. Correcting it is a value-semantics
  question (gopherstack-uox6's separate axis, not "field never declared")
  and out of this pass's scope; recorded here rather than silently reached
  for.

**Tests**: new file `restore_param_option_group_test.go`, 34
`require`/`assert` calls across 4 top-level tests (8 subtests), driving the
real `aws-sdk-go-v2` client and asserting on the decoded
`DBParameterGroups[0].DBParameterGroupName` /
`OptionGroupMemberships[0].OptionGroupName` response fields. Every subtest
confirmed failing against unmodified code first (the wire-shape fix and the
field-wiring fixes were both required for any of them to pass). The two
"omitted field defaults" subtests (`RestoreDBInstanceFromDBSnapshot`,
`RestoreDBInstanceFromS3`) never set `DBParameterGroupName` in the request at
all. Two existing test files (`db_instances_fields_test.go`,
`db_instances_operations_test.go`) were touched only to add `"", ""` at call
sites for the two backend signature changes below -- zero assertions added,
changed, or dropped in either (diff is call-site-only).

**Signature changes** (services/rds-internal only; `go vet ./...` repo-wide
confirmed no external callers): `CreateDBInstanceReadReplica` gained
`paramGroupName, optionGroupName string`; `RestoreDBInstanceFromS3` gained
`paramGroupName, optionGroupName string`. `RestoreDBInstanceFromDBSnapshot`
and `RestoreDBInstanceToPointInTime` needed no signature change --
`DBInstanceOptions` already carried both fields; only the handlers and
backend logic were wired up.

Gates: `go build ./services/rds/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/rds/...` (pass), `golangci-lint run
./services/rds/...` (0 issues, no `--fix` needed). No `nolint` directives in
any file touched this pass.

## Wrapper-key/per-item sweep, ops absent from this file (2026-08-31, gopherstack-6flj/21my)

Targeted the 19 List*/Describe* operations in rds@v1.124.1 whose names never
appeared anywhere in this file before today -- the standing shortcut for
finding where a dated sweep never reached: DescribeAccountAttributes,
DescribeCertificates, DescribeDBClusterParameterGroups,
DescribeDBClusterSnapshotAttributes, DescribeDBParameterGroups,
DescribeDBProxies, DescribeDBProxyEndpoints, DescribeDBProxyTargetGroups,
DescribeDBProxyTargets, DescribeDBSecurityGroups, DescribeDBSubnetGroups,
DescribeEngineDefaultClusterParameters, DescribeEventCategories,
DescribeEventSubscriptions, DescribeOptionGroupOptions,
DescribeOptionGroups, DescribeReservedDBInstances,
DescribeReservedDBInstancesOfferings, DescribeValidDBInstanceModifications.

Protocol confirmed from rds@v1.124.1's own deserializers.go: `awsAwsquery_`
prefix throughout (query/XML), so smithyxml's `strings.EqualFold` applies --
a wire-tag differing only in case would decode correctly and could not be
caught by this method. No case-only mismatches were found or would apply.

**Bug found and fixed**: `xmlAccountAttribute`
(handler_reference_data.go) had its two non-Max tags swapped. The real
`AccountQuota` member is `AccountQuotaName` (rds@v1.124.1 deserializers.go's
`awsAwsquery_deserializeDocumentAccountQuota`), but the Go field holding the
attribute name string was tagged `xml:"AttributeName"` -- a tag the real
deserializer never matches -- while the Go field holding the numeric `Used`
count was tagged `xml:"AccountQuotaName"`. Pre-fix, a real client's
`AccountQuotaName` decoded a stringified count (e.g. `"40"`) instead of the
attribute's name, and `Used` was permanently nil since nothing was ever
emitted under the real `Used` tag. `Max` was already correct. Fixed by
swapping the two tags to `AccountQuotaName`/`Used`. Test:
`TestDescribeAccountAttributes_QuotaFields_RealClient`
(wire_field_fixes_test.go), drives the real SDK client and asserts
`AccountQuotaName`/`Used`/`Max` all round-trip for the `DBInstances` quota.
Confirmed failing against unmodified code first.

Proxies family (DescribeDBProxies/Endpoints/TargetGroups/Targets),
DBParameterGroups/DBClusterParameterGroups/DBClusterParameters/
EngineDefault(Cluster)Parameters, DBSecurityGroups, DBSubnetGroups,
EventCategories/EventSubscriptions, OptionGroups, ReservedDBInstances(Offerings),
and ValidDBInstanceModifications were all field-diffed per-op against their
own real deserializer (wrapper key, member-wrap shape, and every emitted
item field) and came back clean for what they emit -- the earlier session's
DBProxyTarget.TargetHealth fix (already committed, `7a9a557d8`) was
independently re-verified against the current deserializer and still holds.
No wrapper-key mismatch, no member-wrap-shape mismatch, and no case-only
mismatch found in any of the 19 (beyond the one field-swap above). No hard
decode errors or panics found -- every mismatch in this batch was the
silent-empty/silent-wrong-value shape.

**Real members genuinely absent from this backend's domain model** (checked
against the SDK type, not fabricated): `Certificate.CertificateArn` and
`.CustomerOverrideValidTill` (models.Certificate has no ARN or override-date
field, and no confirmed real ARN format was found in the pinned module
cache's doc comments to synthesize safely); `AccountQuota` has none extra.
`DBSecurityGroup.EC2SecurityGroups`/`.OwnerId`/`.VpcId` (EC2-Classic legacy
fields, not modelled at all -- this backend's `DBSecurityGroup` has no VPC
concept). `Subnet.SubnetAvailabilityZone`/`.SubnetStatus`/`.SubnetOutpost`
(models.DBSubnetGroup stores subnet IDs as bare strings, no per-subnet AZ/
status/outpost data). `DBProxy.DefaultAuthScheme`/`.EndpointNetworkType`/
`.TargetConnectionNetworkType`/`.VpcId`, `UserAuthConfigInfo.ClientPasswordAuthType`
(none tracked by the domain model). `DBProxyEndpoint.VpcId` is declared on
the domain struct (`proxies.go`) but never populated by any code path --
always the zero value, so emitting it would add nothing; left unemitted
rather than wiring a field that can only ever be empty. `Parameter.AllowedValues`/
`.MinimumEngineVersion`/`.SupportedEngineModes` (models.DBParameter tracks
only Name/Value/Description/ApplyType/DataType/Source/ApplyMethod/
IsModifiable). `OptionGroup.Option.DBSecurityGroupMemberships`/
`.OptionSettings`/`.Permanent`/`.Persistent`/`.Port`/`.VpcSecurityGroupMemberships`
(models.OptionGroupOption tracks only OptionName/OptionVersion).
`ReservedDBInstance.LeaseId`/`.RecurringCharges`/`.ReservedDBInstanceArn` and
`ReservedDBInstancesOffering.RecurringCharges` (not modelled; no confirmed
ARN format found to synthesize the Arn field safely).
`ValidDBInstanceModificationsMessage.Storage`/`.AdditionalStorage`/
`.SupportsDedicatedLogVolume` (would need a per-instance-class storage-type
catalog this backend does not have; only `ValidProcessorFeatures` is
modelled and it is correct). `EventSubscription.SubscriptionCreationTime`
(not modelled). These are real, verified-against-the-SDK gaps, not
fabricated ones -- recorded here rather than filled with guessed values.

**Structural gap, not fixed**: `DescribeOptionGroupOptions` is a full stub
(`handleDescribeOptionGroupOptions` in handler_option_groups.go always
returns an empty response with no `OptionGroupOptions` field at all --
confirmed against rds@v1.124.1's real `DescribeOptionGroupOptionsOutput`,
which wraps `[]types.OptionGroupOption`, a static per-engine catalog type
distinct from the per-group `Option` type used elsewhere in this file).
Real AWS returns this catalog unconditionally for any known engine; this
backend has no such catalog data (hundreds of option definitions across
engines) and none was fabricated. Filed as a gap, not a wrapper-key bug,
since there is no wrapper key present to be wrong.

Gates: `go build ./services/rds/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/rds/...` (pass), `golangci-lint run
./services/rds/...` (0 issues). No `nolint` directives in either file
touched this pass (handler_reference_data.go, wire_field_fixes_test.go).

## errtargetaudit class A sweep (2026-09-07, gopherstack-33jc)

`cmd/errtargetaudit` flagged 12 class-A findings (an emitted error code not
in the op's SDK-declared set), all "sentinel reference" mechanism -- a
handler/backend line returning a shared error sentinel whose mapped wire
code isn't in that op's declared set. Verified each against
`aws-sdk-go-v2/service/rds@v1.124.1/deserializers.go`'s
`awsAwsquery_deserializeOpError<Op>` functions (`awk
"/deserializeOpError<Op>\(/,/^}/" deserializers.go | grep -oE
'"[A-Za-z0-9]+"'`; the digit in the class matters, though no rds op's
declared set happened to contain a digit). No adjustment to the extraction
pattern was needed for this query/XML-protocol service -- the same
per-op-function-name/quoted-literal shape used for other protocols held
here too.

**Error-response shape** (confirmed in `handler_dispatch.go`'s
`writeError`/`rdsErrorResponse`, matching `TestRDSErrorCodes_FaultSuffix`'s
comment and `awsxml.GetErrorResponseComponents(errorBody, false)` in the SDK):
every RDS error is HTTP 400 with body
`<ErrorResponse><Error><Code>...</Code><Message>...</Message><Type>Sender</Type></Error></ErrorResponse>`.
A separate translation layer, `rdsErrorCode()` in handler_dispatch.go, maps
each Go sentinel error to its wire `<Code>` string (independent of the
sentinel's own `awserr.New` msg argument) -- the actual bug surface, since a
call site can reference the *wrong* sentinel while that sentinel's own
mapping entry is itself correct for the ops that do use it correctly.

**Verdict table** (12 findings -> 5 root causes):

| Op | Emitted code | Verdict | Root cause | Fix |
|---|---|---|---|---|
| ApplyPendingMaintenanceAction | DBInstanceNotFound | CONFIRMED | RC1 | ErrInstanceNotFound -> new ErrResourceNotFound |
| EnableHttpEndpoint (Handler) | DBClusterNotFoundFault | CONFIRMED | RC2 | ErrClusterNotFound -> ErrResourceNotFound |
| EnableHttpEndpoint (InMemoryBackend) | DBClusterNotFoundFault | CONFIRMED | RC2 | same line as above |
| DisableHttpEndpoint (Handler) | DBClusterNotFoundFault | CONFIRMED | RC2 | ErrClusterNotFound -> ErrResourceNotFound |
| DisableHttpEndpoint (InMemoryBackend) | DBClusterNotFoundFault | CONFIRMED | RC2 | same line as above |
| ModifyActivityStream | DBClusterNotFoundFault | CONFIRMED, NOT FIXED | RC2-adjacent | declared set is {DBInstanceNotFound, InvalidDBInstanceState, ResourceNotFoundFault} -- two plausible replacements, ambiguous, left for filing |
| DeleteCustomDBEngineVersion | DBInstanceNotFound | CONFIRMED | RC3 | ErrInstanceNotFound -> new ErrCustomDBEngineVersionNotFound |
| ModifyCustomDBEngineVersion | DBInstanceNotFound | CONFIRMED | RC3 | ErrInstanceNotFound -> new ErrCustomDBEngineVersionNotFound |
| CreateCustomDBEngineVersion | DBInstanceAlreadyExists | CONFIRMED | RC3 | ErrInstanceAlreadyExists -> new ErrCustomDBEngineVersionAlreadyExists |
| DescribeDBClusterSnapshotAttributes | DBSnapshotNotFound | CONFIRMED | RC4 | ErrSnapshotNotFound -> ErrClusterSnapshotNotFound |
| ModifyDBClusterSnapshotAttribute | DBSnapshotNotFound | CONFIRMED | RC4 | ErrSnapshotNotFound -> ErrClusterSnapshotNotFound |
| DescribeDBClusterEndpoints | DBClusterEndpointNotFoundFault | CONFIRMED | RC5 | declared set is {DBClusterNotFoundFault} only -- DBClusterEndpointIdentifier is a filter param per the real SDK input doc, not an existence check; removed the not-found branch entirely, made it filter like DBClusterIdentifier |

11 of 12 fixed; 1 (ModifyActivityStream) confirmed but left, described
above and in-line, for filing as its own issue -- two declared codes both
plausibly fit and disambiguating needs a real-AWS behavioral test this
session didn't have access to run.

**Root causes**:
- RC1: ApplyPendingMaintenanceAction's ARN can name an instance or a
  cluster; its declared set has no resource-type-specific code, only the
  generic ResourceNotFoundFault (new sentinel, `errors.go`).
- RC2: EnableHttpEndpoint/DisableHttpEndpoint's ResourceArn is likewise
  generic; declared set is {InvalidResourceStateFault, ResourceNotFoundFault}
  -- no DBClusterNotFoundFault at all, despite the sibling
  StartActivityStream/StopActivityStream ops legitimately declaring it.
  Reused the same new ErrResourceNotFound sentinel as RC1.
- RC3: CreateCustomDBEngineVersion's not-found and already-exists sentinels
  were copy-pasted from the DB-instance sentinels (ErrInstanceNotFound/
  ErrInstanceAlreadyExists) instead of engine-version-specific ones; no
  CustomDBEngineVersion*Fault sentinels previously existed. Added both.
- RC4: DescribeDBClusterSnapshotAttributes/ModifyDBClusterSnapshotAttribute
  used ErrSnapshotNotFound (DBSnapshotNotFound, correct for the sibling
  *instance*-snapshot ops DescribeDBSnapshotAttributes/
  ModifyDBSnapshotAttribute) instead of the already-existing
  ErrClusterSnapshotNotFound (DBClusterSnapshotNotFoundFault) -- an
  instance/cluster sentinel mixup, same shape as RC3.
- RC5: DescribeDBClusterEndpoints treated its optional
  DBClusterEndpointIdentifier as a must-exist lookup key instead of a
  filter; real AWS's declared error set for the op has no endpoint-specific
  not-found code at all (confirmed against
  `api_op_DescribeDBClusterEndpoints.go`'s doc comment: "The identifier of
  the endpoint to describe," no not-found language, unlike
  DeleteDBClusterEndpoint/ModifyDBClusterEndpoint, which correctly declare
  and correctly emit DBClusterEndpointNotFoundFault for their own mandatory
  identifiers).

**Gap noted, not fixed** (out of this audit's scope -- a missing declared
code, not a class-A wrong-emitted-code finding): DescribeDBClusterEndpoints
never validates that a non-empty DBClusterIdentifier filter names a real
cluster, so it silently returns an empty list instead of the
DBClusterNotFoundFault real AWS declares for that case. Left as a lead for
the next pass.

Fixed the same day as a follow-up, see "DescribeDBClusterEndpoints
DBClusterIdentifier not-found fix" below (gopherstack-l20u) -- the mirror
image of RC5 above, in the same function.

**Shared-helper check**: grepped `services/docdb` and `services/neptune` for
any import of `github.com/blackbirdworks/gopherstack/services/rds` --
zero hits. Both packages have their own independent
ApplyPendingMaintenanceAction/pending-maintenance implementations (own
files, own InMemoryBackend); neither imports or calls into rds's package.
No shared-helper risk for any of the 5 fixed sites.

**Files changed**: `errors.go` (3 new sentinels: ErrResourceNotFound,
ErrCustomDBEngineVersionNotFound, ErrCustomDBEngineVersionAlreadyExists),
`handler_dispatch.go` (3 new `rdsErrorCode()` mapping entries),
`maintenance.go`, `engine_versions.go`, `cluster_snapshots.go`,
`data_api.go`, `cluster_endpoints.go` (behavioral fix, not just a sentinel
swap -- see RC5).

**Pre-existing tests corrected** (each was pinning the wrong code -- passed
against the unfixed handler, so each is hollow proof until now):
- `maintenance_test.go` `TestRDSBackend_ApplyPendingMaintenanceAction/resource_not_found`:
  `wantErrIs: rds.ErrInstanceNotFound` -> `rds.ErrResourceNotFound`.
- `data_api_test.go` `TestEnableDisableHttpEndpoint/not_found_enable`:
  `wantErrIs: rds.ErrClusterNotFound` -> `rds.ErrResourceNotFound`.
- `cluster_snapshots_test.go` `TestDescribeDBClusterSnapshotAttributes/not_found`
  and `TestModifyDBClusterSnapshotAttribute/not_found`: both
  `wantErrIs: rds.ErrSnapshotNotFound` -> `rds.ErrClusterSnapshotNotFound`.
- `form_actions_cluster_test.go` "DescribeDBClusterEndpoints_NotFound":
  renamed `..._NoMatch`, `wantCode` 400+"DBClusterEndpointNotFound" ->
  200 OK + `wantNotContains: "DBClusterEndpointNotFound"` (RC5's behavior
  change, not just a code swap).
- `cluster_endpoints_test.go` `TestDeleteDBCluster_CascadeDeletesClusterEndpoints`:
  post-delete assertions on the two leaked endpoints changed from
  `require.ErrorIs(t, err, rds.ErrClusterEndpointNotFound)` to
  `require.NoError` + `assert.Empty`, matching RC5.
- `dispatch_test.go` `TestRDSHandler_NewOperations2/ApplyPendingMaintenanceAction_not_found`:
  `wantContains: "DBInstanceNotFound"` -> `"ResourceNotFoundFault"`.

6 pre-existing tests corrected across 5 files. New coverage:
`error_codes_test.go`'s new `TestRDSErrorCodes_ClassASweep` (8 subtests, one
per fixed call site except the merged Enable/Disable-by-domain pair, each
asserting the correct wire `<Code>` present AND the old wrong code absent).
Legitimate uses of the sentinels these fixes stopped misusing are already
covered by pre-existing, unmodified tests and were re-verified to still
pass: `DBInstanceNotFound` by `DownloadDBLogFilePortion_NotFound` and
`DescribeValidDBInstanceModifications_NotFound`
(form_actions_cluster_test.go); `DBClusterNotFoundFault` by "DeleteDBCluster
not found" (error_codes_test.go); `DBInstanceAlreadyExists` by
`CreateDBInstance_Duplicate` (form_actions_test.go); `DBSnapshotNotFound` by
`TestDescribeDBSnapshotAttributes/not_found` (db_snapshots_test.go);
`DBClusterEndpointNotFoundFault` by `DeleteDBClusterEndpoint_NotFound`
(form_actions_cluster_test.go).

**Neuter pass**: each of the 8 changed lines/blocks (RC1 x1, RC2 x2, RC3
x3, RC4 x2, RC5 x1 behavioral) was individually reverted, confirmed to
still `go build`, confirmed to fail exactly the expected test(s)
(`TestRDSErrorCodes_ClassASweep` subtests plus
`TestRDSBackend_ApplyPendingMaintenanceAction`,
`TestEnableDisableHttpEndpoint`,
`TestDeleteDBCluster_CascadeDeletesClusterEndpoints`,
`TestRDSHandler_FormActions_Clusters/DescribeDBClusterEndpoints_NoMatch`,
`TestRDSHandler_NewOperations2/ApplyPendingMaintenanceAction_not_found`),
then restored. Final `git diff --stat` on the 7 non-test source files
matched the intended fix exactly, confirming no stray change survived the
revert/restore cycles.

**No persisted struct fields were added** -- all changes are to error
sentinels and one filter-vs-lookup behavior change, so the
`pkgs/persistence` guard does not apply this pass.

Gates: `go build ./services/rds/...` (clean), `go vet ./services/rds/...`
(clean), `go test -race -count=1 ./services/rds/...` (pass, 0 failures),
`golangci-lint run services/rds/...` (0 issues).
`cmd/errtargetaudit` rds class-A findings: 12 -> 1.

## DescribeDBClusterEndpoints DBClusterIdentifier not-found fix (2026-09-07, gopherstack-l20u)

Follow-up to RC5 above (gopherstack-33jc), same function, opposite param.
33jc fixed the case where `DBClusterEndpointIdentifier` was wrongly treated
as a must-exist key (it's a filter; declared set has no endpoint-specific
not-found code). This pass fixes the mirror-image gap `errtargetaudit`
can't see, because it's a *missing* check, not a wrong emitted code:
`DBClusterIdentifier` is the op's one declared error,
`DBClusterNotFoundFault` (confirmed: `awk
"/deserializeOpErrorDescribeDBClusterEndpoints\(/,/^}/"
aws-sdk-go-v2/service/rds@v1.124.1/deserializers.go | grep -oE
'"[A-Za-z0-9]+"'` returns only `DescribeDBClusterEndpointsResult`,
`UnknownError`, and `DBClusterNotFoundFault`), and a supplied-but-unknown
cluster was silently returning an empty list instead.

Both `DBClusterIdentifier` and `DBClusterEndpointIdentifier` are optional
pointer fields on `DescribeDBClusterEndpointsInput` (confirmed against
`api_op_DescribeDBClusterEndpoints.go`) -- so omitting `DBClusterIdentifier`
must still list all endpoints; only a non-empty, unmatched value should
fault. In-service control: `DescribeDBClusters`/`DescribeDBInstances`
already follow exactly this shape -- non-empty id not found -> the op's
not-found sentinel; empty id -> list all
(db_clusters.go:109-121, db_instances.go:352-372).

**Fix**: `InMemoryBackend.DescribeDBClusterEndpoints` (cluster_endpoints.go)
now checks, when `clusterID != ""`, that the cluster exists before
filtering, returning `ErrClusterNotFound` (wire: `DBClusterNotFoundFault`,
already mapped in `handler_dispatch.go`'s `rdsErrorCode()` table -- no new
sentinel or mapping entry needed) otherwise. The endpoint-filter loop below
is unchanged.

**Pre-existing test corrected** (was pinning the bug):
`cluster_endpoints_test.go`'s `TestDeleteDBCluster_CascadeDeletesClusterEndpoints`
asserted, for the deleted (now-unknown) `"leak-cluster"` identifier itself:
```go
after, err := b.DescribeDBClusterEndpoints("leak-cluster", "")
require.NoError(t, err)
assert.Empty(t, after)
```
changed to:
```go
_, err = b.DescribeDBClusterEndpoints("leak-cluster", "")
require.ErrorIs(t, err, rds.ErrClusterNotFound)
```
(The two endpoint-identifier-filter assertions in the same test, `got1`/
`got2`, are unaffected -- that's the 33jc half, still correctly empty.)

**New coverage**: `TestDescribeDBClusterEndpoints_IdentifierVsFilter`
(cluster_endpoints_test.go), driven through the HTTP handler and asserting
the wire `<Code>`, pins both halves in one place plus the non-broad-guard
case: unknown `DBClusterIdentifier` -> `DBClusterNotFoundFault` (400);
unknown `DBClusterEndpointIdentifier` -> 200 empty; valid cluster + valid
endpoint filter -> 200 with the endpoint present.

**Neuter pass**: commented out the new existence check (the 5-line `if
clusterID != ""` block in cluster_endpoints.go); confirmed `go build
./services/rds/...` still succeeded; confirmed exactly the expected two
failures --
`TestDeleteDBCluster_CascadeDeletesClusterEndpoints` (cluster_endpoints_test.go:56)
and
`TestDescribeDBClusterEndpoints_IdentifierVsFilter/unknown_cluster_identifier_faults`
(cluster_endpoints_test.go:108, HTTP 200 instead of 400) -- then restored
and reran the full package, 0 failures.

**No persisted struct fields added** -- `pkgs/persistence` guard not
applicable.

`cmd/errtargetaudit` rds class-A findings: unchanged at 1 (this was a
missing-check gap, not a wrong-emitted-code finding, so the tool doesn't
and didn't flag it either before or after).

Gates: `go test -race -count=1 ./services/rds/...` (pass, 0 failures),
`golangci-lint run services/rds/...` (0 issues).

## ModifyActivityStream not-found code fix (2026-09-07, gopherstack-fm1e)

Follow-up to the 33jc verdict table above, which left this one confirmed
but unfixed: `ModifyActivityStream` emitted `DBClusterNotFoundFault`, not
in its declared set `{DBInstanceNotFound, InvalidDBInstanceState,
ResourceNotFoundFault}` (re-confirmed via `awk
"/deserializeOpErrorModifyActivityStream\(/,/^}/"
aws-sdk-go-v2/service/rds@v1.124.1/deserializers.go | grep -oE
'"[A-Za-z0-9]+"'`). Two declared codes plausibly fit by name
(`DBInstanceNotFound`, `ResourceNotFoundFault`); the issue asked to pick
deliberately by checking what real AWS resolves and what this backend
looks up, not by analogy to the sibling fix (`ApplyPendingMaintenanceAction`
-> `ResourceNotFoundFault`).

**What real AWS does**: unlike `Start`/`StopActivityStream` (Aurora
cluster-scoped, ARN doc "the DB cluster", and their own declared sets
include `DBClusterNotFoundFault`), `ModifyActivityStream`'s own doc
comment reads "This operation is supported for RDS for Oracle and
Microsoft SQL Server" and its `ResourceArn` field doc reads "The Amazon
Resource Name (ARN) of the RDS for Oracle or Microsoft SQL Server DB
instance" (`api_op_ModifyActivityStream.go`) -- confirmed by the absence
of `DBClusterNotFoundFault` from its declared set entirely, unlike
`StartActivityStream`'s (which declares both `DBClusterNotFoundFault` and
`DBInstanceNotFound`, since it's genuinely dual-scoped). So real
`ModifyActivityStream` never receives a cluster ARN; the identifier is
always a DB instance identifier by contract, not ambiguous the way
`ApplyPendingMaintenanceAction`'s ARN genuinely is.

**What gopherstack looks up**: `InMemoryBackend.ModifyActivityStream`
(activity_stream.go) resolves the ARN's trailing segment
(`arnToClusterID`, shared verbatim with Start/Stop) against `b.clusters`
only -- it has no DB-instance-scoped activity-stream modeling at all, an
implementation-storage detail shared with Start/Stop rather than a
faithful model of the real op's instance-only scope. That gap is
unfixed here (out of scope: a modeling gap, not a wrong-emitted-code
finding) and not previously disclosed; noted for a future pass.

**Fix**: the not-found branch now returns `ErrInstanceNotFound` (wire
`DBInstanceNotFound`, already mapped in `handler_dispatch.go`'s
`rdsErrorCode()` table -- no new sentinel needed), justified by
`DBInstanceNotFoundFault`'s own doc comment, "DBInstanceIdentifier
doesn't refer to an existing DB instance" (`types/errors.go`), a
word-for-word fit for "the identifier this instance-scoped op received
does not resolve." `ResourceNotFoundFault` (generic, "The specified
resource ID was not found") was rejected as the wrong choice here
specifically because the op is *not* ambiguous the way
`ApplyPendingMaintenanceAction`'s is -- a more specific declared code
with matching doc text is available. Single call site
(`handler_activity_stream.go`'s `handleModifyActivityStream` is the only
caller of `Backend.ModifyActivityStream`); Start/Stop untouched, both
still correctly cluster-scoped.

**Pre-existing test corrected** (was pinning the wrong behavior):
`activity_stream_test.go`'s `TestActivityStream_ClusterNotFound` asserted
`DBClusterNotFound` for all three of Start/Stop/Modify against a missing
cluster ARN. Split into a per-case table asserting `DBClusterNotFound` for
Start/Stop (unchanged, still correct) and `DBInstanceNotFound` (absence of
`DBClusterNotFound`) for Modify. Pre-fix run of the Modify subtest failed:
```
activity_stream_test.go:171: ... does not contain "DBInstanceNotFound"
activity_stream_test.go:173: ... should not contain "DBClusterNotFound"
```
confirming the emitted code was `DBClusterNotFoundFault` before the fix.

**No persisted struct fields added.**

`cmd/errtargetaudit` rds class-A findings: 1 -> 0.

Gates: `go test -race -count=1 ./services/rds/...` (pass, 0 failures),
`golangci-lint run ./services/rds/...` (0 issues).
