---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: ssm
sdk_module: aws-sdk-go-v2/service/ssm@v1.77.0
last_audit_commit: d3b4494d3
last_audit_date: 2026-08-21
overall: A                 # cursor-population sweep (2026-08-29, fix/wrapper-key-sweep-rds-cloudwatch-sqs-sns):
                            # audited every List/Describe/Get op that declares a real NextToken (53 of
                            # 80 ops, from the pinned SDK Output structs directly, not by grep) for the
                            # elbv2-style bug: a response struct that declares NextToken/a request that
                            # models MaxResults+NextToken, where the handler never populates/reads either
                            # so a paginating client silently sees one truncated page. 15 ops were
                            # genuinely broken and are now fixed via the existing paginateSlice helper
                            # (store.go:257): DescribeEffectiveInstanceAssociations, DescribeInstance-
                            # AssociationsStatus, DescribeAssociationExecutions, DescribeAssociation-
                            # ExecutionTargets, DescribeAutomationStepExecutions, ListNodesSummary,
                            # DescribeInstancePatches, DescribeInstanceProperties, DescribeInstancePatch-
                            # StatesForPatchGroup, DescribeInstancePatchStates, DescribeInventoryDeletions
                            # (a second, independent bug from this family's epoch-seconds fix below --
                            # NextToken was still never set), DescribeMaintenanceWindowExecutionTasks,
                            # DescribePatchProperties, and DescribeMaintenanceWindowTargets/Tasks -- the
                            # last two had NO MaxResults/NextToken/Filters members on either their Input
                            # or Output structs at all (worse than "declared but unpopulated": the real
                            # SDK client had no field to even ask for a second page), now added and wired
                            # through a new shared windowScopedPage[T] helper (maintenance_window.go) so a
                            # third window-scoped Describe op reuses it instead of hand-rolling a fourth
                            # copy. Six ops loop over store.Table.All() (unspecified Go-map order per its
                            # own doc comment) to build the page; a sort.Slice by a stable key (Associat-
                            # ionId/InstanceId/WindowTargetId/WindowTaskId/BaselineName as applicable) was
                            # added ahead of pagination in each -- without it the offset-index scheme
                            # paginateSlice uses would skip/duplicate items across pages even though a
                            # cursor was returned; TestDescribeInstanceProperties_Pagination caught this
                            # exact miss during self-review (duplicate ActivationId on both pages) before
                            # a sort was added. Proven via 3 new real-SDK-client tests (pagination_cursor_
                            # fixes_test.go: DescribeAssociationExecutions via repeated StartAssociations-
                            # Once, DescribeInstanceProperties via repeated CreateActivation, DescribeMain-
                            # tenanceWindowTargets via repeated RegisterTargetWithMaintenanceWindow), each
                            # confirmed to fail against the pre-fix code. The remaining 38 ops with a real
                            # NextToken were re-verified correct (paginateSlice or equivalent already
                            # wired) or are provably bounded and left alone with the reason recorded in
                            # their own note / gaps below: ListAssociationVersions and DescribeMaintenance-
                            # WindowExecutions/Schedule/ExecutionTaskInvocations always return <=1 synthetic
                            # record (no history modeled); ListDocumentMetadataHistory.ReviewerResponse and
                            # GetInventorySchema's Custom: types are always empty/a fixed 13-entry built-in
                            # catalogue; GetOpsMetadata/GetOpsSummary's un-wired pagination is a pre-
                            # existing DELIBERATE, documented scope decision (gopherstack-a250) correctly
                            # left alone, not re-litigated. DescribeInstanceProperties' Filters/Filters-
                            # WithOperator remain unhonored (adjacent bug, out of this pass's scope, not
                            # fixed) -- see gaps. organizations/directoryservice/waf were also swept for
                            # this class this pass and came back clean (organizations: ListParents/
                            # ListRoots are AWS-structurally bounded to exactly one item; directoryservice:
                            # DescribeHybridADUpdate never truncates so its declared-but-unset NextToken is
                            # truthful; waf: ListSubscribedRuleGroups' backend is permanently empty, no
                            # real AWS Marketplace subscription state to page over) -- no fixes needed in
                            # those three services.
                            # --- doc-prose/bidirectional re-audit pass 11 (2026-08-22) history below, preserved ---
                            # gopherstack-enpq (2026-08-22, doc-prose/bidirectional re-audit pass 11):
                            # pass 10 closed out ssm by cmd/structfielddiff's own method (field-list
                            # diff against the pinned SDK). That method never asks whether an op can be
                            # CALLED the way its own doc prose prescribes -- exactly the gap that turned
                            # kinesis's claimed A-grade sweep into 11 more real bugs and cloudwatchlogs's
                            # into 2 missing alternate identifiers (see gopherstack-enpq's own bd notes).
                            # Applying that lens to ssm found one real bug of the same class: SendCommand
                            # (commands family) -- see families.commands below. This pass's coverage is
                            # partial and explicitly NOT a re-audit of all 152 ops; see the dated note
                            # under families.commands and the "doc-prose pass 11" ## Notes section below
                            # for exactly what was and was not covered.
                            # --- structfielddiff pass 10 (2026-08-21) history below, preserved ---
                            # gopherstack-enpq (2026-08-21, structfielddiff pass 10): documents is now
                            # FIXED, closing out the gopherstack-enpq ssm sweep -- every family this
                            # campaign touched is settled and no family remains partial. The three ops
                            # pass 7 diffed but disclosed rather than fixed (documentPermissionsStore was
                            # a flat []string, with no per-account version pin to plug SharedDocumentVersion/
                            # AccountSharingInfoList into) are now fixed: DeleteDocument honors
                            # DocumentVersion/VersionName version-scoping instead of always deleting the
                            # whole document, and rejects deleting a still-shared document
                            # (InvalidDocumentOperation, one of its own declared errors) -- an existing
                            # test had asserted the opposite and was corrected. ModifyDocumentPermission
                            # now tracks a per-account SharedDocumentVersion pin via a new, purely
                            # ADDITIVE documentSharedVersionsStore (region -> document -> account ->
                            # pinned version) kept as a companion map rather than reshaping
                            # documentPermissions' on-disk type, so no ssmSnapshotVersion bump was needed
                            # (confirmed by pkgs/persistence's TestSnapshotVersionGuard, which hard-fails
                            # on exactly the destructive pattern this would otherwise have been).
                            # DescribeDocumentPermission now paginates (MaxResults/NextToken) and reports
                            # real AccountSharingInfoList entries instead of a permanently-empty stub.
                            # --- structfielddiff pass 9 (2026-08-21) history below, preserved ---
                            # gopherstack-enpq (2026-08-21, structfielddiff pass 9): patch-baselines (16
                            # ops, the last family gopherstack-enpq's own history had recorded as
                            # entirely untouched by structfielddiff) is now settled -- 6 real bugs fixed:
                            # PatchStatus.ApprovalDate was a string RFC3339 timestamp where the real
                            # member is epoch seconds (a real client failed to unmarshal it at all for
                            # any approved patch); Patch.State had no wire representation in types.Patch
                            # yet leaked onto the wire from the built-in catalogue (removed, was also
                            # dead code internally); effectivePatchesForBaseline read the available-
                            # patches map directly instead of through the lazy-seeding helper
                            # DescribeAvailablePatches itself uses, so its output silently depended on
                            # call order (fixed, lock upgraded RLock->Lock to match); the fabricated
                            # enum value "AVAILABLE" (real: PENDING_APPROVAL) and rejected-patches being
                            # dropped from the effective set entirely (now EXPLICIT_REJECTED) were both
                            # fixed in the same function; DescribeAvailablePatches' Filters was parsed
                            # and ignored, and DescribePatchGroupsInput had no Filters member at all --
                            # both now wired. 8 of the family's 16 ops were on TestStubOps_SimpleCalls's
                            # bare-{}-body stub list and all 8 read nothing; DeregisterPatchBaselineForPatchGroup/
                            # RegisterPatchBaselineForPatchGroup were not on that list but had the
                            # identical defect; DeletePatchBaseline/GetPatchBaseline/UpdatePatchBaseline
                            # separately had the wrong error CLASS for a missing required BaselineId
                            # (DoesNotExistException instead of ValidationException) rather than a
                            # silent-success stub. Two existing tests ratified defects and were
                            # corrected -- see families.patch-baselines for full detail and gaps: for
                            # what was disclosed rather than fixed (DescribePatchProperties' deeper
                            # data-source/semantics bug, Replace, ClientToken, BaselineOverride, 6
                            # missing DescribePatchGroupStateOutput counters, 4 unhonored filter keys).
                            # documents remains PARTIAL (3 of 12 ops disclosed, not fixed, since
                            # gopherstack-enpq pass 7) -- the one family left unsettled by this
                            # campaign's method as of this pass.
                            # --- structfielddiff pass 7 (2026-08-21) history below, preserved ---
                            # gopherstack-enpq (2026-08-21, structfielddiff pass 7): two more families
                            # settled via cmd/structfielddiff -- cloud-connectors (6 ops, RE-VERIFIED,
                            # genuinely clean, no new findings after checking every enum/serializer/
                            # deserializer directly) and sessions (7 ops, 1 real bug: DescribeSessions
                            # marshalled the internal Session record straight to the wire, leaking
                            # Parameters/StreamUrl/TokenValue -- three fields types.Session does not
                            # declare -- the same reused-domain-struct class pass 6's GetParameter fix
                            # was. Fixed via a new SessionOutput projection type). documents (12 ops)
                            # STILL NOT fully settled -- partially reached: CreateDocument/UpdateDocument/
                            # DescribeDocument/GetDocument/ListDocuments/ListDocumentVersions were
                            # diffed and fixed (Attachments was a functional no-op with a wrong wire
                            # key; DisplayName/Hash/HashType/Sha1/TargetType-on-Update were missing
                            # entirely), and ListDocumentMetadataHistory/UpdateDocumentMetadata/
                            # UpdateDocumentDefaultVersion had their missing required-field validation
                            # fixed (all three silently accepted an empty body from
                            # TestStubOps_SimpleCalls's stub-op list even though the real ops require
                            # Name+Metadata / Name+DocumentReviews / Name+DocumentVersion) -- but
                            # DescribeDocumentPermission/ModifyDocumentPermission/DeleteDocument WERE
                            # structfielddiff'd this pass but their real findings (ModifyDocumentPermission
                            # has no SharedDocumentVersion member at all; DescribeDocumentPermission has
                            # no MaxResults/NextToken pagination and its AccountSharingInfoList is a
                            # permanently-empty []any stub even for accounts ModifyDocumentPermission DID
                            # add; DeleteDocument has no DocumentVersion/Force members, so a version-scoped
                            # delete request silently deletes the whole document instead) were disclosed
                            # rather than fixed this pass -- see gaps. Still entirely unswept by this
                            # method: patch-baselines, maintenance-windows,
                            # state-manager-associations, ops-center, automation-executions.
                            # --- structfielddiff pass 6 (2026-08-21) history below, preserved ---
                            # gopherstack-enpq (2026-08-21, structfielddiff pass 6): four more families
                            # settled via cmd/structfielddiff -- resource-data-sync (4 ops, genuinely
                            # untouched by any prior pass), nodes (2 ops, RE-VERIFIED, no new findings),
                            # commands (5 ops), parameter-store (10 ops) -- 21 ops total this pass, on
                            # top of pass 4's 24. Still unswept by this method: documents (12 ops),
                            # sessions, patch-baselines, maintenance-windows,
                            # state-manager-associations, ops-center, automation-executions,
                            # cloud-connectors (all previously audited by op-by-op reads, not by
                            # structfielddiff -- see each family's own note for what's trusted vs.
                            # re-derived). kinesis is a separate service, out of this file's scope.
                            # Real bugs found in every family attempted except nodes: CreateResourceDataSync's
                            # S3Destination/SyncSource had NO Go struct members at all (a real
                            # SyncFromSource create's source config was silently dropped, recoverable
                            # only via a follow-up UpdateResourceDataSync call); ListCommands'
                            # InstanceId filter and CancelCommand's InstanceIds scoping both existed as
                            # struct fields but were never read in the handler body (CancelCommand
                            # always cancelled every invocation regardless of which ones were named --
                            # the same "field parsed, body never consults it" class pass 5 warned is
                            # worse than a wire miss); and parameter-store had the pass's headline
                            # finding -- GetParameter/GetParameters/GetParametersByPath's wire type
                            # (types.Parameter) was Parameter, the SAME struct used for internal
                            # storage, so Description/KeyId/Tier/AllowedPattern/Policies/Tags -- six
                            # fields with NO counterpart on the real narrower wire shape -- were
                            # fabricated on every Get* response, while DescribeParameters'
                            # ParameterMetadata.Policies was the wrong TYPE entirely (a raw string
                            # instead of []ParameterInlinePolicy, which a real aws-sdk-go-v2 client
                            # cannot even unmarshal). Two existing tests (parameter_encryption_test.go)
                            # ratified the fabricated KeyId field on GetParameter's response body,
                            # corrected rather than left in place. See ops:/families:/gaps: below and
                            # this pass's receipt for full detail, SDK citations, and hand-revert proof.
                            # --- structfielddiff pass 4 (2026-08-14) history below, preserved ---
                            # gopherstack-enpq (2026-08-14, structfielddiff pass 4 of the sqs/sns/
                            # cloudwatchlogs/kinesis sweep): ssm is 152 ops, too large to sweep to
                            # genuine completion in one sitting -- split by whole op families per the
                            # issue's own guidance rather than an arbitrary op-count cutoff. THIS PASS
                            # settled 7 families completely (24 ops): tags, resource-policies,
                            # service-settings, compliance, inventory, managed-instance, activations.
                            # This is a DIFFERENT method than the op-by-op reads that produced the
                            # rest of this file's history below -- it does not supersede that history,
                            # it independently re-verifies the families it touched via mechanical
                            # struct-field diff against the pinned SDK (cmd/structfielddiff) plus
                            # hand-verification against serializers.go/deserializers.go, and found
                            # real bugs the op-by-op passes had missed. 7 real bugs fixed, all with
                            # tests driving the real aws-sdk-go-v2 client, each hand-reverted (both
                            # halves independently where a fix had two) and confirmed to fail against
                            # the unfixed code before restoring byte-identical -- see families below
                            # for each. NOT touched this pass, deliberately: parameter-store, documents,
                            # commands (this pass's own next-candidate families) and everything already
                            # untouched per gopherstack-enpq's history (sessions, patch-baselines,
                            # maintenance-windows, state-manager-associations, ops-center,
                            # automation-executions, cloud-connectors, nodes, resource-data-sync) --
                            # still genuinely unswept by this method, not re-confirmed.
                            # --- history below: parity-3 PHASE 2 (2026-07-24) pass notes, preserved ---
                            # parity-3 PHASE 2 (2026-07-24): closed every remaining gap from the
                            # 2026-07-23 A- pass. State Manager associations (CreateAssociationInput
                            # AND UpdateAssociationInput AND CreateAssociationBatchRequestEntry) now
                            # carry all 11 previously-missing fields (bd gopherstack-ouvq, closed).
                            # OpsCenter (CreateOpsItemInput/UpdateOpsItemInput) now carries all 7
                            # previously-missing Change-Manager fields (bd gopherstack-iq4m, closed).
                            # PatchBaseline.ApprovedPatchesEnableNonSecurity converted bool -> *bool
                            # across Create/Update/PatchBaseline so UpdatePatchBaseline can explicitly
                            # turn the flag off, not just on. NoChangeNotification/ExpirationNotification
                            # parameter policies are now actually evaluated by a new janitor sweep and
                            # emit real events through an injectable ParameterPolicyNotifier (see
                            # families.parameter-store and Notes) -- the EventBridge-side adapter
                            # (services/eventbridge/ssm_integration.go) is implemented and proven via
                            # a cross-package test, only the cli.go injection line remains (see
                            # Notes: "cli.go wiring still needed"). ListCloudConnectors/
                            # ValidateCloudConnector's MaxResults bound is no longer a guess -- AWS
                            # published the real API reference pages (API_ListCloudConnectors.html /
                            # API_ValidateCloudConnector.html) sometime between the 2026-07-23 and
                            # 2026-07-24 passes; re-checked and confirmed present this pass (Minimum 0/
                            # Maximum 10 for List, Minimum 0/Maximum 75 for Validate), now enforced
                            # with a real ValidationException instead of silently accepting any value.
                            # ValidateCloudConnector's inability to make a real outbound Azure call
                            # remains a genuine, structural sandbox impossibility (see gaps) -- not
                            # closed, and cannot be. Prior pass's headline epoch-seconds fix and
                            # Sessions/PatchBaselines/MaintenanceWindows re-verification carried
                            # forward unchanged (files not touched this pass).
                            # --- history below: parity-sweep-3 (2026-07-23) pass notes, preserved ---
                            # THIS PASS (parity-sweep-3, worker=ssm): closed all 5 previously-deferred
                            # families and re-verified the 4 previously-open gaps. Headline finding:
                            # a systemic wire-shape bug affecting ~9 structs across 6 files -- every
                            # raw `time.Time`/`*time.Time` JSON field (AssociationExecution.ExecutionDate,
                            # MaintenanceWindowExecution*.StartTime/EndTime x4 structs,
                            # InstanceInformation/NodeInfo/InstanceAssociationStatusInfo.RegistrationDate/
                            # ExecutionDate, InstancePatchState.OperationStartTime,
                            # PatchComplianceData.InstalledTime, ResourceDataSync.SyncCreatedTime/
                            # LastSyncTime, InventoryDeletion.DeletionStartTime) was serializing as a
                            # Go-default RFC3339Nano *string*, but real AWS SSM (awsjson1.1) always
                            # encodes DateTime as an epoch-seconds JSON *number*
                            # (smithytime.ParseEpochSeconds, confirmed directly in aws-sdk-go-v2's
                            # deserializers.go for every one of these fields) -- a real aws-sdk-go-v2
                            # client would fail to deserialize these responses. Fixed by converting
                            # every affected field to float64 (UnixTimeFloat), this package's existing,
                            # already-correct convention for every other timestamp. Also: Sessions
                            # (deferred) fully re-verified and fixed (see families.sessions); Patch
                            # baselines and Maintenance windows re-verified with real field-diff fixes;
                            # State Manager associations and OpsCenter spot-checked with one real fix
                            # each (OpsItem.Priority) but NOT fully field-diffed -- see families and
                            # gaps below for exactly what remains open. Every ops: row carried over
                            # from the 2026-07-11 audit (last_audit_commit 2d2b1b9b) whose backing
                            # files were not touched this pass is trusted unchanged per protocol.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  # --- gopherstack-enpq, structfielddiff pass 4 (2026-08-14) ---
  AddTagsToResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified via structfielddiff, matches api_op_AddTagsToResource.go exactly, no changes needed"}
  RemoveTagsFromResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified, matches exactly"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: n/a, note: "re-verified, matches exactly"}
  PutResourcePolicy: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "FIXED -- PolicyId/PolicyHash had NO Go struct members at all (api_op_PutResourcePolicy.go's own doc comment: \"Creates or updates ... To update a policy, you must specify PolicyId and PolicyHash\"), so every call unconditionally appended a new policy -- a real client doing the documented update flow got a growing pile of duplicates instead of one updated policy. Now: PolicyId supplied looks up the existing policy (ResourcePolicyNotFoundException if absent) and updates in place after a PolicyHash match (ResourcePolicyConflictException on mismatch); PolicyId omitted creates as before. ResourceArn/Policy required-field validation added (previously accepted empty)."}
  DeleteResourcePolicy: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "FIXED -- PolicyHash had NO Go struct member (real DeleteResourcePolicyInput: ResourceArn/PolicyId/PolicyHash all required, PolicyHash exists specifically \"to prevent multiple calls from attempting to overwrite a policy\"), so any caller could delete any policy by ID with no concurrency check at all, and an empty ResourceArn silently no-op'd success instead of erroring. Now validates all three required fields and returns ResourcePolicyConflictException on a hash mismatch, ResourcePolicyNotFoundException on an unknown PolicyId. Also fixed: ErrResourcePolicyNotFound was declared in errors.go but had the WRONG error code (\"ResourcePolicyInvalidRequest\" instead of the real \"ResourcePolicyNotFoundException\") and had no case in classifySSMErrorExtended at all -- same missing-mapping bug class ResourceDataSync had (gopherstack-4ggy), just never triggered because nothing returned the sentinel."}
  GetResourcePolicies: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED -- MaxResults/NextToken had NO Go struct members at all (real GetResourcePoliciesInput has both); GetResourcePolicies always returned every policy for a resource with no pagination. Now paginates via the shared paginateSlice helper."}
  GetServiceSetting: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED -- ARN and LastModifiedDate had NO Go struct members at all (real ServiceSetting: types/types.go:5818). ARN now built from the real documented shape (api_op_UpdateServiceSetting.go:46: \"arn:aws:ssm:<region>:<account>:servicesetting<settingId>\"). LastModifiedDate is this package's epoch-seconds float64 convention, set on UpdateServiceSetting, omitted for Default (never-customized) settings. LastModifiedUser deliberately NOT modeled -- see gaps."}
  UpdateServiceSetting: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED -- see GetServiceSetting; now stamps LastModifiedDate on write"}
  ResetServiceSetting: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED -- see GetServiceSetting; ARN now populated on the Default response too"}
  PutComplianceItems: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "FIXED -- ExecutionSummary (types.ComplianceExecutionSummary, api_op_PutComplianceItems.go: required) had NO Go struct member at all and was never validated; ComplianceType/ResourceType (also required) were never validated either, and per-item Severity/Status (required on types.ComplianceItemEntry) were never validated. Now all enforced with ValidationException. UploadType (COMPLETE/PARTIAL) is accepted but not evaluated -- see gaps."}
  ListComplianceItems: {wire: fixed, errors: ok, state: fixed, persist: n/a, note: "FIXED (two bugs) -- (1) ComplianceItem.Id and .ExecutionSummary had NO Go struct members at all, so real ComplianceItem output fields silently never round-tripped. (2) ListComplianceItemsInput modeled a singular \"ResourceId\"/\"ResourceType\" wire key that does not exist on the real, and-until-now-never-checked, wire shape -- the real members are ResourceIds/ResourceTypes, both LISTS (api_op_ListComplianceItems.go). A real SDK client's ResourceIds filter was silently discarded by every prior version of this handler; filtering never worked for a real caller. Now ResourceIds/ResourceTypes (both []string) are modeled and applied. Filters ([]ComplianceStringFilter) still unmodeled -- see gaps."}
  ListComplianceSummaries: {wire: ok, errors: ok, state: ok, persist: n/a, note: "re-verified via structfielddiff -- MaxResults/NextToken match; Filters unmodeled, see gaps (shared with ListComplianceItems/ListResourceComplianceSummaries)"}
  ListResourceComplianceSummaries: {wire: ok, errors: ok, state: ok, persist: n/a, note: "re-verified -- MaxResults/NextToken match, no ResourceId/ResourceType top-level filter on the real input either so no wire-key bug here (unlike ListComplianceItems); Filters unmodeled, see gaps"}
  PutInventory: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified -- Context/TypeName/SchemaVersion/CaptureTime/ContentHash/Content all match. PutInventoryOutput.Message (informational only, no behavioral content) not modeled -- low-value, disclosed. Merge-by-TypeName semantics proven correct."}
  GetInventory: {wire: ok, errors: ok, state: ok, persist: n/a, note: "re-verified, existing fields correct; Aggregators/Filters/ResultAttributes unmodeled (no query/filter engine over inventory data) -- disclosed, not rushed, see gaps"}
  GetInventorySchema: {wire: ok, errors: ok, state: n/a, persist: n/a, note: "static built-in AWS:/Custom: schema catalog, matches real SSM's documented inventory types (TypeName/Version only, confirmed against InventorySchemaItem in models_inventory.go). real InventoryItemSchema.Attributes ([]InventoryItemAttribute, required) is not modeled -- gopherstack's static built-in schema catalog has no per-type attribute list to draw from without fabricating AWS's actual field names, disclosed rather than invented, see gaps. (2026-08-23, gopherstack-fg0u: merges a duplicate entry that omitted this disclosed gap -- verified against source, the gap is real and current.)"}
  DeleteInventory: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED -- DryRun (real member, api_op_DeleteInventory.go: \"view a summary of the deletion request without deleting any data\") had NO Go struct member at all, so a caller validating a delete before committing got a real, irreversible delete instead -- more permissive than AWS. Now DryRun computes and returns the same DeletionSummary without mutating the store or recording a deletion job. ClientToken/SchemaDeleteOption not modeled -- disclosed, see gaps. Records a real DeletionId job consumed by DescribeInventoryDeletions."}
  DescribeInventoryDeletions: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified, matches. FIXED this pass — same epoch-seconds bug, InventoryDeletion.DeletionStartTime. FIXED again (cursor-population sweep, 2026-08-29): NextToken was still declared but never populated, MaxResults/NextToken never read -- now paginates via paginateSlice."}
  ListInventoryEntries: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED -- CaptureTime/SchemaVersion (real output members, api_op_ListInventoryEntries.go) had NO Go struct members at all; the matched InventoryItem already carried both, they were just never echoed onto the response. Filters ([]InventoryFilter) unmodeled -- disclosed, see gaps."}
  DeregisterManagedInstance: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified via structfielddiff, matches api_op_DeregisterManagedInstance.go exactly"}
  UpdateManagedInstanceRole: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified, matches exactly"}
  CreateActivation: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified -- RegistrationMetadata ([]types.RegistrationMetadataItem, optional tag-like metadata) not modeled, low-value, disclosed rather than added speculatively, see gaps"}
  DeleteActivation: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified, matches exactly"}
  # DescribeActivations already fully covered above (gopherstack-a250 entry) -- re-verified this pass, no changes.
  PutParameter: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "FIXED prior pass — see Notes: hierarchy-level limit, labeled-oldest-version eviction guard, Intelligent-Tiering auto-upgrade, Policies-require-Advanced-tier. FIXED (gopherstack-enpq, 2026-08-21, structfielddiff pass 6): Tags ([]types.Tag, api_op_PutParameter.go:196-204) had NO Go struct member at all -- a real client's tags supplied on create were silently dropped, only reachable afterward via a second AddTagsToResource call. Now applied via applyPutParameterTagsLocked, but only when creating a brand-new parameter (Overwrite of an existing one does not apply Tags, per the op's own doc comment: \"To add tags to an existing Systems Manager parameter, use the AddTagsToResource operation\"). TestCreateOpsWithTags_RoundTrip/parameter subtest, hand-verified failing against unfixed code, plus a second subtest proving Overwrite does not apply new tags."}
  GetParameter: {wire: fixed, errors: ok, state: ok, persist: ok, note: "selector suffix (:version/:label), SecureString decrypt, ARN population all proven correct. FIXED (gopherstack-enpq, 2026-08-21, structfielddiff pass 6) -- THE PASS'S HEADLINE FINDING: GetParameterOutput.Parameter reused the internal Parameter domain/storage struct directly as the wire type, so Description/KeyId/Tier/AllowedPattern/Policies/Tags -- six fields with NO counterpart on the real types.Parameter wire shape (types/types.go:4738-4782: ARN/DataType/LastModifiedDate/Name/Selector/SourceResult/Type/Value/Version only) -- were fabricated on every response. A real aws-sdk-go-v2 client's own JSON unmarshal silently drops unknown fields (so this was never a hard client crash), but it is a real over-disclosure: KeyId in particular leaked which KMS key encrypts a SecureString parameter through an endpoint real AWS deliberately does not expose it on. Fixed via a new ParameterOutput wire type + Parameter.toParameterOutput() projection (same pattern as this file's existing Document/DocumentDescription precedent). TWO EXISTING TESTS (parameter_encryption_test.go: TestPutParameter_KeyId_Stored, TestFull_ParameterStore_SecureString_EncryptDecrypt) RATIFIED the fabricated KeyId field, asserting its presence on GetParameter's raw response body -- corrected to assert its ABSENCE there and its presence on DescribeParameters' ParameterMetadata instead, where it actually belongs. SourceResult (real, populated only for aws:ssm:parameter/aws:ec2:image source resolution this backend has no engine for) remains unmodeled -- see gaps."}
  GetParameters: {wire: fixed, errors: ok, state: ok, persist: ok, note: "unresolvable names/labels/decrypt failures correctly become InvalidParameters entries, not a hard error. FIXED (gopherstack-enpq, 2026-08-21, structfielddiff pass 6) -- same ParameterOutput projection as GetParameter, same six fabricated fields removed from every entry in GetParametersOutput.Parameters."}
  GetParameterHistory: {wire: fixed, errors: ok, state: ok, persist: ok, note: "MaxResults 1-50 default 50 (matches AWS), label backfill via parameterLabelsStore proven correct, pagination via opaque index token. FIXED (gopherstack-enpq, 2026-08-21, structfielddiff pass 6): Policies (real, types.ParameterHistory's own []ParameterInlinePolicy member) had NO Go struct member at all on the internal ParameterHistory type, so a real client could never see which policies were attached to a historical version. New ParameterHistoryOutput wire type (mirrors GetParameter's ParameterOutput fix) projects the internal ParameterHistory (which now also stores the raw Policies JSON, same convention as Parameter.Policies) via the shared parameterPoliciesToWire converter. LastModifiedUser (real, no caller-identity infra to derive an IAM ARN from) remains unmodeled -- see gaps. TestGetParameterHistory_PoliciesWireShape, hand-verified failing against unfixed code."}
  DeleteParameter: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified via structfielddiff -- Name (required) matches exactly, no changes needed"}
  DeleteParameters: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified via structfielddiff -- Names/DeletedParameters/InvalidParameters match exactly, no changes needed"}
  GetParametersByPath: {wire: fixed, errors: ok, state: ok, persist: ok, note: "MaxResults 1-10 default 10 (matches AWS), recursive/non-recursive prefix matching, ParameterFilters proven correct. FIXED (gopherstack-enpq, 2026-08-21, structfielddiff pass 6) -- same ParameterOutput projection as GetParameter, same six fabricated fields removed from every entry in GetParametersByPathOutput.Parameters."}
  DescribeParameters: {wire: fixed, errors: ok, state: ok, persist: ok, note: "MaxResults 1-50 default 50 (matches AWS). FIXED (gopherstack-enpq, 2026-08-21, structfielddiff pass 6): ParameterMetadata.Policies was modeled as a raw string (the verbatim PutParameterInput.Policies request text) instead of the real wire shape, []types.ParameterInlinePolicy (types/types.go:4840-4857, an array of {PolicyText, PolicyType, PolicyStatus} objects) -- a real aws-sdk-go-v2 client's json.Unmarshal into that slice type would fail entirely against a JSON string where it expects an array of objects, a harder failure than the Get*-family's over-disclosure bugs fixed alongside this one. Fixed via a new parameterPoliciesToWire converter (parameter_policy_notifications.go, next to the pre-existing parseParameterPolicies it reuses) that parses the stored raw string and re-emits each entry as {PolicyText: <that entry's own JSON>, PolicyType: <its Type>, PolicyStatus: \"Finished\"} -- PolicyStatus is always \"Finished\" since this backend applies every policy synchronously and in full on PutParameter, with no Pending/InProgress/Failed phase to observe. Shared/deprecated-Filters remain unmodeled (Shared needs cross-account infra this backend does not have) -- see gaps. TestDescribeParameters_PoliciesWireShape, hand-verified failing against unfixed code."}
  LabelParameterVersion: {wire: fixed, errors: ok, state: ok, persist: ok, note: "10-label-per-version cap (appendLabelsWithLimit) and move-label-between-versions semantics proven correct; (2026-08-14, gopherstack-7185) FIXED -- LabelParameterVersionOutputFull serialized an invented AddedLabels field with no counterpart in aws-sdk-go-v2/service/ssm@v1.73.4's LabelParameterVersionOutput (InvalidLabels + ParameterVersion only, confirmed against api_op_LabelParameterVersion.go and the awsAwsjson11_deserializeOpDocumentLabelParameterVersionOutput case switch). Several existing tests asserted AddedLabels' presence, entrenching the wrong shape; corrected to verify actually-attached labels via GetParameterHistory instead. RE-VERIFIED via structfielddiff (gopherstack-enpq, 2026-08-21) -- no further changes."}
  UnlabelParameterVersion: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified via structfielddiff -- Labels/Name/ParameterVersion (all required) and InvalidLabels/RemovedLabels output match exactly"}
  CreateDocument: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-enpq pass 7 (structfielddiff): Attachments ([]AttachmentsSource) was a functional no-op -- parsed successfully off the wire but never consulted by the handler, so a supplied attachment name never appeared anywhere in the response. DisplayName/Hash/HashType/Sha1 had no Go struct members at all (Hash/HashType/Sha1 are directly computable from Content -- sha256/sha1 -- so now are). Fixed by projecting Attachments into a new AttachmentInformation{Name}-shaped field wired under the real key AttachmentsInformation (was previously marshalled under the wrong key \"Attachments\" using an over-broad DocumentAttachment{Name,Url,Hash,Size} type that was itself dead code -- nothing ever populated it). Prior-pass Content-leak fix (see Notes) unchanged."}
  GetDocument: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-enpq pass 7 (structfielddiff): DisplayName had no Go struct member; added. Prior-pass $DEFAULT/$LATEST fix (see Notes) unchanged. AttachmentsContent (real GetDocumentOutput member, types.AttachmentContent{Hash,HashType,Name,Size,Url}) deliberately not modeled -- see gaps."}
  UpdateDocument: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-enpq pass 7 (structfielddiff): TargetType and Attachments had no Go struct members at all (a caller re-targeting a document via UpdateDocument, or updating its attachments, was silently ignored); DisplayName/Hash/HashType/Sha1 same fix as CreateDocument. Version cap (maxDocumentVersionCap=1000) still proven correct."}
  DescribeDocument: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-enpq pass 7 (structfielddiff): DisplayName/Hash/HashType/Sha1/AttachmentsInformation now correctly swapped to the resolved DocumentVersion's own values when an explicit/non-latest version selector is given (previously the per-version swap only touched DocumentVersion/DocumentFormat/Status, so Hash etc. would silently describe the latest version's content while claiming to describe an older one -- would have been a real bug the moment Hash was added without this). Prior-pass Content-leak and version-selector fixes (see Notes) unchanged."}
  DeleteDocument: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "gopherstack-enpq pass 10 (2026-08-21): DocumentVersion/VersionName now scope the delete to one version (api_op_DeleteDocument.go:34-38) instead of always deleting the entire document -- proven by a real-client test asserting the sibling version survives. Deleting a document's only remaining version still deletes the document. A nonexistent DocumentVersion/VersionName is rejected (ErrDocumentNotFound; DeleteDocument's own error set omits InvalidDocumentVersion -- confirmed via deserializers.go:2182-2240, unlike GetDocument/DescribeDocument/UpdateDocument which do declare it). Also added: real AWS rejects deleting a still-shared document with InvalidDocumentOperation (one of DeleteDocument's own declared errors, deserializers.go:2225-2226) -- previously DeleteDocument ignored documentPermissions entirely; an existing test (TestInMemoryBackend_DeleteDocumentCleansUp) had asserted success deleting a shared document, corrected. Force remains parsed but inert -- real AWS requires it only for a document of type ApplicationConfigurationSchema, which this backend does not model (disclosed, same shallow-scalar class as other unmodeled document types)."}
  ListDocuments: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-enpq pass 7 (structfielddiff): DocumentIdentifier.DisplayName had no Go struct member; added. FIXED (gopherstack-uox6, value-semantics sweep, 2026-08-30): documentMatchesFilters' switch had no case for TargetType or PlatformTypes -- two of the five documented DocumentKeyValuesFilter keys (types.DocumentKeyValuesFilter doc / api_op_ListDocuments.go: 'valid keys include Owner, Name, PlatformTypes, DocumentType, and TargetType') -- so filtering on either silently matched every document instead of narrowing, identical bug shape to a switch with no default case. Both fields exist on Document (TargetType scalar, PlatformTypes []string) so both are now honored (TargetType exact-match, PlatformTypes intersect-any); Owner (needs caller-identity 'Self' resolution, unmodeled) and tag:tagName keys (needs the misc-tag store threaded in) remain gaps -- see gaps."}
  ListDocumentVersions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-enpq pass 7 (structfielddiff): DocumentVersionInfo.DisplayName had no Go struct member; added, populated per-version (the DisplayName active at the time that version was created/updated, not always the document's current one)."}
  DescribeDocumentPermission: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-enpq pass 10 (2026-08-21): MaxResults/NextToken pagination now implemented (same offset-index scheme as ListDocuments/ListDocumentVersions in this file). AccountSharingInfoList now emits real types.AccountSharingInfo{AccountId,SharedDocumentVersion} entries (was a permanently-empty []any stub) -- SharedDocumentVersion is sourced from the new documentSharedVersionsStore ModifyDocumentPermission populates. Proven by a real-client test asserting both the pinned versions and pagination behavior."}
  ModifyDocumentPermission: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-enpq pass 10 (2026-08-21): SharedDocumentVersion is now modeled and pinned per (document, account) in the new documentSharedVersionsStore -- a companion map to documentPermissionsStore kept additive rather than reshaping that field's on-disk type, so restoring an older snapshot (with no such pins) stays a safe zero-value default instead of needing an incompatible ssmSnapshotVersion bump (gopherstack-5i6p; confirmed via pkgs/persistence's TestSnapshotVersionGuard, which treats this exact addition as PURELY ADDITIVE). An omitted SharedDocumentVersion pins the document's current DefaultVersion, matching api_op_ModifyDocumentPermission.go:51-53 ('If it isn't specified, the system choose the Default version to share')."}
  UpdateDocumentDefaultVersion: {wire: ok, errors: fixed, state: ok, persist: ok, note: "gopherstack-enpq pass 7 (structfielddiff): Name and DocumentVersion are both required on the real op, but an empty body previously returned a silent empty-success stub (it was on TestStubOps_SimpleCalls's ~59-op lead list) instead of ValidationException. Fixed; verifies requested version exists in documentVersionsStore before pinning (unchanged)."}
  ListDocumentMetadataHistory: {wire: ok, errors: fixed, state: ok, persist: n/a, note: "gopherstack-enpq pass 7 (structfielddiff): Name and Metadata (the single real enum value \"DocumentReviews\") are both required on the real op, but an empty body previously returned a well-formed empty stub (it was on TestStubOps_SimpleCalls's lead list) instead of ValidationException. Fixed. Review-history state itself remains an intentional empty-stub gap -- no review workflow is modeled, see gaps."}
  UpdateDocumentMetadata: {wire: ok, errors: fixed, state: gap, persist: n/a, note: "gopherstack-enpq pass 7 (structfielddiff): Name and DocumentReviews (with a valid Action) are both required on the real op, but an empty body previously silently succeeded (it was on TestStubOps_SimpleCalls's lead list) instead of ValidationException. Fixed, including Action enum validation (SendForReview/UpdateReview/Approve/Reject). The op still does not persist or apply any review-state change -- no review workflow is modeled, see gaps."}
  SendCommand: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-enpq (2026-08-21, structfielddiff pass 6): DocumentVersion, ServiceRoleArn (echoed as Command.ServiceRole -- the real input/output member names genuinely differ, api_op_SendCommand.go), MaxConcurrency, MaxErrors all had NO Go struct members at all on either SendCommandInput or Command -- a real client's values were silently dropped, never round-tripped even though not enforced. Now modeled and echoed. Command.TargetCount/CompletedCount/ErrorCount (real, required-shape int32 members, previously always the Go zero value regardless of actual invocation outcomes) now computed from the command's own invocations via commandCounts (commands.go), can never drift out of sync. AlarmConfiguration/CloudWatchOutputConfig/NotificationConfig/DocumentHash/DocumentHashType/TriggeredAlarms/DeliveryTimedOutCount remain unmodeled -- see gaps. TestSendCommand_RoundTripsRealSDKFields (commands_test.go), hand-verified failing (undefined struct field) against unfixed code. gopherstack-s7aq (2026-09-07): MaxConcurrency/MaxErrors were accepted with NO format validation despite the wire model declaring a pattern for both -- FIXED via validateMaxConcurrency/validateMaxErrors (commands.go). MaxConcurrency's concurrency-throttling effect and MaxErrors' stop-after-N-failures effect on the actual fan-out remain unapplied -- see gaps for why (fan-out over per-instance CommandInvocation is real, but concurrency has nothing to throttle in a synchronous zero-elapsed-time emulator, and MaxErrors has no per-instance failure variance to threshold against since renderCommandOutput computes one outcome per SendCommand call, not per instance)."}
  ListCommands: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "gopherstack-enpq (2026-08-21, structfielddiff pass 6): ListCommandsInput.InstanceId (real, optional) existed as a Go struct field but was never read in the handler body -- a real client's InstanceId filter silently returned every command regardless. Fixed: now filters via slices.Contains(cmd.InstanceIDs, input.InstanceID). Filters ([]types.CommandFilter, deeper Key/Value pairs) remain unmodeled -- see gaps. TestListCommands_FilterByInstanceID, hand-verified failing against unfixed code."}
  ListCommandInvocations: {wire: ok, errors: ok, state: ok, persist: ok, note: "re-verified via structfielddiff -- CommandId/InstanceId filters and pagination already correct (see TestFull_Command_ListByInstanceId). CommandInvocation.DocumentVersion now round-trips (see SendCommand fix). CommandPlugins (per-document-step status/output breakdown), NotificationConfig, CloudWatchOutputConfig, InstanceName, TraceOutput, and Details/Filters on the input remain unmodeled -- this backend has no per-plugin execution model (a whole document runs as one synchronous unit, see command_exec.go), so a genuine per-plugin breakdown would require fabricating step names/outputs this backend has no real data for -- see gaps."}
  GetCommandInvocation: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-enpq (2026-08-21, structfielddiff pass 6): DocumentVersion, PluginName (accepted and echoed, not used to select among plugins -- see ListCommandInvocations' CommandPlugins gap), ExecutionStartDateTime/ExecutionEndDateTime (real, ISO 8601 STRINGS per the op's own doc comment -- api_op_GetCommandInvocation.go:90-109 -- NOT epoch-seconds numbers like every other timestamp in this package, a genuine format exception verified directly against the doc comment) all had NO Go struct members at all. Now modeled: ExecutionStartDateTime is always populated (this backend transitions Pending to InProgress synchronously and immediately inside SendCommand, so a real client never observes a not-yet-started invocation), ExecutionEndDateTime only once the invocation reaches a terminal status. CloudWatchOutputConfig and ResponseCode (a real shell exit code this backend has no real process to derive one from -- fabricating a specific nonzero value for Failed would be inventing data, not verifying it) remain unmodeled -- see gaps. TestGetCommandInvocation_OutputFields extended, hand-verified failing against unfixed code."}
  CancelCommand: {wire: ok, errors: fixed, state: fixed, persist: ok, note: "gopherstack-enpq (2026-08-21, structfielddiff pass 6): CancelCommandInput.InstanceIds (real, optional -- api_op_CancelCommand.go: \"If not provided, the command is canceled on every node on which it was requested\") existed as a Go struct field but was never read in the handler body -- CancelCommand always cancelled every invocation and the command itself unconditionally, regardless of which InstanceIds were actually named. This is the same functional-no-op-for-one-field bug class pass 5 flagged as worse than a wire miss, just scoped to one parameter rather than a whole handler. Fixed: only the named invocations (or all, if InstanceIds is empty) are marked Cancelled; the command-level Status only flips to Cancelled once every invocation has been. TestCancelCommand_ScopedToInstanceIDs, hand-verified failing against unfixed code."}
  DescribeActivations: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (gopherstack-a250): input was a literal struct{}; real DescribeActivationsInput (api_op_DescribeActivations.go) has optional Filters/MaxResults/NextToken, all discarded on every request. Now filters by FilterKey (ActivationIds/DefaultInstanceName/IamRole, the only 3 real DescribeActivationsFilterKeys — unrecognized keys accept-and-echo, mirroring ListNodes) and paginates via the shared paginateSlice helper (store.go). TestDescribeActivations_FiltersAndPagination (empty_struct_inputs_test.go), hand-verified failing against unfixed code."}
  DeleteAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeAssociation: {wire: ok, errors: ok, state: ok, persist: ok}
  CreatePatchBaseline: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (parity-sweep-3) — was missing ApprovalRules/GlobalFilters/Sources/RejectedPatchesAction/AvailableSecurityUpdatesComplianceStatus/ApprovedPatchesEnableNonSecurity entirely (confirmed against aws-sdk-go-v2 v1.73.4's api_op_CreatePatchBaseline.go); all now round-trip for real. FIXED phase-2 — ApprovedPatchesEnableNonSecurity converted bool -> *bool (confirmed *bool in CreatePatchBaselineInput/UpdatePatchBaselineInput/PatchBaseline via go doc). STRUCTFIELDDIFF PASS 9 (gopherstack-enpq, 2026-08-21): re-diffed, confirmed already-correct; ClientToken (real, idempotency token) deliberately left unmodeled, see gaps."}
  DeletePatchBaseline: {wire: ok, errors: ok, state: ok, persist: ok, note: "STRUCTFIELDDIFF PASS 9 (gopherstack-enpq, 2026-08-21): BaselineId is required (validateOpDeletePatchBaselineInput, validators.go) but was entirely unvalidated -- an empty BaselineId fell through to the not-found lookup and returned DoesNotExistException, the wrong error class for a missing required field. Fixed to return ValidationException before the lookup."}
  GetPatchBaseline: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — PatchGroups (patch groups currently registered with this baseline) was entirely unpopulated; now derived from the reverse patchGroup->baselineID map, excluding the synthetic default/default-<OS> bookkeeping keys. STRUCTFIELDDIFF PASS 9 (gopherstack-enpq, 2026-08-21): same wrong-error-class bug as DeletePatchBaseline -- BaselineId (required) was unvalidated and an empty value fell through to DoesNotExistException instead of ValidationException. Fixed."}
  UpdatePatchBaseline: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (parity-sweep-3) — same missing-fields gap as CreatePatchBaseline, now merges them. FIXED phase-2 — ApprovedPatchesEnableNonSecurity is now *bool so an explicit false is distinguishable from omitted and can actually turn the flag back off (previously could only ever turn it on). STRUCTFIELDDIFF PASS 9 (gopherstack-enpq, 2026-08-21): same wrong-error-class bug as DeletePatchBaseline/GetPatchBaseline, fixed. Replace (real, changes merge-vs-replace update semantics, same class as UpdateAssociation's disclosed replace-semantics gap) remains unmodeled, see gaps."}
  StartSession: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — StartSessionInput previously accepted 4 gopherstack-invented fields (OutputS3BucketName/OutputS3KeyPrefix/CloudWatchLogGroupName/CloudWatchOutputEnabled) not present anywhere in the real SDK's StartSessionInput (confirmed: only Target/DocumentName/Parameters/Reason exist) — deleted per parity-principles' invented-field rule. Session.OutputUrl (real field name SessionManagerOutputUrl, members CloudWatchOutputUrl/S3OutputUrl) is documented \"Reserved for future use\" in the SDK and is correctly never populated now. Session.AccessType now defaults to \"Standard\" (was entirely absent)."}
  TerminateSession: {wire: ok, errors: ok, state: ok, persist: ok}
  ResumeSession: {wire: ok, errors: ok, state: ok, persist: ok}
  DescribeSessions: {wire: fixed, errors: ok, state: ok, persist: ok, note: "gopherstack-enpq pass 7 (structfielddiff): marshalled the internal Session record directly, which carries Parameters/StreamUrl/TokenValue -- three fields real types.Session does not declare (fabricated fields the real deserializer's default case would silently drop, not a wire-break, but still fabrication per parity-principles). Fixed via a new SessionOutput{SessionId,Target,Status,Owner,Reason,DocumentName,AccessType,MaxSessionDuration,StartDate,EndDate} projection type. — Prior-pass notes preserved: FIXED (parity-sweep-3) — 3 real bugs: (1) State ('Active'/'History', types.SessionState) was compared directly against a session's own Status ('Connected'/'Terminated', types.SessionStatus) — two different enums — so a real client's State filter could never match; added sessionStateMatchesFilter bucketing. (2) Filters ([]SessionFilter, Target/Owner/SessionId/AccessType/Status/InvokedAfter/InvokedBefore) was accepted on the wire and silently discarded. (3) MaxResults/NextToken pagination was entirely missing (always returned every session). SessionFilter's wire keys are lowercase \"key\"/\"value\" — confirmed a deliberate AWS quirk via serializers.go, not a copy-paste bug to '''fix'''."}
  GetConnectionStatus: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass — the not-connected case returned \"notConnected\" (camelCase); real AWS's ConnectionStatus enum is all-lowercase \"notconnected\" (confirmed types/enums.go). The connected case (\"connected\") was already correct."}
  GetAccessToken: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — previously a pure stub returning a fabricated TokenValue/AccessRequestId regardless of input, matching neither the real request shape (AccessRequestId, required) nor response shape (AccessRequestStatus + Credentials{AccessKeyId,SecretAccessKey,SessionToken,ExpirationTime} — confirmed api_op_GetAccessToken.go). Now looks up a real AccessRequest created by StartAccessRequest and mints mock Credentials only when Approved."}
  StartAccessRequest: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — previously ignored Reason/Targets/Tags entirely and returned a random ID with no backing state. Now validates Reason+Targets are present (both required in the real SDK) and persists a real AccessRequest (new *store.Table[AccessRequest], services/ssm/store_setup.go), auto-approved since gopherstack has no approver workflow to model — documented rather than left as a silent no-op."}
  CreateOpsItem: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (parity-sweep-3) — Priority (confirmed present in api_op_CreateOpsItem.go) was entirely absent; now round-trips. FIXED phase-2 (bd gopherstack-iq4m, closed) — AccountId/ActualStartTime/ActualEndTime/Notifications/PlannedStartTime/PlannedEndTime/RelatedOpsItems all now round-trip, confirmed against api_op_CreateOpsItem.go. OperationalDataToDelete (an UpdateOpsItem-only field, out of the bd issue's field list) deliberately left out of scope — documented in models_ops_items.go."}
  UpdateOpsItem: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED (parity-sweep-3) — Priority now round-trips (see CreateOpsItem). FIXED phase-2 (bd gopherstack-iq4m, closed) — same 7 fields as CreateOpsItem now round-trip via UpdateOpsItem, confirmed against api_op_UpdateOpsItem.go."}
  CreateAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED phase-2 (bd gopherstack-ouvq, closed) — ApplyOnlyAtCronInterval/ComplianceSeverity/MaxConcurrency/MaxErrors/OutputLocation/ScheduleExpression/SyncCompliance/CalendarNames/AssociationDispatchAssumeRole/AutomationTargetParameterName/Duration all now round-trip, confirmed against api_op_CreateAssociation.go. Name/Targets/Parameters/DocumentVersion/AssociationName/InstanceID were already correct (parity-sweep-3). gopherstack-s7aq (2026-09-07): MaxConcurrency/MaxErrors were accepted with NO format validation despite the wire model declaring a pattern for both (same finding as SendCommand) -- FIXED via the same validateMaxConcurrency/validateMaxErrors (commands.go). Same unvalidated gap still exists on UpdateAssociation -- see gaps."}
  UpdateAssociation: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED phase-2 — same 10 optional fields as CreateAssociation (all but Name, which UpdateAssociationInput doesn't carry) now round-trip, confirmed against api_op_UpdateAssociation.go. Previously only AssociationName/DocumentVersion/Parameters/Targets were settable."}
  CreateAssociationBatch: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED phase-2 — CreateAssociationBatchRequestEntry carries the same extended fields as CreateAssociationInput, confirmed against types.CreateAssociationBatchRequestEntry."}
  CreateCloudConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW this pass — see Notes: implemented from scratch, previously entirely unimplemented (excluded from sdk_completeness_test.go rather than stubbed)"}
  DeleteCloudConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW this pass"}
  GetCloudConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW this pass"}
  ListCloudConnectors: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW (parity-sweep-3) — SubscriptionId/TenantId filters incl. documented \"NONE\" tenant-level-only value, opaque index-token pagination matching DescribeParameters. FIXED phase-2 — MaxResults bound was a guess (50, aliased to defaultDescribeMaxResults); AWS has since published the real bound (Minimum 0, Maximum 10, confirmed 2026-07-24 via API_ListCloudConnectors.html) — now enforced with ValidationException outside [0,10] instead of silently accepting any positive value."}
  UpdateCloudConnector: {wire: ok, errors: ok, state: ok, persist: ok, note: "NEW this pass"}
  ValidateCloudConnector: {wire: ok, errors: ok, state: ok, persist: n/a, note: "NEW (parity-sweep-3) — see Notes: no real Azure tenant to call out to, so findings are deterministically derived from the connector's own stored configuration rather than a fabricated always-success stub (this specific limitation remains a genuine sandbox impossibility, see gaps). FIXED phase-2 — MaxResults bound was a guess (50); AWS has since published the real bound (Minimum 0, Maximum 75, confirmed 2026-07-24 via API_ValidateCloudConnector.html) — now enforced with ValidationException outside [0,75]."}
  RegisterTaskWithMaintenanceWindow: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — Targets (the managed nodes/window-targets a task applies to; confirmed required-in-practice for Run Command tasks per api_op_RegisterTaskWithMaintenanceWindow.go) was accepted on the wire but silently discarded; now round-trips through Register/Update/Describe."}
  UpdateMaintenanceWindowTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — same Targets gap as RegisterTaskWithMaintenanceWindow"}
  CreateMaintenanceWindow: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — StartDate/EndDate/ScheduleTimezone/ScheduleOffset were confirmed present in api_op_CreateMaintenanceWindow.go but entirely absent from this package; now round-trip (stored as-is, not evaluated against Schedule — see gaps)."}
  UpdateMaintenanceWindow: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — same StartDate/EndDate/ScheduleTimezone/ScheduleOffset gap, plus AllowUnassociatedTargets was previously create-only (confirmed updatable in api_op_UpdateMaintenanceWindow.go)."}
  DescribeAssociationExecutions: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — see Notes: AssociationExecution.ExecutionDate was a raw time.Time (RFC3339 string on the wire); real AWS DateTime fields in this awsjson1.1 API are epoch-seconds numbers. FIXED again (cursor-population sweep, 2026-08-29): Input had no MaxResults/NextToken members at all and Output never set NextToken -- now paginates via paginateSlice, proven by TestDescribeAssociationExecutions_Pagination against the real SDK client."}
  DescribeAssociationExecutionTargets: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (cursor-population sweep, 2026-08-29): same defect and fix as its sibling DescribeAssociationExecutions above -- Input had no MaxResults/NextToken members, Output never set NextToken; now paginates via paginateSlice. Targets list is per-execution and caller-supplied-order already stable, no extra sort needed."}
  DescribeEffectiveInstanceAssociations: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (cursor-population sweep, 2026-08-29): Input had no MaxResults/NextToken members at all and Output never set NextToken despite the real op declaring both; now paginates via paginateSlice, sorted by AssociationId first since the source store iterates in unspecified order."}
  DescribeMaintenanceWindowExecutions: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass — same epoch-seconds bug, MaintenanceWindowExecution.StartTime/EndTime"}
  DescribeMaintenanceWindowExecutionTasks: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass — same epoch-seconds bug, MaintenanceWindowExecutionTask.StartTime. FIXED again (cursor-population sweep, 2026-08-29): Input had no MaxResults/NextToken members at all -- now paginates via paginateSlice."}
  DescribeMaintenanceWindowTargets: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (cursor-population sweep, 2026-08-29): Input AND Output had no MaxResults/NextToken members at all (real api_op_DescribeMaintenanceWindowTargets.go declares both plus Filters) -- a real SDK client had no field to even ask for a second page. Now paginates via a new shared windowScopedPage[T] helper (maintenance_window.go, also used by DescribeMaintenanceWindowTasks below) that filters+sorts (by WindowTargetId, since the source store iterates in unspecified order)+pages in one call; proven by TestDescribeMaintenanceWindowTargets_Pagination against the real SDK client. Filters remains unmodeled -- see gaps."}
  DescribeMaintenanceWindowTasks: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (cursor-population sweep, 2026-08-29): same defect and fix as its sibling DescribeMaintenanceWindowTargets above (windowScopedPage[T], sorted by WindowTaskId). Filters remains unmodeled -- see gaps."}
  DescribeMaintenanceWindowExecutionTaskInvocations: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass — same epoch-seconds bug, MaintenanceWindowExecutionTaskInvocation.StartTime"}
  GetMaintenanceWindowExecution: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass — same epoch-seconds bug"}
  GetMaintenanceWindowExecutionTask: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass — same epoch-seconds bug"}
  GetMaintenanceWindowExecutionTaskInvocation: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass — same epoch-seconds bug"}
  DescribeInstanceInformation: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — same epoch-seconds bug, InstanceInformation.RegistrationDate. FIXED (gopherstack-a250): input was a literal struct{}; real DescribeInstanceInformationInput (api_op_DescribeInstanceInformation.go) has optional Filters/InstanceInformationFilterList/MaxResults/NextToken, all discarded. Now filters on the attributes InstanceInformation actually tracks (InstanceIds/ActivationIds/AgentVersion/PingStatus/PlatformTypes) and paginates. TestDescribeInstanceInformation_FilterAndPagination, hand-verified failing against unfixed code."}
  DescribeInstanceAssociationsStatus: {wire: ok, errors: ok, state: ok, persist: n/a, note: "FIXED this pass — same epoch-seconds bug, InstanceAssociationStatusInfo.ExecutionDate. FIXED again (cursor-population sweep, 2026-08-29): Input had no MaxResults/NextToken members at all -- now paginates via paginateSlice (sorted by AssociationId first, since the source store iterates in unspecified order)."}
  DescribeInstancePatchStates: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — same epoch-seconds bug, InstancePatchState.OperationStartTime. FIXED again (cursor-population sweep, 2026-08-29): MaxResults/NextToken were modeled but never read/populated -- now paginates via paginateSlice, sorted by InstanceId for the no-InstanceIds branch."}
  DescribeInstancePatchStatesForPatchGroup: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED (cursor-population sweep, 2026-08-29): same defect and fix as its sibling DescribeInstancePatchStates above -- MaxResults/NextToken were modeled but never read/populated; now paginates via paginateSlice, sorted by InstanceId. Filters remains unmodeled -- see gaps."}
  DescribeInstancePatches: {wire: ok, errors: ok, state: ok, persist: ok, note: "FIXED this pass — same epoch-seconds bug, PatchComplianceData.InstalledTime. FIXED again (cursor-population sweep, 2026-08-29): MaxResults/NextToken were modeled but never read/populated -- now paginates via paginateSlice."}
  DescribeInstanceProperties: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (cursor-population sweep, 2026-08-29): handler took the request as `_ *DescribeInstancePropertiesInput` -- MaxResults/NextToken were modeled on the wire but structurally could never be read. Now accepts and reads input, sorts by InstanceId (activations+instance-properties tables iterate in unspecified order) and paginates via paginateSlice; proven by TestDescribeInstanceProperties_Pagination against the real SDK client, which also caught a missing sort during self-review (duplicate items across pages) before this was published. FiltersWithOperator/InstancePropertyFilterList remain unhonored -- see gaps."}
  DescribeAutomationStepExecutions: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (cursor-population sweep, 2026-08-29): Input had no MaxResults/NextToken members at all (real api_op_DescribeAutomationStepExecutions.go declares both) -- now paginates via paginateSlice. Steps is already document-order (a slice field), no extra sort needed."}
  DescribePatchProperties: {wire: fixed, errors: ok, state: ok, persist: n/a, note: "FIXED (cursor-population sweep, 2026-08-29): Input had no MaxResults/NextToken members at all -- now sorts by BaselineName (patch-baselines store iterates in unspecified order) and paginates via paginateSlice. Its pre-existing, separate data-source/filtering gap (disclosed in the patch-baselines family note below) is unchanged by this fix."}
  ListResourceDataSync: {wire: fixed, errors: ok, state: ok, persist: ok, note: "FIXED prior pass — same epoch-seconds bug, ResourceDataSync.SyncCreatedTime/LastSyncTime. FIXED this pass (gopherstack-4ggy): ResourceDataSyncItem.SyncSource (types.ResourceDataSyncSourceWithState) now echoed back per item, populated by UpdateResourceDataSync's fix below (was previously nil for every sync). FIXED (gopherstack-a250): input was a literal struct{}; real ListResourceDataSyncInput (api_op_ListResourceDataSync.go) has optional SyncType/MaxResults/NextToken, all discarded. Now filters by SyncType (an exact field match, real backing state) and paginates. TestListResourceDataSync_FilterAndPagination, hand-verified failing against unfixed code. RE-VERIFIED via structfielddiff (gopherstack-enpq, 2026-08-21, structfielddiff pass 6) alongside this pass's CreateResourceDataSync fix -- S3Destination now also echoed per item (see CreateResourceDataSync), no other gaps found."}
  UpdateResourceDataSync: {wire: fixed, errors: fixed, state: fixed, persist: ok, note: "gopherstack-4ggy: SyncSource AND SyncType (both required UpdateResourceDataSyncInput members alongside SyncName -- api_op_UpdateResourceDataSync.go:36-54) were dropped entirely; the handler read only SyncName and silently returned success on an empty one instead of erroring, and never errored on an unknown sync name either. Now both required, SyncSource's own SourceType/SourceRegions validated when present (validateResourceDataSyncSource, validators.go), and stored/echoed on the ResourceDataSync (see ListResourceDataSync). Also fixed while wiring the not-found path: ErrResourceDataSyncNotFound had NO case in classifySSMErrorExtended (handler.go) at all, so both this op's and DeleteResourceDataSync's not-found path fell through to a 500 InternalServerError -- an existing test (TestDeleteResourceDataSync_Handler_NotFound) literally asserted the 500 as expected behavior under the name non_existent_sync_returns_500, now corrected to this service's uniform 400 convention. ErrResourceDataSyncExists (CreateResourceDataSync's duplicate-name case) had the same missing-mapping bug, fixed alongside since it's the same class of gap one line away. RE-VERIFIED via structfielddiff (gopherstack-enpq, 2026-08-21) -- validation now reuses a shared validateResourceDataSyncSource(*ResourceDataSyncSource) error helper (previously inlined here only; the doc comment referencing that name was stale until this pass), no wire-shape changes needed."}
  CreateResourceDataSync: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-enpq (2026-08-21, structfielddiff pass 6): S3Destination and SyncSource (api_op_CreateResourceDataSync.go:59-70 -- S3Destination \"is required if the SyncType value is SyncToDestination\", the default; SyncSource \"is required if the SyncType value is SyncFromSource\") had NO Go struct members at all -- every create silently dropped whichever config a real client sent, leaving a sync that could only ever be given a source/destination via a follow-up UpdateResourceDataSync call. Now both modeled (new ResourceDataSyncS3Destination type, models_activations.go), enforced per the op's own doc-comment conditional requirement (not a smithy-validator-enforced field -- structfielddiff correctly does not flag either as [required] since the requirement is prose-only; enforced here as ValidationException, this service's existing convention for doc-stated-but-not-struct-tagged requirements), and each S3Destination/SyncSource's own real required subfields (BucketName/Region/SyncFormat; SourceType/SourceRegions) validated via new validateResourceDataSyncS3Destination/validateResourceDataSyncSource helpers (the latter shared with UpdateResourceDataSync). Also removed CreateResourceDataSyncInputFull, a dead, unused duplicate-field type left over from an earlier incomplete attempt at this same fix (grepped for readers first -- none). DestinationDataSharing (S3Destination's own optional nested Organizations cross-account config) and AwsOrganizationsSource (SyncSource's) remain deliberately unmodeled, matching the same shallow-scalar convention this file already documents for SyncSource. TestCreateResourceDataSync_RequiredFields/TestResourceDataSync_CRUD (activations_test.go), hand-verified failing against unfixed code (undefined type, since the fix touches the request struct directly)."}
  DeleteResourceDataSync: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-enpq (2026-08-21, structfielddiff pass 6): re-verified via structfielddiff, SyncName (required) matches. SyncType (real, optional, disambiguates a sync when the same SyncName exists under two different SyncType values) is NOT modeled -- this backend's resourceDataSyncsStore keys purely by SyncName (store_setup.go's resourceDataSyncKeyFn), so two syncs can never coexist under the same name regardless of type in this backend's model; adding a SyncType match/mismatch check would be validating a state this backend structurally cannot reach, not a real gap in observable behavior. Disclosed, not fabricated -- see gaps."}
  StartChangeRequestExecution: {wire: fixed, errors: ok, state: fixed, persist: ok, note: "gopherstack-4ggy: Runbooks (a required StartChangeRequestExecutionInput member, api_op_StartChangeRequestExecution.go:37-51) was dropped entirely -- request only read the top-level DocumentName (the change template document) and built automation steps from IT directly, when the actual Automation runbook(s) to execute live in Runbooks[].DocumentName instead. Now required (each entry's own DocumentName required per validateRunbook, validators.go), steps built from Runbooks[0].DocumentName (this backend's AutomationExecution models one step list; real AWS runs each Runbook as its own workflow -- an accepted simplification, not attempted to fully multi-runbook this pass), and the full Runbooks list echoed back on AutomationExecution.Runbooks (new field, types.AutomationExecution.Runbooks, types.go:761/943) for both GetAutomationExecution and DescribeAutomationExecutions. Runbook itself models only DocumentName/DocumentVersion/MaxConcurrency/MaxErrors/Parameters -- TargetLocations/TargetMaps/TargetParameterName/Targets deliberately unmodeled, matching the same shallow-scalar simplification StartAutomationExecutionInput already makes for its own Targets/TargetLocations/TargetParameterName (pre-existing convention, not new scope)."}
  ListNodes: {wire: ok, errors: ok, state: ok, persist: n/a, note: "2026-08-13 (gopherstack-6uag): input was a literal struct{}, same surface pattern as ListNodesSummary's pre-fix bug (gopherstack-m53b) but a different case on inspection -- ListNodesInput (api_op_ListNodes.go:31-53) has no required members (Filters/MaxResults/NextToken/SyncName all optional), unlike ListNodesSummaryInput's required Aggregators, so this was never the required-field-ignored/fabricated-response-key stub class. It was still a real bug: the struct{} silently discarded all four real optional fields from every request, so Filters never filtered and MaxResults/NextToken never paginated. Fixed by giving ListNodesInput real fields, applying Filters via the shared filterNodes (extracted from ListNodesSummary's own fix, no behavior change there), and paginating via this service's established parseNextToken convention (50-item default, matching DescribeOpsItems). Reading the whole operation found a second, more severe bug: the real ListNodesOutput element (types.Node, types/types.go:4087-4106) is CaptureTime/Id/NodeType/Owner/Region, with PlatformType/AgentVersion nested three levels down under NodeType.Instance (types.InstanceInfo, types/types.go:2693-2747) -- this backend instead serialized NodeInfo directly under top-level InstanceId/PlatformType/AgentVersion/RegistrationDate keys, none of which exist on the real wire, and RegistrationDate doesn't correspond to any real field at all (renamed the wire-facing struct's field to CaptureTime, the real epoch-seconds member). New wire types Node/NodeType/NodeInstanceInfo/NodeOwnerInfo added; NodeInfo keeps its old field set as a purely internal domain struct, converted to Node by nodeToWire at response time. Owner is always nil: no account/OU tracking exists. Proven via TestFleetManager_ListNodes_FromActivations (rewritten to drive the real SDK client and assert the nested NodeType.Instance.PlatformType location instead of a raw top-level map key, which would have passed against the bug), new TestFleetManager_ListNodes_Filters, and TestEpochSecondsWireShape_Node (renamed from _NodeInfo) -- all three hand-verified to fail against the pre-fix code. RE-VERIFIED via structfielddiff (gopherstack-enpq, 2026-08-21, structfielddiff pass 6) -- ListNodesInput.Filters/MaxResults/NextToken/SyncName and Node.CaptureTime/Id/NodeType/Owner/Region all match exactly, no new fields on the real wire since. SyncName remains accept-and-echo (this backend has no multi-region resource-data-sync-scoped node index to filter against), consistent with this op's existing accept-and-echo convention for unbacked filter keys."}
  ListNodesSummary: {wire: ok, errors: ok, state: ok, persist: n/a, note: "gopherstack-m53b (required-member sweep pass 4): input was a literal struct{} (api_op_ListNodesSummary.go:31-62 shows Aggregators is a required []types.NodeAggregator, Filters/MaxResults/NextToken/SyncName optional) and the backend ignored its own parameter entirely, returning a fixed synthetic {\"NodeCount\": activationCount} regardless of what was requested — the fabricated \"NodeCount\" key does not exist on the real wire either (real Summary is []map[string]string with no fixed key schema). Op WAS reachable (JSON-RPC 1.1 dispatch keys off the X-Amz-Target header, not the input shape) — confirmed with the sdkshape script and by reading handler.go's ssmDispatchTable/jsonOp, so this was a backend-logic bug, not a routing bug. Fixed: Aggregators is now required (InvalidAggregatorException, one of this op's own declared exceptions per deserializeOpErrorListNodesSummary — not the generic ValidationException most other ssm ops use) and actually drives real per-attribute grouping (aggregateNodes in instances.go) over managed nodes derived from the activations store, with Filters applied (matchesNodeFilter) before grouping. This backend only tracks InstanceId/PlatformType/AgentVersion per node (see NodeInfo) — the other five NodeAttributeName/NodeFilterKey values (PlatformName/PlatformVersion/Region/ResourceType/SourceType/AvailabilityZone/...) have no backing state and are honestly left as \"\" rather than fabricated; nested NodeAggregator.Aggregators (multi-level grouping) are accepted on the wire but not applied. Proven via Test_SDKRoundTrip_ListNodesSummary_GroupsByAggregator/TestListNodesSummary_Filters/TestListNodesSummary_MissingAggregators (list_nodes_summary_test.go) and TestFleetManager_ListNodesSummary_NodeCount (activations_test.go, converted to drive the real SDK client) — all fail against the unfixed backend. TestStubOps_SimpleCalls's bare-{}-body manifest (maintenance_window_lifecycle_test.go) had ListNodesSummary removed per parity-principles.md's de-stub-hygiene rule, since an empty body is no longer valid input. RE-VERIFIED via structfielddiff (gopherstack-enpq, 2026-08-21, structfielddiff pass 6) -- no new fields, no changes needed. FIXED again (cursor-population sweep, 2026-08-29): MaxResults/NextToken were modeled on the wire but never read/populated -- now paginates via paginateSlice."}
families:
  filter_semantics: {status: fixed, note: "gopherstack-uox6 (value-semantics sweep, 2026-08-30): read every hand-rolled filter/matcher/comparison helper against its SDK doc comment (aws-sdk-go-v2/service/ssm@v1.73.4) for the class field-diff tools can't see (right field, wrong algorithm). 2 real bugs fixed: (1) ListDocuments' documentMatchesFilters had no case for TargetType/PlatformTypes -- see ListDocuments note above. (2) DescribeOpsItems' opsItemMatchesFilters ignored OpsItemFilter.Operator, always comparing for exact equality even on Title/Source, which api_op_DescribeOpsItems.go documents as also supporting Operator=Contains -- see ops-center note above. Confirmed CORRECT (read against doc comment, no bug): matchesActivationFilter (DescribeActivationsFilterKeys, multi-value slices.Contains, correct unknown-key accept-and-echo), cloudConnectorMatchesFilter (honors the documented FilterValues='NONE' special case for tenant-level connectors), matchesNodeFilter (ListNodes' NodeFilterOperatorType: all 3 enum members Equal/NotEqual/BeginWith correctly implemented, not just the default), matchesInstanceInformationFilter, sessionMatchesFilter/sessionStateMatchesFilter (correctly distinguishes SessionStatus from the coarse SessionState bucket -- a previously-fixed bug, re-verified not regressed), matchesAutomationExecutionFilter (DocumentNamePrefix correctly prefix- not exact-matched), matchesAssociationFilter, paramMatchesFilter/fieldMatchesFilterOption/paramMatchesPathFilter (Equals/BeginsWith/Contains options and Path's Recursive/OneLevel both correctly implemented per types.ParameterStringFilter's doc comment), patchMatchesFilters/patchBaselineMatchesFilters/patchGroupMappingMatchesFilters. One false-positive self-caught and reverted: DescribeAvailablePatchesInput.Filters is actually typed []types.PatchOrchestratorFilter (confirmed via api_op_DescribeAvailablePatches.go), not the wildcard-supporting types.PatchFilter (whose '*'-matches-all doc comment only applies to patch-baseline ApprovalRules, an unevaluated field elsewhere in this service) -- an initial fix assumed the wrong type and was reverted before landing. Not this bug class, not touched (field never read at all rather than read-and-misapplied, already disclosed elsewhere in this file): GetInventory/ListComplianceItems/ListCommands Filters, DescribeInstanceProperties' FiltersWithOperator, DescribeAssociationExecutionsInput's entirely-missing Filters member, DescribeMaintenanceWindowTargets/TasksInput's entirely-missing Filters member (already correctly disclosed in those ops' own notes above, re-verified accurate)."}
  tags: {status: ok, note: "gopherstack-enpq (2026-08-14): AddTagsToResource/RemoveTagsFromResource/ListTagsForResource re-verified via structfielddiff against api_op_*.go for all three -- Tag{Key,Value}/ResourceId/ResourceType/TagKeys all match exactly, zero real hits after filtering ResultMetadata noise. No changes needed."}
  resource-policies: {status: ok, note: "gopherstack-enpq (2026-08-14): 2 real bugs fixed, see PutResourcePolicy/DeleteResourcePolicy/GetResourcePolicies notes above -- PolicyId/PolicyHash update-in-place semantics were entirely unimplemented (every Put appended a duplicate), DeleteResourcePolicy's required PolicyHash concurrency check was entirely unimplemented (any caller could delete any policy, no ResourcePolicyConflictException path existed), and GetResourcePolicies had no pagination. Also fixed a dead/wrong error mapping: ErrResourcePolicyNotFound existed in errors.go with the wrong error code and no case in classifySSMErrorExtended, so it would have 500'd if anything had ever returned it (nothing did, until this pass's fix)."}
  service-settings: {status: ok, note: "gopherstack-enpq (2026-08-14): 2 real bugs fixed, see GetServiceSetting/UpdateServiceSetting/ResetServiceSetting notes above -- ARN and LastModifiedDate had no Go struct members at all. LastModifiedUser deliberately not modeled, see gaps."}
  compliance: {status: ok, note: "gopherstack-enpq (2026-08-14): 3 real bugs fixed across PutComplianceItems/ListComplianceItems, see notes above -- ExecutionSummary (required) and per-item Severity/Status (required) were never validated; ComplianceItem.Id/.ExecutionSummary had no Go struct members; ListComplianceItemsInput modeled a singular ResourceId/ResourceType wire key where the real members are plural ResourceIds/ResourceTypes lists, so a real client's resource filter silently never matched anything. ListComplianceSummaries/ListResourceComplianceSummaries re-verified, no changes needed there. Filters ([]ComplianceStringFilter, shared across all three List ops) and PutComplianceItemsInput.UploadType's PARTIAL-mode per-association merge semantics are real gaps, disclosed rather than rushed -- see gaps."}
  inventory: {status: ok, note: "gopherstack-enpq (2026-08-14): 2 real bugs fixed -- DeleteInventory's DryRun had no Go struct member at all, so a caller asking to preview a deletion got a real irreversible one instead (more permissive than AWS); ListInventoryEntries dropped CaptureTime/SchemaVersion from its response even though the matched item already carried both. PutInventory/GetInventory/GetInventorySchema/DescribeInventoryDeletions re-verified, no changes needed there. GetInventory's Aggregators/Filters/ResultAttributes, ListInventoryEntries' Filters, and GetInventorySchema's per-type Attributes are real gaps, disclosed rather than fabricated -- see gaps."}
  managed-instance: {status: ok, note: "gopherstack-enpq (2026-08-14): DeregisterManagedInstance/UpdateManagedInstanceRole re-verified via structfielddiff, both trivially match the real SDK exactly (no nested types, both already required-field-validated). No changes needed."}
  activations: {status: ok, note: "gopherstack-enpq (2026-08-14): CreateActivation/DeleteActivation re-verified via structfielddiff (DescribeActivations already covered by its own ops: row, gopherstack-a250). CreateActivationInput.RegistrationMetadata ([]types.RegistrationMetadataItem, optional tag-like metadata attached to the registered instance) not modeled -- low-value, disclosed rather than added speculatively, see gaps."}
  cloud-connectors: {status: ok, note: "gopherstack-enpq pass 7 (structfielddiff, 2026-08-21): RE-VERIFIED via structfielddiff for all 6 ops, the first time via this mechanical method rather than op-by-op reads. Genuinely clean -- zero new findings after checking every request/response field, the CloudConnectorFilterKey/ValidationFindingCode/ValidationFindingType/ValidationFindingScopeType enum values against types/enums.go, and the AzureConfiguration/CloudConnectorConfiguration union wire-wrapping directly against serializers.go/deserializers.go. — Prior-pass notes preserved: NEW (parity-3 phase-2) — aws-sdk-go-v2 bumped v1.69.5 to v1.71.0 (see sdk_module) added CreateCloudConnector/DeleteCloudConnector/GetCloudConnector/ListCloudConnectors/UpdateCloudConnector/ValidateCloudConnector (Azure-only third-party cloud environment connectors). Implemented as a real *store.Table[CloudConnector]-backed resource (services/ssm/cloud_connector.go): required-field validation (ConfigConnectorArn/DisplayName/RoleArn/Configuration.AzureConfiguration.{ApplicationId,TenantId}) on Create, ResourceNotFoundException (the SDK's generic not-found type — no CloudConnector-specific error exists) on Get/Delete/Update/Validate of an unknown ID, tag integration via the existing generic miscResourceTags fallback path (ResourceTypeForTagging enum confirms \"CloudConnector\" is a valid AddTagsToResource/ListTagsForResource resource type, and that path was already resource-type-agnostic), and full Snapshot/Restore persistence via the existing store.Registry generic mechanism (store_setup.go's getOrCreateTable/tableAccessorsByPrefix — no persistence.go changes needed). Wire shapes verified against aws-sdk-go-v2/service/ssm@v1.73.4's serializers.go/deserializers.go directly (not the SDK's own doc comments): CreatedAt/UpdatedAt are epoch-seconds JSON numbers, matching this package's existing UnixTimeFloat convention, NOT ISO8601 strings; Configuration is a one-member Azure-only union wire-wrapped by member name (\"AzureConfiguration\")."}

  resource-data-sync: {status: ok, note: "gopherstack-enpq (2026-08-21, structfielddiff pass 6): genuinely untouched by any prior audit pass (unlike most of this file's other families, no op-by-op history existed for this one at all). 1 real bug fixed: CreateResourceDataSync's S3Destination/SyncSource had no Go struct members at all -- see CreateResourceDataSync ops: note. DeleteResourceDataSync/ListResourceDataSync/UpdateResourceDataSync re-verified, one small addition (S3Destination now echoed on List, matching Create's fix). DeleteResourceDataSync.SyncType and S3Destination.DestinationDataSharing/SyncSource.AwsOrganizationsSource remain unmodeled -- see gaps."}
  nodes: {status: ok, note: "gopherstack-enpq (2026-08-21, structfielddiff pass 6): RE-VERIFIED via structfielddiff -- ListNodes/ListNodesSummary were already fully field-diffed and fixed op-by-op in prior passes (gopherstack-6uag, gopherstack-m53b); this pass independently re-confirms via the mechanical struct-field-diff method rather than trusting the prior op-by-op note, per this issue's own governing rule that a prior claim is not evidence. No new findings -- both ops' Go structs match the real wire exactly, including the previously-fixed Node/NodeType/NodeInstanceInfo/NodeOwnerInfo nesting. This is the one family this pass touched where the stronger method found nothing the weaker method had missed."}
  commands: {status: ok, note: "gopherstack-enpq (2026-08-22, doc-prose re-audit pass 11): SendCommand's own doc comment on InstanceIds says, verbatim, 'we recommend using Targets instead' for at-scale sends, and types.Command.Targets' doc comment says 'Targets is required if you don't provide one or more managed node IDs in the call' -- the same required-alternate-identifier shape as kinesis's StreamARN bug. Targets round-tripped (echoed back on Command/SendCommandOutput, previously typed as a raw []any) but the invocation-building loop in commands.go only ever ranged over InstanceIds -- a Targets-only caller, exactly AWS's documented pattern, got TargetCount 0 and zero CommandInvocations. Fixed: Targets retyped to a new CommandTarget{Key,Values} struct (matches types.Target; purely a type refinement of the same JSON shape, confirmed compatible with pkgs/persistence's TestSnapshotVersionGuard, no ssmSnapshotVersion bump needed) and a new mergeUniqueInstanceIDs(InstanceIds, commandTargetInstanceIDs(Targets)) union now drives both the invocation loop and the AWS-RunPatchBaseline apply loop, deduped so a node named in both stays single-invoked. commandTargetInstanceIDs treats every target's Values as literal node IDs regardless of Key, matching the same simplification associations.go's buildAssocExecTargets already makes for AssociationTarget -- no tag-based resolution infra exists in this backend and this keeps the convention consistent rather than inventing a second one. Proven by TestSendCommand_TargetsOnly_RealClient and TestSendCommand_InstanceIDsAndTargets_Dedup_RealClient (wire_field_fixes_test.go) via the real aws-sdk-go-v2 client; both hand-verified to fail against the pre-fix code (TargetCount 0 / missing third invocation) before the fix was restored byte-identical. CancelCommand/ListCommands/ListCommandInvocations/GetCommandInvocation re-checked for the same doc-prescribed-alternate pattern this pass and found clean (no Targets-shaped alternate on any of them). Two of pass 6's original 3 bugs already covered (ListCommands.InstanceId, CancelCommand.InstanceIds); this is a 4th, independently found bug in the same family via the different method. Everything pass 6 already disclosed as unmodeled (AlarmConfiguration/CloudWatchOutputConfig/NotificationConfig/CommandPlugins/DocumentHash/DocumentHashType/ResponseCode/Filters) re-confirmed still accurate -- see gaps."}
  parameter-store: {status: ok, note: "FIXED (parity-sweep-3, PutParameter): 15-level hierarchy limit (HierarchyLevelLimitExceededException, previously unenforced), labeled-oldest-version eviction guard (ParameterMaxVersionLimitExceeded, previously silently evicted labeled versions and leaked their parameterLabels entries forever), Intelligent-Tiering auto-upgrade-to-Advanced on >4KiB value or Policies attached (previously hard-rejected instead of auto-selecting Advanced, defeating the entire point of Intelligent-Tiering), Policies-require-Advanced-tier (previously any tier accepted policies). Tier value-size limits (4096 Standard / 8192 Advanced), AllowedPattern regex validation, SecureString KMS encrypt/decrypt round-trip via per-instance AES-256 key, parameter selector suffix (:version/:label) parsing were all already correct. FIXED phase-2 (2026-07-24) — NoChangeNotification/ExpirationNotification policies were stored and round-tripped but never evaluated; now a new janitor sweep (sweepParameterPolicyNotifications, parameter_policy_notifications.go) evaluates every parameter's Policies each tick and reports newly-due policies through an injectable ParameterPolicyNotifier, with per-policy-instance dedupe (never refires until the parameter is re-written, matching AWS's documented LastModifiedTime-reset semantics for NoChangeNotification) and cascade cleanup on delete (no ghost dedupe rows). The EventBridge-side adapter is implemented for real (services/eventbridge/ssm_integration.go, publishes source=\"aws.ssm\"/detail-type=\"Parameter Store Policy Action\"/detail={\"parameter-name\",\"policy-type\"} — confirmed via sysman-paramstore-cwe.html) and proven by TestNotifyParameterPolicyAction using an EventBridge Archive with a matching EventPattern as an independent wire-shape observer. Only the cli.go line wiring InMemoryBackend.SetParameterPolicyNotifier(ebBackend) remains — see Notes. RE-AUDITED via structfielddiff (gopherstack-enpq, 2026-08-21, structfielddiff pass 6), the first time this family was checked by this mechanical method rather than op-by-op reads -- see each op's own ops: note above for the 3 real bugs found this pass (PutParameter.Tags, and the Get*-family's fabricated-metadata-fields/DescribeParameters' wrong-Policies-type headline finding). All 10 parameter-store ops are now individually re-verified via structfielddiff; DeleteParameter/DeleteParameters/UnlabelParameterVersion were already trivially correct and needed no change."}
  documents: {status: fixed, note: "gopherstack-enpq pass 10 (2026-08-21): all 12 ops now genuinely fixed, closing out the last family this campaign's method had left partial. DeleteDocument/DescribeDocumentPermission/ModifyDocumentPermission -- disclosed rather than fixed since pass 7 pending a documentPermissionsStore reshape -- are now settled (see each op's own note): DeleteDocument honors DocumentVersion/VersionName scoping and rejects deleting a still-shared document; ModifyDocumentPermission tracks a per-account SharedDocumentVersion pin via the new additive documentSharedVersionsStore; DescribeDocumentPermission paginates and reports real AccountSharingInfoList entries. The other 9 ops (CreateDocument/UpdateDocument/DescribeDocument/GetDocument/ListDocuments/ListDocumentVersions/ListDocumentMetadataHistory/UpdateDocumentMetadata/UpdateDocumentDefaultVersion) were already fixed in prior passes (7). One remaining disclosed gap survives verification: VersionName (modeled on DocumentVersionInfo only, never populated, and entirely absent from CreateDocumentInput/UpdateDocumentInput/DocumentDescription/GetDocumentOutput) still needs a full selector redesign mirroring resolveDocumentVersionSelector's $LATEST/$DEFAULT handling -- DeleteDocument's own VersionName parameter is parsed but can never resolve a match for the same reason, so a VersionName-scoped delete request is honestly rejected as not-found rather than silently mismatched against the numeric DocumentVersion namespace. — Prior-pass notes preserved: CreateDocument/UpdateDocument/DescribeDocument content-leak and $DEFAULT/$LATEST conflation (bd gopherstack-1hg, closed) fixed; the version-cap eviction (maxDocumentVersionCap=1000) never evicts the DefaultVersion-pinned entry."}
  command-execution: {status: ok, note: "no goroutines/timers in command_exec.go or automation_exec.go — command progression is driven synchronously plus the single ctx-cancel-aware janitor sweep (janitor.go), not per-command background workers. Nothing to leak."}
  automation-executions: {status: fixed, note: "gopherstack-gt9o: modeled AutomationExecution/AutomationExecutionMetadata/StepExecution's WarningMessage *string field (confirmed at aws-sdk-go-v2/service/ssm@v1.73.4 types/types.go:803,969,6079), but deliberately left it permanently unset (json omitempty). Reasoning: WarningMessage is output-only, set by real SSM's engine for a non-critical issue detected mid-run; this emulator's automation run (automation_exec.go completeAutomationLocked) unconditionally drives every step to Success with no partial-failure/degraded/timeout path — automationStatusFailed is declared in store.go but never assigned anywhere. There is no genuine condition to derive a warning from, so inventing one would be a fabricated string a real client could surface to an operator. Same precedent as apigatewayv2's failOnWarnings (validated but a documented no-op). Test coverage (automations_test.go TestAutomationExecution_WarningMessageAbsentFromWire) asserts the raw response body genuinely omits the key (not merely empty-string) across GetAutomationExecution/DescribeAutomationExecutions/DescribeAutomationStepExecutions; neuter-tested by dropping omitempty, confirming all three subtests fail, then restoring. FIXED (gopherstack-a250): DescribeAutomationExecutions' input was a literal struct{}; real DescribeAutomationExecutionsInput (api_op_DescribeAutomationExecutions.go) has an optional Filters member (plus MaxResults/NextToken), all discarded. Now filters on ExecutionId/ExecutionStatus (exact match) and DocumentNamePrefix (prefix match, the three attributes AutomationExecution actually tracks) and paginates. TestDescribeAutomationExecutions_FilterAndPagination, hand-verified failing against unfixed code. STRUCTFIELDDIFF PASS 8 (gopherstack-enpq, 2026-08-21), all 10 ops re-diffed against ssm@v1.73.4: 4 real bugs fixed, one of them a wrong-key bug at the same strength as sns's XMLOriginationPhone (reverting fails to compile). (1) AutomationExecution's Automation-subtype field was wire key 'ExecutionType' set to 'Standard'/'ChangeRequest' — that key belongs to a completely different, unrelated type (types.ComplianceExecutionSummary, deserializers.go:27630/models_inventory.go's own correct ExecutionType field) and does not exist anywhere on AutomationExecution/AutomationExecutionMetadata at all; the real member is AutomationSubtype (deserializers.go:25863, types.go:874), whose only real value is ChangeRequest, omitted for standard executions. Fixed, plus added AutomationExecution.MaxConcurrency/MaxErrors (types.go:727,730,917,920) which StartAutomationExecutionInput already accepted and silently discarded (parsed-then-ignored, same class as pass 6's CancelCommand.InstanceIds) — now round-trip through GetAutomationExecution. (2) StopAutomationExecution and SendAutomationSignal's StopStep case both set the fabricated status string 'Stopped', which is not a valid AutomationExecutionStatus enum value at all (types/enums.go has Cancelled, not Stopped) — a real client switching on the enum would never match it. Fixed: StopAutomationExecutionInput.Type (Cancel/Complete, types.StopType, previously accepted-and-ignored) now selects Cancelled vs Success. (3) ErrAutomationExecutionNotFound was defined and used by GetAutomationExecution's not-found path but never classified in classifySSMErrorExtended (handler.go) — every not-found GetAutomationExecution (and, pre-fix, every attempt to reach the same class of error from Stop/SendAutomationSignal) fell through to the default case and returned 500 InternalServerError instead of 400 AutomationExecutionNotFoundException; TestGetAutomationExecution_Handler_NotFound explicitly asserted 500 as correct (fixed, see below). Now StopAutomationExecution/SendAutomationSignal also return this error for an unknown execution ID instead of silently no-op succeeding. STUB-OP LEAD: 6 of this family's ops were on TestStubOps_SimpleCalls's bare-{}-body list and all 6 read nothing — GetCalendarState (CalendarNames required), DescribeAutomationStepExecutions/StopAutomationExecution (AutomationExecutionId required), GetExecutionPreview (ExecutionPreviewId required), SendAutomationSignal (AutomationExecutionId+SignalType required), StartAutomationExecution/StartExecutionPreview (DocumentName required) — none enforced; all now validate and reject with ValidationException. GetCalendarState's real output also carries AtTime (echoing the input or defaulting to now), missing entirely; added. DescribeAutomationExecutions correctly has no required fields and stays on the stub list. SendAutomationSignal's Payload (real, required for StartStep/StopStep/Resume to name the target step) is now modeled as a Go field but not consulted — this backend has no per-step Waiting/InProgress state for a signal to target (every step is driven straight to Success), same simplification already disclosed for WarningMessage."}
  sessions: {status: ok, note: "gopherstack-enpq pass 7 (structfielddiff, 2026-08-21): RE-VERIFIED via structfielddiff, the first time this family was checked by this mechanical method rather than op-by-op reads. 1 real bug: DescribeSessions marshalled the internal Session record straight to the wire, which carries Parameters/StreamUrl/TokenValue -- three fields types.Session (aws-sdk-go-v2/service/ssm@v1.73.4) does not declare at all (they're ephemeral values StartSession/ResumeSession return, never echoed back by DescribeSessions). Same reused-domain-struct class as pass 6's GetParameter fix. Fixed via a new SessionOutput projection type; TestSession_Parameters_RoundTrip (which asserted the fabricated Parameters field's presence) rewritten to assert its absence instead. All other ops (GetAccessToken/GetConnectionStatus/ResumeSession/StartAccessRequest/StartSession/TerminateSession) re-confirmed exact matches against the real SDK, zero findings. — Prior-pass notes preserved: FULLY RE-VERIFIED and FIXED (parity-sweep-3, previously deferred) — see the per-op notes above (StartSession/DescribeSessions/GetConnectionStatus/GetAccessToken/StartAccessRequest) for the 6 real bugs found and fixed then: invented StartSessionInput fields, State/Status enum confusion, missing Filters/pagination, wrong ConnectionStatus casing, and GetAccessToken/StartAccessRequest being non-functional stubs. TerminateSession/ResumeSession/evictExcessTerminatedSessionsLocked were already correct. New AccessRequest resource (services/ssm/models_sessions.go, sessions.go) is a real *store.Table[AccessRequest]-backed resource with full Snapshot/Restore persistence via the existing store_setup.go mechanism."}
  patch-baselines: {status: fixed, note: "FULLY RE-VERIFIED and FIXED (parity-sweep-3, split out of the previously-deferred 'patch-maintenance-associations-inventory' family) — see CreatePatchBaseline/UpdatePatchBaseline/GetPatchBaseline notes above. FIXED phase-2 — ApprovedPatchesEnableNonSecurity bool->*bool (see CreatePatchBaseline/UpdatePatchBaseline notes and Notes section). STRUCTFIELDDIFF PASS 9 (gopherstack-enpq, 2026-08-21) — this family's earlier 'confirmed already-correct' claim above did not hold up under a mechanical field diff; all 16 ops re-diffed against ssm@v1.73.4, 6 real bugs fixed plus the campaign's stub-op lead. (1) PatchStatus.ApprovalDate (DescribeEffectivePatchesForPatchBaseline) was a plain string carrying an RFC3339 timestamp; the real member is a JSON number, epoch seconds (deserializers.go awsAwsjson11_deserializeDocumentPatchStatus, case \"ApprovalDate\": ParseEpochSeconds(f64)) — a real client failed to unmarshal the field at all for any baseline with an explicitly-approved patch. Fixed: type changed to float64, matching this file's CreatedDate/ModifiedDate convention. (2) Patch.State had no wire representation in types.Patch at all (confirmed: AdvisoryIds/Arch/BugzillaIds/CVEIds/Classification/ContentUrl/Description/Epoch/Id/KbNumber/Language/MsrcNumber/MsrcSeverity/Name/Product/ProductFamily/Release/ReleaseDate/Repository/Severity/Title/Vendor/Version, no State) yet the built-in catalogue (defaultPatchCatalog) set it on every seeded entry, leaking it onto the wire for DescribeAvailablePatches and DescribeEffectivePatchesForPatchBaseline; the field was also dead internally (patchComplianceFromEffective computes its own PatchComplianceData.State, never reading Patch.State) — removed entirely. (3) effectivePatchesForBaseline read b.availablePatches[region] directly instead of the lazy-seeding availablePatchesFor helper DescribeAvailablePatches itself uses, so DescribeEffectivePatchesForPatchBaseline's catalogue-derived entries silently depended on whether DescribeAvailablePatches (or applyPatchBaselineOperation, which pre-seeds as a workaround) had already run in that region — fixed to always call availablePatchesFor, and the op's lock upgraded RLock->Lock to match (lazy seeding writes to the map). (4) effectivePatchesForBaseline used the fabricated PatchDeploymentStatus value \"AVAILABLE\" (real enum, types/enums.go: APPROVED/PENDING_APPROVAL/EXPLICIT_APPROVED/EXPLICIT_REJECTED, no AVAILABLE) for catalogue patches with no explicit decision — fixed to PENDING_APPROVAL. RejectedPatches entries were also silently excluded from the effective set entirely instead of appearing with EXPLICIT_REJECTED status — fixed to synthesize an entry per rejected patch, same as the pre-existing ApprovedPatches convention (patchComplianceFromEffective, a sibling instances-family function that also consumes this output, was given a matching skip for EXPLICIT_REJECTED so its own pre-existing Missing/Installed semantics for a sibling family are unchanged). (5) DescribeAvailablePatchesInput.Filters was accepted but never consulted (parsed-then-ignored) — fixed: honors PRODUCT/NAME/SEVERITY/CLASSIFICATION (real keys per api_op_DescribeAvailablePatches.go's doc comment, the ones backed by fields this emulator's Patch actually models); MaxResults/NextToken pagination added to match this family's other list ops. (6) DescribePatchGroupsInput had no Filters member at all (real op has one, api_op_DescribePatchGroups.go) — added and wired, honoring the same NAME_PREFIX/OPERATING_SYSTEM keys DescribePatchBaselines already supports. STUB-OP LEAD: 8 of this family's 16 ops were on TestStubOps_SimpleCalls's bare-{}-body list; DescribeEffectivePatchesForPatchBaseline (BaselineId)/DescribePatchGroupState (PatchGroup)/DescribePatchProperties (OperatingSystem+Property)/GetDeployablePatchSnapshotForInstance (InstanceId+SnapshotId)/GetPatchBaselineForPatchGroup (PatchGroup)/RegisterDefaultPatchBaseline (BaselineId) all read nothing and are now validated; DescribeAvailablePatches/DescribePatchGroups correctly stay on the list (their real inputs are entirely optional). Beyond the stub list, DeregisterPatchBaselineForPatchGroup (BaselineId+PatchGroup) and RegisterPatchBaselineForPatchGroup (BaselineId+PatchGroup) were also entirely unvalidated despite both being required on the real ops — fixed; an existing table test (TestDeregisterPatchBaselineForPatchGroup_TableDriven) explicitly asserted an empty BaselineId as a 200 success, a ratified defect now corrected to expect ValidationException. TestDescribeEffectivePatches_FromApprovedAndCatalog also ratified the order-dependence bug in (3) by asserting exactly 2 EffectivePatches (only the explicit approvals, no catalogue) — corrected to exercise both approved and rejected patches against the always-seeded catalogue. Disclosed rather than fixed, see gaps: DescribePatchProperties' data source and Property/OS filtering (its own separate, pre-existing functional bug, independent of the stub-list fix); UpdatePatchBaselineInput.Replace; CreatePatchBaselineInput.ClientToken; GetDeployablePatchSnapshotForInstanceInput.BaselineOverride/UseS3DualStackEndpoint; DescribePatchGroupStateOutput's 6 missing *int32 security-update-specific counters; DescribeAvailablePatches' PATCH_ID/MSRC_SEVERITY/PRODUCT_FAMILY/PATCH_SET filter keys (real, no backing Go field)."}
  maintenance-windows: {status: fixed, note: "FULLY RE-VERIFIED and FIXED this pass (split out of the previously-deferred combined family) — see RegisterTaskWithMaintenanceWindow/UpdateMaintenanceWindowTask/CreateMaintenanceWindow/UpdateMaintenanceWindow and the DescribeMaintenanceWindowExecution*/GetMaintenanceWindowExecution* epoch-seconds notes above. RegisterTargetWithMaintenanceWindow/DeregisterTargetFromMaintenanceWindow/UpdateMaintenanceWindowTarget/DeregisterTaskFromMaintenanceWindow/DescribeMaintenanceWindows/DescribeMaintenanceWindowTargets/DescribeMaintenanceWindowTasks/DescribeMaintenanceWindowsForTarget/DescribeMaintenanceWindowSchedule/CancelMaintenanceWindowExecution/DeleteMaintenanceWindow re-diffed and confirmed already-correct. STRUCTFIELDDIFF PASS 8 (gopherstack-enpq, 2026-08-21), all 23 ops re-diffed against ssm@v1.73.4: 4 real wrong-wire-key bugs and 1 missing-fields bug fixed, plus the largest stub-op-lead haul of the campaign. WRONG KEYS (reverting each fails a real-SDK-client test, same strength of proof as sns's XMLOriginationPhone): (1) RegisterTargetWithMaintenanceWindowInput/MaintenanceWindowTarget/UpdateMaintenanceWindowTarget's OwnerInfo member was wire key \"OwnerInfo\"; the real key (serializers.go awsAwsjson11_serializeOpRegisterTargetWithMaintenanceWindow, and confirmed identically on the other two) is \"OwnerInformation\" -- a real client's owner info was silently dropped everywhere this field appears. (2) GetMaintenanceWindowExecutionTaskInput and GetMaintenanceWindowExecutionTaskInvocationInput both modeled TaskExecutionID as wire key \"TaskExecutionId\"; the real request member on both ops is \"TaskId\" (confirmed against both serializers directly) -- a real client's TaskId was silently dropped on every call to either op, which combined with this pass's new required-field checks would have newly broken every legitimate caller had it shipped unfixed (caught before landing). (3) GetMaintenanceWindowExecutionTaskOutput's task-type member is real wire key \"Type\" (deserializers.go awsAwsjson11_deserializeOpDocumentGetMaintenanceWindowExecutionTaskOutput, case \"Type\"), not \"TaskType\" as its sibling MaintenanceWindowExecutionTaskIdentity (DescribeMaintenanceWindowExecutionTasks) genuinely does use -- an AWS API inconsistency confirmed by reading both deserializer functions directly. (4) The shared MaintenanceWindowTask type (DescribeMaintenanceWindowTasks) also uses \"Type\", but GetMaintenanceWindowTaskOutput -- a distinct real response shape describing the same concept -- uses \"TaskType\" instead; gopherstack previously modeled both ops with one shared Go type and one wire key, which could only ever be correct for one of the two. Fixed by splitting GetMaintenanceWindowTaskOutput into its own projection (maintenanceWindowTaskToGetOutput) rather than embedding MaintenanceWindowTask. MISSING FIELDS: MaintenanceWindowIdentity (DescribeMaintenanceWindows/DescribeMaintenanceWindowsForTarget) was missing ScheduleTimezone/StartDate/EndDate/ScheduleOffset/NextExecutionTime entirely (real types.MaintenanceWindowIdentity, types.go:3706) -- fixed via a shared mwToIdentity projection; NextExecutionTime is synthesized via the same fixed mwExecutionScheduleHours-from-now heuristic DescribeMaintenanceWindowSchedule already used (no real cron/rate evaluator exists), also added to GetMaintenanceWindowOutput/UpdateMaintenanceWindowOutput. GetMaintenanceWindowExecutionTaskOutput was also missing ServiceRole and GetMaintenanceWindowExecutionTaskInvocationOutput was missing OwnerInformation (now sourced from the matched target's already-tracked OwnerInfo), both real members with no Go field at all. STUB-OP LEAD (the largest single haul this campaign has found in one family): 11 of this family's ops were on TestStubOps_SimpleCalls's bare-{}-body list and all 11 read nothing -- every one fabricated a synthetic \"Succeeded\" execution/task/invocation record even for a body missing every field, rather than rejecting per the real op's required members (DescribeMaintenanceWindowExecutions: WindowId; DescribeMaintenanceWindowExecutionTasks: WindowExecutionId; DescribeMaintenanceWindowExecutionTaskInvocations: WindowExecutionId+TaskId; DescribeMaintenanceWindowTargets/-Tasks: WindowId; DescribeMaintenanceWindowsForTarget: ResourceType+Targets; GetMaintenanceWindowExecution: WindowExecutionId; GetMaintenanceWindowExecutionTask: WindowExecutionId+TaskExecutionId; GetMaintenanceWindowExecutionTaskInvocation: WindowExecutionId+TaskExecutionId+InvocationId; GetMaintenanceWindowTask: WindowId+WindowTaskId). A 12th op, CancelMaintenanceWindowExecution, was not on that list but had the identical defect (WindowExecutionId silently optional) -- fixed too. CreateMaintenanceWindow was also missing a required-field check for Schedule. DescribeMaintenanceWindowSchedule correctly has no required fields and stays on the stub list."}
  state-manager-associations: {status: fixed, note: "SPOT-CHECKED (parity-sweep-3, split out of the previously-deferred combined family) — AssociationExecution.ExecutionDate epoch-seconds bug fixed (DescribeAssociationExecutions). FULLY FIELD-DIFFED phase-2 (bd gopherstack-ouvq, closed) — CreateAssociationInput/UpdateAssociationInput/CreateAssociationBatchRequestEntry were missing ApplyOnlyAtCronInterval/ComplianceSeverity/MaxConcurrency/MaxErrors/OutputLocation/ScheduleExpression/SyncCompliance/CalendarNames/AssociationDispatchAssumeRole/AutomationTargetParameterName/Duration, confirmed against api_op_CreateAssociation.go/api_op_UpdateAssociation.go/types.CreateAssociationBatchRequestEntry; all 11 now round-trip through Create/CreateBatch/Update and are covered by wire-shape-asserting tests (associations_test.go). DeleteAssociation/DescribeAssociation/UpdateAssociationStatus/ListAssociationVersions/StartAssociationsOnce/DescribeAssociationExecutionTargets re-confirmed already-correct, no changes needed. CORRECTION (gopherstack-a250): ListAssociations WAS wrong — input was a literal struct{}; real ListAssociationsInput (api_op_ListAssociations.go) has optional AssociationFilterList/MaxResults/NextToken, all discarded, and the response never carried NextToken either (a dead, unused ListAssociationsOutputFull type already had the right shape). Now filters on InstanceId/Name/AssociationId/AssociationName/AssociationStatusName (the attributes Association actually tracks) and paginates; backend return type switched to ListAssociationsOutputFull. TestListAssociations_FilterAndPagination, hand-verified failing against unfixed code. STRUCTFIELDDIFF PASS 8 (gopherstack-enpq, 2026-08-21), all 11 ops re-diffed against ssm@v1.73.4: 2 real bugs fixed. (1) UpdateAssociationStatusInput.AssociationStatus (AssociationStatusValue) modeled a fabricated 'ExecutionSummary' member that appears nowhere in the real types.AssociationStatus wire shape (serializers.go awsAwsjson11_serializeDocumentAssociationStatus only emits AdditionalInfo/Date/Message/Name) and was missing the two other required members, Date and Message; fixed, plus required-field validation (previously any AssociationStatus, including one missing Date/Message, was silently accepted). (2) Association (the shared domain struct returned by Create/CreateBatch/Update/UpdateAssociationStatus/Describe/List) had no Go member at all for Status or AssociationVersion, both present on every real AssociationDescription/ListAssociations response (deserializers.go awsAwsjson11_deserializeDocumentAssociationDescription cases 'Status'/'AssociationVersion') — UpdateAssociationStatus recorded the new status into Overview.Status only, so a real client reading resp.AssociationDescription.Status ever saw nil regardless of what UpdateAssociationStatus was called with; fixed via new AssociationStatusInfo type and AssociationVersion:\"1\" on create. STUB-OP LEAD: 4 of this family's ops were on TestStubOps_SimpleCalls's bare-{}-body list and all 4 read nothing — DescribeAssociationExecutionTargets, DescribeAssociationExecutions and ListAssociationVersions all require AssociationId (api_op_*.go, all mark it required) and StartAssociationsOnce requires non-empty AssociationIds, none enforced; DescribeAssociationExecutionTargets's own table test asserted the empty-AssociationId 200 as correct before this fix (removed, was a defect-ratifying test). ListAssociations correctly has no required fields and stays on the stub list. Real aws-sdk-go-v2 client itself validates AssociationStatus.Date/Message client-side and refuses to send a request missing them (confirmed the hard way — see TestUpdateAssociationStatus_RequiresDateAndMessage_HTTP), so that one fix's rejection path is proven over raw HTTP rather than through ssmsdk.Client. All fixes hand-reverted (both source files) and confirmed to fail to compile against the unfixed types before restoring byte-identical."}
  ops-center: {status: fixed, note: "SPOT-CHECKED (parity-sweep-3, split out of the previously-deferred combined family) — Priority confirmed missing and fixed. FULLY FIELD-DIFFED phase-2 (bd gopherstack-iq4m, closed) — CreateOpsItemInput/UpdateOpsItemInput were missing AccountId/ActualStartTime/ActualEndTime/Notifications/PlannedStartTime/PlannedEndTime/RelatedOpsItems (mostly Change-Manager /aws/changerequest-oriented), confirmed against api_op_CreateOpsItem.go/api_op_UpdateOpsItem.go; all 7 now round-trip and are covered by wire-shape-asserting tests (ops_items_test.go). UpdateOpsItemInput.OperationalDataToDelete (confirmed present but outside the bd issue's field list) deliberately left out of scope, documented in models_ops_items.go. GetOpsItem/DeleteOpsItem/DescribeOpsItems (filters+pagination)/AssociateOpsItemRelatedItem/DisassociateOpsItemRelatedItem/ListOpsItemRelatedItems/ListOpsItemEvents/CreateOpsMetadata/GetOpsMetadata/DeleteOpsMetadata re-confirmed already-correct, no changes needed. CORRECTION (gopherstack-7rq1): UpdateOpsMetadata was NOT actually correct -- UpdateOpsMetadataInput's Metadata field carried json tag \"Metadata\", but the real UpdateOpsMetadataRequest member (ssm/2014-11-06/service-2.json) is \"MetadataToUpdate\" (CreateOpsMetadataRequest genuinely does use \"Metadata\", which is presumably how this got missed). A real client's update payload was silently dropped by json.Unmarshal every time, making UpdateOpsMetadata a complete no-op; the existing test asserting HTTP 200 with a body keyed \"Metadata\" passed despite this. Fixed the json tag; TestOpsMetadata_FullCRUD's Update step now sends the real wire key and asserts the update actually lands. CORRECTION (gopherstack-a250): ListOpsMetadata was NOT actually correct either -- input was a literal struct{}; real ListOpsMetadataInput (api_op_ListOpsMetadata.go) has optional Filters/MaxResults/NextToken, all discarded. Now filters by Key==\"ResourceId\" (the only OpsMetadata attribute with real backing state; other keys accept-and-echo) and paginates. TestListOpsMetadata_FilterAndPagination, hand-verified failing against unfixed code. GetOpsSummary's Aggregators/Filters/MaxResults/NextToken/ResultAttributes/SyncName (also a literal struct{}) deliberately left unwired: this backend's GetOpsSummary always returns one fixed AWS:OpsItem/Count entity, not a queryable multi-type OpsData dataset these members could honestly filter or aggregate over -- documented in models_ops_items.go rather than fabricating query semantics. STRUCTFIELDDIFF PASS 8 (gopherstack-enpq, 2026-08-21), all 15 ops re-diffed against ssm@v1.73.4: 5 real bugs fixed. (1) GetOpsItemOutput/DescribeOpsItems' OpsItem marshalled the internal OpsItem record straight to the wire, fabricating AccountId -- real types.OpsItem/types.OpsItemSummary have no AccountId member at all (it exists only on CreateOpsItemInput); UpdateOpsItemInput also modeled AccountId (again with no such member on the real api_op_UpdateOpsItem.go) and applied it, letting a caller silently rewrite an OpsItem's AccountId through an op the real SDK cannot even express. Fixed via a new OpsItemOutput projection type (GetOpsItem) and removing AccountId from UpdateOpsItemInput/applyOpsItemChangeManagerUpdates, whose own doc comment falsely claimed AccountId as one of UpdateOpsItemInput's real members -- also corrected. Added the real OpsItemArn member UpdateOpsItemInput does have (previously entirely missing) and Version (real types.OpsItem member, increments on every edit; had no Go member at all). (2) GetOpsMetadataOutput embedded the full OpsMetadata type, fabricating OpsMetadataArn/CreationDate/LastModifiedDate -- the real op's output (api_op_GetOpsMetadata.go) is only Metadata/NextToken/ResourceId, a narrower and different shape than the OpsMetadata type ListOpsMetadata returns; fixed via a dedicated GetOpsMetadataOutput type. (3) OpsItemSummary (DescribeOpsItems) was missing OperationalData/PlannedEndTime/PlannedStartTime/ActualEndTime/ActualStartTime/OpsItemType/Category/Severity/LastModifiedTime -- all real types.OpsItemSummary members with no Go field at all; added and wired from the stored OpsItem. (4) CreateOpsItemInput.Description had no required-field validation at all despite being required on the real op (api_op_CreateOpsItem.go marks it 'This member is required.', discovered via a real-client test that the SDK itself refused to send without it) -- fixed, ~15 existing test call sites updated to supply it. (5) AssociateOpsItemRelatedItem/DisassociateOpsItemRelatedItem's required fields (AssociationType/ResourceType/ResourceUri; OpsItemId/AssociationId respectively, all marked required on api_op_AssociateOpsItemRelatedItem.go/api_op_DisassociateOpsItemRelatedItem.go) were entirely unvalidated -- fixed. STUB-OP LEAD: 1 of this family's ops was on TestStubOps_SimpleCalls's bare-{}-body list, DisassociateOpsItemRelatedItem, and it read nothing (empty OpsItemId silently returned 200); now validates and rejects with ValidationException. FIXED (gopherstack-uox6, value-semantics sweep, 2026-08-30): opsItemMatchesFilters ignored OpsItemFilter.Operator entirely and always compared for exact equality, even though api_op_DescribeOpsItems.go's doc comment documents Title and Source as also supporting Operator=Contains (substring) -- a real client asking for a Contains match on either key got either nothing (values that happen to equal the substring) or a silent exact-match instead. Now Operator=\"Contains\" does a substring compare on the two keys that support it; Status stays Equals-only per the same doc comment. Disclosed rather than fixed: OpsItemFilter only honors Status/Title/Source of the ~35 real DescribeOpsItems filter keys (no generic filter-operator engine, same disclosed-gap class as GetInventory/ListComplianceItems); ListOpsItemRelatedItemsInput/ListOpsItemEventsInput.MaxResults are *int64 where the real type is *int32 (zero practical wire impact, left as-is given the ripple through existing bounds-check tests using an int64 helper); OpsItemSummary/OpsMetadata's CreatedBy/LastModifiedBy/LastModifiedUser remain unmodeled (no caller-identity infra, same class as ServiceSetting.LastModifiedUser)."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - "gopherstack-e91b (2026-09-06): RejectedPatchesAction (BLOCK/ALLOW_AS_DEPENDENCY) is now
    validated but not fully semantically evaluated -- the two values only diverge in real AWS via
    package-dependency installation and INSTALLED_REJECTED history for a patch installed before
    it was rejected, and this backend's synthetic Patch catalogue has no dependency graph while
    InstancePatchState/PatchComplianceData are recomputed fresh per AWS-RunPatchBaseline run
    rather than tracked incrementally, so there is no pre-existing-install history to distinguish
    the two against. Structural -- needs a package-dependency feature and incremental (not
    recomputed) instance patch history, both real features of their own. PatchBaseline's
    GlobalFilters (restricts which catalogue patches a baseline covers at all) also remains
    round-trip-only, same class as the pre-existing ApprovalRules/RejectedPatches gap this issue
    fixed one layer of. See PARITY.md's gopherstack-e91b section above for what was fixed."
  - "2026-08-30 (region-isolation sweep, fix/wrapper-key-sweep-rds-cloudwatch-sqs-sns): checked
    the cloudwatchlogs/memorydb bug class (identifier/storage key built from the backend's fixed
    default region instead of the request's; a read that discards ctx and scans every region) --
    CONFIRMED CLEAN, not a gap. Traced getRegion(ctx) (store.go) -- sourced from
    httputils.ExtractRegionFromRequest at handler.go's request entry point, defaultRegion used
    ONLY as getRegion's ctx-missing fallback and at bootstrap/seed/restore call sites
    (registerDefaultDocuments, janitor-absent-context paths), never in a live request path -- back
    through every *Store(region) accessor's call site across the package (parametersStore,
    documentsStore, sessionsStore, etc., ~230 backend methods total): every one derives its
    `region` local from getRegion(ctx), no exceptions, no backend method discards ctx (`_
    context.Context`) despite touching a per-region map. The one cross-region background scan
    (collectDueParameterPolicyNotificationsLocked, parameter_policy_notifications.go) legitimately
    ranges every region -- it is a scheduled sweep like janitor.go's, not a client-facing read, and
    tags each result with its own region rather than conflating them. This package already carries
    its own proof test for this exact class (isolation_test.go's TestSSMRegionIsolation: same-named
    Parameter in two regions via regionContextKey, asserts each region's Get/Delete only affects
    its own). No fix needed."
  - "gopherstack-enpq (2026-08-14): ServiceSetting.LastModifiedUser (real member: \"The ARN of the last modified user\", populated only when the setting value was overwritten) is not modeled -- this emulator has no caller-identity/SigV4-principal tracking to derive a real IAM ARN from, same disclosed-gap class as sns's ConfirmSubscriptionInput.AuthenticateOnUnsubscribe (gopherstack-cu4g)."
  - "gopherstack-enpq (2026-08-14): PutComplianceItemsInput.UploadType (COMPLETE/PARTIAL) is accepted but not evaluated -- real AWS's PARTIAL mode only overwrites one association's compliance data (requiring SyncCompliance=MANUAL) while leaving other associations for the same resource untouched; this backend always applies COMPLETE semantics (replaces every item for ResourceId). Needs compliance storage reshaped to key by association, not just ResourceId -- disclosed rather than rushed."
  - "gopherstack-enpq (2026-08-14): GetInventory's Aggregators/Filters/ResultAttributes, ListInventoryEntries' Filters, and ListComplianceItems/ListComplianceSummaries/ListResourceComplianceSummaries' Filters (all InventoryFilter/ComplianceStringFilter with a shared Key/Values/QueryOperatorType shape: Equal/NotEqual/BeginWith/GreaterThan/LessThan/Exists) are entirely unmodeled -- these backends return everything and let the caller filter client-side. Implementing this needs a generic filter-operator evaluator shared across ~5 ops; a real feature, not a one-line fix, disclosed rather than half-built."
  - "gopherstack-enpq (2026-08-14): GetInventorySchema's real InventoryItemSchema.Attributes ([]InventoryItemAttribute, required, each with Name/DataType) is not modeled -- gopherstack's static built-in schema catalog only tracks TypeName/Version. AWS has not published the exact per-type attribute list in the SDK source (only in web docs this agent did not fetch), so fabricating attribute names for each of the 13 built-in types would be inventing wire content rather than verifying it -- disclosed instead."
  - "gopherstack-enpq (2026-08-14): CreateActivationInput.RegistrationMetadata ([]types.RegistrationMetadataItem, optional Key/Value pairs attached to the registered instance) is not modeled. Low-value (no consumer reads it back anywhere in this backend's Activation-derived Node/Instance types), disclosed rather than added speculatively."
  - "gopherstack-enpq (2026-08-14): DeleteInventoryInput's ClientToken (idempotency token) and SchemaDeleteOption (DisableSchema/DeleteSchema) are not modeled -- this backend only tracks inventory items, not versioned schema state, so the two SchemaDeleteOption values have no distinct effect to honor without inventing schema-versioning state that does not otherwise exist here."
  - "gopherstack-enpq (2026-08-14): PutInventoryOutput.Message (a free-text informational string, no documented behavioral meaning) is not modeled -- low value, disclosed rather than fabricating placeholder text."
  - "NoChangeNotification/ExpirationNotification are now fully EVALUATED (see families.parameter-store and Notes: 'Parameter policy notifications') — a new janitor sweep computes due-ness and calls an injectable ParameterPolicyNotifier, and the real EventBridge-side adapter (services/eventbridge/ssm_integration.go) is implemented and proven by a cross-package test (TestNotifyParameterPolicyAction). The ONE remaining piece, deliberately left undone because this agent was instructed not to edit cli.go, is the single wiring call — `ssmBackend.SetParameterPolicyNotifier(eventbridgeBackend)` (mirroring the existing SetEventBridgeIntegration/SetSQSIntegration/SetGlueIntegration wiring block in cli.go around wireStepFunctionsServiceIntegrations) — that actually injects the real notifier into the running SSM backend at startup. Until that line lands, PutParameter/the janitor behave exactly as before from an external caller's perspective (b.parameterPolicyNotifier is nil, so the sweep is a safe no-op) — see cli_wiring_note in the pass receipt."
  - "ValidateCloudConnector cannot make a real outbound call to Azure (gopherstack has no Azure tenant), so its ValidationFindings are derived deterministically from the connector's own stored Configuration (tenant/subscription IDs) rather than reflecting real third-party connectivity/permission state. This is an inherent sandbox constraint (same category as KMS being locally emulated instead of a real HSM call), not a wire/state bug — re-confirmed phase-2, still genuinely impossible for the same reason (no Azure credentials/tenant/egress available to the emulator, and reaching out to a live Azure tenant from an AWS emulator's request handler would be inappropriate even if it were possible) — documented here so a future reader doesn't mistake the mocked findings for verified AWS behavior."
  - "CreateMaintenanceWindow/UpdateMaintenanceWindow's new StartDate/EndDate/ScheduleTimezone/ScheduleOffset fields are stored and round-tripped verbatim but not evaluated — DescribeMaintenanceWindowSchedule/DescribeMaintenanceWindowExecutions do not yet factor StartDate/EndDate into whether a window is currently active, or ScheduleOffset into the computed next-run time. Untouched this pass — out of scope (not one of this pass's assigned gaps)."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 6): CreateResourceDataSync's S3Destination.DestinationDataSharing and SyncSource.AwsOrganizationsSource (both real, nested one level deeper -- Organizations cross-account config) are not modeled, matching the shallow-scalar convention this file already documents for SyncSource. DeleteResourceDataSync's SyncType is not modeled -- this backend's resourceDataSyncsStore keys solely by SyncName (store_setup.go's resourceDataSyncKeyFn), so two syncs can never coexist under one name with different types in this backend's model, making a SyncType match check unobservable rather than a real behavioral gap."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 6): ssm's commands family has no per-plugin execution model -- a whole SSM document runs as one synchronous unit (command_exec.go), so CommandPlugins (real, per-document-step status/output/timing breakdown on CommandInvocation), GetCommandInvocation's PluginName (accepted and echoed, cannot actually select among plugins that don't exist as distinct objects here), and ResponseCode (a real shell exit code this backend has no real process to derive a nonzero value from -- fabricating one for a Failed invocation would be inventing data) are all disclosed rather than fabricated. AlarmConfiguration/CloudWatchOutputConfig/NotificationConfig/TriggeredAlarms (real, round-trip-only fields on SendCommand/Command/CommandInvocation) are not modeled -- this backend has no CloudWatch-alarm-polling or SNS/EventBridge notification-firing infra for a Run Command execution to plug into; modeling the wire shape without any real behavior behind it would be indistinguishable from a stub. DocumentHash/DocumentHashType (client-side document-integrity validation, input-only) and Filters ([]types.CommandFilter on ListCommands/ListCommandInvocations, deeper Key/Value matching than the InstanceId/CommandId fields already modeled) are not modeled -- low value and no generic filter-operator engine exists yet (same class of gap already disclosed for GetInventory/ListComplianceItems' Filters)."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 6): GetParameter/GetParameters/GetParametersByPath's real wire type (types.Parameter) has no SourceResult member modeled -- real AWS populates it only when the parameter's Value is itself a reference into another AWS resource (aws:ssm:parameter/aws:ec2:image \"advanced parameter\" source resolution), a feature this backend does not implement at all (every parameter's Value is a caller-supplied literal). GetParameterHistory's LastModifiedUser and DescribeParameters' ParameterMetadata.LastModifiedUser both remain unmodeled -- no caller-identity/SigV4-principal infra to derive a real IAM ARN from, same disclosed-gap class as ServiceSetting.LastModifiedUser (pass 4). DescribeParametersInput.Shared (real, cross-account shared-parameter visibility) and the deprecated Filters ([]types.ParametersFilter, superseded by ParameterFilters/ParameterStringFilter which this backend already models) are not modeled -- Shared needs cross-account infra this backend does not have, and Filters is AWS's own deprecated field."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 7): DocumentDescription/GetDocumentOutput/DocumentIdentifier/DocumentVersionInfo's real VersionName member (an alternate, caller-assigned name for a specific document version, usable as a selector alongside $LATEST/$DEFAULT/an explicit version number) is modeled on DocumentVersionInfo only and was never populated by anything -- a dead, always-empty field. Not extended to CreateDocumentInput/UpdateDocumentInput/DocumentDescription/GetDocumentOutput this pass: doing so honestly needs a resolveDocumentVersionSelector-style lookup-by-name path threaded through GetDocument/DescribeDocument/UpdateDocumentDefaultVersion/ListDocumentMetadataHistory, which is a small feature of its own, not a field-diff-sized fix -- disclosed rather than half-wired."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 7): DocumentDescription's ApprovedVersion/PendingReviewVersion/ReviewInformation/ReviewStatus (the document review-approval workflow) and Category/CategoryEnum (AWS-curated document-marketplace categorization) remain entirely unmodeled, matching UpdateDocumentMetadata/ListDocumentMetadataHistory's existing stub status (no review state machine exists in this backend at all -- UpdateDocumentMetadata's required-field validation was fixed this pass, but it still does not persist or apply any review-state change). Author/Owner remain unmodeled for the same no-caller-identity-infra reason already disclosed for ServiceSetting.LastModifiedUser/Parameter's LastModifiedUser. GetDocumentOutput.AttachmentsContent (types.AttachmentContent{Hash,HashType,Name,Size,Url}, the actual attachment bytes' hash/size/URL) is not modeled -- this backend has no S3-backed object store to derive real content-addressed values from, same class as ValidateCloudConnector's no-real-Azure-tenant gap."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 8): Association/AssociationDescription's AlarmConfiguration, Date (creation date, distinct from the now-modeled Status.Date), LastExecutionDate, LastSuccessfulExecutionDate, ScheduleOffset, TargetLocations, TargetMaps and TriggeredAlarms remain unmodeled -- CreateAssociationInput/UpdateAssociationInput never carried the first and last three either. AlarmConfiguration/TriggeredAlarms need the same CloudWatch-alarm infra already disclosed as missing for commands (pass 6); TargetLocations/TargetMaps are alternate multi-account/multi-region and key-value targeting schemes this backend's single-region Targets-only model doesn't support; ScheduleOffset/LastExecutionDate/LastSuccessfulExecutionDate need a real scheduler this backend (which runs associations synchronously on demand, not on a cron loop) does not have."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 8): DescribeAssociationInput.AssociationVersion (view a specific historical version, real member on api_op_DescribeAssociation.go) is accepted-and-ignored -- this backend keeps only the current version of an association (ListAssociationVersions always returns exactly one synthesized entry), so there is no version history to select among. Real multi-version storage is a feature of its own, not a field-diff-sized fix."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 8): ListAssociations' real per-item response type is the narrower types.Association (AssociationId/AssociationName/AssociationVersion/DocumentVersion/Duration/InstanceId/LastExecutionDate/Name/Overview/ScheduleExpression/ScheduleOffset/TargetMaps/Targets only -- api_op_ListAssociations.go), not the full types.AssociationDescription every other op in this family returns; gopherstack's ListAssociations marshals the same internal Association record used everywhere else, so it over-projects fields real AWS's ListAssociations response never carries (ComplianceSeverity/SyncCompliance/MaxConcurrency/MaxErrors/Parameters/OutputLocation/CalendarNames/ApplyOnlyAtCronInterval/AssociationDispatchAssumeRole/AutomationTargetParameterName/AssociationDispatchAssumeRole/LastUpdateAssociationDate). Not a wire break (aws-sdk-go-v2's json unmarshaler silently discards unrecognized keys), but a real over-projection of an internal record straight to the wire the campaign has otherwise fixed elsewhere (GetParameter/DescribeSessions) -- disclosed rather than reshaped, since a narrower type here would also need to be kept in sync by hand with the same underlying store."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 8): UpdateAssociation's own doc comment (api_op_UpdateAssociation.go) states real AWS replaces every optional parameter with null when omitted from the request ('the system removes all optional parameters from the request and overwrites the association with null values for those parameters'); gopherstack's UpdateAssociation merges instead -- an omitted field leaves the prior value untouched (applyAssociationCoreUpdates/applyAssociationExtendedUpdates, associations.go). Implementing real replace semantics needs UpdateAssociationInput's plain string/bool fields switched to pointers (or a raw-JSON presence check) to distinguish 'omitted' from 'explicitly cleared', which no other op in this struct currently needs and would ripple through every existing merge-semantics test in associations_test.go (e.g. TestUpdateAssociation/update_name_and_version relies on Targets/Duration/etc. surviving an update that doesn't mention them) -- disclosed as a real, doc-confirmed behavioral gap rather than reshaped under time pressure."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 8): StartAutomationExecutionInput's AlarmConfiguration/ClientToken/Tags/TargetLocations/TargetLocationsURL/TargetMaps/TargetParameterName/Targets remain unmodeled, matching the pre-existing shallow-scalar simplification Runbook already documents (this backend runs one synchronous, single-account/region execution per call, so multi-target rate-control and cross-account/region fan-out have nothing to plug into). SendAutomationSignal's Payload (real, required for StartStep/StopStep/Resume) is now a Go field but not consulted for per-step targeting -- this backend has no per-step Waiting/InProgress state (every step goes straight to Success), same simplification as WarningMessage (gopherstack-gt9o)."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 8): RegisterTaskWithMaintenanceWindowInput/UpdateMaintenanceWindowTaskInput's AlarmConfiguration/ClientToken/CutoffBehavior/LoggingInfo/TaskInvocationParameters/TaskParameters remain unmodeled. TaskInvocationParameters is the real per-task-type union (RunCommand/Automation/StepFunctions/Lambda parameters) that actually carries what a registered task executes -- this backend's task model (MaintenanceWindowTask) has no concept of invocation-specific parameters at all, only the shallow TaskArn/TaskType/Targets/MaxConcurrency/MaxErrors/Priority already modeled; implementing the full 4-variant union is a feature of its own, not a field-diff-sized fix, disclosed rather than half-modeled. TaskParameters is AWS's own deprecated predecessor to TaskInvocationParameters, lower value. UpdateMaintenanceWindowTaskInput.Replace (real, changes merge-vs-replace update semantics, same class as UpdateAssociation's replace-semantics gap) is also unmodeled -- this backend always merges."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 8): DeregisterTargetFromMaintenanceWindowInput.Safe (real, when true rejects deregistering a target still referenced by a registered task instead of deregistering unconditionally) is not modeled -- this backend does not check for referencing tasks at all before deregistering a target, a real permissiveness gap. GetMaintenanceWindowExecutionTaskInvocationOutput.Parameters (real, the actual command/automation parameters used for one invocation) is not modeled -- this backend has no per-invocation parameter snapshot, only task-level defaults."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 9): DescribePatchPropertiesOutput.Properties aggregates baseline name/OS pairs and ignores both required input fields (OperatingSystem, Property) entirely -- real AWS instead lists distinct catalogue values of the requested Property (PRODUCT/PRODUCT_FAMILY/CLASSIFICATION/MSRC_SEVERITY/PRIORITY/SEVERITY) for the given OS (api_op_DescribePatchProperties.go doc comment). This is a real, pre-existing functional bug independent of the required-field validation this pass added, but the real per-Property map-key convention for the output's untyped []map[string]string cannot be verified from the pinned SDK source (no member-level schema exists to diff against) -- same disclosed-gap class as GetInventorySchema.Attributes (pass 4), left unfixed rather than fabricating a differently-wrong shape."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 9): UpdatePatchBaselineInput.Replace (real, switches update semantics from merge to replace-on-omit) is not modeled -- same class as UpdateAssociation's disclosed replace-semantics gap (pass 8), needs pointer fields throughout UpdatePatchBaselineInput to distinguish omitted from explicitly-cleared, which would ripple through existing merge-semantics tests. CreatePatchBaselineInput.ClientToken (idempotency token) is not modeled -- low value, same class as CreateActivationInput.RegistrationMetadata (pass 4)."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 9): GetDeployablePatchSnapshotForInstanceInput.BaselineOverride (real, a full inline PatchBaseline substituted for the instance's actual registered baseline when computing the snapshot) and UseS3DualStackEndpoint (affects only which S3 endpoint form the fabricated SnapshotDownloadUrl would use) are not modeled -- this backend's snapshot response is already synthetic (no real snapshot-generation lifecycle backs SnapshotId), so BaselineOverride would need real effective-patch computation threaded through a second baseline that isn't the instance's registered one, a feature of its own."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 9): DescribePatchGroupStateOutput is missing 6 real *int32 members with no Go field at all -- InstancesWithAvailableSecurityUpdates/InstancesWithInstalledPendingRebootPatches/InstancesWithInstalledRejectedPatches/InstancesWithOtherNonCompliantPatches/InstancesWithSecurityNonCompliantPatches/InstancesWithUnreportedNotApplicablePatches. These need per-instance security-update-specific and pending-reboot compliance tracking this backend's InstancePatchState does not carry (only FailedCount/InstalledCount/MissingCount), a feature of its own rather than a field-diff-sized fix."
  - "gopherstack-enpq (2026-08-21, structfielddiff pass 9): DescribeAvailablePatches' real filter keys PATCH_ID/MSRC_SEVERITY/PRODUCT_FAMILY/PATCH_SET (api_op_DescribeAvailablePatches.go doc comment) are not honored -- this pass wired PRODUCT/NAME/SEVERITY/CLASSIFICATION, the four backed by fields Patch already models; the rest would need Patch extended with fields the built-in catalogue has no real per-patch data to populate honestly."
  - "gopherstack-uox6 (value-semantics sweep, 2026-08-30): documentMatchesFilters' DocumentKeyValuesFilter Owner key ('Self' vs. other accounts) is not modeled -- this backend has no caller-identity infra to resolve 'Self' against, same disclosed-gap class as ServiceSetting.LastModifiedUser. The tag:tagName custom-key form is also not modeled -- would need the per-resource misc-tag store (already used for Document.Tags on the response side) threaded into the filter matcher, not wired this pass."
  - "gopherstack-s7aq (2026-09-07): audited SendCommand/CreateAssociation's MaxConcurrency/MaxErrors against
    what the two fields are documented to actually do, since the issue was filed title-only. First
    established the fan-out DOES exist (the issue's own premise was worth checking): SendCommand resolves
    InstanceIds+Targets into one CommandInvocation per instance (commandTargetInstanceIDs/
    mergeUniqueInstanceIDs, commands.go) stored per-command and readable via ListCommandInvocations -- not
    a single unexpanded command record. MaxConcurrency (SendCommandInput/CreateAssociationInput doc
    comments, api_op_SendCommand.go:95-100/api_op_CreateAssociation.go:157-165: 'maximum number of managed
    nodes/targets ... allowed to run the command/association at the same time') throttles CONCURRENT
    in-flight execution -- a notion this backend has nothing to violate: SendCommand builds every
    invocation, drives Pending->InProgress->Success/Failed and returns, all within one synchronous call
    with no elapsed wall-clock time and no concept of two invocations overlapping. There is no state for a
    concurrency cap to constrain, so applying it would mean fabricating a delay/queue purely to make a
    number 'do something' -- rejected as invented behavior, not verified AWS behavior. Structural, matches
    this file's existing precedent for other unobservable-in-a-synchronous-emulator fields (e.g.
    ValidateCloudConnector's real-Azure-call gap). MaxErrors (api_op_SendCommand.go:102-106/
    api_op_CreateAssociation.go:167-180: 'stops sending the command to additional targets' after N
    failures, i.e. a final-state stop condition, not just a timing artifact) was checked separately per the
    issue's own instruction, since a stop-after-N-failures rule IS observable in a synchronous model in
    principle -- but renderCommandOutput (command_exec.go) computes exactly ONE (stdout, stderr, status)
    result per SendCommand call from the document/parameters, and commands.go's SendCommand stamps that
    SAME finalStatus onto every resolved instance's invocation outside the per-instance loop -- so every
    invocation of one command always shares an identical outcome; there is no per-instance failure
    variance for MaxErrors to threshold against today. Implementing MaxErrors now would require first
    building genuine per-instance outcome variance (e.g. a way for individual target scripts/documents to
    fail independently), which is a real feature of its own -- explicitly out of this issue's scope per its
    own SCOPE NOTE against speculative fan-out/invocation-subsystem work. Both left unfixed, disclosed
    rather than faked. FIXED, self-contained (the issue's own suggested fallback): NEITHER field was
    validated against its documented format at all. ssm/2014-11-06/service-2.json (botocore wire model)
    declares MaxConcurrency pattern ^([1-9][0-9]*|[1-9][0-9]%|[1-9]%|100%)$ and MaxErrors pattern
    ^([1-9][0-9]*|[0]|[1-9][0-9]%|[0-9]%|100%)$ (both min 1/max 7 chars) -- matching the same two doc
    comments' 'You can specify a number such as 10 or a percentage such as 10%' language -- but SendCommand
    and CreateAssociation both accepted any string verbatim: '0' for MaxConcurrency (only MaxErrors allows
    the literal zero), a leading-zero '05', 'abc', or an out-of-range '150%' all round-tripped with no
    error. Now rejected via new validateMaxConcurrency/validateMaxErrors (commands.go), returning
    ValidationException like every other required/format check in this package. The identical unvalidated
    gap exists on UpdateAssociation, RegisterTaskWithMaintenanceWindow/UpdateMaintenanceWindowTask, and
    StartAutomationExecution's Runbooks[].MaxConcurrency/MaxErrors -- left alone as out of this issue's
    stated SendCommand/CreateAssociation scope, though the same validateMaxConcurrency/validateMaxErrors
    helpers apply directly if those are visited later. TestSendCommand_MaxConcurrencyMaxErrorsValidation
    (commands_test.go)/TestCreateAssociation_MaxConcurrencyMaxErrorsValidation (associations_test.go),
    both hand-verified failing (every malformed-value subtest got nil instead of ValidationException)
    against unfixed code."
deferred: []              # phase-2 (2026-07-24): closed CreateAssociationInput/UpdateAssociationInput/
                           # CreateAssociationBatchRequestEntry field gaps (bd gopherstack-ouvq),
                           # CreateOpsItemInput/UpdateOpsItemInput field gaps (bd gopherstack-iq4m),
                           # PatchBaseline.ApprovedPatchesEnableNonSecurity bool->*bool, ListCloudConnectors/
                           # ValidateCloudConnector MaxResults bounds (now AWS-published), and implemented
                           # NoChangeNotification/ExpirationNotification evaluation+emission end-to-end
                           # except the single cli.go injection line. Remaining open items are proven
                           # impossibilities (ValidateCloudConnector's Azure call) or genuinely out of this
                           # pass's assigned scope (MaintenanceWindow schedule evaluation).
leaks: {status: clean, note: "Janitor (janitor.go) is the only background goroutine, ctx.Done()-aware, single Run() loop shared across all sweeps (parameters/commands/sessions). PutParameter's history cap now also deletes the corresponding parameterLabels[version] entries on eviction (previously left as an unbounded-growth leak: labels attached to since-evicted versions stayed in the map forever with no key ever removed). THIS PASS: new AccessRequest store (services/ssm/sessions.go) follows the same pattern as patchBaselines/opsItems/documents — a user-managed resource with no automatic janitor sweep, not a leak (consistent with existing precedent for resources the caller is expected to explicitly delete). No new goroutines/tickers/timers introduced this pass; the epoch-seconds timestamp fix (see overall note) touched only struct field types and their few call-site assignments, no new state or locking."}
---

## Notes

### 2026-09-07 (gopherstack-jpfk: is ValidationException-sentinel reuse a defect class?)

Follow-up to gopherstack-mq6m (mgn probe: shared-helper artifact, not a defect, STOP). ssm's 71
class A findings spread across 129 distinct source lines (no site over 2 findings) rather than
2 funnel functions, so mgn's verdict does not automatically transfer. Sampled the full set of
non-"is required" (enum/pattern/cross-field) checks plus a dozen "is required" checks spread
across 9 files (activations.go, associations.go, automations.go, cloud_connector.go,
documents.go, inventory.go, maintenance_window.go, parameters.go, patch_baselines.go,
resource_policies.go), and checked each op's own `deserializeOpError<Op>` switch
(ssm@v1.73.4 deserializers.go) plus the *doc comment* on each candidate declared exception type
(types/errors.go) — not just its name.

**Verdict: split, and small.** The overwhelming majority (126 of 129 sites) are genuinely
idiomatic: every "X is required" check guards a field the SDK's own input struct marks
`// This member is required` (a Smithy `@required` trait), and every "must be one of"/enum
check guards a modeled enum member. Only 3 of ssm's 152 ops declare `ValidationException` at
all — consistent with real AWS Smithy services validating `@required`/enum/range-trait
violations at the protocol front door, uniformly, *before* dispatching to an operation's own
business logic, which is exactly why that layer isn't in the op's own modeled exception union.
Confirmed against several superficially-plausible-looking alternatives that turned out to be a
false fit on closer reading of the exception's own doc comment, not just its name — `InvalidParameters`
("values for all required parameters in the SSM **document**", i.e. CreateAssociation/
CreateActivation's *document*-parameters concept, not general required-input) and
`InvalidAutomationSignalException` ("the signal isn't valid **for the current execution**",
a runtime-state check, not "not a recognized enum value") both looked like fits by name and
were not by meaning.

**3 sites in PutParameter are the exception, fixed this pass** — supersedes the 2026-08-29 note
below, which left `validateParameterName` unfixed because it "could not establish with
confidence" what code AWS sends for a shape-constraint violation. This pass found that
confidence in the *doc comments* PutParameter's own three declared exceptions carry (types/errors.go):
`ParameterPatternMismatchException` ("The parameter name isn't valid"),
`UnsupportedParameterType` ("The parameter type isn't supported"), and
`InvalidAllowedPatternException` ("The request doesn't meet the regular expression
requirement") match `validateParameterName`, the `Type` enum check, and `validateAllowedPattern`
respectively, word for word. New sentinels `ErrParameterNamePattern`, `ErrUnsupportedParameterType`,
`ErrInvalidAllowedPattern` (errors.go), wired via a new `classifySSMParameterValidationError`
split-out (handler.go, single call site each — verified no other op shares
`validateParameterName`/`isValidParameterType`/`validateAllowedPattern`). PutParameter's
`DataType` check (no declared exception fits) is left on `ValidationException` with a landmine
comment, and `TestPutParameter_DataType_Invalid` now pins that as deliberate. Pre-existing
`TestValidateParameterName` (handler_test.go) and `TestPutParameter_Validation` (three cases,
parameters_handler_test.go) asserted the old generic `ErrValidationException` and were
strengthened to the specific sentinel; `TestSSMHandler_ValidationError` (parameters_handler_test.go)
had a stale comment/weak assertion and was tightened to check `__type` exactly.

Remaining 68 findings (across the other files/ops) are not fixed and, per the above, are not a
defect class — no code change beyond the three PutParameter sites.

### 2026-08-29 (error-path sweep: what a typed client sees on failure)

Extracted all 152 `awsAwsjson11_deserializeOpError<Op>` switches from ssm@v1.73.4's
deserializers.go (ground truth for which exception codes each op actually models) and
cross-referenced every backend call site that raises a sentinel error against its own op's set.
The shared error table (`classifySSMError`/`classifySSMErrorExtended` in handler.go) was
correct; every bug was the sentinel chosen at a specific call site, consistent with this
campaign's pattern across other services. Six real bugs fixed, all covered by new
`error_path_sweep_test.go` (real `aws-sdk-go-v2/service/ssm` client, `errors.As` against the
SDK's own typed exception):

- **Sentinel reused across the Parameter resource family**: `AddTagsToResource`,
  `RemoveTagsFromResource`, `ListTagsForResource` all raised `ErrParameterNotFound`
  ("ParameterNotFound") for a missing Parameter target — none of these three ops model that
  code; their own deserializers model `InvalidResourceId` ("The resource ID isn't valid...",
  ssm@v1.73.4 types/errors.go). New `ErrInvalidResourceID` sentinel wired at all three call sites.
- **Fabricated code**: `DeleteActivation` raised a wire code of `"ActivationNotFound"`, which
  does not appear anywhere in ssm@v1.73.4 (not `types/errors.go`, not any deserializer) — not a
  real AWS SSM error at all. Its own deserializer models `InvalidActivationId`. Renamed the
  sentinel to `ErrInvalidActivationID` (single call site, single blast radius) and fixed two
  existing tests (`activations_test.go`, `ops_metadata_test.go`) that asserted the fabricated
  code as correct.
- **Should-not-error (idempotent delete)**: `DeleteMaintenanceWindow` and `DeletePatchBaseline`
  both raised a not-found error for an unknown ID, but neither op's deserializer models any
  not-found-shaped exception (only `InternalServerError`, plus `ResourceInUseException` for the
  latter) — matching `DeleteOpsItem`'s sibling pattern, whose own SDK doc comment states
  explicitly: "This operation is idempotent. The system doesn't throw an exception if you
  repeatedly call this operation for the same OpsItem." `DeleteOpsItem` itself had the identical
  bug (raised `OpsItemNotFoundException`, which it doesn't model either). All three now delete
  idempotently and return success.
- **Missing-error → wrong success behavior**: `GetPatchBaselineForPatchGroup` raised
  `DoesNotExistException` (unmodeled — the op's deserializer declares zero exceptions besides
  `InternalServerError`) when no explicit patch-group mapping was registered. Real AWS always
  resolves a patch group to a baseline, falling back to the AWS-managed default for the OS — the
  same fallback `GetDefaultPatchBaseline` already implements. Now shares that fallback logic
  (new `defaultPatchGroupKey` constant replacing three duplicated `"default"` literals).

**Left, not fixed — codes I could not establish with confidence:**
- `ErrValidationException` ("ValidationException") is raised across roughly 60 of the 152 ops as
  a generic "field X is required" catch-all, but only 3 ops (`GetAccessToken`,
  `StartAccessRequest`, `StartExecutionPreview`) declare it in their own deserializer. The
  overwhelming majority of these call sites check a field the SDK's own client-side
  `validateOp<Op>Input` already marks `// This member is required` and rejects before the
  request is ever sent (confirmed for several, e.g. `PutParameterInput.Name`,
  `DeletePatchBaselineInput.BaselineId`) — unreachable through a real typed client, so not part
  of this bug class and not fixed. A smaller set are semantic (non-required-field) checks — e.g.
  `validateParameterName`'s length/regex/reserved-prefix checks in `PutParameter`, which the
  client-side validator does *not* block — where I could not establish with the SDK's own
  deserializer what code AWS actually sends for a shape-constraint violation on a legacy
  JSON-RPC service like ssm (unlike newer REST-JSON services, ssm's op models don't uniformly
  declare `ValidationException`, and I could not rule out that AWS's front-end applies it
  uniformly regardless of per-op modeling). Left rather than guessed, per this campaign's
  restraint principle. **Superseded 2026-09-07** (gopherstack-jpfk, see Notes above): the three
  `validateParameterName`/`Type`/`validateAllowedPattern` checks in `PutParameter` now map to
  `ErrParameterNamePattern`/`ErrUnsupportedParameterType`/`ErrInvalidAllowedPattern`, found by
  reading the *doc comment* on PutParameter's own declared exception types rather than only
  their names. The generic "field X is required" majority described above still stands.
- `ErrCiphertextTooShort` falls through to a generic 500 (never classified) but is only
  reachable via a corrupted/forged stored ciphertext — not a condition a well-formed client
  request can trigger — so left as an internal invariant guard, not a client-facing bug.
- **Missing-error, fixed**: `ErrInvalidKeyID` (wire `InvalidKeyId`) was raised at exactly the
  right call site (`encryptSSMValue`, parameter_encryption.go:78 — `PutParameter` models
  `InvalidKeyId`) but was never wired into `classifySSMErrorExtended`, so a KMS-backed
  `PutParameter` failure always surfaced as an opaque 500 `InternalServerError` (and got
  retried 3x by the SDK's retry logic as a result) instead of the modeled 400. Fixed via a new
  `classifySSMResourceIdentityError` split-out (also covers `ErrInvalidActivationID`/
  `ErrInvalidResourceID`, keeping `classifySSMErrorExtended` under the cyclop budget). Covered
  by `TestPutParameter_InvalidKMSKey_RealClient`.

**Also observed, not part of this bug class**: `GetPatchBaselineForPatchBaselineOutput` (the
gopherstack-internal type name for `GetPatchBaselineForPatchGroup`'s response) has a typo baked
into its name; unrelated to error wiring, left as-is. `ErrExecutionPreviewNotFound`,
`ErrInventoryNotFound`, `ErrDocumentVersionNotFound` are declared in errors.go but never raised
anywhere in the package — dead sentinels, not wired to any call site; left as-is (declaring but
not using is not itself a wire bug).

### 2026-08-22 (gopherstack-enpq, doc-prose/bidirectional re-audit pass 11)

Passes 4–10 swept all 152 ssm ops with `cmd/structfielddiff` (field-list diff against the pinned
SDK) and closed every family the campaign named. That method compares field lists; it does not ask
whether an op can be **called the way its own documentation prescribes** — the gap that turned
kinesis's claimed A-grade sweep into 11 more bugs (`23acbec3b`) and cloudwatchlogs's into 2 missing
alternate identifiers, both found only once the doc prose was read directly. This pass applied that
same lens to a slice of ssm rather than re-running structfielddiff.

**Ops covered by this pass's doc-prose method** (not a re-diff — a fresh read of each op's SDK doc
comment plus its request validator, checked in both directions): `SendCommand`, `CancelCommand`,
`ListCommands`, `ListCommandInvocations`, `GetCommandInvocation` (commands family, full);
`DescribeInstanceInformation` (re-checked its documented `InstanceInformationFilterList`-is-legacy
note — already correctly modeled, no bug); `UpdateAssociation` (re-confirmed the existing
merge-vs-replace disclosure in `families.state-manager-associations` still describes reality);
`GetPatchBaseline`/`RegisterDefaultPatchBaseline` (checked, see rejected candidate below);
`StartAutomationExecution`/`Runbook.Targets`/`TargetParameterName`/`TargetMaps` (re-confirmed the
existing `models_automations.go` disclosure — deliberately unmodeled, still accurate);
`PutComplianceItems` (re-checked its validator against `validateOpPutComplianceItemsInput` —
matches exactly, no over/under-strict mismatch found).

**Ops NOT covered by this pass's method** — the remaining ~145 ops across
activations/associations/automation-executions/cloud-connectors/compliance/documents/inventory/
maintenance-windows/managed-instance/nodes/ops-center/parameter-store/patch-baselines/
resource-data-sync/resource-policies/service-settings/sessions/state-manager-associations/tags were
NOT re-read against their doc prose this pass. They carry passes 4–10's structfielddiff-class
verification (field-list diff, hand-verified against serializers/deserializers) but not this pass's
"can it be called the documented way, checked both directions" method. Treat that as the honest
boundary of this pass, not as those families being fully clean by this stronger method.

**Bug found and fixed**: `SendCommand`'s `Targets` (see `families.commands` above for full detail)
— a required-alternate-identifier gap of the same shape as kinesis's `StreamARN`, fixed via a new
`CommandTarget` type and an instance-ID union in `commands.go`.

**Rejected candidate**: `GetPatchBaseline`/`RegisterDefaultPatchBaseline`'s doc comments say a
caller "can specify the full patch baseline Amazon Resource Name (ARN)... instead of" the plain
`pb-...` ID, scoped explicitly to **AWS-managed** baselines (e.g. `AWS-AmazonLinuxDefaultPatchBaseline`).
Not counted: gopherstack does not model AWS-managed/predefined patch baselines at all (no baseline
is ever seeded, and `CreatePatchBaseline` only ever hands back a plain `pb-`-prefixed ID, never an
ARN) — there is no reachable path by which a real caller would end up needing to pass the specific
ARN form the docs describe, since the managed baselines that alternate identifier is documented for
don't exist in this backend. Disqualified as not reachable, not as a false positive on the field
diff itself.

SSM speaks the **json-1.1 protocol** (`AmazonSSM.<Op>` `X-Amz-Target`, `application/x-amz-json-1.1`
content type) — confirmed via `handler.go`'s `classifySSMError`/`handleError` using
`service.JSONErrorResponse` with a bare `{"Type":..., "Message":...}` body, not XML.

### Empty-struct-input candidates: fixed (gopherstack-a250, closing the gopherstack-6uag follow-up)

The 7 ops flagged by the previous pass — `DescribeActivations`, `ListResourceDataSync`,
`DescribeInstanceInformation`, `ListAssociations`, `DescribeAutomationExecutions`,
`ListOpsMetadata`, `GetOpsSummary` — were re-verified against the pinned `ssm@v1.73.4` SDK and 6
of the 7 fixed: each real `*Input` has optional `Filters`/`MaxResults`/`NextToken`-class members a
literal `struct{}` input discarded on every request. See the per-op notes above (and
`families.state-manager-associations`/`ops-center`/`automation-executions`) for exactly what each
now filters on and its SDK citation. `GetOpsSummary` is the one exception, deliberately left
unwired — see its note under `families.ops-center` above: this backend's `GetOpsSummary` returns
one fixed synthetic entity, not a queryable dataset its `Aggregators`/`Filters`/`ResultAttributes`
could honestly project across. All 6 fixes are proven by real-`aws-sdk-go-v2`-client tests in
`empty_struct_inputs_test.go`, each hand-verified to fail against the pre-fix code.

### Real bug: Intelligent-Tiering was rejecting the exact case it exists for

`resolveTier` treated `Intelligent-Tiering` identically to `Standard` for the 4096-byte size
check — a `PutParameter` with `Tier: "Intelligent-Tiering"` and a 5000-byte value returned
`ValidationException` instead of succeeding. This defeats the entire purpose of the tier: per AWS
docs (confirmed via websearch against the GitHub aws-sdk-net issue tracker and the AWS
"Managing tiers" user guide), Intelligent-Tiering auto-promotes to Advanced whenever the request
needs a capability Standard doesn't support — either a value over 4 KiB, or parameter policies
attached — rather than failing. Fixed: `resolveTier` now upgrades `tier` to `"Advanced"` in that
case (and still enforces the 8 KiB Advanced ceiling on top). An explicit `Tier: "Standard"` still
hard-fails on the same conditions, since the caller opted out of auto-selection by naming a
concrete tier. Confirmed via websearch that Policies (Expiration/ExpirationNotification/
NoChangeNotification) are Advanced-tier-only — Standard rejects them outright (this AWS constraint
was previously entirely unenforced; any tier could carry a Policies string). Three existing tests
in `parity_emr_test.go` (`TestParityEMR_ParameterExpiration_JanitorEvicts`) attached an Expiration
policy without ever setting `Tier`, i.e. exercised Standard+Policies — updated those to
`Tier: "Advanced"` since that combination is what real AWS requires; the janitor-eviction behavior
under test is otherwise untouched.

### Real bug: labeled parameter versions could be silently evicted

`PutParameter` caps stored history at 100 versions (`maxHistoryCap`), evicting the oldest entry on
overflow. AWS's actual behavior (confirmed via websearch of the
`ParameterMaxVersionLimitExceeded` exception docs) is that this eviction is refused — and the
whole `PutParameter` call fails with `ParameterMaxVersionLimitExceeded` — when the version about to
be evicted has a label attached, specifically so a labeled ("prod", "release-42", etc.) version is
never silently destroyed out from under a consumer pinned to that label. The emulator previously
evicted unconditionally. Fixed with a pre-mutation check (oldest history entry's
`parameterLabelsStore` entry) that aborts the whole write before any state changes if the oldest
version is labeled. Also closed a companion leak: `parameterLabels[name][version]` entries for
already-evicted versions were never deleted, so a parameter updated thousands of times would
accumulate stale label-map entries forever; eviction now deletes them.

### Real bug: parameter name hierarchy depth was never validated

AWS caps a parameter name at 15 "/"-delimited levels (confirmed via the `PutParameter` API
reference's own worked example: `/L1/.../L14/name` is valid, one more level throws
`HierarchyLevelLimitExceededException`). `validateParameterName` checked length, double-slashes,
reserved prefixes, and the name-charset regex, but never counted hierarchy depth. Added
`parameterHierarchyLevels`/`maxParamHierarchyLevels` and the new `ErrHierarchyLevelLimitExceeded`
sentinel, wired into `classifySSMError`.

### Real bug: DescribeDocument/CreateDocument/UpdateDocument leaked Content in a metadata-only response

AWS's real `DocumentDescription` structure (returned by all three ops) has **no `Content` field**
— confirmed by grepping `aws-sdk-go-v2/service/ssm/types/types.go` for `DocumentDescription
struct`. Only `GetDocument` returns document content; the metadata ops deliberately omit it (likely
so a `ListDocuments`-adjacent describe call doesn't have to re-transmit a potentially large
document body). This emulator's `CreateDocumentOutput`/`UpdateDocumentOutput`/
`DescribeDocumentOutput` all embedded the full internal `Document` struct — which does carry
`Content` (no `omitempty`) for `GetDocument`'s own use — so every describe/create/update response
included the entire document body. A conformant SDK client ignores unknown response fields, so this
wasn't client-breaking, but it is a real wire-shape deviation (and a needless
content-in-metadata-response leak) per the audit's wire-shape-accuracy bar. Fixed by introducing a
separate `DocumentDescription` type (mirrors `Document` minus `Content`) and a
`Document.toDocumentDescription()` converter; the three ops now return that type. Covered by a new
JSON-serialization assertion (`Test_DescribeDocument_OmitsContentAndHonorsVersionSelector`) since a
Go zero-value-string field is indistinguishable from an absent field in a struct-level comparison
— only marshaling and checking the wire bytes actually catches this class of bug.

### Real bug: explicit "$DEFAULT" document version was conflated with "$LATEST"

`GetDocument` and `DescribeDocument` both special-cased `""`, `"$LATEST"`, and `"$DEFAULT"`
identically, always serving the document's latest content/metadata. But `$DEFAULT` is a distinct,
explicit selector — pinned independently via `UpdateDocumentDefaultVersion` — that can diverge from
`$LATEST` (create v1, `UpdateDocument` to v2, never repoint the default: v1 is still `$DEFAULT`,
v2 is `$LATEST`). A caller explicitly asking for `$DEFAULT` in that state got v2's content instead
of v1's. Fixed via a shared `resolveDocumentVersionSelector(doc, requested)` helper used by both
ops; `DescribeDocument` additionally now looks up the resolved version's own
`DocumentVersion`/`DocumentFormat`/`Status` from `documentVersionsStore` instead of always
reporting the top-level (latest) document's fields.

**Deliberately NOT changed**: what an *omitted* `DocumentVersion` resolves to. AWS's API reference,
CLI reference, and user guide (all checked via WebFetch this pass) do not state whether omitting
the parameter is equivalent to `$DEFAULT` or `$LATEST` — evidence was genuinely ambiguous — and an
existing test (`document_test.go`'s `TestInMemoryBackend_Snapshot_IncludesDocumentsAndCommands` /
`document_survives_round_trip`) explicitly asserts that an omitted version returns the *latest*
content after an `UpdateDocument`. Changing that risked a real regression on weak secondary
evidence, so omitted-version behavior is left exactly as before (== `$LATEST`); only the
unambiguous explicit-`$DEFAULT` case was fixed.

### Already-correct traps (do not re-flag)

- `GetParametersByPath` (`MaxResults` 1-10, default 10) and `DescribeParameters`/
  `GetParameterHistory` (`MaxResults` 1-50, default 50) look asymmetric but are correct — these are
  AWS's actual, independently-documented per-op limits, not a copy-paste inconsistency.
- `resolveTier`'s explicit-`Standard`-tier hard-fail on `Policies` is intentional per AWS
  (`Standard tier parameters ... can't be configured to use parameter policies`) — do not "fix" it
  to silently upgrade the tier the way `Intelligent-Tiering` does; only `Intelligent-Tiering` gets
  auto-promotion, `Standard` is an explicit opt-out of that.
- `PutParameter`'s `Intelligent-Tiering` tier is echoed back verbatim in the response (`Tier:
  "Intelligent-Tiering"`) when no promotion is needed — it does **not** resolve to the concrete
  `"Standard"` tier in the wire response. The `ParameterTier` enum in
  `aws-sdk-go-v2/service/ssm/types/enums.go` lists `Intelligent-Tiering` as a first-class value
  distinct from `Standard`/`Advanced`, confirming AWS reports what was requested, not the
  internally-selected concrete tier, except when a promotion actually occurs.
- `DeleteInventory` succeeding with `removed=0` for a `TypeName` with no stored items is correct,
  not a missing not-found check — AWS's `DeleteInventory` operates on a type across the whole
  fleet and a zero-item deletion is a valid, successful job (see `DeletionSummary.TotalCount`), not
  an error. The unused `ErrInventoryNotFound`/`ErrDocumentVersionNotFound` (duplicate of
  `ErrInvalidDocumentVersion`)/`ErrExecutionPreviewNotFound`/`ErrResourcePolicyNotFound` sentinels
  declared in `backend_ops.go`/`backend_batch2.go` are dead code from an earlier pass, not evidence
  of missing error handling — the operations that would use them either don't need a not-found path
  (see DeleteInventory above) or already return a differently-named sentinel with the same string.

### New feature this pass: Cloud Connectors (aws-sdk-go-v2 v1.69.5 → v1.73.4 added surface)

The re-audit protocol's "check the SDK module for ops added since sdk_version" step turned up 6
brand-new operations (`CreateCloudConnector`, `DeleteCloudConnector`, `GetCloudConnector`,
`ListCloudConnectors`, `UpdateCloudConnector`, `ValidateCloudConnector`) that the prior dependency
bump (`e51c0de9`) had silently carved out of `sdk_completeness_test.go`'s exclusion list rather
than stubbing or implementing — i.e. genuinely unimplemented surface, not a disguised stub. Per
parity-principles.md rule 1 ("if an op genuinely can't be implemented yet, say so explicitly —
never a half-working stub"), the alternative to implementing it would have been to leave it
excluded and documented; since Cloud Connectors turned out to be a well-scoped, single-union
(Azure-only) CRUD resource with no cross-service dependency, it was implemented for real instead
(`services/ssm/cloud_connector.go`, ~410 LOC + `cloud_connector_test.go`, ~340 LOC). All wire
shapes (field names, the epoch-seconds `CreatedAt`/`UpdatedAt` DateTime shape, the
`{"AzureConfiguration": {...}}` union-by-member-name wrapping, `CloudConnectorSummary`'s narrower
field set vs. the full `CloudConnector`/`GetCloudConnectorOutput`) were read directly out of
`aws-sdk-go-v2/service/ssm@v1.73.4`'s generated `serializers.go`/`deserializers.go`, not inferred
from Go doc comments. `ResourceNotFoundException` (a generic SDK error type, not a
CloudConnector-specific one) was chosen for the not-found case since no dedicated error type exists
for this resource. See `gaps` for the two known limitations (unconfirmed pagination bound; findings
are derived from stored config rather than a real Azure connectivity check) that were consciously
left as-is rather than guessed at with false confidence.

### parity-sweep-3 (2026-07-23): systemic epoch-seconds timestamp bug (the headline finding)

SSM speaks awsjson1.1. Every `DateTime`-shaped field in
`aws-sdk-go-v2/service/ssm@v1.73.4`'s generated `deserializers.go` is deserialized via
`smithytime.ParseEpochSeconds(f64)` from a `json.Number` — confirmed by grepping every one of the
~15 `case "<FieldName>":` blocks for the affected fields (see below), every single one hit the
`case json.Number: ... ParseEpochSeconds` branch and explicitly rejects anything else
(`default: return fmt.Errorf("expected DateTime to be a JSON Number, got %T instead", value)`).
This package's own convention for timestamps (`CreatedDate`, `ModifiedDate`, `StartDate`, `EndDate`
on Parameter/Document/PatchBaseline/MaintenanceWindow/etc.) already does this correctly via the
`UnixTimeFloat(t time.Time) float64` helper (`models.go`). But 9 structs across 6 files had instead
declared the field as a raw `time.Time` or `*time.Time`, which Go's `encoding/json` marshals as an
RFC3339Nano **string** by default (e.g. `"ExecutionDate":"2026-07-23T00:00:00Z"`) — a real
`aws-sdk-go-v2` client parsing gopherstack's response for any of these fields would hard-fail with
exactly the deserializer error quoted above. This is the exact bug class the audit brief flagged as
having recurred in sagemaker/glue, and it had silently spread across a third service. Fixed by
converting every field to `float64` + `UnixTimeFloat` at each population site:

- `AssociationExecution.ExecutionDate` (`models_associations.go`, `associations.go`) — DescribeAssociationExecutions
- `MaintenanceWindowExecution.StartTime`/`EndTime`, `MaintenanceWindowExecutionTask.StartTime`,
  `MaintenanceWindowExecutionTaskInvocation.StartTime`, and the 3 `Get*OutputFull` variants of the
  same shapes (`models_maintenance_window.go`, `maintenance_window.go`) — the whole
  DescribeMaintenanceWindowExecution*/GetMaintenanceWindowExecution* op family
- `InstanceInformation.RegistrationDate`, `Node.CaptureTime` (renamed from `NodeInfo.RegistrationDate`
  when `ListNodes` was fixed for real, gopherstack-6uag, see below),
  `InstanceAssociationStatusInfo.ExecutionDate`, `InstancePatchState.OperationStartTime`,
  `PatchComplianceData.InstalledTime` (`models_instances.go`, `instances.go`, `patch_inventory.go`) —
  DescribeInstanceInformation/ListNodes/DescribeInstanceAssociationsStatus/DescribeInstancePatchStates/DescribeInstancePatches
- `ResourceDataSync.SyncCreatedTime`/`LastSyncTime` (`models_activations.go`, `activations.go`) — ListResourceDataSync
- `InventoryDeletion.DeletionStartTime` (`patch_inventory.go`, `inventory.go`) — DescribeInventoryDeletions

Two call sites (`instances.go`'s `RegistrationDate`/`ExecutionDate` population) had been doing a
pointless `time.Unix(int64(x.CreatedDate), 0).UTC()` round-trip from an *already-float64* source
field just to satisfy the (wrong) `time.Time` field type — simplified to a direct assignment now
that the field type matches the source. Locked in by a new dedicated test file,
`epoch_seconds_wire_shape_test.go`, asserting byte-for-byte that each affected field marshals as a
bare JSON number (not a quoted string) — a Go zero-value-string vs. absent-field comparison
wouldn't catch this class of bug, same rationale as the earlier `DocumentDescription` content-leak
fix's dedicated marshal-byte test.

### parity-sweep-3: Session Manager (previously deferred, now fully re-verified)

Field-diffed every session op against `aws-sdk-go-v2/service/ssm@v1.73.4`'s `api_op_*.go` files.
Six real bugs, all fixed (see the `ops:` notes for `StartSession`/`DescribeSessions`/
`GetConnectionStatus`/`GetAccessToken`/`StartAccessRequest` above for the specifics). The most
significant: `GetAccessToken`/`StartAccessRequest` were pure stubs that didn't implement the
just-in-time node access workflow at all — `GetAccessToken` returned an ad-hoc `TokenValue` field
that doesn't exist anywhere in the real `GetAccessTokenOutput` shape (`AccessRequestStatus` +
`Credentials{AccessKeyId,SecretAccessKey,SessionToken,ExpirationTime}`), and `StartAccessRequest`
never stored the request it claimed to create, so a `GetAccessToken` call against a real
`AccessRequestId` had no state to look up even in principle. Implemented as a real
`*store.Table[AccessRequest]`-backed resource, auto-approved (documented — gopherstack has no
approver workflow to model, and leaving every request permanently "Pending" would make
`GetAccessToken` a dead end for every caller).

### parity-sweep-3: Patch baselines & Maintenance windows (previously deferred, now fully re-verified)

Patch baselines: `CreatePatchBaselineInput`/`PatchBaseline` were missing `ApprovalRules`
(auto-approval rules), `GlobalFilters`, `Sources` (custom Linux repos), `RejectedPatchesAction`,
`AvailableSecurityUpdatesComplianceStatus`, and `ApprovedPatchesEnableNonSecurity` — six fields
confirmed present in `api_op_CreatePatchBaseline.go` and entirely absent from this package. Added
as real, persisted, round-tripped fields. `ApprovalRules` was round-trip-only at first
(not evaluated against actual patch matching); see gopherstack-e91b below for the fix that made
it real. Also: `GetPatchBaselineOutput.PatchGroups` (the patch
groups currently registered with a baseline) was entirely unpopulated — confirmed unique to
`GetPatchBaselineOutput` (absent from `UpdatePatchBaselineOutput`) via
`api_op_UpdatePatchBaseline.go`, now derived from the reverse `patchGroup->baselineID` map.

Maintenance windows: `RegisterTaskWithMaintenanceWindowInput.Targets` (the managed nodes/window-
targets a task runs against — required in practice for Run Command-type tasks per
`api_op_RegisterTaskWithMaintenanceWindow.go`) was accepted on the wire and silently discarded,
meaning a registered task could never actually record what it targets. Fixed through
Register/Update/Describe. `CreateMaintenanceWindowInput`/`UpdateMaintenanceWindowInput` were also
missing `StartDate`/`EndDate`/`ScheduleTimezone`/`ScheduleOffset` (confirmed present in both
`api_op_CreateMaintenanceWindow.go` and `api_op_UpdateMaintenanceWindow.go`), and
`AllowUnassociatedTargets` was previously create-only despite being documented as updatable — all
now round-trip (stored, not yet factored into schedule-execution logic — see `gaps`).

### gopherstack-e91b (2026-09-06): Patch baseline ApprovalRules/RejectedPatchesAction evaluation

`ApprovalRules` and `RejectedPatchesAction` were round-tripped verbatim (parity-sweep-3, above) but
never evaluated: `effectivePatchesForBaseline` branched only on the explicit `ApprovedPatches`/
`RejectedPatches` lists, leaving every other catalogue patch permanently `PENDING_APPROVAL`
regardless of any auto-approval rule, and this fed straight into `InstancePatchState`/
`PatchComplianceData` via `AWS-RunPatchBaseline`.

Precedence (AWS Systems Manager User Guide, "How security patches are selected", quoted via a
2026-09-06 fetch — not in the SDK source, which only models the fields): "A patch specified in the
approved patches list will be installed irrespective of whether it is matched by an approval rule
... Items in the rejected patches list will exclude those patches from being installed, even if
they match an approval rule and/or approved patch." i.e. `RejectedPatches` > `ApprovedPatches` >
`ApprovalRules`. The existing explicit-list handling already matched this order; `ApprovalRules` is
now evaluated only for patches in neither explicit list (`ruleOutcomeForPatch`,
`effectivePatchesForBaseline`), and a rule match produces the real `APPROVED` deployment status
(`types.PatchDeploymentStatusApproved`, distinct from `EXPLICIT_APPROVED`), which
`patchComplianceFromEffective` now also treats as installable.

`PatchFilterGroup` matching (`ruleFilterGroupMatches`) reuses `patchMatchesFilters`' supported
subset (`PRODUCT`/`NAME`/`SEVERITY`/`CLASSIFICATION` — the only `PatchFilterKey` values this
synthetic `Patch` catalogue has backing fields for, of the 18 in `types/enums.go`) but, unlike that
read-only helper (where an unsupported key is silently skipped, loosening the filter), a rule
containing any unsupported key here fails closed — the rule never matches — since silently
rule-approving a patch this emulator can't actually evaluate would be a compliance-data-corrupting
bug, not just a broader read.

`Patch` gained a real `ReleaseDate` field (epoch seconds, confirmed against
`deserializers.go`'s `awsAwsjson11_deserializeDocumentPatch`/`ParseEpochSeconds` — a genuine
pre-existing wire gap, not invented for this fix) so `ApproveAfterDays` has something to evaluate
against; `defaultPatchCatalog`'s five entries got fixed, deterministic release dates (never
`time.Now()`).

`ApproveAfterDays`/`ApproveUntilDate`: the SDK's `validatePatchRule` (validators.go) does not
enforce either being set or being mutually exclusive — only `PatchFilterGroup` is client-validated
as required. The API reference doc says "your request must include a value for either
ApproveAfterDays or ApproveUntilDate" (requires at least one) but has no "not both"/"mutually
exclusive" language anywhere in the `PatchRule` doc block, so this backend rejects
PatchFilterGroup-missing, Key/Values-missing (SDK-required fields), and neither-set with
`ValidationException`, plus the documented `ApproveAfterDays` range (0-360) and `ApproveUntilDate`
format (`YYYY-MM-DD`) — but accepts both being set rather than fabricating a rejection the real
service may not enforce. When both are set, `ApproveUntilDate` wins (`ruleOutcomeForPatch` checks
it first) as the more specific of the two (a fixed cutoff date vs. a relative day-count) — no AWS
source documents this tie-break either way, so it's a disclosed judgment call, not a verified fact.

`RejectedPatchesAction` (`BLOCK`/`ALLOW_AS_DEPENDENCY`) is now validated against the real
`PatchAction` enum (previously any string was silently stored). Full semantic evaluation — AWS
distinguishes them only via package **dependency** installation and `INSTALLED_REJECTED` history
for a patch installed *before* it was rejected — remains structural: this backend's synthetic
`Patch` catalogue has no dependency graph, and `InstancePatchState`/`PatchComplianceData` are
recomputed fresh on every `AWS-RunPatchBaseline` run rather than tracking pre-existing installed
state independent of the current baseline, so there is no "already installed, later rejected"
history to distinguish `BLOCK` from `ALLOW_AS_DEPENDENCY` against. Modeling this for real needs a
package-dependency feature and an incremental (not recomputed-per-run) instance patch history —
both real features of their own, not a field-diff-sized fix — so it's disclosed rather than faked;
current behavior (rejected patches always excluded from the compliance surface) already matches
`BLOCK`'s observable effect for the no-dependency case this backend can express.

Also disclosed, left untouched this pass: `GlobalFilters` (baseline-wide filter restricting which
catalogue patches the baseline covers at all) is still round-trip-only; a `PatchRuleGroup` with
more than one rule matching the same patch resolves via first-rule-in-list-order, not AWS's
undocumented internal resolution order, since the synthetic catalogue never needs more than one
matching rule to exercise real behavior.

### parity-sweep-3: State Manager associations & OpsCenter (previously deferred, spot-checked only)

Ran out of scope budget to fully field-diff these two op-by-op after the epoch-seconds fix (which
itself touched `DescribeAssociationExecutions`) and the Session/PatchBaseline/MaintenanceWindow
work above consumed the pass. What was verified: `CreateAssociationInput` is missing ~10 fields
(`ApplyOnlyAtCronInterval`, `ComplianceSeverity`, `MaxConcurrency`, `MaxErrors`, `OutputLocation`,
`ScheduleExpression`, `SyncCompliance`, `CalendarNames`, `AssociationDispatchAssumeRole`,
`AutomationTargetParameterName`, `Duration`) confirmed absent against
`api_op_CreateAssociation.go` (bd: gopherstack-ouvq); `CreateOpsItemInput`/`UpdateOpsItemInput` are
missing `AccountId`/`ActualStartTime`/`ActualEndTime`/`Notifications`/`PlannedStartTime`/
`PlannedEndTime`/`RelatedOpsItems` (mostly Change-Manager `/aws/changerequest`-oriented) confirmed
absent against `api_op_CreateOpsItem.go`/`api_op_UpdateOpsItem.go` (bd: gopherstack-iq4m) — `Priority`
was the one field from that list fixed this pass since it's simple and broadly useful outside
Change Manager specifically. Per the audit brief's instruction, these are recorded honestly as
`partial`/open gaps with bd issues filed, not reclassified to `ok`.

### parity-3 phase-2 (2026-07-24): closing all remaining gaps from the 2026-07-23 A- pass

This pass closed all six items in the PARITY.md `gaps:` list as of 2026-07-23, four for real
(State Manager associations, OpsCenter, PatchBaseline pointer semantics, ListCloudConnectors/
ValidateCloudConnector MaxResults bounds) and one to the maximum extent this pass's constraints
allow (parameter policy notifications — implemented end-to-end except a single cli.go injection
line this agent was explicitly instructed not to touch). The sixth (ValidateCloudConnector's
inability to call a real Azure tenant) was re-confirmed as a genuine, unclosable sandbox
constraint.

**State Manager associations** (bd gopherstack-ouvq, closed): `CreateAssociationInput` was missing
11 fields confirmed present in `api_op_CreateAssociation.go` — `ApplyOnlyAtCronInterval`,
`AssociationDispatchAssumeRole`, `AutomationTargetParameterName`, `CalendarNames`,
`ComplianceSeverity`, `Duration`, `MaxConcurrency`, `MaxErrors`, `OutputLocation`,
`ScheduleExpression`, `SyncCompliance`. All 11 were added to `CreateAssociationInput`,
`CreateAssociationBatchRequestEntry` (confirmed the same 11 fields exist on
`types.CreateAssociationBatchRequestEntry`), `UpdateAssociationInput` (confirmed the same fields,
minus `Name`, on `api_op_UpdateAssociation.go` — UpdateAssociation was previously not even
mentioned in the gap, but leaving it unable to touch fields CreateAssociation could set would have
been a new, self-inflicted asymmetry), and the stored `Association`/`AssociationDescription` type
so they round-trip through Describe/List. Two new wire types were added to model
`OutputLocation`: `InstanceAssociationOutputLocation` (the `S3Location` wrapper) and
`S3OutputLocation` (`OutputS3BucketName`/`OutputS3KeyPrefix`/`OutputS3Region`) — field names and
nesting confirmed against `types.InstanceAssociationOutputLocation`/`types.S3OutputLocation`.
`UpdateAssociation`'s cyclomatic complexity crossed the package's cyclop limit (15) once the new
fields were wired in (17); split into `applyAssociationCoreUpdates`/`applyAssociationExtendedUpdates`
rather than adding a `//nolint:cyclop` per this campaign's hard constraint against banned nolints.

**OpsCenter** (bd gopherstack-iq4m, closed): `CreateOpsItemInput`/`UpdateOpsItemInput` were missing
`AccountId`, `ActualStartTime`, `ActualEndTime`, `Notifications`, `PlannedStartTime`,
`PlannedEndTime`, `RelatedOpsItems` — confirmed present on both `api_op_CreateOpsItem.go` and
`api_op_UpdateOpsItem.go`. All 7 added to `CreateOpsItemInput`, `UpdateOpsItemInput`, and the
stored `OpsItem` type (two new small wire types: `OpsItemNotification{Arn}`,
`RelatedOpsItemRef{OpsItemId}` — named `RelatedOpsItemRef` rather than reusing the existing
`OpsItemRelatedItem` type, since that's a different, unrelated resource: the associate/disassociate
"related item" feature keyed by `AssociationId`/`AssociationType`/`ResourceType`/`ResourceUri`, not
the `RelatedOpsItems` field's simple `{OpsItemId}` list). `ActualStartTime`/`ActualEndTime`/
`PlannedStartTime`/`PlannedEndTime` are modeled as `*float64` (this package's `UnixTimeFloat`
epoch-seconds convention, matching the systemic fix from the 2026-07-23 pass) rather than a bare
`float64`, since these are genuinely optional (Change-Manager-only in real AWS) and a bare
`float64` couldn't distinguish "not applicable" from epoch-zero.
`UpdateOpsItemInput.OperationalDataToDelete` (confirmed present in `api_op_UpdateOpsItem.go` but
NOT one of the 7 fields bd gopherstack-iq4m tracked) was deliberately left unimplemented — adding it
would have been scope creep beyond the specific field list this pass was asked to close; documented
in a comment in `models_ops_items.go` so it isn't mistaken for an oversight. `UpdateOpsItem`'s
cyclomatic complexity also crossed the cyclop limit once wired in; split into
`applyOpsItemCoreUpdates`/`applyOpsItemChangeManagerUpdates`.

**PatchBaseline.ApprovedPatchesEnableNonSecurity**: confirmed via `go doc` that
`CreatePatchBaselineInput`/`UpdatePatchBaselineInput`/`PatchBaseline` (aliased as
`GetPatchBaselineOutput`) all declare this field `*bool`, not `bool`. Converted across all three;
`CreatePatchBaseline`'s assignment was already a straight field copy (works unchanged for either
type), `UpdatePatchBaseline`'s `if input.ApprovedPatchesEnableNonSecurity { ... }` (which could only
ever read `true`) became `if input.ApprovedPatchesEnableNonSecurity != nil { ... }`, now able to
merge an explicit `false`. Locked in by a new table test,
`TestUpdatePatchBaseline_ApprovedPatchesEnableNonSecurityPointerSemantics`, covering all four
true/false x omitted/explicit combinations.

**ListCloudConnectors/ValidateCloudConnector MaxResults bounds**: the 2026-07-23 pass recorded this
as a legitimate proven-impossibility (checked both the SDK's Go doc comments and a public API
reference search at the time; neither had a published bound for this brand-new, July-2026-added API
family). Re-checked this pass via `WebFetch` against
`https://docs.aws.amazon.com/systems-manager/latest/APIReference/API_ListCloudConnectors.html` and
`.../API_ValidateCloudConnector.html` directly (not a search-engine summary, which surfaced
unreliable noise from an unrelated AWS IoT service also named "ListCloudConnectors" before the
direct fetch): both pages are now live and state exact bounds — `ListCloudConnectorsInput.MaxResults`
"Valid Range: Minimum value of 0. Maximum value of 10.", `ValidateCloudConnectorInput.MaxResults`
"Minimum value of 0. Maximum value of 75." These pages were apparently published sometime between
the 2026-07-23 and 2026-07-24 audit passes — the prior pass's "not yet published" finding was
accurate as of when it was made. Replaced the shared `defaultCloudConnectorMaxResults=50` guess
with two operation-specific constants (`listCloudConnectorsMaxResults=10`,
`validateCloudConnectorMaxResults=75`) and, matching this package's existing convention for
range-constrained `MaxResults` fields (`GetParametersByPath`, `DescribeParameters`), replaced the
previous silent accept-any-positive-value handling with a real `ValidationException` for any value
outside `[0, bound]` — including honoring the documented minimum of literal `0` (previously any
value `<= 0` silently fell back to the default instead of being treated as a valid explicit
request for zero items). Locked in by a new table test, `Test_CloudConnector_MaxResultsBounds`.

**ValidateCloudConnector's Azure-call limitation** (gap, not closed, genuinely can't be):
re-confirmed this pass. gopherstack has no Azure tenant, no Azure credentials, and — as a locally
running AWS-API emulator — no business making an unbounded outbound HTTPS call to a third-party
cloud provider from inside a request handler even if credentials were somehow available (no such
egress path exists in the test/dev sandbox this runs in, and doing so would be a meaningful
architectural and security departure from how every other emulated AWS service in this codebase
behaves — all state is local and deterministic). `ValidateCloudConnector`'s `ValidationFindings`
continue to be derived deterministically from the connector's own stored `Configuration`, which is
the same category of limitation as KMS being a local AES-256 emulation rather than a real HSM call.

### parity-3 phase-2 (2026-07-24): Parameter policy notifications (NoChangeNotification / ExpirationNotification)

Real AWS SSM enforces parameter policies via periodic background scans (confirmed via the
parameter-store-policies user guide: "Parameter Store enforces parameter policies by using
asynchronous, periodic scans") and, for `ExpirationNotification`/`NoChangeNotification`
specifically, publishes an EventBridge event when a policy becomes due (confirmed via
`sysman-paramstore-cwe.html`'s "Parameter policy" event pattern example): `source: "aws.ssm"`,
`detail-type: "Parameter Store Policy Action"`, `detail: {"parameter-name": <name>,
"policy-type": "Expiration"|"ExpirationNotification"|"NoChangeNotification"}`. Prior to this pass,
gopherstack stored and round-tripped the `Policies` string but never evaluated
`ExpirationNotification`/`NoChangeNotification` at all (only `Expiration` was enforced, via the
pre-existing janitor sweep that deletes the parameter outright).

Implemented as a new janitor ticker, `sweepParameterPolicyNotifications`
(`parameter_policy_notifications.go`, wired into `janitor.go`'s `Run`/`SweepOnce`), which on each
tick:
1. Parses every parameter's `Policies` JSON into a generic `parameterStorePolicy{Type, Version,
   Attributes}` shape (rather than the pre-existing `Expiration`-only `parameterExpirationPolicy`).
2. Computes a due-time for each `ExpirationNotification` (`Before` `Unit` ahead of the parameter's
   `Expiration` policy timestamp, if any — an `ExpirationNotification` with no `Expiration` policy
   on the same parameter has nothing to count down to and never fires) and `NoChangeNotification`
   (`After` `Unit` after `LastModifiedDate`) policy, supporting both AWS's documented `Unit` values
   (`Days`, `Hours`).
3. Reports each newly-due policy instance through an injectable `ParameterPolicyNotifier` interface
   (`NotifyParameterPolicyAction(ctx, parameterName, policyType) error`) — the same
   injected-cross-service-hook pattern `services/stepfunctions/asl.EventBridgeIntegration` uses,
   deliberately keeping this package free of any direct dependency on `services/eventbridge`.
4. Dedupes per (parameter name, policy Type+Attributes) so a policy instance notifies at most once
   until the parameter is next written — `PutParameter` always resets `LastModifiedDate` and
   wholesale-replaces `Policies` (confirmed via the user guide: "If you add a new policy to a
   parameter that already has policies, Systems Manager overwrites the policies attached to the
   parameter"; and specifically for `NoChangeNotification`: "If you change or edit a parameter, the
   system resets the notification time period"), so `PutParameter`/`DeleteParameter`/
   `DeleteParameters`/the existing expiry sweep all now call
   `clearParameterPolicyNotificationStateLocked` to invalidate/cascade-clean this dedupe state —
   proven by `TestSSMJanitor_ParameterPolicyNotifications`'s `put_parameter_resets_dedupe_state` and
   `delete_then_recreate_leaves_no_ghost_dedupe_state` cases.
5. A `nil` notifier (the default, until wired) makes the whole sweep a cheap no-op that evaluates
   and marks nothing — so a policy that becomes due before a real notifier is injected is still
   reported once the notifier lands, rather than being silently lost to premature dedup-marking.

New backend state: `notifiedParameterPolicies map[string]map[string]map[string]struct{}` (region ->
parameter name -> dedupe key), added to `store.go` (init in `NewInMemoryBackend`/`Reset`) and fully
wired into `persistence.go`'s `Snapshot`/`Restore` (own JSON field, `initSnapshotPatchOpsFields`-style
nil-init on restore of an older snapshot).

**The real EventBridge-side adapter is implemented, not deferred**: `services/eventbridge/
ssm_integration.go` adds `(*eventbridge.InMemoryBackend).NotifyParameterPolicyAction`, implementing
`ssm.ParameterPolicyNotifier` directly on the EventBridge backend (mirroring
`services/eventbridge/sfn_integration.go`'s `SFNPutEvents` — a provider-package adapter satisfying a
consumer package's interface by name, so no wrapper struct is needed), translating the notification
into a real `PutEvents` call with the exact documented wire shape. Proven end-to-end by
`TestNotifyParameterPolicyAction` (`services/eventbridge/put_events_test.go`), which uses an
EventBridge Archive with an `EventPattern` matching the exact `source`/`detail-type`/`policy-type`
values as an independent observer — the archive's `EventCount` only increments if the published
event genuinely matches that wire shape, not merely "PutEvents returned no error". A compile-time
assertion (`var _ ssm.ParameterPolicyNotifier = (*eventbridge.InMemoryBackend)(nil)`) locks in that
the adapter satisfies the interface.

**cli.go wiring still needed** (the one deliberately-undone piece, per this agent's explicit
instruction not to touch cli.go): a single call, following the existing pattern already in
`wireStepFunctionsServiceIntegrations` (e.g. `sfnBk.SetEventBridgeIntegration(ebBk)`), needs to be
added wherever the SSM and EventBridge backends are both resolved from the service registry:

```go
if ssmH, ok := ssmReg.(*ssmbackend.Handler); ok {
    if ssmBk, ok := ssmH.Backend.(*ssmbackend.InMemoryBackend); ok {
        if ebH, ok := ebReg.(*ebbackend.Handler); ok {
            if ebBk, ok := ebH.Backend.(*ebbackend.InMemoryBackend); ok {
                ssmBk.SetParameterPolicyNotifier(ebBk)
            }
        }
    }
}
```

Until this lands, `SetParameterPolicyNotifier` is never called in the running binary, so
`b.parameterPolicyNotifier` stays `nil` and the janitor sweep remains a no-op in production exactly
as it was before this pass — this pass changes nothing observable for a real client until that one
line is added, by design (no risk of a half-wired feature misbehaving in the interim).

## 2026-08-29: constraint-parameter sweep (a filter/sort/page limit silently not honoured)

Coherent slice audited: `DescribeParameters` and `GetParametersByPath`
(`api_op_DescribeParameters.go`, `api_op_GetParametersByPath.go`,
`types.ParameterStringFilter`, ssm@v1.73.4). Both share pagination
(`Marker`/`NextToken`+`MaxResults`) and filtering (`ParameterFilters`)
logic in `parameters.go`; pagination already filters-then-paginates
correctly (no bug there, unlike IAM's PathPrefix bug this campaign found
in the same pass) — this slice is scoped to the filter-*matching* code,
`paramMatchesFilter`.

**Found and fixed**: `ParameterStringFilter{Key:"Path"}` (documented valid
for `DescribeParameters`, `Option` `Recursive`|`OneLevel`) had no case in
`paramMatchesFilter`'s switch, so it fell into `default: return true` —
every parameter matched regardless of the filter, an over-permissive
silent no-op (class: read under no key at all / narrower-than-documented
vocabulary implemented as none). Added a dedicated `paramMatchesPathFilter`
handling both `Option` values against the parameter's full name (itself a
path). Proven by `TestDescribeParameters_PathFilter`
(`list_filter_params_test.go`, `OneLevel`/`Recursive` subtests), both
failing against unmodified code (returned every parameter, including a
non-descendant).

**Checked and left as-is**: `Name`/`Type`/`KeyId`/`Tier`/`DataType` keys
with `Equals`/`BeginsWith`/`Contains` options are all correctly read and
applied (verified by walking `paramMatchesFilter` field-by-field against
`ParameterMetadata`). `DescribeParameters`/`GetParametersByPath`'s own
filter-then-paginate order (`parameters.go`) is correct — filters are
applied to the full unpaginated set before `Marker`/`MaxResults`
windowing, so truncation is never miscomputed the way IAM's PathPrefix was
this same sweep.

**Disclosed, not fixed** (documented judgment call, not silently skipped):
- `ParameterStringFilter{Key:"Label"}` — documented valid for
  `GetParametersByPath` only (the reverse of `Path`) — has no case either,
  same `default: return true` no-op. Not fixed this pass: real semantics
  are not a simple boolean filter, they also change *which stored version's
  value* is returned (the labeled version, not necessarily latest), which
  needs `b.parameterLabels`/`b.history` plumbed into `collectPathParams`'s
  per-parameter value selection, not just its match predicate — a larger,
  riskier change than this slice's scope. Left as a named gap rather than
  a half-correct boolean-only match.
- `ParameterStringFilter{Key:"tag:<TagKey>"}` — documented valid for
  `DescribeParameters` — same no-op. Not fixed: needs the per-region
  `b.tags` map threaded into `paramMatchesFilter`, which currently only
  sees `ParameterMetadata` (no tag data). Structural, same reasoning as
  Label.
- The comment above `paramMatchesFilter` previously asserted "Returns an
  error for unrecognised filter keys (AWS behavior)" while the code did
  the opposite (silently matched everything) — corrected the comment to
  describe the actual behavior rather than fix silently-match-everything
  into an error, since AWS's real validation behavior for a genuinely
  unrecognized key was not independently confirmed this pass.

Not covered this pass: the rest of ssm's ~50 List/Describe operations
(DescribeInstanceInformation, DescribeAutomationExecutions, ListCommands,
ListCommandInvocations, DescribeSessions, DescribeMaintenanceWindows,
DescribeOpsItems, etc.) were not re-audited for this constraint-parameter
class — scoped to the parameters family after it surfaced a live bug.

Gates: `go build ./...`, `go vet ./...` (repo-wide, clean),
`go test -race -count=1 ./services/ssm/...` (pass), `golangci-lint run
./services/ssm/...` (0 issues after splitting `paramMatchesFilter`'s
Equals/BeginsWith/Contains loop into `fieldMatchesFilterOption` to stay
under the `cyclop` budget — no `nolint`, per this repo's ban).

## Map-walk pagination sweep (2026-08-30, fix/wrapper-key-sweep-rds-cloudwatch-sqs-sns)

Audited every `sort.Slice`/`sort.SliceStable` call and every hand-rolled
offset-pagination site (`parseNextToken`/`paginateSlice`) in `services/ssm`
for the "sort on a tie-prone field over `store.Table.All()`/`Range()` (a Go
map walk, unstable between calls), no unique tiebreak" bug class — and,
separately, for a listing with NO sort at all feeding `paginateSlice`'s
offset cursor, which fails the same way even without a tie. Discriminator:
`Table.All()`/`Range()` (unstable between calls) is a bug source; `Index.Get()`
and a direct per-key slice lookup (`map[string][]T` accessed by one key) are
insertion-ordered and stable, so a non-unique/no sort there is provably
harmless and was left alone.

**Bugs found and fixed** (each proven first: construct >1 record sharing the
sort key or, where none existed, just enough records to exceed one page;
walk pages 30x; confirm the concatenation reproduces the full set with no
drops/duplicates; confirm the test fails on unmodified code, usually on
iteration 0):

- `DescribeActivations` (activations.go) — built its list from
  `activations.All()` and handed it straight to `paginateSlice` with **no
  sort at all**. Fixed: sort by `ActivationID` (the table's own key, unique).
- `DescribeAutomationExecutions` (automations.go) — sorted by `StartTime`
  alone over `automationExecutionsStore.All()`; `StartTime` is not the table
  key and ties (two executions starting at the same instant) are plausible.
  Fixed: added `AutomationExecutionID` as tiebreak.
- `ListAssociations` (associations.go) — no sort at all over
  `associationsStore.All()`. Fixed: sort by `AssociationID` (table key).
- `DescribeMaintenanceWindows` (maintenance_window.go) — no sort at all over
  `maintenanceWindowsStore.All()`. Fixed: sort by `WindowID` (table key).
- `DescribeMaintenanceWindowsForTarget` (maintenance_window.go) — matched
  window IDs were collected into a local `map[string]struct{}` (a second,
  independent layer of unspecified Go map order) and ranged with no sort.
  Fixed: sort by `WindowID` (table key) after building `identities`.
- `GetInventory` (inventory.go) — no sort at all over a raw
  `map[string][]InventoryItem` walk (`b.inventory`, keyed by instance ID).
  Fixed: sort by `ID` (the map's own key, unique).
- `ListComplianceItems` (inventory.go) — no sort at all over a raw
  `map[string][]ComplianceItem` walk (`b.compliance`, keyed by resource ID);
  items *within* one resource ID were already insertion-ordered (the whole
  slice is replaced atomically by `PutComplianceItems`), so only the outer
  per-resource grouping order was unstable. Fixed: `sort.SliceStable` by
  `ResourceID` — stable, not `sort.Slice`, specifically to preserve that
  already-correct within-group order.
- `ListComplianceSummaries` (inventory.go) — no sort at all over a
  `map[string]*complianceTally` keyed by `ComplianceType`. Fixed: sort by
  `ComplianceType` (the map's own key, unique).
- `ListResourceComplianceSummaries` (inventory.go) — no sort at all over the
  same raw `b.compliance` map walk, keyed by resource ID. Fixed: sort by
  `ResourceID` (the map's own key, unique).
- `DescribeOpsItems` (ops_items.go) — no sort at all over
  `opsItemsStore.All()`. Fixed: sort by `OpsItemID` (table key).
- `ListOpsItemRelatedItems` (ops_items.go) — when `OpsItemId` is omitted
  (a real, optional input member), flattens `opsItemRelatedItemsStore` (a
  raw `map[string][]OpsItemRelatedItem` keyed by OpsItem ID) with no sort.
  Fixed: sort by `AssociationID`, which `AssociateOpsItemRelatedItem` always
  assigns via `uuid.NewString()` and is therefore globally unique regardless
  of which OpsItem it belongs to.
- `DescribePatchBaselines` (patch_baselines.go) — no sort at all over
  `patchBaselinesStore.All()`. Fixed: sort by `BaselineID` (table key).

**Confirmed clean (tie-prone sort, but over a stable source, or key is
already unique) — left unchanged, with the reason:**
- Every sort keyed on a `store.Table`'s own key field over `.All()`
  (`ListResourceDataSync`/SyncName, `ListCommands`/CommandID,
  `ListDocuments`/Name, `DescribeMaintenanceWindows`-window helper via
  `windowScopedPage`/WindowTaskID+WindowTargetID, `buildNodeInfos`/
  InstanceID, `DescribeEffectiveInstanceAssociations`+
  `DescribeInstanceAssociationsStatus`/AssociationID,
  `DescribeInstancePatchStates(ForPatchGroup)`/InstanceID,
  `DescribeInstanceProperties`/InstanceID (dedup-merged from two sources,
  still unique), `ListAll`/`collectPathParams`/`DescribeParameters`/Name,
  `ListOpsMetadata`/OpsMetadataArn, `DescribeSessions`/SessionID,
  `ListStacks`… wait, that's cloudformation — see that service's note) can
  never tie, so map-walk instability is unobservable regardless.
- `ListCommandInvocations` (commands.go) sorts a raw `map[string][]T` walk
  by `(CommandID, InstanceID)`; that composite is unique per invocation
  (one invocation per instance per command, written once), so ties are
  structurally impossible.
- `DescribePatchProperties` (patch_baselines.go) sorts by `BaselineName`
  alone over a map walk, but a `seen["OS:Name"]` dedup guard runs first and
  `OperatingSystem` is a required, fixed input — so within one call the
  surviving set already has unique `BaselineName` values.
- `ListCloudConnectors` reads `cloudConnectorsStore.Snapshot()`, not
  `.All()` — `Snapshot()` is documented key-sorted and deterministic.
- Every `Index.Get()`-sourced or direct-per-key-slice-sourced list
  (`DescribeDocumentPermission`, `ListDocumentVersions`, `GetResourcePolicies`,
  `DescribeInventoryDeletions`, `DescribeAssociationExecutions`,
  `DescribeAssociationExecutionTargets`, `DescribeAutomationStepExecutions`,
  `ListNodes` via `buildNodeInfos`) — insertion-ordered, stable across calls,
  so no sort (or a tie-prone one) is provably harmless.

**PARITY claims checked, not just trusted**: this file's own header block
(above) documents an earlier pagination-population sweep with a long list of
ops fixed for a *different* bug (NextToken never populated at all). None of
that block's claims were relied on without re-reading the current code —
every op touched this pass was re-read from source, not from the prior
note's description of it, per this repo's standing "PARITY notes have been
wrong nine times" caution.

**Existing-test gap**: no pre-existing test in this package constructed a
tie and walked pages asserting item-identity reproduction; pagination tests
here asserted page sizes / NextToken presence / that *a* item appeared, not
that the full set survives a multi-page walk under randomized source order.
New tests added this pass (`activations_test.go`, `automations_test.go`,
`pagination_tie_sweep_test.go`) all assert exact reproduction of the full ID
set across a 30-iteration page walk, per bug.

**Unaudited this pass**: `resources_*`-style single-collaboration/service
listings outside the sort/pagination surface (already covered by the header
block's own pass); `evictDeletedStacks`-equivalents don't exist in ssm, but
note the analogous internal (non-paginated) eviction/GC helpers elsewhere in
this codebase were explicitly treated as out of scope for this bug class
(no customer-facing page boundary exists for them to corrupt).

Gates: `go build ./services/ssm/...`, `go vet ./services/ssm/...`,
`go test -race -count=1 ./services/ssm/...` (pass), `golangci-lint run
./services/ssm/...` (0 issues; one `dupl` finding between `ListAssociations`
and `ListOpsMetadata` — mirrored shapes are the fix itself, not copy-paste —
suppressed with `//nolint:dupl` on both, not a banned type).

**2026-08-30 (gopherstack-r3pr fabricated-error-code re-audit, no code change)**:
re-ran `cmd/errcodeaudit`; both confident findings (`ErrExecutionPreviewNotFound`
"ExecutionPreviewNotFoundException", `ErrInventoryNotFound` "InventoryTypeNotFound",
errors.go:39/49) independently re-confirmed dead — `grep` across the package finds
each only in `errors.go` and this file, never `errors.Is`-checked or raised at any
call site, so the literal never reaches a response writer. Matches the existing
"declared but never raised" record above; no correction needed.
