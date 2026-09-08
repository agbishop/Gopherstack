---
# PARITY MANIFEST — IMPLEMENTED. See "Implementation summary (this pass)" below the frontmatter for
# the hard-design-problem decisions, corrections found, and gate results from the original
# 2026-08-01 implementation pass and its gopherstack-i6oz cli.go-wiring follow-up. The original
# pre-implementation audit prose (everything from "## Purpose of this document" onward) is left
# otherwise unmodified as the wire-shape ground truth the implementation was built from.
#
# 2026-08-05 pass (this one): the frontmatter above this comment previously still read as a
# pre-implementation spec -- every families: row said "gap", and there was no ops: key at all --
# despite the body of this same file documenting a completed implementation in detail ("all 95 ops
# routed/backed/persisted", gate results, an A- grade). This pass corrected that mismatch: read the
# actual .go files (all backend files: sourceservers.go, jobs.go, applications.go, waves.go,
# connectors.go, vcenterclients.go, exportimport.go, s3import.go, actions.go, serviceinit.go,
# networkmigration.go, networkmigrationjobs.go, launchconfig.go, replicationconfig.go, tagging.go)
# and verified GetSupportedOperations()/routes() against the SDK's own 95-op list, confirmed
# `go test ./services/mgn/...` and `go test -race -count=1 ./services/mgn/...` both pass, confirmed
# wireMGNS3 is present and called in cli.go, confirmed SeedSourceServer was in fact removed, and
# confirmed several specific honest-gap claims already made in prose below (mapper segments always
# empty/404, StartImport's ModifiedCount always zero, ListManagedAccounts always returns only the
# caller's own account, StartTest/StartCutover mint a synthetic non-cross-checked EC2 instance ID)
# by reading the exact code paths, not by trusting the prose. overall: is left unchanged (see its own
# note below).
#
# 2026-08-06 pass (gopherstack-xd34, A- -> A): added test/integration/mgn_test.go, the SDK-driven
# integration suite this service previously had zero of (parity-principles.md rule 3: unit tests are
# not parity proof) -- 9 test funcs, Docker-verified, covering source-server/job/template/application-
# wave lifecycles, tagging across 5 resource kinds, not-found/validation error tables, network
# migration, and the new cross-service ListManagedAccounts wiring. Closed every buildable gap this
# pass found: (1) StartImport's CSV schema was a fully invented flat column set with zero AWS
# provenance -- replaced with the real "mgn:server:*" namespaced parameter names AWS's own MGN User
# Guide documents (WebFetch of docs.aws.amazon.com/mgn/latest/ug/import-main.html), scoped to the
# SourceServer-level subset (see s3import.go's doc comment for why mgn:app:*/mgn:wave:*/mgn:launch:*
# stayed out of scope). (2) ModifiedCount was hardcoded zero -- now a real count, using
# mgn:server:user-provided-id as the natural re-import dedup key AWS's own docs say it's for.
# (3) StartTest/StartCutover minted a synthetic, non-cross-checked EC2 instance ID -- now launches a
# real services/ec2 instance via a new cross_service.go (grafana's cross-service pattern), falling
# back to synthetic only when EC2 isn't wired. (4) UpdateSourceServer silently dropped
# FqdnForActionFramework/UserProvidedID entirely (a real bug the integration suite caught, not
# something this pass set out to fix) -- now wired end to end, and no longer unconditionally wipes
# ConnectorAction on every call. (5) ListManagedAccounts always returned only the caller's own account
# -- now resolves real AWS Organizations member accounts via the same cross_service.go when this
# account is the org's management account or a registered MGN delegated administrator. Judged
# structural and left alone: NetworkMigrationExecutionID/VcenterClient creation (no public op exists
# in either case), and Network Migration analysis/codegen/deployment/mapper-segment CONTENT (no
# analysis engine exists to produce it) -- moved into structural_gaps: per services/_PARITY_TEMPLATE.md
# with individual justification, not used as a blanket escape hatch.
service: mgn
sdk_module: aws-sdk-go-v2/service/mgn@v1.48.4   # gopherstack-u8my: go.mod had already moved to
# v1.48.4; the "unchanged since 2026-08-01" note was stale. Diffed v1.48.3 vs v1.48.4:
# types/{types,enums,errors}.go, serializers.go, deserializers.go, validators.go byte-identical --
# only client middleware plumbing differs, so no wire-shape claim in this file was affected.
last_audit_commit: ee8d5788f
last_audit_date: 2026-08-21
# 2026-08-30: cursor-population sweep (does every List/Describe response struct that DECLARES a
# NextToken actually SET one before the collection can exceed a page?). Enumerated all 29 SDK ops
# whose Input/Output declare NextToken. 22 already correct via the shared pkgs/page.New chokepoint
# (page.go + listNMJobs helper). 6 correctly left unpopulated: ListExportErrors,
# ListNetworkMigrationAnalysisResults, ListNetworkMigrationCodeGenerationSegments,
# ListNetworkMigrationDeployedStacks, ListNetworkMigrationMapperSegmentConstructs,
# ListNetworkMigrationMapperSegments -- each documented in-code as provably always-empty (no op in
# this SDK's surface can ever populate them; see networkmigrationjobs.go/networkmigration.go doc
# comments). 1 genuine bug found and fixed: ListManagedAccounts (see its ops: entry) -- the one op
# in the family that bypassed the shared pagination pattern.
# 2026-08-30 sort-totality sweep (Class F: a sort that exists but is not total,
# and Class G: parallel result lists truncated independently). This service has
# NO explicit sort.Slice/slices.Sort* call in any listing -- every paginated op
# builds its page via the shared pkgs/page.New chokepoint over
# store.Table.Snapshot() (or a filtered clone of it), never store.Table.All().
# Table.Snapshot() (pkgs/store/table.go) returns items ordered by the table's
# own keyFn, ascending -- and every mgn table is keyed by that resource's real
# unique ID (SourceServerID/ApplicationID/WaveID/ConnectorID/JobID/...), so the
# base order is already total by construction; page.New itself only offset-
# slices a slice its own doc comment requires to already be "fully sorted", it
# does not sort. Confirmed no listing response in this service carries two-or-
# more collections the API defines as one ordered sequence (each op returns
# exactly one paginated array). No bugs found or fixed this pass; 0 code
# changes for Class F/G.
# 2026-08-31 value-semantics sweep (gopherstack-uox6): audited every filter-typed
# field across all 30 List*/Describe* input structs (~40 filter fields by this
# pass's own count, including AccountID/ID-list scoping members). covledger
# reported no filter_default_semantics row for this service; git log/PARITY.md
# confirmed no prior audit on this specific axis (the 08-29 "unhonoured list
# constraints" pass, 43eab7be5, is the sibling request_field_never_read class --
# it fixed ActionIDs/JobIDs filters that were parsed but never wired to the
# backend at all; this pass checked the ones that WERE wired for correctness).
# ONE BUG FOUND AND FIXED: DescribeJobs' Filters.FromDate/ToDate were decoded
# off the wire (describeJobsFiltersWire has both fields) but the handler only
# ever read Filters.JobIDs -- FromDate/ToDate were silently dropped. The
# pre-fix doc comment on DescribeJobsFilters claimed this was deliberate
# ("not implemented... not exercised by round-trip tests"), which was untrue:
# Job.CreationDateTime is real, comparable backing data (nowRFC3339, a single
# fixed-width UTC RFC3339 format every Job write uses). Fixed: both bounds
# now applied as inclusive lexicographic comparisons against CreationDateTime
# (jobs.go's matchesJobFilter); no field-name qualifier like "Exclusive"
# exists to suggest otherwise. Every other filter surface checked clean: all
# ID-list filters (ApplicationIDs/WaveIDs/ConnectorIDs/ExportIDs/ImportIDs/
# JobIDs/SegmentIDs/ActionIDs/etc.) match their own op's serializer key and
# empty-means-unfiltered; every enum-typed filter (ReplicationTypes,
# LifeCycleStates, NetworkMigrationExecutionStatuses) compares against the
# same enum its own doc comment names, verified constant-by-constant against
# the pinned SDK; every MaxResults doc comment across all 30 ops states no
# specific number, so the uniform defaultPageLimit=100 violates nothing; no
# switch-over-filter-name shape exists anywhere in this service's filter
# surface (all matching is containsStr/pointer-equality, not a switch). No
# second bug found downstream of the DescribeJobs fix (JobIDs filtering was
# already correct, so nothing was previously unreachable). Proven via
# list_filter_params_test.go's new TestDescribeJobs_DateRangeFilterHonoured
# (5 subtests: unfiltered, exact-boundary-both-inclusive, fromDate-excludes,
# toDate-excludes, in-range-includes), confirmed to fail against unmodified
# code on the two exclusion subtests before the fix landed. Assertion count
# in that file: 15 -> 20 require/assert calls, all additions, 0 drops.
overall: A   # raised from A- (gopherstack-xd34): the SDK-driven integration suite this A-/B distinction
# hinges on now exists and passes under Docker, and every buildable gap this pass found (5 items,
# enumerated in the comment block above) is closed. What remains in gaps:/structural_gaps: below is
# either genuinely unfixable (no data source can exist) or a proportionate, explicitly justified scope
# decision (mgn:app:*/mgn:wave:*/mgn:launch:* CSV columns) -- the same class of remaining gap other
# A-grade services in this repo carry (e.g. services/grafana/PARITY.md's own gaps: list).
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  # source_server_lifecycle (16)
  DescribeSourceServers: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateSourceServer: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-06: FqdnForActionFramework/UserProvidedID were parsed off the wire request but never applied -- ConnectorAction was the only field the backend actually wired, and it was applied unconditionally (silently clearing ConnectorAction on any update that didn't re-send it). Fixed: SourceServerUpdate (sourceservers.go) applies each field only when the caller's JSON body includes it, matching AWS's own partial-update semantics. Platform is accepted off the wire and dropped -- the real SDK's own SourceServer/SourceProperties output has no Platform field to read it back from either."}
  UpdateSourceServerReplicationType: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteSourceServer: {wire: ok, errors: ok, state: ok, persist: ok}
  ChangeServerLifeCycleState: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED (gopherstack-kwhp): api_op_ChangeServerLifeCycleState.go:12-15 (\"This command only works if the Source Server is already launchable (dataReplicationInfo.lagDuration is not null.)\") was not enforced, and sourceservers.go:30-32's comment justified that with a 'manual-override' purpose neither aws-sdk-go-v2 nor botocore documents -- both document a precondition instead ('manual'/'override' occur zero times in the botocore MGN model). This backend never fabricates LagDuration (models.go:136-140), but DataReplicationState reaching CONTINUOUS is the same real, deterministic signal sourceservers.go:23 already treats as launchable, so the precondition is now enforced against it: an earlier call returns ConflictException. The 4 call sites that previously relied on the op succeeding immediately after import (sourceserver_lifecycle_precondition_test.go's MarkAsArchived 'cutover state allowed' subtest, all 3 TerminateTargetInstances subtests, and test/integration/mgn_test.go's TestIntegration_MGN_SourceServerLifecycle) were shortcuts to reach a LifeCycleState, not tests of pre-replication behavior -- each now waits for CONTINUOUS first."}
  DisconnectFromService: {wire: ok, errors: ok, state: ok, persist: ok}
  FinalizeCutover: {wire: ok, errors: ok, state: ok, persist: ok}
  MarkAsArchived: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED (2026-09-04 delete/update precondition sweep): api_op_MarkAsArchived.go:13-14 (\"This command only works for SourceServers with a lifecycle. state which equals DISCONNECTED or CUTOVER.\") was never enforced -- any lifecycle state could be archived. Now returns ConflictException (modelled on this op) unless LifeCycleState is DISCONNECTED or CUTOVER."}
  StartTest: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-06: on Job completion, launches a real services/ec2 instance via launchParticipantInstanceLocked (cross_service.go), resolving AMI/instance type from the source server's LaunchConfiguration.Ec2LaunchTemplateID when it names a real EC2 launch template, else the EC2 backend's own stub AMI catalogue + a documented default instance type. Falls back to a synthetic gopherstack-format instance ID (newSyntheticInstanceID) only when the EC2 backend isn't wired (unit tests) or RunInstances itself fails -- verified end to end against a real Docker container in test/integration/mgn_test.go's TestIntegration_MGN_JobLifecycle (DescribeInstances against the launched ID)."}
  StartCutover: {wire: ok, errors: ok, state: ok, persist: ok, note: "same real-EC2-launch path as StartTest (jobs.go, cross_service.go)"}
  StartReplication: {wire: ok, errors: ok, state: ok, persist: ok}
  StopReplication: {wire: ok, errors: ok, state: ok, persist: ok}
  PauseReplication: {wire: ok, errors: ok, state: ok, persist: ok}
  ResumeReplication: {wire: ok, errors: ok, state: ok, persist: ok}
  RetryDataReplication: {wire: ok, errors: ok, state: ok, persist: ok}
  TerminateTargetInstances: {wire: ok, errors: ok, state: fixed, persist: ok, note: "clears LaunchedInstance for real (jobs.go:226-228); does not mint a synthetic id, unlike StartTest/StartCutover. FIXED (2026-09-04 delete/update precondition sweep): api_op_TerminateTargetInstances.go:13-14 (\"This command will not work for any Source Server with a lifecycle.state of TESTING, CUTTING_OVER, or CUTOVER.\") was never enforced -- requireLifecyclePrecondition had no case for InitiatedByTerminate at all. Now returns ConflictException (modelled on this op) when LifeCycleState is TESTING/CUTTING_OVER/CUTOVER."}
  # jobs (3)
  DescribeJobs: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (value-semantics sweep, gopherstack-uox6): Filters.FromDate/ToDate were decoded off the wire but never applied -- a source comment claimed this was deliberate ('not exercised by round-trip tests'), but Job.CreationDateTime (nowRFC3339, fixed-width UTC) is real, comparable, backing data. Now both-inclusive lexicographic bounds against CreationDateTime; JobIDs filtering was already correct."}
  DescribeJobLogItems: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteJob: {wire: ok, errors: ok, state: ok, persist: ok}
  # launch_configuration (6)
  GetLaunchConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "flattened per-server shape backed by an internal LaunchConfiguration type this package invented -- no named SDK struct exists for it (models.go)"}
  UpdateLaunchConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateLaunchConfigurationTemplate: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-101r): Ec2LaunchTemplateID was accepted from the request body, but it is Output-only on the real Input (api_op_CreateLaunchConfigurationTemplate.go:104) -- no real client can ever send it. Removed from the wire request and the backend Input struct rather than derived: this backend has no imageID to hand a companion EC2 launch template at template-creation time (unlike LaunchConfiguration.Ec2LaunchTemplateID, which real UpdateLaunchConfigurationInput does accept -- a distinct, per-source-server field), so deriving one would mean fabricating it. Stays permanently empty on Output, same honesty bar as ImportErrorData's Ec2LaunchTemplateID."}
  DeleteLaunchConfigurationTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeLaunchConfigurationTemplates: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateLaunchConfigurationTemplate: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-101r): same Ec2LaunchTemplateID removal as CreateLaunchConfigurationTemplate (api_op_UpdateLaunchConfigurationTemplate.go:106 is Output-only too)."}
  # replication_configuration (6)
  GetReplicationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "flattened per-server shape, same invented-internal-type pattern as GetLaunchConfiguration"}
  UpdateReplicationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateReplicationConfigurationTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteReplicationConfigurationTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeReplicationConfigurationTemplates: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateReplicationConfigurationTemplate: {wire: ok, errors: ok, state: ok, persist: ok}
  # applications (8)
  CreateApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  ListApplications: {wire: ok, errors: ok, state: ok, persist: ok, note: "AggregatedStatus rollup (rollupHealthStatus/rollupProgressStatus, applications.go) is this package's own invented aggregation rule, not SDK-specified"}
  ArchiveApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  UnarchiveApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateSourceServers: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateSourceServers: {wire: ok, errors: ok, state: ok, persist: ok}
  # waves (8)
  CreateWave: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateWave: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteWave: {wire: ok, errors: ok, state: ok, persist: ok}
  ListWaves: {wire: ok, errors: ok, state: ok, persist: ok, note: "AggregatedStatus rollup (waves.go), same invented-aggregation-rule pattern as ListApplications"}
  ArchiveWave: {wire: ok, errors: ok, state: ok, persist: ok}
  UnarchiveWave: {wire: ok, errors: ok, state: ok, persist: ok}
  AssociateApplications: {wire: ok, errors: ok, state: ok, persist: ok}
  DisassociateApplications: {wire: ok, errors: ok, state: ok, persist: ok}
  # connectors (4)
  CreateConnector: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateConnector: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteConnector: {wire: ok, errors: ok, state: ok, persist: ok}
  ListConnectors: {wire: ok, errors: ok, state: ok, persist: ok}
  # vcenter_clients (2) -- no Create op exists in this SDK surface at all (real AWS creates these via
  # the vCenter connector appliance registering itself); SeedVcenterClient is this package's own
  # non-SDK, unrouted creation seam, documented in the "gaps" list below, not counted as one of the 95.
  DescribeVcenterClients: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteVcenterClient: {wire: ok, errors: ok, state: ok, persist: ok}
  # export_import (8)
  StartExport: {wire: ok, errors: ok, state: ok, persist: ok, note: "Summary is a real live count of the account's Applications/Waves/SourceServers, never fabricated"}
  ListExports: {wire: ok, errors: ok, state: ok, persist: ok}
  ListExportErrors: {wire: ok, errors: ok, state: ok, persist: ok}
  StartImport: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-06: CSV schema replaced -- the prior column set (hostname/fqdn/cpuCores/ramBytes/...) was fully invented with zero AWS provenance. Now uses AWS's own documented \"mgn:server:*\" namespaced parameters (MGN User Guide's Import parameters table: mgn:server:user-provided-id, mgn:server:fqdn-for-action-framework, mgn:server:tag:<key>), plus a same-convention extension onto the SDK's real IdentificationHints fields (hostname/fqdn/aws-instance-id/vmware-uuid/vmpath) for the identification requirement AWS's docs state in prose but don't formally tabulate. ModifiedCount is now real: a row whose mgn:server:user-provided-id matches an existing SourceServer updates it (documented AWS dedup behavior) instead of always creating a new one. Scoped to SourceServer-level columns only -- mgn:app:*/mgn:wave:*/mgn:launch:* (implicit Application/Wave creation, per-row LaunchConfiguration overrides) are real, doc-confirmed parameters this pass did not implement (see gaps) and s3import.go's doc comment."}
  ListImports: {wire: ok, errors: ok, state: ok, persist: ok}
  ListImportErrors: {wire: ok, errors: ok, state: ok, persist: ok}
  StartImportFileEnrichment: {wire: ok, errors: ok, state: partial, persist: ok, note: "PENDING->STARTED->SUCCEEDED bookkeeping only (exportimport.go:301-343) -- never reads or actually enriches the target S3 object with real network/segment metadata; no such discovery engine exists"}
  ListImportFileEnrichments: {wire: ok, errors: ok, state: ok, persist: ok}
  # actions (6) -- state-only (documents listed/ordered/active), never invokes any SSM document; this
  # repo has no SSM execution engine, and real AWS's own public API for this family is likewise
  # metadata-only (execution happens as part of a launch, outside this API surface).
  PutSourceServerAction: {wire: ok, errors: ok, state: ok, persist: ok}
  ListSourceServerActions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (constraint sweep): Filters.ActionIDs was decoded from the wire but never passed to the backend -- every source server's full action list came back regardless of the filter."}
  RemoveSourceServerAction: {wire: ok, errors: ok, state: ok, persist: ok}
  PutTemplateAction: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTemplateActions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (constraint sweep): same Filters.ActionIDs-dropped bug as ListSourceServerActions."}
  RemoveTemplateAction: {wire: ok, errors: ok, state: ok, persist: ok}
  # service_init (2)
  InitializeService: {wire: ok, errors: ok, state: ok, persist: ok}
  ListManagedAccounts: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-08-06: now resolves real AWS Organizations member accounts (resolveManagedAccountsLocked, cross_service.go) when this account is the org's management account or a registered delegated administrator for mgnServicePrincipal (\"mgn.amazonaws.com\" -- an unconfirmed but conventionally-derived value, same evidentiary standard this file already applies to ARN resource-path segments), falling back to just the caller's own account otherwise. Verified against a real Organizations backend in test/integration/mgn_test.go's TestIntegration_MGN_ListManagedAccounts. FIXED (2026-08-30, cursor sweep) -- ListManagedAccountsOutput.NextToken (api_op_ListManagedAccounts.go) was never populated: handleListManagedAccounts ignored req.NextToken/MaxResults entirely and returned the full member-account list unpaginated every call, silently truncating nothing only because no caller-controllable path could exceed one page before this fix, but a real org with many delegated/managed accounts could. Backend now returns page.Page[ManagedAccount] via pkgs/page, same chokepoint every other List/Describe op in this service already used. Proven via TestListManagedAccounts_Pagination (services/mgn/cross_service_test.go), a real Organizations backend wired through SetAppConfig with 3 accounts, MaxResults=2 + hand-revert (confirmed 3-of-3 returned on one page, no NextToken, pre-fix)."}
  # tagging (3)
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  # network_migration_definitions (13)
  CreateNetworkMigrationDefinition: {wire: ok, errors: ok, state: ok, persist: ok}
  GetNetworkMigrationDefinition: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateNetworkMigrationDefinition: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteNetworkMigrationDefinition: {wire: ok, errors: ok, state: ok, persist: ok}
  ListNetworkMigrationDefinitions: {wire: ok, errors: ok, state: ok, persist: ok}
  GetNetworkMigrationMapperSegmentConstruct: {wire: ok, errors: ok, state: partial, persist: ok, note: "always 404s (networkmigration.go:233-253) -- no network-analysis engine ever produces a segment construct to return"}
  ListNetworkMigrationMapperSegmentConstructs: {wire: ok, errors: ok, state: partial, persist: ok, note: "always returns an empty list after validating the (definition, execution) scope exists (networkmigration.go:254-277)"}
  ListNetworkMigrationMapperSegments: {wire: ok, errors: ok, state: partial, persist: ok, note: "always returns an empty list, same reason as ListNetworkMigrationMapperSegmentConstructs (networkmigration.go:278-287)"}
  UpdateNetworkMigrationMapperSegment: {wire: ok, errors: ok, state: partial, persist: ok, note: "always 404s -- no segment ever exists to update (networkmigration.go:288-297)"}
  ListNetworkMigrationMappings: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (constraint sweep): Filters.JobIDs -- the shared listNMScopedRequest wire struct did not even carry a filters field, so it was silently dropped regardless of what a real client sent."}
  ListNetworkMigrationMappingUpdates: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (constraint sweep): same Filters.JobIDs gap as ListNetworkMigrationMappings."}
  StartNetworkMigrationMapping: {wire: ok, errors: ok, state: ok, persist: ok, note: "auto-vivifies a NetworkMigrationExecution on first reference to an unseen (DefinitionID, ExecutionID) pair, since no op in this SDK surface creates one explicitly (resolveOrCreateExecutionLocked, networkmigrationjobs.go:74-118) -- a documented, deliberate convention, not independently confirmed against real AWS behavior"}
  StartNetworkMigrationMappingUpdate: {wire: ok, errors: ok, state: ok, persist: ok, note: "same auto-vivification convention as StartNetworkMigrationMapping"}
  # network_migration_analysis_deploy (10)
  StartNetworkMigrationAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "real PENDING->STARTED->SUCCEEDED job bookkeeping (networkmigrationjobs.go); same auto-vivification convention"}
  ListNetworkMigrationAnalyses: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (constraint sweep): same Filters.JobIDs gap as ListNetworkMigrationMappings."}
  ListNetworkMigrationAnalysisResults: {wire: ok, errors: ok, state: partial, persist: ok, note: "always returns an empty Items list even after the parent job SUCCEEDS (networkmigrationjobs.go:207-211) -- no real network-analysis engine exists to produce findings"}
  StartNetworkMigrationCodeGeneration: {wire: ok, errors: ok, state: ok, persist: ok}
  ListNetworkMigrationCodeGenerations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (constraint sweep): same Filters.JobIDs gap as ListNetworkMigrationMappings."}
  ListNetworkMigrationCodeGenerationSegments: {wire: ok, errors: ok, state: partial, persist: ok, note: "always empty Items, same reason as ListNetworkMigrationAnalysisResults (networkmigrationjobs.go:230-234) -- no code-generation engine exists"}
  StartNetworkMigrationDeployment: {wire: ok, errors: ok, state: ok, persist: ok}
  ListNetworkMigrationDeployments: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (constraint sweep): same Filters.JobIDs gap as ListNetworkMigrationMappings."}
  ListNetworkMigrationDeployedStacks: {wire: ok, errors: ok, state: partial, persist: ok, note: "always empty Items -- no real CloudFormation-equivalent deployment engine exists (networkmigrationjobs.go:250-257)"}
  ListNetworkMigrationExecutions: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  source_server_lifecycle: {status: ok, note: "16 ops, real state mutation throughout; StartTest/StartCutover launch a real services/ec2 instance as of 2026-08-06 -- see their ops: entries. No CreateSourceServer op exists anywhere in this 95-op surface (structural, see structural_gaps) -- StartImport is the only public-API creation path."}
  jobs: {status: ok, note: "3 ops: DescribeJobs, DescribeJobLogItems, DeleteJob -- real listing/deletion over Job records created by the source-server-lifecycle and export_import families."}
  launch_configuration: {status: ok, note: "6 ops: per-server GetLaunchConfiguration/UpdateLaunchConfiguration (flattened wire shape, backed by an internal type since no types.LaunchConfiguration struct exists) plus the separate LaunchConfigurationTemplate family (Create/Delete/Describe/Update), all real CRUD."}
  replication_configuration: {status: ok, note: "6 ops, same real-CRUD pattern as launch_configuration."}
  applications: {status: ok, note: "8 ops, real CRUD; AggregatedStatus rollup rules are this package's own invented, documented aggregation logic, not SDK-specified."}
  waves: {status: ok, note: "8 ops, same real-CRUD + invented-aggregation-rollup pattern as applications."}
  connectors: {status: ok, note: "4 ops, real CRUD."}
  vcenter_clients: {status: ok, note: "2 ops: DescribeVcenterClients (the ONLY GET besides the tagging trio), DeleteVcenterClient -- both real. No CreateVcenterClient op exists in this SDK surface (see gaps); SeedVcenterClient is this package's own non-SDK, unrouted creation seam."}
  export_import: {status: partial, note: "8 ops; StartExport/ListExports/ListExportErrors/ListImports/ListImportErrors/ListImportFileEnrichments are real. StartImport genuinely reads S3, uses AWS's own documented mgn:server:* CSV schema, and creates or updates real SourceServers with a real CreatedCount/ModifiedCount split (natural key: mgn:server:user-provided-id) as of 2026-08-06 -- see s3import.go's doc comment for the mgn:app:*/mgn:wave:*/mgn:launch:* columns still out of scope (gaps). StartImportFileEnrichment is PENDING->STARTED->SUCCEEDED bookkeeping only -- it never reads or actually enriches the target S3 object, since no network/segment discovery engine exists."}
  actions: {status: ok, note: "6 ops: PutSourceServerAction/ListSourceServerActions/RemoveSourceServerAction and the template-scoped PutTemplateAction/ListTemplateActions/RemoveTemplateAction -- real state-only bookkeeping (documents listed/ordered/active), matching real AWS's own API scope (SSM document execution happens at launch time, outside this API)."}
  service_init: {status: ok, note: "2 ops: InitializeService is real. ListManagedAccounts resolves real AWS Organizations member accounts as of 2026-08-06 when this account is the org's management account or a registered MGN delegated administrator, else returns just the caller's own account -- see its ops: entry."}
  tagging: {status: ok, note: "3 ops: TagResource/UntagResource/ListTagsForResource, the only ops sharing the /tags/{resourceArn} path and a distinct error set (AccessDenied/InternalServer/ResourceNotFound/Throttling/Validation) from every other op family in this service. Real ARN-keyed tag store."}
  network_migration_definitions: {status: partial, note: "13 ops under /network-migration/; CreateNetworkMigrationDefinition/Get/Update/Delete/List and ListNetworkMigrationMappings/ListNetworkMigrationMappingUpdates/StartNetworkMigrationMapping/StartNetworkMigrationMappingUpdate (9 ops) are real. The 4 mapper-segment ops (GetNetworkMigrationMapperSegmentConstruct, ListNetworkMigrationMapperSegmentConstructs, ListNetworkMigrationMapperSegments, UpdateNetworkMigrationMapperSegment) always return empty/404 -- no network-analysis engine ever produces a segment to report, a deliberate scope decision documented in 'Implementation summary' below (mapper segments left genuinely empty rather than given a second synthetic seeding seam)."}
  network_migration_analysis_deploy: {status: partial, note: "10 ops under /network-migration/; the 5 Start*/List*(non-Results/Segments/Stacks) ops (StartNetworkMigrationAnalysis, ListNetworkMigrationAnalyses, StartNetworkMigrationCodeGeneration, ListNetworkMigrationCodeGenerations, StartNetworkMigrationDeployment, ListNetworkMigrationDeployments, ListNetworkMigrationExecutions -- 7 ops) run a real PENDING->STARTED->SUCCEEDED job bookkeeping state machine with auto-vivified NetworkMigrationExecutionID (see gaps). ListNetworkMigrationAnalysisResults/ListNetworkMigrationCodeGenerationSegments/ListNetworkMigrationDeployedStacks (3 ops) always return an empty Items list -- no real analysis/codegen/deployment engine exists to produce content, honestly flagged rather than fabricated."}
gaps:
  - "StartImport's CSV schema (2026-08-06 fix, see StartImport's ops: entry) implements only the SourceServer-scoped subset of AWS's documented mgn:server:* parameters. AWS's MGN User Guide also documents mgn:app:*/mgn:wave:*/mgn:launch:* parameters for implicit Application/Wave creation and per-row LaunchConfiguration overrides during import -- real, doc-confirmed, and genuinely buildable (Applications/Waves already have real backends), but acting on the mgn:launch:* sub-fields (instance profile, per-NIC subnet/security-group/private-IP, placement, licensing, volume type) would require adding a dozen fields this backend's LaunchConfiguration type doesn't have at all -- a materially larger feature than the schema fix this pass scoped in. Left as an explicit, proportionate scope decision (s3import.go's doc comment), the same class of remaining gap other A-grade services in this repo carry (e.g. services/grafana/PARITY.md's DisassociateLicense limitation). (bd: gopherstack-xd34)"
structural_gaps:
  - "No CreateSourceServer op exists anywhere in this SDK's 95 operations. In real AWS, a SourceServer record is created only by the MGN Replication Agent (installed on the actual on-prem/cloud source machine) calling an internal, non-public control-plane API to register itself -- that registration call is NOT part of this public SDK surface at all. StartImport's bulk CSV import is the ONLY public-API path that creates SourceServer records in this implementation (createSourceServerLocked, sourceservers.go), and is now wire-reachable with a real, doc-derived CSV schema (2026-08-06) -- there is no further public-API creation path to add."
  - "No CreateVcenterClient op exists either, for the same reason: VcenterClient records are created by the MGN vCenter connector appliance registering itself, not via any public API in this surface, and StartImport's schema has no VcenterClient-creating columns (real AWS's own ImportTaskSummary has no VcenterClients count field, confirming this). DescribeVcenterClients/DeleteVcenterClient are read/delete only; SeedVcenterClient (vcenterclients.go) remains this emulator's only way to get a VcenterClient into the backend at all -- there is no public-API path to replace it with."
  - "No op creates a NetworkMigrationExecutionID. StartNetworkMigrationMapping, StartNetworkMigrationMappingUpdate, StartNetworkMigrationAnalysis, StartNetworkMigrationCodeGeneration, and StartNetworkMigrationDeployment all take NetworkMigrationExecutionID as a REQUIRED input field, and ListNetworkMigrationExecutions only lists existing ones -- no public op in this 95-op surface ever creates one. This implementation's resolution (resolveOrCreateExecutionLocked, networkmigrationjobs.go): auto-vivify a NetworkMigrationExecution the first time any of the 5 Start* ops references an unseen (DefinitionID, ExecutionID) pair, generalizing the only documented convention available -- a deliberate, already-optimal design given no creation op exists to defer to."
  - "The Network Migration sub-product's analysis/code-generation/deployment CONTENT (ListNetworkMigrationAnalysisResults/ListNetworkMigrationCodeGenerationSegments/ListNetworkMigrationDeployedStacks, plus the 4 mapper-segment ops under network_migration_definitions) analyzes exported on-prem network configuration and generates infrastructure-as-code/CloudFormation-equivalent artifacts. None of analysis, code generation, or deployment can be honestly performed by this emulator without either (a) a real network-analysis/codegen engine that does not exist in this repo, or (b) fabricating analysis findings and generated code as free-text strings -- no data source can exist for this content in an emulator. The state-bookkeeping shell (definitions, executions, mapper segments/constructs with their CRUD and status enums, real PENDING->STARTED->SUCCEEDED job progression) is honestly simulated; the CONTENT stays genuinely empty/404, never invented."
  - "AWS's own Service Authorization Reference and MGN User Guide pages for ARN/ID formats return a JS-shell body to automated fetches (same failure mode this repo's outposts/grafana audits hit on the same docs.aws.amazon.com domain), and Terraform's AWS provider has zero MGN resources (`internal/service/mgn/` is 4 auto-generated boilerplate files, FrameworkResources()/SDKResources() both empty) -- unlike directconnect/outposts, there is no Terraform-provider-source corroboration available for this service's ARN resource-path segments or ID formats at all. The only corroborating evidence is botocore's service-2.json metadata (endpointPrefix/serviceId/signingName all literally \"mgn\"), consistent with but not proof of the ARN service segment. No AWS::MGN::* CloudFormation resource type exists in this repo either (`grep -rli 'mgn\\b' services/cloudformation/` returns zero hits) -- MGN's own real CloudFormation support, if any, cannot be verified from this repo's tree. These are epistemic limits on independent verification, not implementation gaps: every specific value derived from them (ARN segments, mgnServicePrincipal) is already flagged inline as best-effort, not presented as confirmed."
deferred:
  - "Nothing this pass. All 95 ops are implemented and this pass's own integration suite (test/integration/mgn_test.go) exercises every op family named in gopherstack-xd34's scope (source servers, replication/launch configuration templates, jobs, applications/waves, tagging, network migration, cross-service EC2/Organizations wiring)."
leaks: {status: clean, note: "Handler.Reset()/Backend.Reset() close every SourceServer/Job/Application/Wave/etc.'s tags.Tags before clearing (tagging.go's 12 taggable kinds); InMemoryBackend.Close() stops the worker.Group backing every scheduled LifeCycleState/Job/ImportTask/NetworkMigrationJob transition timer -- verified by direct code read this pass, not re-derived from scratch."}
---

## 2026-08-21 pass (gopherstack-r80d batch 32): required-output-member audit, clean

Part of the mgn/redshiftdata/scheduler batch testing r80d's hypothesis that
op count predicts this bug class better than required-field count (batches
24-29 found bugs almost everywhere; batches 30-31 found none across six
services tied at 6 fields; see `services/_REQUIRED_OUTPUT_CANDIDATES.md`).
mgn is the hypothesis's primary test: **95 ops, only 5 required output
fields** (`cmd/requiredoutputfields`, cross-checked against a standalone
`go/ast`-style walk of `mgn@v1.48.4`'s `api_op_*.go` files -- both agreed
exactly). Module resolved directly (directory `mgn` == SDK module
`aws-sdk-go-v2/service/mgn@v1.48.4`, no `dirModuleOverride` entry needed);
confirmed no `drs` or `applicationmigration` sibling directory exists in this
repo to accidentally audit instead (`services/drs` doesn't exist; `drs` isn't
even a pinned dependency in `go.mod`).

The flat scan's 5 ops (`Create`/`UpdateLaunchConfigurationTemplate`,
`Create`/`UpdateReplicationConfigurationTemplate`, `ListManagedAccounts`) are
clean: `launchConfigurationTemplateWire.LaunchConfigurationTemplateID` and
`replicationConfigurationTemplateWire.ReplicationConfigurationTemplateID`
(wire.go:393,514) carry no `omitempty` and are always populated from a
`newLaunchTemplateID()`/equivalent call at `Create` time (launchconfig.go:161,
replicationconfig.go:222); `listManagedAccountsResponse.Items` (wire.go:1101)
is built via `make([]managedAccountWire, len(accounts))`, never gated on
length.

**Below the flat scan** (this hypothesis's whole point, given 95 ops): an
AST walk of `mgn@v1.48.4/types/types.go` found 19 domain structs carrying 34
required members total. Of those reachable through a response (as opposed to
request-only types like `StartNetworkMigrationMappingUpdateConstruct`/
`Segment`, `S3BucketSource`, `EnrichmentSource`/`TargetS3Configuration`,
`ChangeServerLifeCycleStateSourceServerLifecycle`, all confirmed
request-only by reading their containing `api_op_*.go` `Input`/`Output`
split): `types.Job.JobID` (via `TerminateTargetInstances`/`StartTest`/
`StartCutover`/`DescribeJobs`) and `types.ParticipatingServer.SourceServerID`
(nested one level inside `Job`) are always populated at job creation
(`jobs.go`'s job-ID generation, `sourceservers.go:709-722`'s
`startBatchJob`) and carry no `omitempty` on gopherstack's `jobWire`/
`participatingServerWire` (wire.go:284-299). `types.TargetNetwork.Topology`,
`types.TargetS3Configuration.{S3Bucket,S3BucketOwner}`, and
`types.SourceConfiguration.{SourceEnvironment,SourceS3Configuration}`
(reachable via `Get`/`Create`/`UpdateNetworkMigrationDefinition`, none of
which have any required field at their own op level, so entirely invisible
to the per-op ranking) are required on `CreateNetworkMigrationDefinitionInput`
too (confirmed via the real SDK's own `validateOpCreateNetworkMigrationDefinitionInput`),
so every stored definition has them, and gopherstack's
`networkMigrationDefinitionWire`/`targetNetworkWire`/`targetS3ConfigurationWire`/
`sourceConfigurationWire` (wire.go:1118-1153) carry no `omitempty` on any of
them either. `types.StorageConfiguration.StorageType` (reachable via
`Create`/`UpdateReplicationConfigurationTemplate`/`GetReplicationConfiguration`/
`UpdateReplicationConfiguration`) and `types.ConnectorSsmCommandConfig.
{CloudWatchOutputEnabled,S3OutputEnabled}` (via `Create`/`UpdateConnector`)
are both non-pointer enum/bool types on the real SDK side (not provable per
this campaign's rule) and, independently, carry no `omitempty` on
gopherstack's wire structs either -- disqualified twice over.
`types.SqlParameter`-equivalent `types.SsmParameterStoreParameter.
{ParameterName,ParameterType}` (via `PutSourceServerAction`/`PutTemplateAction`,
neither required at the op level) round-trip through `ssmParameterWire`
(wire.go:16-18), also no `omitempty`.

0 bugs found; no code changes. Companion services this batch: `scheduler`
(2 bugs -- a wrong-cased wire key hiding `CapacityProviderStrategyItem.
CapacityProvider` entirely, plus a reachably-empty `omitempty` on
`AwsVpcConfiguration.Subnets`; see `services/scheduler/PARITY.md`) and
`redshiftdata` (0 bugs; both came back clean too).

## Implementation summary (this pass)

All 95 operations implemented: routed via a flat method+operation-name dispatch table
(`handler.go`'s `routes()`, merged from 8 per-family `handler_*.go` builder functions, matching
`operationSegment`'s handling of the `/network-migration/` prefix and the `/tags/{resourceArn}`
trio), backed by real `InMemoryBackend` state (`pkgs/store.Table`/`Index` per resource kind, one
coarse `lockmetrics.RWMutex`), and persisted via `Snapshot`/`Restore` (`persistence.go`).
`sdk_completeness_test.go` passes with an empty exception list. A real `aws-sdk-go-v2/service/mgn`
client round-trips against every major flow (`sdk_roundtrip_test.go`, 13 tests) — this caught two
real wire-shape bugs before they shipped (see "Corrected during implementation" below), not after.

### The hard design problem — decision made

**SourceServer/VcenterClient creation**: `StartImport`/`StartExport` are implemented as fully
honest bookkeeping (`ImportTask`/`ExportTask` progress `PENDING -> STARTED -> SUCCEEDED` on the
same deterministic timer every other async resource in this package uses); `StartImport`'s
`Summary` counts are always zero and `StartExport`'s are a real live count of this account's
Applications/Waves/SourceServers — neither ever reads or fabricates real S3 object content, since
no schema for that content exists anywhere in this SDK to derive one from. Given that leaves the
entire 70-op replication surface unreachable through the public API (the actual point of this
service), this package adds `SeedSourceServer` (`sourceservers.go`) and `SeedVcenterClient`
(`vcenterclients.go`): EXPORTED, non-SDK Go functions, reachable only by calling into this package
directly, never routed by `handler.go`, never described as SDK operations in
`GetSupportedOperations()`. They are the explicit, documented emulator decision the task asked
for — this package's own round-trip tests use them to seed a server before exercising StartTest/
StartCutover/FinalizeCutover/etc. A newly seeded SourceServer starts `NOT_READY`/`INITIATING` and
progresses to `READY_FOR_TEST`/`CONTINUOUS` over 3 `asyncTransitionDelay` ticks
(`sourceservers.go`'s `scheduleReplicationLocked`) — the same honest, deterministic, time-based
walk a real `StartImport`-seeded server would need, just reachable without inventing an S3
file-format schema.

**NetworkMigrationExecutionID**: no op creates one; all 5 `StartNetworkMigration*` ops require one
as input. This package's decision: `resolveOrCreateExecutionLocked` (`networkmigrationjobs.go`)
auto-vivifies a `NetworkMigrationExecution` the first time any of the 5 Start* ops references an
(DefinitionID, ExecutionID) pair not previously seen — generalizing the task's own suggested
convention ("minting one automatically the first time StartNetworkMigrationMapping is called") to
all five entry points, since none is more privileged than the others as a creation trigger. Every
subsequent Start* call against the same pair updates that execution's Activity/Stage/Status in
place rather than minting a second record.

**Mapper segments/constructs**: deliberately the OTHER option the task weighed — "leave the
families genuinely empty and record it as a gap" — rather than a third synthetic seeding seam.
`ListNetworkMigrationMapperSegments`/`ListNetworkMigrationMapperSegmentConstructs` always return
empty (after validating the (definition, execution) scope exists);
`GetNetworkMigrationMapperSegmentConstruct`/`UpdateNetworkMigrationMapperSegment` always 404. No
segment is ever produced by any op (there is no real network-analysis engine to produce one from),
and mapper segments back only bookkeeping display within the already-honest-gapped Network
Migration analysis sub-feature, not the primary 70-op replication surface — a smaller payoff than
SourceServer/VcenterClient's seam, so a second `Seed*` convenience was not added for it.

**Network Migration analysis/codegen/deployment content**: `StartNetworkMigrationAnalysis`/
`CodeGeneration`/`Deployment` progress a shared internal `NetworkMigrationJob` bookkeeping record
(one generic type backing all 5 real job-details SDK shapes, which are byte-identical in field
layout — `networkmigrationjobs.go`) through `PENDING -> STARTED -> SUCCEEDED`, and the parent
execution's own `Status` mirrors it on completion. `ListNetworkMigrationAnalysisResults`/
`ListNetworkMigrationCodeGenerationSegments`/`ListNetworkMigrationDeployedStacks` always return
empty `Items`, even after the parent job SUCCEEDS — real analysis findings, generated
infrastructure code, and deployed CloudFormation stacks all require engines this repo does not
have, and PARITY.md's own honest-gap section is explicit that fabricating this content "would
misrepresent what the emulator actually did."

**Two error generations**: implemented as documented, per-op — `requireInitializedLocked` gates
every legacy op (69 of 95, confirmed by direct per-op extraction — see below), never called by the
tagging trio or any `/network-migration/` op; `classifyMGNError` (`handler.go`) maps all 8 wire
exception shapes, but `errAccessDenied`/`errQuotaExceeded`/`errThrottling` are never actually
constructed by any call site in this pass (no permission/quota/rate-limiter model exists to trigger
them honestly) — documented explicitly in `errors.go` as a real, deliberate gap rather than a
forced fake trigger just to exercise the constructor.

**Flattened-vs-nested outputs**: `sourceServerWire`/`toSourceServerWire` back the 11
flattened-SourceServer ops; `StartTest`/`StartCutover`/`TerminateTargetInstances` return a nested
`jobEnvelope{Job: ...}` instead (`handler_sourceservers.go`); `GetLaunchConfiguration`/
`GetReplicationConfiguration` flatten their own distinct wire shapes with no backing named SDK type
at all, matching internal `LaunchConfiguration`/`ReplicationConfiguration` types this package
invented for exactly that purpose (`models.go`).

**EC2 cutover scope**: narrowed, explicitly. `LaunchedInstance.Ec2InstanceID` is a synthetic,
gopherstack-format ID (`"i-"` + hex, `jobs.go`'s `newSyntheticInstanceID`), never cross-checked
against a real `services/ec2` instance. Real EC2 instance launch on cutover (the directconnect
precedent this task asked to weigh) was assessed and NOT done this pass — flagged explicitly as a
follow-on, not silently skipped.

### Corrected during implementation

1. **`UpdateNetworkMigrationMapperSegmentInput` has only one mutable field, `ScopeTags`** — the
   pre-implementation audit's family-M table said "rest optional (ScopeTags, TargetAccount)",
   but direct re-read of the real Input struct during implementation found no `TargetAccount`
   field on it at all (confirmed: `TargetAccount` only appears on the *segment's own* stored shape,
   never as an input the caller can set via this op) — documented at `networkmigration.go`'s
   `UpdateNetworkMigrationMapperSegment`.
2. **`types.ManagedAccount.AccountId` wire-serializes as `"accountId"` (lowercase `d`), not
   `"accountID"`** — every other `AccountID`-suffixed field in this SDK (69 legacy ops' own
   `AccountID *string`) is the Go field `AccountID` (capital ID) wire-keyed `"accountID"`, but
   `ManagedAccount`'s field is spelled `AccountId` (lowercase `d`) in the Go source itself, which
   this SDK's own lowercase-first-rune convention serializes to `"accountId"`. First caught by
   `TestRoundTrip_ManagedAccounts` failing against the real SDK client, not by static inspection —
   exactly the class of bug this campaign's SDK-round-trip-test standard exists to catch. Fixed in
   `wire.go`'s `managedAccountWire`.

### Judgment calls, each documented at its call site

- ARN resource-path segments (`store.go`'s ARN builders) are this package's best-effort,
  UNCONFIRMED guesses from AWS naming convention (e.g. `"source-server/"`, `"application/"`,
  `"vcenter-client/"`) — Terraform's AWS provider has zero MGN resources to corroborate against
  (confirmed by the pre-implementation audit), so unlike directconnect/outposts there is no
  Terraform-source cross-check available at all for any of the 12 taggable kinds.
- ID formats (`store.go`'s `new*ID` functions) are similarly UNCONFIRMED hex-suffix conventions
  (e.g. `"s-"` + 16 hex for SourceServer) — no doc-comment ID-shape examples exist anywhere in this
  SDK module to confirm against, unlike directconnect's own published `"dxcon-ffabc123"` examples.
- `LifeCycleState`/`DataReplicationState` transition tables (`sourceservers.go`'s package doc
  comment) are this package's own inference from field/enum semantics, not independently
  SDK-confirmed — AWS's real state machine is not published anywhere in this SDK.
- Application/Wave `AggregatedStatus` rollup rules (`applications.go`'s `rollupHealthStatus`/
  `rollupProgressStatus`, `waves.go`'s analogous pair) are documented, invented aggregation rules
  (e.g. a Wave is `LAGGING` if any member Application is `LAGGING`), not SDK-specified.
- Job progression (`jobs.go`) is a deterministic, always-succeeds 4-tick simulation — no
  `JobLogEvent` describing failure/skip/cancel is ever emitted, since no real launch engine exists
  to fail; `JobStatus` itself has no `FAILED` value at all, confirmed by direct SDK read.
- Timestamps: every SDK member typed as a `*string` "DateTime"-suffixed field (confirmed via direct
  read to deserialize via a bare `value.(string)` type assertion, not `smithytime`) is wire-coded as
  an RFC3339 string by this package's own convention (`store.go`'s `nowRFC3339`); every real smithy
  `*time.Time` field (the Network Migration family's `CreatedAt`/`UpdatedAt`/`EndedAt`) is
  epoch-seconds via `pkgs/awstime.Epoch`, confirmed against the SDK's own
  `smithytime.ParseEpochSeconds` deserializer call.

### Gate results (see final report for full output)

`go build ./...`, `go vet ./...`, `go vet -tags e2e ./test/e2e/...`, `gofmt -l`, and
`golangci-lint run ./services/mgn/... .` (0 issues) all clean. `go test -race -count=1
./services/mgn/...` run 3 times, all 3 clean. `grep -rnE '//nolint:.*(funlen|gocyclo|gocognit|cyclop)'
services/mgn/` empty — every complexity issue golangci-lint's `cyclop`/`gocognit` flagged during
this pass (`UpdateLaunchConfigurationTemplate`, `scheduleJobLocked`, `scheduleReplicationLocked`)
was fixed by decomposing into named helper functions, never suppressed.

## gopherstack-i6oz follow-up pass (2026-08-01): StartImport made wire-reachable

Tracked as `bd` issue `gopherstack-i6oz`, filed against the gap this service's `overall: B` grade
above cited by name: no AWS-wire operation created a `SourceServer`, so anyone driving gopherstack
through the real AWS CLI/SDK/Terraform (as opposed to calling into this Go package directly) could
not exercise MGN's actual 70-op replication surface at all — `SeedSourceServer`/`SeedVcenterClient`
only helped in-process callers and this package's own tests. User decision (2026-08-01): implement
the wire-reachable path with a best-guess CSV schema rather than continuing to block on AWS's
unpublished real one; keep scope small ("niche, do not over-engineer").

### What changed

- **`StartImport` now really reads S3.** `s3import.go` adds `S3Accessor` (a 1-method interface —
  just `GetObject`, since StartImport only ever reads a single object — satisfied by the in-process
  S3 backend, same cross-service pattern `services/dynamodb`'s `ImportTable`/
  `ExportTableToPointInTime` already use for their own `S3Accessor`) plus `SetS3Backend`/
  `s3Backend()`. **cli.go wiring applied by a later pass** (this pass could not touch `cli.go` —
  another agent held it at the time) — see "cli.go wiring" below for what was added and how it was
  verified.
- **A documented, best-effort CSV schema**, since AWS does not publish StartImport's real column set
  anywhere in this SDK (`types.SourceServer`/`types.SourceProperties` are the wire OUTPUT shape, not
  an input format). Header row required; the ONLY required column is `hostname`
  (`IdentificationHints.Hostname`). Optional columns, each mapped onto a real field this backend's
  `SourceServer`/`SourceProperties` already models (`models.go`) rather than an invented concept:
  `fqdn` (`IdentificationHints.Fqdn`), `userProvidedID` (`SourceServer.UserProvidedID`),
  `operatingSystem` (`Os.FullString`), `recommendedInstanceType`, `cpuCores`/`cpuModelName` (one
  `CPU` entry), `ramBytes`, `diskDeviceName`/`diskBytes` (one `Disk` entry, defaulting device name to
  `/dev/sda1` — the same default `createSourceServerLocked` itself falls back to), and
  `networkInterfaceMac`/`networkInterfaceIPs` (one, `IsPrimary: true`, `NetworkInterface` entry; IPs
  semicolon-separated, e.g. `"10.0.0.5;10.0.0.6"`). One CPU/Disk/NetworkInterface entry per row, not
  the full multi-value arrays a real per-server inventory tool might report — CSV's one-row-one-
  record shape does not naturally carry repeating groups without a much richer, still-unpublished
  convention, and a single entry is the smaller, defensible shape the task asked for. No
  `ApplicationID` column: real AWS assigns Application membership via the separate
  `AssociateSourceServers` call, never as part of `StartImport` itself, so this schema does not
  invent one either.
- **Real counts, real per-row errors, never fabricated.** `ImportTaskSummary.Servers.CreatedCount`
  is the actual number of rows that parsed and created a `SourceServer` (`ModifiedCount` stays
  always zero — this emulator has no natural key to detect "this row re-describes a previously-
  imported server," an honest, documented simplification). A malformed row (empty `hostname`, or a
  numeric column present but unparseable) is never silently dropped nor counted as created: it is
  recorded as a real `types.ImportTaskError` (new internal `ImportTaskError`/`ImportErrorData`
  types, `models.go`; new wire types, `wire.go`/`wire_convert.go`) and surfaced by `ListImportErrors`
  — which itself moves from "always empty" to genuinely returning per-row failures, since
  `ImportErrorData.RowNumber`/`RawError` are real AWS-defined fields seemingly designed for exactly
  this row-based-format use case (strong incidental corroboration for the CSV-schema decision, found
  while reading the SDK's own `types.go` for this pass). `ErrorType` is `VALIDATION_ERROR` for a
  malformed row, `PROCESSING_ERROR` for a whole-object read/parse failure — both real
  `types.ImportErrorType` wire values, confirmed by direct SDK read, never invented.
- **Whole-object failure is honest, never a fabricated success.** A missing S3 backend, a missing
  bucket/key (`GetObject` failing), or an object with no parseable header row all fail the entire
  `ImportTask` (`Status` -> `FAILED`, one `ImportTaskError` recorded describing why) rather than
  reporting zero-but-successful, matching the task's explicit requirement.
- **`SeedSourceServer` removed** (`sourceservers.go`). Once `StartImport` itself became a genuine
  creation path, this package's own round-trip tests were confirmed to be `SeedSourceServer`'s ONLY
  remaining consumer (`grep -rn SeedSourceServer` across the repo, non-test files: doc comments
  only) — so tests were rewritten to drive the real wire path instead
  (`seedSourceServerViaImport`, `sdk_roundtrip_helper_test.go`, using a minimal in-test `mockS3`
  rather than the full `services/s3` backend, the same lightweight-mock pattern
  `services/dynamodb`'s own `import_export_s3_test.go` already uses for its `S3Accessor`). This
  removes the exact divergence ("SourceServers exist because Go code called a non-SDK function, not
  because of anything AWS-shaped") the original `B` grade was docked for, and was preferred over
  keeping the seam per the task's own explicit guidance once tests were confirmed to be the only
  consumer. The internal creation logic itself was NOT deleted — it was renamed/refactored to
  `createSourceServerLocked` (`sourceservers.go`), now StartImport's own single creation path.
- **`SeedVcenterClient` (`vcenterclients.go`) KEPT, unchanged.** No import (or any other public
  creation) operation exists for `VcenterClient` anywhere in this 95-op surface — StartImport's CSV
  schema is SourceServer-specific (real AWS's own `ImportTaskSummary` has no `VcenterClients` count
  field at all, confirming this). `SeedVcenterClient` remains this emulator's only way to get a
  `VcenterClient` into the backend at all, exactly the "no import path exists" case the task
  flagged as the reason to keep it.

### cli.go wiring (now applied and verified end-to-end)

`services/mgn/InMemoryBackend.SetS3Backend(S3Accessor)` is now actually called from `cli.go`.
`wireMGNS3` was added next to `wireDynamoDBS3` (same one-function-plus-one-call pattern, same
`mgnbackend`/`s3backend` imports `wireTaggingMGN`/`wireDynamoDBS3` already used — no new import
needed) and is called from `wireStorageAndSecretsIntegrations` as `wireMGNS3(byName["MGN"],
byName["S3"])`, alongside `wireDynamoDBS3`.

Verified end-to-end, not just by compiling: a throwaway test (written outside this package, run
once, then deleted — never committed) called this repo's own `initializeServices` /
`wireCrossServiceDependencies` / `serviceByName` exactly as the real running server does, pulled
out the resulting `*mgnbackend.Handler` and `*s3backend.S3Handler`, created a real bucket and put a
real CSV object via the S3 backend's own `CreateBucket`/`PutObject` (real `aws-sdk-go-v2/service/s3`
input types, not a mock), called `InitializeService()` then `StartImport` against that object, and
polled `DescribeSourceServers` until real `SourceServer`s appeared. It passed: two `SourceServer`s
were created from the two CSV rows, confirming `wireMGNS3` binds the exact `S3Accessor` interface
this package's own round-trip tests already exercise — the wire-reachability gap this service was
originally docked a full letter grade for is now closed both in this package's code AND in the
actual application wiring a real caller (AWS CLI/SDK/Terraform) goes through.

### Gate results (this follow-up pass)

`go build ./services/mgn/...`, `go vet ./services/mgn/...`, `gofmt -l services/mgn/` (empty),
`golangci-lint run ./services/mgn/...` (0 issues), and `grep -rnE
'//nolint:.*(funlen|gocyclo|gocognit|cyclop)' services/mgn/` (empty) all clean. `go test -race
-count=1 ./services/mgn/...` run 3 times, all 3 clean. `go build ./...`/`go vet ./...` at the
whole-repo level fail, but only in `services/networkmanager/` — pre-existing, unrelated, uncommitted
work from another concurrently-running agent (this pass never touched anything outside
`services/mgn/`).

### Gate results (cli.go-wiring pass)

Full whole-repo gates, run after adding `wireMGNS3` to `cli.go` (the only file this pass touched
outside `services/mgn/`): `go build ./...`, `go vet ./...`, `go vet -tags e2e ./test/e2e/...`,
`gofmt -l services/mgn/ cli.go` (empty), `golangci-lint run ./services/mgn/... .` (0 issues), and
`grep -rnE '//nolint:.*(funlen|gocyclo|gocognit|cyclop)' services/mgn/` (empty) all clean —
`services/networkmanager/` no longer fails, since the concurrent work referenced above had landed by
this point. `go test -race -count=1 ./services/mgn/...` run 3 times, all 3 clean. The end-to-end
wiring verification described above in "cli.go wiring" passed on its first run.

## 2026-08-06 pass (gopherstack-xd34): integration suite + gap closures, A- -> A

Three-part mandate: (1) add the SDK-driven integration suite this service had zero of, the primary
A-/A- blocker per gopherstack-r9yz; (2) close every reachable gap, including cross-service validation
against this emulator's own ec2/organizations backends; (3) move only genuinely-underivable gaps to
`structural_gaps:`.

### Integration suite (`test/integration/mgn_test.go`)

9 test functions, following `test/integration/accessanalyzer_test.go`'s harness exactly
(`createMGNClient`, static test/test creds, `o.BaseEndpoint`, `dumpContainerLogsOnFailure`,
`t.Context()`, a `mgnCleanupCtx()` helper for `t.Cleanup` bodies since Go 1.24+ cancels `t.Context()`
before cleanups run):

- `TestIntegration_MGN_SourceServerLifecycle` — real StartImport (real S3 bucket/object via
  `createS3Client`, read through the actual `wireMGNS3` cross-service binding, not a mock) ->
  DescribeSourceServers -> UpdateSourceServer -> ChangeServerLifeCycleState -> DisconnectFromService
  -> DeleteSourceServer. Sequential: each step consumes the previous step's state.
- `TestIntegration_MGN_ConfigurationTemplateLifecycle` — tables Create -> Describe -> Update -> Delete
  across LaunchConfigurationTemplate/ReplicationConfigurationTemplate (2 cases): same CRUD shape,
  different resource, merged from two near-duplicate sequential functions into one table mid-pass once
  the duplication was pointed out.
- `TestIntegration_MGN_JobLifecycle` — the highest-value case: StartTest through to a COMPLETED Job,
  then a **real `ec2sdk.DescribeInstances` call** against the participant's `LaunchedEc2InstanceID`,
  proving the cross-service EC2 launch (see below) actually produced a real instance, not just a
  well-formed-looking ID. Then TerminateTargetInstances and confirms `LaunchedInstance` clears.
- `TestIntegration_MGN_ApplicationsAndWaves` — CreateApplication/CreateWave -> AssociateApplications ->
  DisassociateApplications -> DeleteWave/DeleteApplication (disassociate must precede delete, per
  `waveHasApplicationsLocked`/`applicationHasServersLocked`'s guards). Sequential.
- `TestIntegration_MGN_Tagging` — tables TagResource/ListTagsForResource/UntagResource across 5 of the
  12 taggable resource kinds (source server, application, wave, launch configuration template,
  connector), each case independently creating its own resource.
- `TestIntegration_MGN_NotFoundErrors` — tables 6 ops against unknown IDs, asserting a real
  `ResourceNotFoundException` wire code via `awsErrorCode`.
- `TestIntegration_MGN_ValidationErrors` — tables 3 real server-side validation failures. (A 4th
  candidate, "StartImport missing s3Bucket", doesn't reach the server at all: the SDK's own
  `validateS3BucketSource` rejects a nil `S3Bucket`/`S3Key` client-side before the request is ever
  sent, confirmed by reading `validators.go` — the table case instead uses an empty-string `S3Bucket`,
  which passes client-side validation and is caught by this backend's own server-side check.)
- `TestIntegration_MGN_NetworkMigration` — CreateNetworkMigrationDefinition -> StartNetworkMigrationMapping
  (auto-vivifying an execution) -> ListNetworkMigrationExecutions -> GetNetworkMigrationMapperSegmentConstruct
  confirmed 404 (the structural mapper-segment gap, proven live, not just asserted in prose).
- `TestIntegration_MGN_ListManagedAccounts` — the new Organizations cross-service wiring (below),
  proven against a real Organizations backend: CreateOrganization (tolerating
  `AlreadyInOrganizationException`, since the org is shared account-wide state) -> CreateAccount ->
  the new member account ID appears in MGN's own ListManagedAccounts.

Run against a real Docker container (`make build-linux && go test -race -count=1 -run
TestIntegration_MGN ./test/integration/...`): all 9 pass.

### Gaps closed

1. **StartImport's CSV schema was fully invented** (flat `hostname,fqdn,cpuCores,...` columns with no
   AWS provenance whatsoever). AWS's SDK module itself publishes none (`StartImportInput` carries only
   an opaque `S3BucketSource`), but AWS's MGN User Guide does document the real parameter set
   (`docs.aws.amazon.com/mgn/latest/ug/import-main.html`'s "Import parameters" table, fetched and
   quoted verbatim this pass): every column is a `mgn:<resource>:<field>` namespaced key.
   `s3import.go` was rewritten around this real convention: `mgn:server:user-provided-id` and
   `mgn:server:fqdn-for-action-framework` are the two AWS-tabulated columns this pass implements;
   `mgn:server:hostname`/`fqdn`/`aws-instance-id`/`vmware-uuid`/`vmpath` extend the same confirmed
   naming convention onto the SDK's own real `IdentificationHints` fields for the identification
   requirement AWS's docs state in prose ("must include either the server IP address, or the FQDN")
   but never formally tabulate; `mgn:server:tag:<key>` is real and dynamic. All CPU/RAM/disk/network-
   interface columns were deleted outright: AWS's own documented table has zero hardware-inventory
   columns (that data comes from the replication agent, not the import file) — their presence in the
   old schema was pure fabrication. `mgn:app:*`/`mgn:wave:*`/`mgn:launch:*` (implicit Application/Wave
   creation, per-row LaunchConfiguration overrides) are real, doc-confirmed parameters this pass did
   not implement — a materially larger feature (this backend's `LaunchConfiguration` type has no
   fields at all for most of the `mgn:launch:*` sub-parameters) left as an explicit, proportionate
   scope decision (`gaps:`).
2. **ModifiedCount was hardcoded zero.** AWS's own docs describe `mgn:server:user-provided-id` as
   "used by MGN to consistently recognize the server replication, and avoid duplication when importing
   inventory from a CSV file" — exactly the natural key the prior pass said didn't exist.
   `resolveSourceServerByUserProvidedIDLocked`/`applyImportRowLocked` (sourceservers.go) now dedup on
   it: a re-imported row with a matching `UserProvidedID` updates the existing `SourceServer`
   (`ModifiedCount`) instead of always creating a new one (`CreatedCount`). Verified end to end
   (`TestStartImport_ModifiedCount`, two real `StartImport` calls) and live over Docker.
3. **StartTest/StartCutover minted a synthetic, non-cross-checked EC2 instance ID.** New
   `cross_service.go` (services/grafana's `SetAppConfig`/lazy-sibling-resolution pattern) resolves the
   emulator's own `services/ec2` backend and calls its real `RunInstances` on Job completion, resolving
   AMI/instance type from the source server's `LaunchConfiguration.Ec2LaunchTemplateID` when it names a
   real EC2 launch template, else the EC2 backend's own stub AMI catalogue plus a documented default
   instance type (`t3.medium` — real MGN right-sizes from source CPU/RAM via
   `TargetInstanceTypeRightSizingMethod`, an algorithm this emulator doesn't model). Falls back to the
   prior synthetic ID only when EC2 isn't wired (unit tests) or `RunInstances` itself fails. `provider.go`
   now calls `backend.SetAppConfig(ctx.Config)` — no `cli.go` edit needed, since `ctx.Config` is already
   populated generically for every service.
4. **UpdateSourceServer silently dropped `FqdnForActionFramework`/`UserProvidedID`** — a real bug the
   integration suite caught directly (not something this pass set out to fix): the wire request struct
   parsed only `connectorAction`, and the backend method's signature only accepted a
   `*SourceServerConnectorAction`, with no way to pass the other two real wire fields at all. Fixed:
   `updateSourceServerRequest` (wire.go) now parses all three real fields (confirmed against
   `serializers.go`'s `awsRestjson1_serializeOpDocumentUpdateSourceServerInput`); the backend's new
   `SourceServerUpdate` (sourceservers.go) applies each field only when present, fixing a second latent
   bug in the same op — `ConnectorAction` was previously applied unconditionally, silently clearing it
   on every update that didn't re-send it. `Platform` is parsed off the wire and intentionally dropped:
   the real SDK's own `SourceServer`/`SourceProperties` output has no `Platform` field to read it back
   from either.
5. **ListManagedAccounts always returned only the caller's own account.** `cross_service.go` extends
   the sibling-resolution pattern to the Organizations backend: `resolveManagedAccountsLocked` returns
   every real account in this account's AWS Organizations organization when this account is the
   org's management account or a registered delegated administrator for `mgnServicePrincipal`
   (`"mgn.amazonaws.com"` — not confirmed against any published AWS source; follows the
   `<endpoint-prefix>.amazonaws.com` convention botocore's `service-2.json` confirms for MGN's
   `endpointPrefix` ("mgn"), the same best-effort evidentiary standard this file already applies to
   MGN's ARN resource-path segments), falling back to just the caller's own account otherwise.

### Judged structural, moved to `structural_gaps:`

`CreateSourceServer`/`CreateVcenterClient` absence, `NetworkMigrationExecutionID` creation absence,
and Network Migration analysis/codegen/deployment/mapper-segment CONTENT — each individually justified
in `structural_gaps:` above (no public creation op exists in the 95-op surface for the first two; no
analysis/codegen/deployment engine exists in this repo for the third, and none could without either
building one or fabricating output). None of these are new findings — they were already correctly
implemented as honest gaps by the original pass; this pass's contribution is reclassifying them per
`services/_PARITY_TEMPLATE.md`'s `structural_gaps:` convention (added to this file for the first time
this pass) rather than leaving them in `gaps:`, where the "every buildable gap closed" A-grade bar
would otherwise misread them as unfinished work.

### Gate results (this pass)

`go build ./...`, `go vet ./...` (whole repo, including concurrently-modified `services/outposts/`)
clean. `gofmt -l services/mgn/ test/integration/mgn_test.go` empty. `golangci-lint run
./services/mgn/...` and `./test/integration/...` (mgn_test.go) both 0 issues. `grep -rnE
'//nolint:.*(funlen|gocyclo|gocognit|cyclop)' services/mgn/ test/integration/mgn_test.go` — the only
hits are prose mentions inside this file, no actual directives. `go test -race -count=1
./services/mgn/...` run 3 times, all 3 clean. `make build-linux && go test -race -count=1 -run
TestIntegration_MGN ./test/integration/...` — all 9 integration test functions pass against a real
Docker container, run twice for confirmation.

## 2026-08-13 pass (gopherstack-l5ir): route reachability audit -- zero mismatches

All 95 real mgn ops were extracted from `mgn@v1.48.4` serializers.go (`request.Method` +
`httpbinding.SplitURI(...)` in each op's `awsRestjson1_serializeOp<Op>.HandleSerialize`) and diffed
mechanically against `handler_routes.go`'s flat `routeKey` table -- the same method that found 35
routing bugs in cloudfront (gopherstack-o31x) and 22 in opensearch (this same pass). mgn came back
clean: **zero mismatches across all 95 ops**, including the three that share a path
(`ListTagsForResource`/`TagResource`/`UntagResource`, all `/tags/{resourceArn}`, correctly
disambiguated by method -- GET/POST/DELETE, not a query-parameter discriminator) and the 25 ops
namespaced under `/network-migration/`, whose `operationSegment` prefix-strip
(`handler.go`'s doc comment) correctly recovers the operation name in both cases.

The likely reason: mgn's router is a flat `map[string]routeEntry` keyed by `"<METHOD> <opSegment>"`
(`handler_routes.go`), built directly from a mechanical `<OperationName>` == path-segment convention
this SDK uses almost universally (92 of 95 ops are literal `POST /<OperationName>`). That shape has no
room for the suffix-matching/nested-dispatch mistakes that produced most of opensearch's and
cloudfront's bugs -- there is no hand-written suffix parsing to get wrong.

Added as a permanent regression test, `TestExtractOperation_SDKRouteTable` in
`handler_paths_sdk_diff_test.go` (one subtest per op, 95/95 pass) -- this converts the one-off audit
into a standing guarantee rather than a report. No routing code changes were needed; only the new
test file. No existing test encoded a wrong path (nothing needed correcting).

## Purpose of this document

`services/mgn/` does not exist. This file is a pre-implementation audit: a complete SDK operation
inventory plus a behavioral spec, written so a follow-up implementation pass does not have to
re-derive wire shapes from the SDK source itself. No `.go` files were touched to produce it. All 95
operation names, the wire protocol, every operation's exact per-op exception set, and every
shared type/enum below were read directly from `aws-sdk-go-v2/service/mgn@v1.48.3`'s
`serializers.go` / `deserializers.go` / `types/types.go` / `types/enums.go` / `types/errors.go` /
individual `api_op_*.go` files in the module cache (resolved via a throwaway `go mod init probe &&
go get .../mgn@latest` in the scratch dir — **not** added to this repo's `go.mod`, which another
agent was concurrently editing during this pass).

## 1. Complete SDK operation inventory

**95 operations**, SDK version **`v1.48.3`** (resolved 2026-08-01, whatever `@latest` currently
resolves to — not a version pinned by this audit). This matches the task's ~95 estimate exactly:

`ls api_op_*.go | grep -v _test.go | wc -l` against
`/home/agbishop/go/pkg/mod/github.com/aws/aws-sdk-go-v2/service/mgn@v1.48.3/` returns **95**.

Alphabetically: ArchiveApplication, ArchiveWave, AssociateApplications, AssociateSourceServers,
ChangeServerLifeCycleState, CreateApplication, CreateConnector, CreateLaunchConfigurationTemplate,
CreateNetworkMigrationDefinition, CreateReplicationConfigurationTemplate, CreateWave,
DeleteApplication, DeleteConnector, DeleteJob, DeleteLaunchConfigurationTemplate,
DeleteNetworkMigrationDefinition, DeleteReplicationConfigurationTemplate, DeleteSourceServer,
DeleteVcenterClient, DeleteWave, DescribeJobLogItems, DescribeJobs,
DescribeLaunchConfigurationTemplates, DescribeReplicationConfigurationTemplates,
DescribeSourceServers, DescribeVcenterClients, DisassociateApplications,
DisassociateSourceServers, DisconnectFromService, FinalizeCutover, GetLaunchConfiguration,
GetNetworkMigrationDefinition, GetNetworkMigrationMapperSegmentConstruct,
GetReplicationConfiguration, InitializeService, ListApplications, ListConnectors,
ListExportErrors, ListExports, ListImportErrors, ListImportFileEnrichments, ListImports,
ListManagedAccounts, ListNetworkMigrationAnalyses, ListNetworkMigrationAnalysisResults,
ListNetworkMigrationCodeGenerations, ListNetworkMigrationCodeGenerationSegments,
ListNetworkMigrationDefinitions, ListNetworkMigrationDeployedStacks,
ListNetworkMigrationDeployments, ListNetworkMigrationExecutions,
ListNetworkMigrationMapperSegmentConstructs, ListNetworkMigrationMapperSegments,
ListNetworkMigrationMappingUpdates, ListNetworkMigrationMappings, ListSourceServerActions,
ListTagsForResource, ListTemplateActions, ListWaves, MarkAsArchived, PauseReplication,
PutSourceServerAction, PutTemplateAction, RemoveSourceServerAction, RemoveTemplateAction,
ResumeReplication, RetryDataReplication, StartCutover, StartExport, StartImport,
StartImportFileEnrichment, StartNetworkMigrationAnalysis, StartNetworkMigrationCodeGeneration,
StartNetworkMigrationDeployment, StartNetworkMigrationMapping,
StartNetworkMigrationMappingUpdate, StartReplication, StartTest, StopReplication, TagResource,
TerminateTargetInstances, UnarchiveApplication, UnarchiveWave, UntagResource, UpdateApplication,
UpdateConnector, UpdateLaunchConfiguration, UpdateLaunchConfigurationTemplate,
UpdateNetworkMigrationDefinition, UpdateNetworkMigrationMapperSegment,
UpdateReplicationConfiguration, UpdateReplicationConfigurationTemplate, UpdateSourceServer,
UpdateSourceServerReplicationType, UpdateWave.

### Protocol and routing shape

Protocol is **REST-JSON** (`awsRestjson1_serializeOp<Op>` struct names throughout
`serializers.go`, 95 `HandleSerialize` methods, one per op — confirmed by direct extraction, not
sampled). Unlike a typical REST-JSON service, **every path is an action-style slug, not a resource
path with parameters** — extracted via a Python regex pass over every `HandleSerialize` method body
(all 95 matched):

- **92 of 95 ops are `POST /<OperationName>`** (e.g. `POST /CreateApplication`, `POST
  /StartCutover`) — the operation name IS the path, with zero `{param}` placeholders anywhere
  except the three tagging ops below. This is close to (but not identical to) the awsjson1.1
  action-dispatch shape directconnect uses (`POST /` with an `X-Amz-Target` header) — here the
  action name is IN the path instead of a header, so a gopherstack router can dispatch purely on
  path suffix.
- **`DescribeVcenterClients` is `GET /DescribeVcenterClients`** — the only non-tagging op that is
  not a POST (confirmed by direct extraction of its `HandleSerialize` body; every sibling `Describe*`
  op in this service, e.g. `DescribeSourceServers`/`DescribeJobs`/`DescribeJobLogItems`/
  `DescribeLaunchConfigurationTemplates`/`DescribeReplicationConfigurationTemplates`, is `POST`).
  Do not assume all `Describe*`-named ops share one HTTP method.
- **The tagging trio uses a real resource path**: `GET /tags/{resourceArn}`
  (`ListTagsForResource`), `POST /tags/{resourceArn}` (`TagResource`), `DELETE
  /tags/{resourceArn}` (`UntagResource`) — the only three ops in the entire service with a path
  parameter.
- **25 ops are namespaced under `/network-migration/<OperationName>`** rather than the bare
  `/<OperationName>` every other op uses — see the Network Migration family tables below for the
  full list. Two of those 25 (`StartImportFileEnrichment`, `ListImportFileEnrichments`) are
  semantically about import-file processing, not network topology, yet still live under the
  `/network-migration/` prefix — a naive router keying purely on "does this look like an import
  op" would misroute these two.

### Errors — 8 shared exception shapes, richer than directconnect's 5

All 8 in `types/errors.go`, confirmed by reading the file directly:

- **`AccessDeniedException`** {`Message`, `Code`} — client fault. "Operating denied due to a file
  permission or access check error."
- **`ConflictException`** {`Message`, `Code`, `ResourceId`, `ResourceType`, `Errors
  []ErrorDetails`} — client fault, the richest shape (carries a list of nested `ErrorDetails`, not
  just a flat message).
- **`InternalServerException`** {`Message`} — server fault. Only appears in the tagging trio's
  error set (see below) — no other op in this 95-op service can return it.
- **`ResourceNotFoundException`** {`Message`, `Code`, `ResourceId`, `ResourceType`} — client fault.
- **`ServiceQuotaExceededException`** {`Message`, `Code`, `ResourceId`, `ResourceType`,
  `ServiceCode`, `QuotaCode`, `QuotaValue *int32`} — client fault.
- **`ThrottlingException`** {`Message`, `ServiceCode`, `QuotaCode`, `RetryAfterSeconds *string`} —
  client fault, appears only on the tagging trio and the 25 `/network-migration/` ops, never on any
  legacy (non-network-migration) op — a real, structural split (see below).
- **`UninitializedAccountException`** {`Message`, `Code`} — client fault. "Uninitialized account
  exception" — appears on almost every legacy op (69 of 95), meaning most of this service assumes
  `InitializeService` has already been called for the caller's account; **zero** of the 25
  `/network-migration/` ops and zero of the tagging trio ever return it.
- **`ValidationException`** {`Message`, `Code`, `Reason ValidationExceptionReason`, `FieldList
  []ValidationExceptionField`} — client fault. `ValidationExceptionReason` has 4 values:
  `unknownOperation`, `cannotParse`, `fieldValidationFailed`, `other` (note: lower-camelCase wire
  values, not `SCREAMING_SNAKE_CASE` like every other enum in this service — verified directly in
  `types/enums.go`, not a transcription error in this note).

**Two structurally distinct error-set "generations" exist in this one service**, extracted per-op
from every `awsRestjson1_deserializeOpError<Op>` switch body in `deserializers.go` (all 95 read
individually, not sampled from the shared `types/errors.go` list):

1. **Legacy MGN ops** (everything except tagging and `/network-migration/`): draw from
   `{AccessDenied, Conflict, ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount,
   Validation}` — never `InternalServerException` or `ThrottlingException`.
2. **Tagging trio + all 25 `/network-migration/` ops**: draw from `{AccessDenied, Conflict,
   InternalServer (tagging only), ResourceNotFound, ServiceQuotaExceeded, Throttling, Validation}`
   — never `UninitializedAccountException`. This strongly suggests the Network Migration feature
   was added later, on a newer internal service-generation template that dropped the
   init-required check and added throttling, while the tagging trio (also common to add later)
   picked up the same newer error conventions.

Per-op exact sets are given in each family table below (every one of the 95 read directly, not
inferred from this generational split — the split is a useful mnemonic, not a substitute for the
per-op ground truth).

### Wire-shape traps worth flagging up front (looks-wrong-but-correct, or just easy to miss)

1. **Every per-`SourceServer`-mutation op flattens the FULL `SourceServer` shape (13 fields: 
   `ApplicationID`, `Arn`, `ConnectorAction`, `DataReplicationInfo`, `FqdnForActionFramework`,
   `IsArchived`, `LaunchedInstance`, `LifeCycle`, `ReplicationType`, `SourceProperties`,
   `SourceServerID`, `Tags`, `UserProvidedID`, `VcenterClientID`) directly onto its own Output
   struct** — confirmed by reading `UpdateSourceServer`, `UpdateSourceServerReplicationType`,
   `ChangeServerLifeCycleState`, `FinalizeCutover`, `MarkAsArchived`, `DisconnectFromService`,
   `StartReplication`, `StopReplication`, `PauseReplication`, `ResumeReplication`,
   `RetryDataReplication` output structs directly (11 ops, byte-identical field list each time; no
   `types.SourceServer` struct is ever nested). By contrast, **`StartTest`, `StartCutover`, and
   `TerminateTargetInstances` instead return a nested `Job *types.Job`** — these three are the only
   source-server-adjacent mutations that produce an async Job rather than directly mutating and
   echoing the SourceServer. A generic "serialize the SourceServer as this op's response" helper
   is correct for 11 ops and actively wrong for these 3 — same class of trap as directconnect's
   flattened-vs-nested VirtualInterface issue.
2. **`GetLaunchConfiguration`/`UpdateLaunchConfiguration` and
   `GetReplicationConfiguration`/`UpdateReplicationConfiguration` each flatten their respective
   per-server configuration directly onto the Output struct too — there is NO `types.LaunchConfiguration`
   or `types.ReplicationConfiguration` struct anywhere in this SDK module** (confirmed: `grep -n
   "^type LaunchConfiguration struct\|^type ReplicationConfiguration struct" types/types.go` returns
   nothing; only `LaunchConfigurationTemplate` and `ReplicationConfigurationTemplate` exist as named
   types). An implementer needs a distinct internal representation for "this source server's launch
   configuration" that happens to share most field names with `LaunchConfigurationTemplate` but is
   never the same wire type.
3. **`DescribeVcenterClients` is the only non-tagging `GET`** — see routing section above. A
   router that assumes "every op in this service is POST" (a reasonable inference from the other
   94) will 405 this one specifically.
4. **`StartImportFileEnrichment`/`ListImportFileEnrichments` are wire-routed under
   `/network-migration/` despite being conceptually part of the Export/Import family** (they
   enrich an *import file*, i.e. the same CSV/JSON source-server-inventory files `StartImport`
   consumes, with additional discovered network/segment metadata for the Network Migration
   feature to consume downstream) — grouping them with `StartImport`/`ListImports` by name-pattern
   alone would misroute them.
5. **`AccountID` (optional, "act on behalf of a delegated/managed account") appears on almost every
   legacy per-source-server/job/wave/application op but is completely absent from
   `LaunchConfigurationTemplate`/`ReplicationConfigurationTemplate`/`Connector`/`VcenterClient` ops
   and from all 25 `/network-migration/` ops** — confirmed via `grep -L AccountID api_op_*.go`
   (42 files with no `AccountID` field at all, cross-checked against the family tables below). Do
   not assume every op accepts this field.
6. **`ValidationExceptionReason`'s wire values are lower-camelCase** (`unknownOperation`,
   `cannotParse`, `fieldValidationFailed`, `other`) while literally every other enum in this
   service (`LifeCycleState`, `JobStatus`, `DataReplicationState`, ...) is `SCREAMING_SNAKE_CASE`
   — verified directly in `types/enums.go`, an easy value to get wrong if hand-typed from memory
   of AWS's usual convention.
7. **`InitializeService` itself never returns `UninitializedAccountException`** (errors:
   `AccessDeniedException`/`ValidationException` only) — logically necessary (the whole point of
   the call is to get PAST the uninitialized state) but worth confirming explicitly rather than
   assuming the generational split above applies without exception; it is a legacy-generation op
   by every other signal (no `ThrottlingException`, has `AccountID`... actually `InitializeService`
   itself has no `AccountID` field either, confirmed by direct read — it initializes the CALLING
   account, not a delegated one).
8. **`ListNetworkMigrationDefinitions` is the only op in the entire service with a single-member
   error set**: `AccessDeniedException` alone (confirmed by direct read of its
   `deserializeOpError` switch) — no `ResourceNotFoundException`, no `ValidationException`, not
   even for a presumably-filterable list call.

## Family tables — every one of the 95 operations

All method/path values below come from the Python-regex extraction over `serializers.go` described
above (all 95 matched, not sampled). All error sets come from the equivalent per-op
`strings.EqualFold` extraction over `deserializers.go` (all 95, not sampled). Field lists come from
directly reading each op's `api_op_<Op>.go` Input/Output struct or the shared `types/types.go`
struct it flattens/nests.

### A. Source server lifecycle & data replication control (16 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| DescribeSourceServers | POST /DescribeSourceServers | `Filters *DescribeSourceServersRequestFilters` (ApplicationIDs[], IsArchived, LifeCycleStates[], ReplicationTypes[], SourceServerIDs[]), MaxResults, NextToken | `Items []SourceServer`, NextToken | UninitializedAccount, Validation |
| UpdateSourceServer | POST /UpdateSourceServer | SourceServerID*, AccountID, `ConnectorAction *SourceServerConnectorAction` | flattened SourceServer (trap #1) | Conflict, ResourceNotFound, UninitializedAccount |
| UpdateSourceServerReplicationType | POST /UpdateSourceServerReplicationType | SourceServerID*, ReplicationType* (AGENT_BASED\|SNAPSHOT_SHIPPING), AccountID | flattened SourceServer | Conflict, ResourceNotFound, UninitializedAccount, Validation |
| DeleteSourceServer | POST /DeleteSourceServer | SourceServerID*, AccountID | empty | Conflict, ResourceNotFound, UninitializedAccount |
| ChangeServerLifeCycleState | POST /ChangeServerLifeCycleState | SourceServerID*, `LifeCycle *ChangeServerLifeCycleStateSourceServerLifecycle`{State: READY_FOR_TEST\|READY_FOR_CUTOVER\|CUTOVER}*, AccountID | flattened SourceServer | Conflict, ResourceNotFound, UninitializedAccount, Validation |
| DisconnectFromService | POST /DisconnectFromService | SourceServerID*, AccountID | flattened SourceServer | Conflict, ResourceNotFound, UninitializedAccount |
| FinalizeCutover | POST /FinalizeCutover | SourceServerID*, AccountID | flattened SourceServer | Conflict, ResourceNotFound, UninitializedAccount, Validation |
| MarkAsArchived | POST /MarkAsArchived | SourceServerID*, AccountID | flattened SourceServer | Conflict, ResourceNotFound, UninitializedAccount |
| StartTest | POST /StartTest | SourceServerIDs*[]string (BATCH — multiple servers, one Job), AccountID, Tags | nested `Job *Job` (trap #1) | Conflict, UninitializedAccount, Validation |
| StartCutover | POST /StartCutover | SourceServerIDs*[]string (batch), AccountID, Tags | nested `Job *Job` | Conflict, UninitializedAccount, Validation |
| StartReplication | POST /StartReplication | SourceServerID*, AccountID | empty | Conflict, ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount, Validation |
| StopReplication | POST /StopReplication | SourceServerID*, AccountID | flattened SourceServer | Conflict, ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount, Validation |
| PauseReplication | POST /PauseReplication | SourceServerID*, AccountID | flattened SourceServer | Conflict, ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount, Validation |
| ResumeReplication | POST /ResumeReplication | SourceServerID*, AccountID | flattened SourceServer | Conflict, ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount, Validation |
| RetryDataReplication | POST /RetryDataReplication | SourceServerID*, AccountID | flattened SourceServer | ResourceNotFound, UninitializedAccount, Validation |
| TerminateTargetInstances | POST /TerminateTargetInstances | SourceServerIDs*[]string (batch), AccountID, Tags | nested `Job *Job` | Conflict, UninitializedAccount, Validation |

Note: `StartReplication`'s empty output is a genuine void-result op (per parity-principles.md rule
4 — confirmed by reading `api_op_StartReplication.go` directly, it really has no output fields
besides `ResultMetadata`), not a disguised stub; every sibling `*Replication` op (Stop/Pause/Resume)
DOES return the flattened SourceServer, so `StartReplication`'s emptiness is a real, deliberate
asymmetry, not an oversight in this table.

### B. Jobs (3 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| DescribeJobs | POST /DescribeJobs | `Filters *DescribeJobsRequestFilters`{FromDate, JobIDs[], ToDate}, MaxResults, NextToken, AccountID | `Items []Job`, NextToken | UninitializedAccount, Validation |
| DescribeJobLogItems | POST /DescribeJobLogItems | JobID*, MaxResults, NextToken, AccountID | `Items []JobLog`{Event, EventData, LogDateTime}, NextToken | UninitializedAccount, Validation |
| DeleteJob | POST /DeleteJob | JobID*, AccountID | empty | Conflict, ResourceNotFound, UninitializedAccount |

`Job`{JobID*, Arn, CreationDateTime, EndDateTime, InitiatedBy, `ParticipatingServers
[]ParticipatingServer`{SourceServerID*, LaunchStatus, LaunchedEc2InstanceID,
PostLaunchActionsStatus}, Status (JobStatus: PENDING/STARTED/COMPLETED — only 3 values, no FAILED —
see State machines), Tags, Type (JobType: LAUNCH/TERMINATE — only 2 values)}. `JobLogEvent` has 16
values (JOB_START, SERVER_SKIPPED, CLEANUP_START/END/FAIL, SNAPSHOT_START/END/FAIL,
USING_PREVIOUS_SNAPSHOT, CONVERSION_START/END/FAIL, LAUNCH_START/FAILED, JOB_CANCEL, JOB_END) —
these are the honest granular steps a simulated job progression should walk through.

### C. Launch configuration (per-server) + Launch Configuration Templates (6 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| GetLaunchConfiguration | POST /GetLaunchConfiguration | SourceServerID*, AccountID | flattened (BootMode, CopyPrivateIp, CopyTags, Ec2LaunchTemplateID, EnableMapAutoTagging, LaunchDisposition, Licensing, MapAutoTaggingMpeID, Name, PostLaunchActions, SourceServerID, TargetInstanceTypeRightSizingMethod — NO `types.LaunchConfiguration` struct exists, trap #2) | ResourceNotFound, UninitializedAccount |
| UpdateLaunchConfiguration | POST /UpdateLaunchConfiguration | SourceServerID*, AccountID, all config fields optional (partial update) | same flattened shape | Conflict, ResourceNotFound, UninitializedAccount, Validation |
| CreateLaunchConfigurationTemplate | POST /CreateLaunchConfigurationTemplate | all fields optional (no field marked required in the Go struct at all — confirmed by direct read) incl. `PostLaunchActions`, `Licensing`, `LargeVolumeConf`/`SmallVolumeConf *LaunchTemplateDiskConf`, Tags | `LaunchConfigurationTemplate` (full, 19 fields incl. `LaunchConfigurationTemplateID`, `Arn`) | AccessDenied, UninitializedAccount, Validation |
| DeleteLaunchConfigurationTemplate | POST /DeleteLaunchConfigurationTemplate | LaunchConfigurationTemplateID* | empty | Conflict, ResourceNotFound, UninitializedAccount |
| DescribeLaunchConfigurationTemplates | POST /DescribeLaunchConfigurationTemplates | LaunchConfigurationTemplateIDs[] (optional filter), MaxResults, NextToken | `Items []LaunchConfigurationTemplate`, NextToken | ResourceNotFound, UninitializedAccount, Validation |
| UpdateLaunchConfigurationTemplate | POST /UpdateLaunchConfigurationTemplate | LaunchConfigurationTemplateID*, rest optional | `LaunchConfigurationTemplate` | AccessDenied, ResourceNotFound, UninitializedAccount, Validation |

### D. Replication configuration (per-server) + Replication Configuration Templates (6 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| GetReplicationConfiguration | POST /GetReplicationConfiguration | SourceServerID*, AccountID | flattened (AssociateDefaultSecurityGroup, BandwidthThrottling, CreatePublicIP, DataPlaneRouting, DefaultLargeStagingDiskType, EbsEncryption, EbsEncryptionKeyArn, InternetProtocol, Name, ReplicatedDisks[], ReplicationServerInstanceType, ReplicationServersSecurityGroupsIDs[], SourceServerID, StagingAreaSubnetId, StagingAreaTags, StorageConfiguration, StoreSnapshotOnLocalZone, UseDedicatedReplicationServer, UseFipsEndpoint — no `types.ReplicationConfiguration` struct exists, trap #2) | ResourceNotFound, UninitializedAccount |
| UpdateReplicationConfiguration | POST /UpdateReplicationConfiguration | SourceServerID*, AccountID, all config fields optional | same flattened shape | AccessDenied, Conflict, ResourceNotFound, UninitializedAccount, Validation |
| CreateReplicationConfigurationTemplate | POST /CreateReplicationConfigurationTemplate | most fields required (AssociateDefaultSecurityGroup*, BandwidthThrottling*, CreatePublicIP*, DataPlaneRouting*, DefaultLargeStagingDiskType*, EbsEncryption*, ReplicationServerInstanceType*, ReplicationServersSecurityGroupsIDs*[], StagingAreaSubnetId*, StagingAreaTags*, UseDedicatedReplicationServer* — unlike its LaunchConfigurationTemplate sibling where nothing is required, confirmed by direct read) | `ReplicationConfigurationTemplate` (full, 19 fields incl. ID/Arn/Tags) | AccessDenied, UninitializedAccount, Validation |
| DeleteReplicationConfigurationTemplate | POST /DeleteReplicationConfigurationTemplate | ReplicationConfigurationTemplateID* | empty | Conflict, ResourceNotFound, UninitializedAccount |
| DescribeReplicationConfigurationTemplates | POST /DescribeReplicationConfigurationTemplates | ReplicationConfigurationTemplateIDs[] (optional filter), MaxResults, NextToken | `Items []ReplicationConfigurationTemplate`, NextToken | ResourceNotFound, UninitializedAccount, Validation |
| UpdateReplicationConfigurationTemplate | POST /UpdateReplicationConfigurationTemplate | ReplicationConfigurationTemplateID*, rest optional | `ReplicationConfigurationTemplate` | AccessDenied, ResourceNotFound, UninitializedAccount, Validation |

Note the required-vs-optional asymmetry between `CreateLaunchConfigurationTemplate` (nothing
required) and `CreateReplicationConfigurationTemplate` (11 fields required) — confirmed by direct
struct read on both, not an inconsistency in this table.

### E. Applications (8 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| CreateApplication | POST /CreateApplication | Name*, AccountID, Description, Tags | `Application`{ApplicationID, Arn, ApplicationAggregatedStatus, CreationDateTime, Description, IsArchived, LastModifiedDateTime, Name, Tags, WaveID} | Conflict, ServiceQuotaExceeded, UninitializedAccount |
| UpdateApplication | POST /UpdateApplication | ApplicationID*, AccountID, Description, Name | `Application` | Conflict, ResourceNotFound, UninitializedAccount |
| DeleteApplication | POST /DeleteApplication | ApplicationID*, AccountID | empty | Conflict, ResourceNotFound, UninitializedAccount |
| ListApplications | POST /ListApplications | `Filters *ListApplicationsRequestFilters`, MaxResults, NextToken, AccountID | `Items []Application`, NextToken | UninitializedAccount |
| ArchiveApplication | POST /ArchiveApplication | ApplicationID*, AccountID | `Application` (IsArchived=true) | Conflict, ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount |
| UnarchiveApplication | POST /UnarchiveApplication | ApplicationID*, AccountID | `Application` (IsArchived=false) | ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount |
| AssociateSourceServers | POST /AssociateSourceServers | ApplicationID*, SourceServerIDs*[], AccountID | empty | Conflict, ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount |
| DisassociateSourceServers | POST /DisassociateSourceServers | ApplicationID*, SourceServerIDs*[], AccountID | empty | Conflict, ResourceNotFound, UninitializedAccount |

`ApplicationAggregatedStatus`{HealthStatus (HEALTHY/LAGGING/ERROR), LastUpdateDateTime,
ProgressStatus (NOT_STARTED/IN_PROGRESS/COMPLETED), TotalSourceServers} — a rollup over the
Application's associated SourceServers, same shape as `WaveAggregatedStatus` below.

### F. Waves (8 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| CreateWave | POST /CreateWave | Name*, AccountID, Description, Tags | `Wave`{WaveID, Arn, CreationDateTime, Description, IsArchived, LastModifiedDateTime, Name, Tags, WaveAggregatedStatus} | Conflict, ServiceQuotaExceeded, UninitializedAccount |
| UpdateWave | POST /UpdateWave | WaveID*, AccountID, Description, Name | `Wave` | Conflict, ResourceNotFound, UninitializedAccount |
| DeleteWave | POST /DeleteWave | WaveID*, AccountID | empty | Conflict, ResourceNotFound, UninitializedAccount |
| ListWaves | POST /ListWaves | `Filters *ListWavesRequestFilters`, MaxResults, NextToken, AccountID | `Items []Wave`, NextToken | UninitializedAccount |
| ArchiveWave | POST /ArchiveWave | WaveID*, AccountID | `Wave` (IsArchived=true) | Conflict, ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount |
| UnarchiveWave | POST /UnarchiveWave | WaveID*, AccountID | `Wave` (IsArchived=false) | ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount |
| AssociateApplications | POST /AssociateApplications | WaveID*, ApplicationIDs*[], AccountID | empty | Conflict, ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount |
| DisassociateApplications | POST /DisassociateApplications | WaveID*, ApplicationIDs*[], AccountID | empty | Conflict, ResourceNotFound, UninitializedAccount |

`WaveAggregatedStatus`{HealthStatus (WAVE: HEALTHY/LAGGING/ERROR — distinct enum type from
Application's but identical values), LastUpdateDateTime, ProgressStatus (NOT_STARTED/IN_PROGRESS/
COMPLETED), `ReplicationStartedDateTime` (Wave-only — no Application equivalent), TotalApplications}.
The grouping hierarchy, confirmed structurally: **Wave contains Applications (via
Associate/DisassociateApplications), Application contains SourceServers (via
Associate/DisassociateSourceServers)** — a SourceServer's own `ApplicationID` field is the reverse
pointer, but there's no direct SourceServer<->Wave association at all; it's always mediated through
an Application.

### G. Connectors (4 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| CreateConnector | POST /CreateConnector | Name*, SsmInstanceID*, `SsmCommandConfig *ConnectorSsmCommandConfig`{CloudWatchOutputEnabled*, S3OutputEnabled*, CloudWatchLogGroupName, OutputS3BucketName}, Tags | `Connector`{ConnectorID, Arn, Name, SsmCommandConfig, SsmInstanceID, Tags} | UninitializedAccount, Validation |
| UpdateConnector | POST /UpdateConnector | ConnectorID*, rest optional | `Connector` | ResourceNotFound, UninitializedAccount, Validation |
| DeleteConnector | POST /DeleteConnector | ConnectorID* | empty | ResourceNotFound, UninitializedAccount, Validation |
| ListConnectors | POST /ListConnectors | `Filters *ListConnectorsRequestFilters`, MaxResults, NextToken | `Items []Connector`, NextToken | UninitializedAccount, Validation |

No `AccountID` field on any Connector op (confirmed) — Connectors, unlike SourceServers/
Applications/Waves, are not delegated-account-scoped in this SDK. A `Connector` represents an SSM
Managed Instance (`SsmInstanceID`) running the MGN connector software that bridges an on-prem
vCenter environment to the AWS control plane — this repo has no SSM Managed Instance concept to
validate `SsmInstanceID` against (not independently confirmed either way this pass).

### H. vCenter clients (2 ops — no create op)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| DescribeVcenterClients | **GET** /DescribeVcenterClients | MaxResults, NextToken | `Items []VcenterClient`{VcenterClientID, Arn, DatacenterName, Hostname, LastSeenDatetime, SourceServerTags, Tags, VcenterUUID}, NextToken | ResourceNotFound, UninitializedAccount, Validation |
| DeleteVcenterClient | POST /DeleteVcenterClient | VcenterClientID* | empty | ResourceNotFound, UninitializedAccount, Validation |

No `CreateVcenterClient` op exists anywhere in this 95-op surface (confirmed against the full
alphabetical op list above) — see gaps. A `VcenterClient` record is created only by the on-prem
vCenter connector appliance registering itself with AWS; this API surface can only list and delete
what already exists.

### I. Export / Import (8 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| StartExport | POST /StartExport | S3Bucket*, S3Key*, S3BucketOwner, Tags | `ExportTask`{ExportID, Arn, CreationDateTime, EndDateTime, ProgressPercentage, S3Bucket, S3BucketOwner, S3Key, Status (ExportStatus: PENDING/STARTED/FAILED/SUCCEEDED), Summary{ApplicationsCount, ServersCount, WavesCount}, Tags} | ServiceQuotaExceeded, UninitializedAccount, Validation |
| ListExports | POST /ListExports | `Filters *ListExportsRequestFilters`, MaxResults, NextToken | `Items []ExportTask`, NextToken | UninitializedAccount |
| ListExportErrors | POST /ListExportErrors | ExportID*, MaxResults, NextToken | `Items []ExportTaskError`{ErrorData, ErrorDateTime}, NextToken | UninitializedAccount, Validation |
| StartImport | POST /StartImport | `S3BucketSource *S3BucketSource`*{S3Bucket*, S3Key*, S3BucketOwner}, ClientToken, Tags | `ImportTask`{ImportID, Arn, CreationDateTime, EndDateTime, ProgressPercentage, S3BucketSource, Status (ImportStatus: PENDING/STARTED/FAILED/SUCCEEDED), Summary{Applications{CreatedCount,ModifiedCount}, Servers{CreatedCount,ModifiedCount}, Waves{CreatedCount,ModifiedCount}}, Tags} | Conflict, ResourceNotFound, ServiceQuotaExceeded, UninitializedAccount, Validation |
| ListImports | POST /ListImports | `Filters *ListImportsRequestFilters`, MaxResults, NextToken | `Items []ImportTask`, NextToken | UninitializedAccount, Validation |
| ListImportErrors | POST /ListImportErrors | ImportID*, MaxResults, NextToken | `Items []ImportTaskError`{ErrorData, ErrorDateTime, ErrorType (VALIDATION_ERROR/PROCESSING_ERROR)}, NextToken | UninitializedAccount, Validation |
| StartImportFileEnrichment | **POST /network-migration/StartImportFileEnrichment** | `SourceS3Configuration *EnrichmentSourceS3Configuration`*, `TargetS3Configuration *EnrichmentTargetS3Configuration`*, Tags | `ImportFileEnrichment`{Status (ImportFileEnrichmentStatus: PENDING/STARTED/FAILED/SUCCEEDED/SUCCEEDED_WITH_WARNINGS — 5 values, richer than plain ImportStatus)} | AccessDenied, Conflict, ServiceQuotaExceeded, Throttling, Validation |
| ListImportFileEnrichments | **POST /network-migration/ListImportFileEnrichments** | `Filters *ListImportFileEnrichmentsFilters`, MaxResults, NextToken | `Items []ImportFileEnrichment`, NextToken | Validation only |

`StartImport`'s `ImportTaskSummary.{Applications,Servers,Waves}.CreatedCount/ModifiedCount`
confirms this op is the ONLY public-API mechanism that creates `SourceServer`/`Application`/`Wave`
records in bulk from an external file — see gaps for why this matters for testability.

### J. Post-launch custom actions — source-server-scoped and template-scoped (6 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| PutSourceServerAction | POST /PutSourceServerAction | ActionID*, ActionName*, DocumentIdentifier*, Order*int32, SourceServerID*, AccountID, Active, Category (ActionCategory), Description, DocumentVersion, ExternalParameters, MustSucceedForCutover, Parameters, TimeoutSeconds | `SourceServerActionDocument` | Conflict, ResourceNotFound, UninitializedAccount, Validation |
| ListSourceServerActions | POST /ListSourceServerActions | SourceServerID*, `Filters *SourceServerActionsRequestFilters`, MaxResults, NextToken, AccountID | `Items []SourceServerActionDocument`, NextToken | ResourceNotFound, UninitializedAccount |
| RemoveSourceServerAction | POST /RemoveSourceServerAction | ActionID*, SourceServerID*, AccountID | empty | ResourceNotFound, UninitializedAccount, Validation |
| PutTemplateAction | POST /PutTemplateAction | ActionID*, ActionName*, DocumentIdentifier*, LaunchConfigurationTemplateID*, Order*int32, Active, Category, Description, DocumentVersion, ExternalParameters, MustSucceedForCutover, OperatingSystem, Parameters, TimeoutSeconds | `TemplateActionDocument` | Conflict, ResourceNotFound, UninitializedAccount, Validation |
| ListTemplateActions | POST /ListTemplateActions | LaunchConfigurationTemplateID*, `Filters *TemplateActionsRequestFilters`, MaxResults, NextToken | `Items []TemplateActionDocument`, NextToken | ResourceNotFound, UninitializedAccount |
| RemoveTemplateAction | POST /RemoveTemplateAction | ActionID*, LaunchConfigurationTemplateID* | empty | ResourceNotFound, UninitializedAccount, Validation |

`ActionCategory` (not read in full this pass — a real enum in `types/enums.go`, worth reading in
full during implementation, not assumed here). `PutTemplateAction` additionally carries
`OperatingSystem *string` (free-form, not typed) so template actions can be gated to specific guest
OSes; `PutSourceServerAction` has no such field (it targets one already-known server, whose OS is
already fixed). Note `RemoveTemplateAction`/`ListTemplateActions` have no `AccountID` field while
`PutTemplateAction` also has none — the whole Template-action family is un-delegated, matching the
Launch/ReplicationConfigurationTemplate family's own lack of `AccountID`.

### K. Service init & managed accounts (2 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| InitializeService | POST /InitializeService | (no input fields at all) | empty | AccessDenied, Validation |
| ListManagedAccounts | POST /ListManagedAccounts | MaxResults, NextToken | `Items []ManagedAccount`{AccountId}, NextToken | UninitializedAccount, Validation |

`InitializeService` is the account-level "opt in" call every other legacy op's
`UninitializedAccountException` implicitly depends on — an honest simulation needs a per-account
"initialized" flag gating almost every other legacy op (69 of 95, per the errors section above)
until this is called once.

### L. Tagging (3 ops)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| TagResource | POST /tags/{resourceArn} | ResourceArn* (path param), Tags* map[string]string | empty | AccessDenied, InternalServer, ResourceNotFound, Throttling, Validation |
| UntagResource | DELETE /tags/{resourceArn} | ResourceArn* (path param), TagKeys*[]string | empty | AccessDenied, InternalServer, ResourceNotFound, Throttling, Validation |
| ListTagsForResource | GET /tags/{resourceArn} | ResourceArn* (path param) | `Tags map[string]string` | AccessDenied, InternalServer, ResourceNotFound, Throttling, Validation |

Every taggable resource type in this service carries its own inline `Tags map[string]string` field
already (`Application.Tags`, `Wave.Tags`, `SourceServer.Tags`, `Job.Tags`, `Connector.Tags`,
`VcenterClient.Tags`, `LaunchConfigurationTemplate.Tags`, `ReplicationConfigurationTemplate.Tags`,
`ExportTask.Tags`, `ImportTask.Tags`, `NetworkMigrationDefinitionSummary.Tags`,
`NetworkMigrationExecution.Tags`) — this generic ARN-keyed API is the cross-cutting way to read/
mutate all of them, not a separate tag store. See Cross-service wiring below.

### M. Network Migration — definitions & mapper segments (13 ops, all under `/network-migration/`)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| CreateNetworkMigrationDefinition | POST | Name*, `TargetNetwork *TargetNetwork`*{Topology* (ISOLATED_VPC/HUB_AND_SPOKE), InboundCidr, InspectionCidr, OutboundCidr}, `TargetS3Configuration`*, Description, ScopeTags, `SourceConfigurations []SourceConfiguration`, Tags, TargetDeployment | `NetworkMigrationDefinitionSummary`{NetworkMigrationDefinitionID, Arn, Name, ScopeTags, SourceEnvironment, Tags} | ServiceQuotaExceeded, Validation |
| GetNetworkMigrationDefinition | POST | NetworkMigrationDefinitionID* | full definition detail | AccessDenied, ResourceNotFound |
| UpdateNetworkMigrationDefinition | POST | NetworkMigrationDefinitionID*, rest optional incl. `TargetNetworkUpdate` | updated definition | AccessDenied, ResourceNotFound, Validation |
| DeleteNetworkMigrationDefinition | POST | NetworkMigrationDefinitionID* | empty | AccessDenied, Conflict, ResourceNotFound |
| ListNetworkMigrationDefinitions | POST | `Filters`, MaxResults, NextToken | `Items []NetworkMigrationDefinitionSummary`, NextToken | **AccessDenied only** (trap #8) |
| GetNetworkMigrationMapperSegmentConstruct | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID*, SegmentID*, ConstructID* (all required — a 4-key composite lookup) | `NetworkMigrationMapperSegmentConstruct` | AccessDenied, ResourceNotFound, Validation |
| ListNetworkMigrationMapperSegmentConstructs | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID*, SegmentID*, `Filters`, MaxResults, NextToken | `Items []NetworkMigrationMapperSegmentConstruct`, NextToken | AccessDenied, ResourceNotFound, Throttling, Validation |
| ListNetworkMigrationMapperSegments | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID*, `Filters`, MaxResults, NextToken | `Items []NetworkMigrationMapperSegment`{SegmentID, Checksum, CreatedAt, Description, JobID, LogicalID, Name, OutputS3Configuration, ReferencedSegments[], ScopeTags, SegmentType (WORKLOAD/APPLIANCE), TargetAccount, UpdatedAt}, NextToken | AccessDenied, ResourceNotFound, Throttling, Validation |
| UpdateNetworkMigrationMapperSegment | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID*, SegmentID*, rest optional (ScopeTags, TargetAccount) | updated segment | AccessDenied, ResourceNotFound, Validation |
| ListNetworkMigrationMappings | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID*, `Filters`, MaxResults, NextToken | mapping job details list | AccessDenied, ResourceNotFound, Throttling, Validation |
| ListNetworkMigrationMappingUpdates | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID*, `Filters`, MaxResults, NextToken | mapping-update job details list | AccessDenied, ResourceNotFound, Throttling, Validation |
| StartNetworkMigrationMapping | POST | NetworkMigrationDefinitionID*, **NetworkMigrationExecutionID*** (required — see gaps: no op creates this ID), SecurityGroupMappingStrategy | `JobID *string` only | AccessDenied, Conflict, ResourceNotFound, ServiceQuotaExceeded, Throttling, Validation |
| StartNetworkMigrationMappingUpdate | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID*, `Constructs []StartNetworkMigrationMappingUpdateConstruct`{ConstructID*, ConstructType*, SegmentID*, `Operation OperationUnion` (union of Delete/Merge/Split/Update operations)}, `Segments []StartNetworkMigrationMappingUpdateSegment` | job details | AccessDenied, Conflict, ResourceNotFound, ServiceQuotaExceeded, Throttling, Validation |

`OperationUnion` members: `OperationUnionMemberDelete` (empty — pure removal),
`OperationUnionMemberMerge`{MergeConstructs []MergeConstruct}, `OperationUnionMemberSplit`
{SplitConstructs []SplitConstruct}, `OperationUnionMemberUpdate`{Excluded, Name, Properties
map[string]string} — a real tagged-union edit-script for reshaping the network mapper's segment
graph, not simple field replacement.

### N. Network Migration — analysis, code generation, deployment, executions (10 ops, all under `/network-migration/`)

| Op | Method/Path | Key in | Key out | Errors |
|---|---|---|---|---|
| StartNetworkMigrationAnalysis | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID* | `JobID` | AccessDenied, Conflict, ResourceNotFound, ServiceQuotaExceeded, Throttling, Validation |
| ListNetworkMigrationAnalyses | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID*, `Filters`, MaxResults, NextToken | `NetworkMigrationAnalysisJobDetails`{JobID, CreatedAt, EndedAt, NetworkMigrationDefinitionID, NetworkMigrationExecutionID, Status (NetworkMigrationJobStatus: PENDING/STARTED/SUCCEEDED/FAILED), StatusDetails} list | AccessDenied, ResourceNotFound, Throttling, Validation |
| ListNetworkMigrationAnalysisResults | POST | same scoping keys, `Filters`, MaxResults, NextToken | `NetworkMigrationAnalysisResult`{AnalysisResult *string (free-text finding), AnalyzerType, JobID, Source{SubnetID,VpcID}, Status (NetworkMigrationAnalysisResultStatus: PENDING/STARTED/SUCCEEDED/FAILED), Target} list | AccessDenied, ResourceNotFound, Throttling, Validation |
| StartNetworkMigrationCodeGeneration | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID* | `JobID` | AccessDenied, Conflict, ResourceNotFound, ServiceQuotaExceeded, Throttling, Validation |
| ListNetworkMigrationCodeGenerations | POST | scoping keys, `Filters`, MaxResults, NextToken | `NetworkMigrationCodeGenerationJobDetails` list | AccessDenied, ResourceNotFound, Throttling, Validation |
| ListNetworkMigrationCodeGenerationSegments | POST | scoping keys + SegmentID?, `Filters`, MaxResults, NextToken | `NetworkMigrationCodeGenerationSegment`{`Artifact *NetworkMigrationCodeGenerationArtifact`{ArtifactType (NetworkMigrationCodeGenerationArtifactType), SubType, Status (CodeGenerationOutputFormatStatusDetails)}} list | AccessDenied, ResourceNotFound, Throttling, Validation |
| StartNetworkMigrationDeployment | POST | NetworkMigrationDefinitionID*, NetworkMigrationExecutionID* | `JobID` | AccessDenied, Conflict, ResourceNotFound, ServiceQuotaExceeded, Throttling, Validation |
| ListNetworkMigrationDeployments | POST | scoping keys, `Filters`, MaxResults, NextToken | `NetworkMigrationDeployerJobDetails` list | AccessDenied, ResourceNotFound, Throttling, Validation |
| ListNetworkMigrationDeployedStacks | POST | scoping keys, `Filters`, MaxResults, NextToken | `NetworkMigrationDeployedStackDetails`{Status (NetworkMigrationDeployedStackStatus: CREATE_COMPLETE/CREATE_FAILED/CREATE_STARTED/DELETE_COMPLETE/DELETE_FAILED/DELETE_STARTED)} list — doc comment: "a CloudFormation stack that has been deployed as part of the network migration" | AccessDenied, ResourceNotFound, Throttling, Validation |
| ListNetworkMigrationExecutions | POST | **NetworkMigrationDefinitionID* only** — no create-side op exists (see gaps) | `Items []NetworkMigrationExecution`{Activity (ExecutionStageActivity: MAPPING/MAPPING_UPDATE/CODE_GENERATION/DEPLOY/DEPLOYED_STACKS_DELETION/ANALYZE), CreatedAt, NetworkMigrationDefinitionID, NetworkMigrationExecutionID, Stage (ExecutionStage: same 5 values minus MAPPING_UPDATE), Status (ExecutionStatus: PENDING/STARTED/SUCCEEDED/FAILED), Tags, UpdatedAt}, NextToken | AccessDenied, ResourceNotFound |

`ExecutionStage` (5 values: MAPPING/CODE_GENERATION/DEPLOY/DEPLOYED_STACKS_DELETION/ANALYZE) is the
top-level workflow phase an execution is in; `ExecutionStageActivity` (6 values, adds
MAPPING_UPDATE) is the finer-grained activity within that stage. Both are genuinely orthogonal to
the per-job `NetworkMigrationJobStatus`/`ExecutionStatus` (PENDING/STARTED/SUCCEEDED/FAILED) —
an execution's `Stage`/`Activity` says WHERE it is in the pipeline, `Status` says whether the
CURRENT step succeeded or failed.

## 2. Missing simulated functionality (the real emulation work)

MGN is fundamentally a **replication/cutover orchestration** service (the legacy 70 ops) with a
**network-topology analysis and code-generation/deployment** feature bolted on (the 25
`/network-migration/` ops). The two halves need almost entirely different honesty treatments.

### SourceServer lifecycle — `LifeCycleState` (10 values, confirmed in `types/enums.go`)

`STOPPED`, `NOT_READY`, `READY_FOR_TEST`, `TESTING`, `READY_FOR_CUTOVER`, `CUTTING_OVER`,
`CUTOVER`, `DISCONNECTED`, `DISCOVERED`, `PENDING_INSTALLATION`. `ChangeServerLifeCycleState`'s own
input enum (`ChangeServerLifeCycleStateSourceServerLifecycleState`) only exposes 3 of these as
CALLER-settable targets: `READY_FOR_TEST`, `READY_FOR_CUTOVER`, `CUTOVER` — every other value
(`NOT_READY`, `TESTING`, `CUTTING_OVER`, `DISCONNECTED`, `DISCOVERED`, `PENDING_INSTALLATION`,
`STOPPED`) is system-driven, reached only as a side effect of some other op or the passage of
(simulated) time, never directly settable. A defensible legal-transition table, inferred from field
names and doc comments (NOT independently confirmed against AWS's real state machine, which is not
published in the SDK — flag this inference explicitly if implemented):

- `PENDING_INSTALLATION` → `DISCOVERED`/`NOT_READY` once the (simulated) replication agent
  reports in and `DataReplicationInfo` begins populating.
- `NOT_READY` → `READY_FOR_TEST` once `DataReplicationState` reaches `CONTINUOUS` (a real
  replicated disk with no active backlog).
- `READY_FOR_TEST` → `TESTING` (via `StartTest`) → `READY_FOR_CUTOVER` or back to
  `READY_FOR_TEST` (test failed/reverted, per `LifeCycleLastTest.Reverted`).
- `READY_FOR_CUTOVER` → `CUTTING_OVER` (via `StartCutover`) → `CUTOVER` (via `FinalizeCutover`) or
  back to `READY_FOR_CUTOVER` (`LifeCycleLastCutover.Reverted`).
- `CUTOVER` → `DISCONNECTED` (via `DisconnectFromService`) is the terminal, expected end state
  once the target instance is fully cut over and the source is no longer needed.
- `MarkAsArchived` sets `SourceServer.IsArchived` — a SEPARATE boolean orthogonal to
  `LifeCycleState` (an archived server can be in any lifecycle state; this is a visibility/cleanup
  flag, not a lifecycle transition itself), confirmed by `IsArchived *bool` living directly on
  `SourceServer`, not inside `LifeCycle`.

`LifeCycle` also carries `LastTest`/`LastCutover` sub-records (`Initiated`{ApiCallDateTime, JobID},
`Reverted`{ApiCallDateTime}, `Finalized`{ApiCallDateTime}) — an honest simulation should populate
`JobID` on `Initiated` pointing back at the `StartTest`/`StartCutover` Job that triggered it, giving
a real audit trail rather than independent, disconnected timestamps.

### `DataReplicationInfo` sub-state-machine

`DataReplicationState` (12 values): `STOPPED`, `INITIATING`, `INITIAL_SYNC`, `BACKLOG`,
`CREATING_SNAPSHOT`, `CONTINUOUS`, `PAUSED`, `RESCAN`, `STALLED`, `DISCONNECTED`,
`PENDING_SNAPSHOT_SHIPPING`, `SHIPPING_SNAPSHOT` — the last two only apply when
`ReplicationType == SNAPSHOT_SHIPPING` rather than `AGENT_BASED` (confirmed by field semantics, not
an explicit SDK-encoded constraint). `DataReplicationInitiation.Steps
[]DataReplicationInitiationStep` walks a fixed 12-step sequence (`WAIT` →
`CREATE_SECURITY_GROUP` → `LAUNCH_REPLICATION_SERVER` → `BOOT_REPLICATION_SERVER` →
`AUTHENTICATE_WITH_SERVICE` → `DOWNLOAD_REPLICATION_SOFTWARE` → `CREATE_STAGING_DISKS` →
`ATTACH_STAGING_DISKS` → `PAIR_REPLICATION_SERVER_WITH_AGENT` →
`CONNECT_AGENT_TO_REPLICATION_SERVER` → `START_DATA_TRANSFER` → `SETUP_FSX_PROXY`), each with its
own `DataReplicationInitiationStepStatus` (`NOT_STARTED`/`IN_PROGRESS`/`SUCCEEDED`/`FAILED`/
`SKIPPED`). `DataReplicationInfoReplicatedDisk`{BackloggedStorageBytes, ReplicatedStorageBytes,
RescannedStorageBytes, TotalStorageBytes — all `int64`} is the real per-disk progress ledger.

**There is no real source machine and no real replication agent** — this entire sub-state-machine
is inherently bookkeeping-only. A defensible simulated progression is a deterministic, time-based
walk through the 12 initiation steps followed by a monotonically increasing
`ReplicatedStorageBytes` toward `TotalStorageBytes` (both caller-supplied or defaulted at
`ReplicationConfiguration` creation time), landing on `CONTINUOUS` once fully caught up — NOT a
fabricated "realistic-looking" bandwidth/lag simulation with invented physical throughput numbers,
which would be indistinguishable from real telemetry to a caller and is exactly the kind of
fabrication parity-principles.md warns against. `DataReplicationErrorString` (18 values, e.g.
`AGENT_NOT_SEEN`, `UNSTABLE_NETWORK`, `FAILED_TO_LAUNCH_REPLICATION_SERVER`) should remain unused
in a first pass (no real failure condition exists to trigger them) rather than being randomly
injected to look more "realistic."

### Test → cutover → finalize flow legality

`StartTest`/`StartCutover` both take a BATCH `SourceServerIDs []string` and return one shared
`Job` with `ParticipatingServers []ParticipatingServer` — a single Job can test/cut over multiple
servers together (e.g. all servers in a Wave), each tracked independently via its own
`ParticipatingServer.LaunchStatus` (`PENDING`/`IN_PROGRESS`/`LAUNCHED`/`FAILED`/`TERMINATED`) while
the parent `Job.Status` (`PENDING`/`STARTED`/`COMPLETED` — only 3 values, **no `FAILED`** at the
Job level) presumably reaches `COMPLETED` once every participating server's `LaunchStatus` is
terminal (LAUNCHED or FAILED), never itself becoming a stuck/failed state — an implementer must
decide and document this rollup rule (not encoded in the SDK) rather than presenting it as
AWS-confirmed. Legal precondition: `StartTest` requires `LifeCycleState == READY_FOR_TEST`,
`StartCutover` requires `READY_FOR_CUTOVER`, `FinalizeCutover` (single-server only, unlike its
batch siblings) requires the server currently be in `CUTOVER`-eligible state after a completed
cutover Job — these preconditions are this audit's inference from field/enum semantics, not
independently SDK-confirmed, and should be flagged as a deliberate implementation choice if built
this way.

### Jobs and job logs — an honest async progression

`JobType` has only 2 values: `LAUNCH` (produced by `StartTest`/`StartCutover`) and `TERMINATE`
(produced by `TerminateTargetInstances`). A defensible simulated Job progression: `PENDING` →
`STARTED` (immediately or after a short deterministic delay) → walk `JobLogEvent`s in order
(`JOB_START` → per-participating-server `SNAPSHOT_START`/`SNAPSHOT_END` →
`CONVERSION_START`/`CONVERSION_END` → `LAUNCH_START` → `JOB_END`) while updating each
`ParticipatingServer.LaunchStatus` in step → `Job.Status = COMPLETED`. `DescribeJobLogItems`
should return these as they're "produced" over the deterministic timeline, not all at once —
though returning them all at once with correct final timestamps is a defensible simpler first pass
if explicitly documented as such (per-tick streaming logs are a real-but-optional fidelity
increment, not a correctness requirement).

### Four distinct configuration-family resources — do not conflate

Confirmed as four genuinely separate resource kinds with only namespace-adjacent naming:
1. **Launch configuration** (per-`SourceServer`, via `GetLaunchConfiguration`/
   `UpdateLaunchConfiguration`) — no dedicated Create/Delete op; it presumably comes into
   existence automatically alongside its SourceServer (with defaults, possibly copied from a
   `LaunchConfigurationTemplate` if one is associated — not confirmed how association happens,
   since no `AssociateLaunchConfigurationTemplate`-style op exists in this 95-op surface either).
2. **Launch Configuration Template** (account-level, reusable, via
   Create/Delete/Describe/UpdateLaunchConfigurationTemplate) — has its own ID/ARN/Tags.
3. **Replication configuration** (per-`SourceServer`, via `GetReplicationConfiguration`/
   `UpdateReplicationConfiguration`) — same no-dedicated-create-op pattern as #1.
4. **Replication Configuration Template** (account-level, reusable, via
   Create/Delete/Describe/UpdateReplicationConfigurationTemplate) — has its own ID/ARN/Tags.

**How template → per-server configuration application actually happens is not exposed by any op in
this SDK** — this is a genuine, unresolved gap (similar in kind to the NetworkMigrationExecutionID
gap): an implementer must pick a defensible convention (e.g. new SourceServers inherit the most
recently created enabled template's settings as their initial per-server configuration) and
document it explicitly as invented, not derived.

### Waves and Applications grouping

Confirmed two-level hierarchy: `Wave` ⊃ `Application` (via Associate/DisassociateApplications) ⊃
`SourceServer` (via Associate/DisassociateSourceServers) — see family F/E tables above. Both
`WaveAggregatedStatus`/`ApplicationAggregatedStatus` are rollups (`HealthStatus`,
`ProgressStatus`, counts) that must be recomputed whenever a member SourceServer's `LifeCycle`/
`DataReplicationInfo` changes — this is real, non-trivial rollup logic (a Wave/Application with
one `LAGGING` source server is presumably `LAGGING` overall; the exact aggregation rule is not
SDK-specified and must be invented and documented).

### Connectors, vCenter clients, export/import

**Connectors** and **vCenter clients** are both inherently agent/appliance-registration resources
— see gaps for the concrete consequence (no create op for either). **Export**
(`StartExport`/`ListExports`/`ListExportErrors`) writes a JSON/CSV dump of Applications/Waves/
Servers metadata to a caller-supplied S3 bucket; **Import** (`StartImport`/`ListImports`/
`ListImportErrors`) reads one back in, creating/modifying records per
`ImportTaskSummary.{Applications,Servers,Waves}.{CreatedCount,ModifiedCount}`. Neither this SDK nor
this repo has any S3-file-format schema for what that JSON/CSV actually contains — a real
implementation would need to invent (and clearly flag as invented) a schema, or treat
`StartExport`/`StartImport` as producing/consuming an opaque blob whose row counts are tracked but
whose content is never actually round-tripped meaningfully. **`StartImportFileEnrichment`**
augments an import file with additional discovered network/segment metadata for the Network
Migration feature — genuinely coupled to that sub-product, not the core import flow, despite the
similar naming.

### Post-launch actions

`PostLaunchActions`{Deployment (TEST_AND_CUTOVER/CUTOVER_ONLY/TEST_ONLY), SsmDocuments
[]SsmDocument, S3LogBucket, CloudWatchLogGroupName} describes SSM documents to run against the
newly-launched EC2 instance after a test/cutover. `PostLaunchActionsStatus` /
`JobPostLaunchActionsLaunchStatus` / `PostLaunchActionExecutionStatus` (IN_PROGRESS/SUCCESS/
FAILED) track per-document execution results, keyed to a `ParticipatingServer` within a Job. This
repo has no SSM document execution engine (not independently confirmed this pass, but no SSM
backend was found under `services/`) — an honest simulation can track the STATE (documents listed,
statuses set to a deterministic SUCCESS after a delay) without ever actually running anything,
clearly flagged as bookkeeping-only, matching this campaign's MACsec/BGP-peering precedent from the
directconnect audit.

### Network Migration sub-product — largely bookkeeping-only, explicitly flagged

The 25 `/network-migration/` ops (families M and N above) model: (1) defining a target AWS network
topology and importing on-prem network config exports, (2) analyzing them
(`StartNetworkMigrationAnalysis` → free-text `AnalysisResult` findings), (3) generating
infrastructure code from the analysis (`StartNetworkMigrationCodeGeneration` →
`NetworkMigrationCodeGenerationArtifact`), and (4) deploying that code as real CloudFormation
stacks (`StartNetworkMigrationDeployment` → `NetworkMigrationDeployedStackDetails`, whose own doc
comment says "a CloudFormation stack that has been deployed"). **Steps 2-4 cannot be honestly
performed without a real network-analysis engine and a real code generator, neither of which exists
in this repo.** The state-bookkeeping shell — definitions, executions (once seeded, see gaps),
mapper segments/constructs, and status enums walking their documented values on a deterministic
timer — is honestly buildable. The CONTENT of `AnalysisResult`, generated code artifacts, and
deployed-stack details must remain clearly-flagged placeholders (e.g. an empty or template string),
never fabricated realistic-looking network analysis or generated Terraform/CloudFormation, which
would misrepresent what the emulator actually did.

## 3. Cross-service wiring needed

**Tagging.** `TagResource`/`UntagResource`/`ListTagsForResource` exist (confirmed:
`api_op_TagResource.go`, `api_op_UntagResource.go`, `api_op_ListTagsForResource.go`, family L
above), so this service should be wired into `cli.go`'s `wireResourceGroupsTagging`
(`/home/agbishop/gopherstack/cli.go:5348`), following the `wireTaggingGrafana`/`wireTaggingEFS`
pattern already used for the other 30 wired services (`cli.go:5327-5399`'s own doc comment
enumerates them: dynamodb, sqs, sns, lambda, kms, secretsmanager, ecs, athena, glue, ecr, kinesis,
stepfunctions, cloudfront, eks, batch, wafv2, backup, efs, docdb, neptune, rds, elasticache,
redshift, sagemaker, firehose, opensearch, cloudwatchlogs, mq, emr, grafana). MGN would be the
31st entry, `wireTaggingMGN(bk, byName["MGN"])` (or whatever name string this service registers
itself under — not confirmed here since the service doesn't exist yet to register anything).
Unlike Outposts (one generic tag store shared by 2 resource kinds), MGN has **12 distinct taggable
resource kinds** (Application, Wave, SourceServer, Job, Connector, VcenterClient,
LaunchConfigurationTemplate, ReplicationConfigurationTemplate, ExportTask, ImportTask,
NetworkMigrationDefinitionSummary, NetworkMigrationExecution) all sharing the one
`/tags/{resourceArn}` API — the tag store backing this wiring needs to be ARN-keyed across all 12,
not scoped to a single resource-type map.

**ARN namespace.** Could NOT be confirmed against Terraform provider source this pass — see the
gaps entry above for the full explanation (Terraform's `internal/service/mgn/` package has zero
resource files, only generated client boilerplate). The best available corroborating evidence is
botocore's `service-2.json` metadata, where `endpointPrefix`/`serviceId`/`signingName` are all
literally `"mgn"` (fetched via `raw.githubusercontent.com/boto/botocore/develop/botocore/data/mgn/
2020-02-26/service-2.json`) — consistent with, but not proof of, the ARN service segment also
being `"mgn"` (this repo's own arn.Build helper, `pkgs/arn/arn.go:34`, takes a bare `service`
string parameter with no MGN-specific handling needed since it's a regional, non-global service
like the vast majority already special-cased only for `"iam"`). The exact resource-path segment
for each of the 12 taggable kinds (e.g. `source-server/<id>`, `application/<id>`) is an HONEST
UNKNOWN, not fabricated here — an implementer should verify before hardcoding, following the same
caution the outposts/directconnect audits flagged for their own under-confirmed resource segments.

**EC2/EBS/IAM/KMS/VPC integration for real cutover.** This repo has real, working backends for
every piece MGN's cutover would touch:
- EC2 instance launch: `services/ec2/handler_instances_lifecycle.go:119` (`handleRunInstances`).
- EBS snapshots: `services/ec2/handler_snapshots.go`.
- IAM roles: `services/iam/handler_roles.go` (`func.*CreateRole`).
- KMS: `services/kms/` (full package exists — `EbsEncryptionKeyArn`/`ParametersEncryptionKey` on
  `ReplicationConfiguration`/`LaunchConfigurationTemplate` could resolve against it).
- VPC subnets/security groups: `services/ec2/store.go` (Subnet/SecurityGroup types exist in the
  same EC2 backend RunInstances already reads).

A real implementation **could** launch actual gopherstack EC2 instances from
`ReplicationConfiguration.ReplicationServerInstanceType`/`StagingAreaSubnetId`/
`ReplicationServersSecurityGroupsIDs` (for the replication server, conceptually — though a
replication server is itself an AWS-internal implementation detail never exposed via any field on
`SourceServer`/`Job`, so simulating it may not even be observable to a caller and could be pure
internal bookkeeping) and from `LaunchConfiguration`'s settings on Job completion (for the actual
`LaunchedInstance.Ec2InstanceID`), producing a real, listable EC2 instance rather than an invented
ID string. This is a substantial, real cross-service integration — validating subnet/security-group
IDs against `services/ec2`'s store, actually calling into EC2's RunInstances path, and populating
`LaunchedInstance.Ec2InstanceID` with a real instance ID this repo's own `DescribeInstances` would
then return. Flagged here as a concrete, buildable follow-on, explicitly NOT required for a
first-pass, honestly-gapped implementation (a first pass can validate the ID format and store it
without cross-checking existence, clearly documented as such).

**Grep results for "mgn"/"migration".** `grep -rni "\bmgn\b" services/ cli.go` (word-boundary,
excluding false positives like "management") returns **zero hits**. `grep -rli "migration"
services/ cli.go` returns hits only in `services/dms/*` (AWS Database Migration Service — an
entirely different, already-implemented AWS product), `services/opensearch/*migrations*` (index
migration, unrelated), `services/elasticache/handler_replication_groups.go` (cache replication,
unrelated), and `services/ec2/handler_instance_attrs.go`/`services/waf/handler_web_acls.go` (both
false-positive substring matches on unrelated concepts, confirmed by reading context) — none
reference AWS Application Migration Service. `grep -rli "SourceServer\|StartCutover\|
ReplicationConfigurationTemplate" services/ cli.go` returns only unrelated false positives
(cognitoidp's `ResourceServer`, elasticache's `Serverless`) — confirmed zero real MGN-adjacent
state anywhere in this tree.

**CloudFormation.** `grep -rli "mgn\b" services/cloudformation/` returns zero hits across all 71
`resources_*.go` files in that directory — no `AWS::MGN::*` resource type exists in this repo. This
audit did not independently verify whether AWS's own real CloudFormation supports any MGN resource
type at all (MGN's agent-driven, non-declarative nature makes broad CFN support unlikely, matching
the directconnect/outposts pattern, but that claim is about AWS's product, not this repo's tree).

## Top 5 hardest/riskiest things about implementing this service

1. **No public op creates a `SourceServer`, a `VcenterClient`, or a `NetworkMigrationExecutionID`.**
   Every one of `StartTest`/`StartCutover`/`ChangeServerLifeCycleState`/the entire replication
   family operates on a `SourceServerID` that must already exist, but the only public creation path
   is `StartImport`'s bulk metadata load (itself schema-unspecified) — real AWS creates them via an
   internal agent-registration call this SDK does not expose at all. Testing this service at all
   requires inventing a seeding mechanism and being explicit that it is a gopherstack-only
   convenience, not a simulation of AWS's real onboarding flow. The same problem applies,
   independently, to `VcenterClient` (no create op) and to `NetworkMigrationExecutionID` (required
   input to 5 different Start* ops, created by none of them).
2. **Two structurally distinct wire "generations" coexist in one service** (69 legacy ops with
   `UninitializedAccountException`/no `ThrottlingException`, versus the tagging trio + 25
   `/network-migration/` ops with the reverse) — an implementer building one shared error-mapping
   table risks silently blending them if not built from the actual per-op extraction in this
   document.
3. **The flattened-vs-nested output-shape split** (11 SourceServer-mutation ops flatten the full
   SourceServer onto their Output; `StartTest`/`StartCutover`/`TerminateTargetInstances` instead
   nest a `Job`; `GetLaunchConfiguration`/`GetReplicationConfiguration` flatten their own distinct
   shapes with no backing named type at all) is exactly the kind of "looks like it should share one
   serializer, doesn't" trap parity-principles.md calls out — a router or generic-response helper
   written by extrapolating from a handful of sampled ops will get several of these wrong.
4. **The Network Migration sub-product (25 of 95 ops) requires either a real network-analysis/
   code-generation engine or an explicit, prominent decision to keep its analysis/codegen/
   deployment CONTENT as placeholder text while still honestly progressing its status enums** —
   there is no middle ground that isn't either unbuildable or fabrication, and the temptation to
   generate "plausible-looking" analysis findings or generated code to make the emulator feel more
   complete is exactly the failure mode this campaign's honesty rules exist to prevent.
5. **The ARN resource-path format for all 12 taggable resource kinds is unconfirmed** — Terraform's
   AWS provider has literally zero MGN resources to check against (unlike directconnect/outposts,
   where at least partial Terraform-source corroboration existed), and AWS's own docs pages
   returned only a JS shell to automated fetching. Only the ARN *service segment* (`"mgn"`) has
   indirect corroboration (botocore's endpoint metadata); every specific resource-path segment is
   this audit's best-effort guess from convention, not a confirmed value, and should be verified
   independently before an implementer hardcodes it.

## gopherstack-21my per-item typed round-trip pass (2026-08-31)

mgn was one of the eighteen services in gopherstack-21my marked "clean at
wrapper level, never swept per-item." Per that issue's own finding (rds's
DescribeDBInstances read clean by hand the session before a real bug was
found underneath it), this pass writes typed round-trip tests instead of
reading `deserializers.go` by eye, on top of this service's already
extensive `sdk_roundtrip_test.go` suite (16 top-level tests).

**Covered, new this pass** (`sdk_roundtrip_nested_test.go`):
- `DescribeSourceServers` -- `DataReplicationInfo.ReplicatedDisks[]` and
  `DataReplicationInfo.DataReplicationInitiation.Steps[]`, two nested lists
  inside each item that no prior test asserted on (the existing suite only
  checked `SourceProperties.IdentificationHints.Hostname`). Seeded two
  source servers via a real two-row `StartImport` CSV, waited for both to
  reach `DataReplicationState` `CONTINUOUS` via this backend's own
  deterministic replication timer, and asserted every field of the
  replicated disk and all 12 initiation steps via the real SDK client.
  `TestSDKRoundTrip_ReplicationNestedLists`. **Result: clean** -- matches
  `backloggedStorageBytes`/`deviceName`/`replicatedStorageBytes`/
  `rescannedStorageBytes`/`totalStorageBytes`/`name`/`status` confirmed
  against `awsRestjson1_deserializeDocumentDataReplicationInfoReplicatedDisk`/
  `...DataReplicationInitiationStep` (`mgn@v1.48.4` deserializers.go) before
  writing the test.
- `DescribeJobs` -- `Job.ParticipatingServers[]`, asserted with two
  distinguishable participants (real `StartTest` against two seeded source
  servers) rather than the existing suite's single-participant checks.
  `TestSDKRoundTrip_JobParticipatingServers`. **Result: clean.**

**Not covered this pass**: `Cpus`/`Disks`/`NetworkInterfaces`/`RAMBytes`/
`RecommendedInstanceType`/`Os` on `SourceProperties` -- confirmed a genuine
gap, not a bug: `parseSourceServerRow` (s3import.go) is the only public path
that ever creates a `SourceServer`, and it only ever populates
`IdentificationHints` from the CSV row; there is no public op (real AWS's
own agent-registration API is not exposed by this SDK either) that could set
these fields, so no legal input can exercise their decode path. Recorded
per this issue's "no legal input could change the outcome" restraint rule --
not fabricated.

`DescribeVcenterClients`/`DescribeReplicationConfigurationTemplates`/
`DescribeLaunchConfigurationTemplates`/`ListConnectors`/network-migration
list ops already have real-client coverage from the pre-existing
`sdk_roundtrip_test.go` suite (`TestRoundTrip_Connectors`,
`TestRoundTrip_VcenterClients`, `TestRoundTrip_ConfigTemplates`,
`TestRoundTrip_NetworkMigrationDefinitions`,
`TestRoundTrip_NetworkMigrationAnalysisAndDeployment`) but were not
re-verified against the pinned SDK source in this pass; scalar/flat fields
only in those shapes (no un-asserted nested lists identified).

**Test-file exposure**: of 11 `*_test.go` files in this service, 8 drive a
real typed `aws-sdk-go-v2` client (`newRoundTripClient`/`NewFromConfig`) --
this service is already unusually well-instrumented compared to the
~15-20% typical elsewhere in this campaign.

Gates: `go build ./services/mgn/...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/mgn/...` (pass), `golangci-lint run
./services/mgn/...` (0 issues).

## 2026-08-31 correction: round-trip tests proven failable (gopherstack-21my)

The commit that added `sdk_roundtrip_nested_test.go` (`9c4a92608`) carried a
caveat saying the tests had never been observed to fail, and that a perturbation
of `DataReplicationInfoReplicatedDisk.DeviceName` in `models.go` had not broken
one. Both halves of that caveat were wrong in ways worth recording.

The perturbation was inert because `models.go` types are never marshalled to the
wire. Every response converts them to a tagged type in `wire.go` first, and only
those tags reach a client. The json tags that do appear on `models.go` types in
other services belong to on-disk snapshot persistence, not the AWS protocol.

All four round-trip tests are now proven failable against the struct that
actually reaches the wire, each by breaking one wire tag, observing the decoded
value come back empty, and restoring.

The caveat also claimed the SDK decoder matches keys case-insensitively, which
would have hidden naming mistakes. That is false for this service. smithy-go's
JSON decoder does no case folding at all, so a casing mismatch here is a hard
bug rather than something tolerated. It is true only of the XML decoder, which
does fold - so REST-XML services would silently tolerate a case-only element
mismatch, and a test passing there is weaker evidence than the same test passing
on a JSON protocol.

## 2026-09-01 wrapper-key sweep (gopherstack-6flj)

Full sweep of all 30 `Describe*`/`List*` collection ops (mgn@v1.48.4 pinned
SDK; count derived from `$GOMODCACHE`, not grepped from this repo's own
source). Protocol is `awsRestjson1` throughout, confirmed by
`addOperationXMiddlewares` on every op -- no `restxml` single-payload
shortcut applies here; every top-level key is checked strictly by
`deserializeOpDocument<Op>Output`, case-sensitive, no fold.

**Layer 1 (top-level wrapper key): clean, 30/30.** mgn follows one uniform
convention -- generic `items`/`nextToken` on every list op except
`ListTagsForResource` (`tags`, a map). Handler struct tags in `wire.go`
match byte-exact on all 30.

**Layer 2 (per-item field completeness): 21 item/nested types checked
field-by-field against their real SDK struct
(`SourceServer`, `Job`, `Wave`, `Application`,
`LaunchConfigurationTemplate`, `ReplicationConfigurationTemplate`, `JobLog`,
`ImportTaskError`, `ManagedAccount`, `SourceServerActionDocument`,
`TemplateActionDocument`, `VcenterClient`, `Connector`, `ExportTask`,
`ImportTask`, `ImportFileEnrichment`,
`NetworkMigrationDefinitionSummary`, `NetworkMigrationMappingJobDetails`,
`NetworkMigrationMappingUpdateJobDetails`, `ParticipatingServer`, and the
shared `NetworkMigrationAnalysisJobDetails`/`NetworkMigrationDeployerJobDetails`
pair backing `networkMigrationJobDetailsWire`).

**One bug found and fixed.** `NetworkMigrationCodeGenerationJobDetails` is
the one exception to the "all five NM job-details types are identical"
claim this file's own doc comment made (`networkmigrationjobs.go`, now
corrected) -- it alone also carries
`CodeGenerationOutputFormatStatusDetailsMap`, keyed by the format types the
caller requested on `StartNetworkMigrationCodeGeneration`
(`CodeGenerationOutputFormatTypes`, `serializers.go:6983`). gopherstack
never read that request field and the shared
`networkMigrationJobDetailsWire`/`NetworkMigrationJob` never carried it, so
`ListNetworkMigrationCodeGenerations` could never populate the map --
silently missing, not wrong-keyed. Fixed: `NetworkMigrationJob` now tracks
`CodeGenerationOutputFormatTypes`; `ListNetworkMigrationCodeGenerations`
surfaces one map entry per requested format once the job reaches
SUCCEEDED/FAILED, its `Status` mirroring the job's own (this backend never
partially fails one format). `StatusDetailList` stays empty -- no per-format
detail text exists to report, matching this service's existing restraint on
not fabricating codegen content. Proven via
`TestRoundTrip_NetworkMigrationCodeGenerationOutputFormatStatus`
(`sdk_roundtrip_test.go`), confirmed failing (empty map) against the
unmodified handler/backend/wire before the fix.

**Also checked and confirmed NOT a bug (restraint, not a gap):**
`ParticipatingServer.PostLaunchActionsStatus` (nullable, per-instance
post-launch-action execution status) is genuinely unmodeled -- this backend
tracks configured `PostLaunchActions` (what to run) but never simulates
running them, so it has no real status to report; omitting the optional
field is honest, not a silent-empty-list bug. `ListNetworkMigrationAnalysisResults`/
`CodeGenerationSegments`/`DeployedStacks`/`MapperSegmentConstructs`/
`MapperSegments` staying permanently empty is the same documented,
deliberate limitation this file already records (no topology/codegen/deploy
engine exists) -- re-verified rather than trusted on the existing comment,
and it held.

Gates: `go build ./...`, `go vet ./services/mgn/...`, `go test -race
-count=1 ./services/mgn/...` (pass, includes the new test),
`golangci-lint run ./services/mgn/...` (0 issues after adding one `//nolint:lll`
on the new map field's struct tag -- long field/type name, not a suppressed
bug), `go test ./pkgs/persistence/... -run TestSnapshotVersionGuard`
(additive-only field on `NetworkMigrationJob`, no version bump required;
golden refreshed with `-update` and re-run clean).
