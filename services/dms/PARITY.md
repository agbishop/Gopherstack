---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: dms
# NOTE: the SDK module name does NOT match the services/dms directory name.
# go.mod pins github.com/aws/aws-sdk-go-v2/service/databasemigrationservice
# (NOT service/dms -- that module does not exist). A prior automated tool
# (cmd/opcensus) resolved the SDK module from the directory name, found no
# service/dms, and reported 0 List/Describe/Get ops for this service --
# which is WRONG; the pinned SDK actually declares 119 operations
# (ls api_op_*.go in the module cache), all 119 of which this service
# implements (handler.go's op-name const block; confirmed by direct count
# 2026-08-20). Do not repeat cmd/opcensus's mistake when re-auditing.
sdk_module: aws-sdk-go-v2/service/databasemigrationservice@v1.66.4
last_audit_commit: f16ac0367fc476ca2ffd1643ed5ef900b9ff0480
last_audit_date: 2026-08-29
overall: A            # 2026-08-29 (gopherstack-21my, parameter-honoring sweep): audited a coherent slice
                       # of ~44 Filters/pagination-bearing Describe ops (Filters+Marker/MaxRecords or
                       # Filters+NextToken/MaxRecords), not the full 47-op Describe/List surface. Fixed
                       # 11 real "declared parameter, never honored" bugs across Fleet Advisor (Collectors/
                       # Databases -- input struct not even bound, plus an adjacent CollectorHealthCheck
                       # wire-shape bug found and fixed while proving it with the real SDK client),
                       # InstanceProfiles, MigrationProjects (5 filters), Recommendations, ReplicationTasks
                       # (3 of 5 documented filters were unread), TableStatistics (real per-table state,
                       # unlike its always-empty ReplicationTableStatistics sibling), Events (5 top-level
                       # request members distinct from Filters, never plumbed at all), DataMigrations
                       # (WithoutSettings), ReplicationSubnetGroups, and EndpointTypes (static catalog, but
                       # cheap and documented, so fixed rather than left). See per-op notes below and
                       # list_filter_params_test.go (12 tests, each driving the real dmssdk client,
                       # confirmed failing pre-fix). Metadata-model family (6 ops sharing
                       # listMetadataModelRequests) and the DescribeReplicationTaskAssessment* family were
                       # re-checked and confirmed already correct, not re-fixed. NOT covered this pass:
                       # DescribeConnections/Endpoints/Certificates/EventSubscriptions/DataProviders/
                       # ReplicationInstances/ReplicationConfigs/Replications filter-honoring (spot-checked
                       # clean, see per-op notes, but not exhaustively re-verified against every documented
                       # filter name this pass) and DescribePendingMaintenanceActions (Filters declared but
                       # genuinely inert -- no pending-maintenance-action state is ever produced by this
                       # backend's ApplyPendingMaintenanceAction, a structural gap, not a bug: filtering an
                       # always-empty list has no observable effect). DescribeReplicationConfigs's Filters
                       # (api_op_DescribeReplicationConfigs.go documents no filter-name vocabulary at all,
                       # unlike every sibling op) was deliberately NOT fixed -- borrowing the sibling
                       # DescribeReplications' replication-config-arn/id names would be a defensible
                       # inference but is not SDK-confirmed for this specific op, and this campaign's rule
                       # is to take vocabulary from the operation's own documentation, never invent it.
                       # 2026-08-20 pass: field-diffed the Endpoint and
                       # ReplicationInstance envelopes (this campaign's top
                       # two priorities for this service) directly against
                       # types.go/api_op_*.go and found + fixed 3 real bugs:
                       # 6 top-level Endpoint connection-settings fields
                       # (CertificateArn/ExtraConnectionAttributes/KmsKeyId/
                       # ServiceAccessRoleArn/SslMode/ExternalTableDefinition)
                       # missing on both Create/Modify request AND response;
                       # 4 top-level ReplicationInstance fields (KmsKeyId/
                       # DnsNameServers/NetworkType/PreferredMaintenanceWindow)
                       # missing the same way; and a fabricated enum value
                       # ("GA") on DescribeOrderableReplicationInstances.
                       # ReleaseStatus where the real ReleaseStatusValues enum
                       # only has beta/prod. Also fixed a lower-severity wire
                       # mismatch (DescribeFleetAdvisorCollectors used a
                       # fabricated Marker field instead of the real
                       # NextToken). See the 2026-08-20 Notes entry below for
                       # full detail, hand-revert proof, and what was
                       # deliberately left as a disclosed gap (engine-specific
                       # settings blocks, PendingModifiedValues,
                       # KerberosAuthenticationSettings, FreeUntil,
                       # SecondaryAvailabilityZone).
                       #
                       # 2026-07-23 pass: closed all 4 gaps + all 3 deferred
                       # families from the prior audit (DescribeMetadataModel
                       # shape, ReloadTables/ReloadReplicationTables state
                       # validation + wire-field-name bug, Endpoint enum
                       # validation, ApplyPendingMaintenanceAction enum
                       # validation, the whole metadata-model Describe/Cancel/
                       # GetTargetSelectionRules wire-shape family, Fleet
                       # Advisor Lsa/SchemaObjectSummary/Schemas field-diff,
                       # and a real premigration-assessment-run state machine
                       # replacing always-empty individual-assessment/result
                       # lists). Also fixed a genuine epoch-timestamp bug
                       # (InstanceCreateTime/ReplicationTaskCreationDate were
                       # missing from the wire entirely).
                       #
                       # 2026-07-31 correction: that A rating was overstated.
                       # A dashboard sweep found 5 real field-level wire-shape
                       # bugs the 07-23 pass's "wire: ok" marks missed on
                       # EventSubscription, ReplicationSubnetGroup,
                       # Certificate, Endpoint, and DescribeConnections (see
                       # per-op notes below) -- all field-diffed against
                       # aws-sdk-go-v2/service/databasemigrationservice
                       # models.go directly this pass, not assumed. All 5 are
                       # now fixed, each with a test that fails against the
                       # pre-fix code and passes after. Re-graded A only
                       # because the fixes are landing in the same pass as
                       # this correction -- the prior "no phantoms" A claim
                       # about the op list itself remains true and unaffected
                       # by this correction (these were field-shape bugs
                       # within existing ok ops, not phantom/missing ops).
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateReplicationInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- InstanceCreateTime was entirely missing from the wire response (epoch-seconds bug class); now emitted via pkgs/awstime.Epoch. FIXED 2026-08-20 -- KmsKeyId/DnsNameServers/NetworkType/PreferredMaintenanceWindow (real CreateReplicationInstanceInput members, api_op_CreateReplicationInstance.go) were entirely absent from the request AND the ReplicationInstance response; now accepted, stored, and echoed. See ReplicationInstanceSettings in replication_instances.go. FIXED 2026-08-29 (write-only-state sweep) -- ReplicationSubnetGroupIdentifier and VpcSecurityGroupIds (also real CreateReplicationInstanceInput members) were STILL entirely unaccepted after the 08-20 pass, which fixed the sibling scalar settings but missed these two: a real client's subnet-group/security-group placement was silently discarded, and the response's ReplicationSubnetGroup was a hardcoded empty-identifier placeholder (VpcSecurityGroups a hardcoded empty list) regardless of what was requested. Now: ReplicationSubnetGroupIdentifier is existence-checked against the ReplicationSubnetGroup store (ResourceNotFoundFault-equivalent if unknown) and its identifier stored/echoed; VpcSecurityGroupIds is stored and echoed as real []types.VpcSecurityGroupMembership{VpcSecurityGroupId, Status:\"active\"} entries. See ReplicationInstanceSettings in replication_instances.go (ReplicationSubnetGroupID/VpcSecurityGroupIDs fields) and riToJSON in handler_replication_instances.go. Proven by TestReplicationInstance_SubnetGroupAndVpcSecurityGroups_RealClient (wire_field_fixes_test.go), real client round trip, hand-reverted and confirmed failing pre-fix."}
  DescribeReplicationInstances: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-20 -- same KmsKeyId/DnsNameServers/NetworkType/PreferredMaintenanceWindow fix as CreateReplicationInstance above (shared riToJSON). FIXED 2026-08-29, see CreateReplicationInstance above -- same riToJSON fix, ReplicationSubnetGroup/VpcSecurityGroups now real values."}
  DeleteReplicationInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "rejects delete while tasks attached"}
  ModifyReplicationInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-20 -- NetworkType/PreferredMaintenanceWindow (real ModifyReplicationInstanceInput members) were accepted nowhere; now accepted and applied. KmsKeyId is deliberately NOT accepted here -- the real ModifyReplicationInstanceInput has no KmsKeyId member (create-only in real AWS); proven unchanged by TestReplicationInstanceSettings_SDKRoundTrip. FIXED 2026-08-29 -- VpcSecurityGroupIds (also a real ModifyReplicationInstanceInput member) now accepted and applied; ReplicationSubnetGroupIdentifier is deliberately NOT accepted here (real ModifyReplicationInstanceInput has no such member, create-only) -- proven unchanged by TestReplicationInstance_SubnetGroupAndVpcSecurityGroups_RealClient."}
  RebootReplicationInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "synchronous no-op reboot is correct emulation -- real reboot causes only a momentary outage, no persistent field changes"}
  ApplyPendingMaintenanceAction: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass -- ApplyAction/OptInType previously accepted arbitrary strings; now validated against the SDK's documented valid-values lists (os-upgrade|system-update|db-upgrade|os-patch and immediate|next-maintenance|undo-opt-in), 400 ValidationException otherwise. Still correctly returns an empty PendingMaintenanceActionDetails -- no pending-maintenance-action producer exists in this emulation, matching a freshly-created instance's real state."}
  CreateEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "EndpointType/EngineName validated against types.ReplicationEndpointTypeValue and the documented EngineName valid-values list. FIXED 2026-07-31 -- Password was accepted in the request but silently dropped (never stored, never usable); now stored on Endpoint.Password and never put on the wire (matching the real Endpoint type, which has no Password field -- AWS never echoes credentials back). FIXED 2026-08-10 (gopherstack-z79q) -- CreateEndpointInput/ModifyEndpointInput's 19 heterogeneous engine-specific settings structs (MySQLSettings/PostgreSQLSettings/S3Settings/OracleSettings/... totaling ~300 fields) were being silently dropped by encoding/json instead of modeled. Judgment: modeling all ~300 fields faithfully (validated types, stored, echoed on Describe, persisted) is not achievable in one pass, and a partial subset would be worse than the honest gap (a client seeing some settings preserved would reasonably assume the rest are too). Per the no-stub rule, the drop is now made visible instead: any request that sets one of the 19 settings fields is rejected with 400 ValidationException naming the field, matching the sagemaker PipelineDefinitionS3Location / cloudformation AccountFilterType precedent for explicitly-rejected-rather-than-silently-dropped fields. See engineSettingsFields in handler_endpoints.go. FIXED 2026-08-20 -- 6 top-level (non-engine-specific) connection-settings members were ALSO missing, separately from the engine-settings gap above: CertificateArn/ExtraConnectionAttributes/KmsKeyId/ServiceAccessRoleArn/SslMode/ExternalTableDefinition (all real CreateEndpointInput members, api_op_CreateEndpoint.go). These are simple scalars unrelated to the ~300-field engine-settings problem and are now accepted, validated (SslMode against types.DmsSslModeValue: none|require|verify-ca|verify-full, defaulting to none), stored, and echoed. See EndpointConnectionSettings in endpoints.go."}
  DescribeEndpoints: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-20 -- same 6-field connection-settings fix as CreateEndpoint above (shared epToJSON)"}
  DeleteEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "rejects delete while referenced by a task"}
  ModifyEndpoint: {wire: ok, errors: ok, state: ok, persist: ok, note: "EndpointType/EngineName accepted on Modify, validated with the same enum check as Create, and applied. FIXED 2026-07-31 -- same Password fix as CreateEndpoint above. FIXED 2026-08-10 (gopherstack-z79q) -- same engine-settings explicit-rejection fix as CreateEndpoint above. FIXED 2026-08-20 -- CertificateArn/ExtraConnectionAttributes/ServiceAccessRoleArn/SslMode/ExternalTableDefinition (same gap as CreateEndpoint above) now accepted and applied. KmsKeyId is deliberately NOT accepted here -- the real ModifyEndpointInput has no KmsKeyId member (create-only in real AWS); proven unchanged by TestEndpointConnectionSettings_SDKRoundTrip."}
  TestConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "records a Connection row, visible via DescribeConnections"}
  DescribeConnections: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-07-31 -- never called dmsPaginate or set Marker on the response, unlike every other Describe op in this service, so MaxRecords/Marker were silently ignored; now paginated like its siblings"}
  DeleteConnection: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateReplicationTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "validates source/target endpoint and instance ARNs exist. FIXED this pass -- ReplicationTaskCreationDate was entirely missing from the wire response (epoch-seconds bug class); now emitted via pkgs/awstime.Epoch. FIXED 2026-08-29 (write-only-state sweep) -- CdcStartPosition, CdcStopPosition, and TaskData (real CreateReplicationTaskInput members, api_op_CreateReplicationTask.go, all three also real top-level types.ReplicationTask response members) were entirely unaccepted: a real client's CDC checkpoint positions and task data were silently discarded by encoding/json with no error, and the domain model had no field to store them even if the wire had accepted them. Now accepted, stored, and echoed -- see ReplicationTaskCDCSettings in replication_tasks.go. CdcStartTime is request-only (no matching response field on types.ReplicationTask) and intentionally not modeled."}
  DescribeReplicationTasks: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29, see CreateReplicationTask above -- same CdcStartPosition/CdcStopPosition/TaskData fix (shared rtToJSON). FIXED 2026-08-29 (wrapper-key/parameter-honoring sweep, gopherstack-21my) -- DescribeReplicationTasksInput documents 5 filter names (replication-task-arn|replication-task-id|migration-type|endpoint-arn|replication-instance-arn, api_op_DescribeReplicationTasks.go); only the first two were honored (via the identifier lookup passed to the backend). migration-type/endpoint-arn/replication-instance-arn were silently ignored -- a client filtering by any of the three got the full unfiltered list back with 200 OK. Now applied as a post-filter against ReplicationTask.MigrationType/Source+TargetEndpointArn/ReplicationInstanceArn. See TestDescribeReplicationTasksFilter_MigrationTypeAndArns (list_filter_params_test.go), real SDK client round trip."}
  StartReplicationTask: {wire: ok, errors: ok, state: ok, persist: ok}
  StopReplicationTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "rejects stop unless currently running"}
  DeleteReplicationTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "rejects delete while running"}
  ModifyReplicationTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "rejects modify while running. FIXED 2026-08-29, see CreateReplicationTask above -- ModifyReplicationTaskInput also carries CdcStartPosition/CdcStopPosition/TaskData; now accepted and applied (only-overwrite-non-empty semantics, matching every other ModifyReplicationTask field)."}
  MoveReplicationTask: {wire: ok, errors: ok, state: ok, persist: ok}
  ReloadTables: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass -- was a disguised no-op that echoed ReplicationTaskArn without validating anything; now requires TablesToReload, validates ReloadOption enum, 404s on an unknown task, and 400 InvalidResourceStateFault unless the task is currently RUNNING (matches the SDK doc: 'You can only use this operation with a task in the RUNNING state')"}
  ReloadReplicationTables: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass -- two bugs: (1) the request field was wrongly named ReplicationTaskArn instead of the real ReplicationConfigArn, silently discarding the client's ARN; (2) it never validated anything. Now requires TablesToReload, validates ReloadOption, 404s on an unknown replication config, and 400s unless the associated Replication is RUNNING"}
  DescribeReplicationTableStatistics: {wire: ok, errors: ok, state: partial, persist: n/a, note: "FIXED 2026-08-11 -- request/response fields were copy-pasted from the sibling DescribeTableStatistics op (ReplicationTaskArn/TableStatistics) instead of this op's real fields (ReplicationConfigArn/ReplicationTableStatistics); the wrong request field meant the config ARN was silently discarded and the handler queried an arbitrary replication task instead. Now validates the config exists (404 if not) and echoes ReplicationConfigArn. Always returns an empty ReplicationTableStatistics list -- ReplicationConfig carries no TableMappings state in this emulation (see models.go), so per-table stats have no honest backend source; adding fabricated stats would be worse than an accurate empty list. 2026-08-12 (gopherstack-o53q): Filters []types.Filter is now accepted on the wire for shape parity, but deliberately left inert and documented as such -- filtering an always-empty list has no observable effect, and there is no per-table state anywhere in this emulation for a filter to narrow."}
  DescribeTableStatistics: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-08-29 (gopherstack-21my) -- unlike its sibling DescribeReplicationTableStatistics (always-empty, inert Filters is correct there), this op DOES have real per-table state (buildTableStatistics derives rows from the task's TableMappings), so its documented Filters (schema-name|table-name|table-state, api_op_DescribeTableStatistics.go) and Marker/MaxRecords pagination were a real, observable gap -- both were declared on the request struct but never read. Now applied. See TestDescribeTableStatisticsFilter (list_filter_params_test.go), real SDK client round trip."}
  CreateReplicationSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "validates ReplicationSubnetGroupDescription/SubnetIds as required (real API marks both required); SubnetIds accepted but not modeled (no VPC subnet emulation), matching pre-existing convention. FIXED 2026-07-31 -- the response wire shape emitted a ReplicationSubnetGroupArn field; the real ReplicationSubnetGroup type has no Arn field at all (subnet groups are referenced by identifier on the wire; a client must build the ARN itself from the deterministic arn:aws:dms:<region>:<account>:subgrp:<identifier> format to tag one). Field removed from the wire struct; the internal Go model still tracks an ARN for indexing/tagging lookups, which is correct -- only the JSON response was wrong"}
  DescribeReplicationSubnetGroups: {wire: ok, errors: ok, state: ok, persist: ok, note: "same Arn-field fix as CreateReplicationSubnetGroup (2026-07-31). FIXED 2026-08-29 (gopherstack-21my) -- Filters []types.Filter (documented: 'Valid filter names: replication-subnet-group-id', api_op_DescribeReplicationSubnetGroups.go) was declared on the request struct but never read at all; now applied against ReplicationSubnetGroupIdentifier. See TestDescribeReplicationSubnetGroupsFilter (list_filter_params_test.go), real SDK client round trip."}
  ModifyReplicationSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "a real backend.ModifyReplicationSubnetGroup mutates and persists the description. Same Arn-field fix as CreateReplicationSubnetGroup (2026-07-31)"}
  DeleteReplicationSubnetGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateReplicationConfig: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-4ggy: ComputeConfig AND TableMappings were both dropped entirely (issue named only ComputeConfig; TableMappings is also a required CreateReplicationConfigInput member per validateOpCreateReplicationConfigInput, validators.go, and was equally absent from the request struct -- floor confirmed). Both now required, stored, and echoed back on the ReplicationConfig response (types.go:3820 ComputeConfig/TableMappings), matching real AWS. ComputeConfig's own members are all optional (no field on types.ComputeConfig, types.go:190, is individually required)."}
  DescribeReplicationConfigs: {wire: ok, errors: ok, state: ok, persist: ok, note: "echoes the ComputeConfig/TableMappings fixed above (shares replicationConfigJSON/rcToJSON with Create/Delete/Modify)"}
  ModifyReplicationConfig: {wire: ok, errors: ok, state: ok, persist: ok, note: "gap not fixed this pass: real ModifyReplicationConfigInput also accepts ComputeConfig/TableMappings/ReplicationSettings/SupplementalSettings for updating an existing config; this handler only accepts ReplicationType. All are optional on Modify (no required-field-drop bug, out of scope for gopherstack-4ggy's required-member sweep), but worth a follow-up completeness pass."}
  DeleteReplicationConfig: {wire: ok, errors: ok, state: ok, persist: ok}
  StartReplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- was a total disguised no-op: ignored ReplicationConfigArn/StartReplicationType, never validated the config existed, and returned an empty envelope instead of the real StartReplicationOutput{Replication}. Now validates StartReplicationType enum, rejects unknown config (404) and already-running (400), transitions Status created->running, and returns the wire-accurate Replication shape."}
  StopReplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- same disguised-no-op class as StartReplication; now validates config exists and is running, transitions Status running->stopped, returns the Replication shape."}
  DescribeReplications: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- always returned an empty list regardless of any StartReplication ever called. Now backed by the replication-config table (every config has an implicit Replication resource, Status starts 'created'), supports replication-config-arn/replication-config-id filters and Marker pagination."}
  AddTagsToResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  RemoveTagsFromResource: {wire: ok, errors: ok, state: ok, persist: ok}
  StartRecommendations: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- ignored DatabaseId entirely and never touched backend state (empty envelope was correct per SDK, but the required side effect -- a recommendation later visible via DescribeRecommendations -- never happened). Now validates DatabaseId is required and records a Recommendation via new backend.StartRecommendation."}
  BatchStartRecommendations: {wire: ok, errors: ok, state: ok, persist: ok, note: "seeds a recommendation per source endpoint; pre-existing, unchanged"}
  DescribeRecommendations: {wire: ok, errors: ok, state: ok, persist: n/a, note: "recommendations are runtime-only (not in backendSnapshot); acceptable since Fleet Advisor overall is a low-value, AWS-EOL'd (May 2026) feature surface. FIXED 2026-08-29 (gopherstack-21my) -- Filters []types.Filter (documented: 'Valid filter names: database-id | engine-name', api_op_DescribeRecommendations.go) and MaxRecords/NextToken pagination were both declared but never read; a client filtering or paging got the full unfiltered list every time. Now applied against Recommendation.DatabaseID/EngineName, with dmsPaginate wired through NextToken. See TestDescribeRecommendationsFilter (list_filter_params_test.go), real SDK client round trip."}
  CreateDataMigration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23 (gopherstack-v4a4) -- dataMigrationJSON wrote NumberOfJobs/EnableCloudwatchLogs flat on DataMigration; the real DataMigration case list (deserializers.go:16304) has no such keys at all -- both nest under a DataMigrationSettings sub-object, and the boolean renames to CloudwatchLogsEnabled there (deserializers.go:16546). Every real client's DataMigration.DataMigrationSettings decoded nil on all 6 ops sharing dmToJSON."}
  DescribeDataMigrations: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-12 (gopherstack-o53q) -- real DescribeDataMigrationsInput carries Filters []types.Filter, entirely absent from the request struct; a client's filter was silently dropped and the call returned success with the unfiltered list. Filters (data-migration-identifier) now merges with the existing DataMigrationIdentifier field and narrows the result. Also FIXED 2026-08-23, see CreateDataMigration -- same dmToJSON DataMigrationSettings bug. FIXED 2026-08-29 (gopherstack-21my) -- WithoutSettings (documented: 'avoid returning information about settings', api_op_DescribeDataMigrations.go) wasn't even declared on the request struct, so a client's true value could never be read; DataMigrationSettings is now a pointer, nilled out when WithoutSettings=true. WithoutStatistics is a disclosed no-op: DataMigrationStatistics is not modeled anywhere in this emulation (no field to suppress), a structural gap distinct from this fix. See TestDescribeDataMigrationsWithoutSettings (list_filter_params_test.go), real SDK client round trip."}
  ModifyDataMigration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23, see CreateDataMigration -- same dmToJSON DataMigrationSettings bug."}
  DeleteDataMigration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23, see CreateDataMigration -- same dmToJSON DataMigrationSettings bug."}
  StartDataMigration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23, see CreateDataMigration -- same dmToJSON DataMigrationSettings bug."}
  StopDataMigration: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-23, see CreateDataMigration -- same dmToJSON DataMigrationSettings bug."}
  CreateDataProvider: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeDataProviders: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-12 (gopherstack-o53q) -- same Filters-absent bug class as DescribeDataMigrations. Filters (data-provider-identifier) now merges with the existing DataProviderIdentifier field and narrows the result."}
  ModifyDataProvider: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-11 -- request field was named DataProviderArn; the real ModifyDataProviderMessage field is DataProviderIdentifier, so every real client's identifier was silently discarded. FIXED 2026-09-04 (gopherstack parity sweep) -- also missing the 'You must remove the data provider from all migration projects before you can modify it' guard (api_op_ModifyDataProvider.go:16-17); DeleteDataProvider already had the analogous guard via migrationProjectUsesDataProviderLocked, Modify did not."}
  DeleteDataProvider: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-11 -- same DataProviderArn/DataProviderIdentifier bug as ModifyDataProvider"}
  CreateEventSubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-07-31 -- the response wire shape (eventSubscriptionJSON) used SubscriptionName and EventCategories, which are CreateEventSubscriptionMessage (request) field names; the real EventSubscription response type uses CustSubscriptionId and EventCategoriesList instead. A real SDK client deserializing the response got an empty subscription identifier and empty categories. Request-side field names (SubscriptionName/EventCategories on the input) were already correct and left unchanged -- the asymmetry between request and response field names is genuine AWS behavior, not a bug"}
  DescribeEventSubscriptions: {wire: ok, errors: ok, state: ok, persist: ok, note: "same CustSubscriptionId/EventCategoriesList fix as CreateEventSubscription (2026-07-31). FIXED 2026-08-12 (gopherstack-o53q) -- real input also carries Filters []types.Filter (event-subscription-arn/event-subscription-id); EventSubscription has no distinct ARN in this emulation, so both filter names resolve against SubscriptionName, the only identifier that exists."}
  ModifyEventSubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "same CustSubscriptionId/EventCategoriesList fix as CreateEventSubscription (2026-07-31)"}
  DeleteEventSubscription: {wire: ok, errors: ok, state: ok, persist: ok, note: "same CustSubscriptionId/EventCategoriesList fix as CreateEventSubscription (2026-07-31)"}
  CreateInstanceProfile: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeInstanceProfiles: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (gopherstack-21my) -- Filters []types.Filter (documented: 'instance-profile-identifier', the only valid filter name, api_op_DescribeInstanceProfiles.go) was declared but never read. Now applied against InstanceProfileName/InstanceProfileArn. See TestDescribeInstanceProfilesFilter (list_filter_params_test.go), real SDK client round trip."}
  ModifyInstanceProfile: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-11 -- request field was named InstanceProfileArn; the real ModifyInstanceProfileMessage field is InstanceProfileIdentifier, so every real client's identifier was silently discarded. FIXED 2026-09-04 (gopherstack parity sweep) -- also missing the 'All migration projects associated with the instance profile must be deleted or modified before you can modify the instance profile' guard (api_op_ModifyInstanceProfile.go:16-17); DeleteInstanceProfile already had the analogous guard via migrationProjectUsesInstanceProfileLocked, Modify did not."}
  DeleteInstanceProfile: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-11 -- same InstanceProfileArn/InstanceProfileIdentifier bug as ModifyInstanceProfile"}
  CreateMigrationProject: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED this pass (gopherstack-u90v) -- request previously dropped InstanceProfileIdentifier, SourceDataProviderDescriptors and TargetDataProviderDescriptors, all required (databasemigrationservice@v1.66.4 api_op_CreateMigrationProject.go:39-52), and the response echoed a fabricated MigrationProjectIdentifier field the real MigrationProject type (types.go:2044-2088) doesn't have. Now requires all three, resolves InstanceProfileIdentifier against the InstanceProfile store and each descriptor's DataProviderIdentifier against the DataProvider store (ResourceNotFoundFault if unresolved -- CreateMigrationProject's own deserializeOpError switch has no ValidationException case, so absence is rejected via this handler's existing ErrValidation->ValidationException mapping, which still round-trips as a generic APIError through the real SDK client's default branch), and echoes InstanceProfileArn/InstanceProfileName/Source+TargetDataProviderDescriptors on the response the way real MigrationProject does."}
  DescribeMigrationProjects: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (gopherstack-21my) -- all 5 documented filter names (migration-project-identifier|instance-profile-identifier|data-provider-identifier|source-data-provider-identifier|target-data-provider-identifier, api_op_DescribeMigrationProjects.go) were declared on the request struct but never read at all. Now applied against MigrationProjectName/Arn, InstanceProfileName/Arn, and Source/TargetDataProviderDescriptors (matched by DataProviderName or DataProviderArn). See TestDescribeMigrationProjectsFilter (list_filter_params_test.go), real SDK client round trip."}
  ModifyMigrationProject: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-11 -- request field was named MigrationProjectArn; the real ModifyMigrationProjectMessage field is MigrationProjectIdentifier, so every real client's identifier was silently discarded"}
  DeleteMigrationProject: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-11 -- same MigrationProjectArn/MigrationProjectIdentifier bug as ModifyMigrationProject"}
  ImportCertificate: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED 2026-07-31 -- the backend stored CertificatePem on Import but the response wire shape (certificateJSON) never returned it, on Import or Describe, even though the real Certificate type carries CertificatePem. Now returned on both"}
  DescribeCertificates: {wire: ok, errors: ok, state: ok, persist: ok, note: "same CertificatePem fix as ImportCertificate (2026-07-31). FIXED 2026-08-12 (gopherstack-o53q) -- real DescribeCertificatesInput carries Filters []types.Filter (certificate-arn/certificate-id), entirely absent from the request struct; a client's filter was silently dropped. Now narrows the returned list; proven with a multi-certificate test."}
  DeleteCertificate: {wire: ok, errors: ok, state: ok, persist: ok, note: "same CertificatePem fix as ImportCertificate (2026-07-31) -- certToJSON is shared by all three certificate ops"}
  DescribeAccountAttributes: {wire: ok, errors: ok, state: ok, persist: n/a, note: "quota usage computed live from real counts"}
  DescribeEvents: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "events recorded on Endpoint/ReplicationTask create/delete/start/stop, not persisted across restarts -- low value, matches many other services' event-log conventions. FIXED 2026-08-12 (gopherstack-o53q) -- real input also carries Filters []types.Filter; per the SDK doc 'the only valid filter is replication-instance-id', which is now applied against Event.SourceIdentifier and narrows the returned list. FIXED 2026-08-21 (gopherstack-g479) -- Event.Date was a hand-built map[string]any value assigning a formatted RFC3339 string; real Event.Date deserializes from a json.Number via ParseEpochSeconds (aws-sdk-go-v2/service/databasemigrationservice@v1.66.4's deserializers.go, awsAwsjson11_deserializeDocumentEvent). Failed with 'expected TStamp to be a JSON Number, got string instead' pre-fix; Event.Date is now time.Time internally, converted at the wire boundary. Found via a new go/types-based map-literal kind scanner (map[string]any{} literals had zero automated coverage before this pass). FIXED 2026-08-29 (gopherstack-21my) -- SourceIdentifier/SourceType/StartTime/EndTime/EventCategories are separate TOP-LEVEL DescribeEventsInput members, distinct from Filters (which the 2026-08-12 fix already covered) -- the handler's input struct declared none of them, so a real client's values could never be read however the handler was written (class 2, never plumbed). Now all five are decoded and applied: SourceIdentifier/SourceType as equality filters, StartTime/EndTime as an inclusive window against Event.Date, EventCategories as a set-intersection against Event.EventCategories. See TestDescribeEventsFilter (list_filter_params_test.go), real SDK client round trip, including a StartTime-in-the-future case asserting an empty result."}
  DescribeOrderableReplicationInstances: {wire: ok, errors: ok, state: n/a, note: "static reference catalog, matches real AWS class list. FIXED 2026-08-20 -- ReleaseStatus was hardcoded to the fabricated value \"GA\"; the real types.ReleaseStatusValues enum only has \"beta\"/\"prod\" (types/enums.go:628-634). Now \"prod\" (these are stable, non-beta instance classes)."}
  DescribeEngineVersions: {wire: ok, errors: ok, state: n/a, note: "static reference catalog"}
  DescribeEndpointTypes: {wire: ok, errors: ok, state: n/a, note: "static reference catalog"}
  DescribeEventCategories: {wire: ok, errors: ok, state: n/a, note: "static reference catalog. FIXED 2026-08-12 (gopherstack-o53q) -- real input also carries Filters []types.Filter alongside the existing SourceType field; a source-type filter value now falls back into the same lookup as the top-level SourceType field."}
  DescribeMetadataModel: {wire: ok, errors: ok, state: n/a, note: "FIXED this pass (gap #1) -- was an always-empty {} that only checked MigrationProjectIdentifier. Now requires MigrationProjectIdentifier/Origin/SelectionRules (all three are 'This member is required' on the real input) and returns the real {Definition, MetadataModelName, MetadataModelType, TargetMetadataModels} shape. Definition/MetadataModelName/MetadataModelType stay empty -- no schema-conversion engine exists to produce them, and the SDK doc explicitly says Definition 'might not be populated for some metadata models', so an empty-but-correctly-shaped response is not a stub (rule 4)."}
  DescribeMetadataModelAssessments: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- wire content used invented field names (MigrationProjectIdentifier/SelectionRules) instead of the real SchemaConversionRequest shape (RequestIdentifier/MigrationProjectArn/Status); now correct. MigrationProjectIdentifier is now required, matching the real input. FIXED 2026-08-12 (gopherstack-o53q) -- real input also carries Filters []types.Filter (request-id/status), absent from the request struct; a client's filter was silently dropped. listMetadataModelRequests now applies request-id/status filtering, shared by all six DescribeMetadataModel* list ops below."}
  DescribeMetadataModelConversions: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as DescribeMetadataModelAssessments, including the 2026-08-12 Filters fix"}
  DescribeMetadataModelCreations: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as DescribeMetadataModelAssessments, including the 2026-08-12 Filters fix"}
  DescribeMetadataModelExportsAsScript: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as DescribeMetadataModelAssessments, including the 2026-08-12 Filters fix"}
  DescribeMetadataModelExportsToTarget: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as DescribeMetadataModelAssessments, including the 2026-08-12 Filters fix"}
  DescribeMetadataModelImports: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as DescribeMetadataModelAssessments, including the 2026-08-12 Filters fix"}
  DescribeMetadataModelChildren: {wire: ok, errors: ok, state: n/a, note: "FIXED this pass -- response field was named 'Items' with the wrong (request) shape; real field is MetadataModelChildren, a list of MetadataModelReference{MetadataModelName,SelectionRules}. Now requires MigrationProjectIdentifier/Origin/SelectionRules like DescribeMetadataModel. Always empty -- no child-model producer exists (there is no StartMetadataModelChildren op in the real API either; children only ever arise from a completed schema conversion)."}
  CancelMetadataModelConversion: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- output was a flat {RequestIdentifier}; real shape is {Request: SchemaConversionRequest}. Cancelling an untracked request still succeeds (real AWS's Cancel ops are fire-and-forget), echoing a minimal SchemaConversionRequest"}
  CancelMetadataModelCreation: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix as CancelMetadataModelConversion"}
  StartMetadataModelCreation: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-4ggy: Properties (types.MetadataModelProperties, a union whose only member is StatementProperties{Definition}) was dropped entirely -- request had only MigrationProjectIdentifier/MetadataModelName/SelectionRules. Now required and validated (top-level Properties, and Definition within StatementProperties when present), matching validateOpStartMetadataModelCreationInput. StartMetadataModelRequest's shared helper (also used by 6 sibling Start* ops) gained a propertiesDefinition parameter, empty for every non-creation caller. Not surfaced on any Describe response -- types.SchemaConversionRequest has no matching field, same as SelectionRules -- tracked on MetadataModelRequest.PropertiesDefinition for internal state fidelity only, consistent with the existing SelectionRules convention."}
  DescribeConversionConfiguration: {wire: ok, errors: ok, state: n/a, note: "pre-existing, matches the real {ConversionConfiguration, MigrationProjectIdentifier} shape"}
  ModifyConversionConfiguration: {wire: ok, errors: ok, state: n/a, note: "pre-existing, matches the real shape; echoes the caller's ConversionConfiguration (no real schema-conversion config store)"}
  DescribeExtensionPackAssociations: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- was hardcoded to always return an empty list, disconnected from StartExtensionPackAssociation. Now reads real extension-pack request rows. FIXED 2026-08-12 (gopherstack-o53q) -- shares the same Filters (request-id/status) fix as DescribeMetadataModelAssessments via listMetadataModelRequests."}
  StartExtensionPackAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- was a disguised no-op returning a random UUID with no backend write, so the request was invisible to DescribeExtensionPackAssociations. Now records a real request row and requires MigrationProjectIdentifier"}
  GetTargetSelectionRules: {wire: ok, errors: ok, state: n/a, note: "FIXED this pass -- output was an invented {Rules: []} list shape; real output is a single TargetSelectionRules string. Now requires MigrationProjectIdentifier/SelectionRules and echoes the source rules as a best-effort identity mapping (no real schema-conversion engine to compute a genuine target counterpart)"}
  ExportMetadataModelAssessment: {wire: ok, errors: ok, state: n/a, note: "FIXED this pass -- PdfReport/CsvReport were missing the ObjectURL field (real ExportMetadataModelAssessmentResultEntry has both ObjectURL and S3ObjectKey); now both are legitimately omitted (optional pointer fields, no real S3 integration exists) instead of one being a fabricated empty string. MigrationProjectIdentifier/SelectionRules are now required"}
  DescribeApplicableIndividualAssessments: {wire: ok, errors: ok, state: n/a, note: "FIXED this pass -- was hardcoded to always return an empty list; now returns a representative static catalog of individual assessment names (legitimate reference-data emulation per rule 4 -- the SDK does not model these names as an enum, they're plain strings)"}
  DescribeReplicationTaskIndividualAssessments: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass (deferred item #3) -- was hardcoded to always return an empty list regardless of any assessment run. Now populated by StartReplicationTaskAssessmentRun and filterable by replication-task-assessment-run-arn/replication-task-arn/status"}
  DescribeReplicationTaskAssessmentResults: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass (deferred item #3) -- was hardcoded to always return an empty list. Now: with ReplicationTaskArn, returns exactly one result for that task's latest run (ignoring Marker/MaxRecords, matching the SDK doc); without it, lists the latest result per assessed task"}
  StartReplicationTaskAssessmentRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- AssessmentRunName/ReplicationTaskArn/ResultLocationBucket/ServiceAccessRoleArn were all documented as required but never validated; IncludeOnly/Exclude were accepted but ignored. Now validates all four required fields, rejects setting both IncludeOnly and Exclude, and synchronously completes the run (Status passed) with real IndividualAssessment rows and ResultStatistic counts -- no goroutines/tickers, matching the service's leak-free convention"}
  CancelReplicationTaskAssessmentRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- output was a hand-rolled {ReplicationTaskAssessmentRunArn, Status} map; now the real {ReplicationTaskAssessmentRun: ReplicationTaskAssessmentRun} shape"}
  DeleteReplicationTaskAssessmentRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "same wire-shape fix as CancelReplicationTaskAssessmentRun"}
  DescribeReplicationTaskAssessmentRuns: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass -- output items were a hand-rolled 4-field map; now the real ReplicationTaskAssessmentRun shape (AssessmentProgress, ResultStatistic, ResultLocationBucket/Folder, ServiceAccessRoleArn, creation-date epoch, IsLatestTaskAssessmentRun). Filters extended to replication-task-assessment-run-arn/replication-instance-arn/status (previously only replication-task-arn)"}
  StartReplicationTaskAssessment: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED 2026-09-06 (gopherstack-cq25) -- errors: ok was wrong; the op previously enforced neither documented precondition. Now enforces task-must-be-stopped (InvalidResourceStateFault). See dated entry below for the ready-vs-stopped verdict and why the connection-test precondition is now also enforced (TestConnection results are modeled, unlike gopherstack-cq25's initial assumption)."}
families:
  fleet-advisor: {status: ok, note: "CreateFleetAdvisorCollector/DeleteFleetAdvisorCollector/DescribeFleetAdvisorCollectors/DescribeFleetAdvisorDatabases/DeleteFleetAdvisorDatabases all mutate/read real backend state and persist. DescribeFleetAdvisorLsaAnalysis/SchemaObjectSummary/Schemas field-diffed this pass (deferred item #2, now resolved): response field names (Analysis/FleetAdvisorSchemaObjects/FleetAdvisorSchemas + NextToken) match types.go exactly; the lists are legitimately always-empty since no LSA-analysis or schema-conversion engine exists to populate them (rule 4). AWS ended support for Fleet Advisor entirely on 2026-05-20 (already past as of this audit) -- low future value. FIXED 2026-08-20 -- DescribeFleetAdvisorCollectors's request struct used a fabricated Marker field; the real DescribeFleetAdvisorCollectorsInput's pagination token field is NextToken (api_op_DescribeFleetAdvisorCollectors.go), like its 4 siblings in this family, not the Marker/MaxRecords convention most other DMS Describe ops use. The response struct was also missing NextToken entirely. Both now match. Low severity: no pagination logic exists for this op (list is always returned in full, same as its siblings), and nothing read the old Marker field, so this was a pure wire-shape correction with no behavioral difference to prove via a discriminating test -- documented here rather than backed by a dedicated test for that reason. FIXED 2026-08-29 (error-path sweep) -- DeleteFleetAdvisorCollector's own deserializeOpError models CollectorNotFoundFault, not the service-wide ResourceNotFoundFault every other DMS delete op raises (deserializers.go:2875-2913, confirmed against all 119 ops' error switches); the backend was returning ErrNotFound (ResourceNotFoundFault) for a missing collector, which a real client's errors.As(&types.CollectorNotFoundFault{}) would never match. Now returns the dedicated ErrCollectorNotFound sentinel. An existing test (TestDeleteFleetAdvisorCollector_NotFound) asserted the old wrong code as correct via a raw __type string check; converted to drive the real SDK client and assert the typed exception via errors.As. FIXED 2026-08-29 (gopherstack-21my, parameter-honoring sweep) -- DescribeFleetAdvisorCollectors and DescribeFleetAdvisorDatabases both had real backing state (a real filterable list, unlike the legitimately-always-empty LsaAnalysis/SchemaObjectSummary/Schemas siblings) but their handlers took `_ *describeFleetAdvisorXInput` -- the request struct was never even bound to a named parameter, so Filters/MaxRecords/NextToken were unreachable however the handler was written (class 1/2). Collectors now applies collector-name/collector-referenced-id (both documented, api_op_DescribeFleetAdvisorCollectors.go); Databases applies all 5 documented names (database-id/database-name/database-engine/database-ip-address/server-ip-address(same field, one IP modeled)/collector-name, api_op_DescribeFleetAdvisorDatabases.go, the last via a join against the collector list). Both now paginate through NextToken/MaxRecords via dmsPaginate. ADJACENT BUG found and fixed while building the real-SDK-client test for this: fleetAdvisorCollectorJSON.CollectorHealthCheck was wired as a bare string (\"HEALTHY\", not even a real CollectorStatus enum value -- the real enum only has UNREGISTERED/ACTIVE, types/enums.go:120-121); the real wire shape (types.CollectorHealthCheck, types/types.go:108) is a nested object with CollectorStatus + 3 access booleans, so a real SDK client's DescribeFleetAdvisorCollectors call failed deserialization outright (\"unexpected JSON type HEALTHY\") rather than merely showing a wrong value. Now a proper nested object with CollectorStatus:\"ACTIVE\" and all 3 access booleans true (this backend never models a collector that fails its S3/role checks). See TestDescribeFleetAdvisorCollectorsFilter/TestDescribeFleetAdvisorDatabasesFilter (list_filter_params_test.go), real SDK client round trip."}
  metadata-model: {status: ok, note: "FIXED this pass -- DescribeMetadataModel/DescribeMetadataModelChildren/the six Describe*Requests list ops/Cancel*/GetTargetSelectionRules/ExportMetadataModelAssessment/StartExtensionPackAssociation were all field-diffed against types.go and api_op_*.go this pass (deferred item #1, now resolved) and every wire-shape bug found was fixed -- see the per-op notes above. Definition/MetadataModelName/MetadataModelType/schema-object contents stay legitimately empty; no schema-conversion SQL-generation engine exists, matching the SDK doc's 'might not be populated' language."}
  static-reference-data: {status: ok, note: "DescribeOrderableReplicationInstances/DescribeEngineVersions/DescribeEndpointTypes/DescribeEventCategories/DescribeApplicableIndividualAssessments return realistic static catalogs; legitimate for AWS reference-data ops (rule 4: an op with no mutable backend state behind it is not a stub). DescribeEndpointTypes FIXED this pass -- EndpointType values were hardcoded uppercase SOURCE/TARGET, but the real enum is lowercase source/target. FIXED 2026-08-29 (gopherstack-21my) -- DescribeEndpointTypesInput documents Filters (engine-name|endpoint-type, api_op_DescribeEndpointTypes.go); a static catalog is not by itself grounds to skip honoring a documented filter (unlike medialive's 3-entry ListOfferings precedent, this catalog has 26 entries and the filter is trivial to apply), so Filters -- previously declared but never read -- now narrows the returned support matrix. See TestDescribeEndpointTypesFilter (list_filter_params_test.go), real SDK client round trip."}
  assessment-runs: {status: ok, note: "FIXED this pass (deferred item #3, now resolved) -- StartReplicationTaskAssessmentRun now validates its four required fields and IncludeOnly/Exclude mutual exclusion, then synchronously runs a real (bounded, static-catalog-backed) set of IndividualAssessment checks, all passing. DescribeReplicationTaskIndividualAssessments and DescribeReplicationTaskAssessmentResults are now backed by that real state instead of hardcoded empty lists. Cancel/Delete/DescribeReplicationTaskAssessmentRuns now return the full real ReplicationTaskAssessmentRun wire shape instead of a hand-rolled 4-field map."}
gaps: []
deferred: []
leaks: {status: clean, note: "no goroutines, janitors, or timers in this service; all state lives in store.Table/store.Index behind the single lockmetrics.RWMutex. leak_test.go / isolation_test.go pre-existing and passing. Confirmed again this pass -- no new goroutines/tickers/channels were introduced by the assessment-run rework (StartReplicationTaskAssessmentRun completes synchronously)."}
---

## Notes

- **2026-08-20 wrapper-key / nested-shape sweep**: this service's directory
  name (`services/dms`) does NOT match its SDK module name
  (`aws-sdk-go-v2/service/databasemigrationservice`, no `service/dms` module
  exists). A prior pass of an automated ranking tool (`cmd/opcensus`)
  resolved the SDK module from the directory name, found nothing, and
  reported this service as having 0 List/Describe/Get operations -- which
  put it at the bottom of a ranked sweep queue as if it were a tiny,
  low-value, unaudited service. In reality the pinned SDK
  (`databasemigrationservice@v1.66.4`) declares 119 operations
  (`ls api_op_*.go` in the module cache), all 119 of which
  `handler.go`'s op-name const block implements, and this file already
  documents multiple thorough prior audit passes (2026-07-12 through
  2026-08-12) covering the large majority of that surface field-by-field.
  This pass's premise -- "first read of this surface, 23 operations" --
  did not hold at the pinned version; noted here so a future reader does
  not repeat either the tool's directory-name mistake or the op-count
  assumption.

  Given the extensive prior coverage, this pass targeted the two areas the
  brief called out as top priority and the most recently field-diffed
  PARITY notes had NOT covered: the `Endpoint` envelope (previously
  verified for `Password`, the 19 engine-specific settings blocks, and
  `EndpointType`/`EngineName` enums, but never for its other top-level
  scalar members) and the `ReplicationInstance` envelope (previously
  verified only for the `InstanceCreateTime` epoch-timestamp field and the
  `ReplicationSubnetGroup`/`VpcSecurityGroups` nesting, per
  `handler_replication_instances.go`, never for its remaining top-level
  scalar members). Both were diffed field-by-field, optional included,
  types checked, directly against
  `aws-sdk-go-v2/service/databasemigrationservice@v1.66.4/types/types.go`
  and each op's own `api_op_*.go` -- not assumed from prior notes.

  **Protocol verified**: `awsAwsjson11_*` serializer prefix
  (`serializers.go`), `X-Amz-Target: AmazonDMSv20160101.<Action>` header
  (confirmed directly, e.g. `serializers.go:61`
  `"AmazonDMSv20160101.AddTagsToResource"`) -- JSON-RPC, matching the
  protocol this file already documented; no cnhp-style surprise (every op
  serializes/deserializes a real flat JSON body, no raw-body ops like
  polly's AudioStream).

  **Pagination convention verified across all 119 ops** (per the brief's
  explicit warning to check, since siblings elsewhere in this repo mix
  conventions): the large majority of `Describe*` ops use `Marker`/
  `MaxRecords` (70 grep hits across `api_op_Describe*.go`), but 7 ops
  genuinely use `NextToken`/`MaxRecords` instead --
  `DescribeFleetAdvisorCollectors`, `DescribeFleetAdvisorDatabases`,
  `DescribeFleetAdvisorLsaAnalysis`, `DescribeFleetAdvisorSchemaObjectSummary`,
  `DescribeFleetAdvisorSchemas`, `DescribeRecommendationLimitations`,
  `DescribeRecommendations` (all of Fleet Advisor + Recommendations).
  gopherstack's `describeFleetAdvisorDatabasesInput`/
  `describeFleetAdvisorLsaAnalysisInput`/
  `describeFleetAdvisorSchemaObjectSummaryInput`/
  `describeFleetAdvisorSchemasInput`/`describeRecommendationLimitationsInput`/
  `describeRecommendationsInput` all already correctly used `NextToken`.
  Only `describeFleetAdvisorCollectorsInput` used the wrong (Marker/
  MaxRecords) convention -- fixed this pass, see the `fleet-advisor` family
  note above. Every other Describe op checked (the 70 Marker/MaxRecords
  ops) already used the right convention.

  **Bugs found and fixed** (3, all field-diffed against the pinned SDK
  types, each proven by hand-revert -- copying the pre-fix file back over
  the working tree via `cp` and confirming the build/test breaks, then
  restoring the fix):

  1. **`Endpoint` envelope, 6 missing top-level fields** (bug class a --
     members missing from the wire on both Create/Modify request AND
     Describe/Create/Modify response): `CertificateArn`,
     `ExtraConnectionAttributes`, `KmsKeyId`, `ServiceAccessRoleArn`,
     `SslMode`, `ExternalTableDefinition` (all real `CreateEndpointInput`
     members, `api_op_CreateEndpoint.go`). These are distinct from the
     already-documented 19-engine-settings-block gap (2026-08-10,
     gopherstack-z79q) -- simple top-level scalars, not part of the
     ~300-field per-engine problem, and cheap to model faithfully. Fixed:
     added `EndpointConnectionSettings` (`endpoints.go`), threaded through
     `CreateEndpoint`/`ModifyEndpoint`, validated `SslMode` against the real
     `DmsSslModeValue` enum (`none`/`require`/`verify-ca`/`verify-full`,
     defaulting to `none` matching the documented default), and echoed on
     `endpointJSON`. `KmsKeyId` is deliberately create-only (real
     `ModifyEndpointInput` has no `KmsKeyId` member). Proof:
     `TestEndpointConnectionSettings_SDKRoundTrip`,
     `TestEndpointSslMode_DefaultsToNone`,
     `TestEndpointSslMode_InvalidRejected`
     (`handler_endpoints_settings_test.go`). Hand-revert symptom: reverting
     `handler_endpoints.go`/`endpoints.go`/`models.go` to the pre-fix
     version breaks the build (`isolation_test.go`/`persistence_test.go`
     call sites no longer compile against the old `CreateEndpoint`/
     `ModifyEndpoint` signatures) -- a stronger proof than a value-level
     assertion failure.
  2. **`ReplicationInstance` envelope, 4 missing top-level fields** (same
     bug class): `KmsKeyId`, `DnsNameServers`, `NetworkType`,
     `PreferredMaintenanceWindow` (all real `CreateReplicationInstanceInput`
     members, `api_op_CreateReplicationInstance.go`; `NetworkType`/
     `PreferredMaintenanceWindow` are also real `ModifyReplicationInstanceInput`
     members). Fixed: added `ReplicationInstanceSettings`
     (`replication_instances.go`), threaded through
     `CreateReplicationInstance`/`ModifyReplicationInstance`, echoed on
     `replicationInstanceJSON`. `KmsKeyId`/`DnsNameServers` are deliberately
     create-only (real `ModifyReplicationInstanceInput` has neither member).
     Proof: `TestReplicationInstanceSettings_SDKRoundTrip`
     (`handler_endpoints_settings_test.go`). Hand-revert symptom: same as
     above -- reverting breaks the build via the test call sites.
  3. **`DescribeOrderableReplicationInstances`, fabricated enum value**
     (bug class e -- right key, wrong/invented value, the bedrockruntime
     `"BLOCKED"` pattern): `ReleaseStatus` was hardcoded to `"GA"` for every
     entry; the real `types.ReleaseStatusValues` enum
     (`types/enums.go:628-634`) only has `"beta"`/`"prod"` -- `"GA"` is not
     a member of this enum at all. Fixed: now `"prod"` (these are stable,
     non-beta instance classes, matching every entry in
     `dmsOrderableInstanceList()`). Proof:
     `TestDescribeOrderableReplicationInstances_ReleaseStatus`
     (asserts every returned `ReleaseStatus` is a member of
     `{types.ReleaseStatusValuesBeta, types.ReleaseStatusValuesProd}`).
     Hand-revert symptom: reverting to `"GA"` fails the assertion directly
     (`does not contain "GA"`), confirmed by actually reverting and
     re-running before restoring.

  Also fixed, lower severity (bug class d -- per-op pagination-token key):
  4. **`DescribeFleetAdvisorCollectors`, wrong pagination field name**:
     request struct declared a fabricated `Marker` field where the real
     `DescribeFleetAdvisorCollectorsInput` has `NextToken`
     (`api_op_DescribeFleetAdvisorCollectors.go`); response struct was
     missing `NextToken` entirely. Fixed (see `fleet-advisor` family note
     above). **Severity note**: grepped for readers of the old `Marker`
     field before fixing -- the handler ignores its input parameter
     entirely (`_ *describeFleetAdvisorCollectorsInput`), so nothing read
     it; this op implements no pagination logic at all (always returns the
     full list, like its 4 correctly-shaped siblings), so there is no
     behavioral difference a round-trip test could discriminate on. Fixed
     for wire-shape correctness and consistency with siblings, not backed
     by a dedicated proof test for that reason -- disclosed rather than
     claiming false rigor.

  **Coverage disclosed, not fixed** (out of scope for this pass, each
  individually verified as a genuine gap rather than assumed):
  - `ReplicationInstance.PendingModifiedValues`
    (`*ReplicationPendingModifiedValues`), `FreeUntil` (`*time.Time`),
    `SecondaryAvailabilityZone` (`*string`), and
    `KerberosAuthenticationSettings` (`*KerberosAuthenticationSettings`,
    also on `CreateReplicationInstanceInput`/`ModifyReplicationInstanceInput`)
    are real `ReplicationInstance` members with no equivalent anywhere in
    this emulation. `PendingModifiedValues` specifically requires an
    `ApplyImmediately=false` deferred-modify state machine this service
    does not have (every `ModifyReplicationInstance` call applies
    immediately); a fabricated non-nil value would be worse than the
    honest absence (rule 4). Not fixed this pass -- flagged for a future
    pass if deferred-apply semantics are ever added.
  - The `Endpoint`/`ReplicationInstance` families were the only two
    envelopes field-diffed this pass, per the brief's explicit
    prioritization ("the Endpoint envelope... first, then
    ReplicationInstance/ReplicationTask, then the rest"). `ReplicationTask`
    (with `ReplicationTaskStats`/`RecoveryCheckpoint`), `Certificate`,
    `Connection`, `EventSubscription`, `TableStatistics`,
    `RefreshSchemasStatus`, `OrderableReplicationInstance` (beyond the
    `ReleaseStatus` fix above), `AccountQuota`, and `SupportedEndpointType`
    were NOT re-diffed this pass -- this file's existing notes (2026-07-31
    and 2026-08-12 passes) already cover `Certificate`/`EventSubscription`/
    `ReplicationSubnetGroup`/`DescribeConnections` field-level fixes, and
    per the campaign's under-claiming guidance, a tree already covered by a
    prior pass was not re-read from scratch this session absent a specific
    reason to distrust those notes. The 24 enums the brief listed
    (`AuthMechanismValue`, `AuthTypeValue`, etc.) were spot-checked, not
    exhaustively re-verified: `MigrationTypeValue`,
    `StartReplicationMigrationTypeValue`, `ReplicationEndpointTypeValue`
    (both directions), `DmsSslModeValue` (both directions, this pass), and
    `ReleaseStatusValues` (this pass) were confirmed correct/fixed; the
    remaining ~19 enums on the brief's list were not individually
    re-checked this pass and are not claimed clean here.

- **2026-07-31 field-level wire-shape sweep**: the 2026-07-23 audit's "119
  operations matching the SDK exactly, no phantoms" claim about the *op
  list* was accurate and remains true, but 5 of its "wire: ok" marks were
  wrong at the *field* level -- caught by an independent dashboard sweep,
  each re-verified directly against
  `aws-sdk-go-v2/service/databasemigrationservice/types/types.go` before
  fixing (not assumed from the ticket description). All 5 are fixed this
  pass, each with a test that fails against the pre-fix code:
  1. `EventSubscription` response used `SubscriptionName`/`EventCategories`
     (the *request* field names) instead of `CustSubscriptionId`/
     `EventCategoriesList`. Request-side names were already correct and
     left alone -- AWS genuinely uses different names on request vs.
     response for this type.
  2. `ReplicationSubnetGroup` response emitted a `ReplicationSubnetGroupArn`
     field; the real type has no Arn field at all.
  3. `Certificate` response never returned `CertificatePem` on Import,
     Describe, or Delete, even though the backend stored it and the real
     type carries it.
  4. `CreateEndpoint`/`ModifyEndpoint` accepted `Password` in the request
     but never stored it (silently dropped); now stored on `Endpoint`
     internally and never put on the wire (matching real AWS, which never
     echoes credentials back). Engine-specific nested settings blocks
     (`MySQLSettings`, `S3Settings`, etc.) were left deliberately unmodeled
     at the time -- resolved 2026-08-10, see the note below.
  5. `DescribeConnections` never called `dmsPaginate` or set `Marker` on the
     response, unlike every other Describe op in this service.

- **2026-08-10 engine-specific endpoint settings (gopherstack-z79q)**:
  `CreateEndpointInput`/`ModifyEndpointInput` accept 19 heterogeneous
  engine-specific settings structs (`MySQLSettings`, `PostgreSQLSettings`,
  `S3Settings`, `OracleSettings`, `MongoDbSettings`, `KafkaSettings`,
  `KinesisSettings`, `RedshiftSettings`, `DynamoDbSettings`,
  `ElasticsearchSettings`, `NeptuneSettings`, `DocDbSettings`,
  `IBMDb2Settings`, `MicrosoftSQLServerSettings`, `SybaseSettings`,
  `DmsTransferSettings`, `GcpMySQLSettings`, `RedisSettings`,
  `TimestreamSettings`), field-counted directly against
  `aws-sdk-go-v2/service/databasemigrationservice/types/types.go`
  (`@v1.61.8`): ~301 fields total (2 to 44 fields per struct; `S3Settings`
  alone has 41, `OracleSettings` 44). Modeling all of them faithfully
  (validated types, stored, echoed on `DescribeEndpoints`, persisted) is not
  achievable in one pass, and per the issue's own instruction a partial
  subset is worse than the honest gap -- a client seeing some settings
  preserved would reasonably assume the rest are too. Instead of leaving the
  silent drop in place (the pre-existing behavior: `encoding/json` ignores
  unknown fields), the drop is now made visible: `engineSettingsFields` in
  `handler_endpoints.go` decodes all 19 fields, and `CreateEndpoint`/
  `ModifyEndpoint` reject the request with 400 `ValidationException` naming
  the field if any is set, rather than accepting and discarding it. This
  follows the same explicit-rejection-over-silent-drop precedent as
  sagemaker's `PipelineDefinitionS3Location` and cloudformation's
  unsupported `AccountFilterType` values. The `Password` fix from
  2026-07-31 is unaffected and unchanged.

- **2026-08-12 Filters-absent sweep (gopherstack-o53q)**: the gopherstack-7rq1
  wire-field audit flagged 14 candidate Describe ops missing the real
  optional `Filters []types.Filter` request member. All 14 were individually
  read against `aws-sdk-go-v2/service/databasemigrationservice@v1.66.4`
  (not assumed from the sibling pattern) and every one genuinely carries
  `Filters` on the real input -- no false positives in this cluster, unlike
  the 197-of-313 discard rate the parent audit found overall. Fixed via the
  service's pre-existing `filterEntry`/`extractFilterValue` convention (see
  `handler.go`, already used by `DescribeConnections`/`DescribeEndpoints`/
  `DescribeReplicationInstances`/etc.): `DescribeCertificates`
  (certificate-arn/certificate-id), `DescribeEventCategories` (source-type,
  merged with the existing top-level field), `DescribeEventSubscriptions`
  (event-subscription-arn/-id, both resolving to SubscriptionName since no
  distinct ARN exists), `DescribeEvents` (replication-instance-id, matching
  the SDK doc's "only valid filter"), `DescribeDataProviders`
  (data-provider-identifier), `DescribeDataMigrations`
  (data-migration-identifier), and the six schema-conversion Describe*
  ops plus `DescribeExtensionPackAssociations` (request-id/status, applied
  once in the shared `listMetadataModelRequests` helper). One op,
  `DescribeReplicationTableStatistics`, genuinely has no state to filter --
  `ReplicationTableStatistics` is always empty in this emulation (see its
  note above) -- so `Filters` is accepted for wire-shape parity only, with no
  filtering logic, documented rather than pretended. Every applied filter is
  covered by a table-driven test in `handler_filters_test.go`;
  `TestDescribeCertificatesFilterNarrows` proves genuine narrowing of a
  multi-item result set (not just field parsing) by importing two
  certificates and confirming the filtered response contains exactly one.

- **Wire protocol**: `application/x-amz-json-1.1` (awsjson1.1), target prefix
  `AmazonDMSv20160101.<Action>`. All request/response bodies are flat JSON
  objects (no XML), matching `service.WrapOp` handler conventions used
  throughout.

- **Persistence**: every resource collection is registered on `b.registry`
  (see `store_setup.go`), and `Handler.Snapshot`/`Handler.Restore` delegate
  straight to `InMemoryBackend.Snapshot`/`Restore`, which round-trip the
  entire registry via `SnapshotAll`/`RestoreAll` plus `reinitTagsLocked` for
  the backend-owned `Tags` field. This service did **not** have the
  "Handler doesn't expose Snapshot/Restore" bug class found elsewhere in this
  sweep -- persistence wiring was already correct going in.

- **DMS Serverless "Replication" vs "ReplicationConfig" are two distinct AWS
  resources** sharing one ARN: `ReplicationConfig` (from
  CreateReplicationConfig/DescribeReplicationConfigs/ModifyReplicationConfig)
  has no `Status` field on the wire; `Replication` (from
  StartReplication/StopReplication/DescribeReplications) is the runtime
  state and does have `Status`. gopherstack models this as one Go struct
  (`ReplicationConfig` with an added `Status`/`StartReplicationType` pair)
  since a config has at most one associated replication in this emulation --
  but the two JSON shapes (`replicationConfigJSON` vs `replicationJSON`) are
  kept separate so `Status` is never accidentally leaked onto the
  ReplicationConfig wire shape. Don't conflate them when re-auditing.

- **`DescribeReplications` lists every ReplicationConfig, even ones never
  started** (`Status: "created"`), mirroring observed real AWS CLI behavior
  where the Replication runtime resource exists implicitly the moment its
  config is created. Do not "fix" this to filter out never-started configs
  without re-verifying against real AWS -- this was a deliberate design
  choice this pass, not an oversight.

- **RebootReplicationInstance is correctly a state no-op.** Real AWS reboot
  causes "a momentary outage" but no persistent field changes once complete;
  gopherstack's synchronous emulation (return the instance unchanged) matches
  the post-reboot steady state. Don't flag this as a disguised no-op again
  without a concrete field AWS actually changes.

- **Void-envelope ops that are legitimately empty** (verified against
  `types.go`/`api_op_*.go` this pass, not just assumed): `AddTagsToResource`,
  `RemoveTagsFromResource`, `DeleteFleetAdvisorCollector`,
  `DeleteReplicationSubnetGroup`, `StartRecommendations`. Each of these
  really does call into real backend state before returning `{}` -- this was
  double-checked, not just grepped.

- **Fleet Advisor and Schema Conversion (metadata-model) op families are
  low future value**: the AWS SDK source (`api_op_StartRecommendations.go`
  et al.) carries an explicit end-of-support notice for Fleet Advisor dated
  2026-05-20, which has already passed as of this audit (2026-07-12). Future
  audit passes should deprioritize these families relative to the core
  ReplicationInstance/Endpoint/ReplicationTask/SubnetGroup/ReplicationConfig
  surface.

- **Epoch-seconds timestamp bug class (2026-07-23 pass)**: `ReplicationInstance`
  and `ReplicationTask` both track a `CreationTime time.Time` field
  internally (used correctly for persistence ordering) but neither
  `InstanceCreateTime` (real field name on `ReplicationInstance`) nor
  `ReplicationTaskCreationDate` (real field name on `ReplicationTask`) was
  ever put on the wire -- not wrong-format, just entirely absent. Fixed by
  adding both fields to `replicationInstanceJSON`/`replicationTaskJSON` as
  `float64` populated via `pkgs/awstime.Epoch`, matching awsjson1.1's
  unixTimestamp format. `Endpoint` has no timestamp field on the real wire
  shape, so it was correctly left alone.

- **`EndpointType` is lowercase, unlike most other DMS enum-ish fields**:
  `types.ReplicationEndpointTypeValue` is `"source"`/`"target"` (not
  `"SOURCE"`/`"TARGET"`). This is easy to get backwards since
  `types.OriginTypeValue` (used by the metadata-model family's `Origin`
  field) genuinely IS uppercase (`"SOURCE"`/`"TARGET"`). Don't "fix" one to
  match the other without re-checking `enums.go`.

- **DMS Serverless "ReloadReplicationTables" targets a *config*, not a
  *task***: `ReloadReplicationTablesInput.ReplicationConfigArn` is the real
  field name (a previous implementation used `ReplicationTaskArn`, silently
  discarding the client's ARN and never validating anything). Don't
  conflate this with `ReloadTablesInput.ReplicationTaskArn`, which is the
  correct field name for the *non*-serverless `ReloadTables` op.

- **2026-08-22 (gopherstack-zquj, keycheck sweep)**: `DescribeFleetAdvisorDatabases`
  wrote each database's engine info under a flat `"EngineName"` key and its
  discovering collector under a flat `"CollectorReferencedId"` key. The real
  `DatabaseResponse` (`aws-sdk-go-v2/service/databasemigrationservice@v1.66.4`
  `types/types.go:332`) has neither member: engine info is nested under
  `SoftwareDetails.Engine` (`types.go:301`,
  `awsAwsjson11_deserializeDocumentDatabaseInstanceSoftwareDetailsResponse`,
  case `"Engine"`), and the collector ID is nested under
  `Collectors[].CollectorReferencedId` (`types.go:178`,
  `awsAwsjson11_deserializeDocumentCollectorShortInfoResponse`, case
  `"CollectorReferencedId"`). An exact-case real client dropped both values
  silently on every `DescribeFleetAdvisorDatabases` call, and
  `TestFleetAdvisorDatabases` ratified the wrong flat key by asserting
  `db0["EngineName"]` directly. Fixed by nesting both under their real
  parent objects in `handleDescribeFleetAdvisorDatabases`
  (`handler_fleet_advisor.go`); the ratifying test now asserts
  `db0["SoftwareDetails"]["Engine"]`. Proof:
  `TestDescribeFleetAdvisorDatabases_EngineDecodesNested`
  (`wire_maplit_fixes_test.go`) drives the real
  `aws-sdk-go-v2/service/databasemigrationservice` client end-to-end and
  asserts both `SoftwareDetails.Engine` and `Collectors[0].CollectorReferencedId`
  decode non-empty; confirmed failing against the pre-fix flat keys via
  hand-revert (`git show HEAD:services/dms/handler_fleet_advisor.go`,
  restored, md5sum-verified byte-identical after re-fixing).

- **Premigration assessment runs complete synchronously in this emulation**:
  real AWS runs `StartReplicationTaskAssessmentRun` asynchronously against
  actual source/target connectivity; gopherstack has neither, so (matching
  the service's leak-free, goroutine-free convention -- see `leak_test.go`)
  the run transitions straight to `Status: "passed"` with every selected
  `IndividualAssessment` also `"passed"`. `defaultApplicableIndividualAssessments()`
  in `assessment_runs.go` is a representative static catalog, not derived
  from AWS docs verbatim -- the SDK does not model these names as an enum
  (`IndividualAssessmentName` is a plain `*string`), so any reasonable
  catalog is wire-accurate; only the *shape* (a flat list of strings) is a
  real constraint.

- **2026-08-23 (gopherstack-v4a4) response struct-tag sweep**: `keycheck`'s
  struct-tag extension flagged `CreateDataMigration`/`DescribeDataMigrations`/
  `ModifyDataMigration`/`DeleteDataMigration`/`StartDataMigration`/
  `StopDataMigration` writing `NumberOfJobs`/`EnableCloudwatchLogs` outside
  the real reachable shape. Confirmed against
  `databasemigrationservice@v1.66.4/deserializers.go`: the real
  `awsAwsjson11_deserializeDocumentDataMigration` case list (line 16304) has
  no top-level `NumberOfJobs` or `EnableCloudwatchLogs` case at all -- both
  live nested under `DataMigrationSettings`
  (`awsAwsjson11_deserializeDocumentDataMigrationSettings`, line 16546),
  which switches on `NumberOfJobs` (same name) and `CloudwatchLogsEnabled`
  (renamed from the request-side `EnableCloudwatchLogs`). The request side
  (`CreateDataMigrationInput`) genuinely is flat and named
  `EnableCloudwatchLogs` (`serializers.go:10061`-`10087`) -- real AWS's own
  API is asymmetric here, request flat, response nested-and-renamed -- so
  only the response-side `dataMigrationJSON` struct
  (`handler_data_migrations.go`) was wrong. Every real client's
  `DataMigration.DataMigrationSettings` decoded nil on all six ops, which
  share the one `dmToJSON` builder. Fixed by adding a
  `dataMigrationSettingsJSON` nested struct and updating `dmToJSON`; the
  domain `DataMigration` struct and its persistence snapshot were untouched
  (only the wire translation layer changed, no golden refresh needed).
  Proof: `TestCreateDataMigration_SettingsNestUnderDataMigrationSettings_RealClient`
  (`wire_field_fixes_test.go`) drives the real
  `aws-sdk-go-v2/service/databasemigrationservice` client end-to-end;
  confirmed failing against the pre-fix flat keys via hand-revert
  (`git show HEAD:services/dms/handler_data_migrations.go`, restored,
  md5sum-verified byte-identical after re-fixing). No existing test
  asserted the wrong response shape, so nothing needed correcting.

- **2026-08-29 write-only-state sweep**: this file already documented 7+
  thorough prior passes, including a 2026-08-20 pass that field-diffed
  `ReplicationInstance`'s top-level scalar members specifically. Per this
  campaign's standing rule that a prior audit does not guarantee a service is
  clean, this pass re-applied the primary write-only-state method (enumerate
  what the backend persists, ask what op reads it back, flag anything
  accepted-and-never-stored or stored-and-never-readable) directly against
  `CreateReplicationInstance`/`ModifyReplicationInstance` and
  `CreateReplicationTask`/`ModifyReplicationTask`'s real SDK input structs,
  field by field, rather than trusting the existing `wire: ok` marks.

  **Two real bugs found and fixed**, both genuine "op accepts nothing for a
  real, required-adjacent, readable field" cases -- see the `ops:` notes above
  for full citations:
  1. `ReplicationInstance`: `ReplicationSubnetGroupIdentifier` and
     `VpcSecurityGroupIds` were both entirely unaccepted by
     `CreateReplicationInstance` (the former also missing from
     `ModifyReplicationInstance`, correctly -- it's create-only in the real
     API), even though both are real request members and both round-trip onto
     real, always-present `ReplicationInstance` response fields
     (`ReplicationSubnetGroup`, `VpcSecurityGroups`). The 08-20 pass's
     `riToJSON` diff covered `KmsKeyId`/`DnsNameServers`/`NetworkType`/
     `PreferredMaintenanceWindow` but missed these two nested/list members,
     which had been hardcoded to an empty placeholder and an empty list
     respectively since before that pass.
  2. `ReplicationTask`: `CdcStartPosition`, `CdcStopPosition`, and `TaskData`
     were entirely unaccepted by both `CreateReplicationTask` and
     `ModifyReplicationTask`, despite all three being real request members on
     both ops AND real top-level `types.ReplicationTask` response members --
     this family had not been field-diffed since the 07-23/07-31 passes, both
     of which predate the top-level-scalar-diff methodology the 08-20 pass
     introduced for `Endpoint`/`ReplicationInstance`; `ReplicationTask` was
     explicitly disclosed as NOT re-diffed in the 08-20 notes above.

  Both proven by real `aws-sdk-go-v2/service/databasemigrationservice` client
  round trips in `wire_field_fixes_test.go`
  (`TestReplicationInstance_SubnetGroupAndVpcSecurityGroups_RealClient`,
  `TestReplicationTask_CDCSettings_RealClient`), each hand-reverted (`git
  checkout --` the touched files, confirmed the new tests fail with the exact
  predicted symptom -- empty string/empty list where a real value was
  expected -- then restored, `md5sum`-verified byte-identical).

  **Other families swept without finding further bugs this pass** (each
  op's own real Input struct read directly, not assumed): `Endpoint`
  (`CreateEndpoint`/`ModifyEndpoint` cover every top-level member except
  `ResourceIdentifier`, which has no distinct response field on real
  `types.Endpoint` either -- baked into the ARN only, not a silent-drop
  candidate), `Volume`/SVM-adjacent DMS concepts (n/a, FSx-only),
  `ReplicationSubnetGroup`, `Certificate`, `Connection`, `DataProvider`,
  `InstanceProfile`, `MigrationProject`. `cmd/enumcheck`, `cmd/acceptguard`,
  `cmd/zeroguard`, and `cmd/xmlitemwrap` all reported zero findings for this
  service both before and after this pass's fixes -- consistent with this
  campaign's repeated observation that these tools miss the write-only-state
  bug class entirely.

- **2026-08-29 error-path sweep**: every prior pass's `errors: ok` mark on
  every one of the 119 op rows was an unverified blanket claim -- no pass had
  actually extracted each op's own `deserializeOpError<Op>` switch from
  `databasemigrationservice@v1.66.4/deserializers.go` and cross-checked it
  against the sentinel each backend call site raises. This pass did: all 119
  `awsAwsjson11_deserializeOpError*` functions extracted (8 model no typed
  exception at all -- `DescribeAccountAttributes`, `DescribeEndpointSettings`,
  `DescribeEndpointTypes`, `DescribeEngineVersions`, `DescribeEventCategories`,
  `DescribeEvents`, `DescribeExtensionPackAssociations`,
  `DescribeOrderableReplicationInstances`; the remaining 111 model between 1
  and 11 typed exceptions each). Protocol confirmed JSON-RPC 1.1
  (`awsAwsjson11_*`), matching the prior notes above.

  **One confirmed wrong-sentinel bug, fixed**: `DeleteFleetAdvisorCollector`
  -- see the `fleet-advisor` note above for the full citation and fix.

  **One systemic fabricated-code finding, left unfixed (RESTRAINT)**:
  `ErrValidation` (wire code `"ValidationException"`) is used at 11 call
  sites across 8 operations (`CreateDataMigration`,
  `ApplyPendingMaintenanceAction`, `CreateEndpoint`/`ModifyEndpoint`,
  `CreateReplicationTask`, `StartReplicationTask`,
  `ReloadTables`/`ReloadReplicationTables`, `StartReplication`,
  `CreateInstanceProfile`) to reject an invalid enum-shaped string on a
  required field (e.g. `MigrationType`, `SslMode`, `ApplyAction`,
  `NetworkType`). `types.ValidationException` **does not exist anywhere** in
  this SDK's `types/errors.go` (confirmed: only 26 exception types are
  declared service-wide, none named `ValidationException` or any generic
  `InvalidParameterValueException`-equivalent), so a real client's
  `errors.As(&types.ValidationException{})` can never succeed against this
  code path -- it always falls through to `smithy.GenericAPIError`. Confirmed
  reachable: `validateOp<Op>Input` for these ops only checks field presence,
  never enum-value membership (e.g. `MigrationTypeValue` is a bare string
  type; `validateOpCreateReplicationTaskInput` only calls
  `smithy.NewErrParamRequired` when the field is empty). Not fixed because no
  operation among the 8 models any typed exception that fits "invalid
  enum-shaped input value" -- most model only resource-shaped faults
  (`ResourceNotFoundFault`, `InvalidResourceStateFault`,
  `ResourceAlreadyExistsFault`, `AccessDeniedFault`, ...) with no
  validation-flavored member at all (`ApplyPendingMaintenanceAction` models
  only `ResourceNotFoundFault`). Per this campaign's restraint rule ("if an
  op models no exception matching the failure, say so and leave it -- do not
  invent an error code"), left as-is rather than guessing a replacement.
  Flagging here so a future pass with more evidence (e.g. real AWS API
  traffic) can resolve it with confidence instead of guessing.

  **Also confirmed unimplemented, not fixed (feature gaps, not sentinel
  bugs)**: three other operation-unique codes have no corresponding backend
  validation at all, so they can never fire: `ImportCertificate` models
  `InvalidCertificateFault` (no PEM-content validation exists);
  `ModifyReplicationSubnetGroup` models `SubnetAlreadyInUse` (no cross-group
  subnet-membership tracking exists); `ModifyReplicationInstance` models
  `UpgradeDependencyFailureFault` (no engine-version upgrade-path modeling
  exists). Each would require adding new business-logic simulation, not
  swapping a wrong sentinel, so left out of scope for this pass.

## 2026-08-30 (gopherstack-4shm WrapOp request-field re-scan, wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)

This service dispatches every op through `service.WrapOp` (119 entries).
Any earlier "exhaustive request-field sweep" or "spot-checked, not
rescanned" verdict that anchored on literal decode calls alone resolves
**0 of 119 operations (0%)**: this service was entirely invisible to that
method, gopherstack-4shm's exact class -- the worst blind spot measured
across the four services covered this pass.

The new `cmd/reqfieldscan` tool reaches **119 of 119 (100%)**, 383 fields
across 119 distinct request types initially, **42 fields flagged**. Every
flagged field was hand-verified against its own operation's real
`databasemigrationservice@v1.66.4` Input struct before being called
anything.

**4 real bugs found and fixed this pass:**
- **`RebootReplicationInstanceInput.ForceFailover`/`ForcePlannedFailover`**
  (`api_op_RebootReplicationInstance.go`: "`--force-planned-failover` and
  `--force-failover` can't both be set to true") were decoded and never
  read at all -- a request setting both got a 200, not a rejection. This is
  distinct from this file's existing "`RebootReplicationInstance` is
  correctly a state no-op" note above, which is about *state* (no field on
  `ReplicationInstance` changes after a real reboot) and still holds; the
  new finding is about *input validation*, never addressed by that note.
  Fixed with a validation check ahead of the backend call. New test
  `TestRebootReplicationInstance_ForceFailoverMutuallyExclusive`
  confirmed failing (200 instead of 400) against unmodified code.
- **`describeSchemasInput.ReplicationInstanceArn` was a fabricated field**
  -- the real `DescribeSchemasInput` (`api_op_DescribeSchemas.go`) declares
  only `EndpointArn`/`Marker`/`MaxRecords`; no client would ever send this
  key under this operation. Deleted rather than wired up, per the
  fabricated-capability shape.
- **`refreshSchemasInput.ReplicationInstanceArn`** is real and "This member
  is required" (`api_op_RefreshSchemas.go`) but was decoded and never read
  -- a request omitting it got a 200. Fixed with a required-field
  validation check. New test
  `TestRefreshSchemas_ReplicationInstanceArnRequired` confirmed failing
  against unmodified code; the pre-existing `TestDescribeSchemas` (which
  already sent a non-empty `ReplicationInstanceArn: "arn:fake"`) needed no
  change and still passes.

**38 fields remain unread, judged as follows:**
- **3 confirmed stubs, not fixed this pass** (feature additions, not field
  wiring): `handleDescribeRecommendationLimitations` and
  `handleDescribeEndpointSettings` both hardcode an empty envelope with
  **no backend call at all** (`describeRecommendationLimitationsInput`'s
  `NextToken`/`MaxRecords`/`Filters` and `describeEndpointSettingsInput`'s
  `EngineName`/`Marker`/`MaxRecords`) -- the "listing never consults its
  store" shape, parity-principles rule 1/4. Notably,
  `DescribeRecommendations` (this same family) WAS fixed for the identical
  shape on 2026-08-29 (`Filters`/pagination applied against real
  `Recommendation` fields); its sibling `DescribeRecommendationLimitations`
  was not caught by that pass. `handleDescribePendingMaintenanceActions`
  is the same shape but likely structural: this backend has no
  "pending maintenance action" data model anywhere (confirmed --
  `ApplyPendingMaintenanceAction` only validates and returns, no state is
  ever recorded to later list), so "always empty" may be a legitimate
  answer rather than a stub; left unresolved for a follow-up to confirm
  against real AWS behavior.
- **Fleet Advisor family (11 fields across
  `DescribeFleetAdvisorLsaAnalysis`/`DescribeFleetAdvisorSchemaObjectSummary`/
  `DescribeFleetAdvisorSchemas`) and the metadata-model export family (5
  fields: `ExportMetadataModelAssessment.FileName`/`AssessmentReportTypes`,
  `StartMetadataModelExportAsScript.FileName`,
  `StartMetadataModelExportToTarget.OverwriteExtensionPack`,
  `StartMetadataModelImport.Refresh`)** -- both already explicitly
  deprioritized in this file's "Fleet Advisor and Schema Conversion
  (metadata-model) op families are low future value" note above (AWS's own
  Fleet Advisor end-of-support notice, dated 2026-05-20, has passed). Not
  re-litigated this pass; `exportMetadataModelAssessmentInput`'s own
  in-code comment ("No schema-conversion engine or S3 integration exists
  in this emulation") already gives the reason correctly and stopped this
  pass from "fixing" it.
- **`DescribePendingMaintenanceActionsInput`'s remaining 3 fields
  (`ReplicationInstanceArn`/`Marker`/`MaxRecords`)** -- same stub as above.
- **`describeEndpointTypesInput.Marker`/`MaxRecords`,
  `describeReplication*StatisticsInput`/`describeReplicationConfigsInput`'s
  `Filters`/`Marker`/`MaxRecords`, `describeMetadataModelChildrenInput`'s
  `Marker`/`MaxRecords`, `describeReplicationInstanceTaskLogsInput`'s
  `Marker`/`MaxRecords`** -- real, unimplemented pagination/filtering on
  otherwise-real listings (each does call its backend). Genuine gaps, not
  fabricated fields; left open as a pagination-completeness follow-up
  rather than fixed piecemeal in this pass.
- **`batchStartRecommendationsInput.Data`,
  `startRecommendationsInput.Settings`,
  `updateSubscriptionsToEventBridgeInput.ForceMove`** -- `BatchStartRecommendations`
  is explicitly noted above as void-envelope-correct
  ("seeds a recommendation per source endpoint; pre-existing, unchanged").
  `UpdateSubscriptionsToEventBridge` (`handleUpdateSubscriptionsToEventBridge`)
  is a full no-op (`_ context.Context, _ *updateSubscriptionsToEventBridgeInput`,
  always returns `Applied: false`) -- likely structural (no EventBridge
  migration-event integration exists here), not independently re-verified
  against a real AWS trace this pass.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run`
-- all clean (`./services/dms/...` and `./cmd/reqfieldscan/...`).

- **2026-09-06 (gopherstack-cq25) StartReplicationTaskAssessment
  preconditions**: `api_op_StartReplicationTaskAssessment.go:16-23`
  (databasemigrationservice@v1.66.4) requires "The task must be in the
  stopped state" and "The task must have successful connections to the
  source and target," else `InvalidResourceStateFault`. Neither was
  enforced; `handleStartReplicationTaskAssessment` only checked the task
  existed.
  - **ready vs. stopped**: settled against `types/types.go`'s
    `ReplicationTask.Status` doc (no formal enum type -- `Status` is a
    documented `*string`). AWS defines `"ready"` and `"stopped"` as
    distinct, mutually exclusive statuses: `"ready"` is "the task is in a
    ready state where it can respond to other task operations" (reached
    after `"creating"`), while `"stopped"` is reached only "in response to
    running the StopReplicationTask operation." This backend's own
    `CreateReplicationTask` already sets a fresh task's `Status` to
    `statusReady` (`replication_tasks.go`), distinct from `statusStopped`
    (only reachable via `StopReplicationTask`, which itself requires
    `statusRunning` first) -- the backend's own state machine already
    tracks AWS's distinction correctly. Treating `ready` as satisfying
    "must be stopped" would fabricate an equivalence AWS's docs explicitly
    reject. Verdict: enforce `Status == "stopped"`, literally.
  - **connection-test precondition**: settled by checking whether this
    backend models `TestConnection` results at all. It does:
    `InMemoryBackend.TestConnection` (`connections.go`) records a
    `Connection{..., Status: statusSuccessful}` keyed by
    `<region>|<replicationInstanceArn>:<endpointArn>`, queryable via
    `DescribeConnections`/the same store. This is a real, queryable
    connection-result model, not something that would need inventing.
    Verdict: enforceable -- and enforced, checking a successful recorded
    `TestConnection` for both `(ReplicationInstanceArn, SourceEndpointArn)`
    and `(ReplicationInstanceArn, TargetEndpointArn)`.
  - **implementation**: new `InMemoryBackend.StartReplicationTaskAssessment`
    (`assessment_runs.go`) replaces the handler's direct
    `DescribeReplicationTasks` call; it returns `ErrInvalidState`
    (`InvalidResourceStateFault`) for either unmet precondition, matching
    the existing `ReloadTables`/`StopReplicationTask` guard convention
    (`replication_tasks.go`).
  - **pre-existing test corrected**: `TestStartReplicationTaskAssessment/returns_task_on_success`
    ran against a freshly created ("ready") task with no `TestConnection`
    calls -- asserting the un-enforced behavior. Corrected to call
    `TestConnection` for both endpoints and drive the task through
    `StartReplicationTask`/`StopReplicationTask` before asserting success,
    so it still tests the real success path (not weakened). Two new
    subtests, `rejects_task_not_stopped` and
    `rejects_task_without_successful_connections`, isolate each guard
    (each holds the other precondition constant) and were confirmed to
    fail against the pre-fix code (400 expected, got 200) by temporarily
    reverting each guard in turn, running the isolated subtest, and
    restoring.
  - Nothing left unenforceable for this op: both documented preconditions
    are backed by real, queryable state in this backend.
