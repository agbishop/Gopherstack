---
service: ecs
sdk_module: aws-sdk-go-v2/service/ecs@v1.96.0
last_audit_commit:                                # unknown: pass ran without git access at write time, never backfilled -- gopherstack-33in
last_audit_date: 2026-07-31
overall: A            # A = genuine fix found (wire-shape bug); B = already-accurate, proven op-by-op
ops:
  CreateCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "added capacityProviders/defaultCapacityProviderStrategy/tags at creation (previously silently dropped); tags echoed on create response; this sweep: defaultCapacityProviderStrategy now validated (rejects unknown capacity provider names, see PutClusterCapacityProviders note)"}
  DescribeClusters: {wire: ok, errors: ok, state: ok, persist: ok, note: "added include=[TAGS] gating (was previously unsupported; tags were never returned). FIXED (value-semantics sweep, gopherstack-uox6): DescribeClustersInput.Clusters docs 'If you do not specify a cluster, the default cluster is assumed' -- an empty Clusters list returned EVERY cluster in the account instead, because the backend method was reused (via `b.DescribeClusters(nil)`) as ListClusters' own implementation, and ListClusters (a different operation, no such default-substitution language) legitimately does return everything. Decoupled: ListClusters now enumerates b.clusters directly; DescribeClusters([]) now describes only the 'default' cluster (auto-vivified via the same ensureClusterLocked lazy-creation already used by RunTask/CreateService/RegisterContainerInstance, so a fresh account's implicit default cluster is describable exactly as real AWS's always is). Two existing tests asserted the old 'empty returns all' behavior and were corrected. Proven by the corrected TestECS_DescribeClusters/empty_describes_default_cluster_only and TestDescribeClusters_FailureSemantics (fail without the fix)."}
  DeleteCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "cascade delete of serviceDeployments fixed (was keyed wrong, silently a no-op); this sweep: also cascade-cleans the resourceTags side-map entry for the cluster itself plus every cascade-deleted service/container-instance (previously a ghost row that could resurrect stale tags on a same-name recreate, or leak permanently for random-ID resources -- see Notes)"}
  ListClusters: {wire: ok, errors: ok, state: ok, persist: ok, note: "decoupled from DescribeClusters this sweep (gopherstack-uox6) -- see DescribeClusters note; behavior unchanged (still returns every cluster, matching ListClustersInput, which has no default-substitution language)."}
  UpdateCluster: {wire: ok, errors: ok, state: ok, persist: ok, note: "CORRECTED this sweep: the 2026-07-26 entry's wire:ok claim was false -- updateClusterInput accepted capacityProviders and defaultCapacityProviderStrategy, and validated the latter (see the entry this replaces), but the real UpdateClusterRequest has neither field (only cluster, settings, configuration, serviceConnectDefaults); capacity-provider association is exclusively PutClusterCapacityProviders's job. A real typed SDK client could never have exercised this surface. Both fields removed from the handler input struct and from UpdateClusterInput/Backend.UpdateCluster; sending them now is silently ignored rather than applied, proven by TestUpdateCluster_DoesNotAcceptCapacityProviders (fails without the fix). configuration and serviceConnectDefaults remain unmodeled (pre-existing, not part of this fix)."}
  UpdateClusterSettings: {wire: ok, errors: ok, state: ok, persist: ok}
  PutClusterCapacityProviders: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED prior sweep: defaultCapacityProviderStrategy items are now validated against real (created via CreateCapacityProvider) or FARGATE/FARGATE_SPOT-builtin capacity providers, returning a 400 ClientException for an unknown name instead of silently accepting any string. Same validateCapacityProviderStrategyLocked helper wired into CreateCluster, CreateService, UpdateService, RunTask, and CreateTaskSet. CORRECTED this sweep: the prior note also claimed UpdateCluster was wired into this validation; that was true of the code at the time, but UpdateCluster's capacityProviders/defaultCapacityProviderStrategy fields were themselves a wire-shape bug (see UpdateCluster entry) and have since been removed, so UpdateCluster is no longer part of this list. Scoped narrowly: only strategy items are validated, not the separate capacityProviders association list (see gaps)."}
  RegisterTaskDefinition: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeTaskDefinition: {wire: ok, errors: ok, state: ok, persist: ok, note: "include=[TAGS] already supported pre-sweep"}
  DeregisterTaskDefinition: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteTaskDefinitions: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: also cleans the resourceTags side-map entry per deleted revision (previously a permanent ghost row, see Notes)"}
  ListTaskDefinitions: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (order-bug sweep): default order was sort.Strings on the full ARN, which compares revision as a string ('family:10' < 'family:2') -- wrong once a family passes revision 9, and the request's sort param (ASC/DESC) was dropped entirely, not even in the input struct. AWS documents 'by default (ASC) task definitions are listed lexicographically by family name and in ascending numerical order by revision' (api_op_ListTaskDefinitions.go). Now sorts by (Family, Revision) with Revision compared numerically, and Sort is threaded through and applied. Proven by TestECS_ListTaskDefinitions_Order (11 revisions across two families, fails without the fix)."}
  ListTaskDefinitionFamilies: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateService: {wire: ok, errors: ok, state: ok, persist: ok, note: "now records a real ServiceDeployment for the initial PRIMARY deployment (was a disguised stub, see gaps/fixes); capacityProviderStrategy validated (see PutClusterCapacityProviders note). FIXED gopherstack-rnka: tags supplied at creation now mirrored into the resourceTags side map (was two never-synced copies -- see TagResource note and Notes)."}
  DescribeServices: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED gopherstack-rnka: added include=[TAGS] gating (previously tags were always returned unconditionally, unlike DescribeClusters/DescribeCapacityProviders/DescribeContainerInstances/DescribeTaskSets/DescribeExpressGatewayService, which already gated correctly); tags now sourced from the resourceTags side map via ListTagsForResource, not the stale Service.Tags snapshot."}
  UpdateService: {wire: ok, errors: ok, state: ok, persist: ok, note: "now syncs ServiceDeployment records when rotating the PRIMARY deployment; capacityProviderStrategy validated (see PutClusterCapacityProviders note). FIXED gopherstack-rnka: response tags now read from the resourceTags side map (authoritative) instead of the stale creation-time snapshot."}
  DeleteService: {wire: ok, errors: ok, state: ok, persist: ok, note: "now cleans up its ServiceDeployment records (was leaking one entry per deleted service); also cleans its resourceTags side-map entry (previously a ghost row, see Notes). FIXED gopherstack-rnka: response echoes the final resourceTags-authoritative tag set, captured before the side-map entry is cleared."}
  ListServices: {wire: ok, errors: ok, state: ok, persist: ok}
  ListServicesByNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateTaskSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: added capacityProviderStrategy (was entirely absent from both CreateTaskSetInput and the TaskSet wire shape -- a real SDK field, now validated + stored + echoed) and tags (stored via the resourceTags side map, echoed unconditionally on Create like CreateCluster)"}
  DeleteTaskSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: also cleans the task set's resourceTags side-map entry (previously a permanent ghost row for every tagged task set ever deleted, see Notes)"}
  DescribeTaskSets: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: added include=[TAGS] gating (tags previously had no wire-shape field at all) and capacityProviderStrategy in the response (see CreateTaskSet note)"}
  UpdateTaskSet: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateServicePrimaryTaskSet: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeServiceRevisions: {wire: ok, errors: ok, state: ok, persist: ok, note: "derived on read from Service.Deployments, not separately stored — intentional (see Notes)"}
  DescribeServiceDeployments: {wire: ok, errors: ok, state: ok, persist: ok, note: "was a disguised stub: filtered a map only the AddServiceDeploymentInternal test seed ever populated. Fixed by syncServiceDeploymentsLocked."}
  ListServiceDeployments: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed (gopherstack-7ux2): returned a wholly wrong shape -- a bare serviceDeploymentArns string list instead of ServiceDeployments ([]types.ServiceDeploymentBrief), so a real client decoded nothing. Now returns Brief objects sourced from ServiceDeployment: ClusterArn/ServiceArn/ServiceDeploymentArn/Status/StatusReason/CreatedAt direct; StartedAt mirrors CreatedAt (the real full ServiceDeployment type has no separate started timestamp either, only CreatedAt/FinishedAt); FinishedAt is UpdatedAt when Status is terminal (SUCCESSFUL/STOPPED), absent otherwise; TargetServiceRevisionArn newly threaded from Deployment.ServiceRevisionArn (was tracked on Deployment but never copied onto ServiceDeployment). Alarms/DeploymentCircuitBreaker/DeploymentConfiguration remain absent -- not modeled on ServiceDeployment, nothing honest to source them from. FIXED (order-bug sweep): also read via b.serviceDeployments.All(), whose documented contract (pkgs/store/table.go) is unspecified (Go map) iteration order -- no sort was applied, so two calls with no mutation in between could differ. AWS documents no order for this op; now sorted by ServiceDeploymentArn, matching the sibling ListDaemonDeployments convention. Proven by TestECS_ListServiceDeployments_StableOrder."}
  StopServiceDeployment: {wire: ok, errors: ok, state: ok, persist: ok, note: "same fix; also now really has data to stop. FIXED 2026-08-30 (gopherstack-101r, fabricated-error-code sweep): the already-STOPPED case emitted 'ServiceDeploymentAlreadyStoppedException', which names no type in ecs@v1.90.0 (absent from types/errors.go and from awsAwsjson11_deserializeOpErrorStopServiceDeployment's switch) -- a prior hand sweep fixed eleven other fabricated codes in this service but missed this twelfth. StopServiceDeployment's own deserializer models ConflictException ('conflict in the current state of the resource'), now used instead. TestStopServiceDeployment_AlreadyStopped_RealClient (error_code_fixes_ecssweep_test.go) confirmed failing pre-fix against the real typed SDK client."}
  ContinueServiceDeployment: {wire: ok, errors: ok, state: partial, persist: n/a, note: "NEW op (was entirely unimplemented / absent from GetSupportedOperations). Lifecycle hooks (blue/green PAUSE stages) are not modeled, so every call returns an honest ClientException that no paused hook exists, after real ARN/hookId validation — never a fabricated success. See gaps."}
  RunTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: added capacityProviderStrategy input (was entirely absent from RunTaskInput -- a real SDK field, now validated) and capacityProviderName output on Task (real SDK field; this backend does not model AWS's weight/base task-distribution algorithm across multiple providers in a strategy, so it always selects the first entry -- documented simplification, not a stub, see Task.CapacityProviderName doc comment in models.go). Per-item failure sweep: RunTaskOutput.Failures (api_op_RunTask.go) is checked but left unpopulated -- runTaskOutput has no Failures field on the wire and every requested task is always placed. Deliberately left: real RunTask.Failures reports pre-placement capacity/constraint failures (e.g. insufficient cluster resources for a subset of the requested count), and this backend does not model cluster resource capacity at all, so no real client input can cause a subset of a RunTask batch to fail placement while the rest succeed -- there is nothing to be dishonest about yet."}
  StartTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (per-item failure sweep): the handler hardcoded Failures to an empty slice and the backend created a task for every requested container instance ARN unconditionally, including ones never registered in the cluster -- StartTaskOutput.Failures (api_op_StartTask.go) was never populated, so a client asking to place a task on a stale/mistyped instance ARN got back a fabricated running task instead of the documented failure. Now unknown ARNs are reported as Failure{Reason: MISSING} and only valid ones get a task, matching the sibling batch-describe ops. Proven by TestStartTask_UnknownContainerInstance_ReportsFailure (fails without the fix)."}
  DescribeTasks: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep (gopherstack-s1u9): a Docker-runner task that finished on its own (its container exited without an explicit StopTask) previously stayed RUNNING forever -- LastStatus only ever reached STOPPED via StopTask. See gaps for the fix."}
  StopTask: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTasks: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (value-semantics sweep, gopherstack-uox6): ListTasksInput.DesiredStatus docs 'The default status filter is RUNNING' -- an omitted desiredStatus returned tasks of every status (RUNNING and STOPPED both), not RUNNING only. Fixed in the shared ListTasksFiltered (also used by the plain ListTasks(cluster) Backend-interface convenience method, which is now correctly RUNNING-only by default like the real op). Three internal tests relied on the old widen-on-empty behavior to see STOPPED tasks (a janitor test, a circuit-breaker task-count test) and were corrected to query DesiredStatus explicitly rather than relying on the default. Proven by TestECS_ListTasks_DesiredStatusDefaultsToRunning (fails without the fix). Gap: daemonName (a distinct documented ListTasksInput filter) is not declared/read at all -- other axis (never-read), not fixed here."}
  RegisterContainerInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "CORRECTED this sweep: the prior wire:ok claim was false -- registerContainerInstanceInput required ec2InstanceId, a field that does not exist on the real RegisterContainerInstanceRequest (only instanceIdentityDocument/instanceIdentityDocumentSignature, plus cluster/attributes/tags/versionInfo/etc.); no real typed SDK client could ever populate it. Fixed by accepting instanceIdentityDocument instead and deriving the EC2 instance ID by parsing its instanceId JSON field (the real document served at the EC2 instance-metadata identity-document endpoint, which real ECS also derives instance identity from). If the document is absent or does not parse, EC2InstanceID is left empty rather than fabricated -- an honest 'could not identify' rather than a plausible-looking invented ID. instanceIdentityDocumentSignature is accepted for wire-shape completeness but not cryptographically verified (this backend does not model EC2 instance-identity attestation). attributes/tags/versionInfo/totalResources/containerInstanceArn/platformDevices on the real request are not modeled at registration time (attributes/tags are already reachable via the separate PutAttributes/TagResource operations); out of scope for this fix, not claimed as done."}
  DeregisterContainerInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: also cleans the container instance's resourceTags side-map entry (previously a ghost row, see Notes)"}
  DescribeContainerInstances: {wire: ok, errors: ok, state: ok, persist: ok, note: "this sweep: added include=[TAGS] gating (tags previously had no wire-shape field at all). Remaining gap: CONTAINER_INSTANCE_HEALTH include value / HealthStatus field not modeled -- no health-check state is tracked for container instances (niche, not in the original gap list, deferred)"}
  ListContainerInstances: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateContainerInstancesState: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (per-item failure sweep): an unknown container instance ARN aborted the whole batch with a request-level InvalidParameterException, and the wire output type had no Failures field at all, though api_op_UpdateContainerInstancesState.go models UpdateContainerInstancesStateOutput.Failures -- a client draining N instances lost the state change on every valid instance because one stale ARN was in the same request. Now valid instances still transition and unknown ones are reported per-item as Failure{Reason: MISSING}, matching the sibling DescribeContainerInstances/DescribeClusters pattern. Two existing tests asserted the old top-level-error behavior as correct (TestECS_UpdateContainerInstancesState_NotFound, error_code_fixes_ecssweep_test.go's UpdateContainerInstancesState subtest) and were updated to assert the per-item Failures shape instead. Proven by TestUpdateContainerInstancesState_UnknownInstance_ReportsFailure (fails without the fix)."}
  UpdateContainerAgent: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateCapacityProvider: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteCapacityProvider: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeCapacityProviders: {wire: ok, errors: ok, state: ok, persist: ok, note: "unknown names report a Failures[] entry (reason MISSING) instead of failing the whole call (fixed prior sweep). This sweep: added include=[TAGS] gating (tags were previously always returned regardless of Include) and the Cluster filter parameter (only capacity providers associated with the named cluster are returned; unknown cluster -> empty result, matching AWS filter-parameter semantics rather than a hard 404)."}
  UpdateCapacityProvider: {wire: ok, errors: ok, state: ok, persist: ok, note: "CORRECTED this sweep: the prior wire:ok claim was false -- updateCapacityProviderInput accepted status and tags, neither of which exist on the real UpdateCapacityProviderRequest (only name, cluster, autoScalingGroupProvider, managedInstancesProvider), and its autoScalingGroupProvider reused the full create-time type (with autoScalingGroupArn) instead of the narrower AutoScalingGroupProviderUpdate (managedScaling/managedTerminationProtection/managedDraining only -- the ASG cannot be swapped after creation). Both fixed: status/tags removed from the input struct; a new AutoScalingGroupProviderUpdate type (no ARN) added end-to-end. This also fixed a latent state bug the old wire shape masked: the backend did `cp.AutoScalingGroupProvider = input.AutoScalingGroupProvider` wholesale, so any update touching autoScalingGroupProvider silently zeroed the stored ARN (the old input struct always carried an ARN field, usually empty on update calls that only meant to change managedScaling). Now merges managedScaling/managedTerminationProtection/managedDraining onto the existing AutoScalingGroupProvider, preserving its ARN -- proven by TestCapacityProvider_Update_ManagedScaling (fails without the fix). managedInstancesProvider is not modeled (no Managed Instances feature in this backend)."}
  DeleteAccountSetting: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAccountSettings: {wire: ok, errors: ok, state: ok, persist: ok}
  PutAccountSetting: {wire: ok, errors: ok, state: ok, persist: ok}
  PutAccountSettingDefault: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  ListAttributes: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (order-bug sweep): built its response by ranging b.attributes[cluster] (a map[string]*Attribute) directly with no sort, so order was raw Go map order -- can differ between two calls with no mutation in between. AWS documents no order for this op; now sorted (Name, TargetID) for a stable, testable result. Proven by TestECS_ListAttributes_StableOrder."}
  PutAttributes: {wire: ok, errors: ok, state: ok, persist: ok}
  ExecuteCommand: {wire: ok, errors: ok, state: ok, persist: n/a}
  GetTaskProtection: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateTaskProtection: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "resourceTags side map was NOT in backendSnapshot at all — fixed, see gaps/fixes. Fixed a real bug where TagResource on an Express Gateway Service ARN silently never became visible on Describe or ListTagsForResource (see ExpressGatewayService notes below). FIXED gopherstack-rnka: the identical disconnect for ordinary Service ARNs (Service.Tags was a creation-time-only snapshot, never synced with resourceTags, and RunTask's propagateTags=SERVICE path read the stale snapshot too) is now closed -- see CreateService/UpdateService/DeleteService/DescribeServices notes. Re-checked this pass (wrapper-key sweep) against the sfn TagResource map/array bug class: ecs's TagResourceInput.Tags is []types.Tag, array of {key,value} (api_op_TagResource.go:82, serializers.go:8688-8700), matching this emulator's []Tag{Key,Value} exactly -- genuinely clean, confirmed via a real-client round-trip test (tag_resource_sdk_test.go)."}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateExpressGatewayService: {wire: ok, errors: ok, state: ok, persist: ok, note: "tags supplied at creation are mirrored into the resourceTags side map (previously only stored on the ExpressGatewayService.Tags struct field, never synced -- see Notes for the TagResource-invisibility bug this caused). FIXED gopherstack-rnka: input now carries the full real CreateExpressGatewayServiceInput surface -- Cpu, Memory, HealthCheckPath, ExecutionRoleArn, NetworkConfiguration, PrimaryContainer, ScalingTarget, TaskDefinitionArn, TaskRoleArn -- validated (taskDefinitionArn is mutually exclusive with primaryContainer/executionRoleArn/taskRoleArn/cpu/memory, matching the real API) and stored as the service's first ActiveConfigurations revision, not silently dropped."}
  DeleteExpressGatewayService: {wire: ok, errors: ok, state: ok, persist: ok, note: "also cleans the service's resourceTags side-map entry (previously a ghost row, see Notes)"}
  DescribeExpressGatewayService: {wire: ok, errors: ok, state: ok, persist: ok, note: "include=[TAGS] gating implemented (tags previously always returned regardless of Include); tags read from the resourceTags side map (kept in sync by TagResource/UntagResource) instead of a stale creation-time snapshot -- see Notes. FIXED gopherstack-rnka: response now carries ActiveConfigurations (full per-revision Cpu/Memory/HealthCheckPath/NetworkConfiguration/PrimaryContainer/ScalingTarget/TaskDefinitionArn/TaskRoleArn/ServiceRevisionArn/IngressPaths), CurrentDeployment, and UpdatedAt, matching types.ECSExpressGatewayService; Status is now the correct nested {statusCode, statusReason} object (types.ExpressGatewayServiceStatus) instead of an invented flat string, and the invented top-level ExecutionRoleArn field (which does not exist on the real type) was removed -- it now lives only on each ActiveConfigurations entry, where the real SDK actually puts it."}
  UpdateExpressGatewayService: {wire: ok, errors: ok, state: ok, persist: ok, note: "tags read from the resourceTags side map (see DescribeExpressGatewayService note; Update itself never accepted a tags parameter, matching real UpdateExpressGatewayServiceInput, which has no Tags field). FIXED gopherstack-rnka: input now carries the full real UpdateExpressGatewayServiceInput surface (same field set as Create, see above) and the response is the correct, narrower UpdatedExpressGatewayService shape (Cluster/CreatedAt/ServiceArn/ServiceName/Status/TargetConfiguration/UpdatedAt -- NOT the same shape as Describe/Create/Delete's ECSExpressGatewayService; notably no top-level InfrastructureRoleArn or Tags on this response, matching the real SDK). Each Update rolls out a brand-new ActiveConfigurations revision (new ServiceRevisionArn) rather than mutating the prior one in place, matching real AWS's service-revision model."}
  DiscoverPollEndpoint: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED (gopherstack-h0x1) — request field names were ClusterArn/ContainerInstanceArn; real DiscoverPollEndpointInput members are Cluster/ContainerInstance (wire keys \"cluster\"/\"containerInstance\", ecs v1.90.0 serializers.go:10302-10314). Renamed. Still INERT: the handler discards its input entirely (only ever returns the region's fixed endpoints), so this had and still has no observable behavior difference -- fixed only so a future change that consults the input isn't silently broken by the wrong field names."}
  SubmitAttachmentStateChanges: {wire: ok, errors: ok, state: ok, persist: ok}
  SubmitContainerStateChange: {wire: ok, errors: ok, state: ok, persist: ok}
  SubmitTaskStateChange: {wire: ok, errors: ok, state: ok, persist: ok}
families:
  daemon: {status: ok, note: "Field-diffed for real (previous ledger entries for this family were no-stub-only assessments, not wire-shape diffs). Fixed a real leak: DeleteDaemon never cleaned up daemonRevisions/daemonDeployments rows at all (only the daemons table entry), and the cluster-purge cleanup path (purgeDaemonsLocked) deleted from daemonRevisions by the wrong key (DaemonArn instead of DaemonRevisionArn, a documented-but-never-fixed no-op preserved through a prior mechanical refactor) -- both fixed via a new shared deleteDaemonAncillaryLocked helper. CORRECTED gopherstack-rnka: the prior ledger entry here (2026-07-23) claimed DescribeDaemonOutput.Daemon was flattened -- daemonName/daemonTaskDefinitionArn/capacityProviderArns/tags/etc. living directly on the response instead of nested under CurrentRevisions -- and downgraded this family to partial on that basis. Re-verified against the real types.DaemonDetail shape (ClusterArn/CreatedAt/CurrentRevisions[]DaemonRevisionDetail{Arn,CapacityProviders[]DaemonCapacityProvider{Arn,RunningCount},TotalRunningCount}/DaemonArn/DeploymentArn/Status/UpdatedAt) field-by-field: handler_daemon.go's daemonDetailView/daemonRevisionDetailView/daemonCapacityProviderView already match this exactly, and DO NOT expose daemonName/daemonTaskDefinitionArn/tags/etc. at the top level. Proven with a new real-SDK-client round-trip test (TestECS_DescribeDaemon_SDKRoundTrip_RevisionNesting) rather than trusting the prior note. The 2026-07-23 gap description was inaccurate at the time it was written (the code was already correct); upgraded back to ok. FIXED (order-bug sweep): ListDaemonTaskDefinitions had the same numeric-vs-lexicographic bug as ListTaskDefinitions -- it sorted by the full ARN string ('daemon-task-definition/family:10' < '...:2'), wrong once a family passes revision 9, though unlike ListTaskDefinitions the request's Sort field WAS threaded through and applied (as a post-hoc reversal of the already-wrong order). AWS documents 'by default (ASC), daemon task definitions are listed in ascending order by family name and revision number' (api_op_ListDaemonTaskDefinitions.go). Now sorts by (Family, Revision) with Revision compared numerically before Sort=DESC reverses it. Proven by TestECS_ListDaemonTaskDefinitions_Order. FIXED (value-semantics sweep, gopherstack-uox6): ListDaemons had the same DescribeClusters-shaped bug -- ListDaemonsInput.ClusterArn docs 'If you do not specify a cluster, the default cluster is assumed', but an empty ClusterArn returned daemons from every cluster in the account instead of scoping to 'default'. Fixed by routing through the same resolveCluster helper every other Cluster-defaulting op uses. Proven by TestECS_ListDaemons_OmittedClusterScopesToDefault (fails without the fix). Gap: ListDaemonDeploymentsInput.CreatedAt (a documented time-range filter) is not declared/read at all -- other axis (never-read), not fixed here."}
gaps:
  - "PutClusterCapacityProviders/CreateService/UpdateService/RunTask/CreateCluster/CreateTaskSet do not validate that the *association* list itself (capacityProviders, as opposed to a capacityProviderStrategy item) references real capacity providers -- e.g. PutClusterCapacityProviders(capacityProviders=[\"typo-cp\"]) is accepted. FIXED a prior sweep for capacityProviderStrategy *items* specifically (see PutClusterCapacityProviders note); the separate capacityProviders association-list gap is unchanged and intentionally not fixed for the same reason (many call sites, tests using ad-hoc provider names in the association list specifically). CORRECTED this sweep: UpdateCluster removed from this list -- it never legitimately had a capacityProviders field to validate (see UpdateCluster entry), so listing it here as a gap was itself inaccurate."
  - "SDK bumped v1.86.2 -> v1.88.0 last sweep (no local services/ecs/ drift; SDK-only, re-confirmed unchanged this sweep). New surface: ServiceRevision.Overrides -> ServiceRevisionOverrides.RuntimePlatform (types.RuntimePlatformOverride, CpuArchitecture only) — an output-only field AWS populates when it auto-detects an architecture mismatch during an ECS Express deployment (doc: \"You can't set this value\"). Not modeled (DescribeServiceRevisions never populates Overrides); no client-visible regression since the field is optional/omitempty and no test or codepath claims architecture-mismatch detection. Niche, deferred."
  - "ContinueServiceDeployment always returns ClientException (no paused lifecycle hook) because PAUSE-stage lifecycle hooks for blue/green deployments are not modeled at all (no hookId tracking, no pause state in the ECS_SERVICE_DEPLOYMENT / EXTERNAL deployment controllers). Implementing real hook pausing is a substantial feature (Lambda-invocation simulation, TEST_TRAFFIC_SHIFT/BAKE_TIME lifecycle stages) out of scope for this sweep; the op is real (validates ARN/hookId, returns AWS-shaped errors) rather than a stub. Re-verified unchanged this sweep."
  - "ECS -> ELB/ELBv2 target registration is config-only: Service.LoadBalancers/ServiceRegistries are stored and echoed back on Describe/Update, but nothing calls services/elbv2 to register/deregister targets in a target group, and ELB health does not feed back into ECS task/service health. Cross-service, lives outside services/ecs/ — reported, not fixed. No bd issue found for this in the tracker at time of writing; recommend filing one scoped to services/elbv2 + services/ecs integration."
  - "ECS -> Auto Scaling Group capacity providers are config-only: AutoScalingGroupProvider (ARN, ManagedScaling, ManagedTerminationProtection, ManagedDraining) is stored/echoed but never calls services/autoscaling to validate the ASG exists or to actually scale it in response to managed-scaling target utilization. Cross-service, lives outside services/ecs/ — reported, not fixed."
  - "Value-semantics sweep (gopherstack-uox6): several documented filter fields are never declared on their handler's wire input struct at all (the other axis -- never-read, not a wrong algorithm, not fixed here): ListServiceDeploymentsInput.CreatedAt and .Status (listServiceDeploymentsInput has only Cluster/Service); ListDaemonDeploymentsInput.CreatedAt (ListDaemonDeploymentsInput backend type has only DaemonArn/Status); ListTasksInput.daemonName; ListServicesInput.resourceManagementType. Also validation-shaped, not fixed here: ListTasksInput.startedBy docs 'When you specify startedBy as the filter, it must be the only filter that you use' -- combining it with another filter is silently ANDed rather than rejected; ListAccountSettingsInput.value and ListAttributesInput.attributeValue both document 'You must also specify a name/attribute name to use this parameter' -- supplying value alone is not rejected."
  - "Value-semantics sweep (gopherstack-uox6): ListContainerInstancesInput.status docs 'If you don't specify this parameter, the default is to include container instances set to all states other than INACTIVE' -- the code has no such exclusion (empty status = no filter at all). Left unfixed: types.ContainerInstanceStatus's own enum (ACTIVE/DRAINING/REGISTERING/DEREGISTERING/REGISTRATION_FAILED) does not even include INACTIVE, and this backend deletes a container instance's row entirely on DeregisterContainerInstance rather than retaining it with Status=INACTIVE, so no container instance persisted in this backend's store can ever carry that status -- the documented default has zero observable effect here (same shape as the discarded-before-response gap recorded in a prior pass). Recorded, not implemented, since there is no reachable state to test it against."
  - "Container exit never moved a task to STOPPED (gopherstack-s1u9): nothing waited for a Docker container to exit -- ContainerWait appeared nowhere in the repo -- so a task whose container quit on its own (the common shape for a batch job invoked via Step Functions ecs:runTask.sync) stayed RUNNING forever; only an explicit StopTask call ever reached STOPPED. Fixed the same additive way gopherstack-jnct added ContainerLogs: dockerClient (docker_runner.go) now declares ContainerWait, matching moby/moby/client@v0.5.1 container_wait.go:41's (*Client).ContainerWait(ctx, containerID, ContainerWaitOptions) ContainerWaitResult, and internal/dockercompat/client.Client got the same method (WaitOptions/WaitResponse/WaitResult added to the compat container package) -- no existing signature touched. RunTask now starts a per-container watchContainerExit goroutine (only when a completion handler is wired, see below) that blocks on ContainerWait and, on exit, calls the handler with (taskArn, containerName, exitCode); StopTask and RunTask's own rollback path cancel that goroutine the same way they already cancel log forwarding, so a self-inflicted stop is never misreported as a natural exit and no goroutine is leaked. The handler is InMemoryBackend.markTaskStoppedByContainerExit (tasks.go), wired in provider.go via a small taskCompletionRunner interface (SetTaskCompletionHandler) mirroring the existing SetCWLogsBackend/SetELBv2Registrar injection pattern; it finalizes the task to STOPPED with real ECS's own stop reason for this case, 'Essential container in task exited', and records the exiting container's ExitCode (siblings keep the pre-existing nil-exitcode approximation). Approximation, documented: the essential-container distinction (ContainerDefinition.Essential) is not modeled here -- the FIRST container in a task to exit drives the task to STOPPED, and other still-running containers in a multi-container task are not force-stopped by this path. Real ECS task definitions used with Step Functions .sync are overwhelmingly single-container batch jobs, where this is exact; multi-container teardown is left for a future change. No new signal for an external consumer (e.g. a future Step Functions .sync poller, gopherstack-tdp6) was added on top of this: DescribeTasks.LastStatus reaching STOPPED reliably (this fix) already gives a poll-based consumer everything real AWS's own .sync waiting needs, matching how this repo's own SDK waiters already work; services/stepfunctions has zero .sync-style polling code today (grepped) and tdp6 itself carries no description of what shape a consumer would want, so adding a push/callback broadcast (cloudwatch's SubscribeAlarmStateChange-style per-key subscriber registry is this repo's only precedent for 'notify me when a resource I don't own changes state', but was built for FIS's genuinely interrupt-driven need to react to an alarm mid-experiment, not a terminal-state wait) would be exactly the speculative API-nobody-calls this sweep was warned against. Glue's equivalent half of the same issue needed no code change at all: GetJobRun already calls advanceStates before reading JobRunState, so SUCCEEDED/TIMEOUT are already a correct, poll-ready completion signal with no gap to fix."
  - "awslogs LogConfiguration (gopherstack-sv5q, gopherstack-jnct): Added ecs.CWLogsBackend (EnsureLogGroupAndStream/PutLogLines, same shape as lambda.CWLogsBackend/firehose.CWLogsBackend) plus SetCWLogsBackend, wired in cli.go's wireEcsCWLogs. RunTask/StartTask call EnsureLogGroupAndStream for every awslogs-driver ContainerDefinition, so awslogs-group/awslogs-stream-prefix become a real, discoverable CloudWatch Logs group/stream. gopherstack-jnct added the source side: docker_runner.go's dockerClient interface now declares ContainerLogs (added to internal/dockercompat's compat Client too, since that's the concrete type NewDockerRunner constructs), and RunTask, once a CWLogsBackend is wired, starts a per-container goroutine that follows ContainerLogs(Follow: true), demultiplexes with moby/moby/api/pkg/stdcopy.StdCopy, and forwards each complete line via PutLogLines as it arrives -- not batched at task/container exit. StopTask and RunTask's own rollback path cancel that goroutine's context once a container is confirmed stopped/removed, so it doesn't block on Follow against a dead container. Stream naming: with awslogs-stream-prefix set, follows the SDK's documented format verbatim (aws-sdk-go-v2 service/ecs@v1.90.0 types/types.go, LogConfiguration doc for awslogs-stream-prefix: 'the log stream takes the format prefix-name/container-name/ecs-task-id'); with no prefix, real ECS names the stream after the Docker-assigned container ID, which ensureAwslogsStreams computes before the container exists and so still cannot use -- approximated with the task ID alone (own choice, not SDK-pinned; unaffected by gopherstack-jnct since RunTask's own log-forwarding path recomputes the same group/stream via the now-shared awslogsTarget helper, not the real container ID either)."
deferred:
  - "Daemon* operation family (CreateDaemon..UpdateDaemon, 12 ops) — field-diffed for real; see families.daemon above for the full writeup (leak fixed; the wire-shape gap previously logged here was a documentation error, corrected gopherstack-rnka -- the nested revision shape was already correct)."
  - "docker_runner.go / real container lifecycle (vs NoopRunner) — re-audited this sweep. Reviewed RunTask (pull/create/start with rollback-on-failure via rollbackContainers, only registers the task's containers in the tracking map after every container in the task started successfully) and StopTask (snapshots container IDs under lock, stops/removes outside the lock, retains only failed-to-stop IDs for retry). No stubs, no goroutine or container-tracking-map leaks found: a task that fails mid-RunTask is fully rolled back before ever being added to r.containers, so there is no leaked entry for it to begin with. No changes needed. gopherstack-jnct added dockerClient.ContainerLogs and per-container log-forwarding goroutines (see gaps' awslogs entry); those goroutines are tracked the same way (cancelLogForwarding called from both rollbackContainers and StopTask, tied to each goroutine's own context), so this leak-freedom claim still holds after that addition."
  - "Full ServiceDeployment wire-shape parity (LifecycleStage, SourceServiceRevisions, Rollback, DeploymentCircuitBreaker, Alarms sub-objects) — the emulator's ServiceDeployment type covers ServiceDeploymentArn/ClusterArn/ServiceArn/Status/StatusReason/CreatedAt/UpdatedAt/TargetServiceRevisionArn (the last added gopherstack-7ux2, sourced from Deployment.ServiceRevisionArn and now surfaced by ListServiceDeployments' Brief). The richer blue/green fields remain unmodeled (same underlying reason ContinueServiceDeployment is deferred -- blue/green lifecycle is not modeled at all in this backend)."
leaks: {status: clean, note: "Prior 'found' status was stale documentation -- that leak (DeleteService's ServiceDeployment-map entry) was already fixed in the same prior sweep that wrote the note; the status field just never got flipped back to clean. Re-verified clean this sweep. Two NEW leaks found and fixed this sweep: (1) DeleteDaemon never cleaned up daemonRevisions/daemonDeployments rows, and purgeDaemonsLocked deleted from daemonRevisions by the wrong key so it silently matched nothing -- both fixed via deleteDaemonAncillaryLocked. (2) resourceTags side-map ghost rows were never cleaned up on delete for clusters/services/container-instances/task-sets/task-definitions/express-gateway-services -- fixed via deleteResourceTagsLocked. See Notes for full writeup and proof tests. Reconciler, janitor, lifecycle stepper, and docker_runner (re-audited this sweep) remain clean."}
---

## Notes

### 2026-08-22 reconciler snapshot race fix (gopherstack-urw6)

`getServicesForReconciler` (`services.go`) built each `serviceSnapshot` via a
shallow `service: *svc` copy. `Service.Deployments` is a slice, and a shallow
struct copy only copies the slice header — the copy's backing array was still
the live stored `Service`'s. `recordServiceTaskFailureLocked` and
`evaluateCircuitBreakerLocked` (`deployment.go`) write into that array in
place (`svc.Deployments[idx].FailedTasks++`, `dep.RolloutState = ...`) under
the write lock on a failed task launch, while the reconciler reads
`Deployments[idx].RolloutState` from the unlocked snapshot via
`reconcileService` → `primaryDeploymentFailed` — the lock is irrelevant once
the slice has escaped, since the reader never takes it. Fixed with
`cloneServiceForSnapshot`, which deep-copies `Deployments` into a fresh
backing array. `DescribeServices`'s `enrichService` was already doing this
correctly (rebuilds a fresh `[]Deployment`) — `getServicesForReconciler` was
the one getter that took the shortcut. Every other `Service` field read from
the reconciler's snapshot (`DeploymentConfiguration`, `DesiredCount`,
`Status`, `ServiceName`, `TaskDefinition`) is either a scalar (copied by
value, no aliasing) or a pointer field that this backend only ever replaces
wholesale (`UpdateService`'s `svc.DeploymentConfiguration = input.…`), never
mutates in place, so those did not need cloning. Reproduced with
`TestReconciler_ConcurrentFailureAttribution_NoDataRace`
(`reconciler_race_internal_test.go`), confirmed to fail under `-race` against
the unfixed code (5/5 runs) and clean after the fix; `rotationSchedulerLoop`
in `services/secretsmanager/rotation.go` was audited under the same bd issue
and found clean — every mutation there goes through `b.mu.Lock()` and every
reader holds the lock (R or W) for the full duration it touches secret/version
fields, with slices always reassigned to a fresh backing array rather than
mutated in place, so no live pointer/slice ever escapes the lock.

### 2026-08-13 wrong-shape fix (gopherstack-7ux2)

`ListServiceDeployments` (`handler_service_deployments.go`) returned a bare
`serviceDeploymentArns` string list. The real output is `ServiceDeployments`,
a list of `types.ServiceDeploymentBrief` (verified against
`aws-sdk-go-v2/service/ecs@v1.90.0`'s `api_op_ListServiceDeployments.go` and
`awsAwsjson11_deserializeDocumentServiceDeploymentBrief` in
`deserializers.go`) — a real client decoded nothing at all, not merely a
dropped field. Fixed with a `serviceDeploymentBriefView` DTO built from the
existing `ServiceDeployment` backend record: `ClusterArn`/`ServiceArn`/
`ServiceDeploymentArn`/`Status`/`StatusReason`/`CreatedAt` map directly;
`StartedAt` mirrors `CreatedAt` (the real full `ServiceDeployment` type has
no separate started timestamp either — only `CreatedAt`/`FinishedAt`);
`FinishedAt` is `UpdatedAt` when `Status` is terminal (`SUCCESSFUL`/
`STOPPED`), left absent otherwise (this backend has no dedicated
"reached-terminal-state" timestamp, and fabricating one for an
in-progress deployment would be worse than omitting it); `TargetServiceRevisionArn`
is newly threaded from `Deployment.ServiceRevisionArn` onto the
`ServiceDeployment` record in `recordServiceDeploymentLocked` — that data
already existed on `Deployment` but was never copied over, so an existing
test comment (`handler_agent_ops_test.go`) anticipating it was previously
unfulfilled. `Alarms`/`DeploymentCircuitBreaker`/`DeploymentConfiguration`
remain absent: not modeled anywhere in this backend, nothing honest to
source them from. Proven end-to-end against a real `aws-sdk-go-v2` client in
`test/integration/ecs_test.go`'s `TestIntegration_ECS_ListServiceDeployments`
(fails against the pre-fix shape with a real client, verified by hand-revert),
plus `TestECS_ListServiceDeployments_Shape` at the unit level.

### 2026-07-31 field-level wire-shape fixes (parity-5 branch)

Three field-level wire-shape mismatches, reported by a dashboard sweep after
TypeScript refused to compile the frontend against the real
`@aws-sdk/client-ecs` types, were re-verified one at a time against the real
SDK's `dist-types/models/models_0.d.ts` before touching any code. All three
were real (none were misreads):

1. **UpdateCluster** accepted `capacityProviders` and
   `defaultCapacityProviderStrategy`, neither of which exist on the real
   `UpdateClusterRequest` (only `cluster`/`settings`/`configuration`/
   `serviceConnectDefaults`). `PutClusterCapacityProviders` was already
   implemented and routed as the real op for capacity-provider association,
   so the fields were pure redundancy, not a missing-capability gap -- removed
   from `updateClusterInput`, `UpdateClusterInput`, and
   `Backend.UpdateCluster`. See UpdateCluster entry.
2. **UpdateCapacityProvider** accepted `status` and `tags` (neither real),
   and its `autoScalingGroupProvider` reused the full create-time type
   (carrying `autoScalingGroupArn`) instead of the real, narrower
   `AutoScalingGroupProviderUpdate`. Fixing the wire shape surfaced a real
   state bug it had been masking: the backend replaced
   `cp.AutoScalingGroupProvider` wholesale on any update, so an update that
   only meant to change `managedScaling` silently zeroed the stored ASG ARN.
   Fixed both together: new `AutoScalingGroupProviderUpdate` type (no ARN)
   end-to-end, and the backend now merges the three real update fields onto
   the existing `AutoScalingGroupProvider` instead of replacing it. See
   UpdateCapacityProvider entry.
3. **RegisterContainerInstance** *required* `ec2InstanceId`, a field with no
   equivalent on the real `RegisterContainerInstanceRequest` -- a real typed
   client can never populate it, only `instanceIdentityDocument` (an EC2
   instance-identity-document JSON blob, of which `instanceId` is one field)
   and `instanceIdentityDocumentSignature`. Fixed by accepting
   `instanceIdentityDocument` and parsing `instanceId` out of it, matching
   how real ECS derives instance identity. An absent or unparseable document
   now yields an empty `EC2InstanceID` rather than a fabricated one -- this
   was a deliberate choice per the instruction not to invent a
   plausible-looking ID and present it as real. See RegisterContainerInstance
   entry.

`sdk_completeness_test.go` (77/77, compares op *names* to
`GetSupportedOperations()`) could not have caught any of these -- it is blind
to field shapes by construction. Widening the search for the same defect
pattern (a test asserting the very field a real client cannot send) found
four more tests doing exactly this: `TestUpdateCluster_CapacityProviders`
(handler_clusters_test.go), the `UpdateCluster` subtest of
`TestCapacityProviderStrategy_RejectsUnknownProvider`
(capacity_provider_strategy_validation_test.go),
`TestCapacityProvider_Update_ManagedScaling` and the `status`-sending case in
`TestECS_UpdateCapacityProvider` (handler_capacity_providers_test.go), plus
every `RegisterContainerInstance` call across handler_container_instances_test.go,
handler_attributes_test.go, handler_test.go, and handler_task_exec_test.go
that sent `ec2InstanceId`. All corrected to the real wire shape; each fix has
a test that demonstrably fails against the pre-fix code (verified by
temporarily reverting the six touched source files while keeping the
corrected tests and re-running them; restored after confirming the failures,
per the plan of record).

2026-08-23 re-verification (batch7): the above finding was STALE by the time
this sweep read it. `handleDescribeCapacityProviders`
(handler_capacity_providers.go:224-231) zeroes `v.Tags` and, when
`include=[TAGS]` is set, populates it via `h.Backend.ListTagsForResource`,
which reads `b.resourceTags` (tags.go:203-219) -- the same live side map
`TagResource`/`UntagResource` write to, not `CapacityProvider.Tags`'s
creation-time snapshot. `capacityProviderWithLiveTagsLocked`
(capacity_providers.go:155-162, added #2417) independently sources the same
side map for the no-filter Describe path. No client-visible bug here;
removed from `gaps`.

`overall` stays `A`: all three findings were genuine wire-shape bugs, and all
three are now fixed with proof tests, not just documented as open gaps.

### 2026-07-26 re-verification (parity-5 branch, bd issue gopherstack-rnka)

bd tracked gopherstack-rnka as still OPEN on this branch, describing the same
three gaps as the 2026-07-25 entry immediately below. Investigation before
touching any code found the fix was already present: it was committed as
`f119bb41c` on the (separate, now-stale) `parity-4` local/remote branch on
2026-07-25, squash-merged into `main` as `8b2553cf7` ("parity-4: 147 new SDK
operations..."), and `parity-5` branched from `main` after that merge landed
-- so the fix has been on this branch since its first commit. `git status`/
`git diff` show zero uncommitted drift in `services/ecs/` at the start of
this session (the file adjacent to this one, `services/cloudwatch/
rpcv2cbor.go`, and `.beads/issues.jsonl` were mid-edit from an unrelated
concurrent agent/process and were left untouched).

Rather than trust the prior note, re-verified field-by-field against the
actually-vendored SDK (`go.mod` pins v1.89.0, one patch ahead of the
v1.88.0 this ledger previously cited -- corrected above; `diff`'d
`ECSExpressGatewayService`/`ExpressGatewayServiceConfiguration`/
`DaemonDetail`/`DaemonRevisionDetail`/`DaemonCapacityProvider` between the
v1.88.0 and v1.89.0 module caches and found them byte-identical, so the
version drift caused no actual staleness):

- `ExpressGatewayService`/`ExpressGatewayServiceConfiguration` in
  `services/ecs/models.go` carry exactly the real field set (no more, no
  less) -- `ActiveConfigurations`, `Cluster`, `CreatedAt`, `CurrentDeployment`,
  `InfrastructureRoleArn`, `ServiceArn`, `ServiceName`, `Status` (nested
  `{statusCode,statusReason}` via `expressGatewayServiceStatusView`), `Tags`,
  `UpdatedAt` at the top level, and `Cpu`, `CreatedAt`, `ExecutionRoleArn`,
  `HealthCheckPath`, `IngressPaths`, `Memory`, `NetworkConfiguration`,
  `PrimaryContainer`, `ScalingTarget`, `ServiceRevisionArn`,
  `TaskDefinitionArn`, `TaskRoleArn` per revision. No fabricated fields.
- `daemonDetailView`/`daemonRevisionDetailView`/`daemonCapacityProviderView`
  (`handler_daemon.go`) match `DaemonDetail{ClusterArn, CreatedAt,
  CurrentRevisions[]DaemonRevisionDetail{Arn, CapacityProviders[]
  DaemonCapacityProvider{Arn, RunningCount}, TotalRunningCount}, DaemonArn,
  DeploymentArn, Status, UpdatedAt}` exactly -- confirmed still
  revision-nested, not flattened.
- `Service.Tags` resourceTags sync and `DescribeServices` `Include=[TAGS]`
  gating: read `services/ecs/services.go` and `handler_services.go` directly
  (not just the note) and confirmed `resourceTags[resourceTagKey(...)]` is
  the source of truth read on Create/Update/Delete/Describe, and
  `describeServiceIncludeTags`/`wantsIncludeTag`/`attachTagsIfWanted`
  (`tags.go`) gate `DescribeServices` the same way as
  `DescribeContainerInstances`.

Ran the full gate suite fresh this session (`go build ./...`, `go vet ./...`,
`go vet -tags e2e ./test/e2e/...`, `go test -race -count=1
./services/ecs/...`, `go test -race -count=1 ./services/cloudformation/...`,
`gofmt -l services/ecs/`, `golangci-lint run ./services/ecs/...`) -- all
clean, 0 issues, 0 banned nolints. `TestService_Tags_ResourceTagSync`
(`handler_services_test.go`) and
`TestECS_DescribeDaemon_SDKRoundTrip_RevisionNesting`
(`handler_daemon_test.go`) both drive the real `aws-sdk-go-v2` client and
prove the Include=[TAGS] gate both ways (tags absent without it, present
with it) and the revision-nested `DaemonDetail` shape. No code changes were
needed; `overall: A` re-confirmed, not just carried forward. bd issue closed
as already-fixed rather than left open against stale ledger content.

### 2026-07-25 follow-up (parity-4 branch, bd issue gopherstack-rnka)

Scope: closed the three gaps the 2026-07-23 sweep explicitly left open (see
that section below), plus corrected one stale gap that turned out to already
be fixed. `git diff` at the start of this pass showed only unrelated
concurrent-agent changes outside `services/ecs/` (medialive, ui/); zero prior
drift in this package.

**Fixed: `Service.Tags` resourceTags sync + `DescribeServices`
`include=[TAGS]` gating.** Applied the identical fix already proven for
`ExpressGatewayService` (see the 2026-07-23 section below). `CreateService`
now mirrors creation-time tags into the `resourceTags` side map
(`setResourceTagsLocked`); `CreateService`/`UpdateService`/`DeleteService` all
now return the `resourceTags`-authoritative tag set (not the stale
create-time struct-field snapshot) since none of the three real AWS ops gate
tags behind `Include`. `DescribeServices` gained real `include=[TAGS]`
gating (`describeServiceIncludeTags`, matching `types.ServiceField` ==
`"TAGS"`) — tags are now omitted unless requested, sourced from
`ListTagsForResource` when they are. `StartTaskForService`'s
`propagateTags=SERVICE` path (used by the reconciler when scaling a service)
was also reading the stale `svc.Tags` field directly; switched to read
`resourceTags` so a `TagResource` call made after `CreateService` is honored
by newly-scheduled tasks, not just tags supplied at creation. To avoid the
`dupl` duplication this created against the pre-existing, structurally
identical `DescribeContainerInstances` include-gating loop, extracted two
shared generics helpers into `tags.go`: `wantsIncludeTag` and
`attachTagsIfWanted`, and switched `DescribeContainerInstances` to use them
too (behavior-preserving refactor, not a functional change to that op).
Proven by `TestService_Tags_ResourceTagSync` (`handler_services_test.go`,
4 cases: create-time sync visible to `ListTagsForResource` immediately,
`TagResource`-after-create visible on Describe only with `Include=[TAGS]`
[real SDK client round trip], `propagateTags=SERVICE` tasks inherit tags
added after creation [drives `StartTaskForService` directly + real SDK
client `DescribeTasks` to confirm the wire shape], delete echoes final tags
and clears the `resourceTags` ghost row).

**Fixed: `ExpressGatewayService` missing real SDK fields.** Field-diffed
`types.ECSExpressGatewayService`/`CreateExpressGatewayServiceInput`/
`UpdateExpressGatewayServiceInput`/`UpdatedExpressGatewayService`/
`ExpressGatewayServiceConfiguration` (and every type nested under them —
`ExpressGatewayContainer`, `ExpressGatewayServiceNetworkConfiguration`,
`ExpressGatewayScalingTarget`, `ExpressGatewayServiceStatus`,
`IngressPathSummary`, `ExpressGatewayServiceAwsLogsConfiguration`,
`ExpressGatewayRepositoryCredentials`) against the installed
`aws-sdk-go-v2/service/ecs@v1.88.0` module's `types`/serializers/
deserializers directly, not against this backend's own handler output. Two
findings beyond the originally-listed missing fields: (1) the real
`ECSExpressGatewayService`/`UpdatedExpressGatewayService` types have **no
top-level `ExecutionRoleArn`/`Cpu`/`Memory`/etc.** — those fields exist only
nested inside each `ActiveConfigurations`/`TargetConfiguration` service
revision; this backend's pre-existing top-level `ExpressGatewayService.
ExecutionRoleArn` field was itself invented and has been removed. (2) the
real `Status` field is a nested `{statusCode, statusReason}` object
(`types.ExpressGatewayServiceStatus`), not a bare string — the pre-existing
flat `status: "ACTIVE"` string was also wrong; fixed to nest. Implemented
end to end: `Create`/`UpdateExpressGatewayServiceInput` now accept the full
real field set (`Cpu`, `Memory`, `HealthCheckPath`, `NetworkConfiguration`,
`PrimaryContainer`, `ScalingTarget`, `TaskDefinitionArn`, `TaskRoleArn`, in
addition to the pre-existing `ExecutionRoleArn`/`InfrastructureRoleArn`),
validated for the real API's mutual-exclusivity rule ("if you provide a
task definition ARN, you cannot also specify primaryContainer,
executionRoleArn, taskRoleArn, cpu, or memory" — a 400
`InvalidParameterException`), applied with the documented AWS defaults
(cpu 256 / memory 512 / healthCheckPath `/ping`) when the task-definition
path isn't in use, and stored as an `ExpressGatewayServiceConfiguration`
service revision. `Create`/`Update` each mint a fresh
`ServiceRevisionArn` (`arn:...:service-revision/cluster/service/id`,
mirroring the existing `serviceRevisionArnFor` pattern for ordinary
Services) and become the service's sole `ActiveConfigurations` entry and
`CurrentDeployment` (this backend does not model AWS's multi-revision
blue/green rollout for Express services — one active revision at a time is
a documented simplification, not a stub, matching the equivalent
simplification already accepted for `Daemon` and `ContinueServiceDeployment`
elsewhere in this ledger). `UpdateExpressGatewayServiceOutput.Service` is
correctly the narrower `UpdatedExpressGatewayService` shape (with
`TargetConfiguration`, not `ActiveConfigurations`/`Tags`/
`InfrastructureRoleArn` — those genuinely aren't on that response type),
distinct from `Create`/`Describe`/`Delete`'s `ECSExpressGatewayService`.
Proven end-to-end via a real `aws-sdk-go-v2` client round trip (not
`map[string]any` assertions) in
`TestExpressGatewayService_RevisionConfiguration_SDKRoundTrip`
(`handler_express_gateway_test.go`, 3 cases: full field round trip through
Create+Describe, Update rolls out a genuinely new revision, taskDefinitionArn
mutual-exclusivity rejected); pre-existing express-gateway tests updated for
the corrected `status` nesting and the `Update` response's narrower shape.

**Corrected: `families.daemon` `DescribeDaemonOutput` wire-shape gap was
stale.** The 2026-07-23 entry claimed `DaemonDetail` was flattened
(daemon-level fields instead of nested under `CurrentRevisions`). Re-checked
`handler_daemon.go`'s `daemonDetailView`/`daemonRevisionDetailView`/
`daemonCapacityProviderView` field-by-field against
`types.DaemonDetail`/`DaemonRevisionDetail`/`DaemonCapacityProvider`: they
already matched exactly (`ClusterArn`/`CreatedAt`/`CurrentRevisions[]`/
`DaemonArn`/`DeploymentArn`/`Status`/`UpdatedAt` at the top level, with
`Arn`/`CapacityProviders[]`/`TotalRunningCount` correctly nested per
revision, and `Arn`/`RunningCount` correctly nested per capacity provider).
No source change was needed. Rather than take the ledger's word for it,
added `TestECS_DescribeDaemon_SDKRoundTrip_RevisionNesting`
(`handler_daemon_test.go`, 2 cases) to prove this through a real SDK client
round trip, then corrected `families.daemon` from `partial` back to `ok` and
removed the stale gap language from `deferred`.

**Grade assessment:** `overall: A` remains honest — arguably more so now,
since it no longer carries three explicitly-acknowledged-but-unfixed gaps in
core, heavily-used surface area (`Service` tags, `ExpressGatewayService`
config fields) plus one materially inaccurate `partial` rating (`daemon`).

### 2026-07-23 re-audit (badges-automation branch, commit fd9a0877)

Scope: worked every item in the prior sweep's `gaps`/`deferred` lists, per
the parity-3 campaign brief for this service. `git diff 95dfa093..HEAD --
services/ecs/` showed zero local drift before this sweep's own changes
(consistent with the prior sweep's finding that the ecs SDK bump was the
only prior-prior change). No `//nolint:cyclop|gocyclo|gocognit|funlen` existed
before or after this sweep.

**Fixed: capacity provider strategy validation** (prior gap #1). Added
`validateCapacityProviderStrategyLocked` (capacity_providers.go): rejects any
`capacityProviderStrategy` item referencing a name that is neither a created
`CreateCapacityProvider` provider nor the FARGATE/FARGATE_SPOT builtins, with
a 400 `ClientException`. Wired into `CreateCluster`, `UpdateCluster`,
`PutClusterCapacityProviders`, `CreateService`, `UpdateService`, `RunTask`,
and `CreateTaskSet`. Field-diffing this gap surfaced two real, previously
undocumented wire-shape holes: `RunTaskInput` had **no**
`capacityProviderStrategy` field at all (real `ecs.RunTaskInput` has one; a
client could never actually set one via RunTask, so the "does not validate"
framing understated the prior gap -- the field didn't exist to validate), and
likewise for `CreateTaskSetInput`/`TaskSet` (real `ecs.CreateTaskSetInput`
and `types.TaskSet` both have `CapacityProviderStrategy`). Added the field to
both, plus `Task.CapacityProviderName` (the real SDK's per-task resolved-
provider output field -- this backend does not model AWS's weight/base
distribution algorithm across multiple strategy providers, so it always
selects the first entry; documented as a simplification, not a stub).
Scoped deliberately narrow: only strategy *items* are validated, not the
separate `capacityProviders` association list (unchanged gap, still listed).
Proven by `capacity_provider_strategy_validation_test.go` (new file:
`TestCapacityProviderStrategy_RejectsUnknownProvider` table-tests all seven
call sites, `TestCapacityProviderStrategy_AcceptsCreatedProvider`,
`TestRunTask_CapacityProviderStrategy_SetsCapacityProviderName`) plus new
cases in `handler_task_sets_test.go`
(`TestCreateTaskSet_CapacityProviderStrategy_Roundtrip`).

**Fixed: `include=[TAGS]` gating** (prior gap #2) for `DescribeCapacityProviders`,
`DescribeContainerInstances`, and `DescribeTaskSets`. `ContainerInstance` and
`TaskSet` had no `tags` field in their wire shape at all (real
`types.ContainerInstance`/`types.TaskSet` both have `Tags []Tag`); added the
field to both view types, gated by `Include`, sourced from the existing
`resourceTags` side map via `ListTagsForResource` (same pattern as
`DescribeClusters`). `CapacityProvider`'s `tags` field already existed but
was unconditionally populated regardless of `Include` -- now gated too.
While auditing this gap's four named ops, also found and fixed the same
"always-on tags" bug plus a deeper, previously-undocumented one for
`DescribeExpressGatewayService` (see the `ExpressGatewayService` writeup
below) -- `DescribeExpressGatewayService` was not in the original gap list at
all, but has the exact same real SDK `Include` parameter and the same bug.
Proven by `TestDescribeCapacityProviders_TagsRequireInclude`,
`TestDescribeContainerInstances_TagsRequireInclude`,
`TestDescribeTaskSets_TagsRequireInclude`,
`TestExpressGatewayService_TagResource_VisibleOnDescribe`.

**Fixed: `DescribeCapacityProviders` `Cluster` filter parameter** (prior gap
#3). When `cluster` is set, only capacity providers associated with that
cluster (via `CreateCluster`/`UpdateCluster`/`PutClusterCapacityProviders`)
are returned; an unknown cluster yields an empty result (AWS
filter-parameter semantics), not a 404. Proven by
`TestDescribeCapacityProviders_ClusterFilter` and
`TestDescribeCapacityProviders_ClusterFilter_UnknownCluster`.

**Re-verified unchanged, still accurate** (prior gaps #4-#7): the
`ServiceRevisionOverrides.RuntimePlatform` SDK-bump gap, `ContinueServiceDeployment`'s
honest-ClientException lifecycle-hook gap, and the two cross-service ECS->ELB
/ ECS->ASG config-only gaps. No changes needed; descriptions carried forward
verbatim except for a "re-verified" note.

**Real bug found and FIXED: `ExpressGatewayService` tags were a disguised
stub for `TagResource`.** `ExpressGatewayService.Tags` was populated at
creation and echoed on every Create/Describe/Update call, but was a
completely separate, never-synchronized copy from the `resourceTags` side
map that `TagResource`/`UntagResource`/`ListTagsForResource` actually read
and write -- so `TagResource(expressServiceArn, ...)` "succeeded" (200 OK)
but was invisible on every subsequent read path, and creation-time tags were
invisible to `ListTagsForResource`. Fixed by mirroring `CreateExpressGatewayService`'s
input tags into `resourceTags` (`setResourceTagsLocked`) and making
`DescribeExpressGatewayService`/`UpdateExpressGatewayService` read tags from
`resourceTags` (the now-authoritative source, kept in sync by TagResource)
instead of the stale struct-field snapshot. `DescribeExpressGatewayService`
also gained `Include=[TAGS]` gating in the same pass (see gap #2 above).
Proven end-to-end by `TestExpressGatewayService_TagResource_VisibleOnDescribe`;
did not regress the pre-existing `TestExpressGatewayService_DeepCopy_Tags`
backend-level test (which asserts the returned `Create` snapshot is an
independent deep copy -- still true, since `resourceTags` is seeded
independently at creation time too).

**Real gap found, NOT fixed: identical tags-disconnect bug exists for
`Service`.** Same pattern as the `ExpressGatewayService` bug just fixed:
`Service.Tags` is a creation-time snapshot, never synced with `resourceTags`.
Not fixed this sweep -- `Service` is the most heavily used and tested
resource in this package (`enrichService` and `RunTask`'s
`propagateTags=SERVICE` tag-resolution path both read `svc.Tags` directly),
so switching the source of truth carries materially higher regression risk
than the narrower `ExpressGatewayService` fix. Filed as a new gap; a future
sweep should budget dedicated time for it plus `DescribeServices`
`Include=[TAGS]` gating (also entirely absent -- a second, related gap found
in the same investigation).

**Real gap found via field-diff, NOT fixed: `ExpressGatewayService`
create/update/describe wire shape is substantially incomplete.** Real
`ecs.CreateExpressGatewayServiceInput`/`UpdateExpressGatewayServiceInput`
carry `Cpu`, `Memory`, `HealthCheckPath`, `NetworkConfiguration`,
`PrimaryContainer`, `ScalingTarget`, `TaskDefinitionArn`, `TaskRoleArn`; real
`types.ECSExpressGatewayService` (the Describe/Update output) also carries
`ActiveConfigurations`, `CurrentDeployment`, `UpdatedAt`. This backend models
none of these -- only `ExecutionRoleArn`/`InfrastructureRoleArn`/`Cluster`/
`ServiceName`/`Tags`. The prior ledger marked all four Express Gateway ops
fully `ok`; that was a no-stub assessment, not a field-diff (Express Gateway
is a newer, still-evolving ECS feature area and had apparently never been
diffed against the real SDK types before this sweep). Downgraded
`CreateExpressGatewayService`/`UpdateExpressGatewayService`/`DescribeExpressGatewayService`
from `wire: ok` to `wire: partial`. Not fixed: `ActiveConfigurations`/
`CurrentDeployment` imply a deployment/config-revision tracking model this
backend has nowhere else for Express services (config-only, same shape as
the pre-existing ECS->ELB/ECS->ASG cross-service gaps); the remaining scalar
and nested-type fields are a smaller, tractable follow-up but were not
attempted given the size of everything else in scope this sweep.

**Fixed: two real leaks in the Daemon family**, found while field-diffing it
(see `families.daemon` above for the wire-shape gap that remains open).
(1) `DeleteDaemon` never cleaned up `daemonRevisions`/`daemonDeployments`
rows at all -- only the `daemons` table entry itself was removed, so every
revision ever created via `CreateDaemon`/`UpdateDaemon` and every deployment
ever made leaked permanently once its owning daemon was deleted. (2)
`purgeDaemonsLocked`'s cleanup called `b.daemonRevisions.Delete(d.DaemonArn)`,
but `daemonRevisions` is keyed by `DaemonRevisionArn`, not `DaemonArn` --
this delete could never match anything. This was **already known and
explicitly documented as unfixed** in a code comment ("Preserved
byte-for-byte from the pre-conversion map-based code rather than fixed, per
the Phase 3.3 mechanical-conversion mandate") -- that mandate governed a
prior mechanical refactor PR, not this sweep, which is explicitly chartered
to find and fix leaks. Both fixed via a new shared
`deleteDaemonAncillaryLocked` helper (daemon.go), called from both
`DeleteDaemon` and `purgeDaemonsLocked`. Proven by
`TestDeleteDaemon_CleansRevisionsAndDeployments` and
`TestPurgeCluster_CleansDaemonRevisionsAndDeployments` (both fail without the
fix).

**Fixed: `resourceTags` ghost rows on delete**, a leak class explicitly
called out in this sweep's brief ("no ghost map rows after delete... tags").
`resourceTags` (the side map backing `TagResource`/`UntagResource`/
`ListTagsForResource`) was never cleaned up on delete for *any* resource
type: clusters, services, container instances, task sets, task definitions,
or express gateway services. For deterministic-ARN resources (cluster/
service/container-instance/express-gateway-service ARNs are all derived from
name, not a random ID) this meant a delete-then-recreate cycle with the same
name could resurrect stale tags from a previous incarnation of the resource;
for random-ID resources (task sets) it meant one permanently-leaked map row
per resource ever created and deleted. Fixed via a new shared
`deleteResourceTagsLocked` helper (tags.go), wired into `DeleteCluster`
(direct + cascaded services + cascaded container instances),
`purgeClusterLocked`'s equivalents, `DeleteService`,
`deleteTaskSetsForServiceLocked` (shared by `DeleteService`/`DeleteCluster`/
`purgeClusterLocked`), `DeleteTaskSet`, `DeregisterContainerInstance`,
`DeleteExpressGatewayService`, and `DeleteTaskDefinitions`. (Capacity
providers were checked and excluded: they store tags inline on the
`CapacityProvider` struct, not via `resourceTags`, so they die naturally with
the struct on delete -- no fix needed there. `DeregisterTaskDefinition` and
`DeleteDaemonTaskDefinition` were also checked and excluded: both only flip a
status flag to INACTIVE/DELETED, they never actually remove the record, so
there is nothing to clean up yet.) Proven by
`TestDeleteResource_CleansGhostResourceTags` (cluster + service subtests;
the same `deleteResourceTagsLocked` call sites cover container instances,
task sets, and express gateway services by construction, and are exercised
indirectly by the existing `TestECS_Delete*`/`TestPurge_*` suites, which all
still pass).

**`daemon` family: field-diffed for real this sweep** (prior ledger entries
were no-stub-only). See `families.daemon` above for the full writeup: leak
fixed (above), wire-shape gap found and remains open (downgraded from `ok`
to `partial`, NOT reclassified to `ok`).

**`docker_runner.go`: re-audited, clean, no changes.** See `deferred` above.

### 2026-07-11 re-audit (parity-4 branch, commit 95dfa093)

Re-audit protocol: `git diff ce30166a..HEAD -- services/ecs/` showed **zero
local drift** (the ledger's stated `last_audit_commit` 86c2f9af was not an
ancestor of HEAD — it's a cloudformation commit on an unrelated,
unmerged-at-audit-time branch `parity-sweep-3`; fell back to `ce30166a`, the
commit that actually authored this ledger, as baseline per protocol). The
only change in scope was an SDK bump, `aws-sdk-go-v2/service/ecs`
v1.86.2 -> v1.88.0 (no new/removed operations, no new enums/errors, only
doc-comment rewording plus one output-only field — see gaps list). Audit
therefore focused on the three previously-flagged `partial` rows.

**Fixed: `DescribeCapacityProviders` returned a whole-request 400 error
when *any* requested name/ARN was unknown**, instead of AWS's documented
partial-success behavior (`DescribeCapacityProvidersOutput.Failures
[]types.Failure`). This is the same `Arn`/`Reason: MISSING`/`Detail` pattern
already used correctly by `DescribeClusters`, `DescribeContainerInstances`,
`DescribeTasks`, `DescribeServices`, and `DeleteTaskDefinitions` in this same
package — `DescribeCapacityProviders` was the outlier, and had a test
(`TestECS_DescribeCapacityProviders` "unknown capacity provider returns
400") and a second test (`TestBatch3_CapacityProvider_Unknown_ReturnsError`)
that encoded the wrong behavior as the expected contract. A real client
calling `DescribeCapacityProviders(["my-cp", "typo-cp"])` to bulk-check
several providers would get a total failure instead of `my-cp`'s data plus
a `Failures` entry for `typo-cp` — this is a real behavioral divergence, not
just a missing optional field.

Fixed by changing `InMemoryBackend.DescribeCapacityProviders` (and the
`Backend` interface) from `([]CapacityProvider, error)` to
`([]CapacityProvider, []Failure, error)`, building a `Failure{Reason:
"MISSING"}` entry per unresolved name/ARN (after the existing
FARGATE/FARGATE_SPOT builtin fallback) instead of returning early with
`ErrCapacityProviderNotFound`. `handleDescribeCapacityProviders` now emits
`failures` in the JSON body (was entirely absent from the wire shape before).
Rewrote the two tests that asserted the old (wrong) all-or-nothing behavior
to assert the correct 200+Failures behavior, added a
"mix of known and unknown returns partial success" case. `ErrCapacityProviderNotFound`
is unchanged and still used correctly by `DeleteCapacityProvider` (single-resource
delete, where AWS *does* 400 on not-found) and `UpdateCapacityProvider`.

Files touched: `backend_iface.go`, `backend_new_ops.go`, `handler_new_ops.go`,
`handler_new_ops_test.go`, `handler_batch3_test.go`, `handler_refinement1_test.go`
(2-value call-site updates), `persistence_internal_test.go` (same). ~66 LOC.

Re-verified `PutClusterCapacityProviders` (still no existence validation of
referenced capacity-provider names — deliberate prior-sweep decision, see
gaps list, not re-litigated: fixing it touches `CreateService`/
`UpdateService`/`RunTask` too and risks breaking ad-hoc-named strategies used
throughout the test suite; out of scope for a targeted bug-fix pass) and
`ContinueServiceDeployment` (still an honest, real, ARN/hookId-validating
`ClientException` — no regression, ledger description remains accurate) —
both confirmed unchanged from the prior sweep's assessment.

### Prior sweep (gopherstack-7wu)

### Severe, fixed this sweep

1. **`Restore()` never rebuilt `serviceIndex`.** `getServicesForReconciler`
   (backend.go) is the *only* feed for the deployment reconciler
   (reconciler.go) and reads the flat `serviceIndex` map with **no linear-scan
   fallback** (unlike `tasksByInstance`, which `enrichContainerInstance`
   explicitly falls back to scanning for — see the comment there). `Restore`
   loaded `b.services` from the snapshot but never repopulated
   `b.serviceIndex`, so every service that existed at snapshot time became
   permanently invisible to the reconciler after a restore/restart: desired-count
   convergence, scale up/down, and circuit-breaker evaluation would silently
   stop forever for pre-existing services. Fixed in `persistence.go` `Restore`
   by rebuilding `serviceIndex` (and, for consistency/performance,
   `tasksByInstance`) from the restored maps. Proven by
   `Test_Restore_RebuildsServiceIndex` (persistence_internal_test.go), which
   fails without the fix (reconciler `RunOnce` after restore launches zero
   tasks for a service with `DesiredCount > 0`).

2. **`resourceTags` side map was entirely absent from `backendSnapshot`.**
   Clusters and Services carry `Tags` inline on their own struct, but task
   definitions and daemon task definitions are tagged only through the
   `TagResource`/`UntagResource`/`ListTagsForResource` side map
   (`b.resourceTags`, keyed by `resourceTagKey(arn)`). That map was never
   included in `Snapshot()`/`Restore()`, so every tag applied via
   `TagResource` on a task definition silently vanished across a
   snapshot/restore cycle. Fixed by adding `ResourceTags` to
   `backendSnapshot` with proper deep-copy on snapshot and restore. Proven by
   `Test_Snapshot_Restore_PreservesResourceTags`.

3. **`DescribeServiceDeployments`/`ListServiceDeployments`/
   `StopServiceDeployment` were disguised stubs** (parity-principles.md rule
   4: "a real-looking op filtering a never-populated map is a disguised
   stub"). `b.serviceDeployments` was only ever written by the
   `AddServiceDeploymentInternal` test-seed helper — no real `CreateService`,
   `UpdateService`, or circuit-breaker rollback path ever created an entry.
   A real client following the documented `CreateService` ->
   `ListServiceDeployments` -> `DescribeServiceDeployments` workflow always
   got an empty result, even though the service had an active PRIMARY
   deployment tracked in `Service.Deployments`. Fixed by
   `syncServiceDeploymentsLocked`/`recordServiceDeploymentLocked`
   (backend_new_ops.go), called from `CreateService`, `UpdateService`, and
   `evaluateCircuitBreakerLocked` (deployment.go, covers both the rollback and
   halt-without-rollback branches). `ServiceDeploymentArn` is derived
   deterministically from the service ARN + `Deployment.ID`
   (`serviceDeploymentArnFor`, mirroring the existing `serviceRevisionArnFor`
   pattern for `arn:...:service-revision/...`). Proven end-to-end via HTTP in
   `TestECS_ServiceDeployments_RealDeploymentsAreVisible`.

   This uncovered a **second, previously-latent bug**: the cascade-delete
   code in `DeleteCluster` and `purgeClusterLocked` did
   `delete(b.serviceDeployments, svc.ServiceArn)` — deleting by
   `ServiceArn` used as a *map key*, but the map is keyed by
   `ServiceDeploymentArn`. This was silently a no-op before (the map was
   always empty in practice), but once real entries started flowing in it
   would have **leaked one entry per deleted service forever** (also true of
   plain `DeleteService`, which never even attempted cleanup). Fixed with a
   shared `deleteServiceDeploymentsForServiceLocked(serviceArn)` helper
   (matches by the `.ServiceArn` *field*) wired into `DeleteCluster`,
   `DeleteService`, and `purgeClusterLocked`. `TestDeleteCluster_
   CascadesServiceDeployments` encoded the old (wrong) key convention — it
   injected a fake entry keyed by `svc.ServiceArn` with the `.ServiceArn`
   field left blank, which only ever passed because of the bug. Rewritten to
   assert against the real auto-created deployment (via `ListServiceDeployments`)
   plus a correctly-keyed injected extra entry. New:
   `TestECS_ServiceDeployments_DeletedOnServiceDelete`.

### Moderate, fixed this sweep

4. **`CreateCluster` silently dropped `capacityProviders`,
   `defaultCapacityProviderStrategy`, and `tags`** — all three are real
   `CreateClusterInput` fields in aws-sdk-go-v2. Terraform's `aws_ecs_cluster`
   resource sets `tags` at creation time (no fallback `TagResource` call), so
   this was a real, silent tag-loss bug for the most common IaC flow, not just
   a theoretical gap. Fixed: `CreateClusterInput`/`createClusterInput` accept
   all three; `CreateCluster` stores capacity-provider fields directly and
   tags via a new `setResourceTagsLocked` helper (extracted from `TagResource`
   so it can be called while the write lock is already held, avoiding a
   self-deadlock on `lockmetrics.RWMutex`). `DescribeClusters` gained
   `include=["TAGS"]` support (previously unsupported — tags were never
   returned by Describe regardless of the wire shape technically supporting
   `tags,omitempty`). CreateCluster's own response always echoes back the
   tags it was just given (matches: no `include` gating exists on Create).
   Proven by `TestECS_CreateCluster_TagsAndCapacityProviders`.

5. **`ContinueServiceDeployment` was entirely unimplemented** — absent from
   `GetSupportedOperations()`/`buildOps()` and carried an explicit
   acknowledged gap in `sdk_completeness_test.go`. Since this backend does not
   model blue/green lifecycle-hook pause stages at all, a full implementation
   (Lambda-invocation simulation, `TEST_TRAFFIC_SHIFT`/`BAKE_TIME` stages) was
   out of scope; instead the op is now real-but-honest: validates
   `serviceDeploymentArn` and `hookId` are present, looks up the deployment
   (404 `ServiceDeploymentNotFoundException` if missing), validates `action`
   is `CONTINUE`/`ROLLBACK`/omitted, and returns `ClientException` reporting
   that no such lifecycle hook is currently paused — never a fabricated
   success. Removed from the `sdk_completeness_test.go` acknowledged-gap list
   since it is now routed. Proven by `TestECS_ContinueServiceDeployment`
   (three cases: real deployment/no hook, deployment not found, missing
   hookId).

### Verified accurate / traps for the next auditor

- `enrichContainerInstance`'s `tasksByInstance` fallback-to-linear-scan
  comment ("e.g. after restore") shows a prior sweep already reasoned about
  the post-restore-index-empty case for that specific map — but the parallel
  `serviceIndex` consumer (`getServicesForReconciler`) had no such fallback
  and no comment acknowledging it. Don't assume one documented index-rebuild
  concern means all of them were considered.
- `enrichService`'s `RolloutState -> COMPLETED` transition is computed
  transiently on every `DescribeServices`/`enrichService` call under an
  RLock and is **not** written back into the stored `Service.Deployments` —
  this is intentional/pre-existing (matches the `DescribeServiceRevisions`
  "derive on read" pattern, see `addServiceRevisionLocked`'s doc comment). Do
  not flag this as a persistence bug; it's a deliberate simplification. Note:
  the new `ServiceDeployment.Status` this sweep added inherits the same
  limitation — it snapshots `RolloutState` at deployment-creation/rollback
  time and will not itself flip to `SUCCESSFUL` once the deployment
  converges, since nothing re-invokes `syncServiceDeploymentsLocked` on the
  read path. Consistent with existing precedent, not a regression, but a
  known simplification worth closing in a future sweep if `DescribeServiceDeployments`
  status accuracy becomes load-bearing for a test.
- ECS deployment-circuit-breaker threshold math
  (`circuitBreakerThreshold`/`deployment.go`) matches AWS's documented
  floor-3/ceiling-200/half-of-desired-count formula exactly — verified
  against https://docs.aws.amazon.com/AmazonECS/latest/developerguide/deployment-circuit-breaker.html,
  no changes needed.
- `builtinCapacityProvider` correctly synthesizes FARGATE/FARGATE_SPOT
  without requiring explicit `CreateCapacityProvider` calls, matching AWS
  (these are AWS-managed, always-available providers) — verified, no gap.
- Reconciler/janitor/lifecycle-stepper goroutine and ticker hygiene was
  re-verified this sweep (ctx-cancelled loops, `EvictCluster` cluster-delete
  hook releasing per-cluster semaphores, task-protection/lifecycle/
  tasksByInstance cleanup on both the fast and stop-delay paths) — all clean,
  no changes needed.

### Cross-service / out-of-scope (reported, not fixed — services/ecs/ only)

- ECS -> ELB/ELBv2 target registration: config-only (`Service.LoadBalancers`
  stored/echoed, never registers/deregisters targets in `services/elbv2`).
  Matches the task brief's "known gap" — confirmed still present, no bd issue
  found referencing it in the tracker at audit time.
- ECS -> Auto Scaling Group capacity providers: config-only
  (`AutoScalingGroupProvider` stored/echoed, never calls `services/autoscaling`
  to validate the ASG or drive managed scaling).
- Repo-wide (unrelated to ecs): at `last_audit_commit` the root package fails
  `go build ./...` with two `cwBk.PutMetricData` call-site mismatches in
  `cli.go` (lines 3220, 4259) — a pre-existing break from a concurrent
  CloudWatch-service sweep on this shared branch, confirmed unrelated to any
  ecs change (`cli.go` has zero diff in this sweep) and out of scope
  (services/ecs/ only, shared file). `go build ./services/ecs/...` and
  `go build ./...` excluding the root package both pass clean.

### 2026-08-29 -- ERROR PATH sweep: fabricated exception codes (class: iam ErrInvalidAction shape)

Extracted ground truth from all 77 `awsAwsjson11_deserializeOpError<Op>` switches
in `ecs@v1.90.0/deserializers.go` (JSON-RPC protocol, matched via
`strings.EqualFold` against `X-Amzn-ErrorType`/body `__type`) and cross-checked
every `awserr.New(...)` sentinel's code string against both that per-op ground
truth and `ecs@v1.90.0/types/errors.go`'s 29 real exception shapes.

**11 fabricated error codes found and fixed** -- each was a code string that
appears in **zero** of the 77 per-op switches AND has no corresponding type in
`types/errors.go` at all (the strongest signal, same as iam's `ErrInvalidAction`
being 0-of-176): `TaskNotFoundException`, `TaskDefinitionNotFoundException`,
`ClusterAlreadyExistsException`, `ServiceAlreadyExistsException`,
`AccountSettingNotFoundException`, `ContainerInstanceNotFoundException`,
`CapacityProviderNotFoundException`, `CapacityProviderAlreadyExistsException`,
`ExpressGatewayServiceNotFoundException`, `ExpressGatewayServiceAlreadyExistsException`.
All were removed from `errors.go`/their owning files along with their now-dead
`ErrXxx` sentinels; call sites now use whichever code that specific op's own
deserializer actually models:

- **StopTask, ExecuteCommand** (`tasks.go`), **DescribeTaskDefinition** /
  **DeregisterTaskDefinition** (`task_definitions.go`, via the shared
  `findTaskDefinitionLocked` also used by CreateService/UpdateService/RunTask/
  StartTask/CreateTaskSet/UpdateTaskSet), **DeleteAccountSetting**
  (`account_settings.go`), **DeregisterContainerInstance** /
  **UpdateContainerInstancesState** / **UpdateContainerAgent**
  (`container_instances.go`), **DeleteCapacityProvider** /
  **UpdateCapacityProvider** (`capacity_providers.go`): all now use
  `ErrInvalidParameter` ("InvalidParameterException"), which every one of the
  77 ops models -- the universal fallback once a fabricated code is ruled out.
- **CreateService** (`services.go`), **CreateCapacityProvider**
  (`capacity_providers.go`), **CreateExpressGatewayService**
  (`express_gateway.go`): duplicate-name/ARN case now uses
  `ErrInvalidParameter` too -- none of these three `Create*` ops model any
  "already exists" exception, matching the established real-AWS pattern
  (`InvalidParameterException: Creation of service was not idempotent.`).
- **DeleteExpressGatewayService** / **UpdateExpressGatewayService**
  (`express_gateway.go`): now use the pre-existing `ErrServiceNotFound`
  ("ServiceNotFoundException", the same code ordinary ECS services use) --
  both ops' own deserializers model that exact shape.
- **DescribeExpressGatewayService** (`express_gateway.go`): now uses a new
  `ErrResourceNotFound` ("ResourceNotFoundException") -- its own deserializer
  models a *different* code from its Delete/Update siblings for what is
  conceptually the same "service not found" condition; verified from its own
  switch, not assumed from the siblings.

**Second-shape bug (should not error at all): `CreateCluster`
(`clusters.go`) is idempotent in real ECS** -- calling it again with an
existing `ClusterName` returns the existing cluster (HTTP 200), not an error.
`CreateCluster`'s own deserializer models zero exceptions for this condition,
and no "ClusterAlreadyExistsException" type exists anywhere in the SDK, same
signal as `RemoveClientIDFromOpenIDConnectProvider`'s idempotency bug from the
iam pass. Fixed to return the existing cluster.

**Pre-existing tests asserting the fabricated codes as correct (found and
fixed, same shape as the iam `InvalidAction` test):**
`handler_clusters_test.go`'s `TestECS_CreateCluster_AlreadyExists` (asserted
400 `ClusterAlreadyExistsException`; renamed
`TestECS_CreateCluster_Idempotent`, now asserts 200),
`handler_services_test.go`'s `TestECS_CreateService_AlreadyExists`,
`handler_express_gateway_test.go`'s `TestECS_CreateExpressGatewayService_DuplicateARN`,
`handler_container_instances_test.go`'s `TestECS_DeregisterContainerInstance_NotFound`,
`handler_capacity_providers_test.go`'s `TestECS_CreateCapacityProvider_AlreadyExists`
(all four updated to assert the real `InvalidParameterException`).

New tests: `error_code_fixes_ecssweep_test.go`, all driving the real
`aws-sdk-go-v2/service/ecs` client and asserting via `errors.As` against the
SDK's own typed exception (or, for `CreateCluster`, asserting success on the
second call) -- confirmed failing against the pre-fix code for every case.

Gates: `go build ./services/ecs/...`, `go vet ./services/ecs/...` and
repo-wide `go vet ./...` (clean except a pre-existing, unrelated
`services/appconfig` failure from a concurrently-edited service), `go test
-race -count=1 ./services/ecs/...` (pass), `golangci-lint run --fix
./services/ecs/...` (0 issues).

**2026-08-30 (gopherstack request-field re-scan, `cmd/reqfieldscan`)**:
first pass of `cmd/reqfieldscan` (added `aa4ec0ad2`) against this service's
request fields -- the ecs pass noted above swept error codes only, request
fields were unscanned until now. Coverage: 77/77 dispatch-table ops (100%)
resolved via `service.WrapOp`, no unresolved ops, no blind spots this tool's
own doc discloses were hit (no local wrapper-around-WrapOp shape like
cognitoidp's `wrapAccuracy`, no non-`handle<Op>`-named handler this scan
needed a suffix guess for). 6 fields flagged; hand-verified each against
`aws-sdk-go-v2/service/ecs@v1.90.0`'s own serializers:

- **`ListDaemonTaskDefinitions.Revision` -- real bug, fixed.**
  `api_op_ListDaemonTaskDefinitions.go`: "Specify LAST_REGISTERED to return
  only the last registered revision for each daemon task definition family"
  -- the field's one documented enum value
  (`types.DaemonTaskDefinitionRevisionFilterLastRegistered`). The handler
  built its backend query from `Family`/`FamilyPrefix`/`Status` only, never
  read `Revision`, so passing `LAST_REGISTERED` silently returned every
  revision of every matching family instead of narrowing to each family's
  highest. Fixed by filtering the already-family+revision-sorted result set
  down to one entry per family when `Revision` case-insensitively equals
  `"LAST_REGISTERED"`. Proof:
  `TestECS_ListDaemonTaskDefinitions_RevisionLastRegistered`
  (`handler_daemon_test.go`), real typed SDK client, confirmed failing
  (returned all 5 registered revisions instead of the 2 latest) against the
  unfixed code.
- **`CreateDaemon.ClientToken` -- verified, structural, not fixed.** No
  idempotency-token dedup pattern exists anywhere else in this service --
  grepped the whole package for `ClientToken`; this is the only field of
  that name in the entire service, meaning no `Create*`/`Run*` op here
  implements request-token deduplication. Consistent with the rest of the
  service rather than a localized gap; implementing dedup would be a new
  cross-cutting feature, not a narrow wire-field fix.
- **`DiscoverPollEndpoint.Cluster`/`.ContainerInstance` -- verified, not a
  bug.** `discoverPollEndpointInput`'s own doc comment states plainly:
  "Currently unused: the handler discards its input" -- the handler's
  parameter is even declared `_ *discoverPollEndpointInput`. A deliberately
  disclosed simplification (a single global poll endpoint regardless of
  cluster/instance), not a silent gap.
- **`ListContainerInstances.Filter` -- verified, structural, not fixed.**
  Real AWS's `filter` here is a Cluster Query Language expression (e.g.
  `attribute:ecs.instance-type =~ t2.*`). Grepped the service for any
  existing CQL parsing (for this op or any sibling `List*` op with a
  `filter` parameter): none. A whole unimplemented query-language feature,
  not a dropped-field fix.
- **`RegisterContainerInstance.InstanceIdentityDocumentSignature` --
  verified, not a bug.** The struct's own doc comment states it is "accepted
  for wire-shape completeness but not cryptographically verified by this
  emulator" -- a deliberate, disclosed simplification (verifying an EC2
  instance identity document signature against AWS's public certificate
  chain is out of scope for an emulator with no real EC2 backing it).

Gates: `go build ./services/ecs/...`, `go build ./...` (repo-wide, clean),
`go vet ./services/ecs/...`, `go vet ./...` (repo-wide, clean), `go test
-race -count=1 ./services/ecs/...` (pass), `golangci-lint run
./services/ecs/...` (0 issues). Work left uncommitted per this pass's
instructions.

## 2026-08-31 (gopherstack-4glf, never-declared-field sweep, `cmd/reqfielddiff`)

`go run ./cmd/reqfielddiff -dir ecs` reported 15 tier-1 findings ("documented
default", the axis `reqfieldscan` structurally cannot see: a field never
declared anywhere in this backend has no struct member for that scanner to
enumerate). All 15 judged genuinely fixable and fixed -- store the field
(declaring it for the first time) and, where the SDK names one fixed default
value, fill it on omission; where the SDK's own doc explicitly says the
behaviour is contingent (not a fixed value), the field is stored/echoed but
no default is fabricated.

- **`CreateCluster`/`UpdateCluster.ServiceConnectDefaults`** -- entirely
  undeclared; a prior pass's comment on `updateClusterInput` explicitly said
  "not modeled by this backend" (still true for `configuration`, now stale
  for this field). Added `Cluster.ServiceConnectDefaults`
  (`*ClusterServiceConnectDefaults{Namespace}`), stored on create, updated
  only when explicitly supplied on update (mirrors `Settings`'
  if-non-nil precedent), echoed on Create/Update/DescribeClusters. Config-only
  -- this backend does not model Service Connect namespace resolution at
  CreateService time (no `ServiceConnectConfiguration.Namespace` fallback is
  simulated either), matching the existing config-only precedent already
  accepted for `Service.LoadBalancers`/`AutoScalingGroupProvider`.
- **`CreateService`/`UpdateService.AvailabilityZoneRebalancing`** -- own doc
  comment: create defaults to `ENABLED` when unspecified; update defaults to
  the existing service's value (a no-op update naturally falls out of
  "only overwrite when the update input is non-empty", once the field is
  stored at all). Added `Service.AvailabilityZoneRebalancing`.
- **`CreateService`/`UpdateService.HealthCheckGracePeriodSeconds`** -- own
  doc comment: "If you do not specify a health check grace period value, the
  default value of 0 is used." Added `Service.HealthCheckGracePeriodSeconds
  *int`; create defaults to a non-nil `0` (not left nil/omitted -- the same
  vanishing-default shape as the `StartRun.NetworkingMode` omics bug fixed
  earlier this campaign), update applies only when the pointer is non-nil.
- **`CreateService`/`UpdateService.Monitoring`** -- own doc comment
  describes a default CloudWatch resolution, but real AWS echoes this field
  on `types.ServiceRevision`, not on `types.Service` itself (verified against
  `ecs@v1.90.0/types/types.go`). Added `Service.Monitoring
  *MonitoringConfiguration` (new type, mirrors `types.MonitoringConfiguration`/
  `types.MetricConfiguration`) threaded through to `ServiceRevision.Monitoring`
  in `buildServiceRevision`. Stored/echoed only -- this backend emits no real
  CloudWatch metrics, so no resolution behaviour is simulated (config-only,
  same precedent as above).
- **`UpdateService.ForceNewDeployment`** -- own doc comment: "you can use
  this option to start a new deployment with no service definition changes."
  `UpdateService` previously rotated the PRIMARY deployment (`newActiveDeployment`
  demoting the prior PRIMARY to ACTIVE) only when `TaskDefinition` itself
  changed, so `ForceNewDeployment=true` with no other change was silently a
  no-op. Fixed: the deployment is now rotated whenever `TaskDefinition`
  changed OR `ForceNewDeployment` is true (reusing the current task
  definition in the latter case, matching real AWS's "same image/tag"
  example). This affects a rotation decision the backend already makes,
  not a new capability.
- **`RegisterDaemonTaskDefinition.IpcMode`/`.PidMode`** -- own doc comments:
  "The default is `none`." for both. Added `DaemonTaskDefinition.IpcMode`/
  `.PidMode`, defaulted to `"none"` on omission, echoed on
  Register/DescribeDaemonTaskDefinition. Config-only (see `RegisterTaskDefinition`
  entry below for why real per-container namespace sharing isn't attempted).
- **`RegisterTaskDefinition.IpcMode`/`.PidMode`** -- own doc comments
  describe the *un*-set behaviour as contingent ("depends on the Docker
  daemon setting on the container instance" for IpcMode; no named enum value
  for PidMode's "private namespace" case), not a single fixed value, so no
  default is fabricated on omission. Added `TaskDefinition.IpcMode`/`.PidMode`,
  stored/echoed as given. This service's `docker_runner.go` does run real
  Docker containers, so actually enforcing shared IPC/PID namespaces across a
  task's containers (Docker `HostConfig.IpcMode`/`.PidMode`
  `"container:<id>"` chaining, ordering the first container's creation before
  the rest) is a real capability this backend could eventually grow into --
  deliberately not attempted this pass: the multi-container chaining is
  enough additional surface (creation ordering, partial-failure handling)
  that a rushed version risked shipping a subtly wrong simulation, which the
  campaign's own guidance rates worse than the gap. Recorded as a genuine
  follow-up, not a refusal.
- **`RegisterTaskDefinition.EnableFaultInjection`** -- own doc comment:
  default `false`, which is Go's zero value, so no explicit defaulting code
  is needed. Added `TaskDefinition.EnableFaultInjection bool`, stored/echoed.
  Config-only -- this is real AWS FIS's inbound fault-injection-from-within-
  the-task-agent capability, unrelated to this repo's own `pkgs/chaos`/
  `aws:ecs:stop-task` FIS action already wired in `fis.go`; no such inbound
  agent endpoint exists here to gate.
- **`ListAccountSettings.EffectiveSettings`** -- own doc comment: "If true,
  the account settings for the root user or the default setting for the
  principalArn are returned." This is the campaign's flagged strong
  candidate: it changes which records a listing returns, and the backend
  already has the records (`PutAccountSettingDefault` already stores an
  account-level default under an empty `PrincipalArn`). Implemented:
  `effectiveSettings=true` returns `principalArn`'s own explicit setting per
  name, falling back to the empty-`PrincipalArn` default for any name
  `principalArn` has no explicit value for; `effectiveSettings=false`
  (default) is unchanged -- exact-match filtering only, no fallback.
  `Backend.ListAccountSettings` gained a third `effectiveSettings bool`
  parameter (interface + one internal test call site updated).

15 of 15 tier-1 findings judged genuinely fixable (0 recorded as
unmodellable) -- unlike prior sweeps in this campaign, every one of these
fell into the "reflect back a stored value" or "affects a decision the
backend already makes" categories the campaign's own guidance calls
honourable, and none required simulating a capability (real Cloud Map
namespace resolution, real CloudWatch metric emission, real per-container
Docker namespace sharing, a real inbound FIS agent endpoint) that this
backend structurally lacks -- those remain config-only/stored-and-echoed,
consistent with this service's existing `LoadBalancers`/
`AutoScalingGroupProvider` precedent, and are disclosed as such above rather
than silently claimed as enforced.

New tests: `wire_field_additions_ecssweep_test.go`, all driving the real
`aws-sdk-go-v2/service/ecs` client. Every default-value test (`AvailabilityZoneRebalancing`
create-default, `HealthCheckGracePeriodSeconds` create-default, daemon
`IpcMode`/`PidMode` default) omits the field entirely rather than setting it
explicitly. Confirmed failing pre-fix by temporarily reverting the specific
defaulting/rotation/fallback logic under test (not by removing the new
struct fields, since most of these fields did not exist before this pass and
removing them would fail the whole package to compile rather than
demonstrate a behavioural gap): `AvailabilityZoneRebalancing` create-default,
`AvailabilityZoneRebalancing` update-preserves-existing,
`HealthCheckGracePeriodSeconds` create-default, `ForceNewDeployment`
rotation, and `EffectiveSettings` fallback all reproduced their expected
pre-fix failures, then were restored byte-identical (`md5sum`-verified) and
re-confirmed green. Assertion count: 0 existing assertions changed or
dropped; all new.

Gates: `go build ./services/ecs/...`, `go vet ./services/ecs/...` (both
clean), `go test -race -count=1 ./services/ecs/...` (pass), `golangci-lint
run ./services/ecs/...` (0 issues, after decomposing `CreateService`
(funlen) into `createServiceDefaults` and `ListAccountSettings` (gocognit)
into `filterAccountSettings`/`effectiveAccountSettings`). Work left
uncommitted per this pass's instructions.
