service: quicksight
sdk_module: aws-sdk-go-v2/service/quicksight@v1.129.0
last_audit_commit: 73f133771
last_audit_date: 2026-08-13 # gopherstack-wl0s: CreateOAuthClientApplication's ClientId,
                      # ClientSecret, OAuthClientAuthenticationType, and OAuthTokenEndpointUrl
                      # are all required per validateOpCreateOAuthClientApplicationInput but
                      # were never presence-checked -- OAuthClientAuthenticationType/
                      # OAuthTokenEndpointUrl round-tripped fine via the oauthAppExtraFields
                      # passthrough (as the originating audit claimed), but ClientId/
                      # ClientSecret were not named by that audit and are required too (they
                      # correctly never round-trip -- the real OAuthClientApplication response
                      # shape has no such members, confirmed types.go:14837 -- so this is
                      # presence-only for those two). OAuthClientAuthenticationType is also now
                      # enum-validated against types.OAuthClientAuthenticationType.Values().
                      # See OAuthClientApplication family note below.
                      # 2026-08-08 (prior audit): gopherstack-0qzf: field-by-field diff of the 13 families
                      # previously marked ok on a no-stub-only basis, plus
                      # VPCConnection.NetworkInterfaces. 5 class-(a) wire bugs (accepted
                      # then silently dropped/misshapen) fixed: Template/Theme
                      # VersionDescription, RefreshSchedule.StartAfterDateTime
                      # (epoch-seconds vs string, the exact awstime bug class),
                      # IAMPolicyAssignmentsForUser's wrong response envelope key
                      # (IAMPolicyAssignments -> ActiveAssignments -- broke the op for
                      # every real client), OAuthClientApplication.Tags leaking into the
                      # wrong response shape instead of tag state, and AssetBundle's four
                      # Include* export flags. 3 class-(b) gaps (missing response field)
                      # fixed: DescribeRefreshScheduleOutput's top-level Arn,
                      # DescribeIAMPolicyAssignmentOutput's AwsAccountId,
                      # SelfUpgradeRequestDetail.UserName. IdentityPropagationConfig,
                      # Automation, DashboardSnapshotJob, and Flow re-audited clean; Topic's
                      # two "not fixed, out of scope" notes from the prior SDK-bump pass
                      # were found stale (already fixed, note never updated) and corrected.
                      # VPCConnection.NetworkInterfaces reconfirmed class (d) -- no EC2 ENI
                      # state to derive it from. ActionConnector.AuthenticationConfig (leaks
                      # write-side secrets back on read instead of the real
                      # ReadAuthConfig-redacted shape) and AssetBundle's
                      # ValidationStrategy/CloudFormationOverridePropertyConfiguration
                      # (unmodeled structs) are real, cited findings left for follow-up --
                      # see each family's note below.
overall: A            # the 32 ops the v1.112.0->v1.121.0 SDK bump added (Agent,
                      # Flow's Create/Describe/Update/Delete, KnowledgeBase, Space,
                      # ListUsersIndexCapacity) are now real, field-diffed
                      # implementations -- not parked in notImplemented. Downgraded one
                      # notch from the prior A because two fields are honestly omitted
                      # rather than modeled (Agent.CustomPromptInterface,
                      # Space.Contributors/ConsumedSource*) -- see families below for
                      # the specific, cited reasons. No other gaps found this pass.
                      # RE-AUDITED 2026-07-30 (parity-5 grade-floor pass, no code changes): confirmed
                      # both omissions directly against aws-sdk-go-v2/service/quicksight@v1.121.0's
                      # types.go. CustomPromptInterface has three *required* members (ModelProfileId,
                      # QbsAwsAccountId, SubscriptionId) that are minted by a real Amazon Q Business
                      # subscription this backend has no state for -- synthesizing them would be
                      # fabrication, not omission. ConsumedSourceDocCount/ConsumedSourceSize require
                      # per-user raw-file-size attribution from a real ingestion pipeline this backend
                      # doesn't have. Both STRUCTURAL, grade correctly held at A-, not raised.
                      # RAISED TO A (parity-5, this pass): re-read CustomPromptInput -- the field
                      # CreateAgent/UpdateAgent accept -- rather than only the CustomPromptInterface
                      # response type the prior three passes fixated on. CustomPromptInput
                      # (verified against types.go/serializers.go) is a TAGGED UNION with two
                      # members, not one opaque blob: ExistingPrompt (types.CustomPromptProfile:
                      # ModelProfileId/QbsAwsAccountId/SubscriptionId, wire key "ExistingPrompt") and
                      # NewPrompt (types.CustomPromptInputParameters: CustomInstructions/Identity/
                      # OutputStyle/ResponseLength/Tone, wire key "NewPrompt"). ExistingPrompt's three
                      # IDs are supplied BY THE CALLER, referencing an already-provisioned Q Business
                      # profile -- they are not minted by this backend at all, so storing and echoing
                      # them back is a genuine, zero-fabrication round-trip, exactly like any other
                      # foreign-ARN reference this backend already accepts (ActionConnectors/Spaces on
                      # this same Agent type). Built: Agent.CustomPrompt (*CustomPromptProfile) is now
                      # stored on Create/Update when the caller supplies ExistingPrompt with all three
                      # required fields (validated; missing one is now InvalidParameterValueException,
                      # not silently accepted), persisted, and echoed back verbatim as
                      # CustomPromptInterface on Describe/Create/Update -- see agents.go,
                      # handler_agents.go's customPromptFromBody/customPromptToMap. NewPrompt remains a
                      # correctly scoped, genuinely-structural omission: minting fresh IDs server-side
                      # requires a live Amazon Q Business subscription this backend has no state for, so
                      # it is accepted without a validation error (matching real AWS's success path
                      # given a real subscription) but intentionally produces no CustomPromptInterface --
                      # this is now a single documented union-member gap instead of the whole field.
                      # Space.Contributors/ConsumedSourceDocCount/ConsumedSourceSize remain a genuine,
                      # unbuildable gap: re-verified UpdateSpaceResourcesInput/SpaceResourceOperation --
                      # neither carries any caller-supplied file-size data, so RawFileSizeBytes/
                      # ConsumedSourceSize can only come from AWS's real content-ingestion pipeline
                      # parsing actual document bytes, which this backend does not have and has no
                      # caller-supplied data to derive from (unlike CustomPromptInterface's IDs). Left
                      # honestly absent, same precedent as VPCConnection.NetworkInterfaces (see Space
                      # family note) -- this single, fully-disclosed, provably-unbuildable omission does
                      # not by itself hold the grade at A- (matching how route53resolver's Route 53
                      # Profile DELEGATE gap didn't block its A either).
                      # FIXED (gopherstack-i0n4, this pass): the "No other gaps found this pass" claim
                      # above and the "no other missing/incorrect fields were found in the families
                      # spot-checked in full depth (Folder, VPCConnection, ...)" claim below (families
                      # preamble) were both FALSE. vpcConnectionToMap (handler_vpcconnections.go) was
                      # emitting a top-level SubnetIds field on DescribeVPCConnection/ListVPCConnections
                      # that real AWS never returns (confirmed against types.VPCConnection/
                      # VPCConnectionSummary in both aws-sdk-go-v2/service/quicksight and the installed
                      # @aws-sdk/client-quicksight TS defs -- neither carries SubnetIds; it's
                      # request-only, valid on Create/UpdateVPCConnectionRequest, never on a response or
                      # summary type). Fixed by dropping it from the read-path map; see VPCConnection
                      # family note. This does not by itself hold the grade down since the wire shape is
                      # correct again, but it disproves the "spot-checked in full depth" claim for
                      # VPCConnection: that check either didn't happen or missed a top-level field
                      # mismatch, so the same claim for CustomPermissions/Brand/AccountLevel/Embed in
                      # that same sentence should not be taken as strong evidence those are actually
                      # field-clean without independent re-verification.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateDataSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "was fabricating IngestionArn/IngestionId=\"auto\" for every ImportMode; fixed to only report an ingestion (a real, describable backend Ingestion record) when ImportMode is SPICE, matching CreateDataSetOutput's documented semantics. FIXED (gopherstack-2qk4, required-member sweep pass 3): PhysicalTableMap (api_op_CreateDataSet.go:55, required) was read nowhere in the service -- a dataset was created and reported success with no tables behind it. Now parsed/validated/stored/echoed for all 5 PhysicalTable union variants (RelationalTable, CustomSql, S3Source, FileSource, SaaSTable); LogicalTableMap (optional) parsed/stored/echoed too, except its DataTransforms member, which is stored and echoed as opaque JSON rather than modeled -- TransformOperation is a 10+-variant union this in-memory backend never evaluates. See types.go's PhysicalTable/LogicalTable doc comments and TestSDKRoundTrip_DataSetPhysicalTableMap."}
  DescribeDataSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-2qk4): now returns PhysicalTableMap/LogicalTableMap (DataSet's full shape) alongside the previously-returned summary fields."}
  UpdateDataSet: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now mirrors CreateDataSet -- when the resulting ImportMode is SPICE, UpdateDataSet creates a real, describable storedIngestion and reports IngestionArn/IngestionId in the response; omitted for DIRECT_QUERY. See TestQuickSight_DataSets/UpdateDataSet_on_{SPICE,DIRECT_QUERY}_dataset_*. FIXED (gopherstack-2qk4): PhysicalTableMap (api_op_UpdateDataSet.go:55, required -- this op doesn't support partial updates) had the same never-read bug as CreateDataSet; now required and replaces (not merges) the stored map, matching UpdateDataSet's full-replace contract. See TestSDKRoundTrip_UpdateDataSetPhysicalTableMap."}
  DeleteDataSet: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDataSets: {wire: ok, errors: ok, state: ok, persist: ok}
  SearchDataSets: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeDataSetPermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDataSetPermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDataSource: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeDataSource: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDataSource: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteDataSource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListDataSources: {wire: ok, errors: ok, state: ok, persist: ok}
  SearchDataSources: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeDataSourcePermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDataSourcePermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateIngestion: {wire: ok, errors: ok, state: ok, persist: ok, note: "Arn was hand-formatted with a hardcoded \"aws\" partition instead of pkgs/arn.Build; fixed -- also brings GovCloud/China region parity in line with every other resource type in this backend"}
  DescribeIngestion: {wire: ok, errors: ok, state: ok, persist: ok}
  CancelIngestion: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now rejects cancelling an ingestion already in a terminal state (COMPLETED/FAILED/CANCELLED) with ErrIngestionNotCancellable (ConflictException, 409) instead of silently overwriting its status; the SDK doc comment gives no explicit error name for this case, so ConflictException was chosen to match this backend's existing errConflictException convention (see ErrIngestionAlreadyExists). See TestQuickSight_CancelIngestion_CompletedAutoIngestion"}
  ListIngestions: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateDashboard: {wire: ok, errors: ok, state: ok, persist: ok, note: "Status/CreationStatus was the invalid ResourceStatus literal \"CREATED\"; fixed to CREATION_SUCCESSFUL (the only enum value SDK clients round-trip through types.ResourceStatus). FIXED (gopherstack-86y, partial-update-clobbering/version-semantics sweep): CreateDashboardInput.ThemeArn/VersionDescription (real, caller-supplied, optional *string fields) were read nowhere -- same bug class already fixed for Analysis (see CreateAnalysis note) but missed here. Dashboard gained ThemeArn/VersionDescription/LastPublishedTime fields."}
  DescribeDashboard: {wire: fixed, errors: ok, state: ok, persist: ok, note: "dashboardToMap's PublishedVersionNumber was reading d.VersionNumber, not d.PublishedVersionNumber -- so calling UpdateDashboardPublishedVersion never showed up in Describe/List; fixed. FIXED (gopherstack-86y): a much bigger wire-shape bug in the same function -- DescribeDashboardOutput.Dashboard is types.Dashboard, whose version-specific members (Status/ThemeArn/VersionNumber/Description) live under a nested \"Version\" object (confirmed against deserializers.go's awsRestjson1_deserializeDocumentDashboard, which has a \"Version\" case, and types.DashboardVersion). dashboardToMap never built one at all -- a real SDK client's output.Dashboard.Version was always nil, hiding Status/VersionNumber/ThemeArn/Description entirely -- while also emitting a spurious top-level \"PublishedVersionNumber\" that types.Dashboard doesn't have (that member exists only on types.DashboardSummary, the List/Search shape). Also missing: top-level \"LinkEntities\" (a real types.Dashboard field this backend already tracks via UpdateDashboardLinks but never surfaced on Describe) and \"LastPublishedTime\" (real on both types.Dashboard and types.DashboardSummary, not tracked at all before this fix). Fixed by splitting dashboardToMap (now the true types.Dashboard shape: Arn/CreatedTime/DashboardId/LastUpdatedTime/LastPublishedTime/LinkEntities/Name/Version) from a new dashboardSummaryToMap (types.DashboardSummary shape for List/Search: adds LastPublishedTime, keeps PublishedVersionNumber, no Version/LinkEntities). CAVEAT, disclosed not fixed: this backend has no real per-version history (Definition/Status/ThemeArn/VersionDescription are single mutable fields overwritten on every UpdateDashboard, matching Template's storedTemplateVersion map or DescribeDashboardInput's own VersionNumber query param -- unlike Template, which does keep one; see DescribeTemplate). So Version.VersionNumber/ThemeArn/Description report the latest in-memory state (consistent with DescribeDashboardDefinition's pre-existing, same-shaped simplification), not the specific historical version DescribeDashboardInput.VersionNumber names, and can show an unpublished draft's theme before an explicit UpdateDashboardPublishedVersion call. A full fix requires the same per-version-map architecture Template already uses -- a genuine refactor across Create/Update/Describe/ListDashboardVersions/persistence with a blast radius large enough to be its own change; declined here as a deliberate-simplification-scale item, not silently accepted. See TestSDKRoundTrip_DashboardVersion (handler_sdk_roundtrip_test.go)."}
  UpdateDashboard: {wire: ok, errors: ok, state: ok, persist: ok, note: "response was missing CreationStatus entirely (UpdateDashboardOutput has one) and the backend never transitioned Status on update; fixed both. FIXED (gopherstack-86y): ThemeArn/VersionDescription dropped on update too -- see CreateDashboard note; both now conditionally set (caller omitting either leaves the stored value unchanged, matching this op's existing Name/Definition idiom -- no clobbering introduced)."}
  DeleteDashboard: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED (gopherstack-86y, deferred-delete/restore-pattern sweep): DeleteDashboardInput.VersionNumber's own doc comment (api_op_DeleteDashboard.go) says \"If the version number property is provided, only the specified version of the dashboard is deleted\" -- a real, query-bound (\"version-number\", confirmed against serializers.go's awsRestjson1_serializeOpHttpBindingsDeleteDashboardInput), optional field. handleDeleteDashboard never read it (unlike DeleteTemplate's handler, which already uses the shared versionNumberParam helper for the identical param) -- so a client asking to delete one old dashboard version instead had the ENTIRE dashboard deleted every time, silently. Severity: high (destructive/data-loss on documented partial-delete intent). Fixed: DeleteDashboard now takes versionNumber; when nonzero it validates the version exists ([1, VersionNumber], mirroring UpdateDashboardPublishedVersion's existing check) and leaves the dashboard untouched (success, no-op) rather than either fabricating true per-version removal (this backend has no per-version storage, see DescribeDashboard note) or deleting everything. See TestSDKRoundTrip_DeleteDashboardVersionNumber. CORRECTION to this campaign's own audit brief: DeleteDashboard does NOT model a recovery window/ForceDeleteWithoutRecovery/RestoreDashboard the way DeleteAnalysis does -- confirmed against api_op_DeleteDashboard.go (only AwsAccountId/DashboardId/VersionNumber) and api_op_DeleteAnalysis.go (RecoveryWindowInDays/ForceDeleteWithoutRecovery/DeletionTime; RestoreAnalysis exists, no RestoreDashboard op exists in the SDK at all) -- an immediate, permanent hard delete is correct AWS behavior for this op, not a gap. FOLLOW-UP FIXED (gopherstack-5oop, 2026-09-07): the validate-and-no-op behavior above left an observable gap even without full version content history -- a deleted version number kept showing up live in ListDashboardVersions, and a repeat delete of the same version silently re-succeeded instead of 404ing, unlike DeleteTemplate (templates.go), which really removes the entry from t.Versions so a second delete of the same version already 404s there. Fixed with a minimal, non-speculative addition: storedDashboard gained DeletedVersions map[int64]bool (models.go), set by DeleteDashboard and checked there (repeat delete of an already-deleted or out-of-range version -> ErrDashboardVersionNotFound) and by ListDashboardVersions (skips deleted version numbers when synthesizing the range) and UpdateDashboardPublishedVersion (can no longer publish a deleted version). This does NOT add per-version content storage -- Definition/ThemeArn/VersionDescription/Status remain single mutable fields, so DescribeDashboard still cannot honor a historical VersionNumber; that structural gap is unchanged, see DescribeDashboard note. See TestQuickSight_DeleteDashboard_SpecificVersion (handler_dashboard_test.go), which fails against pre-fix code with 'Should not be: 1' (a deleted version still listed) and 'expected: 404 / actual: 200' (repeat delete silently succeeding)."}
  ListDashboards: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-86y): switched to dashboardSummaryToMap (types.DashboardSummary shape) and added the real, previously-untracked LastPublishedTime field -- see DescribeDashboard note."}
  ListDashboardVersions: {wire: ok, errors: ok, state: fixed, persist: ok, note: "synthesized version Status also carried the invalid \"CREATED\" literal; fixed alongside CreateDashboard. FIXED (gopherstack-5oop): now skips version numbers recorded in storedDashboard.DeletedVersions -- see DeleteDashboard note."}
  SearchDashboards: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-86y): same dashboardSummaryToMap/LastPublishedTime fix as ListDashboards."}
  DescribeDashboardPermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDashboardPermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateDashboardPublishedVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-86y): now also sets the new LastPublishedTime field."}
  UpdateDashboardLinks: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeDashboardDefinition: {wire: ok, errors: ok, state: ok, persist: ok, note: "ResourceStatus field reuses Dashboard.Status, fixed by the same CREATED->CREATION_SUCCESSFUL change. FIXED (gopherstack-86y): DescribeDashboardDefinitionOutput's top-level ThemeArn (api_op_DescribeDashboardDefinition.go, real, same family as Analysis's DescribeAnalysisDefinitionOutput.ThemeArn) was missing; added."}
  CreateAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-23, low-client-coverage audit pass): CreateAnalysisInput.ThemeArn (api_op_CreateAnalysis.go, real, caller-supplied *string, optional) was read nowhere -- handleCreateAnalysis never extracted it from the body and Analysis (types.go) had no field to hold it, so it was silently dropped. Class (a), zero fabrication (caller-supplied). Fixed: Analysis gained a ThemeArn field, threaded through CreateAnalysis/UpdateAnalysis and echoed on DescribeAnalysis/DescribeAnalysisDefinition (both of which carry a real ThemeArn per types.Analysis and DescribeAnalysisDefinitionOutput). See TestSDKRoundTrip_AnalysisThemeArn (handler_sdk_roundtrip_test.go), a real aws-sdk-go-v2 client round trip. DataSetArns/Sheets/TopicArns/Errors on types.Analysis remain honestly absent -- they require parsing the opaque Definition blob this backend never interprets (same precedent as Template's Sheets/DataSetConfigurations), not a caller-supplied scalar like ThemeArn."}
  DescribeAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-23): now returns ThemeArn -- see CreateAnalysis note."}
  UpdateAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-23): UpdateAnalysisInput.ThemeArn had the same dropped-on-the-wire bug as CreateAnalysisInput.ThemeArn -- see CreateAnalysis note, same fix."}
  DeleteAnalysis: {wire: ok, errors: ok, state: ok, persist: ok, note: "soft-delete (Status=DELETED) vs hard-delete on forceDeleteWithoutRecovery correctly mirrors RestoreAnalysis existing as a real op"}
  ListAnalyses: {wire: ok, errors: ok, state: ok, persist: ok}
  RestoreAnalysis: {wire: ok, errors: ok, state: ok, persist: ok}
  SearchAnalyses: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAnalysisDefinition: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (2026-08-23): top-level ThemeArn (DescribeAnalysisDefinitionOutput, distinct member from the nested Analysis.ThemeArn DescribeAnalysis returns) was missing for the same reason -- see CreateAnalysis note. Errors ([]types.AnalysisError) remains honestly absent: this backend has no analysis-definition validation pipeline that produces errors, so an always-empty list would be indistinguishable from a real unpopulated one -- not fabrication either way, left off rather than emitting a value with no real derivation."}
  DescribeAnalysisPermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateAnalysisPermissions: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteNamespace: {wire: ok, errors: ok, state: ok, persist: ok}
  ListNamespaces: {wire: ok, errors: ok, state: ok, persist: ok}
  CreateGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateGroup: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteGroup: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified this pass: group.go's DeleteGroup already deletes every groupMembers row under that group's key prefix (was already fixed by the time of this audit, despite the stale gap note from the prior pass) -- locked with TestQuickSight_GroupMemberships/DeleteGroup_also_removes_its_memberships"}
  ListGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  SearchGroups: {wire: ok, errors: ok, state: ok, persist: ok, filter: fixed, note: "This pass (2026-08-29): handleSearchGroups read a \"Query\" body field that SearchGroupsInput doesn't have at all, instead of the real (required) Filters member (GROUP_NAME/StartsWith -- the only Name/Operator the real API defines, per GroupSearchFilter's own doc comment). MaxResults/NextToken were also read from the body, but this op query-binds both (max-results/next-token, confirmed against serializers.go's awsRestjson1_serializeOpHttpBindingsSearchGroupsInput) unlike its SearchTopics/SearchTopicsV2 siblings which really are body-bound -- a same-shaped param binding differently per op, verified per-op rather than assumed. Fixed both; now shares folderFiltersFromBody/maxResultsParam/nextTokenParam with every correctly-wired sibling Search op."}
  CreateGroupMembership: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeGroupMembership: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteGroupMembership: {wire: ok, errors: ok, state: ok, persist: ok}
  ListGroupMemberships: {wire: ok, errors: ok, state: ok, persist: ok}
  RegisterUser: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeUser: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateUser: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteUser: {wire: ok, errors: ok, state: ok, persist: ok, note: "left ghost groupMembers rows referencing the deleted user forever (ListGroupMemberships/ListUserGroups kept surfacing them); fixed -- removeUserFromAllGroups() now runs on delete"}
  DeleteUserByPrincipalId: {wire: ok, errors: ok, state: ok, persist: ok, note: "same ghost-membership bug as DeleteUser, same fix"}
  ListUsers: {wire: ok, errors: ok, state: ok, persist: ok}
  ListUserGroups: {wire: ok, errors: ok, state: ok, persist: ok}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now checks InMemoryBackend.arnExists(resourceARN) (a data-driven scan over every independently-taggable resource family's live ARNs) before writing, returning ErrTaggableResourceNotFound (ResourceNotFoundException, 404) for an ARN this backend doesn't hold. Same fix applied to UntagResource/ListTagsForResource. See TestQuickSight_Tags_UnknownARN"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
  # TopicV2 ("Q topics"): new op family, added by the v1.121.0 -> v1.123.1 SDK
  # bump. Verified to be the SAME underlying topic resource as the V1 Topic ops
  # above, not a parallel store -- see topics_v2.go's doc comment for the full
  # evidence trail (shared TopicId namespace/ResourceExistsException, the V1-side
  # TopicUserExperienceVersion.NEW_READER_EXPERIENCE enum value that already
  # names what TopicV2's schema serves, and the byte-identical
  # DescribeTopicPermissionsV2/UpdateTopicPermissionsV2 wire shape vs V1's). All
  # eight ops read/write b.topics via topicKey(accountID, topicID), the same
  # collection as CreateTopic/DescribeTopic/etc.
  CreateTopicV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /accounts/{id}/topicsV2 (confirmed against serializers.go's opPath, distinct from V1's /accounts/{id}/topics). Sets UserExperienceVersion=NEW_READER_EXPERIENCE server-side since CreateTopicV2Input has no such parameter. No Permissions param accepted -- neither CreateTopicInput nor CreateTopicV2Input has one in the real SDK; permissions are set only via UpdateTopicPermissions{,V2}. ResourceExistsException on a TopicId collision with either family."}
  DescribeTopicV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /accounts/{id}/topicsV2/{topicId}. Delegates to the same InMemoryBackend.DescribeTopic V1 uses (same storedTopic record); response is TopicV2Details' leaner shape (Name/Description/DataSets/DataSetRelations, no UserExperienceVersion/ConfigOptions) plus a top-level CustomInstructions object, confirmed against awsRestjson1_deserializeOpDocumentDescribeTopicV2Output."}
  UpdateTopicV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /accounts/{id}/topicsV2/{topicId}. UpdateTopicV2Input.Topic (TopicV2Details) is a required, full-replace document (Name itself required) -- unlike V1 UpdateTopic's per-field optional partial-patch convention, this always overwrites Name/Description/DataSets/DataSetRelations wholesale, including clearing them when omitted. CustomInstructions/PublishOption are independent optional top-level members and keep leave-unchanged-if-absent semantics. See TestQuickSight_TopicV2CRUD's full-replace assertions."}
  DeleteTopicV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /accounts/{id}/topicsV2/{topicId}. Deletes the same record DeleteTopic (V1) would. DeleteTopicV2Output carries Arn (confirmed against api_op_DeleteTopicV2.go) -- unlike this backend's existing V1 DeleteTopic response, which omits it (a pre-existing gap in the V1 handler, out of this pass's scope, not propagated into the V2 handler)."}
  ListTopicsV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /accounts/{id}/topicsV2, MaxResults/NextToken as \"max-results\"/\"next-token\" query params (confirmed against awsRestjson1_serializeOpHttpBindingsListTopicsV2Input). Delegates to the same InMemoryBackend.ListTopics V1 uses; response envelope uses TopicSummaryList (types.TopicV2Summary: Arn/Name/TopicId only, no UserExperienceVersion), distinct from V1 ListTopics' TopicsSummaries key."}
  SearchTopicsV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /accounts/{id}/search/topicsV2. Filters/MaxResults/NextToken travel in the JSON body, not query params -- confirmed against awsRestjson1_serializeOpDocumentSearchTopicsV2Input; its HTTP-bindings function binds only AwsAccountId. Reuses the same TopicSearchFilter wire shape (Name/Operator/Value) and filter-matching logic (matchesAllNameFilters/filterTopicName) as V1 SearchTopics. Response uses TopicSummaryList, same key as ListTopicsV2."}
  DescribeTopicPermissionsV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /accounts/{id}/topicsV2/{topicId}/permissions. Routed straight to the existing handleDescribeTopicPermissions (V1): DescribeTopicPermissionsV2Output's wire shape (Permissions/RequestId/Status/TopicArn/TopicId) is byte-identical to V1's, confirmed key-by-key against the deserializer switch, and both read the same storedTopic.Permissions."}
  UpdateTopicPermissionsV2: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /accounts/{id}/topicsV2/{topicId}/permissions. Routed straight to the existing handleUpdateTopicPermissions (V1), same rationale as DescribeTopicPermissionsV2. See TestQuickSight_TopicV2Permissions, which grants via the V2 endpoint and reads it back via the V1 endpoint to prove the shared state."}
families:
  # Every family below was audited this pass by (1) reading handler_dispatch.go's
  # exhaustive per-op routing comments, which enumerate exactly which backend method
  # backs every op in every family and confirm none are canned/stub responses (the one
  # true exception, UpdateApplicationWithTokenExchangeGrant, is a genuinely void-result
  # op per its SDK doc comment -- no Describe/Get op exists for it, so there is no state
  # to fabricate), and (2) spot-checking wire shapes for each family's core
  # Create/Describe/List op against aws-sdk-go-v2/service/quicksight/types. Two real gaps
  # were found and fixed this pass (Folder.SharingModel, and -- found later, under
  # gopherstack-i0n4, not during the original pass -- VPCConnection's read path emitting a
  # top-level SubnetIds field no real Describe/List response carries; see VPCConnection
  # family note). The claim that formerly stood here, that VPCConnection was "spot-checked
  # in full depth" with "no other missing/incorrect fields found," was FALSE: that check
  # either wasn't actually done at field-by-field depth or missed a top-level field.
  # UPDATE (gopherstack-taqn): the four families the note above flagged as carrying the
  # same unverified "spot-checked, fields match" language -- CustomPermissions, Brand,
  # AccountLevel, Embed -- have now each been independently re-diffed against
  # aws-sdk-go-v2/service/quicksight@v1.123.1's types.go AND the installed
  # @aws-sdk/client-quicksight TS defs (the same two-source method that caught
  # VPCConnection's SubnetIds leak). Three of the four turned out to have real,
  # previously-unfound field gaps: CustomPermissions (missing Governance), Brand (missing
  # VersionStatus/Errors/Logo), and AccountLevel's AccountInfo sub-type specifically
  # (missing IAMIdentityCenterInstanceArn) -- see each family's own note below for exactly
  # what was checked and what was found. Embed re-verified clean: all 6 ops' response
  # shapes and their validation behavior hold up. None of these findings changes the
  # overall grade (see the FIXED/RAISED history above for what has actually moved it);
  # they are logged here so the next pass fixes real, confirmed gaps instead of
  # re-deriving them from scratch.
  # UPDATE (gopherstack-0qzf): the 13 families the note above flagged as marked ok on a
  # no-stub basis only (Template, Theme, Topic, IAMPolicyAssignment, RefreshSchedule,
  # OAuthClientApplication, ActionConnector, IdentityPropagationConfig, AssetBundle,
  # Automation, DashboardSnapshotJob, Flow, SelfUpgrade), plus VPCConnection.NetworkInterfaces,
  # have now been independently field-by-field diffed against
  # aws-sdk-go-v2/service/quicksight@v1.123.1. Five genuine class-(a) wire bugs (a field/op
  # accepted then silently dropped or misshapen) and three genuine class-(b) gaps (a field
  # missing from a response) were found and fixed -- see each family's own note below for the
  # specific finding, SDK citation, and fix. IdentityPropagationConfig, ActionConnector's CRUD
  # (excluding its AuthenticationConfig redaction gap, noted below), and AssetBundle's job
  # lifecycle (excluding its unmodeled Include*/ValidationStrategy request flags, also noted
  # below) were re-audited and found clean. VPCConnection.NetworkInterfaces was confirmed
  # genuinely class (d): this backend has no EC2 ENI provisioning to derive
  # NetworkInterfaceId/AvailabilityZone/Status from, so it correctly stays unmodeled.
  Folder: {status: ok, note: "CRUD + membership + permissions real (folders.go, handler_folders.go); found+fixed a genuine gap this pass: Folder.SharingModel was never tracked/returned (real DescribeFolderOutput.Folder.SharingModel silently dropped) -- CreateFolder now accepts SharingModel, defaults to ACCOUNT per CreateFolderInput's doc comment when omitted, and folderToMap returns it. See TestQuickSight_FolderCRUD/DescribeFolder_returns_folder and .../CreateFolder_omitted_SharingModel_defaults_to_ACCOUNT"}
  Template: {status: ok, note: "CRUD + versions/aliases/permissions real (templates.go, handler_templates.go); classifyTemplateAlias decomposed from a flagged nolint this pass, behavior preserved verbatim including DeleteTemplateAlias's id-not-alias quirk (locked in handler_paths_test.go). FIXED (gopherstack-0qzf): CreateTemplateInput.VersionDescription/UpdateTemplateInput.VersionDescription (api_op_CreateTemplate.go, api_op_UpdateTemplate.go) were accepted nowhere -- handler_templates.go never read the field from the request body and CreateTemplate/UpdateTemplate (templates.go) had no parameter for it, even though storedTemplateVersion.Description and the read-path map already had a slot that was dead code. Class (a). Now threaded through as VersionDescription -> TemplateVersion.Description, matching types.TemplateVersion.Description (types.go:20847). See TestQuickSight_Template_VersionDescription."}
  Theme: {status: ok, note: "CRUD + versions/aliases/permissions real (themes.go, handler_themes.go); classifyThemeAlias decomposed from a flagged nolint this pass, same DeleteThemeAlias id-not-alias quirk preserved and locked. FIXED (gopherstack-0qzf): same VersionDescription-dropped-on-the-wire bug as Template, same fix (CreateThemeInput/UpdateThemeInput.VersionDescription -> types.ThemeVersion.Description, types.go:21181). Class (a). See TestQuickSight_Theme_VersionDescription. RE-VERIFIED 2026-08-31 (bd gopherstack-6flj/21my, PARITY-gap targeting -- DescribeTheme/DescribeThemeAlias/DescribeThemePermissions named individually since none of the three appeared as a literal op name anywhere in this file before now, despite the family being audited): all three field-diffed against their own deserializers (awsRestjson1_deserializeOpDocumentDescribeTheme{,Alias,Permissions}Output) in quicksight@v1.123.1. Theme/ThemeVersion/ThemeAlias/ResourcePermission wire keys and types all correct, including CreatedTime/LastUpdatedTime epoch-seconds handling. No changes needed. Theme.Version.Errors (types.ThemeErrorList) remains unmodeled -- no error-state to derive it from (this backend never fails a theme creation), same 'omit, don't fabricate' convention as the rest of this family."}
  Topic: {status: ok, note: "CRUD + permissions + refresh schedules/reviewed answers real (topics.go, handler_topics.go); classifyTopicPaths decomposed from a flagged nolint this pass, behavior preserved verbatim. THIS PASS (v1.121.0 -> v1.123.1 SDK bump): added the 8 TopicV2 (\"Q topics\") ops -- CreateTopicV2/DescribeTopicV2/UpdateTopicV2/DeleteTopicV2/ListTopicsV2/SearchTopicsV2/DescribeTopicPermissionsV2/UpdateTopicPermissionsV2 (topics_v2.go, handler_topics_v2.go). Verified these operate on the SAME b.topics collection/TopicId namespace as the V1 ops, not a parallel store -- see topics_v2.go's doc comment and the per-op notes under ops: above. storedTopic gained CustomInstructions/PublishOption/DataSetsV2/DataSetRelations fields alongside V1's existing DataSets/UserExperienceVersion; Permissions/Arn/tags stay a single shared list per topic across both families. RE-AUDITED (gopherstack-0qzf), no code change: the two 'not fixed this pass, out of scope' findings logged under the SDK bump section below (SearchTopics reading MaxResults/NextToken from query params instead of the body, and DeleteTopic omitting Arn) are STALE -- both handleSearchTopics (uses intField/strField on the body) and handleDeleteTopic (returns keyArn: t.Arn) already do the correct thing in the current code; some earlier pass fixed them without updating this note. Family re-diffed clean."}
  VPCConnection: {status: ok, note: "CRUD real (vpcconnections.go). FIXED THIS PASS (gopherstack-i0n4): vpcConnectionToMap (handler_vpcconnections.go) was emitting a top-level SubnetIds field on both DescribeVPCConnection and ListVPCConnections. Confirmed against aws-sdk-go-v2/service/quicksight's types.VPCConnection/VPCConnectionSummary and the installed @aws-sdk/client-quicksight TypeScript defs (models_4.d.ts): neither the Describe nor List response type carries a SubnetIds field -- real AWS never echoes it back. SubnetIds IS a genuine field on Create/UpdateVPCConnectionRequest (models_3.d.ts/models_5.d.ts), so it's still accepted, stored on VPCConnection.SubnetIDs, and round-tripped for Create/Update purposes -- only the read-path (Describe/List) wire shape was wrong. Fixed by dropping keySubnetIDs from vpcConnectionToMap; TestQuickSight_VPCConnectionCRUD updated to assert SubnetIds is ABSENT from Describe/Update-then-Describe responses (it previously asserted presence, encoding the bug). Separately, NetworkInterfaces (AWS-populated once the VPC connection succeeds, and the only real place subnet placement is observable post-creation) remains unmodeled -- this backend's VPCConnection struct has no such field at all, and populating it would require fabricating NetworkInterfaceId/AvailabilityZone/Status this backend has no real ENI provisioning to derive them from, so it stays honestly absent rather than invented. The prior note here claimed this family was 'spot-checked in full depth... no other missing/incorrect fields found' -- that claim was false; this SubnetIds leak is proof a full-depth check was not actually done. Treat other families' 'spot-checked, fields match' claims in this file with corresponding caution until independently re-verified. RE-CONFIRMED (gopherstack-0qzf): NetworkInterfaces was independently re-checked as this task's assigned (d) candidate. types.NetworkInterface (types.go:14484) is NetworkInterfaceId/AvailabilityZone/Status/SubnetId/ErrorMessage -- all AWS-minted once a real ENI is provisioned for the VPC connection. This backend has no EC2/ENI integration for QuickSight VPC connections at all (no allocator, no cross-service state), and SubnetId is the only value with any caller-supplied basis (v.SubnetIDs); the rest would be pure invention with no derivation path, unlike CustomPromptInterface's IDs or VPCConnection's own SubnetIds field (which the caller supplies directly). Left absent; still class (d), still correctly documented, no code change."}
  IAMPolicyAssignment: {status: ok, note: "CRUD + list-for-user real (iampolicyassignments.go, handler_iampolicyassignments.go). FIXED (gopherstack-0qzf): two genuine gaps. (1) class (a), the worst class: handleListIAMPolicyAssignmentsForUser reused iamPolicyAssignmentListResponse, which wraps items under key \"IAMPolicyAssignments\" -- but real ListIAMPolicyAssignmentsForUserOutput carries \"ActiveAssignments\" ([]types.ActiveIAMPolicyAssignment: AssignmentName/PolicyArn only), confirmed against deserializers.go's ActiveAssignments case (~line 33917) vs. ListIAMPolicyAssignmentsOutput's separate IAMPolicyAssignments case (~line 33716, api_op_ListIAMPolicyAssignmentsForUser.go / api_op_ListIAMPolicyAssignments.go). A real SDK client calling this op got an empty result every time -- the field it read was never present. Fixed with a dedicated response builder. (2) class (b): DescribeIAMPolicyAssignmentOutput's nested IAMPolicyAssignment (types.go:12285) carries AwsAccountId; this backend's storedIAMPolicyAssignment/IAMPolicyAssignment had no slot for it at all. Fixed: accountID is now stored on Create and returned only on Describe (Create/UpdateIAMPolicyAssignmentOutput and the List summary type genuinely don't carry it, confirmed against the same file, so iamPolicyAssignmentToMap was deliberately left alone). See TestQuickSight_ListIAMPolicyAssignmentsForUser, TestQuickSight_IAMPolicyAssignmentCRUD. FIXED (gopherstack-g3jk): ListIAMPolicyAssignments itself (the sibling of the ListIAMPolicyAssignmentsForUser fix above) reused iamPolicyAssignmentToMap unscoped, leaking AssignmentId/PolicyArn/Identities -- types.IAMPolicyAssignmentSummary (types.go:12309-12318) declares only AssignmentName/AssignmentStatus. Added iamPolicyAssignmentSummaryToMap scoped to those two fields, the same distinction ListIAMPolicyAssignmentsForUser's fix already documented but this sibling op had missed. See TestQuickSight_ListIAMPolicyAssignments_SummaryScoping, a raw-body assertion (an SDK client can't prove this: its deserializer silently drops unrecognized members)."}
  CustomPermissions: {status: ok, note: "CRUD + role membership + role/user custom-permission sub-families real (custompermissions.go, handler_custompermissions.go). RE-VERIFIED (gopherstack-taqn): the 'spot-checked against types.CustomPermissions -- fields match exactly' claim that stood here was FALSE. Diffed customPermissionsToMap (handler_custompermissions.go) against types.CustomPermissions in both aws-sdk-go-v2/service/quicksight@v1.123.1 and the installed @aws-sdk/client-quicksight TS defs (models_3.d.ts): both sources agree the real type carries a Governance (*Governance) field that this backend's own CustomPermissions struct (types.go) doesn't even have a slot for -- not stored on Create, not accepted, not returned on Describe. A genuine, unfixed field gap, not previously found. FIXED (gopherstack-hnyl): isValidRole was a hand-copied 8-entry allowlist that invented two nonexistent roles, RESTRICTED_AUTHOR and RESTRICTED_READER (types.Role only has 6 members) -- UpdateRoleCustomPermission/CreateRoleMembership accepted role values the real API would reject. Now derives from types.Role.Values()."}
  RefreshSchedule: {status: ok, note: "DataSet refresh-schedule + refresh-properties CRUD real (refreshschedule.go, handler_refreshschedule.go); classifyDataSetSubRes/SubResID decomposed from classifyDataSetPaths's flagged nolint this pass, behavior preserved verbatim. FIXED (gopherstack-0qzf): two gaps. (1) class (a), the exact bug class pkgs/awstime exists to prevent: StartAfterDateTime is a *time.Time on both types.RefreshSchedule (types.go:17365) and CreateRefreshScheduleInput.Schedule/UpdateRefreshScheduleInput.Schedule, serialized as an epoch-seconds JSON number (confirmed against serializers.go:50284's smithytime.FormatEpochSeconds and deserializers.go:111058's smithytime.ParseEpochSeconds). This backend modeled it as a plain string: a real client's numeric StartAfterDateTime was silently read as \"\" by strField (write side), and any stored value was echoed back as a JSON string a real client's deserializer would reject outright (\"expected Timestamp to be a JSON Number, got string instead\") on the read side. Fixed by changing storedRefreshSchedule/RefreshSchedule.StartAfterDateTime to time.Time and adding a shared epochField body-parsing helper (handler_paths.go) alongside pkgs/awstime.Epoch for the response side. (2) class (b): DescribeRefreshScheduleOutput (api_op_DescribeRefreshSchedule.go) carries a top-level Arn in addition to the nested RefreshSchedule.Arn; only the nested one was returned. Fixed. See TestQuickSight_RefreshSchedule_StartAfterDateTime."}
  AccountLevel: {status: ok, note: "large family: customizations, settings, subscription, IP restriction, key registration, public sharing, Q personalization/search config, SPICE capacity, default Q Business app, token-exchange grant, identity context, PredictQAResults (account.go, handler_account.go) -- all real, no stubs. RE-VERIFIED (gopherstack-taqn): the 'spot-checked AccountSettings/AccountInfo against SDK types, fields match' claim was only half true. AccountSettings (accountSettingsToMap) does genuinely match types.AccountSettings field-for-field (AccountName/DefaultNamespace/Edition/NotificationEmail/PublicSharingEnabled/TerminationProtectionEnabled, all 6 present). AccountInfo (handleDescribeAccountSubscription's response map) does NOT match: types.AccountInfo (confirmed against both aws-sdk-go-v2@v1.123.1 and the installed @aws-sdk/client-quicksight TS defs, models_0.d.ts) carries a 6th field, IAMIdentityCenterInstanceArn, that this backend's AccountSubscription struct (types.go) has no slot for at all -- a genuine, unfixed field gap. Only these two types named by the original claim were re-checked this pass; the family's other ~10 sub-resources (IPRestriction, key registration, Q personalization/search config, SPICE capacity, etc.) were not independently re-diffed and should not be assumed field-clean on the strength of this note. dispatchAccountConfig's flat switch decomposed into a sync.OnceValue map[op]handler-method table a prior pass, unrelated to this re-audit. RE-VERIFIED 2026-08-31 (bd gopherstack-6flj/21my, PARITY-gap targeting -- DescribeAccountSettings/DescribeDashboardsQAConfiguration/DescribeQPersonalizationConfiguration/DescribeQuickSightQSearchConfiguration named individually since none of the four appeared as a literal op name anywhere in this file before now, despite the family being audited): all four field-diffed against their own deserializers in quicksight@v1.123.1 (awsRestjson1, no case folding). AccountSettings wrapper key/fields, DashboardsQAStatus, PersonalizationMode, QSearchStatus all confirmed correct, no changes needed."}
  Embed: {status: ok, note: "GenerateEmbedUrlFor*, GetSessionEmbedUrl, GetDashboardEmbedUrl, GetIdentityContext (embedurl.go; internally named GenerateIdentityContext, matching its own doc comment) -- all real. RE-VERIFIED (gopherstack-taqn), this family's claim holds up: diffed all 6 ops' response maps against their real Output types (GenerateEmbedUrlForAnonymousUser/ForRegisteredUser/ForRegisteredUserWithIdentity, GetDashboardEmbedUrl, GetSessionEmbedUrl, GetIdentityContext) in aws-sdk-go-v2/service/quicksight@v1.123.1 -- every field (EmbedUrl/AnonymousUserArn/RequestId/Status/Context) is present, none extra, none missing. The behavioral claim also re-checked against embedurl.go directly: GenerateEmbedURLForAnonymousUser validates the namespace exists, GenerateEmbedURLForRegisteredUser validates the user exists when its ARN is parseable, GetDashboardEmbedURL validates the dashboard exists; GenerateEmbedURLForRegisteredUserWithIdentity performs no such lookup, but its own doc comment explains why (identity-enhanced sessions authenticate via signing credentials, not an explicit UserArn/accountID to validate) -- not a discrepancy. Every URL/token is freshly generated per call, matching real AWS's single-use, time-limited embed URLs."}
  Brand: {status: ok, note: "CRUD + assignment + published-version real (brands.go, handler_brands.go). RE-VERIFIED (gopherstack-taqn): the 'spot-checked against types.BrandDetail, fields match' claim was FALSE. Diffed brandToMap (handler_brands.go) against types.BrandDetail in aws-sdk-go-v2/service/quicksight@v1.123.1: three fields are missing from the emitted map. VersionStatus is the most notable -- the internal Brand struct (types.go) already tracks it as CurrentVersionStat, and a keyVersionStatus=\"VersionStatus\" JSON-key constant even exists in handler_brands.go, but it is never wired into brandToMap's returned map, so tracked data is silently dropped on every read. Errors ([]string) and Logo (*Logo) are missing too, but those are genuinely unbuildable: the internal Brand struct has no slot for either and no real per-brand error/logo state to derive them from, so that part is a structural gap, not a wiring bug like VersionStatus."}
  OAuthClientApplication: {status: ok, note: "CRUD real (oauth.go, handler_oauth.go). FIXED (gopherstack-0qzf): class (a). CreateOAuthClientApplicationInput.Tags (api_op_CreateOAuthClientApplication.go; OAuthClientApp ARNs are already taggable per arnCollectorFuncs) was the only Create handler in this backend NOT calling the tagsFromBody + b.tags[arn] pattern every sibling family (ActionConnector, VPCConnection, Template, Theme, Topic, Dashboard, Analysis, DataSet, DataSource, CustomPermissions, Folder, Agent, KnowledgeBase) already uses -- instead handleCreateOAuthClientApp's isOAuthAppModeledField catch-all dumped the raw \"Tags\" body value into the Extra passthrough bag, which oauthAppToMap then echoed back verbatim on every Describe/List call as a top-level Tags field. Confirmed against types.OAuthClientApplication/OAuthClientApplicationSummary (types.go:14837): neither has a Tags member -- real AWS never returns tags there; they only surface via ListTagsForResource. Fixed: Tags now excluded from the Extra bag and applied via the standard tagsFromBody path. See TestQuickSight_OAuthClientApp_CreateTags. Everything else in this family (ClientId/ClientSecret correctly never echoed, CreationStatus/UpdateStatus wire-accurate) re-verified clean. FIXED (gopherstack-wl0s, 2026-08-13): CreateOAuthClientApplication accepted a request omitting ClientId, ClientSecret, OAuthClientAuthenticationType, or OAuthTokenEndpointUrl -- all four are 'This member is required' per validateOpCreateOAuthClientApplicationInput. OAuthClientAuthenticationType/OAuthTokenEndpointUrl already round-tripped correctly through the Extra passthrough bag (matching the originating audit's claim); ClientId/ClientSecret are and remain write-only by design (no response-shape member exists for either), so their fix is presence-validation only, same as the other two, just without a round-trip to prove. OAuthClientAuthenticationType is additionally validated against types.OAuthClientAuthenticationType.Values() (currently just TOKEN) rather than a hand-copied check. All four now return InvalidParameterValueException (the code CreateOAuthClientApplication's own awsRestjson1_deserializeOpErrorCreateOAuthClientApplication switch declares) when absent. See validateCreateOAuthClientAppFields (handler_oauth.go) and TestQuickSight_CreateOAuthClientApp_PresenceValidation."}
  ActionConnector: {status: ok, note: "CRUD + search + permissions real (actionconnector.go, handler_actionconnector.go). CORRECTED (2026-08-23, continuation pass): the note that used to stand here claimed AuthenticationConfig redaction was 'NOT fixed... flagged for follow-up' -- that was stale. actionconnector_auth.go/redactAuthenticationConfig (added 2026-08-11, PR #2414, predating gopherstack-0qzf's audit note) implements the real ReadAuthConfig projection in full: redacts ApiKey/Password/ClientSecret per AuthenticationType, adds SourceArn for IamConnectionMetadata, renames the write-side ClientCredentialsDetails/AuthorizationCodeGrantCredentialsDetails wrappers to their read-side Read* names -- confirmed against types.go:16760 (ReadAuthConfig) and deserializers.go:109643+. Wired into actionConnectorToMap and tested end-to-end (TestQuickSight_ActionConnector_AuthConfigRedaction, actionconnector_redaction_test.go, 6 subtests covering every AuthenticationMetadata variant). Some earlier pass fixed this without updating the note -- same failure mode as the stale SearchTopics/DeleteTopic notes gopherstack-0qzf already caught once in this file. FIXED (2026-08-23, continuation pass): actionConnectorSummaryToMap (ListActionConnectors/SearchActionConnectors) omitted CreatedTime entirely even though it's a real, tracked, non-fabricated field on both this backend's ActionConnector struct and types.ActionConnectorSummary (types.go:197). See TestQuickSight_ListActionConnectors_Pagination. Rest of the family (CRUD, Search, Describe/UpdateActionConnectorPermissions envelope keys) diffed clean against ActionConnectorSummary/DescribeActionConnectorPermissionsOutput/UpdateActionConnectorPermissionsOutput."}
  IdentityPropagationConfig: {status: ok, note: "list/update/delete real (identitypropagation.go, handler_identitypropagation.go). AUDITED (gopherstack-0qzf), no findings: Update/DeleteIdentityPropagationConfigOutput carry no data fields beyond RequestId/Status (api_op_Update/DeleteIdentityPropagationConfig.go) and none are fabricated; ListIdentityPropagationConfigsOutput.Services ([]types.AuthorizedTargetsByService: Service/AuthorizedTargets, types.go:2324) matches handleListIdentityPropagationConfigs's response map key-for-key. Genuinely clean. FIXED (gopherstack-hnyl): isValidServiceType was a hand-copied 3-entry allowlist missing GLUE_DATA_CATALOG, the 4th types.ServiceType member -- UpdateIdentityPropagationConfig falsely rejected it. Now derives from types.ServiceType.Values()."}
  AssetBundle: {status: ok, note: "export/import job lifecycle real (assetbundle.go, handler_assetbundle.go). FIXED (gopherstack-0qzf): class (a). StartAssetBundleExportJobInput (api_op_StartAssetBundleExportJob.go) accepts IncludeFolderMembers/IncludeFolderMemberships/IncludePermissions/IncludeTags, all four echoed back on DescribeAssetBundleExportJobOutput -- none were read from the request body, stored, or returned; a caller setting IncludeTags=true had no way to observe it back. Fixed: threaded through Start/storedAssetBundleExportJob/AssetBundleExportJob/exportJobToMap. See TestQuickSight_AssetBundleExportJob_IncludeFlags. NOT fixed, flagged for follow-up: CloudFormationOverridePropertyConfiguration and ValidationStrategy (both structs, api_op_StartAssetBundleExportJob.go) are also accepted-and-dropped class (a) findings, but modeling them (even as opaque pass-through) was judged out of this pass's bounded-fix scope; ExportFormat-conditional CLOUDFORMATION_JSON behavior isn't modeled at all. Import job lifecycle (StartAssetBundleImportJobInput/Output, DescribeAssetBundleImportJobOutput) diffed clean -- no comparable gaps. FIXED (gopherstack-g3jk): ListAssetBundleExportJobs reused exportJobToMap (the Describe shape) for its list items, leaking ResourceArns/IncludeFolderMemberships/DownloadUrl/IncludeFolderMembers -- none of which types.AssetBundleExportJobSummary (types.go:1278-1308) declares. Added a separate exportJobSummaryToMap scoped to the summary's 8 real members, mirroring the sibling ListAssetBundleImportJobs/importJobToMap, which was already correctly scoped. An SDK client can't prove this (its deserializer silently drops unrecognized members); see TestQuickSight_ListAssetBundleExportJobs_SummaryScoping, a raw-body assertion."}
  Automation: {status: ok, note: "StartAutomationJob/DescribeAutomationJob real (automation.go, handler_automation.go). AUDITED (gopherstack-0qzf), no findings: StartAutomationJobInput has no InputPayload-adjacent fields this backend misses (confirmed against api_op_StartAutomationJob.go), DescribeAutomationJobOutput's conditional IncludeInputPayload/IncludeOutputPayload query-param gating is implemented correctly (handleDescribeAutomationJob). Genuinely clean."}
  DashboardSnapshotJob: {status: ok, note: "StartDashboardSnapshotJob(Schedule)/Describe*Result real (dashboardsnapshot.go, handler_assetbundle.go); classifyDashboardSubRes/SubResID/SubSubRes decomposed from classifyDashboardPaths's flagged nolint this pass, behavior preserved verbatim. AUDITED (gopherstack-0qzf), no findings: StartDashboardSnapshotJobInput's SnapshotConfiguration is stored/returned as an opaque pass-through document (matching the Dashboard.Definition precedent for deeply nested config this backend doesn't interpret), StartDashboardSnapshotJobScheduleOutput correctly carries no data fields (confirmed against api_op_StartDashboardSnapshotJobSchedule.go), and DescribeDashboardSnapshotJobResultOutput's Result wrapper (S3Uri) matches the real S3-download-URL shape. Genuinely clean."}
  Flow: {status: ok, note: "ListFlows/SearchFlows/GetFlowMetadata/permissions real (flow.go, handler_flow.go); as of the SDK's v1.121.0 bump CreateFlow/DescribeFlow/UpdateFlow/DeleteFlow now exist too and are implemented for real: CreateFlow generates a server-side FlowID (uuid.New, matching CreateFlowInput having no FlowId field), stores the caller's FlowDefinition document verbatim (map[string]any pass-through, like Dashboard.Definition elsewhere), and reports PublishState PUBLISHED (this backend has no draft/published divergence, matching the real op's documented auto-publish). DescribeFlow returns the FlowDetail shape (distinct field set from FlowSummary -- confirmed against types.FlowDetail: no RunCount/UserCount/LastPublishedAt/LastPublishedBy). StepAliases is always empty: real AWS derives it by parsing the flow definition's steps, which this backend stores opaquely rather than interpreting -- an honest omission, not fabricated. SeedFlow remains for tests that want FlowSummary-shaped fixtures without exercising Create. RE-AUDITED (gopherstack-0qzf), no findings: CreateFlowInput's ClientToken (idempotency-only, never echoed in any response, no observable effect either way) is the only unmodeled field; genuinely out of scope, not a wire-shape bug. Family confirmed clean."}
  SelfUpgrade: {status: ok, note: "config + request list/update real (selfupgrade.go, handler_selfupgrade.go); classifyNsSelfUpgradeConfig/Requests/UpdateSelfUpgrade decomposed from classifyNsWithSubRes's flagged nolint this pass. FIXED (gopherstack-0qzf): class (b). types.SelfUpgradeRequestDetail (types.go:18593) carries UserName (the requester); this backend's SelfUpgradeRequestDetail (types.go) had no slot for it at all, so it was silently absent from every ListSelfUpgrades/UpdateSelfUpgrade response. Since there is no real CreateSelfUpgradeRequest API (requests only enter state via the test-only seedSelfUpgradeRequest/SeedSelfUpgradeRequest), this is bounded, caller-supplied data exactly like OriginalRole/RequestedRole/RequestNote already are -- not fabrication. Fixed. See TestQuickSight_ListAndUpdateSelfUpgrades. RE-VERIFIED 2026-08-31 (bd gopherstack-6flj/21my, PARITY-gap targeting -- DescribeSelfUpgradeConfiguration named individually since it never appeared as a literal op name in this file before now): wrapper key SelfUpgradeConfiguration/SelfUpgradeStatus field-diffed against awsRestjson1_deserializeDocumentSelfUpgradeConfiguration in quicksight@v1.123.1; correct, no changes needed."}
  Agent: {status: ok, note: "new family (SDK v1.121.0): CreateAgent/DescribeAgent/UpdateAgent/DeleteAgent/ListAgents/SearchAgents/permissions real (agents.go, handler_agents.go), field-diffed against types.Agent/AgentSummary/CreateAgentOutput/UpdateAgentOutput (all PascalCase, confirmed via deserializers.go -- CreateAgentOutput uniquely uses AgentName, not Name). UpdateAgent's action-connector/space attach-detach validates each ARN against arnExists (a real, derived check) before accepting it, reporting genuine per-ARN failures in FailedToAdd*/FailedToRemove* rather than always succeeding. BUILT THIS PASS (parity-5): CustomPromptInput is a tagged union (verified against serializers.go), not one opaque blob -- its ExistingPrompt member (types.CustomPromptProfile: ModelProfileId/QbsAwsAccountId/SubscriptionId) is caller-supplied, referencing an already-provisioned Amazon Q Business profile, so it is now genuinely stored (Agent.CustomPrompt) and echoed back as CustomPromptInterface on Create/Update/Describe -- zero fabrication, since none of the three IDs originate in this backend. Missing one of the three required fields is now InvalidParameterValueException (400), not silently accepted. Remaining documented, non-fabricated omission: the NewPrompt union member (asks AWS to mint a brand-new profile server-side) is accepted without error but produces no CustomPromptInterface, because its IDs would have to come from a live Amazon Q Business subscription this backend has no state for -- synthesizing them would be fabrication (parity-principles.md rule 1). See TestQuickSight_Agents/CustomPromptInput_ExistingPrompt_round-trips_on_create_and_update, .../CustomPromptInput_ExistingPrompt_missing_a_required_field_is_rejected, .../CustomPromptInput_NewPrompt_is_accepted_but_not_echoed_back (handler_flow_test.go)."}
  KnowledgeBase: {status: ok, note: "new family (SDK v1.121.0): CreateKnowledgeBase/DescribeKnowledgeBase/UpdateKnowledgeBase/DeleteKnowledgeBase/BatchDeleteKnowledgeBase/ListKnowledgeBases/SearchKnowledgeBases/permissions real (knowledgebases.go, handler_knowledgebases.go), field-diffed against types.KnowledgeBase/KnowledgeBaseSummary. Found and correctly implemented a real API quirk: UpdateKnowledgeBase and UpdateKnowledgeBasePermissions are POST, not PUT, unlike every other resource family's Update* op in this backend -- confirmed against serializers.go, not assumed. Configuration/AccessControlConfiguration/MediaExtractionConfiguration are opaque pass-through documents (map[string]any), matching the Dashboard.Definition precedent for deeply-nested config blobs this backend has no processing logic for. BatchDeleteKnowledgeBase partitions per-ID success/failure for real (an unknown ID is a genuine per-item error, not swallowed into a whole-request failure)."}
  Space: {status: ok, note: "new family (SDK v1.121.0): CreateSpace/DescribeSpace/UpdateSpace/DeleteSpace/ListSpaces/SearchSpaces/permissions/ListSpaceResources/UpdateSpaceResources real (spaces.go, handler_spaces.go). Field-diffed against deserializers.go and found the Space family's wire shape is NOT PascalCase like every other family in this backend: spaceId/spaceArn are camelCase on every op's envelope, the nested Space/SpaceSummary document is fully camelCase, and UpdateSpacePermissionsOutput is uniquely fully-lowercase even for permissions/requestId (confirmed key-by-key against the deserializer switch statements, not assumed) -- see handler_spaces.go's wire-shape note. UpdateSpaceResources validates each resource ARN against arnExists before attaching it, same real-failure pattern as Agent's association updates. One documented, non-fabricated omission: DescribeSpace's Contributors is always an empty list and Space carries no ConsumedSourceSize/ConsumedSourceDocCount fields, because both require per-user raw-file-size attribution from a real ingestion pipeline this backend doesn't have -- an honest omission, matching the VPCConnection.NetworkInterfaces precedent from the prior pass. SECOND item, CORRECTED this pass (gopherstack-r80d, required-output-member sweep -- gopherstack-lx5h's prior conclusion here was wrong and is superseded): ListSpacesOutput/SearchSpacesOutput both declare a required top-level spaceId (SpaceArn is optional, not required -- gopherstack-lx5h mischaracterized neither as fabrication-worthy, but conflated the two) alongside the required spaceSummaries list -- verified against api_op_ListSpaces.go:44-63/api_op_SearchSpaces.go:49-68 and both ops' own deserializers.go switches. gopherstack-lx5h left spaceId/spaceArn entirely absent, reasoning that emitting an empty string would "misrepresent a real value" -- but a required Smithy output member is a structural wire guarantee from AWS's real server: leaving it absent means a real aws-sdk-go-v2 client's *string decodes nil, the exact "zero value where AWS guarantees content" bug this sweep hunts, not an honest omission. This is the same shape as this session's opensearch NextToken fix: when a required field has no natural per-call value (no single space is in scope for an account-wide list/search), the correct move is present-but-empty, not absent -- absence is what breaks the client, not what protects the caller from a misleading value. handleListSpaces/handleSearchSpaces (handler_spaces.go) now emit spaceId:\"\"/spaceArn:\"\" alongside spaceSummaries+requestId(+nextToken)."}
  UserIndexCapacity: {status: ok, note: "new op (SDK v1.121.0), ListUsersIndexCapacity: real, derived computation (userindexcapacity.go, handler_userindexcapacity.go) -- KBCount/SpaceCount and TotalKBCapacityBytes are computed by scanning this backend's actual KnowledgeBase/Space state for PrimaryOwnerArn/CreatedByArn matches against each user, never a fabricated placeholder. TotalSpaceCapacityBytes stays honestly 0 (Space carries no ConsumedSourceSize field to sum, per the Space family note above). Wire shape is fully camelCase (filters/maxResults/namespace/nextToken/sortBy/sortOrder on the request; nextToken/requestId/users on the response, with UserIndexCapacity's own fields all camelCase too) -- confirmed against (de)serializers.go, matching the Space family's convention rather than this backend's usual PascalCase."}
gaps:
  - TopicV2 cross-family field projection: a topic's V1-only fields (ConfigOptions,
    DataSets' full DatasetMetadata -- Columns/CalculatedFields/Filters/
    NamedEntities/DataAggregation) are not visible through DescribeTopicV2, and a
    topic's V2-only fields (DataSetRelations, the leaner TopicV2DataSetReference
    DataSets, CustomInstructions) are not visible through DescribeTopic (V1). This
    is a documented, non-fabricated omission, not a bug: TopicV2Details is not a
    losslessly-convertible schema of V1's TopicDetails (verified field-by-field
    against types.go -- neither is a superset of the other), and there is no SDK
    evidence describing how real AWS projects one schema's fields into the other's
    response, so synthesizing a translation would be exactly the kind of
    unverified claim parity-principles.md warns against. Both families do share
    the SAME TopicId/Arn/Name/Description/Permissions -- see topics_v2.go's doc
    comment and TestQuickSight_TopicV2_SharesResourceWithV1.
  # All 5 previously-named gaps fixed several passes back (UpdateDataSet ingestion
  # reporting, CancelIngestion terminal-status handling, Tag/Untag/ListTags ARN
  # existence check, Folder.SharingModel). parity-5: Agent.CustomPromptInterface's
  # ExistingPrompt path (caller-supplied IDs) was found to be genuinely buildable and
  # built -- see Agent family note. The two remaining non-fabricated omissions
  # (CustomPromptInput.NewPrompt, Space.Contributors/ConsumedSource*) are documented
  # choices, not gaps: parity-principles.md rule 1 says never fabricate a field this
  # backend has no real state to back, and both are safe, visible omissions
  # (nil/empty), not silently-wrong values.
  # gopherstack-i0n4 (separate task, same day): a 6th real gap surfaced and was fixed --
  # VPCConnection's DescribeVPCConnection/ListVPCConnections were emitting a top-level
  # SubnetIds field real AWS never returns from those ops (it's request-only, on
  # Create/UpdateVPCConnectionRequest). This was NOT caught by the "spot-checked in full
  # depth" pass claimed above for VPCConnection -- that claim was false and has been
  # corrected in the VPCConnection family note and the families preamble. Fixed by
  # dropping the field from vpcConnectionToMap; the model still stores/round-trips
  # SubnetIDs for Create/Update. See handler_vpcconnections.go, handler_vpcconnections_test.go.
deferred: []
  # All families audited across the prior and this pass; see families above. None
  # remain deferred.
leaks: {status: clean, note: "no goroutines/timers/janitors found in this service -- it's a synchronous in-memory backend behind a single coarse lockmetrics.RWMutex. DeleteUser's groupMembers cleanup (fixed prior pass) and DeleteGroup's groupMembers cleanup (re-verified this pass, already correct) both cascade-clean group membership rows on delete. DeleteFolder cascade-cleans folderMembers rows the same way. DeleteAgent/DeleteKnowledgeBase/DeleteSpace/DeleteFlow (new this pass) all cascade-clean their tags map entries the same way as every other delete in this backend (see arnCollectorFuncs in tags.go, extended this pass to recognize Agent/KnowledgeBase/Space ARNs so TagResource/UntagResource/ListTagsForResource work on them too). No ghost rows found in any family audited this pass."}

---

## Notes

### 2026-08-29 (filter/pagination-not-honoured sweep, partial)

This service is large (277 `api_op_*.go` files; ~40 List/Search ops
return a collection by output shape) and this pass did not audit it
exhaustively -- see "what remains unworked" below. Time was spent
verifying the established `Search*` pattern (`folderFiltersFromBody` +
`maxResultsParam`/`nextTokenParam`, shared by `SearchDashboards`/
`SearchAnalyses`/`SearchDataSets`/`SearchDataSources`/`SearchFolders`/
`SearchFlows`/`SearchSpaces`/`SearchKnowledgeBases`/
`SearchActionConnectors`/`SearchAgents`) and finding the one op that
didn't follow it.

**Found and fixed:** `SearchGroups` -- see its `ops:` entry above. Two
bugs: the real (required) `Filters` member was never read (a
nonexistent `"Query"` body field was read instead, so a real client's
`GROUP_NAME`/`StartsWith` filter was silently dropped and every group in
the namespace always came back), and `MaxResults`/`NextToken` were read
from the JSON body when this op actually query-binds both
(`max-results`/`next-token`) -- confirmed per-op against
`serializers.go`, not assumed from the sibling `Search*` ops' pattern,
since `SearchTopics`/`SearchTopicsV2` genuinely *are* body-bound for
those same two parameters (also verified against `serializers.go`) --
a live instance of the "same-named parameter binds differently per
operation" trap this campaign has repeatedly flagged elsewhere.
New tests `TestSearchGroups_Filters`/`TestSearchGroups_Pagination`
(`list_filter_params_test.go`) drive the real SDK client and fail
against the pre-fix code.

**Checked and already correct, no change:** `SearchDashboards` (`Name`
filter with `StringEquals`/`StringLike` operators and AND semantics
across multiple filters, correctly implemented in `matchesNameFilter`/
`matchesAllNameFilters`, folders.go; ownership-related filter names
explicitly and correctly treated as pass-through since this backend
doesn't track principals), `SearchTopics`/`SearchTopicsV2` (Filters/
MaxResults/NextToken all body-bound and all read from the body
correctly).

**What remains unworked:** `SearchFlows`, `SearchSpaces`,
`SearchKnowledgeBases`, `SearchActionConnectors`, `SearchAgents`,
`SearchAnalyses`, `SearchDataSets`, `SearchDataSources`, `SearchFolders`
were confirmed to call the shared `folderFiltersFromBody`/
`maxResultsParam`/`nextTokenParam` pattern (so are unlikely to share
`SearchGroups`' binding bug) but their filter *semantics* (which
`Name`/`Operator` combinations each op's own backend method actually
applies, matching the real per-op filter type such as
`AnalysisSearchFilter`/`DataSetSearchFilter`) were not verified
field-by-field this pass. Plain `List*` ops (`ListDataSets`,
`ListDashboards`, `ListAnalyses`, `ListTemplates`, `ListThemes`,
`ListNamespaces`, `ListVPCConnections`, and the rest -- these take only
MaxResults/NextToken in the real API, no filter/sort member) were not
individually re-verified for pagination correctness this pass beyond
the general pattern already documented elsewhere in this file. This is
reported as scope-remaining, not "audited and clean."

### 2026-08-29 (pagination-arithmetic sweep)

Follow-up to the note directly above: this pass audited exactly the
gap it left open -- the arithmetic inside every plain `List*` op's
`MaxResults`/`NextToken` pagination (not filter semantics, not
`Search*` binding). Census: ~40 `List*` backend methods, none call
`pkgs/page` -- every one hand-rolls its own cursor window, either via
one of 8 small shared `paginate<Type>` helpers (agents.go, group.go,
flow.go, actionconnector.go, iampolicyassignments.go, spaces.go,
knowledgebases.go, userindexcapacity.go) or inline in the `List*`
method itself (the majority).

**Two bug classes found, both systemic (not per-op mistakes):**

- **Class A (panic).** 7 helpers encode the cursor as a raw integer
  offset (`encodePageToken`/`decodePageToken`, store.go) with no upper
  bound check: `paginateFolders`, `paginateNamespaces`,
  `ListTemplates`, `ListDashboardVersions`, `ListVPCConnections`,
  `ListThemes`, `ListBrands`. A token issued before items were deleted
  can decode to an offset past the new, shorter collection, and
  `all[start:end]` panics (`slice bounds out of range`) instead of
  returning an empty page. `pkgs/page.New` already has the guard
  (`start >= len(all)` returns `Page{}`) these five never adopted.
  Fixed by clamping `start` to `len(all)` (or the version count, for
  `ListDashboardVersions`) right after decoding, matching `pkgs/page`'s
  behavior without changing the wire-compatible token format (both use
  the same `base64(strconv.Itoa(offset))` encoding).
- **Class B (infinite loop).** 28 call sites (8 shared helpers + 20
  inline `List*`/`ListXVersions`/`ListXAliases`/`ListXMembers`
  methods) search linearly for the item named by an equality-matched
  cursor and leave `start` at its zero value on a miss -- a client
  whose cursor names a since-deleted item gets page one forever, never
  terminating. Fixed uniformly: default `start` to `len(collection)`
  (end, not beginning) when the cursor doesn't resolve, matching the
  safe pattern `ssoadmin.paginateOrdered` already uses in this repo.
- **Adjacent (unsorted collection).** 9 operations
  (`ListAnalyses`, `ListDataSources`, `ListDataSets`, `ListDashboards`,
  `ListUsers`, `ListIngestions`, `ListGroups`, `ListUserGroups`,
  `ListGroupMemberships`) paginated a slice built straight from
  `store.Table.All()` (or a raw map range) with no `sort.Slice`/
  `sort.Strings` call -- `Table.All()`'s doc comment is explicit that
  iteration order is unspecified. Two back-to-back calls with no
  mutation in between could already drop or duplicate items purely
  from Go's randomized map iteration, independent of the cursor bugs
  above. Their `Search*` siblings already sorted (compared side by
  side, e.g. `ListDataSets` vs `SearchDataSets` in dataset.go); fixed
  by adding the same sort to each.

**Verified clean, no bug:** `ssoadmin`'s three pagination helpers
(`paginateStrings`/`paginateBy` use threshold search — `keyFn(item) >=
cursor` — which cannot express Class A/B/C by construction;
`paginateOrdered` uses equality search but already defaults to
`len(items)` on a miss). Not touched.

New tests: `pagination_arithmetic_test.go` -- table-driven boundary
walk (N=7 items, page size 3, concatenation reproduces the exact
collection), stale-cursor (Class A and B), and final-page/empty/exact
checks against `ListGroups` (shared-helper + unsorted shape),
`ListAnalyses` (inline + unsorted shape), `ListFolders` and
`ListTemplates` (index-cursor/Class A shape). All four failed against
the pre-fix code (confirmed panics/duplicated items in this pass), and
pass after the fix. The full existing suite
(`go test -race ./services/quicksight/...`) also still passes.
Confirmed through the real typed client (`aws quicksight create-group`
x5, `list-groups --max-results 2` across 3 pages, `delete-group` +
re-list with the deleted item's stale token -> empty page, not page
one again).

**Not touched, left recorded:** the ~20 `Search*` ops' own filter
*semantics* (as scoped out by the note above -- this pass only
verified the pagination arithmetic downstream of whatever the filter
step already returned). The unused `filter/-` alignment in the
`CustomPermissions`/`RoleMemberships`/`FolderMembers`/
`FoldersForResource` families' non-page-size list bodies was not
re-examined; only their pagination cursors were in scope and were
fixed as part of Class B above.

### 2026-08-29 (error-path sweep: what a typed client sees on failure)

Extracted all 277 `awsRestjson1_deserializeOpError<Op>` switches from quicksight@v1.123.1's
deserializers.go. quicksight's error-write mechanism is structurally different from most other
gopherstack services: there is no per-sentinel wire-code lookup table — every handler calls a
single shared `httpErr(c, err)` (handler_paths.go) that classifies by the sentinel's
**category** (`awserr.ErrNotFound`/`ErrAlreadyExists`/`ErrConflict`/`ErrInvalidParameter`) and
writes one of exactly 4 hardcoded wire codes (`ResourceNotFoundException`/`ConflictException`/
`ConflictException`/`InvalidParameterValueException`) — plus a generic `InternalFailure`
fallback. The specific `Code` string passed to each sentinel's `awserr.New(...)` call in
errors.go is otherwise discarded on the wire.

**This looked like a systemic bug and turned out not to be one — recorded because it took real
verification to rule out.** ~57 ops model `ResourceExistsException` distinctly from
`ConflictException` for their own "already exists" case (real AWS QuickSight uses both, for
different conditions within the same op), and all 20 of the `errResourceExists`-coded sentinels
in errors.go (`ErrFolderAlreadyExists`, `ErrTemplateAlreadyExists`, `ErrAgentAlreadyExists`, ...)
would be wrongly flattened to `ConflictException` by `httpErr`'s hardcoded default. Checked every
one: every single raise site already has a call-site-local workaround (`if errors.Is(err,
ErrXAlreadyExists) { return writeError(c, http.StatusConflict, errResourceExistsCode,
err.Error()) }` before falling through to `httpErr`) — confirmed by grepping each sentinel's
raise site(s) against its workaround site(s) 1:1 (e.g. `ErrTopicAlreadyExists` has two raise
sites, in `topics.go` and `topics_v2.go`, each with its own matching workaround in
`handler_topics.go`/`handler_topics_v2.go`). Same for the one `PreconditionNotMetException`
sentinel (`ErrAccountTerminationProtectionEnabled`, `handler_account.go:244`). Genuinely clean —
not fixed, because there was nothing to fix.

**Real bug found and fixed**: `GetFlowMetadata`, `GetFlowPermissions`, `UpdateFlowPermissions`
raised `ErrFlowNotFound` (wire `ResourceNotFoundException`) for an unresolvable `FlowId` — the
same sentinel their siblings `DescribeFlow`/`UpdateFlow`/`DeleteFlow` correctly use. But unlike
those three, none of these ops model `ResourceNotFoundException` in their own deserializer; they
model only `InvalidParameterValueException`, `AccessDeniedException`, `InternalFailureException`,
`ThrottlingException` — a real, deliberate asymmetry in AWS's own Smithy model for this
newer/permissions-scoped corner of the Flow API family. Repointed all three call sites to
`ErrValidation` (wire `InvalidParameterValueException`). Two existing tests
(`handler_flow_test.go`) asserted the wrong 404/`ResourceNotFoundException` behavior as correct
and were fixed. Covered by `error_path_sweep_test.go` (real `aws-sdk-go-v2/service/quicksight`
client, `errors.As` against `types.InvalidParameterValueException`).

**Method note**: found by diffing, for each of the 277 ops, its own modeled code set against the
4 codes `httpErr`/its workarounds can ever emit, and flagging any op with zero overlap on a
condition gopherstack actually raises for it (27 ops model none of
`ResourceNotFoundException`/`ConflictException`/`ResourceExistsException`/
`InvalidParameterValueException`/`PreconditionNotMetException`; of those, only the three Flow
permission ops had a live not-found raise site — the rest are List/Search ops with no natural
not-found condition, or `BatchDeleteKnowledgeBase`, whose per-item failures are correctly
reported in the success response body rather than as a top-level exception, confirmed by reading
its backend method).

**Not exhaustively re-verified**: given the sheer op count (277) and that the shared
category-based mechanism narrows the space where a wrong-code bug can hide (mixing up which
*specific* not-found/conflict sentinel to raise is wire-invisible here, unlike ssm/cognitoidp,
since same-category sentinels collapse to the same code), this pass targeted the two highest-
yield angles — the category-flattening theory (false alarm) and the no-core-code-modeled op list
(real bug, fixed) — rather than tracing every op's full call graph as was done for ssm/cognitoidp.
A deeper pass could still check for wrong-*category* selections (e.g. a condition raising
`ErrAlreadyExists`/`ErrConflict` where the op's model wants `ErrInvalidParameter`, or vice versa)
across the remaining ~250 ops not covered here.

### Notes below this line predate the 2026-08-29 error-path sweep.

Protocol: **REST-JSON (restjson1)**, not action-header dispatch -- routing is by HTTP
method + URL path (`classifyRequest` in handler.go), unlike most gopherstack services
that dispatch on an `X-Amz-Target`-style op header. `GetSupportedOperations()` still
enumerates the full op catalog for chaos-injection wiring.

Timestamps: all `CreatedTime`/`LastUpdatedTime` fields go over the wire as
`.Unix()` epoch-seconds numbers (correct for this JSON protocol) rather than via
`pkgs/awstime.Epoch` -- functionally equivalent, but worth normalizing to the shared
helper in a future pass for consistency with other services' bug history.

`Status`/`CreationStatus`/`UpdateStatus`/`ResourceStatus` fields all share the real
SDK's seven-value `types.ResourceStatus` enum: `CREATION_IN_PROGRESS`,
`CREATION_SUCCESSFUL`, `CREATION_FAILED`, `UPDATE_IN_PROGRESS`, `UPDATE_SUCCESSFUL`,
`UPDATE_FAILED`, `DELETED`. This backend only ever synthesizes the terminal-success
values (`*_SUCCESSFUL`) or `DELETED`, never the `_IN_PROGRESS`/`_FAILED` states, which
is fine for parity (this backend has no async failure modes) but means a client
polling for `CREATION_IN_PROGRESS` to flip will never observe it -- everything is
synchronously done. Before this pass, Dashboard alone used the invalid literal
`"CREATED"` for this field family; that's fixed now, so **all** resource types
consistently use only real enum values. Don't reintroduce a bespoke "CREATED" string.

`CreateDataSetOutput`/`UpdateDataSetOutput` both document `IngestionArn`/`IngestionId`
as "triggered as a result of dataset creation if the import mode is SPICE" -- i.e.
these fields are conditional on ImportMode, not unconditional. Before this pass,
`CreateDataSet` fabricated `IngestionArn: "{arn}/ingestion/auto"` /
`IngestionId: "auto"` unconditionally, for every import mode, without ever creating a
backing `Ingestion` record -- a classic disguised-no-op (see parity-principles.md
rule 1: fabricated IDs that skip real state). A client calling
`DescribeIngestion(dataSetId, "auto")` right after `CreateDataSet` would get a 404
despite the create response claiming that ingestion existed. Fixed by having
`CreateDataSet` create a real `storedIngestion` (status `COMPLETED`, since this
backend has no async pipeline) when, and only when, `ImportMode == "SPICE"`, and
omitting `IngestionArn`/`IngestionId` entirely for `DIRECT_QUERY`.

ARN construction: every resource type in this backend builds ARNs via
`pkgs/arn.Build` (partition derived from region -- GovCloud/China/ISO-correct)
**except** `CreateIngestion`, which used to hand-format
`fmt.Sprintf("arn:aws:quicksight:%s:%s:dataset/%s/ingestion/%s", ...)` with a
hardcoded `"aws"` partition. Fixed to use `arn.Build` like every other resource.
Grep for `fmt.Sprintf("arn:` before adding new resource types to catch regressions.

`dashboardToMap()` (handler.go) is the single place that flattens a `*Dashboard` for
`DescribeDashboard`/`ListDashboards` JSON responses; it had a copy-paste bug reading
`d.VersionNumber` into the `PublishedVersionNumber` wire key instead of
`d.PublishedVersionNumber` -- the two fields diverge as soon as
`UpdateDashboardPublishedVersion` is called with anything other than the latest
version, or `UpdateDashboard` bumps `VersionNumber` without a matching publish. Fixed.

Group membership storage (`b.groupMembers map[string]bool`, not a `store.Table`) is
keyed `"{accountID}/{namespace}/{groupName}/{memberName}"` with no escaping -- this is
safe only because namespace/group/member names are assumed not to contain literal
`/` characters, consistent with every other composite key in this file (`userKey`,
`dataSourceKey`, etc.). Don't add resource names with `/` without revisiting all of
these key builders.

## SDK v1.112.0 -> v1.121.0 bump (this pass)

The Go SDK module was bumped, revealing 32 operations across four new/extended
families that didn't exist at the prior audit: Agent, Flow's
Create/Describe/Update/Delete (Flow itself was already a family), KnowledgeBase,
Space, and ListUsersIndexCapacity. All 32 are implemented for real (see families
above) and added to `GetSupportedOperations()` -- none were parked in
`TestSDKCompleteness`'s `notImplemented` list.

**Routing: KnowledgeBase and Space are minted under `/v1/accounts/...`, not
`/accounts/...`.** Confirmed against `aws-sdk-go-v2/service/quicksight`'s
`serializers.go`: every other family's `opPath` starts
`/accounts/{AwsAccountId}/...`; these two start `/v1/accounts/{AwsAccountId}/...`.
`handler_paths.go`'s `stripV1Prefix` drops the leading `"v1"` segment so the rest of
the routing/handler code treats them identically to every other
`/accounts/{id}/{resourceType}/...` family. **Both `classifyRequest` (routing) and
`pathSegsFromCtx` (every handler's own segment re-parse) call `stripV1Prefix`** --
an earlier version of this fix only stripped it in `classifyRequest`, which routed
correctly but left every KnowledgeBase/Space handler re-parsing the *unstripped*
path for its own `seg(segs, segAccountID)`/`seg(segs, segResID)` calls, silently
reading the wrong segments (e.g. accountID becoming the literal string
`"accounts"`). Caught by `TestQuickSight_KnowledgeBases`/`TestQuickSight_Spaces`
before landing. If a future op adds a third path-prefix convention, route it through
a shared strip helper the same way, not a copy-pasted one-off in `classifyRequest`
alone.

**`RouteMatcher` needed a matching update.** QuickSight's `RouteMatcher` gates on a
literal path-prefix check (`/accounts/` or `/resources/`) before checking the
`Authorization` header's signing-service name; a `/v1/accounts/...` request would
never have reached the handler at all without adding `quicksightV1PathPrefix` to
that check. Safe to add broadly since the `Authorization` header check still
disambiguates from any other service that might also use a `/v1/...` path
(`isQuickSightRequest`).

**Space's wire shape breaks this backend's PascalCase convention** -- see the
Space family note above and the wire-shape comment at the top of
`handler_spaces.go`'s const block. `ListUsersIndexCapacity` is fully camelCase too
(matching Space's convention, not this backend's usual PascalCase). Don't
"normalize" either to PascalCase in a future pass; they are faithfully replicating
a real, inconsistent upstream API, confirmed key-by-key against
`(de)serializers.go`, not assumed from pattern-matching the rest of the SDK.

`UpdateKnowledgeBase`/`UpdateKnowledgeBasePermissions` are POST, not PUT -- the one
resource family in this backend where "Update" doesn't map to an HTTP PUT.
Confirmed against `serializers.go`; don't "fix" `classifyKnowledgeBasePaths` to use
PUT by pattern-matching every other family.

`Agent`/`Space` association updates (`UpdateAgent`'s action-connector/space
attach-detach, `UpdateSpaceResources`) validate each referenced ARN against
`arnExists` before accepting it, reporting genuine per-ARN failures rather than
always succeeding -- the same real-failure pattern, and reusing the same
`arnExists` helper `tags.go` already had for `TagResource`. `arnCollectorFuncs` was
extended this pass to include Agent/KnowledgeBase/Space ARNs so both this
validation and `TagResource`/`UntagResource`/`ListTagsForResource` work on the new
resource types.

## SDK v1.121.0 -> v1.123.1 bump (this pass)

The Go SDK module was bumped again, revealing 8 new operations: the TopicV2
("Q topics") family -- `CreateTopicV2`, `DescribeTopicV2`, `UpdateTopicV2`,
`DeleteTopicV2`, `ListTopicsV2`, `SearchTopicsV2`,
`DescribeTopicPermissionsV2`, `UpdateTopicPermissionsV2`. All 8 are
implemented for real (see the `Topic` family note and the per-op notes under
`ops:` above) and added to `GetSupportedOperations()`; none were parked in
`TestSDKCompleteness`'s `notImplemented` list. New files: `topics_v2.go`
(backend), `handler_topics_v2.go` (wire/routing).

**TopicV2 and V1 Topic are the same underlying resource, confirmed against
the SDK, not assumed from the similar names** -- see `topics_v2.go`'s doc
comment for the full evidence trail. This drove the whole design: both
families read/write the same `b.topics` collection keyed by
`topicKey(accountID, topicID)`, so `CreateTopic`/`CreateTopicV2` collide on a
shared `TopicId`, `DeleteTopic`/`DeleteTopicV2` delete the one record, and
permissions/tags/ARN are shared. Only `CreateTopicV2`/`UpdateTopicV2` needed
new `StorageBackend` methods (a genuinely different accepted parameter set);
`DescribeTopicV2`/`DeleteTopicV2`/`ListTopicsV2`/`SearchTopicsV2` call the
existing V1 `DescribeTopic`/`DeleteTopic`/`ListTopics`/`SearchTopics`
directly, and `DescribeTopicPermissionsV2`/`UpdateTopicPermissionsV2` route
straight to the existing V1 permission handlers (byte-identical wire shape,
confirmed key-by-key against the deserializers) -- see
`handler_topics_v2.go`'s `dispatchTopicV2` doc comment.

**`SearchTopicsV2` puts `MaxResults`/`NextToken` in the JSON body, not query
params** -- confirmed against `awsRestjson1_serializeOpDocumentSearchTopicsV2Input`
(its HTTP-bindings function binds only `AwsAccountId`), unlike `ListTopicsV2`
which uses `max-results`/`next-token` query params (confirmed against
`awsRestjson1_serializeOpHttpBindingsListTopicsV2Input`). Implemented
correctly for `SearchTopicsV2`; see `TestQuickSight_SearchTopicsV2`'s
body-pagination assertions.

**Pre-existing V1 wire-shape findings, NOT fixed this pass (out of this
task's assigned scope, which was the 8 TopicV2 ops only -- flagging for a
follow-up pass):**

- `SearchTopics` (V1)'s real `SearchTopicsInput` puts `MaxResults`/`NextToken`
  in the JSON body (same as `SearchTopicsV2`, confirmed against
  `awsRestjson1_serializeOpDocumentSearchTopicsInput`), but this backend's
  existing `handleSearchTopics` reads them from query params via
  `maxResultsParam(c)`/`nextTokenParam(c)` -- a real client's `MaxResults`/
  `NextToken` would be silently ignored. `SearchTopicsV2` was implemented
  correctly (body-based) rather than copying this bug forward.
- `DeleteTopic` (V1)'s real `DeleteTopicOutput` carries an `Arn` field
  (confirmed against `api_op_DeleteTopic.go`), but this backend's existing
  `handleDeleteTopic` response omits it. `DeleteTopicV2`'s response correctly
  includes `Arn` rather than copying this omission forward.

## required-member sweep pass 3 (gopherstack-2qk4)

`CreateDataSet`/`UpdateDataSet` never read `PhysicalTableMap`, required at
`quicksight@v1.123.1` `api_op_CreateDataSet.go:55` and
`api_op_UpdateDataSet.go:55` -- the field that defines what a dataset's rows
actually come from. Grepping the service for `PhysicalTableMap` or
`LogicalTableMap` previously returned zero hits anywhere: not the handler, not
the model, not storage. A dataset was created, reported success, and had
nothing behind it.

Fixed: `types.go` models `PhysicalTable` as a struct with one populated
pointer per wire union member (`RelationalTable`, `CustomSql`/`CustomSQL` in
Go, `S3Source`, `FileSource`, `SaaSTable`), each with its own real fields
(`InputColumn`, `UploadSettings`, `TablePathElement`) rather than flattened
onto `DataSet`. `dataset_physicaltable.go` parses/validates/stores/echoes the
full map for `CreateDataSet`/`UpdateDataSet`/`DescribeDataSet`; `ListDataSets`/
`SearchDataSets` correctly omit it, matching `DataSetSummary`'s (not `DataSet`'s)
real shape. `LogicalTableMap` (optional) is modeled the same way for `Alias`/
`Source` (`LogicalTableSource`, including a real `JoinInstruction`), but its
`DataTransforms` member is left inert: `TransformOperation` is a 10+-variant
union (`CastColumnTypeOperation`, `CreateColumnsOperation`,
`FilterOperation`, `RenameColumnOperation`, ...) this in-memory backend never
evaluates, so the caller's raw JSON is stored and echoed back verbatim
instead of being lossily re-derived into typed structs nothing acts on --
the same treatment `Dashboard`/`Analysis.Definition` already give other
genuinely-open-ended blobs elsewhere in this package. `JoinInstruction`'s
optional `LeftJoinKeyProperties`/`RightJoinKeyProperties` are similarly left
unmodeled for the same reason (never applied, so no backing state either way).

Both ops now reject a request with no physical tables (`ErrValidation`,
`InvalidParameterValueException`) rather than silently accepting one, and
`UpdateDataSet` replaces (not merges) the stored map on every call, matching
its real full-replace contract ("Partial updates are not supported by this
operation"). See `TestSDKRoundTrip_DataSetPhysicalTableMap` and
`TestSDKRoundTrip_UpdateDataSetPhysicalTableMap` (`handler_sdk_roundtrip_test.go`),
which drive the real `aws-sdk-go-v2` client end to end -- create with a
populated `PhysicalTableMap`/`LogicalTableMap`, then describe and assert the
exact typed union variant and field values round-trip, not just a 2xx.

## gopherstack-y1zn (2026-08-21): unknown-key sweep, 5 confirmed bugs

Part of the gopherstack-us9u/g479 map-literal scanner's 526-key unknown-key
bucket triage. All 5 proven via real `aws-sdk-go-v2/service/quicksight`
client round trips or raw-body assertion, hand-reverted against `git show
HEAD:<path>`, confirmed failing, restored, `md5sum`-verified byte-identical.

- `CreateAccountSubscription`: {wire: fixed} -- `SignupResponse.UserLoginName`
  was emitted PascalCase ("UserLoginName"); the real member
  (deserializers.go's awsRestjson1_deserializeDocumentSignupResponse) is the
  one lowercase-first member on that type, "userLoginName".
- `DescribeKeyRegistration`: {wire: fixed} -- wrapped the list under
  "RegisteredCustomerManagedKeys" (the name of the array's own item type,
  types.RegisteredCustomerManagedKey); real member is "KeyRegistration".
- `DescribeDefaultQBusinessApplication`: {wire: fixed} -- wrapped its result
  under a fabricated "DefaultQBusinessApplication" key with a "Namespace"
  echo; the real output (deserializers.go) is flat -- ApplicationId/RequestId
  only, no wrapper, no Namespace (Namespace is Input-side only).
- `BatchCreateTopicReviewedAnswer`/`BatchDeleteTopicReviewedAnswer`: {wire:
  fixed} -- wrapped the succeeded list under "SucceededAnswer" (singular);
  real member is "SucceededAnswers" (plural).
- `ListTopicReviewedAnswers`: {wire: fixed} -- each item emitted a fabricated
  "Mode" key; types.TopicReviewedAnswer has no such member
  (AnswerId/Arn/DatasetArn/Mir/PrimaryVisual/Question/Template only; Arn/Mir
  remain honestly absent gaps, not touched this pass).
- `UpdateSelfUpgrade`/`ListSelfUpgrades` (selfUpgradeRequestDetailToMap):
  {wire: fixed} -- "LastUpdateAttemptTime"/"LastUpdateFailureReason" were
  PascalCase, matching every sibling member on the type; the real wire keys
  (deserializers.go's awsRestjson1_deserializeDocumentSelfUpgradeRequestDetail)
  are lowercase-first for these two specific members only, unlike every other
  member on the same type.

New test file: `wire_field_fixes_y1zn_test.go`.

## 2026-08-23: low-client-coverage audit pass, coverage number confirmed but scope corrected

`cmd/clientcoverage` measures quicksight at 25/277 real-SDK-client-driven ops
(9.0%), the lowest of any large service -- that number is accurate, confirmed
by running `cmd/opcensus`/`cmd/clientcoverage` directly. But it does NOT mean
252 ops were "never checked": this file's own history (gopherstack-0qzf,
taqn, i0n4, g3jk, hnyl, r80d/lx5h, 2qk4, y1zn, wl0s, parity-5, spanning back
to 2026-08-08 and as recent as 2026-08-21) already documents a near-complete
hand field-diff of nearly every op family against the pinned SDK's
types.go/(de)serializers.go, with concrete SDK line citations. Spot-checked
five of those citations directly against
`aws-sdk-go-v2/service/quicksight@v1.123.1` this pass (ListIAMPolicyAssignmentsForUser's
ActiveAssignments key, Space's camelCase spaceId, DescribeRoleCustomPermission's
CustomPermissionsName, ListRoleMemberships' MembersList, Brand's published-version/
assignment sub-op wire shapes) -- all five matched the real SDK source exactly, so
the audit trail is genuine, not aspirational. The low client-coverage number
reflects methodology (most of this file's findings came from hand-diffing
against SDK source, not from live-client round-trip tests) rather than
neglect. Same correction shape as bedrock's audit this session: "never
counted by clientcoverage" is not "never checked."

One genuine, previously-missed gap found and fixed this pass anyway, in a
family whose PARITY.md coverage was narrative rather than per-op
(`Analysis`, ops: section only listed 9 of the family's actual ops -- see the
per-op notes above): `CreateAnalysisInput.ThemeArn`/`UpdateAnalysisInput.ThemeArn`
(real, caller-supplied, optional `*string` fields) were accepted and silently
dropped -- `Analysis` (types.go) had no field for them at all, so a real
client's `ThemeArn` was never observable on `DescribeAnalysis` or
`DescribeAnalysisDefinition` even though both real output shapes carry it
(`types.Analysis.ThemeArn`, `DescribeAnalysisDefinitionOutput.ThemeArn`).
Class (a), zero fabrication. Proven with a real `aws-sdk-go-v2` client round
trip (`TestSDKRoundTrip_AnalysisThemeArn`), confirmed failing against the
pre-fix code by hand-reverting `analysis.go`/`handler_analysis.go`/
`models.go`/`types.go`/`interfaces.go` to `git show HEAD:<path>` (all three
assertions failed with the expected-vs-empty diff), then restored via `cp`
and `md5sum`-verified byte-identical to the fixed version.

Note on persistence: `storedAnalysis` gained a new `ThemeArn` field
(purely additive -- every existing field/tag unchanged). This does NOT
warrant a `quicksightSnapshotVersion` bump -- bumping for a purely-additive
change is exactly the destructive pattern `pkgs/persistence`'s
`TestSnapshotVersionGuard` exists to catch (an older snapshot decodes a
missing field to its zero value fine; bumping instead routes `Restore`
through `ResetAll` and discards every user's persisted state). In practice
the guard doesn't even reach quicksight: it scans `services/*/persistence.go`
for a `<service>SnapshotVersion`-carrying struct, and quicksight's
`persistence.go` only has `Handler.Snapshot`/`Restore` delegation methods --
the version constant lives in `store.go` instead -- so quicksight has no
entry in `pkgs/persistence/testdata/snapshot_inventory.json` at all and
`TestSnapshotVersionGuard` silently doesn't cover this service either way.
Confirmed by running it before and after this change: no violations, no
golden diff, no `quicksight` key present in the golden JSON.

Ops audited this pass beyond the 5 spot-checks above (all clean, no new
findings besides Analysis/ThemeArn): `DescribeBrandPublishedVersion`,
`UpdateBrandPublishedVersion`, `DescribeBrandAssignment`,
`UpdateBrandAssignment`, `DescribeRoleCustomPermission`,
`UpdateRoleCustomPermission`, `DeleteRoleCustomPermission`,
`CreateRoleMembership`, `DeleteRoleMembership`, `ListRoleMemberships`,
`UpdateUserCustomPermission`, `DeleteUserCustomPermission`.

Not reached this pass (named per the campaign's depth-over-breadth rule,
not implying a problem -- most already carry a dated, cited family-level
audit above): the full `Brand` CRUD (`CreateBrand`/`DeleteBrand`/
`DeleteBrandAssignment`/`ListBrands`), `CustomPermissions` CRUD proper
(`CreateCustomPermissions`/`DeleteCustomPermissions`/`DescribeCustomPermissions`/
`ListCustomPermissions`/`UpdateCustomPermissions`), the `AccountLevel` family's
~10 sub-resources beyond `AccountSettings`/`AccountInfo` (already flagged as
not independently re-diffed by the `taqn` pass), `Folder` membership/search
ops (`CreateFolderMembership`/`DeleteFolderMembership`/`ListFolderMembers`/
`ListFolders`/`ListFoldersForResource`/`SearchFolders`/`UpdateFolder`/
`DescribeFolderPermissions`/`DescribeFolderResolvedPermissions`/
`UpdateFolderPermissions`), `Template`/`Theme` alias and version sub-families
(`CreateTemplateAlias`/`DescribeTemplateAlias`/`ListTemplateAliases`/
`ListTemplateVersions`/`UpdateTemplateAlias`/`UpdateTemplatePermissions`/
`DescribeTemplate`/`ListTemplates`/`DescribeTemplateDefinition`/
`DescribeTemplatePermissions` and the equivalent `Theme` ops),
`OAuthClientApplication`'s `DeleteOAuthClientApplication`/
`DescribeOAuthClientApplication`/`ListOAuthClientApplications`/
`UpdateOAuthClientApplication`, `ActionConnector`'s `CreateActionConnector`/
`DeleteActionConnector`/`ListActionConnectors`/`SearchActionConnectors`,
`VPCConnection`'s `CreateVPCConnection`/`DeleteVPCConnection`,
`RefreshSchedule`'s Topic-refresh sub-ops (`CreateTopicRefreshSchedule`/
`DeleteTopicRefreshSchedule`/`DescribeTopicRefresh`/
`DescribeTopicRefreshSchedule`/`ListTopicRefreshSchedules`/
`UpdateTopicRefreshSchedule`) and DataSet-refresh-properties ops
(`DeleteDataSetRefreshProperties`/`DescribeDataSetRefreshProperties`/
`ListRefreshSchedules`/`DeleteRefreshSchedule`/`PutDataSetRefreshProperties`),
`Agent`/`KnowledgeBase`/`Space`/`Flow` permission ops
(`DescribeAgentPermissions`/`UpdateAgentPermissions`/
`DescribeKnowledgeBasePermissions`/`DescribeSpacePermissions`/
`GetFlowPermissions`/`UpdateFlowPermissions`), and `Embed`'s
`GenerateEmbedUrlForRegisteredUser`/`GenerateEmbedUrlForRegisteredUserWithIdentity`
(already covered narratively by the `taqn` Embed re-diff, not independently
re-verified this pass).
(SUPERSEDED by the 2026-08-23 continuation pass below -- most of the ops
named here have now actually been reached.)

## 2026-08-23: continuation pass, 13 wire-shape bugs found and fixed

Continued the low-client-coverage campaign into the "Not reached" queue named
directly above. All 13 findings are the same two bug classes this campaign
kept finding elsewhere in this file: (A) a List/Search summary handler reusing
a Describe-shape map-builder and leaking fields the real `*Summary` type
doesn't carry, and (B) a mutate op (mostly Delete) whose real output carries a
field (usually `Arn`) this backend already tracks on the record but the
handler discarded instead of returning. Every fix below is proven with a real
`aws-sdk-go-v2`-shaped raw-body assertion (an SDK client's deserializer can't
prove class-A leaks -- it silently drops unrecognized members, same
limitation already documented for `ListAssetBundleExportJobs`/
`ListIAMPolicyAssignments`), hand-reverted against `git show HEAD:<path>`,
confirmed failing, restored via `cp`, `md5sum`-verified byte-identical.

Class A (List/Search summary leaking Describe-only fields):
- `ListFolders`/`SearchFolders`: `folderToMap` (Folder's full shape, includes
  `FolderPath`) was reused for `FolderSummaryList` items. `types.FolderSummary`
  (types.go:10457) has no `FolderPath` -- that's `Folder`-only (types.go:10356),
  populated only by `DescribeFolder`. Added `folderSummaryToMap`, scoped to the
  7 real `FolderSummary` fields. See
  `TestQuickSight_ListFolders_OmitsFolderPath` (handler_folders_test.go).
- `ListTemplateVersions`: `templateVersionToMap` (full `TemplateVersion`,
  includes `SourceEntityArn`/`Definition`) was reused for
  `TemplateVersionSummaryList`. `types.TemplateVersionSummary` (types.go:20950)
  has neither field. Added `templateVersionSummaryToMap`. See
  `TestQuickSight_ListTemplateVersions_OmitsDefinition`
  (handler_templates_test.go).
- `ListThemeVersions`: same bug, `themeVersionToMap`'s `BaseThemeId`/
  `Configuration` leaking into `ThemeVersionSummaryList`.
  `types.ThemeVersionSummary` (types.go:21196) has neither. Added
  `themeVersionSummaryToMap`. See
  `TestQuickSight_ListThemeVersions_OmitsBaseThemeIDAndConfiguration`
  (handler_themes_test.go).
- `ListOAuthClientApplications`: `oauthAppToMap` (full
  `OAuthClientApplication`, includes `OAuthAuthorizationEndpointUrl`/
  `OAuthScopes`/`OAuthTokenEndpointUrl`) was reused for
  `OAuthClientApplications` (the list). `types.OAuthClientApplicationSummary`
  (types.go:14882), confirmed key-by-key against
  `awsRestjson1_deserializeDocumentOAuthClientApplicationSummary`, has none of
  the three. Added `oauthAppSummaryToMap`. See
  `TestQuickSight_ListOAuthClientApps_OmitsDescribeOnlyFields`
  (handler_oauth_test.go).

Class B (mutate op dropping a field this backend already tracks):
- `ListActionConnectors`/`SearchActionConnectors`: `actionConnectorSummaryToMap`
  omitted `CreatedTime` entirely, even though it's a real, non-fabricated field
  on both `ActionConnector` (this backend's struct) and
  `types.ActionConnectorSummary` (types.go:197, optional but genuinely
  tracked). Fixed. See `TestQuickSight_ListActionConnectors_Pagination`'s new
  assertion (handler_actionconnector_test.go).
- `ListBrands`: `types.BrandSummary.Description` (types.go:3220) is real,
  caller-supplied data already sitting in `brand.Definition["Description"]` --
  the same map `BrandName` was already being pulled from two lines above --
  just never read out. Fixed. See `TestQuickSight_ListBrands_SurfacesDescription`
  (handler_brands_test.go).
- `DeleteCustomPermissions`: real `DeleteCustomPermissionsOutput` carries `Arn`
  (api_op_DeleteCustomPermissions.go); this backend already builds it
  deterministically at Create time. Backend signature changed to
  `(*CustomPermissions, error)`; interface + handler updated. See
  `TestQuickSight_CustomPermissionsCRUD`'s new assertion.
- `DeleteVPCConnection`: real `DeleteVPCConnectionOutput` carries
  `Arn`/`AvailabilityStatus`/`DeletionStatus`; this backend already tracks the
  first two and the third is the same `statusDeleted` constant every other
  hard-delete in this file already uses. Backend signature changed to
  `(*VPCConnection, error)`. See `TestQuickSight_VPCConnectionCRUD`'s Delete
  assertions.
- `CreateTopicRefreshSchedule`/`DescribeTopicRefreshSchedule`/
  `UpdateTopicRefreshSchedule`/`DeleteTopicRefreshSchedule`/
  `ListTopicRefreshSchedules`: all 5 real outputs carry a top-level `TopicArn`
  (confirmed against each op's own `api_op_*.go`); none were returning it,
  even though `h.Backend.DescribeTopic` was one call away. Added a shared
  `topicArn` helper. `DeleteTopicRefreshScheduleOutput` additionally carries
  `DatasetArn`, also dropped -- backend signature changed to
  `(*TopicRefreshSchedule, error)` so the handler has it. See
  `TestQuickSight_TopicRefreshScheduleCRUD`'s new assertions
  (handler_topics_test.go).
- `DeleteRefreshSchedule` (DataSet-level, V1): real
  `DeleteRefreshScheduleOutput` carries `Arn`/`ScheduleId`
  (api_op_DeleteRefreshSchedule.go); this backend fetched the schedule to
  validate it existed and then discarded it. Backend signature changed to
  `(*RefreshSchedule, error)`. See
  `TestQuickSight_DataSetRefreshScheduleCRUD`'s new assertions.
- `UpdateIpRestriction`: real `UpdateIpRestrictionOutput` carries
  `AwsAccountId` (api_op_UpdateIpRestriction.go); `accountID` was already a
  local variable in the handler. See
  `TestQuickSight_UpdateIPRestriction_ReturnsAwsAccountId` (handler_account_test.go).
- `UpdateQPersonalizationConfiguration`/`UpdateQuickSightQSearchConfiguration`:
  both real outputs echo back the mode/status that was just set
  (`PersonalizationMode`/`QSearchStatus`); both handlers already received that
  exact value from the backend call and discarded it with `_, err :=`. See
  `TestQuickSight_UpdateQPersonalizationAndQSearchConfig_EchoValue`.

Ops re-audited clean this pass, no findings: `CreateVPCConnection` (already
correct), `CustomPermissions` CRUD proper aside from the Delete gap above
(`CreateCustomPermissions`/`DescribeCustomPermissions`/
`UpdateCustomPermissions`/`ListCustomPermissions` -- `Governance` remains the
pre-existing, already-documented gap, not re-filed), `DescribeTopicRefresh`,
`PutDataSetRefreshProperties`/`DescribeDataSetRefreshProperties`/
`DeleteDataSetRefreshProperties` (all three real outputs carry no data fields
beyond `RequestId`/`Status`), `DescribeIpRestriction`,
`UpdateKeyRegistration`, `DescribeDefaultQBusinessApplication`/
`UpdateDefaultQBusinessApplication`/`DeleteDefaultQBusinessApplication`,
`DeleteAccountSubscription`, `UpdateSPICECapacity`,
`UpdatePublicSharingSettings`, all 6 `Agent`/`KnowledgeBase`/`Space`/`Flow`
permission ops (`DescribeAgentPermissions`/`UpdateAgentPermissions`/
`DescribeKnowledgeBasePermissions`/`UpdateKnowledgeBasePermissions`/
`DescribeSpacePermissions`/`UpdateSpacePermissions`/`GetFlowPermissions`/
`UpdateFlowPermissions` -- wire shapes, including Space's camelCase quirk,
all confirmed correct), and both `Embed` registered-user ops
(`GenerateEmbedUrlForRegisteredUser`/
`GenerateEmbedUrlForRegisteredUserWithIdentity`, confirmed against their real
`EmbedUrl`/`RequestId`/`Status`-only output shapes, and the documented
user-existence-validation behavior re-confirmed in code).

Corrected one stale claim: the `ActionConnector` family note above says
`AuthenticationConfig` redaction is "NOT fixed... flagged for follow-up" --
that is no longer true. `actionconnector_auth.go`/`redactAuthenticationConfig`
(added 2026-08-11, PR #2414, predating that note) implements the real
`ReadAuthConfig` projection in full (redacts `ApiKey`/`Password`/
`ClientSecret` per `AuthenticationType`, adds `SourceArn` for IAM), is wired
into `actionConnectorToMap`, and is tested end-to-end in
`TestQuickSight_ActionConnector_AuthConfigRedaction`
(actionconnector_redaction_test.go). Some earlier pass fixed this without
updating the note -- same failure mode as the stale `SearchTopics`/
`DeleteTopic` notes `gopherstack-0qzf` already caught once in this file.

One modelling gap found, NOT fixed (fabrication risk, not a bounded fix):
`Create`/`Describe`/`UpdateAccountCustomizationOutput` all carry a real `Arn`
field (api_op_{Create,Describe,Update}AccountCustomization.go); this backend's
`AccountCustomization` struct has no `Arn` slot and no confirmed SDK evidence
for the exact ARN resource-type path segment QuickSight uses for a
namespace-scoped account customization (unlike every other resource type in
this backend, whose ARN suffix -- e.g. `custom-permissions/{name}`,
`vpc-connection/{id}` -- was confirmed once, historically, against real AWS
console/CLI output or SDK doc comments). Minting one now would be exactly the
unverified-wire-shape risk parity-principles.md rule 1 warns against.
Left absent; flagged here for a follow-up pass with SDK/console evidence for
the correct format, not guessed at.

Still not reached (named, not a claim of a problem): full `Brand` CRUD proper
(`CreateBrand`/`DeleteBrand`/`DeleteBrandAssignment`; `DescribeBrand` was
already field-diffed by the `taqn` pass), `Folder` membership/search ops
(`CreateFolderMembership`/`DeleteFolderMembership`/`ListFolderMembers`/
`ListFoldersForResource`/`UpdateFolder`/`DescribeFolderPermissions`/
`DescribeFolderResolvedPermissions`/`UpdateFolderPermissions`), `Template`/
`Theme` alias sub-families in full (`CreateTemplateAlias`/
`DescribeTemplateAlias`/`ListTemplateAliases`/`UpdateTemplateAlias`/
`UpdateTemplatePermissions`/`DescribeTemplate`/`ListTemplates`/
`DescribeTemplateDefinition`/`DescribeTemplatePermissions` and the equivalent
`Theme` ops -- `ListTemplateVersions`/`ListThemeVersions` themselves are now
fixed and covered above), `OAuthClientApplication`'s
`DeleteOAuthClientApplication`/`DescribeOAuthClientApplication`/
`UpdateOAuthClientApplication` (`ListOAuthClientApplications` now fixed and
covered above), `ActionConnector`'s `CreateActionConnector`/
`DeleteActionConnector` (`List`/`SearchActionConnectors` now fixed and covered
above), and `AccountLevel`'s remaining un-independently-diffed sub-resources:
`AccountCustomization` CRUD's `AccountCustomization` struct field-shape itself
(only the missing top-level `Arn` was checked, see modelling gap above) and
`AccountCustomPermission` (`DescribeAccountCustomPermission`/
`UpdateAccountCustomPermission`/`DeleteAccountCustomPermission` -- all three
carry no data fields beyond `RequestId`/`Status` per their SDK output structs,
so spot-checked clean by inspection but not independently exercised with a
test this pass).

## 2026-08-23: cross-service sweep for the class-A leak, two more instances

The `2026-08-23: continuation pass` above fixed 4 class-A instances
(`Folder`/`TemplateVersion`/`ThemeVersion`/`OAuthClientApplication`) but did
not audit every shared map-builder in the service; a follow-up sweep of every
function called from both a `Describe`/`Get` handler and a `List`/`Search`
handler in this package, cross-checked field-by-field against each
`*Summary` type's own definition in `quicksight@v1.123.1/types/types.go`,
found two more:

- `ListAnalyses`/`SearchAnalyses`: `analysisToMap` set `ThemeArn` whenever the
  analysis had one, and was reused for both `Describe` and `List`/`Search`.
  `types.AnalysisSummary` has no `ThemeArn` (only `AnalysisId`/`Arn`/
  `CreatedTime`/`LastUpdatedTime`/`Name`/`Status`); `ThemeArn` is
  `Analysis`-only (types.go, `type Analysis struct`), populated by
  `DescribeAnalysis`. Added `analysisSummaryToMap`, scoped to the 6 real
  `AnalysisSummary` fields; `analysisToMap` now wraps it and adds `ThemeArn`
  on top for `DescribeAnalysis`. See `TestQuickSight_AnalysisSummaryOmitsThemeArn`
  (handler_analysis_test.go), which also asserts `DescribeAnalysis` still
  returns `ThemeArn`.
- `ListDataSources`/`SearchDataSources`: `dataSourceToMap` always set
  `Status`, reused for both `Describe` and `List`/`Search`. `types.
  DataSourceSummary` has no `Status` (only `Arn`/`CreatedTime`/
  `DataSourceId`/`LastUpdatedTime`/`Name`/`Type`); `Status` is
  `DataSource`-only. Added `dataSourceSummaryToMap`; `dataSourceToMap` now
  wraps it and adds `Status` on top for `DescribeDataSource`. See
  `TestQuickSight_DataSourceSummaryOmitsStatus` (handler_datasource_test.go),
  which also asserts `DescribeDataSource` still returns `Status`.

Both fixes proven with a raw-body assertion (`doRequest`/`parseBody`, the
convention this package's own tests already use): the real SDK client can't
see either leak, since `AnalysisSummary`/`DataSourceSummary` have no field to
decode a leaked key into. Both hand-reverted, confirmed the new test fails
pre-fix, restored via `cp`, `md5sum`-verified byte-identical.

Every other function in this package shared between a `Describe`/`Get`
handler and a `List`/`Search` handler was checked the same way and found
correct: `vpcConnectionToMap` (types.VPCConnection/VPCConnectionSummary field
sets are identical -- no leak), `dashboardToMap` (already scoped to exactly
`DashboardSummary`'s fields, no `LinkEntities`/`Version` leak),
`customPermissionsToMap`/`templateAliasToMap`/`themeAliasToMap`/
`groupToMap`/`userToMap`/`dataSetRefreshScheduleToMap` (no separate SDK
`*Summary` type exists for any of these, so sharing one shape for `Describe`
and `List` is correct), and `topicRefreshScheduleToMap`/`topicToMap`/
`importJobToMap`/`flowSummaryToMap` (the reverse case: only a `*Summary`
type exists in the SDK, so `Describe` itself returns the summary shape --
no full type to leak from).

This sweep also ran across every other gopherstack service with the same
shared-builder shape (~90 candidate functions across ~45 services, filtered
from 361 raw List/Search+Describe/Get co-callers down to functions building
a map/struct wire shape). No further confirmed class-A instances were found
outside quicksight; see the session report for the full signal derivation
and per-service results. One related-but-different anomaly was found in
`services/iotanalytics` (a `pipelineReprocessingSummary` carrying
`StartTime`/`EndTime` fields absent from `types.ReprocessingSummary` in
*both* `DescribePipeline` and `ListPipelines`, not just the List side) --
flagged there, not fixed here, since it is a different bug shape than the
one this pass targets.

## 2026-08-30 sort-totality sweep (wrapper-key-sweep-rds-cloudwatch-sqs-sns branch)

Audited every `sort.Slice` call for whether its comparator is a *total*
order, following on from the 2026-08-29 pagination-arithmetic sweep (which
checked cursor arithmetic and confirmed every prior "40 hand-rolled
paginators" bug, but never asked whether the sorts that do exist are total).
Every collection here is a `store.Table[V]`, so `.All()` is unordered map
iteration; a comparator with no secondary key can reorder tied records
across two calls in the same paginated walk.

Almost every sort in this service is over an ID/ARN field
(`ActionConnectorID`, `AnalysisID`, `AgentID`, `BrandID`, `DataSourceID`,
`DataSetID`, `IngestionID`, `JobID`, `DashboardID`, `FolderID`, `FlowID`,
`UpgradeRequestID`, `KnowledgeBaseID`, `SpaceID`, `ThemeID`, `TopicID`,
`ClientID`, `TemplateID`, `VPCConnectionID`) or a `Name`-shaped field that
IS the table's own primary key within its scope
(`namespaces`→`Name`, `groups`→`Namespace+GroupName` with `ListGroups`
itself namespace-scoped, `users`→`Namespace+UserName` with `ListUsers`
namespace-scoped, `customPermissions`→`Name`,
`identityPropagationConfigs`→`Service`, `iamPolicyAssignments`→`Namespace+AssignmentName`
with `ListIAMPolicyAssignments` namespace-scoped) — all confirmed against
`store_setup.go`'s `keyFn` closures, all total by construction, nothing to
fix. `folders.go`'s `ListFolderMembers` sorts on `(MemberType, MemberID)`,
exactly the pair `folderMemberKey` uses beyond `FolderID` — also total.

**Fixed (non-total sort found) — `ListUsersIndexCapacity`:** sorted on
`UserName` alone, but `storedUser`'s key is `accountID/namespace/UserName`
— UserName is only unique *within one namespace*. This op's own handler
passes `namespace` straight from an optional request-body field, so
`namespace == ""` is a real, reachable call shape (not a hypothetical) that
scans every namespace at once; two different namespaces can each register a
user named the same thing. Worse than an ordering flip: `paginateUserIndexCapacity`'s
cursor is an *equality match* against `UserName`
(`if u.UserName == nextToken`), so a tied `UserName` made every subsequent
page's cursor resolve back to the *first* matching user and repeat — not
just reordered results, a stuck cursor that never reaches the second tied
user. Fixed by sorting on `(UserName, UserArn)` and switching the cursor
itself to match on `UserArn` (globally unique, since it embeds the
namespace) instead of `UserName`.
`TestListUsersIndexCapacityCrossNamespaceSortIsTotal`
(pagination_sort_totality_test.go) constructs the two-namespace tie and
reproduces both symptoms (a record repeated across pages, and — guarded by
an explicit page-count cap so the test fails cleanly rather than hanging —
the walk never terminating) against unfixed code.

Gates: `go build`, `go vet`, `go test -race -count=1`, `golangci-lint run` —
all clean (`./services/quicksight/...`).

**2026-08-30 (negative-continuation-token sweep)**: `store.go`'s `decodePageToken` accepted a
token that base64-decoded to a negative integer and returned it verbatim; 6 of its 7 callers
(`brands.go`, `folders.go`'s `paginateFolders`, `templates.go`, `vpcconnections.go`,
`namespace.go`'s `paginateNamespaces`, `themes.go`) only clamp the upper bound (`if start >
len(all) { start = len(all) }`), which does not catch a negative `start`, so
`all[start:end]` panicked given `LTU=` (base64 for `-5`) as `next-token`. (The 7th caller,
`dashboard.go`'s `ListDashboardVersions`, doesn't slice — it synthesizes version numbers from
a counter range, so a negative `start` produced nonsensical negative `VersionNumber` entries
rather than a panic; also fixed by the same decode-site change.) Fixed at the decode site, so
all 7 callers inherit the fix. The existing `TestPaginationTokensAreOpaque` table in
`pagination_test.go` covers all 7 of these operations but never supplied a hostile token.

Proof: `TestPagination_NegativeOffsetToken` (`pagination_test.go`, table-driven over all 6
slicing operations) confirmed panicking pre-fix for every subtest, passes now. Gates: `go
build ./services/quicksight/...`, `go vet ./services/quicksight/...`, `go test -race -count=1
./services/quicksight/...`, `golangci-lint run ./services/quicksight/...` (0 issues — one
`err113` finding from the fix's first draft, a dynamic `fmt.Errorf`, was replaced with the
existing `ErrValidation` sentinel). Work left uncommitted per this pass's instructions.

## 2026-08-30 gopherstack-wlo1: error-envelope sweep, confirmed clean

QuickSight is restjson1 (`aws-sdk-go-v2/service/quicksight@v1.123.1`:
`awsRestjson1_` prefix). Read all 277 `deserializeOpError` functions in
`deserializers.go` (277-of-277, not sampled): all identically call
`restjson.GetErrorInfo(decoder)` after checking `X-Amzn-ErrorType`, and
`GetErrorInfo` checks body key `Code` (untagged Go field, exact-matches
JSON key `"Code"`) *before* falling back to `__type`. `handler_paths.go`'s
`writeError` writes `{"Code": errCode, "Message": msg}` with no header --
this satisfies the client's body fallback directly via the `Code` key,
which is actually checked ahead of `__type` in the real SDK, so this
service's different-looking envelope (`Code`, not `__type`) is just as
correct as the `__type`-shaped ones used elsewhere in this campaign.
Grepped every direct `http.Status{Bad,NotFound,Conflict,...}` use across
all `handler_*.go` files (33 files hit): every one resolves to a call
through `writeError`/`httpErr`, no bypass found. Spot-checked the
AlreadyExists family (folders/templates/themes/topics use
`ResourceExistsException`, not the generic `httpErr` ConflictException
default) -- these are handled explicitly at each call site
(`errors.Is(err, ErrXAlreadyExists)` before falling through to `httpErr`),
so the more specific code is preserved; not a bug, just worth recording
since `httpErr`'s own `ErrAlreadyExists` branch always emits
`ConflictException`.

No bug found. Added
`TestErrorEnvelope_DescribeDataSetNotFoundDecodesToTypedError`
(`error_envelope_test.go`), driving a real `quicksightsdk.Client` through
`DescribeDataSet` for a nonexistent dataset: asserts `errors.As` unwraps to
the concrete `*types.ResourceNotFoundException`, and separately asserts on
the raw response bytes for the same case (raw HTTP request needs an
`Authorization` header naming the SigV4 credential scope `quicksight`,
since `RouteMatcher`'s `isQuickSightRequest` reads it directly off the
header). Passed against unmodified code, confirming this service's error
envelope was already wire-correct.

Gates (this pass, `services/quicksight/` only): `go build`, `go vet`,
`go test -race -count=1`, `golangci-lint run` -- all clean.

## 2026-08-31 gopherstack-uox6: Search filter value-semantics sweep

Audited all 13 `Search*` operations' filter surface (`SearchActionConnectors`,
`SearchAgents`, `SearchAnalyses`, `SearchDashboards`, `SearchDataSets`,
`SearchDataSources`, `SearchFlows`, `SearchFolders`, `SearchGroups`,
`SearchKnowledgeBases`, `SearchSpaces`, `SearchTopics`, `SearchTopicsV2`)
member-by-member against each operation's own filter type and enum in
`quicksight@v1.123.1 types/types.go`/`types/enums.go` -- not a sibling's, per
this class's standing lesson. Six real bugs, all UNDER-matching (a documented,
backed filter silently passed through as "everything matches").

**Two filters entirely unread despite tracked backing data:**
`SearchActionConnectors` checked only `ACTION_CONNECTOR_NAME`
(`actionconnector.go`); `ACTION_CONNECTOR_TYPE` -- a plain field
(`storedActionConnector.Type`), not an untracked ownership ARN -- fell
through the ownership-pass-through default and matched every connector
regardless of type. `SearchFlows` checked only `assetName` (`flow.go`);
`assetDescription` (`types.FieldName`) fell through the same way despite
`storedFlow.Description` being tracked.

**Four more on `SearchKnowledgeBases`** (`knowledgebases.go`):
`KNOWLEDGE_BASE_ID`, `DATASOURCE_ARN`, and `PRIMARY_OWNER` were unread despite
being plain tracked fields; `KNOWLEDGE_BASE_SIZE_BYTES` was unread and its
operator (`GREATER_THAN_OR_EQUALS`/`LESS_THAN_OR_EQUALS`, values
`KnowledgeBaseSearchOperator` adds beyond `STRING_EQUALS`/`STRING_LIKE`) was
never parsed at all.

**Two operator-string mismatches, same root cause, both silent.**
`KnowledgeBaseSearchOperator` and `SpaceSearchOperator` emit uppercase-
underscore wire values (`"STRING_LIKE"`) -- unlike `FilterOperator`/
`ComparisonOperator`/`SearchFilterOperator`/`TopicFilterOperator`, which all
nine other Search ops use and which emit PascalCase (`"StringLike"`). The
shared `matchesNameFilter` (`folders.go`) compared against the PascalCase
constant unconditionally, so a real `STRING_LIKE` request for either op
silently fell back to exact-equality comparison -- affecting the one filter
each op DID implement (`KNOWLEDGE_BASE_NAME`, `SPACE_NAME`), on top of the
unread-field bugs above. Fixed by parameterizing the shared comparison
(`matchesStringOp(actual, op, value, likeOp string)`) so each caller supplies
its own operation's wire spelling.

**A separate, more fundamental bug underneath both of those: wrong wire-key
casing, dropping every filter unconditionally.** `KnowledgeBaseSearchFilter`
and `SpaceQuicksightSearchFilter` are the only two Search filter types in
this service whose serializer emits lowercase `"name"`/`"operator"`/`"value"`
(confirmed in `serializers.go`'s
`awsRestjson1_serializeDocumentKnowledgeBaseSearchFilter`/
`...SpaceQuicksightSearchFilter`) -- the other nine emit PascalCase
`"Name"`/`"Operator"`/`"Value"`. `handleSearchKnowledgeBases`/
`handleSearchSpaces` both called the shared `folderFiltersFromBody`, which
reads the PascalCase keys, so every filter field -- Name, Operator, and Value
alike -- parsed to `""` for both operations. An empty Name matches no
handled case and falls through the ownership-pass-through default, so EVERY
filter on EVERY `Search{KnowledgeBases,Spaces}` call, including the
previously-"working" name filter, has always silently matched everything.
This is the compound-axis shape from earlier in this class (a wrong key plus
an empty-case default), except here the wrong key is a casing mismatch
rather than singular-vs-plural, and it hid the operator-string bug above
completely: the operator string was never even reached, because Operator
decoded to `""` before it could be compared. Fixed with a dedicated
`lowercaseFiltersFromBody` (`handler_folders.go`) used only by these two
handlers.

**Verified, not a bug: `SearchSpaces`' `CONTRIBUTED_BY`/`CONSUMED_SOURCE_SIZE`
correctly pass through** -- `storedSpace` tracks neither a resource
contributor nor a consumed-size figure, so there's no backing data to filter
on. **`CREATED_BY` also correctly passes through**, but for a different
reason: real `CreateSpaceInput` has no request field for it (confirmed
against `api_op_CreateSpace.go`) -- it's principal-derived, same as the
`DIRECT_QUICKSIGHT_OWNER` family every other Search op already leaves
untracked, not a field this backend chose not to read.

**Confirmed correct, not fixed:** the AND-across-filters combining rule
(`matchesAllNameFilters`/the new per-operation matcher loops) has no
documented override anywhere in this filter family and was left as-is.
`SearchGroups` (`GROUP_NAME`/`StartsWith` only), `SearchAgents`,
`SearchAnalyses`, `SearchDashboards`, `SearchDataSets`, `SearchDataSources`,
`SearchTopics`/`SearchTopicsV2` were checked member-by-member against their
own filter-name enums and are clean: each has exactly one non-ownership
filter name and it's the one implemented. `SearchFolders` already handled
both of its non-ownership names (`PARENT_FOLDER_ARN`, `FOLDER_NAME`).

Two pre-existing tests (`handler_flow_test.go`'s "search filters by
KNOWLEDGE_BASE_NAME"/"search filters by SPACE_NAME") asserted the bug
without knowing it: both built their filter directly as a Go map with
PascalCase keys and the old `"StringLike"` operator spelling, bypassing the
real SDK's serializer entirely, so neither the wire-key-casing bug nor the
operator-string bug was reachable from them. Corrected to the wire values a
real client actually sends (lowercase keys, `"STRING_LIKE"`); same two
assertions each, unchanged.

New tests (`search_filter_semantics_test.go`), driven through the real
`aws-sdk-go-v2` client, each seeding 2+ records that differ only on the
filtered attribute and asserting both inclusion and exclusion:
`TestSearchActionConnectors_TypeFilter`,
`TestSearchActionConnectors_MultipleFiltersAND` (proves AND, not OR, across
two filters naming different connectors), `TestSearchFlows_DescriptionFilter`,
`TestSearchSpaces_IDFilter`, and `TestSearchKnowledgeBases_FilterSemantics`
(five subtests: id, datasource-arn, primary-owner, size-bytes GTE/LTE, and
the STRING_LIKE-substring regression). Every new test confirmed failing
against unmodified code before the corresponding fix landed, verified in
three separate revert/rebuild/restore passes (action connector type filter
alone; the KB/Space wire-key-casing fix alone, which broke all KB/Space
Search tests including the pre-existing name-filter ones; the KB/Space
per-field matcher fix alone) with byte-identical restores confirmed by diff
after each. `KNOWLEDGE_BASE_SIZE_BYTES` has no request-settable path to a
non-zero value in this backend (`CreateKnowledgeBase` has no
`KnowledgeBaseSizeBytes` parameter -- it's computed from ingestion, which
this backend doesn't model), so that subtest distinguishes GTE/LTE by
varying the filter's target value against a fixed size-0 record rather than
varying the record.

Coverage is a full slice of the Search family, not the whole service: the
121 non-Search List/Describe operations (default handling, page-size
defaults, other filter/parameter semantics) are unaudited by this pass.

Gates (this pass, `services/quicksight/` only): `go build`, `go vet`,
`go test -race -count=1`, `golangci-lint run` -- all clean. Repo-wide
`go vet ./...` could not complete (build cache disk at 100%, an environment
issue unrelated to this diff -- `dashboard` package failed with "no space
left on device"); the only out-of-scope importer of this package
(`cli.go`, root package) was vetted directly instead (`go vet .` -- clean),
and no exported signature changed in this diff, so no caller outside this
package could be affected regardless.

## 2026-08-31 gopherstack-uox6: List/Describe value-semantics sweep, 4 bugs

Continuation of the pass above: the 13-operation Search family is done; this
pass covers a slice of the 109 `List*`/`Describe*` operations (constant
names in `handler*.go`/`interfaces.go`: 65 `opDescribe*`, 44 `opList*`;
`bd`'s prior estimate of 121 was high, likely double-counting a few names
under different labels).

**Page-size axis, checked exhaustively across all 39 `List*` operations
carrying `MaxResults`:** none document a numeric default in
`quicksight@v1.123.1`'s doc comments -- only `ListActionConnectors`
documents a bound ("Valid range is 1 to 100") with no default. This matches
the Search family's own finding, so the uniform `defaultMaxResults = 100`
clamp (`store.go`, 40 call sites) contradicts nothing documented anywhere in
this service. Targeting swept every `List*`/`Describe*` doc comment for
"if you omit"/"if not specified"/"by default"/"default" language
(not just `MaxResults`) to find the real filter/enum/bool surface, since
most `List*` operations here have no filter fields at all beyond
`AwsAccountId`/`MaxResults`/`NextToken` -- confirmed by listing every
request-struct field across all 39 and finding exactly four with a
typed filter/enum/bool field beyond the common three
(`ListIAMPolicyAssignments.AssignmentStatus`, `ListRoleMemberships.Role`,
`ListThemes.Type`, `ListUsersIndexCapacity.{Filters,SortBy,SortOrder}`) plus
six on `Describe*` operations found the same way
(`DescribeAccountCustomization.Resolved`,
`DescribeAutomationJob.{IncludeInputPayload,IncludeOutputPayload}`,
`DescribeBrand.VersionId`, `DescribeFlow.PublishState`,
`DescribeKeyRegistration.DefaultKeyOnly`,
`DescribeRoleCustomPermission.Role`).

**Bug 1: `ListThemes.Type` never read at all.** `handleListThemes`
(`handler_themes.go`) called `Backend.ListThemes(accountID, maxResults,
nextToken)` -- no `type` query parameter (confirmed against
`awsRestjson1_serializeOpHttpBindingsListThemesInput`, which
`encoder.SetQuery("type")`s it) reached the backend at all. `ALL (default) -
Display all existing themes... CUSTOM... QUICKSIGHT` per
`api_op_ListThemes.go`. This backend's `CreateTheme` always stores
`Type: "CUSTOM"` (no seeded QUICKSIGHT starting theme), so `Type=QUICKSIGHT`
is exactly the value no stored theme can legally carry -- before the fix it
returned every CUSTOM theme anyway; after, it correctly returns none. Fixed
by filtering `allThemesLocked`'s result in `Backend.ListThemes`
(`themes.go`) on an added `themeType` parameter, empty/`"ALL"` meaning no
filter.

**Bug 2: `DescribeKeyRegistration.DefaultKeyOnly` never read.**
`handleDescribeKeyRegistration` (`handler_account.go`) never read the
"default-key-only" boolean query parameter (confirmed in serializers.go),
so a client asking for only the default key got every registered key back
regardless -- even though `RegisteredCustomerManagedKey.DefaultKey` is real,
request-settable data via `UpdateKeyRegistration`. Fixed by threading a
`defaultKeyOnly bool` through `Backend.DescribeKeyRegistration`
(`account.go`) and filtering on `DefaultKey`.

**Bug 3: `ListUsersIndexCapacity`'s `Filters`/`SortBy`/`SortOrder` never
applied.** `handleListUsersIndexCapacity` (`handler_userindexcapacity.go`)
read only `namespace`/`maxResults`/`nextToken` from the body. A comment on
`Backend.ListUsersIndexCapacity` (`userindexcapacity.go`) explicitly
justified this as "matching this backend's existing precedent of no-op
unrecognized search-filter attributes" -- but `Filters`
(`totalCapacityBytes` range, `userNameOrEmail` prefix) and `SortBy`/
`SortOrder` are documented, backed fields, not unrecognized ones: the
precedent this cited doesn't apply. `TotalCapacityBytes`/`Email`/`UserName`
are real fields this backend already computes/tracks, so both filters have
real data to act on. Fixed by adding a `UserIndexCapacityQuery` struct
(`types.go`), parsing it from the body (`handler_userindexcapacity.go`), and
applying it before pagination (`userindexcapacity.go`): the capacity range
is inclusive on both bounds per `CapacityBytesRangeFilter`'s doc comment,
the prefix matches username OR email per `UserNameOrEmailFilter`'s "starts-
with match against username or email". `SortBy`
(`UserIndexCapacitySortBy` has exactly one legal member,
`TOTAL_CAPACITY_BYTES`) now switches the sort key from `UserName` to
`TotalCapacityBytes`, honoring `SortOrder`'s documented "Defaults to DESC if
not specified" -- this half has **no observable effect today** and is
reported as such: this backend has no ingestion pipeline, so every user's
`TotalCapacityBytes` is provably 0 (same reasoning `userIndexCapacityFor`'s
existing comment already gives for `TotalSpaceCapacityBytes`), and the code
change was verified by inspection rather than a test that could actually
distinguish ASC from DESC. Not implemented: `Namespace` "Required when the
userNameOrEmail filter is present" -- a missing-rejection/validation
concern, kept on that separate axis rather than folded into this fix.

**Bug 4: `DescribeAccountCustomization.Resolved` never read.**
"The Resolved flag works with the other parameters to determine which view
of Quick Sight customizations is returned... Omit this flag... to reveal
customizations that are configured at different levels"
(`api_op_DescribeAccountCustomization.go`). `handleDescribeAccountCustomization`
(`handler_account.go`) only ever did an exact `accountID/namespace` key
lookup, so a namespace-scoped `Resolved=true` request for a namespace with
no customization of its own -- only an account-level default -- got
`ErrAccountCustomizationNotFound` (404) where real AWS resolves to the
account-level view. Fixed by adding a `resolved bool` parameter to
`Backend.DescribeAccountCustomization` (`account.go`): when set and
`namespace != ""`, it merges field-by-field, namespace value winning where
non-empty, else falling back to the account-level value; unresolved lookups
and the account level itself (`namespace == ""`, nothing to fall back to)
are unchanged.

**Confirmed correct, not fixed, verified against each operation's own
input type and serializer, not a sibling's:**
`ListIAMPolicyAssignments.AssignmentStatus` (`handler_iampolicyassignments.go`)
already reads the `assignment-status` query parameter and filters
correctly (`""` matches everything, matching `ListThemes`' documented `ALL`
default -- no equivalent default is documented here, but empty already
means "no filter" either way). `ListRoleMemberships.Role` and
`DescribeRoleCustomPermission.Role` are required path-segment selectors
(which role's memberships/permissions to fetch), not filters -- both
correctly read from the URL path. `DescribeAutomationJob`'s
`IncludeInputPayload`/`IncludeOutputPayload` correctly read their query
parameters and default to excluded (matching the documented "If set to
false, ... returned as null", and Go's zero-value `bool` already means
`false`). `DescribeBrand.VersionId`'s documented "default value is the
latest version" is correctly honored: `toBrand()` reads `CurrentVersionID`,
which `UpdateBrand` bumps on every new version.

**Recorded on the other axis, not fixed (structural, not a semantics
bug):** `DescribeFlow.PublishState` is required and bound to the
"publish-state" query parameter, but this backend stores one definition per
flow with no draft/published divergence (`CreateFlow`'s existing comment:
real AWS auto-publishes on create, matching this backend's single-state
model) -- there is nothing for the parameter to select between, so it isn't
read at all. The doc comment previously claimed it was "accepted... for
wire fidelity", which was false (never read); corrected to state plainly
that it isn't read and why that's structurally correct here, not an
oversight.

Coverage is a slice, stated plainly: of 109 `List*`/`Describe*` operations,
this pass verified the page-size axis exhaustively (all 39 `MaxResults`
operations) and the filter/enum/bool axis on the 10 operations found to
carry one (4 bugs, 6 confirmed clean/structural). The remaining ~99
operations -- almost all pure `AwsAccountId`/`MaxResults`/`NextToken`/
resource-ID listings or single-resource describes with no filter surface,
per the same sweep that found the ten above -- are unaudited by this pass.

Tests: `list_describe_value_semantics_test.go` (new), driven through the
real `aws-sdk-go-v2` client: `TestListThemes_TypeFilter`,
`TestDescribeKeyRegistration_DefaultKeyOnly`,
`TestListUsersIndexCapacity_PrefixFilter`,
`TestListUsersIndexCapacity_CapacityBytesFilter` (boundary-tests MinBytes=0
vs MinBytes=1 against a provably-always-0 capacity, the same technique the
prior pass used for `KNOWLEDGE_BASE_SIZE_BYTES`), and
`TestDescribeAccountCustomization_Resolved`. All five confirmed failing
against unmodified code before the corresponding fix landed. No existing
test was modified; two existing tests' call sites
(`pagination_sort_totality_test.go`, `store_roundtrip_test.go`) were updated
for the two interface-signature changes those tests call directly
(`ListUsersIndexCapacity`, `DescribeAccountCustomization`) with no assertion
changes.

No pages fetched this pass -- everything resolved from the pinned
`quicksight@v1.123.1` module cache.

Gates (this pass, `services/quicksight/` only): `go build`, `go vet`,
`go test -race -count=1` all clean. `golangci-lint run` found six issues
introduced by this diff (gocognit on the filter-body parser, two golines
line-length violations, four govet shadowed-`ok` warnings, two nestif
nested-block warnings) -- all fixed by decomposing the parser into small
named-return helpers and extracting the sort comparator. A first re-run
still showed a `dupl` pair -- `themes.go`'s `UpdateTheme`/`DeleteTheme` vs
`templates.go`'s `UpdateTemplate`/`DeleteTemplate` -- initially misreported
here as pre-existing (`themes.go` IS modified by this diff; only
`templates.go` was untouched, and the two files' bodies not changing
doesn't mean the *pairing* dupl reports wasn't a consequence of lines
shifting elsewhere). A clean-HEAD worktree check (`git worktree add
--detach`, never a bare `git stash`) confirmed the real mechanism: at HEAD,
dupl's clustering already merges `UpdateTheme`+`DeleteTheme`+
`allThemesLocked`+...+`ListThemeAliases` into one larger match against the
equivalent `templates.go` span (the existing `//nolint:dupl` on
`ListThemeAliases`/`ListTemplateAliases` covers that merged report's
attributed line, which is why it read as "clean" before). This diff's added
lines inside `ListThemes` sit between the Update/Delete pair and the
ListAliases pair, splitting that one merged match into two separate ones --
the Update/Delete pair losing its coverage as a result. Fixed per this
repo's own established convention for this exact shape (12+ existing
`//nolint:dupl // list functions share structure but operate on different
stored types` directives across this file family for same-CRUD-shape/
different-stored-type pairs): added matching directives on `UpdateTemplate`
(`templates.go:145`) and `UpdateTheme` (`themes.go:152`), reworded for
"update/delete" rather than "list". Sharing via generics was considered and
rejected: it would require a getter/setter interface spanning
`storedTemplate`/`storedTheme` (and their version types) for a lint-only
concern, a pattern this file family has consistently not adopted anywhere
else. `golangci-lint run ./services/quicksight/...` now reports `0 issues.`
-- confirmed no other `nolint` directive anywhere in the package is flagged
unused (nolintlint fires on the whole package, not just `dupl`, so a clean
run is the authoritative check). Repo-wide `go vet ./...` ran clean (disk
at 19% this pass, no cache issue) both before and after this correction.
All four interface-signature changes (`ListThemes`,
`DescribeKeyRegistration`, `DescribeAccountCustomization`,
`ListUsersIndexCapacity`) have no callers outside this package (confirmed
by grep); the two in-package test call sites they broke were updated, not
weakened -- no test assertions changed by either this pass or this
correction.

## Handler-collision determinism re-audit (2026-08-31, gopherstack-id70)

Re-checked for damage from the handler-resolution defect fixed in
`ef0eef041`. Built the unpatched `cmd/reqfieldscan`/`cmd/reqfielddiff` from
`ef0eef041~1` in a worktree, ran both five times against this package, and
diffed against HEAD.

`cmd/reqfieldscan`: byte-identical across all 5 old runs and HEAD.
`cmd/reqfielddiff`: 794 findings in every one of the 5 old runs and at
HEAD, op.field key sets identical. ZERO DAMAGE.

## User.CustomPermissionsName write-only fix (2026-09-06, gopherstack-rt14)

Filed title-only ("UpdateUserCustomPermission writes to userCustomPermissions
which DescribeUser and ListUsers never read"), no description. Re-derived
and confirmed real: `UpdateUserCustomPermission` (`custompermissions.go:315`)
writes `b.userCustomPermissions[userCustomPermissionKey(...)]`, but
`DescribeUser` (`user.go:63`) and `ListUsers` (`user.go:147`) built their
response from `storedUser.toUser()` alone, which never consulted that map --
a client could set a user's custom permissions profile and never observe it
back.

Deciding SDK quote: `types/types.go:23202` (aws-sdk-go-v2/service/quicksight
v1.123.1) -- `types.User` carries `CustomPermissionsName *string // The
custom permissions profile associated with this user.` Both
`DescribeUserOutput.User` and `ListUsersOutput.UserList` (`api_op_DescribeUser.go:59`,
`api_op_ListUsers.go:65`) are `types.User`, so the SDK models this field on
exactly the two read paths the issue named. `serializers.go:1360-1362` omits
the JSON key entirely when the pointer is nil (no empty-string emission).
No dedicated `DescribeUserCustomPermission` op exists in the SDK's op list
(unlike `DescribeAccountCustomPermission`/`DescribeRoleCustomPermission`,
which do) -- `DescribeUser`/`ListUsers` are the only read path for this
value, so the write was genuinely unobservable before this fix.

Verdict: real bug, write path correct, read path wrong. Fix: added
`CustomPermissionsName string` to the domain `User` struct (`types.go`),
populated it in `DescribeUser` and `ListUsers` from `b.userCustomPermissions`,
and emitted it from `userToMap` (`handler_user.go`) only when non-empty,
matching the SDK's omit-when-unset semantics (existing precedent:
`handler_agents.go`'s `agentToMap`).

Adjacent fix in the same map: `DeleteUser`/`DeleteUserByPrincipalID`
(`user.go`) previously cleaned up group memberships on delete
(`removeUserFromAllGroups`) but not `userCustomPermissions`, so a deleted
and re-registered user with the same name would have silently inherited a
stale custom-permissions assignment -- invisible before this fix since
nothing read the map, but a live bug once `DescribeUser`/`ListUsers` do.
Added the matching `delete(b.userCustomPermissions, ...)` call to both.

Not changed: `RegisterUser` and `UpdateUser` also return `*User` via the
same `toUser()` path but were left alone -- the issue named only
`DescribeUser`/`ListUsers`, and a freshly-registered user has no
custom-permissions entry yet by construction. `UpdateUser` returning a
stale/absent value when a custom permission was set earlier via a separate
call is a real, smaller version of the same gap; flagging it here rather
than fixing it to keep this change scoped to the named issue.

Regression test: `TestQuickSight_UserCustomPermission_ReflectedInReads`
(`handler_custompermissions_test.go`) -- registers a user, asserts
`CustomPermissionsName` absent from `DescribeUser`, calls
`UpdateUserCustomPermission`, asserts it now appears in both `DescribeUser`
and `ListUsers`, deletes it, asserts absent again. Proved against
unmodified code by reverting only the `DescribeUser`/`ListUsers` population
lines (keeping the struct field so it still compiles): the two
"surfaces it" subtests failed with `expected: string("cp1") / actual:
<nil>(<nil>)`; restored and reconfirmed green.

Also fixed as a side effect: `ListUsers`'s `//nolint:dupl` and
`ListIngestions`'s (`dataset.go:388`) `//nolint:dupl` both went from
matched-and-suppressed to genuinely non-duplicate once `ListUsers`'s body
changed shape (dupl's clustering had paired these two `List*` functions;
the new lines in `ListUsers` broke that pairing, same mechanism as the
2026-08-31 `ListThemes`/`UpdateTheme` note above). `nolintlint` flagged both
as unused; removed both now-dead directives.

Gates (`services/quicksight/` only): `go build`, `go vet`,
`go test -race ./services/quicksight/...` all clean.
`golangci-lint run services/quicksight/...` -- `0 issues.`
`go test ./pkgs/persistence/ -run TestSnapshotVersionGuard` -- PASS
(read-only; no persisted struct field was added -- `CustomPermissionsName`
lives on the domain `User` type, not `storedUser` or the snapshot).

## Dashboard per-version history re-audit (2026-09-07, gopherstack-5oop)

Filed title-only ("dashboards have no per-version history unlike the
Template family, so DeleteDashboard with a VersionNumber can only validate
and no-op"), empty description. Re-derived and verdict: **mostly already
true and already disclosed** (the exact validate-and-no-op behavior the
title names is the gopherstack-86y fix above, filed the same day as this
issue as its own deliberate, disclosed follow-up) -- but re-deriving from
scratch surfaced one genuinely fixable gap the title didn't call out:
deleting a version left no trace at all, so `ListDashboardVersions` still
listed it and a second delete of the same version kept silently
succeeding. Fixed that; declined the full per-version-content refactor as
out of reach for this pass, per this issue's own scope guidance and
gopherstack-86y's prior assessment.

**Crux quote**, `api_op_DeleteDashboard.go:39-40` (aws-sdk-go-v2/service/quicksight
v1.129.0): `// The version number of the dashboard. If the version number
property is provided, only the specified version of the dashboard is
deleted.` Confirmed against the wire model too --
`service-2.json`'s `DeleteDashboardRequest.VersionNumber` documentation is
character-for-character the same sentence, bound `"location": "querystring",
"locationName": "version-number"`. So real AWS's `DeleteDashboard` with a
`VersionNumber` deletes *that one version's* stored content and leaves the
dashboard and its other versions intact -- it is not a whole-dashboard
delete filtered by a parameter, and it is not a no-op.

**What Template does differently** (the reference implementation): `storedTemplate`
(`templates.go:25`) carries `Versions map[int64]*storedTemplateVersion` plus
`LatestVersion int64`. `DeleteTemplate` (`templates.go:190`) with a nonzero
`versionNumber` does `delete(t.Versions, versionNumber)` for real, 404s
(`ErrTemplateNotFound`) if that key is already absent, and reassigns
`LatestVersion` if the deleted version was the latest. `storedDashboard`
(`models.go:140`, before this fix) has no such map -- `Definition`,
`ThemeArn`, `VersionDescription`, `Status` are single mutable fields
clobbered by every `UpdateDashboard`, so there is no per-version *content*
to delete. This was already correctly diagnosed in the `DescribeDashboard`
row above (gopherstack-86y) and in `DeleteDashboard`'s own doc comment
(`dashboard.go:103-111`, pre-fix): "This backend has no real per-version
history ... validates the version exists ... and leaves the dashboard
untouched (success, no-op) rather than either fabricating true per-version
removal ... or deleting everything."

**Confirmed via `bd show`**: gopherstack-5oop (created 2026-09-04) and
gopherstack-86y (the audit epic task that produced the validate-and-no-op
fix, closed the same day) share a creation date -- consistent with 5oop
being 86y's own disclosed-follow-up ticket for the item its PARITY.md note
explicitly declined: "A full fix requires the same per-version-map
architecture Template already uses -- a genuine refactor across
Create/Update/Describe/ListDashboardVersions/persistence with a blast
radius large enough to be its own change." Re-reading the current code
confirmed that fix is present and working as documented (`dashboard.go`'s
`DeleteDashboard`, pre-this-pass): a nonzero `versionNumber` outside
`[1, VersionNumber]` already returned `ErrDashboardVersionNotFound`
(`ResourceNotFoundException`), and an in-range one already left the
dashboard untouched rather than deleting it -- so the specific defect this
issue's own scope guidance called out ("if DeleteDashboard accepts a
VersionNumber and silently no-ops... if [real AWS] returns an error and
gopherstack returns success, that is a real bug") does not exist for a
*first* delete of a genuinely nonexistent version; it was already fixed.

**What was still wrong** (found by re-deriving rather than trusting the
title): a *repeat* delete of the same, already-"deleted" version kept
returning success instead of `ErrDashboardVersionNotFound` -- the no-op
left no record that the version had been touched, so from the caller's
perspective delete had no effect at all: `ListDashboardVersions` (which
synthesizes `DashboardVersion` entries for `1..VersionNumber`, see its
`gopherstack-86y` note above) kept listing the "deleted" version forever,
and nothing stopped `UpdateDashboardPublishedVersion` from publishing a
version the caller had just deleted. That is a real, narrow, fixable
defect distinct from the full-history structural gap: it doesn't require
storing per-version content, only remembering which version numbers have
been deleted.

**Fix** (verdict (a) for this narrow piece, verdict (b) confirmed for the
rest): added `storedDashboard.DeletedVersions map[int64]bool`
(`models.go`, `json:"deletedVersions,omitempty"`, purely additive -- see
gate below). `DeleteDashboard` (`dashboard.go`) now checks and sets it;
`ListDashboardVersions` skips deleted version numbers when synthesizing its
range; `UpdateDashboardPublishedVersion` now also rejects a deleted version
number. `DescribeDashboard` is unchanged -- it still doesn't read
`DescribeDashboardInput.VersionNumber` at all (a separate, already-disclosed
gap in the same `DescribeDashboard` PARITY row: "Version.VersionNumber/
ThemeArn/Description report the latest in-memory state... not the specific
historical version"), and fixing that would require exactly the
per-version-content storage this pass declines to build speculatively.

**Regression test**: `TestQuickSight_DeleteDashboard_SpecificVersion`
(`handler_dashboard_test.go`), modeled directly on
`TestQuickSight_DeleteTemplate_SpecificVersion`
(`handler_templates_test.go:354`). Confirmed failing against unmodified
code:

```
handler_dashboard_test.go:288: Error: Should not be: 1
    Messages: deleted version 1 must not be listed
handler_dashboard_test.go:294: Error: Not equal:
    expected: 404
    actual  : 200
```

Passes after the fix. No pre-existing test was modified.

**Gates** (`services/quicksight/` only): `go build`, `go vet`,
`go test -race -count=1 ./services/quicksight/...` all clean.
`golangci-lint run ./services/quicksight/...` -- `0 issues` (one
`govet: shadow` finding in the new test's first draft, fixed by renaming
the inner-scope `ok`).
`go test ./pkgs/persistence/ -run TestSnapshotVersionGuard` -- failed
before `-update` with "quicksight: backendSnapshot fields changed without a
version bump; ... this is bookkeeping, not a version-bump case: every old
field is still present unchanged, so the diff is additive only and needs
no bump", exactly matching `storedDashboard.DeletedVersions` being a new
`omitempty` map field. Ran `go test ./pkgs/persistence/ -run
TestSnapshotVersionGuard -update` to refresh
`pkgs/persistence/testdata/snapshot_inventory.json` (one line added, no
`quicksightSnapshotVersion` bump -- correct, since `encoding/json` decodes
an older snapshot missing this field fine); re-ran read-only and it passes.

**Confidence**: high on the SDK-behavior read (doc comment and wire model
agree verbatim) and on the current-code read (all claims verified by
reading `dashboard.go`/`templates.go`/`models.go` directly, not from
memory). Not independently verified: whether real AWS also blocks deleting
the *currently published* version of a dashboard (`ConflictException` is a
possible error on `DeleteDashboard`, but its shape doc
-- `service-2.json`'s `ConflictException.documentation`: "Updating or
deleting a resource can cause an inconsistent state." -- is generic and
gives no version-specific detail, and no AWS example/doc text found in
`botocore`'s `examples-1.json` covers this case either); left unimplemented
rather than guessed.
