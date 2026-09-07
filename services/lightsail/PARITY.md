---
# PARITY MANIFEST -- IMPLEMENTED. See "Implementation summary (this pass)" below the frontmatter
# for what this re-audit actually verified by reading code, the two real gaps it found (neither
# claimed by the implementing commit's own file comments), and the grade justification. The original
# pre-implementation audit prose (everything from "## Purpose of this document" onward, including
# every family table's Input/Output/Errors columns) is left UNMODIFIED below as the wire-shape
# ground truth the implementation was built from -- it is still accurate as a wire-shape reference;
# only its "gap" verdicts were stale. This pass touched ONLY this file: no .go file, go.mod, go.sum,
# or cli.go was read or written to produce the status change below (all evidence came from `go
# build`/`go vet`/`go test -race`/`golangci-lint` against the already-committed tree, plus direct
# reading of services/lightsail/*.go and the relevant cli.go wiring lines, never modifying either).
service: lightsail
sdk_module: aws-sdk-go-v2/service/lightsail@v1.58.4   # gopherstack-u8my: was recorded v1.58.3 but
# go.mod actually pinned v1.58.4 -- the "confirmed unchanged" claim below was wrong. Diffed v1.58.3
# vs v1.58.4: types/{types,enums,errors}.go, serializers.go, deserializers.go, validators.go are all
# byte-identical; only client middleware plumbing differs. No wire-shape claim in this file rested
# on the wrong pin. Originally resolved via `go get .../lightsail@latest` in a throwaway scratch
# module, and the version sdk_completeness_test.go's real *lightsailsdk.Client{} type-checks against.
last_audit_commit: c397a0243   # the commit that actually implemented all 161 ops, registered the
# handler, and wired cli.go. A follow-up pass on top of that HEAD (same day) closed the
# CreateCloudFormationStack wiring gap below; see "cli.go wiring" section for the cli.go diff, which
# is the only .go-file change this follow-up pass made (plus errors.go's disclosure comment).
last_audit_date: 2026-08-01
overall: A   # raised from A- by a follow-up pass that closed the one load-bearing gap the re-audit
# above found: cli.go now calls wireLightsailCloudFormation(byName["Lightsail"], cfnSvc) from
# registerCloudFormationAndDashboard (not from wireStorageAndSecretsIntegrations -- CloudFormation
# isn't registered yet when that runs), wiring a real cfnLightsailStackAdapter that calls the real
# services/cloudformation backend's CreateStack. Verified end-to-end (not just compiled) via a
# throwaway root-package test, since deleted, that drove this repo's own
# initializeServices/wireCrossServiceDependencies/serviceByName path with a real CLI/AppContext,
# created a real Lightsail instance/snapshot/export record, called the real
# CreateCloudFormationStack, and confirmed (a) the CloudFormationStackRecord's DestinationInfoID was
# populated and its State was SUCCEEDED, and (b) the real services/cloudformation backend's
# ListAll() returned exactly one real Stack whose StackName matched the record. This closes the
# wire-reachability gap that kept this package off A -- directly mirroring how mgn's own B+->A-
# follow-up pass closed its identical SetS3Backend-never-called gap. The second re-audit finding (5
# of 8 wire exception shapes never constructed by any call site) is not fixed -- this backend
# genuinely has no permission, account-setup, or region-setup state model to hang a real
# AccessDeniedException/AccountSetupInProgressException/OperationFailureException/
# RegionSetupInProgressException/UnauthenticatedException trigger off of, and inventing one purely
# to exercise a constructor would be exactly the fabrication parity-principles.md forbids -- but it
# is now DISCLOSED directly in errors.go (mirroring mgn/errors.go's identical disclosure of its own
# unused errAccessDenied/errQuotaExceeded/errThrottling), which was the specific thing this package
# was previously docked for not doing (mgn's own analogous gap was already disclosed and did not by
# itself keep mgn below A-). All 161 ops routed (sdk_completeness_test.go passes with an empty
# exception list), real per-op InMemoryBackend state (not stub echoes), real Operation-model
# Started->Succeeded async transitions (operations.go), a real 7-state/9-detail-code/
# 4-deployment-state container-service machine that walks its documented intermediate steps on real
# wall-clock timers rather than jumping straight to RUNNING, cli.go registration AND
# resourcegroupstaggingapi tag-bridge wiring both already present (wireTaggingLightsail,
# cli.go:7061). `go build`/`go vet`/`go test -race -count=1` all clean (3+ consecutive race runs
# across both services/lightsail/... and services/cloudformation/..., since this pass touches
# cloudformation's cli.go wiring too); `golangci-lint run` reports 0 issues; `grep -rnE
# '//nolint:.*(funlen|gocyclo|gocognit|cyclop)' services/lightsail/` is empty -- no banned complexity
# suppressions. The six MetricData ops, GetCostEstimate, GetContainerLog, and
# GetRelationalDatabaseLogEvents all genuinely return real, well-formed, EMPTY payloads (verified by
# reading every one of those handler bodies directly, not inferred from doc comments) -- exactly the
# honest non-fabrication this audit's own brief called for, so these do NOT count against the grade.
# No fabrication anywhere in this pass: the CloudFormation handoff creates a real Stack from real
# instance/export data (no invented template resources -- see cfnLightsailStackAdapter's own doc
# comment in cli.go for why an empty template body was the honest choice over fabricating EC2
# resources Lightsail's export never actually described), and the 5 unused error shapes are
# disclosed, not silently missing or wrongly triggered.
families:
  reference_data: {status: ok, note: "9 ops, referencedata.go/handler_referencedata.go. Real filtering (appCategory/IncludeInactive/pagination) over small, explicitly-labeled-SYNTHETIC seed tables (seedBlueprints/seedBundles/seedRDSBlueprints/seedRDSBundles/seedBucketBundles/seedDistributionBundles/seedContainerServicePowers) -- an emulator decision, not a claim about AWS's real current catalog/pricing, exactly as the pre-implementation audit recommended. GetRegions is genuinely SDK-derived (types.RegionName's own Values()), the one part of this family sourced from the SDK rather than invented. GetContainerAPIMetadata correctly omits InvalidInputException (its own zero-field input)."}
  instances_core: {status: ok, note: "9 ops, instances.go. Real create/delete/list/reboot/start/stop with a genuine Pending->Running->Stopped state walk on real timers (asyncTransitionDelay). InstanceStateCode* constants (consts.go) are the conventional EC2 numeric mapping, EXPLICITLY commented UNCONFIRMED since this SDK module publishes no typed enum/numeric mapping for Lightsail instance state at all -- not presented as SDK-confirmed."}
  instance_access_ports: {status: ok, note: "8 ops, instance_access.go/handler_instance_access.go. Real port-rule CRUD (Open/Close/Put), PortStateOpen hardcoded per the SDK's own doc comment ('port state for Lightsail instances is always open'), real SetupInstanceHttps/GetSetupHistory audit trail, real DeleteKnownHostKeys bookkeeping."}
  instance_snapshots: {status: ok, note: "4 ops, instance_extras.go. Real FromAttachedDisks capture (Path/SizeInGb metadata, no byte-for-byte restore -- consistent with how this repo already handles snapshots elsewhere)."}
  instance_metrics_metadata: {status: partial, note: "2 ops. UpdateInstanceMetadataOptions is real (instance_extras.go). GetInstanceMetricData deliberately returns a real, well-formed, EMPTY MetricData (instance_extras.go:127) after validating the instance exists -- an honest non-fabrication choice per this family's own explicit warning, marked partial (not ok) rather than gap since the wire/error/existence-check plumbing is fully real, only the telemetry content is intentionally absent."}
  auto_snapshots_addons: {status: ok, note: "4 ops, addons.go. Real AddOn CRUD (config toggle per resource); EnableAddOn with AutoSnapshot seeds one real AutoSnapshotDetails entry at enable time. Minor undisclosed scope note (not a fabrication): no ongoing scheduled daily-snapshot cadence runs after that initial entry -- GetAutoSnapshots will not accumulate further entries on its own over wall-clock days the way a real account would."}
  key_pairs: {status: ok, note: "6 ops, keypairs_staticips.go. CreateKeyPair returns real generated key material exactly once; DownloadDefaultKeyPair is a true lazily-created account/region singleton, matching the pre-implementation audit's spec exactly."}
  static_ips: {status: ok, note: "6 ops, keypairs_staticips.go. Real attach/detach against a named instance, no ENI concept, as spec'd."}
  disks: {status: ok, note: "7 ops, disks.go. DiskState is a real typed 5-value enum (consts.go) driving genuine available<->in-use transitions on Attach/Detach; AutoMounting/AutoMountStatus bookkeeping-only by explicit design (no real guest OS to mount into), documented at the call site."}
  disk_snapshots: {status: ok, note: "5 ops incl. CopySnapshot, disks.go. CopySnapshot is an honest, documented scoped-down same-process record copy, never claiming a real second-region call (disks.go:357-358's own comment)."}
  export_cloudformation: {status: ok, note: "4 ops, exportcfn.go. ExportSnapshot/GetExportSnapshotRecords/GetCloudFormationStackRecords are fully real. CreateCloudFormationStack's real cross-service handoff (CloudFormationBackend.CreateStackFromLightsail, store.go) is now WIRED: cli.go's registerCloudFormationAndDashboard calls wireLightsailCloudFormation(byName[\"Lightsail\"], cfnSvc) right after CloudFormation is registered (it cannot run from wireStorageAndSecretsIntegrations/wireCrossServiceDependencies like wireMGNS3/wireDynamoDBS3 do, since those run before CloudFormation exists), handing off to a cfnLightsailStackAdapter that calls the real services/cloudformation backend's CreateStack with an empty template body (no fabricated EC2 resources -- Lightsail's export gives instance names, not a template) and one Parameter per source instance name. Verified end-to-end via a throwaway root-package test (since deleted) that drove real initializeServices, created a real instance/snapshot/export, called CreateCloudFormationStack, and confirmed a real Stack now exists in the CloudFormation backend with DestinationInfoID populated and State SUCCEEDED. Previously the one confirmed genuine cross-service handoff in this whole 161-op surface was unreachable in the running application; that gap is now closed."}
  load_balancers: {status: partial, note: "9 ops, loadbalancers.go/handler_loadbalancers.go. 8/9 fully real (create/delete/attach/detach/UpdateLoadBalancerAttribute/SetIpAddressType, real InstanceHealthState per-instance tracking). GetLoadBalancerMetricData deliberately returns real, well-formed, EMPTY MetricData (loadbalancers.go:298) after existence validation -- same honest non-fabrication pattern as family E."}
  lb_tls_certs: {status: ok, note: "5 ops, loadbalancers.go. Real cert lifecycle distinct from the CDN-facing Certificate family, GetLoadBalancerTlsPolicies a real static policy catalog."}
  relational_databases_core: {status: ok, note: "9 ops, databases.go. Real Creating->Available/Starting/Stopping/Rebooting state walk. RelationalDatabaseState string constants (consts.go) EXPLICITLY commented UNCONFIRMED (no typed SDK enum exists at all), matching the pre-implementation audit's own flagged unknown -- not presented as SDK-confirmed."}
  relational_databases_ops: {status: partial, note: "7 ops, databases.go. GetRelationalDatabaseEvents (real recorded event log), GetRelationalDatabaseLogStreams (real seed stream-name catalog), GetRelationalDatabaseMasterUserPassword (genuinely stores/returns CURRENT and PREVIOUS password material; PENDING falls back to CURRENT since this backend applies changes immediately, documented), and GetRelationalDatabaseParameters/UpdateRelationalDatabaseParameters are all real. GetRelationalDatabaseMetricData (databases.go:495) and GetRelationalDatabaseLogEvents (databases.go:442) both deliberately return real, well-formed, EMPTY payloads -- this backend runs no real MySQL server to produce genuine log lines or metrics from, and both call sites say so explicitly. 5 of 7 ops fully real; 2 honestly empty by design."}
  relational_database_snapshots: {status: ok, note: "4 ops, databases.go. Real snapshot CRUD, restored via CreateRelationalDatabaseFromSnapshot in family N."}
  container_services_core: {status: partial, note: "5 ops, containers.go. 4/5 fully real, including the single most complex state machine in this service: ContainerServiceState/StateDetailCode/per-deployment ContainerServiceDeploymentState genuinely walk their documented intermediate steps (CREATING_SYSTEM_RESOURCES -> CREATING_NETWORK_INFRASTRUCTURE -> ... -> DEPLOYING sub-codes -> RUNNING) on real wall-clock timers, never jumping straight to RUNNING -- exactly what the pre-implementation audit warned a rushed implementation would skip. GetContainerServiceMetricData (containers.go:307) deliberately returns real, well-formed, EMPTY MetricData, same honest pattern as families E/L/O/S/T. Explicit, disclosed scope decision (containers.go's own file header): state-machine bookkeeping only, no real image is ever pulled or run via pkgs/container -- a defensible, clearly-labeled MVP, not a silent claim of full container execution."}
  container_deployments_images: {status: partial, note: "7 ops, containers.go. CreateContainerServiceDeployment/GetContainerServiceDeployments/CreateContainerServiceRegistryLogin/RegisterContainerImage/GetContainerImages/DeleteContainerImage are all real (real per-label monotonic `:service.label.N` image versioning, real CurrentDeployment/NextDeployment handoff, real 12-hour-expiring synthetic registry credentials never claimed as real ECR-issued). GetContainerLog (containers.go:451) deliberately returns a real, well-formed, EMPTY log-event page -- this backend runs no real container to produce genuine log output from, documented at the call site as the same honesty rationale as GetRelationalDatabaseLogEvents."}
  buckets: {status: partial, note: "10 ops, buckets.go/handler_buckets.go. 9/10 fully real (independent of this repo's real services/s3, matching the pre-implementation audit's own recommendation; CreateBucketAccessKey returns real secret material exactly once, matching CreateKeyPair's pattern). GetBucketMetricData (handler_buckets.go:212) deliberately returns real, well-formed, EMPTY MetricData after existence validation."}
  distributions: {status: partial, note: "10 ops, certificates_distributions.go. FIXED (gopherstack-jigw, 2026-08-13): was claimed '9/10 fully real', which was false and undisclosed -- CreateDistribution silently dropped its own REQUIRED member DefaultCacheBehavior (api_op_CreateDistribution.go, client-side-validated in validators.go's validateOpCreateDistributionInput via NewErrParamRequired) along with the optional CacheBehaviorSettings/CacheBehaviors/ViewerMinimumTlsProtocolVersion: createDistributionRequest (handler_distributions_certs.go) never decoded any of the four, Distribution (models.go) had no fields for three of them (ViewerMinTLSVersion existed but was dead -- assigned nowhere), and distributionWire never echoed any. A distribution with no cache configuration silently created and returned success. Now: CreateDistribution rejects a missing/empty DefaultCacheBehavior as InvalidInputException (declared in awsAwsjson11_deserializeOpErrorCreateDistribution's own error set -- CreateDistribution has no ValidationException in its catalog, unlike some other lightsail ops); all four fields round-trip through GetDistributions (CacheBehavior/CacheBehaviorPerPath/CacheSettings/CookieObject/HeaderObject/QueryStringObject modeled in models.go, wired in handler_distributions_certs.go). UpdateDistribution, which had the identical undisclosed gap (accepted only CertificateName/IsEnabled despite the real UpdateDistributionInput supporting the same four fields plus Origin), now also accepts and replaces DefaultCacheBehavior/CacheBehaviorSettings/CacheBehaviors/ViewerMinimumTlsProtocolVersion -- Origin is NOT wired (disclosed gap below, unchanged scope). See TestDistributionCacheBehaviorRoundTrip and TestCreateDistribution_RequiresDefaultCacheBehavior (sdk_roundtrip_network_test.go). GetDistributionMetricData (certificates_distributions.go:306) deliberately returns real, well-formed, EMPTY MetricData -- unchanged, still the one disclosed gap of 10."}
  domains_dns: {status: ok, note: "7 ops, domains.go. Domain.Arn genuinely uses the literal region segment \"global\" (domainGlobalRegion, consts.go/store.go's globalARN) matching the SDK's own doc-comment example exactly, not pkgs/arn.BuildGlobal's empty-segment convention -- a deliberate, documented divergence."}
  certificates: {status: ok, note: "3 ops, certificates_distributions.go. Real CDN-facing certificate lifecycle, distinct from the LB-TLS-certificate family."}
  alarms_contacts: {status: partial, note: "8 ops, alarms_contacts.go. Alarm/contact-method CRUD and state storage is fully real; TestAlarm (a pure caller-driven State set against an explicit input, not an evaluation) is faithfully implemented. PutAlarm's automatic threshold evaluation against real metric data is explicitly NOT implemented (alarms_contacts.go's own file header: 'meaningless without real MetricDatapoint values this emulator does not honestly have') -- exactly option (a) of the two the pre-implementation audit itself proposed as defensible, chosen and disclosed rather than silently skipped."}
  vpc_peering: {status: ok, note: "3 ops, tagging_vpc_misc.go. Real single implicit account-wide boolean, zero-input-field ops as spec'd."}
  tagging: {status: ok, note: "2 ops, tagging_vpc_misc.go. Real name-first resolution (ResourceArn accepted but resourceName is what's actually keyed on, matching the wire spec's actually-required field). The 4 of 20 ResourceType kinds with no Tags field on their own SDK struct (StaticIp, PeeredVpc, ExportSnapshotRecord, CloudFormationStackRecord -- tagsNotSupportedKinds, tagging_vpc_misc.go) are resolved by name and then HONESTLY REFUSE with a validation error, rather than silently no-op'ing or fabricating a tag store for them -- verified by direct read, exactly the wire-shape asymmetry the pre-implementation audit called out."}
  operations: {status: ok, note: "3 ops, operations.go. Genuinely real: every mutating op creates a Started (never synchronously-fabricated-terminal) Operation and schedules a real async transition to Succeeded via pkgs/worker, following services/eks/services/grafana's established timer pattern. The Succeeded-vs-Completed per-op split this SDK leaves undocumented is resolved by a single, disclosed, UNCONFIRMED convention (always Succeeded) rather than a guessed per-op mapping."}
  gui_sessions: {status: ok, note: "3 ops, tagging_vpc_misc.go. Real SettingUp->Ready timer-driven state walk per instance, real Stop/restart bookkeeping."}
  misc: {status: partial, note: "2 ops, tagging_vpc_misc.go. GetActiveNames is fully real (backed directly by the activeNames global-uniqueness index every other family maintains). GetCostEstimate (tagging_vpc_misc.go:729) deliberately returns a real, well-formed, EMPTY cost-estimate response after existence validation -- a real cost estimate needs real usage-based billing logic this emulator has no grounds to fabricate, disclosed at the call site."}
gaps:
  - "2026-08-30 (region-isolation sweep, fix/wrapper-key-sweep-rds-cloudwatch-sqs-sns): checked
    the cloudwatchlogs/memorydb bug class (an identifier/storage key built from the backend's
    fixed default region instead of the request's) against this service. Confirmed CLEAN, and by
    a stronger margin than a mere absence of evidence: this service's OWN code explicitly
    documents the intended architecture at disks.go's CopySnapshot (the one genuinely cross-region
    op in the whole 161-op surface) -- \"This repo models each AWS region as its own separate
    InMemoryBackend instance\" -- and every handler in this package discards ctx
    ((_ context.Context, body []byte)) because NewInMemoryBackend(ctx, accountID, region) fixes
    both identity dimensions once, at construction, for the life of the instance; every
    store_setup.go KeyFn is Name-alone (not even AccountID-scoped, since one instance is also one
    account). This is the same single-account-single-region-per-process design already
    independently confirmed correct for regionalARN/globalARN/distributionARN (store.go, section
    5.1/1047-1055 above -- Domain literal-\"global\", Distribution region-agnostic-but-reports-
    us-east-1, everything else regional-via-b.region) with zero sibling inconsistency: no operation
    anywhere in this package derives region from a request the way services/ssm's
    getRegion(ctx)/httputils.ExtractRegionFromRequest does (confirmed absent from this package).
    Not a bug per this task's own criterion that a uniformly single-region service can be a
    legitimate design -- no fix made."
  - "NEW this pass (gopherstack-jigw, 2026-08-13): UpdateDistributionInput.Origin
    (*types.InputOrigin) is real and optional but not wired -- UpdateDistribution
    (certificates_distributions.go) now accepts and replaces
    CertificateName/IsEnabled/DefaultCacheBehavior/CacheBehaviorSettings/
    CacheBehaviors/ViewerMinimumTlsProtocolVersion but has no code path for
    changing a distribution's origin resource after creation. Disclosed at
    UpdateDistributionRequest's own doc comment (certificates_distributions.go)
    rather than silently accepted-and-dropped like the DefaultCacheBehavior bug
    this same pass fixed."
  - "NEW this pass (gopherstack-jigw, 2026-08-13): SetupInstanceHttpsInput.EmailAddress
    is decoded (handler_instance_access.go) but not passed to
    Backend.SetupInstanceHTTPS -- SetupInstanceHTTPS's signature has no parameter
    for it and SetupHistoryEntry never stores it. Confirmed genuinely unobservable,
    not merely undisclosed: EmailAddress does not appear anywhere in
    aws-sdk-go-v2/service/lightsail/types/types.go, so no real read API (including
    GetInstanceSetupHistory) could ever echo it back even if this backend stored it.
    Left inert with a comment at the decode site rather than wired to a field with
    nothing real to observe it."
  - "RESOLVED this pass: CreateCloudFormationStack's real cross-service handoff to services/cloudformation (CloudFormationBackend.CreateStackFromLightsail) was implemented correctly but UNREACHABLE (SetCloudFormationBackend, store.go, had zero call sites anywhere in this repo). Fixed by adding cli.go's cfnLightsailStackAdapter + wireLightsailCloudFormation, called from registerCloudFormationAndDashboard (the only place both Lightsail and a just-constructed CloudFormation handler are simultaneously available -- wireStorageAndSecretsIntegrations/wireCrossServiceDependencies run before CloudFormation is registered, so wiring from there, as first attempted, is a silent no-op; this matters for anyone repeating this fix pattern elsewhere). Verified end-to-end via a throwaway root-package test (since deleted per this task's own instructions): real initializeServices, a real Lightsail instance -> snapshot -> ExportSnapshot -> CreateCloudFormationStack chain, and confirmed the real services/cloudformation backend's ListAll() now returns the created Stack, with the CloudFormationStackRecord's DestinationInfoID populated and State SUCCEEDED. Directly analogous to mgn's own original SetS3Backend-never-called gap and its dedicated follow-up-pass fix (mgn's PARITY.md, 'gopherstack-i6oz follow-up pass')."
  - "PARTIALLY ADDRESSED this pass: 5 of the 8 wire exception shapes this service's classifyLightsailError (errors.go) correctly maps to the right HTTP status/`__type` string -- AccessDeniedException, AccountSetupInProgressException, OperationFailureException, RegionSetupInProgressException, UnauthenticatedException -- are still never actually returned by any business-logic call site in this package (unchanged: grepping errAccessDenied/errAccountSetup/errOperationFailure/errRegionSetup/errUnauthenticated still returns zero hits outside errors.go's own definitions). Checked this pass whether any had an unambiguous correct call site per the SDK's own doc comments (aws-sdk-go-v2/service/lightsail/types/errors.go): none do -- AccessDeniedException/UnauthenticatedException need a caller-identity/permission model this backend doesn't have; AccountSetupInProgressException/RegionSetupInProgressException need an account/region provisioning-state model (like mgn's InitializeService) this backend doesn't have either; OperationFailureException's own doc comment ('an operation fails to execute') names no specific operation to hang a trigger off of. Wiring any of them would mean inventing a state/permission model purely to exercise a constructor -- fabrication, not a genuine fix -- so none were wired. What WAS fixed: errors.go itself now discloses this gap directly (mirroring mgn/errors.go's identical disclosure of its own unused errAccessDenied/errQuotaExceeded/errThrottling), which is the specific thing this package was previously docked for not doing relative to mgn's otherwise-identical situation. This means the family tables below, which list e.g. '+AcctSetup +NotFound +OpFailure +RegionSetup' as the real per-op AWS error signature for 103 of 161 ops, still describe what the REAL AWS API returns, not what THIS emulator will ever actually produce -- this emulator's real observable error surface, for every op, remains {InvalidInputException, NotFoundException, ServiceException}."
  - "InstanceState (GetInstanceState, embedded in Instance) has no typed SDK enum (confirmed unchanged from the pre-implementation audit); this backend's InstanceStateCode*/InstanceStateName* constants (consts.go) are the conventional EC2 numeric mapping, EXPLICITLY commented as an UNCONFIRMED, non-SDK-sourced convention at the const block itself -- carried through correctly from audit to implementation, not silently presented as confirmed."
  - "RelationalDatabaseState has no typed SDK enum (confirmed unchanged); this backend's RelationalDatabaseState* constants (consts.go) are similarly commented UNCONFIRMED, following general AWS RDS-family convention rather than anything this SDK module actually publishes -- carried through correctly."
  - "No AWS::Lightsail::* CloudFormation resource type exists in this repo's services/cloudformation/ (not independently re-checked this pass; the original audit's `grep -rli lightsail services/cloudformation/*.go` zero-hit finding was not disputed by anything read this pass)."
  - "No ListTagsForResource op exists in this 161-op surface (confirmed unchanged); TagResource/UntagResource resolve by ResourceName, matching the original audit's spec exactly, implemented in tagging_vpc_misc.go."
  - "Container services are explicitly, disclosedly state-machine bookkeeping only -- no image is ever pulled or run via pkgs/container (containers.go's own file header states this as a scope decision, not a silent gap), matching the 'legitimate, honestly-labeled MVP' option the pre-implementation audit explicitly allowed for."
  - "EnableAddOn's AutoSnapshot add-on seeds exactly one AutoSnapshotDetails entry at enable time (addons.go) but runs no ongoing scheduled daily-snapshot cadence afterward -- a minor, real scope limitation this re-audit found that is not disclosed at its own call site (unlike nearly everything else in this package)."
deferred:
  - "A full per-op {wire, errors, state, persist} grid (161 rows) was not written into this frontmatter, in favor of per-family status plus explicit per-op call-outs within each family's note above -- with 28 families already enumerating all 161 ops individually in the body's section 3 tables (left unmodified as ground truth), a second 161-row restatement here would duplicate rather than add information. Any future audit needing finer grain than family-level should start from the body's existing per-op tables plus this frontmatter's per-family notes, not re-derive from scratch."
  - "Whether real EC2/ELB/RDS state should eventually back Instance/LoadBalancer/RelationalDatabase (PARITY.md 5.2's architectural question) remains unresolved -- this implementation chose independent modeling (matching the original audit's own recommendation), not revisited by this pass."
leaks: {status: clean, note: "store.go's Close() stops the pkgs/worker.Group backing every scheduled transition (instance state, container-service steps, GUI session ready, export/CFN-stack completion, disk/RDS/operation timers); confirmed via `go test -race -count=1 ./services/lightsail/...` run 3 consecutive times this pass, all 3 clean, 0 races. `golangci-lint run ./services/lightsail/...` reports 0 issues and `grep -rnE '//nolint:.*(funlen|gocyclo|gocognit|cyclop)' services/lightsail/` is empty -- no banned complexity suppressions anywhere in this package."}
---

## Implementation summary (this pass)

This was a documentation-only re-audit: `services/lightsail/` was fully implemented in commit
`c397a0243` (all 161 ops, real backend state, cli.go registration, resourcegroupstaggingapi wiring)
but this file still read as the from-scratch pre-implementation audit written before that commit,
grading every op "gap." This pass changed nothing in `.go`/`go.mod`/`go.sum`/`cli.go` -- it read the
implementation (`handler.go`, every `handler_*.go`/op-family `.go` file, `persistence.go`,
`sdk_completeness_test.go`, `operations.go`, `errors.go`, and the relevant `cli.go` wiring lines) and
ran `go build`/`go vet`/`go test -race -count=1`/`golangci-lint` against the already-committed tree,
then rewrote this file's frontmatter (`overall`, `families`, `gaps`, `deferred`, `leaks`,
`last_audit_commit`) to match what the code actually does. Everything from "## Purpose of this
document" onward below is the ORIGINAL pre-implementation audit prose, left unmodified as the
wire-shape ground truth the implementation was built from -- its Input/Output/Errors columns are
still an accurate SDK reference; only its per-family "gap" verdicts were stale, which is what the
frontmatter above now corrects.

### What this re-audit verified as genuinely real (not stub echoes)

- **All 161 ops routed and registered**: `GetSupportedOperations()` (`handler.go`) lists exactly 161
  names, `go test ./services/lightsail/...`'s `TestSDKCompleteness` passes with an empty exception
  list against a real `*lightsailsdk.Client{}`, and `cli.go` both registers the handler
  (`lightsailbackend.Provider{}`, `cli.go:3267`) and wires it into `resourcegroupstaggingapi`
  (`wireTaggingLightsail`, `cli.go:7061`) -- unlike mgn's original pass, there was no
  service-registration or cross-service tag-bridge gap left to close here.
- **The Operation model is real** (`operations.go`): every mutating call creates a `Started`
  `Operation` and schedules a genuine timed transition to `Succeeded` via `pkgs/worker`, never a
  synchronously-fabricated terminal status -- exactly the behavior PARITY.md's section 2 calls out
  as the single most under-modelable-if-rushed part of this service.
- **Container services' full state machine walks its real intermediate steps** (`containers.go`):
  `ContainerServiceState`/`StateDetailCode` genuinely progress `PENDING` (with its two sub-codes) ->
  `DEPLOYING` (with its three sub-codes) -> `RUNNING` on real timers, and
  `CurrentDeployment`/`NextDeployment` handoff is real -- not a shortcut straight to `RUNNING`, which
  PARITY.md 4.5 explicitly warned would be "a materially incomplete simulation."
- **The six MetricData ops, plus `GetCostEstimate`/`GetContainerLog`/`GetRelationalDatabaseLogEvents`,
  all genuinely return real, well-formed, EMPTY payloads** after validating the referenced resource
  exists -- verified by reading every one of those handler/backend bodies directly (not inferred
  from doc comments), confirming none of them fabricate plausible-looking telemetry, cost, or log
  content. This is precisely the honest non-fabrication PARITY.md's own section 4.10 asked for.
- **Reference-data catalogs are small, explicitly-labeled SYNTHETIC seed tables**
  (`referencedata.go`), never presented as AWS's real current blueprint/bundle/pricing catalog;
  `GetRegions` is the one part genuinely SDK-derived from `types.RegionName`'s own enum.
- **The four `ResourceType` kinds with no `Tags` field on their own SDK struct** (`StaticIp`,
  `PeeredVpc`, `ExportSnapshotRecord`, `CloudFormationStackRecord`) are resolved by name and then
  HONESTLY REFUSE tagging with a validation error (`tagsNotSupportedKinds`,
  `tagging_vpc_misc.go`), rather than silently no-op'ing or inventing a tag store for them.
- **`InstanceState`/`RelationalDatabaseState` numeric/string conventions are explicitly commented
  UNCONFIRMED** at their own `const` blocks (`consts.go`) -- carried through faithfully from the
  audit's own flagged unknowns, never presented as SDK-confirmed.
- **`Domain`'s ARN genuinely uses the literal region segment `"global"`** (`store.go`'s `globalARN`,
  `domainGlobalRegion` constant), matching the SDK's own doc-comment example exactly, distinct from
  `pkgs/arn.BuildGlobal`'s empty-segment convention -- as the task's own brief anticipated.
- **Gates**: `go build ./services/lightsail/...`, `go vet` (implicit via build), `gofmt` (implicit,
  no lint findings), and `golangci-lint run ./services/lightsail/...` all clean (0 issues). `go test
  ./services/lightsail/...` and `go test -race -count=1 ./services/lightsail/...` both pass, the
  latter run 3 times, all 3 clean (0 races). `grep -rnE
  '//nolint:.*(funlen|gocyclo|gocognit|cyclop)' services/lightsail/` is empty.

### Two real gaps the prior re-audit found -- both addressed by this follow-up pass

1. **`CreateCloudFormationStack`'s cross-service handoff was unreachable -- FIXED.** `exportcfn.go`'s
   own file header calls this "the one confirmed genuine cross-service handoff in this whole 161-op
   surface," and the code path (`b.cfnBackend.CreateStackFromLightsail`, gated on `b.cfnBackend !=
   nil`) was already correctly implemented. But `SetCloudFormationBackend` (`store.go`) had zero call
   sites anywhere in this repo outside its own definition and doc comments -- not in `cli.go`, not in
   this package's own tests -- so `b.cfnBackend` was always `nil` in the running application. This
   pass added `cli.go`'s `cfnLightsailStackAdapter` (adapts the real `services/cloudformation`
   backend's `CreateStack` to the narrow `CreateStackFromLightsail(stackName string, instanceNames
   []string) (string, error)` shape `CloudFormationBackend` expects, with an empty template body --
   Lightsail's export gives instance names, not a CloudFormation template, so fabricating EC2/AMI
   resources would be worse than the honest empty-template stack) and `wireLightsailCloudFormation`.
   The call site is `registerCloudFormationAndDashboard`, not one of the `wire*Integrations` helpers
   `wireCrossServiceDependencies` dispatches to: those all run *before* CloudFormation is registered
   (`cli.go`'s `initializeServices` explicitly registers CloudFormation only after cross-service
   wiring, so its own dashboard can see every other handler), so wiring from there is a silent no-op
   -- confirmed the hard way, by trying it first and watching the throwaway verification test still
   see a `nil` `DestinationInfoID`. Verified end-to-end (not just compiled) via a throwaway
   root-package test, deleted afterward per this task's brief: it drove this repo's real
   `initializeServices`/`wireCrossServiceDependencies`/`serviceByName` path with a real
   `CLI`/`AppContext`, created a real Lightsail instance, snapshot, and export record, called the
   real `CreateCloudFormationStack`, and confirmed the `CloudFormationStackRecord`'s
   `DestinationInfoID` was populated with `State` `SUCCEEDED`, and that the real
   `services/cloudformation` backend's `ListAll()` returned exactly one `Stack` whose `StackName`
   matched. Directly analogous to mgn's own original `SetS3Backend`-never-called gap and its
   dedicated follow-up-pass fix (mgn's PARITY.md, `gopherstack-i6oz` follow-up pass).
2. **5 of 8 wire exception shapes are dead code -- not fixed, now disclosed.** `errors.go`'s
   `classifyLightsailError` correctly maps all 8 Lightsail exception shapes to their HTTP
   status/`__type` string, but
   `errAccessDenied`/`errAccountSetup`/`errOperationFailure`/`errRegionSetup`/`errUnauthenticated` are
   still never actually constructed by any call site in this package (confirmed by grepping each
   identifier across every non-generated `.go` file, tests included). This pass checked each of the
   five against the real SDK's own doc comments
   (`aws-sdk-go-v2/service/lightsail@v1.58.3/types/errors.go`) for an unambiguous correct call site
   and found none: `AccessDeniedException`/`UnauthenticatedException` both require a caller-identity/
   permission model this backend, like directconnect/mgn, does not simulate;
   `AccountSetupInProgressException`/`RegionSetupInProgressException` both require an account/region
   provisioning-state model (something like mgn's `InitializeService`) this package has no equivalent
   of; `OperationFailureException`'s own doc comment ("Lightsail throws this exception when an
   operation fails to execute") names no specific operation or precondition to hang a real trigger
   off. Wiring any of the five would mean inventing a permission or provisioning-state model this
   backend does not otherwise have, purely to exercise an otherwise-unused constructor -- exactly the
   fabrication `parity-principles.md` forbids, so none were wired. What this pass DID fix: `errors.go`
   itself now carries a disclosure comment naming all five and explaining why each is unwired,
   mirroring `mgn/errors.go`'s identical disclosure of its own unused
   `errAccessDenied`/`errQuotaExceeded`/`errThrottling` -- the specific thing this package was
   previously docked for not doing relative to mgn's otherwise-identical situation. This emulator's
   real, observable error surface for every one of the 161 ops therefore remains
   `{InvalidInputException, NotFoundException, ServiceException}`, regardless of what richer per-op
   signature the family tables below (preserved from the original audit) list as AWS's real behavior.

Finding 1 was the load-bearing wire-reachability gap keeping this package off **A** -- with it closed
and verified end-to-end, and finding 2 now disclosed rather than silently missing (matching mgn's own
A-calibrated posture, where an identical disclosed-and-legitimate gap does not itself cost a grade),
this package moves from **A-** to **A**. Neither finding was ever fabrication, and finding 2 still
isn't: no wrong response is ever returned, only an honestly narrower error surface than the real AWS
API's, clearly disclosed at both the `errors.go` call site and here.

## Purpose of this document

`services/lightsail/` does not exist. This file is a pre-implementation audit: a complete SDK
operation inventory plus a behavioral spec, written so a follow-up implementation pass does not
have to re-derive wire shapes from the SDK source itself. No `.go` files were touched to produce
it. All 161 operation names, the wire protocol, every operation's exact per-op exception set, and
every shared type/enum below were read directly from
`aws-sdk-go-v2/service/lightsail@v1.58.3`'s `serializers.go` / `deserializers.go` / `types/types.go`
/ `types/enums.go` / `types/errors.go` / individual `api_op_*.go` files in the module cache
(resolved via a throwaway `go mod init probe && go get .../lightsail@latest` in the scratch dir --
**not** added to this repo's `go.mod`, which another agent was concurrently editing during this
pass).

Lightsail is AWS's simplified-VPS product: a broad surface of loosely-coupled resource families
(instances, block storage, managed MySQL databases, load balancers, a CDN, an S3-like bucket
product, a container-orchestration sub-product, DNS, alerting) each with their own small
CRUD+lifecycle op set, all funneled through a single async `Operation` polling model. At 161 ops
this is the largest of the seven services flagged by the UI audit as pointing at nonexistent
backends.

## 1. Complete SDK operation inventory

**161 operations**, SDK version **`v1.58.3`** (resolved 2026-08-01, whatever `@latest` currently
resolves to -- not a version pinned by this audit). This matches the task's ~161 estimate exactly:

`ls api_op_*.go | grep -v _test.go | wc -l` against
`/home/agbishop/go/pkg/mod/github.com/aws/aws-sdk-go-v2/service/lightsail@v1.58.3/` returns **161**.

### Protocol and routing shape

Protocol is **`awsjson1.1`** (`awsAwsjson11_serializeOp<Op>` struct names throughout
`serializers.go`), confirming this task's brief expectation rather than directconnect's REST-XML
or networkmanager's REST-JSON. Every single one of the 161 ops:

- is a **`POST /`** request (`request.Request.Method = "POST"`, and the URL path is always exactly
  `/` -- confirmed via `path.Join(request.Request.URL.Path, "/")` logic identical across all 161
  serializer functions, not sampled),
- carries an **`X-Amz-Target: Lightsail_20161128.<OpName>`** header (`grep -c "X-Amz-Target"
  serializers.go` => 161; `grep -o '"Lightsail_[0-9]*\.' serializers.go | sort -u` => exactly one
  distinct prefix, `Lightsail_20161128.`), and
- carries a JSON body encoded via `smithyjson.NewEncoder()`.

This means a router for this service dispatches PURELY on the `X-Amz-Target` header value, not on
HTTP method or URL path -- the same dispatch shape as mgn (`AWSElasticDisasterRecoveryService.` or
similar prefix, per that audit) and directconnect (`OvertureService.` prefix), and unlike
networkmanager's genuine REST path/verb routing. There is no REST path parameter extraction to
implement for Lightsail at all.

### Errors -- 8 shared exception shapes, all read directly from `types/errors.go`

All 8 shapes share the **exact same field set**: `{Message *string, ErrorCodeOverride *string, Code
*string, Docs *string, Tip *string}` -- a uniform Lightsail-specific error envelope (the `Code`/
`Docs`/`Tip` triad is not seen in any prior audit this campaign; it appears to be a Lightsail
console-friendliness convention, giving callers a doc URL and a remediation tip alongside the
error). Only `ServiceException` is a server fault (`smithy.FaultServer`); the other 7 are client
faults.

- **`AccessDeniedException`** -- client fault.
- **`AccountSetupInProgressException`** -- client fault. Lightsail-specific: the caller's account
  is still being provisioned for Lightsail use in this region/globally.
- **`InvalidInputException`** -- client fault. The general-purpose validation-failure shape (no
  `Reason`/`Fields` structured breakdown unlike networkmanager's `ValidationException` -- just
  `Message`).
- **`NotFoundException`** -- client fault.
- **`OperationFailureException`** -- client fault. Distinct from a request being rejected outright:
  this fires when the ASYNC operation itself (see the Operation model, below) failed after being
  accepted -- i.e. it's surfaced synchronously for what is conceptually an async failure in some
  code paths.
- **`RegionSetupInProgressException`** -- client fault. Same shape as `AccountSetupInProgressException`
  but scoped to a specific region rather than the whole account.
- **`ServiceException`** -- **server fault** (the only one of the 8).
- **`UnauthenticatedException`** -- client fault.

**Per-op sets, extracted from every op's own `awsAwsjson11_deserializeOpError<Op>` switch in
`deserializers.go` (all 161 read individually via a Python regex pass over the whole file, not
sampled)**: the true baseline across ALL 161 ops is only 3 shapes
(`AccessDeniedException`+`ServiceException`+`UnauthenticatedException`) -- **`InvalidInputException`
is present on 160 of 161 ops**, missing from exactly one:

- **`GetContainerAPIMetadata` is the sole op with ZERO input fields** (`GetContainerAPIMetadataInput`
  has no members at all) **and correspondingly lacks `InvalidInputException`** in its error switch
  (nothing to validate) while still carrying `RegionSetupInProgressException`. Its output is also
  the least-typed response in the whole surface: `Metadata []map[string]string` -- a free-form list
  of string maps, not even a named struct field per entry.

Beyond that near-universal `{AccessDenied, InvalidInput, Service, Unauthenticated}` "base4", the 161
ops fall into exactly **7 distinct error-set signatures** depending on 4 additional shapes in
combination (`AccountSetupInProgressException`, `NotFoundException`, `OperationFailureException`,
`RegionSetupInProgressException`):

| Signature (beyond base4) | Op count | Example ops |
|---|---|---|
| +AcctSetup +NotFound +OpFailure +RegionSetup | 103 | Nearly every Instance/Disk/StaticIp/KeyPair/RelationalDatabase/LoadBalancer/Domain/VpcPeering/Tag/Operation op |
| +NotFound +RegionSetup (no AcctSetup, no OpFailure) | 31 | Bucket, Certificate, ContainerService reads/writes, GUI sessions |
| +NotFound +OpFailure +RegionSetup (no AcctSetup) | 12 | Alarms, contact methods |
| +NotFound +OpFailure (no AcctSetup, no RegionSetup) | 11 | All Distribution-family ops |
| +RegionSetup only | 2 | CreateBucket, GetBucketBundles |
| +RegionSetup only, no InvalidInput | 1 | GetContainerAPIMetadata (see above) |
| +AcctSetup +RegionSetup (no NotFound, no OpFailure) | 1 | GetLoadBalancerTlsPolicies |

This 7-signature breakdown was computed programmatically (Python `set()` grouping over all 161
per-op error lists), not eyeballed, and re-verified to sum to 161 (103+31+12+11+2+1+1=161) before
writing this document.

## 2. The Operation model -- central to this entire service

Nearly every mutating op returns one or more `Operation` records (`types/types.go`):

```
type Operation struct {
    CreatedAt        *time.Time
    ErrorCode        *string
    ErrorDetails     *string
    Id               *string
    IsTerminal       *bool
    Location         *ResourceLocation   // {AvailabilityZone *string, RegionName RegionName}
    OperationDetails *string
    OperationType    OperationType       // 84 real values, one per major mutating op name
    ResourceName     *string
    ResourceType     ResourceType        // 20 values, see Cross-service wiring
    Status           OperationStatus     // 5 values
    StatusChangedAt  *time.Time
}
```

`OperationStatus` has exactly **5 values**: `NotStarted`, `Started`, `Failed`, `Completed`,
`Succeeded` (`types/enums.go`). `OperationType` has exactly **84 values** (`types/enums.go`), each
named identically to a mutating op (`CreateInstance` -- singular, even though the actual op is
`CreateInstances` plural; `DeleteKnownHostKeys`; `SetupInstanceHttps`; etc.) -- confirming that
every batch-style mutating op (e.g. `CreateInstances` creating N instances) still emits one
`Operation` per affected resource, each tagged with the SAME `OperationType` value.

Callers poll operation status via family Z (`GetOperation`/`GetOperations`/
`GetOperationsForResource`, 3 ops). An honest implementation must:

1. Return a real `Operation` record (with a real `Id`, a real initial `Status` of `Started` or
   `NotStarted`) from every mutating op, not a stub with a fabricated ID that is never queryable
   afterward.
2. Advance `Status` toward `Succeeded`/`Completed` (both appear in the enum; this audit could not
   determine from the SDK alone which of the two AWS actually uses for which op family, since
   `OperationStatus.Values()` lists both with no per-op mapping documented in this module -- an
   honest unknown, not guessed) via the same timer-driven auto-advance pattern already used
   elsewhere in this tree (services/eks's `scheduleClusterActivation`, services/grafana's
   analogous pattern, both via `pkgs/worker`).
3. Set `IsTerminal true` once `Status` reaches a terminal value, and populate `ErrorCode`/
   `ErrorDetails` on failure paths rather than leaving them nil forever.

## 3. Family tables -- every one of the 161 operations

All 161 ops share the identical `POST /` + `X-Amz-Target: Lightsail_20161128.<Op>` wire shape
described above, so the per-op tables below omit a redundant method/path column and instead show
Input / Output / error-set delta from the near-universal base4 (`AccessDeniedException`+
`InvalidInputException`+`ServiceException`+`UnauthenticatedException`; see Errors section above for
the one exception, `GetContainerAPIMetadata`, which lacks `InvalidInputException`). Field lists come
directly from each op's `api_op_<Op>.go` Input/Output struct (`ResultMetadata middleware.Metadata`
omitted as boilerplate); types shown in parens are non-primitive (structs, enums, `*time.Time`,
slices of non-string) -- bare names with no parens are `*string`/`[]string`/`*bool`/`*int32`/
`*int64`/`*float32`/`*float64`. Error-delta tags: `+AcctSetup`=`AccountSetupInProgressException`,
`+NotFound`=`NotFoundException`, `+OpFailure`=`OperationFailureException`,
`+RegionSetup`=`RegionSetupInProgressException`.

Family letters A-BB below were assigned by this audit for readability; they group op names
alphabetically-adjacent operations from `ls api_op_*.go` into 28 coherent families with **zero
overlaps and zero omissions**, re-verified programmatically (161 assigned, 0 missing, 0 duplicated)
before writing this document.

### A. Static reference data (blueprints, bundles, regions, powers) (9 ops)

These 9 ops return AWS-curated catalog data (available OS/app blueprints, instance/db/bucket/distribution bundle tiers, region list, container power tiers) that this emulator cannot derive from any user action -- it must SEED static tables, not compute them.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| GetBlueprints | AppCategory(AppCategory), IncludeInactive, PageToken | Blueprints([]Blueprint), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetBundles | AppCategory(AppCategory), IncludeInactive, PageToken | Bundles([]Bundle), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabaseBlueprints | PageToken | Blueprints([]RelationalDatabaseBlueprint), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabaseBundles | IncludeInactive, PageToken | Bundles([]RelationalDatabaseBundle), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetBucketBundles | IncludeInactive | Bundles([]BucketBundle) | +RegionSetup |
| GetDistributionBundles | (none) | Bundles([]DistributionBundle) | +NotFound +OpFailure |
| GetContainerServicePowers | (none) | Powers([]ContainerServicePower) | +NotFound +RegionSetup |
| GetRegions | IncludeAvailabilityZones, IncludeRelationalDatabaseAvailabilityZones | Regions([]Region) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetContainerAPIMetadata | (none) | Metadata([]map[string]string) | NO InvalidInput +RegionSetup |

### B. Instances -- core lifecycle (9 ops)

CreateInstances/CreateInstancesFromSnapshot are plural (batch) ops -- one call can create N named instances, returning one Operation per instance. GetInstanceState returns the free-form InstanceState{Code *int32, Name *string} struct (see Missing simulated functionality: no typed enum exists in this SDK for instance state).

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateInstances | AvailabilityZone, BlueprintId, BundleId, InstanceNames, AddOns([]AddOnRequest), CustomImageName, IpAddressType(IpAddressType), KeyPairName, Tags([]Tag), UserData | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| CreateInstancesFromSnapshot | AvailabilityZone, BundleId, InstanceNames, AddOns([]AddOnRequest), AttachedDiskMapping(map[string][]DiskMap), InstanceSnapshotName, IpAddressType(IpAddressType), KeyPairName, RestoreDate, SourceInstanceName, Tags([]Tag), UseLatestRestorableAutoSnapshot, UserData | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteInstance | InstanceName, ForceDeleteAddOns | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetInstance | InstanceName | Instance(*Instance) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetInstances | PageToken | Instances([]Instance), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetInstanceState | InstanceName | State(*InstanceState) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| RebootInstance | InstanceName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| StartInstance | InstanceName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| StopInstance | InstanceName, Force | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### C. Instance access, firewall ports, HTTPS setup (8 ops)

PutInstancePublicPorts replaces the entire port-rule set in one call; Open/Close are additive/subtractive single-rule variants. SetupInstanceHttps/GetSetupHistory model Bitnami HTTPS auto-configuration (Let's Encrypt certs on supported app blueprints) as an audited history, not just a boolean flag.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| GetInstanceAccessDetails | InstanceName, Protocol(InstanceAccessProtocol) | AccessDetails(*InstanceAccessDetails) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetInstancePortStates | InstanceName | PortStates([]InstancePortState) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| OpenInstancePublicPorts | InstanceName, PortInfo(*PortInfo) | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| CloseInstancePublicPorts | InstanceName, PortInfo(*PortInfo) | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| PutInstancePublicPorts | InstanceName, PortInfos([]PortInfo) | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| SetupInstanceHttps | CertificateProvider(CertificateProvider), DomainNames, EmailAddress, InstanceName | Operations([]Operation) | +NotFound +RegionSetup |
| GetSetupHistory | ResourceName, PageToken | NextPageToken, SetupHistory([]SetupHistory) | +NotFound +RegionSetup |
| DeleteKnownHostKeys | InstanceName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### D. Instance (system-volume) snapshots (4 ops)

A snapshot captures the instance's attached disks (InstanceSnapshot.FromAttachedDisks) at a point in time; restored via CreateInstancesFromSnapshot in family B.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateInstanceSnapshot | InstanceName, InstanceSnapshotName, Tags([]Tag) | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteInstanceSnapshot | InstanceSnapshotName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetInstanceSnapshot | InstanceSnapshotName | InstanceSnapshot(*InstanceSnapshot) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetInstanceSnapshots | PageToken | InstanceSnapshots([]InstanceSnapshot), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |

### E. Instance CloudWatch-style metrics and IMDS options (2 ops)

GetInstanceMetricData is one of the six honestly-unfakeable telemetry ops flagged below. UpdateInstanceMetadataOptions mirrors EC2's IMDSv2 token/hop-limit knobs (HttpTokens/HttpEndpoint/HttpProtocolIpv6/HttpPutResponseHopLimit).

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| GetInstanceMetricData | EndTime(*time.Time), InstanceName, MetricName(InstanceMetricName), Period, StartTime(*time.Time), Statistics([]MetricStatistic), Unit(MetricUnit) | MetricData([]MetricDatapoint), MetricName(InstanceMetricName) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| UpdateInstanceMetadataOptions | InstanceName, HttpEndpoint(HttpEndpoint), HttpProtocolIpv6(HttpProtocolIpv6), HttpPutResponseHopLimit, HttpTokens(HttpTokens) | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### F. Add-ons (AutoSnapshot, StopInstanceOnIdle) and their snapshot history (4 ops)

AddOnType has exactly 2 values (AutoSnapshot, StopInstanceOnIdle); GetAutoSnapshots/DeleteAutoSnapshot manage the automatic daily-snapshot history the AutoSnapshot add-on produces for an instance or disk.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| GetAutoSnapshots | ResourceName | AutoSnapshots([]AutoSnapshotDetails), ResourceName, ResourceType(ResourceType) | +NotFound +OpFailure +RegionSetup |
| DeleteAutoSnapshot | Date, ResourceName | Operations([]Operation) | +NotFound +OpFailure +RegionSetup |
| EnableAddOn | AddOnRequest(*AddOnRequest), ResourceName | Operations([]Operation) | +NotFound +OpFailure +RegionSetup |
| DisableAddOn | AddOnType(AddOnType), ResourceName | Operations([]Operation) | +NotFound +OpFailure +RegionSetup |

### G. SSH/RDP key pairs (6 ops)

CreateKeyPair returns the private key exactly once (PrivateKeyBase64/PublicKeyBase64 on the response, never retrievable again -- GetKeyPair/GetKeyPairs only ever return the public KeyPair metadata). DownloadDefaultKeyPair is a singleton -- one shared default key pair per account/region, no name parameter at all.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateKeyPair | KeyPairName, Tags([]Tag) | KeyPair(*KeyPair), Operation(*Operation), PrivateKeyBase64, PublicKeyBase64 | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteKeyPair | KeyPairName, ExpectedFingerprint | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DownloadDefaultKeyPair | (none) | CreatedAt(*time.Time), PrivateKeyBase64, PublicKeyBase64 | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetKeyPair | KeyPairName | KeyPair(*KeyPair) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetKeyPairs | IncludeDefaultKeyPair, PageToken | KeyPairs([]KeyPair), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| ImportKeyPair | KeyPairName, PublicKeyBase64 | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### H. Static IPs (6 ops)

A simplified analogue of an EC2 Elastic IP, scoped to attach/detach against a named Lightsail instance rather than any ENI.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| AllocateStaticIp | StaticIpName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| AttachStaticIp | InstanceName, StaticIpName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DetachStaticIp | StaticIpName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetStaticIp | StaticIpName | StaticIp(*StaticIp) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetStaticIps | PageToken | NextPageToken, StaticIps([]StaticIp) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| ReleaseStaticIp | StaticIpName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### I. Block-storage disks (attach/detach) (7 ops)

AttachDisk takes a DiskPath (the in-guest device path, e.g. /dev/xvdf) plus an AutoMounting bool -- Lightsail can manage guest-OS mounting itself via its agent, a real behavior this emulator cannot honestly perform (see Missing simulated functionality).

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| AttachDisk | DiskName, DiskPath, InstanceName, AutoMounting | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DetachDisk | DiskName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| CreateDisk | AvailabilityZone, DiskName, SizeInGb, AddOns([]AddOnRequest), Tags([]Tag) | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| CreateDiskFromSnapshot | AvailabilityZone, DiskName, SizeInGb, AddOns([]AddOnRequest), DiskSnapshotName, RestoreDate, SourceDiskName, Tags([]Tag), UseLatestRestorableAutoSnapshot | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteDisk | DiskName, ForceDeleteAddOns | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetDisk | DiskName | Disk(*Disk) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetDisks | PageToken | Disks([]Disk), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |

### J. Disk snapshots (+ cross-region CopySnapshot) (5 ops)

CopySnapshot is shared by both disk and instance snapshots (SourceResourceName is polymorphic) and is the only op in this whole surface with a SourceRegion(RegionName) parameter, i.e. the only explicitly cross-region op in the entire API.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateDiskSnapshot | DiskSnapshotName, DiskName, InstanceName, Tags([]Tag) | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteDiskSnapshot | DiskSnapshotName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetDiskSnapshot | DiskSnapshotName | DiskSnapshot(*DiskSnapshot) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetDiskSnapshots | PageToken | DiskSnapshots([]DiskSnapshot), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| CopySnapshot | SourceRegion(RegionName), TargetSnapshotName, RestoreDate, SourceResourceName, SourceSnapshotName, UseLatestRestorableAutoSnapshot | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### K. Export-to-EC2 and CloudFormation stack import (4 ops)

ExportSnapshot converts a Lightsail instance/disk snapshot into an EC2-compatible export record; CreateCloudFormationStack then launches a REAL CloudFormation stack (in the caller's own standard EC2 account, not Lightsail) from that exported snapshot -- a genuine Lightsail-to-EC2 migration path, not decorative.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| ExportSnapshot | SourceSnapshotName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetExportSnapshotRecords | PageToken | ExportSnapshotRecords([]ExportSnapshotRecord), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| CreateCloudFormationStack | Instances([]InstanceEntry) | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetCloudFormationStackRecords | PageToken | CloudFormationStackRecords([]CloudFormationStackRecord), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |

### L. Load balancers (9 ops)

Lightsail load balancers are a simplified single-listener ALB analogue (LoadBalancerProtocol has exactly 2 values: HTTP or HTTP_HTTPS) fronting a named set of instances; SetIpAddressType is shared across Instance/LoadBalancer/Distribution via the ResourceType(ResourceType) discriminator in its input.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateLoadBalancer | InstancePort(int32), LoadBalancerName, CertificateAlternativeNames, CertificateDomainName, CertificateName, HealthCheckPath, IpAddressType(IpAddressType), Tags([]Tag), TlsPolicyName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteLoadBalancer | LoadBalancerName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetLoadBalancer | LoadBalancerName | LoadBalancer(*LoadBalancer) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetLoadBalancers | PageToken | LoadBalancers([]LoadBalancer), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| AttachInstancesToLoadBalancer | InstanceNames, LoadBalancerName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DetachInstancesFromLoadBalancer | InstanceNames, LoadBalancerName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| UpdateLoadBalancerAttribute | AttributeName(LoadBalancerAttributeName), AttributeValue, LoadBalancerName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| SetIpAddressType | IpAddressType(IpAddressType), ResourceName, ResourceType(ResourceType), AcceptBundleUpdate | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetLoadBalancerMetricData | EndTime(*time.Time), LoadBalancerName, MetricName(LoadBalancerMetricName), Period, StartTime(*time.Time), Statistics([]MetricStatistic), Unit(MetricUnit) | MetricData([]MetricDatapoint), MetricName(LoadBalancerMetricName) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### M. Load-balancer TLS certificates (5 ops)

Distinct from the CDN-facing family (certificates, below) and from ACM -- this is Lightsail's own LB-scoped certificate object, created/attached/deleted per load balancer, with its own GetLoadBalancerTlsPolicies (cipher/protocol policy catalog, static reference data).

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateLoadBalancerTlsCertificate | CertificateDomainName, CertificateName, LoadBalancerName, CertificateAlternativeNames, Tags([]Tag) | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteLoadBalancerTlsCertificate | CertificateName, LoadBalancerName, Force | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| AttachLoadBalancerTlsCertificate | CertificateName, LoadBalancerName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetLoadBalancerTlsCertificates | LoadBalancerName | TlsCertificates([]LoadBalancerTlsCertificate) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetLoadBalancerTlsPolicies | PageToken | NextPageToken, TlsPolicies([]LoadBalancerTlsPolicy) | +AcctSetup +RegionSetup |

### N. Managed relational databases -- core lifecycle (9 ops)

RelationalDatabaseEngine's only SDK-known enum value is "mysql" (types/enums.go) even though the Engine field on the RelationalDatabase resource itself is a plain *string, not the typed enum (types/types.go) -- see Missing simulated functionality for why this cannot be assumed to mean Postgres is unsupported.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateRelationalDatabase | MasterDatabaseName, MasterUsername, RelationalDatabaseBlueprintId, RelationalDatabaseBundleId, RelationalDatabaseName, AvailabilityZone, MasterUserPassword, PreferredBackupWindow, PreferredMaintenanceWindow, PubliclyAccessible, Tags([]Tag) | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| CreateRelationalDatabaseFromSnapshot | RelationalDatabaseName, AvailabilityZone, PubliclyAccessible, RelationalDatabaseBundleId, RelationalDatabaseSnapshotName, RestoreTime(*time.Time), SourceRelationalDatabaseName, Tags([]Tag), UseLatestRestorableTime | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteRelationalDatabase | RelationalDatabaseName, FinalRelationalDatabaseSnapshotName, SkipFinalSnapshot | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabase | RelationalDatabaseName | RelationalDatabase(*RelationalDatabase) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabases | PageToken | NextPageToken, RelationalDatabases([]RelationalDatabase) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| StartRelationalDatabase | RelationalDatabaseName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| StopRelationalDatabase | RelationalDatabaseName, RelationalDatabaseSnapshotName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| RebootRelationalDatabase | RelationalDatabaseName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| UpdateRelationalDatabase | RelationalDatabaseName, ApplyImmediately, CaCertificateIdentifier, DisableBackupRetention, EnableBackupRetention, MasterUserPassword, PreferredBackupWindow, PreferredMaintenanceWindow, PubliclyAccessible, RelationalDatabaseBlueprintId, RotateMasterUserPassword | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### O. Managed relational databases -- events, logs, parameters, password (7 ops)

GetRelationalDatabaseMasterUserPassword decrypts and returns the actual master password (CURRENT/PREVIOUS/PENDING versions) -- a real secret-retrieval op, not a metadata read.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| GetRelationalDatabaseEvents | RelationalDatabaseName, DurationInMinutes, PageToken | NextPageToken, RelationalDatabaseEvents([]RelationalDatabaseEvent) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabaseLogEvents | LogStreamName, RelationalDatabaseName, EndTime(*time.Time), PageToken, StartFromHead, StartTime(*time.Time) | NextBackwardToken, NextForwardToken, ResourceLogEvents([]LogEvent) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabaseLogStreams | RelationalDatabaseName | LogStreams | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabaseMasterUserPassword | RelationalDatabaseName, PasswordVersion(RelationalDatabasePasswordVersion) | CreatedAt(*time.Time), MasterUserPassword | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabaseMetricData | EndTime(*time.Time), MetricName(RelationalDatabaseMetricName), Period, RelationalDatabaseName, StartTime(*time.Time), Statistics([]MetricStatistic), Unit(MetricUnit) | MetricData([]MetricDatapoint), MetricName(RelationalDatabaseMetricName) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabaseParameters | RelationalDatabaseName, PageToken | NextPageToken, Parameters([]RelationalDatabaseParameter) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| UpdateRelationalDatabaseParameters | Parameters([]RelationalDatabaseParameter), RelationalDatabaseName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### P. Managed relational database snapshots (4 ops)

Symmetric with family D/J; restored via CreateRelationalDatabaseFromSnapshot in family N.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateRelationalDatabaseSnapshot | RelationalDatabaseName, RelationalDatabaseSnapshotName, Tags([]Tag) | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteRelationalDatabaseSnapshot | RelationalDatabaseSnapshotName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabaseSnapshot | RelationalDatabaseSnapshotName | RelationalDatabaseSnapshot(*RelationalDatabaseSnapshot) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetRelationalDatabaseSnapshots | PageToken | NextPageToken, RelationalDatabaseSnapshots([]RelationalDatabaseSnapshot) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### Q. Container services -- service lifecycle (5 ops)

ContainerServiceState has 7 values (PENDING/READY/RUNNING/UPDATING/DELETING/DISABLED/DEPLOYING) with a companion ContainerServiceStateDetailCode (9 values: CREATING_SYSTEM_RESOURCES, CREATING_NETWORK_INFRASTRUCTURE, PROVISIONING_CERTIFICATE, PROVISIONING_SERVICE, CREATING_DEPLOYMENT, EVALUATING_HEALTH_CHECK, ACTIVATING_DEPLOYMENT, CERTIFICATE_LIMIT_EXCEEDED, UNKNOWN_ERROR) populated only during PENDING/DEPLOYING/UPDATING -- see Missing simulated functionality, this is effectively its own mini-orchestrator state machine.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateContainerService | Power(ContainerServicePowerName), Scale, ServiceName, Deployment(*ContainerServiceDeploymentRequest), PrivateRegistryAccess(*PrivateRegistryAccessRequest), PublicDomainNames(map[string][]string), Tags([]Tag) | ContainerService(*ContainerService) | +NotFound +RegionSetup |
| UpdateContainerService | ServiceName, IsDisabled, Power(ContainerServicePowerName), PrivateRegistryAccess(*PrivateRegistryAccessRequest), PublicDomainNames(map[string][]string), Scale | ContainerService(*ContainerService) | +NotFound +RegionSetup |
| DeleteContainerService | ServiceName | (none) | +NotFound +RegionSetup |
| GetContainerServices | ServiceName | ContainerServices([]ContainerService) | +NotFound +RegionSetup |
| GetContainerServiceMetricData | EndTime(*time.Time), MetricName(ContainerServiceMetricName), Period, ServiceName, StartTime(*time.Time), Statistics([]MetricStatistic) | MetricData([]MetricDatapoint), MetricName(ContainerServiceMetricName) | +NotFound +RegionSetup |

### R. Container services -- deployments, images, registry, logs (7 ops)

A ContainerService holds CurrentDeployment + NextDeployment (ContainerServiceDeploymentState: ACTIVATING/ACTIVE/INACTIVE/FAILED); RegisterContainerImage stores an already-pushed image under a Lightsail-managed private registry-style Label/Digest pair referenced later from Container.Image with a leading ':' (e.g. :container-service-1.mystaticsite.3).

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateContainerServiceDeployment | ServiceName, Containers(map[string]Container), PublicEndpoint(*EndpointRequest) | ContainerService(*ContainerService) | +NotFound +RegionSetup |
| GetContainerServiceDeployments | ServiceName | Deployments([]ContainerServiceDeployment) | +NotFound +RegionSetup |
| CreateContainerServiceRegistryLogin | (none) | RegistryLogin(*ContainerServiceRegistryLogin) | +NotFound +RegionSetup |
| RegisterContainerImage | Digest, Label, ServiceName | ContainerImage(*ContainerImage) | +NotFound +RegionSetup |
| GetContainerImages | ServiceName | ContainerImages([]ContainerImage) | +NotFound +RegionSetup |
| DeleteContainerImage | Image, ServiceName | (none) | +NotFound +RegionSetup |
| GetContainerLog | ContainerName, ServiceName, EndTime(*time.Time), FilterPattern, PageToken, StartTime(*time.Time) | LogEvents([]ContainerServiceLogEvent), NextPageToken | +NotFound +RegionSetup |

### S. Buckets (Lightsail's own object storage, distinct from S3) (10 ops)

Bucket.ResourceType is a plain *string (unlike almost every other resource here, which uses the typed ResourceType enum) -- see wire-shape notes. AccessKey.SecretAccessKey is returned in full on CreateBucketAccessKey (never retrievable again, same pattern as CreateKeyPair).

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateBucket | BucketName, BundleId, EnableObjectVersioning, Tags([]Tag) | Bucket(*Bucket), Operations([]Operation) | +RegionSetup |
| DeleteBucket | BucketName, ForceDelete | Operations([]Operation) | +NotFound +RegionSetup |
| UpdateBucket | BucketName, AccessLogConfig(*BucketAccessLogConfig), AccessRules(*AccessRules), Cors(*BucketCorsConfig), ReadonlyAccessAccounts, Versioning | Bucket(*Bucket), Operations([]Operation) | +NotFound +RegionSetup |
| UpdateBucketBundle | BucketName, BundleId | Operations([]Operation) | +NotFound +RegionSetup |
| GetBuckets | BucketName, IncludeConnectedResources, IncludeCors, PageToken | AccountLevelBpaSync(*AccountLevelBpaSync), Buckets([]Bucket), NextPageToken | +NotFound +RegionSetup |
| SetResourceAccessForBucket | Access(ResourceBucketAccess), BucketName, ResourceName | Operations([]Operation) | +NotFound +RegionSetup |
| GetBucketMetricData | BucketName, EndTime(*time.Time), MetricName(BucketMetricName), Period, StartTime(*time.Time), Statistics([]MetricStatistic), Unit(MetricUnit) | MetricData([]MetricDatapoint), MetricName(BucketMetricName) | +NotFound +RegionSetup |
| CreateBucketAccessKey | BucketName | AccessKey(*AccessKey), Operations([]Operation) | +NotFound +RegionSetup |
| DeleteBucketAccessKey | AccessKeyId, BucketName | Operations([]Operation) | +NotFound +RegionSetup |
| GetBucketAccessKeys | BucketName | AccessKeys([]AccessKey) | +NotFound +RegionSetup |

### T. Distributions -- Lightsail's simplified CDN (CloudFront-backed) (10 ops)

LightsailDistribution.Location's own doc comment states distributions are billed/modeled as global but 'all distributions are located in the us-east-1 Region' (types/types.go) -- a real, quotable AWS behavior, not an assumption.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateDistribution | BundleId, DefaultCacheBehavior(*CacheBehavior), DistributionName, Origin(*InputOrigin), CacheBehaviorSettings(*CacheSettings), CacheBehaviors([]CacheBehaviorPerPath), CertificateName, IpAddressType(IpAddressType), Tags([]Tag), ViewerMinimumTlsProtocolVersion(ViewerMinimumTlsProtocolVersionEnum) | Distribution(*LightsailDistribution), Operation(*Operation) | +NotFound +OpFailure |
| UpdateDistribution | DistributionName, CacheBehaviorSettings(*CacheSettings), CacheBehaviors([]CacheBehaviorPerPath), CertificateName, DefaultCacheBehavior(*CacheBehavior), IsEnabled, Origin(*InputOrigin), UseDefaultCertificate, ViewerMinimumTlsProtocolVersion(ViewerMinimumTlsProtocolVersionEnum) | Operation(*Operation) | +NotFound +OpFailure |
| DeleteDistribution | DistributionName | Operation(*Operation) | +NotFound +OpFailure |
| GetDistributions | DistributionName, PageToken | Distributions([]LightsailDistribution), NextPageToken | +NotFound +OpFailure |
| UpdateDistributionBundle | BundleId, DistributionName | Operation(*Operation) | +NotFound +OpFailure |
| ResetDistributionCache | DistributionName | CreateTime(*time.Time), Operation(*Operation), Status | +NotFound +OpFailure |
| GetDistributionLatestCacheReset | DistributionName | CreateTime(*time.Time), Status | +NotFound +OpFailure |
| GetDistributionMetricData | DistributionName, EndTime(*time.Time), MetricName(DistributionMetricName), Period, StartTime(*time.Time), Statistics([]MetricStatistic), Unit(MetricUnit) | MetricData([]MetricDatapoint), MetricName(DistributionMetricName) | +NotFound +OpFailure |
| AttachCertificateToDistribution | CertificateName, DistributionName | Operation(*Operation) | +NotFound +OpFailure |
| DetachCertificateFromDistribution | DistributionName | Operation(*Operation) | +NotFound +OpFailure |

### U. Domains and DNS records (7 ops)

DomainEntry.Type is a plain *string (A/AAAA/CNAME/MX/NS/SOA/SRV/TXT listed only in the doc comment, not a typed enum) -- Domain.Arn's own doc comment gives the literal example arn:aws:lightsail:global:123456789101:Domain/824cede0-... , confirming Domain is a GLOBAL resource (region segment literally the word "global"), unlike Instance/Disk/etc. which are regional (see Cross-service wiring).

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateDomain | DomainName, Tags([]Tag) | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteDomain | DomainName | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetDomain | DomainName | Domain(*Domain) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetDomains | PageToken | Domains([]Domain), NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| CreateDomainEntry | DomainEntry(*DomainEntry), DomainName | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| DeleteDomainEntry | DomainEntry(*DomainEntry), DomainName | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| UpdateDomainEntry | DomainEntry(*DomainEntry), DomainName | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### V. Certificates (CDN/distribution-facing, ACM-like but Lightsail-native) (3 ops)

Distinct from the LB-TLS-certificate family (M) -- these are used with CreateDistribution's CertificateName / AttachCertificateToDistribution, issued by Lightsail's own CA integration (CertificateProvider has exactly 1 value: LetsEncrypt).

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateCertificate | CertificateName, DomainName, SubjectAlternativeNames, Tags([]Tag) | Certificate(*CertificateSummary), Operations([]Operation) | +NotFound +RegionSetup |
| DeleteCertificate | CertificateName | Operations([]Operation) | +NotFound +RegionSetup |
| GetCertificates | CertificateName, CertificateStatuses([]CertificateStatus), IncludeCertificateDetails(bool), PageToken | Certificates([]CertificateSummary), NextPageToken | +NotFound +RegionSetup |

### W. CloudWatch-style alarms and contact methods (SNS/SMS/Email analogue) (8 ops)

ContactProtocol has exactly 2 values (Email, SMS); PutAlarm's NotificationTriggers([]AlarmState) lets a caller choose exactly which of OK/ALARM/INSUFFICIENT_DATA transitions notify, mirroring real CloudWatch alarm semantics.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| PutAlarm | AlarmName, ComparisonOperator(ComparisonOperator), EvaluationPeriods, MetricName(MetricName), MonitoredResourceName, Threshold, ContactProtocols([]ContactProtocol), DatapointsToAlarm, NotificationEnabled, NotificationTriggers([]AlarmState), Tags([]Tag), TreatMissingData(TreatMissingData) | Operations([]Operation) | +NotFound +OpFailure +RegionSetup |
| DeleteAlarm | AlarmName | Operations([]Operation) | +NotFound +OpFailure +RegionSetup |
| GetAlarms | AlarmName, MonitoredResourceName, PageToken | Alarms([]Alarm), NextPageToken | +NotFound +OpFailure +RegionSetup |
| TestAlarm | AlarmName, State(AlarmState) | Operations([]Operation) | +NotFound +OpFailure +RegionSetup |
| CreateContactMethod | ContactEndpoint, Protocol(ContactProtocol), Tags([]Tag) | Operations([]Operation) | +NotFound +OpFailure +RegionSetup |
| DeleteContactMethod | Protocol(ContactProtocol) | Operations([]Operation) | +NotFound +OpFailure +RegionSetup |
| GetContactMethods | Protocols([]ContactProtocol) | ContactMethods([]ContactMethod) | +NotFound +OpFailure +RegionSetup |
| SendContactMethodVerification | Protocol(ContactMethodVerificationProtocol) | Operations([]Operation) | +NotFound +OpFailure +RegionSetup |

### X. VPC peering (Lightsail <-> default VPC of the standard EC2 account) (3 ops)

All three ops take ZERO input fields -- PeerVpc/UnpeerVpc/IsVpcPeered operate on a single implicit account-wide peering connection between the Lightsail-managed VPC and the caller's default EC2 VPC, not a named/identified resource.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| PeerVpc | (none) | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| UnpeerVpc | (none) | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| IsVpcPeered | (none) | IsPeered | +AcctSetup +NotFound +OpFailure +RegionSetup |

### Y. Tagging (2 ops)

See Cross-service wiring: NO ListTagsForResource op exists anywhere in this 161-op surface -- confirmed by directory listing, not a sampling gap. Both ops key off ResourceName (required) with ResourceArn optional, the reverse of the ARN-first convention most other audited services use.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| TagResource | ResourceName, Tags([]Tag), ResourceArn | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| UntagResource | ResourceName, TagKeys, ResourceArn | Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### Z. The Operation model -- async-operation bookkeeping (3 ops)

See Missing simulated functionality: nearly every mutating op above returns one or more Operation records tracked here; this family's 3 ops are how a caller polls that state to completion.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| GetOperation | OperationId | Operation(*Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetOperations | PageToken | NextPageToken, Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetOperationsForResource | ResourceName, PageToken | NextPageCount, NextPageToken, Operations([]Operation) | +AcctSetup +NotFound +OpFailure +RegionSetup |

### AA. Lightsail for Research GUI sessions (browser-streamed remote desktop) (3 ops)

A newer, narrower sub-product (several AddOn/OperationType doc comments explicitly say 'This add-on only applies to Lightsail for Research resources') layering a remote-desktop-in-browser session on top of an existing instance; Session.Url and the Status enum's GUI-specific values (settingUpInstance/failedStartingGUISession/failedStoppingGUISession) are unique to this family.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| CreateGUISessionAccessDetails | ResourceName | FailureReason, PercentageComplete, ResourceName, Sessions([]Session), Status(Status) | +NotFound +RegionSetup |
| StartGUISession | ResourceName | Operations([]Operation) | +NotFound +RegionSetup |
| StopGUISession | ResourceName | Operations([]Operation) | +NotFound +RegionSetup |

### BB. Miscellaneous account-wide reads (2 ops)

GetActiveNames lists every resource name in use account-wide (Lightsail enforces global name uniqueness across ALL resource kinds, not per-kind); GetCostEstimate is a genuine unfakeable-telemetry-adjacent op, see Missing simulated functionality.

| Op | Input | Output | Errors beyond [base4] |
|---|---|---|---|
| GetActiveNames | PageToken | ActiveNames, NextPageToken | +AcctSetup +NotFound +OpFailure +RegionSetup |
| GetCostEstimate | EndTime(*time.Time), ResourceName, StartTime(*time.Time) | ResourcesBudgetEstimate([]ResourceBudgetEstimate) | +NotFound +RegionSetup |

## 4. Missing simulated functionality

### 4.1 Instances, blueprints/bundles, key pairs, static IPs, instance snapshots, and the instance state machine

- **Instance state has no typed SDK enum.** `InstanceState{Code *int32, Name *string}`
  (`types/types.go`) is free-form; its own doc comment gives only two example values ("the state of
  the instance ( running or pending )"). Unlike EC2 (which this repo already emulates with a real
  `InstanceState`-equivalent), Lightsail's SDK module provides no authoritative full enum. An
  implementer porting the conventional EC2 numeric codes (pending=0/running=16/shutting-down=32/
  terminated=48/stopping=64/stopped=80) is making a reasonable but NON-SDK-SOURCED assumption -- flag
  it as such in the implementation's own comments, don't present it as confirmed by this audit.
- **`InstancePortState.State`'s own doc comment says "The port state for Lightsail instances is
  always open"** (`types/types.go`) -- a real, quotable simplification: an honest implementation can
  hardcode `PortStateOpen` for every configured port rule rather than modeling a real closed state,
  since AWS's own SDK comment says the field is degenerate.
- **Blueprints (`GetBlueprints`) and bundles (`GetBundles`)** are static reference-data catalogs
  (OS/app images like `amazon_linux_2023`, and size tiers like `micro_x_x`) that must be SEEDED, not
  computed -- there is no way to derive real blueprint/bundle catalogs from SDK types alone; an
  implementer needs a hardcoded seed table (this audit does not attempt to enumerate real blueprint/
  bundle IDs, since doing so from memory would be exactly the kind of fabrication this project
  forbids -- the real catalog changes over time and must come from either a live account or is
  explicitly out of scope for a first pass returning a smaller, clearly-synthetic-but-labeled seed
  set).
- **Key pairs**: `CreateKeyPair` returns the private key material (`PrivateKeyBase64`/
  `PublicKeyBase64`) exactly once on the create response -- `GetKeyPair`/`GetKeyPairs` never return
  it again (only public `KeyPair` metadata). `DownloadDefaultKeyPair` is a true account/region
  singleton with no name parameter (zero input fields) -- it needs exactly one lazily-created
  default key pair per backend instance, not a named resource.
- **Static IPs** are a simplified Elastic-IP analogue scoped only to attach/detach against a named
  Lightsail instance (no ENI concept at all).
- **Instance snapshots** capture `FromAttachedDisks []AttachedDisk` (`Path`, `SizeInGb` only -- no
  actual disk *content* to snapshot in an emulator, which is fine and consistent with how this
  project already handles snapshot state elsewhere: real snapshot metadata, not real byte-for-byte
  restore).

### 4.2 Disks, disk snapshots, attach/detach semantics

- `Disk.State` IS a typed enum (`DiskState`: `pending`/`error`/`available`/`in-use`/`unknown` -- 5
  values, `types/enums.go`) -- unlike `RelationalDatabase.State`, this one is genuinely derivable
  and should drive a real `available` <-> `in-use` transition on `AttachDisk`/`DetachDisk`.
- `AttachDisk` takes `DiskPath` (guest device path, e.g. `/dev/xvdf`) and an `AutoMounting` bool.
  Real Lightsail can drive the guest OS to automatically mount the disk via its own in-instance
  agent (`AutoMountStatus` enum: `Failed`/`Pending`/`Mounted`/`NotMounted`, 4 values) -- this
  emulator has no real guest OS to mount anything inside, so `AutoMountStatus` should honestly track
  as a bookkeeping-only field (e.g. always transition to `Mounted` once attached, or leave
  `NotMounted` and say so plainly) rather than pretending real guest-side mount detection occurred.
- `DiskSnapshotState` (`pending`/`completed`/`error`/`unknown`, 4 values) and `CopySnapshot`'s
  `SourceRegion(RegionName)` parameter (the ONLY explicitly cross-region op in this entire 161-op
  surface) both need real state; cross-region copy in an emulator most plausibly means copying the
  snapshot record into whatever this backend's region-keyed storage convention is (see how other
  regional services in this repo key state per-region) rather than literally talking to a second
  region.

### 4.3 Managed databases (relational databases)

- **No typed `RelationalDatabaseState` enum exists in this SDK module at all** -- confirmed by
  grepping `types/enums.go` for the string `RelationalDatabaseState` (zero hits) and reading
  `RelationalDatabase.State`'s field type directly (`*string`, `types/types.go`), whose own doc
  comment gives zero enumerated values (contrast with `ContainerServiceState`'s doc comment, which
  DOES enumerate all 7 of its values). This is a genuine SDK-module gap, not an extraction miss by
  this audit -- an implementer needs the real value set (AWS's public docs list values like
  `available`/`creating`/`modifying`/`backing-up`/`deleting`/`maintenance`/`rebooting`/`starting`/
  `stopping`/`stopped`/`storage-full`/`incompatible-restore`/`incompatible-parameters`/
  `unavailable`/`resetting-master-credentials`, per general AWS RDS-family convention) but this
  audit explicitly did NOT source those values from this SDK module and does not assert them as
  confirmed -- treat as an honest unknown requiring an external, non-SDK source before hardcoding.
- **`RelationalDatabaseEngine`'s only SDK-known value is `"mysql"`** (`types/enums.go`) even though
  the actual `RelationalDatabase.Engine`/`RelationalDatabaseBlueprintId` fields are plain `*string`
  (`types/types.go`) -- the enum type is explicitly self-documented as open/expandable. Do not
  hardcode MySQL as the only supported engine on the strength of this SDK's `Values()` list alone.
- **Snapshots**: `CreateRelationalDatabaseFromSnapshot` supports both a named
  `RelationalDatabaseSnapshotName` AND point-in-time restore via `RestoreTime(*time.Time)` +
  `UseLatestRestorableTime` against a live `SourceRelationalDatabaseName` -- i.e. real
  point-in-time-restore semantics (`RelationalDatabase.LatestRestorableTime`), not just
  snapshot-restore. An honest implementation needs to track a restorable-time window per live
  database, not just a snapshot list.
- **Parameters**: `GetRelationalDatabaseParameters`/`UpdateRelationalDatabaseParameters` manage a
  real parameter-group-like key/value list (`RelationalDatabaseParameter`) -- this needs seed
  parameter names consistent with the "mysql" engine (or whatever engine set is actually
  supported), again requiring reference data this audit does not fabricate.
- **Log streams**: `GetRelationalDatabaseLogStreams`/`GetRelationalDatabaseLogEvents` need a real
  log-stream-name catalog (e.g. `error/mysqld.log`, `slow-query/mysql-slow.log` by AWS convention)
  which this SDK module does not enumerate anywhere -- another reference-data gap.
- **Master password**: `GetRelationalDatabaseMasterUserPassword` genuinely decrypts and returns
  password material by `PasswordVersion` (`CURRENT`/`PREVIOUS`/`PENDING`, 3 values) -- a real
  multi-version secret store per database, not a single string.

### 4.4 Load balancers and TLS certificate attachment

- `LoadBalancerState` has 5 values (`active`/`provisioning`/`active_impaired`/`failed`/`unknown`).
  `LoadBalancerProtocol` has exactly 2 values (`HTTP_HTTPS`/`HTTP`) -- Lightsail LBs are a
  single-listener simplification, not a full ALB rule engine.
  `InstanceHealthState` has 6 values (`initial`/`healthy`/`unhealthy`/`unused`/`draining`/
  `unavailable`) and `InstanceHealthReason` has 11 values covering both LB-side
  (`Lb.RegistrationInProgress`/`Lb.InitialHealthChecking`/`Lb.InternalError`) and instance-side
  reasons -- a real target-health state machine per attached instance, analogous to ELB/ELBv2
  target-group health but simplified to one LB : many instances with no target groups.
- `LoadBalancerTlsCertificate`'s own domain-validation sub-state
  (`LoadBalancerTlsCertificateDomainStatus`, `LoadBalancerTlsCertificateRenewalStatus`,
  `LoadBalancerTlsCertificateRevocationReason`, `LoadBalancerTlsCertificateFailureReason`, and a
  `LoadBalancerTlsCertificateDnsRecordCreationStateCode`) mirrors ACM's own DNS-validation lifecycle
  but as a Lightsail-native, LB-scoped object -- distinct from the CDN-facing `Certificate`/
  `CertificateSummary` family (which has its own, simpler `CertificateStatus`, 7 values) and from
  real ACM. Both certificate families need their own validation-state timers.

### 4.5 Container services -- effectively a whole sub-product with its own state machine

This is the most structurally complex family in the service:

- **`ContainerServiceState`** (7 values: `PENDING`/`READY`/`RUNNING`/`UPDATING`/`DELETING`/
  `DISABLED`/`DEPLOYING`) is the top-level service state.
- **`ContainerServiceStateDetailCode`** (9 values) is populated ONLY while the service is in
  `PENDING`/`DEPLOYING`/`UPDATING`, per its own doc comment, and further splits into two disjoint
  sub-groups by parent state: `CREATING_SYSTEM_RESOURCES`/`CREATING_NETWORK_INFRASTRUCTURE`/
  `PROVISIONING_CERTIFICATE`/`PROVISIONING_SERVICE`/`CREATING_DEPLOYMENT`/
  `EVALUATING_HEALTH_CHECK`/`ACTIVATING_DEPLOYMENT` apply during `DEPLOYING`/`UPDATING`, while
  `CERTIFICATE_LIMIT_EXCEEDED`/`UNKNOWN_ERROR` apply during `PENDING` (confirmed directly from the
  `ContainerServiceStateDetail.Code` doc comment, `types/types.go`).
- **`ContainerServiceDeploymentState`** (4 values: `ACTIVATING`/`ACTIVE`/`INACTIVE`/`FAILED`) is
  tracked PER-DEPLOYMENT via `ContainerService.CurrentDeployment`/`NextDeployment` -- a service can
  have at most one deployment `ACTIVE` at a time, per the `CurrentDeployment` doc comment.
  `CreateContainerServiceDeployment` creates a new pending deployment that must transition through
  `ACTIVATING` before superseding the current one.
- **Container images** (`RegisterContainerImage`) are versioned per label
  (`:container-service-1.mystaticsite.3` naming convention, confirmed from `Container.Image`'s doc
  comment) -- an implementation needs a monotonic per-label version counter per service, not a flat
  image list.
- **This entire sub-product genuinely needs `pkgs/container`** (`pkgs/container/container.go`,
  which already provides a runtime-agnostic Docker/Podman abstraction with image management,
  container lifecycle, and a warm-container pool -- currently used only by `services/lambda`, per
  `grep -rl "pkgs/container" services/` returning only lambda's `container_cleanup_test.go`,
  `containers.go`, `handler_runtime_test.go`, `store.go`, `provider.go`) if it is to actually RUN the
  registered container images rather than just track state bookkeeping around them. An honest
  first-pass implementation should be explicit about which layer it's building: state-machine
  bookkeeping only (a legitimate, honestly-labeled MVP), versus actually launching containers via
  `pkgs/container` (a materially larger lift). Either is defensible; silently claiming the latter
  while only doing the former would not be.

### 4.6 Buckets and access keys (Lightsail's own object storage, distinct from S3)

- `Bucket.ResourceType` is a plain `*string` (`types/types.go`) -- notably NOT the typed
  `ResourceType` enum that almost every other resource in this service uses for its own
  `ResourceType` field (compare `Instance.ResourceType ResourceType`, `Disk.ResourceType
  ResourceType`, etc.) -- a genuine, confirmed wire-shape asymmetry worth preserving faithfully
  rather than "fixing" to match the pattern.
- `AccessKey.SecretAccessKey` is returned in full on `CreateBucketAccessKey` and never retrievable
  again (`AccessKey.LastUsed`'s own doc comment: "This object does not include data in the response
  of a CreateBucketAccessKey action... the region and serviceName values are N/A") -- same
  write-once-readable pattern as `CreateKeyPair`'s private key.
- `BucketState.Code` is `*string` with exactly 2 documented example values in its own comment ("OK"
  / "Unknown") -- again free-form, not a typed enum, despite having a real doc-sourced value set
  this time.
- Buckets are conceptually S3-like (versioning via `ObjectVersioning` string with 3 documented
  values `Enabled`/`Suspended`/`NeverEnabled`, CORS via `BucketCorsConfig`, access logging via
  `BucketAccessLogConfig`) but this repo's real `services/s3` is a SEPARATE, unrelated backend --
  Lightsail buckets should almost certainly be modeled independently, not backed by real S3 state
  (see Cross-service wiring for the parallel EC2/RDS/ELB question, where the answer differs).

### 4.7 Distributions (CDN), domains/DNS, certificates

- `LightsailDistribution.Location`'s own doc comment: "Lightsail distributions are global resources
  that can reference an origin in any Amazon Web Services Region, and distribute its content
  globally. However, all distributions are located in the us-east-1 Region." -- a real, quotable
  AWS behavior an implementation should replicate literally (always report `Location.RegionName =
  "us-east-1"` for a Distribution regardless of what region the backend process is otherwise
  configured for).
- `Origin`/`InputOrigin` reference "a Lightsail instance, bucket, or load balancer" (per
  `LightsailDistribution.Origin`'s doc comment) -- i.e. Distribution has a real FK dependency on
  three other Lightsail-native resource families, not an arbitrary URL.
- **Domain is confirmed a GLOBAL resource**: `Domain.Arn`'s own doc comment gives the literal
  example `arn:aws:lightsail:global:123456789101:Domain/824cede0-abc7-4f84-8dbc-12345EXAMPLE`
  (`types/types.go`) -- the region segment is literally the word `global`, not empty (contrast with
  networkmanager's prior-audited convention of an EMPTY region segment for its global resources,
  `arn:${Partition}:networkmanager::${Account}:...`). This is a genuinely different global-ARN
  convention from a prior audit in this same campaign and should not be assumed to generalize.
- `DomainEntry.Type` is `*string`, not a typed enum -- its doc comment lists 8 example values (A,
  AAAA, CNAME, MX, NS, SOA, SRV, TXT) but Lightsail's own DNS-record model is otherwise unstructured
  free text, unlike Route 53's richer typed `RRType`.
- Certificates (CDN-facing family V) are issued via Lightsail's own Let's-Encrypt integration
  (`CertificateProvider` has exactly 1 value, `LetsEncrypt`) with a 7-value `CertificateStatus`
  (`PENDING_VALIDATION`/`ISSUED`/`INACTIVE`/`EXPIRED`/`VALIDATION_TIMED_OUT`/`REVOKED`/`FAILED`) and
  real DNS-validation-record semantics (`DomainValidationRecord`) -- structurally similar to ACM's
  own validation flow but a distinct, Lightsail-native object, not backed by real ACM.

### 4.8 Alarms, contact methods, and metric data

- `AlarmState` has 3 values (`OK`/`ALARM`/`INSUFFICIENT_DATA`) and `Alarm.NotificationTriggers
  []AlarmState` lets a caller select exactly which state transitions notify -- real CloudWatch
  Alarms-style semantics (`ComparisonOperator`, `EvaluationPeriods`, `DatapointsToAlarm`,
  `TreatMissingData` with 4 values, `Period`, `Statistic`) that an honest implementation must
  actually evaluate against real metric data to be meaningful.
- **Which brings this to the central telemetry-fabrication risk**: `PutAlarm`'s entire evaluation
  logic is meaningless without real underlying `MetricDatapoint` values to compare against a
  threshold. Since (see below) this audit recommends NOT fabricating metric values, an honest
  `PutAlarm`/`TestAlarm`/alarm-evaluation implementation has two defensible options: (a) implement
  the alarm CRUD/state-storage faithfully while being explicit that automatic threshold evaluation
  cannot run against real data and is therefore not implemented, or (b) evaluate only against
  metrics this emulator CAN honestly produce (e.g. real instance-count-derived signals), never
  synthesized CPU/network numbers. `TestAlarm`'s own purpose (force a specific `AlarmState` for
  testing notification wiring) is the one part of this family that is trivially honest to implement
  regardless -- it takes an explicit `State(AlarmState)` input, so it's a pure state-set operation,
  not a fabrication.
- Contact methods: `ContactProtocol` has exactly 2 values (`Email`/`SMS`); a real implementation
  would need actual email/SMS delivery integration to honestly "verify" a contact method via
  `SendContactMethodVerification` -- an honest first pass should model `ContactMethodStatus`'s
  3-value lifecycle (`PendingVerification`/`Valid`/`Invalid`) as caller-driven state (e.g. always
  transition to a fixed test state, clearly documented as not sending real email/SMS) rather than
  pretending real message delivery occurred.

### 4.9 The Operation model (see section 2 above for the full struct/enum breakdown)

Already covered in detail in section 2 -- flagged here again because it is genuinely the single
most under-modelable-if-rushed piece of this service: nearly all 161 ops interact with it, and a
shortcut ("always return `Status: Succeeded` synchronously, never track polling") would pass a
shallow smoke test while failing any caller that actually polls `GetOperation` expecting to observe
a real `NotStarted`/`Started` -> terminal transition.

### 4.10 Telemetry ops that would require inventing metric values -- explicit fabrication warning

Six ops return `MetricData []MetricDatapoint` against a real CloudWatch-shaped
`MetricName`/`MetricStatistic`/`MetricUnit`/`Period` query: **`GetInstanceMetricData`,
`GetRelationalDatabaseMetricData`, `GetDistributionMetricData`, `GetBucketMetricData`,
`GetLoadBalancerMetricData`, `GetContainerServiceMetricData`**. Each has its own `*MetricName` enum
(`InstanceMetricName` 9 values including `BurstCapacityTime`/`BurstCapacityPercentage`/
`MetadataNoToken`; `RelationalDatabaseMetricName` 6 values; `DistributionMetricName` 6 values;
`BucketMetricName` 2 values; `LoadBalancerMetricName` 12 values; `ContainerServiceMetricName` 2
values) -- all real, SDK-confirmed enums, but the actual NUMBERS behind them (CPU percentages,
byte counts, request counts) do not exist anywhere in an emulator with no real compute/network
load. Inventing plausible-looking values -- e.g. a seeded-random CPU percentage that trends
realistically -- is EXACTLY the fabrication `parity-principles.md` forbids ("no ... fabricated IDs,
or stub-output-style responses that skip real state"). **The honest alternative recommended here**:
return a real, well-formed, EMPTY `MetricData` response (zero datapoints) until/unless a genuine
metering hook exists (e.g. real request counts for a Distribution/LoadBalancer that this emulator
DOES actually process could honestly back `RequestCount`/`Requests`, since those requests really do
pass through this process) -- and clearly comment in the code that the zero-datapoint response is a
deliberate honesty choice, not an unfinished stub. `GetCostEstimate` (family BB) is adjacent to this
same risk -- a real cost estimate requires real usage-based billing logic this emulator has no
grounds to fabricate either.

### 4.11 Static reference-data ops needing seeds, not computation

`GetBlueprints`, `GetBundles`, `GetRelationalDatabaseBlueprints`, `GetRelationalDatabaseBundles`,
`GetBucketBundles`, `GetDistributionBundles`, `GetContainerServicePowers`, `GetRegions` (20 real
`RegionName` values enumerated in `types/enums.go`, at least, so that one IS SDK-derivable) all need
static catalog tables that either come directly from the SDK enum (regions) or must be seeded from
an external, explicitly-labeled-as-synthetic source (blueprints/bundles/powers, whose real IDs and
prices this audit does not fabricate).

## 5. Cross-service wiring needed

### 5.1 Tagging

- **Confirmed: `TagResource`/`UntagResource` exist; `ListTagsForResource` does NOT.**
  `ls api_op_*.go | grep -i tag` in the module cache returns only `api_op_TagResource.go` and
  `api_op_UntagResource.go` -- no third file. This means Lightsail genuinely diverges from the
  ARN-first, `ListTagsForResource`-having convention nearly every other wired service in this repo
  follows: tags are visible ONLY by reading a resource's own `Tags []Tag` field from its `Get*`/
  `Describe*`-equivalent op (e.g. `GetInstance().Instance.Tags`), never via a dedicated list-by-ARN
  call.
- **`TagResource`/`UntagResource` key on `ResourceName` (required) with `ResourceArn` OPTIONAL**
  (`api_op_TagResource.go`: `ResourceName *string` marked "This member is required", `ResourceArn
  *string` with no such marker) -- the reverse of the ARN-required convention. An implementer
  wiring `resourcegroupstaggingapi` (`cli.go:5357`, `wireResourceGroupsTagging`) will need a
  resource-NAME-keyed lookup path in addition to (or instead of) the ARN-keyed
  `wireTaggingCtxARNResources` helper pattern seen in e.g. `wireTaggingEFS` (`cli.go:6137-6162`,
  which derives resource type FROM the ARN's own service segment via `resourceTypeFromARN(arn,
  "elasticfilesystem")`) -- Lightsail resources are name-addressed first, ARN-addressed second.
- **ARN namespace**: confirmed literally `lightsail` (NOT a diverging case like stepfunctions ->
  `states` or efs -> `elasticfilesystem`) -- every ARN example embedded in `types/types.go`'s doc
  comments uses `arn:aws:lightsail:...` verbatim (e.g. `Instance.Arn`'s doc comment:
  `arn:aws:lightsail:us-east-2:123456789101:Instance/244ad76f-8aad-4741-809f-12345EXAMPLE`). This
  means `pkgs/arn.Build("lightsail", region, accountID, resource)` (`pkgs/arn/arn.go:36-39`) needs
  NO special-casing for the service-name segment, unlike the seven diverging cases flagged in prior
  audits this campaign.
- **ARN resource-type segment is CAPITALIZED and matches the `ResourceType` enum's own casing
  exactly**: `Instance/{uuid}`, `StaticIp/{uuid}`, `KeyPair/{uuid}`, `InstanceSnapshot/{uuid}`,
  `Domain/{uuid}` (all four confirmed via literal doc-comment examples in `types/types.go`) --
  meaning an ARN builder for this service can derive the resource-type path segment directly from
  the same `ResourceType` enum used elsewhere on each struct's own `ResourceType` field, rather than
  needing a second, separately-maintained lowercase mapping table (contrast with typical AWS
  services, including this repo's own EC2, which use lowercase resource-type prefixes like
  `instance/i-xxx`).
- **Regional vs. global is SPLIT, not uniform** -- a genuine, confirmed divergence worth flagging
  clearly since it's easy to get wrong: `Instance`, `Disk`, `StaticIp`, `KeyPair`,
  `InstanceSnapshot`, and (by strong implication, though not independently doc-comment-confirmed
  for every single one of the remaining 15 `ResourceType` values) most other resource kinds are
  REGIONAL (`arn:aws:lightsail:us-east-2:...`), while **`Domain` is confirmed GLOBAL**
  (`arn:aws:lightsail:global:...`, literal doc-comment example) and **`Distribution` is a
  region-agnostic product that nonetheless reports a literal `us-east-1` `Location.RegionName`**
  (per `LightsailDistribution.Location`'s own doc comment, section 4.7 above) -- a THIRD pattern
  distinct from both "purely regional" and "purely global/no-region-segment". This audit could NOT
  independently confirm the ARN region-segment convention for the other 17 `ResourceType` values
  (`PeeredVpc`, `LoadBalancer`, `LoadBalancerTlsCertificate`, `DiskSnapshot`, `RelationalDatabase`,
  `RelationalDatabaseSnapshot`, `ExportSnapshotRecord`, `CloudFormationStackRecord`, `Alarm`,
  `ContactMethod`, `Certificate`, `Bucket`, `ContainerService`) from this SDK module alone -- no
  further doc-comment ARN examples exist for them in `types/types.go` (confirmed:
  `grep -n "arn:aws:lightsail" types/types.go` returns exactly the 6 lines cited above and no
  others). The safest default recommendation is regional (matching the majority observed pattern),
  but this is an honest gap, not a verified fact, for those 13 kinds.
- **20 distinct taggable resource kinds** share the `lightsail` ARN namespace, per the full
  `ResourceType` enum (`types/enums.go`): `ContainerService`, `Instance`, `StaticIp`, `KeyPair`,
  `InstanceSnapshot`, `Domain`, `PeeredVpc`, `LoadBalancer`, `LoadBalancerTlsCertificate`, `Disk`,
  `DiskSnapshot`, `RelationalDatabase`, `RelationalDatabaseSnapshot`, `ExportSnapshotRecord`,
  `CloudFormationStackRecord`, `Alarm`, `ContactMethod`, `Distribution`, `Certificate`, `Bucket` --
  more than mgn's 12 and directconnect's 5, meaning the eventual tag store needs real multi-kind ARN
  dispatch (or, given the name-first tagging convention above, multi-kind NAME dispatch) at least as
  rich as networkmanager's 9-kind case, likely richer.

### 5.2 EC2/EBS/ELB/RDS/Route53/ACM/CloudFront: which backends exist, and the independent-vs-backed decision

All seven real AWS services Lightsail abstracts over ALREADY EXIST as working code in this tree:

- **EC2**: `services/ec2/store.go:130` (`type Instance struct`), `services/ec2/handler.go:32`
  (`type Handler struct`), package `ec2` (`services/ec2/accept_ops.go:1`).
- **RDS**: `services/rds/models.go:88` (`type DBInstance struct`), `services/rds/handler_dispatch.go:19`
  (`type Handler struct`).
- **ELB (Classic)**: `services/elb/models.go:82` (`type LoadBalancer struct`),
  `services/elb/handler.go:34` (`type Handler struct`).
- **ELBv2 (ALB/NLB)**: `services/elbv2/models.go:38` (`type LoadBalancer struct`),
  `services/elbv2/handler.go:30` (`type Handler struct`).
- **Route 53**: `services/route53/handler.go:67-69` (`type Handler struct { Backend StorageBackend
  }`, `Name() string { return "Route53" }`).
- **ACM**: `services/acm/handler.go:26` (`type Handler struct`).
- **CloudFront**: `services/cloudfront/handler.go:236` (`type Handler struct`).

**Recommendation: model Lightsail resources INDEPENDENTLY, not backed by these real services'
state**, for these reasons:

1. **Wire-shape mismatch is severe, not superficial.** Lightsail's `Instance` has NO EC2 concepts
   at all in its SDK shape -- no VPC ID, no subnet ID, no security-group ID list, no AMI ID (uses
   `BlueprintId`/`BundleId` instead), no instance-type string (uses `BundleId`), and a completely
   different `InstanceState` shape (bare `{Code, Name}` vs. EC2's richer state). Backing a Lightsail
   `Instance` with a real `services/ec2` `Instance` (`services/ec2/store.go:130`) would require a
   nontrivial translation layer for every field, and Lightsail's OWN simplifications (single
   public+private IP, no VPC/subnet exposed to the caller at all, `IpAddressType` instead of
   dual-stack ENI configuration) would have to be reverse-engineered FROM real EC2 concepts on every
   read -- more work than modeling independently, for a service whose entire value proposition is
   hiding those concepts from the caller.
2. **Lightsail's real product DOES run on top of a hidden, caller-inaccessible VPC** (confirmed by
   the `PeerVpc`/`UnpeerVpc`/`IsVpcPeered` family X existing at all, and by `DomainEntry`'s own doc
   comment describing Lightsail load-balancer/distribution/container-service DNS targets as real
   AWS-managed hostnames like `*.us-east-2.elb.amazonaws.com` / `*.cloudfront.net` /
   `*.cs.amazonlightsail.com`) -- but that hidden VPC and its real backing resources are NOT
   observable through the Lightsail API surface itself. An emulator only needs to honestly implement
   what's OBSERVABLE through the 161 ops audited here; it does not need to also stand up a shadow
   EC2/RDS/ELB reality behind the scenes to be faithful, since real Lightsail customers can't
   observe that reality either.
3. **Precedent in this repo**: services that wrap other AWS products (this audit did not
   exhaustively survey for a perfect precedent, but the general pattern in this tree favors
   independent backend state per service, coordinated only through the tagging bridge
   (`resourcegroupstaggingapi`) and explicit cross-service reference fields where the wire shape
   itself demands it) -- Lightsail's OWN wire shape demands FK-style references in only a few
   places (`AttachCertificateToDistribution`'s `CertificateName`, `Origin` referencing an
   instance/bucket/LB by name) which are all intra-Lightsail references, not cross-service ones.
4. **The one place a real cross-service link is unavoidable and explicit in the wire shape**:
   `CreateCloudFormationStack` (family K) launches a REAL CloudFormation stack outside Lightsail
   entirely, converting an exported Lightsail snapshot into real EC2 resources -- if
   `services/cloudformation` supports EC2 instance/resource types (it has 48 `resources_*.go`
   files, confirmed to include `resources_acm.go` etc., not independently checked for EC2 instance
   support in this pass), THIS is the one legitimate integration point where Lightsail's
   independent snapshot state should hand off to a real `services/cloudformation` stack creation
   call, not fabricate a plausible-looking stack record.

**Counter-consideration, stated honestly**: independent modeling means double-maintaining similar
state-machine logic (e.g. Lightsail's own `DiskState`/`InstanceSnapshotState` vs. EC2's equivalent)
in two places in this repo. If a future refactor wants to reduce that duplication, the RIGHT
extraction point is probably a shared internal state-machine helper (not shared AWS-shaped structs),
so each service's public wire shape stays independently faithful to its own real SDK while the
underlying transition logic is DRY.

### 5.3 Existing Lightsail references in this repo

`grep -rni "lightsail" services/ cli.go` (case-insensitive, whole tree) returns **zero hits** --
confirmed via direct grep in this session, no prior partial work, no stray references anywhere.

### 5.4 CloudFormation

`grep -rli lightsail services/cloudformation/*.go` across all 48 `resources_*.go`-pattern files
(confirmed count: `ls services/cloudformation/resources_*.go | grep -v test | wc -l` => 48) returns
**zero hits** -- no `AWS::Lightsail::*` resource type is implemented in this repo's CloudFormation
emulation. This audit did not independently verify whether AWS's own real CloudFormation supports
any Lightsail resource type at all (that would be a claim about AWS's actual product, not about
this repo's tree, and this pass did not check AWS's own CloudFormation resource-type registry) --
reported as absent-in-this-repo only, consistent with, not contradicting, the 4.b architectural note
above about `CreateCloudFormationStack` being the one legitimate cross-service handoff point.

## Suggested implementation ordering

Given the size (161 ops across 28 families), landing this incrementally rather than all at once is
strongly recommended:

1. **Operation model + Instances core (families Z, B, C)** first -- nearly everything else returns
   `Operation` records, and Instances are the load-bearing resource every other family (snapshots,
   disks, LB attachment, GUI sessions) references by name.
2. **Key pairs, static IPs, disks + disk snapshots (families G, H, I, J)** -- self-contained,
   instance-adjacent, no complex sub-state-machines.
3. **Instance snapshots, auto-snapshots/add-ons, export/CloudFormation (families D, F, K)** --
   builds on 1-2.
4. **Load balancers + LB TLS certs (families L, M)** -- introduces multi-instance attachment and a
   real health-state machine.
5. **Domains/DNS, certificates (families U, V)** -- needed before distributions (which reference
   certificates) and container services (which reference domains).
6. **Distributions (family T)** -- depends on 5 and on instances/buckets/LBs existing as origins.
7. **Buckets (family S)** -- fully independent of 1-6, could be parallelized with them by a second
   implementer.
8. **Managed relational databases (families N, O, P)** -- the largest self-contained sub-product
   (20 ops); its master-password/parameter/log-stream reference-data needs are real work.
9. **Container services (families Q, R)** -- deliberately last: the most complex state machine
   (section 4.5) and the one family where a real vs. bookkeeping-only implementation decision must
   be made explicitly and documented, not glossed over.
10. **Alarms/contact methods, GUI sessions, VPC peering, misc, reference-data (families W, AA, X,
    BB, A)** -- lower-priority, mostly independent reads/small state machines; reference-data family
    A should actually be seeded EARLY in practice (many other families' `Get*Blueprints`/
    `Get*Bundles` calls are referenced by create ops throughout), noted last here only because it
    requires the most externally-sourced (non-SDK) reference data to do honestly.
11. **Tagging wiring into `resourcegroupstaggingapi`** (`cli.go:5357`) can happen incrementally
    alongside any of the above, once at least one resource kind's name-first tag storage exists to
    prove out the pattern described in 5.1.

## Self-review of numeric claims

Before finalizing, all counts in this document were re-derived programmatically rather than
eyeballed, per this campaign's stated practice of catching an audit's own arithmetic errors:

- **161 total ops**: `ls api_op_*.go | grep -v _test.go | wc -l` = 161, independently re-confirmed
  by `len()` of a Python dict built from parsing every `api_op_*.go` file's Input/Output structs
  (161 keys).
- **28 families sum to 161 with zero gaps/duplicates**: verified programmatically (set difference
  between the full 161-op list and the union of all 28 family lists = empty; per-op assignment
  count histogram showed no op assigned to more than one family).
- **7 error-set signatures summing to 161**: 103+31+12+11+2+1+1 = 161, computed via Python
  `collections.defaultdict` grouping over all 161 per-op error lists (not counted by hand).
- **8 shared error shapes, all with the identical 5-field `{Message, ErrorCodeOverride, Code, Docs,
  Tip}` shape**: verified by `awk`-extracting each of the 8 struct bodies directly and diffing them
  by eye (all 8 identical field lists, confirmed).
- **"160 of 161 ops carry `InvalidInputException`"**: verified via Python set-difference (`ops
  lacking InvalidInputException` = `["GetContainerAPIMetadata"]`, length 1, not assumed).
- **20 `ResourceType` values, 84 `OperationType` values, 5 `OperationStatus` values**: counted
  directly from each enum's own `Values()` function body length in `types/enums.go` (not
  hand-counted from the `const` block, to avoid miscounting past a line-wrap).

No corrections were needed to the counts above after this re-check; this note itself IS that
re-check, done before this document was finalized rather than after.

## 2026-08-30 sort-totality sweep (wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)

Audited every `sort.Slice` call for whether its comparator is a *total*
order. Every resource collection in this backend is a `store.Table[V]`
keyed by `Name` (or, for a handful of families, another field), so a sort on
that same key field is total by construction — `Table.Put` cannot produce
two distinct entries with equal key. That covers every `Name`-sorted site
(instances, disks, static IPs, key pairs, snapshots, load balancers,
databases, buckets, distributions, domains, certificates, alarms, CFN stack
records, export snapshot records, container services) and the one
`Protocol`-sorted site (`alarms_contacts.go`'s contact-method listing —
`contactMethodKeyFn` keys the table on `Protocol` itself, so two contact
methods with the same protocol cannot coexist). None of those needed a
change.

**Fixed (non-total sort, tiebreak added) — `CreatedAt`-based, sourced from
`store.Table.All()` (unordered map iteration) with no tiebreak:**

- `GetOperations` — sorted on `Operation.CreatedAt` alone. Operations are
  routinely created in batches (one mutating call can spawn several), so a
  tie is the ordinary case, not a contrived one. Added `ID` tiebreak.
- `GetOperationsForResource` — same `CreatedAt`-alone sort, same fix
  (`ID` tiebreak). Its source is `opsByResource.Get` (an `Index`), not
  `Table.All()` — see the mutation-safety fix below, which is the more
  serious bug at this call site.
- `GetSetupHistory` — same `CreatedAt`-alone sort in both branches
  (`resourceName`-scoped via the `setupHistoryByResource` index, and the
  unscoped `setupHistory.All()` branch). Added `OperationID` tiebreak to
  both.

**Also fixed — a second, more serious bug found at the same two call
sites while auditing them for totality:** `Index.Get` returns the index's
own backing slice (its doc comment: *"The returned slice is owned by the
index — the caller must not mutate it"*), but `GetOperationsForResource` and
`GetSetupHistory`'s `resourceName`-scoped branch both passed that slice
straight into `sort.Slice`, reordering the index's live bucket in place.
Under this package's coarse-lock convention that's a correctness bug even
single-threaded (a concurrent `RLock` reader of the same `resourceName`
observes the sort mid-flight) and a `go test -race` hazard the moment two
goroutines call either method concurrently for the same resource. Fixed by
copying the slice (`append([]*T(nil), idx.Get(...)...)`) before sorting.
Verified with `go test -race -count=1 ./services/lightsail/...` — clean.

**Confirmed correct, left unfixed (evidence, not presumption):**

- `addons.go`'s `sortAutoSnapshots` (sorts `AutoSnapshotDetails.Date`) reads
  from `Instance.AutoSnapshots`/`Disk.AutoSnapshots`, a strictly
  append-ordered slice field (`i.AutoSnapshots = append(...)`), never
  rebuilt from a map or index. Same shape as the `ram`-listings precedent
  from the prior pass: append-ordered source means a tied `Date`'s relative
  order is a fixed function of insertion order, reproducible across repeated
  calls with no intervening mutation — and in practice an auto-snapshot's
  `Date` (one per calendar day) cannot tie for the same resource anyway. Not
  fixed; not observably unstable.
- `databases.go`'s `GetRelationalDatabaseEvents` sorts `db.Events`, also a
  strictly append-ordered slice field (`db.Events = append(...)`) reached via
  a single unique-key `Table.Get(name)` lookup, not iteration. Same
  reasoning; not fixed.

**Existing test-suite weakness confirmed:** no existing pagination test in
this package constructed a tie group and compared item identity across a
full multi-page walk. `GetOperations`/`GetOperationsForResource`/
`GetSetupHistory` all page at a fixed size (`defaultPageLimit` = 100 — none
of the three real Lightsail ops they mirror takes a caller-supplied page
size), so the new tests (`pagination_sort_totality_test.go`) seed 105 tied
records per case to force a real two-page boundary, via new
`SeedOperationForTest`/`SeedSetupHistoryEntryForTest` test-only helpers
(`export_test.go`, this package's first — added following the same pattern
already established in `bedrock`/`cloudwatchlogs`) since neither type's
`CreatedAt` is otherwise reachable from a `_test` package to force an exact
tie.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/lightsail/...`).

## 2026-08-30 gopherstack-wlo1: error-envelope sweep, confirmed clean

Lightsail is `awsAwsjson11` (AWS JSON 1.1 RPC), not restjson1 -- confirmed
by `deserializers.go`'s `awsAwsjson11_deserializeOpError<Op>` function name
prefix on all 161 ops. Read all 161 (161-of-161, not sampled): every one is
byte-identical generated boilerplate calling `getProtocolErrorInfo(decoder)`
plus `response.Header.Get("X-Amzn-ErrorType")`, resolved via
`resolveProtocolErrorType` (header first, else body `__type`, else body
`code`), with `message`/`Message` (case-insensitive, untagged struct field)
for the message. `handler.go`'s `handleError` writes exactly
`{"__type": errType, "message": err.Error()}` -- no header needed since the
body `__type` key alone satisfies the client's fallback. Single error path
confirmed: grepped for any other `JSONBlob`/`__type` writer in the package,
found none -- `handleError` is the sole call site, used for both real
business-logic errors (`classifyLightsailError`) and framework-level
dispatch failures (`pkgs/service/jsondisp.go`'s shared `writeDispatchError`,
already fixed for the whole JSON-target family). HTTP status doesn't affect
identification here -- the client's error path triggers on any status
outside 200-299, confirmed in the generated deserializer.

No bug found. Added `TestErrorEnvelope_NotFoundDecodesToTypedError`
(`error_envelope_test.go`), driving a real `lightsailsdk.Client` through
`GetInstance` for a nonexistent instance: asserts `errors.As` unwraps to
the concrete `*types.NotFoundException` (not just that an error occurred),
and separately asserts on the raw response bytes/status for the same case.
Passed against unmodified code, confirming this service's error envelope
was already wire-correct.

Gates (this pass, `services/lightsail/` only): `go build`, `go vet`,
`go test -race -count=1`, `golangci-lint run` -- all clean.

## Handler-collision determinism sweep (2026-08-31, gopherstack-id70)

Same defect and fix as the census in `cmd/reqfielddiff`/`cmd/reqfieldscan`
(ef0eef041, appsync e2643a6dd). This package's `Ip`/`IP`, `Tls`/`TLS`, and `Https`/`HTTPS` acronym casing
gives it 13 op/handler pairs needing the ambiguous fold, 13 of them
genuine collisions between an exported backend method and the real
unexported handler: `AllocateStaticIp`, `AttachLoadBalancerTlsCertificate`, `AttachStaticIp`, `CreateLoadBalancerTlsCertificate`, `DeleteLoadBalancerTlsCertificate`, `DetachStaticIp`, `GetLoadBalancerTlsCertificates`, `GetLoadBalancerTlsPolicies`, `GetStaticIp`, `GetStaticIps`, `ReleaseStaticIp`, `SetIpAddressType`, `SetupInstanceHttps`.

Verified directly rather than assumed: ran the unpatched tool from
`ef0eef041~1` five times and diffed against the fixed tool at HEAD, for
both `cmd/reqfieldscan` and `cmd/reqfielddiff`. Both were byte-identical
across all 5 old runs and HEAD (161 SDK operations compared) -- the
determinism defect never flipped a finding here, because the resolution
that actually mattered (this package's dispatch-table union) already
carried the correct field set regardless of which fold candidate won.

Verdict: confirmed zero damage, not merely predicted.

## 2026-09-06: StartInstance now reassigns the dynamic public IP (gopherstack-i2s6)

`publicIPForName` was a pure function of instance name, so `StartInstance`
never changed `PublicIPAddress` across a stop/start cycle. Source for the
real behavior: `api_op_StartInstance.go`'s doc comment in
`aws-sdk-go-v2/service/lightsail@v1.58.4` (also on `api_op_StopInstance.go`):
"When you start a stopped instance, Lightsail assigns a new public IP
address to the instance. To use the same IP address after stopping and
starting an instance, create a static IP address and attach it to the
instance." `types.Instance.IsStaticIp`'s doc comment ("A Boolean value
indicating whether this instance has a static IP assigned to it") confirms
AWS models the two cases distinctly.

Static IPs were already modelled here (`Instance.IsStaticIP`,
`AttachStaticIp`/`DetachStaticIp`/`ReleaseStaticIp`), so the fix is
conditional: `StartInstance` now assigns a new public IP on a
stopped->running transition only when `!IsStaticIP`, leaving an attached
static IP untouched. `publicIPForName` gained a `generation int32`
parameter (`Instance.PublicIPGeneration`, bumped each qualifying restart)
folded into its hash input, so the address stays a pure, reproducible
function of (name, generation) instead of drawing on `time.Now` or
`crypto/rand`.
