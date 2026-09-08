---
service: appconfig
sdk_module: aws-sdk-go-v2/service/appconfig@v1.48.4    # version audited against (bumped from v1.43.11)
last_audit_commit: f86ef17b                            # this pass (2026-08-13, gopherstack-xs7l) fixed the
                                                        # seven List-op Get-field leaks below; commit hash not
                                                        # yet known at edit time
last_audit_date: 2026-09-07   # bd gopherstack-kpvs: re-derived a title-only, empty-description issue
                               # that conflated two separate claims about DeletionProtectionCheck. Claim 1
                               # ("header never read") was TRUE and independently fixable: DeleteEnvironment/
                               # DeleteConfigurationProfile now read+validate the 'X-Amzn-Deletion-Protection-
                               # Check' header (serializers.go:1121, :1268) and reject an unrecognized value
                               # with BadRequestException instead of silently accepting garbage -- FIXED, see
                               # op notes and TestHandler_Delete{Environment,ConfigurationProfile}_
                               # DeletionProtectionCheck. Claim 2 ("full enforcement needs appconfigdata
                               # recency + a reverse hook") was independently verified TRUE and remains
                               # structurally blocked -- confirmed no cross-service backend-lookup pattern
                               # exists anywhere in this repo (grepped for any package outside services/
                               # appconfig{,data} importing either backend directly; each Provider.Init wires
                               # its own backend with no registry handle to another service's), and
                               # appconfigdata's own LastAccessedAt (models.go:94/111, set in
                               # configuration.go:196) is scoped to its private session store, never exposed
                               # to appconfig. Not built -- out of scope, left disclosed. BYPASS/APPLY/
                               # ACCOUNT_DEFAULT are all accepted and all behave like an absent header (never
                               # block), which is the correct behavior for BYPASS and a disclosed non-
                               # enforcement for the other two -- see the deletion_protection_check gap entry.
                               # 2026-09-04 (prior pass) bd gopherstack-5pl: full ranked bug-pattern sweep (delete preconditions,
                               # deployment state machine, deployment strategy numeric ranges, ghost rows,
                               # referential integrity, inert config, fabricated values/unstable
                               # sort/leaks) against all 6 Delete ops, the deployment strategy/validator
                               # fields, and the appconfigdata cross-service boundary. One new, previously
                               # undisclosed gap found and documented (not code-fixed at the time, see gaps
                               # below): DeleteEnvironment/DeleteConfigurationProfile's DeletionProtectionCheck
                               # header was entirely unread -- now fixed per the 2026-09-07 entry above.
                               # Everything else re-checked this pass (all 6 Delete ops' modeled error
                               # sets against deserializers.go, DeploymentDurationInMinutes/GrowthFactor/
                               # FinalBakeTimeInMinutes -- confirmed the SDK's own validators.go models no
                               # numeric range for any of them, only required-ness, so there is no range to
                               # enforce; Validators -- SDK gives no documented trigger for when/how
                               # JSON_SCHEMA/LAMBDA validators execute, so left unimplemented per the same
                               # honesty rule; all 11 sort.Slice call sites' sort keys confirmed unique
                               # (ID/VersionNumber/Run/DeploymentNumber/ExtensionAssociationID), so no
                               # unstable-sort bug) came back clean -- confirmed, not merely re-asserted.
                               # Stays A.
                               # 2026-08-29 (prior pass) bd gopherstack-6flj/21my continuation: StartDeployment's Tags/
                               # KmsKeyIdentifier/LatestDeploymentNumber fixed (see overall/ops notes).
                               # prior pass 2026-08-15, bd gopherstack-6flj wrapper-key/discarded-input sweep: 4 real bugs fixed
                               # (ConfigurationProfile/Deployment.KmsKeyIdentifier discarded on input and
                               # never echoed; StopDeployment returned 204 empty instead of the real 200
                               # body -- major, silent all-zero output; ExtensionParameter.Dynamic
                               # discarded/unmodeled; AccountSettings.VendedMetrics discarded/unmodeled).
                               # See the CreateConfigurationProfile/GetDeployment/StopDeployment/
                               # GetAccountSettings/CreateExtension op notes below for detail. None of
                               # these are grade-changing on their own (each is a narrow, now-fixed field
                               # gap, not a structural failure), so overall stays A, but see this date's
                               # citations for what a "5+ pass A grade" audit had not actually checked:
                               # member-set diffs on Get/Create/Update outputs beyond the fields already
                               # flagged, not full request/response struct diffs against the pinned SDK.
overall: A            # 2026-08-29 (gopherstack-21my, parameter-honoring sweep, same day continuation):
                       # measured and audited all 11 List ops with a filter or pagination parameter
                       # (ListApplications/DeploymentStrategies have pagination only, no filters --
                       # confirmed clean). 10 of 11 already correctly honored every documented filter
                       # (ListConfigurationProfiles.Type, ListExperimentDefinitions' 4 filters,
                       # ListExperimentRuns.Status, ListExtensions.Name, ListHostedConfigurationVersions.
                       # VersionLabel, ListExtensionAssociations' extension_version_number/
                       # resource_identifier, pagination via the shared appConfigPaginate chokepoint used
                       # by every List op with no bypass found) -- confirmed by reading each op's own
                       # backend filter logic against its SDK-documented parameter list, not re-asserted.
                       # One real bug found and fixed: ListExtensionAssociations.ExtensionIdentifier
                       # (name/ID/ARN documented) only matched an ARN -- see its op note below for the
                       # shared-resolver root cause and fix.
                       # 2026-08-29 (gopherstack-6flj/21my wrapper-key sweep continuation): StartDeployment
                       # silently discarded three real StartDeploymentInput members it never bound at all
                       # (Tags, KmsKeyIdentifier, LatestDeploymentNumber) -- see the StartDeployment op note
                       # for detail and the new DynamicExtensionParameters gap entry for what was found but
                       # NOT fixed (no honest sink, same class as the pre-existing ActionInvocations/
                       # AppliedExtensions-content precedent). Every other family re-walked this pass
                       # (ConfigurationProfileSummary/DeploymentSummary/ExperimentDefinitionSummary/
                       # ExperimentRunSummary/ExtensionAssociationSummary/ExtensionSummary/
                       # HostedConfigurationVersionSummary field-diffed member-by-member against the pinned
                       # SDK, Environment.Monitors, Treatment.Weight/FlagValue/AttributeValues, GetExtension
                       # family, tags plumbing) came back clean -- confirmed, not merely re-asserted.
                       # Stays A.
                       # 2026-08-21 (gopherstack-c8ge): fixed UpdateAccountSettings (singleton, no Create
                       # op) wholesale-swapping DeletionProtectionSettings' pointer instead of merging its
                       # two independently-optional fields -- see the UpdateAccountSettings op row.
                       # RAISED from A- (parity-5, this pass). The 2026-07-30 re-audit confirmed all four
                       # reasons the experiment-family pass cited for the A- downgrade, then this pass acted
                       # on that finding: three are genuine documented-API-behavior gaps that should never
                       # have held the grade down by themselves (an unverifiable-against-the-SDK default, or
                       # a server-assigned value the SDK gives no scheme for, is not a backend bug -- see
                       # HONESTY RULES precedent elsewhere in this campaign), and the fourth (FlagKey
                       # validated for presence only) was a real, closeable gap that is now closed. (1)
                       # StartExperimentRun's ExposurePercentage default and (2)
                       # DeleteExperimentDefinition's delete_type default: api_op_StartExperimentRun.go /
                       # api_op_DeleteExperimentDefinition.go document no default for either omitted field --
                       # confirmed unverifiable against the SDK, unchanged this pass, and per this campaign's
                       # own standard a documented absence of a default is a disclosed assumption, not a bug,
                       # so it does not independently justify A- over A (see the gaps list below, still
                       # disclosed for client awareness). (3) Treatment.Key's server-generated naming scheme:
                       # TreatmentInput (types.go) has no client-supplied Key member at all, so AWS must
                       # assign one server-side, but the exact scheme is nowhere in the SDK -- same
                       # unverifiable-not-a-bug treatment, unchanged, still disclosed. (4) FlagKey: FIXED
                       # THIS PASS. validators.go's validateOpCreateExperimentDefinitionInput only ever
                       # required FlagKey be non-nil, but CreateExperimentDefinitionInput.FlagKey's own doc
                       # comment reads "The key of the **existing** feature flag to use with the experiment"
                       # -- existence against real content is the documented contract, and this backend now
                       # has the feature-flag-content model needed to check it (feature_flags.go): a
                       # ConfigurationProfile's AWS.AppConfig.FeatureFlags-typed content, once uploaded via
                       # CreateHostedConfigurationVersion, was already stored as opaque bytes (see
                       # HostedConfigurationVersion.Content) -- it is now parsed against AWS's published
                       # AWS.AppConfig.FeatureFlags JSON schema (not part of the Go SDK, which treats this
                       # content as opaque -- field-diffed against
                       # https://docs.aws.amazon.com/appconfig/latest/userguide/appconfig-type-reference-feature-flags.html
                       # directly) and CreateExperimentDefinition's FlagKey is checked against the parsed
                       # "flags" key set. A profile with no uploaded content yet, or non-feature-flag
                       # content, stays permissive (same "unspecified, not wrong" precedent already used for
                       # an unset ConfigurationProfile.Type) so pre-existing content-less test fixtures are
                       # unaffected. The pre-existing 45 ops, and the rest of the experiment family (real
                       # state, real errors, real reference validation, real persistence, the six
                       # pre-existing Create* handlers' bd gopherstack-lcan inline-Tags fix), are unchanged
                       # and still hold.
                       # 2026-08-29 wrapper-key sweep (query/path/header key hunt, cross-service with
                       # apigateway/efs/transfer): every REQUEST-direction Query/URI/Header binding in
                       # appconfig@v1.48.4 serializers.go checked op-by-op against this handler's actual
                       # parameter reads. 3 real bugs found and fixed, all "parameter never read" (not
                       # mis-keyed -- appconfig's existing read keys were already correct where present):
                       # ListConfigurationProfiles' type filter, ListExtensionAssociations'
                       # extension_version_number filter, and GetConfiguration's
                       # client_configuration_version (real AWS returns 204 empty-Content when it matches
                       # the deployed version instead of resending data). Everything else -- max_results/
                       # next_token pagination and every other filter across all 33 Query/URI-bound ops
                       # (name, status, application_identifier, configuration_profile_identifier,
                       # environment_identifier, delete_type, version, version_number, version_label,
                       # extension_identifier, resource_identifier) -- was already reading the correct
                       # wire key. GetConfiguration's client_id remains an unfixed gap: no weighted
                       # per-client gradual-rollout bucketing model exists to key it against.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (bd gopherstack-lcan): inline Tags were never bound/forwarded (dropped silently, ListTagsForResource returned empty); handler now binds Tags and CreateApplication applies them directly to b.tags (not via TagResource, to avoid re-entrant locking -- same pattern as CreateExperimentDefinition)."}
  GetApplication: {wire: ok, errors: ok, state: ok, persist: ok}
  ListApplications: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "Description omit-means-unchanged semantics verified against optional *string UpdateApplicationInput members."}
  DeleteApplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — cascade delete now also removes ExtensionAssociations targeting the app/env/profile ARNs being deleted (previously left as ghost rows referencing deleted resources) and deployedConfigs tracking entries for the app."}
  CreateEnvironment: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (bd gopherstack-lcan): same inline-Tags-dropped bug/fix as CreateApplication."}
  GetEnvironment: {wire: ok, errors: ok, state: ok, persist: ok}
  ListEnvironments: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateEnvironment: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteEnvironment: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED — same ExtensionAssociation + deployedConfigs cascade-cleanup as DeleteApplication. FIXED 2026-09-07 (bd gopherstack-kpvs): real DeleteEnvironmentInput.DeletionProtectionCheck (api_op_DeleteEnvironment.go, bound to the 'X-Amzn-Deletion-Protection-Check' request header per serializers.go:1268) was never read by this handler at all -- now parsed and validated against the types.DeletionProtectionCheck enum (BYPASS|APPLY|ACCOUNT_DEFAULT); an unrecognized value now gets BadRequestException instead of being silently accepted. NOT fixed, disclosed: actually enforcing the check (blocking a delete when APPLY/ACCOUNT_DEFAULT and the resource was recently read via appconfigdata GetLatestConfiguration) -- see the deletion_protection_check gap entry below for why that half is structurally out of reach right now."}
  CreateConfigurationProfile: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (bd gopherstack-lcan): same inline-Tags-dropped bug/fix as CreateApplication. ALSO FIXED (bd gopherstack-6flj): real CreateConfigurationProfileInput.KmsKeyIdentifier (api_op_CreateConfigurationProfile.go) was silently discarded -- not bound in the request struct at all -- and never echoed on CreateConfigurationProfileOutput/GetConfigurationProfileOutput/UpdateConfigurationProfileOutput. A prior audit pass explicitly considered this and concluded 'no honest value to put here' (see ListHostedConfigurationVersions/GetDeployment notes below, now corrected); that reasoning conflated KmsKeyIdentifier (a caller-supplied string, trivially echoable) with KmsKeyArn (which genuinely does require unavailable KMS-ARN resolution and correctly stays unmodeled). KmsKeyIdentifier is now accepted, stored, and echoed on Create/Get/Update; KmsKeyArn remains absent."}
  GetConfigurationProfile: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (bd gopherstack-6flj): now echoes KmsKeyIdentifier -- see CreateConfigurationProfile note."}
  ListConfigurationProfiles: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-xs7l: was raw-marshaling the full ConfigurationProfile domain struct; now emits types.ConfigurationProfileSummary (types.go:193, deserializers.go:12061) via a dedicated configurationProfileToSummary -- dropped Description/RetrievalRoleArn/the full Validators list (3 leaked members), and added ValidatorTypes (a real Summary member that was simply never emitted -- derived honestly from each Validators[i].Type, an already-stored field). 2026-08-29 wrapper-key sweep: REQUEST direction verified against appconfig@v1.48.4 serializers.go. type query filter (serializers.go:2700) was never read -- always returned every profile regardless of type; ConfigurationProfile.Type already existed as a backing field, now wired through a new profileType param on the interface method (call sites updated)."}
  UpdateConfigurationProfile: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (bd gopherstack-6flj): KmsKeyIdentifier is now accepted (nil-means-unchanged, matching every other optional *string member here) and echoed -- see CreateConfigurationProfile note."}
  DeleteConfigurationProfile: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED — same ExtensionAssociation + deployedConfigs cascade-cleanup. FIXED 2026-09-07 (bd gopherstack-kpvs): real DeleteConfigurationProfileInput.DeletionProtectionCheck (api_op_DeleteConfigurationProfile.go, bound to the 'X-Amzn-Deletion-Protection-Check' request header per serializers.go:1121) was never read by this handler at all -- now parsed and validated against the types.DeletionProtectionCheck enum (BYPASS|APPLY|ACCOUNT_DEFAULT); an unrecognized value now gets BadRequestException instead of being silently accepted. NOT fixed, disclosed: actually enforcing the check (blocking a delete when APPLY/ACCOUNT_DEFAULT and the resource was recently read via appconfigdata GetLatestConfiguration) -- see the deletion_protection_check gap entry below for why that half is structurally out of reach right now."}
  CreateHostedConfigurationVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED — the previously-ignored optional 'Latest-Version-Number' request header (an optimistic-concurrency check: real CreateHostedConfigurationVersionInput.LatestVersionNumber must match the profile's current latest version or the SDK client expects a conflict) is now parsed and validated; a stale value now returns ConflictException instead of silently racing another writer. httpPayload response-body/header split (Application-Id/Configuration-Profile-Id/Content-Type/Description/VersionLabel/Version-Number headers, raw content body) verified against deserializers.go, matching the prior audit pass."}
  GetHostedConfigurationVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  ListHostedConfigurationVersions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-xs7l: was raw-marshaling the full HostedConfigurationVersion domain struct; now emits types.HostedConfigurationVersionSummary (types.go:610, deserializers.go:13825) via a dedicated hostedConfigurationVersionToSummary -- dropped CreatedAt (Get-only, 1 leaked member). KmsKeyArn is a real Summary member too, but this backend has no honest source for it (genuine ARN resolution is unavailable) -- stays absent rather than fabricated, same rationale as personalize's undocumented FailureReason members (gopherstack-sm02). CORRECTED (bd gopherstack-6flj): this note previously also claimed CreateConfigurationProfile doesn't accept a KmsKeyIdentifier as the reason KmsKeyArn couldn't be modeled; that premise was itself the bug -- KmsKeyIdentifier is now accepted and echoed on the ConfigurationProfile family (see CreateConfigurationProfile), it was simply never wired up. Only KmsKeyArn (the resolved-ARN member) remains genuinely unavailable here."}
  DeleteHostedConfigurationVersion: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDeploymentStrategy: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (bd gopherstack-lcan): same inline-Tags-dropped bug/fix as CreateApplication."}
  GetDeploymentStrategy: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDeploymentStrategies: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDeploymentStrategy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDeploymentStrategy: {wire: ok, errors: ok, state: ok, persist: ok, note: "misspelled /deployementstrategies/{Id} DELETE URI (real AWS typo, hard-coded in the SDK serializer) matched correctly."}
  StartDeployment: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED 2026-08-29 (bd gopherstack-6flj/21my continuation): real StartDeploymentInput (appconfig@v1.48.4 api_op_StartDeployment.go) had three real members this handler's request struct did not bind at all -- Tags (inline tags, applied to the deployment's own ARN via the new deploymentArn helper; StartDeployment was NOT one of the six ops fixed under bd gopherstack-lcan despite also accepting inline Tags -- see TestStartDeploymentViaSDKClient_TagsKmsKeyIdentifierLatestDeploymentNumber in wire_field_fixes_test.go), KmsKeyIdentifier (a per-deployment override of the profile's own stored KmsKeyIdentifier -- previously only ever the profile's value was used, silently discarding a caller override), and LatestDeploymentNumber (an optimistic-concurrency check identical in shape to CreateHostedConfigurationVersion's already-fixed latestVersionNumber -- a stale value now returns ConflictException instead of silently racing another writer). DeleteApplication's cascade-delete now also cleans up deployment tags (previously deployments had no ARN/tags at all). NOT fixed, disclosed instead: DynamicExtensionParameters (real StartDeploymentInput member, 'passed to associated extensions with PRE_START_DEPLOYMENT actions') is parsed nowhere and has no honest sink -- this backend does not simulate extension-action execution (same rationale as DeploymentEvent.ActionInvocations/AppliedExtensions being empty, and the pre-existing DeploymentParameters-on-experiment-ops precedent) -- see gaps below. FIXED (major) — two real bugs closed: (1) ConfigurationVersion was never validated against an actual HostedConfigurationVersion for AppConfig-hosted profiles (LocationUri=='hosted'); a real client got a 201 for a deployment referencing a version that never existed. Now resolved via resolveHostedConfigVersion (accepts version number OR label, matching real semantics) and rejected with ResourceNotFoundException when unresolvable — non-hosted profiles (SSM/S3/...) are intentionally NOT validated since this backend has no way to check the external source. (2) Deployments completed synchronously (State=COMPLETE immediately) regardless of the strategy's DeploymentDurationInMinutes/FinalBakeTimeInMinutes, so a real client's StartDeploymentOutput.State/PercentageComplete/EventLog/GrowthType/GrowthFactor/DeploymentDurationInMinutes/FinalBakeTimeInMinutes/VersionLabel/AppliedExtensions were either zero-valued or wrong. A zero-duration, zero-bake strategy (e.g. AppConfig.AllAtOnce) still completes synchronously (matches real AWS: no growth curve to run), but any other strategy now genuinely progresses DEPLOYING -> [BAKING] -> COMPLETE via a compressed-time background reconciler (see deployments.go's package doc comment for why real minute-scale durations are simulated on a millisecond timescale, mirroring the precedent already set by services/rds and services/acm). EventLog now records DEPLOYMENT_STARTED / PERCENTAGE_UPDATED / BAKE_TIME_STARTED / DEPLOYMENT_COMPLETED events, most-recent-first, matching real AWS ordering. AppliedExtensions is populated from real ExtensionAssociations targeting the app/env/profile ARNs at start time."}
  GetDeployment: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED — GetDeploymentOutput's AppliedExtensions/ConfigurationName/ConfigurationLocationUri/DeploymentDurationInMinutes/EventLog/FinalBakeTimeInMinutes/GrowthFactor/GrowthType/VersionLabel fields were entirely absent from the Deployment struct (always zero-valued on a real client) — all now populated. CORRECTED (bd gopherstack-6flj): this note previously claimed KmsKeyIdentifier was an acceptable unmodeled gap alongside KmsKeyArn; that premise was the bug (see CreateConfigurationProfile note) -- KmsKeyIdentifier is now snapshotted from the deployed profile at StartDeployment time, same as ConfigurationName/ConfigurationLocationUri. KmsKeyArn (the resolved-ARN member) remains genuinely unavailable."}
  ListDeployments: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-xs7l: CORRECTED — this entry previously argued the same (superset) Deployment shape as GetDeployment was fine because extra fields are harmless (real deserializers ignore unknown JSON keys). The premise is true but the conclusion was wrong: types.DeploymentSummary (types.go:329, deserializers.go:12583) is a real, narrower type distinct from GetDeploymentOutput, so emitting the full Deployment struct was a genuine wire-shape lie regardless of SDK-client tolerance -- a raw-body or non-SDK caller sees the leak. Now emits DeploymentSummary via a dedicated deploymentToSummary -- dropped ApplicationId/EnvironmentId/DeploymentStrategyId/Description/ConfigurationLocationUri/EventLog/AppliedExtensions (7 leaked members). Type is a real DeploymentSummary member that GetDeploymentOutput's own shape lacks entirely and was never emitted -- always deploymentTypeUser ('USER') here, since every Deployment this backend creates comes from StartDeployment (there is no MANAGED/AppConfig-initiated deployment path anywhere in this service), making the constant an honest structural fact, not a fabricated per-instance value."}
  StopDeployment: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (major) — real StopDeploymentInput.AllowRevert (bound to the 'Allow-Revert' request header, not a body/query field) was not modeled at all: any call, including on an already-COMPLETE deployment, was unconditionally accepted and force-set to ROLLED_BACK. Now: (1) AllowRevert is parsed from the real header; (2) a non-terminal deployment (BAKING/DEPLOYING/VALIDATING) stops to ROLLED_BACK as before; (3) a COMPLETE deployment can ONLY be stopped via AllowRevert=true, moving it to REVERTED and reverting deployedConfigs to the previous COMPLETE deployment's ConfigurationVersion for that environment/profile (or clearing it if there was none) — previously a COMPLETE deployment could be silently rolled back with no AllowRevert check at all, and GetConfiguration/CurrentDeployedConfiguration would still have served the (self-)deployed version. StopDeployment on a COMPLETE deployment without AllowRevert now correctly returns BadRequestException. FIXED (major, separate bug, bd gopherstack-6flj): the handler returned 204 No Content with an empty body; the real op returns 200 with a full StopDeploymentOutput body (every Deployment field, api_op_StopDeployment.go) that this audit's own wire:ok rating never verified. A real client tolerates the empty body silently (json.Decoder treats io.EOF as 'no document', not an error) and decodes every field to its zero value -- State/DeploymentNumber/PercentageComplete/etc. all came back blank/0 despite the stop having actually happened server-side. Now returns 200 with the full post-stop Deployment."}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateExtension: {wire: fixed, errors: ok, state: ok, persist: ok, note: "creates version 1 of a versioned resource — see the family-wide versioning note under GetExtension. FIXED THIS PASS (bd gopherstack-lcan): same inline-Tags-dropped bug/fix as CreateApplication (tags applied to the extension's own Arn). ALSO FIXED (bd gopherstack-6flj): real types.Parameter.Dynamic (deserializers.go, shared by every Parameters map[string]Parameter member across Create/UpdateExtensionInput and Get/CreateExtensionOutput) was entirely unmodeled on ExtensionParameter -- silently discarded on input, never emitted on output. Now present."}
  GetExtension: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (major, closes prior gap) — extensions are versioned resources in real AWS AppConfig: GetExtensionInput's optional 'version_number' query param must resolve a SPECIFIC historical version, not always 'whatever is current'. This backend previously stored Extension as one mutable record overwritten in place by every UpdateExtension, so version_number was always ignored and prior versions were unrecoverable. The extensions table is now keyed by composite (extensionID, versionNumber); UpdateExtension inserts a new row instead of mutating, and GetExtension honors an explicit version_number or defaults to the highest version (matching 'If no version number was defined, AppConfig uses the highest version'). ALSO FIXED (bd gopherstack-6flj): Parameter.Dynamic now round-trips -- see CreateExtension note."}
  ListExtensions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED — DELETED the gopherstack-invented 'extension_version_number' filter parameter: real ListExtensionsInput has no version filter at all (confirmed via api_op_ListExtensions.go), and a real SDK client can never send it. ListExtensions now summarizes one row per distinct extension ID at its latest version, matching real AWS (there is no ListExtensionVersions API). gopherstack-xs7l: was ALSO raw-marshaling the full Extension domain struct on top of that; now emits types.ExtensionSummary (types.go:574, deserializers.go:13700) via a dedicated extensionToSummary -- dropped Actions/Parameters (2 leaked members, so ExtensionSummary itself never carries Dynamic either way)."}
  UpdateExtension: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED — now creates a new, independently addressable version (VersionNumber = latest+1) rather than mutating the existing record in place, so a prior version remains gettable via GetExtension?version_number=N after an update, matching real AWS. ALSO FIXED (bd gopherstack-6flj): Parameter.Dynamic now round-trips -- see CreateExtension note."}
  DeleteExtension: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (major, closes prior gap) — DeleteExtensionInput's optional 'version' query param now deletes ONLY that specific version (or the highest version, if omitted — matching 'If omitted, the highest version is deleted', NOT a full wipe of every version as the pre-fix single-record model implicitly did). Deleting an extension's last remaining version removes the extension (and its tags) entirely. Also FIXED: deleting a version still referenced by an ExtensionAssociation now returns ConflictException instead of silently succeeding and leaving the association pointing at a deleted extension version."}
  CreateExtensionAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "explicit ExtensionVersionNumber is now validated to actually exist (returns ResourceNotFoundException if not); previously any integer was accepted uncritically. FIXED THIS PASS (bd gopherstack-lcan): same inline-Tags-dropped bug/fix as CreateApplication (tags applied to the association's own Arn)."}
  GetExtensionAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  ListExtensionAssociations: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-xs7l: was raw-marshaling the full ExtensionAssociation domain struct; now emits types.ExtensionAssociationSummary (types.go:556, deserializers.go:13608) via a dedicated extensionAssociationToSummary -- dropped Arn/Parameters/ExtensionVersionNumber (3 leaked members). 2026-08-29 wrapper-key sweep: REQUEST direction verified. extension_version_number query filter (serializers.go:3282) was never read -- always returned every association regardless of version; ExtensionAssociation.ExtensionVersionNumber already existed as a backing field, now wired through a new extensionVersionNumber param on the interface method (call sites updated). FIXED 2026-08-29 (gopherstack-21my, parameter-honoring sweep) -- ExtensionIdentifier is documented 'The name, the ID, or the Amazon Resource Name (ARN) of the extension' (api_op_ListExtensionAssociations.go), but the backend compared the raw request value straight against ExtensionAssociation.ExtensionArn, so a client filtering by the extension's name or ID (not its ARN) silently got zero results instead of the matching association. Root cause was shared, wider infrastructure: resolveExtensionID (extensions.go) only resolved by ID or name, never ARN -- the same gap also affected CreateExtensionAssociation, GetExtension, UpdateExtension, and DeleteExtension's own ExtensionIdentifier parameter (all documented name/ID/ARN), confirmed by CreateExtensionAssociation failing outright with a 404 when given an ARN in a hand-written repro. Fixed at the shared resolver so all of the above benefit, then ListExtensionAssociations' filter now resolves ExtensionIdentifier to the canonical ARN before comparing. See TestListExtensionAssociationsFilter_ByNameAndID (list_filter_params_test.go), real SDK client round trip, confirmed failing pre-fix."}
  UpdateExtensionAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteExtensionAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  GetAccountSettings: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED THIS PASS (bd gopherstack-6flj): real GetAccountSettingsOutput has a second top-level member, VendedMetrics (types.VendedMetricsSettings{Enabled}, api_op_GetAccountSettings.go), entirely unmodeled alongside DeletionProtection -- now present."}
  UpdateAccountSettings: {wire: ok, errors: ok, state: fixed, persist: ok, note: "FIXED (bd gopherstack-6flj): real UpdateAccountSettingsInput.VendedMetrics was silently discarded (not bound in the request struct) -- see GetAccountSettings note. Now accepted and applied, same nil-means-unchanged semantics as DeletionProtection. 2026-08-21 (gopherstack-c8ge): DeletionProtection is a singleton with no Create op; DeletionProtectionSettings{Enabled,ProtectionPeriodInMinutes} are both independently-optional pointers on the real input, but the handler swapped the whole sub-struct pointer wholesale, so an Update naming only Enabled wiped a previously-set ProtectionPeriodInMinutes. Fixed to merge field by field. See TestHandler_UpdateAccountSettings_DeletionProtectionFieldsSurviveIndependentUpdates."}
  GetConfiguration: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (major) — real GetConfiguration ('Retrieves the latest DEPLOYED configuration', deprecated) was actually implemented as 'return the highest-numbered HostedConfigurationVersion ever created for this profile', completely ignoring environment/deployment state — a real client would see content that was uploaded via CreateHostedConfigurationVersion but never deployed to that environment, and creating a newer hosted version would change what GetConfiguration returned even with zero deployments. Now backed by a real deployedConfigs map updated only when a deployment reaches COMPLETE (see StartDeployment/StopDeployment notes), correctly returning empty content until an actual deployment has completed and the correct version thereafter. deployedConfigs is cascade-cleaned on DeleteApplication/DeleteEnvironment/DeleteConfigurationProfile and persisted (survives Snapshot/Restore). 2026-08-29 wrapper-key sweep: REQUEST direction verified. client_configuration_version query param (api_op_GetConfiguration.go:89,101-104) was never read -- a matching value must return 204 with empty Content instead of resending the same data; now does. client_id remains a gap: real AWS uses it to consistently bucket a given client into old-vs-new config during a percentage-based gradual rollout, and this backend has no weighted per-client rollout model to bucket against -- not fabricated."}
  ValidateConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateExperimentDefinition: {wire: ok, errors: ok, state: ok, persist: ok, note: "field-diffed against CreateExperimentDefinitionInput/Output in api_op_CreateExperimentDefinition.go + types.Treatment(Input)/FlagValue/AttributeValue in types.go; POST /applications/{ApplicationIdentifier}/experimentdefinitions per serializers.go. ApplicationIdentifier/EnvironmentIdentifier/ConfigurationProfileIdentifier are resolved (ID or name) against real Application/Environment/ConfigurationProfile state via the pre-existing resolveAppID/resolveEnvID/resolveProfileID helpers (configuration.go) -- not accepted as any string. Additionally validates the referenced ConfigurationProfile.Type is AWS.AppConfig.FeatureFlags when Type was explicitly set (empty Type is treated as unspecified, not wrong, so pre-existing freeform-profile test fixtures are not retroactively broken). FIXED THIS PASS: FlagKey is now checked against the profile's actual feature-flag content (feature_flags.go), not merely non-empty, when the profile has any parseable AWS.AppConfig.FeatureFlags content uploaded -- matching FlagKey's own doc comment ('The key of the existing feature flag to use with the experiment'). Inline Tags are applied correctly (see tags_handling in the campaign return receipt) -- this op did NOT repeat the bd gopherstack-lcan inline-Tags-dropped bug the six pre-existing Create* handlers had (now fixed there too, see their ops entries above)."}
  GetExperimentDefinition: {wire: ok, errors: ok, state: ok, persist: ok, note: "resolves by ID or name within the application, matching real AWS's 'ID or name' ExperimentDefinitionIdentifier contract."}
  ListExperimentDefinitions: {wire: ok, errors: ok, state: ok, persist: ok, note: "account-wide GET /experimentdefinitions (query filters application_identifier/configuration_profile_identifier/environment_identifier/status/max_results/next_token) verified against api_op_ListExperimentDefinitions.go's httpBindings. gopherstack-xs7l: CORRECTED — this entry previously argued the full ExperimentDefinition shape was fine to return in place of a separate ExperimentDefinitionSummary DTO because extra fields are ignored by real deserializers. The premise is true but the conclusion was wrong: types.ExperimentDefinitionSummary (types.go:446, deserializers.go:13090) is a real, narrower type, so the superset response was a genuine wire-shape lie regardless of SDK-client tolerance. Now emits ExperimentDefinitionSummary via a dedicated experimentDefinitionToSummary -- dropped AudienceDescription/AudienceRule/Control/KmsKeyIdentifier/LaunchCriteria/Treatments (6 leaked members). FIXED 2026-08-23 (RAISED from partial): a configuration_profile_identifier/environment_identifier filter could previously only be resolved by NAME when application_identifier was also supplied -- without one, a name-form value was compared literally against the ID field only and matched nothing, even though ListExperimentDefinitionsInput documents all three identifiers as independently resolvable by ID or name (api_op_ListExperimentDefinitions.go). buildExperimentDefinitionFilterLocked now resolves configuration_profile_identifier/environment_identifier per-candidate (experimentDefinitionMatchesProfileLocked/experimentDefinitionMatchesEnvLocked, experiment_definitions.go), scoping each name resolution to that candidate's own ApplicationID instead of requiring one upfront. See TestListExperimentDefinitions_NameFormFilterWithoutApplicationIdentifier (list_experiment_definitions_name_filter_test.go)."}
  UpdateExperimentDefinition: {wire: ok, errors: ok, state: ok, persist: ok, note: "nil-means-unchanged semantics verified against optional *string/*Treatment/*[]Treatment UpdateExperimentDefinitionInput members (Name/ApplicationIdentifier/ExperimentDefinitionIdentifier are the only required members). Returns ConflictException when a RUNNING run exists for the definition, matching the real doc text 'You cannot update an experiment definition while an experiment run is active.'"}
  DeleteExperimentDefinition: {wire: ok, errors: ok, state: ok, persist: ok, note: "DeleteType query param ('delete_type', ARCHIVE|DESTROY per types/enums.go) verified against api_op_DeleteExperimentDefinition.go. ARCHIVE sets Status=ARCHIVED and preserves the definition/runs; DESTROY permanently removes the definition plus every run/event/tag scoped to it (cascade, no ghost rows). ASSUMPTION (unverified against real AWS, called out explicitly): when delete_type is omitted, this backend defaults to ARCHIVE -- the SDK documents no default for either identifier."}
  StartExperimentRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "field-diffed against StartExperimentRunInput/Output in api_op_StartExperimentRun.go; POST /applications/{ApplicationIdentifier}/experimentdefinitions/{ExperimentDefinitionIdentifier}/experimentruns. Only one RUNNING run per definition is allowed (ConflictException otherwise); starting a run against an ARCHIVED definition is rejected (BadRequestException). Moves the parent ExperimentDefinition.Status to ACTIVE. Inline Tags applied correctly to the run's own ARN (same lcan-avoidance as CreateExperimentDefinition). ASSUMPTION (unverified, called out explicitly): ExposurePercentage defaults to 0 when omitted -- the SDK documents no default, only that 'Set to 0 to validate the experiment before exposing production users' is a valid use, which this backend read as the safer default absent a confirmed value."}
  GetExperimentRun: {wire: ok, errors: ok, state: ok, persist: ok}
  ListExperimentRuns: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-xs7l: CORRECTED — this entry previously argued returning the full ExperimentRun shape (a superset of ExperimentRunSummary) was fine on the same harmless-extra-fields precedent as ListExperimentDefinitions/ListDeployments. The premise is true but the conclusion was wrong: types.ExperimentRunSummary (types.go:527, deserializers.go:13430) is a real, narrower type, so the superset response was a genuine wire-shape lie regardless of SDK-client tolerance. Now emits ExperimentRunSummary via a dedicated experimentRunToSummary -- dropped ApplicationId/ExperimentDefinitionSnapshot/ExposurePercentage/Result/TreatmentOverrides (5 leaked members). Status query-param filter verified against api_op_ListExperimentRuns.go."}
  UpdateExperimentRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "only permitted while the run is RUNNING (BadRequestException otherwise, real AWS doc: run must be active to update). ExposurePercentage can only increase, never decrease, matching 'This value can only be increased from the current setting' -- verified by rejecting any decrease with BadRequestException. TreatmentOverrides modeled as the real single-member union (types.TreatmentOverridesMemberInline, wire key 'Inline')."}
  StopExperimentRun: {wire: ok, errors: ok, state: ok, persist: ok, note: "PATCH .../experimentruns/{Run}/stop verified against api_op_StopExperimentRun.go. Only permitted while RUNNING (BadRequestException on an already-DONE run, matching real semantics -- no re-stop). Moves Status to DONE, sets EndedAt, reverts the parent ExperimentDefinition.Status to IDLE (no other run can be RUNNING -- StartExperimentRun enforces at most one). Optional Result (ExperimentRunResult) is stored and echoed back; see results_verdict in the campaign return receipt for why this backend never computes it itself."}
  ListExperimentRunEvents: {wire: ok, errors: ok, state: ok, persist: ok, note: "returns exactly the RUN_STARTED/EXPOSURE_UPDATED/OVERRIDES_UPDATED/RUN_STOPPED events this backend actually recorded during the run's lifecycle (experiment_runs.go's appendExperimentRunEventLocked, most-recent-first -- the same ordering convention as the pre-existing DeploymentEvent family), never a synthesized timeline. GET .../experimentruns/{Run}/events verified against api_op_ListExperimentRunEvents.go."}
families:
  route_matcher: {status: ok, note: "every op's REST path prefix + HTTP method verified against aws-sdk-go-v2/service/appconfig@v1.48.0's serializers.go SplitURI calls, including the 11 new experiment routes this pass (POST/GET/PATCH/DELETE under /applications/{id}/experimentdefinitions[/...] plus the account-wide GET /experimentdefinitions)."}
  persistence: {status: ok, note: "Handler.Snapshot/Restore delegate to InMemoryBackend; new deployedConfigs map added to the snapshot (nil-guarded on restore, matching every other raw map). deploymentTimers (in-flight deployment-progression state) is deliberately NOT persisted — see its doc comment in store.go and finalizeStaleDeploymentsLocked in deployments.go, which immediately completes any deployment restored in a non-terminal state rather than leaving it stuck forever with no timer to drive it. This pass additionally added experimentDefinitions/experimentRuns (store.Table-backed, byApp/byAppName/byDef indexes rebuilt on Restore) and the raw experimentRunEvents/experimentRunCounters maps (nil-guarded, same convention as versionCounters/deploymentCounters) -- verified round-trip by TestInMemoryBackend_SnapshotRestore_FullState in persistence_test.go, extended this pass to seed and assert an experiment definition + run + its event history survives Snapshot/Restore, that byAppName/experimentRunCounters were rebuilt (not left stale), and that DeleteApplication's cascade delete now removes experiment definitions too."}
  experiment_family: {status: ok, note: "11 new ops (CreateExperimentDefinition, GetExperimentDefinition, ListExperimentDefinitions, UpdateExperimentDefinition, DeleteExperimentDefinition, StartExperimentRun, GetExperimentRun, ListExperimentRuns, UpdateExperimentRun, StopExperimentRun, ListExperimentRunEvents) added the prior pass -- AppConfig's A/B-testing surface, shipped since the v1.43.11 audit. Real state machine: ExperimentDefinition.Status transitions IDLE -> ACTIVE (a run starts) -> IDLE (the run stops) or -> ARCHIVED (DeleteExperimentDefinition, delete_type=ARCHIVE); ExperimentRun.Status transitions RUNNING -> DONE (StopExperimentRun only; no automatic timer-driven progression, unlike Deployment's growth curve -- a run has no duration to progress through, it just stays RUNNING until stopped). ListExperimentRunEvents returns exactly the RUN_STARTED/EXPOSURE_UPDATED/OVERRIDES_UPDATED/RUN_STOPPED events this backend actually recorded, never a fabricated timeline. References (ApplicationIdentifier/EnvironmentIdentifier/ConfigurationProfileIdentifier) are validated against real backend state via the pre-existing resolveAppID/resolveEnvID/resolveProfileID helpers, plus a ConfigurationProfile.Type==AWS.AppConfig.FeatureFlags check. RAISED to ok this pass: FlagKey is now validated against real feature-flag content (feature_flags.go) when the profile has any uploaded, closing the one gap in this family that was an actual backend shortcoming rather than an unverifiable-against-the-SDK assumption -- see the 'overall' grade note above and gaps below for the three assumption-class items (ExposurePercentage/delete_type defaults, Treatment.Key's naming scheme) that remain disclosed but no longer hold status at partial, since a documented absence of a default is not the same class of gap as unmodeled behavior."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  # bd gopherstack-lcan (six pre-existing Create* handlers' inline Tags silently dropped) FIXED
  # 2026-07-25 -- see the CreateApplication/CreateEnvironment/CreateConfigurationProfile/
  # CreateDeploymentStrategy/CreateExtension/CreateExtensionAssociation ops notes above. Tags are now
  # applied directly to b.tags at creation time (not via TagResource, to avoid re-entrant locking --
  # same pattern CreateExperimentDefinition/StartExperimentRun already used), and each handler's
  # request struct now binds Tags. Table-test coverage added per handler
  # (TestHandler_Create*_TagsAppliedInline) proving tags set at create are visible via
  # ListTagsForResource, so this class of regression is now caught.
  - "Deployment progression (StartDeployment's DEPLOYING/BAKING growth curve) runs on a fixed compressed timescale (single-digit milliseconds per step, clamped GrowthFactor) rather than being proportional to the strategy's actual configured DeploymentDurationInMinutes/FinalBakeTimeInMinutes -- e.g. a 1-minute strategy and a 1440-minute strategy complete in comparable wall-clock time. This is a deliberate, documented simplification (see deployments.go's package doc comment) matching the precedent set by services/rds and services/acm for the same reason (real AWS timings are impractical to emulate literally in a test-driven in-memory backend); not something a client can observe via any single API call, only via wall-clock timing across polls."
  - "StartExperimentRun's ExposurePercentage default (when the optional field is omitted) is UNVERIFIABLE against real AWS -- the SDK's ExposurePercentage doc text ('Set to 0 to validate the experiment before exposing production users') implies 0 is a meaningful value but never states it is the default for an omitted field, and the SDK ships no default for this field at all (re-confirmed 2026-07-30). This backend defaults to 0 (the safer, least-surprising reading: no audience exposed without an explicit non-zero value) rather than fabricate a different unverified number. A real client that always sends ExposurePercentage explicitly is unaffected; one that omits it may observe a different default than real AWS. A disclosed assumption, not a backend bug -- does not by itself hold the grade below A."
  - "DeleteExperimentDefinition's delete_type default (when omitted) is UNVERIFIABLE against real AWS -- DeleteType's doc text describes ARCHIVE as 'hide but preserve' and DESTROY as the explicit opt-in to permanent removal, but the SDK documents no default for an omitted value (re-confirmed 2026-07-30). This backend defaults to ARCHIVE (the non-destructive choice) rather than assume irreversible deletion was intended. A real client that always sends delete_type explicitly is unaffected. A disclosed assumption, not a backend bug -- does not by itself hold the grade below A."
  - "Treatment.Key's server-generated naming scheme ('Control' for the control treatment, 'Treatment1'..'TreatmentN' 1-indexed by creation order for the rest) is UNVERIFIABLE against real AWS: real CreateExperimentDefinitionInput/UpdateExperimentDefinitionInput's TreatmentInput has no client-supplied Key at all (re-confirmed 2026-07-30), so AWS itself must assign one, but the exact scheme AWS uses is not documented anywhere in the SDK. A real client that treats Key as an opaque server-assigned identifier (which is the only documented contract) is unaffected; one that asserts an exact Key string may see a different value than real AWS. A disclosed assumption, not a backend bug -- does not by itself hold the grade below A."
  - "DeploymentParameters (accepted on StartExperimentRun/StopExperimentRun/UpdateExperimentRun) is parsed but intentionally discarded rather than stored or acted upon -- real GetExperimentRun/StartExperimentRun/etc. output shapes never echo it back either, so a real client observes nothing different; but this backend also does not create the underlying 'real' deployment AWS uses internally to actually serve treatment variations to production traffic, so DynamicExtensionParameters/Tags on that inner deployment have no addressable resource here to apply to."
  - "StartDeploymentInput.DynamicExtensionParameters (real member, api_op_StartDeployment.go: 'a map of dynamic extension parameter names to values to pass to associated extensions with PRE_START_DEPLOYMENT actions') is accepted but has no honest sink to write to -- this backend does not simulate real extension-action execution (Lambda invocation, SNS/SQS/EventBridge notification, ...), matching the pre-existing DeploymentEvent.ActionInvocations/Deployment.AppliedExtensions-content rationale and the already-disclosed DeploymentParameters-on-experiment-ops gap above. A real client observes no difference since no GetDeployment/StartDeployment output shape echoes this field back either."
  - "KmsKeyArn (ConfigurationProfile/HostedConfigurationVersionSummary/Deployment's Get/Create/Update outputs) remains unmodeled -- unlike KmsKeyIdentifier (a caller-supplied string, now correctly accepted/echoed as of bd gopherstack-6flj, see CreateConfigurationProfile), KmsKeyArn requires resolving that identifier to a real KMS key ARN, which this backend has no KMS integration to do honestly. Left absent rather than fabricated."
  - "deletion_protection_check (bd gopherstack-kpvs, updated 2026-09-07; originally found 2026-09-04 bd gopherstack-5pl): DeleteEnvironmentInput/DeleteConfigurationProfileInput.DeletionProtectionCheck (BYPASS|APPLY|ACCOUNT_DEFAULT, bound to the 'X-Amzn-Deletion-Protection-Check' request header, serializers.go:1121 and :1268) is a real request member. The title-only bd issue conflated two claims that turned out to have different answers: (1) 'the header is never read' -- TRUE and independently fixable, now FIXED: both ops parse the header and reject a value outside the enum with BadRequestException (rejectInvalidDeletionProtectionCheck, handler.go), covered by TestHandler_Delete{Environment,ConfigurationProfile}_DeletionProtectionCheck. (2) 'full enforcement needs appconfigdata recency tracking plus a reverse hook back to appconfig' -- separately verified TRUE and NOT built, deliberately: DeletionProtectionSettings' own doc comment (types.go:222-247) states the check fires when 'AppConfig has called [GetLatestConfiguration] for the configuration profile or from the environment during the specified interval' -- i.e. the trigger condition is recent appconfigdata GetLatestConfiguration call history, not resource age or account settings alone. appconfigdata DOES track per-session recency (Session.LastAccessedAt, models.go:94/111, set on every GetLatestConfiguration call in configuration.go:196) but that state is private to appconfigdata's own store/interfaces -- nothing in either service imports the other's backend, and every services/*/provider.go Init() wires its own backend in isolation with no registry handle to look another service up by name, so there is no existing hook pattern to reuse; building one is new cross-service architecture, out of scope for this fix. AccountSettings.DeletionProtection.Enabled defaults to nil/unset (account_settings.go, store.go:125's zero-value AccountSettings) and CreatedAt IS tracked on both Environment and ConfigurationProfile (models.go), so the 'skip resources created in the past hour' half of the doc'd behavior COULD be modeled honestly -- but wiring only that half while the actual trigger (access recency) is always false is observably identical to today's behavior (delete is never blocked either way), so there is nothing to fix that would change any test-observable output; implementing a blanket 'block whenever Enabled=true' policy instead would be fabrication in the wrong direction (real AWS does NOT block deletion of a never-accessed resource merely because deletion protection is enabled). Left unenforced rather than fabricated, same class as the pre-existing GetConfiguration client_id gap above."
deferred:                 # consciously not audited this pass (scope) — next pass targets
  - "GetExtensionInput/DeleteExtensionInput document 'name, ID, or ARN' identifier resolution; this backend's resolveExtensionID only resolves by ID or name (pre-existing, unchanged this pass) -- ARN-based lookup was not added. Low risk: gopherstack conventionally addresses resources by ID/name elsewhere in this service too."
leaks: {status: clean, note: "FIXED — DeleteApplication/DeleteEnvironment/DeleteConfigurationProfile previously left ExtensionAssociation rows referencing deleted app/env/profile ARNs as ghosts (unbounded growth under repeated create/delete cycles); all three now cascade-delete associations targeting the resource being removed, plus deployedConfigs tracking entries. The new deploymentTimers map (in-flight deployment progression) and its background reconciler goroutine are self-draining/self-terminating (same ephemeral-goroutine pattern as services/rds's lifecycle reconciler): TestDeploymentTimers_DrainToZero (leak_test.go) verifies the map returns to empty once every deployment reaches a terminal state, at which point the goroutine exits on its own -- no ctx-parenting or explicit Shutdown drain is needed since nothing outlives the deployments that scheduled it. leak_test.go's pre-existing NameIndexBounded tests (Application/Extension/DeploymentStrategy) still pass under -race. This pass additionally verified (TestBackend_DeleteApplication_CascadesExperimentDefinitions, TestBackend_DeleteExperimentDefinition_DestroyCascadesRunsAndTags) that DeleteApplication and DeleteExperimentDefinition(delete_type=DESTROY) both cascade-remove every experiment run/event/tag scoped to the definition being removed -- no ghost rows survive either deletion path."}
---

## Notes

**2026-08-13 (gopherstack-jqh2 pass 3):** re-extracted all 56 ops' real
method+path directly from `appconfig@v1.48.4` serializers.go and drove them
through `ExtractOperation` via the new `handler_sdk_route_table_test.go`
(`TestExtractOperation_SDKRouteTable`, one subtest per op, `t.Parallel()`).
Confirmed the real AWS `DeleteDeploymentStrategy` path typo
(`/deployementstrategies/{Id}`, extra "e", every sibling op uses
`/deploymentstrategies`) and the account-wide (non-app-nested)
`ListExperimentDefinitions` path were both already correctly handled with
doc comments in handler.go. One test-construction wrinkle, not a service
bug: `DeploymentNumber`, `VersionNumber`, and `Run` are wire-serialized as
integers (`encoder.SetURI(...).Integer(...)`, not `.String()`), and this
handler's route parser requires them to `strconv.ParseInt` to resolve
`GetDeployment`/`StopDeployment`, `Get/DeleteHostedConfigurationVersion`,
and the four Run-numbered experiment-run ops — a non-numeric placeholder in
that position resolves to `Unknown`, which a real client can never trigger
since the SDK always sends a real integer there. Table uses a numeric
literal for those 8 entries instead of the generic PLACEHOLDER. No
pre-existing table existed to check, and no real routing bugs found. This
test is now the permanent regression guard for route-table drift.

Protocol: restjson1 (REST paths + JSON bodies), like the rest of the newer AWS services.
Two response operations are httpPayload-based rather than JSON-bodied: **CreateHostedConfigurationVersion**
and **GetHostedConfigurationVersion** both return the raw configuration content as the response body, with
every other field — including the version number — bound to a response header (`Application-Id`,
`Configuration-Profile-Id`, `Content-Type`, `Description`, `Versionlabel`, `Version-Number`). See
`setHostedConfigurationVersionHeaders`'s doc comment in handler_hosted_configuration_versions.go.

**Extensions are versioned resources.** Every `UpdateExtension` call in real AWS AppConfig produces a new,
independently addressable version rather than mutating the extension in place — `GetExtension`/
`DeleteExtension` both accept an optional version number (`version_number` / `version` query params
respectively) that must resolve a *specific* historical version, defaulting to the highest when omitted.
This backend's `extensions` table is keyed by the composite `(extensionID, versionNumber)` (see
`extensionVersionKey` in store.go) rather than by ID alone; `extensionsByID` groups every version of one
extension for latest-version lookup and cascade operations, while `extensionsByName` answers name-based
identifier resolution and the create-time name-uniqueness check. Any future extension-family change must
preserve this shape — collapsing back to "one mutable record per extension" reintroduces the exact bug this
pass closed.

**Deployment state machine.** `StartDeployment` now genuinely progresses `DEPLOYING` -> (`BAKING` if the
strategy's `FinalBakeTimeInMinutes` > 0) -> `COMPLETE` for any strategy with a non-zero
`DeploymentDurationInMinutes`, via a background reconciler goroutine (`scheduleDeploymentReconcilerLocked`
in deployments.go) that advances every in-flight deployment's `PercentageComplete` per its
`GrowthType`/`GrowthFactor` on a **compressed** timescale — see the package doc comment at the top of
deployments.go for why real minute-scale durations are simulated in milliseconds, and
`effectiveGrowthFactor`'s clamp for why worst-case test runtime stays bounded regardless of a strategy's
configured `GrowthFactor`. A zero-duration, zero-bake strategy (e.g. `AppConfig.AllAtOnce`) still completes
synchronously, matching real AWS (no growth curve to run). `StopDeployment` now honors the real
`AllowRevert` header: a non-terminal deployment stops to `ROLLED_BACK`; a `COMPLETE` deployment can *only*
be stopped via `AllowRevert=true`, moving to `REVERTED` and rolling `deployedConfigs` back to the previous
`COMPLETE` deployment's version. `EventLog` is recorded most-recent-first, matching real AWS ordering.
`deploymentTimers` (the in-flight progression bookkeeping) is intentionally not persisted — see
`finalizeStaleDeploymentsLocked`'s doc comment for what happens to a deployment restored mid-flight.

**`GetConfiguration` / `CurrentDeployedConfiguration` now track real deployment state**, not "the
highest-numbered hosted version ever created." A `deployedConfigs` map (keyed by
application/environment/profile) is updated only when a deployment reaches `COMPLETE`, and rolled back on a
`StopDeployment(..., allowRevert=true)` revert. `CurrentDeployedConfiguration` (configuration.go) is a
**public read accessor with no caller inside this package** — it exists for a future
`appconfig` -> `appconfigdata` bridge (see bd `gopherstack-uiyi`: appconfigdata's config store is never
populated by a real deployment today). `cli.go` wiring to actually call it on deployment completion is out
of scope for this change; adding the accessor itself is the in-scope half of closing that cross-service gap.

**Cascade-delete**: `DeleteApplication`/`DeleteEnvironment`/`DeleteConfigurationProfile` now also remove
`ExtensionAssociation` rows targeting the ARN being deleted (previously left as ghost rows pointing at a
resource that no longer exists — an unbounded-growth leak under repeated create/delete cycles in a
long-running process) and `deployedConfigs` tracking entries for the deleted app/env/profile.

`DeleteDeploymentStrategy` alone uses `/deployementstrategies/{Id}` (missing the second "n") — a genuine AWS
typo baked into the real SDK's serializer, not a gopherstack bug; the route matcher already special-cases
this correctly (unchanged this pass).

CLOSED 2026-08-13: this backend added `CreatedAt`/`UpdatedAt` fields to `Application`/`Environment`/
`DeploymentStrategy` JSON responses that don't exist in the real AWS shapes. Evidence:
`aws-sdk-go-v2/service/appconfig@v1.48.4`, `types/types.go`, checked 2026-08-13 — `types.Application` has
`Description`/`Id`/`Name` only; `types.Environment` has `ApplicationId`/`Description`/`Id`/`Monitors`/`Name`/
`State` only; `types.DeploymentStrategy` has `DeploymentDurationInMinutes`/`Description`/
`FinalBakeTimeInMinutes`/`GrowthFactor`/`GrowthType`/`Id`/`Name`/`ReplicateTo` only — none declare
`CreatedAt`/`UpdatedAt`. Unlike the other single-field deletes this same sweep found elsewhere, a bare
delete wasn't safe here: `Application`/`Environment`/`DeploymentStrategy` are the same structs
`store.Table`'s `Snapshot`/`Restore` JSON-marshals directly (see `backendSnapshot`'s doc comment in
persistence.go — "none of them need a DTO wrapper"), so blanking the JSON tag would have silently dropped
both fields across every snapshot/restore cycle. Fixed instead with a converter per type
(`applicationToOutput`/`environmentToOutput`/`deploymentStrategyToOutput`, each next to its backend file) —
the domain struct keeps its `CreatedAt`/`UpdatedAt` JSON tags for persistence, and Create/Get/Update/List
handlers now serialize the converted `*Output` type instead of the raw domain struct. Raw-body regression
tests: `TestHandler_ListOps_SummaryShape`'s new `applications`/`environments`/`deploymentstrategies` cases
and `TestHandler_GetOps_NoCreatedAtUpdatedAt` (`handler_list_summary_test.go`).

**Experiment family (new this pass, SDK bumped v1.43.11 -> v1.48.0).** AppConfig's A/B-testing surface: an
`ExperimentDefinition` (a treatment plan attached to a feature-flag `ConfigurationProfile`) can be run
zero or more times as an `ExperimentRun`. `ApplicationIdentifier`/`EnvironmentIdentifier`/
`ConfigurationProfileIdentifier` are all resolved (ID or name) against real backend state via the
pre-existing `resolveAppID`/`resolveEnvID`/`resolveProfileID` helpers (`configuration.go`) — an
experiment can never be created against an application/environment/profile that does not exist, and the
referenced profile must be `AWS.AppConfig.FeatureFlags`-typed when its `Type` was explicitly set.
`ExperimentDefinition.Status` transitions `IDLE` -> `ACTIVE` when a run starts, back to `IDLE` when it
stops (`StartExperimentRun` allows only one `RUNNING` run per definition at a time — a second concurrent
`StartExperimentRun` is rejected with `ConflictException`), or -> `ARCHIVED` via
`DeleteExperimentDefinition(delete_type=ARCHIVE)`; `delete_type=DESTROY` instead permanently removes the
definition plus every run/event/tag scoped to it (cascade, verified leak-free —
`TestBackend_DeleteExperimentDefinition_DestroyCascadesRunsAndTags`). Unlike `Deployment`'s
compressed-timescale growth curve, `ExperimentRun` has **no automatic progression**: a run has no
duration to advance through, it simply stays `RUNNING` — at the configured `ExposurePercentage` — until a
caller calls `StopExperimentRun` (-> `DONE`) or `UpdateExperimentRun` (exposure/overrides/description,
RUNNING-only, exposure can only increase). `ListExperimentRunEvents` returns exactly the
`RUN_STARTED`/`EXPOSURE_UPDATED`/`OVERRIDES_UPDATED`/`RUN_STOPPED` events this backend actually recorded
as those calls happened (`experiment_runs.go`'s `appendExperimentRunEventLocked`, most-recent-first —
same ordering convention as `DeploymentEvent`), never a fabricated or interpolated timeline.
`DeleteApplication` now also cascade-deletes every `ExperimentDefinition` (and its runs/events/tags)
scoped to the application being removed, extending the pre-existing
`ExtensionAssociation`/`deployedConfigs` cascade-cleanup precedent.

**Experiment results are never fabricated.** `ExperimentRunResult` (`ExecutiveSummary`/
`ReasonsToLaunch`/`ReasonsNotToLaunch`) is free-text a caller supplies to `StopExperimentRun`; this
backend has no analytics engine and computes no variant metrics, confidence intervals, or statistical
significance from exposure data (there is none to compute — this backend does not simulate real user
traffic against treatments), so it only ever stores and echoes back what a caller explicitly provides,
same rationale as `AppliedExtensions`/`ActionInvocations` being left empty elsewhere in this service
(no execution engine backing them either).

**Two experiment-family defaults are documented assumptions, not verified wire facts** — flagged
explicitly rather than silently guessed: `StartExperimentRun`'s `ExposurePercentage` defaults to `0` when
omitted (the SDK documents no default), and `DeleteExperimentDefinition`'s `delete_type` defaults to
`ARCHIVE` when omitted (same: no documented default). Both reasoning chains are in `gaps` above. A real
client that always sends these fields explicitly observes no difference from real AWS either way.

**`Treatment.Key`** (`"Control"` for the control treatment, `"Treatment1"`.."TreatmentN"` for the rest,
1-indexed by creation/update order) is server-generated: real `TreatmentInput` carries no client-supplied
key, so AWS must assign one, but the exact scheme is undocumented — this backend's choice is a documented
assumption (see `Treatment`'s doc comment in `models.go`), not a verified wire fact.

### 2026-07-25 follow-up: six pre-existing Create* handlers' inline Tags fixed (bd gopherstack-lcan)

Fixed the gap tracked above (previously left unfixed as too large a mechanical change to fit alongside
the deployment-state-machine/extension-versioning/GetConfiguration/experiment-family work). Confirmed
against `aws-sdk-go-v2/service/appconfig@v1.48.0`'s `api_op_Create*.go` that `CreateApplicationInput`,
`CreateEnvironmentInput`, `CreateConfigurationProfileInput`, `CreateDeploymentStrategyInput`,
`CreateExtensionInput`, and `CreateExtensionAssociationInput` each have an optional inline
`Tags map[string]string` member — and that `CreateHostedConfigurationVersionInput` does **not** (hosted
configuration versions are immutable content blobs, not a taggable resource type in the real API), which
is why that op was correctly excluded from both the original bug and this fix: exactly six affected ops,
matching the bd issue.

Each of the six handlers now binds `Tags` from the JSON request body (previously not bound at all — the
field was not merely mis-parsed, it was entirely absent from every request struct) and threads it through
to the corresponding `InMemoryBackend.Create*` method, which was extended with a `tags map[string]string`
parameter on both the method itself and the `StorageBackend` interface. Tags are applied directly to
`b.tags[arn] = maps.Clone(tags)` immediately after the resource is created and while still holding the
same lock — **not** via a call to `TagResource`, because `TagResource` takes its own lock and would
deadlock if called re-entrantly from inside `Create*` while that lock is already held. This mirrors the
pattern `CreateExperimentDefinition`/`StartExperimentRun` already used (see their doc comments in
`experiment_definitions.go`/`experiment_runs.go`), which is exactly how those two newer ops avoided
repeating this bug in the first place.

Threading the new parameter touched every existing call site of the six `Create*` backend methods across
the package's test suite (~90 call sites across 9 test files: `applications_test.go`,
`configuration_test.go`, `configuration_profiles_test.go`, `hosted_configuration_versions_test.go`,
`extensions_test.go`, `deployment_strategies_test.go`, `leak_test.go`, `persistence_test.go`,
`deployments_test.go`) — each updated to pass `nil` for the new trailing `tags` parameter where the test
was not itself exercising tagging, preserving existing behavior/assertions unchanged. `golangci-lint`'s
`fieldalignment` check additionally required reordering the new `Tags` field in each of the six handler
request structs to minimize struct padding.

New table-test coverage was added as one new table test per handler (`TestHandler_Create*_TagsAppliedInline`
in each of `handler_applications_test.go`, `handler_environments_test.go`,
`handler_configuration_profiles_test.go`, `handler_deployment_strategies_test.go`, and two in
`handler_extensions_test.go` for `CreateExtension`/`CreateExtensionAssociation`), each with a
`tags_applied_at_create` case (POSTs `Tags` inline, then asserts the exact map is returned by a follow-up
`ListTagsForResource` call) and a `no_tags_is_not_an_error` case (regression guard: an absent/nil `Tags`
must not error or panic). This directly exercises the bug's repro path (tags set at create time were
previously invisible to `ListTagsForResource`), so a regression to "field bound but not forwarded" or
"forwarded but not applied" would now fail these tests directly rather than passing silently as it did
before this pass.
